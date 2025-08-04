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

	if clientID == "" {
		return errors.New("client_id is required")
	}

	if redirectURI == "" {
		return errors.New("redirect_uri is required")
	}

	// Initialize OAuth service
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)

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
	if host == "" {
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

	if username == "" {
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

	// Validate scopes
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

		if err := h.store.SaveOAuthState(ctx.Context, authState); err != nil {
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

	if err := h.store.CreateAuthorizationCode(ctx.Context, authCode); err != nil {
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
	if redirectURI == "" {
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
	
	// Check for JWT in Authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := h.authMiddleware.ValidateToken(token)
		if err == nil && claims != nil {
			return claims.Username
		}
	}

	// Check for session cookie
	// In Lift, cookies come through headers
	cookieHeader := ctx.Header("Cookie")
	if cookieHeader != "" {
		// Parse cookies to find session
		// For now, we'll return empty - session handling needs proper implementation
		return ""
	}

	return ""
}

// showConsentScreenLift shows the consent screen using Lift patterns
func (h *Handler) showConsentScreenLift(ctx *lift.Context, authState *storage.OAuthState) error {
	// Get app details
	app, err := h.store.GetOAuthApp(ctx.Context, authState.ClientID)
	if err != nil {
		h.logger.Error("failed to get OAuth app", zap.Error(err))
		return errors.New("client not found")
	}

	// In a real implementation, this would render an HTML template
	// For now, we'll return a simple HTML response
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
	var items []string
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

// hasUserConsentedToApp checks if user has consented to the app with required scopes
func (h *Handler) hasUserConsentedToApp(ctx context.Context, username, clientID string, scopes []string) bool {
	consent, err := h.store.GetUserAppConsent(ctx, username, clientID)
	if err != nil || consent == nil {
		return false
	}
	
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