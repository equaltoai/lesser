package routing

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// FakeRepositoryInterface implements RepositoryInterface for testing
type FakeRepositoryInterface struct {
	mu sync.Mutex

	// Function hooks for customizing behavior
	RecordDeliveryResultFunc      func(ctx context.Context, result *types.DeliveryResult) error
	GetRouteMetricsFunc           func(ctx context.Context, routeID string) (*types.RouteMetrics, error)
	GetRoutePerformanceFunc       func(ctx context.Context, routeID string) (interface{}, error)
	StoreOptimizationDecisionFunc func(ctx context.Context, routes []*types.Route, messageSize int64) error

	// Call counters for verification
	CallCounts map[string]int
}

func NewFakeRepositoryInterface() *FakeRepositoryInterface {
	return &FakeRepositoryInterface{
		CallCounts: make(map[string]int),
	}
}

func (f *FakeRepositoryInterface) incrementCall(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CallCounts[method]++
}

func (f *FakeRepositoryInterface) GetCallCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.CallCounts[method]
}

func (f *FakeRepositoryInterface) RecordDeliveryResult(ctx context.Context, result *types.DeliveryResult) error {
	f.incrementCall("RecordDeliveryResult")
	if f.RecordDeliveryResultFunc != nil {
		return f.RecordDeliveryResultFunc(ctx, result)
	}
	return nil
}

func (f *FakeRepositoryInterface) GetRouteMetrics(ctx context.Context, routeID string) (*types.RouteMetrics, error) {
	f.incrementCall("GetRouteMetrics")
	if f.GetRouteMetricsFunc != nil {
		return f.GetRouteMetricsFunc(ctx, routeID)
	}
	// Return default metrics
	return &types.RouteMetrics{
		TotalMessages:   100,
		SuccessfulCount: 95,
		FailedCount:     5,
		AvgLatency:      100 * time.Millisecond,
		P95Latency:      200 * time.Millisecond,
		P99Latency:      500 * time.Millisecond,
		TotalBytes:      1024 * 1024,
		TotalCost:       0.10,
		LastUpdated:     time.Now(),
	}, nil
}

func (f *FakeRepositoryInterface) GetRoutePerformance(ctx context.Context, routeID string) (interface{}, error) {
	f.incrementCall("GetRoutePerformance")
	if f.GetRoutePerformanceFunc != nil {
		return f.GetRoutePerformanceFunc(ctx, routeID)
	}
	return nil, nil
}

func (f *FakeRepositoryInterface) StoreOptimizationDecision(ctx context.Context, routes []*types.Route, messageSize int64) error {
	f.incrementCall("StoreOptimizationDecision")
	if f.StoreOptimizationDecisionFunc != nil {
		return f.StoreOptimizationDecisionFunc(ctx, routes, messageSize)
	}
	return nil
}

// Helper to create a test route
func createTestRoute(id string, latency time.Duration, successRate float64, costPerByte float64) *types.Route {
	endpoint, _ := url.Parse("https://example.com/inbox")
	return &types.Route{
		ID:            id,
		InstanceID:    "instance-" + id,
		Domain:        "example.com",
		Endpoint:      endpoint,
		Priority:      1,
		Latency:       latency,
		SuccessRate:   successRate,
		CostPerByte:   costPerByte,
		CircuitStatus: types.CircuitClosed,
	}
}

// === NewSmartRouteOptimizerFromInterface Tests ===

