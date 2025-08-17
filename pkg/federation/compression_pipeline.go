package federation

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// CompressionPipeline implements the progressive compression strategy
// from federation-analytics-guidance.md for time series data
type CompressionPipeline struct {
	federationRepo *repositories.FederationRepository
	logger         *zap.Logger
}

// NewCompressionPipeline creates a new compression pipeline
func NewCompressionPipeline(federationRepo *repositories.FederationRepository, logger *zap.Logger) *CompressionPipeline {
	return &CompressionPipeline{
		federationRepo: federationRepo,
		logger:         logger,
	}
}

// CompressOldData implements the multi-level compression strategy
func (c *CompressionPipeline) CompressOldData(ctx context.Context) error {
	now := time.Now()

	// Level 1: Compress 24h old 5-minute data (GZIP_JSON)
	cutoff24h := now.Add(-24 * time.Hour)
	if err := c.compressTimeSeriesData(ctx, "5min", cutoff24h, "GZIP_JSON"); err != nil {
		c.logger.Error("Failed to compress 24h old data", zap.Error(err))
		return fmt.Errorf("failed to compress 24h old data: %w", err)
	}

	// Level 2: Archive 7-day old data to S3 (would integrate with S3 in production)
	cutoff7d := now.Add(-7 * 24 * time.Hour)
	if err := c.archiveToS3(ctx, cutoff7d); err != nil {
		c.logger.Error("Failed to archive 7d old data", zap.Error(err))
		return fmt.Errorf("failed to archive 7d old data: %w", err)
	}

	c.logger.Info("Compression pipeline completed",
		zap.Time("24h_cutoff", cutoff24h),
		zap.Time("7d_cutoff", cutoff7d))

	return nil
}

