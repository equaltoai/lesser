# Moderation ML Architecture

## Overview

The Moderation ML Pipeline is a production-grade, event-driven system for training and deploying custom machine learning models using AWS Bedrock. It follows Lesser's serverless architecture patterns using DynamoDB streams, Lambda functions, and centralized event emission.

## Architecture Principles

### Event-Driven State Machine

The system uses **DynamoDB as the orchestration backbone**, not scheduled Lambdas or in-process polling:

1. **State Changes via DynamoDB Records**: All state transitions are represented as DynamoDB items
2. **Stream Processing**: DynamoDB Streams trigger Lambda functions to react to state changes
3. **Asynchronous Polling**: Job status checks are implemented as poll request records that trigger stream processors
4. **No In-Process Loops**: No `time.Sleep()` or goroutine-based polling in Lambda functions

### Components

```
┌─────────────────────┐
│   GraphQL Mutation  │
│  trainModerationML  │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  ModerationML       │
│  Service            │
│  - CreateTrainingJob│
│  - PrepareDataset   │
│  - SubmitToBedr ock │
│  - EmitEvent        │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│   DynamoDB Table    │
│  - TrainingJob      │
│  - PollRequest      │
│  - Event Item       │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  DynamoDB Streams   │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ ml-training-        │
│ processor           │
│  - HandleJobChange  │
│  - HandlePollReq    │
│  - CheckJobStatus   │
│  - HandleCompletion │
└─────────────────────┘
```

## Data Models

### ModelTrainingJob

Tracks the full lifecycle of a Bedrock training job.

```go
type ModelTrainingJob struct {
    PK      string  // "MLJOB#{job_id}"
    SK      string  // "JOB#{job_id}"
    Type    string  // "ML_TRAINING_JOB"
    
    // Job identification
    JobID        string  // Bedrock job ARN
    JobName      string  // Human-readable name
    DatasetS3Key string  // S3 key of training dataset
    
    // Status tracking
    Status       string  // SUBMITTED, IN_PROGRESS, COMPLETED, FAILED
    ErrorMessage string  // Error details if FAILED
    
    // Training configuration
    SamplesUsed     int
    MinSamples      int
    BaseModelID     string
    OutputModelName string
    
    // Metrics (populated on completion)
    Metrics struct {
        Accuracy     float64
        Precision    float64
        Recall       float64
        F1Score      float64
        TrainingTime int64
    }
    
    // Timestamps
    CreatedAt   time.Time
    UpdatedAt   time.Time
    CompletedAt *time.Time
}
```

### MLPollRequest

Represents a request to check job status. Created when a job is submitted and recursively created until job completes.

```go
type MLPollRequest struct {
    PK  string  // "MLPOLL#{job_id}"
    SK  string  // "REQUEST#{timestamp_unix}"
    Type string // "ML_POLL_REQUEST"
    
    JobID         string
    RequestTime   time.Time
    PollInterval  int  // Seconds until next poll
    TTL           int64
}
```

### MLPrediction

Tracks every inference made by the model for effectiveness calculation.

```go
type MLPrediction struct {
    PK  string  // "MLPRED#{object_id}"
    SK  string  // "TIME#{RFC3339}#{prediction_id}"
    Type string // "ML_PREDICTION"
    
    // GSI1 - Query by model version
    gsi1PK string  // "MODEL#{model_version}"
    gsi1SK string  // "TIME#{RFC3339}"
    
    // GSI2 - Query by review status
    gsi2PK string  // "REVIEW#{true|false}"
    gsi2SK string  // "TIME#{RFC3339}"
    
    PredictionID    string
    ObjectID        string
    ObjectType      string
    ModelVersion    string
    PredictedLabel  string
    Confidence      float64
    HumanLabel      *string  // Set when human reviews
    HumanConfidence *float64
    Timestamp       time.Time
}
```

### ModerationModelVersion

Represents a trained model version with its metrics.

