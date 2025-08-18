package lift

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// relationshipOperation performs common relationship operations (follow/unfollow/block/unblock)
func (h *Handler) relationshipOperation(ctx *lift.Context, operation string) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Check if relationships service is available
	if h.registry == nil || h.registry.Relationships() == nil {
		return common.RespondServiceUnavailable(ctx, "service unavailable")
	}

	switch operation {
	case "follow":
		var req models.FollowRequest
		_ = ctx.ParseRequest(&req)
		r, err := h.registry.Relationships().Follow(ctx.Context, &relationships.FollowCommand{
			FollowerID:  claims.Username,
			FollowingID: accountID,
			ShowReblogs: req.Reblogs == nil || *req.Reblogs,
			Notify:      req.Notify != nil && *req.Notify,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return ctx.JSON(r.Relationship)
	case "unfollow":
		r, err := h.registry.Relationships().Unfollow(ctx.Context, &relationships.UnfollowCommand{
			FollowerID:  claims.Username,
			FollowingID: accountID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return ctx.JSON(r.Relationship)
	case "block":
		r, err := h.registry.Relationships().Block(ctx.Context, &relationships.BlockCommand{
			BlockerID: claims.Username,
			BlockedID: accountID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return ctx.JSON(r.Relationship)
	case "unblock":
		r, err := h.registry.Relationships().Unblock(ctx.Context, &relationships.UnblockCommand{
			BlockerID: claims.Username,
			BlockedID: accountID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return ctx.JSON(r.Relationship)
	default:
		return common.RespondBadRequest(ctx, "invalid operation")
	}
}

// HandleFollowLift handles POST /api/v1/accounts/:id/follow
func (h *Handler) HandleFollowLift(ctx *lift.Context) error {
	return h.relationshipOperation(ctx, "follow")
}

// HandleUnfollowLift handles POST /api/v1/accounts/:id/unfollow
func (h *Handler) HandleUnfollowLift(ctx *lift.Context) error {
	return h.relationshipOperation(ctx, "unfollow")
}

// HandleBlockLift handles POST /api/v1/accounts/:id/block
func (h *Handler) HandleBlockLift(ctx *lift.Context) error {
	return h.relationshipOperation(ctx, "block")
}

// HandleUnblockLift handles POST /api/v1/accounts/:id/unblock
func (h *Handler) HandleUnblockLift(ctx *lift.Context) error {
	return h.relationshipOperation(ctx, "unblock")
}

// HandleGetBlocksLift handles GET /api/v1/blocks
func (h *Handler) HandleGetBlocksLift(ctx *lift.Context) error {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Parse query parameters
	maxID := ctx.Query("max_id")

	// Use Relationships service to get blocked users
	result, err := h.registry.Relationships().GetBlockedUsers(ctx.Context, &relationships.GetBlockedUsersQuery{
		UserID: claims.Username,
		Limit:  40,
		Cursor: maxID,
	})
	if err != nil {
		h.logger.Error("failed to get blocked users", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert storage accounts to API accounts
	accounts := []models.Account{}
	for _, blockedAccount := range result.BlockedUsers {
		if blockedAccount.Actor != nil {
			account := transformations.ActorToAccountBase(blockedAccount.Actor, h.cfg.BaseURL())
			accounts = append(accounts, account)
		}
	}

	// Set Link header for pagination if there's a cursor
	if result.NextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/blocks?max_id=%s>; rel="next"`,
			h.cfg.BaseURL(), result.NextCursor)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(accounts)
}

// statusInteraction performs common status interactions (favorite/unfavorite/reblog/unreblog)
func (h *Handler) statusInteraction(ctx *lift.Context, operation string) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	switch operation {
	case "favorite":
		r, err := h.registry.Notes().LikeNote(ctx.Context, &notes.LikeNoteCommand{
			StatusID: statusID,
			LikerID:  claims.Username,
		})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return common.RespondNotFound(ctx, "status not found")
			}
			return common.RespondInternalServerError(ctx, "failed to like status")
		}
		mastodonStatus := transformations.NotesToStatusAny(r.Status, h.cfg.BaseURL())
		mastodonStatus.Favourited = true
		return ctx.JSON(mastodonStatus)
	case "unfavorite":
		r, err := h.registry.Notes().UnlikeNote(ctx.Context, &notes.UnlikeNoteCommand{
			StatusID:  statusID,
			UnlikerID: claims.Username,
		})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return common.RespondNotFound(ctx, "status not found")
			}
			return common.RespondInternalServerError(ctx, "failed to unlike status")
		}
		mastodonStatus := transformations.NotesToStatusAny(r.Status, h.cfg.BaseURL())
		mastodonStatus.Favourited = false
		return ctx.JSON(mastodonStatus)
	case "reblog":
		r, err := h.registry.Notes().ReblogNote(ctx.Context, &notes.ReblogNoteCommand{
			StatusID:    statusID,
			RebloggerID: claims.Username,
		})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return common.RespondNotFound(ctx, "status not found")
			}
			return common.RespondInternalServerError(ctx, "failed to reblog status")
		}
		mastodonStatus := transformations.NotesToStatusAny(r.Status, h.cfg.BaseURL())
		mastodonStatus.Reblogged = true
		return ctx.JSON(mastodonStatus)
	case "unreblog":
		r, err := h.registry.Notes().UnreblogNote(ctx.Context, &notes.UnreblogNoteCommand{
			StatusID:      statusID,
			UnrebloggerID: claims.Username,
		})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return common.RespondNotFound(ctx, "status not found")
			}
			return common.RespondInternalServerError(ctx, "failed to unreblog status")
		}
		mastodonStatus := transformations.NotesToStatusAny(r.Status, h.cfg.BaseURL())
		mastodonStatus.Reblogged = false
		return ctx.JSON(mastodonStatus)
	default:
		return common.RespondBadRequest(ctx, "invalid operation")
	}
}

// HandleFavoriteLift handles POST /api/v1/statuses/:id/favourite
func (h *Handler) HandleFavoriteLift(ctx *lift.Context) error {
	return h.statusInteraction(ctx, "favorite")
}

// HandleUnfavoriteLift handles POST /api/v1/statuses/:id/unfavourite
func (h *Handler) HandleUnfavoriteLift(ctx *lift.Context) error {
	return h.statusInteraction(ctx, "unfavorite")
}

// HandleReblogLift handles POST /api/v1/statuses/:id/reblog
func (h *Handler) HandleReblogLift(ctx *lift.Context) error {
	return h.statusInteraction(ctx, "reblog")
}

// HandleUnreblogLift handles POST /api/v1/statuses/:id/unreblog
func (h *Handler) HandleUnreblogLift(ctx *lift.Context) error {
	return h.statusInteraction(ctx, "unreblog")
}
