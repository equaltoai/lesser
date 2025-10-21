# DynamORM Stream Processing Example

This example demonstrates how to use the `DynamORMStreamProcessor` construct to create sophisticated DynamoDB stream processing patterns with DynamORM integration.

## Features Demonstrated

### 1. DynamORM Table with Streaming
- Multi-tenant DynamoDB table with DynamORM patterns
- Stream enabled with NEW_AND_OLD_IMAGES
- Global Secondary Indexes for common query patterns
- Point-in-time recovery and encryption

### 2. Multiple Stream Processors
- **User Events Processor**: Handles user lifecycle events
- **Analytics Processor**: Processes data for real-time analytics
- **Notifications Processor**: Sends notifications for critical events

### 3. Advanced Stream Configuration
- Event filtering by event type and attributes
- Tenant-based filtering for multi-tenant applications
- Different processing modes (sequential, parallel, batched)
- Custom batch sizes and processing windows
- Dead letter queues for error handling

### 4. Comprehensive Monitoring
- CloudWatch metrics and alarms
- X-Ray tracing for distributed tracing
- Custom metrics collection
- Multi-tenant metrics with tenant isolation
- SNS alerts for operational issues

### 5. Security and Permissions
- IAM roles with least-privilege access
- Tenant-isolated access patterns
- DynamORM-specific permissions
- Stream read permissions

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────────┐
│   DynamORM      │    │   DynamoDB       │    │   Lambda            │
│   Application   │───▶│   Table          │───▶│   Stream            │
│                 │    │   (with Stream)  │    │   Processors        │
└─────────────────┘    └──────────────────┘    └─────────────────────┘
                                │                          │
                                │                          │
                                ▼                          ▼
                       ┌──────────────────┐    ┌─────────────────────┐
                       │   CloudWatch     │    │   Dead Letter       │
                       │   Metrics &      │    │   Queues            │
                       │   Alarms         │    │                     │
                       └──────────────────┘    └─────────────────────┘
```

## Stream Processors

### User Stream Processor
- **Purpose**: Process user lifecycle events (create, update, delete)
- **Batch Size**: 25 records
- **Processing Mode**: Sequential
- **Features**: 
  - Multi-tenant filtering
  - Event validation
  - Data enrichment
  - Audit logging

### Analytics Stream Processor
- **Purpose**: Real-time analytics and reporting
- **Batch Size**: 100 records (larger for efficiency)
- **Processing Mode**: Parallel
- **Features**:
  - Data aggregation
  - Metrics computation
  - Time-series data generation

### Notification Stream Processor
- **Purpose**: Send notifications for critical events
- **Batch Size**: 1 record (for speed)
- **Processing Mode**: Sequential
- **Features**:
  - High-priority event filtering
  - Immediate notification delivery
  - Failure tracking

## Event Filtering

The example demonstrates sophisticated event filtering:

### User Events Processor
```go
EventFilters: []constructs.StreamEventFilter{
    {
        EventName: jsii.String("INSERT"),
        AttributeFilters: map[string]string{
            "entity_type": "User",
        },
    },
    {
        EventName: jsii.String("MODIFY"),
        AttributeFilters: map[string]string{
            "entity_type": "User",
            "status": "active",
        },
    },
}
```

### Notification Processor
```go
EventFilters: []constructs.StreamEventFilter{
    {
        EventName: jsii.String("INSERT"),
        AttributeFilters: map[string]string{
            "entity_type": "User",
            "priority":    "high",
        },
    },
}
```

## Monitoring and Observability

### CloudWatch Metrics
- Function invocations, errors, throttles
- Stream iterator age
- Dead letter queue message count
- Custom business metrics

### X-Ray Tracing
- End-to-end request tracing
- Service map visualization
- Performance analysis
- Error tracking

### Custom Metrics
- `UserCreated`, `UserUpdated`, `UserDeleted`
- `ProcessingLatency`
- `ValidationErrors`
- `NotificationsSent`

## Multi-Tenant Support

The example demonstrates multi-tenant patterns:

- **Tenant Isolation**: Stream events filtered by tenant ID
- **Tenant Metrics**: Separate metrics per tenant
- **Access Control**: IAM policies enforce tenant boundaries

## Deployment

### Prerequisites
1. AWS CDK v2 installed
2. Go 1.21+ installed
3. AWS credentials configured
4. Lambda function code built

### Lambda Function Structure
```
examples/dynamorm-stream-processing/
├── lambda/                    # User events processor
│   └── user-stream-handler
├── analytics-lambda/          # Analytics processor  
│   └── analytics-handler
├── notification-lambda/       # Notification processor
│   └── notification-handler
├── main.go                   # CDK stack definition
└── README.md                 # This file
```

### Build Lambda Functions
```bash
# Build user events processor
cd lambda
GOOS=linux GOARCH=arm64 go build -o user-stream-handler main.go

