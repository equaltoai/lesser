// Package performance provides comprehensive benchmarking utilities for serverless Lambda function performance testing.
package performance

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// BenchmarkConfig defines benchmark configuration
type BenchmarkConfig struct {
	Name                string
	WarmupIterations    int
	BenchmarkIterations int
	ConcurrentWorkers   int
	RequestsPerWorker   int
	TargetRPS           int
	MaxDuration         time.Duration
	CollectMetrics      bool
	ProfileCPU          bool
	ProfileMemory       bool
}

// BenchmarkResult contains benchmark results
type BenchmarkResult struct {
	Name               string
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	TotalDuration      time.Duration
	MinLatency         time.Duration
	MaxLatency         time.Duration
	AvgLatency         time.Duration
	P50Latency         time.Duration
	P90Latency         time.Duration
	P95Latency         time.Duration
	P99Latency         time.Duration
	RequestsPerSecond  float64
	MemoryStats        MemoryStats
	CPUStats           CPUStats
	Errors             map[string]int
}

// MemoryStats tracks memory usage
type MemoryStats struct {
	InitialHeap  uint64
	PeakHeap     uint64
	FinalHeap    uint64
	TotalAlloc   uint64
	NumGC        uint32
	GCPauseTotal time.Duration
	GCPauseAvg   time.Duration
}

// CPUStats tracks CPU usage
type CPUStats struct {
	UserTime      time.Duration
	SystemTime    time.Duration
	NumCPU        int
	NumGoroutines int
}

// PerformanceBenchmark runs performance benchmarks
//
//nolint:revive // Performance prefix clarifies this is performance-specific benchmark
type PerformanceBenchmark struct {
	config      BenchmarkConfig
	handler     lift.Handler
	metrics     *MetricsCollector
	results     *BenchmarkResult
	rateLimiter *RateLimiter
}

// MetricsCollector collects performance metrics
type MetricsCollector struct {
	mu        sync.RWMutex
	latencies []time.Duration
	errors    map[string]int
	startTime time.Time
	endTime   time.Time
	requests  int64
	successes int64
	failures  int64
}

// NewPerformanceBenchmark creates a new benchmark
func NewPerformanceBenchmark(config BenchmarkConfig, handler lift.Handler) *PerformanceBenchmark {
	return &PerformanceBenchmark{
		config:  config,
		handler: handler,
		metrics: &MetricsCollector{
			latencies: make([]time.Duration, 0),
			errors:    make(map[string]int),
		},
		results: &BenchmarkResult{
			Name:   config.Name,
			Errors: make(map[string]int),
		},
		rateLimiter: NewRateLimiter(config.TargetRPS),
	}
}

// Run executes the benchmark
func (b *PerformanceBenchmark) Run(t *testing.T) *BenchmarkResult {
	t.Logf("Starting benchmark: %s", b.config.Name)

	// Warmup phase
	if b.config.WarmupIterations > 0 {
		t.Logf("Warming up with %d iterations...", b.config.WarmupIterations)
		b.runWarmup()
	}

	// Record initial memory stats
	initialMem := b.recordMemoryStats()

	// Start metrics collection
	b.metrics.startTime = time.Now()

	// Run benchmark
	ctx, cancel := context.WithTimeout(context.Background(), b.config.MaxDuration)
	defer cancel()

	var wg sync.WaitGroup
	workerRequests := b.config.RequestsPerWorker
	if workerRequests == 0 {
		workerRequests = b.config.BenchmarkIterations / b.config.ConcurrentWorkers
	}

	// Launch workers
	for i := 0; i < b.config.ConcurrentWorkers; i++ {
		wg.Add(1)
		go b.runWorker(ctx, &wg, i, workerRequests)
	}

	// Wait for completion
	wg.Wait()
	b.metrics.endTime = time.Now()

	// Calculate results
	b.calculateResults(initialMem)

	// Print results
	b.printResults(t)

	return b.results
}

// runWarmup performs warmup iterations
func (b *PerformanceBenchmark) runWarmup() {
	for i := 0; i < b.config.WarmupIterations; i++ {
		req := b.createTestRequest(i)
		_ = b.handler.Handle(req)
	}
}

