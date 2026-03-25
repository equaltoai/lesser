package models

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostProjection(t *testing.T) {
	t.Run("UpdateKeys sets keys and TTL", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		calculated := ts.Add(1 * time.Hour)

		cp := &CostProjection{
			Period:       "daily",
			Timestamp:    ts,
			CalculatedAt: calculated,
		}
		cp.UpdateKeys()

		assert.Equal(t, CostProjectionPK, cp.PK)
		assert.Equal(t, "daily#"+ts.Format(time.RFC3339), cp.SK)
		assert.Equal(t, calculated.AddDate(0, 3, 0).Unix(), cp.TTL)
		assert.Equal(t, MainTableName, cp.TableName())
	})

	t.Run("constructor and key helpers", func(t *testing.T) {
		cp := NewCostProjection("weekly")
		assert.Equal(t, CostProjectionPK, cp.PK)
		assert.NotEmpty(t, cp.SK)
		assert.Equal(t, "weekly", cp.Period)
		assert.NotNil(t, cp.TopDrivers)
		assert.NotNil(t, cp.Recommendations)

		ts := time.Unix(1700000000, 0).UTC()
		pk, sk := GetCostProjectionKey("weekly", ts)
		assert.Equal(t, CostProjectionPK, pk)
		assert.Equal(t, "weekly#"+ts.Format(time.RFC3339), sk)

		pk, prefix := GetLatestProjectionKeys("weekly")
		assert.Equal(t, CostProjectionPK, pk)
		assert.Equal(t, "weekly#", prefix)

		pk, start, end := GetProjectionRangeKeys("weekly", ts, ts.Add(time.Hour))
		assert.Equal(t, CostProjectionPK, pk)
		assert.Equal(t, "weekly#"+ts.Format(time.RFC3339), start)
		assert.Equal(t, "weekly#"+ts.Add(time.Hour).Format(time.RFC3339), end)
	})

	t.Run("variance, drivers, recommendations and budget checks", func(t *testing.T) {
		cp := &CostProjection{CurrentCost: 100, ProjectedCost: 130}
		cp.CalculateVariance()
		assert.InDelta(t, 30.0, cp.Variance, 0.0001)

		cp = &CostProjection{CurrentCost: 0, ProjectedCost: 10}
		cp.CalculateVariance()
		assert.Equal(t, 0.0, cp.Variance)

		// AddDriver sorts by cost and caps at 10.
		for i := 0; i < 12; i++ {
			cp.AddDriver(Driver{Type: fmt.Sprintf("d%d", i), Cost: float64(i)})
		}
		require.Len(t, cp.TopDrivers, 10)
		assert.GreaterOrEqual(t, cp.TopDrivers[0].Cost, cp.TopDrivers[1].Cost)

		cp.CurrentCost = 100
		cp.ProjectedCost = 130
		cp.CalculateVariance()
		cp.TopDrivers = []Driver{
			{Type: "compute", Cost: 150, PercentOfTotal: 40, Trend: "increasing"},
			{Type: "network", Domain: "example.com", Cost: 200, PercentOfTotal: 35, Trend: "increasing"},
		}
		cp.GenerateRecommendations()
		assert.NotEmpty(t, cp.Recommendations)
		assert.True(t, cp.IsOverBudget(129.0))
		assert.False(t, cp.IsOverBudget(2000.0))
	})
}

