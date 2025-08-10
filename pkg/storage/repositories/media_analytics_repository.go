package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// MediaAnalyticsRepository handles media analytics operations using DynamORM
type MediaAnalyticsRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewMediaAnalyticsRepository creates a new media analytics repository
func NewMediaAnalyticsRepository(db core.DB, tableName string, logger *zap.Logger) *MediaAnalyticsRepository {
	return &MediaAnalyticsRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// RecordMediaAnalytics records a media analytics entry
func (r *MediaAnalyticsRepository) RecordMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	// Ensure keys are properly initialized
	if analytics.PK == "" || analytics.SK == "" {
		analytics.UpdateKeys()
	}

	err := r.db.WithContext(ctx).Model(analytics).Create()
	if err != nil {
		r.logger.Error("Failed to record media analytics",
			zap.String("media_id", analytics.MediaID),
			zap.String("format", analytics.Format),
			zap.String("event_type", analytics.EventType),
			zap.Error(err))
		return fmt.Errorf("failed to record media analytics: %w", err)
	}

	r.logger.Debug("Recorded media analytics",
		zap.String("media_id", analytics.MediaID),
		zap.String("format", analytics.Format),
		zap.String("event_type", analytics.EventType),
		zap.Int64("total_cost", analytics.TotalVariantCost))

	return nil
}

// GetMediaAnalyticsByID retrieves media analytics by media ID and timestamp
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByID(ctx context.Context, format string, timestamp time.Time, mediaID string) (*models.MediaAnalytics, error) {
	pk := fmt.Sprintf("MEDIA_ANALYTICS#%s", format)
	sk := fmt.Sprintf("%d#%s", timestamp.Unix(), mediaID)

	var analytics models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&analytics)

	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get media analytics: %w", err)
	}

	return &analytics, nil
}

// GetMediaAnalyticsByDate retrieves media analytics for a specific date
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByDate(ctx context.Context, date string) ([]*models.MediaAnalytics, error) {
	gsi1pk := fmt.Sprintf("DATE#%s", date)

	var analyticsList []*models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("GSI1PK", "=", gsi1pk).
		Scan(&analyticsList)

	if err != nil {
		return nil, fmt.Errorf("failed to get media analytics by date: %w", err)
	}

	return analyticsList, nil
}

// GetMediaAnalyticsByVariant retrieves media analytics for a specific variant
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByVariant(ctx context.Context, variantKey string) ([]*models.MediaAnalytics, error) {
	gsi2pk := fmt.Sprintf("VARIANT#%s", variantKey)

	var analyticsList []*models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("GSI2PK", "=", gsi2pk).
		Scan(&analyticsList)

	if err != nil {
		return nil, fmt.Errorf("failed to get media analytics by variant: %w", err)
	}

	return analyticsList, nil
}

// UpdateMediaAnalytics updates an existing media analytics record
func (r *MediaAnalyticsRepository) UpdateMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	// Ensure keys are properly initialized
	analytics.UpdateKeys()

	err := r.db.WithContext(ctx).Model(analytics).Update()
	if err != nil {
		r.logger.Error("Failed to update media analytics",
			zap.String("media_id", analytics.MediaID),
			zap.String("format", analytics.Format),
			zap.Error(err))
		return fmt.Errorf("failed to update media analytics: %w", err)
	}

	r.logger.Debug("Updated media analytics",
		zap.String("media_id", analytics.MediaID),
		zap.String("format", analytics.Format))

	return nil
}

