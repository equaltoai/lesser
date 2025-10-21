package middleware

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

// Helper function to create properly synchronized mock metrics using sync.Map
func newMockServiceMeshMetrics() *mockServiceMeshMetrics {
	return &mockServiceMeshMetrics{
		metrics: &sync.Map{},
		tags:    make(map[string]string),
		mu:      sync.RWMutex{},
	}
}

// Mock implementations for testing service mesh patterns

type mockServiceMeshLogger struct {
	logs []map[string]any
	mu   sync.RWMutex
}

func (m *mockServiceMeshLogger) Info(msg string, fields ...map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := map[string]any{"level": "info", "message": msg}
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			entry[k] = v
		}
	}
	m.logs = append(m.logs, entry)
}

func (m *mockServiceMeshLogger) Error(msg string, fields ...map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := map[string]any{"level": "error", "message": msg}
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			entry[k] = v
		}
	}
	m.logs = append(m.logs, entry)
}

func (m *mockServiceMeshLogger) Warn(msg string, fields ...map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := map[string]any{"level": "warn", "message": msg}
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			entry[k] = v
		}
	}
	m.logs = append(m.logs, entry)
}

func (m *mockServiceMeshLogger) Debug(msg string, fields ...map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := map[string]any{"level": "debug", "message": msg}
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			entry[k] = v
		}
	}
	m.logs = append(m.logs, entry)
}

// lift.Logger interface methods
func (m *mockServiceMeshLogger) WithField(_ string, _ any) lift.Logger {
	return m
}
func (m *mockServiceMeshLogger) WithFields(_ map[string]any) lift.Logger {
	return m
}

// observability.StructuredLogger interface methods
func (m *mockServiceMeshLogger) WithRequestID(_ string) observability.StructuredLogger {
	return m
}
func (m *mockServiceMeshLogger) WithTenantID(_ string) observability.StructuredLogger {
	return m
}
func (m *mockServiceMeshLogger) WithUserID(_ string) observability.StructuredLogger  { return m }
func (m *mockServiceMeshLogger) WithTraceID(_ string) observability.StructuredLogger { return m }
func (m *mockServiceMeshLogger) WithSpanID(_ string) observability.StructuredLogger  { return m }
func (m *mockServiceMeshLogger) Flush(_ context.Context) error                       { return nil }
func (m *mockServiceMeshLogger) Close() error                                        { return nil }
func (m *mockServiceMeshLogger) IsHealthy() bool                                     { return true }
func (m *mockServiceMeshLogger) GetStats() observability.LoggerStats {
	return observability.LoggerStats{}
}

type mockServiceMeshMetrics struct {
	metrics *sync.Map
	tags    map[string]string
	mu      sync.RWMutex
}

// lift.MetricsCollector interface methods
func (m *mockServiceMeshMetrics) Counter(name string, _ ...map[string]string) lift.Counter {
	return &mockServiceMeshCounter{metrics: m.metrics, name: name}
}

func (m *mockServiceMeshMetrics) Histogram(name string, _ ...map[string]string) lift.Histogram {
	return &mockServiceMeshHistogram{metrics: m.metrics, name: name}
}

func (m *mockServiceMeshMetrics) Gauge(name string, _ ...map[string]string) lift.Gauge {
	return &mockServiceMeshGauge{metrics: m.metrics, name: name}
}

func (m *mockServiceMeshMetrics) Flush() error {
	return nil
}

// observability.MetricsCollector interface methods
func (m *mockServiceMeshMetrics) WithTags(tags map[string]string) observability.MetricsCollector {
	newTags := make(map[string]string)
	for k, v := range m.tags {
		newTags[k] = v
	}
	for k, v := range tags {
		newTags[k] = v
	}
	return &mockServiceMeshMetrics{
		metrics: m.metrics,
		tags:    newTags,
		mu:      sync.RWMutex{},
	}
}

func (m *mockServiceMeshMetrics) WithTag(key, value string) observability.MetricsCollector {
	return m.WithTags(map[string]string{key: value})
}

func (m *mockServiceMeshMetrics) RecordBatch(_ []*observability.MetricEntry) error { return nil }
func (m *mockServiceMeshMetrics) Close() error                                     { return nil }
func (m *mockServiceMeshMetrics) GetStats() observability.MetricsStats {
	return observability.MetricsStats{}
}

// Add missing methods to implement updated MetricsCollector interface
func (m *mockServiceMeshMetrics) RecordLatency(operation string, duration time.Duration) {
	m.metrics.Store(operation+".latency", duration.Milliseconds())
}

