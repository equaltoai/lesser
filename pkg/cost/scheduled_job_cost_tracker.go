package cost

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// ScheduledJobCostRepository defines the interface for scheduled job cost persistence
type ScheduledJobCostRepository interface {
	Create(ctx context.Context, record *models.ScheduledJobCostRecord) error
	Update(ctx context.Context, record *models.ScheduledJobCostRecord) error
	GetByID(ctx context.Context, id string) (*models.ScheduledJobCostRecord, error)
	ListByJob(ctx context.Context, jobName, schedule string, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error)
	ListByStatus(ctx context.Context, status string, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error)
	ListByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ScheduledJobCostRecord, error)
	GetFailedJobs(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error)
	GetLongRunningJobs(ctx context.Context, thresholdMs int64, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error)
	GetHighCostJobs(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error)
	AggregateJobCosts(ctx context.Context, jobName, period string, windowStart, windowEnd time.Time) error
}

// ScheduledJobCostTracker tracks costs for scheduled/cron jobs
type ScheduledJobCostTracker struct {
	repository ScheduledJobCostRepository
	logger     *zap.Logger
}

// NewScheduledJobCostTracker creates a new scheduled job cost tracker
func NewScheduledJobCostTracker(repository ScheduledJobCostRepository, logger *zap.Logger) *ScheduledJobCostTracker {
	return &ScheduledJobCostTracker{
		repository: repository,
		logger:     logger,
	}
}

// JobExecution represents a tracked job execution
type JobExecution struct {
	// Job identification
	JobName     string
	Schedule    string
	CronPattern string
	Category    string
	Priority    string

	// Execution context
	Environment       string
	Region            string
	FunctionName      string
	RequestID         string
	ScheduledTime     time.Time
	NextScheduledTime time.Time

	// Execution timing
	StartTime time.Time
	EndTime   time.Time

	// Resource usage tracking
	lambdaInvocations     int64
	lambdaDurationMs      int64
	lambdaMemoryUsedMB    int
	dynamoDBReadOps       int64
	dynamoDBWriteOps      int64
	dynamoDBReadCapacity  float64
	dynamoDBWriteCapacity float64
	sqsMessages           int64
	s3Operations          int64
	cloudWatchLogs        int64
	dataTransferBytes     int64
	externalAPIRequests   int64

	// Business metrics
	itemsProcessed int64
	itemsSkipped   int64
	itemsErrored   int64
	batchSize      int

	// Error tracking
	errorMessage string
	retryCount   int
	maxRetries   int

	// Cascading tracking
	triggeredJobs           []string
	cascadingCostMicroCents int64
	downstreamOperations    int64

	// Custom properties and metrics
	properties         map[string]interface{}
	performanceMetrics map[string]float64
	tags               map[string]string

	// Internal tracking
	record *models.ScheduledJobCostRecord
}

// NewJobExecution creates a new job execution tracker
func NewJobExecution(jobName, schedule string) *JobExecution {
	return &JobExecution{
		JobName:            jobName,
		Schedule:           schedule,
		StartTime:          time.Now(),
		properties:         make(map[string]interface{}),
		performanceMetrics: make(map[string]float64),
		tags:               make(map[string]string),
		triggeredJobs:      make([]string, 0),
	}
}

// WithCategory sets the job category
func (je *JobExecution) WithCategory(category string) *JobExecution {
	je.Category = category
	return je
}

// WithPriority sets the job priority
func (je *JobExecution) WithPriority(priority string) *JobExecution {
	je.Priority = priority
	return je
}

// WithContext sets the execution context
func (je *JobExecution) WithContext(environment, region, functionName, requestID string) *JobExecution {
	je.Environment = environment
	je.Region = region
	je.FunctionName = functionName
	je.RequestID = requestID
	return je
}

// WithScheduling sets scheduling information
func (je *JobExecution) WithScheduling(cronPattern string, scheduledTime, nextScheduledTime time.Time) *JobExecution {
	je.CronPattern = cronPattern
	je.ScheduledTime = scheduledTime
	je.NextScheduledTime = nextScheduledTime
	return je
}

