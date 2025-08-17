// Package observability provides latency aggregation and percentile calculation services
package observability

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// Trend direction constants
const (
	TrendDirectionStable = "stable"
)

// LatencyAggregator handles real-time latency aggregation and percentile calculations
type LatencyAggregator struct {
	mu                sync.RWMutex
	buckets           map[string]*LatencyBucket
	logger            *zap.Logger
	metricsRecorder   MetricsRecorder
	aggregateInterval time.Duration
	retentionPeriod   time.Duration
	maxBuckets        int
	stopCh            chan struct{}
	started           bool
}

// LatencyBucket holds latency measurements for a specific operation/time window
type LatencyBucket struct {
	Operation   string
	Service     string
	WindowStart time.Time
	WindowEnd   time.Time
	Measurements []float64
	Count       int64
	Sum         float64
	Min         float64
	Max         float64
	mu          sync.Mutex
}

// LatencyStats represents calculated latency statistics
type LatencyStats struct {
	Operation   string          `json:"operation"`
	Service     string          `json:"service"`
	WindowStart time.Time       `json:"window_start"`
	WindowEnd   time.Time       `json:"window_end"`
	Count       int64           `json:"count"`
	Sum         float64         `json:"sum"`
	Average     float64         `json:"average"`
	Min         float64         `json:"min"`
	Max         float64         `json:"max"`
	Percentiles map[string]float64 `json:"percentiles"`
	StdDev      float64         `json:"std_dev"`
}

// LatencyTrend represents latency trending over time
type LatencyTrend struct {
	Operation      string               `json:"operation"`
	Service        string               `json:"service"`
	TimeRange      string               `json:"time_range"`
	DataPoints     []LatencyDataPoint   `json:"data_points"`
	TrendAnalysis  TrendAnalysis        `json:"trend_analysis"`
	Percentiles    map[string][]float64 `json:"percentiles"`
}

// LatencyDataPoint represents a single data point in a trend
type LatencyDataPoint struct {
	Timestamp   time.Time          `json:"timestamp"`
	Average     float64            `json:"average"`
	Count       int64              `json:"count"`
	Percentiles map[string]float64 `json:"percentiles"`
}

// TrendAnalysis provides statistical analysis of latency trends
type TrendAnalysis struct {
	Slope             float64 `json:"slope"`               // Trend direction (positive = increasing)
	RSquared          float64 `json:"r_squared"`           // Trend strength (0-1)
	TrendDirection    string  `json:"trend_direction"`     // "increasing", "decreasing", "stable"
	PercentChange     float64 `json:"percent_change"`      // Change from first to last data point
	Volatility        float64 `json:"volatility"`          // Standard deviation of changes
	IsSignificant     bool    `json:"is_significant"`      // Whether trend is statistically significant
	ChangeClassification string `json:"change_classification"` // "significant_improvement", "significant_degradation", "stable"
}

// NewLatencyAggregator creates a new latency aggregator
func NewLatencyAggregator(logger *zap.Logger, recorder MetricsRecorder, options ...LatencyAggregatorOption) *LatencyAggregator {
	la := &LatencyAggregator{
		buckets:           make(map[string]*LatencyBucket),
		logger:            logger,
		metricsRecorder:   recorder,
		aggregateInterval: 5 * time.Minute,  // Default 5-minute windows
		retentionPeriod:   24 * time.Hour,   // Keep data for 24 hours in memory
		maxBuckets:        1000,             // Maximum number of buckets to keep in memory
		stopCh:            make(chan struct{}),
	}

	// Apply options
	for _, option := range options {
		option(la)
	}

	return la
}

// LatencyAggregatorOption configures the latency aggregator
type LatencyAggregatorOption func(*LatencyAggregator)

// WithAggregateInterval sets the aggregation window interval
func WithAggregateInterval(interval time.Duration) LatencyAggregatorOption {
	return func(la *LatencyAggregator) {
		la.aggregateInterval = interval
	}
}

// WithRetentionPeriod sets how long to keep data in memory
func WithRetentionPeriod(period time.Duration) LatencyAggregatorOption {
	return func(la *LatencyAggregator) {
		la.retentionPeriod = period
	}
}

