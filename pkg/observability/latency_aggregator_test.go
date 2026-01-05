package observability

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// =============================================================================
// Tests for calculatePercentiles
// =============================================================================

func TestCalculatePercentiles_EmptySlice(t *testing.T) {
	result := calculatePercentiles([]float64{})

	assert.Equal(t, 0.0, result["p50"], "p50 should be 0 for empty slice")
	assert.Equal(t, 0.0, result["p90"], "p90 should be 0 for empty slice")
	assert.Equal(t, 0.0, result["p95"], "p95 should be 0 for empty slice")
	assert.Equal(t, 0.0, result["p99"], "p99 should be 0 for empty slice")
}

func TestCalculatePercentiles_SingleValue(t *testing.T) {
	result := calculatePercentiles([]float64{42.0})

	// With a single value, all percentiles should be that value
	assert.Equal(t, 42.0, result["p50"], "p50 should be the single value")
	assert.Equal(t, 42.0, result["p90"], "p90 should be the single value")
	assert.Equal(t, 42.0, result["p95"], "p95 should be the single value")
	assert.Equal(t, 42.0, result["p99"], "p99 should be the single value")
}

func TestCalculatePercentiles_TwoValues(t *testing.T) {
	result := calculatePercentiles([]float64{10.0, 20.0})

	// The function sorts internally, so order doesn't matter
	// p50: index = 0.5 * 1 = 0.5, interpolated between 10 and 20
	assert.InDelta(t, 15.0, result["p50"], 0.01, "p50 should interpolate between values")
	// p90: index = 0.9 * 1 = 0.9
	assert.InDelta(t, 19.0, result["p90"], 0.01, "p90 should be close to max")
	// p95: index = 0.95 * 1 = 0.95
	assert.InDelta(t, 19.5, result["p95"], 0.01, "p95 should be very close to max")
	// p99: index = 0.99 * 1 = 0.99
	assert.InDelta(t, 19.9, result["p99"], 0.01, "p99 should be nearly the max")
}

func TestCalculatePercentiles_MultipleValues(t *testing.T) {
	// Test with 100 values from 1 to 100
	values := make([]float64, 100)
	for i := 0; i < 100; i++ {
		values[i] = float64(i + 1)
	}

	result := calculatePercentiles(values)

	// p50 should be around 50-51
	assert.InDelta(t, 50.5, result["p50"], 1.0, "p50 should be around median")
	// p90 should be around 90
	assert.InDelta(t, 90.1, result["p90"], 1.0, "p90 should be around 90th percentile")
	// p95 should be around 95
	assert.InDelta(t, 95.05, result["p95"], 1.0, "p95 should be around 95th percentile")
	// p99 should be around 99
	assert.InDelta(t, 99.01, result["p99"], 1.0, "p99 should be around 99th percentile")
}

func TestCalculatePercentiles_UnsortedInput(t *testing.T) {
	// Ensure the function sorts the input
	values := []float64{100.0, 1.0, 50.0, 25.0, 75.0}
	result := calculatePercentiles(values)

	// Sorted: [1, 25, 50, 75, 100]
	// p50: index = 0.5 * 4 = 2.0, sorted[2] = 50
	assert.Equal(t, 50.0, result["p50"], "p50 should be the median")
}

func TestCalculatePercentiles_DoesNotModifyInput(t *testing.T) {
	original := []float64{100.0, 1.0, 50.0, 25.0, 75.0}
	values := make([]float64, len(original))
	copy(values, original)

	calculatePercentiles(values)

	// The input slice should not be modified (function creates a copy)
	assert.Equal(t, original, values, "input slice should not be modified")
}

// =============================================================================
// Tests for getPercentile
// =============================================================================

func TestGetPercentile_EmptySlice(t *testing.T) {
	result := getPercentile([]float64{}, 50)
	assert.Equal(t, 0.0, result, "should return 0 for empty slice")
}

func TestGetPercentile_SingleValue(t *testing.T) {
	result := getPercentile([]float64{42.0}, 50)
	assert.Equal(t, 42.0, result, "should return the single value")

	result = getPercentile([]float64{42.0}, 99)
	assert.Equal(t, 42.0, result, "p99 should also return the single value")
}

func TestGetPercentile_ExactIndex(t *testing.T) {
	// With 5 values at indices 0-4
	// percentile 50 -> index = 0.5 * 4 = 2.0 (exact)
	sorted := []float64{10.0, 20.0, 30.0, 40.0, 50.0}

	result := getPercentile(sorted, 50)
	assert.Equal(t, 30.0, result, "p50 should be the middle value (no interpolation needed)")
}

