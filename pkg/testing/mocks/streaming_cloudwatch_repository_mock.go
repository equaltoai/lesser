// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockStreamingCloudWatchRepository is a mock implementation of interfaces.StreamingCloudWatchRepository
// using testify/mock for expectation-based testing.
type MockStreamingCloudWatchRepository struct {
	mock.Mock
}

// NewMockStreamingCloudWatchRepository creates a new mock streaming CloudWatch repository
func NewMockStreamingCloudWatchRepository() *MockStreamingCloudWatchRepository {
	return &MockStreamingCloudWatchRepository{}
}

// ===== Quality Metrics Operations =====

// GetQualityBreakdown mocks the GetQualityBreakdown method
func (m *MockStreamingCloudWatchRepository) GetQualityBreakdown(ctx context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error) {
	args := m.Called(ctx, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.StreamingCloudWatchMetrics), args.Error(1)
}

// CacheQualityBreakdown mocks the CacheQualityBreakdown method
func (m *MockStreamingCloudWatchRepository) CacheQualityBreakdown(ctx context.Context, mediaID string, qualityMetrics map[string]models.QualityMetric) error {
	args := m.Called(ctx, mediaID, qualityMetrics)
	return args.Error(0)
}

// ===== Geographic Metrics Operations =====

// GetGeographicData mocks the GetGeographicData method
func (m *MockStreamingCloudWatchRepository) GetGeographicData(ctx context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error) {
	args := m.Called(ctx, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.StreamingCloudWatchMetrics), args.Error(1)
}

// CacheGeographicData mocks the CacheGeographicData method
func (m *MockStreamingCloudWatchRepository) CacheGeographicData(ctx context.Context, mediaID string, geoMetrics map[string]models.GeographicMetric) error {
	args := m.Called(ctx, mediaID, geoMetrics)
	return args.Error(0)
}

// ===== Concurrent Viewer Operations =====

// GetConcurrentViewers mocks the GetConcurrentViewers method
func (m *MockStreamingCloudWatchRepository) GetConcurrentViewers(ctx context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error) {
	args := m.Called(ctx, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.StreamingCloudWatchMetrics), args.Error(1)
}

// CacheConcurrentViewers mocks the CacheConcurrentViewers method
func (m *MockStreamingCloudWatchRepository) CacheConcurrentViewers(ctx context.Context, mediaID string, concurrentMetrics models.ConcurrentViewerMetrics) error {
	args := m.Called(ctx, mediaID, concurrentMetrics)
	return args.Error(0)
}

// ===== Performance Metrics Operations =====

// GetPerformanceMetrics mocks the GetPerformanceMetrics method
func (m *MockStreamingCloudWatchRepository) GetPerformanceMetrics(ctx context.Context, mediaID string) (*models.StreamingCloudWatchMetrics, error) {
	args := m.Called(ctx, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.StreamingCloudWatchMetrics), args.Error(1)
}

// CachePerformanceMetrics mocks the CachePerformanceMetrics method
func (m *MockStreamingCloudWatchRepository) CachePerformanceMetrics(ctx context.Context, mediaID string, perfMetrics models.StreamingPerformanceMetrics) error {
	args := m.Called(ctx, mediaID, perfMetrics)
	return args.Error(0)
}

// ===== Aggregate Operations =====

// GetAllCachedMetrics mocks the GetAllCachedMetrics method
func (m *MockStreamingCloudWatchRepository) GetAllCachedMetrics(ctx context.Context, mediaID string) (map[string]*models.StreamingCloudWatchMetrics, error) {
	args := m.Called(ctx, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*models.StreamingCloudWatchMetrics), args.Error(1)
}

// ===== Cleanup Operations =====

// CleanupExpiredMetrics mocks the CleanupExpiredMetrics method
func (m *MockStreamingCloudWatchRepository) CleanupExpiredMetrics(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Ensure MockStreamingCloudWatchRepository implements interfaces.StreamingCloudWatchRepository
var _ interfaces.StreamingCloudWatchRepository = (*MockStreamingCloudWatchRepository)(nil)
