package lift_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pay-theory/lift/pkg/lift"
)

func TestMetricsFactoryHonorsToggle(t *testing.T) {
	metricsCreated := 0

	app := lift.New(lift.WithConfig(&lift.Config{MetricsEnabled: true}))
	app.WithMetricsFactory(func(*lift.Config) lift.MetricsCollector {
		metricsCreated++
		return &lift.NoOpMetrics{}
	})

	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			require.NotNil(t, ctx.Metrics)
			return next.Handle(ctx)
		})
	})

	require.NoError(t, app.GET("/metrics", func(ctx *lift.Context) error {
		require.NotNil(t, ctx.Metrics)
		return ctx.JSON(map[string]string{"ok": "true"})
	}))

	_, err := app.HandleRequest(context.Background(), newAPIGatewayEvent("GET", "/metrics"))
	require.NoError(t, err)
	require.Equal(t, 1, metricsCreated)
}

func TestMetricsFactoryDisabled(t *testing.T) {
	metricsCreated := 0

	app := lift.New(lift.WithConfig(&lift.Config{MetricsEnabled: false}))
	app.WithMetricsFactory(func(*lift.Config) lift.MetricsCollector {
		metricsCreated++
		return &lift.NoOpMetrics{}
	})

	require.NoError(t, app.GET("/metrics-disabled", func(ctx *lift.Context) error {
		require.Nil(t, ctx.Metrics)
		return ctx.JSON(map[string]string{"ok": "true"})
	}))

	_, err := app.HandleRequest(context.Background(), newAPIGatewayEvent("GET", "/metrics-disabled"))
	require.NoError(t, err)
	require.Equal(t, 0, metricsCreated)
}

func TestTracerFactoryHonorsToggle(t *testing.T) {
	app := lift.New(lift.WithConfig(&lift.Config{TracingEnabled: true}))
	app.WithTracerFactory(func(*lift.Config) any {
		return "test-tracer"
	})

	require.NoError(t, app.GET("/trace", func(ctx *lift.Context) error {
		require.Equal(t, "test-tracer", ctx.GetTracer())
		return ctx.JSON(map[string]string{"ok": "true"})
	}))

	_, err := app.HandleRequest(context.Background(), newAPIGatewayEvent("GET", "/trace"))
	require.NoError(t, err)
}

func TestTracerFactoryDisabled(t *testing.T) {
	app := lift.New(lift.WithConfig(&lift.Config{TracingEnabled: false}))
	app.WithTracerFactory(func(*lift.Config) any {
		return "should-not-be-used"
	})

	require.NoError(t, app.GET("/trace-off", func(ctx *lift.Context) error {
		require.Nil(t, ctx.GetTracer())
		return ctx.JSON(map[string]string{"ok": "true"})
	}))

	_, err := app.HandleRequest(context.Background(), newAPIGatewayEvent("GET", "/trace-off"))
	require.NoError(t, err)
}

func newAPIGatewayEvent(method, path string) map[string]any {
	return map[string]any{
		"version":  "2.0",
		"routeKey": method + " " + path,
		"rawPath":  path,
		"requestContext": map[string]any{
			"http": map[string]any{"method": method, "path": path},
		},
		"headers":         map[string]any{},
		"isBase64Encoded": false,
	}
}
