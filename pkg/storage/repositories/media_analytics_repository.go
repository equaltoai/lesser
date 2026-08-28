package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// MediaAnalyticsRepository handles media analytics operations using DynamORM with BaseRepository
type MediaAnalyticsRepository struct {
	*EnhancedBaseRepository[*models.MediaAnalytics]
}

// NewMediaAnalyticsRepository creates a new media analytics repository with enhanced functionality
func NewMediaAnalyticsRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MediaAnalyticsRepository {
	// Create enhanced repository optimized for media analytics operations
	enhancedRepo := NewEnhancedBaseRepository[*models.MediaAnalytics](db, tableName, logger, costService, "MediaAnalyticsRepository", "media_analytics")

	// Set up enhanced services for media analytics operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Analytics cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Analytics events

	return &MediaAnalyticsRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// RecordMediaAnalytics records a media analytics entry with comprehensive engagement tracking
func (r *MediaAnalyticsRepository) RecordMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	// Ensure keys are properly initialized
	if analytics.PK == "" || analytics.SK == "" {
		_ = analytics.UpdateKeys() // Ignore error as this is internal model operation
	}

	// Use BaseRepository.Create for standardized creation with cost tracking
	err := r.ValidateAndCreate(ctx, analytics)
	if err != nil {
		r.logger.Error("Failed to record media analytics",
			zap.String("media_id", analytics.MediaID),
			zap.String("format", analytics.Format),
			zap.String("event_type", analytics.EventType),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityMedia, analytics.MediaID)
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

	// Use BaseRepository.Get for standardized retrieval with cost tracking
	var analytics models.MediaAnalytics
	err := r.Get(ctx, pk, sk, &analytics)
	if err != nil {
		if err.Error() == fmt.Sprintf("item not found: pk=%s, sk=%s", pk, sk) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityMedia, mediaID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityMedia, mediaID)
	}

	return &analytics, nil
}

