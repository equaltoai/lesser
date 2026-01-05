package routing

import (
	"net/url"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// === Balance Tests ===

func TestBalance_EmptyRoutes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)

	result := lb.Balance([]*types.Route{}, 100)

	assert.Empty(t, result)
}

func TestBalance_RoundRobin(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)
	lb.SetAlgorithm(AlgorithmRoundRobin)

	routes := []*types.Route{
		createLBTestRoute("route-1"),
		createLBTestRoute("route-2"),
		createLBTestRoute("route-3"),
	}

	t.Run("distributes_evenly", func(t *testing.T) {
		result := lb.Balance(routes, 9)

		// 9 items across 3 routes = 3 each
		assert.Equal(t, 3, result["route-1"])
		assert.Equal(t, 3, result["route-2"])
		assert.Equal(t, 3, result["route-3"])
	})

	t.Run("handles_uneven_load", func(t *testing.T) {
		result := lb.Balance(routes, 10)

		// Total should equal load
		total := 0
		for _, count := range result {
			total += count
		}
		assert.Equal(t, 10, total)
	})

	t.Run("total_equals_load_invariant", func(t *testing.T) {
		for load := 1; load <= 100; load++ {
			result := lb.Balance(routes, load)
			total := 0
			for _, count := range result {
				total += count
			}
			assert.Equal(t, load, total, "load=%d should distribute exactly", load)
		}
	})
}

func TestBalance_LeastConnections(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)
	lb.SetAlgorithm(AlgorithmLeastConnections)

	routes := []*types.Route{
		createLBTestRoute("route-1"),
		createLBTestRoute("route-2"),
		createLBTestRoute("route-3"),
	}

	t.Run("favors_less_connected_routes", func(t *testing.T) {
		// Preload some connections
		lb.weights.Store("route-1", &routeWeight{
			RouteID:           "route-1",
			ActiveConnections: 50,
			BaseWeight:        1.0,
			CurrentWeight:     1.0,
		})
		lb.weights.Store("route-2", &routeWeight{
			RouteID:           "route-2",
			ActiveConnections: 10,
			BaseWeight:        1.0,
			CurrentWeight:     1.0,
		})
		lb.weights.Store("route-3", &routeWeight{
			RouteID:           "route-3",
			ActiveConnections: 5,
			BaseWeight:        1.0,
			CurrentWeight:     1.0,
		})

		result := lb.Balance(routes, 30)

		// Route-3 (least connections) should get more load than route-1
		assert.GreaterOrEqual(t, result["route-3"], result["route-1"])
	})

	t.Run("total_equals_load_invariant", func(t *testing.T) {
		result := lb.Balance(routes, 50)
		total := 0
		for _, count := range result {
			total += count
		}
		assert.Equal(t, 50, total)
	})
}

func TestBalance_WeightedRandom(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)
	lb.SetAlgorithm(AlgorithmWeightedRandom)

	routes := []*types.Route{
		createLBTestRoute("route-1"),
		createLBTestRoute("route-2"),
	}

	result := lb.Balance(routes, 5)

	total := 0
	for routeID, count := range result {
		total += count
		assert.Contains(t, []string{"route-1", "route-2"}, routeID)
		assert.GreaterOrEqual(t, count, 0)
	}
	assert.Equal(t, 5, total)
}

func TestBalance_Adaptive(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)
	lb.SetAlgorithm(AlgorithmAdaptive)

	routes := []*types.Route{
		createLBTestRoute("route-1"),
		createLBTestRoute("route-2"),
	}

	t.Run("distributes_proportionally_to_score", func(t *testing.T) {
		// Set up weights with different scores
		lb.weights.Store("route-1", &routeWeight{
			RouteID:       "route-1",
			BaseWeight:    1.0,
			CurrentWeight: 2.0, // Higher weight
			SuccessRate:   0.99,
			AvgLatency:    50 * time.Millisecond,
		})
		lb.weights.Store("route-2", &routeWeight{
			RouteID:       "route-2",
			BaseWeight:    1.0,
			CurrentWeight: 1.0, // Lower weight
			SuccessRate:   0.90,
			AvgLatency:    200 * time.Millisecond,
		})

		result := lb.Balance(routes, 100)

		// Route-1 should get more load due to higher weight
		assert.Greater(t, result["route-1"], result["route-2"])
	})

	t.Run("handles_rounding", func(t *testing.T) {
		result := lb.Balance(routes, 100)

		total := 0
		for _, count := range result {
			total += count
		}
		assert.Equal(t, 100, total, "rounding should not lose load")
	})
}

