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

	// Manually validate required parameters (before calling ValidateMultipleRequiredParams
	// to avoid error middleware converting ValidationError to JSON)
	if clientID == "" || redirectURI == "" {
		// If validation fails, redirect to auth-ui authorize page with error parameter
		// This will show the error inline on the styled page
		// Preserve OAuth params in redirect if available for better error context
		authDomain := fmt.Sprintf("auth.%s", h.cfg.Domain)
		errorMessage := url.QueryEscape("Invalid authorization request - missing required parameters. Please restart the authorization flow from your application.")

		// Build error URL with OAuth params if available
		errorParams := url.Values{}
		errorParams.Set("error", errorMessage)
		if clientID != "" {
			errorParams.Set("client_id", clientID)
		}
		if redirectURI != "" {
			errorParams.Set("redirect_uri", redirectURI)
		}
		if state != "" {
			errorParams.Set("state", state)
		}
		if scope != "" {
			errorParams.Set("scope", scope)
		}
		if responseType != "" {
			errorParams.Set("response_type", responseType)
		}

		errorURL := fmt.Sprintf("https://%s/oauth/authorize?%s", authDomain, errorParams.Encode())

		ctx.Response.Header("Location", errorURL)
		ctx.Status(http.StatusFound)
		return nil
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

	// Note: We skip the general ValidateRedirectURL check here because:
	// 1. OAuth redirect URIs are already validated via ValidateRedirectURI which checks against
	//    the client's registered redirect URIs
	// 2. OAuth clients can legitimately redirect to localhost (for local dev) or other registered domains
	// 3. The OAuth spec requires exact matching of registered redirect URIs, which we enforce above
	// The ValidateRedirectURI check at line 50 is sufficient for OAuth security

	// Check if user is authenticated (from cookie, header, or query param token)
	// Also check for access_token in query params (for cross-domain auth flow from auth subdomain)
	// This is a temporary mechanism to pass authentication from auth.dev.lesser.host to dev.lesser.host
	// The token is validated immediately and not exposed to the final client redirect
	accessTokenFromQuery := ctx.Query("access_token")
	if accessTokenFromQuery != "" {
		if decodedToken, err := url.QueryUnescape(accessTokenFromQuery); err == nil {
			accessTokenFromQuery = strings.ReplaceAll(decodedToken, " ", "+")
		} else {
			h.logger.Warn("failed to decode access_token query parameter",
				zap.Error(err))
		}
	}

	username := h.getUserFromSessionLift(ctx)

	// If not authenticated via normal methods and we have a token from query param,
	// validate it and use it for authentication
	if username == "" && accessTokenFromQuery != "" {
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
		if claims, err := oauthSvc.ValidateAccessToken(accessTokenFromQuery); err == nil {
			username = claims.Username
		}
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		// User needs to login first - redirect to passwordless auth UI
		// Build auth subdomain from config
		authDomain := fmt.Sprintf("auth.%s", h.cfg.Domain)

		// Store authorization request in session for after login
		authRequest := map[string]string{
			"client_id":             clientID,
			"redirect_uri":          redirectURI,
			"scope":                 scope,
			"state":                 state,
			"response_type":         responseType, // Include response_type in auth_request
			"code_challenge":        codeChallenge,
			"code_challenge_method": codeChallengeMethod,
		}

		// Encode request as query string
		authRequestEncoded, _ := json.Marshal(authRequest)

		// Redirect to passwordless login UI (hosted on auth.{domain})
		loginURL := fmt.Sprintf("https://%s/login?return_to=%s&auth_request=%s",
			authDomain,
			url.QueryEscape(fmt.Sprintf("https://%s/oauth/authorize", h.cfg.Domain)),
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

	// Check if registry is initialized
	if h.registry == nil {
		h.logger.Error("service registry not initialized for OAuth authorization")
		return h.oauthErrorLift(ctx, "server_error", "Service unavailable", redirectURI, state)
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

		// Get access token from query param to pass to consent UI (for stateless auth)
		accessToken := accessTokenFromQuery
		if accessToken == "" {
			// Try to get from Authorization header as fallback
			authHeader := ctx.Header("Authorization")
			if authHeader != "" {
				if token, err := auth.ExtractBearerToken(authHeader); err == nil {
					accessToken = token
				}
			}
		}

		h.logger.Info("redirecting to consent UI",
			zap.String("username", username),
			zap.String("client_id", clientID),
			zap.Bool("has_access_token", accessToken != ""))

		// Redirect to hosted consent UI instead of inline HTML
		return h.redirectToConsentUI(ctx, authState, accessToken)
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
	// Check for authentication context from unified middleware
	if username := ctx.Get("username"); username != nil {
		if usernameStr, ok := username.(string); ok && usernameStr != "" {
			return usernameStr
		}
	}

	// Check for Bearer token in Authorization header (for cross-subdomain auth)
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
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
func (h *Handler) redirectToConsentUI(ctx *lift.Context, authState *storage.OAuthState, accessToken string) error {
	// Check if registry is initialized
	if h.registry == nil {
		h.logger.Error("service registry not initialized for OAuth consent")
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error":             "server_error",
			"error_description": "Service unavailable",
		})
	}

	// Get app details to pass to consent UI
	result, err := h.registry.Accounts().GetOAuthApp(ctx.Context, &accounts.GetOAuthAppQuery{
		ClientID: authState.ClientID,
	})
	if err != nil {
		h.logger.Error("failed to get OAuth app", zap.Error(err))
		return errors.New("client not found")
	}
	app := result.App

	// Build auth subdomain
	authDomain := fmt.Sprintf("auth.%s", h.cfg.Domain)

	// Build consent URL with all necessary parameters including access_token for stateless auth
	consentURL := fmt.Sprintf("https://%s/consent?state=%s&client_id=%s&client_name=%s&client_url=%s&scopes=%s&redirect_uri=%s&access_token=%s",
		authDomain,
		url.QueryEscape(authState.State),
		url.QueryEscape(authState.ClientID),
		url.QueryEscape(app.Name),
		url.QueryEscape(app.Website),
		url.QueryEscape(strings.Join(authState.Scopes, " ")),
		url.QueryEscape(authState.RedirectURI),
		url.QueryEscape(accessToken))

	ctx.Response.Header("Location", consentURL)
	ctx.Status(http.StatusFound)
	return nil
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
