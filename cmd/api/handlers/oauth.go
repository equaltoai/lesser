package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

type authorizeRequest struct {
	responseType        string
	clientID            string
	redirectURI         string
	scope               string
	state               string
	codeChallenge       string
	codeChallengeMethod string
}

type authorizeFlow struct {
	request  *authorizeRequest
	oauthSvc *auth.OAuthService
	username string
	scopes   []string
}

func (h *Handler) isOAuthAuthorizeUIMode(ctx *apptheory.Context) bool {
	if strings.EqualFold(queryValue(ctx, "mode"), "ui") {
		return true
	}

	accept := headerValue(ctx, "Accept")
	if accept == "" {
		accept = headerValue(ctx, "accept")
	}

	return strings.Contains(accept, "application/json")
}

func (h *Handler) writeOAuthAuthorizeRedirect(ctx *apptheory.Context, nextURL string) (*apptheory.Response, error) {
	if h.isOAuthAuthorizeUIMode(ctx) {
		return okJSON(apimodels.OAuthAuthorizeResponse{NextURL: nextURL})
	}

	resp := &apptheory.Response{Status: http.StatusFound}
	setHeader(resp, "Location", nextURL)
	return resp, nil
}

// HandleOAuthAuthorizeLift handles the OAuth authorization endpoint using native Lift patterns
// GET /oauth/authorize
func (h *Handler) HandleOAuthAuthorizeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	flow, resp, err := h.initializeAuthorizeFlow(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	resp, err = h.ensureConsentForFlow(ctx, flow)
	if resp != nil || err != nil {
		return resp, err
	}

	return h.completeAuthorizationFlow(ctx, flow)
}

func (h *Handler) initializeAuthorizeFlow(ctx *apptheory.Context) (*authorizeFlow, *apptheory.Response, error) {
	req, resp, err := h.extractAuthorizeRequest(ctx)
	if resp != nil || err != nil {
		return nil, resp, err
	}

	flow := &authorizeFlow{
		request:  req,
		oauthSvc: createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger),
	}

	if err := flow.oauthSvc.ValidateRedirectURI(ctx.Context(), req.clientID, req.redirectURI); err != nil {
		h.logger.Error("invalid redirect URI",
			zap.String("client_id", req.clientID),
			zap.String("redirect_uri", req.redirectURI),
			zap.Error(err))
		resp, respErr := h.oauthErrorLift(ctx, "invalid_request", "Invalid redirect_uri", "", req.state)
		return nil, resp, respErr
	}

	username, resp, err := h.resolveAuthorizeUser(ctx, req)
	if resp != nil || err != nil {
		return nil, resp, err
	}
	flow.username = username

	scopes, err := h.normalizeAuthorizeScopes(req.scope)
	if err != nil {
		resp, respErr := h.oauthErrorLift(ctx, "invalid_scope", err.Error(), req.redirectURI, req.state)
		return nil, resp, respErr
	}
	flow.scopes = scopes

	return flow, nil, nil
}

func (h *Handler) extractAuthorizeRequest(ctx *apptheory.Context) (*authorizeRequest, *apptheory.Response, error) {
	req := &authorizeRequest{
		responseType:        queryValue(ctx, "response_type"),
		clientID:            queryValue(ctx, "client_id"),
		redirectURI:         queryValue(ctx, "redirect_uri"),
		scope:               queryValue(ctx, "scope"),
		state:               queryValue(ctx, "state"),
		codeChallenge:       queryValue(ctx, "code_challenge"),
		codeChallengeMethod: queryValue(ctx, "code_challenge_method"),
	}

	if req.responseType != "code" {
		resp, err := h.oauthErrorLift(ctx, "unsupported_response_type", "Only 'code' response type is supported", req.redirectURI, req.state)
		return nil, resp, err
	}

	if req.clientID == "" || req.redirectURI == "" {
		resp, err := h.redirectMissingAuthorizeParams(ctx, req)
		return nil, resp, err
	}

	return req, nil, nil
}

