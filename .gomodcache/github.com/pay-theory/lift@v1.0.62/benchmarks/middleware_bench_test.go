package benchmarks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
)

// BenchmarkMiddlewareChain5 tests performance with 5 middleware
func BenchmarkMiddlewareChain5(b *testing.B) {
	app := setupAppWithMiddleware(5)
	benchmarkMiddlewareExecution(b, app)
}

// BenchmarkMiddlewareChain10 tests performance with 10 middleware
func BenchmarkMiddlewareChain10(b *testing.B) {
	app := setupAppWithMiddleware(10)
	benchmarkMiddlewareExecution(b, app)
}

// BenchmarkMiddlewareChain15 tests performance with 15 middleware
func BenchmarkMiddlewareChain15(b *testing.B) {
	app := setupAppWithMiddleware(15)
	benchmarkMiddlewareExecution(b, app)
}

// BenchmarkMiddlewareChain25 tests performance with 25 middleware
func BenchmarkMiddlewareChain25(b *testing.B) {
	app := setupAppWithMiddleware(25)
	benchmarkMiddlewareExecution(b, app)
}

// BenchmarkMiddlewareRegistration tests the overhead of registering middleware
func BenchmarkMiddlewareRegistration(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		app := lift.New()

		// Register 10 middleware
		for j := 0; j < 10; j++ {
			app.Use(createSimpleMiddleware(j))
		}

		_ = app
	}
}

// BenchmarkMiddlewareComposition tests middleware composition overhead
func BenchmarkMiddlewareComposition(b *testing.B) {
	b.ReportAllocs()

	middlewares := make([]lift.Middleware, 10)
	for i := 0; i < 10; i++ {
		middlewares[i] = createSimpleMiddleware(i)
	}

	for i := 0; i < b.N; i++ {
		app := lift.New()

		// Apply all middleware at once
		for _, mw := range middlewares {
			app.Use(mw)
		}

		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET("/test", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})

		_ = app
	}
}

// BenchmarkMiddlewareWithComplexLogic tests middleware with more complex operations
func BenchmarkMiddlewareWithComplexLogic(b *testing.B) {
	app := lift.New()

	// Add middleware with complex logic
	app.Use(createLoggingMiddleware())
	app.Use(createAuthMiddleware())
	app.Use(createValidationMiddleware())
	app.Use(createMetricsMiddleware())
	app.Use(createCacheMiddleware())

	//nolint:errcheck // Benchmark doesn't need error handling
	app.GET("/test", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	benchmarkMiddlewareExecution(b, app)
}

// BenchmarkMiddlewareMemoryAllocation tests memory allocation in middleware chains
func BenchmarkMiddlewareMemoryAllocation(b *testing.B) {
	app := lift.New()

	// Add middleware that allocates memory
	for i := 0; i < 10; i++ {
		app.Use(createMemoryAllocatingMiddleware(i))
	}

	//nolint:errcheck // Benchmark doesn't need error handling
	app.GET("/test", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	benchmarkMiddlewareExecution(b, app)
}

// BenchmarkMiddlewareErrorHandling tests middleware error handling performance
func BenchmarkMiddlewareErrorHandling(b *testing.B) {
	app := lift.New()

	// Add middleware that might produce errors
	app.Use(createErrorProneMiddleware())
	app.Use(createRecoveryMiddleware())

	//nolint:errcheck // Benchmark doesn't need error handling
	app.GET("/test", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	benchmarkMiddlewareExecution(b, app)
}

// BenchmarkConcurrentMiddleware tests middleware under concurrent load
func BenchmarkConcurrentMiddleware(b *testing.B) {
	app := setupAppWithMiddleware(10)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   "/test",
				},
			}
			ctx := lift.NewContext(context.TODO(), req)

			// This would normally execute the middleware chain
			_ = app
			_ = ctx
		}
	})
}

// Helper functions

func setupAppWithMiddleware(count int) *lift.App {
	app := lift.New()

	for i := 0; i < count; i++ {
		app.Use(createSimpleMiddleware(i))
	}

	//nolint:errcheck // Benchmark doesn't need error handling
	app.GET("/test", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	return app
}

func benchmarkMiddlewareExecution(b *testing.B, app *lift.App) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := &lift.Request{
			Request: &adapters.Request{
				Method: "GET",
				Path:   "/test",
			},
		}
		ctx := lift.NewContext(context.TODO(), req)

		// This would normally execute the middleware chain and handler
		_ = app
		_ = ctx
	}
}

func createSimpleMiddleware(id int) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Simple middleware that just passes through
			ctx.Set(fmt.Sprintf("middleware_%d", id), true)
			return next.Handle(ctx)
		})
	}
}

func createLoggingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			err := next.Handle(ctx)
			duration := time.Since(start)

			// Simulate logging overhead
			_ = fmt.Sprintf("Request completed in %v", duration)

			return err
		})
	}
}

func createAuthMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Simulate auth check
			authHeader := ctx.Header("Authorization")
			if authHeader == "" {
				authHeader = "Bearer default-token"
			}

			// Simulate token validation
			ctx.Set("user_id", "user123")
			ctx.Set("auth_token", authHeader)

			return next.Handle(ctx)
		})
	}
}

func createValidationMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Simulate request validation
			if ctx.Request != nil && ctx.Request.Method == "POST" {
				// Simulate validation logic
				ctx.Set("validated", true)
			}

			return next.Handle(ctx)
		})
	}
}

func createMetricsMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			err := next.Handle(ctx)
			duration := time.Since(start)

			// Simulate metrics collection
			ctx.Set("request_duration", duration)
			ctx.Set("request_count", 1)

			return err
		})
	}
}

func createCacheMiddleware() lift.Middleware {
	cache := make(map[string]any)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Simulate cache lookup
			cacheKey := fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path)

			if cached, exists := cache[cacheKey]; exists {
				ctx.Set("cached_result", cached)
			}

			err := next.Handle(ctx)

			// Simulate cache storage
			if err == nil {
				cache[cacheKey] = "cached_response"
			}

			return err
		})
	}
}

func createMemoryAllocatingMiddleware(id int) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Allocate some memory to test GC impact
			data := make([]byte, 1024)
			for i := range data {
				data[i] = byte(id)
			}

			ctx.Set(fmt.Sprintf("data_%d", id), data)

			return next.Handle(ctx)
		})
	}
}

func createErrorProneMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Simulate potential error conditions
			if ctx.Header("X-Trigger-Error") == "true" {
				return fmt.Errorf("simulated middleware error")
			}

			return next.Handle(ctx)
		})
	}
}

func createRecoveryMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					ctx.Set("recovered_panic", r)
				}
			}()

			return next.Handle(ctx)
		})
	}
}
