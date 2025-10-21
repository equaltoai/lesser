# Event Processing Error Handling Patterns

This guide provides comprehensive error handling strategies for event-driven architectures built with Lift CDK constructs.

## Required Imports

```go
import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "net"
    "strconv"
    "sync"
    "time"

    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
    "github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/aws/smithy-go"
    "github.com/pay-theory/lift/pkg/cdk/constructs"
)
```

## Error Handling Principles

### The Error Handling Hierarchy

1. **Prevent**: Validate and sanitize inputs
2. **Detect**: Identify errors quickly and accurately
3. **Recover**: Implement retry and fallback strategies
4. **Isolate**: Prevent cascade failures
5. **Learn**: Log, monitor, and improve

## Error Types in Event Processing

### Transient Errors
- Network timeouts
- Temporary service unavailability
- Rate limiting
- Concurrent modification

### Permanent Errors
- Invalid message format
- Business rule violations
- Missing required data
- Authentication failures

### System Errors
- Out of memory
- Lambda timeout
- Permission denied
- Service limits exceeded

## Dead Letter Queue Patterns

### Basic DLQ Configuration

```go
// Configure DLQ for all event sources
sqsProcessor := constructs.NewSQSProcessor(stack, jsii.String("Processor"), &constructs.SQSProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("main-processor"),
    },
    EnableDeadLetterQueue: jsii.Bool(true),
    MaxReceiveCount:       jsii.Number(3), // Retry 3 times before DLQ
    DeadLetterQueueProps: &awssqs.QueueProps{
        RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)), // Keep for analysis
    },
})
```

### DLQ Processing Pattern

```go
// Dedicated DLQ processor for analysis and recovery
dlqProcessor := constructs.NewLiftFunction(stack, jsii.String("DLQProcessor"), &constructs.LiftFunctionProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("dlq-processor"),
        Environment: &map[string]*string{
            "SLACK_WEBHOOK": jsii.String("https://hooks.slack.com/..."),
            "ERROR_TABLE":   jsii.String("error-analysis"),
        },
    },
})

// Process DLQ messages
dlqProcessor.Function.AddEventSource(awslambda.NewSqsEventSource(sqsProcessor.DeadLetterQueue, &awslambda.SqsEventSourceProps{
    BatchSize: jsii.Number(1), // Process errors individually
}))
```

### DLQ Handler Implementation

```go
type DLQMessage struct {
    OriginalMessage json.RawMessage `json:"originalMessage"`
    ErrorMessage    string          `json:"errorMessage"`
    ErrorType       string          `json:"errorType"`
    Timestamp       time.Time       `json:"timestamp"`
    RetryCount      int             `json:"retryCount"`
    Source          string          `json:"source"`
}

func processDLQMessage(ctx context.Context, sqsEvent events.SQSEvent) error {
    for _, record := range sqsEvent.Records {
        var dlqMsg DLQMessage
        if err := json.Unmarshal([]byte(record.Body), &dlqMsg); err != nil {
            log.Printf("Failed to parse DLQ message: %v", err)
            continue
        }
        
        // Categorize error
        category := categorizeError(dlqMsg.ErrorType, dlqMsg.ErrorMessage)
        
        switch category {
        case "RETRIABLE":
            return reprocessMessage(dlqMsg)
        case "DATA_QUALITY":
            return sendToDataQualityQueue(dlqMsg)
        case "PERMANENT":
            return archiveAndAlert(dlqMsg)
        default:
            return investigateUnknownError(dlqMsg)
        }
    }
    
    return nil
}
```

## Retry Strategies

### Exponential Backoff

