package lift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleOAuthAuthorizeLift handles the OAuth authorization endpoint using native Lift patterns
// GET /oauth/authorize
func (h *Handler) HandleOAuthAuthorizeLift(ctx *lift.Context) error {
	// Extract query parameters using Lift's request methods
	responseType := ctx.Query("response_type")
	clientID := ctx.Query("client_id")
	redirectURI := ctx.Query("redirect_uri")
	scope := ctx.Query("scope")
	state := ctx.Query("state")
	codeChallenge := ctx.Query("code_challenge")
	codeChallengeMethod := ctx.Query("code_challenge_method")

	// Validate required parameters
	if responseType != "code" {
		return h.oauthErrorLift(ctx, "unsupported_response_type", "Only 'code' response type is supported", redirectURI, state)
	}

	// Validate required parameters using centralized validation
	if err := common.ValidateMultipleRequiredParams(map[string]string{
		"client_id":    clientID,
		"redirect_uri": redirectURI,
	}); err != nil {
		return err
	}

	// Initialize OAuth service
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)

	// Validate client and redirect URI
	if err := oauthSvc.ValidateRedirectURI(ctx.Context, clientID, redirectURI); err != nil {
		h.logger.Error("invalid redirect URI",
			zap.String("client_id", clientID),
			zap.String("redirect_uri", redirectURI),
			zap.Error(err))
		return errors.New("invalid redirect_uri")
	}

	// Additional validation to prevent open redirects
	host := ctx.Header("Host")
	if err := common.ValidateRequiredParam("host_header", host); err != nil {
		// Fallback to config domain
		host = h.cfg.Domain
	}

	if err := common.ValidateRedirectURL(redirectURI, host); err != nil {
		h.logger.Error("potentially malicious redirect URI",
			zap.String("client_id", clientID),
			zap.String("redirect_uri", redirectURI),
			zap.Error(err))
		return errors.New("redirect_uri not allowed")
	}

	// Check if user is authenticated (from cookie or header)
	username := h.getUserFromSessionLift(ctx)

	if err := common.ValidateRequiredParam("username", username); err != nil {
		// User needs to login first
		// Store authorization request in session for after login
		authRequest := map[string]string{
			"client_id":             clientID,
			"redirect_uri":          redirectURI,
			"scope":                 scope,
			"state":                 state,
			"code_challenge":        codeChallenge,
			"code_challenge_method": codeChallengeMethod,
		}

		// Encode request as query string
		authRequestEncoded, _ := json.Marshal(authRequest)

		// Redirect to login page with return URL
		loginURL := fmt.Sprintf("/auth/login?return_to=/oauth/authorize&auth_request=%s",
			url.QueryEscape(string(authRequestEncoded)))

		ctx.Response.Header("Location", loginURL)
		ctx.Status(http.StatusFound)
		return nil
	}

	// Parse requested scopes
	scopes := []string{"read", "write"} // Default scopes
	if scope != "" {
		scopes = strings.Fields(scope)
	}

	// Validate scopes using centralized validation
	scopesStr := strings.Join(scopes, " ")
	if err := common.ValidateApplicationScopes(scopesStr); err != nil {
		return h.oauthErrorLift(ctx, "invalid_scope", fmt.Sprintf("Invalid scopes: %v", err), redirectURI, state)
	}

	// Additional validation using auth package for backward compatibility
	if err := auth.ValidateScopes(scopes); err != nil {
		return h.oauthErrorLift(ctx, "invalid_scope", "One or more requested scopes are invalid", redirectURI, state)
	}

	// Check if user has previously consented to this app
	if !h.hasUserConsentedToApp(ctx.Context, username, clientID, scopes) {
		// Store authorization request state for consent flow
		authState := &storage.OAuthState{
			State:               state,
			ClientID:            clientID,
			Username:            username,
			Scopes:              scopes,
			RedirectURI:         redirectURI,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			ExpiresAt:           time.Now().Add(10 * time.Minute),
		}

		if _, err := h.registry.Accounts().StoreOAuthState(ctx.Context, &accounts.StoreOAuthStateCommand{
			State:      authState.State,
			OAuthState: authState,
		}); err != nil {
			h.logger.Error("failed to save OAuth state", zap.Error(err))
			return h.oauthErrorLift(ctx, "server_error", "Failed to save authorization state", redirectURI, state)
		}

		return h.showConsentScreenLift(ctx, authState)
	}

	// Generate authorization code
	code, err := oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		h.logger.Error("failed to generate authorization code", zap.Error(err))
		return h.oauthErrorLift(ctx, "server_error", "Failed to generate authorization code", redirectURI, state)
	}

	// Store authorization code
	authCode := &storage.AuthorizationCode{
		Code:          code,
		ClientID:      clientID,
		Username:      username,
		CodeChallenge: codeChallenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
		Scopes:        scopes,
	}

	if _, err := h.registry.Accounts().CreateAuthorizationCode(ctx.Context, &accounts.CreateAuthorizationCodeCommand{
		AuthCode: authCode,
	}); err != nil {
		h.logger.Error("failed to store authorization code", zap.Error(err))
		return h.oauthErrorLift(ctx, "server_error", "Failed to store authorization code", redirectURI, state)
	}

	// Build redirect URL
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()

	ctx.Response.Header("Location", u.String())
	ctx.Status(http.StatusFound)
	return nil
}

