package repositories

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// ScheduledJobCostRepository handles scheduled job cost tracking persistence
type ScheduledJobCostRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewScheduledJobCostRepository creates a new scheduled job cost repository
func NewScheduledJobCostRepository(db core.DB, tableName string, logger *zap.Logger) *ScheduledJobCostRepository {
	return &ScheduledJobCostRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Create creates a new scheduled job cost record
func (r *ScheduledJobCostRepository) Create(ctx context.Context, record *models.ScheduledJobCostRecord) error {
	// Call BeforeCreate to set up the model
	if err := record.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the scheduled job cost record
	err := r.db.WithContext(ctx).Model(record).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create scheduled job cost record")
	}

	r.logger.Debug("created scheduled job cost record",
		zap.String("id", record.ID),
		zap.String("job_name", record.JobName),
		zap.String("schedule", record.Schedule),
		zap.String("status", record.Status),
		zap.Float64("cost_dollars", record.TotalCostDollars),
		zap.Int64("duration_ms", record.Duration))

	return nil
}

// Update updates an existing scheduled job cost record
func (r *ScheduledJobCostRepository) Update(ctx context.Context, record *models.ScheduledJobCostRecord) error {
	// Call BeforeUpdate to set up the model
	if err := record.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the scheduled job cost record
	err := r.db.WithContext(ctx).Model(record).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update scheduled job cost record")
	}

	r.logger.Debug("updated scheduled job cost record",
		zap.String("id", record.ID),
		zap.String("job_name", record.JobName),
		zap.String("status", record.Status))

	return nil
}

// Get retrieves a scheduled job cost record by job name, schedule, timestamp and ID
func (r *ScheduledJobCostRepository) Get(ctx context.Context, jobName, schedule string, timestamp time.Time, id string) (*models.ScheduledJobCostRecord, error) {
	record := &models.ScheduledJobCostRecord{}

	// Construct the keys
	pk := fmt.Sprintf("SCHEDULED_JOB_COST#%s#%s", jobName, schedule)
	sk := fmt.Sprintf("RUN#%s#%s", timestamp.Format("20060102150405"), id)

	err := r.db.WithContext(ctx).Model(record).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(record)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get scheduled job cost record")
	}

	return record, nil
}

// GetByID retrieves a scheduled job cost record by ID
func (r *ScheduledJobCostRepository) GetByID(ctx context.Context, id string) (*models.ScheduledJobCostRecord, error) {
	// Query across all job status partitions to find the record by ID
	statuses := []string{"success", "failed", "timeout", "cancelled", "running", "queued"}
	
	for _, status := range statuses {
		var statusRecords []*models.ScheduledJobCostRecord
		
		err := r.db.WithContext(ctx).Model(&models.ScheduledJobCostRecord{}).
			Index("job-status-index").
			Where("GSI1PK", "=", fmt.Sprintf("SCHEDULED_JOB_STATUS#%s", status)).
			Where("GSI1SK", "contains", fmt.Sprintf("#%s", id)).
			All(&statusRecords)

		if err != nil {
			r.logger.Warn("failed to query job cost records by status",
				zap.String("status", status),
				zap.Error(err))
			continue
		}

		// Filter by exact ID match
		for _, record := range statusRecords {
			if record.ID == id {
				return record, nil
			}
		}
	}

	return nil, fmt.Errorf("scheduled job cost record not found with ID: %s", id)
}

// ListByJob lists scheduled job cost records for a specific job within a time range
func (r *ScheduledJobCostRepository) ListByJob(ctx context.Context, jobName, schedule string, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	var records []*models.ScheduledJobCostRecord

	// Construct SK range for time-based query
	pk := fmt.Sprintf("SCHEDULED_JOB_COST#%s#%s", jobName, schedule)
	startSK := fmt.Sprintf("RUN#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("RUN#%s", endTime.Format("20060102150405"))

	query := r.db.WithContext(ctx).Model(&models.ScheduledJobCostRecord{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&records)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list scheduled job cost records by job")
	}

	return records, nil
}

// ListByStatus lists scheduled job cost records by status within a time range
func (r *ScheduledJobCostRepository) ListByStatus(ctx context.Context, status string, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	var records []*models.ScheduledJobCostRecord

	// Use GSI1 for status-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.WithContext(ctx).Model(&models.ScheduledJobCostRecord{}).
		Index("job-status-index").
		Where("GSI1PK", "=", fmt.Sprintf("SCHEDULED_JOB_STATUS#%s", status)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&records)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list scheduled job cost records by status")
	}

	return records, nil
}

