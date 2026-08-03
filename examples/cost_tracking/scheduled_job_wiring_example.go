//go:build example
// +build example

package examples

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// This file shows how to properly wire up scheduled job cost tracking
// to avoid import cycles while maintaining clean architecture.

// WireScheduledJobCostTracking creates a properly wired scheduled job cost tracker
// This function would typically be called from main.go or a dependency injection setup
func WireScheduledJobCostTracking(db core.DB, logger *zap.Logger) *cost.ScheduledJobCostTracker {
	// Create the concrete repository implementation
	repository := repositories.NewScheduledJobCostRepository(db, "lesser-main", logger)

	// Create the tracker with the repository interface
	tracker := cost.NewScheduledJobCostTracker(repository, logger)

	return tracker
}

// ExampleScheduledJobExecution shows how to use the tracker in a real scheduled job
func ExampleScheduledJobExecution(ctx context.Context, tracker *cost.ScheduledJobCostTracker, logger *zap.Logger) error {
	// Start tracking a scheduled job
	execution := cost.NewJobExecution("example-cleanup-job", "daily").
		WithCategory("maintenance").
		WithPriority("low").
		WithContext("production", "us-east-1", "cleanup-lambda", "req-12345")

	// Track some work being done
	execution.TrackLambdaUsage(5, 10000, 512)         // 5 invocations, 10 seconds total, 512MB
	execution.TrackDynamoDBUsage(100, 50, 50.0, 50.0) // 100 reads, 50 writes
	execution.TrackItemsProcessed(1000, 100, 5)       // 1000 processed, 100 skipped, 5 errors

	// Set job-specific properties
	execution.SetProperty("cleanup_type", "expired_tokens")
	execution.SetProperty("batch_size", 100)
	execution.SetPerformanceMetric("processing_rate", 100.0)
	execution.AddTag("service", "auth")

	// Complete the job tracking
	if execution.itemsErrored > 0 {
		return execution.FinishWithError(ctx, tracker, "some items failed processing")
	}

	return execution.FinishWithSuccess(ctx, tracker)
}

// ExampleRetrieveJobAnalytics shows how to retrieve and analyze job cost data
func ExampleRetrieveJobAnalytics(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -7) // Last 7 days

	// Get failed jobs
	failedJobs, err := tracker.GetFailedJobs(ctx, startTime, endTime, 10)
	if err != nil {
		return err
	}

	for _, job := range failedJobs {
		logger.Warn("failed scheduled job",
			zap.String("job_name", job.JobName),
			zap.String("schedule", job.Schedule),
			zap.Time("timestamp", job.Timestamp),
			zap.String("error", job.ErrorMessage),
			zap.Float64("cost_dollars", job.TotalCostDollars))
	}

	// Get high-cost jobs
	highCostJobs, err := tracker.GetHighCostJobs(ctx, 0.01, startTime, endTime, 10) // > $0.01
	if err != nil {
		return err
	}

	for _, job := range highCostJobs {
		logger.Info("high-cost scheduled job",
			zap.String("job_name", job.JobName),
			zap.Float64("cost_dollars", job.TotalCostDollars),
			zap.Int64("duration_ms", job.Duration),
			zap.Int64("items_processed", job.ItemsProcessed))
	}

	// Get long-running jobs
	longRunningJobs, err := tracker.GetLongRunningJobs(ctx, 300000, startTime, endTime, 10) // > 5 minutes
	if err != nil {
		return err
	}

	for _, job := range longRunningJobs {
		logger.Info("long-running scheduled job",
			zap.String("job_name", job.JobName),
			zap.Int64("duration_ms", job.Duration),
			zap.Float64("duration_minutes", float64(job.Duration)/60000),
			zap.Int64("items_processed", job.ItemsProcessed))
	}

	return nil
}

