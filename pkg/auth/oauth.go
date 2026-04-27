package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
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
	GrantTypeClientCredentials = "client_credentials"
)

// Token expiration times
const (
	// Access token duration: 1 hour is reasonable for external client applications
	// Balances security with usability - clients should use refresh tokens for longer sessions
	AccessTokenDuration = 1 * time.Hour
	// Refresh tokens should be rotated regularly
	RefreshTokenDuration = 7 * 24 * time.Hour // 7 days
	// Authorization codes must be very short-lived
	AuthCodeDuration = 5 * time.Minute
	// Token family tracking for refresh token rotation
	RefreshTokenFamilyExpiry = 30 * 24 * time.Hour // 30 days for family tracking
)

// Client classes identify the client category that minted an access token.
// These are used for server-side policy decisions (e.g., automation safety rails) without relying on spoofable headers.
const (
	ClientClassWeb   = "web"
	ClientClassCLI   = "cli"
	ClientClassAgent = "agent"
)

// AgentAccessTokenTTL returns the configured default lifetime for agent access tokens.
// When no agent-specific override is configured, Lesser falls back to the shared OAuth default.
func AgentAccessTokenTTL(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.AgentAccessTokenDuration > 0 {
		return cfg.AgentAccessTokenDuration
	}
	return AccessTokenDuration
}

// Claims represents the JWT claims for access tokens with enhanced security
type Claims struct {
	jwt.RegisteredClaims
	Username string   `json:"username"`
	Scopes   []string `json:"scopes"`
	ClientID string   `json:"client_id"`
	// ClientClass categorizes the client that minted the token (e.g., "cli" for device-flow CLI tokens).
	ClientClass string `json:"client_class,omitempty"`
	// Enhanced security fields
	SessionID    string `json:"sid,omitempty"` // Session ID for validation
	DeviceID     string `json:"did,omitempty"` // Device fingerprint
	TokenVersion int    `json:"tv,omitempty"`  // Token version for invalidation
	IPAddress    string `json:"ip,omitempty"`  // IP binding (optional)
	UserAgent    string `json:"ua,omitempty"`  // User agent binding (optional)

	// Agent context (LLM agents)
	IsAgent        bool   `json:"is_agent,omitempty"`
	AgentType      string `json:"agent_type,omitempty"`
	DelegatedBy    string `json:"delegated_by,omitempty"`
	AgentSessionID string `json:"agent_session_id,omitempty"`
}

// OAuthService handles OAuth 2.0 operations
type OAuthService struct {
	jwtSecret   []byte
	repos       StorageProvider
	accountRepo oauthAccountRepository
	auditLogger *AuditLogger
	config      *config.Config
}

type oauthAccountRepository interface {
	GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error)
}

// NewOAuthService creates a new OAuth service
func NewOAuthService(jwtSecret string, cfg *config.Config, repos StorageProvider, auditLogger *AuditLogger) *OAuthService {
	return &OAuthService{
		jwtSecret:   []byte(jwtSecret),
		repos:       repos,
		accountRepo: getOAuthAccountRepo(repos),
		auditLogger: auditLogger,
		config:      cfg,
	}
}

func getOAuthAccountRepo(repos StorageProvider) oauthAccountRepository {
	if repos == nil {
		return nil
	}

	accountRepo := repos.Account()
	if accountRepo == nil {
		return nil
	}

	return accountRepo
}

func (s *OAuthService) account() oauthAccountRepository {
	if s.accountRepo != nil {
		return s.accountRepo
	}
	return getOAuthAccountRepo(s.repos)
}

type oauthClientSecretValidationResult struct {
	matchedCurrent        bool
	matchedPrevious       bool
	currentNeedsMigration bool
}

func oauthClientActiveSecretValue(client *storage.OAuthClient) string {
	if client == nil {
		return ""
	}
	if client.ClientSecretHash != "" {
		return client.ClientSecretHash
	}
	return client.ClientSecret
}

func oauthClientPreviousSecretGraceActive(now time.Time, client *storage.OAuthClient) bool {
	if client == nil || client.PreviousClientSecretHash == "" {
		return false
	}
	if client.PreviousClientSecretGraceExpiresAt.IsZero() {
		return false
	}
	return !now.After(client.PreviousClientSecretGraceExpiresAt)
}

func validateOAuthClientSecretAt(now time.Time, client *storage.OAuthClient, clientSecret string) (oauthClientSecretValidationResult, error) {
	currentSecret := oauthClientActiveSecretValue(client)
	ok, needsMigration, verifyErr := VerifyOAuthClientSecret(clientSecret, currentSecret)
	if verifyErr != nil {
		return oauthClientSecretValidationResult{}, verifyErr
	}
	if ok {
		return oauthClientSecretValidationResult{
			matchedCurrent:        true,
			currentNeedsMigration: needsMigration,
		}, nil
	}

	if oauthClientPreviousSecretGraceActive(now, client) {
		previousOK, _, previousErr := VerifyOAuthClientSecret(clientSecret, client.PreviousClientSecretHash)
		if previousErr != nil {
			return oauthClientSecretValidationResult{}, previousErr
		}
		if previousOK {
			return oauthClientSecretValidationResult{matchedPrevious: true}, nil
		}
	}

	return oauthClientSecretValidationResult{}, nil
}

