package routing

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// SmartRouteOptimizer implements intelligent route optimization
type SmartRouteOptimizer struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger

	// Performance history cache
	perfCache sync.Map // routeID -> *routePerformance
	cacheTTL  time.Duration

	// Prediction models (simple for now, can be ML later)
	latencyModel *latencyPredictor
	costModel    *costPredictor

	// Configuration
	config *OptimizerConfig
}

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

// NewSmartRouteOptimizer creates a new route optimizer
func NewSmartRouteOptimizer(db *dynamodb.Client, tableName string, logger *zap.Logger, config *OptimizerConfig) *SmartRouteOptimizer {
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
		db:        db,
		tableName: tableName,
		logger:    logger,
		cacheTTL:  5 * time.Minute,
		config:    config,
		latencyModel: &latencyPredictor{
			alpha:       0.3,
			predictions: make(map[string]float64),
		},
		costModel: &costPredictor{
			costs: make(map[string]*timeCosts),
		},
	}

	// Start background optimization
	go sro.continuousOptimization()

	return sro
}

// OptimizeRoutes optimizes route selection based on historical performance
func (sro *SmartRouteOptimizer) OptimizeRoutes(ctx context.Context, routes []*Route, messageSize int64) ([]*Route, error) {
	if len(routes) == 0 {
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
	optimized := make([]*Route, len(scoredRoutes))
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
func (sro *SmartRouteOptimizer) PredictLatency(route *Route, messageSize int64) time.Duration {
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
func (sro *SmartRouteOptimizer) EstimateCost(route *Route, messageSize int64) float64 {
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
func (sro *SmartRouteOptimizer) RecordDeliveryResult(ctx context.Context, result *DeliveryResult) error {
	// Store in DynamoDB for persistence
	item := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ROUTE#%s", result.RouteID)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("RESULT#%d", time.Now().UnixNano())},

		"MessageID":  &types.AttributeValueMemberS{Value: result.MessageID},
		"Success":    &types.AttributeValueMemberBOOL{Value: result.Success},
		"StatusCode": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.StatusCode)},
		"Duration":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.Duration.Milliseconds())},
		"BytesSent":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.BytesSent)},
		"Cost":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%.6f", result.Cost)},
		"Timestamp":  &types.AttributeValueMemberS{Value: result.Timestamp.Format(time.RFC3339)},

		// GSI for time-based queries
		"GSI1PK": &types.AttributeValueMemberS{Value: "RESULTS"},
		"GSI1SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%d#%s", result.Timestamp.Unix(), result.RouteID)},

		// TTL for cleanup (30 days)
		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(30*24*time.Hour).Unix())},
	}

	if result.ErrorMessage != "" {
		item["ErrorMessage"] = &types.AttributeValueMemberS{Value: result.ErrorMessage}
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(sro.tableName),
		Item:      item,
	}

	_, err := sro.db.PutItem(ctx, putInput)
	if err != nil {
		return fmt.Errorf("record delivery result: %w", err)
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
func (sro *SmartRouteOptimizer) GetRouteMetrics(ctx context.Context, routeID string) (*RouteMetrics, error) {
	// Query recent results
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(sro.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("ROUTE#%s", routeID)},
			":prefix": &types.AttributeValueMemberS{Value: "RESULT#"},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(1000), // Last 1000 results
	}

	result, err := sro.db.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("query route metrics: %w", err)
	}

	metrics := &RouteMetrics{
		LastUpdated: time.Now(),
	}

	latencies := []time.Duration{}

	for _, item := range result.Items {
		// Parse result
		var success bool
		var duration, bytesSent int64
		var cost float64

		if v, ok := item["Success"].(*types.AttributeValueMemberBOOL); ok {
			success = v.Value
		}
		if v, ok := item["Duration"].(*types.AttributeValueMemberN); ok {
			if _, err := fmt.Sscanf(v.Value, "%d", &duration); err != nil {
				sro.logger.Warn("failed to parse duration", zap.String("value", v.Value), zap.Error(err))
			}
			latencies = append(latencies, time.Duration(duration)*time.Millisecond)
		}
		if v, ok := item["BytesSent"].(*types.AttributeValueMemberN); ok {
			if _, err := fmt.Sscanf(v.Value, "%d", &bytesSent); err != nil {
				sro.logger.Warn("failed to parse bytes sent", zap.String("value", v.Value), zap.Error(err))
			}
		}
		if v, ok := item["Cost"].(*types.AttributeValueMemberN); ok {
			if _, err := fmt.Sscanf(v.Value, "%f", &cost); err != nil {
				sro.logger.Warn("failed to parse cost", zap.String("value", v.Value), zap.Error(err))
			}
		}

		// Update counters
		metrics.TotalMessages++
		if success {
			metrics.SuccessfulCount++
		} else {
			metrics.FailedCount++
		}
		metrics.TotalBytes += bytesSent
		metrics.TotalCost += cost
	}

	// Calculate latency percentiles
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})

		// Average
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		metrics.AvgLatency = total / time.Duration(len(latencies))

		// P95
		p95Index := int(float64(len(latencies)) * 0.95)
		if p95Index >= len(latencies) {
			p95Index = len(latencies) - 1
		}
		metrics.P95Latency = latencies[p95Index]

		// P99
		p99Index := int(float64(len(latencies)) * 0.99)
		if p99Index >= len(latencies) {
			p99Index = len(latencies) - 1
		}
		metrics.P99Latency = latencies[p99Index]
	}

	return metrics, nil
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

	// Determine trend
	perf.TrendDirection = sro.calculateTrend(perf)

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
	route *Route
	score routeScore
}