// ListByDateRange lists scheduled job cost records across all jobs within a date range
func (r *ScheduledJobCostRepository) ListByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	var allRecords []*models.ScheduledJobCostRecord

	// Query by daily partitions using GSI2
	currentDate := startDate
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		dateStr := currentDate.Format("20060102")
		
		var dailyRecords []*models.ScheduledJobCostRecord
		query := r.db.WithContext(ctx).Model(&models.ScheduledJobCostRecord{}).
			Index("job-date-index").
			Where("GSI2PK", "=", fmt.Sprintf("SCHEDULED_JOB_DATE#%s", dateStr)).
			OrderBy("GSI2SK", "DESC").
			Limit(limit)

		err := query.All(&dailyRecords)
		if err != nil {
			r.logger.Warn("failed to get scheduled job costs for date",
				zap.String("date", dateStr),
				zap.Error(err))
			// Continue with next date
		} else {
			allRecords = append(allRecords, dailyRecords...)
		}

		// Move to next day
		currentDate = currentDate.AddDate(0, 0, 1)
		
		// Break if we have enough results
		if len(allRecords) >= limit {
			break
		}
	}

	// Sort by timestamp (newest first) and limit
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].Timestamp.After(allRecords[j].Timestamp)
	})
	
	if len(allRecords) > limit {
		allRecords = allRecords[:limit]
	}

	return allRecords, nil
}

// GetFailedJobs returns failed job executions within a time range
func (r *ScheduledJobCostRepository) GetFailedJobs(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	return r.ListByStatus(ctx, "failed", startTime, endTime, limit)
}

// GetLongRunningJobs returns jobs that exceeded a duration threshold
func (r *ScheduledJobCostRepository) GetLongRunningJobs(ctx context.Context, thresholdMs int64, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	// Get all successful jobs first, then filter by duration
	allJobs, err := r.ListByStatus(ctx, "success", startTime, endTime, limit*5) // Get more to filter
	if err != nil {
		return nil, err
	}

	var longRunningJobs []*models.ScheduledJobCostRecord
	for _, job := range allJobs {
		if job.Duration >= thresholdMs {
			longRunningJobs = append(longRunningJobs, job)
			if len(longRunningJobs) >= limit {
				break
			}
		}
	}

	return longRunningJobs, nil
}

// GetHighCostJobs returns jobs that exceeded a cost threshold
func (r *ScheduledJobCostRepository) GetHighCostJobs(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	// Get recent jobs across all statuses
	allJobs, err := r.ListByDateRange(ctx, startTime, endTime, limit*10) // Get more to filter
	if err != nil {
		return nil, err
	}

	var highCostJobs []*models.ScheduledJobCostRecord
	for _, job := range allJobs {
		if job.TotalCostDollars >= thresholdDollars {
			highCostJobs = append(highCostJobs, job)
			if len(highCostJobs) >= limit {
				break
			}
		}
	}

	// Sort by cost (highest first)
	sort.Slice(highCostJobs, func(i, j int) bool {
		return highCostJobs[i].TotalCostDollars > highCostJobs[j].TotalCostDollars
	})

	return highCostJobs, nil
}

