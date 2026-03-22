package models

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashtagSearchResult(t *testing.T) {
	t.Run("basic helpers and history aggregation", func(t *testing.T) {
		h := NewHashtagSearchResult("golang", "https://example.com/tags/golang")
		assert.Equal(t, "golang", h.Name)
		assert.NotNil(t, h.History)
		assert.False(t, h.IsFollowing())

		h.SetFollowing(true)
		assert.True(t, h.IsFollowing())

		h.AddHistory(nil)
		assert.Empty(t, h.History)

		t1 := &TrendingHashtag{UseCount: 10, UpdatedAt: time.Unix(10, 0).UTC(), Date: "2025-01-01", Hashtag: "golang", Score: 1.0}
		t2 := &TrendingHashtag{UseCount: 20, UpdatedAt: time.Unix(20, 0).UTC(), Date: "2025-01-01", Hashtag: "golang", Score: 2.0}
		require.NoError(t, t1.UpdateKeys())
		require.NoError(t, t2.UpdateKeys())

		h.AddHistory(t1)
		h.AddHistory(t2)
		assert.Equal(t, int64(30), h.GetTotalUsage())
		assert.Equal(t, int64(20), h.GetLatestUsage())
		assert.False(t, h.HasRecentActivity())

		// Mark one entry as recent.
		t2.UpdatedAt = time.Now()
		assert.True(t, h.HasRecentActivity())
		assert.Equal(t, MainTableName, h.TableName())
	})

	t.Run("empty history returns zeroes", func(t *testing.T) {
		h := &HashtagSearchResult{}
		assert.Equal(t, int64(0), h.GetLatestUsage())
		assert.Equal(t, int64(0), h.GetTotalUsage())
	})
}

func TestCollectionItem(t *testing.T) {
	t.Run("keys, lifecycle and extract helpers", func(t *testing.T) {
		now := time.Unix(1700000000, 0).UTC()
		item := &CollectionItem{Collection: CollectionFeatured, ItemID: "s1", AddedAt: now}
		item.UpdateKeys()
		assert.Equal(t, "COLLECTION#featured", item.PK)
		assert.Equal(t, "ITEM#s1", item.SK)
		assert.Equal(t, "ITEM#s1", item.GSI1PK)
		assert.Equal(t, "COLLECTION#featured", item.GSI1SK)
		assert.Equal(t, MainTableName, item.TableName())
		assert.Equal(t, "featured", item.ExtractCollection())
		assert.Equal(t, "s1", item.ExtractItemID())

		item2 := NewCollectionItem(CollectionLikes, "s2", "Note", "alice")
		assert.Equal(t, "COLLECTION#likes", item2.PK)
		assert.Equal(t, "ITEM#s2", item2.SK)
		item2.SetPosition(5)
		assert.Equal(t, 5, item2.Position)

		ttl := time.Unix(1700001000, 0).UTC()
		item2.SetTTL(ttl)
		require.NotNil(t, item2.TTL)
		assert.Equal(t, ttl.Unix(), *item2.TTL)

		require.NoError(t, item2.BeforeCreate())
		assert.False(t, item2.CreatedAt.IsZero())
	})
}

