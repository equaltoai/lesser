package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const (
	agentDefaultMaxFollowsPerHour         = 20
	agentVerifiedDefaultMaxFollowsPerHour = 100
)

// relationshipOperation performs common relationship operations (follow/unfollow/block/unblock)
func (h *Handler) relationshipOperation(ctx *apptheory.Context, operation string) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	var (
		claims *auth.Claims
		err    error
	)
	switch operation {
	case "follow", "unfollow":
		claims, err = h.authenticateWithAnyScope(ctx, "follow", "write:follows", auth.ScopeWrite)
	case "block", "unblock":
		claims, err = h.authenticateWithAnyScope(ctx, "write:blocks", auth.ScopeWrite)
	default:
		claims, err = h.authenticateWithScope(ctx, auth.ScopeWrite)
	}
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Check if relationships service is available
	if h.registry == nil || h.registry.Relationships() == nil {
		return common.RespondServiceUnavailable(ctx, "service unavailable")
	}

	switch operation {
	case "follow":
		var req models.FollowRequest
		_ = common.ParseRequestWithFallback(ctx, &req)

		if claims.IsAgent {
			if resp, err := h.enforceAgentFollowRails(ctx, claims.Username); resp != nil || err != nil {
				return resp, err
			}
		}

		r, err := h.registry.Relationships().Follow(ctx.Context(), &relationships.FollowCommand{
			FollowerID:  claims.Username,
			FollowingID: accountID,
			ShowReblogs: req.Reblogs == nil || *req.Reblogs,
			Notify:      req.Notify != nil && *req.Notify,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromService(r.Relationship))
	case "unfollow":
		r, err := h.registry.Relationships().Unfollow(ctx.Context(), &relationships.UnfollowCommand{
			FollowerID:  claims.Username,
			FollowingID: accountID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromService(r.Relationship))
	case "block":
		r, err := h.registry.Relationships().Block(ctx.Context(), &relationships.BlockCommand{
			BlockerID: claims.Username,
			BlockedID: accountID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromService(r.Relationship))
	case "unblock":
		r, err := h.registry.Relationships().Unblock(ctx.Context(), &relationships.UnblockCommand{
			BlockerID: claims.Username,
			BlockedID: accountID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromService(r.Relationship))
	default:
		return common.RespondBadRequest(ctx, "invalid operation")
	}
}

func (h *Handler) enforceAgentFollowRails(ctx *apptheory.Context, username string) (*apptheory.Response, error) {
	if h == nil || ctx == nil {
		return nil, nil
	}
	if h.repos == nil || h.repos.RateLimit() == nil || h.repos.User() == nil {
		return nil, nil
	}

	agentUser, _ := h.repos.User().GetUser(ctx.Context(), username)
	if agentUser == nil || !agentUser.IsAgent {
		return nil, nil
	}

	limit := agentDefaultMaxFollowsPerHour
	verifiedLimit := agentVerifiedDefaultMaxFollowsPerHour
	if h.repos.Instance() != nil {
		if policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context()); err == nil && policy != nil {
			if policy.AgentMaxFollowsPerHour > 0 {
				limit = policy.AgentMaxFollowsPerHour
			}
			if policy.VerifiedAgentMaxFollowsPerHour > 0 {
				verifiedLimit = policy.VerifiedAgentMaxFollowsPerHour
			}
		}
	}

	allowed := limit
	if agentMetadataBool(agentUser, "agent_verified") {
		allowed = verifiedLimit
	}

	if allowed <= 0 {
		return nil, nil
	}

	if err := h.repos.RateLimit().CheckAPIRateLimit(ctx.Context(), "agent:"+username, "agent_follows_per_hour", allowed, time.Hour); err != nil {
		return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
			"error":             "too_many_requests",
			"error_description": "agent follow limit exceeded",
		})
	}

	return nil, nil
}

// HandleFollowLift handles POST /api/v1/accounts/:id/follow
func (h *Handler) HandleFollowLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.relationshipOperation(ctx, "follow")
}

// HandleUnfollowLift handles POST /api/v1/accounts/:id/unfollow
func (h *Handler) HandleUnfollowLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.relationshipOperation(ctx, "unfollow")
}

// HandleBlockLift handles POST /api/v1/accounts/:id/block
func (h *Handler) HandleBlockLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.relationshipOperation(ctx, "block")
}

// HandleUnblockLift handles POST /api/v1/accounts/:id/unblock
func (h *Handler) HandleUnblockLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.relationshipOperation(ctx, "unblock")
}

// HandleGetBlocksLift handles GET /api/v1/blocks
func (h *Handler) HandleGetBlocksLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse query parameters
	maxID := queryValue(ctx, "max_id")

	// Use Relationships service to get blocked users
	result, err := h.registry.Relationships().GetBlockedUsers(ctx.Context(), &relationships.GetBlockedUsersQuery{
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
	resp, err := okJSON(accounts)
	if err != nil {
		return nil, err
	}

	if result.NextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/blocks?max_id=%s>; rel="next"`,
			h.cfg.BaseURL(), result.NextCursor)
		setHeader(resp, "Link", linkHeader)
	}

	return resp, nil
}

// statusInteraction performs common status interactions (favorite/unfavorite/reblog/unreblog)
func (h *Handler) statusInteraction(ctx *apptheory.Context, operation string) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	switch operation {
	case "favorite":
		r, err := h.registry.Notes().LikeNote(ctx.Context(), &notes.LikeNoteCommand{
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
		return okJSON(mastodonStatus)
	case "unfavorite":
		r, err := h.registry.Notes().UnlikeNote(ctx.Context(), &notes.UnlikeNoteCommand{
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
		return okJSON(mastodonStatus)
	case "reblog":
		r, err := h.registry.Notes().ReblogNote(ctx.Context(), &notes.ReblogNoteCommand{
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
		return okJSON(mastodonStatus)
	case "unreblog":
		r, err := h.registry.Notes().UnreblogNote(ctx.Context(), &notes.UnreblogNoteCommand{
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
		return okJSON(mastodonStatus)
	default:
		return common.RespondBadRequest(ctx, "invalid operation")
	}
}

// HandleFavoriteLift handles POST /api/v1/statuses/:id/favourite
func (h *Handler) HandleFavoriteLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusInteraction(ctx, "favorite")
}

// HandleUnfavoriteLift handles POST /api/v1/statuses/:id/unfavourite
func (h *Handler) HandleUnfavoriteLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusInteraction(ctx, "unfavorite")
}

// HandleReblogLift handles POST /api/v1/statuses/:id/reblog
func (h *Handler) HandleReblogLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusInteraction(ctx, "reblog")
}

// HandleUnreblogLift handles POST /api/v1/statuses/:id/unreblog
func (h *Handler) HandleUnreblogLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusInteraction(ctx, "unreblog")
}
