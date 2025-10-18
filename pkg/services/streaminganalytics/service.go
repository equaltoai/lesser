// Package streaminganalytics provides streaming analytics and performance telemetry services.
// It aggregates real-time streaming metrics, processes time-series data, and provides
// analytical insights for media streaming performance, bandwidth usage, and viewer behavior.
package streaminganalytics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// Event type constants
const (
	EventTypeSessionStart  = "session_start"
	EventTypeSessionEnd    = "session_end"
	EventTypeRebufferStart = "rebuffer_start"
	EventTypeBuffering     = "buffering"
)

// AnalyticsRepository defines the interface for media analytics data access
type AnalyticsRepository interface {
	GetMediaAnalyticsByTimeRange(ctx context.Context, mediaID string, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error)
	GetAllMediaAnalyticsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error)
	GetBandwidthByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error)
	StoreMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error
}

// PopularityRepository defines the interface for media popularity aggregates
type PopularityRepository interface {
	GetPopularMediaByPeriod(ctx context.Context, period string, limit int, cursor *string) ([]*models.MediaPopularity, error)
	IncrementViewCount(ctx context.Context, mediaID, period string, incrementBy int64) error
	UpsertPopularity(ctx context.Context, popularity *models.MediaPopularity) error
}

// SessionRepository defines the interface for media session data access
type SessionRepository interface {
	// Methods would go here as needed
}

// Service provides streaming analytics functionality
type Service struct {
	analyticsRepo  AnalyticsRepository
	popularityRepo PopularityRepository
	sessionRepo    SessionRepository
	logger         *zap.Logger
}

// NewService creates a new streaming analytics service
func NewService(
	analyticsRepo AnalyticsRepository,
	popularityRepo PopularityRepository,
	sessionRepo SessionRepository,
	logger *zap.Logger,
) *Service {
	return &Service{
		analyticsRepo:  analyticsRepo,
		popularityRepo: popularityRepo,
		sessionRepo:    sessionRepo,
		logger:         logger,
	}
}

// GetStreamingAnalytics retrieves detailed analytics for a specific media item
func (s *Service) GetStreamingAnalytics(ctx context.Context, mediaID string) (*model.StreamingAnalytics, error) {
	if err := common.ValidateRequiredParam("mediaID", mediaID); err != nil {
		return nil, err
	}

	// Get analytics events
	analytics, err := s.fetchAnalytics(ctx, mediaID)
	if err != nil {
		return nil, err
	}

	// Aggregate all metrics
	metrics := s.aggregateMetrics(analytics)

	// Build and return result
	return s.buildAnalyticsResult(metrics), nil
}

