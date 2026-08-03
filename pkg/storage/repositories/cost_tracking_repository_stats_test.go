package repositories

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// mean() and standardDeviation() Tests
// ============================================================================

func TestMean_EmptySlice(t *testing.T) {
	result := mean([]float64{})
	require.Equal(t, 0.0, result)
}

func TestMean_SingleValue(t *testing.T) {
	result := mean([]float64{5.0})
	require.Equal(t, 5.0, result)
}

func TestMean_MultipleValues(t *testing.T) {
	result := mean([]float64{1.0, 2.0, 3.0, 4.0, 5.0})
	require.Equal(t, 3.0, result)
}

func TestMean_NegativeValues(t *testing.T) {
	result := mean([]float64{-1.0, 1.0})
	require.Equal(t, 0.0, result)
}

func TestStandardDeviation_EmptySlice(t *testing.T) {
	result := standardDeviation([]float64{}, 0)
	require.Equal(t, 0.0, result)
}

func TestStandardDeviation_SingleValue(t *testing.T) {
	result := standardDeviation([]float64{5.0}, 5.0)
	require.Equal(t, 0.0, result)
}

func TestStandardDeviation_ConstantValues(t *testing.T) {
	values := []float64{5.0, 5.0, 5.0, 5.0}
	m := mean(values)
	result := standardDeviation(values, m)
	require.Equal(t, 0.0, result)
}

func TestStandardDeviation_KnownValues(t *testing.T) {
	// Standard deviation of [2, 4, 4, 4, 5, 5, 7, 9] with sample formula
	values := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	m := mean(values)
	result := standardDeviation(values, m)
	// Using sample std dev formula (n-1), expect approximately 2.14
	require.InDelta(t, 2.14, result, 0.1)
}

// ============================================================================
// approximateTTestPValue Tests
// ============================================================================

