package streaming

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// CostTracker interface for tracking AWS costs
type CostTracker interface {
	TrackDynamoRead(units int)
	TrackDynamoWrite(units int)
}

// BandwidthTracker tracks bandwidth usage for users
type BandwidthTracker struct {
	storage        core.RepositoryStorage
	logger         *zap.Logger
	costTracker    CostTracker
	unifiedTracker *cost.UnifiedTracker
	tableName      string
	cloudWatch     *cloudwatch.Client

	// In-memory cache for active sessions
	sessionCache sync.Map
	cacheTTL     time.Duration
	namespace    string
}

// NewBandwidthTracker creates a new bandwidth tracker
func NewBandwidthTracker(storage core.RepositoryStorage, logger *zap.Logger, costTracker CostTracker, cloudWatch *cloudwatch.Client) *BandwidthTracker {
	cfg := config.Get()
	tableName := cfg.DynamoTableName
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main"
	}

	// Create unified tracker for centralized cost tracking
	unifiedTracker := cost.NewRepositoryTracker(cloudWatch, logger, "BandwidthTracker", "", "")

	return &BandwidthTracker{
		storage:        storage,
		logger:         logger,
		costTracker:    costTracker,
		unifiedTracker: unifiedTracker,
		tableName:      tableName,
		cloudWatch:     cloudWatch,
		cacheTTL:       5 * time.Minute,
		namespace:      "Lesser/Streaming/Bandwidth",
	}
}

// TrackBandwidth records bandwidth usage for a user
func (bt *BandwidthTracker) TrackBandwidth(ctx context.Context, userID string, bytesTransferred int64) error {
	now := time.Now()

	// Update in-memory cache first for real-time performance
	bt.updateCache(userID, bytesTransferred, now)

	// Record bandwidth event using analytics storage
	err := bt.storage.Analytics().RecordMediaEvent(ctx, "bandwidth_usage", userID, userID)
	if err != nil {
		bt.logger.Error("failed to track bandwidth event",
			zap.String("user", userID),
			zap.Int64("bytes", bytesTransferred),
			zap.Error(err))
		// Don't return error to avoid breaking streaming - just log it
	}

	// Also publish to CloudWatch for real-time bandwidth tracking
	bt.publishBandwidthMetric(ctx, userID, bytesTransferred, now)

	// Track analytics operation cost using centralized tracker
	if err := bt.unifiedTracker.TrackDynamoWrite(ctx, bt.tableName, 1); err != nil {
		bt.logger.Warn("failed to track cost", zap.Error(err))
	}

	// Log significant bandwidth usage
	if bytesTransferred > 10*1024*1024 { // 10MB
		bt.logger.Info("significant bandwidth usage",
			zap.String("user", userID),
			zap.Int64("bytes", bytesTransferred),
			zap.String("size_mb", fmt.Sprintf("%.2f", float64(bytesTransferred)/(1024*1024))))
	}

	return nil
}

// GetBandwidthStats retrieves bandwidth statistics for a user
func (bt *BandwidthTracker) GetBandwidthStats(ctx context.Context, userID string) (*BandwidthStats, error) {
	// Check cache first
	if cached, ok := bt.sessionCache.Load(userID); ok {
		if stats, ok := cached.(*cachedBandwidthStats); ok && time.Since(stats.lastUpdate) < bt.cacheTTL {
			return &stats.BandwidthStats, nil
		}
	}

	// For now, return default stats since the complex DynamoDB logic is simplified
	// In a full implementation, this could query aggregated analytics data
	stats := &BandwidthStats{
		UserID:            userID,
		TotalBytes:        0,
		SessionBytes:      0,
		AverageBandwidth:  5000, // Default to 5 Mbps for reasonable quality selection
		PeakBandwidth:     0,
		LastMeasurement:   time.Now(),
		MeasurementWindow: 5 * time.Minute,
	}

	// Track operation cost
	if bt.costTracker != nil {
		// Track cost using centralized tracker
		if err := bt.unifiedTracker.TrackDynamoRead(ctx, bt.tableName, 1); err != nil {
			bt.logger.Warn("failed to track cost", zap.Error(err))
		}
	}

	return stats, nil
}

