# ML Training Processor Lambda

## Purpose

Processes DynamoDB Stream events for ML training job lifecycle management using an **event-driven, asynchronous polling** pattern via `MLPollRequest` and `ModelTrainingJob` records.

## Architecture

This Lambda function follows Lesser's **DynamoDB Streams + Event Bus** architecture pattern with **NO in-process polling or goroutines**.

### Event Flow (Phase 2.3 Implementation)

```
1. TrainModel mutation → Creates ModelTrainingJob (status=SUBMITTED)
                       → Creates initial MLPollRequest (NextPollAfter=+30s)
   ↓
2. DynamoDB Stream → INSERT on MLPollRequest → ml-training-processor
   ↓
3. Lambda checks if NextPollAfter has passed
   ↓
4. Calls Bedrock GetModelCustomizationJob()
   ↓
5. Updates ModelTrainingJob with new status
   ↓
6. If still IN_PROGRESS:
   - Creates NEW MLPollRequest (NextPollAfter=+60s)
   ↓
7. On COMPLETED/FAILED:
   - DynamoDB Stream → MODIFY on ModelTrainingJob → ml-training-processor
   ↓
8. Lambda handles completion:
   - Deactivates previous active models
   - Extracts real metrics from Bedrock TrainingMetrics (with S3 fallback)
   - Creates ModerationModelVersion (IsActive=true)
   - Emits MODEL_TRAINING_COMPLETED event
```

**Key Principle**: **No goroutines, no time.Sleep(), no in-process polling**. All async behavior via DynamoDB writes that trigger stream events.

## Responsibilities

### 1. Poll Request Handling (INSERT on MLPollRequest)
- Detects new `MLPollRequest` records with `Status=PENDING`
- Checks if `NextPollAfter` timestamp has passed
- Calls `bedrock.GetModelCustomizationJob()` to get current status
- Updates `ModelTrainingJob` with latest status from Bedrock
- Schedules next poll by creating new `MLPollRequest` (if job still running)
- Handles timeouts after `MaxAttempts` (default: 120 attempts = 2 hours)

### 2. Job Status Change Handling (MODIFY on ModelTrainingJob)
- Detects status changes on `ModelTrainingJob` records
- Routes to appropriate handler based on new status:
  - `COMPLETED` → `handleJobCompletion()`
  - `FAILED` or `TIMEOUT` → `handleJobFailure()`

### 3. Completion Handling
**On COMPLETED**:
- ✅ Deactivates all existing active model versions
- ✅ Extracts training metrics from Bedrock `TrainingMetrics` field
- ✅ Falls back to S3 output logs if `TrainingMetrics` unavailable
- ✅ Creates `ModerationModelVersion` record with real metrics
- ✅ Sets `IsActive=true` for the new model
- ✅ Emits `MODEL_TRAINING_COMPLETED` event to DynamoDB for stream processing

**On FAILED/TIMEOUT**:
- ✅ Logs error details with full context
- ✅ Emits `MODEL_TRAINING_FAILED` event to DynamoDB
- ✅ Stream processors handle downstream notifications

## Metrics Extraction (Phase 2.3 Complete)

The processor implements a **3-tier fallback strategy** for extracting real training metrics:

### Tier 1: Bedrock TrainingMetrics (Primary)
```go
if output.TrainingMetrics != nil {
    metrics := p.extractMetricsFromBedrockOutput(output.TrainingMetrics)
    // Handles multiple formats: direct fields, validation_metrics, evaluation
    // Supports both "f1_score" and "f1" field names
}
```

### Tier 2: S3 Output Logs (Fallback)
```go
else if output.OutputDataConfig.S3Uri != nil {
    metrics := p.parseMetricsFromS3(ctx, s3OutputPath)
    // Tries multiple file names: metrics.json, training_results.json, etc.
}
```

### Tier 3: Default Values (Last Resort)
```go
else {
    logger.Warn("no training metrics available")
    job.Metrics = {Accuracy: 0.0, Precision: 0.0, Recall: 0.0, F1Score: 0.0}
}
```

**Test Coverage**: 9 unit tests in `metrics_test.go` validate all extraction paths and error scenarios.

## Configuration

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `DYNAMODB_TABLE_NAME` | Main DynamoDB table name | Yes |
| `AWS_REGION` | AWS region for all services | Yes |

### IAM Permissions

```yaml
- bedrock:GetModelCustomizationJob
- dynamodb:GetItem
- dynamodb:UpdateItem
- dynamodb:PutItem
- dynamodb:Query
- dynamodb:DescribeStream
- dynamodb:GetRecords
- dynamodb:GetShardIterator
- dynamodb:ListStreams
- s3:GetObject  # For metrics fallback
```

## Stream Processing

### Handled Event Types

1. **INSERT on MLPollRequest** (`Type=ML_POLL_REQUEST`)
   - Checks if poll is due (`NextPollAfter` < now)
   - Fetches Bedrock job status
   - Updates `ModelTrainingJob`
   - Schedules next poll if needed

2. **MODIFY on ModelTrainingJob** (`Type=ML_TRAINING_JOB`)
   - Detects status changes (old status ≠ new status)
   - Routes to completion/failure handlers

### Entity Type Detection

Uses `stream.GetEventType()` to identify record types:
- `ML_TRAINING_JOB` - Training job records
- `ML_POLL_REQUEST` - Async poll requests

## Error Handling

### Graceful Degradation
- ✅ Bedrock API throttling → Logs warning, schedules next poll with backoff
- ✅ Transient network errors → Logs error, schedules retry
- ✅ Metrics extraction failure → Uses fallback chain, logs warnings
- ✅ S3 file not found → Logs warning, uses default metrics (0.0)

