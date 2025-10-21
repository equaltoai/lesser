# Lift Observability Demo

This example demonstrates the comprehensive observability features of the Lift framework, including:

- **Zap Integration**: High-performance structured logging
- **CloudWatch Logs**: AWS-native log aggregation with batching
- **Multi-tenant Support**: Tenant isolation in logging
- **Performance Monitoring**: Sub-millisecond logging overhead
- **Comprehensive Testing**: Mock implementations for unit testing

## Features Demonstrated

### 1. Zap Logger (Development)
- Console-friendly output with colors
- Structured logging with fields
- Context propagation (request ID, tenant ID, user ID)
- Performance metrics and health checking

### 2. CloudWatch Logger (Production)
- Batched log shipping to AWS CloudWatch Logs
- Automatic log group and stream creation
- Configurable flush intervals and batch sizes
- Error handling and retry logic
- Performance monitoring with <1ms overhead

### 3. Multi-tenant Logging
- Tenant-specific context in all log entries
- Isolated logging per tenant
- Trace ID propagation for distributed tracing
- User context preservation

### 4. Testing Support
- Comprehensive mock implementations
- Call tracking and verification
- Error simulation for testing failure scenarios
- In-memory log capture for assertions

## Running the Demo

```bash
# Run the complete demo
go run examples/observability-demo/main.go

# Run specific tests
go test ./pkg/observability/cloudwatch -v
go test ./pkg/observability/zap -v
```

## Configuration Examples

### Development Configuration (Zap)
```go
config := observability.LoggerConfig{
    Level:  "debug",
    Format: "console", // Pretty console output
}

factory := zap.NewZapLoggerFactory()
logger, err := factory.CreateConsoleLogger(config)
```

### Production Configuration (CloudWatch)
```go
config := observability.LoggerConfig{
    LogGroup:      "/aws/lambda/my-service",
    LogStream:     "production-stream",
    BatchSize:     25,
    FlushInterval: 5 * time.Second,
    BufferSize:    100,
    Level:         "info",
    Format:        "json",
}

client := cloudwatch.NewCloudWatchLogsClient(awsConfig)
logger, err := cloudwatch.NewCloudWatchLogger(config, client)
```

### Testing Configuration (Mock)
```go
mockClient := cloudwatch.NewMockCloudWatchLogsClient()
config := observability.LoggerConfig{
    LogGroup:      "test-log-group",
    LogStream:     "test-stream",
    BatchSize:     5,
    FlushInterval: 100 * time.Millisecond,
}

logger, err := cloudwatch.NewCloudWatchLogger(config, mockClient)
```

## Multi-tenant Usage

```go
// Base logger
logger := createLogger()

// Tenant-specific logger
tenantLogger := logger.
    WithTenantID("tenant-123").
    WithUserID("user-456").
    WithRequestID("req-789").
    WithTraceID("trace-abc")

// All subsequent logs will include tenant context
tenantLogger.Info("Processing payment", map[string]interface{}{
    "amount":   1000,
    "currency": "USD",
})
```

## Performance Characteristics

- **Zap Logger**: <0.1ms per log entry
- **CloudWatch Logger**: <1ms overhead (including batching)
- **Memory Usage**: <1MB buffer per Lambda instance
- **Throughput**: >10,000 log entries per second

## AWS Permissions Required

For production CloudWatch logging, ensure your Lambda execution role has:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "logs:CreateLogGroup",
                "logs:CreateLogStream",
                "logs:PutLogEvents"
            ],
            "Resource": "arn:aws:logs:*:*:*"
        }
    ]
}
```

## Integration with Lift Middleware

```go
// Add observability to your Lift handlers
func createHandler() lift.Handler {
    return lift.Chain(
        middleware.RequestID(),
        middleware.Logger(),
        middleware.Metrics(),
        // Your business logic
    )(myBusinessHandler)
}
```

## Monitoring and Alerting

The logger provides health checks and statistics that can be used for:

- **Health Endpoints**: `logger.IsHealthy()`
- **Metrics Collection**: `logger.GetStats()`
- **Error Rate Monitoring**: Track error count vs total entries
- **Performance Monitoring**: Average flush times and buffer utilization

## Best Practices

1. **Use appropriate log levels**: Debug for development, Info for production
2. **Include context**: Always add tenant, user, and request IDs
3. **Batch efficiently**: Configure batch sizes based on your traffic patterns
4. **Monitor performance**: Keep logging overhead under 1ms per request
5. **Test thoroughly**: Use mocks to verify logging behavior in tests
6. **Handle failures gracefully**: Logger should never crash your application 