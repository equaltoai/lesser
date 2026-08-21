package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type compositeAuthAccountRepo struct {
	session *storage.Session
}

func (r compositeAuthAccountRepo) GetUser(_ context.Context, _ string) (*storage.User, error) {
	return nil, errors.New("not implemented")
}

func (r compositeAuthAccountRepo) UpdateUser(_ context.Context, _ string, _ map[string]any) error {
	return errors.New("not implemented")
}

func (r compositeAuthAccountRepo) GetActor(_ context.Context, _ string) (*activitypub.Actor, error) {
	return nil, errors.New("not implemented")
}

func (r compositeAuthAccountRepo) GetDevice(_ context.Context, _ string) (*storage.Device, error) {
	return nil, errors.New("not implemented")
}

func (r compositeAuthAccountRepo) GetSession(_ context.Context, _ string) (*storage.Session, error) {
	if r.session == nil {
		return nil, errors.New("missing session")
	}
	return r.session, nil
}

func (r compositeAuthAccountRepo) GetUserWalletCredentials(_ context.Context, _ string) ([]*storage.WalletCredential, error) {
	return nil, errors.New("not implemented")
}

func (r compositeAuthAccountRepo) MarkWalletChallengeSpent(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

func (r compositeAuthAccountRepo) ResetWalletChallengeSpent(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

func (r compositeAuthAccountRepo) GetWalletChallenge(_ context.Context, _ string) (*storage.WalletChallenge, error) {
	return nil, errors.New("not implemented")
}

func (r compositeAuthAccountRepo) StoreRecoveryToken(_ context.Context, _ string, _ map[string]any) error {
	return errors.New("not implemented")
}

func testAppTheoryClaims() *Claims {
	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "subject-1",
		},
		Username:               "Alice",
		Scopes:                 []string{"read", "write"},
		ClientID:               "web",
		ClientClass:            "cli",
		SessionID:              "session-1",
		AgentSessionID:         "agent-session-1",
		IsAgent:                true,
		AgentType:              "assistant",
		DelegatedBy:            "@owner",
		DelegationPrincipal:    "owner",
		DelegationAgent:        "Alice",
		DelegationContentClass: DelegationContentClassNote,
	}
}

func TestAppTheoryPrincipalResolverResolve(t *testing.T) {
	t.Run("marks attempted and skips when authorization is missing", func(t *testing.T) {
		resolver := newAppTheoryPrincipalResolver(func(string) (*Claims, error) {
			t.Fatal("validator should not be called without an authorization header")
			return nil, nil
		}, zap.NewNop(), "api")

		ctx := newTestContext("GET", "/api/v1/statuses")
		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		assert.Nil(t, principal)
		assert.True(t, principalResolutionAttempted(ctx))
	})

	t.Run("logs invalid bearer format as debug", func(t *testing.T) {
		core, observed := observer.New(zap.DebugLevel)
		resolver := newAppTheoryPrincipalResolver(func(string) (*Claims, error) {
			t.Fatal("validator should not be called for an invalid bearer header")
			return nil, nil
		}, zap.New(core), "api")

		ctx := newTestContext("GET", "/api/v1/statuses", withHeaders(map[string]string{
			"Authorization": "Token nope",
		}))

		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		assert.Nil(t, principal)
		assert.True(t, principalResolutionAttempted(ctx))

		entries := observed.FilterMessage("principal authentication skipped - invalid bearer token format").AllUntimed()
		require.Len(t, entries, 1)
		assert.Equal(t, "api", entries[0].ContextMap()["service"])
		assert.Equal(t, "/api/v1/statuses", entries[0].ContextMap()["path"])
		assert.Equal(t, true, entries[0].ContextMap()["has_auth_header"])
	})

	t.Run("logs validation failure as warning", func(t *testing.T) {
		core, observed := observer.New(zap.WarnLevel)
		resolver := newAppTheoryPrincipalResolver(func(token string) (*Claims, error) {
			assert.Equal(t, "bad-token", token)
			return nil, errors.New("boom")
		}, zap.New(core), "graphql")

		ctx := newTestContext("GET", "/graphql", withHeaders(map[string]string{
			"Authorization": "Bearer bad-token",
		}))

		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		assert.Nil(t, principal)
		assert.True(t, principalResolutionAttempted(ctx))

		entries := observed.FilterMessage("principal authentication failed - header present but validation failed").AllUntimed()
		require.Len(t, entries, 1)
		assert.Equal(t, "graphql", entries[0].ContextMap()["service"])
		assert.Equal(t, "/graphql", entries[0].ContextMap()["path"])
		assert.Equal(t, true, entries[0].ContextMap()["has_auth_header"])
	})

	t.Run("returns principal from validated claims", func(t *testing.T) {
		claims := testAppTheoryClaims()
		resolver := newAppTheoryPrincipalResolver(func(token string) (*Claims, error) {
			assert.Equal(t, "good-token", token)
			return claims, nil
		}, zap.NewNop(), "api")

		ctx := newTestContext("GET", "/api/v1/me", withHeaders(map[string]string{
			"Authorization": "Bearer good-token",
		}))

		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		require.NotNil(t, principal)
		assert.Equal(t, "alice", principal.Identity)
		assert.Equal(t, []string{"read", "write"}, principal.Scopes)
		assert.Equal(t, claims, principal.Claims[principalClaimsRawKey])
		assert.Equal(t, "alice", principal.Claims["username"])
		assert.Equal(t, "web", principal.Claims["client_id"])
		assert.Equal(t, "cli", principal.Claims["client_class"])
		assert.Equal(t, "session-1", principal.Claims["session_id"])
		assert.Equal(t, "agent-session-1", principal.Claims["agent_session_id"])
		assert.Equal(t, true, principal.Claims["is_agent"])
		assert.Equal(t, "assistant", principal.Claims["agent_type"])
		assert.Equal(t, "@owner", principal.Claims["delegated_by"])
		assert.Equal(t, "subject-1", principal.Claims["subject"])
	})
}