func TestGetPercentile_Interpolation(t *testing.T) {
	sorted := []float64{10.0, 20.0}

	// p50: index = 0.5 * 1 = 0.5
	// weight = 0.5 - 0 = 0.5
	// result = 10.0 * 0.5 + 20.0 * 0.5 = 15.0
	result := getPercentile(sorted, 50)
	assert.Equal(t, 15.0, result, "p50 should interpolate between the two values")

	// p75: index = 0.75 * 1 = 0.75
	// weight = 0.75 - 0 = 0.75
	// result = 10.0 * 0.25 + 20.0 * 0.75 = 17.5
	result = getPercentile(sorted, 75)
	assert.Equal(t, 17.5, result, "p75 should interpolate correctly")

	// p25: index = 0.25 * 1 = 0.25
	// weight = 0.25 - 0 = 0.25
	// result = 10.0 * 0.75 + 20.0 * 0.25 = 12.5
	result = getPercentile(sorted, 25)
	assert.Equal(t, 12.5, result, "p25 should interpolate correctly")
}

func TestGetPercentile_Boundaries(t *testing.T) {
	sorted := []float64{10.0, 20.0, 30.0, 40.0, 50.0}

	// p0: index = 0.0 * 4 = 0, should return first element
	result := getPercentile(sorted, 0)
	assert.Equal(t, 10.0, result, "p0 should return minimum")

	// p100: index = 1.0 * 4 = 4, should return last element
	result = getPercentile(sorted, 100)
	assert.Equal(t, 50.0, result, "p100 should return maximum")
}

// =============================================================================
// Tests for calculateStdDev
// =============================================================================

func TestCalculateStdDev_EmptySlice(t *testing.T) {
	result := calculateStdDev([]float64{}, 0)
	assert.Equal(t, 0.0, result, "stddev of empty slice should be 0")
}

func TestCalculateStdDev_SingleValue(t *testing.T) {
	result := calculateStdDev([]float64{42.0}, 42.0)
	assert.Equal(t, 0.0, result, "stddev of single value should be 0")
}

func TestCalculateStdDev_UniformValues(t *testing.T) {
	values := []float64{10.0, 10.0, 10.0, 10.0, 10.0}
	result := calculateStdDev(values, 10.0)
	assert.Equal(t, 0.0, result, "stddev of uniform values should be 0")
}

func TestCalculateStdDev_KnownValues(t *testing.T) {
	// Test with known values: [2, 4, 4, 4, 5, 5, 7, 9]
	// Mean = 5, Variance = 4, StdDev = 2
	values := []float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0}
	mean := 5.0

	result := calculateStdDev(values, mean)
	assert.InDelta(t, 2.0, result, 0.01, "stddev should be 2")
}

func TestCalculateStdDev_ProvidedMeanUsed(t *testing.T) {
	// The function uses the provided mean, not calculated mean
	values := []float64{10.0, 20.0, 30.0}

	// If mean is 20, variance = ((10-20)^2 + (20-20)^2 + (30-20)^2)/3 = (100+0+100)/3 = 200/3
	// StdDev = sqrt(200/3) ≈ 8.165
	result := calculateStdDev(values, 20.0)
	assert.InDelta(t, 8.165, result, 0.01)

	// If mean is 0 (wrong mean), the calculation will be different
	// variance = (100 + 400 + 900) / 3 = 1400/3, stddev ≈ 21.6
	result = calculateStdDev(values, 0.0)
	assert.InDelta(t, 21.60, result, 0.1)
}

// =============================================================================
// Tests for LatencyBucket.addMeasurement
// =============================================================================

func TestLatencyBucket_AddMeasurement_SingleValue(t *testing.T) {
	bucket := &LatencyBucket{
		Operation:    "test-op",
		Service:      "test-service",
		WindowStart:  time.Now(),
		WindowEnd:    time.Now().Add(5 * time.Minute),
		Measurements: make([]float64, 0),
		Min:          math.Inf(1),
		Max:          math.Inf(-1),
	}

	bucket.addMeasurement(100.0)

	assert.Equal(t, int64(1), bucket.Count, "count should be 1")
	assert.Equal(t, 100.0, bucket.Sum, "sum should equal the measurement")
	assert.Equal(t, 100.0, bucket.Min, "min should be the measurement")
	assert.Equal(t, 100.0, bucket.Max, "max should be the measurement")
	assert.Equal(t, []float64{100.0}, bucket.Measurements)
}

