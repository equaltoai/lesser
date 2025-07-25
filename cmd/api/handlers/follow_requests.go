package handlers

import (
	"context"
	"errors"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetFollowRequests handles GET /api/v1/follow_requests
// Returns pending follow requests for locked accounts
func (h *Handler) HandleGetFollowRequests(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read:follows scope
	if !claims.HasScope("read:follows") && !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Check if the user has a locked account
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// If account is not locked, return empty array
	if !actor.ManuallyApprovesFollowers {
		return common.OK([]any{}), nil
	}

	// Get pending follow requests
	pendingRequests, _, err := h.store.GetPendingFollowRequests(ctx, claims.Username, 100, "")
	if err != nil {
		h.logger.Error("failed to get pending follow requests", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to account format
	accounts := make([]map[string]any, 0, len(pendingRequests))
	for _, followerID := range pendingRequests {
		// Get follower actor
		followerActor, err := h.store.GetActor(ctx, followerID)
		if err != nil {
			h.logger.Warn("failed to get follower actor",
				zap.String("follower_id", followerID),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := h.convertActorToAccount(ctx, followerActor)
		accounts = append(accounts, account)
	}

	h.logger.Info("follow requests retrieved",
		zap.String("username", claims.Username),
		zap.Int("count", len(accounts)))

	return common.OK(accounts), nil
}

// HandleAuthorizeFollowRequest handles POST /api/v1/follow_requests/:account_id/authorize
// Accepts a pending follow request
func (h *Handler) HandleAuthorizeFollowRequest(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:follows scope
	if !claims.HasScope("write:follows") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Check if the user has a locked account
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Only locked accounts can have follow requests
	if !actor.ManuallyApprovesFollowers {
		return common.BadRequest(errors.New("account is not locked")), nil
	}

	// Find the pending follow request
	_, err = h.store.GetFollowRequest(ctx, accountID, claims.Username)
	if err != nil {
		return common.NotFound(errors.New("follow request not found")), nil
	}

	// Update the relationship state to accepted
	if err := h.store.AcceptFollowRequest(ctx, accountID, claims.Username); err != nil {
		h.logger.Error("failed to accept follow request", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Send Accept activity to the follower
	go func() {
		if err := h.sendAcceptActivity(ctx, accountID, claims.Username); err != nil {
			h.logger.Error("failed to send accept activity", zap.Error(err))
		}
	}()

	h.logger.Info("follow request authorized",
		zap.String("username", claims.Username),
		zap.String("follower_id", accountID))

	// Build relationship response
	relationship := map[string]any{
		"id":                   accountID,
		"following":            false,
		"showing_reblogs":      true,
		"notifying":            false,
		"followed_by":          true, // Now following after authorization
		"blocking":             false,
		"blocked_by":           false,
		"muting":               false,
		"muting_notifications": false,
		"requested":            false, // No longer requested
		"domain_blocking":      false,
		"endorsed":             false,
		"note":                 "",
	}

	return common.OK(relationship), nil
}

// HandleRejectFollowRequest handles POST /api/v1/follow_requests/:account_id/reject
// Rejects a pending follow request
func (h *Handler) HandleRejectFollowRequest(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:follows scope
	if !claims.HasScope("write:follows") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Check if the user has a locked account
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Only locked accounts can have follow requests
	if !actor.ManuallyApprovesFollowers {
		return common.BadRequest(errors.New("account is not locked")), nil
	}

	// Find the pending follow request
	_, err = h.store.GetFollowRequest(ctx, accountID, claims.Username)
	if err != nil {
		return common.NotFound(errors.New("follow request not found")), nil
	}

	// Delete/reject the follow request
	if err := h.store.RejectFollowRequest(ctx, accountID, claims.Username); err != nil {
		h.logger.Error("failed to reject follow request", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Send Reject activity to the follower
	go func() {
		if err := h.sendRejectActivity(ctx, accountID, claims.Username); err != nil {
			h.logger.Error("failed to send reject activity", zap.Error(err))
		}
	}()

	h.logger.Info("follow request rejected",
		zap.String("username", claims.Username),
		zap.String("follower_id", accountID))

	// Build relationship response
	relationship := map[string]any{
		"id":                   accountID,
		"following":            false,
		"showing_reblogs":      false,
		"notifying":            false,
		"followed_by":          false, // No longer following after rejection
		"blocking":             false,
		"blocked_by":           false,
		"muting":               false,
		"muting_notifications": false,
		"requested":            false, // No longer requested
		"domain_blocking":      false,
		"endorsed":             false,
		"note":                 "",
	}

	return common.OK(relationship), nil
}
