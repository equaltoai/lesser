package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGraphAgentAccessLeaseOptions_TokenTTLPolicy(t *testing.T) {
	opts, err := normalizeGraphAgentAccessLeaseOptions(
		"",
		"agent1",
		"owner",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"",
		[]string{"write", "read"},
		"",
		24,
		72,
		0,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, 24, opts.TokenTTLHours)

	opts, err = normalizeGraphAgentAccessLeaseOptions(
		"",
		"agent1",
		"owner",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"",
		[]string{"write", "read"},
		"",
		24,
		72,
		12,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, 12, opts.TokenTTLHours)

	opts, err = normalizeGraphAgentAccessLeaseOptions(
		"",
		"agent1",
		"owner",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"",
		[]string{"write", "read"},
		"",
		24,
		72,
		36,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, 24, opts.TokenTTLHours)
}

func TestGraphAgentAccessLeaseModel_UsesEffectiveTokenTTL(t *testing.T) {
	now := time.Now().UTC()
	lease := &storageModels.AgentAccessLease{
		ID:                "lease-1",
		Username:          "agent1",
		PrincipalUsername: "owner",
		PrincipalWallet:   "0x1111111111111111111111111111111111111111",
		AgentWallet:       "0x2222222222222222222222222222222222222222",
		Scopes:            []string{"read", "write"},
		DeviceLabel:       "local-agent",
		Status:            graphAgentAccessLeaseStatusActive,
		IdleTimeoutHours:  24,
		IdleExpiresAt:     now.Add(24 * time.Hour),
		AbsoluteExpiresAt: now.Add(72 * time.Hour),
		LastUsedAt:        now,
		LeaseVersion:      1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	model := graphAgentAccessLeaseModel(lease, now)
	require.NotNil(t, model)
	require.Equal(t, 24, model.TokenTTLHours)

	lease.TokenTTLHours = 12
	model = graphAgentAccessLeaseModel(lease, now)
	require.NotNil(t, model)
	require.Equal(t, 12, model.TokenTTLHours)
}

func TestGraphAgentAccessLeaseAccountGuards_RejectSuspendedAgent(t *testing.T) {
	state := newDelegationGraphState("agent1", []string{auth.ScopeRead})
	state.users["agent1"].Suspended = true
	resolver := newDelegationResolver(t, state, false)
	ctx := delegatedAgentAuthContext("owner", "write:accounts")

	_, _, err := resolver.requireOwnedAgentLeaseAccount(ctx, "agent1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent not found")

	_, _, err = resolver.requireManagedAgentLeaseAccount(ctx, "agent1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent not found")

	err = resolver.ensureActiveAgentLeaseAccount(context.Background(), "agent1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent not found")
}

func TestGraphOwnedAgentLeaseAccountUsesSharedOwnerMatching(t *testing.T) {
	state := newDelegationGraphState("agent1", []string{auth.ScopeRead})
	state.users["agent1"].AgentOwner = "http://localhost/users/owner"
	resolver := newDelegationResolver(t, state, false)
	ctx := delegatedAgentAuthContext("owner", "write:accounts")

	claims, account, err := resolver.requireOwnedAgentLeaseAccount(ctx, "agent1")
	require.NoError(t, err)
	require.Equal(t, "owner", claims.Username)
	require.Equal(t, "agent1", account.User.Username)

	state.users["agent1"].AgentOwner = "https://remote.example/users/owner"
	_, _, err = resolver.requireOwnedAgentLeaseAccount(ctx, "agent1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not authorized to manage agent lease enrollment")
}
