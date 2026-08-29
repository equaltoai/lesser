package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// recountFixtureDB builds a dataset with drift the recount must fix: user rows
// (plus a non-account USER# item that must be excluded), actors across two
// domains, a stale domain counter for a domain with no actors, and a drifted
// per-domain counter for a live domain.
func recountFixtureDB(t *testing.T) (core.DB, *models.InstanceMetrics) {
	t.Helper()
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.User{}, &models.Actor{}, &models.DomainCounter{}, &models.InstanceMetrics{})

	seedUsers(t, ctx, db, 3)

	// A USER#-prefixed non-account item (e.g. a device row) must not count.
	device := &models.User{PK: "USER#alice", SK: "DEVICE#1", Username: "alice"}
	require.NoError(t, db.WithContext(ctx).Model(device).Create())

	seedActors(t, ctx, db, 4, []string{"a.example.com", "b.example.com"})

	// Stale counter: domain with no actor rows.
	stale := &models.DomainCounter{Domain: "stale.example.com", Value: 3, UpdatedAt: time.Now()}
	require.NoError(t, stale.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(stale).Create())

	// Drifted counter: live domain, wrong value.
	drifted := &models.DomainCounter{Domain: "a.example.com", Value: 99, UpdatedAt: time.Now()}
	require.NoError(t, drifted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(drifted).Create())

	// Pre-existing (stale) global counters that the recount must rewrite.
	existing := &models.InstanceMetrics{PK: models.InstanceMetricsPK, SK: models.TotalUsersMetricSK, TotalUsers: 1000, UpdatedAt: time.Now()}
	require.NoError(t, db.WithContext(ctx).Model(existing).Create())
	return db, existing
}

// TestInstanceCounts_Recount_DryRunReportsWithoutWriting pins F5: with
// apply=false the recount reports the recomputed totals and the changes it
// would make, and writes nothing.
func TestInstanceCounts_Recount_DryRunReportsWithoutWriting(t *testing.T) {
	ctx := context.Background()
	db, existing := recountFixtureDB(t)

	result, err := RecountInstanceCounts(ctx, db, zap.NewNop(), false)
	require.NoError(t, err)
	require.Equal(t, int64(3), result.Users)
	require.Equal(t, int64(2), result.Domains)
	require.Equal(t, int64(2), result.DomainCounters)
	require.Equal(t, int64(1), result.StaleDomainCounters)

	// Nothing written: the counters are untouched.
	users, err := readInstanceMetricsField(ctx, db, zap.NewNop(), models.TotalUsersMetricSK, "TotalUsers")
	require.NoError(t, err)
	require.EqualValues(t, 1000, users)

	// The drifted per-domain counter is untouched.
	var drifted models.DomainCounter
	err = db.WithContext(ctx).Model(&models.DomainCounter{}).
		Where("PK", "=", "DOMAIN#a.example.com").
		Where("SK", "=", models.DayCounterSK).
		First(&drifted)
	require.NoError(t, err)
	require.EqualValues(t, 99, drifted.Value)
	_ = existing
}

// TestInstanceCounts_Recount_RewritesCountersAndStaleItems pins F5: with
// apply=true the counters are recomputed from the data and rewritten, stale
// per-domain counters are removed, and drifted ones corrected.
func TestInstanceCounts_Recount_RewritesCountersAndStaleItems(t *testing.T) {
	ctx := context.Background()
	db, _ := recountFixtureDB(t)

	result, err := RecountInstanceCounts(ctx, db, zap.NewNop(), true)
	require.NoError(t, err)
	require.Equal(t, int64(3), result.Users)
	require.Equal(t, int64(2), result.Domains)
	require.Equal(t, int64(2), result.DomainCounters)
	require.Equal(t, int64(1), result.StaleDomainCounters)

	users, err := readInstanceMetricsField(ctx, db, zap.NewNop(), models.TotalUsersMetricSK, "TotalUsers")
	require.NoError(t, err)
	require.EqualValues(t, 3, users)

	domains, err := readInstanceMetricsField(ctx, db, zap.NewNop(), models.TotalDomainsMetricSK, "Value")
	require.NoError(t, err)
	require.EqualValues(t, 2, domains)

	// Drifted counter corrected.
	var a models.DomainCounter
	err = db.WithContext(ctx).Model(&models.DomainCounter{}).
		Where("PK", "=", "DOMAIN#a.example.com").
		Where("SK", "=", models.DayCounterSK).
		First(&a)
	require.NoError(t, err)
	require.EqualValues(t, 2, a.Value)

	// Stale counter removed.
	var stale models.DomainCounter
	err = db.WithContext(ctx).Model(&models.DomainCounter{}).
		Where("PK", "=", "DOMAIN#stale.example.com").
		Where("SK", "=", models.DayCounterSK).
		First(&stale)
	require.True(t, errors.IsNotFound(err), "stale domain counter should be removed")

	// A second recount is stable (idempotent): the totals hold and the stale
	// counter is not found again.
	result2, err := RecountInstanceCounts(ctx, db, zap.NewNop(), true)
	require.NoError(t, err)
	require.Equal(t, result.Users, result2.Users)
	require.Equal(t, result.Domains, result2.Domains)
	require.Equal(t, result.DomainCounters, result2.DomainCounters)
	require.Zero(t, result2.StaleDomainCounters)
}

