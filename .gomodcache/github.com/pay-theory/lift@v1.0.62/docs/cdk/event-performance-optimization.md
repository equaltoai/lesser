# Event Processing Performance Optimization Guide

This guide provides comprehensive strategies for optimizing the performance of event-driven applications built with Lift CDK constructs.

## Performance Principles

### The Four Pillars of Event Processing Performance

1. **Throughput**: Maximum events processed per second
2. **Latency**: Time from event creation to processing completion
3. **Concurrency**: Parallel processing capability
4. **Efficiency**: Resource utilization and cost per event

## Lambda Optimization

### Memory Configuration

Memory directly impacts CPU allocation and network bandwidth:

```go
// Performance testing shows optimal memory for event processing
functionProps := &constructs.LiftFunctionProps{
    MemorySize: jsii.Number(1769), // 1 vCPU allocation
    Timeout:    awscdk.Duration_Minutes(jsii.Number(5)),
}
```

**Memory Guidelines:**
- 128-512 MB: Simple transformations, lightweight processing (Lift default: 512MB)
- 512-1769 MB: JSON parsing, API calls, moderate computation
- 1769-3008 MB: Heavy computation, large payloads, multiple API calls
- 3008-10240 MB: Memory-intensive operations, ML inference

*Note: Lift functions default to 512MB memory. The guidelines above are optimization recommendations based on workload requirements.*

### CPU Optimization

```go
// Use ARM64 for better price/performance
functionProps := &constructs.LiftFunctionProps{
    Architecture: awslambda.Architecture_ARM_64(),
    Runtime:      awslambda.Runtime_PROVIDED_AL2023(), // Faster cold starts
}
```

### Cold Start Mitigation

1. **Provisioned Concurrency**
```go
function.AddAlias(jsii.String("live"), &awslambda.AliasOptions{
    ProvisionedConcurrentExecutions: jsii.Number(5),
})
```

2. **SnapStart** (Java only)
```java
// Note: SnapStart is only available for Java Lambda functions
// This optimization is not applicable to Go functions
functionProps := FunctionProps.builder()
    .snapStart(SnapStartConf.ON_PUBLISHED_VERSIONS)
    .build();
```

3. **Lambda Extensions**
```go
// Cache connections and config using Lift constructs
cacheLayer := awslambda.NewLayerVersion(stack, jsii.String("CacheLayer"), &awslambda.LayerVersionProps{
    Code: awslambda.Code_FromAsset(jsii.String("./cache-extension")),
})

// Use with Lift function
function := constructs.NewLiftFunction(stack, jsii.String("OptimizedFunction"), &constructs.LiftFunctionProps{
    FunctionProps: awslambda.FunctionProps{
        Layers: &[]awslambda.ILayerVersion{cacheLayer},
    },
})
```

## Event Source Optimization

### SQS Optimization

```go
sqsProcessor := constructs.NewSQSProcessor(stack, jsii.String("OptimizedQueue"), &constructs.SQSProcessorProps{
    QueueName: jsii.String("high-throughput-queue"),
    EventSourceProps: &awslambda.SqsEventSourceProps{
        BatchSize:                  jsii.Number(25),      // Maximize batch
        MaxBatchingWindowInSeconds: jsii.Number(20),     // Balance latency
        MaxConcurrency:            jsii.Number(1000),    // Scale out
        ReportBatchItemFailures:   jsii.Bool(true),     // Partial failures
    },
    QueueProps: &awssqs.QueueProps{
        ReceiveMessageWaitTimeSeconds: jsii.Number(20), // Long polling
        VisibilityTimeout:            awscdk.Duration_Minutes(jsii.Number(6)),
    },
})
```

**Optimization Strategies:**
- Batch Size: 1-10 for low latency, 25+ for throughput
- Batching Window: 0-20 seconds based on traffic patterns
- Concurrency: 2-1000 based on downstream capacity

### Kinesis Optimization