func (h *Handler) redirectMissingAuthorizeParams(ctx *apptheory.Context, req *authorizeRequest) (*apptheory.Response, error) {
	authUIBaseURL := fmt.Sprintf("https://%s/auth", h.cfg.Domain)
	errorMessage := "Invalid authorization request - missing required parameters. Please restart the authorization flow from your application."

	errorParams := url.Values{}
	errorParams.Set("error", errorMessage)
	if req.clientID != "" {
		errorParams.Set("client_id", req.clientID)
	}
	if req.redirectURI != "" {
		errorParams.Set("redirect_uri", req.redirectURI)
	}
	if req.state != "" {
		errorParams.Set("state", req.state)
	}
	if req.scope != "" {
		errorParams.Set("scope", req.scope)
	}
	if req.responseType != "" {
		errorParams.Set("response_type", req.responseType)
	}

	errorURL := fmt.Sprintf("%s/oauth/authorize?%s", authUIBaseURL, errorParams.Encode())
	return h.writeOAuthAuthorizeRedirect(ctx, errorURL)
}

func (h *Handler) resolveAuthorizeUser(ctx *apptheory.Context, req *authorizeRequest) (string, *apptheory.Response, error) {
	username := h.getUserFromSessionLift(ctx)

	if err := common.ValidateRequiredParam("username", username); err != nil {
		resp, respErr := h.redirectUserToLogin(ctx, req)
		return "", resp, respErr
	}

	return username, nil, nil
}

func (h *Handler) redirectUserToLogin(ctx *apptheory.Context, req *authorizeRequest) (*apptheory.Response, error) {
	authUIBaseURL := fmt.Sprintf("https://%s/auth", h.cfg.Domain)
	authRequest := map[string]string{
		"client_id":             req.clientID,
		"redirect_uri":          req.redirectURI,
		"scope":                 req.scope,
		"state":                 req.state,
		"response_type":         req.responseType,
		"code_challenge":        req.codeChallenge,
		"code_challenge_method": req.codeChallengeMethod,
	}

	payload, _ := json.Marshal(authRequest)
	loginURL := fmt.Sprintf("%s/login?return_to=%s&auth_request=%s",
		authUIBaseURL,
		url.QueryEscape("/oauth/authorize"),
		url.QueryEscape(string(payload)))
	return h.writeOAuthAuthorizeRedirect(ctx, loginURL)
}

func (h *Handler) normalizeAuthorizeScopes(scope string) ([]string, error) {
	scopes := []string{"read", "write"}
	if strings.TrimSpace(scope) != "" {
		scopes = strings.Fields(scope)
	}

	scopesStr := strings.Join(scopes, " ")
	if err := common.ValidateApplicationScopes(scopesStr); err != nil {
		return nil, fmt.Errorf("invalid scopes: %v", err)
	}

	if err := auth.ValidateScopes(scopes); err != nil {
		return nil, errors.New("one or more requested scopes are invalid")
	}

	return scopes, nil
}

func (h *Handler) ensureConsentForFlow(ctx *apptheory.Context, flow *authorizeFlow) (*apptheory.Response, error) {
	if h.registry == nil {
		h.logger.Error("service registry not initialized for OAuth authorization")
		return h.oauthErrorLift(ctx, "server_error", "Service unavailable", flow.request.redirectURI, flow.request.state)
	}

	if h.hasUserConsentedToApp(ctx.Context(), flow.username, flow.request.clientID, flow.scopes) {
		return nil, nil
	}

	authState := &storage.OAuthState{
		State:               flow.request.state,
		ClientID:            flow.request.clientID,
		Username:            flow.username,
		Scopes:              flow.scopes,
		RedirectURI:         flow.request.redirectURI,
		CodeChallenge:       flow.request.codeChallenge,
		CodeChallengeMethod: flow.request.codeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}

	if _, err := h.registry.Accounts().StoreOAuthState(ctx.Context(), &accounts.StoreOAuthStateCommand{
		State:      authState.State,
		OAuthState: authState,
	}); err != nil {
		h.logger.Error("failed to save OAuth state", zap.Error(err))
		return h.oauthErrorLift(ctx, "server_error", "Failed to save authorization state", flow.request.redirectURI, flow.request.state)
	}

	h.logger.Info("redirecting to consent UI",
		zap.String("username", flow.username),
		zap.String("client_id", flow.request.clientID))

	return h.redirectToConsentUI(ctx, authState)
}

