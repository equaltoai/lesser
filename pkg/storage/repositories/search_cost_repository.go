package repositories

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// SearchCostRepository manages search cost tracking and budgets
type SearchCostRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewSearchCostRepository creates a new search cost tracking repository
func NewSearchCostRepository(db core.DB, logger *zap.Logger) *SearchCostRepository {
	return &SearchCostRepository{
		db:     db,
		logger: logger,
	}
}

// RecordSearchCost records cost tracking data for a search operation
func (r *SearchCostRepository) RecordSearchCost(ctx context.Context, costData *models.SearchCostTracking) error {
	if costData == nil {
		return fmt.Errorf("cost data cannot be nil")
	}

	// Update keys and set defaults
	costData.UpdateKeys()

	// Calculate total cost
	costData.TotalCostMicros = r.calculateTotalCostMicros(costData)

	// Calculate cost per result if results exist
	if costData.ResultCount > 0 {
		costData.CostPerResult = costData.TotalCostMicros / int64(costData.ResultCount)
	}

	// Store the cost tracking record
	err := r.db.WithContext(ctx).Model(costData).Create()
	if err != nil {
		r.logger.Error("failed to record search cost",
			zap.String("user_id", costData.UserID),
			zap.String("operation_type", costData.OperationType),
			zap.String("search_type", costData.SearchType),
			zap.Error(err))
		return fmt.Errorf("failed to record search cost: %w", err)
	}

	// Update user budget usage
	if err := r.updateBudgetUsage(ctx, costData); err != nil {
		r.logger.Warn("failed to update budget usage",
			zap.String("user_id", costData.UserID),
			zap.Error(err))
		// Don't fail the main operation for budget tracking issues
	}

	// Update query statistics
	if err := r.updateQueryStats(ctx, costData); err != nil {
		r.logger.Warn("failed to update query statistics",
			zap.String("query", costData.Query),
			zap.Error(err))
		// Don't fail the main operation for stats tracking issues
	}

	r.logger.Debug("search cost recorded successfully",
		zap.String("user_id", costData.UserID),
		zap.String("operation_type", costData.OperationType),
		zap.Int64("total_cost_micros", costData.TotalCostMicros),
		zap.Int("result_count", costData.ResultCount))

	return nil
}

// CheckBudget checks if a user can perform a search operation within budget
func (r *SearchCostRepository) CheckBudget(ctx context.Context, userID, operationType string, estimatedCostMicros int64) error {
	budget, err := r.getUserBudget(ctx, userID, "daily")
	if err != nil {
		if errors.IsNotFound(err) {
			// Create default budget if none exists
			if err := r.createDefaultBudget(ctx, userID); err != nil {
				return fmt.Errorf("failed to create default budget: %w", err)
			}
			// Retry getting budget
			budget, err = r.getUserBudget(ctx, userID, "daily")
			if err != nil {
				return fmt.Errorf("failed to get budget after creation: %w", err)
			}
		} else {
			return fmt.Errorf("failed to check budget: %w", err)
		}
	}

	// Check if user can make the request
	if !budget.CanMakeRequest(operationType, estimatedCostMicros) {
		return fmt.Errorf("budget exceeded: operation %s would cost %d microcents but budget allows %d remaining",
			operationType, estimatedCostMicros, budget.BudgetLimitMicros-budget.UsedBudgetMicros)
	}

	return nil
}

// RecordBudgetUsage records budget usage for a completed operation
func (r *SearchCostRepository) RecordBudgetUsage(ctx context.Context, userID, operationType string, actualCostMicros int64) error {
	budget, err := r.getUserBudget(ctx, userID, "daily")
	if err != nil {
		return fmt.Errorf("failed to get budget for usage recording: %w", err)
	}

	// Record the usage
	budget.RecordUsage(operationType, actualCostMicros)

	// Update the budget record
	err = r.db.WithContext(ctx).Model(budget).Update()
	if err != nil {
		r.logger.Error("failed to update budget usage",
			zap.String("user_id", userID),
			zap.String("operation_type", operationType),
			zap.Int64("cost_micros", actualCostMicros),
			zap.Error(err))
		return fmt.Errorf("failed to update budget usage: %w", err)
	}

	return nil
}

// GetUserBudget retrieves the current budget for a user
func (r *SearchCostRepository) GetUserBudget(ctx context.Context, userID, period string) (*models.SearchBudget, error) {
	return r.getUserBudget(ctx, userID, period)
}

