package common

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestResponses_MoreCoverage(t *testing.T) {
	t.Run("SendNoContent sets 204", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		resp, err := SendNoContent(ctx)
		require.NoError(t, err)
		assert.Equal(t, 204, resp.Status)
	})

	t.Run("SendMastodonError adds description for 429", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		resp, err := SendMastodonError(ctx, 429, "slow down")
		require.NoError(t, err)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))

		assert.Equal(t, "slow down", body["error"])
		assert.Equal(t, "Rate limit exceeded", body["error_description"])
	})

	t.Run("SendStreamingMessage formats SSE", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		resp, err := SendStreamingMessage(ctx, "update", map[string]string{"a": "b"})
		require.NoError(t, err)
		assert.Equal(t, 200, resp.Status)
		assert.NotEmpty(t, resp.Headers["content-type"])
		assert.Contains(t, resp.Headers["content-type"][0], "text/event-stream")

		body := string(resp.Body)
		assert.Contains(t, body, "event: update\n")
		assert.Contains(t, body, "data: ")
		assert.True(t, strings.HasSuffix(body, "\n\n"))
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
		resp := &apptheory.Response{Status: 200}
		SetCORSHeaders(resp)
		SetJSONHeaders(resp)
		SetSecurityHeaders(resp)
		assert.NotEmpty(t, resp.Headers["access-control-allow-origin"])
		assert.NotEmpty(t, resp.Headers["content-type"])
		assert.NotEmpty(t, resp.Headers["x-frame-options"])

		resp2 := &apptheory.Response{Status: 200}
		SetActivityPubHeaders(resp2)
		assert.NotEmpty(t, resp2.Headers["content-type"])
	})

	t.Run("Cache helpers", func(t *testing.T) {
		resp := &apptheory.Response{Status: 200}
		SetCacheHeaders(resp, 60)
		assert.Contains(t, resp.Headers["cache-control"][0], "max-age=")

		resp2 := &apptheory.Response{Status: 200}
		SetNoCache(resp2)
		assert.Contains(t, resp2.Headers["cache-control"][0], "no-cache")
	})

	t.Run("utility senders", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		resp, err := SendEmpty(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.Status)

		ctx2 := newTestContext("GET", "/test")
		_, err = SendBool(ctx2, true)
		require.NoError(t, err)

		ctx3 := newTestContext("GET", "/test")
		_, err = SendCount(ctx3, 5)
		require.NoError(t, err)

		ctx4 := newTestContext("GET", "/test")
		_, err = SendID(ctx4, "id")
		require.NoError(t, err)
	})

	t.Run("rate limit headers", func(t *testing.T) {
		resp := &apptheory.Response{Status: 200}
		SendRateLimitHeaders(resp, 10, 9, time.Now().Unix())
		assert.Equal(t, "10", resp.Headers["x-ratelimit-limit"][0])
		assert.Equal(t, "9", resp.Headers["x-ratelimit-remaining"][0])
		assert.NotEmpty(t, resp.Headers["x-ratelimit-reset"][0])
	})

	t.Run("health check", func(t *testing.T) {
		okCtx := newTestContext("GET", "/test")
		okResp, err := SendHealthCheck(okCtx, HealthCheckResponse{Status: "ok"})
		require.NoError(t, err)
		assert.Equal(t, 200, okResp.Status)

		badCtx := newTestContext("GET", "/test")
		badResp, err := SendHealthCheck(badCtx, HealthCheckResponse{Status: "bad"})
		require.NoError(t, err)
		assert.Equal(t, 503, badResp.Status)
	})
}
