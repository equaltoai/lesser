package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrInvalidGrant is returned when the authorization code is invalid or expired
	ErrInvalidGrant = errors.New("invalid_grant")
	// ErrInvalidClient is returned when the client is not authorized
	ErrInvalidClient = errors.New("invalid_client")
	// ErrInvalidRequest is returned when the request is malformed
	ErrInvalidRequest = errors.New("invalid_request")
	// ErrUnauthorizedClient is returned when the client is not authorized for this grant type
	ErrUnauthorizedClient = errors.New("unauthorized_client")
	// ErrUnsupportedGrantType is returned when the grant type is not supported
	ErrUnsupportedGrantType = errors.New("unsupported_grant_type")
	// ErrInvalidToken is returned when the token is invalid
	ErrInvalidToken = errors.New("invalid_token")
	// ErrInvalidCodeChallenge is returned when the code challenge doesn't match
	ErrInvalidCodeChallenge = errors.New("invalid_code_challenge")
	// ErrInvalidScope is returned when requested scopes are invalid
	ErrInvalidScope = errors.New("invalid_scope")
	// ErrInvalidAPIKey is returned when the API key is invalid
	ErrInvalidAPIKey = errors.New("invalid_api_key")
)

// Scopes define the permissions that can be granted
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// Grant types
const (
	GrantTypeAuthorizationCode = "authorization_code"
	GrantTypeRefreshToken      = "refresh_token"
)

// Token expiration times
const (
	AccessTokenDuration  = 1 * time.Hour
	RefreshTokenDuration = 30 * 24 * time.Hour // 30 days
	AuthCodeDuration     = 10 * time.Minute
)

// Claims represents the JWT claims for access tokens
type Claims struct {
	jwt.RegisteredClaims
	Username string   `json:"username"`
	Scopes   []string `json:"scopes"`
	ClientID string   `json:"client_id"`
}

// OAuthService handles OAuth 2.0 operations
type OAuthService struct {
	jwtSecret []byte
	repos     core.RepositoryStorage
}

// NewOAuthService creates a new OAuth service
func NewOAuthService(jwtSecret string, repos core.RepositoryStorage) *OAuthService {
	return &OAuthService{
		jwtSecret: []byte(jwtSecret),
		repos:     repos,
	}
}

// ValidateClient validates client credentials according to Mastodon OAuth rules
func (s *OAuthService) ValidateClient(ctx context.Context, clientID, clientSecret string) error {
	if clientID == "" {
		return ErrInvalidRequest
	}

	client, err := s.repos.Account().GetOAuthClient(ctx, clientID)
	if err != nil {
		// Return invalid_client for any client lookup error (not found, etc.)
		return ErrInvalidClient
	}

	// For client authentication, secret is required and must match exactly
	if clientSecret == "" || client.ClientSecret != clientSecret {
		return ErrInvalidClient
	}

	return nil
}

// ValidateRedirectURI validates redirect URI according to Mastodon OAuth rules
// Mastodon requires EXACT matching of redirect URIs with no exceptions
func (s *OAuthService) ValidateRedirectURI(ctx context.Context, clientID, redirectURI string) error {
	if clientID == "" || redirectURI == "" {
		return ErrInvalidRequest
	}

	client, err := s.repos.Account().GetOAuthClient(ctx, clientID)
	if err != nil {
		// Return invalid_client for any client lookup error
		return ErrInvalidClient
	}

	// Mastodon requires EXACT matching - no prefix matching, no exceptions
	// The only special case is the out-of-band URI
	for _, registeredURI := range client.RedirectURIs {
		if registeredURI == redirectURI {
			return nil // Exact match found
		}
	}

	// Special case: out-of-band URI (displays code instead of redirecting)
	if redirectURI == "urn:ietf:wg:oauth:2.0:oob" {
		// Only allowed if client registered it explicitly
		for _, registeredURI := range client.RedirectURIs {
			if registeredURI == "urn:ietf:wg:oauth:2.0:oob" {
				return nil
			}
		}
	}

	// No match found - this is an error per Mastodon OAuth spec
	return ErrInvalidRequest
}

