# Event Processing Cost Optimization Guide

This guide provides strategies for optimizing costs in event-driven architectures while maintaining performance and reliability.

## Cost Model Overview

### AWS Service Pricing Components

> **Note**: Pricing information below should be verified against current AWS pricing pages as rates may have changed.

| Service | Pricing Dimensions | Free Tier |
|---------|-------------------|-----------|
| Lambda | Requests + GB-seconds | 1M requests, 400K GB-s/month |
| SQS | Requests (per million) | 1M requests/month |
| EventBridge | Events published | 1M events/month |
| S3 | Storage + Requests + Transfer | 5GB storage, 20K GET, 2K PUT |
| Kinesis | Shard hours + PUT records | None |
| SNS | Notifications + Data transfer | 1M mobile push, 1K email |
| DynamoDB | RCU/WCU or On-Demand + Storage | 25GB storage, 25 RCU/WCU |

## Lambda Cost Optimization

### Right-Sizing Memory

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/lift/cdk/constructs"
)

// Cost-efficient memory configuration
// Use AWS Lambda Power Tuning to find optimal memory
functionProps := &constructs.LiftFunctionProps{
    MemorySize: jsii.Number(512), // Often most cost-effective
    Timeout:    awscdk.Duration_Seconds(jsii.Number(30)),
    Architecture: awslambda.Architecture_ARM_64(), // 20% cheaper than x86
}
```

### Reduce Invocation Count

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/lift/cdk/constructs"
)

// Batch processing to reduce invocations
sqsProcessor := constructs.NewSQSProcessor(stack, jsii.String("BatchProcessor"), &constructs.SQSProcessorProps{
    EventSourceProps: &awslambdaeventsources.SqsEventSourceProps{
        BatchSize:                  jsii.Number(10),    // Process 10 messages per invocation (default)
        MaxBatchingWindowInSeconds: jsii.Number(5),    // Wait up to 5s to fill batch (default)
    },
})
```

### Optimize Function Duration

```go
import (
    "context"
    "database/sql"
    "net/http"
    "os"
    "time"
)

// Pre-initialize expensive operations
var (
    httpClient *http.Client
    dbConn     *sql.DB
)

func init() {
    // Initialize outside handler to reuse across invocations
    httpClient = &http.Client{Timeout: 10 * time.Second}
    dbConn, _ = sql.Open("postgres", os.Getenv("DB_URL"))
}

func handler(ctx context.Context, event Event) error {
    // Reuse initialized resources
    return processEvent(event, httpClient, dbConn)
}
```

## SQS Cost Optimization

### Long Polling

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
    "github.com/aws/jsii-runtime-go"
)

// Reduce API calls with long polling
queueProps := &awssqs.QueueProps{
    QueueName: jsii.String("cost-optimized-queue"),
    ReceiveMessageWaitTimeSeconds: jsii.Number(20), // Max long polling
}
```

### Batch Operations

```go
import (
    "context"
    "fmt"
    
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/sqs"
    "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// Send messages in batches
func sendMessagesBatch(queueURL string, messages []Message) error {
    svc := sqs.NewFromConfig(cfg)
    
    // Batch up to 10 messages
    for i := 0; i < len(messages); i += 10 {
        end := i + 10
        if end > len(messages) {
            end = len(messages)
        }
        
        batch := messages[i:end]
        entries := make([]types.SendMessageBatchRequestEntry, len(batch))
        
        for j, msg := range batch {
            entries[j] = types.SendMessageBatchRequestEntry{
                Id:          aws.String(fmt.Sprintf("%d", j)),
                MessageBody: aws.String(msg.Body),
            }
        }
        
        _, err := svc.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
            QueueUrl: aws.String(queueURL),
            Entries:  entries,
        })
        
        if err != nil {
            return err
        }
    }
    
    return nil
}
```

### Message Deduplication

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/lift/cdk/constructs"
)

// For FIFO queues, use content-based deduplication
fifoQueue := constructs.NewSQSProcessor(stack, jsii.String("FIFOQueue"), &constructs.SQSProcessorProps{
    FifoQueue: jsii.Bool(true),
    QueueProps: &awssqs.QueueProps{
        ContentBasedDeduplication: jsii.Bool(true), // Automatic deduplication
        DeduplicationScope:       awssqs.DeduplicationScope_QUEUE,
    },
})
```

## EventBridge Cost Optimization

### Efficient Event Patterns

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/lift/cdk/constructs"
)