```go
kinesisProcessor := constructs.NewKinesisProcessor(stack, jsii.String("OptimizedStream"), &constructs.KinesisProcessorProps{
    StreamName: jsii.String("high-velocity-stream"),
    StreamMode: awskinesis.StreamMode_ON_DEMAND,
    EventSourceProps: &awslambda.KinesisEventSourceProps{
        BatchSize:                    jsii.Number(100),    // Max for Lambda
        StartingPosition:             awslambda.StartingPosition_LATEST,
        MaxBatchingWindowInSeconds:   jsii.Number(5),     // Low latency
        ParallelizationFactor:        jsii.Number(10),    // 10x parallelism
        BisectBatchOnError:          jsii.Bool(true),     // Error isolation
        ReportBatchItemFailures:     jsii.Bool(true),     // Checkpoint per record
        TumblingWindow:              awscdk.Duration_Seconds(jsii.Number(60)),
    },
})
```

**Enhanced Fan-Out for Multiple Consumers:**
```go
// Enable for dedicated throughput per consumer
kinesisProcessor.Stream.GrantRead(consumerFunction)
consumerFunction.AddEnvironment("KINESIS_ENHANCED_FANOUT", "true")
```

### EventBridge Optimization

```go
eventHandler := constructs.NewEventBridgeHandler(stack, jsii.String("OptimizedEvents"), &constructs.EventBridgeHandlerProps{
    EventPattern: &awseventbridge.EventPattern{
        Source:     &[]*string{jsii.String("order.service")},          // Specific source
        DetailType: &[]*string{jsii.String("Order Placed")},          // Specific type
        Account:    &[]*string{jsii.String("123456789012")},          // Reduce scope
        Region:     &[]*string{jsii.String("us-east-1")},             // Single region
        Detail: &map[string]interface{}{
            "customerId": &map[string]interface{}{
                "exists": true,                                        // Required field
            },
            "orderAmount": &map[string]interface{}{
                "numeric": &[]interface{}{">", 100},                   // High-value only
            },
        },
    },
    RetryPolicy: &awseventbridge.RetryPolicy{
        MaximumRetryAttempts: jsii.Number(2),                         // Fail fast
        MaximumEventAge:      awscdk.Duration_Minutes(jsii.Number(5)), // Short timeout
    },
})
```

### DynamoDB Streams Optimization

```go
dynamoProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("OptimizedStreams"), &constructs.DynamoStreamProcessorProps{
    TableName: jsii.String("high-write-table"),
    StreamViewType: awsdynamodb.StreamViewType_KEYS_ONLY, // Minimize payload
    EventSourceProps: &awslambda.DynamoEventSourceProps{
        StartingPosition:          awslambda.StartingPosition_LATEST,
        BatchSize:                jsii.Number(100),        // Maximum
        MaxBatchingWindowInSeconds: jsii.Number(5),       // Balance
        ParallelizationFactor:     jsii.Number(10),       // Scale out
        BisectBatchOnError:       jsii.Bool(true),        // Error handling
        MaximumRecordAgeInSeconds: jsii.Number(3600),     // 1 hour
    },
})
```

## Batch Processing Optimization

### Efficient Batch Handling

```go
func processEventBatch(events []Event) error {
    // Pre-allocate slices
    results := make([]Result, 0, len(events))
    errors := make([]error, 0)
    
    // Process in parallel with worker pool
    workerCount := runtime.NumCPU()
    jobs := make(chan Event, len(events))
    results := make(chan Result, len(events))
    
    // Start workers
    var wg sync.WaitGroup
    for w := 0; w < workerCount; w++ {
        wg.Add(1)
        go worker(jobs, results, &wg)
    }
    
    // Send jobs
    for _, event := range events {
        jobs <- event
    }
    close(jobs)
    
    // Wait for completion
    wg.Wait()
    close(results)
    
    // Batch write results
    return batchWriteResults(results)
}
```

### Connection Pooling

```go
var (
    httpClient *http.Client
    dbPool     *sql.DB
    once       sync.Once
)

func init() {
    once.Do(func() {
        // HTTP client with connection pooling
        httpClient = &http.Client{
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
            Timeout: 30 * time.Second,
        }
        
        // Database connection pool
        var err error
        dbPool, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
        if err == nil {
            dbPool.SetMaxOpenConns(25)
            dbPool.SetMaxIdleConns(10)
            dbPool.SetConnMaxLifetime(5 * time.Minute)
        }
    })
}
```