```go
type ExponentialBackoff struct {
    InitialInterval time.Duration
    MaxInterval     time.Duration
    Multiplier      float64
    MaxRetries      int
}

func (b *ExponentialBackoff) Retry(fn func() error) error {
    interval := b.InitialInterval
    
    for attempt := 0; attempt < b.MaxRetries; attempt++ {
        err := fn()
        if err == nil {
            return nil
        }
        
        // Check if error is retryable
        if !isRetryable(err) {
            return err
        }
        
        if attempt < b.MaxRetries-1 {
            time.Sleep(interval)
            interval = time.Duration(float64(interval) * b.Multiplier)
            if interval > b.MaxInterval {
                interval = b.MaxInterval
            }
        }
    }
    
    return fmt.Errorf("max retries exceeded")
}

func isRetryable(err error) bool {
    // Check for specific error types using AWS SDK v2
    var ae smithy.APIError
    if errors.As(err, &ae) {
        code := ae.ErrorCode()
        switch code {
        case "ThrottlingException", "TooManyRequestsException", "ProvisionedThroughputExceededException":
            return true
        case "ValidationException", "InvalidParameterException":
            return false
        }
    }
    
    // Check for network errors
    var netErr net.Error
    if errors.As(err, &netErr) && netErr.Timeout() {
        return true
    }
    
    return false
}
```

### Circuit Breaker Pattern

```go
type CircuitBreaker struct {
    maxFailures  int
    resetTimeout time.Duration
    
    mu           sync.Mutex
    failures     int
    lastFailTime time.Time
    state        string // "closed", "open", "half-open"
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    // Check if circuit should be reset
    if cb.state == "open" && time.Since(cb.lastFailTime) > cb.resetTimeout {
        cb.state = "half-open"
        cb.failures = 0
    }
    
    // If circuit is open, fail fast
    if cb.state == "open" {
        return fmt.Errorf("circuit breaker is open")
    }
    
    // Execute function
    err := fn()
    
    if err != nil {
        cb.failures++
        cb.lastFailTime = time.Now()
        
        if cb.failures >= cb.maxFailures {
            cb.state = "open"
            return fmt.Errorf("circuit breaker opened: %w", err)
        }
        
        return err
    }
    
    // Success - reset circuit
    cb.failures = 0
    cb.state = "closed"
    return nil
}
```

### Bulkhead Pattern

```go
type Bulkhead struct {
    semaphore chan struct{}
}

func NewBulkhead(maxConcurrent int) *Bulkhead {
    return &Bulkhead{
        semaphore: make(chan struct{}, maxConcurrent),
    }
}

func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
    select {
    case b.semaphore <- struct{}{}:
        defer func() { <-b.semaphore }()
        return fn()
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

## Using Lift's Built-in Error Handling Patterns

Lift provides several middleware patterns that are fully implemented and ready to use:

### Circuit Breaker Middleware

```go
import "github.com/pay-theory/lift/pkg/middleware"

// Create a circuit breaker with custom configuration
circuitBreakerConfig := middleware.NewBasicCircuitBreaker("my-service")
circuitBreakerConfig.FailureThreshold = 5
circuitBreakerConfig.Timeout = 60 * time.Second

// Apply to your handler
app.Use(middleware.CircuitBreakerMiddleware(circuitBreakerConfig))
```

### Bulkhead Middleware

```go
// Create a bulkhead to limit concurrent requests
bulkheadConfig := middleware.NewBasicBulkhead("my-service", 10)
bulkheadConfig.MaxConcurrentRequests = 10
bulkheadConfig.MaxWaitTime = 5 * time.Second

// Apply to your handler
app.Use(middleware.BulkheadMiddleware(bulkheadConfig))
```

### Retry Middleware

```go
// Create retry configuration
retryConfig := middleware.NewBasicRetry("my-service", 3)
retryConfig.InitialDelay = 100 * time.Millisecond
retryConfig.MaxDelay = 30 * time.Second
retryConfig.Strategy = middleware.RetryStrategyExponential

