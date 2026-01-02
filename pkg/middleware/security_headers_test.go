package middleware

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestLiftContext(method, path string) *lift.Context {
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method:      method,
			Path:        path,
			Headers:     make(map[string]string),
			QueryParams: make(map[string]string),
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}

	// Initialize internal maps that Lift uses for ctx.Set/ctx.Get.
	ctx.Set("__test", "init")
	ctx.Get("__test")

	return ctx
}

func TestEnhancedSecurityHeaders_Middleware_SetsResponseHeaders(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultSecurityHeadersConfig()
	config.CustomHeaders = map[string]string{"X-Test": "ok"}
	sh := NewEnhancedSecurityHeaders(config, logger)

	ctx := newTestLiftContext(http.MethodGet, "/api/test")
	err := sh.Middleware()(func(ctx *lift.Context) error {
		return ctx.Status(200).Text("ok")
	})(ctx)
	require.NoError(t, err)

	nonce, _ := ctx.Get("csp-nonce").(string)
	require.NotEmpty(t, nonce)

	require.NotEmpty(t, ctx.Response.Headers["Content-Security-Policy"])
	assert.Contains(t, ctx.Response.Headers["Content-Security-Policy"], "'nonce-")
	assert.Equal(t, "ok", ctx.Response.Headers["X-Test"])
	assert.Equal(t, "", ctx.Response.Headers["X-Powered-By"])
}

func TestNewEnhancedSecurityHeaders_DefaultConfigWhenNil(t *testing.T) {
	sh := NewEnhancedSecurityHeaders(nil, zap.NewNop())
	require.NotNil(t, sh)
	require.NotNil(t, sh.config)
	assert.True(t, sh.config.EnableCSP)
}

func TestDevelopmentSecurityHeadersConfig(t *testing.T) {
	cfg := DevelopmentSecurityHeadersConfig()
	assert.True(t, cfg.DevelopmentMode)
	assert.False(t, cfg.EnableHSTS)
	assert.Equal(t, "SAMEORIGIN", cfg.XFrameOptions)
	assert.Contains(t, strings.Join(cfg.CSPDirectives["connect-src"], " "), "localhost")
}

func TestGetSecurityConfigForEndpoint(t *testing.T) {
	federation := GetSecurityConfigForEndpoint("/inbox")
	require.NotNil(t, federation)
	assert.False(t, federation.EnableCSP)
	assert.Equal(t, "cross-origin", federation.CrossOriginResourcePolicy)

	media := GetSecurityConfigForEndpoint("/media/file.jpg")
	require.NotNil(t, media)
	assert.Equal(t, "nosniff", media.XContentTypeOptions)
	assert.Equal(t, "bytes", media.CustomHeaders["Accept-Ranges"])

	api := GetSecurityConfigForEndpoint("/api/v1/statuses")
	require.NotNil(t, api)
	assert.True(t, api.EnableCSP)
	assert.Equal(t, "DENY", api.XFrameOptions)

	ws := GetSecurityConfigForEndpoint("/api/v1/streaming/user")
	require.NotNil(t, ws)
	assert.Equal(t, "v1", ws.CustomHeaders["X-WebSocket-Protocol"])

	fallback := GetSecurityConfigForEndpoint("/some/other/path")
	require.NotNil(t, fallback)
	assert.True(t, fallback.EnableCSP)
}

func TestBodyLimits(t *testing.T) {
	mw := BodyLimits()

	t.Run("rejects oversized oauth request by content-length", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodPost, "/oauth/token")
		ctx.Request.Headers["Content-Length"] = "20000"

		called := false
		err := mw(func(ctx *lift.Context) error {
			called = true
			return ctx.Status(200).Text("ok")
		})(ctx)
		require.NoError(t, err)
		assert.False(t, called)
		assert.Equal(t, 413, ctx.Response.StatusCode)
	})

	t.Run("allows small request body through", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodPost, "/oauth/token")
		ctx.Request.Body = []byte("small")

		called := false
		err := mw(func(ctx *lift.Context) error {
			called = true
			return ctx.Status(200).Text("ok")
		})(ctx)
		require.NoError(t, err)
		assert.True(t, called)
		assert.Equal(t, 200, ctx.Response.StatusCode)
	})

	t.Run("rejects oversized inbox request by content-length", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodPost, "/inbox")
		ctx.Request.Headers["Content-Length"] = "1048577"

		err := mw(func(ctx *lift.Context) error {
			return ctx.Status(200).Text("ok")
		})(ctx)
		require.NoError(t, err)
		assert.Equal(t, 413, ctx.Response.StatusCode)

		body, ok := ctx.Response.Body.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(1048576), body["max_size"])
	})

	t.Run("invalid content-length falls back to body size check", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodPost, "/api/v1/statuses")
		ctx.Request.Headers["Content-Length"] = "not-an-int"
		ctx.Request.Body = make([]byte, 512*1024+1)

		err := mw(func(ctx *lift.Context) error {
			return ctx.Status(200).Text("ok")
		})(ctx)
		require.NoError(t, err)
		assert.Equal(t, 413, ctx.Response.StatusCode)
	})
}

