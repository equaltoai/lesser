package routing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestSmartRouteOptimizer_getRoutePerformance_CacheAndFallbacks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	fakeRepo := NewFakeRepositoryInterface()
	fakeRepo.GetRouteMetricsFunc = func(_ context.Context, _ string) (*types.RouteMetrics, error) {
		return &types.RouteMetrics{
			TotalMessages:   10,
			SuccessfulCount: 8,
			FailedCount:     2,
			AvgLatency:      100 * time.Millisecond,
			P95Latency:      200 * time.Millisecond,
			P99Latency:      300 * time.Millisecond,
			TotalBytes:      1000,
			TotalCost:       2.0,
			LastUpdated:     time.Now(),
		}, nil
	}

	fakeRepo.GetRoutePerformanceFunc = func(_ context.Context, _ string) (interface{}, error) {
		return nil, errors.New("repo unavailable")
	}

	optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

	perf, err := optimizer.getRoutePerformance(ctx, "route-1")
	require.NoError(t, err)
	assert.Equal(t, trendStable, perf.TrendDirection)
	assert.InDelta(t, 0.8, perf.SuccessRate, 0.0001)
	assert.InDelta(t, 0.002, perf.AvgCostPerByte, 0.0001)
	assert.Equal(t, 1, fakeRepo.GetCallCount("GetRouteMetrics"))
	assert.Equal(t, 1, fakeRepo.GetCallCount("GetRoutePerformance"))

	// Cache hit should avoid repo calls.
	perfAgain, err := optimizer.getRoutePerformance(ctx, "route-1")
	require.NoError(t, err)
	assert.Same(t, perf, perfAgain)
	assert.Equal(t, 1, fakeRepo.GetCallCount("GetRouteMetrics"))
	assert.Equal(t, 1, fakeRepo.GetCallCount("GetRoutePerformance"))

	// Force cache expiry and return an unsupported raw performance type.
	fakeRepo.GetRoutePerformanceFunc = func(_ context.Context, _ string) (interface{}, error) {
		return []string{"not a delivery result"}, nil
	}
	perf.LastUpdated = time.Now().Add(-10 * time.Minute)

	perfExpired, err := optimizer.getRoutePerformance(ctx, "route-1")
	require.NoError(t, err)
	assert.NotSame(t, perf, perfExpired)
	assert.Equal(t, trendStable, perfExpired.TrendDirection)
	assert.Equal(t, 2, fakeRepo.GetCallCount("GetRouteMetrics"))
	assert.Equal(t, 2, fakeRepo.GetCallCount("GetRoutePerformance"))
}

func TestSmartRouteOptimizer_calculateTrend(t *testing.T) {
	logger := zaptest.NewLogger(t)
	optimizer := NewSmartRouteOptimizerFromInterface(NewFakeRepositoryInterface(), logger, nil)

	t.Run("stable_when_insufficient_samples", func(t *testing.T) {
		perf := &routePerformance{Samples: make([]performanceSample, 19)}
		assert.Equal(t, trendStable, optimizer.calculateTrend(perf))
	})

	t.Run("improving_when_latency_improves", func(t *testing.T) {
		samples := make([]performanceSample, 20)
		for i := range samples {
			samples[i].Success = true
			if i < 10 {
				samples[i].Latency = 200 * time.Millisecond
			} else {
				samples[i].Latency = 100 * time.Millisecond
			}
		}

		perf := &routePerformance{Samples: samples}
		assert.Equal(t, trendImproving, optimizer.calculateTrend(perf))
	})

	t.Run("degrading_when_latency_degrades", func(t *testing.T) {
		samples := make([]performanceSample, 20)
		for i := range samples {
			samples[i].Success = true
			if i < 10 {
				samples[i].Latency = 100 * time.Millisecond
			} else {
				samples[i].Latency = 200 * time.Millisecond
			}
		}

		perf := &routePerformance{Samples: samples}
		assert.Equal(t, trendDegrading, optimizer.calculateTrend(perf))
	})

	t.Run("stable_when_mixed_signals", func(t *testing.T) {
		samples := make([]performanceSample, 20)
		for i := range samples {
			if i < 10 {
				samples[i].Success = false
				samples[i].Latency = 0
			} else {
				samples[i].Success = true
				samples[i].Latency = 100 * time.Millisecond
			}
		}

		perf := &routePerformance{Samples: samples}
		assert.Equal(t, trendStable, optimizer.calculateTrend(perf))
	})
}

