package models

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocialRecoveryRequest(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()

	s := &SocialRecoveryRequest{
		ID:            "r1",
		Username:      "alice",
		InitiatedAt:   now,
		ExpiresAt:     now.Add(1 * time.Hour),
		RequiredVotes: 2,
		Status:        StatusPending,
	}
	s.UpdateKeys()

	assert.Equal(t, "RECOVERY#alice", s.PK)
	assert.Equal(t, "REQUEST#r1", s.SK)
	assert.Equal(t, "RECOVERY_STATUS#pending", s.GSI1PK)
	assert.Contains(t, s.GSI1SK, "r1")
	assert.Equal(t, "USER#alice", s.GSI2PK)
	assert.Contains(t, s.GSI2SK, "RECOVERY#")
	assert.Equal(t, s.ExpiresAt.Unix(), s.TTL)
	assert.Equal(t, MainTableName, s.TableName())

	assert.True(t, s.AddVote("t1"))
	assert.False(t, s.AddVote("t1"))
	assert.Equal(t, 1, s.GetVoteCount())
	assert.InDelta(t, 50.0, s.GetProgress(), 0.0001)

	assert.True(t, s.AddVote("t2"))
	assert.Equal(t, "approved", s.Status)
	assert.Equal(t, "", s.GSI2PK)

	s.RequiredVotes = 0
	assert.Equal(t, 0.0, s.GetProgress())

	s.Status = StatusPending
	s.ExpiresAt = time.Now().Add(-1 * time.Second)
	assert.True(t, s.IsExpired())
	assert.False(t, s.IsActive())

	s.ExpiresAt = time.Now().Add(1 * time.Hour)
	assert.True(t, s.IsActive())

	s.Cancel()
	assert.Equal(t, "cancelled", s.Status)
	s.MarkExpired()
	assert.Equal(t, "expired", s.Status)
}

func TestDomainBlockModels(t *testing.T) {
	t.Run("UserDomainBlock keys", func(t *testing.T) {
		udb := &UserDomainBlock{Username: "alice", Domain: "example.com"}
		require.NoError(t, udb.UpdateKeys())
		assert.Equal(t, "USER#alice", udb.PK)
		assert.Equal(t, "DOMAIN_BLOCK#example.com", udb.SK)
		assert.Equal(t, MainTableName, udb.TableName())
		assert.Equal(t, udb.PK, udb.GetPK())
		assert.Equal(t, udb.SK, udb.GetSK())
	})

	t.Run("InstanceDomainBlock keys", func(t *testing.T) {
		created := time.Unix(1700000000, 0).UTC()
		idb := &InstanceDomainBlock{Domain: "example.com", CreatedAt: created}
		require.NoError(t, idb.UpdateKeys())
		assert.Equal(t, "DOMAIN_BLOCK#example.com", idb.PK)
		assert.Equal(t, "DOMAIN_BLOCK#example.com", idb.SK)
		assert.Equal(t, "DOMAIN_BLOCKS", idb.GSI1PK)
		assert.Equal(t, "INSTANCE_DOMAIN_BLOCK", idb.Type)
		assert.Equal(t, idb.PK, idb.GetPK())
		assert.Equal(t, idb.SK, idb.GetSK())
	})

	t.Run("EmailDomainBlock and DomainAllow keys", func(t *testing.T) {
		created := time.Unix(1700000000, 0).UTC()
		edb := &EmailDomainBlock{ID: "e1", Domain: "example.com", CreatedAt: created}
		require.NoError(t, edb.UpdateKeys())
		assert.Equal(t, "EMAIL_DOMAIN_BLOCK#example.com", edb.PK)
		assert.Equal(t, "EMAIL_DOMAIN_BLOCK#example.com", edb.SK)
		assert.Equal(t, "EMAIL_DOMAIN_BLOCKS", edb.GSI1PK)
		assert.Equal(t, created.Format(time.RFC3339), edb.GSI1SK)
		assert.Equal(t, "e1", edb.GetID())
		assert.Equal(t, "example.com", edb.GetDomain())

		allow := &DomainAllow{ID: "a1", Domain: "example.com", CreatedAt: created}
		require.NoError(t, allow.UpdateKeys())
		assert.Equal(t, "DOMAIN_ALLOW#example.com", allow.PK)
		assert.Equal(t, "DOMAIN_ALLOW#example.com", allow.SK)
		assert.Equal(t, "DOMAIN_ALLOWS", allow.GSI1PK)
		assert.Equal(t, created.Format(time.RFC3339), allow.GSI1SK)
		assert.Equal(t, "a1", allow.GetID())
		assert.Equal(t, "example.com", allow.GetDomain())
	})
}

