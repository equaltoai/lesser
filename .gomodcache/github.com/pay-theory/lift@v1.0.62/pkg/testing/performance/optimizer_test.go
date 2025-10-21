package performance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func TestPerformanceOptimizer_OptimizePerformance(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := PerformanceConfig{
		MonitoringInterval:  1 * time.Second,
		BenchmarkTimeout:    10 * time.Second,
		RegressionThreshold: 0.1,
		AlertThresholds: AlertThresholds{
			ResponseTime: 100 * time.Millisecond,
			Throughput:   100.0,
			ErrorRate:    0.05,
			MemoryUsage:  0.8,
			CPUUsage:     0.8,
		},
		TrendAnalysisWindow: 5 * time.Minute,
		OptimizationLevel:   OptimizationLevelStandard,
		EnablePredictive:    true,
		EnableAutoOptimize:  false,
	}

	optimizer := NewPerformanceOptimizer(config)

	// Add test monitor
	monitor := NewTestPerformanceMonitor()
	optimizer.AddMonitor(monitor)

	// Add test benchmark
	benchmark := NewTestBenchmark()
	optimizer.AddBenchmark(benchmark)

	// Add test analyzer
	analyzer := NewTestPerformanceAnalyzer()
	optimizer.AddAnalyzer(analyzer)

	ctx := context.Background()
	result, err := optimizer.OptimizePerformance(ctx, server.URL)

	if err != nil {
		t.Fatalf("OptimizePerformance failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if result.Target != server.URL {
		t.Errorf("Expected target %s, got %s", server.URL, result.Target)
	}

	if len(result.Monitoring) == 0 {
		t.Error("Expected monitoring results, got none")
	}

	if len(result.Benchmarks) == 0 {
		t.Error("Expected benchmark results, got none")
	}

	if len(result.Analysis) == 0 {
		t.Error("Expected analysis results, got none")
	}

	if result.PerformanceScore.Overall < 0 || result.PerformanceScore.Overall > 100 {
		t.Errorf("Expected performance score between 0-100, got %f", result.PerformanceScore.Overall)
	}

	t.Logf("Performance optimization completed successfully")
	t.Logf("Overall Score: %.2f", result.PerformanceScore.Overall)
	t.Logf("Response Time Score: %.2f", result.PerformanceScore.ResponseTime)
	t.Logf("Throughput Score: %.2f", result.PerformanceScore.Throughput)
	t.Logf("Reliability Score: %.2f", result.PerformanceScore.Reliability)
}

// Test Performance Monitor Implementation
type TestPerformanceMonitor struct {
	metrics    PerformanceMetrics
	monitoring bool
}

func NewTestPerformanceMonitor() *TestPerformanceMonitor {
	return &TestPerformanceMonitor{
		metrics: PerformanceMetrics{
			Timestamp:    time.Now(),
			ResponseTime: 50 * time.Millisecond,
			Throughput:   150.0,
			ErrorRate:    0.02,
			MemoryUsage: MemoryMetrics{
				Used:       1024 * 1024 * 100,  // 100MB
				Available:  1024 * 1024 * 900,  // 900MB
				Total:      1024 * 1024 * 1000, // 1GB
				Percentage: 10.0,
			},
			CPUUsage: CPUMetrics{
				Usage: 25.0,
				Cores: runtime.NumCPU(),
			},
			RequestCount: 1000,
			ErrorCount:   20,
			ActiveUsers:  50,
		},
	}
}

func (t *TestPerformanceMonitor) StartMonitoring(_ context.Context, _ string) error {
	t.monitoring = true
	return nil
}

func (t *TestPerformanceMonitor) StopMonitoring() error {
	t.monitoring = false
	return nil
}

func (t *TestPerformanceMonitor) GetMetrics(_ context.Context) (PerformanceMetrics, error) {
	return t.metrics, nil
}

func (t *TestPerformanceMonitor) DetectRegression(current, baseline PerformanceMetrics) bool {
	responseTimeDiff := float64(current.ResponseTime-baseline.ResponseTime) / float64(baseline.ResponseTime)
	return responseTimeDiff > 0.1 // 10% regression threshold
}

func (t *TestPerformanceMonitor) GetThresholds() PerformanceThresholds {
	return PerformanceThresholds{
		MaxResponseTime: 100 * time.Millisecond,
		MinThroughput:   100.0,
		MaxErrorRate:    0.05,
		MaxMemoryUsage:  0.8,
		MaxCPUUsage:     0.8,
	}
}

func (t *TestPerformanceMonitor) SetThresholds(_ PerformanceThresholds) error {
	return nil
}

func TestPerformanceOptimizer_AutoOptimize(t *testing.T) {
	config := PerformanceConfig{
		BenchmarkTimeout:   time.Second,
		EnableAutoOptimize: true,
	}

	optimizer := NewPerformanceOptimizer(config)
	optimizer.AddMonitor(NewTestPerformanceMonitor())

	successful := &TestOptimizer{optimizationType: OptimizationTypeCPU}
	failing := &TestOptimizer{optimizationType: OptimizationTypeMemory, shouldError: true}

	optimizer.AddOptimizer(successful)
	optimizer.AddOptimizer(failing)

	result, err := optimizer.OptimizePerformance(context.Background(), "service-A")
	if err != nil {
		t.Fatalf("OptimizePerformance failed: %v", err)
	}

	if !successful.called {
		t.Fatal("expected successful optimizer to be invoked")
	}

	if _, ok := result.Optimizations[string(OptimizationTypeCPU)]; !ok {
		t.Fatal("expected optimization results for CPU optimizer")
	}

	if _, ok := result.Optimizations[string(OptimizationTypeMemory)]; ok {
		t.Fatal("did not expect results for failing optimizer")
	}

	if len(result.Errors) == 0 {
		t.Fatal("expected optimizer error to be recorded")
	}
}

// Test Benchmark Implementation
type TestBenchmark struct {
	baseline BenchmarkResult
}

func NewTestBenchmark() *TestBenchmark {
	return &TestBenchmark{
		baseline: BenchmarkResult{
			StartTime:      time.Now().Add(-1 * time.Hour),
			EndTime:        time.Now().Add(-1*time.Hour + 5*time.Minute),
			Duration:       5 * time.Minute,
			TotalRequests:  10000,
			SuccessfulReqs: 9950,
			FailedRequests: 50,
			ResponseTimes: ResponseTimeStats{
				Min:    10 * time.Millisecond,
				Max:    200 * time.Millisecond,
				Mean:   50 * time.Millisecond,
				Median: 45 * time.Millisecond,
				P95:    80 * time.Millisecond,
				P99:    120 * time.Millisecond,
			},
			Throughput: ThroughputStats{
				RequestsPerSecond: 33.3,
				BytesPerSecond:    1024 * 33.3,
				Peak:              50.0,
				Average:           33.3,
			},
			ErrorStats: ErrorStats{
				TotalErrors: 50,
				ErrorRate:   0.005,
			},
		},
	}
}

func (t *TestBenchmark) Run(_ context.Context, config BenchmarkConfig) (BenchmarkResult, error) {
	// Simulate benchmark execution
	result := BenchmarkResult{
		Config:         config,
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(config.Duration),
		Duration:       config.Duration,
		TotalRequests:  1000,
		SuccessfulReqs: 980,
		FailedRequests: 20,
		ResponseTimes: ResponseTimeStats{
			Min:    8 * time.Millisecond,
			Max:    150 * time.Millisecond,
			Mean:   45 * time.Millisecond,
			Median: 40 * time.Millisecond,
			P95:    75 * time.Millisecond,
			P99:    110 * time.Millisecond,
		},
		Throughput: ThroughputStats{
			RequestsPerSecond: 166.7,
			BytesPerSecond:    1024 * 166.7,
			Peak:              200.0,
			Average:           166.7,
		},
		ErrorStats: ErrorStats{
			TotalErrors: 20,
			ErrorRate:   0.02,
		},
		ResourceUsage: ResourceUsageStats{
			Memory: MemoryUsageStats{
				Peak:    1024 * 1024 * 150, // 150MB
				Average: 1024 * 1024 * 120, // 120MB
			},
			CPU: CPUUsageStats{
				Peak:    45.0,
				Average: 30.0,
				Cores:   runtime.NumCPU(),
			},
		},
	}

	return result, nil
}

func (t *TestBenchmark) GetBaseline() BenchmarkResult {
	return t.baseline
}

func (t *TestBenchmark) SetBaseline(baseline BenchmarkResult) error {
	t.baseline = baseline
	return nil
}

type TestOptimizer struct {
	optimizationType OptimizationType
	shouldError      bool
	called           bool
}

func (t *TestOptimizer) Optimize(_ context.Context, target OptimizationTarget) (OptimizationResult, error) {
	t.called = true
	if t.shouldError {
		return OptimizationResult{}, fmt.Errorf("optimizer %s failed", t.optimizationType)
	}

	return OptimizationResult{
		Timestamp: time.Now(),
		Target:    target,
		Success:   true,
	}, nil
}

func (t *TestOptimizer) GetOptimizationType() OptimizationType {
	return t.optimizationType
}

func (t *TestOptimizer) EstimateImpact(_ OptimizationTarget) ImpactEstimate {
	return ImpactEstimate{}
}

func (t *TestOptimizer) ValidateOptimization(_ OptimizationResult) ValidationResult {
	return ValidationResult{Valid: true}
}

func (t *TestBenchmark) Compare(current, baseline BenchmarkResult) ComparisonResult {
	improvements := []Improvement{}
	regressions := []Regression{}

	// Compare response times
	responseTimeChange := float64(current.ResponseTimes.Mean-baseline.ResponseTimes.Mean) / float64(baseline.ResponseTimes.Mean)
	if responseTimeChange < -0.05 { // 5% improvement
		improvements = append(improvements, Improvement{
			Metric:     "response_time",
			OldValue:   float64(baseline.ResponseTimes.Mean.Milliseconds()),
			NewValue:   float64(current.ResponseTimes.Mean.Milliseconds()),
			Change:     responseTimeChange,
			Percentage: responseTimeChange * 100,
		})
	} else if responseTimeChange > 0.05 { // 5% regression
		regressions = append(regressions, Regression{
			Metric:     "response_time",
			OldValue:   float64(baseline.ResponseTimes.Mean.Milliseconds()),
			NewValue:   float64(current.ResponseTimes.Mean.Milliseconds()),
			Change:     responseTimeChange,
			Percentage: responseTimeChange * 100,
			Severity:   RegressionSeverityMinor,
		})
	}

	// Compare throughput
	throughputChange := (current.Throughput.RequestsPerSecond - baseline.Throughput.RequestsPerSecond) / baseline.Throughput.RequestsPerSecond
	if throughputChange > 0.05 { // 5% improvement
		improvements = append(improvements, Improvement{
			Metric:     "throughput",
			OldValue:   baseline.Throughput.RequestsPerSecond,
			NewValue:   current.Throughput.RequestsPerSecond,
			Change:     throughputChange,
			Percentage: throughputChange * 100,
		})
	}

	overallChange := PerformanceChangeImproved
	if len(regressions) > len(improvements) {
		overallChange = PerformanceChangeRegressed
	} else if len(improvements) == 0 && len(regressions) == 0 {
		overallChange = PerformanceChangeUnchanged
	}

	return ComparisonResult{
		Baseline:      baseline,
		Current:       current,
		Improvements:  improvements,
		Regressions:   regressions,
		OverallChange: overallChange,
		Significance: StatisticalSignificance{
			PValue:      0.05,
			Confidence:  0.95,
			Significant: len(improvements)+len(regressions) > 0,
			TestType:    "t-test",
		},
	}
}

func (t *TestBenchmark) GenerateReport() BenchmarkReport {
	return BenchmarkReport{
		Summary: BenchmarkSummary{
			TotalBenchmarks: 1,
			Improvements:    1,
			Regressions:     0,
			Unchanged:       0,
			OverallScore:    85.0,
		},
		Timestamp: time.Now(),
		Version:   "1.0.0",
	}
}

// Test Performance Analyzer Implementation
type TestPerformanceAnalyzer struct{}

func NewTestPerformanceAnalyzer() *TestPerformanceAnalyzer {
	return &TestPerformanceAnalyzer{}
}

func (t *TestPerformanceAnalyzer) Analyze(_ context.Context, metrics []PerformanceMetrics) (AnalysisResult, error) {
	bottlenecks := []Bottleneck{}
	patterns := []PerformancePattern{}
	anomalies := []Anomaly{}
	trends := []Trend{}
	recommendations := []Recommendation{}

	// Analyze for bottlenecks
	if len(metrics) > 0 {
		metric := metrics[0]

		if metric.CPUUsage.Usage > 80.0 {
			bottlenecks = append(bottlenecks, Bottleneck{
				Type:        BottleneckTypeCPU,
				Component:   "application",
				Severity:    BottleneckSeverityHigh,
				Impact:      ImpactLevelHigh,
				Description: "High CPU utilization detected",
				Metrics:     map[string]float64{"cpu_usage": metric.CPUUsage.Usage},
				Suggestions: []string{"Optimize CPU-intensive operations", "Consider horizontal scaling"},
				Priority:    1,
			})
		}

		if metric.MemoryUsage.Percentage > 80.0 {
			bottlenecks = append(bottlenecks, Bottleneck{
				Type:        BottleneckTypeMemory,
				Component:   "application",
				Severity:    BottleneckSeverityMedium,
				Impact:      ImpactLevelModerate,
				Description: "High memory utilization detected",
				Metrics:     map[string]float64{"memory_usage": metric.MemoryUsage.Percentage},
				Suggestions: []string{"Optimize memory usage", "Implement memory pooling"},
				Priority:    2,
			})
		}

		if metric.ResponseTime > 100*time.Millisecond {
			bottlenecks = append(bottlenecks, Bottleneck{
				Type:        BottleneckTypeAPI,
				Component:   "api_endpoint",
				Severity:    BottleneckSeverityMedium,
				Impact:      ImpactLevelModerate,
				Description: "High response time detected",
				Metrics:     map[string]float64{"response_time_ms": float64(metric.ResponseTime.Milliseconds())},
				Suggestions: []string{"Optimize database queries", "Implement caching"},
				Priority:    3,
			})
		}
	}

	// Generate recommendations based on analysis
	if len(bottlenecks) > 0 {
		recommendations = append(recommendations, Recommendation{
			ID:          "PERF-001",
			Type:        RecommendationTypeOptimization,
			Priority:    RecommendationPriorityHigh,
			Title:       "Address Performance Bottlenecks",
			Description: "Multiple performance bottlenecks detected that require immediate attention",
			Impact: ImpactEstimate{
				Performance: 0.3,                // 30% improvement expected
				Cost:        0.1,                // 10% cost increase
				Risk:        0.2,                // 20% risk
				Timeline:    2 * 24 * time.Hour, // 2 days
				Confidence:  0.8,                // 80% confidence
			},
			Effort: EffortEstimate{
				Hours:      16.0,
				Complexity: ComplexityLevelMedium,
				Skills:     []string{"performance optimization", "profiling"},
				Resources:  []string{"development team", "performance tools"},
			},
			Steps: []RecommendationStep{
				{Order: 1, Description: "Profile application performance", Action: "Run performance profiler"},
				{Order: 2, Description: "Identify bottlenecks", Action: "Analyze profiling results"},
				{Order: 3, Description: "Implement optimizations", Action: "Apply performance fixes"},
				{Order: 4, Description: "Validate improvements", Action: "Run performance tests"},
			},
			Tags: []string{"performance", "optimization", "bottleneck"},
		})
	}

	return AnalysisResult{
		Timestamp:    time.Now(),
		AnalysisType: AnalysisTypeRealTime,
		Bottlenecks:  bottlenecks,
		Patterns:     patterns,
		Anomalies:    anomalies,
		Trends:       trends,
		Score: PerformanceScore{
			Overall:      75.0,
			ResponseTime: 70.0,
			Throughput:   80.0,
			Reliability:  85.0,
			Efficiency:   65.0,
			Scalability:  75.0,
		},
		Recommendations: recommendations,
	}, nil
}

func (t *TestPerformanceAnalyzer) IdentifyBottlenecks(metrics PerformanceMetrics) []Bottleneck {
	bottlenecks := []Bottleneck{}

	if metrics.CPUUsage.Usage > 80.0 {
		bottlenecks = append(bottlenecks, Bottleneck{
			Type:        BottleneckTypeCPU,
			Severity:    BottleneckSeverityHigh,
			Description: "High CPU usage",
		})
	}

	return bottlenecks
}

func (t *TestPerformanceAnalyzer) GenerateRecommendations(analysis AnalysisResult) []Recommendation {
	return analysis.Recommendations
}

func (t *TestPerformanceAnalyzer) PredictScaling(_ []PerformanceMetrics) ScalingPrediction {
	return ScalingPrediction{
		CurrentCapacity: CapacityMetrics{
			CPU: CapacityInfo{
				Current:     50.0,
				Maximum:     100.0,
				Utilization: 0.5,
				Available:   50.0,
				Unit:        "percent",
			},
		},
		PredictedDemand: DemandForecast{
			TimeHorizon: 30 * 24 * time.Hour, // 30 days
			Confidence:  0.75,
			Methodology: "linear_regression",
		},
		ScalingNeeds: ScalingRequirements{
			CPU: ScalingRequirement{
				Current:  50.0,
				Required: 75.0,
				Increase: 25.0,
				Timeline: 7 * 24 * time.Hour, // 7 days
				Priority: ScalingPriorityMedium,
			},
		},
	}
}

func TestPerformanceMetrics(t *testing.T) {
	metrics := PerformanceMetrics{
		ResponseTime: 50 * time.Millisecond,
		Throughput:   150.0,
		ErrorRate:    0.02,
		MemoryUsage: MemoryMetrics{
			Used:       1024 * 1024 * 100,
			Available:  1024 * 1024 * 900,
			Total:      1024 * 1024 * 1000,
			Percentage: 10.0,
		},
		CPUUsage: CPUMetrics{
			Usage: 25.0,
			Cores: runtime.NumCPU(),
		},
	}

	if metrics.ResponseTime <= 0 {
		t.Error("Response time should be positive")
	}

	if metrics.Throughput <= 0 {
		t.Error("Throughput should be positive")
	}

	if metrics.ErrorRate < 0 || metrics.ErrorRate > 1 {
		t.Error("Error rate should be between 0 and 1")
	}

	if metrics.MemoryUsage.Percentage < 0 || metrics.MemoryUsage.Percentage > 100 {
		t.Error("Memory usage percentage should be between 0 and 100")
	}

	if metrics.CPUUsage.Usage < 0 || metrics.CPUUsage.Usage > 100 {
		t.Error("CPU usage should be between 0 and 100")
	}

	t.Logf("Performance metrics validation passed")
	t.Logf("Response Time: %v", metrics.ResponseTime)
	t.Logf("Throughput: %.2f req/s", metrics.Throughput)
	t.Logf("Error Rate: %.2f%%", metrics.ErrorRate*100)
	t.Logf("Memory Usage: %.2f%%", metrics.MemoryUsage.Percentage)
	t.Logf("CPU Usage: %.2f%%", metrics.CPUUsage.Usage)
}

func TestBenchmarkResult(t *testing.T) {
	result := BenchmarkResult{
		TotalRequests:  10000,
		SuccessfulReqs: 9950,
		FailedRequests: 50,
		ResponseTimes: ResponseTimeStats{
			Min:    10 * time.Millisecond,
			Max:    200 * time.Millisecond,
			Mean:   50 * time.Millisecond,
			Median: 45 * time.Millisecond,
			P95:    80 * time.Millisecond,
			P99:    120 * time.Millisecond,
		},
		Throughput: ThroughputStats{
			RequestsPerSecond: 33.3,
			BytesPerSecond:    1024 * 33.3,
		},
		ErrorStats: ErrorStats{
			TotalErrors: 50,
			ErrorRate:   0.005,
		},
	}

	if result.TotalRequests != result.SuccessfulReqs+result.FailedRequests {
		t.Error("Total requests should equal successful + failed requests")
	}

	if result.ErrorStats.ErrorRate != float64(result.FailedRequests)/float64(result.TotalRequests) {
		t.Error("Error rate calculation is incorrect")
	}

	if result.ResponseTimes.Min > result.ResponseTimes.Mean {
		t.Error("Min response time should be less than or equal to mean")
	}

	if result.ResponseTimes.Mean > result.ResponseTimes.Max {
		t.Error("Mean response time should be less than or equal to max")
	}

	t.Logf("Benchmark result validation passed")
	t.Logf("Total Requests: %d", result.TotalRequests)
	t.Logf("Success Rate: %.2f%%", float64(result.SuccessfulReqs)/float64(result.TotalRequests)*100)
	t.Logf("Mean Response Time: %v", result.ResponseTimes.Mean)
	t.Logf("Throughput: %.2f req/s", result.Throughput.RequestsPerSecond)
}

func TestPerformanceConfig(t *testing.T) {
	config := PerformanceConfig{
		MonitoringInterval:  1 * time.Second,
		BenchmarkTimeout:    30 * time.Second,
		RegressionThreshold: 0.1,
		AlertThresholds: AlertThresholds{
			ResponseTime: 100 * time.Millisecond,
			Throughput:   100.0,
			ErrorRate:    0.05,
			MemoryUsage:  0.8,
			CPUUsage:     0.8,
		},
		OptimizationLevel: OptimizationLevelStandard,
	}

	if config.MonitoringInterval <= 0 {
		t.Error("Monitoring interval should be positive")
	}

	if config.BenchmarkTimeout <= 0 {
		t.Error("Benchmark timeout should be positive")
	}

	if config.RegressionThreshold <= 0 || config.RegressionThreshold >= 1 {
		t.Error("Regression threshold should be between 0 and 1")
	}

	if config.AlertThresholds.ErrorRate < 0 || config.AlertThresholds.ErrorRate > 1 {
		t.Error("Error rate threshold should be between 0 and 1")
	}

	if config.OptimizationLevel == "" {
		t.Error("Optimization level should not be empty")
	}

	t.Logf("Performance config validation passed")
}

func TestBottleneckDetection(t *testing.T) {
	analyzer := NewTestPerformanceAnalyzer()

	// Test high CPU usage
	metrics := PerformanceMetrics{
		CPUUsage: CPUMetrics{Usage: 85.0},
	}

	bottlenecks := analyzer.IdentifyBottlenecks(metrics)

	if len(bottlenecks) == 0 {
		t.Error("Expected to detect CPU bottleneck")
	}

	found := false
	for _, bottleneck := range bottlenecks {
		if bottleneck.Type == BottleneckTypeCPU {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find CPU bottleneck")
	}

	t.Logf("Detected %d bottlenecks", len(bottlenecks))
	for _, bottleneck := range bottlenecks {
		t.Logf("- %s: %s (%s)", bottleneck.Type, bottleneck.Description, bottleneck.Severity)
	}
}

func TestRecommendationGeneration(t *testing.T) {
	analyzer := NewTestPerformanceAnalyzer()

	metrics := []PerformanceMetrics{
		{
			CPUUsage:     CPUMetrics{Usage: 85.0},
			ResponseTime: 150 * time.Millisecond,
		},
	}

	ctx := context.Background()
	analysis, err := analyzer.Analyze(ctx, metrics)

	if err != nil {
		t.Fatalf("Analysis failed: %v", err)
	}

	if len(analysis.Recommendations) == 0 {
		t.Error("Expected recommendations to be generated")
	}

	for _, rec := range analysis.Recommendations {
		if rec.ID == "" {
			t.Error("Recommendation ID should not be empty")
		}
		if rec.Title == "" {
			t.Error("Recommendation title should not be empty")
		}
		if rec.Priority == "" {
			t.Error("Recommendation priority should not be empty")
		}
		if len(rec.Steps) == 0 {
			t.Error("Recommendation should have steps")
		}
	}

	t.Logf("Generated %d recommendations", len(analysis.Recommendations))
	for _, rec := range analysis.Recommendations {
		t.Logf("- %s: %s (%s)", rec.ID, rec.Title, rec.Priority)
	}
}

func TestScalingPrediction(t *testing.T) {
	analyzer := NewTestPerformanceAnalyzer()

	trends := []PerformanceMetrics{
		{CPUUsage: CPUMetrics{Usage: 40.0}},
		{CPUUsage: CPUMetrics{Usage: 45.0}},
		{CPUUsage: CPUMetrics{Usage: 50.0}},
	}

	prediction := analyzer.PredictScaling(trends)

	if prediction.CurrentCapacity.CPU.Current <= 0 {
		t.Error("Current CPU capacity should be positive")
	}

	if prediction.ScalingNeeds.CPU.Required <= prediction.ScalingNeeds.CPU.Current {
		t.Error("Required capacity should be greater than current")
	}

	if prediction.PredictedDemand.Confidence < 0 || prediction.PredictedDemand.Confidence > 1 {
		t.Error("Confidence should be between 0 and 1")
	}

	t.Logf("Scaling prediction completed")
	t.Logf("Current CPU: %.2f%%", prediction.CurrentCapacity.CPU.Current)
	t.Logf("Required CPU: %.2f%%", prediction.ScalingNeeds.CPU.Required)
	t.Logf("Confidence: %.2f%%", prediction.PredictedDemand.Confidence*100)
}

func BenchmarkPerformanceOptimizer_OptimizePerformance(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := PerformanceConfig{
		MonitoringInterval:  100 * time.Millisecond,
		BenchmarkTimeout:    1 * time.Second,
		RegressionThreshold: 0.1,
		OptimizationLevel:   OptimizationLevelBasic,
		EnablePredictive:    false,
		EnableAutoOptimize:  false,
	}

	optimizer := NewPerformanceOptimizer(config)
	optimizer.AddMonitor(NewTestPerformanceMonitor())
	optimizer.AddBenchmark(NewTestBenchmark())
	optimizer.AddAnalyzer(NewTestPerformanceAnalyzer())

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := optimizer.OptimizePerformance(ctx, server.URL)
		if err != nil {
			b.Fatalf("Optimization failed: %v", err)
		}
	}
}

func BenchmarkPerformanceAnalyzer_Analyze(b *testing.B) {
	analyzer := NewTestPerformanceAnalyzer()

	metrics := []PerformanceMetrics{
		{
			Timestamp:    time.Now(),
			ResponseTime: 50 * time.Millisecond,
			Throughput:   150.0,
			ErrorRate:    0.02,
			CPUUsage:     CPUMetrics{Usage: 25.0},
			MemoryUsage:  MemoryMetrics{Percentage: 30.0},
		},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzer.Analyze(ctx, metrics)
		if err != nil {
			b.Fatalf("Analysis failed: %v", err)
		}
	}
}

func BenchmarkBenchmark_Run(b *testing.B) {
	benchmark := NewTestBenchmark()

	config := BenchmarkConfig{
		Duration:    1 * time.Second,
		Concurrency: 1,
		Targets:     []string{"http://localhost"},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchmark.Run(ctx, config)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}
