package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestModerationMLRepository_Round09_Coverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)

	t.Run("CreateSample sets defaults and persists", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
		sample := &models.ModerationSample{
			ObjectID:   "obj-1",
			ObjectType: "status",
			Label:      "spam",
			ReviewerID: "reviewer-1",
			Confidence: 0.9,
		}
		require.NoError(t, repo.CreateSample(ctx, sample))
		require.NotEmpty(t, sample.ID)
		require.False(t, sample.CreatedAt.IsZero())
		require.False(t, sample.UpdatedAt.IsZero())
		require.False(t, sample.Timestamp.IsZero())
	})

	t.Run("GetSample not found maps to message", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mockMatchedByType[*models.ModerationSample]()).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
		got, err := repo.GetSample(ctx, "missing")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("ListSamplesByLabel and reviewer", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("Scan", mockMatchedByType[*[]models.ModerationSample]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.ModerationSample)
				*out = append(*out,
					models.ModerationSample{ID: "s1", Label: "spam"},
					models.ModerationSample{ID: "s2", Label: "spam"},
				)
			}).
			Return(nil).
			Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())

		byLabel, err := repo.ListSamplesByLabel(ctx, "spam", 0)
		require.NoError(t, err)
		require.Len(t, byLabel, 2)

		byReviewer, err := repo.ListSamplesByReviewer(ctx, "reviewer-1", -1)
		require.NoError(t, err)
		require.Len(t, byReviewer, 2)
	})

	t.Run("Model version CRUD branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())

		ver := &models.ModerationModelVersion{
			Accuracy: 0.75,
			IsActive: true,
		}
		require.NoError(t, repo.CreateModelVersion(ctx, ver))
		require.NotEmpty(t, ver.VersionID)

		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Twice()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewModerationMLRepository(mockDBNF, "test-table", zap.NewNop())

		got, err := repoNF.GetModelVersion(ctx, "missing")
		require.Error(t, err)
		require.Nil(t, got)

		got, err = repoNF.GetActiveModelVersion(ctx)
		require.Error(t, err)
		require.Nil(t, got)

		ver2 := &models.ModerationModelVersion{VersionID: "v1", Accuracy: 0.8}
		require.NoError(t, repo.UpdateModelVersion(ctx, ver2))
	})

	t.Run("Effectiveness metric operations", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mockMatchedByType[*models.ModerationEffectivenessMetric]()).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())

		metric := &models.ModerationEffectivenessMetric{
			PatternID:      "p1",
			Period:         "daily",
			StartTime:      baseTime,
			EndTime:        baseTime.Add(24 * time.Hour),
			TruePositives:  10,
			FalsePositives: 1,
			TrueNegatives:  5,
			FalseNegatives: 2,
			TotalReviewed:  18,
		}

		require.NoError(t, repo.CreateEffectivenessMetric(ctx, metric))

		got, err := repo.GetEffectivenessMetric(ctx, "p1", "daily", baseTime)
		require.Error(t, err)
		require.Nil(t, got)

		byPattern, err := repo.ListEffectivenessMetricsByPattern(ctx, "p1", 0)
		require.NoError(t, err)
		require.Len(t, byPattern, 2)

		byPeriod, err := repo.ListEffectivenessMetricsByPeriod(ctx, "daily", 0)
		require.NoError(t, err)
		require.Len(t, byPeriod, 2)
	})
}
