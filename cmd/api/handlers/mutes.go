package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// HandleMuteAccount handles POST /api/v1/accounts/:id/mute
func (h *Handler) HandleMuteAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get the account to mute
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil || targetActor == nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Check if already muted
	existingMute, err := h.store.GetMute(ctx, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to check existing mute", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Parse parameters
	hideNotifications := false
	if request.Body != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(request.Body), &params); err == nil {
			if notifications, ok := params["notifications"].(bool); ok {
				hideNotifications = notifications
			}
		}
	}

	// Return existing relationship if already muted
	if existingMute != nil {
		// Update notification setting if different
		if existingMute.HideNotifications != hideNotifications {
			// For now, we'll just return the existing mute
			// In a full implementation, you'd update the mute here
		}

		relationship := h.getRelationship(ctx, claims.Username, accountID)
		return common.OK(relationship), nil
	}

	// Create the mute
	mute := &storage.Mute{
		ID:                uuid.New().String(),
		Actor:             claims.Username,
		Object:            accountID,
		HideNotifications: hideNotifications,
		Published:         time.Now(),
		CreatedAt:         time.Now(),
	}

	if err := h.store.CreateMute(ctx, mute); err != nil {
		h.logger.Error("failed to create mute", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return updated relationship
	relationship := h.getRelationship(ctx, claims.Username, accountID)
	return common.OK(relationship), nil
}

// HandleUnmuteAccount handles POST /api/v1/accounts/:id/unmute
func (h *Handler) HandleUnmuteAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Delete the mute
	if err := h.store.DeleteMute(ctx, claims.Username, accountID); err != nil {
		h.logger.Error("failed to delete mute", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return updated relationship
	relationship := h.getRelationship(ctx, claims.Username, accountID)
	return common.OK(relationship), nil
}

// HandleGetMutedAccounts handles GET /api/v1/mutes
func (h *Handler) HandleGetMutedAccounts(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Parse pagination parameters
	limit := 40
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 80 {
			limit = parsed
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get muted accounts
	mutes, nextCursor, err := h.store.GetMutedActors(ctx, claims.Username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get muted actors", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to account models
	accounts := make([]models.Account, 0, len(mutes))
	for _, mute := range mutes {
		actor, err := h.store.GetActor(ctx, mute.Object)
		if err != nil || actor == nil {
			h.logger.Warn("muted actor not found", zap.String("actor", mute.Object))
			continue
		}

		// Get follower/following counts
		followers, _, _ := h.store.GetFollowers(ctx, actor.PreferredUsername, 0, "")
		following, _, _ := h.store.GetFollowing(ctx, actor.PreferredUsername, 0, "")
		statuses, _, _ := h.store.GetObjectsByActor(ctx, fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), actor.PreferredUsername), "", 0)

		account := h.converter.ActorToAccountWithCounts(actor, len(followers), len(following), len(statuses))
		accounts = append(accounts, account)
	}

	// Build response with Link header for pagination
	resp := &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}

	if nextCursor != "" {
		resp.Headers["Link"] = fmt.Sprintf("<%s/api/v1/mutes?max_id=%s>; rel=\"next\"", h.cfg.BaseURL(), nextCursor)
	}

	body, _ := json.Marshal(accounts)
	resp.Body = string(body)

	return resp, nil
}

// getRelationship is a helper to get the relationship between two users
func (h *Handler) getRelationship(ctx context.Context, sourceUsername, targetUsername string) *models.Relationship {
	// Check various relationship states
	following, _ := h.store.IsFollowing(ctx, sourceUsername, targetUsername)
	followedBy, _ := h.store.IsFollowing(ctx, targetUsername, sourceUsername)
	blocked, _ := h.store.IsBlocked(ctx, sourceUsername, targetUsername)
	blockedBy, _ := h.store.IsBlocked(ctx, targetUsername, sourceUsername)

	// Check mute status
	mute, _ := h.store.GetMute(ctx, sourceUsername, targetUsername)

	relationship := &models.Relationship{
		ID:                  targetUsername,
		Following:           following,
		FollowedBy:          followedBy,
		Blocking:            blocked,
		BlockedBy:           blockedBy,
		Muting:              mute != nil,
		MutingNotifications: mute != nil && mute.HideNotifications,
		ShowingReblogs:      true, // Default to true
		Notifying:           false,
		Requested:           false,
		DomainBlocking:      false,
		Endorsed:            false,
		Note:                "",
	}

	return relationship
}
