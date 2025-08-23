package repositories

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// CostAwareRepository wraps repository operations with comprehensive cost tracking
type CostAwareRepository struct {
	*dynamorm.BaseRepository
	costTracker    *cost.DynamORMCostTracker
	costThresholds Thresholds
	logger         *zap.Logger
	mu             sync.RWMutex
	operationStats map[string]*OperationStats
}

// Thresholds defines warning and limit thresholds for operations
type Thresholds struct {
	// Per-operation thresholds (in cents)
	WarningCostPerOp float64
	MaxCostPerOp     float64

	// Per-request thresholds (in cents)
	WarningCostPerRequest float64
	MaxCostPerRequest     float64

	// Time-based thresholds
	WarningCostPerMinute float64
	MaxCostPerMinute     float64

	// Operation count thresholds
	MaxOperationsPerRequest int
	MaxOperationsPerMinute  int
}

// DefaultCostThresholds returns default cost thresholds aligned with Lesser's cost goals
func DefaultCostThresholds() Thresholds {
	return Thresholds{
		WarningCostPerOp:        0.0001, // $0.0001 (0.01 cents)
		MaxCostPerOp:            0.001,  // $0.001 (0.1 cents)
		WarningCostPerRequest:   0.001,  // $0.001
		MaxCostPerRequest:       0.01,   // $0.01
		WarningCostPerMinute:    0.01,   // $0.01 per minute
		MaxCostPerMinute:        0.1,    // $0.10 per minute
		MaxOperationsPerRequest: 100,    // Max 100 operations per request
		MaxOperationsPerMinute:  1000,   // Max 1000 operations per minute
	}
}

// OperationStats tracks statistics for specific operations
type OperationStats struct {
	TotalOperations int64
	TotalCost       float64
	TotalDuration   time.Duration
	AverageCost     float64
	AverageDuration time.Duration
	LastOperation   time.Time
	ErrorCount      int64
	mu              sync.RWMutex
}

// NewCostAwareRepository creates a repository with comprehensive cost tracking
func NewCostAwareRepository(db core.DB, tableName string, logger *zap.Logger, _ *cost.Tracker) *CostAwareRepository {
	costTracker := cost.WrapWithCostTracking(db, logger)

	return &CostAwareRepository{
		BaseRepository: dynamorm.NewBaseRepository(db, tableName),
		costTracker:    costTracker,
		costThresholds: DefaultCostThresholds(),
		logger:         logger,
		operationStats: make(map[string]*OperationStats),
	}
}

// NewCostAwareRepositoryWithRequest creates a repository with request-scoped cost tracking
func NewCostAwareRepositoryWithRequest(db core.DB, tableName, requestID, operationType string, logger *zap.Logger, _ *cost.Tracker) *CostAwareRepository {
	costTracker := cost.WrapWithCostTrackingAndRequest(db, requestID, operationType, logger)

	return &CostAwareRepository{
		BaseRepository: dynamorm.NewBaseRepository(db, tableName),
		costTracker:    costTracker,
		costThresholds: DefaultCostThresholds(),
		logger:         logger,
		operationStats: make(map[string]*OperationStats),
	}
}

// SetCostThresholds updates the cost thresholds for this repository
func (r *CostAwareRepository) SetCostThresholds(thresholds Thresholds) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.costThresholds = thresholds
}

// trackOperation wraps a repository operation with comprehensive cost tracking
func (r *CostAwareRepository) trackOperation(ctx context.Context, operationName string, fn func() error) error {
	startTime := time.Now()

	// Get initial cost
	initialCost := r.costTracker.CalculateCost()

	// Check pre-operation limits
	if err := r.checkPreOperationLimits(ctx, operationName); err != nil {
		return err
	}

	// Execute operation
	err := r.costTracker.TrackOperation(ctx, operationName, fn)

	// Calculate operation cost
	finalCost := r.costTracker.CalculateCost()
	operationCost := float64(finalCost.TotalCostMicroCents-initialCost.TotalCostMicroCents) / float64(cost.MicroCentsToCents)
	duration := time.Since(startTime)

	// Update operation statistics
	r.updateOperationStats(operationName, operationCost, duration, err != nil)

	// Check post-operation thresholds
	r.checkPostOperationThresholds(operationName, operationCost, duration)

	// Log operation details
	if r.logger != nil {
		fields := []zap.Field{
			zap.String("operation", operationName),
			zap.Float64("cost_cents", operationCost),
			zap.Duration("duration", duration),
			zap.String("table", r.GetTableName()),
		}

		if err != nil {
			fields = append(fields, zap.Error(err))
			r.logger.Error("cost_aware_operation_failed", fields...)
		} else {
			r.logger.Debug("cost_aware_operation_completed", fields...)
		}
	}

	return err
}