// GetSearchCosts retrieves search costs for a user in a date range
func (r *SearchCostRepository) GetSearchCosts(ctx context.Context, userID string, startDate, endDate time.Time) ([]*models.SearchCostTracking, error) {
	var costs []*models.SearchCostTracking

	// Query each day in the range
	current := startDate
	for !current.After(endDate) {
		dateStr := current.Format(common.DateFormat)

		var dayCosts []models.SearchCostTracking
		err := r.db.WithContext(ctx).Model(&models.SearchCostTracking{}).
			Where("PK", "=", fmt.Sprintf("SEARCH_COST#%s#%s", dateStr, userID)).
			All(&dayCosts)

		if err != nil && !errors.IsNotFound(err) {
			r.logger.Warn("failed to get search costs for date",
				zap.String("user_id", userID),
				zap.String("date", dateStr),
				zap.Error(err))
		} else {
			for i := range dayCosts {
				costs = append(costs, &dayCosts[i])
			}
		}

		current = current.AddDate(0, 0, 1)
	}

	// Sort by timestamp
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].Timestamp.Before(costs[j].Timestamp)
	})

	return costs, nil
}

// GetSearchCostSummary retrieves aggregated search cost data
func (r *SearchCostRepository) GetSearchCostSummary(ctx context.Context, userID string, period string) (*SearchCostSummary, error) {
	var startDate time.Time
	now := time.Now()

	switch period {
	case "daily":
		startDate = now.AddDate(0, 0, -1)
	case "weekly":
		startDate = now.AddDate(0, 0, -7)
	case "monthly":
		startDate = now.AddDate(0, -1, 0)
	default:
		startDate = now.AddDate(0, 0, -7) // Default to weekly
	}

	costs, err := r.GetSearchCosts(ctx, userID, startDate, now)
	if err != nil {
		return nil, fmt.Errorf("failed to get search costs: %w", err)
	}

	return r.calculateSummary(costs), nil
}

// GetPopularQueries retrieves the most popular search queries
func (r *SearchCostRepository) GetPopularQueries(ctx context.Context, limit int, period string) ([]*models.SearchQueryStats, error) {
	var stats []models.SearchQueryStats

	// Get current period date
	// var periodDate string
	// now := time.Now()
	// switch period {
	// case "daily":
	// 	periodDate = now.Format(common.DateFormat)
	// case "weekly":
	// 	year, week := now.ISOWeek()
	// 	periodDate = fmt.Sprintf("%d-W%d", year, week)
	// case "monthly":
	// 	periodDate = now.Format(common.MonthFormat)
	// default:
	// 	periodDate = now.Format(common.DateFormat)
	// }
	// Note: periodDate could be used for more specific queries in the future

	// Query all query stats for the period
	err := r.db.WithContext(ctx).Model(&models.SearchQueryStats{}).
		Filter("SK", "=", fmt.Sprintf("STATS#%s", period)).
		Limit(limit * 2). // Get more to filter and sort
		All(&stats)

	if err != nil {
		return nil, fmt.Errorf("failed to get popular queries: %w", err)
	}

	// Sort by query count
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].QueryCount > stats[j].QueryCount
	})

	// Apply limit
	if len(stats) > limit {
		stats = stats[:limit]
	}

	// Convert to pointers
	result := make([]*models.SearchQueryStats, len(stats))
	for i := range stats {
		result[i] = &stats[i]
	}

	return result, nil
}

// ResetBudgets resets daily budgets (called by scheduled job)
func (r *SearchCostRepository) ResetBudgets(ctx context.Context, period string) error {
	// This would typically be called by a scheduled Lambda
	// For now, implement basic logic to reset expired budgets

	var budgets []models.SearchBudget
	periodDate := time.Now().Format(common.DateFormat)

	// Scan for budgets that need resetting
	err := r.db.WithContext(ctx).Model(&models.SearchBudget{}).
		Filter("SK", "=", fmt.Sprintf("PERIOD#%s", periodDate)).
		All(&budgets)

	if err != nil {
		return fmt.Errorf("failed to query budgets for reset: %w", err)
	}

	for _, budget := range budgets {
		// Reset usage counters
		budget.UsedBudgetMicros = 0
		budget.SearchUsedMicros = 0
		budget.SemanticUsedMicros = 0
		budget.IndexingUsedMicros = 0
		budget.CurrentRequests = 0
		budget.CurrentSemanticRequests = 0
		budget.BudgetExceeded = false
		budget.LastResetTime = time.Now()
		budget.UpdatedAt = time.Now()

		err = r.db.WithContext(ctx).Model(&budget).Update()
		if err != nil {
			r.logger.Error("failed to reset budget",
				zap.String("user_id", budget.UserID),
				zap.Error(err))
		}
	}

	r.logger.Info("reset budgets completed",
		zap.String("period", period),
		zap.Int("budgets_reset", len(budgets)))

	return nil
}

