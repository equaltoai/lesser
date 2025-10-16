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
func (r *queryResolver) Conversations(ctx context.Context, first *int, after *model.Cursor) ([]*model.Conversation, error) {
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

	result, err := r.Registry.Conversations().ListConversations(ctx, &conversations.ListConversationsQuery{
		UserID:     username,
		Pagination: pagination,
	})
	if err != nil {
		r.Logger.Error("Failed to list conversations",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list conversations"), err)
	}

	var convos []*model.Conversation
	if result.Conversations != nil {
		convos = make([]*model.Conversation, len(result.Conversations.Items))
		for i, conv := range result.Conversations.Items {
			convos[i] = r.convertConversationToGraphQL(ctx, conv)
		}
	}

	return convos, nil
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
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
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
