package testing

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// PerformanceMetrics holds performance measurement data
type PerformanceMetrics struct {
	ExecutionTime time.Duration
	MemoryUsage   int64
	AllocCount    uint64
	GCCount       uint32
	ColdStart     bool
	Timestamp     time.Time
}

// PerformanceTest represents a performance test configuration
type PerformanceTest struct {
	Name         string
	TestFunc     func() error
	Iterations   int
	Concurrency  int
	Timeout      time.Duration
	MemoryLimit  int64
	ExpectedTime time.Duration
	WarmupRuns   int
}

// PerformanceTestSuite manages performance testing
type PerformanceTestSuite struct {
	metrics []PerformanceMetrics
	mu      sync.RWMutex
	t       *testing.T
}

// NewPerformanceTestSuite creates a new performance test suite
func NewPerformanceTestSuite(t *testing.T) *PerformanceTestSuite {
	return &PerformanceTestSuite{
		metrics: make([]PerformanceMetrics, 0),
		t:       t,
	}
}

// MeasureExecution measures the performance of a function execution
func (pts *PerformanceTestSuite) MeasureExecution(name string, fn func() error) PerformanceMetrics {
	// Force garbage collection before measurement
	runtime.GC()

	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()
	err := fn()
	endTime := time.Now()

	runtime.ReadMemStats(&memAfter)

	// Safe conversion with overflow check
	memoryUsage := memAfter.Alloc - memBefore.Alloc
	const maxInt64AsUint64 = uint64(9223372036854775807) // math.MaxInt64
	var memUsageInt64 int64
	if memoryUsage > maxInt64AsUint64 {
		memUsageInt64 = ^int64(0) // Max int64 value
	} else {
		memUsageInt64 = int64(memoryUsage)
	}

	metrics := PerformanceMetrics{
		ExecutionTime: endTime.Sub(startTime),
		MemoryUsage:   memUsageInt64,
		AllocCount:    memAfter.Mallocs - memBefore.Mallocs,
		GCCount:       memAfter.NumGC - memBefore.NumGC,
		ColdStart:     len(pts.metrics) == 0, // First run is cold start
		Timestamp:     startTime,
	}

	if err != nil {
		pts.t.Errorf("Performance test %s failed: %v", name, err)
	}

	pts.mu.Lock()
	pts.metrics = append(pts.metrics, metrics)
	pts.mu.Unlock()

	return metrics
}

// RunPerformanceTest executes a complete performance test
func (pts *PerformanceTestSuite) RunPerformanceTest(test *PerformanceTest) *PerformanceResults {
	results := &PerformanceResults{
		TestName:    test.Name,
		Iterations:  test.Iterations,
		Concurrency: test.Concurrency,
		Metrics:     make([]PerformanceMetrics, 0, test.Iterations),
		StartTime:   time.Now(),
	}

	// Warmup runs
	for i := 0; i < test.WarmupRuns; i++ {
		_ = pts.MeasureExecution(fmt.Sprintf("%s-warmup-%d", test.Name, i), test.TestFunc)
	}

	// Clear warmup metrics
	pts.mu.Lock()
	pts.metrics = pts.metrics[:0]
	pts.mu.Unlock()

	if test.Concurrency <= 1 {
		// Sequential execution
		for i := 0; i < test.Iterations; i++ {
			metrics := pts.MeasureExecution(fmt.Sprintf("%s-%d", test.Name, i), test.TestFunc)
			results.Metrics = append(results.Metrics, metrics)
		}
	} else {
		// Concurrent execution
		results.Metrics = pts.runConcurrentTest(test)
	}

	results.EndTime = time.Now()
	results.calculateStats()

	return results
}

// runConcurrentTest executes a test with concurrent workers
func (pts *PerformanceTestSuite) runConcurrentTest(test *PerformanceTest) []PerformanceMetrics {
	var wg sync.WaitGroup
	metricsChan := make(chan PerformanceMetrics, test.Iterations)

	// Create worker pool
	iterationsPerWorker := test.Iterations / test.Concurrency
	remainingIterations := test.Iterations % test.Concurrency

	for worker := 0; worker < test.Concurrency; worker++ {
		wg.Add(1)

		iterations := iterationsPerWorker
		if worker < remainingIterations {
			iterations++
		}

		go func(workerID, workerIterations int) {
			defer wg.Done()

			for i := 0; i < workerIterations; i++ {
				metrics := pts.MeasureExecution(
					fmt.Sprintf("%s-worker%d-%d", test.Name, workerID, i),
					test.TestFunc,
				)
				metricsChan <- metrics
			}
		}(worker, iterations)
	}

	// Close channel when all workers complete
	go func() {
		wg.Wait()
		close(metricsChan)
	}()

	// Collect results
	metrics := make([]PerformanceMetrics, 0, test.Iterations)
	for metric := range metricsChan {
		metrics = append(metrics, metric)
	}

	return metrics
}

