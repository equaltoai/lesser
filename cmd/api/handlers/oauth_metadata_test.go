package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleOAuthAuthorizationServerMetadataLift(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

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
	require.ElementsMatch(t, []any{"code"}, body["response_types_supported"].([]any))
	require.ElementsMatch(t, []any{"authorization_code", "refresh_token", "client_credentials", oauthDeviceCodeGrantType}, body["grant_types_supported"].([]any))
	require.ElementsMatch(t, []any{"client_secret_post", "none"}, body["token_endpoint_auth_methods_supported"].([]any))
	require.ElementsMatch(t, []any{"S256"}, body["code_challenge_methods_supported"].([]any))
	require.ElementsMatch(t, []any{"read", "write", "follow", "push"}, body["scopes_supported"].([]any))
}
