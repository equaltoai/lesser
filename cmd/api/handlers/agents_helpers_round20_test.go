package handlers

import (
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentHelpersRound20(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	baseState := &round10QueryState{
		agentInstanceConfig: func() *storagemodels.AgentInstanceConfig {
			p := storagemodels.NewAgentInstanceConfig()
			p.AllowAgents = true
			p.AllowAgentRegistration = true
			return p
		}(),
		usersByUsername: map[string]storagemodels.User{
			"agent1": {
				PK:           "USER#agent1",
				SK:           storagemodels.SKMetadata,
				Username:     "agent1",
				Approved:     true,
				Version:      1,
				CreatedAt:    now.Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
				Metadata: map[string]any{
					"agent_delegated_scopes": []any{"read", "write:statuses"},
				},
			},
			"human": {
				PK:        "USER#human",
				SK:        storagemodels.SKMetadata,
				Username:  "human",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
				IsAgent:   false,
			},
		},
	}

	t.Run("resolveDelegatedAgentAccount branches", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, err := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner"}, "agent1", []string{"write:statuses"})
		require.NoError(t, err)
		require.Nil(t, resp)
		require.NotNil(t, account)

		account, resp, err = h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner"}, "human", []string{"read"})
		require.NoError(t, err)
		require.Nil(t, account)
		require.NotNil(t, resp)

		account, resp, err = h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "intruder"}, "agent1", []string{"read"})
		require.NoError(t, err)
		require.Nil(t, account)
		require.NotNil(t, resp)

		account, resp, err = h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner"}, "agent1", []string{"follow"})
		require.NoError(t, err)
		require.Nil(t, account)
		require.NotNil(t, resp)
	})

	t.Run("validateAgentDelegationRequest invalid username", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)
		resp, err := h.validateAgentDelegationRequest(ctx, &apimodels.AgentDelegationRequest{AgentUsername: "not valid"})
		require.NotNil(t, resp)
		require.NoError(t, err)
	})

	t.Run("agent envelope helpers", func(t *testing.T) {
		user := &storage.User{
			Metadata: map[string]any{
				"agent_delegated_scopes": []any{"read", "write"},
			},
		}
		scopes, ok := agentDelegationEnvelope(user)
		require.True(t, ok)
		require.Equal(t, []string{"read", "write"}, scopes)

		scopes, ok = agentDelegationEnvelope(&storage.User{})
		require.False(t, ok)
		require.Nil(t, scopes)

		require.NoError(t, validateDelegationAgainstAgentEnvelope(user, []string{"read"}))
		require.Error(t, validateDelegationAgainstAgentEnvelope(user, []string{"follow"}))
	})

	t.Run("deriveAgentCapabilitiesFromScopes", func(t *testing.T) {
		caps := deriveAgentCapabilitiesFromScopes([]string{"write", "follow", "write:statuses"})
		require.True(t, caps.CanPost)
		require.True(t, caps.CanReply)
		require.True(t, caps.CanBoost)
		require.True(t, caps.CanDM)
		require.True(t, caps.CanFollow)
	})
}
