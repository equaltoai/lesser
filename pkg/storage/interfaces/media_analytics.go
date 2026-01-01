// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaAnalyticsRepository defines the interface for media analytics operations.
// This handles media streaming analytics with variant-level cost attribution.
type MediaAnalyticsRepository interface {
	// Core analytics operations
	RecordMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error
	GetMediaAnalyticsByID(ctx context.Context, format string, timestamp time.Time, mediaID string) (*models.MediaAnalytics, error)
	UpdateMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error
	StoreMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error

	// Analytics queries by date and variant
	GetMediaAnalyticsByDate(ctx context.Context, date string) ([]*models.MediaAnalytics, error)
	GetMediaAnalyticsByVariant(ctx context.Context, variantKey string) ([]*models.MediaAnalytics, error)
	GetMediaAnalyticsByTimeRange(ctx context.Context, mediaID string, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error)
	GetAllMediaAnalyticsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error)

	// Cost and summary operations
	GetDailyCostSummary(ctx context.Context, date string) (map[string]interface{}, error)
	GetTopVariantsByDemand(ctx context.Context, date string, limit int) ([]map[string]interface{}, error)

	// Media view and behavior tracking
	RecordMediaView(ctx context.Context, mediaID, userID string, duration time.Duration, quality string) error
	TrackUserBehavior(ctx context.Context, userID string, behaviorData map[string]interface{}) error

	// Popularity and metrics
	CalculatePopularityMetrics(ctx context.Context, mediaID string, days int) (map[string]interface{}, error)
	GetMediaMetricsForDate(ctx context.Context, mediaID, date string) (map[string]interface{}, error)

	// Reporting and recommendations
	GenerateAnalyticsReport(ctx context.Context, startDate, endDate string) (map[string]interface{}, error)
	GetContentRecommendations(ctx context.Context, userID string, limit int) ([]map[string]interface{}, error)

	// Bandwidth and popular media queries
	GetBandwidthByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error)
	GetPopularMedia(ctx context.Context, startTime, endTime time.Time, limit int, cursor *string) ([]*models.MediaAnalytics, error)

	// Cleanup operations
	CleanupOldAnalytics(ctx context.Context, olderThan time.Duration) error
}
