// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockWebSocketCostRepository is a mock implementation of interfaces.WebSocketCostRepository
// using testify/mock for expectation-based testing.
type MockWebSocketCostRepository struct {
	mock.Mock
}

// NewMockWebSocketCostRepository creates a new mock WebSocket cost repository
func NewMockWebSocketCostRepository() *MockWebSocketCostRepository {
	return &MockWebSocketCostRepository{}
}

// ===== Core Cost Record Operations =====

// CreateRecord mocks the CreateRecord method
func (m *MockWebSocketCostRepository) CreateRecord(ctx context.Context, record *models.WebSocketCostRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

// Create mocks the Create method
func (m *MockWebSocketCostRepository) Create(ctx context.Context, record *models.WebSocketCostRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

// BatchCreate mocks the BatchCreate method
func (m *MockWebSocketCostRepository) BatchCreate(ctx context.Context, records []*models.WebSocketCostRecord) error {
	args := m.Called(ctx, records)
	return args.Error(0)
}

// GetRecord mocks the GetRecord method
func (m *MockWebSocketCostRepository) GetRecord(ctx context.Context, operationType, id string, timestamp time.Time) (*models.WebSocketCostRecord, error) {
	args := m.Called(ctx, operationType, id, timestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.WebSocketCostRecord), args.Error(1)
}

// Get mocks the Get method
func (m *MockWebSocketCostRepository) Get(ctx context.Context, operationType, id string, timestamp time.Time) (*models.WebSocketCostRecord, error) {
	args := m.Called(ctx, operationType, id, timestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.WebSocketCostRecord), args.Error(1)
}

// ListByOperationType mocks the ListByOperationType method
func (m *MockWebSocketCostRepository) ListByOperationType(ctx context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	args := m.Called(ctx, operationType, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.WebSocketCostRecord), args.Error(1)
}

// ListByConnection mocks the ListByConnection method
func (m *MockWebSocketCostRepository) ListByConnection(ctx context.Context, connectionID string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	args := m.Called(ctx, connectionID, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.WebSocketCostRecord), args.Error(1)
}

// ListByUser mocks the ListByUser method
func (m *MockWebSocketCostRepository) ListByUser(ctx context.Context, userID string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	args := m.Called(ctx, userID, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.WebSocketCostRecord), args.Error(1)
}

// GetRecentCosts mocks the GetRecentCosts method
func (m *MockWebSocketCostRepository) GetRecentCosts(ctx context.Context, since time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.WebSocketCostRecord), args.Error(1)
}

// ===== Cost Summary Operations =====

// GetConnectionCostSummary mocks the GetConnectionCostSummary method
func (m *MockWebSocketCostRepository) GetConnectionCostSummary(ctx context.Context, connectionID string, startTime, endTime time.Time) (*interfaces.WebSocketConnectionCostSummary, error) {
	args := m.Called(ctx, connectionID, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.WebSocketConnectionCostSummary), args.Error(1)
}

// GetUserCostSummary mocks the GetUserCostSummary method
func (m *MockWebSocketCostRepository) GetUserCostSummary(ctx context.Context, userID string, startTime, endTime time.Time) (*interfaces.WebSocketUserCostSummary, error) {
	args := m.Called(ctx, userID, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.WebSocketUserCostSummary), args.Error(1)
}

// ===== Budget Management Operations =====

// CreateBudget mocks the CreateBudget method
func (m *MockWebSocketCostRepository) CreateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error {
	args := m.Called(ctx, budget)
	return args.Error(0)
}

// UpdateBudget mocks the UpdateBudget method
func (m *MockWebSocketCostRepository) UpdateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error {
	args := m.Called(ctx, budget)
	return args.Error(0)
}

// GetBudget mocks the GetBudget method
func (m *MockWebSocketCostRepository) GetBudget(ctx context.Context, userID, period string) (*models.WebSocketCostBudget, error) {
	args := m.Called(ctx, userID, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.WebSocketCostBudget), args.Error(1)
}

// GetUserBudgets mocks the GetUserBudgets method
func (m *MockWebSocketCostRepository) GetUserBudgets(ctx context.Context, userID string) ([]*models.WebSocketCostBudget, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.WebSocketCostBudget), args.Error(1)
}

// UpdateBudgetUsage mocks the UpdateBudgetUsage method
func (m *MockWebSocketCostRepository) UpdateBudgetUsage(ctx context.Context, userID string, additionalCostMicroCents int64) error {
	args := m.Called(ctx, userID, additionalCostMicroCents)
	return args.Error(0)
}

// CheckBudgetLimits mocks the CheckBudgetLimits method
func (m *MockWebSocketCostRepository) CheckBudgetLimits(ctx context.Context, userID string) (*interfaces.BudgetStatus, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.BudgetStatus), args.Error(1)
}

// ===== Aggregation Operations =====

// CreateAggregation mocks the CreateAggregation method
func (m *MockWebSocketCostRepository) CreateAggregation(ctx context.Context, aggregation *models.WebSocketCostAggregation) error {
	args := m.Called(ctx, aggregation)
	return args.Error(0)
}

// UpdateAggregation mocks the UpdateAggregation method
func (m *MockWebSocketCostRepository) UpdateAggregation(ctx context.Context, aggregation *models.WebSocketCostAggregation) error {
	args := m.Called(ctx, aggregation)
	return args.Error(0)
}

// GetAggregation mocks the GetAggregation method
func (m *MockWebSocketCostRepository) GetAggregation(ctx context.Context, period, operationType string, windowStart time.Time) (*models.WebSocketCostAggregation, error) {
	args := m.Called(ctx, period, operationType, windowStart)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.WebSocketCostAggregation), args.Error(1)
}

// GetUserAggregation mocks the GetUserAggregation method
func (m *MockWebSocketCostRepository) GetUserAggregation(ctx context.Context, userID, period, operationType string, windowStart time.Time) (*models.WebSocketCostAggregation, error) {
	args := m.Called(ctx, userID, period, operationType, windowStart)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.WebSocketCostAggregation), args.Error(1)
}

// ListAggregationsByPeriod mocks the ListAggregationsByPeriod method
func (m *MockWebSocketCostRepository) ListAggregationsByPeriod(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostAggregation, error) {
	args := m.Called(ctx, period, operationType, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.WebSocketCostAggregation), args.Error(1)
}

// AggregateWebSocketCosts mocks the AggregateWebSocketCosts method
func (m *MockWebSocketCostRepository) AggregateWebSocketCosts(ctx context.Context, operationType, period string, windowStart, windowEnd time.Time) error {
	args := m.Called(ctx, operationType, period, windowStart, windowEnd)
	return args.Error(0)
}

// Ensure MockWebSocketCostRepository implements interfaces.WebSocketCostRepository
var _ interfaces.WebSocketCostRepository = (*MockWebSocketCostRepository)(nil)
