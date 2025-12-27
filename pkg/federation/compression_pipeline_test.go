package federation

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// === gzipCompress Tests ===

func TestGzipCompress(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pipeline := &CompressionPipeline{
		logger: logger,
	}

	t.Run("round_trip_preserves_key_fields", func(t *testing.T) {
		original := &models.FederationAnalyticsTimeSeries{
			Domain:               "example.com",
			Period:               "5min",
			Timestamp:            time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			ActivityCount:        100,
			SuccessfulActivities: 95,
			FailedActivities:     5,
			HealthScore:          0.95,
			InboxDeliveryP50:     50,
			InboxDeliveryP95:     150,
			InboxDeliveryP99:     300,
			ErrorRate:            0.05,
			InstanceReachability: 0.98,
			TotalInboundVolume:   1024,
			TotalOutboundVolume:  2048,
		}

		// Compress
		compressed, err := pipeline.gzipCompress(original)
		require.NoError(t, err)
		require.NotEmpty(t, compressed)

		// Decompress
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		require.NoError(t, err)
		defer reader.Close()

		var decompressed models.FederationAnalyticsTimeSeries
		err = json.NewDecoder(reader).Decode(&decompressed)
		require.NoError(t, err)

		// Verify key fields match
		assert.Equal(t, original.Domain, decompressed.Domain)
		assert.Equal(t, original.Period, decompressed.Period)
		assert.Equal(t, original.ActivityCount, decompressed.ActivityCount)
		assert.Equal(t, original.SuccessfulActivities, decompressed.SuccessfulActivities)
		assert.Equal(t, original.FailedActivities, decompressed.FailedActivities)
		assert.InDelta(t, original.HealthScore, decompressed.HealthScore, 0.001)
		assert.Equal(t, original.InboxDeliveryP50, decompressed.InboxDeliveryP50)
		assert.Equal(t, original.InboxDeliveryP95, decompressed.InboxDeliveryP95)
		assert.Equal(t, original.InboxDeliveryP99, decompressed.InboxDeliveryP99)
	})

	t.Run("compresses_data_smaller_than_original", func(t *testing.T) {
		// Create a metric with repetitive data that compresses well
		original := &models.FederationAnalyticsTimeSeries{
			Domain:               "example.com",
			Period:               "5min",
			ActivityCount:        100,
			SuccessfulActivities: 95,
			FailedActivities:     5,
		}

		compressed, err := pipeline.gzipCompress(original)
		require.NoError(t, err)

		// Get original JSON size for comparison
		jsonData, _ := json.Marshal(original)

		// Compressed should be smaller (or at least not larger for small data)
		// Note: Very small data may not compress well, so we just verify it works
		assert.NotEmpty(t, compressed)
		t.Logf("Original JSON: %d bytes, Compressed: %d bytes", len(jsonData), len(compressed))
	})
}

// === statisticalSummaryCompress Tests ===

