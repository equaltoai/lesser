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
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleOAuthConsentLift handles the OAuth consent form submission using Lift patterns
// POST /oauth/consent
func (h *Handler) HandleOAuthConsentLift(ctx *lift.Context) error {
	// Parse form data from request body
	if err := common.ValidateSliceNotEmpty("requestBody", ctx.Request.Body); err != nil {
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
	state := params["state"]
	action := params["action"] // "approve" or "deny"

	if state == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Missing state parameter",
		})
	}

	// Initialize OAuth session repository
	oauthSessionRepo := repositories.NewOAuthSessionRepository(h.repos.GetDB(), h.repos.GetTableName(), h.logger, nil)

	// Get OAuth state from storage
	authState, err := h.repos.OAuth().GetOAuthState(ctx.Context, state)
	if err != nil {
		h.logger.Error("failed to get OAuth state", zap.Error(err))
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid or expired authorization request",
		})
	}

	// Create or get OAuth session for this authorization flow
	oauthSession, err := h.getOrCreateOAuthSession(ctx.Context, authState, oauthSessionRepo)
	if err != nil {
		h.logger.Error("failed to get OAuth session", zap.Error(err))
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "server_error",
			"error_description": "Failed to process authorization request",
		})
	}

	// Handle user action
	switch action {
	case "deny":
		return h.handleConsentDenial(ctx, authState, oauthSession)
	case "approve":
		return h.handleConsentApproval(ctx, authState, oauthSession, oauthSessionRepo)
	default:
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid action parameter",
		})
	}
}

// getOrCreateOAuthSession gets or creates an OAuth session for the authorization flow
func (h *Handler) getOrCreateOAuthSession(ctx context.Context, authState *storage.OAuthState, repo *repositories.OAuthSessionRepository) (*models.OAuthAuthSession, error) {
	// Try to get existing session by state
	session, err := repo.GetOAuthSessionByState(ctx, authState.State)
	if err == nil {
		return session, nil
	}

	// Create new OAuth session
	clientIP := ""  // Extract from Lambda event context if available
	userAgent := "" // Extract from Lambda event headers if available

	oauthSession := &models.OAuthAuthSession{
		ClientID:            authState.ClientID,
		RedirectURI:         authState.RedirectURI,
		Scopes:              authState.Scopes,
		State:               authState.State,
		CodeChallenge:       authState.CodeChallenge,
		CodeChallengeMethod: authState.CodeChallengeMethod,
		Username:            authState.Username,
		FlowStep:            "consent",
		IPAddress:           clientIP,
		UserAgent:           userAgent,
		IsSecure:            true, // Assume HTTPS in production
	}

	err = repo.CreateOAuthSession(ctx, oauthSession)
	if err != nil {
		return nil, errors.Join(failedToCreateOAuthSession(), err)
	}

	return oauthSession, nil
}

// handleConsentDenial handles when the user denies consent
func (h *Handler) handleConsentDenial(ctx *lift.Context, authState *storage.OAuthState, oauthSession *models.OAuthAuthSession) error {
	// Update OAuth session to reflect denial
	oauthSession.SetFlowStep("denied", map[string]interface{}{
		"denied_at": time.Now(),
		"reason":    "user_denied",
	})

	// Clean up the OAuth state
	if err := h.repos.OAuth().DeleteOAuthState(ctx.Context, authState.State); err != nil {
		h.logger.Warn("failed to clean up OAuth state", zap.Error(err))
	}

	// Build redirect URL with error
	redirectURL, err := url.Parse(authState.RedirectURI)
	if err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid redirect URI",
		})
	}

	q := redirectURL.Query()
	q.Set("error", "access_denied")
	q.Set("error_description", "The user denied the authorization request")
	if authState.State != "" {
		q.Set("state", authState.State)
	}
	redirectURL.RawQuery = q.Encode()

	ctx.Response.Header("Location", redirectURL.String())
	ctx.Status(http.StatusFound)
	return nil
}

// handleConsentApproval handles when the user approves consent
func (h *Handler) handleConsentApproval(ctx *lift.Context, authState *storage.OAuthState, oauthSession *models.OAuthAuthSession, repo *repositories.OAuthSessionRepository) error {
	// Update OAuth session to reflect approval
	err := repo.AuthorizeOAuthSession(ctx.Context, oauthSession.SessionID)
	if err != nil {
		h.logger.Error("failed to authorize OAuth session", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error":             "server_error",
			"error_description": "Failed to process authorization",
		})
	}

	// Store user consent for future requests
	consent := &storage.UserAppConsent{
		Username:  authState.Username,
		ClientID:  authState.ClientID,
		Scopes:    authState.Scopes,
		GrantedAt: time.Now(),
	}

	if err := h.repos.OAuth().SaveUserAppConsent(ctx.Context, consent); err != nil {
		h.logger.Warn("failed to store user consent", zap.Error(err))
		// Non-fatal - continue with authorization
	}

	// Generate authorization code
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	code, err := oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		h.logger.Error("failed to generate authorization code", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error":             "server_error",
			"error_description": "Failed to generate authorization code",
		})
	}

	// Store authorization code
	authCode := &storage.AuthorizationCode{
		Code:          code,
		ClientID:      authState.ClientID,
		Username:      authState.Username,
		CodeChallenge: authState.CodeChallenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
		Scopes:        authState.Scopes,
	}

	if err := h.repos.OAuth().CreateAuthorizationCode(ctx.Context, authCode); err != nil {
		h.logger.Error("failed to store authorization code", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error":             "server_error",
			"error_description": "Failed to store authorization code",
		})
	}

	// Clean up the OAuth state
	if err := h.repos.OAuth().DeleteOAuthState(ctx.Context, authState.State); err != nil {
		h.logger.Warn("failed to clean up OAuth state", zap.Error(err))
	}

	// Create user session for successful authorization
	err = h.createUserSessionAfterAuth(ctx.Context, authState.Username, oauthSession)
	if err != nil {
		h.logger.Warn("failed to create user session", zap.Error(err))
		// Non-fatal - continue with OAuth flow
	}

	// Build redirect URL with authorization code
	redirectURL, err := url.Parse(authState.RedirectURI)
	if err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid redirect URI",
		})
	}

	q := redirectURL.Query()
	q.Set("code", code)
	if authState.State != "" {
		q.Set("state", authState.State)
	}
	redirectURL.RawQuery = q.Encode()

	ctx.Response.Header("Location", redirectURL.String())
	ctx.Status(http.StatusFound)
	return nil
}

