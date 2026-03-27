package handlers

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAgentGovernanceRound19_AgentPolicyFromStorage_NilAndCopy(t *testing.T) {
	out := agentPolicyFromStorage(nil)
	require.False(t, out.AllowAgents)
	require.Nil(t, out.BlockedAgentDomains)

	cfg := storagemodels.NewAgentInstanceConfig()
	cfg.AllowAgents = true
	cfg.BlockedAgentDomains = []string{"blocked.example"}
	cfg.TrustedAgentDomains = []string{"trusted.example"}

	out = agentPolicyFromStorage(cfg)
	require.True(t, out.AllowAgents)
	require.Equal(t, []string{"blocked.example"}, out.BlockedAgentDomains)
	require.Equal(t, []string{"trusted.example"}, out.TrustedAgentDomains)

	cfg.BlockedAgentDomains[0] = "changed.example"
	cfg.TrustedAgentDomains[0] = "changed.example"
	require.Equal(t, "blocked.example", out.BlockedAgentDomains[0])
	require.Equal(t, "trusted.example", out.TrustedAgentDomains[0])
}

func TestAgentGovernanceRound19_AdminGetAgentPolicy_InsufficientScope(t *testing.T) {
	cfg := round10TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/agents/policy", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusForbidden)(h.HandleAdminGetAgentPolicyLift(ctx))
}

func TestAgentGovernanceRound19_AdminUpdateAgentPolicy_ValidationAndUpdateErrorBranches(t *testing.T) {
	cfg := round10TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	cases := []struct {
		name string
		req  apimodels.UpdateAdminAgentPolicyRequest
	}{
		{name: "default_quarantine_days too high", req: apimodels.UpdateAdminAgentPolicyRequest{DefaultQuarantineDays: 366}},
		{name: "max_agents_per_owner too high", req: apimodels.UpdateAdminAgentPolicyRequest{MaxAgentsPerOwner: 1001}},
		{name: "remote_quarantine_days negative", req: apimodels.UpdateAdminAgentPolicyRequest{RemoteQuarantineDays: -1}},
		{name: "agent_max_posts_per_hour too high", req: apimodels.UpdateAdminAgentPolicyRequest{AgentMaxPostsPerHour: 10001}},
		{name: "verified_agent_max_posts_per_hour negative", req: apimodels.UpdateAdminAgentPolicyRequest{VerifiedAgentMaxPostsPerHour: -1}},
		{name: "agent_max_follows_per_hour negative", req: apimodels.UpdateAdminAgentPolicyRequest{AgentMaxFollowsPerHour: -1}},
		{name: "verified_agent_max_follows_per_hour too high", req: apimodels.UpdateAdminAgentPolicyRequest{VerifiedAgentMaxFollowsPerHour: 10001}},
		{name: "hybrid_retrieval_max_candidates too high", req: apimodels.UpdateAdminAgentPolicyRequest{HybridRetrievalMaxCandidates: 5001}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/agents/policy", headers, nil, tc.req)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleAdminUpdateAgentPolicyLift(ctx))
		})
	}

	t.Run("storage update failures return 500", func(t *testing.T) {
		state := &round10QueryState{updateErrorOnce: errors.New("boom")}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/agents/policy", headers, nil, apimodels.UpdateAdminAgentPolicyRequest{
			AllowAgents:                    true,
			AllowAgentRegistration:         true,
			DefaultQuarantineDays:          7,
			MaxAgentsPerOwner:              3,
			AllowRemoteAgents:              true,
			RemoteQuarantineDays:           7,
			AgentMaxPostsPerHour:           50,
			VerifiedAgentMaxPostsPerHour:   200,
			AgentMaxFollowsPerHour:         20,
			VerifiedAgentMaxFollowsPerHour: 100,
			HybridRetrievalEnabled:         true,
			HybridRetrievalMaxCandidates:   200,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminUpdateAgentPolicyLift(ctx))
	})
}

