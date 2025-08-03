package routing

import (
	"crypto/rand"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"go.uber.org/zap"
)

// AdaptiveLoadBalancer implements intelligent load distribution
type AdaptiveLoadBalancer struct {
	logger *zap.Logger

	// Weights for each route
	weights sync.Map // routeID -> *routeWeight

	// Algorithm configuration
	algorithm LoadBalancingAlgorithm
}

type LoadBalancingAlgorithm string

const (
	AlgorithmRoundRobin       LoadBalancingAlgorithm = "round_robin"
	AlgorithmWeightedRandom   LoadBalancingAlgorithm = "weighted_random"
	AlgorithmLeastConnections LoadBalancingAlgorithm = "least_connections"
	AlgorithmAdaptive         LoadBalancingAlgorithm = "adaptive"
)

type routeWeight struct {
	RouteID           string
	BaseWeight        float64
	CurrentWeight     float64
	ActiveConnections int64
	LastUpdated       time.Time

	// Performance metrics for adaptive algorithm
	SuccessRate float64
	AvgLatency  time.Duration
	ErrorCount  int64

	mu sync.RWMutex
}

// NewAdaptiveLoadBalancer creates a new load balancer
func NewAdaptiveLoadBalancer(logger *zap.Logger) *AdaptiveLoadBalancer {
	return &AdaptiveLoadBalancer{
		logger:    logger,
		algorithm: AlgorithmAdaptive,
	}
}

// Balance distributes load across routes
func (alb *AdaptiveLoadBalancer) Balance(routes []*types.Route, load int) map[string]int {
	if len(routes) == 0 {
		return make(map[string]int)
	}

	// Initialize or update weights
	alb.updateWeights(routes)

	// Distribute load based on algorithm
	switch alb.algorithm {
	case AlgorithmRoundRobin:
		return alb.roundRobin(routes, load)

	case AlgorithmWeightedRandom:
		return alb.weightedRandom(routes, load)

	case AlgorithmLeastConnections:
		return alb.leastConnections(routes, load)

	case AlgorithmAdaptive:
		return alb.adaptive(routes, load)

	default:
		return alb.adaptive(routes, load)
	}
}

// UpdateWeights updates route weights based on metrics
func (alb *AdaptiveLoadBalancer) UpdateWeights(metrics map[string]*types.RouteMetrics) error {
	for routeID, metric := range metrics {
		weight, _ := alb.weights.LoadOrStore(routeID, &routeWeight{
			RouteID:     routeID,
			BaseWeight:  1.0,
			LastUpdated: time.Now(),
		})

		rw := weight.(*routeWeight)
		rw.mu.Lock()

		// Update performance metrics
		if metric.TotalMessages > 0 {
			rw.SuccessRate = float64(metric.SuccessfulCount) / float64(metric.TotalMessages)
			rw.AvgLatency = metric.AvgLatency
			rw.ErrorCount = metric.FailedCount
		}

		// Calculate new weight based on performance
		rw.CurrentWeight = alb.calculateWeight(rw)
		rw.LastUpdated = time.Now()

		rw.mu.Unlock()

		alb.logger.Debug("updated route weight",
			zap.String("routeID", routeID),
			zap.Float64("weight", rw.CurrentWeight),
			zap.Float64("successRate", rw.SuccessRate))
	}

	return nil
}

// GetCurrentWeights returns current weights for all routes
func (alb *AdaptiveLoadBalancer) GetCurrentWeights() map[string]float64 {
	weights := make(map[string]float64)

	alb.weights.Range(func(key, value any) bool {
		rw := value.(*routeWeight)
		rw.mu.RLock()
		weights[rw.RouteID] = rw.CurrentWeight
		rw.mu.RUnlock()
		return true
	})

	return weights
}

// Private methods

