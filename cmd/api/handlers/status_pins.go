package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// HandlePinStatusLift handles POST /api/v1/statuses/:id/pin
func (h *Handler) HandlePinStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get status ID from path
	statusID := ctx.Param("id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("status_id_initial", statusID); err != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/pin
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == pathComponentStatuses && parts[5] == "pin" {
			statusID = parts[4]
		}
	}

	if err := common.ValidateMastodonStatusID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Get the user's actor
	actor, err := h.repos.Actor().GetActor(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to verify ownership
	object, err := h.repos.Object().GetObject(ctx.Context(), objectID)
	if err != nil {
		if apperrors.HasCode(err, apperrors.CodeNotFound) || apperrors.HasCode(err, apperrors.CodeActorNotFound) {
			return common.RespondNotFound(ctx, "status")
		}
		return common.RespondInternalServerError(ctx)
	}

	// Check if the user owns this object
	var attributedTo string
	switch obj := object.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case *activitypub.Article:
		attributedTo = obj.AttributedTo
	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo != actor.ID {
		return common.RespondForbidden(ctx, "you can only pin your own statuses")
	}

	// Create pin
	pin := &storage.StatusPin{
		Username:  claims.Username,
		StatusID:  objectID,
		CreatedAt: time.Now(),
	}

	// Store the pin
	if err := h.repos.Social().CreateStatusPin(ctx.Context(), pin); err != nil {
		if apperrors.HasCode(err, apperrors.CodeAlreadyExists) || apperrors.HasCode(err, apperrors.CodeConflict) {
			return nil, err
		}
		h.logger.Error("failed to pin status", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Return the status with pinned flag set to true
	status := transformations.ObjectToStatusAny(object, actor, h.cfg.BaseURL())
	status.Pinned = true

	return okJSON(status)
}

// HandleUnpinStatusLift handles POST /api/v1/statuses/:id/unpin
func (h *Handler) HandleUnpinStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get status ID from path
	statusID := ctx.Param("id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("status_id_initial", statusID); err != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/unpin
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == pathComponentStatuses && parts[5] == "unpin" {
			statusID = parts[4]
		}
	}

	if err := common.ValidateMastodonStatusID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Delete the pin
	if err := h.repos.Social().DeleteStatusPin(ctx.Context(), claims.Username, objectID); err != nil {
		h.logger.Error("failed to unpin status", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Get the object to return status information
	object, err := h.repos.Object().GetObject(ctx.Context(), objectID)
	if err != nil {
		if apperrors.HasCode(err, apperrors.CodeNotFound) || apperrors.HasCode(err, apperrors.CodeActorNotFound) {
			return common.RespondNotFound(ctx, "status")
		}
		return common.RespondInternalServerError(ctx)
	}

	// Get actor
	actor, _ := h.repos.Actor().GetActor(ctx.Context(), claims.Username)

	// Return the status with pinned flag set to false
	status := transformations.ObjectToStatusAny(object, actor, h.cfg.BaseURL())
	status.Pinned = false

	return okJSON(status)
}

// HandleMuteConversationLift handles POST /api/v1/statuses/:id/mute
func (h *Handler) HandleMuteConversationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	claims, err := h.authenticateMuteRequest(ctx)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get and validate status ID
	statusID, err := h.extractMuteStatusID(ctx)
	if err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Normalize status ID to object ID
	objectID := h.normalizeMuteObjectID(statusID)
	conversationID := objectID // For now, conversation ID equals object ID

	// Parse mute duration from request
	duration := h.parseMuteDuration(ctx)

	// Create and store conversation mute
	if err := h.createConversationMute(ctx, claims.Username, conversationID, duration); err != nil {
		h.logger.Error("failed to mute conversation", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Build and return muted status response
	return h.buildMutedStatusResponse(ctx, objectID, claims.Username)
}

// authenticateMuteRequest authenticates the mute request
func (h *Handler) authenticateMuteRequest(ctx *apptheory.Context) (*auth.Claims, error) {
	return h.authenticateWithScope(ctx, auth.ScopeWrite)
}

// extractMuteStatusID extracts the status ID from the request
func (h *Handler) extractMuteStatusID(ctx *apptheory.Context) (string, error) {
	statusID := ctx.Param("id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("status_id_initial", statusID); err != nil {
		statusID = h.extractStatusIDFromPath(ctx, "mute")
	}

	if err := common.ValidateMastodonStatusID(statusID); err != nil {
		return "", err
	}

	return statusID, nil
}

// extractStatusIDFromPath extracts status ID from the request path
func (h *Handler) extractStatusIDFromPath(ctx *apptheory.Context, action string) string {
	if err := common.ValidateRequiredParam("request_path", ctx.Request.Path); err != nil {
		return ""
	}

	// Extract id from path like /api/v1/statuses/test-status-123/{action}
	parts := strings.Split(ctx.Request.Path, "/")
	if len(parts) > 5 && parts[3] == pathComponentStatuses && parts[5] == action {
		return parts[4]
	}
	return ""
}

// normalizeMuteObjectID normalizes the status ID to a full URL
func (h *Handler) normalizeMuteObjectID(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// parseMuteDuration parses the mute duration from request body
func (h *Handler) parseMuteDuration(ctx *apptheory.Context) int {
	var params struct {
		Duration int `json:"duration"` // Duration in seconds (0 = indefinite)
	}

	_ = common.ParseRequestWithFallback(ctx, &params)

	return params.Duration
}

// createConversationMute creates and stores the conversation mute
func (h *Handler) createConversationMute(ctx *apptheory.Context, username, conversationID string, duration int) error {
	mute := &storage.ConversationMute{
		Username:       username,
		ConversationID: conversationID,
		CreatedAt:      time.Now(),
	}

	// Set expiration if duration is specified
	if duration > 0 {
		mute.ExpiresAt = time.Now().Add(time.Duration(duration) * time.Second)
	}

	// Store the mute
	if err := h.storeMuteWithRetry(ctx, username, conversationID, mute); err != nil {
		return err
	}

	return nil
}

// storeMuteWithRetry stores the mute with retry for existing mutes
func (h *Handler) storeMuteWithRetry(ctx *apptheory.Context, username, conversationID string, mute *storage.ConversationMute) error {
	err := h.repos.Conversation().CreateConversationMute(ctx.Context(), mute)
	if err == nil {
		return nil
	}

	if apperrors.HasCode(err, apperrors.CodeAlreadyExists) {
		// Handle already muted - update existing mute (idempotent)
		return h.replaceMute(ctx, username, conversationID, mute)
	}

	h.logger.Error("failed to mute conversation", zap.Error(err))
	return apperrors.InternalWithCause(err, "internal server error")
}

// replaceMute replaces an existing mute
func (h *Handler) replaceMute(ctx *apptheory.Context, username, conversationID string, mute *storage.ConversationMute) error {
	// Delete existing mute
	if err := h.repos.Conversation().DeleteConversationMute(ctx.Context(), username, conversationID); err != nil {
		h.logger.Warn("failed to delete existing conversation mute", zap.Error(err))
	}

	// Create new mute
	if err := h.repos.Conversation().CreateConversationMute(ctx.Context(), mute); err != nil {
		h.logger.Error("failed to recreate conversation mute", zap.Error(err))
		return apperrors.InternalWithCause(err, "internal server error")
	}

	return nil
}

// buildMutedStatusResponse builds the response for the muted status
func (h *Handler) buildMutedStatusResponse(ctx *apptheory.Context, objectID, username string) (*apptheory.Response, error) {
	// Get the object
	object, err := h.repos.Object().GetObject(ctx.Context(), objectID)
	if err != nil {
		if apperrors.HasCode(err, apperrors.CodeNotFound) || apperrors.HasCode(err, apperrors.CodeActorNotFound) {
			return common.RespondNotFound(ctx, "status")
		}
		return common.RespondInternalServerError(ctx)
	}

	// Get actor
	actor, _ := h.repos.Actor().GetActor(ctx.Context(), username)

	// Return the status with muted flag set to true
	status := transformations.ObjectToStatusAny(object, actor, h.cfg.BaseURL())
	status.Muted = true

	return okJSON(status)
}

// HandleUnmuteConversationLift handles POST /api/v1/statuses/:id/unmute
func (h *Handler) HandleUnmuteConversationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get status ID from path
	statusID := ctx.Param("id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("status_id_initial", statusID); err != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/unmute
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == pathComponentStatuses && parts[5] == "unmute" {
			statusID = parts[4]
		}
	}

	if err := common.ValidateMastodonStatusID(statusID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
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
	if err := h.repos.Conversation().DeleteConversationMute(ctx.Context(), claims.Username, conversationID); err != nil {
		h.logger.Error("failed to unmute conversation", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Get the object to return status information
	object, err := h.repos.Object().GetObject(ctx.Context(), objectID)
	if err != nil {
		if apperrors.HasCode(err, apperrors.CodeNotFound) || apperrors.HasCode(err, apperrors.CodeActorNotFound) {
			return common.RespondNotFound(ctx, "status")
		}
		return common.RespondInternalServerError(ctx)
	}

	// Get actor
	actor, _ := h.repos.Actor().GetActor(ctx.Context(), claims.Username)

	// Return the status with muted flag set to false
	status := transformations.ObjectToStatusAny(object, actor, h.cfg.BaseURL())
	status.Muted = false

	return okJSON(status)
}
