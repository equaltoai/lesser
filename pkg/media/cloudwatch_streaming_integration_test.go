package media

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestStreamingCloudWatchMetrics(t *testing.T) {
	t.Run("model initialization and key updates", func(t *testing.T) {
		// Test the model directly
		metrics := &models.StreamingCloudWatchMetrics{}
		qualityMetrics := map[string]models.QualityMetric{
			"720p": {
				Quality:            "720p",
				ViewerCount:        400,
				ViewerPercentage:   0.4,
				BufferingRate:      0.05,
				AverageLatencyMs:   300,
				ErrorRate:          0.01,
				BitrateUtilization: 0.85,
				StartupTimeMs:      600,
			},
			"1080p": {
				Quality:            "1080p",
				ViewerCount:        600,
				ViewerPercentage:   0.6,
				BufferingRate:      0.03,
				AverageLatencyMs:   250,
				ErrorRate:          0.005,
				BitrateUtilization: 0.9,
				StartupTimeMs:      500,
			},
		}

		metrics.SetQualityBreakdown("test-media-123", qualityMetrics)

		assert.Equal(t, "test-media-123", metrics.MediaID)
		assert.Equal(t, "quality_breakdown", metrics.MetricType)
		assert.Equal(t, "STREAMING_METRICS#quality_breakdown", metrics.PK)
		assert.Contains(t, metrics.SK, "test-media-123#")
		assert.Equal(t, 2, len(metrics.QualityMetrics))
		assert.Equal(t, int64(400), metrics.QualityMetrics["720p"].ViewerCount)
		assert.Equal(t, int64(600), metrics.QualityMetrics["1080p"].ViewerCount)
		assert.False(t, metrics.IsExpired())
	})

	t.Run("geographic metrics", func(t *testing.T) {
		metrics := &models.StreamingCloudWatchMetrics{}
		geoMetrics := map[string]models.GeographicMetric{
			"US": {
				Region:             "US",
				ViewerCount:        800,
				ViewerPercentage:   0.6,
				AverageLatencyMs:   150,
				PreferredQuality:   "1080p",
				CacheHitRate:       0.9,
				BandwidthUsageMbps: 5.2,
			},
			"EU": {
				Region:             "EU",
				ViewerCount:        400,
				ViewerPercentage:   0.3,
				AverageLatencyMs:   200,
				PreferredQuality:   "720p",
				CacheHitRate:       0.85,
				BandwidthUsageMbps: 3.8,
			},
		}

		metrics.SetGeographicData("test-media-456", geoMetrics)

		assert.Equal(t, "test-media-456", metrics.MediaID)
		assert.Equal(t, "geographic_data", metrics.MetricType)
		assert.Equal(t, "STREAMING_METRICS#geographic_data", metrics.PK)
		assert.Equal(t, 2, len(metrics.GeographicMetrics))
		assert.Equal(t, int64(800), metrics.GeographicMetrics["US"].ViewerCount)
		assert.Equal(t, "1080p", metrics.GeographicMetrics["US"].PreferredQuality)
	})

	t.Run("best quality selection", func(t *testing.T) {
		metrics := &models.StreamingCloudWatchMetrics{}
		qualityMetrics := map[string]models.QualityMetric{
			"720p": {
				Quality:          "720p",
				BufferingRate:    0.2,  // High buffering
				AverageLatencyMs: 1000, // High latency
				ViewerPercentage: 0.3,
			},
			"1080p": {
				Quality:          "1080p",
				BufferingRate:    0.05, // Low buffering
				AverageLatencyMs: 300,  // Low latency
				ViewerPercentage: 0.7,  // Popular
			},
		}

		metrics.SetQualityBreakdown("test-media-789", qualityMetrics)

		bestQuality := metrics.GetBestQuality()
		assert.Equal(t, "1080p", bestQuality)
	})

	t.Run("quality adaptation decision", func(t *testing.T) {
		metrics := &models.StreamingCloudWatchMetrics{}
		qualityMetrics := map[string]models.QualityMetric{
			"1080p": {
				Quality:          "1080p",
				BufferingRate:    0.15, // High buffering - should adapt down
				AverageLatencyMs: 2500, // High latency
				ViewerPercentage: 0.5,
			},
			"720p": {
				Quality:          "720p",
				BufferingRate:    0.02, // Low buffering
				AverageLatencyMs: 200,  // Low latency
				ViewerPercentage: 0.4,
			},
		}

		metrics.SetQualityBreakdown("test-media-adapt", qualityMetrics)

		shouldAdapt, newQuality := metrics.ShouldAdaptQuality("1080p")
		assert.True(t, shouldAdapt)
		assert.Equal(t, "720p", newQuality)

		// Test good performance - should not adapt
		shouldAdapt2, _ := metrics.ShouldAdaptQuality("720p")
		assert.False(t, shouldAdapt2)
	})
}

func TestCloudWatchEnhancedStreamingService_Fallbacks(t *testing.T) {
	t.Run("fallback data generation", func(t *testing.T) {
		logger := zap.NewNop()

		// Create service without storage (will use fallbacks)
		service := &CloudWatchEnhancedStreamingService{
			logger:    logger,
			namespace: "Lesser/Streaming",
		}
		service.cloudWatch = nil // Simulate no CloudWatch client

		totalViews := int64(1000)

		// Test fallback quality breakdown
		fallbackQuality := service.generateFallbackQualityBreakdown(totalViews)
		assert.Equal(t, int64(300), fallbackQuality["480p"])  // 30%
		assert.Equal(t, int64(400), fallbackQuality["720p"])  // 40%
		assert.Equal(t, int64(250), fallbackQuality["1080p"]) // 25%
		assert.Equal(t, int64(50), fallbackQuality["4k"])     // 5%

		// Test fallback geographic data
		fallbackGeo := service.generateFallbackGeographicData(totalViews)
		assert.Equal(t, int64(600), fallbackGeo["US"]) // 60%
		assert.Equal(t, int64(250), fallbackGeo["EU"]) // 25%
		assert.Equal(t, int64(150), fallbackGeo["AS"]) // 15%

		// Test region-based quality preferences
		assert.Equal(t, "1080p", service.getPreferredQualityForRegion("US"))
		assert.Equal(t, "1080p", service.getPreferredQualityForRegion("EU"))
		assert.Equal(t, "720p", service.getPreferredQualityForRegion("AS"))
		assert.Equal(t, "480p", service.getPreferredQualityForRegion("AF"))
	})
}

func TestStreamingServiceEnhancement(t *testing.T) {
	t.Run("streaming service with CloudWatch integration", func(t *testing.T) {
		// Test that the streaming service can be created and used without CloudWatch enhancement
		service := &streamingService{
			distributionDomain: "cdn.example.com",
			keyPairID:          "test-key",
			privateKey:         []byte("test-private-key"),
			cloudWatchEnhanced: nil, // No CloudWatch enhancement
		}

		// Test helper methods
		totalViews := int64(1000)

		// Without CloudWatch enhancement, should return fallback data
		geoData := service.getGeographicData(totalViews)
		assert.Equal(t, int64(600), geoData["US"])
		assert.Equal(t, int64(250), geoData["EU"])
		assert.Equal(t, int64(150), geoData["AS"])

		peakViewers := service.getPeakConcurrentViewers(totalViews)
		assert.Equal(t, int64(41), peakViewers) // totalViews / 24
	})
}
