package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// Handler contains dependencies for API handlers.
type Handler struct {
	cfg           *config.Config
	repos         core.RepositoryStorage
	logger        *zap.Logger
	converter     mastodon.Converter
	businessLogic services.BusinessLogicService
	authService   services.AuthenticationService
	registry      ServiceRegistry
	streamQueue   streaming.StreamQueueService
	remoteSearch  remoteSearchServiceFactory
	soulsService  soulHandlerService
	skillsService skillHandlerService

	// DataLoader instances for batched data loading to prevent N+1 queries
	loaders *graph.Loaders

	// Additional business logic frameworks for enhanced semantic consolidation
	commonBusinessLogic *common.BusinessLogicService
	activityPubLogic    *common.ActivityPubBusinessLogic
	mastodonLogic       *common.MastodonBusinessLogic

	legacyTrustConfigWarnOnce       sync.Once
	legacyTranslationConfigWarnOnce sync.Once
	legacyTipsConfigWarnOnce        sync.Once
}

type liftAuthResponder func(*apptheory.Context) (*apptheory.Response, error)

// streamingEventEmitter adapts streaming.StreamQueueService to common.EventEmitter interface
type streamingEventEmitter struct {
	streamQueue streaming.StreamQueueService
}

// EmitEvents implements the common.EventEmitter interface
func (e *streamingEventEmitter) EmitEvents(ctx context.Context, events []*common.StreamingEvent) error {
	// Convert common.StreamingEvent to streaming events and queue them
	for _, event := range events {
		// Queue the event using the stream queue service - use default stream for now
		if err := e.streamQueue.QueueEventForStream(ctx, "user", event.Type, event.Metadata); err != nil {
			return err
		}
	}

	return nil
}

// NewHandler creates a new handler with dependencies
func NewHandler(cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, streamQueue streaming.StreamQueueService) *Handler {
	// Create emoji repository
	emojiRepo := repositories.NewEmojiRepository(repos.GetDB(), cfg.DynamoTableName, logger, nil)

	// Create converter with emoji repository access
	converter := mastodon.NewConverterWithEmojis(cfg.BaseURL(), emojiRepo)

	// Create service layer
	serviceConfig := &services.ServiceConfig{
		BaseURL:   cfg.BaseURL(),
		JWTSecret: cfg.JWTSecret,
		Config:    cfg, // Add full config reference
	}
	serviceFactory := services.NewServiceFactory(repos, serviceConfig, logger)
	businessLogic := serviceFactory.CreateBusinessLogicService()
	authService := serviceFactory.CreateAuthenticationService()

	// Initialize service registry
	// Only supply dependencies we actually have to avoid validation failures
	registryOpts := []services.RegistryOption{
		services.WithStorage(repos),
		services.WithLogger(logger),
		services.WithConfig(serviceConfig),
	}

	// Wire streaming events through the Dynamo-backed queue so API requests never
	// require a direct websocket publisher.
	if streamQueue != nil {
		registryOpts = append(registryOpts, services.WithPublisher(streaming.NewQueuePublisher(streamQueue, logger)))
		logger.Info("initialized registry with streaming queue publisher")
	} else {
		logger.Warn("streamQueue is nil, registry will use noop publisher")
	}

	registryImpl, err := services.NewRegistry(registryOpts...)
	if err != nil {
		logger.Error("failed to initialize service registry", zap.Error(err))
		// Continue with nil registry for now - will be handled gracefully
	}
	registry := newServiceRegistry(registryImpl)

	// Initialize enhanced business logic frameworks
	streamingEmitter := &streamingEventEmitter{streamQueue: streamQueue}
	commonBusinessLogic := common.NewBusinessLogicService(logger, streamingEmitter, cfg.Domain)
	federationConfig := &common.FederationConfig{
		Domain:         cfg.Domain,
		UserAgent:      "Lesser/1.0",
		MaxRetries:     3,
		RetryDelay:     5 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
	activityPubLogic := common.NewActivityPubBusinessLogic(federationConfig, logger)
	mastodonConfig := common.DefaultMastodonConfig()
	mastodonConfig.Domain = cfg.Domain
	mastodonAPILogic := common.NewMastodonBusinessLogic(mastodonConfig, logger)

	// Initialize DataLoader instances for batched data loading
	loaders := graph.NewLoaders(repos, logger)

	return &Handler{
		cfg:                 cfg,
		repos:               repos,
		logger:              logger,
		converter:           converter,
		businessLogic:       businessLogic,
		authService:         authService,
		registry:            registry,
		streamQueue:         streamQueue,
		remoteSearch:        defaultRemoteSearchServiceFactory,
		soulsService:        nil,
		skillsService:       nil,
		loaders:             loaders,
		commonBusinessLogic: commonBusinessLogic,
		activityPubLogic:    activityPubLogic,
		mastodonLogic:       mastodonAPILogic,
	}
}

// getBearerTokenLift extracts Bearer token from Authorization header
func (h *Handler) getBearerTokenLift(ctx *apptheory.Context) string {
	authHeader := common.ExtractAuthHeader(ctx)

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ""
	}

	return token
}