func (alb *AdaptiveLoadBalancer) updateWeights(routes []*types.Route) {
	for _, route := range routes {
		weight, _ := alb.weights.LoadOrStore(route.ID, &routeWeight{
			RouteID:       route.ID,
			BaseWeight:    1.0,
			CurrentWeight: 1.0,
			SuccessRate:   route.SuccessRate,
			LastUpdated:   time.Now(),
		})

		// Update from route data
		rw := weight.(*routeWeight)
		rw.mu.Lock()
		rw.SuccessRate = route.SuccessRate
		rw.AvgLatency = route.Latency
		rw.mu.Unlock()
	}
}

func (alb *AdaptiveLoadBalancer) roundRobin(routes []*types.Route, load int) map[string]int {
	distribution := make(map[string]int)

	// Simple round-robin distribution
	for i := 0; i < load; i++ {
		route := routes[i%len(routes)]
		distribution[route.ID]++
	}

	return distribution
}

func (alb *AdaptiveLoadBalancer) weightedRandom(routes []*types.Route, load int) map[string]int {
	distribution := make(map[string]int)

	// Calculate total weight
	totalWeight := 0.0
	weights := make([]float64, len(routes))
	for i, route := range routes {
		if w, ok := alb.weights.Load(route.ID); ok {
			rw := w.(*routeWeight)
			rw.mu.RLock()
			weights[i] = rw.CurrentWeight
			rw.mu.RUnlock()
		} else {
			weights[i] = 1.0
		}
		totalWeight += weights[i]
	}

	// Distribute load based on weights
	for i := 0; i < load; i++ {
		// Use crypto/rand for secure random number generation
		randVal, err := rand.Int(rand.Reader, big.NewInt(int64(totalWeight*1000)))
		if err != nil {
			// Fallback to less random source on error, or handle error appropriately
			alb.logger.Error("failed to generate random number for load balancing", zap.Error(err))
			// Simple fallback to round-robin for this iteration
			route := routes[i%len(routes)]
			distribution[route.ID]++
			continue
		}

		r := float64(randVal.Int64()) / 1000.0
		cumWeight := 0.0

		for j, weight := range weights {
			cumWeight += weight
			if r <= cumWeight {
				distribution[routes[j].ID]++
				break
			}
		}
	}

	return distribution
}

func (alb *AdaptiveLoadBalancer) leastConnections(routes []*types.Route, load int) map[string]int {
	distribution := make(map[string]int)

	// Sort routes by active connections
	sorted := make([]*routeConnection, len(routes))
	for i, route := range routes {
		connections := int64(0)
		if w, ok := alb.weights.Load(route.ID); ok {
			rw := w.(*routeWeight)
			rw.mu.RLock()
			connections = rw.ActiveConnections
			rw.mu.RUnlock()
		}
		sorted[i] = &routeConnection{
			route:       route,
			connections: connections,
		}
	}

	// Distribute to routes with least connections
	for i := 0; i < load; i++ {
		sort.Slice(sorted, func(a, b int) bool {
			return sorted[a].connections < sorted[b].connections
		})

		// Assign to route with least connections
		distribution[sorted[0].route.ID]++
		sorted[0].connections++

		// Update active connections
		if w, ok := alb.weights.Load(sorted[0].route.ID); ok {
			rw := w.(*routeWeight)
			rw.mu.Lock()
			rw.ActiveConnections++
			rw.mu.Unlock()
		}
	}

	return distribution
}

