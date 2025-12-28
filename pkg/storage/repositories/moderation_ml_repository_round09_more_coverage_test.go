package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestModerationMLRepository_Round09_MoreCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("CreateSample propagates create errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
		err := repo.CreateSample(ctx, &models.ModerationSample{
			ObjectID:   "obj-1",
			ObjectType: "status",
			Label:      "spam",
			ReviewerID: "reviewer-1",
		})
		require.Error(t, err)
	})

	t.Run("GetSample non-notfound errors propagate", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetSample(ctx, "s1")
		require.Error(t, err)
	})

	t.Run("List scans error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Scan", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.ListSamplesByLabel(ctx, "spam", 1)
		require.Error(t, err)
	})

	t.Run("Model version and metrics errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// CreateModelVersion uses Model(version).Create().
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
		require.Error(t, repo.CreateModelVersion(ctx, &models.ModerationModelVersion{VersionID: "v1"}))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewModerationMLRepository(mockDB2, "test-table", zap.NewNop())
		_, err := repo2.GetModelVersion(ctx, "v1")
		require.Error(t, err)

		mockDB3 := new(mocks.MockDB)
		mockQuery3 := new(mocks.MockQuery)
		mockQuery3.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB3, mockQuery3, nil, baseTime)
		repo3 := NewModerationMLRepository(mockDB3, "test-table", zap.NewNop())
		_, err = repo3.GetActiveModelVersion(ctx)
		require.Error(t, err)

		mockDB4 := new(mocks.MockDB)
		mockQuery4 := new(mocks.MockQuery)
		mockQuery4.On("Update", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB4, mockQuery4, nil, baseTime)
		repo4 := NewModerationMLRepository(mockDB4, "test-table", zap.NewNop())
		require.Error(t, repo4.UpdateModelVersion(ctx, &models.ModerationModelVersion{VersionID: "v1"}))

		mockDB5 := new(mocks.MockDB)
		mockQuery5 := new(mocks.MockQuery)
		mockQuery5.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB5, mockQuery5, nil, baseTime)
		repo5 := NewModerationMLRepository(mockDB5, "test-table", zap.NewNop())
		require.Error(t, repo5.CreateEffectivenessMetric(ctx, &models.ModerationEffectivenessMetric{
			PatternID: "p1", Period: "daily", StartTime: baseTime, EndTime: baseTime.Add(time.Hour),
		}))

		mockDB6 := new(mocks.MockDB)
		mockQuery6 := new(mocks.MockQuery)
		mockQuery6.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB6, mockQuery6, nil, baseTime)
		repo6 := NewModerationMLRepository(mockDB6, "test-table", zap.NewNop())
		_, err = repo6.GetEffectivenessMetric(ctx, "p1", "daily", baseTime)
		require.Error(t, err)

		mockDB7 := new(mocks.MockDB)
		mockQuery7 := new(mocks.MockQuery)
		mockQuery7.On("Scan", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB7, mockQuery7, nil, baseTime)
		repo7 := NewModerationMLRepository(mockDB7, "test-table", zap.NewNop())
		_, err = repo7.ListEffectivenessMetricsByPeriod(ctx, "daily", 1)
		require.Error(t, err)

		mockDB8 := new(mocks.MockDB)
		mockQuery8 := new(mocks.MockQuery)
		mockQuery8.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB8, mockQuery8, nil, baseTime)
		repo8 := NewModerationMLRepository(mockDB8, "test-table", zap.NewNop())
		_, err = repo8.GetEffectivenessMetric(ctx, "p1", "daily", baseTime)
		require.Error(t, err)
	})
}
