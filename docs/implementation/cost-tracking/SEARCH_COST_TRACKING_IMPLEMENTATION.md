# Search Cost Tracking Implementation

This document outlines the comprehensive search cost tracking implementation using DynamORM/Lift patterns for the Lesser ActivityPub server.

## Overview

The search cost tracking system provides detailed monitoring and budgeting for all search operations, including:
- Text search (accounts, content, hashtags)
- Semantic search using AI embeddings
- Search suggestions and autocomplete
- Search indexing operations
- Budget limits and rate limiting per user

## Cost Categories Tracked

### 1. DynamoDB Operations
- **Read Capacity Units (RCU)**: $0.25 per million read units
- **Write Capacity Units (WCU)**: $1.25 per million write units  
- **GSI Query Operations**: Tracked separately for advanced search
- **Scan Operations**: For full-text search fallbacks

### 2. AWS Bedrock/AI Operations
- **Embedding Generation**: ~$0.0001 per 1K tokens (Titan embeddings)
- **Token Usage**: Tracked for semantic search queries
- **Vector Comparisons**: Computational cost for similarity matching
- **AI Content Analysis**: For content moderation and analysis

### 3. Lambda Execution Costs
- **Execution Time**: $0.0000166667 per GB-second
- **Memory Allocation**: Cost varies by memory configuration
- **Cold Start Impact**: Tracked for performance optimization

### 4. Search-Specific Metrics
- **Query Complexity**: Weighted scoring based on search strategies
- **Cache Hit Rate**: Cost savings from result caching
- **Result Quality**: Cost-per-result efficiency metrics

## Implementation Architecture

### Models (`pkg/storage/models/search_cost_tracking.go`)

#### SearchCostTracking
Primary cost tracking record for individual search operations:
```go
type SearchCostTracking struct {
    // Keys: PK=SEARCH_COST#date#user_id, SK=OPERATION#timestamp#operation_type
    UserID        string  // User performing search
    OperationType string  // text_search, semantic_search, etc.
    Query         string  // Search query (for analysis)
    ResultCount   int     // Results returned
    
    // DynamoDB costs
    DynamoReads   int64   // RCU consumed
    DynamoWrites  int64   // WCU consumed
    
    // AI/Bedrock costs
    BedrockRequests    int     // API calls to Bedrock
    EmbeddingTokens    int     // Tokens processed
    VectorComparisons  int     // Similarity computations
    
    // Performance metrics
    ResponseTimeMs     int64   // Total response time
    CacheHit          bool    // Whether result was cached
    
    // Cost calculations
    TotalCostMicros   int64   // Total cost in microcents
}
```

#### SearchBudget
Budget tracking and limits per user:
```go
type SearchBudget struct {
    // Keys: PK=SEARCH_BUDGET#user_id, SK=PERIOD#date_period
    UserID                 string  // User ID
    BudgetLimitMicros     int64   // Total budget limit
    SemanticBudgetMicros  int64   // Budget for AI search
    UsedBudgetMicros      int64   // Currently used budget
    MaxRequestsPerHour    int     // Rate limits
    BudgetExceeded        bool    // Budget status
}
```

#### SearchCostAggregation
Aggregated cost data for reporting and analytics:
```go
type SearchCostAggregation struct {
    // Keys: PK=SEARCH_AGG#date#aggregation_type, SK=METRIC#metric_name
    TotalCostMicros       int64   // Aggregated costs
    TotalRequests         int64   // Request volume
    CacheHitRate          float64 // Efficiency metrics
    AverageResponseTimeMs int64   // Performance metrics
}
```

### Repositories

#### SearchCostRepository (`pkg/storage/repositories/search_cost_repository.go`)
Manages cost tracking, budgets, and analytics:

**Key Methods:**
- `RecordSearchCost()`: Records individual search operation costs
- `CheckBudget()`: Validates if user can perform search within budget
- `RecordBudgetUsage()`: Updates budget consumption
- `GetSearchCosts()`: Retrieves cost history for analysis
- `GetSearchCostSummary()`: Provides aggregated cost insights

**Default Budget Limits:**
- Daily budget: $0.01 per user
- Regular search: $0.008 allocation
- Semantic search: $0.0015 allocation (more expensive)
- Search indexing: $0.0005 allocation
- Request limits: 100 searches/hour, 10 semantic searches/hour

#### SearchCostTrackingWrapper (`pkg/storage/repositories/search_cost_tracking_wrapper.go`)
Decorator pattern wrapper that adds cost tracking to all search operations:

**Features:**
- Pre-operation budget checking
- Real-time cost estimation
- Post-operation cost recording
- Performance metrics collection
- Automatic budget enforcement

**Tracked Operations:**
- `SearchAccounts()`: User/account search
- `SearchStatuses()`: Content/status search  
- `SearchHashtags()`: Hashtag search
- `SearchAll()`: Comprehensive search
- `GetSearchSuggestions()`: Autocomplete
- `SearchByEmbedding()`: Semantic search

### AI Cost Tracking (`pkg/ai/cost_tracking.go`)

#### AIServiceWithCostTracking
Wraps AI service with detailed cost tracking for semantic search:

**Key Methods:**
- `GenerateEmbeddingWithCostTracking()`: Tracks embedding generation costs
- `SemanticSearchWithCostTracking()`: End-to-end semantic search tracking
- `AnalyzeContentWithCostTracking()`: Content analysis cost tracking
- `BulkEmbeddingGenerationWithCostTracking()`: Optimized bulk processing

