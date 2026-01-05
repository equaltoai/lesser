package conversations

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
)

type failingPublisher struct {
	userErr         error
	conversationErr error
	events          []streaming.Event
}

func (p *failingPublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	if event != nil {
		if event.Stream == "" {
			event.Stream = fmt.Sprintf("user:%s", userID)
		}
		p.events = append(p.events, *event)
	}
	return p.userErr
}

func (p *failingPublisher) PublishToStream(_ context.Context, streamName string, event *streaming.Event) error {
	if event != nil {
		event.Stream = streamName
		p.events = append(p.events, *event)
	}
	return nil
}

func (p *failingPublisher) PublishToConversation(_ context.Context, conversationID string, event *streaming.Event) error {
	if event != nil {
		if event.Stream == "" {
			event.Stream = fmt.Sprintf("conversation:%s", conversationID)
		}
		p.events = append(p.events, *event)
	}
	return p.conversationErr
}

func (p *failingPublisher) Close() error { return nil }

func TestService_DeleteConversation_NotFoundAndRepoErrors(t *testing.T) {
	t.Parallel()

	service, conversationRepo, _, _, _, _ := createTestService()
	ctx := context.Background()

	t.Run("not found returns business error", func(t *testing.T) {
		conversationRepo.On("GetConversation", ctx, "conv-404").Return((*models.Conversation)(nil), fmt.Errorf("not found")).Once()
		_, err := service.DeleteConversation(ctx, &DeleteConversationCommand{ConversationID: "conv-404", UserID: "user-1"})
		require.ErrorIs(t, err, ErrConversationNotFound)
	})

	t.Run("unexpected repo error returns get conversation error", func(t *testing.T) {
		conversationRepo.On("GetConversation", ctx, "conv-err").Return((*models.Conversation)(nil), errors.New("boom")).Once()
		_, err := service.DeleteConversation(ctx, &DeleteConversationCommand{ConversationID: "conv-err", UserID: "user-1"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrGetConversation)
	})
}

func TestService_DeleteConversation_AllowsActorIDParticipant(t *testing.T) {
	t.Parallel()

	service, conversationRepo, _, accountRepo, publisher, _ := createTestService()
	ctx := context.Background()

	conversation := &models.Conversation{
		ID:           "conv-1",
		Participants: []string{"ap://example.com/actors/alice"},
	}

	conversationRepo.On("GetConversation", ctx, "conv-1").Return(conversation, nil).Once()
	accountRepo.On("GetAccount", ctx, "user-1").Return(&storage.Account{
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "ap://example.com/actors/alice"}},
	}, nil).Once()
	conversationRepo.On("DeleteConversation", ctx, "conv-1").Return(nil).Once()

	result, err := service.DeleteConversation(ctx, &DeleteConversationCommand{ConversationID: "conv-1", UserID: "user-1"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Events, 2)
	require.Len(t, publisher.GetEvents(), 2)
	require.Equal(t, "conversation.deleted", result.Events[0].Type)
}

func TestService_DeleteConversation_StillReturnsEventsWhenPublishFails(t *testing.T) {
	t.Parallel()

	service, conversationRepo, _, _, _, _ := createTestService()
	ctx := context.Background()

	service.publisher = &failingPublisher{
		userErr:         errors.New("user stream down"),
		conversationErr: errors.New("conversation stream down"),
	}

	conversation := &models.Conversation{
		ID:           "conv-2",
		Participants: []string{"user-1"},
	}

	conversationRepo.On("GetConversation", ctx, "conv-2").Return(conversation, nil).Once()
	conversationRepo.On("DeleteConversation", ctx, "conv-2").Return(nil).Once()

	result, err := service.DeleteConversation(ctx, &DeleteConversationCommand{ConversationID: "conv-2", UserID: "user-1"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Events, 2)
	require.Equal(t, "conversation.deleted", result.Events[0].Type)
}

func TestService_DeleteConversation_PermissionAndDeleteFailures(t *testing.T) {
	t.Parallel()

	service, conversationRepo, _, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	t.Run("account lookup failures return get account error", func(t *testing.T) {
		conversation := &models.Conversation{
			ID:           "conv-3",
			Participants: []string{"someone-else"},
		}
		conversationRepo.On("GetConversation", ctx, "conv-3").Return(conversation, nil).Once()
		accountRepo.On("GetAccount", ctx, "user-1").Return((*storage.Account)(nil), errors.New("boom")).Once()

		_, err := service.DeleteConversation(ctx, &DeleteConversationCommand{ConversationID: "conv-3", UserID: "user-1"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrGetAccount)
	})

	t.Run("missing participation returns not participant error", func(t *testing.T) {
		conversation := &models.Conversation{
			ID:           "conv-4",
			Participants: []string{"someone-else"},
		}
		conversationRepo.On("GetConversation", ctx, "conv-4").Return(conversation, nil).Once()
		accountRepo.On("GetAccount", ctx, "user-1").Return(&storage.Account{}, nil).Once()

		_, err := service.DeleteConversation(ctx, &DeleteConversationCommand{ConversationID: "conv-4", UserID: "user-1"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotConversationParticipant)
	})

	t.Run("repository delete failures return delete conversation error", func(t *testing.T) {
		conversation := &models.Conversation{
			ID:           "conv-5",
			Participants: []string{"user-1"},
		}
		conversationRepo.On("GetConversation", ctx, "conv-5").Return(conversation, nil).Once()
		conversationRepo.On("DeleteConversation", ctx, "conv-5").Return(errors.New("boom")).Once()

		_, err := service.DeleteConversation(ctx, &DeleteConversationCommand{ConversationID: "conv-5", UserID: "user-1"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrDeleteConversation)
	})
}
