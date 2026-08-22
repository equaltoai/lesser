package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
	frameworkoauth "github.com/theory-cloud/apptheory/v3/runtime/oauth"
)

func TestHandleOAuthAuthorizationServerMetadataLift(t *testing.T) {
	for _, allowDeviceFlow := range []bool{false, true} {
		t.Run(fmt.Sprintf("device_flow_%t_uses_framework_document", allowDeviceFlow), func(t *testing.T) {
			cfg := round11TestConfig()
			cfg.AllowDeviceFlow = allowDeviceFlow
			h, _, _ := round11NewHandler(t, cfg, nil)

			ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/oauth-authorization-server", nil, nil, nil)
			require.NoError(t, err)

			resp := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizationServerMetadataLift(ctx))
			require.Equal(t, []string{"public, max-age=300"}, resp.Headers["cache-control"])

			var body frameworkoauth.AuthorizationServerMetadata
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			var rawBody map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(resp.Body, &rawBody))
			require.Equal(t, "https://example.com", body.Issuer)
			require.Equal(t, "https://example.com/authorize", body.AuthorizationEndpoint)
			require.Equal(t, "https://example.com/token", body.TokenEndpoint)
			require.Equal(t, "https://example.com/register", body.RegistrationEndpoint)
			require.Equal(t, "https://example.com/oauth/revoke", body.RevocationEndpoint)
			if allowDeviceFlow {
				require.Equal(t, "https://example.com/oauth/device/code", body.DeviceAuthorizationEndpoint)
			} else {
				require.Empty(t, body.DeviceAuthorizationEndpoint)
			}
			_, advertisesDeviceFlow := rawBody["device_authorization_endpoint"]
			require.Equal(t, allowDeviceFlow, advertisesDeviceFlow)
			require.Empty(t, body.JWKSURI)
			require.Equal(t, []string{"code"}, body.ResponseTypesSupported)
			expectedGrantTypes := []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken}
			if allowDeviceFlow {
				expectedGrantTypes = append(expectedGrantTypes, oauthDeviceCodeGrantType)
			}
			require.Equal(t, expectedGrantTypes, body.GrantTypesSupported)
			require.Equal(t, []string{"client_secret_basic", "client_secret_post", "none"}, body.TokenEndpointAuthMethodsSupported)
			require.Equal(t, []string{"S256"}, body.CodeChallengeMethodsSupported)
			require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, auth.ScopeFollow, auth.ScopePush}, body.ScopesSupported)
		})
	}
}
