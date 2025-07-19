package lift

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v4"
)

// TenantConfig represents the configuration for tenant middleware
type TenantConfig struct {
	// Required tenant ID in JWT claims
	RequireTenantID bool

	// JWT claim key for tenant ID (default: "tenant_id")
	TenantIDClaim string

	// Header name for tenant ID (default: "X-Tenant-ID")
	TenantIDHeader string

	// Query parameter name for tenant ID (default: "tenant_id")
	TenantIDQuery string

	// Allow tenant ID from header
	AllowHeaderTenant bool

	// Allow tenant ID from query parameter
	AllowQueryTenant bool

	// Validate tenant ID function
	ValidateTenantID func(tenantID string) bool
}

// DefaultTenantConfig returns the default tenant configuration
func DefaultTenantConfig() TenantConfig {
	return TenantConfig{
		RequireTenantID:   true,
		TenantIDClaim:     "tenant_id",
		TenantIDHeader:    "X-Tenant-ID",
		TenantIDQuery:     "tenant_id",
		AllowHeaderTenant: true,
		AllowQueryTenant:  false,
		ValidateTenantID:  nil,
	}
}

// TenantMiddleware creates a middleware that extracts tenant ID from JWT, header, or query parameter
func TenantMiddleware(config TenantConfig) Middleware {
	// Use default config if not provided
	if config.TenantIDClaim == "" {
		config.TenantIDClaim = "tenant_id"
	}
	if config.TenantIDHeader == "" {
		config.TenantIDHeader = "X-Tenant-ID"
	}
	if config.TenantIDQuery == "" {
		config.TenantIDQuery = "tenant_id"
	}

	return func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			// Try to get tenant ID from JWT claims
			tenantID := getTenantIDFromClaims(ctx, config.TenantIDClaim)

			// If not found in JWT, try header
			if tenantID == "" && config.AllowHeaderTenant {
				tenantID = ctx.Header(config.TenantIDHeader)
			}

			// If not found in header, try query parameter
			if tenantID == "" && config.AllowQueryTenant {
				tenantID = ctx.Query(config.TenantIDQuery)
			}

			// Validate tenant ID if required
			if config.RequireTenantID && tenantID == "" {
				return Unauthorized("Tenant ID is required")
			}

			// Validate tenant ID if validation function is provided
			if tenantID != "" && config.ValidateTenantID != nil {
				if !config.ValidateTenantID(tenantID) {
					return Unauthorized("Invalid tenant ID")
				}
			}

			// Set tenant ID in context
			ctx.SetTenantID(tenantID)

			// Call next handler
			return next.Handle(ctx)
		})
	}
}

// getTenantIDFromClaims gets the tenant ID from JWT claims
func getTenantIDFromClaims(ctx *Context, claimKey string) string {
	// Get JWT claims from context
	claims, ok := ctx.Get("jwt_claims").(jwt.MapClaims)
	if !ok {
		return ""
	}

	// Get tenant ID from claims
	tenantID, ok := claims[claimKey].(string)
	if !ok {
		return ""
	}

	return tenantID
}

// TenantRequired is a middleware that requires a tenant ID
func TenantRequired() Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			// Check if tenant ID is set
			tenantID := ctx.TenantID()
			if tenantID == "" {
				return Unauthorized("Tenant ID is required")
			}

			// Call next handler
			return next.Handle(ctx)
		})
	}
}

// ValidateTenantAccess validates that the tenant ID matches the resource tenant ID
func ValidateTenantAccess(resourceTenantID string, ctx *Context) error {
	// Get tenant ID from context
	tenantID := ctx.TenantID()
	if tenantID == "" {
		return Unauthorized("Tenant ID is required")
	}

	// Check if tenant ID matches resource tenant ID
	if tenantID != resourceTenantID {
		return Forbidden(fmt.Sprintf("Access denied to resource in tenant %s", resourceTenantID))
	}

	return nil
}

// TenantIsolationKey creates a tenant-isolated key
func TenantIsolationKey(tenantID, resourceType, resourceID string) string {
	return fmt.Sprintf("tenant#%s#%s#%s", tenantID, resourceType, resourceID)
}

// ParseTenantIsolationKey parses a tenant-isolated key
func ParseTenantIsolationKey(key string) (tenantID, resourceType, resourceID string) {
	parts := strings.Split(key, "#")
	if len(parts) != 4 || parts[0] != "tenant" {
		return "", "", ""
	}

	return parts[1], parts[2], parts[3]
}
