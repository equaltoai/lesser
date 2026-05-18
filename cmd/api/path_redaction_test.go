package main

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestCreateLoggingMiddlewareSanitizesPrivateMintConversationPath(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	mw := createLoggingMiddleware(logger)

	rawConversationID := "conv-private-log"
	rawCursor := "raw-cursor"
	ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID+"?cursor="+rawCursor)

	resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(http.StatusOK, ""), nil
	})(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)

	for _, entry := range observed.All() {
		fields := entry.ContextMap()
		path, ok := fields["path"].(string)
		require.Truef(t, ok, "expected log path field in %s", entry.Message)
		require.Contains(t, path, "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:")
		require.NotContains(t, path, rawConversationID)
		require.NotContains(t, path, rawCursor)
	}
}

func TestCreateLoggingMiddlewareSanitizesPathOnErrors(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	mw := createLoggingMiddleware(logger)

	rawConversationID := "conv-private-error"
	ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID)

	_, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
		return nil, errors.New("boom")
	})(ctx)
	require.Error(t, err)

	require.NotEmpty(t, observed.All())
	for _, entry := range observed.All() {
		fields := entry.ContextMap()
		path, ok := fields["path"].(string)
		require.Truef(t, ok, "expected log path field in %s", entry.Message)
		require.Contains(t, path, "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:")
		require.NotContains(t, path, rawConversationID)
	}
}

func TestExtractRequestInfoSanitizesPrivateMintConversationPath(t *testing.T) {
	rawConversationID := "conv-private-metric"
	ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations/"+rawConversationID+"?cursor=raw-cursor")

	info := extractRequestInfo(ctx)
	require.Equal(t, http.MethodGet, info.method)
	require.Contains(t, info.path, "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:")
	require.NotContains(t, info.path, rawConversationID)
	require.NotContains(t, info.path, "raw-cursor")
	require.Equal(t, info.method+" "+info.path, info.endpoint)
}

func TestExtractRequestInfoSanitizesConversationIDPath(t *testing.T) {
	rawConversationID := "conv-agent-readable"
	ctx := newTestAppTheoryContext(http.MethodGet, "/api/v1/conversations/"+rawConversationID+"?max_id=raw-cursor")

	info := extractRequestInfo(ctx)
	require.Equal(t, http.MethodGet, info.method)
	require.Contains(t, info.path, "/api/v1/conversations/conversation-sha256:")
	require.NotContains(t, info.path, rawConversationID)
	require.NotContains(t, info.path, "raw-cursor")
	require.Equal(t, info.method+" "+info.path, info.endpoint)
}

func TestExtractRequestInfoSanitizesConversationReadPath(t *testing.T) {
	rawConversationID := "conv-read-path"
	ctx := newTestAppTheoryContext(http.MethodPost, "/api/v1/conversations/"+rawConversationID+"/read")

	info := extractRequestInfo(ctx)
	require.Equal(t, http.MethodPost, info.method)
	require.Contains(t, info.path, "/api/v1/conversations/conversation-sha256:")
	require.Contains(t, info.path, "/read")
	require.NotContains(t, info.path, rawConversationID)
	require.Equal(t, info.method+" "+info.path, info.endpoint)
}