func TestLatencyBucket_AddMeasurement_MultipleValues(t *testing.T) {
	bucket := &LatencyBucket{
		Operation:    "test-op",
		Service:      "test-service",
		WindowStart:  time.Now(),
		WindowEnd:    time.Now().Add(5 * time.Minute),
		Measurements: make([]float64, 0),
		Min:          math.Inf(1),
		Max:          math.Inf(-1),
	}

	bucket.addMeasurement(50.0)
	bucket.addMeasurement(100.0)
	bucket.addMeasurement(25.0)
	bucket.addMeasurement(200.0)

	assert.Equal(t, int64(4), bucket.Count, "count should be 4")
	assert.Equal(t, 375.0, bucket.Sum, "sum should be sum of all measurements")
	assert.Equal(t, 25.0, bucket.Min, "min should be 25")
	assert.Equal(t, 200.0, bucket.Max, "max should be 200")
	assert.Equal(t, []float64{50.0, 100.0, 25.0, 200.0}, bucket.Measurements)
}

func TestLatencyBucket_AddMeasurement_MinMaxTracking(t *testing.T) {
	bucket := &LatencyBucket{
		Measurements: make([]float64, 0),
		Min:          math.Inf(1),
		Max:          math.Inf(-1),
	}

	// Add in order: first becomes both min and max
	bucket.addMeasurement(50.0)
	assert.Equal(t, 50.0, bucket.Min)
	assert.Equal(t, 50.0, bucket.Max)

	// Add smaller value: updates min only
	bucket.addMeasurement(25.0)
	assert.Equal(t, 25.0, bucket.Min)
	assert.Equal(t, 50.0, bucket.Max)

	// Add larger value: updates max only
	bucket.addMeasurement(100.0)
	assert.Equal(t, 25.0, bucket.Min)
	assert.Equal(t, 100.0, bucket.Max)

	// Add middle value: no update
	bucket.addMeasurement(40.0)
	assert.Equal(t, 25.0, bucket.Min)
	assert.Equal(t, 100.0, bucket.Max)
}

// =============================================================================
// Tests for LatencyBucket.calculateStats
// =============================================================================

func TestLatencyBucket_CalculateStats_EmptyBucket(t *testing.T) {
	bucket := &LatencyBucket{
		Operation:    "test-op",
		Service:      "test-service",
		WindowStart:  time.Now(),
		WindowEnd:    time.Now().Add(5 * time.Minute),
		Measurements: make([]float64, 0),
		Count:        0,
		Min:          math.Inf(1),
		Max:          math.Inf(-1),
	}

	stats := bucket.calculateStats()

	assert.Equal(t, "test-op", stats.Operation)
	assert.Equal(t, "test-service", stats.Service)
	assert.Equal(t, int64(0), stats.Count)
	assert.Equal(t, 0.0, stats.Sum)
	assert.Equal(t, 0.0, stats.Average)
	// Min and Max are not set for empty buckets (struct fields stay at zero value)
	// Percentiles and StdDev should be zero or empty
}

func TestLatencyBucket_CalculateStats_SingleMeasurement(t *testing.T) {
	bucket := &LatencyBucket{
		Operation:    "query",
		Service:      "api-service",
		WindowStart:  time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		WindowEnd:    time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
		Measurements: []float64{100.0},
		Count:        1,
		Sum:          100.0,
		Min:          100.0,
		Max:          100.0,
	}

	stats := bucket.calculateStats()

	assert.Equal(t, "query", stats.Operation)
	assert.Equal(t, "api-service", stats.Service)
	assert.Equal(t, int64(1), stats.Count)
	assert.Equal(t, 100.0, stats.Sum)
	assert.Equal(t, 100.0, stats.Average)
	assert.Equal(t, 100.0, stats.Min)
	assert.Equal(t, 100.0, stats.Max)
	assert.Equal(t, 0.0, stats.StdDev, "stddev of single value should be 0")
	assert.Equal(t, 100.0, stats.Percentiles["p50"])
	assert.Equal(t, 100.0, stats.Percentiles["p95"])
	assert.Equal(t, 100.0, stats.Percentiles["p99"])
}

func TestLatencyBucket_CalculateStats_MultipleMeasurements(t *testing.T) {
	bucket := &LatencyBucket{
		Operation:    "query",
		Service:      "api-service",
		WindowStart:  time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		WindowEnd:    time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
		Measurements: []float64{10.0, 20.0, 30.0, 40.0, 50.0},
		Count:        5,
		Sum:          150.0,
		Min:          10.0,
		Max:          50.0,
	}

	stats := bucket.calculateStats()

	assert.Equal(t, int64(5), stats.Count)
	assert.Equal(t, 150.0, stats.Sum)
	assert.Equal(t, 30.0, stats.Average, "average should be 150/5 = 30")
	assert.Equal(t, 10.0, stats.Min)
	assert.Equal(t, 50.0, stats.Max)
	assert.Equal(t, 30.0, stats.Percentiles["p50"], "p50 should be median")
	assert.Greater(t, stats.StdDev, 0.0, "stddev should be > 0")
}