### Timeout Handling
- Poll requests have `MaxAttempts` (default: 120)
- After max attempts → Marks job as `TIMEOUT`
- Prevents infinite polling loops

### Partial Batch Failures
- Processes all records in batch
- Collects errors but doesn't fail fast
- Logs partial failure count
- Returns success to prevent DynamoDB Stream retries

## Monitoring

### Key CloudWatch Metrics
- Training jobs submitted per hour
- Training job completion time distribution
- Training job failure rate  
- Poll request processing latency
- DynamoDB Stream lag
- Metrics extraction success rate

### Structured Logging

All log entries include:
```go
zap.String("job_id", jobARN)
zap.String("job_name", jobName)
zap.String("status", status)
zap.Int("attempt", pollAttempt)
zap.String("request_id", ctx.GetRequestID())
```

## Testing

### Unit Tests

```bash
./lesser test unit
# Or directly:
go test ./cmd/ml-training-processor/...
```

**Test Files**:
- `metrics_test.go` - Metrics extraction and parsing (9 tests)
  - JSON parsing from S3 files
  - Bedrock TrainingMetrics extraction
  - Error handling and edge cases

### Coverage Areas
- ✅ Bedrock TrainingMetrics extraction (direct fields, nested structures)
- ✅ S3 metrics file parsing (multiple formats)
- ✅ Invalid JSON handling
- ✅ Partial metrics extraction
- ✅ Null/empty input handling
- ✅ ARN version extraction

## Deployment

### CDK Configuration

```go
// In infra/cdk/constructs/lambda_functions.go
func (c *LambdaFunctionsConstruct) createMLTrainingProcessor() lambda.IFunction {
    return lambda.NewFunction(c.scope, jsii.String("MLTrainingProcessor"), &lambda.FunctionProps{
        Runtime:     lambda.Runtime_PROVIDED_AL2023(),
        Handler:     jsii.String("bootstrap"),
        Code:        lambda.Code_FromAsset(jsii.String("bin/ml-training-processor.zip"), nil),
        Timeout:     awscdk.Duration_Seconds(jsii.Number(60)),
        MemorySize:  jsii.Number(512),
        Environment: &map[string]*string{
            "DYNAMODB_TABLE_NAME": c.table.TableName(),
        },
        // DynamoDB Stream event source configured in stream_processors.go
    })
}
```

### Stream Event Source

```go
// In infra/cdk/constructs/stream_processors.go
table.GrantStreamRead(mlTrainingProcessor, nil)

lambda.NewEventSourceMapping(scope, jsii.String("MLTrainingProcessorStreamMapping"), &lambda.EventSourceMappingProps{
    Target:          mlTrainingProcessor,
    EventSourceArn:  table.TableStreamArn(),
    StartingPosition: lambda.StartingPosition_TRIM_HORIZON,
    BatchSize:       jsii.Number(10),
    RetryAttempts:   jsii.Number(3),
    MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(5)),
})
```

## Implementation Notes

### Why DynamoDB Stream Polling (Not Scheduled Lambda)

**Advantages**:
1. ✅ **Serverless-native**: No schedulers, uses native DynamoDB triggers
2. ✅ **Self-cleaning**: Poll requests are one-time items that auto-expire
3. ✅ **Backpressure**: Lambda concurrency limits prevent runaway polling
4. ✅ **Auditable**: All poll attempts visible in DynamoDB
5. ✅ **Scalable**: Handles multiple concurrent training jobs automatically

**Trade-offs**:
- More DynamoDB writes (poll requests)
- Stream processing latency (~1-2s per poll)
- Acceptable for training jobs (minutes to hours duration)

### Poll Request Pattern

```go
type MLPollRequest struct {
    PK            string    // "MLPOLL#{job_id}"
    SK            string    // "POLL#{timestamp}"
    JobID         string    // Bedrock job ARN
    JobName       string
    Attempt       int       // Polling attempt counter
    MaxAttempts   int       // Timeout threshold
    NextPollAfter time.Time // When to poll next
    Status        string    // "PENDING", "PROCESSED"
    TTL           int64     // Auto-expire after 48 hours
}
```

Each poll creates a **new** request record (not updates) to trigger fresh stream events.

## Comparison: Old vs New Architecture

### ❌ Old Approach (Pre-Phase 2.3)
```go
// Anti-pattern: Blocking Lambda with goroutines
go func() {
    for {
        time.Sleep(60 * time.Second)  // ← Keeps Lambda alive
        status := checkBedrock()
        if status == "COMPLETED" {
            break
        }
    }
}()
// Lambda times out after 15 minutes or costs accumulate
```

### ✅ New Approach (Phase 2.3)
```go
// Event-driven: Lambda returns immediately
func processRecord(record DynamoDBRecord) {
    if shouldPoll(pollRequest) {
        status := checkBedrock()
        updateJob(status)
        if status == "IN_PROGRESS" {
            createNextPollRequest()  // ← Triggers future event
        }
    }
    return // Lambda exits, no blocking
}
// Total Lambda execution: <1s per poll
```

## Related Documentation

- [MODERATION_ML_ARCHITECTURE.md](../../docs/MODERATION_ML_ARCHITECTURE.md) - Full ML pipeline architecture
- [PHASE_2_3_COMPLETION.md](../../docs/PHASE_2_3_COMPLETION.md) - Phase 2.3 completion summary
- [PHASE_2_3_FINAL_VERIFICATION.md](../../docs/PHASE_2_3_FINAL_VERIFICATION.md) - Verification report

---

**Status**: ✅ Production Ready  
**Last Updated**: October 17, 2025 (Phase 2.3 completion)  
**Next Phase**: 2.4 (Severed Relationships)