func TestApproximateTTestPValue(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewTrackingRepository(mockDB, "test-table", logger, nil)

	tests := []struct {
		name     string
		tStat    float64
		df       int
		expected float64
	}{
		{
			name:     "large df, high t-stat > 2.576",
			tStat:    3.0,
			df:       50,
			expected: 0.01,
		},
		{
			name:     "large df, t-stat > 1.96",
			tStat:    2.0,
			df:       50,
			expected: 0.05,
		},
		{
			name:     "large df, t-stat > 1.645",
			tStat:    1.7,
			df:       50,
			expected: 0.10,
		},
		{
			name:     "large df, low t-stat",
			tStat:    1.0,
			df:       50,
			expected: 0.20,
		},
		{
			name:     "small df, high t-stat > 3.0",
			tStat:    3.5,
			df:       10,
			expected: 0.01,
		},
		{
			name:     "small df, t-stat > 2.0",
			tStat:    2.5,
			df:       10,
			expected: 0.05,
		},
		{
			name:     "small df, t-stat > 1.5",
			tStat:    1.7,
			df:       10,
			expected: 0.10,
		},
		{
			name:     "small df, low t-stat",
			tStat:    1.0,
			df:       10,
			expected: 0.20,
		},
		{
			name:     "negative t-stat uses absolute value",
			tStat:    -3.0,
			df:       50,
			expected: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.approximateTTestPValue(tt.tStat, tt.df)
			require.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// mannKendallTest Tests
// ============================================================================

func TestMannKendallTest_TooFewValues(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	tau, pValue := repo.mannKendallTest([]float64{1.0, 2.0})

	require.Equal(t, 0.0, tau)
	require.Equal(t, 1.0, pValue)
}

func TestMannKendallTest_IncreasingTrend(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Strongly increasing series
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	tau, pValue := repo.mannKendallTest(values)

	// Tau should be positive for increasing trend
	require.Greater(t, tau, 0.0)
	// P-value should be small (significant)
	require.LessOrEqual(t, pValue, 0.05)
}

func TestMannKendallTest_DecreasingTrend(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Strongly decreasing series
	values := []float64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	tau, pValue := repo.mannKendallTest(values)

	// Tau should be negative for decreasing trend
	require.Less(t, tau, 0.0)
	// P-value should be small (significant)
	require.LessOrEqual(t, pValue, 0.05)
}

func TestMannKendallTest_ConstantSeries(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Constant series - no trend
	values := []float64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}
	tau, _ := repo.mannKendallTest(values)

	// Tau should be 0 for constant series
	require.Equal(t, 0.0, tau)
}

// ============================================================================
// theilSenSlope Tests
// ============================================================================

func TestTheilSenSlope_SingleValue(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	result := repo.theilSenSlope([]float64{5.0})
	require.Equal(t, 0.0, result)
}

func TestTheilSenSlope_TwoValues(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	result := repo.theilSenSlope([]float64{1.0, 3.0})
	require.Equal(t, 2.0, result)
}

func TestTheilSenSlope_LinearIncrease(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Perfect linear increase with slope 1
	values := []float64{1, 2, 3, 4, 5}
	result := repo.theilSenSlope(values)

	require.InDelta(t, 1.0, result, 0.01)
}

func TestTheilSenSlope_LinearDecrease(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Perfect linear decrease with slope -2
	values := []float64{10, 8, 6, 4, 2}
	result := repo.theilSenSlope(values)

	require.InDelta(t, -2.0, result, 0.01)
}

func TestTheilSenSlope_ConstantSeries(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	values := []float64{5, 5, 5, 5, 5}
	result := repo.theilSenSlope(values)

	require.Equal(t, 0.0, result)
}

// ============================================================================
// durbinWatsonTest Tests
// ============================================================================

func TestDurbinWatsonTest_SingleValue(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	result := repo.durbinWatsonTest([]float64{5.0})
	require.Equal(t, 2.0, result) // Default no autocorrelation
}

func TestDurbinWatsonTest_NoAutocorrelation(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Random-like residuals should give DW close to 2
	residuals := []float64{0.5, -0.3, 0.1, -0.4, 0.2, -0.1, 0.3, -0.2}
	result := repo.durbinWatsonTest(residuals)

	// DW statistic around 2 indicates no autocorrelation
	require.InDelta(t, 2.0, result, 1.0)
}

func TestDurbinWatsonTest_PositiveAutocorrelation(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Residuals with positive autocorrelation (similar consecutive values)
	residuals := []float64{1.0, 0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.3}
	result := repo.durbinWatsonTest(residuals)

	// DW < 2 indicates positive autocorrelation
	require.Less(t, result, 2.0)
}

func TestDurbinWatsonTest_ZeroDenominator(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// All zeros
	residuals := []float64{0, 0, 0, 0}
	result := repo.durbinWatsonTest(residuals)

	require.Equal(t, 2.0, result) // Default when denominator is 0
}

// ============================================================================
// jarqueBeraTest Tests
// ============================================================================

func TestJarqueBeraTest_TooFewValues(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	jb, pValue := repo.jarqueBeraTest([]float64{1.0, 2.0, 3.0})

	require.Equal(t, 0.0, jb)
	require.Equal(t, 1.0, pValue)
}

func TestJarqueBeraTest_ConstantSeries(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	values := []float64{5, 5, 5, 5, 5, 5, 5, 5}
	jb, pValue := repo.jarqueBeraTest(values)

	// Constant series has stdDev = 0, returns early
	require.Equal(t, 0.0, jb)
	require.Equal(t, 1.0, pValue)
}

func TestJarqueBeraTest_NormalDistribution(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Approximately normal distribution (symmetric, no heavy tails)
	values := []float64{-2, -1, -0.5, 0, 0.5, 1, 2}
	jb, pValue := repo.jarqueBeraTest(values)

	// Normal distribution should have low JB and high p-value
	require.GreaterOrEqual(t, jb, 0.0)
	require.Greater(t, pValue, 0.0)
}

func TestJarqueBeraTest_HighSkewness(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Highly skewed distribution
	values := []float64{1, 1, 1, 1, 1, 1, 1, 100}
	jb, _ := repo.jarqueBeraTest(values)

	// High JB indicates non-normality
	require.Greater(t, jb, 0.0)
}

// ============================================================================
// analyzeResiduals Tests
// ============================================================================

func TestAnalyzeResiduals_TooFewValues(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	result := repo.analyzeResiduals([]float64{1.0, 2.0})

	require.Nil(t, result)
}

func TestAnalyzeResiduals_SymmetricResiduals(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Symmetric residuals around 0
	residuals := []float64{-2, -1, 0, 1, 2}
	result := repo.analyzeResiduals(residuals)

	require.NotNil(t, result)
	require.InDelta(t, 0, result.Mean, 0.01)
	require.InDelta(t, 0, result.Skewness, 0.5) // Low skewness
	require.Equal(t, 0, result.OutlierCount)    // No outliers
	require.Greater(t, result.NormalityScore, 0.5)
}

func TestAnalyzeResiduals_SkewedResiduals(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Skewed residuals
	residuals := []float64{0, 0, 0, 0, 10, 20}
	result := repo.analyzeResiduals(residuals)

	require.NotNil(t, result)
	require.Greater(t, math.Abs(result.Skewness), 0.0)
}

func TestAnalyzeResiduals_WithOutliers(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Residuals with clear outliers (beyond 2 std deviations)
	residuals := []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 100}
	result := repo.analyzeResiduals(residuals)

	require.NotNil(t, result)
	require.GreaterOrEqual(t, result.OutlierCount, 1)
}

// ============================================================================
// calculateLinearRegressionStats Tests
// ============================================================================

func TestCalculateLinearRegressionStats_TooFewPoints(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	dataPoints := []CostDataPoint{
		{Timestamp: time.Now(), CostDollars: 1.0},
	}

	result := repo.calculateLinearRegressionStats(dataPoints)

	require.Nil(t, result)
}

func TestCalculateLinearRegressionStats_PerfectLinearIncrease(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now()
	dataPoints := []CostDataPoint{
		{Timestamp: now.Add(-4 * time.Hour), CostDollars: 1.0},
		{Timestamp: now.Add(-3 * time.Hour), CostDollars: 2.0},
		{Timestamp: now.Add(-2 * time.Hour), CostDollars: 3.0},
		{Timestamp: now.Add(-1 * time.Hour), CostDollars: 4.0},
		{Timestamp: now, CostDollars: 5.0},
	}

	result := repo.calculateLinearRegressionStats(dataPoints)

	require.NotNil(t, result)
	require.Greater(t, result.Slope, 0.0)
	require.InDelta(t, 1.0, result.RSquared, 0.01)
	require.Equal(t, "increasing", result.TrendDirection)
}

func TestCalculateLinearRegressionStats_PerfectLinearDecrease(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now()
	dataPoints := []CostDataPoint{
		{Timestamp: now.Add(-4 * time.Hour), CostDollars: 5.0},
		{Timestamp: now.Add(-3 * time.Hour), CostDollars: 4.0},
		{Timestamp: now.Add(-2 * time.Hour), CostDollars: 3.0},
		{Timestamp: now.Add(-1 * time.Hour), CostDollars: 2.0},
		{Timestamp: now, CostDollars: 1.0},
	}

	result := repo.calculateLinearRegressionStats(dataPoints)

	require.NotNil(t, result)
	require.Less(t, result.Slope, 0.0)
	require.InDelta(t, 1.0, result.RSquared, 0.01)
	require.Equal(t, "decreasing", result.TrendDirection)
}

func TestCalculateLinearRegressionStats_ConstantValues(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now()
	dataPoints := []CostDataPoint{
		{Timestamp: now.Add(-4 * time.Hour), CostDollars: 5.0},
		{Timestamp: now.Add(-3 * time.Hour), CostDollars: 5.0},
		{Timestamp: now.Add(-2 * time.Hour), CostDollars: 5.0},
		{Timestamp: now.Add(-1 * time.Hour), CostDollars: 5.0},
		{Timestamp: now, CostDollars: 5.0},
	}

	result := repo.calculateLinearRegressionStats(dataPoints)

	require.NotNil(t, result)
	require.Equal(t, 0.0, result.Slope)
	require.Equal(t, "stable", result.TrendDirection)
}

// ============================================================================
// calculateStatisticalTests Tests
// ============================================================================

func TestCalculateStatisticalTests_NilRegression(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	result := repo.calculateStatisticalTests(nil, nil)

	require.Nil(t, result)
}

func TestCalculateStatisticalTests_TooFewPoints(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	dataPoints := []CostDataPoint{
		{CostDollars: 1.0},
		{CostDollars: 2.0},
	}
	regression := &LinearRegressionStats{Slope: 1.0, Intercept: 0.0}

	result := repo.calculateStatisticalTests(dataPoints, regression)

	require.Nil(t, result)
}

func TestCalculateStatisticalTests_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now()
	dataPoints := []CostDataPoint{
		{Timestamp: now.Add(-4 * time.Hour), CostDollars: 1.0},
		{Timestamp: now.Add(-3 * time.Hour), CostDollars: 2.0},
		{Timestamp: now.Add(-2 * time.Hour), CostDollars: 3.0},
		{Timestamp: now.Add(-1 * time.Hour), CostDollars: 4.0},
		{Timestamp: now, CostDollars: 5.0},
	}
	regression := &LinearRegressionStats{Slope: 1.0, Intercept: 1.0}

	result := repo.calculateStatisticalTests(dataPoints, regression)

	require.NotNil(t, result)
	require.NotEqual(t, 0.0, result.TheilSenSlope)
	require.GreaterOrEqual(t, result.DurbinWatson, 0.0)
	require.LessOrEqual(t, result.DurbinWatson, 4.0)
}

