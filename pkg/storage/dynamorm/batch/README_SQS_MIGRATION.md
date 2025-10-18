# SQS-Based Batch Processing Migration

## Overview

This document outlines the migration from polling-based `StreamingBatchProcessor` to event-driven SQS-based batch processing for Lambda functions.

## Problem with Original Implementation

The original `StreamingBatchProcessor` had fundamental issues for serverless Lambda functions:

```go
// ❌ PROBLEMATIC: Polling with ticker
func (sbp *StreamingBatchProcessor) ProcessStream(ctx context.Context, itemChan <-chan any, errorCallback func(error)) {
    buffer := make([]any, 0, sbp.batchSize)
    ticker := time.NewTicker(5 * time.Second) // Flush buffer every 5 seconds
    defer ticker.Stop()

    for {
        select {
        case item, ok := <-itemChan:
            // Buffer items...
        case <-ticker.C:
            // Periodic flush - DOESN'T WORK IN LAMBDA!
        }
    }
}
```

### Issues:
1. **Lambda Lifecycle**: Lambda functions terminate after handling requests, not continuously running
2. **Buffered Data Loss**: Items buffered in memory are lost when Lambda container freezes
3. **Ticker-based Processing**: Periodic flushing assumes long-running processes
4. **No Event-Driven Architecture**: Doesn't leverage serverless event patterns

## New SQS-Based Solution

### Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Handler   │    │   SQS Queue     │    │ Batch Processor │
│                 │───▶│                 │───▶│    Lambda       │
│ Creates batches │    │  Message Store  │    │  Processes all  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Key Components

#### 1. SQSBatchProcessor

```go
type SQSBatchProcessor struct {
    db           core.DB
    logger       *zap.Logger
    tracker      *cost.Tracker
    batchWriter  *BatchWriter
    maxBatchSize int
}
```

#### 2. Lambda Handler Pattern

```go
func HandleSQSBatch(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
    processor := NewSQSBatchProcessor(db, SQSBatchProcessorConfig{
        Logger:       logger,
        Tracker:      tracker,
        MaxBatchSize: 25,
    })
    
    return processor.ProcessBatch(ctx, event)
}
```

#### 3. Batch Message Format

```go
type BatchMessage struct {
    Operation string `json:"operation"` // "create", "update", "delete"
    Items     []any  `json:"items"`
    TableName string `json:"table_name,omitempty"`
    Metadata  map[string]any `json:"metadata,omitempty"`
}
```

## Migration Benefits

### 1. Event-Driven Processing
- **No Polling**: React to SQS messages directly
- **No Buffering**: Process messages synchronously within Lambda invocation
- **Guaranteed Processing**: SQS handles message persistence and retry

### 2. Serverless Optimized
- **Lambda Lifecycle Aware**: Process and return within single invocation
- **Auto-scaling**: SQS triggers Lambda instances based on queue depth
- **Cost Efficient**: Pay only for processing time, not idle polling

### 3. Fault Tolerance
- **Batch Item Failures**: Return failed message IDs for SQS retry
- **Automatic Retries**: SQS handles retry logic with exponential backoff
- **Dead Letter Queues**: Failed messages can be routed for investigation

### 4. Observability
- **Structured Logging**: Comprehensive logging with request IDs
- **Cost Tracking**: DynamoDB operation cost tracking
- **Metrics**: Processing duration, success/failure rates

## Usage Examples

### 1. Timeline Batch Processing

```go
// Create timeline entries for new post
timelineMsg := CreateTimelineMessage(
    followerIDs,     // []string
    statusID,        // string
    authorID,        // string
    createdAt,       // time.Time
)

// Send to SQS (in actual implementation)
sqsClient.SendMessage(&sqs.SendMessageInput{
    QueueUrl:    aws.String(timelineQueueURL),
    MessageBody: aws.String(string(messageBody)),
})

// Lambda processes batch automatically
func main() {
    lambda.Start(HandleTimelineBatch)
}
```

### 2. Notification Batch Processing

```go
// Create mention notifications
notifMsg := CreateNotificationMessage(
    mentionedUserIDs, // []string
    statusID,         // string
    authorID,         // string
    "mention",        // notification type
    "status",         // target type
)

// Lambda handler processes notifications
func HandleNotificationBatch(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
    return processor.ProcessNotifications(ctx, event)
}
```

### 3. Batch Delete Operations

```go
// Delete user data
deleteKeys := []any{
    map[string]any{"PK": "USER#user123", "SK": "PROFILE"},
    map[string]any{"PK": "USER#user123", "SK": "SETTINGS"},
}

deleteMsg := CreateBatchDeleteMessage(deleteKeys, "users")

// Processed via generic batch handler
func main() {
    lambda.Start(HandleGenericBatch)
}
```

## Configuration Best Practices

### 1. Batch Size Optimization

```go
// Optimize batch size based on item size
func OptimalBatchSize(items []any) int {
    if len(items) == 0 {
        return 0
    }
    
    sampleItem, _ := json.Marshal(items[0])
    estimatedItemSize := len(sampleItem)
    
    if estimatedItemSize > 10000 { // 10KB per item
        return 10 // Smaller batches for large items
    } else if estimatedItemSize > 1000 { // 1KB per item
        return 20
    }
    
    return 25 // Maximum DynamoDB batch size
}
```