// WithMaxBuckets sets the maximum number of buckets to keep
func WithMaxBuckets(maxBuckets int) LatencyAggregatorOption {
	return func(la *LatencyAggregator) {
		la.maxBuckets = maxBuckets
	}
}

// Start begins the aggregation process
func (la *LatencyAggregator) Start() {
	la.mu.Lock()
	if la.started {
		la.mu.Unlock()
		return
	}
	la.started = true
	la.mu.Unlock()

	go la.aggregationLoop()
	go la.cleanupLoop()
	
	la.logger.Info("latency aggregator started",
		zap.Duration("aggregate_interval", la.aggregateInterval),
		zap.Duration("retention_period", la.retentionPeriod),
		zap.Int("max_buckets", la.maxBuckets))
}

// Stop stops the aggregation process
func (la *LatencyAggregator) Stop() {
	la.mu.Lock()
	if !la.started {
		la.mu.Unlock()
		return
	}
	la.started = false
	la.mu.Unlock()

	close(la.stopCh)
	la.logger.Info("latency aggregator stopped")
}

// RecordLatency records a latency measurement
func (la *LatencyAggregator) RecordLatency(operation, service string, duration time.Duration) {
	la.mu.Lock()
	defer la.mu.Unlock()

	bucketKey := la.getBucketKey(operation, service, time.Now())
	bucket, exists := la.buckets[bucketKey]
	
	if !exists {
		bucket = la.createBucket(operation, service, time.Now())
		la.buckets[bucketKey] = bucket
	}

	bucket.addMeasurement(float64(duration.Milliseconds()))
}

// GetCurrentStats returns current latency statistics for an operation
func (la *LatencyAggregator) GetCurrentStats(operation, service string) (*LatencyStats, error) {
	la.mu.RLock()
	defer la.mu.RUnlock()

	bucketKey := la.getBucketKey(operation, service, time.Now())
	bucket, exists := la.buckets[bucketKey]
	
	if !exists {
		return nil, fmt.Errorf("no data available for operation %s", operation)
	}

	return bucket.calculateStats(), nil
}

