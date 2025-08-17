package lift

import (
	"fmt"
	"net/http"
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

// HandleFollowLift handles POST /api/v1/accounts/:id/follow
func (h *Handler) HandleFollowLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": err.Error()})
	}

	var req models.FollowRequest
	_ = ctx.ParseRequest(&req)

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Check if relationships service is available
	if h.registry == nil || h.registry.Relationships() == nil {
		return ctx.Status(http.StatusServiceUnavailable).JSON(map[string]string{"error": "service unavailable"})
	}

	result, err := h.registry.Relationships().Follow(ctx.Context, &relationships.FollowCommand{
		FollowerID:  claims.Username,
		FollowingID: accountID,
		ShowReblogs: req.Reblogs == nil || *req.Reblogs,
		Notify:      req.Notify != nil && *req.Notify,
	})
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": err.Error()})
	}

	return ctx.JSON(result.Relationship)
}

// HandleUnfollowLift handles POST /api/v1/accounts/:id/unfollow
func (h *Handler) HandleUnfollowLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": err.Error()})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Check if relationships service is available
	if h.registry == nil || h.registry.Relationships() == nil {
		return ctx.Status(http.StatusServiceUnavailable).JSON(map[string]string{"error": "service unavailable"})
	}

	result, err := h.registry.Relationships().Unfollow(ctx.Context, &relationships.UnfollowCommand{
		FollowerID:  claims.Username,
		FollowingID: accountID,
	})
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": err.Error()})
	}

	return ctx.JSON(result.Relationship)
}

// HandleBlockLift handles POST /api/v1/accounts/:id/block
func (h *Handler) HandleBlockLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": err.Error()})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Check if relationships service is available
	if h.registry == nil || h.registry.Relationships() == nil {
		return ctx.Status(http.StatusServiceUnavailable).JSON(map[string]string{"error": "service unavailable"})
	}

	result, err := h.registry.Relationships().Block(ctx.Context, &relationships.BlockCommand{
		BlockerID: claims.Username,
		BlockedID: accountID,
	})
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": err.Error()})
	}

	return ctx.JSON(result.Relationship)
}

// HandleUnblockLift handles POST /api/v1/accounts/:id/unblock
func (h *Handler) HandleUnblockLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": err.Error()})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Check if relationships service is available
	if h.registry == nil || h.registry.Relationships() == nil {
		return ctx.Status(http.StatusServiceUnavailable).JSON(map[string]string{"error": "service unavailable"})
	}

	result, err := h.registry.Relationships().Unblock(ctx.Context, &relationships.UnblockCommand{
		BlockerID: claims.Username,
		BlockedID: accountID,
	})
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": err.Error()})
	}

	return ctx.JSON(result.Relationship)
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
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
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

// HandleFavoriteLift handles POST /api/v1/statuses/:id/favourite
func (h *Handler) HandleFavoriteLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": err.Error()})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Notes service to like the status
	result, err := h.registry.Notes().LikeNote(ctx.Context, &notes.LikeNoteCommand{
		StatusID: statusID,
		LikerID:  claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to like status"})
	}

	// Convert to Mastodon API format
	mastodonStatus := transformations.NotesToStatusAny(result.Status, h.cfg.BaseURL())
	mastodonStatus.Favourited = true

	return ctx.JSON(mastodonStatus)
}

// HandleUnfavoriteLift handles POST /api/v1/statuses/:id/unfavourite
func (h *Handler) HandleUnfavoriteLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": err.Error()})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Notes service to unlike the status
	result, err := h.registry.Notes().UnlikeNote(ctx.Context, &notes.UnlikeNoteCommand{
		StatusID:  statusID,
		UnlikerID: claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to unlike status"})
	}

	// Convert to Mastodon API format
	mastodonStatus := transformations.NotesToStatusAny(result.Status, h.cfg.BaseURL())
	mastodonStatus.Favourited = false

	return ctx.JSON(mastodonStatus)
}

// HandleReblogLift handles POST /api/v1/statuses/:id/reblog
func (h *Handler) HandleReblogLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": err.Error()})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Notes service to reblog the status
	result, err := h.registry.Notes().ReblogNote(ctx.Context, &notes.ReblogNoteCommand{
		StatusID:    statusID,
		RebloggerID: claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to reblog status"})
	}

	// Convert to Mastodon API format
	mastodonStatus := transformations.NotesToStatusAny(result.Status, h.cfg.BaseURL())
	mastodonStatus.Reblogged = true

	return ctx.JSON(mastodonStatus)
}

// HandleUnreblogLift handles POST /api/v1/statuses/:id/unreblog
func (h *Handler) HandleUnreblogLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": err.Error()})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Notes service to unreblog the status
	result, err := h.registry.Notes().UnreblogNote(ctx.Context, &notes.UnreblogNoteCommand{
		StatusID:      statusID,
		UnrebloggerID: claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to unreblog status"})
	}

	// Convert to Mastodon API format
	mastodonStatus := transformations.NotesToStatusAny(result.Status, h.cfg.BaseURL())
	mastodonStatus.Reblogged = false

	return ctx.JSON(mastodonStatus)
}
