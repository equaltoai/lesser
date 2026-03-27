package conversations

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestService_GetConversation_ReturnsErrGetConversationMessages_OnThreadError(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}

	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil).Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv123", "alice").Return((*models.ConversationParticipantRecord)(nil), errors.New("boom")).Once()

	noteRepo.On("GetConversationThread", ctx, "conv123", mock.Anything).Return((*interfaces.PaginatedResult[*models.Status])(nil), errors.New("boom")).Once()

	_, err := service.GetConversation(ctx, &GetConversationQuery{
		ConversationID: "conv123",
		ViewerID:       "alice",
		Pagination:     interfaces.PaginationOptions{Limit: 10},
	})
	require.ErrorIs(t, err, ErrGetConversationMessages)
}

func TestService_GetConversation_ReturnsErrGetConversationMessages_OnTombstoneError(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	dmTombstoneRepo := &mockDirectMessageTombstoneRepository{}

	service := NewService(conversationRepo, noteRepo, dmTombstoneRepo, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversation := createTestConversation("conv123", []string{"alice", "bob"})
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil).Once()
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv123", "alice").Return(&models.ConversationParticipantRecord{Unread: true}, nil).Once()

	paginated := &interfaces.PaginatedResult[*models.Status]{
		Items: []*models.Status{
			{
				StatusID:       "msg-1",
				Visibility:     VisibilityDirect,
				AuthorUsername: "alice",
				ToRecipients:   []string{"https://example.com/users/bob"},
			},
		},
		HasMore: false,
	}
	noteRepo.On("GetConversationThread", ctx, "conv123", mock.Anything).Return(paginated, nil).Once()

	dmTombstoneRepo.
		On("TombstonesByStatusID", ctx, "alice", mock.Anything).
		Return((map[string]bool)(nil), errors.New("boom")).
		Once()

	_, err := service.GetConversation(ctx, &GetConversationQuery{
		ConversationID: "conv123",
		ViewerID:       "alice",
		Pagination:     interfaces.PaginationOptions{Limit: 10},
	})
	require.ErrorIs(t, err, ErrGetConversationMessages)
}

func TestService_auditDMRequestEvent_UsesMediumSeverityOnFailure(t *testing.T) {
	ctx := context.Background()

	auditRepo := testmocks.NewMockAuditRepository()
	auditRepo.
		On(
			"StoreAuditEvent",
			mock.Anything,
			"dm.request.accept",
			"MEDIUM",
			"alice",
			"alice",
			"",
			"",
			"",
			"",
			"",
			false,
			"boom",
			mock.MatchedBy(func(md map[string]interface{}) bool {
				return md != nil && md["conversation_id"] == "conv123" && md["x"] == "y"
			}),
		).
		Return(nil).
		Once()

	service := NewService(nil, nil, nil, nil, nil, nil, nil, auditRepo, nil, nil, zaptest.NewLogger(t), "example.com")
	service.auditDMRequestEvent(ctx, "dm.request.accept", "alice", "conv123", false, "boom", map[string]any{
		"x": "y",
	})

	auditRepo.AssertExpectations(t)
}

func TestService_getUserConversationStateForSend_ReturnsNilOnErrorOrMissingState(t *testing.T) {
	ctx := context.Background()

	conversationRepo := &mockConversationRepository{}
	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	conversationRepo.
		On("GetUserConversationState", ctx, "bob", "conv123").
		Return((*interfaces.UserConversationStateContract)(nil), errors.New("boom")).
		Once()
	record, err := service.getUserConversationStateForSend(ctx, "conv123", "bob")
	require.Error(t, err)
	require.Nil(t, record)

	conversationRepo.
		On("GetUserConversationState", ctx, "bob", "conv123").
		Return((*interfaces.UserConversationStateContract)(nil), nil).
		Once()
	record, err = service.getUserConversationStateForSend(ctx, "conv123", "bob")
	require.NoError(t, err)
	require.Nil(t, record)
}