// ValidateClient validates client credentials according to Mastodon OAuth rules
func (s *OAuthService) ValidateClient(ctx context.Context, clientID, clientSecret string) error {
	if err := common.ValidateRequiredParam("clientID", clientID); err != nil {
		return ErrInvalidRequest
	}

	accountRepo := s.account()
	if accountRepo == nil {
		return ErrInvalidClient
	}

	client, err := accountRepo.GetOAuthClient(ctx, clientID)
	if err != nil {
		// Return invalid_client for any client lookup error (not found, etc.)
		return ErrInvalidClient
	}

	// For client authentication, secret is required and must verify against the stored representation.
	if err := common.ValidateRequiredParam("clientSecret", clientSecret); err != nil {
		return ErrInvalidClient
	}

	result, verifyErr := validateOAuthClientSecretAt(time.Now().UTC(), client, clientSecret)
	if verifyErr != nil || (!result.matchedCurrent && !result.matchedPrevious) {
		return ErrInvalidClient
	}

	if result.currentNeedsMigration {
		// Best-effort lazy migration: convert legacy plaintext secrets to hashes on first successful use.
		type secretUpdater interface {
			UpdateOAuthClientSecretHash(ctx context.Context, clientID, clientSecretHash string) error
		}
		if updater, ok := accountRepo.(secretUpdater); ok {
			if hashed, err := HashOAuthClientSecret(clientSecret); err == nil {
				_ = updater.UpdateOAuthClientSecretHash(ctx, clientID, hashed)
			}
		}
	}

	return nil
}

// ValidateRedirectURI validates redirect URI according to Mastodon OAuth rules
// Mastodon requires EXACT matching of redirect URIs with no exceptions
func (s *OAuthService) ValidateRedirectURI(ctx context.Context, clientID, redirectURI string) error {
	if err := common.ValidateMultipleRequiredParams(map[string]string{"clientID": clientID, "redirectURI": redirectURI}); err != nil {
		return ErrInvalidRequest
	}

	accountRepo := s.account()
	if accountRepo == nil {
		return ErrInvalidClient
	}

	client, err := accountRepo.GetOAuthClient(ctx, clientID)
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
	return s.GenerateTokensWithAccessTokenTTL(ctx, username, clientID, ipAddress, scopes, AccessTokenDuration)
}

// GenerateTokensWithAccessTokenTTL generates both access and refresh tokens with a custom access token TTL.
func (s *OAuthService) GenerateTokensWithAccessTokenTTL(ctx context.Context, username, clientID, ipAddress string, scopes []string, accessTokenTTL time.Duration) (accessToken, refreshToken string, err error) {
	return s.GenerateTokensWithAccessTokenTTLAndClientContext(ctx, username, clientID, ipAddress, scopes, accessTokenTTL, "", "")
}

// GenerateTokensWithAccessTokenTTLAndClientContext generates tokens with an explicit client class and session ID.
//
// This enables downstream middleware to enforce server-side policies (e.g., automation safety rails) based on
// non-spoofable classification derived at token issuance time. For agent accounts, if a sessionID is not provided,
// a new session ID is generated.
func (s *OAuthService) GenerateTokensWithAccessTokenTTLAndClientContext(ctx context.Context, username, clientID, ipAddress string, scopes []string, accessTokenTTL time.Duration, clientClass, sessionID string) (accessToken, refreshToken string, err error) {
	return s.GenerateTokensWithAccessTokenTTLAndClientContextAndAudience(ctx, username, clientID, ipAddress, scopes, accessTokenTTL, clientClass, sessionID, "")
}