func TestNewSmartRouteOptimizerFromInterface(t *testing.T) {
	logger := zaptest.NewLogger(t)
	fakeRepo := NewFakeRepositoryInterface()

	t.Run("creates_optimizer_with_default_config", func(t *testing.T) {
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		require.NotNil(t, optimizer)
		assert.NotNil(t, optimizer.config)
		assert.Equal(t, 0.4, optimizer.config.LatencyWeight)
		assert.Equal(t, 0.4, optimizer.config.ReliabilityWeight)
		assert.Equal(t, 0.2, optimizer.config.CostWeight)
		assert.Equal(t, 2*time.Second, optimizer.config.MaxAcceptableLatency)
		assert.Equal(t, 0.95, optimizer.config.MinAcceptableSuccess)
	})

	t.Run("creates_optimizer_with_custom_config", func(t *testing.T) {
		customConfig := &OptimizerConfig{
			LatencyWeight:        0.5,
			ReliabilityWeight:    0.3,
			CostWeight:           0.2,
			MaxAcceptableLatency: 5 * time.Second,
			MinAcceptableSuccess: 0.90,
			MaxCostPerMB:         0.50,
			HistoryWindow:        12 * time.Hour,
			MinSamplesRequired:   20,
			AdaptationRate:       0.2,
		}

		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, customConfig)

		require.NotNil(t, optimizer)
		assert.Equal(t, 0.5, optimizer.config.LatencyWeight)
		assert.Equal(t, 0.3, optimizer.config.ReliabilityWeight)
		assert.Equal(t, 5*time.Second, optimizer.config.MaxAcceptableLatency)
		assert.Equal(t, 20, optimizer.config.MinSamplesRequired)
	})
}

// === OptimizeRoutes Tests ===

func TestOptimizeRoutes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("empty_routes_returns_empty", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		result, err := optimizer.OptimizeRoutes(ctx, []*types.Route{}, 1024)

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("single_route_returns_same_route", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001)

		result, err := optimizer.OptimizeRoutes(ctx, []*types.Route{route}, 1024)

		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "route-1", result[0].ID)
		assert.Equal(t, 1, result[0].Priority) // Priority updated to 1
	})

	t.Run("ranks_routes_by_score", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		// Route with better latency and success rate should rank higher
		goodRoute := createTestRoute("good-route", 50*time.Millisecond, 0.99, 0.0001)
		badRoute := createTestRoute("bad-route", 1500*time.Millisecond, 0.80, 0.001)

		result, err := optimizer.OptimizeRoutes(ctx, []*types.Route{badRoute, goodRoute}, 1024)

		require.NoError(t, err)
		require.Len(t, result, 2)
		// Good route should be ranked first
		assert.Equal(t, "good-route", result[0].ID)
		assert.Equal(t, "bad-route", result[1].ID)
	})

	t.Run("updates_priority_based_on_ranking", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		routes := []*types.Route{
			createTestRoute("route-1", 100*time.Millisecond, 0.95, 0.0001),
			createTestRoute("route-2", 50*time.Millisecond, 0.99, 0.0001),
			createTestRoute("route-3", 200*time.Millisecond, 0.90, 0.0001),
		}

		result, err := optimizer.OptimizeRoutes(ctx, routes, 1024)

		require.NoError(t, err)
		require.Len(t, result, 3)
		// Verify priorities are set correctly (1-indexed)
		for i, route := range result {
			assert.Equal(t, i+1, route.Priority, "route %s should have priority %d", route.ID, i+1)
		}
	})

	t.Run("stores_optimization_decision", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001)

		_, err := optimizer.OptimizeRoutes(ctx, []*types.Route{route}, 1024)

		require.NoError(t, err)
		assert.Equal(t, 1, fakeRepo.GetCallCount("StoreOptimizationDecision"))
	})
}

// === PredictLatency Tests ===

func TestPredictLatency(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("uses_route_latency_when_no_cache", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001)

		// Predict latency for 1KB message
		predicted := optimizer.PredictLatency(route, 1024)

		// Should be based on route.Latency (100ms) with hourly factor
		// At minimum, it should be close to the route's latency
		assert.Greater(t, predicted, 50*time.Millisecond)
		assert.Less(t, predicted, 200*time.Millisecond)
	})

	t.Run("scales_with_message_size", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001)

		smallMessage := optimizer.PredictLatency(route, 1024)    // 1KB
		largeMessage := optimizer.PredictLatency(route, 10*1024) // 10KB

		// Larger messages should have higher latency
		assert.Greater(t, largeMessage, smallMessage)
	})

	t.Run("uses_cached_prediction_when_available", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001)

		// Pre-populate the latency model cache
		optimizer.latencyModel.mu.Lock()
		optimizer.latencyModel.predictions[route.ID] = 200.0 // 200ms
		optimizer.latencyModel.mu.Unlock()

		predicted := optimizer.PredictLatency(route, 1024)

		// Should use cached 200ms as base instead of route's 100ms
		assert.Greater(t, predicted, 100*time.Millisecond)
	})
}

