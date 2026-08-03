package auth

import (
	"errors"
	"os"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap/zaptest"
)

func TestUnifiedAuthMiddleware_OptionalAuth_AuthenticatedBranch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := oauthServiceStub{
		claims: testClaims{username: "alice", scopes: map[string]bool{common.ScopeRead: true}},
	}

	ctx := newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))

	mw := OptionalAuth(MiddlewareConfig{OAuthService: stub, Logger: logger, ServiceName: "api"})
	called := false
	handler := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
		called = true
		require.True(t, IsAuthenticated(c))
		require.Equal(t, "alice", GetAuthenticatedUsername(c))
		require.NotNil(t, GetJWTClaims(c))
		return &apptheory.Response{Status: 200}, nil
	})

	_, err := handler(ctx)
	require.NoError(t, err)
	require.True(t, called)
}

func TestUnifiedAuthMiddleware_RequiredAuth_InvalidTokenBranch(t *testing.T) {
	stub := oauthServiceStub{err: errors.New("bad token")}

	ctx := newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))

	mw := RequiredAuth(MiddlewareConfig{
		OAuthService:  stub,
		ServiceName:   "api",
		RequiredScope: common.ScopeRead,
	})
	handler := mw(func(*apptheory.Context) (*apptheory.Response, error) { return &apptheory.Response{Status: 200}, nil })
	resp, err := handler(ctx)
	require.NoError(t, err)
	require.Equal(t, 401, resp.Status)
}

func TestUnifiedAuthMiddleware_WrappersAndTestMode(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := oauthServiceStub{
		claims: testClaims{username: "alice", scopes: map[string]bool{
			common.ScopeRead:   true,
			common.ScopeWrite:  true,
			common.AdminRead:   true,
			common.ScopeFollow: true,
		}},
	}

	ctx := newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))

	write := WriteAuth(MiddlewareConfig{OAuthService: stub, Logger: logger, ServiceName: "api"})
	_, err := write(func(*apptheory.Context) (*apptheory.Response, error) { return &apptheory.Response{Status: 200}, nil })(ctx)
	require.NoError(t, err)

	ctx = newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))
	read := ReadAuth(MiddlewareConfig{OAuthService: stub, Logger: logger, ServiceName: "api"})
	_, err = read(func(*apptheory.Context) (*apptheory.Response, error) { return &apptheory.Response{Status: 200}, nil })(ctx)
	require.NoError(t, err)

	ctx = newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))
	admin := AdminAuth(MiddlewareConfig{OAuthService: stub, Logger: logger, ServiceName: "api"})
	_, err = admin(func(*apptheory.Context) (*apptheory.Response, error) { return &apptheory.Response{Status: 200}, nil })(ctx)
	require.NoError(t, err)

	ctx = newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))
	follow := FollowAuth(MiddlewareConfig{OAuthService: stub, Logger: logger, ServiceName: "api"})
	_, err = follow(func(*apptheory.Context) (*apptheory.Response, error) { return &apptheory.Response{Status: 200}, nil })(ctx)
	require.NoError(t, err)

	// IsTestMode environment variable switch.
	t.Setenv("TEST_MODE", "true")
	require.True(t, IsTestMode(ctx))
	os.Unsetenv("TEST_MODE")
	require.False(t, IsTestMode(ctx))
}

func TestUnifiedAuthMiddleware_GetLegacyAuthContext_WrongType(t *testing.T) {
	ctx := &apptheory.Context{}
	ctx.Set("auth_context", "bad")
	require.Empty(t, GetLegacyAuthContext(ctx).Username)
}
