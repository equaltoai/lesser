package repositories

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// newInstanceCountsTestDB builds a fakedb-backed tabletheory DB with the given
// models registered on the shared main table.
func newInstanceCountsTestDB(t *testing.T, modelTypes ...any) (core.DB, *fakedb.Fake) {
	t.Helper()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	for _, m := range modelTypes {
		require.NoError(t, db.CreateTable(m))
	}
	return db, fake
}

// seedUsers writes n user rows directly (simulating pre-existing data that
// predates the counters).
func seedUsers(t *testing.T, ctx context.Context, db core.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		username := fmt.Sprintf("user-%d", i)
		user := &models.User{Username: username, Email: username + "@example.com", Role: "user"}
		require.NoError(t, user.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(user).Create())
	}
}

// seedActors writes n actor rows directly, distributing them across the given
// domains.
func seedActors(t *testing.T, ctx context.Context, db core.DB, n int, domains []string) {
	t.Helper()
	for i := 0; i < n; i++ {
		domain := domains[i%len(domains)]
		username := fmt.Sprintf("actor-%d", i)
		actor := &models.Actor{
			Username:  username,
			NumericID: fmt.Sprintf("num-%d", i),
			Actor: &activitypub.Actor{
				PreferredUsername: username,
				BaseObject:        activitypub.BaseObject{ID: "https://" + domain + "/users/" + username},
			},
		}
		require.NoError(t, actor.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(actor).Create())
	}
}

// seedActivities writes n activity rows directly with distinct actors and
// published times spread across the last daysCount days.
func seedActivities(t *testing.T, ctx context.Context, db core.DB, n, daysCount int) {
	t.Helper()
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		username := fmt.Sprintf("user-%d", i)
		actorID := "https://example.com/users/" + username
		published := now.AddDate(0, 0, -(i % daysCount))
		activity := &models.Activity{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        fmt.Sprintf("activity-%d", i),
					Type:      "Create",
					Published: &published,
				},
				Actor:  actorID,
				Object: map[string]any{"content": "hello"},
			},
			CreatedAt: published,
		}
		require.NoError(t, activity.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(activity).Create())
	}
}

// deleteAllUsers removes every user row (single-table scans can surface items
// of other entity types; guard on the USER# key prefix so counters survive).
func deleteAllUsers(t *testing.T, ctx context.Context, db core.DB) {
	t.Helper()
	var users []models.User
	require.NoError(t, db.WithContext(ctx).Model(&models.User{}).All(&users))
	for _, u := range users {
		if !strings.HasPrefix(u.PK, "USER#") {
			continue
		}
		require.NoError(t, db.WithContext(ctx).Model(&models.User{}).
			Where("PK", "=", u.PK).
			Where("SK", "=", u.SK).
			Delete())
	}
}

// deleteAllActors removes every actor row (guard on the ACTOR#/#PROFILE keys).
func deleteAllActors(t *testing.T, ctx context.Context, db core.DB) {
	t.Helper()
	var actors []models.Actor
	require.NoError(t, db.WithContext(ctx).Model(&models.Actor{}).All(&actors))
	for _, a := range actors {
		if !strings.HasPrefix(a.PK, "ACTOR#") || a.SK != "PROFILE" {
			continue
		}
		require.NoError(t, db.WithContext(ctx).Model(&models.Actor{}).
			Where("PK", "=", a.PK).
			Where("SK", "=", a.SK).
			Delete())
	}
}

// deleteAllActivities removes every activity row (guard on the ACTIVITY# SK).
func deleteAllActivities(t *testing.T, ctx context.Context, db core.DB) {
	t.Helper()
	var activities []models.Activity
	require.NoError(t, db.WithContext(ctx).Model(&models.Activity{}).All(&activities))
	for _, a := range activities {
		if !strings.HasPrefix(a.SK, "ACTIVITY#") {
			continue
		}
		require.NoError(t, db.WithContext(ctx).Model(&models.Activity{}).
			Where("PK", "=", a.PK).
			Where("SK", "=", a.SK).
			Delete())
	}
}

// seedTotalUsersMetric writes the TOTAL_USERS counter item directly.
func seedTotalUsersMetric(t *testing.T, ctx context.Context, db core.DB, value int64) {
	t.Helper()
	metric := &models.InstanceMetrics{
		PK:         models.InstanceMetricsPK,
		SK:         models.TotalUsersMetricSK,
		TotalUsers: value,
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, db.WithContext(ctx).Model(metric).Create())
}

