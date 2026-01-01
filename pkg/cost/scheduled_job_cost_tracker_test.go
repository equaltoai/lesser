package cost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockScheduledJobCostRepository is a mock implementation of ScheduledJobCostRepository
type MockScheduledJobCostRepository struct {
	mock.Mock
}

func (m *MockScheduledJobCostRepository) Create(ctx context.Context, record *models.ScheduledJobCostRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

func (m *MockScheduledJobCostRepository) Update(ctx context.Context, record *models.ScheduledJobCostRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

func (m *MockScheduledJobCostRepository) GetByID(ctx context.Context, id string) (*models.ScheduledJobCostRecord, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ScheduledJobCostRecord), args.Error(1)
}

func (m *MockScheduledJobCostRepository) ListByJob(ctx context.Context, jobName, schedule string, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	args := m.Called(ctx, jobName, schedule, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ScheduledJobCostRecord), args.Error(1)
}

func (m *MockScheduledJobCostRepository) ListByStatus(ctx context.Context, status string, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	args := m.Called(ctx, status, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ScheduledJobCostRecord), args.Error(1)
}

func (m *MockScheduledJobCostRepository) ListByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	args := m.Called(ctx, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ScheduledJobCostRecord), args.Error(1)
}

func (m *MockScheduledJobCostRepository) GetFailedJobs(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	args := m.Called(ctx, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ScheduledJobCostRecord), args.Error(1)
}

func (m *MockScheduledJobCostRepository) GetLongRunningJobs(ctx context.Context, thresholdMs int64, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	args := m.Called(ctx, thresholdMs, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ScheduledJobCostRecord), args.Error(1)
}

func (m *MockScheduledJobCostRepository) GetHighCostJobs(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.ScheduledJobCostRecord, error) {
	args := m.Called(ctx, thresholdDollars, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ScheduledJobCostRecord), args.Error(1)
}

func (m *MockScheduledJobCostRepository) AggregateJobCosts(ctx context.Context, jobName, period string, windowStart, windowEnd time.Time) error {
	args := m.Called(ctx, jobName, period, windowStart, windowEnd)
	return args.Error(0)
}

func TestNewScheduledJobCostTracker(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()

	tracker := NewScheduledJobCostTracker(mockRepo, logger)
	require.NotNil(t, tracker)
	assert.Equal(t, mockRepo, tracker.repository)
	assert.Equal(t, logger, tracker.logger)
}

func TestNewJobExecution(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")
	require.NotNil(t, execution)

	assert.Equal(t, "test-job", execution.JobName)
	assert.Equal(t, "hourly", execution.Schedule)
	assert.False(t, execution.StartTime.IsZero())
	assert.NotNil(t, execution.properties)
	assert.NotNil(t, execution.performanceMetrics)
	assert.NotNil(t, execution.tags)
}

func TestJobExecution_WithCategory(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly").
		WithCategory("maintenance")

	assert.Equal(t, "maintenance", execution.Category)
}

func TestJobExecution_WithPriority(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly").
		WithPriority("high")

	assert.Equal(t, "high", execution.Priority)
}

func TestJobExecution_WithContext(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly").
		WithContext("production", "us-east-1", "my-function", "req-123")

	assert.Equal(t, "production", execution.Environment)
	assert.Equal(t, "us-east-1", execution.Region)
	assert.Equal(t, "my-function", execution.FunctionName)
	assert.Equal(t, "req-123", execution.RequestID)
}

func TestJobExecution_WithScheduling(t *testing.T) {
	now := time.Now()
	next := now.Add(time.Hour)

	execution := NewJobExecution("test-job", "hourly").
		WithScheduling("0 * * * *", now, next)

	assert.Equal(t, "0 * * * *", execution.CronPattern)
	assert.Equal(t, now, execution.ScheduledTime)
	assert.Equal(t, next, execution.NextScheduledTime)
}

func TestJobExecution_TrackLambdaUsage(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackLambdaUsage(1, 100, 256)
	execution.TrackLambdaUsage(1, 200, 128)

	assert.Equal(t, int64(2), execution.lambdaInvocations)
	assert.Equal(t, int64(300), execution.lambdaDurationMs)
	assert.Equal(t, 256, execution.lambdaMemoryUsedMB) // Peak memory
}

func TestJobExecution_TrackDynamoDBUsage(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackDynamoDBUsage(10, 5, 2.5, 1.5)
	execution.TrackDynamoDBUsage(20, 10, 5.0, 3.0)

	assert.Equal(t, int64(30), execution.dynamoDBReadOps)
	assert.Equal(t, int64(15), execution.dynamoDBWriteOps)
	assert.InDelta(t, 7.5, execution.dynamoDBReadCapacity, 0.01)
	assert.InDelta(t, 4.5, execution.dynamoDBWriteCapacity, 0.01)
}

