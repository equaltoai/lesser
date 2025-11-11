package repositories

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// ScheduledJobCostRepository handles scheduled job cost tracking persistence using enhanced patterns
type ScheduledJobCostRepository struct {
	*EnhancedBaseRepository[*models.ScheduledJobCostRecord]
	logger *zap.Logger
}

// NewScheduledJobCostRepository creates a new scheduled job cost repository with enhanced functionality
func NewScheduledJobCostRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *ScheduledJobCostRepository {
	// Create enhanced repository optimized for scheduled job cost operations
	enhancedRepo := NewEnhancedBaseRepository[*models.ScheduledJobCostRecord](db, tableName, logger, costService, "ScheduledJobCostRepository", "scheduledjobcost")

	// Set up enhanced services for scheduled job cost operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cost data cached for analytics
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for job cost monitoring

	return &ScheduledJobCostRepository{
		EnhancedBaseRepository: enhancedRepo,
		logger:                 logger,
	}
}

// Create creates a new scheduled job cost record
func (r *ScheduledJobCostRepository) Create(ctx context.Context, record *models.ScheduledJobCostRecord) error {
	// Call BeforeCreate to set up the model
	if err := record.BeforeCreate(); err != nil {
		return fmt.Errorf("%w: %w", ErrScheduledJobCostBeforeCreateFailed, err)
	}

	// Create the scheduled job cost record using enhanced repository
	err := r.ValidateAndCreate(ctx, record)
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
		return fmt.Errorf("%w: %w", ErrScheduledJobCostBeforeUpdateFailed, err)
	}

	// Update the scheduled job cost record using BaseRepository
	err := r.BaseRepository.Update(ctx, record)
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

	err := r.BaseRepository.Get(ctx, pk, sk, record)
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

		err := r.BaseRepository.GetDB().WithContext(ctx).Model(&models.ScheduledJobCostRecord{}).
			Index("job-status-index").
			Where("gsi1PK", "=", fmt.Sprintf("SCHEDULED_JOB_STATUS#%s", status)).
			Where("gsi1SK", "contains", fmt.Sprintf("#%s", id)).
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

	return nil, fmt.Errorf("%w: %s", ErrScheduledJobCostNotFound, id)
}

// ListByJob lists scheduled job cost records for a specific job within a time range
func (r *ScheduledJobCostRepository) ListByJob(ctx context.Context, jobName, schedule string, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	var records []*models.ScheduledJobCostRecord

	// Construct SK range for time-based query
	pk := fmt.Sprintf("SCHEDULED_JOB_COST#%s#%s", jobName, schedule)
	startSK := fmt.Sprintf("RUN#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("RUN#%s", endTime.Format("20060102150405"))

	query := r.BaseRepository.GetDB().WithContext(ctx).Model(&models.ScheduledJobCostRecord{}).
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

	query := r.BaseRepository.GetDB().WithContext(ctx).Model(&models.ScheduledJobCostRecord{}).
		Index("job-status-index").
		Where("gsi1PK", "=", fmt.Sprintf("SCHEDULED_JOB_STATUS#%s", status)).
		Where("gsi1SK", ">=", startSK).
		Where("gsi1SK", "<=", endSK).
		OrderBy("gsi1SK", "DESC").
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
		query := r.BaseRepository.GetDB().WithContext(ctx).Model(&models.ScheduledJobCostRecord{}).
			Index("job-date-index").
			Where("gsi2PK", "=", fmt.Sprintf("SCHEDULED_JOB_DATE#%s", dateStr)).
			OrderBy("gsi2SK", "DESC").
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

	if err := common.ValidateSliceLength("allRecords", allRecords, limit); err == nil {
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
		JobName:           jobName,
		Schedule:          schedule,
		StartTime:         startTime,
		EndTime:           endTime,
		TotalRuns:         len(records),
		StatusBreakdown:   make(map[string]int64),
		CategoryBreakdown: make(map[string]*JobCategoryStats),
	}

	if stats.TotalRuns == 0 {
		return stats, nil
	}

	// Collect values for percentile calculations
	durations := make([]float64, 0, len(records))
	costs := make([]float64, 0, len(records))
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
		JobName:    jobName,
		Schedule:   schedule,
		StartTime:  startTime,
		EndTime:    endTime,
		DataPoints: make([]JobPerformanceDataPoint, 0),
	}

	if err := common.ValidateSliceNotEmpty("records", records); err != nil {
		return trend, nil
	}

	// Group records by day
	dailyData := make(map[string]*JobPerformanceDataPoint)

	for _, record := range records {
		dayKey := record.Timestamp.Format(common.DateFormat)

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
	if err := common.ValidateSliceNotEmpty("trend.DataPoints", trend.DataPoints); err == nil {
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
		return fmt.Errorf("%w: %w", ErrScheduledJobCostBeforeCreateFailed, err)
	}

	// Create the aggregation
	err := r.BaseRepository.GetDB().WithContext(ctx).Model(aggregation).Create()
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
		return fmt.Errorf("%w: %w", ErrScheduledJobCostBeforeUpdateFailed, err)
	}

	// Update the aggregation
	err := r.BaseRepository.GetDB().WithContext(ctx).Model(aggregation).Update()
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

	err := r.BaseRepository.GetDB().WithContext(ctx).Model(aggregation).
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
	records, err := r.ListByJob(ctx, jobName, "all", windowStart, windowEnd, 10000)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrScheduledJobCostAggregationFailed, err)
	}

	if err := common.ValidateSliceNotEmpty("records", records); err != nil {
		return nil // Nothing to aggregate
	}

	// Initialize aggregation
	aggregation := r.initializeAggregation(jobName, period, windowStart, windowEnd)

	// Process records and collect metrics
	metrics := r.processJobRecords(records, aggregation)

	// Calculate statistics
	r.calculateAggregationStatistics(aggregation, metrics)

	// Calculate percentiles
	r.calculateAggregationPercentiles(aggregation, metrics)

	// Finalize breakdowns
	r.finalizeBreakdownStatistics(aggregation)

	// Save aggregation
	return r.saveAggregation(ctx, aggregation, period, jobName, windowStart)
}