func TestAppTheorySecurePrincipalResolver_OptionalInvalidTokenDowngradesToAnonymousForPreMigrationParity(t *testing.T) {
	resolver := newAppTheoryPrincipalResolver(func(token string) (*Claims, error) {
		assert.Equal(t, "expired-token", token)
		return nil, ErrInvalidToken
	}, zap.NewNop(), "api")
	ctx := newTestContext("GET", "/api/v1/statuses/1", withHeaders(map[string]string{
		"Authorization": "Bearer expired-token",
	}))

	principal, err := resolver.ResolveSecure(ctx)
	require.NoError(t, err)
	assert.Nil(t, principal, "Optional routes deliberately preserve the pre-SecureApp anonymous downgrade")
}

func TestAppTheoryPrincipalBridgeHydratesContext(t *testing.T) {
	t.Run("reuses legacy claims to seed principal and auth context", func(t *testing.T) {
		claims := testAppTheoryClaims()
		ctx := newTestContext("GET", "/api/v1/accounts")
		ctx.Set("claims", claims)

		nextCalled := false
		middleware := createPrincipalContextBridge(nil)
		resp, err := middleware(func(ctx *apptheory.Context) (*apptheory.Response, error) {
			nextCalled = true
			require.NotNil(t, ctx.AuthPrincipal)
			assert.Equal(t, "alice", ctx.AuthIdentity)
			assert.Same(t, claims, ctx.Get("claims"))
			assert.Equal(t, "alice", ctx.Get("username"))
			assert.Equal(t, true, ctx.Get("is_authenticated"))

			authContext, ok := ctx.Get("auth_context").(*common.AuthContext)
			require.True(t, ok)
			assert.Equal(t, "alice", authContext.Username)
			assert.Same(t, claims, authContext.Claims)

			return &apptheory.Response{Status: 204}, nil
		})(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, nextCalled)
	})

	t.Run("resolves from oauth service and populates legacy keys", func(t *testing.T) {
		oauthService := &OAuthService{jwtSecret: []byte("test-secret")}
		token, err := oauthService.generateAccessTokenWithMetadata("Alice", "web", []string{"read"}, accessTokenMetadata{
			ClientClass: "cli",
			SessionID:   "session-2",
		})
		require.NoError(t, err)

		ctx := newTestContext("GET", "/api/v1/timelines/public", withHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}))

		nextCalled := false
		middleware := CreatePrincipalContextBridgeFromOAuthService(oauthService, zap.NewNop(), "api")
		resp, err := middleware(func(ctx *apptheory.Context) (*apptheory.Response, error) {
			nextCalled = true
			require.NotNil(t, ctx.AuthPrincipal)
			assert.Equal(t, "alice", ctx.AuthIdentity)
			assert.Equal(t, "alice", UsernameFromAppTheoryContext(ctx))

			claims := ClaimsFromAppTheoryContext(ctx)
			require.NotNil(t, claims)
			assert.Equal(t, "alice", claims.GetUsername())
			assert.Equal(t, "web", claims.ClientID)
			assert.Equal(t, "cli", claims.ClientClass)
			assert.Equal(t, "session-2", claims.SessionID)

			authContext, ok := ctx.Get("auth_context").(*common.AuthContext)
			require.True(t, ok)
			assert.Equal(t, "alice", authContext.Username)
			require.NotNil(t, authContext.Claims)
			authClaims, ok := authContext.Claims.(*Claims)
			require.True(t, ok)
			assert.Equal(t, "web", authClaims.ClientID)
			assert.Equal(t, true, ctx.Get("is_authenticated"))

			return &apptheory.Response{Status: 200}, nil
		})(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, nextCalled)
		assert.True(t, principalResolutionAttempted(ctx))
	})

	t.Run("prefers an existing principal without re-resolving", func(t *testing.T) {
		claims := testAppTheoryClaims()
		ctx := newTestContext("GET", "/api/v1/notifications")
		ctx.AuthPrincipal = PrincipalFromClaims(claims)

		nextCalled := false
		middleware := createPrincipalContextBridge(newAppTheoryPrincipalResolver(func(string) (*Claims, error) {
			t.Fatal("resolver should not be called when a principal is already present")
			return nil, nil
		}, zap.NewNop(), "api"))

		resp, err := middleware(func(ctx *apptheory.Context) (*apptheory.Response, error) {
			nextCalled = true
			assert.Equal(t, "alice", ctx.AuthIdentity)
			assert.Equal(t, "alice", ctx.Get("username"))
			assert.Equal(t, true, ctx.Get("is_authenticated"))
			return &apptheory.Response{Status: 204}, nil
		})(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, nextCalled)
	})
}

