package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func setupMetricsRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()

	mockQuery.On("BatchCreate", mock.Anything).Return(nil).Maybe()
	mockQuery.On("BatchDelete", mock.Anything).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.Metrics")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.Metrics)
		*out = []*models.Metrics{
			{
				ID:        "m-1",
				Type:      "request",
				Service:   "api",
				Period:    "minute",
				Timestamp: baseTime.Add(-2 * time.Minute),
				Count:     2,
				Sum:       20,
				Min:       5,
				Max:       15,
				Average:   10,
				Dimensions: map[string]string{
					"route": "/v1/statuses",
				},
			},
			{
				ID:        "m-2",
				Type:      "request",
				Service:   "api",
				Period:    "minute",
				Timestamp: baseTime.Add(-1 * time.Minute),
				Count:     1,
				Sum:       40,
				Min:       40,
				Max:       40,
				Average:   40,
				Dimensions: map[string]string{
					"route": "/v1/accounts",
				},
			},
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]models.Metrics")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Metrics)
		*out = []models.Metrics{
			{
				PK:        "metrics#request",
				SK:        "ts#20251228100000#m-1",
				Timestamp: baseTime.Add(-48 * time.Hour),
			},
			{
				PK:        "metrics#request",
				SK:        "ts#20251228120000#m-2",
				Timestamp: baseTime.Add(48 * time.Hour),
			},
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]models.AggregatedMetrics")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.AggregatedMetrics)
		*out = []models.AggregatedMetrics{
			{
				PK:          "metrics_agg#day#request",
				SK:          "window#2025-12-20T00:00:00Z",
				Type:        "request",
				Period:      "day",
				WindowStart: baseTime.Add(-48 * time.Hour),
				WindowEnd:   baseTime.Add(-47 * time.Hour),
			},
			{
				PK:          "metrics_agg#day#request",
				SK:          "window#2026-01-20T00:00:00Z",
				Type:        "request",
				Period:      "day",
				WindowStart: baseTime.Add(48 * time.Hour),
				WindowEnd:   baseTime.Add(49 * time.Hour),
			},
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.MetricRecord")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.MetricRecord)
		*out = []*models.MetricRecord{
			{
				MetricType:       "latency",
				ServiceName:      "api",
				Timestamp:        baseTime.Add(-2 * time.Minute),
				MetricID:         "mr-1",
				AggregationLevel: "raw",
				Count:            2,
				Sum:              30,
				Min:              10,
				Max:              20,
				Dimensions:       map[string]string{"route": "/v1/statuses"},
				GSI3PK:           "DATE#2025-12-28",
				GSI3SK:           "SERVICE#api#2025-12-28T10:00:00Z",
				GSI4PK:           "AGGREGATION#raw",
				GSI4SK:           "TIMESTAMP#2025-12-28T10:00:00Z",
				GSI1PK:           "SERVICE#api",
				GSI1SK:           "TIMESTAMP#2025-12-28T10:00:00Z",
				GSI2PK:           "METRIC_TYPE#latency",
				GSI2SK:           "TIMESTAMP#2025-12-28T10:00:00Z",
			},
		}
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.Metrics")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.Metrics)
		out.ID = "m-1"
		out.Type = "request"
		out.Service = "api"
		out.Period = "minute"
		out.Timestamp = baseTime
		_ = out.UpdateKeys()
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.AggregatedMetrics")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AggregatedMetrics)
		out.Period = "day"
		out.Type = "request"
		out.WindowStart = baseTime.Add(-1 * time.Hour)
		out.WindowEnd = baseTime
		out.CreatedAt = baseTime.Add(-2 * time.Hour)
		_ = out.UpdateKeys()
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.MetricRecord")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.MetricRecord)
		out.MetricType = "latency"
		out.ServiceName = "api"
		out.Timestamp = baseTime
		out.AggregationLevel = "raw"
		out.MetricID = "mr-1"
		_ = out.UpdateKeys()
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
}

