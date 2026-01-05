// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockTrendingRepository is a mock implementation of interfaces.TrendingRepository
// using testify/mock for expectation-based testing.
type MockTrendingRepository struct {
	mock.Mock
}

// NewMockTrendingRepository creates a new mock trending repository
func NewMockTrendingRepository() *MockTrendingRepository {
	return &MockTrendingRepository{}
}

// RecordHashtagUsage mocks the RecordHashtagUsage method
func (m *MockTrendingRepository) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	args := m.Called(ctx, hashtag, statusID, authorID)
	return args.Error(0)
}

// RecordStatusEngagement mocks the RecordStatusEngagement method
func (m *MockTrendingRepository) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	args := m.Called(ctx, statusID, engagementType, userID)
	return args.Error(0)
}

// RecordLinkShare mocks the RecordLinkShare method
func (m *MockTrendingRepository) RecordLinkShare(ctx context.Context, linkURL string, statusID string, authorID string) error {
	args := m.Called(ctx, linkURL, statusID, authorID)
	return args.Error(0)
}

// GetTrendingHashtags mocks the GetTrendingHashtags method
func (m *MockTrendingRepository) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

// GetTrendingStatuses mocks the GetTrendingStatuses method
func (m *MockTrendingRepository) GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingStatus), args.Error(1)
}

// GetTrendingLinks mocks the GetTrendingLinks method
func (m *MockTrendingRepository) GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingLink), args.Error(1)
}


// GetRecentHashtags mocks the GetRecentHashtags method
func (m *MockTrendingRepository) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

// GetRecentStatusesWithEngagement mocks the GetRecentStatusesWithEngagement method
func (m *MockTrendingRepository) GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingStatus), args.Error(1)
}

// GetRecentLinks mocks the GetRecentLinks method
func (m *MockTrendingRepository) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingLink), args.Error(1)
}

// StoreEngagementMetrics mocks the StoreEngagementMetrics method
func (m *MockTrendingRepository) StoreEngagementMetrics(ctx context.Context, metrics *storage.EngagementMetrics) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

// GetEngagementMetrics mocks the GetEngagementMetrics method
func (m *MockTrendingRepository) GetEngagementMetrics(ctx context.Context, statusID string) (*storage.EngagementMetrics, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.EngagementMetrics), args.Error(1)
}

// StoreHashtagTrend mocks the StoreHashtagTrend method
func (m *MockTrendingRepository) StoreHashtagTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

// StoreStatusTrend mocks the StoreStatusTrend method
func (m *MockTrendingRepository) StoreStatusTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

// StoreLinkTrend mocks the StoreLinkTrend method
func (m *MockTrendingRepository) StoreLinkTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

// SetStatusRepository mocks the SetStatusRepository method
func (m *MockTrendingRepository) SetStatusRepository(statusRepo interface{}) {
	m.Called(statusRepo)
}

// Ensure MockTrendingRepository implements interfaces.TrendingRepository
var _ interfaces.TrendingRepository = (*MockTrendingRepository)(nil)