### 2. SQS Queue Configuration

```yaml
# CloudFormation/CDK
TimelineBatchQueue:
  Type: AWS::SQS::Queue
  Properties:
    VisibilityTimeoutSeconds: 180  # 3x Lambda timeout
    MessageRetentionPeriod: 1209600  # 14 days
    ReddrivePolicy:
      deadLetterTargetArn: !GetAtt TimelineBatchDLQ.Arn
      maxReceiveCount: 3
    
TimelineBatchDLQ:
  Type: AWS::SQS::Queue
  Properties:
    MessageRetentionPeriod: 1209600
```

### 3. Lambda Function Configuration

```yaml
TimelineBatchProcessor:
  Type: AWS::Lambda::Function
  Properties:
    Runtime: provided.al2023  # ARM64 for better performance
    Architecture: arm64
    MemorySize: 512
    Timeout: 60
    Environment:
      Variables:
        DYNAMODB_TABLE_NAME: !Ref MainTable
    EventSourceMapping:
      EventSourceArn: !GetAtt TimelineBatchQueue.Arn
      BatchSize: 10  # Process up to 10 SQS messages per invocation
      MaximumBatchingWindowInSeconds: 5
```

## Error Handling and Retries

### 1. Batch Item Failures

```go
func (p *SQSBatchProcessor) ProcessBatch(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
    var batchItemFailures []events.SQSBatchItemFailure
    
    for _, record := range event.Records {
        if err := p.processMessage(ctx, record); err != nil {
            // Mark message for retry
            batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
                ItemIdentifier: record.MessageId,
            })
        }
    }
    
    return events.SQSEventResponse{
        BatchItemFailures: batchItemFailures,
    }, nil
}
```

### 2. Retry Strategy

- **SQS Automatic Retries**: Failed messages automatically retried with exponential backoff
- **Dead Letter Queues**: Messages failing after max retries sent to DLQ for investigation
- **Partial Failures**: Only failed messages retried, successful ones acknowledged

### 3. Monitoring and Alerting

```go
// CloudWatch metrics
func (p *SQSBatchProcessor) trackMetrics(processed, failed int, duration time.Duration) {
    cloudwatch.PutMetric("BatchProcessor/ProcessedMessages", float64(processed))
    cloudwatch.PutMetric("BatchProcessor/FailedMessages", float64(failed))
    cloudwatch.PutMetric("BatchProcessor/ProcessingDuration", duration.Seconds())
}

// CloudWatch alarms
BatchProcessorHighFailureRate:
  Type: AWS::CloudWatch::Alarm
  Properties:
    MetricName: FailedMessages
    Namespace: BatchProcessor
    Statistic: Sum
    Period: 300
    EvaluationPeriods: 2
    Threshold: 10
    ComparisonOperator: GreaterThanThreshold
```

## Performance Characteristics

### 1. Cold Start Optimization
- **Lambda Optimized DynamORM**: 91% faster cold starts vs AWS SDK
- **Connection Reuse**: Reuse database connections across invocations
- **Model Pre-registration**: Pre-register DynamORM models in init()

### 2. Throughput
- **Concurrent Processing**: Multiple Lambda instances process queue in parallel
- **Batch Processing**: Process up to 25 items per DynamoDB operation
- **Auto-scaling**: SQS triggers additional Lambda instances based on queue depth

### 3. Cost Optimization
- **Pay per Use**: Only pay for Lambda execution time, not idle polling
- **Batch Efficiency**: Reduce per-operation costs with batch operations
- **Cost Tracking**: Built-in cost tracking for DynamoDB operations

## Migration Checklist

### 1. Remove Old Implementation
- [x] Remove `StreamingBatchProcessor` from `batch_repository.go`
- [x] Remove ticker-based processing methods
- [x] Remove channel-based streaming patterns
- [x] Update tests to remove streaming processor tests

### 2. Implement New SQS Processor
- [x] Create `SQSBatchProcessor` with event-driven processing
- [x] Implement batch message handling for create/update/delete operations
- [x] Add specialized handlers for timeline and notification processing
- [x] Create comprehensive test suite

### 3. Update Lambda Functions
- [ ] Replace streaming processors with SQS handlers in Lambda functions
- [ ] Update infrastructure to create SQS queues and event source mappings
- [ ] Configure dead letter queues and retry policies
- [ ] Update monitoring and alerting

### 4. Message Producers
- [ ] Update API handlers to send messages to SQS instead of channels
- [ ] Implement message batching for high-volume operations
- [ ] Add circuit breakers for SQS failures

### 5. Monitoring and Operations
- [ ] Set up CloudWatch dashboards for batch processing metrics
- [ ] Configure alarms for high failure rates and DLQ depth
- [ ] Implement log aggregation and search
- [ ] Create runbooks for common failure scenarios

## Conclusion

The migration from polling-based to SQS-based batch processing provides:

1. **Serverless-Native Architecture**: Designed for Lambda execution model
2. **Improved Reliability**: No data loss from buffer management issues
3. **Better Scalability**: Auto-scaling based on queue depth
4. **Enhanced Observability**: Comprehensive logging and metrics
5. **Cost Efficiency**: Pay-per-use model with no idle costs

This architecture leverages AWS serverless patterns for robust, scalable batch processing that works seamlessly with Lambda functions.