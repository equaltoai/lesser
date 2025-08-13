# Scheduled Job Cost Tracking Implementation

## Overview

This implementation provides comprehensive cost tracking for scheduled/cron jobs using DynamORM/Lift patterns. It supports multi-step jobs, cascading cost tracking, performance metrics, and detailed analytics while maintaining clean architecture and avoiding import cycles.

## Components Implemented

### 1. Model Layer (`pkg/storage/models/scheduled_job_cost_tracking.go`)

**ScheduledJobCostRecord** - Tracks individual job executions with:
- **Job Identification**: job name, schedule pattern, category, priority
- **Execution Timing**: scheduled time, actual start/end, delays, duration
- **Resource Usage**: Lambda invocations, DynamoDB operations, SQS messages, S3 operations
- **Cost Breakdown**: Lambda, DynamoDB, SQS, S3, CloudWatch, data transfer, external API costs (in microcents)
- **Business Metrics**: items processed/skipped/errored, batch size, retry counts
- **Performance Metrics**: custom performance indicators
- **Cascading Costs**: costs of operations triggered by this job
- **Multi-tenancy**: environment, region, function context
- **Error Tracking**: detailed error messages and retry attempts

**Key Patterns Used**:
```
PK: "SCHEDULED_JOB_COST#{jobName}#{schedule}" 
SK: "RUN#{timestamp}#{id}"
GSI1: Status-based queries (GSI1PK: "SCHEDULED_JOB_STATUS#{status}")
GSI2: Date-based queries (GSI2PK: "SCHEDULED_JOB_DATE#{dateStr}")
TTL: 90 days for execution records
```

**ScheduledJobCostAggregation** - Pre-computed aggregations with:
- Execution statistics (success/failure rates, percentiles)
- Resource aggregations (total Lambda duration, DynamoDB capacity)
- Cost aggregations (total costs, averages, efficiency metrics)
- Breakdown by category, environment, schedule pattern
- Performance trends and percentiles

### 2. Repository Layer (`pkg/storage/repositories/scheduled_job_cost_repository.go`)

**ScheduledJobCostRepository** provides:
- **CRUD Operations**: Create, Update, Get, GetByID
- **Query Methods**: ListByJob, ListByStatus, ListByDateRange
- **Analytics Methods**: GetFailedJobs, GetHighCostJobs, GetLongRunningJobs
- **Statistics**: GetJobExecutionStats, GetJobPerformanceTrends, GetScheduledJobsSummary
- **Aggregation**: AggregateJobCosts for rolling up raw data

**Query Patterns**:
- Primary key queries for specific executions
- GSI1 queries for status-based analysis (failed jobs, etc.)
- GSI2 queries for date-range analysis
- Efficient pagination and limiting

### 3. Cost Tracker Service (`pkg/cost/scheduled_job_cost_tracker.go`)

**ScheduledJobCostTracker** provides:
- **Interface-based Design**: Uses repository interface to avoid import cycles
- **JobExecution**: Fluent API for tracking job execution with resource usage
- **Multi-step Support**: MultiStepJobExecution for complex workflows
- **Built-in Job Types**: Helpers for common job patterns (cleanup, aggregation, analytics)
- **Cost Calculation**: Automatic cost calculation based on AWS pricing

**Key Features**:
```go
// Simple job tracking
execution := cost.NewJobExecution("cleanup-job", "daily").
    WithCategory("maintenance").
    WithPriority("low")

// Track resource usage
execution.TrackLambdaUsage(5, 10000, 512) // invocations, duration, memory
execution.TrackDynamoDBUsage(100, 50, 50.0, 50.0) // reads, writes, capacity
execution.TrackItemsProcessed(1000, 100, 5) // processed, skipped, errors

// Complete tracking
execution.FinishWithSuccess(ctx, tracker)
```

**Multi-step Job Support**:
```go
multiStep := cost.NewMultiStepJobExecution("data-pipeline", "hourly")
step1 := multiStep.StartStep("extract")
step1.TrackStepLambdaUsage(5000)
multiStep.FinishStep("extract", "success", nil)
```

## Architecture Decisions

### 1. Import Cycle Avoidance
- **Problem**: Direct imports between `pkg/cost` and `pkg/storage/repositories` create cycles
- **Solution**: Define repository interface in cost package, implement in repositories
- **Result**: Clean separation of concerns, testable interfaces

### 2. DynamORM Integration
- **Single Table Design**: All data in `lesser-main` table with proper key patterns
- **GSI Strategy**: Two GSIs for status-based and date-based queries
- **TTL Management**: Automatic cleanup with different retention periods
- **Batch Operations**: Explicit support for batch processing patterns

### 3. Cost Precision
- **Microcents**: All costs stored as int64 microcents for precision
- **AWS Pricing**: Built-in constants for Lambda, DynamoDB, S3, etc.
- **Real-time Calculation**: Costs calculated during execution, not post-processing
- **Cascading Costs**: Track costs of operations triggered by jobs

