package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// This file pins the unbounded-scan elimination wave (#1469, batch E): every
// event-path read below — moderation engine, trend aggregator, federation
// aggregator, threat intel, instance health, notification cascade — must
// resolve through a keyed query (primary or GSI on the pre-provisioned
// gsi1–gsi8 slots). Any regression that reintroduces a DynamoDB Scan fails
// the test through newWave1469ScanForbiddingTestDB (the fake overrides the
// DynamoDB client Scan method itself). Batch E rerouted 12 key-less All sites
// and converted 4 literal-.Scan files; per-site key shapes and legacy-row
// consequences are documented in
// docs/architecture/dynamodb-scan-inventory.md.

// ---------------------------------------------------------------------------
// MODERATION ENGINE
// ---------------------------------------------------------------------------

// 1) GetPatterns — keyed gsi3 (MODERATION_PATTERNS#ALL, batch A shape).
func TestScanFreeWave_Event_GetPatterns(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.ModerationPattern{})

	now := time.Now().UTC()
	for _, id := range []string{"pat-1", "pat-2"} {
		p := &models.ModerationPattern{PatternID: id, Name: id, Type: "keyword", Pattern: "x", Severity: 0.8, Active: true, Category: "spam", UpdatedAt: now}
		require.NoError(t, p.UpdateKeys()) // sets gsi3 MODERATION_PATTERNS#ALL
		require.NoError(t, db.WithContext(ctx).Model(p).Create())
	}

	repo := NewPatternRepository(db, "test-table", zap.NewNop(), nil)
	got, err := repo.GetPatterns(ctx, "spam", true)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Zero(t, s.scanCalls, "GetPatterns must not scan")
}

// 2) GetPendingModerationCount — reports via GSI4 assignee index, flags via
// keyed GSI2 (FLAG_STATUS#pending).
func TestScanFreeWave_Event_GetPendingModerationCount(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.Report{}, &models.Flag{})

	now := time.Now()
	open := &models.Report{ID: "r1", ReporterID: "u1", Status: string(storage.ReportStatusOpen), AssignedTo: "mod-1", CreatedAt: now}
	open.UpdateKeys() // sets GSI4 ASSIGNED#mod-1 / open#REPORT#...
	require.NoError(t, db.WithContext(ctx).Model(open).Create())

	otherMod := &models.Report{ID: "r2", ReporterID: "u2", Status: string(storage.ReportStatusOpen), AssignedTo: "mod-2", CreatedAt: now}
	otherMod.UpdateKeys()
	require.NoError(t, db.WithContext(ctx).Model(otherMod).Create())

	flag := &models.Flag{ID: "f1", Actor: "u1", Object: []string{"obj-1"}, Status: string(storage.FlagStatusPending), Published: now}
	flag.UpdateKeys() // sets GSI2 FLAG_STATUS#pending
	require.NoError(t, db.WithContext(ctx).Model(flag).Create())

	repo := NewModerationRepository(db, "test-table", zap.NewNop())
	count, err := repo.GetPendingModerationCount(ctx, "mod-1")
	require.NoError(t, err)
	require.Equal(t, 1, count, "only mod-1's open assigned report counts")
	require.Zero(t, s.scanCalls, "GetPendingModerationCount must not scan")
}

