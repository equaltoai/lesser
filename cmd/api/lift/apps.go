package lift

import (
	"encoding/base64"
	"fmt"
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
	// Get content type - check multiple variations
	contentType := ctx.Header("Content-Type")
	if contentType == "" {
		contentType = ctx.Header("content-type")
	}

	// Normalize content type to lowercase for comparison
	contentTypeLower := strings.ToLower(contentType)

	// Get raw body
	body := string(ctx.Request.Body)

	// Log the raw request for debugging
	h.logger.Info("raw app registration request",
		zap.String("content_type", contentType),
		zap.String("body", body),
		zap.Int("body_length", len(body)))

	var req models.AppRegistrationRequest

	// Parse request based on content type
	if strings.Contains(contentTypeLower, "multipart/form-data") {
		// Parse multipart form data (used by Mastodon clients like Ivory)
		params, err := common.ParseMultipartForm(body, contentType)
		if err != nil {
			h.logger.Error("failed to parse multipart form", zap.Error(err))
			return ctx.Status(400).JSON(map[string]string{"error": err.Error()})
		}

		req.ClientName = params["client_name"]
		req.RedirectURIs = params["redirect_uris"]
		req.Scopes = params["scopes"]
		req.Website = params["website"]
	} else if strings.Contains(contentTypeLower, "application/x-www-form-urlencoded") {
		// Parse URL-encoded form data
		params, err := common.ParseFormURLEncoded(body)
		if err != nil {
			h.logger.Error("failed to parse form data", zap.Error(err))
			return ctx.Status(400).JSON(map[string]string{"error": err.Error()})
		}

		req.ClientName = params["client_name"]
		req.RedirectURIs = params["redirect_uris"]
		req.Scopes = params["scopes"]
		req.Website = params["website"]

		// Log parsed params for debugging
		h.logger.Info("parsed form params",
			zap.Any("params", params),
			zap.String("client_name_from_params", params["client_name"]))
	} else {
		// First, try to parse as form data (common for browser-based requests)
		params, formErr := common.ParseFormURLEncoded(body)
		if formErr == nil && (params["client_name"] != "" || len(params) > 0) {
			// Successfully parsed as form data
			req.ClientName = params["client_name"]
			req.RedirectURIs = params["redirect_uris"]
			req.Scopes = params["scopes"]
			req.Website = params["website"]

			h.logger.Info("parsed as form data (fallback)",
				zap.Any("params", params),
				zap.String("detected_content_type", "application/x-www-form-urlencoded"))
		} else {
			// Try JSON as last resort
			if jsonErr := ctx.ParseRequest(&req); jsonErr == nil && req.ClientName != "" {
				h.logger.Info("parsed as JSON (fallback)",
					zap.String("detected_content_type", "application/json"))
			} else {
				// Log both errors for debugging
				h.logger.Error("failed to parse request body",
					zap.Error(formErr),
					zap.Error(jsonErr),
					zap.String("body_preview", truncateStringLift(body, 200)))
				return ctx.Status(400).JSON(map[string]string{"error": "unable to parse request body as form data or JSON"})
			}
		}
	}

	// Log the parsed request for debugging
	h.logger.Info("app registration request",
		zap.String("client_name", req.ClientName),
		zap.String("redirect_uris", req.RedirectURIs),
		zap.String("scopes", req.Scopes),
		zap.String("website", req.Website))

	// Validate request
	if req.ClientName == "" {
		h.logger.Info("validation failed: client_name is required")
		return ctx.Status(422).JSON(map[string]string{"error": "client_name is required"})
	}

	if req.RedirectURIs == "" {
		h.logger.Info("validation failed: redirect_uris is required")
		return ctx.Status(422).JSON(map[string]string{"error": "redirect_uris is required"})
	}

	// Parse redirect URIs (can be space or newline separated)
	redirectURIs := strings.Fields(req.RedirectURIs)
	if len(redirectURIs) == 0 {
		h.logger.Info("validation failed: at least one redirect_uri is required")
		return ctx.Status(422).JSON(map[string]string{"error": "at least one redirect_uri is required"})
	}

	h.logger.Info("parsed redirect URIs",
		zap.Strings("redirect_uris", redirectURIs),
		zap.Int("count", len(redirectURIs)))

	// Validate redirect URIs
	for _, uri := range redirectURIs {
		if uri == "" {
			continue
		}
		// Allow special redirect URI for out-of-band flows
		if uri == "urn:ietf:wg:oauth:2.0:oob" {
			continue
		}
		// For custom app schemes, just check that there's a colon
		// Examples:
		// - com.example.app:/callback
		// - myapp://callback
		// - https://example.com/callback
		if !strings.Contains(uri, ":") {
			h.logger.Info("validation failed: invalid redirect_uri format",
				zap.String("uri", uri))
			return ctx.Status(422).JSON(map[string]string{"error": fmt.Sprintf("invalid redirect_uri format: %s", uri)})
		}
	}

	// Parse scopes
	var scopes []string
	if req.Scopes != "" {
		scopes = strings.Fields(req.Scopes)
	} else {
		// Default scopes
		scopes = []string{"read", "write"}
	}

	// Create OAuth client
	client := &storage.OAuthClient{
		Name:         req.ClientName,
		Website:      req.Website,
		RedirectURIs: redirectURIs,
		Scopes:       scopes,
	}

	h.logger.Info("creating OAuth client",
		zap.String("client_name", client.Name),
		zap.Strings("redirect_uris", client.RedirectURIs),
		zap.Strings("scopes", client.Scopes))

	if err := h.repos.Account().CreateOAuthClient(ctx.Context, client); err != nil {
		h.logger.Error("failed to create OAuth client", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get VAPID public key
	var vapidKey string
	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context)
	if err != nil {
		h.logger.Warn("failed to get VAPID keys, push notifications will not be available", zap.Error(err))
		vapidKey = ""
	} else {
		vapidKey = vapidKeys.PublicKey
	}

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
		zap.String("client_id", resp.ClientID),
		zap.String("client_secret", resp.ClientSecret))

	return ctx.JSON(resp)
}

