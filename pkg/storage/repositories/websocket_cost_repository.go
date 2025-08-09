package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// WebSocketCostRepository handles WebSocket cost tracking persistence
type WebSocketCostRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewWebSocketCostRepository creates a new WebSocket cost tracking repository
func NewWebSocketCostRepository(db core.DB, tableName string, logger *zap.Logger) *WebSocketCostRepository {
	return &WebSocketCostRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Create creates a new WebSocket cost tracking record
func (r *WebSocketCostRepository) Create(ctx context.Context, record *models.WebSocketCostRecord) error {
	// Call BeforeCreate to set up the model
	if err := record.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the cost tracking record
	err := r.db.WithContext(ctx).Model(record).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create WebSocket cost tracking")
	}

	r.logger.Debug("created WebSocket cost tracking",
		zap.String("id", record.ID),
		zap.String("operation_type", record.OperationType),
		zap.String("connection_id", record.ConnectionID),
		zap.String("user_id", record.UserID),
		zap.Float64("cost_dollars", record.EstimatedCostDollars))

	return nil
}

// BatchCreate creates multiple WebSocket cost tracking records efficiently
func (r *WebSocketCostRepository) BatchCreate(ctx context.Context, records []*models.WebSocketCostRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Prepare all records
	for _, record := range records {
		if err := record.BeforeCreate(); err != nil {
			return fmt.Errorf("before create validation failed for record %s: %w", record.ID, err)
		}
	}

	// Create records in batches
	for _, record := range records {
		if err := r.db.WithContext(ctx).Model(record).Create(); err != nil {
			r.logger.Error("failed to create WebSocket cost tracking in batch",
				zap.String("id", record.ID),
				zap.Error(err))
			// Continue with other records
		}
	}

	return nil
}

// Get retrieves a WebSocket cost tracking record by operation type, timestamp and ID
func (r *WebSocketCostRepository) Get(ctx context.Context, operationType, id string, timestamp time.Time) (*models.WebSocketCostRecord, error) {
	record := &models.WebSocketCostRecord{}

	// Construct the keys
	pk := fmt.Sprintf("WS_COST#%s", operationType)
	sk := fmt.Sprintf("ts#%s#%s", timestamp.Format("20060102150405"), id)

	err := r.db.WithContext(ctx).Model(record).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(record)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get WebSocket cost tracking")
	}

	return record, nil
}

// ListByOperationType lists WebSocket cost tracking records by operation type within a time range
func (r *WebSocketCostRepository) ListByOperationType(ctx context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	var records []*models.WebSocketCostRecord

	// Construct SK range for time-based query
	pk := fmt.Sprintf("WS_COST#%s", operationType)
	startSK := fmt.Sprintf("ts#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("ts#%s", endTime.Format("20060102150405"))

	query := r.db.WithContext(ctx).Model(&models.WebSocketCostRecord{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&records)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list WebSocket cost tracking by operation type")
	}

	return records, nil
}

// ListByConnection lists WebSocket cost tracking records by connection ID within a time range
func (r *WebSocketCostRepository) ListByConnection(ctx context.Context, connectionID string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	var records []*models.WebSocketCostRecord

	// Use GSI1 for connection-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.WithContext(ctx).Model(&models.WebSocketCostRecord{}).
		Index("connection-index").
		Where("GSI1PK", "=", fmt.Sprintf("WS_CONN#%s", connectionID)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&records)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list WebSocket cost tracking by connection")
	}

	return records, nil
}

// ListByUser lists WebSocket cost tracking records by user ID within a time range
func (r *WebSocketCostRepository) ListByUser(ctx context.Context, userID string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	var records []*models.WebSocketCostRecord

	// Use GSI2 for user-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.WithContext(ctx).Model(&models.WebSocketCostRecord{}).
		Index("user-index").
		Where("GSI2PK", "=", fmt.Sprintf("WS_USER#%s", userID)).
		Where("GSI2SK", ">=", startSK).
		Where("GSI2SK", "<=", endSK).
		OrderBy("GSI2SK", "DESC").
		Limit(limit)

	err := query.All(&records)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list WebSocket cost tracking by user")
	}

	return records, nil
}

// GetRecentCosts retrieves recent WebSocket cost tracking records across all operations
func (r *WebSocketCostRepository) GetRecentCosts(ctx context.Context, since time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	var allCosts []*models.WebSocketCostRecord

	// Query each operation type
	operationTypes := []string{
		"connect", "disconnect", "message_in", "message_out",
		"subscribe", "unsubscribe", "idle_time", "ping", "error",
	}

	for _, opType := range operationTypes {
		costs, err := r.ListByOperationType(ctx, opType, since, time.Now(), limit/len(operationTypes))
		if err != nil {
			r.logger.Warn("failed to get costs for WebSocket operation type",
				zap.String("operation_type", opType),
				zap.Error(err))
			continue
		}
		allCosts = append(allCosts, costs...)
	}

	// Sort by timestamp (newest first)
	sort.Slice(allCosts, func(i, j int) bool {
		return allCosts[i].Timestamp.After(allCosts[j].Timestamp)
	})

	// Limit results
	if len(allCosts) > limit {
		allCosts = allCosts[:limit]
	}

	return allCosts, nil
}

