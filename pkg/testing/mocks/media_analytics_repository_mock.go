// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockMediaAnalyticsRepository is a mock implementation of interfaces.MediaAnalyticsRepository
// using testify/mock for expectation-based testing.
type MockMediaAnalyticsRepository struct {
	mock.Mock
}

// NewMockMediaAnalyticsRepository creates a new mock media analytics repository
func NewMockMediaAnalyticsRepository() *MockMediaAnalyticsRepository {
	return &MockMediaAnalyticsRepository{}
}

// ===== Core Analytics Operations =====

// RecordMediaAnalytics mocks the RecordMediaAnalytics method
func (m *MockMediaAnalyticsRepository) RecordMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	args := m.Called(ctx, analytics)
	return args.Error(0)
}

// GetMediaAnalyticsByID mocks the GetMediaAnalyticsByID method
func (m *MockMediaAnalyticsRepository) GetMediaAnalyticsByID(ctx context.Context, format string, timestamp time.Time, mediaID string) (*models.MediaAnalytics, error) {
	args := m.Called(ctx, format, timestamp, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaAnalytics), args.Error(1)
}

// UpdateMediaAnalytics mocks the UpdateMediaAnalytics method
func (m *MockMediaAnalyticsRepository) UpdateMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	args := m.Called(ctx, analytics)
	return args.Error(0)
}

// StoreMediaAnalytics mocks the StoreMediaAnalytics method
func (m *MockMediaAnalyticsRepository) StoreMediaAnalytics(ctx context.Context, analytics *models.MediaAnalytics) error {
	args := m.Called(ctx, analytics)
	return args.Error(0)
}

// ===== Analytics Queries =====

// GetMediaAnalyticsByDate mocks the GetMediaAnalyticsByDate method
func (m *MockMediaAnalyticsRepository) GetMediaAnalyticsByDate(ctx context.Context, date string) ([]*models.MediaAnalytics, error) {
	args := m.Called(ctx, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaAnalytics), args.Error(1)
}

// GetMediaAnalyticsByVariant mocks the GetMediaAnalyticsByVariant method
func (m *MockMediaAnalyticsRepository) GetMediaAnalyticsByVariant(ctx context.Context, variantKey string) ([]*models.MediaAnalytics, error) {
	args := m.Called(ctx, variantKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaAnalytics), args.Error(1)
}

// GetMediaAnalyticsByTimeRange mocks the GetMediaAnalyticsByTimeRange method
func (m *MockMediaAnalyticsRepository) GetMediaAnalyticsByTimeRange(ctx context.Context, mediaID string, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	args := m.Called(ctx, mediaID, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaAnalytics), args.Error(1)
}

// GetAllMediaAnalyticsByTimeRange mocks the GetAllMediaAnalyticsByTimeRange method
func (m *MockMediaAnalyticsRepository) GetAllMediaAnalyticsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	args := m.Called(ctx, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaAnalytics), args.Error(1)
}

// ===== Cost and Summary Operations =====

// GetDailyCostSummary mocks the GetDailyCostSummary method
func (m *MockMediaAnalyticsRepository) GetDailyCostSummary(ctx context.Context, date string) (map[string]interface{}, error) {
	args := m.Called(ctx, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// GetTopVariantsByDemand mocks the GetTopVariantsByDemand method
func (m *MockMediaAnalyticsRepository) GetTopVariantsByDemand(ctx context.Context, date string, limit int) ([]map[string]interface{}, error) {
	args := m.Called(ctx, date, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]map[string]interface{}), args.Error(1)
}

// ===== Media View and Behavior Tracking =====

// RecordMediaView mocks the RecordMediaView method
func (m *MockMediaAnalyticsRepository) RecordMediaView(ctx context.Context, mediaID, userID string, duration time.Duration, quality string) error {
	args := m.Called(ctx, mediaID, userID, duration, quality)
	return args.Error(0)
}

// TrackUserBehavior mocks the TrackUserBehavior method
func (m *MockMediaAnalyticsRepository) TrackUserBehavior(ctx context.Context, userID string, behaviorData map[string]interface{}) error {
	args := m.Called(ctx, userID, behaviorData)
	return args.Error(0)
}

// ===== Popularity and Metrics =====

// CalculatePopularityMetrics mocks the CalculatePopularityMetrics method
func (m *MockMediaAnalyticsRepository) CalculatePopularityMetrics(ctx context.Context, mediaID string, days int) (map[string]interface{}, error) {
	args := m.Called(ctx, mediaID, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// GetMediaMetricsForDate mocks the GetMediaMetricsForDate method
func (m *MockMediaAnalyticsRepository) GetMediaMetricsForDate(ctx context.Context, mediaID, date string) (map[string]interface{}, error) {
	args := m.Called(ctx, mediaID, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// ===== Reporting and Recommendations =====

// GenerateAnalyticsReport mocks the GenerateAnalyticsReport method
func (m *MockMediaAnalyticsRepository) GenerateAnalyticsReport(ctx context.Context, startDate, endDate string) (map[string]interface{}, error) {
	args := m.Called(ctx, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// GetContentRecommendations mocks the GetContentRecommendations method
func (m *MockMediaAnalyticsRepository) GetContentRecommendations(ctx context.Context, userID string, limit int) ([]map[string]interface{}, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]map[string]interface{}), args.Error(1)
}

// ===== Bandwidth and Popular Media Queries =====

// GetBandwidthByTimeRange mocks the GetBandwidthByTimeRange method
func (m *MockMediaAnalyticsRepository) GetBandwidthByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.MediaAnalytics, error) {
	args := m.Called(ctx, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaAnalytics), args.Error(1)
}

// GetPopularMedia mocks the GetPopularMedia method
func (m *MockMediaAnalyticsRepository) GetPopularMedia(ctx context.Context, startTime, endTime time.Time, limit int, cursor *string) ([]*models.MediaAnalytics, error) {
	args := m.Called(ctx, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaAnalytics), args.Error(1)
}

// ===== Cleanup Operations =====

// CleanupOldAnalytics mocks the CleanupOldAnalytics method
func (m *MockMediaAnalyticsRepository) CleanupOldAnalytics(ctx context.Context, olderThan time.Duration) error {
	args := m.Called(ctx, olderThan)
	return args.Error(0)
}

// Ensure MockMediaAnalyticsRepository implements interfaces.MediaAnalyticsRepository
var _ interfaces.MediaAnalyticsRepository = (*MockMediaAnalyticsRepository)(nil)
