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

func TestAgentManagementHandlersRound20(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	state := &round10QueryState{
		agentInstanceConfig: func() *storagemodels.AgentInstanceConfig {
			p := storagemodels.NewAgentInstanceConfig()
			p.AllowAgents = true
			p.AllowAgentRegistration = true
			p.AgentMaxPostsPerHour = 12
			return p
		}(),
		usersByUsername: map[string]storagemodels.User{
			"owner": {
				PK:        "USER#owner",
				SK:        storagemodels.SKMetadata,
				Username:  "owner",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
			},
			"admin": {
				PK:        "USER#admin",
				SK:        storagemodels.SKMetadata,
				Username:  "admin",
				Role:      "admin",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
			},
			"agent1": {
				PK:           "USER#agent1",
				SK:           storagemodels.SKMetadata,
				Username:     "agent1",
				DisplayName:  "Agent One",
				Approved:     true,
				Version:      1,
				CreatedAt:    now.Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
			},
		},
		agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
			"agent1": {
				PK:               "USER#agent1",
				SK:               storagemodels.SKAgentGovernance,
				Username:         "agent1",
				QuarantineStatus: "quarantined",
				QuarantineEnd:    ptrTime(now.Add(24 * time.Hour)),
				CreatedAt:        now.Add(-24 * time.Hour),
				UpdatedAt:        now.Add(-time.Hour),
			},
		},
		auditLogsByUser: map[string][]*storagemodels.AuthAuditLog{
			"agent1": {
				{EventType: "agent.status.create", Timestamp: now.Add(-time.Minute), Metadata: `{"target_id":"status-1"}`},
				{EventType: "user.login", Timestamp: now.Add(-2 * time.Minute)},
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	ownerHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead})}
	adminHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})}

	t.Run("get agent success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp := requireStatus(t, http.StatusOK)(h.HandleGetAgentLift(ctx))
		var out apimodels.Agent
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		bundle := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent1")
		require.Equal(t, "agent1", out.Username)
		require.Equal(t, bundle.MCPURL, out.MCPAccess.MCPURL)
		require.Equal(t, bundle.ProtectedResourceURL, out.MCPAccess.ProtectedResourceURL)
		require.Equal(t, bundle.AuthorizationServerURL, out.MCPAccess.AuthorizationServerURL)
		require.Equal(t, bundle.RegistrationURL, out.MCPAccess.RegistrationURL)
		require.Len(t, out.MCPAccess.Guidance, 5)
	})

	t.Run("update agent success", func(t *testing.T) {
		req := apimodels.UpdateAgentRequest{
			DisplayName: "Updated Agent",
			Bio:         "New bio",
			AgentCapabilities: &apimodels.AgentCapabilities{
				MaxPostsPerHour: 99,
			},
			ExitQuarantine: true,
		}
		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/agent1", ownerHeaders, nil, req)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp := requireStatus(t, http.StatusOK)(h.HandleUpdateAgentLift(ctx))
		var out apimodels.Agent
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "Updated Agent", out.DisplayName)
	})

	t.Run("delete agent forbidden for wrong owner", func(t *testing.T) {
		badHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "intruder", []string{auth.ScopeWrite})}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/agent1", badHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(h.HandleDeleteAgentLift(ctx))
	})

	t.Run("delete agent success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/agent1", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusOK)(h.HandleDeleteAgentLift(ctx))
	})

	t.Run("agent activity success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/activity", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp := requireStatus(t, http.StatusOK)(h.HandleGetAgentActivityLift(ctx))
		var out []apimodels.AgentActivityLogEntry
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 1)
		require.Equal(t, "agent.status.create", out[0].Action)
	})

	t.Run("suspend agent success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/suspend", adminHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusOK)(h.HandleSuspendAgentLift(ctx))
	})
}