// === EstimateCost Tests ===

func TestEstimateCost(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("uses_route_cost_when_no_cache", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001) // $0.0001/byte

		cost := optimizer.EstimateCost(route, 1024) // 1KB

		// 1024 bytes * $0.0001/byte = $0.1024
		assert.InDelta(t, 0.1024, cost, 0.001)
	})

	t.Run("scales_linearly_with_message_size", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001)

		smallCost := optimizer.EstimateCost(route, 1024)
		largeCost := optimizer.EstimateCost(route, 2048)

		// Cost should double when message size doubles
		assert.InDelta(t, smallCost*2, largeCost, 0.001)
	})

	t.Run("uses_hourly_costs_when_cached", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001)

		// Pre-populate hourly costs
		currentHour := time.Now().Hour()
		optimizer.costModel.mu.Lock()
		optimizer.costModel.costs[route.ID] = &timeCosts{}
		optimizer.costModel.costs[route.ID].hourly[currentHour] = 0.0002 // $0.0002/byte
		optimizer.costModel.mu.Unlock()

		cost := optimizer.EstimateCost(route, 1024)

		// Should use cached $0.0002/byte instead of route's $0.0001/byte
		assert.InDelta(t, 0.2048, cost, 0.001)
	})

	t.Run("falls_back_to_route_cost_when_hourly_is_zero", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.99, 0.0001)

		// Pre-populate costs but with zero for current hour
		optimizer.costModel.mu.Lock()
		optimizer.costModel.costs[route.ID] = &timeCosts{} // All zeros
		optimizer.costModel.mu.Unlock()

		cost := optimizer.EstimateCost(route, 1024)

		// Should fall back to route's $0.0001/byte
		assert.InDelta(t, 0.1024, cost, 0.001)
	})
}

// === scoreRoute Tests ===