func TestBlockModel(t *testing.T) {
	t.Run("UpdateKeys derives usernames for PK/SK and reverse lookup index", func(t *testing.T) {
		b := &Block{
			Actor:  "https://example.com/users/alice",
			Object: "https://other.example/users/bob",
		}
		require.NoError(t, b.UpdateKeys())
		assert.Equal(t, "ACTOR#alice#BLOCKS", b.PK)
		assert.Equal(t, "BLOCKED#bob", b.SK)
		assert.Equal(t, "BLOCKED#bob", b.GSI5PK)
		assert.Equal(t, "BLOCKER#alice", b.GSI5SK)
		assert.Equal(t, MainTableName, b.TableName())
		assert.Equal(t, b.PK, b.GetPK())
		assert.Equal(t, b.SK, b.GetSK())
	})

	t.Run("BeforeCreate populates ID and timestamps", func(t *testing.T) {
		b := &Block{
			Actor:  "https://example.com/users/alice",
			Object: "https://other.example/users/bob",
		}
		require.NoError(t, b.BeforeCreate())
		assert.Equal(t, "Block", b.Type)
		assert.NotEmpty(t, b.ID)
		assert.False(t, b.Published.IsZero())
		assert.False(t, b.CreatedAt.IsZero())
	})
}

func TestThreadNode(t *testing.T) {
	t.Run("keys, TTL, and structural helpers", func(t *testing.T) {
		before := time.Now()
		n := NewThreadNode("root", "s1", "", 0, "a1")
		after := time.Now()
		require.NoError(t, n.UpdateKeys())
		assert.Equal(t, "THREAD#root", n.PK)
		assert.Equal(t, "NODE#s1", n.SK)
		assert.Equal(t, "STATUS#s1", n.GSI1PK)
		assert.Equal(t, "THREAD_NODE", n.GSI1SK)
		assert.Equal(t, MainTableName, n.TableName())
		assert.Equal(t, n.PK, n.GetPK())
		assert.Equal(t, n.SK, n.GetSK())
		assert.GreaterOrEqual(t, n.TTL, before.Add(30*24*time.Hour).Unix()-2)
		assert.LessOrEqual(t, n.TTL, after.Add(30*24*time.Hour).Unix()+2)

		assert.True(t, n.IsRoot())
		assert.True(t, n.IsLeaf())

		n.UpdatePath("parent.path")
		assert.Equal(t, "s1", n.Path)

		n.AddChild("c1")
		n.AddChild("c1")
		assert.Equal(t, 1, n.ReplyCount)
		assert.NotNil(t, n.LastReplyAt)
		assert.False(t, n.IsLeaf())

		n.IncrementDescendantCount(2)
		assert.Equal(t, 2, n.DescendantCount)

		child := &ThreadNode{StatusID: "c1", ParentID: "s1", Depth: 1}
		child.UpdatePath("s1")
		assert.Equal(t, "s1.c1", child.Path)
	})
}