// runWorker executes requests for a single worker
func (b *PerformanceBenchmark) runWorker(ctx context.Context, wg *sync.WaitGroup, workerID, numRequests int) {
	defer wg.Done()

	for i := 0; i < numRequests; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Rate limiting
		if b.config.TargetRPS > 0 {
			b.rateLimiter.Wait()
		}

		// Create and execute request
		req := b.createTestRequest(workerID*numRequests + i)
		start := time.Now()
		err := b.handler.Handle(req)
		latency := time.Since(start)

		// Record metrics
		b.recordMetrics(latency, err)
	}
}

// createTestRequest creates a test request
func (b *PerformanceBenchmark) createTestRequest(index int) *lift.Context {
	return &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method: "POST",
			Path:   "/api/test",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"X-Request-ID": fmt.Sprintf("bench-%d", index),
			},
			Body: []byte(fmt.Sprintf(`{"id":%d,"data":"test"}`, index)),
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}
}

// recordMetrics records request metrics
func (b *PerformanceBenchmark) recordMetrics(latency time.Duration, err error) {
	b.metrics.mu.Lock()
	defer b.metrics.mu.Unlock()

	atomic.AddInt64(&b.metrics.requests, 1)
	b.metrics.latencies = append(b.metrics.latencies, latency)

	if err != nil {
		atomic.AddInt64(&b.metrics.failures, 1)
		errStr := err.Error()
		b.metrics.errors[errStr]++
	} else {
		atomic.AddInt64(&b.metrics.successes, 1)
	}
}

// calculateResults calculates benchmark results
func (b *PerformanceBenchmark) calculateResults(initialMem runtime.MemStats) {
	b.results.TotalRequests = atomic.LoadInt64(&b.metrics.requests)
	b.results.SuccessfulRequests = atomic.LoadInt64(&b.metrics.successes)
	b.results.FailedRequests = atomic.LoadInt64(&b.metrics.failures)
	b.results.TotalDuration = b.metrics.endTime.Sub(b.metrics.startTime)

	// Calculate latency statistics
	if len(b.metrics.latencies) > 0 {
		b.calculateLatencyStats()
	}

	// Calculate RPS
	if b.results.TotalDuration > 0 {
		b.results.RequestsPerSecond = float64(b.results.TotalRequests) / b.results.TotalDuration.Seconds()
	}

	// Copy errors
	for err, count := range b.metrics.errors {
		b.results.Errors[err] = count
	}

	// Record final memory stats
	if b.config.CollectMetrics {
		finalMem := b.recordMemoryStats()
		b.results.MemoryStats = b.calculateMemoryDiff(initialMem, finalMem)
	}
}

// calculateLatencyStats calculates latency percentiles
func (b *PerformanceBenchmark) calculateLatencyStats() {
	latencies := b.metrics.latencies
	sortDurations(latencies)

	b.results.MinLatency = latencies[0]
	b.results.MaxLatency = latencies[len(latencies)-1]

	// Calculate average
	var total time.Duration
	for _, l := range latencies {
		total += l
	}
	b.results.AvgLatency = total / time.Duration(len(latencies))

	// Calculate percentiles
	b.results.P50Latency = percentile(latencies, 0.50)
	b.results.P90Latency = percentile(latencies, 0.90)
	b.results.P95Latency = percentile(latencies, 0.95)
	b.results.P99Latency = percentile(latencies, 0.99)
}

// recordMemoryStats records current memory statistics
func (b *PerformanceBenchmark) recordMemoryStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// calculateMemoryDiff calculates memory usage difference
func (b *PerformanceBenchmark) calculateMemoryDiff(initial, final runtime.MemStats) MemoryStats {
	// Safe uint64 subtraction and conversion to Duration
	const maxInt64AsUint64 = uint64(9223372036854775807) // math.MaxInt64
	var gcPauseTotal time.Duration
	if final.PauseTotalNs >= initial.PauseTotalNs {
		pauseDiff := final.PauseTotalNs - initial.PauseTotalNs
		if pauseDiff <= maxInt64AsUint64 {
			gcPauseTotal = time.Duration(pauseDiff)
		} else {
			gcPauseTotal = time.Duration(^int64(0)) // Max duration
		}
	} else {
		gcPauseTotal = 0 // Handle underflow case
	}

	return MemoryStats{
		InitialHeap:  initial.HeapAlloc,
		FinalHeap:    final.HeapAlloc,
		TotalAlloc:   final.TotalAlloc - initial.TotalAlloc,
		NumGC:        final.NumGC - initial.NumGC,
		GCPauseTotal: gcPauseTotal,
	}
}