// Helper methods for Lift implementation

// oauthErrorLift handles OAuth errors in Lift style
func (h *Handler) oauthErrorLift(ctx *lift.Context, errorCode, errorDescription, redirectURI, state string) error {
	// If no redirect URI, return JSON error
	if err := common.ValidateRequiredParam("redirect_uri", redirectURI); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             errorCode,
			"error_description": errorDescription,
		})
	}

	// Build redirect URL with error parameters
	u, err := url.Parse(redirectURI)
	if err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid redirect_uri",
		})
	}

	q := u.Query()
	q.Set("error", errorCode)
	q.Set("error_description", errorDescription)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()

	ctx.Response.Header("Location", u.String())
	ctx.Status(http.StatusFound)
	return nil
}

// getUserFromSessionLift extracts the username from the session using Lift patterns
func (h *Handler) getUserFromSessionLift(ctx *lift.Context) string {
	// Test mode - check for test header
	if testUsername := ctx.Header("X-Test-Username"); testUsername != "" {
		return testUsername
	}

	// Check for authentication context from unified middleware
	if username := ctx.Get("username"); username != nil {
		if usernameStr, ok := username.(string); ok && usernameStr != "" {
			return usernameStr
		}
	}

	// Check for session cookie
	cookieHeader := ctx.Header("Cookie")
	if cookieHeader != "" {
		username := h.extractUsernameFromSessionCookie(ctx.Context, cookieHeader)
		if username != "" {
			return username
		}
	}

	return ""
}

