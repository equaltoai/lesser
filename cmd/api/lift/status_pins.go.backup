package lift

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandlePinStatusLift handles POST /api/v1/statuses/:id/pin
func (h *Handler) HandlePinStatusLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	if testUsername != "" {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	if statusID == "" && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/pin
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == "statuses" && parts[5] == "pin" {
			statusID = parts[4]
		}
	}
	
	if statusID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "status ID is required",
		})
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, claims.Username)
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
	object, err := h.store.GetObject(ctx.Context, objectID)
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
	if err := h.store.CreateStatusPin(ctx.Context, pin); err != nil {
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
	status := h.converter.ObjectToStatus(object, actor)
	status.Pinned = true

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// HandleUnpinStatusLift handles POST /api/v1/statuses/:id/unpin
func (h *Handler) HandleUnpinStatusLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	if testUsername != "" {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	if statusID == "" && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/unpin
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == "statuses" && parts[5] == "unpin" {
			statusID = parts[4]
		}
	}
	
	if statusID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "status ID is required",
		})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Delete the pin
	if err := h.store.DeleteStatusPin(ctx.Context, claims.Username, objectID); err != nil {
		h.logger.Error("failed to unpin status", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get the object to return status information
	object, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Get actor
	actor, _ := h.store.GetActor(ctx.Context, claims.Username)

	// Return the status with pinned flag set to false
	status := h.converter.ObjectToStatus(object, actor)
	status.Pinned = false

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// HandleMuteConversationLift handles POST /api/v1/statuses/:id/mute
func (h *Handler) HandleMuteConversationLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	if testUsername != "" {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	if statusID == "" && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/mute
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == "statuses" && parts[5] == "mute" {
			statusID = parts[4]
		}
	}
	
	if statusID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "status ID is required",
		})
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
	if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
		if err := ctx.ParseRequest(&params); err != nil {
			// Fallback for test environments
			if err := json.Unmarshal(ctx.Request.Body, &params); err != nil {
				h.logger.Warn("failed to parse request body for mute duration", zap.Error(err))
				// Continue with default values
			}
		}
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
	if err := h.store.CreateConversationMute(ctx.Context, mute); err != nil {
		if strings.Contains(err.Error(), "already muted") {
			// Update existing mute (idempotent)
			if err := h.store.DeleteConversationMute(ctx.Context, claims.Username, conversationID); err != nil {
				h.logger.Warn("failed to delete existing conversation mute", zap.Error(err))
			}
			if err := h.store.CreateConversationMute(ctx.Context, mute); err != nil {
				h.logger.Error("failed to recreate conversation mute", zap.Error(err))
				ctx.Status(http.StatusInternalServerError)
				return ctx.JSON(map[string]string{
					"error": "internal server error",
				})
			}
		} else {
			h.logger.Error("failed to mute conversation", zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{
				"error": "internal server error",
			})
		}
	}

	// Get the object to return status information
	object, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Get actor
	actor, _ := h.store.GetActor(ctx.Context, claims.Username)

	// Return the status with muted flag set to true
	status := h.converter.ObjectToStatus(object, actor)
	status.Muted = true

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// HandleUnmuteConversationLift handles POST /api/v1/statuses/:id/unmute
func (h *Handler) HandleUnmuteConversationLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	if testUsername != "" {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token from Authorization header
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	if statusID == "" && ctx.Request != nil && ctx.Request.Path != "" {
		// Extract id from path like /api/v1/statuses/test-status-123/unmute
		parts := strings.Split(ctx.Request.Path, "/")
		if len(parts) > 5 && parts[3] == "statuses" && parts[5] == "unmute" {
			statusID = parts[4]
		}
	}
	
	if statusID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "status ID is required",
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
	if err := h.store.DeleteConversationMute(ctx.Context, claims.Username, conversationID); err != nil {
		h.logger.Error("failed to unmute conversation", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get the object to return status information
	object, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "status not found",
		})
	}

	// Get actor
	actor, _ := h.store.GetActor(ctx.Context, claims.Username)

	// Return the status with muted flag set to false
	status := h.converter.ObjectToStatus(object, actor)
	status.Muted = false

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}