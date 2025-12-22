# Migration Guide: Event-Driven Lambdas to Lift

This guide explains how to migrate event-driven Lambda functions to use the Lift framework with our standardized patterns.

## DynamoDB Streams Migration

### Before (Traditional Lambda)
```go
func handler(ctx context.Context, event events.DynamoDBEvent) error {
    logger := common.Logger()
    for _, record := range event.Records {
        // Process record
    }
    return nil
}

func main() {
    lambda.Start(handler)
}
```

### After (Lift Pattern)
```go
type MyStreamProcessor struct {
    db     core.DB
    logger *zap.Logger
}

func (p *MyStreamProcessor) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
    for _, record := range event.Records {
        // Process record with access to Lift context
    }
    return nil
}

func main() {
    logger := common.Logger()
    processor := &MyStreamProcessor{
        db:     db,
        logger: logger,
    }

    app := lift.New()
    app.Use(lift.MarkGlobalMiddleware(liftMiddleware.RequestID()))
    app.Use(lift.MarkGlobalMiddleware(liftMiddleware.Recover()))

    _ = app.DynamoDB("*", func(ctx *lift.Context) error {
        records, err := ctx.DynamoDBRecords()
        if err != nil {
            return err
        }
        return processor.HandleStream(ctx, events.DynamoDBEvent{Records: records})
    })

    lambda.Start(app.HandleRequest)
}
```

## SQS Migration

### Before (Traditional Lambda)
```go
func handler(ctx context.Context, event events.SQSEvent) error {
    for _, msg := range event.Records {
        // Process message
    }
    return nil
}
```

### After (Lift Pattern)
```go
type MyQueueProcessor struct {
    service *MyService
    logger  *zap.Logger
}

func (p *MyQueueProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
    // Use the batch processor for partial failure support
    return patterns.ProcessSQSBatch(ctx, event, func(ctx *lift.Context, msg events.SQSMessage) error {
        // Process individual message
        return p.processMessage(ctx, msg)
    })
}

func main() {
    logger := common.Logger()
    processor := &MyQueueProcessor{
        service: service,
        logger:  logger,
    }
    patterns.StartSQSLambda("my-queue", processor, logger)
}
```

## EventBridge/CloudWatch Events Migration

### Before (Traditional Lambda)
```go
func handler(ctx context.Context, event events.CloudWatchEvent) error {
    // Process scheduled event
    return nil
}
```

### After (Lift Pattern - Scheduled)
```go
type MyScheduledTask struct {
    aggregator *MyAggregator
    logger     *zap.Logger
}

func (t *MyScheduledTask) HandleScheduledEvent(ctx *lift.Context) error {
    // Perform scheduled work with Lift context
    return t.aggregator.RunDaily(ctx.Request.Context())
}

func main() {
    logger := common.Logger()
    task := &MyScheduledTask{
        aggregator: aggregator,
        logger:     logger,
    }
    patterns.StartScheduledLambda("daily-task", task, logger)
}
```

## Key Benefits

1. **Standardized Middleware**: All event types get request ID, logging, and recovery middleware
2. **Lift Context**: Access to Lift's context features in event handlers
3. **Consistent Error Handling**: Lift's error types work across all event types
4. **Unified Logging**: Request tracking and structured logging
5. **Partial Batch Failure**: Built-in support for SQS partial batch failures

## Migration Steps

1. **Update imports**: Use `github.com/pay-theory/lift/pkg/lift` (and `github.com/pay-theory/lift/pkg/middleware` if you want request IDs/recovery).
2. **Create handler struct**: Keep your existing handler methods and call them from a Lift route.
3. **Move logic**: Transfer your handler logic to the new method.
4. **Update main**: Use `app.DynamoDB(...)` / `app.SQS(...)` / `app.EventBridge(...)` and `lambda.Start(app.HandleRequest)`.
5. **Test**: Ensure the migrated handler works correctly

## Error Handling

Use Lift's error types for consistency:

```go
// Instead of returning raw errors
if err != nil {
    return fmt.Errorf("failed to process: %w", err)
}

// Use Lift errors
if err != nil {
    return lift.NewLiftError("PROCESSING_FAILED", "failed to process record", 500).WithCause(err)
}
```

## Advanced Usage

For more control, create the app manually:

```go
func main() {
    logger := common.Logger()
    handler := &MyHandler{logger: logger}
    
    // Create app with custom configuration
    app := patterns.CreateDynamoDBStreamApp("my-processor", handler, logger)
    
    // Add custom middleware if needed
    app.Use(myCustomMiddleware)
    
    // Start the Lambda
    lambda.Start(app.HandleRequest)
}
```
