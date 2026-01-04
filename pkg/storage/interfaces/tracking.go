// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// TrackingRepository defines the interface for cost tracking operations.
// This handles DynamoDB cost tracking, aggregation, relay costs, and metrics.
type TrackingRepository interface {
	// ===== Core Cost Tracking Operations =====

	// Create creates a new cost tracking record
	Create(ctx context.Context, tracking *models.DynamoDBCostRecord) error

	// BatchCreate creates multiple cost tracking records efficiently
	BatchCreate(ctx context.Context, trackingList []*models.DynamoDBCostRecord) error

	// Get retrieves a cost tracking record by operation type, timestamp and ID
	Get(ctx context.Context, operationType, id string, timestamp time.Time) (*models.DynamoDBCostRecord, error)

	// ListByOperationType lists cost tracking records by operation type within a time range
	ListByOperationType(ctx context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error)

	// ListByTable lists cost tracking records by table within a time range
	ListByTable(ctx context.Context, tableName string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostRecord, string, error)

	// GetRecentCosts retrieves recent cost tracking records across all operations
	GetRecentCosts(ctx context.Context, since time.Time, limit int) ([]*models.DynamoDBCostRecord, error)

	// ===== Aggregation Operations =====

	// GetAggregated retrieves aggregated cost tracking
	GetAggregated(ctx context.Context, period, operationType string, windowStart time.Time) (*models.DynamoDBCostAggregation, error)

	// CreateAggregated creates an aggregated cost tracking record
	CreateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error

	// UpdateAggregated updates an existing aggregated cost tracking record
	UpdateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error

	// ListAggregatedByPeriod lists aggregated cost tracking for a period
	ListAggregatedByPeriod(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error)

	// Aggregate performs aggregation of raw cost tracking data
	Aggregate(ctx context.Context, operationType, period string, windowStart, windowEnd time.Time) error

	// GetAggregatedCostsByPeriod retrieves aggregated costs for a specific period
	GetAggregatedCostsByPeriod(ctx context.Context, period string, startDate, endDate time.Time) ([]*models.DynamoDBCostAggregation, error)

	// ===== Statistics and Analysis Operations =====

	// GetTableCostStats calculates cost statistics for a table
	GetTableCostStats(ctx context.Context, tableName string, startTime, endTime time.Time) (*TableCostStats, error)

	// GetHighCostOperations returns operations that exceed a cost threshold
	GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error)

	// GetCostTrends calculates cost trends over time
	GetCostTrends(ctx context.Context, period string, operationType string, lookbackDays int) (*CostTrend, error)

	// GetCostsByOperationType retrieves costs grouped by operation type
	GetCostsByOperationType(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error)

	// GetCostsByService retrieves costs grouped by service/function
	GetCostsByService(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error)

	// GetCostsByDateRange returns individual cost records for the specified date range
	GetCostsByDateRange(ctx context.Context, startDate, endDate time.Time) ([]*models.DynamoDBCostRecord, error)

	// GetDailyAggregates returns aggregated daily costs for the specified date range
	GetDailyAggregates(ctx context.Context, startDate, endDate time.Time) ([]*DailyAggregate, error)

	// GetMonthlyAggregate returns aggregated costs for the specified month
	GetMonthlyAggregate(ctx context.Context, year, month int) (*MonthlyAggregate, error)

	// GetCostProjections retrieves the most recent cost projection for the given period
	GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error)

	// ===== Relay Cost Operations =====

	// CreateRelayCost creates a new relay cost record
	CreateRelayCost(ctx context.Context, relayCost *models.RelayCost) error

	// GetRelayCostsByURL retrieves relay costs for a specific relay URL within a time range
	GetRelayCostsByURL(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int, cursor string, operationType string) ([]*models.RelayCost, string, error)

	// GetRelayCostsByDateRange retrieves relay costs for all relays within a date range
	GetRelayCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.RelayCost, error)

	// ===== Relay Metrics Operations =====

	// CreateRelayMetrics creates or updates relay metrics
	CreateRelayMetrics(ctx context.Context, metrics *models.RelayMetrics) error

	// UpdateRelayMetrics updates existing relay metrics
	UpdateRelayMetrics(ctx context.Context, metrics *models.RelayMetrics) error

	// GetRelayMetrics retrieves relay metrics for a specific relay and period
	GetRelayMetrics(ctx context.Context, relayURL, period string, windowStart time.Time) (*models.RelayMetrics, error)

	// GetRelayMetricsHistory retrieves metrics history for a relay
	GetRelayMetricsHistory(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int, cursor string) ([]*models.RelayMetrics, string, error)

	// ===== Relay Budget Operations =====

	// CreateRelayBudget creates a new relay budget configuration
	CreateRelayBudget(ctx context.Context, budget *models.RelayBudget) error
}

// TableCostStats represents cost statistics for a table
type TableCostStats struct {
	TableName               string
	StartTime               time.Time
	EndTime                 time.Time
	Count                   int
	TotalOperations         int64
	TotalItemCount          int64
	TotalReadCapacityUnits  float64
	TotalWriteCapacityUnits float64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostPerOperation float64
	OperationBreakdown      map[string]OperationCostStats
}

// OperationCostStats represents cost statistics for a specific operation type
type OperationCostStats struct {
	OperationType           string
	Count                   int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostMicroCents   int64
	TotalReadCapacityUnits  float64
	TotalWriteCapacityUnits float64
}

// CostTrend represents cost trend analysis
type CostTrend struct {
	Period          string
	OperationType   string
	StartTime       time.Time
	EndTime         time.Time
	DataPoints      []CostDataPoint
	TotalCost       float64
	AverageCost     float64
	MinCost         float64
	MaxCost         float64
	TrendPercentage float64
}

// CostDataPoint represents a single point in the cost trend
type CostDataPoint struct {
	Timestamp     time.Time
	CostDollars   float64
	Operations    int64
	ReadCapacity  float64
	WriteCapacity float64
}

// DailyAggregate represents aggregated daily costs
type DailyAggregate struct {
	Date            time.Time
	TotalCost       float64
	TotalOperations int64
}

// MonthlyAggregate represents aggregated monthly costs
type MonthlyAggregate struct {
	Year            int
	Month           int
	TotalCost       float64
	TotalOperations int64
}