// 2b) UnassignReport must REMOVE the GSI4 assignee keys. tabletheory v3.0.6's
// implicit Update() skips empty omitempty attributes, so after UpdateKeys
// zeroes gsi4PK/gsi4SK an unassigned report would otherwise keep its stale
// ASSIGNED#<mod> partition entry and overcount GetPendingModerationCount.
// Probe (adversary shape): assign → unassign → count must be 0 and the stored
// item must not carry the stale keys; UpdateReportStatus-while-unassigned
// must keep them gone; re-assign + status change must move the sort key.
func TestScanFreeWave_Event_UnassignReportClearsGSI4(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.Report{}, &models.Flag{})
	repo := NewModerationRepository(db, "test-table", zap.NewNop())

	require.NoError(t, repo.CreateReport(ctx, &storage.Report{ID: "r1", ReporterID: "u1", TargetAccountID: "u2", Status: "open"}))
	require.NoError(t, repo.AssignReport(ctx, "r1", "mod-1"))

	count, err := repo.GetPendingModerationCount(ctx, "mod-1")
	require.NoError(t, err)
	require.Equal(t, 1, count, "assigned open report counts for its assignee")

	// Unassign → the stored item must not carry the stale GSI4 keys.
	require.NoError(t, repo.UnassignReport(ctx, "r1"))
	var unassigned models.Report
	require.NoError(t, db.WithContext(ctx).Model(&unassigned).
		Where("PK", "=", "REPORT#r1").
		Where("SK", "=", "REPORT").
		First(&unassigned))
	require.Empty(t, unassigned.GSI4PK, "unassigned report must not keep stale gsi4PK")
	require.Empty(t, unassigned.GSI4SK, "unassigned report must not keep stale gsi4SK")

	count, err = repo.GetPendingModerationCount(ctx, "mod-1")
	require.NoError(t, err)
	require.Zero(t, count, "unassigned report must not overcount its former assignee")

	// UpdateReportStatus-while-unassigned must not resurrect the stale keys.
	require.NoError(t, repo.UpdateReportStatus(ctx, "r1", storage.ReportStatusResolved, "", "mod-1"))
	var afterStatus models.Report
	require.NoError(t, db.WithContext(ctx).Model(&afterStatus).
		Where("PK", "=", "REPORT#r1").
		Where("SK", "=", "REPORT").
		First(&afterStatus))
	require.Empty(t, afterStatus.GSI4PK, "status update on unassigned report must not restore gsi4PK")
	require.Empty(t, afterStatus.GSI4SK, "status update on unassigned report must not restore gsi4SK")

	count, err = repo.GetPendingModerationCount(ctx, "mod-1")
	require.NoError(t, err)
	require.Zero(t, count, "status update must not re-count an unassigned report")

	// Re-assign + status change moves the GSI4 sort key to the new status.
	require.NoError(t, repo.AssignReport(ctx, "r1", "mod-1"))
	require.NoError(t, repo.UpdateReportStatus(ctx, "r1", storage.ReportStatusInProgress, "", "mod-1"))
	var reassigned models.Report
	require.NoError(t, db.WithContext(ctx).Model(&reassigned).
		Where("PK", "=", "REPORT#r1").
		Where("SK", "=", "REPORT").
		First(&reassigned))
	require.Equal(t, "ASSIGNED#mod-1", reassigned.GSI4PK)
	require.Contains(t, reassigned.GSI4SK, "in_progress#REPORT#")

	count, err = repo.GetPendingModerationCount(ctx, "mod-1")
	require.NoError(t, err)
	require.Equal(t, 1, count, "in-progress assigned report counts for its assignee")
	require.Zero(t, s.scanCalls, "report assignment lifecycle must not scan")
}

// 3) GetModerationDecisionsByModerator — keyed gsi1 (REVIEWER#<reviewerID>).
func TestScanFreeWave_Event_GetModerationDecisionsByModerator(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.ModerationReview{})

	now := time.Now()
	review := &models.ModerationReview{ID: "rev-1", EventID: "evt-1", ReviewerID: "mod-1", Action: "remove", Severity: "high", Created: now}
	review.UpdateKeys() // sets GSI1 REVIEWER#mod-1 / TIME#...#REVIEW#evt-1
	require.NoError(t, db.WithContext(ctx).Model(review).Create())

	repo := NewModerationRepository(db, "test-table", zap.NewNop())
	got, err := repo.GetModerationDecisionsByModerator(ctx, "mod-1", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "rev-1", got[0].ID)
	require.Zero(t, s.scanCalls, "GetModerationDecisionsByModerator must not scan")
}

// ---------------------------------------------------------------------------
// ANALYTICS / TRENDS
// ---------------------------------------------------------------------------