// GetOptimalQuality determines the best quality based on user's bandwidth
func (bt *BandwidthTracker) GetOptimalQuality(ctx context.Context, userID string, availableBandwidth int) Quality {
	// If availableBandwidth is provided, use it directly
	if availableBandwidth > 0 {
		return bt.selectQualityByBandwidth(availableBandwidth)
	}

	// Get user's bandwidth stats
	stats, err := bt.GetBandwidthStats(ctx, userID)
	if err != nil {
		bt.logger.Warn("failed to get bandwidth stats, using default quality",
			zap.String("user", userID),
			zap.Error(err))
		return Quality720p // Safe default
	}

	// Use average bandwidth with safety margin
	safeBandwidth := int(float64(stats.AverageBandwidth) * 0.8) // 80% of average

	return bt.selectQualityByBandwidth(safeBandwidth)
}

// RecordBandwidthMeasurement records a bandwidth measurement sample with CloudWatch integration
func (bt *BandwidthTracker) RecordBandwidthMeasurement(ctx context.Context, userID string, bandwidth int) error {
	now := time.Now()

	// Update in-memory cache
	bt.updateCache(userID, int64(bandwidth), now)

	// Publish real-time bandwidth measurement to CloudWatch
	bt.publishBandwidthMetric(ctx, userID, int64(bandwidth), now)

	// Track operation cost
	if bt.costTracker != nil {
		// Track cost using centralized tracker
		if err := bt.unifiedTracker.TrackDynamoWrite(ctx, bt.tableName, 1); err != nil {
			bt.logger.Warn("failed to track cost", zap.Error(err))
		}
	}

	return nil
}

// GetBandwidthHistory retrieves bandwidth measurement history from CloudWatch
func (bt *BandwidthTracker) GetBandwidthHistory(ctx context.Context, userID string, duration time.Duration) ([]BandwidthMeasurement, error) {
	if bt.cloudWatch == nil {
		bt.logger.Warn("CloudWatch client not available, returning empty history")
		return []BandwidthMeasurement{}, nil
	}

	endTime := time.Now()
	startTime := endTime.Add(-duration)

	// Query CloudWatch for bandwidth measurements
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(bt.namespace),
		MetricName: aws.String("BytesTransferred"),
		Dimensions: []types.Dimension{
			{
				Name:  aws.String("UserID"),
				Value: aws.String(userID),
			},
		},
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(60), // 1-minute intervals
		Statistics: []types.Statistic{types.StatisticSum, types.StatisticAverage},
	}

	result, err := bt.cloudWatch.GetMetricStatistics(ctx, input)
	if err != nil {
		bt.logger.Error("failed to get bandwidth history from CloudWatch",
			zap.String("userID", userID),
			zap.Duration("duration", duration),
			zap.Error(err))
		return []BandwidthMeasurement{}, nil
	}

	// Convert CloudWatch datapoints to BandwidthMeasurement
	measurements := make([]BandwidthMeasurement, 0, len(result.Datapoints))
	for _, datapoint := range result.Datapoints {
		if datapoint.Sum != nil && datapoint.Timestamp != nil {
			// Convert bytes to bandwidth (kbps)
			// Assume 1-minute period, convert bytes to bits and then to kbps
			bandwidthKbps := int((*datapoint.Sum * 8) / 1000 / 60) // bytes to kbps over 60 seconds

			measurements = append(measurements, BandwidthMeasurement{
				UserID:    userID,
				Bandwidth: bandwidthKbps,
				Timestamp: *datapoint.Timestamp,
			})
		}
	}

	bt.logger.Debug("retrieved bandwidth history from CloudWatch",
		zap.String("userID", userID),
		zap.Duration("duration", duration),
		zap.Int("measurements", len(measurements)))

	// Track cost (CloudWatch query)
	if bt.costTracker != nil {
		// Track cost using centralized tracker
		if err := bt.unifiedTracker.TrackDynamoRead(ctx, bt.tableName, 1); err != nil {
			bt.logger.Warn("failed to track cost", zap.Error(err))
		}
	}

	return measurements, nil
}

// Helper types and methods

type cachedBandwidthStats struct {
	BandwidthStats
	lastUpdate time.Time
}

