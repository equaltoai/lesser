package auth

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_NewMiddlewareFallbackAndHelpers(t *testing.T) {
	cfg := config.Get()
	orig := cfg.JWTSecret
	t.Cleanup(func() { cfg.JWTSecret = orig })
	cfg.JWTSecret = "a-very-strong-jwt-key-without-weak-patterns-9876543210"

	m := NewMiddleware()
	require.NotNil(t, m)
	require.NotNil(t, m.oauthService)

	ctx := context.Background()
	claims := &Claims{Username: "alice", Scopes: []string{"read"}}
	ctx = WithClaims(ctx, claims)
	got, ok := GetClaims(ctx)
	require.True(t, ok)
	assert.Equal(t, "alice", got.Username)

	assert.NoError(t, m.RequireScope(claims, "read"))
	assert.Error(t, m.RequireScope(claims, "write"))
	assert.NoError(t, m.RequireUser(claims, "alice"))
	assert.Error(t, m.RequireUser(claims, "bob"))
}

func TestMiddleware_ValidateTokenAndRequireAuth(t *testing.T) {
	secret := "a-very-strong-jwt-key-without-weak-patterns-9876543210"
	m := &Middleware{oauthService: &OAuthService{jwtSecret: []byte(secret)}}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "alice",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Username:  "alice",
		Scopes:    []string{"read"},
		ClientID:  "web",
		SessionID: "sid",
	})

	tokenString, err := token.SignedString([]byte(secret))
	require.NoError(t, err)

	// Lift-style validation uses header string directly.
	parsed, err := m.ValidateToken("Bearer " + tokenString)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	assert.Equal(t, "alice", parsed.Username)

	_, err = m.ValidateToken("")
	assert.ErrorIs(t, err, ErrMissingAuthHeader)

	_, err = m.ValidateToken("invalid")
	assert.ErrorIs(t, err, ErrInvalidToken)

	// APIGateway middleware path.
	req := events.APIGatewayV2HTTPRequest{
		Headers: map[string]string{
			"Authorization": "Bearer " + tokenString,
			"User-Agent":    "ua",
		},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				SourceIP: "192.0.2.1",
			},
		},
	}

	got, err := m.RequireAuth(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "alice", got.Username)

	_, err = m.RequireAuth(context.Background(), events.APIGatewayV2HTTPRequest{})
	assert.ErrorIs(t, err, ErrMissingAuthHeader)
}
