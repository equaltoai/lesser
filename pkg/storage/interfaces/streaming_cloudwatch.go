// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// StreamingCloudWatchRepository defines the interface for streaming CloudWatch metrics caching.
// This handles caching of streaming-related CloudWatch metrics for performance optimization.
type StreamingCloudWatchRepository interface {
	// ===== Quality Metrics Operations =====

	// GetQualityBreakdown retrieves cached quality breakdown metrics for a media item
	GetQualityBreakdown(ctx context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error)

	// CacheQualityBreakdown stores quality breakdown metrics in cache
	CacheQualityBreakdown(ctx context.Context, mediaID string, qualityMetrics map[string]models.QualityMetric) error

	// ===== Geographic Metrics Operations =====

	// GetGeographicData retrieves cached geographic distribution metrics
	GetGeographicData(ctx context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error)

	// CacheGeographicData stores geographic distribution metrics in cache
	CacheGeographicData(ctx context.Context, mediaID string, geoMetrics map[string]models.GeographicMetric) error

	// ===== Concurrent Viewer Operations =====

	// GetConcurrentViewers retrieves cached concurrent viewer metrics
	GetConcurrentViewers(ctx context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error)

	// CacheConcurrentViewers stores concurrent viewer metrics in cache
	CacheConcurrentViewers(ctx context.Context, mediaID string, concurrentMetrics models.ConcurrentViewerMetrics) error

	// ===== Performance Metrics Operations =====

	// GetPerformanceMetrics retrieves cached performance metrics
	GetPerformanceMetrics(ctx context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error)

	// CachePerformanceMetrics stores performance metrics in cache
	CachePerformanceMetrics(ctx context.Context, mediaID string, perfMetrics models.StreamingPerformanceMetrics) error

	// ===== Aggregate Operations =====

	// GetAllCachedMetrics retrieves all cached metrics for a media item
	GetAllCachedMetrics(ctx context.Context, mediaID string) (map[string]*models.StreamingCloudWatchMetrics, error)

	// ===== Cleanup Operations =====

	// CleanupExpiredMetrics removes expired metrics from cache
	CleanupExpiredMetrics(ctx context.Context) error
}
