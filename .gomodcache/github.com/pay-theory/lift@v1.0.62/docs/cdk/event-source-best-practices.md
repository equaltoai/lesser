# Event Source Best Practices Guide

This guide helps you select the right AWS event source for your use case and provides best practices for implementation, performance, security, and cost optimization.

## Event Source Selection Matrix

### Quick Decision Guide

| Use Case | Recommended Service | Why |
|----------|-------------------|-----|
| Task queues, job processing | SQS | Reliable delivery, automatic retries, DLQ support |
| Event routing, fan-out | EventBridge | Content-based routing, multiple targets, replay capability |
| File processing | S3 | Direct bucket notifications, large payload support |
| Real-time streaming | Kinesis | High throughput, ordered processing, multiple consumers |
| Pub/Sub messaging | SNS | Simple fan-out, email/SMS support, cross-region |
| Database changes | DynamoDB Streams | Exactly-once processing, change capture |
| Real-time bidirectional | WebSocket API | Persistent connections, low latency |

## Detailed Service Comparison

### SQS (Simple Queue Service)

**Best For:**
- Decoupling microservices
- Background job processing
- Rate limiting and buffering
- Reliable message delivery

**Key Features:**
- At-least-once delivery guarantee
- Automatic retries with exponential backoff
- Dead letter queue support
- FIFO queues for ordering
- Long polling for cost efficiency

**When to Use:**
```go
// Use SQS when you need reliable task processing
sqsProcessor := constructs.NewSQSProcessor(stack, jsii.String("JobQueue"), &constructs.SQSProcessorProps{
    QueueName:        jsii.String("job-queue"),
    FifoQueue:        jsii.Bool(true), // When order matters
    MaxReceiveCount:  jsii.Number(3),   // Retry 3 times before DLQ
    VisibilityTimeout: awscdk.Duration_Minutes(jsii.Number(5)),
})
```

**Best Practices:**
- Set visibility timeout > max processing time
- Use batch operations (up to 10 messages)
- Implement idempotent processing
- Monitor DLQ depth

### EventBridge

**Best For:**
- Event-driven architectures
- Application integration
- Scheduled tasks
- Cross-account events

**Key Features:**
- Content-based routing
- Schema registry
- Event replay
- Archive and replay
- Built-in retry with DLQ

**When to Use:**
```go
// Use EventBridge for complex event routing
// import "github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
eventHandler := constructs.NewEventBridgeHandler(stack, jsii.String("OrderEvents"), &constructs.EventBridgeHandlerProps{
    EventPattern: &awseventbridge.EventPattern{
        Source:     &[]*string{jsii.String("order.service")},
        DetailType: &[]*string{jsii.String("Order Placed"), jsii.String("Order Shipped")},
        Detail: &map[string]interface{}{
            "amount": &map[string]interface{}{
                "numeric": &[]interface{}{">", 100},
            },
        },
    },
})
```

**Best Practices:**
- Use specific event patterns to reduce invocations
- Implement event versioning
- Archive critical events
- Use custom event buses for domain separation

### S3

**Best For:**
- File/object processing
- Data lake ingestion
- Large payload handling
- Batch processing

**Key Features:**
- Direct bucket notifications
- Prefix/suffix filtering
- Multiple event types
- No size limits (object size)
- Cross-region replication events

**When to Use:**
```go
// Use S3 for file-based workflows
s3Processor := constructs.NewS3Processor(stack, jsii.String("DataProcessor"), &constructs.S3ProcessorProps{
    BucketName: jsii.String("data-lake"),
    EventTypes: &[]awss3.EventType{
        awss3.EventType_OBJECT_CREATED,
        awss3.EventType_OBJECT_REMOVED,
    },
    Filters: &[]*awss3.NotificationKeyFilter{
        {Prefix: jsii.String("raw-data/")},
        {Suffix: jsii.String(".csv")},
    },
})
```

**Best Practices:**
- Use lifecycle policies for cost optimization
- Enable versioning for critical data
- Implement multipart upload for large files
- Use S3 Transfer Acceleration for global uploads