// showConsentScreenLift shows the consent screen using Lift patterns
func (h *Handler) showConsentScreenLift(ctx *lift.Context, authState *storage.OAuthState) error {
	// Get app details
	result, err := h.registry.Accounts().GetOAuthApp(ctx.Context, &accounts.GetOAuthAppQuery{
		ClientID: authState.ClientID,
	})
	if err != nil {
		h.logger.Error("failed to get OAuth app", zap.Error(err))
		return errors.New("client not found")
	}
	app := result.App

	// Render OAuth authorization HTML template with app details and permissions
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Authorize %s</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 600px; margin: 50px auto; padding: 20px; }
        .app-info { background: #f0f0f0; padding: 20px; border-radius: 5px; margin: 20px 0; }
        .scopes { margin: 20px 0; }
        .scopes li { margin: 5px 0; }
        .buttons { margin-top: 30px; }
        button { padding: 10px 20px; margin: 0 10px; font-size: 16px; cursor: pointer; }
        .approve { background: #4CAF50; color: white; border: none; }
        .deny { background: #f44336; color: white; border: none; }
    </style>
</head>
<body>
    <h1>Authorization Request</h1>
    <div class="app-info">
        <h2>%s</h2>
        <p>This application is requesting access to your account.</p>
    </div>
    <div class="scopes">
        <h3>Requested permissions:</h3>
        <ul>
            %s
        </ul>
    </div>
    <form method="POST" action="/oauth/consent" class="buttons">
        <input type="hidden" name="state" value="%s">
        <button type="submit" name="action" value="approve" class="approve">Approve</button>
        <button type="submit" name="action" value="deny" class="deny">Deny</button>
    </form>
</body>
</html>
	`, app.Name, app.Name, h.formatScopes(authState.Scopes), authState.State)

	ctx.Response.Header("Content-Type", "text/html; charset=utf-8")
	return ctx.Text(html)
}

// formatScopes formats scopes for display
func (h *Handler) formatScopes(scopes []string) string {
	items := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		description := h.getScopeDescription(scope)
		items = append(items, fmt.Sprintf("<li><strong>%s</strong>: %s</li>", scope, description))
	}
	return strings.Join(items, "\n")
}

// getScopeDescription returns a human-readable description for a scope
func (h *Handler) getScopeDescription(scope string) string {
	descriptions := map[string]string{
		"read":   "Read your account information and posts",
		"write":  "Post on your behalf",
		"follow": "Follow and unfollow accounts",
		"push":   "Send push notifications",
		"admin":  "Access admin functions",
	}

	if desc, ok := descriptions[scope]; ok {
		return desc
	}
	return "Unknown permission"
}

// extractUsernameFromSessionCookie extracts username from session cookie
func (h *Handler) extractUsernameFromSessionCookie(ctx context.Context, cookieHeader string) string {
	// Parse cookies from header
	cookies := common.ParseCookies(cookieHeader)

	// Look for session cookie
	sessionToken := cookies["session_token"]
	if err := common.ValidateRequiredParam("session_token", sessionToken); err != nil {
		// Try alternate cookie names
		sessionToken = cookies["user_session"]
	}

	if err := common.ValidateRequiredParam("session_token", sessionToken); err != nil {
		return ""
	}

	// Validate session token and get user
	session, err := h.repos.Account().GetSessionByRefreshToken(ctx, sessionToken)
	if err != nil {
		h.logger.Debug("failed to get session by token",
			zap.String("error", err.Error()))
		return ""
	}

	// Check if session is valid (not expired)
	if time.Now().After(session.ExpiresAt) {
		h.logger.Debug("session expired",
			zap.String("sessionID", session.SessionID))
		return ""
	}

	// Update session activity
	ipAddress := h.getClientIP(cookieHeader) // Extract from context
	if err := h.updateSessionActivity(ctx, session.SessionID, ipAddress); err != nil {
		h.logger.Warn("failed to update session activity",
			zap.String("sessionID", session.SessionID),
			zap.Error(err))
	}

	return session.Username
}

// getClientIP extracts client IP from request context
func (h *Handler) getClientIP(_ string) string {
	// In Lambda/API Gateway, the client IP should come from the event context
	// For now, return empty - this would be enhanced based on your Lambda setup
	return ""
}

// updateSessionActivity updates the last activity for a session
func (h *Handler) updateSessionActivity(ctx context.Context, sessionID, ipAddress string) error {
	// Get the session manager service
	sessionManager := auth.NewSessionManager(h.repos)
	return sessionManager.UpdateSessionActivity(ctx, sessionID, ipAddress)
}

// hasUserConsentedToApp checks if user has consented to the app with required scopes
func (h *Handler) hasUserConsentedToApp(ctx context.Context, username, clientID string, scopes []string) bool {
	result, err := h.registry.Accounts().GetUserAppConsent(ctx, &accounts.GetUserAppConsentQuery{
		Username: username,
		ClientID: clientID,
	})
	if err != nil || result == nil || result.Consent == nil {
		return false
	}
	consent := result.Consent

	// Check if all requested scopes are granted
	grantedMap := make(map[string]bool)
	for _, s := range consent.Scopes {
		grantedMap[s] = true
	}

	for _, s := range scopes {
		if !grantedMap[s] {
			return false
		}
	}

	return true
}

// HandleOAuthTokenLift handles the OAuth token endpoint using native Lift patterns
// POST /oauth/token
func (h *Handler) HandleOAuthTokenLift(ctx *lift.Context) error {
	// Parse form data from request body
	if err := common.ValidateSliceNotEmpty("request_body", ctx.Request.Body); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Empty request body",
		})
	}

	params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
	if err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Unable to parse form data",
		})
	}

	// Extract form parameters
	grantType := params["grant_type"]
	code := params["code"]
	redirectURI := params["redirect_uri"]
	clientID := params["client_id"]
	clientSecret := params["client_secret"]
	codeVerifier := params["code_verifier"]
	refreshToken := params["refresh_token"]

	// Initialize OAuth service
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)

	switch grantType {
	case "authorization_code":
		// Validate required parameters using centralized validation
		if err := common.ValidateMultipleRequiredParams(map[string]string{
			"code":         code,
			"client_id":    clientID,
			"redirect_uri": redirectURI,
		}); err != nil {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
				"error":             "invalid_request",
				"error_description": err.Error(),
			})
		}

		// Exchange authorization code for tokens
		accessToken, refreshTokenOut, err := h.exchangeAuthorizationCode(ctx.Context, oauthSvc, code, clientID, redirectURI, codeVerifier, clientSecret)
		if err != nil {
			h.logger.Error("failed to exchange authorization code", zap.Error(err))

			// Return appropriate OAuth error based on the error type
			if err == auth.ErrInvalidGrant {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
					"error":             "invalid_grant",
					"error_description": "Invalid authorization code or expired",
				})
			}
			if err == auth.ErrInvalidClient {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
					"error":             "invalid_client",
					"error_description": "Invalid client credentials",
				})
			}
			if err == auth.ErrInvalidCodeChallenge {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
					"error":             "invalid_grant",
					"error_description": "PKCE verification failed",
				})
			}

			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
				"error":             "invalid_grant",
				"error_description": "Authorization code exchange failed",
			})
		}

		return ctx.JSON(map[string]interface{}{
			"access_token":  accessToken,
			"token_type":    "Bearer",
			"scope":         "read write follow push",
			"created_at":    fmt.Sprintf("%d", time.Now().Unix()),
			"expires_in":    3600,
			"refresh_token": refreshTokenOut,
		})

	case "refresh_token":
		// Validate required parameters using centralized validation
		if err := common.ValidateMultipleRequiredParams(map[string]string{
			"refresh_token": refreshToken,
			"client_id":     clientID,
		}); err != nil {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
				"error":             "invalid_request",
				"error_description": err.Error(),
			})
		}

		// Exchange refresh token for new tokens
		accessToken, newRefreshToken, err := h.exchangeRefreshToken(ctx.Context, oauthSvc, refreshToken, clientID, clientSecret)
		if err != nil {
			h.logger.Error("failed to refresh tokens", zap.Error(err))

			// Return appropriate OAuth error based on the error type
			if err == auth.ErrInvalidToken {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
					"error":             "invalid_grant",
					"error_description": "Invalid or expired refresh token",
				})
			}
			if err == auth.ErrInvalidClient {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
					"error":             "invalid_client",
					"error_description": "Invalid client credentials",
				})
			}

			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
				"error":             "invalid_grant",
				"error_description": "Refresh token exchange failed",
			})
		}

		return ctx.JSON(map[string]interface{}{
			"access_token":  accessToken,
			"token_type":    "Bearer",
			"scope":         "read write follow push",
			"created_at":    fmt.Sprintf("%d", time.Now().Unix()),
			"expires_in":    3600,
			"refresh_token": newRefreshToken,
		})

	default:
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "unsupported_grant_type",
			"error_description": "Only authorization_code and refresh_token grant types are supported",
		})
	}
}

// exchangeAuthorizationCode exchanges an authorization code for access and refresh tokens
func (h *Handler) exchangeAuthorizationCode(ctx context.Context, oauthSvc *auth.OAuthService, code, clientID, redirectURI, codeVerifier, clientSecret string) (string, string, error) {
	// Validate client credentials if provided
	if clientSecret != "" {
		if err := oauthSvc.ValidateClient(ctx, clientID, clientSecret); err != nil {
			return "", "", err
		}
	}

	// Validate redirect URI
	if err := oauthSvc.ValidateRedirectURI(ctx, clientID, redirectURI); err != nil {
		return "", "", err
	}

	// Get authorization code from storage
	authCode, err := h.repos.Account().GetAuthorizationCode(ctx, code)
	if err != nil {
		return "", "", auth.ErrInvalidGrant
	}

	// Validate authorization code
	if authCode.ClientID != clientID {
		return "", "", auth.ErrInvalidGrant
	}

	// Check expiration
	if time.Now().After(authCode.ExpiresAt) {
		// Clean up expired code
		_ = h.repos.Account().DeleteAuthorizationCode(ctx, code)
		return "", "", auth.ErrInvalidGrant
	}

	// Verify PKCE if used
	if authCode.CodeChallenge != "" || codeVerifier != "" {
		if err := oauthSvc.VerifyCodeChallenge(authCode.CodeChallenge, codeVerifier, "S256"); err != nil {
			return "", "", err
		}
	}

	// Generate tokens
	accessToken, refreshToken, err := oauthSvc.GenerateTokens(ctx, authCode.Username, clientID, "", authCode.Scopes)
	if err != nil {
		return "", "", errors.Join(ErrFailedToGenerateTokens, err)
	}

	// Store refresh token in storage for later validation
	oauthRefreshToken := &storage.RefreshToken{
		Token:     refreshToken,
		Username:  authCode.Username,
		ClientID:  clientID,
		Scopes:    authCode.Scopes,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(auth.RefreshTokenDuration),
	}

	if err := h.repos.Account().CreateRefreshToken(ctx, oauthRefreshToken); err != nil {
		h.logger.Error("failed to store refresh token", zap.Error(err))
		// Continue - access token is still valid
	}

	// Delete the used authorization code
	if err := h.repos.Account().DeleteAuthorizationCode(ctx, code); err != nil {
		h.logger.Error("failed to delete authorization code", zap.Error(err))
		// Non-critical error, continue
	}

	return accessToken, refreshToken, nil
}

// exchangeRefreshToken exchanges a refresh token for new access and refresh tokens
func (h *Handler) exchangeRefreshToken(ctx context.Context, oauthSvc *auth.OAuthService, refreshToken, clientID, clientSecret string) (string, string, error) {
	// Validate client credentials if provided
	if clientSecret != "" {
		if err := oauthSvc.ValidateClient(ctx, clientID, clientSecret); err != nil {
			return "", "", err
		}
	}

	// Get refresh token from storage
	storedToken, err := h.repos.Account().GetRefreshToken(ctx, refreshToken)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", "", auth.ErrInvalidToken
		}
		if strings.Contains(err.Error(), "expired") {
			return "", "", auth.ErrInvalidToken
		}
		return "", "", errors.Join(ErrFailedToValidateRefreshToken, err)
	}

	// Validate refresh token belongs to the client
	if storedToken.ClientID != clientID {
		return "", "", auth.ErrInvalidToken
	}

	// Check expiration
	if time.Now().After(storedToken.ExpiresAt) {
		// Clean up expired token
		_ = h.repos.Account().DeleteRefreshToken(ctx, refreshToken)
		return "", "", auth.ErrInvalidToken
	}

	// Generate new tokens with same scopes
	accessToken, newRefreshToken, err := oauthSvc.GenerateTokens(ctx, storedToken.Username, clientID, "", storedToken.Scopes)
	if err != nil {
		return "", "", errors.Join(ErrFailedToGenerateNewTokens, err)
	}

	// Create new refresh token record
	newOAuthRefreshToken := &storage.RefreshToken{
		Token:     newRefreshToken,
		Username:  storedToken.Username,
		ClientID:  clientID,
		Scopes:    storedToken.Scopes,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(auth.RefreshTokenDuration),
	}

	// Store new refresh token
	if err := h.repos.Account().CreateRefreshToken(ctx, newOAuthRefreshToken); err != nil {
		h.logger.Error("failed to store new refresh token", zap.Error(err))
		return "", "", errors.Join(ErrFailedToStoreNewRefreshToken, err)
	}

	// Delete the old refresh token to prevent reuse
	if err := h.repos.Account().DeleteRefreshToken(ctx, refreshToken); err != nil {
		h.logger.Error("failed to delete old refresh token", zap.Error(err))
		// Continue - new token is already stored
	}

	return accessToken, newRefreshToken, nil
}
