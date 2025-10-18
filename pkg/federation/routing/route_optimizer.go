package routing

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// Trend direction constants
const (
	trendStable    = "stable"
	trendImproving = "improving"
	trendDegrading = "degrading"
)

// SmartRouteOptimizer implements intelligent route optimization
type SmartRouteOptimizer struct {
	repoInterface RepositoryInterface
	logger        *zap.Logger

	// Performance history cache
	perfCache sync.Map // routeID -> *routePerformance
	cacheTTL  time.Duration

	// Prediction models (simple for now, can be ML later)
	latencyModel *latencyPredictor
	costModel    *costPredictor

	// Configuration
	config *OptimizerConfig
}

// OptimizerConfig contains configuration for route optimization
type OptimizerConfig struct {
	// Weights for scoring
	LatencyWeight     float64
	ReliabilityWeight float64
	CostWeight        float64

	// Thresholds
	MaxAcceptableLatency time.Duration
	MinAcceptableSuccess float64
	MaxCostPerMB         float64

	// Learning parameters
	HistoryWindow      time.Duration
	MinSamplesRequired int
	AdaptationRate     float64
}

type routePerformance struct {
	RouteID     string
	Samples     []performanceSample
	LastUpdated time.Time

	// Aggregated metrics
	AvgLatency     time.Duration
	P95Latency     time.Duration
	SuccessRate    float64
	AvgCostPerByte float64
	TrendDirection string // "improving", "stable", "degrading"
}

type performanceSample struct {
	Timestamp time.Time
	Latency   time.Duration
	Success   bool
	BytesSent int64
	Cost      float64
	ErrorType string
}

type latencyPredictor struct {
	// Simple exponential smoothing for now
	alpha       float64 // Smoothing factor
	mu          sync.RWMutex
	predictions map[string]float64 // routeID -> predicted latency (ms)
}

type costPredictor struct {
	// Cost per byte by route and time of day
	mu    sync.RWMutex
	costs map[string]*timeCosts
}

type timeCosts struct {
	hourly [24]float64 // Cost per byte by hour
}

// RepositoryInterface defines the methods needed from a repository
type RepositoryInterface interface {
	RecordDeliveryResult(ctx context.Context, result *types.DeliveryResult) error
	GetRouteMetrics(ctx context.Context, routeID string) (*types.RouteMetrics, error)
	GetRoutePerformance(ctx context.Context, routeID string) (interface{}, error)
	StoreOptimizationDecision(ctx context.Context, routes []*types.Route, messageSize int64) error
}

// NewSmartRouteOptimizer creates a new route optimizer with concrete repository
func NewSmartRouteOptimizer(repo *repositories.RouteOptimizerRepository, logger *zap.Logger, config *OptimizerConfig) *SmartRouteOptimizer {
	return newSmartRouteOptimizer(repo, logger, config)
}

// NewSmartRouteOptimizerFromInterface creates a new route optimizer from interface
func NewSmartRouteOptimizerFromInterface(repo RepositoryInterface, logger *zap.Logger, config *OptimizerConfig) *SmartRouteOptimizer {
	return newSmartRouteOptimizer(repo, logger, config)
}

// newSmartRouteOptimizer is the internal constructor
func newSmartRouteOptimizer(repo RepositoryInterface, logger *zap.Logger, config *OptimizerConfig) *SmartRouteOptimizer {
	if config == nil {
		config = &OptimizerConfig{
			LatencyWeight:        0.4,
			ReliabilityWeight:    0.4,
			CostWeight:           0.2,
			MaxAcceptableLatency: 2 * time.Second,
			MinAcceptableSuccess: 0.95,
			MaxCostPerMB:         0.10,
			HistoryWindow:        24 * time.Hour,
			MinSamplesRequired:   10,
			AdaptationRate:       0.1,
		}
	}

	sro := &SmartRouteOptimizer{
		repoInterface: repo,
		logger:        logger,
		cacheTTL:      5 * time.Minute,
		config:        config,
		latencyModel: &latencyPredictor{
			alpha:       0.3,
			predictions: make(map[string]float64),
		},
		costModel: &costPredictor{
			costs: make(map[string]*timeCosts),
		},
	}

	return sro
}

