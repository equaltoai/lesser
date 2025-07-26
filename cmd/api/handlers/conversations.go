package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetConversations retrieves all conversations for the authenticated user
func (h *Handler) HandleGetConversations(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check read:statuses scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get current user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := request.QueryStringParameters["max_id"]

	// Get conversations
	conversations, cursor, err := h.store.GetUserConversations(ctx, actor.ID, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get conversations", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to API format
	apiConversations := make([]models.Conversation, 0, len(conversations))
	for _, conv := range conversations {
		// Get participants (excluding current user)
		participantActors := make([]*activitypub.Actor, 0, len(conv.Participants)-1)
		for _, participantID := range conv.Participants {
			if participantID == actor.ID {
				continue // Skip current user
			}

			// Extract username from actor ID
			username := h.converter.ExtractUsernameFromActorID(participantID)
			if username != "" {
				participantActor, err := h.store.GetActor(ctx, username)
				if err == nil {
					participantActors = append(participantActors, participantActor)
				}
			}
		}

		// Get last status
		var lastStatus any
		if conv.LastStatusID != "" {
			lastStatus, _ = h.store.GetObject(ctx, conv.LastStatusID)
		}

		// Check unread status by comparing last message time with user's last read time
		unread := h.isConversationUnread(ctx, conv.ID, actor.ID, &conv.UpdatedAt)

		apiConv := h.converter.ConversationToAPI(conv, participantActors, lastStatus, unread)
		apiConversations = append(apiConversations, apiConv)
	}

	// Set Link header for pagination if there's a cursor
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if cursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/conversations?max_id=%s", h.cfg.BaseURL(), cursor)
		if limit != 20 {
			nextURL += fmt.Sprintf("&limit=%d", limit)
		}
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	body, _ := json.Marshal(apiConversations)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleDeleteConversation removes a conversation from the user's list
func (h *Handler) HandleDeleteConversation(ctx context.Context, request events.APIGatewayV2HTTPRequest, conversationID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write scope (conversations are part of write scope)
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get current user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get conversation to verify ownership
	conversation, err := h.store.GetConversation(ctx, conversationID)
	if err != nil {
		return common.NotFound(errors.New("conversation not found")), nil
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
		return common.NotFound(errors.New("conversation not found")), nil
	}

	// Delete conversation (or remove user from it)
	err = h.store.DeleteConversation(ctx, conversationID)
	if err != nil {
		h.logger.Error("failed to delete conversation", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}, nil
}

// HandleMarkConversationRead marks a conversation as read
func (h *Handler) HandleMarkConversationRead(ctx context.Context, request events.APIGatewayV2HTTPRequest, conversationID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write scope (conversations are part of write scope)
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get current user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get conversation to verify ownership
	conversation, err := h.store.GetConversation(ctx, conversationID)
	if err != nil {
		return common.NotFound(errors.New("conversation not found")), nil
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
		return common.NotFound(errors.New("conversation not found")), nil
	}

	// Mark as read
	err = h.store.MarkConversationRead(ctx, conversationID, claims.Username)
	if err != nil {
		h.logger.Error("failed to mark conversation as read", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return updated conversation
	participantActors := make([]*activitypub.Actor, 0, len(conversation.Participants)-1)
	for _, participantID := range conversation.Participants {
		if participantID == actor.ID {
			continue
		}

		username := h.converter.ExtractUsernameFromActorID(participantID)
		if username != "" {
			participantActor, err := h.store.GetActor(ctx, username)
			if err == nil {
				participantActors = append(participantActors, participantActor)
			}
		}
	}

	var lastStatus any
	if conversation.LastStatusID != "" {
		lastStatus, _ = h.store.GetObject(ctx, conversation.LastStatusID)
	}

	apiConversation := h.converter.ConversationToAPI(conversation, participantActors, lastStatus, false)

	body, _ := json.Marshal(apiConversation)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}

// isConversationUnread checks if a conversation has unread messages for a user
func (h *Handler) isConversationUnread(ctx context.Context, conversationID, userID string, lastMessageAt *time.Time) bool {
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
