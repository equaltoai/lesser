package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// Conversations is the resolver for the conversations field.
func (r *queryResolver) Conversations(ctx context.Context, folder *model.ConversationFolder, first *int, after *model.Cursor) ([]*model.Conversation, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	pagination := interfaces.PaginationOptions{
		Limit: 20,
	}
	if first != nil && *first > 0 && *first <= 100 {
		pagination.Limit = *first
	}
	if after != nil {
		pagination.Cursor = string(*after)
	}

	folderArg := conversations.ConversationFolderInbox
	if folder != nil && *folder == model.ConversationFolderRequests {
		folderArg = conversations.ConversationFolderRequests
	}

	result, err := r.Registry.Conversations().ListConversations(ctx, &conversations.ListConversationsQuery{
		UserID:     username,
		Pagination: pagination,
		Folder:     folderArg,
	})
	if err != nil {
		r.Logger.Error("Failed to list conversations",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list conversations"), err)
	}

	var convos []*model.Conversation
	if result.Conversations != nil {
		prefetch := r.loadConversationListPrefetch(ctx, username, result.Conversations.Items)
		convos = make([]*model.Conversation, len(result.Conversations.Items))
		for i, conv := range result.Conversations.Items {
			convos[i] = r.convertConversationListToGraphQL(ctx, conv, prefetch)
		}
	}

	return convos, nil
}

// ConversationConnection is the resolver for the conversationConnection field.
func (r *queryResolver) ConversationConnection(ctx context.Context, folder *model.ConversationFolder, first *int, after *model.Cursor) (*model.ConversationConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	pagination := interfaces.PaginationOptions{Limit: 20}
	if first != nil && *first > 0 && *first <= 100 {
		pagination.Limit = *first
	}
	if after != nil {
		pagination.Cursor = string(*after)
	}

	folderArg := conversations.ConversationFolderInbox
	if folder != nil && *folder == model.ConversationFolderRequests {
		folderArg = conversations.ConversationFolderRequests
	}

	result, err := r.Registry.Conversations().ListConversations(ctx, &conversations.ListConversationsQuery{
		UserID: username, Pagination: pagination, Folder: folderArg,
	})
	if err != nil {
		return nil, errors.Join(errors.New("failed to list conversations"), err)
	}

	edges := make([]*model.ConversationEdge, 0)
	if result != nil && result.Conversations != nil {
		prefetch := r.loadConversationListPrefetch(ctx, username, result.Conversations.Items)
		for _, conv := range result.Conversations.Items {
			node := r.convertConversationListToGraphQL(ctx, conv, prefetch)
			if node == nil {
				continue
			}
			cursor := model.Cursor(node.ID)
			if node.Cursor != nil && *node.Cursor != "" {
				cursor = *node.Cursor
			}
			edges = append(edges, &model.ConversationEdge{Node: node, Cursor: cursor})
		}
	}

	pageInfo := &model.PageInfo{HasPreviousPage: after != nil}
	if result != nil && result.Conversations != nil {
		pageInfo.HasNextPage = result.Conversations.HasMore
	}
	if len(edges) > 0 {
		start, end := edges[0].Cursor, edges[len(edges)-1].Cursor
		pageInfo.StartCursor, pageInfo.EndCursor = &start, &end
	} else if result != nil && result.Conversations != nil && result.Conversations.NextCursor != "" {
		end := model.Cursor(result.Conversations.NextCursor)
		pageInfo.EndCursor = &end
	}

	return &model.ConversationConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// Conversation is the resolver for the conversation field.
func (r *queryResolver) Conversation(ctx context.Context, id string) (*model.Conversation, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Conversations().GetConversation(ctx, &conversations.GetConversationQuery{
		ViewerID:       username,
		ConversationID: id,
	})
	if err != nil {
		if conversationLookupMustHideExistence(err) {
			return nil, ErrAccessDenied
		}
		r.Logger.Error("Failed to get conversation",
			zap.String("user", username),
			zap.String("id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get conversation"), err)
	}

	isParticipant := false
	for _, participantID := range result.Conversation.Participants {
		if participantID == username {
			isParticipant = true
			break
		}
	}
	if !isParticipant {
		return nil, ErrAccessDenied
	}

	return r.convertConversationToGraphQL(ctx, result.Conversation), nil
}

func conversationLookupMustHideExistence(err error) bool {
	return err != nil && (strings.Contains(strings.ToLower(err.Error()), "not found") ||
		errors.Is(err, conversations.ErrNotConversationParticipant))
}

// ConversationMessages is the resolver for the conversationMessages field.
func (r *queryResolver) ConversationMessages(ctx context.Context, conversationID string, first *int, after *model.Cursor) (*model.ObjectConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	pagination := interfaces.PaginationOptions{
		Limit: 50,
	}
	if first != nil && *first > 0 && *first <= 200 {
		pagination.Limit = *first
	}
	if after != nil {
		pagination.Cursor = string(*after)
	}

	result, err := r.Registry.Conversations().GetConversation(ctx, &conversations.GetConversationQuery{
		ViewerID:       username,
		ConversationID: conversationID,
		Pagination:     pagination,
	})
	if err != nil {
		if conversationLookupMustHideExistence(err) {
			return nil, nil
		}
		r.Logger.Error("Failed to get conversation messages",
			zap.String("user", username),
			zap.String("conversation_id", conversationID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get conversation messages"), err)
	}
	if result == nil {
		return nil, nil
	}
	r.prefetchQuoteControls(ctx, result.Messages.Items)

	edges := make([]*model.ObjectEdge, 0, len(result.Messages.Items))
	for _, status := range result.Messages.Items {
		if status == nil {
			continue
		}
		obj := r.convertStatusToObject(ctx, status)
		if obj == nil {
			continue
		}

		cursor := strings.TrimSpace(status.GSI3SK)
		if cursor == "" {
			cursor = status.StatusID
		}
		edges = append(edges, &model.ObjectEdge{
			Node:   obj,
			Cursor: model.Cursor(cursor),
		})
	}

	pageInfo := &model.PageInfo{
		HasNextPage:     result.Messages.HasMore,
		HasPreviousPage: false,
	}
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &start
		pageInfo.EndCursor = &end
	}

	return &model.ObjectConnection{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: len(edges),
	}, nil
}
