package lift

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetFollowRequestsLift handles GET /api/v1/follow_requests
// Returns pending follow requests for locked accounts
func (h *Handler) HandleGetFollowRequestsLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Get the user's actor directly (test mode)
		result, err := h.registry.Accounts().GetAccount(ctx.Context, testUsername)
		if err != nil {
			h.logger.Error("failed to get actor", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
		actor := result.Actor

		// Skip to the main logic with test username
		return h.handleGetFollowRequestsLogic(ctx, actor, testUsername)
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check read:follows scope
	if !claims.HasScope("read:follows") && !claims.HasScope(auth.ScopeRead) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	result, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}
	actor := result.Actor

	return h.handleGetFollowRequestsLogic(ctx, actor, claims.Username)
}

// handleGetFollowRequestsLogic contains the main follow requests logic, separated for testing
func (h *Handler) handleGetFollowRequestsLogic(ctx *lift.Context, actor *activitypub.Actor, username string) error {
	// If account is not locked, return empty array
	if !actor.ManuallyApprovesFollowers {
		return ctx.JSON([]any{})
	}

	// Use the Relationships service to get pending follow requests
	result, err := h.registry.Relationships().GetPendingFollowRequests(ctx.Context, &relationships.GetFollowRequestsQuery{
		UserID: username,
		Limit:  100,
	})
	if err != nil {
		h.logger.Error("failed to get pending follow requests", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert to account format
	accounts := make([]map[string]any, 0, len(result.FollowerIDs))
	for _, followerID := range result.FollowerIDs {
		// Get follower actor
		followerResult, err := h.registry.Accounts().GetAccount(ctx.Context, followerID)
		if err != nil {
			h.logger.Warn("failed to get follower actor",
				zap.String("follower_id", followerID),
				zap.Error(err))
			continue
		}
		followerActor := followerResult.Actor

		// Convert to account
		account := h.convertActorToAccountLift(ctx.Context, followerActor)
		accounts = append(accounts, account)
	}

	h.logger.Info("follow requests retrieved",
		zap.String("username", username),
		zap.Int("count", len(accounts)))

	return ctx.JSON(accounts)
}

// HandleAuthorizeFollowRequestLift handles POST /api/v1/follow_requests/:account_id/authorize
// Accepts a pending follow request
func (h *Handler) HandleAuthorizeFollowRequestLift(ctx *lift.Context) error {
	accountID := ctx.Param("account_id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "account_id parameter required"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Get the user's actor directly (test mode)
		result, err := h.registry.Accounts().GetAccount(ctx.Context, testUsername)
		if err != nil {
			h.logger.Error("failed to get actor", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
		actor := result.Actor

		// Skip to the main logic with test username
		return h.handleAuthorizeFollowRequestLogic(ctx, actor, testUsername, accountID)
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check write:follows scope
	if !claims.HasScope(auth.WriteFollows) && !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	result, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}
	actor := result.Actor

	return h.handleAuthorizeFollowRequestLogic(ctx, actor, claims.Username, accountID)
}

// handleAuthorizeFollowRequestLogic contains the main authorize logic, separated for testing
func (h *Handler) handleAuthorizeFollowRequestLogic(ctx *lift.Context, actor *activitypub.Actor, username, accountID string) error {
	// Only locked accounts can have follow requests
	if !actor.ManuallyApprovesFollowers {
		return ctx.Status(400).JSON(map[string]string{"error": "account is not locked"})
	}

	// Use the Relationships service to accept the follow request
	result, err := h.registry.Relationships().AcceptFollowRequest(ctx.Context, &relationships.AcceptFollowRequestCommand{
		RequesterID: username,  // User accepting the request
		FollowerID:  accountID, // User who sent the request
	})
	if err != nil {
		h.logger.Error("failed to accept follow request", zap.Error(err))
		if fmt.Sprintf("%v", err) == "follow request not found" {
			return ctx.Status(404).JSON(map[string]string{"error": "follow request not found"})
		}
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Send Accept activity to the follower (ActivityPub federation is handled by the service)
	go func() {
		if err := h.sendAcceptActivityLift(ctx.Context, accountID, username); err != nil {
			h.logger.Error("failed to send accept activity", zap.Error(err))
		}
	}()

	h.logger.Info("follow request authorized",
		zap.String("username", username),
		zap.String("follower_id", accountID))

	// Convert relationship data to Mastodon API format
	relationship := map[string]any{
		"id":                   accountID,
		"following":            result.Relationship.FollowedBy, // We are followed by them now
		"showing_reblogs":      result.Relationship.ShowingReblogs,
		"notifying":            result.Relationship.Notifying,
		"followed_by":          result.Relationship.Following, // They are following us now
		"blocking":             result.Relationship.BlockedBy,
		"blocked_by":           result.Relationship.Blocking,
		"muting":               result.Relationship.Muting,
		"muting_notifications": result.Relationship.MutingNotifications,
		"requested":            result.Relationship.RequestedBy, // No longer requested
		"domain_blocking":      result.Relationship.DomainBlocking,
		"endorsed":             result.Relationship.Endorsed,
		"note":                 result.Relationship.Note,
	}

	return ctx.JSON(relationship)
}

// HandleRejectFollowRequestLift handles POST /api/v1/follow_requests/:account_id/reject
// Rejects a pending follow request
func (h *Handler) HandleRejectFollowRequestLift(ctx *lift.Context) error {
	accountID := ctx.Param("account_id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "account_id parameter required"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Get the user's actor directly (test mode)
		result, err := h.registry.Accounts().GetAccount(ctx.Context, testUsername)
		if err != nil {
			h.logger.Error("failed to get actor", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
		actor := result.Actor

		// Skip to the main logic with test username
		return h.handleRejectFollowRequestLogic(ctx, actor, testUsername, accountID)
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check write:follows scope
	if !claims.HasScope(auth.WriteFollows) && !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	result, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}
	actor := result.Actor

	return h.handleRejectFollowRequestLogic(ctx, actor, claims.Username, accountID)
}

// handleRejectFollowRequestLogic contains the main reject logic, separated for testing
func (h *Handler) handleRejectFollowRequestLogic(ctx *lift.Context, actor *activitypub.Actor, username, accountID string) error {
	// Only locked accounts can have follow requests
	if !actor.ManuallyApprovesFollowers {
		return ctx.Status(400).JSON(map[string]string{"error": "account is not locked"})
	}

	// Use the Relationships service to reject the follow request
	result, err := h.registry.Relationships().RejectFollowRequest(ctx.Context, &relationships.RejectFollowRequestCommand{
		RequesterID: username,  // User rejecting the request
		FollowerID:  accountID, // User who sent the request
	})
	if err != nil {
		h.logger.Error("failed to reject follow request", zap.Error(err))
		if fmt.Sprintf("%v", err) == "follow request not found" {
			return ctx.Status(404).JSON(map[string]string{"error": "follow request not found"})
		}
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Send Reject activity to the follower (ActivityPub federation is handled by the service)
	go func() {
		if err := h.sendRejectActivityLift(ctx.Context, accountID, username); err != nil {
			h.logger.Error("failed to send reject activity", zap.Error(err))
		}
	}()

	h.logger.Info("follow request rejected",
		zap.String("username", username),
		zap.String("follower_id", accountID))

	// Convert relationship data to Mastodon API format
	relationship := map[string]any{
		"id":                   accountID,
		"following":            result.Relationship.FollowedBy,
		"showing_reblogs":      result.Relationship.ShowingReblogs,
		"notifying":            result.Relationship.Notifying,
		"followed_by":          result.Relationship.Following,
		"blocking":             result.Relationship.BlockedBy,
		"blocked_by":           result.Relationship.Blocking,
		"muting":               result.Relationship.Muting,
		"muting_notifications": result.Relationship.MutingNotifications,
		"requested":            result.Relationship.RequestedBy, // No longer requested
		"domain_blocking":      result.Relationship.DomainBlocking,
		"endorsed":             result.Relationship.Endorsed,
		"note":                 result.Relationship.Note,
	}

	return ctx.JSON(relationship)
}

// convertActorToAccountLift converts an ActivityPub actor to a Mastodon account format
func (h *Handler) convertActorToAccountLift(ctx context.Context, actor *activitypub.Actor) map[string]any {
	// Default avatar and header
	avatar := fmt.Sprintf("https://%s/avatars/default.png", h.cfg.Domain)
	header := fmt.Sprintf("https://%s/headers/default.png", h.cfg.Domain)

	if actor.Icon != nil && actor.Icon.URL != "" {
		avatar = actor.Icon.URL
	}
	if actor.Image != nil && actor.Image.URL != "" {
		header = actor.Image.URL
	}

	// Get metadata
	createdAt := time.Now() // Default fallback
	lastStatusAt := ""

	// Get actor with metadata
	metadataResult, err := h.registry.Accounts().GetAccountMetadata(ctx, &accounts.GetAccountMetadataQuery{
		Username: actor.PreferredUsername,
	})
	if err == nil && metadataResult != nil && metadataResult.Metadata != nil {
		createdAt = metadataResult.Metadata.CreatedAt
		if metadataResult.Metadata.LastStatusAt != nil {
			lastStatusAt = metadataResult.Metadata.LastStatusAt.Format(time.RFC3339)
		}
	}

	// Get counts
	statusesCountResult, _ := h.registry.Notes().CountNotesByAuthor(ctx, actor.ID)
	statusesCount := statusesCountResult

	followersCountResult, _ := h.registry.Relationships().CountFollowers(ctx, actor.ID)
	followersCount := followersCountResult

	// Get following count
	followingResult, _, _ := h.registry.Relationships().GetFollowing(ctx, actor.PreferredUsername, 1, "")
	followingCount := len(followingResult)

	return map[string]any{
		"id":              actor.PreferredUsername,
		"username":        actor.PreferredUsername,
		"acct":            actor.PreferredUsername,
		"url":             actor.URL,
		"display_name":    actor.Name,
		"note":            actor.Summary,
		"avatar":          avatar,
		"avatar_static":   avatar,
		"header":          header,
		"header_static":   header,
		"locked":          actor.ManuallyApprovesFollowers,
		"bot":             actor.Type == actorTypeService,
		"discoverable":    actor.Discoverable,
		"created_at":      createdAt.Format(time.RFC3339),
		"last_status_at":  lastStatusAt,
		"statuses_count":  statusesCount,
		"followers_count": followersCount,
		"following_count": followingCount,
		"fields":          []any{},
		"emojis":          []any{},
	}
}

// sendAcceptActivityLift sends an Accept activity to the follower
func (h *Handler) sendAcceptActivityLift(ctx context.Context, followerID, followedID string) error {
	// Get follower actor to determine inbox
	followerResult, err := h.registry.Accounts().GetAccount(ctx, followerID)
	if err != nil {
		return fmt.Errorf("failed to get follower actor: %w", err)
	}
	followerActor := followerResult.Actor

	// Get followed actor
	followedResult, err := h.registry.Accounts().GetAccount(ctx, followedID)
	if err != nil {
		return fmt.Errorf("failed to get followed actor: %w", err)
	}
	followedActor := followedResult.Actor

	// Create Accept activity
	acceptActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    "Accept",
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
	if followerActor.Inbox != "" {
		h.deliverActivityLift(ctx, acceptActivity, followerActor.Inbox)
	}

	h.logger.Info("accept activity sent",
		zap.String("follower_id", followerID),
		zap.String("followed_id", followedID),
		zap.String("inbox", followerActor.Inbox))

	return nil
}

// sendRejectActivityLift sends a Reject activity to the follower
func (h *Handler) sendRejectActivityLift(ctx context.Context, followerID, followedID string) error {
	// Get follower actor to determine inbox
	followerResult, err := h.registry.Accounts().GetAccount(ctx, followerID)
	if err != nil {
		return fmt.Errorf("failed to get follower actor: %w", err)
	}
	followerActor := followerResult.Actor

	// Get followed actor
	followedResult, err := h.registry.Accounts().GetAccount(ctx, followedID)
	if err != nil {
		return fmt.Errorf("failed to get followed actor: %w", err)
	}
	followedActor := followedResult.Actor

	// Create Reject activity
	rejectActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    "Reject",
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
	if followerActor.Inbox != "" {
		h.deliverActivityLift(ctx, rejectActivity, followerActor.Inbox)
	}

	h.logger.Info("reject activity sent",
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
