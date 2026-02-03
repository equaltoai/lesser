package handlers

import (
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

func TestAgentsRound17_HandleGetAgentLift_Branches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	state := &round10QueryState{
		agentInstanceConfig: policy,
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:        "USER#agent",
				SK:        storagemodels.SKMetadata,
				Username:  "agent",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: time.Now().Add(-24 * time.Hour),
				IsAgent:   true,
			},
			"suspended": {
				PK:        "USER#suspended",
				SK:        storagemodels.SKMetadata,
				Username:  "suspended",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: time.Now().Add(-24 * time.Hour),
				IsAgent:   true,
				Suspended: true,
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	t.Run("missing username returns 400", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleGetAgentLift(ctx))
	})

	t.Run("non-agent user returns 404", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/missing", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleGetAgentLift(ctx))
	})

	t.Run("suspended agents return 404", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/suspended", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "suspended"
		requireStatus(t, http.StatusNotFound)(h.HandleGetAgentLift(ctx))
	})

	t.Run("agents return 200", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"
		requireStatus(t, http.StatusOK)(h.HandleGetAgentLift(ctx))
	})
}

func TestAgentsRound17_HandleListAgentsLift_Branches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	t.Run("repository errors return 500", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policy,
			allErrorOnce:        errors.New("boom"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleListAgentsLift(ctx))
	})

	t.Run("filters suspended agents", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "user", Approved: true, Version: 1, IsAgent: true},
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, IsAgent: true, Suspended: true},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleListAgentsLift(ctx))
		require.NotEmpty(t, resp.Body)
	})

	t.Run("filters non-agent users", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "user", Approved: true, Version: 1, IsAgent: false},
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, IsAgent: true},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleListAgentsLift(ctx))
		require.NotEmpty(t, resp.Body)
	})
}

func TestAgentsRound17_HandleSuspendAgentLift_Branches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken}

	t.Run("missing token returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/suspend", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"
		requireStatus(t, http.StatusUnauthorized)(h.HandleSuspendAgentLift(ctx))
	})

	t.Run("invalid username returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/not a username/suspend", adminHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "not a username"
		requireStatus(t, http.StatusBadRequest)(h.HandleSuspendAgentLift(ctx))
	})

	t.Run("missing agent returns 404", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/missing/suspend", adminHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleSuspendAgentLift(ctx))
	})

	t.Run("update errors return 500", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"agent": {PK: "USER#agent", SK: storagemodels.SKMetadata, Username: "agent", Role: "user", Approved: true, Version: 1, IsAgent: true},
			},
			executeErrorOnce: errors.New("boom"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/suspend", adminHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"
		requireStatus(t, http.StatusInternalServerError)(h.HandleSuspendAgentLift(ctx))
	})

	t.Run("success suspends agent", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"agent": {PK: "USER#agent", SK: storagemodels.SKMetadata, Username: "agent", Role: "user", Approved: true, Version: 1, IsAgent: true},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/suspend", adminHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"
		requireStatus(t, http.StatusOK)(h.HandleSuspendAgentLift(ctx))
	})
}

func TestAgentsRound17_HandleGetAgentActivityLift_Branches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	now := time.Now().UTC()
	logs := []*storagemodels.AuthAuditLog{
		nil,
		{Timestamp: now.Add(-1 * time.Hour), EventType: "unrelated.event"},
		{Timestamp: now.Add(-2 * time.Hour), EventType: "agent.status.create", Metadata: `{"target_id":"s1"}`},
		{Timestamp: now.Add(-3 * time.Hour), EventType: "agent.key_rotated", Metadata: `{bad json`},
	}

	state := &round10QueryState{
		agentInstanceConfig: policy,
		usersByUsername: map[string]storagemodels.User{
			"owner": {PK: "USER#owner", SK: storagemodels.SKMetadata, Username: "owner", Role: "user", Approved: true, Version: 1},
			"agent": {PK: "USER#agent", SK: storagemodels.SKMetadata, Username: "agent", Role: "user", Approved: true, Version: 1, IsAgent: true, AgentOwner: "@owner"},
		},
		auditLogsByUser: map[string][]*storagemodels.AuthAuditLog{
			"agent": logs,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	t.Run("missing token returns 401", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent/activity", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"
		requireStatus(t, http.StatusUnauthorized)(h.HandleGetAgentActivityLift(ctx))
	})

	t.Run("insufficient scope returns 403", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent/activity", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"
		requireStatus(t, http.StatusForbidden)(h.HandleGetAgentActivityLift(ctx))
	})

	t.Run("non-owner viewers are forbidden", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "bob", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent/activity", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"
		requireStatus(t, http.StatusForbidden)(h.HandleGetAgentActivityLift(ctx))
	})

	t.Run("owner can view activity and payload is stable", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent/activity", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAgentActivityLift(ctx))
		var out apimodels.AgentActivityLogList
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.NotEmpty(t, out)
	})
}