func (h *Handler) completeAuthorizationFlow(ctx *apptheory.Context, flow *authorizeFlow) (*apptheory.Response, error) {
	code, err := flow.oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		h.logger.Error("failed to generate authorization code", zap.Error(err))
		return h.oauthErrorLift(ctx, "server_error", "Failed to generate authorization code", flow.request.redirectURI, flow.request.state)
	}

	authCode := &storage.AuthorizationCode{
		Code:          code,
		ClientID:      flow.request.clientID,
		Username:      flow.username,
		CodeChallenge: flow.request.codeChallenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
		Scopes:        flow.scopes,
	}

	if _, err := h.registry.Accounts().CreateAuthorizationCode(ctx.Context(), &accounts.CreateAuthorizationCodeCommand{
		AuthCode: authCode,
	}); err != nil {
		h.logger.Error("failed to store authorization code", zap.Error(err))
		return h.oauthErrorLift(ctx, "server_error", "Failed to store authorization code", flow.request.redirectURI, flow.request.state)
	}

	redirectURL, _ := url.Parse(flow.request.redirectURI)
	query := redirectURL.Query()
	query.Set("code", code)
	if flow.request.state != "" {
		query.Set("state", flow.request.state)
	}
	redirectURL.RawQuery = query.Encode()
	return h.writeOAuthAuthorizeRedirect(ctx, redirectURL.String())
}

// Helper methods for Lift implementation

// oauthErrorLift handles OAuth errors in Lift style
func (h *Handler) oauthErrorLift(ctx *apptheory.Context, errorCode, errorDescription, redirectURI, state string) (*apptheory.Response, error) {
	// If no redirect URI, return JSON error
	if err := common.ValidateRequiredParam("redirect_uri", redirectURI); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             errorCode,
			"error_description": errorDescription,
		})
	}

	// Build redirect URL with error parameters
	u, err := url.Parse(redirectURI)
	if err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
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
	return h.writeOAuthAuthorizeRedirect(ctx, u.String())
}

// getUserFromSessionLift extracts the username from the session using Lift patterns
func (h *Handler) getUserFromSessionLift(ctx *apptheory.Context) string {
	// Check for authentication context from unified middleware
	if username := ctx.Get("username"); username != nil {
		if usernameStr, ok := username.(string); ok && usernameStr != "" {
			return usernameStr
		}
	}

	// Check for Bearer token in Authorization header (for cross-subdomain auth)
	authHeader := headerValue(ctx, "Authorization")
	if authHeader == "" {
		authHeader = headerValue(ctx, "authorization")
	}
	if authHeader != "" {
		if token, err := auth.ExtractBearerToken(authHeader); err == nil {
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
			if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
				return claims.Username
			}
		}
	}

	return ""
}

