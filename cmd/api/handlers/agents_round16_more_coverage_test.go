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
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap/zaptest"
)

func TestAgentsRound16_AgentEnablementGuards(t *testing.T) {
	t.Run("ensureAgentsEnabled returns 500 on instance repo error", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			firstErrorOnce: errors.New("boom"),
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.ensureAgentsEnabled(ctx))
	})

	t.Run("ensureAgentsEnabled rejects when instance policy disables agents", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = false

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policy,
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.ensureAgentsEnabled(ctx))
	})

	t.Run("ensureAgentsEnabled allows when instance policy enables agents", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policy,
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents", nil, nil, nil)
		require.NoError(t, err)

		resp, err := h.ensureAgentsEnabled(ctx)
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("ensureAgentRegistrationEnabled rejects when cfg disables registration", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true
		cfg.AllowAgentRegistration = false

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.AllowAgentRegistration = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policy,
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/register/challenge", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.ensureAgentRegistrationEnabled(ctx))
	})

	t.Run("ensureAgentRegistrationEnabled rejects when instance policy disables registration", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true
		cfg.AllowAgentRegistration = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.AllowAgentRegistration = false

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policy,
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/register/challenge", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.ensureAgentRegistrationEnabled(ctx))
	})
}

func TestAgentsRound16_UpdateAgentHelpers(t *testing.T) {
	t.Run("parseUpdateAgentRequest rejects invalid JSON", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPatch, "/api/v1/agents/alice", nil, nil, []byte("{bad"))
		_, resp, err := parseUpdateAgentRequest(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("ensureAgentActor hydrates missing actor", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{cfg: cfg}

		account := &storage.Account{User: &storage.User{Username: "agent"}}
		h.ensureAgentActor("agent", account)

		require.NotNil(t, account.Actor)
		require.True(t, account.User.IsAgent)
	})
}

func TestAgentsRound16_DelegationAndOwnershipHelpers(t *testing.T) {
	t.Run("validateAgentDelegationRequest enforces username and allows blank legacy profile fields", func(t *testing.T) {
		h := &Handler{}

		resp, err := h.validateAgentDelegationRequest(&apptheory.Context{}, &apimodels.AgentDelegationRequest{
			AgentUsername: "not a username",
			DisplayName:   "Agent",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)

		resp, err = h.validateAgentDelegationRequest(&apptheory.Context{}, &apimodels.AgentDelegationRequest{
			AgentUsername: "agent",
			DisplayName:   "",
		})
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("validateAgentDelegationRequest applies mastodon rules when present", func(t *testing.T) {
		h := &Handler{
			mastodonLogic: common.NewMastodonBusinessLogic(common.DefaultMastodonConfig(), zaptest.NewLogger(t)),
		}

		resp, err := h.validateAgentDelegationRequest(&apptheory.Context{}, &apimodels.AgentDelegationRequest{
			AgentUsername: "agent",
			DisplayName:   "this display name is far too long for mastodon rules",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("clampMaxPostsPerHour applies defaults and clamps", func(t *testing.T) {
		caps := &agents.Capabilities{MaxPostsPerHour: 0}
		clampMaxPostsPerHour(caps, 10)
		require.Equal(t, 10, caps.MaxPostsPerHour)

		caps.MaxPostsPerHour = 100
		clampMaxPostsPerHour(caps, 10)
		require.Equal(t, 10, caps.MaxPostsPerHour)
	})

	t.Run("applyAgentQuarantineExit initializes metadata and stamps approvals", func(t *testing.T) {
		now := time.Now().UTC()
		user := &storage.User{}
		applyAgentQuarantineExit(user, &auth.Claims{Username: "admin"}, true, now)

		require.NotNil(t, user.Metadata)
		require.Equal(t, "approved", user.Metadata["agent_quarantine_status"])
		require.Equal(t, "admin", user.Metadata["agent_quarantine_approved_by"])
	})
}
