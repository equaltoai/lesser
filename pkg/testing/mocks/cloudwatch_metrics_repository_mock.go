// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockCloudWatchMetricsRepository is a mock implementation of interfaces.CloudWatchMetricsRepository
// using testify/mock for expectation-based testing.
type MockCloudWatchMetricsRepository struct {
	mock.Mock
}

// NewMockCloudWatchMetricsRepository creates a new mock CloudWatch metrics repository
func NewMockCloudWatchMetricsRepository() *MockCloudWatchMetricsRepository {
	return &MockCloudWatchMetricsRepository{}
}

// ===== Service Metrics Operations =====

// GetServiceMetrics mocks the GetServiceMetrics method
func (m *MockCloudWatchMetricsRepository) GetServiceMetrics(ctx context.Context, serviceName string, period time.Duration) (*interfaces.ServiceMetrics, error) {
	args := m.Called(ctx, serviceName, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.ServiceMetrics), args.Error(1)
}

// GetInstanceMetrics mocks the GetInstanceMetrics method
func (m *MockCloudWatchMetricsRepository) GetInstanceMetrics(ctx context.Context, period time.Duration) (*interfaces.ServiceMetrics, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.ServiceMetrics), args.Error(1)
}

// ===== Cost Operations =====

// GetCostBreakdown mocks the GetCostBreakdown method
func (m *MockCloudWatchMetricsRepository) GetCostBreakdown(ctx context.Context, period time.Duration) (*interfaces.CostBreakdown, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.CostBreakdown), args.Error(1)
}

// ===== Caching Operations =====

// CacheMetrics mocks the CacheMetrics method
func (m *MockCloudWatchMetricsRepository) CacheMetrics(ctx context.Context, serviceName string, metrics *interfaces.ServiceMetrics) error {
	args := m.Called(ctx, serviceName, metrics)
	return args.Error(0)
}

// GetCachedMetrics mocks the GetCachedMetrics method
func (m *MockCloudWatchMetricsRepository) GetCachedMetrics(ctx context.Context, serviceName string) (*interfaces.ServiceMetrics, error) {
	args := m.Called(ctx, serviceName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.ServiceMetrics), args.Error(1)
}

// Ensure MockCloudWatchMetricsRepository implements interfaces.CloudWatchMetricsRepository
var _ interfaces.CloudWatchMetricsRepository = (*MockCloudWatchMetricsRepository)(nil)
