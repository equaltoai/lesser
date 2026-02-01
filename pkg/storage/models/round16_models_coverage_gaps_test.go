package models

import (
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommunityNote_KeysAndVote(t *testing.T) {
	note := &CommunityNote{
		ID:               "n1",
		ObjectID:         "obj1",
		AuthorID:         "alice",
		Score:            1.5,
		VisibilityStatus: "public",
		CreatedAt:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, note.UpdateKeys())
	assert.Equal(t, "NOTE#n1", note.PK)
	assert.Equal(t, SKMetadata, note.SK)
	assert.Equal(t, "OBJECT#obj1#NOTES", note.GSI1PK)
	assert.Equal(t, "SCORE#001.500000#n1", note.GSI1SK)
	assert.Equal(t, "NOTES#public", note.GSI2PK)
	assert.Equal(t, "2025-01-01T00:00:00Z#n1", note.GSI2SK)
	assert.Equal(t, "AUTHOR#alice#NOTES", note.GSI3PK)
	assert.Equal(t, "2025-01-01T00:00:00Z#n1", note.GSI3SK)
	assert.Equal(t, note.PK, note.GetPK())
	assert.Equal(t, note.SK, note.GetSK())
	assert.Equal(t, MainTableName, note.TableName())

	vote := &CommunityNoteVote{NoteID: "n1", VoterID: "bob"}
	require.NoError(t, vote.UpdateKeys())
	assert.Equal(t, "NOTE#n1", vote.PK)
	assert.Equal(t, "VOTE#bob", vote.SK)
	assert.Equal(t, vote.PK, vote.GetPK())
	assert.Equal(t, vote.SK, vote.GetSK())
	assert.Equal(t, MainTableName, vote.TableName())
}

func TestTrendingStatus_ScoringAndKeys(t *testing.T) {
	t.Run("pow handles zero and positive exponent", func(t *testing.T) {
		assert.Equal(t, 1.0, pow(0.5, 0))
		assert.Equal(t, 0.125, pow(0.5, 3))
	})

	t.Run("CalculateTrendingScore applies decay only for past timestamps", func(t *testing.T) {
		future := &TrendingStatus{Likes: 1, Boosts: 1, Replies: 1, PublishedAt: time.Now().Add(time.Hour)}
		future.CalculateTrendingScore()
		assert.Equal(t, int64(3), future.Engagements)
		assert.InDelta(t, 6.0, future.TrendingScore, 0.0001) // 1*1 + 1*2 + 1*3 = 6

		past := &TrendingStatus{Likes: 10, Boosts: 0, Replies: 0, PublishedAt: time.Now().Add(-48 * time.Hour)}
		past.CalculateTrendingScore()
		assert.Equal(t, int64(10), past.Engagements)
		assert.InDelta(t, 10.0*0.25, past.TrendingScore, 1.0) // ~48h => int(exp)=2 => 0.25
	})

	t.Run("UpdateKeys sets TTL for valid dates and leaves TTL for invalid dates", func(t *testing.T) {
		ts := &TrendingStatus{ID: "s1", Date: "2025-01-01", TrendingScore: 123}
		ts.UpdateKeys()
		assert.Equal(t, "TRENDING#2025-01-01", ts.PK)
		assert.Equal(t, "STATUS#9999999877#s1", ts.SK)
		assert.Equal(t, time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC).Unix(), ts.TTL)

		bad := &TrendingStatus{ID: "s1", Date: "not-a-date", TrendingScore: 1}
		bad.UpdateKeys()
		assert.Equal(t, int64(0), bad.TTL)
	})

	t.Run("constructors and key helpers", func(t *testing.T) {
		base := &TrendingStatus{
			ID:          "s2",
			URL:         "https://example/statuses/2",
			AuthorID:    "alice",
			Content:     "hi",
			Likes:       2,
			Boosts:      1,
			Replies:     0,
			PublishedAt: time.Now().Add(time.Hour), // no decay
		}
		trending := NewTrendingStatus("2025-01-01", base)
		assert.Equal(t, "2025-01-01", trending.Date)
		assert.Equal(t, "TRENDING#2025-01-01", trending.PK)
		assert.NotEmpty(t, trending.SK)

		pk, sk := GetTrendingStatusKey("2025-01-01", "s2", 10)
		assert.Equal(t, "TRENDING#2025-01-01", pk)
		assert.Equal(t, "STATUS#9999999990#s2", sk)

		pk, prefix := GetTrendingStatusesKeys("2025-01-01")
		assert.Equal(t, "TRENDING#2025-01-01", pk)
		assert.Equal(t, "STATUS#", prefix)

		pk, start, end := GetTrendingStatusRangeKeys("2025-01-01", 0)
		assert.Equal(t, "TRENDING#2025-01-01", pk)
		assert.Equal(t, "STATUS#", start)
		assert.Equal(t, "STATUS#10000000000", end)
	})

	t.Run("IsStillTrending and FormatTrendingSummary", func(t *testing.T) {
		ts := &TrendingStatus{TrendingScore: 10, PublishedAt: time.Now().Add(-time.Minute), Rank: 1, Likes: 1, Boosts: 2, Replies: 3}
		assert.True(t, ts.IsStillTrending(5, time.Hour))
		assert.False(t, ts.IsStillTrending(50, time.Hour))
		assert.False(t, ts.IsStillTrending(5, time.Second))
		assert.Contains(t, ts.FormatTrendingSummary(), "Rank #1")
		assert.Equal(t, MainTableName, ts.TableName())
	})
}

