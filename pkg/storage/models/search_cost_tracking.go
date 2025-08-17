package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// SearchCostTracking tracks costs for search operations
type SearchCostTracking struct {
	PK   string `dynamorm:"pk"` // SEARCH_COST#date#user_id
	SK   string `dynamorm:"sk"` // OPERATION#timestamp#operation_type
	Type string `json:"type"`   // Always "SearchCostTracking"

	// Core identifiers
	UserID        string `json:"user_id"`
	RequestID     string `json:"request_id"`
	OperationType string `json:"operation_type"` // text_search, hashtag_search, user_search, semantic_search, search_suggestions, search_indexing

	// Search-specific details
	Query           string  `json:"query"`
	SearchType      string  `json:"search_type"`      // accounts, statuses, hashtags, all, suggestions, semantic
	ResultCount     int     `json:"result_count"`     // Number of results returned
	QueryLength     int     `json:"query_length"`     // Length of search query
	CacheHit        bool    `json:"cache_hit"`        // Whether result was cached
	QueryComplexity float64 `json:"query_complexity"` // Complexity score for the query

	// DynamoDB costs
	DynamoReads    int64 `json:"dynamo_reads"`    // Read capacity units consumed
	DynamoWrites   int64 `json:"dynamo_writes"`   // Write capacity units consumed
	DynamoQueries  int   `json:"dynamo_queries"`  // Number of DynamoDB queries executed
	GSIQueries     int   `json:"gsi_queries"`     // Number of GSI queries executed
	ScanOperations int   `json:"scan_operations"` // Number of scan operations

	// Bedrock/AI costs (for semantic search)
	BedrockRequests    int   `json:"bedrock_requests"`    // Number of Bedrock API calls
	EmbeddingTokens    int   `json:"embedding_tokens"`    // Tokens used for embedding generation
	EmbeddingDimension int   `json:"embedding_dimension"` // Dimension of embeddings
	VectorComparisons  int   `json:"vector_comparisons"`  // Number of vector similarity comparisons
	BedrockCostMicros  int64 `json:"bedrock_cost_micros"` // Bedrock cost in microcents

	// Lambda execution costs
	LambdaDurationMs int64 `json:"lambda_duration_ms"` // Lambda execution time
	LambdaMemoryMB   int64 `json:"lambda_memory_mb"`   // Lambda memory allocation

	// Search performance metrics
	ResponseTimeMs    int64 `json:"response_time_ms"`     // Total response time
	IndexLookupTimeMs int64 `json:"index_lookup_time_ms"` // Time spent on index lookups
	FilteringTimeMs   int64 `json:"filtering_time_ms"`    // Time spent filtering results
	RankingTimeMs     int64 `json:"ranking_time_ms"`      // Time spent ranking/sorting results

	// Cost calculations
	TotalCostMicros  int64 `json:"total_cost_micros"`  // Total cost in microcents
	DynamoCostMicros int64 `json:"dynamo_cost_micros"` // DynamoDB cost in microcents
	LambdaCostMicros int64 `json:"lambda_cost_micros"` // Lambda cost in microcents
	CostPerResult    int64 `json:"cost_per_result"`    // Cost per result in microcents
	EstimatedSavings int64 `json:"estimated_savings"`  // Savings from caching in microcents

	// Timestamps
	Timestamp time.Time `json:"timestamp"`
	Date      string    `json:"date"` // YYYY-MM-DD for aggregation

	// TTL for cleanup
	TTL int64 `dynamorm:"ttl"`
}

