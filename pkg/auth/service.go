package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// AuthService provides comprehensive authentication functionality
//
//nolint:revive // Auth prefix clarifies this is the authentication service
type AuthService struct {
	repos           StorageProvider
	oauthService    *OAuthService
	sessionManager  *SessionManager
	rateLimiter     *RateLimiter
	webAuthnService *WebAuthnService
	walletService   *WalletService
	auditLogger     *AuditLogger
	jwtSecret       []byte
	config          *config.Config
}

// NewAuthService creates a comprehensive auth service
func NewAuthService(cfg *config.Config, repos StorageProvider) (*AuthService, error) {
	jwtSecret := cfg.JWTSecret
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	
	// Validate JWT secret strength
	if err := validateJWTSecretStrength(jwtSecret); err != nil {
		return nil, fmt.Errorf("invalid JWT_SECRET: %w", err)
	}

	// Get domain for WebAuthn configuration
	domain := cfg.Domain
	if domain == "" {
		domain = "lesser.app"
	}

	// Initialize WebAuthn service
	webAuthnService, err := NewWebAuthnService(repos, domain, "Lesser")
	if err != nil {
		common.Logger().Warn("failed to initialize WebAuthn service", zap.Error(err))
		// Continue without WebAuthn support
		webAuthnService = nil
	}

	// Initialize Wallet service
	walletService := NewWalletService(repos)

	// Initialize Audit Logger
	auditLogger := NewAuditLogger(repos, common.Logger(), DefaultAuditConfig())

	return &AuthService{
		repos:           repos,
		oauthService:    NewOAuthService(jwtSecret, cfg, repos, auditLogger),
		sessionManager:  NewSessionManager(repos),
		rateLimiter:     NewRateLimiter(repos),
		webAuthnService: webAuthnService,
		walletService:   walletService,
		auditLogger:     auditLogger,
		jwtSecret:       []byte(jwtSecret),
		config:          cfg,
	}, nil
}

