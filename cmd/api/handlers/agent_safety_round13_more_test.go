package handlers

import (
	"net/http"
	"strings"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

func TestAgentSafetyRound13_EnforceRails_AndLockout(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

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

	ctx := &apptheory.Context{}

	t.Run("tag limit returns 400", func(t *testing.T) {
		req := &apimodels.CreateStatusRequest{Status: "#a #b #c #d #e #f"}
		claims := &auth.Claims{Username: "agent", IsAgent: true}
		requireStatus(t, http.StatusBadRequest)(h.enforceAgentStatusCreateRails(ctx, claims, req))
	})

	t.Run("character limit returns 400", func(t *testing.T) {
		req := &apimodels.CreateStatusRequest{Status: strings.Repeat("x", 501)}
		claims := &auth.Claims{Username: "agent", IsAgent: true}
		requireStatus(t, http.StatusBadRequest)(h.enforceAgentStatusCreateRails(ctx, claims, req))
	})

	t.Run("agent_required when user missing", func(t *testing.T) {
		req := &apimodels.CreateStatusRequest{Status: "hello"}
		claims := &auth.Claims{Username: "missing", IsAgent: true}
		requireStatus(t, http.StatusForbidden)(h.enforceAgentStatusCreateRails(ctx, claims, req))
	})

	t.Run("missing governance fails closed", func(t *testing.T) {
		req := &apimodels.CreateStatusRequest{Status: "hello"}
		claims := &auth.Claims{Username: "agent", IsAgent: true}
		requireStatus(t, http.StatusServiceUnavailable)(h.enforceAgentStatusCreateRails(ctx, claims, req))
	})

	t.Run("imposeAgentLockout calls repository when available", func(t *testing.T) {
		h.imposeAgentLockout(ctx.Context(), "agent", "rapid_fire")
		h.imposeAgentLockout(ctx.Context(), "", "noop")
		require.Equal(t, "agent:unknown", agentLockoutIdentifier(""))
		require.Equal(t, "agent:agent", agentRateLimitUserID("Agent"))
		require.Equal(t, []string{"agent:agent-0", "agent:Agent-0"}, agentRateLimitUserIDVariants("Agent-0"))
		require.Equal(t, []string{"agent:unknown"}, agentRateLimitUserIDVariants(""))
		require.Equal(t, []string{"agent:agent"}, agentRateLimitUserIDVariants("agent"))
	})
}
