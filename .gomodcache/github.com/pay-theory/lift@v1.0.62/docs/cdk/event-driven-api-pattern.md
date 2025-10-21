# Event-Driven API Pattern

The EventDrivenAPI pattern combines API Gateway with EventBridge to create asynchronous APIs with request tracking and event-driven processing. This pattern is ideal for long-running operations and microservice architectures.

## Overview

The EventDrivenAPI pattern provides:
- Asynchronous request processing with EventBridge
- Request tracking and status management using DynamORM
- Built-in monitoring and tracing capabilities
- Multi-tenant support
- CORS and access logging configuration
- **Lift-optimized defaults**: ARM64 architecture, PROVIDED_AL2023 runtime, optimized memory/timeout
- **Automatic DLQ**: Dead letter queue for failed event processing
- **DynamORM integration**: Optional DynamORM environment configuration

## Basic Usage

```go
import (
    "github.com/pay-theory/lift/pkg/cdk/patterns"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
)

api := patterns.NewEventDrivenAPI(stack, jsii.String("MyAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("my-app"),
    ApiName: jsii.String("async-api"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EnableRequestTracking: jsii.Bool(true),
    EnableTracing: jsii.Bool(true),
})
```

## Lift-Optimized Defaults

The EventDrivenAPI pattern automatically applies Lift's performance optimizations:

### Lambda Function Defaults
- **Runtime**: `PROVIDED_AL2023` (latest Amazon Linux 2023)
- **Architecture**: `ARM64` (better price/performance ratio)
- **Memory**: `512MB` (balanced for most workloads)
- **Timeout**: `30 seconds` (reasonable default)
- **Tracing**: X-Ray tracing when enabled
- **Multi-tenant**: Automatic tenant isolation when enabled

### DynamORM Integration
When `EnableDynamORM` is set to `true`, the pattern automatically configures:
- AWS region detection
- Retry configuration (3 attempts, 100ms base delay)
- Debug mode support
- Table name environment variable

### CORS Configuration
Lift provides optimized CORS headers:
- **Allowed Headers**: `Content-Type`, `Authorization`, `X-Tenant-ID`, `X-Request-ID`, `X-Api-Key`
- **Exposed Headers**: `X-Request-ID`, `X-Rate-Limit-Limit`, `X-Rate-Limit-Remaining`, `X-Rate-Limit-Reset`
- **Methods**: `GET`, `POST`, `PUT`, `DELETE`, `OPTIONS`

## Configuration Options

### Core Properties

| Property | Type | Description | Default |
|----------|------|-------------|---------|
| AppName | *string | Application name (used for resource naming) | "event-driven-api" |
| ApiName | *string | Name of the API | Derived from AppName |
| Description | *string | API description | "Event-driven API with async processing" |
| FunctionProps | awslambda.FunctionProps | Lambda function configuration | Required |
| MemorySize | *float64 | Lambda memory size in MB | 512 |
| Timeout | *float64 | Lambda timeout in seconds | 30 |
| Environment | *map[string]*string | Additional environment variables | nil |

### API Configuration

| Property | Type | Description |
|----------|------|-------------|
| EnableCORS | *bool | Enable CORS support |
| AllowOrigins | *[]*string | CORS allowed origins (defaults to ["*"]) |
| EnableAccessLogging | *bool | Enable API Gateway access logging |
| ThrottleRateLimit | *float64 | API throttling rate limit |
| ThrottleBurstLimit | *float64 | API throttling burst limit |
| DomainName | *string | Custom domain name for the API |
| CertificateArn | *string | Certificate ARN for custom domain |
| StageName | *string | API Gateway stage name |

### Lambda Function Configuration

| Property | Type | Description |
|----------|------|-------------|
| FunctionProps | awslambda.FunctionProps | Lambda function configuration |
| MemorySize | *float64 | Lambda memory size in MB | 512 |
| Timeout | *float64 | Lambda timeout in seconds | 30 |
| Environment | *map[string]*string | Additional environment variables |
| ReservedConcurrentExecutions | *float64 | Limit concurrent executions |
| EnableDynamORM | *bool | Enable DynamORM environment variables |
| DynamORMTableName | *string | DynamORM table name |
| DynamORMDebug | *bool | Enable DynamORM debug mode |

