package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	ddbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
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

	t.Run("requestStateFetchLimit", func(t *testing.T) {
		require.Equal(t, 20, requestStateFetchLimit(1))
		require.Equal(t, 60, requestStateFetchLimit(20))
		require.Equal(t, 200, requestStateFetchLimit(100))
	})

	t.Run("matchesRequestState", func(t *testing.T) {
		require.True(t, matchesRequestState(models.DmRequestStateAccepted, models.DmRequestStateAccepted))
		require.True(t, matchesRequestState("", models.DmRequestStateAccepted))
		require.False(t, matchesRequestState("", models.DmRequestStatePending))
		require.False(t, matchesRequestState(models.DmRequestStateDeclined, models.DmRequestStateAccepted))
	})

	t.Run("appendRequestStateMatches", func(t *testing.T) {
		deletedAt := time.Unix(1, 0).UTC()
		conv := &models.Conversation{ID: "conv-1"}

		records := []*models.ConversationParticipantRecord{
			nil,
			{
				SK:           "sk-deleted",
				DeletedAt:    &deletedAt,
				RequestState: models.DmRequestStateAccepted,
				Conversation: &models.Conversation{ID: "deleted"},
			},
			{
				SK:           "sk-pending",
				RequestState: models.DmRequestStatePending,
				Conversation: &models.Conversation{ID: "pending"},
			},
			{
				SK:           "sk-accepted",
				RequestState: models.DmRequestStateAccepted,
				Unread:       true,
				Conversation: conv,
			},
			{
				SK:               "sk-nil-conv",
				RequestState:     models.DmRequestStateAccepted,
				ConversationData: &models.ConversationSnapshot{ID: "from-snapshot"},
			},
		}

		got, lastSK := appendRequestStateMatches(records, models.DmRequestStateAccepted, 1, []*models.Conversation{})
		require.Len(t, got, 1)
		require.Equal(t, "conv-1", got[0].ID)
		require.True(t, got[0].Unread)
		require.Equal(t, "sk-accepted", lastSK)

		got, lastSK = appendRequestStateMatches(records[4:], models.DmRequestStateAccepted, 1, []*models.Conversation{})
		require.Len(t, got, 1)
		require.Equal(t, "from-snapshot", got[0].ID)
		require.Equal(t, "sk-nil-conv", lastSK)
	})
}

func TestRound34_ConversationRepository_fetchUserConversationParticipantRecords_NotFoundReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "USER_CONVERSATIONS#alice").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()

	records, hasMore, err := repo.fetchUserConversationParticipantRecords(ctx, "alice", 20, "")
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Len(t, records, 0)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestRound34_ConversationRepository_fetchUserConversationParticipantRecords_TruncatesAndCursor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "USER_CONVERSATIONS#alice").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 3).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "<", "cursor").Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ConversationParticipantRecord)
		*dest = []*models.ConversationParticipantRecord{{SK: "1"}, {SK: "2"}, {SK: "3"}}
	}).Return(nil).Once()

	records, hasMore, err := repo.fetchUserConversationParticipantRecords(ctx, "alice", 2, "cursor")
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Len(t, records, 2)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestRound34_ConversationRepository_scanUserConversationsByRequestState_ClampsBounds(t *testing.T) {
	ctx := context.Background()

	t.Run("default limit when <=0", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "USER_CONVERSATIONS#alice").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 61).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Once()

		items, nextCursor, hasMore, err := repo.scanUserConversationsByRequestState(ctx, "alice", models.DmRequestStateAccepted, 0, "")
		require.NoError(t, err)
		require.Empty(t, items)
		require.Empty(t, nextCursor)
		require.False(t, hasMore)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("max limit when >100", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "USER_CONVERSATIONS#alice").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 201).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Once()

		items, nextCursor, hasMore, err := repo.scanUserConversationsByRequestState(ctx, "alice", models.DmRequestStateAccepted, 101, "")
		require.NoError(t, err)
		require.Empty(t, items)
		require.Empty(t, nextCursor)
		require.False(t, hasMore)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestRound34_ConversationRepository_GetUserConversationsByRequestState_ReturnsCursorWhenLimitReached(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "USER_CONVERSATIONS#alice").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ConversationParticipantRecord)
		*dest = []*models.ConversationParticipantRecord{
			{
				SK:           "cursor#1",
				RequestState: models.DmRequestStateAccepted,
				Unread:       true,
				Conversation: &models.Conversation{ID: "conv-1"},
			},
		}
	}).Return(nil).Once()

	result, err := repo.GetUserConversationsByRequestState(ctx, "alice", models.DmRequestStateAccepted, interfaces.PaginationOptions{Limit: 1})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Items, 1)
	require.True(t, result.Items[0].Unread)
	require.Equal(t, "cursor#1", result.NextCursor)
	require.True(t, result.HasMore)
	require.Equal(t, int64(-1), result.Total)
}

func TestRound34_ConversationRepository_scanUserConversationsByRequestState_PropagatesFetchError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "USER_CONVERSATIONS#alice").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()

	_, _, _, err := repo.scanUserConversationsByRequestState(ctx, "alice", models.DmRequestStateAccepted, 1, "")
	require.Error(t, err)
}
