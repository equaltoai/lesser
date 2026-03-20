package handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleOAuthDevicePageLift(t *testing.T) {
	t.Run("renders hosted device approval page with prefilled code", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = true
		h, _, _ := round11NewHandler(t, cfg, nil)

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/device?user_code=abcd-efgh", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthDevicePageLift(ctx))
		require.Equal(t, "text/html; charset=utf-8", firstStringValue(resp.Headers, "content-type"))
		require.Equal(t, "no-store", firstStringValue(resp.Headers, "cache-control"))

		body := string(resp.Body)
		require.Contains(t, body, "Approve a device login")
		require.Contains(t, body, "value=\"ABCD-EFGH\"")
		require.Contains(t, body, "/oauth/device/verify")
		require.Contains(t, body, "/oauth/device/consent")
		require.Contains(t, body, "Operator access token")
		require.Contains(t, body, "Approve")
		require.Contains(t, body, "Deny")
		require.Contains(t, body, "nonce=")
	})

	t.Run("escapes unexpected query content", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = true
		h, _, _ := round11NewHandler(t, cfg, nil)

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/device?user_code=%3Cscript%3E", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthDevicePageLift(ctx))
		body := string(resp.Body)
		require.NotContains(t, body, "value=\"<script>\"")
		require.Contains(t, body, "value=\"")
	})

	t.Run("disabled device flow returns access denied", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AllowDeviceFlow = false
		h, _, _ := round11NewHandler(t, cfg, nil)

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/device", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusForbidden)(h.HandleOAuthDevicePageLift(ctx))
		require.Contains(t, string(resp.Body), "device authorization is disabled")
	})
}
