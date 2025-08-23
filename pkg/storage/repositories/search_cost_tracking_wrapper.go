package repositories

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// SearchCostTrackingWrapper wraps a SearchRepository with comprehensive cost tracking
type SearchCostTrackingWrapper struct {
	searchRepo      *SearchRepository
	costRepo        *SearchCostRepository
	costTracker     *cost.Tracker
	unifiedTracker  *cost.UnifiedTracker
	tableName       string
	logger          *zap.Logger
}

// NewSearchCostTrackingWrapper creates a new cost tracking wrapper for search operations
func NewSearchCostTrackingWrapper(searchRepo *SearchRepository, costRepo *SearchCostRepository, costTracker *cost.Tracker, logger *zap.Logger) *SearchCostTrackingWrapper {
	cfg := config.Get()
	tableName := cfg.DynamoTableName
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main"
	}
	
	// Create unified tracker for centralized cost tracking
	unifiedTracker := cost.NewRepositoryTracker(nil, logger, "SearchCostTrackingWrapper", "", "")
	
	return &SearchCostTrackingWrapper{
		searchRepo:      searchRepo,
		costRepo:        costRepo,
		costTracker:     costTracker,
		unifiedTracker:  unifiedTracker,
		tableName:       tableName,
		logger:          logger,
	}
}

// SearchAccounts wraps account search with cost tracking
func (w *SearchCostTrackingWrapper) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	return w.searchAccountsWithCostTracking(ctx, "", query, limit, followingOnly, offset, "text_search")
}

// SearchAccountsAdvanced wraps advanced account search with cost tracking
func (w *SearchCostTrackingWrapper) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	return w.searchAccountsAdvancedWithCostTracking(ctx, accountID, query, resolve, limit, offset, following, "user_search")
}

// SearchStatuses wraps status search with cost tracking
func (w *SearchCostTrackingWrapper) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	return w.searchStatusesWithCostTracking(ctx, "", query, limit, "text_search")
}

// SearchStatusesWithOptions wraps status search with options and cost tracking
func (w *SearchCostTrackingWrapper) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	return w.searchStatusesWithOptionsAndCostTracking(ctx, options.AccountID, query, options, "text_search")
}

// SearchStatusesAdvanced wraps advanced status search with cost tracking
func (w *SearchCostTrackingWrapper) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	return w.searchStatusesAdvancedWithCostTracking(ctx, accountID, query, limit, maxID, minID, "text_search")
}

// SearchAll wraps comprehensive search with cost tracking
func (w *SearchCostTrackingWrapper) SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error) {
	return w.searchAllWithCostTracking(ctx, accountID, query, limit, "all_search")
}

// SearchHashtags wraps hashtag search with cost tracking
func (w *SearchCostTrackingWrapper) SearchHashtags(ctx context.Context, query string, limit int) ([]*storage.Hashtag, error) {
	return w.searchHashtagsWithCostTracking(ctx, "", query, limit, "hashtag_search")
}

// SearchHashtagsAdvanced wraps advanced hashtag search with cost tracking
func (w *SearchCostTrackingWrapper) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	return w.searchHashtagsAdvancedWithCostTracking(ctx, accountID, query, limit, "hashtag_search")
}

// GetSearchSuggestions wraps search suggestions with cost tracking
func (w *SearchCostTrackingWrapper) GetSearchSuggestions(ctx context.Context, prefix string, limit int) ([]*models.SearchSuggestion, error) {
	return w.getSearchSuggestionsWithCostTracking(ctx, "", prefix, limit, "search_suggestions")
}

// SearchByEmbedding wraps semantic search with cost tracking
func (w *SearchCostTrackingWrapper) SearchByEmbedding(ctx context.Context, queryEmbedding []float32, limit int, threshold float64) ([]*models.SearchEmbedding, error) {
	return w.searchByEmbeddingWithCostTracking(ctx, "", queryEmbedding, limit, threshold, "semantic_search")
}

// Core implementation methods with cost tracking

