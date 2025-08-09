package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/equaltoai/lesser/pkg/common"
)

// ScheduledJobCostRecord represents detailed cost tracking for scheduled/cron jobs
type ScheduledJobCostRecord struct {
	// Primary key
	PK string `dynamorm:"pk" json:"pk"` // Format: "SCHEDULED_JOB_COST#{jobName}#{schedule}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "RUN#{timestamp}#{id}"

	// GSI1 - Job status and performance queries
	GSI1PK string `dynamorm:"index:job-status-index,pk" json:"gsi1_pk"` // Format: "SCHEDULED_JOB_STATUS#{status}"
	GSI1SK string `dynamorm:"index:job-status-index,sk" json:"gsi1_sk"` // Format: "{timestamp}#{jobName}#{id}"

	// GSI2 - Date range queries across all jobs
	GSI2PK string `dynamorm:"index:job-date-index,pk" json:"gsi2_pk"` // Format: "SCHEDULED_JOB_DATE#{dateStr}"
	GSI2SK string `dynamorm:"index:job-date-index,sk" json:"gsi2_sk"` // Format: "{timestamp}#{jobName}#{id}"

	// Core job information
	ID          string    `json:"id"`                     // Unique execution ID
	JobName     string    `json:"job_name"`               // e.g., "cost-aggregation", "cleanup-expired-data"
	Schedule    string    `json:"schedule"`               // e.g., "hourly", "daily", "weekly", "monthly"
	CronPattern string    `json:"cron_pattern,omitempty"` // Actual cron expression if available
	Timestamp   time.Time `json:"timestamp"`              // When the job ran
	StartTime   time.Time `json:"start_time"`             // Job start time
	EndTime     time.Time `json:"end_time"`               // Job end time
	Duration    int64     `json:"duration_ms"`            // Duration in milliseconds

	// Job execution status
	Status       string `json:"status"`                  // "success", "failed", "timeout", "cancelled"
	Success      bool   `json:"success"`                 // Whether job completed successfully
	ErrorMessage string `json:"error_message,omitempty"` // Error details if failed
	RetryCount   int    `json:"retry_count"`             // Number of retries attempted
	MaxRetries   int    `json:"max_retries"`             // Maximum retries configured

	// Resource usage tracking
	LambdaInvocations  int64 `json:"lambda_invocations"`    // Number of Lambda invocations
	LambdaDurationMs   int64 `json:"lambda_duration_ms"`    // Total Lambda execution time
	LambdaMemoryUsedMB int   `json:"lambda_memory_used_mb"` // Peak memory usage
	LambdaRequestCount int64 `json:"lambda_request_count"`  // Total Lambda requests

	// DynamoDB operations performed by the job
	DynamoDBReadOperations  int64   `json:"dynamodb_read_operations"`  // Read operations count
	DynamoDBWriteOperations int64   `json:"dynamodb_write_operations"` // Write operations count
	DynamoDBReadCapacity    float64 `json:"dynamodb_read_capacity"`    // Read capacity consumed
	DynamoDBWriteCapacity   float64 `json:"dynamodb_write_capacity"`   // Write capacity consumed

	// Other AWS service usage
	SQSMessages         int64 `json:"sqs_messages"`          // SQS messages processed
	S3Operations        int64 `json:"s3_operations"`         // S3 operations performed
	CloudWatchLogs      int64 `json:"cloudwatch_logs"`       // Log entries written
	DataTransferBytes   int64 `json:"data_transfer_bytes"`   // Data transfer volume
	ExternalAPIRequests int64 `json:"external_api_requests"` // External API calls made

	// Cost breakdown (in microcents for precision)
	LambdaCostMicroCents       int64 `json:"lambda_cost_micro_cents"`        // Lambda execution cost
	DynamoDBCostMicroCents     int64 `json:"dynamodb_cost_micro_cents"`      // DynamoDB operations cost
	SQSCostMicroCents          int64 `json:"sqs_cost_micro_cents"`           // SQS cost
	S3CostMicroCents           int64 `json:"s3_cost_micro_cents"`            // S3 operations cost
	CloudWatchCostMicroCents   int64 `json:"cloudwatch_cost_micro_cents"`    // CloudWatch logs cost
	DataTransferCostMicroCents int64 `json:"data_transfer_cost_micro_cents"` // Data transfer cost
	ExternalAPICostMicroCents  int64 `json:"external_api_cost_micro_cents"`  // External API costs
	TotalCostMicroCents        int64 `json:"total_cost_micro_cents"`         // Total cost

	// Cost in dollars for display
	TotalCostDollars float64 `json:"total_cost_dollars"`

	// Job-specific metrics and properties
	ItemsProcessed     int64                  `json:"items_processed"`               // Number of items/records processed
	ItemsSkipped       int64                  `json:"items_skipped"`                 // Items skipped due to conditions
	ItemsErrored       int64                  `json:"items_errored"`                 // Items that failed processing
	BatchSize          int                    `json:"batch_size"`                    // Batch size used
	JobProperties      map[string]interface{} `json:"job_properties,omitempty"`      // Job-specific properties
	PerformanceMetrics map[string]float64     `json:"performance_metrics,omitempty"` // Performance indicators

	// Cascading costs (costs triggered by this job)
	TriggeredJobs           []string `json:"triggered_jobs,omitempty"`   // Jobs triggered by this execution
	CascadingCostMicroCents int64    `json:"cascading_cost_micro_cents"` // Cost of triggered operations
	DownstreamOperations    int64    `json:"downstream_operations"`      // Operations triggered downstream

	// Job context and metadata
	Environment  string            `json:"environment"`    // "production", "staging", etc.
	Region       string            `json:"region"`         // AWS region
	FunctionName string            `json:"function_name"`  // Lambda function name
	RequestID    string            `json:"request_id"`     // AWS request ID
	Tags         map[string]string `json:"tags,omitempty"` // Custom tags
	JobCategory  string            `json:"job_category"`   // "maintenance", "aggregation", "cleanup", etc.
	Priority     string            `json:"priority"`       // "low", "normal", "high", "critical"

	// Scheduling context
	ScheduledTime     time.Time `json:"scheduled_time"`      // When job was scheduled to run
	ActualStartDelay  int64     `json:"actual_start_delay"`  // Delay between scheduled and actual start (ms)
	NextScheduledTime time.Time `json:"next_scheduled_time"` // Next scheduled execution

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup (90 days for job execution records)
	ExpiresAt int64 `dynamorm:"ttl" json:"expires_at"` // Unix timestamp
}

