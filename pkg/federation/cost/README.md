# Cost-Aware Federation

This package implements intelligent cost tracking and management for federation activities in Lesser. It provides budget controls, health monitoring, and tiered service levels for federated instances.

## Features

### 1. Cost Tracking & Budgeting
- Real-time cost estimation for all federation activities
- Per-instance and global budget enforcement
- AWS service cost calculations (Lambda, DynamoDB, S3, Data Transfer)
- Cost transparency headers in federation requests

### 2. Instance Health Monitoring
- Automatic health scoring based on success rate, response time, and failures
- Instance quarantine for consistently failing servers
- Intelligent retry policies based on instance health

### 3. Tiered Federation Service
- **Premium**: Unlimited federation, priority processing
- **Standard**: Normal limits and processing
- **Limited**: Reduced limits, lower priority
- **Blocked**: No federation allowed

### 4. Smart Retry Logic
- Instance-specific retry policies
- Exponential backoff with jitter
- Health-aware retry decisions

## Usage

### Basic Setup

```go
import (
    "github.com/aron23/lesser/pkg/federation/cost"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// Create storage backend
storage := cost.NewDynamoStorage(
    dynamoClient,
    "lesser-main-table",
    logger,
    costTracker,
)

// Define budget limits
budget := &cost.FederationBudget{
    TotalBudgetUSD:       1000.0,  // $1000/month total
    PerInstanceBudgetUSD: 10.0,    // $10/month per instance
    BudgetPeriod:         "monthly",
    InstanceOverrides: map[string]float64{
        "mastodon.social": 50.0,   // Higher budget for large instances
    },
}

// Set thresholds
thresholds := &cost.CostThresholds{
    WarnThresholdPercent:  80,  // Warn at 80% budget
    BlockThresholdPercent: 95,  // Block at 95% budget
}

// Create controller
calculator := cost.NewCostCalculator("us-east-1")
controller := cost.NewController(
    storage,
    calculator,
    logger,
    budget,
    thresholds,
)
```

### Integration with Federation Delivery

```go
// Wrap existing delivery function
deliveryMiddleware := cost.NewDeliveryMiddleware(controller, logger)
wrappedDelivery := deliveryMiddleware.WrapDelivery(originalDeliveryFunc)

// Use wrapped delivery
err := wrappedDelivery(ctx, "https://mastodon.social", activityJSON)
```

### HTTP Transport Integration

```go
// Create cost-aware HTTP client
transport := cost.NewHTTPTransportWrapper(
    http.DefaultTransport,
    controller,
    logger,
)

client := &http.Client{
    Transport: transport,
    Timeout:   30 * time.Second,
}
```

### Retry with Instance-Specific Policy

```go
retryMiddleware := cost.NewRetryMiddleware(controller, logger)

err := retryMiddleware.RetryWithPolicy(ctx, "mastodon.social", func() error {
    // Your federation operation here
    return sendActivity(activity)
})
```

## DynamoDB Schema

The cost tracking uses the following DynamoDB patterns:

### Primary Table Structure

| PK | SK | Type | Purpose |
|---|---|---|---|
| `FEDCOST#domain` | `PERIOD#YYYY-MM` | FederationCost | Monthly cost tracking |
| `INSTANCE#domain` | `HEALTH` | InstanceHealth | Health metrics |
| `INSTANCE#domain` | `CONFIG` | InstanceConfig | Instance configuration |

### Global Secondary Indexes

**GSI1**: Period-based queries
- PK: `PERIOD#YYYY-MM`
- SK: `INSTANCE#domain`

**GSI2**: Unhealthy instances
- PK: `UNHEALTHY`
- SK: `SCORE#0.xxxx#domain`

**GSI3**: Tier-based queries
- PK: `TIER#premium/standard/limited`
- SK: `domain`

## Cost Calculation

The system tracks costs for:

1. **Data Transfer**: $0.09/GB (with tiered pricing)
2. **Lambda Invocations**: $0.20/million requests + compute time
3. **DynamoDB**: $0.25/million reads, $1.25/million writes
4. **S3 Storage**: $0.023/GB + request costs

Free tier allowances are automatically applied.

## Monitoring & Alerts

The system provides:

- Warning logs when instances approach budget limits
- Automatic instance health checks
- Cost metrics aggregation by period
- Instance reputation tracking

## Configuration Examples

### Premium Instance

```go
config := &cost.InstanceConfig{
    Domain: "important.social",
    Tier:   cost.TierPremium,
    CustomBudgetUSD: &premiumBudget,
    RetryPolicy: &cost.RetryPolicy{
        MaxRetries:     5,
        InitialBackoff: 500 * time.Millisecond,
        MaxBackoff:     30 * time.Second,
        BackoffFactor:  1.5,
    },
}
```

### Limiting Problematic Instance

```go
config := &cost.InstanceConfig{
    Domain: "spammy.instance",
    Tier:   cost.TierLimited,
    RateLimitOverride: &lowRateLimit,
    RetryPolicy: &cost.RetryPolicy{
        MaxRetries: 1,  // Minimal retries
    },
}
```

## Best Practices

1. **Set Reasonable Budgets**: Start conservative and adjust based on actual usage
2. **Monitor Health Metrics**: Use the unhealthy instances query to identify problems
3. **Use Tiering**: Assign appropriate tiers based on instance importance
4. **Review Cost Reports**: Regularly check cost metrics to optimize spending
5. **Implement Gradual Rollout**: Test with a few instances before enabling globally

## Future Enhancements

- Machine learning for cost prediction
- Automatic tier adjustment based on behavior
- Cross-region cost optimization
- Detailed cost attribution by activity type 