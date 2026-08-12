// Package common provides shared authentication helper functions and types for the Lesser project.
// These helpers consolidate common authentication patterns found across all services.
package common //nolint:revive // package name "common" is intentional and widely used throughout the codebase

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/logging"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// ContextKey is the type for context keys
type ContextKey string

const (
	// ContextKeyClaims is the context key for JWT claims
	ContextKeyClaims ContextKey = "claims"
	// ContextKeyActAsAgent carries the raw X-Lesser-Act-As indicator value from the
	// HTTP transport into GraphQL resolvers for share-grant act-as resolution.
	ContextKeyActAsAgent ContextKey = "act_as_agent"
)

// Claims represents JWT claims interface
type Claims interface {
	HasScope(scope string) bool
	GetUsername() string
}

// AuthContext holds authentication state
type AuthContext struct {
	Username string
	Claims   Claims
}

// AuthenticationResult holds the result of authentication validation
type AuthenticationResult struct {
	Context   *AuthContext
	Error     error
	ErrorCode int
}

// Common authentication patterns consolidated into reusable functions

// ExtractAndValidateAuth performs the complete authentication flow
// This consolidates the most common pattern found across 80+ files:
//   - Extract Authorization header (with fallback patterns)
//   - Extract bearer token
//   - Validate JWT token
//   - Check required scopes
func ExtractAndValidateAuth(ctx *apptheory.Context, requiredScope string, oauthService OAuthServiceInterface) *AuthenticationResult {

	// Extract Authorization header with multiple fallback patterns
	authHeader := ExtractAuthHeader(ctx)
	if authHeader == "" {
		return &AuthenticationResult{
			ErrorCode: 401,
			Error:     ErrMissingAuthorizationHeader,
		}
	}

	// Extract bearer token
	token, err := ExtractBearerToken(authHeader)
	if err != nil {
		return &AuthenticationResult{
			ErrorCode: 401,
			Error:     fmt.Errorf("invalid bearer token: %w", err),
		}
	}

	// Validate token and get claims
	claims, err := oauthService.ValidateAccessToken(token)
	if err != nil {
		return &AuthenticationResult{
			ErrorCode: 401,
			Error:     fmt.Errorf("invalid token: %w", err),
		}
	}

	// Check required scope if specified
	if requiredScope != "" && !claims.HasScope(requiredScope) {
		return &AuthenticationResult{
			ErrorCode: 403,
			Error:     fmt.Errorf("%w: requires %s", ErrInsufficientScope, requiredScope),
		}
	}

	return &AuthenticationResult{
		Context: &AuthContext{
			Username: claims.GetUsername(),
			Claims:   claims,
		},
	}
}

// ExtractAndValidateAuthWithMultipleScopes validates authentication with multiple allowed scopes
// This handles the pattern where either of several scopes is acceptable
func ExtractAndValidateAuthWithMultipleScopes(ctx *apptheory.Context, allowedScopes []string, oauthService OAuthServiceInterface) *AuthenticationResult {

	// Extract Authorization header
	authHeader := ExtractAuthHeader(ctx)
	if authHeader == "" {
		return &AuthenticationResult{
			ErrorCode: 401,
			Error:     ErrMissingAuthorizationHeader,
		}
	}

	// Extract bearer token
	token, err := ExtractBearerToken(authHeader)
	if err != nil {
		return &AuthenticationResult{
			ErrorCode: 401,
			Error:     fmt.Errorf("invalid bearer token: %w", err),
		}
	}

	// Validate token and get claims
	claims, err := oauthService.ValidateAccessToken(token)
	if err != nil {
		return &AuthenticationResult{
			ErrorCode: 401,
			Error:     fmt.Errorf("invalid token: %w", err),
		}
	}

	// Check if user has any of the allowed scopes
	if len(allowedScopes) > 0 {
		hasValidScope := false
		for _, scope := range allowedScopes {
			if claims.HasScope(scope) {
				hasValidScope = true
				break
			}
		}

		if !hasValidScope {
			return &AuthenticationResult{
				ErrorCode: 403,
				Error:     fmt.Errorf("%w: requires one of %v", ErrInsufficientScope, allowedScopes),
			}
		}
	}

	return &AuthenticationResult{
		Context: &AuthContext{
			Username: claims.GetUsername(),
			Claims:   claims,
		},
	}
}

