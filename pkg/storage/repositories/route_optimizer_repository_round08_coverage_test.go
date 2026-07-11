package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRouteOptimizerRepository_Round08_CRUDAndQueries(t *testing.T) {
	ctx := context.Background()

	t.Run("RecordDeliveryResult success creates model", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.RecordDeliveryResult(ctx, &types.DeliveryResult{
			MessageID:  "m1",
			RouteID:    "https://relay.example/route",
			Success:    true,
			StatusCode: 200,
			Duration:   150 * time.Millisecond,
			BytesSent:  123,
			Cost:       0.01,
			Timestamp:  time.Now().Add(-time.Minute),
		})
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("RecordDeliveryResult create error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.RecordDeliveryResult(ctx, &types.DeliveryResult{MessageID: "m1", RouteID: "r1"})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetRouteResults returns results and supports errors", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", 2).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]*models.RouteDeliveryResult)
				*dest = []*models.RouteDeliveryResult{
					{RouteID: "r1", MessageID: "m1", Success: true, Duration: 100, BytesSent: 1, Cost: 0.1},
				}
			}).Once()

			repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
			results, err := repo.GetRouteResults(ctx, "r1", 2)
			require.NoError(t, err)
			require.Len(t, results, 1)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", 2).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

			repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
			_, err := repo.GetRouteResults(ctx, "r1", 2)
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("GetRecentResults and GetOptimizationDecisions query GSIs", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryResults := new(mocks.MockQuery)
		mockQueryDecisions := new(mocks.MockQuery)

		// GetRecentResults.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQueryResults).Once()
		mockQueryResults.On("Index", "gsi1").Return(mockQueryResults).Once()
		mockQueryResults.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryResults).Twice()
		mockQueryResults.On("OrderBy", "gsi1SK", "DESC").Return(mockQueryResults).Once()
		mockQueryResults.On("Limit", 1).Return(mockQueryResults).Once()
		mockQueryResults.On("All", mock.Anything).Return(nil).Once()

		// GetOptimizationDecisions.
		mockDB.On("Model", mock.Anything).Return(mockQueryDecisions).Once()
		mockQueryDecisions.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDecisions).Twice()
		mockQueryDecisions.On("OrderBy", "SK", "DESC").Return(mockQueryDecisions).Once()
		mockQueryDecisions.On("Limit", 1).Return(mockQueryDecisions).Once()
		mockQueryDecisions.On("All", mock.Anything).Return(nil).Once()

		repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)

		_, err := repo.GetRecentResults(ctx, time.Now().Add(-time.Hour), 1)
		require.NoError(t, err)

		_, err = repo.GetOptimizationDecisions(ctx, time.Now().Add(-time.Hour), 1)
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQueryResults.AssertExpectations(t)
		mockQueryDecisions.AssertExpectations(t)
	})

	t.Run("GetRecentResults and GetOptimizationDecisions map query errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryResults := new(mocks.MockQuery)
		mockQueryDecisions := new(mocks.MockQuery)

		// GetRecentResults error.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQueryResults).Once()
		mockQueryResults.On("Index", "gsi1").Return(mockQueryResults).Once()
		mockQueryResults.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryResults).Twice()
		mockQueryResults.On("OrderBy", "gsi1SK", "DESC").Return(mockQueryResults).Once()
		mockQueryResults.On("Limit", 1).Return(mockQueryResults).Once()
		mockQueryResults.On("All", mock.Anything).Return(assert.AnError).Once()

		// GetOptimizationDecisions error.
		mockDB.On("Model", mock.Anything).Return(mockQueryDecisions).Once()
		mockQueryDecisions.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDecisions).Twice()
		mockQueryDecisions.On("OrderBy", "SK", "DESC").Return(mockQueryDecisions).Once()
		mockQueryDecisions.On("Limit", 1).Return(mockQueryDecisions).Once()
		mockQueryDecisions.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)

		_, err := repo.GetRecentResults(ctx, time.Now().Add(-time.Hour), 1)
		require.Error(t, err)

		_, err = repo.GetOptimizationDecisions(ctx, time.Now().Add(-time.Hour), 1)
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQueryResults.AssertExpectations(t)
		mockQueryDecisions.AssertExpectations(t)
	})

	t.Run("GetRouteMetricsForFederation returns default metrics when empty and computes percentiles when present", func(t *testing.T) {
		t.Run("empty results", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", 1000).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]*models.RouteDeliveryResult)
				*dest = []*models.RouteDeliveryResult{}
			}).Once()

			repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
			metrics, err := repo.GetRouteMetricsForFederation(ctx, "r1")
			require.NoError(t, err)
			require.NotNil(t, metrics)
			assert.Equal(t, int64(0), metrics.TotalMessages)
			assert.NotZero(t, metrics.LastUpdated)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("non-empty results compute counts and latencies", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", 1000).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]*models.RouteDeliveryResult)
				*dest = []*models.RouteDeliveryResult{
					{RouteID: "r1", MessageID: "m1", Success: true, Duration: 100, BytesSent: 10, Cost: 0.01},
					{RouteID: "r1", MessageID: "m2", Success: true, Duration: 200, BytesSent: 20, Cost: 0.02},
					{RouteID: "r1", MessageID: "m3", Success: false, Duration: 999, BytesSent: 30, Cost: 0.03},
				}
			}).Once()

			repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
			metrics, err := repo.GetRouteMetricsForFederation(ctx, "r1")
			require.NoError(t, err)
			require.NotNil(t, metrics)
			assert.Equal(t, int64(3), metrics.TotalMessages)
			assert.Equal(t, int64(2), metrics.SuccessfulCount)
			assert.Equal(t, int64(1), metrics.FailedCount)
			assert.Equal(t, int64(60), metrics.TotalBytes)
			assert.InDelta(t, 0.06, metrics.TotalCost, 0.00001)
			assert.NotZero(t, metrics.AvgLatency)
			assert.NotZero(t, metrics.P95Latency)
			assert.NotZero(t, metrics.P99Latency)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})
}

