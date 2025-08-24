package repositories

import (
	"context"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// SearchServiceIntegration shows how to integrate all search cost tracking components
type SearchServiceIntegration struct {
	// Core repositories
	searchRepo *SearchRepository
	costRepo   *SearchCostRepository

	// Cost tracking wrappers
	trackedSearch *SearchCostTrackingWrapper
	trackedAI     *cost.AIServiceWithCostTracking

	// Cost tracker
	costTracker *cost.Tracker
	logger      *zap.Logger
}

// NewSearchServiceIntegration creates a fully integrated search service with cost tracking
func NewSearchServiceIntegration(db core.DB, aiService *ai.AIService, requestID string, logger *zap.Logger) *SearchServiceIntegration {
	// Create base repositories
	searchRepo := NewSearchRepository(db, "main_table", logger, nil)
	costRepo := NewSearchCostRepository(db, "main_table", logger, nil)

	// Create cost tracker for this request
	costTracker := cost.NewWithRequest(requestID, "search_operation")

	// Create cost tracking wrappers
	trackedSearch := NewSearchCostTrackingWrapper(searchRepo, costRepo, costTracker, logger)
	trackedAI := cost.NewAIServiceWithCostTracking(aiService, costTracker, logger)

	return &SearchServiceIntegration{
		searchRepo:    searchRepo,
		costRepo:      costRepo,
		trackedSearch: trackedSearch,
		trackedAI:     trackedAI,
		costTracker:   costTracker,
		logger:        logger,
	}
}

// ComprehensiveSearchExample demonstrates comprehensive search with cost tracking
func (s *SearchServiceIntegration) ComprehensiveSearchExample(ctx context.Context, userID, query string, useSemanticSearch bool) (*SearchResultsWithCosts, error) {
	results := &SearchResultsWithCosts{
		CostBreakdown: make(map[string]int64),
	}

	// 1. Check user budget first
	estimatedCost := s.estimateSearchCost(query, useSemanticSearch)
	if err := s.costRepo.CheckBudget(ctx, userID, "comprehensive_search", estimatedCost); err != nil {
		return nil, err
	}

	// 2. Perform text-based searches
	actors, err := s.trackedSearch.SearchAccounts(ctx, query, 20, false, 0)
	if err != nil {
		return nil, err
	}
	results.Accounts = actors

	statuses, err := s.trackedSearch.SearchStatuses(ctx, query, 20)
	if err != nil {
		return nil, err
	}
	results.Statuses = statuses

	hashtags, err := s.trackedSearch.SearchHashtags(ctx, query, 10)
	if err != nil {
		return nil, err
	}
	results.Hashtags = hashtags

	// 3. Add semantic search if requested
	if useSemanticSearch {
		embedding, semanticResults, costData, err := s.trackedAI.SemanticSearchWithCostTracking(ctx, query, userID, 10, 0.7)
		if err != nil {
			s.logger.Warn("semantic search failed", zap.Error(err))
		} else {
			results.SemanticResults = semanticResults
			results.QueryEmbedding = embedding
			results.CostBreakdown["semantic_search"] = costData.TotalCostMicros
		}
	}

	// 4. Get search suggestions
	suggestions, err := s.trackedSearch.GetSearchSuggestions(ctx, query, 5)
	if err != nil {
		s.logger.Warn("search suggestions failed", zap.Error(err))
	} else {
		results.Suggestions = suggestions
	}

	// 5. Calculate total costs
	totalCost := s.costTracker.CalculateCost()
	results.TotalCostMicros = totalCost.TotalCostMicroCents
	results.CostBreakdown["dynamo_reads"] = totalCost.DynamoDBReads * 25 / 1000000 // Convert to microcents
	results.CostBreakdown["dynamo_writes"] = totalCost.DynamoDBWrites * 125 / 1000000

	// 6. Update user budget
	if err := s.costRepo.RecordBudgetUsage(ctx, userID, "comprehensive_search", results.TotalCostMicros); err != nil {
		s.logger.Warn("failed to update budget usage", zap.Error(err))
	}

	s.logger.Info("comprehensive_search_completed",
		zap.String("user_id", userID),
		zap.String("query", query),
		zap.Int("total_results", len(actors)+len(statuses)+len(hashtags)),
		zap.Bool("semantic_enabled", useSemanticSearch),
		zap.Int64("total_cost_micros", results.TotalCostMicros))

	return results, nil
}

// BudgetManagementExample demonstrates budget management for search operations
func (s *SearchServiceIntegration) BudgetManagementExample(ctx context.Context, userID string) (*BudgetSummary, error) {
	// Get current budget
	budget, err := s.costRepo.GetUserBudget(ctx, userID, "daily")
	if err != nil {
		return nil, err
	}

	// Get recent search costs
	costSummary, err := s.costRepo.GetSearchCostSummary(ctx, userID, "daily")
	if err != nil {
		return nil, err
	}

	// Calculate budget utilization
	utilizationPercent := float64(budget.UsedBudgetMicros) / float64(budget.BudgetLimitMicros) * 100

	// Determine budget status
	var status string
	var recommendations []string

	switch {
	case utilizationPercent > 90:
		status = "critical"
		recommendations = []string{
			"Consider reducing search frequency",
			"Use cached results when possible",
			"Avoid expensive semantic searches",
		}
	case utilizationPercent > 70:
		status = "warning"
		recommendations = []string{
			"Monitor search usage closely",
			"Consider optimizing search queries",
		}
	default:
		status = "healthy"
		recommendations = []string{
			"Budget usage is within normal limits",
		}
	}

	return &BudgetSummary{
		Budget:             budget,
		CostSummary:        costSummary,
		UtilizationPercent: utilizationPercent,
		Status:             status,
		Recommendations:    recommendations,
	}, nil
}

// PerformanceAnalyticsExample demonstrates performance analytics collection
func (s *SearchServiceIntegration) PerformanceAnalyticsExample(ctx context.Context, _ string, _ int) (*PerformanceAnalytics, error) {
	// Get popular queries
	popularQueries, err := s.costRepo.GetPopularQueries(ctx, 10, "daily")
	if err != nil {
		return nil, err
	}

	// Calculate efficiency metrics
	analytics := &PerformanceAnalytics{
		PopularQueries: popularQueries,
		Metrics:        make(map[string]interface{}),
	}

	// Analyze query patterns
	var totalCost, totalResults int64
	var totalResponseTime int64
	var cacheHitCount int

	for _, query := range popularQueries {
		totalCost += query.TotalCostMicros
		totalResults += query.TotalResultCount
		totalResponseTime += int64(query.AverageResponseTimeMs * float64(query.QueryCount))
		cacheHitCount += int(query.CacheHitCount)
	}

	if len(popularQueries) > 0 {
		analytics.Metrics["average_cost_per_query"] = totalCost / int64(len(popularQueries))
		analytics.Metrics["average_results_per_query"] = totalResults / int64(len(popularQueries))
		analytics.Metrics["average_response_time_ms"] = totalResponseTime / int64(len(popularQueries))
		analytics.Metrics["cache_hit_rate"] = float64(cacheHitCount) / float64(len(popularQueries))
	}

	// Cost efficiency recommendations
	if totalResults > 0 {
		costPerResult := totalCost / totalResults
		if costPerResult > 1000 { // > 1000 microcents per result
			analytics.Recommendations = append(analytics.Recommendations,
				"High cost per result - consider query optimization")
		}
	}

	return analytics, nil
}

// Helper methods
func (s *SearchServiceIntegration) estimateSearchCost(query string, useSemanticSearch bool) int64 {
	baseCost := int64(500) // Base search cost in microcents

	// Add cost for query complexity
	queryCost := int64(len(query) * 2) // 2 microcents per character

	// Add semantic search cost if enabled
	var semanticCost int64
	if useSemanticSearch {
		semanticCost = 5000 // Semantic search is expensive
	}

	return baseCost + queryCost + semanticCost
}

// Data structures for examples

// SearchResultsWithCosts combines search results with cost tracking information
type SearchResultsWithCosts struct {
	Accounts        []*activitypub.Actor          `json:"accounts"`
	Statuses        []*storage.StatusSearchResult `json:"statuses"`
	Hashtags        []*storage.Hashtag            `json:"hashtags"`
	SemanticResults []*models.SearchEmbedding     `json:"semantic_results,omitempty"`
	Suggestions     []*models.SearchSuggestion    `json:"suggestions,omitempty"`
	QueryEmbedding  []float32                     `json:"query_embedding,omitempty"`
	TotalCostMicros int64                         `json:"total_cost_micros"`
	CostBreakdown   map[string]int64              `json:"cost_breakdown"`
}

// BudgetSummary contains budget usage statistics for search operations
type BudgetSummary struct {
	Budget             *models.SearchBudget `json:"budget"`
	CostSummary        *SearchCostSummary   `json:"cost_summary"`
	UtilizationPercent float64              `json:"utilization_percent"`
	Status             string               `json:"status"`
	Recommendations    []string             `json:"recommendations"`
}

// PerformanceAnalytics contains performance metrics and recommendations
type PerformanceAnalytics struct {
	PopularQueries  []*models.SearchQueryStats `json:"popular_queries"`
	Metrics         map[string]interface{}     `json:"metrics"`
	Recommendations []string                   `json:"recommendations"`
}

// GetCostTracker returns the cost tracker for external monitoring
func (s *SearchServiceIntegration) GetCostTracker() *cost.Tracker {
	return s.costTracker
}

// GetCostSummary returns the current cost summary
func (s *SearchServiceIntegration) GetCostSummary() *cost.OperationCost {
	return s.costTracker.CalculateCost()
}