// ExampleMultiStepJobTracking shows how to track complex jobs with multiple steps
func ExampleMultiStepJobTracking(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {
	// Create multi-step job execution
	execution := NewMultiStepJobExecution("data-processing-pipeline", "hourly").
		WithCategory("analytics").
		WithPriority("normal").
		WithContext("production", "us-east-1", "pipeline-lambda", "req-67890")

	// Step 1: Data extraction
	step1 := execution.StartStep("extract_data")
	step1.TrackStepLambdaUsage(5000)      // 5 seconds
	step1.TrackStepDynamoDBUsage(200)     // 200 operations
	step1.TrackStepItemsProcessed(500, 0) // 500 items extracted
	step1.SetStepProperty("source", "user_activity_table")
	execution.FinishStep("extract_data", "success", nil)

	// Step 2: Data transformation
	step2 := execution.StartStep("transform_data")
	step2.TrackStepLambdaUsage(8000)       // 8 seconds
	step2.TrackStepItemsProcessed(500, 10) // 500 processed, 10 with errors
	step2.SetStepProperty("transformation_rules", "aggregate_by_user")
	execution.FinishStep("transform_data", "success", nil)

	// Step 3: Data loading
	step3 := execution.StartStep("load_data")
	step3.TrackStepLambdaUsage(3000)      // 3 seconds
	step3.TrackStepDynamoDBUsage(100)     // 100 write operations
	step3.TrackStepItemsProcessed(490, 0) // 490 items loaded (10 were errors)
	step3.SetStepProperty("destination", "analytics_table")
	execution.FinishStep("load_data", "success", nil)

	// Set overall job metrics
	execution.SetPerformanceMetric("data_quality_score", 0.98) // 98% success rate
	execution.SetPerformanceMetric("throughput", 490.0/16.0)   // items per second
	execution.AddTag("pipeline_version", "v2.1")

	// Complete the multi-step job
	return execution.FinishWithSuccess(ctx, tracker)
}

// ExampleJobCostAggregation shows how to perform cost aggregation
func ExampleJobCostAggregation(ctx context.Context, tracker *ScheduledJobCostTracker, logger *zap.Logger) error {
	// Aggregate costs for a specific job over the last day
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -1) // Last 24 hours

	// Perform hourly aggregation for all cleanup jobs
	err := tracker.AggregateJobCosts(ctx, "cleanup-expired-tokens", "hour", startTime, endTime)
	if err != nil {
		logger.Error("failed to aggregate cleanup job costs", zap.Error(err))
		return err
	}

	// Perform daily aggregation for analytics jobs
	err = tracker.AggregateJobCosts(ctx, "data-processing-pipeline", "day", startTime, endTime)
	if err != nil {
		logger.Error("failed to aggregate analytics job costs", zap.Error(err))
		return err
	}

	logger.Info("completed scheduled job cost aggregation",
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))

	return nil
}

// Integration patterns for different job types

// TrackCostAggregationJob is a helper for tracking cost aggregation jobs themselves
func TrackCostAggregationJob(ctx context.Context, tracker *ScheduledJobCostTracker, period string, windowStart, windowEnd time.Time) (*JobExecution, error) {
	execution := NewJobExecution("cost-aggregation", "hourly").
		WithCategory("maintenance").
		WithPriority("normal")

	execution.SetProperty("aggregation_period", period)
	execution.SetProperty("window_start", windowStart)
	execution.SetProperty("window_end", windowEnd)
	execution.AddTag("automation", "true")
	execution.AddTag("service", "cost-management")

	return execution, nil
}

// TrackDataCleanupJob is a helper for tracking data cleanup jobs
func TrackDataCleanupJob(ctx context.Context, tracker *ScheduledJobCostTracker, cleanupType, targetTable string) (*JobExecution, error) {
	execution := NewJobExecution(fmt.Sprintf("cleanup-%s", cleanupType), "daily").
		WithCategory("maintenance").
		WithPriority("low")

	execution.SetProperty("cleanup_type", cleanupType)
	execution.SetProperty("target_table", targetTable)
	execution.AddTag("cleanup", "true")
	execution.AddTag("data_management", "true")

	return execution, nil
}

// TrackAnalyticsJob is a helper for tracking analytics and reporting jobs
func TrackAnalyticsJob(ctx context.Context, tracker *ScheduledJobCostTracker, analysisType string, schedule string) (*JobExecution, error) {
	execution := NewJobExecution(fmt.Sprintf("analytics-%s", analysisType), schedule).
		WithCategory("analytics").
		WithPriority("normal")

	execution.SetProperty("analysis_type", analysisType)
	execution.AddTag("analytics", "true")
	execution.AddTag("reporting", "true")

	return execution, nil
}

// TrackMaintenanceJob is a helper for tracking system maintenance jobs
func TrackMaintenanceJob(ctx context.Context, tracker *ScheduledJobCostTracker, maintenanceType string, schedule string) (*JobExecution, error) {
	execution := NewJobExecution(fmt.Sprintf("maintenance-%s", maintenanceType), schedule).
		WithCategory("maintenance").
		WithPriority("low")

	execution.SetProperty("maintenance_type", maintenanceType)
	execution.AddTag("maintenance", "true")
	execution.AddTag("system", "true")

	return execution, nil
}