func TestJobExecution_TrackSQSUsage(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackSQSUsage(100)
	execution.TrackSQSUsage(50)

	assert.Equal(t, int64(150), execution.sqsMessages)
}

func TestJobExecution_TrackS3Usage(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackS3Usage(10)
	execution.TrackS3Usage(5)

	assert.Equal(t, int64(15), execution.s3Operations)
}

func TestJobExecution_TrackCloudWatchLogs(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackCloudWatchLogs(1000)
	execution.TrackCloudWatchLogs(500)

	assert.Equal(t, int64(1500), execution.cloudWatchLogs)
}

func TestJobExecution_TrackDataTransfer(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackDataTransfer(1024)
	execution.TrackDataTransfer(2048)

	assert.Equal(t, int64(3072), execution.dataTransferBytes)
}

func TestJobExecution_TrackExternalAPIRequests(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackExternalAPIRequests(5)
	execution.TrackExternalAPIRequests(3)

	assert.Equal(t, int64(8), execution.externalAPIRequests)
}

func TestJobExecution_TrackItemsProcessed(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackItemsProcessed(100, 10, 5)
	execution.TrackItemsProcessed(50, 5, 2)

	assert.Equal(t, int64(150), execution.itemsProcessed)
	assert.Equal(t, int64(15), execution.itemsSkipped)
	assert.Equal(t, int64(7), execution.itemsErrored)
}

func TestJobExecution_SetBatchSize(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.SetBatchSize(100)

	assert.Equal(t, 100, execution.batchSize)
}

func TestJobExecution_TrackCascadingCosts(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.TrackCascadingCosts([]string{"job-a", "job-b"}, 1000, 10)
	execution.TrackCascadingCosts([]string{"job-c"}, 500, 5)

	assert.Len(t, execution.triggeredJobs, 3)
	assert.Equal(t, int64(1500), execution.cascadingCostMicroCents)
	assert.Equal(t, int64(15), execution.downstreamOperations)
}

func TestJobExecution_SetProperty(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.SetProperty("custom_key", "custom_value")

	assert.Equal(t, "custom_value", execution.properties["custom_key"])
}

func TestJobExecution_SetPerformanceMetric(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.SetPerformanceMetric("latency_p99", 150.5)

	assert.Equal(t, 150.5, execution.performanceMetrics["latency_p99"])
}

func TestJobExecution_AddTag(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.AddTag("environment", "production")

	assert.Equal(t, "production", execution.tags["environment"])
}

func TestJobExecution_MarkError(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	execution.MarkError("connection timeout", 2, 3)

	assert.Equal(t, "connection timeout", execution.errorMessage)
	assert.Equal(t, 2, execution.retryCount)
	assert.Equal(t, 3, execution.maxRetries)
}

func TestJobExecution_FinishWithSuccess(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	execution := NewJobExecution("test-job", "hourly")
	execution.TrackLambdaUsage(1, 100, 256)
	execution.TrackItemsProcessed(50, 5, 0)

	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.ScheduledJobCostRecord")).Return(nil)

	ctx := context.Background()
	err := execution.FinishWithSuccess(ctx, tracker)
	assert.NoError(t, err)

	assert.NotNil(t, execution.GetRecord())
	mockRepo.AssertExpectations(t)
}

func TestJobExecution_FinishWithError(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	execution := NewJobExecution("test-job", "hourly")

	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.ScheduledJobCostRecord")).Return(nil)

	ctx := context.Background()
	err := execution.FinishWithError(ctx, tracker, "database connection failed")
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)
}

func TestJobExecution_FinishWithTimeout(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	execution := NewJobExecution("test-job", "hourly")

	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.ScheduledJobCostRecord")).Return(nil)

	ctx := context.Background()
	err := execution.FinishWithTimeout(ctx, tracker)
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)
}

func TestJobExecution_FinishWithCancellation(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	execution := NewJobExecution("test-job", "hourly")

	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.ScheduledJobCostRecord")).Return(nil)

	ctx := context.Background()
	err := execution.FinishWithCancellation(ctx, tracker)
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)
}

func TestJobExecution_FinishWithError_RepositoryError(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	execution := NewJobExecution("test-job", "hourly")

	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.ScheduledJobCostRecord")).Return(errors.New("database error"))

	ctx := context.Background()
	err := execution.FinishWithSuccess(ctx, tracker)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to persist job cost record")
}