// ScheduledJobCostAggregation represents pre-computed aggregations for scheduled job costs
type ScheduledJobCostAggregation struct {
	// Primary key
	PK string `dynamorm:"pk" json:"pk"` // Format: "SCHEDULED_JOB_AGG#{period}#{jobName}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "WINDOW#{windowStart}"

	// Aggregation details
	JobName     string    `json:"job_name"`     // Specific job or "all" for all jobs
	Period      string    `json:"period"`       // "hour", "day", "week", "month"
	Schedule    string    `json:"schedule"`     // Job schedule pattern
	WindowStart time.Time `json:"window_start"` // Start of aggregation window
	WindowEnd   time.Time `json:"window_end"`   // End of aggregation window

	// Execution statistics
	TotalExecutions      int64   `json:"total_executions"`       // Total job executions
	SuccessfulExecutions int64   `json:"successful_executions"`  // Successful executions
	FailedExecutions     int64   `json:"failed_executions"`      // Failed executions
	TimeoutExecutions    int64   `json:"timeout_executions"`     // Timed out executions
	CancelledExecutions  int64   `json:"cancelled_executions"`   // Cancelled executions
	SuccessRate          float64 `json:"success_rate"`           // Success rate percentage
	AverageExecutionTime float64 `json:"average_execution_time"` // Average execution time (ms)
	MedianExecutionTime  float64 `json:"median_execution_time"`  // Median execution time (ms)
	P95ExecutionTime     float64 `json:"p95_execution_time"`     // 95th percentile execution time (ms)

	// Resource aggregations
	TotalLambdaInvocations    int64   `json:"total_lambda_invocations"`
	TotalLambdaDurationMs     int64   `json:"total_lambda_duration_ms"`
	AverageLambdaMemoryUsedMB float64 `json:"average_lambda_memory_used_mb"`
	TotalDynamoDBOperations   int64   `json:"total_dynamodb_operations"`
	TotalDynamoDBCapacity     float64 `json:"total_dynamodb_capacity"`
	TotalItemsProcessed       int64   `json:"total_items_processed"`
	TotalItemsErrored         int64   `json:"total_items_errored"`

	// Cost aggregations
	TotalLambdaCostMicroCents       int64   `json:"total_lambda_cost_micro_cents"`
	TotalDynamoDBCostMicroCents     int64   `json:"total_dynamodb_cost_micro_cents"`
	TotalSQSCostMicroCents          int64   `json:"total_sqs_cost_micro_cents"`
	TotalS3CostMicroCents           int64   `json:"total_s3_cost_micro_cents"`
	TotalCloudWatchCostMicroCents   int64   `json:"total_cloudwatch_cost_micro_cents"`
	TotalDataTransferCostMicroCents int64   `json:"total_data_transfer_cost_micro_cents"`
	TotalExternalAPICostMicroCents  int64   `json:"total_external_api_cost_micro_cents"`
	TotalCascadingCostMicroCents    int64   `json:"total_cascading_cost_micro_cents"`
	TotalCostMicroCents             int64   `json:"total_cost_micro_cents"`
	TotalCostDollars                float64 `json:"total_cost_dollars"`
	AverageCostPerExecution         float64 `json:"average_cost_per_execution"`

	// Cost efficiency metrics
	CostPerItemProcessed       float64 `json:"cost_per_item_processed"`       // Cost efficiency
	CostPerSuccessfulExecution float64 `json:"cost_per_successful_execution"` // Cost of successful runs
	CostEfficiencyTrend        float64 `json:"cost_efficiency_trend"`         // Trend in cost efficiency

	// Breakdown by job properties
	JobCategoryBreakdown map[string]*ScheduledJobCategoryStats    `json:"job_category_breakdown,omitempty"`
	EnvironmentBreakdown map[string]*ScheduledJobEnvironmentStats `json:"environment_breakdown,omitempty"`
	ScheduleBreakdown    map[string]*ScheduledJobScheduleStats    `json:"schedule_breakdown,omitempty"`

	// Performance trends
	ExecutionTimePercentiles map[string]float64 `json:"execution_time_percentiles,omitempty"` // p50, p90, p95, p99
	CostPercentiles          map[string]float64 `json:"cost_percentiles,omitempty"`           // Cost distribution

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL (longer for aggregated data - 365 days)
	ExpiresAt int64 `dynamorm:"ttl" json:"expires_at"`
}