## Caching Strategies

### In-Memory Caching

```go
type Cache struct {
    mu    sync.RWMutex
    items map[string]CacheItem
}

type CacheItem struct {
    Value      interface{}
    Expiration time.Time
}

var cache = &Cache{
    items: make(map[string]CacheItem),
}

func (c *Cache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    item, found := c.items[key]
    if !found || time.Now().After(item.Expiration) {
        return nil, false
    }
    return item.Value, true
}
```

### Lambda Layer Caching

```go
// Create layer for shared cache
cacheLayer := awslambda.NewLayerVersion(stack, jsii.String("CacheLayer"), &awslambda.LayerVersionProps{
    Code:        awslambda.Code_FromAsset(jsii.String("./layers/cache")),
    Description: jsii.String("Shared cache layer"),
})

// Use in function
function.AddLayers(cacheLayer)
```

### ElastiCache Integration

```go
// For distributed caching
import "github.com/go-redis/redis/v8"

var redisClient *redis.Client

func init() {
    redisClient = redis.NewClient(&redis.Options{
        Addr:         os.Getenv("REDIS_ENDPOINT"),
        PoolSize:     10,
        MinIdleConns: 5,
        MaxRetries:   3,
    })
}
```

## Concurrency Patterns

### Worker Pool Pattern

```go
func processWithWorkerPool(events []Event, workerCount int) {
    eventChan := make(chan Event, len(events))
    var wg sync.WaitGroup
    
    // Start workers
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for event := range eventChan {
                processEvent(event)
            }
        }()
    }
    
    // Send work
    for _, event := range events {
        eventChan <- event
    }
    close(eventChan)
    
    wg.Wait()
}
```

### Semaphore Pattern

```go
func processWithSemaphore(events []Event, maxConcurrent int) {
    sem := make(chan struct{}, maxConcurrent)
    var wg sync.WaitGroup
    
    for _, event := range events {
        wg.Add(1)
        sem <- struct{}{} // Acquire
        
        go func(e Event) {
            defer func() {
                <-sem      // Release
                wg.Done()
            }()
            processEvent(e)
        }(event)
    }
    
    wg.Wait()
}
```

## DynamORM Performance Optimization

### Single Table Design

DynamORM's single table design provides significant performance benefits for event processing:

```go
// Optimize DynamORM table for event processing
table := constructs.NewLiftTable(stack, jsii.String("EventTable"), &constructs.LiftTableProps{
    TableName: jsii.String("events"),
    PartitionKey: &constructs.Attribute{
        Name: jsii.String("PK"),
        Type: awsdynamodb.AttributeType_STRING,
    },
    SortKey: &constructs.Attribute{
        Name: jsii.String("SK"),
        Type: awsdynamodb.AttributeType_STRING,
    },
    // Performance optimizations
    BillingMode: awsdynamodb.BillingMode_PAY_PER_REQUEST,
    PointInTimeRecovery: jsii.Bool(true),
    StreamSpecification: &constructs.StreamSpecification{
        StreamViewType: awsdynamodb.StreamViewType_KEYS_ONLY, // Minimize stream payload
    },
})
```

### Efficient Query Patterns

```go
// Use GSI for efficient event filtering
table.AddGlobalSecondaryIndex(&constructs.GlobalSecondaryIndexProps{
    IndexName: jsii.String("EventTypeIndex"),
    PartitionKey: &constructs.Attribute{
        Name: jsii.String("EventType"),
        Type: awsdynamodb.AttributeType_STRING,
    },
    SortKey: &constructs.Attribute{
        Name: jsii.String("Timestamp"),
        Type: awsdynamodb.AttributeType_NUMBER,
    },
    ProjectionType: awsdynamodb.ProjectionType_INCLUDE,
    NonKeyAttributes: &[]*string{
        jsii.String("EventId"),
        jsii.String("TenantId"),
        jsii.String("Status"),
    },
})

// Batch operations for high throughput
func processEventBatch(events []Event) error {
    // Use DynamORM batch operations
    batchWriter := dynamorm.NewBatchWriter(table)
    
    for _, event := range events {
        batchWriter.PutItem(event)
    }
    
    return batchWriter.Execute()
}
```

