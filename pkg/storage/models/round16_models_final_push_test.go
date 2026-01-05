package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeline_UpdateKeys_SortKey_AndGSIBranches(t *testing.T) {
	t.Run("UpdateKeys validates required fields", func(t *testing.T) {
		tl := &Timeline{}
		assert.Error(t, tl.UpdateKeys())

		tl.TimelineType = "HOME"
		assert.Error(t, tl.UpdateKeys())

		tl.TimelineID = "alice"
		assert.Error(t, tl.UpdateKeys())
	})

	t.Run("UpdateKeys sets keys and GSIs for public timeline", func(t *testing.T) {
		timestamp := time.Unix(1700000000, 0).UTC()
		tl := &Timeline{
			TimelineType: TimelinePublic,
			TimelineID:   "LOCAL",
			PostID:       "post1",
			ActorID:      "actor1",
			Visibility:   "private",
			Language:     "",
			TimelineAt:   timestamp,
			EntryID:      "e1",
		}

		require.NoError(t, tl.UpdateKeys())
		assert.Equal(t, "TIMELINE#PUBLIC#LOCAL", tl.PK)
		assert.Contains(t, tl.SK, "#post1")

		assert.Equal(t, "TIMELINE#PUBLIC#LOCAL", tl.GSI1PK)
		assert.Contains(t, tl.GSI1SK, "#post1")

		assert.Equal(t, "ACTOR#actor1", tl.GSI2PK)
		assert.Contains(t, tl.GSI2SK, "#e1")

		// private visibility clears GSI3
		assert.Empty(t, tl.GSI3PK)
		assert.Empty(t, tl.GSI3SK)
		// empty language clears GSI4
		assert.Empty(t, tl.GSI4PK)
		assert.Empty(t, tl.GSI4SK)

		assert.Equal(t, tl.PK, tl.GetPK())
		assert.Equal(t, tl.SK, tl.GetSK())
	})

	t.Run("GetSortKey uses reverse timestamp and EntryID", func(t *testing.T) {
		tl := &Timeline{TimelineAt: time.Unix(1700000000, 0).UTC(), EntryID: "e1"}
		assert.True(t, strings.HasSuffix(tl.GetSortKey(), "#e1"))
	})

	t.Run("SetTTL sets TTL only when non-zero", func(t *testing.T) {
		tl := &Timeline{}
		tl.SetTTL(time.Time{})
		assert.Equal(t, int64(0), tl.TTL)

		expiresAt := time.Unix(1700000000, 0).UTC()
		tl.SetTTL(expiresAt)
		assert.Equal(t, expiresAt.Unix(), tl.TTL)
	})

	t.Run("BeforeCreate sets TimelineAt when missing", func(t *testing.T) {
		tl := &Timeline{
			TimelineType: "HOME",
			TimelineID:   "alice",
			PostID:       "post1",
			ActorID:      "actor1",
			Visibility:   "public",
		}
		require.NoError(t, tl.BeforeCreate())
		assert.False(t, tl.TimelineAt.IsZero())
	})
}

