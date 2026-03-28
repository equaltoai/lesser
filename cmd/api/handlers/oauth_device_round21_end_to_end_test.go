package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthDeviceFlowConnectorAgentRejectedRound21(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AllowDeviceFlow = true

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
	deviceResp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDeviceCodeLift(deviceCtx))

	var body apimodels.OAuthErrorResponse
	require.NoError(t, json.Unmarshal(deviceResp.Body, &body))
	require.Equal(t, "unauthorized_client", body.Error)
	require.Equal(t, "device_code is not allowed for this client", body.ErrorDescription)
	require.Empty(t, state.oauthDeviceSessionsByHash)
}
