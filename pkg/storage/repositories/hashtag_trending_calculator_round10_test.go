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