// updateCache updates the in-memory cache with current bandwidth data
func (bt *BandwidthTracker) updateCache(userID string, bytesTransferred int64, now time.Time) {
	// Load or create cached stats
	var stats *cachedBandwidthStats
	if cached, ok := bt.sessionCache.Load(userID); ok {
		stats = cached.(*cachedBandwidthStats)
	} else {
		stats = &cachedBandwidthStats{
			BandwidthStats: BandwidthStats{
				UserID:            userID,
				TotalBytes:        0,
				SessionBytes:      0,
				AverageBandwidth:  0,
				PeakBandwidth:     0,
				LastMeasurement:   now,
				MeasurementWindow: 5 * time.Minute,
			},
			lastUpdate: now,
		}
	}

	previousMeasurement := stats.LastMeasurement

	// Update stats
	stats.TotalBytes += bytesTransferred
	stats.SessionBytes += bytesTransferred
	stats.LastMeasurement = now
	stats.lastUpdate = now

	// Calculate bandwidth in Kbps
	if !previousMeasurement.IsZero() {
		duration := now.Sub(previousMeasurement)
		if duration > 0 {
			bandwidth := int(float64(bytesTransferred*8)/duration.Seconds()/1000) // Convert to Kbps

			// Update average (simple moving average)
			if stats.AverageBandwidth == 0 {
				stats.AverageBandwidth = bandwidth
			} else {
				stats.AverageBandwidth = (stats.AverageBandwidth + bandwidth) / 2
			}

			// Update peak
			if bandwidth > stats.PeakBandwidth {
				stats.PeakBandwidth = bandwidth
			}
		}
	}

	// Store back in cache
	bt.sessionCache.Store(userID, stats)
}

// selectQualityByBandwidth selects appropriate quality based on available bandwidth
func (bt *BandwidthTracker) selectQualityByBandwidth(bandwidth int) Quality {
	// Bandwidth in Kbps - conservative estimates
	switch {
	case bandwidth >= 25000: // 25 Mbps
		return Quality4K
	case bandwidth >= 8000: // 8 Mbps
		return Quality1080p
	case bandwidth >= 3000: // 3 Mbps
		return Quality720p
	case bandwidth >= 1000: // 1 Mbps
		return Quality480p
	case bandwidth >= 500: // 500 Kbps
		return Quality360p
	default:
		return Quality240p
	}
}

// publishBandwidthMetric publishes bandwidth data to CloudWatch
func (bt *BandwidthTracker) publishBandwidthMetric(_ context.Context, userID string, bytesTransferred int64, timestamp time.Time) {
	if bt.cloudWatch == nil {
		return
	}

	// Create bandwidth metric
	metricData := []types.MetricDatum{
		{
			MetricName: aws.String("BytesTransferred"),
			Dimensions: []types.Dimension{
				{
					Name:  aws.String("UserID"),
					Value: aws.String(userID),
				},
			},
			Timestamp: aws.Time(timestamp),
			Value:     aws.Float64(float64(bytesTransferred)),
			Unit:      types.StandardUnitBytes,
		},
	}

	// Also publish bandwidth rate (kbps)
	if cached, ok := bt.sessionCache.Load(userID); ok {
		if stats, ok := cached.(*cachedBandwidthStats); ok {
			if stats.AverageBandwidth > 0 {
				metricData = append(metricData, types.MetricDatum{
					MetricName: aws.String("BandwidthKbps"),
					Dimensions: []types.Dimension{
						{
							Name:  aws.String("UserID"),
							Value: aws.String(userID),
						},
					},
					Timestamp: aws.Time(timestamp),
					Value:     aws.Float64(float64(stats.AverageBandwidth)),
					Unit:      types.StandardUnitCount,
				})
			}
		}
	}

	// Publish metrics to CloudWatch (async)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		input := &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(bt.namespace),
			MetricData: metricData,
		}

		_, err := bt.cloudWatch.PutMetricData(ctx, input)
		if err != nil {
			bt.logger.Warn("failed to publish bandwidth metrics to CloudWatch",
				zap.String("userID", userID),
				zap.Error(err))
		}
	}()
}
