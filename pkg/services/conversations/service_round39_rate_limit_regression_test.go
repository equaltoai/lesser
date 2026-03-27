package conversations

import (
	"context"
	"errors"
	"testing"
	"time"

	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestService_SendDirectMessage_FailedRequestDoesNotConsumeRateLimitBudget(t *testing.T) {
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
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow).
		Return(dmSendTotalLimit, resetTime, nil).
		Once()
	rateLimitRepo.
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow).
		Return(dmRequestTotalLimit, resetTime, nil).
		Once()
	rateLimitRepo.
		On("GetAPIRateLimitInfo", mock.Anything, "dm:alice", "dm_request_to:bob", dmRequestPerRecipientLimit, dmRequestPerRecipientWindow).
		Return(dmRequestPerRecipientLimit, resetTime, nil).
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
		nil,
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

	rateLimitRepo.AssertNotCalled(t, "CheckAPIRateLimit", mock.Anything, "dm:alice", "dm_send_total", dmSendTotalLimit, dmSendTotalWindow)
	rateLimitRepo.AssertNotCalled(t, "CheckAPIRateLimit", mock.Anything, "dm:alice", "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow)
	rateLimitRepo.AssertNotCalled(t, "CheckAPIRateLimit", mock.Anything, "dm:alice", "dm_request_to:bob", dmRequestPerRecipientLimit, dmRequestPerRecipientWindow)
}

func TestDirectMessageRequestPerRecipientLimit_IsUsable(t *testing.T) {
	require.Greater(t, dmRequestPerRecipientLimit, 1)
	require.Equal(t, 24*time.Hour, dmRequestPerRecipientWindow)
}
