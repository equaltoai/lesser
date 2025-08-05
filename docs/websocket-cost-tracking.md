# WebSocket Cost Tracking Implementation

This document describes the comprehensive WebSocket cost tracking system implemented for Lesser using DynamORM/Lift patterns.

## Overview

The WebSocket cost tracking system provides detailed monitoring and budget management for WebSocket connections used in real-time streaming. It tracks all costs associated with:

- API Gateway WebSocket connections and messages
- Lambda execution costs for WebSocket handlers
- DynamoDB operations for connection management
- Data transfer costs for streaming
- Connection duration and idle time tracking

## Architecture

### Core Components

1. **WebSocketCostRecord Model** (`pkg/storage/models/websocket_cost_tracking.go`)
   - Detailed cost tracking for individual WebSocket operations
   - Uses DynamORM patterns with proper GSI keys
   - Tracks connection lifecycle, messages, and performance metrics

2. **WebSocketCostBudget Model** 
   - Per-user budget management with configurable limits
   - Supports daily, weekly, and monthly budget periods
   - Automatic alerts and connection suspension capabilities

3. **WebSocketCostAggregation Model**
   - Pre-computed cost aggregations for analysis
   - Supports multiple time periods (hour, day, week, month)
   - Includes cost breakdowns, percentiles, and efficiency metrics

4. **WebSocketCostRepository** (`pkg/storage/repositories/websocket_cost_repository.go`)
   - DynamORM-based repository for all WebSocket cost operations
   - Supports queries by user, connection, operation type, and time range
   - Includes budget management and aggregation functions

5. **WebSocketCostTracker** (`pkg/cost/websocket_cost_tracker.go`)
   - Main cost tracking service with middleware integration
   - Tracks operation costs in real-time
   - Manages budget limits and provides analytics

### Lambda Functions

1. **streaming** (`cmd/streaming/main.go`)
   - Enhanced with cost tracking middleware
   - Tracks connect, disconnect, and message operations
   - Automatically records costs for all WebSocket activities

2. **websocket-cost-aggregator** (`cmd/websocket-cost-aggregator/main.go`)
   - Scheduled Lambda for cost aggregation and cleanup
   - Tracks idle connection costs
   - Performs hourly and daily cost aggregations
   - Manages budget alerts and notifications

## Cost Model

### Pricing Components

Based on AWS pricing (as of 2024):

- **API Gateway WebSocket Connection**: $0.25 per million connection minutes
- **API Gateway WebSocket Messages**: $1.00 per million messages  
- **Lambda Execution**: $0.0000166667 per GB-second (512MB function)
- **DynamoDB Operations**: Standard on-demand pricing
- **Data Transfer**: $0.09 per GB outbound

All costs are tracked in **microcents** for precision (1 microcent = $0.000001).

### Operation Types

- `connect` - WebSocket connection establishment
- `disconnect` - WebSocket disconnection
- `message_in` - Messages received from clients
- `message_out` - Messages sent to clients
- `subscribe` - Stream subscription operations
- `unsubscribe` - Stream unsubscription operations
- `idle_time` - Connection idle time tracking
- `ping` - Ping/pong operations
- `error` - Error handling operations

## Key Features

### Real-Time Cost Tracking

- Every WebSocket operation automatically tracked
- Costs calculated using accurate AWS pricing models
- Detailed breakdown by cost category (connection, messages, Lambda, DynamoDB)
- Performance metrics including latency and memory usage

### Budget Management

- Per-user budget limits with configurable periods
- Automatic alerts at configurable thresholds (e.g., 50%, 75%, 90%)
- Connection suspension when budgets are exceeded
- Rate limiting based on budget status

### Cost Analytics

- Comprehensive cost analytics API (`cmd/api/lift/websocket_cost_analytics.go`)
- Top users by cost analysis
- High-cost operation identification
- Cost trends and growth analysis
- Efficiency metrics and optimization recommendations

### Data Models

#### WebSocketCostRecord Key Patterns

```
PK: WS_COST#{operation_type}
SK: ts#{timestamp}#{id}

GSI1PK: WS_CONN#{connection_id}    # Connection-based queries
GSI1SK: {timestamp}#{operation_type}#{id}

GSI2PK: WS_USER#{user_id}          # User-based queries  
GSI2SK: {timestamp}#{operation_type}#{id}
```

#### WebSocketCostBudget Key Patterns

```
PK: WS_BUDGET#{user_id}#{period}
SK: BUDGET#{period}

GSI1PK: WS_USER_BUDGET#{user_id}   # User budget queries
GSI1SK: {period}#{status}
```

#### WebSocketCostAggregation Key Patterns

```
PK: WS_AGG#{period}#{operation_type}
SK: window#{windowStart}

GSI1PK: WS_USER_AGG#{user_id}#{period}  # User aggregations
GSI1SK: {timestamp}#{operation_type}
```

## API Endpoints

### Cost Analytics

```http
GET /api/v1/websocket/analytics
```

Parameters:
- `start_date`: Start date (YYYY-MM-DD format)
- `end_date`: End date (YYYY-MM-DD format)
- `period`: Aggregation period (hour, day, week, month)
- `user_id`: Optional user filter
- `limit`: Result limit (default 50)

### User Budget Management

