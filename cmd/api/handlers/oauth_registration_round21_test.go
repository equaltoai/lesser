package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHandleOAuthDynamicClientRegistrationLift_Round21(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("registers public cli client by default", func(t *testing.T) {
		state := &round10QueryState{}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/oauth/register", map[string]string{
			"Content-Type": "application/json",
		}, nil, apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:              "Claude Code",
			RedirectURIs:            []string{"http://127.0.0.1:8787/callback"},
			Scope:                   "read write",
			TokenEndpointAuthMethod: oauthTokenEndpointAuthMethodNone,
			ClientURI:               "https://claude.example/client",
			SoftwareID:              "claude-code",
			SoftwareVersion:         "1.0.0",
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusCreated)(handler.HandleOAuthDynamicClientRegistrationLift(ctx))
		var body apimodels.OAuthDynamicClientRegistrationResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.ClientID)
		require.Empty(t, body.ClientSecret)
		require.Equal(t, oauthTokenEndpointAuthMethodNone, body.TokenEndpointAuthMethod)
		require.Equal(t, auth.ClientClassCLI, body.ClientClass)
		require.Equal(t, oauthRegistrationSourceDynamic, body.RegistrationSource)
		require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken}, body.GrantTypes)

		require.Len(t, state.oauthClientsByID, 1)
		for _, client := range state.oauthClientsByID {
			require.Equal(t, auth.ClientClassCLI, client.ClientClass)
			require.False(t, client.Confidential)
			require.Equal(t, oauthRegistrationSourceDynamic, client.RegistrationSource)
			require.Equal(t, "claude-code", client.SoftwareID)
			require.Equal(t, "https://claude.example/client", client.ClientURI)
		}
	})

	t.Run("cli default adds device code when enabled", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		handler, _, _ := round11NewHandler(t, cfgDevice, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/oauth/register", map[string]string{
			"Content-Type": "application/json",
		}, nil, apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:              "Device Client",
			RedirectURIs:            []string{"http://127.0.0.1:8787/callback"},
			TokenEndpointAuthMethod: oauthTokenEndpointAuthMethodNone,
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusCreated)(handler.HandleOAuthDynamicClientRegistrationLift(ctx))
		var body apimodels.OAuthDynamicClientRegistrationResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken, oauthDeviceCodeGrantType}, body.GrantTypes)
	})

	t.Run("registers confidential owned agent client", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@owner"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodPost, "/oauth/register", map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + ownerToken,
		}, nil, apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:    "Agent Connector",
			RedirectURIs:  []string{"https://example.com/callback"},
			ClientClass:   auth.ClientClassAgent,
			AgentUsername: "agent1",
			Scope:         "read write",
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusCreated)(handler.HandleOAuthDynamicClientRegistrationLift(ctx))
		var body apimodels.OAuthDynamicClientRegistrationResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.ClientSecret)
		require.Equal(t, oauthTokenEndpointAuthMethodClientSecretPost, body.TokenEndpointAuthMethod)
		require.Equal(t, auth.ClientClassAgent, body.ClientClass)
		require.Equal(t, "agent1", body.AgentUsername)
		require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken, auth.GrantTypeClientCredentials}, body.GrantTypes)

		require.Len(t, state.oauthClientsByID, 1)
		for _, client := range state.oauthClientsByID {
			require.Equal(t, "owner", client.OwnerID)
			require.True(t, client.Confidential)
			require.Equal(t, "agent1", client.AgentUsername)
			require.Equal(t, oauthRegistrationSourceDynamic, client.RegistrationSource)
		}
	})

	t.Run("rejects agent registration without authenticated owner", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/oauth/register", map[string]string{
			"Content-Type": "application/json",
		}, nil, apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:    "Agent Connector",
			RedirectURIs:  []string{"https://example.com/callback"},
			ClientClass:   auth.ClientClassAgent,
			AgentUsername: "agent1",
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusUnauthorized)(handler.HandleOAuthDynamicClientRegistrationLift(ctx))
		require.Contains(t, string(resp.Body), "\"error\":\"access_denied\"")
	})

	t.Run("rejects invalid redirect policy", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/oauth/register", map[string]string{
			"Content-Type": "application/json",
		}, nil, apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:              "Bad Redirect Client",
			RedirectURIs:            []string{"http://example.com/callback"},
			TokenEndpointAuthMethod: oauthTokenEndpointAuthMethodNone,
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusBadRequest)(handler.HandleOAuthDynamicClientRegistrationLift(ctx))
		require.Contains(t, string(resp.Body), "\"error\":\"invalid_redirect_uri\"")
	})

	t.Run("rejects unknown metadata fields", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", map[string]string{
			"Content-Type": "application/json",
		}, nil, []byte(`{"client_name":"Unknown","redirect_uris":["https://example.com/callback"],"jwks_uri":"https://example.com/jwks"}`))

		resp := requireStatus(t, http.StatusBadRequest)(handler.HandleOAuthDynamicClientRegistrationLift(ctx))
		require.Contains(t, string(resp.Body), "\"error\":\"invalid_client_metadata\"")
	})
}

func TestOAuthAuthorizeLift_RequiresPKCEForDynamicPublicClients_Round21(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"client-public-dynamic": {
				ClientID:           "client-public-dynamic",
				ClientSecret:       "server-secret",
				Name:               "Dynamic Public Client",
				RedirectURIs:       []string{"http://127.0.0.1:8787/callback"},
				GrantTypes:         []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken},
				Scopes:             []string{auth.ScopeRead, auth.ScopeWrite},
				Confidential:       false,
				RegistrationSource: oauthRegistrationSourceDynamic,
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type": "code",
		"client_id":     "client-public-dynamic",
		"redirect_uri":  "http://127.0.0.1:8787/callback",
		"scope":         "read",
		"state":         "xyz",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusFound)(handler.HandleOAuthAuthorizeLift(ctx))
	location := strings.Join(resp.Headers["location"], "")
	require.Contains(t, location, "error=invalid_request")
	require.Contains(t, location, "public+clients+must+use+PKCE")
}
