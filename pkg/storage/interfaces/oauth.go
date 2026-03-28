// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// OAuthRepository defines the interface for OAuth-related storage operations.
// This handles OAuth 2.0 client management, authorization codes, tokens, and user consent.
type OAuthRepository interface {
	// ===== OAuth State Operations =====

	// StoreOAuthState stores OAuth state for CSRF protection
	StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error

	// GetOAuthState retrieves OAuth state
	GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error)

	// DeleteOAuthState deletes OAuth state
	DeleteOAuthState(ctx context.Context, state string) error

	// ===== Authorization Code Operations =====

	// CreateAuthorizationCode creates a new OAuth authorization code
	CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error

	// GetAuthorizationCode retrieves an OAuth authorization code
	GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error)

	// DeleteAuthorizationCode deletes an OAuth authorization code
	DeleteAuthorizationCode(ctx context.Context, code string) error

	// ===== Refresh Token Operations =====

	// CreateRefreshToken creates a new OAuth refresh token
	CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error

	// GetRefreshToken retrieves an OAuth refresh token
	GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error)

	// DeleteRefreshToken deletes an OAuth refresh token
	DeleteRefreshToken(ctx context.Context, token string) error

	// ===== OAuth Client Operations =====

	// CreateOAuthClient creates a new OAuth client
	CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error

	// GetOAuthClient retrieves an OAuth client by client ID
	GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error)

	// UpdateOAuthClient updates an existing OAuth client
	UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error

	// DeleteOAuthClient deletes an OAuth client
	DeleteOAuthClient(ctx context.Context, clientID string) error

	// ListOAuthClients lists OAuth clients with pagination
	ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error)

	// ===== Token Cleanup =====

	// DeleteExpiredTokens removes expired OAuth tokens
	DeleteExpiredTokens(ctx context.Context) error

	// ===== User Consent Operations =====

	// SaveUserAppConsent saves user consent for an OAuth app
	SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error

	// GetUserAppConsent retrieves user consent for an OAuth app
	GetUserAppConsent(ctx context.Context, userID, appID, resource string) (*storage.UserAppConsent, error)
}
