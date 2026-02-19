package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"go.uber.org/zap"
)

// CreateConversation is the resolver for the createConversation field.
func (r *mutationResolver) CreateConversation(ctx context.Context, participantID string) (*model.Conversation, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Conversations().CreateConversation(ctx, &conversations.CreateConversationCommand{
		CreatorID:     username,
		ParticipantID: participantID,
	})
	if err != nil {
		r.Logger.Error("Failed to create conversation",
			zap.String("user", username),
			zap.String("participant", participantID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create conversation"), err)
	}

	return r.convertConversationToGraphQL(ctx, result.Conversation), nil
}

// SendDirectMessage is the resolver for the sendDirectMessage field.
func (r *mutationResolver) SendDirectMessage(ctx context.Context, to string, content string, mediaIds []string) (*model.SendMessagePayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Conversations().SendDirectMessage(ctx, &conversations.SendDirectMessageCommand{
		SenderID:   username,
		Recipients: []string{to},
		Content:    content,
		MediaIDs:   mediaIds,
	})
	if err != nil {
		r.Logger.Error("Failed to send direct message",
			zap.String("user", username),
			zap.String("to", to),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to send direct message"), err)
	}

	return &model.SendMessagePayload{
		Conversation: r.convertConversationToGraphQL(ctx, result.Conversation),
		Message:      r.convertStatusToObject(ctx, result.Message),
	}, nil
}

// SendMessage is the resolver for the sendMessage field.
func (r *mutationResolver) SendMessage(ctx context.Context, conversationID string, content string, mediaIds []string) (*model.SendMessagePayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	storage := r.Registry.GetStorage()
	if storage == nil || storage.Conversation() == nil {
		return nil, errors.New("conversation repository is not available")
	}

	conversation, err := storage.Conversation().GetConversation(ctx, conversationID)
	if err != nil {
		r.Logger.Error("Failed to load conversation for sendMessage",
			zap.String("user", username),
			zap.String("conversation_id", conversationID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to load conversation"), err)
	}

	// Authorize + enforce v1 1:1 only.
	recipientID := ""
	for _, participant := range conversation.Participants {
		if participant == username {
			continue
		}
		if recipientID != "" {
			return nil, errors.New("conversation is not 1:1")
		}
		recipientID = participant
	}
	if recipientID == "" {
		return nil, ErrAccessDenied
	}

	result, err := r.Registry.Conversations().SendDirectMessage(ctx, &conversations.SendDirectMessageCommand{
		SenderID:   username,
		Recipients: []string{recipientID},
		Content:    content,
		MediaIDs:   mediaIds,
	})
	if err != nil {
		r.Logger.Error("Failed to send message",
			zap.String("user", username),
			zap.String("conversation_id", conversationID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to send message"), err)
	}

	return &model.SendMessagePayload{
		Conversation: r.convertConversationToGraphQL(ctx, result.Conversation),
		Message:      r.convertStatusToObject(ctx, result.Message),
	}, nil
}

// AcceptMessageRequest is the resolver for the acceptMessageRequest field.
func (r *mutationResolver) AcceptMessageRequest(ctx context.Context, conversationID string) (*model.Conversation, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Conversations().AcceptMessageRequest(ctx, &conversations.AcceptMessageRequestCommand{
		ConversationID: conversationID,
		UserID:         username,
	})
	if err != nil {
		r.Logger.Error("Failed to accept message request",
			zap.String("user", username),
			zap.String("conversation_id", conversationID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to accept message request"), err)
	}

	return r.convertConversationToGraphQL(ctx, result.Conversation), nil
}

// DeclineMessageRequest is the resolver for the declineMessageRequest field.
func (r *mutationResolver) DeclineMessageRequest(ctx context.Context, conversationID string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	_, err = r.Registry.Conversations().DeclineMessageRequest(ctx, &conversations.DeclineMessageRequestCommand{
		ConversationID: conversationID,
		UserID:         username,
	})
	if err != nil {
		r.Logger.Error("Failed to decline message request",
			zap.String("user", username),
			zap.String("conversation_id", conversationID),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to decline message request"), err)
	}

	return true, nil
}