// AuthenticateWithPassword authenticates a user with username and password
func (as *AuthService) AuthenticateWithPassword(ctx context.Context, username, password, deviceName, userAgent, ipAddress string) (*AuthResponse, error) {
	// Check rate limits first
	if err := as.rateLimiter.CheckRateLimit(ctx, username, ipAddress); err != nil {
		// Log rate limited attempt
		as.auditLogger.LogLogin(ctx, username, ipAddress, userAgent, deviceName, false, "rate limited")
		return nil, err
	}

	// Get user from storage
	user, err := as.repos.Account().GetUser(ctx, username)
	if err != nil {
		// Record failed attempt
		_ = as.rateLimiter.RecordAttempt(ctx, username, ipAddress, false)
		// Log failed login - user not found
		as.auditLogger.LogLogin(ctx, username, ipAddress, userAgent, deviceName, false, "user not found")
		return nil, ErrInvalidCredentials
	}

	// Check if user is active
	if user.Suspended {
		_ = as.rateLimiter.RecordAttempt(ctx, username, ipAddress, false)
		// Log suspended account attempt
		if err := as.auditLogger.LogEvent(ctx, &AuditEvent{
			EventType:     AuditLoginSuspended,
			Username:      username,
			IPAddress:     ipAddress,
			UserAgent:     userAgent,
			DeviceName:    deviceName,
			Success:       false,
			FailureReason: "account suspended",
		}); err != nil {
			// Log audit error but continue
			as.auditLogger.logger.Warn("Failed to log audit event", zap.Error(err))
		}
		return nil, ErrUserSuspended
	}
	if !user.Approved {
		_ = as.rateLimiter.RecordAttempt(ctx, username, ipAddress, false)
		// Log unapproved account attempt
		if err := as.auditLogger.LogEvent(ctx, &AuditEvent{
			EventType:     AuditLoginNotApproved,
			Username:      username,
			IPAddress:     ipAddress,
			UserAgent:     userAgent,
			DeviceName:    deviceName,
			Success:       false,
			FailureReason: "account not approved",
		}); err != nil {
			// Log audit error but continue
			as.auditLogger.logger.Warn("Failed to log audit event", zap.Error(err))
		}
		return nil, ErrUserNotApproved
	}

	// Verify password
	if err := VerifyPassword(password, user.PasswordHash); err != nil {
		// Record failed attempt
		_ = as.rateLimiter.RecordAttempt(ctx, username, ipAddress, false)
		// Log failed login - wrong password
		as.auditLogger.LogLogin(ctx, username, ipAddress, userAgent, deviceName, false, "invalid password")

		// Check if this might be a brute force attempt
		if count, _ := as.rateLimiter.GetFailedAttempts(ctx, username); count >= 5 {
			as.auditLogger.LogSecurityEvent(ctx, AuditBruteForceDetected, username, ipAddress, map[string]interface{}{
				"failed_attempts": count,
			})
		}
		return nil, ErrInvalidCredentials
	}

	// Record successful attempt
	if err := as.rateLimiter.RecordAttempt(ctx, username, ipAddress, true); err != nil {
		common.Logger().Error("failed to record successful login", zap.Error(err))
	}

	// Log successful login
	as.auditLogger.LogLogin(ctx, username, ipAddress, userAgent, deviceName, true, "")

	// Get actor ID for activity recording
	actor, err := as.repos.Account().GetActor(ctx, username)
	if err != nil {
		// Log but don't fail login
		common.Logger().Warn("failed to get actor for activity recording", zap.Error(err))
	} else {
		// Record login activity for metrics
		if err := as.repos.Activity().RecordActivity(ctx, "login", actor.ID, time.Now()); err != nil {
			// Log the error but don't fail the login
			common.Logger().Warn("failed to record login activity", zap.Error(err))
		}
	}

	// Create session
	session, err := as.sessionManager.CreateSession(ctx, username, deviceName, userAgent, ipAddress, "password")
	if err != nil {
		return nil, errors.Join(ErrSessionCreationFailed, err)
	}

	// Generate tokens with shorter access token duration
	accessToken, err := as.generateShortLivedAccessToken(username, session.SessionID, session.DeviceID, DefaultScopes())
	if err != nil {
		return nil, errors.Join(ErrAccessTokenGenerationFailed, err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(ShortAccessTokenDuration.Seconds()),
		RefreshToken: session.RefreshToken,
		Scope:        "read write",
		CreatedAt:    time.Now().Unix(),
		Me:           username,
	}, nil
}

// RefreshAccessToken exchanges a refresh token for a new access token
func (as *AuthService) RefreshAccessToken(ctx context.Context, refreshToken, ipAddress string) (*AuthResponse, error) {
	// Validate refresh token and get session
	session, err := as.sessionManager.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	// Update session activity
	if err := as.sessionManager.UpdateSessionActivity(ctx, session.SessionID, ipAddress); err != nil {
		common.Logger().Error("failed to update session activity", zap.Error(err))
	}

	// Rotate refresh token for enhanced security
	newRefreshToken, err := as.sessionManager.RotateRefreshToken(ctx, session)
	if err != nil {
		// Log but don't fail - we can still issue a new access token
		common.Logger().Error("failed to rotate refresh token", zap.Error(err))
		newRefreshToken = refreshToken
	}

	// Generate new short-lived access token
	accessToken, err := as.generateShortLivedAccessToken(session.Username, session.SessionID, session.DeviceID, DefaultScopes())
	if err != nil {
		return nil, errors.Join(ErrAccessTokenGenerationFailed, err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(ShortAccessTokenDuration.Seconds()),
		RefreshToken: newRefreshToken,
		Scope:        "read write",
		CreatedAt:    time.Now().Unix(),
		Me:           session.Username,
	}, nil
}

// Logout revokes a session
func (as *AuthService) Logout(ctx context.Context, sessionID string) error {
	return as.sessionManager.RevokeSession(ctx, sessionID)
}

// LogoutAllDevices revokes all sessions for a user
func (as *AuthService) LogoutAllDevices(ctx context.Context, username string) error {
	return as.sessionManager.RevokeAllUserSessions(ctx, username)
}

// GetUserDevices returns all devices for a user
func (as *AuthService) GetUserDevices(ctx context.Context, username string) ([]*Device, error) {
	return as.sessionManager.GetUserDevices(ctx, username)
}

// TrustDevice marks a device as trusted
func (as *AuthService) TrustDevice(ctx context.Context, username, deviceID string) error {
	// Verify the device belongs to the user
	device, err := as.repos.Account().GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if device.Username != username {
		return ErrDeviceOwnershipMismatch
	}

	return as.sessionManager.TrustDevice(ctx, deviceID)
}

// generateShortLivedAccessToken creates a JWT with enhanced claims
func (as *AuthService) generateShortLivedAccessToken(username, sessionID, deviceID string, scopes []string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ShortAccessTokenDuration)),
			NotBefore: jwt.NewNumericDate(now),
		},
		Username:  username,
		ClientID:  "web", // Can be extended for different clients
		Scopes:    scopes,
		SessionID: sessionID,
		DeviceID:  deviceID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(as.jwtSecret)
}

