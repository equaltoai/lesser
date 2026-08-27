package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// wave1469ScanForbiddingDB mirrors the newWave1469ScanForbiddingDB helper from
// instance_counts_scanfree_test.go (which concurrent edits may churn): any
// request-adjacent read that issues a DynamoDB Scan fails the test.
type wave1469ScanForbiddingDB struct {
	*fakedb.Fake
	scanCalls int
}

func (s *wave1469ScanForbiddingDB) Scan(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	s.scanCalls++
	return nil, errors.New("full-table scan forbidden on request path")
}

func newWave1469ScanForbiddingTestDB(t *testing.T, modelTypes ...any) (core.DB, *wave1469ScanForbiddingDB) {
	t.Helper()
	s := &wave1469ScanForbiddingDB{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, s)
	require.NoError(t, err)
	for _, m := range modelTypes {
		require.NoError(t, db.CreateTable(m))
	}
	return db, s
}

// This file pins the unbounded-scan elimination wave (#1469): every read
// path below must resolve through a keyed query (primary or GSI). Any
// regression that reintroduces a request-adjacent DynamoDB Scan fails the
// test through newWave1469ScanForbiddingDB.

func TestScanFreeWave_GetScheduledStatus(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.ScheduledStatus{})

	now := time.Now().UTC()
	scheduled := &models.ScheduledStatus{
		ID:          "sched-1",
		Username:    "alice",
		Status:      "hello",
		Visibility:  "public",
		ScheduledAt: now,
	}
	require.NoError(t, scheduled.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(scheduled).Create())

	repo := NewScheduledStatusRepository(db, "test-table", zap.NewNop(), nil)
	got, err := repo.GetScheduledStatus(ctx, "sched-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "sched-1", got.ID)
	require.Equal(t, "alice", got.Username)

	_, err = repo.GetScheduledStatus(ctx, "missing")
	require.Error(t, err)
	require.Zero(t, s.scanCalls, "GetScheduledStatus must never scan")
}

func TestScanFreeWave_DeleteAnnouncementDismissalCleanup(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t,
		&models.Announcement{}, &models.AnnouncementDismissal{}, &models.AnnouncementReaction{},
	)

	ann := &models.Announcement{ID: "ann-1", Content: "hello", CreatedBy: "admin"}
	require.NoError(t, ann.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(ann).Create())

	// Seed two dismissals: one for ann-1, one for ann-2 (must survive).
	for _, tc := range []struct{ username, announcementID string }{
		{"alice", "ann-1"},
		{"bob", "ann-2"},
	} {
		dismissal := &models.AnnouncementDismissal{
			Username:       tc.username,
			AnnouncementID: tc.announcementID,
		}
		require.NoError(t, dismissal.BeforeCreate()) // sets GSI1 keys
		require.NoError(t, db.WithContext(ctx).Model(dismissal).Create())
	}

	repo := NewAnnouncementRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.DeleteAnnouncement(ctx, "ann-1"))
	require.Zero(t, s.scanCalls, "DeleteAnnouncement cleanup must never scan")

	// ann-1 dismissal is gone; ann-2 dismissal survives.
	var gone models.AnnouncementDismissal
	err := db.WithContext(ctx).Model(&gone).
		Where("PK", "=", "USER#alice").
		Where("SK", "=", "ANNOUNCEMENT_DISMISSED#ann-1").
		First(&gone)
	require.Error(t, err)

	var kept models.AnnouncementDismissal
	require.NoError(t, db.WithContext(ctx).Model(&kept).
		Where("PK", "=", "USER#bob").
		Where("SK", "=", "ANNOUNCEMENT_DISMISSED#ann-2").
		First(&kept))
}

func TestScanFreeWave_ClearAPIRateLimitsForUser(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.APIRateLimit{})

	now := time.Now().UTC()
	// Two counters for user-1 (both get GSI1 keys via NewAPIRateLimit).
	u1a := models.NewAPIRateLimit("user-1", "/v1/notes", now)
	require.NoError(t, db.WithContext(ctx).Model(u1a).Create())
	u1b := models.NewAPIRateLimit("user-1", "/v1/home", now)
	require.NoError(t, db.WithContext(ctx).Model(u1b).Create())
	// One counter for user-2 (must survive).
	u2 := models.NewAPIRateLimit("user-2", "/v1/notes", now)
	require.NoError(t, db.WithContext(ctx).Model(u2).Create())

	repo := NewRateLimitRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.ClearAPIRateLimitsForUser(ctx, "user-1"))
	require.Zero(t, s.scanCalls, "ClearAPIRateLimitsForUser must never scan")

	var gone models.APIRateLimit
	require.Error(t, db.WithContext(ctx).Model(&gone).
		Where("PK", "=", u1a.PK).
		Where("SK", "=", u1a.SK).
		First(&gone))
	var kept models.APIRateLimit
	require.NoError(t, db.WithContext(ctx).Model(&kept).
		Where("PK", "=", u2.PK).
		Where("SK", "=", u2.SK).
		First(&kept))
}