```go
type ModerationModelVersion struct {
    PK  string  // "MLMODEL#{tenant_id}"
    SK  string  // "VERSION#{version}"
    Type string // "ML_MODEL_VERSION"
    
    // GSI9 - Active model lookup
    gsi9PK string  // "MLACTIVE#{tenant_id}"
    gsi9SK string  // "VERSION#{version}"
    
    TenantID        string
    Version         string
    ModelARN        string
    TrainingJobID   string
    
    // Metrics from training
    Accuracy        float64
    Precision       float64
    Recall          float64
    F1Score         float64
    SamplesUsed     int
    TrainingTime    int64
    
    // Effectiveness tracking
    TotalPredictions      int
    CorrectPredictions    int
    AverageConfidence     float64
    EffectivenessScore    float64
    
    IsActive        bool
    CreatedAt       time.Time
}
```

## Event Flow

### 1. Job Submission

```
GraphQL trainModerationML
    ↓
Service.CreateTrainingJob()
    ↓
Write ModelTrainingJob (status=SUBMITTED)
    ↓
Submit to Bedrock
    ↓
Write MLPollRequest (initial poll, +30s)
    ↓
Emit EVENT#MODEL_TRAINING_SUBMITTED
    ↓
Return to client immediately
```

**Key Point**: The GraphQL mutation returns immediately with `status=SUBMITTED`. The job continues processing asynchronously.

### 2. Status Polling Loop

```
DynamoDB Stream detects MLPollRequest
    ↓
ml-training-processor.handlePollRequest()
    ↓
Call Bedrock GetModelCustomizationJob()
    ↓
Update ModelTrainingJob with new status
    ↓
If still IN_PROGRESS:
    Write new MLPollRequest (+60s)
Else:
    Stop polling
```

**Poll Intervals**:
- Initial: 30 seconds
- Subsequent: 60 seconds
- TTL: 48 hours (prevents infinite polling)

### 3. Job Completion

```
DynamoDB Stream detects ModelTrainingJob
    (status changed to COMPLETED)
    ↓
ml-training-processor.handleJobCompletion()
    ↓
Deactivate existing active models
    ↓
Extract real metrics from Bedrock TrainingMetrics
    (with S3 fallback if unavailable)
    ↓
Create ModerationModelVersion with real metrics
    ↓
Emit EVENT#MODEL_TRAINING_COMPLETED
    ↓
Stream processors handle notifications
```

### 4. Inference and Tracking

```
ModerationML.ScoreContent()
    ↓
Get active model version
    ↓
Call Bedrock InvokeModel()
    ↓
Write MLPrediction record
    ↓
Return scores
```

Every inference creates a prediction record for later effectiveness analysis.

### 5. Effectiveness Calculation

```
ModerationML.computeEffectiveness()
    ↓
Query MLPredictions for model version
    (within time window)
    ↓
Filter predictions with human labels
    ↓
Calculate:
    - Accuracy = correct / total
    - Avg confidence
    - Weighted effectiveness
    ↓
Update ModerationModelVersion
```

## Real Data Requirements

### Training Content

The system fetches **real content** from the database:

```go
// For statuses
status := statusRepo.GetStatus(ctx, objectID)
content := status.Content
if content == "" && status.Note != nil {
    content = status.Note.Content
    if status.Note.Summary != "" {
        content = status.Note.Summary + "\n\n" + content
    }
}

// Error if content is missing
if content == "" {
    return fmt.Errorf("status %s has no content", objectID)
}
```

**No placeholder text allowed**. If content cannot be fetched, the training fails early.

### Bedrock Metrics

Real metrics are extracted from Bedrock job output:

```go
output, err := s.bedrockClient.GetModelCustomizationJob(ctx, &bedrock.GetModelCustomizationJobInput{
    JobIdentifier: aws.String(jobID),
})

// Extract from TrainingMetrics map
if output.TrainingMetrics != nil {
    if acc, ok := output.TrainingMetrics["accuracy"].(float64); ok {
        job.Metrics.Accuracy = acc
    }
    // ... precision, recall, f1_score
}

// Calculate training duration
job.Metrics.TrainingTime = job.CompletedAt.Unix() - job.CreatedAt.Unix()
```

If metrics cannot be fetched, default to 0.0 (indicates incomplete training).

## Configuration

### Environment Variables

