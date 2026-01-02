package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeInstanceConnectionsRepository struct {
	connections []*storage.InstanceConnection
	err         error

	calls int
}

func (f *fakeInstanceConnectionsRepository) GetInstanceConnections(_ context.Context, domain string, cursor string) ([]*storage.InstanceConnection, error) {
	f.calls++
	_ = domain
	_ = cursor
	return f.connections, f.err
}

func TestTrendAnalyzer_AnalyzeTrends_ErrorWrap(t *testing.T) {
	ta := &TrendAnalyzer{
		connectionsRepo: &fakeInstanceConnectionsRepository{err: errors.New("boom")},
	}

	_, err := ta.AnalyzeTrends(context.Background(), "example.com", 24*time.Hour)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGetConnectionsFailed)
}

func TestTrendAnalyzer_AnalyzeTrends_ComputesPatternsAndTrendingInstances(t *testing.T) {
	now := time.Now()
	connections := []*storage.InstanceConnection{
		{
			TargetDomain:   "trending.example",
			LastActivity:   now.Add(-2 * time.Hour),
			VolumeIn:       600,
			VolumeOut:      700,
			ResponseTimeMs: 1000,
			Success:        true,
		},
		{
			TargetDomain:   "trending.example",
			LastActivity:   now.Add(-1 * time.Hour),
			VolumeIn:       800,
			VolumeOut:      900,
			ResponseTimeMs: 3000,
			Success:        true,
		},
		{
			TargetDomain:   "other.example",
			LastActivity:   now.Add(-30 * time.Minute),
			VolumeIn:       10,
			VolumeOut:      20,
			ResponseTimeMs: 5000,
			Success:        false,
		},
		{
			TargetDomain: "trending.example",
			LastActivity: now.Add(-48 * time.Hour), // Different weekday for weekly patterns
			VolumeIn:     200,
			VolumeOut:    250,
			Success:      true,
		},
	}

	ta := &TrendAnalyzer{
		connectionsRepo: &fakeInstanceConnectionsRepository{connections: connections},
	}

	analysis, err := ta.AnalyzeTrends(context.Background(), "example.com", 8*24*time.Hour)
	require.NoError(t, err)
	require.NotNil(t, analysis)

	assert.Equal(t, "example.com", analysis.Domain)
	assert.NotNil(t, analysis.VolumeTrend)
	assert.NotNil(t, analysis.ResponseTimeTrend)
	assert.NotNil(t, analysis.ErrorRateTrend)
	assert.NotNil(t, analysis.Patterns)

	// Trending instance should be detected due to high volume + recency.
	require.NotEmpty(t, analysis.TrendingInstances)
	assert.Equal(t, "trending.example", analysis.TrendingInstances[0].Domain)
	assert.NotEmpty(t, analysis.TrendingInstances[0].TrendReason)

	// At least daily peak should be present; weekly peak depends on data distribution.
	hasDaily := false
	for _, p := range analysis.Patterns {
		if p.Type == "daily_peak" {
			hasDaily = true
			break
		}
	}
	assert.True(t, hasDaily)
	assert.GreaterOrEqual(t, analysis.OverallTrendScore, 0.0)
	assert.LessOrEqual(t, analysis.OverallTrendScore, 10.0)
}

