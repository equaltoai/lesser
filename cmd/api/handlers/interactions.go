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
	notificationservice "github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
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

	claims, err := h.authenticateRelationshipOperation(ctx, operation)
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

	targetID, targetPublicID, targetUsername, err := h.resolveRelationshipTarget(ctx.Context(), accountID)
	if err != nil {
		if relationshipTargetNotFound(err) {
			return common.RespondAccountNotFound(ctx)
		}
		return common.RespondInternalServerError(ctx, err.Error())
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
		h.createFollowNotification(ctx.Context(), claims.Username, targetUsername, r)
		return okJSON(h.relationshipFromServiceWithPublicID(r.Relationship, targetPublicID))
	case relationshipOpUnfollow:
		relationship, err := h.handleUnfollow(ctx.Context(), claims.Username, targetID)
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromServiceWithPublicID(relationship, targetPublicID))
	case "block":
		relationship, err := h.handleBlock(ctx.Context(), claims.Username, targetID)
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromServiceWithPublicID(relationship, targetPublicID))
	case "unblock":
		relationship, err := h.handleUnblock(ctx.Context(), claims.Username, targetID)
		if err != nil {
			return common.RespondInternalServerError(ctx, err.Error())
		}
		return okJSON(h.relationshipFromServiceWithPublicID(relationship, targetPublicID))
	default:
		return common.RespondBadRequest(ctx, "invalid operation")
	}
}

func (h *Handler) authenticateRelationshipOperation(ctx *apptheory.Context, operation string) (*auth.Claims, error) {
	switch operation {
	case relationshipOpFollow, relationshipOpUnfollow:
		return h.authenticateWithAnyScope(ctx, relationshipOpFollow, "write:follows", auth.ScopeWrite)
	case "block", "unblock":
		return h.authenticateWithAnyScope(ctx, "write:blocks", auth.ScopeWrite)
	default:
		return h.authenticateWithScope(ctx, auth.ScopeWrite)
	}
}

func (h *Handler) resolveRelationshipTarget(ctx context.Context, accountID string) (targetID, targetPublicID, targetUsername string, err error) {
	targetID, err = h.resolveRelationshipTargetID(ctx, accountID)
	if err != nil {
		return "", "", "", err
	}

	targetAccount, lookupErr := h.lookupStorageAccountByID(ctx, accountID)
	if lookupErr != nil || targetAccount == nil {
		return targetID, "", "", nil
	}

	targetPublicID = h.publicAccountFromStorageAccount(targetAccount).ID
	if targetAccount.User != nil {
		targetUsername = strings.TrimSpace(targetAccount.User.Username)
	}

	return targetID, targetPublicID, targetUsername, nil
}

func (h *Handler) handleUnfollow(ctx context.Context, username, targetID string) (*relationships.RelationshipData, error) {
	r, err := h.registry.Relationships().Unfollow(ctx, &relationships.UnfollowCommand{
		FollowerID:  username,
		FollowingID: targetID,
	})
	if err != nil {
		return nil, err
	}
	return r.Relationship, nil
}

func (h *Handler) handleBlock(ctx context.Context, username, targetID string) (*relationships.RelationshipData, error) {
	r, err := h.registry.Relationships().Block(ctx, &relationships.BlockCommand{
		BlockerID: username,
		BlockedID: targetID,
	})
	if err != nil {
		return nil, err
	}
	return r.Relationship, nil
}