func (alb *AdaptiveLoadBalancer) adaptive(routes []*types.Route, load int) map[string]int {
	distribution := make(map[string]int)

	// Score each route
	scoredRoutes := make([]*scoredRoute, len(routes))
	totalScore := 0.0

	for i, route := range routes {
		score := alb.scoreRoute(route)
		scoredRoutes[i] = &scoredRoute{
			route: route,
			score: routeScore{total: score},
		}
		totalScore += score
	}

	// Distribute load proportionally to scores
	for _, sr := range scoredRoutes {
		if totalScore > 0 {
			share := int(math.Round(float64(load) * sr.score.total / totalScore))
			distribution[sr.route.ID] = share
		}
	}

	// Handle rounding errors
	distributed := 0
	for _, count := range distribution {
		distributed += count
	}

	// Distribute remaining load to best routes
	if distributed < load {
		remaining := load - distributed
		sort.Slice(scoredRoutes, func(i, j int) bool {
			return scoredRoutes[i].score.total > scoredRoutes[j].score.total
		})

		for i := 0; i < remaining && i < len(scoredRoutes); i++ {
			distribution[scoredRoutes[i].route.ID]++
		}
	}

	return distribution
}

func (alb *AdaptiveLoadBalancer) calculateWeight(rw *routeWeight) float64 {
	weight := rw.BaseWeight

	// Success rate factor (0.5 - 1.5)
	successFactor := 0.5 + rw.SuccessRate
	weight *= successFactor

	// Latency factor (0.5 - 1.5)
	latencyFactor := 1.5
	if rw.AvgLatency > 0 {
		// Better latency = higher weight
		// 100ms = 1.5, 500ms = 1.0, 1000ms = 0.5
		latencyMs := float64(rw.AvgLatency.Milliseconds())
		latencyFactor = 1.5 - (latencyMs / 1000.0)
		latencyFactor = math.Max(0.5, math.Min(1.5, latencyFactor))
	}
	weight *= latencyFactor

	// Active connections penalty
	if rw.ActiveConnections > 100 {
		connectionPenalty := 1.0 - float64(rw.ActiveConnections-100)/1000.0
		connectionPenalty = math.Max(0.1, connectionPenalty)
		weight *= connectionPenalty
	}

	// Recent errors penalty
	if rw.ErrorCount > 0 {
		errorPenalty := 1.0 - float64(rw.ErrorCount)/100.0
		errorPenalty = math.Max(0.1, errorPenalty)
		weight *= errorPenalty
	}

	return math.Max(0.1, weight) // Minimum weight of 0.1
}

func (alb *AdaptiveLoadBalancer) scoreRoute(route *types.Route) float64 {
	score := 1.0

	// Get weight data
	if w, ok := alb.weights.Load(route.ID); ok {
		rw := w.(*routeWeight)
		rw.mu.RLock()
		defer rw.mu.RUnlock()

		// Use calculated weight as base score
		score = rw.CurrentWeight

		// Circuit breaker factor
		switch route.CircuitStatus {
		case types.CircuitOpen:
			score *= 0.0 // No traffic to open circuits
		case types.CircuitHalfOpen:
			score *= 0.1 // Minimal traffic for testing
		}

		// Priority factor
		if route.Priority > 0 {
			priorityFactor := 1.0 / float64(route.Priority)
			score *= priorityFactor
		}
	}

	return score
}

type routeConnection struct {
	route       *types.Route
	connections int64
}

// IncrementConnections increments active connections for a route
func (alb *AdaptiveLoadBalancer) IncrementConnections(routeID string) {
	if w, ok := alb.weights.Load(routeID); ok {
		rw := w.(*routeWeight)
		rw.mu.Lock()
		rw.ActiveConnections++
		rw.mu.Unlock()
	}
}

// DecrementConnections decrements active connections for a route
func (alb *AdaptiveLoadBalancer) DecrementConnections(routeID string) {
	if w, ok := alb.weights.Load(routeID); ok {
		rw := w.(*routeWeight)
		rw.mu.Lock()
		if rw.ActiveConnections > 0 {
			rw.ActiveConnections--
		}
		rw.mu.Unlock()
	}
}

// SetAlgorithm changes the load balancing algorithm
func (alb *AdaptiveLoadBalancer) SetAlgorithm(algorithm LoadBalancingAlgorithm) {
	alb.algorithm = algorithm
	alb.logger.Info("load balancing algorithm changed",
		zap.String("algorithm", string(algorithm)))
}