// TrackLambdaUsage tracks Lambda resource usage
func (je *JobExecution) TrackLambdaUsage(invocations, durationMs int64, memoryMB int) {
	je.lambdaInvocations += invocations
	je.lambdaDurationMs += durationMs
	if memoryMB > je.lambdaMemoryUsedMB {
		je.lambdaMemoryUsedMB = memoryMB // Track peak memory usage
	}
}

// TrackDynamoDBUsage tracks DynamoDB operations and capacity
func (je *JobExecution) TrackDynamoDBUsage(readOps, writeOps int64, readCapacity, writeCapacity float64) {
	je.dynamoDBReadOps += readOps
	je.dynamoDBWriteOps += writeOps
	je.dynamoDBReadCapacity += readCapacity
	je.dynamoDBWriteCapacity += writeCapacity
}

// TrackSQSUsage tracks SQS message processing
func (je *JobExecution) TrackSQSUsage(messages int64) {
	je.sqsMessages += messages
}

// TrackS3Usage tracks S3 operations
func (je *JobExecution) TrackS3Usage(operations int64) {
	je.s3Operations += operations
}

// TrackCloudWatchLogs tracks CloudWatch logs written
func (je *JobExecution) TrackCloudWatchLogs(logEntries int64) {
	je.cloudWatchLogs += logEntries
}

// TrackDataTransfer tracks data transfer volume
func (je *JobExecution) TrackDataTransfer(bytes int64) {
	je.dataTransferBytes += bytes
}

// TrackExternalAPIRequests tracks external API calls
func (je *JobExecution) TrackExternalAPIRequests(requests int64) {
	je.externalAPIRequests += requests
}

// TrackItemsProcessed tracks business logic metrics
func (je *JobExecution) TrackItemsProcessed(processed, skipped, errored int64) {
	je.itemsProcessed += processed
	je.itemsSkipped += skipped
	je.itemsErrored += errored
}

// SetBatchSize sets the batch size used
func (je *JobExecution) SetBatchSize(size int) {
	je.batchSize = size
}

// TrackCascadingCosts tracks costs of operations triggered by this job
func (je *JobExecution) TrackCascadingCosts(triggeredJobs []string, costMicroCents int64, downstreamOps int64) {
	je.triggeredJobs = append(je.triggeredJobs, triggeredJobs...)
	je.cascadingCostMicroCents += costMicroCents
	je.downstreamOperations += downstreamOps
}

// SetProperty sets a custom property
func (je *JobExecution) SetProperty(key string, value interface{}) {
	je.properties[key] = value
}

// SetPerformanceMetric sets a performance metric
func (je *JobExecution) SetPerformanceMetric(key string, value float64) {
	je.performanceMetrics[key] = value
}

// AddTag adds a tag
func (je *JobExecution) AddTag(key, value string) {
	je.tags[key] = value
}

// MarkError marks the job execution as failed with error details
func (je *JobExecution) MarkError(errorMessage string, retryCount, maxRetries int) {
	je.errorMessage = errorMessage
	je.retryCount = retryCount
	je.maxRetries = maxRetries
}

// FinishWithSuccess marks the job as successfully completed and persists the cost record
func (je *JobExecution) FinishWithSuccess(ctx context.Context, tracker *ScheduledJobCostTracker) error {
	je.EndTime = time.Now()
	return je.finishExecution(ctx, tracker, "success")
}

// FinishWithError marks the job as failed and persists the cost record
func (je *JobExecution) FinishWithError(ctx context.Context, tracker *ScheduledJobCostTracker, errorMessage string) error {
	je.EndTime = time.Now()
	je.errorMessage = errorMessage
	return je.finishExecution(ctx, tracker, "failed")
}

// FinishWithTimeout marks the job as timed out and persists the cost record
func (je *JobExecution) FinishWithTimeout(ctx context.Context, tracker *ScheduledJobCostTracker) error {
	je.EndTime = time.Now()
	return je.finishExecution(ctx, tracker, "timeout")
}