// seedTotalDomainsMetric writes the TOTAL_DOMAINS counter item directly.
func seedTotalDomainsMetric(t *testing.T, ctx context.Context, db core.DB, value int64) {
	t.Helper()
	metric := &models.InstanceMetrics{
		PK:        models.InstanceMetricsPK,
		SK:        models.TotalDomainsMetricSK,
		Value:     value,
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.WithContext(ctx).Model(metric).Create())
}

// seedActiveMonthCounters writes a single day counter for today with the given
// value plus the seed marker, so the lazy seed does not overwrite the fixture.
func seedActiveMonthCounters(t *testing.T, ctx context.Context, db core.DB, total int64) {
	t.Helper()
	now := time.Now().UTC()
	day := models.DayFormat(now)
	counter := &models.ActivityDayCounter{Date: day, Value: total, UpdatedAt: now}
	require.NoError(t, counter.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(counter).Create())

	marker := &models.InstanceMetrics{
		PK:        models.InstanceMetricsPK,
		SK:        models.ActiveMonthSeedMetricSK,
		UpdatedAt: now,
	}
	require.NoError(t, db.WithContext(ctx).Model(marker).Create())
}

// TestInstanceCounts_TotalUsers_CounterWinsOverRows is the regression guard
// for issue #1467: with a seeded dataset well above the threshold, the public
// count read must come from the maintained counter item, never from fetching
// item bodies. A counter value of 42 over 500 user rows proves the read is
// O(1) and body-free.
func TestInstanceCounts_TotalUsers_CounterWinsOverRows(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.User{}, &models.InstanceMetrics{})

	seedUsers(t, ctx, db, 500)
	seedTotalUsersMetric(t, ctx, db, 42)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 42, count)
}

// TestInstanceCounts_TotalUsers_LazySeedOnce pins the one-time lazy seed: the
// first read computes the total from a scan and persists it; afterwards the
// read never touches item bodies (deleting every row leaves the count intact).
func TestInstanceCounts_TotalUsers_LazySeedOnce(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.User{}, &models.InstanceMetrics{})

	seedUsers(t, ctx, db, 500)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 500, count)

	// Counter now authoritative: removing every row must not change the read.
	deleteAllUsers(t, ctx, db)
	count, err = repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 500, count)

	// And the seed never runs twice.
	seedUsers(t, ctx, db, 3)
	count, err = repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 500, count)
}

// TestInstanceCounts_TotalUsers_Maintenance pins the write-path increment and
// decrement of the TOTAL_USERS counter.
func TestInstanceCounts_TotalUsers_Maintenance(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.User{}, &models.InstanceMetrics{})

	userRepo := NewUserRepository(db, models.MainTableName, zap.NewNop())
	userRepo.SetValidationService(nil)
	userRepo.SetPermissionService(nil)

	for i := 0; i < 3; i++ {
		require.NoError(t, userRepo.CreateUser(ctx, &storage.User{
			Username: fmt.Sprintf("alice-%d", i),
			Email:    fmt.Sprintf("alice-%d@example.com", i),
			Role:     "user",
		}))
	}

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)

	require.NoError(t, userRepo.DeleteUser(ctx, "alice-0"))
	count, err = repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

// TestInstanceCounts_Domains_CounterWinsOverRows is the domain-count variant
// of the no-body-read guard.
func TestInstanceCounts_Domains_CounterWinsOverRows(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.Actor{}, &models.InstanceMetrics{})

	seedActors(t, ctx, db, 500, []string{"a.example.com", "b.example.com"})
	seedTotalDomainsMetric(t, ctx, db, 7)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 7, count)
}

// TestInstanceCounts_Domains_LazySeed pins the domain seed (distinct hosts of
// actor records) and its one-time nature.
func TestInstanceCounts_Domains_LazySeed(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.Actor{}, &models.InstanceMetrics{})

	seedActors(t, ctx, db, 500, []string{"a.example.com", "b.example.com"})

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	deleteAllActors(t, ctx, db)
	count, err = repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

// TestInstanceCounts_Domains_Maintenance pins the per-domain tally and the
// global TOTAL_DOMAINS increment/decrement on first/last actor of a domain.
func TestInstanceCounts_Domains_Maintenance(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	db, _ := newInstanceCountsTestDB(t, &models.DomainCounter{}, &models.InstanceMetrics{})

	// Two actors in the same domain count as one domain.
	recordActorDomain(ctx, db, logger, "a.example.com")
	recordActorDomain(ctx, db, logger, "a.example.com")
	require.EqualValues(t, 1, mustReadDomains(t, ctx, db))

	// A second domain bumps the global counter.
	recordActorDomain(ctx, db, logger, "b.example.com")
	require.EqualValues(t, 2, mustReadDomains(t, ctx, db))

	// Removing one actor from a multi-actor domain keeps the domain.
	releaseActorDomain(ctx, db, logger, "a.example.com")
	require.EqualValues(t, 2, mustReadDomains(t, ctx, db))

	// Removing the last actor of a domain drops it.
	releaseActorDomain(ctx, db, logger, "a.example.com")
	releaseActorDomain(ctx, db, logger, "b.example.com")
	require.EqualValues(t, 0, mustReadDomains(t, ctx, db))
}

