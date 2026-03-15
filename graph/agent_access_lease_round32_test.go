package graph

import (
	"testing"
	"time"

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
