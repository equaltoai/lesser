// relationships_full.go - Complete service-based implementation of relationship endpoints
// This implements Phase 4 with service layer integration

package handlers

import (
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

// HandleFollowAccountFull follows an account using Relationships service
func (h *Handler) HandleFollowAccountFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	var req models.FollowRequest
	_ = common.ParseRequestWithFallback(ctx, &req)

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	result, err := h.registry.Relationships().Follow(ctx.Context(), &relationships.FollowCommand{
		FollowerID:  claims.Username,
		FollowingID: accountID,
		ShowReblogs: req.Reblogs == nil || *req.Reblogs,
		Notify:      req.Notify != nil && *req.Notify,
	})
	if err != nil {
		return common.RespondInternalServerError(ctx, err.Error())
	}

	return okJSON(result.Relationship)
}

// HandleUnfollowAccountFull unfollows an account using Relationships service
func (h *Handler) HandleUnfollowAccountFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	result, err := h.registry.Relationships().Unfollow(ctx.Context(), &relationships.UnfollowCommand{
		FollowerID:  claims.Username,
		FollowingID: accountID,
	})
	if err != nil {
		return common.RespondInternalServerError(ctx, err.Error())
	}

	return okJSON(result.Relationship)
}

// HandleBlockAccountFull blocks an account using Relationships service
func (h *Handler) HandleBlockAccountFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	result, err := h.registry.Relationships().Block(ctx.Context(), &relationships.BlockCommand{
		BlockerID: claims.Username,
		BlockedID: accountID,
	})
	if err != nil {
		return common.RespondInternalServerError(ctx, err.Error())
	}

	return okJSON(result.Relationship)
}

// HandleUnblockAccountFull unblocks an account using Relationships service
func (h *Handler) HandleUnblockAccountFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	result, err := h.registry.Relationships().Unblock(ctx.Context(), &relationships.UnblockCommand{
		BlockerID: claims.Username,
		BlockedID: accountID,
	})
	if err != nil {
		return common.RespondInternalServerError(ctx, err.Error())
	}

	return okJSON(result.Relationship)
}

// HandleMuteAccountFull mutes an account using Relationships service
func (h *Handler) HandleMuteAccountFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	var req models.MuteRequest
	_ = common.ParseRequestWithFallback(ctx, &req)

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	result, err := h.registry.Relationships().Mute(ctx.Context(), &relationships.MuteCommand{
		MuterID:           claims.Username,
		MutedID:           accountID,
		MuteNotifications: req.Notifications != nil && *req.Notifications,
	})
	if err != nil {
		return common.RespondInternalServerError(ctx, err.Error())
	}

	return okJSON(result.Relationship)
}

// HandleUnmuteAccountFull unmutes an account using Relationships service
func (h *Handler) HandleUnmuteAccountFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	result, err := h.registry.Relationships().Unmute(ctx.Context(), &relationships.UnmuteCommand{
		MuterID: claims.Username,
		MutedID: accountID,
	})
	if err != nil {
		return common.RespondInternalServerError(ctx, err.Error())
	}

	return okJSON(result.Relationship)
}

// HandleGetRelationshipsFull gets relationships with multiple accounts using Relationships service
func (h *Handler) HandleGetRelationshipsFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	accountIDsParam := queryValue(ctx, "id")
	if err := common.ValidateRequiredParam("account_ids", accountIDsParam); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	accountIDs := strings.Split(accountIDsParam, ",")
	if err := common.ValidateSliceNotEmpty("accountIDs", accountIDs); err != nil {
		return common.RespondBadRequest(ctx, "invalid account ids")
	}

	result, err := h.registry.Relationships().GetRelationships(ctx.Context(), &relationships.GetRelationshipsQuery{
		RequesterID: claims.Username,
		TargetIDs:   accountIDs,
	})
	if err != nil {
		return common.RespondInternalServerError(ctx, err.Error())
	}

	return okJSON(result.Relationships)
}