func TestAuthAuditLog(t *testing.T) {
	t.Run("UpdateKeys sets PK/SK and optional GSIs", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		a := &AuthAuditLog{
			ID:        "e1",
			Timestamp: ts,
			Username:  "alice",
			IPAddress: "1.2.3.4",
			SessionID: "s1",
			Severity:  "high",
		}
		require.NoError(t, a.UpdateKeys())
		assert.Equal(t, "AUDIT#"+ts.Format("2006-01-02"), a.PK)
		assert.Contains(t, a.SK, "EVENT#e1#")
		assert.Equal(t, "USER#alice", a.GSI1PK)
		assert.Contains(t, a.GSI1SK, "AUDIT#")
		assert.Equal(t, "IP#1.2.3.4", a.GSI2PK)
		assert.Equal(t, "SESSION#s1", a.GSI3PK)
		assert.Equal(t, "SEVERITY#high", a.GSI4PK)
		assert.Equal(t, MainTableName, a.TableName())
		assert.Equal(t, a.PK, a.GetPK())
		assert.Equal(t, a.SK, a.GetSK())

		a.DataRetentionDays = 1
		require.NoError(t, a.UpdateKeys())
		assert.InDelta(t, time.Now().Add(24*time.Hour).Unix(), a.TTL, 2)

		require.NoError(t, a.BeforeSave())
	})

	t.Run("empty optional gsi sources stay unset and tags omit empty keys", func(t *testing.T) {
		a := &AuthAuditLog{
			ID:       "e2",
			Severity: "info",
		}

		require.NoError(t, a.UpdateKeys())
		assert.Empty(t, a.GSI1PK)
		assert.Empty(t, a.GSI1SK)
		assert.Empty(t, a.GSI2PK)
		assert.Empty(t, a.GSI2SK)
		assert.Empty(t, a.GSI3PK)
		assert.Empty(t, a.GSI3SK)
		assert.Equal(t, "SEVERITY#info", a.GSI4PK)
		assert.NotEmpty(t, a.GSI4SK)

		typ := reflect.TypeOf(AuthAuditLog{})
		for _, fieldName := range []string{"GSI1PK", "GSI1SK", "GSI2PK", "GSI2SK", "GSI3PK", "GSI3SK", "GSI4PK", "GSI4SK"} {
			field, ok := typ.FieldByName(fieldName)
			require.True(t, ok)
			assert.Contains(t, field.Tag.Get("theorydb"), "omitempty")
		}
	})
}

func TestInstanceRule(t *testing.T) {
	r := &InstanceRule{ID: "r1", Order: 2, Active: true, Severity: "warning"}
	r.UpdateKeys()
	assert.Equal(t, "INSTANCE#RULES", r.PK)
	assert.Equal(t, "RULE#002#r1", r.SK)
	assert.Equal(t, "INSTANCE#ACTIVE_RULES", r.GSI1PK)
	assert.Equal(t, "002#r1", r.GSI1SK)
	assert.Equal(t, MainTableName, r.TableName())

	r.Deactivate()
	assert.False(t, r.Active)
	assert.Nil(t, r.EnforcedAt)
	assert.Empty(t, r.GSI1PK)

	r.Activate()
	assert.True(t, r.Active)
	assert.NotNil(t, r.EnforcedAt)
	assert.Equal(t, "INSTANCE#ACTIVE_RULES", r.GSI1PK)

	r.SetOrder(10)
	assert.Equal(t, 10, r.Order)
	assert.Equal(t, "RULE#010#r1", r.SK)

	assert.Equal(t, 2, r.GetSeverityLevel())
	r.Severity = "critical"
	assert.Equal(t, 3, r.GetSeverityLevel())
	r.Severity = "info"
	assert.Equal(t, 1, r.GetSeverityLevel())
	r.Severity = "other"
	assert.Equal(t, 0, r.GetSeverityLevel())
}

func TestAccountFeatures(t *testing.T) {
	t.Run("AccountPin and AccountNote key generation", func(t *testing.T) {
		pin := &AccountPin{Username: "alice", PinnedActorID: "https://example.com/users/bob"}
		require.NoError(t, pin.BeforeCreate())
		assert.Equal(t, "ACCOUNT_PIN#alice", pin.PK)
		assert.Equal(t, "PIN#https://example.com/users/bob", pin.SK)
		assert.False(t, pin.CreatedAt.IsZero())
		assert.Equal(t, MainTableName, pin.TableName())
		assert.Equal(t, pin.PK, pin.GetPK())
		assert.Equal(t, pin.SK, pin.GetSK())

		note := &AccountNote{Username: "alice", TargetActorID: "bob"}
		require.NoError(t, note.BeforeCreate())
		assert.Equal(t, "ACCOUNT_NOTE#alice", note.PK)
		assert.Equal(t, "NOTE#bob", note.SK)
		assert.False(t, note.CreatedAt.IsZero())
		assert.Equal(t, note.CreatedAt, note.UpdatedAt)
		assert.Equal(t, MainTableName, note.TableName())
	})
}