func TestMetricsRepository_Round08_CoverageSweep(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupMetricsRepoMocks(mockDB, mockQuery, baseTime)

	constructed := NewMetricsRepository(mockDB, "test-table", logger, nil)
	require.NotNil(t, constructed)

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", logger, nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", logger, nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 logger,
	}

	err := repo.Create(ctx, &models.Metrics{
		Type:      "request",
		Service:   "api",
		Period:    "minute",
		Timestamp: baseTime,
		Count:     1,
		Sum:       10,
		Min:       10,
		Max:       10,
	})
	require.NoError(t, err)

	err = repo.BatchCreate(ctx, nil)
	require.NoError(t, err)

	err = repo.BatchCreate(ctx, []*models.Metrics{{Service: "api"}})
	require.Error(t, err)

	_, err = repo.Get(ctx, "request", "m-1", baseTime)
	require.NoError(t, err)

	_, err = repo.ListByType(ctx, "request", baseTime.Add(-10*time.Minute), baseTime, 10)
	require.NoError(t, err)

	_, err = repo.ListByService(ctx, "api", baseTime.Add(-10*time.Minute), baseTime, 10)
	require.NoError(t, err)

	err = repo.CreateAggregated(ctx, &models.AggregatedMetrics{
		Period:      "day",
		Type:        "request",
		Service:     "api",
		WindowStart: baseTime.Add(-1 * time.Hour),
		WindowEnd:   baseTime,
	})
	require.NoError(t, err)

	err = repo.UpdateAggregated(ctx, &models.AggregatedMetrics{
		Period:      "day",
		Type:        "request",
		Service:     "api",
		WindowStart: baseTime.Add(-1 * time.Hour),
		WindowEnd:   baseTime,
	})
	require.NoError(t, err)

	_, err = repo.GetAggregated(ctx, "day", "request", baseTime.Add(-1*time.Hour))
	require.NoError(t, err)

	_, _, err = repo.ListAggregatedByPeriod(ctx, "day", "request", baseTime.Add(-24*time.Hour), baseTime, 10, "")
	require.NoError(t, err)

	stats, err := repo.GetServiceStats(ctx, "api", "request", baseTime.Add(-10*time.Minute), baseTime)
	require.NoError(t, err)
	require.Greater(t, stats.Count, 0)

	err = repo.Aggregate(ctx, "request", "day", baseTime.Add(-1*time.Hour), baseTime)
	require.NoError(t, err)

	deleted, err := repo.CleanupOldMetrics(ctx, "all", baseTime)
	require.NoError(t, err)
	require.GreaterOrEqual(t, deleted, 0)

	percentiles := calculateMetricPercentiles([]float64{1, 2, 3, 4, 5})
	require.NotEmpty(t, percentiles)
	require.Equal(t, float64(0), calculateStandardDeviation([]float64{1}, 1))
}

func TestMetricsRepository_ListByType_Error(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Metrics")).Return(errors.New("query failed")).Once()

	repo := NewMetricsRepository(mockDB, "test-table", logger, nil)
	_, err := repo.ListByType(ctx, "request", time.Now().Add(-time.Hour), time.Now(), 10)
	require.Error(t, err)
}

func TestMetricRecordRepository_Round08_CoverageSweep(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupMetricsRepoMocks(mockDB, mockQuery, baseTime)

	constructed := NewMetricRecordRepository(mockDB, "test-table", logger, nil)
	require.NotNil(t, constructed)

	repo := &MetricRecordRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.MetricRecord](mockDB, "test-table", logger, nil, "MetricRecordRepository", "metricrecord"),
		logger:                 logger,
	}

	_, err := repo.GetMetricsByService(ctx, "api", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, err)

	_, err = repo.GetMetricsByType(ctx, "latency", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, err)

	_, err = repo.GetMetricsByAggregationLevel(ctx, "raw", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, err)

	_, err = repo.GetMetricsByDate(ctx, baseTime, "")
	require.NoError(t, err)

	_, err = repo.GetMetricsByDate(ctx, baseTime, "api")
	require.NoError(t, err)

	err = repo.CreateMetricRecord(ctx, &models.MetricRecord{
		MetricType:       "latency",
		ServiceName:      "api",
		Timestamp:        baseTime,
		AggregationLevel: "raw",
		Count:            1,
		Sum:              12,
		Min:              12,
		Max:              12,
		Dimensions:       map[string]string{"route": "/v1/statuses"},
	})
	require.NoError(t, err)

	err = repo.BatchCreateMetricRecords(ctx, nil)
	require.NoError(t, err)

	err = repo.BatchCreateMetricRecords(ctx, []*models.MetricRecord{{ServiceName: "api"}})
	require.Error(t, err)

	_, err = repo.GetMetricRecord(ctx, "latency", "bucket", baseTime.Format(time.RFC3339))
	require.NoError(t, err)

	err = repo.UpdateMetricRecord(ctx, &models.MetricRecord{
		MetricType:       "latency",
		ServiceName:      "api",
		Timestamp:        baseTime,
		AggregationLevel: "raw",
		MetricID:         "mr-1",
	})
	require.NoError(t, err)

	err = repo.DeleteMetricRecord(ctx, "latency", "bucket", baseTime.Format(time.RFC3339))
	require.NoError(t, err)

	stats, err := repo.GetServiceMetricsStats(ctx, "api", "latency", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Count, 0)
}

