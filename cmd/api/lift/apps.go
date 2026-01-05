package lift

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleAppRegistrationLift handles OAuth app registration requests
func (h *Handler) HandleAppRegistrationLift(ctx *lift.Context) error {
	// Parse and validate request
	req, err := h.parseAppRegistrationRequest(ctx)
	if err != nil {
		return err
	}

	// Validate application parameters
	redirectURIs, err := h.validateAppRegistrationParams(ctx, &req)
	if err != nil {
		return err
	}

	// Create OAuth client and return response
	return h.createOAuthClientAndRespond(ctx, &req, redirectURIs)
}

// HandleAppVerifyCredentialsLift verifies OAuth app credentials
func (h *Handler) HandleAppVerifyCredentialsLift(ctx *lift.Context) error {
	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("auth_header", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	// This endpoint expects a Bearer token with app credentials
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return h.respondUnauthorized(ctx)
	}

	// Parse the token to get app credentials
	// The token should be in the format "client_id:client_secret" base64 encoded
	// or a valid OAuth access token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)

	// First try to validate as an access token
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err == nil && claims.ClientID != "" {
		// Valid access token, get the client
		client, err := h.repos.Account().GetOAuthClient(ctx.Context, claims.ClientID)
		if err != nil {
			h.logger.Error("failed to get OAuth client", zap.Error(err))
			return h.respondUnauthorized(ctx)
		}

		// Get VAPID public key
		var vapidKey string
		vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context)
		if err != nil {
			h.logger.Warn("failed to get VAPID keys", zap.Error(err))
			vapidKey = ""
		} else {
			vapidKey = vapidKeys.PublicKey
		}

		resp := models.AppRegistrationResponse{
			ID:          client.ClientID,
			Name:        client.Name,
			Website:     client.Website,
			RedirectURI: client.RedirectURIs[0], // Return first redirect URI for compatibility
			ClientID:    client.ClientID,
			VapidKey:    vapidKey,
		}

		return ctx.JSON(resp)
	}

	// Try to parse as basic auth (client_id:client_secret)
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return common.RespondUnauthorized(ctx, "invalid credentials")
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return common.RespondUnauthorized(ctx, "invalid credentials")
	}

	clientID := parts[0]
	clientSecret := parts[1]

	// Verify client credentials
	if err := oauthSvc.ValidateClient(ctx.Context, clientID, clientSecret); err != nil {
		return common.RespondUnauthorized(ctx, "invalid credentials")
	}

	client, err := h.repos.Account().GetOAuthClient(ctx.Context, clientID)
	if err != nil {
		return common.RespondUnauthorized(ctx, "invalid credentials")
	}

	// Get VAPID public key
	var vapidKey string
	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context)
	if err != nil {
		h.logger.Warn("failed to get VAPID keys", zap.Error(err))
		vapidKey = ""
	} else {
		vapidKey = vapidKeys.PublicKey
	}

	resp := models.AppRegistrationResponse{
		ID:          client.ClientID,
		Name:        client.Name,
		Website:     client.Website,
		RedirectURI: client.RedirectURIs[0], // Return first redirect URI for compatibility
		ClientID:    client.ClientID,
		VapidKey:    vapidKey,
	}

	return ctx.JSON(resp)
}

// Helper functions for Lift implementation

// parseAppRegistrationRequest parses the incoming request based on content type
func (h *Handler) parseAppRegistrationRequest(ctx *lift.Context) (models.AppRegistrationRequest, error) {

	// Get and normalize content type
	contentType := ctx.Header("Content-Type")
	if contentType == "" {
		contentType = ctx.Header("content-type")
	}
	contentTypeLower := strings.ToLower(contentType)

	// Get raw body
	body := string(ctx.Request.Body)

	// Log the raw request for debugging
	h.logger.Info("raw app registration request",
		zap.String("content_type", contentType),
		zap.String("body", body),
		zap.Int("body_length", len(body)))

	// Parse based on content type
	if strings.Contains(contentTypeLower, "application/json") {
		// Parse as JSON
		var req models.AppRegistrationRequest
		if err := ctx.ParseRequest(&req); err != nil {
			h.logger.Error("failed to parse JSON request", zap.Error(err), zap.String("body", body))
			return models.AppRegistrationRequest{}, err
		}
		h.logger.Info("parsed as JSON", zap.String("client_name", req.ClientName))
		return req, nil
	}
	if strings.Contains(contentTypeLower, "multipart/form-data") {
		return h.parseMultipartFormRequest(body, contentType)
	}
	if strings.Contains(contentTypeLower, "application/x-www-form-urlencoded") {
		return h.parseFormURLEncodedRequest(body)
	}
	return h.parseFallbackRequest(ctx, body)
}

