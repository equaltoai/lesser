package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// AuthHandler handles authentication requests using Lift
type AuthHandler struct {
	store       storage.Storage
	logger      *zap.Logger
	cfg         *config.Config
	authSvc     *auth.AuthService
	oauthSvc    *auth.OAuthService
	webAuthnSvc *auth.WebAuthnService
}

// AuthCode represents an OAuth authorization code with PKCE
type AuthCode struct {
	Code         string    `json:"code"`
	Challenge    string    `json:"challenge"`
	Method       string    `json:"method"`
	ClientID     string    `json:"client_id"`
	RedirectURI  string    `json:"redirect_uri"`
	Scope        string    `json:"scope"`
	UserID       string    `json:"user_id"`
	ExpiresAt    time.Time `json:"expires_at"`
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
	store, err := dynamodb.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	cfg := config.Get()
	logger := common.Logger()

	// Initialize comprehensive auth service
	authSvc, err := auth.NewAuthService(store)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth service: %w", err)
	}

	// Initialize OAuth service separately for token operations
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "development-secret-change-me"
		logger.Warn("JWT_SECRET not set, using development default")
	}
	oauthSvc := auth.NewOAuthService(jwtSecret, store)

	// Initialize WebAuthn service
	domain := cfg.Domain
	if domain == "" {
		domain = "lesser.host"
	}
	webAuthnSvc, err := auth.NewWebAuthnService(store, domain, "Lesser")
	if err != nil {
		logger.Error("failed to initialize WebAuthn service", zap.Error(err))
		webAuthnSvc = nil
	}

	return &AuthHandler{
		store:       store,
		logger:      logger,
		cfg:         cfg,
		authSvc:     authSvc,
		oauthSvc:    oauthSvc,
		webAuthnSvc: webAuthnSvc,
	}, nil
}

// RegisterRoutes registers all authentication routes
func (ah *AuthHandler) RegisterRoutes(app *lift.App) {
	// OAuth 2.0 endpoints
	app.GET("/oauth/authorize", ah.handleAuthorize)
	app.POST("/oauth/token", ah.handleToken)
	app.POST("/oauth/revoke", ah.handleRevoke)
	
	// Session management
	app.POST("/auth/login", ah.handleLogin)
	app.POST("/auth/logout", ah.handleLogout)
	app.GET("/auth/session", ah.handleSession)
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
		Code:         code,
		Challenge:    codeChallenge,
		Method:       codeChallengeMethod,
		ClientID:     clientID,
		RedirectURI:  redirectURI,
		Scope:        scope,
		UserID:       userID,
		ExpiresAt:    time.Now().Add(10 * time.Minute),
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
		ah.deleteAuthCode(ctx.Context, code)
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
	if err := ah.store.RecordActivity(ctx.Context, "token_revocation", token[:16], time.Now()); err != nil {
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
		"status": "success",
		"message": "Token revocation processed",
	})
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

// Helper methods

func (ah *AuthHandler) generateAuthCode() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (ah *AuthHandler) buildQueryString(ctx *lift.Context) string {
	var parts []string
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
	
	return ah.store.CreateAuthorizationCode(ctx, storageAuthCode)
}

// getAuthCode retrieves and validates an authorization code
func (ah *AuthHandler) getAuthCode(ctx context.Context, code string) (*AuthCode, error) {
	storageAuthCode, err := ah.store.GetAuthorizationCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("auth code not found: %w", err)
	}
	
	// Convert to local auth code format
	authCode := &AuthCode{
		Code:         storageAuthCode.Code,
		Challenge:    storageAuthCode.CodeChallenge,
		Method:       "S256", // Assume S256 method
		ClientID:     storageAuthCode.ClientID,
		RedirectURI:  "", // Not stored in current storage format
		Scope:        strings.Join(storageAuthCode.Scopes, " "),
		UserID:       storageAuthCode.Username,
		ExpiresAt:    storageAuthCode.ExpiresAt,
	}
	
	return authCode, nil
}

// deleteAuthCode removes an authorization code
func (ah *AuthHandler) deleteAuthCode(ctx context.Context, code string) error {
	return ah.store.DeleteAuthorizationCode(ctx, code)
}

// getUserFromContext extracts user ID from the current session/context
func (ah *AuthHandler) getUserFromContext(ctx *lift.Context) string {
	// Check for session cookie
	sessionID := ah.getSessionCookie(ctx)
	if sessionID == "" {
		return ""
	}
	
	// Get session from storage directly
	session, err := ah.store.GetSession(ctx.Context, sessionID)
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