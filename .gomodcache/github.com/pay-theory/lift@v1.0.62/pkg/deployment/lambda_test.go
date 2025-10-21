package deployment_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pay-theory/lift/pkg/deployment"
	"github.com/pay-theory/lift/pkg/lift"
)

func TestLambdaDeploymentHandlesHTTPAndSQSEvents(t *testing.T) {
	app := lift.New()

	var httpMiddlewareCount int
	var globalMiddlewareCount int

	app.Use(lift.MarkGlobalMiddleware(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			globalMiddlewareCount++
			ctx.Set("saw_global_middleware", true)
			return next.Handle(ctx)
		})
	}))

	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			httpMiddlewareCount++
			return next.Handle(ctx)
		})
	})

	require.NoError(t, app.GET("/", func(ctx *lift.Context) error {
		require.True(t, ctx.Get("saw_global_middleware").(bool))
		return ctx.JSON(map[string]string{"message": "ok"})
	}))

	require.NoError(t, app.SQS("test-queue", func(ctx *lift.Context) error {
		require.True(t, ctx.Get("saw_global_middleware").(bool))
		records, err := ctx.SQSRecords()
		require.NoError(t, err)
		require.Len(t, records, 1)
		return nil
	}))

	config := deployment.DefaultDeploymentConfig()
	config.MetricsEnabled = false
	config.TracingEnabled = false

	deploy, err := deployment.NewLambdaDeployment(app, config)
	require.NoError(t, err)

	handler := deploy.Handler()

	httpEvent := map[string]any{
		"version":         "2.0",
		"routeKey":        "GET /",
		"rawPath":         "/",
		"rawQueryString":  "",
		"requestContext":  map[string]any{"http": map[string]any{"method": "GET", "path": "/"}},
		"headers":         map[string]any{"host": "example.com"},
		"isBase64Encoded": false,
	}

	httpPayload, err := json.Marshal(httpEvent)
	require.NoError(t, err)

	respBytes, err := handler.Invoke(context.Background(), httpPayload)
	require.NoError(t, err)
	require.NotNil(t, respBytes)
	var httpResponse map[string]any
	require.NoError(t, json.Unmarshal(respBytes, &httpResponse))
	require.Equal(t, float64(200), httpResponse["statusCode"])

	require.Equal(t, 1, httpMiddlewareCount)
	require.Equal(t, 1, globalMiddlewareCount)

	sqsEvent := map[string]any{
		"Records": []any{
			map[string]any{
				"messageId":      "1",
				"receiptHandle":  "abc",
				"body":           "{\"order_id\":42}",
				"eventSource":    "aws:sqs",
				"eventSourceARN": "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
		},
	}

	sqsPayload, err := json.Marshal(sqsEvent)
	require.NoError(t, err)

	sqsResp, err := handler.Invoke(context.Background(), sqsPayload)
	require.NoError(t, err)
	if len(sqsResp) > 0 {
		var sqsResponse map[string]any
		require.NoError(t, json.Unmarshal(sqsResp, &sqsResponse))
		require.Equal(t, float64(200), sqsResponse["statusCode"])
	}

	require.Equal(t, 1, httpMiddlewareCount, "HTTP-only middleware should not run for SQS events")
	require.Equal(t, 2, globalMiddlewareCount, "Global middleware should execute for both HTTP and SQS events")
}
