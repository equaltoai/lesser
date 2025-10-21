# S3 Processing Patterns with Lift CDK

The `S3Processor` construct provides a complete solution for processing S3 events with Lambda functions, including automatic bucket management, event filtering, dead letter queues, and lifecycle management.

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration Options](#configuration-options)
- [Event Filtering](#event-filtering)
- [Lifecycle Management](#lifecycle-management)
- [Security Features](#security-features)
- [Error Handling](#error-handling)
- [Advanced Patterns](#advanced-patterns)
- [Performance Optimization](#performance-optimization)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Quick Start

### Basic S3 Processor

```go
import (
    "github.com/pay-theory/lift/pkg/cdk/constructs"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
    "github.com/aws/aws-cdk-go/awscdk/v2/awss3"
    "github.com/aws/aws-cdk-go/awscdk/v2/awss3notifications"
    "github.com/aws/jsii-runtime-go"
)

processor := constructs.NewS3Processor(stack, jsii.String("S3Processor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("image-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
})
```

### With Event Filtering

```go
processor := constructs.NewS3Processor(stack, jsii.String("ImageProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("image-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventTypes: &[]awss3.EventType{
        awss3.EventType_OBJECT_CREATED_PUT,
        awss3.EventType_OBJECT_CREATED_POST,
    },
    KeyPrefix: jsii.String("uploads/images/"),
    KeySuffix: jsii.String(".jpg"),
})
```

## Configuration Options

### Basic Properties

```go
type S3ProcessorProps struct {
    // Lambda function properties (required)
    FunctionProps awslambda.FunctionProps

    // S3 bucket properties (optional - creates new bucket if not provided)
    BucketProps *awss3.BucketProps

    // Existing bucket to use (optional)
    ExistingBucket awss3.IBucket

    // S3 event types to process (default: ObjectCreated)
    EventTypes *[]awss3.EventType

    // Key prefix filter for S3 events
    KeyPrefix *string

    // Key suffix filter for S3 events  
    KeySuffix *string

    // Dead letter queue configuration
    DeadLetterQueueProps *awssqs.QueueProps
    EnableDeadLetterQueue *bool // default: true

    // S3 event source configuration
    EventSourceProps *awslambdaeventsources.S3EventSourceProps

    // Additional S3 processor settings
    BatchSize         *float64        // Default: 10
    MaxBatchingWindow awscdk.Duration // Default: 5 seconds

    // Multi-region support
    CrossRegionReplication *bool
    ReplicationBucket      awss3.IBucket

    // Lifecycle rules
    EnableLifecycleRules *bool
    LifecycleRules       *[]*awss3.LifecycleRule

    // External bucket support
    ExternalBucket awss3.IBucket

    // Event filtering
    EventFilter *S3EventFilter

    // Access logging
    EnableAccessLogging *bool
    AccessLogsBucket    awss3.IBucket
    AccessLogsPrefix    *string

    // Versioning
    EnableVersioning *bool

    // Lift-specific settings
    EnableTracing     *bool
    EnableMultiTenant *bool
    EnableMonitoring  *bool
}

// S3EventFilter defines event filtering options
type S3EventFilter struct {
    Prefix *string
    Suffix *string
}
```

### Custom Bucket Configuration

```go
processor := constructs.NewS3Processor(stack, jsii.String("CustomBucketProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("document-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    BucketProps: &awss3.BucketProps{
        BucketName:        jsii.String("my-document-bucket"),
        Versioned:         jsii.Bool(true),
        Encryption:        awss3.BucketEncryption_S3_MANAGED,
        BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
    },
    EnableVersioning: jsii.Bool(true),
})
```

### Using External Bucket

```go
// Reference external bucket
externalBucket := awss3.Bucket_FromBucketName(stack, jsii.String("ExternalBucket"), jsii.String("my-external-bucket"))

processor := constructs.NewS3Processor(stack, jsii.String("ExternalBucketProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("external-bucket-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    ExternalBucket: externalBucket,
    EventFilter: &constructs.S3EventFilter{
        Prefix: jsii.String("uploads/"),
        Suffix: jsii.String(".jpg"),
    },
})
```

### Event Filtering with S3EventFilter

The `S3EventFilter` type provides a structured way to define event filtering:

```go
processor := constructs.NewS3Processor(stack, jsii.String("FilteredProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("filtered-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventFilter: &constructs.S3EventFilter{
        Prefix: jsii.String("data/"),
        Suffix: jsii.String(".json"),
    },
})
```

## Event Filtering

### Event Types

```go
// Process only PUT events
EventTypes: &[]awss3.EventType{
    awss3.EventType_OBJECT_CREATED_PUT,
}

// Process all creation events
EventTypes: &[]awss3.EventType{
    awss3.EventType_OBJECT_CREATED,
}

// Process creation and deletion
EventTypes: &[]awss3.EventType{
    awss3.EventType_OBJECT_CREATED,
    awss3.EventType_OBJECT_REMOVED,
}

// Specific event types
EventTypes: &[]awss3.EventType{
    awss3.EventType_OBJECT_CREATED_PUT,
    awss3.EventType_OBJECT_CREATED_POST,
    awss3.EventType_OBJECT_CREATED_COPY,
    awss3.EventType_OBJECT_CREATED_COMPLETE_MULTIPART_UPLOAD,
}
```

### Key Filtering

```go
// Filter by prefix
KeyPrefix: jsii.String("uploads/images/")

// Filter by suffix (file extension)
KeySuffix: jsii.String(".jpg")

// Combine prefix and suffix
KeyPrefix: jsii.String("data/"),
KeySuffix: jsii.String(".json"),

// Using custom event source props for complex filtering
EventSourceProps: &awslambdaeventsources.S3EventSourceProps{
    Events: &[]awss3.EventType{awss3.EventType_OBJECT_CREATED},
    Filters: &[]*awss3.NotificationKeyFilter{
        {
            Prefix: jsii.String("uploads/"),
            Suffix: jsii.String(".pdf"),
        },
        {
            Prefix: jsii.String("documents/"),
            Suffix: jsii.String(".docx"),
        },
    },
}
```

## Lifecycle Management

### Enable Default Lifecycle Rules

```go
processor := constructs.NewS3Processor(stack, jsii.String("LifecycleProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("lifecycle-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EnableLifecycleRules: jsii.Bool(true), // Applies default rules
})
```

### Custom Lifecycle Rules

```go
customLifecycleRules := []*awss3.LifecycleRule{
    {
        Id:      jsii.String("DeleteOldVersions"),
        Enabled: jsii.Bool(true),
        NoncurrentVersionExpiration: awscdk.Duration_Days(jsii.Number(30)),
    },
    {
        Id:      jsii.String("TransitionToGlacier"),
        Enabled: jsii.Bool(true),
        Transitions: &[]*awss3.Transition{
            {
                StorageClass:    awss3.StorageClass_GLACIER(),
                TransitionAfter: awscdk.Duration_Days(jsii.Number(90)),
            },
            {
                StorageClass:    awss3.StorageClass_DEEP_ARCHIVE(),
                TransitionAfter: awscdk.Duration_Days(jsii.Number(365)),
            },
        },
    },
    {
        Id:      jsii.String("CleanupMultipartUploads"),
        Enabled: jsii.Bool(true),
        AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(7)),
    },
}

processor := constructs.NewS3Processor(stack, jsii.String("CustomLifecycleProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("custom-lifecycle-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EnableLifecycleRules: jsii.Bool(true),
    LifecycleRules:       &customLifecycleRules,
})
```

## Security Features

### Access Logging

```go
// Create access logs bucket
logsBucket := awss3.NewBucket(stack, jsii.String("AccessLogsBucket"), &awss3.BucketProps{
    BucketName: jsii.String("my-access-logs-bucket"),
})

processor := constructs.NewS3Processor(stack, jsii.String("SecureProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("secure-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EnableAccessLogging: jsii.Bool(true),
    AccessLogsBucket:    logsBucket,
    AccessLogsPrefix:    jsii.String("access-logs/"),
})
```

### Versioning and Backup

```go
processor := constructs.NewS3Processor(stack, jsii.String("BackupProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("backup-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EnableVersioning: jsii.Bool(true),
})
```

### Encryption

```go
import (
    "github.com/pay-theory/lift/pkg/cdk/constructs"
    "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
    "github.com/aws/aws-cdk-go/awscdk/v2/awss3"
    "github.com/aws/aws-cdk-go/awscdk/v2/awskms"
    "github.com/aws/jsii-runtime-go"
)

// Create KMS key for encryption
key := awskms.NewKey(stack, jsii.String("S3Key"), &awskms.KeyProps{
    Description: jsii.String("S3 bucket encryption key"),
})

processor := constructs.NewS3Processor(stack, jsii.String("EncryptedProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("encrypted-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    BucketProps: &awss3.BucketProps{
        Encryption:    awss3.BucketEncryption_KMS,
        EncryptionKey: key,
    },
})
```

## Error Handling

### Dead Letter Queue Configuration

```go
processor := constructs.NewS3Processor(stack, jsii.String("ReliableProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("reliable-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EnableDeadLetterQueue: jsii.Bool(true),
    DeadLetterQueueProps: &awssqs.QueueProps{
        QueueName:       jsii.String("s3-processing-dlq"),
        RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
    },
})
```

### Disable Dead Letter Queue

```go
processor := constructs.NewS3Processor(stack, jsii.String("NoDLQProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("no-dlq-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EnableDeadLetterQueue: jsii.Bool(false),
})
```

## Advanced Patterns

### Multi-Region Setup

```go
// Create replication bucket in another region
replicationBucket := awss3.NewBucket(stack, jsii.String("ReplicationBucket"), &awss3.BucketProps{
    BucketName: jsii.String("my-replication-bucket"),
    // Configure in different region via stack or app settings
})

processor := constructs.NewS3Processor(stack, jsii.String("MultiRegionProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("multi-region-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    CrossRegionReplication: jsii.Bool(true),
    ReplicationBucket:      replicationBucket,
})
```

### File Processing Pipeline

```go
// Image processing pipeline
imageProcessor := constructs.NewS3Processor(stack, jsii.String("ImageProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("image-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist/image-processor")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        MemorySize:   jsii.Number(1024),
        Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
    },
    KeyPrefix: jsii.String("uploads/images/"),
    KeySuffix: jsii.String(".jpg"),
    EventTypes: &[]awss3.EventType{
        awss3.EventType_OBJECT_CREATED_PUT,
    },
})

// Document processing pipeline
docProcessor := constructs.NewS3Processor(stack, jsii.String("DocumentProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("document-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist/doc-processor")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        MemorySize:   jsii.Number(512),
        Timeout:      awscdk.Duration_Minutes(jsii.Number(2)),
    },
    ExistingBucket: imageProcessor.Bucket, // Share the same bucket
    KeyPrefix:      jsii.String("uploads/documents/"),
    KeySuffix:      jsii.String(".pdf"),
})
```

### Fan-Out Pattern

```go
// Primary processor
primaryProcessor := constructs.NewS3Processor(stack, jsii.String("PrimaryProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("primary-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist/primary")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
})

// Additional processors using the same bucket
thumbnailProcessor := awslambda.NewFunction(stack, jsii.String("ThumbnailProcessor"), &awslambda.FunctionProps{
    FunctionName: jsii.String("thumbnail-processor"),
    Code:         awslambda.Code_FromAsset(jsii.String("./dist/thumbnail")),
    Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
})

metadataProcessor := awslambda.NewFunction(stack, jsii.String("MetadataProcessor"), &awslambda.FunctionProps{
    FunctionName: jsii.String("metadata-processor"),
    Code:         awslambda.Code_FromAsset(jsii.String("./dist/metadata")),
    Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
})

// Grant permissions to additional processors
primaryProcessor.GrantRead(thumbnailProcessor)
primaryProcessor.GrantRead(metadataProcessor)

// Add additional notifications manually
if bucket, ok := primaryProcessor.Bucket.(awss3.Bucket); ok {
    bucket.AddEventNotification(
        awss3.EventType_OBJECT_CREATED,
        awss3notifications.NewLambdaDestination(thumbnailProcessor),
        &awss3.NotificationKeyFilter{Suffix: jsii.String(".jpg")},
    )
}
```

## Performance Optimization

### Memory and Timeout Configuration

```go
processor := constructs.NewS3Processor(stack, jsii.String("OptimizedProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("optimized-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        Architecture: awslambda.Architecture_ARM_64(), // Better price/performance
        MemorySize:   jsii.Number(1024),               // Adjust based on workload
        Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
        Environment: &map[string]*string{
            "RUST_LOG": jsii.String("info"),
        },
    },
    EnableTracing: jsii.Bool(true), // For performance monitoring
})
```

### Batch Processing Configuration

```go
processor := constructs.NewS3Processor(stack, jsii.String("BatchProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("batch-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    BatchSize:         jsii.Number(5),                    // Process up to 5 events per invocation
    MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(10)), // Wait up to 10 seconds to batch events
})
```

### Reserved Concurrency

```go
processor := constructs.NewS3Processor(stack, jsii.String("ConcurrencyProcessor"), &constructs.S3ProcessorProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName:                    jsii.String("concurrency-processor"),
        Code:                           awslambda.Code_FromAsset(jsii.String("./dist")),
        Runtime:                        awslambda.Runtime_PROVIDED_AL2023(),
        ReservedConcurrentExecutions:   jsii.Number(10), // Limit concurrent executions
    },
})
```

## Best Practices

### 1. Event Filtering

- Use specific event types instead of broad wildcards
- Apply key prefix/suffix filters to reduce unnecessary invocations
- Consider the cost of Lambda invocations vs. processing logic

```go
// Good: Specific filtering
EventTypes: &[]awss3.EventType{awss3.EventType_OBJECT_CREATED_PUT},
KeyPrefix: jsii.String("invoices/"),
KeySuffix: jsii.String(".pdf"),

// Avoid: Too broad
EventTypes: &[]awss3.EventType{awss3.EventType_OBJECT_CREATED},
```

### 2. Error Handling

- Always enable dead letter queues for production workloads
- Set appropriate retention periods for DLQs
- Monitor DLQ metrics and set up alerts

```go
EnableDeadLetterQueue: jsii.Bool(true),
DeadLetterQueueProps: &awssqs.QueueProps{
    RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
},
```

### 3. Security

- Enable versioning for important data
- Use server-side encryption
- Block public access by default
- Enable access logging for audit trails

```go
BucketProps: &awss3.BucketProps{
    Versioned:           jsii.Bool(true),
    Encryption:          awss3.BucketEncryption_S3_MANAGED,
    BlockPublicAccess:   awss3.BlockPublicAccess_BLOCK_ALL(),
    EnforceSSL:          jsii.Bool(true),
},
EnableAccessLogging: jsii.Bool(true),
```

### 4. Cost Optimization

- Use lifecycle rules to transition to cheaper storage classes
- Clean up incomplete multipart uploads
- Monitor storage costs and usage patterns

```go
EnableLifecycleRules: jsii.Bool(true),
LifecycleRules: &[]*awss3.LifecycleRule{
    {
        Id:      jsii.String("CostOptimization"),
        Enabled: jsii.Bool(true),
        Transitions: &[]*awss3.Transition{
            {
                StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
                TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
            },
        },
        AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(1)),
    },
},
```

### 5. Monitoring

- Enable tracing for performance insights
- Set up CloudWatch alarms for errors and duration
- Monitor DLQ message counts

```go
EnableTracing:    jsii.Bool(true),
EnableMonitoring: jsii.Bool(true),
```

## Environment Variables

The S3Processor automatically injects the following environment variables into your Lambda function:

- `S3_BUCKET_NAME`: The name of the S3 bucket
- `S3_BUCKET_ARN`: The ARN of the S3 bucket  
- `S3_DLQ_URL`: The URL of the dead letter queue (if enabled)
- `S3_REPLICATION_BUCKET_NAME`: The name of the replication bucket (if configured)

### Using in Go Code

```go
import (
    "os"
    "github.com/pay-theory/lift/pkg/lift"
)

func handler(ctx *lift.Context) error {
    bucketName := os.Getenv("S3_BUCKET_NAME")
    bucketARN := os.Getenv("S3_BUCKET_ARN")
    dlqURL := os.Getenv("S3_DLQ_URL")
    
    // Process S3 event
    return processS3Event(ctx, bucketName)
}
```

## Helper Methods

### Permission Management

```go
// Grant permissions to other functions
processor.GrantRead(otherFunction)
processor.GrantWrite(otherFunction)
processor.GrantReadWrite(otherFunction)
processor.GrantDelete(otherFunction)

// Add environment variables
processor.AddEnvironmentVariable("CUSTOM_CONFIG", "value")

// Get bucket information
bucketName := processor.GetBucketName()
bucketARN := processor.GetBucketArn()
domainName := processor.GetBucketDomainName()
```

**Note**: Permission methods accept `awslambda.IFunction` interface, not specific function types.

### CORS Configuration

```go
// Add CORS rules
corsRule := &awss3.CorsRule{
    AllowedMethods: &[]awss3.HttpMethods{
        awss3.HttpMethods_GET,
        awss3.HttpMethods_POST,
    },
    AllowedOrigins: &[]*string{jsii.String("https://myapp.com")},
    AllowedHeaders: &[]*string{jsii.String("*")},
}

processor.AddCorsRule(corsRule)
```

**Note**: CORS rules can only be added to concrete `awss3.Bucket` instances, not `awss3.IBucket` interfaces. For interface buckets, configure CORS in the `BucketProps` during creation.

## Troubleshooting

### Common Issues

#### 1. Lambda Function Not Triggered

**Symptoms**: S3 events occur but Lambda function is not invoked

**Solutions**:
- Check event filtering (prefix/suffix, event types)
- Verify bucket notification configuration
- Check Lambda function permissions
- Review CloudWatch logs for errors

```go
// Debug: Use broader event filtering temporarily
EventTypes: &[]awss3.EventType{awss3.EventType_OBJECT_CREATED},
// Remove KeyPrefix and KeySuffix temporarily
```

#### 2. Permission Errors

**Symptoms**: Access denied errors in Lambda logs

**Solutions**:
- Verify IAM permissions are correctly granted
- Check bucket policies
- Ensure Lambda execution role has necessary permissions

```go
// Ensure permissions are granted
processor.GrantRead(processor.Function.Function)
processor.GrantWrite(processor.Function.Function)
```

#### 3. Memory or Timeout Issues

**Symptoms**: Lambda functions timing out or running out of memory

**Solutions**:
- Increase memory allocation
- Increase timeout duration
- Optimize code for better performance
- Consider processing files asynchronously

```go
FunctionProps: awslambda.FunctionProps{
    MemorySize: jsii.Number(1024), // Increase memory
    Timeout:    awscdk.Duration_Minutes(jsii.Number(5)), // Increase timeout
}
```

#### 4. Dead Letter Queue Issues

**Symptoms**: Messages not appearing in DLQ despite failures

**Solutions**:
- Verify DLQ is enabled
- Check DLQ permissions
- Review retry configuration
- Monitor CloudWatch metrics

```go
// Ensure DLQ is properly configured
EnableDeadLetterQueue: jsii.Bool(true),
DeadLetterQueueProps: &awssqs.QueueProps{
    RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
},
```

### Debugging Steps

1. **Check CloudWatch Logs**: Review Lambda function logs for errors
2. **Monitor Metrics**: Check invocation count, error rate, and duration
3. **Test Manually**: Upload test files to trigger events
4. **Verify Configuration**: Review CDK template and deployed resources
5. **Check Permissions**: Ensure all required permissions are granted

### Performance Monitoring

```go
// Enable detailed monitoring
EnableTracing:    jsii.Bool(true),
EnableMonitoring: jsii.Bool(true),

// Add custom metrics in your Lambda function
func handler(ctx *lift.Context) error {
    start := time.Now()
    defer func() {
        duration := time.Since(start)
        // Log processing time
        log.Printf("Processing took %v", duration)
    }()
    
    // Your processing logic here
    return nil
}
```

## Migration Guide

### From Manual S3 + Lambda Setup

If you're migrating from a manual S3 and Lambda setup:

1. **Identify Current Configuration**: Document your current bucket settings, event types, and Lambda configuration
2. **Create S3Processor**: Replace manual resources with S3Processor construct
3. **Migrate Environment Variables**: Update your Lambda code to use the new environment variables
4. **Test Thoroughly**: Verify all event processing works as expected
5. **Update Monitoring**: Migrate any custom monitoring to use the new construct's monitoring features

### Example Migration

```go
// Before: Manual setup
bucket := awss3.NewBucket(stack, jsii.String("Bucket"), bucketProps)
function := awslambda.NewFunction(stack, jsii.String("Function"), functionProps)
bucket.AddEventNotification(awss3.EventType_OBJECT_CREATED, awss3notifications.NewLambdaDestination(function))

// After: Using S3Processor
processor := constructs.NewS3Processor(stack, jsii.String("Processor"), &constructs.S3ProcessorProps{
    FunctionProps: functionProps,
    BucketProps:   bucketProps,
    EventTypes:    &[]awss3.EventType{awss3.EventType_OBJECT_CREATED},
})
```

This migration provides additional benefits:
- Automatic dead letter queue setup
- Built-in monitoring capabilities  
- Environment variable injection
- Lifecycle management
- Security best practices by default