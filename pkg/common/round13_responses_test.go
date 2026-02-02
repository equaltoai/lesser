package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
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
	ctx := newTestContext("GET", "/test")
	resp, err := SendError(ctx, 404, "missing")
	require.NoError(t, err)
	assert.Equal(t, 404, resp.Status)

	var std StandardErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body, &std))
	assert.Equal(t, "missing", std.Error)
	assert.Equal(t, "NOT_FOUND", std.Code)

	ctx2 := newTestContext("GET", "/test")
	resp2, err := SendMastodonError(ctx2, 422, "bad input")
	require.NoError(t, err)
	assert.Equal(t, 422, resp2.Status)

	var m map[string]any
	require.NoError(t, json.Unmarshal(resp2.Body, &m))
	assert.Equal(t, "bad input", m["error"])
	assert.Equal(t, "UNPROCESSABLE_ENTITY", m["error_code"])
	assert.Equal(t, "Validation failed", m["error_description"])
}

func TestGetBaseURL_AndPaginatedMastodonResponse_LinkHeader(t *testing.T) {
	ctx := newTestContext("GET", "/api/v1/test", func(ctx *apptheory.Context) {
		ctx.Request.Headers = map[string][]string{
			"host":              {"example.com"},
			"x-forwarded-proto": {"http"},
		}
	})

	assert.Equal(t, "http://example.com/api/v1/test", GetBaseURL(ctx))

	params := PaginationParams{Limit: 20}
	resp, err := SendPaginatedMastodonResponse(ctx, []string{"a"}, params, true, false, "next", "")
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Headers["link"])
}
