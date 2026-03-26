package conversations

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRound43_Service_SendDirectMessage_RetriesTransactionalCreateRace(t *testing.T) {
	service, conversationRepo, _, accountRepo, publisher, federation := createTestService()
	ctx := context.Background()

	senderAccount := createTestAccount("alice", "alice")
	recipientAccount := createTestAccount("bob", "bob")
	existingConversation := createTestConversation("conv-existing", []string{"alice", "bob"})

	accountRepo.On("GetAccount", ctx, "alice").Return(senderAccount, nil).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(recipientAccount, nil).Once()

	conversationRepo.On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
		Return((*models.Conversation)(nil), fmt.Errorf("not found")).
		Once()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
		return transition != nil && transition.CreateConversation
	})).
		Return(storage.ErrAlreadyExists).
		Once()

	conversationRepo.On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
		Return(existingConversation, nil).
		Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv-existing", "bob").
		Return(&models.ConversationParticipantRecord{Conversation: existingConversation}, nil).
		Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv-existing", "alice").
		Return(&models.ConversationParticipantRecord{Conversation: existingConversation}, nil).
		Once()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
		return transition != nil && !transition.CreateConversation && transition.Conversation != nil && transition.Conversation.ID == "conv-existing"
	})).
		Return(nil).
		Once()

	result, err := service.SendDirectMessage(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hello after race",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "conv-existing", result.Conversation.ID)
	require.Len(t, result.Events, 3)
	require.Len(t, publisher.GetEvents(), 3)
	require.Empty(t, federation.GetQueuedActivities())
}
