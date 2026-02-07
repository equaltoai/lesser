package handlers

import (
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentGovernance_AdminPolicy_ErrorBranches_Round15(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	state := &round10QueryState{
		agentInstanceConfig: policy,
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	t.Run("missing token returns unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/agents/policy", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleAdminGetAgentPolicyLift(ctx))
	})

	t.Run("repos nil returns internal error", func(t *testing.T) {
		origRepos := h.repos
		t.Cleanup(func() { h.repos = origRepos })
		h.repos = nil

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/agents/policy", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetAgentPolicyLift(ctx))
	})

	t.Run("invalid JSON body returns bad request", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/admin/agents/policy", headers, nil, []byte("{bad"))
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminUpdateAgentPolicyLift(ctx))
	})

	t.Run("validation errors return bad request", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/agents/policy", headers, nil, apimodels.UpdateAdminAgentPolicyRequest{
			DefaultQuarantineDays: -1,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminUpdateAgentPolicyLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPut, "/api/v1/admin/agents/policy", headers, nil, apimodels.UpdateAdminAgentPolicyRequest{
			RemoteQuarantineDays: 999,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminUpdateAgentPolicyLift(ctx))
	})
}