// parseMultipartFormRequest parses multipart form data
func (h *Handler) parseMultipartFormRequest(body, contentType string) (models.AppRegistrationRequest, error) {
	params, err := common.ParseMultipartForm(body, contentType)
	if err != nil {
		h.logger.Error("failed to parse multipart form", zap.Error(err))
		return models.AppRegistrationRequest{}, errors.Join(failedToParseMultipartForm(), err)
	}

	return h.buildRequestFromParams(params)
}

// parseFormURLEncodedRequest parses URL-encoded form data
func (h *Handler) parseFormURLEncodedRequest(body string) (models.AppRegistrationRequest, error) {
	params, err := common.ParseFormURLEncoded(body)
	if err != nil {
		h.logger.Error("failed to parse form data", zap.Error(err))
		return models.AppRegistrationRequest{}, errors.Join(failedToParseFormData(), err)
	}

	// Log parsed params for debugging
	h.logger.Info("parsed form params",
		zap.Any("params", params),
		zap.String("client_name_from_params", params["client_name"]))

	return h.buildRequestFromParams(params)
}

// parseFallbackRequest attempts to parse request as form data or JSON
func (h *Handler) parseFallbackRequest(ctx *lift.Context, body string) (models.AppRegistrationRequest, error) {
	// First, try to parse as form data
	params, formErr := common.ParseFormURLEncoded(body)
	if formErr == nil && (params["client_name"] != "" || len(params) > 0) {
		h.logger.Info("parsed as form data (fallback)",
			zap.Any("params", params),
			zap.String("detected_content_type", "application/x-www-form-urlencoded"))
		return h.buildRequestFromParams(params)
	}

	// Try JSON as last resort
	var req models.AppRegistrationRequest
	if jsonErr := ctx.ParseRequest(&req); jsonErr == nil && req.ClientName != "" {
		h.logger.Info("parsed as JSON (fallback)",
			zap.String("detected_content_type", "application/json"))
		return req, nil
	}

	// Log both errors for debugging
	h.logger.Error("failed to parse request body",
		zap.Error(formErr),
		zap.String("body_preview", truncateStringLift(body, 200)))
	return models.AppRegistrationRequest{}, unableToParseRequestBodyAsFormOrJSON()
}

// buildRequestFromParams builds AppRegistrationRequest from parsed parameters
func (h *Handler) buildRequestFromParams(params map[string]string) (models.AppRegistrationRequest, error) {
	if err := common.ValidateApplicationName(params["client_name"]); err != nil {
		return models.AppRegistrationRequest{}, err
	}
	if err := common.ValidateRedirectURIs(params["redirect_uris"]); err != nil {
		return models.AppRegistrationRequest{}, err
	}

	req := models.AppRegistrationRequest{
		ClientName:   params["client_name"],
		RedirectURIs: params["redirect_uris"],
		Scopes:       params["scopes"],
		Website:      params["website"],
	}

	return req, nil
}

// validateAppRegistrationParams validates the parsed request parameters
func (h *Handler) validateAppRegistrationParams(ctx *lift.Context, req *models.AppRegistrationRequest) ([]string, error) {
	// Log the parsed request for debugging
	h.logger.Info("app registration request",
		zap.String("client_name", req.ClientName),
		zap.String("redirect_uris", req.RedirectURIs),
		zap.String("scopes", req.Scopes),
		zap.String("website", req.Website))

	// Validate application registration parameters
	params := map[string]interface{}{
		"client_name":   req.ClientName,
		"redirect_uris": req.RedirectURIs,
		"scopes":        req.Scopes,
		"website":       req.Website,
	}
	if err := common.ValidateApplicationParams(params); err != nil {
		h.logger.Info("application validation failed", zap.Error(err))
		return nil, h.respondUnprocessableEntity(ctx, err.Error())
	}

	// Validate required parameters
	if err := h.validateRequiredAppParams(ctx, req); err != nil {
		return nil, err
	}

	// Parse and validate redirect URIs
	return h.parseAndValidateRedirectURIs(ctx, req.RedirectURIs)
}

