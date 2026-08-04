package common

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestErrorHandlingMiddlewareSanitizesPrivateMintConversationPathInPanicLogs(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	cfg := ErrorMiddlewareConfig{
		Logger:              zap.New(core),
		ServiceName:         "svc",
		EnablePanicRecovery: true,
		MaxErrorLogLength:   100,
	}

	rawConversationID := "conv-panic-private"
	rawCursor := "raw-panic-cursor"
	ctx := newTestContext("GET", "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID+"?cursor="+rawCursor)
	resp, err := ErrorHandlingMiddleware(cfg)(func(*apptheory.Context) (*apptheory.Response, error) {
		panic("boom")
	})(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	requireObservedSanitizedPath(t, observed, "panic recovered in error middleware", rawConversationID, rawCursor)
}

func TestErrorHandlingMiddlewareSanitizesPrivateMintConversationPathInAppErrorLogs(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	cfg := ErrorMiddlewareConfig{
		Logger:              zap.New(core),
		ServiceName:         "svc",
		EnablePanicRecovery: false,
		MaxErrorLogLength:   100,
	}

	rawConversationID := "conv-app-error-private"
	ctx := newTestContext("GET", "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID)
	resp, err := ErrorHandlingMiddleware(cfg)(func(*apptheory.Context) (*apptheory.Response, error) {
		return nil, errors.Forbidden("nope")
	})(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	requireObservedSanitizedPath(t, observed, "client error", rawConversationID)
}

func TestTimeoutErrorMiddlewareSanitizesPrivateMintConversationPathLogs(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)

	rawConversationID := "conv-timeout-private"
	rawCursor := "raw-timeout-cursor"
	ctx := newTestContext("GET", "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID+"?cursor="+rawCursor)
	resp, err := TimeoutErrorMiddleware("svc", zap.New(core))(func(*apptheory.Context) (*apptheory.Response, error) {
		return nil, context.DeadlineExceeded
	})(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	requireObservedSanitizedPath(t, observed, "request timeout", rawConversationID, rawCursor)
}

func requireObservedSanitizedPath(t *testing.T, observed *observer.ObservedLogs, message string, rawParts ...string) {
	t.Helper()

	entries := observed.FilterMessage(message).All()
	require.Len(t, entries, 1)
	path, ok := entries[0].ContextMap()["path"].(string)
	require.True(t, ok)
	require.Contains(t, path, "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:")
	for _, raw := range rawParts {
		require.NotContains(t, path, raw)
	}
}
