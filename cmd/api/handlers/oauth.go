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

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleOAuthAuthorize handles the OAuth authorization endpoint
// GET /oauth/authorize
func (h *Handler) HandleOAuthAuthorize(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract query parameters
	params := request.QueryStringParameters

	// Required parameters
	responseType := params["response_type"]
	clientID := params["client_id"]
	redirectURI := params["redirect_uri"]

	// Optional parameters
	scope := params["scope"]
	state := params["state"]
	codeChallenge := params["code_challenge"]
	codeChallengeMethod := params["code_challenge_method"]

	// Validate required parameters
	if responseType != "code" {
		return h.oauthError("unsupported_response_type", "Only 'code' response type is supported", redirectURI, state), nil
	}

	if clientID == "" {
		return common.BadRequest(errors.New("client_id is required")), nil
	}

	if redirectURI == "" {
		return common.BadRequest(errors.New("redirect_uri is required")), nil
	}

	// Initialize OAuth service
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)

	// Validate client and redirect URI
	if err := oauthSvc.ValidateRedirectURI(ctx, clientID, redirectURI); err != nil {
		h.logger.Error("invalid redirect URI",
			zap.String("client_id", clientID),
			zap.String("redirect_uri", redirectURI),
			zap.Error(err))
		return common.BadRequest(errors.New("invalid redirect_uri")), nil
	}

	// Check if user is authenticated (from cookie or header)
	username := h.getUserFromSession(request)

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

		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusFound,
			Headers: map[string]string{
				"Location": loginURL,
			},
		}, nil
	}

	// Parse requested scopes
	scopes := []string{"read", "write"} // Default scopes
	if scope != "" {
		scopes = strings.Fields(scope)
	}

	// Validate scopes
	if err := auth.ValidateScopes(scopes); err != nil {
		return h.oauthError("invalid_scope", "One or more requested scopes are invalid", redirectURI, state), nil
	}

	// TODO: Show consent screen if user hasn't consented to this app
	// For now, auto-approve

	// Generate authorization code
	code, err := oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		h.logger.Error("failed to generate authorization code", zap.Error(err))
		return h.oauthError("server_error", "Failed to generate authorization code", redirectURI, state), nil
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

	if err := h.store.CreateAuthorizationCode(ctx, authCode); err != nil {
		h.logger.Error("failed to store authorization code", zap.Error(err))
		return h.oauthError("server_error", "Failed to store authorization code", redirectURI, state), nil
	}

	// Build redirect URL
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusFound,
		Headers: map[string]string{
			"Location": u.String(),
		},
	}, nil
}

// HandleOAuthToken handles the OAuth token endpoint
// POST /oauth/token
func (h *Handler) HandleOAuthToken(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req models.OAuthTokenRequest

	// Parse request based on content type
	contentType := request.Headers["content-type"]
	if contentType == "" {
		contentType = request.Headers["Content-Type"]
	}

	// OAuth token endpoint must accept form-encoded data
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		params, err := common.ParseFormURLEncoded(request.Body)
		if err != nil {
			return h.tokenError("invalid_request", "Failed to parse request"), nil
		}

		req.GrantType = params["grant_type"]
		req.Code = params["code"]
		req.RedirectURI = params["redirect_uri"]
		req.ClientID = params["client_id"]
		req.ClientSecret = params["client_secret"]
		req.CodeVerifier = params["code_verifier"]
		req.RefreshToken = params["refresh_token"]
		req.Scope = params["scope"]
	} else {
		// Also support JSON for compatibility
		if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
			return h.tokenError("invalid_request", "Failed to parse request"), nil
		}
	}

	// Initialize OAuth service
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)

	// Validate client credentials
	if err := oauthSvc.ValidateClient(ctx, req.ClientID, req.ClientSecret); err != nil {
		return h.tokenError("invalid_client", "Invalid client credentials"), nil
	}

	// Handle different grant types
	switch req.GrantType {
	case "authorization_code":
		return h.handleAuthorizationCodeGrant(ctx, req, oauthSvc)
	case "refresh_token":
		return h.handleRefreshTokenGrant(ctx, req, oauthSvc)
	default:
		return h.tokenError("unsupported_grant_type", "Grant type not supported"), nil
	}
}