// ============================================================================
// detectCostAnomalies Tests
// ============================================================================

func TestDetectCostAnomalies_NilRegression(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	result := repo.detectCostAnomalies(nil, nil)

	require.Nil(t, result)
}

func TestDetectCostAnomalies_TooFewPoints(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	dataPoints := []CostDataPoint{
		{CostDollars: 1.0},
		{CostDollars: 2.0},
		{CostDollars: 3.0},
		{CostDollars: 4.0},
	}
	regression := &LinearRegressionStats{Slope: 1.0, Intercept: 0.0}

	result := repo.detectCostAnomalies(dataPoints, regression)

	require.Nil(t, result)
}

func TestDetectCostAnomalies_NoAnomalies(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now()
	// Perfect linear data - no anomalies
	dataPoints := []CostDataPoint{
		{Timestamp: now.Add(-6 * time.Hour), CostDollars: 1.0},
		{Timestamp: now.Add(-5 * time.Hour), CostDollars: 2.0},
		{Timestamp: now.Add(-4 * time.Hour), CostDollars: 3.0},
		{Timestamp: now.Add(-3 * time.Hour), CostDollars: 4.0},
		{Timestamp: now.Add(-2 * time.Hour), CostDollars: 5.0},
		{Timestamp: now.Add(-1 * time.Hour), CostDollars: 6.0},
		{Timestamp: now, CostDollars: 7.0},
	}
	regression := &LinearRegressionStats{Slope: 1.0, Intercept: 1.0}

	result := repo.detectCostAnomalies(dataPoints, regression)

	// No anomalies in perfect linear data
	require.Empty(t, result)
}

