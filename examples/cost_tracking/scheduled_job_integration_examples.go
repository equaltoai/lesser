//go:build example
// +build example

package examples

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// This file provides examples of how to integrate scheduled job cost tracking
// with various types of scheduled jobs in the Lesser system.

// Example 1: Cost Aggregation Job Integration
// This shows how to track the cost of the cost aggregation job itself

func ExampleCostAggregationJobWithTracking(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {

	// Start tracking the cost aggregation job
	execution := NewJobExecution("cost-aggregation", "hourly").
		WithCategory("maintenance").
		WithPriority("normal").
		WithContext("production", "us-east-1", "cost-aggregator", "req-123")

	execution.SetProperty("aggregation_type", "hourly_rollup")
	execution.AddTag("service", "cost-management")
	execution.AddTag("automation", "true")

	// Track the work being done
	startTime := time.Now()

	// Simulate cost aggregation work
	var itemsProcessed int64
	var dynamoDBOps int64

	// Example: Process cost records for aggregation
	operationTypes := []string{"GetItem", "PutItem", "Query", "Scan"}
	for _, opType := range operationTypes {
		// Simulate processing each operation type
		execution.TrackLambdaUsage(1, 2000, 512) // 1 invocation, 2 seconds, 512MB

		// Simulate DynamoDB operations
		readOps := int64(100)
		writeOps := int64(50)
		execution.TrackDynamoDBUsage(readOps, writeOps, float64(readOps)*0.5, float64(writeOps)*1.0)
		dynamoDBOps += readOps + writeOps

		// Track items processed
		processed := int64(1000)
		execution.TrackItemsProcessed(processed, 0, 0)
		itemsProcessed += processed

		logger.Info("processed operation type for aggregation",
			zap.String("operation_type", opType),
			zap.Int64("items_processed", processed))
	}

	// Track performance metrics
	duration := time.Since(startTime)
	execution.SetPerformanceMetric("processing_rate", float64(itemsProcessed)/duration.Seconds())
	execution.SetPerformanceMetric("db_efficiency", float64(itemsProcessed)/float64(dynamoDBOps))

	// Complete the job tracking
	err := execution.FinishWithSuccess(ctx, tracker)
	if err != nil {
		logger.Error("failed to track cost aggregation job", zap.Error(err))
		return err
	}

	logger.Info("completed cost aggregation job with tracking",
		zap.Int64("items_processed", itemsProcessed),
		zap.Int64("dynamodb_operations", dynamoDBOps),
		zap.Float64("cost_dollars", execution.GetRecord().TotalCostDollars))

	return nil
}

// Example 2: Cleanup Job Integration
// This shows how to track cleanup jobs that remove expired data

func ExampleCleanupJobWithTracking(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {

	// Start tracking the cleanup job
	execution := NewJobExecution("cleanup-expired-tokens", "daily").
		WithCategory("maintenance").
		WithPriority("low").
		WithContext("production", "us-east-1", "cleanup-service", "req-456")

	execution.SetProperty("cleanup_type", "expired_tokens")
	execution.SetProperty("retention_days", 90)
	execution.AddTag("service", "auth")
	execution.AddTag("cleanup", "true")

	// Multi-step cleanup process
	multiStepExecution := NewMultiStepJobExecution("cleanup-expired-tokens", "daily")
	multiStepExecution.JobExecution = execution

	// Step 1: Find expired tokens
	step1 := multiStepExecution.StartStep("find_expired_tokens")

	// Simulate querying for expired tokens
	step1.TrackStepLambdaUsage(5000)  // 5 seconds
	step1.TrackStepDynamoDBUsage(500) // 500 operations
	expiredTokens := int64(250)
	step1.TrackStepItemsProcessed(expiredTokens, 0)
	step1.SetStepProperty("tokens_found", expiredTokens)

	multiStepExecution.FinishStep("find_expired_tokens", "success", nil)

	// Step 2: Delete expired tokens
	step2 := multiStepExecution.StartStep("delete_expired_tokens")

	// Simulate batch deletion
	batchSize := 25
	batches := int(expiredTokens) / batchSize
	multiStepExecution.SetBatchSize(batchSize)

	var deletedTokens int64
	for i := 0; i < batches; i++ {
		step2.TrackStepLambdaUsage(1000)               // 1 second per batch
		step2.TrackStepDynamoDBUsage(int64(batchSize)) // Delete operations
		deletedTokens += int64(batchSize)

		// Track in main execution
		multiStepExecution.TrackDynamoDBUsage(0, int64(batchSize), 0, float64(batchSize))
		multiStepExecution.TrackItemsProcessed(int64(batchSize), 0, 0)
	}

	step2.TrackStepItemsProcessed(deletedTokens, 0)
	step2.SetStepProperty("tokens_deleted", deletedTokens)
	multiStepExecution.FinishStep("delete_expired_tokens", "success", nil)

	// Step 3: Update cleanup metrics
	step3 := multiStepExecution.StartStep("update_metrics")
	step3.TrackStepLambdaUsage(500) // 0.5 seconds
	step3.TrackStepDynamoDBUsage(5) // Update metrics
	multiStepExecution.FinishStep("update_metrics", "success", nil)

	// Set performance metrics
	multiStepExecution.SetPerformanceMetric("cleanup_efficiency", float64(deletedTokens)/float64(expiredTokens))
	multiStepExecution.SetPerformanceMetric("batch_processing_rate", float64(deletedTokens)/float64(batches))

	// Complete the job tracking
	err := multiStepExecution.FinishWithSuccess(ctx, tracker)
	if err != nil {
		logger.Error("failed to track cleanup job", zap.Error(err))
		return err
	}

	logger.Info("completed cleanup job with tracking",
		zap.Int64("tokens_processed", expiredTokens),
		zap.Int64("tokens_deleted", deletedTokens),
		zap.Int("steps_completed", len(multiStepExecution.GetStepSummary())))

	return nil
}

// Example 3: Trend Calculation Job Integration
// This shows how to track analytical jobs that calculate trends

func ExampleTrendCalculationJobWithTracking(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {

	// Start tracking the trend calculation job
	execution := NewJobExecution("trend-calculation-user-activity", "daily").
		WithCategory("analytics").
		WithPriority("normal").
		WithContext("production", "us-east-1", "analytics-service", "req-789")

	execution.SetProperty("trend_type", "user_activity")
	execution.SetProperty("calculation_window_days", 30)
	execution.AddTag("service", "analytics")
	execution.AddTag("data_processing", "true")

	// Track external API calls for data enrichment
	externalAPICalls := int64(50)
	execution.TrackExternalAPIRequests(externalAPICalls)

	// Simulate trend calculation work
	var trendsCalculated int64
	var dataPointsProcessed int64

	// Process user activity data
	userBatches := 100
	for i := 0; i < userBatches; i++ {
		// Track processing each batch of users
		execution.TrackLambdaUsage(1, 3000, 1024) // 3 seconds per batch, 1GB memory

		// Read user activity data
		readOps := int64(200)
		execution.TrackDynamoDBUsage(readOps, 0, float64(readOps)*0.5, 0)

		// Write trend data
		writeOps := int64(10)
		execution.TrackDynamoDBUsage(0, writeOps, 0, float64(writeOps)*1.0)

		// Track business metrics
		batchDataPoints := int64(2000)
		batchTrends := int64(50)
		execution.TrackItemsProcessed(batchDataPoints, 0, 0)

		dataPointsProcessed += batchDataPoints
		trendsCalculated += batchTrends
	}

	// Track data transfer for storing results
	resultDataMB := int64(10)                               // 10MB of trend data
	execution.TrackDataTransfer(resultDataMB * 1024 * 1024) // Convert to bytes

	// Track S3 operations for storing trend reports
	s3Operations := int64(25)
	execution.TrackS3Usage(s3Operations)

	// Set performance metrics
	totalDuration := time.Since(execution.StartTime)
	execution.SetPerformanceMetric("data_processing_rate", float64(dataPointsProcessed)/totalDuration.Seconds())
	execution.SetPerformanceMetric("trend_accuracy", 0.95) // Example accuracy metric
	execution.SetPerformanceMetric("computational_efficiency", float64(trendsCalculated)/float64(userBatches))

	// Set job-specific properties
	execution.SetProperty("trends_calculated", trendsCalculated)
	execution.SetProperty("data_points_processed", dataPointsProcessed)
	execution.SetProperty("output_size_mb", resultDataMB)

	// Complete the job tracking
	err := execution.FinishWithSuccess(ctx, tracker)
	if err != nil {
		logger.Error("failed to track trend calculation job", zap.Error(err))
		return err
	}

	logger.Info("completed trend calculation job with tracking",
		zap.Int64("trends_calculated", trendsCalculated),
		zap.Int64("data_points_processed", dataPointsProcessed),
		zap.Float64("cost_dollars", execution.GetRecord().TotalCostDollars))

	return nil
}

// Example 4: Index Optimization Job Integration
// This shows how to track database optimization jobs

func ExampleIndexOptimizationJobWithTracking(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {

	// Start tracking the index optimization job
	execution := NewJobExecution("index-optimization", "weekly").
		WithCategory("optimization").
		WithPriority("low").
		WithContext("production", "us-east-1", "optimization-service", "req-101112")

	execution.SetProperty("optimization_type", "gsi_usage_analysis")
	execution.SetProperty("table_name", "lesser-main")
	execution.AddTag("service", "database")
	execution.AddTag("optimization", "true")

	// Track CloudWatch metrics analysis
	execution.TrackCloudWatchLogs(1000) // 1000 log entries analyzed

	// Simulate index analysis work
	var indicesAnalyzed int64
	var recommendationsGenerated int64

	// Analyze GSI usage patterns
	gsiList := []string{"user-index", "status-index", "activity-index", "federation-index"}
	for _, gsiName := range gsiList {
		// Track analysis of each GSI
		execution.TrackLambdaUsage(1, 10000, 512) // 10 seconds per GSI, 512MB

		// Query usage metrics from CloudWatch/DynamoDB
		readOps := int64(500)
		execution.TrackDynamoDBUsage(readOps, 0, float64(readOps)*0.5, 0)

		indicesAnalyzed++

		// Generate recommendations based on usage patterns
		recommendationsForGSI := int64(3) // Average 3 recommendations per GSI
		recommendationsGenerated += recommendationsForGSI

		execution.SetProperty(fmt.Sprintf("gsi_%s_recommendations", gsiName), recommendationsForGSI)

		logger.Debug("analyzed GSI usage patterns",
			zap.String("gsi_name", gsiName),
			zap.Int64("recommendations", recommendationsForGSI))
	}

	// Generate optimization report
	execution.TrackS3Usage(5) // Store report in S3
	reportSize := int64(2)    // 2MB report
	execution.TrackDataTransfer(reportSize * 1024 * 1024)

	// Track performance metrics
	execution.SetPerformanceMetric("analysis_efficiency", float64(indicesAnalyzed)/time.Since(execution.StartTime).Hours())
	execution.SetPerformanceMetric("recommendation_density", float64(recommendationsGenerated)/float64(indicesAnalyzed))

	// Set job-specific properties
	execution.SetProperty("indices_analyzed", indicesAnalyzed)
	execution.SetProperty("recommendations_generated", recommendationsGenerated)
	execution.SetProperty("report_size_mb", reportSize)

	// Track items processed (indices analyzed)
	execution.TrackItemsProcessed(indicesAnalyzed, 0, 0)

	// Complete the job tracking
	err := execution.FinishWithSuccess(ctx, tracker)
	if err != nil {
		logger.Error("failed to track index optimization job", zap.Error(err))
		return err
	}

	logger.Info("completed index optimization job with tracking",
		zap.Int64("indices_analyzed", indicesAnalyzed),
		zap.Int64("recommendations_generated", recommendationsGenerated),
		zap.Float64("cost_dollars", execution.GetRecord().TotalCostDollars))

	return nil
}

// Example 5: Dead Letter Queue Processing Job Integration
// This shows how to track error recovery jobs

func ExampleDLQProcessingJobWithTracking(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {

	// Start tracking the DLQ processing job
	execution := NewJobExecution("dlq-processing", "hourly").
		WithCategory("recovery").
		WithPriority("high").
		WithContext("production", "us-east-1", "dlq-processor", "req-131415")

	execution.SetProperty("queue_name", "federation-dlq")
	execution.SetProperty("max_retry_attempts", 3)
	execution.AddTag("service", "federation")
	execution.AddTag("error_recovery", "true")

	// Track SQS message processing
	var messagesProcessed int64
	var messagesReprocessed int64
	var messagesDiscarded int64
	var retriesPerformed int

	// Simulate processing messages from DLQ
	dlqMessages := int64(150)
	execution.TrackSQSUsage(dlqMessages) // Track SQS operations

	for i := int64(0); i < dlqMessages; i++ {
		// Track processing each message
		execution.TrackLambdaUsage(1, 2000, 256) // 2 seconds per message, 256MB

		// Simulate message analysis and reprocessing attempt
		messageProcessingSuccessful := i%4 != 0 // 75% success rate

		if messageProcessingSuccessful {
			// Successfully reprocessed - write back to main queue or process directly
			execution.TrackDynamoDBUsage(0, 2, 0, 2.0) // 2 write operations
			messagesReprocessed++
		} else {
			// Failed to reprocess - check retry count
			if retriesPerformed < 3 {
				retriesPerformed++
				execution.TrackSQSUsage(1) // Put back in DLQ for retry
			} else {
				// Max retries exceeded - move to permanent failure storage
				execution.TrackS3Usage(1) // Store failed message
				messagesDiscarded++
			}
		}

		messagesProcessed++
	}

	// Track cascading costs (messages that triggered downstream processing)
	downstreamProcessingCost := messagesReprocessed * 50 // 50 microcents per downstream message
	triggeredJobs := []string{"federation-processor", "activity-processor"}
	execution.TrackCascadingCosts(triggeredJobs, downstreamProcessingCost, messagesReprocessed)

	// Track performance metrics
	processingDuration := time.Since(execution.StartTime)
	execution.SetPerformanceMetric("processing_rate", float64(messagesProcessed)/processingDuration.Seconds())
	execution.SetPerformanceMetric("reprocessing_success_rate", float64(messagesReprocessed)/float64(messagesProcessed))
	execution.SetPerformanceMetric("message_recovery_rate", float64(messagesReprocessed)/(float64(messagesReprocessed)+float64(messagesDiscarded)))

	// Set job-specific properties
	execution.SetProperty("messages_processed", messagesProcessed)
	execution.SetProperty("messages_reprocessed", messagesReprocessed)
	execution.SetProperty("messages_discarded", messagesDiscarded)
	execution.SetProperty("retry_attempts", retriesPerformed)

	// Track items processed (messages processed, skipped discarded, errored failed)
	execution.TrackItemsProcessed(messagesReprocessed, 0, messagesDiscarded)

	// Complete the job tracking
	status := "success"
	if messagesDiscarded > messagesReprocessed {
		status = "failed"
		execution.MarkError(fmt.Sprintf("High failure rate: %d messages discarded vs %d reprocessed", messagesDiscarded, messagesReprocessed), 0, 0)
	}

	var err error
	if status == "success" {
		err = execution.FinishWithSuccess(ctx, tracker)
	} else {
		err = execution.FinishWithError(ctx, tracker, execution.errorMessage)
	}

	if err != nil {
		logger.Error("failed to track DLQ processing job", zap.Error(err))
		return err
	}

	logger.Info("completed DLQ processing job with tracking",
		zap.Int64("messages_processed", messagesProcessed),
		zap.Int64("messages_reprocessed", messagesReprocessed),
		zap.Int64("messages_discarded", messagesDiscarded),
		zap.String("final_status", status))

	return nil
}

// Example 6: Cost Aggregation and Analysis Integration
// This shows how to analyze scheduled job costs themselves

func ExampleScheduledJobCostAnalysis(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {

	// Get summary of all scheduled jobs for the last 7 days
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -7)

	logger.Info("analyzing scheduled jobs cost data",
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))

	// Get high-cost jobs
	highCostJobs, err := tracker.GetHighCostJobs(ctx, 0.01, startTime, endTime, 10) // Jobs costing more than $0.01
	if err != nil {
		logger.Error("failed to get high-cost jobs", zap.Error(err))
		return err
	}

	for _, job := range highCostJobs {
		logger.Warn("high-cost job execution",
			zap.String("job_name", job.JobName),
			zap.String("schedule", job.Schedule),
			zap.Time("timestamp", job.Timestamp),
			zap.Float64("cost_dollars", job.TotalCostDollars),
			zap.Int64("duration_ms", job.Duration),
			zap.String("status", job.Status))
	}

	// Get failed jobs
	failedJobs, err := tracker.GetFailedJobs(ctx, startTime, endTime, 10)
	if err != nil {
		logger.Error("failed to get failed jobs", zap.Error(err))
		return err
	}

	for _, job := range failedJobs {
		logger.Error("failed job execution",
			zap.String("job_name", job.JobName),
			zap.String("schedule", job.Schedule),
			zap.Time("timestamp", job.Timestamp),
			zap.String("error_message", job.ErrorMessage),
			zap.Int("retry_count", job.RetryCount),
			zap.Float64("cost_dollars", job.TotalCostDollars))
	}

	logger.Info("job cost analysis completed successfully")

	return nil
}

// Helper function to create scheduled job cost tracker with a repository implementation
// The repository would be created elsewhere to avoid import cycles

// Example integration with existing cron jobs or EventBridge rules
func ExampleIntegratingWithExistingScheduledJobs(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) {
	// This example shows how to modify existing scheduled Lambda functions
	// to include cost tracking

	logger.Info("example of integrating cost tracking with existing scheduled jobs",
		zap.String("approach", "wrap_existing_handlers"))

	// Example pattern for wrapping existing job handlers:
	/*
		Original handler:
		func handleCostAggregation(ctx context.Context, event events.CloudWatchEvent) error {
			// existing job logic
			return nil
		}

		Modified handler with cost tracking:
		func handleCostAggregationWithTracking(ctx context.Context, event events.CloudWatchEvent) error {
			// Initialize cost tracking
			execution := tracker.TrackCostAggregationJob(ctx, "hourly", windowStart, windowEnd)

			// Execute original logic with tracking
			err := executeOriginalLogic(ctx, execution)

			// Complete tracking
			if err != nil {
				return execution.FinishWithError(ctx, tracker, err.Error())
			}
			return execution.FinishWithSuccess(ctx, tracker)
		}
	*/
}
