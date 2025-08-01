package lift

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleFollowLift handles POST /api/v1/accounts/:id/follow
func (h *Handler) HandleFollowLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing account id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write:follows scope
		if !claims.HasScope("write:follows") && !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get the target actor
	targetActor, err := h.store.GetActor(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "account not found"})
	}

	// Create a Follow activity
	followActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      fmt.Sprintf("%s/activities/follow-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:      []string{targetActor.ID},
		},
		Actor:  actor.ID,
		Object: targetActor.ID,
	}
	now := time.Now()
	followActivity.Published = &now

	// Create the follow relationship record
	if err := h.store.CreateFollow(ctx.Context, username, accountID, followActivity.ID); err != nil {
		h.logger.Error("failed to create follow relationship", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Auto-accept if target doesn't require manual approval
	if !targetActor.ManuallyApprovesFollowers {
		if err := h.store.AcceptFollow(ctx.Context, username, accountID); err != nil {
			h.logger.Error("failed to auto-accept follow", zap.Error(err))
			// Don't return error - the follow was created, just not auto-accepted
		}
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx.Context, followActivity); err != nil {
		h.logger.Error("failed to create follow activity", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert to Mastodon relationship format
	relationship := models.Relationship{
		ID:                  targetActor.PreferredUsername,
		Following:           true,
		Requested:           targetActor.ManuallyApprovesFollowers, // If they manually approve, it's a request
		FollowedBy:          false,                                 // We don't know this yet
		Muting:              false,
		MutingNotifications: false,
		ShowingReblogs:      true,
		Notifying:           false,
		Blocking:            false,
		BlockedBy:           false,
		DomainBlocking:      false,
		Endorsed:            false,
		Note:                "",
	}

	// Check the final relationship status after creating the follow
	isFollowing, err := h.store.IsFollowing(ctx.Context, username, accountID)
	if err == nil && isFollowing {
		// Follow was accepted (either manually approved accounts or auto-accepted)
		relationship.Following = true
		relationship.Requested = false
	} else {
		// Follow is pending approval
		relationship.Following = false
		relationship.Requested = targetActor.ManuallyApprovesFollowers
	}

	return ctx.JSON(relationship)
}

// HandleUnfollowLift handles POST /api/v1/accounts/:id/unfollow
func (h *Handler) HandleUnfollowLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing account id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write:follows scope
		if !claims.HasScope("write:follows") && !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get the target actor
	targetActor, err := h.store.GetActor(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "account not found"})
	}

	// Check if following
	isFollowing, err := h.store.IsFollowing(ctx.Context, username, accountID)
	if err != nil || !isFollowing {
		// Not following, but return success anyway for idempotency
		h.logger.Info("follow not found",
			zap.String("actor", actor.ID),
			zap.String("target", targetActor.ID))
	} else {
		// Create an Undo Follow activity
		undoActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      fmt.Sprintf("%s/activities/undo-follow-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
				To:      []string{targetActor.ID},
			},
			Actor: actor.ID,
			Object: map[string]any{
				"type":   activitypub.FollowType,
				"actor":  actor.ID,
				"object": targetActor.ID,
			},
		}
		now := time.Now()
		undoActivity.Published = &now

		// Remove the follow relationship record
		if err := h.store.RemoveFollow(ctx.Context, username, accountID); err != nil {
			h.logger.Error("failed to remove follow relationship", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}

		// Store the activity in the outbox (this will trigger delivery)
		if err := h.store.CreateActivity(ctx.Context, undoActivity); err != nil {
			h.logger.Error("failed to create undo follow activity", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
	}

	// Convert to Mastodon relationship format
	relationship := models.Relationship{
		ID:                  targetActor.PreferredUsername,
		Following:           false,
		Requested:           false,
		FollowedBy:          false, // We don't know this yet
		Muting:              false,
		MutingNotifications: false,
		ShowingReblogs:      true,
		Notifying:           false,
		Blocking:            false,
		BlockedBy:           false,
		DomainBlocking:      false,
		Endorsed:            false,
		Note:                "",
	}

	// Check if following now
	isFollowing, err = h.store.IsFollowing(ctx.Context, username, accountID)
	if err == nil && isFollowing {
		relationship.Following = true
	}

	return ctx.JSON(relationship)
}

// HandleBlockLift handles POST /api/v1/accounts/:id/block
func (h *Handler) HandleBlockLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing account id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write:blocks scope
		if !claims.HasScope("write:blocks") && !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get the target actor
	targetActor, err := h.store.GetActor(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "account not found"})
	}

	// Create a Block activity
	blockActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.BlockType,
			ID:      fmt.Sprintf("%s/activities/block-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:      []string{targetActor.ID},
		},
		Actor:  actor.ID,
		Object: targetActor.ID,
	}
	now := time.Now()
	blockActivity.Published = &now

	// Store the block
	block := &storage.Block{
		Actor:     actor.ID,
		Object:    targetActor.ID,
		ID:        blockActivity.ID,
		Published: now,
		CreatedAt: now,
	}
	if err := h.store.CreateBlock(ctx.Context, block); err != nil {
		h.logger.Error("failed to create block", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx.Context, blockActivity); err != nil {
		h.logger.Error("failed to create block activity", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Unfollow if following
	isFollowing, err := h.store.IsFollowing(ctx.Context, username, accountID)
	if err == nil && isFollowing {
		// Create an Undo Follow activity
		undoFollowActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      fmt.Sprintf("%s/activities/undo-follow-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
				To:      []string{targetActor.ID},
			},
			Actor: actor.ID,
			Object: map[string]any{
				"type":   activitypub.FollowType,
				"actor":  actor.ID,
				"object": targetActor.ID,
			},
		}
		undoFollowActivity.Published = &now
		if err := h.store.CreateActivity(ctx.Context, undoFollowActivity); err != nil {
			// Log error but continue with the response
		}
	}

	// Convert to Mastodon relationship format
	relationship := models.Relationship{
		ID:                  targetActor.PreferredUsername,
		Following:           false,
		Requested:           false,
		FollowedBy:          false,
		Muting:              false,
		MutingNotifications: false,
		ShowingReblogs:      false,
		Notifying:           false,
		Blocking:            true,
		BlockedBy:           false,
		DomainBlocking:      false,
		Endorsed:            false,
		Note:                "",
	}

	// Check if following now
	isFollowing, err = h.store.IsFollowing(ctx.Context, username, accountID)
	if err == nil && isFollowing {
		relationship.Following = true
	}

	return ctx.JSON(relationship)
}

// HandleUnblockLift handles POST /api/v1/accounts/:id/unblock
func (h *Handler) HandleUnblockLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing account id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write:blocks scope
		if !claims.HasScope("write:blocks") && !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get the target actor
	targetActor, err := h.store.GetActor(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "account not found"})
	}

	// Check if blocked
	_, err = h.store.GetBlock(ctx.Context, actor.ID, targetActor.ID)
	if err != nil {
		// Not blocked, but return success anyway for idempotency
		h.logger.Info("block not found",
			zap.String("actor", actor.ID),
			zap.String("target", targetActor.ID))
	} else {
		// Delete the block
		if err := h.store.DeleteBlock(ctx.Context, actor.ID, targetActor.ID); err != nil {
			h.logger.Error("failed to delete block", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}

		// Create an Undo Block activity
		undoActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      fmt.Sprintf("%s/activities/undo-block-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
				To:      []string{targetActor.ID},
			},
			Actor: actor.ID,
			Object: map[string]any{
				"type":   activitypub.BlockType,
				"actor":  actor.ID,
				"object": targetActor.ID,
			},
		}
		now := time.Now()
		undoActivity.Published = &now

		// Store the activity in the outbox (this will trigger delivery)
		if err := h.store.CreateActivity(ctx.Context, undoActivity); err != nil {
			h.logger.Error("failed to create undo block activity", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
	}

	// Convert to Mastodon relationship format
	relationship := models.Relationship{
		ID:                  targetActor.PreferredUsername,
		Following:           false,
		Requested:           false,
		FollowedBy:          false, // We don't know this yet
		Muting:              false,
		MutingNotifications: false,
		ShowingReblogs:      true,
		Notifying:           false,
		Blocking:            false,
		BlockedBy:           false,
		DomainBlocking:      false,
		Endorsed:            false,
		Note:                "",
	}

	// Check if following now
	isFollowing, err := h.store.IsFollowing(ctx.Context, username, accountID)
	if err == nil && isFollowing {
		relationship.Following = true
	}

	return ctx.JSON(relationship)
}

// HandleGetBlocksLift handles GET /api/v1/blocks
func (h *Handler) HandleGetBlocksLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check read:blocks scope
		if !claims.HasScope("read:blocks") && !claims.HasScope(auth.ScopeRead) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Parse query parameters
	maxID := ctx.Query("max_id")
	if maxID == "" && ctx.Request != nil && ctx.Request.Request != nil {
		maxID = ctx.Request.Request.QueryParams["max_id"]
	}

	// Get blocks
	blocks, cursor, err := h.store.GetBlockedActors(ctx.Context, actor.ID, 40, maxID)
	if err != nil {
		h.logger.Error("failed to get blocks", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get blocks"})
	}

	// Convert blocked actor IDs to accounts
	accounts := []models.Account{}
	for _, block := range blocks {
		blockedID := block.Object
		// Extract username from actor ID
		parts := strings.Split(blockedID, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			blockedActor, err := h.store.GetActor(ctx.Context, username)
			if err != nil {
				h.logger.Warn("failed to get blocked actor", zap.String("actor_id", blockedID), zap.Error(err))
				continue
			}

			account := models.Account{
				ID:             blockedActor.PreferredUsername,
				Username:       blockedActor.PreferredUsername,
				Acct:           blockedActor.PreferredUsername,
				DisplayName:    blockedActor.Name,
				URL:            blockedActor.URL,
				CreatedAt:      h.formatActorCreatedTime(blockedActor.CreatedAt),
				Note:           blockedActor.Summary,
				Avatar:         "",
				AvatarStatic:   "",
				Header:         "",
				HeaderStatic:   "",
				FollowersCount: 0,
				FollowingCount: 0,
				StatusesCount:  0,
				Emojis:         []any{},
				Fields:         []any{},
			}

			if blockedActor.Icon != nil {
				account.Avatar = blockedActor.Icon.URL
				account.AvatarStatic = blockedActor.Icon.URL
			}

			accounts = append(accounts, account)
		}
	}

	// Set Link header for pagination if there's a cursor
	if cursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/blocks?max_id=%s>; rel="next"`,
			h.cfg.BaseURL(), cursor)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(accounts)
}

// Helper function to format actor created time
func (h *Handler) formatActorCreatedTime(createdAt *time.Time) string {
	if createdAt == nil {
		return time.Now().Format(time.RFC3339)
	}
	return createdAt.Format(time.RFC3339)
}

// HandleFavoriteLift handles POST /api/v1/statuses/:id/favourite
func (h *Handler) HandleFavoriteLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Create a Like activity
	likeActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.LikeType,
			ID:      fmt.Sprintf("%s/activities/like-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:      []string{activitypub.PublicAddress},
		},
		Actor:  actor.ID,
		Object: objectID,
	}
	now := time.Now()
	likeActivity.Published = &now

	// Create the Like record in dedicated storage
	like := &storage.Like{
		Actor:     actor.ID,
		Object:    objectID,
		ID:        likeActivity.ID,
		Published: *likeActivity.Published,
	}
	if err := h.store.CreateLike(ctx.Context, like); err != nil {
		h.logger.Error("failed to create like", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx.Context, likeActivity); err != nil {
		h.logger.Error("failed to create like activity", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Record engagement for trending
	if err := h.store.RecordStatusEngagement(ctx.Context, objectID, "like", actor.ID); err != nil {
		h.logger.Warn("failed to record status engagement",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Get object to return status information
	_, err = h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		// If object not found locally, still return success but with minimal info
		h.logger.Warn("object not found locally", zap.String("object_id", objectID), zap.Error(err))
	}

	// Get like count for the object
	likeCount, _ := h.store.CountObjectLikes(ctx.Context, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:              statusID,
		CreatedAt:       likeActivity.Published.Format("2006-01-02T15:04:05.000Z"),
		Favourited:      true,
		FavouritesCount: likeCount,
		URI:             objectID,
		URL:             objectID,
		Content:         "", // Would be populated from object
		Visibility:      "public",
		Language:        "en",
	}

	return ctx.JSON(resp)
}

// HandleUnfavoriteLift handles POST /api/v1/statuses/:id/unfavourite
func (h *Handler) HandleUnfavoriteLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the like exists
	_, err = h.store.GetLike(ctx.Context, actor.ID, objectID)
	if err != nil {
		// Like doesn't exist, return success anyway for idempotency
		h.logger.Info("like not found",
			zap.String("actor", actor.ID),
			zap.String("object", objectID))
	} else {
		// Create an Undo Like activity
		undoActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      fmt.Sprintf("%s/activities/undo-like-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
				To:      []string{activitypub.PublicAddress},
			},
			Actor: actor.ID,
			Object: map[string]any{
				"type":   activitypub.LikeType,
				"actor":  actor.ID,
				"object": objectID,
			},
		}
		now := time.Now()
		undoActivity.Published = &now

		// Delete the Like record from dedicated storage
		if err := h.store.DeleteLike(ctx.Context, actor.ID, objectID); err != nil {
			h.logger.Error("failed to delete like", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}

		// Store the activity in the outbox (this will trigger delivery)
		if err := h.store.CreateActivity(ctx.Context, undoActivity); err != nil {
			h.logger.Error("failed to create undo like activity", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
	}

	// Get like count for the object
	likeCount, _ := h.store.CountObjectLikes(ctx.Context, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:              statusID,
		CreatedAt:       time.Now().Format("2006-01-02T15:04:05.000Z"),
		Favourited:      false,
		FavouritesCount: likeCount,
		URI:             objectID,
		URL:             objectID,
		Content:         "", // Would be populated from object
		Visibility:      "public",
		Language:        "en",
	}

	return ctx.JSON(resp)
}

// HandleReblogLift handles POST /api/v1/statuses/:id/reblog
func (h *Handler) HandleReblogLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Create an Announce activity
	announceActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AnnounceType,
			ID:      fmt.Sprintf("%s/activities/announce-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:      []string{activitypub.PublicAddress},
		},
		Actor:  actor.ID,
		Object: objectID,
	}
	if actor.Followers != "" {
		announceActivity.CC = []string{actor.Followers}
	}
	now := time.Now()
	announceActivity.Published = &now

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx.Context, announceActivity); err != nil {
		h.logger.Error("failed to create announce activity", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Record engagement for trending
	if err := h.store.RecordStatusEngagement(ctx.Context, objectID, "boost", actor.ID); err != nil {
		h.logger.Warn("failed to record status engagement",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Get object to return status information
	_, err = h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		// If object not found locally, still return success but with minimal info
		h.logger.Warn("object not found locally", zap.String("object_id", objectID), zap.Error(err))
	}

	// Get announce count for the object
	announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:           statusID,
		CreatedAt:    announceActivity.Published.Format("2006-01-02T15:04:05.000Z"),
		Reblogged:    true,
		ReblogsCount: announceCount,
		URI:          objectID,
		URL:          objectID,
		Content:      "", // Would be populated from object
		Visibility:   "public",
		Language:     "en",
	}

	return ctx.JSON(resp)
}

// HandleUnreblogLift handles POST /api/v1/statuses/:id/unreblog
func (h *Handler) HandleUnreblogLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the announce exists
	_, err = h.store.GetAnnounce(ctx.Context, actor.ID, objectID)
	if err != nil {
		// Announce doesn't exist, return success anyway for idempotency
		h.logger.Info("announce not found",
			zap.String("actor", actor.ID),
			zap.String("object", objectID))
	} else {
		// Create an Undo Announce activity
		undoActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      fmt.Sprintf("%s/activities/undo-announce-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
				To:      []string{activitypub.PublicAddress},
			},
			Actor: actor.ID,
			Object: map[string]any{
				"type":   activitypub.AnnounceType,
				"actor":  actor.ID,
				"object": objectID,
			},
		}
		now := time.Now()
		undoActivity.Published = &now

		// Delete the Announce record from dedicated storage
		if err := h.store.DeleteAnnounce(ctx.Context, actor.ID, objectID); err != nil {
			h.logger.Error("failed to delete announce", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}

		// Store the activity in the outbox (this will trigger delivery)
		if err := h.store.CreateActivity(ctx.Context, undoActivity); err != nil {
			h.logger.Error("failed to create undo announce activity", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
	}

	// Get announce count for the object
	announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:           statusID,
		CreatedAt:    time.Now().Format("2006-01-02T15:04:05.000Z"),
		Reblogged:    false,
		ReblogsCount: announceCount,
		URI:          objectID,
		URL:          objectID,
		Content:      "", // Would be populated from object
		Visibility:   "public",
		Language:     "en",
	}

	return ctx.JSON(resp)
}

// generateRandomString generates a random string of the specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}