### EventBridge Configuration

| Property | Type | Description |
|----------|------|-------------|
| EventBusName | *string | EventBridge bus name | "default" |
| EventSource | *string | Event source identifier | Derived from AppName |
| DetailType | *string | Event detail type | "APIRequest" |

### Request Tracking Configuration

| Property | Type | Description |
|----------|------|-------------|
| RequestTrackingTableProps | *RequestTrackingTableProps | Request tracking table configuration |
| EnableRequestTracking | *bool | Enable request status tracking | true |
| RequestRetentionDays | *float64 | Request retention period in days |

### Lift-Specific Settings

| Property | Type | Description |
|----------|------|-------------|
| EnableTracing | *bool | Enable X-Ray tracing |
| EnableMultiTenant | *bool | Enable multi-tenant support |
| EnableMonitoring | *bool | Enable CloudWatch monitoring |
| EnableMetrics | *bool | Enable CloudWatch metrics |
| EnableDynamORM | *bool | Enable DynamORM environment variables |
| DynamORMTableName | *string | DynamORM table name |
| DynamORMDebug | *bool | Enable DynamORM debug mode |

### Dead Letter Queue Configuration

The EventDrivenAPI pattern automatically creates a dead letter queue for failed event processing through its underlying EventBridgeHandler. DLQ is **enabled by default** with the following configuration:

- **Retry Attempts**: 3 (configurable via EventBridgeHandler)
- **Max Event Age**: 1 hour (configurable via EventBridgeHandler)
- **DLQ Retention**: 14 days
- **Environment Variable**: `EVENTBRIDGE_DLQ_URL` is automatically set

To customize DLQ settings, you can access the underlying EventBridgeHandler:

```go
api := patterns.NewEventDrivenAPI(stack, jsii.String("MyAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("my-app"),
    ApiName: jsii.String("my-api"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
})

// Access the underlying EventBridgeHandler for DLQ configuration
if api.EventHandler != nil {
    // DLQ is automatically created and configured
    dlqUrl := api.EventHandler.DeadLetterQueue.QueueUrl()
    // You can grant additional permissions or configure monitoring
}
```

### Custom Domain Configuration

The EventDrivenAPI pattern supports custom domains through the underlying LiftAPI:

```go
api := patterns.NewEventDrivenAPI(stack, jsii.String("MyAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("my-app"),
    ApiName: jsii.String("my-api"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
    DomainName: jsii.String("api.example.com"),
    CertificateArn: jsii.String("arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012"),
})

// Access the underlying API for additional domain configuration
if api.API != nil {
    // The LiftAPI construct handles custom domain setup automatically
    // when DomainName and CertificateArn are provided
}
```

## Request Tracking

Enable request tracking to monitor async operation status:

```go
api := patterns.NewEventDrivenAPI(stack, jsii.String("MyAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("my-app"),
    ApiName: jsii.String("async-api"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EnableRequestTracking: jsii.Bool(true),
    RequestTrackingTableProps: &liftconstructs.RequestTrackingTableProps{
        TableName: jsii.String("custom-requests"),
    },
})

// The tracking table uses DynamORM pattern with:
// - Primary key: PK (request#{request_id})
// - Sort key: SK (request#{request_id})
// - GSIs defined in DynamORM models
// - TTL attribute for automatic cleanup
```

### Request Tracking Schema

The request tracking table uses DynamORM with the following pattern:

```go
// Example DynamORM model for request tracking:
type RequestTracking struct {
    PK         string    `dynamorm:"pk"`                             // request#{request_id}
    SK         string    `dynamorm:"sk"`                             // request#{request_id}

    // Indexes for queries
    CorrelationID string `dynamorm:"index:correlation-index,pk"`    // correlation_id
    Timestamp     string `dynamorm:"index:correlation-index,sk"`    // ISO timestamp
    Status        string `dynamorm:"index:status-index,pk"`         // status
    UserID        string `dynamorm:"index:user-index,pk"`           // user_id
    Date          string `dynamorm:"index:timestamp-index,pk"`      // YYYY-MM-DD

    // Request data
    RequestID     string `json:"request_id"`
    TTL           int64  `json:"ttl"`
}
```

## Environment Variables

