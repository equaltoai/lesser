package handlers

import (
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

// Tests for CSR-024: OAuth device consent scope escalation across clients.
//
// These tests prove that:
//  1. Scope escalation at consent time is rejected — a malicious form POST
//     cannot add scopes beyond what the device authorization session holds.
//  2. Cross-client token exchange is rejected — client B cannot exchange a
//     device_code that belongs to client A.
//  3. Edge cases in consent binding (missing scope, duplicate keys, empty
//     scope when session has scopes) are correctly rejected.

func TestOAuthDeviceConsent_CSR024_ScopeBindingEdgeCases(t *testing.T) {
	cfgDevice := round11TestConfig()
	cfgDevice.AllowDeviceFlow = true
	now := time.Now().UTC()

	state := func() *round10QueryState {
		return &round10QueryState{
			oauthDeviceSessionsByUserCode: map[string]storagemodels.OAuthDeviceSession{
				"ABCD-EFGH": {
					DeviceCodeHash:  "hash-csr024",
					UserCode:        "ABCD-EFGH",
					ClientID:        "client-csr024",
					Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
					Status:          "pending",
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					CreatedAt:       now.Add(-1 * time.Minute),
					UpdatedAt:       now.Add(-1 * time.Minute),
					ExpiresAt:       now.Add(5 * time.Minute),
				},
			},
		}
	}

	h, _, _ := round11NewHandler(t, cfgDevice, state())
	oauthSvc := mustCreateOAuthService(t, cfgDevice.JWTSecret, cfgDevice, h.repos, h.logger)
	accessToken, _, err := oauthSvc.GenerateTokens(t.Context(), "alice", "client-csr024", "", []string{auth.ScopeRead, auth.ScopeWrite})
	require.NoError(t, err)

	authHeaders := map[string]string{"authorization": "Bearer " + accessToken}

	t.Run("missing scope when session has scopes is rejected", func(t *testing.T) {
		// The session requires "read write" but the consent form omits the scope
		// parameter entirely. This must be rejected.
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", authHeaders, nil,
			[]byte("user_code=ABCD-EFGH&action=approve&client_id=client-csr024"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body.Error)
		require.Equal(t, "scope is required", body.ErrorDescription)
	})

	t.Run("scope with extra space-delimited padding is accepted when equivalent", func(t *testing.T) {
		s := state()
		h2, _, _ := round11NewHandler(t, cfgDevice, s)

		// Session has ["read", "write"]; consent sends " read  write " with
		// extra spaces. splitOAuthSpaceDelimited normalizes this.
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", authHeaders, nil,
			[]byte("user_code=ABCD-EFGH&action=approve&client_id=client-csr024&scope=+read++write+"))
		resp := requireStatus(t, http.StatusOK)(h2.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthDeviceConsentResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "approved", strings.ToLower(body.Status))
	})

	t.Run("scope with different key name 'scopes' is also validated", func(t *testing.T) {
		s := state()
		h2, _, _ := round11NewHandler(t, cfgDevice, s)

		// Consent form uses "scopes" (plural). oauthDeviceConsentScopeParam
		// accepts both "scope" and "scopes", but the value must still match.
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", authHeaders, nil,
			[]byte("user_code=ABCD-EFGH&action=approve&client_id=client-csr024&scopes=read+write+follow"))
		resp := requireStatus(t, http.StatusBadRequest)(h2.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_scope", body.Error)
		require.Equal(t, "scope does not match device authorization", body.ErrorDescription)
	})

	t.Run("empty scope value when session has scopes is rejected", func(t *testing.T) {
		s := state()
		h2, _, _ := round11NewHandler(t, cfgDevice, s)

		// Empty scope value is not equal to the session's scopes.
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", authHeaders, nil,
			[]byte("user_code=ABCD-EFGH&action=approve&client_id=client-csr024&scope="))
		resp := requireStatus(t, http.StatusBadRequest)(h2.HandleOAuthDeviceConsentLift(ctx))

		var body apimodels.OAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_scope", body.Error)
	})
}

func TestOAuthDeviceToken_CSR024_CrossClientExchangeRejected(t *testing.T) {
	cfgDevice := round11TestConfig()
	cfgDevice.AllowDeviceFlow = true

	deviceCode := "cross-client-dc"
	deviceHash := oauthDeviceCodeHash(deviceCode)

	makeApprovedSession := func() storagemodels.OAuthDeviceSession {
		return storagemodels.OAuthDeviceSession{
			DeviceCodeHash:   deviceHash,
			UserCode:         "ABCD-EFGH",
			ClientID:         "client-a",
			Scopes:           []string{auth.ScopeRead},
			Status:           "approved",
			ApprovedUsername: "alice",
			IntervalSeconds:  oauthDevicePollIntervalSeconds,
			CreatedAt:        time.Now().Add(-2 * time.Minute),
			UpdatedAt:        time.Now().Add(-1 * time.Minute),
			ExpiresAt:        time.Now().Add(5 * time.Minute),
		}
	}

	baseForm := "grant_type=" + oauthDeviceCodeGrantType + "&device_code=" + deviceCode

	t.Run("client B cannot exchange client A device_code", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-a": {
					ClientID:    "client-a",
					ClientClass: auth.ClientClassCLI,
					GrantTypes:  []string{oauthDeviceCodeGrantType, auth.GrantTypeRefreshToken},
					CreatedAt:   time.Now().Add(-24 * time.Hour),
				},
				"client-b": {
					ClientID:    "client-b",
					ClientClass: auth.ClientClassCLI,
					GrantTypes:  []string{oauthDeviceCodeGrantType, auth.GrantTypeRefreshToken},
					CreatedAt:   time.Now().Add(-24 * time.Hour),
				},
			},
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: makeApprovedSession(),
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil,
			[]byte(baseForm+"&client_id=client-b"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
		require.Equal(t, "Invalid device_code", body["error_description"])
	})

	t.Run("client A can exchange its own approved session", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-a": {
					ClientID:    "client-a",
					ClientClass: auth.ClientClassCLI,
					GrantTypes:  []string{oauthDeviceCodeGrantType, auth.GrantTypeRefreshToken},
					CreatedAt:   time.Now().Add(-24 * time.Hour),
				},
			},
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: makeApprovedSession(),
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil,
			[]byte(baseForm+"&client_id=client-a"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.Equal(t, auth.ScopeRead, strings.TrimSpace(body.Scope))
	})
}
