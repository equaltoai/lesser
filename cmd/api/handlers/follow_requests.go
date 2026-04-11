package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// followRequestConfig holds configuration for follow request actions
type followRequestConfig struct {
	actionType       string
	serviceMethod    interface{} // Will be either AcceptFollowRequest or RejectFollowRequest
	activitySender   func(ctx context.Context, accountID, username string) error
	logMessage       string
	errorLogMessage  string
	activityLogError string
}

// handleFollowRequestOperation consolidates common follow request operation logic
func (h *Handler) handleFollowRequestOperation(ctx *apptheory.Context, actor *activitypub.Actor, username, accountID string, config followRequestConfig) (*apptheory.Response, error) {
	// Only locked accounts can have follow requests
	if !actor.ManuallyApprovesFollowers {
		return common.RespondBadRequest(ctx, "account is not locked")
	}

	// Call the appropriate service method
	var result *relationships.RelationshipResult
	var err error

	switch config.actionType {
	case "accept":
		if acceptMethod, ok := config.serviceMethod.(func(context.Context, *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error)); ok {
			result, err = acceptMethod(ctx.Context(), &relationships.AcceptFollowRequestCommand{
				RequesterID: username,
				FollowerID:  accountID,
			})
		}
	case "reject":
		if rejectMethod, ok := config.serviceMethod.(func(context.Context, *relationships.RejectFollowRequestCommand) (*relationships.RelationshipResult, error)); ok {
			result, err = rejectMethod(ctx.Context(), &relationships.RejectFollowRequestCommand{
				RequesterID: username,
				FollowerID:  accountID,
			})
		}
	}

	if err != nil {
		h.logger.Error(config.errorLogMessage, zap.Error(err))
		if err.Error() == common.ErrorFollowRequestNotFound {
			return common.RespondNotFound(ctx, "follow request")
		}
		return common.RespondInternalServerError(ctx, "internal server error")
	}

	// Send activity to the follower
	go func() {
		if err := config.activitySender(ctx.Context(), accountID, username); err != nil {
			h.logger.Error(config.activityLogError, zap.Error(err))
		}
	}()

	h.logger.Info(config.logMessage,
		zap.String("username", username),
		zap.String("follower_id", accountID))

	// Convert relationship data to Mastodon API format using consolidated formatter
	return h.respondWithRelationship(ctx, accountID, result.Relationship)
}

// HandleGetFollowRequestsLift handles GET /api/v1/follow_requests
// Returns pending follow requests for locked accounts
func (h *Handler) HandleGetFollowRequestsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		return resp, err
	}

	if !claimsHaveAnyScope(claims, "read:follows", auth.ScopeRead) {
		return common.RespondInsufficientScope(ctx)
	}

	// Get the user's actor
	result, err := h.registry.Accounts().GetAccount(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.RespondInternalServerError(ctx, "internal server error")
	}
	actor := result.Actor

	return h.handleGetFollowRequestsLogic(ctx, actor, claims.Username)
}

func (h *Handler) handleFollowRequestClaims(ctx *apptheory.Context, requiredScopes ...string) (*auth.Claims, *apptheory.Response, error) {
	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		return nil, resp, err
	}
	if !claimsHaveAnyScope(claims, requiredScopes...) {
		resp, err := common.RespondInsufficientScope(ctx)
		return nil, resp, err
	}
	return claims, nil, nil
}

// handleGetFollowRequestsLogic contains the main follow requests logic, separated for testing
func (h *Handler) handleGetFollowRequestsLogic(ctx *apptheory.Context, actor *activitypub.Actor, username string) (*apptheory.Response, error) {
	// If account is not locked, return empty array
	if !actor.ManuallyApprovesFollowers {
		return okJSON([]apimodels.Account{})
	}

	// Use the Relationships service to get pending follow requests
	result, err := h.registry.Relationships().GetPendingFollowRequests(ctx.Context(), &relationships.GetFollowRequestsQuery{
		UserID: username,
		Limit:  100,
	})
	if err != nil {
		h.logger.Error("failed to get pending follow requests", zap.Error(err))
		return common.RespondInternalServerError(ctx, "internal server error")
	}

	accounts := make([]apimodels.Account, 0, len(result.FollowerIDs))
	for _, followerID := range result.FollowerIDs {
		// Get follower actor
		followerResult, err := h.registry.Accounts().GetAccount(ctx.Context(), followerID)
		if err != nil {
			h.logger.Warn("failed to get follower actor",
				zap.String("follower_id", followerID),
				zap.Error(err))
			continue
		}
		followerActor := followerResult.Actor

		accounts = append(accounts, h.convertActorToAccountWithCounts(ctx.Context(), followerActor))
	}

	h.logger.Info("follow requests retrieved",
		zap.String("username", username),
		zap.Int("count", len(accounts)))

	return okJSON(accounts)
}

// HandleAuthorizeFollowRequestLift handles POST /api/v1/follow_requests/:account_id/authorize
// Accepts a pending follow request
func (h *Handler) HandleAuthorizeFollowRequestLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleFollowRequestAction(ctx, "authorize", h.handleAuthorizeFollowRequestLogic)
}

// handleAuthorizeFollowRequestLogic contains the main authorize logic, separated for testing
func (h *Handler) handleAuthorizeFollowRequestLogic(ctx *apptheory.Context, actor *activitypub.Actor, username, accountID string) (*apptheory.Response, error) {
	return h.handleFollowRequestOperation(ctx, actor, username, accountID, followRequestConfig{
		actionType:       "accept",
		serviceMethod:    h.registry.Relationships().AcceptFollowRequest,
		activitySender:   h.sendAcceptActivityLift,
		logMessage:       "follow request authorized",
		errorLogMessage:  "failed to accept follow request",
		activityLogError: "failed to send accept activity",
	})
}