// printResults prints benchmark results
func (b *PerformanceBenchmark) printResults(t *testing.T) {
	t.Logf("\n=== Benchmark Results: %s ===", b.results.Name)
	t.Logf("Total Requests: %d", b.results.TotalRequests)
	t.Logf("Successful: %d (%.2f%%)", b.results.SuccessfulRequests,
		float64(b.results.SuccessfulRequests)/float64(b.results.TotalRequests)*100)
	t.Logf("Failed: %d", b.results.FailedRequests)
	t.Logf("Duration: %v", b.results.TotalDuration)
	t.Logf("Requests/sec: %.2f", b.results.RequestsPerSecond)

	t.Logf("\nLatency Statistics:")
	t.Logf("  Min: %v", b.results.MinLatency)
	t.Logf("  Max: %v", b.results.MaxLatency)
	t.Logf("  Avg: %v", b.results.AvgLatency)
	t.Logf("  P50: %v", b.results.P50Latency)
	t.Logf("  P90: %v", b.results.P90Latency)
	t.Logf("  P95: %v", b.results.P95Latency)
	t.Logf("  P99: %v", b.results.P99Latency)

	if b.config.CollectMetrics {
		t.Logf("\nMemory Statistics:")
		t.Logf("  Initial Heap: %d MB", b.results.MemoryStats.InitialHeap/1024/1024)
		t.Logf("  Final Heap: %d MB", b.results.MemoryStats.FinalHeap/1024/1024)
		t.Logf("  Total Allocated: %d MB", b.results.MemoryStats.TotalAlloc/1024/1024)
		t.Logf("  GC Runs: %d", b.results.MemoryStats.NumGC)
		t.Logf("  GC Pause Total: %v", b.results.MemoryStats.GCPauseTotal)
	}

	if len(b.results.Errors) > 0 {
		t.Logf("\nErrors:")
		for err, count := range b.results.Errors {
			t.Logf("  %s: %d", err, count)
		}
	}
}

// Helper functions

// sortDurations sorts durations in place
func sortDurations(durations []time.Duration) {
	for i := range durations {
		for j := i + 1; j < len(durations); j++ {
			if durations[i] > durations[j] {
				durations[i], durations[j] = durations[j], durations[i]
			}
		}
	}
}

// percentile calculates the percentile value
func percentile(sorted []time.Duration, p float64) time.Duration {
	index := int(float64(len(sorted)-1) * p)
	return sorted[index]
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	rate   int
	bucket chan struct{}
	ticker *time.Ticker
	stopCh chan struct{}
}

// NewRateLimiter creates a rate limiter
func NewRateLimiter(rps int) *RateLimiter {
	if rps <= 0 {
		return nil
	}

	rl := &RateLimiter{
		rate:   rps,
		bucket: make(chan struct{}, rps),
		ticker: time.NewTicker(time.Second / time.Duration(rps)),
		stopCh: make(chan struct{}),
	}

	// Fill bucket initially
	for i := 0; i < rps; i++ {
		rl.bucket <- struct{}{}
	}

	// Refill bucket
	go func() {
		for {
			select {
			case <-rl.ticker.C:
				select {
				case rl.bucket <- struct{}{}:
				default:
				}
			case <-rl.stopCh:
				return
			}
		}
	}()

	return rl
}

// Wait waits for a token
func (rl *RateLimiter) Wait() {
	if rl == nil {
		return
	}
	<-rl.bucket
}

// Stop stops the rate limiter
func (rl *RateLimiter) Stop() {
	if rl != nil {
		rl.ticker.Stop()
		close(rl.stopCh)
	}
}

// Benchmark Scenarios