// OptimizeRoutes optimizes route selection based on historical performance
func (sro *SmartRouteOptimizer) OptimizeRoutes(ctx context.Context, routes []*types.Route, messageSize int64) ([]*types.Route, error) {
	if err := common.ValidateSliceNotEmpty("routes", routes); err != nil {
		return routes, nil
	}

	// Fetch performance data for all routes
	perfData := make(map[string]*routePerformance)
	for _, route := range routes {
		perf, err := sro.getRoutePerformance(ctx, route.ID)
		if err != nil {
			sro.logger.Warn("failed to get route performance",
				zap.String("routeID", route.ID),
				zap.Error(err))
			continue
		}
		perfData[route.ID] = perf
	}

	// Score and rank routes
	scoredRoutes := make([]scoredRoute, 0, len(routes))
	for _, route := range routes {
		score := sro.scoreRoute(route, perfData[route.ID], messageSize)
		scoredRoutes = append(scoredRoutes, scoredRoute{
			route: route,
			score: score,
		})

		sro.logger.Debug("route scored",
			zap.String("routeID", route.ID),
			zap.Float64("score", score.total),
			zap.Float64("latencyScore", score.latency),
			zap.Float64("reliabilityScore", score.reliability),
			zap.Float64("costScore", score.cost))
	}

	// Sort by score (descending)
	sort.Slice(scoredRoutes, func(i, j int) bool {
		return scoredRoutes[i].score.total > scoredRoutes[j].score.total
	})

	// Extract sorted routes
	optimized := make([]*types.Route, len(scoredRoutes))
	for i, sr := range scoredRoutes {
		optimized[i] = sr.route
		// Update route priority based on ranking
		optimized[i].Priority = i + 1
	}

	// Store optimization decision for learning
	sro.storeOptimizationDecision(ctx, optimized, messageSize)

	return optimized, nil
}

// PredictLatency predicts latency for a route based on historical data
func (sro *SmartRouteOptimizer) PredictLatency(route *types.Route, messageSize int64) time.Duration {
	sro.latencyModel.mu.RLock()
	defer sro.latencyModel.mu.RUnlock()

	// Base prediction from historical data
	basePrediction, exists := sro.latencyModel.predictions[route.ID]
	if !exists {
		// Use route's current latency as baseline
		basePrediction = float64(route.Latency.Milliseconds())
	}

	// Adjust for message size (simple linear model)
	// Assume 1KB takes base time, scale linearly
	sizeFactorKB := float64(messageSize) / 1024.0
	adjustedLatency := basePrediction * (1.0 + (sizeFactorKB-1.0)*0.1)

	// Consider network conditions (time of day, etc.)
	hourFactor := sro.getHourlyLatencyFactor()
	finalLatency := adjustedLatency * hourFactor

	return time.Duration(finalLatency) * time.Millisecond
}

// EstimateCost estimates the cost of sending a message through a route
func (sro *SmartRouteOptimizer) EstimateCost(route *types.Route, messageSize int64) float64 {
	sro.costModel.mu.RLock()
	defer sro.costModel.mu.RUnlock()

	// Get time-based cost
	costs, exists := sro.costModel.costs[route.ID]
	if !exists {
		// Use route's base cost
		return route.CostPerByte * float64(messageSize)
	}

	// Use current hour's cost
	hour := time.Now().Hour()
	costPerByte := costs.hourly[hour]
	if costPerByte == 0 {
		costPerByte = route.CostPerByte
	}

	return costPerByte * float64(messageSize)
}

// RecordDeliveryResult records the result of a delivery for learning
func (sro *SmartRouteOptimizer) RecordDeliveryResult(ctx context.Context, result *types.DeliveryResult) error {
	// Use repository to store the result
	err := sro.repoInterface.RecordDeliveryResult(ctx, result)
	if err != nil {
		sro.logger.Error("failed to record delivery result",
			zap.Error(err),
			zap.String("route_id", result.RouteID),
			zap.Bool("success", result.Success))
		return errors.Join(ErrRecordDeliveryResultFailed, err)
	}

	// Update predictions
	sro.updatePredictions(result)

	// Update cache
	if perf, ok := sro.perfCache.Load(result.RouteID); ok {
		rp := perf.(*routePerformance)
		rp.Samples = append(rp.Samples, performanceSample{
			Timestamp: result.Timestamp,
			Latency:   result.Duration,
			Success:   result.Success,
			BytesSent: result.BytesSent,
			Cost:      result.Cost,
			ErrorType: result.ErrorMessage,
		})

		// Keep only recent samples
		cutoff := time.Now().Add(-sro.config.HistoryWindow)
		filtered := rp.Samples[:0]
		for _, s := range rp.Samples {
			if s.Timestamp.After(cutoff) {
				filtered = append(filtered, s)
			}
		}
		rp.Samples = filtered
		rp.LastUpdated = time.Now()

		// Recalculate aggregates
		sro.updateAggregates(rp)
	}

	return nil
}

