package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandlePinStatus handles POST /api/v1/statuses/:id/pin
func (h *Handler) HandlePinStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to verify ownership
	object, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Check if the user owns this object
	var attributedTo string
	switch obj := object.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case map[string]interface{}:
		if attr, ok := obj["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo != actor.ID {
		return common.Forbidden(fmt.Errorf("you can only pin your own statuses")), nil
	}

	// Create pin
	pin := &storage.StatusPin{
		Username:  claims.Username,
		StatusID:  objectID,
		CreatedAt: time.Now(),
	}

	// Store the pin
	if err := h.store.CreateStatusPin(ctx, pin); err != nil {
		if strings.Contains(err.Error(), "already pinned") {
			return common.UnprocessableEntity(fmt.Errorf("status already pinned")), nil
		}
		if strings.Contains(err.Error(), "too many pinned") {
			return common.UnprocessableEntity(fmt.Errorf("too many pinned statuses (maximum 5)")), nil
		}
		h.logger.Error("failed to pin status", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return the status with pinned flag set to true
	status := h.converter.ObjectToStatus(object, actor)
	status.Pinned = true

	return common.OK(status), nil
}

// HandleUnpinStatus handles POST /api/v1/statuses/:id/unpin
func (h *Handler) HandleUnpinStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Delete the pin
	if err := h.store.DeleteStatusPin(ctx, claims.Username, objectID); err != nil {
		h.logger.Error("failed to unpin status", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get the object to return status information
	object, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Get actor
	actor, _ := h.store.GetActor(ctx, claims.Username)

	// Return the status with pinned flag set to false
	status := h.converter.ObjectToStatus(object, actor)
	status.Pinned = false

	return common.OK(status), nil
}

// HandleMuteConversation handles POST /api/v1/statuses/:id/mute
func (h *Handler) HandleMuteConversation(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Find the root conversation ID (for threads, this would be the root post)
	// For now, we'll use the status ID itself as the conversation ID
	conversationID := objectID

	// Parse optional duration from request body
	var params struct {
		Duration int `json:"duration"` // Duration in seconds (0 = indefinite)
	}
	if request.Body != "" {
		json.Unmarshal([]byte(request.Body), &params)
	}

	// Create conversation mute
	mute := &storage.ConversationMute{
		Username:       claims.Username,
		ConversationID: conversationID,
		CreatedAt:      time.Now(),
	}

	// Set expiration if duration is specified
	if params.Duration > 0 {
		mute.ExpiresAt = time.Now().Add(time.Duration(params.Duration) * time.Second)
	}

	// Store the mute
	if err := h.store.CreateConversationMute(ctx, mute); err != nil {
		if strings.Contains(err.Error(), "already muted") {
			// Update existing mute (idempotent)
			h.store.DeleteConversationMute(ctx, claims.Username, conversationID)
			h.store.CreateConversationMute(ctx, mute)
		} else {
			h.logger.Error("failed to mute conversation", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	}

	// Get the object to return status information
	object, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Get actor
	actor, _ := h.store.GetActor(ctx, claims.Username)

	// Return the status with muted flag set to true
	status := h.converter.ObjectToStatus(object, actor)
	status.Muted = true

	return common.OK(status), nil
}

// HandleUnmuteConversation handles POST /api/v1/statuses/:id/unmute
func (h *Handler) HandleUnmuteConversation(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Use the status ID as the conversation ID
	conversationID := objectID

	// Delete the mute
	if err := h.store.DeleteConversationMute(ctx, claims.Username, conversationID); err != nil {
		h.logger.Error("failed to unmute conversation", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get the object to return status information
	object, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Get actor
	actor, _ := h.store.GetActor(ctx, claims.Username)

	// Return the status with muted flag set to false
	status := h.converter.ObjectToStatus(object, actor)
	status.Muted = false

	return common.OK(status), nil
}