// ScheduledJobCategoryStats represents cost statistics for a job category
type ScheduledJobCategoryStats struct {
	Category                string  `json:"category"`
	ExecutionCount          int64   `json:"execution_count"`
	TotalCostMicroCents     int64   `json:"total_cost_micro_cents"`
	TotalCostDollars        float64 `json:"total_cost_dollars"`
	AverageCostPerExecution float64 `json:"average_cost_per_execution"`
	SuccessRate             float64 `json:"success_rate"`
}

// ScheduledJobEnvironmentStats represents cost statistics for an environment
type ScheduledJobEnvironmentStats struct {
	Environment             string  `json:"environment"`
	ExecutionCount          int64   `json:"execution_count"`
	TotalCostMicroCents     int64   `json:"total_cost_micro_cents"`
	TotalCostDollars        float64 `json:"total_cost_dollars"`
	AverageCostPerExecution float64 `json:"average_cost_per_execution"`
}

// ScheduledJobScheduleStats represents cost statistics for a schedule pattern
type ScheduledJobScheduleStats struct {
	Schedule                string  `json:"schedule"`
	ExecutionCount          int64   `json:"execution_count"`
	TotalCostMicroCents     int64   `json:"total_cost_micro_cents"`
	TotalCostDollars        float64 `json:"total_cost_dollars"`
	AverageCostPerExecution float64 `json:"average_cost_per_execution"`
	AverageExecutionTime    float64 `json:"average_execution_time"`
}

