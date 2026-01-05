package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type testClaims struct {
	username string
	scopes   map[string]bool
}

func (c testClaims) HasScope(scope string) bool { return c.scopes[scope] }
func (c testClaims) GetUsername() string        { return c.username }

type oauthServiceStub struct {
	claims common.Claims
	err    error
}

func (s oauthServiceStub) ValidateAccessToken(_ string) (common.Claims, error) {
	return s.claims, s.err
}

func TestUnifiedAuthMiddleware_RequiredAuth_SetsContextAndCallsNext(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := oauthServiceStub{
		claims: testClaims{username: "alice", scopes: map[string]bool{common.ScopeRead: true}},
	}

	req := lift.NewRequest(nil)
	req.Headers = map[string]string{"Authorization": "Bearer token"}
	ctx := lift.NewContext(context.Background(), req)

	mw := RequiredAuth(MiddlewareConfig{
		OAuthService:  stub,
		Logger:        logger,
		ServiceName:   "api",
		RequiredScope: common.ScopeRead,
		AllowedScopes: nil,
		Required:      true,
	})

	called := false
	handler := mw(lift.HandlerFunc(func(c *lift.Context) error {
		called = true
		assert.Equal(t, "alice", c.Get("username"))
		assert.NotNil(t, c.Get("claims"))
		assert.NotNil(t, c.Get("auth_context"))
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	assert.True(t, called)
}

func TestUnifiedAuthMiddleware_RequiredAuth_RespondsOnMissingHeader(t *testing.T) {
	stub := oauthServiceStub{claims: testClaims{username: "alice", scopes: map[string]bool{common.ScopeRead: true}}}
	req := lift.NewRequest(nil)
	ctx := lift.NewContext(context.Background(), req)

	mw := RequiredAuth(MiddlewareConfig{
		OAuthService:  stub,
		ServiceName:   "api",
		RequiredScope: common.ScopeRead,
	})

	handler := mw(lift.HandlerFunc(func(*lift.Context) error { return nil }))
	_ = handler.Handle(ctx)
	assert.Equal(t, 401, ctx.Response.StatusCode)
}

func TestUnifiedAuthMiddleware_RequiredAuthWithMultipleScopes(t *testing.T) {
	stub := oauthServiceStub{
		claims: testClaims{username: "alice", scopes: map[string]bool{common.ScopeWrite: true}},
	}

	req := lift.NewRequest(nil)
	req.Headers = map[string]string{"Authorization": "Bearer token"}
	ctx := lift.NewContext(context.Background(), req)

	mw := RequiredAuthWithMultipleScopes(MiddlewareConfig{
		OAuthService:  stub,
		ServiceName:   "api",
		AllowedScopes: []string{common.ScopeRead, common.ScopeWrite},
		RequiredScope: "",
		Required:      true,
	})

	called := false
	handler := mw(lift.HandlerFunc(func(c *lift.Context) error {
		called = true
		assert.Equal(t, "alice", c.Get("username"))
		return nil
	}))
	require.NoError(t, handler.Handle(ctx))
	assert.True(t, called)
}

func TestUnifiedAuthMiddleware_OptionalAuth_PopulatesContextButAllowsAnonymous(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// No header -> unauthenticated
	req := lift.NewRequest(nil)
	ctx := lift.NewContext(context.Background(), req)

	mw := OptionalAuth(MiddlewareConfig{
		OAuthService: oauthServiceStub{claims: nil, err: errors.New("bad token")},
		Logger:       logger,
		ServiceName:  "api",
	})

	called := false
	handler := mw(lift.HandlerFunc(func(c *lift.Context) error {
		called = true
		assert.False(t, IsAuthenticated(c))
		assert.Equal(t, "", GetAuthenticatedUsername(c))
		return nil
	}))
	require.NoError(t, handler.Handle(ctx))
	assert.True(t, called)

	// Header present but validation fails -> still anonymous
	req = lift.NewRequest(nil)
	req.Headers = map[string]string{"Authorization": "Bearer token"}
	ctx = lift.NewContext(context.Background(), req)
	handler = mw(lift.HandlerFunc(func(c *lift.Context) error {
		assert.False(t, IsAuthenticated(c))
		return nil
	}))
	require.NoError(t, handler.Handle(ctx))
}

func TestUnifiedAuthMiddleware_ContextHelpersAndAccessChecks(t *testing.T) {
	req := lift.NewRequest(nil)
	ctx := lift.NewContext(context.Background(), req)

	assert.Equal(t, "", GetAuthenticatedUsername(ctx))
	assert.Nil(t, GetJWTClaims(ctx))
	assert.False(t, IsAuthenticated(ctx))

	// Provide a minimal auth context and claims.
	authCtx := &common.AuthContext{
		Username: "alice",
		Claims:   testClaims{username: "alice", scopes: map[string]bool{common.ScopeRead: true}},
	}
	ctx.Set("auth_context", authCtx)
	ctx.Set("username", authCtx.Username)
	ctx.Set("claims", authCtx.Claims)

	assert.Equal(t, authCtx, GetLegacyAuthContext(ctx))
	assert.Equal(t, "alice", GetAuthenticatedUsername(ctx))
	assert.NotNil(t, GetJWTClaims(ctx))
	assert.True(t, IsAuthenticated(ctx))

	require.Error(t, RequireWriteAccess(ctx))
	require.NoError(t, RequireReadAccess(ctx))
}