// FinishWithCancellation marks the job as cancelled and persists the cost record
func (je *JobExecution) FinishWithCancellation(ctx context.Context, tracker *ScheduledJobCostTracker) error {
	je.EndTime = time.Now()
	return je.finishExecution(ctx, tracker, "cancelled")
}

// finishExecution creates and persists the cost record
func (je *JobExecution) finishExecution(ctx context.Context, tracker *ScheduledJobCostTracker, status string) error {
	// Calculate costs based on resource usage
	lambdaCost, dynamoDBCost, sqsCost, s3Cost, cloudWatchCost, dataTransferCost := je.calculateCosts()

	// External API costs (placeholder - would need specific pricing)
	externalAPICost := je.externalAPIRequests * 100 // 100 microcents per request as example

	// Build the cost record
	record := models.NewScheduledJobCostRecordBuilder().
		ForJob(je.JobName, je.Schedule).
		WithStatus(status).
		WithTiming(je.ScheduledTime, je.StartTime, je.EndTime).
		WithLambdaUsage(je.lambdaInvocations, je.lambdaDurationMs, je.lambdaMemoryUsedMB).
		WithDynamoDBUsage(je.dynamoDBReadOps, je.dynamoDBWriteOps, je.dynamoDBReadCapacity, je.dynamoDBWriteCapacity).
		WithCosts(lambdaCost, dynamoDBCost, sqsCost, s3Cost, cloudWatchCost, dataTransferCost, externalAPICost).
		WithItemsProcessed(je.itemsProcessed, je.itemsSkipped, je.itemsErrored).
		WithContext(je.Environment, je.Region, je.FunctionName, je.RequestID).
		WithCategory(je.Category, je.Priority).
		WithCascadingCosts(je.triggeredJobs, je.cascadingCostMicroCents, je.downstreamOperations).
		Build()

	// Set additional fields
	record.CronPattern = je.CronPattern
	record.NextScheduledTime = je.NextScheduledTime
	record.BatchSize = je.batchSize
	record.SQSMessages = je.sqsMessages
	record.S3Operations = je.s3Operations
	record.CloudWatchLogs = je.cloudWatchLogs
	record.DataTransferBytes = je.dataTransferBytes
	record.ExternalAPIRequests = je.externalAPIRequests

	// Set error information if present
	if je.errorMessage != "" {
		record.ErrorMessage = je.errorMessage
		record.RetryCount = je.retryCount
		record.MaxRetries = je.maxRetries
	}

	// Set custom properties and metrics
	for key, value := range je.properties {
		record.SetJobProperty(key, value)
	}

	for key, value := range je.performanceMetrics {
		record.SetPerformanceMetric(key, value)
	}

	for key, value := range je.tags {
		record.AddTag(key, value)
	}

	// Persist the record
	err := tracker.repository.Create(ctx, record)
	if err != nil {
		tracker.logger.Error("failed to persist scheduled job cost record",
			zap.String("job_name", je.JobName),
			zap.String("schedule", je.Schedule),
			zap.String("status", status),
			zap.Error(err))
		return fmt.Errorf("failed to persist job cost record: %w", err)
	}

	je.record = record

	tracker.logger.Info("completed scheduled job execution tracking",
		zap.String("job_name", je.JobName),
		zap.String("schedule", je.Schedule),
		zap.String("status", status),
		zap.Int64("duration_ms", record.Duration),
		zap.Float64("cost_dollars", record.TotalCostDollars),
		zap.Int64("items_processed", je.itemsProcessed),
		zap.Int64("items_errored", je.itemsErrored))

	return nil
}

// calculateCosts calculates the cost breakdown based on resource usage
func (je *JobExecution) calculateCosts() (lambdaCost, dynamoDBCost, sqsCost, s3Cost, cloudWatchCost, dataTransferCost int64) {
	// Use the calculation functions from the models package
	logSizeMB := je.cloudWatchLogs / 1000 // Approximate log size
	dataTransferMB := je.dataTransferBytes / (1024 * 1024)

	lambdaCost, dynamoDBCost, sqsCost, s3Cost, cloudWatchCost, dataTransferCost, _ = models.CalculateScheduledJobCosts(
		je.lambdaDurationMs,
		je.lambdaMemoryUsedMB,
		je.dynamoDBReadOps,
		je.dynamoDBWriteOps,
		je.sqsMessages,
		je.s3Operations,
		logSizeMB,
		dataTransferMB,
	)

	return
}