func TestAgentGovernanceRound19_RecordAgentGovernanceEvent_MetadataMarshalBranches(t *testing.T) {
	cfg := round10TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/policy", map[string]string{
		"x-forwarded-for": "203.0.113.10",
		"user-agent":      "test",
	}, nil, nil)
	require.NoError(t, err)

	h.recordAgentGovernanceEvent(ctx, "", "agent.test", map[string]any{"k": "v"})
	h.recordAgentGovernanceEvent(ctx, "alice", "", map[string]any{"k": "v"})

	h.recordAgentGovernanceEvent(ctx, "alice", "policy_updated", map[string]any{
		"bad": math.Inf(1),
	})
	h.recordAgentGovernanceEvent(ctx, "alice", "agent.policy_updated", map[string]any{
		"ok": true,
	})
}

func TestAgentGovernanceRound19_AdminVerifyAndUnverify_ErrorBranches(t *testing.T) {
	cfg := round10TestConfig()
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	t.Run("invalid username returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/not a username/verify", headers, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "not a username"

		requireStatus(t, http.StatusBadRequest)(h.HandleAdminVerifyAgentLift(ctx))
	})

	t.Run("agent not found returns 404", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"USER#missing#METADATA": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/missing/verify", headers, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "missing"

		requireStatus(t, http.StatusNotFound)(h.HandleAdminVerifyAgentLift(ctx))
	})

	t.Run("missing token returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent/verify", nil, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		requireStatus(t, http.StatusUnauthorized)(h.HandleAdminVerifyAgentLift(ctx))
	})

	t.Run("insufficient scope returns 403", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		token := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeRead})
		limitedHeaders := map[string]string{"Authorization": "Bearer " + token}

		verifyCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent/verify", limitedHeaders, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		verifyCtx.Params["username"] = "agent"

		requireStatus(t, http.StatusForbidden)(h.HandleAdminVerifyAgentLift(verifyCtx))

		unverifyCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent/unverify", limitedHeaders, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		unverifyCtx.Params["username"] = "agent"

		requireStatus(t, http.StatusForbidden)(h.HandleAdminUnverifyAgentLift(unverifyCtx))
	})

	t.Run("missing governance returns 503", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent": {
					PK:       "USER#agent",
					SK:       storagemodels.SKMetadata,
					Username: "agent",
					Role:     "user",
					Approved: true,
					Version:  1,
					IsAgent:  true,
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		verifyCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent/verify", headers, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		verifyCtx.Params["username"] = "agent"
		requireStatus(t, http.StatusServiceUnavailable)(h.HandleAdminVerifyAgentLift(verifyCtx))

		unverifyCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent/unverify", headers, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		unverifyCtx.Params["username"] = "agent"
		requireStatus(t, http.StatusServiceUnavailable)(h.HandleAdminUnverifyAgentLift(unverifyCtx))
	})
}

func TestAgentGovernanceRound19_AdminUnlockAgent_SuccessAndErrors(t *testing.T) {
	cfg := round10TestConfig()
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	t.Run("invalid username returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/not a username/unlock", headers, nil, apimodels.AdminUnlockAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "not a username"

		requireStatus(t, http.StatusBadRequest)(h.HandleAdminUnlockAgentLift(ctx))
	})

	t.Run("missing token returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent/unlock", nil, nil, apimodels.AdminUnlockAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		requireStatus(t, http.StatusUnauthorized)(h.HandleAdminUnlockAgentLift(ctx))
	})

	t.Run("agent not found returns 404", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{"USER#missing#METADATA": true},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/missing/unlock", headers, nil, apimodels.AdminUnlockAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "missing"

		requireStatus(t, http.StatusNotFound)(h.HandleAdminUnlockAgentLift(ctx))
	})

	t.Run("rate limit clear failures return 500", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"Agent-0": {
					PK:       "USER#Agent-0",
					SK:       storagemodels.SKMetadata,
					Username: "Agent-0",
					Role:     "user",
					Approved: true,
					Version:  1,
					IsAgent:  true,
				},
			},
			allErrorOnce: errors.New("boom"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/Agent-0/unlock", headers, nil, apimodels.AdminUnlockAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "Agent-0"

		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminUnlockAgentLift(ctx))
	})

	t.Run("success returns unlock metadata", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"Agent-0": {
					PK:       "USER#Agent-0",
					SK:       storagemodels.SKMetadata,
					Username: "Agent-0",
					Role:     "user",
					Approved: true,
					Version:  1,
					IsAgent:  true,
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/Agent-0/unlock", headers, nil, apimodels.AdminUnlockAgentRequest{
			Reason: "resume testing",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "Agent-0"

		resp := requireStatus(t, http.StatusOK)(h.HandleAdminUnlockAgentLift(ctx))
		var out apimodels.AdminUnlockAgentResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "Agent-0", out.Username)
		require.True(t, out.Unlocked)
		require.Equal(t, "admin", out.UnlockedBy)
		require.Equal(t, "resume testing", out.Reason)
		require.False(t, out.UnlockedAt.IsZero())
	})
}

