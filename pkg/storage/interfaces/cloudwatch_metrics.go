// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"
)

// ServiceMetrics represents aggregated metrics for a service.
type ServiceMetrics struct {
	ServiceName       string
	RequestCount      int64
	ErrorCount        int64
	LatencyP50Ms      float64
	LatencyP90Ms      float64
	LatencyP99Ms      float64
	DynamoDBReads     int64
	DynamoDBWrites    int64
	LambdaInvocations int64
	S3Requests        int64
	DataTransferBytes int64
	EstimatedCostUSD  float64
}

// CostBreakdown represents cost breakdown data.
type CostBreakdown struct {
	TotalCost        float64
	DynamoDBCost     float64
	LambdaCost       float64
	APIGatewayCost   float64
	S3Cost           float64
	DataTransferCost float64
	Breakdown        []*CostItem
}

// CostItem represents a single cost item.
type CostItem struct {
	Operation string
	Count     int
	Cost      float64
}

// CloudWatchMetricsRepository defines the interface for CloudWatch metrics operations.
// This handles querying CloudWatch metrics with optional DynamoDB caching.
type CloudWatchMetricsRepository interface {
	// ===== Service Metrics Operations =====

	// GetServiceMetrics retrieves comprehensive metrics for a service over the specified period
	GetServiceMetrics(ctx context.Context, serviceName string, period time.Duration) (*ServiceMetrics, error)

	// GetInstanceMetrics retrieves instance-level metrics for the past period
	GetInstanceMetrics(ctx context.Context, period time.Duration) (*ServiceMetrics, error)

	// ===== Cost Operations =====

	// GetCostBreakdown retrieves detailed cost breakdown for the specified period
	GetCostBreakdown(ctx context.Context, period time.Duration) (*CostBreakdown, error)

	// ===== Caching Operations =====

	// CacheMetrics stores metrics in DynamoDB for performance optimization
	CacheMetrics(ctx context.Context, serviceName string, metrics *ServiceMetrics) error

	// GetCachedMetrics retrieves cached metrics from DynamoDB
	GetCachedMetrics(ctx context.Context, serviceName string) (*ServiceMetrics, error)
}
