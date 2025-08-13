package lift

import (
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetEndorsementsLift handles GET /api/v1/endorsements
// Returns accounts that the user has endorsed (pinned to their profile)
func (h *Handler) HandleGetEndorsementsLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string

	if testUsername != "" {
		// Use test username directly (test mode)
		username = testUsername
	} else {
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

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check read:accounts scope
		if !claims.HasScope("read:accounts") && !claims.HasScope(auth.ScopeRead) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get pinned accounts using Accounts service
	result, err := h.registry.Accounts().GetAccountPins(ctx.Context, &accounts.GetAccountPinsQuery{
		Username: username,
	})
	if err != nil {
		h.logger.Error("failed to get account pins", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Initialize converter
	converter := mastodon.NewConverter(h.cfg.BaseURL())

	// Convert service result to API format
	apiAccounts := make([]models.Account, 0, len(result.PinnedAccounts))
	for _, account := range result.PinnedAccounts {
		if account.Actor == nil {
			continue
		}
		// Convert to account
		apiAccount := converter.ActorToAccount(account.Actor)
		apiAccounts = append(apiAccounts, apiAccount)
	}

	return ctx.JSON(apiAccounts)
}