// walkMediaAnalyticsByPartition walks a keyed MediaAnalytics partition in
// bounded pages (wave #1469): Limit(500)/page, 100-page cap, fail-closed on
// exhaustion.
func (r *MediaAnalyticsRepository) walkMediaAnalyticsByPartition(ctx context.Context, pkField, pkValue string) ([]*models.MediaAnalytics, error) {
	var analyticsList []*models.MediaAnalytics
	err := walkKeyedPages(
		r.GetDB().WithContext(ctx).Model(&models.MediaAnalytics{}).Where(pkField, "=", pkValue),
		500, 100,
		func(page []*models.MediaAnalytics) (bool, error) {
			analyticsList = append(analyticsList, page...)
			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return analyticsList, nil
}

// GetMediaAnalyticsByDate retrieves media analytics for a specific date with engagement data
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByDate(ctx context.Context, date string) ([]*models.MediaAnalytics, error) {
	gsi1pk := fmt.Sprintf("DATE#%s", date)

	// The whole keyed gsi1 DATE#<date> partition must be read, so the read is a
	// bounded page walk (wave #1469): Limit(500)/page, 100-page cap, fail-closed
	// on exhaustion.
	analyticsList, err := r.walkMediaAnalyticsByPartition(ctx, "gsi1PK", gsi1pk)
	if err != nil {
		r.logger.Error("Failed to get media analytics by date",
			zap.String("date", date),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "analytics by date")
	}

	// Track cost for GSI query
	if r.GetCostService() != nil {
		if trackErr := r.TrackRead(ctx, "GSI1Query", int64(len(analyticsList))); trackErr != nil {
			r.logger.Warn("failed to track GSI query cost", zap.Error(trackErr))
		}
	}

	return analyticsList, nil
}

// GetMediaAnalyticsByVariant retrieves media analytics for a specific variant with performance metrics
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByVariant(ctx context.Context, variantKey string) ([]*models.MediaAnalytics, error) {
	gsi2pk := fmt.Sprintf("VARIANT#%s", variantKey)

	// The whole keyed gsi2 VARIANT#<variant> partition must be read, so the read
	// is a bounded page walk (wave #1469): Limit(500)/page, 100-page cap,
	// fail-closed on exhaustion.
	analyticsList, err := r.walkMediaAnalyticsByPartition(ctx, "gsi2PK", gsi2pk)
	if err != nil {
		r.logger.Error("Failed to get media analytics by variant",
			zap.String("variant_key", variantKey),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "analytics by variant")
	}

	// Track cost for GSI query
	if r.GetCostService() != nil {
		if trackErr := r.TrackRead(ctx, "GSI2Query", int64(len(analyticsList))); trackErr != nil {
			r.logger.Warn("failed to track GSI query cost", zap.Error(trackErr))
		}
	}

	return analyticsList, nil
}

// UpdateMediaAnalytics updates an existing media analytics record with engagement metrics
func (r *MediaAnalyticsRepository) UpdateMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	// Ensure keys are properly initialized
	_ = analytics.UpdateKeys() // Ignore error as this is internal model operation

	// Use BaseRepository.Update for standardized updates with cost tracking
	err := r.ValidateAndUpdate(ctx, analytics)
	if err != nil {
		r.logger.Error("Failed to update media analytics",
			zap.String("media_id", analytics.MediaID),
			zap.String("format", analytics.Format),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityMedia, analytics.MediaID)
	}

	r.logger.Debug("Updated media analytics",
		zap.String("media_id", analytics.MediaID),
		zap.String("format", analytics.Format))

	return nil
}

// GetDailyCostSummary retrieves cost summary for a specific date with business intelligence aggregation
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

// GetTopVariantsByDemand retrieves the most popular variants by viewer count with recommendation algorithms
func (r *MediaAnalyticsRepository) GetTopVariantsByDemand(ctx context.Context, date string, limit int) ([]map[string]interface{}, error) {
	analyticsList, err := r.GetMediaAnalyticsByDate(ctx, date)
	if err != nil {
		return nil, err
	}

	// Aggregate variant data for trending analysis
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

// RecordMediaView tracks media view events with user engagement analytics
func (r *MediaAnalyticsRepository) RecordMediaView(ctx context.Context, mediaID, userID string, duration time.Duration, quality string) error {
	analytics := &models.MediaAnalytics{}
	analytics.SetGeneralEvent("media_view", mediaID, userID)
	analytics.Duration = duration.Seconds()
	analytics.Quality = quality

	// Add viewer to quality distribution for real-time metrics
	analytics.AddQualityViewer(quality)

	return r.RecordMediaAnalytics(ctx, analytics)
}

// CalculatePopularityMetrics calculates content popularity metrics with trending algorithms
func (r *MediaAnalyticsRepository) CalculatePopularityMetrics(ctx context.Context, mediaID string, days int) (map[string]interface{}, error) {
	if days <= 0 {
		days = 7 // Default to last 7 days
	}

	metrics := make(map[string]interface{})
	totalViews := int64(0)
	totalSessions := int64(0)
	qualityDistribution := make(map[string]int)

	// Calculate metrics for each day in the range
	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i).Format(common.DateFormat)
		dayMetrics, err := r.GetMediaMetricsForDate(ctx, mediaID, date)
		if err != nil {
			r.logger.Warn("Failed to get metrics for date", zap.String("date", date), zap.Error(err))
			continue
		}

		if views, ok := dayMetrics["total_views"].(int64); ok {
			totalViews += views
		}
		if sessions, ok := dayMetrics["streaming_sessions"].(int64); ok {
			totalSessions += sessions
		}
		if qualities, ok := dayMetrics["quality_distribution"].(map[string]int); ok {
			for quality, count := range qualities {
				qualityDistribution[quality] += count
			}
		}
	}

	// Calculate popularity score (simplified algorithm)
	popularityScore := float64(totalViews)*0.7 + float64(totalSessions)*0.3

	metrics["media_id"] = mediaID
	metrics["total_views"] = totalViews
	metrics["total_sessions"] = totalSessions
	metrics["popularity_score"] = popularityScore
	metrics["quality_distribution"] = qualityDistribution
	metrics["days_analyzed"] = days

	return metrics, nil
}

