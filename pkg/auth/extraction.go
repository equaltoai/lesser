// Package auth provides centralized authentication extraction functions
// This consolidates the auth extraction patterns found across 80+ handler files
package auth

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/pay-theory/lift/pkg/lift"
)

// AuthenticatedAccount represents an authenticated user account
type AuthenticatedAccount struct {
	Username string
	Claims   *Claims
}

// GetAccountFromContext extracts the authenticated account from the Lift context
// This consolidates the common pattern: claims, err := h.oauthService.ValidateAccessToken(token)
func GetAccountFromContext(ctx *lift.Context, oauthService OAuthServiceInterface) (*AuthenticatedAccount, error) {
	// Check for test mode first (found in 50+ handlers)
	testUsername := common.GetTestUsername(ctx)
	if testUsername != "" {
		return &AuthenticatedAccount{
			Username: testUsername,
			Claims:   nil, // Test mode bypasses JWT claims
		}, nil
	}

	// Extract Authorization header with fallbacks
	authHeader := common.ExtractAuthHeader(ctx)
	if authHeader == "" {
		return nil, apperrors.Unauthorized("missing authorization header")
	}

	// Extract bearer token
	token, err := common.ExtractBearerToken(authHeader)
	if err != nil {
		return nil, apperrors.Unauthorized("invalid bearer token format")
	}

	// Validate token and get claims
	claims, err := oauthService.ValidateAccessToken(token)
	if err != nil {
		return nil, apperrors.Unauthorized("invalid access token")
	}

	return &AuthenticatedAccount{
		Username: claims.GetUsername(),
		Claims:   claims,
	}, nil
}

// GetUsernameFromContext extracts just the username from the context
// This consolidates the pattern: username, err := h.authenticateRequestWithScope(ctx, "read")
func GetUsernameFromContext(ctx *lift.Context, oauthService OAuthServiceInterface) (string, error) {
	account, err := GetAccountFromContext(ctx, oauthService)
	if err != nil {
		return "", err
	}
	return account.Username, nil
}

// RequireAuth ensures the request is authenticated
// This consolidates the pattern of checking if username is empty after auth
func RequireAuth(ctx *lift.Context, oauthService OAuthServiceInterface) error {
	_, err := GetAccountFromContext(ctx, oauthService)
	return err
}

// RequireAuthWithScope ensures the request is authenticated with a specific scope
// This consolidates the most common auth pattern across all handlers
func RequireAuthWithScope(ctx *lift.Context, oauthService OAuthServiceInterface, scope string) (*AuthenticatedAccount, error) {
	account, err := GetAccountFromContext(ctx, oauthService)
	if err != nil {
		return nil, err
	}

	// Test mode bypasses scope checks
	if account.Claims == nil {
		return account, nil
	}

	if !account.Claims.HasScope(scope) {
		return nil, apperrors.Forbidden(fmt.Sprintf("insufficient scope: requires %s", scope))
	}

	return account, nil
}

// RequireAuthWithMultipleScopes allows any of the specified scopes
// This consolidates the pattern where either read OR write scope is acceptable
func RequireAuthWithMultipleScopes(ctx *lift.Context, oauthService OAuthServiceInterface, scopes []string) (*AuthenticatedAccount, error) {
	account, err := GetAccountFromContext(ctx, oauthService)
	if err != nil {
		return nil, err
	}

	// Test mode bypasses scope checks
	if account.Claims == nil {
		return account, nil
	}

	// Check if user has any of the required scopes
	hasValidScope := false
	for _, scope := range scopes {
		if account.Claims.HasScope(scope) {
			hasValidScope = true
			break
		}
	}

	if !hasValidScope {
		return nil, apperrors.Forbidden(fmt.Sprintf("insufficient scope: requires one of %v", scopes))
	}

	return account, nil
}

// ExtractOptionalAuth extracts authentication if present, but doesn't require it
// This consolidates the pattern used in public endpoints that benefit from auth context  
func ExtractOptionalAuth(ctx *lift.Context, oauthService OAuthServiceInterface) (*AuthenticatedAccount, error) {
	// Check for test mode first
	testUsername := common.GetTestUsername(ctx)
	if testUsername != "" {
		return &AuthenticatedAccount{
			Username: testUsername,
			Claims:   nil,
		}, nil
	}

	// Extract Authorization header
	authHeader := common.ExtractAuthHeader(ctx)
	if authHeader == "" {
		// No auth provided - this is OK for optional auth
		return nil, nil
	}

	// Extract bearer token
	token, err := common.ExtractBearerToken(authHeader)
	if err != nil {
		// Invalid token format - return nil for optional auth
		return nil, nil
	}

	// Validate token and get claims
	claims, err := oauthService.ValidateAccessToken(token)
	if err != nil {
		// Invalid token - return nil for optional auth
		return nil, nil
	}

	return &AuthenticatedAccount{
		Username: claims.GetUsername(),
		Claims:   claims,
	}, nil
}

// RequireReadScope is a convenience function for requiring read access
func RequireReadScope(ctx *lift.Context, oauthService OAuthServiceInterface) (*AuthenticatedAccount, error) {
	return RequireAuthWithScope(ctx, oauthService, ScopeRead)
}

// RequireWriteScope is a convenience function for requiring write access
func RequireWriteScope(ctx *lift.Context, oauthService OAuthServiceInterface) (*AuthenticatedAccount, error) {
	return RequireAuthWithScope(ctx, oauthService, ScopeWrite)
}

