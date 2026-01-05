package common

import (
	"encoding/json"
	"testing"
	"time"

	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponses_MoreCoverage(t *testing.T) {
	t.Run("SendNoContent sets 204", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendNoContent(ctx))
		assert.Equal(t, 204, ctx.Response.StatusCode)
	})

	t.Run("SendMastodonError adds description for 429", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendMastodonError(ctx, 429, "slow down"))

		var body map[string]any
		switch v := ctx.Response.Body.(type) {
		case map[string]any:
			body = v
		case []byte:
			require.NoError(t, json.Unmarshal(v, &body))
		default:
			t.Fatalf("unexpected response body type %T", ctx.Response.Body)
		}

		assert.Equal(t, "slow down", body["error"])
		assert.Equal(t, "Rate limit exceeded", body["error_description"])
	})

	t.Run("SendStreamingMessage formats SSE", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendStreamingMessage(ctx, "update", map[string]string{"a": "b"}))
		assert.Equal(t, 200, ctx.Response.StatusCode)
		assert.NotEmpty(t, ctx.Response.Headers["Content-Type"])

		var payload map[string]string
		switch v := ctx.Response.Body.(type) {
		case map[string]string:
			payload = v
		case []byte:
			require.NoError(t, json.Unmarshal(v, &payload))
		default:
			t.Fatalf("unexpected response body type %T", ctx.Response.Body)
		}
		assert.Contains(t, payload["sse"], "event: update")
	})

	t.Run("CreateWebSocketMessage", func(t *testing.T) {
		msg := CreateWebSocketMessage("event", []string{"stream"}, "update", map[string]any{"a": 1})
		assert.Equal(t, "event", msg.Type)
		assert.Equal(t, []string{"stream"}, msg.Stream)
		assert.Equal(t, "update", msg.Event)
	})

	t.Run("ValidateResponseData", func(t *testing.T) {
		assert.NotNil(t, ValidateResponseData(nil))
		assert.Equal(t, "x", ValidateResponseData("x"))
	})

	t.Run("Header helpers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		SetCORSHeaders(ctx)
		SetJSONHeaders(ctx)
		SetActivityPubHeaders(ctx)
		SetSecurityHeaders(ctx)
		assert.NotEmpty(t, ctx.Response.Headers["Access-Control-Allow-Origin"])
		assert.NotEmpty(t, ctx.Response.Headers["Content-Type"])
		assert.NotEmpty(t, ctx.Response.Headers["X-Frame-Options"])
	})

	t.Run("Cache helpers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		SetCacheHeaders(ctx, 60)
		assert.Contains(t, ctx.Response.Headers["Cache-Control"], "max-age=")

		ctx2 := liftTesting.MockLiftContext("GET", "/test")
		SetNoCache(ctx2)
		assert.Contains(t, ctx2.Response.Headers["Cache-Control"], "no-cache")
	})

	t.Run("utility senders", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendEmpty(ctx))
		assert.Equal(t, 200, ctx.Response.StatusCode)

		ctx2 := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendBool(ctx2, true))

		ctx3 := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendCount(ctx3, 5))

		ctx4 := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendID(ctx4, "id"))
	})

	t.Run("rate limit headers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		SendRateLimitHeaders(ctx, 10, 9, time.Now().Unix())
		assert.Equal(t, "10", ctx.Response.Headers["X-RateLimit-Limit"])
		assert.Equal(t, "9", ctx.Response.Headers["X-RateLimit-Remaining"])
		assert.NotEmpty(t, ctx.Response.Headers["X-RateLimit-Reset"])
	})

	t.Run("health check", func(t *testing.T) {
		okCtx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendHealthCheck(okCtx, HealthCheckResponse{Status: "ok"}))
		assert.Equal(t, 200, okCtx.Response.StatusCode)

		badCtx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, SendHealthCheck(badCtx, HealthCheckResponse{Status: "bad"}))
		assert.Equal(t, 503, badCtx.Response.StatusCode)
	})
}