func (m *mockServiceMeshMetrics) RecordError(operation string) {
	key := operation + ".errors"
	val, _ := m.metrics.LoadOrStore(key, int64(0))
	for {
		current := val.(int64)
		if ok := m.metrics.CompareAndSwap(key, current, current+1); ok {
			break
		}
		val, _ = m.metrics.Load(key)
	}
}

func (m *mockServiceMeshMetrics) RecordSuccess(operation string) {
	key := operation + ".success"
	val, _ := m.metrics.LoadOrStore(key, int64(0))
	for {
		current := val.(int64)
		if ok := m.metrics.CompareAndSwap(key, current, current+1); ok {
			break
		}
		val, _ = m.metrics.Load(key)
	}
}

// Helper method to get metrics count for testing
func (m *mockServiceMeshMetrics) GetMetricsCount() int {
	count := 0
	m.metrics.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// mockMetricEntry removed - was unused

type mockServiceMeshCounter struct {
	metrics *sync.Map
	name    string
}

func (c *mockServiceMeshCounter) Inc() {
	val, _ := c.metrics.LoadOrStore(c.name, int64(0))
	for {
		current := val.(int64)
		if ok := c.metrics.CompareAndSwap(c.name, current, current+1); ok {
			break
		}
		val, _ = c.metrics.Load(c.name)
	}
}

func (c *mockServiceMeshCounter) Add(value float64) {
	val, _ := c.metrics.LoadOrStore(c.name, float64(0))
	for {
		current := val.(float64)
		if ok := c.metrics.CompareAndSwap(c.name, current, current+value); ok {
			break
		}
		val, _ = c.metrics.Load(c.name)
	}
}

type mockServiceMeshHistogram struct {
	metrics *sync.Map
	name    string
}

func (h *mockServiceMeshHistogram) Observe(value float64) {
	h.metrics.Store(h.name, value)
}

type mockServiceMeshGauge struct {
	metrics *sync.Map
	name    string
}

func (g *mockServiceMeshGauge) Set(value float64) {
	g.metrics.Store(g.name, value)
}

func (g *mockServiceMeshGauge) Inc() {
	val, _ := g.metrics.LoadOrStore(g.name, float64(0))
	for {
		current := val.(float64)
		if ok := g.metrics.CompareAndSwap(g.name, current, current+1); ok {
			break
		}
		val, _ = g.metrics.Load(g.name)
	}
}

func (g *mockServiceMeshGauge) Dec() {
	val, _ := g.metrics.LoadOrStore(g.name, float64(0))
	for {
		current := val.(float64)
		if ok := g.metrics.CompareAndSwap(g.name, current, current-1); ok {
			break
		}
		val, _ = g.metrics.Load(g.name)
	}
}

func (g *mockServiceMeshGauge) Add(value float64) {
	val, _ := g.metrics.LoadOrStore(g.name, float64(0))
	for {
		current := val.(float64)
		if ok := g.metrics.CompareAndSwap(g.name, current, current+value); ok {
			break
		}
		val, _ = g.metrics.Load(g.name)
	}
}

// Circuit Breaker Tests

func TestCircuitBreakerMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		requests       []bool
		expectedStates []CircuitBreakerState
		config         CircuitBreakerConfig
	}{
		{
			name: "basic circuit breaker flow",
			config: CircuitBreakerConfig{
				Name:             "test",
				FailureThreshold: 3,
				SuccessThreshold: 2,
				Timeout:          100 * time.Millisecond,
				EnableMetrics:    true,
			},
			requests:       []bool{false, false, false, true, true}, // 3 failures, then 2 successes
			expectedStates: []CircuitBreakerState{CircuitBreakerClosed, CircuitBreakerClosed, CircuitBreakerOpen, CircuitBreakerHalfOpen, CircuitBreakerClosed},
		},
		{
			name: "error rate threshold",
			config: CircuitBreakerConfig{
				Name:                "test",
				ErrorRateThreshold:  0.5,
				MinRequestThreshold: 4,
				SlidingWindowSize:   time.Minute,
				EnableMetrics:       true,
			},
			requests:       []bool{true, false, false, false}, // 75% error rate
			expectedStates: []CircuitBreakerState{CircuitBreakerClosed, CircuitBreakerClosed, CircuitBreakerClosed, CircuitBreakerOpen},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &mockServiceMeshLogger{}
			metrics := newMockServiceMeshMetrics()

			tt.config.Logger = logger
			tt.config.Metrics = metrics

			middleware := CircuitBreakerMiddleware(tt.config)

			for i, shouldSucceed := range tt.requests {
				handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
					if shouldSucceed {
						return nil
					}
					return errors.New("test error")
				}))

				ctx := &lift.Context{
					Context: context.Background(),
					Request: &lift.Request{
						Method: "GET",
						Path:   "/test",
					},
					Response: &lift.Response{},
				}

				err := handler.Handle(ctx)

				// For open circuit, we expect fallback response
				if tt.expectedStates[i] == CircuitBreakerOpen && i > 0 {
					if err == nil {
						t.Errorf("Expected error for open circuit at request %d", i)
					}
				}
			}
		})
	}
}