// ValidateAccessToken validates and parses an enhanced JWT access token
func (as *AuthService) ValidateAccessToken(tokenString string) (*EnhancedClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &EnhancedClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			common.Logger().Error("unexpected JWT signing method", zap.Any("method", token.Header["alg"]))
			return nil, ErrJWTUnexpectedSigningMethod
		}
		return as.jwtSecret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*EnhancedClaims); ok && token.Valid {
		// Verify session is still valid
		session, err := as.repos.Account().GetSession(context.Background(), claims.SessionID)
		if err != nil || time.Now().After(session.ExpiresAt) {
			return nil, ErrInvalidToken
		}

		return claims, nil
	}

	return nil, ErrInvalidToken
}

// ChangePassword changes a user's password
func (as *AuthService) ChangePassword(ctx context.Context, username, oldPassword, newPassword string) error {
	// Get user
	user, err := as.repos.Account().GetUser(ctx, username)
	if err != nil {
		return ErrUserNotFound
	}

	// Verify old password
	if err := VerifyPassword(oldPassword, user.PasswordHash); err != nil {
		return ErrInvalidCredentials
	}

	// Hash new password
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return errors.Join(ErrPasswordHashingFailed, err)
	}

	// Update user
	updates := map[string]any{
		"password_hash": newHash,
		"updated_at":    time.Now(),
	}

	if err := as.repos.Account().UpdateUser(ctx, username, updates); err != nil {
		return errors.Join(ErrPasswordUpdateFailed, err)
	}

	// Revoke all sessions to force re-authentication
	_ = as.sessionManager.RevokeAllUserSessions(ctx, username)

	return nil
}

// GetAccountStatus returns the rate limit status for an account
func (as *AuthService) GetAccountStatus(ctx context.Context, username string) (*RateLimitStatus, error) {
	return as.rateLimiter.GetAccountStatus(ctx, username)
}

// ClearAccountLockout clears rate limiting for a specific account (admin action)
func (as *AuthService) ClearAccountLockout(ctx context.Context, username string) error {
	return as.rateLimiter.ClearAccountLockout(ctx, username)
}

// Legacy OAuth methods for compatibility

// ValidateClient validates OAuth client credentials
func (as *AuthService) ValidateClient(ctx context.Context, clientID, clientSecret string) error {
	return as.oauthService.ValidateClient(ctx, clientID, clientSecret)
}

// ValidateRedirectURI validates OAuth redirect URI
func (as *AuthService) ValidateRedirectURI(ctx context.Context, clientID, redirectURI string) error {
	return as.oauthService.ValidateRedirectURI(ctx, clientID, redirectURI)
}

// GenerateAuthorizationCode generates OAuth authorization code
func (as *AuthService) GenerateAuthorizationCode() (string, error) {
	return as.oauthService.GenerateAuthorizationCode()
}

// WebAuthn methods

// BeginWebAuthnRegistration starts the WebAuthn registration process
func (as *AuthService) BeginWebAuthnRegistration(ctx context.Context, username string) (any, string, error) {
	if as.webAuthnService == nil {
		return nil, "", ErrWebAuthnNotConfigured
	}
	return as.webAuthnService.BeginRegistration(ctx, username)
}

