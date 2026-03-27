package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentGovernanceStateBeforeCreateNormalizesAndSetsKeys(t *testing.T) {
	start := time.Date(2026, 3, 27, 9, 0, 0, 0, time.FixedZone("offset", -4*60*60))
	end := start.Add(24 * time.Hour)
	approvedAt := start.Add(90 * time.Minute)

	state := &AgentGovernanceState{
		Username:             " Agent ",
		QuarantineStatus:     " quarantined ",
		QuarantineStart:      &start,
		QuarantineEnd:        &end,
		QuarantineApprovedBy: " admin ",
		QuarantineApprovedAt: &approvedAt,
		DelegatedScopes:      []string{"write", "read", "read", ""},
		SelfScopes:           []string{"follow", "write", "follow"},
		Verified:             true,
		VerifiedAt:           &approvedAt,
		VerifiedBy:           " reviewer ",
		VerifiedReason:       " approved ",
		UnverifiedAt:         &end,
		UnverifiedBy:         " owner ",
		UnverifiedReason:     " manual ",
		KeyRotatedAt:         &start,
	}

	require.NoError(t, state.BeforeCreate())
	require.Equal(t, MainTableName, state.TableName())
	require.Equal(t, "agent", state.Username)
	require.Equal(t, "USER#agent", state.PK)
	require.Equal(t, SKAgentGovernance, state.SK)
	require.Equal(t, "quarantined", state.QuarantineStatus)
	require.Equal(t, "admin", state.QuarantineApprovedBy)
	require.Equal(t, "reviewer", state.VerifiedBy)
	require.Equal(t, "approved", state.VerifiedReason)
	require.Equal(t, "owner", state.UnverifiedBy)
	require.Equal(t, "manual", state.UnverifiedReason)
	require.Equal(t, []string{"read", "write"}, state.DelegatedScopes)
	require.Equal(t, []string{"follow", "write"}, state.SelfScopes)
	require.Equal(t, start.UTC(), *state.QuarantineStart)
	require.Equal(t, end.UTC(), *state.QuarantineEnd)
	require.Equal(t, approvedAt.UTC(), *state.QuarantineApprovedAt)
	require.Equal(t, approvedAt.UTC(), *state.VerifiedAt)
	require.Equal(t, end.UTC(), *state.UnverifiedAt)
	require.Equal(t, start.UTC(), *state.KeyRotatedAt)
	require.False(t, state.CreatedAt.IsZero())
	require.False(t, state.UpdatedAt.IsZero())
}

func TestAgentGovernanceStateBeforeUpdatePreservesTimesAndKeys(t *testing.T) {
	createdAt := time.Date(2026, 3, 20, 8, 0, 0, 0, time.FixedZone("offset", -7*60*60))
	updatedAt := time.Date(2026, 3, 27, 11, 0, 0, 0, time.FixedZone("offset", -7*60*60))

	state := &AgentGovernanceState{
		Username:         "AGENT",
		DelegatedScopes:  []string{"write", "read"},
		SelfScopes:       []string{"follow", "follow"},
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		QuarantineStatus: " approved ",
	}

	require.NoError(t, state.BeforeUpdate())
	require.Equal(t, "agent", state.Username)
	require.Equal(t, "USER#agent", state.GetPK())
	require.Equal(t, SKAgentGovernance, state.GetSK())
	require.Equal(t, createdAt.UTC(), state.CreatedAt)
	require.Equal(t, updatedAt.UTC(), state.UpdatedAt)
	require.Equal(t, []string{"read", "write"}, state.DelegatedScopes)
	require.Equal(t, []string{"follow"}, state.SelfScopes)
	require.Equal(t, "approved", state.QuarantineStatus)
}

func TestAgentGovernanceStateValidationHelpers(t *testing.T) {
	state := &AgentGovernanceState{}
	require.Error(t, state.UpdateKeys())

	var nilState *AgentGovernanceState
	require.Empty(t, nilState.GetPK())
	require.Empty(t, nilState.GetSK())
	require.Error(t, nilState.UpdateKeys())
	require.Error(t, nilState.BeforeCreate())
	require.Error(t, nilState.BeforeUpdate())

	require.Error(t, (&AgentGovernanceState{Username: " "}).BeforeCreate())
	require.Error(t, (&AgentGovernanceState{Username: " "}).BeforeUpdate())
}