func (w *SearchCostTrackingWrapper) searchAccountsWithCostTracking(ctx context.Context, userID, query string, limit int, followingOnly bool, offset int, operationType string) ([]*activitypub.Actor, error) {
	// Start cost tracking
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, query, "accounts")

	// Check budget before proceeding
	estimatedCost := w.estimateSearchCost(operationType, limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			w.logger.Warn("search budget exceeded",
				zap.String("user_id", userID),
				zap.String("operation", operationType),
				zap.Error(err))
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	// Track database operations
	var queryCount, dynamoReads int64

	// Execute the search
	results, err := w.searchRepo.SearchAccounts(ctx, query, limit, followingOnly, offset)

	// Estimate database operations (based on search strategies)
	queryCount = 2 // Exact match + prefix search
	if len(query) >= 2 {
		queryCount++ // Display name search
	}
	dynamoReads = queryCount * int64(limit/10+1) // Estimate reads per query

	// Complete cost tracking
	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) searchAccountsAdvancedWithCostTracking(ctx context.Context, userID, query string, resolve bool, limit int, offset int, following bool, operationType string) ([]*activitypub.Actor, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, query, "accounts")

	// Check budget
	estimatedCost := w.estimateSearchCost(operationType, limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	var queryCount, dynamoReads int64

	// Execute the search
	results, err := w.searchRepo.SearchAccountsAdvanced(ctx, query, resolve, limit, offset, following, userID)

	// Estimate operations
	queryCount = 3 // More complex search strategies
	if following {
		queryCount++ // Additional following lookup
	}
	dynamoReads = queryCount * int64(limit/10+1)

	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) searchStatusesWithCostTracking(ctx context.Context, userID, query string, limit int, operationType string) ([]*storage.StatusSearchResult, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, query, "statuses")

	estimatedCost := w.estimateSearchCost(operationType, limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	var queryCount, dynamoReads int64

	results, err := w.searchRepo.SearchStatuses(ctx, query, limit)

	// Status search is more expensive due to content scanning
	queryCount = 3                              // URL, hashtag, content search
	dynamoReads = queryCount * int64(limit/5+1) // More reads for content search

	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) searchStatusesWithOptionsAndCostTracking(ctx context.Context, userID, query string, options storage.StatusSearchOptions, operationType string) ([]*storage.StatusSearchResult, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, query, "statuses")

	estimatedCost := w.estimateSearchCost(operationType, options.Limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	var queryCount, dynamoReads int64

	results, err := w.searchRepo.SearchStatusesWithOptions(ctx, query, options)

	queryCount = 3
	if options.AccountID != "" {
		queryCount++ // Additional filtering
	}
	if options.LocalOnly {
		queryCount++ // Local filtering
	}
	dynamoReads = queryCount * int64(options.Limit/5+1)

	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) searchStatusesAdvancedWithCostTracking(ctx context.Context, userID, query string, limit int, maxID, minID *string, operationType string) ([]*storage.StatusSearchResult, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, query, "statuses")

	estimatedCost := w.estimateSearchCost(operationType, limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	var queryCount, dynamoReads int64

	results, err := w.searchRepo.SearchStatusesAdvanced(ctx, query, limit, maxID, minID, userID)

	queryCount = 4 // Advanced search with pagination
	dynamoReads = queryCount * int64(limit/5+1)

	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) searchAllWithCostTracking(ctx context.Context, userID, query string, limit int, operationType string) (*storage.SearchResults, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, query, "all")

	estimatedCost := w.estimateSearchCost(operationType, limit*3) // Searches across multiple types
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	var queryCount, dynamoReads int64

	results, err := w.searchRepo.SearchAll(ctx, query, limit, userID)

	// SearchAll queries accounts, statuses, and hashtags
	queryCount = 8 // Multiple searches across different types
	dynamoReads = queryCount * int64(limit/8+1)

	totalResults := len(results.Accounts) + len(results.Statuses) + len(results.Hashtags)
	w.completeCostTracking(ctx, costData, startTime, totalResults, queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) searchHashtagsWithCostTracking(ctx context.Context, userID, query string, limit int, operationType string) ([]*storage.Hashtag, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, query, "hashtags")

	estimatedCost := w.estimateSearchCost(operationType, limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	var queryCount, dynamoReads int64

	results, err := w.searchRepo.SearchHashtags(ctx, query, limit)

	queryCount = 1                    // Simple hashtag GSI query
	dynamoReads = int64(limit/20 + 1) // Efficient hashtag search

	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) searchHashtagsAdvancedWithCostTracking(ctx context.Context, userID, query string, limit int, operationType string) ([]*storage.HashtagSearchResult, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, query, "hashtags")

	estimatedCost := w.estimateSearchCost(operationType, limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	var queryCount, dynamoReads int64

	results, err := w.searchRepo.SearchHashtagsAdvanced(ctx, query, limit, userID)

	queryCount = 2 // Hashtag search + trend data
	dynamoReads = int64(limit/15 + 1)

	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) getSearchSuggestionsWithCostTracking(ctx context.Context, userID, prefix string, limit int, operationType string) ([]*models.SearchSuggestion, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, prefix, "suggestions")

	estimatedCost := w.estimateSearchCost(operationType, limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "budget check")
		}
	}

	var queryCount, dynamoReads int64

	results, err := w.searchRepo.GetSearchSuggestions(ctx, prefix, limit)

	queryCount = 3                    // Username, display name, hashtag searches
	dynamoReads = int64(limit/10 + 1) // Efficient suggestion lookup

	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