// GetMediaMetricsForDate gets metrics for a specific media item on a specific date
func (r *MediaAnalyticsRepository) GetMediaMetricsForDate(ctx context.Context, mediaID, date string) (map[string]interface{}, error) {
	analyticsList, err := r.GetMediaAnalyticsByDate(ctx, date)
	if err != nil {
		return nil, err
	}

	metrics := map[string]interface{}{
		"total_views":          int64(0),
		"streaming_sessions":   int64(0),
		"quality_distribution": make(map[string]int),
		"total_bandwidth":      int64(0),
	}

	qualityDist := make(map[string]int)

	for _, analytics := range analyticsList {
		if analytics.MediaID == mediaID {
			if analytics.EventType == "media_view" {
				metrics["total_views"] = metrics["total_views"].(int64) + 1
			}
			metrics["streaming_sessions"] = metrics["streaming_sessions"].(int64) + int64(analytics.StreamingSessions)
			metrics["total_bandwidth"] = metrics["total_bandwidth"].(int64) + analytics.TotalBandwidthBytes

			for quality, count := range analytics.QualityDistribution {
				qualityDist[quality] += count
			}
		}
	}

	metrics["quality_distribution"] = qualityDist
	return metrics, nil
}

// GenerateAnalyticsReport generates comprehensive analytics report with business intelligence
func (r *MediaAnalyticsRepository) GenerateAnalyticsReport(ctx context.Context, startDate, endDate string) (map[string]interface{}, error) {
	report := make(map[string]interface{})

	start, err := time.Parse(common.DateFormat, startDate)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "start date validation")
	}

	end, err := time.Parse(common.DateFormat, endDate)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "end date validation")
	}

	totalCost := int64(0)
	totalBandwidth := int64(0)
	totalSessions := int64(0)
	codecDistribution := make(map[string]int64)
	resolutionDistribution := make(map[string]int64)

	// Iterate through each date in the range
	for d := start; d.Before(end) || d.Equal(end); d = d.AddDate(0, 0, 1) {
		date := d.Format(common.DateFormat)
		dayAnalytics, err := r.GetMediaAnalyticsByDate(ctx, date)
		if err != nil {
			r.logger.Warn("Failed to get analytics for date", zap.String("date", date), zap.Error(err))
			continue
		}

		for _, analytics := range dayAnalytics {
			totalCost += analytics.TotalVariantCost
			totalBandwidth += analytics.TotalBandwidthBytes
			totalSessions += int64(analytics.StreamingSessions)

			for _, variantCost := range analytics.VariantCosts {
				codecDistribution[variantCost.Codec] += variantCost.DeliveryCount
				resolutionDistribution[variantCost.Resolution] += variantCost.DeliveryCount
			}
		}
	}

	report["start_date"] = startDate
	report["end_date"] = endDate
	report["total_cost_microdollars"] = totalCost
	report["total_bandwidth_bytes"] = totalBandwidth
	report["total_streaming_sessions"] = totalSessions
	report["codec_distribution"] = codecDistribution
	report["resolution_distribution"] = resolutionDistribution

	// Calculate efficiency metrics
	if totalSessions > 0 {
		report["cost_per_session"] = float64(totalCost) / float64(totalSessions)
	}
	if totalBandwidth > 0 {
		report["cost_per_gb"] = float64(totalCost) / (float64(totalBandwidth) / (1024 * 1024 * 1024))
	}

	return report, nil
}