func TestScoreRoute(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("uses_route_values_when_no_perf_data", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.95, 0.0001)

		score := optimizer.scoreRoute(route, nil, 1024)

		// Latency score: 1.0 - min(100ms/2000ms, 1.0) = 0.95
		// Reliability score: 0.95
		// Cost score: 0.0 (cost calculation with default MaxCostPerMB)
		// Total: 0.95*0.4 + 0.95*0.4 + 0.0*0.2 = 0.76
		assert.Greater(t, score.total, 0.7)
		assert.Greater(t, score.latency, 0.9)
		assert.Equal(t, 0.95, score.reliability)
	})

	t.Run("uses_perf_data_when_sufficient_samples", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.95, 0.0001)

		// Create perf data with sufficient samples
		perf := &routePerformance{
			RouteID:        route.ID,
			Samples:        make([]performanceSample, 20), // More than MinSamplesRequired (10)
			AvgLatency:     50 * time.Millisecond,
			SuccessRate:    0.98,
			AvgCostPerByte: 0.00005,
			TrendDirection: trendStable,
		}

		score := optimizer.scoreRoute(route, perf, 1024)

		// Should use perf data (better than route static values)
		assert.Equal(t, 0.98, score.reliability)
		assert.Greater(t, score.latency, 0.95) // Better latency from perf
	})

	t.Run("applies_trend_boost_for_improving", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.95, 0.0001)

		stablePerf := &routePerformance{
			Samples:        make([]performanceSample, 20),
			AvgLatency:     100 * time.Millisecond,
			SuccessRate:    0.90,
			TrendDirection: trendStable,
		}
		improvingPerf := &routePerformance{
			Samples:        make([]performanceSample, 20),
			AvgLatency:     100 * time.Millisecond,
			SuccessRate:    0.90,
			TrendDirection: trendImproving,
		}

		stableScore := optimizer.scoreRoute(route, stablePerf, 1024)
		improvingScore := optimizer.scoreRoute(route, improvingPerf, 1024)

		// Improving trend should boost score (1.1x on latency and reliability)
		assert.Greater(t, improvingScore.total, stableScore.total)
	})

	t.Run("applies_trend_penalty_for_degrading", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)
		route := createTestRoute("route-1", 100*time.Millisecond, 0.95, 0.0001)

		stablePerf := &routePerformance{
			Samples:        make([]performanceSample, 20),
			AvgLatency:     100 * time.Millisecond,
			SuccessRate:    0.90,
			TrendDirection: trendStable,
		}
		degradingPerf := &routePerformance{
			Samples:        make([]performanceSample, 20),
			AvgLatency:     100 * time.Millisecond,
			SuccessRate:    0.90,
			TrendDirection: trendDegrading,
		}

		stableScore := optimizer.scoreRoute(route, stablePerf, 1024)
		degradingScore := optimizer.scoreRoute(route, degradingPerf, 1024)

		// Degrading trend should reduce score (0.9x on latency and reliability)
		assert.Less(t, degradingScore.total, stableScore.total)
	})

	t.Run("applies_circuit_breaker_penalty_open", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		closedRoute := createTestRoute("closed-route", 100*time.Millisecond, 0.95, 0.0001)
		closedRoute.CircuitStatus = types.CircuitClosed

		openRoute := createTestRoute("open-route", 100*time.Millisecond, 0.95, 0.0001)
		openRoute.CircuitStatus = types.CircuitOpen

		closedScore := optimizer.scoreRoute(closedRoute, nil, 1024)
		openScore := optimizer.scoreRoute(openRoute, nil, 1024)

		// Open circuit applies 0.1x penalty to reliability
		assert.Less(t, openScore.reliability, closedScore.reliability*0.2)
	})

	t.Run("applies_circuit_breaker_penalty_half_open", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		closedRoute := createTestRoute("closed-route", 100*time.Millisecond, 0.95, 0.0001)
		closedRoute.CircuitStatus = types.CircuitClosed

		halfOpenRoute := createTestRoute("half-open-route", 100*time.Millisecond, 0.95, 0.0001)
		halfOpenRoute.CircuitStatus = types.CircuitHalfOpen

		closedScore := optimizer.scoreRoute(closedRoute, nil, 1024)
		halfOpenScore := optimizer.scoreRoute(halfOpenRoute, nil, 1024)

		// Half-open circuit applies 0.5x penalty to reliability
		assert.InDelta(t, closedScore.reliability*0.5, halfOpenScore.reliability, 0.01)
	})

	t.Run("weighted_total_calculation", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		config := &OptimizerConfig{
			LatencyWeight:        0.4,
			ReliabilityWeight:    0.4,
			CostWeight:           0.2,
			MaxAcceptableLatency: 2 * time.Second,
			MinAcceptableSuccess: 0.95,
			MaxCostPerMB:         0.10,
			MinSamplesRequired:   10,
		}
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, config)
		route := createTestRoute("route-1", 1*time.Second, 0.80, 0.00005) // 50% latency, 80% reliability

		score := optimizer.scoreRoute(route, nil, 1024)

		// Verify weighted total = latency*0.4 + reliability*0.4 + cost*0.2
		expectedTotal := score.latency*0.4 + score.reliability*0.4 + score.cost*0.2
		assert.InDelta(t, expectedTotal, score.total, 0.001)
	})
}

// === ScoreRoute Ordering Tests (Table-Driven) ===

