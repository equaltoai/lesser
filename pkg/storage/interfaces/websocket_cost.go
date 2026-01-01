// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// WebSocketCostRepository defines the interface for WebSocket cost tracking operations.
// This handles WebSocket connection costs, budgets, and aggregations.
type WebSocketCostRepository interface {
	// ===== Core Cost Record Operations =====

	// CreateRecord creates a new WebSocket cost tracking record
	CreateRecord(ctx context.Context, record *models.WebSocketCostRecord) error

	// Create creates a new WebSocket cost tracking record (legacy method name)
	Create(ctx context.Context, record *models.WebSocketCostRecord) error

	// BatchCreate creates multiple WebSocket cost tracking records efficiently
	BatchCreate(ctx context.Context, records []*models.WebSocketCostRecord) error

	// GetRecord retrieves a WebSocket cost tracking record by operation type, timestamp and ID
	GetRecord(ctx context.Context, operationType, id string, timestamp time.Time) (*models.WebSocketCostRecord, error)

	// Get retrieves a WebSocket cost tracking record (legacy method name)
	Get(ctx context.Context, operationType, id string, timestamp time.Time) (*models.WebSocketCostRecord, error)

	// ListByOperationType lists WebSocket cost tracking records by operation type within a time range
	ListByOperationType(ctx context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error)

	// ListByConnection lists WebSocket cost tracking records by connection ID within a time range
	ListByConnection(ctx context.Context, connectionID string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error)

	// ListByUser lists WebSocket cost tracking records by user ID within a time range
	ListByUser(ctx context.Context, userID string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error)

	// GetRecentCosts retrieves recent WebSocket cost tracking records across all operations
	GetRecentCosts(ctx context.Context, since time.Time, limit int) ([]*models.WebSocketCostRecord, error)

	// ===== Cost Summary Operations =====

	// GetConnectionCostSummary calculates cost summary for a specific connection
	GetConnectionCostSummary(ctx context.Context, connectionID string, startTime, endTime time.Time) (*WebSocketConnectionCostSummary, error)

	// GetUserCostSummary calculates cost summary for a specific user
	GetUserCostSummary(ctx context.Context, userID string, startTime, endTime time.Time) (*WebSocketUserCostSummary, error)

	// ===== Budget Management Operations =====

	// CreateBudget creates a new WebSocket cost budget for a user
	CreateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error

	// UpdateBudget updates an existing WebSocket cost budget
	UpdateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error

	// GetBudget retrieves WebSocket cost budget for a user and period
	GetBudget(ctx context.Context, userID, period string) (*models.WebSocketCostBudget, error)

	// GetUserBudgets retrieves all budgets for a user
	GetUserBudgets(ctx context.Context, userID string) ([]*models.WebSocketCostBudget, error)

	// UpdateBudgetUsage updates budget usage based on new cost records
	UpdateBudgetUsage(ctx context.Context, userID string, additionalCostMicroCents int64) error

	// CheckBudgetLimits checks if a user has exceeded their budget limits
	CheckBudgetLimits(ctx context.Context, userID string) (*BudgetStatus, error)

	// ===== Aggregation Operations =====

	// CreateAggregation creates a new WebSocket cost aggregation
	CreateAggregation(ctx context.Context, aggregation *models.WebSocketCostAggregation) error

	// UpdateAggregation updates an existing WebSocket cost aggregation
	UpdateAggregation(ctx context.Context, aggregation *models.WebSocketCostAggregation) error

	// GetAggregation retrieves WebSocket cost aggregation
	GetAggregation(ctx context.Context, period, operationType string, windowStart time.Time) (*models.WebSocketCostAggregation, error)

	// GetUserAggregation retrieves WebSocket cost aggregation for a specific user
	GetUserAggregation(ctx context.Context, userID, period, operationType string, windowStart time.Time) (*models.WebSocketCostAggregation, error)

	// ListAggregationsByPeriod lists WebSocket cost aggregations for a period
	ListAggregationsByPeriod(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostAggregation, error)

	// AggregateWebSocketCosts performs aggregation of raw WebSocket cost data
	AggregateWebSocketCosts(ctx context.Context, operationType, period string, windowStart, windowEnd time.Time) error
}

// WebSocketConnectionCostSummary represents cost summary for a WebSocket connection
type WebSocketConnectionCostSummary struct {
	ConnectionID            string
	UserID                  string
	Username                string
	StartTime               time.Time
	EndTime                 time.Time
	Count                   int
	TotalConnectionMinutes  float64
	TotalMessages           int64
	TotalMessageBytes       int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	TotalOperations         int64
	AverageCostPerOperation float64
	AverageMessageSize      float64
	OperationBreakdown      map[string]*WebSocketOperationCostStats
}

// WebSocketUserCostSummary represents cost summary for a user's WebSocket usage
type WebSocketUserCostSummary struct {
	UserID                    string
	Username                  string
	StartTime                 time.Time
	EndTime                   time.Time
	Count                     int
	TotalConnectionMinutes    float64
	TotalMessages             int64
	TotalMessageBytes         int64
	TotalCostMicroCents       int64
	TotalCostDollars          float64
	TotalOperations           int64
	TotalIdleTime             int64
	UniqueConnections         int64
	UniqueStreams             int64
	AverageCostPerOperation   float64
	AverageMessageSize        float64
	AverageIdleTime           float64
	AverageCostPerConnection  float64
	AverageConnectionDuration float64
	OperationBreakdown        map[string]*WebSocketOperationCostStats
	StreamBreakdown           map[string]*WebSocketStreamCostStats
}

// WebSocketOperationCostStats represents cost statistics for a WebSocket operation type
type WebSocketOperationCostStats struct {
	OperationType         string
	Count                 int64
	TotalCostMicroCents   int64
	TotalCostDollars      float64
	AverageCostMicroCents int64
	TotalProcessingTime   int64
	AverageProcessingTime float64
	TotalMessages         int64
}

// WebSocketStreamCostStats represents cost statistics for a WebSocket stream
type WebSocketStreamCostStats struct {
	StreamName            string
	OperationCount        int64
	MessageCount          int64
	TotalCostMicroCents   int64
	TotalCostDollars      float64
	AverageCostMicroCents int64
}

// BudgetStatus represents the current budget status for a user
type BudgetStatus struct {
	UserID          string
	Budgets         map[string]*BudgetPeriodStatus
	AllowConnection bool
	AllowMessages   bool
	ExceededBudgets []string
	WarningBudgets  []string
}

// BudgetPeriodStatus represents budget status for a specific period
type BudgetPeriodStatus struct {
	Period              string
	BudgetMicroCents    int64
	UsedMicroCents      int64
	UsagePercent        float64
	Status              string
	RemainingMicroCents int64
}