func TestAppTheoryPrincipalContextHelpers(t *testing.T) {
	t.Run("claims can be synthesized from principal fields", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/search")
		ctx.AuthPrincipal = &apptheory.AuthPrincipal{
			Identity: "principal-id",
			Scopes:   []string{"read", "write"},
			Claims: map[string]any{
				"username":         "principal-user",
				"client_id":        "client-1",
				"client_class":     "web",
				"session_id":       "session-3",
				"agent_session_id": "agent-session-3",
				"is_agent":         true,
				"agent_type":       "summarizer",
				"delegated_by":     "@operator",
				"subject":          "subject-3",
			},
		}

		claims := ClaimsFromAppTheoryContext(ctx)
		require.NotNil(t, claims)
		assert.Equal(t, "principal-user", claims.Username)
		assert.Equal(t, []string{"read", "write"}, claims.Scopes)
		assert.Equal(t, "client-1", claims.ClientID)
		assert.Equal(t, "web", claims.ClientClass)
		assert.Equal(t, "session-3", claims.SessionID)
		assert.Equal(t, "agent-session-3", claims.AgentSessionID)
		assert.True(t, claims.IsAgent)
		assert.Equal(t, "summarizer", claims.AgentType)
		assert.Equal(t, "@operator", claims.DelegatedBy)
		assert.Equal(t, "subject-3", claims.Subject)

		assert.Equal(t, "principal-user", UsernameFromAppTheoryContext(ctx))
		assert.True(t, IsAppTheoryContextAuthenticated(ctx))

		authContext := LegacyAuthContextFromAppTheoryContext(ctx)
		require.NotNil(t, authContext)
		assert.Equal(t, "principal-user", authContext.Username)
		require.NotNil(t, authContext.Claims)
		authClaims, ok := authContext.Claims.(*Claims)
		require.True(t, ok)
		assert.Equal(t, "client-1", authClaims.ClientID)
	})

	t.Run("raw claims and legacy fallbacks are preferred when present", func(t *testing.T) {
		rawClaims := testAppTheoryClaims()
		ctx := newTestContext("GET", "/api/v1/accounts")
		ctx.AuthPrincipal = &apptheory.AuthPrincipal{
			Identity: "principal-id",
			Claims: map[string]any{
				principalClaimsRawKey: rawClaims,
			},
		}

		assert.Same(t, rawClaims, ClaimsFromAppTheoryContext(ctx))
		assert.Equal(t, "alice", UsernameFromAppTheoryContext(ctx))

		legacyOnly := newTestContext("GET", "/api/v1/accounts")
		legacyOnly.Set("claims", rawClaims)
		legacyOnly.Set("username", " LegacyUser ")
		legacyOnly.Set("is_authenticated", true)

		assert.Same(t, rawClaims, ClaimsFromAppTheoryContext(legacyOnly))
		assert.Equal(t, "alice", UsernameFromAppTheoryContext(legacyOnly))
		assert.True(t, IsAppTheoryContextAuthenticated(legacyOnly))
	})

	t.Run("empty context remains unauthenticated", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/public")
		assert.Nil(t, ClaimsFromAppTheoryContext(nil))
		assert.Equal(t, "", UsernameFromAppTheoryContext(nil))
		assert.False(t, IsAppTheoryContextAuthenticated(nil))

		authContext := LegacyAuthContextFromAppTheoryContext(ctx)
		require.NotNil(t, authContext)
		assert.Empty(t, authContext.Username)
		assert.Nil(t, authContext.Claims)
		assert.False(t, IsAppTheoryContextAuthenticated(ctx))
	})
}