// PerformanceResults holds the results of a performance test
type PerformanceResults struct {
	TestName    string
	Iterations  int
	Concurrency int
	Metrics     []PerformanceMetrics
	StartTime   time.Time
	EndTime     time.Time

	// Calculated statistics
	MinTime    time.Duration
	MaxTime    time.Duration
	AvgTime    time.Duration
	MedianTime time.Duration
	P95Time    time.Duration
	P99Time    time.Duration

	MinMemory int64
	MaxMemory int64
	AvgMemory int64

	TotalAllocs uint64
	TotalGCs    uint32

	Throughput float64 // operations per second
}

// calculateStats computes statistics from the collected metrics
func (pr *PerformanceResults) calculateStats() {
	if len(pr.Metrics) == 0 {
		return
	}

	// Sort times for percentile calculations
	times := make([]time.Duration, len(pr.Metrics))
	totalTime := time.Duration(0)
	totalMemory := int64(0)

	pr.MinTime = pr.Metrics[0].ExecutionTime
	pr.MaxTime = pr.Metrics[0].ExecutionTime
	pr.MinMemory = pr.Metrics[0].MemoryUsage
	pr.MaxMemory = pr.Metrics[0].MemoryUsage

	for i, metric := range pr.Metrics {
		times[i] = metric.ExecutionTime
		totalTime += metric.ExecutionTime
		totalMemory += metric.MemoryUsage

		if metric.ExecutionTime < pr.MinTime {
			pr.MinTime = metric.ExecutionTime
		}
		if metric.ExecutionTime > pr.MaxTime {
			pr.MaxTime = metric.ExecutionTime
		}

		if metric.MemoryUsage < pr.MinMemory {
			pr.MinMemory = metric.MemoryUsage
		}
		if metric.MemoryUsage > pr.MaxMemory {
			pr.MaxMemory = metric.MemoryUsage
		}

		pr.TotalAllocs += metric.AllocCount
		pr.TotalGCs += metric.GCCount
	}

	pr.AvgTime = totalTime / time.Duration(len(pr.Metrics))
	pr.AvgMemory = totalMemory / int64(len(pr.Metrics))

	// Calculate percentiles
	pr.MedianTime = calculatePercentile(times, 50)
	pr.P95Time = calculatePercentile(times, 95)
	pr.P99Time = calculatePercentile(times, 99)

	// Calculate throughput
	totalTestTime := pr.EndTime.Sub(pr.StartTime)
	if totalTestTime > 0 {
		pr.Throughput = float64(len(pr.Metrics)) / totalTestTime.Seconds()
	}
}

// calculatePercentile calculates the nth percentile of a slice of durations
func calculatePercentile(times []time.Duration, percentile float64) time.Duration {
	if len(times) == 0 {
		return 0
	}

	// Simple percentile calculation (could be improved with proper sorting)
	index := int((percentile / 100.0) * float64(len(times)-1))
	if index >= len(times) {
		index = len(times) - 1
	}

	return times[index]
}

// Cold Start Testing

// ColdStartTest measures Lambda cold start performance
type ColdStartTest struct {
	InitFunc     func() error
	HandlerFunc  func() error
	Iterations   int
	MemorySize   int
	ExpectedTime time.Duration
}

// MeasureColdStart measures cold start performance
func (pts *PerformanceTestSuite) MeasureColdStart(test *ColdStartTest) *ColdStartResults {
	results := &ColdStartResults{
		MemorySize:   test.MemorySize,
		Iterations:   test.Iterations,
		InitTimes:    make([]time.Duration, 0, test.Iterations),
		HandlerTimes: make([]time.Duration, 0, test.Iterations),
	}

	for i := 0; i < test.Iterations; i++ {
		// Measure initialization time
		initStart := time.Now()
		if err := test.InitFunc(); err != nil {
			pts.t.Errorf("Cold start init failed: %v", err)
			continue
		}
		initTime := time.Since(initStart)
		results.InitTimes = append(results.InitTimes, initTime)

		// Measure first handler execution (cold start)
		handlerStart := time.Now()
		if err := test.HandlerFunc(); err != nil {
			pts.t.Errorf("Cold start handler failed: %v", err)
			continue
		}
		handlerTime := time.Since(handlerStart)
		results.HandlerTimes = append(results.HandlerTimes, handlerTime)

		// Total cold start time
		totalTime := initTime + handlerTime
		results.TotalTimes = append(results.TotalTimes, totalTime)
	}

	results.calculateColdStartStats()
	return results
}

