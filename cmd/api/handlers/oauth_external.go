package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/auth/providers"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// OAuthState represents the state stored during OAuth flow
type OAuthState struct {
	State       string    `json:"state"`
	Provider    string    `json:"provider"`
	RedirectURI string    `json:"redirect_uri"`
	Username    string    `json:"username,omitempty"` // For linking existing account
	CreatedAt   time.Time `json:"created_at"`
}

// HandleOAuthProviderAuthorize initiates OAuth flow with external provider
// GET /oauth/{provider}/authorize
func (h *Handler) HandleOAuthProviderAuthorize(ctx context.Context, request events.APIGatewayV2HTTPRequest, provider string) (*events.APIGatewayV2HTTPResponse, error) {
	// Generate state token
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return common.InternalServerError(err), nil
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Get redirect URI from query params or use default
	redirectURI := request.QueryStringParameters["redirect_uri"]
	if redirectURI == "" {
		// Default to our callback endpoint
		redirectURI = fmt.Sprintf("%s/oauth/%s/callback", h.cfg.BaseURL(), provider)
	}

	// Check if user is linking account (authenticated)
	username := ""
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			authService, _ := auth.NewAuthService(h.store)
			claims, err := authService.ValidateAccessToken(token)
			if err == nil {
				username = claims.Username
			}
		}
	}

	// Store state in DynamoDB
	oauthState := &storage.OAuthState{
		State:       state,
		Provider:    provider,
		RedirectURI: redirectURI,
		Username:    username,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(10 * time.Minute), // 10 minute expiration
	}

	if err := h.store.StoreOAuthState(ctx, state, oauthState); err != nil {
		h.logger.Error("failed to store OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to initialize OAuth flow")), nil
	}

	h.logger.Info("stored OAuth state",
		zap.String("state", state),
		zap.String("provider", provider),
		zap.String("username", username))

	// Get provider
	p := h.getProvider(provider)
	if p == nil {
		return common.BadRequest(fmt.Errorf("unknown provider: %s", provider)), nil
	}

	// Get authorization URL
	authURL := p.GetAuthURL(state, redirectURI)

	// For API response, return JSON with auth URL
	if strings.Contains(request.Headers["Accept"], "application/json") {
		return common.OK(map[string]string{
			"auth_url": authURL,
			"state":    state,
		}), nil
	}

	// Otherwise redirect
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusFound,
		Headers: map[string]string{
			"Location": authURL,
		},
	}, nil
}