// GenerateAuthorizationCode generates a new authorization code
func (s *OAuthService) GenerateAuthorizationCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// VerifyCodeChallenge verifies the PKCE code challenge per Mastodon 4.3.0+ requirements
// Mastodon only supports S256 method for PKCE
func (s *OAuthService) VerifyCodeChallenge(codeChallenge, codeVerifier, challengeMethod string) error {
	// If PKCE is not used, skip verification
	if codeChallenge == "" && codeVerifier == "" && challengeMethod == "" {
		return nil
	}

	// If any PKCE parameter is provided, all must be provided
	if codeChallenge == "" || codeVerifier == "" {
		return ErrInvalidRequest
	}

	// Mastodon 4.3.0+ only supports S256 method
	if challengeMethod != "S256" {
		return ErrInvalidRequest
	}

	// Verify S256 code challenge
	h := sha256.Sum256([]byte(codeVerifier))
	computedChallenge := base64.RawURLEncoding.EncodeToString(h[:])
	if codeChallenge != computedChallenge {
		return ErrInvalidCodeChallenge
	}

	return nil
}

// GenerateTokens generates both access and refresh tokens
func (s *OAuthService) GenerateTokens(username, clientID string, scopes []string) (accessToken, refreshToken string, err error) {
	// Generate access token
	accessToken, err = s.generateAccessToken(username, clientID, scopes)
	if err != nil {
		return "", "", err
	}

	// Generate refresh token
	refreshToken, err = s.generateRefreshToken()
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// generateAccessToken creates a JWT access token
func (s *OAuthService) generateAccessToken(username, clientID string, scopes []string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenDuration)),
			NotBefore: jwt.NewNumericDate(now),
		},
		Username: username,
		ClientID: clientID,
		Scopes:   scopes,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// generateRefreshToken creates a random refresh token
func (s *OAuthService) generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ValidateAccessToken validates and parses a JWT access token
func (s *OAuthService) ValidateAccessToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

// ExtractBearerToken extracts the token from the Authorization header
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrInvalidToken
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", ErrInvalidToken
	}

	return parts[1], nil
}

// ValidateScopes validates scopes against client's registered scopes per Mastodon rules
func (s *OAuthService) ValidateScopes(ctx context.Context, clientID string, requestedScopes []string) error {
	// Get client to check registered scopes
	client, err := s.repos.Account().GetOAuthClient(ctx, clientID)
	if err != nil {
		return ErrInvalidClient
	}

	// If no scopes requested, default to "read" per Mastodon spec
	if len(requestedScopes) == 0 {
		requestedScopes = []string{ScopeRead}
	}

	// Create map of client's registered scopes for efficient lookup
	registeredScopes := make(map[string]bool)
	for _, scope := range client.Scopes {
		registeredScopes[scope] = true
	}

	// If client has no registered scopes, allow default Mastodon scopes
	if len(client.Scopes) == 0 {
		registeredScopes = map[string]bool{
			ScopeRead:  true,
			ScopeWrite: true,
			"follow":   true,
			"push":     true,
		}
	}

	// Validate that all requested scopes are subset of registered scopes
	for _, requestedScope := range requestedScopes {
		if !registeredScopes[requestedScope] {
			return ErrInvalidScope
		}
	}

	return nil
}

// ValidateScopes checks if the requested scopes are valid globally
func ValidateScopes(scopes []string) error {
	validScopes := map[string]bool{
		ScopeRead:  true,
		ScopeWrite: true,
		"follow":   true, // Mastodon-specific
		"push":     true, // Mastodon-specific for push notifications
		"admin":    true, // Admin access
	}

	for _, scope := range scopes {
		if !validScopes[scope] {
			return ErrInvalidScope
		}
	}

	return nil
}

// HasScope checks if the claims contain a specific scope
func (c *Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// DefaultScopes returns the default scopes for a user
func DefaultScopes() []string {
	return []string{ScopeRead, ScopeWrite}
}