// RequireAdminScope is a convenience function for requiring admin access
func RequireAdminScope(ctx *lift.Context, oauthService OAuthServiceInterface) (*AuthenticatedAccount, error) {
	return RequireAuthWithScope(ctx, oauthService, ScopeAdmin)
}

// RequireReadOrWriteScope allows either read or write scope
func RequireReadOrWriteScope(ctx *lift.Context, oauthService OAuthServiceInterface) (*AuthenticatedAccount, error) {
	return RequireAuthWithMultipleScopes(ctx, oauthService, []string{ScopeRead, ScopeWrite})
}

// ValidateAccountOwnership checks if the authenticated user owns the specified account
// This consolidates the pattern: if username != targetUsername { return forbidden }
func ValidateAccountOwnership(account *AuthenticatedAccount, targetUsername string) error {
	if account.Username != targetUsername {
		return apperrors.Forbidden("not authorized to access this account")
	}
	return nil
}

// ValidateAccountOwnershipOrAdmin checks if the user owns the account or is admin
func ValidateAccountOwnershipOrAdmin(account *AuthenticatedAccount, targetUsername string) error {
	if account.Username == targetUsername {
		return nil
	}

	// Test mode bypasses admin checks
	if account.Claims == nil {
		return nil
	}

	if account.Claims.HasScope(ScopeAdmin) {
		return nil
	}

	return apperrors.Forbidden("not authorized to access this account")
}

// OAuthServiceInterface defines the OAuth service interface to avoid import cycles
// This matches the interface expected by the common auth helpers
type OAuthServiceInterface interface {
	ValidateAccessToken(token string) (*Claims, error)
}

// AuthenticationMiddleware provides a Lift middleware for authentication
type AuthenticationMiddleware struct {
	oauthService OAuthServiceInterface
}

// NewAuthenticationMiddleware creates a new authentication middleware
func NewAuthenticationMiddleware(oauthService OAuthServiceInterface) *AuthenticationMiddleware {
	return &AuthenticationMiddleware{
		oauthService: oauthService,
	}
}

// RequireAuthMiddleware returns a Lift middleware that requires authentication  
// Note: This would need to be implemented based on the specific Lift middleware interface
func (am *AuthenticationMiddleware) RequireAuthMiddleware() func(*lift.Context) error {
	return func(ctx *lift.Context) error {
		_, err := GetAccountFromContext(ctx, am.oauthService)
		if err != nil {
			return common.RespondUnauthorized(ctx, err.Error())
		}
		// Continue to next handler - implementation depends on Lift framework
		return nil
	}
}

// RequireScopeMiddleware returns a Lift middleware that requires a specific scope
func (am *AuthenticationMiddleware) RequireScopeMiddleware(scope string) func(*lift.Context) error {
	return func(ctx *lift.Context) error {
		_, err := RequireAuthWithScope(ctx, am.oauthService, scope)
		if err != nil {
			// Check if it's an auth error or scope error
			if apperrors.HasCode(err, apperrors.CodeUnauthorized) {
				return common.RespondUnauthorized(ctx, err.Error())
			}
			return common.RespondForbidden(ctx, err.Error())
		}
		// Continue to next handler - implementation depends on Lift framework
		return nil
	}
}

// OptionalAuthMiddleware returns a Lift middleware that extracts optional auth
// The auth context is stored in the Lift context for downstream handlers
func (am *AuthenticationMiddleware) OptionalAuthMiddleware() func(*lift.Context) error {
	return func(ctx *lift.Context) error {
		account, _ := ExtractOptionalAuth(ctx, am.oauthService)
		if account != nil {
			ctx.Set("authenticated_account", account)
		}
		// Continue to next handler - implementation depends on Lift framework  
		return nil
	}
}

// GetAuthenticatedAccountFromContext retrieves the account set by OptionalAuthMiddleware
func GetAuthenticatedAccountFromContext(ctx *lift.Context) (*AuthenticatedAccount, bool) {
	if account := ctx.Get("authenticated_account"); account != nil {
		if authAccount, ok := account.(*AuthenticatedAccount); ok {
			return authAccount, true
		}
	}
	return nil, false
}

// Context key for standard context.Context usage
type contextKey string

const (
	// ContextKeyAuthenticatedAccount is the key for storing authenticated account in context
	ContextKeyAuthenticatedAccount contextKey = "authenticated_account"
)

// GetAccountFromStandardContext extracts authenticated account from standard context
func GetAccountFromStandardContext(ctx context.Context) (*AuthenticatedAccount, error) {
	if account := ctx.Value(ContextKeyAuthenticatedAccount); account != nil {
		if authAccount, ok := account.(*AuthenticatedAccount); ok {
			return authAccount, nil
		}
	}
	return nil, apperrors.Unauthorized("no authenticated account in context")
}

// GetUsernameFromStandardContext extracts username from standard context
func GetUsernameFromStandardContext(ctx context.Context) (string, error) {
	account, err := GetAccountFromStandardContext(ctx)
	if err != nil {
		return "", err
	}
	return account.Username, nil
}

// RequireAuthFromStandardContext ensures standard context contains authenticated account
func RequireAuthFromStandardContext(ctx context.Context) error {
	_, err := GetAccountFromStandardContext(ctx)
	return err
}

// SetAccountInStandardContext stores authenticated account in standard context
func SetAccountInStandardContext(ctx context.Context, account *AuthenticatedAccount) context.Context {
	return context.WithValue(ctx, ContextKeyAuthenticatedAccount, account)
}