// GetRouteMetrics retrieves detailed metrics for a route
func (sro *SmartRouteOptimizer) GetRouteMetrics(ctx context.Context, routeID string) (*types.RouteMetrics, error) {
	return sro.repoInterface.GetRouteMetrics(ctx, routeID)
}

// Helper methods

func (sro *SmartRouteOptimizer) getRoutePerformance(ctx context.Context, routeID string) (*routePerformance, error) {
	// Check cache
	if cached, ok := sro.perfCache.Load(routeID); ok {
		if perf, ok := cached.(*routePerformance); ok && time.Since(perf.LastUpdated) < sro.cacheTTL {
			return perf, nil
		}
	}

	// Fetch from database
	metrics, err := sro.GetRouteMetrics(ctx, routeID)
	if err != nil {
		return nil, err
	}

	perf := &routePerformance{
		RouteID:        routeID,
		LastUpdated:    time.Now(),
		AvgLatency:     metrics.AvgLatency,
		P95Latency:     metrics.P95Latency,
		SuccessRate:    float64(metrics.SuccessfulCount) / float64(metrics.TotalMessages),
		AvgCostPerByte: metrics.TotalCost / float64(metrics.TotalBytes),
	}

	// Get raw performance data for trend calculation
	rawData, err := sro.repoInterface.GetRoutePerformance(ctx, routeID)
	if err != nil {
		sro.logger.Warn("failed to get raw performance data", zap.String("routeID", routeID), zap.Error(err))
		perf.TrendDirection = trendStable // Default to stable if we can't calculate trend
	} else {
		// Convert raw data to samples for trend calculation
		if results, ok := rawData.([]*models.RouteDeliveryResult); ok {
			perf.Samples = make([]performanceSample, len(results))
			for i, result := range results {
				perf.Samples[i] = performanceSample{
					Timestamp: result.Timestamp,
					Latency:   time.Duration(result.Duration) * time.Millisecond,
					Success:   result.Success,
					BytesSent: result.BytesSent,
					Cost:      result.Cost,
					ErrorType: result.ErrorMessage,
				}
			}
			// Determine trend
			perf.TrendDirection = sro.calculateTrend(perf)
		} else {
			perf.TrendDirection = trendStable
		}
	}

	// Cache it
	sro.perfCache.Store(routeID, perf)

	return perf, nil
}

type routeScore struct {
	total       float64
	latency     float64
	reliability float64
	cost        float64
}

type scoredRoute struct {
	route *types.Route
	score routeScore
}

func (sro *SmartRouteOptimizer) scoreRoute(route *types.Route, perf *routePerformance, messageSize int64) routeScore {
	score := routeScore{}

	// Default values if no performance data
	if perf == nil || len(perf.Samples) < sro.config.MinSamplesRequired {
		// Use route's static values
		score.latency = 1.0 - math.Min(float64(route.Latency)/float64(sro.config.MaxAcceptableLatency), 1.0)
		score.reliability = route.SuccessRate
		score.cost = 1.0 - math.Min(route.CostPerByte*float64(messageSize)/sro.config.MaxCostPerMB, 1.0)
	} else {
		// Use performance data
		score.latency = 1.0 - math.Min(float64(perf.AvgLatency)/float64(sro.config.MaxAcceptableLatency), 1.0)
		score.reliability = perf.SuccessRate
		score.cost = 1.0 - math.Min(perf.AvgCostPerByte*float64(messageSize)/sro.config.MaxCostPerMB, 1.0)

		// Boost/penalize based on trend
		switch perf.TrendDirection {
		case trendImproving:
			score.latency *= 1.1
			score.reliability *= 1.1
		case trendDegrading:
			score.latency *= 0.9
			score.reliability *= 0.9
		}
	}

	// Apply circuit breaker penalty
	if route.CircuitStatus != types.CircuitClosed {
		if route.CircuitStatus == types.CircuitOpen {
			score.reliability *= 0.1
		} else { // Half-open
			score.reliability *= 0.5
		}
	}

	// Calculate weighted total
	score.total = score.latency*sro.config.LatencyWeight +
		score.reliability*sro.config.ReliabilityWeight +
		score.cost*sro.config.CostWeight

	return score
}

