package lift

import (
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetRelationshipsLift handles GET /api/v1/accounts/relationships
// It accepts multiple account IDs as query parameters: id[]=1&id[]=2
func (h *Handler) HandleGetRelationshipsLift(ctx *lift.Context) error {
	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{"read:follows", auth.ScopeRead})
	if err != nil {
		return err // Error response already set by authenticateUser
	}

	return h.handleRelationshipsLogic(ctx, username)
}

// handleRelationshipsLogic contains the main relationships logic, separated for testing
func (h *Handler) handleRelationshipsLogic(ctx *lift.Context, username string) error {
	// Extract account IDs from query parameters
	accountIDs := h.extractAccountIDsLift(ctx)
	if err := common.ValidateSliceNotEmpty("account_ids", accountIDs); err != nil {
		return common.RespondBadRequest(ctx, "no account IDs provided")
	}

	// Build relationships for each requested account
	relationships := make([]models.Relationship, 0, len(accountIDs))

	for _, accountID := range accountIDs {
		// Skip invalid IDs
		if err := common.ValidateAccountParamID(accountID); err != nil {
			h.logger.Debug("invalid account ID in relationship request", 
				zap.String("account_id", accountID), 
				zap.Error(err))
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
			return common.RespondInternalServerError(ctx, "failed to get relationships")
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
	// Use the shared parseArrayParam helper
	return h.parseArrayParam(ctx, "id")
}
