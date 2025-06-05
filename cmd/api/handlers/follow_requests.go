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

	// TODO: Implement locked accounts and pending follow requests
	// For now, Lesser doesn't support locked accounts, so we always return an empty array

	// When implementing locked accounts:
	// 1. Check if the authenticated user has a locked account
	// 2. Query for follow relationships with State="pending" where followed=user
	// 3. Convert the follower actors to accounts
	// 4. Return the list with pagination

	h.logger.Info("follow requests requested",
		zap.String("username", claims.Username),
		zap.String("note", "locked accounts not yet implemented"))

	// Return empty array for now
	return common.OK([]interface{}{}), nil
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

	// TODO: Implement locked accounts and follow request authorization
	// For now, since Lesser doesn't support locked accounts, this is a no-op

	// When implementing:
	// 1. Check if the authenticated user has a locked account
	// 2. Find the pending follow relationship from accountID to user
	// 3. Update the relationship state from "pending" to "accepted"
	// 4. Send an Accept activity to the follower
	// 5. Build and return the relationship

	h.logger.Info("follow request authorization attempted",
		zap.String("username", claims.Username),
		zap.String("account_id", accountID),
		zap.String("note", "locked accounts not yet implemented"))

	// For now, build a default relationship response
	// In a real implementation, this would reflect the actual relationship
	relationship := map[string]interface{}{
		"id":                   accountID,
		"following":            false,
		"showing_reblogs":      true,
		"notifying":            false,
		"followed_by":          true, // They were trying to follow us
		"blocking":             false,
		"blocked_by":           false,
		"muting":               false,
		"muting_notifications": false,
		"requested":            false, // No longer requested after authorization
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

	// TODO: Implement locked accounts and follow request rejection
	// For now, since Lesser doesn't support locked accounts, this is a no-op

	// When implementing:
	// 1. Check if the authenticated user has a locked account
	// 2. Find the pending follow relationship from accountID to user
	// 3. Delete the relationship or update state to "rejected"
	// 4. Optionally send a Reject activity to the follower
	// 5. Build and return the relationship

	h.logger.Info("follow request rejection attempted",
		zap.String("username", claims.Username),
		zap.String("account_id", accountID),
		zap.String("note", "locked accounts not yet implemented"))

	// For now, build a default relationship response
	relationship := map[string]interface{}{
		"id":                   accountID,
		"following":            false,
		"showing_reblogs":      false,
		"notifying":            false,
		"followed_by":          false, // They're no longer following after rejection
		"blocking":             false,
		"blocked_by":           false,
		"muting":               false,
		"muting_notifications": false,
		"requested":            false, // No longer requested after rejection
		"domain_blocking":      false,
		"endorsed":             false,
		"note":                 "",
	}

	return common.OK(relationship), nil
}