// === UpdateWeights Tests ===

func TestUpdateWeights(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)

	t.Run("updates_from_metrics", func(t *testing.T) {
		metrics := map[string]*types.RouteMetrics{
			"route-1": {
				TotalMessages:   100,
				SuccessfulCount: 95,
				FailedCount:     5,
				AvgLatency:      100 * time.Millisecond,
			},
		}

		err := lb.UpdateWeights(metrics)

		require.NoError(t, err)

		weights := lb.GetCurrentWeights()
		assert.Contains(t, weights, "route-1")
		assert.Greater(t, weights["route-1"], 0.0)
	})

	t.Run("higher_success_rate_increases_weight", func(t *testing.T) {
		metricsLow := map[string]*types.RouteMetrics{
			"route-low": {
				TotalMessages:   100,
				SuccessfulCount: 70,
				FailedCount:     30,
				AvgLatency:      100 * time.Millisecond,
			},
		}
		metricsHigh := map[string]*types.RouteMetrics{
			"route-high": {
				TotalMessages:   100,
				SuccessfulCount: 99,
				FailedCount:     1,
				AvgLatency:      100 * time.Millisecond,
			},
		}

		_ = lb.UpdateWeights(metricsLow)
		_ = lb.UpdateWeights(metricsHigh)

		weights := lb.GetCurrentWeights()
		assert.Greater(t, weights["route-high"], weights["route-low"])
	})

	t.Run("lower_latency_increases_weight", func(t *testing.T) {
		metricsSlow := map[string]*types.RouteMetrics{
			"route-slow": {
				TotalMessages:   100,
				SuccessfulCount: 95,
				FailedCount:     5,
				AvgLatency:      800 * time.Millisecond,
			},
		}
		metricsFast := map[string]*types.RouteMetrics{
			"route-fast": {
				TotalMessages:   100,
				SuccessfulCount: 95,
				FailedCount:     5,
				AvgLatency:      50 * time.Millisecond,
			},
		}

		_ = lb.UpdateWeights(metricsSlow)
		_ = lb.UpdateWeights(metricsFast)

		weights := lb.GetCurrentWeights()
		assert.Greater(t, weights["route-fast"], weights["route-slow"])
	})
}

// === calculateWeight Tests ===

func TestCalculateWeight(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)

	t.Run("success_factor_range", func(t *testing.T) {
		// 0% success rate -> factor = 0.5
		rw0 := &routeWeight{BaseWeight: 1.0, SuccessRate: 0.0}
		weight0 := lb.calculateWeight(rw0)

		// 100% success rate -> factor = 1.5
		rw100 := &routeWeight{BaseWeight: 1.0, SuccessRate: 1.0}
		weight100 := lb.calculateWeight(rw100)

		// Higher success rate should yield higher weight
		assert.Greater(t, weight100, weight0)
	})

	t.Run("latency_factor_range", func(t *testing.T) {
		// 100ms latency -> factor = 1.4
		rwFast := &routeWeight{BaseWeight: 1.0, SuccessRate: 0.95, AvgLatency: 100 * time.Millisecond}
		weightFast := lb.calculateWeight(rwFast)

		// 1000ms latency -> factor = 0.5
		rwSlow := &routeWeight{BaseWeight: 1.0, SuccessRate: 0.95, AvgLatency: 1000 * time.Millisecond}
		weightSlow := lb.calculateWeight(rwSlow)

		assert.Greater(t, weightFast, weightSlow)
	})

	t.Run("connection_penalty", func(t *testing.T) {
		rwLow := &routeWeight{BaseWeight: 1.0, SuccessRate: 0.95, ActiveConnections: 50}
		weightLow := lb.calculateWeight(rwLow)

		rwHigh := &routeWeight{BaseWeight: 1.0, SuccessRate: 0.95, ActiveConnections: 500}
		weightHigh := lb.calculateWeight(rwHigh)

		// High connections should penalize weight
		assert.Greater(t, weightLow, weightHigh)
	})

	t.Run("error_penalty", func(t *testing.T) {
		rwNoErrors := &routeWeight{BaseWeight: 1.0, SuccessRate: 0.95, ErrorCount: 0}
		weightNoErrors := lb.calculateWeight(rwNoErrors)

		rwErrors := &routeWeight{BaseWeight: 1.0, SuccessRate: 0.95, ErrorCount: 50}
		weightErrors := lb.calculateWeight(rwErrors)

		assert.Greater(t, weightNoErrors, weightErrors)
	})

	t.Run("minimum_weight_enforced", func(t *testing.T) {
		// Worst case scenario
		rw := &routeWeight{
			BaseWeight:        1.0,
			SuccessRate:       0.0,
			AvgLatency:        2 * time.Second,
			ActiveConnections: 1000,
			ErrorCount:        100,
		}
		weight := lb.calculateWeight(rw)

		assert.GreaterOrEqual(t, weight, 0.1, "weight should never drop below 0.1")
	})
}