The pattern automatically sets environment variables:

### Core Variables
- `EVENT_BUS_NAME`: EventBridge bus name
- `EVENT_SOURCE`: Event source identifier
- `EVENT_DETAIL_TYPE`: Event detail type

### Request Tracking Variables (if enabled)
- `REQUEST_TRACKING_TABLE`: DynamoDB table name
- `REQUEST_TRACKING_TABLE_ARN`: DynamoDB table ARN

### Dead Letter Queue Variables (automatically set)
- `EVENTBRIDGE_DLQ_URL`: Dead letter queue URL for failed events

### Lift-Specific Variables (automatically set)
- `LIFT_VERSION`: Lift library version
- `LIFT_MULTI_TENANT`: "true" if multi-tenant is enabled
- `LIFT_METRICS_ENABLED`: "true" if metrics are enabled

### DynamORM Variables (if enabled)
- `DYNAMORM_REGION`: AWS region
- `DYNAMODB_TABLE_NAME`: DynamoDB table name
- `DYNAMORM_DEBUG`: DynamORM debug mode
- `DYNAMORM_RETRY_MAX_ATTEMPTS`: Retry attempts (default: 3)
- `DYNAMORM_RETRY_BASE_DELAY`: Base retry delay (default: 100ms)

## Helper Methods

The pattern provides helper methods:

```go
// Get API endpoint URL
endpoint := api.GetAPIEndpoint()

// Get request tracking table name
tableName := api.GetRequestTrackingTableName()

// Grant permissions to request tracking table
api.GrantRequestTrackingAccess(myFunction)

// Add custom routes to the API
api.AddAPIRoute(jsii.String("/custom"), "POST", myFunction)

// Access dead letter queue (if EventHandler exists)
if api.EventHandler != nil && api.EventHandler.DeadLetterQueue != nil {
    dlqUrl := api.EventHandler.DeadLetterQueue.QueueUrl()
    // Grant permissions to DLQ
    api.EventHandler.DeadLetterQueue.GrantSendMessages(myFunction)
}
```

## Common Patterns

### Long-Running Operations

```go
// API endpoint initiates processing
func handleRequest(ctx *lift.Context) error {
    requestId := uuid.New().String()
    
    // Store initial request status
    err := trackRequest(requestId, "pending", ctx.UserID())
    if err != nil {
        return err
    }
    
    // Send event for processing
    event := map[string]interface{}{
        "requestId": requestId,
        "userId":    ctx.UserID(),
        "data":      ctx.Request.Body,
    }
    
    err = sendEvent("ProcessRequest", event)
    if err != nil {
        return err
    }
    
    return ctx.JSON(map[string]string{
        "requestId": requestId,
        "status":    "accepted",
    })
}

// Event processor handles the work
func processEvent(event events.EventBridgeEvent) error {
    var request ProcessRequest
    json.Unmarshal(event.Detail, &request)
    
    // Update status
    updateRequestStatus(request.RequestId, "processing")
    
    // Do the work
    result, err := performLongOperation(request)
    
    if err != nil {
        updateRequestStatus(request.RequestId, "failed", err)
        return err
    }
    
    // Update completion
    updateRequestStatus(request.RequestId, "completed", result)
    
    return nil
}
```

### Status Polling

```go
// Status endpoint for polling
func getRequestStatus(ctx *lift.Context) error {
    requestId := ctx.Param("requestId")
    
    status, err := queryRequestStatus(requestId)
    if err != nil {
        return lift.NotFound("Request not found")
    }
    
    return ctx.JSON(status)
}

// Batch status check
func getBatchStatus(ctx *lift.Context) error {
    var requestIds []string
    ctx.ParseRequest(&requestIds)
    
    statuses, err := queryBatchStatus(requestIds)
    if err != nil {
        return err
    }
    
    return ctx.JSON(statuses)
}
```

## Advanced Patterns

### Custom Event Sources

```go
api := patterns.NewEventDrivenAPI(stack, jsii.String("CustomAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("custom-app"),
    ApiName: jsii.String("custom-api"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EnableRequestTracking:       jsii.Bool(true),
    EventSource: jsii.String("custom.service"),
    DetailType: jsii.String("CustomEvent"),
```

### Multi-Region Deployment