// GetConnectionCostSummary calculates cost summary for a specific connection
func (r *WebSocketCostRepository) GetConnectionCostSummary(ctx context.Context, connectionID string, startTime, endTime time.Time) (*WebSocketConnectionCostSummary, error) {
	costs, err := r.ListByConnection(ctx, connectionID, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	summary := &WebSocketConnectionCostSummary{
		ConnectionID:       connectionID,
		StartTime:          startTime,
		EndTime:            endTime,
		Count:              len(costs),
		OperationBreakdown: make(map[string]*WebSocketOperationCostStats),
	}

	if summary.Count == 0 {
		return summary, nil
	}

	// Extract user info from first record
	summary.UserID = costs[0].UserID
	summary.Username = costs[0].Username

	// Calculate statistics
	for _, cost := range costs {
		summary.TotalConnectionMinutes += cost.ConnectionDurationMs / (60 * 1000) // Convert ms to minutes
		summary.TotalMessages += int64(cost.MessageCount)
		summary.TotalMessageBytes += cost.MessageSizeBytes
		summary.TotalCostMicroCents += cost.TotalCostMicroCents
		summary.TotalOperations++

		// Track by operation type
		opStats, exists := summary.OperationBreakdown[cost.OperationType]
		if !exists {
			opStats = &WebSocketOperationCostStats{
				OperationType: cost.OperationType,
			}
		}

		opStats.Count++
		opStats.TotalCostMicroCents += cost.TotalCostMicroCents
		opStats.TotalProcessingTime += cost.ProcessingTimeMs
		if cost.MessageCount > 0 {
			opStats.TotalMessages += int64(cost.MessageCount)
		}

		summary.OperationBreakdown[cost.OperationType] = opStats
	}

	// Calculate totals and averages
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1_000_000.0
	if summary.TotalOperations > 0 {
		summary.AverageCostPerOperation = summary.TotalCostDollars / float64(summary.TotalOperations)
	}
	if summary.TotalMessages > 0 {
		summary.AverageMessageSize = float64(summary.TotalMessageBytes) / float64(summary.TotalMessages)
	}

	// Calculate operation averages
	for _, opStats := range summary.OperationBreakdown {
		if opStats.Count > 0 {
			opStats.AverageCostMicroCents = opStats.TotalCostMicroCents / opStats.Count
			opStats.TotalCostDollars = float64(opStats.TotalCostMicroCents) / 1_000_000.0
			opStats.AverageProcessingTime = float64(opStats.TotalProcessingTime) / float64(opStats.Count)
		}
	}

	return summary, nil
}

// GetUserCostSummary calculates cost summary for a specific user
func (r *WebSocketCostRepository) GetUserCostSummary(ctx context.Context, userID string, startTime, endTime time.Time) (*WebSocketUserCostSummary, error) {
	costs, err := r.ListByUser(ctx, userID, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	summary := &WebSocketUserCostSummary{
		UserID:             userID,
		StartTime:          startTime,
		EndTime:            endTime,
		Count:              len(costs),
		OperationBreakdown: make(map[string]*WebSocketOperationCostStats),
		StreamBreakdown:    make(map[string]*WebSocketStreamCostStats),
	}

	if summary.Count == 0 {
		return summary, nil
	}

	// Extract username from first record
	summary.Username = costs[0].Username

	// Track unique connections and streams
	uniqueConnections := make(map[string]bool)
	uniqueStreams := make(map[string]bool)

	// Calculate statistics
	for _, cost := range costs {
		summary.TotalConnectionMinutes += cost.ConnectionDurationMs / (60 * 1000) // Convert ms to minutes
		summary.TotalMessages += int64(cost.MessageCount)
		summary.TotalMessageBytes += cost.MessageSizeBytes
		summary.TotalCostMicroCents += cost.TotalCostMicroCents
		summary.TotalOperations++
		summary.TotalIdleTime += cost.IdleTimeMs

		// Track unique connections
		uniqueConnections[cost.ConnectionID] = true

		// Track unique streams
		for _, stream := range cost.ActiveStreams {
			uniqueStreams[stream] = true
		}

		// Track by operation type
		opStats, exists := summary.OperationBreakdown[cost.OperationType]
		if !exists {
			opStats = &WebSocketOperationCostStats{
				OperationType: cost.OperationType,
			}
			summary.OperationBreakdown[cost.OperationType] = opStats
		}

		opStats.Count++
		opStats.TotalCostMicroCents += cost.TotalCostMicroCents
		opStats.TotalProcessingTime += cost.ProcessingTimeMs
		if cost.MessageCount > 0 {
			opStats.TotalMessages += int64(cost.MessageCount)
		}

		// Track by stream
		for _, stream := range cost.ActiveStreams {
			streamStats, exists := summary.StreamBreakdown[stream]
			if !exists {
				streamStats = &WebSocketStreamCostStats{
					StreamName: stream,
				}
				summary.StreamBreakdown[stream] = streamStats
			}

			streamStats.OperationCount++
			streamStats.TotalCostMicroCents += cost.TotalCostMicroCents
			if cost.MessageCount > 0 {
				streamStats.MessageCount += int64(cost.MessageCount)
			}
		}
	}

	// Set unique counts
	summary.UniqueConnections = int64(len(uniqueConnections))
	summary.UniqueStreams = int64(len(uniqueStreams))

	// Calculate totals and averages
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1_000_000.0
	if summary.TotalOperations > 0 {
		summary.AverageCostPerOperation = summary.TotalCostDollars / float64(summary.TotalOperations)
		summary.AverageIdleTime = float64(summary.TotalIdleTime) / float64(summary.TotalOperations)
	}
	if summary.TotalMessages > 0 {
		summary.AverageMessageSize = float64(summary.TotalMessageBytes) / float64(summary.TotalMessages)
	}

	if summary.UniqueConnections > 0 {
		summary.AverageCostPerConnection = summary.TotalCostDollars / float64(summary.UniqueConnections)
		summary.AverageConnectionDuration = float64(summary.TotalConnectionMinutes) / float64(summary.UniqueConnections)
	}

	// Calculate operation averages
	for _, opStats := range summary.OperationBreakdown {
		if opStats.Count > 0 {
			opStats.AverageCostMicroCents = opStats.TotalCostMicroCents / opStats.Count
			opStats.TotalCostDollars = float64(opStats.TotalCostMicroCents) / 1_000_000.0
			opStats.AverageProcessingTime = float64(opStats.TotalProcessingTime) / float64(opStats.Count)
		}
	}

	// Calculate stream averages
	for _, streamStats := range summary.StreamBreakdown {
		if streamStats.OperationCount > 0 {
			streamStats.AverageCostMicroCents = streamStats.TotalCostMicroCents / streamStats.OperationCount
			streamStats.TotalCostDollars = float64(streamStats.TotalCostMicroCents) / 1_000_000.0
		}
	}

	return summary, nil
}

// Budget Management

// CreateBudget creates a new WebSocket cost budget for a user
func (r *WebSocketCostRepository) CreateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error {
	// Call BeforeCreate to set up the model
	if err := budget.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the budget record
	err := r.db.WithContext(ctx).Model(budget).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create WebSocket cost budget")
	}

	r.logger.Info("created WebSocket cost budget",
		zap.String("user_id", budget.UserID),
		zap.String("period", budget.Period),
		zap.Int64("budget_micro_cents", budget.BudgetMicroCents),
		zap.String("status", budget.Status))

	return nil
}

// UpdateBudget updates an existing WebSocket cost budget
func (r *WebSocketCostRepository) UpdateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error {
	// Call BeforeUpdate to set up the model
	if err := budget.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the budget record
	err := r.db.WithContext(ctx).Model(budget).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update WebSocket cost budget")
	}

	return nil
}