func TestTrendAnalyzer_InternalHelpersAndBranches(t *testing.T) {
	ta := &TrendAnalyzer{}
	now := time.Now()

	t.Run("calculateLinearRegression_edge_cases", func(t *testing.T) {
		slope, r2 := ta.calculateLinearRegression([]int64{1})
		assert.Equal(t, 0.0, slope)
		assert.Equal(t, 0.0, r2)

		// Constant values => zero y variance.
		slope, r2 = ta.calculateLinearRegression([]int64{10, 10, 10})
		assert.InDelta(t, 0.0, slope, 0.0001)
		assert.InDelta(t, 0.0, r2, 0.0001)
	})

	t.Run("volumeTrend_directions", func(t *testing.T) {
		increasing := []*storage.InstanceConnection{
			{LastActivity: now.Add(-3 * time.Hour), VolumeIn: 10, VolumeOut: 0},
			{LastActivity: now.Add(-2 * time.Hour), VolumeIn: 100, VolumeOut: 0},
			{LastActivity: now.Add(-1 * time.Hour), VolumeIn: 1000, VolumeOut: 0},
		}
		vt := ta.analyzeVolumeTrend(increasing, now.Add(-4*time.Hour), now)
		assert.Equal(t, trendIncreasing, vt.Direction)
		assert.Greater(t, vt.TotalVolume, int64(0))
		assert.Greater(t, vt.PeakVolume, int64(0))

		decreasing := []*storage.InstanceConnection{
			{LastActivity: now.Add(-3 * time.Hour), VolumeIn: 1000, VolumeOut: 0},
			{LastActivity: now.Add(-2 * time.Hour), VolumeIn: 100, VolumeOut: 0},
			{LastActivity: now.Add(-1 * time.Hour), VolumeIn: 10, VolumeOut: 0},
		}
		vt = ta.analyzeVolumeTrend(decreasing, now.Add(-4*time.Hour), now)
		assert.Equal(t, trendDecreasing, vt.Direction)
	})

	t.Run("responseTimeTrend_directions", func(t *testing.T) {
		degrading := []*storage.InstanceConnection{
			{LastActivity: now.Add(-3 * time.Hour), ResponseTimeMs: 100},
			{LastActivity: now.Add(-2 * time.Hour), ResponseTimeMs: 2000},
			{LastActivity: now.Add(-1 * time.Hour), ResponseTimeMs: 4000},
		}
		rt := ta.analyzeResponseTimeTrend(degrading, now.Add(-4*time.Hour), now)
		assert.Equal(t, "degrading", rt.Direction)

		improving := []*storage.InstanceConnection{
			{LastActivity: now.Add(-3 * time.Hour), ResponseTimeMs: 4000},
			{LastActivity: now.Add(-2 * time.Hour), ResponseTimeMs: 2000},
			{LastActivity: now.Add(-1 * time.Hour), ResponseTimeMs: 100},
		}
		rt = ta.analyzeResponseTimeTrend(improving, now.Add(-4*time.Hour), now)
		assert.Equal(t, "improving", rt.Direction)
	})

	t.Run("errorRateTrend_directions", func(t *testing.T) {
		increasing := []*storage.InstanceConnection{
			{LastActivity: now.Add(-3 * time.Hour), Success: true},
			{LastActivity: now.Add(-2 * time.Hour), Success: false},
			{LastActivity: now.Add(-1 * time.Hour), Success: false},
		}
		et := ta.analyzeErrorRateTrend(increasing, now.Add(-4*time.Hour), now)
		assert.Equal(t, trendIncreasing, et.Direction)

		decreasing := []*storage.InstanceConnection{
			{LastActivity: now.Add(-3 * time.Hour), Success: false},
			{LastActivity: now.Add(-2 * time.Hour), Success: false},
			{LastActivity: now.Add(-1 * time.Hour), Success: true},
		}
		et = ta.analyzeErrorRateTrend(decreasing, now.Add(-4*time.Hour), now)
		assert.Equal(t, trendDecreasing, et.Direction)
	})

	t.Run("determineTrendReason_branches", func(t *testing.T) {
		assert.Equal(t, "high_volume", ta.determineTrendReason(&DomainStats{TotalVolume: 2000}, 0))
		assert.Equal(t, "recent_activity", ta.determineTrendReason(&DomainStats{LastActivity: time.Now()}, 0))
		assert.Equal(t, "high_reliability", ta.determineTrendReason(&DomainStats{LastActivity: time.Now().Add(-2 * time.Hour), ErrorCount: 0}, 0))
		assert.Equal(t, "general_activity", ta.determineTrendReason(&DomainStats{LastActivity: time.Now().Add(-2 * time.Hour), ErrorCount: 10}, 0))
	})

	t.Run("calculateOverallTrendScore_clamps", func(t *testing.T) {
		analysis := &TrendAnalysis{
			VolumeTrend:       &VolumeTrend{Direction: trendIncreasing},
			ResponseTimeTrend: &ResponseTimeTrend{Direction: "improving"},
			ErrorRateTrend:    &ErrorRateTrend{Direction: trendDecreasing},
			TrendingInstances: make([]*TrendingInstance, 100),
		}

		score := ta.calculateOverallTrendScore(analysis)
		assert.LessOrEqual(t, score, 10.0)

		analysis.VolumeTrend.Direction = trendDecreasing
		analysis.ResponseTimeTrend.Direction = "degrading"
		analysis.ErrorRateTrend.Direction = trendIncreasing
		analysis.TrendingInstances = nil
		score = ta.calculateOverallTrendScore(analysis)
		assert.GreaterOrEqual(t, score, 0.0)
	})
}