func TestNotificationDelivery_LifecycleAndValidation(t *testing.T) {
	t.Run("Validate checks required fields and enums", func(t *testing.T) {
		d := &NotificationDelivery{}
		assert.ErrorIs(t, d.Validate(), ErrNotificationIDRequired)

		d.NotificationID = "n1"
		assert.ErrorIs(t, d.Validate(), ErrDeliveryMethodRequired)

		d.DeliveryMethod = "email"
		d.Status = StatusPending
		assert.True(t, errors.Is(d.Validate(), ErrInvalidDeliveryMethod))

		d.DeliveryMethod = "push"
		d.Status = "bogus"
		assert.True(t, errors.Is(d.Validate(), ErrInvalidDeliveryStatus))

		d.Status = StatusPending
		require.NoError(t, d.Validate())
	})

	t.Run("BeforeCreate sets defaults and keys", func(t *testing.T) {
		d := &NotificationDelivery{NotificationID: "n1", DeliveryMethod: "push"}
		before := time.Now()
		require.NoError(t, d.BeforeCreate())
		after := time.Now()

		assert.Equal(t, "NOTIFICATION#n1", d.PK)
		assert.Equal(t, "DELIVERY#push", d.SK)
		assert.Equal(t, StatusPending, d.Status)
		assert.Equal(t, 1, d.AttemptCount)
		assert.WithinDuration(t, before, d.CreatedAt, 2*time.Second)
		assert.WithinDuration(t, before, d.UpdatedAt, 2*time.Second)
		assert.WithinDuration(t, before, d.LastAttempt, 2*time.Second)
		assert.InDelta(t, before.Add(7*24*time.Hour).Unix(), d.TTL, 5)
		assert.True(t, time.Unix(d.TTL, 0).Before(after.Add(7*24*time.Hour).Add(5*time.Second)))
		assert.Equal(t, MainTableName, d.TableName())
	})

	t.Run("BeforeUpdate updates UpdatedAt and validates", func(t *testing.T) {
		d := &NotificationDelivery{NotificationID: "n1", DeliveryMethod: "push", Status: StatusPending}
		require.NoError(t, d.BeforeCreate())
		old := d.UpdatedAt
		require.NoError(t, d.BeforeUpdate())
		assert.True(t, d.UpdatedAt.After(old))
	})

	t.Run("state transitions", func(t *testing.T) {
		d := &NotificationDelivery{NotificationID: "n1", DeliveryMethod: "push"}
		require.NoError(t, d.BeforeCreate())

		d.MarkSent()
		assert.Equal(t, "sent", d.Status)
		assert.False(t, d.SentAt.IsZero())
		assert.Empty(t, d.Error)
		assert.False(t, d.CanRetry())

		d.MarkFailed("oops")
		assert.Equal(t, StatusFailed, d.Status)
		assert.Equal(t, "oops", d.Error)
		assert.Greater(t, d.AttemptCount, 1)
		assert.True(t, d.CanRetry())

		d.AttemptCount = 3
		assert.False(t, d.CanRetry())

		d.MarkPending()
		assert.Equal(t, StatusPending, d.Status)
		assert.Empty(t, d.Error)

		prevAttempt := d.AttemptCount
		d.IncrementAttempt()
		assert.Equal(t, prevAttempt+1, d.AttemptCount)
		assert.False(t, d.LastAttempt.IsZero())
	})

	t.Run("delivery method/status validators cover unsupported methods", func(t *testing.T) {
		assert.True(t, isValidDeliveryMethod("push"))
		assert.False(t, isValidDeliveryMethod("email"))
		assert.False(t, isValidDeliveryMethod("sms"))
		assert.False(t, isValidDeliveryMethod("unknown"))

		assert.True(t, isValidDeliveryStatus(StatusPending))
		assert.True(t, isValidDeliveryStatus("sent"))
		assert.True(t, isValidDeliveryStatus(StatusFailed))
		assert.False(t, isValidDeliveryStatus("unknown"))
	})

	t.Run("NewNotificationDelivery returns minimal record", func(t *testing.T) {
		d := NewNotificationDelivery("n1", "push")
		assert.Equal(t, "n1", d.NotificationID)
		assert.Equal(t, "push", d.DeliveryMethod)
		assert.Equal(t, StatusPending, d.Status)
		assert.Equal(t, 0, d.AttemptCount)
	})
}

