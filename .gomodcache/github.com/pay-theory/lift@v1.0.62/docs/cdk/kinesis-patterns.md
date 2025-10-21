# Kinesis Patterns with Lift CDK

This guide demonstrates how to use the KinesisProcessor construct to implement real-time streaming patterns in your Lift applications.

## Table of Contents
- [Overview](#overview)
- [Basic Usage](#basic-usage)
- [Stream Configuration](#stream-configuration)
- [Enhanced Fan-Out](#enhanced-fan-out)
- [Error Handling](#error-handling)
- [Windowing and Aggregation](#windowing-and-aggregation)
- [Common Patterns](#common-patterns)
- [Performance Optimization](#performance-optimization)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

The `KinesisProcessor` construct provides a complete Kinesis stream with Lambda processor setup, including:
- Automatic stream creation with on-demand or provisioned capacity
- Lambda function integration with configurable event source mapping
- Dead letter queue (DLQ) support for failed records
- Enhanced fan-out support for low-latency processing
- Batch processing with configurable windows
- Built-in retry and error handling mechanisms

### Default Configuration

The KinesisProcessor uses the following defaults:
- **Stream Mode**: On-demand (auto-scaling)
- **Retention Period**: 24 hours
- **Batch Size**: 100 records
- **Max Batching Window**: 5 seconds
- **DLQ**: Enabled by default
- **Enhanced Fan-Out**: Disabled by default
- **Architecture**: ARM64 (via LiftFunction defaults)
- **Memory**: 512MB (via LiftFunction defaults)
- **Timeout**: 30 seconds (via LiftFunction defaults)

## Basic Usage

### Simple Stream Processor

```go
import (
    "github.com/pay-theory/lift/pkg/cdk/constructs"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
)

processor := constructs.NewKinesisProcessor(stack, jsii.String("StreamProcessor"), &constructs.KinesisProcessorProps{
    FunctionProps: &constructs.LiftFunctionProps{
        Runtime: awslambda.Runtime_PROVIDED_AL2023(),
        Handler: jsii.String("bootstrap"),
        Code:    awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Environment: &map[string]*string{
            "SERVICE_NAME": jsii.String("stream-processor"),
        },
    },
})
```

### Publishing Records

```go
// Grant write permissions to another Lambda
producer := constructs.NewLiftFunction(stack, jsii.String("Producer"), producerProps)
processor.GrantWrite(producer.Function)

// In your Go application code
import (
    "context"
    "encoding/json"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/kinesis"
    "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
    "github.com/google/uuid"
)

func publishRecord(ctx context.Context, streamName string, data interface{}) error {
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return err
    }
    
    kinesisClient := kinesis.NewFromConfig(cfg)
    
    dataJSON, _ := json.Marshal(data)
    
    _, err = kinesisClient.PutRecord(ctx, &kinesis.PutRecordInput{
        StreamName:   aws.String(streamName),
        Data:         dataJSON,
        PartitionKey: aws.String(uuid.New().String()),
    })
    return err
}

// Batch publishing
func publishBatch(ctx context.Context, streamName string, records []interface{}) error {
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return err
    }
    
    kinesisClient := kinesis.NewFromConfig(cfg)
    
    var kinesisRecords []types.PutRecordsRequestEntry
    for _, record := range records {
        dataJSON, _ := json.Marshal(record)
        kinesisRecords = append(kinesisRecords, types.PutRecordsRequestEntry{
            Data:         dataJSON,
            PartitionKey: aws.String(uuid.New().String()),
        })
    }
    
    resp, err := kinesisClient.PutRecords(ctx, &kinesis.PutRecordsInput{
        StreamName: aws.String(streamName),
        Records:    kinesisRecords,
    })
    
    // Handle failed records
    if resp.FailedRecordCount != nil && *resp.FailedRecordCount > 0 {
        // Retry failed records
    }
    
    return err
}
```

## Stream Configuration

### On-Demand vs Provisioned Mode

```go
// On-demand mode (default) - auto-scales
onDemandProcessor := constructs.NewKinesisProcessor(stack, jsii.String("OnDemand"), &constructs.KinesisProcessorProps{
    FunctionProps: functionProps,
    StreamMode: awskinesis.StreamMode_OnDemand(),
})

// Provisioned mode - fixed capacity
provisionedProcessor := constructs.NewKinesisProcessor(stack, jsii.String("Provisioned"), &constructs.KinesisProcessorProps{
    FunctionProps: functionProps,
    StreamMode: awskinesis.StreamMode_Provisioned(jsii.Number(10)),
    ShardCount: jsii.Number(10),
})
```

### Retention and Encryption

```go
processor := constructs.NewKinesisProcessor(stack, jsii.String("SecureStream"), &constructs.KinesisProcessorProps{
    FunctionProps: functionProps,
    RetentionPeriodHours: jsii.Number(168), // 7 days
    Encryption: awskinesis.StreamEncryption_KMS,
    StreamProps: &awskinesis.StreamProps{
        StreamName: jsii.String("secure-events"),
        EncryptionKey: kmsKey, // Use specific KMS key
    },
})
```

## Enhanced Fan-Out

### Low-Latency Processing

```go
processor := constructs.NewKinesisProcessor(stack, jsii.String("EnhancedProcessor"), &constructs.KinesisProcessorProps{
    FunctionProps: functionProps,
    EnableEnhancedFanOut: jsii.Bool(true),
    ConsumerName: jsii.String("real-time-consumer"),
    EventSourceProps: &awslambdaeventsources.KinesisEventSourceProps{
        BatchSize: jsii.Number(10), // Smaller batches for lower latency
        MaxBatchingWindow: awscdk.Duration_Millis(jsii.Number(100)),
    },
})

// Access the enhanced fan-out consumer
if processor.Consumer != nil {
    log.Printf("Enhanced fan-out consumer created: %s", processor.Consumer.ConsumerArn())
}
```

## Error Handling

### Configuring Retry and DLQ

```go
processor := constructs.NewKinesisProcessor(stack, jsii.String("ResilientProcessor"), &constructs.KinesisProcessorProps{
    FunctionProps: functionProps,
    EnableDLQ: jsii.Bool(true),
    DLQProps: &awssqs.QueueProps{
        QueueName: jsii.String("stream-dlq"),
        RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
    },
    EventSourceProps: &awslambdaeventsources.KinesisEventSourceProps{
        BisectBatchOnError: jsii.Bool(true), // Split batch on error
        RetryAttempts: jsii.Number(3),
        MaxRecordAge: awscdk.Duration_Hours(jsii.Number(2)),
        ReportBatchItemFailures: jsii.Bool(true), // Enable partial batch responses
    },
})
```

### Handling Failed Records

```go
// Lambda handler with partial batch failure support
type KinesisEvent struct {
    Records []KinesisEventRecord `json:"Records"`
}

type KinesisEventRecord struct {
    Kinesis KinesisRecord `json:"kinesis"`
    EventID string        `json:"eventID"`
}

type BatchItemFailure struct {
    ItemIdentifier string `json:"itemIdentifier"`
}

type BatchItemFailures struct {
    BatchItemFailures []BatchItemFailure `json:"batchItemFailures"`
}

func handler(ctx context.Context, event KinesisEvent) (*BatchItemFailures, error) {
    var failures []BatchItemFailure
    
    for _, record := range event.Records {
        if err := processRecord(record); err != nil {
            log.Printf("Failed to process record %s: %v", record.EventID, err)
            failures = append(failures, BatchItemFailure{
                ItemIdentifier: record.EventID,
            })
        }
    }
    
    if len(failures) > 0 {
        return &BatchItemFailures{BatchItemFailures: failures}, nil
    }
    
    return nil, nil
}
```

## Windowing and Aggregation

### Tumbling Windows

```go
processor := constructs.NewKinesisProcessor(stack, jsii.String("WindowProcessor"), &constructs.KinesisProcessorProps{
    FunctionProps: functionProps,
    TumblingWindowSeconds: jsii.Number(60), // 1-minute windows
    EventSourceProps: &awslambdaeventsources.KinesisEventSourceProps{
        ParallelizationFactor: jsii.Number(5), // Process multiple windows in parallel
    },
})

// Handler for windowed data
type WindowedEvent struct {
    Window struct {
        Start string `json:"start"`
        End   string `json:"end"`
    } `json:"window"`
    Records []KinesisEventRecord `json:"records"`
    State   map[string]interface{} `json:"state"`
}

func windowHandler(ctx context.Context, event WindowedEvent) (map[string]interface{}, error) {
    // Aggregate records in the window
    aggregated := event.State
    if aggregated == nil {
        aggregated = make(map[string]interface{})
    }
    
    for _, record := range event.Records {
        // Update aggregated state
    }
    
    // Return state for next window
    return aggregated, nil
}
```

## Common Patterns

### Real-Time Analytics

```go
// Create analytics pipeline
analyticsProcessor := constructs.NewKinesisProcessor(stack, jsii.String("Analytics"), &constructs.KinesisProcessorProps{
    FunctionProps: &constructs.LiftFunctionProps{
        Runtime: awslambda.Runtime_PROVIDED_AL2023(),
        Handler: jsii.String("bootstrap"),
        Code:    awslambda.Code_FromAsset(jsii.String("./dist/analytics"), nil),
        MemorySize: jsii.Number(1024), // More memory for analytics
        Timeout: awscdk.Duration_Minutes(jsii.Number(5)),
    },
    BatchSize: jsii.Number(1000), // Large batches for efficiency
    MaxBatchingWindowSeconds: jsii.Number(10), // Wait up to 10 seconds
    ParallelizationFactor: jsii.Number(10), // Maximum parallelization
})

// Analytics handler
func analyticsHandler(ctx context.Context, event KinesisEvent) error {
    metrics := make(map[string]float64)
    
    for _, record := range event.Records {
        data, _ := base64.StdEncoding.DecodeString(record.Kinesis.Data)
        
        var metric Metric
        json.Unmarshal(data, &metric)
        
        // Aggregate metrics
        metrics[metric.Name] += metric.Value
    }
    
    // Store aggregated metrics
    return storeMetrics(metrics)
}
```

### Log Processing Pipeline

```go
// Log ingestion stream
logProcessor := constructs.NewKinesisProcessor(stack, jsii.String("LogProcessor"), &constructs.KinesisProcessorProps{
    FunctionProps: &constructs.LiftFunctionProps{
        Runtime: awslambda.Runtime_PROVIDED_AL2023(),
        Handler: jsii.String("bootstrap"),
        Code:    awslambda.Code_FromAsset(jsii.String("./dist/log-processor"), nil),
        Environment: &map[string]*string{
            "ELASTICSEARCH_URL": jsii.String("https://search.example.com"),
        },
    },
    StreamProps: &awskinesis.StreamProps{
        StreamName: jsii.String("application-logs"),
        StreamMode: awskinesis.StreamMode_OnDemand(),
    },
    EventSourceProps: &awslambdaeventsources.KinesisEventSourceProps{
        StartingPosition: awslambda.StartingPosition_LATEST,
        BatchSize: jsii.Number(100),
        MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(5)),
    },
})
```

### Multi-Consumer Pattern

```go
// Main stream
mainStream := awskinesis.NewStream(stack, jsii.String("MainStream"), &awskinesis.StreamProps{
    StreamName: jsii.String("events"),
    StreamMode: awskinesis.StreamMode_OnDemand(),
})

// Real-time processor
realtimeProcessor := constructs.NewKinesisProcessor(stack, jsii.String("Realtime"), &constructs.KinesisProcessorProps{
    FunctionProps: realtimeFunctionProps,
    ExistingStream: mainStream,
    EnableEnhancedFanOut: jsii.Bool(true),
    ConsumerName: jsii.String("realtime-consumer"),
})

// Batch processor
batchProcessor := constructs.NewKinesisProcessor(stack, jsii.String("Batch"), &constructs.KinesisProcessorProps{
    FunctionProps: batchFunctionProps,
    ExistingStream: mainStream,
    BatchSize: jsii.Number(1000),
    MaxBatchingWindowSeconds: jsii.Number(60), // 1 minute
})

// Archive processor
archiveProcessor := constructs.NewKinesisProcessor(stack, jsii.String("Archive"), &constructs.KinesisProcessorProps{
    FunctionProps: archiveFunctionProps,
    ExistingStream: mainStream,
    StartingPosition: awslambda.StartingPosition_TRIM_HORIZON,
})
```

## Performance Optimization

### Batch Processing Optimization

```go
processor := constructs.NewKinesisProcessor(stack, jsii.String("OptimizedProcessor"), &constructs.KinesisProcessorProps{
    FunctionProps: &constructs.LiftFunctionProps{
        Runtime: awslambda.Runtime_PROVIDED_AL2023(),
        Handler: jsii.String("bootstrap"),
        Code:    awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        MemorySize: jsii.Number(3008), // Maximum memory
        Architecture: awslambda.Architecture_ARM_64(), // Better price/performance
    },
    BatchSize: jsii.Number(10000), // Maximum batch size
    MaxBatchingWindowSeconds: jsii.Number(20),
    ParallelizationFactor: jsii.Number(10),
})
```

### Efficient Record Processing

```go
// Process records in parallel
func handler(ctx context.Context, event KinesisEvent) error {
    var wg sync.WaitGroup
    errors := make(chan error, len(event.Records))
    
    // Process records concurrently
    for _, record := range event.Records {
        wg.Add(1)
        go func(r KinesisEventRecord) {
            defer wg.Done()
            if err := processRecord(r); err != nil {
                errors <- err
            }
        }(record)
    }
    
    wg.Wait()
    close(errors)
    
    // Check for errors
    var errs []error
    for err := range errors {
        errs = append(errs, err)
    }
    
    if len(errs) > 0 {
        return fmt.Errorf("failed to process %d records", len(errs))
    }
    
    return nil
}
```

## Best Practices

### 1. Partition Key Strategy

```go
// Use meaningful partition keys for even distribution
func getPartitionKey(event Event) string {
    // Option 1: Use customer ID for customer-centric apps
    return event.CustomerID
    
    // Option 2: Use hash of multiple fields
    hash := sha256.Sum256([]byte(event.CustomerID + event.EventType))
    return fmt.Sprintf("%x", hash)
    
    // Option 3: Random for even distribution
    return uuid.New().String()
}
```

### 2. Data Compression

```go
import (
    "bytes"
    "compress/gzip"
    "context"
    "encoding/json"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/kinesis"
    "github.com/google/uuid"
)

func publishCompressed(ctx context.Context, streamName string, data interface{}) error {
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return err
    }
    
    kinesisClient := kinesis.NewFromConfig(cfg)
    
    dataJSON, _ := json.Marshal(data)
    
    var buf bytes.Buffer
    gz := gzip.NewWriter(&buf)
    gz.Write(dataJSON)
    gz.Close()
    
    _, err = kinesisClient.PutRecord(ctx, &kinesis.PutRecordInput{
        StreamName:   aws.String(streamName),
        Data:         buf.Bytes(),
        PartitionKey: aws.String(uuid.New().String()),
    })
    return err
}

// In Lambda handler
func decompressRecord(data string) ([]byte, error) {
    compressed, _ := base64.StdEncoding.DecodeString(data)
    reader, _ := gzip.NewReader(bytes.NewReader(compressed))
    return io.ReadAll(reader)
}
```

### 3. Checkpointing

```go
import (
    "context"
    "fmt"
    "time"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Store processing checkpoint
type Checkpoint struct {
    ShardID        string
    SequenceNumber string
    Timestamp      time.Time
}

func saveCheckpoint(ctx context.Context, shardID, sequenceNumber string) error {
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return err
    }
    
    dynamoClient := dynamodb.NewFromConfig(cfg)
    
    checkpoint := Checkpoint{
        ShardID:        shardID,
        SequenceNumber: sequenceNumber,
        Timestamp:      time.Now(),
    }
    
    // Save to DynamoDB
    _, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String("kinesis-checkpoints"),
        Item: map[string]types.AttributeValue{
            "ShardID":        &types.AttributeValueMemberS{Value: checkpoint.ShardID},
            "SequenceNumber": &types.AttributeValueMemberS{Value: checkpoint.SequenceNumber},
            "Timestamp":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", checkpoint.Timestamp.Unix())},
        },
    })
    return err
}
```

### 4. Monitoring

```go
// Add CloudWatch alarms
iteratorAgeAlarm := awscloudwatch.NewAlarm(stack, jsii.String("IteratorAge"), &awscloudwatch.AlarmProps{
    Metric: processor.Stream.MetricGetRecordsIteratorAgeMilliseconds(),
    Threshold: jsii.Number(60000), // 60 seconds
    EvaluationPeriods: jsii.Number(2),
})

// Monitor Lambda errors
errorAlarm := awscloudwatch.NewAlarm(stack, jsii.String("ProcessorErrors"), &awscloudwatch.AlarmProps{
    Metric: processor.Function.Function.MetricErrors(),
    Threshold: jsii.Number(10),
    EvaluationPeriods: jsii.Number(1),
})
```

## Troubleshooting

### Common Issues

1. **Iterator Age Increasing**
   - Lambda function is falling behind
   - Increase parallelization factor
   - Optimize processing logic
   - Consider enhanced fan-out

2. **Throttling Errors**
   - Exceeded shard limits
   - Switch to on-demand mode
   - Increase shard count
   - Implement exponential backoff

3. **DLQ Messages**
   - Check Lambda logs for processing errors
   - Verify data format
   - Ensure sufficient Lambda timeout
   - Check memory allocation

4. **Duplicate Processing**
   - Implement idempotency
   - Track sequence numbers
   - Use exactly-once processing patterns

### Debugging Tips

```go
// Enable detailed logging
func handler(ctx context.Context, event KinesisEvent) error {
    log.Printf("Processing %d records", len(event.Records))
    
    for _, record := range event.Records {
        log.Printf("Record: EventID=%s, ShardID=%s, SequenceNumber=%s",
            record.EventID,
            record.Kinesis.PartitionKey,
            record.Kinesis.SequenceNumber,
        )
        
        // Decode and log data
        data, _ := base64.StdEncoding.DecodeString(record.Kinesis.Data)
        log.Printf("Data: %s", string(data))
    }
    
    return nil
}
```

### Performance Tuning

1. **Optimize Batch Size**
   ```go
   // Start with defaults and adjust based on metrics
   batchSize := 100
   if averageRecordSize > 1000 { // bytes
       batchSize = 50
   } else if averageRecordSize < 100 {
       batchSize = 1000
   }
   ```

2. **Memory Allocation**
   ```go
   // Base memory on processing needs
   memorySize := 512
   if processingType == "analytics" {
       memorySize = 3008
   } else if processingType == "simple-transform" {
       memorySize = 256
   }
   ```

## KinesisProcessor Methods

The KinesisProcessor construct provides several methods for managing permissions and accessing stream information:

### Permission Methods

```go
// Grant write permissions to another Lambda function
processor.GrantWrite(producer.Function)

// Grant read permissions to another Lambda function  
processor.GrantRead(reader.Function)

// Grant both read and write permissions
processor.GrantReadWrite(processor.Function)
```

### Utility Methods

```go
// Add environment variables to the processing function
processor.AddEnvironmentVariable("CUSTOM_CONFIG", "value")

// Get stream information
streamName := processor.GetStreamName()
streamArn := processor.GetStreamArn()

// Get DLQ URL (if enabled)
dlqUrl := processor.GetDeadLetterQueueUrl()
if dlqUrl != nil {
    log.Printf("DLQ URL: %s", *dlqUrl)
}
```

### Accessing Components

```go
// Access the underlying AWS resources
stream := processor.Stream
function := processor.Function
dlq := processor.DLQ
consumer := processor.Consumer // Enhanced fan-out consumer (if enabled)

// Use stream metrics
iteratorAgeMetric := stream.MetricGetRecordsIteratorAgeMilliseconds()
```

## Next Steps

- Explore [EventBridge patterns](./eventbridge-patterns.md) for event-driven architectures
- Learn about [DynamoDB Streams patterns](./dynamo-streams-patterns.md) for change data capture
- See [Multi-Event patterns](./multi-event-patterns.md) for complex workflows