// GetLatencyTrend returns latency trend analysis over a time period
func (la *LatencyAggregator) GetLatencyTrend(ctx context.Context, operation, service string, startTime, endTime time.Time, interval time.Duration) (*LatencyTrend, error) {
	// Get data points from storage and memory
	dataPoints, err := la.getDataPoints(ctx, operation, service, startTime, endTime, interval)
	if err != nil {
		return nil, fmt.Errorf("failed to get data points: %w", err)
	}

	if len(dataPoints) < 2 {
		return &LatencyTrend{
			Operation: operation,
			Service:   service,
			TimeRange: fmt.Sprintf("%s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)),
			DataPoints: dataPoints,
			TrendAnalysis: TrendAnalysis{
				TrendDirection: "insufficient_data",
				ChangeClassification: "insufficient_data",
			},
		}, nil
	}

	// Calculate trend analysis
	analysis := la.calculateTrendAnalysis(dataPoints)
	
	// Calculate percentile trends
	percentileTrends := la.calculatePercentileTrends(dataPoints)

	return &LatencyTrend{
		Operation:     operation,
		Service:       service,
		TimeRange:     fmt.Sprintf("%s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)),
		DataPoints:    dataPoints,
		TrendAnalysis: analysis,
		Percentiles:   percentileTrends,
	}, nil
}

// GetAggregatedStats returns aggregated statistics for multiple operations
func (la *LatencyAggregator) GetAggregatedStats(service string, timeWindow time.Duration) (map[string]*LatencyStats, error) {
	la.mu.RLock()
	defer la.mu.RUnlock()

	results := make(map[string]*LatencyStats)
	cutoffTime := time.Now().Add(-timeWindow)

	for _, bucket := range la.buckets {
		if bucket.Service == service && bucket.WindowStart.After(cutoffTime) {
			stats := bucket.calculateStats()
			existing, exists := results[bucket.Operation]
			
			if !exists {
				results[bucket.Operation] = stats
			} else {
				// Merge stats
				results[bucket.Operation] = la.mergeStats(existing, stats)
			}
		}
	}

	return results, nil
}

// Private methods

func (la *LatencyAggregator) getBucketKey(operation, service string, timestamp time.Time) string {
	bucketStart := timestamp.Truncate(la.aggregateInterval)
	return fmt.Sprintf("%s:%s:%d", service, operation, bucketStart.Unix())
}

func (la *LatencyAggregator) createBucket(operation, service string, timestamp time.Time) *LatencyBucket {
	bucketStart := timestamp.Truncate(la.aggregateInterval)
	bucketEnd := bucketStart.Add(la.aggregateInterval)
	
	return &LatencyBucket{
		Operation:    operation,
		Service:      service,
		WindowStart:  bucketStart,
		WindowEnd:    bucketEnd,
		Measurements: make([]float64, 0),
		Min:          math.Inf(1),
		Max:          math.Inf(-1),
	}
}

func (lb *LatencyBucket) addMeasurement(value float64) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.Measurements = append(lb.Measurements, value)
	lb.Count++
	lb.Sum += value
	
	if value < lb.Min {
		lb.Min = value
	}
	if value > lb.Max {
		lb.Max = value
	}
}

func (lb *LatencyBucket) calculateStats() *LatencyStats {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if lb.Count == 0 {
		return &LatencyStats{
			Operation:   lb.Operation,
			Service:     lb.Service,
			WindowStart: lb.WindowStart,
			WindowEnd:   lb.WindowEnd,
		}
	}

	average := lb.Sum / float64(lb.Count)
	percentiles := calculatePercentiles(lb.Measurements)
	stdDev := calculateStdDev(lb.Measurements, average)

	return &LatencyStats{
		Operation:   lb.Operation,
		Service:     lb.Service,
		WindowStart: lb.WindowStart,
		WindowEnd:   lb.WindowEnd,
		Count:       lb.Count,
		Sum:         lb.Sum,
		Average:     average,
		Min:         lb.Min,
		Max:         lb.Max,
		Percentiles: percentiles,
		StdDev:      stdDev,
	}
}

func (la *LatencyAggregator) aggregationLoop() {
	ticker := time.NewTicker(la.aggregateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			la.aggregateAndFlush()
		case <-la.stopCh:
			return
		}
	}
}

func (la *LatencyAggregator) cleanupLoop() {
	ticker := time.NewTicker(time.Hour) // Clean up every hour
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			la.cleanup()
		case <-la.stopCh:
			return
		}
	}
}

func (la *LatencyAggregator) aggregateAndFlush() {
	la.mu.Lock()
	defer la.mu.Unlock()

	cutoffTime := time.Now().Add(-la.aggregateInterval * 2) // Keep current and previous window
	toFlush := make([]*LatencyBucket, 0)
	
	for key, bucket := range la.buckets {
		if bucket.WindowEnd.Before(cutoffTime) && bucket.Count > 0 {
			toFlush = append(toFlush, bucket)
			delete(la.buckets, key)
		}
	}

	// Flush aggregated data to storage
	for _, bucket := range toFlush {
		go la.flushBucket(bucket)
	}

	if err := common.ValidateSliceNotEmpty("toFlush", toFlush); err == nil {
		la.logger.Debug("flushed latency buckets",
			zap.Int("count", len(toFlush)),
			zap.Int("remaining_buckets", len(la.buckets)))
	}
}

func (la *LatencyAggregator) cleanup() {
	la.mu.Lock()
	defer la.mu.Unlock()

	cutoffTime := time.Now().Add(-la.retentionPeriod)
	removed := 0

	for key, bucket := range la.buckets {
		if bucket.WindowStart.Before(cutoffTime) {
			delete(la.buckets, key)
			removed++
		}
	}

	// If we still have too many buckets, remove the oldest
	if err := common.ValidateIntRange("buckets_length", len(la.buckets), la.maxBuckets+1, 10000); err == nil {
		// Convert to slice for sorting
		type bucketEntry struct {
			key    string
			bucket *LatencyBucket
		}
		
		entries := make([]bucketEntry, 0, len(la.buckets))
		for k, b := range la.buckets {
			entries = append(entries, bucketEntry{k, b})
		}
		
		// Sort by window start time
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].bucket.WindowStart.Before(entries[j].bucket.WindowStart)
		})
		
		// Remove oldest entries
		excessCount := len(la.buckets) - la.maxBuckets
		for i := 0; i < excessCount; i++ {
			delete(la.buckets, entries[i].key)
			removed++
		}
	}

	if removed > 0 {
		la.logger.Debug("cleaned up old latency buckets",
			zap.Int("removed", removed),
			zap.Int("remaining", len(la.buckets)))
	}
}

