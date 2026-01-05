// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockTrackingRepository is a mock implementation of interfaces.TrackingRepository
// using testify/mock for expectation-based testing.
type MockTrackingRepository struct {
	mock.Mock
}

// NewMockTrackingRepository creates a new mock tracking repository
func NewMockTrackingRepository() *MockTrackingRepository {
	return &MockTrackingRepository{}
}

// ===== Core Cost Tracking Operations =====

// Create mocks the Create method
func (m *MockTrackingRepository) Create(ctx context.Context, tracking *models.DynamoDBCostRecord) error {
	args := m.Called(ctx, tracking)
	return args.Error(0)
}

// BatchCreate mocks the BatchCreate method
func (m *MockTrackingRepository) BatchCreate(ctx context.Context, trackingList []*models.DynamoDBCostRecord) error {
	args := m.Called(ctx, trackingList)
	return args.Error(0)
}

// Get mocks the Get method
func (m *MockTrackingRepository) Get(ctx context.Context, operationType, id string, timestamp time.Time) (*models.DynamoDBCostRecord, error) {
	args := m.Called(ctx, operationType, id, timestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.DynamoDBCostRecord), args.Error(1)
}

// ListByOperationType mocks the ListByOperationType method
func (m *MockTrackingRepository) ListByOperationType(ctx context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	args := m.Called(ctx, operationType, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DynamoDBCostRecord), args.Error(1)
}

// ListByTable mocks the ListByTable method
func (m *MockTrackingRepository) ListByTable(ctx context.Context, tableName string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostRecord, string, error) {
	args := m.Called(ctx, tableName, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.DynamoDBCostRecord), args.String(1), args.Error(2)
}

// GetRecentCosts mocks the GetRecentCosts method
func (m *MockTrackingRepository) GetRecentCosts(ctx context.Context, since time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DynamoDBCostRecord), args.Error(1)
}

// ===== Aggregation Operations =====

// GetAggregated mocks the GetAggregated method
func (m *MockTrackingRepository) GetAggregated(ctx context.Context, period, operationType string, windowStart time.Time) (*models.DynamoDBCostAggregation, error) {
	args := m.Called(ctx, period, operationType, windowStart)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.DynamoDBCostAggregation), args.Error(1)
}

// CreateAggregated mocks the CreateAggregated method
func (m *MockTrackingRepository) CreateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error {
	args := m.Called(ctx, aggregated)
	return args.Error(0)
}

// UpdateAggregated mocks the UpdateAggregated method
func (m *MockTrackingRepository) UpdateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error {
	args := m.Called(ctx, aggregated)
	return args.Error(0)
}

// ListAggregatedByPeriod mocks the ListAggregatedByPeriod method
func (m *MockTrackingRepository) ListAggregatedByPeriod(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error) {
	args := m.Called(ctx, period, operationType, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.DynamoDBCostAggregation), args.String(1), args.Error(2)
}

// Aggregate mocks the Aggregate method
func (m *MockTrackingRepository) Aggregate(ctx context.Context, operationType, period string, windowStart, windowEnd time.Time) error {
	args := m.Called(ctx, operationType, period, windowStart, windowEnd)
	return args.Error(0)
}

// GetAggregatedCostsByPeriod mocks the GetAggregatedCostsByPeriod method
func (m *MockTrackingRepository) GetAggregatedCostsByPeriod(ctx context.Context, period string, startDate, endDate time.Time) ([]*models.DynamoDBCostAggregation, error) {
	args := m.Called(ctx, period, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DynamoDBCostAggregation), args.Error(1)
}

// ===== Statistics and Analysis Operations =====

// GetTableCostStats mocks the GetTableCostStats method
func (m *MockTrackingRepository) GetTableCostStats(ctx context.Context, tableName string, startTime, endTime time.Time) (*interfaces.TableCostStats, error) {
	args := m.Called(ctx, tableName, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.TableCostStats), args.Error(1)
}

// GetHighCostOperations mocks the GetHighCostOperations method
func (m *MockTrackingRepository) GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	args := m.Called(ctx, thresholdDollars, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DynamoDBCostRecord), args.Error(1)
}