func TestScoreRoute_Ordering(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name           string
		routeA         *types.Route
		routeB         *types.Route
		expectedWinner string // ID of route that should score higher
	}{
		{
			name:           "lower_latency_wins",
			routeA:         createTestRoute("fast", 50*time.Millisecond, 0.95, 0.0001),
			routeB:         createTestRoute("slow", 500*time.Millisecond, 0.95, 0.0001),
			expectedWinner: "fast",
		},
		{
			name:           "higher_reliability_wins",
			routeA:         createTestRoute("reliable", 100*time.Millisecond, 0.99, 0.0001),
			routeB:         createTestRoute("unreliable", 100*time.Millisecond, 0.70, 0.0001),
			expectedWinner: "reliable",
		},
		{
			name:           "lower_cost_wins",
			routeA:         createTestRoute("cheap", 100*time.Millisecond, 0.95, 0.00001),
			routeB:         createTestRoute("expensive", 100*time.Millisecond, 0.95, 0.01),
			expectedWinner: "cheap",
		},
		{
			name: "closed_circuit_beats_open",
			routeA: func() *types.Route {
				r := createTestRoute("closed", 100*time.Millisecond, 0.95, 0.0001)
				r.CircuitStatus = types.CircuitClosed
				return r
			}(),
			routeB: func() *types.Route {
				r := createTestRoute("open", 100*time.Millisecond, 0.95, 0.0001)
				r.CircuitStatus = types.CircuitOpen
				return r
			}(),
			expectedWinner: "closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRepo := NewFakeRepositoryInterface()
			optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

			scoreA := optimizer.scoreRoute(tt.routeA, nil, 1024)
			scoreB := optimizer.scoreRoute(tt.routeB, nil, 1024)

			if tt.expectedWinner == tt.routeA.ID {
				assert.Greater(t, scoreA.total, scoreB.total, "%s should beat %s", tt.routeA.ID, tt.routeB.ID)
			} else {
				assert.Greater(t, scoreB.total, scoreA.total, "%s should beat %s", tt.routeB.ID, tt.routeA.ID)
			}
		})
	}
}

// === RecordDeliveryResult Tests ===

func TestRecordDeliveryResult(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("calls_repository", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		result := &types.DeliveryResult{
			RouteID:   "route-1",
			Success:   true,
			Duration:  100 * time.Millisecond,
			BytesSent: 1024,
			Cost:      0.001,
			Timestamp: time.Now(),
		}

		err := optimizer.RecordDeliveryResult(ctx, result)

		require.NoError(t, err)
		assert.Equal(t, 1, fakeRepo.GetCallCount("RecordDeliveryResult"))
	})

	t.Run("updates_latency_predictions", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		result := &types.DeliveryResult{
			RouteID:   "route-1",
			Success:   true,
			Duration:  150 * time.Millisecond,
			BytesSent: 1024,
			Cost:      0.001,
			Timestamp: time.Now(),
		}

		err := optimizer.RecordDeliveryResult(ctx, result)

		require.NoError(t, err)

		// Verify prediction was updated
		optimizer.latencyModel.mu.RLock()
		prediction, exists := optimizer.latencyModel.predictions["route-1"]
		optimizer.latencyModel.mu.RUnlock()

		assert.True(t, exists)
		assert.Greater(t, prediction, 0.0)
	})

	t.Run("updates_cost_model", func(t *testing.T) {
		fakeRepo := NewFakeRepositoryInterface()
		optimizer := NewSmartRouteOptimizerFromInterface(fakeRepo, logger, nil)

		result := &types.DeliveryResult{
			RouteID:   "route-1",
			Success:   true,
			Duration:  100 * time.Millisecond,
			BytesSent: 1024,
			Cost:      0.10,
			Timestamp: time.Now(),
		}

		err := optimizer.RecordDeliveryResult(ctx, result)

		require.NoError(t, err)

		// Verify cost model was updated
		optimizer.costModel.mu.RLock()
		costs, exists := optimizer.costModel.costs["route-1"]
		optimizer.costModel.mu.RUnlock()

		assert.True(t, exists)
		currentHour := result.Timestamp.Hour()
		assert.Greater(t, costs.hourly[currentHour], 0.0)
	})
}