func TestSmartRouteOptimizer_updateAggregates(t *testing.T) {
	logger := zaptest.NewLogger(t)
	optimizer := NewSmartRouteOptimizerFromInterface(NewFakeRepositoryInterface(), logger, nil)

	t.Run("no_samples_noop", func(t *testing.T) {
		perf := &routePerformance{
			Samples:        []performanceSample{},
			SuccessRate:    0.123,
			AvgLatency:     99 * time.Millisecond,
			AvgCostPerByte: 0.456,
		}

		optimizer.updateAggregates(perf)

		assert.InDelta(t, 0.123, perf.SuccessRate, 0.0001)
		assert.Equal(t, 99*time.Millisecond, perf.AvgLatency)
		assert.InDelta(t, 0.456, perf.AvgCostPerByte, 0.0001)
	})

	t.Run("success_latency_and_cost_aggregates", func(t *testing.T) {
		perf := &routePerformance{
			Samples: []performanceSample{
				{Success: true, Latency: 100 * time.Millisecond, BytesSent: 100, Cost: 0.10},
				{Success: false, Latency: 999 * time.Millisecond, BytesSent: 50, Cost: 0.05},
				{Success: true, Latency: 200 * time.Millisecond, BytesSent: 50, Cost: 0.05},
			},
		}

		optimizer.updateAggregates(perf)

		assert.InDelta(t, 2.0/3.0, perf.SuccessRate, 0.0001)
		assert.Equal(t, 150*time.Millisecond, perf.AvgLatency) // successes only
		assert.InDelta(t, 0.2/200.0, perf.AvgCostPerByte, 0.0001)
	})

	t.Run("no_successes_does_not_update_avg_latency", func(t *testing.T) {
		perf := &routePerformance{
			AvgLatency: 123 * time.Millisecond,
			Samples: []performanceSample{
				{Success: false, BytesSent: 100, Cost: 0.10},
				{Success: false, BytesSent: 100, Cost: 0.10},
			},
		}

		optimizer.updateAggregates(perf)

		assert.InDelta(t, 0.0, perf.SuccessRate, 0.0001)
		assert.Equal(t, 123*time.Millisecond, perf.AvgLatency)
		assert.InDelta(t, 0.2/200.0, perf.AvgCostPerByte, 0.0001)
	})

	t.Run("zero_total_bytes_does_not_update_avg_cost", func(t *testing.T) {
		perf := &routePerformance{
			AvgCostPerByte: 0.999,
			Samples: []performanceSample{
				{Success: true, Latency: 100 * time.Millisecond, BytesSent: 0, Cost: 0.10},
			},
		}

		optimizer.updateAggregates(perf)

		assert.InDelta(t, 1.0, perf.SuccessRate, 0.0001)
		assert.Equal(t, 100*time.Millisecond, perf.AvgLatency)
		assert.InDelta(t, 0.999, perf.AvgCostPerByte, 0.0001)
	})
}

