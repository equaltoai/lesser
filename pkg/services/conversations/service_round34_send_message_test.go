package conversations

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestService_CreateConversation_CreatesNewConversation(t *testing.T) {
	service, conversationRepo, _, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil)

	conversationRepo.
		On("GetConversationByParticipants", ctx, []string{"alice", "bob"}).
		Return((*models.Conversation)(nil), errors.New("not found"))
	conversationRepo.
		On("CreateConversation", ctx, mock.AnythingOfType("*models.Conversation"), []string{"alice", "bob"}).
		Return(nil).
		Once()

	creatorRecord := &models.ConversationParticipantRecord{Unread: true}
	participantRecord := &models.ConversationParticipantRecord{}
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "alice").Return(creatorRecord, nil)
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "bob").Return(participantRecord, nil)
	conversationRepo.On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).Return(nil)

	result, err := service.CreateConversation(ctx, &CreateConversationCommand{
		CreatorID:     "alice",
		ParticipantID: "bob",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Conversation)
	require.Contains(t, result.Conversation.Participants, "alice")
	require.Contains(t, result.Conversation.Participants, "bob")
	require.True(t, result.Conversation.Unread)
}

func TestService_SendMessage_Success(t *testing.T) {
	service, conversationRepo, noteRepo, accountRepo, publisher, federation := createTestService()
	ctx := context.Background()

	conversation := createTestConversation("conv123", []string{"sender123", "recipient456"})
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)

	accountRepo.On("GetAccount", ctx, "sender123").Return(createTestAccount("sender123", "alice"), nil)
	accountRepo.On("GetAccount", ctx, "recipient456").Return(createTestAccount("recipient456", "bob"), nil)

	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil).Once()
	conversationRepo.On("UpdateConversation", ctx, mock.AnythingOfType("*models.Conversation")).Return(nil).Once()

	conversationRepo.
		On("GetConversationParticipantRecord", ctx, "conv123", "sender123").
		Return(&models.ConversationParticipantRecord{}, nil).
		Once()
	conversationRepo.
		On("GetConversationParticipantRecord", ctx, "conv123", "recipient456").
		Return(&models.ConversationParticipantRecord{RequestState: models.DmRequestStateDeclined}, nil).
		Once()
	conversationRepo.
		On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).
		Return(nil).
		Twice()

	conversationRepo.On("MarkConversationRead", ctx, "conv123", "sender123").Return(nil).Once()
	conversationRepo.On("MarkConversationUnread", ctx, "conv123", "recipient456").Return(nil).Once()

	result, err := service.SendMessage(ctx, &SendMessageCommand{
		SenderID:       "sender123",
		ConversationID: "conv123",
		Content:        "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "conv123", result.Conversation.ID)
	require.Equal(t, "hello", result.Message.Content)
	require.Len(t, result.Events, 3)
	require.Len(t, publisher.GetEvents(), 3)
	require.Empty(t, federation.GetQueuedActivities())

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}

func TestService_SendMessage_ReplyTargetMustMatchConversation(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	conversationRepo.On("GetConversation", ctx, "conv123").Return(createTestConversation("conv123", []string{"alice", "bob"}), nil)

	parent := &models.Status{
		Visibility:     VisibilityDirect,
		ConversationID: "other-conv",
	}
	noteRepo.On("GetStatus", ctx, "parent-1").Return(parent, nil).Twice()

	_, err := service.SendMessage(ctx, &SendMessageCommand{
		SenderID:       "alice",
		ConversationID: "conv123",
		Content:        "reply",
		InReplyToID:    "parent-1",
	})
	require.ErrorIs(t, err, ErrInvalidInReplyToIDConversation)
}

func TestService_MessageRequestDecisions_UpdateParticipantStateAndAudit(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	auditRepo := testmocks.NewMockAuditRepository()

	service := NewService(
		conversationRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		auditRepo,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	record := &models.ConversationParticipantRecord{Conversation: conversation, RequestState: models.DmRequestStatePending}

	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv123", "alice").Return(record, nil).Twice()
	conversationRepo.On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).Return(nil).Once()

	auditRepo.
		On(
			"StoreAuditEvent",
			mock.Anything,
			"dm.request.accept",
			"LOW",
			"alice",
			"alice",
			"",
			"",
			"",
			"",
			"",
			true,
			"",
			mock.MatchedBy(func(md map[string]interface{}) bool {
				return md != nil && md["conversation_id"] == "conv123"
			}),
		).
		Return(nil).
		Once()

	result, err := service.AcceptMessageRequest(ctx, &AcceptMessageRequestCommand{
		ConversationID: "conv123",
		UserID:         "alice",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, models.DmRequestStateAccepted, record.RequestState)

	conversationRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestService_DeclineMessageRequest_UpdatesParticipantStateAndAudit(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	auditRepo := testmocks.NewMockAuditRepository()

	service := NewService(
		conversationRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		auditRepo,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	record := &models.ConversationParticipantRecord{Conversation: conversation, RequestState: models.DmRequestStatePending}

	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv123", "alice").Return(record, nil).Twice()
	conversationRepo.On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).Return(nil).Once()

	auditRepo.
		On(
			"StoreAuditEvent",
			mock.Anything,
			"dm.request.decline",
			"LOW",
			"alice",
			"alice",
			"",
			"",
			"",
			"",
			"",
			true,
			"",
			mock.MatchedBy(func(md map[string]interface{}) bool {
				return md != nil && md["conversation_id"] == "conv123"
			}),
		).
		Return(nil).
		Once()

	result, err := service.DeclineMessageRequest(ctx, &DeclineMessageRequestCommand{
		ConversationID: "conv123",
		UserID:         "alice",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, models.DmRequestStateDeclined, record.RequestState)
	require.NotNil(t, record.DeclinedAt)
	require.Nil(t, record.AcceptedAt)

	conversationRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestService_DeclineMessageRequest_ReturnsErrNotConversationParticipant(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(
		conversationRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		zaptest.NewLogger(t),
		"example.com",
	)

	conversationRepo.On("GetConversation", ctx, "conv123").Return(createTestConversation("conv123", []string{"bob"}), nil)

	_, err := service.DeclineMessageRequest(ctx, &DeclineMessageRequestCommand{
		ConversationID: "conv123",
		UserID:         "alice",
	})
	require.ErrorIs(t, err, ErrNotConversationParticipant)
}