func TestUserPreferencesModel(t *testing.T) {
	t.Run("keying and conversion helpers", func(t *testing.T) {
		up := &UserPreferences{Username: "alice", Language: "en"}
		up.UpdateKeys()
		assert.Equal(t, "USER#alice", up.PK)
		assert.Equal(t, "PREFERENCES", up.SK)
		assert.Equal(t, MainTableName, up.TableName())

		st := up.ToStorage()
		assert.Equal(t, "en", st.Language)

		up2 := &UserPreferences{}
		up2.FromStorage("alice", st)
		assert.Equal(t, "alice", up2.Username)
		assert.Equal(t, "USER#alice", up2.PK)
		assert.Equal(t, "PREFERENCES", up2.SK)
		assert.False(t, up2.UpdatedAt.IsZero())

		def := GetDefaultPreferences()
		assert.Equal(t, "en", def.Language)
		assert.NotNil(t, def.ReblogFilters)
		assert.Equal(t, MainTableName, (UserPreferencesStorage{}).TableName())
	})
}

func TestUserFeaturesModels(t *testing.T) {
	t.Run("UserPreference keys and lifecycle", func(t *testing.T) {
		p := &UserPreference{Username: "alice", Key: "theme"}
		require.NoError(t, p.BeforeCreate())
		assert.Equal(t, "USER#alice", p.PK)
		assert.Equal(t, "PREFERENCE#theme", p.SK)
		assert.False(t, p.UpdatedAt.IsZero())
		assert.Equal(t, MainTableName, p.TableName())
	})

	t.Run("FollowRequestState keys", func(t *testing.T) {
		f := &FollowRequestState{RequesterID: "alice", TargetID: "bob"}
		require.NoError(t, f.BeforeCreate())
		assert.Equal(t, "FOLLOW_REQUEST#alice", f.PK)
		assert.Equal(t, "TARGET#bob", f.SK)
		assert.False(t, f.CreatedAt.IsZero())
		assert.Equal(t, f.CreatedAt, f.UpdatedAt)
		assert.Equal(t, MainTableName, f.TableName())
	})

	t.Run("FieldVerification expiry", func(t *testing.T) {
		fv := &FieldVerification{Username: "alice", FieldName: "website", ExpiresAt: time.Now().Add(1 * time.Hour)}
		require.NoError(t, fv.BeforeCreate())
		assert.Equal(t, "USER#alice", fv.PK)
		assert.Equal(t, "FIELD_VERIFICATION#website", fv.SK)
		assert.False(t, fv.IsExpired())
		fv.ExpiresAt = time.Now().Add(-1 * time.Second)
		assert.True(t, fv.IsExpired())
		assert.Equal(t, MainTableName, fv.TableName())
	})
}

func TestRoutingMetrics(t *testing.T) {
	t.Run("windows update keys and set TTL", func(t *testing.T) {
		start := time.Unix(1700000000, 0).UTC()
		before := time.Now()

		r := &RouteMetricsWindow{RouteID: "r1", WindowStart: start}
		require.NoError(t, r.UpdateKeys())
		assert.Equal(t, "METRICS#ROUTE#r1", r.PK)
		assert.Equal(t, "WINDOW#"+fmt.Sprintf("%d", start.Unix()), r.SK)
		assert.GreaterOrEqual(t, r.TTL, before.Add(30*24*time.Hour).Unix()-2)

		g := &GlobalMetricsWindow{WindowStart: start}
		require.NoError(t, g.UpdateKeys())
		assert.Equal(t, "METRICS#GLOBAL#SUMMARY", g.PK)
		assert.Equal(t, "METRICS#GLOBAL", g.GSI1PK)

		i := &InstanceMetricsWindow{InstanceID: "example.com", WindowStart: start}
		require.NoError(t, i.UpdateKeys())
		assert.Equal(t, "METRICS#INSTANCE#example.com", i.PK)

		assert.Equal(t, MainTableName, r.TableName())
		assert.Equal(t, r.PK, r.GetPK())
		assert.Equal(t, r.SK, r.GetSK())
	})
}

