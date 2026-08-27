package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