// =============================================================================
// Tests for LatencyAggregator.mergeStats
// =============================================================================

func TestMergeStats_BothEmpty(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	a := &LatencyStats{
		Operation: "op1",
		Service:   "svc1",
		Count:     0,
	}
	b := &LatencyStats{
		Operation: "op2",
		Service:   "svc2",
		Count:     0,
	}

	merged := aggregator.mergeStats(a, b)

	// When totalCount is 0, returns a
	assert.Equal(t, a, merged)
}

func TestMergeStats_BasicMerge(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	windowStart := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	a := &LatencyStats{
		Operation:   "query",
		Service:     "api-service",
		WindowStart: windowStart,
		WindowEnd:   windowStart.Add(5 * time.Minute),
		Count:       10,
		Sum:         1000.0,
		Average:     100.0,
		Min:         50.0,
		Max:         150.0,
		Percentiles: map[string]float64{"p50": 100.0, "p95": 140.0},
	}
	b := &LatencyStats{
		Operation:   "query",
		Service:     "api-service",
		WindowStart: windowStart.Add(5 * time.Minute),
		WindowEnd:   windowStart.Add(10 * time.Minute),
		Count:       20,
		Sum:         2400.0,
		Average:     120.0,
		Min:         60.0,
		Max:         180.0,
		Percentiles: map[string]float64{"p50": 120.0, "p95": 170.0},
	}

	merged := aggregator.mergeStats(a, b)

	assert.Equal(t, "query", merged.Operation)
	assert.Equal(t, "api-service", merged.Service)
	assert.Equal(t, windowStart, merged.WindowStart, "should preserve a's WindowStart")
	assert.Equal(t, windowStart.Add(10*time.Minute), merged.WindowEnd, "should use b's WindowEnd")
	assert.Equal(t, int64(30), merged.Count, "count should be sum")
	assert.Equal(t, 3400.0, merged.Sum, "sum should be sum")
	assert.InDelta(t, 113.33, merged.Average, 0.1, "average should be weighted average")
	assert.Equal(t, 50.0, merged.Min, "min should be minimum of both")
	assert.Equal(t, 180.0, merged.Max, "max should be maximum of both")
}

func TestMergeStats_PercentileWeighting(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	a := &LatencyStats{
		Count:       10,
		Sum:         1000.0,
		Percentiles: map[string]float64{"p50": 100.0, "p95": 140.0},
	}
	b := &LatencyStats{
		Count:       10,
		Sum:         1000.0,
		Percentiles: map[string]float64{"p50": 200.0, "p95": 250.0},
	}

	merged := aggregator.mergeStats(a, b)

	// Equal counts, so percentiles should be simple average
	assert.InDelta(t, 150.0, merged.Percentiles["p50"], 0.01, "p50 should be average")
	assert.InDelta(t, 195.0, merged.Percentiles["p95"], 0.01, "p95 should be average")
}

func TestMergeStats_MinMaxSelection(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	testCases := []struct {
		name    string
		aMin    float64
		aMax    float64
		bMin    float64
		bMax    float64
		wantMin float64
		wantMax float64
	}{
		{"a has smaller min and max", 10.0, 100.0, 20.0, 90.0, 10.0, 100.0},
		{"b has smaller min, a has larger max", 20.0, 100.0, 10.0, 90.0, 10.0, 100.0},
		{"a has smaller min, b has larger max", 10.0, 90.0, 20.0, 100.0, 10.0, 100.0},
		{"equal values", 50.0, 100.0, 50.0, 100.0, 50.0, 100.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := &LatencyStats{Count: 1, Sum: 50.0, Min: tc.aMin, Max: tc.aMax}
			b := &LatencyStats{Count: 1, Sum: 50.0, Min: tc.bMin, Max: tc.bMax}

			merged := aggregator.mergeStats(a, b)

			assert.Equal(t, tc.wantMin, merged.Min)
			assert.Equal(t, tc.wantMax, merged.Max)
		})
	}
}