### Kinesis

**Best For:**
- Real-time analytics
- Log aggregation
- IoT data streams
- Clickstream analysis

**Key Features:**
- Ordered processing per shard
- Multiple consumers (fan-out)
- Data retention (24h-365d)
- Serverless with on-demand mode
- Kinesis Analytics integration

**When to Use:**
```go
// Use Kinesis for high-throughput streaming
kinesisProcessor := constructs.NewKinesisProcessor(stack, jsii.String("ClickStream"), &constructs.KinesisProcessorProps{
    StreamName:           jsii.String("clickstream"),
    StreamMode:           awskinesis.StreamMode_ON_DEMAND,
    EnableEnhancedFanOut: jsii.Bool(true), // For multiple consumers
    EnableTumblingWindow: jsii.Bool(true),
    TumblingWindowSeconds: jsii.Number(60), // 1-minute windows
})
```

**Best Practices:**
- Use enhanced fan-out for multiple consumers
- Implement checkpointing for exactly-once processing
- Monitor shard metrics for scaling
- Use Kinesis Data Firehose for S3/Redshift delivery

### SNS (Simple Notification Service)

**Best For:**
- Fan-out messaging
- Mobile push notifications
- Email/SMS alerts
- Cross-region messaging

**Key Features:**
- Push-based delivery
- Multiple protocols (HTTP/S, Email, SMS, Lambda)
- Message filtering
- FIFO topics
- Cross-region support

**When to Use:**
```go
// Use SNS for pub/sub patterns
snsProcessor := constructs.NewSNSProcessor(stack, jsii.String("Notifications"), &constructs.SNSProcessorProps{
    TopicName:  jsii.String("user-notifications"),
    EnableFIFO: jsii.Bool(false), // Standard for fan-out
    FilterPolicy: &map[string]interface{}{
        "notificationType": &[]string{"order", "payment"},
        "priority":         &map[string]interface{}{"numeric": &[]interface{}{">=", 3}},
    },
})
```

**Best Practices:**
- Use message attributes for filtering
- Implement retry logic for HTTP endpoints
- Monitor delivery failures
- Use FIFO topics only when ordering is critical

### DynamoDB Streams

**Best For:**
- Change data capture (CDC)
- Real-time data replication
- Audit logging
- Cache invalidation

**Key Features:**
- Exactly-once processing
- Ordered per item
- 24-hour retention
- Multiple stream readers
- Integration with Kinesis adapter

**When to Use:**
```go
// Use DynamoDB Streams for change capture
dynamoProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("UserChanges"), &constructs.DynamoStreamProcessorProps{
    TableName:      jsii.String("users"),
    StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
    ParallelizationFactor: jsii.Number(10), // For high throughput
})
```

**Best Practices:**
- Choose appropriate stream view type
- Implement idempotent processing
- Use parallelization for high throughput
- Monitor iterator age metric

### WebSocket API

**Best For:**
- Real-time chat applications
- Live dashboards
- Collaborative editing
- Gaming backends

**Key Features:**
- Persistent connections
- Bidirectional communication
- Connection management
- Route selection
- Automatic scaling

**When to Use:**
```go
// Use WebSocket for real-time features
wsApi := constructs.NewWebSocketAPI(stack, jsii.String("ChatAPI"), &constructs.WebSocketAPIProps{
    APIName:                    jsii.String("chat-api"),
    EnableConnectionManagement: jsii.Bool(true),
    RouteSelectionExpression:   jsii.String("$request.body.action"),
})
```

**Best Practices:**
- Implement connection pooling
- Use connection table for state management
- Handle connection lifecycle events
- Implement heartbeat/ping-pong

## Performance Optimization

### General Principles

1. **Batch Processing**
   - Process multiple records per invocation
   - Reduces Lambda cold starts
   - Improves throughput

2. **Concurrent Processing**
   - Configure appropriate concurrency limits
   - Use parallelization factors
   - Monitor throttling

3. **Memory and Timeout**
   - Right-size Lambda memory
   - Set appropriate timeouts
   - Monitor duration metrics