// handleAuthorizationCodeGrant handles the authorization code grant flow
func (h *Handler) handleAuthorizationCodeGrant(ctx context.Context, req models.OAuthTokenRequest, oauthSvc *auth.OAuthService) (*events.APIGatewayV2HTTPResponse, error) {
	// Validate required parameters
	if req.Code == "" {
		return h.tokenError("invalid_request", "Authorization code is required"), nil
	}

	// Get authorization code
	authCode, err := h.store.GetAuthorizationCode(ctx, req.Code)
	if err != nil || authCode == nil {
		return h.tokenError("invalid_grant", "Invalid authorization code"), nil
	}

	// Check if code is expired
	if time.Now().After(authCode.ExpiresAt) {
		h.store.DeleteAuthorizationCode(ctx, req.Code)
		return h.tokenError("invalid_grant", "Authorization code has expired"), nil
	}

	// Validate client ID
	if authCode.ClientID != req.ClientID {
		return h.tokenError("invalid_grant", "Authorization code was issued to a different client"), nil
	}

	// Verify PKCE if used
	if authCode.CodeChallenge != "" {
		if req.CodeVerifier == "" {
			return h.tokenError("invalid_request", "Code verifier is required"), nil
		}

		challengeMethod := "S256" // Default to S256
		if err := oauthSvc.VerifyCodeChallenge(authCode.CodeChallenge, req.CodeVerifier, challengeMethod); err != nil {
			return h.tokenError("invalid_grant", "Code verifier does not match"), nil
		}
	}

	// Generate tokens
	accessToken, refreshToken, err := oauthSvc.GenerateTokens(authCode.Username, authCode.ClientID, authCode.Scopes)
	if err != nil {
		h.logger.Error("failed to generate tokens", zap.Error(err))
		return h.tokenError("server_error", "Failed to generate tokens"), nil
	}

	// Store refresh token
	refreshTokenRecord := &storage.RefreshToken{
		Token:     refreshToken,
		ClientID:  authCode.ClientID,
		Username:  authCode.Username,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour), // 30 days
		Scopes:    authCode.Scopes,
	}

	if err := h.store.CreateRefreshToken(ctx, refreshTokenRecord); err != nil {
		h.logger.Error("failed to store refresh token", zap.Error(err))
		return h.tokenError("server_error", "Failed to store refresh token"), nil
	}

	// Delete used authorization code
	h.store.DeleteAuthorizationCode(ctx, req.Code)

	// Return tokens
	resp := models.OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600, // 1 hour
		RefreshToken: refreshToken,
		Scope:        strings.Join(authCode.Scopes, " "),
		CreatedAt:    time.Now().Unix(),
	}

	return common.OK(resp), nil
}

// handleRefreshTokenGrant handles the refresh token grant flow
func (h *Handler) handleRefreshTokenGrant(ctx context.Context, req models.OAuthTokenRequest, oauthSvc *auth.OAuthService) (*events.APIGatewayV2HTTPResponse, error) {
	// Validate required parameters
	if req.RefreshToken == "" {
		return h.tokenError("invalid_request", "Refresh token is required"), nil
	}

	// Get refresh token
	refreshToken, err := h.store.GetRefreshToken(ctx, req.RefreshToken)
	if err != nil || refreshToken == nil {
		return h.tokenError("invalid_grant", "Invalid refresh token"), nil
	}

	// Check if token is expired
	if time.Now().After(refreshToken.ExpiresAt) {
		h.store.DeleteRefreshToken(ctx, req.RefreshToken)
		return h.tokenError("invalid_grant", "Refresh token has expired"), nil
	}

	// Validate client ID
	if refreshToken.ClientID != req.ClientID {
		return h.tokenError("invalid_grant", "Refresh token was issued to a different client"), nil
	}

	// Determine scopes
	scopes := refreshToken.Scopes
	if req.Scope != "" {
		// Can only request subset of original scopes
		requestedScopes := strings.Fields(req.Scope)
		for _, scope := range requestedScopes {
			found := false
			for _, originalScope := range refreshToken.Scopes {
				if scope == originalScope {
					found = true
					break
				}
			}
			if !found {
				return h.tokenError("invalid_scope", "Requested scope exceeds original grant"), nil
			}
		}
		scopes = requestedScopes
	}

	// Generate new access token
	accessToken, _, err := oauthSvc.GenerateTokens(refreshToken.Username, refreshToken.ClientID, scopes)
	if err != nil {
		h.logger.Error("failed to generate access token", zap.Error(err))
		return h.tokenError("server_error", "Failed to generate access token"), nil
	}

	// Return new access token (keep same refresh token)
	resp := models.OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600,             // 1 hour
		RefreshToken: req.RefreshToken, // Return same refresh token
		Scope:        strings.Join(scopes, " "),
		CreatedAt:    time.Now().Unix(),
	}

	return common.OK(resp), nil
}

