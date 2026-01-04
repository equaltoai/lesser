package middleware

import (
	"context"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestApplySecurityMiddleware_EndToEnd(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "https://app.example")

	logger := zap.NewNop()

	t.Run("api security", func(t *testing.T) {
		app := lift.New()
		ApplySecurityMiddleware(app, SecurityTypeAPI, logger)
		require.NoError(t, app.Handle("GET", "/api/test", func(ctx *lift.Context) error {
			return ctx.Status(200).Text("ok")
		}))

		ctx := newTestLiftContext("GET", "/api/test")
		ctx.Request.Headers["Origin"] = "https://app.example"
		require.NoError(t, app.HandleTestRequest(ctx))

		assert.Equal(t, 200, ctx.Response.StatusCode)
		assert.Equal(t, "https://app.example", ctx.Response.Headers["Access-Control-Allow-Origin"])
		assert.Equal(t, "true", ctx.Response.Headers["Access-Control-Allow-Credentials"])
		assert.NotEmpty(t, ctx.Response.Headers["Content-Security-Policy"])
		assert.NotContains(t, ctx.Response.Headers["Content-Security-Policy"], "'unsafe-inline'")
		assert.NotEmpty(t, ctx.Get("csp-nonce"))
	})

	t.Run("federation security", func(t *testing.T) {
		app := lift.New()
		ApplySecurityMiddleware(app, SecurityTypeFederation, logger)
		require.NoError(t, app.Handle("GET", "/inbox", func(ctx *lift.Context) error {
			return ctx.Status(200).Text("ok")
		}))

		ctx := newTestLiftContext("GET", "/inbox")
		ctx.Request.Headers["Origin"] = "https://mastodon.example"
		require.NoError(t, app.HandleTestRequest(ctx))

		assert.Equal(t, "*", ctx.Response.Headers["Access-Control-Allow-Origin"])
		assert.Empty(t, ctx.Response.Headers["Content-Security-Policy"])
		assert.Equal(t, "noindex, nofollow", ctx.Response.Headers["X-Robots-Tag"])
	})

	t.Run("media security", func(t *testing.T) {
		app := lift.New()
		ApplySecurityMiddleware(app, SecurityTypeMedia, logger)
		require.NoError(t, app.Handle("GET", "/media/file", func(ctx *lift.Context) error {
			return ctx.Status(200).Text("ok")
		}))

		ctx := newTestLiftContext("GET", "/media/file")
		ctx.Request.Headers["Origin"] = "https://app.example"
		require.NoError(t, app.HandleTestRequest(ctx))

		assert.Equal(t, "public, max-age=31536000, immutable", ctx.Response.Headers["Cache-Control"])
	})

	t.Run("websocket security", func(t *testing.T) {
		app := lift.New()
		ApplySecurityMiddleware(app, SecurityTypeWebSocket, logger)
		require.NoError(t, app.Handle("GET", "/streaming", func(ctx *lift.Context) error {
			return ctx.Status(200).Text("ok")
		}))

		ctx := newTestLiftContext("GET", "/streaming")
		ctx.Request.Headers["Origin"] = "https://app.example"
		require.NoError(t, app.HandleTestRequest(ctx))

		assert.Equal(t, "v1", ctx.Response.Headers["X-WebSocket-Protocol"])
	})
}

func TestCreateLiftCORSMiddleware_Preflight(t *testing.T) {
	logger := zap.NewNop()
	cfg := GetFederationCORSConfig()
	mw := createLiftCORSMiddleware(&cfg, logger)

	ctx := newTestLiftContext("OPTIONS", "/inbox")
	ctx.Request.Headers["Origin"] = "https://mastodon.example"

	called := false
	err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		called = true
		return ctx.Status(200).Text("ok")
	})).Handle(ctx)
	require.NoError(t, err)

	assert.False(t, called)
	assert.Equal(t, 204, ctx.Response.StatusCode)
	assert.Equal(t, "*", ctx.Response.Headers["Access-Control-Allow-Origin"])
	assert.Contains(t, ctx.Response.Headers["Access-Control-Allow-Methods"], "POST")
	assert.Equal(t, "Origin", ctx.Response.Headers["Vary"])
}

func TestCreateLiftCORSMiddleware_ActualRequest(t *testing.T) {
	logger := zap.NewNop()
	cfg := GetWebClientCORSConfig()
	cfg.ExposedHeaders = []string{"Location", "X-Test"}
	cfg.AllowedOrigins = []string{"https://app.example"}
	mw := createLiftCORSMiddleware(&cfg, logger)

	ctx := newTestLiftContext("GET", "/oauth/authorize")
	ctx.Request.Headers["Origin"] = "https://app.example"

	called := false
	err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		called = true
		return ctx.Status(200).Text("ok")
	})).Handle(ctx)
	require.NoError(t, err)

	assert.True(t, called)
	assert.Equal(t, "https://app.example", ctx.Response.Headers["Access-Control-Allow-Origin"])
	assert.Equal(t, "Location, X-Test", ctx.Response.Headers["Access-Control-Expose-Headers"])
	assert.Equal(t, "Location, X-Test", ctx.Get("_cors_exposed_headers"))
	assert.Contains(t, ctx.Response.Headers["Vary"], "Origin")
}