// GetCostTrends mocks the GetCostTrends method
func (m *MockTrackingRepository) GetCostTrends(ctx context.Context, period string, operationType string, lookbackDays int) (*interfaces.CostTrend, error) {
	args := m.Called(ctx, period, operationType, lookbackDays)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.CostTrend), args.Error(1)
}

// GetCostsByOperationType mocks the GetCostsByOperationType method
func (m *MockTrackingRepository) GetCostsByOperationType(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
	args := m.Called(ctx, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*models.DynamoDBServiceCostStats), args.Error(1)
}

// GetCostsByService mocks the GetCostsByService method
func (m *MockTrackingRepository) GetCostsByService(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
	args := m.Called(ctx, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*models.DynamoDBServiceCostStats), args.Error(1)
}

// GetCostsByDateRange mocks the GetCostsByDateRange method
func (m *MockTrackingRepository) GetCostsByDateRange(ctx context.Context, startDate, endDate time.Time) ([]*models.DynamoDBCostRecord, error) {
	args := m.Called(ctx, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DynamoDBCostRecord), args.Error(1)
}

// GetDailyAggregates mocks the GetDailyAggregates method
func (m *MockTrackingRepository) GetDailyAggregates(ctx context.Context, startDate, endDate time.Time) ([]*interfaces.DailyAggregate, error) {
	args := m.Called(ctx, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*interfaces.DailyAggregate), args.Error(1)
}

// GetMonthlyAggregate mocks the GetMonthlyAggregate method
func (m *MockTrackingRepository) GetMonthlyAggregate(ctx context.Context, year, month int) (*interfaces.MonthlyAggregate, error) {
	args := m.Called(ctx, year, month)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.MonthlyAggregate), args.Error(1)
}

// GetCostProjections mocks the GetCostProjections method
func (m *MockTrackingRepository) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CostProjection), args.Error(1)
}

// ===== Relay Cost Operations =====

// CreateRelayCost mocks the CreateRelayCost method
func (m *MockTrackingRepository) CreateRelayCost(ctx context.Context, relayCost *models.RelayCost) error {
	args := m.Called(ctx, relayCost)
	return args.Error(0)
}

// GetRelayCostsByURL mocks the GetRelayCostsByURL method
func (m *MockTrackingRepository) GetRelayCostsByURL(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int, cursor string, operationType string) ([]*models.RelayCost, string, error) {
	args := m.Called(ctx, relayURL, startTime, endTime, limit, cursor, operationType)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.RelayCost), args.String(1), args.Error(2)
}

// GetRelayCostsByDateRange mocks the GetRelayCostsByDateRange method
func (m *MockTrackingRepository) GetRelayCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.RelayCost, error) {
	args := m.Called(ctx, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.RelayCost), args.Error(1)
}

// ===== Relay Metrics Operations =====

// CreateRelayMetrics mocks the CreateRelayMetrics method
func (m *MockTrackingRepository) CreateRelayMetrics(ctx context.Context, metrics *models.RelayMetrics) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

// UpdateRelayMetrics mocks the UpdateRelayMetrics method
func (m *MockTrackingRepository) UpdateRelayMetrics(ctx context.Context, metrics *models.RelayMetrics) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

// GetRelayMetrics mocks the GetRelayMetrics method
func (m *MockTrackingRepository) GetRelayMetrics(ctx context.Context, relayURL, period string, windowStart time.Time) (*models.RelayMetrics, error) {
	args := m.Called(ctx, relayURL, period, windowStart)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.RelayMetrics), args.Error(1)
}

// GetRelayMetricsHistory mocks the GetRelayMetricsHistory method
func (m *MockTrackingRepository) GetRelayMetricsHistory(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int, cursor string) ([]*models.RelayMetrics, string, error) {
	args := m.Called(ctx, relayURL, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.RelayMetrics), args.String(1), args.Error(2)
}

// ===== Relay Budget Operations =====

// CreateRelayBudget mocks the CreateRelayBudget method
func (m *MockTrackingRepository) CreateRelayBudget(ctx context.Context, budget *models.RelayBudget) error {
	args := m.Called(ctx, budget)
	return args.Error(0)
}

// Ensure MockTrackingRepository implements interfaces.TrackingRepository
var _ interfaces.TrackingRepository = (*MockTrackingRepository)(nil)
