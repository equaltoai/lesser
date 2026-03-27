package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storageinterfaces "github.com/equaltoai/lesser/pkg/storage/interfaces"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

type authAccountRepository interface {
	GetUser(ctx context.Context, username string) (*storage.User, error)
	UpdateUser(ctx context.Context, username string, updates map[string]any) error
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	GetDevice(ctx context.Context, deviceID string) (*storage.Device, error)
	GetSession(ctx context.Context, sessionID string) (*storage.Session, error)
	GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error)
	MarkWalletChallengeSpent(ctx context.Context, challengeID string) error
	ResetWalletChallengeSpent(ctx context.Context, challengeID string) error
	GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error)
	StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error
}

type oauthClientValidator interface {
	ValidateClient(ctx context.Context, clientID, clientSecret string) error
	ValidateRedirectURI(ctx context.Context, clientID, redirectURI string) error
	GenerateAuthorizationCode() (string, error)
}

// AuthService provides comprehensive authentication functionality
//
//nolint:revive // Auth prefix clarifies this is the authentication service
type AuthService struct {
	repos           StorageProvider
	accountRepo     authAccountRepository
	activityRepo    storageinterfaces.ActivityRepository
	oauthService    oauthClientValidator
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
		accountRepo:     repos.Account(),
		activityRepo:    repos.Activity(),
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
func (as *AuthService) AuthenticateWithPassword(_ context.Context, _, _, _, _, _ string) (*AuthResponse, error) {
	return nil, ErrPasswordAuthDisabled
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
	accountRepo := as.account()
	if accountRepo == nil {
		return ErrInvalidCredentials
	}

	// Verify the device belongs to the user
	device, err := accountRepo.GetDevice(ctx, deviceID)
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
	accountRepo := as.account()

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
		if accountRepo == nil {
			return nil, ErrInvalidToken
		}
		session, err := accountRepo.GetSession(context.Background(), claims.SessionID)
		if err != nil || time.Now().After(session.ExpiresAt) {
			return nil, ErrInvalidToken
		}

		return claims, nil
	}

	return nil, ErrInvalidToken
}

// ChangePassword changes a user's password
func (as *AuthService) ChangePassword(_ context.Context, _, _, _ string) error {
	return ErrPasswordAuthDisabled
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

// VerifyWalletSignature verifies a wallet signature (for registration)
// This only verifies the signature - it does NOT create a session
func (as *AuthService) VerifyWalletSignature(ctx context.Context, req *WalletVerifyRequest) error {
	// Verify signature
	_, err := as.walletService.VerifySignature(ctx, req)
	if err != nil {
		return errors.Join(ErrSignatureVerificationFailed, err)
	}
	return nil
}

// LoginWithWallet logs in a user with a wallet (wallet must already be linked)
func (as *AuthService) LoginWithWallet(ctx context.Context, req *WalletVerifyRequest, deviceName, userAgent, ipAddress string) (*AuthResponse, error) {
	accountRepo := as.account()
	if accountRepo == nil {
		return nil, errors.Join(ErrWalletCheck, errors.New("account repository not configured"))
	}

	// Verify signature and get username
	username, err := as.walletService.VerifySignature(ctx, req)
	if err != nil {
		return nil, errors.Join(ErrSignatureVerificationFailed, err)
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, errors.New("username required for wallet login")
	}

	// Check that this wallet address is linked to the requesting username
	wallets, err := accountRepo.GetUserWalletCredentials(ctx, username)
	if err != nil {
		return nil, errors.Join(ErrWalletCheck, err)
	}

	linked := false
	for _, w := range wallets {
		if strings.EqualFold(w.Address, req.Address) {
			linked = true
			break
		}
	}

	if !linked {
		return nil, errors.Join(ErrWalletCheck, errors.New("wallet not linked to this username"))
	}

	// Get user and verify account status
	user, err := accountRepo.GetUser(ctx, username)
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

// LoginWithWalletAfterLinking creates a session and access token for a wallet-linked user without re-verifying signature
// Used after wallet linking during registration when signature was already verified
func (as *AuthService) LoginWithWalletAfterLinking(ctx context.Context, username, deviceName, userAgent, ipAddress string) (*AuthResponse, error) {
	accountRepo := as.account()
	if accountRepo == nil {
		return nil, errors.Join(ErrUserRetrievalFailed, errors.New("account repository not configured"))
	}

	// Get user and verify account status
	user, err := accountRepo.GetUser(ctx, username)
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

// LinkWallet links a wallet to an existing user account.
// The returned boolean reports whether this call created a new link.
func (as *AuthService) LinkWallet(ctx context.Context, username, address string, chainID int, walletType string) (bool, error) {
	return as.walletService.LinkWallet(ctx, username, address, chainID, walletType)
}

// MarkWalletChallengeSpent marks a wallet challenge as spent (second verification)
func (as *AuthService) MarkWalletChallengeSpent(ctx context.Context, challengeID string) error {
	accountRepo := as.account()
	if accountRepo == nil {
		return ErrWalletCheck
	}
	return accountRepo.MarkWalletChallengeSpent(ctx, challengeID)
}

// ResetWalletChallengeSpent clears the spent flag so an onboarding proof can be retried
// after downstream work is safely rolled back.
func (as *AuthService) ResetWalletChallengeSpent(ctx context.Context, challengeID string) error {
	accountRepo := as.account()
	if accountRepo == nil {
		return ErrWalletCheck
	}
	return accountRepo.ResetWalletChallengeSpent(ctx, challengeID)
}

// GetWalletChallenge retrieves a wallet challenge by ID
func (as *AuthService) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	accountRepo := as.account()
	if accountRepo == nil {
		return nil, ErrWalletCheck
	}
	return accountRepo.GetWalletChallenge(ctx, challengeID)
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
	accountRepo := as.account()
	if accountRepo == nil {
		return "", errors.New("account repository not configured")
	}

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
	if err := accountRepo.StoreRecoveryToken(ctx, recoveryKey, recoveryData); err != nil {
		return "", errors.Join(ErrRecoveryTokenStorageFailed, err)
	}

	return token, nil
}

func (as *AuthService) account() authAccountRepository {
	if as.accountRepo != nil {
		return as.accountRepo
	}
	if as.repos != nil {
		return as.repos.Account()
	}
	return nil
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