// GetBudget retrieves WebSocket cost budget for a user and period
func (r *WebSocketCostRepository) GetBudget(ctx context.Context, userID, period string) (*models.WebSocketCostBudget, error) {
	budget := &models.WebSocketCostBudget{}

	pk := fmt.Sprintf("WS_BUDGET#%s#%s", userID, period)
	sk := fmt.Sprintf("BUDGET#%s", period)

	err := r.db.WithContext(ctx).Model(budget).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(budget)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get WebSocket cost budget")
	}

	return budget, nil
}

// GetUserBudgets retrieves all budgets for a user
func (r *WebSocketCostRepository) GetUserBudgets(ctx context.Context, userID string) ([]*models.WebSocketCostBudget, error) {
	var budgets []*models.WebSocketCostBudget

	query := r.db.WithContext(ctx).Model(&models.WebSocketCostBudget{}).
		Index("user-budget-index").
		Where("GSI1PK", "=", fmt.Sprintf("WS_USER_BUDGET#%s", userID)).
		OrderBy("GSI1SK", "ASC")

	err := query.All(&budgets)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get user WebSocket budgets")
	}

	return budgets, nil
}

// UpdateBudgetUsage updates budget usage based on new cost records
func (r *WebSocketCostRepository) UpdateBudgetUsage(ctx context.Context, userID string, additionalCostMicroCents int64) error {
	// Get all active budgets for the user
	budgets, err := r.GetUserBudgets(ctx, userID)
	if err != nil {
		r.logger.Warn("failed to get user budgets for usage update",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil // Don't fail the operation if budget tracking fails
	}

	now := time.Now()

	// Update each applicable budget
	for _, budget := range budgets {
		// Check if current time is within budget window
		if now.Before(budget.WindowStart) || now.After(budget.WindowEnd) {
			continue
		}

		// Update usage
		budget.UsedMicroCents += additionalCostMicroCents

		// Update the budget
		if err := r.UpdateBudget(ctx, budget); err != nil {
			r.logger.Error("failed to update budget usage",
				zap.String("user_id", userID),
				zap.String("period", budget.Period),
				zap.Error(err))
			// Continue with other budgets
		}

		// Log budget status changes
		if budget.Status == "exceeded" && budget.UsagePercent >= 100 {
			r.logger.Warn("WebSocket budget exceeded",
				zap.String("user_id", userID),
				zap.String("period", budget.Period),
				zap.Float64("usage_percent", budget.UsagePercent))
		}
	}

	return nil
}

// CheckBudgetLimits checks if a user has exceeded their budget limits
func (r *WebSocketCostRepository) CheckBudgetLimits(ctx context.Context, userID string) (*BudgetStatus, error) {
	budgets, err := r.GetUserBudgets(ctx, userID)
	if err != nil {
		return nil, err
	}

	status := &BudgetStatus{
		UserID:          userID,
		Budgets:         make(map[string]*BudgetPeriodStatus),
		AllowConnection: true,
		AllowMessages:   true,
	}

	now := time.Now()

	for _, budget := range budgets {
		// Check if current time is within budget window
		if now.Before(budget.WindowStart) || now.After(budget.WindowEnd) {
			continue
		}

		periodStatus := &BudgetPeriodStatus{
			Period:              budget.Period,
			BudgetMicroCents:    budget.BudgetMicroCents,
			UsedMicroCents:      budget.UsedMicroCents,
			UsagePercent:        budget.UsagePercent,
			Status:              budget.Status,
			RemainingMicroCents: budget.RemainingMicroCents,
		}

		status.Budgets[budget.Period] = periodStatus

		// Check if any budget restricts operations
		switch budget.Status {
		case "exceeded", "suspended":
			status.AllowConnection = false
			status.AllowMessages = false
			status.ExceededBudgets = append(status.ExceededBudgets, budget.Period)
		case "warning":
			status.WarningBudgets = append(status.WarningBudgets, budget.Period)
		}
	}

	return status, nil
}

// Aggregation

// CreateAggregation creates a new WebSocket cost aggregation
func (r *WebSocketCostRepository) CreateAggregation(ctx context.Context, aggregation *models.WebSocketCostAggregation) error {
	// Call BeforeCreate to set up the model
	if err := aggregation.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the aggregation record
	err := r.db.WithContext(ctx).Model(aggregation).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create WebSocket cost aggregation")
	}

	r.logger.Debug("created WebSocket cost aggregation",
		zap.String("operation_type", aggregation.OperationType),
		zap.String("period", aggregation.Period),
		zap.Time("window_start", aggregation.WindowStart),
		zap.Float64("total_cost_dollars", aggregation.TotalCostDollars),
		zap.Int64("total_connections", aggregation.TotalConnections))

	return nil
}

