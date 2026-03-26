package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound34_ConversationRepository_RequestStateHelpers(t *testing.T) {
	t.Run("clampListLimit", func(t *testing.T) {
		require.Equal(t, 20, clampListLimit(0, 20, 100))
		require.Equal(t, 20, clampListLimit(-1, 20, 100))
		require.Equal(t, 100, clampListLimit(101, 20, 100))
		require.Equal(t, 50, clampListLimit(50, 20, 100))
	})

	t.Run("folderFromRequestState", func(t *testing.T) {
		require.Equal(t, models.UserConversationFolderInbox, folderFromRequestState(models.DmRequestStateAccepted))
		require.Equal(t, models.UserConversationFolderRequests, folderFromRequestState(models.DmRequestStatePending))
		require.Equal(t, models.UserConversationFolderDeclined, folderFromRequestState(models.DmRequestStateDeclined))
		require.Equal(t, models.UserConversationFolderInbox, folderFromRequestState(""))
	})

	t.Run("participantRecordFolder prefers explicit folder", func(t *testing.T) {
		require.Equal(t, models.UserConversationFolderHidden, participantRecordFolder(nil))
		require.Equal(t, models.UserConversationFolderRequests, participantRecordFolder(&models.ConversationParticipantRecord{
			Folder: models.UserConversationFolderRequests,
		}))
		require.Equal(t, models.UserConversationFolderHidden, participantRecordFolder(&models.ConversationParticipantRecord{
			DeletedAt: conversationTimePtr(time.Unix(1, 0).UTC()),
		}))
		require.Equal(t, models.UserConversationFolderDeclined, participantRecordFolder(&models.ConversationParticipantRecord{
			RequestState: models.DmRequestStateDeclined,
		}))
	})

	t.Run("mergeVisibleConversationStatePages orders by sort time", func(t *testing.T) {
		inbox := []*models.UserConversationState{{
			ConversationID: "conv-1",
			SortAt:         time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		}}
		requests := []*models.UserConversationState{{
			ConversationID: "conv-2",
			SortAt:         time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
		}}

		merged, nextCursor, hasMore := mergeVisibleConversationStatePages(inbox, requests, 1)
		require.Len(t, merged, 1)
		require.Equal(t, "conv-1", merged[0].ConversationID)
		require.True(t, hasMore)
		require.Equal(t, merged[0].LegacyListCursor(), nextCursor)
	})
}

func TestRound34_ConversationRepository_ListUserConversationStatesByFolderModels(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#INBOX").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 3).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<", "cursor").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{
			{ViewerID: "alice", ConversationID: "conv-1", Folder: models.UserConversationFolderInbox, SortAt: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)},
			{ViewerID: "alice", ConversationID: "conv-2", Folder: models.UserConversationFolderInbox, SortAt: time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC)},
			{ViewerID: "alice", ConversationID: "conv-3", Folder: models.UserConversationFolderInbox, SortAt: time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)},
		}
	}).Return(nil).Once()

	states, nextCursor, hasMore, err := repo.listUserConversationStatesByFolderModels(ctx, "alice", models.UserConversationFolderInbox, interfaces.PaginationOptions{
		Limit:  2,
		Cursor: "cursor",
	})
	require.NoError(t, err)
	require.Len(t, states, 2)
	require.True(t, hasMore)
	require.Equal(t, states[1].LegacyListCursor(), nextCursor)
}

func TestRound34_ConversationRepository_GetUserConversationsByRequestState_UsesFolderQuery(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	requestQuery := new(mocks.MockQuery)
	conversationQuery := new(mocks.MockQuery)
	conversation := &models.Conversation{
		ID:           "conv-1",
		Participants: []string{"alice", "bob"},
		UpdatedAt:    time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.UserConversationState")).Return(requestQuery).Once()
	requestQuery.On("Index", "gsi1").Return(requestQuery).Once()
	requestQuery.On("Where", "gsi1PK", "=", "USER_CONVERSATION_FOLDER#alice#REQUESTS").Return(requestQuery).Once()
	requestQuery.On("OrderBy", "gsi1SK", "DESC").Return(requestQuery).Once()
	requestQuery.On("Limit", 2).Return(requestQuery).Once()
	requestQuery.On("All", mock.AnythingOfType("*[]*models.UserConversationState")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.UserConversationState)
		*dest = []*models.UserConversationState{{
			ViewerID:       "alice",
			ConversationID: "conv-1",
			CounterpartID:  "bob",
			Folder:         models.UserConversationFolderRequests,
			SortAt:         conversation.UpdatedAt,
			Unread:         true,
		}}
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.Conversation")).Return(conversationQuery).Once()
	conversationQuery.On("Where", "PK", "=", "CONVERSATION#conv-1").Return(conversationQuery).Once()
	conversationQuery.On("Where", "SK", "=", "METADATA").Return(conversationQuery).Once()
	conversationQuery.On("First", mock.AnythingOfType("*models.Conversation")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Conversation)
		*dest = *conversation
	}).Return(nil).Once()

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetUserConversationsByRequestState(ctx, "alice", models.DmRequestStatePending, interfaces.PaginationOptions{Limit: 1})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "conv-1", result.Items[0].ID)
	require.True(t, result.Items[0].Unread)
}

func conversationTimePtr(t time.Time) *time.Time {
	return &t
}