// 4a) GetRecentHashtags — keyed gsi1 (HASHTAGS#ALL / <LastUsed>#<name>).
func TestScanFreeWave_Event_GetRecentHashtags(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.Hashtag{})

	now := time.Now().UTC()
	for _, tc := range []struct {
		name     string
		lastUsed time.Time
	}{
		{"golang", now},
		{"python", now.Add(-48 * time.Hour)}, // outside the window
	} {
		h := &models.Hashtag{Name: tc.name, LastUsed: tc.lastUsed, UsageCount: 10}
		require.NoError(t, h.UpdateKeys()) // sets GSI1 HASHTAGS#ALL
		require.NoError(t, db.WithContext(ctx).Model(h).Create())
	}

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	got, err := repo.GetRecentHashtags(ctx, now.Add(-24*time.Hour), 20)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "golang", got[0].Name)
	require.Zero(t, s.scanCalls, "GetRecentHashtags must not scan")
}

// 4b) GetRecentStatusesWithEngagement — keyed gsi1 (ENGAGEMENTS#ALL).
func TestScanFreeWave_Event_GetRecentStatusesWithEngagement(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.StatusEngagement{})

	now := time.Now().UTC()
	for _, tc := range []struct {
		id      string
		engaged time.Time
	}{
		{"status-1", now},
		{"status-2", now.Add(-48 * time.Hour)}, // outside the window
	} {
		e := &models.StatusEngagement{StatusID: tc.id, EngagementType: "like", UserID: "user-1", EngagedAt: tc.engaged}
		require.NoError(t, e.UpdateKeys()) // sets GSI1 ENGAGEMENTS#ALL
		require.NoError(t, db.WithContext(ctx).Model(e).Create())
	}

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	got, err := repo.GetRecentStatusesWithEngagement(ctx, now.Add(-24*time.Hour), 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "status-1", got[0].ID)
	require.Zero(t, s.scanCalls, "GetRecentStatusesWithEngagement must not scan")
}

// 4c) GetRecentLinks — keyed gsi1 (LINK_SHARES#ALL).
func TestScanFreeWave_Event_GetRecentLinks(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.LinkShare{})

	now := time.Now().UTC()
	for _, tc := range []struct {
		url    string
		shared time.Time
	}{
		{"https://example.com/a", now},
		{"https://example.com/b", now.Add(-48 * time.Hour)}, // outside the window
	} {
		l := &models.LinkShare{URL: tc.url, StatusID: "s1", AuthorID: "u1", SharedAt: tc.shared}
		require.NoError(t, l.UpdateKeys()) // sets GSI1 LINK_SHARES#ALL
		require.NoError(t, db.WithContext(ctx).Model(l).Create())
	}

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	got, err := repo.GetRecentLinks(ctx, now.Add(-24*time.Hour), 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "https://example.com/a", got[0].URL)
	require.Zero(t, s.scanCalls, "GetRecentLinks must not scan")
}

// 5) GetPopularSearchQueries — delegates to GetTopQueries, keyed gsi8
// (POPULAR#<bucket>#<date> / COUNT#<count>#<hash>) on PopularQueryCounter.
func TestScanFreeWave_Event_GetPopularSearchQueries(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.PopularQueryCounter{})

	now := time.Now().UTC()
	counter := &models.PopularQueryCounter{
		QueryHash:    "abc",
		Query:        "golang",
		TimeBucket:   string(models.PeriodDaily),
		Date:         now.Format(common.DateFormat),
		Count:        7,
		UserCount:    3,
		AvgResults:   10,
		LastQueried:  now,
		FirstQueried: now,
		UpdatedAt:    now,
	}
	require.NoError(t, counter.UpdateKeys()) // sets GSI8 POPULAR#daily#<date>
	require.NoError(t, db.WithContext(ctx).Model(counter).Create())

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	got, err := repo.GetPopularSearchQueries(ctx, 10, 24*time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "golang", got[0].Query)
	require.Zero(t, s.scanCalls, "GetPopularSearchQueries must not scan")
}

// 6) getCandidateHashtags — keyed gsi1 (HASHTAGS#ALL) via the trending engine.
func TestScanFreeWave_Event_GetCandidateHashtags(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.Hashtag{})

	now := time.Now().UTC()
	h := &models.Hashtag{Name: "golang", LastUsed: now, UsageCount: 10}
	require.NoError(t, h.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(h).Create())

	engine := NewTrendingEngine(db, zap.NewNop())
	engine.config.CandidateLimit = 500
	engine.config.MinimumUsage = 1

	got, err := engine.getCandidateHashtags(ctx, now.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "golang", got[0].Name)
	require.Zero(t, s.scanCalls, "getCandidateHashtags must not scan")
}

