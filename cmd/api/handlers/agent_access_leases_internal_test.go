package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentAccessLeasesRound20_InternalHelpers(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	state := &round10QueryState{
		agentInstanceConfig: func() *storagemodels.AgentInstanceConfig {
			p := storagemodels.NewAgentInstanceConfig()
			p.AllowAgents = true
			p.AllowAgentRegistration = true
			return p
		}(),
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
				Metadata: map[string]any{
					"agent_delegated_scopes": []any{"read", "write"},
				},
			},
		},
		walletCredentialsByUser: map[string][]storagemodels.WalletCredential{
			"owner": {{
				Username: "owner",
				Address:  "0x1111111111111111111111111111111111111111",
				Type:     "ethereum",
			}},
			"agent1": {{
				Username: "agent1",
				Address:  "0x2222222222222222222222222222222222222222",
				Type:     "ethereum",
			}},
		},
		agentAccessLeasesByKey: map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#lease-1": {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#lease-1",
				ID:                "lease-1",
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   "0x1111111111111111111111111111111111111111",
				AgentWallet:       "0x2222222222222222222222222222222222222222",
				Scopes:            []string{"read", "write"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				TokenTTLHours:     12,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		},
		agentAccessChallengesByID: map[string]storagemodels.AgentAccessLeaseChallenge{
			"challenge-1": {
				PK:                "AGENT_ACCESS_CHALLENGE#challenge-1",
				SK:                "CHALLENGE",
				ID:                "challenge-1",
				LeaseID:           "lease-1",
				Username:          "agent1",
				Action:            agentAccessLeaseActionPrincipal,
				Address:           "0x1111111111111111111111111111111111111111",
				PrincipalUsername: "owner",
				PrincipalWallet:   "0x1111111111111111111111111111111111111111",
				AgentWallet:       "0x2222222222222222222222222222222222222222",
				Scopes:            []string{"read", "write"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				TokenTTLHours:     12,
				Message:           "challenge",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	headers := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{"write"})}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["username"] = "agent1"

	claims, account, resp, err := h.requireOwnedAgentAccount(ctx, "agent1")
	require.Nil(t, resp)
	require.NoError(t, err)
	require.Equal(t, "owner", claims.Username)
	require.Equal(t, "agent1", account.User.Username)

	claims, account, resp, err = h.requireManagedAgentAccount(ctx, "agent1")
	require.Nil(t, resp)
	require.NoError(t, err)
	require.Equal(t, "owner", claims.Username)
	require.Equal(t, "agent1", account.User.Username)

	ok, err := h.userHasWallet(ctx, "owner", "0x1111111111111111111111111111111111111111")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = h.userHasWallet(ctx, "owner", "0x3333333333333333333333333333333333333333")
	require.NoError(t, err)
	require.False(t, ok)

	lease, resp, err := h.loadAgentAccessLease(ctx, "agent1", "lease-1")
	require.Nil(t, resp)
	require.NoError(t, err)
	require.Equal(t, "lease-1", lease.ID)

	leases, err := h.listAgentAccessLeases(ctx, "agent1")
	require.NoError(t, err)
	require.Len(t, leases, 1)

	challenge, resp, err := h.loadAgentAccessLeaseChallenge(ctx, "challenge-1")
	require.Nil(t, resp)
	require.NoError(t, err)
	require.Equal(t, "challenge-1", challenge.ID)

	require.NoError(t, h.markAgentAccessLeaseChallengeUsed(ctx, "challenge-1"))

	createdChallenge, err := h.createAgentAccessLeaseChallenge(ctx, agentAccessLeaseOptions{
		LeaseID:           "lease-2",
		Username:          "agent1",
		PrincipalUsername: "owner",
		PrincipalWallet:   "0x1111111111111111111111111111111111111111",
		AgentWallet:       "0x2222222222222222222222222222222222222222",
		Scopes:            []string{"read", "write"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		AbsoluteTTLHours:  48,
		TokenTTLHours:     12,
	}, agentAccessLeaseActionAgent)
	require.NoError(t, err)
	require.Equal(t, "agent1", createdChallenge.Username)
	require.Equal(t, "AGENT_ACCESS_CHALLENGE#"+createdChallenge.ID, createdChallenge.PK)
	require.Equal(t, "CHALLENGE", createdChallenge.SK)
	require.Equal(t, createdChallenge.ExpiresAt.Unix(), createdChallenge.TTL)
	require.Equal(t, 12, createdChallenge.EffectiveTokenTTLHours())

	badCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "intruder", []string{"write"})}, nil, nil)
	require.NoError(t, err)
	badCtx.Params["username"] = "agent1"
	_, _, resp, err = h.requireOwnedAgentAccount(badCtx, "agent1")
	require.NotNil(t, resp)
	require.NoError(t, err)

	t.Run("require helpers cover unauthorized and not-found paths", func(t *testing.T) {
		noAuthCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", nil, nil, nil)
		require.NoError(t, err)
		noAuthCtx.Params["username"] = "agent1"
		_, _, resp, err := h.requireManagedAgentAccount(noAuthCtx, "agent1")
		require.NotNil(t, resp)
		require.NoError(t, err)

		missingState := &round10QueryState{
			agentInstanceConfig: policyLike(cfg),
			usersByUsername:     map[string]storagemodels.User{"owner": state.usersByUsername["owner"]},
		}
		hMissing, _, _ := round11NewHandler(t, cfg, missingState)
		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", headers, nil, nil)
		require.NoError(t, err)
		ctxMissing.Params["username"] = "agent1"
		_, _, resp, err = hMissing.requireOwnedAgentAccount(ctxMissing, "agent1")
		require.NotNil(t, resp)
		require.NoError(t, err)
	})

	t.Run("load helpers cover not-found and invalid-challenge branches", func(t *testing.T) {
		notFoundState := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"AGENT_ACCESS_LEASE#agent1#LEASE#missing":          true,
				"AGENT_ACCESS_CHALLENGE#missing#CHALLENGE":         true,
				"AGENT_ACCESS_CHALLENGE#expired#CHALLENGE":         false,
				"AGENT_ACCESS_CHALLENGE#used#CHALLENGE":            false,
				"AGENT_ACCESS_CHALLENGE#storage-missing#CHALLENGE": true,
			},
			agentAccessChallengesByID: map[string]storagemodels.AgentAccessLeaseChallenge{
				"expired": {
					PK:        "AGENT_ACCESS_CHALLENGE#expired",
					SK:        "CHALLENGE",
					ID:        "expired",
					LeaseID:   "lease-1",
					Username:  "agent1",
					Action:    agentAccessLeaseActionPrincipal,
					Address:   "0x1111111111111111111111111111111111111111",
					Message:   "expired",
					IssuedAt:  now.Add(-2 * time.Minute),
					ExpiresAt: now.Add(-time.Minute),
					TTL:       now.Add(-time.Minute).Unix(),
				},
				"used": {
					PK:        "AGENT_ACCESS_CHALLENGE#used",
					SK:        "CHALLENGE",
					ID:        "used",
					LeaseID:   "lease-1",
					Username:  "agent1",
					Action:    agentAccessLeaseActionPrincipal,
					Address:   "0x1111111111111111111111111111111111111111",
					Message:   "used",
					IssuedAt:  now,
					ExpiresAt: now.Add(time.Minute),
					TTL:       now.Add(time.Minute).Unix(),
					Used:      true,
				},
			},
		}
		hNotFound, _, _ := round11NewHandler(t, cfg, notFoundState)
		ctxBase, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", headers, nil, nil)
		require.NoError(t, err)
		ctxBase.Params["username"] = "agent1"

		lease, resp, err := hNotFound.loadAgentAccessLease(ctxBase, "agent1", "missing")
		require.Nil(t, lease)
		require.NotNil(t, resp)
		require.NoError(t, err)

		challenge, resp, err := hNotFound.loadAgentAccessLeaseChallenge(ctxBase, "missing")
		require.Nil(t, challenge)
		require.NotNil(t, resp)
		require.NoError(t, err)

		challenge, resp, err = hNotFound.loadAgentAccessLeaseChallenge(ctxBase, "expired")
		require.Nil(t, challenge)
		require.NotNil(t, resp)
		require.NoError(t, err)

		challenge, resp, err = hNotFound.loadAgentAccessLeaseChallenge(ctxBase, "used")
		require.Nil(t, challenge)
		require.NotNil(t, resp)
		require.NoError(t, err)
	})
}

func policyLike(cfg *config.Config) *storagemodels.AgentInstanceConfig {
	p := storagemodels.NewAgentInstanceConfig()
	if cfg != nil {
		p.AllowAgents = cfg.AllowAgents
		p.AllowAgentRegistration = cfg.AllowAgentRegistration
	}
	return p
}