// Bulkhead Tests

func TestBulkheadMiddleware(t *testing.T) {
	tests := []struct {
		name               string
		config             BulkheadConfig
		concurrentRequests int
		expectedRejections int
	}{
		{
			name: "basic bulkhead limiting",
			config: BulkheadConfig{
				Name:                  "test",
				MaxConcurrentRequests: 2,
				MaxWaitTime:           10 * time.Millisecond,
				EnableMetrics:         true,
			},
			concurrentRequests: 5,
			expectedRejections: 3,
		},
		{
			name: "tenant isolation",
			config: BulkheadConfig{
				Name:                  "test",
				MaxConcurrentRequests: 10,
				EnableTenantIsolation: true,
				DefaultTenantLimit:    1,
				MaxWaitTime:           10 * time.Millisecond,
				EnableMetrics:         true,
			},
			concurrentRequests: 3,
			expectedRejections: 2, // Only 1 allowed per tenant
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &mockServiceMeshLogger{}
			metrics := newMockServiceMeshMetrics()

			tt.config.Logger = logger
			tt.config.Metrics = metrics

			middleware := BulkheadMiddleware(tt.config)

			var rejections int32
			var wg sync.WaitGroup

			// Start concurrent requests simultaneously
			wg.Add(tt.concurrentRequests)
			for i := 0; i < tt.concurrentRequests; i++ {
				go func(_ int) {
					defer wg.Done()

					handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
						time.Sleep(100 * time.Millisecond) // Longer work simulation
						return nil
					}))

					ctx := &lift.Context{
						Context: context.Background(),
						Request: &lift.Request{
							Method: "GET",
							Path:   "/test",
						},
						Response: &lift.Response{},
					}
					ctx.SetTenantID("tenant1")

					err := handler.Handle(ctx)
					if err != nil {
						atomic.AddInt32(&rejections, 1)
					}
				}(i)
			}

			// Wait for all requests to complete
			wg.Wait()

			actualRejections := atomic.LoadInt32(&rejections)
			if int(actualRejections) != tt.expectedRejections {
				t.Errorf("Expected %d rejections, got %d", tt.expectedRejections, actualRejections)
			}
		})
	}
}

// Retry Tests

func TestRetryMiddleware(t *testing.T) {
	tests := []struct {
		name             string
		failurePattern   []bool
		config           RetryConfig
		expectedAttempts int
		expectSuccess    bool
	}{
		{
			name: "successful retry after failures",
			config: RetryConfig{
				Name:          "test",
				MaxAttempts:   3,
				InitialDelay:  1 * time.Millisecond,
				Strategy:      RetryStrategyFixed,
				EnableMetrics: true,
			},
			failurePattern:   []bool{true, true, false}, // Fail twice, then succeed
			expectedAttempts: 3,
			expectSuccess:    true,
		},
		{
			name: "max attempts exceeded",
			config: RetryConfig{
				Name:          "test",
				MaxAttempts:   2,
				InitialDelay:  1 * time.Millisecond,
				Strategy:      RetryStrategyFixed,
				EnableMetrics: true,
			},
			failurePattern:   []bool{true, true, true}, // Always fail
			expectedAttempts: 2,
			expectSuccess:    false,
		},
		{
			name: "non-retryable error",
			config: RetryConfig{
				Name:                    "test",
				MaxAttempts:             3,
				InitialDelay:            1 * time.Millisecond,
				Strategy:                RetryStrategyFixed,
				NonRetryableStatusCodes: []int{400},
				EnableMetrics:           true,
			},
			failurePattern:   []bool{true}, // Fail with 400 status
			expectedAttempts: 1,
			expectSuccess:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &mockServiceMeshLogger{}
			metrics := newMockServiceMeshMetrics()

			tt.config.Logger = logger
			tt.config.Metrics = metrics

			middleware := RetryMiddleware(tt.config)

			attemptCount := 0
			handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
				shouldFail := attemptCount < len(tt.failurePattern) && tt.failurePattern[attemptCount]
				attemptCount++

				if shouldFail {
					if tt.name == "non-retryable error" {
						return lift.ValidationError("validation error")
					}
					return errors.New("temporary error")
				}
				return nil
			}))

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method: "GET",
					Path:   "/test",
				},
				Response: &lift.Response{},
			}

			err := handler.Handle(ctx)

			if tt.expectSuccess && err != nil {
				t.Errorf("Expected success but got error: %v", err)
			}
			if !tt.expectSuccess && err == nil {
				t.Error("Expected failure but got success")
			}
			if attemptCount != tt.expectedAttempts {
				t.Errorf("Expected %d attempts, got %d", tt.expectedAttempts, attemptCount)
			}
		})
	}
}