// redirectToConsentUI redirects to the hosted OAuth consent UI
func (h *Handler) redirectToConsentUI(ctx *apptheory.Context, authState *storage.OAuthState) (*apptheory.Response, error) {
	// Check if registry is initialized
	if h.registry == nil {
		h.logger.Error("service registry not initialized for OAuth consent")
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error":             "server_error",
			"error_description": "Service unavailable",
		})
	}

	// Get app details to pass to consent UI
	result, err := h.registry.Accounts().GetOAuthApp(ctx.Context(), &accounts.GetOAuthAppQuery{
		ClientID: authState.ClientID,
	})
	if err != nil {
		h.logger.Error("failed to get OAuth app", zap.Error(err))
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_client",
			"error_description": "client not found",
		})
	}
	app := result.App

	authUIBaseURL := fmt.Sprintf("https://%s/auth", h.cfg.Domain)

	// Build consent URL with all necessary parameters.
	consentURL := fmt.Sprintf("%s/consent?state=%s&client_id=%s&client_name=%s&client_url=%s&scopes=%s&redirect_uri=%s",
		authUIBaseURL,
		url.QueryEscape(authState.State),
		url.QueryEscape(authState.ClientID),
		url.QueryEscape(app.Name),
		url.QueryEscape(app.Website),
		url.QueryEscape(strings.Join(authState.Scopes, " ")),
		url.QueryEscape(authState.RedirectURI))
	return h.writeOAuthAuthorizeRedirect(ctx, consentURL)
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
func (h *Handler) HandleOAuthTokenLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse form data from request body
	if err := common.ValidateSliceNotEmpty("request_body", ctx.Request.Body); err != nil {
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
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_request",
				"error_description": err.Error(),
			})
		}

		// Exchange authorization code for tokens
		accessToken, refreshTokenOut, err := h.exchangeAuthorizationCode(ctx.Context(), oauthSvc, code, clientID, redirectURI, codeVerifier, clientSecret)
		if err != nil {
			h.logger.Error("failed to exchange authorization code", zap.Error(err))

			// Return appropriate OAuth error based on the error type
			if err == auth.ErrInvalidGrant {
				return apptheory.JSON(http.StatusBadRequest, map[string]string{
					"error":             "invalid_grant",
					"error_description": "Invalid authorization code or expired",
				})
			}
			if err == auth.ErrInvalidClient {
				return apptheory.JSON(http.StatusBadRequest, map[string]string{
					"error":             "invalid_client",
					"error_description": "Invalid client credentials",
				})
			}
			if err == auth.ErrInvalidCodeChallenge {
				return apptheory.JSON(http.StatusBadRequest, map[string]string{
					"error":             "invalid_grant",
					"error_description": "PKCE verification failed",
				})
			}

			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_grant",
				"error_description": "Authorization code exchange failed",
			})
		}

		return okJSON(apimodels.OAuthTokenResponse{
			AccessToken:  accessToken,
			TokenType:    "Bearer",
			Scope:        "read write follow push",
			CreatedAt:    time.Now().Unix(),
			ExpiresIn:    3600,
			RefreshToken: refreshTokenOut,
		})

	case "refresh_token":
		// Validate required parameters using centralized validation
		if err := common.ValidateMultipleRequiredParams(map[string]string{
			"refresh_token": refreshToken,
			"client_id":     clientID,
		}); err != nil {
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_request",
				"error_description": err.Error(),
			})
		}

		// Exchange refresh token for new tokens
		accessToken, newRefreshToken, err := h.exchangeRefreshToken(ctx.Context(), oauthSvc, refreshToken, clientID, clientSecret)
		if err != nil {
			h.logger.Error("failed to refresh tokens", zap.Error(err))

			// Return appropriate OAuth error based on the error type
			if err == auth.ErrInvalidToken {
				return apptheory.JSON(http.StatusBadRequest, map[string]string{
					"error":             "invalid_grant",
					"error_description": "Invalid or expired refresh token",
				})
			}
			if err == auth.ErrInvalidClient {
				return apptheory.JSON(http.StatusBadRequest, map[string]string{
					"error":             "invalid_client",
					"error_description": "Invalid client credentials",
				})
			}

			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_grant",
				"error_description": "Refresh token exchange failed",
			})
		}

		return okJSON(apimodels.OAuthTokenResponse{
			AccessToken:  accessToken,
			TokenType:    "Bearer",
			Scope:        "read write follow push",
			CreatedAt:    time.Now().Unix(),
			ExpiresIn:    3600,
			RefreshToken: newRefreshToken,
		})

	default:
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
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
		return "", "", errors.Join(failedToGenerateTokens(), err)
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
		return "", "", errors.Join(failedToValidateRefreshToken(), err)
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
		return "", "", errors.Join(failedToGenerateNewTokens(), err)
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
		return "", "", errors.Join(failedToStoreNewRefreshToken(), err)
	}

	// Delete the old refresh token to prevent reuse
	if err := h.repos.Account().DeleteRefreshToken(ctx, refreshToken); err != nil {
		h.logger.Error("failed to delete old refresh token", zap.Error(err))
		// Continue - new token is already stored
	}

	return accessToken, newRefreshToken, nil
}