// BenchmarkAPIEndpoint benchmarks an API endpoint
func BenchmarkAPIEndpoint(b *testing.B, handler lift.Handler) {
	benchmark := NewPerformanceBenchmark(BenchmarkConfig{
		Name:                "API Endpoint",
		WarmupIterations:    100,
		BenchmarkIterations: b.N,
		ConcurrentWorkers:   10,
		TargetRPS:           0, // No limit
		MaxDuration:         5 * time.Minute,
		CollectMetrics:      true,
	}, handler)

	b.ResetTimer()
	result := benchmark.Run(&testing.T{})

	b.ReportMetric(float64(result.RequestsPerSecond), "req/s")
	b.ReportMetric(float64(result.P99Latency.Microseconds()), "p99_μs")
}

// BenchmarkDatabaseOperations benchmarks database operations
func BenchmarkDatabaseOperations(b *testing.B, operation func() error) {
	b.ResetTimer()

	var totalDuration time.Duration
	errors := 0

	for i := 0; i < b.N; i++ {
		start := time.Now()
		if err := operation(); err != nil {
			errors++
		}
		totalDuration += time.Since(start)
	}

	avgDuration := totalDuration / time.Duration(b.N)
	b.ReportMetric(float64(avgDuration.Microseconds()), "avg_μs")
	b.ReportMetric(float64(errors), "errors")
}

// LoadTestScenario defines a load test scenario
type LoadTestScenario struct {
	Name           string
	Duration       time.Duration
	RampUpTime     time.Duration
	InitialRPS     int
	TargetRPS      int
	StepDuration   time.Duration
	Handler        lift.Handler
	RequestBuilder func(int) *lift.Context
}

// RunLoadTest executes a load test scenario
func RunLoadTest(t *testing.T, scenario LoadTestScenario) {
	t.Logf("Starting load test: %s", scenario.Name)

	startTime := time.Now()
	currentRPS := scenario.InitialRPS
	step := (scenario.TargetRPS - scenario.InitialRPS) / int(scenario.RampUpTime/scenario.StepDuration)

	results := make([]*BenchmarkResult, 0)

	// Ramp up phase
	for elapsed := time.Duration(0); elapsed < scenario.RampUpTime; elapsed += scenario.StepDuration {
		t.Logf("Load test at %d RPS", currentRPS)

		benchmark := NewPerformanceBenchmark(BenchmarkConfig{
			Name:              fmt.Sprintf("%s-%dRPS", scenario.Name, currentRPS),
			ConcurrentWorkers: currentRPS / 10, // Adjust based on target
			TargetRPS:         currentRPS,
			MaxDuration:       scenario.StepDuration,
			CollectMetrics:    true,
		}, scenario.Handler)

		result := benchmark.Run(t)
		results = append(results, result)

		// Check for degradation
		if result.P99Latency > 500*time.Millisecond ||
			float64(result.FailedRequests)/float64(result.TotalRequests) > 0.01 {
			t.Logf("Performance degradation detected at %d RPS", currentRPS)
			break
		}

		currentRPS += step
	}

	// Sustained load phase
	remainingTime := scenario.Duration - time.Since(startTime)
	if remainingTime > 0 {
		t.Logf("Sustaining load at %d RPS for %v", currentRPS, remainingTime)

		benchmark := NewPerformanceBenchmark(BenchmarkConfig{
			Name:              fmt.Sprintf("%s-sustained", scenario.Name),
			ConcurrentWorkers: currentRPS / 10,
			TargetRPS:         currentRPS,
			MaxDuration:       remainingTime,
			CollectMetrics:    true,
		}, scenario.Handler)

		result := benchmark.Run(t)
		results = append(results, result)
	}

	// Generate report
	generateLoadTestReport(t, results)
}

// generateLoadTestReport generates a load test report
func generateLoadTestReport(t *testing.T, results []*BenchmarkResult) {
	t.Logf("\n=== Load Test Summary ===")

	for _, result := range results {
		t.Logf("\n%s:", result.Name)
		t.Logf("  RPS: %.2f", result.RequestsPerSecond)
		t.Logf("  Success Rate: %.2f%%",
			float64(result.SuccessfulRequests)/float64(result.TotalRequests)*100)
		t.Logf("  P99 Latency: %v", result.P99Latency)
	}
}
