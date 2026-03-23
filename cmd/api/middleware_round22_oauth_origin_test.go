package main

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestOAuthOriginHelpers_Round22(t *testing.T) {
	t.Run("oauth path detection includes oauth-sensitive surfaces", func(t *testing.T) {
		require.False(t, isOAuthSensitivePath(""))
		require.True(t, isOAuthSensitivePath("/oauth/token"))
		require.True(t, isOAuthSensitivePath("/.well-known/oauth-authorization-server"))
		require.True(t, isOAuthSensitivePath("/api/v1/apps"))
		require.True(t, isOAuthSensitivePath("/api/v1/apps/123/rotate_secret"))
		require.False(t, isOAuthSensitivePath("/api/v1/statuses"))
	})

	t.Run("origin allowlist accepts supported browser clients and localhost dev", func(t *testing.T) {
		cfg := &config.Config{Domain: "sim.example.com"}

		require.True(t, isAllowedOAuthOrigin("https://claude.ai", cfg))
		require.True(t, isAllowedOAuthOrigin("https://claude.com/", cfg))
		require.True(t, isAllowedOAuthOrigin("https://sim.example.com", cfg))
		require.True(t, isAllowedOAuthOrigin("http://localhost:3000", cfg))
		require.True(t, isAllowedOAuthOrigin("http://127.0.0.1:5173", cfg))
		require.True(t, isAllowedOAuthOrigin("http://[::1]:4173", cfg))

		require.False(t, isAllowedOAuthOrigin("https://evil.example.com", cfg))
		require.False(t, isAllowedOAuthOrigin("https://localhost:3000", cfg))
		require.False(t, isAllowedOAuthOrigin("https://sim.example.com", nil))
		require.False(t, isAllowedOAuthOrigin("null", cfg))
		require.False(t, isAllowedOAuthOrigin("https://claude.ai/app", cfg))
	})

	t.Run("origin normalization rejects invalid forms", func(t *testing.T) {
		_, _, ok := normalizeOrigin("")
		require.False(t, ok)

		_, _, ok = normalizeOrigin("https://claude.ai/path")
		require.False(t, ok)

		_, _, ok = normalizeOrigin("https://claude.ai?state=1")
		require.False(t, ok)
	})

	t.Run("localhost helper only allows http loopback origins", func(t *testing.T) {
		require.False(t, isAllowedLocalDevelopmentOrigin(nil))

		parsed, err := url.Parse("https://localhost:3000")
		require.NoError(t, err)
		require.False(t, isAllowedLocalDevelopmentOrigin(parsed))
	})
}

func TestOAuthOriginRestrictionMiddleware_Round22(t *testing.T) {
	cfg := &config.Config{Domain: "sim.example.com"}
	mw := createOAuthOriginRestrictionMiddleware(cfg)

	tests := []struct {
		name       string
		path       string
		origin     string
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "allows claude on oauth endpoint",
			path:       "/oauth/token",
			origin:     "https://claude.ai",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "allows instance origin on oauth endpoint",
			path:       "/oauth/token",
			origin:     "https://sim.example.com",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "allows localhost dev origin on oauth endpoint",
			path:       "/oauth/token",
			origin:     "http://localhost:5173",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "blocks unrelated origins on oauth endpoint",
			path:       "/oauth/token",
			origin:     "https://evil.example.com",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
		},
		{
			name:       "blocks unrelated origins on app registration endpoint",
			path:       "/api/v1/apps",
			origin:     "https://evil.example.com",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
		},
		{
			name:       "allows requests with no origin",
			path:       "/oauth/token",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "leaves non oauth endpoints alone",
			path:       "/api/v1/statuses",
			origin:     "https://evil.example.com",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestAppTheoryContext(http.MethodPost, tc.path)
			if tc.origin != "" {
				ctx.Request.Headers["origin"] = []string{tc.origin}
			}

			called := false
			resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
				called = true
				return apptheory.Text(http.StatusOK, "ok"), nil
			})(ctx)

			require.NoError(t, err)
			require.Equal(t, tc.wantNext, called)
			require.Equal(t, tc.wantStatus, resp.Status)
		})
	}
}