// HandleOAuthRevoke handles token revocation
// POST /oauth/revoke
func (h *Handler) HandleOAuthRevoke(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req models.OAuthRevokeRequest

	// Parse request
	contentType := request.Headers["content-type"]
	if contentType == "" {
		contentType = request.Headers["Content-Type"]
	}

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		params, err := common.ParseFormURLEncoded(request.Body)
		if err != nil {
			return common.BadRequest(err), nil
		}

		req.Token = params["token"]
		req.TokenTypeHint = params["token_type_hint"]
		req.ClientID = params["client_id"]
		req.ClientSecret = params["client_secret"]
	} else {
		if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
			return common.BadRequest(err), nil
		}
	}

	// Validate client credentials
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	if err := oauthSvc.ValidateClient(ctx, req.ClientID, req.ClientSecret); err != nil {
		return common.Unauthorized(errors.New("invalid client credentials")), nil
	}

	// Try to revoke as refresh token first
	if req.TokenTypeHint == "" || req.TokenTypeHint == "refresh_token" {
		refreshToken, err := h.store.GetRefreshToken(ctx, req.Token)
		if err == nil && refreshToken != nil && refreshToken.ClientID == req.ClientID {
			h.store.DeleteRefreshToken(ctx, req.Token)
			return common.OK(nil), nil
		}
	}

	// Try to revoke as access token
	if req.TokenTypeHint == "" || req.TokenTypeHint == "access_token" {
		// Parse JWT to check if it's valid
		claims, err := oauthSvc.ValidateAccessToken(req.Token)
		if err == nil && claims.ClientID == req.ClientID {
			// Access tokens are stateless, so we can't revoke them directly
			// In a production system, you might want to maintain a revocation list
			// For now, just return success
			return common.OK(nil), nil
		}
	}

	// Token not found or doesn't belong to client, but still return success
	// per OAuth spec
	return common.OK(nil), nil
}

// oauthError returns an OAuth error response
func (h *Handler) oauthError(error, description, redirectURI, state string) *events.APIGatewayV2HTTPResponse {
	if redirectURI != "" {
		// Redirect with error
		u, _ := url.Parse(redirectURI)
		q := u.Query()
		q.Set("error", error)
		if description != "" {
			q.Set("error_description", description)
		}
		if state != "" {
			q.Set("state", state)
		}
		u.RawQuery = q.Encode()

		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusFound,
			Headers: map[string]string{
				"Location": u.String(),
			},
		}
	}

	// Return JSON error
	return common.BadRequest(fmt.Errorf("%s: %s", error, description))
}

// tokenError returns an OAuth token error response
func (h *Handler) tokenError(error, description string) *events.APIGatewayV2HTTPResponse {
	resp := map[string]string{
		"error": error,
	}
	if description != "" {
		resp["error_description"] = description
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusBadRequest,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Cache-Control":               "no-store",
			"Pragma":                      "no-cache",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(body),
	}
}

// getUserFromSession extracts the username from session cookie or JWT
func (h *Handler) getUserFromSession(request events.APIGatewayV2HTTPRequest) string {
	// First try JWT from Authorization header
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

	// Try session cookie
	cookies := request.Headers["Cookie"]
	if cookies == "" {
		cookies = request.Headers["cookie"]
	}

	// Simple cookie parsing (in production, use a proper parser)
	for _, cookie := range strings.Split(cookies, ";") {
		parts := strings.SplitN(strings.TrimSpace(cookie), "=", 2)
		if len(parts) == 2 && parts[0] == "lesser_session" {
			// Validate session token
			authService, _ := auth.NewAuthService(h.store)
			claims, err := authService.ValidateAccessToken(parts[1])
			if err == nil {
				return claims.Username
			}
		}
	}

	return ""
}