func TestDetectCostAnomalies_WithSpike(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now()
	// Data with a clear spike
	dataPoints := []CostDataPoint{
		{Timestamp: now.Add(-6 * time.Hour), CostDollars: 1.0},
		{Timestamp: now.Add(-5 * time.Hour), CostDollars: 1.0},
		{Timestamp: now.Add(-4 * time.Hour), CostDollars: 1.0},
		{Timestamp: now.Add(-3 * time.Hour), CostDollars: 100.0}, // Spike
		{Timestamp: now.Add(-2 * time.Hour), CostDollars: 1.0},
		{Timestamp: now.Add(-1 * time.Hour), CostDollars: 1.0},
		{Timestamp: now, CostDollars: 1.0},
	}
	regression := &LinearRegressionStats{Slope: 0.0, Intercept: 1.0}

	result := repo.detectCostAnomalies(dataPoints, regression)

	// Should detect at least one anomaly
	require.NotEmpty(t, result)
	// The anomaly should be identified as a spike
	found := false
	for _, anomaly := range result {
		if anomaly.AnomalyType == "spike" {
			found = true
			require.Equal(t, 100.0, anomaly.ActualCost)
		}
	}
	require.True(t, found, "Should have detected the spike")
}

// ============================================================================
// generateCostForecast Tests
// ============================================================================

func TestGenerateCostForecast_NilRegression(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	result := repo.generateCostForecast(nil, nil)

	require.Nil(t, result)
}

func TestGenerateCostForecast_TooFewPoints(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	dataPoints := []CostDataPoint{
		{CostDollars: 1.0},
		{CostDollars: 2.0},
	}
	regression := &LinearRegressionStats{Slope: 1.0, Intercept: 0.0}

	result := repo.generateCostForecast(dataPoints, regression)

	require.Nil(t, result)
}

