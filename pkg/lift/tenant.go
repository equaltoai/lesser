package lift

import (
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
)

// TenantContext provides tenant-aware utilities for data isolation
type TenantContext struct {
	TenantID string
}

// NewTenantContext creates a new tenant context from Lift context
func NewTenantContext(ctx *lift.Context) (*TenantContext, error) {
	tenantID, err := getTenantIDFromContextLocal(ctx)
	if err != nil {
		return nil, err
	}
	return &TenantContext{TenantID: tenantID}, nil
}

// GetTenantContextOrDefault gets tenant context or returns a default
func GetTenantContextOrDefault(ctx *lift.Context, defaultTenantID string) *TenantContext {
	tenantID, err := getTenantIDFromContextLocal(ctx)
	if err != nil {
		tenantID = defaultTenantID
	}
	return &TenantContext{TenantID: tenantID}
}

// getTenantIDFromContextLocal is a local helper to get tenant ID
func getTenantIDFromContextLocal(ctx *lift.Context) (string, error) {
	tenantID, ok := ctx.Get("tenant_id").(string)
	if !ok {
		return "", errors.New("tenant context required")
	}
	if err := common.ValidateRequiredParam("tenantID", tenantID); err != nil {
		return "", errors.New("tenant context required")
	}
	return tenantID, nil
}

// PrefixKey adds tenant prefix to DynamoDB keys for isolation
func (tc *TenantContext) PrefixKey(key string) string {
	if err := common.ValidateRequiredParam("tenantID", tc.TenantID); err != nil {
		return key
	}
	return fmt.Sprintf("tenant#%s#%s", tc.TenantID, key)
}

// PrefixPK adds tenant prefix to DynamoDB partition key
func (tc *TenantContext) PrefixPK(pk string) string {
	if err := common.ValidateRequiredParam("tenantID", tc.TenantID); err != nil {
		return pk
	}
	// Check if already prefixed to avoid double prefixing
	prefix := fmt.Sprintf("tenant#%s#", tc.TenantID)
	if strings.HasPrefix(pk, prefix) {
		return pk
	}
	return prefix + pk
}

// StripPrefix removes tenant prefix from keys for client responses
func (tc *TenantContext) StripPrefix(key string) string {
	if err := common.ValidateRequiredParam("tenantID", tc.TenantID); err != nil {
		return key
	}
	prefix := fmt.Sprintf("tenant#%s#", tc.TenantID)
	return strings.TrimPrefix(key, prefix)
}

// ValidateAccess checks if a key belongs to the current tenant
func (tc *TenantContext) ValidateAccess(key string) error {
	if err := common.ValidateRequiredParam("tenantID", tc.TenantID); err != nil {
		return nil // No tenant context means no validation
	}
	prefix := fmt.Sprintf("tenant#%s#", tc.TenantID)
	if !strings.HasPrefix(key, prefix) {
		return errors.New("access denied: cross-tenant access attempt")
	}
	return nil
}

// TenantIsolationConfig provides configuration for tenant isolation
type TenantIsolationConfig struct {
	// EnableStrict enforces tenant isolation on all operations
	EnableStrict bool

	// AllowCrossTenantRead allows read operations across tenants (for admin use)
	AllowCrossTenantRead bool

	// SharedResources lists resource types that are shared across tenants
	SharedResources []string

	// DefaultTenantID is used when no tenant context is available
	DefaultTenantID string
}

// DefaultTenantIsolationConfig returns default tenant isolation settings
func DefaultTenantIsolationConfig() *TenantIsolationConfig {
	return &TenantIsolationConfig{
		EnableStrict:         true,
		AllowCrossTenantRead: false,
		SharedResources:      []string{"system", "config"},
		DefaultTenantID:      "", // No default tenant - strict mode
	}
}

// IsSharedResource checks if a resource type is shared across tenants
func (tic *TenantIsolationConfig) IsSharedResource(resourceType string) bool {
	for _, shared := range tic.SharedResources {
		if shared == resourceType {
			return true
		}
	}
	return false
}

// TenantAwareMiddleware adds tenant isolation to data operations
func TenantAwareMiddleware(config *TenantIsolationConfig) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Skip tenant isolation if not enabled
			if !config.EnableStrict {
				return next.Handle(ctx)
			}

			// Try to resolve tenant, use default if not found
			tenantID, err := resolveTenantFromContext(ctx)
			if err != nil && config.DefaultTenantID != "" {
				tenantID = config.DefaultTenantID
			} else if err != nil {
				return ctx.Forbidden("Tenant context required", err)
			}

			// Store tenant ID and config in context
			ctx.Set("tenant_id", tenantID)
			ctx.Set("tenant_config", config)

			return next.Handle(ctx)
		})
	}
}

// GetTenantConfig retrieves tenant configuration from context
func GetTenantConfig(ctx *lift.Context) *TenantIsolationConfig {
	config, ok := ctx.Get("tenant_config").(*TenantIsolationConfig)
	if !ok {
		return DefaultTenantIsolationConfig()
	}
	return config
}

// WithTenantContext creates a new context with tenant information
func WithTenantContext(ctx *lift.Context, tenantID string) *lift.Context {
	ctx.Set("tenant_id", tenantID)
	return ctx
}

// resolveTenantFromContext resolves tenant ID using multiple strategies (renamed to avoid conflicts)
func resolveTenantFromContext(ctx *lift.Context) (string, error) {
	// Strategy 1: X-Tenant-ID header
	if tenantID := ctx.Header("X-Tenant-ID"); tenantID != "" {
		return tenantID, nil
	}

	// Strategy 2: Subdomain extraction
	if host := ctx.Header("Host"); host != "" {
		if tenantID := extractTenantFromSubdomainLocal(host); tenantID != "" {
			return tenantID, nil
		}
	}

	// Strategy 3: Path-based tenant (e.g., /tenant/{tenant_id}/...)
	if tenantID := extractTenantFromPathLocal(ctx.Request.Path); tenantID != "" {
		return tenantID, nil
	}

	// Strategy 4: Use existing tenant_id from context if available
	if tenantID, ok := ctx.Get("tenant_id").(string); ok && tenantID != "" {
		return tenantID, nil
	}

	return "", errors.New("tenant context required")
}

// extractTenantFromSubdomainLocal extracts tenant ID from subdomain (local version)
// Example: tenant1.lesser.app -> tenant1
func extractTenantFromSubdomainLocal(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) >= 3 {
		// Check if it's not a reserved subdomain
		subdomain := parts[0]
		reserved := []string{"www", "api", "admin", "mail", "ftp", "cdn"}
		for _, r := range reserved {
			if subdomain == r {
				return ""
			}
		}
		return subdomain
	}
	return ""
}

// extractTenantFromPathLocal extracts tenant ID from URL path (local version)
// Example: /tenant/tenant1/api/... -> tenant1
func extractTenantFromPathLocal(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == PathSegmentTenant {
		return parts[1]
	}
	return ""
}