// UpdateAggregation updates an existing WebSocket cost aggregation
func (r *WebSocketCostRepository) UpdateAggregation(ctx context.Context, aggregation *models.WebSocketCostAggregation) error {
	// Call BeforeUpdate to set up the model
	if err := aggregation.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the aggregation record
	err := r.db.WithContext(ctx).Model(aggregation).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update WebSocket cost aggregation")
	}

	return nil
}

// GetAggregation retrieves WebSocket cost aggregation
func (r *WebSocketCostRepository) GetAggregation(ctx context.Context, period, operationType string, windowStart time.Time) (*models.WebSocketCostAggregation, error) {
	aggregation := &models.WebSocketCostAggregation{}

	pk := fmt.Sprintf("WS_AGG#%s#%s", period, operationType)
	sk := fmt.Sprintf("window#%s", windowStart.Format(time.RFC3339))

	err := r.db.WithContext(ctx).Model(aggregation).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(aggregation)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get WebSocket cost aggregation")
	}

	return aggregation, nil
}

// GetUserAggregation retrieves WebSocket cost aggregation for a specific user
func (r *WebSocketCostRepository) GetUserAggregation(ctx context.Context, userID, period, operationType string, windowStart time.Time) (*models.WebSocketCostAggregation, error) {
	var aggregations []*models.WebSocketCostAggregation

	query := r.db.WithContext(ctx).Model(&models.WebSocketCostAggregation{}).
		Index("user-agg-index").
		Where("GSI1PK", "=", fmt.Sprintf("WS_USER_AGG#%s#%s", userID, period)).
		Where("GSI1SK", "=", fmt.Sprintf("%s#%s", windowStart.Format(time.RFC3339), operationType)).
		Limit(1)

	err := query.All(&aggregations)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get user WebSocket cost aggregation")
	}

	if len(aggregations) == 0 {
		return nil, fmt.Errorf("aggregation not found")
	}

	return aggregations[0], nil
}

