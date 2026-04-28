package models

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlag_KeysAndLifecycle(t *testing.T) {
	t.Run("UpdateKeys uses first object when present", func(t *testing.T) {
		f := &Flag{
			ID:        "flag-1",
			Actor:     "alice",
			Object:    []string{"status-1", "status-2"},
			Published: time.Date(2025, 1, 2, 3, 4, 5, 6, time.UTC),
			Status:    StatusPending,
		}

		f.UpdateKeys()

		assert.Equal(t, "FLAG#status-1", f.PK)
		assert.Equal(t, "TIME#2025-01-02T03:04:05.000000006Z#flag-1", f.SK)
		assert.Equal(t, "ACTOR#alice", f.GSI1PK)
		assert.Equal(t, "FLAG#2025-01-02T03:04:05.000000006Z", f.GSI1SK)
		assert.Equal(t, "FLAG_STATUS#pending", f.GSI2PK)
		assert.Equal(t, "TIME#2025-01-02T03:04:05.000000006Z", f.GSI2SK)
	})

	t.Run("UpdateKeys tolerates missing objects", func(t *testing.T) {
		f := &Flag{
			ID:        "flag-2",
			Actor:     "bob",
			Object:    nil,
			Published: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
			Status:    "reviewed",
		}
		f.UpdateKeys()
		assert.Equal(t, "FLAG#", f.PK)
		assert.Equal(t, "TIME#2025-01-02T03:04:05Z#flag-2", f.SK)
		assert.Equal(t, "FLAG_STATUS#reviewed", f.GSI2PK)
	})

	t.Run("BeforeCreate sets timestamps, defaults, and keys", func(t *testing.T) {
		f := &Flag{
			ID:     "flag-3",
			Actor:  "carol",
			Object: []string{"obj"},
		}
		before := time.Now()
		require.NoError(t, f.BeforeCreate())
		after := time.Now()

		assert.Equal(t, StatusPending, f.Status)
		assert.WithinDuration(t, before, f.CreatedAt, 2*time.Second)
		assert.WithinDuration(t, before, f.Published, 2*time.Second)
		assert.True(t, f.CreatedAt.Before(after.Add(2*time.Second)))
		assert.True(t, f.Published.Before(after.Add(2*time.Second)))
		assert.Equal(t, "FLAG#obj", f.PK)
		assert.NotEmpty(t, f.SK)
	})

	t.Run("BeforeSave updates keys", func(t *testing.T) {
		f := &Flag{
			ID:        "flag-4",
			Actor:     "dan",
			Object:    []string{"o1"},
			Published: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Status:    "resolved",
		}
		require.NoError(t, f.BeforeSave())
		assert.Equal(t, "FLAG#o1", f.PK)
		assert.Equal(t, "FLAG_STATUS#resolved", f.GSI2PK)
		assert.Equal(t, MainTableName, f.TableName())
	})
}

func TestHashtag_AndUsage(t *testing.T) {
	t.Run("UpdateKeys normalizes name and sets prefix index", func(t *testing.T) {
		h := &Hashtag{Name: "#Go"}
		require.NoError(t, h.UpdateKeys())
		assert.Equal(t, "HASHTAG#go", h.PK)
		assert.Equal(t, SKMetadata, h.SK)
		assert.Equal(t, "HASHTAG_SEARCH#go", h.GSI3PK)
		assert.Equal(t, "go", h.GSI3SK)
		assert.Equal(t, h.PK, h.GetPK())
		assert.Equal(t, h.SK, h.GetSK())
	})

	t.Run("getHashtagPrefix handles short names", func(t *testing.T) {
		assert.Equal(t, "", getHashtagPrefix(""))
		assert.Equal(t, "a", getHashtagPrefix("A"))
		assert.Equal(t, "ab", getHashtagPrefix("#Abc"))
	})

	t.Run("HashtagUsage can be keyed with explicit hashtag", func(t *testing.T) {
		usedAt := time.Unix(1700000000, 0).UTC()
		hu := &HashtagUsage{
			StatusID: "status-1",
			UsedAt:   usedAt,
		}

		hu.UpdateKeysWithHashtag("#Go")

		assert.Equal(t, "HASHTAG#go", hu.PK)
		assert.Equal(t, "USAGE#1700000000#status-1", hu.SK)
		assert.Equal(t, hu.PK, hu.GetPK())
		assert.Equal(t, hu.SK, hu.GetSK())
		require.NoError(t, hu.UpdateKeys())
		assert.Equal(t, MainTableName, hu.TableName())
	})
}

func TestAuthRefreshToken_KeysAndStatus(t *testing.T) {
	t.Run("UpdateKeys sets indexes and TTL from ExpiresAt", func(t *testing.T) {
		a := &AuthRefreshToken{
			UserID:    "u1",
			Family:    "fam1",
			Token:     "tok",
			CreatedAt: 1700000000,
			ExpiresAt: 1700001000,
		}
		require.NoError(t, a.UpdateKeys())
		assert.Equal(t, "tok", a.PK)
		assert.Equal(t, SKToken, a.SK)
		assert.Equal(t, "u1#fam1", a.UserFamily)
		assert.Equal(t, "USER#u1", a.GSI1PK)
		assert.Equal(t, "1700000000", a.GSI1SK)
		assert.Equal(t, "FAMILY#fam1", a.GSI2PK)
		assert.Equal(t, "USER_FAMILY#u1#fam1", a.GSI3PK)
		assert.Equal(t, int64(1700001000), a.TTL)
		assert.Equal(t, MainTableName, a.TableName())
		assert.Equal(t, a.PK, a.GetPK())
		assert.Equal(t, a.SK, a.GetSK())
	})

	t.Run("UpdateKeys does not override existing TTL", func(t *testing.T) {
		a := &AuthRefreshToken{
			UserID:    "u1",
			Family:    "fam1",
			Token:     "tok",
			CreatedAt: 1700000000,
			ExpiresAt: 1700001000,
			TTL:       42,
		}
		require.NoError(t, a.UpdateKeys())
		assert.Equal(t, int64(42), a.TTL)
	})

	t.Run("BeforeCreate/BeforeUpdate call UpdateKeys", func(t *testing.T) {
		a := &AuthRefreshToken{UserID: "u1", Family: "f", Token: "tok", CreatedAt: 1}
		require.NoError(t, a.BeforeCreate())
		assert.Equal(t, "tok", a.PK)
		require.NoError(t, a.BeforeUpdate())
		assert.Equal(t, "tok", a.PK)
	})

	t.Run("IsExpired and IsActive reflect revocation and expiry", func(t *testing.T) {
		a := &AuthRefreshToken{ExpiresAt: time.Now().Add(-time.Second).Unix()}
		assert.True(t, a.IsExpired())

		a = &AuthRefreshToken{ExpiresAt: time.Now().Add(time.Minute).Unix()}
		assert.False(t, a.IsExpired())
		assert.True(t, a.IsActive())

		a.Revoked = true
		assert.False(t, a.IsActive())
	})
}