func TestRelationshipRecord_DomainGSIsAndHelpers(t *testing.T) {
	t.Run("UpdateKeys requires PK and SK", func(t *testing.T) {
		r := &RelationshipRecord{}
		assert.ErrorContains(t, r.UpdateKeys(), "PK is required")
		r.PK = "FOLLOW#alice"
		assert.ErrorContains(t, r.UpdateKeys(), "SK is required")
	})

	t.Run("extractRelationshipDomain handles local, localhost, and mixed-case", func(t *testing.T) {
		_, ok := extractRelationshipDomain("alice")
		assert.False(t, ok)
		_, ok = extractRelationshipDomain("alice@localhost")
		assert.False(t, ok)

		domain, ok := extractRelationshipDomain("alice@Example.COM")
		assert.True(t, ok)
		assert.Equal(t, "example.com", domain)
	})

	t.Run("UpdateKeys sets domain GSIs for federated handles", func(t *testing.T) {
		r := NewRelationshipRecord("alice@example.com", "bob@remote.tld", "act1")
		require.NoError(t, r.UpdateKeys())
		assert.Equal(t, "FOLLOWER_DOMAIN#example.com", r.GSI2PK)
		assert.Equal(t, r.SK, r.GSI2SK)
		assert.Equal(t, "FOLLOWING_DOMAIN#remote.tld", r.GSI3PK)
		assert.Equal(t, r.GSI1SK, r.GSI3SK)
	})

	t.Run("BeforeCreate/BeforeUpdate wrap key errors", func(t *testing.T) {
		r := &RelationshipRecord{}
		assert.ErrorContains(t, r.BeforeCreate(), "failed to update keys")

		r = NewRelationshipRecord("alice", "bob", "act1")
		before := time.Now()
		require.NoError(t, r.BeforeCreate())
		assert.WithinDuration(t, before, r.CreatedAt, 2*time.Second)
		assert.WithinDuration(t, before, r.UpdatedAt, 2*time.Second)

		old := r.UpdatedAt
		require.NoError(t, r.BeforeUpdate())
		assert.True(t, r.UpdatedAt.After(old))
	})

	t.Run("state helpers and username extractors", func(t *testing.T) {
		r := NewRelationshipRecord("alice", "bob", "act1")
		assert.Equal(t, RelationshipPending, r.State)

		r.Accept()
		assert.Equal(t, RelationshipAccepted, r.State)
		r.Reject()
		assert.Equal(t, RelationshipRejected, r.State)

		assert.Equal(t, "alice", r.ExtractFollowerUsername())
		assert.Equal(t, "bob", r.ExtractFollowingUsername())
		assert.Equal(t, "alice", r.ExtractFollowerFromGSI())

		r.PK = "FOLLOW#"
		assert.Equal(t, "", r.ExtractFollowerUsername())
	})
}

