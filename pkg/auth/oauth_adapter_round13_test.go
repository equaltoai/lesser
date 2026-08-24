package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

func TestOAuthServiceAdapter_ValidateAccessToken_WithSessionBackedClaims(t *testing.T) {
	authService := newSessionAuthServiceForPrincipalTests("session-1")
	adapter := NewOAuthServiceAdapter(authService)
	require.NotNil(t, adapter)

	claims, err := adapter.ValidateAccessToken(newSessionAccessTokenForPrincipalTests(t, "session-1"))
	require.NoError(t, err)
	require.NotNil(t, claims)
	assert.Equal(t, "alice", claims.GetUsername())
}

func TestOAuthAdapterMiddlewareWrappers(t *testing.T) {
	authService := newSessionAuthServiceForPrincipalTests("session-1")
	oauthService := &OAuthService{jwtSecret: []byte("test-secret")}
	sessionToken := newSessionAccessTokenForPrincipalTests(t, "session-1")
	oauthToken, err := oauthService.generateAccessTokenWithMetadata("alice", "client-agent", []string{"read"}, accessTokenMetadata{
		SessionID: "session-oauth",
	})
	require.NoError(t, err)

	tests := []struct {
		name   string
		token  string
		build  func() apptheory.Middleware
		expect string
	}{
		{
			name:  "api auth service wrapper",
			token: sessionToken,
			build: func() apptheory.Middleware {
				return CreateAPIAuthMiddlewareFromAuthService(authService, zap.NewNop())
			},
			expect: "alice",
		},
		{
			name:  "graphql auth service wrapper",
			token: sessionToken,
			build: func() apptheory.Middleware {
				return CreateGraphQLAuthMiddlewareFromAuthService(authService, zap.NewNop())
			},
			expect: "alice",
		},
		{
			name:  "federation auth service wrapper",
			token: sessionToken,
			build: func() apptheory.Middleware {
				return CreateFederationAuthMiddlewareFromAuthService(authService, zap.NewNop())
			},
			expect: "alice",
		},
		{
			name:  "api composite auth wrapper",
			token: oauthToken,
			build: func() apptheory.Middleware {
				return CreateAPIAuthMiddlewareFromAuthAndOAuthServices(&AuthService{
					jwtSecret:   []byte("test-secret"),
					accountRepo: compositeAuthAccountRepo{},
				}, oauthService, zap.NewNop())
			},
			expect: "alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			resp, err := tt.build()(func(ctx *apptheory.Context) (*apptheory.Response, error) {
				nextCalled = true
				assert.Equal(t, tt.expect, UsernameFromAppTheoryContext(ctx))
				return &apptheory.Response{Status: 204}, nil
			})(newTestContext("GET", "/auth", withHeaders(map[string]string{
				"Authorization": "Bearer " + tt.token,
			})))
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.True(t, nextCalled)
		})
	}
}
