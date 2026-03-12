package handlers

import (
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentDelegationHelpersRound20(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true
	policy.DefaultQuarantineDays = 5
	policy.AgentMaxPostsPerHour = 12

	state := &round10QueryState{
		agentInstanceConfig: policy,
	}
	h, _, _ := round11NewHandler(t, cfg, state)

	t.Run("parseAgentDelegationInfo defaults", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)
		info, resp, err := parseAgentDelegationInfo(ctx, map[string]any{}, []string{"write", "follow"})
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, agentTypeCustom, info.AgentType)
		require.Equal(t, agentVersionUnknown, info.AgentVersion)
		require.NotNil(t, info.Capabilities)
		require.True(t, info.Capabilities.CanPost)
		require.True(t, info.Capabilities.CanFollow)
	})

	t.Run("parseAgentDelegationInfo invalid payload", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)
		_, resp, err := parseAgentDelegationInfo(ctx, "bad", []string{"read"})
		require.NotNil(t, resp)
		require.NoError(t, err)
	})

	t.Run("agentRegistrationLimits use policy values", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)
		quarantineDays, maxPosts := h.agentRegistrationLimits(ctx)
		require.Equal(t, 5, quarantineDays)
		require.Equal(t, 12, maxPosts)
	})

	t.Run("clampMaxPostsPerHour bounds values", func(t *testing.T) {
		var nilCaps *agents.Capabilities
		clampMaxPostsPerHour(nilCaps, 10)

		caps := &agents.Capabilities{}
		clampMaxPostsPerHour(caps, 10)
		require.Equal(t, 10, caps.MaxPostsPerHour)

		caps.MaxPostsPerHour = 99
		clampMaxPostsPerHour(caps, 10)
		require.Equal(t, 10, caps.MaxPostsPerHour)
	})

	t.Run("createDelegatedAgentAccount success", func(t *testing.T) {
		now := time.Now().UTC()
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)
		h.repos.Account().SetEncryptor(noopEncryptor{})
		account, resp, err := h.createDelegatedAgentAccount(
			ctx,
			&auth.Claims{Username: "owner"},
			&apimodels.AgentDelegationRequest{
				AgentUsername: "agent1",
				DisplayName:   "Agent One",
				Bio:           "helper",
			},
			delegationResolvedInfo{
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
				Capabilities: &agents.Capabilities{},
			},
			[]string{"read", "write"},
			now,
		)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.NotNil(t, account)
		require.Equal(t, "agent1", account.User.Username)
		require.True(t, account.User.IsAgent)
		require.Equal(t, "@owner", account.User.AgentOwner)
		require.Equal(t, "quarantined", account.User.Metadata["agent_quarantine_status"])
		require.NotEmpty(t, account.PrivateKey)
		require.NotNil(t, account.Actor)
	})

	t.Run("createDelegatedAgentAccount conflict", func(t *testing.T) {
		conflictState := &round10QueryState{
			agentInstanceConfig: policy,
			createErrorOnce:     common.ConflictError{Resource: "agent", Message: "exists"},
		}
		hConflict, _, _ := round11NewHandler(t, cfg, conflictState)
		hConflict.repos.Account().SetEncryptor(noopEncryptor{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)
		account, resp, err := hConflict.createDelegatedAgentAccount(
			ctx,
			&auth.Claims{Username: "owner"},
			&apimodels.AgentDelegationRequest{AgentUsername: "agent1"},
			delegationResolvedInfo{
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
				Capabilities: &agents.Capabilities{},
			},
			[]string{"read"},
			time.Now().UTC(),
		)
		require.NoError(t, err)
		require.Nil(t, account)
		require.NotNil(t, resp)
	})

	t.Run("createDelegatedAgentAccount internal error", func(t *testing.T) {
		errorState := &round10QueryState{
			agentInstanceConfig: policy,
			createErrorOnce:     errors.New("boom"),
		}
		hError, _, _ := round11NewHandler(t, cfg, errorState)
		hError.repos.Account().SetEncryptor(noopEncryptor{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)
		account, resp, err := hError.createDelegatedAgentAccount(
			ctx,
			&auth.Claims{Username: "owner"},
			&apimodels.AgentDelegationRequest{AgentUsername: "agent1"},
			delegationResolvedInfo{
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
				Capabilities: &agents.Capabilities{},
			},
			[]string{"read"},
			time.Now().UTC(),
		)
		require.NoError(t, err)
		require.Nil(t, account)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})
}
