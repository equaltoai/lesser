package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_TimelineRepository_FindWithPaginationAndCursor(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewTimelineRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	home, next, err := repo.GetHomeTimeline(ctx, "user-1", 1, "")
	require.NoError(t, err)
	require.Len(t, home, 1)
	require.NotEmpty(t, next)

	list, _, err := repo.GetListTimeline(ctx, "list-1", 1, next)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	direct, _, err := repo.GetDirectTimeline(ctx, "user-1", 1, "")
	require.NoError(t, err)
	require.NotEmpty(t, direct)

	hashtag, _, err := repo.GetHashtagTimeline(ctx, "tag", true, 1, "")
	require.NoError(t, err)
	require.NotEmpty(t, hashtag)
}

func TestRound09_TimelineRepository_GSIAndErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewTimelineRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	_, _, err := repo.GetPublicTimeline(ctx, true, 1, "cursor")
	require.NoError(t, err)

	_, _, err = repo.GetTimelineEntriesByPost(ctx, "post-1", 1, "cursor")
	require.NoError(t, err)

	mockDBErr := new(mocks.MockDB)
	mockQueryErr := new(mocks.MockQuery)
	mockQueryErr.On("All", mock.Anything).Return(errors.New("boom")).Once()
	mockQueryErr.On("First", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)

	repoErr := NewTimelineRepository(mockDBErr, "test-table", zap.NewNop(), nil)
	repoErr.SetValidationService(nil)
	repoErr.SetPermissionService(nil)
	repoErr.SetEventService(nil)
	repoErr.SetCachingService(nil)

	_, _, err = repoErr.GetPublicTimeline(ctx, false, 1, "")
	require.Error(t, err)

	_, err = repoErr.GetTimelineEntry(ctx, "HOME", "user-1", "missing", baseTime)
	require.Error(t, err)

	mockDBUpdate := new(mocks.MockDB)
	mockQueryUpdate := new(mocks.MockQuery)
	mockQueryUpdate.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBUpdate, mockQueryUpdate, nil, baseTime)

	repoUpdate := NewTimelineRepository(mockDBUpdate, "test-table", zap.NewNop(), nil)
	repoUpdate.SetValidationService(nil)
	repoUpdate.SetPermissionService(nil)
	repoUpdate.SetEventService(nil)
	repoUpdate.SetCachingService(nil)

	require.Error(t, repoUpdate.UpdateTimelineEntry(ctx, &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "user-1",
		PostID:       "post-1",
		TimelineAt:   baseTime,
	}))
}