// ListAggregationsByPeriod lists WebSocket cost aggregations for a period
func (r *WebSocketCostRepository) ListAggregationsByPeriod(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostAggregation, error) {
	var aggregations []*models.WebSocketCostAggregation

	pk := fmt.Sprintf("WS_AGG#%s#%s", period, operationType)
	startSK := fmt.Sprintf("window#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("window#%s", endTime.Format(time.RFC3339))

	query := r.db.WithContext(ctx).Model(&models.WebSocketCostAggregation{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&aggregations)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list WebSocket cost aggregations")
	}

	return aggregations, nil
}

// AggregateWebSocketCosts performs aggregation of raw WebSocket cost data
func (r *WebSocketCostRepository) AggregateWebSocketCosts(ctx context.Context, operationType, period string, windowStart, windowEnd time.Time) error {
	// Get all cost records in the window
	costs, err := r.ListByOperationType(ctx, operationType, windowStart, windowEnd, 10000)
	if err != nil {
		return fmt.Errorf("failed to list costs for aggregation: %w", err)
	}

	if len(costs) == 0 {
		return nil // Nothing to aggregate
	}

	// Initialize aggregation
	aggregation := r.initializeAggregation(period, operationType, windowStart, windowEnd)
	
	// Create collectors for tracking metrics
	collectors := r.createMetricCollectors(len(costs))
	
	// Process each cost record
	for _, cost := range costs {
		r.processCostRecord(cost, aggregation, collectors)
	}

	// Finalize aggregation calculations
	r.finalizeAggregation(aggregation, collectors, windowStart, windowEnd)

	// Save or update aggregation
	return r.saveOrUpdateAggregation(ctx, aggregation, period, operationType, windowStart)
}

// initializeAggregation creates a new aggregation with default values
func (r *WebSocketCostRepository) initializeAggregation(period, operationType string, windowStart, windowEnd time.Time) *models.WebSocketCostAggregation {
	return &models.WebSocketCostAggregation{
		Period:                        period,
		OperationType:                 operationType,
		WindowStart:                   windowStart,
		WindowEnd:                     windowEnd,
		CostPercentiles:               make(map[string]float64),
		LatencyPercentiles:            make(map[string]float64),
		ConnectionDurationPercentiles: make(map[string]float64),
		StreamPopularity:              make(map[string]int64),
		StreamTypeBreakdown:           make(map[string]int64),
		CostByTier:                    make(map[string]*models.WebSocketTierCostStats),
	}
}

// webSocketMetricCollectors holds collectors for various metrics
type webSocketMetricCollectors struct {
	uniqueUsers        map[string]bool
	uniqueConnections  map[string]bool
	uniqueStreams      map[string]bool
	costValues         []float64
	latencyValues      []float64
	durationValues     []float64
	totalProcessingTime float64
	totalResponseLatency float64
	totalMemoryUsage    float64
	measurementCount    int64
}

// createMetricCollectors initializes metric collectors
func (r *WebSocketCostRepository) createMetricCollectors(capacity int) *webSocketMetricCollectors {
	return &webSocketMetricCollectors{
		uniqueUsers:       make(map[string]bool),
		uniqueConnections: make(map[string]bool),
		uniqueStreams:     make(map[string]bool),
		costValues:        make([]float64, 0, capacity),
		latencyValues:     []float64{},
		durationValues:    []float64{},
	}
}

// processCostRecord processes a single cost record and updates aggregation
func (r *WebSocketCostRepository) processCostRecord(cost *models.WebSocketCostRecord, aggregation *models.WebSocketCostAggregation, collectors *webSocketMetricCollectors) {
	// Track unique entities
	r.trackUniqueEntities(cost, collectors, aggregation)
	
	// Process operation-specific metrics
	r.processOperationMetrics(cost, aggregation, collectors)
	
	// Aggregate cost components
	r.aggregateCostComponents(cost, aggregation)
	
	// Collect performance metrics
	r.collectPerformanceMetrics(cost, collectors)
}

// trackUniqueEntities tracks unique users, connections, and streams
func (r *WebSocketCostRepository) trackUniqueEntities(cost *models.WebSocketCostRecord, collectors *webSocketMetricCollectors, aggregation *models.WebSocketCostAggregation) {
	if cost.UserID != "" {
		collectors.uniqueUsers[cost.UserID] = true
	}
	collectors.uniqueConnections[cost.ConnectionID] = true
	
	for _, stream := range cost.ActiveStreams {
		collectors.uniqueStreams[stream] = true
		aggregation.StreamPopularity[stream]++
	}
	
	for _, streamType := range cost.StreamTypes {
		aggregation.StreamTypeBreakdown[streamType]++
	}
}

// processOperationMetrics processes metrics based on operation type
func (r *WebSocketCostRepository) processOperationMetrics(cost *models.WebSocketCostRecord, aggregation *models.WebSocketCostAggregation, collectors *webSocketMetricCollectors) {
	switch cost.OperationType {
	case WSEventConnect:
		r.processConnectOperation(cost, aggregation, collectors)
	case WSEventDisconnect:
		aggregation.DroppedConnections++
	case WSEventMessageIn:
		aggregation.TotalMessagesIn += int64(cost.MessageCount)
		aggregation.TotalMessageBytes += cost.MessageSizeBytes
	case WSEventMessageOut:
		aggregation.TotalMessagesOut += int64(cost.MessageCount)
		aggregation.TotalMessageBytes += cost.MessageSizeBytes
	case WSEventSubscribe:
		aggregation.TotalStreamSubscriptions++
	case "error":
		aggregation.MessageDeliveryFailures++
	}
}

// processConnectOperation handles connect operation metrics
func (r *WebSocketCostRepository) processConnectOperation(cost *models.WebSocketCostRecord, aggregation *models.WebSocketCostAggregation, collectors *webSocketMetricCollectors) {
	aggregation.TotalConnections++
	if cost.ConnectionDurationMs > 0 {
		durationMinutes := float64(cost.ConnectionDurationMs) / (60 * 1000)
		aggregation.TotalConnectionMinutes += int64(durationMinutes)
		aggregation.AverageConnectionDuration += durationMinutes
		collectors.durationValues = append(collectors.durationValues, durationMinutes)
	}
}

// aggregateCostComponents aggregates all cost components
func (r *WebSocketCostRepository) aggregateCostComponents(cost *models.WebSocketCostRecord, aggregation *models.WebSocketCostAggregation) {
	aggregation.TotalAPIGatewayConnectionCost += cost.APIGatewayConnectionCost
	aggregation.TotalAPIGatewayMessageCost += cost.APIGatewayMessageCost
	aggregation.TotalLambdaExecutionCost += cost.LambdaExecutionCost
	aggregation.TotalDynamoDBCost += cost.DynamoDBCost
	aggregation.TotalDataTransferCost += cost.DataTransferCost
	aggregation.TotalCostMicroCents += cost.TotalCostMicroCents
}

// collectPerformanceMetrics collects performance-related metrics
func (r *WebSocketCostRepository) collectPerformanceMetrics(cost *models.WebSocketCostRecord, collectors *webSocketMetricCollectors) {
	collectors.costValues = append(collectors.costValues, cost.EstimatedCostDollars)
	
	if cost.ProcessingTimeMs > 0 {
		collectors.totalProcessingTime += float64(cost.ProcessingTimeMs)
		collectors.measurementCount++
	}
	if cost.ResponseLatencyMs > 0 {
		collectors.totalResponseLatency += float64(cost.ResponseLatencyMs)
		collectors.latencyValues = append(collectors.latencyValues, float64(cost.ResponseLatencyMs))
	}
	if cost.MemoryUsedMB > 0 {
		collectors.totalMemoryUsage += cost.MemoryUsedMB
	}
}

// finalizeAggregation calculates final aggregation values
func (r *WebSocketCostRepository) finalizeAggregation(aggregation *models.WebSocketCostAggregation, collectors *webSocketMetricCollectors, windowStart, windowEnd time.Time) {
	// Set unique counts
	aggregation.UniqueUsers = int64(len(collectors.uniqueUsers))
	aggregation.UniqueStreamsUsed = int64(len(collectors.uniqueStreams))
	
	// Calculate averages
	r.calculateAverages(aggregation, collectors)
	
	// Calculate message metrics
	r.calculateMessageMetrics(aggregation, windowStart, windowEnd)
	
	// Calculate percentiles
	r.calculatePercentiles(aggregation, collectors)
}

// calculateAverages calculates average metrics
func (r *WebSocketCostRepository) calculateAverages(aggregation *models.WebSocketCostAggregation, collectors *webSocketMetricCollectors) {
	if collectors.measurementCount > 0 {
		aggregation.AverageProcessingTime = collectors.totalProcessingTime / float64(collectors.measurementCount)
		aggregation.AverageMemoryUsage = collectors.totalMemoryUsage / float64(collectors.measurementCount)
	}
	
	if len(collectors.latencyValues) > 0 {
		var totalLatency float64
		for _, latency := range collectors.latencyValues {
			totalLatency += latency
		}
		aggregation.AverageResponseLatency = totalLatency / float64(len(collectors.latencyValues))
	}
	
	if aggregation.TotalConnections > 0 {
		aggregation.AverageConnectionDuration = aggregation.AverageConnectionDuration / float64(aggregation.TotalConnections)
	}
}

// calculateMessageMetrics calculates message-related metrics
func (r *WebSocketCostRepository) calculateMessageMetrics(aggregation *models.WebSocketCostAggregation, windowStart, windowEnd time.Time) {
	totalMessages := aggregation.TotalMessagesIn + aggregation.TotalMessagesOut
	if totalMessages > 0 {
		aggregation.AverageMessageSize = float64(aggregation.TotalMessageBytes) / float64(totalMessages)
		
		windowSeconds := windowEnd.Sub(windowStart).Seconds()
		if windowSeconds > 0 {
			aggregation.MessageThroughputPerSec = float64(totalMessages) / windowSeconds
		}
	}
}

// calculatePercentiles calculates percentile metrics
func (r *WebSocketCostRepository) calculatePercentiles(aggregation *models.WebSocketCostAggregation, collectors *webSocketMetricCollectors) {
	if len(collectors.costValues) > 0 {
		aggregation.CostPercentiles = calculateWebSocketPercentiles(collectors.costValues)
	}
	if len(collectors.latencyValues) > 0 {
		aggregation.LatencyPercentiles = calculateWebSocketPercentiles(collectors.latencyValues)
	}
	if len(collectors.durationValues) > 0 {
		aggregation.ConnectionDurationPercentiles = calculateWebSocketPercentiles(collectors.durationValues)
	}
}

// saveOrUpdateAggregation saves or updates the aggregation
func (r *WebSocketCostRepository) saveOrUpdateAggregation(ctx context.Context, aggregation *models.WebSocketCostAggregation, period, operationType string, windowStart time.Time) error {
	existing, err := r.GetAggregation(ctx, period, operationType, windowStart)
	if err == nil && existing != nil {
		aggregation.CreatedAt = existing.CreatedAt
		return r.UpdateAggregation(ctx, aggregation)
	}
	return r.CreateAggregation(ctx, aggregation)
}

// GetHighCostOperations returns WebSocket operations that exceed a cost threshold
func (r *WebSocketCostRepository) GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	// Get all recent costs
	allCosts, err := r.GetRecentCosts(ctx, startTime, limit*10) // Get more to filter
	if err != nil {
		return nil, err
	}

	// Filter by threshold and time range
	var highCostOps []*models.WebSocketCostRecord
	for _, cost := range allCosts {
		if cost.Timestamp.After(endTime) {
			continue
		}
		if cost.EstimatedCostDollars >= thresholdDollars {
			highCostOps = append(highCostOps, cost)
			if len(highCostOps) >= limit {
				break
			}
		}
	}

	return highCostOps, nil
}