func TestSmartRouteOptimizer_RecordDeliveryResult_ErrorAndCacheUpdates(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("wraps_repo_error_and_does_not_update_models", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		fakeRepo.RecordDeliveryResultFunc = func(_ context.Context, _ *types.DeliveryResult) error {
			return errors.New("write failed")
		}
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		err := optimizer.RecordDeliveryResult(ctx, &types.DeliveryResult{
			RouteID:   "route-err",
			Success:   true,
			Duration:  123 * time.Millisecond,
			BytesSent: 100,
			Cost:      0.01,
			Timestamp: time.Now(),
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRecordDeliveryResultFailed)

		optimizer.latencyModel.mu.RLock()
		_, latencyExists := optimizer.latencyModel.predictions["route-err"]
		optimizer.latencyModel.mu.RUnlock()
		assert.False(t, latencyExists)

		optimizer.costModel.mu.RLock()
		_, costExists := optimizer.costModel.costs["route-err"]
		optimizer.costModel.mu.RUnlock()
		assert.False(t, costExists)
	})

	t.Run("updates_predictions_and_cached_performance", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, &OptimizerConfig{
			HistoryWindow:        5 * time.Second,
			MinSamplesRequired:   1,
			LatencyWeight:        0.4,
			ReliabilityWeight:    0.4,
			CostWeight:           0.2,
			MaxAcceptableLatency: 2 * time.Second,
			MinAcceptableSuccess: 0.95,
			MaxCostPerMB:         0.10,
			AdaptationRate:       0.1,
		})

		routeID := "route-cache"
		optimizer.perfCache.Store(routeID, &routePerformance{
			RouteID:     routeID,
			LastUpdated: time.Now(),
			Samples: []performanceSample{
				{Timestamp: time.Now().Add(-1 * time.Hour), Success: true, Latency: 1 * time.Millisecond, BytesSent: 10, Cost: 0.001},
			},
		})

		ts := time.Now().UTC()
		err := optimizer.RecordDeliveryResult(ctx, &types.DeliveryResult{
			RouteID:   routeID,
			Success:   true,
			Duration:  100 * time.Millisecond,
			BytesSent: 100,
			Cost:      0.10,
			Timestamp: ts,
		})
		require.NoError(t, err)

		cached, ok := optimizer.perfCache.Load(routeID)
		require.True(t, ok)
		rp := cached.(*routePerformance)

		// Old sample outside window should be dropped.
		require.Len(t, rp.Samples, 1)
		assert.Equal(t, ts, rp.Samples[0].Timestamp)
		assert.InDelta(t, 1.0, rp.SuccessRate, 0.0001)

		// Second write triggers smoothing / moving average branches.
		err = optimizer.RecordDeliveryResult(ctx, &types.DeliveryResult{
			RouteID:   routeID,
			Success:   true,
			Duration:  200 * time.Millisecond,
			BytesSent: 100,
			Cost:      0.20,
			Timestamp: ts.Add(1 * time.Second),
		})
		require.NoError(t, err)

		optimizer.latencyModel.mu.RLock()
		latencyPrediction := optimizer.latencyModel.predictions[routeID]
		optimizer.latencyModel.mu.RUnlock()
		assert.InDelta(t, 130.0, latencyPrediction, 0.0001) // 0.3*200 + 0.7*100

		optimizer.costModel.mu.RLock()
		costs := optimizer.costModel.costs[routeID]
		optimizer.costModel.mu.RUnlock()
		assert.NotNil(t, costs)

		hour := ts.Hour()
		assert.InDelta(t, 0.0011, costs.hourly[hour], 0.0000001) // 0.9*0.001 + 0.1*0.002
	})

	t.Run("getRoutePerformance_converts_delivery_results", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		fakeRepo.GetRouteMetricsFunc = func(_ context.Context, _ string) (*types.RouteMetrics, error) {
			return &types.RouteMetrics{
				TotalMessages:   20,
				SuccessfulCount: 20,
				FailedCount:     0,
				AvgLatency:      100 * time.Millisecond,
				P95Latency:      200 * time.Millisecond,
				P99Latency:      300 * time.Millisecond,
				TotalBytes:      1000,
				TotalCost:       2.0,
				LastUpdated:     time.Now(),
			}, nil
		}
		fakeRepo.GetRoutePerformanceFunc = func(_ context.Context, _ string) (interface{}, error) {
			now := time.Now()
			results := make([]*models.RouteDeliveryResult, 20)
			for i := range results {
				results[i] = &models.RouteDeliveryResult{
					Timestamp: now.Add(time.Duration(-i) * time.Second),
					Duration:  100,
					Success:   true,
					BytesSent: 100,
					Cost:      0.1,
				}
			}
			return results, nil
		}

		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		perf, err := optimizer.getRoutePerformance(ctx, "route-delivery-results")
		require.NoError(t, err)
		require.Len(t, perf.Samples, 20)
		assert.NotEmpty(t, perf.TrendDirection)
	})
}
