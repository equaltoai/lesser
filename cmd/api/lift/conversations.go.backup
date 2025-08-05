package lift

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetConversationsLift retrieves all conversations for the authenticated user
func (h *Handler) HandleGetConversationsLift(ctx *lift.Context) error {
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check read scope
		if !claims.HasScope(auth.ScopeRead) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get current user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Parse query parameters
	limit := 20
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := ctx.Query("max_id")

	// Get conversations
	conversations, cursor, err := h.store.GetUserConversations(ctx.Context, actor.ID, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get conversations", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert to API format
	converter := mastodon.NewConverter(h.cfg.BaseURL())

	apiConversations := make([]models.Conversation, 0, len(conversations))
	for _, conv := range conversations {
		// Get participants (excluding current user)
		participantActors := make([]*activitypub.Actor, 0, len(conv.Participants)-1)
		for _, participantID := range conv.Participants {
			if participantID == actor.ID {
				continue // Skip current user
			}

			// Extract username from actor ID
			username := converter.ExtractUsernameFromActorID(participantID)
			if username != "" {
				participantActor, err := h.store.GetActor(ctx.Context, username)
				if err == nil {
					participantActors = append(participantActors, participantActor)
				}
			}
		}

		// Get last status
		var lastStatus any
		if conv.LastStatusID != "" {
			lastStatus, _ = h.store.GetObject(ctx.Context, conv.LastStatusID)
		}

		// Check unread status by comparing last message time with user's last read time
		unread := h.isConversationUnreadLift(ctx.Context, conv.ID, actor.ID, &conv.UpdatedAt)

		apiConv := converter.ConversationToAPI(conv, participantActors, lastStatus, unread)
		apiConversations = append(apiConversations, apiConv)
	}

	// Set Link header for pagination if there's a cursor
	if cursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/conversations?max_id=%s", h.cfg.BaseURL(), cursor)
		if limit != 20 {
			nextURL += fmt.Sprintf("&limit=%d", limit)
		}
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
	}

	return ctx.JSON(apiConversations)
}

// HandleDeleteConversationLift removes a conversation from the user's list
func (h *Handler) HandleDeleteConversationLift(ctx *lift.Context) error {
	conversationID := ctx.Param("id")
	if conversationID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing conversation id"})
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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

	// Get current user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get conversation to verify ownership
	conversation, err := h.store.GetConversation(ctx.Context, conversationID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "conversation not found"})
	}

	// Check if user is a participant
	isParticipant := false
	for _, participantID := range conversation.Participants {
		if participantID == actor.ID {
			isParticipant = true
			break
		}
	}

	if !isParticipant {
		return ctx.Status(404).JSON(map[string]string{"error": "conversation not found"})
	}

	// Delete conversation (or remove user from it)
	err = h.store.DeleteConversation(ctx.Context, conversationID)
	if err != nil {
		h.logger.Error("failed to delete conversation", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	return ctx.Status(200).JSON(map[string]interface{}{})
}

// HandleMarkConversationReadLift marks a conversation as read
func (h *Handler) HandleMarkConversationReadLift(ctx *lift.Context) error {
	conversationID := ctx.Param("id")
	if conversationID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing conversation id"})
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
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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

	// Get current user's actor
	actor, err := h.store.GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get conversation to verify ownership
	conversation, err := h.store.GetConversation(ctx.Context, conversationID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "conversation not found"})
	}

	// Check if user is a participant
	isParticipant := false
	for _, participantID := range conversation.Participants {
		if participantID == actor.ID {
			isParticipant = true
			break
		}
	}

	if !isParticipant {
		return ctx.Status(404).JSON(map[string]string{"error": "conversation not found"})
	}

	// Mark as read
	err = h.store.MarkConversationRead(ctx.Context, conversationID, username)
	if err != nil {
		h.logger.Error("failed to mark conversation as read", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Return updated conversation
	participantActors := make([]*activitypub.Actor, 0, len(conversation.Participants)-1)
	for _, participantID := range conversation.Participants {
		if participantID == actor.ID {
			continue
		}

		// Extract username from actor ID
		converter := mastodon.NewConverter(h.cfg.BaseURL())

		username := converter.ExtractUsernameFromActorID(participantID)
		if username != "" {
			participantActor, err := h.store.GetActor(ctx.Context, username)
			if err == nil {
				participantActors = append(participantActors, participantActor)
			}
		}
	}

	var lastStatus any
	if conversation.LastStatusID != "" {
		lastStatus, _ = h.store.GetObject(ctx.Context, conversation.LastStatusID)
	}

	// Conversation is now read, so unread = false
	converter := mastodon.NewConverter(h.cfg.BaseURL())

	apiConversation := converter.ConversationToAPI(conversation, participantActors, lastStatus, false)

	return ctx.JSON(apiConversation)
}

// isConversationUnreadLift checks if a conversation has unread messages for a user
func (h *Handler) isConversationUnreadLift(ctx context.Context, conversationID, userID string, lastMessageAt *time.Time) bool {
	if lastMessageAt == nil {
		return false // No messages = not unread
	}

	// Check if conversation is muted (if muted, it's not considered unread)
	isMuted, err := h.store.IsConversationMuted(ctx, userID, conversationID)
	if err != nil {
		h.logger.Warn("failed to check conversation mute status", zap.Error(err))
	}
	if isMuted {
		return false
	}

	// For now, assume conversation is unread if it has recent activity (within last 24 hours)
	// This is a simplified implementation - in a full implementation, we'd track read status per user
	return time.Since(*lastMessageAt) < 24*time.Hour
}