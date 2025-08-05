# Federation Cost Tracking Implementation

## Overview

I have successfully implemented comprehensive federation activity cost tracking for the Lesser ActivityPub implementation using DynamORM/Lift patterns. This system tracks costs for all federation activities including incoming and outgoing activities, HTTP signature verification, data transfer, DynamoDB operations, and more.

## Implementation Details

### 1. Enhanced Cost Tracking Models

**File:** `/Users/aronprice/lesser/pkg/storage/models/federation_cost_tracking.go`

#### FederationCostTracking Model
- **Primary Keys**: `FED_COST#{domain}#{timestamp}` pattern
- **GSI1**: Time-based queries (`FED_COSTS#{date}`, `TS#{timestamp}#{domain}`)  
- **GSI2**: Activity type queries (`FED_TYPE#{activity_type}`, `DOMAIN#{domain}#{timestamp}`)
- **TTL**: 30 days automatic cleanup

**Cost Categories Tracked:**
- Lambda execution costs (compute time + memory)
- HTTP signature verification costs (CPU intensive operations)
- Network costs (HTTP requests + data transfer)
- DynamoDB costs (read/write operations with capacity units)
- DNS/WebFinger lookup costs
- SQS message costs
- Retry penalty costs (exponential penalties)

**Performance Metrics:**
- Response time, processing time, queue wait time
- Payload size, compression ratio
- Success/failure rates by activity type

#### FederationBudget Model
- **Primary Keys**: `FED_BUDGET#{domain}#{period}` pattern
- **GSI1**: Active budget queries (`ACTIVE_BUDGETS`, `DOMAIN#{domain}#{period}`)
- **Per-instance budget limits** (inbound, outbound, combined)
- **Per-activity type limits** (Create, Follow, Like, etc.)
- **Alert thresholds** and enforcement settings
- **Automatic period reset** functionality

### 2. Cost Calculation Engine

**File:** `/Users/aronprice/lesser/pkg/federation/cost_calculator.go`

#### CostCalculator Features
- **Standardized AWS pricing rates** in microdollars (1/1,000,000 of a dollar)
- **Comprehensive cost estimation** for all federation operations
- **Activity-specific cost models** (different costs for Create vs Like activities)
- **Retry penalty calculations** (exponential cost increases)
- **Data transfer cost optimization** (only outbound charged)

#### Cost Rates (in microdollars)
- Lambda: ~17 microdollars per GB-second
- HTTP requests: 100 microdollars per request ($0.0001)
- Data transfer: 90,000 microdollars per GB ($0.09 outbound)
- DynamoDB writes: ~1 microdollar per request
- Signature verification: 5 microdollars base + time-based

### 3. Repository Implementation

**File:** `/Users/aronprice/lesser/pkg/storage/repositories/federation_cost_repository.go`

#### Key Methods
- `RecordFederationCost()` - Store detailed cost records
- `GetFederationCosts()` - Query costs by domain/time range  
- `GetFederationCostsByActivityType()` - Query by activity type
- `CheckBudgetLimits()` - Pre-flight budget validation
- `UpdateBudgetUsage()` - Update budget consumption
- `GetBudgetsOverLimit()` - Find domains exceeding budgets
- `ResetPeriodBudgets()` - Reset usage for new periods

#### Budget Management
- **Default budget creation** with conservative limits
- **Real-time budget enforcement** with blocking/rate limiting
- **Alert system** with configurable thresholds
- **Multi-period support** (daily, weekly, monthly)

### 4. Inbox Handler Integration

**File:** `/Users/aronprice/lesser/cmd/inbox/main.go`

#### Comprehensive Cost Tracking Added
- **Pre-processing cost estimation** 
- **Real-time budget limit checking**
- **Detailed activity processing cost tracking**:
  - Rate limiting costs
  - Domain blocking costs  
  - HTTP signature verification costs (CPU intensive)
  - Public key fetching costs (DNS + HTTP)
  - Activity processing costs by type
  - DynamoDB operation costs

#### Error Handling with Cost Tracking
Every error path now records comprehensive costs:
- Rate limit exceeded → Record partial processing costs
- Domain suspended → Record validation costs
- Key fetch failure → Record network costs  
- Signature verification failure → Record crypto costs
- Processing failure → Record full processing costs

### 5. Outbox Handler Integration

**File:** `/Users/aronprice/lesser/cmd/outbox/main.go`

#### Outbound Federation Cost Tracking
- **Pre-delivery budget validation** with blocking
- **Comprehensive delivery cost tracking**:
  - SQS message processing costs
  - HTTP delivery costs
  - Data transfer costs (charged for outbound)
  - Retry penalty costs
  - DNS lookup costs
  - DynamoDB tracking costs

#### Delivery Flow Cost Management
1. **Budget Check** → Block if over limit
2. **Delivery Attempt** → Track all costs
3. **Retry Logic** → Exponential cost penalties
4. **Success/Failure** → Record final costs + update budgets

## Cost Tracking Categories