### Service-Specific Optimizations

#### SQS Optimization
```go
// Optimize for throughput
EventSourceProps: &awslambda.SqsEventSourceProps{
    BatchSize:                     jsii.Number(10),
    MaxBatchingWindowInSeconds:    jsii.Number(20),
    MaxConcurrency:               jsii.Number(100),
    ReportBatchItemFailures:      jsii.Bool(true),
}
```

#### Kinesis Optimization
```go
// Optimize for real-time processing
EventSourceProps: &awslambda.KinesisEventSourceProps{
    BatchSize:              jsii.Number(100),
    ParallelizationFactor:  jsii.Number(10),
    StartingPosition:       awslambda.StartingPosition_LATEST,
    BisectBatchOnError:    jsii.Bool(true),
}
```

#### EventBridge Optimization
```go
// Optimize event patterns
EventPattern: &awseventbridge.EventPattern{
    Source: &[]*string{jsii.String("specific.source")}, // Be specific
    Account: &[]*string{jsii.String("123456789012")},   // Reduce scope
    Region: &[]*string{jsii.String("us-east-1")},       // Limit regions
}
```

## Security Best Practices

### Encryption
- Enable encryption at rest for all services
- Use KMS keys for sensitive data
- Rotate encryption keys regularly

### Access Control
```go
// Implement least privilege
function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
    Actions:   &[]*string{jsii.String("dynamodb:PutItem")},
    Resources: &[]*string{table.TableArn()},
    Conditions: &map[string]interface{}{
        "StringEquals": &map[string]*string{
            "dynamodb:LeadingKeys": jsii.String("${cognito-identity.amazonaws.com:sub}"),
        },
    },
}))
```

### Network Security
- Use VPC endpoints for private communication
- Implement API throttling
- Use WAF for API Gateway

### Data Protection
- Redact sensitive data in logs
- Implement field-level encryption
- Use secure transport (TLS)

## Cost Optimization

### Service Selection Impact

| Service | Cost Model | Optimization Tips |
|---------|-----------|-------------------|
| SQS | Per request | Use long polling, batch operations |
| EventBridge | Per event | Filter events early, use archive selectively |
| S3 | Storage + requests | Use lifecycle policies, intelligent tiering |
| Kinesis | Per shard hour/data | Use on-demand mode, compress data |
| SNS | Per notification | Filter messages, batch when possible |
| DynamoDB Streams | Included with table | Use efficient stream processing |
| WebSocket | Per connection minute | Implement idle timeouts |

### Cost Optimization Strategies

1. **Right-size Resources**
```go
// Use on-demand for variable workloads
streamProps := &awskinesis.StreamProps{
    StreamMode: awskinesis.StreamMode_ON_DEMAND,
}

// Use provisioned for predictable workloads
streamProps := &awskinesis.StreamProps{
    StreamMode: awskinesis.StreamMode_PROVISIONED,
    ShardCount: jsii.Number(2),
}
```

2. **Implement Efficient Filtering**
```go
// Filter at the source to reduce invocations
filterPolicy := &map[string]interface{}{
    "eventType": &[]string{"ORDER_CREATED"}, // Only process specific events
    "amount": &map[string]interface{}{
        "numeric": &[]interface{}{">", 100}, // Only high-value orders
    },
}
```

3. **Use Appropriate Retention**
```go
// Set retention based on requirements
queueProps := &awssqs.QueueProps{
    RetentionPeriod: awscdk.Duration_Days(jsii.Number(4)), // Reduce from default 14
}
```

## Monitoring and Observability

### Key Metrics by Service

#### SQS Metrics
- ApproximateNumberOfMessagesVisible
- ApproximateAgeOfOldestMessage
- NumberOfMessagesReceived
- NumberOfMessagesSent

#### EventBridge Metrics
- SuccessfulRuleMatches
- FailedInvocations
- TriggeredRules
- Latency

#### Kinesis Metrics
- IncomingRecords
- IteratorAgeMilliseconds
- ReadProvisionedThroughputExceeded
- UserRecordsPerSecond