// fetchAnalytics retrieves analytics data for the media
func (s *Service) fetchAnalytics(ctx context.Context, mediaID string) ([]*models.MediaAnalytics, error) {
	endTime := time.Now()
	startTime := endTime.Add(-30 * 24 * time.Hour)

	analytics, err := s.analyticsRepo.GetMediaAnalyticsByTimeRange(ctx, mediaID, startTime, endTime, 1000)
	if err != nil {
		s.logger.Error("failed to get media analytics",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get media analytics: %w", err)
	}
	return analytics, nil
}

// aggregateMetrics processes analytics data and calculates aggregates
func (s *Service) aggregateMetrics(analytics []*models.MediaAnalytics) *analyticsMetrics {
	metrics := &analyticsMetrics{
		uniqueUsers:  make(map[string]bool),
		qualityViews: make(map[string]*qualityAggregator),
	}

	for _, a := range analytics {
		s.processEvent(a, metrics)
	}

	return metrics
}

// processEvent processes a single analytics event
func (s *Service) processEvent(a *models.MediaAnalytics, metrics *analyticsMetrics) {
	if a.EventType == "session_start" {
		metrics.totalViews++
		metrics.totalSessions++
		if a.UserID != "" {
			metrics.uniqueUsers[a.UserID] = true
		}
	}

	if a.Quality != "" {
		s.trackQuality(a, metrics)
	}

	if a.EventType == "session_end" {
		metrics.completedSessions++
	}

	if a.EventType == "rebuffer_start" || a.EventType == "buffering" {
		metrics.bufferingEvents++
	}

	metrics.totalWatchTimeSeconds += int64(a.Duration)
}

// trackQuality tracks quality metrics
func (s *Service) trackQuality(a *models.MediaAnalytics, metrics *analyticsMetrics) {
	if _, exists := metrics.qualityViews[a.Quality]; !exists {
		metrics.qualityViews[a.Quality] = &qualityAggregator{
			quality:        a.Quality,
			viewCount:      0,
			totalBandwidth: 0,
		}
	}
	agg := metrics.qualityViews[a.Quality]
	agg.viewCount++

	if a.VariantBandwidth != nil {
		for _, bytes := range a.VariantBandwidth {
			agg.totalBandwidth += bytes
		}
	}
}

// buildAnalyticsResult builds the final analytics result
func (s *Service) buildAnalyticsResult(metrics *analyticsMetrics) *model.StreamingAnalytics {
	return &model.StreamingAnalytics{
		TotalViews:          metrics.totalViews,
		UniqueViewers:       len(metrics.uniqueUsers),
		AverageWatchTime:    s.calculateAvgWatchTime(metrics.totalWatchTimeSeconds, metrics.totalViews),
		QualityDistribution: s.buildQualityDistribution(metrics.qualityViews),
		BufferingEvents:     metrics.bufferingEvents,
		CompletionRate:      s.calculateCompletionRate(metrics.totalSessions, metrics.completedSessions),
	}
}

// calculateAvgWatchTime calculates average watch time
func (s *Service) calculateAvgWatchTime(totalSeconds int64, totalViews int) model.Duration {
	if totalViews > 0 {
		avgSeconds := float64(totalSeconds) / float64(totalViews)
		return model.Duration(int(avgSeconds))
	}
	return 0
}

// calculateCompletionRate calculates completion rate
func (s *Service) calculateCompletionRate(totalSessions, completedSessions int) float64 {
	if totalSessions > 0 {
		return float64(completedSessions) / float64(totalSessions)
	}
	return 0.0
}

// buildQualityDistribution builds quality distribution stats
func (s *Service) buildQualityDistribution(qualityViews map[string]*qualityAggregator) []*model.QualityStats {
	totalViews := s.countTotalQualityViews(qualityViews)
	distribution := s.convertQualityAggregates(qualityViews, totalViews)
	s.sortQualityDistribution(distribution)
	return distribution
}

// countTotalQualityViews counts total views across all qualities
func (s *Service) countTotalQualityViews(qualityViews map[string]*qualityAggregator) int {
	total := 0
	for _, agg := range qualityViews {
		total += agg.viewCount
	}
	return total
}

// convertQualityAggregates converts quality aggregates to stats
func (s *Service) convertQualityAggregates(qualityViews map[string]*qualityAggregator, totalViews int) []*model.QualityStats {
	distribution := make([]*model.QualityStats, 0, len(qualityViews))
	for quality, agg := range qualityViews {
		distribution = append(distribution, s.createQualityStats(quality, agg, totalViews))
	}
	return distribution
}

// createQualityStats creates a single quality stat entry
func (s *Service) createQualityStats(quality string, agg *qualityAggregator, totalViews int) *model.QualityStats {
	percentage := 0.0
	if totalViews > 0 {
		percentage = float64(agg.viewCount) / float64(totalViews) * 100.0
	}

	avgBandwidthMbps := 0.0
	if agg.viewCount > 0 {
		avgBandwidthBytes := float64(agg.totalBandwidth) / float64(agg.viewCount)
		avgBandwidthMbps = (avgBandwidthBytes * 8) / (1000 * 1000)
	}

	return &model.QualityStats{
		Quality:      s.convertQuality(quality),
		ViewCount:    agg.viewCount,
		Percentage:   percentage,
		AvgBandwidth: avgBandwidthMbps,
	}
}

// sortQualityDistribution sorts by view count descending
func (s *Service) sortQualityDistribution(distribution []*model.QualityStats) {
	sort.Slice(distribution, func(i, j int) bool {
		return distribution[i].ViewCount > distribution[j].ViewCount
	})
}

// GetPopularStreams retrieves the most popular streams based on stored popularity data
func (s *Service) GetPopularStreams(ctx context.Context, first int, after *string) (*model.StreamConnection, error) {
	if first < 1 {
		first = 10
	}
	if first > 100 {
		first = 100 // Cap at 100
	}

	// Determine period (default to weekly trending)
	period := "WEEK"

	// Fetch limit+1 for pagination
	limit := first + 1

	// Get popular media from stored aggregates (already sorted by DynamoDB)
	popularityRecords, err := s.popularityRepo.GetPopularMediaByPeriod(ctx, period, limit, after)
	if err != nil {
		s.logger.Error("failed to get popular streams from popularity aggregates",
			zap.Int("limit", limit),
			zap.String("period", period),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get popular streams: %w", err)
	}

	// Handle pagination (cursor from extra item pattern)
	hasNextPage := len(popularityRecords) > first
	if hasNextPage {
		// Trim to requested size
		popularityRecords = popularityRecords[:first]
	}

	// Convert to GraphQL types
	edges := make([]*model.StreamEdge, 0, len(popularityRecords))

	for _, pop := range popularityRecords {
		// Build cursor from popularity record
		cursor := s.encodeStreamCursor(pop.MediaID, pop.Timestamp)

		// Convert popularity aggregate to stream representation
		edges = append(edges, &model.StreamEdge{
			Node: &model.Stream{
				ID:         pop.MediaID,
				MediaID:    pop.MediaID,
				Title:      fmt.Sprintf("Stream %s", pop.MediaID), // Would come from media metadata
				Thumbnail:  fmt.Sprintf("/media/%s/thumbnail.jpg", pop.MediaID),
				Duration:   model.Duration(int(pop.CalculateAvgWatchTime())),
				ViewCount:  int(pop.ViewCount),
				Quality:    s.selectDominantQuality(pop.QualityViews),
				Popularity: pop.PopularityScore,
				CreatedAt:  model.Time(pop.FirstViewed),
			},
			Cursor: model.Cursor(cursor),
		})
	}

	return &model.StreamConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: false, // Forward pagination only
			StartCursor:     s.getFirstCursorPtr(edges),
			EndCursor:       s.getLastCursorPtr(edges),
		},
		TotalCount: len(edges),
	}, nil
}