// Apply to your handler
app.Use(middleware.RetryMiddleware(retryConfig))
```

### Combined Error Handling

```go
// Combine multiple error handling patterns
app.Use(middleware.CircuitBreakerMiddleware(circuitBreakerConfig))
app.Use(middleware.BulkheadMiddleware(bulkheadConfig))
app.Use(middleware.RetryMiddleware(retryConfig))
```

## Event-Specific Error Handling

### SQS Error Handling

```go
func processSQSWithErrorHandling(ctx context.Context, event events.SQSEvent) error {
    var batchItemFailures []events.SQSBatchItemFailure
    
    for _, record := range event.Records {
        if err := processRecord(record); err != nil {
            // Check if error is retryable
            if isRetryable(err) {
                // Add to batch failures for retry
                batchItemFailures = append(batchItemFailures, 
                    events.SQSBatchItemFailure{
                        ItemIdentifier: record.MessageId,
                    })
            } else {
                // Log permanent error
                log.Printf("Permanent error for message %s: %v", 
                    record.MessageId, err)
                
                // Delete message to prevent infinite retry
                deleteMessage(record.ReceiptHandle)
            }
        }
    }
    
    // Return partial batch failure
    if len(batchItemFailures) > 0 {
        return events.NewSQSPartialBatchFailure(batchItemFailures)
    }
    
    return nil
}
```

### Kinesis Error Handling

```go
func processKinesisWithCheckpointing(ctx context.Context, event events.KinesisEvent) error {
    checkpoint := loadCheckpoint()
    
    for _, record := range event.Records {
        // Skip if already processed
        if record.Kinesis.SequenceNumber <= checkpoint {
            continue
        }
        
        // Process with timeout
        processCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
        err := processKinesisRecord(processCtx, record)
        cancel()
        
        if err != nil {
            // For Kinesis, we need to handle errors carefully
            if isPoisonRecord(err) {
                // Skip and checkpoint
                log.Printf("Skipping poison record: %s", 
                    record.Kinesis.SequenceNumber)
            } else {
                // Stop processing to maintain order
                return fmt.Errorf("failed at sequence %s: %w", 
                    record.Kinesis.SequenceNumber, err)
            }
        }
        
        // Update checkpoint
        saveCheckpoint(record.Kinesis.SequenceNumber)
    }
    
    return nil
}
```

### EventBridge Error Handling

```go
// Configure EventBridge with error handling
eventHandler, err := constructs.NewEventBridgeHandler(stack, jsii.String("Handler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("event-handler"),
    },
    EnableDeadLetterQueue: jsii.Bool(true),
    RetryAttempts:         jsii.Number(2),
    MaxEventAge:           awscdk.Duration_Hours(jsii.Number(1)),
    DeadLetterQueueProps:  &awssqs.QueueProps{},
})
if err != nil {
    panic(err)
}

// Handler with structured error responses
func processEventBridgeEvent(ctx context.Context, event events.CloudWatchEvent) error {
    span, ctx := opentracing.StartSpanFromContext(ctx, "processEvent")
    defer span.Finish()
    
    // Add event metadata to span
    span.SetTag("event.source", event.Source)
    span.SetTag("event.type", event.DetailType)
    span.SetTag("event.id", event.ID)
    
    // Process with detailed error capture
    if err := processWithContext(ctx, event); err != nil {
        // Categorize error for monitoring
        errorType := categorizeError(err)
        span.SetTag("error", true)
        span.SetTag("error.type", errorType)
        
        // Emit metrics
        emitErrorMetric(event.Source, event.DetailType, errorType)
        
        // Determine if retry is appropriate
        if shouldRetry(errorType) {
            return err // Let EventBridge retry
        }
        
        // For non-retryable errors, handle gracefully
        handlePermanentError(event, err)
        return nil // Don't retry
    }
    
    return nil
}
```

## Monitoring and Alerting

### Error Metrics

```go
type ErrorMetrics struct {
    namespace string
    client    *cloudwatch.Client
}

func (m *ErrorMetrics) RecordError(source, errorType string, count int) {
    m.client.PutMetricData(context.TODO(), &cloudwatch.PutMetricDataInput{
        Namespace: aws.String(m.namespace),
        MetricData: []types.MetricDatum{
            {
                MetricName: aws.String("ProcessingErrors"),
                Value:      aws.Float64(float64(count)),
                Dimensions: []types.Dimension{
                    {Name: aws.String("Source"), Value: aws.String(source)},
                    {Name: aws.String("ErrorType"), Value: aws.String(errorType)},
                },
                Timestamp: aws.Time(time.Now()),
            },
        },
    })
}