func TestRelayInfo(t *testing.T) {
	t.Run("UpdateKeys configures active and status indexes", func(t *testing.T) {
		seen := time.Unix(1700000000, 0).UTC()
		r := &RelayInfo{Domain: "example.com", Active: true, LastSeenAt: seen, Status: "pending"}
		r.UpdateKeys()
		assert.Equal(t, "RELAY#example.com", r.PK)
		assert.Equal(t, SKInfo, r.SK)
		assert.Equal(t, "ACTIVE_RELAYS", r.GSI1PK)
		assert.Contains(t, r.GSI1SK, "example.com")
		assert.Equal(t, "RELAY_STATUS#pending", r.GSI2PK)
		assert.Equal(t, "example.com", r.GSI2SK)

		r.Active = false
		r.UpdateKeys()
		assert.Empty(t, r.GSI1PK)
		assert.Empty(t, r.GSI1SK)
	})

	t.Run("expiry and retry logic", func(t *testing.T) {
		r := &RelayInfo{}
		assert.False(t, r.IsExpired())

		r.TTL = time.Now().Add(-1 * time.Second).Unix()
		assert.True(t, r.IsExpired())

		// errorCount=0 always retries
		r = &RelayInfo{ErrorCount: 0}
		assert.True(t, r.ShouldRetry())

		// rejected never retries
		r = &RelayInfo{ErrorCount: 1, Status: "rejected"}
		assert.False(t, r.ShouldRetry())

		// not enough time since last attempt
		r = &RelayInfo{ErrorCount: 1, LastSeenAt: time.Now()}
		assert.False(t, r.ShouldRetry())

		// enough time since last attempt
		r.LastSeenAt = time.Now().Add(-10 * time.Minute)
		assert.True(t, r.ShouldRetry())
	})

	t.Run("SetError/SetSuccess update state", func(t *testing.T) {
		r := &RelayInfo{}
		r.SetError()
		assert.Equal(t, 1, r.ErrorCount)
		assert.False(t, r.LastSeenAt.IsZero())

		r.ErrorCount = 10
		r.Active = true
		r.SetError()
		assert.Equal(t, "error", r.Status)
		assert.False(t, r.Active)

		r.SetSuccess()
		assert.Equal(t, 0, r.ErrorCount)
		assert.Equal(t, StatusActive, r.Status)
		assert.True(t, r.Active)
	})
}

func TestFilterModels(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()

	f := &Filter{Username: "alice", ID: "f1"}
	require.NoError(t, f.BeforeCreate())
	assert.Equal(t, "USER#alice", f.PK)
	assert.Equal(t, "FILTER#f1", f.SK)
	assert.False(t, f.CreatedAt.IsZero())
	assert.Equal(t, f.CreatedAt, f.UpdatedAt)

	updated := f.UpdatedAt
	require.NoError(t, f.BeforeSave())
	assert.True(t, f.UpdatedAt.After(updated) || f.UpdatedAt.Equal(updated))
	assert.Equal(t, MainTableName, f.TableName())
	assert.Equal(t, f.PK, f.GetPK())
	assert.Equal(t, f.SK, f.GetSK())

	fk := &FilterKeyword{FilterID: "f1", ID: "k1", Keyword: "foo", CreatedAt: now}
	require.NoError(t, fk.BeforeCreate())
	assert.Equal(t, "FILTER#f1", fk.PK)
	assert.Equal(t, "KEYWORD#k1", fk.SK)
	assert.Equal(t, MainTableName, fk.TableName())

	fs := &FilterStatus{FilterID: "f1", StatusID: "s1"}
	require.NoError(t, fs.BeforeCreate())
	assert.Equal(t, "FILTER#f1", fs.PK)
	assert.Equal(t, "STATUS#s1", fs.SK)
	assert.Equal(t, MainTableName, fs.TableName())
}

func TestTombstone(t *testing.T) {
	t.Run("UpdateKeys validates ID and sets GSIs when fields present", func(t *testing.T) {
		ts := &Tombstone{}
		assert.ErrorContains(t, ts.UpdateKeys(), "ID is required")

		deleted := time.Unix(1700000000, 0).UTC()
		ts = &Tombstone{
			ID:         "o1",
			Deleted:    deleted,
			DeletedBy:  "alice",
			FormerType: "Note",
		}
		require.NoError(t, ts.UpdateKeys())
		assert.Equal(t, "OBJECT#o1", ts.PK)
		assert.Equal(t, "TOMBSTONE", ts.SK)
		assert.Equal(t, "ACTOR#alice#TOMBSTONES", ts.GSI1PK)
		assert.Contains(t, ts.GSI1SK, "DELETED#")
		assert.Equal(t, "TOMBSTONE#Note", ts.GSI2PK)
		assert.Contains(t, ts.GSI2SK, "DELETED#")
	})

	t.Run("BeforeCreate sets defaults and cleanup behavior", func(t *testing.T) {
		ts := &Tombstone{ID: "o1", DeletedBy: "alice", FormerType: "Note"}
		require.NoError(t, ts.BeforeCreate())
		assert.Equal(t, "Tombstone", ts.Type)
		assert.Equal(t, "OBJECT#o1", ts.PK)
		assert.Equal(t, "TOMBSTONE", ts.SK)
		assert.True(t, ts.IsTombstone())
		assert.Equal(t, "o1", ts.GetOriginalID())
		assert.Equal(t, "alice", ts.GetDeletedBy())
		assert.Equal(t, "Note", ts.GetFormerType())
		assert.False(t, ts.ShouldCleanup())
	})
}