func (la *LatencyAggregator) flushBucket(bucket *LatencyBucket) {
	if la.metricsRecorder == nil {
		return
	}

	stats := bucket.calculateStats()

	// Create aggregated metric record
	if dmr, ok := la.metricsRecorder.(*DefaultMetricsRecorder); ok {
		metric := &models.MetricRecord{
			MetricType:       "latency_aggregated",
			ServiceName:      bucket.Service,
			Timestamp:        bucket.WindowStart,
			AggregationLevel: "5min", // Based on aggregation interval
			Unit:             "ms",
			Count:            stats.Count,
			Sum:              stats.Sum,
			Min:              stats.Min,
			Max:              stats.Max,
			P50:              stats.Percentiles["p50"],
			P95:              stats.Percentiles["p95"],
			P99:              stats.Percentiles["p99"],
		}

		metric.AddDimension("operation", bucket.Operation)
		metric.AddDimension("window_start", bucket.WindowStart.Format(time.RFC3339))
		metric.AddDimension("window_end", bucket.WindowEnd.Format(time.RFC3339))

		if err := dmr.createMetricFn(context.Background(), metric); err != nil {
			la.logger.Warn("failed to flush latency bucket",
				zap.String("operation", bucket.Operation),
				zap.String("service", bucket.Service),
				zap.Int64("count", bucket.Count),
				zap.Error(err))
		}
	}
}

func (la *LatencyAggregator) getDataPoints(_ context.Context, operation, service string, startTime, endTime time.Time, _ time.Duration) ([]LatencyDataPoint, error) {
	dataPoints := make([]LatencyDataPoint, 0)
	
	// Get data from memory buckets first
	la.mu.RLock()
	for _, bucket := range la.buckets {
		if bucket.Operation == operation && bucket.Service == service &&
			bucket.WindowStart.After(startTime) && bucket.WindowStart.Before(endTime) {
			
			stats := bucket.calculateStats()
			dataPoint := LatencyDataPoint{
				Timestamp:   bucket.WindowStart,
				Average:     stats.Average,
				Count:       stats.Count,
				Percentiles: stats.Percentiles,
			}
			dataPoints = append(dataPoints, dataPoint)
		}
	}
	la.mu.RUnlock()

	// TODO: Get historical data from storage if needed
	// This would query the MetricRecord repository for historical aggregated data

	// Sort by timestamp
	sort.Slice(dataPoints, func(i, j int) bool {
		return dataPoints[i].Timestamp.Before(dataPoints[j].Timestamp)
	})

	return dataPoints, nil
}