func TestWalletModels_KeyingAndLifecycle(t *testing.T) {
	t.Run("WalletChallenge UpdateKeys and BeforeCreate", func(t *testing.T) {
		expiresAt := time.Unix(1700000000, 0).UTC()
		w := &WalletChallenge{ID: "c1", ExpiresAt: expiresAt}
		require.NoError(t, w.UpdateKeys())
		assert.Equal(t, "WALLET_CHALLENGE#c1", w.PK)
		assert.Equal(t, "CHALLENGE", w.SK)
		assert.Equal(t, expiresAt.Unix(), w.TTL)
		assert.Equal(t, MainTableName, w.TableName())
		assert.Equal(t, w.PK, w.GetPK())
		assert.Equal(t, w.SK, w.GetSK())

		w2 := &WalletChallenge{ID: "c2", ExpiresAt: expiresAt}
		before := time.Now()
		require.NoError(t, w2.BeforeCreate())
		assert.WithinDuration(t, before, w2.IssuedAt, 2*time.Second)
	})

	t.Run("WalletCredential normalizes address and sets timestamps", func(t *testing.T) {
		w := &WalletCredential{Username: "alice", Address: "0xABC", ChainID: 1}
		require.NoError(t, w.BeforeCreate())
		assert.Equal(t, "USER#alice", w.PK)
		assert.Equal(t, "WALLET#0xabc", w.SK)
		assert.False(t, w.LinkedAt.IsZero())
		assert.True(t, w.LastUsed.Equal(w.LinkedAt))
		assert.Equal(t, MainTableName, w.TableName())
		assert.Equal(t, w.PK, w.GetPK())
		assert.Equal(t, w.SK, w.GetSK())

		old := w.LastUsed
		require.NoError(t, w.BeforeUpdate())
		assert.True(t, w.LastUsed.After(old))
	})

	t.Run("WalletIndex BeforeCreate defaults wallet type and normalizes address", func(t *testing.T) {
		idx := &WalletIndex{Address: "0xABC", Username: "alice"}
		require.NoError(t, idx.BeforeCreate())
		assert.Equal(t, "WALLET#ethereum#0xabc", idx.PK)
		assert.Equal(t, "USER#alice", idx.SK)
		assert.Equal(t, "alice", idx.Username)
		assert.Equal(t, "ethereum", idx.WalletType)
		assert.Equal(t, "0xabc", idx.Address)
		assert.Equal(t, idx.PK, idx.GetPK())
		assert.Equal(t, idx.SK, idx.GetSK())
		assert.Equal(t, MainTableName, idx.TableName())
	})
}

func TestRelationshipRecord_LocalDomainClearsDomainGSIs(t *testing.T) {
	r := NewRelationshipRecord("alice", "bob", "act1")
	require.NoError(t, r.UpdateKeys())
	assert.Empty(t, r.GSI2PK)
	assert.Empty(t, r.GSI2SK)
	assert.Empty(t, r.GSI3PK)
	assert.Empty(t, r.GSI3SK)

	// Cover remaining username extractors for short SK/GSI values.
	r.SK = "FOLLOWING#"
	assert.Equal(t, "", r.ExtractFollowingUsername())
	r.GSI1SK = "FOLLOWER#"
	assert.Equal(t, "", r.ExtractFollowerFromGSI())
}

func TestRelationshipRecord_TableName_GetPK_GetSK_AndBeforeUpdateError(t *testing.T) {
	r := &RelationshipRecord{}
	assert.Equal(t, MainTableName, r.TableName())
	assert.Equal(t, "", r.GetPK())
	assert.Equal(t, "", r.GetSK())
	assert.ErrorContains(t, r.BeforeUpdate(), "failed to update keys")
}

func TestConversationStatus_UpdateKeys_SetsKeys(t *testing.T) {
	s := &ConversationStatus{ConversationID: "c1", UserID: "alice"}
	require.NoError(t, s.UpdateKeys())
	assert.Equal(t, "CONVERSATION_STATUS#c1", s.PK)
	assert.Equal(t, "USER#alice", s.SK)
}

func TestStreamingCloudWatchMetrics_TableNamesAndKeyAccessors(t *testing.T) {
	s := &StreamingCloudWatchMetrics{PK: "p", SK: "s"}
	assert.Equal(t, MainTableName, s.TableName())
	assert.Equal(t, "p", s.GetPK())
	assert.Equal(t, "s", s.GetSK())
	assert.Equal(t, MainTableName, (QualityMetric{}).TableName())
	assert.Equal(t, MainTableName, (GeographicMetric{}).TableName())
	assert.Equal(t, MainTableName, (ConcurrentViewerMetrics{}).TableName())
	assert.Equal(t, MainTableName, (StreamingPerformanceMetrics{}).TableName())
}

