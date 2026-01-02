package auth

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_SingletonBehaviors(t *testing.T) {
	orig := globalMiddleware
	t.Cleanup(func() { globalMiddleware = orig })

	singleton := &Middleware{oauthService: &OAuthService{jwtSecret: []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210")}}
	globalMiddleware = singleton

	got, err := GetMiddleware()
	require.NoError(t, err)
	require.Same(t, singleton, got)

	got2 := NewMiddleware()
	require.Same(t, singleton, got2)
}

func TestMiddleware_RequireAuth_LowercaseHeader_AndInvalidTokenPaths(t *testing.T) {
	secret := "a-very-strong-jwt-key-without-weak-patterns-9876543210"
	m := &Middleware{oauthService: &OAuthService{jwtSecret: []byte(secret)}}

	// Lowercase authorization header path.
	_, err := m.RequireAuth(context.Background(), events.APIGatewayV2HTTPRequest{
		Headers: map[string]string{
			"authorization": "Bearer not-a-jwt",
			"User-Agent":    "ua",
		},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{SourceIP: "192.0.2.1"},
		},
	})
	require.ErrorIs(t, err, ErrInvalidToken)

	// X-Forwarded-For branch in the error logging path.
	_, err = m.RequireAuth(context.Background(), events.APIGatewayV2HTTPRequest{
		Headers: map[string]string{
			"Authorization":   "Bearer not-a-jwt",
			"X-Forwarded-For": "203.0.113.9",
			"User-Agent":      "ua",
		},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{SourceIP: "192.0.2.1"},
		},
	})
	require.ErrorIs(t, err, ErrInvalidToken)
}