// GetJobExecutionStats calculates execution statistics for a job
func (r *ScheduledJobCostRepository) GetJobExecutionStats(ctx context.Context, jobName, schedule string, startTime, endTime time.Time) (*JobExecutionStats, error) {
	records, err := r.ListByJob(ctx, jobName, schedule, startTime, endTime, 10000) // Get all records
	if err != nil {
		return nil, err
	}

	stats := &JobExecutionStats{
		JobName:     jobName,
		Schedule:    schedule,
		StartTime:   startTime,
		EndTime:     endTime,
		TotalRuns:   len(records),
		StatusBreakdown: make(map[string]int64),
		CategoryBreakdown: make(map[string]*JobCategoryStats),
	}

	if stats.TotalRuns == 0 {
		return stats, nil
	}

	// Collect values for percentile calculations
	var durations []float64
	var costs []float64
	var totalDuration int64
	var totalCost float64

	for _, record := range records {
		// Count by status
		stats.StatusBreakdown[record.Status]++
		
		// Track success/failure
		if record.Success {
			stats.SuccessfulRuns++
		} else {
			stats.FailedRuns++
		}

		// Collect duration and cost data
		durations = append(durations, float64(record.Duration))
		costs = append(costs, record.TotalCostDollars)
		totalDuration += record.Duration
		totalCost += record.TotalCostDollars

		stats.TotalItemsProcessed += record.ItemsProcessed
		stats.TotalItemsErrored += record.ItemsErrored
		stats.TotalCostMicroCents += record.TotalCostMicroCents

		// Track lambda usage
		stats.TotalLambdaInvocations += record.LambdaInvocations
		stats.TotalLambdaDurationMs += record.LambdaDurationMs

		// Track by category
		if record.JobCategory != "" {
			categoryStats, exists := stats.CategoryBreakdown[record.JobCategory]
			if !exists {
				categoryStats = &JobCategoryStats{
					Category: record.JobCategory,
				}
				stats.CategoryBreakdown[record.JobCategory] = categoryStats
			}
			
			categoryStats.ExecutionCount++
			categoryStats.TotalCostMicroCents += record.TotalCostMicroCents
			categoryStats.TotalDurationMs += record.Duration
			if record.Success {
				categoryStats.SuccessfulExecutions++
			}
		}
	}

	// Calculate averages and percentiles
	stats.AverageDurationMs = float64(totalDuration) / float64(stats.TotalRuns)
	stats.AverageCostDollars = totalCost / float64(stats.TotalRuns)
	stats.TotalCostDollars = totalCost
	stats.SuccessRate = (float64(stats.SuccessfulRuns) / float64(stats.TotalRuns)) * 100

	// Calculate efficiency metrics
	if stats.TotalItemsProcessed > 0 {
		stats.CostPerItemProcessed = totalCost / float64(stats.TotalItemsProcessed)
	}
	if stats.SuccessfulRuns > 0 {
		stats.CostPerSuccessfulExecution = totalCost / float64(stats.SuccessfulRuns)
	}

	// Calculate percentiles
	stats.DurationPercentiles = calculatePercentiles(durations)
	stats.CostPercentiles = calculatePercentiles(costs)

	// Calculate category statistics
	for category, categoryStats := range stats.CategoryBreakdown {
		categoryStats.TotalCostDollars = float64(categoryStats.TotalCostMicroCents) / 1_000_000.0
		if categoryStats.ExecutionCount > 0 {
			categoryStats.AverageCostPerExecution = categoryStats.TotalCostDollars / float64(categoryStats.ExecutionCount)
			categoryStats.AverageDurationMs = float64(categoryStats.TotalDurationMs) / float64(categoryStats.ExecutionCount)
			categoryStats.SuccessRate = (float64(categoryStats.SuccessfulExecutions) / float64(categoryStats.ExecutionCount)) * 100
		}
		stats.CategoryBreakdown[category] = categoryStats
	}

	return stats, nil
}

