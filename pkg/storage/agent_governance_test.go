package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentGovernanceStateCloneAndCopies(t *testing.T) {
	start := time.Date(2026, 3, 27, 10, 0, 0, 0, time.FixedZone("offset", -5*60*60))
	end := start.Add(24 * time.Hour)
	verifiedAt := start.Add(2 * time.Hour)

	original := &AgentGovernanceState{
		Username:             "agent",
		QuarantineStatus:     AgentQuarantineStatusQuarantined,
		QuarantineStart:      &start,
		QuarantineEnd:        &end,
		QuarantineApprovedBy: "admin",
		QuarantineApprovedAt: &verifiedAt,
		DelegatedScopes:      []string{"read", "write"},
		SelfScopes:           []string{"follow", "write"},
		SelfSovereign:        true,
		Verified:             true,
		VerifiedAt:           &verifiedAt,
		VerifiedBy:           "admin",
		VerifiedReason:       "ok",
		UnverifiedAt:         &end,
		UnverifiedBy:         "owner",
		UnverifiedReason:     "manual",
		KeyRotatedAt:         &start,
		CreatedAt:            start,
		UpdatedAt:            verifiedAt,
		Version:              7,
	}

	clone := original.Clone()
	require.NotSame(t, original, clone)
	require.Equal(t, original.Username, clone.Username)
	require.Equal(t, original.QuarantineStatus, clone.QuarantineStatus)
	require.Equal(t, original.QuarantineApprovedBy, clone.QuarantineApprovedBy)
	require.Equal(t, original.DelegatedScopes, clone.DelegatedScopes)
	require.Equal(t, original.SelfScopes, clone.SelfScopes)
	require.Equal(t, original.SelfSovereign, clone.SelfSovereign)
	require.Equal(t, original.Verified, clone.Verified)
	require.Equal(t, original.VerifiedBy, clone.VerifiedBy)
	require.Equal(t, original.VerifiedReason, clone.VerifiedReason)
	require.Equal(t, original.UnverifiedBy, clone.UnverifiedBy)
	require.Equal(t, original.UnverifiedReason, clone.UnverifiedReason)
	require.Equal(t, original.CreatedAt, clone.CreatedAt)
	require.Equal(t, original.UpdatedAt, clone.UpdatedAt)
	require.Equal(t, original.Version, clone.Version)
	require.Equal(t, start.UTC(), *clone.QuarantineStart)
	require.Equal(t, end.UTC(), *clone.QuarantineEnd)
	require.Equal(t, verifiedAt.UTC(), *clone.QuarantineApprovedAt)
	require.Equal(t, verifiedAt.UTC(), *clone.VerifiedAt)
	require.Equal(t, end.UTC(), *clone.UnverifiedAt)
	require.Equal(t, start.UTC(), *clone.KeyRotatedAt)
	require.NotSame(t, original.QuarantineStart, clone.QuarantineStart)
	require.NotSame(t, original.QuarantineEnd, clone.QuarantineEnd)

	clone.DelegatedScopes[0] = "push"
	clone.SelfScopes[0] = "read"
	require.Equal(t, []string{"read", "write"}, original.DelegatedScopes)
	require.Equal(t, []string{"follow", "write"}, original.SelfScopes)

	delegated := original.DelegatedScopesCopy()
	selfScopes := original.SelfScopesCopy()
	delegated[0] = "custom"
	selfScopes[0] = "custom"
	require.Equal(t, []string{"read", "write"}, original.DelegatedScopes)
	require.Equal(t, []string{"follow", "write"}, original.SelfScopes)

	var nilState *AgentGovernanceState
	require.Nil(t, nilState.Clone())
	require.Nil(t, nilState.DelegatedScopesCopy())
	require.Nil(t, nilState.SelfScopesCopy())
}

func TestAgentGovernanceStateQuarantineActiveAt(t *testing.T) {
	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	future := now.Add(2 * time.Hour)
	past := now.Add(-2 * time.Hour)

	var nilState *AgentGovernanceState
	active, until := nilState.QuarantineActiveAt(now)
	require.False(t, active)
	require.True(t, until.IsZero())

	active, until = (&AgentGovernanceState{
		QuarantineStatus: AgentQuarantineStatusApproved,
		QuarantineEnd:    &future,
	}).QuarantineActiveAt(now)
	require.False(t, active)
	require.True(t, until.IsZero())

	active, until = (&AgentGovernanceState{}).QuarantineActiveAt(now)
	require.False(t, active)
	require.True(t, until.IsZero())

	active, until = (&AgentGovernanceState{QuarantineEnd: &future}).QuarantineActiveAt(now)
	require.True(t, active)
	require.Equal(t, future.UTC(), until)

	active, until = (&AgentGovernanceState{QuarantineEnd: &past}).QuarantineActiveAt(now)
	require.False(t, active)
	require.Equal(t, past.UTC(), until)

	autoFuture := time.Now().UTC().Add(time.Hour)
	active, until = (&AgentGovernanceState{QuarantineEnd: &autoFuture}).QuarantineActiveAt(time.Time{})
	require.True(t, active)
	require.Equal(t, autoFuture.UTC(), until)
}

func TestAgentGovernanceStateQuarantineSummaryAt(t *testing.T) {
	now := time.Date(2026, 4, 4, 18, 0, 0, 0, time.UTC)
	start := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	past := now.Add(-time.Hour)
	approvedAt := now.Add(-30 * time.Minute)

	t.Run("nil state", func(t *testing.T) {
		var nilState *AgentGovernanceState
		summary := nilState.QuarantineSummaryAt(now)
		require.False(t, summary.Active)
		require.Empty(t, summary.Status)
		require.Nil(t, summary.Start)
		require.Nil(t, summary.End)
		require.Nil(t, summary.ApprovedAt)
		require.Empty(t, summary.ApprovedBy)
	})

	t.Run("active quarantine normalizes to quarantined", func(t *testing.T) {
		summary := (&AgentGovernanceState{
			QuarantineStatus: AgentQuarantineStatusQuarantined,
			QuarantineStart:  &start,
			QuarantineEnd:    &future,
		}).QuarantineSummaryAt(now)
		require.True(t, summary.Active)
		require.Equal(t, AgentQuarantineStatusQuarantined, summary.Status)
		require.Equal(t, start.UTC(), *summary.Start)
		require.Equal(t, future.UTC(), *summary.End)
	})

	t.Run("approved quarantine exposes release details", func(t *testing.T) {
		summary := (&AgentGovernanceState{
			QuarantineStatus:     AgentQuarantineStatusApproved,
			QuarantineStart:      &start,
			QuarantineEnd:        &past,
			QuarantineApprovedBy: "admin",
			QuarantineApprovedAt: &approvedAt,
		}).QuarantineSummaryAt(now)
		require.False(t, summary.Active)
		require.Equal(t, AgentQuarantineStatusApproved, summary.Status)
		require.Equal(t, "admin", summary.ApprovedBy)
		require.Equal(t, approvedAt.UTC(), *summary.ApprovedAt)
		require.Equal(t, past.UTC(), *summary.End)
	})

	t.Run("elapsed quarantine normalizes to expired", func(t *testing.T) {
		summary := (&AgentGovernanceState{
			QuarantineStatus: AgentQuarantineStatusQuarantined,
			QuarantineStart:  &start,
			QuarantineEnd:    &past,
		}).QuarantineSummaryAt(now)
		require.False(t, summary.Active)
		require.Equal(t, AgentQuarantineStatusExpired, summary.Status)
		require.Equal(t, past.UTC(), *summary.End)
	})
}
