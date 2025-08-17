package lift

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandlePinStatusLift handles POST /api/v1/statuses/:id/pin
func (h *Handler) HandlePinStatusLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if err := common.ValidateRequiredParam("test_username", testUsername); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	if err := common.ValidateRequiredParam("test_username", testUsername); err == nil {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if err := common.ValidateRequiredParam("token", token); err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid token",
			})
		}
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "insufficient scope",
		})
	}

	// Get status ID from path
	statusID := ctx.Param("id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("status_id_initial", statusID); err != nil && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/pin
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == pathComponentStatuses && parts[5] == "pin" {
			statusID = parts[4]
		}
	}

	if err := common.ValidateRequiredParam("status_id", statusID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}

	// Get the user's actor
	actor, err := h.repos.Actor().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to verify ownership
	object, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Check if the user owns this object
	var attributedTo string
	switch obj := object.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo != actor.ID {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "you can only pin your own statuses",
		})
	}

	// Create pin
	pin := &storage.StatusPin{
		Username:  claims.Username,
		StatusID:  objectID,
		CreatedAt: time.Now(),
	}

	// Store the pin
	if err := h.repos.Social().CreateStatusPin(ctx.Context, pin); err != nil {
		if strings.Contains(err.Error(), "already pinned") {
			ctx.Status(http.StatusUnprocessableEntity)
			return ctx.JSON(map[string]string{
				"error": "status already pinned",
			})
		}
		if strings.Contains(err.Error(), "too many pinned") {
			ctx.Status(http.StatusUnprocessableEntity)
			return ctx.JSON(map[string]string{
				"error": "too many pinned statuses (maximum 5)",
			})
		}
		h.logger.Error("failed to pin status", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Return the status with pinned flag set to true
	status := transformations.ObjectToStatusAny(object, actor, h.cfg.BaseURL())
	status.Pinned = true

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// HandleUnpinStatusLift handles POST /api/v1/statuses/:id/unpin
func (h *Handler) HandleUnpinStatusLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if err := common.ValidateRequiredParam("test_username", testUsername); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	if err := common.ValidateRequiredParam("test_username", testUsername); err == nil {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if err := common.ValidateRequiredParam("token", token); err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid token",
			})
		}
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "insufficient scope",
		})
	}

	// Get status ID from path
	statusID := ctx.Param("id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("status_id_initial", statusID); err != nil && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/unpin
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == pathComponentStatuses && parts[5] == "unpin" {
			statusID = parts[4]
		}
	}

	if err := common.ValidateRequiredParam("status_id", statusID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Delete the pin
	if err := h.repos.Social().DeleteStatusPin(ctx.Context, claims.Username, objectID); err != nil {
		h.logger.Error("failed to unpin status", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get the object to return status information
	object, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Get actor
	actor, _ := h.repos.Actor().GetActor(ctx.Context, claims.Username)

	// Return the status with pinned flag set to false
	status := transformations.ObjectToStatusAny(object, actor, h.cfg.BaseURL())
	status.Pinned = false

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// HandleMuteConversationLift handles POST /api/v1/statuses/:id/mute
func (h *Handler) HandleMuteConversationLift(ctx *lift.Context) error {
	// Authenticate user
	claims, err := h.authenticateMuteRequest(ctx)
	if err != nil {
		return err
	}

	// Get and validate status ID
	statusID, err := h.extractMuteStatusID(ctx)
	if err != nil {
		return err
	}

	// Normalize status ID to object ID
	objectID := h.normalizeMuteObjectID(statusID)
	conversationID := objectID // For now, conversation ID equals object ID

	// Parse mute duration from request
	duration := h.parseMuteDuration(ctx)

	// Create and store conversation mute
	if err := h.createConversationMute(ctx, claims.Username, conversationID, duration); err != nil {
		return err
	}

	// Build and return muted status response
	return h.buildMutedStatusResponse(ctx, objectID, claims.Username)
}

// authenticateMuteRequest authenticates the mute request
func (h *Handler) authenticateMuteRequest(ctx *lift.Context) (*auth.Claims, error) {
	// Check for test mode
	testUsername := h.getMuteTestUsername(ctx)
	if err := common.ValidateRequiredParam("test_username", testUsername); err == nil {
		return h.createTestClaims(testUsername), nil
	}

	// Normal authentication flow
	return h.authenticateWithWriteScope(ctx)
}

// getMuteTestUsername extracts test username from headers
func (h *Handler) getMuteTestUsername(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if err := common.ValidateRequiredParam("test_username", testUsername); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// createTestClaims creates test mode claims
func (h *Handler) createTestClaims(username string) *auth.Claims {
	return &auth.Claims{
		Username: username,
		Scopes:   []string{auth.ScopeWrite},
	}
}

// authenticateWithWriteScope authenticates and checks write scope
func (h *Handler) authenticateWithWriteScope(ctx *lift.Context) (*auth.Claims, error) {
	// Extract token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return nil, ctx.JSON(map[string]string{"error": "authentication required"})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return nil, ctx.JSON(map[string]string{"error": "invalid token"})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		ctx.Status(http.StatusForbidden)
		return nil, ctx.JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims, nil
}

// extractMuteStatusID extracts the status ID from the request
func (h *Handler) extractMuteStatusID(ctx *lift.Context) (string, error) {
	statusID := ctx.Param("id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("status_id_initial", statusID); err != nil {
		statusID = h.extractStatusIDFromPath(ctx, "mute")
	}

	if err := common.ValidateRequiredParam("statusID", statusID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return "", ctx.JSON(map[string]string{"error": "status ID is required"})
	}

	return statusID, nil
}

// extractStatusIDFromPath extracts status ID from the request path
func (h *Handler) extractStatusIDFromPath(ctx *lift.Context, action string) string {
	if ctx.Request == nil {
		return ""
	}
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
func (h *Handler) parseMuteDuration(ctx *lift.Context) int {
	if ctx.Request == nil || ctx.Request.Body == nil {
		return 0
	}
	if err := common.ValidateSliceNotEmpty("requestBody", ctx.Request.Body); err != nil {
		return 0
	}

	var params struct {
		Duration int `json:"duration"` // Duration in seconds (0 = indefinite)
	}

	if err := ctx.ParseRequest(&params); err != nil {
		// Fallback for test environments
		if err := json.Unmarshal(ctx.Request.Body, &params); err != nil {
			h.logger.Warn("failed to parse request body for mute duration", zap.Error(err))
		}
	}

	return params.Duration
}

// createConversationMute creates and stores the conversation mute
func (h *Handler) createConversationMute(ctx *lift.Context, username, conversationID string, duration int) error {
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
func (h *Handler) storeMuteWithRetry(ctx *lift.Context, username, conversationID string, mute *storage.ConversationMute) error {
	err := h.repos.Conversation().CreateConversationMute(ctx.Context, mute)
	if err == nil {
		return nil
	}

	if !strings.Contains(err.Error(), "already muted") {
		h.logger.Error("failed to mute conversation", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "internal server error"})
	}

	// Handle already muted - update existing mute (idempotent)
	return h.replaceMute(ctx, username, conversationID, mute)
}

// replaceMute replaces an existing mute
func (h *Handler) replaceMute(ctx *lift.Context, username, conversationID string, mute *storage.ConversationMute) error {
	// Delete existing mute
	if err := h.repos.Conversation().DeleteConversationMute(ctx.Context, username, conversationID); err != nil {
		h.logger.Warn("failed to delete existing conversation mute", zap.Error(err))
	}

	// Create new mute
	if err := h.repos.Conversation().CreateConversationMute(ctx.Context, mute); err != nil {
		h.logger.Error("failed to recreate conversation mute", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "internal server error"})
	}

	return nil
}

// buildMutedStatusResponse builds the response for the muted status
func (h *Handler) buildMutedStatusResponse(ctx *lift.Context, objectID, username string) error {
	// Get the object
	object, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{"error": "status not found"})
	}

	// Get actor
	actor, _ := h.repos.Actor().GetActor(ctx.Context, username)

	// Return the status with muted flag set to true
	status := transformations.ObjectToStatusAny(object, actor, h.cfg.BaseURL())
	status.Muted = true

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// HandleUnmuteConversationLift handles POST /api/v1/statuses/:id/unmute
func (h *Handler) HandleUnmuteConversationLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if err := common.ValidateRequiredParam("test_username", testUsername); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	if err := common.ValidateRequiredParam("test_username", testUsername); err == nil {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if err := common.ValidateRequiredParam("token", token); err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid token",
			})
		}
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "insufficient scope",
		})
	}

	// Get status ID from path
	statusID := ctx.Param("id")

	// Test mode fallback - extract from path
	if err := common.ValidateRequiredParam("status_id_initial", statusID); err != nil && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/unmute
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == pathComponentStatuses && parts[5] == "unmute" {
			statusID = parts[4]
		}
	}

	if err := common.ValidateRequiredParam("status_id", statusID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
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
	if err := h.repos.Conversation().DeleteConversationMute(ctx.Context, claims.Username, conversationID); err != nil {
		h.logger.Error("failed to unmute conversation", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get the object to return status information
	object, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Get actor
	actor, _ := h.repos.Actor().GetActor(ctx.Context, claims.Username)

	// Return the status with muted flag set to false
	status := transformations.ObjectToStatusAny(object, actor, h.cfg.BaseURL())
	status.Muted = false

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}