func TestOAuthClient(t *testing.T) {
	t.Run("UpdateKeys sets owner index pointers and descending timestamp index", func(t *testing.T) {
		created := time.Unix(1700000000, 123).UTC()
		c := &OAuthClient{ClientID: "c1", OwnerID: "alice", CreatedAt: created}
		require.NoError(t, c.UpdateKeys())
		assert.Equal(t, "OAUTH_CLIENT#c1", c.PK)
		assert.Equal(t, "CLIENT", c.SK)
		require.NotNil(t, c.GSI1PK)
		require.NotNil(t, c.GSI1SK)
		assert.Equal(t, "OWNER#alice", *c.GSI1PK)
		assert.Equal(t, "CLIENT#c1", *c.GSI1SK)
		assert.Equal(t, "OAUTH_CLIENTS", c.OAuthClientsPK)

		expectedDesc := math.MaxInt64 - created.UTC().UnixNano()
		assert.Equal(t, "CREATED_AT#"+fmt.Sprintf("%019d", expectedDesc)+"#CLIENT#c1", c.OAuthClientsSK)
	})

	t.Run("owner-less client clears pointers", func(t *testing.T) {
		c := &OAuthClient{ClientID: "c1", CreatedAt: time.Unix(1700000000, 0).UTC()}
		require.NoError(t, c.UpdateKeys())
		assert.Nil(t, c.GSI1PK)
		assert.Nil(t, c.GSI1SK)
	})

	t.Run("lifecycle hooks set timestamps", func(t *testing.T) {
		c := &OAuthClient{ClientID: "c1"}
		require.NoError(t, c.BeforeCreate())
		assert.False(t, c.CreatedAt.IsZero())
		assert.Equal(t, c.CreatedAt, c.UpdatedAt)

		before := c.UpdatedAt
		require.NoError(t, c.BeforeUpdate())
		assert.True(t, c.UpdatedAt.After(before) || c.UpdatedAt.Equal(before))

		desc := encodeDescendingTimestamp(time.Time{})
		assert.Greater(t, desc, int64(0))
		assert.Less(t, desc, int64(math.MaxInt64))
	})
}

func TestPublicationModels(t *testing.T) {
	t.Run("Publication requires ID and stamps timestamps", func(t *testing.T) {
		p := &Publication{}
		assert.ErrorContains(t, p.UpdateKeys(), "ID is required")

		p.ID = "pub1"
		require.NoError(t, p.UpdateKeys())
		assert.Equal(t, "PUBLICATION#pub1", p.PK)
		assert.Equal(t, "METADATA", p.SK)
		assert.False(t, p.CreatedAt.IsZero())
		assert.False(t, p.UpdatedAt.IsZero())
		assert.Equal(t, MainTableName, p.TableName())
	})

	t.Run("PublicationMember requires IDs and stamps timestamps", func(t *testing.T) {
		pm := &PublicationMember{}
		assert.ErrorContains(t, pm.UpdateKeys(), "PublicationID is required")
		pm.PublicationID = "pub1"
		assert.ErrorContains(t, pm.UpdateKeys(), "UserID is required")

		pm.UserID = "u1"
		require.NoError(t, pm.UpdateKeys())
		assert.Equal(t, "PUBLICATION#pub1#MEMBER", pm.PK)
		assert.Equal(t, "USER#u1", pm.SK)
		assert.Equal(t, "USER#u1#PUBLICATION", pm.GSI1PK)
		assert.Equal(t, "PUBLICATION#pub1", pm.GSI1SK)
		assert.False(t, pm.JoinedAt.IsZero())
		assert.Equal(t, MainTableName, pm.TableName())
	})
}