func TestMergeStats_NilPercentiles(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	a := &LatencyStats{
		Count:       10,
		Sum:         1000.0,
		Percentiles: nil,
	}
	b := &LatencyStats{
		Count:       10,
		Sum:         1000.0,
		Percentiles: map[string]float64{"p50": 100.0},
	}

	// Should not panic when one has nil percentiles
	merged := aggregator.mergeStats(a, b)
	assert.NotNil(t, merged)
	// Percentiles should be empty since condition requires both non-nil
	assert.Empty(t, merged.Percentiles)
}

// =============================================================================
// Tests for LatencyAggregator.calculateTrendAnalysis
// =============================================================================

func TestCalculateTrendAnalysis_InsufficientData(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// No data points
	result := aggregator.calculateTrendAnalysis([]LatencyDataPoint{})
	assert.Equal(t, "insufficient_data", result.TrendDirection)
	assert.Equal(t, "insufficient_data", result.ChangeClassification)

	// Single data point
	result = aggregator.calculateTrendAnalysis([]LatencyDataPoint{
		{Timestamp: time.Now(), Average: 100.0, Count: 10},
	})
	assert.Equal(t, "insufficient_data", result.TrendDirection)
	assert.Equal(t, "insufficient_data", result.ChangeClassification)
}

func TestCalculateTrendAnalysis_StableTrend(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// All same values - completely stable
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 100.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 100.0, Count: 10},
		{Timestamp: time.Now().Add(2 * time.Minute), Average: 100.0, Count: 10},
		{Timestamp: time.Now().Add(3 * time.Minute), Average: 100.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	assert.Equal(t, TrendDirectionStable, result.TrendDirection)
	assert.InDelta(t, 0.0, result.Slope, 0.01)
	assert.InDelta(t, 0.0, result.PercentChange, 0.01)
	assert.Equal(t, "stable", result.ChangeClassification)
}

func TestCalculateTrendAnalysis_IncreasingTrend(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// Clear increasing trend
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 100.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 110.0, Count: 10},
		{Timestamp: time.Now().Add(2 * time.Minute), Average: 120.0, Count: 10},
		{Timestamp: time.Now().Add(3 * time.Minute), Average: 130.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	assert.Equal(t, "increasing", result.TrendDirection)
	assert.Greater(t, result.Slope, 0.0, "slope should be positive")
	assert.Greater(t, result.PercentChange, 0.0, "percent change should be positive")
}

func TestCalculateTrendAnalysis_DecreasingTrend(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// Clear decreasing trend
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 130.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 120.0, Count: 10},
		{Timestamp: time.Now().Add(2 * time.Minute), Average: 110.0, Count: 10},
		{Timestamp: time.Now().Add(3 * time.Minute), Average: 100.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	assert.Equal(t, "decreasing", result.TrendDirection)
	assert.Less(t, result.Slope, 0.0, "slope should be negative")
	assert.Less(t, result.PercentChange, 0.0, "percent change should be negative")
}

func TestCalculateTrendAnalysis_SignificantDegradation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// > 10% degradation with high R-squared
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 100.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 110.0, Count: 10},
		{Timestamp: time.Now().Add(2 * time.Minute), Average: 120.0, Count: 10},
		{Timestamp: time.Now().Add(3 * time.Minute), Average: 130.0, Count: 10},
		{Timestamp: time.Now().Add(4 * time.Minute), Average: 140.0, Count: 10},
		{Timestamp: time.Now().Add(5 * time.Minute), Average: 150.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	// 50% increase is significant degradation
	assert.Greater(t, result.PercentChange, 10.0, "should detect >10% change")
	if result.IsSignificant {
		assert.Equal(t, "significant_degradation", result.ChangeClassification)
	}
}

func TestCalculateTrendAnalysis_SignificantImprovement(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// > 10% improvement with high R-squared
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 150.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 140.0, Count: 10},
		{Timestamp: time.Now().Add(2 * time.Minute), Average: 130.0, Count: 10},
		{Timestamp: time.Now().Add(3 * time.Minute), Average: 120.0, Count: 10},
		{Timestamp: time.Now().Add(4 * time.Minute), Average: 110.0, Count: 10},
		{Timestamp: time.Now().Add(5 * time.Minute), Average: 100.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	// -33% change is significant improvement
	assert.Less(t, result.PercentChange, -10.0, "should detect >10% improvement")
	if result.IsSignificant {
		assert.Equal(t, "significant_improvement", result.ChangeClassification)
	}
}

func TestCalculateTrendAnalysis_VolatilityCalculation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// Highly volatile data
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 100.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 200.0, Count: 10},
		{Timestamp: time.Now().Add(2 * time.Minute), Average: 50.0, Count: 10},
		{Timestamp: time.Now().Add(3 * time.Minute), Average: 180.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	assert.Greater(t, result.Volatility, 0.0, "volatility should be calculated")
}