```yaml
ML_TRAINING_BUCKET: "lesser-training-{env}"
ML_BEDROCK_REGION: "us-east-1"
ML_BEDROCK_MODEL_ID: "amazon.titan-text-lite-v1"
ML_BEDROCK_CUSTOMIZATION_ROLE_ARN: "arn:aws:iam::..."
ML_POLL_INITIAL_DELAY: "30"     # seconds
ML_POLL_INTERVAL: "60"          # seconds
ML_MIN_SAMPLES: "10"            # minimum training samples
```

### IAM Permissions

The `ml-training-processor` Lambda needs:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:CreateModelCustomizationJob",
        "bedrock:GetModelCustomizationJob",
        "bedrock:ListModelCustomizationJobs",
        "bedrock:InvokeModel"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::lesser-training-*/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": ["iam:PassRole"],
      "Resource": "${BEDROCK_CUSTOMIZATION_ROLE_ARN}"
    }
  ]
}
```

The Bedrock customization role needs:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::lesser-training-*",
        "arn:aws:s3:::lesser-training-*/*"
      ]
    }
  ]
}
```

## Deployment

### CDK Configuration

The `ml-training-processor` Lambda is configured in `infra/cdk/constructs/stream_processors.go`:

```go
mlTrainingProcessor := lambda.NewFunction(stack, jsii.String("MLTrainingProcessor"), &lambda.FunctionProps{
    Runtime: lambda.Runtime_PROVIDED_AL2023(),
    Handler: jsii.String("bootstrap"),
    Code:    lambda.Code_FromAsset(jsii.String("bin/ml-training-processor.zip"), nil),
    Timeout: awscdk.Duration_Minutes(jsii.Number(5)),
    MemorySize: jsii.Number(512),
    Environment: &map[string]*string{
        "ML_TRAINING_BUCKET": jsii.String(config.ML.TrainingBucket),
        "ML_BEDROCK_REGION": jsii.String(config.ML.BedrockRegion),
        // ...
    },
})

// Add DynamoDB stream trigger
mlTrainingProcessor.AddEventSource(lambdaeventsources.NewDynamoEventSource(table, &lambda.DynamoEventSourceProps{
    StartingPosition: lambda.StartingPosition_LATEST,
    BatchSize: jsii.Number(10),
    Filters: []*map[string]interface{}{
        {
            "eventName": []string{"INSERT", "MODIFY"},
            "dynamodb": map[string]interface{}{
                "NewImage": map[string]interface{}{
                    "Type": map[string]interface{}{
                        "S": []string{"ML_TRAINING_JOB", "ML_POLL_REQUEST"},
                    },
                },
            },
        },
    },
}))
```

### Deployment Steps

1. **Build binaries**:
   ```bash
   make build
   ```

2. **Update CDK config** (add `BEDROCK_CUSTOMIZATION_ROLE_ARN`):
   ```yaml
   ml:
     bedrockCustomizationRoleArn: "arn:aws:iam::..."
   ```

3. **Deploy infrastructure**:
   ```bash
   cd infra/cdk
   cdk deploy lesser-{env}-stack
   ```

4. **Verify**:
   ```bash
   aws lambda list-functions --query 'Functions[?contains(FunctionName, `ml-training`)].FunctionName'
   ```

## Testing

### Manual Testing

1. **Queue training samples**:
   ```graphql
   mutation {
     queueModerationMLSample(
       objectId: "status_123"
       objectType: "status"
       label: "safe"
       confidence: 0.95
     ) {
       success
     }
   }
   ```

2. **Trigger training**:
   ```graphql
   mutation {
     trainModerationML(
       minSamples: 10
     ) {
       success
       status
       jobId
       jobName
     }
   }
   ```

3. **Check job status**:
   - Query DynamoDB for `PK=MLJOB#{job_id}`
   - Check CloudWatch Logs for `ml-training-processor`
   - Verify poll requests are being created

4. **Wait for completion** (typically 20-60 minutes for Bedrock training)

5. **Verify model version created**:
   ```graphql
   query {
     moderationMLModels {
       version
       isActive
       accuracy
       precision
       recall
       f1Score
     }
   }
   ```

### Unit Testing

Key test scenarios:

1. **Job submission creates poll request**
2. **Poll request triggers status check**
3. **Completion deactivates old models**
4. **Predictions are tracked**
5. **Effectiveness calculation is accurate**
6. **Real content fetching handles missing data**
7. **Metrics extraction from Bedrock**

## Monitoring

### CloudWatch Metrics

- `MLTrainingJobsSubmitted` - Count of jobs submitted
- `MLTrainingJobsCompleted` - Count of jobs completed
- `MLTrainingJobsFailed` - Count of jobs failed
- `MLPredictions` - Count of predictions made
- `MLEffectivenessScore` - Current effectiveness score

### CloudWatch Logs

Key log groups:
- `/aws/lambda/lesser-{env}-ml-training-processor`
- `/aws/lambda/lesser-{env}-graphql`

Key log patterns:
```
"Submitted Bedrock training job"  # Job submission
"Poll request created"             # Poll scheduling
"Job status changed"               # Status updates
"Training job completed"           # Completion
"Created model version"            # Model activation
"Prediction tracked"               # Inference tracking
```

### Alarms

Recommended CloudWatch Alarms:

1. **High failure rate**: `MLTrainingJobsFailed > 3 in 1 hour`
2. **Stuck jobs**: No status change for 2 hours
3. **Low effectiveness**: `MLEffectivenessScore < 0.7 for 24 hours`
4. **Lambda errors**: `ml-training-processor` error rate > 1%

## Troubleshooting

### Job Stuck in IN_PROGRESS

**Symptoms**: Job shows `IN_PROGRESS` for hours, no poll requests being created.

**Diagnosis**:
1. Check CloudWatch Logs for `ml-training-processor`
2. Look for errors in stream processing
3. Verify DynamoDB stream is enabled

**Resolution**:
1. Manually create a poll request:
   ```python
   dynamodb.put_item(
       TableName='lesser-{env}',
       Item={
           'PK': {'S': f'MLPOLL#{job_id}'},
           'SK': {'S': f'REQUEST#{int(time.time())}'},
           'Type': {'S': 'ML_POLL_REQUEST'},
           'JobID': {'S': job_id},
           'RequestTime': {'S': datetime.utcnow().isoformat()},
           'TTL': {'N': str(int(time.time()) + 172800)}
       }
   )
   ```

### Training Fails with "Insufficient Samples"

**Symptoms**: Job fails immediately with error about minimum samples.

**Diagnosis**:
1. Check `minSamples` parameter
2. Query for queued samples: `PK=MLSAMPLE#{tenant_id}`

**Resolution**:
1. Queue more samples using `queueModerationMLSample`
2. Lower `minSamples` (minimum 10 recommended)

### Predictions Not Being Tracked

**Symptoms**: No `MLPrediction` records in DynamoDB.

**Diagnosis**:
1. Check `moderationMLEnabled` feature flag
2. Verify active model version exists
3. Check CloudWatch Logs for `ScoreContent` calls

**Resolution**:
1. Ensure at least one training job has completed
2. Verify model version has `IsActive=true`
3. Check IAM permissions for writing predictions

### Effectiveness Score is 0

**Symptoms**: `effectivenessScore` field shows 0.0.

**Diagnosis**:
1. Check if any predictions have human labels
2. Verify time window (default 7 days)
3. Check for prediction records: `gsi1PK=MODEL#{version}`

**Resolution**:
1. Human reviewers must set labels on moderation actions
2. Wait for sufficient labeled predictions (minimum 10)
3. Call `computeEffectiveness` manually if needed

## Future Enhancements

### Short Term
- [ ] Add SNS notifications for job completion
- [ ] Implement automatic retraining triggers
- [ ] Add A/B testing for model versions
- [ ] Create admin dashboard for model management

### Long Term
- [ ] Multi-model ensemble predictions
- [ ] Active learning sample selection
- [ ] Distributed training for large datasets
- [ ] Real-time model performance monitoring

## References

- [AWS Bedrock Documentation](https://docs.aws.amazon.com/bedrock/)
- [DynamoDB Streams](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Streams.html)
- [Lesser Architecture Docs](./architecture/SYSTEM_DESIGN.md)
- [GraphQL Schema](../graph/phase2.graphql)
