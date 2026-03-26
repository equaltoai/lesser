package conversations

import (
	"context"
	"fmt"
	"testing"
	"time"

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

func TestRound43_Service_SendMessage_RetriesTransactionalVersionConflict(t *testing.T) {
	service, conversationRepo, _, accountRepo, publisher, federation := createTestService()
	ctx := context.Background()

	initialConversation := createTestConversation("conv-existing", []string{"alice", "bob"})
	initialConversation.TotalMessageCount = 5
	initialConversation.UpdatedAt = time.Date(2026, 3, 26, 17, 0, 0, 0, time.UTC)

	reloadedConversation := createTestConversation("conv-existing", []string{"alice", "bob"})
	reloadedConversation.TotalMessageCount = 6
	reloadedConversation.UpdatedAt = time.Date(2026, 3, 26, 17, 1, 0, 0, time.UTC)
	reloadedConversation.LastStatusID = "status-winner"

	senderAccount := createTestAccount("alice", "alice")
	recipientAccount := createTestAccount("bob", "bob")
	record := &models.ConversationParticipantRecord{
		RequestState:             models.DmRequestStateAccepted,
		PreviewStatusID:          "status-winner",
		PreviewStatusPublishedAt: reloadedConversation.UpdatedAt,
		SortAt:                   reloadedConversation.UpdatedAt,
		Conversation:             reloadedConversation,
	}

	conversationRepo.On("GetConversation", ctx, "conv-existing").Return(initialConversation, nil).Once()
	accountRepo.On("GetAccount", ctx, "alice").Return(senderAccount, nil).Twice()
	accountRepo.On("GetAccount", ctx, "bob").Return(recipientAccount, nil).Twice()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv-existing", "bob").Return(record, nil).Twice()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv-existing", "alice").Return(record, nil).Twice()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
		return transition != nil && transition.Conversation != nil && transition.Conversation.ID == "conv-existing"
	})).
		Return(storage.ErrVersionConflict).
		Once()

	conversationRepo.On("GetConversation", ctx, "conv-existing").Return(reloadedConversation, nil).Once()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
		return transition != nil &&
			transition.Conversation != nil &&
			transition.Conversation.ID == "conv-existing" &&
			len(transition.ExpectedParticipantStates) == 2
	})).
		Return(nil).
		Once()

	result, err := service.SendMessage(ctx, &SendMessageCommand{
		SenderID:       "alice",
		ConversationID: "conv-existing",
		Content:        "hello after conflict",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "conv-existing", result.Conversation.ID)
	require.Len(t, result.Events, 3)
	require.Len(t, publisher.GetEvents(), 3)
	require.Empty(t, federation.GetQueuedActivities())
}
