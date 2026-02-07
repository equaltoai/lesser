package common

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func decodeJSON(t *testing.T, resp *apptheory.Response) map[string]any {
	t.Helper()

	require.NotNil(t, resp)
	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	return out
}

func TestResponses_BasicSuccessHelpers(t *testing.T) {
	ctx := newTestContext("POST", "/v1/test")

	resp, err := SendCreated(ctx, map[string]string{"id": "1"})
	require.NoError(t, err)
	assert.Equal(t, 201, resp.Status)
	assert.Equal(t, "1", decodeJSON(t, resp)["id"])

	resp, err = SendAccepted(ctx, map[string]string{"ok": "true"})
	require.NoError(t, err)
	assert.Equal(t, 202, resp.Status)
	assert.Equal(t, "true", decodeJSON(t, resp)["ok"])

	resp, err = SendNoContent(ctx)
	require.NoError(t, err)
	assert.Equal(t, 204, resp.Status)

	resp, err = SendEmptyObject(ctx)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	assert.Empty(t, decodeJSON(t, resp))
}

func TestResponses_MastodonHelpers(t *testing.T) {
	ctx := newTestContext("GET", "/api/v1/test")

	resp, err := SendMastodonError(ctx, 422, "bad")
	require.NoError(t, err)
	body := decodeJSON(t, resp)
	assert.Equal(t, "bad", body["error"])
	assert.Equal(t, string(errors.CodeUnprocessableEntity), body["error_code"])
	assert.Equal(t, "Validation failed", body["error_description"])

	resp, err = SendMastodonError(ctx, 429, "slow down")
	require.NoError(t, err)
	body = decodeJSON(t, resp)
	assert.Equal(t, string(errors.CodeRateLimited), body["error_code"])
	assert.Equal(t, "Rate limit exceeded", body["error_description"])

	account := MastodonAccount{ID: "1", Username: "alice"}
	resp, err = SendMastodonAccount(ctx, account)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)

	resp, err = SendMastodonAccounts(ctx, []MastodonAccount{account})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)

	status := MastodonStatus{ID: "s1", Content: "hello", Account: account}
	resp, err = SendMastodonStatus(ctx, status)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)

	resp, err = SendMastodonStatuses(ctx, []MastodonStatus{status})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
}

func TestResponses_PaginationAndAliases(t *testing.T) {
	ctx := newTestContext("GET", "/api/v1/test", withHeaders(map[string]string{
		"Host":              "example.com",
		"X-Forwarded-Proto": "http",
	}))

	p := &Pagination{Limit: 10, HasNext: true, NextCursor: "next"}
	resp, err := SendPaginatedResponse(ctx, []string{"a"}, p)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	body := decodeJSON(t, resp)
	assert.NotNil(t, body["pagination"])

	resp, err = RespondWithJSON(ctx, 200, map[string]string{"ok": "true"})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)

	errResp, err := RespondWithError(ctx, 400, "bad")
	require.NoError(t, err)
	statusCode, parsed := parseResponse(t, errResp)
	assert.Equal(t, 400, statusCode)
	assert.Equal(t, "bad", parsed.Error)
	assert.Equal(t, string(errors.CodeBadRequest), parsed.Code)

	assert.Equal(t, "http://example.com/api/v1/test", GetBaseURL(ctx))
	assert.Equal(t, "", GetBaseURL(nil))
}

func TestResponses_StreamingAndHeaders(t *testing.T) {
	ctx := newTestContext("GET", "/sse")

	resp, err := SendStreamingMessage(ctx, "event", map[string]string{"ok": "true"})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "text/event-stream; charset=utf-8", resp.Headers["content-type"][0])

	_, err = SendStreamingMessage(ctx, "event", func() {})
	assert.Error(t, err)

	assert.Equal(t, map[string]any{}, ValidateResponseData(nil))
	assert.Equal(t, "x", ValidateResponseData("x"))

	SetCORSHeaders(nil)
	SetActivityPubHeaders(nil)
	SetJSONHeaders(nil)
	SetSecurityHeaders(nil)
	SetCacheHeaders(nil, 1)

	out := &apptheory.Response{}
	SetActivityPubHeaders(out)
	SetSecurityHeaders(out)
	SetCacheHeaders(out, 60)
	assert.NotEmpty(t, out.Headers["cache-control"])

	SetNoCache(out)
	assert.NotEmpty(t, out.Headers["pragma"])

	SendRateLimitHeaders(out, 10, 9, time.Now().Unix())
	assert.NotEmpty(t, out.Headers["x-ratelimit-limit"])
}

func TestResponses_HealthCheck(t *testing.T) {
	ctx := newTestContext("GET", "/health")

	okResp, err := SendHealthCheck(ctx, HealthCheckResponse{Status: "ok"})
	require.NoError(t, err)
	assert.Equal(t, 200, okResp.Status)

	failResp, err := SendHealthCheck(ctx, HealthCheckResponse{Status: "degraded"})
	require.NoError(t, err)
	status, parsed := parseResponse(t, failResp)
	assert.Equal(t, 503, status)
	assert.Equal(t, "Service Unavailable", parsed.Error)
}
