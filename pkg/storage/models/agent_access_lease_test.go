package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentAccessLease_BeforeCreateDefaultsAndKeys(t *testing.T) {
	now := time.Now().UTC()
	lease := &AgentAccessLease{
		ID:                "lease-1",
		Username:          "Agent-0",
		PrincipalUsername: "simulacrum",
		PrincipalWallet:   "0x1111111111111111111111111111111111111111",
		AgentWallet:       "0x2222222222222222222222222222222222222222",
		Scopes:            []string{"read", "write"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		TokenTTLHours:     12,
		IdleExpiresAt:     now.Add(24 * time.Hour),
		AbsoluteExpiresAt: now.Add(48 * time.Hour),
	}

	require.NoError(t, lease.BeforeCreate())
	require.Equal(t, "AGENT_ACCESS_LEASE#Agent-0", lease.PK)
	require.Equal(t, "LEASE#lease-1", lease.SK)
	require.Equal(t, lease.AbsoluteExpiresAt.Unix(), lease.TTL)
	require.Equal(t, agentAccessLeaseStatusActive, lease.Status)
	require.Equal(t, 1, lease.LeaseVersion)
	require.Equal(t, 12, lease.EffectiveTokenTTLHours())
	require.False(t, lease.CreatedAt.IsZero())
	require.False(t, lease.UpdatedAt.IsZero())
	require.False(t, lease.LastUsedAt.IsZero())
}

func TestAgentAccessLease_BeforeUpdateRefreshesUpdatedAt(t *testing.T) {
	lease := &AgentAccessLease{
		ID:       "lease-1",
		Username: "Agent-0",
	}
	original := time.Now().UTC().Add(-time.Hour)
	lease.UpdatedAt = original

	require.NoError(t, lease.BeforeUpdate())
	require.Equal(t, "AGENT_ACCESS_LEASE#Agent-0", lease.PK)
	require.Equal(t, "LEASE#lease-1", lease.SK)
	require.True(t, lease.UpdatedAt.After(original))
}

func TestAgentAccessLease_UpdateKeysRequiresFields(t *testing.T) {
	lease := &AgentAccessLease{}
	require.Error(t, lease.UpdateKeys())

	var nilLease *AgentAccessLease
	require.Error(t, nilLease.UpdateKeys())
	require.Error(t, nilLease.BeforeCreate())
	require.Error(t, nilLease.BeforeUpdate())
}

func TestAgentAccessLeaseChallenge_BeforeCreateAndKeys(t *testing.T) {
	now := time.Now().UTC()
	challenge := &AgentAccessLeaseChallenge{
		ID:                "challenge-1",
		LeaseID:           "lease-1",
		Username:          "Agent-0",
		Action:            "principal_approve",
		Address:           "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD",
		PrincipalUsername: "simulacrum",
		PrincipalWallet:   "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD",
		AgentWallet:       "0x0123456789012345678901234567890123456789",
		SessionPublicKey:  "  pubkey  ",
		SessionKeyType:    "  ed25519  ",
		Scopes:            []string{"read"},
		DeviceLabel:       " local-agent ",
		TokenTTLHours:     12,
		Message:           "typed-data-payload",
		ExpiresAt:         now.Add(5 * time.Minute),
	}

	require.NoError(t, challenge.BeforeCreate())
	require.Equal(t, "AGENT_ACCESS_CHALLENGE#challenge-1", challenge.PK)
	require.Equal(t, "CHALLENGE", challenge.SK)
	require.Equal(t, challenge.ExpiresAt.Unix(), challenge.TTL)
	require.Equal(t, strings.ToLower("0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"), challenge.Address)
	require.Equal(t, strings.ToLower("0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"), challenge.PrincipalWallet)
	require.Equal(t, strings.ToLower("0x0123456789012345678901234567890123456789"), challenge.AgentWallet)
	require.Equal(t, "pubkey", challenge.SessionPublicKey)
	require.Equal(t, "ed25519", challenge.SessionKeyType)
	require.Equal(t, "local-agent", challenge.DeviceLabel)
	require.Equal(t, 12, challenge.EffectiveTokenTTLHours())
	require.False(t, challenge.IssuedAt.IsZero())
}

func TestAgentAccessLeaseChallenge_UpdateKeysAllowsSessionOnlyRenewal(t *testing.T) {
	challenge := &AgentAccessLeaseChallenge{
		ID:               "challenge-1",
		LeaseID:          "lease-1",
		Username:         "Agent-0",
		Action:           "renew_session",
		SessionPublicKey: "session-key",
		Message:          "renew",
		ExpiresAt:        time.Now().UTC().Add(time.Minute),
	}

	require.NoError(t, challenge.UpdateKeys())
	require.Equal(t, "AGENT_ACCESS_CHALLENGE#challenge-1", challenge.PK)
	require.Equal(t, "CHALLENGE", challenge.SK)
}

func TestAgentAccessLeaseChallenge_UpdateKeysRequiresSigner(t *testing.T) {
	challenge := &AgentAccessLeaseChallenge{
		ID:        "challenge-1",
		LeaseID:   "lease-1",
		Username:  "Agent-0",
		Action:    "renew_wallet",
		Message:   "renew",
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}

	require.Error(t, challenge.UpdateKeys())

	var nilChallenge *AgentAccessLeaseChallenge
	require.Error(t, nilChallenge.UpdateKeys())
	require.Error(t, nilChallenge.BeforeCreate())
}

func TestNormalizeAgentAccessLeaseTokenTTLHours(t *testing.T) {
	require.Equal(t, 24, NormalizeAgentAccessLeaseTokenTTLHours(24, 72, 0))
	require.Equal(t, 12, NormalizeAgentAccessLeaseTokenTTLHours(24, 72, 12))
	require.Equal(t, 24, NormalizeAgentAccessLeaseTokenTTLHours(24, 72, 48))
	require.Equal(t, 6, NormalizeAgentAccessLeaseTokenTTLHours(24, 6, 12))
	require.Equal(t, 0, NormalizeAgentAccessLeaseTokenTTLHours(0, 0, 0))
}

func TestAgentAccessLeaseTokenExpiresAt(t *testing.T) {
	now := time.Now().UTC()
	lease := &AgentAccessLease{
		IdleTimeoutHours:  24,
		TokenTTLHours:     6,
		AbsoluteExpiresAt: now.Add(72 * time.Hour),
	}

	expiresAt := AgentAccessLeaseTokenExpiresAt(now, lease, now.Add(24*time.Hour))
	require.WithinDuration(t, now.Add(6*time.Hour), expiresAt, time.Second)

	lease.TokenTTLHours = 48
	expiresAt = AgentAccessLeaseTokenExpiresAt(now, lease, now.Add(24*time.Hour))
	require.WithinDuration(t, now.Add(24*time.Hour), expiresAt, time.Second)

	lease.TokenTTLHours = 12
	lease.AbsoluteExpiresAt = now.Add(2 * time.Hour)
	expiresAt = AgentAccessLeaseTokenExpiresAt(now, lease, now.Add(24*time.Hour))
	require.WithinDuration(t, now.Add(2*time.Hour), expiresAt, time.Second)

	var nilLease *AgentAccessLease
	require.Equal(t, 0, nilLease.EffectiveTokenTTLHours())
	require.WithinDuration(t, now.Add(time.Hour), AgentAccessLeaseTokenExpiresAt(now, nilLease, now.Add(time.Hour)), time.Second)

	var nilChallenge *AgentAccessLeaseChallenge
	require.Equal(t, 0, nilChallenge.EffectiveTokenTTLHours())
}