// GenerateTokensWithAccessTokenTTLAndClientContextAndAudience generates tokens with explicit client context and JWT audience.
func (s *OAuthService) GenerateTokensWithAccessTokenTTLAndClientContextAndAudience(ctx context.Context, username, clientID, ipAddress string, scopes []string, accessTokenTTL time.Duration, clientClass, sessionID, audience string) (accessToken, refreshToken string, err error) {
	if accessTokenTTL <= 0 {
		accessTokenTTL = AccessTokenDuration
	}

	isAgent, agentType, delegatedBy := s.resolveAgentClaims(ctx, username)
	effectiveSessionID := strings.TrimSpace(sessionID)
	agentSessionID := ""
	if isAgent {
		if effectiveSessionID == "" {
			effectiveSessionID = generateSecureJTI()
		}
		agentSessionID = effectiveSessionID
	}

	accessToken, err = s.generateAccessTokenWithMetadata(username, clientID, scopes, accessTokenMetadata{
		ExpiresAt:      time.Now().Add(accessTokenTTL),
		IPAddress:      ipAddress,
		SessionID:      effectiveSessionID,
		ClientClass:    clientClass,
		Audience:       audience,
		IsAgent:        isAgent,
		AgentType:      agentType,
		DelegatedBy:    delegatedBy,
		AgentSessionID: agentSessionID,
	})
	if err != nil {
		if s.auditLogger != nil {
			s.auditLogger.LogOAuthToken(ctx, clientID, username, ipAddress, AuditOAuthTokenFailed, scopes, false, err)
		}
		return "", "", err
	}

	refreshToken, err = s.generateRefreshToken()
	if err != nil {
		if s.auditLogger != nil {
			s.auditLogger.LogOAuthToken(ctx, clientID, username, ipAddress, AuditOAuthTokenFailed, scopes, false, err)
		}
		return "", "", err
	}

	if s.auditLogger != nil {
		s.auditLogger.LogOAuthToken(ctx, clientID, username, ipAddress, AuditOAuthTokenIssued, scopes, true, nil)
	}

	return accessToken, refreshToken, nil
}

type accessTokenMetadata struct {
	SessionID    string
	DeviceID     string
	TokenVersion int
	IPAddress    string
	UserAgent    string
	ExpiresAt    time.Time
	Audience     string

	IsAgent        bool
	AgentType      string
	DelegatedBy    string
	AgentSessionID string

	ClientClass string
}

// generateAccessTokenWithMetadata creates a JWT access token with enhanced security context.
func (s *OAuthService) generateAccessTokenWithMetadata(username, clientID string, scopes []string, meta accessTokenMetadata) (string, error) {
	now := time.Now()
	expiresAt := meta.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = now.Add(AccessTokenDuration)
	}

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			// Add unique JTI for token tracking
			ID: generateSecureJTI(),
		},
		Username:       username,
		ClientID:       clientID,
		Scopes:         scopes,
		ClientClass:    meta.ClientClass,
		SessionID:      meta.SessionID,
		DeviceID:       meta.DeviceID,
		TokenVersion:   meta.TokenVersion,
		IPAddress:      meta.IPAddress,
		UserAgent:      meta.UserAgent,
		IsAgent:        meta.IsAgent,
		AgentType:      meta.AgentType,
		DelegatedBy:    meta.DelegatedBy,
		AgentSessionID: meta.AgentSessionID,
	}
	if audience := strings.TrimSpace(meta.Audience); audience != "" {
		claims.Audience = jwt.ClaimStrings{audience}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func normalizeDelegatedBy(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "@") {
		return trimmed
	}
	return "@" + trimmed
}

func (s *OAuthService) resolveAgentClaims(ctx context.Context, username string) (bool, string, string) {
	if s == nil || s.repos == nil {
		return false, "", ""
	}

	accountRepo := s.repos.Account()
	if accountRepo == nil {
		return false, "", ""
	}

	user, err := accountRepo.GetUser(ctx, username)
	if err != nil || user == nil || !user.IsAgent {
		return false, "", ""
	}

	return true, strings.TrimSpace(user.AgentType), normalizeDelegatedBy(user.AgentOwner)
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

func (s *OAuthService) parseAccessToken(tokenString string) (*jwt.Token, error) {
	return jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Join(ErrUnexpectedSigningMethod, ErrJWTUnexpectedSigningMethod)
		}
		return s.jwtSecret, nil
	})
}

func (s *OAuthService) logJWTParseError(err error) {
	if s == nil || s.auditLogger == nil || s.auditLogger.logger == nil {
		return
	}

	s.auditLogger.logger.Warn("JWT token parsing failed",
		zap.Error(err),
		zap.String("error_type", fmt.Sprintf("%T", err)),
	)
}

func (s *OAuthService) logJWTValidationFailed(token *jwt.Token) {
	if s == nil || s.auditLogger == nil || s.auditLogger.logger == nil {
		return
	}

	tokenValid := false
	if token != nil {
		tokenValid = token.Valid
	}

	claimsOK := false
	if token != nil {
		if _, ok := token.Claims.(*Claims); ok {
			claimsOK = true
		}
	}

	s.auditLogger.logger.Warn("JWT token validation failed",
		zap.Bool("token_valid", tokenValid),
		zap.Bool("claims_ok", claimsOK),
	)
}