func (w *SearchCostTrackingWrapper) searchByEmbeddingWithCostTracking(ctx context.Context, userID string, queryEmbedding []float32, limit int, threshold float64, operationType string) ([]*models.SearchEmbedding, error) {
	startTime := time.Now()
	costData := w.initializeCostData(ctx, userID, operationType, fmt.Sprintf("vector_%d", len(queryEmbedding)), "semantic")

	// Semantic search is expensive
	estimatedCost := w.estimateSemanticSearchCost(len(queryEmbedding), limit)
	if userID != "" {
		if err := w.costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "search cost tracking", "semantic budget check")
		}
	}

	var queryCount, dynamoReads int64

	results, err := w.searchRepo.SearchByEmbedding(ctx, queryEmbedding, limit, threshold)

	// Semantic search scans embeddings and computes similarity
	queryCount = 1                                         // Single scan operation
	dynamoReads = 500                                      // Scans many embeddings for comparison
	costData.VectorComparisons = len(queryEmbedding) * 100 // Estimate comparisons
	costData.EmbeddingDimension = len(queryEmbedding)

	w.completeCostTracking(ctx, costData, startTime, len(results), queryCount, dynamoReads, err)

	return results, err
}

// Helper methods

func (w *SearchCostTrackingWrapper) initializeCostData(ctx context.Context, userID, operationType, query, searchType string) *models.SearchCostTracking {
	requestID := w.getRequestID(ctx)

	return &models.SearchCostTracking{
		UserID:        userID,
		RequestID:     requestID,
		OperationType: operationType,
		Query:         query,
		SearchType:    searchType,
		QueryLength:   len(query),
		Timestamp:     time.Now(),
	}
}

func (w *SearchCostTrackingWrapper) completeCostTracking(ctx context.Context, costData *models.SearchCostTracking, startTime time.Time, resultCount int, queryCount, dynamoReads int64, err error) {
	// Calculate response time
	responseTime := time.Since(startTime)
	costData.ResponseTimeMs = responseTime.Milliseconds()
	costData.ResultCount = resultCount
	costData.DynamoQueries = int(queryCount)
	costData.DynamoReads = dynamoReads
	costData.DynamoWrites = 0 // Search operations don't write

	// Estimate GSI queries (assume 50% of queries use GSI)
	costData.GSIQueries = int(queryCount / 2)

	// Track costs using centralized tracker
	if err := w.unifiedTracker.TrackDynamoRead(ctx, w.tableName, dynamoReads); err != nil {
		w.logger.Warn("failed to track cost", zap.Error(err))
	}
	// Search operations don't typically write to DynamoDB
	// Writes would be tracked if costData.DynamoWrites > 0

	// Record the cost data (async to not impact response time)
	go func() {
		ctx := context.Background() // Use background context for async logging
		if err := w.costRepo.RecordSearchCost(ctx, costData); err != nil {
			w.logger.Error("failed to record search cost",
				zap.String("user_id", costData.UserID),
				zap.String("operation", costData.OperationType),
				zap.Error(err))
		}
	}()

	// Log search metrics
	w.logger.Info("search_operation_completed",
		zap.String("user_id", costData.UserID),
		zap.String("operation_type", costData.OperationType),
		zap.String("search_type", costData.SearchType),
		zap.Int("result_count", resultCount),
		zap.Int64("response_time_ms", costData.ResponseTimeMs),
		zap.Int64("dynamo_reads", dynamoReads),
		zap.Bool("success", err == nil))
}

func (w *SearchCostTrackingWrapper) estimateSearchCost(operationType string, limit int) int64 {
	// Estimate cost in microcents based on operation type and result limit
	baseCost := int64(100) // Base cost: 100 microcents

	switch operationType {
	case "text_search", "user_search":
		return baseCost + int64(limit*2) // 2 microcents per potential result
	case "hashtag_search":
		return baseCost + int64(limit*1) // Hashtag search is cheaper
	case "all_search":
		return baseCost + int64(limit*5) // Search across all types
	case "search_suggestions":
		return baseCost + int64(limit*1) // Suggestions are cached/optimized
	default:
		return baseCost + int64(limit*3)
	}
}

func (w *SearchCostTrackingWrapper) estimateSemanticSearchCost(embeddingDim, limit int) int64 {
	// Semantic search is much more expensive due to vector operations
	baseCost := int64(5000) // 5000 microcents base cost

	// Cost increases with embedding dimension and result limit
	dimCost := int64(embeddingDim * 10) // 10 microcents per dimension
	resultCost := int64(limit * 100)    // 100 microcents per potential result

	return baseCost + dimCost + resultCost
}

func (w *SearchCostTrackingWrapper) getRequestID(ctx context.Context) string {
	// Try to get request ID from context
	if requestID, ok := ctx.Value("request_id").(string); ok {
		return requestID
	}

	// Generate a simple request ID if not available
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return fmt.Sprintf("%x", hash[:4])
}

