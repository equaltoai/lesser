# Relay Cost Tracking Implementation

## Overview

This document outlines the comprehensive relay cost tracking implementation added to the Lesser ActivityPub server. The implementation provides detailed cost tracking, budget management, and optimization recommendations for relay operations using DynamORM/Lift patterns.

## Architecture

The relay cost tracking system consists of several key components:

1. **DynamORM Models** - Type-safe data models for cost tracking
2. **Repository Methods** - Cost tracking operations using DynamORM patterns  
3. **Service Integration** - Cost tracking integrated into relay operations
4. **Budget Management** - Budget limits and alert system
5. **Cost Optimization** - Recommendations and analytics

## Key Components

### 1. DynamORM Models (`pkg/storage/models/relay_cost.go`)

#### RelayCost Model
Tracks individual relay operation costs:
- **Primary Key**: `RELAY_COST#{relayURL}#{operationType}`
- **Sort Key**: `TS#{timestamp}#{requestID}`
- **GSI1**: Query by relay URL (`RELAY_COSTS#{relayURL}`)
- **GSI2**: Query by date (`RELAY_COSTS_DAILY#{date}`)

**Cost Categories Tracked:**
- HTTP request costs ($0.0001 per request)
- Data transfer costs ($0.09 per GB outbound)
- Lambda processing costs (based on duration)
- DynamoDB operation costs
- SQS message costs

#### RelayMetrics Model
Aggregated relay metrics:
- **Primary Key**: `RELAY_METRICS#{relayURL}#{period}`
- **Sort Key**: `WINDOW#{windowStart}`
- **GSI1**: Query by relay URL across periods

**Aggregated Data:**
- Total operations, success/failure counts
- Cost breakdowns by operation type
- Performance metrics (response time, success rate)
- Budget usage tracking

#### RelayBudget Model
Budget configuration for relays:
- **Primary Key**: `RELAY_BUDGET#{relayURL}#{period}`
- **Sort Key**: `CONFIG`

**Budget Features:**
- Configurable limits per period (daily/weekly/monthly)
- Warning and critical thresholds
- Automated actions (pause relay, reduce frequency)
- Usage tracking and reset logic

### 2. Repository Methods (`pkg/storage/repositories/cost_tracking_repository.go`)

Extended CostTrackingRepository with relay-specific methods:

#### Core Methods
- `CreateRelayCost()` - Store individual cost records
- `GetRelayCostsByURL()` - Query costs for specific relay
- `GetRelayCostsByDateRange()` - Query costs across date range
- `AggregateRelayCosts()` - Aggregate raw costs into metrics

#### Budget Management
- `CreateRelayBudget()` / `UpdateRelayBudget()` - Budget CRUD operations
- `GetRelayBudget()` - Retrieve budget configuration
- `GetRelayCostSummary()` - Cost analysis and statistics

#### Analytics
- `GetHighCostRelayOperations()` - Identify expensive operations
- `GetRelayMetricsHistory()` - Historical performance data

### 3. Service Integration

#### RelayService (`pkg/federation/relay.go`)
Enhanced with comprehensive cost tracking:

**SubscribeToRelay()** - Tracks subscription costs:
- HTTP requests to fetch relay actor
- Data transfer for Follow activity
- Lambda processing time
- DynamoDB operations for storing relay info

**ForwardToRelays()** - Tracks forwarding costs:
- Budget checks before delivery
- Per-relay cost tracking
- Success/failure rate monitoring
- Bulk operation optimization

**HandleRelayActivity()** - Tracks inbound processing:
- Activity processing costs
- Lambda execution time
- DynamoDB updates

#### Cost Calculation Methods
- `calculateDataTransferCost()` - AWS data transfer pricing
- `calculateLambdaCost()` - Lambda execution pricing
- `calculateDynamoDBCost()` - DynamoDB operation pricing
- Budget checking with early termination

### 4. Budget Management Service (`pkg/federation/relay_budget.go`)

**RelayBudgetService** provides:

#### Budget Operations
- `CreateRelayBudget()` - Set up budget limits
- `UpdateRelayBudgetUsage()` - Track actual usage
- `GetRelayBudgetStatus()` - Current budget status
- `CheckRelayBudget()` - Pre-operation budget validation

#### Automated Management
- Period-based budget resets (daily/weekly/monthly)
- Threshold monitoring (warning/critical)
- Automated actions (pause relay, reduce frequency)
- Admin notifications

#### Analytics & Recommendations
- `GetRelayBudgetRecommendations()` - Optimization suggestions
- Cost efficiency analysis
- Usage pattern analysis
- Suggested budget limits

### 5. Outbox Integration (`cmd/outbox/main.go`)

Enhanced outbox processor with relay detection:

**Relay Detection:**
- Pattern matching for relay inbox URLs
- Domain-based relay identification
- Automatic cost categorization

