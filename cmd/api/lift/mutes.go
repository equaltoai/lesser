package lift

import (
	"encoding/json"
	"fmt"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	relationshipsvc "github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)


// convertRelationshipResponse converts service relationship to API format
func (h *Handler) convertRelationshipResponse(ctx *lift.Context, relationship *relationshipsvc.RelationshipData) error {
	response := models.Relationship{
		ID:                  relationship.ID,
		Following:           relationship.Following,
		ShowingReblogs:      relationship.ShowingReblogs,
		Notifying:           relationship.Notifying,
		FollowedBy:          relationship.FollowedBy,
		Blocking:            relationship.Blocking,
		BlockedBy:           relationship.BlockedBy,
		Muting:              relationship.Muting,
		MutingNotifications: relationship.MutingNotifications,
		Requested:           relationship.Requested,
		DomainBlocking:      relationship.DomainBlocking,
		Endorsed:            relationship.Endorsed,
		Note:                relationship.Note,
	}
	return ctx.JSON(response)
}

// HandleMuteAccountLift handles POST /api/v1/accounts/:id/mute
func (h *Handler) HandleMuteAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	username, err := h.authenticateRequestWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
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
			return common.RespondInternalServerError(ctx)
		}

		return h.convertRelationshipResponse(ctx, result.Relationship)
	}

	// If we reach here, service is not available - return error
	return common.RespondInternalServerError(ctx)
}

// HandleUnmuteAccountLift handles POST /api/v1/accounts/:id/unmute
func (h *Handler) HandleUnmuteAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	username, err := h.authenticateRequestWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Use Relationships service if available
	if h.registry != nil && h.registry.Relationships() != nil {
		result, err := h.registry.Relationships().Unmute(ctx.Context, &relationshipsvc.UnmuteCommand{
			MuterID: username,
			MutedID: accountID,
		})
		if err != nil {
			h.logger.Error("failed to unmute via service", zap.Error(err))
			return common.RespondInternalServerError(ctx)
		}

		return h.convertRelationshipResponse(ctx, result.Relationship)
	}

	// If we reach here, service is not available - return error
	return common.RespondInternalServerError(ctx)
}

// HandleGetMutedAccountsLift handles GET /api/v1/mutes
func (h *Handler) HandleGetMutedAccountsLift(ctx *lift.Context) error {
	username, err := h.authenticateRequestWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Parse pagination parameters
	limit := 40
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsed, err := common.ParseFollowLimit(limitStr); err == nil {
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
					// Get follower/following counts (simplified for service response)
					account := transformations.ActorToAccountWithCounts(mutedUser.Actor, h.cfg.BaseURL(), 0, 0, 0)
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
	return common.RespondInternalServerError(ctx)
}
