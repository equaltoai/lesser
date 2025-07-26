package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/trends"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"go.uber.org/zap"
)

// Handler contains shared dependencies for all handlers
type Handler struct {
	cfg            *config.Config
	store          storage.Storage
	logger         *zap.Logger
	authMiddleware *auth.Middleware
	converter      mastodon.Converter
	trendService   *trends.Service
	emojiParser    *mastodon.EmojiParser
	dynamoClient   *dynamodb.Client
}

// NewHandler creates a new handler with all dependencies
func NewHandler(cfg *config.Config, store storage.Storage, logger *zap.Logger, authMiddleware *auth.Middleware) *Handler {
	converter := mastodon.NewConverter(cfg.BaseURL())
	trendService := trends.NewService(store)
	emojiParser := mastodon.NewEmojiParser(store)

	return &Handler{
		cfg:            cfg,
		store:          store,
		logger:         logger,
		authMiddleware: authMiddleware,
		converter:      converter,
		trendService:   trendService,
		emojiParser:    emojiParser,
	}
}

// resolveAccountID resolves an account ID (which can be a username, numeric ID, or URL) to an actor
func (h *Handler) resolveAccountID(ctx context.Context, accountID string) (*activitypub.Actor, error) {
	// Handle different account ID formats
	if strings.HasPrefix(accountID, "http://") || strings.HasPrefix(accountID, "https://") {
		// Full ActivityPub actor URL
		// Extract username from URL like https://lesser.host/users/aron
		if strings.Contains(accountID, h.cfg.Domain) && strings.Contains(accountID, "/users/") {
			parts := strings.Split(accountID, "/users/")
			if len(parts) == 2 {
				username := parts[1]
				return h.store.GetActor(ctx, username)
			}
			return nil, fmt.Errorf("invalid account URL")
		}
		// Remote actor - not supported yet
		return nil, fmt.Errorf("remote accounts not yet supported")
	}

	// Check if it's a numeric ID (Mastodon compatibility)
	if _, err := strconv.ParseInt(accountID, 10, 64); err == nil && len(accountID) >= 10 {
		// It's a numeric ID - use the dedicated lookup method
		return h.store.GetActorByNumericID(ctx, accountID)
	}

	// Assume it's a username for local accounts
	return h.store.GetActor(ctx, accountID)
}