### Implementing Monitoring
```go
// Create CloudWatch alarms
cloudwatch.NewAlarm(stack, jsii.String("DLQAlarm"), &cloudwatch.AlarmProps{
    Metric: dlq.MetricApproximateNumberOfMessagesVisible(),
    Threshold: jsii.Number(10),
    EvaluationPeriods: jsii.Number(1),
})

// Add X-Ray tracing
functionProps := &constructs.LiftFunctionProps{
    EnableTracing: jsii.Bool(true),
}
```

## Error Handling Strategies

### Retry Strategies

1. **Exponential Backoff**
```go
retryPolicy := &awseventbridge.RetryPolicy{
    MaximumRetryAttempts: jsii.Number(3),
    MaximumEventAge:      awscdk.Duration_Hours(jsii.Number(1)),
}
```

2. **Dead Letter Queues**
```go
dlqProps := &constructs.DeadLetterQueueProps{
    MaxReceiveCount: jsii.Number(3),
    RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
}
```

3. **Bisect on Error**
```go
// For stream processing
eventSourceProps := &awslambda.KinesisEventSourceProps{
    BisectBatchOnError: jsii.Bool(true),
    OnFailure: dlq,
}
```

### Error Recovery Patterns

1. **Circuit Breaker**
```go
// Implement circuit breaker pattern
type CircuitBreaker struct {
    failureCount int
    threshold    int
    timeout      time.Duration
    lastFailure  time.Time
}
```

2. **Poison Message Handling**
```go
// Move problematic messages to DLQ
if processAttempts > maxAttempts {
    sendToDLQ(message)
    return nil // Don't retry
}
```

## Migration Strategies

### Migrating Between Event Sources

1. **Dual Publishing**
   - Publish to both old and new sources
   - Migrate consumers gradually
   - Monitor both systems

2. **Blue-Green Deployment**
   - Deploy new event source
   - Switch traffic using feature flags
   - Keep old system as fallback

3. **Replay and Catch-up**
   - Use EventBridge archive/replay
   - Process historical events
   - Ensure idempotency

## Common Patterns

### Event Sourcing
```go
// Use DynamoDB Streams for event sourcing
eventStore := constructs.NewDynamoStreamProcessor(stack, jsii.String("EventStore"), &constructs.DynamoStreamProcessorProps{
    TableName:      jsii.String("events"),
    StreamViewType: awsdynamodb.StreamViewType_NEW_IMAGE,
})
```

### CQRS (Command Query Responsibility Segregation)
```go
// Separate read and write models
writeApi := patterns.NewEventDrivenAPI(stack, jsii.String("WriteAPI"), writeProps)
readModel := constructs.NewDynamoStreamProcessor(stack, jsii.String("ReadModel"), readProps)
```

### Saga Pattern
```go
// Orchestrate distributed transactions
orchestrator := patterns.NewEventOrchestrator(stack, jsii.String("SagaOrchestrator"), &patterns.EventOrchestratorProps{
    EnableSagaPattern:      jsii.Bool(true),
    EnableEventCorrelation: jsii.Bool(true),
})
```

## Troubleshooting Guide

### Common Issues and Solutions

1. **High Lambda Concurrency**
   - Set reserved concurrent executions
   - Use SQS as a buffer
   - Implement rate limiting

2. **Message Loss**
   - Enable DLQ on all event sources
   - Implement message deduplication
   - Use FIFO queues when ordering matters

3. **Cost Overruns**
   - Review polling configuration
   - Optimize batch sizes
   - Implement efficient filtering

4. **Latency Issues**
   - Use enhanced fan-out for Kinesis
   - Optimize Lambda memory
   - Consider regional deployment

## Summary

Choose your event source based on:
1. **Delivery semantics** (at-least-once vs exactly-once)
2. **Ordering requirements**
3. **Latency requirements**
4. **Scale and throughput needs**
5. **Cost constraints**
6. **Integration requirements**

Remember:
- Start simple, evolve as needed
- Monitor everything
- Plan for failure
- Optimize for your specific use case