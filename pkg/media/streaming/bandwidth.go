package streaming

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	storage     core.RepositoryStorage
	logger      *zap.Logger
	costTracker CostTracker

	// In-memory cache for active sessions
	sessionCache sync.Map
	cacheTTL     time.Duration
}

// NewBandwidthTracker creates a new bandwidth tracker
func NewBandwidthTracker(storage core.RepositoryStorage, logger *zap.Logger, costTracker CostTracker) *BandwidthTracker {
	return &BandwidthTracker{
		storage:     storage,
		logger:      logger,
		costTracker: costTracker,
		cacheTTL:    5 * time.Minute,
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

	// Track cost (simplified since we're not doing direct DynamoDB operations)
	if bt.costTracker != nil {
		bt.costTracker.TrackDynamoWrite(1)
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
func (bt *BandwidthTracker) GetBandwidthStats(_ context.Context, userID string) (*BandwidthStats, error) {
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

	// Track cost (simplified)
	if bt.costTracker != nil {
		bt.costTracker.TrackDynamoRead(1)
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

// RecordBandwidthMeasurement records a bandwidth measurement sample (simplified)
func (bt *BandwidthTracker) RecordBandwidthMeasurement(_ context.Context, userID string, bandwidth int) error {
	// Update in-memory cache
	bt.updateCache(userID, int64(bandwidth), time.Now())

	// Track cost (simplified)
	if bt.costTracker != nil {
		bt.costTracker.TrackDynamoWrite(1)
	}

	return nil
}

// GetBandwidthHistory retrieves bandwidth measurement history (simplified)
func (bt *BandwidthTracker) GetBandwidthHistory(_ context.Context, userID string, duration time.Duration) ([]BandwidthMeasurement, error) {
	// Return empty history for now - in a full implementation this could query analytics data
	bt.logger.Debug("bandwidth history requested",
		zap.String("userID", userID),
		zap.Duration("duration", duration))

	// Track cost (simplified)
	if bt.costTracker != nil {
		bt.costTracker.TrackDynamoRead(1)
	}

	return []BandwidthMeasurement{}, nil
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

	// Update stats
	stats.TotalBytes += bytesTransferred
	stats.SessionBytes += bytesTransferred
	stats.LastMeasurement = now
	stats.lastUpdate = now

	// Calculate bandwidth in bits per second
	if !stats.LastMeasurement.IsZero() {
		duration := now.Sub(stats.LastMeasurement)
		if duration > 0 {
			bandwidth := int(float64(bytesTransferred*8) / duration.Seconds()) // Convert to bits per second

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
