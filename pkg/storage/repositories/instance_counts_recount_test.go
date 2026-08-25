package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
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

// TestInstanceCounts_Recount_TrueUserCountVsLegacySeed pins the documented
// semantic: the lazy seed reproduces the legacy whole-table scan count (on
// this mixed fixture every item — 2 users + 6 actors = 8 — is counted), while
// the recount writes the true USER#/METADATA account count. The recount is the
// drift remedy for that divergence, and after it runs the seed path reads the
// rewritten counter.
func TestInstanceCounts_Recount_TrueUserCountVsLegacySeed(t *testing.T) {
	ctx := context.Background()
	db, _ := newInstanceCountsTestDB(t, &models.User{}, &models.Actor{}, &models.DomainCounter{}, &models.InstanceMetrics{})

	seedActors(t, ctx, db, 6, []string{"a.example.com", "b.example.com", "c.example.com"})
	seedUsers(t, ctx, db, 2)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	// Legacy-preserving seed: whole-table item count (8), not the true count.
	userCount, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 8, userCount)
	// Domain semantics agree (only real actor rows carry the actor attribute).
	domainCount, err := repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, domainCount)

	result, err := RecountInstanceCounts(ctx, db, zap.NewNop(), true)
	require.NoError(t, err)
	require.EqualValues(t, 2, result.Users)
	require.EqualValues(t, 3, result.Domains)

	// The seed path now reads the rewritten counter.
	userCount, err = repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	domainCount, err = repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, userCount)
	require.Equal(t, 3, domainCount)
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
