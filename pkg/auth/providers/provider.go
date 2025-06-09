package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aron23/lesser/pkg/common"
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

// baseProvider provides common OAuth functionality
type baseProvider struct {
	name   string
	config Config
	client *http.Client
}

// GetName returns the provider name
func (p *baseProvider) GetName() string {
	return p.name
}

// GetAuthURL returns the authorization URL
func (p *baseProvider) GetAuthURL(state, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("state", state)

	if len(p.config.Scopes) > 0 {
		params.Set("scope", strings.Join(p.config.Scopes, " "))
	}

	return fmt.Sprintf("%s?%s", p.config.AuthURL, params.Encode())
}

// ExchangeCode exchanges an authorization code for tokens
func (p *baseProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", p.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var tokenResp TokenResponse
	if err := common.ParseHTTPResponse(resp.Body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// makeAPIRequest makes an authenticated API request
func (p *baseProvider) makeAPIRequest(ctx context.Context, method, url, accessToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}