// 7) Media analytics reads — keyed gsi1 (DATE#<date>) / gsi2 (VARIANT#<key>).
func TestScanFreeWave_Event_MediaAnalyticsReads(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.MediaAnalytics{})

	now := time.Now().UTC()
	row := &models.MediaAnalytics{}
	row.SetManifestGeneration("media-1", "hls", 10.5) // sets Date + GSI1 DATE#<date>
	row.DominantVariant = "720p"
	row.TotalBandwidthBytes = 100 // bandwidth filter requires > 0
	_ = row.UpdateKeys()          // refreshes GSI1/GSI2
	require.NoError(t, db.WithContext(ctx).Model(row).Create())

	repo := NewMediaAnalyticsRepository(db, "test-table", zap.NewNop(), nil)

	byDate, err := repo.GetMediaAnalyticsByDate(ctx, now.Format(common.DateFormat))
	require.NoError(t, err)
	require.Len(t, byDate, 1)

	byVariant, err := repo.GetMediaAnalyticsByVariant(ctx, "720p")
	require.NoError(t, err)
	require.Len(t, byVariant, 1)

	byRange, err := repo.GetMediaAnalyticsByTimeRange(ctx, "media-1", now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, byRange, 1)

	allRange, err := repo.GetAllMediaAnalyticsByTimeRange(ctx, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, allRange, 1)

	bandwidth, err := repo.GetBandwidthByTimeRange(ctx, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, bandwidth, 1)

	require.Zero(t, s.scanCalls, "media analytics reads must not scan")
}

// 8) Media metadata reads — keyed gsi1 (STATUS#<status>).
func TestScanFreeWave_Event_MediaMetadataReads(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.MediaMetadata{})

	now := time.Now().UTC()
	pending := &models.MediaMetadata{MediaID: "media-1", Status: "pending", ProcessedAt: now}
	require.NoError(t, pending.UpdateKeys()) // sets GSI1 STATUS#pending
	require.NoError(t, db.WithContext(ctx).Model(pending).Create())

	failed := &models.MediaMetadata{MediaID: "media-2", Status: "failed", ProcessedAt: now.Add(-10 * 24 * time.Hour)}
	require.NoError(t, failed.UpdateKeys()) // sets GSI1 STATUS#failed, PROCESSED#<old>
	require.NoError(t, db.WithContext(ctx).Model(failed).Create())

	repo := NewMediaMetadataRepository(db, "test-table", zap.NewNop(), nil)

	got, err := repo.GetMediaMetadataByStatus(ctx, "pending", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)

	require.NoError(t, repo.CleanupExpiredMetadata(ctx)) // keyed STATUS#failed + gsi1SK < cutoff

	require.Zero(t, s.scanCalls, "media metadata reads must not scan")
}

// ---------------------------------------------------------------------------
// FEDERATION-ADJACENT
// ---------------------------------------------------------------------------

// 9) GetRecentActivities — keyed gsi3 (FED_ACTIVITY#ALL).
func TestScanFreeWave_Event_GetRecentActivities(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.FederationActivity{})

	now := time.Now().UTC()
	act := &models.FederationActivity{ID: "act-1", Domain: "remote.example", ActivityType: "Create", Timestamp: now}
	require.NoError(t, act.UpdateKeys()) // sets GSI3 FED_ACTIVITY#ALL
	require.NoError(t, db.WithContext(ctx).Model(act).Create())

	repo := NewFederationActivityRepository(db, "test-table", zap.NewNop(), nil)
	got, err := repo.GetRecentActivities(ctx, now.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "act-1", got[0].ID)
	require.Zero(t, s.scanCalls, "GetRecentActivities must not scan")
}

// ---------------------------------------------------------------------------
// OPS / ROUTING
// ---------------------------------------------------------------------------

