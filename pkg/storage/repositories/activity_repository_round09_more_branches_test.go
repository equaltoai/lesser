package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityRepository_Round09_MoreBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 2, 3, 4, 0, time.UTC)

	t.Run("GetActivity query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.GetActivity(ctx, "id")
		require.Error(t, err)
	})

	t.Run("GetInboxActivities query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		defer func() { _ = repo.costService.Close(ctx) }()

		_, _, err := repo.GetInboxActivities(ctx, "alice", 1, "")
		require.Error(t, err)
	})

	t.Run("GetHashtagActivity skips nil activities and handles query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Activity)
				*out = append(*out,
					&models.Activity{CreatedAt: baseTime, Activity: nil},
					&models.Activity{CreatedAt: baseTime, Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "x", Type: "Create"}, Object: map[string]interface{}{"content": "no match"}}},
				)
			}).
			Return(nil).
			Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
		acts, err := repo.GetHashtagActivity(ctx, "go", baseTime.Add(-time.Hour))
		require.NoError(t, err)
		require.Empty(t, acts)

		mockDBErr := new(mocks.MockDB)
		mockQueryErr := new(mocks.MockQuery)
		mockQueryErr.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)
		repoErr := NewActivityRepository(mockDBErr, "test-table", zap.NewNop(), nil)
		_, err = repoErr.GetHashtagActivity(ctx, "go", baseTime.Add(-time.Hour))
		require.Error(t, err)
	})
}
