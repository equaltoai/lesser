package streaminganalytics

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock repository implementations
type mockAnalyticsRepo struct {
	analytics []*models.MediaAnalytics
	err       error
}

func (m *mockAnalyticsRepo) GetMediaAnalyticsByTimeRange(ctx context.Context, mediaID string, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	if m.err != nil {
		return nil, m.err
	}
	results := make([]*models.MediaAnalytics, 0)
	for _, a := range m.analytics {
		if a.MediaID == mediaID && !a.Timestamp.Before(startTime) && !a.Timestamp.After(endTime) {
			results = append(results, a)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockAnalyticsRepo) GetAllMediaAnalyticsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	if m.err != nil {
		return nil, m.err
	}
	results := make([]*models.MediaAnalytics, 0)
	for _, a := range m.analytics {
		if !a.Timestamp.Before(startTime) && !a.Timestamp.After(endTime) {
			results = append(results, a)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockAnalyticsRepo) GetPopularMedia(ctx context.Context, startTime, endTime time.Time, limit int, cursor *string) ([]*models.MediaAnalytics, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return analytics sorted by StreamingSessions descending
	results, _ := m.GetAllMediaAnalyticsByTimeRange(ctx, startTime, endTime, limit)
	return results, nil
}

func (m *mockAnalyticsRepo) GetBandwidthByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	if m.err != nil {
		return nil, m.err
	}
	results := make([]*models.MediaAnalytics, 0)
	for _, a := range m.analytics {
		if !a.Timestamp.Before(startTime) && !a.Timestamp.After(endTime) && a.TotalBandwidthBytes > 0 {
			results = append(results, a)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *mockAnalyticsRepo) StoreMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	if m.err != nil {
		return m.err
	}
	m.analytics = append(m.analytics, analytics)
	return nil
}

type mockPopularityRepo struct {
	popularityRecords []*models.MediaPopularity
	err               error
	incrementCalls    []incrementCall
}

type incrementCall struct {
	mediaID string
	period  string
	count   int64
}

func (m *mockPopularityRepo) GetPopularMediaByPeriod(ctx context.Context, period string, limit int, cursor *string) ([]*models.MediaPopularity, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.popularityRecords, nil
}

func (m *mockPopularityRepo) IncrementViewCount(ctx context.Context, mediaID, period string, incrementBy int64) error {
	if m.err != nil {
		return m.err
	}
	// Track the increment call
	m.incrementCalls = append(m.incrementCalls, incrementCall{
		mediaID: mediaID,
		period:  period,
		count:   incrementBy,
	})
	return nil
}

func (m *mockPopularityRepo) UpsertPopularity(ctx context.Context, popularity *models.MediaPopularity) error {
	if m.err != nil {
		return m.err
	}
	m.popularityRecords = append(m.popularityRecords, popularity)
	return nil
}

type mockSessionRepo struct{}

// TestGetStreamingAnalytics_WithViews tests analytics with real view data
func TestGetStreamingAnalytics_WithViews(t *testing.T) {
	now := time.Now()

	// Create test data: 3 session starts, 2 completions, 1 buffering event
	analytics := []*models.MediaAnalytics{
		{
			MediaID:   "media-1",
			EventType: "session_start",
			UserID:    "user-1",
			Quality:   "720p",
			Duration:  120.0,
			Timestamp: now.Add(-1 * time.Hour),
			Date:      now.Format(common.DateFormat),
			VariantBandwidth: map[string]int64{
				"720p_h264_4000": 50 * 1024 * 1024, // 50 MB
			},
		},
		{
			MediaID:   "media-1",
			EventType: "session_start",
			UserID:    "user-2",
			Quality:   "1080p",
			Duration:  180.0,
			Timestamp: now.Add(-2 * time.Hour),
			Date:      now.Format(common.DateFormat),
			VariantBandwidth: map[string]int64{
				"1080p_h264_8000": 100 * 1024 * 1024, // 100 MB
			},
		},
		{
			MediaID:   "media-1",
			EventType: "session_start",
			UserID:    "user-1", // Same user, different session
			Quality:   "720p",
			Duration:  90.0,
			Timestamp: now.Add(-3 * time.Hour),
			Date:      now.Format(common.DateFormat),
		},
		{
			MediaID:   "media-1",
			EventType: "session_end",
			UserID:    "user-1",
			Duration:  110.0,
			Timestamp: now.Add(-90 * time.Minute),
			Date:      now.Format(common.DateFormat),
		},
		{
			MediaID:   "media-1",
			EventType: "session_end",
			UserID:    "user-2",
			Duration:  170.0,
			Timestamp: now.Add(-2 * time.Hour),
			Date:      now.Format(common.DateFormat),
		},
		{
			MediaID:   "media-1",
			EventType: "rebuffer_start",
			UserID:    "user-2",
			Timestamp: now.Add(-2 * time.Hour),
			Date:      now.Format(common.DateFormat),
		},
	}

	mockRepo := &mockAnalyticsRepo{analytics: analytics}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	result, err := service.GetStreamingAnalytics(context.Background(), "media-1")

	require.NoError(t, err)
	require.NotNil(t, result)

	// Assert specific values
	assert.Equal(t, 3, result.TotalViews, "Should count 3 session_start events")
	assert.Equal(t, 2, result.UniqueViewers, "Should count 2 unique users")
	assert.Equal(t, 1, result.BufferingEvents, "Should count 1 rebuffer_start event")
	assert.Equal(t, 2.0/3.0, result.CompletionRate, "Should be 2 completions / 3 starts = 0.666...")

	// Average watch time: sum of ALL event durations / total views
	// (120 + 180 + 90 + 110 + 170 + 0) / 3 = 670 / 3 = 223.33 seconds
	assert.Equal(t, model.Duration(223), result.AverageWatchTime)

	// Quality distribution
	require.Len(t, result.QualityDistribution, 2, "Should have 2 quality levels")

	// Find 720p and 1080p in distribution
	var q720p, q1080p *model.QualityStats
	for _, q := range result.QualityDistribution {
		if q.Quality == model.StreamQualityMedium {
			q720p = q
		} else if q.Quality == model.StreamQualityHigh {
			q1080p = q
		}
	}

	require.NotNil(t, q720p, "Should have 720p quality stats")
	require.NotNil(t, q1080p, "Should have 1080p quality stats")

	assert.Equal(t, 2, q720p.ViewCount, "720p should have 2 views")
	assert.Equal(t, 1, q1080p.ViewCount, "1080p should have 1 view")

	// Percentages: 720p=66.66%, 1080p=33.33%
	assert.InDelta(t, 66.66, q720p.Percentage, 0.1)
	assert.InDelta(t, 33.33, q1080p.Percentage, 0.1)
}

// TestGetStreamingAnalytics_NoData tests empty analytics
func TestGetStreamingAnalytics_NoData(t *testing.T) {
	mockRepo := &mockAnalyticsRepo{analytics: []*models.MediaAnalytics{}}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	result, err := service.GetStreamingAnalytics(context.Background(), "media-nonexistent")

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 0, result.TotalViews)
	assert.Equal(t, 0, result.UniqueViewers)
	assert.Equal(t, model.Duration(0), result.AverageWatchTime)
	assert.Equal(t, 0.0, result.CompletionRate)
	assert.Equal(t, 0, result.BufferingEvents)
	assert.Len(t, result.QualityDistribution, 0)
}

// TestGetBandwidthUsage_Day tests bandwidth calculation for DAY period
func TestGetBandwidthUsage_Day(t *testing.T) {
	now := time.Now()

	// Create bandwidth data: 150 MB in 720p, 100 MB in 1080p
	analytics := []*models.MediaAnalytics{
		{
			MediaID:             "media-1",
			Quality:             "720p",
			Duration:            3600.0,            // 1 hour
			TotalBandwidthBytes: 150 * 1024 * 1024, // 150 MB
			Timestamp:           now.Add(-1 * time.Hour),
			Date:                now.Format(common.DateFormat),
		},
		{
			MediaID:             "media-2",
			Quality:             "1080p",
			Duration:            3600.0,            // 1 hour
			TotalBandwidthBytes: 100 * 1024 * 1024, // 100 MB
			Timestamp:           now.Add(-2 * time.Hour),
			Date:                now.Format(common.DateFormat),
		},
	}

	mockRepo := &mockAnalyticsRepo{analytics: analytics}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	result, err := service.GetBandwidthUsage(context.Background(), model.TimePeriodDay)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Total: 250 MB = 0.244140625 GB
	expectedGB := 250.0 / 1024.0
	assert.InDelta(t, expectedGB, result.TotalGb, 0.01, "Should be ~0.244 GB")

	// Cost: 0.244 GB * $0.085/GB ≈ $0.0207
	assert.InDelta(t, expectedGB*0.085, result.Cost, 0.001)

	// Should have 2 quality levels
	require.Len(t, result.ByQuality, 2)

	// Find 720p and 1080p
	var q720p, q1080p *model.QualityBandwidth
	for _, q := range result.ByQuality {
		if q.Quality == model.StreamQualityMedium {
			q720p = q
		} else if q.Quality == model.StreamQualityHigh {
			q1080p = q
		}
	}

	require.NotNil(t, q720p)
	require.NotNil(t, q1080p)

	// 720p: 150/250 = 60%
	assert.InDelta(t, 60.0, q720p.Percentage, 0.1)
	// 1080p: 100/250 = 40%
	assert.InDelta(t, 40.0, q1080p.Percentage, 0.1)
}

// TestGetBandwidthUsage_Week tests WEEK period aggregation
func TestGetBandwidthUsage_Week(t *testing.T) {
	now := time.Now()

	// Create bandwidth data over multiple days
	analytics := []*models.MediaAnalytics{
		{
			MediaID:             "media-1",
			Quality:             "720p",
			Duration:            3600.0,
			TotalBandwidthBytes: 1024 * 1024 * 1024, // 1 GB
			Timestamp:           now.Add(-1 * 24 * time.Hour),
			Date:                now.Add(-1 * 24 * time.Hour).Format(common.DateFormat),
		},
		{
			MediaID:             "media-2",
			Quality:             "720p",
			Duration:            3600.0,
			TotalBandwidthBytes: 2 * 1024 * 1024 * 1024, // 2 GB
			Timestamp:           now.Add(-3 * 24 * time.Hour),
			Date:                now.Add(-3 * 24 * time.Hour).Format(common.DateFormat),
		},
	}

	mockRepo := &mockAnalyticsRepo{analytics: analytics}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	result, err := service.GetBandwidthUsage(context.Background(), model.TimePeriodWeek)

	require.NoError(t, err)
	assert.Equal(t, model.TimePeriodWeek, result.Period)
	assert.InDelta(t, 3.0, result.TotalGb, 0.01, "Should be 3 GB total")

	// Hourly breakdown should exist
	assert.NotEmpty(t, result.ByHour)
}

// TestGetPopularStreams_Pagination tests cursor pagination with real popularity data
func TestGetPopularStreams_Pagination(t *testing.T) {
	now := time.Now()

	// Create popularity records (sorted by view count descending via SK)
	// Include 3 items to test limit+1 pagination (request 2, get 3, trim to 2, hasNextPage=true)
	popularityRecords := []*models.MediaPopularity{
		{
			MediaID:        "media-1",
			ViewCount:      200,
			FirstViewed:    now.Add(-2 * time.Hour),
			Timestamp:      now,
			QualityViews:   map[string]int64{"720p": 150, "1080p": 50},
			TotalWatchTime: 12000, // 60s average (12000/200)
		},
		{
			MediaID:        "media-2",
			ViewCount:      100,
			FirstViewed:    now.Add(-1 * time.Hour),
			Timestamp:      now,
			QualityViews:   map[string]int64{"720p": 100},
			TotalWatchTime: 6000, // 60s average (6000/100)
		},
		{
			MediaID:        "media-3",
			ViewCount:      50,
			FirstViewed:    now.Add(-3 * time.Hour),
			Timestamp:      now,
			QualityViews:   map[string]int64{"480p": 50},
			TotalWatchTime: 2250, // 45s average (2250/50)
		},
	}

	mockRepo := &mockAnalyticsRepo{}
	mockPop := &mockPopularityRepo{popularityRecords: popularityRecords}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	// Request first 2 items
	result, err := service.GetPopularStreams(context.Background(), 2, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have exactly 2 edges (limit enforced)
	assert.Len(t, result.Edges, 2, "Should return exactly 2 items")

	// Should indicate more pages available (because we had 3 items)
	assert.NotNil(t, result.PageInfo)
	assert.True(t, result.PageInfo.HasNextPage, "Should have page 3 available")
	assert.NotNil(t, result.PageInfo.StartCursor)
	assert.NotNil(t, result.PageInfo.EndCursor)

	// Verify first edge (highest popularity: media-1 with 200 views)
	assert.Equal(t, "media-1", result.Edges[0].Node.MediaID)
	assert.Equal(t, 200, result.Edges[0].Node.ViewCount)
	assert.Equal(t, model.Duration(60), result.Edges[0].Node.Duration) // 12000/200

	// Verify second edge (media-2 with 100 views)
	assert.Equal(t, "media-2", result.Edges[1].Node.MediaID)
	assert.Equal(t, 100, result.Edges[1].Node.ViewCount)
	assert.Equal(t, model.Duration(60), result.Edges[1].Node.Duration) // 6000/100

	// Cursors should be unique
	assert.NotEqual(t, result.Edges[0].Cursor, result.Edges[1].Cursor)
}

// TestRecordStreamingEvent tests event ingestion
func TestRecordStreamingEvent(t *testing.T) {
	mockRepo := &mockAnalyticsRepo{analytics: []*models.MediaAnalytics{}}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	err := service.RecordStreamingEvent(
		context.Background(),
		"media-123",
		"user-456",
		EventTypeSessionStart,
		"720p",
		0.0,
		0,
	)

	require.NoError(t, err)
	assert.Len(t, mockRepo.analytics, 1, "Should store 1 analytics event")

	stored := mockRepo.analytics[0]
	assert.Equal(t, "media-123", stored.MediaID)
	assert.Equal(t, "user-456", stored.UserID)
	assert.Equal(t, EventTypeSessionStart, stored.EventType)
	assert.Equal(t, "720p", stored.Quality)
}

// TestRecordStreamingEvent_UpdatesPopularity tests that popularity is updated on view events
func TestRecordStreamingEvent_UpdatesPopularity(t *testing.T) {
	mockRepo := &mockAnalyticsRepo{analytics: []*models.MediaAnalytics{}}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	// Record a session start (should trigger popularity updates)
	err := service.RecordStreamingEvent(
		context.Background(),
		"media-123",
		"user-456",
		EventTypeSessionStart,
		"720p",
		0.0,
		0,
	)

	require.NoError(t, err)

	// Verify analytics event was stored
	require.Len(t, mockRepo.analytics, 1, "Should store analytics event")

	// Verify popularity was updated for all 3 periods
	require.Len(t, mockPop.incrementCalls, 3, "Should update DAY, WEEK, MONTH")

	// Check each period was incremented
	periods := make(map[string]bool)
	for _, call := range mockPop.incrementCalls {
		assert.Equal(t, "media-123", call.mediaID)
		assert.Equal(t, int64(1), call.count)
		periods[call.period] = true
	}

	assert.True(t, periods["DAY"], "Should update DAY period")
	assert.True(t, periods["WEEK"], "Should update WEEK period")
	assert.True(t, periods["MONTH"], "Should update MONTH period")
}

// TestRecordStreamingEvent_NonViewEvent tests that non-view events don't update popularity
func TestRecordStreamingEvent_NonViewEvent(t *testing.T) {
	mockRepo := &mockAnalyticsRepo{analytics: []*models.MediaAnalytics{}}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	// Record a session end (should NOT trigger popularity updates)
	err := service.RecordStreamingEvent(
		context.Background(),
		"media-123",
		"user-456",
		EventTypeSessionEnd,
		"720p",
		120.0,
		0,
	)

	require.NoError(t, err)
	assert.Empty(t, mockPop.incrementCalls, "Should not update popularity for session_end events")
}

// TestRecordStreamingEvent_Validation tests parameter validation
func TestRecordStreamingEvent_Validation(t *testing.T) {
	mockRepo := &mockAnalyticsRepo{}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	tests := []struct {
		name      string
		mediaID   string
		eventType string
		wantError bool
	}{
		{"Valid", "media-1", "session_start", false},
		{"Empty mediaID", "", "session_start", true},
		{"Empty eventType", "media-1", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.RecordStreamingEvent(
				context.Background(),
				tt.mediaID,
				"user-1",
				tt.eventType,
				"720p",
				0,
				0,
			)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestConvertQuality tests all quality conversions
func TestConvertQuality(t *testing.T) {
	service := &Service{}

	tests := []struct {
		input    string
		expected model.StreamQuality
	}{
		{"480p", model.StreamQualityLow},
		{"360p", model.StreamQualityLow},
		{"low", model.StreamQualityLow},
		{"720p", model.StreamQualityMedium},
		{"medium", model.StreamQualityMedium},
		{"1080p", model.StreamQualityHigh},
		{"high", model.StreamQualityHigh},
		{"4k", model.StreamQualityUltra},
		{"2160p", model.StreamQualityUltra},
		{"ultra", model.StreamQualityUltra},
		{"unknown", model.StreamQualityMedium}, // Default
		{"", model.StreamQualityMedium},        // Empty defaults to medium
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := service.convertQuality(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetStreamingAnalytics_MultipleQualities tests quality distribution
func TestGetStreamingAnalytics_MultipleQualities(t *testing.T) {
	now := time.Now()

	// 5 views: 3×720p, 2×1080p
	analytics := []*models.MediaAnalytics{
		{MediaID: "media-1", EventType: "session_start", Quality: "720p", UserID: "u1", Timestamp: now, Date: now.Format(common.DateFormat)},
		{MediaID: "media-1", EventType: "session_start", Quality: "720p", UserID: "u2", Timestamp: now, Date: now.Format(common.DateFormat)},
		{MediaID: "media-1", EventType: "session_start", Quality: "720p", UserID: "u3", Timestamp: now, Date: now.Format(common.DateFormat)},
		{MediaID: "media-1", EventType: "session_start", Quality: "1080p", UserID: "u4", Timestamp: now, Date: now.Format(common.DateFormat)},
		{MediaID: "media-1", EventType: "session_start", Quality: "1080p", UserID: "u5", Timestamp: now, Date: now.Format(common.DateFormat)},
	}

	mockRepo := &mockAnalyticsRepo{analytics: analytics}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	result, err := service.GetStreamingAnalytics(context.Background(), "media-1")

	require.NoError(t, err)
	assert.Equal(t, 5, result.TotalViews)
	assert.Equal(t, 5, result.UniqueViewers)

	// Quality distribution: 720p should be first (3 views > 2 views)
	require.Len(t, result.QualityDistribution, 2)
	assert.Equal(t, model.StreamQualityMedium, result.QualityDistribution[0].Quality, "720p should sort first")
	assert.Equal(t, 3, result.QualityDistribution[0].ViewCount)
	assert.InDelta(t, 60.0, result.QualityDistribution[0].Percentage, 0.1) // 3/5 = 60%

	assert.Equal(t, model.StreamQualityHigh, result.QualityDistribution[1].Quality)
	assert.Equal(t, 2, result.QualityDistribution[1].ViewCount)
	assert.InDelta(t, 40.0, result.QualityDistribution[1].Percentage, 0.1) // 2/5 = 40%
}

// TestGetBandwidthUsage_EmptyData tests empty bandwidth report
func TestGetBandwidthUsage_EmptyData(t *testing.T) {
	mockRepo := &mockAnalyticsRepo{analytics: []*models.MediaAnalytics{}}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	result, err := service.GetBandwidthUsage(context.Background(), model.TimePeriodDay)

	require.NoError(t, err)
	assert.Equal(t, 0.0, result.TotalGb)
	assert.Equal(t, 0.0, result.PeakMbps)
	assert.Equal(t, 0.0, result.AvgMbps)
	assert.Equal(t, 0.0, result.Cost)
	assert.Empty(t, result.ByQuality)
	assert.Empty(t, result.ByHour)
}

// TestAggregateRollup tests rollup aggregation
func TestAggregateRollup(t *testing.T) {
	now := time.Now()

	analytics := []*models.MediaAnalytics{
		{
			MediaID:             "media-1",
			EventType:           EventTypeSessionStart,
			TotalBandwidthBytes: 1024 * 1024,
			Timestamp:           now.Add(-1 * time.Hour),
			Date:                now.Format(common.DateFormat),
		},
		{
			MediaID:             "media-2",
			EventType:           EventTypeSessionStart,
			TotalBandwidthBytes: 2 * 1024 * 1024,
			Timestamp:           now.Add(-2 * time.Hour),
			Date:                now.Format(common.DateFormat),
		},
	}

	mockRepo := &mockAnalyticsRepo{analytics: analytics}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	err := service.AggregateRollup(context.Background(), 24*time.Hour)
	assert.NoError(t, err, "Rollup should succeed with valid data")
}

// TestRecordStreamingEvent_MultipleIncrements tests cumulative popularity updates
func TestRecordStreamingEvent_MultipleIncrements(t *testing.T) {
	mockRepo := &mockAnalyticsRepo{analytics: []*models.MediaAnalytics{}}
	mockPop := &mockPopularityRepo{}
	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	// Record first session_start
	err := service.RecordStreamingEvent(
		context.Background(),
		"media-123",
		"user-1",
		EventTypeSessionStart,
		"720p",
		0.0,
		0,
	)
	require.NoError(t, err)

	// Should have 3 increment calls (DAY/WEEK/MONTH)
	assert.Len(t, mockPop.incrementCalls, 3, "First event should trigger 3 increments")

	// Record second session_start (different user)
	err = service.RecordStreamingEvent(
		context.Background(),
		"media-123",
		"user-2",
		EventTypeSessionStart,
		"1080p",
		0.0,
		0,
	)
	require.NoError(t, err)

	// Should now have 6 increment calls total (3 more)
	assert.Len(t, mockPop.incrementCalls, 6, "Second event should trigger 3 more increments")

	// Verify both increments were for the same media
	for _, call := range mockPop.incrementCalls {
		assert.Equal(t, "media-123", call.mediaID)
		assert.Equal(t, int64(1), call.count)
	}

	// Count increments by period
	periodCounts := make(map[string]int)
	for _, call := range mockPop.incrementCalls {
		periodCounts[call.period]++
	}

	assert.Equal(t, 2, periodCounts["DAY"], "Should have 2 DAY increments")
	assert.Equal(t, 2, periodCounts["WEEK"], "Should have 2 WEEK increments")
	assert.Equal(t, 2, periodCounts["MONTH"], "Should have 2 MONTH increments")
}

// TestRecordStreamingEvent_PopularityFailure tests that analytics still succeeds if popularity update fails
func TestRecordStreamingEvent_PopularityFailure(t *testing.T) {
	mockRepo := &mockAnalyticsRepo{analytics: []*models.MediaAnalytics{}}

	// Mock that returns error on IncrementViewCount
	mockPop := &mockPopularityRepo{
		err: assert.AnError, // Will cause IncrementViewCount to fail
	}

	service := NewService(mockRepo, mockPop, &mockSessionRepo{}, zap.NewNop())

	// Record should succeed even if popularity update fails
	err := service.RecordStreamingEvent(
		context.Background(),
		"media-123",
		"user-1",
		EventTypeSessionStart,
		"720p",
		0.0,
		0,
	)

	// Should return nil (we log warning but don't fail the operation)
	require.NoError(t, err, "Analytics storage should succeed even if popularity update fails")

	// Analytics event should still be stored
	assert.Len(t, mockRepo.analytics, 1, "Should store analytics despite popularity failure")
}
