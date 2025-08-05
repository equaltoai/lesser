package lift

import (
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage/core"
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
}

// NewHandler creates a new handler with dependencies
func NewHandler(cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger, authMiddleware *auth.Middleware) *Handler {
	converter := mastodon.NewConverter(cfg.BaseURL())
	
	return &Handler{
		cfg:            cfg,
		repos:          repos,
		logger:         logger,
		authMiddleware: authMiddleware,
		converter:      converter,
	}
}

// getBearerTokenLift extracts Bearer token from Authorization header
func (h *Handler) getBearerTokenLift(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}
	
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ""
	}
	
	return token
}