// jobMetrics holds metrics collected during processing
type jobMetrics struct {
	executionTimes     []float64
	costValues         []float64
	totalExecutionTime float64
}

// initializeAggregation creates a new aggregation structure
func (r *ScheduledJobCostRepository) initializeAggregation(jobName, period string, windowStart, windowEnd time.Time) *models.ScheduledJobCostAggregation {
	return &models.ScheduledJobCostAggregation{
		JobName:                  jobName,
		Period:                   period,
		WindowStart:              windowStart,
		WindowEnd:                windowEnd,
		ExecutionTimePercentiles: make(map[string]float64),
		CostPercentiles:          make(map[string]float64),
		JobCategoryBreakdown:     make(map[string]*models.ScheduledJobCategoryStats),
		EnvironmentBreakdown:     make(map[string]*models.ScheduledJobEnvironmentStats),
		ScheduleBreakdown:        make(map[string]*models.ScheduledJobScheduleStats),
	}
}

// processJobRecords processes all records and updates aggregation
func (r *ScheduledJobCostRepository) processJobRecords(records []*models.ScheduledJobCostRecord, aggregation *models.ScheduledJobCostAggregation) *jobMetrics {
	metrics := &jobMetrics{
		executionTimes: make([]float64, 0, len(records)),
		costValues:     make([]float64, 0, len(records)),
	}

	for _, record := range records {
		r.processJobRecord(record, aggregation, metrics)
	}

	return metrics
}

// processJobRecord processes a single job record
func (r *ScheduledJobCostRepository) processJobRecord(record *models.ScheduledJobCostRecord, aggregation *models.ScheduledJobCostAggregation, metrics *jobMetrics) {
	aggregation.TotalExecutions++

	// Update status counts
	r.updateStatusCounts(record, aggregation)

	// Aggregate resource usage
	r.aggregateResourceUsage(record, aggregation)

	// Aggregate costs
	r.aggregateCosts(record, aggregation)

	// Collect metrics
	metrics.executionTimes = append(metrics.executionTimes, float64(record.Duration))
	metrics.costValues = append(metrics.costValues, record.TotalCostDollars)
	metrics.totalExecutionTime += float64(record.Duration)

	// Update breakdowns
	r.updateCategoryBreakdown(record, aggregation)
	r.updateEnvironmentBreakdown(record, aggregation)
	r.updateScheduleBreakdown(record, aggregation)
}

