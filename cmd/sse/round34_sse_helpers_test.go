package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

func TestSSEPanicRecovery_Round34(t *testing.T) {
	mw := ssePanicRecovery(nil)

	t.Run("passes through when no panic", func(t *testing.T) {
		next := func(_ *apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		}

		handler := mw(next)
		resp, err := handler(&apptheory.Context{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("recovers panic and returns 500", func(t *testing.T) {
		next := func(_ *apptheory.Context) (*apptheory.Response, error) {
			panic("boom")
		}

		handler := mw(next)
		resp, err := handler(&apptheory.Context{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})
}

func TestSSEHeaderValueAndQueryParam_Round34(t *testing.T) {
	require.Equal(t, "", sseHeaderValue(nil, "x"))
	require.Equal(t, "", sseQueryParam(nil, "x"))

	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Headers: map[string][]string{
				"authorization": {"Bearer token"},
			},
			Query: map[string][]string{
				"tag": {"cats"},
			},
		},
	}

	require.Equal(t, "Bearer token", sseHeaderValue(ctx, " Authorization "))
	require.Equal(t, "", sseHeaderValue(ctx, "missing"))

	require.Equal(t, "", sseQueryParam(ctx, ""))
	require.Equal(t, "cats", sseQueryParam(ctx, "tag"))
	require.Equal(t, "", sseQueryParam(ctx, "missing"))
}
