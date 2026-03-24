package conversations

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_SendDirectMessage_UsesResolvedLegacyMixedCaseRecipientIdentity(t *testing.T) {
	service, conversationRepo, noteRepo, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	senderAccount := createTestAccount("Medic", "Medic")
	recipientAccount := createTestAccount("arch", "Arch")

	cmd := &SendDirectMessageCommand{
		SenderID:   "Medic",
		Recipients: []string{"arch"},
		Content:    "Testing lowercase mention delivery",
	}

	accountRepo.On("GetAccount", ctx, "Medic").Return(senderAccount, nil).Once()
	accountRepo.On("GetAccount", ctx, "arch").Return(recipientAccount, nil).Once()

	conversationRepo.On("GetConversationByParticipants", ctx, []string{"Arch", "Medic"}).
		Return((*models.Conversation)(nil), fmt.Errorf("not found")).
		Once()
	conversationRepo.On("CreateConversation", ctx, mock.AnythingOfType("*models.Conversation"), []string{"Arch", "Medic"}).
		Return(nil).
		Once()
	noteRepo.On("CreateStatus", ctx, mock.MatchedBy(func(status *models.Status) bool {
		return assert.Contains(t, status.ToRecipients, "https://example.com/users/Arch")
	})).
		Return(nil).
		Once()
	conversationRepo.On("UpdateConversation", ctx, mock.AnythingOfType("*models.Conversation")).Return(nil).Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "Medic").
		Return(&models.ConversationParticipantRecord{}, nil).
		Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "Arch").
		Return(&models.ConversationParticipantRecord{}, nil).
		Twice()
	conversationRepo.On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).
		Return(nil).
		Twice()
	conversationRepo.On("MarkConversationRead", ctx, mock.Anything, "Medic").Return(nil).Once()
	conversationRepo.On("MarkConversationUnread", ctx, mock.Anything, "Arch").Return(nil).Once()

	result, err := service.SendDirectMessage(ctx, cmd)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Conversation)
	assert.Equal(t, []string{"Arch", "Medic"}, result.Conversation.Participants)
	assert.Equal(t, []string{"https://example.com/users/Arch"}, result.Message.ToRecipients)

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}
