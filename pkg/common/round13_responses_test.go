package common

import (
	"encoding/json"
	"testing"

	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorCodeForHTTPStatus_MappingAndDefaults(t *testing.T) {
	assert.Equal(t, "BAD_REQUEST", errorCodeForHTTPStatus(400))
	assert.Equal(t, "UNAUTHORIZED", errorCodeForHTTPStatus(401))
	assert.Equal(t, "FORBIDDEN", errorCodeForHTTPStatus(403))
	assert.Equal(t, "NOT_FOUND", errorCodeForHTTPStatus(404))
	assert.Equal(t, "METHOD_NOT_ALLOWED", errorCodeForHTTPStatus(405))
	assert.Equal(t, "CONFLICT", errorCodeForHTTPStatus(409))
	assert.Equal(t, "GONE", errorCodeForHTTPStatus(410))
	assert.Equal(t, "UNPROCESSABLE_ENTITY", errorCodeForHTTPStatus(422))
	assert.Equal(t, "RATE_LIMITED", errorCodeForHTTPStatus(429))
	assert.Equal(t, "EXTERNAL_SERVICE_UNAVAILABLE", errorCodeForHTTPStatus(503))

	assert.Equal(t, "BAD_REQUEST", errorCodeForHTTPStatus(418)) // generic 4xx
	assert.Equal(t, "INTERNAL_ERROR", errorCodeForHTTPStatus(599))
}

func TestSendError_AndMastodonError(t *testing.T) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	err := SendError(ctx, 404, "missing")
	require.NoError(t, err)
	assert.Equal(t, 404, ctx.Response.StatusCode)

	var std StandardErrorResponse
	switch body := ctx.Response.Body.(type) {
	case StandardErrorResponse:
		std = body
	case []byte:
		require.NoError(t, json.Unmarshal(body, &std))
	default:
		t.Fatalf("unexpected response body type %T", ctx.Response.Body)
	}
	assert.Equal(t, "missing", std.Error)
	assert.Equal(t, "NOT_FOUND", std.Code)

	ctx2 := liftTesting.MockLiftContext("GET", "/test")
	err = SendMastodonError(ctx2, 422, "bad input")
	require.NoError(t, err)
	assert.Equal(t, 422, ctx2.Response.StatusCode)

	var m map[string]any
	switch body := ctx2.Response.Body.(type) {
	case map[string]any:
		m = body
	case []byte:
		require.NoError(t, json.Unmarshal(body, &m))
	default:
		t.Fatalf("unexpected response body type %T", ctx2.Response.Body)
	}
	assert.Equal(t, "bad input", m["error"])
	assert.Equal(t, "UNPROCESSABLE_ENTITY", m["error_code"])
	assert.Equal(t, "Validation failed", m["error_description"])
}

func TestGetBaseURL_AndPaginatedMastodonResponse_LinkHeader(t *testing.T) {
	ctx := liftTesting.MockLiftContext("GET", "/api/v1/test", liftTesting.WithHeaders(map[string]string{
		"Host":              "example.com",
		"X-Forwarded-Proto": "http",
	}))
	ctx.Request.Request.Headers["X-Forwarded-Proto"] = "http"

	assert.Equal(t, "http://example.com/api/v1/test", GetBaseURL(ctx))

	params := PaginationParams{Limit: 20}
	err := SendPaginatedMastodonResponse(ctx, []string{"a"}, params, true, false, "next", "")
	require.NoError(t, err)
	assert.NotEmpty(t, ctx.Response.Headers["Link"])
}