func TestCostDriver(t *testing.T) {
	t.Run("UpdateKeys and key helpers", func(t *testing.T) {
		measured := time.Unix(1700000000, 0).UTC()
		d := &Driver{Category: ResourceStorage, Resource: "s3", MeasuredAt: measured}
		d.UpdateKeys()
		assert.Equal(t, CostDriverPK, d.PK)
		assert.Equal(t, "storage#s3", d.SK)
		assert.Equal(t, measured.AddDate(0, 3, 0).Unix(), d.TTL)
		assert.Equal(t, MainTableName, d.TableName())

		pk, sk := GetCostDriverKey("storage", "s3")
		assert.Equal(t, "COST#DRIVER", pk)
		assert.Equal(t, "storage#s3", sk)

		pk, prefix := GetCostDriversByCategoryKeys("storage")
		assert.Equal(t, "COST#DRIVER", pk)
		assert.Equal(t, "storage#", prefix)
	})

	t.Run("trend, volume metrics, cost per unit, type, significance and formatting", func(t *testing.T) {
		d := &Driver{Cost: 100, PreviousCost: 0}
		d.CalculateTrend()
		assert.Equal(t, TrendStable, d.Trend)

		d.PreviousCost = 100
		d.Cost = 120
		d.CalculateTrend()
		assert.Equal(t, TrendIncreasing, d.Trend)

		d.Cost = 80
		d.CalculateTrend()
		assert.Equal(t, TrendDecreasing, d.Trend)

		d.Cost = 105
		d.CalculateTrend()
		assert.Equal(t, TrendStable, d.Trend)

		d.SetVolumeMetric("requests", 10)
		assert.Equal(t, float64(10.5), d.GetCostPerUnit("requests"))
		assert.Equal(t, float64(0), d.GetCostPerUnit("missing"))

		d.Category = ResourceStorage
		d.Resource = "dynamodb"
		d.DetermineCostType()
		assert.Contains(t, d.Type, "Storage")

		d.Category = ResourceCompute
		d.DetermineCostType()
		assert.Contains(t, d.Type, "Compute")

		d.Category = "network"
		d.Domain = "example.com"
		d.DetermineCostType()
		assert.Contains(t, d.Type, "Federation with example.com")

		d.Domain = ""
		d.Resource = "egress"
		d.DetermineCostType()
		assert.Contains(t, d.Type, "Network - egress")

		d.Category = "api"
		d.Resource = "GetItem"
		d.DetermineCostType()
		assert.Contains(t, d.Type, "API Calls")

		d.Category = "other"
		d.Resource = "x"
		d.DetermineCostType()
		assert.Contains(t, d.Type, "other - x")

		d.PercentOfTotal = 6
		d.Cost = 1
		assert.True(t, d.IsSignificant())
		d.PercentOfTotal = 1
		d.Cost = 51
		assert.True(t, d.IsSignificant())
		d.PercentOfTotal = 1
		d.Cost = 1
		assert.False(t, d.IsSignificant())

		d.Type = "Compute - x"
		d.Cost = 10
		d.PercentOfTotal = 15.5
		d.Trend = TrendIncreasing
		assert.Contains(t, d.FormatCostSummary(), "↑")
		d.Trend = TrendDecreasing
		assert.Contains(t, d.FormatCostSummary(), "↓")
	})
}