func TestConversationStatus_AndMessage(t *testing.T) {
	t.Run("ConversationStatus BeforeCreate validates required fields", func(t *testing.T) {
		s := &ConversationStatus{}
		assert.ErrorIs(t, s.BeforeCreate(), ErrConversationStatusIDRequired)
		s.ConversationID = "c1"
		assert.ErrorIs(t, s.BeforeCreate(), ErrConversationStatusUserIDRequired)
	})

	t.Run("ConversationStatus BeforeCreate sets keys and LastReadAt", func(t *testing.T) {
		s := &ConversationStatus{ConversationID: "c1", UserID: "alice"}
		before := time.Now()
		require.NoError(t, s.BeforeCreate())
		assert.Equal(t, "CONVERSATION_STATUS#c1", s.PK)
		assert.Equal(t, "USER#alice", s.SK)
		assert.WithinDuration(t, before, s.LastReadAt, 2*time.Second)
		assert.Equal(t, MainTableName, s.TableName())
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())

		empty := &ConversationStatus{}
		require.NoError(t, empty.UpdateKeys())
		assert.Empty(t, empty.PK)
		assert.Empty(t, empty.SK)
	})

	t.Run("ConversationMessage BeforeCreate validates and initializes ReadBy", func(t *testing.T) {
		m := &ConversationMessage{}
		assert.ErrorIs(t, m.BeforeCreate(), ErrConversationStatusIDRequired)
		m.ConversationID = "c1"
		assert.ErrorIs(t, m.BeforeCreate(), ErrConversationStatusStatusIDRequired)

		m.StatusID = "s1"
		before := time.Now()
		require.NoError(t, m.BeforeCreate())
		assert.Equal(t, "CONVERSATION#c1", m.PK)
		assert.Contains(t, m.SK, "STATUS#")
		assert.NotNil(t, m.ReadBy)
		assert.WithinDuration(t, before, m.CreatedAt, 2*time.Second)
		assert.Equal(t, MainTableName, m.TableName())
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())

		m2 := &ConversationMessage{ConversationID: "c2", StatusID: "s2"}
		require.NoError(t, m2.UpdateKeys())
		assert.Equal(t, "CONVERSATION#c2", m2.PK)
		assert.Contains(t, m2.SK, "#s2")
	})
}

func TestActor_BeforeCreate_UpdateKeys_AndMetadata(t *testing.T) {
	t.Run("BeforeCreate sets primary keys and indexes", func(t *testing.T) {
		a := &Actor{
			Username:      "alice",
			FollowerCount: 12,
			Actor:         &activitypub.Actor{Name: "Alice"},
		}
		before := time.Now()
		require.NoError(t, a.BeforeCreate())
		assert.Equal(t, "ACTOR#alice", a.PK)
		assert.Equal(t, SKProfile, a.SK)
		assert.WithinDuration(t, before, a.CreatedAt, 2*time.Second)
		assert.WithinDuration(t, before, a.UpdatedAt, 2*time.Second)
		assert.Equal(t, "USERNAME_SEARCH#al", a.GSI1PK)
		assert.Equal(t, "alice", a.GSI1SK)
		assert.Equal(t, "NAME_SEARCH#al", a.GSI2PK)
		assert.Equal(t, "alice#alice", a.GSI2SK)
		assert.Equal(t, "ACTOR_RANK#10+", a.GSI4PK)
		assert.Equal(t, a.PK, a.GetPK())
		assert.Equal(t, a.SK, a.GetSK())
		assert.Equal(t, MainTableName, a.TableName())
	})

	t.Run("UpdateKeys validates username", func(t *testing.T) {
		a := &Actor{}
		assert.Error(t, a.UpdateKeys())
	})

	t.Run("ActorField and ActorMetadata TableName", func(t *testing.T) {
		assert.Equal(t, MainTableName, (ActorField{}).TableName())
		assert.Equal(t, MainTableName, (ActorMetadata{}).TableName())
	})
}