// GetRecord returns the persisted cost record (if execution is finished)
func (je *JobExecution) GetRecord() *models.ScheduledJobCostRecord {
	return je.record
}

// Multi-step job support for complex workflows

// MultiStepJobExecution tracks a job with multiple steps/subtasks
type MultiStepJobExecution struct {
	*JobExecution
	steps       map[string]*JobStepExecution
	currentStep string
}

// JobStepExecution represents a single step in a multi-step job
type JobStepExecution struct {
	StepName  string
	StartTime time.Time
	EndTime   time.Time
	Status    string
	Error     string

	// Resource usage for this step
	lambdaDurationMs   int64
	dynamoDBOperations int64
	itemsProcessed     int64
	itemsErrored       int64

	// Step-specific metrics
	properties map[string]interface{}
	metrics    map[string]float64
}

// NewMultiStepJobExecution creates a new multi-step job execution tracker
func NewMultiStepJobExecution(jobName, schedule string) *MultiStepJobExecution {
	return &MultiStepJobExecution{
		JobExecution: NewJobExecution(jobName, schedule),
		steps:        make(map[string]*JobStepExecution),
	}
}

// StartStep starts tracking a new step
func (msje *MultiStepJobExecution) StartStep(stepName string) *JobStepExecution {
	step := &JobStepExecution{
		StepName:   stepName,
		StartTime:  time.Now(),
		Status:     "running",
		properties: make(map[string]interface{}),
		metrics:    make(map[string]float64),
	}

	msje.steps[stepName] = step
	msje.currentStep = stepName

	return step
}

// FinishStep completes tracking for a step
func (msje *MultiStepJobExecution) FinishStep(stepName, status string, err error) {
	step, exists := msje.steps[stepName]
	if !exists {
		return
	}

	step.EndTime = time.Now()
	step.Status = status
	if err != nil {
		step.Error = err.Error()
	}

	// Aggregate step metrics into job metrics
	msje.TrackLambdaUsage(1, step.lambdaDurationMs, 0)
	msje.TrackItemsProcessed(step.itemsProcessed, 0, step.itemsErrored)

	// Add step summary to job properties
	stepSummary := map[string]interface{}{
		"status":          step.Status,
		"duration_ms":     step.EndTime.Sub(step.StartTime).Milliseconds(),
		"items_processed": step.itemsProcessed,
		"items_errored":   step.itemsErrored,
	}

	if step.Error != "" {
		stepSummary["error"] = step.Error
	}

	msje.SetProperty(fmt.Sprintf("step_%s", stepName), stepSummary)
}

// GetCurrentStep returns the currently executing step
func (msje *MultiStepJobExecution) GetCurrentStep() *JobStepExecution {
	if msje.currentStep == "" {
		return nil
	}
	return msje.steps[msje.currentStep]
}

// GetStepSummary returns a summary of all steps
func (msje *MultiStepJobExecution) GetStepSummary() map[string]*JobStepExecution {
	return msje.steps
}

// TrackStepLambdaUsage tracks Lambda usage for the current step
func (step *JobStepExecution) TrackStepLambdaUsage(durationMs int64) {
	step.lambdaDurationMs += durationMs
}

// TrackStepDynamoDBUsage tracks DynamoDB usage for the current step
func (step *JobStepExecution) TrackStepDynamoDBUsage(operations int64) {
	step.dynamoDBOperations += operations
}

// TrackStepItemsProcessed tracks items processed for the current step
func (step *JobStepExecution) TrackStepItemsProcessed(processed, errored int64) {
	step.itemsProcessed += processed
	step.itemsErrored += errored
}

// SetStepProperty sets a step-specific property
func (step *JobStepExecution) SetStepProperty(key string, value interface{}) {
	step.properties[key] = value
}

// SetStepMetric sets a step-specific metric
func (step *JobStepExecution) SetStepMetric(key string, value float64) {
	step.metrics[key] = value
}

