package main

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

func TestOAuthOriginHelpers_Round22(t *testing.T) {
	t.Run("oauth path detection includes oauth-sensitive surfaces", func(t *testing.T) {
		require.False(t, isOAuthSensitivePath(""))
		require.True(t, isOAuthSensitivePath("/oauth/token"))
		require.True(t, isOAuthSensitivePath("/token"))
		require.True(t, isOAuthSensitivePath("/register"))
		require.True(t, isOAuthSensitivePath("/authorize"))
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

		_, _, ok = normalizeOrigin("://bad")
		require.False(t, ok)

		_, _, ok = normalizeOrigin("https://user@claude.ai")
		require.False(t, ok)

		_, _, ok = normalizeOrigin("https://claude.ai/path")
		require.False(t, ok)

		_, _, ok = normalizeOrigin("https://claude.ai?state=1")
		require.False(t, ok)

		normalized, parsed, ok := normalizeOrigin("https://claude.ai/")
		require.True(t, ok)
		require.Equal(t, "https://claude.ai", normalized)
		require.NotNil(t, parsed)
	})

	t.Run("localhost helper only allows http loopback origins", func(t *testing.T) {
		require.False(t, isAllowedLocalDevelopmentOrigin(nil))

		parsed, err := url.Parse("http://localhost:3000")
		require.NoError(t, err)
		require.True(t, isAllowedLocalDevelopmentOrigin(parsed))

		parsed, err = url.Parse("http://example.com:3000")
		require.NoError(t, err)
		require.False(t, isAllowedLocalDevelopmentOrigin(parsed))

		parsed, err = url.Parse("https://localhost:3000")
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
			name:       "blocks unrelated origins on framework token endpoint",
			path:       "/token",
			origin:     "https://evil.example.com",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
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

func TestAPISecurityHeadersMiddleware_Round22(t *testing.T) {
	mw := apiSecurityHeaders()

	t.Run("passes through nil response", func(t *testing.T) {
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return nil, nil
		})(newTestAppTheoryContext(http.MethodGet, "/"))
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("sets defaults without overriding existing values", func(t *testing.T) {
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{
				Status: http.StatusOK,
				Headers: map[string][]string{
					"x-frame-options":         {"SAMEORIGIN"},
					"content-security-policy": {"default-src 'self'"},
				},
			}, nil
		})(newTestAppTheoryContext(http.MethodGet, "/"))
		require.NoError(t, err)
		require.Equal(t, []string{"nosniff"}, resp.Headers["x-content-type-options"])
		require.Equal(t, []string{"SAMEORIGIN"}, resp.Headers["x-frame-options"])
		require.Equal(t, []string{"default-src 'self'"}, resp.Headers["content-security-policy"])
		require.Equal(t, []string{"max-age=31536000; includeSubDomains"}, resp.Headers["strict-transport-security"])
		require.Equal(t, []string{"strict-origin-when-cross-origin"}, resp.Headers["referrer-policy"])
	})

	t.Run("initializes missing header map", func(t *testing.T) {
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: http.StatusOK}, nil
		})(newTestAppTheoryContext(http.MethodGet, "/"))
		require.NoError(t, err)
		require.Equal(t, []string{"default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; object-src 'none'"}, resp.Headers["content-security-policy"])
		require.Equal(t, []string{"same-origin"}, resp.Headers["cross-origin-resource-policy"])
		require.Equal(t, []string{"same-origin"}, resp.Headers["cross-origin-opener-policy"])
		require.Equal(t, []string{"camera=(), geolocation=(), microphone=(), payment=(), usb=()"}, resp.Headers["permissions-policy"])
		require.Equal(t, []string{"none"}, resp.Headers["x-permitted-cross-domain-policies"])
		require.Equal(t, []string{"noindex, nofollow"}, resp.Headers["x-robots-tag"])
	})
}

func TestAPICORSAllowedOrigins_Round23(t *testing.T) {
	t.Run("defaults to instance origin", func(t *testing.T) {
		t.Setenv(apiCORSAllowedOriginsEnv, "")
		t.Setenv(apiCORSAllowedOriginsLegacyEnv, "")

		require.Equal(t, []string{"https://sim.example.com"}, apiCORSAllowedOrigins(&config.Config{Domain: "sim.example.com"}))
		require.Equal(t, []string{}, apiCORSAllowedOrigins(nil))
	})

	t.Run("operator allowlist normalizes and deduplicates origins", func(t *testing.T) {
		t.Setenv(apiCORSAllowedOriginsEnv, " https://SIM.example.com/ , https://app.example, https://sim.example.com, https://bad.example/path ")
		t.Setenv(apiCORSAllowedOriginsLegacyEnv, "")

		require.Equal(t, []string{"https://sim.example.com", "https://app.example"}, apiCORSAllowedOrigins(&config.Config{Domain: "sim.example.com"}))
	})

	t.Run("legacy env alias is still honored", func(t *testing.T) {
		t.Setenv(apiCORSAllowedOriginsEnv, "")
		t.Setenv(apiCORSAllowedOriginsLegacyEnv, "https://legacy.example")

		require.Equal(t, []string{"https://legacy.example"}, apiCORSAllowedOrigins(&config.Config{Domain: "sim.example.com"}))
	})

	t.Run("wildcard requires explicit operator opt in", func(t *testing.T) {
		t.Setenv(apiCORSAllowedOriginsEnv, "*")
		t.Setenv(apiCORSAllowedOriginsLegacyEnv, "")

		require.Equal(t, []string{"*"}, apiCORSAllowedOrigins(&config.Config{Domain: "sim.example.com"}))
	})
}

func TestAPICORSRuntime_Round23(t *testing.T) {
	app := apptheory.New(apptheory.WithCORS(apptheory.CORSConfig{
		AllowedOrigins: apiCORSAllowedOrigins(&config.Config{Domain: "sim.example.com"}),
		AllowHeaders:   []string{"Authorization", "Content-Type"},
	}))

	request := apptheory.Request{
		Method: http.MethodOptions,
		Path:   "/api/v1/statuses",
		Headers: map[string][]string{
			"access-control-request-method": {"GET"},
		},
	}

	request.Headers["origin"] = []string{"https://evil.example"}
	resp := app.Serve(context.Background(), request)
	require.Equal(t, http.StatusNoContent, resp.Status)
	require.Empty(t, resp.Headers["access-control-allow-origin"])

	request.Headers["origin"] = []string{"https://sim.example.com"}
	resp = app.Serve(context.Background(), request)
	require.Equal(t, http.StatusNoContent, resp.Status)
	require.Equal(t, []string{"https://sim.example.com"}, resp.Headers["access-control-allow-origin"])
	require.Equal(t, []string{"origin"}, resp.Headers["vary"])
}
