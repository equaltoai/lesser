package auth

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newSessionAuthServiceForPrincipalTests(sessionID string) *AuthService {
	return &AuthService{
		jwtSecret: []byte("test-secret"),
		accountRepo: compositeAuthAccountRepo{
			session: &storage.Session{
				SessionID: sessionID,
				ExpiresAt: time.Now().Add(time.Hour),
			},
		},
	}
}

func newSessionAccessTokenForPrincipalTests(t *testing.T, sessionID string) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "alice",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Username:  "alice",
		Scopes:    []string{"read"},
		SessionID: sessionID,
	}).SignedString([]byte("test-secret"))
	require.NoError(t, err)

	return token
}

func TestAppTheoryPrincipalResolver_EdgeCases(t *testing.T) {
	t.Run("nil resolver marks attempted and returns nil", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/statuses")

		var resolver *appTheoryPrincipalResolver
		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		assert.Nil(t, principal)
		assert.True(t, principalResolutionAttempted(ctx))
	})

	t.Run("nil context returns nil without panic", func(t *testing.T) {
		resolver := newAppTheoryPrincipalResolver(func(string) (*Claims, error) {
			t.Fatal("validator should not run for nil context")
			return nil, nil
		}, zap.NewNop(), "api")

		principal, err := resolver.Resolve(nil)
		require.NoError(t, err)
		assert.Nil(t, principal)
	})

	t.Run("nil validator returns nil principal", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/statuses", withHeaders(map[string]string{
			"Authorization": "Bearer token",
		}))

		resolver := &appTheoryPrincipalResolver{}
		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		assert.Nil(t, principal)
		assert.True(t, principalResolutionAttempted(ctx))
	})

	t.Run("nil claims still logs validation failure", func(t *testing.T) {
		core, observed := observer.New(zap.WarnLevel)
		resolver := newAppTheoryPrincipalResolver(func(token string) (*Claims, error) {
			assert.Equal(t, "token", token)
			return nil, nil
		}, zap.New(core), "api")

		ctx := newTestContext("GET", "/api/v1/statuses", withHeaders(map[string]string{
			"Authorization": "Bearer token",
		}))

		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		assert.Nil(t, principal)

		entries := observed.FilterMessage("principal authentication failed - header present but validation failed").AllUntimed()
		require.Len(t, entries, 1)
		assert.Equal(t, "api", entries[0].ContextMap()["service"])
	})
}

func TestAppTheoryPrincipalContextBridge_FallbacksAndSkips(t *testing.T) {
	t.Run("nil context passes through unchanged", func(t *testing.T) {
		nextCalled := false
		resp, err := createPrincipalContextBridge(nil)(func(ctx *apptheory.Context) (*apptheory.Response, error) {
			nextCalled = true
			assert.Nil(t, ctx)
			return &apptheory.Response{Status: 204}, nil
		})(nil)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, nextCalled)
	})

	t.Run("attempted context does not re-resolve", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/public", withHeaders(map[string]string{
			"Authorization": "Bearer token",
		}))
		markPrincipalResolutionAttempted(ctx)

		nextCalled := false
		middleware := createPrincipalContextBridge(newAppTheoryPrincipalResolver(func(string) (*Claims, error) {
			t.Fatal("resolver should not run when principal resolution was already attempted")
			return nil, nil
		}, zap.NewNop(), "api"))

		resp, err := middleware(func(ctx *apptheory.Context) (*apptheory.Response, error) {
			nextCalled = true
			assert.False(t, IsAppTheoryContextAuthenticated(ctx))
			assert.Equal(t, false, ctx.Get("is_authenticated"))
			return &apptheory.Response{Status: 204}, nil
		})(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, nextCalled)
	})

	t.Run("principal identity seeds legacy auth context when username claims are absent", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/public")
		ctx.AuthPrincipal = &apptheory.AuthPrincipal{
			Identity: "  principal-user  ",
			Claims:   map[string]any{},
		}

		nextCalled := false
		resp, err := createPrincipalContextBridge(nil)(func(ctx *apptheory.Context) (*apptheory.Response, error) {
			nextCalled = true
			assert.Equal(t, "principal-user", ctx.AuthIdentity)
			assert.Equal(t, "principal-user", ctx.Get("username"))
			assert.Equal(t, true, ctx.Get("is_authenticated"))
			return &apptheory.Response{Status: 204}, nil
		})(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, nextCalled)
	})
}

func TestAppTheoryPrincipalHelperFallbackBranches(t *testing.T) {
	assert.Nil(t, PrincipalFromClaims(nil))
	assert.False(t, principalResolutionAttempted(nil))
	assert.Equal(t, principalTokenFamilyUnknown, inferPrincipalTokenFamily("   "))

	t.Run("username falls back to legacy username field when no claims exist", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/accounts")
		ctx.Set("username", "  legacy-user  ")
		assert.Nil(t, ClaimsFromAppTheoryContext(ctx))
		assert.Equal(t, "legacy-user", UsernameFromAppTheoryContext(ctx))
	})

	t.Run("claims are synthesized from principal username claim when raw claims are absent", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/accounts")
		ctx.AuthPrincipal = &apptheory.AuthPrincipal{
			Identity: "fallback-id",
			Scopes:   []string{"read"},
			Claims: map[string]any{
				"username": "  principal-user  ",
				"subject":  "subject-2",
			},
		}

		claims := ClaimsFromAppTheoryContext(ctx)
		require.NotNil(t, claims)
		assert.Equal(t, "principal-user", claims.GetUsername())
		assert.Equal(t, "subject-2", claims.Subject)
		assert.Equal(t, "principal-user", UsernameFromAppTheoryContext(ctx))
	})
}