func TestCMSSeriesSlugIndex(t *testing.T) {
	t.Run("PK helpers and UpdateKeys validation", func(t *testing.T) {
		assert.Equal(t, "", CMSSeriesSlugIndexPK(""))
		assert.Equal(t, "REF", CMSSeriesSlugIndexSK())

		i := &CMSSeriesSlugIndex{}
		assert.ErrorContains(t, i.UpdateKeys(), "slug is required")

		i.Slug = "s1"
		assert.ErrorContains(t, i.UpdateKeys(), "authorID is required")
		i.AuthorID = "a1"
		assert.ErrorContains(t, i.UpdateKeys(), "seriesID is required")
		i.SeriesID = "ser1"

		require.NoError(t, i.UpdateKeys())
		assert.Equal(t, "CMS#SERIES#SLUG#s1", i.PK)
		assert.Equal(t, "REF", i.SK)
		assert.False(t, i.CreatedAt.IsZero())
		assert.False(t, i.UpdatedAt.IsZero())
		assert.Equal(t, MainTableName, i.TableName())
	})
}

func TestPublicKeyCache(t *testing.T) {
	t.Run("constructor and keying", func(t *testing.T) {
		before := time.Now()
		p := NewPublicKeyCache("https://example.com/users/alice", "k1", "pem", "rsa-sha256")
		require.NotNil(t, p)
		assert.Equal(t, "PUBKEY_CACHE#https://example.com/users/alice", p.PK)
		assert.Equal(t, "KEY", p.SK)
		assert.Equal(t, []byte("pem"), p.GetPublicKeyPEM())
		assert.True(t, p.IsValid())
		assert.GreaterOrEqual(t, p.TTL, before.Add(24*time.Hour).Unix()-2)
		assert.Equal(t, MainTableName, p.TableName())
	})

	t.Run("success/failure counters, refresh logic and TTL extension", func(t *testing.T) {
		p := &PublicKeyCache{ActorURL: "x", TTL: time.Now().Add(10 * time.Second).Unix()}
		require.NoError(t, p.UpdateKeys())

		p.RecordSuccess()
		p.RecordFailure()
		assert.Equal(t, 1, p.SuccessCount)
		assert.Equal(t, 1, p.FailureCount)

		// 4 attempts, 3 failures -> failure rate 75% but <5 attempts so no refresh.
		p.SuccessCount = 1
		p.FailureCount = 3
		assert.False(t, p.ShouldRefresh())

		p.FailureCount = 4
		assert.True(t, p.ShouldRefresh())

		p.ExtendTTL(1 * time.Hour)
		assert.InDelta(t, time.Now().Add(1*time.Hour).Unix(), p.TTL, 2)
	})
}

func TestWebAuthnCredential(t *testing.T) {
	t.Run("lifecycle hooks and UpdateKeys build expected keys", func(t *testing.T) {
		w := &WebAuthnCredential{ID: "c1", UserID: "alice"}
		require.NoError(t, w.BeforeCreate())
		assert.Equal(t, "USER#alice", w.PK)
		assert.Equal(t, "WEBAUTHN_CRED#c1", w.SK)
		assert.Equal(t, "WEBAUTHN_CREDENTIAL#c1", w.GSI1PK)
		assert.Equal(t, "USER#alice", w.GSI1SK)
		assert.Equal(t, "WebAuthnCredential", w.Type)
		assert.False(t, w.CreatedAt.IsZero())
		assert.Equal(t, w.CreatedAt, w.LastUsedAt)
		assert.Equal(t, MainTableName, w.TableName())
		assert.Equal(t, w.PK, w.GetPK())
		assert.Equal(t, w.SK, w.GetSK())

		before := w.LastUsedAt
		require.NoError(t, w.BeforeUpdate())
		assert.True(t, w.LastUsedAt.After(before) || w.LastUsedAt.Equal(before))

		require.NoError(t, w.UpdateKeys())
		assert.Equal(t, "WEBAUTHN_CRED#c1", w.SK)
	})
}