// Integration Tests

func TestServiceMeshIntegration(t *testing.T) {
	// Test combining circuit breaker, bulkhead, and retry
	logger := &mockServiceMeshLogger{}
	metrics := newMockServiceMeshMetrics()

	// Create middleware stack
	circuitBreakerConfig := NewBasicCircuitBreaker("integration-test")
	circuitBreakerConfig.Logger = logger
	circuitBreakerConfig.Metrics = metrics
	circuitBreakerConfig.FailureThreshold = 2

	bulkheadConfig := NewBasicBulkhead("integration-test", 5)
	bulkheadConfig.Logger = logger
	bulkheadConfig.Metrics = metrics

	retryConfig := NewBasicRetry("integration-test", 4)
	retryConfig.Logger = logger
	retryConfig.Metrics = metrics
	retryConfig.InitialDelay = 1 * time.Millisecond

	// Stack middleware: retry -> circuit breaker -> bulkhead -> handler
	retryMiddleware := RetryMiddleware(retryConfig)
	circuitBreakerMiddleware := CircuitBreakerMiddleware(circuitBreakerConfig)
	bulkheadMiddleware := BulkheadMiddleware(bulkheadConfig)

	failureCount := 0
	handler := retryMiddleware(circuitBreakerMiddleware(bulkheadMiddleware(lift.HandlerFunc(func(_ *lift.Context) error {
		failureCount++
		if failureCount <= 3 {
			return errors.New("temporary failure")
		}
		return nil
	}))))

	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method: "GET",
			Path:   "/test",
		},
		Response: &lift.Response{},
	}

	// First request should eventually succeed after retries
	err := handler.Handle(ctx)
	if err != nil {
		t.Errorf("Expected success after retries, got: %v", err)
	}

	// Verify metrics were recorded
	if metrics.GetMetricsCount() == 0 {
		t.Error("Expected metrics to be recorded")
	}

	// Verify logs were generated
	if len(logger.logs) == 0 {
		t.Error("Expected logs to be generated")
	}
}

// Benchmark Tests

func BenchmarkCircuitBreakerMiddleware(b *testing.B) {
	config := NewBasicCircuitBreaker("benchmark")
	config.EnableMetrics = false // Disable for pure performance test

	middleware := CircuitBreakerMiddleware(config)
	handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx := &lift.Context{
			Context: context.Background(),
			Request: &lift.Request{
				Method: "GET",
				Path:   "/test",
			},
			Response: &lift.Response{},
		}
		_ = handler.Handle(ctx)
	}
}

func BenchmarkBulkheadMiddleware(b *testing.B) {
	config := NewBasicBulkhead("benchmark", 1000)
	config.EnableMetrics = false // Disable for pure performance test

	middleware := BulkheadMiddleware(config)
	handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx := &lift.Context{
			Context: context.Background(),
			Request: &lift.Request{
				Method: "GET",
				Path:   "/test",
			},
			Response: &lift.Response{},
		}
		_ = handler.Handle(ctx)
	}
}

func BenchmarkRetryMiddleware(b *testing.B) {
	config := NewBasicRetry("benchmark", 1) // No retries for performance test
	config.EnableMetrics = false            // Disable for pure performance test

	middleware := RetryMiddleware(config)
	handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx := &lift.Context{
			Context: context.Background(),
			Request: &lift.Request{
				Method: "GET",
				Path:   "/test",
			},
			Response: &lift.Response{},
		}
		_ = handler.Handle(ctx)
	}
}

func BenchmarkServiceMeshStack(b *testing.B) {
	// Test performance of complete service mesh stack
	circuitBreakerConfig := NewBasicCircuitBreaker("benchmark")
	circuitBreakerConfig.EnableMetrics = false

	bulkheadConfig := NewBasicBulkhead("benchmark", 1000)
	bulkheadConfig.EnableMetrics = false

	retryConfig := NewBasicRetry("benchmark", 1)
	retryConfig.EnableMetrics = false

	retryMiddleware := RetryMiddleware(retryConfig)
	circuitBreakerMiddleware := CircuitBreakerMiddleware(circuitBreakerConfig)
	bulkheadMiddleware := BulkheadMiddleware(bulkheadConfig)

	handler := retryMiddleware(circuitBreakerMiddleware(bulkheadMiddleware(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx := &lift.Context{
			Context: context.Background(),
			Request: &lift.Request{
				Method: "GET",
				Path:   "/test",
			},
			Response: &lift.Response{},
		}
		_ = handler.Handle(ctx)
	}
}
