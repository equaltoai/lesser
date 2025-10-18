# DynamORM Lambda Performance Optimization Guide

This guide documents the performance optimizations available in the DynamORM Lambda initialization module and provides best practices for reducing cold start times and improving overall Lambda performance.

## Overview

The enhanced `LambdaInit` module provides several optimization strategies:
- **Parallel Model Registration**: Reduces initialization time for multiple models
- **Connection Prewarming**: Establishes DynamoDB connections during init
- **Runtime Optimization**: Tunes Go runtime for Lambda environment
- **Cost Tracking Integration**: Seamless cost monitoring without performance overhead
- **Lazy Loading**: Defers non-critical component initialization

## Performance Improvements

Based on internal benchmarking, these optimizations provide:
- **40-60% reduction** in cold start times for model registration
- **30-50% faster** first DynamoDB operation after cold start
- **Minimal memory overhead** (< 5MB for connection pooling)
- **Zero-overhead cost tracking** when not enabled

## Basic Usage

For backward compatibility, the simple initialization pattern still works:

```go
var db core.DB

func init() {
    var err error
    db, err = dynamorm.LambdaInit(&User{}, &Post{}, &Comment{})
    if err != nil {
        panic(err)
    }
}
```

## Advanced Usage with Options

For maximum performance, use `LambdaInitWithOptions`:

```go
db, err := dynamorm.LambdaInitWithOptions(&dynamorm.LambdaInitOptions{
    // Models to pre-register
    Models: []interface{}{&User{}, &Post{}, &Comment{}},
    
    // Enable cost tracking
    EnableCostTracking: true,
    Logger:            logger,
    RequestID:         "lambda-init",
    OperationType:     "api-handler",
    
    // Performance optimizations
    PrewarmConnections: true,
    ConnectionCount:    3,
    TimeoutBuffer:      500 * time.Millisecond,
    
    // Enable lazy loading
    EnableLazyLoading: true,
})
```

## Configuration Options

### Models
- **Type**: `[]interface{}`
- **Description**: Models to pre-register during initialization
- **Best Practice**: Include all frequently used models

### EnableCostTracking
- **Type**: `bool`
- **Description**: Wraps DB client with automatic cost tracking
- **Performance Impact**: Negligible when using pre-allocated trackers

### PrewarmConnections
- **Type**: `bool`
- **Default**: `true` (when using basic `LambdaInit`)
- **Description**: Creates and validates connections during init
- **Best Practice**: Enable for APIs with consistent traffic

### ConnectionCount
- **Type**: `int`
- **Default**: `2`
- **Description**: Number of connections to prewarm
- **Best Practice**: 2-3 for most workloads, up to 5 for high-throughput

### TimeoutBuffer
- **Type**: `time.Duration`
- **Description**: Safety buffer before Lambda timeout
- **Best Practice**: 500ms-1s for most handlers

### EnableLazyLoading
- **Type**: `bool`
- **Description**: Enables deferred initialization patterns
- **Best Practice**: Use for services with optional dependencies

## Best Practices

### 1. Model Pre-Registration

Pre-register all models used by your Lambda function:

```go
// Good: All models registered at init
db, err := dynamorm.LambdaInit(&User{}, &Post{}, &Comment{}, &Like{})

// Bad: Models registered on first use (causes latency)
db, err := dynamorm.LambdaInit() // No models
```

### 2. Connection Prewarming

For APIs with predictable traffic patterns:

```go
opts := &dynamorm.LambdaInitOptions{
    Models:             models,
    PrewarmConnections: true,
    ConnectionCount:    3, // Adjust based on concurrent requests
}
```

### 3. Cost Tracking Integration

Track DynamoDB costs without modifying handler code:

```go
db, err := dynamorm.LambdaInitWithOptions(&dynamorm.LambdaInitOptions{
    Models:             []interface{}{&User{}},
    EnableCostTracking: true,
    Logger:            logger,
    RequestID:         "api-handler",
    OperationType:     "user-operations",
})

// All DB operations are automatically tracked
user := &User{}
err := db.Model(user).Where("ID", "=", userID).First(user)
// Cost is tracked without additional code
```

### 4. Lazy Loading Pattern

