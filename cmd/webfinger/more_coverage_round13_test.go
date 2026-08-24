package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

func TestWebFingerHelperFunctions_Round13(t *testing.T) {
	t.Run("webfingerQueryValue handles nil and empty inputs", func(t *testing.T) {
		require.Equal(t, "", webfingerQueryValue(nil, "resource"))
		require.Equal(t, "", webfingerQueryValue(&apptheory.Context{}, ""))
		require.Equal(t, "", webfingerQueryValue(&apptheory.Context{}, "   "))
	})

	t.Run("webfingerQueryValue reads first value", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Query: map[string][]string{
					"resource": {"acct:alice@example.com"},
				},
			},
		}
		require.Equal(t, "acct:alice@example.com", webfingerQueryValue(ctx, "resource"))
	})

	t.Run("webfingerRequestID prefers request id and generates fallback", func(t *testing.T) {
		require.Equal(t, "rid", webfingerRequestID(&apptheory.Context{RequestID: "rid"}, "webfinger"))
		id := webfingerRequestID(&apptheory.Context{}, "")
		require.True(t, strings.HasPrefix(id, "webfinger-"))
	})

	t.Run("webfingerContextRequestID prefers stored requestID", func(t *testing.T) {
		ctx := &apptheory.Context{RequestID: "rid"}
		ctx.Set("requestID", "stored")
		require.Equal(t, "stored", webfingerContextRequestID(ctx))
		require.Equal(t, "rid", webfingerContextRequestID(&apptheory.Context{RequestID: "rid"}))
		require.Equal(t, "", webfingerContextRequestID(nil))
	})

	t.Run("webfingerJSONError defaults message", func(t *testing.T) {
		resp := webfingerJSONError(http.StatusInternalServerError, "")
		require.Equal(t, http.StatusInternalServerError, resp.Status)
		require.Contains(t, string(resp.Body), "internal server error")
	})

	t.Run("webfingerJRDJSON marshals response and errors on invalid value", func(t *testing.T) {
		resp, err := webfingerJRDJSON(200, map[string]any{"subject": "acct:alice@example.com"})
		require.NoError(t, err)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"application/jrd+json"}, resp.Headers["content-type"])

		_, err = webfingerJRDJSON(200, make(chan int))
		require.Error(t, err)
	})

	t.Run("safeConfigDomain handles nil", func(t *testing.T) {
		require.Equal(t, "", safeConfigDomain(nil))
		require.Equal(t, "example.com", safeConfigDomain(&config.Config{Domain: "example.com"}))
	})

	t.Run("webfingerPanicRecovery converts panic to 500 response", func(t *testing.T) {
		mw := webfingerPanicRecovery(zap.NewNop())
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			panic("boom")
		})(&apptheory.Context{})
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})
}