**Cost Tracking:**
- Per-delivery cost calculation
- Retry cost penalties
- Payload size-based pricing
- Success/failure tracking

## Cost Tracking Categories

### 1. Outbound Relay Costs
- **HTTP Requests**: $0.0001 per request
- **Data Transfer**: $0.09 per GB (free inbound)
- **Lambda Processing**: $0.0000166667 per GB-second
- **DynamoDB Operations**: ~$0.75 per million mixed operations
- **SQS Messages**: $0.0000004 per message

### 2. Inbound Relay Costs
- **Lambda Processing**: Processing received activities
- **DynamoDB Operations**: Storing and indexing activities
- **Data Transfer**: Free (AWS doesn't charge for inbound)

### 3. Administrative Costs
- **Budget Management**: DynamoDB operations for budget tracking
- **Metrics Aggregation**: Periodic cost rollups
- **Analytics**: Query costs for reporting

## Usage Examples

### Setting Up a Relay Budget

```go
budgetService := federation.NewRelayBudgetService(store, logger)

// Create daily budget of $1.00
err := budgetService.CreateRelayBudget(ctx, 
    "https://relay.example.com", 
    "daily", 
    1000000, // $1.00 in microdollars
    75.0,    // Warning at 75%
    90.0)    // Critical at 90%
```

### Checking Budget Before Operation

```go
relayService := federation.NewRelayService(store, domain, logger)

// Budget check is automatic in ForwardToRelays
err := relayService.ForwardToRelays(ctx, activity, actor)
if err != nil && strings.Contains(err.Error(), "budget") {
    // Handle budget exceeded
}
```

### Getting Cost Analytics

```go
// Get cost summary for last 30 days
summary, err := store.Cost().GetRelayCostSummary(ctx, 
    "https://relay.example.com",
    time.Now().AddDate(0, 0, -30),
    time.Now())

// Get optimization recommendations
recommendations, err := budgetService.GetRelayBudgetRecommendations(ctx,
    "https://relay.example.com")
```

## Monitoring and Alerts

### Budget Thresholds
- **Warning (75%)**: Log warning, send notification
- **Critical (90%)**: Log error, trigger actions
- **Exceeded (100%)**: Block operations, pause relay

### Automated Actions
- **Pause Relay**: Stop forwarding to over-budget relays
- **Reduce Frequency**: Implement backoff for expensive relays
- **Admin Notifications**: Alert administrators of budget issues

### Cost Optimization
- **High-Cost Operation Detection**: Identify expensive operations
- **Success Rate Analysis**: Remove unreliable relays
- **Usage Pattern Analysis**: Optimize forwarding patterns
- **Budget Recommendations**: Suggest optimal budget limits

## Performance Considerations

### DynamoDB Design
- **Single Table Design**: All relay data in main table
- **Efficient Querying**: GSIs for time-based and relay-based queries
- **TTL Management**: Automatic cleanup of old cost records
- **Hot Partition Avoidance**: Date-based partitioning

### Cost Tracking Overhead
- **Async Processing**: Cost tracking doesn't block main operations
- **Batched Writes**: Efficient DynamoDB usage
- **Intelligent Sampling**: Reduce tracking overhead for high-volume relays
- **Lazy Aggregation**: On-demand metrics calculation

### Error Handling
- **Graceful Degradation**: Operations continue if cost tracking fails
- **Retry Logic**: Robust error handling for tracking operations
- **Fallback Logging**: Log costs if database operations fail

## Deployment Considerations

### Environment Variables
- Cost tracking enabled/disabled per environment
- Budget alert endpoints
- Cost calculation parameters

### Monitoring
- CloudWatch metrics for cost tracking operations
- Alarms for budget threshold breaches
- Dashboard for relay cost visualization

### Security
- Budget configuration access controls
- Cost data encryption at rest
- Secure admin notification channels

## Future Enhancements

### Advanced Analytics
- Machine learning-based cost prediction
- Anomaly detection for unusual cost patterns
- Automated budget optimization

### Integration Improvements
- Real-time cost streaming
- Integration with AWS Cost Explorer
- Third-party monitoring systems

### Optimization Features
- Intelligent relay selection based on cost
- Dynamic frequency adjustment
- Cost-aware activity prioritization

## Cost Model Summary

This implementation provides comprehensive relay cost tracking with:

1. **Granular Tracking**: Every relay operation tracked with detailed cost breakdown
2. **Budget Management**: Flexible budget limits with automated enforcement
3. **Performance Analytics**: Success rates, response times, cost efficiency
4. **Optimization Recommendations**: Data-driven suggestions for cost reduction
5. **Operational Safety**: Budget limits prevent runaway costs

The system follows Lesser's existing patterns using DynamORM for data access and Lift for Lambda handling, ensuring consistency with the rest of the codebase while providing powerful cost management capabilities.