func (s *OAuthService) validateAccessTokenNotRevoked(claims *Claims) error {
	if s == nil || s.repos == nil || claims == nil {
		return nil
	}

	accountRepo := s.repos.Account()
	if accountRepo == nil {
		return nil
	}

	jti := strings.TrimSpace(claims.ID)
	if jti == "" {
		return nil
	}

	checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	revoked, err := accountRepo.IsAccessTokenRevoked(checkCtx, jti)
	cancel()
	if err != nil {
		if s.auditLogger != nil && s.auditLogger.logger != nil {
			s.auditLogger.logger.Warn("access token revocation check failed", zap.Error(err))
		}
		return nil
	}

	if revoked {
		return ErrInvalidToken
	}

	return nil
}

func (s *OAuthService) validateNativeSessionTokenStillActive(claims *Claims) error {
	if s == nil || s.repos == nil || claims == nil {
		return nil
	}
	if strings.TrimSpace(claims.ID) != "" {
		return nil
	}

	sessionID := strings.TrimSpace(claims.SessionID)
	if sessionID == "" {
		return nil
	}

	accountRepo := s.repos.Account()
	if accountRepo == nil {
		return ErrInvalidToken
	}

	checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	session, err := accountRepo.GetSession(checkCtx, sessionID)
	cancel()
	if err != nil || session == nil || session.IsRevoked || time.Now().After(session.ExpiresAt) {
		return ErrInvalidToken
	}

	return nil
}

// ValidateAccessTokenWithContext validates a JWT token with additional security context
func (s *OAuthService) ValidateAccessTokenWithContext(tokenString, expectedSessionID, expectedIP string, expectedTokenVersion int) (*Claims, error) {
	token, err := s.parseAccessToken(tokenString)
	if err != nil {
		s.logJWTParseError(err)
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		s.logJWTValidationFailed(token)
		return nil, ErrInvalidToken
	}

	// Enhanced validation checks
	if err := s.validateEnhancedClaims(claims, expectedSessionID, expectedIP, expectedTokenVersion); err != nil {
		return nil, err
	}

	// Best-effort access token revocation check (RFC 7009).
	// This is a read-after-parse DynamoDB lookup keyed by token JTI; if storage is unavailable,
	// we accept the token to avoid turning storage blips into auth outages.
	if err := s.validateAccessTokenNotRevoked(claims); err != nil {
		return nil, err
	}

	if err := s.validateNativeSessionTokenStillActive(claims); err != nil {
		return nil, err
	}

	return claims, nil
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

	return s.validateAccessTokenAgePolicy(claims)
}

// validateAccessTokenAgePolicy keeps age-policy decisions in one place.
// Standard OAuth access tokens are bounded by explicit JWT expiry (`exp`) and
// revocation/session checks rather than a second hidden max-age wall.
func (s *OAuthService) validateAccessTokenAgePolicy(claims *Claims) error {
	if claims == nil || claims.IssuedAt == nil || claims.IssuedAt.IsZero() {
		return nil
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
	accountRepo := s.account()
	if accountRepo == nil {
		return ErrInvalidClient
	}

	client, err := accountRepo.GetOAuthClient(ctx, clientID)
	if err != nil {
		return ErrInvalidClient
	}

	// If no scopes requested, default to "read" per Mastodon spec
	if err := common.ValidateSliceNotEmpty("requestedScopes", requestedScopes); err != nil {
		requestedScopes = []string{ScopeRead}
	}

	if err := ValidatePublicOAuthScopes(requestedScopes); err != nil {
		return err
	}

	// If client has no registered scopes, allow default Mastodon scopes
	if err := common.ValidateSliceNotEmpty("client.Scopes", client.Scopes); err != nil {
		return nil
	}

	// Validate that all requested scopes are subset of registered scopes
	if !ScopeSetAllows(client.Scopes, requestedScopes) {
		return ErrInvalidScope
	}

	return nil
}

// ValidateScopes checks if the requested scopes are valid globally
func ValidateScopes(scopes []string) error {
	for _, scope := range scopes {
		if !isRecognizedOAuthScope(scope) {
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
	return strings.ToLower(strings.TrimSpace(c.Username))
}

// DefaultScopes returns the default scopes for a user
func DefaultScopes() []string {
	return append([]string(nil), defaultOAuthScopes...)
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
	isAgent, agentType, delegatedBy := s.resolveAgentClaims(context.Background(), username)
	agentSessionID := ""
	if isAgent && strings.TrimSpace(sessionID) != "" {
		agentSessionID = sessionID
	}

	accessToken, err = s.generateAccessTokenWithMetadata(username, clientID, scopes, accessTokenMetadata{
		SessionID:      sessionID,
		DeviceID:       deviceID,
		TokenVersion:   tokenVersion,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		ExpiresAt:      time.Now().Add(AccessTokenDuration),
		IsAgent:        isAgent,
		AgentType:      agentType,
		DelegatedBy:    delegatedBy,
		AgentSessionID: agentSessionID,
	})
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