// GetTopCostlyUsers returns users with highest WebSocket costs
func (r *WebSocketCostRepository) GetTopCostlyUsers(ctx context.Context, startDate, endDate time.Time, limit int) ([]*WebSocketUserCostRanking, error) {
	// Get all costs in the time range
	costs, err := r.GetRecentCosts(ctx, startDate, 10000)
	if err != nil {
		return nil, err
	}

	userCostMap := make(map[string]*WebSocketUserCostRanking)

	for _, cost := range costs {
		if cost.Timestamp.After(endDate) {
			continue
		}

		if cost.UserID == "" {
			continue
		}

		ranking, exists := userCostMap[cost.UserID]
		if !exists {
			ranking = &WebSocketUserCostRanking{
				UserID:   cost.UserID,
				Username: cost.Username,
			}
			userCostMap[cost.UserID] = ranking
		}

		ranking.TotalCostMicroCents += cost.TotalCostMicroCents
		ranking.TotalOperations++
		ranking.TotalConnectionMinutes += cost.ConnectionDurationMs / (60 * 1000) // Convert to minutes
		ranking.TotalMessages += int64(cost.MessageCount)

		// Track by operation type
		switch cost.OperationType {
		case WSEventConnect, WSEventDisconnect:
			ranking.ConnectionOperations++
		case WSEventMessageIn, WSEventMessageOut:
			ranking.MessageOperations++
		case "subscribe", "unsubscribe":
			ranking.SubscriptionOperations++
		}
	}

	// Convert map to slice and calculate totals
	rankings := make([]*WebSocketUserCostRanking, 0, len(userCostMap))
	for _, ranking := range userCostMap {
		ranking.TotalCostDollars = float64(ranking.TotalCostMicroCents) / 1_000_000.0

		if ranking.TotalOperations > 0 {
			ranking.AverageCostPerOperation = ranking.TotalCostDollars / float64(ranking.TotalOperations)
		}

		rankings = append(rankings, ranking)
	}

	// Sort by total cost (highest first)
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].TotalCostMicroCents > rankings[j].TotalCostMicroCents
	})

	// Limit results
	if len(rankings) > limit {
		rankings = rankings[:limit]
	}

	return rankings, nil
}

