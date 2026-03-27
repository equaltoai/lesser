package handlers

import (
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func TestAgentHandlersRound20_GuardBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	newState := func() *round10QueryState {
		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.AllowAgentRegistration = true
		policy.AgentMaxPostsPerHour = 12

		return &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"owner": {
					PK:        "USER#owner",
					SK:        storagemodels.SKMetadata,
					Username:  "owner",
					Approved:  true,
					Version:   1,
					CreatedAt: now.Add(-24 * time.Hour),
				},
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
				},
			},
			auditLogsByUser: map[string][]*storagemodels.AuthAuditLog{
				"agent1": {
					{EventType: "agent.status.create", Timestamp: now.Add(-time.Minute)},
				},
			},
		}
	}

	ownerHeaders := map[string]string{
		"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead}),
	}
	intruderHeaders := map[string]string{
		"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "intruder", []string{auth.ScopeWrite, auth.ScopeRead}),
	}
	writeOnlyHeaders := map[string]string{
		"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite}),
	}

	t.Run("update agent covers disabled invalid username missing auth and not found", func(t *testing.T) {
		disabledCfg := round10TestConfig()
		disabledCfg.AllowAgents = false
		hDisabled, _, _ := round11NewHandler(t, disabledCfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/agent1", ownerHeaders, nil, apimodels.UpdateAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(hDisabled.HandleUpdateAgentLift(ctx))

		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err = round10NewLiftContext(http.MethodPatch, "/api/v1/agents/bad user", ownerHeaders, nil, apimodels.UpdateAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		requireStatus(t, http.StatusBadRequest)(h.HandleUpdateAgentLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPatch, "/api/v1/agents/agent1", nil, nil, apimodels.UpdateAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleUpdateAgentLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPatch, "/api/v1/agents/missing", ownerHeaders, nil, apimodels.UpdateAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleUpdateAgentLift(ctx))
	})

	t.Run("update agent covers forbidden malformed body and update failure", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/agent1", intruderHeaders, nil, apimodels.UpdateAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(h.HandleUpdateAgentLift(ctx))

		ctx = round10NewLiftContextWithBodyBytes(http.MethodPatch, "/api/v1/agents/agent1", ownerHeaders, nil, []byte("{bad"))
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleUpdateAgentLift(ctx))

		state := newState()
		state.executeErrorOnce = errors.New("boom")
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodPatch, "/api/v1/agents/agent1", ownerHeaders, nil, apimodels.UpdateAgentRequest{
			DisplayName: "Updated Agent",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleUpdateAgentLift(ctx))
	})

	t.Run("delete agent covers disabled invalid username missing auth not found and update failure", func(t *testing.T) {
		disabledCfg := round10TestConfig()
		disabledCfg.AllowAgents = false
		hDisabled, _, _ := round11NewHandler(t, disabledCfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/agent1", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(hDisabled.HandleDeleteAgentLift(ctx))

		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err = round10NewLiftContext(http.MethodDelete, "/api/v1/agents/bad user", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		requireStatus(t, http.StatusBadRequest)(h.HandleDeleteAgentLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodDelete, "/api/v1/agents/agent1", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleDeleteAgentLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodDelete, "/api/v1/agents/missing", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleDeleteAgentLift(ctx))

		state := newState()
		state.executeErrorOnce = errors.New("boom")
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodDelete, "/api/v1/agents/agent1", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleDeleteAgentLift(ctx))
	})

	t.Run("activity covers invalid username insufficient scope not found and forbidden viewer", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/bad user/activity", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		requireStatus(t, http.StatusBadRequest)(h.HandleGetAgentActivityLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/activity", writeOnlyHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(h.HandleGetAgentActivityLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/agents/missing/activity", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleGetAgentActivityLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/activity", intruderHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(h.HandleGetAgentActivityLift(ctx))
	})
}

func TestAgentHelpersRound20_AdditionalGuardBranches(t *testing.T) {
	now := time.Now().UTC()

	applyAgentProfileUpdates(nil, &apimodels.UpdateAgentRequest{DisplayName: "Agent"})
	applyAgentProfileUpdates(&storage.Account{}, nil)
	applyAgentInfoUpdates(nil, &apimodels.UpdateAgentRequest{AgentType: agentTypeCustom})
	applyAgentInfoUpdates(&storage.User{}, nil)
	applyAgentQuarantineExit(nil, nil, true, now)

	var nilHandler *Handler
	nilHandler.ensureAgentActor("agent1", nil)
	nilHandler.applyAgentCapabilitiesUpdate(&apptheory.Context{}, nil, nil, &apimodels.AgentCapabilities{})
	nilHandler.applyAgentCapabilitiesUpdate(&apptheory.Context{}, &storage.User{}, nil, nil)

	scopes, ok := agentDelegationEnvelope(&storage.AgentGovernanceState{})
	require.False(t, ok)
	require.Nil(t, scopes)

	require.NoError(t, validateDelegationAgainstAgentEnvelope(&storage.AgentGovernanceState{}, []string{"read"}))

	_, resp, err := nilHandler.resolveDelegatedAgentAccount(&apptheory.Context{}, &auth.Claims{Username: "owner"}, "agent1", []string{"read"})
	require.NoError(t, err)
	require.NotNil(t, resp)

	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true
	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true
	policy.AgentMaxPostsPerHour = -1

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
	require.NoError(t, err)
	_, maxPosts := h.agentRegistrationLimits(ctx)
	require.Equal(t, agentDefaultMaxPostsPerHour, maxPosts)

	h.mastodonLogic = common.NewMastodonBusinessLogic(common.DefaultMastodonConfig(), zap.NewNop())
	resp, err = h.validateUpdateAgentRequest(ctx, &apimodels.UpdateAgentRequest{
		DisplayName: "Agent One",
		Bio:         "Ready to help.",
	})
	require.NoError(t, err)
	require.Nil(t, resp)
}
