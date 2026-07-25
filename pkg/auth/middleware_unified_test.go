package auth

import (
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
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

	ctx := newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))

	mw := RequiredAuth(MiddlewareConfig{
		OAuthService:  stub,
		Logger:        logger,
		ServiceName:   "api",
		RequiredScope: common.ScopeRead,
		AllowedScopes: nil,
		Required:      true,
	})

	called := false
	handler := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
		called = true
		assert.Equal(t, "alice", c.Get("username"))
		assert.NotNil(t, c.Get("claims"))
		assert.NotNil(t, c.Get("auth_context"))
		return &apptheory.Response{Status: 200}, nil
	})

	resp, err := handler(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	assert.True(t, called)
}

func TestNewUnifiedAuthMiddleware(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	stub := oauthServiceStub{}

	mw := NewUnifiedAuthMiddleware(MiddlewareConfig{
		OAuthService: stub,
		Logger:       logger,
		ServiceName:  "api",
	})

	require.NotNil(t, mw)
	require.Equal(t, "api", mw.serviceName)
	require.Equal(t, logger, mw.logger)
	require.Equal(t, stub, mw.oauthService)
}

func TestUnifiedAuthMiddleware_RequiredAuth_RespondsOnMissingHeader(t *testing.T) {
	stub := oauthServiceStub{claims: testClaims{username: "alice", scopes: map[string]bool{common.ScopeRead: true}}}
	ctx := newTestContext("GET", "/test")

	mw := RequiredAuth(MiddlewareConfig{
		OAuthService:  stub,
		ServiceName:   "api",
		RequiredScope: common.ScopeRead,
	})

	called := false
	handler := mw(func(*apptheory.Context) (*apptheory.Response, error) {
		called = true
		return &apptheory.Response{Status: 200}, nil
	})
	resp, err := handler(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, called)
	assert.Equal(t, 401, resp.Status)
}

func TestUnifiedAuthMiddleware_RequiredAuthWithMultipleScopes(t *testing.T) {
	stub := oauthServiceStub{
		claims: testClaims{username: "alice", scopes: map[string]bool{common.ScopeWrite: true}},
	}

	ctx := newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))

	mw := RequiredAuthWithMultipleScopes(MiddlewareConfig{
		OAuthService:  stub,
		ServiceName:   "api",
		AllowedScopes: []string{common.ScopeRead, common.ScopeWrite},
		RequiredScope: "",
		Required:      true,
	})

	called := false
	handler := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
		called = true
		assert.Equal(t, "alice", c.Get("username"))
		return &apptheory.Response{Status: 200}, nil
	})
	_, err := handler(ctx)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestUnifiedAuthMiddleware_OptionalAuth_PopulatesContextButAllowsAnonymous(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// No header -> unauthenticated
	ctx := newTestContext("GET", "/test")

	mw := OptionalAuth(MiddlewareConfig{
		OAuthService: oauthServiceStub{claims: nil, err: errors.New("bad token")},
		Logger:       logger,
		ServiceName:  "api",
	})

	called := false
	handler := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
		called = true
		assert.False(t, IsAuthenticated(c))
		assert.Equal(t, "", GetAuthenticatedUsername(c))
		return &apptheory.Response{Status: 200}, nil
	})
	_, err := handler(ctx)
	require.NoError(t, err)
	assert.True(t, called)

	// Header present but validation fails -> still anonymous
	ctx = newTestContext("GET", "/test", withHeaders(map[string]string{"Authorization": "Bearer token"}))
	handler = mw(func(c *apptheory.Context) (*apptheory.Response, error) {
		assert.False(t, IsAuthenticated(c))
		return &apptheory.Response{Status: 200}, nil
	})
	_, err = handler(ctx)
	require.NoError(t, err)
}

func TestUnifiedAuthMiddleware_ContextHelpersAndAccessChecks(t *testing.T) {
	ctx := &apptheory.Context{}

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

	principalOnly := &apptheory.Context{
		AuthIdentity: "agent",
		AuthPrincipal: PrincipalFromClaims(&Claims{
			Username:       "agent",
			Scopes:         []string{common.ScopeRead},
			SessionID:      "sess-1",
			IsAgent:        true,
			ClientClass:    ClientClassAgent,
			AgentSessionID: "sess-1",
		}),
	}
	assert.Equal(t, "agent", GetAuthenticatedUsername(principalOnly))
	assert.True(t, IsAuthenticated(principalOnly))
	require.NotNil(t, GetJWTClaims(principalOnly))
}