// compressTimeSeriesData compresses time series records older than the cutoff
func (c *CompressionPipeline) compressTimeSeriesData(ctx context.Context, period string, cutoff time.Time, method string) error {
	// Get old time series data (would need to implement this query in the repo)
	endTime := cutoff
	startTime := cutoff.Add(-24 * time.Hour) // Process 24 hours at a time

	// Get data by period across all domains
	oldMetrics, err := c.federationRepo.GetDetailedMetricsByPeriod(ctx, period, startTime, endTime, 1000)
	if err != nil {
		return fmt.Errorf("failed to get old metrics: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("oldMetrics", oldMetrics); err != nil {
		return nil // Nothing to compress
	}

	compressed := 0
	for _, metric := range oldMetrics {
		// Compress the metric data
		compressedData, err := c.compressMetric(metric, method)
		if err != nil {
			c.logger.Warn("Failed to compress metric",
				zap.String("domain", metric.Domain),
				zap.Time("timestamp", metric.Timestamp),
				zap.Error(err))
			continue
		}

		// Update the record with compressed data
		metric.CompressedData = compressedData

		// Store the compressed version
		if err := c.federationRepo.StoreDetailedFederationMetrics(ctx, metric); err != nil {
			c.logger.Warn("Failed to store compressed metric",
				zap.String("domain", metric.Domain),
				zap.Time("timestamp", metric.Timestamp),
				zap.Error(err))
			continue
		}

		compressed++
	}

	c.logger.Info("Compressed time series data",
		zap.String("period", period),
		zap.String("method", method),
		zap.Int("compressed", compressed),
		zap.Int("total", len(oldMetrics)),
		zap.Time("cutoff", cutoff))

	return nil
}

// compressMetric compresses a single time series metric using the specified method
func (c *CompressionPipeline) compressMetric(metric *models.FederationAnalyticsTimeSeries, method string) ([]byte, error) {
	switch method {
	case "GZIP_JSON":
		return c.gzipCompress(metric)
	case "STATISTICAL_SUMMARY":
		return c.statisticalSummaryCompress(metric)
	default:
		return c.gzipCompress(metric) // Default fallback
	}
}

// gzipCompress compresses the metric using GZIP
func (c *CompressionPipeline) gzipCompress(metric *models.FederationAnalyticsTimeSeries) ([]byte, error) {
	// Convert to JSON
	jsonData, err := json.Marshal(metric)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metric: %w", err)
	}

	// Compress with GZIP
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	if _, err := gzWriter.Write(jsonData); err != nil {
		return nil, fmt.Errorf("failed to write gzip data: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	compressionRatio := float64(len(jsonData)) / float64(buf.Len())
	c.logger.Debug("GZIP compression completed",
		zap.String("domain", metric.Domain),
		zap.Int("original_size", len(jsonData)),
		zap.Int("compressed_size", buf.Len()),
		zap.Float64("compression_ratio", compressionRatio))

	return buf.Bytes(), nil
}

// statisticalSummaryCompress creates a statistical summary for high compression
func (c *CompressionPipeline) statisticalSummaryCompress(metric *models.FederationAnalyticsTimeSeries) ([]byte, error) {
	// Create a statistical summary that keeps only essential data
	summary := StatisticalSummary{
		Domain:               metric.Domain,
		Period:               metric.Period,
		Timestamp:            metric.Timestamp,
		ActivityCount:        metric.ActivityCount,
		SuccessfulActivities: metric.SuccessfulActivities,
		FailedActivities:     metric.FailedActivities,
		HealthScore:          metric.HealthScore,

		// Statistical aggregates
		Percentiles: map[string]int64{
			"p50": metric.InboxDeliveryP50,
			"p95": metric.InboxDeliveryP95,
			"p99": metric.InboxDeliveryP99,
		},

		// Key ratios
		ErrorRate:            metric.ErrorRate,
		InstanceReachability: metric.InstanceReachability,

		// Volume summary
		TotalBytes: metric.TotalInboundVolume + metric.TotalOutboundVolume,
	}

	// Serialize the summary
	return json.Marshal(summary)
}

// archiveToS3 simulates archiving old data to S3 (would implement actual S3 integration)
func (c *CompressionPipeline) archiveToS3(_ context.Context, cutoff time.Time) error {
	// In production, this would:
	// 1. Export old time series data to Parquet format
	// 2. Upload to S3 with Intelligent Tiering
	// 3. Delete the DynamoDB records (TTL handles this automatically)
	// 4. Update lifecycle policies for progressive archival

	c.logger.Info("S3 archival simulation",
		zap.Time("cutoff", cutoff),
		zap.String("format", "parquet"),
		zap.String("storage_class", "INTELLIGENT_TIERING"))

	// Simulate archival process
	// This would be implemented with actual S3 SDK calls
	return nil
}

// StatisticalSummary represents a compressed statistical summary of time series data
type StatisticalSummary struct {
	Domain               string           `json:"domain"`
	Period               string           `json:"period"`
	Timestamp            time.Time        `json:"timestamp"`
	ActivityCount        int64            `json:"activity_count"`
	SuccessfulActivities int64            `json:"successful_activities"`
	FailedActivities     int64            `json:"failed_activities"`
	HealthScore          float64          `json:"health_score"`
	Percentiles          map[string]int64 `json:"percentiles"`
	ErrorRate            float64          `json:"error_rate"`
	InstanceReachability float64          `json:"instance_reachability"`
	TotalBytes           int64            `json:"total_bytes"`
}

// CompressionStats tracks compression effectiveness
type CompressionStats struct {
	TotalRecords        int     `json:"total_records"`
	CompressedRecords   int     `json:"compressed_records"`
	OriginalSizeBytes   int64   `json:"original_size_bytes"`
	CompressedSizeBytes int64   `json:"compressed_size_bytes"`
	CompressionRatio    float64 `json:"compression_ratio"`
	ProcessingTimeMs    int64   `json:"processing_time_ms"`
}

// CalculateCompressionStats calculates compression effectiveness metrics
func CalculateCompressionStats(originalSize, compressedSize int64, recordCount int) *CompressionStats {
	ratio := 1.0
	if compressedSize > 0 {
		ratio = float64(originalSize) / float64(compressedSize)
	}

	return &CompressionStats{
		TotalRecords:        recordCount,
		CompressedRecords:   recordCount,
		OriginalSizeBytes:   originalSize,
		CompressedSizeBytes: compressedSize,
		CompressionRatio:    ratio,
	}
}
