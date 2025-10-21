package lift

import (
	"net"
	"time"

	"github.com/google/uuid"

	"github.com/pay-theory/lift/pkg/security"
)

// SecurityContext wraps a Lift Context with additional security helpers for
// principals, authorization checks, audit logging, and request identity.
type SecurityContext struct {
	*Context
	principal *security.Principal
	requestID string
}

// NewSecurityContext wraps an existing Context with security features and
// generates a per-request ID used for audit trails.
func NewSecurityContext(ctx *Context) *SecurityContext {
	return &SecurityContext{
		Context:   ctx,
		requestID: generateRequestID(),
	}
}

// SetPrincipal attaches the authenticated principal to the context and exposes
// common fields (user_id, tenant_id, roles, etc.) via Context values. It also
// records request tracking fields on the principal.
func (sc *SecurityContext) SetPrincipal(principal *security.Principal) {
	sc.principal = principal

	// Store principal information in context values
	sc.Set("principal", principal)
	sc.Set("user_id", principal.UserID)
	sc.Set("tenant_id", principal.TenantID)
	sc.Set("account_id", principal.AccountID)
	sc.Set("roles", principal.Roles)
	sc.Set("scopes", principal.Scopes)
	sc.Set("auth_method", principal.AuthMethod)

	// Update request tracking
	principal.RequestID = sc.requestID
	principal.IPAddress = sc.GetClientIP()
	principal.UserAgent = sc.GetUserAgent()
}

// GetPrincipal returns the authenticated principal (if any).
func (sc *SecurityContext) GetPrincipal() *security.Principal {
	return sc.principal
}

// RequestID returns the unique security request ID associated with this
// SecurityContext.
func (sc *SecurityContext) RequestID() string {
	return sc.requestID
}

// HasRole reports whether the principal has the specified role.
func (sc *SecurityContext) HasRole(role string) bool {
	if sc.principal == nil {
		return false
	}
	return sc.principal.HasRole(role)
}

// HasPermission reports whether the principal is authorized for the given
// resource and action.
func (sc *SecurityContext) HasPermission(resource, action string) bool {
	if sc.principal == nil {
		return false
	}
	return sc.principal.CanAccessResource(resource, action)
}

// IsAuthenticated reports whether a non-expired principal is attached.
func (sc *SecurityContext) IsAuthenticated() bool {
	return sc.principal != nil && !sc.principal.IsExpired()
}

// GetClientIP extracts the client IP address from the request using standard
// headers and request context.
func (sc *SecurityContext) GetClientIP() string {
	// Use headers directly from the request
	headers := sc.Request.Headers

	// Get request context
	requestContext := sc.Request.RequestContext()

	// Use the security package's IP extraction utility
	ip, err := security.ExtractClientIP(headers, requestContext)
	if err != nil {
		// Log the error for debugging purposes (if logging is available)
		// For now, return "unknown" to maintain backward compatibility
		return "unknown"
	}

	return ip
}

// GetUserAgent returns the User-Agent header value for the request.
func (sc *SecurityContext) GetUserAgent() string {
	return sc.Header("User-Agent")
}

// ValidateIP checks whether the client IP belongs to one of the provided CIDR
// ranges.
func (sc *SecurityContext) ValidateIP(allowedCIDRs []string) bool {
	clientIP := sc.GetClientIP()
	if clientIP == "unknown" {
		return false
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	for _, cidr := range allowedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}

		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// ValidateTenant reports whether the request is scoped to the expected tenant
// identifier.
func (sc *SecurityContext) ValidateTenant(expectedTenantID string) bool {
	currentTenantID := sc.TenantID()
	if currentTenantID == "" {
		return false
	}

	return currentTenantID == expectedTenantID
}

// ToAuditMap returns a map of request and principal fields suitable for audit
// logging.
func (sc *SecurityContext) ToAuditMap() map[string]any {
	auditData := map[string]any{
		"request_id":  sc.requestID,
		"method":      sc.Request.Method,
		"path":        sc.Request.Path,
		"status_code": sc.Response.StatusCode,
		"elapsed_ms":  sc.Duration().Milliseconds(),
		"client_ip":   sc.GetClientIP(),
		"user_agent":  sc.GetUserAgent(),
		"tenant_id":   sc.TenantID(),
		"user_id":     sc.UserID(),
		"timestamp":   time.Now().Unix(),
	}

	// Add principal information if available
	if sc.principal != nil {
		for k, v := range sc.principal.ToAuditMap() {
			auditData[k] = v
		}
	}

	return auditData
}

// RequireAuthentication returns a LiftError if the request is not
// authenticated.
func (sc *SecurityContext) RequireAuthentication() error {
	if !sc.IsAuthenticated() {
		return NewLiftError("UNAUTHORIZED", "Authentication required", 401)
	}
	return nil
}

// RequireRole returns a LiftError if the principal lacks the specified role.
func (sc *SecurityContext) RequireRole(role string) error {
	if err := sc.RequireAuthentication(); err != nil {
		return err
	}

	if !sc.HasRole(role) {
		return AuthorizationError("Insufficient role permissions")
	}

	return nil
}

// RequirePermission returns a LiftError if the principal is not authorized for
// the given resource action.
func (sc *SecurityContext) RequirePermission(resource, action string) error {
	if err := sc.RequireAuthentication(); err != nil {
		return err
	}

	if !sc.HasPermission(resource, action) {
		return AuthorizationError("Insufficient permissions")
	}

	return nil
}

// RequireTenant returns a LiftError if the principal is not scoped to the
// expected tenant.
func (sc *SecurityContext) RequireTenant(expectedTenantID string) error {
	if err := sc.RequireAuthentication(); err != nil {
		return err
	}

	if !sc.ValidateTenant(expectedTenantID) {
		return AuthorizationError("Invalid tenant access")
	}

	return nil
}

// OverrideUserID returns the user ID from the principal if available, otherwise from request
func (sc *SecurityContext) UserID() string {
	if sc.principal != nil {
		return sc.principal.UserID
	}
	return sc.Context.UserID()
}

// OverrideTenantID returns the tenant ID from the principal if available, otherwise from request
func (sc *SecurityContext) TenantID() string {
	if sc.principal != nil {
		return sc.principal.TenantID
	}
	return sc.Context.TenantID()
}

// OverrideAccountID returns the account ID from the principal if available
func (sc *SecurityContext) AccountID() string {
	if sc.principal != nil {
		return sc.principal.AccountID
	}
	return sc.Context.AccountID()
}

// generateRequestID creates a unique request ID
func generateRequestID() string {
	return uuid.New().String()
}

// WithSecurity converts a regular Context to a SecurityContext
func WithSecurity(ctx *Context) *SecurityContext {
	return NewSecurityContext(ctx)
}
