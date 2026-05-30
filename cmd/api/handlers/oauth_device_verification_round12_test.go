package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthDeviceVerifyLiftRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("disabled returns access_denied", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/verify", nil, nil, []byte("user_code=ABCD-EFGH"))
		resp := requireStatus(t, http.StatusForbidden)(h.HandleOAuthDeviceVerifyLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "access_denied", body.Error)
	})

	t.Run("invalid user_code returns invalid_request", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		h, _, _ := round11NewHandler(t, cfgDevice, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/verify", nil, nil, []byte("user_code=not-a-code"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceVerifyLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body.Error)
	})

	t.Run("success returns device session metadata", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true

		now := time.Now().UTC()
		state := &round10QueryState{
			oauthDeviceSessionsByUserCode: map[string]storagemodels.OAuthDeviceSession{
				"ABCD-EFGH": {
					DeviceCodeHash:  "hash",
					UserCode:        "ABCD-EFGH",
					ClientID:        "client-1",
					Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
					Status:          "pending",
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					CreatedAt:       now.Add(-1 * time.Minute),
					UpdatedAt:       now.Add(-1 * time.Minute),
					ExpiresAt:       now.Add(5 * time.Minute),
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/verify", nil, nil, []byte("user_code=abcd-efgh"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceVerifyLift(ctx))

		var body apimodels.OAuthDeviceVerifyResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "ABCD-EFGH", body.UserCode)
		require.Equal(t, "client-1", body.ClientID)
		require.NotEmpty(t, body.ClientName)
		require.Contains(t, body.Scopes, auth.ScopeRead)
		require.Equal(t, "pending", body.Status)
		require.Greater(t, body.ExpiresIn, 0)
		require.Equal(t, oauthDevicePollIntervalSeconds, body.Interval)
	})
}

func TestOAuthDeviceConsentLiftRound12(t *testing.T) {
	cfgDevice := round11TestConfig()
	cfgDevice.AllowDeviceFlow = true

	now := time.Now().UTC()
	state := &round10QueryState{
		oauthDeviceSessionsByUserCode: map[string]storagemodels.OAuthDeviceSession{
			"ABCD-EFGH": {
				DeviceCodeHash:  "hash",
				UserCode:        "ABCD-EFGH",
				ClientID:        "client-1",
				Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
				Status:          "pending",
				IntervalSeconds: oauthDevicePollIntervalSeconds,
				CreatedAt:       now.Add(-1 * time.Minute),
				UpdatedAt:       now.Add(-1 * time.Minute),
				ExpiresAt:       now.Add(5 * time.Minute),
			},
			"JKLM-NPQR": {
				DeviceCodeHash:  "hash-deny",
				UserCode:        "JKLM-NPQR",
				ClientID:        "client-1",
				Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
				Status:          "pending",
				IntervalSeconds: oauthDevicePollIntervalSeconds,
				CreatedAt:       now.Add(-1 * time.Minute),
				UpdatedAt:       now.Add(-1 * time.Minute),
				ExpiresAt:       now.Add(5 * time.Minute),
			},
			"STUV-WXYZ": {
				DeviceCodeHash:  "hash-mismatch",
				UserCode:        "STUV-WXYZ",
				ClientID:        "client-1",
				Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
				Status:          "pending",
				IntervalSeconds: oauthDevicePollIntervalSeconds,
				CreatedAt:       now.Add(-1 * time.Minute),
				UpdatedAt:       now.Add(-1 * time.Minute),
				ExpiresAt:       now.Add(5 * time.Minute),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfgDevice, state)

	oauthSvc := mustCreateOAuthService(t, cfgDevice.JWTSecret, cfgDevice, h.repos, h.logger)
	accessToken, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "client-1", "", []string{auth.ScopeRead, auth.ScopeWrite})
	require.NoError(t, err)

	t.Run("missing auth returns 401", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", nil, nil, []byte("user_code=ABCD-EFGH&action=approve&client_id=client-1&scope=read+write"))
		requireStatus(t, http.StatusUnauthorized)(h.HandleOAuthDeviceConsentLift(ctx))
	})

	t.Run("approve updates status", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
			"authorization": "Bearer " + accessToken,
		}, nil, []byte("user_code=ABCD-EFGH&action=approve&client_id=client-1&scope=write+read"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthDeviceConsentResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "approved", strings.ToLower(body.Status))
	})

	t.Run("deny updates status", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
			"authorization": "Bearer " + accessToken,
		}, nil, []byte("user_code=JKLM-NPQR&action=deny&client_id=client-1&scope=read+write"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthDeviceConsentResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "denied", strings.ToLower(body.Status))
	})

	t.Run("mismatched client is rejected before consent update", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
			"authorization": "Bearer " + accessToken,
		}, nil, []byte("user_code=STUV-WXYZ&action=approve&client_id=other-client&scope=read+write"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body.Error)
		require.Equal(t, "pending", strings.ToLower(state.oauthDeviceSessionsByUserCode["STUV-WXYZ"].Status))
	})

	t.Run("scope escalation is rejected before consent update", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
			"authorization": "Bearer " + accessToken,
		}, nil, []byte("user_code=STUV-WXYZ&action=approve&client_id=client-1&scope=read+write+admin"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_scope", body.Error)
		require.Equal(t, "pending", strings.ToLower(state.oauthDeviceSessionsByUserCode["STUV-WXYZ"].Status))
	})
}

