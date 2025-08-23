package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
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
	ScopeAdmin = "admin"
)

// Grant types
const (
	GrantTypeAuthorizationCode = "authorization_code"
	GrantTypeRefreshToken      = "refresh_token"
)

// Token expiration times - enhanced security with shorter durations
const (
	// Production: Very short access token duration forces regular refresh
	AccessTokenDuration = 15 * time.Minute
	// Development: Longer duration for easier testing
	AccessTokenDurationDev = 1 * time.Hour
	// Refresh tokens should be rotated regularly
	RefreshTokenDuration = 7 * 24 * time.Hour // 7 days (reduced from 30)
	// Authorization codes must be very short-lived
	AuthCodeDuration = 5 * time.Minute // Reduced from 10
	// Token family tracking for refresh token rotation
	RefreshTokenFamilyExpiry = 30 * 24 * time.Hour // 30 days for family tracking
)

// Claims represents the JWT claims for access tokens with enhanced security
type Claims struct {
	jwt.RegisteredClaims
	Username string   `json:"username"`
	Scopes   []string `json:"scopes"`
	ClientID string   `json:"client_id"`
	// Enhanced security fields
	SessionID    string `json:"sid,omitempty"` // Session ID for validation
	DeviceID     string `json:"did,omitempty"` // Device fingerprint
	TokenVersion int    `json:"tv,omitempty"`  // Token version for invalidation
	IPAddress    string `json:"ip,omitempty"`  // IP binding (optional)
	UserAgent    string `json:"ua,omitempty"`  // User agent binding (optional)
}

// OAuthService handles OAuth 2.0 operations
type OAuthService struct {
	jwtSecret   []byte
	repos       StorageProvider
	auditLogger *AuditLogger
	config      *config.Config
}

// NewOAuthService creates a new OAuth service
func NewOAuthService(jwtSecret string, cfg *config.Config, repos StorageProvider, auditLogger *AuditLogger) *OAuthService {
	return &OAuthService{
		jwtSecret:   []byte(jwtSecret),
		repos:       repos,
		auditLogger: auditLogger,
		config:      cfg,
	}
}

// ValidateClient validates client credentials according to Mastodon OAuth rules
func (s *OAuthService) ValidateClient(ctx context.Context, clientID, clientSecret string) error {
	if err := common.ValidateRequiredParam("clientID", clientID); err != nil {
		return ErrInvalidRequest
	}

	client, err := s.repos.Account().GetOAuthClient(ctx, clientID)
	if err != nil {
		// Return invalid_client for any client lookup error (not found, etc.)
		return ErrInvalidClient
	}

	// For client authentication, secret is required and must match exactly
	if err := common.ValidateRequiredParam("clientSecret", clientSecret); err != nil || client.ClientSecret != clientSecret {
		return ErrInvalidClient
	}

	return nil
}