// GetJobPerformanceTrends calculates performance trends for a job over time
func (r *ScheduledJobCostRepository) GetJobPerformanceTrends(ctx context.Context, jobName, schedule string, lookbackDays int) (*JobPerformanceTrend, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -lookbackDays)

	records, err := r.ListByJob(ctx, jobName, schedule, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	trend := &JobPerformanceTrend{
		JobName:     jobName,
		Schedule:    schedule,
		StartTime:   startTime,
		EndTime:     endTime,
		DataPoints:  make([]JobPerformanceDataPoint, 0),
	}

	if len(records) == 0 {
		return trend, nil
	}

	// Group records by day
	dailyData := make(map[string]*JobPerformanceDataPoint)
	
	for _, record := range records {
		dayKey := record.Timestamp.Format("2006-01-02")
		
		dataPoint, exists := dailyData[dayKey]
		if !exists {
			dataPoint = &JobPerformanceDataPoint{
				Date: record.Timestamp.Truncate(24 * time.Hour),
			}
			dailyData[dayKey] = dataPoint
		}
		
		dataPoint.TotalExecutions++
		dataPoint.TotalCostDollars += record.TotalCostDollars
		dataPoint.TotalDurationMs += record.Duration
		dataPoint.TotalItemsProcessed += record.ItemsProcessed
		
		if record.Success {
			dataPoint.SuccessfulExecutions++
		} else {
			dataPoint.FailedExecutions++
		}
	}

	// Convert map to sorted slice and calculate derived metrics
	for _, dataPoint := range dailyData {
		if dataPoint.TotalExecutions > 0 {
			dataPoint.AverageCostPerExecution = dataPoint.TotalCostDollars / float64(dataPoint.TotalExecutions)
			dataPoint.AverageDurationMs = float64(dataPoint.TotalDurationMs) / float64(dataPoint.TotalExecutions)
			dataPoint.SuccessRate = (float64(dataPoint.SuccessfulExecutions) / float64(dataPoint.TotalExecutions)) * 100
		}
		
		if dataPoint.TotalItemsProcessed > 0 {
			dataPoint.CostPerItemProcessed = dataPoint.TotalCostDollars / float64(dataPoint.TotalItemsProcessed)
		}
		
		trend.DataPoints = append(trend.DataPoints, *dataPoint)
	}

	// Sort by date
	sort.Slice(trend.DataPoints, func(i, j int) bool {
		return trend.DataPoints[i].Date.Before(trend.DataPoints[j].Date)
	})

	// Calculate overall trend statistics
	if len(trend.DataPoints) > 0 {
		firstDay := &trend.DataPoints[0]
		lastDay := &trend.DataPoints[len(trend.DataPoints)-1]
		
		// Calculate trend percentages
		if firstDay.AverageCostPerExecution > 0 {
			trend.CostTrendPercentage = ((lastDay.AverageCostPerExecution - firstDay.AverageCostPerExecution) / firstDay.AverageCostPerExecution) * 100
		}
		
		if firstDay.AverageDurationMs > 0 {
			trend.DurationTrendPercentage = ((lastDay.AverageDurationMs - firstDay.AverageDurationMs) / firstDay.AverageDurationMs) * 100
		}
		
		if firstDay.SuccessRate > 0 {
			trend.SuccessRateTrendPercentage = ((lastDay.SuccessRate - firstDay.SuccessRate) / firstDay.SuccessRate) * 100
		}
	}

	return trend, nil
}

// CreateAggregation creates a scheduled job cost aggregation
func (r *ScheduledJobCostRepository) CreateAggregation(ctx context.Context, aggregation *models.ScheduledJobCostAggregation) error {
	// Call BeforeCreate to set up the model
	if err := aggregation.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the aggregation
	err := r.db.WithContext(ctx).Model(aggregation).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create scheduled job cost aggregation")
	}

	r.logger.Debug("created scheduled job cost aggregation",
		zap.String("job_name", aggregation.JobName),
		zap.String("period", aggregation.Period),
		zap.Time("window_start", aggregation.WindowStart),
		zap.Int64("total_executions", aggregation.TotalExecutions),
		zap.Float64("total_cost_dollars", aggregation.TotalCostDollars))

	return nil
}

// UpdateAggregation updates an existing scheduled job cost aggregation
func (r *ScheduledJobCostRepository) UpdateAggregation(ctx context.Context, aggregation *models.ScheduledJobCostAggregation) error {
	// Call BeforeUpdate to set up the model
	if err := aggregation.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the aggregation
	err := r.db.WithContext(ctx).Model(aggregation).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update scheduled job cost aggregation")
	}

	return nil
}

// GetAggregation retrieves a scheduled job cost aggregation
func (r *ScheduledJobCostRepository) GetAggregation(ctx context.Context, period, jobName string, windowStart time.Time) (*models.ScheduledJobCostAggregation, error) {
	aggregation := &models.ScheduledJobCostAggregation{}

	pk := fmt.Sprintf("SCHEDULED_JOB_AGG#%s#%s", period, jobName)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	err := r.db.WithContext(ctx).Model(aggregation).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(aggregation)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get scheduled job cost aggregation")
	}

	return aggregation, nil
}