// TableName returns the DynamoDB table name
func (ScheduledJobCostRecord) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table name
func (ScheduledJobCostAggregation) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the model before creation
func (sjcr *ScheduledJobCostRecord) BeforeCreate() error {
	now := time.Now()
	sjcr.CreatedAt = now
	sjcr.UpdatedAt = now

	// Generate ID if not provided
	if sjcr.ID == "" {
		sjcr.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if sjcr.Timestamp.IsZero() {
		sjcr.Timestamp = now
	}

	// Set start time if not provided
	if sjcr.StartTime.IsZero() {
		sjcr.StartTime = now
	}

	// Set end time if not provided and status is completed
	if sjcr.EndTime.IsZero() && (sjcr.Status == StatusSuccess || sjcr.Status == StatusFailed) {
		sjcr.EndTime = now
	}

	// Calculate duration if end time is set
	if !sjcr.EndTime.IsZero() && !sjcr.StartTime.IsZero() {
		sjcr.Duration = sjcr.EndTime.Sub(sjcr.StartTime).Milliseconds()
	}

	// Calculate actual start delay
	if !sjcr.ScheduledTime.IsZero() && !sjcr.StartTime.IsZero() {
		sjcr.ActualStartDelay = sjcr.StartTime.Sub(sjcr.ScheduledTime).Milliseconds()
	}

	// Calculate total cost in dollars
	sjcr.TotalCostDollars = float64(sjcr.TotalCostMicroCents) / 1_000_000.0

	// Set success flag based on status
	sjcr.Success = sjcr.Status == StatusSuccess

	// Set TTL (90 days for job execution records)
	sjcr.ExpiresAt = now.Add(90 * 24 * time.Hour).Unix()

	// Set up primary key
	sjcr.PK = fmt.Sprintf("SCHEDULED_JOB_COST#%s#%s", sjcr.JobName, sjcr.Schedule)
	timestampStr := sjcr.Timestamp.Format(common.CompactTimeFormat)
	sjcr.SK = fmt.Sprintf("RUN#%s#%s", timestampStr, sjcr.ID)

	// Set up GSI keys
	sjcr.setupGSIKeys()

	return sjcr.Validate()
}

// BeforeUpdate sets up the model before update
func (sjcr *ScheduledJobCostRecord) BeforeUpdate() error {
	sjcr.UpdatedAt = time.Now()

	// Recalculate duration if end time is set
	if !sjcr.EndTime.IsZero() && !sjcr.StartTime.IsZero() {
		sjcr.Duration = sjcr.EndTime.Sub(sjcr.StartTime).Milliseconds()
	}

	// Recalculate total cost in dollars
	sjcr.TotalCostDollars = float64(sjcr.TotalCostMicroCents) / 1_000_000.0

	// Update success flag based on status
	sjcr.Success = sjcr.Status == StatusSuccess

	// Update GSI keys in case indexed fields changed
	sjcr.setupGSIKeys()

	return sjcr.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (sjcr *ScheduledJobCostRecord) setupGSIKeys() {
	timestampStr := sjcr.Timestamp.Format(time.RFC3339)

	// GSI1 - Job status queries
	sjcr.GSI1PK = fmt.Sprintf("SCHEDULED_JOB_STATUS#%s", sjcr.Status)
	sjcr.GSI1SK = fmt.Sprintf("%s#%s#%s", timestampStr, sjcr.JobName, sjcr.ID)

	// GSI2 - Date range queries
	dateStr := sjcr.Timestamp.Format(common.CompactDateFormat)
	sjcr.GSI2PK = fmt.Sprintf("SCHEDULED_JOB_DATE#%s", dateStr)
	sjcr.GSI2SK = fmt.Sprintf("%s#%s#%s", timestampStr, sjcr.JobName, sjcr.ID)
}

// Validate performs validation on the ScheduledJobCostRecord
func (sjcr *ScheduledJobCostRecord) Validate() error {
	if strings.TrimSpace(sjcr.ID) == "" {
		return fmt.Errorf("ID is required")
	}
	if strings.TrimSpace(sjcr.JobName) == "" {
		return fmt.Errorf("JobName is required")
	}
	if strings.TrimSpace(sjcr.Schedule) == "" {
		return fmt.Errorf("schedule is required")
	}
	if strings.TrimSpace(sjcr.Status) == "" {
		return fmt.Errorf("Status is required")
	}
	if !isValidScheduledJobStatus(sjcr.Status) {
		return fmt.Errorf("invalid status: %s", sjcr.Status)
	}
	if !isValidSchedulePattern(sjcr.Schedule) {
		return fmt.Errorf("invalid schedule: %s", sjcr.Schedule)
	}

	return nil
}

// BeforeCreate for ScheduledJobCostAggregation
func (sjca *ScheduledJobCostAggregation) BeforeCreate() error {
	now := time.Now()
	sjca.CreatedAt = now
	sjca.UpdatedAt = now

	// Calculate total cost in dollars
	sjca.TotalCostDollars = float64(sjca.TotalCostMicroCents) / 1_000_000.0

	// Calculate averages
	if sjca.TotalExecutions > 0 {
		sjca.AverageCostPerExecution = sjca.TotalCostDollars / float64(sjca.TotalExecutions)
		sjca.SuccessRate = (float64(sjca.SuccessfulExecutions) / float64(sjca.TotalExecutions)) * 100
	}

	if sjca.TotalItemsProcessed > 0 {
		sjca.CostPerItemProcessed = sjca.TotalCostDollars / float64(sjca.TotalItemsProcessed)
	}

	if sjca.SuccessfulExecutions > 0 {
		sjca.CostPerSuccessfulExecution = sjca.TotalCostDollars / float64(sjca.SuccessfulExecutions)
	}

	// Set TTL (365 days for aggregated data)
	sjca.ExpiresAt = now.Add(365 * 24 * time.Hour).Unix()

	// Set up primary key
	sjca.PK = fmt.Sprintf("SCHEDULED_JOB_AGG#%s#%s", sjca.Period, sjca.JobName)
	sjca.SK = fmt.Sprintf("WINDOW#%s", sjca.WindowStart.Format(time.RFC3339))

	return sjca.Validate()
}

// BeforeUpdate for ScheduledJobCostAggregation
func (sjca *ScheduledJobCostAggregation) BeforeUpdate() error {
	sjca.UpdatedAt = time.Now()

	// Recalculate totals and averages
	sjca.TotalCostDollars = float64(sjca.TotalCostMicroCents) / 1_000_000.0

	if sjca.TotalExecutions > 0 {
		sjca.AverageCostPerExecution = sjca.TotalCostDollars / float64(sjca.TotalExecutions)
		sjca.SuccessRate = (float64(sjca.SuccessfulExecutions) / float64(sjca.TotalExecutions)) * 100
	}

	if sjca.TotalItemsProcessed > 0 {
		sjca.CostPerItemProcessed = sjca.TotalCostDollars / float64(sjca.TotalItemsProcessed)
	}

	if sjca.SuccessfulExecutions > 0 {
		sjca.CostPerSuccessfulExecution = sjca.TotalCostDollars / float64(sjca.SuccessfulExecutions)
	}

	return sjca.Validate()
}

// Validate for ScheduledJobCostAggregation
func (sjca *ScheduledJobCostAggregation) Validate() error {
	if strings.TrimSpace(sjca.JobName) == "" {
		return fmt.Errorf("JobName is required")
	}
	if strings.TrimSpace(sjca.Period) == "" {
		return fmt.Errorf("period is required")
	}
	if sjca.WindowStart.IsZero() {
		return fmt.Errorf("WindowStart is required")
	}
	if sjca.WindowEnd.IsZero() {
		return fmt.Errorf("WindowEnd is required")
	}
	if sjca.WindowEnd.Before(sjca.WindowStart) {
		return fmt.Errorf("WindowEnd must be after WindowStart")
	}
	if !isValidScheduledJobPeriod(sjca.Period) {
		return fmt.Errorf("invalid period: %s", sjca.Period)
	}

	return nil
}

// AddTag adds a tag to the scheduled job cost record
func (sjcr *ScheduledJobCostRecord) AddTag(key, value string) {
	if sjcr.Tags == nil {
		sjcr.Tags = make(map[string]string)
	}
	sjcr.Tags[key] = value
}

// SetJobProperty sets a job-specific property
func (sjcr *ScheduledJobCostRecord) SetJobProperty(key string, value interface{}) {
	if sjcr.JobProperties == nil {
		sjcr.JobProperties = make(map[string]interface{})
	}
	sjcr.JobProperties[key] = value
}

// GetJobProperty gets a job-specific property
func (sjcr *ScheduledJobCostRecord) GetJobProperty(key string) (interface{}, bool) {
	if sjcr.JobProperties == nil {
		return nil, false
	}
	value, exists := sjcr.JobProperties[key]
	return value, exists
}

// SetPerformanceMetric sets a performance metric
func (sjcr *ScheduledJobCostRecord) SetPerformanceMetric(key string, value float64) {
	if sjcr.PerformanceMetrics == nil {
		sjcr.PerformanceMetrics = make(map[string]float64)
	}
	sjcr.PerformanceMetrics[key] = value
}

// GetPerformanceMetric gets a performance metric
func (sjcr *ScheduledJobCostRecord) GetPerformanceMetric(key string) (float64, bool) {
	if sjcr.PerformanceMetrics == nil {
		return 0, false
	}
	value, exists := sjcr.PerformanceMetrics[key]
	return value, exists
}

// isValidScheduledJobStatus checks if the job status is valid
func isValidScheduledJobStatus(status string) bool {
	validStatuses := map[string]bool{
		StatusSuccess:   true,
		"failed":    true,
		"timeout":   true,
		"cancelled": true,
		"running":   true,
		"queued":    true,
	}
	return validStatuses[status]
}

// isValidSchedulePattern checks if the schedule pattern is valid
func isValidSchedulePattern(schedule string) bool {
	validSchedules := map[string]bool{
		"minutely": true,
		"hourly":   true,
		"daily":    true,
		"weekly":   true,
		"monthly":  true,
		"yearly":   true,
		"custom":   true,
	}
	return validSchedules[schedule]
}

// isValidScheduledJobPeriod checks if the period is valid for scheduled jobs
func isValidScheduledJobPeriod(period string) bool {
	validPeriods := map[string]bool{
		"minute": true,
		"hour":   true,
		"day":    true,
		"week":   true,
		"month":  true,
		"year":   true,
	}
	return validPeriods[period]
}

// ScheduledJobCostRecordBuilder helps create scheduled job cost tracking records
type ScheduledJobCostRecordBuilder struct {
	record *ScheduledJobCostRecord
}

// NewScheduledJobCostRecordBuilder creates a new builder
func NewScheduledJobCostRecordBuilder() *ScheduledJobCostRecordBuilder {
	return &ScheduledJobCostRecordBuilder{
		record: &ScheduledJobCostRecord{
			Tags:               make(map[string]string),
			JobProperties:      make(map[string]interface{}),
			PerformanceMetrics: make(map[string]float64),
			TriggeredJobs:      make([]string, 0),
		},
	}
}

// ForJob sets the job name and schedule
func (builder *ScheduledJobCostRecordBuilder) ForJob(jobName, schedule string) *ScheduledJobCostRecordBuilder {
	builder.record.JobName = jobName
	builder.record.Schedule = schedule
	return builder
}

// WithStatus sets the execution status
func (builder *ScheduledJobCostRecordBuilder) WithStatus(status string) *ScheduledJobCostRecordBuilder {
	builder.record.Status = status
	return builder
}

// WithTiming sets the execution timing
func (builder *ScheduledJobCostRecordBuilder) WithTiming(scheduledTime, startTime, endTime time.Time) *ScheduledJobCostRecordBuilder {
	builder.record.ScheduledTime = scheduledTime
	builder.record.StartTime = startTime
	builder.record.EndTime = endTime
	return builder
}

// WithLambdaUsage sets Lambda resource usage
func (builder *ScheduledJobCostRecordBuilder) WithLambdaUsage(invocations, durationMs int64, memoryMB int) *ScheduledJobCostRecordBuilder {
	builder.record.LambdaInvocations = invocations
	builder.record.LambdaDurationMs = durationMs
	builder.record.LambdaMemoryUsedMB = memoryMB
	return builder
}

// WithDynamoDBUsage sets DynamoDB resource usage
func (builder *ScheduledJobCostRecordBuilder) WithDynamoDBUsage(readOps, writeOps int64, readCapacity, writeCapacity float64) *ScheduledJobCostRecordBuilder {
	builder.record.DynamoDBReadOperations = readOps
	builder.record.DynamoDBWriteOperations = writeOps
	builder.record.DynamoDBReadCapacity = readCapacity
	builder.record.DynamoDBWriteCapacity = writeCapacity
	return builder
}

// WithCosts sets the cost breakdown in microcents
func (builder *ScheduledJobCostRecordBuilder) WithCosts(lambdaCost, dynamoDBCost, sqsCost, s3Cost, cloudWatchCost, dataTransferCost, externalAPICost int64) *ScheduledJobCostRecordBuilder {
	builder.record.LambdaCostMicroCents = lambdaCost
	builder.record.DynamoDBCostMicroCents = dynamoDBCost
	builder.record.SQSCostMicroCents = sqsCost
	builder.record.S3CostMicroCents = s3Cost
	builder.record.CloudWatchCostMicroCents = cloudWatchCost
	builder.record.DataTransferCostMicroCents = dataTransferCost
	builder.record.ExternalAPICostMicroCents = externalAPICost

	// Calculate total
	builder.record.TotalCostMicroCents = lambdaCost + dynamoDBCost + sqsCost + s3Cost + cloudWatchCost + dataTransferCost + externalAPICost
	return builder
}

// WithItemsProcessed sets the items processing metrics
func (builder *ScheduledJobCostRecordBuilder) WithItemsProcessed(processed, skipped, errored int64) *ScheduledJobCostRecordBuilder {
	builder.record.ItemsProcessed = processed
	builder.record.ItemsSkipped = skipped
	builder.record.ItemsErrored = errored
	return builder
}

// WithError sets error information
func (builder *ScheduledJobCostRecordBuilder) WithError(errorMessage string, retryCount, maxRetries int) *ScheduledJobCostRecordBuilder {
	builder.record.ErrorMessage = errorMessage
	builder.record.RetryCount = retryCount
	builder.record.MaxRetries = maxRetries
	return builder
}

// WithContext sets execution context
func (builder *ScheduledJobCostRecordBuilder) WithContext(environment, region, functionName, requestID string) *ScheduledJobCostRecordBuilder {
	builder.record.Environment = environment
	builder.record.Region = region
	builder.record.FunctionName = functionName
	builder.record.RequestID = requestID
	return builder
}

// WithCategory sets job category and priority
func (builder *ScheduledJobCostRecordBuilder) WithCategory(category, priority string) *ScheduledJobCostRecordBuilder {
	builder.record.JobCategory = category
	builder.record.Priority = priority
	return builder
}

// WithCascadingCosts sets cascading cost information
func (builder *ScheduledJobCostRecordBuilder) WithCascadingCosts(triggeredJobs []string, cascadingCost int64, downstreamOps int64) *ScheduledJobCostRecordBuilder {
	builder.record.TriggeredJobs = triggeredJobs
	builder.record.CascadingCostMicroCents = cascadingCost
	builder.record.DownstreamOperations = downstreamOps
	return builder
}

// WithTag adds a tag
func (builder *ScheduledJobCostRecordBuilder) WithTag(key, value string) *ScheduledJobCostRecordBuilder {
	builder.record.AddTag(key, value)
	return builder
}

// WithJobProperty sets a job property
func (builder *ScheduledJobCostRecordBuilder) WithJobProperty(key string, value interface{}) *ScheduledJobCostRecordBuilder {
	builder.record.SetJobProperty(key, value)
	return builder
}

// WithPerformanceMetric sets a performance metric
func (builder *ScheduledJobCostRecordBuilder) WithPerformanceMetric(key string, value float64) *ScheduledJobCostRecordBuilder {
	builder.record.SetPerformanceMetric(key, value)
	return builder
}

// Build creates the scheduled job cost record
func (builder *ScheduledJobCostRecordBuilder) Build() *ScheduledJobCostRecord {
	return builder.record
}

// Common AWS pricing constants for cost calculations (in microcents)
const (
	// Lambda pricing (per 1ms, 128MB)
	LambdaCostPerMSMicroCents = 2 // Approximately $0.0000000021 per ms

	// DynamoDB on-demand pricing (per 1000 units)
	DynamoDBReadCostMicroCentsPerUnit  = 25  // $0.25 per million reads = 25 microcents per 1000
	DynamoDBWriteCostMicroCentsPerUnit = 125 // $1.25 per million writes = 125 microcents per 1000

	// SQS pricing (per 1 million requests)
	SQSCostMicroCentsPerMessage = 40 // $0.40 per million = 40 microcents per 1000

	// S3 pricing (per 1000 requests)
	S3GetCostMicroCentsPerRequest = 40  // $0.0004 per 1000 GET requests
	S3PutCostMicroCentsPerRequest = 500 // $0.005 per 1000 PUT requests

	// CloudWatch Logs pricing (per GB)
	CloudWatchLogsCostMicroCentsPerMB = 50 // $0.50 per GB = 50 microcents per MB

	// Data transfer pricing (per GB)
	DataTransferCostMicroCentsPerMB = 9 // $0.09 per GB = 9 microcents per MB
)

// CalculateScheduledJobCosts calculates costs for a scheduled job based on resource usage
func CalculateScheduledJobCosts(lambdaDurationMs int64, memoryMB int, dynamoDBReadOps, dynamoDBWriteOps int64, sqsMessages int64, s3Operations int64, logSizeMB int64, dataTransferMB int64) (lambdaCost, dynamoDBCost, sqsCost, s3Cost, cloudWatchCost, dataTransferCost, totalCost int64) {
	// Lambda cost: duration * memory factor
	memoryFactor := float64(memoryMB) / 128.0 // Base is 128MB
	lambdaCost = int64(float64(lambdaDurationMs) * memoryFactor * float64(LambdaCostPerMSMicroCents))

	// DynamoDB cost
	dynamoDBCost = (dynamoDBReadOps * DynamoDBReadCostMicroCentsPerUnit / 1000) +
		(dynamoDBWriteOps * DynamoDBWriteCostMicroCentsPerUnit / 1000)

	// SQS cost
	sqsCost = sqsMessages * SQSCostMicroCentsPerMessage / 1000

	// S3 cost (assuming mostly GET operations)
	s3Cost = s3Operations * S3GetCostMicroCentsPerRequest / 1000

	// CloudWatch Logs cost
	cloudWatchCost = logSizeMB * CloudWatchLogsCostMicroCentsPerMB

	// Data transfer cost
	dataTransferCost = dataTransferMB * DataTransferCostMicroCentsPerMB

	// Total cost
	totalCost = lambdaCost + dynamoDBCost + sqsCost + s3Cost + cloudWatchCost + dataTransferCost

	return
}
