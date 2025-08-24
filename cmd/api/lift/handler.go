package lift

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Handler contains dependencies for Lift handlers
type Handler struct {
	cfg            *config.Config
	repos          core.RepositoryStorage
	logger         *zap.Logger
	authMiddleware lift.Middleware
	converter      mastodon.Converter
	businessLogic  services.BusinessLogicService
	authService    services.AuthenticationService
	registry       *services.Registry
	streamQueue    streaming.StreamQueueService

	// Additional business logic frameworks for enhanced semantic consolidation
	commonBusinessLogic *common.BusinessLogicService
	activityPubLogic    *common.ActivityPubBusinessLogic
	mastodonLogic       *common.MastodonBusinessLogic
}

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
func NewHandler(cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, authMiddleware lift.Middleware, streamQueue streaming.StreamQueueService) *Handler {
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
	// Note: Registry still expects Publisher interface, but we're using StreamQueueService
	// This is fine as the registry is for future service-first architecture
	registry, err := services.NewRegistry(
		services.WithStorage(repos),
		services.WithPublisher(nil), // Publisher will be handled by stream-router, not API
		services.WithLogger(logger),
		services.WithConfig(serviceConfig),
	)
	if err != nil {
		logger.Error("failed to initialize service registry", zap.Error(err))
		// Continue with nil registry for now - will be handled gracefully
	}

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

	return &Handler{
		cfg:                 cfg,
		repos:               repos,
		logger:              logger,
		authMiddleware:      authMiddleware,
		converter:           converter,
		businessLogic:       businessLogic,
		authService:         authService,
		registry:            registry,
		streamQueue:         streamQueue,
		commonBusinessLogic: commonBusinessLogic,
		activityPubLogic:    activityPubLogic,
		mastodonLogic:       mastodonAPILogic,
	}
}

// getBearerTokenLift extracts Bearer token from Authorization header
func (h *Handler) getBearerTokenLift(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ""
	}

	return token
}

// authenticateWithScope handles authentication and scope validation
func (h *Handler) authenticateWithScope(ctx *lift.Context, requiredScope string) (*auth.Claims, error) {
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return nil, common.RespondMissingAuth(ctx)
	}

	// Validate required scope format using centralized validation
	if err := common.ValidateApplicationScopes(requiredScope); err != nil {
		return nil, common.RespondBadRequest(ctx, fmt.Sprintf("invalid required scope: %v", err))
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, common.RespondUnauthorized(ctx, err.Error())
	}

	// Validate token scopes using centralized validation
	tokenScopes := strings.Join(claims.Scopes, " ")
	if err := common.ValidateApplicationScopes(tokenScopes); err != nil {
		return nil, common.RespondForbidden(ctx, fmt.Sprintf("invalid token scopes: %v", err))
	}

	if !claims.HasScope(requiredScope) {
		return nil, common.RespondInsufficientScope(ctx, requiredScope)
	}

	return claims, nil
}

// getOptionalAuthenticatedUser extracts user context if authentication is provided and valid
// Returns empty string if not authenticated or token is invalid (for public content access)
func (h *Handler) getOptionalAuthenticatedUser(ctx *lift.Context) string {
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return ""
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		// Token validation failed but we continue for public content
		h.logger.Debug("Token validation failed, continuing for public content", zap.Error(err))
		return ""
	}

	return claims.Username
}