// Specific patterns reduce rule evaluations
eventHandler, err := constructs.NewEventBridgeHandler(stack, jsii.String("EfficientHandler"), &constructs.EventBridgeHandlerProps{
    EventPattern: &awsevents.EventPattern{
        Source:     &[]*string{jsii.String("order.service")},     // Specific source
        DetailType: &[]*string{jsii.String("Order Placed")},     // Specific type
        Account:    &[]*string{jsii.String("123456789012")},     // Specific account
        Detail: &map[string]interface{}{
            "orderValue": &map[string]interface{}{
                "numeric": &[]interface{}{">", 100},              // Only high-value orders
            },
        },
    },
})
if err != nil {
    // Handle error appropriately
    panic(err)
}
```

### Archive Strategy

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
)

// Archive selectively to control costs
archive := awsevents.NewArchive(stack, jsii.String("CriticalEventsArchive"), &awsevents.ArchiveProps{
    ArchiveName: jsii.String("critical-events"),
    EventPattern: &awsevents.EventPattern{
        DetailType: &[]*string{
            jsii.String("Payment Failed"),
            jsii.String("Order Cancelled"),
        },
    },
    Retention: awscdk.Duration_Days(jsii.Number(7)), // Short retention
})
```

## S3 Cost Optimization

### Storage Classes

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awss3"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/lift/cdk/constructs"
)

s3Processor := constructs.NewS3Processor(stack, jsii.String("OptimizedStorage"), &constructs.S3ProcessorProps{
    BucketProps: &awss3.BucketProps{
        LifecycleRules: &[]*awss3.LifecycleRule{
            {
                Id:      jsii.String("TransitionRule"),
                Enabled: jsii.Bool(true),
                Transitions: &[]*awss3.Transition{
                    {
                        StorageClass: awss3.StorageClass_INFREQUENT_ACCESS,
                        TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
                    },
                    {
                        StorageClass: awss3.StorageClass_GLACIER,
                        TransitionAfter: awscdk.Duration_Days(jsii.Number(90)),
                    },
                },
            },
        },
        IntelligentTieringConfigurations: &[]*awss3.IntelligentTieringConfiguration{
            {
                Name: jsii.String("auto-tiering"),
                ArchiveAccessTierTime: awscdk.Duration_Days(jsii.Number(90)),
                DeepArchiveAccessTierTime: awscdk.Duration_Days(jsii.Number(180)),
            },
        },
    },
})
```

### Request Optimization

```go
// Minimize S3 requests
func processS3Events(events []S3Event) error {
    // Group by bucket for batch operations
    bucketGroups := make(map[string][]S3Event)
    for _, event := range events {
        bucket := event.Bucket
        bucketGroups[bucket] = append(bucketGroups[bucket], event)
    }
    
    // Process each bucket group with multi-object operations
    for bucket, group := range bucketGroups {
        if err := batchProcessBucket(bucket, group); err != nil {
            return err
        }
    }
    
    return nil
}
```

## Kinesis Cost Optimization

### On-Demand vs Provisioned

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2/awskinesis"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/lift/cdk/constructs"
)

// Use on-demand for variable workloads
kinesisProcessor := constructs.NewKinesisProcessor(stack, jsii.String("OnDemandStream"), &constructs.KinesisProcessorProps{
    FunctionProps: &constructs.LiftFunctionProps{
        FunctionProps: awslambda.FunctionProps{
            Code:    awslambda.Code_FromAsset(jsii.String("lambda")),
            Handler: jsii.String("main"),
        },
    },
    StreamMode: awskinesis.StreamMode_ON_DEMAND, // Pay per GB
})

// Use provisioned for predictable workloads
kinesisProcessor := constructs.NewKinesisProcessor(stack, jsii.String("ProvisionedStream"), &constructs.KinesisProcessorProps{
    FunctionProps: &constructs.LiftFunctionProps{
        FunctionProps: awslambda.FunctionProps{
            Code:    awslambda.Code_FromAsset(jsii.String("lambda")),
            Handler: jsii.String("main"),
        },
    },
    StreamMode: awskinesis.StreamMode_PROVISIONED,
    ShardCount: jsii.Number(1), // Start small, scale as needed
})
```

### Compression

```go
import (
    "bytes"
    "compress/gzip"
    "context"
    "encoding/json"
    
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/kinesis"
)

// Compress data to reduce PUT payload charges
func putRecordCompressed(streamName string, data interface{}) error {
    jsonData, _ := json.Marshal(data)
    
    var buf bytes.Buffer
    gz := gzip.NewWriter(&buf)
    gz.Write(jsonData)
    gz.Close()
    
    _, err := kinesisClient.PutRecord(ctx, &kinesis.PutRecordInput{
        StreamName:   aws.String(streamName),
        Data:         buf.Bytes(),
        PartitionKey: aws.String(generatePartitionKey(data)),
    })
    
    return err
}
```

### Aggregation

