package handlers

import (
	"context"
	"fmt"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func (h *Handler) convertConversationToAPI(ctx context.Context, conv *storageModels.Conversation, viewerUsername string) (apimodels.Conversation, error) {
	return h.convertConversationToAPIWithPrefetch(ctx, conv, viewerUsername, nil)
}

func (h *Handler) convertConversationToAPIWithPrefetch(ctx context.Context, conv *storageModels.Conversation, viewerUsername string, prefetch *conversationAPIPrefetch) (apimodels.Conversation, error) {
	if conv == nil {
		return apimodels.Conversation{}, nil
	}

	apiConversation := apimodels.Conversation{
		ID:     conv.ID,
		Unread: conv.Unread,
	}

	apiConversation.Accounts = h.conversationAPIAccounts(ctx, conv, viewerUsername, prefetch)

	if status := h.conversationAPIStatus(ctx, conv, prefetch); status != nil {
		apiStatus, err := h.convertStorageStatusToAPIWithContext(conversationAPIStatusContext(h, prefetch), status, viewerUsername)
		if err == nil {
			apiConversation.LastStatus = apiStatus
		}
	}

	return apiConversation, nil
}

// HandleGetConversationsLift retrieves all conversations for the authenticated user
func (h *Handler) HandleGetConversationsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Parse query parameters
	limit := h.parseConversationLimit(ctx)
	maxID := queryValue(ctx, "max_id")

	// Build pagination options
	pagination := interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: maxID,
	}

	// Call service
	result, err := h.registry.Conversations().ListConversations(ctx.Context(), &conversations.ListConversationsQuery{
		UserID:     username,
		Pagination: pagination,
	})
	if err != nil {
		h.logger.Error("failed to list conversations", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	response := make([]apimodels.Conversation, 0, len(result.Conversations.Items))
	prefetch := h.loadConversationAPIPrefetch(ctx.Context(), result.Conversations.Items, username)
	for _, conv := range result.Conversations.Items {
		apiConv, err := h.convertConversationToAPIWithPrefetch(ctx.Context(), conv, username, prefetch)
		if err != nil {
			continue
		}
		response = append(response, apiConv)
	}

	resp, err := okJSON(response)
	if err != nil {
		return nil, err
	}

	// Set pagination header if there are more results
	if result.Conversations.HasMore && result.Conversations.NextCursor != "" {
		h.setConversationPaginationHeader(resp, result.Conversations.NextCursor, limit)
	}

	return resp, nil
}

// parseConversationLimit parses the limit query parameter
func (h *Handler) parseConversationLimit(ctx *apptheory.Context) int {
	limitStr := queryValue(ctx, "limit")
	limit, err := common.ParseTimelineLimit(limitStr)
	if err != nil {
		limit = 20
	}
	return limit
}

// setConversationPaginationHeader sets the Link header for pagination
func (h *Handler) setConversationPaginationHeader(resp *apptheory.Response, cursor string, limit int) {
	if err := common.ValidateRequiredParam("cursor", cursor); err != nil {
		return
	}

	nextURL := fmt.Sprintf("%s/api/v1/conversations?max_id=%s", h.cfg.BaseURL(), cursor)
	if limit != 20 {
		nextURL += fmt.Sprintf("&limit=%d", limit)
	}
	setHeader(resp, "Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
}

// HandleDeleteConversationLift removes a conversation from the user's list
func (h *Handler) HandleDeleteConversationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	conversationID := ctx.Param("id")
	if err := common.ValidateRequiredParam("conversation_id", conversationID); err != nil {
		return common.RespondBadRequest(ctx, "missing conversation id")
	}

	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Call service to delete conversation
	_, err = h.registry.Conversations().DeleteConversation(ctx.Context(), &conversations.DeleteConversationCommand{
		ConversationID: conversationID,
		UserID:         username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "conversation not found")
		}
		if strings.Contains(err.Error(), "not a participant") {
			return common.RespondNotFound(ctx, "conversation not found")
		}
		h.logger.Error("failed to delete conversation", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(apimodels.EmptyObject{})
}

// HandleMarkConversationReadLift marks a conversation as read
func (h *Handler) HandleMarkConversationReadLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	conversationID := ctx.Param("id")
	if err := common.ValidateRequiredParam("conversation_id", conversationID); err != nil {
		return common.RespondBadRequest(ctx, "missing conversation id")
	}

	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Call service
	result, err := h.registry.Conversations().MarkConversationRead(ctx.Context(), &conversations.MarkConversationReadCommand{
		ConversationID: conversationID,
		UserID:         username,
	})
	if err != nil {
		h.logger.Error("failed to mark conversation as read", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	apiConv, err := h.convertConversationToAPI(ctx.Context(), result.Conversation, username)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	return okJSON(apiConv)
}
