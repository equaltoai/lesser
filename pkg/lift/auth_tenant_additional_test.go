package lift

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTenant_Strategies(t *testing.T) {
	t.Run("header wins", func(t *testing.T) {
		ctx := createTestContext()
		ctx.Request.Headers["X-Tenant-ID"] = "tenant-h"

		got, err := resolveTenant(ctx)
		require.NoError(t, err)
		assert.Equal(t, "tenant-h", got)
	})

	t.Run("subdomain extraction", func(t *testing.T) {
		ctx := createTestContext()
		ctx.Request.Headers["Host"] = "tenant1.lesser.app"

		got, err := resolveTenant(ctx)
		require.NoError(t, err)
		assert.Equal(t, "tenant1", got)
	})

	t.Run("path extraction", func(t *testing.T) {
		ctx := createTestContext()
		ctx.Request.Path = "/tenant/acme/api/v1"

		got, err := resolveTenant(ctx)
		require.NoError(t, err)
		assert.Equal(t, "acme", got)
	})

	t.Run("claims federated username uses domain tenant", func(t *testing.T) {
		ctx := createTestContext()
		ctx.Set("claims", &auth.EnhancedClaims{Username: "alice@tenant2.lesser.app"})

		got, err := resolveTenant(ctx)
		require.NoError(t, err)
		assert.Equal(t, "tenant2", got)
	})

	t.Run("claims non-federated falls back to deriveTenantFromUsername", func(t *testing.T) {
		ctx := createTestContext()
		ctx.Set("claims", &auth.EnhancedClaims{Username: "tenant3_bob"})

		got, err := resolveTenant(ctx)
		require.NoError(t, err)
		assert.Equal(t, "tenant3", got)
	})

	t.Run("reserved subdomain does not resolve and returns error", func(t *testing.T) {
		ctx := createTestContext()
		ctx.Request.Headers["Host"] = "api.lesser.app"

		_, err := resolveTenant(ctx)
		require.Error(t, err)
	})
}

func TestRequireTenantWithConfig_DefaultTenantFallback(t *testing.T) {
	svc := &LiftAuthService{}
	cfg := DefaultTenantIsolationConfig()
	cfg.DefaultTenantID = "default-tenant"

	mw := svc.RequireTenantWithConfig(cfg)
	ctx := createTestContext()

	handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		tenantID, err := GetTenantID(ctx)
		require.NoError(t, err)
		require.Equal(t, "default-tenant", tenantID)

		tenantCtx, err := GetTenantContext(ctx)
		require.NoError(t, err)
		require.Equal(t, "default-tenant", tenantCtx.TenantID)

		return ctx.Status(200).JSON(map[string]string{"ok": "1"})
	}))

	require.NoError(t, handler.Handle(ctx))
	require.Equal(t, 200, ctx.Response.StatusCode)
}

func TestRequireTenantWithConfig_ForbiddenWhenMissing(t *testing.T) {
	svc := &LiftAuthService{}
	cfg := DefaultTenantIsolationConfig()
	cfg.DefaultTenantID = ""

	mw := svc.RequireTenantWithConfig(cfg)
	ctx := createTestContext()

	handler := mw(lift.HandlerFunc(func(*lift.Context) error { return nil }))
	err := handler.Handle(ctx)
	require.Error(t, err)
	require.Equal(t, 403, ctx.Response.StatusCode)
}

func TestGetClientIP_Extraction(t *testing.T) {
	ctx := createTestContext()

	ctx.Request.Headers["X-Forwarded-For"] = "203.0.113.1, 70.41.3.18"
	assert.Equal(t, "203.0.113.1", getClientIP(ctx))

	ctx2 := createTestContext()
	ctx2.Request.Headers["X-Real-IP"] = "198.51.100.2"
	assert.Equal(t, "198.51.100.2", getClientIP(ctx2))

	ctx3 := createTestContext()
	assert.Equal(t, UnknownIP, getClientIP(ctx3))
}

func TestDeriveTenantFromUsername_AndValidation(t *testing.T) {
	assert.Equal(t, "tenant1", deriveTenantFromUsername("tenant1_user"))
	assert.Equal(t, "default", deriveTenantFromUsername(""))
	assert.Equal(t, "shard1", deriveTenantFromUsername("1user"))
	assert.Equal(t, "sharda", deriveTenantFromUsername("Alice"))
	assert.Equal(t, "default", deriveTenantFromUsername("*notalnum"))

	assert.False(t, isValidTenantID(""))
	assert.False(t, isValidTenantID("A")) // uppercase not allowed by validator
	assert.False(t, isValidTenantID("bad!id"))
	assert.True(t, isValidTenantID("tenant-1"))
	assert.False(t, isValidTenantID(string(make([]byte, 65))))
}

func TestGetTenantContext_CreatesFromTenantID(t *testing.T) {
	ctx := createTestContext()
	ctx.Set("tenant_id", "t1")

	tenantCtx, err := GetTenantContext(ctx)
	require.NoError(t, err)
	require.Equal(t, "t1", tenantCtx.TenantID)
}

