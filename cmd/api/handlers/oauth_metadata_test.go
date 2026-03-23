package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestHandleOAuthAuthorizationServerMetadataLift(t *testing.T) {
	t.Run("device flow enabled advertises device metadata", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = true
		h, _, _ := round11NewHandler(t, cfg, nil)

		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/oauth-authorization-server", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizationServerMetadataLift(ctx))
		require.Equal(t, []string{"public, max-age=300"}, resp.Headers["cache-control"])

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "https://example.com", body["issuer"])
		require.Equal(t, "https://example.com/oauth/authorize", body["authorization_endpoint"])
		require.Equal(t, "https://example.com/oauth/token", body["token_endpoint"])
		require.Equal(t, "https://example.com/oauth/revoke", body["revocation_endpoint"])
		require.Equal(t, "https://example.com/oauth/device/code", body["device_authorization_endpoint"])
		require.Equal(t, false, body["client_id_metadata_document_supported"])
		require.ElementsMatch(t, []any{"code"}, body["response_types_supported"].([]any))
		require.ElementsMatch(t, []any{"authorization_code", "refresh_token", "client_credentials", oauthDeviceCodeGrantType}, body["grant_types_supported"].([]any))
		require.ElementsMatch(t, []any{"client_secret_basic", "client_secret_post", "none"}, body["token_endpoint_auth_methods_supported"].([]any))
		require.ElementsMatch(t, []any{"S256"}, body["code_challenge_methods_supported"].([]any))
		require.ElementsMatch(t, []any{auth.ScopeRead, auth.ScopeWrite, auth.ScopeFollow, auth.ScopePush}, body["scopes_supported"].([]any))
	})

	t.Run("device flow disabled omits device metadata", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = false
		h, _, _ := round11NewHandler(t, cfg, nil)

		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/oauth-authorization-server", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizationServerMetadataLift(ctx))

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, false, body["client_id_metadata_document_supported"])
		require.NotContains(t, body, "device_authorization_endpoint")
		require.ElementsMatch(t, []any{"authorization_code", "refresh_token", "client_credentials"}, body["grant_types_supported"].([]any))
	})
}