```http
GET /api/v1/users/{user_id}/websocket/budget
POST /api/v1/users/{user_id}/websocket/budget
```

Budget creation body:
```json
{
  "period": "daily",
  "budget_dollars": 1.00,
  "alert_thresholds": [50, 75, 90],
  "suspend_at": 100,
  "max_connections": 10,
  "messages_per_minute": 100
}
```

## Configuration

### Environment Variables

- `WEBSOCKET_COST_TRACKING_ENABLED`: Enable/disable cost tracking (default: true)
- `WEBSOCKET_BUDGET_ENFORCEMENT`: Enable budget enforcement (default: true)
- `WEBSOCKET_IDLE_TRACKING_INTERVAL`: Idle tracking interval in minutes (default: 5)
- `WEBSOCKET_AGGREGATION_SCHEDULE`: Cost aggregation schedule (default: hourly)

### Scheduled Events

The `websocket-cost-aggregator` should be scheduled to run:

- **Every 5 minutes**: Idle connection tracking
- **Every hour**: Hourly cost aggregation
- **Daily at midnight**: Daily cost aggregation and budget resets
- **Weekly**: Budget alerts and cleanup

Example CloudWatch Events rule:
```json
{
  "ScheduleExpression": "rate(5 minutes)",
  "Targets": [
    {
      "Arn": "arn:aws:lambda:region:account:function:websocket-cost-aggregator",
      "Input": "{\"operations\": [\"idle_tracking\"]}"
    }
  ]
}
```

## Performance Considerations

### Lambda Optimizations

- Cost tracking runs asynchronously to avoid impacting WebSocket response times
- Uses DynamORM Lambda-optimized client (91% faster cold starts)
- Minimal memory footprint with efficient batch operations

### DynamoDB Design

- Single-table design with optimized GSI access patterns
- TTL-based automatic cleanup (30 days for detailed records, 90 days for aggregations)
- Batch operations for bulk cost record creation

### Cost Efficiency

- Tracking overhead: < 0.1% of total WebSocket costs
- Cost records automatically expire to manage storage costs
- Aggregations reduce query costs for analytics

## Monitoring and Alerts

### Metrics to Monitor

- Total WebSocket costs per user/period
- Budget utilization percentages
- High-cost operations (> $0.01)
- Connection duration outliers
- Message throughput rates

### Alert Conditions

- User exceeds 90% of budget
- Single operation costs > $0.10
- Connection duration > 24 hours
- Message rate > 1000/minute per connection
- Error rate > 5%

## Usage Examples

### Track WebSocket Operation Cost

```go
// In a WebSocket handler
opCtx := &cost.WebSocketOperationContext{
    ConnectionID:  connectionID,
    UserID:        userID,
    OperationType: "message_in",
    StartTime:     time.Now(),
}

result := &cost.WebSocketOperationResult{
    Success:          true,
    MessageCount:     1,
    MessageSizeBytes: int64(len(messageBody)),
}

err := costTracker.TrackWebSocketOperation(ctx, opCtx, result)
```

### Check Budget Limits

```go
budgetStatus, err := costTracker.CheckBudgetLimits(ctx, userID)
if err != nil {
    return err
}

if !budgetStatus.AllowConnection {
    return lift.NewLiftError("BUDGET_EXCEEDED", "WebSocket budget exceeded", 429)
}
```

### Get User Cost Summary

```go
summary, err := costRepo.GetUserCostSummary(ctx, userID, startTime, endTime)
if err != nil {
    return err
}

fmt.Printf("User %s spent $%.4f on WebSocket operations\n", 
    userID, summary.TotalCostDollars)
```

## Testing

### Unit Tests

- Repository tests with DynamORM mocks
- Cost calculation validation
- Budget limit enforcement

### Integration Tests

- End-to-end WebSocket operation tracking
- Budget alert generation
- Cost aggregation accuracy

### Load Tests

- High-volume WebSocket connections
- Cost tracking performance under load
- Budget enforcement at scale

## Future Enhancements

### Advanced Analytics

- Machine learning for cost prediction
- Anomaly detection for unusual usage patterns
- Cost optimization recommendations

### Enhanced Budgeting

- Hierarchical budgets (organization → team → user)
- Dynamic budget adjustments based on usage patterns
- Integration with billing systems

### Real-Time Monitoring

- Live cost dashboards
- Real-time budget alerts via webhooks
- Usage trend visualizations

## Troubleshooting

### Common Issues

1. **Cost tracking not working**: Check middleware order and DynamORM configuration
2. **Budget limits not enforced**: Verify budget records exist and are not expired
3. **High aggregation costs**: Optimize aggregation queries and reduce frequency
4. **Missing cost records**: Check async tracking error logs

### Debug Queries

```go
// Check recent cost records for a user
costs, err := costRepo.ListByUser(ctx, userID, time.Now().Add(-24*time.Hour), time.Now(), 100)

// Verify budget configuration
budget, err := costRepo.GetBudget(ctx, userID, "daily")

// Check aggregation status
agg, err := costRepo.GetAggregation(ctx, "hour", "connect", hourStart)
```

This comprehensive WebSocket cost tracking system provides detailed visibility into WebSocket usage costs while maintaining the performance and scalability requirements of the Lesser platform.