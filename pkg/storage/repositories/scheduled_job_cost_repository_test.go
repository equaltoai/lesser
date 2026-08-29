package repositories

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func setupScheduledJobRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateScheduledJobSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateScheduledJobStructForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()
}

func populateScheduledJobSliceForCoverage(target any, baseTime time.Time) {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
		return
	}

	slice := v.Elem()
	elemType := slice.Type().Elem()
	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}

	switch baseElemType {
	case reflect.TypeOf(models.ScheduledJobCostRecord{}):
		records := []*models.ScheduledJobCostRecord{
			{
				ID:                  "run-1",
				JobName:             "cleanup-expired-data",
				Schedule:            "daily",
				Status:              "success",
				Timestamp:           baseTime.Add(-2 * time.Hour),
				StartTime:           baseTime.Add(-2 * time.Hour),
				EndTime:             baseTime.Add(-2*time.Hour + 2*time.Minute),
				Duration:            2 * 60 * 1000,
				TotalCostMicroCents: 2000,
				ItemsProcessed:      100,
				JobCategory:         "maintenance",
				Environment:         "production",
			},
			{
				ID:                  "run-2",
				JobName:             "cost-aggregation",
				Schedule:            "hourly",
				Status:              "failed",
				Timestamp:           baseTime.Add(-time.Hour),
				StartTime:           baseTime.Add(-time.Hour),
				EndTime:             baseTime.Add(-time.Hour + time.Minute),
				Duration:            60 * 1000,
				TotalCostMicroCents: 5000,
				ItemsProcessed:      10,
				ItemsErrored:        3,
				JobCategory:         "aggregation",
				Environment:         "production",
			},
			{
				ID:                  "target-id",
				JobName:             "router-refresh",
				Schedule:            "daily",
				Status:              "timeout",
				Timestamp:           baseTime.Add(-30 * time.Minute),
				StartTime:           baseTime.Add(-30 * time.Minute),
				EndTime:             baseTime.Add(-29 * time.Minute),
				Duration:            60 * 1000,
				TotalCostMicroCents: 10000,
				ItemsProcessed:      1,
				JobCategory:         "cleanup",
				Environment:         "staging",
			},
			{
				ID:                  "run-3",
				JobName:             "router-refresh",
				Schedule:            "daily",
				Status:              "cancelled",
				Timestamp:           baseTime.Add(-15 * time.Minute),
				StartTime:           baseTime.Add(-15 * time.Minute),
				EndTime:             baseTime.Add(-14 * time.Minute),
				Duration:            60 * 1000,
				TotalCostMicroCents: 100,
				ItemsProcessed:      1,
				JobCategory:         "cleanup",
				Environment:         "staging",
			},
		}
		for _, record := range records {
			_ = record.BeforeCreate()
			slice = reflect.Append(slice, reflect.ValueOf(record))
		}
		v.Elem().Set(slice)
		return

	case reflect.TypeOf(models.ScheduledJobCostAggregation{}):
		agg := &models.ScheduledJobCostAggregation{
			JobName:             "router-refresh",
			Period:              "day",
			WindowStart:         baseTime.Truncate(24 * time.Hour),
			WindowEnd:           baseTime.Truncate(24 * time.Hour).Add(24 * time.Hour),
			TotalCostMicroCents: 10000,
			TotalCostDollars:    0.01,
		}
		_ = agg.BeforeCreate()
		slice = reflect.Append(slice, reflect.ValueOf(agg))
		v.Elem().Set(slice)
		return
	}
}

func populateScheduledJobStructForCoverage(target any, baseTime time.Time) {
	switch model := target.(type) {
	case *models.ScheduledJobCostRecord:
		*model = models.ScheduledJobCostRecord{
			ID:             "run-1",
			JobName:        "cleanup-expired-data",
			Schedule:       "daily",
			Status:         "success",
			Timestamp:      baseTime,
			StartTime:      baseTime,
			EndTime:        baseTime.Add(2 * time.Minute),
			Duration:       2 * 60 * 1000,
			JobCategory:    "maintenance",
			Environment:    "production",
			ItemsProcessed: 100,
		}
		_ = model.BeforeCreate()

	case *models.ScheduledJobCostAggregation:
		*model = models.ScheduledJobCostAggregation{
			JobName:     "router-refresh",
			Period:      "day",
			WindowStart: baseTime.Truncate(24 * time.Hour),
			WindowEnd:   baseTime.Truncate(24 * time.Hour).Add(24 * time.Hour),
		}
		_ = model.BeforeCreate()
	}
}