// HandleOAuthProviderCallback handles the callback from external OAuth provider
// GET /oauth/{provider}/callback
func (h *Handler) HandleOAuthProviderCallback(ctx context.Context, request events.APIGatewayV2HTTPRequest, provider string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract code and state from query params
	code := request.QueryStringParameters["code"]
	state := request.QueryStringParameters["state"]
	errorParam := request.QueryStringParameters["error"]

	// Check for OAuth errors
	if errorParam != "" {
		errorDesc := request.QueryStringParameters["error_description"]
		h.logger.Error("OAuth provider error",
			zap.String("provider", provider),
			zap.String("error", errorParam),
			zap.String("description", errorDesc))
		return common.BadRequest(fmt.Errorf("OAuth error: %s - %s", errorParam, errorDesc)), nil
	}

	if code == "" || state == "" {
		return common.BadRequest(fmt.Errorf("missing code or state parameter")), nil
	}

	// Retrieve and validate state from DynamoDB
	oauthState, err := h.store.GetOAuthState(ctx, state)
	if err != nil {
		h.logger.Error("failed to get OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return common.BadRequest(fmt.Errorf("invalid or expired state")), nil
	}

	// Verify provider matches
	if oauthState.Provider != provider {
		h.logger.Error("provider mismatch",
			zap.String("expected", oauthState.Provider),
			zap.String("actual", provider))
		return common.BadRequest(fmt.Errorf("provider mismatch")), nil
	}

	// Clean up state now that we've used it
	if err := h.store.DeleteOAuthState(ctx, state); err != nil {
		h.logger.Error("failed to delete OAuth state", zap.Error(err))
		// Continue - not critical
	}

	// Get provider
	p := h.getProvider(provider)
	if p == nil {
		return common.BadRequest(fmt.Errorf("unknown provider: %s", provider)), nil
	}

	// Build redirect URI (must match what was sent to provider)
	redirectURI := fmt.Sprintf("%s/oauth/%s/callback", h.cfg.BaseURL(), provider)

	// Exchange code for tokens
	tokenResp, err := p.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		h.logger.Error("failed to exchange code",
			zap.String("provider", provider),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to exchange code: %w", err)), nil
	}

	// Get user info from provider
	userInfo, err := p.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		h.logger.Error("failed to get user info",
			zap.String("provider", provider),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get user info: %w", err)), nil
	}

	h.logger.Info("got user info from provider",
		zap.String("provider", provider),
		zap.String("provider_id", userInfo.ProviderID),
		zap.String("username", userInfo.Username),
		zap.String("email", userInfo.Email))

	// If this is a linking flow (username in state), handle differently
	if oauthState.Username != "" {
		// Verify the linking user is authenticated
		// TODO: Could add additional verification here

		// Link the provider account
		if err := h.store.LinkProviderAccount(ctx, oauthState.Username, provider, userInfo.ProviderID); err != nil {
			h.logger.Error("failed to link provider account",
				zap.String("username", oauthState.Username),
				zap.String("provider", provider),
				zap.Error(err))
			return common.InternalServerError(fmt.Errorf("failed to link account")), nil
		}

		// Return success without creating new session
		return common.OK(map[string]string{
			"message":           fmt.Sprintf("Successfully linked %s account", provider),
			"provider":          provider,
			"provider_username": userInfo.Username,
		}), nil
	}

	// Check if user exists with this provider ID
	existingUser, err := h.store.GetUserByProviderID(ctx, provider, userInfo.ProviderID)
	if err == nil && existingUser != nil {
		// User exists, log them in
		return h.loginExternalUser(ctx, existingUser)
	}

	// Check if user with email exists (for account linking)
	if userInfo.Email != "" {
		emailUser, err := h.store.GetUserByEmail(ctx, userInfo.Email)
		if err == nil && emailUser != nil {
			// Link provider to existing account
			if err := h.store.LinkProviderAccount(ctx, emailUser.Username, provider, userInfo.ProviderID); err != nil {
				h.logger.Error("failed to link provider account",
					zap.String("username", emailUser.Username),
					zap.String("provider", provider),
					zap.Error(err))
			} else {
				// Successfully linked, log them in
				return h.loginExternalUser(ctx, emailUser)
			}
		}
	}

	// Create new user account
	username := h.generateUsername(userInfo.Username, provider)

	// Create user in storage
	user := &storage.User{
		Username:     username,
		Email:        userInfo.Email,
		PasswordHash: "", // No password for OAuth users
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Approved:     true, // Auto-approve OAuth users
		Suspended:    false,
	}

	if err := h.store.CreateUser(ctx, user); err != nil {
		h.logger.Error("failed to create user",
			zap.String("username", username),
			zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Link provider account
	if err := h.store.LinkProviderAccount(ctx, username, provider, userInfo.ProviderID); err != nil {
		h.logger.Error("failed to link provider account for new user",
			zap.String("username", username),
			zap.String("provider", provider),
			zap.Error(err))
	}

	// Create actor profile
	// TODO: Create ActivityPub actor with info from provider

	// Log them in
	return h.loginExternalUser(ctx, user)
}

// HandleLinkOAuthProvider links an OAuth provider to existing account
// POST /oauth/{provider}/link
func (h *Handler) HandleLinkOAuthProvider(ctx context.Context, request events.APIGatewayV2HTTPRequest, provider string) (*events.APIGatewayV2HTTPResponse, error) {
	// Must be authenticated
	username := h.getCurrentUser(request)
	if username == "" {
		return common.Unauthorized(fmt.Errorf("authentication required")), nil
	}

	// Parse request
	var req struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Get provider
	p := h.getProvider(provider)
	if p == nil {
		return common.BadRequest(fmt.Errorf("unknown provider: %s", provider)), nil
	}

	// Exchange code for tokens
	tokenResp, err := p.ExchangeCode(ctx, req.Code, req.RedirectURI)
	if err != nil {
		return common.InternalServerError(fmt.Errorf("failed to exchange code: %w", err)), nil
	}

	// Get user info
	userInfo, err := p.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return common.InternalServerError(fmt.Errorf("failed to get user info: %w", err)), nil
	}

	// Check if this provider account is already linked
	existingUser, err := h.store.GetUserByProviderID(ctx, provider, userInfo.ProviderID)
	if err == nil && existingUser != nil {
		if existingUser.Username != username {
			return common.Conflict(fmt.Errorf("this %s account is already linked to another user", provider)), nil
		}
		// Already linked to this user
		return common.OK(map[string]string{
			"message": fmt.Sprintf("%s account already linked", provider),
		}), nil
	}

	// Link provider account
	if err := h.store.LinkProviderAccount(ctx, username, provider, userInfo.ProviderID); err != nil {
		return common.InternalServerError(err), nil
	}

	return common.OK(map[string]string{
		"message":           fmt.Sprintf("Successfully linked %s account", provider),
		"provider":          provider,
		"provider_username": userInfo.Username,
	}), nil
}

// HandleUnlinkOAuthProvider unlinks an OAuth provider from account
// DELETE /oauth/{provider}/unlink
func (h *Handler) HandleUnlinkOAuthProvider(ctx context.Context, request events.APIGatewayV2HTTPRequest, provider string) (*events.APIGatewayV2HTTPResponse, error) {
	// Must be authenticated
	username := h.getCurrentUser(request)
	if username == "" {
		return common.Unauthorized(fmt.Errorf("authentication required")), nil
	}

	// Check if user has a password or other auth methods
	user, err := h.store.GetUser(ctx, username)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	// Count linked providers
	linkedProviders, err := h.store.GetLinkedProviders(ctx, username)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	// Don't allow unlinking if it's the only auth method
	if user.PasswordHash == "" && len(linkedProviders) <= 1 {
		return common.BadRequest(fmt.Errorf("cannot unlink the only authentication method")), nil
	}

	// Unlink provider
	if err := h.store.UnlinkProviderAccount(ctx, username, provider); err != nil {
		return common.InternalServerError(err), nil
	}

	return common.OK(map[string]string{
		"message": fmt.Sprintf("Successfully unlinked %s account", provider),
	}), nil
}

// getProvider returns the OAuth provider implementation
func (h *Handler) getProvider(name string) providers.Provider {
	switch strings.ToLower(name) {
	case "github":
		// TODO: Get from environment variables or config
		clientID := os.Getenv("GITHUB_CLIENT_ID")
		clientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
		if clientID == "" || clientSecret == "" {
			return nil
		}
		return providers.NewGitHubProvider(clientID, clientSecret)
	// TODO: Add Discord and Google providers
	default:
		return nil
	}
}

// loginExternalUser creates session for external OAuth user
func (h *Handler) loginExternalUser(ctx context.Context, user *storage.User) (*events.APIGatewayV2HTTPResponse, error) {
	// Get request metadata from context or defaults
	deviceName := "OAuth Login"
	userAgent := "OAuth"
	ipAddress := "127.0.0.1"

	// Create session through session manager
	sessionManager := auth.NewSessionManager(h.store)
	session, err := sessionManager.CreateSession(ctx, user.Username, deviceName, userAgent, ipAddress, "oauth")
	if err != nil {
		return common.InternalServerError(err), nil
	}

	// Generate access token using the same approach as other auth methods
	// Since we can't access generateShortLivedAccessToken directly, we'll create
	// a JWT with the same structure
	now := time.Now()
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)

	claims["sub"] = user.Username
	claims["username"] = user.Username
	claims["session_id"] = session.SessionID
	claims["device_id"] = session.DeviceID
	claims["client_id"] = "web"
	claims["scopes"] = []string{"read", "write"}
	claims["iat"] = now.Unix()
	claims["exp"] = now.Add(15 * time.Minute).Unix() // 15 minute expiration
	claims["nbf"] = now.Unix()

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "development-secret-change-me"
	}

	accessToken, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return common.InternalServerError(fmt.Errorf("failed to generate access token: %w", err)), nil
	}

	// Return tokens
	return common.OK(map[string]interface{}{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"refresh_token": session.RefreshToken,
		"expires_in":    900, // 15 minutes in seconds
		"scope":         "read write",
		"created_at":    time.Now().Unix(),
		"username":      user.Username,
		"me":            user.Username,
		"email":         user.Email,
		"session_id":    session.SessionID,
	}), nil
}

// generateUsername creates a unique username from provider info
func (h *Handler) generateUsername(providerUsername, provider string) string {
	// Start with provider username
	base := providerUsername
	if base == "" {
		base = provider + "_user"
	}

	// Clean username (lowercase, alphanumeric + underscore only)
	base = strings.ToLower(base)
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, base)

	// Try base username first
	username := base
	counter := 1

	// Keep trying with incrementing numbers until we find unique one
	for {
		user, _ := h.store.GetUser(context.Background(), username)
		if user == nil {
			return username
		}
		counter++
		username = fmt.Sprintf("%s_%d", base, counter)
	}
}

// getCurrentUser extracts username from auth token
func (h *Handler) getCurrentUser(request events.APIGatewayV2HTTPRequest) string {
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			authService, _ := auth.NewAuthService(h.store)
			claims, err := authService.ValidateAccessToken(token)
			if err == nil {
				return claims.Username
			}
		}
	}

	return ""
}
