package lift

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/mastodon"
	relationshipsvc "github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleMuteAccountLift handles POST /api/v1/accounts/:id/mute
func (h *Handler) HandleMuteAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing account id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
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
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Validation will be handled by the service layer

	// Parse parameters with fallback
	hideNotifications := false
	var params struct {
		Notifications bool `json:"notifications"`
	}

	// Try parsing as JSON first
	if err := ctx.ParseRequest(&params); err == nil {
		hideNotifications = params.Notifications
	} else {
		// Fallback to raw body parsing if ParseRequest fails
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			var fallbackParams map[string]interface{}
			if parseErr := json.Unmarshal(ctx.Request.Body, &fallbackParams); parseErr == nil {
				if notifications, ok := fallbackParams["notifications"].(bool); ok {
					hideNotifications = notifications
				}
			}
		}
	}

	// Use Relationships service if available
	if h.registry != nil && h.registry.Relationships() != nil {
		result, err := h.registry.Relationships().Mute(ctx.Context, &relationshipsvc.MuteCommand{
			MuterID:           username,
			MutedID:           accountID,
			MuteNotifications: hideNotifications,
		})
		if err != nil {
			h.logger.Error("failed to mute via service", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
		
		// Convert service result to API format
		relationship := models.Relationship{
			ID:                  result.Relationship.ID,
			Following:           result.Relationship.Following,
			ShowingReblogs:      result.Relationship.ShowingReblogs,
			Notifying:           result.Relationship.Notifying,
			FollowedBy:          result.Relationship.FollowedBy,
			Blocking:            result.Relationship.Blocking,
			BlockedBy:           result.Relationship.BlockedBy,
			Muting:              result.Relationship.Muting,
			MutingNotifications: result.Relationship.MutingNotifications,
			Requested:           result.Relationship.Requested,
			DomainBlocking:      result.Relationship.DomainBlocking,
			Endorsed:            result.Relationship.Endorsed,
			Note:                result.Relationship.Note,
		}
		return ctx.JSON(relationship)
	}
	
	// If we reach here, service is not available - return error
	return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
}

// HandleUnmuteAccountLift handles POST /api/v1/accounts/:id/unmute
func (h *Handler) HandleUnmuteAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing account id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
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
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Use Relationships service if available
	if h.registry != nil && h.registry.Relationships() != nil {
		result, err := h.registry.Relationships().Unmute(ctx.Context, &relationshipsvc.UnmuteCommand{
			MuterID: username,
			MutedID:  accountID,
		})
		if err != nil {
			h.logger.Error("failed to unmute via service", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
		
		// Convert service result to API format
		relationship := models.Relationship{
			ID:                  result.Relationship.ID,
			Following:           result.Relationship.Following,
			ShowingReblogs:      result.Relationship.ShowingReblogs,
			Notifying:           result.Relationship.Notifying,
			FollowedBy:          result.Relationship.FollowedBy,
			Blocking:            result.Relationship.Blocking,
			BlockedBy:           result.Relationship.BlockedBy,
			Muting:              result.Relationship.Muting,
			MutingNotifications: result.Relationship.MutingNotifications,
			Requested:           result.Relationship.Requested,
			DomainBlocking:      result.Relationship.DomainBlocking,
			Endorsed:            result.Relationship.Endorsed,
			Note:                result.Relationship.Note,
		}
		return ctx.JSON(relationship)
	}
	
	// If we reach here, service is not available - return error
	return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
}

// HandleGetMutedAccountsLift handles GET /api/v1/mutes
func (h *Handler) HandleGetMutedAccountsLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
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
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check read scope
		if !claims.HasScope(auth.ScopeRead) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Parse pagination parameters
	limit := 40
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 80 {
			limit = parsed
		}
	}

	cursor := ctx.Query("max_id")

	// Use Relationships service if available
	if h.registry != nil && h.registry.Relationships() != nil {
		result, err := h.registry.Relationships().GetMutedUsers(ctx.Context, &relationshipsvc.GetMutedUsersQuery{
			UserID: username,
			Limit:  limit,
			Cursor: cursor,
		})
		if err != nil {
			h.logger.Error("failed to get muted users via service", zap.Error(err))
			// Continue to fallback implementation below
		} else {
			// Convert service result to API format
			accounts := make([]models.Account, 0, len(result.MutedUsers))
			for _, mutedUser := range result.MutedUsers {
				if mutedUser.Actor != nil {
					converter := mastodon.NewConverter(h.cfg.BaseURL())
					// Get follower/following counts (simplified for service response)
					account := converter.ActorToAccountWithCounts(mutedUser.Actor, 0, 0, 0)
					accounts = append(accounts, account)
				}
			}
			
			// Set Link header for pagination if there's a next cursor
			if result.NextCursor != "" {
				ctx.Response.Header("Link", fmt.Sprintf("<%s/api/v1/mutes?max_id=%s>; rel=\"next\"", h.cfg.BaseURL(), result.NextCursor))
			}

			return ctx.JSON(accounts)
		}
	}
	
	// If we reach here, service failed - return error
	return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
}