func TestAppTheoryPrincipalUtilityFunctions(t *testing.T) {
	assert.Nil(t, NewAppTheoryPrincipalHookFromAuthService(nil, zap.NewNop(), "api"))
	assert.Nil(t, NewAppTheoryPrincipalHookFromAuthAndOAuthServices(nil, nil, zap.NewNop(), "api"))
	assert.Nil(t, NewAppTheorySecurePrincipalResolverFromAuthService(nil, zap.NewNop(), "api"))
	assert.Nil(t, NewAppTheorySecurePrincipalResolverFromOAuthService(nil, zap.NewNop(), "api"))
	assert.Nil(t, NewAppTheorySecurePrincipalResolverFromAuthAndOAuthServices(nil, nil, zap.NewNop(), "api"))

	noOpAuthBridge := CreatePrincipalContextBridgeFromAuthService(nil, zap.NewNop(), "api")
	nextCalled := false
	_, err := noOpAuthBridge(func(ctx *apptheory.Context) (*apptheory.Response, error) {
		nextCalled = true
		return &apptheory.Response{Status: 204}, nil
	})(newTestContext("GET", "/api/v1/public"))
	require.NoError(t, err)
	assert.True(t, nextCalled)

	assert.Nil(t, NewAppTheoryPrincipalHookFromOAuthService(nil, zap.NewNop(), "api"))
	noOpCompositeBridge := CreatePrincipalContextBridgeFromAuthAndOAuthServices(nil, nil, zap.NewNop(), "api")
	nextCalled = false
	_, err = noOpCompositeBridge(func(ctx *apptheory.Context) (*apptheory.Response, error) {
		nextCalled = true
		return &apptheory.Response{Status: 204}, nil
	})(newTestContext("GET", "/api/v1/public"))
	require.NoError(t, err)
	assert.True(t, nextCalled)

	noOpOAuthBridge := CreatePrincipalContextBridgeFromOAuthService(nil, zap.NewNop(), "api")
	nextCalled = false
	_, err = noOpOAuthBridge(func(ctx *apptheory.Context) (*apptheory.Response, error) {
		nextCalled = true
		return &apptheory.Response{Status: 204}, nil
	})(newTestContext("GET", "/api/v1/public"))
	require.NoError(t, err)
	assert.True(t, nextCalled)

	claims := testAppTheoryClaims()
	assert.Equal(t, "alice", principalIdentityFromClaims(claims))
	assert.Equal(t, "subject-only", principalIdentityFromClaims(&Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "subject-only"},
	}))
	assert.Equal(t, "client-only", principalIdentityFromClaims(&Claims{ClientID: "client-only"}))
	assert.Equal(t, "", principalIdentityFromClaims(nil))

	assert.Same(t, claims, claimsFromLegacyValue(claims))
	assert.Nil(t, claimsFromLegacyValue("not-claims"))

	assert.Equal(t, "named-principal", identityFromPrincipal(&apptheory.AuthPrincipal{
		Identity: "fallback-id",
		Claims:   map[string]any{"username": " named-principal "},
	}))
	assert.Equal(t, "fallback-id", identityFromPrincipal(&apptheory.AuthPrincipal{Identity: " fallback-id "}))
	assert.Equal(t, "", identityFromPrincipal(nil))

	ctx := newTestContext("GET", "/api/v1/utility")
	setPrincipalContext(ctx, &apptheory.AuthPrincipal{Identity: " trimmed-user "})
	require.NotNil(t, ctx.AuthPrincipal)
	assert.Equal(t, "trimmed-user", ctx.AuthIdentity)
	assert.False(t, principalResolutionAttempted(ctx))
	markPrincipalResolutionAttempted(ctx)
	assert.True(t, principalResolutionAttempted(ctx))

	setPrincipalContext(nil, &apptheory.AuthPrincipal{Identity: "ignored"})
	setPrincipalContext(ctx, nil)
	markPrincipalResolutionAttempted(nil)

	oauthService := &OAuthService{jwtSecret: []byte("test-secret")}
	token, err := oauthService.generateAccessTokenWithMetadata("Alice", "web", []string{"read"}, accessTokenMetadata{})
	require.NoError(t, err)

	hook := NewAppTheoryPrincipalHookFromOAuthService(oauthService, zap.NewNop(), "api")
	require.NotNil(t, hook)

	principal, err := hook(newTestContext("GET", "/api/v1/me", withHeaders(map[string]string{
		"Authorization": "Bearer " + token,
	})))
	require.NoError(t, err)
	require.NotNil(t, principal)
	assert.Equal(t, "alice", principal.Identity)

	secureResolver := NewAppTheorySecurePrincipalResolverFromOAuthService(oauthService, zap.NewNop(), "api")
	require.NotNil(t, secureResolver)
	securePrincipal, err := secureResolver(newTestContext("GET", "/api/v1/me", withHeaders(map[string]string{
		"Authorization": "Bearer " + token,
	})))
	require.NoError(t, err)
	require.NotNil(t, securePrincipal)
	assert.Equal(t, "alice", securePrincipal.Identity)
	assert.Equal(t, []string{"read"}, securePrincipal.Scopes)
	assert.Equal(t, apptheory.PrincipalExternal, securePrincipal.Kind)

	sessionToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "alice"},
		Username:         "alice",
		Scopes:           []string{"read"},
		SessionID:        "session-1",
	}).SignedString([]byte("test-secret"))
	require.NoError(t, err)
	assert.Equal(t, principalTokenFamilySession, inferPrincipalTokenFamily(sessionToken))
	assert.Equal(t, principalTokenFamilyUnknown, inferPrincipalTokenFamily("not-a-jwt"))

	oauthToken, err := oauthService.generateAccessTokenWithMetadata("Alice", "web", []string{"read"}, accessTokenMetadata{
		SessionID: "session-1",
	})
	require.NoError(t, err)
	assert.Equal(t, principalTokenFamilyOAuth, inferPrincipalTokenFamily(oauthToken))
}

