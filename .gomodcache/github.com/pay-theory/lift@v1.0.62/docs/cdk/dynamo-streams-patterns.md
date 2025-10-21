# DynamoDB Streams Patterns Guide

This guide provides comprehensive patterns and examples for using the `DynamoStreamProcessor` construct to build event-driven applications with DynamoDB Streams.

## Table of Contents

1. [Overview](#overview)
2. [Basic Usage](#basic-usage)
3. [Configuration Options](#configuration-options)
4. [Event Processing Patterns](#event-processing-patterns)
5. [Performance Optimization](#performance-optimization)
6. [Error Handling](#error-handling)
7. [Best Practices](#best-practices)
8. [Advanced Patterns](#advanced-patterns)
9. [Troubleshooting](#troubleshooting)

## Overview

The `DynamoStreamProcessor` construct creates a complete DynamoDB Streams processing pipeline with:

- **DynamoDB Table** with streams enabled
- **Lambda Function** optimized for stream processing
- **Event Source Mapping** with configurable settings
- **Dead Letter Queue** for failed processing
- **IAM Permissions** automatically configured
- **Environment Variables** for runtime integration

### Key Features

- **Stream View Types**: Support for all DynamoDB stream views (KEYS_ONLY, NEW_IMAGE, OLD_IMAGE, NEW_AND_OLD_IMAGES)
- **Retry Logic**: Configurable retry attempts and bisect batch on error
- **Parallelization**: Control processing parallelization factor
- **Tumbling Windows**: Support for time-based aggregation
- **Auto-scaling**: Optional table auto-scaling configuration
- **Multi-tenant**: Built-in multi-tenant support

## Basic Usage

### Default Configuration

```go
import "github.com/pay-theory/lift/pkg/cdk/constructs"

// Minimal configuration - creates table with streams and processor
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("OrderProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("order-stream-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
})
```

**Default Settings:**
- Table: Pay-per-request billing, NEW_AND_OLD_IMAGES stream
- Processing: Batch size 10, starting position LATEST
- Error handling: Dead letter queue enabled, 10000 retry attempts

### Using Custom Table Configuration

```go
// Configure streaming table with custom properties
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("OrderProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("order-stream-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    StreamingTableProps: &constructs.StreamingTableProps{
        TableName: jsii.String("orders"),
        StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
        EnableAutoScaling: jsii.Bool(true),
    },
})
```

## Configuration Options

### Stream Configuration

```go
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("Processor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("stream-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    
    // Stream settings
    StreamViewType: awsdynamodb.StreamViewType_KEYS_ONLY,
    StartingPosition: awslambda.StartingPosition_TRIM_HORIZON,
    
    // Processing settings
    BatchSize: jsii.Number(25),
    MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(10)),
    ParallelizationFactor: jsii.Number(2),
    
    // Error handling
    BisectBatchOnError: jsii.Bool(true),
    RetryAttempts: jsii.Number(100),
    MaxRecordAge: awscdk.Duration_Hours(jsii.Number(2)),
})
```

### Table Configuration

```go
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("Processor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    
    // Custom table configuration
    StreamingTableProps: &constructs.StreamingTableProps{
        TableName: jsii.String("events"),
        StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
        EnableAutoScaling: jsii.Bool(true),
    },
})
```

## Event Processing Patterns

### Real-time Analytics

```go
// Process streams for real-time analytics
analyticsProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("AnalyticsProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("analytics-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./analytics-dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
        MemorySize:   jsii.Number(1024),
    },
    
    // High throughput settings
    BatchSize: jsii.Number(100),
    ParallelizationFactor: jsii.Number(10),
    MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(5)),
    
    // Stream view for analytics
    StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
})
```

### Change Data Capture (CDC)

```go
// Capture changes for downstream systems
cdcProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("CDCProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("cdc-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./cdc-dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        Environment: &map[string]*string{
            "TARGET_QUEUE_URL": targetQueue.QueueUrl(),
            "WEBHOOK_URL":      jsii.String("https://api.example.com/webhook"),
        },
    },
    
    // Reliable processing settings
    StartingPosition: awslambda.StartingPosition_TRIM_HORIZON,
    BisectBatchOnError: jsii.Bool(true),
    RetryAttempts: jsii.Number(1000),
    
    // Enable comprehensive monitoring
    EnableMonitoring: jsii.Bool(true),
})

// Grant permissions to send to target queue
targetQueue.GrantSendMessages(cdcProcessor.Function.Function)
```

### Event Sourcing

```go
// Event sourcing with DynamoDB Streams
eventProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("EventProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("event-sourcing-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./event-dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    
    // Event sourcing table
    StreamingTableProps: &constructs.StreamingTableProps{
        TableName: jsii.String("event-store"),
        StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
    },
    
    // Start from beginning for event replay
    StartingPosition: awslambda.StartingPosition_TRIM_HORIZON,
})
```

## Performance Optimization

### High Throughput Processing

```go
// Optimized for high throughput
highThroughputProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("HighThroughputProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("high-throughput-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        
        // Optimized Lambda settings
        MemorySize: jsii.Number(3008),  // Maximum memory
        Timeout:    awscdk.Duration_Minutes(jsii.Number(15)),  // Maximum timeout
        ReservedConcurrentExecutions: jsii.Number(100),
    },
    
    // Maximize parallelization
    ParallelizationFactor: jsii.Number(10),  // Maximum parallelization
    BatchSize: jsii.Number(1000),  // Large batch size
    MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(20)),
    
    // Monitoring for performance tuning
    EnableMonitoring: jsii.Bool(true),
})
```

### Batch Processing with Tumbling Windows

```go
// Process records in time-based batches
batchProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("BatchProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("batch-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./batch-dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    
    // Tumbling window for aggregation
    TumblingWindow: awscdk.Duration_Minutes(jsii.Number(5)),
    BatchSize: jsii.Number(500),
})
```

## Error Handling

### Custom Dead Letter Queue

```go
// Custom DLQ configuration
customDLQ := awssqs.NewQueue(stack, jsii.String("CustomDLQ"), &awssqs.QueueProps{
    QueueName: jsii.String("stream-processing-dlq"),
    RetentionPeriod: awscdk.Duration_Days(jsii.Number(30)),
    VisibilityTimeout: awscdk.Duration_Minutes(jsii.Number(10)),
})

processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("Processor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    
    DeadLetterQueueProps: &awssqs.QueueProps{
        Queue: customDLQ,
    },
    
    // Error handling configuration
    BisectBatchOnError: jsii.Bool(true),
    RetryAttempts: jsii.Number(5),
    MaxRecordAge: awscdk.Duration_Hours(jsii.Number(1)),
})
```

### Disabled Error Handling

```go
// Disable DLQ for simple processing
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("SimpleProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("simple-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    
    EnableDeadLetterQueue: jsii.Bool(false),
    RetryAttempts: jsii.Number(0),  // No retries
})
```

## Best Practices

### 1. Lambda Function Configuration

```go
// Production-ready Lambda configuration
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("ProductionProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("production-stream-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        
        // Right-sized for your workload
        MemorySize: jsii.Number(1024),
        Timeout:    awscdk.Duration_Minutes(jsii.Number(5)),
        
        // Environment variables for configuration
        Environment: &map[string]*string{
            "LOG_LEVEL":     jsii.String("INFO"),
            "BATCH_SIZE":    jsii.String("25"),
            "MAX_RETRIES":   jsii.String("3"),
        },
    },
    
    // Optimized for your use case
    BatchSize: jsii.Number(25),
    ParallelizationFactor: jsii.Number(2),
    
    // Enable Lift features
    EnableTracing: jsii.Bool(true),
    EnableMultiTenant: jsii.Bool(true),
    EnableMonitoring: jsii.Bool(true),
})
```

### 2. Monitoring and Observability

```go
// Add custom CloudWatch alarms
import "github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"

// Create processor
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("MonitoredProcessor"), &constructs.DynamoStreamProcessorProps{
    // ... configuration
    EnableMonitoring: jsii.Bool(true),
})

// Add custom metrics and alarms
errorAlarm := awscloudwatch.NewAlarm(stack, jsii.String("ProcessorErrorAlarm"), &awscloudwatch.AlarmProps{
    AlarmName: jsii.String("stream-processor-errors"),
    Metric: processor.Function.Function.MetricErrors(&awscloudwatch.MetricOptions{
        Period: awscdk.Duration_Minutes(jsii.Number(5)),
        Statistic: jsii.String("Sum"),
    }),
    Threshold: jsii.Number(10),
    EvaluationPeriods: jsii.Number(2),
})

latencyAlarm := awscloudwatch.NewAlarm(stack, jsii.String("ProcessorLatencyAlarm"), &awscloudwatch.AlarmProps{
    AlarmName: jsii.String("stream-processor-latency"),
    Metric: processor.Function.Function.MetricDuration(&awscloudwatch.MetricOptions{
        Period: awscdk.Duration_Minutes(jsii.Number(5)),
        Statistic: jsii.String("Average"),
    }),
    Threshold: jsii.Number(30000), // 30 seconds
    EvaluationPeriods: jsii.Number(3),
})
```

### 3. Security Configuration

```go
// Secure configuration with encryption
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("SecureProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("secure-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    
    // Encrypted streams
    StreamingTableProps: &constructs.StreamingTableProps{
        StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
    },
    
    // Secure DLQ
    DeadLetterQueueProps: &awssqs.QueueProps{
        Encryption: awssqs.QueueEncryption_KMS_MANAGED,
    },
})

// Add additional security policies
processor.Function.Function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
    Effect: awsiam.Effect_DENY,
    Actions: &[]*string{
        jsii.String("logs:CreateLogGroup"),
        jsii.String("logs:CreateLogStream"),
    },
    Resources: &[]*string{jsii.String("*")},
    Conditions: &map[string]interface{}{
        "StringNotEquals": map[string]interface{}{
            "aws:RequestedRegion": "us-east-1",
        },
    },
}))
```

## Advanced Patterns

### Multi-Region Stream Processing

```go
// Primary region processor
primaryProcessor := constructs.NewDynamoStreamProcessor(primaryStack, jsii.String("PrimaryProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("primary-stream-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        Environment: &map[string]*string{
            "REGION":           jsii.String("us-east-1"),
            "CROSS_REGION_QUEUE": crossRegionQueue.QueueUrl(),
        },
    },
    StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
})

// Disaster recovery region processor
drProcessor := constructs.NewDynamoStreamProcessor(drStack, jsii.String("DRProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("dr-stream-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        Environment: &map[string]*string{
            "REGION":     jsii.String("us-west-2"),
            "IS_REPLICA": jsii.String("true"),
        },
    },
    StartingPosition: awslambda.StartingPosition_TRIM_HORIZON,
})
```

### Fan-out Processing

```go
// Create multiple processors for different use cases
func createFanOutProcessors(stack awscdk.Stack) {
    
    // Analytics processor
    analyticsProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("AnalyticsProcessor"), &constructs.DynamoStreamProcessorProps{
        FunctionProps: awslambda.FunctionProps{
            FunctionName: jsii.String("analytics-processor"),
            Code:         awslambda.Code_FromAsset(jsii.String("./analytics-dist")),
            Handler:      jsii.String("bootstrap"),
            Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        },
        StreamingTableProps: &constructs.StreamingTableProps{
            TableName: jsii.String("analytics-table"),
            StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
        },
        BatchSize: jsii.Number(100),
        ParallelizationFactor: jsii.Number(5),
    })
    
    // Notification processor
    notificationProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("NotificationProcessor"), &constructs.DynamoStreamProcessorProps{
        FunctionProps: awslambda.FunctionProps{
            FunctionName: jsii.String("notification-processor"),
            Code:         awslambda.Code_FromAsset(jsii.String("./notification-dist")),
            Handler:      jsii.String("bootstrap"),
            Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        },
        StreamingTableProps: &constructs.StreamingTableProps{
            TableName: jsii.String("notification-table"),
            StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
        },
        BatchSize: jsii.Number(10),
        MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(1)),
    })
    
    // Audit processor
    auditProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("AuditProcessor"), &constructs.DynamoStreamProcessorProps{
        FunctionProps: awslambda.FunctionProps{
            FunctionName: jsii.String("audit-processor"),
            Code:         awslambda.Code_FromAsset(jsii.String("./audit-dist")),
            Handler:      jsii.String("bootstrap"),
            Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        },
        StreamingTableProps: &constructs.StreamingTableProps{
            TableName: jsii.String("audit-table"),
            StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
        },
        StartingPosition: awslambda.StartingPosition_TRIM_HORIZON,
        BisectBatchOnError: jsii.Bool(true),
    })
}
```

### Stream Filtering

```go
// Filter specific events using event source mapping filters
filteredProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("FilteredProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("filtered-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    
    // Custom event source configuration with filters
    EventSourceProps: &awslambdaeventsources.DynamoEventSourceProps{
        Filters: &[]*map[string]interface{}{
            {
                "eventName": []string{"INSERT", "MODIFY"},
                "dynamodb": map[string]interface{}{
                    "NewImage": map[string]interface{}{
                        "status": map[string]interface{}{
                            "S": []string{"ACTIVE", "PENDING"},
                        },
                    },
                },
            },
        },
    },
})
```

## Troubleshooting

### Common Issues

#### 1. High Iterator Age

**Problem**: DynamoDB stream iterator age is increasing
**Solutions**:
- Increase `ParallelizationFactor`
- Optimize Lambda function performance
- Increase Lambda memory/timeout
- Check for errors causing retries

```go
// High-performance configuration
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("HighPerfProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        MemorySize: jsii.Number(3008),  // Max memory
        Timeout:    awscdk.Duration_Minutes(jsii.Number(15)),  // Max timeout
    },
    ParallelizationFactor: jsii.Number(10),  // Max parallelization
    BatchSize: jsii.Number(100),  // Larger batches
})
```

#### 2. Throttling Issues

**Problem**: Lambda function being throttled
**Solutions**:
- Set reserved concurrency
- Implement exponential backoff
- Monitor concurrent executions

```go
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("ThrottledProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        ReservedConcurrentExecutions: jsii.Number(50),  // Limit concurrency
    },
    ParallelizationFactor: jsii.Number(2),  // Reduce parallelization
})
```

#### 3. Memory/Timeout Issues

**Problem**: Lambda function running out of memory or timing out
**Solutions**:
- Right-size Lambda configuration
- Optimize batch processing
- Implement checkpointing

```go
processor := constructs.NewDynamoStreamProcessor(stack, jsii.String("OptimizedProcessor"), &constructs.DynamoStreamProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        MemorySize: jsii.Number(1024),  // Right-sized
        Timeout:    awscdk.Duration_Minutes(jsii.Number(5)),  // Appropriate timeout
    },
    BatchSize: jsii.Number(25),  // Smaller batches
    MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(5)),
})
```

### Monitoring and Debugging

#### CloudWatch Metrics to Monitor

1. **Iterator Age**: `AWS/Lambda/IteratorAge`
2. **Duration**: `AWS/Lambda/Duration`
3. **Errors**: `AWS/Lambda/Errors`
4. **Throttles**: `AWS/Lambda/Throttles`
5. **Concurrent Executions**: `AWS/Lambda/ConcurrentExecutions`

#### Debugging Steps

1. **Check Lambda Logs**: Look for errors and performance issues
2. **Monitor DLQ**: Check dead letter queue for failed records
3. **Review Metrics**: Use CloudWatch metrics for performance analysis
4. **Test Locally**: Use DynamoDB Local for development testing

### Environment Variables Available

The construct automatically sets these environment variables:

- `DYNAMODB_TABLE_NAME`: The table name
- `DYNAMODB_TABLE_ARN`: The table ARN
- `DYNAMODB_STREAM_ARN`: The stream ARN
- `DYNAMODB_DLQ_URL`: Dead letter queue URL (if enabled)
- `LIFT_VERSION`: Lift framework version
- `LIFT_MULTI_TENANT`: Multi-tenant mode (if enabled)
- `LIFT_METRICS_ENABLED`: Metrics collection (if enabled)

### Helper Methods

```go
// Access construct properties
tableName := processor.GetTableName()
tableArn := processor.GetTableArn()
streamArn := processor.GetStreamArn()
dlqUrl := processor.GetDeadLetterQueueUrl()

// Grant permissions to other resources
processor.GrantReadWriteData(otherFunction)
processor.GrantStreamRead(readerFunction)
processor.GrantReadData(readOnlyFunction)
processor.GrantWriteData(writerFunction)

// Add environment variables
processor.AddEnvironmentVariable("CUSTOM_CONFIG", "value")
```

This comprehensive guide provides the foundation for implementing robust DynamoDB Streams processing with the Lift CDK constructs. Adapt the patterns to your specific use cases and requirements.