// HandleRejectFollowRequestLift handles POST /api/v1/follow_requests/:account_id/reject
// Rejects a pending follow request
func (h *Handler) HandleRejectFollowRequestLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleFollowRequestAction(ctx, "reject", h.handleRejectFollowRequestLogic)
}

// handleFollowRequestAction consolidates the common authentication and validation logic
// for both authorize and reject follow request actions
func (h *Handler) handleFollowRequestAction(ctx *apptheory.Context, _ string, logicHandler func(*apptheory.Context, *activitypub.Actor, string, string) (*apptheory.Response, error)) (*apptheory.Response, error) {
	accountID := ctx.Param("account_id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, resp, err := h.handleFollowRequestClaims(ctx, auth.WriteFollows, auth.ScopeWrite)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get the user's actor
	result, err := h.registry.Accounts().GetAccount(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.RespondInternalServerError(ctx, "internal server error")
	}
	actor := result.Actor

	return logicHandler(ctx, actor, claims.Username, accountID)
}

// handleRejectFollowRequestLogic contains the main reject logic, separated for testing
func (h *Handler) handleRejectFollowRequestLogic(ctx *apptheory.Context, actor *activitypub.Actor, username, accountID string) (*apptheory.Response, error) {
	return h.handleFollowRequestOperation(ctx, actor, username, accountID, followRequestConfig{
		actionType:       "reject",
		serviceMethod:    h.registry.Relationships().RejectFollowRequest,
		activitySender:   h.sendRejectActivityLift,
		logMessage:       "follow request rejected",
		errorLogMessage:  "failed to reject follow request",
		activityLogError: "failed to send reject activity",
	})
}

// sendAcceptActivityLift sends an Accept activity to the follower
func (h *Handler) sendAcceptActivityLift(ctx context.Context, followerID, followedID string) error {
	return h.sendFollowResponseActivity(ctx, followerID, followedID, "Accept")
}

// sendRejectActivityLift sends a Reject activity to the follower
func (h *Handler) sendRejectActivityLift(ctx context.Context, followerID, followedID string) error {
	return h.sendFollowResponseActivity(ctx, followerID, followedID, "Reject")
}

// sendFollowResponseActivity consolidates the common logic for sending Accept/Reject activities
func (h *Handler) sendFollowResponseActivity(ctx context.Context, followerID, followedID, activityType string) error {
	// Get follower actor to determine inbox
	followerResult, err := h.registry.Accounts().GetAccount(ctx, followerID)
	if err != nil {
		return errors.Join(failedToGetFollowerActor(), err)
	}
	followerActor := followerResult.Actor

	// Get followed actor
	followedResult, err := h.registry.Accounts().GetAccount(ctx, followedID)
	if err != nil {
		return errors.Join(failedToGetFollowedActor(), err)
	}
	followedActor := followedResult.Actor

	// Create response activity (Accept or Reject)
	responseActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activityType,
			ID:      fmt.Sprintf("https://%s/activities/%d", h.cfg.Domain, time.Now().UnixNano()),
		},
		Actor: followedActor.ID,
		Object: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: "Follow",
				ID:   fmt.Sprintf("https://%s/follows/%s", followerActor.URL, followedID),
			},
			Actor:  followerActor.ID,
			Object: followedActor.ID,
		},
	}

	// Send to follower's inbox
	if err := common.ValidateRequiredParam("inbox", followerActor.Inbox); err == nil {
		h.deliverActivityLift(ctx, responseActivity, followerActor.Inbox)
	}

	h.logger.Info(fmt.Sprintf("%s activity sent", strings.ToLower(activityType)),
		zap.String("follower_id", followerID),
		zap.String("followed_id", followedID),
		zap.String("inbox", followerActor.Inbox))

	return nil
}

// deliverActivityLift delivers an ActivityPub activity to a remote inbox
func (h *Handler) deliverActivityLift(_ context.Context, activity *activitypub.Activity, inboxURL string) {
	// This would implement HTTP signature authentication and delivery
	// For now, just log the delivery attempt
	h.logger.Info("delivering activity",
		zap.String("type", activity.Type),
		zap.String("id", activity.ID),
		zap.String("inbox", inboxURL))

	// In production, this would involve:
	// 1. Serializing the activity to JSON
	// 2. Signing the request with the server's private key
	// 3. Making an HTTP POST to the inbox URL
	// 4. Handling delivery failures and retries
}

// respondWithRelationship consolidates the duplicate relationship response formatting
// This eliminates the identical relationship mapping found in authorize and reject handlers
func (h *Handler) respondWithRelationship(_ *apptheory.Context, accountID string, relationship *relationships.RelationshipData) (*apptheory.Response, error) {
	// Convert relationship data to Mastodon API format
	relationshipResponse := map[string]any{
		"id":                   accountID,
		"following":            relationship.FollowedBy,
		"showing_reblogs":      relationship.ShowingReblogs,
		"notifying":            relationship.Notifying,
		"followed_by":          relationship.Following,
		"blocking":             relationship.BlockedBy,
		"blocked_by":           relationship.Blocking,
		"muting":               relationship.Muting,
		"muting_notifications": relationship.MutingNotifications,
		"requested":            relationship.RequestedBy,
		"domain_blocking":      relationship.DomainBlocking,
		"endorsed":             relationship.Endorsed,
		"note":                 relationship.Note,
	}

	return okJSON(relationshipResponse)
}