// Helper methods

func (r *SearchCostRepository) getUserBudget(ctx context.Context, userID, period string) (*models.SearchBudget, error) {
	var budget models.SearchBudget

	periodDate := r.getPeriodDate(period)

	err := r.db.WithContext(ctx).Model(&models.SearchBudget{}).
		Where("PK", "=", fmt.Sprintf("SEARCH_BUDGET#%s", userID)).
		Where("SK", "=", fmt.Sprintf("PERIOD#%s", periodDate)).
		First(&budget)

	if err != nil {
		return nil, err
	}

	return &budget, nil
}

func (r *SearchCostRepository) createDefaultBudget(ctx context.Context, userID string) error {
	budget := &models.SearchBudget{
		UserID:     userID,
		Period:     "daily",
		PeriodDate: time.Now().Format(common.DateFormat),

		// Default budget limits (in microcents)
		BudgetLimitMicros:    1000000, // $0.01 per day
		SearchBudgetMicros:   800000,  // $0.008 for regular search
		SemanticBudgetMicros: 150000,  // $0.0015 for semantic search
		IndexingBudgetMicros: 50000,   // $0.0005 for indexing

		// Request limits
		MaxRequestsPerHour: 100,
		MaxSemanticPerHour: 10,

		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	budget.UpdateKeys()

	err := r.db.WithContext(ctx).Model(budget).Create()
	if err != nil {
		return fmt.Errorf("failed to create default budget: %w", err)
	}

	return nil
}

func (r *SearchCostRepository) updateBudgetUsage(ctx context.Context, costData *models.SearchCostTracking) error {
	return r.RecordBudgetUsage(ctx, costData.UserID, costData.OperationType, costData.TotalCostMicros)
}

func (r *SearchCostRepository) updateQueryStats(ctx context.Context, costData *models.SearchCostTracking) error {
	if costData.Query == "" {
		return nil // Skip empty queries
	}

	// Hash the query for privacy
	queryHash := r.hashQuery(costData.Query)

	// Get or create query stats
	var stats models.SearchQueryStats
	// periodDate := time.Now().Format(common.DateFormat) // Reserved for future use

	err := r.db.WithContext(ctx).Model(&models.SearchQueryStats{}).
		Where("PK", "=", fmt.Sprintf("SEARCH_STATS#%s", queryHash)).
		Where("SK", "=", "STATS#daily").
		First(&stats)

	if errors.IsNotFound(err) {
		// Create new stats record
		stats = models.SearchQueryStats{
			QueryHash:    queryHash,
			QueryType:    costData.SearchType,
			QueryLength:  costData.QueryLength,
			Period:       "daily",
			FirstQueried: costData.Timestamp,
		}
	} else if err != nil {
		return fmt.Errorf("failed to get query stats: %w", err)
	}

	// Update statistics
	stats.QueryCount++
	stats.TotalResultCount += int64(costData.ResultCount)
	stats.AverageResults = float64(stats.TotalResultCount) / float64(stats.QueryCount)
	stats.TotalResponseTimeMs += costData.ResponseTimeMs
	stats.AverageResponseTimeMs = float64(stats.TotalResponseTimeMs) / float64(stats.QueryCount)
	stats.TotalCostMicros += costData.TotalCostMicros
	stats.AverageCostMicros = float64(stats.TotalCostMicros) / float64(stats.QueryCount)
	stats.LastQueried = costData.Timestamp
	stats.UpdatedAt = time.Now()

	if costData.CacheHit {
		stats.CacheHitCount++
		stats.CacheHitRate = float64(stats.CacheHitCount) / float64(stats.QueryCount)
	}

	// Calculate cost efficiency (cost per result)
	if stats.TotalResultCount > 0 {
		stats.CostEfficiency = float64(stats.TotalCostMicros) / float64(stats.TotalResultCount)
	}

	// Update min/max response times
	if stats.MinResponseTimeMs == 0 || costData.ResponseTimeMs < stats.MinResponseTimeMs {
		stats.MinResponseTimeMs = costData.ResponseTimeMs
	}
	if costData.ResponseTimeMs > stats.MaxResponseTimeMs {
		stats.MaxResponseTimeMs = costData.ResponseTimeMs
	}

	stats.UpdateKeys()

	// Create or update the record
	if err == nil {
		err = r.db.WithContext(ctx).Model(&stats).Update()
	} else {
		err = r.db.WithContext(ctx).Model(&stats).Create()
	}

	if err != nil {
		return fmt.Errorf("failed to update query stats: %w", err)
	}

	return nil
}

func (r *SearchCostRepository) calculateTotalCostMicros(costData *models.SearchCostTracking) int64 {
	var total int64

	// DynamoDB costs (using pricing constants from cost package)
	const (
		DynamoDBReadRequestUnit  = 25  // $0.25 per million read request units
		DynamoDBWriteRequestUnit = 125 // $1.25 per million write request units
	)

	// DynamoDB costs
	total += (costData.DynamoReads * DynamoDBReadRequestUnit) / 1000000
	total += (costData.DynamoWrites * DynamoDBWriteRequestUnit) / 1000000

	// Bedrock costs for semantic search
	if costData.BedrockRequests > 0 {
		// Titan embeddings: ~$0.0001 per 1K tokens
		bedrockCostPerToken := int64(100) // 100 microcents per 1K tokens
		total += (int64(costData.EmbeddingTokens) * bedrockCostPerToken) / 1000
	}

	// Lambda execution costs
	if costData.LambdaDurationMs > 0 && costData.LambdaMemoryMB > 0 {
		// $0.0000166667 per GB-second
		gbSeconds := float64(costData.LambdaDurationMs) * float64(costData.LambdaMemoryMB) / (1000 * 1024)
		lambdaCost := int64(gbSeconds * 1667) // 1667 microcents per GB-second
		total += lambdaCost
	}

	return total
}

func (r *SearchCostRepository) hashQuery(query string) string {
	hash := sha256.Sum256([]byte(query))
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes for compact hash
}

func (r *SearchCostRepository) getPeriodDate(period string) string {
	now := time.Now()
	switch period {
	case "daily":
		return now.Format(common.DateFormat)
	case "weekly":
		year, week := now.ISOWeek()
		return fmt.Sprintf("%d-W%d", year, week)
	case "monthly":
		return now.Format(common.MonthFormat)
	case "yearly":
		return now.Format("2006")
	default:
		return now.Format(common.DateFormat)
	}
}

func (r *SearchCostRepository) calculateSummary(costs []*models.SearchCostTracking) *SearchCostSummary {
	if len(costs) == 0 {
		return &SearchCostSummary{}
	}

	summary := &SearchCostSummary{
		TotalRequests: int64(len(costs)),
	}

	var totalCost, totalResponseTime int64
	var cacheHits int
	operationCounts := make(map[string]int64)

	for _, cost := range costs {
		totalCost += cost.TotalCostMicros
		totalResponseTime += cost.ResponseTimeMs

		if cost.CacheHit {
			cacheHits++
		}

		operationCounts[cost.OperationType]++
		summary.TotalResults += int64(cost.ResultCount)
	}

	summary.TotalCostMicros = totalCost
	summary.AverageCostMicros = totalCost / summary.TotalRequests
	summary.AverageResponseTimeMs = totalResponseTime / summary.TotalRequests
	summary.CacheHitRate = float64(cacheHits) / float64(len(costs))

	if summary.TotalResults > 0 {
		summary.AverageResultsPerRequest = float64(summary.TotalResults) / float64(summary.TotalRequests)
		summary.CostPerResult = totalCost / summary.TotalResults
	}

	summary.OperationBreakdown = operationCounts

	return summary
}

// SearchCostSummary provides aggregated search cost information
type SearchCostSummary struct {
	TotalRequests            int64            `json:"total_requests"`
	TotalCostMicros          int64            `json:"total_cost_micros"`
	AverageCostMicros        int64            `json:"average_cost_micros"`
	TotalResults             int64            `json:"total_results"`
	AverageResultsPerRequest float64          `json:"average_results_per_request"`
	CostPerResult            int64            `json:"cost_per_result"`
	AverageResponseTimeMs    int64            `json:"average_response_time_ms"`
	CacheHitRate             float64          `json:"cache_hit_rate"`
	OperationBreakdown       map[string]int64 `json:"operation_breakdown"`
}