```go
// Primary region
primaryAPI := patterns.NewEventDrivenAPI(primaryStack, jsii.String("PrimaryAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("global-api"),
    ApiName: jsii.String("global-api"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EnableRequestTracking: jsii.Bool(true),
})

// Secondary region
secondaryAPI := patterns.NewEventDrivenAPI(secondaryStack, jsii.String("SecondaryAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("global-api"),
    ApiName: jsii.String("global-api"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EventBusName: jsii.String("default"), // Use default bus for cross-region
})
```

## Best Practices

### 1. Request Design
- Use UUIDs for request IDs
- Include correlation IDs for tracing
- Set appropriate TTLs for tracking records
- Implement idempotency for retries

### 2. Event Processing
- Keep event payloads small (<256KB)
- Use S3 for large payloads with presigned URLs
- Implement proper error handling and retries
- Monitor CloudWatch logs for failed events
- **Monitor DLQ for failed events** - check `EVENTBRIDGE_DLQ_URL` queue regularly

### 3. Performance
- Configure appropriate Lambda memory/timeout
- Implement circuit breakers for external calls
- Cache frequently accessed data
- Use DynamORM for efficient data access

### 4. Security
- Validate all incoming requests
- Use API keys or authentication
- Encrypt sensitive data in events
- Implement proper IAM permissions

## Monitoring

Monitor your event-driven API:

```go
// Enable monitoring when creating the API
api := patterns.NewEventDrivenAPI(stack, jsii.String("MyAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("my-app"),
    ApiName: jsii.String("my-api"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EnableMonitoring: jsii.Bool(true),
    EnableTracing: jsii.Bool(true),
})

// Access API endpoint for monitoring
endpoint := api.GetAPIEndpoint()

// Access request tracking table for monitoring
tableName := api.GetRequestTrackingTableName()

// Access dead letter queue for monitoring
if api.EventHandler != nil && api.EventHandler.DeadLetterQueue != nil {
    dlqUrl := api.EventHandler.DeadLetterQueue.QueueUrl()
    // Monitor DLQ for failed events
    // Set up CloudWatch alarms on DLQ message count
}
```

## Complete Example

```go
// E-commerce order processing API
orderAPI := patterns.NewEventDrivenAPI(stack, jsii.String("OrderAPI"), &patterns.EventDrivenAPIProps{
    AppName: jsii.String("order-api"),
    ApiName: jsii.String("order-api"),
    Description: jsii.String("E-commerce order processing API"),
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./dist/order-api"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EnableRequestTracking: jsii.Bool(true),
    EnableTracing: jsii.Bool(true),
    EnableMultiTenant: jsii.Bool(true),
    EnableMonitoring: jsii.Bool(true),
    EnableCORS: jsii.Bool(true),
    EnableAccessLogging: jsii.Bool(true),
    ThrottleRateLimit: jsii.Number(100),
    ThrottleBurstLimit: jsii.Number(200),
    EventSource: jsii.String("order.api"),
    DetailType: jsii.String("Order Processing"),
    
    Environment: &map[string]*string{
        "PAYMENT_API_URL":    jsii.String("https://payment.api.com"),
        "INVENTORY_API_URL":  jsii.String("https://inventory.api.com"),
        "NOTIFICATION_EMAIL": jsii.String("orders@example.com"),
    },
})

// Grant permissions to external services
paymentService := lambda.Function_FromFunctionArn(stack, jsii.String("PaymentService"), paymentArn)
orderAPI.GrantRequestTrackingAccess(paymentService)
```

## Troubleshooting

### Events Not Processing
1. Check EventBridge rule is active
2. Verify Lambda has permissions
3. Check event pattern matches
4. Review CloudWatch logs

### Request Tracking Issues
1. Verify DynamoDB table exists
2. Check TTL configuration
3. Monitor table throttling
4. Review GSI usage

### API Issues
1. Check API Gateway logs
2. Verify Lambda function configuration
3. Review CORS settings
4. Check throttling limits

### Dead Letter Queue Issues
1. Check DLQ for failed events (`EVENTBRIDGE_DLQ_URL`)
2. Review retry configuration in EventBridgeHandler
3. Monitor DLQ message count and age
4. Investigate root cause of event processing failures
