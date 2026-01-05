package lift

import (
	"errors"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
)

// Constants for common values
const (
	UnknownIP = "unknown"
)

// LiftAuthService provides Lift-native authentication middleware
//
//nolint:revive // Lift prefix clarifies this is Lift framework specific
type LiftAuthService struct {
	authService *auth.AuthService
}

// NewLiftAuthService creates a new Lift-native auth service
func NewLiftAuthService(authService *auth.AuthService) *LiftAuthService {
	return &LiftAuthService{
		authService: authService,
	}
}

// RequireAuth middleware that requires authentication
func (las *LiftAuthService) RequireAuth() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract Bearer token directly from Lift context
			token := extractBearerToken(ctx)
			if err := common.ValidateRequiredParam("token", token); err != nil {
				// Log authentication failure
				common.LogAuthFailure("missing authorization token", "", getClientIP(ctx), ctx.Header("User-Agent"), ctx.GetRequestID())
				return ctx.Unauthorized("Authentication required", nil)
			}

			// Validate token using existing auth service
			claims, err := las.authService.ValidateAccessToken(token)
			if err != nil {
				// Log authentication failure
				common.LogAuthFailure(err.Error(), "", getClientIP(ctx), ctx.Header("User-Agent"), ctx.GetRequestID())
				return ctx.Unauthorized("Invalid token", err)
			}

			// Store enhanced claims in context
			ctx.Set("claims", claims)
			ctx.Set("username", claims.Username)
			ctx.Set("session_id", claims.SessionID)
			ctx.Set("device_id", claims.DeviceID)

			return next.Handle(ctx)
		})
	}
}

// RequireScope middleware that checks for required scopes
func (las *LiftAuthService) RequireScope(scope string) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
			if !ok {
				return ctx.Forbidden("Authentication required", nil)
			}

			if !claims.HasScope(scope) {
				return ctx.Forbidden("Insufficient permissions", nil)
			}

			return next.Handle(ctx)
		})
	}
}

// OptionalAuth middleware that optionally authenticates if token is present
func (las *LiftAuthService) OptionalAuth() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract Bearer token
			token := extractBearerToken(ctx)
			if err := common.ValidateRequiredParam("token", token); err != nil {
				// No token provided, continue without authentication
				return next.Handle(ctx)
			}

			// Validate token if present
			claims, err := las.authService.ValidateAccessToken(token)
			if err != nil {
				// Invalid token, but don't fail - just continue without auth
				return next.Handle(ctx)
			}

			// Store claims in context
			ctx.Set("claims", claims)
			ctx.Set("username", claims.Username)
			ctx.Set("session_id", claims.SessionID)
			ctx.Set("device_id", claims.DeviceID)

			return next.Handle(ctx)
		})
	}
}

// RequireTenant middleware that requires tenant context
func (las *LiftAuthService) RequireTenant() lift.Middleware {
	return las.RequireTenantWithConfig(DefaultTenantIsolationConfig())
}

// RequireTenantWithConfig middleware that requires tenant context with custom config
func (las *LiftAuthService) RequireTenantWithConfig(config *TenantIsolationConfig) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			tenantID, err := resolveTenant(ctx)
			if err != nil {
				// Try default tenant if configured
				if config.DefaultTenantID != "" {
					tenantID = config.DefaultTenantID
				} else {
					return ctx.Forbidden("Tenant context required", err)
				}
			}

			// Store tenant ID and config in context
			ctx.Set("tenant_id", tenantID)
			ctx.Set("tenant_config", config)

			// Create tenant context for easy access
			tenantCtx := &TenantContext{TenantID: tenantID}
			ctx.Set("tenant_context", tenantCtx)

			return next.Handle(ctx)
		})
	}
}

// Tenant resolution strategies

// resolveTenant resolves tenant ID using multiple strategies
func resolveTenant(ctx *lift.Context) (string, error) {
	// Strategy 1: X-Tenant-ID header
	if tenantID := ctx.Header("X-Tenant-ID"); tenantID != "" {
		return tenantID, nil
	}

	// Strategy 2: Subdomain extraction
	if host := ctx.Header("Host"); host != "" {
		if tenantID := extractTenantFromSubdomain(host); tenantID != "" {
			return tenantID, nil
		}
	}

	// Strategy 3: Path-based tenant (e.g., /tenant/{tenant_id}/...)
	if tenantID := extractTenantFromPath(ctx.Request.Path); tenantID != "" {
		return tenantID, nil
	}

	// Strategy 4: JWT claims (if authenticated)
	if claims, ok := ctx.Get("claims").(*auth.EnhancedClaims); ok {
		// Extract tenant from username domain if federated
		// e.g., "user@tenant.example.com" -> "tenant"
		if strings.Contains(claims.Username, "@") {
			parts := strings.Split(claims.Username, "@")
			if len(parts) == 2 {
				domain := parts[1]
				// Extract subdomain as tenant if it's a federated identity
				if tenantID := extractTenantFromSubdomain(domain); tenantID != "" {
					return tenantID, nil
				}
			}
		}

		// For non-federated users, derive tenant from username
		// This could be enhanced with a user->tenant mapping in the future
		return deriveTenantFromUsername(claims.Username), nil
	}

	return "", errors.New("tenant context required")
}

