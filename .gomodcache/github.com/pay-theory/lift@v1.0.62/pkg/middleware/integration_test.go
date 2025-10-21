package middleware

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
)

// Integration test suite for all middleware components

func TestCompleteMiddlewareStack(t *testing.T) {
	// Create mock observability components
	logger := &mockServiceMeshLogger{}
	metrics := newMockServiceMeshMetrics()

	// Configure all middleware components
	observabilityConfig := EnhancedObservabilityConfig{
		EnableLogging: true,
		EnableMetrics: true,
		EnableTracing: false, // Skip X-Ray for unit tests
		Logger:        logger,
		Metrics:       metrics,
		SampleRate:    1.0,
	}

	circuitBreakerConfig := NewBasicCircuitBreaker("integration-test")
	circuitBreakerConfig.Logger = logger
	circuitBreakerConfig.Metrics = metrics
	circuitBreakerConfig.FailureThreshold = 3

	bulkheadConfig := NewBasicBulkhead("integration-test", 10)
	bulkheadConfig.Logger = logger
	bulkheadConfig.Metrics = metrics

	retryConfig := NewBasicRetry("integration-test", 3)
	retryConfig.Logger = logger
	retryConfig.Metrics = metrics
	retryConfig.InitialDelay = 1 * time.Millisecond

	loadSheddingConfig := NewBasicLoadShedding("integration-test")
	loadSheddingConfig.Strategy = LoadSheddingRandom
	loadSheddingConfig.MaxSheddingRate = 0.1 // 10% max shedding rate
	loadSheddingConfig.EnableMetrics = true
	loadSheddingConfig.Logger = logger
	loadSheddingConfig.Metrics = metrics

	timeoutConfig := TimeoutConfig{
		Name:           "integration-test",
		DefaultTimeout: 100 * time.Millisecond,
		EnableMetrics:  true,
		Logger:         logger,
		Metrics:        metrics,
	}

	// Create middleware stack
	observabilityMiddleware := EnhancedObservabilityMiddleware(observabilityConfig)
	timeoutMiddleware := TimeoutMiddleware(timeoutConfig)
	loadSheddingMiddleware := LoadSheddingMiddleware(loadSheddingConfig)
	retryMiddleware := RetryMiddleware(retryConfig)
	circuitBreakerMiddleware := CircuitBreakerMiddleware(circuitBreakerConfig)
	bulkheadMiddleware := BulkheadMiddleware(bulkheadConfig)

	// Test successful request flow
	t.Run("successful_request_flow", func(t *testing.T) {
		handler := observabilityMiddleware(
			timeoutMiddleware(
				loadSheddingMiddleware(
					retryMiddleware(
						circuitBreakerMiddleware(
							bulkheadMiddleware(
								lift.HandlerFunc(func(ctx *lift.Context) error {
									return ctx.JSON(map[string]string{"status": "success"})
								}),
							),
						),
					),
				),
			),
		)

		ctx := createTestContext()
		err := handler.Handle(ctx)

		if err != nil {
			t.Errorf("Expected successful request, got error: %v", err)
		}

		// Verify observability data was collected
		if len(logger.logs) == 0 {
			t.Error("Expected logs to be generated")
		}
		if metrics.GetMetricsCount() == 0 {
			t.Error("Expected metrics to be recorded")
		}
	})

	// Test failure handling and recovery
	t.Run("failure_handling_and_recovery", func(t *testing.T) {
		failureCount := int32(0)
		handler := observabilityMiddleware(
			timeoutMiddleware(
				loadSheddingMiddleware(
					retryMiddleware(
						circuitBreakerMiddleware(
							bulkheadMiddleware(
								lift.HandlerFunc(func(ctx *lift.Context) error {
									count := atomic.AddInt32(&failureCount, 1)
									if count <= 2 {
										return errors.New("temporary failure")
									}
									return ctx.JSON(map[string]string{"status": "recovered"})
								}),
							),
						),
					),
				),
			),
		)

		ctx := createTestContext()
		err := handler.Handle(ctx)

		if err != nil {
			t.Errorf("Expected recovery after retries, got error: %v", err)
		}

		// Verify retry attempts
		if atomic.LoadInt32(&failureCount) != 3 {
			t.Errorf("Expected 3 attempts, got %d", atomic.LoadInt32(&failureCount))
		}
	})

	// Test concurrent request handling
	t.Run("concurrent_request_handling", func(t *testing.T) {
		handler := observabilityMiddleware(
			timeoutMiddleware(
				loadSheddingMiddleware(
					retryMiddleware(
						circuitBreakerMiddleware(
							bulkheadMiddleware(
								lift.HandlerFunc(func(ctx *lift.Context) error {
									time.Sleep(10 * time.Millisecond) // Simulate work
									return ctx.JSON(map[string]string{"status": "success"})
								}),
							),
						),
					),
				),
			),
		)

		const numRequests = 20
		var wg sync.WaitGroup
		successCount := int32(0)
		errorCount := int32(0)

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(requestID int) {
				defer wg.Done()

				ctx := createTestContext()
				ctx.Set("tenant_id", fmt.Sprintf("tenant-%d", requestID%3)) // 3 different tenants

				err := handler.Handle(ctx)
				if err != nil {
					atomic.AddInt32(&errorCount, 1)
				} else {
					atomic.AddInt32(&successCount, 1)
				}
			}(i)
		}

		wg.Wait()

		totalRequests := atomic.LoadInt32(&successCount) + atomic.LoadInt32(&errorCount)
		if totalRequests != numRequests {
			t.Errorf("Expected %d total requests, got %d", numRequests, totalRequests)
		}

		// Should have some successes (not all rejected)
		if atomic.LoadInt32(&successCount) == 0 {
			t.Error("Expected some successful requests")
		}
	})
}