func TestAgentGovernanceRound19_AdminUpdateAgentPolicy_ReposNilReturns500(t *testing.T) {
	cfg := round10TestConfig()
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	h.repos = nil

	ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/agents/policy", headers, nil, apimodels.UpdateAdminAgentPolicyRequest{AllowAgents: true})
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(h.HandleAdminUpdateAgentPolicyLift(ctx))
}

func TestAgentGovernanceRound19_AdminGetAgentPolicy_InstanceErrorReturns500(t *testing.T) {
	cfg := round10TestConfig()
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	state := &round10QueryState{firstErrorOnce: errors.New("boom")}
	h, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/agents/policy", headers, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetAgentPolicyLift(ctx))
}

func TestAgentGovernanceRound19_AdminUnverifyAgent_NotFoundBranch(t *testing.T) {
	cfg := round10TestConfig()
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	state := &round10QueryState{
		notFoundPKSK: map[string]bool{"USER#missing#METADATA": true},
	}
	h, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/missing/unverify", headers, nil, apimodels.AdminVerifyAgentRequest{})
	require.NoError(t, err)
	ctx.Params["username"] = "missing"

	requireStatus(t, http.StatusNotFound)(h.HandleAdminUnverifyAgentLift(ctx))
}

func TestAgentGovernanceRound19_NormalizeDomainList_Empty(t *testing.T) {
	require.Nil(t, normalizeDomainList(nil))
	require.Nil(t, normalizeDomainList([]string{}))
	require.Empty(t, normalizeDomainList([]string{" ", "HTTP://", "https://"}))
}

func TestAgentGovernanceRound19_RecordAgentGovernanceEvent_NoReposIsNoOp(t *testing.T) {
	h := &Handler{repos: nil}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/policy", nil, nil, nil)
	require.NoError(t, err)

	h.recordAgentGovernanceEvent(ctx, "alice", "policy_updated", map[string]any{"k": "v"})
}

func TestAgentGovernanceRound19_AdminUpdateAgentPolicy_InstanceNilReturns500(t *testing.T) {
	cfg := round10TestConfig()

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	repos := &MockRepositoryStorage{}
	repos.On("Audit").Return(nil).Maybe()
	repos.On("Account").Return(nil).Maybe()
	repos.On("Instance").Return(nil).Maybe()

	h := &Handler{
		cfg:    cfg,
		repos:  repos,
		logger: zap.NewNop(),
	}

	ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/agents/policy", headers, nil, apimodels.UpdateAdminAgentPolicyRequest{AllowAgents: true})
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(h.HandleAdminUpdateAgentPolicyLift(ctx))
}

func TestAgentGovernanceRound19_AdminUnverifyAgent_InvalidUsernameAndMissingToken(t *testing.T) {
	cfg := round10TestConfig()

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	t.Run("invalid username returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/not a username/unverify", headers, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "not a username"

		requireStatus(t, http.StatusBadRequest)(h.HandleAdminUnverifyAgentLift(ctx))
	})

	t.Run("missing token returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent/unverify", nil, nil, apimodels.AdminVerifyAgentRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		requireStatus(t, http.StatusUnauthorized)(h.HandleAdminUnverifyAgentLift(ctx))
	})
}
