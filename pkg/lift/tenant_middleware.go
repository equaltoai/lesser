package lift

import (
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
)

// TenantMiddleware creates middleware for multi-tenant support following Lift patterns
func TenantMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract tenant ID using multiple strategies
			tenantID, err := resolveTenantID(ctx)
			if err != nil {
				return lift.NewLiftError("TENANT_REQUIRED", "Tenant context required", 401)
			}

			// Set tenant ID in context (Lift provides this method)
			ctx.Set("tenant_id", tenantID)

			// Log tenant access for monitoring
			if logger := ctx.Logger; logger != nil {
				logger.WithField("tenant_id", tenantID).Info("Tenant access")
			}

			return next.Handle(ctx)
		})
	}
}

// resolveTenantID resolves tenant ID using multiple strategies
func resolveTenantID(ctx *lift.Context) (string, error) {
	// Strategy 1: X-Tenant-ID header
	if tenantID := ctx.Header("X-Tenant-ID"); tenantID != "" {
		return tenantID, nil
	}

	// Strategy 2: Subdomain extraction
	if host := ctx.Header("Host"); host != "" {
		if tenantID := extractTenantFromSubdomainMiddleware(host); tenantID != "" {
			return tenantID, nil
		}
	}

	// Strategy 3: Path-based tenant (e.g., /tenant/{tenant_id}/...)
	if tenantID := extractTenantFromPathMiddleware(ctx.Request.Path); tenantID != "" {
		return tenantID, nil
	}

	// Strategy 4: Use existing tenant_id from context if available
	if tenantID, ok := ctx.Get("tenant_id").(string); ok && tenantID != "" {
		return tenantID, nil
	}

	return "", errors.New("tenant context required")
}

// extractTenantFromSubdomainMiddleware extracts tenant ID from subdomain (middleware version)
// Example: tenant1.lesser.app -> tenant1
func extractTenantFromSubdomainMiddleware(host string) string {
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

// extractTenantFromPathMiddleware extracts tenant ID from URL path (middleware version)
// Example: /tenant/tenant1/api/... -> tenant1
func extractTenantFromPathMiddleware(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "tenant" {
		return parts[1]
	}
	return ""
}

// GetTenantIDFromMiddleware safely gets tenant ID from Lift context
func GetTenantIDFromMiddleware(ctx *lift.Context) (string, error) {
	tenantID, ok := ctx.Get("tenant_id").(string)
	if !ok || tenantID == "" {
		return "", errors.New("tenant context required")
	}
	return tenantID, nil
}

// GetTenantIDOrDefault gets tenant ID or returns default value
func GetTenantIDOrDefault(ctx *lift.Context, defaultTenantID string) string {
	tenantID, err := GetTenantIDFromMiddleware(ctx)
	if err != nil {
		return defaultTenantID
	}
	return tenantID
}

// TenantPrefix creates a tenant-prefixed key for DynamoDB isolation
func TenantPrefix(tenantID, key string) string {
	if err := common.ValidateRequiredParam("tenantID", tenantID); err != nil {
		return key
	}
	return fmt.Sprintf("tenant#%s", tenantID)
}

// TenantSortKey creates a tenant-aware sort key
func TenantSortKey(entityType, entityID string) string {
	return fmt.Sprintf("%s#%s", entityType, entityID)
}

// RequireTenant is a middleware that enforces tenant context
func RequireTenant() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			_, err := GetTenantIDFromMiddleware(ctx)
			if err != nil {
				return lift.NewLiftError("UNAUTHORIZED", "Tenant context required", 401)
			}
			return next.Handle(ctx)
		})
	}
}