func TestQueryCacheEntry_AndBatchGetKeys(t *testing.T) {
	t.Run("QueryCacheEntry UpdateKeys sets TTL and timestamps", func(t *testing.T) {
		expires := time.Now().Add(10 * time.Minute)
		q := &QueryCacheEntry{
			CacheKey:  "k1",
			ExpiresAt: expires,
		}
		before := time.Now()
		require.NoError(t, q.UpdateKeys())
		after := time.Now()

		assert.Equal(t, "CACHE#k1", q.PK)
		assert.Equal(t, "KEY#k1", q.SK)
		assert.Equal(t, expires.Unix(), q.TTL)
		assert.WithinDuration(t, before, q.UpdatedAt, 2*time.Second)
		assert.WithinDuration(t, before, q.CreatedAt, 2*time.Second)
		assert.True(t, q.CreatedAt.Before(after.Add(2*time.Second)))
		assert.Equal(t, q.PK, q.GetPK())
		assert.Equal(t, q.SK, q.GetSK())
		assert.Equal(t, MainTableName, q.TableName())
	})

	t.Run("QueryCacheEntry IsExpired reflects ExpiresAt", func(t *testing.T) {
		q := &QueryCacheEntry{ExpiresAt: time.Now().Add(-time.Second)}
		assert.True(t, q.IsExpired())
		q.ExpiresAt = time.Now().Add(time.Second)
		assert.False(t, q.IsExpired())
	})

	t.Run("BatchGetKeys UpdateKeys sets short TTL and CreatedAt", func(t *testing.T) {
		b := &BatchGetKeys{BatchType: "instance", Key: "key1"}
		before := time.Now()
		require.NoError(t, b.UpdateKeys())
		after := time.Now()

		assert.Equal(t, "BATCH#instance", b.PK)
		assert.Equal(t, "KEY#key1", b.SK)
		assert.InDelta(t, before.Add(time.Minute).Unix(), b.TTL, 2)
		assert.WithinDuration(t, before, b.CreatedAt, 2*time.Second)
		assert.True(t, b.CreatedAt.Before(after.Add(2*time.Second)))
		assert.Equal(t, b.PK, b.GetPK())
		assert.Equal(t, b.SK, b.GetSK())
		assert.Equal(t, MainTableName, b.TableName())
	})
}

func TestNotificationLegacy_KeysAndConstructor(t *testing.T) {
	t.Run("UpdateKeys validates required fields", func(t *testing.T) {
		n := &NotificationLegacy{}
		assert.ErrorContains(t, n.UpdateKeys(), "username is required")

		n.Username = "alice"
		assert.ErrorContains(t, n.UpdateKeys(), "id is required")

		n.ID = "id1"
		assert.ErrorContains(t, n.UpdateKeys(), "created at is required")

		n.CreatedAt = 1700000000
		require.NoError(t, n.UpdateKeys())
		assert.Equal(t, "NOTIFICATIONS#alice", n.PK)
		assert.Equal(t, "1700000000#id1", n.SK)
	})

	t.Run("SetPrimaryKey and SetSortKey use legacy patterns", func(t *testing.T) {
		n := &NotificationLegacy{}
		n.SetPrimaryKey("bob")
		n.SetSortKey(123, "nid")
		assert.Equal(t, "NOTIFICATIONS#bob", n.PK)
		assert.Equal(t, "123#nid", n.SK)
	})

	t.Run("NewNotificationLegacy initializes keys and TTL", func(t *testing.T) {
		before := time.Now()
		n := NewNotificationLegacy("alice", "mention", "acct1")
		after := time.Now()

		require.NotEmpty(t, n.ID)
		assert.Equal(t, "mention", n.Type)
		assert.Equal(t, "alice", n.Username)
		assert.Equal(t, "acct1", n.AccountID)
		assert.False(t, n.Read)
		assert.Equal(t, "NOTIFICATIONS#alice", n.PK)
		assert.Contains(t, n.SK, "#"+n.ID)

		ttlTime := time.Unix(n.TTL, 0)
		assert.True(t, ttlTime.After(before.Add(30*24*time.Hour).Add(-5*time.Second)))
		assert.True(t, ttlTime.Before(after.Add(30*24*time.Hour).Add(5*time.Second)))
		assert.Equal(t, MainTableName, n.TableName())
	})
}

func TestThreatIntel_Keys(t *testing.T) {
	t.Run("ThreatIntel UpdateKeys sets legacy patterns", func(t *testing.T) {
		ti := &ThreatIntel{
			ID:         "t1",
			ThreatType: "spam",
			LastSeen:   time.Unix(1700000000, 0).UTC(),
		}
		require.NoError(t, ti.UpdateKeys())
		assert.Equal(t, "THREAT#t1", ti.PK)
		assert.Equal(t, SKMetadata, ti.SK)
		assert.Equal(t, "TYPE#spam", ti.GSI1PK)
		assert.Equal(t, "THREAT#t1", ti.GSI1SK)
		assert.Equal(t, "THREATS", ti.GSI2PK)
		assert.Equal(t, "1700000000#t1", ti.GSI2SK)
		assert.Equal(t, ti.PK, ti.GetPK())
		assert.Equal(t, ti.SK, ti.GetSK())
		assert.Equal(t, MainTableName, ti.TableName())
	})

	t.Run("ThreatIndicator UpdateKeys sets TTL and keys", func(t *testing.T) {
		ti := &ThreatIndicator{}
		before := time.Now()
		require.NoError(t, ti.UpdateKeys("indicator", "t1"))
		after := time.Now()

		assert.Equal(t, "INDICATOR#indicator", ti.PK)
		assert.Equal(t, "THREAT#t1", ti.SK)
		assert.Equal(t, "t1", ti.ThreatID)
		assert.InDelta(t, before.Add(30*24*time.Hour).Unix(), ti.TTL, 5)
		assert.True(t, time.Unix(ti.TTL, 0).Before(after.Add(30*24*time.Hour).Add(5*time.Second)))
		assert.Equal(t, ti.PK, ti.GetPK())
		assert.Equal(t, ti.SK, ti.GetSK())
		assert.Equal(t, MainTableName, ti.TableName())
	})
}

func TestAnalyticsModels(t *testing.T) {
	t.Run("StatusEngagement UpdateKeys", func(t *testing.T) {
		engagedAt := time.Date(2025, 1, 1, 2, 3, 4, 0, time.UTC)
		s := &StatusEngagement{
			StatusID:       "s1",
			EngagementType: "like",
			UserID:         "u1",
			EngagedAt:      engagedAt,
		}
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "STATUS_ENGAGEMENT#s1", s.PK)
		assert.Equal(t, "like#2025-01-01T02:03:04Z#u1", s.SK)
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())
		assert.Equal(t, MainTableName, s.TableName())
	})

	t.Run("LinkShare UpdateKeys", func(t *testing.T) {
		l := &LinkShare{URL: "https://example.com", StatusID: "s1"}
		require.NoError(t, l.UpdateKeys())
		assert.Equal(t, "LINK_SHARE#https://example.com", l.PK)
		assert.Equal(t, "STATUS#s1", l.SK)
		assert.Equal(t, l.PK, l.GetPK())
		assert.Equal(t, l.SK, l.GetSK())
		assert.Equal(t, MainTableName, l.TableName())
	})

	t.Run("EngagementMetrics UpdateKeys mirrors PK/SK onto GSI8", func(t *testing.T) {
		e := &EngagementMetrics{PK: "METRICS#views#2025-01-01", SK: "target#u1"}
		require.NoError(t, e.UpdateKeys())
		assert.Equal(t, e.PK, e.GSI8PK)
		assert.Equal(t, e.SK, e.GSI8SK)
		assert.Equal(t, e.PK, e.GetPK())
		assert.Equal(t, e.SK, e.GetSK())
		assert.Equal(t, MainTableName, e.TableName())
	})
}

func TestPasswordReset_KeysAndValidation(t *testing.T) {
	t.Run("BeforeCreate sets keys and TTL from ExpiresAt", func(t *testing.T) {
		expiresAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		r := &PasswordReset{
			Username:  "alice",
			Token:     "tok",
			ExpiresAt: expiresAt,
		}
		require.NoError(t, r.BeforeCreate())
		assert.Equal(t, "USER#alice", r.PK)
		assert.Equal(t, "RESET#tok", r.SK)
		assert.Equal(t, "RESET_TOKEN#tok", r.GSI1PK)
		assert.Equal(t, "USERNAME#alice", r.GSI1SK)
		assert.Equal(t, expiresAt.Add(24*time.Hour).Unix(), r.TTL)
		assert.Equal(t, r.PK, r.GetPK())
		assert.Equal(t, r.SK, r.GetSK())
		assert.Equal(t, MainTableName, r.TableName())
	})

	t.Run("UpdateKeys validates required fields", func(t *testing.T) {
		r := &PasswordReset{}
		assert.ErrorContains(t, r.UpdateKeys(), "username is required")
		r.Username = "alice"
		assert.ErrorContains(t, r.UpdateKeys(), "token is required")
		r.Token = "tok"
		require.NoError(t, r.UpdateKeys())
		assert.Equal(t, "USER#alice", r.PK)
		assert.Equal(t, "RESET#tok", r.SK)
	})
}