// checkPreOperationLimits checks if operation should be allowed based on current costs
func (r *CostAwareRepository) checkPreOperationLimits(ctx context.Context, operationName string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check operation count limits
	if stats, exists := r.operationStats[operationName]; exists {
		// Check operations per minute
		if time.Since(stats.LastOperation) < time.Minute {
			estimatedOpsPerMinute := int(float64(stats.TotalOperations) / time.Since(stats.LastOperation).Minutes())
			if estimatedOpsPerMinute > r.costThresholds.MaxOperationsPerMinute {
				return fmt.Errorf("operation rate limit exceeded: %d ops/min > %d", estimatedOpsPerMinute, r.costThresholds.MaxOperationsPerMinute)
			}
		}
	}

	// Check context for request-level limits
	if requestTracker := cost.FromContext(ctx); requestTracker != nil {
		requestCost := requestTracker.CalculateCost()
		currentRequestCost := float64(requestCost.TotalCostMicroCents) / float64(cost.MicroCentsToCents)

		if currentRequestCost > r.costThresholds.MaxCostPerRequest {
			return fmt.Errorf("request cost limit exceeded: $%.6f > $%.6f", currentRequestCost, r.costThresholds.MaxCostPerRequest)
		}
	}

	return nil
}

// checkPostOperationThresholds checks and logs threshold violations
func (r *CostAwareRepository) checkPostOperationThresholds(operationName string, operationCost float64, _ time.Duration) {
	// Check operation cost thresholds
	if operationCost > r.costThresholds.MaxCostPerOp {
		if r.logger != nil {
			r.logger.Error("operation_cost_limit_exceeded",
				zap.String("operation", operationName),
				zap.Float64("cost", operationCost),
				zap.Float64("limit", r.costThresholds.MaxCostPerOp),
			)
		}
	} else if operationCost > r.costThresholds.WarningCostPerOp {
		if r.logger != nil {
			r.logger.Warn("operation_cost_warning",
				zap.String("operation", operationName),
				zap.Float64("cost", operationCost),
				zap.Float64("warning_threshold", r.costThresholds.WarningCostPerOp),
			)
		}
	}
}

// updateOperationStats updates statistics for an operation
func (r *CostAwareRepository) updateOperationStats(operationName string, cost float64, duration time.Duration, hadError bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stats, exists := r.operationStats[operationName]
	if !exists {
		stats = &OperationStats{}
		r.operationStats[operationName] = stats
	}

	stats.mu.Lock()
	defer stats.mu.Unlock()

	stats.TotalOperations++
	stats.TotalCost += cost
	stats.TotalDuration += duration
	stats.AverageCost = stats.TotalCost / float64(stats.TotalOperations)
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TotalOperations)
	stats.LastOperation = time.Now()

	if hadError {
		stats.ErrorCount++
	}
}

// Cost-aware repository operations

// GetWithCostTracking retrieves an item with cost tracking
func (r *CostAwareRepository) GetWithCostTracking(ctx context.Context, model any, key map[string]any) error {
	return r.trackOperation(ctx, "get", func() error {
		// Use the base repository's GetDB() to get the client
		query := r.GetDB().Model(model)
		for k, v := range key {
			query = query.Where(k, "=", v)
		}
		return query.First(model)
	})
}

// CreateWithCostTracking creates an item with cost tracking
func (r *CostAwareRepository) CreateWithCostTracking(ctx context.Context, model any) error {
	return r.trackOperation(ctx, "create", func() error {
		return r.GetDB().Model(model).Create()
	})
}

// UpdateWithCostTracking updates an item with cost tracking
func (r *CostAwareRepository) UpdateWithCostTracking(ctx context.Context, model any, fields ...string) error {
	return r.trackOperation(ctx, "update", func() error {
		return r.GetDB().Model(model).Update(fields...)
	})
}

// DeleteWithCostTracking deletes an item with cost tracking
func (r *CostAwareRepository) DeleteWithCostTracking(ctx context.Context, model any) error {
	return r.trackOperation(ctx, "delete", func() error {
		return r.GetDB().Model(model).Delete()
	})
}

// QueryWithCostTracking performs a query with cost tracking
func (r *CostAwareRepository) QueryWithCostTracking(ctx context.Context, query core.Query, dest any) error {
	return r.trackOperation(ctx, "query", func() error {
		return query.All(dest)
	})
}

// BatchWriteWithCostTracking performs batch write with cost tracking
func (r *CostAwareRepository) BatchWriteWithCostTracking(ctx context.Context, items []any) error {
	return r.trackOperation(ctx, fmt.Sprintf("batch_write_%d", len(items)), func() error {
		if err := common.ValidateSliceNotEmpty("items", items); err != nil {
			return nil
		}
		// Use first item to determine model type
		return r.GetDB().Model(items[0]).BatchCreate(items)
	})
}