func (la *LatencyAggregator) calculateTrendAnalysis(dataPoints []LatencyDataPoint) TrendAnalysis {
	if len(dataPoints) < 2 {
		return TrendAnalysis{
			TrendDirection: "insufficient_data",
			ChangeClassification: "insufficient_data",
		}
	}

	// Calculate linear regression
	n := float64(len(dataPoints))
	var sumX, sumY, sumXY, sumX2 float64
	
	for i, point := range dataPoints {
		x := float64(i)
		y := point.Average
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Calculate slope and R-squared
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	
	yMean := sumY / n
	var totalSumSquares, residualSumSquares float64
	
	for i, point := range dataPoints {
		x := float64(i)
		y := point.Average
		predicted := slope*x + (yMean - slope*sumX/n)
		
		totalSumSquares += (y - yMean) * (y - yMean)
		residualSumSquares += (y - predicted) * (y - predicted)
	}
	
	rSquared := 1 - (residualSumSquares / totalSumSquares)
	if math.IsNaN(rSquared) {
		rSquared = 0
	}

	// Calculate percent change
	firstAvg := dataPoints[0].Average
	lastAvg := dataPoints[len(dataPoints)-1].Average
	percentChange := 0.0
	if firstAvg > 0 {
		percentChange = ((lastAvg - firstAvg) / firstAvg) * 100
	}

	// Calculate volatility (standard deviation of changes)
	var changes []float64
	for i := 1; i < len(dataPoints); i++ {
		change := dataPoints[i].Average - dataPoints[i-1].Average
		changes = append(changes, change)
	}
	volatility := calculateStdDev(changes, 0)

	// Determine trend direction
	var trendDirection string
	if math.Abs(slope) < 0.1 {
		trendDirection = TrendDirectionStable
	} else if slope > 0 {
		trendDirection = "increasing"
	} else {
		trendDirection = "decreasing"
	}

	// Determine significance and classification
	isSignificant := rSquared > 0.5 && math.Abs(percentChange) > 5 // 5% change threshold
	
	var changeClassification string
	if !isSignificant {
		changeClassification = "stable"
	} else if percentChange < -10 { // 10% improvement
		changeClassification = "significant_improvement"
	} else if percentChange > 10 { // 10% degradation
		changeClassification = "significant_degradation"
	} else {
		changeClassification = "stable"
	}

	return TrendAnalysis{
		Slope:                slope,
		RSquared:             rSquared,
		TrendDirection:       trendDirection,
		PercentChange:        percentChange,
		Volatility:           volatility,
		IsSignificant:        isSignificant,
		ChangeClassification: changeClassification,
	}
}

func (la *LatencyAggregator) calculatePercentileTrends(dataPoints []LatencyDataPoint) map[string][]float64 {
	percentileTrends := make(map[string][]float64)
	
	percentiles := []string{"p50", "p95", "p99"}
	for _, p := range percentiles {
		values := make([]float64, len(dataPoints))
		for i, point := range dataPoints {
			if point.Percentiles != nil {
				values[i] = point.Percentiles[p]
			}
		}
		percentileTrends[p] = values
	}
	
	return percentileTrends
}

func (la *LatencyAggregator) mergeStats(a, b *LatencyStats) *LatencyStats {
	totalCount := a.Count + b.Count
	if totalCount == 0 {
		return a
	}

	merged := &LatencyStats{
		Operation:   a.Operation,
		Service:     a.Service,
		WindowStart: a.WindowStart,
		WindowEnd:   b.WindowEnd,
		Count:       totalCount,
		Sum:         a.Sum + b.Sum,
		Average:     (a.Sum + b.Sum) / float64(totalCount),
		Min:         math.Min(a.Min, b.Min),
		Max:         math.Max(a.Max, b.Max),
		Percentiles: make(map[string]float64),
	}

	// Approximate percentile merging (simplified)
	if a.Percentiles != nil && b.Percentiles != nil {
		for p := range a.Percentiles {
			weightA := float64(a.Count) / float64(totalCount)
			weightB := float64(b.Count) / float64(totalCount)
			merged.Percentiles[p] = a.Percentiles[p]*weightA + b.Percentiles[p]*weightB
		}
	}

	return merged
}

// Utility functions

func calculatePercentiles(values []float64) map[string]float64 {
	if err := common.ValidateSliceNotEmpty("values", values); err != nil {
		return map[string]float64{"p50": 0, "p90": 0, "p95": 0, "p99": 0}
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	return map[string]float64{
		"p50": getPercentile(sorted, 50),
		"p90": getPercentile(sorted, 90),
		"p95": getPercentile(sorted, 95),
		"p99": getPercentile(sorted, 99),
	}
}

func getPercentile(sorted []float64, percentile float64) float64 {
	if err := common.ValidateSliceNotEmpty("sorted", sorted); err != nil {
		return 0
	}
	
	index := percentile / 100 * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	
	if lower == upper {
		return sorted[lower]
	}
	
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func calculateStdDev(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}

	var sum float64
	for _, value := range values {
		diff := value - mean
		sum += diff * diff
	}

	variance := sum / float64(len(values))
	return math.Sqrt(variance)
}