### 4. Performance Metrics
- **Custom Metrics**: Flexible key-value store for job-specific metrics
- **Percentiles**: Built-in percentile calculations for duration and cost
- **Trends**: Time-series analysis for performance degradation detection
- **Efficiency Tracking**: Cost per item processed, success rates, etc.

## Usage Patterns

### 1. Simple Job Tracking
```go
// In your scheduled Lambda function
func handler(ctx context.Context, event events.CloudWatchEvent) error {
    execution := cost.NewJobExecution("user-cleanup", "daily").
        WithCategory("maintenance").
        WithContext("production", "us-east-1", "cleanup-lambda", requestID)
    
    // Do work and track resources
    execution.TrackLambdaUsage(1, 5000, 256)
    execution.TrackDynamoDBUsage(100, 20, 50.0, 20.0)
    execution.TrackItemsProcessed(500, 10, 2)
    
    return execution.FinishWithSuccess(ctx, tracker)
}
```

### 2. Multi-step Job Tracking
```go
multiStep := cost.NewMultiStepJobExecution("etl-pipeline", "hourly")

// Step 1: Extract
step1 := multiStep.StartStep("extract")
// ... do extraction work
step1.TrackStepItemsProcessed(1000, 0)
multiStep.FinishStep("extract", "success", nil)

// Step 2: Transform  
step2 := multiStep.StartStep("transform")
// ... do transformation work
multiStep.FinishStep("transform", "success", nil)

return multiStep.FinishWithSuccess(ctx, tracker)
```

### 3. Cost Aggregation
```go
// Aggregate hourly costs for the last day
err := tracker.AggregateJobCosts(ctx, "cleanup-job", "hour", startTime, endTime)

// Get high-cost jobs for analysis
highCostJobs, err := tracker.GetHighCostJobs(ctx, 0.01, startTime, endTime, 10)
```

### 4. Analytics and Monitoring
```go
// Get failed jobs for alerting
failedJobs, err := tracker.GetFailedJobs(ctx, startTime, endTime, 20)

// Get long-running jobs for optimization
longRunning, err := tracker.GetLongRunningJobs(ctx, 300000, startTime, endTime, 10) // 5+ minutes
```

## Integration Examples

Complete integration examples are available in:
- `/examples/cost_tracking/scheduled_job_wiring_example.go` - Shows how to wire up the system
- `/examples/cost_tracking/scheduled_job_integration_examples.go` - Real-world usage patterns

Key integration patterns:
1. **Cost Aggregation Jobs** - Track the cost of cost tracking itself
2. **Cleanup Jobs** - Multi-step cleanup with batch processing
3. **Analytics Jobs** - ETL pipelines with external API calls
4. **Maintenance Jobs** - Index optimization and system maintenance
5. **Error Recovery Jobs** - DLQ processing with retry logic

## Benefits

### 1. Cost Visibility
- **Granular Tracking**: Track costs down to individual job executions
- **Cost Attribution**: Understand which jobs drive costs
- **Trend Analysis**: Identify cost increases over time
- **Budget Management**: Set thresholds and alerts

### 2. Performance Monitoring
- **Duration Tracking**: Identify slow jobs
- **Resource Usage**: Optimize Lambda memory and DynamoDB usage
- **Success Rates**: Monitor job reliability
- **Efficiency Metrics**: Cost per item processed

### 3. Operational Excellence
- **Error Analysis**: Detailed tracking of job failures
- **Retry Monitoring**: Track retry costs and patterns
- **Cascading Impact**: Understand downstream cost impacts
- **Multi-step Visibility**: Track complex workflow performance

### 4. Compliance and Governance
- **Audit Trail**: Complete history of job executions
- **Cost Allocation**: Attribute costs to business units
- **SLA Monitoring**: Track against performance targets
- **Resource Optimization**: Data-driven optimization decisions

## Future Enhancements

1. **Real-time Alerting**: Integration with CloudWatch alarms
2. **Budget Enforcement**: Automatic job throttling based on costs
3. **Predictive Analytics**: ML-based cost and performance predictions
4. **Cost Optimization Recommendations**: Automated suggestions
5. **Multi-region Support**: Cross-region cost aggregation
6. **Integration with AWS Cost Explorer**: Enhanced cost visibility

## Testing

The implementation follows existing patterns in the codebase:
- **Repository Tests**: Use mocks for DynamoDB operations
- **Cost Tracker Tests**: Test job execution tracking logic
- **Integration Tests**: End-to-end testing with real DynamoDB
- **Performance Tests**: Verify aggregation performance

## Deployment

The system requires:
1. **DynamoDB Table**: Uses existing `lesser-main` table
2. **GSI Creation**: Two additional GSIs for efficient querying
3. **IAM Permissions**: Standard DynamoDB read/write permissions
4. **Lambda Configuration**: Memory and timeout optimization
5. **CloudWatch Integration**: For metrics and alarms

This implementation provides a robust, scalable foundation for tracking and optimizing scheduled job costs while maintaining the architectural patterns established in the Lesser codebase.