// 10) LoadActiveThreats — keyed gsi2 (THREATS).
func TestScanFreeWave_Event_LoadActiveThreats(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.ThreatIntel{})

	now := time.Now().UTC()
	threat := &models.ThreatIntel{ID: "threat-1", ThreatType: "spam", Severity: "high", LastSeen: now, TTL: now.Add(time.Hour).Unix()}
	require.NoError(t, threat.UpdateKeys()) // sets GSI2 THREATS
	require.NoError(t, db.WithContext(ctx).Model(threat).Create())

	repo := NewThreatIntelRepository(db, "test-table", zap.NewNop(), nil)
	got, err := repo.LoadActiveThreats(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "threat-1", got[0].ID)
	require.Zero(t, s.scanCalls, "LoadActiveThreats must not scan")
}

// 11) GetUnhealthyInstances — keyed gsi1 (HEALTH_SUMMARY#1h).
func TestScanFreeWave_Event_GetUnhealthyInstances(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.InstanceHealthSummary{})

	unhealthy := models.NewInstanceHealthSummary("bad.example", time.Hour) // sets GSI1 HEALTH_SUMMARY#1h
	unhealthy.HealthScore = 30.0
	require.NoError(t, unhealthy.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(unhealthy).Create())

	healthy := models.NewInstanceHealthSummary("good.example", time.Hour)
	healthy.HealthScore = 95.0
	require.NoError(t, healthy.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(healthy).Create())

	repo := NewInstanceHealthRepository(db, "test-table", zap.NewNop(), nil)
	got, err := repo.GetUnhealthyInstances(ctx, 80.0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "bad.example", got[0])
	require.Zero(t, s.scanCalls, "GetUnhealthyInstances must not scan")
}

// 12) DeleteNotificationsByObject — keyed gsi5 (NOTIF_OBJECT#<targetID>) with
// cursor pagination; the cascade actually deletes (ObjectID filter previously
// matched nothing — Notification references objects via TargetID).
func TestScanFreeWave_Event_DeleteNotificationsByObject(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.Notification{})

	now := time.Now().UTC()
	seed := func(userID, notifID, targetID string) {
		n := &models.Notification{ID: notifID, UserID: userID, Type: "mention", ActorID: "https://remote.example/users/a1", TargetID: targetID, TargetType: "status", CreatedAt: now}
		require.NoError(t, n.BeforeCreate()) // sets PK/SK + GSI5 NOTIF_OBJECT#targetID
		require.NoError(t, db.WithContext(ctx).Model(n).Create())
	}
	seed("user-1", "n1", "obj-1")
	seed("user-2", "n2", "obj-1")
	seed("user-3", "n3", "obj-2") // must survive

	repo := NewNotificationRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.DeleteNotificationsByObject(ctx, "obj-1"))
	require.Zero(t, s.scanCalls, "DeleteNotificationsByObject must not scan")

	var kept models.Notification
	require.NoError(t, db.WithContext(ctx).Model(&kept).
		Where("PK", "=", "USER#user-3").
		Where("SK", "=", "notif#"+now.Format("20060102150405")+"#n3").
		First(&kept))
	require.Equal(t, "n3", kept.ID)
}

// ---------------------------------------------------------------------------
// WRITER MAINTENANCE — new key shapes are maintained by the production writers
// (UpdateKeys is called on every write; exhaustive writer enumeration per
// model is documented in docs/architecture/dynamodb-scan-inventory.md).
// ---------------------------------------------------------------------------