func TestDevice_KeysTrustAndActivity(t *testing.T) {
	lastSeen := time.Unix(1700000000, 0).UTC()
	d := &Device{
		DeviceID:   "dev1",
		Username:   "alice",
		LastSeenAt: lastSeen,
		TrustLevel: "trusted",
		Active:     true,
	}

	t.Run("UpdateKeys sets PK/SK, GSIs, and TTL", func(t *testing.T) {
		d.UpdateKeys()
		assert.Equal(t, "USER#alice", d.PK)
		assert.Equal(t, "DEVICE#dev1", d.SK)
		assert.Equal(t, "USER#alice#DEVICES", d.GSI1PK)
		assert.Equal(t, "1700000000#dev1", d.GSI1SK)
		assert.Equal(t, "TRUST_LEVEL#trusted", d.GSI2PK)
		assert.Equal(t, "1700000000#dev1", d.GSI2SK)
		assert.Equal(t, lastSeen.Add(90*24*time.Hour).Unix(), d.TTL)
		assert.Equal(t, MainTableName, d.TableName())
	})

	t.Run("UpdateLastSeen updates timestamp and derived keys", func(t *testing.T) {
		before := time.Now()
		d.UpdateLastSeen("127.0.0.1", "ua")
		after := time.Now()

		assert.WithinDuration(t, before, d.LastSeenAt, 2*time.Second)
		assert.True(t, d.LastSeenAt.Before(after.Add(2*time.Second)))
		assert.Equal(t, "127.0.0.1", d.LastIPAddress)
		assert.Equal(t, "ua", d.LastUserAgent)
		assert.Equal(t, "USER#alice#DEVICES", d.GSI1PK)
	})

	t.Run("SetTrustLevel only accepts supported values", func(t *testing.T) {
		d.TrustLevel = "trusted"
		d.SetTrustLevel("suspicious")
		assert.Equal(t, "suspicious", d.TrustLevel)

		d.SetTrustLevel("bogus")
		assert.Equal(t, "suspicious", d.TrustLevel)
	})

	t.Run("IsActive checks both Active flag and time window", func(t *testing.T) {
		now := time.Now()
		d.Active = true
		d.LastSeenAt = now.Add(-10 * 24 * time.Hour)
		assert.True(t, d.IsActive())

		d.LastSeenAt = now.Add(-40 * 24 * time.Hour)
		assert.False(t, d.IsActive())

		d.Active = false
		d.LastSeenAt = now.Add(-10 * 24 * time.Hour)
		assert.False(t, d.IsActive())
	})
}

func TestUserAppConsent_ScopesAndRevocation(t *testing.T) {
	c := &UserAppConsent{
		UserID:    "u1",
		AppID:     "app1",
		Scopes:    []string{"read", "write"},
		Active:    true,
		RevokedAt: nil,
	}

	require.NoError(t, c.UpdateKeys())
	assert.Equal(t, "USER#u1", c.PK)
	assert.Equal(t, "CONSENT#app1", c.SK)
	assert.Equal(t, "APP#app1", c.GSI1PK)
	assert.Equal(t, "USER#u1", c.GSI1SK)
	assert.True(t, c.HasScope("read"))
	assert.False(t, c.HasScope("admin"))
	assert.True(t, c.IsValid())

	c.Revoke()
	assert.False(t, c.Active)
	require.NotNil(t, c.RevokedAt)
	assert.False(t, c.IsValid())

	c.Resource = "https://example.com/mcp/agent-1"
	require.NoError(t, c.UpdateKeys())
	assert.Equal(t, "CONSENT#app1#RESOURCE#https://example.com/mcp/agent-1", c.SK)
	assert.Equal(t, "USER#u1#RESOURCE#https://example.com/mcp/agent-1", c.GSI1SK)
	assert.Equal(t, MainTableName, c.TableName())
}

func TestWebSocketSubscriptionModels(t *testing.T) {
	t.Run("WebSocketEventConnection UpdateKeys sets optional GSI", func(t *testing.T) {
		c := &WebSocketEventConnection{ConnectionID: "c1", UserID: "u1", Role: "moderator"}
		require.NoError(t, c.UpdateKeys())
		assert.Equal(t, "CONNECTION#c1", c.PK)
		assert.Equal(t, SKMetadata, c.SK)
		assert.Equal(t, "USER#u1", c.GSI1PK)
		assert.Equal(t, "CONNECTION#c1", c.GSI1SK)
		assert.Equal(t, "moderator", c.Role)
		assert.Equal(t, c.PK, c.GetPK())
		assert.Equal(t, c.SK, c.GetSK())
		assert.Equal(t, MainTableName, c.TableName())

		c2 := &WebSocketEventConnection{ConnectionID: "c2"}
		require.NoError(t, c2.UpdateKeys())
		assert.Equal(t, "CONNECTION#c2", c2.PK)
		assert.Equal(t, "", c2.GSI1PK)
	})

	t.Run("WebSocketEventSubscription UpdateKeys sets subscription index", func(t *testing.T) {
		s := &WebSocketEventSubscription{ConnectionID: "c1", SubscriptionType: "status.created"}
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "CONNECTION#c1", s.PK)
		assert.Equal(t, "SUBSCRIPTION#status.created", s.SK)
		assert.Equal(t, "SUBSCRIPTION#status.created", s.GSI1PK)
		assert.Equal(t, "CONNECTION#c1", s.GSI1SK)
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())
		assert.Equal(t, MainTableName, s.TableName())
	})
}

func TestExportModels_DateRangeAndKeys(t *testing.T) {
	t.Run("NewExportDateRangeFromStrings returns nil on missing values", func(t *testing.T) {
		dr, err := NewExportDateRangeFromStrings("", "2025-01-01")
		require.NoError(t, err)
		assert.Nil(t, dr)

		dr, err = NewExportDateRangeFromStrings("2025-01-01", "")
		require.NoError(t, err)
		assert.Nil(t, dr)
	})

	t.Run("NewExportDateRangeFromStrings wraps invalid format errors", func(t *testing.T) {
		_, err := NewExportDateRangeFromStrings("not-a-date", "2025-01-01")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrExportInvalidStartDate))

		_, err = NewExportDateRangeFromStrings("2025-01-01", "bad")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrExportInvalidEndDate))
	})

	t.Run("NewExportDateRangeFromStrings parses dates", func(t *testing.T) {
		dr, err := NewExportDateRangeFromStrings("2025-01-01", "2025-01-31")
		require.NoError(t, err)
		require.NotNil(t, dr)
		assert.Equal(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), dr.Start)
		assert.Equal(t, time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC), dr.End)
		assert.Equal(t, MainTableName, dr.TableName())
	})

	t.Run("Export UpdateKeys sets primary and user index keys", func(t *testing.T) {
		createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		e := &Export{ID: "exp1", Username: "alice", CreatedAt: createdAt, Status: "pending"}
		e.UpdateKeys()
		assert.Equal(t, "EXPORT#exp1", e.PK)
		assert.Equal(t, "EXPORT#exp1", e.SK)
		assert.Equal(t, "USER#alice", e.GSI1PK)
		assert.Equal(t, "CREATED#2025-01-01T00:00:00Z", e.GSI1SK)
		assert.Equal(t, "pending", e.GetStatus())
		assert.Equal(t, createdAt, e.GetCreatedAt())
		assert.Equal(t, MainTableName, e.TableName())
	})
}

