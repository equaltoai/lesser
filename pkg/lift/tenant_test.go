package lift

import (
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantContext(t *testing.T) {
	t.Run("PrefixKey", func(t *testing.T) {
		tc := &TenantContext{TenantID: "tenant123"}

		// Test prefixing
		assert.Equal(t, "tenant#tenant123#user:1", tc.PrefixKey("user:1"))
		assert.Equal(t, "tenant#tenant123#", tc.PrefixKey(""))

		// Test with empty tenant
		emptyTC := &TenantContext{TenantID: ""}
		assert.Equal(t, "user:1", emptyTC.PrefixKey("user:1"))
	})

	t.Run("PrefixPK", func(t *testing.T) {
		tc := &TenantContext{TenantID: "tenant123"}

		// Test prefixing
		assert.Equal(t, "tenant#tenant123#USER#1", tc.PrefixPK("USER#1"))

		// Test avoiding double prefix
		already := "tenant#tenant123#USER#1"
		assert.Equal(t, already, tc.PrefixPK(already))
	})

	t.Run("StripPrefix", func(t *testing.T) {
		tc := &TenantContext{TenantID: "tenant123"}

		// Test stripping
		assert.Equal(t, "user:1", tc.StripPrefix("tenant#tenant123#user:1"))
		assert.Equal(t, "user:1", tc.StripPrefix("user:1"))

		// Test with empty tenant
		emptyTC := &TenantContext{TenantID: ""}
		assert.Equal(t, "user:1", emptyTC.StripPrefix("user:1"))
	})

	t.Run("ValidateAccess", func(t *testing.T) {
		tc := &TenantContext{TenantID: "tenant123"}

		// Valid access
		err := tc.ValidateAccess("tenant#tenant123#user:1")
		assert.NoError(t, err)

		// Invalid access
		err = tc.ValidateAccess("tenant#other#user:1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cross-tenant access attempt")

		// No prefix
		err = tc.ValidateAccess("user:1")
		assert.Error(t, err)

		// Empty tenant allows all
		emptyTC := &TenantContext{TenantID: ""}
		err = emptyTC.ValidateAccess("anything")
		assert.NoError(t, err)
	})
}

func TestTenantIsolationConfig(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		config := DefaultTenantIsolationConfig()

		assert.True(t, config.EnableStrict)
		assert.False(t, config.AllowCrossTenantRead)
		assert.Equal(t, "", config.DefaultTenantID)
		assert.Contains(t, config.SharedResources, "system")
		assert.Contains(t, config.SharedResources, "config")
	})

	t.Run("IsSharedResource", func(t *testing.T) {
		config := &TenantIsolationConfig{
			SharedResources: []string{"system", "config", "public"},
		}

		assert.True(t, config.IsSharedResource("system"))
		assert.True(t, config.IsSharedResource("config"))
		assert.True(t, config.IsSharedResource("public"))
		assert.False(t, config.IsSharedResource("user"))
		assert.False(t, config.IsSharedResource("private"))
	})
}

func TestResolveTenant(t *testing.T) {
	t.Run("HeaderStrategy", func(t *testing.T) {
		ctx := &lift.Context{
			Request: &lift.Request{
				Headers: map[string]string{
					"X-Tenant-ID": "tenant-from-header",
				},
			},
		}

		tenantID, err := resolveTenantFromContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, "tenant-from-header", tenantID)
	})

	t.Run("SubdomainStrategy", func(t *testing.T) {
		tests := []struct {
			host     string
			expected string
			hasError bool
		}{
			{"tenant1.lesser.app", "tenant1", false},
			{"tenant-2.example.com", "tenant-2", false},
			{"www.lesser.app", "", true},
			{"api.lesser.app", "", true},
			{"lesser.app", "", true},
			{"localhost", "", true},
		}

		for _, tt := range tests {
			t.Run(tt.host, func(t *testing.T) {
				ctx := &lift.Context{
					Request: &lift.Request{
						Headers: map[string]string{
							"Host": tt.host,
						},
					},
				}

				tenantID, err := resolveTenantFromContext(ctx)
				if tt.hasError {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.expected, tenantID)
				}
			})
		}
	})

	t.Run("PathStrategy", func(t *testing.T) {
		tests := []struct {
			path     string
			expected string
			hasError bool
		}{
			{"/tenant/tenant1/api/users", "tenant1", false},
			{"/tenant/my-tenant/data", "my-tenant", false},
			{"/api/users", "", true},
			{"/tenant/", "", true},
			{"/", "", true},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				ctx := &lift.Context{
					Request: &lift.Request{
						Path:    tt.path,
						Headers: map[string]string{},
					},
				}

				tenantID, err := resolveTenantFromContext(ctx)
				if tt.hasError {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.expected, tenantID)
				}
			})
		}
	})

	t.Run("PriorityOrder", func(t *testing.T) {
		// Header takes priority over subdomain
		ctx := &lift.Context{
			Request: &lift.Request{
				Headers: map[string]string{
					"X-Tenant-ID": "header-tenant",
					"Host":        "subdomain-tenant.lesser.app",
				},
				Path: "/tenant/path-tenant/api",
			},
		}

		tenantID, err := resolveTenantFromContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, "header-tenant", tenantID)
	})
}

// Note: TestExtractBearerToken and TestGetClientIP are already defined in other test files