func TestGenerateCostForecast_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now()
	dataPoints := []CostDataPoint{
		{Timestamp: now.Add(-4 * time.Hour), CostDollars: 1.0},
		{Timestamp: now.Add(-3 * time.Hour), CostDollars: 2.0},
		{Timestamp: now.Add(-2 * time.Hour), CostDollars: 3.0},
		{Timestamp: now.Add(-1 * time.Hour), CostDollars: 4.0},
		{Timestamp: now, CostDollars: 5.0},
	}
	regression := &LinearRegressionStats{Slope: 1.0, Intercept: 1.0}

	result := repo.generateCostForecast(dataPoints, regression)

	require.NotNil(t, result)
	require.Equal(t, 7, result.ForecastHorizon)
	require.Len(t, result.Predictions, 7)
	require.Equal(t, 0.95, result.ConfidenceLevel)
	require.Equal(t, "linear", result.ModelType)

	// Verify predictions are in the future
	for _, pred := range result.Predictions {
		require.True(t, pred.Timestamp.After(now))
		require.GreaterOrEqual(t, pred.LowerBound, 0.0)
		require.GreaterOrEqual(t, pred.UpperBound, pred.LowerBound)
	}
}

// ============================================================================
// simpleSeasonalDecomposition Tests
// ============================================================================

func TestSimpleSeasonalDecomposition_BasicDecomposition(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Simple data with period 3
	values := []float64{10, 15, 20, 11, 16, 21, 12, 17, 22}
	period := 3

	trend, seasonal, residual := repo.simpleSeasonalDecomposition(values, period)

	// All outputs should have same length as input
	require.Len(t, trend, len(values))
	require.Len(t, seasonal, len(values))
	require.Len(t, residual, len(values))

	// Sum of components should approximately equal original
	for i := range values {
		reconstructed := trend[i] + seasonal[i] + residual[i]
		require.InDelta(t, values[i], reconstructed, 0.01)
	}
}

// ============================================================================
// calculateDecompositionR2 Tests
// ============================================================================

func TestCalculateDecompositionR2_EmptyInput(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	result := repo.calculateDecompositionR2([]float64{}, []float64{}, []float64{})

	require.Equal(t, 0.0, result)
}

func TestCalculateDecompositionR2_PerfectFit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	original := []float64{10, 20, 30, 40, 50}
	trend := []float64{10, 20, 30, 40, 50}
	seasonal := []float64{0, 0, 0, 0, 0}

	result := repo.calculateDecompositionR2(original, trend, seasonal)

	require.InDelta(t, 1.0, result, 0.01)
}

func TestCalculateDecompositionR2_ConstantOriginal(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// When original is constant, ssTotal = 0
	original := []float64{5, 5, 5, 5, 5}
	trend := []float64{5, 5, 5, 5, 5}
	seasonal := []float64{0, 0, 0, 0, 0}

	result := repo.calculateDecompositionR2(original, trend, seasonal)

	require.Equal(t, 0.0, result)
}

// ============================================================================
// extractSeasonalPatterns Tests
// ============================================================================

func TestExtractSeasonalPatterns(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	seasonal := []float64{1, 2, 3, 1, 2, 3, 1, 2, 3}
	period := 3

	result := repo.extractSeasonalPatterns(seasonal, period)

	require.Len(t, result, period)
	require.InDelta(t, 1.0, result["period_0"], 0.01)
	require.InDelta(t, 2.0, result["period_1"], 0.01)
	require.InDelta(t, 3.0, result["period_2"], 0.01)
}

// ============================================================================
// calculateSeasonalStrength Tests
// ============================================================================

func TestCalculateSeasonalStrength_InsufficientData(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Less than 2*period data points
	values := []float64{1, 2, 3, 4, 5}
	period := 3

	result := repo.calculateSeasonalStrength(values, period)

	require.Equal(t, 0.0, result)
}

func TestCalculateSeasonalStrength_StrongSeasonality(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Strong seasonal pattern with period 3
	values := []float64{1, 2, 3, 1, 2, 3, 1, 2, 3}
	period := 3

	result := repo.calculateSeasonalStrength(values, period)

	require.Greater(t, result, 0.0)
	require.LessOrEqual(t, result, 1.0)
}

func TestCalculateSeasonalStrength_NoSeasonality(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Constant values - no seasonality
	values := []float64{5, 5, 5, 5, 5, 5, 5, 5, 5}
	period := 3

	result := repo.calculateSeasonalStrength(values, period)

	require.Equal(t, 0.0, result)
}