func TestCompositePrincipalClaimsValidator_ServiceAvailabilityBranches(t *testing.T) {
	authService := newSessionAuthServiceForPrincipalTests("session-1")
	oauthService := &OAuthService{jwtSecret: []byte("test-secret")}

	require.Nil(t, newCompositePrincipalClaimsValidator(nil, nil))

	sessionToken := newSessionAccessTokenForPrincipalTests(t, "session-1")
	oauthToken, err := oauthService.generateAccessTokenWithMetadata("alice", "client-agent", []string{"read"}, accessTokenMetadata{
		SessionID: "session-oauth",
	})
	require.NoError(t, err)

	oauthOnly := newCompositePrincipalClaimsValidator(nil, oauthService)
	require.NotNil(t, oauthOnly)
	_, err = oauthOnly(sessionToken)
	require.ErrorIs(t, err, ErrInvalidToken)
	oauthClaims, err := oauthOnly(oauthToken)
	require.NoError(t, err)
	require.NotNil(t, oauthClaims)

	authOnly := newCompositePrincipalClaimsValidator(authService, nil)
	require.NotNil(t, authOnly)
	sessionClaims, err := authOnly(sessionToken)
	require.NoError(t, err)
	require.NotNil(t, sessionClaims)
	_, err = authOnly(oauthToken)
	require.ErrorIs(t, err, ErrInvalidToken)
	_, err = authOnly("not-a-jwt")
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestAppTheoryPrincipalHooksAndBridges_WithRealServices(t *testing.T) {
	authService := newSessionAuthServiceForPrincipalTests("session-1")
	oauthService := &OAuthService{jwtSecret: []byte("test-secret")}

	sessionToken := newSessionAccessTokenForPrincipalTests(t, "session-1")
	oauthToken, err := oauthService.generateAccessTokenWithMetadata("alice", "client-agent", []string{"read"}, accessTokenMetadata{
		SessionID: "session-oauth",
	})
	require.NoError(t, err)

	t.Run("auth service hook validates native session token", func(t *testing.T) {
		hook := NewAppTheoryPrincipalHookFromAuthService(authService, zap.NewNop(), "api")
		require.NotNil(t, hook)

		principal, err := hook(newTestContext("GET", "/api/v1/me", withHeaders(map[string]string{
			"Authorization": "Bearer " + sessionToken,
		})))
		require.NoError(t, err)
		require.NotNil(t, principal)
		assert.Equal(t, "alice", principal.Identity)
	})

	t.Run("auth service bridge hydrates legacy context", func(t *testing.T) {
		nextCalled := false
		middleware := CreatePrincipalContextBridgeFromAuthService(authService, zap.NewNop(), "api")

		resp, err := middleware(func(ctx *apptheory.Context) (*apptheory.Response, error) {
			nextCalled = true
			assert.Equal(t, "alice", UsernameFromAppTheoryContext(ctx))
			assert.Equal(t, true, ctx.Get("is_authenticated"))
			return &apptheory.Response{Status: 204}, nil
		})(newTestContext("GET", "/api/v1/me", withHeaders(map[string]string{
			"Authorization": "Bearer " + sessionToken,
		})))
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, nextCalled)
	})

	t.Run("composite hook accepts oauth token without session row", func(t *testing.T) {
		hook := NewAppTheoryPrincipalHookFromAuthAndOAuthServices(&AuthService{
			jwtSecret:   []byte("test-secret"),
			accountRepo: compositeAuthAccountRepo{},
		}, oauthService, zap.NewNop(), "api")
		require.NotNil(t, hook)

		principal, err := hook(newTestContext("GET", "/api/v1/me", withHeaders(map[string]string{
			"Authorization": "Bearer " + oauthToken,
		})))
		require.NoError(t, err)
		require.NotNil(t, principal)
		assert.Equal(t, "alice", principal.Identity)
	})

	t.Run("composite bridge accepts oauth token without session row", func(t *testing.T) {
		nextCalled := false
		middleware := CreatePrincipalContextBridgeFromAuthAndOAuthServices(&AuthService{
			jwtSecret:   []byte("test-secret"),
			accountRepo: compositeAuthAccountRepo{},
		}, oauthService, zap.NewNop(), "api")

		resp, err := middleware(func(ctx *apptheory.Context) (*apptheory.Response, error) {
			nextCalled = true
			assert.Equal(t, "alice", UsernameFromAppTheoryContext(ctx))
			assert.Equal(t, true, ctx.Get("is_authenticated"))
			return &apptheory.Response{Status: 204}, nil
		})(newTestContext("GET", "/api/v1/me", withHeaders(map[string]string{
			"Authorization": "Bearer " + oauthToken,
		})))
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, nextCalled)
	})
}