func TestRouteOptimizerRepository_Round08_MetricsInRangeAndHelpers(t *testing.T) {
	ctx := context.Background()

	t.Run("StoreOptimizationDecision persists model and maps create errors", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Create").Return(nil).Once()

			repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
			err := repo.StoreOptimizationDecision(ctx, []*types.Route{{ID: "r1"}, {ID: "r2"}}, 123)
			require.NoError(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("create error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Create").Return(assert.AnError).Once()

			repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
			err := repo.StoreOptimizationDecision(ctx, []*types.Route{{ID: "r1"}}, 123)
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("CleanupExpiredResults is a no-op", func(t *testing.T) {
		repo := NewRouteOptimizerRepository(new(mocks.MockDB), "table", zap.NewNop(), nil)
		require.NoError(t, repo.CleanupExpiredResults(ctx, time.Now()))
	})

	t.Run("GetRouteMetrics and GetRoutePerformance delegate to federation helpers", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Times(4)
		mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Twice()
		mockQuery.On("Limit", 1000).Return(mockQuery).Twice()
		mockQuery.On("All", mock.Anything).Return(nil).Twice().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.RouteDeliveryResult)
			*dest = []*models.RouteDeliveryResult{
				{RouteID: "r1", MessageID: "m1", Success: true, Duration: 100, BytesSent: 1, Cost: 0.1},
			}
		})

		repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)

		metrics, err := repo.GetRouteMetrics(ctx, "r1")
		require.NoError(t, err)
		require.NotNil(t, metrics)

		perf, err := repo.GetRoutePerformance(ctx, "r1")
		require.NoError(t, err)
		require.NotNil(t, perf)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetRoutePerformanceData returns raw results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
		mockQuery.On("Limit", 1000).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.RouteDeliveryResult)
			*dest = []*models.RouteDeliveryResult{{RouteID: "r1", MessageID: "m1"}}
		}).Once()

		repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
		raw, err := repo.GetRoutePerformanceData(ctx, "r1")
		require.NoError(t, err)
		require.NotNil(t, raw)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetMetricsInRange supports route-specific and global GSI paths", func(t *testing.T) {
		t.Run("route-specific", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Times(3)
			mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", 10).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]*models.RouteDeliveryResult)
				*dest = []*models.RouteDeliveryResult{
					{RouteID: "foo@relay.example", MessageID: "m1", Success: true, StatusCode: 200, Duration: 1, BytesSent: 1, Cost: 0.01, Timestamp: time.Now()},
					{RouteID: "foo@relay.example", MessageID: "m2", Success: false, StatusCode: 500, Duration: 2, BytesSent: 2, Cost: 0.02, Timestamp: time.Now()},
					{RouteID: "foo@relay.example", MessageID: "m3", Success: false, StatusCode: 404, Duration: 3, BytesSent: 3, Cost: 0.03, Timestamp: time.Now()},
					{RouteID: "foo@relay.example", MessageID: "m4", Success: false, StatusCode: 0, Duration: 4, BytesSent: 4, Cost: 0.04, Timestamp: time.Now()},
				}
			}).Once()

			repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
			items, err := repo.GetMetricsInRange(ctx, "foo@relay.example", time.Now().Add(-time.Hour), time.Now(), 10)
			require.NoError(t, err)
			require.Len(t, items, 4)
			assert.Equal(t, "relay.example", items[0].InstanceID)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("global GSI", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Times(3)
			mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", 5).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]*models.RouteDeliveryResult)
				*dest = []*models.RouteDeliveryResult{
					{RouteID: "https://relay.example/route", MessageID: "m1", Success: true, StatusCode: 200, Duration: 1, BytesSent: 1, Cost: 0.01, Timestamp: time.Now()},
				}
			}).Once()

			repo := NewRouteOptimizerRepository(mockDB, "table", zap.NewNop(), nil)
			items, err := repo.GetMetricsInRange(ctx, "", time.Now().Add(-time.Hour), time.Now(), 5)
			require.NoError(t, err)
			require.Len(t, items, 1)
			assert.Equal(t, "relay.example", items[0].InstanceID)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("helpers cover parsing and attempt estimation", func(t *testing.T) {
		repo := NewRouteOptimizerRepository(new(mocks.MockDB), "table", zap.NewNop(), nil)

		assert.Equal(t, "example.com", repo.extractInstanceFromRoute("id@example.com"))
		assert.Equal(t, "relay.example", repo.extractInstanceFromRoute("https://relay.example/route"))
		assert.Empty(t, repo.extractInstanceFromRoute("http://%zz"))
		assert.Empty(t, repo.extractInstanceFromRoute("nohost"))

		assert.Equal(t, 1, repo.estimateAttempts(true, 500))
		assert.Equal(t, 3, repo.estimateAttempts(false, 500))
		assert.Equal(t, 2, repo.estimateAttempts(false, 404))
		assert.Equal(t, 3, repo.estimateAttempts(false, 0))
		assert.Equal(t, 1, repo.estimateAttempts(false, 123))
	})
}