// Forward remaining SearchRepository methods without cost tracking (for compatibility)

// SetDependencies forwards dependency configuration to the underlying search repository
func (w *SearchCostTrackingWrapper) SetDependencies(deps SearchRepositoryDeps) {
	w.searchRepo.SetDependencies(deps)
}

// CreateSearchSuggestion forwards search suggestion creation to the underlying repository
func (w *SearchCostTrackingWrapper) CreateSearchSuggestion(ctx context.Context, suggestion *models.SearchSuggestion) error {
	return w.searchRepo.CreateSearchSuggestion(ctx, suggestion)
}

// UpdateSearchSuggestion forwards search suggestion updates to the underlying repository
func (w *SearchCostTrackingWrapper) UpdateSearchSuggestion(ctx context.Context, suggestionType, term string, updates map[string]interface{}) error {
	return w.searchRepo.UpdateSearchSuggestion(ctx, suggestionType, term, updates)
}

// IncrementSuggestionUse forwards suggestion use count increments to the underlying repository
func (w *SearchCostTrackingWrapper) IncrementSuggestionUse(ctx context.Context, suggestionType, term string) error {
	return w.searchRepo.IncrementSuggestionUse(ctx, suggestionType, term)
}

// PruneOldSuggestions forwards old suggestion cleanup to the underlying repository
func (w *SearchCostTrackingWrapper) PruneOldSuggestions(ctx context.Context, olderThan time.Time) error {
	return w.searchRepo.PruneOldSuggestions(ctx, olderThan)
}

// IndexStatus forwards status indexing operations to the underlying repository
func (w *SearchCostTrackingWrapper) IndexStatus(ctx context.Context, status *models.Object) error {
	return w.searchRepo.IndexStatus(ctx, status)
}

// UnindexStatus forwards status unindexing operations to the underlying repository
func (w *SearchCostTrackingWrapper) UnindexStatus(ctx context.Context, statusID string) error {
	return w.searchRepo.UnindexStatus(ctx, statusID)
}

// SearchStatusesByHashtag forwards hashtag-based status searches to the underlying repository
func (w *SearchCostTrackingWrapper) SearchStatusesByHashtag(ctx context.Context, hashtag string, limit int) ([]*storage.StatusSearchResult, error) {
	return w.searchRepo.SearchStatusesByHashtag(ctx, hashtag, limit)
}

// SearchStatusesByAuthor forwards author-based status searches to the underlying repository
func (w *SearchCostTrackingWrapper) SearchStatusesByAuthor(ctx context.Context, authorID string, limit int) ([]*storage.StatusSearchResult, error) {
	return w.searchRepo.SearchStatusesByAuthor(ctx, authorID, limit)
}

// RecordSearch forwards search event recording to the underlying repository
func (w *SearchCostTrackingWrapper) RecordSearch(ctx context.Context, event *models.SearchAnalytics) error {
	return w.searchRepo.RecordSearch(ctx, event)
}

// GetSearchAnalytics forwards search analytics retrieval to the underlying repository
func (w *SearchCostTrackingWrapper) GetSearchAnalytics(ctx context.Context, startDate, endDate time.Time) ([]*models.SearchAnalytics, error) {
	return w.searchRepo.GetSearchAnalytics(ctx, startDate, endDate)
}

// GetPopularSearches forwards popular search queries retrieval to the underlying repository
func (w *SearchCostTrackingWrapper) GetPopularSearches(ctx context.Context, limit int, timeWindow time.Duration) ([]*models.SearchQueryStats, error) {
	return w.searchRepo.GetPopularSearches(ctx, limit, timeWindow)
}

// GetSearchTrends forwards search trend analysis to the underlying repository
func (w *SearchCostTrackingWrapper) GetSearchTrends(ctx context.Context, days int) (map[string]int, error) {
	return w.searchRepo.GetSearchTrends(ctx, days)
}

// IndexContentEmbedding forwards content embedding indexing to the underlying repository
func (w *SearchCostTrackingWrapper) IndexContentEmbedding(ctx context.Context, embedding *models.SearchEmbedding) error {
	return w.searchRepo.IndexContentEmbedding(ctx, embedding)
}

// UpdateEmbedding forwards embedding updates to the underlying repository
func (w *SearchCostTrackingWrapper) UpdateEmbedding(ctx context.Context, contentID string, embedding []float32) error {
	return w.searchRepo.UpdateEmbedding(ctx, contentID, embedding)
}

// DeleteEmbedding forwards embedding deletion to the underlying repository
func (w *SearchCostTrackingWrapper) DeleteEmbedding(ctx context.Context, contentID string) error {
	return w.searchRepo.DeleteEmbedding(ctx, contentID)
}