### 1. Incoming Activities (Inbox)
- **Lambda execution**: Processing time + memory usage
- **Signature verification**: CPU-intensive cryptographic operations
- **HTTP requests**: Public key fetching from remote instances
- **DynamoDB operations**: Activity storage + relationship updates
- **Data transfer**: Inbound data (free) + DNS lookups
- **Processing**: Activity type-specific operations (Follow, Create, etc.)

### 2. Outgoing Activities (Outbox) 
- **Lambda execution**: Delivery processing time
- **HTTP requests**: Federation delivery to remote inboxes
- **Data transfer**: Outbound data (charged at $0.09/GB)
- **SQS messages**: Queue processing costs
- **DynamoDB operations**: Delivery tracking + status updates
- **Retry penalties**: Exponential cost increases for failed deliveries

### 3. Budget Management
- **Per-domain budgets**: Prevent federation abuse
- **Activity type limits**: Control expensive operations
- **Alert thresholds**: Proactive monitoring at 75% usage
- **Enforcement options**: Rate limiting vs hard blocking
- **Automatic reset**: Daily/weekly/monthly budget periods

## Key Features

### Cost Optimization
- **Comprehensive tracking** of all federation costs in microdollars
- **Real-time budget enforcement** to prevent runaway costs
- **Activity-specific cost models** (Create costs more than Like)
- **Retry penalty system** to discourage failed deliveries
- **Automatic TTL cleanup** of detailed cost records (30 days)

### Budget Protection
- **Default conservative budgets** for new domains
- **Multi-level enforcement**: Alerts → Rate limits → Hard blocks
- **Per-activity type limits** (e.g., limit expensive Create activities)
- **Budget check before processing** to prevent cost overruns
- **Historical cost tracking** for budget planning

### Performance Monitoring
- **Detailed response time tracking** by activity type and domain
- **Success/failure rate monitoring** with error categorization
- **Resource utilization tracking** (Lambda, DynamoDB, network)
- **Cost per activity** and **cost per byte** metrics
- **Compression ratio tracking** for payload optimization

## Integration Benefits

### 1. Cost Awareness
Every federation activity now has full cost visibility with breakdown by:
- Compute resources (Lambda execution time/memory)
- Network resources (HTTP requests, data transfer, DNS)
- Storage resources (DynamoDB read/write operations)  
- Cryptographic resources (signature verification CPU time)

### 2. Budget Protection
- **Prevents federation abuse** through per-domain budget limits
- **Early warning system** via configurable alert thresholds
- **Graduated enforcement** from alerts to rate limiting to blocking
- **Activity-specific controls** to limit expensive operations

### 3. Performance Optimization
- **Identifies expensive operations** through detailed cost breakdown
- **Tracks efficiency metrics** like compression ratios and response times
- **Monitors federation health** via success rates and error tracking
- **Provides cost attribution** by domain and activity type

## Usage Examples

### 1. Check Federation Costs
```go
costs, err := federationCostRepo.GetFederationCosts(ctx, "mastodon.social", startTime, endTime, 100)
for _, cost := range costs {
    log.Printf("Activity %s cost $%.6f", cost.ActivityType, cost.GetTotalCostDollars())
}
```

### 2. Set Domain Budget
```go
budget := &models.FederationBudget{
    Domain: "expensive.instance",
    Period: "daily", 
    CombinedLimitMicroCents: 50000, // $0.05 per day
    AlertThresholdPercent: 75.0,
    BlockOnLimitExceeded: true,
}
err := federationCostRepo.CreateOrUpdateBudget(ctx, budget)
```

### 3. Monitor Over-Budget Domains
```go
overBudgetDomains, err := federationCostRepo.GetBudgetsOverLimit(ctx, 100)
for _, budget := range overBudgetDomains {
    log.Printf("Domain %s is over budget: $%.6f / $%.6f", 
        budget.Domain,
        float64(budget.CurrentCombinedCost)/1000000,
        float64(budget.CombinedLimitMicroCents)/1000000)
}
```

## Files Created/Modified

### New Files
1. `/Users/aronprice/lesser/pkg/storage/models/federation_cost_tracking.go` - Cost tracking models
2. `/Users/aronprice/lesser/pkg/storage/repositories/federation_cost_repository.go` - Repository implementation  
3. `/Users/aronprice/lesser/pkg/federation/cost_calculator.go` - Cost calculation engine

### Modified Files
1. `/Users/aronprice/lesser/cmd/inbox/main.go` - Added comprehensive inbound cost tracking
2. `/Users/aronprice/lesser/cmd/outbox/main.go` - Added comprehensive outbound cost tracking

## Architecture Compliance

✅ **DynamORM/Lift Patterns**: All implementations follow existing patterns
✅ **Single Table Design**: Uses existing DynamoDB table with proper key patterns  
✅ **Cost Tracking Standards**: Follows existing cost tracking patterns (microdollars)
✅ **Error Handling**: Comprehensive error tracking with cost attribution
✅ **Performance**: Async cost recording to avoid impacting federation performance
✅ **TTL Management**: Automatic cleanup of detailed records with configurable retention

This implementation provides comprehensive federation cost tracking while maintaining the existing architectural patterns and performance characteristics of the Lesser ActivityPub implementation.