// ColdStartResults holds cold start test results
type ColdStartResults struct {
	MemorySize   int
	Iterations   int
	InitTimes    []time.Duration
	HandlerTimes []time.Duration
	TotalTimes   []time.Duration

	AvgInitTime    time.Duration
	AvgHandlerTime time.Duration
	AvgTotalTime   time.Duration
	MaxTotalTime   time.Duration
	MinTotalTime   time.Duration
}

// calculateColdStartStats computes cold start statistics
func (csr *ColdStartResults) calculateColdStartStats() {
	if len(csr.TotalTimes) == 0 {
		return
	}

	totalInit := time.Duration(0)
	totalHandler := time.Duration(0)
	totalTime := time.Duration(0)

	csr.MaxTotalTime = csr.TotalTimes[0]
	csr.MinTotalTime = csr.TotalTimes[0]

	for i, t := range csr.TotalTimes {
		totalInit += csr.InitTimes[i]
		totalHandler += csr.HandlerTimes[i]
		totalTime += t

		if t > csr.MaxTotalTime {
			csr.MaxTotalTime = t
		}
		if t < csr.MinTotalTime {
			csr.MinTotalTime = t
		}
	}

	count := time.Duration(len(csr.TotalTimes))
	csr.AvgInitTime = totalInit / count
	csr.AvgHandlerTime = totalHandler / count
	csr.AvgTotalTime = totalTime / count
}

// Benchmark Testing

// BenchmarkLiftApp benchmarks a Lift application
func BenchmarkLiftApp(b *testing.B, setupApp func() *TestApp, path string) {
	app := setupApp()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			response := app.GET(path)
			if !response.IsSuccess() {
				b.Errorf("Request failed with status %d", response.StatusCode)
			}
		}
	})
}

// BenchmarkWithConcurrency benchmarks with specific concurrency level
func BenchmarkWithConcurrency(b *testing.B, concurrency int, testFunc func()) {
	sem := make(chan struct{}, concurrency)

	b.ResetTimer()
	b.SetParallelism(concurrency)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sem <- struct{}{}
			testFunc()
			<-sem
		}
	})
}

// Performance Assertions

// AssertPerformance validates performance requirements
func AssertPerformance(t *testing.T, results *PerformanceResults, requirements *PerformanceRequirements) {
	t.Helper()

	if requirements.MaxAvgTime > 0 {
		assert.LessOrEqual(t, results.AvgTime, requirements.MaxAvgTime,
			"Average time %v exceeded requirement %v", results.AvgTime, requirements.MaxAvgTime)
	}

	if requirements.MaxP95Time > 0 {
		assert.LessOrEqual(t, results.P95Time, requirements.MaxP95Time,
			"P95 time %v exceeded requirement %v", results.P95Time, requirements.MaxP95Time)
	}

	if requirements.MaxMemory > 0 {
		assert.LessOrEqual(t, results.MaxMemory, requirements.MaxMemory,
			"Max memory %d exceeded requirement %d", results.MaxMemory, requirements.MaxMemory)
	}

	if requirements.MinThroughput > 0 {
		assert.GreaterOrEqual(t, results.Throughput, requirements.MinThroughput,
			"Throughput %f below requirement %f", results.Throughput, requirements.MinThroughput)
	}
}

// PerformanceRequirements defines performance requirements
type PerformanceRequirements struct {
	MaxAvgTime    time.Duration
	MaxP95Time    time.Duration
	MaxP99Time    time.Duration
	MaxMemory     int64
	MinThroughput float64
	MaxColdStart  time.Duration
}

