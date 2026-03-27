package conversations

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_SendDirectMessage_DoesNotRunLegacyFollowUpSyncs(t *testing.T) {
	service, conversationRepo, _, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	senderAccount := createTestAccount("alice", "alice")
	recipientAccount := createTestAccount("bob", "bob")

	accountRepo.On("GetAccount", ctx, "alice").Return(senderAccount, nil).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(recipientAccount, nil).Once()

	conversationRepo.On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
		Return((*models.Conversation)(nil), fmt.Errorf("not found")).
		Once()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition"), mock.Anything).
		Return(nil).
		Once()

	result, err := service.SendDirectMessage(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	conversationRepo.AssertNotCalled(t, "UpdateConversationParticipantRecord", mock.Anything, mock.Anything)
	conversationRepo.AssertNotCalled(t, "MarkConversationRead", mock.Anything, mock.Anything, mock.Anything)
	conversationRepo.AssertNotCalled(t, "MarkConversationUnread", mock.Anything, mock.Anything, mock.Anything)
}