// validateRequiredAppParams validates required application parameters
func (h *Handler) validateRequiredAppParams(ctx *lift.Context, req *models.AppRegistrationRequest) error {
	if err := common.ValidateRequiredParam("client_name", req.ClientName); err != nil {
		h.logger.Info("validation failed: client_name is required")
		return h.respondUnprocessableEntity(ctx, err.Error())
	}

	if err := common.ValidateRequiredParam("redirect_uris", req.RedirectURIs); err != nil {
		h.logger.Info("validation failed: redirect_uris is required")
		return h.respondUnprocessableEntity(ctx, err.Error())
	}

	return nil
}

// parseAndValidateRedirectURIs parses and validates redirect URIs
func (h *Handler) parseAndValidateRedirectURIs(ctx *lift.Context, redirectURIString string) ([]string, error) {
	// Parse redirect URIs (can be space or newline separated)
	redirectURIs := strings.Fields(redirectURIString)
	if err := common.ValidateSliceNotEmpty("redirect_uris", redirectURIs); err != nil {
		h.logger.Info("validation failed: at least one redirect_uri is required")
		return nil, h.respondUnprocessableEntity(ctx, "at least one redirect_uri is required")
	}

	h.logger.Info("parsed redirect URIs",
		zap.Strings("redirect_uris", redirectURIs),
		zap.Int("count", len(redirectURIs)))

	// Validate each redirect URI
	for _, uri := range redirectURIs {
		if err := h.validateSingleRedirectURI(ctx, uri); err != nil {
			return nil, err
		}
	}

	return redirectURIs, nil
}

// validateSingleRedirectURI validates a single redirect URI
func (h *Handler) validateSingleRedirectURI(ctx *lift.Context, uri string) error {
	if err := common.ValidateRequiredParam("uri", uri); err != nil {
		return nil // Skip empty URIs
	}

	// Allow special redirect URI for out-of-band flows
	if uri == "urn:ietf:wg:oauth:2.0:oob" {
		return nil
	}

	// For custom app schemes, just check that there's a colon
	if !strings.Contains(uri, ":") {
		h.logger.Info("validation failed: invalid redirect_uri format",
			zap.String("uri", uri))
		return h.respondUnprocessableEntity(ctx, invalidRedirectURIFormat().Error())
	}

	return nil
}

// createOAuthClientAndRespond creates OAuth client and returns response
func (h *Handler) createOAuthClientAndRespond(ctx *lift.Context, req *models.AppRegistrationRequest, redirectURIs []string) error {
	// Parse scopes
	scopes := h.parseScopes(req.Scopes)

	// Try to extract authenticated user (optional - public registration allowed for initial OAuth flow)
	// But if authenticated, set OwnerID for security and proper ownership
	ownerID := h.getOptionalAuthenticatedUser(ctx)

	// Create OAuth client
	client := &storage.OAuthClient{
		Name:         req.ClientName,
		Website:      req.Website,
		RedirectURIs: redirectURIs,
		Scopes:       scopes,
		OwnerID:      ownerID, // Set owner if authenticated
	}

	h.logger.Info("creating OAuth client",
		zap.String("client_name", client.Name),
		zap.Strings("redirect_uris", client.RedirectURIs),
		zap.Strings("scopes", client.Scopes),
		zap.String("owner_id", ownerID))

	if err := h.repos.Account().CreateOAuthClient(ctx.Context, client); err != nil {
		h.logger.Error("failed to create OAuth client", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Get VAPID public key
	vapidKey := h.getVAPIDKey()

	// Return response
	resp := models.AppRegistrationResponse{
		ID:           client.ClientID,
		Name:         client.Name,
		Website:      client.Website,
		RedirectURI:  redirectURIs[0], // Return first redirect URI for compatibility
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		VapidKey:     vapidKey,
	}

	h.logger.Info("returning app registration response",
		zap.String("client_id", resp.ClientID))

	return ctx.JSON(resp)
}

// parseScopes parses the scopes string into a slice
func (h *Handler) parseScopes(scopesString string) []string {
	if scopesString != "" {
		return strings.Fields(scopesString)
	}
	// Default scopes
	return []string{"read", "write"}
}

// getVAPIDKey retrieves the VAPID public key
func (h *Handler) getVAPIDKey() string {
	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(context.Background())
	if err != nil {
		h.logger.Warn("failed to get VAPID keys, push notifications will not be available", zap.Error(err))
		return ""
	}
	return vapidKeys.PublicKey
}

// truncateStringLift truncates a string to a maximum length
func truncateStringLift(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
