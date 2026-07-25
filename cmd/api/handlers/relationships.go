package handlers

import (
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

// HandleGetRelationshipsLift handles GET /api/v1/accounts/relationships
// It accepts multiple account IDs as query parameters: id[]=1&id[]=2
func (h *Handler) HandleGetRelationshipsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{"read:follows", auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	return h.handleRelationshipsLogic(ctx, username)
}

// handleRelationshipsLogic contains the main relationships logic, separated for testing
func (h *Handler) handleRelationshipsLogic(ctx *apptheory.Context, username string) (*apptheory.Response, error) {
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

		targetAccount, err := h.lookupStorageAccountByID(ctx.Context(), accountID)
		if err != nil || targetAccount == nil {
			// Skip accounts that don't exist
			h.logger.Warn("account not found for relationship",
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}

		targetLookupID := relationshipLookupTargetID(targetAccount, accountID)

		// Use the Relationships service to get relationship data
		relationshipData, err := h.registry.Relationships().GetRelationship(ctx.Context(), username, targetLookupID)
		if err != nil {
			h.logger.Error("failed to get relationship from service",
				zap.String("requester", username),
				zap.String("target", targetLookupID),
				zap.Error(err))
			return common.RespondInternalServerError(ctx, "failed to get relationships")
		}

		relationship := h.relationshipFromService(relationshipData)
		if publicID := strings.TrimSpace(h.publicAccountFromStorageAccount(targetAccount).ID); publicID != "" {
			relationship.ID = publicID
		}
		relationships = append(relationships, relationship)
	}

	return okJSON(relationships)
}

// extractAccountIDsLift extracts account IDs from query parameters
// Supports both id[]=1&id[]=2 and id=1,2 formats
func (h *Handler) extractAccountIDsLift(ctx *apptheory.Context) []string {
	// Use the shared parseArrayParam helper
	return h.parseArrayParam(ctx, "id")
}
