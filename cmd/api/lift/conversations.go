package lift

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
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

func (h *Handler) convertConversationToAPI(ctx context.Context, conv *storageModels.Conversation, viewerUsername string) (apimodels.Conversation, error) {
	if conv == nil {
		return apimodels.Conversation{}, nil
	}

	apiConversation := apimodels.Conversation{
		ID:     conv.ID,
		Unread: conv.Unread,
	}

	accounts := make([]apimodels.Account, 0, len(conv.Participants))
	for _, participant := range conv.Participants {
		if participant == "" || participant == viewerUsername {
			continue
		}

		actor, err := h.repos.Actor().GetActor(ctx, participant)
		if err != nil || actor == nil {
			continue
		}

		account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
		accounts = append(accounts, account)
	}
	apiConversation.Accounts = accounts

	if conv.LastStatusID != "" {
		status, err := h.repos.Status().GetStatus(ctx, conv.LastStatusID)
		if err == nil && status != nil {
			apiStatus, err := h.convertStorageStatusToAPI(status, viewerUsername)
			if err == nil {
				apiConversation.LastStatus = apiStatus
			}
		}
	}

	return apiConversation, nil
}

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
		return common.RespondInternalServerError(ctx)
	}

	// Set pagination header if there are more results
	if result.Conversations.HasMore && result.Conversations.NextCursor != "" {
		h.setConversationPaginationHeader(ctx, result.Conversations.NextCursor, limit)
	}

	response := make([]apimodels.Conversation, 0, len(result.Conversations.Items))
	for _, conv := range result.Conversations.Items {
		apiConv, err := h.convertConversationToAPI(ctx.Context, conv, claims.Username)
		if err != nil {
			continue
		}
		response = append(response, apiConv)
	}

	return ctx.JSON(response)
}

// parseConversationLimit parses the limit query parameter
func (h *Handler) parseConversationLimit(ctx *lift.Context) int {
	limitStr := ctx.Query("limit")
	limit, err := common.ParseTimelineLimit(limitStr)
	if err != nil {
		limit = 20
	}
	return limit
}

// setConversationPaginationHeader sets the Link header for pagination
func (h *Handler) setConversationPaginationHeader(ctx *lift.Context, cursor string, limit int) {
	if err := common.ValidateRequiredParam("cursor", cursor); err != nil {
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
	if err := common.ValidateRequiredParam("conversation_id", conversationID); err != nil {
		return common.RespondBadRequest(ctx, "missing conversation id")
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
			return common.RespondNotFound(ctx, "conversation not found")
		}
		if strings.Contains(err.Error(), "not a participant") {
			return common.RespondNotFound(ctx, "conversation not found")
		}
		h.logger.Error("failed to delete conversation", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return ctx.Status(200).JSON(apimodels.EmptyObject{})
}

// HandleMarkConversationReadLift marks a conversation as read
func (h *Handler) HandleMarkConversationReadLift(ctx *lift.Context) error {
	conversationID := ctx.Param("id")
	if err := common.ValidateRequiredParam("conversation_id", conversationID); err != nil {
		return common.RespondBadRequest(ctx, "missing conversation id")
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
		return common.RespondInternalServerError(ctx)
	}

	apiConv, err := h.convertConversationToAPI(ctx.Context, result.Conversation, claims.Username)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	return ctx.JSON(apiConv)
}
