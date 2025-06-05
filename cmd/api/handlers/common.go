package handlers

import (
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/mastodon"
	"github.com/aron23/lesser/pkg/storage"
	"go.uber.org/zap"
)

// Handler contains shared dependencies for all handlers
type Handler struct {
	cfg            *config.Config
	store          storage.Storage
	logger         *zap.Logger
	authMiddleware *auth.Middleware
	converter      mastodon.Converter
}

// NewHandler creates a new handler with all dependencies
func NewHandler(cfg *config.Config, store storage.Storage, logger *zap.Logger, authMiddleware *auth.Middleware) *Handler {
	converter := mastodon.NewConverter(cfg.BaseURL())
	return &Handler{
		cfg:            cfg,
		store:          store,
		logger:         logger,
		authMiddleware: authMiddleware,
		converter:      converter,
	}
}