func TestScanFreeWave_Event_WritersMaintainNewKeys(t *testing.T) {
	ctx := context.Background()

	t.Run("RecordStatusEngagement maintains GSI1", func(t *testing.T) {
		db, _ := newWave1469ScanForbiddingTestDB(t, &models.StatusEngagement{})
		repo := NewTrendingRepository(db, zap.NewNop(), nil)
		require.NoError(t, repo.RecordStatusEngagement(ctx, "status-1", "like", "user-1"))

		var got models.StatusEngagement
		require.NoError(t, db.WithContext(ctx).Model(&got).
			Where("PK", "=", "STATUS_ENGAGEMENT#status-1").
			Where("SK", "begins_with", "like#").
			First(&got))
		require.Equal(t, "ENGAGEMENTS#ALL", got.GSI1PK)
	})

	t.Run("RecordLinkShare maintains GSI1", func(t *testing.T) {
		db, _ := newWave1469ScanForbiddingTestDB(t, &models.LinkShare{})
		repo := NewTrendingRepository(db, zap.NewNop(), nil)
		require.NoError(t, repo.RecordLinkShare(ctx, "https://example.com/a", "status-1", "user-1"))

		var got models.LinkShare
		require.NoError(t, db.WithContext(ctx).Model(&got).
			Where("PK", "=", "LINK_SHARE#https://example.com/a").
			Where("SK", "=", "STATUS#status-1").
			First(&got))
		require.Equal(t, "LINK_SHARES#ALL", got.GSI1PK)
	})

	t.Run("CreateReport + AssignReport maintain GSI4", func(t *testing.T) {
		db, _ := newWave1469ScanForbiddingTestDB(t, &models.Report{})
		repo := NewModerationRepository(db, "test-table", zap.NewNop())
		require.NoError(t, repo.CreateReport(ctx, &storage.Report{ID: "r1", ReporterID: "u1", TargetAccountID: "u2", Status: "open"}))
		require.NoError(t, repo.AssignReport(ctx, "r1", "mod-1"))

		var got models.Report
		require.NoError(t, db.WithContext(ctx).Model(&got).
			Where("PK", "=", "REPORT#r1").
			Where("SK", "=", "REPORT").
			First(&got))
		require.Equal(t, "ASSIGNED#mod-1", got.GSI4PK)
		require.Contains(t, got.GSI4SK, "open#REPORT#")
	})

	t.Run("RecordFederationActivity maintains GSI3", func(t *testing.T) {
		db, _ := newWave1469ScanForbiddingTestDB(t, &models.FederationActivity{})
		repo := NewFederationActivityRepository(db, "test-table", zap.NewNop(), nil)
		act := &models.FederationActivity{ID: "act-1", Domain: "remote.example", ActivityType: "Create", ActorID: "https://remote.example/users/alice", Timestamp: time.Now().UTC()}
		require.NoError(t, repo.RecordFederationActivity(ctx, act))

		var got models.FederationActivity
		require.NoError(t, db.WithContext(ctx).Model(&got).
			Where("PK", "=", "fed_activity#remote.example").
			Where("SK", "begins_with", "activity#").
			First(&got))
		require.Equal(t, "FED_ACTIVITY#ALL", got.GSI3PK)
	})

	t.Run("CreateNotification maintains GSI5", func(t *testing.T) {
		db, _ := newWave1469ScanForbiddingTestDB(t, &models.Notification{})
		repo := NewNotificationRepository(db, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.CreateNotification(ctx, &models.Notification{
			ID: "n1", UserID: "user-1", Type: "mention",
			ActorID: "https://remote.example/users/a1", TargetID: "obj-1", TargetType: "status",
		}))

		var got models.Notification
		require.NoError(t, db.WithContext(ctx).Model(&got).
			Where("PK", "=", "USER#user-1").
			Where("SK", "begins_with", "notif#").
			First(&got))
		require.Equal(t, "NOTIF_OBJECT#obj-1", got.GSI5PK)
	})
}

// SaveHealthSummary — the InstanceHealthSummary writer — maintains the GSI1
// window listing key on every save.
func TestScanFreeWave_Event_SaveHealthSummaryMaintainsGSI1(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.InstanceHealthSummary{})

	repo := NewInstanceHealthRepository(db, "test-table", zap.NewNop(), nil)
	summary := models.NewInstanceHealthSummary("remote.example", time.Hour)
	summary.HealthScore = 88.0
	require.NoError(t, repo.SaveHealthSummary(ctx, summary))

	var got models.InstanceHealthSummary
	require.NoError(t, db.WithContext(ctx).Model(&got).
		Where("PK", "=", "INSTANCE#remote.example").
		Where("SK", "=", "SUMMARY#1h").
		First(&got))
	require.Equal(t, "HEALTH_SUMMARY#1h", got.GSI1PK)
	require.Equal(t, "INSTANCE#remote.example", got.GSI1SK)
	require.Zero(t, s.scanCalls, "SaveHealthSummary must not scan")
}