func (h *Handler) handleUnblock(ctx context.Context, username, targetID string) (*relationships.RelationshipData, error) {
	r, err := h.registry.Relationships().Unblock(ctx, &relationships.UnblockCommand{
		BlockerID: username,
		BlockedID: targetID,
	})
	if err != nil {
		return nil, err
	}
	return r.Relationship, nil
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
		// When the target was specified as a URL, validate that the resolved
		// actor's declared ID belongs to the same domain. This prevents a
		// follow/block request to https://victim.example/users/bob from
		// resolving to a spoofed actor on a different domain.
		if strings.HasPrefix(accountID, "http://") || strings.HasPrefix(accountID, "https://") {
			if err := common.ValidateActorDomainConsistency(accountID, actorID); err != nil {
				h.logger.Warn("relationship target domain mismatch",
					zap.String("requested_id", accountID),
					zap.String("resolved_actor_id", actorID),
					zap.Error(err))
				return "", fmt.Errorf("actor identity mismatch")
			}
		}
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
	governance, err := requireAgentGovernanceState(ctx.Context(), h.repos, username)
	if err != nil {
		return respondAgentGovernanceUnavailable(ctx)
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
	if agentVerifiedState(governance) {
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
	h.createStatusInteractionNotification(ctx.Context(), operation, claims.Username, result.Status)

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

func (h *Handler) createFollowNotification(ctx context.Context, followerUsername, targetUsername string, result *relationships.FollowResult) {
	if h == nil || h.registry == nil || result == nil {
		return
	}

	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return
	}

	if !result.IsFollowing && (result.Relationship == nil || !result.Relationship.Following) {
		return
	}

	recipient := strings.TrimSpace(targetUsername)
	actor := strings.TrimSpace(followerUsername)
	if recipient == "" || actor == "" || strings.EqualFold(recipient, actor) {
		return
	}

	title := fmt.Sprintf("%s started following you", actor)
	cmd := &notificationservice.CreateNotificationCommand{
		UserID:     recipient,
		Type:       common.NotificationTypeFollow,
		ActorID:    actor,
		ActorType:  "user",
		TargetID:   recipient,
		TargetType: "account",
		Title:      title,
		Body:       title,
		GroupKey:   fmt.Sprintf("follow:%s", actor),
		Data: map[string]interface{}{
			"follower": actor,
		},
	}

	if _, err := notificationService.CreateNotification(ctx, cmd); err != nil {
		h.logger.Warn("failed to create follow notification",
			zap.String("recipient", recipient),
			zap.String("follower", actor),
			zap.Error(err))
	}
}

func (h *Handler) createStatusInteractionNotification(ctx context.Context, operation, actorUsername string, status *storagemodels.Status) {
	if operation != statusOpFavorite || h == nil || h.registry == nil || status == nil {
		return
	}

	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return
	}

	recipient := strings.TrimSpace(status.AuthorUsername)
	if recipient == "" {
		recipient = extractUsernameFromActorID(status.AuthorID)
	}
	actor := strings.TrimSpace(actorUsername)
	if recipient == "" || actor == "" || strings.EqualFold(recipient, actor) {
		return
	}

	title := fmt.Sprintf("%s favourited your post", actor)
	cmd := &notificationservice.CreateNotificationCommand{
		UserID:     recipient,
		Type:       common.NotificationTypeFavourite,
		ActorID:    actor,
		ActorType:  "user",
		TargetID:   strings.TrimSpace(status.StatusID),
		TargetType: "status",
		Title:      title,
		Body:       title,
		GroupKey:   fmt.Sprintf("favourite:%s", strings.TrimSpace(status.StatusID)),
		Data: map[string]interface{}{
			"status_id": strings.TrimSpace(status.StatusID),
			"liker":     actor,
		},
	}

	if _, err := notificationService.CreateNotification(ctx, cmd); err != nil {
		h.logger.Warn("failed to create favourite notification",
			zap.String("recipient", recipient),
			zap.String("actor", actor),
			zap.String("status_id", strings.TrimSpace(status.StatusID)),
			zap.Error(err))
	}
}

func extractUsernameFromActorID(actorID string) string {
	trimmed := strings.TrimSpace(strings.TrimSuffix(actorID, "/"))
	if trimmed == "" {
		return ""
	}

	if parts := strings.Split(trimmed, "/users/"); len(parts) == 2 {
		return strings.Split(parts[1], "/")[0]
	}
	if parts := strings.Split(trimmed, "/@"); len(parts) == 2 {
		return strings.Split(parts[1], "/")[0]
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
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