func TestMediaPopularity(t *testing.T) {
	t.Run("UpdateKeys sets stable keys and inverted popularity sort key", func(t *testing.T) {
		m := &MediaPopularity{
			MediaID:   "m1",
			Period:    "DAY",
			ViewCount: 10,
			Date:      "2025-01-01",
		}
		require.NoError(t, m.UpdateKeys())
		assert.Equal(t, "MEDIA_POPULARITY#DAY", m.PK)
		assert.Equal(t, "MEDIA#m1", m.SK)
		assert.Equal(t, "PERIOD#DAY", m.GSI1PK)
		assert.Len(t, m.GSI1SK, 20)
		assert.Equal(t, "DATE#2025-01-01", m.GSI2PK)
		assert.Equal(t, "MEDIA#m1", m.GSI2SK)
		assert.Equal(t, MainTableName, m.TableName())
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())
	})

	t.Run("SetForPeriod initializes maps and TTL based on period", func(t *testing.T) {
		before := time.Now()
		m := &MediaPopularity{}
		m.SetForPeriod("m1", "DAY", 10)
		after := time.Now()

		assert.Equal(t, "m1", m.MediaID)
		assert.Equal(t, "DAY", m.Period)
		assert.Equal(t, int64(10), m.ViewCount)
		assert.NotEmpty(t, m.Date)
		assert.NotNil(t, m.QualityViews)
		assert.InDelta(t, float64(10), m.PopularityScore, 0.0001)
		assert.GreaterOrEqual(t, m.TTL, before.Add(7*24*time.Hour).Unix()-2)
		assert.LessOrEqual(t, m.TTL, after.Add(7*24*time.Hour).Unix()+2)

		m2 := &MediaPopularity{}
		m2.SetForPeriod("m1", "WEEK", 10)
		assert.InDelta(t, time.Now().Add(30*24*time.Hour).Unix(), m2.TTL, 2)

		m3 := &MediaPopularity{}
		m3.SetForPeriod("m1", "MONTH", 10)
		assert.InDelta(t, time.Now().Add(90*24*time.Hour).Unix(), m3.TTL, 2)
	})

	t.Run("increment and averages", func(t *testing.T) {
		m := &MediaPopularity{MediaID: "m1", Period: "DAY", Date: "2025-01-01", ViewCount: 0}
		m.IncrementViews(2)
		assert.Equal(t, int64(2), m.ViewCount)
		assert.False(t, m.LastViewed.IsZero())

		m.AddQualityView("720p", 1)
		m.AddQualityView("720p", 2)
		assert.Equal(t, int64(3), m.QualityViews["720p"])

		m.CompletionCount = 1
		assert.InDelta(t, 0.5, m.CalculateCompletionRate(), 0.0001)
		m.TotalWatchTime = 10
		assert.InDelta(t, 5.0, m.CalculateAvgWatchTime(), 0.0001)

		m.ViewCount = 0
		assert.Equal(t, 0.0, m.CalculateCompletionRate())
		assert.Equal(t, 0.0, m.CalculateAvgWatchTime())
	})
}

func TestQuotePermissions(t *testing.T) {
	t.Run("UpdateKeys sets keys and permissions logic respects block list and visibility", func(t *testing.T) {
		q := &QuotePermissions{Username: "alice"}
		require.NoError(t, q.UpdateKeys())
		assert.Equal(t, "USER#alice", q.PK)
		assert.Equal(t, "QUOTE_PERMISSIONS", q.SK)
		assert.Equal(t, MainTableName, q.TableName())
		assert.Equal(t, q.PK, q.GetPK())
		assert.Equal(t, q.SK, q.GetSK())

		q.SetDefaults()
		assert.True(t, q.IsAllowed("bob", false, false))

		q.AllowPublic = false
		q.AllowFollowers = true
		q.AllowMentioned = false
		assert.True(t, q.IsAllowed("bob", true, false))
		assert.False(t, q.IsAllowed("bob", false, false))

		q.AddToBlockList("bob")
		assert.False(t, q.IsAllowed("bob", true, true))
		q.AddToBlockList("bob")
		assert.Len(t, q.BlockList, 1)

		q.RemoveFromBlockList("bob")
		assert.Empty(t, q.BlockList)

		q.ApplyVisibilityDefaults("direct")
		assert.False(t, q.AllowPublic)
		assert.False(t, q.AllowFollowers)
		assert.True(t, q.AllowMentioned)

		q.ApplyVisibilityDefaults("followers-only")
		assert.False(t, q.AllowPublic)
		assert.True(t, q.AllowFollowers)

		q.ApplyVisibilityDefaults("public")
		assert.True(t, q.AllowPublic)
		assert.True(t, q.AllowFollowers)
		assert.True(t, q.AllowMentioned)
	})
}