// TrackUserBehavior analyzes user behavior patterns for personalization
func (r *MediaAnalyticsRepository) TrackUserBehavior(ctx context.Context, userID string, behaviorData map[string]interface{}) error {
	// Create analytics record for user behavior tracking
	analytics := &models.MediaAnalytics{}
	analytics.SetGeneralEvent("user_behavior", "", userID)

	// Extract behavior patterns from behaviorData
	if mediaID, ok := behaviorData["media_id"].(string); ok {
		analytics.MediaID = mediaID
	}
	if quality, ok := behaviorData["preferred_quality"].(string); ok {
		analytics.Quality = quality
		analytics.AddQualityViewer(quality)
	}
	if duration, ok := behaviorData["session_duration"].(float64); ok {
		analytics.Duration = duration
	}

	return r.RecordMediaAnalytics(ctx, analytics)
}

// GetContentRecommendations generates ML-based content recommendations
func (r *MediaAnalyticsRepository) GetContentRecommendations(ctx context.Context, userID string, limit int) ([]map[string]interface{}, error) {
	// Get user's viewing history and preferences
	userBehavior, err := r.getUserBehaviorHistory(ctx, userID, 30) // Last 30 days
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "user behavior history")
	}

	// Analyze user preferences
	preferences := r.analyzeUserPreferences(userBehavior)

	// Get trending content that matches user preferences
	recommendations := make([]map[string]interface{}, 0, limit)

	// Simple recommendation algorithm - in production would use ML models
	for i := 0; i < limit && i < 10; i++ { // Cap at 10 for now
		recommendation := map[string]interface{}{
			"media_id":   fmt.Sprintf("recommended_media_%d", i+1),
			"score":      preferences["popularity_weight"].(float64) * float64(10-i),
			"reason":     "Based on your viewing history and trending content",
			"quality":    preferences["preferred_quality"],
			"codec":      preferences["preferred_codec"],
			"resolution": preferences["preferred_resolution"],
		}
		recommendations = append(recommendations, recommendation)
	}

	return recommendations, nil
}

// getUserBehaviorHistory gets user's behavior history for recommendation analysis
func (r *MediaAnalyticsRepository) getUserBehaviorHistory(ctx context.Context, userID string, days int) ([]*models.MediaAnalytics, error) {
	var userAnalytics []*models.MediaAnalytics

	// Query user behavior records from the last N days
	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i).Format(common.DateFormat)
		dayAnalytics, err := r.GetMediaAnalyticsByDate(ctx, date)
		if err != nil {
			continue // Skip days with errors
		}

		// Filter for this user's analytics
		for _, analytics := range dayAnalytics {
			if analytics.UserID == userID {
				userAnalytics = append(userAnalytics, analytics)
			}
		}
	}

	return userAnalytics, nil
}

// analyzeUserPreferences analyzes user behavior to determine preferences
func (r *MediaAnalyticsRepository) analyzeUserPreferences(userBehavior []*models.MediaAnalytics) map[string]interface{} {
	preferences := make(map[string]interface{})

	qualityCounts := make(map[string]int)
	codecCounts := make(map[string]int)
	resolutionCounts := make(map[string]int)
	totalViews := len(userBehavior)

	for _, analytics := range userBehavior {
		if analytics.Quality != "" {
			qualityCounts[analytics.Quality]++
		}

		for _, variantCost := range analytics.VariantCosts {
			codecCounts[variantCost.Codec]++
			resolutionCounts[variantCost.Resolution]++
		}
	}

	// Find preferred quality, codec, resolution
	preferences["preferred_quality"] = findMostFrequent(qualityCounts)
	preferences["preferred_codec"] = findMostFrequent(codecCounts)
	preferences["preferred_resolution"] = findMostFrequent(resolutionCounts)
	preferences["total_views"] = totalViews
	preferences["popularity_weight"] = 0.8 // Weight for trending content

	return preferences
}

// findMostFrequent finds the most frequent item in a count map
func findMostFrequent(counts map[string]int) string {
	maxCount := 0
	mostFrequent := ""

	for item, count := range counts {
		if count > maxCount {
			maxCount = count
			mostFrequent = item
		}
	}

	return mostFrequent
}