func TestOAuthDeviceConsentLiftRound12_L01ScopeSubsetEnforced(t *testing.T) {
	cfgDevice := round11TestConfig()
	cfgDevice.AllowDeviceFlow = true
	now := time.Now().UTC()

	newHandler := func(t *testing.T, userCode string, sessionScopes []string) (*Handler, *round10QueryState) {
		t.Helper()
		state := &round10QueryState{
			oauthDeviceSessionsByUserCode: map[string]storagemodels.OAuthDeviceSession{
				userCode: {
					DeviceCodeHash:  "hash-" + userCode,
					UserCode:        userCode,
					ClientID:        "device-client",
					Scopes:          sessionScopes,
					Status:          oauthDeviceSessionStatusPending,
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					CreatedAt:       now.Add(-1 * time.Minute),
					UpdatedAt:       now.Add(-1 * time.Minute),
					ExpiresAt:       now.Add(5 * time.Minute),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		return h, state
	}

	accessToken := func(t *testing.T, h *Handler, scopes []string) string {
		t.Helper()
		// The approving token is intentionally minted for a different OAuth client to
		// keep hosted UI consent working while the scope subset gate carries the
		// privilege boundary.
		oauthSvc := mustCreateOAuthService(t, cfgDevice.JWTSecret, cfgDevice, h.repos, h.logger)
		token, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "hosted-ui-client", "", scopes)
		require.NoError(t, err)
		return token
	}

	consentBody := func(userCode, scope string) []byte {
		return []byte("user_code=" + userCode + "&action=approve&client_id=device-client&scope=" + scope)
	}

	t.Run("read token cannot approve write admin device session", func(t *testing.T) {
		userCode := "ABCD-EFGH"
		h, state := newHandler(t, userCode, []string{auth.ScopeRead, auth.ScopeWrite, auth.ScopeAdmin})
		token := accessToken(t, h, []string{auth.ScopeRead})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
			"authorization": "Bearer " + token,
		}, nil, consentBody(userCode, "read+write+admin"))
		resp := requireStatus(t, http.StatusForbidden)(h.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "insufficient_scope", body.Error)
		require.Contains(t, body.ErrorDescription, auth.ScopeWrite)
		session := state.oauthDeviceSessionsByUserCode[userCode]
		require.Equal(t, oauthDeviceSessionStatusPending, strings.ToLower(session.Status))
		require.Empty(t, session.ApprovedUsername)
	})

	t.Run("equal scopes approve", func(t *testing.T) {
		userCode := "JKLM-NPQR"
		h, state := newHandler(t, userCode, []string{auth.ScopeRead, auth.ScopeWrite})
		token := accessToken(t, h, []string{auth.ScopeRead, auth.ScopeWrite})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
			"authorization": "Bearer " + token,
		}, nil, consentBody(userCode, "write+read"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthDeviceConsentResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, oauthDeviceSessionStatusApproved, strings.ToLower(body.Status))
		session := state.oauthDeviceSessionsByUserCode[userCode]
		require.Equal(t, oauthDeviceSessionStatusApproved, strings.ToLower(session.Status))
		require.Equal(t, "alice", session.ApprovedUsername)
	})

	t.Run("superset scopes approve", func(t *testing.T) {
		userCode := "PQRS-TUVW"
		h, state := newHandler(t, userCode, []string{auth.ScopeRead, auth.ScopeWrite})
		token := accessToken(t, h, []string{auth.ScopeRead, auth.ScopeWrite, auth.ScopeAdmin})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
			"authorization": "Bearer " + token,
		}, nil, consentBody(userCode, "read+write"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthDeviceConsentResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, oauthDeviceSessionStatusApproved, strings.ToLower(body.Status))
		session := state.oauthDeviceSessionsByUserCode[userCode]
		require.Equal(t, oauthDeviceSessionStatusApproved, strings.ToLower(session.Status))
		require.Equal(t, "alice", session.ApprovedUsername)
	})
}