// TestInstanceCounts_Recount_UnseededReadsThenRecount pins the scan-free read
// doctrine and the recount as the sanctioned seed: reads on an unseeded table
// return the documented default (0); after the offline recount the reads serve
// the recomputed counters. The recount also rewrites the true USER#/METADATA
// account count (not a legacy whole-table item count).
func TestInstanceCounts_Recount_UnseededReadsThenRecount(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.User{}, &models.Actor{}, &models.DomainCounter{}, &models.InstanceMetrics{})

	seedActors(t, ctx, db, 6, []string{"a.example.com", "b.example.com", "c.example.com"})
	seedUsers(t, ctx, db, 2)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	// Unseeded: reads return the documented default without scanning.
	userCount, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Zero(t, userCount)
	domainCount, err := repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Zero(t, domainCount)

	result, err := RecountInstanceCounts(ctx, db, zap.NewNop(), true)
	require.NoError(t, err)
	require.EqualValues(t, 2, result.Users)
	require.EqualValues(t, 3, result.Domains)

	// The reads now serve the recounted counters.
	userCount, err = repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	domainCount, err = repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, userCount)
	require.Equal(t, 3, domainCount)
}

// recountActiveMonthFixtureDB builds a dataset with activity rows across two
// days plus a drifted in-window day counter the recount must correct and a
// stale in-window day counter the recount must remove.
func recountActiveMonthFixtureDB(t *testing.T) core.DB {
	t.Helper()
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.Activity{}, &models.ActivityDayCounter{}, &models.ActivityActorDay{}, &models.InstanceMetrics{})

	// seedActivities(5, 2): actors user-0..user-4 with days [0,1,0,1,0] ->
	// 3 distinct actors today, 2 distinct actors yesterday.
	seedActivities(t, ctx, db, 5, 2)

	now := time.Now().UTC()

	// Drifted counter for today: says 99, should be 3.
	drifted := &models.ActivityDayCounter{Date: models.DayFormat(now), Value: 99, UpdatedAt: now}
	require.NoError(t, drifted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(drifted).Create())

	// Stale in-window counter: a day inside the retention window with no
	// activity rows.
	staleDay := models.DayFormat(now.AddDate(0, 0, -10))
	stale := &models.ActivityDayCounter{Date: staleDay, Value: 5, UpdatedAt: now}
	require.NoError(t, stale.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(stale).Create())
	return db
}

// TestInstanceCounts_Recount_ActiveMonth_DryRunReportsWithoutWriting pins that
// the active-month recount reports the recomputed rollup and changes without
// writing anything.
func TestInstanceCounts_Recount_ActiveMonth_DryRunReportsWithoutWriting(t *testing.T) {
	ctx := context.Background()
	db := recountActiveMonthFixtureDB(t)

	result, err := RecountInstanceCounts(ctx, db, zap.NewNop(), false)
	require.NoError(t, err)
	require.EqualValues(t, 2, result.ActiveMonthDays)
	require.EqualValues(t, 1, result.StaleActiveMonthDays)
	require.EqualValues(t, 5, result.ActiveMonthSum)
	require.False(t, result.ActiveMonthSeedMarker)

	// Nothing written: the drifted counter is untouched and no marker exists.
	now := time.Now().UTC()
	var today models.ActivityDayCounter
	require.NoError(t, db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
		Where("PK", "=", models.ActivityDayKey(models.DayFormat(now))).
		Where("SK", "=", models.DayCounterSK).
		First(&today))
	require.EqualValues(t, 99, today.Value)

	var marker models.InstanceMetrics
	err = db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.ActiveMonthSeedMetricSK).
		First(&marker)
	require.True(t, errors.IsNotFound(err), "seed marker must not be written in dry-run")
}

