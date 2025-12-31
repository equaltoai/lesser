package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeveredRelationship_KeysAndLifecycle(t *testing.T) {
	t.Run("NewSeveredRelationship sets defaults", func(t *testing.T) {
		before := time.Now()
		s := NewSeveredRelationship("local", "remote", SeveranceReasonDefederation)
		require.NotNil(t, s)

		assert.NotEmpty(t, s.ID)
		assert.Equal(t, "local", s.LocalInstance)
		assert.Equal(t, "remote", s.RemoteInstance)
		assert.Equal(t, SeveranceReasonDefederation, s.Reason)
		assert.Equal(t, SeveranceStatusActive, s.Status)
		assert.True(t, s.Reversible)
		assert.True(t, time.Unix(s.TTL, 0).After(before.Add(179*24*time.Hour)))
		assert.Equal(t, MainTableName, s.TableName())
	})

	t.Run("UpdateKeys sets PK/SK and status index", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		s := &SeveredRelationship{
			LocalInstance:  "local",
			RemoteInstance: "remote",
			Status:         SeveranceStatusActive,
			DetectedAt:     ts,
		}
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "SEVERED#local", s.PK)
		assert.Equal(t, "INSTANCE#remote#1700000000", s.SK)
		assert.Equal(t, "STATUS#ACTIVE", s.GSI1PK)
		assert.Equal(t, "TIMESTAMP#1700000000", s.GSI1SK)
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())
	})

	t.Run("Acknowledge and MarkReconnectionAttempt update fields", func(t *testing.T) {
		s := &SeveredRelationship{Status: SeveranceStatusActive}
		s.Acknowledge()
		assert.Equal(t, SeveranceStatusAcknowledged, s.Status)
		require.NotNil(t, s.AcknowledgedAt)
		assert.False(t, s.UpdatedAt.IsZero())

		prev := s.UpdatedAt
		s.MarkReconnectionAttempt()
		assert.True(t, s.ReconnectionAttempt)
		assert.True(t, s.UpdatedAt.After(prev) || prev.IsZero())
	})
}

func TestAffectedRelationship_Keys(t *testing.T) {
	before := time.Now()
	ar := NewAffectedRelationship("sev1", "actor1", "@a", "example.com", "follower", time.Unix(1700000000, 0).UTC())
	require.NotNil(t, ar)
	assert.True(t, time.Unix(ar.TTL, 0).After(before.Add(179*24*time.Hour)))

	require.NoError(t, ar.UpdateKeys())
	assert.Equal(t, "SEVERED#sev1", ar.PK)
	assert.Equal(t, "AFFECTED#actor1", ar.SK)
	assert.Equal(t, "ACTOR#actor1", ar.GSI1PK)
	assert.Equal(t, "SEVERED#sev1", ar.GSI1SK)
	assert.Equal(t, ar.PK, ar.GetPK())
	assert.Equal(t, ar.SK, ar.GetSK())
	assert.Equal(t, MainTableName, (AffectedRelationship{}).TableName())
}

func TestSeveranceReconnectionAttempt_Lifecycle(t *testing.T) {
	t.Run("NewSeveranceReconnectionAttempt sets defaults", func(t *testing.T) {
		before := time.Now()
		ra := NewSeveranceReconnectionAttempt("sev1", "admin")
		require.NotNil(t, ra)
		assert.NotEmpty(t, ra.ID)
		assert.Equal(t, "sev1", ra.SeveranceID)
		assert.Equal(t, "pending", ra.Status)
		assert.Empty(t, ra.ErrorMessages)
		assert.True(t, time.Unix(ra.TTL, 0).After(before.Add(89*24*time.Hour)))
		assert.Equal(t, MainTableName, (SeveranceReconnectionAttempt{}).TableName())

		require.NoError(t, ra.UpdateKeys())
		assert.Equal(t, "SEVERED#sev1", ra.PK)
		assert.Contains(t, ra.SK, "RECONNECT#")
		assert.Equal(t, ra.PK, ra.GetPK())
		assert.Equal(t, ra.SK, ra.GetSK())
	})

	t.Run("MarkInProgress/MarkCompleted/MarkFailed", func(t *testing.T) {
		ra := &SeveranceReconnectionAttempt{}
		ra.MarkInProgress()
		assert.Equal(t, "in_progress", ra.Status)

		ra.MarkCompleted(2, 1)
		assert.Equal(t, "completed", ra.Status)
		assert.Equal(t, 2, ra.SuccessCount)
		assert.Equal(t, 1, ra.FailureCount)
		require.NotNil(t, ra.CompletedAt)

		ra.MarkFailed("boom")
		assert.Equal(t, "failed", ra.Status)
		assert.Contains(t, ra.ErrorMessages, "boom")
		require.NotNil(t, ra.CompletedAt)
	})
}