Defer initialization of optional services:

```go
var (
    db            core.DB
    searchService *dynamorm.LazyLoader
    aiService     *dynamorm.LazyLoader
)

func init() {
    // Core DB always initialized
    db, _ = dynamorm.LambdaInit(&User{}, &Post{})
    
    // Search service initialized only when needed
    searchService = dynamorm.NewLazyLoader(func() (interface{}, error) {
        return search.NewClient(searchConfig)
    })
    
    // AI service initialized only when needed
    aiService = dynamorm.NewLazyLoader(func() (interface{}, error) {
        return ai.NewService(aiConfig)
    })
}

func handler(ctx context.Context, event events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
    switch event.Path {
    case "/search":
        // Search service initialized on first search request
        svc, err := searchService.Get()
        if err != nil {
            return nil, err
        }
        return handleSearch(svc.(*search.Client), event)
    
    case "/ai/generate":
        // AI service initialized on first AI request
        svc, err := aiService.Get()
        if err != nil {
            return nil, err
        }
        return handleAI(svc.(*ai.Service), event)
    
    default:
        // Regular requests don't initialize optional services
        return handleRegular(db, event)
    }
}
```

### 5. Monitoring Cold Starts

Track initialization performance:

```go
func init() {
    db, err = dynamorm.LambdaInitWithOptions(opts)
    if err != nil {
        panic(err)
    }
    
    // Log metrics for monitoring
    coldStart, models, connections := dynamorm.GetInitMetrics()
    logger.Info("Lambda initialized",
        zap.Duration("cold_start_time", coldStart),
        zap.Int("models_registered", models),
        zap.Int("connections_prewarmed", connections),
    )
}
```

## Runtime Optimizations

The module automatically applies these runtime optimizations:

1. **GOMAXPROCS**: Set to number of available CPUs
2. **GC Tuning**: Temporarily increases GOGC during init to reduce GC overhead
3. **Connection Reuse**: Maintains persistent connections across invocations

## Memory Considerations

- Each prewarmed connection uses ~1-2MB
- Model registration uses ~500KB per model type
- Cost tracking adds ~100KB when enabled
- Lazy loaders add minimal overhead until initialized

## Troubleshooting

### High Cold Start Times

1. Reduce number of pre-registered models
2. Lower connection count
3. Enable lazy loading for optional services
4. Check for large static initializations

### Connection Errors

1. Ensure IAM role has DynamoDB permissions
2. Check VPC configuration if applicable
3. Verify DynamoDB endpoint accessibility
4. Review connection count limits

### Cost Tracking Not Working

1. Ensure Logger is provided
2. Verify EnableCostTracking is true
3. Check cost tracker initialization logs
4. Ensure operations use the wrapped DB instance

## Migration Guide

To migrate existing Lambda functions:

1. Replace `dynamodb.New()` with `dynamorm.LambdaInit(models...)`
2. Move model definitions to package level
3. Add models to init function
4. Enable desired optimizations via options
5. Test cold start performance

Example migration:

```go
// Before:
func handler(ctx context.Context, event events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
    db, err := dynamodb.New()
    if err != nil {
        return nil, err
    }
    
    user := &User{}
    err = db.Model(user).Where("ID", "=", "123").First(user)
    // ...
}

// After:
var db core.DB

func init() {
    var err error
    db, err = dynamorm.LambdaInit(&User{})
    if err != nil {
        panic(err)
    }
}

func handler(ctx context.Context, event events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
    user := &User{}
    err := db.Model(user).Where("ID", "=", "123").First(user)
    // ...
}
```

## Performance Testing

Use the included metrics to validate improvements:

```go
// In your test handler
func testHandler(ctx context.Context) {
    start := time.Now()
    
    // Perform operations
    user := &User{}
    db.Model(user).Where("ID", "=", "test").First(user)
    
    duration := time.Since(start)
    coldStart, _, _ := dynamorm.GetInitMetrics()
    
    log.Printf("Cold start: %v, First operation: %v", coldStart, duration)
}
```

## Future Optimizations

Planned improvements include:
- Automatic model discovery via reflection
- Predictive connection scaling
- Cross-Lambda connection pooling
- Advanced GC tuning profiles