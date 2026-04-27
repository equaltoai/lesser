package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthRevokeLift_Round22(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("empty request body", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, nil)
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthRevokeLift(ctx))

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("invalid form data is invalid_request", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=%"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthRevokeLift(ctx))

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("missing token is invalid_request", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthRevokeLift(ctx))

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("invalid basic auth is invalid_client", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost,
			"/oauth/revoke",
			map[string]string{"Authorization": "Basic not-base64"},
			nil,
			[]byte("token=rt-1&token_type_hint=refresh_token"),
		)
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthRevokeLift(ctx))

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
	})

	t.Run("access_token hint is a no-op", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=at-1&token_type_hint=access_token"))
		_ = requireStatus(t, http.StatusOK)(h.HandleOAuthRevokeLift(ctx))
	})

	t.Run("access_token hint revokes and invalidates token", func(t *testing.T) {
		state := &round10QueryState{}
		h, repos, _ := round11NewHandler(t, cfg, state)

		oauthSvc := createOAuthService(cfg.JWTSecret, cfg, repos, h.logger)
		access, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "client-1", "192.0.2.10", []string{auth.ScopeRead})
		require.NoError(t, err)

		claims, err := oauthSvc.ValidateAccessToken(access)
		require.NoError(t, err)
		require.NotEmpty(t, claims.ID)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token="+url.QueryEscape(access)+"&token_type_hint=access_token"))
		_ = requireStatus(t, http.StatusOK)(h.HandleOAuthRevokeLift(ctx))

		record, ok := state.revokedAccessTokensByJTI[claims.ID]
		require.True(t, ok)
		require.Equal(t, "REVOKEDTOKEN#"+claims.ID, record.PK)
		require.Equal(t, storagemodels.SKToken, record.SK)
		require.Equal(t, claims.ID, record.JTI)
		require.False(t, record.ExpiresAt.IsZero())
		require.False(t, record.RevokedAt.IsZero())
		require.NotZero(t, record.TTL)

		_, err = oauthSvc.ValidateAccessToken(access)
		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("refresh_token returns 200 even when delete fails", func(t *testing.T) {
		state := &round10QueryState{
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-1": {
					Token:       "rt-1",
					ClientID:    "client-1",
					Username:    "alice",
					Scopes:      []string{auth.ScopeRead, auth.ScopeWrite},
					ClientClass: auth.ClientClassCLI,
					ExpiresAt:   time.Now().Add(1 * time.Hour),
				},
			},
			deleteErrorOnce: errors.New("delete failed"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=rt-1&token_type_hint=refresh_token&client_id=client-1"))
		_ = requireStatus(t, http.StatusOK)(h.HandleOAuthRevokeLift(ctx))
	})

	t.Run("refresh_token without client_id does not revoke known token", func(t *testing.T) {
		state := &round10QueryState{
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-no-client": {
					Token:       "rt-no-client",
					ClientID:    "client-1",
					Username:    "alice",
					Scopes:      []string{auth.ScopeRead},
					ClientClass: auth.ClientClassCLI,
					ExpiresAt:   time.Now().Add(1 * time.Hour),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=rt-no-client&token_type_hint=refresh_token"))
		_ = requireStatus(t, http.StatusOK)(h.HandleOAuthRevokeLift(ctx))

		require.Contains(t, state.refreshTokensByToken, "rt-no-client")
	})

	t.Run("confidential refresh_token requires client_secret", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-conf": {
					ClientID:     "client-conf",
					ClientSecret: "secret",
					Confidential: true,
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-conf": {
					Token:       "rt-conf",
					ClientID:    "client-conf",
					Username:    "alice",
					Scopes:      []string{auth.ScopeRead},
					ClientClass: auth.ClientClassWeb,
					ExpiresAt:   time.Now().Add(1 * time.Hour),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=rt-conf&token_type_hint=refresh_token&client_id=client-conf"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthRevokeLift(ctx))

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
		require.Contains(t, state.refreshTokensByToken, "rt-conf")
	})

	t.Run("confidential refresh_token rejects wrong client_secret", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-conf": {
					ClientID:     "client-conf",
					ClientSecret: "secret",
					Confidential: true,
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-conf-wrong": {
					Token:       "rt-conf-wrong",
					ClientID:    "client-conf",
					Username:    "alice",
					Scopes:      []string{auth.ScopeRead},
					ClientClass: auth.ClientClassWeb,
					ExpiresAt:   time.Now().Add(1 * time.Hour),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=rt-conf-wrong&token_type_hint=refresh_token&client_id=client-conf&client_secret=wrong"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthRevokeLift(ctx))

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
		require.Contains(t, state.refreshTokensByToken, "rt-conf-wrong")
	})

	t.Run("confidential refresh_token accepts basic auth over form client", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-conf": {
					ClientID:     "client-conf",
					ClientSecret: "secret",
					Confidential: true,
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-conf-basic": {
					Token:       "rt-conf-basic",
					ClientID:    "client-conf",
					Username:    "alice",
					Scopes:      []string{auth.ScopeRead},
					ClientClass: auth.ClientClassWeb,
					ExpiresAt:   time.Now().Add(1 * time.Hour),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("client-conf:secret"))
		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost,
			"/oauth/revoke",
			map[string]string{"Authorization": authHeader},
			nil,
			[]byte("token=rt-conf-basic&token_type_hint=refresh_token&client_id=other-client"),
		)
		_ = requireStatus(t, http.StatusOK)(h.HandleOAuthRevokeLift(ctx))

		require.NotContains(t, state.refreshTokensByToken, "rt-conf-basic")
	})
}
