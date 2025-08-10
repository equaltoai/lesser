// Package providers defines OAuth provider interfaces and user information structures for social authentication.
package providers

import (
	"context"
)

// Provider represents an OAuth provider interface
type Provider interface {
	// GetName returns the provider name (e.g., "github", "discord", "google")
	GetName() string

	// GetAuthURL returns the authorization URL for the provider
	GetAuthURL(state, redirectURI string) string

	// ExchangeCode exchanges an authorization code for an access token
	ExchangeCode(ctx context.Context, code, redirectURI string) (*TokenResponse, error)

	// GetUserInfo fetches user information using the access token
	GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error)
}

// TokenResponse represents the OAuth token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// UserInfo represents user information from the provider
type UserInfo struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	AvatarURL  string `json:"avatar_url"`
	Provider   string `json:"provider"`
	ProviderID string `json:"provider_id"`
}

// Config represents OAuth provider configuration
type Config struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
}