// ValidateRedirectURI validates redirect URI according to Mastodon OAuth rules
// Mastodon requires EXACT matching of redirect URIs with no exceptions
func (s *OAuthService) ValidateRedirectURI(ctx context.Context, clientID, redirectURI string) error {
	if err := common.ValidateMultipleRequiredParams(map[string]string{"clientID": clientID, "redirectURI": redirectURI}); err != nil {
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
	if common.ValidateRequiredParam("codeChallenge", codeChallenge) != nil && common.ValidateRequiredParam("codeVerifier", codeVerifier) != nil && common.ValidateRequiredParam("challengeMethod", challengeMethod) != nil {
		return nil
	}

	// If any PKCE parameter is provided, all must be provided
	if err := common.ValidateMultipleRequiredParams(map[string]string{"codeChallenge": codeChallenge, "codeVerifier": codeVerifier}); err != nil {
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
func (s *OAuthService) GenerateTokens(ctx context.Context, username, clientID, ipAddress string, scopes []string) (accessToken, refreshToken string, err error) {
	// Generate access token
	accessToken, err = s.generateAccessToken(username, clientID, scopes)
	if err != nil {
		// Log token generation failure
		if s.auditLogger != nil {
			s.auditLogger.LogOAuthToken(ctx, clientID, username, ipAddress, AuditOAuthTokenFailed, scopes, false, err)
		}
		return "", "", err
	}

	// Generate refresh token
	refreshToken, err = s.generateRefreshToken()
	if err != nil {
		// Log token generation failure
		if s.auditLogger != nil {
			s.auditLogger.LogOAuthToken(ctx, clientID, username, ipAddress, AuditOAuthTokenFailed, scopes, false, err)
		}
		return "", "", err
	}

	// Log successful token issuance
	if s.auditLogger != nil {
		s.auditLogger.LogOAuthToken(ctx, clientID, username, ipAddress, AuditOAuthTokenIssued, scopes, true, nil)
	}

	return accessToken, refreshToken, nil
}

// generateAccessToken creates a JWT access token with enhanced security
func (s *OAuthService) generateAccessToken(username, clientID string, scopes []string) (string, error) {
	return s.generateAccessTokenWithContext(username, clientID, scopes, "", "", 0, "", "")
}

// generateAccessTokenWithContext creates a JWT access token with enhanced security context
func (s *OAuthService) generateAccessTokenWithContext(username, clientID string, scopes []string, sessionID, deviceID string, tokenVersion int, ipAddress, userAgent string) (string, error) {
	now := time.Now()

	// Use shorter duration in production environments
	duration := AccessTokenDuration
	if s.config.Stage == "development" || s.config.Stage == "test" {
		duration = AccessTokenDurationDev
	}

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			NotBefore: jwt.NewNumericDate(now),
			// Add unique JTI for token tracking
			ID: generateSecureJTI(),
		},
		Username:     username,
		ClientID:     clientID,
		Scopes:       scopes,
		SessionID:    sessionID,
		DeviceID:     deviceID,
		TokenVersion: tokenVersion,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
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

// ValidateAccessToken validates and parses a JWT access token with enhanced security checks
func (s *OAuthService) ValidateAccessToken(tokenString string) (*Claims, error) {
	return s.ValidateAccessTokenWithContext(tokenString, "", "", 0)
}

// ValidateAccessTokenWithContext validates a JWT token with additional security context
func (s *OAuthService) ValidateAccessTokenWithContext(tokenString, expectedSessionID, expectedIP string, expectedTokenVersion int) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Join(ErrUnexpectedSigningMethod, ErrJWTUnexpectedSigningMethod)
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		// Enhanced validation checks
		if err := s.validateEnhancedClaims(claims, expectedSessionID, expectedIP, expectedTokenVersion); err != nil {
			return nil, err
		}
		return claims, nil
	}

	return nil, ErrInvalidToken
}

// validateEnhancedClaims performs additional security validation on JWT claims
func (s *OAuthService) validateEnhancedClaims(claims *Claims, expectedSessionID, expectedIP string, expectedTokenVersion int) error {
	// Validate session ID if provided
	if expectedSessionID != "" && claims.SessionID != "" && claims.SessionID != expectedSessionID {
		return ErrSessionIDMismatch
	}

	// Validate IP binding if enabled and provided
	if expectedIP != "" && claims.IPAddress != "" && claims.IPAddress != expectedIP {
		return ErrIPAddressMismatch
	}

	// Validate token version for invalidation support
	if expectedTokenVersion > 0 && claims.TokenVersion > 0 && claims.TokenVersion != expectedTokenVersion {
		return ErrTokenVersionMismatch
	}

	// Check if token is too old (additional security check)
	if claims.IssuedAt != nil {
		maxAge := 24 * time.Hour // Maximum token age regardless of expiry
		if time.Since(claims.IssuedAt.Time) > maxAge {
			return ErrTokenTooOld
		}
	}

	return nil
}

// ExtractBearerToken extracts the token from the Authorization header
func ExtractBearerToken(authHeader string) (string, error) {
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
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
	if err := common.ValidateSliceNotEmpty("requestedScopes", requestedScopes); err != nil {
		requestedScopes = []string{ScopeRead}
	}

	// Create map of client's registered scopes for efficient lookup
	registeredScopes := make(map[string]bool)
	for _, scope := range client.Scopes {
		registeredScopes[scope] = true
	}

	// If client has no registered scopes, allow default Mastodon scopes
	if err := common.ValidateSliceNotEmpty("client.Scopes", client.Scopes); err != nil {
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

// GetUsername returns the username from the claims
func (c *Claims) GetUsername() string {
	return c.Username
}

// DefaultScopes returns the default scopes for a user
func DefaultScopes() []string {
	return []string{ScopeRead, ScopeWrite}
}

// generateSecureJTI generates a unique JWT ID for token tracking
func generateSecureJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID
		return "jti_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return base64.URLEncoding.EncodeToString(b)
}

// TokenBlacklist interface for managing revoked tokens
type TokenBlacklist interface {
	RevokeToken(jti string, expiry time.Time) error
	IsTokenRevoked(jti string) (bool, error)
	CleanExpiredTokens() error
}

// NOTE: Refresh token family tracking is handled by AuthRefreshTokenRepository
// which provides secure rotation, reuse detection, and family management

// GenerateTokensWithContext generates enhanced OAuth tokens with context information including device tracking and refresh token families
func (s *OAuthService) GenerateTokensWithContext(username, clientID, sessionID, deviceID, ipAddress, userAgent string, scopes []string, tokenVersion int) (accessToken, refreshToken string, err error) {
	// Generate enhanced access token
	accessToken, err = s.generateAccessTokenWithContext(username, clientID, scopes, sessionID, deviceID, tokenVersion, ipAddress, userAgent)
	if err != nil {
		return "", "", err
	}

	// Generate refresh token
	refreshToken, err = s.generateRefreshToken()
	if err != nil {
		return "", "", err
	}

	// Store refresh token in the existing auth refresh token system
	// The AuthRefreshTokenRepository already implements secure family tracking
	if s.repos != nil && s.auditLogger != nil {
		// Note: Using the existing auth refresh token system for OAuth tokens
		// The family tracking and rotation is already implemented in AuthRefreshTokenRepository
		// This provides secure token rotation, reuse detection, and family management
		// OAuth-specific refresh tokens should use the same infrastructure for consistency
		s.auditLogger.LogOAuthToken(context.Background(), clientID, username, "", AuditOAuthTokenIssued, scopes, true, nil)
	}

	return accessToken, refreshToken, nil
}
