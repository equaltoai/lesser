package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentMemorySearchRound13_AuthAndValidationBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.HybridRetrievalEnabled = false

	now := time.Now().UTC()
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
				CreatedAt: now.Add(-24 * time.Hour),
				IsAgent:   true,
				AgentCapabilities: &agents.Capabilities{
					CanPost:         true,
					MaxPostsPerHour: 10,
				},
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	t.Run("missing token -> unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleAgentMemorySearchLift(ctx))
	})

	t.Run("non-agent token -> forbidden", func(t *testing.T) {
		userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + userToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAgentMemorySearchLift(ctx))
	})

	t.Run("invalid mode -> validation error", func(t *testing.T) {
		agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + agentToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", headers, map[string]string{
			"mode": "nope",
		}, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleAgentMemorySearchLift(ctx))
	})

	t.Run("hybrid mode disabled by policy -> forbidden", func(t *testing.T) {
		agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + agentToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", headers, map[string]string{
			"mode":  "hybrid",
			"query": "hello",
		}, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleAgentMemorySearchLift(ctx))
	})

	t.Run("invalid date range -> validation error", func(t *testing.T) {
		agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + agentToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", headers, map[string]string{
			"since_date": "2026-99-01",
		}, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleAgentMemorySearchLift(ctx))
	})
}