// Cost tracking query helpers

// CostAwareQuery wraps a query with cost tracking
type CostAwareQuery struct {
	query      core.Query
	repository *CostAwareRepository
	ctx        context.Context
}

// NewCostAwareQuery creates a cost-aware query wrapper
func (r *CostAwareRepository) NewCostAwareQuery(ctx context.Context, model any) *CostAwareQuery {
	return &CostAwareQuery{
		query:      r.GetDB().Model(model),
		repository: r,
		ctx:        ctx,
	}
}

// Where adds a where condition
func (cq *CostAwareQuery) Where(field, op string, value any) *CostAwareQuery {
	cq.query = cq.query.Where(field, op, value)
	return cq
}

// Index specifies an index to use
func (cq *CostAwareQuery) Index(indexName string) *CostAwareQuery {
	cq.query = cq.query.Index(indexName)
	return cq
}

// Limit sets the query limit
func (cq *CostAwareQuery) Limit(limit int) *CostAwareQuery {
	cq.query = cq.query.Limit(limit)
	return cq
}

// First retrieves the first result with cost tracking
func (cq *CostAwareQuery) First(dest any) error {
	return cq.repository.trackOperation(cq.ctx, "query_first", func() error {
		return cq.query.First(dest)
	})
}

// All retrieves all results with cost tracking
func (cq *CostAwareQuery) All(dest any) error {
	// Estimate result count for better operation naming
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() == reflect.Ptr && destValue.Elem().Kind() == reflect.Slice {
		// This is a slice pointer, we can estimate based on limit
		operationName := "query_all"
		return cq.repository.trackOperation(cq.ctx, operationName, func() error {
			return cq.query.All(dest)
		})
	}

	return cq.repository.trackOperation(cq.ctx, "query_all", func() error {
		return cq.query.All(dest)
	})
}

// Count counts results with cost tracking
func (cq *CostAwareQuery) Count() (int64, error) {
	var count int64
	err := cq.repository.trackOperation(cq.ctx, "query_count", func() error {
		var err error
		count, err = cq.query.Count()
		return err
	})
	return count, err
}

// Cost reporting and analysis

// GetOperationStats returns statistics for all operations
func (r *CostAwareRepository) GetOperationStats() map[string]*OperationStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]*OperationStats)
	for op, stats := range r.operationStats {
		stats.mu.RLock()
		result[op] = &OperationStats{
			TotalOperations: stats.TotalOperations,
			TotalCost:       stats.TotalCost,
			TotalDuration:   stats.TotalDuration,
			AverageCost:     stats.AverageCost,
			AverageDuration: stats.AverageDuration,
			LastOperation:   stats.LastOperation,
			ErrorCount:      stats.ErrorCount,
		}
		stats.mu.RUnlock()
	}

	return result
}

// GetCostSummary returns a summary of costs for this repository
func (r *CostAwareRepository) GetCostSummary() *RepositoryCostSummary {
	stats := r.GetOperationStats()

	summary := &RepositoryCostSummary{
		TableName:        r.GetTableName(),
		TotalOperations:  0,
		TotalCost:        0,
		TotalErrors:      0,
		OperationSummary: make(map[string]OperationSummary),
	}

	for op, stat := range stats {
		summary.TotalOperations += stat.TotalOperations
		summary.TotalCost += stat.TotalCost
		summary.TotalErrors += stat.ErrorCount

		summary.OperationSummary[op] = OperationSummary{
			Count:           stat.TotalOperations,
			TotalCost:       stat.TotalCost,
			AverageCost:     stat.AverageCost,
			AverageDuration: stat.AverageDuration,
			ErrorRate:       float64(stat.ErrorCount) / float64(stat.TotalOperations),
			LastUsed:        stat.LastOperation,
		}
	}

	if summary.TotalOperations > 0 {
		summary.AverageCostPerOperation = summary.TotalCost / float64(summary.TotalOperations)
		summary.ErrorRate = float64(summary.TotalErrors) / float64(summary.TotalOperations)
	}

	return summary
}

// RepositoryCostSummary provides a summary of repository costs
type RepositoryCostSummary struct {
	TableName               string                      `json:"table_name"`
	TotalOperations         int64                       `json:"total_operations"`
	TotalCost               float64                     `json:"total_cost"`
	AverageCostPerOperation float64                     `json:"average_cost_per_operation"`
	TotalErrors             int64                       `json:"total_errors"`
	ErrorRate               float64                     `json:"error_rate"`
	OperationSummary        map[string]OperationSummary `json:"operation_summary"`
}

// OperationSummary provides a summary for a specific operation
type OperationSummary struct {
	Count           int64         `json:"count"`
	TotalCost       float64       `json:"total_cost"`
	AverageCost     float64       `json:"average_cost"`
	AverageDuration time.Duration `json:"average_duration"`
	ErrorRate       float64       `json:"error_rate"`
	LastUsed        time.Time     `json:"last_used"`
}