### Connection Pooling with DynamORM

```go
// Configure DynamORM for optimal performance
func init() {
    dynamorm.Configure(&dynamorm.Config{
        MaxRetries:        3,
        RetryDelay:        100 * time.Millisecond,
        MaxRetryDelay:     5 * time.Second,
        ConnectionTimeout: 30 * time.Second,
        RequestTimeout:    30 * time.Second,
        // Enable connection pooling
        MaxConnections:    100,
        IdleTimeout:       5 * time.Minute,
    })
}
```

## Data Optimization

### Compression

```go
import (
    "compress/gzip"
    "encoding/base64"
)

func compressPayload(data []byte) (string, error) {
    var buf bytes.Buffer
    gz := gzip.NewWriter(&buf)
    
    if _, err := gz.Write(data); err != nil {
        return "", err
    }
    
    if err := gz.Close(); err != nil {
        return "", err
    }
    
    return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
```

### Protocol Buffers

```go
// Use protobuf for efficient serialization
import "google.golang.org/protobuf/proto"

func serializeEvent(event *Event) ([]byte, error) {
    return proto.Marshal(event)
}

func deserializeEvent(data []byte) (*Event, error) {
    event := &Event{}
    return event, proto.Unmarshal(data, event)
}
```

### Lift-Specific Monitoring

```go
// Use Lift's built-in monitoring capabilities
function := constructs.NewLiftFunction(stack, jsii.String("MonitoredFunction"), &constructs.LiftFunctionProps{
    FunctionProps: awslambda.FunctionProps{
        Code: awslambda.Code_FromAsset(jsii.String("./function")),
    },
    EnableTracing:     jsii.Bool(true),  // X-Ray tracing
    EnableMetrics:     jsii.Bool(true),  // CloudWatch metrics
    EnableMultiTenant: jsii.Bool(true),  // Multi-tenant support
})

// Add custom metrics using Lift's observability package
import "github.com/pay-theory/lift/pkg/observability/cloudwatch"

func recordPerformanceMetrics(ctx context.Context, operation string, duration time.Duration) {
    metrics := cloudwatch.NewCloudWatchMetrics(client, cloudwatch.CloudWatchMetricsConfig{
        Namespace: "LiftApp/Performance",
        Dimensions: map[string]string{
            "FunctionName": os.Getenv("AWS_LAMBDA_FUNCTION_NAME"),
            "Operation":    operation,
        },
    })
    
    metrics.RecordLatency(operation, duration)
    metrics.RecordCount("operations.completed", 1)
}
```

## Monitoring Performance

### Custom Metrics

```go
import (
    "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
    "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func recordMetric(name string, value float64, unit types.StandardUnit) {
    cwClient.PutMetricData(context.TODO(), &cloudwatch.PutMetricDataInput{
        Namespace: aws.String("LiftApp/Performance"),
        MetricData: []types.MetricDatum{
            {
                MetricName: aws.String(name),
                Value:      aws.Float64(value),
                Unit:       unit,
                Dimensions: []types.Dimension{
                    {
                        Name:  aws.String("FunctionName"),
                        Value: aws.String(os.Getenv("AWS_LAMBDA_FUNCTION_NAME")),
                    },
                },
            },
        },
    })
}
```

### X-Ray Tracing

```go
import (
    "github.com/aws/aws-xray-sdk-go/xray"
)

func processWithTracing(ctx context.Context, event Event) error {
    ctx, seg := xray.BeginSubsegment(ctx, "ProcessEvent")
    defer seg.Close(nil)
    
    // Add metadata
    seg.AddMetadata("eventId", event.ID)
    seg.AddMetadata("eventType", event.Type)
    
    // Process with timing
    start := time.Now()
    err := processEvent(event)
    
    seg.AddMetadata("processingTime", time.Since(start).Milliseconds())
    
    if err != nil {
        seg.AddError(err)
    }
    
    return err
}
```

