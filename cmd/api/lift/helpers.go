package lift

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

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
				return h.repos.Actor().GetActor(ctx, username)
			}
			return nil, fmt.Errorf("invalid account URL")
		}
		// Remote actor - not supported yet
		return nil, fmt.Errorf("remote accounts not yet supported")
	}

	// Check if it's a numeric ID (Mastodon compatibility)
	if _, err := strconv.ParseInt(accountID, 10, 64); err == nil && len(accountID) >= 10 {
		// It's a numeric ID - use the dedicated lookup method
		return h.repos.Actor().GetActorByNumericID(ctx, accountID)
	}

	// Assume it's a username for local accounts
	return h.repos.Actor().GetActor(ctx, accountID)
}

// authenticateUser handles the common pattern of extracting and validating user authentication
// It supports both test mode (via X-Test-Username header) and production OAuth
func (h *Handler) authenticateUser(ctx *lift.Context, requiredScope string) (username string, err error) {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Test mode - skip auth
		return testUsername, nil
	}

	// Extract and validate token
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", fmt.Errorf("unauthorized")
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", fmt.Errorf("unauthorized")
	}

	// Check scope if provided
	if requiredScope != "" && !claims.HasScope(requiredScope) {
		return "", fmt.Errorf("insufficient scope")
	}

	return claims.Username, nil
}

// statusActionHandler provides a generic handler for status operations like bookmark, favorite, etc.
func (h *Handler) statusActionHandler(ctx *lift.Context, requiredScope string, action func(statusID, username string) (*models.Status, error)) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Authenticate user
	username, err := h.authenticateUser(ctx, requiredScope)
	if err != nil {
		if err.Error() == "insufficient scope" {
			return ctx.Status(403).JSON(map[string]string{"error": err.Error()})
		}
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Execute the action
	status, err := action(statusID, username)
	if err != nil {
		h.logger.Error("status action failed", 
			zap.String("action", "generic"),
			zap.String("status_id", statusID),
			zap.String("username", username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": err.Error()})
	}

	return ctx.JSON(status)
}

// getTestUsername extracts test username from headers
func (h *Handler) getTestUsername(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// getAuthHeader extracts authorization header from request
func (h *Handler) getAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// getQueryParam extracts query parameter from request
func (h *Handler) getQueryParam(ctx *lift.Context, key string) string {
	value := ctx.Query(key)
	if value == "" && ctx.Request != nil && ctx.Request.Request != nil {
		value = ctx.Request.Request.QueryParams[key]
	}
	return value
}