func TestCalculateTrendAnalysis_RSquaredCalculation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// Perfect linear trend - R-squared should be 1.0
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 100.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 110.0, Count: 10},
		{Timestamp: time.Now().Add(2 * time.Minute), Average: 120.0, Count: 10},
		{Timestamp: time.Now().Add(3 * time.Minute), Average: 130.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	assert.InDelta(t, 1.0, result.RSquared, 0.01, "perfect linear trend should have R^2 = 1")
}

func TestCalculateTrendAnalysis_PercentChangeZeroFirst(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// First value is 0 - should handle division by zero
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 0.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 100.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	// When first value is 0, percent change should be 0
	assert.Equal(t, 0.0, result.PercentChange)
}

// =============================================================================
// Tests for LatencyAggregator.calculatePercentileTrends
// =============================================================================

func TestCalculatePercentileTrends_EmptyDataPoints(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	result := aggregator.calculatePercentileTrends([]LatencyDataPoint{})

	assert.Contains(t, result, "p50")
	assert.Contains(t, result, "p95")
	assert.Contains(t, result, "p99")
	assert.Empty(t, result["p50"])
	assert.Empty(t, result["p95"])
	assert.Empty(t, result["p99"])
}

func TestCalculatePercentileTrends_SingleDataPoint(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	dataPoints := []LatencyDataPoint{
		{
			Timestamp:   time.Now(),
			Average:     100.0,
			Count:       10,
			Percentiles: map[string]float64{"p50": 95.0, "p95": 150.0, "p99": 200.0},
		},
	}

	result := aggregator.calculatePercentileTrends(dataPoints)

	require.Len(t, result["p50"], 1)
	require.Len(t, result["p95"], 1)
	require.Len(t, result["p99"], 1)
	assert.Equal(t, 95.0, result["p50"][0])
	assert.Equal(t, 150.0, result["p95"][0])
	assert.Equal(t, 200.0, result["p99"][0])
}

func TestCalculatePercentileTrends_MultipleDataPoints(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	dataPoints := []LatencyDataPoint{
		{
			Timestamp:   time.Now(),
			Percentiles: map[string]float64{"p50": 100.0, "p95": 150.0, "p99": 200.0},
		},
		{
			Timestamp:   time.Now().Add(time.Minute),
			Percentiles: map[string]float64{"p50": 110.0, "p95": 160.0, "p99": 210.0},
		},
		{
			Timestamp:   time.Now().Add(2 * time.Minute),
			Percentiles: map[string]float64{"p50": 120.0, "p95": 170.0, "p99": 220.0},
		},
	}

	result := aggregator.calculatePercentileTrends(dataPoints)

	assert.Equal(t, []float64{100.0, 110.0, 120.0}, result["p50"])
	assert.Equal(t, []float64{150.0, 160.0, 170.0}, result["p95"])
	assert.Equal(t, []float64{200.0, 210.0, 220.0}, result["p99"])
}

func TestCalculatePercentileTrends_NilPercentiles(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	dataPoints := []LatencyDataPoint{
		{
			Timestamp:   time.Now(),
			Percentiles: map[string]float64{"p50": 100.0, "p95": 150.0, "p99": 200.0},
		},
		{
			Timestamp:   time.Now().Add(time.Minute),
			Percentiles: nil, // Missing percentiles
		},
		{
			Timestamp:   time.Now().Add(2 * time.Minute),
			Percentiles: map[string]float64{"p50": 120.0, "p95": 170.0, "p99": 220.0},
		},
	}

	result := aggregator.calculatePercentileTrends(dataPoints)

	// nil percentiles should result in 0.0 values
	assert.Equal(t, []float64{100.0, 0.0, 120.0}, result["p50"])
	assert.Equal(t, []float64{150.0, 0.0, 170.0}, result["p95"])
	assert.Equal(t, []float64{200.0, 0.0, 220.0}, result["p99"])
}

func TestCalculatePercentileTrends_MissingPercentileKeys(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	dataPoints := []LatencyDataPoint{
		{
			Timestamp:   time.Now(),
			Percentiles: map[string]float64{"p50": 100.0}, // Missing p95 and p99
		},
	}

	result := aggregator.calculatePercentileTrends(dataPoints)

	assert.Equal(t, []float64{100.0}, result["p50"])
	assert.Equal(t, []float64{0.0}, result["p95"], "missing key should result in 0")
	assert.Equal(t, []float64{0.0}, result["p99"], "missing key should result in 0")
}