func TestCategory_Keys(t *testing.T) {
	t.Run("UpdateKeys validates required ID", func(t *testing.T) {
		c := &Category{}
		assert.ErrorContains(t, c.UpdateKeys(), "ID is required")
	})

	t.Run("UpdateKeys sets keys and parent index", func(t *testing.T) {
		parent := "p1"
		c := &Category{
			ID:       "c1",
			ParentID: &parent,
		}
		before := time.Now()
		require.NoError(t, c.UpdateKeys())
		after := time.Now()

		assert.Equal(t, "INSTANCE#CATEGORY", c.PK)
		assert.Equal(t, "ID#c1", c.SK)
		assert.Equal(t, "CATEGORY#p1", c.GSI1PK)
		assert.Equal(t, "ID#c1", c.GSI1SK)
		assert.WithinDuration(t, before, c.CreatedAt, 2*time.Second)
		assert.WithinDuration(t, before, c.UpdatedAt, 2*time.Second)
		assert.True(t, c.UpdatedAt.Before(after.Add(2*time.Second)))
		assert.Equal(t, MainTableName, c.TableName())
		assert.Equal(t, c.PK, c.GetPK())
		assert.Equal(t, c.SK, c.GetSK())

		c2 := &Category{ID: "c2"}
		require.NoError(t, c2.UpdateKeys())
		assert.Equal(t, "CATEGORY#ROOT", c2.GSI1PK)
	})
}

func TestMediaSession_QualityChange_CloudWatchMetrics(t *testing.T) {
	t.Run("MediaSession UpdateKeys and SetTTL", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 1, 2, 3, 0, time.UTC)
		m := &MediaSession{
			SessionID: "s1",
			UserID:    "u1",
			StartTime: start,
		}
		require.NoError(t, m.UpdateKeys())
		assert.Equal(t, "SESSION#s1", m.PK)
		assert.Equal(t, SKMetadata, m.SK)
		assert.Equal(t, "USER#u1", m.GSI1PK)
		assert.Equal(t, "SESSION#2025-01-01T01:02:03Z", m.GSI1SK)

		before := time.Now()
		m.SetTTL(1 * time.Hour)
		assert.InDelta(t, before.Add(time.Hour).Unix(), m.TTL, 2)
		assert.Equal(t, MainTableName, m.TableName())
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())
	})

	t.Run("QualityChange UpdateKeys sets TTL", func(t *testing.T) {
		q := &QualityChange{SessionID: "s1", Timestamp: time.Unix(1700000000, 42).UTC()}
		before := time.Now()
		require.NoError(t, q.UpdateKeys())
		assert.Equal(t, "QUALITY#s1", q.PK)
		assert.Equal(t, "1700000000000000042", q.SK)
		assert.InDelta(t, before.Add(7*24*time.Hour).Unix(), q.TTL, 5)
		assert.Equal(t, MainTableName, q.TableName())
		assert.Equal(t, q.PK, q.GetPK())
		assert.Equal(t, q.SK, q.GetSK())
	})

	t.Run("CloudWatchMetrics UpdateKeys validates service name and sets derived fields", func(t *testing.T) {
		c := &CloudWatchMetrics{}
		assert.ErrorIs(t, c.UpdateKeys(), ErrCloudWatchMetricServiceNameRequired)

		c.ServiceName = "api"
		c.Timestamp = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, c.UpdateKeys())
		assert.Equal(t, "2025-01-01", c.Date)
		assert.Equal(t, "SERVICE#api", c.PK)
		assert.Equal(t, "METRICS#2025-01-01T00:00:00Z", c.SK)
		assert.Equal(t, "METRIC_DATE#2025-01-01", c.GSI1PK)
		assert.Equal(t, "api#2025-01-01T00:00:00Z", c.GSI1SK)
		assert.True(t, c.TTL > 0)

		c.CacheExpiry = time.Now().Add(-time.Second)
		assert.True(t, c.IsExpired())
		c.CacheExpiry = time.Now().Add(time.Second)
		assert.False(t, c.IsExpired())

		c.SetCacheExpiry()
		assert.True(t, c.CacheExpiry.After(time.Now()))
		assert.False(t, c.CloudWatchQueryTime.IsZero())
		assert.Equal(t, MainTableName, c.TableName())
		assert.Equal(t, c.PK, c.GetPK())
		assert.Equal(t, c.SK, c.GetSK())
	})
}

func TestPollVote_Like_RouteOptimizer_FederationStats(t *testing.T) {
	t.Run("Poll UpdateKeys sets TTL only when ExpiresAt provided", func(t *testing.T) {
		p := &Poll{ID: "p1", StatusID: "s1"}
		require.NoError(t, p.UpdateKeys())
		assert.Equal(t, "POLL#p1", p.PK)
		assert.Equal(t, SKMetadata, p.SK)
		assert.Equal(t, "STATUS#s1", p.GSI1PK)
		assert.Equal(t, int64(0), p.TTL)

		p.ExpiresAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, p.UpdateKeys())
		assert.Equal(t, p.ExpiresAt.Add(24*time.Hour).Unix(), p.TTL)
		assert.Equal(t, MainTableName, p.TableName())
		assert.Equal(t, p.PK, p.GetPK())
		assert.Equal(t, p.SK, p.GetSK())
	})

	t.Run("PollVote SetPollID sets PK/SK", func(t *testing.T) {
		v := &PollVote{VoterID: "v1"}
		v.SetPollID("p1")
		assert.Equal(t, "POLL#p1", v.PK)
		assert.Equal(t, "VOTE#v1", v.SK)
		require.NoError(t, v.UpdateKeys())
		assert.Equal(t, MainTableName, v.TableName())
		assert.Equal(t, v.PK, v.GetPK())
		assert.Equal(t, v.SK, v.GetSK())
	})

	t.Run("Like constructor and keying", func(t *testing.T) {
		like := NewLike("https://example.com/users/alice", "status-1", "bob")
		assert.True(t, strings.HasPrefix(like.ID, "https://example.com/users/alice/activities/like-"))
		assert.Equal(t, "object#status-1#likes", like.PK)
		assert.Equal(t, "actor#https://example.com/users/alice", like.SK)
		assert.Equal(t, "actor#https://example.com/users/alice#likes", like.GSI1PK)
		assert.Equal(t, like.Actor, like.GetUserID())
		assert.Equal(t, like.Object, like.GetStatusID())
		assert.Equal(t, "bob", like.GetStatusAuthorID())
		assert.Equal(t, MainTableName, like.TableName())
		assert.Equal(t, like.PK, like.GetPK())
		assert.Equal(t, like.SK, like.GetSK())
	})

	t.Run("Route optimizer models UpdateKeys set TTL and keys", func(t *testing.T) {
		r := &RouteDeliveryResult{RouteID: "r1", Timestamp: time.Unix(1700000000, 42).UTC()}
		before := time.Now()
		require.NoError(t, r.UpdateKeys())
		assert.Equal(t, "ROUTE#r1", r.PK)
		assert.Contains(t, r.SK, "RESULT#")
		assert.Equal(t, "RESULTS", r.GSI1PK)
		assert.Equal(t, "1700000000#r1", r.GSI1SK)
		assert.InDelta(t, before.Add(30*24*time.Hour).Unix(), r.TTL, 5)
		assert.Equal(t, MainTableName, r.TableName())
		assert.Equal(t, r.PK, r.GetPK())
		assert.Equal(t, r.SK, r.GetSK())

		o := &OptimizationDecision{Timestamp: time.Unix(1700000000, 42).UTC()}
		before = time.Now()
		require.NoError(t, o.UpdateKeys())
		assert.Equal(t, "OPTIMIZATION", o.PK)
		assert.Contains(t, o.SK, "DECISION#")
		assert.InDelta(t, before.Add(7*24*time.Hour).Unix(), o.TTL, 5)
		assert.Equal(t, MainTableName, o.TableName())
		assert.Equal(t, o.PK, o.GetPK())
		assert.Equal(t, o.SK, o.GetSK())
	})

	t.Run("FederationStats helpers and keying", func(t *testing.T) {
		stats := NewFederationStats("2025-01-01")
		assert.Equal(t, FederationStatsPK, stats.PK)
		assert.Equal(t, "2025-01-01", stats.SK)
		assert.Equal(t, time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC).Unix(), stats.TTL)

		pk, sk := GetFederationStatsKey("2025-01-01")
		assert.Equal(t, "FEDERATION#STATS", pk)
		assert.Equal(t, "2025-01-01", sk)
		pk, start, end := GetFederationStatsRangeKeys("2025-01-01", "2025-02-01")
		assert.Equal(t, "FEDERATION#STATS", pk)
		assert.Equal(t, "2025-01-01", start)
		assert.Equal(t, "2025-02-01", end)

		stats.IncrementStats(1, 2, 3)
		assert.Equal(t, 1, stats.ActiveInstances)
		assert.Equal(t, int64(2), stats.TotalMessages)
		assert.Equal(t, 3, stats.TotalUsers)
		assert.Equal(t, MainTableName, stats.TableName())
		assert.Equal(t, "2025-01-02", FormatStatsDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)))
	})
}