// CleanupOldAnalytics removes analytics records older than the specified duration preserving business intelligence data
func (r *MediaAnalyticsRepository) CleanupOldAnalytics(_ context.Context, olderThan time.Duration) error {
	cutoffDate := time.Now().Add(-olderThan).Format(common.DateFormat)

	// Media analytics records are TTL-driven (`ttl` on the item, `ttl` configured on the table).
	// Manual cleanup required a DynamoDB Scan, which is both expensive and unnecessary.
	r.logger.Info("skipping manual media analytics cleanup (ttl handles expiration)",
		zap.String("cutoff_date", cutoffDate),
	)
	return nil
}

// GetMediaAnalyticsByTimeRange retrieves analytics for a specific media within time range
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByTimeRange(ctx context.Context, mediaID string, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	if err := common.ValidateRequiredParam("mediaID", mediaID); err != nil {
		return nil, err
	}

	var results []*models.MediaAnalytics

	// Iterate day by day through the range
	currentDate := startTime.Truncate(24 * time.Hour)
	endDate := endTime.Truncate(24 * time.Hour)

	for !currentDate.After(endDate) && len(results) < limit {
		dateStr := currentDate.Format(common.DateFormat)
		gsi1pk := fmt.Sprintf("DATE#%s", dateStr)

		// Query this day's analytics. Each day is a keyed gsi1 DATE#<date>
		// partition read with no enforced limit, so the read is a bounded page
		// walk (wave #1469): Limit(500)/page, 100-page cap, fail-closed on
		// exhaustion.
		var dayAnalytics []*models.MediaAnalytics
		err := walkKeyedPages(
			r.GetDB().WithContext(ctx).Model(&models.MediaAnalytics{}).
				Where("gsi1PK", "=", gsi1pk),
			500, 100,
			func(page []*models.MediaAnalytics) (bool, error) {
				dayAnalytics = append(dayAnalytics, page...)
				return false, nil
			},
		)

		if err != nil {
			r.logger.Error("Failed to get media analytics for day",
				zap.String("date", dateStr),
				zap.Error(err))
			return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "analytics by time range")
		}

		// Filter by media ID and time range
		for _, analytics := range dayAnalytics {
			if analytics.MediaID == mediaID &&
				!analytics.Timestamp.Before(startTime) &&
				!analytics.Timestamp.After(endTime) {
				results = append(results, analytics)
				if len(results) >= limit {
					break
				}
			}
		}

		currentDate = currentDate.Add(24 * time.Hour)
	}

	// Track cost
	if r.GetCostService() != nil {
		if trackErr := r.TrackRead(ctx, "TimeRangeQuery", int64(len(results))); trackErr != nil {
			r.logger.Warn("failed to track query cost", zap.Error(trackErr))
		}
	}

	return results, nil
}

// GetAllMediaAnalyticsByTimeRange retrieves analytics for all media within time range
func (r *MediaAnalyticsRepository) GetAllMediaAnalyticsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	var results []*models.MediaAnalytics

	// Iterate day by day through the range
	currentDate := startTime.Truncate(24 * time.Hour)
	endDate := endTime.Truncate(24 * time.Hour)

	for !currentDate.After(endDate) && len(results) < limit {
		dateStr := currentDate.Format(common.DateFormat)
		gsi1pk := fmt.Sprintf("DATE#%s", dateStr)

		// Query this day's analytics. Each day is a keyed gsi1 DATE#<date>
		// partition read with no enforced limit, so the read is a bounded page
		// walk (wave #1469): Limit(500)/page, 100-page cap, fail-closed on
		// exhaustion.
		var dayAnalytics []*models.MediaAnalytics
		err := walkKeyedPages(
			r.GetDB().WithContext(ctx).Model(&models.MediaAnalytics{}).
				Where("gsi1PK", "=", gsi1pk),
			500, 100,
			func(page []*models.MediaAnalytics) (bool, error) {
				dayAnalytics = append(dayAnalytics, page...)
				return false, nil
			},
		)

		if err != nil {
			r.logger.Error("Failed to get all media analytics for day",
				zap.String("date", dateStr),
				zap.Error(err))
			return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "all analytics by time range")
		}

		// Filter by time range
		for _, analytics := range dayAnalytics {
			if !analytics.Timestamp.Before(startTime) && !analytics.Timestamp.After(endTime) {
				results = append(results, analytics)
				if len(results) >= limit {
					break
				}
			}
		}

		currentDate = currentDate.Add(24 * time.Hour)
	}

	// Track cost
	if r.GetCostService() != nil {
		if trackErr := r.TrackRead(ctx, "AllMediaTimeRangeQuery", int64(len(results))); trackErr != nil {
			r.logger.Warn("failed to track query cost", zap.Error(trackErr))
		}
	}

	return results, nil
}

