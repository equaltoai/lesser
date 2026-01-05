package cost

import (
	"context"
	"errors"
	"testing"
	"time"

	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeAICostRepo struct {
	costs []*models.AICost
	err   error
}

func (f fakeAICostRepo) GetAICostsByTimeRange(context.Context, time.Time, time.Time, string, int) ([]*models.AICost, error) {
	return f.costs, f.err
}

type fakeWebSocketCostRepo struct {
	costs []*models.WebSocketCostRecord
	err   error
}

func (f fakeWebSocketCostRepo) GetRecentCosts(context.Context, time.Time, int) ([]*models.WebSocketCostRecord, error) {
	return f.costs, f.err
}

func TestAnalyticsService_CalculateGrowthTrends_CoversMajorBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 30, 12, 0, 0, 0, time.UTC)

	svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

	t.Run("unsupported_metric", func(t *testing.T) {
		_, err := svc.CalculateGrowthTrends(ctx, "nope", 10, now)
		require.ErrorIs(t, err, svcErrors.ErrUnsupportedMetric)
	})

	t.Run("ai_cost_nil_repo", func(t *testing.T) {
		_, err := svc.CalculateGrowthTrends(ctx, "ai_cost", 10, now)
		require.ErrorIs(t, err, svcErrors.ErrGetAICostData)
	})

	t.Run("websocket_cost_nil_repo_falls_back_to_sample_data_and_runs_full_pipeline", func(t *testing.T) {
		out, err := svc.CalculateGrowthTrends(ctx, "websocket_cost", 30, now)
		require.NoError(t, err)
		require.NotNil(t, out)
		require.NotEmpty(t, out.TrendDirection)
		require.NotNil(t, out.SeasonalPatterns)
		require.NotEmpty(t, out.ConfidenceInterval)
	})

	t.Run("websocket_cost_repo_error_falls_back_to_sample_data", func(t *testing.T) {
		svc.webSocketCostRepo = fakeWebSocketCostRepo{err: errors.New("boom")}
		out, err := svc.CalculateGrowthTrends(ctx, "websocket_cost", 30, now)
		require.NoError(t, err)
		require.NotNil(t, out)
	})

	t.Run("websocket_cost_repo_success_uses_data_points", func(t *testing.T) {
		records := make([]*models.WebSocketCostRecord, 0, 30)
		for i := 0; i < 30; i++ {
			records = append(records, &models.WebSocketCostRecord{
				Timestamp:            now.AddDate(0, 0, -i),
				EstimatedCostDollars: float64((i%7)+1) * 0.5,
			})
		}

		svc.webSocketCostRepo = fakeWebSocketCostRepo{costs: records}
		out, err := svc.CalculateGrowthTrends(ctx, "websocket_cost", 30, now)
		require.NoError(t, err)
		require.NotNil(t, out)
	})

	t.Run("ai_cost_repo_error", func(t *testing.T) {
		svc.aiCostRepo = fakeAICostRepo{err: errors.New("boom")}
		_, err := svc.CalculateGrowthTrends(ctx, "ai_cost", 10, now)
		require.Error(t, err)
		require.ErrorIs(t, err, svcErrors.ErrGetAICostData)
	})

	t.Run("ai_cost_repo_success", func(t *testing.T) {
		costs := make([]*models.AICost, 0, 14)
		for i := 0; i < 14; i++ {
			costs = append(costs, &models.AICost{
				Timestamp:           now.AddDate(0, 0, -i),
				TotalCostMicroCents: int64((i%7)+1) * 1_000_000, // $1-$7
			})
		}

		svc.aiCostRepo = fakeAICostRepo{costs: costs}
		out, err := svc.CalculateGrowthTrends(ctx, "ai_cost", 14, now)
		require.NoError(t, err)
		require.NotNil(t, out)
	})
}

func TestAnalyticsService_MiscHelpers_Coverage(t *testing.T) {
	svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

	require.Equal(t, "increasing", svc.determineTrendDirection(0.02))
	require.Equal(t, "decreasing", svc.determineTrendDirection(-0.02))
	require.Equal(t, "stable", svc.determineTrendDirection(0.0))

	require.Equal(t, 0.0, svc.calculateCompoundGrowthRate(nil))
	require.Equal(t, 0.0, svc.calculateCompoundGrowthRate([]float64{0, 1}))
	require.Equal(t, 0.0, svc.calculateCompoundGrowthRate([]float64{1}))

	require.NotNil(t, svc.calculateConfidenceInterval([]float64{1, 2, 3}, 10.0))
	require.Equal(t, [2]float64{10, 10}, svc.calculateConfidenceInterval([]float64{1, 2}, 10.0))

	// Forecast fallbacks.
	require.Equal(t, 0.0, svc.forecastNextPeriod(nil, &TrendAnalysis{}))
	require.Equal(t, 10.0, svc.forecastNextPeriod([]float64{10}, &TrendAnalysis{}))
	require.Equal(t, 11.0, svc.forecastNextPeriod([]float64{10}, &TrendAnalysis{LinearRegression: RegressionAnalysis{Slope: 1}}))

	// Autocorrelation edge case.
	require.Nil(t, svc.calculateAutocorrelation([]float64{1, 2, 3}, 7))
}

func TestAnalyticsService_AssessModelAccuracy_CoversBranches(t *testing.T) {
	svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

	// Length mismatch / empty guard.
	require.Equal(t, ModelAccuracy{}, svc.assessModelAccuracy([]float64{1, 2}, []float64{1}))
	require.Equal(t, ModelAccuracy{}, svc.assessModelAccuracy(nil, nil))

	// Constant series gives ssTot == 0 path.
	acc := svc.assessModelAccuracy([]float64{5, 5, 5, 5}, []float64{5, 5, 5, 5})
	require.Equal(t, 0.0, acc.R2Score)

	// Non-constant series covers full metric calculations.
	acc = svc.assessModelAccuracy([]float64{1, 2, 3, 4, 5}, []float64{1, 2, 2.5, 4.2, 5})
	require.NotZero(t, acc.RMSE)
}
