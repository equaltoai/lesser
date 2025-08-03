# Serverless Health Monitoring

This document describes the new EventBridge-triggered health monitoring system that replaces the problematic polling-based approach.

## Architecture Overview

The new serverless health monitoring system consists of:

1. **EventBridge-triggered health checks** - Lambda functions triggered by EventBridge events
2. **Synchronous health monitoring** - No background processes or polling
3. **DynamORM integration** - Uses DynamORM patterns instead of direct AWS SDK calls
4. **Configurable health checks** - Support for multiple component types and configurations

## Key Files

- `/pkg/monitoring/serverless_health.go` - Main serverless health monitor
- `/pkg/monitoring/component_checkers.go` - Individual component checkers (DynamoDB, Lambda, SQS)
- `/pkg/monitoring/health_event_types.go` - EventBridge event definitions
- `/pkg/storage/models/health_check_result.go` - DynamORM models for storing results

## Usage Example

### Basic Health Check

```go
// Initialize the monitor
monitor := monitoring.NewServerlessHealthMonitor(db, logger)

// Create a health check event
event := monitoring.HealthCheckEvent{
    Action: "check_health",
    Components: []monitoring.ComponentCheckConfig{
        {Type: "dynamodb", Identifier: "lesser-main", Name: "Main Database"},
        {Type: "lambda", Identifier: "lesser-api", Name: "API Handler"},
    },
    Options: monitoring.HealthCheckOptions{
        StoreResults:    true,
        PublishMetrics:  true,
        IncludeMetadata: true,
        TimeoutSeconds:  30,
        RetryAttempts:   2,
    },
}

// Process the health check synchronously
response, err := monitor.ProcessHealthCheckEvent(ctx, event)
if err != nil {
    log.Fatal("Health check failed:", err)
}

// Check results
fmt.Printf("Overall Status: %s\n", response.OverallStatus)
fmt.Printf("Execution Time: %dms\n", response.ExecutionTime)
for _, result := range response.ComponentResults {
    fmt.Printf("Component %s: %s (%dms)\n", 
        result.Component, result.Status, result.LatencyMs)
}
```

### Lambda Handler for EventBridge

```go
package main

import (
    "context"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/equaltoai/lesser/pkg/monitoring"
)

func healthCheckHandler(ctx context.Context, event monitoring.HealthCheckEvent) (*monitoring.HealthCheckResponse, error) {
    // Initialize monitor (this would be done in init() in practice)
    monitor := monitoring.NewServerlessHealthMonitor(db, logger)
    
    // Process the health check
    return monitor.ProcessHealthCheckEvent(ctx, event)
}

func main() {
    lambda.Start(healthCheckHandler)
}
```

### EventBridge Event Examples

#### Quick Health Check
```json
{
  "action": "check_health",
  "components": [
    {"type": "dynamodb", "identifier": "lesser-main", "name": "Main Database"}
  ],
  "options": {
    "store_results": false,
    "publish_metrics": true,
    "include_metadata": false,
    "timeout_seconds": 15,
    "retry_attempts": 1
  }
}
```

#### Comprehensive Health Check
```json
{
  "action": "check_health",
  "components": [
    {"type": "dynamodb", "identifier": "lesser-main", "name": "Main Database"},
    {"type": "lambda", "identifier": "lesser-api", "name": "API Handler"},
    {"type": "lambda", "identifier": "lesser-processor-timeline", "name": "Timeline Processor"},
    {"type": "sqs", "identifier": "timeline-updates", "name": "Timeline Updates Queue"},
    {"type": "sqs", "identifier": "federation-delivery", "name": "Federation Delivery Queue"}
  ],
  "options": {
    "store_results": true,
    "publish_metrics": true,
    "include_metadata": true,
    "timeout_seconds": 45,
    "retry_attempts": 3
  }
}
```

## Component Checkers

### DynamoDB Checker
- Tests table connectivity with minimal query
- Measures query latency
- Status based on response time (>2s = critical, >500ms = warning)

### Lambda Checker  
- Creates health check record in DynamoDB (proxy for Lambda health)
- Measures operation latency
- Status based on response time (>5s = critical, >1s = warning)

### SQS Checker
- Creates queue health record in DynamoDB (proxy for SQS health)
- Measures operation latency  
- Status based on response time (>3s = critical, >500ms = warning)

## Data Storage

Health check results are stored in DynamoDB using the following patterns:

### Health Check Results
- **PK**: `HEALTH_CHECK#{timestamp}`
- **SK**: `RESULT#{component_type}#{component}`
- **GSI1PK**: `COMPONENT#{component_type}#{component}`
- **GSI1SK**: `{timestamp}`
- **TTL**: 30 days

### Component Health History
- **PK**: `COMPONENT_HISTORY#{component_type}#{component}`
- **SK**: `HISTORY#{timestamp}`
- **TTL**: 7 days

### Health Check Summary (Hourly Aggregates)
- **PK**: `HEALTH_SUMMARY#{date}`
- **SK**: `SUMMARY#{hour}`
- **GSI1PK**: `DATE#{date}`
- **GSI1SK**: `HOUR#{hour}`
- **TTL**: 90 days

## Deployment with EventBridge

### EventBridge Rule (CDK Example)
```typescript
new events.Rule(this, 'HealthCheckRule', {
  schedule: events.Schedule.rate(cdk.Duration.minutes(5)),
  targets: [
    new targets.LambdaFunction(healthCheckLambda, {
      event: events.RuleTargetInput.fromObject({
        action: 'check_health',
        components: [
          { type: 'dynamodb', identifier: 'lesser-main', name: 'Main Database' }
        ],
        options: {
          store_results: true,
          publish_metrics: true,
          timeout_seconds: 30,
          retry_attempts: 2
        }
      })
    })
  ]
});
```

## Key Benefits

1. **No Polling**: Eliminates problematic ticker-based background processes
2. **Serverless-Native**: Works correctly with Lambda lifecycle (start/stop)
3. **Event-Driven**: Uses EventBridge for triggering health checks
4. **DynamORM Integration**: Follows project patterns, no direct AWS SDK usage
5. **Configurable**: Flexible event-based configuration
6. **Storage**: Optional result storage with automatic TTL cleanup
7. **Metrics**: Optional CloudWatch metrics publishing
8. **Retry Logic**: Built-in retry with exponential backoff
9. **Timeout Handling**: Per-check timeout configuration

## Migration from Legacy System

The legacy `StartHealthChecks()` method has been replaced. Instead of:

```go
// OLD - Don't use this
monitor.StartHealthChecks(ctx, 5*time.Minute, components)
```

Use EventBridge-triggered Lambda functions with the new monitor:

```go
// NEW - Use this pattern
response, err := monitor.ProcessHealthCheckEvent(ctx, event)
```

This ensures health checks work correctly in serverless environments without relying on persistent background processes.