func TestThreadSync(t *testing.T) {
	t.Run("constructor and key updates", func(t *testing.T) {
		before := time.Now()
		ts := NewThreadSync("s1")
		after := time.Now()
		require.NoError(t, ts.UpdateKeys())
		assert.Equal(t, "THREAD_SYNC#s1", ts.PK)
		assert.Equal(t, SKMetadata, ts.SK)
		assert.Equal(t, "pending", ts.SyncStatus)
		assert.GreaterOrEqual(t, ts.TTL, before.Add(30*24*time.Hour).Unix()-2)
		assert.LessOrEqual(t, ts.TTL, after.Add(30*24*time.Hour).Unix()+2)
		assert.Equal(t, MainTableName, ts.TableName())
	})

	t.Run("state transitions and missing reply list management", func(t *testing.T) {
		ts := NewThreadSync("s1")

		ts.MarkSyncing()
		assert.Equal(t, "syncing", ts.SyncStatus)
		assert.False(t, ts.LastSyncAt.IsZero())

		ts.AddMissingReply("r1")
		ts.AddMissingReply("r1")
		assert.Equal(t, []string{"r1"}, ts.MissingReplies)
		ts.RemoveMissingReply("r1")
		assert.Empty(t, ts.MissingReplies)

		ts.MarkFailed("boom")
		assert.Equal(t, StatusFailed, ts.SyncStatus)
		assert.Equal(t, "boom", ts.LastErrorMessage)

		ts.MarkCompleted()
		assert.Equal(t, StatusCompleted, ts.SyncStatus)
		assert.Equal(t, "", ts.LastErrorMessage)
		assert.True(t, ts.IsRecentlyCompleted())

		ts.LastSyncAt = time.Now().Add(-1 * time.Hour)
		assert.False(t, ts.IsRecentlyCompleted())
	})
}

func TestCMSSlugIndex(t *testing.T) {
	t.Run("PK helpers and UpdateKeys validation", func(t *testing.T) {
		assert.Equal(t, "", CMSArticleSlugIndexPK(" "))
		assert.Equal(t, "", CMSCategorySlugIndexPK(""))
		assert.Equal(t, "", CMSPublicationSlugIndexPK("\n"))
		assert.Equal(t, cmsSlugIndexSK, CMSSlugIndexSK())

		i := &CMSSlugIndex{}
		assert.ErrorContains(t, i.UpdateKeys(), "PK is required")
		i.PK = CMSArticleSlugIndexPK("slug")
		assert.ErrorContains(t, i.UpdateKeys(), "slug is required")
		i.Slug = "slug"
		assert.ErrorContains(t, i.UpdateKeys(), "targetID is required")
		i.TargetID = "t1"
		require.NoError(t, i.UpdateKeys())
		assert.Equal(t, cmsSlugIndexSK, i.SK)
		assert.False(t, i.CreatedAt.IsZero())
		assert.False(t, i.UpdatedAt.IsZero())
		assert.Equal(t, MainTableName, i.TableName())
	})
}

func TestRecoveryModels(t *testing.T) {
	t.Run("Trustee keys", func(t *testing.T) {
		tr := &Trustee{Username: "alice", ActorID: "@bob@example.com"}
		require.NoError(t, tr.UpdateKeys())
		assert.Equal(t, "USER#alice", tr.PK)
		assert.Equal(t, "TRUSTEE#@bob@example.com", tr.SK)
		assert.Equal(t, MainTableName, tr.TableName())
	})

	t.Run("RecoveryRequest, RecoveryCode, and RecoveryToken keys and TTLs", func(t *testing.T) {
		init := time.Unix(1700000000, 0).UTC()
		exp := init.Add(1 * time.Hour)
		rr := &RecoveryRequest{ID: "r1", Username: "alice", InitiatedAt: init, ExpiresAt: exp}
		require.NoError(t, rr.UpdateKeys())
		assert.Equal(t, "RECOVERY#r1", rr.PK)
		assert.Equal(t, "REQUEST", rr.SK)
		assert.Equal(t, "USER#alice", rr.GSI1PK)
		assert.Contains(t, rr.GSI1SK, "RECOVERY#")
		assert.Equal(t, exp.Unix(), rr.TTL)

		rc := &RecoveryCode{Username: "alice", Position: 2}
		require.NoError(t, rc.UpdateKeys())
		assert.Equal(t, "USER#alice", rc.PK)
		assert.Equal(t, "RECOVERY_CODE#2", rc.SK)

		rt := &RecoveryToken{PK: "RECOVERY_TOKEN#x", CreatedAt: init}
		require.NoError(t, rt.UpdateKeys())
		assert.Equal(t, SKToken, rt.SK)
		assert.Equal(t, init.Add(24*time.Hour).Unix(), rt.TTL)
	})
}