**Cost Calculation:**
- Token estimation: ~4 characters per token
- Bedrock API costs: Base 50 microcents + 100 microcents per 1K tokens
- Vector search costs: Includes similarity computation overhead

### Search Indexer Cost Tracking

#### Modified SearchIndexer (`cmd/search-indexer/main.go`)
Tracks costs for search indexing operations:

**Features:**
- Cost tracking for DynamoDB writes during indexing
- Performance metrics for indexing operations
- Asynchronous cost recording to avoid performance impact
- Write operation estimation (main index + GSI indexes + tag indexes)

**Cost Factors:**
- Main search index: 1 write operation
- Actor-specific index: +1 write operation  
- Tag indexes: +1 write per tag
- Total cost: Write operations × $1.25 per million writes

## Integration Points

### Lift Context Integration
Cost tracking integrates with Lift framework context:
```go
// Budget checking before search
if err := costRepo.CheckBudget(ctx, userID, operationType, estimatedCost); err != nil {
    return nil, fmt.Errorf("search budget exceeded: %w", err)
}

// Cost recording after search
costData := &models.SearchCostTracking{
    UserID:        userID,
    RequestID:     ctx.GetRequestID(),
    OperationType: operationType,
    // ... other fields
}
```

### DynamORM Cost Tracking
Leverages existing cost tracking infrastructure:
```go
// Track DynamoDB operations
if costTracker != nil {
    costTracker.TrackDynamoRead(int(readCount))
    costTracker.TrackDynamoWrite(int(writeCount))
}
```

## Cost Optimization Strategies

### 1. Budget-Based Rate Limiting
- Per-user daily budgets prevent runaway costs
- Different budget allocations for different operation types
- Real-time budget checking before expensive operations

### 2. Caching Strategy
- Track cache hit rates to measure efficiency
- Estimated savings calculation for cached results
- Cache optimization recommendations based on cost analysis

### 3. Query Optimization
- Query complexity scoring to identify expensive patterns
- Search strategy selection based on cost-effectiveness
- GSI usage optimization to minimize scan operations

### 4. Performance Monitoring
- Response time tracking for cost-performance analysis
- Result quality metrics (cost per relevant result)
- Search pattern analysis for optimization opportunities

## Usage Examples

### Basic Search with Cost Tracking
```go
// Wrap search repository with cost tracking
searchRepo := repositories.NewSearchRepository(db, logger)
costRepo := repositories.NewSearchCostRepository(db, logger)
costTracker := cost.NewWithRequest(requestID, "search")

wrappedSearch := repositories.NewSearchCostTrackingWrapper(
    searchRepo, costRepo, costTracker, logger)

// Search accounts with automatic cost tracking
actors, err := wrappedSearch.SearchAccounts(ctx, query, limit, false, 0)
```

### Semantic Search with AI Cost Tracking
```go
// AI service with cost tracking
aiService := ai.NewAIService(awsConfig, aiConfig)
costTracker := cost.NewWithRequest(requestID, "semantic_search")
wrappedAI := ai.NewAIServiceWithCostTracking(aiService, costTracker, logger)

// Generate embeddings with cost tracking
embedding, costData, err := wrappedAI.GenerateEmbeddingWithCostTracking(
    ctx, query, userID)
```

### Budget Management
```go
// Check user budget before expensive operation
err := costRepo.CheckBudget(ctx, userID, "semantic_search", estimatedCost)
if err != nil {
    return fmt.Errorf("budget exceeded: %w", err)
}

// Get user's current budget status
budget, err := costRepo.GetUserBudget(ctx, userID, "daily")
fmt.Printf("Used: %d/%d microcents", budget.UsedBudgetMicros, budget.BudgetLimitMicros)
```

## Monitoring and Analytics

### Cost Dashboards
- Real-time cost monitoring per user and operation type
- Budget utilization tracking and alerts
- Cost trend analysis and forecasting

### Performance Analytics
- Search response time percentiles
- Cache hit rate optimization
- Query complexity distribution

### Business Intelligence
- Cost per user segmentation
- Search pattern analysis
- ROI analysis for search features

## Benefits

### 1. Cost Control
- Prevents runaway search costs through budget limits
- Real-time cost monitoring and alerting
- Granular cost attribution by user and operation type

### 2. Performance Optimization
- Identifies expensive search patterns for optimization
- Cache effectiveness measurement
- Query optimization opportunities

### 3. Business Intelligence
- User search behavior analysis
- Feature usage and cost-effectiveness metrics
- Data-driven optimization decisions

### 4. Compliance and Reporting
- Detailed audit trail for all search operations
- Cost allocation for multi-tenant scenarios
- Performance SLA monitoring

## Future Enhancements

### 1. Machine Learning Optimization
- Predictive budget management
- Automated query optimization
- Smart caching strategies

### 2. Advanced Analytics
- Anomaly detection for unusual search patterns
- Cost optimization recommendations
- Predictive scaling based on search trends

### 3. Multi-Tenant Enhancements
- Organization-level budget management
- Cross-user search pattern analysis
- Tenant-specific optimization strategies

## Summary

The search cost tracking implementation provides comprehensive monitoring and control over all search-related operations in the Lesser ActivityPub server. By leveraging DynamORM/Lift patterns, it offers:

- **Complete Cost Visibility**: Track every microcent spent on search operations
- **Budget Control**: Prevent cost overruns with per-user budget limits
- **Performance Optimization**: Identify and optimize expensive operations
- **Business Intelligence**: Data-driven insights for search feature development

The system is designed to scale with the application while maintaining sub-millisecond overhead for cost tracking operations, ensuring that monitoring doesn't impact user experience.