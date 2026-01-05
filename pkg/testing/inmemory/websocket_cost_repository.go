// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// WebSocketCostRepository is a thread-safe in-memory implementation of interfaces.WebSocketCostRepository.
type WebSocketCostRepository struct {
	mu sync.RWMutex
	// records stores cost records keyed by "operationType:id:timestamp"
	records map[string]*models.WebSocketCostRecord
	// recordsByOp stores record keys by operation type
	recordsByOp map[string][]string
	// recordsByConn stores record keys by connection ID
	recordsByConn map[string][]string
	// recordsByUser stores record keys by user ID
	recordsByUser map[string][]string
	// budgets stores budgets keyed by "userID:period"
	budgets map[string]*models.WebSocketCostBudget
	// aggregations stores aggregations
	aggregations map[string]*models.WebSocketCostAggregation
}

// NewWebSocketCostRepository creates a new in-memory WebSocket cost repository
func NewWebSocketCostRepository() *WebSocketCostRepository {
	return &WebSocketCostRepository{
		records:       make(map[string]*models.WebSocketCostRecord),
		recordsByOp:   make(map[string][]string),
		recordsByConn: make(map[string][]string),
		recordsByUser: make(map[string][]string),
		budgets:       make(map[string]*models.WebSocketCostBudget),
		aggregations:  make(map[string]*models.WebSocketCostAggregation),
	}
}

func (r *WebSocketCostRepository) makeKey(opType, id string, ts time.Time) string {
	return opType + ":" + id + ":" + ts.Format(time.RFC3339Nano)
}

// CreateRecord creates a new WebSocket cost tracking record
func (r *WebSocketCostRepository) CreateRecord(_ context.Context, record *models.WebSocketCostRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(record.OperationType, record.ID, record.Timestamp)
	r.records[key] = record
	r.recordsByOp[record.OperationType] = append(r.recordsByOp[record.OperationType], key)
	r.recordsByConn[record.ConnectionID] = append(r.recordsByConn[record.ConnectionID], key)
	r.recordsByUser[record.UserID] = append(r.recordsByUser[record.UserID], key)
	return nil
}

// Create creates a new WebSocket cost tracking record (legacy)
func (r *WebSocketCostRepository) Create(ctx context.Context, record *models.WebSocketCostRecord) error {
	return r.CreateRecord(ctx, record)
}

