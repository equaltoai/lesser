package conversations

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestService_DMWriteMutationsRespectMigrationFreeze(t *testing.T) {
	ctx := context.Background()
	repo := &mockConversationRepository{directMessageWritesFrozen: true}
	service := NewService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	t.Run("create conversation", func(t *testing.T) {
		result, err := service.CreateConversation(ctx, &CreateConversationCommand{
			CreatorID:     "alice",
			ParticipantID: "bob",
		})
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrDirectMessageWritesFrozen)
	})

	t.Run("send direct message", func(t *testing.T) {
		result, err := service.SendDirectMessage(ctx, &SendDirectMessageCommand{
			SenderID:   "alice",
			Recipients: []string{"bob"},
			Content:    "hello",
		})
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrDirectMessageWritesFrozen)
	})

	t.Run("send message", func(t *testing.T) {
		result, err := service.SendMessage(ctx, &SendMessageCommand{
			SenderID:       "alice",
			ConversationID: "conv-1",
			Content:        "hello",
		})
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrDirectMessageWritesFrozen)
	})

	t.Run("accept request", func(t *testing.T) {
		result, err := service.AcceptMessageRequest(ctx, &AcceptMessageRequestCommand{
			ConversationID: "conv-1",
			UserID:         "alice",
		})
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrDirectMessageWritesFrozen)
	})

	t.Run("decline request", func(t *testing.T) {
		result, err := service.DeclineMessageRequest(ctx, &DeclineMessageRequestCommand{
			ConversationID: "conv-1",
			UserID:         "alice",
		})
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrDirectMessageWritesFrozen)
	})

	t.Run("mark read", func(t *testing.T) {
		result, err := service.MarkConversationRead(ctx, &MarkConversationReadCommand{
			ConversationID: "conv-1",
			UserID:         "alice",
		})
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrDirectMessageWritesFrozen)
	})

	t.Run("delete conversation", func(t *testing.T) {
		result, err := service.DeleteConversation(ctx, &DeleteConversationCommand{
			ConversationID: "conv-1",
			UserID:         "alice",
		})
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrDirectMessageWritesFrozen)
	})

	t.Run("delete message", func(t *testing.T) {
		deleted, err := service.DeleteMessage(ctx, &DeleteMessageCommand{
			MessageID: "status-1",
			UserID:    "alice",
		})
		require.False(t, deleted)
		require.ErrorIs(t, err, ErrDirectMessageWritesFrozen)
	})
}

func TestService_EnsureDirectMessageWritesAllowed_UsesOptionalFreezeChecker(t *testing.T) {
	ctx := context.Background()
	service := NewService(&mockConversationRepository{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	require.NoError(t, service.ensureDirectMessageWritesAllowed(ctx))

	service = NewService(&mockConversationRepository{directMessageWritesFrozenErr: models.ErrConversationDataRequired}, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	require.ErrorIs(t, service.ensureDirectMessageWritesAllowed(ctx), models.ErrConversationDataRequired)
}
