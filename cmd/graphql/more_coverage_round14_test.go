package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
)

func TestGraphQLContextRequestID_Branches_Round14(t *testing.T) {
	require.Equal(t, "", graphqlContextRequestID(nil))

	ctx := &apptheory.Context{RequestID: "rid"}
	require.Equal(t, "rid", graphqlContextRequestID(ctx))

	ctx.Set("requestID", 123)
	require.Equal(t, "rid", graphqlContextRequestID(ctx))

	ctx.Set("requestID", "  ")
	require.Equal(t, "rid", graphqlContextRequestID(ctx))

	ctx.RequestID = "  "
	require.Equal(t, "", graphqlContextRequestID(ctx))
}

func TestGraphQLAuthMiddleware_Behavior_Round14(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	origRepos := repos
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
		repos = origRepos
	})

	cfg = &config.Config{JWTSecret: "secret"}
	logger = zap.NewNop()
	repos = nil

	mw := createAuthMiddleware()
	nextCalled := 0
	next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
		nextCalled++
		return apptheory.Text(http.StatusOK, "ok"), nil
	}

	t.Run("nil context passes through", func(t *testing.T) {
		nextCalled = 0
		resp, err := mw(next)(nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, 1, nextCalled)
	})

	t.Run("missing token marks unauthenticated", func(t *testing.T) {
		nextCalled = 0
		ctx := &apptheory.Context{Request: apptheory.Request{Path: "/graphql", Headers: map[string][]string{}}}
		resp, err := mw(next)(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, false, ctx.Get("is_authenticated"))
		require.Equal(t, 1, nextCalled)
	})

	t.Run("invalid token marks unauthenticated", func(t *testing.T) {
		nextCalled = 0
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Path: "/graphql",
				Headers: map[string][]string{
					"authorization": {"Bearer not-a-token"},
				},
			},
		}
		resp, err := mw(next)(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, false, ctx.Get("is_authenticated"))
		require.Equal(t, 1, nextCalled)
	})

	t.Run("valid token sets claims and username", func(t *testing.T) {
		nextCalled = 0
		claims := &auth.Claims{
			Username: "alice",
			Scopes:   []string{"read"},
			RegisteredClaims: jwt.RegisteredClaims{
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
		require.NoError(t, err)

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Path: "/graphql",
				Headers: map[string][]string{
					"authorization": {"Bearer " + token},
				},
			},
		}
		resp, err := mw(next)(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, true, ctx.Get("is_authenticated"))
		require.NotNil(t, ctx.Get("claims"))
		require.Equal(t, "alice", ctx.Get("username"))
		require.NotNil(t, ctx.AuthPrincipal)
		require.Equal(t, "alice", ctx.AuthIdentity)
		require.Equal(t, 1, nextCalled)
	})
}

func TestGraphQLSimpleHelpers_Round14(t *testing.T) {
	require.Equal(t, "", graphqlHeaderValue(nil, "authorization"))
	require.Equal(t, "", graphqlPath(nil))
	base := context.Background()
	out := graphqlWithCostTracker(base, nil)
	require.True(t, out == base)
	require.Nil(t, out.Value(contextKeyCostTracker))
}