// FinishWebAuthnRegistration completes the WebAuthn registration process
func (as *AuthService) FinishWebAuthnRegistration(ctx context.Context, username string, challenge string, response []byte, credentialName string) error {
	if as.webAuthnService == nil {
		return ErrWebAuthnNotConfigured
	}
	return as.webAuthnService.FinishRegistration(ctx, username, challenge, response, credentialName)
}

// BeginWebAuthnLogin starts the WebAuthn login process
func (as *AuthService) BeginWebAuthnLogin(ctx context.Context, username string) (any, string, error) {
	if as.webAuthnService == nil {
		return nil, "", ErrWebAuthnNotConfigured
	}
	return as.webAuthnService.BeginLogin(ctx, username)
}

// FinishWebAuthnLogin completes the WebAuthn login process and creates a session
func (as *AuthService) FinishWebAuthnLogin(ctx context.Context, username string, challenge string, response []byte, deviceName, userAgent, ipAddress string) (*AuthResponse, error) {
	if as.webAuthnService == nil {
		return nil, ErrWebAuthnNotConfigured
	}

	// Verify the WebAuthn credential
	credential, err := as.webAuthnService.FinishLogin(ctx, username, challenge, response)
	if err != nil {
		return nil, err
	}

	// Create session with WebAuthn auth method
	session, err := as.sessionManager.CreateSession(ctx, username, deviceName, userAgent, ipAddress, "passkey")
	if err != nil {
		return nil, errors.Join(ErrSessionCreationFailed, err)
	}

	// Generate tokens
	accessToken, err := as.generateShortLivedAccessToken(username, session.SessionID, session.DeviceID, DefaultScopes())
	if err != nil {
		return nil, errors.Join(ErrAccessTokenGenerationFailed, err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(ShortAccessTokenDuration.Seconds()),
		RefreshToken: session.RefreshToken,
		Scope:        "read write",
		CreatedAt:    time.Now().Unix(),
		Me:           username,
		CredentialID: credential.ID, // Include credential ID in response
	}, nil
}

// GetWebAuthnCredentials returns all WebAuthn credentials for a user
func (as *AuthService) GetWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	if as.webAuthnService == nil {
		return nil, ErrWebAuthnNotConfigured
	}
	return as.webAuthnService.GetUserCredentials(ctx, username)
}

// DeleteWebAuthnCredential removes a WebAuthn credential
func (as *AuthService) DeleteWebAuthnCredential(ctx context.Context, username string, credentialID string) error {
	if as.webAuthnService == nil {
		return ErrWebAuthnNotConfigured
	}
	return as.webAuthnService.DeleteCredential(ctx, username, credentialID)
}

// UpdateWebAuthnCredentialName updates the display name of a credential
func (as *AuthService) UpdateWebAuthnCredentialName(ctx context.Context, username string, credentialID string, newName string) error {
	if as.webAuthnService == nil {
		return ErrWebAuthnNotConfigured
	}
	return as.webAuthnService.UpdateCredentialName(ctx, username, credentialID, newName)
}

// Wallet authentication methods

// CreateWalletChallenge creates a new authentication challenge for wallet signing
func (as *AuthService) CreateWalletChallenge(ctx context.Context, address string, chainID int, username string) (*storage.WalletChallenge, error) {
	return as.walletService.CreateChallenge(ctx, address, chainID, username)
}

