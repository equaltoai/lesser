package lift

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetConversationsLift retrieves all conversations for the authenticated user
func (h *Handler) HandleGetConversationsLift(ctx *lift.Context) error {
	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Parse query parameters
	limit := h.parseConversationLimit(ctx)
	maxID := ctx.Query("max_id")

	// Build pagination options
	pagination := interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: maxID,
	}

	// Call service
	result, err := h.registry.Conversations().ListConversations(ctx.Context, &conversations.ListConversationsQuery{
		UserID:     claims.Username,
		Pagination: pagination,
	})
	if err != nil {
		h.logger.Error("failed to list conversations", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Set pagination header if there are more results
	if result.Conversations.HasMore && result.Conversations.NextCursor != "" {
		h.setConversationPaginationHeader(ctx, result.Conversations.NextCursor, limit)
	}

	return ctx.JSON(result.Conversations.Items)
}

// parseConversationLimit parses the limit query parameter
func (h *Handler) parseConversationLimit(ctx *lift.Context) int {
	limit := 20
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}
	return limit
}

// setConversationPaginationHeader sets the Link header for pagination
func (h *Handler) setConversationPaginationHeader(ctx *lift.Context, cursor string, limit int) {
	if cursor == "" {
		return
	}

	nextURL := fmt.Sprintf("%s/api/v1/conversations?max_id=%s", h.cfg.BaseURL(), cursor)
	if limit != 20 {
		nextURL += fmt.Sprintf("&limit=%d", limit)
	}
	ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
}

// HandleDeleteConversationLift removes a conversation from the user's list
func (h *Handler) HandleDeleteConversationLift(ctx *lift.Context) error {
	conversationID := ctx.Param("id")
	if conversationID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing conversation id"})
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call service to delete conversation
	_, err = h.registry.Conversations().DeleteConversation(ctx.Context, &conversations.DeleteConversationCommand{
		ConversationID: conversationID,
		UserID:         claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(404).JSON(map[string]string{"error": "conversation not found"})
		}
		if strings.Contains(err.Error(), "not a participant") {
			return ctx.Status(404).JSON(map[string]string{"error": "conversation not found"})
		}
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

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call service
	result, err := h.registry.Conversations().MarkConversationRead(ctx.Context, &conversations.MarkConversationReadCommand{
		ConversationID: conversationID,
		UserID:         claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to mark conversation as read", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	return ctx.JSON(result.Conversation)
}