// Scheduled Job Cost Tracker Service Methods

// TrackCostAggregationJob tracks the cost aggregation job itself
func (tracker *ScheduledJobCostTracker) TrackCostAggregationJob(_ context.Context, period string, windowStart, windowEnd time.Time) (*JobExecution, error) {
	execution := NewJobExecution("cost-aggregation", "hourly").
		WithCategory("maintenance").
		WithPriority("normal")

	execution.SetProperty("aggregation_period", period)
	execution.SetProperty("window_start", windowStart)
	execution.SetProperty("window_end", windowEnd)

	return execution, nil
}

// TrackCleanupJob tracks cleanup jobs (expired data, old logs, etc.)
func (tracker *ScheduledJobCostTracker) TrackCleanupJob(_ context.Context, cleanupType string, schedule string) (*JobExecution, error) {
	execution := NewJobExecution(fmt.Sprintf("cleanup-%s", cleanupType), schedule).
		WithCategory("maintenance").
		WithPriority("low")

	execution.SetProperty("cleanup_type", cleanupType)

	return execution, nil
}

// TrackTrendCalculationJob tracks trend calculation jobs
func (tracker *ScheduledJobCostTracker) TrackTrendCalculationJob(_ context.Context, trendType string, schedule string) (*JobExecution, error) {
	execution := NewJobExecution(fmt.Sprintf("trend-calculation-%s", trendType), schedule).
		WithCategory("analytics").
		WithPriority("normal")

	execution.SetProperty("trend_type", trendType)

	return execution, nil
}

// TrackIndexOptimizationJob tracks database index optimization jobs
func (tracker *ScheduledJobCostTracker) TrackIndexOptimizationJob(_ context.Context, tableName string) (*JobExecution, error) {
	execution := NewJobExecution("index-optimization", "weekly").
		WithCategory("optimization").
		WithPriority("low")

	execution.SetProperty("table_name", tableName)

	return execution, nil
}

// TrackDeadLetterQueueProcessingJob tracks DLQ processing jobs
func (tracker *ScheduledJobCostTracker) TrackDeadLetterQueueProcessingJob(_ context.Context, queueName string) (*JobExecution, error) {
	execution := NewJobExecution("dlq-processing", "hourly").
		WithCategory("recovery").
		WithPriority("high")

	execution.SetProperty("queue_name", queueName)

	return execution, nil
}

// AggregateJobCosts performs aggregation of scheduled job costs
func (tracker *ScheduledJobCostTracker) AggregateJobCosts(ctx context.Context, jobName, period string, windowStart, windowEnd time.Time) error {
	tracker.logger.Info("starting scheduled job cost aggregation",
		zap.String("job_name", jobName),
		zap.String("period", period),
		zap.Time("window_start", windowStart),
		zap.Time("window_end", windowEnd))

	err := tracker.repository.AggregateJobCosts(ctx, jobName, period, windowStart, windowEnd)
	if err != nil {
		tracker.logger.Error("failed to aggregate scheduled job costs",
			zap.String("job_name", jobName),
			zap.String("period", period),
			zap.Error(err))
		return fmt.Errorf("failed to aggregate job costs: %w", err)
	}

	tracker.logger.Info("completed scheduled job cost aggregation",
		zap.String("job_name", jobName),
		zap.String("period", period))

	return nil
}

// Core tracking methods - advanced analytics would be handled by the repository implementation

// GetFailedJobs retrieves failed job executions
func (tracker *ScheduledJobCostTracker) GetFailedJobs(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	return tracker.repository.GetFailedJobs(ctx, startTime, endTime, limit)
}

// GetHighCostJobs retrieves high-cost job executions
func (tracker *ScheduledJobCostTracker) GetHighCostJobs(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	return tracker.repository.GetHighCostJobs(ctx, thresholdDollars, startTime, endTime, limit)
}

// GetLongRunningJobs retrieves long-running job executions
func (tracker *ScheduledJobCostTracker) GetLongRunningJobs(ctx context.Context, thresholdMs int64, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	return tracker.repository.GetLongRunningJobs(ctx, thresholdMs, startTime, endTime, limit)
}