func TestInstanceHealthReport(t *testing.T) {
	t.Run("keys and key helpers", func(t *testing.T) {
		checked := time.Unix(1700000000, 0).UTC()
		h := &InstanceHealthReport{Domain: "example.com", Timestamp: "t1", LastChecked: checked}
		h.UpdateKeys()
		assert.Equal(t, "INSTANCE#example.com", h.PK)
		assert.Equal(t, "HEALTH#t1", h.SK)
		assert.Equal(t, checked.AddDate(0, 0, 30).Unix(), h.TTL)
		assert.Equal(t, MainTableName, h.TableName())

		pk, sk := GetHealthReportKey("example.com", "t1")
		assert.Equal(t, "INSTANCE#example.com", pk)
		assert.Equal(t, "HEALTH#t1", sk)

		pk, prefix := GetLatestHealthReportKeys("example.com")
		assert.Equal(t, "INSTANCE#example.com", pk)
		assert.Equal(t, "HEALTH#", prefix)

		pk, start, end := GetHealthReportRangeKeys("example.com", checked, checked.Add(time.Second))
		assert.Equal(t, "INSTANCE#example.com", pk)
		assert.Contains(t, start, "HEALTH#")
		assert.Contains(t, end, "HEALTH#")
	})

	t.Run("status classification and helpers", func(t *testing.T) {
		h := &InstanceHealthReport{}

		h.ErrorRate = 0.6
		h.ResponseTime = 100
		h.SetHealthStatus()
		assert.Equal(t, StatusCritical, h.Status)
		assert.False(t, h.IsHealthy())
		assert.True(t, h.IsCritical())

		h.ErrorRate = 0.0
		h.ResponseTime = 3000
		h.QueueDepth = 0
		h.SetHealthStatus()
		assert.Equal(t, StatusWarning, h.Status)

		h.ErrorRate = 0.0
		h.ResponseTime = 100
		h.QueueDepth = 2000
		h.SetHealthStatus()
		assert.Equal(t, StatusWarning, h.Status)

		h.ErrorRate = 0.0
		h.ResponseTime = 100
		h.QueueDepth = 0
		h.FederationDelay = 301
		h.SetHealthStatus()
		assert.Equal(t, "healthy", h.Status)
		assert.NotEmpty(t, h.Issues)
		assert.NotEmpty(t, h.Recommendations)
	})
}

func TestTrustModels(t *testing.T) {
	t.Run("domain extraction helper", func(t *testing.T) {
		assert.Equal(t, "example.com", getDomainFromActorID("https://example.com/users/alice"))
		assert.Equal(t, "example.com", getDomainFromActorID("@alice@example.com"))
		assert.Equal(t, "local", getDomainFromActorID("alice"))
	})

	t.Run("TrustRelationship sets keys for GSIs", func(t *testing.T) {
		tr := &TrustRelationship{
			TrusterID: "https://local/users/bob",
			TrusteeID: "https://example.com/users/alice",
			Category:  TrustCategoryContent,
			Score:     0.5,
		}
		require.NoError(t, tr.UpdateKeys())
		assert.Equal(t, "TRUST#https://local/users/bob#content", tr.PK)
		assert.Equal(t, "TRUSTEE#https://example.com/users/alice", tr.SK)
		assert.Equal(t, "TRUSTED#https://example.com/users/alice#content", tr.GSI1PK)
		assert.Equal(t, "TRUSTER#https://local/users/bob", tr.GSI1SK)
		assert.Equal(t, "DOMAIN#example.com", tr.GSI2PK)
		assert.Contains(t, tr.GSI2SK, "TRUST#content#")
		assert.Equal(t, "RELATIONSHIP", tr.Type)
		assert.Equal(t, MainTableName, tr.TableName())
	})

	t.Run("TrustScore and TrustUpdate set keys and TTL", func(t *testing.T) {
		cacheTTL := time.Unix(1700000000, 0).UTC()
		ts := &TrustScore{ActorID: "alice", Category: TrustCategoryGeneral, CacheTTL: cacheTTL}
		require.NoError(t, ts.UpdateKeys())
		assert.Equal(t, "SCORE#alice#general", ts.PK)
		assert.Equal(t, SKCurrent, ts.SK)
		assert.Equal(t, cacheTTL.Unix(), ts.TTL)
		assert.Equal(t, "SCORE", ts.Type)

		tu := &TrustUpdate{ActorID: "alice", Category: TrustCategoryGeneral, EventID: "e1", Timestamp: cacheTTL}
		require.NoError(t, tu.UpdateKeys())
		assert.Equal(t, "UPDATES#alice", tu.PK)
		assert.Equal(t, "UPDATE", tu.Type)
		assert.NotZero(t, tu.TTL)
	})
}

