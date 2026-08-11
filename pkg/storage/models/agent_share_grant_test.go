package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentShareGrantTableName(t *testing.T) {
	require.Equal(t, MainTableName, (AgentShareGrant{}).TableName())
}

func TestAgentShareGrantUpdateKeys(t *testing.T) {
	grantedAt := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	grant := &AgentShareGrant{
		AgentUsername:   " Agent-One ",
		GranteeUsername: " Alice ",
		GrantedBy:       " Owner ",
		GrantedAt:       grantedAt,
	}
	require.NoError(t, grant.UpdateKeys())
	require.Equal(t, "agent-one", grant.AgentUsername)
	require.Equal(t, "alice", grant.GranteeUsername)
	require.Equal(t, "owner", grant.GrantedBy)
	require.Equal(t, "USER#agent-one", grant.PK)
	require.Equal(t, "AGENT_SHARE#GRANTEE#alice", grant.SK)
	require.Equal(t, "AGENT_SHARE#GRANTEE#alice", grant.GSI2PK)
	require.Equal(t, "AGENT#agent-one", grant.GSI2SK)
	require.Equal(t, time.UTC, grant.GrantedAt.Location())
	require.Nil(t, grant.RevokedAt)
}

func TestAgentShareGrantUpdateKeysRevokedGrantIsNotIndexed(t *testing.T) {
	revokedAt := time.Now()
	grant := &AgentShareGrant{
		AgentUsername:   "agent",
		GranteeUsername: "alice",
		GrantedBy:       "owner",
		GrantedAt:       time.Now(),
		RevokedAt:       &revokedAt,
		RevokedBy:       "owner",
	}
	require.NoError(t, grant.UpdateKeys())
	require.Empty(t, grant.GSI2PK)
	require.Empty(t, grant.GSI2SK)
	require.Equal(t, time.UTC, grant.RevokedAt.Location())
}

func TestAgentShareGrantUpdateKeysValidation(t *testing.T) {
	require.Error(t, (*AgentShareGrant)(nil).UpdateKeys())
	require.Error(t, (&AgentShareGrant{}).UpdateKeys())
	require.Error(t, (&AgentShareGrant{
		AgentUsername:   "agent",
		GranteeUsername: "alice",
		GrantedBy:       "owner",
		RevokedAt:       new(time.Time),
	}).UpdateKeys())
}
