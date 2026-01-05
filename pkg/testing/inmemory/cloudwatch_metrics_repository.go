// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// CloudWatchMetricsRepository is a thread-safe in-memory implementation of interfaces.CloudWatchMetricsRepository.
type CloudWatchMetricsRepository struct {
	mu            sync.RWMutex
	serviceMetrics map[string]*interfaces.ServiceMetrics // keyed by serviceName
}

// NewCloudWatchMetricsRepository creates a new in-memory CloudWatch metrics repository.
func NewCloudWatchMetricsRepository() *CloudWatchMetricsRepository {
	return &CloudWatchMetricsRepository{
		serviceMetrics: make(map[string]*interfaces.ServiceMetrics),
	}
}

// ===== Service Metrics Operations =====

// GetServiceMetrics retrieves comprehensive metrics for a service over the specified period.
func (r *CloudWatchMetricsRepository) GetServiceMetrics(_ context.Context, serviceName string, _ time.Duration) (*interfaces.ServiceMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if metrics, exists := r.serviceMetrics[serviceName]; exists {
		return metrics, nil
	}

	// Return default empty metrics
	return &interfaces.ServiceMetrics{
		ServiceName: serviceName,
	}, nil
}

// GetInstanceMetrics retrieves instance-level metrics for the past period.
func (r *CloudWatchMetricsRepository) GetInstanceMetrics(ctx context.Context, period time.Duration) (*interfaces.ServiceMetrics, error) {
	return r.GetServiceMetrics(ctx, "instance", period)
}

// ===== Cost Operations =====

// GetCostBreakdown retrieves detailed cost breakdown for the specified period.
func (r *CloudWatchMetricsRepository) GetCostBreakdown(_ context.Context, _ time.Duration) (*interfaces.CostBreakdown, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return default empty cost breakdown
	return &interfaces.CostBreakdown{
		TotalCost:        0,
		DynamoDBCost:     0,
		LambdaCost:       0,
		APIGatewayCost:   0,
		S3Cost:           0,
		DataTransferCost: 0,
		Breakdown:        []*interfaces.CostItem{},
	}, nil
}

// ===== Caching Operations =====

// CacheMetrics stores metrics in DynamoDB for performance optimization.
func (r *CloudWatchMetricsRepository) CacheMetrics(_ context.Context, serviceName string, metrics *interfaces.ServiceMetrics) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.serviceMetrics[serviceName] = metrics
	return nil
}

// GetCachedMetrics retrieves cached metrics from DynamoDB.
func (r *CloudWatchMetricsRepository) GetCachedMetrics(_ context.Context, serviceName string) (*interfaces.ServiceMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if metrics, exists := r.serviceMetrics[serviceName]; exists {
		return metrics, nil
	}

	return nil, nil
}

// ===== Test Helper Methods =====

// Clear clears all cached data.
func (r *CloudWatchMetricsRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.serviceMetrics = make(map[string]*interfaces.ServiceMetrics)
}

// SetServiceMetrics sets metrics for a service (test helper).
func (r *CloudWatchMetricsRepository) SetServiceMetrics(serviceName string, metrics *interfaces.ServiceMetrics) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.serviceMetrics[serviceName] = metrics
}

// Count returns the number of cached service metrics.
func (r *CloudWatchMetricsRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.serviceMetrics)
}

// Ensure CloudWatchMetricsRepository implements interfaces.CloudWatchMetricsRepository
var _ interfaces.CloudWatchMetricsRepository = (*CloudWatchMetricsRepository)(nil)