// Helper function to calculate percentiles (same as in cost_tracking_repository.go)
func calculateWebSocketPercentiles(values []float64) map[string]float64 {
	if len(values) == 0 {
		return map[string]float64{
			"p50": 0,
			"p90": 0,
			"p95": 0,
			"p99": 0,
		}
	}

	// Sort values
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// Calculate percentiles
	percentiles := map[string]float64{
		"p50": getWebSocketPercentileValue(sorted, 50),
		"p90": getWebSocketPercentileValue(sorted, 90),
		"p95": getWebSocketPercentileValue(sorted, 95),
		"p99": getWebSocketPercentileValue(sorted, 99),
	}

	return percentiles
}

// getWebSocketPercentileValue calculates the value at a specific percentile
func getWebSocketPercentileValue(sorted []float64, percentile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if len(sorted) == 1 {
		return sorted[0]
	}

	// Calculate the index
	index := (percentile / 100.0) * float64(len(sorted)-1)
	lowerIndex := int(math.Floor(index))
	upperIndex := int(math.Ceil(index))

	if lowerIndex == upperIndex {
		return sorted[lowerIndex]
	}

	// Linear interpolation between two values
	lowerValue := sorted[lowerIndex]
	upperValue := sorted[upperIndex]
	fraction := index - float64(lowerIndex)

	return lowerValue + (upperValue-lowerValue)*fraction
}

