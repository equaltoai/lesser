package cost

import (
	"context"
	"math"
	"testing"

	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAnalyticsService_PredictFutureCosts(t *testing.T) {
	t.Run("insufficient_historical_data", func(t *testing.T) {
		svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

		prediction, err := svc.PredictFutureCosts(context.Background(), []float64{1, 2, 3, 4, 5, 6}, 5)
		require.ErrorIs(t, err, svcErrors.ErrInsufficientHistoricalData)
		require.Nil(t, prediction)
	})

	t.Run("produces_forecast_points_and_intervals", func(t *testing.T) {
		svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

		history := []float64{10, 11, 12, 13, 14, 15, 16, 16.5, 17, 18, 19, 19.5, 20, 21}
		prediction, err := svc.PredictFutureCosts(context.Background(), history, 5)
		require.NoError(t, err)
		require.NotNil(t, prediction)

		require.Equal(t, 5, prediction.ForecastHorizon)
		require.Len(t, prediction.PredictedValues, 5)
		require.Len(t, prediction.ConfidenceIntervals, 5)
		require.NotNil(t, prediction.SeasonalDecomposition)

		for i, point := range prediction.PredictedValues {
			assert.False(t, point.Timestamp.IsZero(), "point %d timestamp", i)
			assert.NotEmpty(t, point.Method, "point %d method", i)
			assert.False(t, math.IsNaN(point.PredictedValue) || math.IsInf(point.PredictedValue, 0), "point %d predicted value", i)
			assert.GreaterOrEqual(t, point.Confidence, 0.0)
			assert.LessOrEqual(t, point.Confidence, 100.0)
		}

		for i, interval := range prediction.ConfidenceIntervals {
			assert.False(t, interval.Timestamp.IsZero(), "interval %d timestamp", i)
			assert.False(t, math.IsNaN(interval.LowerBound) || math.IsInf(interval.LowerBound, 0), "interval %d lower bound", i)
			assert.False(t, math.IsNaN(interval.UpperBound) || math.IsInf(interval.UpperBound, 0), "interval %d upper bound", i)
			assert.LessOrEqual(t, interval.LowerBound, interval.UpperBound, "interval %d bounds", i)
		}

		assert.False(t, math.IsNaN(prediction.ModelAccuracy.R2Score) || math.IsInf(prediction.ModelAccuracy.R2Score, 0))
	})
}

func TestAnalyticsService_calculateLinearRegression(t *testing.T) {
	svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

	// y = 2x + 1 for x = 0..3
	out := svc.calculateLinearRegression([]float64{1, 3, 5, 7})

	assert.InDelta(t, 2.0, out.Slope, 0.0001)
	assert.InDelta(t, 1.0, out.Intercept, 0.0001)
	assert.InDelta(t, 1.0, out.RSquared, 0.0001)
	assert.Equal(t, "significant", out.TrendSignificance)
}

func TestAnalyticsService_calculateMovingAverage(t *testing.T) {
	svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

	out := svc.calculateMovingAverage([]float64{1, 2, 3, 4, 5, 6, 7}, 3)
	require.Len(t, out, 5)
	assert.Equal(t, []float64{2, 3, 4, 5, 6}, out)
}

func TestAnalyticsService_DetectAnomalies(t *testing.T) {
	t.Run("insufficient_data_returns_empty_report", func(t *testing.T) {
		svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

		report, err := svc.DetectAnomalies(context.Background(), []float64{1, 2, 3})
		require.NoError(t, err)
		require.NotNil(t, report)
		assert.Equal(t, 0, report.TotalAnomalies)
	})

	t.Run("detects_spike", func(t *testing.T) {
		svc := NewAnalyticsService(nil, nil, nil, zap.NewNop())

		series := []float64{10, 10, 10, 10, 10, 10, 10, 500, 10, 10, 10, 10, 10, 10}
		report, err := svc.DetectAnomalies(context.Background(), series)
		require.NoError(t, err)
		require.NotNil(t, report)

		assert.Greater(t, report.TotalAnomalies, 0)
		assert.NotEmpty(t, report.Severity)
		assert.NotEmpty(t, report.Categories)
	})
}