func TestCompositePrincipalClaimsValidatorRoutesByTokenFamily(t *testing.T) {
	authService := &AuthService{
		jwtSecret: []byte("test-secret"),
		accountRepo: compositeAuthAccountRepo{
			session: &storage.Session{
				SessionID: "session-1",
				ExpiresAt: time.Now().Add(time.Hour),
			},
		},
	}
	oauthService := &OAuthService{jwtSecret: []byte("test-secret")}

	validator := newCompositePrincipalClaimsValidator(authService, oauthService)
	require.NotNil(t, validator)

	sessionToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "alice",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Username:  "alice",
		Scopes:    []string{"read"},
		SessionID: "session-1",
	}).SignedString([]byte("test-secret"))
	require.NoError(t, err)

	sessionClaims, err := validator(sessionToken)
	require.NoError(t, err)
	require.NotNil(t, sessionClaims)
	assert.Equal(t, "alice", sessionClaims.GetUsername())
	assert.Equal(t, "session-1", sessionClaims.SessionID)

	oauthToken, err := oauthService.generateAccessTokenWithMetadata("alice", "client-agent", []string{"read"}, accessTokenMetadata{
		SessionID: "session-oauth",
	})
	require.NoError(t, err)

	oauthClaims, err := validator(oauthToken)
	require.NoError(t, err)
	require.NotNil(t, oauthClaims)
	assert.Equal(t, "alice", oauthClaims.GetUsername())
	assert.Equal(t, "session-oauth", oauthClaims.SessionID)
	assert.NotEmpty(t, oauthClaims.ID)

	authService.accountRepo = compositeAuthAccountRepo{}
	_, err = validator(sessionToken)
	require.ErrorIs(t, err, ErrInvalidToken)
}
