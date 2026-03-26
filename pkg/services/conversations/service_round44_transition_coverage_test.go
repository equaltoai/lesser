package conversations

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestDefaultSendConversationState_AllowsNilConversation(t *testing.T) {
	state := defaultSendConversationState(nil, "alice", "bob")

	require.NotNil(t, state)
	require.Equal(t, "alice", state.ViewerID)
	require.Equal(t, "bob", state.CounterpartID)
	require.Empty(t, state.ConversationID)
	require.Equal(t, models.UserConversationFolderHidden, state.Folder)
	require.False(t, state.SortAt.IsZero())
	require.False(t, state.CreatedAt.IsZero())
	require.False(t, state.UpdatedAt.IsZero())
}

func TestService_resolveDirectMessageConversationForSend_ReturnsExistingConversation(t *testing.T) {
	ctx := context.Background()
	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	existingConversation := createTestConversation("conv-existing", []string{"alice", "bob"})

	conversationRepo.
		On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
		Return(existingConversation, nil).
		Once()

	conversation, createConversation, err := service.resolveDirectMessageConversationForSend(ctx, "bob", "alice")
	require.NoError(t, err)
	require.False(t, createConversation)
	require.Same(t, existingConversation, conversation)
	conversationRepo.AssertExpectations(t)
}

func TestService_executeDirectMessageSendAttempt_RetriesCreateRace(t *testing.T) {
	ctx := context.Background()
	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	sender := createTestAccount("alice", "alice")
	recipient := createTestAccount("bob", "bob")

	conversationRepo.
		On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
		Return((*models.Conversation)(nil), fmt.Errorf("not found")).
		Once()
	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
			return transition != nil && transition.CreateConversation
		})).
		Return(storage.ErrAlreadyExists).
		Once()

	attempt, retry, err := service.executeDirectMessageSendAttempt(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hello",
	}, sender, map[string]*storage.Account{
		"bob": recipient,
	}, "bob")
	require.NoError(t, err)
	require.True(t, retry)
	require.Nil(t, attempt)
	conversationRepo.AssertExpectations(t)
}