func TestTrustModels_DomainExtraction_TTL_AndKeys(t *testing.T) {
	t.Run("getDomainFromActorID handles URL, handle, and local", func(t *testing.T) {
		assert.Equal(t, "example.com", getDomainFromActorID("https://example.com/users/alice"))
		assert.Equal(t, "remote.tld", getDomainFromActorID("@alice@remote.tld"))
		assert.Equal(t, "local", getDomainFromActorID("alice"))
	})

	t.Run("TrustRelationship UpdateKeys sets all indexes and type", func(t *testing.T) {
		tr := &TrustRelationship{
			TrusterID: "alice",
			TrusteeID: "https://example.com/users/bob",
			Category:  TrustCategoryContent,
			Score:     0.5,
		}
		require.NoError(t, tr.UpdateKeys())
		assert.Equal(t, "TRUST#alice#content", tr.PK)
		assert.Equal(t, "TRUSTEE#https://example.com/users/bob", tr.SK)
		assert.Equal(t, "TRUSTED#https://example.com/users/bob#content", tr.GSI1PK)
		assert.Equal(t, "TRUSTER#alice", tr.GSI1SK)
		assert.Equal(t, "DOMAIN#example.com", tr.GSI2PK)
		assert.Contains(t, tr.GSI2SK, "TRUST#content#")
		assert.Equal(t, "RELATIONSHIP", tr.Type)
		assert.Equal(t, tr.PK, tr.GetPK())
		assert.Equal(t, tr.SK, tr.GetSK())
		assert.Equal(t, MainTableName, tr.TableName())
	})

	t.Run("TrustScore UpdateKeys sets TTL from CacheTTL when present", func(t *testing.T) {
		cacheTTL := time.Unix(1700000000, 0).UTC()
		ts := &TrustScore{ActorID: "alice", Category: TrustCategoryBehavior, CacheTTL: cacheTTL}
		require.NoError(t, ts.UpdateKeys())
		assert.Equal(t, "SCORE#alice#behavior", ts.PK)
		assert.Equal(t, SKCurrent, ts.SK)
		assert.Equal(t, cacheTTL.Unix(), ts.TTL)
		assert.Equal(t, "SCORE", ts.Type)
		assert.Equal(t, ts.PK, ts.GetPK())
		assert.Equal(t, ts.SK, ts.GetSK())
		assert.Equal(t, MainTableName, ts.TableName())

		ts2 := &TrustScore{ActorID: "alice", Category: TrustCategoryBehavior}
		require.NoError(t, ts2.UpdateKeys())
		assert.Equal(t, int64(0), ts2.TTL)
	})

	t.Run("TrustUpdate UpdateKeys sets TTL only when unset", func(t *testing.T) {
		when := time.Unix(1700000000, 0).UTC()
		tu := &TrustUpdate{ActorID: "alice", EventID: "e1", Timestamp: when, Category: TrustCategoryGeneral}
		require.NoError(t, tu.UpdateKeys())
		assert.Equal(t, "UPDATES#alice", tu.PK)
		assert.Contains(t, tu.SK, "TIME#")
		assert.Equal(t, "UPDATE", tu.Type)
		assert.Equal(t, when.Add(30*24*time.Hour).Unix(), tu.TTL)
		assert.Equal(t, tu.PK, tu.GetPK())
		assert.Equal(t, tu.SK, tu.GetSK())
		assert.Equal(t, MainTableName, tu.TableName())

		tu2 := &TrustUpdate{ActorID: "alice", EventID: "e1", Timestamp: when, TTL: 123}
		require.NoError(t, tu2.UpdateKeys())
		assert.Equal(t, int64(123), tu2.TTL)
	})

	t.Run("TrustEvidence TableName", func(t *testing.T) {
		assert.Equal(t, MainTableName, (TrustEvidence{}).TableName())
	})
}
