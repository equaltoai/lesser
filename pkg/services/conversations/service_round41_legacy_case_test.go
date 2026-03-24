package conversations

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
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

func TestService_SendDirectMessage_UsesResolvedLegacyMixedCaseSenderIdentity(t *testing.T) {
	service, conversationRepo, noteRepo, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	senderAccount := createTestAccount("medic", "Medic")
	recipientAccount := createTestAccount("arch", "Arch")

	cmd := &SendDirectMessageCommand{
		SenderID:   "medic",
		Recipients: []string{"arch"},
		Content:    "Testing lowercase sender identity",
	}

	accountRepo.On("GetAccount", ctx, "medic").Return(senderAccount, nil).Once()
	accountRepo.On("GetAccount", ctx, "arch").Return(recipientAccount, nil).Once()

	conversationRepo.On("GetConversationByParticipants", ctx, []string{"Arch", "Medic"}).
		Return((*models.Conversation)(nil), fmt.Errorf("not found")).
		Once()
	conversationRepo.On("CreateConversation", ctx, mock.AnythingOfType("*models.Conversation"), []string{"Arch", "Medic"}).
		Return(nil).
		Once()
	noteRepo.On("CreateStatus", ctx, mock.MatchedBy(func(status *models.Status) bool {
		return assert.Equal(t, "Medic", status.AuthorID)
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
	assert.Equal(t, "Medic", result.Message.AuthorID)

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}

func TestResolvedLegacyLocalAccountID(t *testing.T) {
	t.Run("returns requested id when account is nil", func(t *testing.T) {
		require.Equal(t, "arch", resolvedLegacyLocalAccountID("arch", nil))
	})

	t.Run("returns requested id when stored username is empty", func(t *testing.T) {
		account := &storage.Account{User: &storage.User{}}
		require.Equal(t, "arch", resolvedLegacyLocalAccountID("arch", account))
	})

	t.Run("keeps exact match unchanged", func(t *testing.T) {
		account := &storage.Account{User: &storage.User{Username: "Arch"}}
		require.Equal(t, "Arch", resolvedLegacyLocalAccountID("Arch", account))
	})

	t.Run("rewrites legacy mixed-case local id", func(t *testing.T) {
		account := &storage.Account{User: &storage.User{Username: "Arch"}}
		require.Equal(t, "Arch", resolvedLegacyLocalAccountID("arch", account))
	})

	t.Run("keeps unrelated ids untouched", func(t *testing.T) {
		account := &storage.Account{User: &storage.User{Username: "Medic"}}
		require.Equal(t, "arch", resolvedLegacyLocalAccountID("arch", account))
	})
}

func TestCloneDirectMessageCommandWithResolvedParticipants(t *testing.T) {
	t.Run("nil command returns nil", func(t *testing.T) {
		require.Nil(t, cloneDirectMessageCommandWithResolvedParticipants(nil, nil, nil))
	})

	t.Run("rewrites only legacy mixed-case participants", func(t *testing.T) {
		cmd := &SendDirectMessageCommand{
			SenderID:   "medic",
			Recipients: []string{"arch"},
			Content:    "hi",
		}

		cloned := cloneDirectMessageCommandWithResolvedParticipants(
			cmd,
			&storage.Account{User: &storage.User{Username: "Medic"}},
			&storage.Account{User: &storage.User{Username: "Arch"}},
		)

		require.NotNil(t, cloned)
		require.Equal(t, "Medic", cloned.SenderID)
		require.Equal(t, []string{"Arch"}, cloned.Recipients)
		require.Equal(t, "medic", cmd.SenderID)
		require.Equal(t, []string{"arch"}, cmd.Recipients)
	})

	t.Run("preserves recipient slice when empty", func(t *testing.T) {
		cmd := &SendDirectMessageCommand{SenderID: "medic"}
		cloned := cloneDirectMessageCommandWithResolvedParticipants(cmd, nil, nil)
		require.NotNil(t, cloned)
		require.Empty(t, cloned.Recipients)
	})
}

func TestService_previewDirectMessageRequestRateLimit_UsesPerRecipientPreview(t *testing.T) {
	ctx := context.Background()
	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	resetTime := time.Now().Add(time.Minute)

	rateLimitRepo.
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow).
		Return(1, resetTime, nil).
		Once()
	rateLimitRepo.
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_request_to:bob", dmRequestPerRecipientLimit, dmRequestPerRecipientWindow).
		Return(1, resetTime, nil).
		Once()

	service := NewService(nil, nil, nil, nil, nil, nil, rateLimitRepo, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	err := service.previewDirectMessageRequestRateLimit(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, "conv123", "bob")
	require.NoError(t, err)

	rateLimitRepo.AssertExpectations(t)
}

func TestService_previewDirectMessageRequestRateLimit_ReturnsTotalPreviewFailure(t *testing.T) {
	ctx := context.Background()
	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	resetTime := time.Now().Add(time.Minute)

	rateLimitRepo.
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow).
		Return(0, resetTime, nil).
		Once()

	service := NewService(nil, nil, nil, nil, nil, nil, rateLimitRepo, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	err := service.previewDirectMessageRequestRateLimit(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, "conv123", "bob")
	require.ErrorIs(t, err, storage.ErrRateLimited)

	rateLimitRepo.AssertExpectations(t)
}

func TestService_evaluateDirectMessageRequestPolicy_DeclinedThreadReopensAsRequest(t *testing.T) {
	ctx := context.Background()
	conversationRepo := &mockConversationRepository{}
	conversationRepo.
		On("GetConversationParticipantRecord", ctx, "conv123", "bob").
		Return(&models.ConversationParticipantRecord{RequestState: models.DmRequestStateDeclined}, nil).
		Once()

	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	willBeRequest, deliversToInbox, state, err := service.evaluateDirectMessageRequestPolicy(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, "conv123", "bob")
	require.NoError(t, err)
	assert.True(t, willBeRequest)
	assert.False(t, deliversToInbox)
	assert.Equal(t, models.DmRequestStateDeclined, state)

	conversationRepo.AssertExpectations(t)
}

func TestService_evaluateDirectMessageRequestPolicy_AcceptedThreadSkipsRequestFlow(t *testing.T) {
	ctx := context.Background()
	conversationRepo := &mockConversationRepository{}
	conversationRepo.
		On("GetConversationParticipantRecord", ctx, "conv123", "bob").
		Return(&models.ConversationParticipantRecord{RequestState: models.DmRequestStateAccepted}, nil).
		Once()

	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	willBeRequest, deliversToInbox, state, err := service.evaluateDirectMessageRequestPolicy(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	}, "conv123", "bob")
	require.NoError(t, err)
	assert.False(t, willBeRequest)
	assert.False(t, deliversToInbox)
	assert.Equal(t, models.DmRequestStateAccepted, state)

	conversationRepo.AssertExpectations(t)
}

func TestService_SendDirectMessage_RateLimitsPendingRequestPreview(t *testing.T) {
	ctx := context.Background()
	service, conversationRepo, _, accountRepo, _, _ := createTestService()
	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	service.rateLimitRepo = rateLimitRepo

	senderAccount := createTestAccount("alice", "alice")
	recipientAccount := createTestAccount("bob", "bob")
	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	resetTime := time.Now().Add(time.Minute)

	accountRepo.On("GetAccount", ctx, "alice").Return(senderAccount, nil).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(recipientAccount, nil).Once()
	conversationRepo.On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).Return(conversation, nil).Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv123", "bob").
		Return(&models.ConversationParticipantRecord{RequestState: models.DmRequestStateDeclined}, nil).
		Once()

	rateLimitRepo.
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow).
		Return(1, resetTime, nil).
		Once()
	rateLimitRepo.
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow).
		Return(1, resetTime, nil).
		Once()
	rateLimitRepo.
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_request_to:bob", dmRequestPerRecipientLimit, dmRequestPerRecipientWindow).
		Return(0, resetTime, nil).
		Once()

	_, err := service.SendDirectMessage(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hi",
	})
	require.ErrorIs(t, err, storage.ErrRateLimited)

	conversationRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	rateLimitRepo.AssertExpectations(t)
}
