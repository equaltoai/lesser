package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	relationshipsvc "github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// relationshipFromService converts service relationship to API format.
func (h *Handler) relationshipFromService(relationship *relationshipsvc.RelationshipData) models.Relationship {
	return models.Relationship{
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
}

// HandleMuteAccountLift handles POST /api/v1/accounts/:id/mute
func (h *Handler) HandleMuteAccountLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, err := h.authenticateWithScope(ctx, "write")
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

	// Validation will be handled by the service layer

	// Parse parameters with fallback
	hideNotifications := false
	var params models.MuteRequest

	// Try parsing as JSON first
	if err := common.ParseRequestWithFallback(ctx, &params); err == nil {
		if params.Notifications != nil {
			hideNotifications = *params.Notifications
		}
	} else {
		// Fallback to raw body parsing if ParseRequest fails
		if len(ctx.Request.Body) > 0 {
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
		result, err := h.registry.Relationships().Mute(ctx.Context(), &relationshipsvc.MuteCommand{
			MuterID:           username,
			MutedID:           accountID,
			MuteNotifications: hideNotifications,
		})
		if err != nil {
			h.logger.Error("failed to mute via service", zap.Error(err))
			return common.RespondInternalServerError(ctx)
		}

		return okJSON(h.relationshipFromService(result.Relationship))
	}

	// If we reach here, service is not available - return error
	return common.RespondInternalServerError(ctx)
}

// HandleUnmuteAccountLift handles POST /api/v1/accounts/:id/unmute
func (h *Handler) HandleUnmuteAccountLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, err := h.authenticateWithScope(ctx, "write")
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

	// Use Relationships service if available
	if h.registry != nil && h.registry.Relationships() != nil {
		result, err := h.registry.Relationships().Unmute(ctx.Context(), &relationshipsvc.UnmuteCommand{
			MuterID: username,
			MutedID: accountID,
		})
		if err != nil {
			h.logger.Error("failed to unmute via service", zap.Error(err))
			return common.RespondInternalServerError(ctx)
		}

		return okJSON(h.relationshipFromService(result.Relationship))
	}

	// If we reach here, service is not available - return error
	return common.RespondInternalServerError(ctx)
}

// HandleGetMutedAccountsLift handles GET /api/v1/mutes
func (h *Handler) HandleGetMutedAccountsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, "read")
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

	// Parse pagination parameters
	limit := 40
	if limitStr := queryValue(ctx, "limit"); limitStr != "" {
		if parsed, err := common.ParseFollowLimit(limitStr); err == nil {
			limit = parsed
		}
	}

	cursor := queryValue(ctx, "max_id")

	// Use Relationships service if available
	if h.registry != nil && h.registry.Relationships() != nil {
		result, err := h.registry.Relationships().GetMutedUsers(ctx.Context(), &relationshipsvc.GetMutedUsersQuery{
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
			resp, err := okJSON(accounts)
			if err != nil {
				return nil, err
			}
			if result.NextCursor != "" {
				setHeader(resp, "Link", fmt.Sprintf("<%s/api/v1/mutes?max_id=%s>; rel=\"next\"", h.cfg.BaseURL(), result.NextCursor))
			}

			return resp, nil
		}
	}

	// If we reach here, service failed - return error
	return common.RespondInternalServerError(ctx)
}