// extractTenantFromSubdomain extracts tenant ID from subdomain
// Example: tenant1.lesser.app -> tenant1
func extractTenantFromSubdomain(host string) string {
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

// extractTenantFromPath extracts tenant ID from URL path
// Example: /tenant/tenant1/api/... -> tenant1
func extractTenantFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "tenant" {
		return parts[1]
	}
	return ""
}

// extractBearerToken extracts Bearer token from Authorization header
func extractBearerToken(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		return ""
	}

	// Check for Bearer token format
	const bearerPrefix = "Bearer "
	if strings.HasPrefix(authHeader, bearerPrefix) {
		return strings.TrimPrefix(authHeader, bearerPrefix)
	}

	return ""
}

// Context helper functions

// GetClaims retrieves enhanced claims from Lift context
func GetClaims(ctx *lift.Context) (*auth.EnhancedClaims, error) {
	claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
	if !ok {
		return nil, ctx.Unauthorized("Authentication required", nil)
	}
	return claims, nil
}

// GetUsername retrieves username from Lift context
func GetUsername(ctx *lift.Context) (string, error) {
	username, ok := ctx.Get("username").(string)
	if !ok || common.ValidateRequiredParam("username", username) != nil {
		return "", ctx.Unauthorized("Authentication required", nil)
	}
	return username, nil
}

// GetSessionID retrieves session ID from Lift context
func GetSessionID(ctx *lift.Context) (string, error) {
	sessionID, ok := ctx.Get("session_id").(string)
	if !ok || common.ValidateRequiredParam("sessionID", sessionID) != nil {
		return "", ctx.Unauthorized("Authentication required", nil)
	}
	return sessionID, nil
}

// GetTenantID retrieves tenant ID from Lift context
func GetTenantID(ctx *lift.Context) (string, error) {
	tenantID, ok := ctx.Get("tenant_id").(string)
	if !ok || common.ValidateRequiredParam("tenantID", tenantID) != nil {
		return "", errors.New("tenant context required")
	}
	return tenantID, nil
}

// GetTenantContext retrieves tenant context from Lift context
func GetTenantContext(ctx *lift.Context) (*TenantContext, error) {
	tenantCtx, ok := ctx.Get("tenant_context").(*TenantContext)
	if !ok {
		// Try to create from tenant_id if available
		tenantID, err := GetTenantID(ctx)
		if err != nil {
			return nil, err
		}
		return &TenantContext{TenantID: tenantID}, nil
	}
	return tenantCtx, nil
}

// GetOptionalUsername retrieves username from context if authenticated
func GetOptionalUsername(ctx *lift.Context) string {
	username, _ := ctx.Get("username").(string)
	return username
}

// GetOptionalClaims retrieves claims from context if authenticated
func GetOptionalClaims(ctx *lift.Context) *auth.EnhancedClaims {
	claims, _ := ctx.Get("claims").(*auth.EnhancedClaims)
	return claims
}

// IsAuthenticated checks if the request is authenticated
func IsAuthenticated(ctx *lift.Context) bool {
	_, ok := ctx.Get("claims").(*auth.EnhancedClaims)
	return ok
}

// HasScope checks if the authenticated user has a specific scope
func HasScope(ctx *lift.Context, scope string) bool {
	claims := GetOptionalClaims(ctx)
	if claims == nil {
		return false
	}
	return claims.HasScope(scope)
}

// getClientIP extracts the client IP from the request
func getClientIP(ctx *lift.Context) string {
	// Check X-Forwarded-For header first (for requests through load balancers)
	if xff := ctx.Header("X-Forwarded-For"); xff != "" {
		// Take the first IP in the comma-separated list
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := ctx.Header("X-Real-IP"); xri != "" {
		return xri
	}

	return UnknownIP
}

// deriveTenantFromUsername derives a tenant ID from a username
// This implements a default mapping strategy that can be customized
func deriveTenantFromUsername(username string) string {
	// Strategy 1: If username contains a tenant prefix (e.g., "tenant1_user")
	if strings.Contains(username, "_") {
		parts := strings.SplitN(username, "_", 2)
		if len(parts) == 2 && isValidTenantID(parts[0]) {
			return parts[0]
		}
	}

	// Strategy 2: Hash-based partitioning for multi-tenant scenarios
	// This provides consistent tenant assignment based on username
	if len(username) > 0 {
		// Use first character for simple partitioning (0-9, a-z)
		firstChar := strings.ToLower(string(username[0]))
		if firstChar >= "0" && firstChar <= "9" {
			return "shard" + firstChar
		} else if firstChar >= "a" && firstChar <= "z" {
			return "shard" + firstChar
		}
	}

	// Strategy 3: Default tenant for single-tenant deployments
	return "default"
}

// isValidTenantID checks if a string is a valid tenant identifier
func isValidTenantID(tenantID string) bool {
	if len(tenantID) == 0 || len(tenantID) > 64 {
		return false
	}

	// Tenant IDs should be alphanumeric with hyphens
	for _, r := range tenantID {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}

	return true
}