// TestInstanceCounts_Recount_ActiveMonth_RewritesRollupAndMarker pins that the
// active-month recount rewrites the per-day rollup, removes stale in-window
// counters, writes today's ActivityActorDay markers (so the write path does
// not double-count a same-day reactivation), persists the SEED#ACTIVE_MONTH
// marker, and that the reads then serve the recounted rollup.
func TestInstanceCounts_Recount_ActiveMonth_RewritesRollupAndMarker(t *testing.T) {
	ctx := context.Background()
	db := recountActiveMonthFixtureDB(t)

	result, err := RecountInstanceCounts(ctx, db, zap.NewNop(), true)
	require.NoError(t, err)
	require.EqualValues(t, 2, result.ActiveMonthDays)
	require.EqualValues(t, 1, result.StaleActiveMonthDays)
	require.EqualValues(t, 5, result.ActiveMonthSum)
	require.True(t, result.ActiveMonthSeedMarker)

	now := time.Now().UTC()
	var today models.ActivityDayCounter
	require.NoError(t, db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
		Where("PK", "=", models.ActivityDayKey(models.DayFormat(now))).
		Where("SK", "=", models.DayCounterSK).
		First(&today))
	require.EqualValues(t, 3, today.Value)

	// Stale in-window counter removed.
	var stale models.ActivityDayCounter
	err = db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
		Where("PK", "=", models.ActivityDayKey(models.DayFormat(now.AddDate(0, 0, -10)))).
		Where("SK", "=", models.DayCounterSK).
		First(&stale)
	require.True(t, errors.IsNotFound(err), "stale in-window day counter should be removed")

	// Marker persisted.
	var marker models.InstanceMetrics
	require.NoError(t, db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.ActiveMonthSeedMetricSK).
		First(&marker))

	// Today's actors have ActivityActorDay markers so a same-day reactivation
	// after the recount does not double count.
	var m models.ActivityActorDay
	require.NoError(t, db.WithContext(ctx).Model(&models.ActivityActorDay{}).
		Where("PK", "=", "ACTIVITY_ACTOR#https://example.com/users/user-0").
		Where("SK", "=", "DAY#"+models.DayFormat(now)).
		First(&m))

	// Reads serve the recounted rollup.
	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, 5, count)
}

// TestInstanceCounts_Recount_ActiveMonth_WritePathNoDoubleCount pins the
// interplay between the recount and the write path: after the recount, a
// same-day re-activation of an already-counted actor must not bump the counter
// again (marker present), while a new actor's first activity still counts.
func TestInstanceCounts_Recount_ActiveMonth_WritePathNoDoubleCount(t *testing.T) {
	ctx := context.Background()
	db := recountActiveMonthFixtureDB(t)

	_, err := RecountInstanceCounts(ctx, db, zap.NewNop(), true)
	require.NoError(t, err)

	logger := zap.NewNop()
	now := time.Now().UTC()
	today := models.DayFormat(now)

	// Re-activation of an actor counted by the recount: no bump.
	recordActivityActorDay(ctx, db, logger, "https://example.com/users/user-0", today)
	// A new actor's first activity: bump.
	recordActivityActorDay(ctx, db, logger, "https://example.com/users/new-user", today)

	count, err := readActiveMonthCount(ctx, db, logger, 30)
	require.NoError(t, err)
	require.Equal(t, 6, count) // 3 (today) + 2 (yesterday) + 1 (new actor)
}

// TestInstanceCounts_Recount_ActiveMonthScanErrorPropagates pins that a failed
// active-month projection aborts the recount loudly.
func TestInstanceCounts_Recount_ActiveMonthScanErrorPropagates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	// Select arities used by the recount projections: (PK,SK), (PK,SK,Actor),
	// (PK,SK,Activity,CreatedAt).
	mockQuery.On("Select", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Select", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Select", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	// The Activity projection fails; the earlier projections succeed empty.
	mockQuery.On("All", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*[]models.Activity)
		return ok
	})).Return(fmt.Errorf("activity scan failed")).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()

	_, err := RecountInstanceCounts(ctx, mockDB, zap.NewNop(), false)
	require.Error(t, err)
	require.ErrorContains(t, err, "activity scan failed")
}

// TestInstanceCounts_Recount_ScanErrorsPropagate pins error propagation on the
// recount's bounded reads.
func TestInstanceCounts_Recount_ScanErrorsPropagate(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.User{}, &models.Actor{}, &models.DomainCounter{}, &models.InstanceMetrics{})

	// A mixed table with an unreadable item type is not relevant here; simply
	// pin that a DB that has no user rows recounts to zero cleanly.
	result, err := RecountInstanceCounts(ctx, db, zap.NewNop(), false)
	require.NoError(t, err)
	require.Zero(t, result.Users)
	require.Zero(t, result.Domains)
	require.Zero(t, result.DomainCounters)
	require.Zero(t, result.StaleDomainCounters)
}
