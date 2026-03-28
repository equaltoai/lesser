package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	storagepkg "github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthClientHelperCoverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("validate client secret if omitted skips validation", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, nil)

		resp, err := h.validateOAuthClientSecretIfProvided(context.Background(), oauthSvc, "client-1", "")
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("validate client secret if invalid returns invalid_client", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Confidential: true,
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, nil)

		resp, err := h.validateOAuthClientSecretIfProvided(context.Background(), oauthSvc, "client-1", "wrong")
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
	})

	t.Run("validate client secret if valid succeeds", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Confidential: true,
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, nil)

		resp, err := h.validateOAuthClientSecretIfProvided(context.Background(), oauthSvc, "client-1", "secret")
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("oauth client supports grant type defaults and explicit lists", func(t *testing.T) {
		require.False(t, oauthClientSupportsGrantType(nil, auth.GrantTypeAuthorizationCode))
		require.True(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{}, auth.GrantTypeAuthorizationCode))
		require.True(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{ClientClass: auth.ClientClassCLI}, oauthDeviceCodeGrantType))
		require.False(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{ClientClass: auth.ClientClassAgent}, oauthDeviceCodeGrantType))
		require.False(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{ClientClass: auth.ClientClassAgent}, auth.GrantTypeClientCredentials))
		require.True(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{GrantTypes: []string{auth.GrantTypeRefreshToken}}, auth.GrantTypeRefreshToken))
		require.False(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{GrantTypes: []string{auth.GrantTypeRefreshToken}}, auth.GrantTypeAuthorizationCode))
	})

	t.Run("internal runtime refresh validation rejects missing confidential secret and records diagnostics", func(t *testing.T) {
		now := time.Now().UTC()
		token := buildRuntimeRefreshToken(t, "rt-agent-1", "agent1", delegatedAgentClientID, "sid-agent-1", "family-agent-1", "connector-app", 1, true, false, now)
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				delegatedAgentClientID: {
					ClientID:     delegatedAgentClientID,
					ClientSecret: "secret",
					ClientClass:  auth.ClientClassAgent,
					Confidential: true,
					GrantTypes:   []string{auth.GrantTypeRefreshToken},
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				token.Token: token,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, nil)

		_, err := h.validateRefreshGrantClient(context.Background(), oauthSvc, "rt-agent-1", delegatedAgentClientID, "")
		require.ErrorIs(t, err, auth.ErrInvalidClient)

		updated := state.refreshTokensByToken["rt-agent-1"]
		require.Equal(t, "invalid_client", updated.LastAuthFailureCode)
		require.Equal(t, "Invalid client credentials", updated.LastAuthFailureMsg)
	})

	t.Run("internal runtime refresh validation rejects unsupported grant and records diagnostics", func(t *testing.T) {
		now := time.Now().UTC()
		token := buildRuntimeRefreshToken(t, "rt-agent-2", "agent1", delegatedAgentClientID, "sid-agent-2", "family-agent-2", "connector-app", 1, true, false, now)
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				delegatedAgentClientID: {
					ClientID:     delegatedAgentClientID,
					ClientSecret: "secret",
					ClientClass:  auth.ClientClassAgent,
					GrantTypes:   []string{auth.GrantTypeAuthorizationCode},
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				token.Token: token,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, nil)

		_, err := h.validateRefreshGrantClient(context.Background(), oauthSvc, "rt-agent-2", delegatedAgentClientID, "secret")
		require.ErrorIs(t, err, auth.ErrUnauthorizedClient)

		updated := state.refreshTokensByToken["rt-agent-2"]
		require.Equal(t, "unauthorized_client", updated.LastAuthFailureCode)
		require.Equal(t, "refresh_token is not allowed for this client", updated.LastAuthFailureMsg)
	})

	t.Run("refresh grant secret validation helper handles public and valid confidential clients", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"confidential-client": {
					ClientID:     "confidential-client",
					ClientSecret: "top-secret",
					Confidential: true,
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, nil)

		err := validateRefreshGrantClientSecret(context.Background(), oauthSvc, &storagepkg.OAuthClient{
			ClientID:     "public-client",
			Confidential: false,
		}, "public-client", "")
		require.NoError(t, err)

		err = validateRefreshGrantClientSecret(context.Background(), oauthSvc, &storagepkg.OAuthClient{
			ClientID:     "confidential-client",
			Confidential: true,
			ClientSecret: "top-secret",
		}, "confidential-client", "top-secret")
		require.NoError(t, err)
	})
}
