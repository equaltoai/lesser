// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaAnalyticsRepository is a thread-safe in-memory implementation of interfaces.MediaAnalyticsRepository.
type MediaAnalyticsRepository struct {
	mu sync.RWMutex

	// Analytics by key: format_timestamp_mediaID -> MediaAnalytics
	analytics map[string]*models.MediaAnalytics

	// Analytics by date: date -> []MediaAnalytics
	analyticsByDate map[string][]*models.MediaAnalytics

	// Analytics by variant: variantKey -> []MediaAnalytics
	analyticsByVariant map[string][]*models.MediaAnalytics

	// Analytics by media: mediaID -> []MediaAnalytics
	analyticsByMedia map[string][]*models.MediaAnalytics
}

// NewMediaAnalyticsRepository creates a new in-memory media analytics repository
func NewMediaAnalyticsRepository() *MediaAnalyticsRepository {
	return &MediaAnalyticsRepository{
		analytics:          make(map[string]*models.MediaAnalytics),
		analyticsByDate:    make(map[string][]*models.MediaAnalytics),
		analyticsByVariant: make(map[string][]*models.MediaAnalytics),
		analyticsByMedia:   make(map[string][]*models.MediaAnalytics),
	}
}

// ===== Core Analytics Operations =====

// RecordMediaAnalytics records a media analytics entry
func (r *MediaAnalyticsRepository) RecordMediaAnalytics(_ context.Context, analytics *models.MediaAnalytics) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s_%d_%s", analytics.Format, analytics.Timestamp.Unix(), analytics.MediaID)
	r.analytics[key] = analytics

	// Index by date
	date := analytics.Date
	r.analyticsByDate[date] = append(r.analyticsByDate[date], analytics)

	// Index by media
	r.analyticsByMedia[analytics.MediaID] = append(r.analyticsByMedia[analytics.MediaID], analytics)

	// Index by variants
	for variantKey := range analytics.VariantCosts {
		r.analyticsByVariant[variantKey] = append(r.analyticsByVariant[variantKey], analytics)
	}

	return nil
}

// GetMediaAnalyticsByID retrieves media analytics by format, timestamp, and media ID
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByID(_ context.Context, format string, timestamp time.Time, mediaID string) (*models.MediaAnalytics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s_%d_%s", format, timestamp.Unix(), mediaID)
	analytics, exists := r.analytics[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return analytics, nil
}

// UpdateMediaAnalytics updates an existing media analytics record
func (r *MediaAnalyticsRepository) UpdateMediaAnalytics(_ context.Context, analytics *models.MediaAnalytics) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s_%d_%s", analytics.Format, analytics.Timestamp.Unix(), analytics.MediaID)
	if _, exists := r.analytics[key]; !exists {
		return storage.ErrNotFound
	}

	r.analytics[key] = analytics
	return nil
}

// StoreMediaAnalytics stores a media analytics event (alias for RecordMediaAnalytics)
func (r *MediaAnalyticsRepository) StoreMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	return r.RecordMediaAnalytics(ctx, analytics)
}

// ===== Analytics Queries =====

// GetMediaAnalyticsByDate retrieves media analytics for a specific date
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByDate(_ context.Context, date string) ([]*models.MediaAnalytics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.analyticsByDate[date], nil
}

// GetMediaAnalyticsByVariant retrieves media analytics for a specific variant
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByVariant(_ context.Context, variantKey string) ([]*models.MediaAnalytics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.analyticsByVariant[variantKey], nil
}