// updateStatusCounts updates execution status counts
func (r *ScheduledJobCostRepository) updateStatusCounts(record *models.ScheduledJobCostRecord, aggregation *models.ScheduledJobCostAggregation) {
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
}

// aggregateResourceUsage aggregates resource usage metrics
func (r *ScheduledJobCostRepository) aggregateResourceUsage(record *models.ScheduledJobCostRecord, aggregation *models.ScheduledJobCostAggregation) {
	aggregation.TotalLambdaInvocations += record.LambdaInvocations
	aggregation.TotalLambdaDurationMs += record.LambdaDurationMs
	aggregation.TotalDynamoDBOperations += record.DynamoDBReadOperations + record.DynamoDBWriteOperations
	aggregation.TotalDynamoDBCapacity += record.DynamoDBReadCapacity + record.DynamoDBWriteCapacity
	aggregation.TotalItemsProcessed += record.ItemsProcessed
	aggregation.TotalItemsErrored += record.ItemsErrored
	aggregation.AverageLambdaMemoryUsedMB += float64(record.LambdaMemoryUsedMB)
}

// aggregateCosts aggregates cost metrics
func (r *ScheduledJobCostRepository) aggregateCosts(record *models.ScheduledJobCostRecord, aggregation *models.ScheduledJobCostAggregation) {
	aggregation.TotalLambdaCostMicroCents += record.LambdaCostMicroCents
	aggregation.TotalDynamoDBCostMicroCents += record.DynamoDBCostMicroCents
	aggregation.TotalSQSCostMicroCents += record.SQSCostMicroCents
	aggregation.TotalS3CostMicroCents += record.S3CostMicroCents
	aggregation.TotalCloudWatchCostMicroCents += record.CloudWatchCostMicroCents
	aggregation.TotalDataTransferCostMicroCents += record.DataTransferCostMicroCents
	aggregation.TotalExternalAPICostMicroCents += record.ExternalAPICostMicroCents
	aggregation.TotalCascadingCostMicroCents += record.CascadingCostMicroCents
	aggregation.TotalCostMicroCents += record.TotalCostMicroCents
}

// updateCategoryBreakdown updates category breakdown statistics
func (r *ScheduledJobCostRepository) updateCategoryBreakdown(record *models.ScheduledJobCostRecord, aggregation *models.ScheduledJobCostAggregation) {
	if err := common.ValidateRequiredParam("JobCategory", record.JobCategory); err != nil {
		return
	}

	categoryStats := r.getOrCreateCategoryStats(record.JobCategory, aggregation)
	categoryStats.ExecutionCount++
	categoryStats.TotalCostMicroCents += record.TotalCostMicroCents
	if record.Success {
		categoryStats.SuccessRate += 1.0
	}
}

// getOrCreateCategoryStats gets or creates category statistics
func (r *ScheduledJobCostRepository) getOrCreateCategoryStats(category string, aggregation *models.ScheduledJobCostAggregation) *models.ScheduledJobCategoryStats {
	categoryStats, exists := aggregation.JobCategoryBreakdown[category]
	if !exists {
		categoryStats = &models.ScheduledJobCategoryStats{
			Category: category,
		}
		aggregation.JobCategoryBreakdown[category] = categoryStats
	}
	return categoryStats
}

// updateEnvironmentBreakdown updates environment breakdown statistics
func (r *ScheduledJobCostRepository) updateEnvironmentBreakdown(record *models.ScheduledJobCostRecord, aggregation *models.ScheduledJobCostAggregation) {
	if err := common.ValidateRequiredParam("Environment", record.Environment); err != nil {
		return
	}

	envStats := r.getOrCreateEnvironmentStats(record.Environment, aggregation)
	envStats.ExecutionCount++
	envStats.TotalCostMicroCents += record.TotalCostMicroCents
}

// getOrCreateEnvironmentStats gets or creates environment statistics
func (r *ScheduledJobCostRepository) getOrCreateEnvironmentStats(environment string, aggregation *models.ScheduledJobCostAggregation) *models.ScheduledJobEnvironmentStats {
	envStats, exists := aggregation.EnvironmentBreakdown[environment]
	if !exists {
		envStats = &models.ScheduledJobEnvironmentStats{
			Environment: environment,
		}
		aggregation.EnvironmentBreakdown[environment] = envStats
	}
	return envStats
}