func TestRelay_Series_UserLogin_Report_WebAuthn(t *testing.T) {
	t.Run("Relay UpdateKeys sets active and domain indexes", func(t *testing.T) {
		r := &Relay{URL: "https://relay.example/inbox", Active: true, Domain: "example"}
		require.NoError(t, r.UpdateKeys())
		assert.Equal(t, "RELAY#https://relay.example/inbox", r.PK)
		assert.Equal(t, SKInfo, r.SK)
		assert.Equal(t, "ACTIVE_RELAYS", r.GSI1PK)
		assert.Equal(t, r.URL, r.GSI1SK)
		assert.Equal(t, "RELAY_DOMAIN#example", r.GSI2PK)
		assert.Equal(t, r.URL, r.GSI2SK)
		assert.Equal(t, MainTableName, r.TableName())
		assert.Equal(t, r.PK, r.GetPK())
		assert.Equal(t, r.SK, r.GetSK())

		r.Active = false
		require.NoError(t, r.UpdateKeys())
		assert.Equal(t, "", r.GSI1PK)
		assert.Equal(t, "", r.GSI1SK)
	})

	t.Run("Series UpdateKeys validates required fields and sets timestamps", func(t *testing.T) {
		s := &Series{}
		assert.ErrorContains(t, s.UpdateKeys(), "AuthorID is required")

		s.AuthorID = "a1"
		assert.ErrorContains(t, s.UpdateKeys(), "ID is required")

		s.ID = "s1"
		before := time.Now()
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "AUTHOR#a1#SERIES", s.PK)
		assert.Equal(t, "ID#s1", s.SK)
		assert.WithinDuration(t, before, s.CreatedAt, 2*time.Second)
		assert.WithinDuration(t, before, s.UpdatedAt, 2*time.Second)
		assert.Equal(t, MainTableName, s.TableName())
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())
	})

	t.Run("UserLogin validates and sets TTL", func(t *testing.T) {
		l := &UserLogin{}
		assert.ErrorContains(t, l.UpdateKeys(), "username is required")
		l.Username = "alice"
		assert.ErrorContains(t, l.UpdateKeys(), "timestamp is required")

		l.Timestamp = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, l.UpdateKeys())
		assert.Equal(t, "USER#alice", l.PK)
		assert.Equal(t, "LOGIN#2025-01-01T00:00:00Z", l.SK)

		before := time.Now()
		require.NoError(t, l.BeforeCreate())
		assert.InDelta(t, before.Add(90*24*time.Hour).Unix(), l.TTL, 5)
		assert.Equal(t, MainTableName, l.TableName())
		assert.Equal(t, l.PK, l.GetPK())
		assert.Equal(t, l.SK, l.GetSK())
	})

	t.Run("Report UpdateKeys sets TTL only when missing", func(t *testing.T) {
		createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		r := &Report{
			ID:              "r1",
			ReporterID:      "alice",
			TargetAccountID: "bob",
			Status:          "pending",
			CreatedAt:       createdAt,
			TTL:             0,
		}
		r.UpdateKeys()
		assert.Equal(t, "REPORT#r1", r.PK)
		assert.Equal(t, "REPORT", r.SK)
		assert.Equal(t, "USER#alice", r.GSI1PK)
		assert.Equal(t, "REPORTED#bob", r.GSI2PK)
		assert.Equal(t, "STATUS#pending", r.GSI3PK)
		assert.Equal(t, createdAt.Add(90*24*time.Hour).Unix(), r.TTL)
		assert.Equal(t, MainTableName, r.TableName())

		r2 := &Report{ID: "r2", ReporterID: "alice", TargetAccountID: "bob", Status: "pending", CreatedAt: createdAt, TTL: 123}
		r2.UpdateKeys()
		assert.Equal(t, int64(123), r2.TTL)

		rs := &ReportStats{}
		rs.UpdateKeys("alice")
		assert.Equal(t, "USER#alice", rs.PK)
		assert.Equal(t, "REPORT_STATS", rs.SK)
		assert.Equal(t, MainTableName, rs.TableName())
	})

	t.Run("WebAuthnChallenge BeforeCreate and UpdateKeys set TTL from ExpiresAt when unset", func(t *testing.T) {
		expiresAt := time.Now().Add(5 * time.Minute)
		w := &WebAuthnChallenge{Challenge: "c1", ExpiresAt: expiresAt}
		require.NoError(t, w.BeforeCreate())
		assert.Equal(t, "CHALLENGE#c1", w.PK)
		assert.Equal(t, "WEBAUTHN", w.SK)
		assert.Equal(t, "WebAuthnChallenge", w.ItemType)
		assert.Equal(t, expiresAt.Unix(), w.TTL)
		assert.Equal(t, MainTableName, w.TableName())
		assert.Equal(t, w.PK, w.GetPK())
		assert.Equal(t, w.SK, w.GetSK())

		w2 := &WebAuthnChallenge{Challenge: "c2", ExpiresAt: expiresAt, TTL: 1}
		require.NoError(t, w2.UpdateKeys())
		assert.Equal(t, int64(1), w2.TTL)
	})
}

func TestOAuthApp_RefreshToken_AuthorizationCode(t *testing.T) {
	t.Run("OAuthApp UpdateKeys and helpers", func(t *testing.T) {
		o := &OAuthApp{
			ClientID:     "cid",
			Name:         "MyApp",
			RedirectURIs: []string{"https://example.com/cb"},
			Scopes:       []string{"read", "write"},
		}
		o.UpdateKeys()
		assert.Equal(t, "OAUTH_APP#cid", o.PK)
		assert.Equal(t, SKMetadata, o.SK)
		assert.Equal(t, "OAUTH_APPS", o.GSI1PK)
		assert.Equal(t, "MyApp#cid", o.GSI1SK)
		assert.True(t, o.HasScope("read"))
		assert.False(t, o.HasScope("admin"))
		assert.True(t, o.IsValidRedirectURI("https://example.com/cb"))
		assert.False(t, o.IsValidRedirectURI("https://evil.example/cb"))
		assert.Equal(t, MainTableName, o.TableName())
	})

	t.Run("RefreshToken and AuthorizationCode set keys and TTL", func(t *testing.T) {
		expiresAt := time.Now().Add(time.Hour)
		rt := &RefreshToken{Token: "rt", ExpiresAt: expiresAt}
		require.NoError(t, rt.BeforeCreate())
		assert.Equal(t, "REFRESHTOKEN#rt", rt.PK)
		assert.Equal(t, SKToken, rt.SK)
		assert.Equal(t, expiresAt.Unix(), rt.TTL)
		assert.Equal(t, MainTableName, rt.TableName())
		assert.Equal(t, rt.PK, rt.GetPK())
		assert.Equal(t, rt.SK, rt.GetSK())
		require.NoError(t, rt.UpdateKeys())
		assert.Equal(t, "TOKEN", rt.SK)

		ac := &AuthorizationCode{Code: "code", ExpiresAt: expiresAt}
		require.NoError(t, ac.BeforeCreate())
		assert.Equal(t, "AUTHCODE#code", ac.PK)
		assert.Equal(t, "CODE", ac.SK)
		assert.Equal(t, expiresAt.Unix(), ac.TTL)
		assert.Equal(t, MainTableName, ac.TableName())
		assert.Equal(t, ac.PK, ac.GetPK())
		assert.Equal(t, ac.SK, ac.GetSK())
		require.NoError(t, ac.UpdateKeys())
		assert.Equal(t, "AUTHCODE#code", ac.PK)
	})
}

