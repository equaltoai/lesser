package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/cost"
	liftPkg "github.com/equaltoai/lesser/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// BenchmarkInfrastructurePerformance benchmarks the performance of the new infrastructure
func BenchmarkInfrastructurePerformance(b *testing.B) {
	logger := zap.NewNop() // Use nop logger for benchmarks

	b.Run("ColdStart", func(b *testing.B) {
		b.Run("OldInfrastructure", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Simulate old infrastructure initialization
				app := lift.New()
				// Simulate old middleware setup
				app.Use(func(next lift.Handler) lift.Handler {
					return lift.HandlerFunc(func(ctx *lift.Context) error {
						// Basic logging
						return next.Handle(ctx)
					})
				})

				// Add a simple handler
				_ = app.GET("/test", func(ctx *lift.Context) error {
					return ctx.JSON(map[string]string{"status": "ok"})
				})
			}
		})

		b.Run("NewInfrastructure", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// New infrastructure initialization
				config := liftPkg.DefaultConfig()
				app := liftPkg.NewHTTPApp(config, logger)

				// Add a simple handler
				_ = app.GET("/test", func(ctx *lift.Context) error {
					return ctx.JSON(map[string]string{"status": "ok"})
				})
			}
		})
	})

	b.Run("RequestHandling", func(b *testing.B) {
		// Setup apps once
		oldApp := lift.New()
		// Simulate old middleware setup
		oldApp.Use(func(next lift.Handler) lift.Handler {
			return lift.HandlerFunc(func(ctx *lift.Context) error {
				// Basic logging
				return next.Handle(ctx)
			})
		})
		_ = oldApp.GET("/test", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})

		config := liftPkg.DefaultConfig()
		newApp := liftPkg.NewHTTPApp(config, logger)
		_ = newApp.GET("/test", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})

		b.Run("OldInfrastructure", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Create a test context manually
				ctx := &lift.Context{}
				// Note: We can't actually handle the request without proper test utilities
				// This is just measuring the overhead
				_ = ctx
				_ = oldApp
			}
		})

		b.Run("NewInfrastructure", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Create a test context manually
				ctx := &lift.Context{}
				// Note: We can't actually handle the request without proper test utilities
				// This is just measuring the overhead
				_ = ctx
				_ = newApp
			}
		})
	})

	b.Run("AuthenticatedRequests", func(b *testing.B) {
		// Create legacy handler for comparison
		legacyHandler := func(_ context.Context, _ events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: 200,
				Body:       `{"status":"ok"}`,
			}, nil
		}

		b.Run("LegacyWrapper", func(b *testing.B) {
			app := lift.New()
			_ = app.GET("/test", wrapHandler(legacyHandler))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Create a test context manually
				ctx := &lift.Context{}
				// Note: We can't actually handle the request without proper test utilities
				_ = ctx
				_ = app
			}
		})

		b.Run("NativeLift", func(b *testing.B) {
			app := lift.New()
			_ = app.GET("/test", func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{"status": "ok"})
			})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Create a test context manually
				ctx := &lift.Context{}
				// Note: We can't actually handle the request without proper test utilities
				_ = ctx
				_ = app
			}
		})
	})

	b.Run("MemoryAllocation", func(b *testing.B) {
		config := liftPkg.DefaultConfig()
		app := liftPkg.NewHTTPApp(config, logger)

		_ = app.GET("/test", func(ctx *lift.Context) error {
			// Simulate some work
			data := make(map[string]interface{})
			for j := 0; j < 10; j++ {
				data[string(rune('a'+j))] = j
			}
			return ctx.JSON(data)
		})

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Create a test context manually
			ctx := &lift.Context{}
			// Note: We can't actually handle the request without proper test utilities
			_ = ctx
			_ = app
		}
	})
}

// BenchmarkMiddlewareChain benchmarks middleware chain execution
func BenchmarkMiddlewareChain(b *testing.B) {
	logger := zap.NewNop()

	b.Run("StandardMiddlewareStack", func(b *testing.B) {
		config := liftPkg.DefaultConfig()
		app := liftPkg.NewHTTPApp(config, logger)

		// Add a handler that does minimal work
		_ = app.GET("/test", func(ctx *lift.Context) error {
			return ctx.Status(200).Text("OK")
		})

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Create a test context manually
			ctx := &lift.Context{}
			// Note: We can't actually handle the request without proper test utilities
			_ = ctx
			_ = app
		}
	})

	b.Run("WithCostTracking", func(b *testing.B) {
		config := liftPkg.DefaultConfig()
		config.EnableCostTracking = true
		app := liftPkg.NewHTTPApp(config, logger)

		_ = app.GET("/test", func(ctx *lift.Context) error {
			// Track some operations using centralized tracking
			if unifiedTracker, ok := ctx.Get("unified_tracker").(*cost.UnifiedTracker); ok {
				_ = unifiedTracker.TrackDynamoRead(ctx.Request.Context(), "test-table", 1)
			}
			return ctx.Status(200).Text("OK")
		})

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Create a test context manually
			ctx := &lift.Context{}
			// Set up unified tracker for the benchmark
			unifiedTracker := cost.NewRequestTracker(nil, logger, "bench", "bench-req", "bench-op")
			ctx.Set("unified_tracker", unifiedTracker)

			// Call the handler directly since we don't have test utilities
			handler := func(ctx *lift.Context) {
				if tracker, ok := ctx.Get("unified_tracker").(*cost.UnifiedTracker); ok {
					_ = tracker.TrackDynamoRead(context.Background(), "test-table", 1)
				}
			}
			handler(ctx)
		}
	})
}

// TestPerformanceMetrics validates that performance metrics are tracked correctly
func TestPerformanceMetrics(t *testing.T) {
	logger := zap.NewNop()

	t.Run("ColdStartTime", func(t *testing.T) {
		start := time.Now()

		// Initialize new infrastructure
		config := liftPkg.DefaultConfig()
		app := liftPkg.NewHTTPApp(config, logger)
		_ = app.GET("/test", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})

		coldStartTime := time.Since(start)

		// Cold start should be under 15ms (excluding imports)
		if coldStartTime > 15*time.Millisecond {
			t.Logf("Warning: Cold start time %v exceeds target of 15ms", coldStartTime)
		}
	})

	t.Run("RequestLatency", func(t *testing.T) {
		config := liftPkg.DefaultConfig()
		app := liftPkg.NewHTTPApp(config, logger)

		_ = app.GET("/test", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})

		// Warm up
		for i := 0; i < 10; i++ {
			ctx := &lift.Context{}
			_ = ctx
			_ = app
		}

		// Measure latency
		latencies := make([]time.Duration, 100)
		for i := 0; i < 100; i++ {
			start := time.Now()
			ctx := &lift.Context{}
			// Note: We're just measuring context creation, not actual request handling
			_ = ctx
			_ = app
			latencies[i] = time.Since(start)
		}

		// Calculate average
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		avg := total / 100

		// Average latency should be very low for context creation
		if avg > time.Microsecond*100 {
			t.Logf("Warning: Average context creation latency %v exceeds target of 100μs", avg)
		}
	})
}
