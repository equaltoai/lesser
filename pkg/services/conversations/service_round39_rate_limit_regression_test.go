package conversations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestService_SendDirectMessage_ConsumesRateLimitBeforeWrites(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	accountRepo := &mockAccountRepository{}

	relationshipRepo := testmocks.NewMockRelationshipRepository()
	relationshipRepo.
		On("IsBlockedBidirectional", mock.Anything, "alice", "bob").
		Return(false, nil).
		Once()
	relationshipRepo.
		On("IsFollowing", mock.Anything, "bob", "alice").
		Return(false, nil).
		Once()

	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	resetTime := time.Now().UTC().Add(time.Minute)
	rateLimitRepo.
		On("CheckFixedWindowRateLimit", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow).
		Return(true, dmSendTotalLimit-1, resetTime, nil).
		Once()
	rateLimitRepo.
		On("CheckFixedWindowRateLimit", mock.Anything, "dm:alice", "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow).
		Return(true, dmRequestTotalLimit-1, resetTime, nil).
		Once()
	rateLimitRepo.
		On("CheckFixedWindowRateLimit", mock.Anything, "dm:alice", "dm_request_to:bob", dmRequestPerRecipientLimit, dmRequestPerRecipientWindow).
		Return(true, dmRequestPerRecipientLimit-1, resetTime, nil).
		Once()

	service := NewService(
		conversationRepo,
		nil,
		nil,
		accountRepo,
		relationshipRepo,
		nil,
		rateLimitRepo,
		nil,
		&mockPublisher{},
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	accountRepo.On("GetAccount", mock.Anything, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
	accountRepo.On("GetAccount", mock.Anything, "bob").Return(createTestAccount("bob", "bob"), nil).Once()
	conversationRepo.On("GetConversationByParticipants", mock.Anything, []string{"alice", "bob"}).Return(conversation, nil).Once()
	conversationRepo.On("GetUserConversationState", mock.Anything, "bob", "conv123").Return(
		testConversationStateContract("bob", "conv123", nil),
		nil,
	).Once()
	conversationRepo.On("GetUserConversationState", mock.Anything, "alice", "conv123").Return(
		testConversationStateContract("alice", "conv123", nil),
		nil,
	).Once()
	conversationRepo.On("ApplyDirectMessageSend", mock.Anything, mock.AnythingOfType("*models.DirectMessageSendTransition"), mock.Anything).Return(errors.New("boom")).Once()

	_, err := service.SendDirectMessage(ctx, &SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "hello",
	})
	require.ErrorIs(t, err, ErrCreateDirectMessage)

	rateLimitRepo.AssertExpectations(t)
}

func TestService_SendMessage_ConsumesTotalRateLimitBeforeWrite(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	accountRepo := &mockAccountRepository{}
	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	resetTime := time.Now().UTC().Add(time.Minute)

	service := NewService(
		conversationRepo,
		nil,
		nil,
		accountRepo,
		nil,
		nil,
		rateLimitRepo,
		nil,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	conversation := createTestConversation("conv-existing", []string{"alice", "bob"})
	conversationRepo.On("GetConversation", ctx, "conv-existing").Return(conversation, nil).Once()
	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()
	rateLimitRepo.
		On("CheckFixedWindowRateLimit", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow).
		Return(false, 0, resetTime, nil).
		Once()

	_, err := service.SendMessage(ctx, &SendMessageCommand{
		SenderID:       "alice",
		ConversationID: "conv-existing",
		Content:        "blocked by total rate limit",
	})
	require.ErrorIs(t, err, storage.ErrRateLimited)

	conversationRepo.AssertNotCalled(t, "ApplyDirectMessageSend", mock.Anything, mock.Anything, mock.Anything)
	conversationRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	rateLimitRepo.AssertExpectations(t)
}

func TestService_SendMessage_ConsumesTotalRateLimitOnceAcrossRetry(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	accountRepo := &mockAccountRepository{}
	rateLimitRepo := testmocks.NewMockRateLimitRepository()
	resetTime := time.Now().UTC().Add(time.Minute)

	service := NewService(
		conversationRepo,
		nil,
		nil,
		accountRepo,
		nil,
		nil,
		rateLimitRepo,
		nil,
		&mockPublisher{},
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	initialConversation := createTestConversation("conv-existing", []string{"alice", "bob"})
	reloadedConversation := createTestConversation("conv-existing", []string{"alice", "bob"})
	conversationRepo.On("GetConversation", ctx, "conv-existing").Return(initialConversation, nil).Once()
	conversationRepo.On("GetConversation", ctx, "conv-existing").Return(reloadedConversation, nil).Once()
	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Twice()
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Twice()
	rateLimitRepo.
		On("CheckFixedWindowRateLimit", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow).
		Return(true, dmSendTotalLimit-1, resetTime, nil).
		Once()
	conversationRepo.On("GetUserConversationState", ctx, "bob", "conv-existing").
		Return(testConversationStateContract("bob", "conv-existing", func(state *interfaces.UserConversationStateContract) {
			state.RequestState = models.DmRequestStateAccepted
		}), nil).
		Twice()
	conversationRepo.On("GetUserConversationState", ctx, "alice", "conv-existing").
		Return(testConversationStateContract("alice", "conv-existing", nil), nil).
		Twice()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition"), mock.Anything).
		Return(storage.ErrVersionConflict).
		Once()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition"), mock.Anything).
		Return(nil).
		Once()

	result, err := service.SendMessage(ctx, &SendMessageCommand{
		SenderID:       "alice",
		ConversationID: "conv-existing",
		Content:        "allowed once across retry",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	conversationRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	rateLimitRepo.AssertExpectations(t)
}

func TestDirectMessageRequestPerRecipientLimit_IsUsable(t *testing.T) {
	require.Greater(t, dmRequestPerRecipientLimit, 1)
	require.Equal(t, 24*time.Hour, dmRequestPerRecipientWindow)
}
