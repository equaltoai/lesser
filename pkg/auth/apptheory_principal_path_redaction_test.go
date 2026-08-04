package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestAppTheoryPrincipalResolverSanitizesPrivateMintConversationPathInAuthFailureLogs(t *testing.T) {
	t.Run("malformed bearer header", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		resolver := newAppTheoryPrincipalResolver(func(string) (*Claims, error) {
			t.Fatal("validator should not run for malformed bearer headers")
			return nil, nil
		}, zap.New(core), "api")

		rawConversationID := "conv-principal-malformed"
		rawCursor := "raw-principal-cursor"
		ctx := newTestContext("GET", "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID+"?cursor="+rawCursor, withHeaders(map[string]string{
			"Authorization": "Token nope",
		}))

		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		require.Nil(t, principal)

		requireAuthLogSanitizedPath(t, observed, "principal authentication skipped - invalid bearer token format", rawConversationID, rawCursor)
	})

	t.Run("invalid bearer token", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		resolver := newAppTheoryPrincipalResolver(func(token string) (*Claims, error) {
			require.Equal(t, "bad-token", token)
			return nil, errors.New("bad token")
		}, zap.New(core), "api")

		rawConversationID := "conv-principal-invalid"
		rawCursor := "raw-invalid-cursor"
		ctx := newTestContext("GET", "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID+"?cursor="+rawCursor, withHeaders(map[string]string{
			"Authorization": "Bearer bad-token",
		}))

		principal, err := resolver.Resolve(ctx)
		require.NoError(t, err)
		require.Nil(t, principal)

		requireAuthLogSanitizedPath(t, observed, "principal authentication failed - header present but validation failed", rawConversationID, rawCursor)
	})
}

func TestOptionalAuthMiddlewareSanitizesPrivateMintConversationPathInAuthFailureLogs(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)

	rawConversationID := "conv-optional-invalid"
	rawCursor := "raw-optional-cursor"
	ctx := newTestContext("GET", "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID+"?cursor="+rawCursor, withHeaders(map[string]string{
		"Authorization": "Bearer bad-token",
	}))

	resp, err := OptionalAuth(MiddlewareConfig{
		OAuthService: oauthServiceStub{err: errors.New("bad token")},
		Logger:       zap.New(core),
		ServiceName:  "api",
	})(func(*apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{Status: 200}, nil
	})(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	requireAuthLogSanitizedPath(t, observed, "optional authentication failed - header present but validation failed", rawConversationID, rawCursor)
}

func requireAuthLogSanitizedPath(t *testing.T, observed *observer.ObservedLogs, message string, rawParts ...string) {
	t.Helper()

	entries := observed.FilterMessage(message).All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "api", fields["service"])
	require.Equal(t, true, fields["has_auth_header"])
	path, ok := fields["path"].(string)
	require.True(t, ok)
	require.Contains(t, path, "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:")
	for _, raw := range rawParts {
		require.NotContains(t, path, raw)
	}
}
