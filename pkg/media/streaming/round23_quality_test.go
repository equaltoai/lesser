package streaming

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAdaptiveQualitySelector_MetricsCache_GetAndCleanup(t *testing.T) {
	aqs := NewAdaptiveQualitySelector(zap.NewNop())

	aqs.UpdateMetrics("s1", 1, 2)
	m := aqs.GetQualityMetrics("s1")
	require.NotNil(t, m)
	assert.Equal(t, 1, m.RebufferEvents)
	assert.Equal(t, 2, m.QualitySwitches)

	// Force cleanup by aging the metric.
	m.LastQualityChange = time.Now().Add(-2 * time.Hour)
	aqs.metricsCache.Store("s1", m)

	aqs.CleanupMetrics(time.Hour)
	assert.Nil(t, aqs.GetQualityMetrics("s1"))
}

func TestAdaptiveQualitySelector_SelectQualityWithSession_StabilitySelection(t *testing.T) {
	metrics := NewMetricsTracker(nil, zap.NewNop())
	metrics.StartSession("s1", "u1", "m1")

	metricsAny, ok := metrics.sessionMetrics.Load("s1")
	require.True(t, ok)
	s := metricsAny.(*SessionMetrics)
	s.StartTime = time.Now().Add(-2 * time.Minute)
	s.CurrentQuality = Quality720p
	s.QualitySwitches = 10 // > 3 per minute
	s.SegmentSuccessRate = 0.99

	aqs := NewAdaptiveQualitySelector(zap.NewNop())
	aqs.SetMetricsTracker(metrics)

	got := aqs.SelectQualityWithSession("s1", 10000, 0.9, []Quality{Quality240p, Quality480p, Quality720p, Quality1080p})
	assert.Equal(t, Quality720p, got)
}

func TestAdaptiveQualitySelector_SelectQualityWithSession_BaseLogic(t *testing.T) {
	aqs := NewAdaptiveQualitySelector(zap.NewNop())

	available := []Quality{Quality240p, Quality360p, Quality480p, Quality720p, Quality1080p}

	// Panic mode (buffer critically low).
	assert.Equal(t, Quality240p, aqs.SelectQualityWithSession("", 3000, 0.1, available))

	// Conservative mode (buffer low but not panic).
	assert.Equal(t, Quality720p, aqs.SelectQualityWithSession("", 10000, 0.3, available))

	// Healthy buffer chooses best supported quality.
	assert.Equal(t, Quality1080p, aqs.SelectQualityWithSession("", 10000, 0.8, available))
}

func TestAdaptiveQualitySelector_SelectOptimalQualityWithMetrics_UsesSessionSignals(t *testing.T) {
	metricsTracker := NewMetricsTracker(nil, zap.NewNop())
	metricsTracker.StartSession("s1", "u1", "m1")

	metricsAny, ok := metricsTracker.sessionMetrics.Load("s1")
	require.True(t, ok)
	s := metricsAny.(*SessionMetrics)
	s.StartTime = time.Now().Add(-2 * time.Minute)
	s.CurrentQuality = Quality720p
	s.SegmentSuccessRate = 0.90
	s.RebufferEvents = 2
	s.TotalRebufferTime = 15 * time.Second
	s.TimeInEachQuality[Quality720p] = time.Minute
	s.QoEScore = 0.9

	aqs := NewAdaptiveQualitySelector(zap.NewNop())
	aqs.SetMetricsTracker(metricsTracker)

	got := aqs.SelectQualityWithSession("s1", 10000, 0.9, []Quality{Quality480p, Quality720p, Quality1080p})
	assert.Contains(t, []Quality{Quality480p, Quality720p, Quality1080p}, got)
}