func TestDraft_UpdateKeys(t *testing.T) {
	t.Run("requires AuthorID and ID", func(t *testing.T) {
		d := &Draft{}
		assert.ErrorContains(t, d.UpdateKeys(), "AuthorID is required")
		d.AuthorID = "alice"
		assert.ErrorContains(t, d.UpdateKeys(), "ID is required")
	})

	t.Run("builds keys for new draft and scheduled draft", func(t *testing.T) {
		updated := time.Unix(1700000000, 0).UTC()
		created := updated.Add(-1 * time.Hour)
		d := &Draft{AuthorID: "alice", ID: "d1", UpdatedAt: updated, CreatedAt: created}
		require.NoError(t, d.UpdateKeys())
		assert.Equal(t, "USER#alice#DRAFT", d.PK)
		assert.Equal(t, "ID#d1", d.SK)
		assert.Equal(t, "USER#alice#NEWDRAFT", d.GSI1PK)
		assert.Contains(t, d.GSI1SK, "TIME#")
		assert.Equal(t, "DRAFT#STATUS#draft", d.GSI4PK)
		assert.Contains(t, d.GSI4SK, "AUTHOR#alice#ID#d1")

		obj := "o1"
		scheduled := updated.Add(2 * time.Hour)
		d = &Draft{
			AuthorID:    "alice",
			ID:          "d1",
			ObjectID:    &obj,
			Status:      "scheduled",
			ScheduledAt: &scheduled,
			UpdatedAt:   updated,
			CreatedAt:   created,
		}
		require.NoError(t, d.UpdateKeys())
		assert.Equal(t, "OBJECT#o1#DRAFT", d.GSI1PK)
		assert.Equal(t, "DRAFT#STATUS#scheduled", d.GSI4PK)
		assert.Contains(t, d.GSI4SK, scheduled.UTC().Format(time.RFC3339Nano))
	})
}

func TestStreamingPreferences(t *testing.T) {
	t.Run("UpdateKeys validates and helpers set appropriate keys and TTLs", func(t *testing.T) {
		p := &StreamingPreferences{}
		assert.ErrorContains(t, p.UpdateKeys(), "username is required")

		updated := time.Unix(1700000000, 0).UTC()
		p = &StreamingPreferences{Username: "alice", UpdatedAt: updated}
		require.NoError(t, p.UpdateKeys())
		assert.Equal(t, "STREAMING_PREFS#alice", p.PK)
		assert.Equal(t, SKCurrent, p.SK)
		assert.Equal(t, "USER#alice", p.GSI1PK)
		assert.Contains(t, p.GSI1SK, updated.Format(time.RFC3339))

		p.Version = 2
		p.SetVersionedPreference()
		assert.Contains(t, p.SK, "VERSION#2#")
		assert.InDelta(t, time.Now().Add(30*24*time.Hour).Unix(), p.TTL, 2)

		p.SetDevicePreference("d1")
		assert.Equal(t, "DEVICE#d1", p.SK)
		assert.Equal(t, "DEVICE#d1", p.GSI2PK)
		assert.Equal(t, "STREAMING_PREFS#alice", p.GSI2SK)

		p.SetBackupPreference()
		assert.Contains(t, p.SK, "BACKUP#")
		assert.InDelta(t, time.Now().Add(90*24*time.Hour).Unix(), p.TTL, 2)
		assert.Equal(t, MainTableName, p.TableName())
		assert.Equal(t, p.PK, p.GetPK())
		assert.Equal(t, p.SK, p.GetSK())
	})

	t.Run("defaults builder populates sensible values", func(t *testing.T) {
		p := GetDefaultStreamingPreferences("alice")
		assert.Equal(t, "alice", p.Username)
		assert.Equal(t, "auto", p.DefaultQuality)
		assert.True(t, p.AutoQuality)
		assert.Equal(t, SKCurrent, p.SK)
	})
}