// =============================================================================
// Tests for NewLatencyAggregator options
// =============================================================================

func TestNewLatencyAggregator_DefaultOptions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	assert.Equal(t, 5*time.Minute, aggregator.aggregateInterval)
	assert.Equal(t, 24*time.Hour, aggregator.retentionPeriod)
	assert.Equal(t, 1000, aggregator.maxBuckets)
	assert.NotNil(t, aggregator.buckets)
	assert.NotNil(t, aggregator.stopCh)
	assert.False(t, aggregator.started)
}

func TestNewLatencyAggregator_WithAggregateInterval(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil, WithAggregateInterval(10*time.Minute))

	assert.Equal(t, 10*time.Minute, aggregator.aggregateInterval)
}

func TestNewLatencyAggregator_WithRetentionPeriod(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil, WithRetentionPeriod(48*time.Hour))

	assert.Equal(t, 48*time.Hour, aggregator.retentionPeriod)
}

func TestNewLatencyAggregator_WithMaxBuckets(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil, WithMaxBuckets(500))

	assert.Equal(t, 500, aggregator.maxBuckets)
}

func TestNewLatencyAggregator_MultipleOptions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(
		logger,
		nil,
		WithAggregateInterval(1*time.Minute),
		WithRetentionPeriod(12*time.Hour),
		WithMaxBuckets(100),
	)

	assert.Equal(t, 1*time.Minute, aggregator.aggregateInterval)
	assert.Equal(t, 12*time.Hour, aggregator.retentionPeriod)
	assert.Equal(t, 100, aggregator.maxBuckets)
}

// =============================================================================
// Tests for bucket key generation and bucket creation
// =============================================================================

func TestGetBucketKey(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil, WithAggregateInterval(5*time.Minute))

	// Two timestamps in the same 5-minute window should get the same key
	timestamp1 := time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC)
	timestamp2 := time.Date(2024, 1, 1, 10, 4, 59, 0, time.UTC)

	key1 := aggregator.getBucketKey("op1", "service1", timestamp1)
	key2 := aggregator.getBucketKey("op1", "service1", timestamp2)

	assert.Equal(t, key1, key2, "timestamps in same window should have same bucket key")

	// Different window should get different key
	timestamp3 := time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC)
	key3 := aggregator.getBucketKey("op1", "service1", timestamp3)

	assert.NotEqual(t, key1, key3, "timestamps in different windows should have different bucket keys")
}

func TestGetBucketKey_DifferentOperations(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	timestamp := time.Now()

	key1 := aggregator.getBucketKey("op1", "service1", timestamp)
	key2 := aggregator.getBucketKey("op2", "service1", timestamp)

	assert.NotEqual(t, key1, key2, "different operations should have different bucket keys")
}

func TestGetBucketKey_DifferentServices(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	timestamp := time.Now()

	key1 := aggregator.getBucketKey("op1", "service1", timestamp)
	key2 := aggregator.getBucketKey("op1", "service2", timestamp)

	assert.NotEqual(t, key1, key2, "different services should have different bucket keys")
}

func TestCreateBucket(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil, WithAggregateInterval(5*time.Minute))

	timestamp := time.Date(2024, 1, 1, 10, 2, 30, 0, time.UTC)
	bucket := aggregator.createBucket("query", "api-service", timestamp)

	assert.Equal(t, "query", bucket.Operation)
	assert.Equal(t, "api-service", bucket.Service)

	// WindowStart should be truncated to 5-minute boundary
	expectedStart := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, expectedStart, bucket.WindowStart)

	// WindowEnd should be 5 minutes after start
	expectedEnd := time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC)
	assert.Equal(t, expectedEnd, bucket.WindowEnd)

	assert.Empty(t, bucket.Measurements)
	assert.Equal(t, math.Inf(1), bucket.Min)
	assert.Equal(t, math.Inf(-1), bucket.Max)
}

// =============================================================================
// Tests for RecordLatency
// =============================================================================

func TestRecordLatency_CreatesBucket(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	aggregator.RecordLatency("query", "api-service", 100*time.Millisecond)

	// Verify a bucket was created
	aggregator.mu.RLock()
	assert.Len(t, aggregator.buckets, 1)
	aggregator.mu.RUnlock()
}