func TestRecoveryModels_BasicKeying(t *testing.T) {
	trustee := &Trustee{Username: "alice", ActorID: "friend@remote"}
	require.NoError(t, trustee.UpdateKeys())
	assert.Equal(t, "USER#alice", trustee.PK)
	assert.Equal(t, "TRUSTEE#friend@remote", trustee.SK)
	assert.Equal(t, trustee.PK, trustee.GetPK())
	assert.Equal(t, trustee.SK, trustee.GetSK())
	assert.Equal(t, MainTableName, trustee.TableName())

	initiatedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	expiresAt := initiatedAt.Add(time.Hour)
	req := &RecoveryRequest{ID: "r1", Username: "alice", InitiatedAt: initiatedAt, ExpiresAt: expiresAt}
	require.NoError(t, req.UpdateKeys())
	assert.Equal(t, "RECOVERY#r1", req.PK)
	assert.Equal(t, "REQUEST", req.SK)
	assert.Equal(t, "USER#alice", req.GSI1PK)
	assert.Equal(t, "RECOVERY#2025-01-01T00:00:00Z", req.GSI1SK)
	assert.Equal(t, expiresAt.Unix(), req.TTL)
	assert.Equal(t, req.PK, req.GetPK())
	assert.Equal(t, req.SK, req.GetSK())
	assert.Equal(t, MainTableName, req.TableName())

	code := &RecoveryCode{Username: "alice", Position: 2}
	require.NoError(t, code.UpdateKeys())
	assert.Equal(t, "USER#alice", code.PK)
	assert.Equal(t, "RECOVERY_CODE#2", code.SK)
	assert.Equal(t, code.PK, code.GetPK())
	assert.Equal(t, code.SK, code.GetSK())
	assert.Equal(t, MainTableName, code.TableName())

	token := &RecoveryToken{PK: "RECOVERY_TOKEN#t1", CreatedAt: initiatedAt}
	require.NoError(t, token.UpdateKeys())
	assert.Equal(t, SKToken, token.SK)
	assert.Equal(t, initiatedAt.Add(24*time.Hour).Unix(), token.TTL)
	assert.Equal(t, token.PK, token.GetPK())
	assert.Equal(t, token.SK, token.GetSK())
	assert.Equal(t, MainTableName, token.TableName())
}

func TestStreamingCloudWatchMetrics_QualitySelectionAndAdaptation(t *testing.T) {
	t.Run("GetBestQuality falls back when QualityMetrics missing", func(t *testing.T) {
		s := &StreamingCloudWatchMetrics{}
		assert.Equal(t, defaultVideoQuality, s.GetBestQuality())
	})

	t.Run("GetBestQuality selects max score deterministically", func(t *testing.T) {
		s := &StreamingCloudWatchMetrics{
			QualityMetrics: map[string]QualityMetric{
				"480p":  {Quality: "480p", BufferingRate: 0.1, AverageLatencyMs: 900, ViewerPercentage: 0.1},
				"1080p": {Quality: "1080p", BufferingRate: 0.5, AverageLatencyMs: 1000, ViewerPercentage: 0.5},
				"720p":  {Quality: "720p", BufferingRate: 0.01, AverageLatencyMs: 100, ViewerPercentage: 0.9, BitrateUtilization: 0.95},
			},
		}
		assert.Equal(t, "720p", s.GetBestQuality())
	})

	t.Run("GetBestRegionQuality prefers region metric and falls back to overall best", func(t *testing.T) {
		s := &StreamingCloudWatchMetrics{
			QualityMetrics: map[string]QualityMetric{
				"720p": {Quality: "720p", BufferingRate: 0.01, AverageLatencyMs: 100, ViewerPercentage: 0.9},
			},
			GeographicMetrics: map[string]GeographicMetric{
				"US": {PreferredQuality: "1080p"},
			},
		}
		assert.Equal(t, "1080p", s.GetBestRegionQuality("US"))
		assert.Equal(t, "720p", s.GetBestRegionQuality("EU"))

		s2 := &StreamingCloudWatchMetrics{}
		assert.Equal(t, defaultVideoQuality, s2.GetBestRegionQuality("US"))
	})

	t.Run("ShouldAdaptQuality handles missing metrics, downshifts, and upshifts", func(t *testing.T) {
		s := &StreamingCloudWatchMetrics{}
		adapt, next := s.ShouldAdaptQuality("720p")
		assert.False(t, adapt)
		assert.Equal(t, "720p", next)

		s.QualityMetrics = map[string]QualityMetric{
			"1080p": {Quality: "1080p", BufferingRate: 0.2, AverageLatencyMs: 100, ViewerPercentage: 0.5},
			"720p":  {Quality: "720p", BufferingRate: 0.01, AverageLatencyMs: 100, ViewerPercentage: 0.9, BitrateUtilization: 0.95},
			"480p":  {Quality: "480p", BufferingRate: 0.5, AverageLatencyMs: 3000, ViewerPercentage: 0.1},
		}

		adapt, next = s.ShouldAdaptQuality("missing")
		assert.True(t, adapt)
		assert.Equal(t, "720p", next)

		adapt, next = s.ShouldAdaptQuality("1080p")
		assert.True(t, adapt)
		assert.Equal(t, "720p", next)

		adapt, next = s.ShouldAdaptQuality("720p")
		assert.True(t, adapt)
		assert.Equal(t, "1080p", next)

		adapt, next = s.ShouldAdaptQuality("480p")
		assert.False(t, adapt)
		assert.Equal(t, "480p", next)
	})
}