func TestJobExecution_GetRecord(t *testing.T) {
	execution := NewJobExecution("test-job", "hourly")

	// Before finish, record should be nil
	assert.Nil(t, execution.GetRecord())
}

func TestNewMultiStepJobExecution(t *testing.T) {
	execution := NewMultiStepJobExecution("multi-step-job", "daily")
	require.NotNil(t, execution)

	assert.Equal(t, "multi-step-job", execution.JobName)
	assert.NotNil(t, execution.steps)
}

func TestMultiStepJobExecution_StartStep(t *testing.T) {
	execution := NewMultiStepJobExecution("multi-step-job", "daily")

	step := execution.StartStep("step-1")
	require.NotNil(t, step)

	assert.Equal(t, "step-1", step.StepName)
	assert.Equal(t, "running", step.Status)
	assert.False(t, step.StartTime.IsZero())
	assert.Equal(t, "step-1", execution.currentStep)
}

func TestMultiStepJobExecution_FinishStep(t *testing.T) {
	execution := NewMultiStepJobExecution("multi-step-job", "daily")

	step := execution.StartStep("step-1")
	step.TrackStepItemsProcessed(100, 5)

	execution.FinishStep("step-1", "success", nil)

	assert.Equal(t, "success", step.Status)
	assert.False(t, step.EndTime.IsZero())
}

func TestMultiStepJobExecution_FinishStep_WithError(t *testing.T) {
	execution := NewMultiStepJobExecution("multi-step-job", "daily")

	step := execution.StartStep("step-1")

	execution.FinishStep("step-1", "failed", errors.New("step failed"))

	assert.Equal(t, "failed", step.Status)
	assert.Equal(t, "step failed", step.Error)
}

func TestMultiStepJobExecution_FinishStep_NonExistent(t *testing.T) {
	execution := NewMultiStepJobExecution("multi-step-job", "daily")

	// Should not panic
	execution.FinishStep("non-existent", "success", nil)
}

func TestMultiStepJobExecution_GetCurrentStep(t *testing.T) {
	execution := NewMultiStepJobExecution("multi-step-job", "daily")

	// No current step
	assert.Nil(t, execution.GetCurrentStep())

	execution.StartStep("step-1")
	step := execution.GetCurrentStep()
	require.NotNil(t, step)
	assert.Equal(t, "step-1", step.StepName)
}

func TestMultiStepJobExecution_GetStepSummary(t *testing.T) {
	execution := NewMultiStepJobExecution("multi-step-job", "daily")

	execution.StartStep("step-1")
	execution.StartStep("step-2")

	summary := execution.GetStepSummary()
	assert.Len(t, summary, 2)
}

func TestJobStepExecution_TrackStepLambdaUsage(t *testing.T) {
	step := &JobStepExecution{
		StepName:   "test-step",
		properties: make(map[string]interface{}),
		metrics:    make(map[string]float64),
	}

	step.TrackStepLambdaUsage(100)
	step.TrackStepLambdaUsage(50)

	assert.Equal(t, int64(150), step.lambdaDurationMs)
}

func TestJobStepExecution_TrackStepDynamoDBUsage(t *testing.T) {
	step := &JobStepExecution{
		StepName:   "test-step",
		properties: make(map[string]interface{}),
		metrics:    make(map[string]float64),
	}

	step.TrackStepDynamoDBUsage(10)
	step.TrackStepDynamoDBUsage(5)

	assert.Equal(t, int64(15), step.dynamoDBOperations)
}

func TestJobStepExecution_TrackStepItemsProcessed(t *testing.T) {
	step := &JobStepExecution{
		StepName:   "test-step",
		properties: make(map[string]interface{}),
		metrics:    make(map[string]float64),
	}

	step.TrackStepItemsProcessed(100, 5)
	step.TrackStepItemsProcessed(50, 2)

	assert.Equal(t, int64(150), step.itemsProcessed)
	assert.Equal(t, int64(7), step.itemsErrored)
}

func TestJobStepExecution_SetStepProperty(t *testing.T) {
	step := &JobStepExecution{
		StepName:   "test-step",
		properties: make(map[string]interface{}),
		metrics:    make(map[string]float64),
	}

	step.SetStepProperty("key", "value")

	assert.Equal(t, "value", step.properties["key"])
}

func TestJobStepExecution_SetStepMetric(t *testing.T) {
	step := &JobStepExecution{
		StepName:   "test-step",
		properties: make(map[string]interface{}),
		metrics:    make(map[string]float64),
	}

	step.SetStepMetric("latency", 100.5)

	assert.Equal(t, 100.5, step.metrics["latency"])
}