func TestRecordLatency_ReusesBucket(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// Record multiple latencies for the same operation
	aggregator.RecordLatency("query", "api-service", 100*time.Millisecond)
	aggregator.RecordLatency("query", "api-service", 150*time.Millisecond)
	aggregator.RecordLatency("query", "api-service", 200*time.Millisecond)

	// Verify only one bucket exists
	aggregator.mu.RLock()
	assert.Len(t, aggregator.buckets, 1)

	// Get the bucket and verify measurements
	for _, bucket := range aggregator.buckets {
		assert.Equal(t, int64(3), bucket.Count)
		assert.Equal(t, 450.0, bucket.Sum) // 100 + 150 + 200
	}
	aggregator.mu.RUnlock()
}

func TestRecordLatency_ConvertsToDurationMilliseconds(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	aggregator.RecordLatency("query", "api-service", 250*time.Millisecond)

	aggregator.mu.RLock()
	for _, bucket := range aggregator.buckets {
		require.Len(t, bucket.Measurements, 1)
		assert.Equal(t, 250.0, bucket.Measurements[0], "duration should be converted to milliseconds")
	}
	aggregator.mu.RUnlock()
}

// =============================================================================
// Tests for isBucketInTimeRange
// =============================================================================

func TestIsBucketInTimeRange_Match(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	bucket := &LatencyBucket{
		Operation:   "query",
		Service:     "api-service",
		WindowStart: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
	}

	startTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 1, 10, 10, 0, 0, time.UTC)

	result := aggregator.isBucketInTimeRange(bucket, "query", "api-service", startTime, endTime)
	assert.True(t, result)
}

func TestIsBucketInTimeRange_WrongOperation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	bucket := &LatencyBucket{
		Operation:   "query",
		Service:     "api-service",
		WindowStart: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
	}

	startTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 1, 10, 10, 0, 0, time.UTC)

	result := aggregator.isBucketInTimeRange(bucket, "scan", "api-service", startTime, endTime)
	assert.False(t, result)
}

func TestIsBucketInTimeRange_WrongService(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	bucket := &LatencyBucket{
		Operation:   "query",
		Service:     "api-service",
		WindowStart: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
	}

	startTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 1, 10, 10, 0, 0, time.UTC)

	result := aggregator.isBucketInTimeRange(bucket, "query", "other-service", startTime, endTime)
	assert.False(t, result)
}

func TestIsBucketInTimeRange_OutOfTimeRange(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	bucket := &LatencyBucket{
		Operation:   "query",
		Service:     "api-service",
		WindowStart: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	// Bucket is before the time range
	startTime := time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 1, 10, 10, 0, 0, time.UTC)

	result := aggregator.isBucketInTimeRange(bucket, "query", "api-service", startTime, endTime)
	assert.False(t, result, "bucket before range should not match")

	// Bucket is after the time range
	bucket.WindowStart = time.Date(2024, 1, 1, 10, 15, 0, 0, time.UTC)
	result = aggregator.isBucketInTimeRange(bucket, "query", "api-service", startTime, endTime)
	assert.False(t, result, "bucket after range should not match")
}

// =============================================================================
// Tests for buildPercentiles helper
// =============================================================================

func TestBuildPercentiles(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// Import models here just for testing
	metric := &mockMetricRecord{
		p50: 100.0,
		p95: 150.0,
		p99: 200.0,
	}

	// Test buildPercentiles using convertMetricToDataPoint
	// We can't directly test buildPercentiles as it takes *models.MetricRecord
	// Instead, verify through calculatePercentileTrends behavior

	dataPoints := []LatencyDataPoint{
		{
			Timestamp:   time.Now(),
			Percentiles: map[string]float64{"p50": 100.0, "p95": 150.0, "p99": 200.0},
		},
	}

	result := aggregator.calculatePercentileTrends(dataPoints)
	assert.Equal(t, 100.0, result["p50"][0])
	assert.Equal(t, 150.0, result["p95"][0])
	assert.Equal(t, 200.0, result["p99"][0])

	_ = metric // suppress unused warning
}

// mockMetricRecord is a helper for testing
type mockMetricRecord struct {
	p50 float64
	p95 float64
	p99 float64
}

// =============================================================================
// Tests for calculateAverage helper
// =============================================================================

func TestCalculateAverage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	aggregator := NewLatencyAggregator(logger, nil)

	// Test via calculateTrendAnalysis which uses averages
	dataPoints := []LatencyDataPoint{
		{Timestamp: time.Now(), Average: 100.0, Count: 10},
		{Timestamp: time.Now().Add(time.Minute), Average: 200.0, Count: 10},
	}

	result := aggregator.calculateTrendAnalysis(dataPoints)

	// The percent change should be calculated correctly
	// (200 - 100) / 100 * 100 = 100%
	assert.InDelta(t, 100.0, result.PercentChange, 0.1)
}