// AggregateJobCosts performs aggregation of raw scheduled job cost data
func (r *ScheduledJobCostRepository) AggregateJobCosts(ctx context.Context, jobName, period string, windowStart, windowEnd time.Time) error {
	// Get all job cost records in the window
	records, err := r.ListByJob(ctx, jobName, "all", windowStart, windowEnd, 10000) // Use "all" to get all schedules
	if err != nil {
		return fmt.Errorf("failed to list job costs for aggregation: %w", err)
	}

	if len(records) == 0 {
		return nil // Nothing to aggregate
	}

	// Calculate aggregated values
	aggregation := &models.ScheduledJobCostAggregation{
		JobName:     jobName,
		Period:      period,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		ExecutionTimePercentiles: make(map[string]float64),
		CostPercentiles:         make(map[string]float64),
		JobCategoryBreakdown:    make(map[string]*models.ScheduledJobCategoryStats),
		EnvironmentBreakdown:    make(map[string]*models.ScheduledJobEnvironmentStats),
		ScheduleBreakdown:       make(map[string]*models.ScheduledJobScheduleStats),
	}

	// Collect values for percentile calculation
	var executionTimes []float64
	var costValues []float64
	var totalExecutionTime float64
	
	for _, record := range records {
		aggregation.TotalExecutions++
		
		// Count by status
		switch record.Status {
		case "success":
			aggregation.SuccessfulExecutions++
		case "failed":
			aggregation.FailedExecutions++
		case "timeout":
			aggregation.TimeoutExecutions++
		case "cancelled":
			aggregation.CancelledExecutions++
		}

		// Aggregate resource usage
		aggregation.TotalLambdaInvocations += record.LambdaInvocations
		aggregation.TotalLambdaDurationMs += record.LambdaDurationMs
		aggregation.TotalDynamoDBOperations += record.DynamoDBReadOperations + record.DynamoDBWriteOperations
		aggregation.TotalDynamoDBCapacity += record.DynamoDBReadCapacity + record.DynamoDBWriteCapacity
		aggregation.TotalItemsProcessed += record.ItemsProcessed
		aggregation.TotalItemsErrored += record.ItemsErrored

		// Sum memory usage for average calculation
		aggregation.AverageLambdaMemoryUsedMB += float64(record.LambdaMemoryUsedMB)

		// Aggregate costs
		aggregation.TotalLambdaCostMicroCents += record.LambdaCostMicroCents
		aggregation.TotalDynamoDBCostMicroCents += record.DynamoDBCostMicroCents
		aggregation.TotalSQSCostMicroCents += record.SQSCostMicroCents
		aggregation.TotalS3CostMicroCents += record.S3CostMicroCents
		aggregation.TotalCloudWatchCostMicroCents += record.CloudWatchCostMicroCents
		aggregation.TotalDataTransferCostMicroCents += record.DataTransferCostMicroCents
		aggregation.TotalExternalAPICostMicroCents += record.ExternalAPICostMicroCents
		aggregation.TotalCascadingCostMicroCents += record.CascadingCostMicroCents
		aggregation.TotalCostMicroCents += record.TotalCostMicroCents

		// Collect values for percentiles
		executionTimes = append(executionTimes, float64(record.Duration))
		costValues = append(costValues, record.TotalCostDollars)
		totalExecutionTime += float64(record.Duration)

		// Aggregate by category
		if record.JobCategory != "" {
			categoryStats, exists := aggregation.JobCategoryBreakdown[record.JobCategory]
			if !exists {
				categoryStats = &models.ScheduledJobCategoryStats{
					Category: record.JobCategory,
				}
				aggregation.JobCategoryBreakdown[record.JobCategory] = categoryStats
			}
			
			categoryStats.ExecutionCount++
			categoryStats.TotalCostMicroCents += record.TotalCostMicroCents
			if record.Success {
				categoryStats.SuccessRate += 1.0
			}
		}

		// Aggregate by environment
		if record.Environment != "" {
			envStats, exists := aggregation.EnvironmentBreakdown[record.Environment]
			if !exists {
				envStats = &models.ScheduledJobEnvironmentStats{
					Environment: record.Environment,
				}
				aggregation.EnvironmentBreakdown[record.Environment] = envStats
			}
			
			envStats.ExecutionCount++
			envStats.TotalCostMicroCents += record.TotalCostMicroCents
		}

		// Aggregate by schedule
		scheduleStats, exists := aggregation.ScheduleBreakdown[record.Schedule]
		if !exists {
			scheduleStats = &models.ScheduledJobScheduleStats{
				Schedule: record.Schedule,
			}
			aggregation.ScheduleBreakdown[record.Schedule] = scheduleStats
		}
		
		scheduleStats.ExecutionCount++
		scheduleStats.TotalCostMicroCents += record.TotalCostMicroCents
		scheduleStats.AverageExecutionTime += float64(record.Duration)
	}

	// Calculate averages and rates
	if aggregation.TotalExecutions > 0 {
		aggregation.AverageExecutionTime = totalExecutionTime / float64(aggregation.TotalExecutions)
		aggregation.AverageLambdaMemoryUsedMB = aggregation.AverageLambdaMemoryUsedMB / float64(aggregation.TotalExecutions)
		aggregation.SuccessRate = (float64(aggregation.SuccessfulExecutions) / float64(aggregation.TotalExecutions)) * 100
		aggregation.AverageCostPerExecution = float64(aggregation.TotalCostMicroCents) / 1_000_000.0 / float64(aggregation.TotalExecutions)
	}

	// Calculate cost efficiency metrics
	if aggregation.TotalItemsProcessed > 0 {
		aggregation.CostPerItemProcessed = float64(aggregation.TotalCostMicroCents) / 1_000_000.0 / float64(aggregation.TotalItemsProcessed)
	}
	if aggregation.SuccessfulExecutions > 0 {
		aggregation.CostPerSuccessfulExecution = float64(aggregation.TotalCostMicroCents) / 1_000_000.0 / float64(aggregation.SuccessfulExecutions)
	}

	// Calculate percentiles
	if len(executionTimes) > 0 {
		sort.Float64s(executionTimes)
		aggregation.MedianExecutionTime = getPercentileValue(executionTimes, 50)
		aggregation.P95ExecutionTime = getPercentileValue(executionTimes, 95)
		aggregation.ExecutionTimePercentiles = calculatePercentiles(executionTimes)
	}
	
	if len(costValues) > 0 {
		aggregation.CostPercentiles = calculatePercentiles(costValues)
	}

	// Finalize breakdown statistics
	for category, categoryStats := range aggregation.JobCategoryBreakdown {
		categoryStats.TotalCostDollars = float64(categoryStats.TotalCostMicroCents) / 1_000_000.0
		if categoryStats.ExecutionCount > 0 {
			categoryStats.AverageCostPerExecution = categoryStats.TotalCostDollars / float64(categoryStats.ExecutionCount)
			categoryStats.SuccessRate = (categoryStats.SuccessRate / float64(categoryStats.ExecutionCount)) * 100
		}
		aggregation.JobCategoryBreakdown[category] = categoryStats
	}

	for env, envStats := range aggregation.EnvironmentBreakdown {
		envStats.TotalCostDollars = float64(envStats.TotalCostMicroCents) / 1_000_000.0
		if envStats.ExecutionCount > 0 {
			envStats.AverageCostPerExecution = envStats.TotalCostDollars / float64(envStats.ExecutionCount)
		}
		aggregation.EnvironmentBreakdown[env] = envStats
	}

	for schedule, scheduleStats := range aggregation.ScheduleBreakdown {
		scheduleStats.TotalCostDollars = float64(scheduleStats.TotalCostMicroCents) / 1_000_000.0
		if scheduleStats.ExecutionCount > 0 {
			scheduleStats.AverageCostPerExecution = scheduleStats.TotalCostDollars / float64(scheduleStats.ExecutionCount)
			scheduleStats.AverageExecutionTime = scheduleStats.AverageExecutionTime / float64(scheduleStats.ExecutionCount)
		}
		aggregation.ScheduleBreakdown[schedule] = scheduleStats
	}

	// Check if aggregation already exists
	existing, err := r.GetAggregation(ctx, period, jobName, windowStart)
	if err == nil && existing != nil {
		// Update existing
		aggregation.CreatedAt = existing.CreatedAt
		return r.UpdateAggregation(ctx, aggregation)
	}

	// Create new aggregation
	return r.CreateAggregation(ctx, aggregation)
}