// UpdateKeys sets the composite keys for search cost tracking
func (sct *SearchCostTracking) UpdateKeys() {
	if sct.Timestamp.IsZero() {
		sct.Timestamp = time.Now()
	}

	if err := common.ValidateRequiredParam("sct.Date", sct.Date); err != nil {
		sct.Date = sct.Timestamp.Format(common.DateFormat)
	}

	// PK: SEARCH_COST#date#user_id for daily aggregation by user
	sct.PK = fmt.Sprintf("SEARCH_COST#%s#%s", sct.Date, sct.UserID)

	// SK: OPERATION#timestamp#operation_type for chronological ordering
	sct.SK = fmt.Sprintf("OPERATION#%s#%s",
		sct.Timestamp.Format("15:04:05.000"),
		sct.OperationType)

	sct.Type = "SearchCostTracking"

	// Set TTL for 90 days
	if sct.TTL == 0 {
		sct.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
}

// SearchBudget tracks search budgets and limits per user
type SearchBudget struct {
	PK   string `dynamorm:"pk"` // SEARCH_BUDGET#user_id
	SK   string `dynamorm:"sk"` // PERIOD#date_period
	Type string `json:"type"`   // Always "SearchBudget"

	// Budget identifiers
	UserID     string `json:"user_id"`
	Period     string `json:"period"`      // daily, monthly, yearly
	PeriodDate string `json:"period_date"` // 2024-01-15, 2024-01, 2024

	// Budget limits (in microcents)
	BudgetLimitMicros    int64 `json:"budget_limit_micros"`    // Total budget limit
	SearchBudgetMicros   int64 `json:"search_budget_micros"`   // Budget for regular search
	SemanticBudgetMicros int64 `json:"semantic_budget_micros"` // Budget for semantic search
	IndexingBudgetMicros int64 `json:"indexing_budget_micros"` // Budget for search indexing

	// Current usage (in microcents)
	UsedBudgetMicros   int64 `json:"used_budget_micros"`   // Total used budget
	SearchUsedMicros   int64 `json:"search_used_micros"`   // Used for regular search
	SemanticUsedMicros int64 `json:"semantic_used_micros"` // Used for semantic search
	IndexingUsedMicros int64 `json:"indexing_used_micros"` // Used for indexing

	// Request limits
	MaxRequestsPerHour      int `json:"max_requests_per_hour"`     // Max search requests per hour
	MaxSemanticPerHour      int `json:"max_semantic_per_hour"`     // Max semantic searches per hour
	CurrentRequests         int `json:"current_requests"`          // Current requests in period
	CurrentSemanticRequests int `json:"current_semantic_requests"` // Current semantic requests

	// Budget status
	BudgetExceeded bool      `json:"budget_exceeded"` // Whether budget is exceeded
	LastResetTime  time.Time `json:"last_reset_time"` // When budget was last reset
	LastUsageTime  time.Time `json:"last_usage_time"` // When budget was last used
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// TTL for cleanup
	TTL int64 `dynamorm:"ttl"`
}

// UpdateKeys sets the composite keys for search budget tracking
func (sb *SearchBudget) UpdateKeys() {
	// PK: SEARCH_BUDGET#user_id
	sb.PK = fmt.Sprintf("SEARCH_BUDGET#%s", sb.UserID)

	// SK: PERIOD#date_period for different time periods
	sb.SK = fmt.Sprintf("PERIOD#%s", sb.PeriodDate)

	sb.Type = "SearchBudget"

	// Set TTL based on period type
	if sb.TTL == 0 {
		switch sb.Period {
		case PeriodDaily:
			sb.TTL = time.Now().Add(7 * 24 * time.Hour).Unix() // Keep daily for 7 days
		case PeriodMonthly:
			sb.TTL = time.Now().Add(365 * 24 * time.Hour).Unix() // Keep monthly for 1 year
		case "yearly":
			sb.TTL = time.Now().Add(5 * 365 * 24 * time.Hour).Unix() // Keep yearly for 5 years
		default:
			sb.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
		}
	}
}

// CanMakeRequest checks if user can make a search request within budget
func (sb *SearchBudget) CanMakeRequest(operationType string, estimatedCostMicros int64) bool {
	// Check overall budget
	if sb.UsedBudgetMicros+estimatedCostMicros > sb.BudgetLimitMicros {
		return false
	}

	// Check specific operation budget
	switch operationType {
	case "semantic_search":
		if sb.SemanticUsedMicros+estimatedCostMicros > sb.SemanticBudgetMicros {
			return false
		}
		if sb.CurrentSemanticRequests >= sb.MaxSemanticPerHour {
			return false
		}
	case "text_search", "hashtag_search", "user_search":
		if sb.SearchUsedMicros+estimatedCostMicros > sb.SearchBudgetMicros {
			return false
		}
		if sb.CurrentRequests >= sb.MaxRequestsPerHour {
			return false
		}
	case "search_indexing":
		if sb.IndexingUsedMicros+estimatedCostMicros > sb.IndexingBudgetMicros {
			return false
		}
	}

	return true
}

// RecordUsage records usage against the budget
func (sb *SearchBudget) RecordUsage(operationType string, costMicros int64) {
	sb.UsedBudgetMicros += costMicros
	sb.LastUsageTime = time.Now()
	sb.UpdatedAt = time.Now()

	switch operationType {
	case "semantic_search":
		sb.SemanticUsedMicros += costMicros
		sb.CurrentSemanticRequests++
	case "text_search", "hashtag_search", "user_search":
		sb.SearchUsedMicros += costMicros
		sb.CurrentRequests++
	case "search_indexing":
		sb.IndexingUsedMicros += costMicros
	}

	// Update budget exceeded status
	sb.BudgetExceeded = sb.UsedBudgetMicros >= sb.BudgetLimitMicros
}

// SearchCostAggregation provides aggregated cost data for reporting
type SearchCostAggregation struct {
	PK   string `dynamorm:"pk"` // SEARCH_AGG#date#aggregation_type
	SK   string `dynamorm:"sk"` // METRIC#metric_name
	Type string `json:"type"`   // Always "SearchCostAggregation"

	// Aggregation identifiers
	Date            string `json:"date"`             // 2024-01-15
	AggregationType string `json:"aggregation_type"` // daily, hourly, user, operation_type
	MetricName      string `json:"metric_name"`      // total_cost, avg_cost_per_search, etc.

	// Aggregated metrics
	TotalCostMicros      int64   `json:"total_cost_micros"`
	TotalRequests        int64   `json:"total_requests"`
	AverageCostMicros    int64   `json:"average_cost_micros"`
	MedianResponseTimeMs int64   `json:"median_response_time_ms"`
	CacheHitRate         float64 `json:"cache_hit_rate"`

	// Operation breakdown
	TextSearches       int64 `json:"text_searches"`
	HashtagSearches    int64 `json:"hashtag_searches"`
	UserSearches       int64 `json:"user_searches"`
	SemanticSearches   int64 `json:"semantic_searches"`
	SuggestionRequests int64 `json:"suggestion_requests"`
	IndexingOperations int64 `json:"indexing_operations"`

	// Cost breakdown
	DynamoCostMicros  int64 `json:"dynamo_cost_micros"`
	BedrockCostMicros int64 `json:"bedrock_cost_micros"`
	LambdaCostMicros  int64 `json:"lambda_cost_micros"`

	// Performance metrics
	AverageResponseTimeMs int64 `json:"average_response_time_ms"`
	P95ResponseTimeMs     int64 `json:"p95_response_time_ms"`
	AverageResultCount    int   `json:"average_result_count"`

	// Timestamps
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	GeneratedAt time.Time `json:"generated_at"`

	// TTL for cleanup
	TTL int64 `dynamorm:"ttl"`
}

// UpdateKeys sets the composite keys for search cost aggregation
func (sca *SearchCostAggregation) UpdateKeys() {
	// PK: SEARCH_AGG#date#aggregation_type
	sca.PK = fmt.Sprintf("SEARCH_AGG#%s#%s", sca.Date, sca.AggregationType)

	// SK: METRIC#metric_name
	sca.SK = fmt.Sprintf("METRIC#%s", sca.MetricName)

	sca.Type = "SearchCostAggregation"

	// Set TTL for 1 year for aggregated data
	if sca.TTL == 0 {
		sca.TTL = time.Now().Add(365 * 24 * time.Hour).Unix()
	}
}

// SearchQueryStats tracks statistics for specific search queries
type SearchQueryStats struct {
	PK   string `dynamorm:"pk"` // SEARCH_STATS#query_hash
	SK   string `dynamorm:"sk"` // STATS#period
	Type string `json:"type"`   // Always "SearchQueryStats"

	// Query identifiers
	QueryHash   string `json:"query_hash"`   // Hash of the query for privacy
	QueryType   string `json:"query_type"`   // text, hashtag, user, semantic
	QueryLength int    `json:"query_length"` // Length of original query
	Period      string `json:"period"`       // daily, weekly, monthly

	// Usage statistics
	QueryCount       int64   `json:"query_count"`        // Number of times queried
	UniqueUsers      int     `json:"unique_users"`       // Number of unique users
	TotalResultCount int64   `json:"total_result_count"` // Total results returned
	AverageResults   float64 `json:"average_results"`    // Average results per query
	CacheHitCount    int64   `json:"cache_hit_count"`    // Number of cache hits
	CacheHitRate     float64 `json:"cache_hit_rate"`     // Cache hit percentage

	// Performance statistics
	TotalResponseTimeMs   int64   `json:"total_response_time_ms"`   // Total response time
	AverageResponseTimeMs float64 `json:"average_response_time_ms"` // Average response time
	MinResponseTimeMs     int64   `json:"min_response_time_ms"`     // Minimum response time
	MaxResponseTimeMs     int64   `json:"max_response_time_ms"`     // Maximum response time

	// Cost statistics
	TotalCostMicros   int64   `json:"total_cost_micros"`   // Total cost for this query
	AverageCostMicros float64 `json:"average_cost_micros"` // Average cost per execution
	CostEfficiency    float64 `json:"cost_efficiency"`     // Cost per result ratio

	// Timestamps
	FirstQueried time.Time `json:"first_queried"`
	LastQueried  time.Time `json:"last_queried"`
	UpdatedAt    time.Time `json:"updated_at"`

	// TTL for cleanup
	TTL int64 `dynamorm:"ttl"`
}

// UpdateKeys sets the composite keys for search query stats
func (sqs *SearchQueryStats) UpdateKeys() {
	// PK: SEARCH_STATS#query_hash
	sqs.PK = fmt.Sprintf("SEARCH_STATS#%s", sqs.QueryHash)

	// SK: STATS#period
	sqs.SK = fmt.Sprintf("STATS#%s", sqs.Period)

	sqs.Type = "SearchQueryStats"

	// Set TTL based on period
	if sqs.TTL == 0 {
		switch sqs.Period {
		case PeriodDaily:
			sqs.TTL = time.Now().Add(30 * 24 * time.Hour).Unix() // Keep daily for 30 days
		case PeriodWeekly:
			sqs.TTL = time.Now().Add(90 * 24 * time.Hour).Unix() // Keep weekly for 90 days
		case PeriodMonthly:
			sqs.TTL = time.Now().Add(365 * 24 * time.Hour).Unix() // Keep monthly for 1 year
		default:
			sqs.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
		}
	}
}
