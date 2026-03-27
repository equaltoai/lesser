package conversations

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_SendMessage_DoesNotCallAddStatusToConversation(t *testing.T) {
	service, conversationRepo, _, accountRepo, _, _ := createTestService()
	ctx := context.Background()
	conversation := createTestConversation("conv-add-status", []string{"alice", "bob"})

	conversationRepo.On("GetConversation", ctx, "conv-add-status").Return(conversation, nil).Once()
	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()
	conversationRepo.On("GetUserConversationState", ctx, "bob", "conv-add-status").
		Return(testConversationStateContract("bob", "conv-add-status", func(state *interfaces.UserConversationStateContract) {
			state.RequestState = models.DmRequestStateAccepted
		}), nil).
		Once()
	conversationRepo.On("GetUserConversationState", ctx, "alice", "conv-add-status").
		Return(testConversationStateContract("alice", "conv-add-status", func(state *interfaces.UserConversationStateContract) {
			state.RequestState = models.DmRequestStateAccepted
		}), nil).
		Once()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition")).
		Return(nil).
		Once()

	result, err := service.SendMessage(ctx, &SendMessageCommand{
		SenderID:       "alice",
		ConversationID: "conv-add-status",
		Content:        "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	conversationRepo.AssertNotCalled(t, "AddStatusToConversation", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