func TestInstanceConfig_TimelineEntry_InstanceMetrics_Article_Activity(t *testing.T) {
	t.Run("InstanceConfig constructors and AI config", func(t *testing.T) {
		rules := NewInstanceRulesConfig(`["a"]`)
		assert.Equal(t, instanceConfigPK, rules.PK)
		assert.Equal(t, "RULES", rules.SK)
		assert.NotZero(t, rules.UpdatedAt)

		desc := NewExtendedDescriptionConfig("desc")
		assert.Equal(t, instanceConfigPK, desc.PK)
		assert.Equal(t, "EXTENDED_DESC", desc.SK)
		assert.Equal(t, "desc", desc.ExtendedDescription)

		ai := NewAIInstanceConfig()
		require.NotNil(t, ai.Managed)
		assert.True(t, ai.Managed.AIEnabled)
		assert.True(t, ai.Managed.ModerationEnabled)
		assert.False(t, ai.Managed.PIIDetectionEnabled)
		require.NoError(t, ai.UpdateKeys())
		assert.Equal(t, instanceConfigPK, ai.PK)
		assert.Equal(t, "AI_CONFIG", ai.SK)
		assert.Equal(t, MainTableName, ai.TableName())
	})

	t.Run("TimelineEntry keying and expiry", func(t *testing.T) {
		timelineAt := time.Unix(1700000000, 0).UTC()
		expiresAt := time.Unix(1700000600, 0).UTC()
		e := &TimelineEntry{
			TimelineType: "PUBLIC",
			TimelineID:   "LOCAL",
			EntryID:      "e1",
			PostID:       "p1",
			TimelineAt:   timelineAt,
			ExpiresAt:    expiresAt,
		}
		e.UpdateKeys()
		assert.Equal(t, "TIMELINE#PUBLIC#LOCAL", e.PK)
		assert.Equal(t, "1700000000#e1", e.SK)
		assert.Equal(t, "TIMELINE#PUBLIC#LOCAL", e.GSI1PK)
		assert.Equal(t, e.SK, e.GSI1SK)
		assert.Equal(t, expiresAt.Unix(), e.TTL)

		e2 := &TimelineEntry{TimelineType: "HOME", TimelineID: "alice", EntryID: "e2", PostID: "p2", TimelineAt: timelineAt}
		e2.UpdateKeys()
		assert.Equal(t, "", e2.GSI1PK)
		assert.Equal(t, "", e2.GSI1SK)

		withGenerated := &TimelineEntry{TimelineAt: timelineAt, PostID: "p3"}
		withGenerated.SetEntryID()
		assert.Equal(t, "1700000000000000000#p3", withGenerated.EntryID)
		withGenerated.SetEntryID()
		assert.Equal(t, "1700000000000000000#p3", withGenerated.EntryID)

		neverExpire := &TimelineEntry{}
		assert.False(t, neverExpire.IsExpired())
		past := &TimelineEntry{ExpiresAt: time.Now().Add(-time.Second)}
		assert.True(t, past.IsExpired())
	})

	t.Run("InstanceMetrics UpdateKeys validates required fields", func(t *testing.T) {
		i := &InstanceMetrics{}
		assert.ErrorContains(t, i.UpdateKeys(), "date is required")
		i.Date = "2025-01-01"
		assert.ErrorContains(t, i.UpdateKeys(), "metric type is required")
		i.MetricType = "total_users"
		require.NoError(t, i.UpdateKeys())
		assert.Equal(t, "INSTANCE_METRICS#2025-01-01", i.PK)
		assert.Equal(t, "METRIC#total_users", i.SK)
		assert.Equal(t, "INSTANCE_METRICS", i.GSI1PK)
		assert.Equal(t, "2025-01-01", i.GSI1SK)
		assert.Equal(t, i.PK, i.GetPK())
		assert.Equal(t, i.SK, i.GetSK())
		assert.Equal(t, MainTableName, i.TableName())
	})

	t.Run("Article UpdateKeys syncs embedded object timestamps", func(t *testing.T) {
		a := &Article{
			Object: Object{
				ID:           "obj1",
				Type:         "Article",
				AttributedTo: "alice",
				Published:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		}
		assert.ErrorContains(t, (&Article{}).UpdateKeys(), "ID is required")

		before := time.Now()
		require.NoError(t, a.UpdateKeys())
		after := time.Now()
		assert.Equal(t, "object#obj1", a.PK)
		assert.Equal(t, "object#obj1", a.SK)
		assert.Equal(t, "actor#alice", a.GSI1PK)
		assert.Equal(t, "object#2025-01-01T00:00:00Z#obj1", a.GSI1SK)
		assert.WithinDuration(t, before, a.CreatedAt, 2*time.Second)
		assert.WithinDuration(t, before, a.UpdatedAt, 2*time.Second)
		assert.True(t, a.UpdatedAt.Before(after.Add(2*time.Second)))
		assert.True(t, a.Updated.Equal(a.UpdatedAt))
		assert.Equal(t, MainTableName, a.TableName())
	})

	t.Run("Activity UpdateKeys derives keys from ActivityPub data", func(t *testing.T) {
		createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		act := &Activity{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{ID: "a1"},
				Actor:      "https://example.com/users/alice",
			},
			CreatedAt: createdAt,
		}
		require.NoError(t, act.UpdateKeys())
		assert.Equal(t, "ACTOR#alice", act.PK)
		assert.Equal(t, "ACTIVITY#2025-01-01T00:00:00Z#a1", act.SK)
		assert.Equal(t, MainTableName, act.TableName())
		assert.Equal(t, act.PK, act.GetPK())
		assert.Equal(t, act.SK, act.GetSK())

		act2 := &Activity{
			PK: "ACTOR#bob",
			SK: "SK",
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{ID: "x"},
			},
			CreatedAt: createdAt,
		}
		require.NoError(t, act2.UpdateKeys())
		assert.Equal(t, "ACTOR#bob", act2.PK)
		assert.Equal(t, "SK", act2.SK)

		act3 := &Activity{}
		require.NoError(t, act3.UpdateKeys())
		assert.Empty(t, act3.PK)
		assert.Empty(t, act3.SK)
	})
}

