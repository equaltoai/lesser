package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthDeviceFlowAgentEndToEndRound21(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AllowDeviceFlow = true
	cfg.AgentAccessTokenDuration = 8 * time.Hour

	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"client-agent": {
				ClientID:      "client-agent",
				Name:          "Agent Device Connector",
				RedirectURIs:  []string{"https://example.com/callback"},
				GrantTypes:    []string{oauthDeviceCodeGrantType, auth.GrantTypeRefreshToken},
				ClientClass:   auth.ClientClassAgent,
				AgentUsername: "agent1",
				CreatedAt:     time.Now().Add(-24 * time.Hour),
			},
		},
		usersByUsername: map[string]storagemodels.User{
			"agent1": {
				Username:   "agent1",
				IsAgent:    true,
				AgentType:  "assistant",
				AgentOwner: "@owner",
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	deviceCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/code", nil, nil, []byte("client_id=client-agent&scope=read%20follow"))
	deviceResp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceCodeLift(deviceCtx))

	var deviceBody apimodels.OAuthDeviceCodeResponse
	require.NoError(t, json.Unmarshal(deviceResp.Body, &deviceBody))
	require.NotEmpty(t, deviceBody.DeviceCode)
	require.NotEmpty(t, deviceBody.UserCode)
	require.Equal(t, "https://example.com/auth/device", deviceBody.VerificationURI)
	require.Equal(t, "https://example.com/auth/device?user_code="+url.QueryEscape(deviceBody.UserCode), deviceBody.VerificationURIComplete)

	deviceHash := oauthDeviceCodeHash(deviceBody.DeviceCode)
	storedSession, ok := state.oauthDeviceSessionsByHash[deviceHash]
	require.True(t, ok)
	require.Equal(t, []string{auth.ScopeRead, auth.ScopeFollow}, storedSession.Scopes)
	require.Equal(t, oauthDeviceSessionStatusPending, storedSession.Status)

	pageCtx, err := round10NewLiftContext(http.MethodGet, "/auth/device", nil, map[string]string{"user_code": deviceBody.UserCode}, nil)
	require.NoError(t, err)
	pageResp := requireStatus(t, http.StatusOK)(h.HandleOAuthDevicePageLift(pageCtx))
	require.Contains(t, string(pageResp.Body), deviceBody.UserCode)

	verifyCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/verify", nil, nil, []byte("user_code="+url.QueryEscape(deviceBody.UserCode)))
	verifyResp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceVerifyLift(verifyCtx))

	var verifyBody apimodels.OAuthDeviceVerifyResponse
	require.NoError(t, json.Unmarshal(verifyResp.Body, &verifyBody))
	require.Equal(t, "client-agent", verifyBody.ClientID)
	require.Equal(t, "Agent Device Connector", verifyBody.ClientName)
	require.Equal(t, []string{auth.ScopeRead, auth.ScopeFollow}, verifyBody.Scopes)
	require.Equal(t, oauthDeviceSessionStatusPending, verifyBody.Status)

	oauthSvc := createOAuthService(cfg.JWTSecret, cfg, h.repos, h.logger)
	operatorAccessToken, _, err := oauthSvc.GenerateTokens(context.Background(), "owner", "device-approver", "", []string{auth.ScopeRead})
	require.NoError(t, err)

	consentCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/device/consent", map[string]string{
		"authorization": "Bearer " + operatorAccessToken,
	}, nil, []byte("user_code="+url.QueryEscape(deviceBody.UserCode)+"&action=approve"))
	consentResp := requireStatus(t, http.StatusOK)(h.HandleOAuthDeviceConsentLift(consentCtx))

	var consentBody apimodels.OAuthDeviceConsentResponse
	require.NoError(t, json.Unmarshal(consentResp.Body, &consentBody))
	require.Equal(t, oauthDeviceSessionStatusApproved, consentBody.Status)

	approvedSession := state.oauthDeviceSessionsByHash[deviceHash]
	require.Equal(t, oauthDeviceSessionStatusApproved, approvedSession.Status)
	require.Equal(t, "owner", approvedSession.ApprovedUsername)

	tokenCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+url.QueryEscape(deviceBody.DeviceCode)+"&client_id=client-agent"))
	tokenResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(tokenCtx))

	var tokenBody apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(tokenResp.Body, &tokenBody))
	require.Equal(t, "read follow", tokenBody.Scope)
	require.Equal(t, int((8 * time.Hour).Seconds()), tokenBody.ExpiresIn)
	require.NotEmpty(t, tokenBody.RefreshToken)

	claims := round12DecodeJWTClaims(t, tokenBody.AccessToken)
	require.Equal(t, "agent1", claims.Username)
	require.True(t, claims.IsAgent)
	require.Equal(t, auth.ClientClassAgent, claims.ClientClass)
	require.Equal(t, "assistant", claims.AgentType)
	require.Equal(t, "@owner", claims.DelegatedBy)
	require.ElementsMatch(t, []string{auth.ScopeRead, auth.ScopeFollow}, claims.Scopes)

	readCtx := round10NewLiftContextWithBodyBytes(http.MethodGet, "/api/v1/accounts/verify_credentials", map[string]string{
		"authorization": "Bearer " + tokenBody.AccessToken,
	}, nil, nil)
	readClaims, err := h.authenticateWithScope(readCtx, auth.ScopeRead)
	require.NoError(t, err)
	require.Equal(t, "agent1", readClaims.Username)

	followCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/accounts/agent1/follow", map[string]string{
		"authorization": "Bearer " + tokenBody.AccessToken,
	}, nil, nil)
	followClaims, err := h.authenticateRelationshipOperation(followCtx, relationshipOpFollow)
	require.NoError(t, err)
	require.Equal(t, "agent1", followClaims.Username)

	finalSession := state.oauthDeviceSessionsByHash[deviceHash]
	require.Equal(t, oauthDeviceSessionStatusConsumed, finalSession.Status)
	require.Equal(t, []string{auth.ScopeRead, auth.ScopeFollow}, finalSession.Scopes)

	storedRefresh, ok := state.refreshTokensByToken[tokenBody.RefreshToken]
	require.True(t, ok)
	require.Equal(t, auth.ClientClassAgent, storedRefresh.ClientClass)
	require.Equal(t, "agent1", storedRefresh.Username)
	require.Equal(t, []string{auth.ScopeRead, auth.ScopeFollow}, storedRefresh.Scopes)
	require.NotEmpty(t, storedRefresh.FamilyID)
	require.True(t, storedRefresh.Current)
	require.Equal(t, int((8 * time.Hour).Seconds()), storedRefresh.AccessTTLSeconds)
}