// GetMediaAnalyticsByTimeRange retrieves analytics for a specific media within time range
func (r *MediaAnalyticsRepository) GetMediaAnalyticsByTimeRange(_ context.Context, mediaID string, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.MediaAnalytics
	for _, analytics := range r.analyticsByMedia[mediaID] {
		if !analytics.Timestamp.Before(startTime) && !analytics.Timestamp.After(endTime) {
			results = append(results, analytics)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

// GetAllMediaAnalyticsByTimeRange retrieves analytics for all media within time range
func (r *MediaAnalyticsRepository) GetAllMediaAnalyticsByTimeRange(_ context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.MediaAnalytics
	for _, analytics := range r.analytics {
		if !analytics.Timestamp.Before(startTime) && !analytics.Timestamp.After(endTime) {
			results = append(results, analytics)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

// ===== Cost and Summary Operations =====

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
		summary["total_cost"] = summary["total_cost"].(int64) + analytics.TotalVariantCost
		summary["session_count"] = summary["session_count"].(int) + analytics.StreamingSessions
		summary["bandwidth_gb"] = summary["bandwidth_gb"].(float64) + (float64(analytics.TotalBandwidthBytes) / (1024 * 1024 * 1024))

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

	result := make([]map[string]interface{}, 0, len(variantData))
	for _, variant := range variantData {
		result = append(result, variant)
	}

	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// ===== Media View and Behavior Tracking =====

// RecordMediaView tracks media view events
func (r *MediaAnalyticsRepository) RecordMediaView(ctx context.Context, mediaID, userID string, duration time.Duration, quality string) error {
	analytics := &models.MediaAnalytics{
		MediaID:   mediaID,
		UserID:    userID,
		Duration:  duration.Seconds(),
		Quality:   quality,
		EventType: "media_view",
		Timestamp: time.Now(),
		Date:      time.Now().Format("2006-01-02"),
	}
	return r.RecordMediaAnalytics(ctx, analytics)
}

// TrackUserBehavior analyzes user behavior patterns
func (r *MediaAnalyticsRepository) TrackUserBehavior(ctx context.Context, userID string, behaviorData map[string]interface{}) error {
	analytics := &models.MediaAnalytics{
		UserID:    userID,
		EventType: "user_behavior",
		Timestamp: time.Now(),
		Date:      time.Now().Format("2006-01-02"),
	}

	if mediaID, ok := behaviorData["media_id"].(string); ok {
		analytics.MediaID = mediaID
	}
	if quality, ok := behaviorData["preferred_quality"].(string); ok {
		analytics.Quality = quality
	}
	if duration, ok := behaviorData["session_duration"].(float64); ok {
		analytics.Duration = duration
	}

	return r.RecordMediaAnalytics(ctx, analytics)
}

// ===== Popularity and Metrics =====

// CalculatePopularityMetrics calculates content popularity metrics
func (r *MediaAnalyticsRepository) CalculatePopularityMetrics(_ context.Context, mediaID string, days int) (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if days <= 0 {
		days = 7
	}

	metrics := make(map[string]interface{})
	totalViews := int64(0)
	totalSessions := int64(0)
	qualityDistribution := make(map[string]int)

	cutoff := time.Now().AddDate(0, 0, -days)

	for _, analytics := range r.analyticsByMedia[mediaID] {
		if analytics.Timestamp.After(cutoff) {
			if analytics.EventType == "media_view" {
				totalViews++
			}
			totalSessions += int64(analytics.StreamingSessions)
			for quality, count := range analytics.QualityDistribution {
				qualityDistribution[quality] += count
			}
		}
	}

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

// ===== Reporting and Recommendations =====

// GenerateAnalyticsReport generates comprehensive analytics report
func (r *MediaAnalyticsRepository) GenerateAnalyticsReport(_ context.Context, startDate, endDate string) (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	report := make(map[string]interface{})

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, err
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, err
	}

	totalCost := int64(0)
	totalBandwidth := int64(0)
	totalSessions := int64(0)
	codecDistribution := make(map[string]int64)
	resolutionDistribution := make(map[string]int64)

	for _, analytics := range r.analytics {
		if !analytics.Timestamp.Before(start) && !analytics.Timestamp.After(end.Add(24*time.Hour)) {
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

	if totalSessions > 0 {
		report["cost_per_session"] = float64(totalCost) / float64(totalSessions)
	}
	if totalBandwidth > 0 {
		report["cost_per_gb"] = float64(totalCost) / (float64(totalBandwidth) / (1024 * 1024 * 1024))
	}

	return report, nil
}

// GetContentRecommendations generates content recommendations
func (r *MediaAnalyticsRepository) GetContentRecommendations(_ context.Context, _ string, limit int) ([]map[string]interface{}, error) {
	recommendations := make([]map[string]interface{}, 0, limit)

	for i := 0; i < limit && i < 10; i++ {
		recommendation := map[string]interface{}{
			"media_id": fmt.Sprintf("recommended_media_%d", i+1),
			"score":    float64(10 - i),
			"reason":   "Based on your viewing history and trending content",
		}
		recommendations = append(recommendations, recommendation)
	}

	return recommendations, nil
}

// ===== Bandwidth and Popular Media Queries =====

// GetBandwidthByTimeRange retrieves bandwidth usage data within time range
func (r *MediaAnalyticsRepository) GetBandwidthByTimeRange(_ context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.MediaAnalytics
	for _, analytics := range r.analytics {
		if !analytics.Timestamp.Before(startTime) && !analytics.Timestamp.After(endTime) && analytics.TotalBandwidthBytes > 0 {
			results = append(results, analytics)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

// GetPopularMedia retrieves popular media (deprecated - returns empty)
func (r *MediaAnalyticsRepository) GetPopularMedia(_ context.Context, _, _ time.Time, _ int, _ *string) ([]*models.MediaAnalytics, error) {
	return []*models.MediaAnalytics{}, nil
}

// ===== Cleanup Operations =====

// CleanupOldAnalytics removes analytics records older than the specified duration
func (r *MediaAnalyticsRepository) CleanupOldAnalytics(_ context.Context, olderThan time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)

	for key, analytics := range r.analytics {
		if analytics.Timestamp.Before(cutoff) {
			delete(r.analytics, key)
		}
	}

	// Rebuild indexes
	r.analyticsByDate = make(map[string][]*models.MediaAnalytics)
	r.analyticsByVariant = make(map[string][]*models.MediaAnalytics)
	r.analyticsByMedia = make(map[string][]*models.MediaAnalytics)

	for _, analytics := range r.analytics {
		r.analyticsByDate[analytics.Date] = append(r.analyticsByDate[analytics.Date], analytics)
		r.analyticsByMedia[analytics.MediaID] = append(r.analyticsByMedia[analytics.MediaID], analytics)
		for variantKey := range analytics.VariantCosts {
			r.analyticsByVariant[variantKey] = append(r.analyticsByVariant[variantKey], analytics)
		}
	}

	return nil
}

// Clear clears all data (test helper)
func (r *MediaAnalyticsRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.analytics = make(map[string]*models.MediaAnalytics)
	r.analyticsByDate = make(map[string][]*models.MediaAnalytics)
	r.analyticsByVariant = make(map[string][]*models.MediaAnalytics)
	r.analyticsByMedia = make(map[string][]*models.MediaAnalytics)
}

// Ensure MediaAnalyticsRepository implements interfaces.MediaAnalyticsRepository
var _ interfaces.MediaAnalyticsRepository = (*MediaAnalyticsRepository)(nil)
