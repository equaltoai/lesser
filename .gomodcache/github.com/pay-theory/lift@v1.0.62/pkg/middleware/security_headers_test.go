package middleware

import (
	"context"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeaders(t *testing.T) {
	t.Run("Default security headers applied", func(t *testing.T) {
		config := DefaultSecurityHeadersConfig()
		middleware := SecurityHeaders(config)

		ctx := createSecurityTestContext("GET", "/test", nil)

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"status": "ok"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		// Check all security headers are set
		assert.Equal(t, config.ContentSecurityPolicy, ctx.Response.Headers["Content-Security-Policy"])
		assert.Equal(t, "DENY", ctx.Response.Headers["X-Frame-Options"])
		assert.Equal(t, "nosniff", ctx.Response.Headers["X-Content-Type-Options"])
		assert.Equal(t, "1; mode=block", ctx.Response.Headers["X-XSS-Protection"])
		assert.Equal(t, config.ReferrerPolicy, ctx.Response.Headers["Referrer-Policy"])
		assert.Equal(t, config.PermissionsPolicy, ctx.Response.Headers["Permissions-Policy"])
	})

	t.Run("HTTPS detection sets HSTS header", func(t *testing.T) {
		config := DefaultSecurityHeadersConfig()
		middleware := SecurityHeaders(config)

		ctx := createSecurityTestContext("GET", "/test", nil)
		ctx.Request.Headers["X-Forwarded-Proto"] = "https"

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"status": "ok"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		assert.Equal(t, config.StrictTransportSecurity, ctx.Response.Headers["Strict-Transport-Security"])
	})

	t.Run("HTTP request does not set HSTS header", func(t *testing.T) {
		config := DefaultSecurityHeadersConfig()
		middleware := SecurityHeaders(config)

		ctx := createSecurityTestContext("GET", "/test", nil)
		// No HTTPS headers set

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"status": "ok"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		// HSTS header should not be set for HTTP
		assert.Empty(t, ctx.Response.Headers["Strict-Transport-Security"])
	})

	t.Run("Sensitive paths get cache control headers", func(t *testing.T) {
		config := DefaultSecurityHeadersConfig()
		middleware := SecurityHeaders(config)

		sensitivePaths := []string{
			"/api/auth/login",
			"/api/payment/process",
			"/api/users/profile",
			"/admin/dashboard",
		}

		for _, path := range sensitivePaths {
			ctx := createSecurityTestContext("GET", path, nil)

			handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
				return ctx.OK(map[string]string{"status": "ok"})
			}))

			err := handler.Handle(ctx)
			require.NoError(t, err)

			assert.Equal(t, "no-store, no-cache, must-revalidate, private", ctx.Response.Headers["Cache-Control"])
			assert.Equal(t, "no-cache", ctx.Response.Headers["Pragma"])
			assert.Equal(t, "0", ctx.Response.Headers["Expires"])
		}
	})

	t.Run("Non-sensitive paths do not get cache control headers", func(t *testing.T) {
		config := DefaultSecurityHeadersConfig()
		middleware := SecurityHeaders(config)

		ctx := createSecurityTestContext("GET", "/api/public/info", nil)

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"status": "ok"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		// Should not have cache control headers
		assert.Empty(t, ctx.Response.Headers["Cache-Control"])
		assert.Empty(t, ctx.Response.Headers["Pragma"])
		assert.Empty(t, ctx.Response.Headers["Expires"])
	})

	t.Run("Development environment can skip headers", func(t *testing.T) {
		config := DefaultSecurityHeadersConfig()
		config.IncludeInDevelopment = false
		middleware := SecurityHeaders(config)

		ctx := createSecurityTestContext("GET", "/test", nil)
		ctx.Set("environment", "development")

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"status": "ok"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		// Headers should not be set in development
		assert.Empty(t, ctx.Response.Headers["Content-Security-Policy"])
		assert.Empty(t, ctx.Response.Headers["X-Frame-Options"])
	})

	t.Run("Custom headers are applied", func(t *testing.T) {
		config := DefaultSecurityHeadersConfig()
		config.CustomHeaders = map[string]string{
			"X-Custom-Security": "enabled",
			"X-API-Version":     "v1.0",
		}
		middleware := SecurityHeaders(config)

		ctx := createSecurityTestContext("GET", "/test", nil)

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"status": "ok"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		assert.Equal(t, "enabled", ctx.Response.Headers["X-Custom-Security"])
		assert.Equal(t, "v1.0", ctx.Response.Headers["X-API-Version"])
	})
}

