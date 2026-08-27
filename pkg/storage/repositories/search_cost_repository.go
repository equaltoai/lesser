package repositories

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// SearchCostRepository manages search cost tracking and budgets using enhanced patterns
type SearchCostRepository struct {
	*EnhancedBaseRepository[*models.SearchCostTracking]
	logger *zap.Logger
}

// NewSearchCostRepository creates a new search cost tracking repository with enhanced functionality
func NewSearchCostRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *SearchCostRepository {
	// Create enhanced repository optimized for search cost operations
	enhancedRepo := NewEnhancedBaseRepository[*models.SearchCostTracking](db, tableName, logger, costService, "SearchCostRepository", "searchcost")

	// Set up enhanced services for search cost operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cost data cached for analytics
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for cost monitoring

	return &SearchCostRepository{
		EnhancedBaseRepository: enhancedRepo,
		logger:                 logger,
	}
}

// RecordSearchCost records cost tracking data for a search operation
func (r *SearchCostRepository) RecordSearchCost(ctx context.Context, costData *models.SearchCostTracking) error {
	if costData == nil {
		return ErrorHandler.HandleCreateError(errors.New("cost data cannot be nil"), EntitySearchCost, "validation")
	}

	// Update keys and set defaults
	_ = costData.UpdateKeys() // Ignore error as this is internal model operation

	// Calculate total cost
	costData.TotalCostMicros = r.calculateTotalCostMicros(costData)

	// Calculate cost per result if results exist
	if costData.ResultCount > 0 {
		costData.CostPerResult = costData.TotalCostMicros / int64(costData.ResultCount)
	}

	// Store the cost tracking record using BaseRepository
	err := r.ValidateAndCreate(ctx, costData)
	if err != nil {
		r.logger.Error("failed to record search cost",
			zap.String("user_id", costData.UserID),
			zap.String("operation_type", costData.OperationType),
			zap.String("search_type", costData.SearchType),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntitySearchCost, costData.UserID)
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
		if dynamormErrors.IsNotFound(err) {
			// Create default budget if none exists
			if err := r.createDefaultBudget(ctx, userID); err != nil {
				return ErrorHandler.HandleCreateError(err, EntitySearchBudget, userID)
			}
			// Retry getting budget
			budget, err = r.getUserBudget(ctx, userID, "daily")
			if err != nil {
				return ErrorHandler.HandleGetError(err, EntitySearchBudget, userID)
			}
		} else {
			return ErrorHandler.HandleGetError(err, EntitySearchBudget, userID)
		}
	}

	// Check if user can make the request
	if !budget.CanMakeRequest(operationType, estimatedCostMicros) {
		return ErrorHandler.HandleQueryError(fmt.Errorf("budget exceeded: operation %s would cost %d microcents but budget allows %d remaining",
			operationType, estimatedCostMicros, budget.BudgetLimitMicros-budget.UsedBudgetMicros), EntitySearchBudget, "budget limit check")
	}

	return nil
}

// RecordBudgetUsage records budget usage for a completed operation
func (r *SearchCostRepository) RecordBudgetUsage(ctx context.Context, userID, operationType string, actualCostMicros int64) error {
	budget, err := r.getUserBudget(ctx, userID, "daily")
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntitySearchBudget, userID)
	}

	// Record the usage
	budget.RecordUsage(operationType, actualCostMicros)

	// Update the budget record using BaseRepository's GetDB for SearchBudget
	err = r.GetDB().WithContext(ctx).Model(budget).Update()
	if err != nil {
		r.logger.Error("failed to update budget usage",
			zap.String("user_id", userID),
			zap.String("operation_type", operationType),
			zap.Int64("cost_micros", actualCostMicros),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntitySearchBudget, userID)
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
		// Each day is a keyed SEARCH_COST#<date>#<user> partition; the whole
		// partition must be read, so the read is a bounded page walk (wave
		// #1469): Limit(500)/page, 100-page cap. Cap exhaustion must fail closed
		// — the pre-existing warn-and-skip swallow would otherwise silently drop
		// it; other (transient) errors keep the skip-this-day behavior.
		err := walkKeyedPages(
			r.GetDB().WithContext(ctx).Model(&models.SearchCostTracking{}).
				Where("PK", "=", fmt.Sprintf("SEARCH_COST#%s#%s", dateStr, userID)),
			500, 100,
			func(page []models.SearchCostTracking) (bool, error) {
				dayCosts = append(dayCosts, page...)
				return false, nil
			},
		)

		if err != nil {
			if errors.Is(err, errBoundedPageCapExceeded) {
				return nil, err
			}
			if !dynamormErrors.IsNotFound(err) {
				r.logger.Warn("failed to get search costs for date",
					zap.String("user_id", userID),
					zap.String("date", dateStr),
					zap.Error(err))
			}
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
		return nil, ErrorHandler.HandleQueryError(err, EntitySearchCost, "date range query")
	}

	return r.calculateSummary(costs), nil
}

// Helper methods

func (r *SearchCostRepository) getUserBudget(ctx context.Context, userID, period string) (*models.SearchBudget, error) {
	var budget models.SearchBudget

	periodDate := r.getPeriodDate(period)

	err := r.GetDB().WithContext(ctx).Model(&models.SearchBudget{}).
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

	_ = budget.UpdateKeys() // Ignore error as this is internal model operation

	err := r.GetDB().WithContext(ctx).Model(budget).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntitySearchBudget, userID)
	}

	return nil
}

func (r *SearchCostRepository) updateBudgetUsage(ctx context.Context, costData *models.SearchCostTracking) error {
	return r.RecordBudgetUsage(ctx, costData.UserID, costData.OperationType, costData.TotalCostMicros)
}

func (r *SearchCostRepository) updateQueryStats(ctx context.Context, costData *models.SearchCostTracking) error {
	if err := common.ValidateRequiredParam("costData.Query", costData.Query); err != nil {
		return nil // Skip empty queries
	}

	// Hash the query for privacy
	queryHash := r.hashQuery(costData.Query)

	// Get or create query stats
	var stats models.SearchQueryStats
	// periodDate := time.Now().Format(common.DateFormat) // Reserved for future use

	err := r.GetDB().WithContext(ctx).Model(&models.SearchQueryStats{}).
		Where("PK", "=", fmt.Sprintf("SEARCH_STATS#%s", queryHash)).
		Where("SK", "=", "STATS#daily").
		First(&stats)

	if dynamormErrors.IsNotFound(err) {
		// Create new stats record
		stats = models.SearchQueryStats{
			QueryHash:    queryHash,
			QueryType:    costData.SearchType,
			QueryLength:  costData.QueryLength,
			Period:       "daily",
			FirstQueried: costData.Timestamp,
		}
	} else if err != nil {
		return ErrorHandler.HandleGetError(err, EntitySearchMetric, "query stats")
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

	_ = stats.UpdateKeys() // Ignore error as this is internal model operation

	// Create or update the record
	if err == nil {
		err = r.GetDB().WithContext(ctx).Model(&stats).Update()
	} else {
		err = r.GetDB().WithContext(ctx).Model(&stats).Create()
	}

	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntitySearchMetric, "query stats")
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
	if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
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
