package benchmarks

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// Simple middleware functions for benchmarking
func loggerMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return next.Handle(ctx)
		})
	}
}

func recoverMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					// Log or handle panic if needed in the future
					_ = r
				}
			}()
			return next.Handle(ctx)
		})
	}
}

func corsMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return next.Handle(ctx)
		})
	}
}

func requestIDMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.RequestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
			return next.Handle(ctx)
		})
	}
}

func timeoutMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return next.Handle(ctx)
		})
	}
}

func metricsMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return next.Handle(ctx)
		})
	}
}

// BenchmarkColdStart measures the overhead of initializing the Lift framework
func BenchmarkColdStart(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		app := lift.New()
		_ = app
	}
}

// BenchmarkColdStartWithBasicRoute measures initialization with a single route
func BenchmarkColdStartWithBasicRoute(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		app := lift.New()
		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET("/health", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})
		_ = app
	}
}

// BenchmarkColdStartWithMiddleware measures initialization with common middleware
func BenchmarkColdStartWithMiddleware(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		app := lift.New()
		app.Use(loggerMiddleware())
		app.Use(recoverMiddleware())
		app.Use(corsMiddleware())
		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET("/health", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})
		_ = app
	}
}

// BenchmarkColdStartWithEventAdapters measures initialization with event adapters
func BenchmarkColdStartWithEventAdapters(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		app := lift.New()

		// Event adapters are automatically registered in the adapter registry
		// No explicit registration needed - they're available by default

		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET("/health", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})
		_ = app
	}
}

// BenchmarkFrameworkInitializationTime measures actual time to initialize
func BenchmarkFrameworkInitializationTime(b *testing.B) {
	b.ReportAllocs()

	var totalDuration time.Duration

	for i := 0; i < b.N; i++ {
		start := time.Now()

		app := lift.New()
		app.Use(loggerMiddleware())
		app.Use(recoverMiddleware())
		app.Use(corsMiddleware())
		app.Use(requestIDMiddleware())
		app.Use(timeoutMiddleware())

		// Add multiple routes
		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET("/health", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})
		//nolint:errcheck // Benchmark doesn't need error handling
		app.POST("/users", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"message": "created"})
		})
		//nolint:errcheck // Benchmark doesn't need error handling
		app.PUT("/users/:id", func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"message": "updated"})
		})
		//nolint:errcheck // Benchmark doesn't need error handling
		app.DELETE("/users/:id", func(ctx *lift.Context) error {
			return ctx.JSON(nil)
		})

		duration := time.Since(start)
		totalDuration += duration

		_ = app
	}

	avgDuration := totalDuration / time.Duration(b.N)
	b.ReportMetric(float64(avgDuration.Nanoseconds()), "ns/init")
	b.ReportMetric(float64(avgDuration.Microseconds()), "μs/init")
	b.ReportMetric(float64(avgDuration.Milliseconds()), "ms/init")
}

// BenchmarkMemoryAllocationDuringInit measures memory allocations during initialization
func BenchmarkMemoryAllocationDuringInit(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		app := lift.New()

		// Simulate typical application setup
		app.Use(loggerMiddleware())
		app.Use(recoverMiddleware())
		app.Use(corsMiddleware())
		app.Use(requestIDMiddleware())
		app.Use(metricsMiddleware())

		// Add routes
		for j := 0; j < 10; j++ {
			path := fmt.Sprintf("/api/v1/resource%d", j)
			//nolint:errcheck // Benchmark doesn't need error handling
			app.GET(path, func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{"id": ctx.Param("id")})
			})
			//nolint:errcheck // Benchmark doesn't need error handling
			app.POST(path, func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{"message": "created"})
			})
		}

		_ = app
	}
}

// BenchmarkGarbageCollectionImpact measures GC impact during initialization
func BenchmarkGarbageCollectionImpact(b *testing.B) {
	b.ReportAllocs()

	// Force GC before benchmark
	runtime.GC()

	var gcBefore, gcAfter runtime.MemStats
	runtime.ReadMemStats(&gcBefore)

	for i := 0; i < b.N; i++ {
		app := lift.New()

		// Heavy initialization to trigger potential GC
		for j := 0; j < 100; j++ {
			path := fmt.Sprintf("/api/v1/resource%d/:id", j)
			//nolint:errcheck // Benchmark doesn't need error handling
			app.GET(path, func(ctx *lift.Context) error {
				return ctx.JSON(map[string]any{
					"id":   ctx.Param("id"),
					"data": make([]byte, 1024), // Allocate some memory
				})
			})
		}

		// Add all middleware
		app.Use(loggerMiddleware())
		app.Use(recoverMiddleware())
		app.Use(corsMiddleware())
		app.Use(requestIDMiddleware())
		app.Use(metricsMiddleware())
		app.Use(timeoutMiddleware())

		_ = app
	}

	runtime.ReadMemStats(&gcAfter)

	b.ReportMetric(float64(gcAfter.NumGC-gcBefore.NumGC), "gc-cycles")
	b.ReportMetric(float64(gcAfter.PauseTotalNs-gcBefore.PauseTotalNs), "gc-pause-ns")
}

// BenchmarkConcurrentInitialization measures performance under concurrent initialization
func BenchmarkConcurrentInitialization(b *testing.B) {
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			app := lift.New()
			app.Use(loggerMiddleware())
			app.Use(recoverMiddleware())
			//nolint:errcheck // Benchmark doesn't need error handling
			app.GET("/health", func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{"status": "ok"})
			})
			_ = app
		}
	})
}
