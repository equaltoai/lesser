package conversations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
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
		On("CreateConversationWithParticipantStates", ctx, mock.AnythingOfType("*models.Conversation"), []string{"alice", "bob"}, mock.AnythingOfType("[]*models.UserConversationState")).
		Return(nil).
		Once()

	conversationRepo.On("GetUserConversationState", ctx, "alice", mock.Anything).Return(
		testConversationStateContract("alice", "", func(state *interfaces.UserConversationStateContract) {
			state.Unread = true
		}),
		nil,
	).Once()

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
	service, conversationRepo, _, accountRepo, publisher, federation := createTestService()
	ctx := context.Background()

	conversation := createTestConversation("conv123", []string{"sender123", "recipient456"})
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)

	accountRepo.On("GetAccount", ctx, "sender123").Return(createTestAccount("sender123", "alice"), nil)
	accountRepo.On("GetAccount", ctx, "recipient456").Return(createTestAccount("recipient456", "bob"), nil)

	conversationRepo.
		On("GetUserConversationState", ctx, "recipient456", "conv123").
		Return(testConversationStateContract("recipient456", "conv123", func(state *interfaces.UserConversationStateContract) {
			state.RequestState = models.DmRequestStateDeclined
		}), nil).
		Once()
	conversationRepo.
		On("GetUserConversationState", ctx, "sender123", "conv123").
		Return(testConversationStateContract("sender123", "conv123", nil), nil).
		Once()
	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
			if transition == nil || transition.Status == nil || len(transition.ParticipantStates) != 2 {
				return false
			}
			stateByViewer := map[string]*models.UserConversationState{}
			for _, state := range transition.ParticipantStates {
				stateByViewer[state.ViewerID] = state
			}

			senderState := stateByViewer["sender123"]
			recipientState := stateByViewer["recipient456"]
			return senderState != nil &&
				recipientState != nil &&
				senderState.Folder == models.UserConversationFolderInbox &&
				senderState.RequestState == models.DmRequestStateAccepted &&
				!senderState.Unread &&
				recipientState.Folder == models.UserConversationFolderRequests &&
				recipientState.RequestState == models.DmRequestStatePending &&
				recipientState.Unread
		})).
		Return(nil).
		Once()

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
	deletedAt := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)
	conversationRepo.On("GetUserConversationState", ctx, "alice", "conv123").Return(
		testConversationStateContract("alice", "conv123", func(state *interfaces.UserConversationStateContract) {
			state.RequestState = models.DmRequestStatePending
			state.Folder = models.UserConversationFolderRequests
			state.DeletedAt = &deletedAt
			state.RequestedAt = &deletedAt
			state.DeclinedAt = &deletedAt
		}),
		nil,
	).Twice()
	conversationRepo.On("PutUserConversationState", ctx, mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.RequestState == models.DmRequestStateAccepted &&
			state.Folder == models.UserConversationFolderInbox &&
			state.DeletedAt == nil &&
			state.RequestedAt == nil &&
			state.DeclinedAt == nil &&
			state.AcceptedAt != nil
	})).Return(nil).Once()

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
	deletedAt := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)
	conversationRepo.On("GetUserConversationState", ctx, "alice", "conv123").Return(
		testConversationStateContract("alice", "conv123", func(state *interfaces.UserConversationStateContract) {
			state.RequestState = models.DmRequestStatePending
			state.Folder = models.UserConversationFolderRequests
			state.DeletedAt = &deletedAt
			state.RequestedAt = &deletedAt
		}),
		nil,
	).Twice()
	conversationRepo.On("PutUserConversationState", ctx, mock.MatchedBy(func(state *models.UserConversationState) bool {
		return state != nil &&
			state.RequestState == models.DmRequestStateDeclined &&
			state.Folder == models.UserConversationFolderDeclined &&
			state.DeletedAt == nil &&
			state.RequestedAt == nil &&
			state.AcceptedAt == nil &&
			state.DeclinedAt != nil
	})).Return(nil).Once()

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