func TestScheduledJobCostTracker_TrackCostAggregationJob(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()
	now := time.Now()

	execution, err := tracker.TrackCostAggregationJob(ctx, "hourly", now.Add(-time.Hour), now)
	assert.NoError(t, err)
	require.NotNil(t, execution)

	assert.Equal(t, "cost-aggregation", execution.JobName)
	assert.Equal(t, "maintenance", execution.Category)
}

func TestScheduledJobCostTracker_TrackCleanupJob(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()

	execution, err := tracker.TrackCleanupJob(ctx, "expired-data", "daily")
	assert.NoError(t, err)
	require.NotNil(t, execution)

	assert.Equal(t, "cleanup-expired-data", execution.JobName)
	assert.Equal(t, "maintenance", execution.Category)
}

func TestScheduledJobCostTracker_TrackTrendCalculationJob(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()

	execution, err := tracker.TrackTrendCalculationJob(ctx, "cost", "hourly")
	assert.NoError(t, err)
	require.NotNil(t, execution)

	assert.Equal(t, "trend-calculation-cost", execution.JobName)
	assert.Equal(t, "analytics", execution.Category)
}

func TestScheduledJobCostTracker_TrackIndexOptimizationJob(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()

	execution, err := tracker.TrackIndexOptimizationJob(ctx, "users")
	assert.NoError(t, err)
	require.NotNil(t, execution)

	assert.Equal(t, "index-optimization", execution.JobName)
	assert.Equal(t, "optimization", execution.Category)
}

func TestScheduledJobCostTracker_TrackDeadLetterQueueProcessingJob(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()

	execution, err := tracker.TrackDeadLetterQueueProcessingJob(ctx, "my-dlq")
	assert.NoError(t, err)
	require.NotNil(t, execution)

	assert.Equal(t, "dlq-processing", execution.JobName)
	assert.Equal(t, "recovery", execution.Category)
}

func TestScheduledJobCostTracker_AggregateJobCosts(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()
	now := time.Now()

	mockRepo.On("AggregateJobCosts", ctx, "test-job", "hourly", mock.Anything, mock.Anything).Return(nil)

	err := tracker.AggregateJobCosts(ctx, "test-job", "hourly", now.Add(-time.Hour), now)
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)
}

func TestScheduledJobCostTracker_AggregateJobCosts_Error(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()
	now := time.Now()

	mockRepo.On("AggregateJobCosts", ctx, "test-job", "hourly", mock.Anything, mock.Anything).Return(errors.New("aggregation failed"))

	err := tracker.AggregateJobCosts(ctx, "test-job", "hourly", now.Add(-time.Hour), now)
	assert.Error(t, err)
}

func TestScheduledJobCostTracker_GetFailedJobs(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()
	now := time.Now()

	expectedRecords := []*models.ScheduledJobCostRecord{
		{JobName: "failed-job-1"},
		{JobName: "failed-job-2"},
	}

	mockRepo.On("GetFailedJobs", ctx, mock.Anything, mock.Anything, 10).Return(expectedRecords, nil)

	records, err := tracker.GetFailedJobs(ctx, now.Add(-24*time.Hour), now, 10)
	assert.NoError(t, err)
	assert.Len(t, records, 2)
}

func TestScheduledJobCostTracker_GetHighCostJobs(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()
	now := time.Now()

	expectedRecords := []*models.ScheduledJobCostRecord{
		{JobName: "expensive-job"},
	}

	mockRepo.On("GetHighCostJobs", ctx, 1.0, mock.Anything, mock.Anything, 10).Return(expectedRecords, nil)

	records, err := tracker.GetHighCostJobs(ctx, 1.0, now.Add(-24*time.Hour), now, 10)
	assert.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestScheduledJobCostTracker_GetLongRunningJobs(t *testing.T) {
	mockRepo := &MockScheduledJobCostRepository{}
	logger := zap.NewNop()
	tracker := NewScheduledJobCostTracker(mockRepo, logger)

	ctx := context.Background()
	now := time.Now()

	expectedRecords := []*models.ScheduledJobCostRecord{
		{JobName: "slow-job"},
	}

	mockRepo.On("GetLongRunningJobs", ctx, int64(60000), mock.Anything, mock.Anything, 10).Return(expectedRecords, nil)

	records, err := tracker.GetLongRunningJobs(ctx, 60000, now.Add(-24*time.Hour), now, 10)
	assert.NoError(t, err)
	assert.Len(t, records, 1)
}