func TestCreateLiftBodyLimitMiddleware(t *testing.T) {
	logger := zap.NewNop()
	mw := createLiftBodyLimitMiddleware(5, logger)

	t.Run("rejects by content-length header", func(t *testing.T) {
		ctx := newTestLiftContext("POST", "/api/test")
		ctx.Request.Headers["Content-Length"] = "6"

		called := false
		err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			called = true
			return nil
		})).Handle(ctx)
		require.NoError(t, err)

		assert.False(t, called)
		assert.Equal(t, 413, ctx.Response.StatusCode)
	})

	t.Run("rejects by body size", func(t *testing.T) {
		ctx := newTestLiftContext("POST", "/api/test")
		ctx.Request.Body = []byte("123456")

		called := false
		err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			called = true
			return nil
		})).Handle(ctx)
		require.NoError(t, err)

		assert.False(t, called)
		assert.Equal(t, 413, ctx.Response.StatusCode)
	})

	t.Run("allows small body through", func(t *testing.T) {
		ctx := newTestLiftContext("POST", "/api/test")
		ctx.Request.Body = []byte("123")

		called := false
		err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			called = true
			return ctx.Status(200).Text("ok")
		})).Handle(ctx)
		require.NoError(t, err)

		assert.True(t, called)
		assert.Equal(t, 200, ctx.Response.StatusCode)
	})
}

func TestInputValidationMiddleware(t *testing.T) {
	logger := zap.NewNop()
	mw := createInputValidationMiddleware(logger)

	t.Run("skips non-federation endpoints", func(t *testing.T) {
		ctx := newTestLiftContext("POST", "/api/test")
		ctx.Request.Body = make([]byte, 2*1024*1024)

		called := false
		err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			called = true
			return ctx.Status(200).Text("ok")
		})).Handle(ctx)
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("rejects oversized federation body", func(t *testing.T) {
		ctx := newTestLiftContext("POST", "/inbox")
		ctx.Request.Body = make([]byte, 1024*1024+1)

		called := false
		err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			called = true
			return nil
		})).Handle(ctx)
		require.NoError(t, err)
		assert.False(t, called)
		assert.Equal(t, 413, ctx.Response.StatusCode)
	})

	t.Run("rejects invalid federation content-type", func(t *testing.T) {
		ctx := newTestLiftContext("POST", "/outbox")
		ctx.Request.Headers["Content-Type"] = "text/plain"

		err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.Status(200).Text("ok")
		})).Handle(ctx)
		require.NoError(t, err)
		assert.Equal(t, 400, ctx.Response.StatusCode)
	})

	t.Run("allows valid federation content-type", func(t *testing.T) {
		ctx := newTestLiftContext("POST", "/outbox")
		ctx.Request.Headers["Content-Type"] = "application/activity+json; charset=utf-8"

		called := false
		err := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			called = true
			return ctx.Status(200).Text("ok")
		})).Handle(ctx)
		require.NoError(t, err)
		assert.True(t, called)
	})
}

func TestApplyInputValidation_RegistersMiddleware(t *testing.T) {
	logger := zap.NewNop()
	app := lift.New()
	ApplyInputValidation(app, logger)

	require.NoError(t, app.Handle("POST", "/inbox", func(ctx *lift.Context) error {
		return ctx.Status(200).Text("ok")
	}))

	ctx := newTestLiftContext("POST", "/inbox")
	ctx.Request.Headers["Content-Type"] = "text/plain"
	require.NoError(t, app.HandleTestRequest(ctx))

	assert.Equal(t, 400, ctx.Response.StatusCode)
}

func TestSecurityServiceHelpers(t *testing.T) {
	assert.True(t, isFederationEndpointForValidation("/inbox"))
	assert.True(t, isFederationEndpointForValidation("/outbox"))
	assert.False(t, isFederationEndpointForValidation("/api/test"))

	assert.True(t, isValidActivityPubContentType("application/activity+json"))
	assert.True(t, isValidActivityPubContentType("Application/LD+JSON"))
	assert.False(t, isValidActivityPubContentType("text/plain"))

	assert.Equal(t, SecurityTypeAPI, GetSecurityTypeForService("api"))
	assert.Equal(t, SecurityTypeFederation, GetSecurityTypeForService("inbox"))
	assert.Equal(t, SecurityTypeMedia, GetSecurityTypeForService("media"))
	assert.Equal(t, SecurityTypeWebSocket, GetSecurityTypeForService("websocket"))
	assert.Equal(t, SecurityTypeAPI, GetSecurityTypeForService("unknown-service"))

	ctx := CreateSecurityContext(context.Background(), SecurityTypeFederation, zap.NewNop())
	assert.Equal(t, SecurityTypeFederation, ctx.Value(securityTypeKey))
}
