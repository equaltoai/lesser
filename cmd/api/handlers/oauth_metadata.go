package handlers

import (
	"net/http"

	apptheory "github.com/theory-cloud/apptheory/runtime"
)

// HandleOAuthAuthorizationServerMetadataLift serves RFC 8414 authorization server metadata.
func (h *Handler) HandleOAuthAuthorizationServerMetadataLift(_ *apptheory.Context) (*apptheory.Response, error) {
	baseURL := h.cfg.BaseURL()
	resp, err := okJSON(map[string]any{
		"issuer":                                baseURL,
		"authorization_endpoint":                baseURL + "/oauth/authorize",
		"token_endpoint":                        baseURL + "/oauth/token",
		"revocation_endpoint":                   baseURL + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "client_credentials", oauthDeviceCodeGrantType},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{"read", "write", "follow", "push"},
	})
	if err != nil {
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "server_error",
		})
	}
	setHeader(resp, "Cache-Control", "public, max-age=300")
	return resp, nil
}