// BenchmarkCompleteMiddlewareStack benchmarks the complete middleware stack performance
func BenchmarkCompleteMiddlewareStack(b *testing.B) {
	// Create mock observability components
	logger := &mockServiceMeshLogger{}
	metrics := newMockServiceMeshMetrics()

	// Configure all middleware components
	observabilityConfig := EnhancedObservabilityConfig{
		EnableLogging: true,
		EnableMetrics: true,
		EnableTracing: false, // Skip X-Ray for benchmarks
		Logger:        logger,
		Metrics:       metrics,
		SampleRate:    1.0,
	}

	circuitBreakerConfig := NewBasicCircuitBreaker("benchmark-test")
	circuitBreakerConfig.Logger = logger
	circuitBreakerConfig.Metrics = metrics
	circuitBreakerConfig.FailureThreshold = 10 // Higher threshold for benchmarks

	bulkheadConfig := NewBasicBulkhead("benchmark-test", 100) // Higher capacity for benchmarks
	bulkheadConfig.Logger = logger
	bulkheadConfig.Metrics = metrics

	retryConfig := NewBasicRetry("benchmark-test", 2) // Fewer retries for benchmarks
	retryConfig.Logger = logger
	retryConfig.Metrics = metrics
	retryConfig.InitialDelay = 1 * time.Microsecond // Minimal delay

	loadSheddingConfig := NewBasicLoadShedding("benchmark-test")
	loadSheddingConfig.Strategy = LoadSheddingRandom
	loadSheddingConfig.MaxSheddingRate = 0.01 // Very low shedding for benchmarks
	loadSheddingConfig.EnableMetrics = true
	loadSheddingConfig.Logger = logger
	loadSheddingConfig.Metrics = metrics

	timeoutConfig := TimeoutConfig{
		Name:           "benchmark-test",
		DefaultTimeout: 1 * time.Second, // Generous timeout for benchmarks
		EnableMetrics:  true,
		Logger:         logger,
		Metrics:        metrics,
	}

	// Create middleware stack
	observabilityMiddleware := EnhancedObservabilityMiddleware(observabilityConfig)
	timeoutMiddleware := TimeoutMiddleware(timeoutConfig)
	loadSheddingMiddleware := LoadSheddingMiddleware(loadSheddingConfig)
	retryMiddleware := RetryMiddleware(retryConfig)
	circuitBreakerMiddleware := CircuitBreakerMiddleware(circuitBreakerConfig)
	bulkheadMiddleware := BulkheadMiddleware(bulkheadConfig)

	// Create complete handler stack
	handler := observabilityMiddleware(
		timeoutMiddleware(
			loadSheddingMiddleware(
				retryMiddleware(
					circuitBreakerMiddleware(
						bulkheadMiddleware(
							lift.HandlerFunc(func(ctx *lift.Context) error {
								// Simulate minimal work
								return ctx.JSON(map[string]string{"status": "success"})
							}),
						),
					),
				),
			),
		),
	)

	// Pre-create context to avoid allocation overhead in benchmark
	ctx := createTestContext()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := handler.Handle(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// Helper functions and mocks

func createTestContext() *lift.Context {
	return &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Request: &adapters.Request{
				Method:      "GET",
				Path:        "/test",
				Headers:     make(map[string]string),
				QueryParams: make(map[string]string),
			},
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}
}