func validateClaimsScopes(claims *auth.Claims) error {
	if claims == nil {
		return apperrors.Unauthorized("authentication required")
	}

	for _, scope := range claims.Scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if err := common.ValidateApplicationScopes(scope); err == nil {
			continue
		}
		if isLegacyLiftScope(scope) {
			continue
		}
		return auth.ErrInvalidToken
	}

	return nil
}

func isLegacyLiftScope(scope string) bool {
	switch strings.TrimSpace(scope) {
	case "moderation", "debug", "admin:write", "admin:all":
		return true
	default:
		return false
	}
}

func (h *Handler) authenticatedClaimsLift(ctx *apptheory.Context) (*auth.Claims, error) {
	if claims := auth.ClaimsFromAppTheoryContext(ctx); claims != nil {
		if err := validateClaimsScopes(claims); err != nil {
			return nil, err
		}
		return claims, nil
	}

	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return nil, apperrors.Unauthorized("authentication required")
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}

	if err := validateClaimsScopes(claims); err != nil {
		return nil, err
	}

	return claims, nil
}

func (h *Handler) optionalAuthenticatedClaimsLift(ctx *apptheory.Context) *auth.Claims {
	if claims := auth.ClaimsFromAppTheoryContext(ctx); claims != nil {
		if err := validateClaimsScopes(claims); err == nil {
			return claims
		}
		return nil
	}

	token := h.getBearerTokenLift(ctx)
	if strings.TrimSpace(token) == "" {
		return nil
	}

	claims, err := h.authenticatedClaimsLift(ctx)
	if err != nil {
		h.logger.Debug("optional authentication failed, continuing anonymously", zap.Error(err))
		return nil
	}

	return claims
}

func (h *Handler) authenticatedClaimsWithResponder(ctx *apptheory.Context, missingResponder, invalidResponder liftAuthResponder) (*auth.Claims, *apptheory.Response, error) {
	if claims := auth.ClaimsFromAppTheoryContext(ctx); claims != nil {
		if err := validateClaimsScopes(claims); err == nil {
			return claims, nil, nil
		}
		if invalidResponder == nil {
			return nil, nil, auth.ErrInvalidToken
		}
		resp, respErr := invalidResponder(ctx)
		return nil, resp, respErr
	}

	if err := common.ValidateRequiredParam("token", h.getBearerTokenLift(ctx)); err != nil {
		if missingResponder == nil {
			return nil, nil, apperrors.Unauthorized("authentication required")
		}
		resp, respErr := missingResponder(ctx)
		return nil, resp, respErr
	}

	claims, err := h.authenticatedClaimsLift(ctx)
	if err == nil {
		return claims, nil, nil
	}
	if invalidResponder == nil {
		return nil, nil, err
	}
	resp, respErr := invalidResponder(ctx)
	return nil, resp, respErr
}

func claimsHaveAnyScope(claims *auth.Claims, scopes ...string) bool {
	if claims == nil {
		return false
	}
	for _, scope := range scopes {
		if claims.HasScope(scope) {
			return true
		}
	}
	return false
}

// authenticateWithScope handles authentication and scope validation
func (h *Handler) authenticateWithScope(ctx *apptheory.Context, requiredScope string) (*auth.Claims, error) {
	// Validate required scope format using centralized validation
	if err := common.ValidateApplicationScopes(requiredScope); err != nil {
		return nil, apperrors.InternalWithCause(err, fmt.Sprintf("invalid required scope: %v", err))
	}

	claims, err := h.authenticatedClaimsLift(ctx)
	if err != nil {
		return nil, err
	}

	if !claims.HasScope(requiredScope) {
		return nil, apperrors.NewAuthError(apperrors.CodeInsufficientScope, fmt.Sprintf("insufficient scope: requires %s", requiredScope))
	}

	return claims, nil
}

// authenticateWithAnyScope authenticates and accepts any one of the provided scopes.
// This is used for Mastodon-compatible endpoints where tokens may carry either broad
// (`write`) or narrow (`follow`, `write:follows`, etc.) scopes.
func (h *Handler) authenticateWithAnyScope(ctx *apptheory.Context, requiredScopes ...string) (*auth.Claims, error) {
	if len(requiredScopes) == 0 {
		return nil, apperrors.Internal("no required scopes provided")
	}

	for _, scope := range requiredScopes {
		if err := common.ValidateApplicationScopes(scope); err != nil {
			return nil, apperrors.InternalWithCause(err, fmt.Sprintf("invalid required scope: %v", err))
		}
	}

	claims, err := h.authenticatedClaimsLift(ctx)
	if err != nil {
		return nil, err
	}

	for _, scope := range requiredScopes {
		if claims.HasScope(scope) {
			return claims, nil
		}
	}

	return nil, apperrors.NewAuthError(apperrors.CodeInsufficientScope, fmt.Sprintf("insufficient scope: requires one of %s", strings.Join(requiredScopes, ", ")))
}

// getOptionalAuthenticatedUser extracts user context if authentication is provided and valid
// Returns empty string if not authenticated or token is invalid (for public content access)
func (h *Handler) getOptionalAuthenticatedUser(ctx *apptheory.Context) string {
	if username := auth.UsernameFromAppTheoryContext(ctx); username != "" {
		return username
	}

	if claims := h.optionalAuthenticatedClaimsLift(ctx); claims != nil {
		return claims.Username
	}

	return ""
}
