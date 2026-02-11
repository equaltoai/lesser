package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
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

	t.Run("access_token hint is a no-op", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=at-1&token_type_hint=access_token"))
		_ = requireStatus(t, http.StatusOK)(h.HandleOAuthRevokeLift(ctx))
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
}