// createUserSessionAfterAuth creates a user session after successful OAuth authorization
func (h *Handler) createUserSessionAfterAuth(ctx context.Context, username string, oauthSession *models.OAuthAuthSession) error {
	// Create session manager
	sessionManager := auth.NewSessionManager(h.repos)

	// Create user session
	session, err := sessionManager.CreateSession(
		ctx,
		username,
		"OAuth Client", // Device name
		oauthSession.UserAgent,
		oauthSession.IPAddress,
		"oauth", // Auth method
	)
	if err != nil {
		return errors.Join(failedToCreateUserSession(), err)
	}

	h.logger.Debug("created user session after OAuth authorization",
		zap.String("username", username),
		zap.String("sessionID", session.SessionID),
		zap.String("oauthSessionID", oauthSession.SessionID))

	return nil
}

// HandleOAuthLoginLift handles redirecting users to login during OAuth flow
// GET /oauth/login
func (h *Handler) HandleOAuthLoginLift(ctx *lift.Context) error {
	// Extract auth request from query parameter
	authRequestParam := ctx.Query("auth_request")
	returnTo := ctx.Query("return_to")

	if authRequestParam == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Missing auth_request parameter",
		})
	}

	// Decode auth request
	var authRequest map[string]string
	if err := json.Unmarshal([]byte(authRequestParam), &authRequest); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid auth_request parameter",
		})
	}

	// Create OAuth session for tracking the flow
	oauthSessionRepo := repositories.NewOAuthSessionRepository(h.repos.GetDB(), h.repos.GetTableName(), h.logger, nil)

	clientIP := ""  // Extract from Lambda event context
	userAgent := "" // Extract from Lambda event headers

	oauthSession := &models.OAuthAuthSession{
		ClientID:            authRequest["client_id"],
		RedirectURI:         authRequest["redirect_uri"],
		State:               authRequest["state"],
		CodeChallenge:       authRequest["code_challenge"],
		CodeChallengeMethod: authRequest["code_challenge_method"],
		FlowStep:            "login",
		IPAddress:           clientIP,
		UserAgent:           userAgent,
		ReturnURL:           returnTo,
		IsSecure:            true,
	}

	// Parse scopes
	if scopeStr := authRequest["scope"]; scopeStr != "" {
		oauthSession.Scopes = strings.Fields(scopeStr)
	}

	err := oauthSessionRepo.CreateOAuthSession(ctx.Context, oauthSession)
	if err != nil {
		h.logger.Error("failed to create OAuth login session", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error":             "server_error",
			"error_description": "Failed to initiate login flow",
		})
	}

	// Render login page with OAuth context
	html := h.renderOAuthLoginPage(authRequest, oauthSession.SessionID)

	ctx.Response.Header("Content-Type", "text/html; charset=utf-8")
	return ctx.Text(html)
}

// renderOAuthLoginPage renders the OAuth login page
func (h *Handler) renderOAuthLoginPage(authRequest map[string]string, sessionID string) string {
	clientName := authRequest["client_id"] // In production, fetch app name from client_id

	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Login - OAuth Authorization</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 500px; margin: 50px auto; padding: 20px; }
        .oauth-info { background: #f8f9fa; padding: 20px; border-radius: 5px; margin-bottom: 20px; }
        .login-form { background: white; padding: 20px; border: 1px solid #ddd; border-radius: 5px; }
        .form-group { margin-bottom: 15px; }
        label { display: block; margin-bottom: 5px; font-weight: bold; }
        input[type="text"], input[type="password"] { 
            width: 100%%; padding: 10px; border: 1px solid #ddd; border-radius: 3px; 
        }
        button { 
            background: #007bff; color: white; padding: 10px 20px; 
            border: none; border-radius: 3px; cursor: pointer; width: 100%%; 
        }
        button:hover { background: #0056b3; }
        .error { color: #dc3545; margin-top: 10px; }
        .security-info { font-size: 12px; color: #666; margin-top: 15px; }
    </style>
</head>
<body>
    <div class="oauth-info">
        <h2>Login Required</h2>
        <p><strong>%s</strong> is requesting access to your account.</p>
        <p>Please login to continue with the authorization.</p>
    </div>
    
    <div class="login-form">
        <form method="POST" action="/auth/login">
            <input type="hidden" name="oauth_session_id" value="%s">
            <input type="hidden" name="return_to" value="/oauth/authorize">
            
            <div class="form-group">
                <label for="username">Username or Email:</label>
                <input type="text" id="username" name="username" required>
            </div>
            
            <div class="form-group">
                <label for="password">Password:</label>
                <input type="password" id="password" name="password" required>
            </div>
            
            <button type="submit">Login</button>
            
            <div class="security-info">
                <p>🔒 Your login is secured with enterprise-grade encryption.</p>
                <p>By logging in, you'll be redirected back to complete the authorization.</p>
            </div>
        </form>
    </div>
</body>
</html>
	`, clientName, sessionID)
}
