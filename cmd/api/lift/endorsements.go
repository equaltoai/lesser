package lift

import (
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetEndorsementsLift handles GET /api/v1/endorsements
// Returns accounts that the user has endorsed (pinned to their profile)
func (h *Handler) HandleGetEndorsementsLift(ctx *lift.Context) error {
	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{"read:accounts", auth.ScopeRead})
	if err != nil {
		return err // Error response already set by authenticateUser
	}

	// Get pinned accounts using Accounts service
	result, err := h.registry.Accounts().GetAccountPins(ctx.Context, &accounts.GetAccountPinsQuery{
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

	return ctx.JSON(apiAccounts)
}