// updateScheduleBreakdown updates schedule breakdown statistics
func (r *ScheduledJobCostRepository) updateScheduleBreakdown(record *models.ScheduledJobCostRecord, aggregation *models.ScheduledJobCostAggregation) {
	scheduleStats := r.getOrCreateScheduleStats(record.Schedule, aggregation)
	scheduleStats.ExecutionCount++
	scheduleStats.TotalCostMicroCents += record.TotalCostMicroCents
	scheduleStats.AverageExecutionTime += float64(record.Duration)
}

// getOrCreateScheduleStats gets or creates schedule statistics
func (r *ScheduledJobCostRepository) getOrCreateScheduleStats(schedule string, aggregation *models.ScheduledJobCostAggregation) *models.ScheduledJobScheduleStats {
	scheduleStats, exists := aggregation.ScheduleBreakdown[schedule]
	if !exists {
		scheduleStats = &models.ScheduledJobScheduleStats{
			Schedule: schedule,
		}
		aggregation.ScheduleBreakdown[schedule] = scheduleStats
	}
	return scheduleStats
}

// calculateAggregationStatistics calculates averages and rates
func (r *ScheduledJobCostRepository) calculateAggregationStatistics(aggregation *models.ScheduledJobCostAggregation, metrics *jobMetrics) {
	if aggregation.TotalExecutions > 0 {
		aggregation.AverageExecutionTime = metrics.totalExecutionTime / float64(aggregation.TotalExecutions)
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
}

// calculateAggregationPercentiles calculates execution time and cost percentiles
func (r *ScheduledJobCostRepository) calculateAggregationPercentiles(aggregation *models.ScheduledJobCostAggregation, metrics *jobMetrics) {
	if err := common.ValidateSliceNotEmpty("metrics.executionTimes", metrics.executionTimes); err == nil {
		sort.Float64s(metrics.executionTimes)
		aggregation.MedianExecutionTime = getPercentileValue(metrics.executionTimes, 50)
		aggregation.P95ExecutionTime = getPercentileValue(metrics.executionTimes, 95)
		aggregation.ExecutionTimePercentiles = calculatePercentiles(metrics.executionTimes)
	}

	if err := common.ValidateSliceNotEmpty("metrics.costValues", metrics.costValues); err == nil {
		aggregation.CostPercentiles = calculatePercentiles(metrics.costValues)
	}
}

// finalizeBreakdownStatistics finalizes all breakdown statistics
func (r *ScheduledJobCostRepository) finalizeBreakdownStatistics(aggregation *models.ScheduledJobCostAggregation) {
	r.finalizeCategoryBreakdown(aggregation)
	r.finalizeEnvironmentBreakdown(aggregation)
	r.finalizeScheduleBreakdown(aggregation)
}

// finalizeCategoryBreakdown finalizes category breakdown statistics
func (r *ScheduledJobCostRepository) finalizeCategoryBreakdown(aggregation *models.ScheduledJobCostAggregation) {
	for category, categoryStats := range aggregation.JobCategoryBreakdown {
		categoryStats.TotalCostDollars = float64(categoryStats.TotalCostMicroCents) / 1_000_000.0
		if categoryStats.ExecutionCount > 0 {
			categoryStats.AverageCostPerExecution = categoryStats.TotalCostDollars / float64(categoryStats.ExecutionCount)
			categoryStats.SuccessRate = (categoryStats.SuccessRate / float64(categoryStats.ExecutionCount)) * 100
		}
		aggregation.JobCategoryBreakdown[category] = categoryStats
	}
}

// finalizeEnvironmentBreakdown finalizes environment breakdown statistics
func (r *ScheduledJobCostRepository) finalizeEnvironmentBreakdown(aggregation *models.ScheduledJobCostAggregation) {
	for env, envStats := range aggregation.EnvironmentBreakdown {
		envStats.TotalCostDollars = float64(envStats.TotalCostMicroCents) / 1_000_000.0
		if envStats.ExecutionCount > 0 {
			envStats.AverageCostPerExecution = envStats.TotalCostDollars / float64(envStats.ExecutionCount)
		}
		aggregation.EnvironmentBreakdown[env] = envStats
	}
}

// finalizeScheduleBreakdown finalizes schedule breakdown statistics
func (r *ScheduledJobCostRepository) finalizeScheduleBreakdown(aggregation *models.ScheduledJobCostAggregation) {
	for schedule, scheduleStats := range aggregation.ScheduleBreakdown {
		scheduleStats.TotalCostDollars = float64(scheduleStats.TotalCostMicroCents) / 1_000_000.0
		if scheduleStats.ExecutionCount > 0 {
			scheduleStats.AverageCostPerExecution = scheduleStats.TotalCostDollars / float64(scheduleStats.ExecutionCount)
			scheduleStats.AverageExecutionTime = scheduleStats.AverageExecutionTime / float64(scheduleStats.ExecutionCount)
		}
		aggregation.ScheduleBreakdown[schedule] = scheduleStats
	}
}

// saveAggregation saves or updates the aggregation
func (r *ScheduledJobCostRepository) saveAggregation(ctx context.Context, aggregation *models.ScheduledJobCostAggregation, period, jobName string, windowStart time.Time) error {
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

	summary := r.initializeJobsSummary(startTime, endTime, records)
	if summary.TotalExecutions == 0 {
		return summary, nil
	}

	// Process all records
	r.processSummaryRecords(summary, records)

	// Calculate all metrics
	r.calculateOverallMetrics(summary)
	r.calculateJobMetrics(summary)
	r.calculateCategoryMetrics(summary)
	r.calculateScheduleMetrics(summary)

	return summary, nil
}

// initializeJobsSummary creates and initializes a new jobs summary
func (r *ScheduledJobCostRepository) initializeJobsSummary(startTime, endTime time.Time, records []*models.ScheduledJobCostRecord) *ScheduledJobsSummary {
	return &ScheduledJobsSummary{
		StartTime:         startTime,
		EndTime:           endTime,
		TotalExecutions:   len(records),
		JobBreakdown:      make(map[string]*JobSummaryStats),
		CategoryBreakdown: make(map[string]*CategorySummaryStats),
		ScheduleBreakdown: make(map[string]*ScheduleSummaryStats),
	}
}

// processSummaryRecords processes all job records and updates the summary
func (r *ScheduledJobCostRepository) processSummaryRecords(summary *ScheduledJobsSummary, records []*models.ScheduledJobCostRecord) {
	for _, record := range records {
		r.updateOverallStats(summary, record)
		r.updateJobSummaryBreakdown(summary, record)
		r.updateCategorySummaryBreakdown(summary, record)
		r.updateScheduleSummaryBreakdown(summary, record)
	}
}

// updateOverallStats updates overall statistics for the summary
func (r *ScheduledJobCostRepository) updateOverallStats(summary *ScheduledJobsSummary, record *models.ScheduledJobCostRecord) {
	summary.TotalCostMicroCents += record.TotalCostMicroCents
	summary.TotalDurationMs += record.Duration
	summary.TotalItemsProcessed += record.ItemsProcessed

	if record.Success {
		summary.SuccessfulExecutions++
	} else {
		summary.FailedExecutions++
	}
}

// updateJobSummaryBreakdown updates job-specific breakdown statistics
func (r *ScheduledJobCostRepository) updateJobSummaryBreakdown(summary *ScheduledJobsSummary, record *models.ScheduledJobCostRecord) {
	jobStats := r.getOrCreateJobStats(summary, record.JobName)
	jobStats.ExecutionCount++
	jobStats.TotalCostMicroCents += record.TotalCostMicroCents
	jobStats.TotalDurationMs += record.Duration
	if record.Success {
		jobStats.SuccessfulExecutions++
	}
}

// getOrCreateJobStats gets or creates job statistics
func (r *ScheduledJobCostRepository) getOrCreateJobStats(summary *ScheduledJobsSummary, jobName string) *JobSummaryStats {
	jobStats, exists := summary.JobBreakdown[jobName]
	if !exists {
		jobStats = &JobSummaryStats{
			JobName: jobName,
		}
		summary.JobBreakdown[jobName] = jobStats
	}
	return jobStats
}

// updateCategorySummaryBreakdown updates category-specific breakdown statistics
func (r *ScheduledJobCostRepository) updateCategorySummaryBreakdown(summary *ScheduledJobsSummary, record *models.ScheduledJobCostRecord) {
	if err := common.ValidateRequiredParam("JobCategory", record.JobCategory); err != nil {
		return
	}

	categoryStats := r.getOrCreateCategorySummaryStats(summary, record.JobCategory)
	categoryStats.ExecutionCount++
	categoryStats.TotalCostMicroCents += record.TotalCostMicroCents
	if record.Success {
		categoryStats.SuccessfulExecutions++
	}
}

// getOrCreateCategorySummaryStats gets or creates category statistics
func (r *ScheduledJobCostRepository) getOrCreateCategorySummaryStats(summary *ScheduledJobsSummary, category string) *CategorySummaryStats {
	categoryStats, exists := summary.CategoryBreakdown[category]
	if !exists {
		categoryStats = &CategorySummaryStats{
			Category: category,
		}
		summary.CategoryBreakdown[category] = categoryStats
	}
	return categoryStats
}

// updateScheduleSummaryBreakdown updates schedule-specific breakdown statistics
func (r *ScheduledJobCostRepository) updateScheduleSummaryBreakdown(summary *ScheduledJobsSummary, record *models.ScheduledJobCostRecord) {
	scheduleStats := r.getOrCreateScheduleSummaryStats(summary, record.Schedule)
	scheduleStats.ExecutionCount++
	scheduleStats.TotalCostMicroCents += record.TotalCostMicroCents
	scheduleStats.TotalDurationMs += record.Duration
	if record.Success {
		scheduleStats.SuccessfulExecutions++
	}
}

// getOrCreateScheduleSummaryStats gets or creates schedule statistics
func (r *ScheduledJobCostRepository) getOrCreateScheduleSummaryStats(summary *ScheduledJobsSummary, schedule string) *ScheduleSummaryStats {
	scheduleStats, exists := summary.ScheduleBreakdown[schedule]
	if !exists {
		scheduleStats = &ScheduleSummaryStats{
			Schedule: schedule,
		}
		summary.ScheduleBreakdown[schedule] = scheduleStats
	}
	return scheduleStats
}

// calculateOverallMetrics calculates overall summary metrics
func (r *ScheduledJobCostRepository) calculateOverallMetrics(summary *ScheduledJobsSummary) {
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1_000_000.0
	summary.AverageCostPerExecution = summary.TotalCostDollars / float64(summary.TotalExecutions)
	summary.AverageDurationMs = float64(summary.TotalDurationMs) / float64(summary.TotalExecutions)
	summary.SuccessRate = (float64(summary.SuccessfulExecutions) / float64(summary.TotalExecutions)) * 100

	if summary.TotalItemsProcessed > 0 {
		summary.CostPerItemProcessed = summary.TotalCostDollars / float64(summary.TotalItemsProcessed)
	}
}

// calculateJobMetrics calculates metrics for each job
func (r *ScheduledJobCostRepository) calculateJobMetrics(summary *ScheduledJobsSummary) {
	for _, jobStats := range summary.JobBreakdown {
		r.calculateJobStatsMetrics(jobStats)
	}
}

// calculateJobStatsMetrics calculates metrics for a single job's statistics
func (r *ScheduledJobCostRepository) calculateJobStatsMetrics(jobStats *JobSummaryStats) {
	jobStats.TotalCostDollars = float64(jobStats.TotalCostMicroCents) / 1_000_000.0
	if jobStats.ExecutionCount > 0 {
		jobStats.AverageCostPerExecution = jobStats.TotalCostDollars / float64(jobStats.ExecutionCount)
		jobStats.AverageDurationMs = float64(jobStats.TotalDurationMs) / float64(jobStats.ExecutionCount)
		jobStats.SuccessRate = (float64(jobStats.SuccessfulExecutions) / float64(jobStats.ExecutionCount)) * 100
	}
}

// calculateCategoryMetrics calculates metrics for each category
func (r *ScheduledJobCostRepository) calculateCategoryMetrics(summary *ScheduledJobsSummary) {
	for _, categoryStats := range summary.CategoryBreakdown {
		r.calculateCategoryStatsMetrics(categoryStats)
	}
}

// calculateCategoryStatsMetrics calculates metrics for a single category's statistics
func (r *ScheduledJobCostRepository) calculateCategoryStatsMetrics(categoryStats *CategorySummaryStats) {
	categoryStats.TotalCostDollars = float64(categoryStats.TotalCostMicroCents) / 1_000_000.0
	if categoryStats.ExecutionCount > 0 {
		categoryStats.AverageCostPerExecution = categoryStats.TotalCostDollars / float64(categoryStats.ExecutionCount)
		categoryStats.SuccessRate = (float64(categoryStats.SuccessfulExecutions) / float64(categoryStats.ExecutionCount)) * 100
	}
}

// calculateScheduleMetrics calculates metrics for each schedule
func (r *ScheduledJobCostRepository) calculateScheduleMetrics(summary *ScheduledJobsSummary) {
	for _, scheduleStats := range summary.ScheduleBreakdown {
		r.calculateScheduleStatsMetrics(scheduleStats)
	}
}

// calculateScheduleStatsMetrics calculates metrics for a single schedule's statistics
func (r *ScheduledJobCostRepository) calculateScheduleStatsMetrics(scheduleStats *ScheduleSummaryStats) {
	scheduleStats.TotalCostDollars = float64(scheduleStats.TotalCostMicroCents) / 1_000_000.0
	if scheduleStats.ExecutionCount > 0 {
		scheduleStats.AverageCostPerExecution = scheduleStats.TotalCostDollars / float64(scheduleStats.ExecutionCount)
		scheduleStats.AverageDurationMs = float64(scheduleStats.TotalDurationMs) / float64(scheduleStats.ExecutionCount)
		scheduleStats.SuccessRate = (float64(scheduleStats.SuccessfulExecutions) / float64(scheduleStats.ExecutionCount)) * 100
	}
}

// Supporting types for repository methods

// JobExecutionStats represents execution statistics for a job
type JobExecutionStats struct {
	JobName        string
	Schedule       string
	StartTime      time.Time
	EndTime        time.Time
	TotalRuns      int
	SuccessfulRuns int64
	FailedRuns     int64
	SuccessRate    float64

	AverageDurationMs   float64
	AverageCostDollars  float64
	TotalCostDollars    float64
	TotalCostMicroCents int64

	TotalItemsProcessed        int64
	TotalItemsErrored          int64
	TotalLambdaInvocations     int64
	TotalLambdaDurationMs      int64
	CostPerItemProcessed       float64
	CostPerSuccessfulExecution float64

	StatusBreakdown     map[string]int64
	CategoryBreakdown   map[string]*JobCategoryStats
	DurationPercentiles map[string]float64
	CostPercentiles     map[string]float64
}

// JobCategoryStats represents statistics for a job category
type JobCategoryStats struct {
	Category                string
	ExecutionCount          int64
	SuccessfulExecutions    int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	TotalDurationMs         int64
	AverageCostPerExecution float64
	AverageDurationMs       float64
	SuccessRate             float64
}

// JobPerformanceTrend represents performance trends for a job over time
type JobPerformanceTrend struct {
	JobName                    string
	Schedule                   string
	StartTime                  time.Time
	EndTime                    time.Time
	DataPoints                 []JobPerformanceDataPoint
	CostTrendPercentage        float64
	DurationTrendPercentage    float64
	SuccessRateTrendPercentage float64
}

// JobPerformanceDataPoint represents a single data point in the performance trend
type JobPerformanceDataPoint struct {
	Date                    time.Time
	TotalExecutions         int64
	SuccessfulExecutions    int64
	FailedExecutions        int64
	TotalCostDollars        float64
	TotalDurationMs         int64
	TotalItemsProcessed     int64
	AverageCostPerExecution float64
	AverageDurationMs       float64
	CostPerItemProcessed    float64
	SuccessRate             float64
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
	JobName                 string
	ExecutionCount          int64
	SuccessfulExecutions    int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	TotalDurationMs         int64
	AverageCostPerExecution float64
	AverageDurationMs       float64
	SuccessRate             float64
}

// CategorySummaryStats represents summary statistics for a category
type CategorySummaryStats struct {
	Category                string
	ExecutionCount          int64
	SuccessfulExecutions    int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostPerExecution float64
	SuccessRate             float64
}

// ScheduleSummaryStats represents summary statistics for a schedule
type ScheduleSummaryStats struct {
	Schedule                string
	ExecutionCount          int64
	SuccessfulExecutions    int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	TotalDurationMs         int64
	AverageCostPerExecution float64
	AverageDurationMs       float64
	SuccessRate             float64
}

// Note: calculatePercentiles and getPercentileValue functions are shared utilities
// imported from the same package. They are defined in cost_tracking_repository.go