```go
// Use Kinesis Producer Library patterns for aggregation
type RecordAggregator struct {
    records    [][]byte
    totalSize  int
    maxSize    int
    maxRecords int
}

func (a *RecordAggregator) AddRecord(data []byte) ([]byte, bool) {
    if len(a.records) >= a.maxRecords || a.totalSize+len(data) > a.maxSize {
        aggregated := a.Flush()
        a.records = [][]byte{data}
        a.totalSize = len(data)
        return aggregated, true
    }
    
    a.records = append(a.records, data)
    a.totalSize += len(data)
    return nil, false
}
```

## DynamoDB Cost Optimization

### On-Demand vs Provisioned

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
)

// Decision matrix for DynamoDB billing mode
func chooseBillingMode(avgRPS, peakRPS float64) awsdynamodb.BillingMode {
    utilizationRatio := avgRPS / peakRPS
    
    if utilizationRatio < 0.18 {
        return awsdynamodb.BillingMode_PAY_PER_REQUEST // On-demand
    }
    return awsdynamodb.BillingMode_PROVISIONED
}

// Implementation
table := awsdynamodb.NewTable(stack, jsii.String("CostOptimizedTable"), &awsdynamodb.TableProps{
    TableName:    jsii.String("events"),
    BillingMode:  chooseBillingMode(100, 1000),
    PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
})
```

### Auto-Scaling Configuration

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
    "github.com/aws/jsii-runtime-go"
)

// Configure auto-scaling for provisioned capacity
readScaling := table.AutoScaleReadCapacity(&awsdynamodb.EnableScalingProps{
    MinCapacity: jsii.Number(5),
    MaxCapacity: jsii.Number(40000),
})

readScaling.ScaleOnUtilization(&awsdynamodb.UtilizationScalingProps{
    TargetUtilizationPercent: jsii.Number(70),
    ScaleInCooldown:          awscdk.Duration_Minutes(jsii.Number(5)),
    ScaleOutCooldown:         awscdk.Duration_Seconds(jsii.Number(60)),
})
```

## Cost Monitoring and Alerts

### CloudWatch Cost Alerts

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
    "github.com/aws/jsii-runtime-go"
)

// Create cost anomaly detector
costAlarm := awscloudwatch.NewAlarm(stack, jsii.String("CostAlarm"), &awscloudwatch.AlarmProps{
    Metric: awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
        Namespace:  jsii.String("AWS/Billing"),
        MetricName: jsii.String("EstimatedCharges"),
        DimensionsMap: &map[string]*string{
            "Currency": jsii.String("USD"),
        },
    }),
    Threshold:         jsii.Number(100), // Alert at $100
    EvaluationPeriods: jsii.Number(1),
})
```

### Resource Tagging

```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/jsii-runtime-go"
)

// Tag resources for cost allocation
awscdk.Tags_Of(sqsProcessor).Add(jsii.String("Environment"), jsii.String("Production"))
awscdk.Tags_Of(sqsProcessor).Add(jsii.String("Team"), jsii.String("Orders"))
awscdk.Tags_Of(sqsProcessor).Add(jsii.String("CostCenter"), jsii.String("CC-123"))
```

## Cost Optimization Patterns

### Circuit Breaker for Cost Control

```go
import (
    "sync"
    "time"
)

type CostCircuitBreaker struct {
    maxCostPerHour float64
    currentCost    float64
    resetTime      time.Time
    mu             sync.Mutex
}

func (cb *CostCircuitBreaker) CanProceed(estimatedCost float64) bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    if time.Now().After(cb.resetTime) {
        cb.currentCost = 0
        cb.resetTime = time.Now().Add(time.Hour)
    }
    
    if cb.currentCost+estimatedCost > cb.maxCostPerHour {
        return false
    }
    
    cb.currentCost += estimatedCost
    return true
}
```

### Tiered Processing

```go
// Process based on value/priority to optimize costs
func routeEventByCost(event Event) string {
    switch {
    case event.Value > 1000:
        return "high-priority-queue"    // Immediate processing
    case event.Value > 100:
        return "standard-queue"          // Standard processing
    default:
        return "batch-queue"             // Batch for efficiency
    }
}
```

## Cost Analysis Tools

### Lambda Cost Calculator

```go
func calculateLambdaCost(invocations, avgDurationMs, memoryMB int64) float64 {
    const (
        requestCost  = 0.0000002  // per request
        gbSecondCost = 0.0000166667 // per GB-second
    )
    
    requests := float64(invocations)
    gbSeconds := float64(invocations) * float64(avgDurationMs) / 1000.0 * float64(memoryMB) / 1024.0
    
    return (requests * requestCost) + (gbSeconds * gbSecondCost)
}
```

### Event Source Cost Comparison

> **Note**: The pricing rates below are examples and should be verified against current AWS pricing.

```go
type EventSourceCost struct {
    Service         string
    MonthlyEvents   int64
    MonthlyCost     float64
    CostPerMillion  float64
}