// ResetStats resets all operation statistics
func (r *CostAwareRepository) ResetStats() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operationStats = make(map[string]*OperationStats)
}

// GetCostTracker returns the underlying cost tracker
func (r *CostAwareRepository) GetCostTracker() *cost.DynamORMCostTracker {
	return r.costTracker
}

// Cost optimization helpers

// OptimizeQuery analyzes a query and suggests optimizations
func (r *CostAwareRepository) OptimizeQuery(_ context.Context, _ core.Query) *QueryOptimizationSuggestion {
	// This is a basic implementation - in practice, you'd analyze:
	// - Index usage
	// - Filter efficiency
	// - Projection optimization
	// - Pagination patterns

	suggestion := &QueryOptimizationSuggestion{
		OriginalQuery:    "query", // Would capture actual query details
		Suggestions:      make([]string, 0),
		EstimatedSavings: 0,
	}

	// Basic optimization suggestions
	suggestion.Suggestions = append(suggestion.Suggestions,
		"Consider using specific projections to reduce data transfer",
		"Ensure queries use appropriate indexes",
		"Consider pagination for large result sets",
	)

	return suggestion
}

// QueryOptimizationSuggestion provides query optimization recommendations
type QueryOptimizationSuggestion struct {
	OriginalQuery    string   `json:"original_query"`
	Suggestions      []string `json:"suggestions"`
	EstimatedSavings float64  `json:"estimated_savings"`
	Priority         string   `json:"priority"`
}

// Cost alerting

// CostAlert represents a cost threshold violation
type CostAlert struct {
	AlertType      string    `json:"alert_type"`
	Operation      string    `json:"operation"`
	CurrentValue   float64   `json:"current_value"`
	ThresholdValue float64   `json:"threshold_value"`
	Severity       string    `json:"severity"`
	Timestamp      time.Time `json:"timestamp"`
	TableName      string    `json:"table_name"`
	Message        string    `json:"message"`
}

// CheckCostAlerts checks for any cost threshold violations
func (r *CostAwareRepository) CheckCostAlerts() []*CostAlert {
	alerts := make([]*CostAlert, 0)
	stats := r.GetOperationStats()
	now := time.Now()

	for op, stat := range stats {
		// Check average cost alerts
		if stat.AverageCost > r.costThresholds.MaxCostPerOp {
			alerts = append(alerts, &CostAlert{
				AlertType:      "average_operation_cost",
				Operation:      op,
				CurrentValue:   stat.AverageCost,
				ThresholdValue: r.costThresholds.MaxCostPerOp,
				Severity:       "critical",
				Timestamp:      now,
				TableName:      r.GetTableName(),
				Message:        fmt.Sprintf("Average cost for %s exceeds maximum threshold", op),
			})
		} else if stat.AverageCost > r.costThresholds.WarningCostPerOp {
			alerts = append(alerts, &CostAlert{
				AlertType:      "average_operation_cost",
				Operation:      op,
				CurrentValue:   stat.AverageCost,
				ThresholdValue: r.costThresholds.WarningCostPerOp,
				Severity:       "warning",
				Timestamp:      now,
				TableName:      r.GetTableName(),
				Message:        fmt.Sprintf("Average cost for %s exceeds warning threshold", op),
			})
		}

		// Check error rate alerts
		if stat.TotalOperations > 0 {
			errorRate := float64(stat.ErrorCount) / float64(stat.TotalOperations)
			if errorRate > 0.1 { // 10% error rate threshold
				alerts = append(alerts, &CostAlert{
					AlertType:      "error_rate",
					Operation:      op,
					CurrentValue:   errorRate * 100,
					ThresholdValue: 10.0,
					Severity:       "warning",
					Timestamp:      now,
					TableName:      r.GetTableName(),
					Message:        fmt.Sprintf("High error rate for %s: %.1f%%", op, errorRate*100),
				})
			}
		}
	}

	return alerts
}

// Integration with context for request-scoped tracking

// ContextKey is used for storing cost tracking in context
type ContextKey string

const (
	// CostAwareRepoKey is the context key for cost-aware repository
	CostAwareRepoKey ContextKey = "cost_aware_repository"
)

// WithCostAwareRepository adds a cost-aware repository to context
func WithCostAwareRepository(ctx context.Context, repo *CostAwareRepository) context.Context {
	return context.WithValue(ctx, CostAwareRepoKey, repo)
}

// FromContext retrieves a cost-aware repository from context
func FromContext(ctx context.Context) *CostAwareRepository {
	if repo, ok := ctx.Value(CostAwareRepoKey).(*CostAwareRepository); ok {
		return repo
	}
	return nil
}
