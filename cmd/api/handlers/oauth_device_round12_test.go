package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthDeviceCodeLiftRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("disabled returns access_denied", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/code", nil, nil, []byte("client_id=client-1"))
		resp := requireStatus(t, http.StatusForbidden)(h.HandleOAuthDeviceCodeLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "access_denied", body.Error)
	})

	t.Run("create failure returns server_error", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		state := &round10QueryState{
			createErrorOnce: errors.New("boom"),
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/code", nil, nil, []byte("client_id=client-1"))
		resp := requireStatus(t, http.StatusInternalServerError)(h.HandleOAuthDeviceCodeLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "server_error", body.Error)
	})

	t.Run("success returns device and user codes", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		h, _, _ := round11NewHandler(t, cfgDevice, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/code", nil, nil, []byte("client_id=client-1&scope=read%20write"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceCodeLift(ctx))

		var body apimodels.OAuthDeviceCodeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.DeviceCode)
		require.NotEmpty(t, body.UserCode)
		require.NotEmpty(t, body.VerificationURI)
		require.NotEmpty(t, body.VerificationURIComplete)
		require.Equal(t, oauthDeviceCodeTTLSeconds, body.ExpiresIn)
		require.Equal(t, oauthDevicePollIntervalSeconds, body.Interval)
	})
}
