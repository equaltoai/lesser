package auth

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestUnifiedAuthMiddleware_OptionalAuth_AuthenticatedBranch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := oauthServiceStub{
		claims: testClaims{username: "alice", scopes: map[string]bool{common.ScopeRead: true}},
	}

	req := lift.NewRequest(nil)
	req.Headers = map[string]string{"Authorization": "Bearer token"}
	ctx := lift.NewContext(context.Background(), req)

	mw := OptionalAuth(MiddlewareConfig{OAuthService: stub, Logger: logger, ServiceName: "api"})
	called := false
	handler := mw(lift.HandlerFunc(func(c *lift.Context) error {
		called = true
		require.True(t, IsAuthenticated(c))
		require.Equal(t, "alice", GetAuthenticatedUsername(c))
		require.NotNil(t, GetJWTClaims(c))
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	require.True(t, called)
}

func TestUnifiedAuthMiddleware_RequiredAuth_InvalidTokenBranch(t *testing.T) {
	stub := oauthServiceStub{err: errors.New("bad token")}

	req := lift.NewRequest(nil)
	req.Headers = map[string]string{"Authorization": "Bearer token"}
	ctx := lift.NewContext(context.Background(), req)

	mw := RequiredAuth(MiddlewareConfig{
		OAuthService:  stub,
		ServiceName:   "api",
		RequiredScope: common.ScopeRead,
	})
	handler := mw(lift.HandlerFunc(func(*lift.Context) error { return nil }))
	_ = handler.Handle(ctx)
	require.Equal(t, 401, ctx.Response.StatusCode)
}

func TestUnifiedAuthMiddleware_WrappersAndTestMode(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := oauthServiceStub{
		claims: testClaims{username: "alice", scopes: map[string]bool{common.ScopeWrite: true, common.AdminRead: true}},
	}

	req := lift.NewRequest(nil)
	req.Headers = map[string]string{"Authorization": "Bearer token"}
	ctx := lift.NewContext(context.Background(), req)

	write := WriteAuth(MiddlewareConfig{OAuthService: stub, Logger: logger, ServiceName: "api"})
	require.NoError(t, write(lift.HandlerFunc(func(*lift.Context) error { return nil })).Handle(ctx))

	req = lift.NewRequest(nil)
	req.Headers = map[string]string{"Authorization": "Bearer token"}
	ctx = lift.NewContext(context.Background(), req)
	admin := AdminAuth(MiddlewareConfig{OAuthService: stub, Logger: logger, ServiceName: "api"})
	require.NoError(t, admin(lift.HandlerFunc(func(*lift.Context) error { return nil })).Handle(ctx))

	// IsTestMode environment variable switch.
	t.Setenv("TEST_MODE", "true")
	require.True(t, IsTestMode(ctx))
	os.Unsetenv("TEST_MODE")
	require.False(t, IsTestMode(ctx))
}

func TestUnifiedAuthMiddleware_GetLegacyAuthContext_WrongType(t *testing.T) {
	req := lift.NewRequest(nil)
	ctx := lift.NewContext(context.Background(), req)
	ctx.Set("auth_context", "bad")
	require.Empty(t, GetLegacyAuthContext(ctx).Username)
}

