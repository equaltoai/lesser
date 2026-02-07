package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentInstanceConfig_DefaultsAndKeys(t *testing.T) {
	cfg := NewAgentInstanceConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, instanceConfigPK, cfg.PK)
	assert.Equal(t, "AGENT_CONFIG", cfg.SK)
	assert.False(t, cfg.AllowAgents)
	assert.True(t, cfg.AllowRemoteAgents)
	assert.False(t, cfg.HybridRetrievalEnabled)
	assert.NotZero(t, cfg.UpdatedAt)

	empty := &AgentInstanceConfig{}
	require.NoError(t, empty.UpdateKeys())
	assert.Equal(t, instanceConfigPK, empty.GetPK())
	assert.Equal(t, "AGENT_CONFIG", empty.GetSK())
	assert.Equal(t, MainTableName, empty.TableName())
}

func TestAgentKeyChallenge_BeforeCreate_AndKeys(t *testing.T) {
	t.Run("nil model", func(t *testing.T) {
		var c *AgentKeyChallenge
		assert.Error(t, c.BeforeCreate())
	})

	t.Run("missing fields", func(t *testing.T) {
		c := &AgentKeyChallenge{ID: "id", Username: "u"}
		assert.Error(t, c.BeforeCreate())
	})

	t.Run("happy path derives keys and ttl", func(t *testing.T) {
		expiresAt := time.Now().Add(5 * time.Minute)
		c := &AgentKeyChallenge{
			ID:        "  challenge-1 ",
			Username:  " alice ",
			Action:    " register ",
			Nonce:     "n",
			Message:   "hello",
			ExpiresAt: expiresAt,
		}
		require.NoError(t, c.BeforeCreate())
		assert.Equal(t, MainTableName, c.TableName())
		assert.Equal(t, "challenge-1", c.ID)
		assert.Equal(t, "alice", c.Username)
		assert.Equal(t, "register", c.Action)
		assert.Equal(t, "AGENT_KEY_CHALLENGE#challenge-1", c.PK)
		assert.Equal(t, "CHALLENGE", c.SK)
		assert.Equal(t, expiresAt.UTC().Unix(), c.TTL)
		assert.False(t, c.IssuedAt.IsZero())
	})
}

func TestAgentMemoryEvent_BeforeCreate_AndIndexes(t *testing.T) {
	t.Run("missing required fields rejected", func(t *testing.T) {
		e := &AgentMemoryEvent{}
		assert.Error(t, e.BeforeCreate())
	})

	t.Run("happy path derives keys", func(t *testing.T) {
		ts := time.Now().Add(-time.Minute)
		e := &AgentMemoryEvent{
			EventID:       " evt ",
			EventType:     "CREATE",
			StatusID:      "status",
			OriginalID:    "orig",
			AgentUsername: "agent",
			Timestamp:     ts,
		}
		require.NoError(t, e.BeforeCreate())
		assert.Equal(t, MainTableName, e.TableName())
		assert.Equal(t, "evt", e.EventID)
		assert.Equal(t, "create", e.EventType)
		assert.Equal(t, "MEMORY#orig", e.PK)
		assert.Contains(t, e.SK, "EVENT#")
		assert.Equal(t, "AGENT#agent", e.GSI1PK)
		assert.Equal(t, e.SK, e.GSI1SK)
		assert.False(t, e.CreatedAt.IsZero())
	})
}

func TestAgentConcurrencySlot_BeforeCreate_AndTTL(t *testing.T) {
	t.Run("nil model", func(t *testing.T) {
		var s *AgentConcurrencySlot
		assert.Error(t, s.BeforeCreate())
	})

	t.Run("missing fields rejected", func(t *testing.T) {
		s := &AgentConcurrencySlot{PK: "x"}
		assert.Error(t, s.BeforeCreate())
	})

	t.Run("happy path trims and derives ttl", func(t *testing.T) {
		exp := time.Now().Add(10 * time.Minute)
		s := &AgentConcurrencySlot{
			PK:        " AGENT_CONCURRENCY#sess ",
			SK:        " SLOT#1 ",
			LeaseID:   " lease ",
			ExpiresAt: exp,
		}
		require.NoError(t, s.BeforeCreate())
		assert.Equal(t, MainTableName, s.TableName())
		assert.Equal(t, "AGENT_CONCURRENCY#sess", s.PK)
		assert.Equal(t, "SLOT#1", s.SK)
		assert.Equal(t, "lease", s.LeaseID)
		assert.Equal(t, exp.UTC().Unix(), s.TTL)
	})
}
