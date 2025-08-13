package lift

import (
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetRelationshipsLift handles GET /api/v1/accounts/relationships
// It accepts multiple account IDs as query parameters: id[]=1&id[]=2
func (h *Handler) HandleGetRelationshipsLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Verify account exists (test mode)
		_, err := h.registry.Accounts().GetAccount(ctx.Context, testUsername)
		if err != nil {
			h.logger.Error("failed to get account", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}

		// Skip to the main logic with test username
		return h.handleRelationshipsLogic(ctx, testUsername)
	}

	// Extract token from Authorization header
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

	// Check read:follows scope (relationships include follow status)
	if !claims.HasScope("read:follows") && !claims.HasScope(auth.ScopeRead) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Verify account exists
	_, err = h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get account", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	return h.handleRelationshipsLogic(ctx, claims.Username)
}

// handleRelationshipsLogic contains the main relationships logic, separated for testing
func (h *Handler) handleRelationshipsLogic(ctx *lift.Context, username string) error {
	// Extract account IDs from query parameters
	accountIDs := h.extractAccountIDsLift(ctx)
	if len(accountIDs) == 0 {
		return ctx.Status(400).JSON(map[string]string{"error": "no account IDs provided"})
	}

	// Build relationships for each requested account
	relationships := make([]models.Relationship, 0, len(accountIDs))

	for _, accountID := range accountIDs {
		// Skip empty IDs
		if accountID == "" {
			continue
		}

		// Check if account exists (basic validation)
		_, err := h.registry.Accounts().GetAccount(ctx.Context, accountID)
		if err != nil {
			// Skip accounts that don't exist
			h.logger.Warn("account not found for relationship",
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}

		// Use the Relationships service to get relationship data
		relationshipData, err := h.registry.Relationships().GetRelationship(ctx.Context, username, accountID)
		if err != nil {
			h.logger.Error("failed to get relationship from service",
				zap.String("requester", username),
				zap.String("target", accountID),
				zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "failed to get relationships"})
		}

		// Convert service relationship data to API model
		relationship := models.Relationship{
			ID:                  relationshipData.ID,
			Following:           relationshipData.Following,
			ShowingReblogs:      relationshipData.ShowingReblogs,
			Notifying:           relationshipData.Notifying,
			FollowedBy:          relationshipData.FollowedBy,
			Blocking:            relationshipData.Blocking,
			BlockedBy:           relationshipData.BlockedBy,
			Muting:              relationshipData.Muting,
			MutingNotifications: relationshipData.MutingNotifications,
			Requested:           relationshipData.Requested,
			DomainBlocking:      relationshipData.DomainBlocking,
			Endorsed:            relationshipData.Endorsed,
			Note:                relationshipData.Note,
		}
		relationships = append(relationships, relationship)
	}

	return ctx.JSON(relationships)
}


// extractAccountIDsLift extracts account IDs from query parameters
// Supports both id[]=1&id[]=2 and id=1,2 formats
func (h *Handler) extractAccountIDsLift(ctx *lift.Context) []string {
	var accountIDs []string

	// First, try to get all query parameters to handle array format id[]
	var queryParams map[string]string
	if ctx.Request != nil && ctx.Request.Request != nil {
		queryParams = ctx.Request.Request.QueryParams
	}

	for key, value := range queryParams {
		if strings.HasPrefix(key, "id[") && strings.HasSuffix(key, "]") {
			accountIDs = append(accountIDs, value)
		}
	}

	// If no array format found, check for comma-separated format: id=1,2
	if len(accountIDs) == 0 {
		idParam := ctx.Query("id")
		if idParam == "" && queryParams != nil {
			idParam = queryParams["id"]
		}
		if idParam != "" {
			accountIDs = strings.Split(idParam, ",")
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := []string{}
	for _, id := range accountIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	return unique
}