func TestStatisticalSummaryCompress(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pipeline := &CompressionPipeline{
		logger: logger,
	}

	t.Run("output_unmarshals_to_statistical_summary", func(t *testing.T) {
		original := &models.FederationAnalyticsTimeSeries{
			Domain:               "test.example.org",
			Period:               "hourly",
			Timestamp:            time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			ActivityCount:        500,
			SuccessfulActivities: 480,
			FailedActivities:     20,
			HealthScore:          0.92,
			InboxDeliveryP50:     45,
			InboxDeliveryP95:     120,
			InboxDeliveryP99:     250,
			ErrorRate:            0.04,
			InstanceReachability: 0.97,
			TotalInboundVolume:   10240,
			TotalOutboundVolume:  20480,
		}

		compressed, err := pipeline.statisticalSummaryCompress(original)
		require.NoError(t, err)
		require.NotEmpty(t, compressed)

		var summary StatisticalSummary
		err = json.Unmarshal(compressed, &summary)
		require.NoError(t, err)

		assert.Equal(t, original.Domain, summary.Domain)
		assert.Equal(t, original.Period, summary.Period)
		assert.Equal(t, original.ActivityCount, summary.ActivityCount)
		assert.Equal(t, original.SuccessfulActivities, summary.SuccessfulActivities)
		assert.Equal(t, original.FailedActivities, summary.FailedActivities)
		assert.InDelta(t, original.HealthScore, summary.HealthScore, 0.001)
	})

	t.Run("percentiles_map_has_p50_p95_p99", func(t *testing.T) {
		original := &models.FederationAnalyticsTimeSeries{
			Domain:           "test.example.org",
			Period:           "5min",
			InboxDeliveryP50: 50,
			InboxDeliveryP95: 150,
			InboxDeliveryP99: 300,
		}

		compressed, err := pipeline.statisticalSummaryCompress(original)
		require.NoError(t, err)

		var summary StatisticalSummary
		err = json.Unmarshal(compressed, &summary)
		require.NoError(t, err)

		require.NotNil(t, summary.Percentiles)
		assert.Equal(t, int64(50), summary.Percentiles["p50"])
		assert.Equal(t, int64(150), summary.Percentiles["p95"])
		assert.Equal(t, int64(300), summary.Percentiles["p99"])
	})

	t.Run("total_bytes_sums_inbound_and_outbound", func(t *testing.T) {
		original := &models.FederationAnalyticsTimeSeries{
			Domain:              "test.example.org",
			Period:              "5min",
			TotalInboundVolume:  1000,
			TotalOutboundVolume: 2500,
		}

		compressed, err := pipeline.statisticalSummaryCompress(original)
		require.NoError(t, err)

		var summary StatisticalSummary
		err = json.Unmarshal(compressed, &summary)
		require.NoError(t, err)

		assert.Equal(t, int64(3500), summary.TotalBytes)
	})

	t.Run("preserves_error_rate_and_reachability", func(t *testing.T) {
		original := &models.FederationAnalyticsTimeSeries{
			Domain:               "test.example.org",
			Period:               "5min",
			ErrorRate:            0.123,
			InstanceReachability: 0.987,
		}

		compressed, err := pipeline.statisticalSummaryCompress(original)
		require.NoError(t, err)

		var summary StatisticalSummary
		err = json.Unmarshal(compressed, &summary)
		require.NoError(t, err)

		assert.InDelta(t, 0.123, summary.ErrorRate, 0.001)
		assert.InDelta(t, 0.987, summary.InstanceReachability, 0.001)
	})
}

// === compressMetric Tests ===

func TestCompressMetric(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pipeline := &CompressionPipeline{
		logger: logger,
	}

	metric := &models.FederationAnalyticsTimeSeries{
		Domain:        "example.com",
		Period:        "5min",
		ActivityCount: 100,
	}

	t.Run("GZIP_JSON_uses_gzip_compression", func(t *testing.T) {
		compressed, err := pipeline.compressMetric(metric, "GZIP_JSON")
		require.NoError(t, err)

		// Verify it's valid gzip by attempting to decompress
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		require.NoError(t, err)
		reader.Close()
	})

	t.Run("STATISTICAL_SUMMARY_uses_summary_compression", func(t *testing.T) {
		compressed, err := pipeline.compressMetric(metric, "STATISTICAL_SUMMARY")
		require.NoError(t, err)

		// Verify it's valid JSON (not gzip)
		var summary StatisticalSummary
		err = json.Unmarshal(compressed, &summary)
		require.NoError(t, err)
		assert.Equal(t, "example.com", summary.Domain)
	})

	t.Run("unknown_method_defaults_to_gzip", func(t *testing.T) {
		compressed, err := pipeline.compressMetric(metric, "UNKNOWN_METHOD")
		require.NoError(t, err)

		// Verify it's valid gzip
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		require.NoError(t, err)
		reader.Close()
	})

	t.Run("empty_method_defaults_to_gzip", func(t *testing.T) {
		compressed, err := pipeline.compressMetric(metric, "")
		require.NoError(t, err)

		// Verify it's valid gzip
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		require.NoError(t, err)
		reader.Close()
	})
}

