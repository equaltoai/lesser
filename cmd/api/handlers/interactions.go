package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const (
	agentDefaultMaxFollowsPerHour         = 20
	agentVerifiedDefaultMaxFollowsPerHour = 100

	relationshipOpFollow   = "follow"
	relationshipOpUnfollow = "unfollow"

	statusOpFavorite   = "favorite"
	statusOpUnfavorite = "unfavorite"
	statusOpReblog     = "reblog"
	statusOpUnreblog   = "unreblog"
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
	case relationshipOpFollow, relationshipOpUnfollow:
		claims, err = h.authenticateWithAnyScope(ctx, relationshipOpFollow, "write:follows", auth.ScopeWrite)
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

	targetID, err := h.resolveRelationshipTargetID(ctx.Context(), accountID)
	if err != nil {
		if relationshipTargetNotFound(err) {
			return common.RespondAccountNotFound(ctx)
		}
		return common.RespondInternalServerError(ctx, err.Error())
	}

	var targetAccount *storage.Account
	targetPublicID := ""
	if resolvedAccount, lookupErr := h.lookupStorageAccountByID(ctx.Context(), accountID); lookupErr == nil && resolvedAccount != nil {
		targetAccount = resolvedAccount
		targetPublicID = h.publicAccountFromStorageAccount(targetAccount).ID
	}

	switch operation {
	case relationshipOpFollow:
		var req models.FollowRequest
		_ = common.ParseRequestWithFallback(ctx, &req)

		if claims.IsAgent {
			if resp, err := h.enforceAgentFollowRails(ctx, claims.Username); resp != nil || err != nil {
				return resp, err
			}
		}

		r, err := h.registry.Relationships().Follow(ctx.Context(), &relationships.FollowCommand{
			FollowerID:  claims.Username,
			FollowingID: targetID,
			ShowReblogs: req.Reblogs == nil || *req.Reblogs,
			Notify:      req.Notify != nil && *req.Notify,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		h.createFollowNotification(ctx.Context(), claims.Username, targetID, targetAccount)
		return okJSON(h.relationshipFromServiceWithPublicID(r.Relationship, targetPublicID))
	case relationshipOpUnfollow:
		r, err := h.registry.Relationships().Unfollow(ctx.Context(), &relationships.UnfollowCommand{
			FollowerID:  claims.Username,
			FollowingID: targetID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromServiceWithPublicID(r.Relationship, targetPublicID))
	case "block":
		r, err := h.registry.Relationships().Block(ctx.Context(), &relationships.BlockCommand{
			BlockerID: claims.Username,
			BlockedID: targetID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromServiceWithPublicID(r.Relationship, targetPublicID))
	case "unblock":
		r, err := h.registry.Relationships().Unblock(ctx.Context(), &relationships.UnblockCommand{
			BlockerID: claims.Username,
			BlockedID: targetID,
		})
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromServiceWithPublicID(r.Relationship, targetPublicID))
	default:
		return common.RespondBadRequest(ctx, "invalid operation")
	}
}

func (h *Handler) createFollowNotification(ctx context.Context, followerUsername, targetID string, targetAccount *storage.Account) {
	if h == nil || h.registry == nil {
		return
	}

	notificationService := h.registry.Notifications()
	if notificationService == nil || targetAccount == nil || targetAccount.User == nil {
		return
	}

	if targetAccount.Actor != nil && !h.actorAppearsLocal(targetAccount.Actor) {
		return
	}

	recipient := strings.TrimSpace(targetAccount.User.Username)
	if recipient == "" || recipient == followerUsername {
		return
	}

	cmd := notifications.NewFollowNotificationCommand(recipient, followerUsername, targetID, "user")

	if _, err := notificationService.CreateNotification(ctx, cmd); err != nil {
		h.logger.Warn("failed to create follow notification",
			zap.String("recipient", recipient),
			zap.String("follower", followerUsername),
			zap.Error(err))
	}
}

func (h *Handler) resolveRelationshipTargetID(ctx context.Context, accountID string) (string, error) {
	actor, err := h.resolveAccountID(ctx, accountID)
	if err != nil {
		return "", err
	}
	if actor == nil {
		return "", fmt.Errorf("actor not found: %s", accountID)
	}
	if actorID := strings.TrimSpace(actor.ID); actorID != "" {
		return actorID, nil
	}
	if username := strings.TrimSpace(actor.PreferredUsername); username != "" {
		return username, nil
	}
	return "", fmt.Errorf("actor not found: %s", accountID)
}

func relationshipTargetNotFound(err error) bool {
	if err == nil {
		return false
	}
	if common.IsNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
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

	if err := h.repos.RateLimit().CheckAPIRateLimit(ctx.Context(), agentRateLimitUserID(username), "agent_follows_per_hour", allowed, time.Hour); err != nil {
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
			accounts = append(accounts, h.publicAccountFromStorageAccount(blockedAccount))
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

	if !isSupportedStatusInteraction(operation) {
		return common.RespondBadRequest(ctx, "invalid operation")
	}

	result, err := h.executeStatusInteraction(ctx.Context(), operation, statusID, claims.Username)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "status not found")
		}
		return common.RespondInternalServerError(ctx, statusInteractionFailureMessage(operation))
	}

	mastodonStatus, convErr := h.convertStorageStatusToAPI(result.Status, claims.Username)
	if convErr != nil {
		h.logger.Error("failed to serialize interacted status", zap.String("operation", operation), zap.Error(convErr))
		return common.RespondInternalServerError(ctx, "failed to serialize status")
	}

	applyStatusInteractionState(mastodonStatus, operation)
	return okJSON(mastodonStatus)
}

func isSupportedStatusInteraction(operation string) bool {
	switch operation {
	case statusOpFavorite, statusOpUnfavorite, statusOpReblog, statusOpUnreblog:
		return true
	default:
		return false
	}
}

func (h *Handler) executeStatusInteraction(ctx context.Context, operation string, statusID string, username string) (*notes.LikeResult, error) {
	switch operation {
	case statusOpFavorite:
		return h.registry.Notes().LikeNote(ctx, &notes.LikeNoteCommand{
			StatusID: statusID,
			LikerID:  username,
		})
	case statusOpUnfavorite:
		return h.registry.Notes().UnlikeNote(ctx, &notes.UnlikeNoteCommand{
			StatusID:  statusID,
			UnlikerID: username,
		})
	case statusOpReblog:
		return h.registry.Notes().ReblogNote(ctx, &notes.ReblogNoteCommand{
			StatusID:    statusID,
			RebloggerID: username,
		})
	case statusOpUnreblog:
		return h.registry.Notes().UnreblogNote(ctx, &notes.UnreblogNoteCommand{
			StatusID:      statusID,
			UnrebloggerID: username,
		})
	default:
		return nil, fmt.Errorf("unsupported status interaction: %s", operation)
	}
}

func statusInteractionFailureMessage(operation string) string {
	switch operation {
	case statusOpFavorite:
		return "failed to like status"
	case statusOpUnfavorite:
		return "failed to unlike status"
	case statusOpReblog:
		return "failed to reblog status"
	case statusOpUnreblog:
		return "failed to unreblog status"
	default:
		return "failed to update status"
	}
}

func applyStatusInteractionState(status *models.Status, operation string) {
	if status == nil {
		return
	}

	switch operation {
	case statusOpFavorite:
		status.Favourited = true
	case statusOpUnfavorite:
		status.Favourited = false
	case statusOpReblog:
		status.Reblogged = true
	case statusOpUnreblog:
		status.Reblogged = false
	}
}

// HandleFavoriteLift handles POST /api/v1/statuses/:id/favourite
func (h *Handler) HandleFavoriteLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusInteraction(ctx, statusOpFavorite)
}

// HandleUnfavoriteLift handles POST /api/v1/statuses/:id/unfavourite
func (h *Handler) HandleUnfavoriteLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusInteraction(ctx, statusOpUnfavorite)
}

// HandleReblogLift handles POST /api/v1/statuses/:id/reblog
func (h *Handler) HandleReblogLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusInteraction(ctx, statusOpReblog)
}

// HandleUnreblogLift handles POST /api/v1/statuses/:id/unreblog
func (h *Handler) HandleUnreblogLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusInteraction(ctx, statusOpUnreblog)
}
