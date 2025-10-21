package security

import (
	"time"
)

// Principal represents an authenticated entity (user, service, etc.) with their permissions
type Principal struct {
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	UserID     string    `json:"user_id"`
	TenantID   string    `json:"tenant_id"`
	AccountID  string    `json:"account_id"`
	AuthMethod string    `json:"auth_method"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	SessionID  string    `json:"session_id"`
	RequestID  string    `json:"request_id"`
	Roles      []string  `json:"roles"`
	Scopes     []string  `json:"scopes"`
}

// Permission represents a specific permission in the RBAC system
type Permission struct {
	// Maps (8 bytes) - largest first
	Conditions map[string]any `json:"conditions"` // Dynamic conditions

	// Strings (16 bytes each)
	Resource string `json:"resource"` // "users", "payments", "accounts"
	Action   string `json:"action"`   // "read", "write", "delete"
}

// Role represents a collection of permissions
type Role struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	TenantID    string       `json:"tenant_id"`
	Permissions []Permission `json:"permissions"`
}

// HasRole checks if the principal has a specific role
func (p *Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasScope checks if the principal has a specific scope
func (p *Principal) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the principal has any of the specified roles
func (p *Principal) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if p.HasRole(role) {
			return true
		}
	}
	return false
}

// HasAllRoles checks if the principal has all of the specified roles
func (p *Principal) HasAllRoles(roles ...string) bool {
	for _, role := range roles {
		if !p.HasRole(role) {
			return false
		}
	}
	return true
}

// IsExpired checks if the principal's authentication has expired
func (p *Principal) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

// IsSameTenant checks if the principal belongs to the specified tenant
func (p *Principal) IsSameTenant(tenantID string) bool {
	return p.TenantID == tenantID
}

// IsValidForTenant checks if the principal is valid for operations on the specified tenant
func (p *Principal) IsValidForTenant(tenantID string) bool {
	// Check if expired
	if p.IsExpired() {
		return false
	}

	// Check tenant isolation (strict mode)
	if p.TenantID != tenantID {
		return false
	}

	return true
}

// CanAccessResource checks if the principal can access a specific resource
func (p *Principal) CanAccessResource(resource, _ string) bool {
	// For now, this is a simple role-based check
	// In the future, this will integrate with the RBAC system

	// System administrators can access everything
	if p.HasRole("admin") || p.HasRole("system") {
		return true
	}

	// Resource-specific checks
	switch resource {
	case "health":
		// Health endpoints are public
		return true
	case "users":
		return p.HasRole("user") || p.HasRole("manager")
	case "payments":
		return p.HasRole("payment_processor") || p.HasRole("manager")
	case "accounts":
		return p.HasRole("account_manager") || p.HasRole("manager")
	default:
		// Deny access to unknown resources by default
		return false
	}
}

// ToAuditMap converts the principal to a map for audit logging
func (p *Principal) ToAuditMap() map[string]any {
	return map[string]any{
		"user_id":     p.UserID,
		"tenant_id":   p.TenantID,
		"account_id":  p.AccountID,
		"roles":       p.Roles,
		"scopes":      p.Scopes,
		"auth_method": p.AuthMethod,
		"ip_address":  p.IPAddress,
		"user_agent":  p.UserAgent,
		"session_id":  p.SessionID,
		"request_id":  p.RequestID,
		"issued_at":   p.IssuedAt,
		"expires_at":  p.ExpiresAt,
	}
}

// AnonymousPrincipal creates a principal for unauthenticated requests
func AnonymousPrincipal() *Principal {
	return &Principal{
		UserID:     "anonymous",
		TenantID:   "",
		AccountID:  "",
		Roles:      []string{"anonymous"},
		Scopes:     []string{},
		AuthMethod: "none",
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour), // Short-lived
	}
}

// SystemPrincipal creates a principal for system/internal operations
func SystemPrincipal() *Principal {
	return &Principal{
		UserID:     "system",
		TenantID:   "",
		AccountID:  "",
		Roles:      []string{"system"},
		Scopes:     []string{"*"},
		AuthMethod: "system",
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(24 * time.Hour), // Long-lived
	}
}

// ServicePrincipal creates a principal for service-to-service communication
func ServicePrincipal(serviceID, tenantID string) *Principal {
	return &Principal{
		UserID:     serviceID,
		TenantID:   tenantID,
		AccountID:  "",
		Roles:      []string{"service"},
		Scopes:     []string{"service"},
		AuthMethod: "service",
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
}

// PrincipalBuilder provides a fluent interface for building principals
type PrincipalBuilder struct {
	principal *Principal
}

// NewPrincipalBuilder creates a new principal builder
func NewPrincipalBuilder() *PrincipalBuilder {
	return &PrincipalBuilder{
		principal: &Principal{
			IssuedAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Hour), // Default 1 hour
		},
	}
}

// WithUserID sets the user ID
func (b *PrincipalBuilder) WithUserID(userID string) *PrincipalBuilder {
	b.principal.UserID = userID
	return b
}

// WithTenantID sets the tenant ID
func (b *PrincipalBuilder) WithTenantID(tenantID string) *PrincipalBuilder {
	b.principal.TenantID = tenantID
	return b
}

// WithAccountID sets the account ID
func (b *PrincipalBuilder) WithAccountID(accountID string) *PrincipalBuilder {
	b.principal.AccountID = accountID
	return b
}

// WithRoles sets the roles
func (b *PrincipalBuilder) WithRoles(roles ...string) *PrincipalBuilder {
	b.principal.Roles = roles
	return b
}

// AddRole adds a single role
func (b *PrincipalBuilder) AddRole(role string) *PrincipalBuilder {
	b.principal.Roles = append(b.principal.Roles, role)
	return b
}

// WithScopes sets the scopes
func (b *PrincipalBuilder) WithScopes(scopes ...string) *PrincipalBuilder {
	b.principal.Scopes = scopes
	return b
}

// AddScope adds a single scope
func (b *PrincipalBuilder) AddScope(scope string) *PrincipalBuilder {
	b.principal.Scopes = append(b.principal.Scopes, scope)
	return b
}

// WithAuthMethod sets the authentication method
func (b *PrincipalBuilder) WithAuthMethod(method string) *PrincipalBuilder {
	b.principal.AuthMethod = method
	return b
}

// WithExpiration sets the expiration time
func (b *PrincipalBuilder) WithExpiration(duration time.Duration) *PrincipalBuilder {
	b.principal.ExpiresAt = b.principal.IssuedAt.Add(duration)
	return b
}

// WithRequest sets request-specific information
func (b *PrincipalBuilder) WithRequest(ipAddress, userAgent, requestID string) *PrincipalBuilder {
	b.principal.IPAddress = ipAddress
	b.principal.UserAgent = userAgent
	b.principal.RequestID = requestID
	return b
}

// Build returns the constructed principal
func (b *PrincipalBuilder) Build() *Principal {
	return b.principal
}

// Validate validates the principal
func (b *PrincipalBuilder) Validate() error {
	if b.principal.UserID == "" {
		return NewSecurityError("INVALID_PRINCIPAL", "User ID is required")
	}

	if b.principal.AuthMethod == "" {
		return NewSecurityError("INVALID_PRINCIPAL", "Authentication method is required")
	}

	if b.principal.ExpiresAt.Before(time.Now()) {
		return NewSecurityError("INVALID_PRINCIPAL", "Principal has expired")
	}

	return nil
}
