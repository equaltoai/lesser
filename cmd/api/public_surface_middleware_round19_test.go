package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func TestPublicSurfaceMiddlewareRound19_Policy(t *testing.T) {
	mw := createPublicSurfaceMiddleware()

	t.Run("allows_public_endpoint_without_auth", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/instance")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("rejects_non_public_endpoint_without_auth", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/notifications")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("allows_non_public_endpoint_with_auth", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/notifications")
		ctx.Set("is_authenticated", true)
		ctx.Set("username", "alice")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("always_allows_options", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodOptions, "/api/v1/notifications")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusNoContent, ""), nil
		})(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, resp.Status)
	})

	t.Run("status_source_is_not_public", func(t *testing.T) {
		ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/statuses/1/source")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})
}
