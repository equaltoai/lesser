package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStreamingCloudWatchMetrics_UpdateKeys_Validations(t *testing.T) {
	m := &StreamingCloudWatchMetrics{}
	assert.Error(t, m.UpdateKeys())

	m.MediaID = "m1"
	assert.Error(t, m.UpdateKeys())

	m.MetricType = "performance"
	assert.Error(t, m.UpdateKeys())

	m.Date = "2024-01-01"
	assert.Error(t, m.UpdateKeys())

	m.PK = "STREAMING_METRICS#performance"
	m.SK = "m1#ts"
	m.Timestamp = time.Unix(1700000000, 0).UTC()
	assert.NoError(t, m.UpdateKeys())
	assert.Equal(t, "METRIC_TIME#2024-01-01", m.GSI1PK)
	assert.Equal(t, "MEDIA#m1", m.GSI2PK)
}

func TestStreamingCloudWatchMetrics_Setters_ExpiryAndFreshness(t *testing.T) {
	m := &StreamingCloudWatchMetrics{}
	m.SetQualityBreakdown("m1", map[string]QualityMetric{
		"720p": {Quality: "720p", ViewerCount: 10},
	})
	assert.Equal(t, "m1", m.MediaID)
	assert.Equal(t, "quality_breakdown", m.MetricType)
	assert.Contains(t, m.PK, "STREAMING_METRICS#quality_breakdown")
	assert.Contains(t, m.SK, "m1#")
	assert.NotEmpty(t, m.GSI1PK)
	assert.NotEmpty(t, m.GSI2PK)
	assert.True(t, m.TTL > 0)

	// Expiry behavior.
	m.CacheExpiry = time.Now().Add(-time.Second)
	assert.True(t, m.IsExpired())
	m.CacheExpiry = time.Now().Add(time.Second)
	assert.False(t, m.IsExpired())

	// Freshness update.
	m.CloudWatchQueryTime = time.Now().Add(-5 * time.Second)
	m.UpdateFreshness()
	assert.GreaterOrEqual(t, m.DataFreshness, int64(5))

	// Other setters exercise different cache TTLs.
	m2 := &StreamingCloudWatchMetrics{}
	m2.SetGeographicData("m2", map[string]GeographicMetric{"US": {Region: "US", PreferredQuality: defaultVideoQuality}})
	assert.Equal(t, "geographic_data", m2.MetricType)

	m3 := &StreamingCloudWatchMetrics{}
	m3.SetConcurrentViewers("m3", ConcurrentViewerMetrics{CurrentViewers: 1})
	assert.Equal(t, "concurrent_viewers", m3.MetricType)

	m4 := &StreamingCloudWatchMetrics{}
	m4.SetPerformanceMetrics("m4", StreamingPerformanceMetrics{OverallLatencyMs: 10})
	assert.Equal(t, "performance", m4.MetricType)
}