// GetDailyCostSummary retrieves cost summary for a specific date
func (r *MediaAnalyticsRepository) GetDailyCostSummary(ctx context.Context, date string) (map[string]interface{}, error) {
	analyticsList, err := r.GetMediaAnalyticsByDate(ctx, date)
	if err != nil {
		return nil, err
	}

	summary := map[string]interface{}{
		"date":            date,
		"total_cost":      int64(0),
		"variant_count":   0,
		"session_count":   0,
		"bandwidth_gb":    float64(0),
		"dominant_codecs": make(map[string]int),
	}

	codecCounts := make(map[string]int)

	for _, analytics := range analyticsList {
		// Add to total cost
		if totalCost, ok := summary["total_cost"].(int64); ok {
			summary["total_cost"] = totalCost + analytics.TotalVariantCost
		}

		// Add to session count
		if sessionCount, ok := summary["session_count"].(int); ok {
			summary["session_count"] = sessionCount + analytics.StreamingSessions
		}

		// Add to bandwidth (convert bytes to GB)
		if bandwidthGB, ok := summary["bandwidth_gb"].(float64); ok {
			summary["bandwidth_gb"] = bandwidthGB + (float64(analytics.TotalBandwidthBytes) / (1024 * 1024 * 1024))
		}

		// Count variants and codecs
		for _, variantCost := range analytics.VariantCosts {
			summary["variant_count"] = summary["variant_count"].(int) + 1
			codecCounts[variantCost.Codec]++
		}
	}

	summary["dominant_codecs"] = codecCounts
	return summary, nil
}

// GetTopVariantsByDemand retrieves the most popular variants by viewer count
func (r *MediaAnalyticsRepository) GetTopVariantsByDemand(ctx context.Context, date string, limit int) ([]map[string]interface{}, error) {
	analyticsList, err := r.GetMediaAnalyticsByDate(ctx, date)
	if err != nil {
		return nil, err
	}

	// Aggregate variant data
	variantData := make(map[string]map[string]interface{})

	for _, analytics := range analyticsList {
		for variantKey, variantCost := range analytics.VariantCosts {
			if _, exists := variantData[variantKey]; !exists {
				variantData[variantKey] = map[string]interface{}{
					"variant_key":     variantKey,
					"resolution":      variantCost.Resolution,
					"codec":           variantCost.Codec,
					"bitrate":         variantCost.Bitrate,
					"total_cost":      int64(0),
					"delivery_count":  int64(0),
					"bandwidth_bytes": int64(0),
					"viewer_minutes":  int64(0),
				}
			}

			variant := variantData[variantKey]
			variant["total_cost"] = variant["total_cost"].(int64) + variantCost.TotalCost
			variant["delivery_count"] = variant["delivery_count"].(int64) + variantCost.DeliveryCount
			variant["bandwidth_bytes"] = variant["bandwidth_bytes"].(int64) + variantCost.BandwidthBytes
			variant["viewer_minutes"] = variant["viewer_minutes"].(int64) + variantCost.ViewerMinutes
		}
	}

	// Convert to slice and sort by viewer minutes (simplified - would need proper sorting)
	result := make([]map[string]interface{}, 0, len(variantData))
	for _, variant := range variantData {
		result = append(result, variant)
	}

	// Return limited results (would implement proper sorting in production)
	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// CleanupOldAnalytics removes analytics records older than the specified duration
func (r *MediaAnalyticsRepository) CleanupOldAnalytics(ctx context.Context, olderThan time.Duration) error {
	cutoffDate := time.Now().Add(-olderThan).Format(common.DateFormat)

	// Query old records (simplified - would need proper cleanup implementation)
	var oldRecords []*models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("Date", "<", cutoffDate).
		Scan(&oldRecords)

	if err != nil {
		return fmt.Errorf("failed to query old analytics records: %w", err)
	}

	// Delete old records
	deletedCount := 0
	for _, record := range oldRecords {
		if err := r.db.WithContext(ctx).Model(record).Delete(); err != nil {
			r.logger.Warn("Failed to delete old analytics record",
				zap.String("media_id", record.MediaID),
				zap.Error(err))
		} else {
			deletedCount++
		}
	}

	r.logger.Info("Cleaned up old media analytics",
		zap.Int("deleted_count", deletedCount),
		zap.String("cutoff_date", cutoffDate))

	return nil
}
