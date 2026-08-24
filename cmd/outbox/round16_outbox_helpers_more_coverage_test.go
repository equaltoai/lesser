package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func TestOutboxQueryAndHeaderHelpers_Round16(t *testing.T) {
	require.Equal(t, "", outboxQueryValue(nil, "page"))
	require.Equal(t, "", outboxHeaderValue(nil, "authorization"))

	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Query: map[string][]string{
				"page": {"1"},
			},
			Headers: map[string][]string{
				"authorization": {"Bearer t"},
			},
		},
	}
	require.Equal(t, "", outboxQueryValue(ctx, "missing"))
	require.Equal(t, "1", outboxQueryValue(ctx, "page"))
	require.Equal(t, "", outboxHeaderValue(ctx, "missing"))
	require.Equal(t, "Bearer t", outboxHeaderValue(ctx, " Authorization "))
}

func TestOutboxRequestID_Round16(t *testing.T) {
	require.Equal(t, "req-1", outboxRequestID(&apptheory.Context{RequestID: "req-1"}, "ignored"))
	require.True(t, strings.HasPrefix(outboxRequestID(nil, ""), "outbox-"))
	require.True(t, strings.HasPrefix(outboxRequestID(nil, "prefix"), "prefix-"))
	require.True(t, strings.HasPrefix(outboxRequestID(&apptheory.Context{RequestID: " "}, "prefix"), "prefix-"))
	require.True(t, strings.HasPrefix(outboxRequestID(&apptheory.Context{RequestID: ""}, ""), "outbox-"))
}

func TestOutboxJSONHelpers_Round16(t *testing.T) {
	resp := outboxJSONError(500, "  ")
	require.Equal(t, 500, resp.Status)

	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "internal server error", body["error"])

	resp, err := outboxActivityJSON(200, map[string]any{"ok": true})
	require.NoError(t, err)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, []string{contentTypeActivityJSON}, resp.Headers["content-type"])
	require.NotEmpty(t, resp.Body)

	resp, err = outboxActivityJSON(200, map[string]any{"ch": make(chan int)})
	require.Error(t, err)
	require.Nil(t, resp)
}
