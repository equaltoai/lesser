package conversations

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestService_MarkConversationRead_ErrorPaths(t *testing.T) {
	t.Parallel()

	service, conversationRepo, _, _, _, _ := createTestService()
	ctx := context.Background()

	t.Run("get conversation error returns expected error", func(t *testing.T) {
		conversationRepo.On("GetConversation", ctx, "conv-err").Return((*models.Conversation)(nil), errors.New("boom")).Once()
		_, err := service.MarkConversationRead(ctx, &MarkConversationReadCommand{ConversationID: "conv-err", UserID: "user-1"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrGetConversation)
	})

	t.Run("mark read error returns expected error", func(t *testing.T) {
		conversation := createTestConversation("conv-1", []string{"user-1"})
		conversationRepo.On("GetConversation", ctx, "conv-1").Return(conversation, nil).Once()
		conversationRepo.On("MarkConversationRead", ctx, "conv-1", "user-1").Return(errors.New("boom")).Once()

		_, err := service.MarkConversationRead(ctx, &MarkConversationReadCommand{ConversationID: "conv-1", UserID: "user-1"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMarkConversationRead)
	})
}
