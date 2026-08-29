package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_TimelineRepositoryCoverageSweep(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	expiresAt := baseTime.Add(10 * time.Minute)
	require.NoError(t, repo.AddToTimeline(ctx, "user-1", &storage.TimelineEntry{
		PostID:      "post-1",
		ActorID:     "https://example.com/users/user-1",
		ActorHandle: "@user-1@example.com",
		Content:     "hello",
		ContentType: "text/plain",
		CreatedAt:   baseTime,
		TimelineAt:  baseTime,
		ExpiresAt:   &expiresAt,
	}))

	_ = repo.RemoveFromTimeline(ctx, "user-1", "post-1")

	_ = repo.MuteConversation(ctx, "user-1", "conv-1")
	_ = repo.UnmuteConversation(ctx, "user-1", "conv-1")
	_, _ = repo.IsConversationMuted(ctx, "user-1", "conv-1")

	_, _ = repo.GetTimelineMarkers(ctx, "user-1", []string{"home", "notifications"})
	_ = repo.UpdateTimelineMarker(ctx, "user-1", "home", "last-1")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_RemoveFromTimeline_NotFoundIsNotError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Delete").Return(errors.ErrItemNotFound).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.NoError(t, repo.RemoveFromTimeline(ctx, "user-1", "missing"))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_MuteConversation_AlreadyMutedIsNotError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(errors.ErrConditionFailed).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.NoError(t, repo.MuteConversation(ctx, "user-1", "conv-1"))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_IsConversationMuted_NotFoundFalse(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	muted, err := repo.IsConversationMuted(ctx, "user-1", "conv-1")
	require.NoError(t, err)
	require.False(t, muted)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_UpdateTimelineMarker_CreateWhenMissing(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.NoError(t, repo.UpdateTimelineMarker(ctx, "user-1", "home", "last"))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_ListTimelineAndMarkerErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	t.Run("get_timeline_markers_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		markers, err := repo.GetTimelineMarkers(ctx, "user-1", []string{"home"})
		require.Error(t, err)
		require.Nil(t, markers)
	})

	t.Run("get_timeline_markers_success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.TimelineMarker)
			dest.LastReadID = "last"
			dest.UpdatedAt = baseTime
		}).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		markers, err := repo.GetTimelineMarkers(ctx, "user-1", []string{"home"})
		require.NoError(t, err)
		require.Contains(t, markers, "home")
		require.Equal(t, "last", markers["home"].LastReadID)
	})
}

func TestAccountRepository_TimelineMutationErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	t.Run("add_to_timeline_create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		err := repo.AddToTimeline(ctx, "user-1", &storage.TimelineEntry{
			PostID:      "post-1",
			ActorID:     "a",
			ActorHandle: "@a@example.com",
			Content:     "x",
			ContentType: "text/plain",
			CreatedAt:   baseTime,
			TimelineAt:  baseTime,
			ExpiresAt:   nil,
		})
		require.Error(t, err)
	})

	t.Run("remove_from_timeline_delete_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		err := repo.RemoveFromTimeline(ctx, "user-1", "post-1")
		require.Error(t, err)
	})

	t.Run("unmute_conversation_delete_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		err := repo.UnmuteConversation(ctx, "user-1", "conv-1")
		require.Error(t, err)
	})

	t.Run("is_conversation_muted_query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		muted, err := repo.IsConversationMuted(ctx, "user-1", "conv-1")
		require.Error(t, err)
		require.False(t, muted)
	})
}

func TestAccountRepository_UpdateTimelineMarker_UpdateExistingPaths(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)

	t.Run("update_success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.UpdateTimelineMarker(ctx, "user-1", "home", "last"))
	})

	t.Run("update_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateTimelineMarker(ctx, "user-1", "home", "last"))
	})
}