func (sro *SmartRouteOptimizer) updatePredictions(result *types.DeliveryResult) {
	// Update latency prediction
	sro.latencyModel.mu.Lock()
	current, exists := sro.latencyModel.predictions[result.RouteID]
	if !exists {
		current = float64(result.Duration.Milliseconds())
	} else {
		// Exponential smoothing
		current = sro.latencyModel.alpha*float64(result.Duration.Milliseconds()) +
			(1-sro.latencyModel.alpha)*current
	}
	sro.latencyModel.predictions[result.RouteID] = current
	sro.latencyModel.mu.Unlock()

	// Update cost model
	sro.costModel.mu.Lock()
	costs, exists := sro.costModel.costs[result.RouteID]
	if !exists {
		costs = &timeCosts{}
		sro.costModel.costs[result.RouteID] = costs
	}

	hour := result.Timestamp.Hour()
	costPerByte := result.Cost / float64(result.BytesSent)
	if costs.hourly[hour] == 0 {
		costs.hourly[hour] = costPerByte
	} else {
		// Moving average
		costs.hourly[hour] = costs.hourly[hour]*0.9 + costPerByte*0.1
	}
	sro.costModel.mu.Unlock()
}

func (sro *SmartRouteOptimizer) updateAggregates(perf *routePerformance) {
	if err := common.ValidateSliceNotEmpty("perf.Samples", perf.Samples); err != nil {
		return
	}

	// Calculate success rate
	successCount := 0
	var totalLatency time.Duration
	var totalCost float64
	var totalBytes int64

	for _, s := range perf.Samples {
		if s.Success {
			successCount++
			totalLatency += s.Latency
		}
		totalCost += s.Cost
		totalBytes += s.BytesSent
	}

	perf.SuccessRate = float64(successCount) / float64(len(perf.Samples))

	if successCount > 0 {
		perf.AvgLatency = totalLatency / time.Duration(successCount)
	}

	if totalBytes > 0 {
		perf.AvgCostPerByte = totalCost / float64(totalBytes)
	}
}

func (sro *SmartRouteOptimizer) calculateTrend(perf *routePerformance) string {
	if len(perf.Samples) < 20 {
		return trendStable
	}

	// Compare recent performance to older performance
	mid := len(perf.Samples) / 2

	var oldLatency, newLatency time.Duration
	var oldSuccess, newSuccess int

	for i := 0; i < mid; i++ {
		if perf.Samples[i].Success {
			oldSuccess++
			oldLatency += perf.Samples[i].Latency
		}
	}

	for i := mid; i < len(perf.Samples); i++ {
		if perf.Samples[i].Success {
			newSuccess++
			newLatency += perf.Samples[i].Latency
		}
	}

	// Calculate averages
	if oldSuccess > 0 {
		oldLatency /= time.Duration(oldSuccess)
	}
	if newSuccess > 0 {
		newLatency /= time.Duration(newSuccess)
	}

	oldSuccessRate := float64(oldSuccess) / float64(mid)
	newSuccessRate := float64(newSuccess) / float64(len(perf.Samples)-mid)

	// Determine trend
	latencyImproved := newLatency < time.Duration(float64(oldLatency)*0.95)
	latencyDegraded := newLatency > time.Duration(float64(oldLatency)*1.05)
	successImproved := newSuccessRate > oldSuccessRate*1.05
	successDegraded := newSuccessRate < oldSuccessRate*0.95

	if (latencyImproved || successImproved) && !latencyDegraded && !successDegraded {
		return trendImproving
	} else if (latencyDegraded || successDegraded) && !latencyImproved && !successImproved {
		return trendDegrading
	}

	return trendStable
}

func (sro *SmartRouteOptimizer) getHourlyLatencyFactor() float64 {
	hour := time.Now().Hour()

	// Simple model: higher latency during business hours
	switch {
	case hour >= 9 && hour <= 17: // Business hours
		return 1.2
	case hour >= 18 && hour <= 21: // Evening peak
		return 1.3
	case hour >= 0 && hour <= 6: // Night/early morning
		return 0.8
	default:
		return 1.0
	}
}

func (sro *SmartRouteOptimizer) storeOptimizationDecision(ctx context.Context, routes []*types.Route, messageSize int64) {
	// Store decision using repository
	err := sro.repoInterface.StoreOptimizationDecision(ctx, routes, messageSize)
	if err != nil {
		sro.logger.Warn("failed to store optimization decision", zap.Error(err))
	}
}

// Note: RefreshPredictions was removed since predictions are now updated
// automatically whenever RecordDeliveryResult is called, eliminating the
// need for separate background processing or manual refresh calls.
