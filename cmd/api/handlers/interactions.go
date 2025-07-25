package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleFollow follows an account
func (h *Handler) HandleFollow(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:follows scope
	if !claims.HasScope("write:follows") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get the target actor
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
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
	if err := h.store.CreateFollow(ctx, claims.Username, accountID, followActivity.ID); err != nil {
		h.logger.Error("failed to create follow relationship", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Auto-accept if target doesn't require manual approval
	if !targetActor.ManuallyApprovesFollowers {
		if err := h.store.AcceptFollow(ctx, claims.Username, accountID); err != nil {
			h.logger.Error("failed to auto-accept follow", zap.Error(err))
			// Don't return error - the follow was created, just not auto-accepted
		}
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx, followActivity); err != nil {
		h.logger.Error("failed to create follow activity", zap.Error(err))
		return common.InternalServerError(err), nil
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
	isFollowing, err := h.store.IsFollowing(ctx, claims.Username, accountID)
	if err == nil && isFollowing {
		// Follow was accepted (either manually approved accounts or auto-accepted)
		relationship.Following = true
		relationship.Requested = false
	} else {
		// Follow is pending approval
		relationship.Following = false
		relationship.Requested = targetActor.ManuallyApprovesFollowers
	}

	body, _ := json.Marshal(relationship)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleUnfollow unfollows an account
func (h *Handler) HandleUnfollow(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:follows scope
	if !claims.HasScope("write:follows") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get the target actor
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Check if following
	isFollowing, err := h.store.IsFollowing(ctx, claims.Username, accountID)
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
		if err := h.store.RemoveFollow(ctx, claims.Username, accountID); err != nil {
			h.logger.Error("failed to remove follow relationship", zap.Error(err))
			return common.InternalServerError(err), nil
		}

		// Store the activity in the outbox (this will trigger delivery)
		if err := h.store.CreateActivity(ctx, undoActivity); err != nil {
			h.logger.Error("failed to create undo follow activity", zap.Error(err))
			return common.InternalServerError(err), nil
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
	isFollowing, err = h.store.IsFollowing(ctx, claims.Username, accountID)
	if err == nil && isFollowing {
		relationship.Following = true
	}

	body, _ := json.Marshal(relationship)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleBlock blocks an account
func (h *Handler) HandleBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:blocks scope
	if !claims.HasScope("write:blocks") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get the target actor
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
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
	if err := h.store.CreateBlock(ctx, block); err != nil {
		h.logger.Error("failed to create block", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx, blockActivity); err != nil {
		h.logger.Error("failed to create block activity", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Unfollow if following
	isFollowing, err := h.store.IsFollowing(ctx, claims.Username, accountID)
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
		if err := h.store.CreateActivity(ctx, undoFollowActivity); err != nil {
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
	isFollowing, err = h.store.IsFollowing(ctx, claims.Username, accountID)
	if err == nil && isFollowing {
		relationship.Following = true
	}

	body, _ := json.Marshal(relationship)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleUnblock unblocks an account
func (h *Handler) HandleUnblock(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:blocks scope
	if !claims.HasScope("write:blocks") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get the target actor
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Check if blocked
	_, err = h.store.GetBlock(ctx, actor.ID, targetActor.ID)
	if err != nil {
		// Not blocked, but return success anyway for idempotency
		h.logger.Info("block not found",
			zap.String("actor", actor.ID),
			zap.String("target", targetActor.ID))
	} else {
		// Delete the block
		if err := h.store.DeleteBlock(ctx, actor.ID, targetActor.ID); err != nil {
			h.logger.Error("failed to delete block", zap.Error(err))
			return common.InternalServerError(err), nil
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
		if err := h.store.CreateActivity(ctx, undoActivity); err != nil {
			h.logger.Error("failed to create undo block activity", zap.Error(err))
			return common.InternalServerError(err), nil
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
	isFollowing, err := h.store.IsFollowing(ctx, claims.Username, accountID)
	if err == nil && isFollowing {
		relationship.Following = true
	}

	body, _ := json.Marshal(relationship)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetBlocks retrieves the list of blocked accounts
func (h *Handler) HandleGetBlocks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read:blocks scope
	if !claims.HasScope("read:blocks") && !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get blocks
	blocks, cursor, err := h.store.GetBlockedActors(ctx, actor.ID, 40, request.QueryStringParameters["max_id"])
	if err != nil {
		h.logger.Error("failed to get blocks", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert blocked actor IDs to accounts
	accounts := []models.Account{}
	for _, block := range blocks {
		blockedID := block.Object
		// Extract username from actor ID
		parts := strings.Split(blockedID, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			blockedActor, err := h.store.GetActor(ctx, username)
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
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if cursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/blocks?max_id=%s", h.cfg.BaseURL(), cursor)
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	body, _ := json.Marshal(accounts)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}
