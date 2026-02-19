package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// MarkConversationAsRead is the resolver for the markConversationAsRead field.
func (r *mutationResolver) MarkConversationAsRead(ctx context.Context, id string) (*model.Conversation, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Conversations().MarkConversationRead(ctx, &conversations.MarkConversationReadCommand{
		UserID:         username,
		ConversationID: id,
	})
	if err != nil {
		r.Logger.Error("Failed to mark conversation as read",
			zap.String("user", username),
			zap.String("conversation", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to mark as read"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertConversationToGraphQL(ctx, result.Conversation), nil
}

// DeleteConversation is the resolver for the deleteConversation field.
func (r *mutationResolver) DeleteConversation(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	_, err = r.Registry.Conversations().DeleteConversation(ctx, &conversations.DeleteConversationCommand{
		UserID:         username,
		ConversationID: id,
	})
	if err != nil {
		r.Logger.Error("Failed to delete conversation",
			zap.String("user", username),
			zap.String("conversation", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to delete conversation"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

// DeleteMessage is the resolver for the deleteMessage field.
func (r *mutationResolver) DeleteMessage(ctx context.Context, messageID string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	ok, err := r.Registry.Conversations().DeleteMessage(ctx, &conversations.DeleteMessageCommand{
		MessageID: messageID,
		UserID:    username,
	})
	if err != nil {
		r.Logger.Error("Failed to delete message",
			zap.String("user", username),
			zap.String("message", messageID),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to delete message"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return ok, nil
}
