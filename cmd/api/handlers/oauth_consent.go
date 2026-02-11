package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleOAuthConsentLift handles the OAuth consent form submission using Lift patterns
// POST /oauth/consent
func (h *Handler) HandleOAuthConsentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse form data from request body
	if err := common.ValidateSliceNotEmpty("requestBody", ctx.Request.Body); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Empty request body",
		})
	}

	params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
	if err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Unable to parse form data",
		})
	}

	// Extract form parameters
	state := params["state"]
	action := params["action"] // "approve" or "deny"

	if state == "" {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Missing state parameter",
		})
	}

	// Get OAuth state from storage
	authState, err := h.repos.OAuth().GetOAuthState(ctx.Context(), state)
	if err != nil {
		h.logger.Error("failed to get OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid or expired authorization request",
		})
	}

	// Handle user action
	// Note: We don't create OAuth sessions for client applications - OAuth state is sufficient
	switch action {
	case oauthConsentActionDeny:
		return h.handleConsentDenial(ctx, authState)
	case actionApprove:
		return h.handleConsentApproval(ctx, authState)
	default:
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid action parameter",
		})
	}
}

// handleConsentDenial handles when the user denies consent
func (h *Handler) handleConsentDenial(ctx *apptheory.Context, authState *storage.OAuthState) (*apptheory.Response, error) {
	// Clean up the OAuth state
	if err := h.repos.OAuth().DeleteOAuthState(ctx.Context(), authState.State); err != nil {
		h.logger.Warn("failed to clean up OAuth state", zap.Error(err))
	}

	// Build redirect URL with error
	redirectURL, err := url.Parse(authState.RedirectURI)
	if err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
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

	return okJSON(apimodels.OAuthConsentResponse{RedirectURI: redirectURL.String()})
}

// handleConsentApproval handles when the user approves consent
func (h *Handler) handleConsentApproval(ctx *apptheory.Context, authState *storage.OAuthState) (*apptheory.Response, error) {
	// Store user consent for future requests
	consent := &storage.UserAppConsent{
		Username:  authState.Username,
		ClientID:  authState.ClientID,
		Scopes:    authState.Scopes,
		GrantedAt: time.Now(),
	}

	if err := h.repos.OAuth().SaveUserAppConsent(ctx.Context(), consent); err != nil {
		h.logger.Warn("failed to store user consent", zap.Error(err))
		// Non-fatal - continue with authorization
	}

	// Generate authorization code
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	code, err := oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		h.logger.Error("failed to generate authorization code", zap.Error(err))
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
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

	if err := h.repos.OAuth().CreateAuthorizationCode(ctx.Context(), authCode); err != nil {
		h.logger.Error("failed to store authorization code", zap.Error(err))
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error":             "server_error",
			"error_description": "Failed to store authorization code",
		})
	}

	// Clean up the OAuth state
	if err := h.repos.OAuth().DeleteOAuthState(ctx.Context(), authState.State); err != nil {
		h.logger.Warn("failed to clean up OAuth state", zap.Error(err))
	}

	// Build redirect URL with authorization code
	redirectURL, err := url.Parse(authState.RedirectURI)
	if err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
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

	return okJSON(apimodels.OAuthConsentResponse{RedirectURI: redirectURL.String()})
}

// HandleOAuthLoginLift handles redirecting users to login during OAuth flow
// GET /oauth/login
func (h *Handler) HandleOAuthLoginLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract auth request from query parameter
	authRequestParam := queryValue(ctx, "auth_request")
	returnTo := queryValue(ctx, "return_to")

	if authRequestParam == "" {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Missing auth_request parameter",
		})
	}

	const authReturnPath = "/oauth/authorize"
	if strings.TrimSpace(returnTo) == "" {
		returnTo = authReturnPath
	}

	returnTo = strings.TrimSpace(returnTo)
	if !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") {
		returnTo = authReturnPath
	} else if parsedReturnTo, err := url.Parse(returnTo); err != nil || parsedReturnTo.IsAbs() || parsedReturnTo.Host != "" {
		returnTo = authReturnPath
	}

	// Validate auth request is valid JSON (Auth UI expects JSON here).
	var authRequest map[string]string
	if err := json.Unmarshal([]byte(authRequestParam), &authRequest); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid auth_request parameter",
		})
	}

	authUIBaseURL := fmt.Sprintf("https://%s/auth", h.cfg.Domain)
	loginURL := fmt.Sprintf("%s/login?return_to=%s&auth_request=%s",
		authUIBaseURL,
		url.QueryEscape(returnTo),
		url.QueryEscape(authRequestParam))

	return h.writeOAuthAuthorizeRedirect(ctx, loginURL)
}