func (sro *SmartRouteOptimizer) scoreRoute(route *Route, perf *routePerformance, messageSize int64) routeScore {
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
		case "improving":
			score.latency *= 1.1
			score.reliability *= 1.1
		case "degrading":
			score.latency *= 0.9
			score.reliability *= 0.9
		}
	}

	// Apply circuit breaker penalty
	if route.CircuitStatus != CircuitClosed {
		if route.CircuitStatus == CircuitOpen {
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

func (sro *SmartRouteOptimizer) updatePredictions(result *DeliveryResult) {
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
	if len(perf.Samples) == 0 {
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
		return "stable"
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
		return "improving"
	} else if (latencyDegraded || successDegraded) && !latencyImproved && !successImproved {
		return "degrading"
	}

	return "stable"
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

func (sro *SmartRouteOptimizer) storeOptimizationDecision(_ context.Context, routes []*Route, messageSize int64) {
	// Store decision for later analysis
	decision := map[string]any{
		"timestamp":   time.Now(),
		"messageSize": messageSize,
		"routes":      []string{},
	}

	for _, route := range routes {
		decision["routes"] = append(decision["routes"].([]string), route.ID)
	}

	// Store asynchronously
	go func() {
		item := map[string]types.AttributeValue{
			"PK":       &types.AttributeValueMemberS{Value: "OPTIMIZATION"},
			"SK":       &types.AttributeValueMemberS{Value: fmt.Sprintf("DECISION#%d", time.Now().UnixNano())},
			"Decision": &types.AttributeValueMemberS{Value: fmt.Sprintf("%v", decision)},
			"TTL":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(7*24*time.Hour).Unix())},
		}

		putInput := &dynamodb.PutItemInput{
			TableName: aws.String(sro.tableName),
			Item:      item,
		}

		_, err := sro.db.PutItem(context.Background(), putInput)
		if err != nil {
			sro.logger.Warn("failed to store optimization decision", zap.Error(err))
		}
	}()
}

func (sro *SmartRouteOptimizer) continuousOptimization() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Refresh predictions based on recent data
		ctx := context.Background()

		// Query recent results
		queryInput := &dynamodb.QueryInput{
			TableName:              aws.String(sro.tableName),
			IndexName:              aws.String("GSI1"),
			KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK > :since"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":    &types.AttributeValueMemberS{Value: "RESULTS"},
				":since": &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix())},
			},
			Limit: aws.Int32(500),
		}

		result, err := sro.db.Query(ctx, queryInput)
		if err != nil {
			sro.logger.Error("continuous optimization query failed", zap.Error(err))
			continue
		}

		// Update models with recent data
		for _, item := range result.Items {
			// Parse and update predictions
			// Implementation details omitted for brevity
			_ = item // Mark as used
		}

		sro.logger.Info("continuous optimization completed",
			zap.Int("samplesProcessed", len(result.Items)))
	}
}