// Create alarms for error rates
alarm := cloudwatch.NewAlarm(stack, jsii.String("ErrorRateAlarm"), &cloudwatch.AlarmProps{
    Metric: cloudwatch.NewMetric(&cloudwatch.MetricProps{
        Namespace:  jsii.String("LiftApp"),
        MetricName: jsii.String("ProcessingErrors"),
        Statistic:  jsii.String("Sum"),
        Period:     awscdk.Duration_Minutes(jsii.Number(5)),
    }),
    Threshold:         jsii.Number(10),
    EvaluationPeriods: jsii.Number(2),
    TreatMissingData:  cloudwatch.TreatMissingData_NOT_BREACHING,
})
```

### Error Logging

```go
type StructuredErrorLogger struct {
    logger *slog.Logger
}

func (l *StructuredErrorLogger) LogError(err error, context map[string]interface{}) {
    // Extract error details
    errorDetails := map[string]interface{}{
        "error":      err.Error(),
        "type":       fmt.Sprintf("%T", err),
        "stacktrace": debug.Stack(),
    }
    
    // Add context
    for k, v := range context {
        errorDetails[k] = v
    }
    
    // Check for AWS errors using SDK v2
    var ae smithy.APIError
    if errors.As(err, &ae) {
        errorDetails["aws_error_code"] = ae.ErrorCode()
        errorDetails["aws_error_message"] = ae.ErrorMessage()
        errorDetails["aws_error_fault"] = ae.ErrorFault().String()
    }
    
    l.logger.Error("Processing error", errorDetails...)
}
```

## Testing Error Scenarios

### Error Scenario Testing

```go
func TestErrorHandling(t *testing.T) {
    scenarios := []struct {
        name     string
        error    error
        expected string
    }{
        {
            name:     "Transient Error",
            error:    &smithy.GenericAPIError{Code: "ThrottlingException"},
            expected: "retry",
        },
        {
            name:     "Permanent Error",
            error:    &smithy.GenericAPIError{Code: "ValidationException"},
            expected: "dlq",
        },
        {
            name:     "Timeout Error",
            error:    context.DeadlineExceeded,
            expected: "retry",
        },
    }
    
    for _, scenario := range scenarios {
        t.Run(scenario.name, func(t *testing.T) {
            result := handleError(scenario.error)
            assert.Equal(t, scenario.expected, result)
        })
    }
}
```

## Best Practices Summary

### 1. Design for Failure
- Assume failures will happen
- Implement graceful degradation
- Use circuit breakers to prevent cascades

### 2. Categorize Errors
- Distinguish transient from permanent
- Handle each category appropriately
- Monitor error patterns

### 3. Implement Proper Error Classification
- Distinguish between retryable and non-retryable errors
- Use appropriate error codes and messages
- Handle AWS SDK v2 error types correctly

### 4. Monitor and Alert
- Track error rates and types
- Set up intelligent alerting
- Use structured logging

### 5. Test Error Paths
- Unit test error handling logic
- Integration test failure scenarios
- Use proper AWS SDK v2 error types in tests

### 6. Document Error Handling
- Document error types and handling strategies
- Maintain runbooks for common errors
- Share learnings with team

## Common Anti-Patterns to Avoid

1. **Infinite Retry Loops**: Always have a maximum retry limit
2. **Swallowing Errors**: Log all errors, even if handled
3. **Generic Error Handling**: Be specific about error types
4. **Missing Context**: Include relevant context in error logs
5. **Cascade Failures**: Use circuit breakers and bulkheads
6. **Ignoring DLQ**: Always process and monitor DLQ messages
7. **Using Outdated SDK**: Always use AWS SDK v2 with smithy-go error types
8. **Incorrect CDK Properties**: Use the correct property names for Lift constructs