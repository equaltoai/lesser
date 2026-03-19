package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

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
		require.True(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{ClientClass: auth.ClientClassAgent}, auth.GrantTypeClientCredentials))
		require.False(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{ClientClass: auth.ClientClassWeb}, auth.GrantTypeClientCredentials))
		require.True(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{GrantTypes: []string{auth.GrantTypeRefreshToken}}, auth.GrantTypeRefreshToken))
		require.False(t, oauthClientSupportsGrantType(&storagepkg.OAuthClient{GrantTypes: []string{auth.GrantTypeRefreshToken}}, auth.GrantTypeAuthorizationCode))
	})
}
