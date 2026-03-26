package conversations

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_SendMessage_DoesNotCallUpdateConversationLastStatus(t *testing.T) {
	service, conversationRepo, _, accountRepo, _, _ := createTestService()
	ctx := context.Background()
	conversation := createTestConversation("conv-last-status", []string{"alice", "bob"})

	conversationRepo.On("GetConversation", ctx, "conv-last-status").Return(conversation, nil).Once()
	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv-last-status", "bob").
		Return(&models.ConversationParticipantRecord{RequestState: models.DmRequestStateAccepted}, nil).
		Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv-last-status", "alice").
		Return(&models.ConversationParticipantRecord{RequestState: models.DmRequestStateAccepted}, nil).
		Once()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition")).
		Return(nil).
		Once()

	result, err := service.SendMessage(ctx, &SendMessageCommand{
		SenderID:       "alice",
		ConversationID: "conv-last-status",
		Content:        "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	conversationRepo.AssertNotCalled(t, "UpdateConversationLastStatus", mock.Anything, mock.Anything, mock.Anything)
}
