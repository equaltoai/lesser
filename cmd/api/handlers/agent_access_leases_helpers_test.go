package handlers

import (
	"testing"
	"time"

	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAgentAccessLeaseOptions_DefaultsAndSorting(t *testing.T) {
	opts, err := normalizeAgentAccessLeaseOptions(
		"",
		"Agent-0",
		"simulacrum",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"",
		[]string{"write", "read", "follow"},
		"",
		0,
		0,
		false,
	)
	require.NoError(t, err)
	require.NotEmpty(t, opts.LeaseID)
	require.Equal(t, "local-agent", opts.DeviceLabel)
	require.Equal(t, agentAccessLeaseDefaultIdleHrs, opts.IdleTimeoutHours)
	require.Equal(t, agentAccessLeaseDefaultAbsHrs, opts.AbsoluteTTLHours)
	require.Equal(t, []string{"follow", "read", "write"}, opts.Scopes)
}

func TestNormalizeAgentAccessLeaseOptions_ClampsAbsoluteToIdle(t *testing.T) {
	opts, err := normalizeAgentAccessLeaseOptions(
		"lease-1",
		"Agent-0",
		"simulacrum",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"",
		[]string{"read"},
		"mac-mini",
		72,
		24,
		true,
	)
	require.NoError(t, err)
	require.Equal(t, 72, opts.IdleTimeoutHours)
	require.Equal(t, 72, opts.AbsoluteTTLHours)
}

func TestComputeLeaseExpiries_ClampsIdleToAbsolute(t *testing.T) {
	now := time.Date(2026, time.March, 12, 15, 0, 0, 0, time.UTC)
	idle, absolute := computeLeaseExpiries(now, 48, 24)
	require.Equal(t, now.Add(24*time.Hour), absolute)
	require.Equal(t, absolute, idle)
}

func TestEffectiveAgentAccessLeaseStatus(t *testing.T) {
	now := time.Date(2026, time.March, 12, 15, 0, 0, 0, time.UTC)

	active := &storageModels.AgentAccessLease{
		Status:            agentAccessLeaseStatusActive,
		IdleExpiresAt:     now.Add(time.Hour),
		AbsoluteExpiresAt: now.Add(24 * time.Hour),
	}
	require.Equal(t, agentAccessLeaseStatusActive, effectiveAgentAccessLeaseStatus(active, now))

	revoked := &storageModels.AgentAccessLease{
		Status:            agentAccessLeaseStatusRevoked,
		IdleExpiresAt:     now.Add(time.Hour),
		AbsoluteExpiresAt: now.Add(24 * time.Hour),
	}
	require.Equal(t, agentAccessLeaseStatusRevoked, effectiveAgentAccessLeaseStatus(revoked, now))

	expired := &storageModels.AgentAccessLease{
		Status:            agentAccessLeaseStatusActive,
		IdleExpiresAt:     now.Add(-time.Minute),
		AbsoluteExpiresAt: now.Add(24 * time.Hour),
	}
	require.Equal(t, "expired", effectiveAgentAccessLeaseStatus(expired, now))
}