func TestConversationModels(t *testing.T) {
	t.Run("Conversation requires ID and sets stable keys", func(t *testing.T) {
		c := &Conversation{}
		assert.ErrorIs(t, c.BeforeCreate(), ErrConversationIDRequired)

		c.ID = "c1"
		require.NoError(t, c.BeforeCreate())
		assert.Equal(t, "CONVERSATION#c1", c.PK)
		assert.Equal(t, SKMetadata, c.SK)
		assert.Empty(t, c.GSI1PK)
		assert.Empty(t, c.GSI1SK)
		assert.Equal(t, MainTableName, c.TableName())

		c2 := &Conversation{ID: "c1"}
		require.NoError(t, c2.UpdateKeys())
		assert.Equal(t, "CONVERSATION#c1", c2.PK)
	})

	t.Run("ConversationParticipantRecord requires conversation data and sets keys", func(t *testing.T) {
		p := &ConversationParticipantRecord{}
		assert.ErrorIs(t, p.BeforeCreate("alice"), ErrConversationDataRequired)

		updated := time.Unix(1700000000, 0).UTC()
		p.Conversation = &Conversation{ID: "c1", Participants: []string{"alice", "bob"}, UpdatedAt: updated}
		require.NoError(t, p.BeforeCreate("alice"))
		assert.Equal(t, "USER_CONVERSATIONS#alice", p.PK)
		assert.Equal(t, updated.Format(time.RFC3339)+"#c1", p.SK)
		assert.Equal(t, "CONVERSATION#c1", p.GSI1PK)
		assert.Equal(t, "PARTICIPANT#alice", p.GSI1SK)
		assert.Equal(t, p.PK, p.GetPK())
		assert.Equal(t, p.SK, p.GetSK())
	})

	t.Run("ConversationParticipantRecord syncs and hydrates embedded snapshots", func(t *testing.T) {
		updated := time.Unix(1700000000, 0).UTC()
		p := &ConversationParticipantRecord{
			Unread:       true,
			Conversation: &Conversation{ID: "c2", Participants: []string{"alice", "bob"}, UpdatedAt: updated},
		}

		conv := p.SyncConversationData()
		require.NotNil(t, conv)
		require.NotNil(t, p.ConversationData)
		assert.Equal(t, "c2", p.ConversationData.ID)
		assert.Equal(t, []string{"alice", "bob"}, p.ConversationData.Participants)
		assert.True(t, p.ConversationData.Unread)

		rehydrated := (&ConversationParticipantRecord{
			Unread:           true,
			ConversationData: p.ConversationData,
		}).HydrateConversation()
		require.NotNil(t, rehydrated)
		assert.Equal(t, "c2", rehydrated.ID)
		assert.Equal(t, []string{"alice", "bob"}, rehydrated.Participants)
		assert.True(t, rehydrated.Unread)
	})

	t.Run("ConversationMessage requires keys and builds sortable SK", func(t *testing.T) {
		m := &ConversationMessage{}
		assert.ErrorIs(t, m.BeforeCreate(), ErrConversationStatusIDRequired)

		m.ConversationID = "c1"
		assert.ErrorIs(t, m.BeforeCreate(), ErrConversationStatusStatusIDRequired)

		created := time.Unix(1700000000, 0).UTC()
		m.StatusID = "s1"
		m.CreatedAt = created
		require.NoError(t, m.BeforeCreate())
		assert.Equal(t, "CONVERSATION#c1", m.PK)
		assert.Equal(t, fmt.Sprintf("STATUS#%s#s1", created.Format(time.RFC3339Nano)), m.SK)
		assert.NotNil(t, m.ReadBy)

		m2 := &ConversationMessage{ConversationID: "c1", StatusID: "s2", CreatedAt: created}
		require.NoError(t, m2.UpdateKeys())
		assert.Equal(t, "CONVERSATION#c1", m2.PK)
	})
}