func TestScheduledJobCostRepository_ConstructAndCoverageSweep(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupScheduledJobRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewScheduledJobCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NotNil(t, repo)

	// Create + Update
	require.NoError(t, repo.Create(ctx, &models.ScheduledJobCostRecord{
		ID:             "run-1",
		JobName:        "cleanup-expired-data",
		Schedule:       "daily",
		Status:         "success",
		Timestamp:      baseTime,
		StartTime:      baseTime,
		EndTime:        baseTime.Add(2 * time.Minute),
		ItemsProcessed: 100,
		JobCategory:    "maintenance",
		Environment:    "production",
	}))
	require.NoError(t, repo.Update(ctx, &models.ScheduledJobCostRecord{
		ID:             "run-1",
		JobName:        "cleanup-expired-data",
		Schedule:       "daily",
		Status:         "success",
		Timestamp:      baseTime,
		StartTime:      baseTime,
		EndTime:        baseTime.Add(2 * time.Minute),
		ItemsProcessed: 100,
		JobCategory:    "maintenance",
		Environment:    "production",
	}))

	// Get + listing
	_, _ = repo.Get(ctx, "cleanup-expired-data", "daily", baseTime, "run-1")
	_, _ = repo.ListByJob(ctx, "cleanup-expired-data", "daily", baseTime.Add(-24*time.Hour), baseTime, 10)
	_, _ = repo.ListByStatus(ctx, "success", baseTime.Add(-24*time.Hour), baseTime, 10)
	_, _ = repo.ListByDateRange(ctx, baseTime.Add(-48*time.Hour), baseTime, 10)
	_, _ = repo.GetFailedJobs(ctx, baseTime.Add(-24*time.Hour), baseTime, 5)
	_, _ = repo.GetLongRunningJobs(ctx, 90*1000, baseTime.Add(-24*time.Hour), baseTime, 5)
	_, _ = repo.GetHighCostJobs(ctx, 0.000001, baseTime.Add(-48*time.Hour), baseTime, 1)

	// Stats and trends
	_, _ = repo.GetJobExecutionStats(ctx, "cleanup-expired-data", "daily", baseTime.Add(-24*time.Hour), baseTime)
	_, _ = repo.GetJobPerformanceTrends(ctx, "cleanup-expired-data", "daily", 2)

	// Aggregation CRUD + aggregation flow
	agg := &models.ScheduledJobCostAggregation{
		JobName:     "cleanup-expired-data",
		Period:      "day",
		WindowStart: baseTime.Truncate(24 * time.Hour),
		WindowEnd:   baseTime.Truncate(24 * time.Hour).Add(24 * time.Hour),
	}
	require.NoError(t, repo.CreateAggregation(ctx, agg))
	require.NoError(t, repo.UpdateAggregation(ctx, agg))
	_, _ = repo.GetAggregation(ctx, "day", "cleanup-expired-data", baseTime.Truncate(24*time.Hour))
	require.NoError(t, repo.AggregateJobCosts(ctx, "cleanup-expired-data", "day", baseTime.Add(-24*time.Hour), baseTime))
}

func TestScheduledJobCostRepository_SummaryHelpers_Coverage(t *testing.T) {
	repo := &ScheduledJobCostRepository{}
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	start := baseTime.Add(-48 * time.Hour)
	end := baseTime

	records := []*models.ScheduledJobCostRecord{
		{ID: "r1", JobName: "cleanup", Schedule: "daily", Status: "success", Success: true, Timestamp: start.Add(time.Hour), Duration: 120000, TotalCostMicroCents: 1000, ItemsProcessed: 100, JobCategory: "maintenance"},
		{ID: "r2", JobName: "cleanup", Schedule: "daily", Status: "failed", Success: false, Timestamp: start.Add(2 * time.Hour), Duration: 60000, TotalCostMicroCents: 5000, ItemsProcessed: 10, JobCategory: "maintenance"},
		{ID: "r3", JobName: "aggregation", Schedule: "hourly", Status: "success", Success: true, Timestamp: start.Add(26 * time.Hour), Duration: 30000, TotalCostMicroCents: 2500, ItemsProcessed: 50, JobCategory: "aggregation"},
	}

	summary := repo.initializeJobsSummary(start, end, records)
	repo.processSummaryRecords(summary, records)
	repo.calculateOverallMetrics(summary)
	repo.calculateJobMetrics(summary)
	repo.calculateCategoryMetrics(summary)
	repo.calculateScheduleMetrics(summary)

	require.Equal(t, 3, summary.TotalExecutions)
	require.NotEmpty(t, summary.JobBreakdown)
	require.NotEmpty(t, summary.CategoryBreakdown)
	require.NotEmpty(t, summary.ScheduleBreakdown)
}

func TestScheduledJobCostRepository_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupScheduledJobRepoMocks(mockDB, mockQuery, baseTime)
	repo := NewScheduledJobCostRepository(mockDB, "test-table", zap.NewNop(), nil)

	// BeforeCreate validation failure
	require.Error(t, repo.Create(ctx, &models.ScheduledJobCostRecord{
		ID:      "run-1",
		JobName: "cleanup-expired-data",
		// invalid schedule triggers Validate failure
		Schedule: "not-a-real-schedule",
		Status:   "success",
	}))

	// BeforeUpdate validation failure
	require.Error(t, repo.Update(ctx, &models.ScheduledJobCostRecord{
		ID:       "run-1",
		JobName:  "cleanup-expired-data",
		Schedule: "daily",
		Status:   "not-a-real-status",
	}))
}