// GetScheduledJobsSummary returns a summary of all scheduled jobs
func (r *ScheduledJobCostRepository) GetScheduledJobsSummary(ctx context.Context, startTime, endTime time.Time) (*ScheduledJobsSummary, error) {
	records, err := r.ListByDateRange(ctx, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	summary := &ScheduledJobsSummary{
		StartTime:       startTime,
		EndTime:         endTime,
		TotalExecutions: len(records),
		JobBreakdown:    make(map[string]*JobSummaryStats),
		CategoryBreakdown: make(map[string]*CategorySummaryStats),
		ScheduleBreakdown: make(map[string]*ScheduleSummaryStats),
	}

	if summary.TotalExecutions == 0 {
		return summary, nil
	}

	for _, record := range records {
		// Overall statistics
		summary.TotalCostMicroCents += record.TotalCostMicroCents
		summary.TotalDurationMs += record.Duration
		summary.TotalItemsProcessed += record.ItemsProcessed

		if record.Success {
			summary.SuccessfulExecutions++
		} else {
			summary.FailedExecutions++
		}

		// Job breakdown
		jobStats, exists := summary.JobBreakdown[record.JobName]
		if !exists {
			jobStats = &JobSummaryStats{
				JobName: record.JobName,
			}
			summary.JobBreakdown[record.JobName] = jobStats
		}
		
		jobStats.ExecutionCount++
		jobStats.TotalCostMicroCents += record.TotalCostMicroCents
		jobStats.TotalDurationMs += record.Duration
		if record.Success {
			jobStats.SuccessfulExecutions++
		}

		// Category breakdown
		if record.JobCategory != "" {
			categoryStats, exists := summary.CategoryBreakdown[record.JobCategory]
			if !exists {
				categoryStats = &CategorySummaryStats{
					Category: record.JobCategory,
				}
				summary.CategoryBreakdown[record.JobCategory] = categoryStats
			}
			
			categoryStats.ExecutionCount++
			categoryStats.TotalCostMicroCents += record.TotalCostMicroCents
			if record.Success {
				categoryStats.SuccessfulExecutions++
			}
		}

		// Schedule breakdown
		scheduleStats, exists := summary.ScheduleBreakdown[record.Schedule]
		if !exists {
			scheduleStats = &ScheduleSummaryStats{
				Schedule: record.Schedule,
			}
			summary.ScheduleBreakdown[record.Schedule] = scheduleStats
		}
		
		scheduleStats.ExecutionCount++
		scheduleStats.TotalCostMicroCents += record.TotalCostMicroCents
		scheduleStats.TotalDurationMs += record.Duration
		if record.Success {
			scheduleStats.SuccessfulExecutions++
		}
	}

	// Calculate overall metrics
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1_000_000.0
	summary.AverageCostPerExecution = summary.TotalCostDollars / float64(summary.TotalExecutions)
	summary.AverageDurationMs = float64(summary.TotalDurationMs) / float64(summary.TotalExecutions)
	summary.SuccessRate = (float64(summary.SuccessfulExecutions) / float64(summary.TotalExecutions)) * 100

	if summary.TotalItemsProcessed > 0 {
		summary.CostPerItemProcessed = summary.TotalCostDollars / float64(summary.TotalItemsProcessed)
	}

	// Calculate job breakdown metrics
	for jobName, jobStats := range summary.JobBreakdown {
		jobStats.TotalCostDollars = float64(jobStats.TotalCostMicroCents) / 1_000_000.0
		if jobStats.ExecutionCount > 0 {
			jobStats.AverageCostPerExecution = jobStats.TotalCostDollars / float64(jobStats.ExecutionCount)
			jobStats.AverageDurationMs = float64(jobStats.TotalDurationMs) / float64(jobStats.ExecutionCount)
			jobStats.SuccessRate = (float64(jobStats.SuccessfulExecutions) / float64(jobStats.ExecutionCount)) * 100
		}
		summary.JobBreakdown[jobName] = jobStats
	}

	// Calculate category breakdown metrics
	for category, categoryStats := range summary.CategoryBreakdown {
		categoryStats.TotalCostDollars = float64(categoryStats.TotalCostMicroCents) / 1_000_000.0
		if categoryStats.ExecutionCount > 0 {
			categoryStats.AverageCostPerExecution = categoryStats.TotalCostDollars / float64(categoryStats.ExecutionCount)
			categoryStats.SuccessRate = (float64(categoryStats.SuccessfulExecutions) / float64(categoryStats.ExecutionCount)) * 100
		}
		summary.CategoryBreakdown[category] = categoryStats
	}

	// Calculate schedule breakdown metrics
	for schedule, scheduleStats := range summary.ScheduleBreakdown {
		scheduleStats.TotalCostDollars = float64(scheduleStats.TotalCostMicroCents) / 1_000_000.0
		if scheduleStats.ExecutionCount > 0 {
			scheduleStats.AverageCostPerExecution = scheduleStats.TotalCostDollars / float64(scheduleStats.ExecutionCount)
			scheduleStats.AverageDurationMs = float64(scheduleStats.TotalDurationMs) / float64(scheduleStats.ExecutionCount)
			scheduleStats.SuccessRate = (float64(scheduleStats.SuccessfulExecutions) / float64(scheduleStats.ExecutionCount)) * 100
		}
		summary.ScheduleBreakdown[schedule] = scheduleStats
	}

	return summary, nil
}

// Supporting types for repository methods

// JobExecutionStats represents execution statistics for a job
type JobExecutionStats struct {
	JobName       string
	Schedule      string
	StartTime     time.Time
	EndTime       time.Time
	TotalRuns     int
	SuccessfulRuns int64
	FailedRuns    int64
	SuccessRate   float64

	AverageDurationMs float64
	AverageCostDollars float64
	TotalCostDollars  float64
	TotalCostMicroCents int64

	TotalItemsProcessed       int64
	TotalItemsErrored         int64
	TotalLambdaInvocations    int64
	TotalLambdaDurationMs     int64
	CostPerItemProcessed      float64
	CostPerSuccessfulExecution float64

	StatusBreakdown      map[string]int64
	CategoryBreakdown    map[string]*JobCategoryStats
	DurationPercentiles  map[string]float64
	CostPercentiles      map[string]float64
}

// JobCategoryStats represents statistics for a job category
type JobCategoryStats struct {
	Category               string
	ExecutionCount         int64
	SuccessfulExecutions   int64
	TotalCostMicroCents    int64
	TotalCostDollars       float64
	TotalDurationMs        int64
	AverageCostPerExecution float64
	AverageDurationMs      float64
	SuccessRate            float64
}

// JobPerformanceTrend represents performance trends for a job over time
type JobPerformanceTrend struct {
	JobName                     string
	Schedule                    string
	StartTime                   time.Time
	EndTime                     time.Time
	DataPoints                  []JobPerformanceDataPoint
	CostTrendPercentage         float64
	DurationTrendPercentage     float64
	SuccessRateTrendPercentage  float64
}

// JobPerformanceDataPoint represents a single data point in the performance trend
type JobPerformanceDataPoint struct {
	Date                   time.Time
	TotalExecutions        int64
	SuccessfulExecutions   int64
	FailedExecutions       int64
	TotalCostDollars       float64
	TotalDurationMs        int64
	TotalItemsProcessed    int64
	AverageCostPerExecution float64
	AverageDurationMs      float64
	CostPerItemProcessed   float64
	SuccessRate            float64
}

// ScheduledJobsSummary represents a summary of all scheduled jobs
type ScheduledJobsSummary struct {
	StartTime            time.Time
	EndTime              time.Time
	TotalExecutions      int
	SuccessfulExecutions int64
	FailedExecutions     int64
	SuccessRate          float64

	TotalCostMicroCents     int64
	TotalCostDollars        float64
	TotalDurationMs         int64
	TotalItemsProcessed     int64
	AverageCostPerExecution float64
	AverageDurationMs       float64
	CostPerItemProcessed    float64

	JobBreakdown      map[string]*JobSummaryStats
	CategoryBreakdown map[string]*CategorySummaryStats
	ScheduleBreakdown map[string]*ScheduleSummaryStats
}

// JobSummaryStats represents summary statistics for a job
type JobSummaryStats struct {
	JobName                string
	ExecutionCount         int64
	SuccessfulExecutions   int64
	TotalCostMicroCents    int64
	TotalCostDollars       float64
	TotalDurationMs        int64
	AverageCostPerExecution float64
	AverageDurationMs      float64
	SuccessRate            float64
}

// CategorySummaryStats represents summary statistics for a category
type CategorySummaryStats struct {
	Category               string
	ExecutionCount         int64
	SuccessfulExecutions   int64
	TotalCostMicroCents    int64
	TotalCostDollars       float64
	AverageCostPerExecution float64
	SuccessRate            float64
}

// ScheduleSummaryStats represents summary statistics for a schedule
type ScheduleSummaryStats struct {
	Schedule               string
	ExecutionCount         int64
	SuccessfulExecutions   int64
	TotalCostMicroCents    int64
	TotalCostDollars       float64
	TotalDurationMs        int64
	AverageCostPerExecution float64
	AverageDurationMs      float64
	SuccessRate            float64
}

// Note: calculatePercentiles and getPercentileValue functions are shared utilities
// imported from the same package. They are defined in cost_tracking_repository.go