func mustReadDomains(t *testing.T, ctx context.Context, db core.DB) int64 {
	t.Helper()
	count, err := readTotalDomainsCount(ctx, db, zap.NewNop())
	require.NoError(t, err)
	return count
}

// TestInstanceCounts_ActiveMonth_CounterWinsOverRows is the active-month
// variant of the no-body-read guard.
func TestInstanceCounts_ActiveMonth_CounterWinsOverRows(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.Activity{}, &models.ActivityDayCounter{}, &models.InstanceMetrics{})

	seedActivities(t, ctx, db, 500, 30)
	seedActiveMonthCounters(t, ctx, db, 9)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, 9, count)
}

// TestInstanceCounts_ActiveMonth_LazySeed pins the active-month backfill and
// its one-time nature.
func TestInstanceCounts_ActiveMonth_LazySeed(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.Activity{}, &models.ActivityDayCounter{}, &models.InstanceMetrics{})

	seedActivities(t, ctx, db, 500, 30)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	// 500 distinct actors across the window.
	require.Equal(t, 500, count)

	deleteAllActivities(t, ctx, db)
	count, err = repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, 500, count)
}

// TestInstanceCounts_ActiveMonth_RollupMaintenance pins the write-path rollup:
// an actor counts once per UTC day, and re-activation on a later day counts
// again (the window is the SUM of per-day distinct counts).
func TestInstanceCounts_ActiveMonth_RollupMaintenance(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	db, _ := newInstanceCountsTestDB(t, &models.ActivityActorDay{}, &models.ActivityDayCounter{}, &models.InstanceMetrics{})

	now := time.Now().UTC()
	today := models.DayFormat(now)

	// Three distinct actors today.
	recordActivityActorDay(ctx, db, logger, "https://example.com/users/a", today)
	recordActivityActorDay(ctx, db, logger, "https://example.com/users/b", today)
	recordActivityActorDay(ctx, db, logger, "https://example.com/users/c", today)
	require.Equal(t, 3, mustReadActiveMonth(t, ctx, db, 30))

	// Same actor again today: no double count.
	recordActivityActorDay(ctx, db, logger, "https://example.com/users/a", today)
	require.Equal(t, 3, mustReadActiveMonth(t, ctx, db, 30))

	// Activity on a different day counts again (window sum semantics).
	yesterday := models.DayFormat(now.AddDate(0, 0, -1))
	recordActivityActorDay(ctx, db, logger, "https://example.com/users/a", yesterday)
	require.Equal(t, 4, mustReadActiveMonth(t, ctx, db, 30))

	// A 1-day window reads only today's counter.
	require.Equal(t, 3, mustReadActiveMonth(t, ctx, db, 1))
}

func mustReadActiveMonth(t *testing.T, ctx context.Context, db core.DB, days int) int {
	t.Helper()
	count, err := readActiveMonthCount(ctx, db, zap.NewNop(), days)
	require.NoError(t, err)
	return count
}

// TestInstanceCounts_ActiveMonth_CreateActivityWiring pins that the activity
// write path maintains the rollup end-to-end.
func TestInstanceCounts_ActiveMonth_CreateActivityWiring(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.Activity{}, &models.ActivityActorDay{}, &models.ActivityDayCounter{}, &models.InstanceMetrics{})

	activityRepo := NewActivityRepository(db, models.MainTableName, zap.NewNop(), nil)

	now := time.Now().UTC()
	makeActivity := func(id, actor string, published time.Time) *activitypub.Activity {
		return &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: id, Type: "Create", Published: &published},
			Actor:      actor,
			Object:     map[string]any{"content": "hello"},
		}
	}

	require.NoError(t, activityRepo.CreateActivity(ctx, makeActivity("act-1", "https://example.com/users/a", now)))
	require.NoError(t, activityRepo.CreateActivity(ctx, makeActivity("act-2", "https://example.com/users/b", now)))
	require.NoError(t, activityRepo.CreateActivity(ctx, makeActivity("act-3", "https://example.com/users/a", now)))

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	count, err := repo.GetActiveUserCount(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, 2, count) // two distinct actors, same day
}
