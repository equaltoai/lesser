package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSanitizeLogPathRedactsPrivateMintConversationIDs(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in          string
		rawSecret   string
		wantPrefix  string
		mustNotLeak []string
	}{
		"single read": {
			in:         "/api/v1/souls/bound/me/mint-conversations/conv-private-1",
			rawSecret:  "conv-private-1",
			wantPrefix: "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:",
		},
		"single read strips query cursor": {
			in:          "/api/v1/souls/bound/me/mint-conversations/conv-private-2?cursor=raw-cursor",
			rawSecret:   "conv-private-2",
			wantPrefix:  "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:",
			mustNotLeak: []string{"raw-cursor"},
		},
		"single read strips fragment": {
			in:          "/api/v1/souls/bound/me/mint-conversations/conv-private-3#raw-fragment",
			rawSecret:   "conv-private-3",
			wantPrefix:  "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:",
			mustNotLeak: []string{"raw-fragment"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeLogPath(tc.in)
			require.Truef(t, strings.HasPrefix(got, tc.wantPrefix), "expected sanitized prefix %q, got %q", tc.wantPrefix, got)
			require.NotContains(t, got, tc.rawSecret)
			for _, forbidden := range tc.mustNotLeak {
				require.NotContains(t, got, forbidden)
			}
		})
	}
}

func TestSanitizeLogPathHandlesPrivateMintConversationListRoute(t *testing.T) {
	t.Parallel()

	raw := "/api/v1/souls/bound/me/mint-conversations?cursor=raw-cursor"
	got := sanitizeLogPath(raw)
	require.Equal(t, "/api/v1/souls/bound/me/mint-conversations", got)
	require.NotContains(t, got, "raw-cursor")
}

func TestSanitizeLogPathLeavesOtherRoutesUnchanged(t *testing.T) {
	t.Parallel()

	path := "/api/v1/statuses/conv-private-1?cursor=raw-cursor"
	require.Equal(t, path, sanitizeLogPath(path))
}

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
