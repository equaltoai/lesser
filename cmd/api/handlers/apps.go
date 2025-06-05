package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleAppRegistration handles OAuth app registration requests
func (h *Handler) HandleAppRegistration(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get content type
	contentType := request.Headers["content-type"]
	if contentType == "" {
		contentType = request.Headers["Content-Type"]
	}

	// Log the raw request for debugging
	h.logger.Info("raw app registration request",
		zap.String("content_type", contentType),
		zap.Bool("is_base64_encoded", request.IsBase64Encoded),
		zap.String("body", request.Body),
		zap.Int("body_length", len(request.Body)))

	// Decode body if base64 encoded
	body := request.Body
	if request.IsBase64Encoded {
		decodedBytes, err := base64.StdEncoding.DecodeString(request.Body)
		if err != nil {
			h.logger.Error("failed to decode base64 body", zap.Error(err))
			return common.BadRequest(fmt.Errorf("failed to decode body: %w", err)), nil
		}
		body = string(decodedBytes)
	}

	var req models.AppRegistrationRequest

	// Parse request based on content type
	if strings.Contains(contentType, "multipart/form-data") {
		// Parse multipart form data (used by Mastodon clients like Ivory)
		params, err := common.ParseMultipartForm(body, contentType)
		if err != nil {
			h.logger.Error("failed to parse multipart form", zap.Error(err))
			return common.BadRequest(err), nil
		}

		req.ClientName = params["client_name"]
		req.RedirectURIs = params["redirect_uris"]
		req.Scopes = params["scopes"]
		req.Website = params["website"]
	} else if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Parse URL-encoded form data
		params, err := common.ParseFormURLEncoded(body)
		if err != nil {
			h.logger.Error("failed to parse form data", zap.Error(err))
			return common.BadRequest(err), nil
		}

		req.ClientName = params["client_name"]
		req.RedirectURIs = params["redirect_uris"]
		req.Scopes = params["scopes"]
		req.Website = params["website"]
	} else {
		// Default to JSON for backward compatibility
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			h.logger.Error("failed to parse JSON", zap.Error(err))
			return common.BadRequest(err), nil
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
		return common.UnprocessableEntity(errors.New("client_name is required")), nil
	}

	if req.RedirectURIs == "" {
		h.logger.Info("validation failed: redirect_uris is required")
		return common.UnprocessableEntity(errors.New("redirect_uris is required")), nil
	}

	// Parse redirect URIs (can be space or newline separated)
	redirectURIs := strings.Fields(req.RedirectURIs)
	if len(redirectURIs) == 0 {
		h.logger.Info("validation failed: at least one redirect_uri is required")
		return common.UnprocessableEntity(errors.New("at least one redirect_uri is required")), nil
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
			return common.UnprocessableEntity(fmt.Errorf("invalid redirect_uri format: %s", uri)), nil
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

	if err := h.store.CreateOAuthClient(ctx, client); err != nil {
		h.logger.Error("failed to create OAuth client", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	h.logger.Info("OAuth client created successfully",
		zap.String("client_id", client.ClientID),
		zap.String("client_name", client.Name))

	// Return response
	resp := models.AppRegistrationResponse{
		ID:           client.ClientID,
		Name:         client.Name,
		Website:      client.Website,
		RedirectURI:  redirectURIs[0], // Return first redirect URI for compatibility
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		VapidKey:     "", // TODO: Implement push notifications
	}

	h.logger.Info("returning app registration response",
		zap.String("client_id", resp.ClientID),
		zap.String("client_secret", resp.ClientSecret))

	return common.OK(resp), nil
}