func TestPushSubscription(t *testing.T) {
	t.Run("UpdateKeys builds keys and endpoint hash index", func(t *testing.T) {
		p := &PushSubscription{Username: "alice", ID: "sub1", Endpoint: "https://push.example/1"}
		require.NoError(t, p.UpdateKeys())
		assert.Equal(t, "PUSH#alice", p.PK)
		assert.Equal(t, "SUB#sub1", p.SK)
		assert.Equal(t, "PUSH_ENDPOINT#"+hashString(p.Endpoint), p.GSI1PK)
		assert.Equal(t, "alice", p.GSI1SK)
		assert.Equal(t, p.PK, p.GetPK())
		assert.Equal(t, p.SK, p.GetSK())
		assert.Equal(t, MainTableName, p.TableName())
	})

	t.Run("BeforeCreate populates timestamps, defaults ID, and validates required fields", func(t *testing.T) {
		p := &PushSubscription{
			Username: "alice",
			Endpoint: "https://push.example/1",
			P256dh:   "p256dh",
			Auth:     "auth",
		}
		require.NoError(t, p.BeforeCreate())
		assert.NotEmpty(t, p.ID)
		assert.False(t, p.CreatedAt.IsZero())
		assert.False(t, p.UpdatedAt.IsZero())

		p2 := &PushSubscription{Username: "alice", Endpoint: "https://push.example/1"}
		err := p2.Validate()
		assert.ErrorIs(t, err, ErrPushSubscriptionP256dhRequired)
	})

	t.Run("BeforeUpdate updates UpdatedAt and can update LastUsed", func(t *testing.T) {
		p := &PushSubscription{
			ID:       "sub1",
			Username: "alice",
			Endpoint: "https://push.example/1",
			P256dh:   "p256dh",
			Auth:     "auth",
		}
		require.NoError(t, p.BeforeCreate())
		before := p.UpdatedAt
		require.NoError(t, p.BeforeUpdate())
		assert.True(t, p.UpdatedAt.After(before) || p.UpdatedAt.Equal(before))

		p.UpdateLastUsed()
		assert.False(t, p.LastUsed.IsZero())
	})
}

func TestHashtagStats(t *testing.T) {
	t.Run("keys, counters, history and computed metrics", func(t *testing.T) {
		h := NewHashtagStats("golang")
		assert.Equal(t, "HASHTAG#golang", h.PK)
		assert.Equal(t, SKStats, h.SK)
		assert.Equal(t, MainTableName, h.TableName())

		h.IncrementUsage()
		h.AddUniqueUser()
		assert.Equal(t, int64(1), h.UsageCount)
		assert.Equal(t, int64(1), h.UniqueUsers)
		assert.Equal(t, int64(1), h.TotalUses)
		assert.Equal(t, int64(1), h.TotalAccounts)

		h.LastUsed = time.Now().Add(-2 * time.Hour)
		h.UpdateTrendingScore()
		assert.Greater(t, h.TrendingScore, 0.0)

		for i := 0; i < 40; i++ {
			h.AddHistoryEntry(time.Unix(int64(i), 0).UTC(), int64(i), int64(i))
		}
		assert.Len(t, h.History, 30)
		assert.Greater(t, h.GetAverageUsage(), 0.0)
		assert.Greater(t, h.GetGrowthRate(), 0.0)
		assert.True(t, h.IsActive())
		assert.True(t, h.IsTrending(0.0))

		empty := &HashtagStats{UsageCount: 10}
		assert.Equal(t, float64(10), empty.GetAverageUsage())
	})
}