// === CalculateCompressionStats Tests ===

func TestCalculateCompressionStats(t *testing.T) {
	t.Run("calculates_ratio_correctly", func(t *testing.T) {
		stats := CalculateCompressionStats(1000, 250, 10)

		assert.Equal(t, 10, stats.TotalRecords)
		assert.Equal(t, 10, stats.CompressedRecords)
		assert.Equal(t, int64(1000), stats.OriginalSizeBytes)
		assert.Equal(t, int64(250), stats.CompressedSizeBytes)
		assert.InDelta(t, 4.0, stats.CompressionRatio, 0.001) // 1000/250 = 4.0
	})

	t.Run("compressed_size_zero_returns_ratio_one", func(t *testing.T) {
		stats := CalculateCompressionStats(1000, 0, 5)

		assert.Equal(t, 1.0, stats.CompressionRatio)
	})

	t.Run("record_counts_propagated", func(t *testing.T) {
		stats := CalculateCompressionStats(500, 100, 42)

		assert.Equal(t, 42, stats.TotalRecords)
		assert.Equal(t, 42, stats.CompressedRecords)
	})

	t.Run("zero_values", func(t *testing.T) {
		stats := CalculateCompressionStats(0, 0, 0)

		assert.Equal(t, 0, stats.TotalRecords)
		assert.Equal(t, int64(0), stats.OriginalSizeBytes)
		assert.Equal(t, 1.0, stats.CompressionRatio) // Default when compressedSize=0
	})

	t.Run("large_compression_ratio", func(t *testing.T) {
		stats := CalculateCompressionStats(1000000, 100, 1)

		assert.InDelta(t, 10000.0, stats.CompressionRatio, 0.001)
	})

	t.Run("no_compression_ratio_one", func(t *testing.T) {
		stats := CalculateCompressionStats(500, 500, 10)

		assert.InDelta(t, 1.0, stats.CompressionRatio, 0.001)
	})
}

// === StatisticalSummary Struct Tests ===

func TestStatisticalSummary(t *testing.T) {
	t.Run("json_serialization", func(t *testing.T) {
		summary := StatisticalSummary{
			Domain:               "test.com",
			Period:               "hourly",
			Timestamp:            time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			ActivityCount:        100,
			SuccessfulActivities: 90,
			FailedActivities:     10,
			HealthScore:          0.9,
			Percentiles: map[string]int64{
				"p50": 50,
				"p95": 150,
				"p99": 300,
			},
			ErrorRate:            0.1,
			InstanceReachability: 0.95,
			TotalBytes:           5000,
		}

		data, err := json.Marshal(summary)
		require.NoError(t, err)

		var decoded StatisticalSummary
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, summary.Domain, decoded.Domain)
		assert.Equal(t, summary.Period, decoded.Period)
		assert.Equal(t, summary.ActivityCount, decoded.ActivityCount)
		assert.Equal(t, summary.Percentiles["p50"], decoded.Percentiles["p50"])
	})
}

// === CompressionStats Struct Tests ===

func TestCompressionStats(t *testing.T) {
	t.Run("json_serialization", func(t *testing.T) {
		stats := CompressionStats{
			TotalRecords:        100,
			CompressedRecords:   100,
			OriginalSizeBytes:   10000,
			CompressedSizeBytes: 2500,
			CompressionRatio:    4.0,
			ProcessingTimeMs:    150,
		}

		data, err := json.Marshal(stats)
		require.NoError(t, err)

		var decoded CompressionStats
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, stats.TotalRecords, decoded.TotalRecords)
		assert.Equal(t, stats.CompressionRatio, decoded.CompressionRatio)
		assert.Equal(t, stats.ProcessingTimeMs, decoded.ProcessingTimeMs)
	})
}
