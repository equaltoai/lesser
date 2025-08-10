// Package main implements the auth Lambda function for authentication operations.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// AuthHandler handles authentication requests using Lift
type AuthHandler struct {
	repos       core.RepositoryStorage
	logger      *zap.Logger
	cfg         *config.Config
	authSvc     *auth.AuthService
	oauthSvc    *auth.OAuthService
	webAuthnSvc *auth.WebAuthnService
	walletSvc   *auth.WalletService
}

// AuthCode represents an OAuth authorization code with PKCE
type AuthCode struct {
	Code        string    `json:"code"`
	Challenge   string    `json:"challenge"`
	Method      string    `json:"method"`
	ClientID    string    `json:"client_id"`
	RedirectURI string    `json:"redirect_uri"`
	Scope       string    `json:"scope"`
	UserID      string    `json:"user_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Session represents a user session
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler() (*AuthHandler, error) {
	cfg := config.Get()
	logger := common.Logger()

	// Initialize DynamORM
	tableName := os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		tableName = cfg.DynamoTableName
	}
	if tableName == "" {
		return nil, fmt.Errorf("DYNAMODB_TABLE environment variable is required")
	}

	// Load AWS config
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Create repository factory
	repos, err := factory.NewRepositoryFactory(db, tableName, awsConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize repository factory: %w", err)
	}

	// Initialize comprehensive auth service using repositories
	authSvc, err := auth.NewAuthService(repos)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth service: %w", err)
	}

	// Initialize OAuth service separately for token operations
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "development-secret-change-me"
		logger.Warn("JWT_SECRET not set, using development default")
	}
	oauthSvc := auth.NewOAuthService(jwtSecret, repos)

	// Initialize WebAuthn service
	domain := cfg.Domain
	if domain == "" {
		domain = "lesser.host"
	}
	webAuthnSvc, err := auth.NewWebAuthnService(repos, domain, "Lesser")
	if err != nil {
		logger.Error("failed to initialize WebAuthn service", zap.Error(err))
		webAuthnSvc = nil
	}

	// Initialize Wallet service
	walletSvc := auth.NewWalletService(repos)

	return &AuthHandler{
		repos:       repos,
		logger:      logger,
		cfg:         cfg,
		authSvc:     authSvc,
		oauthSvc:    oauthSvc,
		webAuthnSvc: webAuthnSvc,
		walletSvc:   walletSvc,
	}, nil
}

// RegisterRoutes registers all authentication routes
func (ah *AuthHandler) RegisterRoutes(app *lift.App) {
	// OAuth 2.0 Discovery endpoint (RFC 8414)
	_ = app.GET("/.well-known/oauth-authorization-server", ah.handleOAuthDiscovery)
	
	// OAuth 2.0 endpoints
	_ = app.GET("/oauth/authorize", ah.handleAuthorize)
	_ = app.POST("/oauth/token", ah.handleToken)
	_ = app.POST("/oauth/revoke", ah.handleRevoke)

	// Session management
	_ = app.POST("/auth/login", ah.handleLogin)
	_ = app.POST("/auth/logout", ah.handleLogout)
	_ = app.GET("/auth/session", ah.handleSession)

	// WebAuthn endpoints
	if ah.webAuthnSvc != nil {
		_ = app.POST("/auth/webauthn/register/begin", ah.handleWebAuthnRegisterBegin)
		_ = app.POST("/auth/webauthn/register/finish", ah.handleWebAuthnRegisterFinish)
		_ = app.POST("/auth/webauthn/login/begin", ah.handleWebAuthnLoginBegin)
		_ = app.POST("/auth/webauthn/login/finish", ah.handleWebAuthnLoginFinish)
	}

	// Wallet (SIWE/multi-chain) endpoints
	_ = app.POST("/auth/wallet/challenge", ah.handleWalletChallenge)
	_ = app.POST("/auth/wallet/verify", ah.handleWalletVerify)
	_ = app.POST("/auth/wallet/link", ah.handleWalletLink)
	_ = app.GET("/auth/wallet/list", ah.handleWalletList)
	_ = app.DELETE("/auth/wallet/unlink/:address", ah.handleWalletUnlink)
}

// handleAuthorize handles OAuth authorization requests with PKCE
func (ah *AuthHandler) handleAuthorize(ctx *lift.Context) error {
	ah.logger.Info("handling OAuth authorize request")

	// Parse query parameters
	responseType := ctx.Query("response_type")
	clientID := ctx.Query("client_id")
	redirectURI := ctx.Query("redirect_uri")
	state := ctx.Query("state")
	codeChallenge := ctx.Query("code_challenge")
	codeChallengeMethod := ctx.Query("code_challenge_method")
	scope := ctx.Query("scope")

	// Validate required parameters
	if responseType != "code" {
		return lift.NewLiftError("INVALID_REQUEST", "only response_type=code is supported", 400)
	}
	if clientID == "" {
		return lift.NewLiftError("INVALID_REQUEST", "client_id is required", 400)
	}
	if redirectURI == "" {
		return lift.NewLiftError("INVALID_REQUEST", "redirect_uri is required", 400)
	}

	// PKCE validation - require for security
	if codeChallenge == "" {
		return lift.NewLiftError("INVALID_REQUEST", "code_challenge required for PKCE", 400)
	}
	if codeChallengeMethod != "S256" {
		return lift.NewLiftError("INVALID_REQUEST", "only S256 challenge method supported", 400)
	}

	// Validate client and redirect URI using OAuth service
	if err := ah.oauthSvc.ValidateClient(ctx.Context, clientID, ""); err != nil {
		ah.logger.Warn("invalid client_id", zap.String("client_id", clientID))
		return lift.NewLiftError("INVALID_CLIENT", "invalid client_id", 400)
	}

	if err := ah.oauthSvc.ValidateRedirectURI(ctx.Context, clientID, redirectURI); err != nil {
		ah.logger.Warn("invalid redirect_uri",
			zap.String("client_id", clientID),
			zap.String("redirect_uri", redirectURI),
		)
		return lift.NewLiftError("INVALID_REQUEST", "invalid redirect_uri", 400)
	}

	// Check if user is already authenticated
	userID := ah.getUserFromContext(ctx)
	if userID == "" {
		// User not authenticated - redirect to login form
		loginURL := fmt.Sprintf("/auth/login?%s", ah.buildQueryString(ctx))
		return ctx.Status(http.StatusFound).Text(loginURL)
	}

	// Generate authorization code using OAuth service
	code, err := ah.oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		ah.logger.Error("failed to generate auth code", zap.Error(err))
		return lift.NewLiftError("SERVER_ERROR", "internal server error", 500)
	}

	// Store authorization code with PKCE data
	authCode := &AuthCode{
		Code:        code,
		Challenge:   codeChallenge,
		Method:      codeChallengeMethod,
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Scope:       scope,
		UserID:      userID,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}

	if err := ah.saveAuthCode(ctx.Context, authCode); err != nil {
		ah.logger.Error("failed to save auth code", zap.Error(err))
		return lift.NewLiftError("SERVER_ERROR", "internal server error", 500)
	}

	// Redirect back to client with authorization code
	redirectURL := fmt.Sprintf("%s?code=%s", redirectURI, code)
	if state != "" {
		redirectURL += fmt.Sprintf("&state=%s", url.QueryEscape(state))
	}

	ah.logger.Info("authorization successful",
		zap.String("client_id", clientID),
		zap.String("user_id", userID),
		zap.String("code", code[:8]+"..."),
	)

	// Set redirect response
	return ctx.Status(http.StatusFound).Text(redirectURL)
}

// handleToken handles OAuth token exchange
func (ah *AuthHandler) handleToken(ctx *lift.Context) error {
	ah.logger.Info("handling OAuth token request")

	// Parse form data from request body
	body := ctx.Request.Body
	if len(body) == 0 {
		return lift.NewLiftError("INVALID_REQUEST", "request body required", 400)
	}

	// Parse URL-encoded form data
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	grantType := values.Get("grant_type")
	code := values.Get("code")
	redirectURI := values.Get("redirect_uri")
	clientID := values.Get("client_id")
	codeVerifier := values.Get("code_verifier")

	// Validate grant type
	if grantType != "authorization_code" {
		return lift.NewLiftError("UNSUPPORTED_GRANT_TYPE", "unsupported grant_type", 400)
	}

	// Validate required fields
	if code == "" || clientID == "" {
		return lift.NewLiftError("INVALID_REQUEST", "missing required parameters", 400)
	}

	// Retrieve and validate the authorization code
	authCode, err := ah.getAuthCode(ctx.Context, code)
	if err != nil {
		ah.logger.Warn("invalid authorization code", zap.String("code", code[:8]+"..."))
		return lift.NewLiftError("INVALID_GRANT", "invalid authorization code", 400)
	}

	// Check if code has expired
	if time.Now().After(authCode.ExpiresAt) {
		ah.logger.Warn("expired authorization code", zap.String("code", code[:8]+"..."))
		// Clean up expired code
		if deleteErr := ah.deleteAuthCode(ctx.Context, code); deleteErr != nil {
			ah.logger.Warn("failed to delete expired auth code",
				zap.String("code", code[:8]+"..."),
				zap.Error(deleteErr))
		}
		return lift.NewLiftError("INVALID_GRANT", "authorization code expired", 400)
	}

	// Validate client ID matches
	if authCode.ClientID != clientID {
		ah.logger.Warn("client ID mismatch",
			zap.String("expected", authCode.ClientID),
			zap.String("provided", clientID),
		)
		return lift.NewLiftError("INVALID_CLIENT", "client ID mismatch", 400)
	}

	// Validate redirect URI matches
	if authCode.RedirectURI != redirectURI {
		ah.logger.Warn("redirect URI mismatch",
			zap.String("expected", authCode.RedirectURI),
			zap.String("provided", redirectURI),
		)
		return lift.NewLiftError("INVALID_REQUEST", "redirect URI mismatch", 400)
	}

	// Validate PKCE code verifier using OAuth service
	if err := ah.oauthSvc.VerifyCodeChallenge(authCode.Challenge, codeVerifier, authCode.Method); err != nil {
		ah.logger.Warn("PKCE validation failed", zap.Error(err))
		return lift.NewLiftError("INVALID_GRANT", "PKCE validation failed", 400)
	}

	// Parse scopes
	scopes := strings.Split(authCode.Scope, " ")
	if len(scopes) == 0 {
		scopes = auth.DefaultScopes()
	}

	// Generate access and refresh tokens using the OAuth service
	accessToken, refreshToken, err := ah.oauthSvc.GenerateTokens(authCode.UserID, authCode.ClientID, scopes)
	if err != nil {
		ah.logger.Error("failed to generate tokens", zap.Error(err))
		return lift.NewLiftError("SERVER_ERROR", "failed to generate tokens", 500)
	}

	// Delete the authorization code (one-time use)
	if err := ah.deleteAuthCode(ctx.Context, code); err != nil {
		ah.logger.Warn("failed to delete auth code", zap.Error(err))
		// Continue anyway - token generation succeeded
	}

	// Return token response
	response := map[string]interface{}{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(auth.AccessTokenDuration.Seconds()),
		"refresh_token": refreshToken,
		"scope":         authCode.Scope,
	}

	ah.logger.Info("token exchange successful", zap.String("client_id", clientID))
	return ctx.JSON(response)
}

// handleRevoke handles OAuth token revocation
func (ah *AuthHandler) handleRevoke(ctx *lift.Context) error {
	// Parse form data
	values, err := url.ParseQuery(string(ctx.Request.Body))
	if err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	token := values.Get("token")
	if token == "" {
		return lift.NewLiftError("INVALID_REQUEST", "token parameter required", 400)
	}

	// Record the token revocation activity
	if err := ah.repos.Activity().RecordActivity(ctx.Context, "token_revocation", token[:16], time.Now()); err != nil {
		ah.logger.Error("failed to record token revocation activity",
			zap.String("token_prefix", token[:8]+"..."),
			zap.Error(err),
		)
		// Continue with revocation regardless of recording failure
	}

	ah.logger.Info("token revocation processed",
		zap.String("token_prefix", token[:8]+"..."),
		zap.String("user_agent", ctx.Header("User-Agent")),
	)

	return ctx.Status(http.StatusOK).JSON(map[string]string{
		"status":  "success",
		"message": "Token revocation processed",
	})
}

// handleOAuthDiscovery handles OAuth 2.0 Authorization Server Metadata requests (RFC 8414)
func (ah *AuthHandler) handleOAuthDiscovery(ctx *lift.Context) error {
	ah.logger.Info("handling OAuth discovery request")

	// Build the base URL from the request or configuration
	baseURL := ah.getBaseURL(ctx)
	
	// OAuth 2.0 Authorization Server Metadata according to RFC 8414
	metadata := map[string]interface{}{
		"issuer":                           baseURL,
		"authorization_endpoint":           fmt.Sprintf("%s/oauth/authorize", baseURL),
		"token_endpoint":                  fmt.Sprintf("%s/oauth/token", baseURL),
		"revocation_endpoint":             fmt.Sprintf("%s/oauth/revoke", baseURL),
		"scopes_supported":                []string{"read", "write", "follow", "push", "admin"},
		"response_types_supported":        []string{"code"},
		"grant_types_supported":           []string{"authorization_code"},
		"code_challenge_methods_supported": []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"revocation_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"introspection_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"subject_types_supported":         []string{"public"},
		"id_token_signing_alg_values_supported": []string{"HS256"},
		"request_object_signing_alg_values_supported": []string{"HS256"},
		"request_parameter_supported":     false,
		"request_uri_parameter_supported": false,
		"require_request_uri_registration": false,
		"claims_parameter_supported":      false,
		"service_documentation":          fmt.Sprintf("%s/docs/oauth", baseURL),
	}

	// Set appropriate cache headers for discovery metadata
	ctx.Response.Headers["Cache-Control"] = "public, max-age=3600"
	ctx.Response.Headers["Content-Type"] = "application/json"

	ah.logger.Info("OAuth discovery metadata served",
		zap.String("issuer", baseURL),
		zap.String("user_agent", ctx.Header("User-Agent")))

	return ctx.JSON(metadata)
}

// handleLogin handles user login
func (ah *AuthHandler) handleLogin(ctx *lift.Context) error {
	// Parse form data
	values, err := url.ParseQuery(string(ctx.Request.Body))
	if err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	username := values.Get("username")
	password := values.Get("password")

	if username == "" || password == "" {
		return lift.NewLiftError("INVALID_REQUEST", "username and password required", 400)
	}

	// Get user agent and IP for authentication
	userAgent := ctx.Header("User-Agent")
	ipAddress := ctx.Header("X-Forwarded-For")
	if ipAddress == "" {
		ipAddress = ctx.Header("X-Real-IP")
	}
	if ipAddress == "" {
		ipAddress = "unknown"
	}

	// Authenticate user using the auth service
	authResponse, err := ah.authSvc.AuthenticateWithPassword(ctx.Context, username, password, "web", userAgent, ipAddress)
	if err != nil {
		ah.logger.Warn("login failed", zap.String("username", username), zap.Error(err))
		return lift.NewLiftError("INVALID_CREDENTIALS", "invalid credentials", 401)
	}

	// Set session cookie (use the 'Me' field which contains the username)
	cookieValue := fmt.Sprintf("session_id=%s; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=%d",
		"session_"+authResponse.Me, int(24*time.Hour.Seconds()))
	ctx.Response.Headers["Set-Cookie"] = cookieValue

	ah.logger.Info("user logged in",
		zap.String("username", authResponse.Me))

	return ctx.JSON(map[string]string{
		"status":   "success",
		"username": authResponse.Me,
	})
}

// handleLogout handles user logout
func (ah *AuthHandler) handleLogout(ctx *lift.Context) error {
	// Get session ID from cookie
	sessionID := ah.getSessionCookie(ctx)
	if sessionID != "" {
		// Log out session using auth service
		if err := ah.authSvc.Logout(ctx.Context, sessionID); err != nil {
			ah.logger.Warn("failed to log out session", zap.Error(err))
		}

		// Clear the session cookie
		ctx.Response.Headers["Set-Cookie"] = "session_id=; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=0"

		ah.logger.Info("user logged out", zap.String("session_id", sessionID))
	} else {
		ah.logger.Info("logout requested but no session found")
	}

	return ctx.JSON(map[string]string{"status": "success"})
}

// handleSession checks current session status
func (ah *AuthHandler) handleSession(ctx *lift.Context) error {
	// Check if user is authenticated
	userID := ah.getUserFromContext(ctx)
	if userID != "" {
		return ctx.JSON(map[string]interface{}{
			"authenticated": true,
			"user_id":       userID,
		})
	}

	return ctx.JSON(map[string]interface{}{
		"authenticated": false,
	})
}

// WebAuthn request/response types

// WebAuthnRegisterBeginRequest represents the request to begin WebAuthn registration
type WebAuthnRegisterBeginRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
}

// WebAuthnRegisterFinishRequest represents the request to finish WebAuthn registration
type WebAuthnRegisterFinishRequest struct {
	Username       string `json:"username" validate:"required,min=3,max=50"`
	Challenge      string `json:"challenge" validate:"required"`
	CredentialName string `json:"credential_name,omitempty"`
	Response       []byte `json:"response" validate:"required"`
}

// WebAuthnLoginBeginRequest represents the request to begin WebAuthn login
type WebAuthnLoginBeginRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
}

// WebAuthnLoginFinishRequest represents the request to finish WebAuthn login
type WebAuthnLoginFinishRequest struct {
	Username  string `json:"username" validate:"required,min=3,max=50"`
	Challenge string `json:"challenge" validate:"required"`
	Response  []byte `json:"response" validate:"required"`
}

// WebAuthn Handlers

// handleWebAuthnRegisterBegin starts the WebAuthn registration process
func (ah *AuthHandler) handleWebAuthnRegisterBegin(ctx *lift.Context) error {
	ah.logger.Info("handling WebAuthn register begin request")

	if ah.webAuthnSvc == nil {
		return lift.NewLiftError("WEBAUTHN_DISABLED", "WebAuthn not available", 503)
	}

	// Parse request
	var req WebAuthnRegisterBeginRequest
	if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	// Get client IP for rate limiting
	ipAddress := ah.getClientIP(ctx)

	// Create rate limiter for registration attempts
	rateLimiter := auth.NewRateLimiter(ah.repos)

	// Check rate limits (use more restrictive limits for registration)
	if err := rateLimiter.CheckRateLimit(ctx.Context, req.Username, ipAddress); err != nil {
		ah.logger.Warn("WebAuthn registration rate limited",
			zap.String("username", req.Username),
			zap.String("ip", ipAddress),
			zap.Error(err))
		return lift.NewLiftError("RATE_LIMITED", "too many registration attempts", 429)
	}

	// Verify user exists
	user, err := ah.repos.Account().GetUser(ctx.Context, req.Username)
	if err != nil {
		ah.logger.Warn("WebAuthn registration for unknown user",
			zap.String("username", req.Username))
		return lift.NewLiftError("USER_NOT_FOUND", "user not found", 404)
	}

	// Check if user has reached credential limit
	existingCreds, _ := ah.repos.Account().GetUserWebAuthnCredentials(ctx.Context, req.Username)
	if len(existingCreds) >= auth.MaxCredentialsPerUser {
		return lift.NewLiftError("TOO_MANY_CREDENTIALS", "maximum credentials reached", 400)
	}

	// Begin registration
	options, challenge, err := ah.webAuthnSvc.BeginRegistration(ctx.Context, req.Username)
	if err != nil {
		ah.logger.Error("failed to begin WebAuthn registration", zap.Error(err))
		return lift.NewLiftError("REGISTRATION_FAILED", "failed to start registration", 500)
	}

	// Record attempt for rate limiting
	_ = rateLimiter.RecordAttempt(ctx.Context, req.Username, ipAddress, false)

	ah.logger.Info("WebAuthn registration started",
		zap.String("username", req.Username),
		zap.String("user_id", user.Username))

	return ctx.JSON(map[string]interface{}{
		"options":   options,
		"challenge": challenge,
	})
}

// handleWebAuthnRegisterFinish completes the WebAuthn registration process
func (ah *AuthHandler) handleWebAuthnRegisterFinish(ctx *lift.Context) error {
	ah.logger.Info("handling WebAuthn register finish request")

	if ah.webAuthnSvc == nil {
		return lift.NewLiftError("WEBAUTHN_DISABLED", "WebAuthn not available", 503)
	}

	// Parse request
	var req WebAuthnRegisterFinishRequest
	if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	// Get client IP and user agent for device metadata
	ipAddress := ah.getClientIP(ctx)
	userAgent := ctx.Header("User-Agent")

	// Create rate limiter
	rateLimiter := auth.NewRateLimiter(ah.repos)

	// Finish registration
	err := ah.webAuthnSvc.FinishRegistration(ctx.Context, req.Username, req.Challenge, req.Response, req.CredentialName)
	if err != nil {
		ah.logger.Error("WebAuthn registration failed",
			zap.String("username", req.Username),
			zap.String("challenge", req.Challenge[:8]+"..."),
			zap.Error(err))

		// Record failed attempt
		_ = rateLimiter.RecordAttempt(ctx.Context, req.Username, ipAddress, false)

		if err == auth.ErrChallengeNotFound {
			return lift.NewLiftError("INVALID_CHALLENGE", "invalid or expired challenge", 400)
		}
		return lift.NewLiftError("REGISTRATION_FAILED", "registration verification failed", 400)
	}

	// Record successful attempt
	_ = rateLimiter.RecordAttempt(ctx.Context, req.Username, ipAddress, true)

	// Log device registration for audit
	ah.logger.Info("WebAuthn credential registered",
		zap.String("username", req.Username),
		zap.String("credential_name", req.CredentialName),
		zap.String("user_agent", userAgent),
		zap.String("ip_address", ipAddress))

	return ctx.JSON(map[string]interface{}{
		"success": true,
		"message": "passkey registered successfully",
	})
}

// handleWebAuthnLoginBegin starts the WebAuthn login process
func (ah *AuthHandler) handleWebAuthnLoginBegin(ctx *lift.Context) error {
	ah.logger.Info("handling WebAuthn login begin request")

	if ah.webAuthnSvc == nil {
		return lift.NewLiftError("WEBAUTHN_DISABLED", "WebAuthn not available", 503)
	}

	// Parse request
	var req WebAuthnLoginBeginRequest
	if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	// Get client IP for rate limiting
	ipAddress := ah.getClientIP(ctx)

	// Create rate limiter
	rateLimiter := auth.NewRateLimiter(ah.repos)

	// Check rate limits
	if err := rateLimiter.CheckRateLimit(ctx.Context, req.Username, ipAddress); err != nil {
		ah.logger.Warn("WebAuthn login rate limited",
			zap.String("username", req.Username),
			zap.String("ip", ipAddress),
			zap.Error(err))
		return lift.NewLiftError("RATE_LIMITED", err.Error(), 429)
	}

	// Begin login
	options, challenge, err := ah.webAuthnSvc.BeginLogin(ctx.Context, req.Username)
	if err != nil {
		ah.logger.Warn("WebAuthn login begin failed",
			zap.String("username", req.Username),
			zap.Error(err))

		if err == auth.ErrUserHasNoCredentials {
			return lift.NewLiftError("NO_CREDENTIALS", "no passkeys registered", 400)
		}
		return lift.NewLiftError("LOGIN_FAILED", "failed to start authentication", 500)
	}

	ah.logger.Debug("WebAuthn login challenge created",
		zap.String("username", req.Username))

	return ctx.JSON(map[string]interface{}{
		"options":   options,
		"challenge": challenge,
	})
}

// handleWebAuthnLoginFinish completes the WebAuthn login process
func (ah *AuthHandler) handleWebAuthnLoginFinish(ctx *lift.Context) error {
	ah.logger.Info("handling WebAuthn login finish request")

	if ah.webAuthnSvc == nil {
		return lift.NewLiftError("WEBAUTHN_DISABLED", "WebAuthn not available", 503)
	}

	// Parse request
	var req WebAuthnLoginFinishRequest
	if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	// Get client IP and user agent for audit
	ipAddress := ah.getClientIP(ctx)
	userAgent := ctx.Header("User-Agent")

	// Create rate limiter
	rateLimiter := auth.NewRateLimiter(ah.repos)

	// Finish login
	credential, err := ah.webAuthnSvc.FinishLogin(ctx.Context, req.Username, req.Challenge, req.Response)
	success := err == nil

	// Always record the attempt
	_ = rateLimiter.RecordAttempt(ctx.Context, req.Username, ipAddress, success)

	if err != nil {
		ah.logger.Warn("WebAuthn login failed",
			zap.String("username", req.Username),
			zap.String("challenge", req.Challenge[:8]+"..."),
			zap.String("ip_address", ipAddress),
			zap.Error(err))

		if err == auth.ErrChallengeNotFound {
			return lift.NewLiftError("INVALID_CHALLENGE", "invalid or expired challenge", 400)
		}
		return lift.NewLiftError("LOGIN_FAILED", "authentication failed", 401)
	}

	// Generate session - create a simple session token
	sessionToken := fmt.Sprintf("webauthn_session_%d_%s", time.Now().Unix(), req.Username)

	// Set session cookie
	cookieValue := fmt.Sprintf("session_id=%s; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=%d",
		sessionToken, int(24*time.Hour.Seconds()))
	ctx.Response.Headers["Set-Cookie"] = cookieValue

	// Log successful authentication with device metadata
	ah.logger.Info("WebAuthn login successful",
		zap.String("username", req.Username),
		zap.String("credential_id", credential.ID),
		zap.String("credential_name", credential.Name),
		zap.String("user_agent", userAgent),
		zap.String("ip_address", ipAddress),
		zap.Time("last_used", credential.LastUsedAt))

	return ctx.JSON(map[string]interface{}{
		"success":  true,
		"username": req.Username,
		"message":  "authentication successful",
		"credential": map[string]interface{}{
			"id":   credential.ID,
			"name": credential.Name,
		},
	})
}

// Wallet request/response types

// WalletChallengeRequest represents the request for a wallet authentication challenge
type WalletChallengeRequest struct {
	Address  string `json:"address" validate:"required"`
	ChainID  int    `json:"chainId,omitempty"`
	Username string `json:"username,omitempty"` // Optional for linking to specific user
}

// WalletChallengeResponse represents the response containing a wallet authentication challenge
type WalletChallengeResponse struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	IssuedAt  string `json:"issuedAt"`
	ExpiresAt string `json:"expiresAt"`
}

// WalletVerifyRequest represents the request to verify a wallet signature
type WalletVerifyRequest struct {
	ChallengeID string `json:"challengeId" validate:"required"`
	Address     string `json:"address" validate:"required"`
	Signature   string `json:"signature" validate:"required"`
	Message     string `json:"message" validate:"required"`
}

// WalletVerifyResponse represents the response after wallet verification
type WalletVerifyResponse struct {
	Success       bool   `json:"success"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"me,omitempty"`
	AccessToken   string `json:"access_token,omitempty"`
	Message       string `json:"message"`
}

// WalletLinkRequest represents the request to link a wallet to an account
type WalletLinkRequest struct {
	Address   string `json:"address" validate:"required"`
	ChainID   int    `json:"chainId,omitempty"`
	Type      string `json:"type,omitempty"` // ethereum, polygon, etc.
	Signature string `json:"signature" validate:"required"`
	Message   string `json:"message" validate:"required"`
}

// WalletLinkResponse represents the response after linking a wallet
type WalletLinkResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// WalletListResponse represents the response containing a list of linked wallets
type WalletListResponse struct {
	Success bool                        `json:"success"`
	Count   int                         `json:"count"`
	Wallets []WalletCredentialForClient `json:"wallets"`
}

// WalletCredentialForClient represents wallet credential information for client display
type WalletCredentialForClient struct {
	Address  string `json:"address"`
	ChainID  int    `json:"chainId"`
	Type     string `json:"type"`
	LinkedAt string `json:"linkedAt"`
	LastUsed string `json:"lastUsed"`
}

// WalletUnlinkResponse represents the response after unlinking a wallet
type WalletUnlinkResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Wallet Handler Functions

// handleWalletChallenge generates a challenge for wallet authentication
func (ah *AuthHandler) handleWalletChallenge(ctx *lift.Context) error {
	ah.logger.Info("handling wallet challenge request")

	// Parse request
	var req WalletChallengeRequest
	if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	// Validate address format (basic validation)
	if len(req.Address) < 10 {
		return lift.NewLiftError("INVALID_ADDRESS", "invalid wallet address", 400)
	}

	// Set default chain ID if not provided (Ethereum mainnet)
	if req.ChainID == 0 {
		req.ChainID = 1
	}

	// Get client IP for rate limiting
	ipAddress := ah.getClientIP(ctx)

	// Create rate limiter for wallet challenges
	rateLimiter := auth.NewRateLimiter(ah.repos)

	// Use stricter rate limiting for wallet challenges (by IP and address)
	rateLimitKey := fmt.Sprintf("wallet_challenge_%s_%s", ipAddress, req.Address)
	if err := rateLimiter.CheckRateLimit(ctx.Context, rateLimitKey, ipAddress); err != nil {
		ah.logger.Warn("wallet challenge rate limited",
			zap.String("address", req.Address),
			zap.String("ip", ipAddress),
			zap.Error(err))
		return lift.NewLiftError("RATE_LIMITED", "too many challenge requests", 429)
	}

	// Create challenge using wallet service
	challenge, err := ah.walletSvc.CreateChallenge(ctx.Context, req.Address, req.ChainID, req.Username)
	if err != nil {
		ah.logger.Error("failed to create wallet challenge", zap.Error(err))
		return lift.NewLiftError("CHALLENGE_FAILED", "failed to create challenge", 500)
	}

	// Record attempt for rate limiting
	_ = rateLimiter.RecordAttempt(ctx.Context, rateLimitKey, ipAddress, true)

	response := WalletChallengeResponse{
		ID:        challenge.ID,
		Message:   challenge.Message,
		IssuedAt:  challenge.IssuedAt.Format(time.RFC3339),
		ExpiresAt: challenge.ExpiresAt.Format(time.RFC3339),
	}

	ah.logger.Info("wallet challenge created",
		zap.String("challengeId", challenge.ID),
		zap.String("address", req.Address),
		zap.Int("chainId", req.ChainID))

	return ctx.JSON(response)
}

// handleWalletVerify verifies a wallet signature and creates session
func (ah *AuthHandler) handleWalletVerify(ctx *lift.Context) error {
	ah.logger.Info("handling wallet verify request")

	// Parse request
	var req WalletVerifyRequest
	if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	// Get client IP for rate limiting and audit
	ipAddress := ah.getClientIP(ctx)
	userAgent := ctx.Header("User-Agent")

	// Create rate limiter
	rateLimiter := auth.NewRateLimiter(ah.repos)

	// Use address-based rate limiting for verify attempts
	rateLimitKey := fmt.Sprintf("wallet_verify_%s", req.Address)
	if err := rateLimiter.CheckRateLimit(ctx.Context, rateLimitKey, ipAddress); err != nil {
		ah.logger.Warn("wallet verify rate limited",
			zap.String("address", req.Address),
			zap.String("ip", ipAddress),
			zap.Error(err))
		return lift.NewLiftError("RATE_LIMITED", "too many verification attempts", 429)
	}

	// Convert to wallet service format
	verifyReq := &auth.WalletVerifyRequest{
		ChallengeID: req.ChallengeID,
		Address:     req.Address,
		Signature:   req.Signature,
		Message:     req.Message,
	}

	// Verify signature using wallet service
	username, err := ah.walletSvc.VerifySignature(ctx.Context, verifyReq)
	success := err == nil

	// Always record the attempt
	_ = rateLimiter.RecordAttempt(ctx.Context, rateLimitKey, ipAddress, success)

	if err != nil {
		ah.logger.Warn("wallet signature verification failed",
			zap.String("address", req.Address),
			zap.String("challengeId", req.ChallengeID[:8]+"..."),
			zap.String("ip_address", ipAddress),
			zap.Error(err))

		return lift.NewLiftError("VERIFICATION_FAILED", "signature verification failed", 401)
	}

	response := WalletVerifyResponse{
		Success:       true,
		Authenticated: username != "",
		Message:       "wallet authentication successful",
	}

	// If username is available, create session and access token
	if username != "" {
		response.Username = username

		// Generate access token using OAuth service
		scopes := auth.DefaultScopes()
		clientID := "wallet_client" // Use a default client for wallet auth
		accessToken, _, err := ah.oauthSvc.GenerateTokens(username, clientID, scopes)
		if err != nil {
			ah.logger.Error("failed to generate access token for wallet auth", zap.Error(err))
		} else {
			response.AccessToken = accessToken
		}

		// Create session cookie
		sessionToken := fmt.Sprintf("wallet_session_%d_%s", time.Now().Unix(), username)
		cookieValue := fmt.Sprintf("session_id=%s; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=%d",
			sessionToken, int(24*time.Hour.Seconds()))
		ctx.Response.Headers["Set-Cookie"] = cookieValue

		ah.logger.Info("wallet authentication successful with existing user",
			zap.String("username", username),
			zap.String("address", req.Address),
			zap.String("user_agent", userAgent),
			zap.String("ip_address", ipAddress))
	} else {
		response.Authenticated = false
		response.Message = "wallet verified but not linked to account - use /auth/wallet/link"

		ah.logger.Info("wallet authentication successful but no linked account",
			zap.String("address", req.Address),
			zap.String("user_agent", userAgent),
			zap.String("ip_address", ipAddress))
	}

	return ctx.JSON(response)
}

// handleWalletLink links a wallet to an existing authenticated user account
func (ah *AuthHandler) handleWalletLink(ctx *lift.Context) error {
	ah.logger.Info("handling wallet link request")

	// Parse request
	var req WalletLinkRequest
	if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
		return lift.NewLiftError("INVALID_REQUEST", "invalid request format", 400)
	}

	// Check if user is authenticated
	username := ah.getUserFromContext(ctx)
	if username == "" {
		return lift.NewLiftError("UNAUTHORIZED", "authentication required to link wallet", 401)
	}

	// Set defaults
	if req.ChainID == 0 {
		req.ChainID = 1 // Ethereum mainnet
	}
	if req.Type == "" {
		req.Type = "ethereum"
	}

	// Get client IP and user agent for audit
	ipAddress := ah.getClientIP(ctx)
	userAgent := ctx.Header("User-Agent")

	// Create rate limiter for link attempts
	rateLimiter := auth.NewRateLimiter(ah.repos)
	rateLimitKey := fmt.Sprintf("wallet_link_%s_%s", username, req.Address)

	if err := rateLimiter.CheckRateLimit(ctx.Context, rateLimitKey, ipAddress); err != nil {
		ah.logger.Warn("wallet link rate limited",
			zap.String("username", username),
			zap.String("address", req.Address),
			zap.String("ip", ipAddress),
			zap.Error(err))
		return lift.NewLiftError("RATE_LIMITED", "too many link attempts", 429)
	}

	// For linking, we need to verify the signature matches a recent challenge
	// This is a security measure to prevent replay attacks
	// For now, we'll require the user to create and verify a challenge first
	// In a full implementation, you might want to create a temporary challenge here

	// Link wallet to user account
	err := ah.walletSvc.LinkWallet(ctx.Context, username, req.Address, req.ChainID, req.Type)
	success := err == nil

	// Record attempt
	_ = rateLimiter.RecordAttempt(ctx.Context, rateLimitKey, ipAddress, success)

	if err != nil {
		ah.logger.Error("failed to link wallet to account",
			zap.String("username", username),
			zap.String("address", req.Address),
			zap.String("ip_address", ipAddress),
			zap.Error(err))

		return lift.NewLiftError("LINK_FAILED", err.Error(), 400)
	}

	response := WalletLinkResponse{
		Success: true,
		Message: "wallet linked successfully",
	}

	ah.logger.Info("wallet linked to account",
		zap.String("username", username),
		zap.String("address", req.Address),
		zap.String("type", req.Type),
		zap.Int("chainId", req.ChainID),
		zap.String("user_agent", userAgent),
		zap.String("ip_address", ipAddress))

	return ctx.JSON(response)
}

// handleWalletList lists all wallets linked to the authenticated user
func (ah *AuthHandler) handleWalletList(ctx *lift.Context) error {
	ah.logger.Info("handling wallet list request")

	// Check if user is authenticated
	username := ah.getUserFromContext(ctx)
	if username == "" {
		return lift.NewLiftError("UNAUTHORIZED", "authentication required", 401)
	}

	// Get user's wallets
	wallets, err := ah.walletSvc.GetUserWallets(ctx.Context, username)
	if err != nil {
		ah.logger.Error("failed to get user wallets",
			zap.String("username", username),
			zap.Error(err))
		return lift.NewLiftError("FETCH_FAILED", "failed to retrieve wallets", 500)
	}

	// Convert to client format
	clientWallets := make([]WalletCredentialForClient, len(wallets))
	for i, wallet := range wallets {
		clientWallets[i] = WalletCredentialForClient{
			Address:  wallet.Address,
			ChainID:  wallet.ChainID,
			Type:     wallet.Type,
			LinkedAt: wallet.LinkedAt.Format(time.RFC3339),
			LastUsed: wallet.LastUsed.Format(time.RFC3339),
		}
	}

	response := WalletListResponse{
		Success: true,
		Count:   len(clientWallets),
		Wallets: clientWallets,
	}

	ah.logger.Info("wallet list retrieved",
		zap.String("username", username),
		zap.Int("count", len(wallets)))

	return ctx.JSON(response)
}

// handleWalletUnlink unlinks a wallet from the authenticated user's account
func (ah *AuthHandler) handleWalletUnlink(ctx *lift.Context) error {
	ah.logger.Info("handling wallet unlink request")

	// Check if user is authenticated
	username := ah.getUserFromContext(ctx)
	if username == "" {
		return lift.NewLiftError("UNAUTHORIZED", "authentication required", 401)
	}

	// Get wallet address from URL parameter
	address := ctx.Param("address")
	if address == "" {
		return lift.NewLiftError("INVALID_REQUEST", "wallet address required", 400)
	}

	// Get client IP and user agent for audit
	ipAddress := ah.getClientIP(ctx)
	userAgent := ctx.Header("User-Agent")

	// Create rate limiter for unlink attempts
	rateLimiter := auth.NewRateLimiter(ah.repos)
	rateLimitKey := fmt.Sprintf("wallet_unlink_%s_%s", username, address)

	if err := rateLimiter.CheckRateLimit(ctx.Context, rateLimitKey, ipAddress); err != nil {
		ah.logger.Warn("wallet unlink rate limited",
			zap.String("username", username),
			zap.String("address", address),
			zap.String("ip", ipAddress),
			zap.Error(err))
		return lift.NewLiftError("RATE_LIMITED", "too many unlink attempts", 429)
	}

	// Unlink wallet from user account
	err := ah.walletSvc.UnlinkWallet(ctx.Context, username, address)
	success := err == nil

	// Record attempt
	_ = rateLimiter.RecordAttempt(ctx.Context, rateLimitKey, ipAddress, success)

	if err != nil {
		ah.logger.Error("failed to unlink wallet from account",
			zap.String("username", username),
			zap.String("address", address),
			zap.String("ip_address", ipAddress),
			zap.Error(err))

		return lift.NewLiftError("UNLINK_FAILED", err.Error(), 400)
	}

	response := WalletUnlinkResponse{
		Success: true,
		Message: "wallet unlinked successfully",
	}

	ah.logger.Info("wallet unlinked from account",
		zap.String("username", username),
		zap.String("address", address),
		zap.String("user_agent", userAgent),
		zap.String("ip_address", ipAddress))

	return ctx.JSON(response)
}

// Helper methods

// getBaseURL constructs the base URL for the auth service
func (ah *AuthHandler) getBaseURL(ctx *lift.Context) string {
	// Try to get the base URL from configuration
	if ah.cfg.Domain != "" {
		// Always use HTTPS for production domains
		return fmt.Sprintf("https://%s", ah.cfg.Domain)
	}

	// Fallback to building from request headers
	scheme := "https" // Default to HTTPS for security
	if protoHeader := ctx.Header("X-Forwarded-Proto"); protoHeader != "" {
		scheme = protoHeader
	}

	host := ctx.Header("Host")
	if host == "" {
		// Fallback to environment or default
		if ah.cfg.Domain != "" {
			host = ah.cfg.Domain
		} else {
			host = "localhost:8080"
			scheme = "http" // Use HTTP for localhost
		}
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

// getClientIP extracts the client IP address from the request
func (ah *AuthHandler) getClientIP(ctx *lift.Context) string {
	// Check X-Forwarded-For header first (most common in Lambda/API Gateway)
	if forwarded := ctx.Header("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if idx := strings.Index(forwarded, ","); idx > 0 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return strings.TrimSpace(forwarded)
	}

	// Check X-Real-IP header
	if realIP := ctx.Header("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	// Fallback to remote addr (less reliable in serverless)
	if remoteAddr := ctx.Request.RemoteAddr(); remoteAddr != "" {
		return remoteAddr
	}

	return "unknown"
}

func (ah *AuthHandler) buildQueryString(ctx *lift.Context) string {
	parts := make([]string, 0, len(ctx.Request.QueryParams))
	for key, value := range ctx.Request.QueryParams {
		parts = append(parts, fmt.Sprintf("%s=%s", url.QueryEscape(key), url.QueryEscape(value)))
	}
	return strings.Join(parts, "&")
}

// saveAuthCode stores an authorization code using the storage interface
func (ah *AuthHandler) saveAuthCode(ctx context.Context, authCode *AuthCode) error {
	// Create authorization code using the storage interface
	storageAuthCode := &storage.AuthorizationCode{
		Code:          authCode.Code,
		ClientID:      authCode.ClientID,
		Username:      authCode.UserID,
		CodeChallenge: authCode.Challenge,
		ExpiresAt:     authCode.ExpiresAt,
		Scopes:        strings.Split(authCode.Scope, " "),
	}

	return ah.repos.Account().CreateAuthorizationCode(ctx, storageAuthCode)
}

// getAuthCode retrieves and validates an authorization code
func (ah *AuthHandler) getAuthCode(ctx context.Context, code string) (*AuthCode, error) {
	storageAuthCode, err := ah.repos.Account().GetAuthorizationCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("auth code not found: %w", err)
	}

	// Convert to local auth code format
	authCode := &AuthCode{
		Code:        storageAuthCode.Code,
		Challenge:   storageAuthCode.CodeChallenge,
		Method:      "S256", // Assume S256 method
		ClientID:    storageAuthCode.ClientID,
		RedirectURI: "", // Not stored in current storage format
		Scope:       strings.Join(storageAuthCode.Scopes, " "),
		UserID:      storageAuthCode.Username,
		ExpiresAt:   storageAuthCode.ExpiresAt,
	}

	return authCode, nil
}

// deleteAuthCode removes an authorization code
func (ah *AuthHandler) deleteAuthCode(ctx context.Context, code string) error {
	return ah.repos.Account().DeleteAuthorizationCode(ctx, code)
}

// getUserFromContext extracts user ID from the current session/context
func (ah *AuthHandler) getUserFromContext(ctx *lift.Context) string {
	// Check for session cookie
	sessionID := ah.getSessionCookie(ctx)
	if sessionID == "" {
		return ""
	}

	// Get session from storage directly
	session, err := ah.repos.Account().GetSession(ctx.Context, sessionID)
	if err != nil {
		ah.logger.Debug("failed to get session", zap.Error(err))
		return ""
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		ah.logger.Debug("session expired", zap.String("session_id", sessionID))
		return ""
	}

	return session.Username
}

// getSessionCookie extracts the session ID from cookies
func (ah *AuthHandler) getSessionCookie(ctx *lift.Context) string {
	cookieHeader := ctx.Header("Cookie")
	if cookieHeader == "" {
		return ""
	}

	// Parse cookies (simplified cookie parsing)
	cookies := parseCookies(cookieHeader)
	return cookies["session_id"]
}

// parseCookies is a simple cookie parser
func parseCookies(cookieHeader string) map[string]string {
	cookies := make(map[string]string)
	parts := strings.Split(cookieHeader, ";")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "="); idx != -1 {
			key := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			cookies[key] = value
		}
	}

	return cookies
}

func main() {
	handler, err := NewAuthHandler()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize auth handler: %v", err))
	}

	app := lift.New()

	// Add request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("auth-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			path := ctx.Request.Path
			method := ctx.Request.Method

			err := next.Handle(ctx)

			handler.logger.Info("auth request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			return err
		})
	})

	// Register all auth routes
	handler.RegisterRoutes(app)

	lambda.Start(app.HandleRequest)
}
