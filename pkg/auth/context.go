// Package auth provides universal authentication context and utilities
// for consolidating authentication patterns across the Lesser application.
package auth

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
)

// Context represents the authenticated user context
type Context struct {
	UserID        string    `json:"user_id"`
	Username      string    `json:"username"`
	Scopes        []string  `json:"scopes"`
	TokenType     string    `json:"token_type"`
	IssuedAt      time.Time `json:"issued_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	ClientID      string    `json:"client_id"`
	Authenticated bool      `json:"authenticated"`
}

// AuthContextKey is the key used to store AuthContext in lift.Context
const AuthContextKey = "auth_context"

// GetAuthContext retrieves authentication context from lift.Context
func GetAuthContext(ctx *lift.Context) *Context {
	if auth := ctx.Get(AuthContextKey); auth != nil {
		if authCtx, ok := auth.(*Context); ok {
			return authCtx
		}
	}
	return &Context{Authenticated: false}
}

// SetAuthContext stores authentication context in lift.Context
func SetAuthContext(ctx *lift.Context, authCtx *Context) {
	ctx.Set(AuthContextKey, authCtx)
}

// RequireAuth returns error if not authenticated
func (ac *Context) RequireAuth() error {
	if !ac.Authenticated {
		return fmt.Errorf("authentication required")
	}
	return nil
}

// RequireScope returns error if scope missing
func (ac *Context) RequireScope(scope string) error {
	if err := ac.RequireAuth(); err != nil {
		return err
	}

	for _, s := range ac.Scopes {
		if s == scope || s == "admin" {
			return nil
		}
	}
	return fmt.Errorf("insufficient scope: requires %s", scope)
}

// HasScope checks if the user has a specific scope
func (ac *Context) HasScope(scope string) bool {
	if !ac.Authenticated {
		return false
	}

	for _, s := range ac.Scopes {
		if s == scope || s == "admin" {
			return true
		}
	}
	return false
}

// IsAdmin checks if the user has admin privileges
func (ac *Context) IsAdmin() bool {
	return ac.HasScope("admin")
}

// RequireAuthWithResponse returns lift response error if not authenticated
func (ac *Context) RequireAuthWithResponse(ctx *lift.Context) error {
	if !ac.Authenticated {
		return common.RespondUnauthorized(ctx)
	}
	return nil
}

// RequireScopeWithResponse returns lift response error if scope missing
func (ac *Context) RequireScopeWithResponse(ctx *lift.Context, scope string) error {
	if err := ac.RequireAuthWithResponse(ctx); err != nil {
		return err
	}

	if !ac.HasScope(scope) {
		return common.RespondInsufficientScope(ctx)
	}
	return nil
}

// CreateAuthContext creates a new authentication context from OAuth claims
func CreateAuthContext(username, clientID, tokenType string, scopes []string, issuedAt, expiresAt time.Time) *Context {
	return &Context{
		UserID:        username, // In Lesser, username is the user ID
		Username:      username,
		Scopes:        scopes,
		TokenType:     tokenType,
		IssuedAt:      issuedAt,
		ExpiresAt:     expiresAt,
		ClientID:      clientID,
		Authenticated: true,
	}
}

// CreateUnauthenticatedContext creates an unauthenticated context
func CreateUnauthenticatedContext() *Context {
	return &Context{
		Authenticated: false,
	}
}

// IsExpired checks if the authentication token has expired
func (ac *Context) IsExpired() bool {
	if !ac.Authenticated {
		return true
	}
	return time.Now().After(ac.ExpiresAt)
}

// TimeUntilExpiry returns the duration until the token expires
func (ac *Context) TimeUntilExpiry() time.Duration {
	if !ac.Authenticated {
		return 0
	}
	return time.Until(ac.ExpiresAt)
}

// ToMap converts the auth context to a map for logging or debugging
func (ac *Context) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"authenticated": ac.Authenticated,
		"username":      ac.Username,
		"scopes":        ac.Scopes,
		"token_type":    ac.TokenType,
		"client_id":     ac.ClientID,
		"expires_at":    ac.ExpiresAt,
		"is_expired":    ac.IsExpired(),
	}
}