func compareEventSourceCosts(monthlyEvents int64) []EventSourceCost {
    return []EventSourceCost{
        {
            Service:        "SQS Standard",
            MonthlyEvents:  monthlyEvents,
            MonthlyCost:    float64(monthlyEvents) / 1000000 * 0.40,
            CostPerMillion: 0.40,
        },
        {
            Service:        "EventBridge",
            MonthlyEvents:  monthlyEvents,
            MonthlyCost:    float64(monthlyEvents) / 1000000 * 1.00,
            CostPerMillion: 1.00,
        },
        {
            Service:        "Kinesis (1 shard)",
            MonthlyEvents:  monthlyEvents,
            MonthlyCost:    36.00, // Fixed shard cost
            CostPerMillion: 36.00 / (float64(monthlyEvents) / 1000000),
        },
    }
}
```

## Cost Optimization Checklist

### Design Phase
- [ ] Choose appropriate event source based on volume
- [ ] Design for batch processing where possible
- [ ] Plan data retention policies
- [ ] Consider multi-region costs
- [ ] Evaluate on-demand vs provisioned options

### Implementation Phase
- [ ] Implement efficient serialization (Protocol Buffers, MessagePack)
- [ ] Use compression for large payloads
- [ ] Configure appropriate batch sizes
- [ ] Implement connection pooling
- [ ] Cache frequently accessed data

### Operational Phase
- [ ] Monitor cost anomalies
- [ ] Review and optimize memory allocation
- [ ] Analyze CloudWatch Logs Insights for patterns
- [ ] Regularly review data lifecycle policies
- [ ] Optimize based on actual usage patterns

## Common Cost Pitfalls

### 1. Over-Provisioning
```go
import (
    "github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
)

// Bad: Fixed high capacity
table := awsdynamodb.NewTable(stack, jsii.String("OverProvisionedTable"), &awsdynamodb.TableProps{
    BillingMode:   awsdynamodb.BillingMode_PROVISIONED,
    ReadCapacity:  jsii.Number(1000),  // Paying for unused capacity
    WriteCapacity: jsii.Number(1000),
})

// Good: Auto-scaling based on demand
table := awsdynamodb.NewTable(stack, jsii.String("AutoScaledTable"), &awsdynamodb.TableProps{
    BillingMode: awsdynamodb.BillingMode_PAY_PER_REQUEST, // Or auto-scaled provisioned
})
```

### 2. Inefficient Polling
```go
import (
    "context"
    "time"
    
    "github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Bad: Frequent short polling
for {
    messages, _ := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
        QueueUrl: queueURL,
        WaitTimeSeconds: 0, // Short polling
    })
    time.Sleep(1 * time.Second)
}

// Good: Long polling
messages, _ := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
    QueueUrl: queueURL,
    WaitTimeSeconds: 20, // Long polling
})
```

### 3. Lambda Memory Waste
```go
import (
    "github.com/aws/jsii-runtime-go"
    "github.com/lift/cdk/constructs"
)

// Bad: Over-allocated memory
functionProps := &constructs.LiftFunctionProps{
    MemorySize: jsii.Number(3008), // Paying for unused memory
}

// Good: Right-sized based on profiling
functionProps := &constructs.LiftFunctionProps{
    MemorySize: jsii.Number(512), // Optimal for workload
}
```

## ROI Calculations

### Cost vs Performance Trade-offs

```go
type OptimizationROI struct {
    Strategy           string
    MonthlySavings     float64
    ImplementationCost float64
    PaybackMonths      float64
}

func calculateOptimizationROI() []OptimizationROI {
    return []OptimizationROI{
        {
            Strategy:           "Lambda ARM migration",
            MonthlySavings:     2000, // 20% cost reduction
            ImplementationCost: 5000, // Dev time
            PaybackMonths:      2.5,
        },
        {
            Strategy:           "S3 Intelligent Tiering",
            MonthlySavings:     500,
            ImplementationCost: 500,
            PaybackMonths:      1.0,
        },
    }
}
```

## Summary

Key cost optimization strategies:
1. **Measure First**: Understand your baseline costs
2. **Right-Size Resources**: Match capacity to actual demand
3. **Batch Operations**: Reduce per-request overhead
4. **Choose Wisely**: Select services based on usage patterns
5. **Monitor Continuously**: Set up cost alerts and anomaly detection
6. **Optimize Iteratively**: Regular review and adjustment