package lift

import (
	"testing"

	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
)

func TestResolveTenantID_Strategies(t *testing.T) {
	ctx := createTestContextWithHeaders(map[string]string{"X-Tenant-ID": "t1"})
	tenantID, err := resolveTenantID(ctx)
	require.NoError(t, err)
	require.Equal(t, "t1", tenantID)

	ctx = createTestContextWithHeaders(map[string]string{"Host": "tenant1.lesser.app"})
	tenantID, err = resolveTenantID(ctx)
	require.NoError(t, err)
	require.Equal(t, "tenant1", tenantID)

	ctx = createTestContextWithPath("/tenant/pathtenant/api/v1/users")
	tenantID, err = resolveTenantID(ctx)
	require.NoError(t, err)
	require.Equal(t, "pathtenant", tenantID)

	ctx = createTestContext()
	ctx.Set("tenant_id", "fromctx")
	tenantID, err = resolveTenantID(ctx)
	require.NoError(t, err)
	require.Equal(t, "fromctx", tenantID)

	_, err = resolveTenantID(createTestContext())
	require.Error(t, err)
}

func TestTenantExtractionHelpers(t *testing.T) {
	require.Equal(t, "tenant1", extractTenantFromSubdomainMiddleware("tenant1.lesser.app"))
	require.Equal(t, "", extractTenantFromSubdomainMiddleware("api.lesser.app"))
	require.Equal(t, "", extractTenantFromSubdomainMiddleware("lesser.app"))

	require.Equal(t, "tenant1", extractTenantFromPathMiddleware("/tenant/tenant1/api"))
	require.Equal(t, "", extractTenantFromPathMiddleware("/api/v1"))
}

func TestTenantMiddleware_SetsTenantID(t *testing.T) {
	mw := TenantMiddleware()

	ctx := createTestContextWithHeaders(map[string]string{"X-Tenant-ID": "t1"})
	ctx.Logger = &liftPkg.NoOpLogger{}

	nextCalled := false
	handler := mw(liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
		nextCalled = true
		require.Equal(t, "t1", ctx.Get("tenant_id"))
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	require.True(t, nextCalled)
}

func TestRequireTenant_Middleware(t *testing.T) {
	mw := RequireTenant()

	ctx := createTestContext()
	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error { return nil }))
	err := handler.Handle(ctx)
	require.Error(t, err)

	ctx = createTestContext()
	ctx.Set("tenant_id", "t1")
	handler = mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error { return nil }))
	require.NoError(t, handler.Handle(ctx))
}

func TestTenantKeyHelpers(t *testing.T) {
	require.Equal(t, "k", TenantPrefix("", "k"))
	require.Equal(t, "tenant#t1", TenantPrefix("t1", "k"))
	require.Equal(t, "user#1", TenantSortKey("user", "1"))
}

func TestTenantIDHelpers(t *testing.T) {
	ctx := createTestContext()
	_, err := GetTenantIDFromMiddleware(ctx)
	require.Error(t, err)

	ctx.Set("tenant_id", "t1")
	tenant, err := GetTenantIDFromMiddleware(ctx)
	require.NoError(t, err)
	require.Equal(t, "t1", tenant)
	require.Equal(t, "t1", GetTenantIDOrDefault(ctx, "default"))
	require.Equal(t, "default", GetTenantIDOrDefault(createTestContext(), "default"))
}