// GetBandwidthUsage retrieves bandwidth usage report for a given time period
func (s *Service) GetBandwidthUsage(ctx context.Context, period model.TimePeriod) (*model.BandwidthReport, error) {
	// Calculate time range based on period
	endTime := time.Now()
	var startTime time.Time
	switch period {
	case model.TimePeriodHour:
		startTime = endTime.Add(-1 * time.Hour)
	case model.TimePeriodDay:
		startTime = endTime.Add(-24 * time.Hour)
	case model.TimePeriodWeek:
		startTime = endTime.Add(-7 * 24 * time.Hour)
	case model.TimePeriodMonth:
		startTime = endTime.Add(-30 * 24 * time.Hour)
	default:
		startTime = endTime.Add(-24 * time.Hour)
	}

	// Get bandwidth data for the period
	bandwidthData, err := s.analyticsRepo.GetBandwidthByTimeRange(ctx, startTime, endTime, 10000)
	if err != nil {
		s.logger.Error("failed to get bandwidth usage",
			zap.Time("start", startTime),
			zap.Time("end", endTime),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get bandwidth usage: %w", err)
	}

	// Aggregate bandwidth by quality and hour
	qualityBandwidth := make(map[string]*bandwidthAggregator)
	hourlyBandwidth := make(map[int64]*hourlyBandwidthAggregator)
	totalBytes := int64(0)
	peakBitrate := 0.0

	for _, data := range bandwidthData {
		// Aggregate by quality
		if data.Quality != "" {
			if _, exists := qualityBandwidth[data.Quality]; !exists {
				qualityBandwidth[data.Quality] = &bandwidthAggregator{
					totalBytes: 0,
				}
			}
			qualityBandwidth[data.Quality].totalBytes += data.TotalBandwidthBytes
		}

		// Aggregate by hour
		hourKey := data.Timestamp.Truncate(time.Hour).Unix()
		if _, exists := hourlyBandwidth[hourKey]; !exists {
			hourlyBandwidth[hourKey] = &hourlyBandwidthAggregator{
				hour:       data.Timestamp.Truncate(time.Hour),
				totalBytes: 0,
				samples:    0,
			}
		}
		agg := hourlyBandwidth[hourKey]
		agg.totalBytes += data.TotalBandwidthBytes
		agg.samples++

		// Update peak
		if data.TotalBandwidthBytes > 0 {
			// Estimate bitrate (bytes to Mbps)
			bitrate := float64(data.TotalBandwidthBytes*8) / (1000 * 1000 * float64(data.Duration))
			if bitrate > peakBitrate {
				peakBitrate = bitrate
			}
		}

		totalBytes += data.TotalBandwidthBytes
	}

	// Convert to GB
	totalGB := float64(totalBytes) / (1024 * 1024 * 1024)

	// Calculate average Mbps
	durationSeconds := endTime.Sub(startTime).Seconds()
	avgMbps := 0.0
	if durationSeconds > 0 {
		avgMbps = (float64(totalBytes) * 8) / (1000 * 1000 * durationSeconds)
	}

	// Build quality bandwidth breakdown
	byQuality := make([]*model.QualityBandwidth, 0, len(qualityBandwidth))
	for quality, agg := range qualityBandwidth {
		percentage := 0.0
		if totalBytes > 0 {
			percentage = float64(agg.totalBytes) / float64(totalBytes) * 100.0
		}
		qualityGB := float64(agg.totalBytes) / (1024 * 1024 * 1024)

		byQuality = append(byQuality, &model.QualityBandwidth{
			Quality:    s.convertQuality(quality),
			TotalGb:    qualityGB,
			Percentage: percentage,
		})
	}

	// Sort by total GB descending
	sort.Slice(byQuality, func(i, j int) bool {
		return byQuality[i].TotalGb > byQuality[j].TotalGb
	})

	// Build hourly bandwidth breakdown
	byHour := make([]*model.HourlyBandwidth, 0, len(hourlyBandwidth))
	hours := make([]int64, 0, len(hourlyBandwidth))
	for h := range hourlyBandwidth {
		hours = append(hours, h)
	}
	sort.Slice(hours, func(i, j int) bool {
		return hours[i] < hours[j]
	})

	for _, h := range hours {
		agg := hourlyBandwidth[h]
		hourGB := float64(agg.totalBytes) / (1024 * 1024 * 1024)

		// Calculate peak Mbps for this hour
		hourPeakMbps := 0.0
		if agg.samples > 0 {
			hourPeakMbps = (float64(agg.totalBytes) * 8) / (1000 * 1000 * 3600) // Per hour
		}

		byHour = append(byHour, &model.HourlyBandwidth{
			Hour:     model.Time(agg.hour),
			TotalGb:  hourGB,
			PeakMbps: hourPeakMbps,
		})
	}

	// Estimate cost (rough estimate: $0.085 per GB for CloudFront)
	estimatedCost := totalGB * 0.085

	return &model.BandwidthReport{
		Period:    period,
		TotalGb:   totalGB,
		PeakMbps:  peakBitrate,
		AvgMbps:   avgMbps,
		ByQuality: byQuality,
		ByHour:    byHour,
		Cost:      estimatedCost,
	}, nil
}

// Helper types

type analyticsMetrics struct {
	totalViews            int
	totalSessions         int
	completedSessions     int
	bufferingEvents       int
	totalWatchTimeSeconds int64
	uniqueUsers           map[string]bool
	qualityViews          map[string]*qualityAggregator
}

type qualityAggregator struct {
	quality        string
	viewCount      int
	totalBandwidth int64
}

type bandwidthAggregator struct {
	totalBytes int64
}

type hourlyBandwidthAggregator struct {
	hour       time.Time
	totalBytes int64
	samples    int
}

// Helper methods

func (s *Service) convertQuality(quality string) model.StreamQuality {
	switch quality {
	case "low", "480p", "360p":
		return model.StreamQualityLow
	case "medium", "720p":
		return model.StreamQualityMedium
	case "high", "1080p":
		return model.StreamQualityHigh
	case "ultra", "4k", "2160p":
		return model.StreamQualityUltra
	default:
		return model.StreamQualityMedium
	}
}

func (s *Service) selectDominantQuality(qualityViews map[string]int64) model.StreamQuality {
	// Find quality with most views
	maxViews := int64(0)
	dominantQuality := "720p" // default

	for quality, views := range qualityViews {
		if views > maxViews {
			maxViews = views
			dominantQuality = quality
		}
	}

	return s.convertQuality(dominantQuality)
}

func (s *Service) encodeStreamCursor(mediaID string, timestamp time.Time) string {
	// Simple cursor encoding: mediaID:timestamp
	return fmt.Sprintf("%s:%d", mediaID, timestamp.Unix())
}

func (s *Service) getFirstCursorPtr(edges []*model.StreamEdge) *model.Cursor {
	if len(edges) == 0 {
		return nil
	}
	cursor := edges[0].Cursor
	return &cursor
}

func (s *Service) getLastCursorPtr(edges []*model.StreamEdge) *model.Cursor {
	if len(edges) == 0 {
		return nil
	}
	cursor := edges[len(edges)-1].Cursor
	return &cursor
}

// RecordStreamingEvent records a streaming event for analytics ingestion
// and updates popularity aggregates
func (s *Service) RecordStreamingEvent(ctx context.Context, mediaID, userID, eventType, quality string, duration float64, bytesLoaded int64) error {
	if err := common.ValidateRequiredParam("mediaID", mediaID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("eventType", eventType); err != nil {
		return err
	}

	// Create analytics event
	analytics := &models.MediaAnalytics{}
	analytics.SetGeneralEvent(eventType, mediaID, userID)
	analytics.Quality = quality
	analytics.Duration = duration
	analytics.TotalBandwidthBytes = bytesLoaded

	// Store the event
	if err := s.analyticsRepo.StoreMediaAnalytics(ctx, analytics); err != nil {
		s.logger.Error("failed to store streaming event",
			zap.String("mediaID", mediaID),
			zap.String("eventType", eventType),
			zap.Error(err))
		return fmt.Errorf("failed to store streaming event: %w", err)
	}

	// Update popularity aggregates if this is a view event
	if eventType == EventTypeSessionStart {
		// Update all time periods
		for _, period := range []string{"DAY", "WEEK", "MONTH"} {
			if err := s.popularityRepo.IncrementViewCount(ctx, mediaID, period, 1); err != nil {
				s.logger.Warn("failed to update popularity aggregate",
					zap.String("mediaID", mediaID),
					zap.String("period", period),
					zap.Error(err))
				// Don't fail the whole operation if popularity update fails
			}
		}
	}

	return nil
}

// AggregateRollup performs time-windowed rollup aggregation (called by scheduled job)
func (s *Service) AggregateRollup(ctx context.Context, window time.Duration) error {
	s.logger.Info("performing streaming analytics rollup",
		zap.Duration("window", window))

	// This would be called by a Lambda scheduled job to pre-aggregate data
	// For now, we query on-demand but this method provides the hook for future optimization

	endTime := time.Now()
	startTime := endTime.Add(-window)

	// Get all analytics data for the window
	analytics, err := s.analyticsRepo.GetAllMediaAnalyticsByTimeRange(ctx, startTime, endTime, 10000)
	if err != nil {
		return fmt.Errorf("failed to get analytics for rollup: %w", err)
	}

	// Aggregate by media ID
	mediaAggregates := make(map[string]*rollupAggregate)
	for _, a := range analytics {
		if _, exists := mediaAggregates[a.MediaID]; !exists {
			mediaAggregates[a.MediaID] = &rollupAggregate{
				mediaID:    a.MediaID,
				totalViews: 0,
				totalBytes: 0,
			}
		}
		agg := mediaAggregates[a.MediaID]
		if a.EventType == "session_start" {
			agg.totalViews++
		}
		agg.totalBytes += a.TotalBandwidthBytes
	}

	s.logger.Info("rollup aggregation complete",
		zap.Int("mediaCount", len(mediaAggregates)),
		zap.Int("eventCount", len(analytics)))

	return nil
}

type rollupAggregate struct {
	mediaID    string
	totalViews int
	totalBytes int64
}