func TestScheduledJobCostRepository_GetByID_WarnsAndNotFound(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// GetByID is now a per-status page-capped walk (wave #1469): Limit(500)/
	// page via AllPaginated. First status walk fails => warn+continue.
	mockQuery.On("Limit", 500).Return(mockQuery).Maybe()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.ScheduledJobCostRecord")).Return(nil, errors.New("query failed")).Once()

	// Next few statuses return records without the ID.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.ScheduledJobCostRecord")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.ScheduledJobCostRecord)
		*out = []*models.ScheduledJobCostRecord{{ID: "other", Timestamp: baseTime}}
	}).Return(&core.PaginatedResult{}, nil).Maybe()

	repo := NewScheduledJobCostRepository(mockDB, "test-table", logger, nil)
	_, err := repo.GetByID(ctx, "missing-id")
	require.Error(t, err)
}

func TestScheduledJobCostRepository_ListByDateRange_ContinuesOnPerDayError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi2").Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	// First day fails and is skipped.
	mockQuery.On("All", mock.AnythingOfType("*[]*models.ScheduledJobCostRecord")).Return(errors.New("day query failed")).Once()
	// Second day returns a record.
	mockQuery.On("All", mock.AnythingOfType("*[]*models.ScheduledJobCostRecord")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.ScheduledJobCostRecord)
		*out = []*models.ScheduledJobCostRecord{
			{ID: "run-1", Timestamp: baseTime.Add(-time.Hour), JobName: "cleanup-expired-data", Schedule: "daily", Status: "success"},
		}
	}).Return(nil).Once()

	repo := NewScheduledJobCostRepository(mockDB, "test-table", logger, nil)
	records, err := repo.ListByDateRange(ctx, baseTime.AddDate(0, 0, -1), baseTime, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
}

func TestScheduledJobCostRepository_AggregateJobCosts_EmptyIsNoop(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	// ListByJob -> query.All returns empty slice.
	mockQuery.On("All", mock.AnythingOfType("*[]*models.ScheduledJobCostRecord")).Return(nil).Once()

	repo := NewScheduledJobCostRepository(mockDB, "test-table", logger, nil)
	require.NoError(t, repo.AggregateJobCosts(ctx, "cleanup-expired-data", "day", baseTime.Add(-time.Hour), baseTime))
}

func TestScheduledJobCostRepository_saveAggregation_UpdatePath(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// GetAggregation returns an existing aggregation
	mockQuery.On("First", mock.AnythingOfType("*models.ScheduledJobCostAggregation")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.ScheduledJobCostAggregation)
		out.CreatedAt = baseTime.Add(-time.Hour)
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewScheduledJobCostRepository(mockDB, "test-table", logger, nil)

	aggregation := repo.initializeAggregation("cleanup-expired-data", "day", baseTime, baseTime.Add(24*time.Hour))
	require.NoError(t, repo.saveAggregation(ctx, aggregation, "day", "cleanup-expired-data", baseTime))
}

func TestScheduledJobCostRepository_saveAggregation_CreatePath(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// GetAggregation fails => CreateAggregation called.
	mockQuery.On("First", mock.AnythingOfType("*models.ScheduledJobCostAggregation")).Return(errors.New("missing")).Once()
	mockQuery.On("Create").Return(nil).Once()

	repo := NewScheduledJobCostRepository(mockDB, "test-table", logger, nil)

	aggregation := repo.initializeAggregation("cleanup-expired-data", "day", baseTime, baseTime.Add(24*time.Hour))
	require.NoError(t, repo.saveAggregation(ctx, aggregation, "day", "cleanup-expired-data", baseTime))
}

func TestScheduledJobCostRepository_GetScheduledJobsSummary_FullProcessingPath(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupScheduledJobRepoMocks(mockDB, mockQuery, baseTime)
	repo := NewScheduledJobCostRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Full path through the wrapper: ListByDateRange accumulates a full 10000
	// record page (4 mocked records/day x 2500 days, the wrapper's hardcoded
	// limit), so the summary processes records instead of short-circuiting on
	// zero executions.
	summary, err := repo.GetScheduledJobsSummary(ctx, baseTime.AddDate(0, 0, -2500), baseTime)
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Greater(t, summary.TotalExecutions, 0)
	require.NotEmpty(t, summary.JobBreakdown)
	require.NotEmpty(t, summary.CategoryBreakdown)
	require.NotEmpty(t, summary.ScheduleBreakdown)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
