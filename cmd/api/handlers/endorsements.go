package handlers

import (
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleGetEndorsementsLift handles GET /api/v1/endorsements
// Returns accounts that the user has endorsed (pinned to their profile)
func (h *Handler) HandleGetEndorsementsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{"read:accounts", auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get pinned accounts using Accounts service
	result, err := h.registry.Accounts().GetAccountPins(ctx.Context(), &accounts.GetAccountPinsQuery{
		Username: username,
	})
	if err != nil {
		h.logger.Error("failed to get account pins", zap.Error(err))
		return h.respondWithError(ctx, 500, "Internal server error")
	}

	// Convert service result to API format using transformations
	apiAccounts := make([]models.Account, 0, len(result.PinnedAccounts))
	for _, account := range result.PinnedAccounts {
		if account.Actor == nil {
			continue
		}
		// Convert to account
		apiAccount := transformations.ActorToAccountBase(account.Actor, h.cfg.BaseURL())
		apiAccounts = append(apiAccounts, apiAccount)
	}

	return okJSON(apiAccounts)
}
