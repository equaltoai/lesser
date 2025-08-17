package lift

import (
	"net/http"

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
	authMiddleware *auth.Middleware
	converter      mastodon.Converter
	businessLogic  services.BusinessLogicService
	authService    services.AuthenticationService
	registry       *services.Registry
	streamQueue    streaming.StreamQueueService
}

// NewHandler creates a new handler with dependencies
func NewHandler(cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, authMiddleware *auth.Middleware, streamQueue streaming.StreamQueueService) *Handler {
	// Create emoji repository
	emojiRepo := repositories.NewEmojiRepository(repos.GetDB(), logger)
	
	// Create converter with emoji repository access
	converter := mastodon.NewConverterWithEmojis(cfg.BaseURL(), emojiRepo)

	// Create service layer
	serviceConfig := &services.ServiceConfig{
		BaseURL:   cfg.BaseURL(),
		JWTSecret: cfg.JWTSecret,
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

	return &Handler{
		cfg:            cfg,
		repos:          repos,
		logger:         logger,
		authMiddleware: authMiddleware,
		converter:      converter,
		businessLogic:  businessLogic,
		authService:    authService,
		registry:       registry,
		streamQueue:    streamQueue,
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
		return nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	if !claims.HasScope(requiredScope) {
		return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
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

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		// Token validation failed but we continue for public content
		h.logger.Debug("Token validation failed, continuing for public content", zap.Error(err))
		return ""
	}

	return claims.Username
}