func TestImportExportCostTracking(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()

	t.Run("ImportCostTracking keys and cost math", func(t *testing.T) {
		i := &ImportCostTracking{
			ImportID:   "imp1",
			Username:   "alice",
			Timestamp:  ts,
			CreatedAt:  ts,
			UpdatedAt:  ts,
			Status:     StatusCompleted,
			FileSize:   10,
			RecordCount: 5,
		}
		i.UpdateKeys()
		compact := ts.Format(common.CompactTimeFormat)
		assert.Equal(t, "IMPORT_COST#imp1#"+compact, i.PK)
		assert.Equal(t, "COST#"+compact, i.SK)
		assert.Equal(t, "USER#alice", i.GSI1PK)
		assert.Contains(t, i.GSI1SK, "COST#")

		i.LambdaExecutionCost = 1
		i.S3StorageCost = 2
		i.S3GetRequestCost = 3
		i.S3DataTransferCost = 4
		i.DynamoDBWriteCost = 5
		i.DynamoDBReadCost = 6
		i.ExternalAPICallCost = 7
		i.CalculateTotalCost()
		assert.Equal(t, int64(28), i.TotalCostMicroCents)
		assert.InDelta(t, 0.000028, i.GetTotalCostDollars(), 0.0000001)
		assert.Equal(t, ts, i.GetTimestamp())
		assert.Equal(t, int64(28), i.GetTotalCostMicroCents())
		assert.Equal(t, 0.0, i.GetSuccessRate())

		i.ProcessedCount = 10
		i.SuccessCount = 7
		assert.InDelta(t, 0.7, i.GetSuccessRate(), 0.0001)

		before := time.Now()
		i2 := &ImportCostTracking{ImportID: "imp2", Username: "alice"}
		require.NoError(t, i2.BeforeCreate())
		after := time.Now()
		assert.GreaterOrEqual(t, i2.TTL, before.AddDate(0, 0, 90).Unix()-2)
		assert.LessOrEqual(t, i2.TTL, after.AddDate(0, 0, 90).Unix()+2)
		require.NoError(t, i2.BeforeUpdate())
		assert.Equal(t, MainTableName, i2.TableName())
	})

	t.Run("ExportCostTracking keys and cost math", func(t *testing.T) {
		e := &ExportCostTracking{
			ExportID:  "exp1",
			Username:  "alice",
			Timestamp: ts,
		}
		e.UpdateKeys()
		compact := ts.Format(common.CompactTimeFormat)
		assert.Equal(t, "EXPORT_COST#exp1#"+compact, e.PK)
		assert.Equal(t, "COST#"+compact, e.SK)
		assert.Equal(t, "USER#alice", e.GSI1PK)

		e.LambdaExecutionCost = 1
		e.S3StorageCost = 2
		e.S3PutRequestCost = 3
		e.S3GetRequestCost = 4
		e.S3DataTransferCost = 5
		e.DynamoDBReadCost = 6
		e.CalculateTotalCost()
		assert.Equal(t, int64(21), e.TotalCostMicroCents)
		assert.InDelta(t, 0.000021, e.GetTotalCostDollars(), 0.0000001)
		assert.Equal(t, ts, e.GetTimestamp())
		assert.Equal(t, int64(21), e.GetTotalCostMicroCents())

		before := time.Now()
		e2 := &ExportCostTracking{ExportID: "exp2", Username: "alice"}
		require.NoError(t, e2.BeforeCreate())
		after := time.Now()
		assert.GreaterOrEqual(t, e2.TTL, before.AddDate(0, 0, 90).Unix()-2)
		assert.LessOrEqual(t, e2.TTL, after.AddDate(0, 0, 90).Unix()+2)
		require.NoError(t, e2.BeforeUpdate())
		assert.Equal(t, MainTableName, e2.TableName())
		assert.Equal(t, MainTableName, (ExportCostSummary{}).TableName())
		assert.Equal(t, MainTableName, (ExportTypeCostStats{}).TableName())
	})
}
