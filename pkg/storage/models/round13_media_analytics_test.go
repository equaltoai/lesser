package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMediaAnalytics_UpdateKeys_ValidationsAndSuccess(t *testing.T) {
	m := &MediaAnalytics{}
	assert.Error(t, m.UpdateKeys())

	m.MediaID = "m1"
	assert.Error(t, m.UpdateKeys())

	m.Date = "2024-01-01"
	assert.Error(t, m.UpdateKeys())

	m.PK = "MANIFEST#hls"
	m.SK = "123#m1"
	m.Format = "hls"
	m.Timestamp = time.Unix(1700000000, 0).UTC()
	m.DominantVariant = "720p_h264_1000"
	assert.NoError(t, m.UpdateKeys())

	assert.Equal(t, "DATE#2024-01-01", m.GSI1PK)
	assert.Contains(t, m.GSI1SK, "hls#")
	assert.Equal(t, "VARIANT#720p_h264_1000", m.GSI2PK)
	assert.Contains(t, m.GSI2SK, "COST#")
}

func TestMediaAnalytics_Setters_AndVariantTracking(t *testing.T) {
	before := time.Now()
	m := &MediaAnalytics{}
	m.SetManifestGeneration("m1", "hls", 12.34)
	after := time.Now()

	assert.Equal(t, "m1", m.MediaID)
	assert.Equal(t, "hls", m.Format)
	assert.Equal(t, "manifest_generated", m.EventType)
	assert.Contains(t, m.PK, "MANIFEST#hls")
	assert.Contains(t, m.SK, "#m1")
	assert.NotNil(t, m.VariantCosts)
	assert.NotNil(t, m.VariantBandwidth)
	assert.True(t, m.TTL > 0)

	ttl := time.Unix(m.TTL, 0)
	assert.True(t, ttl.After(before.Add(30*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(30*24*time.Hour+5*time.Second)))

	m2 := &MediaAnalytics{}
	m2.SetQualityChange("m1", "u1", "480p", "720p")
	assert.Equal(t, "quality_changed", m2.EventType)
	assert.Contains(t, m2.PK, "QUALITY_CHANGE#u1")
	assert.Contains(t, m2.SK, "#720p")

	m3 := &MediaAnalytics{}
	m3.SetGeneralEvent("session_start", "m1", "u1")
	assert.Equal(t, "session_start", m3.EventType)
	assert.Contains(t, m3.PK, "MEDIA_EVENT#session_start")
	assert.Contains(t, m3.SK, "#m1")

	// AddVariantCost + dominant variant selection.
	m.AddVariantCost("720p", "h264", 1000, MediaVariantCost{
		ProcessingCost: 10,
		StorageCost:    20,
		BandwidthCost:  30,
		ViewerMinutes:  10,
	})
	m.AddVariantCost("1080p", "h264", 2000, MediaVariantCost{
		ProcessingCost: 100,
		StorageCost:    0,
		BandwidthCost:  0,
		ViewerMinutes:  1,
	})

	assert.Equal(t, int64(160), m.TotalVariantCost)
	assert.Equal(t, "1080p_h264_2000", m.DominantVariant)
	assert.Equal(t, int64(110), m.MediaConvertCost)
	assert.Equal(t, int64(20), m.S3StorageCost)
	assert.Equal(t, int64(30), m.CloudFrontCost)

	// Track delivery updates running averages and per-variant counters.
	m.TrackVariantDelivery("720p_h264_1000", 100, 50, true, true)
	m.TrackVariantDelivery("720p_h264_1000", 200, 150, false, false)
	assert.Equal(t, int64(300), m.TotalBandwidthBytes)
	assert.Equal(t, int64(300), m.VariantBandwidth["720p_h264_1000"])
	assert.Equal(t, int64(100), m.VariantLatency["720p_h264_1000"])
	assert.InDelta(t, 0.5, m.VariantCacheHitRate["720p_h264_1000"], 0.00001)
	assert.InDelta(t, 0.05, m.VariantErrorRate["720p_h264_1000"], 0.00001)

	cost := m.VariantCosts["720p_h264_1000"]
	assert.Equal(t, int64(2), cost.DeliveryCount)
	assert.Equal(t, int64(300), cost.BandwidthBytes)
	assert.Equal(t, int64(150), cost.AverageLatencyMs)

	// Quality viewer tracking and popularity.
	m.AddQualityViewer("720p")
	m.AddQualityViewer("720p")
	m.AddQualityViewer("1080p")
	assert.Equal(t, 3, m.StreamingSessions)
	pop, viewers := m.GetMostPopularQuality()
	assert.Equal(t, "720p", pop)
	assert.Equal(t, 2, viewers)

	m.RemoveQualityViewer("720p")
	m.RemoveQualityViewer("720p")
	m.RemoveQualityViewer("720p") // should not go negative
	assert.Equal(t, 1, m.StreamingSessions)
}

func TestMediaAnalytics_GetCostEfficiencyMetrics(t *testing.T) {
	m := &MediaAnalytics{}
	m.AddVariantCost("720p", "h264", 1000, MediaVariantCost{
		ProcessingCost:   10,
		StorageCost:      20,
		BandwidthCost:    30,
		ViewerMinutes:    10,
		BandwidthBytes:   1024 * 1024 * 1024, // 1GB
		DeliveryCount:    2,
		CompressionRatio: 0.5,
		AverageLatencyMs: 25,
		ErrorRate:        0.1,
		CacheHitRate:     0.9,
	})

	m.StreamingSessions = 2
	m.TotalBandwidthBytes = 1024 * 1024 * 1024

	metrics := m.GetCostEfficiencyMetrics()
	assert.Contains(t, metrics, "720p_h264_1000")
	assert.Contains(t, metrics, "overall")
}