// === Connection Tracking Tests ===

func TestConnectionTracking(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)

	// Initialize weight for route
	lb.weights.Store("route-1", &routeWeight{
		RouteID:           "route-1",
		BaseWeight:        1.0,
		CurrentWeight:     1.0,
		ActiveConnections: 0,
	})

	t.Run("increment_connections", func(t *testing.T) {
		lb.IncrementConnections("route-1")
		lb.IncrementConnections("route-1")
		lb.IncrementConnections("route-1")

		w, _ := lb.weights.Load("route-1")
		rw := w.(*routeWeight)
		rw.mu.RLock()
		connections := rw.ActiveConnections
		rw.mu.RUnlock()

		assert.Equal(t, int64(3), connections)
	})

	t.Run("decrement_connections", func(t *testing.T) {
		lb.DecrementConnections("route-1")

		w, _ := lb.weights.Load("route-1")
		rw := w.(*routeWeight)
		rw.mu.RLock()
		connections := rw.ActiveConnections
		rw.mu.RUnlock()

		assert.Equal(t, int64(2), connections)
	})

	t.Run("decrement_never_goes_negative", func(t *testing.T) {
		lb.weights.Store("route-2", &routeWeight{
			RouteID:           "route-2",
			BaseWeight:        1.0,
			CurrentWeight:     1.0,
			ActiveConnections: 0,
		})

		lb.DecrementConnections("route-2")

		w, _ := lb.weights.Load("route-2")
		rw := w.(*routeWeight)
		rw.mu.RLock()
		connections := rw.ActiveConnections
		rw.mu.RUnlock()

		assert.Equal(t, int64(0), connections, "connections should not go negative")
	})
}

// === SetAlgorithm Tests ===

func TestSetAlgorithm(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)

	// Default is adaptive
	assert.Equal(t, AlgorithmAdaptive, lb.algorithm)

	lb.SetAlgorithm(AlgorithmRoundRobin)
	assert.Equal(t, AlgorithmRoundRobin, lb.algorithm)

	lb.SetAlgorithm(AlgorithmLeastConnections)
	assert.Equal(t, AlgorithmLeastConnections, lb.algorithm)

	lb.SetAlgorithm(AlgorithmWeightedRandom)
	assert.Equal(t, AlgorithmWeightedRandom, lb.algorithm)
}

// === GetCurrentWeights Tests ===

func TestGetCurrentWeights(t *testing.T) {
	logger := zaptest.NewLogger(t)
	lb := NewAdaptiveLoadBalancer(logger)

	t.Run("empty_initially", func(t *testing.T) {
		weights := lb.GetCurrentWeights()
		assert.Empty(t, weights)
	})

	t.Run("returns_all_weights", func(t *testing.T) {
		lb.weights.Store("route-1", &routeWeight{RouteID: "route-1", CurrentWeight: 1.5})
		lb.weights.Store("route-2", &routeWeight{RouteID: "route-2", CurrentWeight: 0.8})

		weights := lb.GetCurrentWeights()

		assert.Len(t, weights, 2)
		assert.Equal(t, 1.5, weights["route-1"])
		assert.Equal(t, 0.8, weights["route-2"])
	})
}

// Helper function
func createLBTestRoute(id string) *types.Route {
	endpoint, _ := url.Parse("https://example.com/inbox")
	return &types.Route{
		ID:            id,
		InstanceID:    "instance-" + id,
		Domain:        "example.com",
		Endpoint:      endpoint,
		Priority:      1,
		Latency:       100 * time.Millisecond,
		SuccessRate:   0.95,
		CostPerByte:   0.0001,
		CircuitStatus: types.CircuitClosed,
	}
}