func TestModerationAnalytics_DeliveryStatus_Revision_StatusPin_RemoteActor_Vouch_HashtagHistory_OAuthState(t *testing.T) {
	t.Run("ModerationAnalytics UpdateKeys supports both patterns", func(t *testing.T) {
		m := &ModerationAnalytics{Date: "2025-01-01", ReportType: "spam"}
		require.NoError(t, m.UpdateKeys())
		assert.Equal(t, "MOD_ANALYTICS#2025-01-01", m.PK)
		assert.Equal(t, "type#spam", m.SK)
		assert.Equal(t, "MOD_ANALYTICS#spam", m.GSI2PK)
		assert.Equal(t, "2025-01-01", m.GSI2SK)
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())

		m2 := &ModerationAnalytics{PatternID: "p1", Timestamp: time.Unix(1700000000, 0).UTC(), Date: "2025-01-01", ReportType: "spam"}
		require.NoError(t, m2.UpdateKeys())
		assert.Equal(t, "PATTERN#p1", m2.PK)
		assert.Contains(t, m2.SK, "ANALYTICS#")
	})

	t.Run("DeliveryStatus UpdateKeys handles failed retries and TTL source", func(t *testing.T) {
		createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		nextRetry := createdAt.Add(time.Hour)
		d := &DeliveryStatus{
			ActivityID:   "a1",
			TargetDomain: "example.com",
			Status:       DeliveryStatusFailed,
			CreatedAt:    createdAt,
			NextRetry:    nextRetry,
		}
		d.UpdateKeys()
		assert.Equal(t, "DELIVERY#a1", d.PK)
		assert.Equal(t, "TARGET#example.com", d.SK)
		assert.Equal(t, "FAILED_DELIVERIES", d.GSI1PK)
		assert.Contains(t, d.GSI1SK, "#example.com#a1")
		assert.Equal(t, createdAt.Add(30*24*time.Hour).Unix(), d.TTL)

		deliveredAt := createdAt.Add(time.Minute)
		d2 := &DeliveryStatus{ActivityID: "a1", TargetDomain: "example.com", Status: DeliveryStatusDelivered, CreatedAt: createdAt, DeliveredAt: deliveredAt}
		d2.UpdateKeys()
		assert.Equal(t, "", d2.GSI1PK)
		assert.Equal(t, deliveredAt.Add(30*24*time.Hour).Unix(), d2.TTL)
		assert.Equal(t, MainTableName, d2.TableName())
	})

	t.Run("Revision UpdateKeys validates required ObjectID and pads version", func(t *testing.T) {
		r := &Revision{}
		assert.ErrorContains(t, r.UpdateKeys(), "ObjectID is required")
		r.ObjectID = "o1"
		r.Version = 2
		before := time.Now()
		require.NoError(t, r.UpdateKeys())
		assert.Equal(t, "OBJECT#o1#REVISION", r.PK)
		assert.Equal(t, "VERSION#00000002", r.SK)
		assert.WithinDuration(t, before, r.CreatedAt, 2*time.Second)
		assert.Equal(t, r.CreatedAt, r.UpdatedAt)
		assert.Equal(t, r.PK, r.GetPK())
		assert.Equal(t, r.SK, r.GetSK())
		assert.Equal(t, MainTableName, r.TableName())
	})

	t.Run("StatusPin sets CreatedAt and keys", func(t *testing.T) {
		s := &StatusPin{Username: "alice", StatusID: "status-1"}
		before := time.Now()
		require.NoError(t, s.BeforeCreate())
		assert.WithinDuration(t, before, s.CreatedAt, 2*time.Second)
		assert.Equal(t, "USER#alice#PINS", s.PK)
		assert.Equal(t, "STATUS#status-1", s.SK)
		assert.Equal(t, MainTableName, s.TableName())
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())
	})

	t.Run("RemoteActor UpdateKeys extracts domain and sets TTL", func(t *testing.T) {
		expires := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		r := &RemoteActor{Handle: "user@domain.tld", ExpiresAt: expires}
		r.UpdateKeys()
		assert.Equal(t, "REMOTE_ACTOR#user@domain.tld", r.PK)
		assert.Equal(t, SKProfile, r.SK)
		assert.Equal(t, "domain.tld", r.Domain)
		assert.Equal(t, expires.Unix(), r.TTL)
		assert.Equal(t, "", extractDomainFromHandle("localuser"))
		assert.Equal(t, MainTableName, r.TableName())
	})

	t.Run("Vouch UpdateKeys sets all indexes", func(t *testing.T) {
		v := &Vouch{}
		created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		expires := created.Add(24 * time.Hour)
		v.UpdateKeys("v1", "from", "to", true, created, expires)
		assert.Equal(t, "VOUCH#v1", v.PK)
		assert.Equal(t, SKMetadata, v.SK)
		assert.Equal(t, "VOUCHER#from", v.GSI1PK)
		assert.Equal(t, "TO#to", v.GSI1SK)
		assert.Equal(t, "VOUCHEE#to", v.GSI2PK)
		assert.Equal(t, "FROM#from", v.GSI2SK)
		assert.True(t, v.Active)
		assert.Equal(t, expires.Unix(), v.ExpiresAt)
		assert.Equal(t, MainTableName, v.TableName())
	})

	t.Run("HashtagHistoryEntry helpers", func(t *testing.T) {
		date := time.Now().Add(-49 * time.Hour)
		h := NewHashtagHistoryEntry(date, 10, 5)
		assert.Equal(t, date, h.Date)
		assert.InDelta(t, 2.0, h.GetEngagement(), 0.0001)
		assert.True(t, h.IsHighActivity(9))
		assert.False(t, h.IsHighActivity(10))
		assert.Equal(t, 2, h.DaysSince())

		assert.Equal(t, 0.0, (HashtagHistoryEntry{}).CompareWith(HashtagHistoryEntry{UsageCount: 10}))
		assert.InDelta(t, 100.0, HashtagHistoryEntry{UsageCount: 10}.CompareWith(HashtagHistoryEntry{UsageCount: 20}), 0.0001)
		assert.Equal(t, MainTableName, h.TableName())
	})

	t.Run("OAuthState UpdateKeys sets TTL and optional keys", func(t *testing.T) {
		expires := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		o := &OAuthState{State: "s1", ExpiresAt: expires}
		require.NoError(t, o.UpdateKeys())
		assert.Equal(t, "OAUTH_STATE#s1", o.PK)
		assert.Equal(t, "STATE", o.SK)
		assert.Equal(t, expires.Unix(), o.TTL)
		assert.Equal(t, o.PK, o.GetPK())
		assert.Equal(t, o.SK, o.GetSK())
		assert.Equal(t, MainTableName, o.TableName())

		o2 := &OAuthState{State: "", ExpiresAt: expires}
		require.NoError(t, o2.UpdateKeys())
		assert.Empty(t, o2.PK)
		assert.Empty(t, o2.SK)
		assert.Equal(t, expires.Unix(), o2.TTL)
	})
}