func TestMetricsRepository_BatchCreate_Success(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupMetricsRepoMocks(mockDB, mockQuery, baseTime)

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", logger, nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", logger, nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 logger,
	}

	err := repo.BatchCreate(ctx, []*models.Metrics{
		{Type: "request", Service: "api", Period: "minute", Timestamp: baseTime, Count: 1, Sum: 10, Min: 10, Max: 10},
		{Type: "error", Service: "api", Period: "minute", Timestamp: baseTime.Add(time.Minute), Count: 1, Sum: 1, Min: 1, Max: 1},
	})
	require.NoError(t, err)
}

func TestMetricsRepository_Aggregate_CreatePathWhenNoExisting(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.AggregatedMetrics")).Return(dynamormErrors.ErrItemNotFound).Once()
	setupMetricsRepoMocks(mockDB, mockQuery, baseTime)

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", logger, nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", logger, nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 logger,
	}

	err := repo.Aggregate(ctx, "request", "day", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, err)
}

func TestMetricsRepository_GetServiceStats_FilteredEmpty(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupMetricsRepoMocks(mockDB, mockQuery, baseTime)

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", logger, nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", logger, nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 logger,
	}

	stats, err := repo.GetServiceStats(ctx, "api", "does-not-match", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, err)
	require.Equal(t, 0, stats.Count)
}

func TestMetricsRepository_MathHelpers_EdgeCases(t *testing.T) {
	require.Equal(t, map[string]float64{"p50": 0, "p90": 0, "p95": 0, "p99": 0}, calculateMetricPercentiles(nil))
	require.Equal(t, float64(0), getMetricPercentileValue(nil, 50))
	require.Equal(t, float64(10), getMetricPercentileValue([]float64{10}, 90))
	require.Equal(t, float64(1), getMetricPercentileValue([]float64{1, 2}, 0))
	require.Equal(t, float64(2), getMetricPercentileValue([]float64{1, 2}, 100))
	require.Greater(t, calculateStandardDeviation([]float64{1, 2}, 1.5), float64(0))
}

func TestMetricsRepository_CleanupOldMetrics_DeleteFailures(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
	setupMetricsRepoMocks(mockDB, mockQuery, baseTime)

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", logger, nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", logger, nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 logger,
	}

	_, err := repo.CleanupOldMetrics(ctx, "all", baseTime)
	require.NoError(t, err)
}

func TestMetricsRepository_ListByService_Error(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Metrics")).Return(errors.New("query failed")).Once()

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", logger, nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", logger, nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 logger,
	}
	_, err := repo.ListByService(ctx, "api", time.Now().Add(-time.Hour), time.Now(), 10)
	require.Error(t, err)
}

func TestMetricsRepository_CreateAggregated_ValidationError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupMetricsRepoMocks(mockDB, mockQuery, time.Now())

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", zap.NewNop(), nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", zap.NewNop(), nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 zap.NewNop(),
	}
	err := repo.CreateAggregated(context.Background(), &models.AggregatedMetrics{
		Period:      "day",
		Type:        "",
		WindowStart: time.Now(),
		WindowEnd:   time.Now().Add(-time.Hour),
	})
	require.Error(t, err)
}

func TestMetricRecordRepository_QueryErrors(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Maybe()

	repo := &MetricRecordRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.MetricRecord](mockDB, "test-table", logger, nil, "MetricRecordRepository", "metricrecord"),
		logger:                 logger,
	}

	_, err := repo.GetMetricsByService(ctx, "api", time.Now().Add(-time.Hour), time.Now())
	require.Error(t, err)

	_, err = repo.GetMetricsByDate(ctx, time.Now(), "api")
	require.Error(t, err)
}

func TestMetricsRepository_UpdateAggregated_ValidationError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupMetricsRepoMocks(mockDB, mockQuery, time.Now())

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", zap.NewNop(), nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", zap.NewNop(), nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 zap.NewNop(),
	}

	err := repo.UpdateAggregated(context.Background(), &models.AggregatedMetrics{
		Period:      "day",
		Type:        "",
		WindowStart: time.Now(),
		WindowEnd:   time.Now().Add(-time.Hour),
	})
	require.Error(t, err)
}

func TestMetricsRepository_CleanupOldMetrics_HandlesQueryErrors(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.AggregatedMetrics")).Return(errors.New("query failed")).Once()
	setupMetricsRepoMocks(mockDB, mockQuery, baseTime)

	repo := &MetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Metrics](mockDB, "test-table", logger, nil, "MetricsRepository", "metrics"),
		aggregatedRepo:         NewEnhancedBaseRepository[*models.AggregatedMetrics](mockDB, "test-table", logger, nil, "AggregatedMetricsRepository", "aggregatedmetrics"),
		logger:                 logger,
	}

	_, err := repo.CleanupOldMetrics(ctx, "aggregated", baseTime)
	require.NoError(t, err)
}