func TestStrictSecurityHeaders(t *testing.T) {
	t.Run("Strict headers are more restrictive", func(t *testing.T) {
		middleware := StrictSecurityHeaders()

		ctx := createSecurityTestContext("GET", "/test", nil)
		ctx.Request.Headers["X-Forwarded-Proto"] = "https"

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"status": "ok"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		// Check strict CSP
		csp := ctx.Response.Headers["Content-Security-Policy"]
		assert.Contains(t, csp, "default-src 'none'")

		// Check strict referrer policy
		assert.Equal(t, "no-referrer", ctx.Response.Headers["Referrer-Policy"])

		// Check additional security headers
		assert.Equal(t, "none", ctx.Response.Headers["X-Permitted-Cross-Domain-Policies"])
		assert.Equal(t, "require-corp", ctx.Response.Headers["Cross-Origin-Embedder-Policy"])
		assert.Equal(t, "same-origin", ctx.Response.Headers["Cross-Origin-Opener-Policy"])
	})
}

func TestAPISecurityHeaders(t *testing.T) {
	t.Run("API headers are optimized for APIs", func(t *testing.T) {
		middleware := APISecurityHeaders()

		ctx := createSecurityTestContext("GET", "/api/v1/data", nil)

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"data": "value"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		// Check API-specific CSP
		csp := ctx.Response.Headers["Content-Security-Policy"]
		assert.Contains(t, csp, "default-src 'none'")
		assert.Contains(t, csp, "frame-ancestors 'none'")

		// Check API version header
		assert.Equal(t, "1.0", ctx.Response.Headers["X-API-Version"])
	})
}

func TestSecurityAuditHeaders(t *testing.T) {
	t.Run("Audit headers are added", func(t *testing.T) {
		middleware := SecurityAuditHeaders()

		ctx := createSecurityTestContext("GET", "/test", nil)

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.OK(map[string]string{"status": "ok"})
		}))

		err := handler.Handle(ctx)
		require.NoError(t, err)

		assert.Equal(t, "enabled", ctx.Response.Headers["X-Security-Headers"])
		assert.Contains(t, ctx.Response.Headers["X-Security-Audit"], "scan-date-")
	})
}

func TestIsSensitivePath(t *testing.T) {
	testCases := []struct {
		path      string
		sensitive bool
	}{
		{"/api/auth/login", true},
		{"/api/payment/process", true},
		{"/api/users/profile", true},
		{"/api/accounts/balance", true},
		{"/admin/dashboard", true},
		{"/dashboard/overview", true},
		{"/api/public/info", false},
		{"/health", false},
		{"/static/css/main.css", false},
		{"/", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := isSensitivePath(tc.path)
			assert.Equal(t, tc.sensitive, result, "Path %s sensitivity mismatch", tc.path)
		})
	}
}

// Helper function to create test context
func createSecurityTestContext(method, path string, body []byte) *lift.Context {
	adapterReq := &adapters.Request{
		Method:      method,
		Path:        path,
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		PathParams:  make(map[string]string),
		Body:        body,
	}
	req := lift.NewRequest(adapterReq)
	ctx := lift.NewContext(context.Background(), req)

	// Initialize response headers if not already done
	if ctx.Response.Headers == nil {
		ctx.Response.Headers = make(map[string]string)
	}

	return ctx
}