func TestExtractBearerToken_ErrorsAndSuccess(t *testing.T) {
	ctx := createTestContext()
	require.Equal(t, "", extractBearerToken(ctx))

	ctx.Request.Headers["Authorization"] = "Basic abc"
	require.Equal(t, "", extractBearerToken(ctx))

	ctx.Request.Headers["Authorization"] = "Bearer token123"
	require.Equal(t, "token123", extractBearerToken(ctx))
}

func TestGetClaims_Unauthorized(t *testing.T) {
	ctx := createTestContext()

	claims, err := GetClaims(ctx)
	// Lift's ctx.Unauthorized may return nil error while setting the response status.
	_ = err
	require.Nil(t, claims)
	require.Equal(t, 401, ctx.Response.StatusCode)
}

func TestGetSessionID_Unauthorized(t *testing.T) {
	ctx := createTestContext()

	_, _ = GetSessionID(ctx)
	require.Equal(t, 401, ctx.Response.StatusCode)
}

func TestGetUsername_Unauthorized(t *testing.T) {
	ctx := createTestContext()

	_, _ = GetUsername(ctx)
	require.Equal(t, 401, ctx.Response.StatusCode)
}

func TestOptionalAuth_NoTokenDoesNotRequireAuthService(t *testing.T) {
	svc := &LiftAuthService{}
	ctx := createTestContext()

	mw := svc.OptionalAuth()
	called := false
	handler := mw(lift.HandlerFunc(func(_ *lift.Context) error {
		called = true
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	require.True(t, called)
}

func TestRequireAuth_MissingTokenDoesNotRequireAuthService(t *testing.T) {
	svc := &LiftAuthService{}
	ctx := createTestContext()

	mw := svc.RequireAuth()
	handler := mw(lift.HandlerFunc(func(*lift.Context) error {
		t.Fatalf("next handler should not be called")
		return nil
	}))

	_ = handler.Handle(ctx)
	require.Equal(t, 401, ctx.Response.StatusCode)
}

func TestRequireScope_Branches(t *testing.T) {
	svc := &LiftAuthService{}
	ctx := createTestContext()

	mw := svc.RequireScope("read")
	handler := mw(lift.HandlerFunc(func(*lift.Context) error { return nil }))

	err := handler.Handle(ctx)
	require.Error(t, err)
	require.Equal(t, 403, ctx.Response.StatusCode)

	ctx2 := createTestContext()
	ctx2.Set("claims", &auth.EnhancedClaims{Scopes: []string{"write"}})
	err = handler.Handle(ctx2)
	require.Error(t, err)
	require.Equal(t, 403, ctx2.Response.StatusCode)

	ctx3 := createTestContext()
	ctx3.Set("claims", &auth.EnhancedClaims{Scopes: []string{"read"}})
	require.NoError(t, handler.Handle(ctx3))
}

func TestIsAuthenticated_OptionalHelpers(t *testing.T) {
	ctx := createTestContext()
	assert.False(t, IsAuthenticated(ctx))
	assert.Empty(t, GetOptionalUsername(ctx))
	assert.Nil(t, GetOptionalClaims(ctx))

	ctx.Set("username", "alice")
	ctx.Set("claims", &auth.EnhancedClaims{Username: "alice"})
	assert.True(t, IsAuthenticated(ctx))
	assert.Equal(t, "alice", GetOptionalUsername(ctx))
	assert.NotNil(t, GetOptionalClaims(ctx))
}

func TestGetTenantID_ErrorWhenMissing(t *testing.T) {
	ctx := createTestContext()
	_, err := GetTenantID(ctx)
	require.Error(t, err)
}

func TestGetTenantContext_ErrorWhenMissing(t *testing.T) {
	ctx := createTestContext()
	_, err := GetTenantContext(ctx)
	require.Error(t, err)
}

func TestHandleRequest_WithTenantContext(t *testing.T) {
	// Ensures tenant_context is stored and accessible when provided.
	svc := &LiftAuthService{}
	cfg := DefaultTenantIsolationConfig()

	ctx := createTestContext()
	ctx.Request.Headers["X-Tenant-ID"] = "tenant-x"

	handler := svc.RequireTenantWithConfig(cfg)(lift.HandlerFunc(func(ctx *lift.Context) error {
		tenantID, err := GetTenantID(ctx)
		require.NoError(t, err)
		require.Equal(t, "tenant-x", tenantID)
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
}

func TestTenantClaimsResolution_UsesContextAfterMiddleware(t *testing.T) {
	// Validate that once tenant middleware runs, values remain in context.
	svc := &LiftAuthService{}
	cfg := DefaultTenantIsolationConfig()

	ctx := createTestContext()
	ctx.Request.Headers["X-Tenant-ID"] = "tenant-x"

	mw := svc.RequireTenantWithConfig(cfg)
	handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		require.Equal(t, "tenant-x", ctx.Get("tenant_id"))
		require.NotNil(t, ctx.Get("tenant_context"))
		require.Equal(t, cfg, ctx.Get("tenant_config"))
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
}

func TestLiftAuth_ContextValues_AreIsolated(t *testing.T) {
	// Simple smoke test that context setters work without leaking across contexts.
	ctx1 := createTestContext()
	ctx2 := createTestContext()

	ctx1.Set("tenant_id", "t1")
	ctx2.Set("tenant_id", "t2")

	require.Equal(t, "t1", ctx1.Get("tenant_id"))
	require.Equal(t, "t2", ctx2.Get("tenant_id"))
}
