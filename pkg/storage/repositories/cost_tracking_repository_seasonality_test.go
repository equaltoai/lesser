package repositories

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTrackingRepository_AnalyzeSeasonality_MonthlyDominant(t *testing.T) {
	repo := &TrackingRepository{}
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// 60 points with a perfect 30-day repeating pattern => strong monthly seasonality.
	dataPoints := make([]CostDataPoint, 60)
	for i := range dataPoints {
		dataPoints[i] = CostDataPoint{
			Timestamp:   baseTime.AddDate(0, 0, i),
			CostDollars: float64(i % 30),
		}
	}

	analysis := repo.analyzeSeasonality(dataPoints)
	require.NotNil(t, analysis)
	require.True(t, analysis.HasSeasonality)
	require.Equal(t, 30, analysis.SeasonalPeriod)
	require.NotEmpty(t, analysis.SeasonalPatterns)
	require.NotEmpty(t, analysis.TrendComponent)
	require.NotEmpty(t, analysis.SeasonalComponent)
	require.NotEmpty(t, analysis.ResidualComponent)
	require.GreaterOrEqual(t, analysis.DecompositionR2, 0.0)
}
