package handlers

import (
	"net/http"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

// HandleOAuthAuthorizationServerMetadataLift serves RFC 8414 authorization server metadata.
func (h *Handler) HandleOAuthAuthorizationServerMetadataLift(_ *apptheory.Context) (*apptheory.Response, error) {
	baseURL := h.cfg.BaseURL()
	metadata := map[string]any{
		"issuer":                                baseURL,
		"authorization_endpoint":                baseURL + "/oauth/authorize",
		"registration_endpoint":                 baseURL + "/oauth/register",
		"token_endpoint":                        baseURL + "/oauth/token",
		"revocation_endpoint":                   baseURL + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 oauthGrantTypesSupported(h.cfg),
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"client_id_metadata_document_supported": false,
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      auth.CanonicalOAuthScopes(),
	}
	if oauthDeviceFlowEnabled(h.cfg) {
		metadata["device_authorization_endpoint"] = baseURL + "/oauth/device/code"
	}

	resp, err := okJSON(metadata)
	if err != nil {
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "server_error",
		})
	}
	setHeader(resp, "Cache-Control", "public, max-age=300")
	return resp, nil
}

func oauthGrantTypesSupported(cfg *config.Config) []string {
	grantTypes := []string{
		"authorization_code",
		"refresh_token",
	}
	if oauthDeviceFlowEnabled(cfg) {
		grantTypes = append(grantTypes, oauthDeviceCodeGrantType)
	}
	return grantTypes
}

func oauthDeviceFlowEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.AllowDeviceFlow
}