// ExtractOptionalAuth performs optional authentication (for public endpoints that benefit from auth)
// This consolidates the pattern where auth is optional but used if present
func ExtractOptionalAuth(ctx *apptheory.Context, oauthService OAuthServiceInterface) *AuthenticationResult {

	// Extract Authorization header
	authHeader := ExtractAuthHeader(ctx)
	if authHeader == "" {
		// No auth provided - this is OK for optional auth
		return &AuthenticationResult{
			Context: &AuthContext{},
		}
	}

	// Extract bearer token
	token, err := ExtractBearerToken(authHeader)
	if err != nil {
		// Invalid token format - return unauthenticated state for optional auth
		// Log the error for debugging
		if logger := Logger(); logger != nil {
			logger.Debug("optional auth: invalid bearer token format",
				zap.Error(err),
				zap.String("path", logging.SanitizeLogPath(ctx.Request.Path)),
			)
		}
		return &AuthenticationResult{
			Context: &AuthContext{},
		}
	}

	// Validate token and get claims
	claims, err := oauthService.ValidateAccessToken(token)
	if err != nil {
		// Invalid token - return unauthenticated state for optional auth
		// Log the error for debugging
		if logger := Logger(); logger != nil {
			tokenPreview := token
			if len(token) > 20 {
				tokenPreview = token[:20] + "..."
			}
			logger.Warn("optional auth: token validation failed",
				zap.Error(err),
				zap.String("path", logging.SanitizeLogPath(ctx.Request.Path)),
				zap.String("token_preview", tokenPreview),
			)
		}
		return &AuthenticationResult{
			Context: &AuthContext{},
		}
	}

	return &AuthenticationResult{
		Context: &AuthContext{
			Username: claims.GetUsername(),
			Claims:   claims,
		},
	}
}

// ExtractAuthHeader extracts Authorization header with multiple fallback patterns
// This consolidates the various patterns found across the codebase
func ExtractAuthHeader(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}
	if ctx.Request.Headers == nil {
		return ""
	}
	if values := ctx.Request.Headers["authorization"]; len(values) > 0 {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}

	for key, values := range ctx.Request.Headers {
		if !strings.EqualFold(strings.TrimSpace(key), "authorization") {
			continue
		}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

// ValidateWriteAccess validates that the authenticated user can perform write operations
func ValidateWriteAccess(authResult *AuthenticationResult) error {
	if authResult.Error != nil {
		return authResult.Error
	}

	if authResult.Context.Claims == nil {
		return ErrAuthenticationRequired
	}

	if !authResult.Context.Claims.HasScope(ScopeWrite) {
		return ErrInsufficientScopeWrite
	}

	return nil
}

// ValidateReadAccess validates that the authenticated user can perform read operations
func ValidateReadAccess(authResult *AuthenticationResult) error {
	if authResult.Error != nil {
		return authResult.Error
	}

	if authResult.Context.Claims == nil {
		return ErrAuthenticationRequiredRead
	}

	if !authResult.Context.Claims.HasScope(ScopeRead) {
		return ErrInsufficientScopeRead
	}

	return nil
}

// OAuthServiceInterface defines the interface for OAuth validation
// This avoids import cycles while providing the needed validation functionality
type OAuthServiceInterface interface {
	ValidateAccessToken(token string) (Claims, error)
}

// Common scope constants to avoid import cycles
const (
	ScopeRead    = "read"
	ScopeWrite   = "write"
	WriteFollows = "write:follows"
	ReadFollows  = "read:follows"
	AdminRead    = "admin:read"
	AdminWrite   = "admin:write"
)

// Common scope validation patterns

// HasAnyScope checks if claims has any of the provided scopes
func HasAnyScope(claims Claims, scopes []string) bool {
	if claims == nil {
		return false
	}

	for _, scope := range scopes {
		if claims.HasScope(scope) {
			return true
		}
	}
	return false
}

// HasAllScopes checks if claims has all of the provided scopes
func HasAllScopes(claims Claims, scopes []string) bool {
	if claims == nil {
		return false
	}

	for _, scope := range scopes {
		if !claims.HasScope(scope) {
			return false
		}
	}
	return true
}

// Common scope combinations found in the codebase
var (
	ReadScopes   = []string{ScopeRead, "read:accounts", "read:statuses", "read:follows"}
	WriteScopes  = []string{ScopeWrite, "write:accounts", "write:statuses", "write:follows"}
	FollowScopes = []string{WriteFollows, ScopeWrite, "write:follows"}
	BlockScopes  = []string{ScopeWrite, "write:blocks"}
	AdminScopes  = []string{AdminRead, AdminWrite}
)

// ValidateReadScopes validates against common read scope combinations
func ValidateReadScopes(claims Claims) bool {
	return HasAnyScope(claims, ReadScopes)
}

// ValidateWriteScopes validates against common write scope combinations
func ValidateWriteScopes(claims Claims) bool {
	return HasAnyScope(claims, WriteScopes)
}

// ValidateFollowScopes validates against common follow scope combinations
func ValidateFollowScopes(claims Claims) bool {
	return HasAnyScope(claims, FollowScopes)
}

// ValidateBlockScopes validates against common block scope combinations
func ValidateBlockScopes(claims Claims) bool {
	return HasAnyScope(claims, BlockScopes)
}

// ValidateAdminScopes validates against common admin scope combinations
func ValidateAdminScopes(claims Claims) bool {
	return HasAnyScope(claims, AdminScopes)
}

// ExtractBearerToken extracts the token from the Authorization header
// This consolidates auth.ExtractBearerToken to avoid import cycles
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrAuthHeaderEmpty
	}

	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return "", ErrAuthHeaderInvalidPrefix
	}

	// Use centralized validation and sanitization for bearer token
	token, err := ValidateAndSanitizeString("bearer_token", authHeader[len(bearerPrefix):], 1, 2000)
	if err != nil {
		return "", fmt.Errorf("bearer token validation failed: %w", err)
	}

	return token, nil
}
