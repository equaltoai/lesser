package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRoutingMetricsRepository_Constructors(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)
	require.NotNil(t, repo)
	require.NotNil(t, repo.routeMetricsRepo)
	require.NotNil(t, repo.globalMetricsRepo)
	require.NotNil(t, repo.instanceMetricsRepo)

	repo2 := NewRoutingMetricsRepositoryWithCostTracking(mockDB, "test-table", logger, nil)
	require.NotNil(t, repo2)
	require.NotNil(t, repo2.routeMetricsRepo)
	require.NotNil(t, repo2.globalMetricsRepo)
	require.NotNil(t, repo2.instanceMetricsRepo)
}

func TestRoutingMetricsRepository_StoreAndGet_RouteWindows(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil).Once()

	windowStart := time.Date(2025, 1, 2, 3, 4, 0, 0, time.UTC)
	require.NoError(t, repo.StoreRouteMetricsWindow(ctx, &models.RouteMetricsWindow{
		RouteID:          "route-1",
		WindowStart:      windowStart,
		WindowSize:       5,
		MessageCount:     10,
		SuccessCount:     9,
		FailureCount:     1,
		TotalBytes:       1234,
		TotalCost:        0.001,
		AvgLatency:       50,
		ErrorTypes:       "{}",
		LatencyHistogram: "{}",
	}))

	// Get windows success path
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.RouteMetricsWindow")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.RouteMetricsWindow)
			*out = []*models.RouteMetricsWindow{
				{RouteID: "route-1", WindowStart: windowStart},
				{RouteID: "route-1", WindowStart: windowStart.Add(-5 * time.Minute)},
			}
		}).
		Return(nil).
		Once()

	windows, err := repo.GetRouteMetricsWindows(ctx, "route-1", windowStart.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, windows, 2)
}

func TestRoutingMetricsRepository_GetMetricsWindows_Error(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.GetInstanceMetricsWindows(ctx, "instance-1", time.Now().Add(-time.Hour), 10)
	require.Error(t, err)
}

func TestRoutingMetricsRepository_GlobalWindows_SuccessAndError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)

	// success path
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.GlobalMetricsWindow")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.GlobalMetricsWindow)
			*out = []*models.GlobalMetricsWindow{{TotalMessages: 123}}
		}).
		Return(nil).
		Once()

	windows, err := repo.GetGlobalMetricsWindows(ctx, time.Now().Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, windows, 1)

	// error path
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()

	_, err = repo.GetGlobalMetricsWindows(ctx, time.Now().Add(-time.Hour), 10)
	require.Error(t, err)
}

func TestRoutingMetricsRepository_StoreGlobalAndInstance_Windows_SuccessAndError(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	windowStart := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	// StoreGlobalMetricsWindow success
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil).Once()

		require.NoError(t, repo.StoreGlobalMetricsWindow(ctx, &models.GlobalMetricsWindow{
			WindowStart:     windowStart,
			WindowSize:      5,
			TotalMessages:   100,
			TotalBytes:      1024,
			TotalCost:       0.001,
			ActiveRoutes:    3,
			UniqueInstances: 2,
		}))
	}

	// StoreGlobalMetricsWindow error
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		require.Error(t, repo.StoreGlobalMetricsWindow(ctx, &models.GlobalMetricsWindow{
			WindowStart: windowStart,
		}))
	}

	// StoreInstanceMetricsWindow success + GetInstanceMetricsWindows success
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Create").Return(nil).Once()

		require.NoError(t, repo.StoreInstanceMetricsWindow(ctx, &models.InstanceMetricsWindow{
			InstanceID:    "instance-1",
			WindowStart:   windowStart,
			WindowSize:    5,
			TotalMessages: 42,
			TotalBytes:    2048,
			TotalCost:     0.002,
			HealthChecks:  10,
			Availability:  0.99,
		}))

		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.InstanceMetricsWindow")).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.InstanceMetricsWindow)
				*out = []*models.InstanceMetricsWindow{{InstanceID: "instance-1", WindowStart: windowStart}}
			}).
			Return(nil).
			Once()

		windows, err := repo.GetInstanceMetricsWindows(ctx, "instance-1", windowStart.Add(-time.Hour), 10)
		require.NoError(t, err)
		require.Len(t, windows, 1)
	}

	// StoreInstanceMetricsWindow error
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		require.Error(t, repo.StoreInstanceMetricsWindow(ctx, &models.InstanceMetricsWindow{
			InstanceID:  "instance-1",
			WindowStart: windowStart,
		}))
	}
}

func TestRoutingMetricsRepository_BatchStoreMetrics_CoversBranches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewRoutingMetricsRepository(mockDB, "test-table", logger, nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()

	windowStart := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, repo.BatchStoreMetrics(ctx,
		[]*models.RouteMetricsWindow{{RouteID: "route-1", WindowStart: windowStart}},
		[]*models.InstanceMetricsWindow{{InstanceID: "instance-1", WindowStart: windowStart}},
		&models.GlobalMetricsWindow{WindowStart: windowStart},
	))

	// route batch error path
	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	repo2 := NewRoutingMetricsRepository(mockDB2, "test-table", logger, nil)
	mockDB2.On("WithContext", ctx).Return(mockDB2)
	mockDB2.On("Model", mock.Anything).Return(mockQuery2)
	mockQuery2.On("Create").Return(errors.New("create failed")).Once()

	err := repo2.BatchStoreMetrics(ctx,
		[]*models.RouteMetricsWindow{{RouteID: "route-1", WindowStart: windowStart}},
		nil,
		nil,
	)
	require.Error(t, err)

	// instance batch error path
	mockDB3 := new(mocks.MockDB)
	mockQuery3 := new(mocks.MockQuery)
	repo3 := NewRoutingMetricsRepository(mockDB3, "test-table", logger, nil)
	mockDB3.On("WithContext", ctx).Return(mockDB3)
	mockDB3.On("Model", mock.Anything).Return(mockQuery3)
	mockQuery3.On("Create").Return(errors.New("create failed")).Once()

	err = repo3.BatchStoreMetrics(ctx,
		nil,
		[]*models.InstanceMetricsWindow{{InstanceID: "instance-1", WindowStart: windowStart}},
		nil,
	)
	require.Error(t, err)

	// global window error path
	mockDB4 := new(mocks.MockDB)
	mockQuery4 := new(mocks.MockQuery)
	repo4 := NewRoutingMetricsRepository(mockDB4, "test-table", logger, nil)
	mockDB4.On("WithContext", ctx).Return(mockDB4)
	mockDB4.On("Model", mock.Anything).Return(mockQuery4)
	mockQuery4.On("Create").Return(errors.New("create failed")).Once()

	err = repo4.BatchStoreMetrics(ctx,
		nil,
		nil,
		&models.GlobalMetricsWindow{WindowStart: windowStart},
	)
	require.Error(t, err)
}

func TestRoutingMetricsRepository_CleanupExpiredMetrics_NoOp(t *testing.T) {
	repo := &RoutingMetricsRepository{logger: zap.NewNop()}
	require.NoError(t, repo.CleanupExpiredMetrics(context.Background(), time.Now().Add(-time.Hour)))
}
