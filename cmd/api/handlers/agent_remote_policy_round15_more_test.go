package handlers

import (
	"errors"
	"testing"
	"time"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRemoteAgentPolicy_Round15_MoreBranches(t *testing.T) {
	t.Run("nil receiver returns false", func(t *testing.T) {
		var h *Handler
		require.False(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://remote.example/users/alice"))
	})

	t.Run("agents disabled returns false", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = false
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		require.False(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://remote.example/users/alice"))
	})

	t.Run("blocked and trusted domain behavior", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.AllowRemoteAgents = true
		policy.RemoteQuarantineDays = 7
		policy.TrustedAgentDomains = []string{"trusted.example"}
		policy.BlockedAgentDomains = []string{"blocked.example"}

		now := time.Now().UTC()
		state := &round10QueryState{
			agentInstanceConfig: policy,
			remoteActorsByPK: map[string]storagemodels.RemoteActor{
				"REMOTE_ACTOR#alice@remote.example": {
					PK:       "REMOTE_ACTOR#alice@remote.example",
					SK:       storagemodels.SKProfile,
					CachedAt: now.Add(-1 * time.Hour),
				},
				"REMOTE_ACTOR#bob@remote.example": {
					PK:       "REMOTE_ACTOR#bob@remote.example",
					SK:       storagemodels.SKProfile,
					CachedAt: now.Add(-10 * 24 * time.Hour),
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)

		require.False(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://trusted.example/users/agent"))
		require.True(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://blocked.example/users/agent"))
		require.False(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://example.com/users/agent"))

		// Quarantine active for alice, expired for bob.
		require.True(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://remote.example/users/alice"))
		require.False(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://remote.example/users/bob"))

		// If remote agents are not allowed at all, hide by default.
		disallow := storagemodels.NewAgentInstanceConfig()
		disallow.AllowAgents = true
		disallow.AllowRemoteAgents = false
		disallow.RemoteQuarantineDays = 7
		hDisallow, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: disallow})
		require.True(t, hDisallow.shouldHideRemoteAgentActor(contextBackground(t), "https://remote.example/users/bob"))
	})

	t.Run("remoteAgentQuarantineActive handles not-found, expired, and lookup errors", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.AllowRemoteAgents = true
		policy.RemoteQuarantineDays = 7

		now := time.Now().UTC()
		state := &round10QueryState{
			agentInstanceConfig: policy,
			remoteActorsByPK: map[string]storagemodels.RemoteActor{
				"REMOTE_ACTOR#expired@remote.example": {
					PK:        "REMOTE_ACTOR#expired@remote.example",
					SK:        storagemodels.SKProfile,
					CachedAt:  now.Add(-1 * time.Hour),
					ExpiresAt: now.Add(-1 * time.Minute),
				},
				"REMOTE_ACTOR#zero@remote.example": {
					PK: "REMOTE_ACTOR#zero@remote.example",
					SK: storagemodels.SKProfile,
				},
			},
			notFoundPKs: map[string]bool{
				"REMOTE_ACTOR#missing@remote.example": true,
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)

		require.True(t, h.remoteAgentQuarantineActive(contextBackground(t), "https://remote.example", policy), "missing handle defaults to quarantine")
		require.True(t, h.remoteAgentQuarantineActive(contextBackground(t), "https://remote.example/users/missing", policy), "not-found defaults to quarantine")
		require.True(t, h.remoteAgentQuarantineActive(contextBackground(t), "https://remote.example/users/expired", policy), "expired cache enforces quarantine")
		require.True(t, h.remoteAgentQuarantineActive(contextBackground(t), "https://remote.example/users/zero", policy), "missing timestamps defaults to quarantine")

		// Non-notfound errors should not enforce quarantine.
		stateErr := &round10QueryState{
			agentInstanceConfig: policy,
			firstErrorOnce:      errors.New("boom"),
		}
		hErr, _, _ := round11NewHandler(t, cfg, stateErr)
		require.False(t, hErr.remoteAgentQuarantineActive(contextBackground(t), "https://remote.example/users/alice", policy))
	})
}