// HandleAppVerifyCredentialsLift verifies OAuth app credentials
func (h *Handler) HandleAppVerifyCredentialsLift(ctx *lift.Context) error {
	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// This endpoint expects a Bearer token with app credentials
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Parse the token to get app credentials
	// The token should be in the format "client_id:client_secret" base64 encoded
	// or a valid OAuth access token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)

	// First try to validate as an access token
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err == nil && claims.ClientID != "" {
		// Valid access token, get the client
		client, err := h.repos.Account().GetOAuthClient(ctx.Context, claims.ClientID)
		if err != nil {
			h.logger.Error("failed to get OAuth client", zap.Error(err))
			return ctx.Status(401).JSON(map[string]string{"error": "invalid credentials"})
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
			ID:           client.ClientID,
			Name:         client.Name,
			Website:      client.Website,
			RedirectURI:  client.RedirectURIs[0], // Return first redirect URI for compatibility
			ClientID:     client.ClientID,
			ClientSecret: client.ClientSecret,
			VapidKey:     vapidKey,
		}

		return ctx.JSON(resp)
	}

	// Try to parse as basic auth (client_id:client_secret)
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "invalid credentials"})
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return ctx.Status(401).JSON(map[string]string{"error": "invalid credentials"})
	}

	clientID := parts[0]
	clientSecret := parts[1]

	// Verify client credentials
	client, err := h.repos.Account().GetOAuthClient(ctx.Context, clientID)
	if err != nil || client.ClientSecret != clientSecret {
		return ctx.Status(401).JSON(map[string]string{"error": "invalid credentials"})
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
		ID:           client.ClientID,
		Name:         client.Name,
		Website:      client.Website,
		RedirectURI:  client.RedirectURIs[0], // Return first redirect URI for compatibility
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		VapidKey:     vapidKey,
	}

	return ctx.JSON(resp)
}

// Helper functions for Lift implementation

// truncateStringLift truncates a string to a maximum length
func truncateStringLift(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