func TestNotificationPreferences_StreamingEvent_OutboxInbox_Scheduled_Setup_Import_DNS_Search(t *testing.T) {
	t.Run("NotificationPreferences lifecycle updates keys and UpdatedAt", func(t *testing.T) {
		p := &NotificationPreferences{Username: "alice"}
		require.NoError(t, p.BeforeCreate())
		assert.Equal(t, "USER#alice", p.PK)
		assert.Equal(t, "NOTIFICATION_PREFS", p.SK)
		before := p.UpdatedAt
		require.NoError(t, p.BeforeUpdate())
		assert.True(t, p.UpdatedAt.After(before))
		assert.Equal(t, MainTableName, p.TableName())
	})

	t.Run("StreamingEvent key helpers", func(t *testing.T) {
		createdAt := time.Unix(1700000000, 0).UTC()
		e := &StreamingEvent{
			EventID:    "e1",
			EventType:  "status.created",
			TargetType: "user",
			TargetID:   "u1",
			CreatedAt:  createdAt,
		}
		e.UpdateKeys()
		assert.Equal(t, "STREAM_EVENT#e1", e.PK)
		assert.Equal(t, "EVENT", e.SK)
		assert.Equal(t, "STREAM_TARGET#user#u1", e.GSI1PK)
		assert.Equal(t, "1700000000#e1", e.GSI1SK)
		assert.Equal(t, "STREAM_TYPE#status.created", e.GSI2PK)
		assert.Equal(t, "1700000000#e1", e.GSI2SK)
		assert.Equal(t, MainTableName, e.GetTableName())
		assert.Equal(t, MainTableName, e.TableName())
		pk, sk := e.GetPrimaryKey()
		assert.Equal(t, e.PK, pk)
		assert.Equal(t, e.SK, sk)
	})

	t.Run("OutboxItem and InboxItem keys differ by GSI1 usage", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		o := &OutboxItem{ActorID: "alice", ActivityID: "a1", Timestamp: ts, Public: true}
		o.UpdateKeys()
		assert.Equal(t, "ACTOR#alice", o.PK)
		assert.Contains(t, o.SK, "ACTIVITY#")
		assert.Equal(t, "PUBLIC_OUTBOX#alice", o.GSI1PK)
		assert.Equal(t, ts.Format(time.RFC3339Nano), o.GSI1SK)
		assert.Equal(t, MainTableName, o.TableName())

		o2 := &OutboxItem{ActorID: "alice", ActivityID: "a2", Timestamp: ts, Public: false}
		o2.UpdateKeys()
		assert.Equal(t, "", o2.GSI1PK)
		assert.Equal(t, "", o2.GSI1SK)

		i := &InboxItem{ActorID: "alice", ActivityID: "a1", Timestamp: ts}
		i.UpdateKeys()
		assert.Equal(t, "ACTOR#alice", i.PK)
		assert.Equal(t, "INBOX#alice", i.GSI1PK)
		assert.Equal(t, ts.Format(time.RFC3339Nano), i.GSI1SK)
		assert.Equal(t, MainTableName, i.TableName())
	})

	t.Run("ScheduledStatus UpdateKeys and basic models", func(t *testing.T) {
		scheduledAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		s := &ScheduledStatus{ID: "id1", Username: "alice", ScheduledAt: scheduledAt}
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "USER#alice#SCHEDULED", s.PK)
		assert.Equal(t, "ID#id1", s.SK)
		assert.Equal(t, "SCHEDULED#DUE", s.GSI1PK)
		assert.Equal(t, "TIME#2025-01-01T00:00:00Z#ID#id1", s.GSI1SK)
		assert.Equal(t, MainTableName, s.TableName())
	})

	t.Run("SetupSession trims ID and sets TTL", func(t *testing.T) {
		expiresAt := time.Unix(1700000000, 0).UTC()
		s := &SetupSession{ID: "  tok  ", ExpiresAt: expiresAt}
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "tok", s.ID)
		assert.Equal(t, "SETUP_SESSION#tok", s.PK)
		assert.Equal(t, "SESSION", s.SK)
		assert.Equal(t, expiresAt.Unix(), s.TTL)
		assert.Equal(t, MainTableName, s.TableName())
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())
	})

	t.Run("Import UpdateKeys and getters", func(t *testing.T) {
		createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		i := &Import{ID: "imp1", Username: "alice", CreatedAt: createdAt, Status: "pending"}
		i.UpdateKeys()
		assert.Equal(t, "IMPORT#imp1", i.PK)
		assert.Equal(t, "IMPORT#imp1", i.SK)
		assert.Equal(t, "USER#alice", i.GSI1PK)
		assert.Equal(t, "CREATED#2025-01-01T00:00:00Z", i.GSI1SK)
		assert.Equal(t, "pending", i.GetStatus())
		assert.Equal(t, createdAt, i.GetCreatedAt())
		assert.Equal(t, MainTableName, i.TableName())
	})

	t.Run("DNSCache UpdateKeys is conditional", func(t *testing.T) {
		d := &DNSCache{}
		require.NoError(t, d.UpdateKeys())
		assert.Empty(t, d.PK)
		assert.Empty(t, d.SK)

		d.Hostname = "example.com"
		require.NoError(t, d.UpdateKeys())
		assert.Equal(t, "DNSCACHE#example.com", d.PK)
		assert.Equal(t, SKEntry, d.SK)
		assert.Equal(t, MainTableName, d.TableName())
	})

	t.Run("Search models keying", func(t *testing.T) {
		s := &SearchSuggestion{Type: "hashtag", Term: "go"}
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "SEARCH_SUGGEST#hashtag", s.PK)
		assert.Equal(t, "go", s.SK)
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())
		assert.Equal(t, MainTableName, s.TableName())

		ts := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		a := &SearchAnalytics{Query: "go", SearchType: "hashtags", Timestamp: ts}
		a.UpdateKeys()
		assert.Equal(t, "SEARCH_LOG#2025-01-02", a.PK)
		assert.Equal(t, "1735776000#hashtags#go", a.SK)
		assert.Equal(t, ts.Add(90*24*time.Hour).Unix(), a.TTL)

		a2 := &SearchAnalytics{Query: "go", SearchType: "hashtags", Timestamp: ts, TTL: 123}
		a2.UpdateKeys()
		assert.Equal(t, int64(123), a2.TTL)

		e := &SearchEmbedding{ContentID: "c1"}
		require.NoError(t, e.UpdateKeys())
		assert.Equal(t, "EMBEDDING#c1", e.PK)
		assert.Equal(t, "VECTOR", e.SK)
		assert.Equal(t, e.PK, e.GetPK())
		assert.Equal(t, e.SK, e.GetSK())
		assert.Equal(t, MainTableName, e.TableName())
	})
}

func TestNumericID_VAPID_FeaturedTag_UpdateHistory_ClickRate_SearchCache(t *testing.T) {
	t.Run("NumericIDMapping BeforeCreate sets static keys and type", func(t *testing.T) {
		n := &NumericIDMapping{NumericID: "1"}
		before := time.Now()
		require.NoError(t, n.BeforeCreate())
		after := time.Now()

		assert.Equal(t, "NUMERIC_ID#1", n.PK)
		assert.Equal(t, SKMetadata, n.SK)
		assert.Equal(t, "NumericIDMapping", n.Type)
		assert.WithinDuration(t, before, n.CreatedAt, 2*time.Second)
		assert.True(t, n.CreatedAt.Before(after.Add(2*time.Second)))
	})

	t.Run("VAPIDKeyRecord UpdateKeys sets static keys", func(t *testing.T) {
		v := &VAPIDKeyRecord{}
		require.NoError(t, v.UpdateKeys())
		assert.Equal(t, instanceConfigPK, v.PK)
		assert.Equal(t, "VAPID_KEYS", v.SK)
		assert.Equal(t, v.PK, v.GetPK())
		assert.Equal(t, v.SK, v.GetSK())
		assert.Equal(t, MainTableName, v.TableName())
	})

	t.Run("FeaturedTag UpdateKeys sets keys", func(t *testing.T) {
		f := &FeaturedTag{Username: "alice", ID: "ft1"}
		require.NoError(t, f.UpdateKeys())
		assert.Equal(t, "USER#alice", f.PK)
		assert.Equal(t, "FEATURED_TAG#ft1", f.SK)
		assert.Equal(t, f.PK, f.GetPK())
		assert.Equal(t, f.SK, f.GetSK())
		assert.Equal(t, MainTableName, f.TableName())
	})

	t.Run("UpdateHistory UpdateKeys pads version", func(t *testing.T) {
		u := &UpdateHistory{}
		u.UpdateKeys()
		assert.Empty(t, u.PK)
		assert.Empty(t, u.SK)

		u.ObjectID = "obj1"
		u.Version = 2
		u.UpdateKeys()
		assert.Equal(t, "OBJECT#obj1#HISTORY", u.PK)
		assert.Equal(t, "VERSION#00002", u.SK)
		assert.Equal(t, MainTableName, u.TableName())
	})

	t.Run("SearchClickRate UpdateKeys keys by query and actor", func(t *testing.T) {
		c := &SearchClickRate{Query: "go", ActorID: "alice"}
		c.UpdateKeys()
		assert.Equal(t, "CTR#go", c.PK)
		assert.Equal(t, "ACTOR#alice", c.SK)
		assert.Equal(t, MainTableName, c.TableName())
	})

	t.Run("SearchCache constructors and invalidation", func(t *testing.T) {
		before := time.Now()
		s := NewSearchCache("go")
		after := time.Now()

		assert.Equal(t, "SEARCH_CACHE#go", s.PK)
		assert.Equal(t, "RESULTS", s.SK)
		assert.NotNil(t, s.Results)
		assert.True(t, s.IsValid())
		assert.InDelta(t, before.Add(24*time.Hour).Unix(), s.TTL, 5)
		assert.True(t, time.Unix(s.TTL, 0).Before(after.Add(24*time.Hour).Add(5*time.Second)))

		s.InvalidateCache("stale")
		assert.False(t, s.IsValid())
		assert.Equal(t, true, s.Results["invalidated"])
		assert.Equal(t, "stale", s.Results["invalidation_reason"])
		assert.NotNil(t, s.Results["invalidated_at"])

		s2 := &SearchCache{TTL: time.Now().Add(-time.Second).Unix()}
		assert.False(t, s2.IsValid())
		assert.Equal(t, MainTableName, s.TableName())
	})
}
