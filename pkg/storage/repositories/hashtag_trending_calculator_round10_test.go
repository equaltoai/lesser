package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_HashtagTrendingCalculator_ConfigAndHistory(t *testing.T) {
	repo := NewHashtagRepository(new(mocks.MockDB), "test-table", zap.NewNop(), "example.com")

	repo.trendingCalculator = nil
	defaultCfg := repo.GetTrendingCalculatorConfig()
	require.Greater(t, defaultCfg.DecayHalfLife, time.Duration(0))

	repo.ReconfigureTrendingCalculator(TrendingCalculatorConfig{MaximumAge: time.Hour})
	require.Equal(t, time.Hour, repo.GetTrendingCalculatorConfig().MaximumAge)

	history, err := repo.GetHashtagTrendingHistory(context.Background(), "tag", 10)
	require.NoError(t, err)
	require.Empty(t, history)
}

func TestRound10_HashtagTrendingCalculator_GetTrendingHashtagsAdvanced(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewHashtagRepository(mockDB, "test-table", zap.NewNop(), "example.com")
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	_, err := repo.GetTrendingHashtagsAdvanced(ctx, TrendingCalculatorConfig{MaximumAge: 10 * time.Minute}, 3)
	require.NoError(t, err)
}

func TestRound10_HashtagTrendingCalculator_GetTrendingAnalytics(t *testing.T) {
	ctx := context.Background()
	since := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("success aggregates usage and trending candidates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Hashtag)
			*dest = []*models.Hashtag{
				{Name: "go", UsageCount: 5},
				{Name: "dev", UsageCount: 3},
			}
		}).Return(nil)

		repo := NewHashtagRepository(mockDB, "test-table", zap.NewNop(), "example.com")
		key := repo.trendingEngine.getCacheKey(since, 100)
		repo.trendingEngine.cache.setTrendingResult(key, &CachedTrendingResult{
			Results:     []*storage.TrendingHashtag{{Name: "go"}},
			GeneratedAt: time.Now(),
		})

		analytics, err := repo.GetTrendingAnalytics(ctx, since)
		require.NoError(t, err)
		require.EqualValues(t, 2, analytics.TotalHashtags)
		require.EqualValues(t, 8, analytics.TotalUsage)
		require.EqualValues(t, 1, analytics.TrendingCandidates)
		require.Greater(t, analytics.AverageUsagePerTag, 0.0)
	})

	t.Run("scan not found is treated as empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Scan", mock.Anything).Return(dynamormErrors.ErrItemNotFound)

		repo := NewHashtagRepository(mockDB, "test-table", zap.NewNop(), "example.com")
		key := repo.trendingEngine.getCacheKey(since, 100)
		repo.trendingEngine.cache.setTrendingResult(key, &CachedTrendingResult{
			Results:     []*storage.TrendingHashtag{},
			GeneratedAt: time.Now(),
		})

		analytics, err := repo.GetTrendingAnalytics(ctx, since)
		require.NoError(t, err)
		require.EqualValues(t, 0, analytics.TotalHashtags)
	})

	t.Run("scan error fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Scan", mock.Anything).Return(errors.New("boom"))

		repo := NewHashtagRepository(mockDB, "test-table", zap.NewNop(), "example.com")
		_, err := repo.GetTrendingAnalytics(ctx, since)
		require.Error(t, err)
	})
}