# Build analytics processor  
cd ../analytics-lambda
GOOS=linux GOARCH=arm64 go build -o analytics-handler main.go

# Build notification processor
cd ../notification-lambda
GOOS=linux GOARCH=arm64 go build -o notification-handler main.go
```

### Deploy Stack
```bash
# Synthesize CloudFormation template
cdk synth

# Deploy the stack
cdk deploy

# View deployed resources
aws cloudformation describe-stacks --stack-name DynamORMStreamProcessingStack
```

### Environment Variables

Each Lambda function receives these environment variables:

#### DynamORM Table Variables
- `DYNAMODB_TABLE_NAME`: Table name
- `AWS_REGION`: AWS region
- `DYNAMODB_STREAM_ARN`: Stream ARN

#### DynamORM Configuration
- `DYNAMORM_STREAM_PROCESSING`: "true"
- `DYNAMORM_TENANT_ATTRIBUTE`: "tenant_id"
- `DYNAMORM_METRICS_ENABLED`: "true"
- `DYNAMORM_PROCESSING_MODE`: Processing mode

#### Custom Variables
- `LOG_LEVEL`: Logging level
- `PROCESSOR_TYPE`: Processor identifier

## Lambda Handler Examples

### User Events Handler (Go)
```go
package main

import (
    "context"
    "encoding/json"
    "log"
    
    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
)

func handler(ctx context.Context, event events.DynamoDBEvent) (events.DynamoDBEventResponse, error) {
    var response events.DynamoDBEventResponse
    
    for _, record := range event.Records {
        // Process each DynamoDB stream record
        if err := processRecord(record); err != nil {
            log.Printf("Error processing record: %v", err)
            
            // Report failed record for retry
            response.BatchItemFailures = append(response.BatchItemFailures, 
                events.DynamoDBBatchItemFailure{
                    ItemIdentifier: record.EventID,
                })
        }
    }
    
    return response, nil
}

func processRecord(record events.DynamoDBEventRecord) error {
    // Extract tenant ID for multi-tenant processing
    tenantID := extractTenantID(record)
    
    switch record.EventName {
    case "INSERT":
        return handleUserCreated(record, tenantID)
    case "MODIFY":
        return handleUserUpdated(record, tenantID)
    case "REMOVE":
        return handleUserDeleted(record, tenantID)
    }
    
    return nil
}

func main() {
    lambda.Start(handler)
}
```

## Testing

### Unit Tests
```bash
go test ./...
```

### Integration Tests
```bash
# Test with DynamoDB Local
docker run -p 8000:8000 amazon/dynamodb-local

# Run integration tests
go test -tags=integration ./...
```

### Load Testing
```bash
# Generate test data
go run scripts/generate-test-data.go

# Monitor stream processing metrics
aws cloudwatch get-metric-statistics \
  --namespace AWS/Lambda \
  --metric-name IteratorAge \
  --dimensions Name=FunctionName,Value=user-stream-processor \
  --start-time 2024-01-01T00:00:00Z \
  --end-time 2024-01-01T01:00:00Z \
  --period 300 \
  --statistics Average
```

## Cleanup

```bash
# Destroy the stack
cdk destroy

# Clean up lambda artifacts
rm lambda/user-stream-handler
rm analytics-lambda/analytics-handler  
rm notification-lambda/notification-handler
```

## Best Practices Demonstrated

1. **Error Handling**: Comprehensive error handling with DLQs
2. **Monitoring**: Extensive CloudWatch metrics and alarms
3. **Security**: Least-privilege IAM policies
4. **Performance**: Optimized batch sizes and parallelization
5. **Multi-tenancy**: Proper tenant isolation
6. **Observability**: X-Ray tracing and custom metrics
7. **Resilience**: Retry logic and dead letter queues

## Related Examples

- [Basic DynamORM Example](../basic-dynamorm/)
- [Multi-Tenant SaaS Example](../multi-tenant-saas/)
- [Event-Driven Architecture Example](../event-driven-architecture/)