func TestCMSArticleIndex(t *testing.T) {
	t.Run("PK and SK helpers, extract, and UpdateKeys validation", func(t *testing.T) {
		assert.Equal(t, "", CMSArticleIndexPKForAuthor(""))
		assert.Equal(t, "", CMSArticleIndexPKForSeries(" "))
		assert.Equal(t, "", CMSArticleIndexPKForCategory("\n"))
		assert.Equal(t, "", CMSArticleIndexSK(time.Time{}, ""))
		assert.Equal(t, "", CMSArticleIndexExtractArticleID(""))

		published := time.Unix(1700000000, 0).UTC()
		sk := CMSArticleIndexSK(published, "a1")
		assert.Contains(t, sk, CMSArticleIndexSKPrefix)
		assert.Equal(t, "a1", CMSArticleIndexExtractArticleID(sk))
		assert.Equal(t, "", CMSArticleIndexExtractArticleID("TIME#x"))

		idx := &CMSArticleIndex{}
		assert.ErrorContains(t, idx.UpdateKeys(), "PK is required")
		idx.PK = CMSArticleIndexPKForAuthor("actor")
		assert.ErrorContains(t, idx.UpdateKeys(), "SK is required")
		idx.SK = sk
		require.NoError(t, idx.UpdateKeys())
		assert.False(t, idx.CreatedAt.IsZero())
		assert.Equal(t, MainTableName, idx.TableName())
	})
}

func TestWeeklyActivity(t *testing.T) {
	t.Run("week normalization and aggregation helpers", func(t *testing.T) {
		// Sunday should normalize to previous Monday.
		sunday := time.Date(2025, 1, 5, 12, 0, 0, 0, time.UTC) // Sunday
		monday := normalizeToWeekStart(sunday)
		assert.Equal(t, time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC), monday)

		w := NewWeeklyActivity(sunday)
		require.NoError(t, w.UpdateKeys())
		assert.Equal(t, "INSTANCE#ACTIVITY", w.PK)
		assert.Contains(t, w.SK, "ACTIVITY#WEEK#")
		assert.Equal(t, MainTableName, w.TableName())

		u := NewUserWeeklyActivity("alice", sunday)
		require.NoError(t, u.UpdateKeys())
		assert.Equal(t, "USER#alice", u.PK)

		w.IncrementStatuses(2)
		w.IncrementLogins(3)
		w.IncrementRegistrations(4)
		assert.Equal(t, int64(9), w.GetTotalActivity())
		assert.True(t, w.IsActive())
		assert.InDelta(t, float64(9)/7.0, w.GetAverageDaily(), 0.0001)
		assert.Equal(t, w.GetWeekStart().AddDate(0, 0, 7), w.GetWeekEnd())

		other := &WeeklyActivity{Week: w.Week, Statuses: 1}
		w.Merge(other)
		assert.Equal(t, int64(3), w.Statuses)

		w.Merge(nil)
		w.Merge(&WeeklyActivity{Week: w.Week + 1, Statuses: 10})
	})
}

func TestStatusSearchOptions(t *testing.T) {
	t.Run("builders and Validate normalize values", func(t *testing.T) {
		o := NewStatusSearchOptions()
		require.NoError(t, o.Validate())
		assert.Equal(t, 20, o.Limit)
		assert.Equal(t, 0, o.Offset)

		// Validate caps/normalizes even if options are set directly (e.g. decoded from JSON).
		o.Limit = 200
		o.Offset = -1
		require.NoError(t, o.Validate())
		assert.Equal(t, 100, o.Limit)
		assert.Equal(t, 0, o.Offset)

		o.WithLimit(10).
			WithOffset(5).
			WithAccountID("acct").
			WithFollowingOnly().
			WithLocalOnly().
			WithMediaOnly().
			WithLanguage("en").
			WithMinEngagement(3)

		assert.Equal(t, 10, o.Limit)
		assert.Equal(t, 5, o.Offset)
		assert.Equal(t, "acct", o.AccountID)
		assert.True(t, o.FollowingOnly)
		assert.True(t, o.LocalOnly)
		assert.True(t, o.MediaOnly)
		assert.Equal(t, "en", o.Language)
		assert.Equal(t, 3, o.MinEngagement)

		start := time.Unix(10, 0).UTC()
		end := time.Unix(5, 0).UTC()
		o.WithTimeRange(start, end)
		require.NoError(t, o.Validate())
		assert.True(t, o.TimeRange.Start.Before(o.TimeRange.End) || o.TimeRange.Start.Equal(o.TimeRange.End))
	})
}