// VerifyWalletSignature verifies a wallet signature and creates a session
func (as *AuthService) VerifyWalletSignature(ctx context.Context, req *WalletVerifyRequest, deviceName, userAgent, ipAddress string) (*AuthResponse, error) {
	// Verify signature and get username
	username, err := as.walletService.VerifySignature(ctx, req)
	if err != nil {
		return nil, errors.Join(ErrSignatureVerificationFailed, err)
	}

	// If no username returned, this is a new wallet
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return &AuthResponse{
			AccessToken:  "", // No token for unlinked wallet
			TokenType:    "Bearer",
			ExpiresIn:    0,
			RefreshToken: "",
			Scope:        "",
			CreatedAt:    time.Now().Unix(),
			Me:           "", // No username yet
		}, nil
	}

	// Check if user is active
	user, err := as.repos.Account().GetUser(ctx, username)
	if err != nil {
		return nil, errors.Join(ErrUserRetrievalFailed, err)
	}
	if user.Suspended {
		return nil, ErrUserSuspended
	}
	if !user.Approved {
		return nil, ErrUserNotApproved
	}

	// Create session with wallet auth method
	session, err := as.sessionManager.CreateSession(ctx, username, deviceName, userAgent, ipAddress, "wallet")
	if err != nil {
		return nil, errors.Join(ErrSessionCreationFailed, err)
	}

	// Generate tokens
	accessToken, err := as.generateShortLivedAccessToken(username, session.SessionID, session.DeviceID, DefaultScopes())
	if err != nil {
		return nil, errors.Join(ErrAccessTokenGenerationFailed, err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(ShortAccessTokenDuration.Seconds()),
		RefreshToken: session.RefreshToken,
		Scope:        "read write",
		CreatedAt:    time.Now().Unix(),
		Me:           username,
	}, nil
}

// LinkWallet links a wallet to an existing user account
func (as *AuthService) LinkWallet(ctx context.Context, username, address string, chainID int, walletType string) error {
	return as.walletService.LinkWallet(ctx, username, address, chainID, walletType)
}

// UnlinkWallet removes a wallet link from a user account
func (as *AuthService) UnlinkWallet(ctx context.Context, username, address string) error {
	return as.walletService.UnlinkWallet(ctx, username, address)
}

// GetUserWallets returns all wallets linked to a user
func (as *AuthService) GetUserWallets(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	return as.walletService.GetUserWallets(ctx, username)
}

// GetStore returns the repository storage instance (for handlers that need direct access)
func (as *AuthService) GetStore() StorageProvider {
	return as.repos
}

// GetConfig returns configuration (for handlers that need environment info)
func (as *AuthService) GetConfig() *ServiceConfig {
	env := as.config.Stage
	if env == "" {
		env = "development"
	}
	return &ServiceConfig{
		Environment: env,
	}
}

// GenerateRecoveryToken generates a recovery token for WebAuthn/federation-based recovery
func (as *AuthService) GenerateRecoveryToken(ctx context.Context, username string, recoveryMethod string) (string, error) {
	// Generate a secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", errors.Join(ErrRecoveryTokenGenerationFailed, err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Store recovery token with metadata
	recoveryData := map[string]any{
		"username":        username,
		"token":           token,
		"recovery_method": recoveryMethod,
		"expiresAt":       time.Now().Add(24 * time.Hour).Unix(),
		"used":            false,
	}

	recoveryKey := fmt.Sprintf("RECOVERY#%s", token)
	if err := as.repos.Account().StoreRecoveryToken(ctx, recoveryKey, recoveryData); err != nil {
		return "", errors.Join(ErrRecoveryTokenStorageFailed, err)
	}

	return token, nil
}

// ServiceConfig represents auth service configuration
type ServiceConfig struct {
	Environment string
}

// Response types

// AuthResponse represents an authentication response
//
//nolint:revive // Auth prefix clarifies this is an authentication response
type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	CreatedAt    int64  `json:"created_at"`
	Me           string `json:"me,omitempty"`            // Username for Mastodon compatibility
	CredentialID string `json:"credential_id,omitempty"` // WebAuthn credential ID (if applicable)
}

// EnhancedClaims is now an alias for the improved Claims struct
type EnhancedClaims = Claims

// validateJWTSecretStrength validates that the JWT secret meets security requirements
func validateJWTSecretStrength(secret string) error {
	// Check minimum length (32 characters for 256-bit security)
	if len(secret) < 32 {
		return fmt.Errorf("must be at least 32 characters long")
	}
	
	// Check for common weak patterns
	lowerSecret := strings.ToLower(secret)
	weakPatterns := []string{
		"default",
		"change",
		"secret",
		"password",
		"12345",
		"admin",
		"test",
		"demo",
		"example",
	}
	
	for _, pattern := range weakPatterns {
		if strings.Contains(lowerSecret, pattern) {
			return fmt.Errorf("contains weak pattern '%s'", pattern)
		}
	}
	
	return nil
}