// BatchCreate creates multiple WebSocket cost tracking records
func (r *WebSocketCostRepository) BatchCreate(ctx context.Context, records []*models.WebSocketCostRecord) error {
	for _, record := range records {
		if err := r.CreateRecord(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

// GetRecord retrieves a WebSocket cost tracking record
func (r *WebSocketCostRepository) GetRecord(_ context.Context, operationType, id string, timestamp time.Time) (*models.WebSocketCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := r.makeKey(operationType, id, timestamp)
	record, exists := r.records[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return record, nil
}

// Get retrieves a WebSocket cost tracking record (legacy)
func (r *WebSocketCostRepository) Get(ctx context.Context, operationType, id string, timestamp time.Time) (*models.WebSocketCostRecord, error) {
	return r.GetRecord(ctx, operationType, id, timestamp)
}

// ListByOperationType lists WebSocket cost tracking records by operation type
func (r *WebSocketCostRepository) ListByOperationType(_ context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.WebSocketCostRecord
	for _, key := range r.recordsByOp[operationType] {
		if record, exists := r.records[key]; exists {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				results = append(results, record)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// ListByConnection lists WebSocket cost tracking records by connection ID
func (r *WebSocketCostRepository) ListByConnection(_ context.Context, connectionID string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.WebSocketCostRecord
	for _, key := range r.recordsByConn[connectionID] {
		if record, exists := r.records[key]; exists {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				results = append(results, record)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// ListByUser lists WebSocket cost tracking records by user ID
func (r *WebSocketCostRepository) ListByUser(_ context.Context, userID string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.WebSocketCostRecord
	for _, key := range r.recordsByUser[userID] {
		if record, exists := r.records[key]; exists {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				results = append(results, record)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// GetRecentCosts retrieves recent WebSocket cost tracking records
func (r *WebSocketCostRepository) GetRecentCosts(_ context.Context, since time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.WebSocketCostRecord
	for _, record := range r.records {
		if record.Timestamp.After(since) {
			results = append(results, record)
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// GetConnectionCostSummary calculates cost summary for a specific connection
func (r *WebSocketCostRepository) GetConnectionCostSummary(_ context.Context, connectionID string, startTime, endTime time.Time) (*interfaces.WebSocketConnectionCostSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summary := &interfaces.WebSocketConnectionCostSummary{
		ConnectionID:       connectionID,
		StartTime:          startTime,
		EndTime:            endTime,
		OperationBreakdown: make(map[string]*interfaces.WebSocketOperationCostStats),
	}

	for _, key := range r.recordsByConn[connectionID] {
		if record, exists := r.records[key]; exists {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				summary.Count++
				summary.TotalCostMicroCents += record.TotalCostMicroCents
			}
		}
	}
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1000000.0
	return summary, nil
}

// GetUserCostSummary calculates cost summary for a specific user
func (r *WebSocketCostRepository) GetUserCostSummary(_ context.Context, userID string, startTime, endTime time.Time) (*interfaces.WebSocketUserCostSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summary := &interfaces.WebSocketUserCostSummary{
		UserID:             userID,
		StartTime:          startTime,
		EndTime:            endTime,
		OperationBreakdown: make(map[string]*interfaces.WebSocketOperationCostStats),
		StreamBreakdown:    make(map[string]*interfaces.WebSocketStreamCostStats),
	}

	for _, key := range r.recordsByUser[userID] {
		if record, exists := r.records[key]; exists {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				summary.Count++
				summary.TotalCostMicroCents += record.TotalCostMicroCents
			}
		}
	}
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1000000.0
	return summary, nil
}

// CreateBudget creates a new WebSocket cost budget for a user
func (r *WebSocketCostRepository) CreateBudget(_ context.Context, budget *models.WebSocketCostBudget) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := budget.UserID + ":" + budget.Period
	r.budgets[key] = budget
	return nil
}

// UpdateBudget updates an existing WebSocket cost budget
func (r *WebSocketCostRepository) UpdateBudget(_ context.Context, budget *models.WebSocketCostBudget) error {
	return r.CreateBudget(context.Background(), budget)
}

// GetBudget retrieves WebSocket cost budget for a user and period
func (r *WebSocketCostRepository) GetBudget(_ context.Context, userID, period string) (*models.WebSocketCostBudget, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := userID + ":" + period
	budget, exists := r.budgets[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return budget, nil
}

// GetUserBudgets retrieves all budgets for a user
func (r *WebSocketCostRepository) GetUserBudgets(_ context.Context, userID string) ([]*models.WebSocketCostBudget, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.WebSocketCostBudget
	for key, budget := range r.budgets {
		if len(key) > len(userID) && key[:len(userID)] == userID {
			results = append(results, budget)
		}
	}
	return results, nil
}

// UpdateBudgetUsage updates budget usage based on new cost records
func (r *WebSocketCostRepository) UpdateBudgetUsage(_ context.Context, userID string, additionalCostMicroCents int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, budget := range r.budgets {
		if len(key) > len(userID) && key[:len(userID)] == userID {
			budget.UsedMicroCents += additionalCostMicroCents
		}
	}
	return nil
}

// CheckBudgetLimits checks if a user has exceeded their budget limits
func (r *WebSocketCostRepository) CheckBudgetLimits(_ context.Context, userID string) (*interfaces.BudgetStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := &interfaces.BudgetStatus{
		UserID:          userID,
		Budgets:         make(map[string]*interfaces.BudgetPeriodStatus),
		AllowConnection: true,
		AllowMessages:   true,
	}
	return status, nil
}

// CreateAggregation creates a new WebSocket cost aggregation
func (r *WebSocketCostRepository) CreateAggregation(_ context.Context, aggregation *models.WebSocketCostAggregation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := aggregation.Period + ":" + aggregation.OperationType + ":" + aggregation.WindowStart.Format(time.RFC3339)
	r.aggregations[key] = aggregation
	return nil
}

// UpdateAggregation updates an existing WebSocket cost aggregation
func (r *WebSocketCostRepository) UpdateAggregation(_ context.Context, aggregation *models.WebSocketCostAggregation) error {
	return r.CreateAggregation(context.Background(), aggregation)
}

// GetAggregation retrieves WebSocket cost aggregation
func (r *WebSocketCostRepository) GetAggregation(_ context.Context, period, operationType string, windowStart time.Time) (*models.WebSocketCostAggregation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := period + ":" + operationType + ":" + windowStart.Format(time.RFC3339)
	agg, exists := r.aggregations[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return agg, nil
}

// GetUserAggregation retrieves WebSocket cost aggregation for a specific user
func (r *WebSocketCostRepository) GetUserAggregation(_ context.Context, userID, period, operationType string, windowStart time.Time) (*models.WebSocketCostAggregation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := userID + ":" + period + ":" + operationType + ":" + windowStart.Format(time.RFC3339)
	agg, exists := r.aggregations[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return agg, nil
}

// ListAggregationsByPeriod lists WebSocket cost aggregations for a period
func (r *WebSocketCostRepository) ListAggregationsByPeriod(_ context.Context, period, operationType string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostAggregation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.WebSocketCostAggregation
	for _, agg := range r.aggregations {
		if agg.Period == period && agg.OperationType == operationType {
			if agg.WindowStart.After(startTime) && agg.WindowStart.Before(endTime) {
				results = append(results, agg)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// AggregateWebSocketCosts performs aggregation of raw WebSocket cost data
func (r *WebSocketCostRepository) AggregateWebSocketCosts(_ context.Context, _, _ string, _, _ time.Time) error {
	return nil // No-op for in-memory
}

// Clear clears all data (test helper)
func (r *WebSocketCostRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = make(map[string]*models.WebSocketCostRecord)
	r.recordsByOp = make(map[string][]string)
	r.recordsByConn = make(map[string][]string)
	r.recordsByUser = make(map[string][]string)
	r.budgets = make(map[string]*models.WebSocketCostBudget)
	r.aggregations = make(map[string]*models.WebSocketCostAggregation)
}

// Ensure WebSocketCostRepository implements interfaces.WebSocketCostRepository
var _ interfaces.WebSocketCostRepository = (*WebSocketCostRepository)(nil)