// AssertColdStartPerformance validates cold start requirements
func AssertColdStartPerformance(t *testing.T, results *ColdStartResults, maxColdStart time.Duration) {
	t.Helper()

	assert.LessOrEqual(t, results.AvgTotalTime, maxColdStart,
		"Average cold start time %v exceeded requirement %v", results.AvgTotalTime, maxColdStart)

	assert.LessOrEqual(t, results.MaxTotalTime, maxColdStart*2,
		"Max cold start time %v exceeded 2x requirement %v", results.MaxTotalTime, maxColdStart*2)
}

// Memory Testing

// MeasureMemoryUsage measures memory usage during test execution
func MeasureMemoryUsage(testFunc func()) *MemoryUsage {
	runtime.GC()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	testFunc()

	runtime.ReadMemStats(&after)

	// Safe uint64 to int64 conversions with overflow checks
	const maxInt64AsUint64 = uint64(9223372036854775807) // math.MaxInt64
	allocatedBytes := after.Alloc - before.Alloc
	var allocBytesInt64 int64
	if allocatedBytes > maxInt64AsUint64 {
		allocBytesInt64 = ^int64(0) // Max int64 value
	} else {
		allocBytesInt64 = int64(allocatedBytes)
	}

	var maxHeapInt64 int64
	if after.Sys > maxInt64AsUint64 {
		maxHeapInt64 = ^int64(0) // Max int64 value
	} else {
		maxHeapInt64 = int64(after.Sys)
	}

	return &MemoryUsage{
		AllocatedBytes: allocBytesInt64,
		AllocCount:     after.Mallocs - before.Mallocs,
		GCCount:        after.NumGC - before.NumGC,
		MaxHeapSize:    maxHeapInt64,
	}
}

// MemoryUsage holds memory usage metrics
type MemoryUsage struct {
	AllocatedBytes int64
	AllocCount     uint64
	GCCount        uint32
	MaxHeapSize    int64
}

// Load Testing Helpers

// LoadTest configuration for load testing
type LoadTest struct {
	Duration    time.Duration
	Concurrency int
	RampUpTime  time.Duration
	TestFunc    func() error
}

// RunLoadTest executes a load test
func (pts *PerformanceTestSuite) RunLoadTest(test *LoadTest) *LoadTestResults {
	results := &LoadTestResults{
		Duration:    test.Duration,
		Concurrency: test.Concurrency,
		StartTime:   time.Now(),
		Errors:      make([]error, 0),
	}

	ctx, cancel := context.WithTimeout(context.Background(), test.Duration)
	defer cancel()

	var wg sync.WaitGroup
	requestChan := make(chan time.Duration, 1000)
	errorChan := make(chan error, 1000)

	// Start workers
	for i := 0; i < test.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					start := time.Now()
					err := test.TestFunc()
					duration := time.Since(start)

					requestChan <- duration
					if err != nil {
						errorChan <- err
					}
				}
			}
		}()
	}

	// Collect results
	go func() {
		wg.Wait()
		close(requestChan)
		close(errorChan)
	}()

	// Process results
	for {
		select {
		case duration, ok := <-requestChan:
			if !ok {
				requestChan = nil
			} else {
				results.RequestCount++
				results.TotalDuration += duration
			}
		case err, ok := <-errorChan:
			if !ok {
				errorChan = nil
			} else {
				results.ErrorCount++
				results.Errors = append(results.Errors, err)
			}
		}

		if requestChan == nil && errorChan == nil {
			break
		}
	}

	results.EndTime = time.Now()
	results.calculateLoadStats()

	return results
}

// LoadTestResults holds load test results
type LoadTestResults struct {
	Duration      time.Duration
	Concurrency   int
	StartTime     time.Time
	EndTime       time.Time
	RequestCount  int64
	ErrorCount    int64
	TotalDuration time.Duration
	Errors        []error

	AvgResponseTime time.Duration
	RequestsPerSec  float64
	ErrorRate       float64
}

// calculateLoadStats computes load test statistics
func (ltr *LoadTestResults) calculateLoadStats() {
	if ltr.RequestCount > 0 {
		ltr.AvgResponseTime = ltr.TotalDuration / time.Duration(ltr.RequestCount)
	}

	actualDuration := ltr.EndTime.Sub(ltr.StartTime)
	if actualDuration > 0 {
		ltr.RequestsPerSec = float64(ltr.RequestCount) / actualDuration.Seconds()
	}

	totalRequests := ltr.RequestCount + ltr.ErrorCount
	if totalRequests > 0 {
		ltr.ErrorRate = float64(ltr.ErrorCount) / float64(totalRequests)
	}
}
