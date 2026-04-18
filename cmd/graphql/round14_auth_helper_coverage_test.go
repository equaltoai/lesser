package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func TestGraphQLAuthHelperCoverage_Round14(t *testing.T) {
	t.Run("createAuthMiddlewareWithService passes through when oauth service is nil", func(t *testing.T) {
		mw := createAuthMiddlewareWithService(nil, zap.NewNop())
		called := false

		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodPost, Path: "/graphql"}}
		resp, err := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
			called = true
			require.Same(t, ctx, c)
			return &apptheory.Response{Status: http.StatusAccepted}, nil
		})(ctx)

		require.NoError(t, err)
		require.True(t, called)
		require.Equal(t, http.StatusAccepted, resp.Status)
	})

	t.Run("newGraphQLOAuthService returns nil when JWT secret is missing", func(t *testing.T) {
		originalCfg := cfg
		t.Cleanup(func() { cfg = originalCfg })

		cfg = &config.Config{}
		require.Nil(t, newGraphQLOAuthService(nil))
	})
}

func TestGraphQLRequestContextHelpers_Round14(t *testing.T) {
	t.Run("graphqlWithLoaders keeps unsupported loader payloads accessible", func(t *testing.T) {
		originalLogger := logger
		t.Cleanup(func() { logger = originalLogger })
		logger = zap.NewNop()

		ctx := &apptheory.Context{}
		ctx.Set("loaders", "unexpected")

		requestCtx := graphqlWithLoaders(context.Background(), ctx)
		require.Equal(t, "unexpected", requestCtx.Value(contextKeyLoaders))
	})

	t.Run("response omits empty header values and defaults status", func(t *testing.T) {
		w := newGraphQLResponseWriter()
		w.header["X-Empty"] = nil
		w.header["X-Test"] = []string{"1"}

		resp := w.Response()
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, []string{"1"}, resp.Headers["x-test"])
		_, ok := resp.Headers["x-empty"]
		require.False(t, ok)
	})
}
