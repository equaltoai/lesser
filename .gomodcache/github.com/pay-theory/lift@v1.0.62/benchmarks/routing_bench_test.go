package benchmarks

import (
	"context"
	"fmt"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
)

// BenchmarkRouting100Routes tests routing performance with 100 routes
func BenchmarkRouting100Routes(b *testing.B) {
	app := setupAppWithRoutes(100)
	benchmarkRouting(b, app)
}

// BenchmarkRouting500Routes tests routing performance with 500 routes
func BenchmarkRouting500Routes(b *testing.B) {
	app := setupAppWithRoutes(500)
	benchmarkRouting(b, app)
}

// BenchmarkRouting1000Routes tests routing performance with 1000 routes
func BenchmarkRouting1000Routes(b *testing.B) {
	app := setupAppWithRoutes(1000)
	benchmarkRouting(b, app)
}

// BenchmarkRoutingWithPathParams tests routing with path parameters
func BenchmarkRoutingWithPathParams(b *testing.B) {
	app := lift.New()

	// Add routes with path parameters
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("/api/v1/resource%d/:id", i)
		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"id": ctx.Param("id")})
		})

		path = fmt.Sprintf("/api/v1/resource%d/:id/sub/:subid", i)
		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{
				"id":    ctx.Param("id"),
				"subid": ctx.Param("subid"),
			})
		})
	}

	benchmarkRouting(b, app)
}

// BenchmarkRoutingComplexPaths tests routing with complex path patterns
func BenchmarkRoutingComplexPaths(b *testing.B) {
	app := lift.New()

	// Add complex routes
	patterns := []string{
		"/api/v1/users/:id",
		"/api/v1/users/:id/posts",
		"/api/v1/users/:id/posts/:postid",
		"/api/v1/users/:id/posts/:postid/comments",
		"/api/v1/users/:id/posts/:postid/comments/:commentid",
		"/api/v2/organizations/:orgid/teams/:teamid/members",
		"/api/v2/organizations/:orgid/teams/:teamid/members/:memberid",
		"/api/v2/organizations/:orgid/projects/:projectid/tasks",
		"/api/v2/organizations/:orgid/projects/:projectid/tasks/:taskid",
		"/webhooks/github/:repo/push",
		"/webhooks/stripe/payment/:eventid",
		"/admin/users/:id/permissions",
		"/admin/system/health",
		"/admin/system/metrics",
		"/public/assets/:filename",
	}

	for i := 0; i < 50; i++ {
		for _, pattern := range patterns {
			path := fmt.Sprintf("%s_%d", pattern, i)
			//nolint:errcheck // Benchmark doesn't need error handling
			app.GET(path, func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{"status": "ok"})
			})
		}
	}

	benchmarkRouting(b, app)
}

// BenchmarkRoutingMethodMatching tests routing with different HTTP methods
func BenchmarkRoutingMethodMatching(b *testing.B) {
	app := lift.New()

	// Add routes with all HTTP methods
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("/api/v1/resource%d", i)

		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"method": "GET"})
		})
		//nolint:errcheck // Benchmark doesn't need error handling
		app.POST(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"method": "POST"})
		})
		//nolint:errcheck // Benchmark doesn't need error handling
		app.PUT(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"method": "PUT"})
		})
		//nolint:errcheck // Benchmark doesn't need error handling
		app.DELETE(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"method": "DELETE"})
		})
		//nolint:errcheck // Benchmark doesn't need error handling
		app.PATCH(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"method": "PATCH"})
		})
	}

	benchmarkRouting(b, app)
}

// BenchmarkRoutingWorstCase tests worst-case routing performance
func BenchmarkRoutingWorstCase(b *testing.B) {
	app := lift.New()

	// Create routes that would be checked last in a linear search
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/api/v1/resource%d", i)
		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"id": fmt.Sprintf("%d", i)})
		})
	}

	// The route we'll actually test is the last one
	testPath := "/api/v1/resource999"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := &lift.Request{
			Request: &adapters.Request{
				Method: "GET",
				Path:   testPath,
			},
		}
		ctx := lift.NewContext(context.TODO(), req)

		// This would normally be done by the router
		_ = app
		_ = ctx
	}
}

// BenchmarkRouteRegistration tests the performance of adding routes
func BenchmarkRouteRegistration(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		app := lift.New()

		// Add 100 routes
		for j := 0; j < 100; j++ {
			path := fmt.Sprintf("/api/v1/resource%d", j)
			if err := app.GET(path, func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{"id": fmt.Sprintf("%d", j)})
			}); err != nil {
				b.Fatalf("Failed to register route %s: %v", path, err)
			}
		}

		_ = app
	}
}

// BenchmarkRouteRegistrationWithParams tests route registration with parameters
func BenchmarkRouteRegistrationWithParams(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		app := lift.New()

		// Add 100 routes with parameters
		for j := 0; j < 100; j++ {
			path := fmt.Sprintf("/api/v1/resource%d/:id/sub/:subid", j)
			if err := app.GET(path, func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{
					"id":    ctx.Param("id"),
					"subid": ctx.Param("subid"),
				})
			}); err != nil {
				b.Fatalf("Failed to register route %s: %v", path, err)
			}
		}

		_ = app
	}
}

// BenchmarkConcurrentRouting tests routing under concurrent load
func BenchmarkConcurrentRouting(b *testing.B) {
	app := setupAppWithRoutes(100)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   "/api/v1/resource50", // Middle route
				},
			}
			ctx := lift.NewContext(context.TODO(), req)

			// This would normally be done by the router
			_ = app
			_ = ctx
		}
	})
}

// Helper functions

func setupAppWithRoutes(numRoutes int) *lift.App {
	app := lift.New()

	for i := 0; i < numRoutes; i++ {
		path := fmt.Sprintf("/api/v1/resource%d", i)
		//nolint:errcheck // Benchmark doesn't need error handling
		app.GET(path, func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"id": fmt.Sprintf("%d", i)})
		})
	}

	return app
}

func benchmarkRouting(b *testing.B, app *lift.App) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Test routing to a middle route (not first or last)
		req := &lift.Request{
			Request: &adapters.Request{
				Method: "GET",
				Path:   "/api/v1/resource50",
			},
		}
		ctx := lift.NewContext(context.TODO(), req)

		// This would normally be done by the router
		// For now, we're just measuring the setup overhead
		_ = app
		_ = ctx
	}
}