func TestSecurityHeaders_LambdaWrapper(t *testing.T) {
	handler := func(_ events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return &events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	}

	wrapped := SecurityHeaders(handler)
	resp, err := wrapped(events.APIGatewayV2HTTPRequest{})
	require.NoError(t, err)
	assert.Equal(t, "nosniff", resp.Headers["X-Content-Type-Options"])
	assert.Equal(t, "DENY", resp.Headers["X-Frame-Options"])
	assert.NotEmpty(t, resp.Headers["Strict-Transport-Security"])
}

func TestSecurityHeadersHTTP(t *testing.T) {
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("adds HSTS for forwarded https", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")

		rec := httptest.NewRecorder()
		SecurityHeadersHTTP(base).ServeHTTP(rec, req)

		assert.NotEmpty(t, rec.Header().Get("Strict-Transport-Security"))
	})

	t.Run("adds HSTS when TLS is present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.TLS = &tls.ConnectionState{}

		rec := httptest.NewRecorder()
		SecurityHeadersHTTP(base).ServeHTTP(rec, req)

		assert.NotEmpty(t, rec.Header().Get("Strict-Transport-Security"))
	})

	t.Run("does not add HSTS for plain http", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		rec := httptest.NewRecorder()
		SecurityHeadersHTTP(base).ServeHTTP(rec, req)

		assert.Empty(t, rec.Header().Get("Strict-Transport-Security"))
	})
}

func TestSecurityHeadersWithConfig(t *testing.T) {
	config := DefaultSecurityConfig
	config.ContentSecurityPolicy = ""
	config.PermissionsPolicy = ""

	wrapped := SecurityHeadersWithConfig(config)(func(_ events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return &events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	})

	resp, err := wrapped(events.APIGatewayV2HTTPRequest{})
	require.NoError(t, err)

	assert.Equal(t, config.ContentTypeOptions, resp.Headers["X-Content-Type-Options"])
	assert.Equal(t, config.FrameOptions, resp.Headers["X-Frame-Options"])
	assert.Empty(t, resp.Headers["Content-Security-Policy"])
	assert.Empty(t, resp.Headers["Permissions-Policy"])
}

func TestAPISecurityHeadersAndWebSocketSecurityHeaders(t *testing.T) {
	api := APISecurityHeaders()
	assert.True(t, api.EnableCSP)
	assert.Equal(t, "DENY", api.XFrameOptions)

	ws := WebSocketSecurityHeaders()
	assert.True(t, ws.EnableCSP)
	assert.Equal(t, "DENY", ws.XFrameOptions)
	assert.Equal(t, "v1", ws.CustomHeaders["X-WebSocket-Protocol"])
}

func TestCSPReportHandler(t *testing.T) {
	logger := zap.NewNop()
	sh := NewEnhancedSecurityHeaders(DefaultSecurityHeadersConfig(), logger)
	handler := sh.CSPReportHandler()

	t.Run("invalid body returns 400", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodPost, "/api/v1/csp-report")
		ctx.Request.Body = []byte("{")
		err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 400, ctx.Response.StatusCode)
	})

	t.Run("valid report returns 204", func(t *testing.T) {
		ctx := newTestLiftContext(http.MethodPost, "/api/v1/csp-report")
		ctx.Request.Body = []byte(`{"csp-report":{"document-uri":"https://example.com","violated-directive":"script-src","blocked-uri":"https://evil.example","source-file":"https://example.com/app.js","line-number":1,"column-number":2,"status-code":200}}`)
		err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 204, ctx.Response.StatusCode)
	})
}

func TestGetWebClientCORSConfig_EnvFallbacks(t *testing.T) {
	t.Run("uses ALLOWED_ORIGINS when set", func(t *testing.T) {
		t.Setenv("ALLOWED_ORIGINS", "https://a.example,https://b.example")
		cfg := GetWebClientCORSConfig()
		assert.ElementsMatch(t, []string{"https://a.example", "https://b.example"}, cfg.AllowedOrigins)
	})

	t.Run("falls back to DOMAIN_NAME", func(t *testing.T) {
		t.Setenv("ALLOWED_ORIGINS", "")
		t.Setenv("DOMAIN_NAME", "example.com")
		cfg := GetWebClientCORSConfig()
		assert.Equal(t, []string{"https://example.com"}, cfg.AllowedOrigins)
	})

	t.Run("falls back to localhost for development", func(t *testing.T) {
		t.Setenv("ALLOWED_ORIGINS", "")
		t.Setenv("DOMAIN_NAME", "")
		cfg := GetWebClientCORSConfig()
		assert.True(t, strings.Contains(strings.Join(cfg.AllowedOrigins, ","), "localhost:3000"))
	})
}