## Performance Testing

### Load Testing Configuration

```go
// Use CDK to create load testing infrastructure
loadTestFunction := constructs.NewLiftFunction(stack, jsii.String("LoadTest"), &constructs.LiftFunctionProps{
    CodeAssetPath: jsii.String("./load-test"),
    MemorySize:    jsii.Number(3008),
    Timeout:       awscdk.Duration_Minutes(jsii.Number(15)),
    Environment: &map[string]*string{
        "TARGET_QUEUE_URL": sqsProcessor.Queue.QueueUrl(),
        "EVENTS_PER_SECOND": jsii.String("1000"),
        "DURATION_MINUTES": jsii.String("10"),
    },
})
```

### Benchmarking Code

```go
func BenchmarkEventProcessing(b *testing.B) {
    events := generateTestEvents(1000)
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            processEventBatch(events)
        }
    })
    
    b.ReportMetric(float64(b.N*1000)/b.Elapsed().Seconds(), "events/sec")
}
```

## Architecture Patterns

### Fan-Out/Fan-In

```go
// Fan-out pattern for parallel processing
orchestrator := patterns.NewEventOrchestrator(stack, jsii.String("FanOut"), &patterns.EventOrchestratorProps{
    EnableSQS:         jsii.Bool(true),
    EnableEventBridge: jsii.Bool(true),
    SQSProcessorProps: &constructs.SQSProcessorProps{
        EventSourceProps: &awslambda.SqsEventSourceProps{
            MaxConcurrency: jsii.Number(100), // Fan-out
        },
    },
})
```

### Aggregation Pattern

```go
// Use Kinesis tumbling windows for aggregation
kinesisProcessor := constructs.NewKinesisProcessor(stack, jsii.String("Aggregator"), &constructs.KinesisProcessorProps{
    EnableTumblingWindow: jsii.Bool(true),
    TumblingWindowSeconds: jsii.Number(60), // 1-minute aggregations
    EventSourceProps: &awslambda.KinesisEventSourceProps{
        BatchSize: jsii.Number(100),
        ParallelizationFactor: jsii.Number(10),
    },
})
```

## Optimization Checklist

### Pre-Deployment
- [ ] Profile code for bottlenecks
- [ ] Choose optimal memory configuration
- [ ] Configure appropriate batch sizes
- [ ] Implement connection pooling
- [ ] Add caching where appropriate
- [ ] Enable compression for large payloads
- [ ] Optimize DynamORM table design and indexes
- [ ] Configure DynamORM connection pooling
- [ ] Enable Lift monitoring features

### Runtime
- [ ] Monitor Lambda concurrent executions
- [ ] Track error rates and retry counts
- [ ] Monitor iterator age (streams)
- [ ] Check for throttling
- [ ] Analyze X-Ray traces
- [ ] Review CloudWatch Insights queries
- [ ] Monitor DynamORM performance metrics
- [ ] Track DynamORM throttling and errors
- [ ] Monitor Lift-specific metrics

### Post-Deployment
- [ ] Analyze cost vs performance
- [ ] Identify optimization opportunities
- [ ] A/B test configuration changes
- [ ] Document performance baselines
- [ ] Set up performance alerts

## Performance Anti-Patterns to Avoid

1. **Synchronous Chaining**: Avoid Lambda-to-Lambda synchronous calls
2. **Large Payloads**: Use S3 for payloads > 256KB
3. **Inefficient Polling**: Configure appropriate polling intervals
4. **Memory Leaks**: Clean up resources properly
5. **Cold Start Amplification**: Use provisioned concurrency wisely
6. **Over-provisioning**: Right-size based on actual metrics

## Summary

Key performance optimization strategies:
1. **Measure First**: Profile before optimizing
2. **Batch Wisely**: Balance throughput and latency
3. **Cache Aggressively**: Reduce redundant operations
4. **Scale Horizontally**: Use concurrency controls
5. **Monitor Continuously**: Set up comprehensive observability
6. **Optimize DynamORM**: Use single table design and efficient queries
7. **Leverage Lift Features**: Use built-in monitoring and multi-tenant support