// Data structures for summary results

// WebSocketConnectionCostSummary represents cost summary for a specific connection
type WebSocketConnectionCostSummary struct {
	ConnectionID            string
	UserID                  string
	Username                string
	StartTime               time.Time
	EndTime                 time.Time
	Count                   int
	TotalOperations         int64
	TotalConnectionMinutes  int64
	TotalMessages           int64
	TotalMessageBytes       int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostPerOperation float64
	AverageMessageSize      float64
	OperationBreakdown      map[string]*WebSocketOperationCostStats
}

// WebSocketUserCostSummary represents cost summary for a specific user
type WebSocketUserCostSummary struct {
	UserID                    string
	Username                  string
	StartTime                 time.Time
	EndTime                   time.Time
	Count                     int
	TotalOperations           int64
	TotalConnectionMinutes    int64
	TotalMessages             int64
	TotalMessageBytes         int64
	TotalIdleTime             int64
	TotalCostMicroCents       int64
	TotalCostDollars          float64
	UniqueConnections         int64
	UniqueStreams             int64
	AverageCostPerOperation   float64
	AverageCostPerConnection  float64
	AverageConnectionDuration float64
	AverageMessageSize        float64
	AverageIdleTime           float64
	OperationBreakdown        map[string]*WebSocketOperationCostStats
	StreamBreakdown           map[string]*WebSocketStreamCostStats
}

// WebSocketOperationCostStats represents cost statistics for a specific operation type
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

// WebSocketStreamCostStats represents cost statistics for a specific stream
type WebSocketStreamCostStats struct {
	StreamName            string
	OperationCount        int64
	MessageCount          int64
	TotalCostMicroCents   int64
	TotalCostDollars      float64
	AverageCostMicroCents int64
}

// WebSocketUserCostRanking represents a user's cost ranking
type WebSocketUserCostRanking struct {
	UserID                  string
	Username                string
	TotalOperations         int64
	ConnectionOperations    int64
	MessageOperations       int64
	SubscriptionOperations  int64
	TotalConnectionMinutes  int64
	TotalMessages           int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostPerOperation float64
}

// BudgetStatus represents current budget status for a user
type BudgetStatus struct {
	UserID          string
	AllowConnection bool
	AllowMessages   bool
	ExceededBudgets []string
	WarningBudgets  []string
	Budgets         map[string]*BudgetPeriodStatus
}

// BudgetPeriodStatus represents budget status for a specific period
type BudgetPeriodStatus struct {
	Period              string
	BudgetMicroCents    int64
	UsedMicroCents      int64
	RemainingMicroCents int64
	UsagePercent        float64
	Status              string
}