func TestNotification_BeforeCreate_BeforeUpdate_UpdateKeys(t *testing.T) {
	t.Run("BeforeCreate populates defaults, keys, and validates", func(t *testing.T) {
		n := &Notification{
			UserID:   "u1",
			Type:     "mention",
			ActorID:  "a1",
			TargetID: "s1",
		}
		require.NoError(t, n.BeforeCreate())
		assert.NotEmpty(t, n.ID)
		assert.Equal(t, "USER#u1", n.PK)
		assert.Contains(t, n.SK, "notif#")
		assert.False(t, n.IsRead)
		assert.False(t, n.PushSent)
		assert.Equal(t, 1, n.GroupCount)
		assert.True(t, n.ExpiresAt > 0)
		assert.NotEmpty(t, n.GroupKey)
		assert.Equal(t, n.PK, n.GetPK())
		assert.Equal(t, n.SK, n.GetSK())
	})

	t.Run("BeforeUpdate updates timestamps and re-validates", func(t *testing.T) {
		n := &Notification{
			ID:        "nid",
			UserID:    "u1",
			Type:      "mention",
			ActorID:   "a1",
			GroupKey:  "g",
			CreatedAt: time.Now().Add(-time.Hour),
		}
		require.NoError(t, n.BeforeUpdate())
		assert.True(t, n.UpdatedAt.After(n.CreatedAt))
	})

	t.Run("UpdateKeys validates required fields", func(t *testing.T) {
		n := &Notification{}
		assert.Error(t, n.UpdateKeys())
		n.UserID = "u1"
		assert.Error(t, n.UpdateKeys())

		n.ID = "nid"
		n.CreatedAt = time.Now()
		require.NoError(t, n.UpdateKeys())
		assert.Equal(t, "USER#u1", n.PK)
		assert.Contains(t, n.SK, "notif#")
	})

	t.Run("setupGSIKeys clears actor index when ActorID missing", func(t *testing.T) {
		n := &Notification{ID: "nid", UserID: "u1", Type: "mention", GroupKey: "g", CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
		n.setupGSIKeys()
		assert.Empty(t, n.GSI2PK)
		assert.Empty(t, n.GSI2SK)
	})

	t.Run("NotificationBuilder has TableName", func(t *testing.T) {
		assert.Equal(t, MainTableName, (NotificationBuilder{}).TableName())
	})
}

func TestImportBudget_WeeklyMonthlyBranchesAndAlerts(t *testing.T) {
	t.Run("BeforeCreate sets period end, TTL, and next reset for weekly/monthly", func(t *testing.T) {
		start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		weekly := &ImportBudget{Username: "alice", Period: PeriodWeekly, PeriodStart: start}
		require.NoError(t, weekly.BeforeCreate())
		assert.Equal(t, start.AddDate(0, 0, 7), weekly.PeriodEnd)
		assert.Equal(t, weekly.PeriodEnd, weekly.NextResetAt)
		assert.True(t, weekly.TTL > 0)

		monthly := &ImportBudget{Username: "alice", Period: PeriodMonthly, PeriodStart: start}
		require.NoError(t, monthly.BeforeCreate())
		assert.Equal(t, start.AddDate(0, 1, 0), monthly.PeriodEnd)
		assert.Equal(t, monthly.PeriodEnd, monthly.NextResetAt)
		assert.True(t, monthly.TTL > 0)
		assert.Equal(t, MainTableName, monthly.TableName())
	})

	t.Run("ShouldSendAlert respects toggle, last-sent, and usage thresholds", func(t *testing.T) {
		b := &ImportBudget{
			Username:              "alice",
			Period:                PeriodDaily,
			IsActive:              true,
			AlertSendingEnabled:   false,
			AlertThresholdPercent: 50,
			ImportLimitMicroCents: 100,
			CurrentImportCost:     90,
		}
		require.NoError(t, b.BeforeCreate())
		assert.False(t, b.ShouldSendAlert())

		b.AlertSendingEnabled = true
		assert.True(t, b.ShouldSendAlert())

		sent := time.Now()
		b.LastAlertSent = &sent
		assert.False(t, b.ShouldSendAlert())
	})

	t.Run("Over-limit checks and remaining budget helpers", func(t *testing.T) {
		b := &ImportBudget{IsActive: false, ImportLimitMicroCents: 100, CurrentImportCost: 90}
		assert.False(t, b.IsImportOverLimit(20))

		b.IsActive = true
		assert.True(t, b.IsImportOverLimit(20))

		b.ExportLimitMicroCents = 100
		b.CurrentExportCost = 110
		assert.Equal(t, int64(0), b.GetRemainingExportBudget())

		b.CombinedLimitMicroCents = 0
		assert.Equal(t, int64(-1), b.GetRemainingCombinedBudget())
	})

	t.Run("BeforeUpdate refreshes UpdatedAt and keys", func(t *testing.T) {
		b := &ImportBudget{Username: "alice", Period: PeriodDaily}
		require.NoError(t, b.BeforeCreate())
		old := b.UpdatedAt
		require.NoError(t, b.BeforeUpdate())
		assert.True(t, b.UpdatedAt.After(old))
		assert.Equal(t, "USER_BUDGET#alice#"+PeriodDaily, b.PK)
	})
}

func TestInstanceConfig_MoreCoverage(t *testing.T) {
	c := &InstanceConfig{PK: instanceConfigPK, SK: "RULES"}
	require.NoError(t, c.UpdateKeys())
	assert.Equal(t, c.PK, c.GetPK())
	assert.Equal(t, c.SK, c.GetSK())
	assert.Equal(t, MainTableName, c.TableName())

	ai := &AIInstanceConfig{}
	require.NoError(t, ai.UpdateKeys())
	assert.Equal(t, ai.PK, ai.GetPK())
	assert.Equal(t, ai.SK, ai.GetSK())
}

func TestTrendingHashtag_UpdateKeysAndKeyAccessors(t *testing.T) {
	th := &TrendingHashtag{}
	assert.ErrorContains(t, th.UpdateKeys(), "date is required")
	th.Date = "2025-01-01"
	assert.ErrorContains(t, th.UpdateKeys(), "hashtag is required")
	th.Hashtag = "go"
	th.Score = 1.23
	require.NoError(t, th.UpdateKeys())
	assert.Equal(t, "TRENDING#2025-01-01", th.PK)
	assert.Contains(t, th.SK, "HASHTAG#")
	assert.Equal(t, th.PK, th.GSI8PK)
	assert.Equal(t, th.SK, th.GSI8SK)
	assert.Equal(t, th.PK, th.GetPK())
	assert.Equal(t, th.SK, th.GetSK())
	assert.Equal(t, MainTableName, th.TableName())
}