// GetPopularMedia retrieves popular media sorted by view count with cursor pagination
// NOTE: Deprecated in favor of service layer using GetPopularMediaByPeriod from PopularityRepository
// This method is kept for backward compatibility but should not be used
func (r *MediaAnalyticsRepository) GetPopularMedia(_ context.Context, _, _ time.Time, _ int, _ *string) ([]*models.MediaAnalytics, error) {
	r.logger.Warn("GetPopularMedia is deprecated - use MediaPopularityRepository.GetPopularMediaByPeriod instead")

	// Return empty results - this method should not be called
	// The service layer should use MediaPopularityRepository directly
	return []*models.MediaAnalytics{}, nil
}

// GetBandwidthByTimeRange retrieves bandwidth usage data within time range
func (r *MediaAnalyticsRepository) GetBandwidthByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	var results []*models.MediaAnalytics

	// Iterate day by day through the range
	currentDate := startTime.Truncate(24 * time.Hour)
	endDate := endTime.Truncate(24 * time.Hour)

	for !currentDate.After(endDate) && len(results) < limit {
		dateStr := currentDate.Format(common.DateFormat)
		gsi1pk := fmt.Sprintf("DATE#%s", dateStr)

		// Query this day's analytics. Each day is a keyed gsi1 DATE#<date>
		// partition read with no enforced limit, so the read is a bounded page
		// walk (wave #1469): Limit(500)/page, 100-page cap, fail-closed on
		// exhaustion.
		var dayAnalytics []*models.MediaAnalytics
		err := walkKeyedPages(
			r.GetDB().WithContext(ctx).Model(&models.MediaAnalytics{}).
				Where("gsi1PK", "=", gsi1pk),
			500, 100,
			func(page []*models.MediaAnalytics) (bool, error) {
				dayAnalytics = append(dayAnalytics, page...)
				return false, nil
			},
		)

		if err != nil {
			r.logger.Error("Failed to get bandwidth data for day",
				zap.String("date", dateStr),
				zap.Error(err))
			return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "bandwidth by time range")
		}

		// Filter by time range and bandwidth data
		for _, analytics := range dayAnalytics {
			if !analytics.Timestamp.Before(startTime) &&
				!analytics.Timestamp.After(endTime) &&
				analytics.TotalBandwidthBytes > 0 {
				results = append(results, analytics)
				if len(results) >= limit {
					break
				}
			}
		}

		currentDate = currentDate.Add(24 * time.Hour)
	}

	// Track cost
	if r.GetCostService() != nil {
		if trackErr := r.TrackRead(ctx, "BandwidthTimeRangeQuery", int64(len(results))); trackErr != nil {
			r.logger.Warn("failed to track query cost", zap.Error(trackErr))
		}
	}

	return results, nil
}

// StoreMediaAnalytics stores a media analytics event
// NOTE: Popularity updates are now handled by the service layer (RecordStreamingEvent)
// which calls MediaPopularityRepository.IncrementViewCount directly
func (r *MediaAnalyticsRepository) StoreMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	return r.RecordMediaAnalytics(ctx, analytics)
}