func TestScanFreeWave_GetReviewerStats(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.ModerationReview{})

	// Reviews by reviewer-1 on two events; one review by another reviewer.
	created := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
	}
	seeds := []models.ModerationReview{
		{EventID: "ev-1", ReviewerID: "reviewer-1", Severity: "high", ReviewerRep: 0.9, Created: created[0]},
		{EventID: "ev-2", ReviewerID: "reviewer-1", Severity: "low", ReviewerRep: 0.2, Created: created[1]},
		{EventID: "ev-3", ReviewerID: "reviewer-2", Severity: "high", ReviewerRep: 0.8, Created: created[2]},
	}
	for i := range seeds {
		seeds[i].UpdateKeys()
		require.NoError(t, db.WithContext(ctx).Model(&seeds[i]).Create())
	}

	repo := NewModerationRepository(db, "test-table", zap.NewNop())
	stats, err := repo.GetReviewerStats(ctx, "reviewer-1")
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Equal(t, 2, stats.TotalReviews)
	require.Equal(t, 1, stats.AccurateReviews)
	require.Equal(t, 0.5, stats.AccuracyRate)
	require.Equal(t, 1, stats.ReviewsByCategory["high"])
	require.Equal(t, 1, stats.ReviewsByCategory["low"])
	require.Equal(t, created[1], stats.LastReviewAt)
	require.Zero(t, s.scanCalls, "GetReviewerStats must never scan")
}

func TestScanFreeWave_FilterKeywordStatusKeyedOps(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.FilterKeyword{}, &models.FilterStatus{})

	kw := &models.FilterKeyword{ID: "kw-1", FilterID: "filter-1", Keyword: "spam", WholeWord: true}
	require.NoError(t, kw.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(kw).Create())

	st := &models.FilterStatus{ID: "fs-1", FilterID: "filter-1", StatusID: "status-1"}
	require.NoError(t, st.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(st).Create())

	repo := NewModerationRepository(db, "test-table", zap.NewNop())

	require.NoError(t, repo.UpdateFilterKeyword(ctx, "filter-1", "kw-1", map[string]any{
		"keyword":    "ham",
		"whole_word": false,
	}))
	var updated models.FilterKeyword
	require.NoError(t, db.WithContext(ctx).Model(&updated).
		Where("PK", "=", "FILTER#filter-1").
		Where("SK", "=", "KEYWORD#kw-1").
		First(&updated))
	require.Equal(t, "ham", updated.Keyword)
	require.False(t, updated.WholeWord)

	require.NoError(t, repo.DeleteFilterKeyword(ctx, "filter-1", "kw-1"))
	require.Error(t, db.WithContext(ctx).Model(&models.FilterKeyword{}).
		Where("PK", "=", "FILTER#filter-1").
		Where("SK", "=", "KEYWORD#kw-1").
		First(&models.FilterKeyword{}))

	require.NoError(t, repo.DeleteFilterStatus(ctx, "filter-1", "status-1"))
	require.Error(t, db.WithContext(ctx).Model(&models.FilterStatus{}).
		Where("PK", "=", "FILTER#filter-1").
		Where("SK", "=", "STATUS#status-1").
		First(&models.FilterStatus{}))

	require.Zero(t, s.scanCalls, "filter keyword/status ops must never scan")
}

func TestScanFreeWave_QueryQualityChangeEvents(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.MediaAnalytics{})

	now := time.Now().UTC()

	// Two quality-change rows for media-1; one for media-2 (must not count).
	row := &models.MediaAnalytics{}
	row.SetQualityChange("media-1", "user-1", "480p", "720p")
	require.NoError(t, db.WithContext(ctx).Model(row).Create())

	row2 := &models.MediaAnalytics{}
	row2.SetQualityChange("media-1", "user-2", "720p", "1080p")
	require.NoError(t, db.WithContext(ctx).Model(row2).Create())

	row3 := &models.MediaAnalytics{}
	row3.SetQualityChange("media-2", "user-1", "480p", "720p")
	require.NoError(t, db.WithContext(ctx).Model(row3).Create())

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.queryQualityChangeEvents(ctx, "media-1", now.Add(-7*24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Zero(t, s.scanCalls, "quality-change query must never scan")
}

func TestScanFreeWave_GetModerationPatternsAll(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.ModerationPattern{})

	for _, tc := range []struct {
		id       string
		severity float64
		active   bool
	}{
		{"pat-1", 0.9, true},
		{"pat-2", 0.2, false},
	} {
		p := &models.ModerationPattern{
			PatternID: tc.id,
			Name:      tc.id,
			Type:      "keyword",
			Pattern:   "x",
			Severity:  tc.severity,
			Active:    tc.active,
			UpdatedAt: now(),
		}
		require.NoError(t, p.UpdateKeys()) // sets GSI3 global-listing keys
		require.NoError(t, db.WithContext(ctx).Model(p).Create())
	}

	repo := NewModerationRepository(db, "test-table", zap.NewNop())
	patterns, err := repo.GetModerationPatterns(ctx, false, "", 10)
	require.NoError(t, err)
	require.Len(t, patterns, 2)
	require.Zero(t, s.scanCalls, "GetModerationPatterns all-branch must never scan")
}

func TestScanFreeWave_GetPatternStatistics(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.EnhancedModerationPattern{})

	for i, id := range []string{"p1", "p2"} {
		p := &models.EnhancedModerationPattern{
			PatternID:      id,
			PatternType:    "text",
			PatternContent: "x",
			Category:       "spam",
			Severity:       "low",
			Priority:       1,
			Active:         i == 0,
			MatchCount:     int64(i * 5),
			Effectiveness:  0.5,
		}
		require.NoError(t, p.UpdateKeys()) // sets GSI4 global-listing keys
		require.NoError(t, db.WithContext(ctx).Model(p).Create())
	}

	repo := NewEnhancedPatternRepository(db, "table", zap.NewNop(), nil)
	stats, err := repo.GetPatternStatistics(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, stats["total_patterns"])
	require.Equal(t, 1, stats["active_patterns"])
	require.Zero(t, s.scanCalls, "GetPatternStatistics must never scan")
}

func now() time.Time { return time.Now().UTC() }
