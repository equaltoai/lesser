package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthDeviceVerifyLiftRound13_MoreBranches(t *testing.T) {
	t.Run("storage not initialized returns 503", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = true
		h := &Handler{cfg: cfg}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/verify", nil, nil, []byte("user_code=ABCD-EFGH"))
		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthDeviceVerifyLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "server_error", body.Error)
	})

	t.Run("empty body returns invalid_request", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/verify", nil, nil, nil)
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceVerifyLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body.Error)
	})

	t.Run("expired session returns expired_token", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = true

		now := time.Now().UTC()
		state := &round10QueryState{
			oauthDeviceSessionsByUserCode: map[string]storagemodels.OAuthDeviceSession{
				"ABCD-EFGH": {
					UserCode:        "ABCD-EFGH",
					ClientID:        "client-1",
					Scopes:          []string{auth.ScopeRead},
					Status:          "pending",
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					ExpiresAt:       now.Add(-1 * time.Minute),
					CreatedAt:       now.Add(-2 * time.Minute),
					UpdatedAt:       now.Add(-2 * time.Minute),
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/verify", nil, nil, []byte("user_code=ABCD-EFGH"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceVerifyLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "expired_token", body.Error)
	})

	t.Run("parse error returns invalid_request", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/verify", nil, nil, []byte("user_code=%zz"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceVerifyLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body.Error)
	})
}

func TestOAuthDeviceConsentLiftRound13_InvalidAction(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AllowDeviceFlow = true

	now := time.Now().UTC()
	state := &round10QueryState{
		oauthDeviceSessionsByUserCode: map[string]storagemodels.OAuthDeviceSession{
			"ABCD-EFGH": {
				DeviceCodeHash:  "hash",
				UserCode:        "ABCD-EFGH",
				ClientID:        "client-1",
				Scopes:          []string{auth.ScopeRead},
				Status:          "pending",
				IntervalSeconds: oauthDevicePollIntervalSeconds,
				CreatedAt:       now.Add(-1 * time.Minute),
				UpdatedAt:       now.Add(-1 * time.Minute),
				ExpiresAt:       now.Add(5 * time.Minute),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	oauthSvc := mustCreateOAuthService(t, cfg.JWTSecret, cfg, h.repos, h.logger)
	accessToken, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "client-1", "", []string{auth.ScopeRead})
	require.NoError(t, err)

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
		"authorization": "Bearer " + accessToken,
	}, nil, []byte("user_code=ABCD-EFGH&action=wat&client_id=client-1&scope=read"))
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceConsentLift(ctx))

	var body apimodels.OAuthErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_request", body.Error)
}

func TestOAuthDeviceConsentLiftRound13_UpdateErrorReturns500(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AllowDeviceFlow = true

	now := time.Now().UTC()
	state := &round10QueryState{
		updateErrorOnce: errors.New("update failed"),
		oauthDeviceSessionsByUserCode: map[string]storagemodels.OAuthDeviceSession{
			"ABCD-EFGH": {
				DeviceCodeHash:  "hash",
				UserCode:        "ABCD-EFGH",
				ClientID:        "client-1",
				Scopes:          []string{auth.ScopeRead},
				Status:          "pending",
				IntervalSeconds: oauthDevicePollIntervalSeconds,
				CreatedAt:       now.Add(-1 * time.Minute),
				UpdatedAt:       now.Add(-1 * time.Minute),
				ExpiresAt:       now.Add(5 * time.Minute),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	oauthSvc := mustCreateOAuthService(t, cfg.JWTSecret, cfg, h.repos, h.logger)
	accessToken, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "client-1", "", []string{auth.ScopeRead})
	require.NoError(t, err)

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
		"authorization": "Bearer " + accessToken,
	}, nil, []byte("user_code=ABCD-EFGH&action=approve&client_id=client-1&scope=read"))
	resp := requireStatus(t, http.StatusInternalServerError)(h.HandleOAuthDeviceConsentLift(ctx))

	var body apimodels.OAuthErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "server_error", body.Error)
}

func TestOAuthDeviceUserCodeHelpersRound13_EmptyValidation(t *testing.T) {
	require.False(t, validateOAuthDeviceUserCode("   "))
}
