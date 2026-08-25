package repositories

import (
	"context"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// This file implements the O(1) instance-count reads and their maintenance
// paths. The four public instance stats (active_month users, total users,
// total statuses, total domains) previously counted rows by loading every item
// body into memory (per-row decrypt on encrypted payloads). They now read
// maintained counter items by primary key:
//
//   - TOTAL_USERS      -> InstanceMetrics item (SK TOTAL_USERS, TotalUsers attr)
//   - TOTAL_STATUSES   -> InstanceMetrics item (SK TOTAL_STATUSES, TotalStatuses attr)
//   - TOTAL_DOMAINS    -> InstanceMetrics item (SK TOTAL_DOMAINS, Value attr)
//   - active_month     -> sum of per-UTC-day ActivityDayCounter items
//
// Write paths maintain the counters: user/actor/account repositories bump the
// totals on create/delete, and the activity repository maintains the
// per-day distinct-actor rollup via ActivityActorDay markers.
//
// All counters are seeded lazily ONCE from a deliberate table scan on first
// read. The scan is gated by a persisted marker on success and by an in-memory
// backoff on failure (see below), so it runs at most once per seed, plus one
// retry after each backoff expiry. Tradeoffs, documented and disclosed:
//
//   - The first public read after deploy pays one table scan per unseeded
//     counter (per-process single-flight under instanceCountsSeedMu, then
//     persisted). After that every read is a point read.
//   - The seed is single-flight only within one process (Lambda instance).
//     A burst across warm instances is bounded by the persisted seed marker
//     (after a successful seed) and by the jittered in-memory seed backoff
//     (after a failed attempt), NOT by single-flight.
//   - A write that lands in the deploy-to-first-read window can be counted
//     once by the maintenance path and once by the seed scan (or lost by an
//     eventually-consistent seed scan). The error is bounded to that window
//     and never accumulates.
//   - active_month is the SUM of per-day distinct actor counts, so an actor
//     active on multiple days inside the window is counted once per day. The
//     legacy implementation returned the true window-distinct count; the sum
//     is an upper bound documented as acceptable for the public surface.
//   - active_month rollup records activity on the day it is persisted
//     (published time when available, else creation time); the legacy scan
//     filtered on publishedAt. The lazy seed reproduces the legacy window for
//     the first requested depth, so the transition is exact for that window.

// instanceCountsSeedMu serializes the one-time lazy seeds within one process,
// so a burst of first reads in the same Lambda instance collapses to a single
// compute. It does NOT span Lambda instances: cross-instance storms are bounded
// by the persisted seed marker on success and the jittered seed backoff on
// failure.
var instanceCountsSeedMu sync.Mutex

// instanceSeedBackoff is an in-memory negative-cache that stops a failed seed
// attempt from re-arming the one-time table scan at every read or 60s cache
// expiry. A sustained write failure (throttle / IAM / 5xx) would otherwise turn
// the one-time seeding scan into a recurring O(table) unauthenticated body
// scan — the exact class this remediation prohibits. While an entry is inside
// its window, seed paths return the last known value (or a documented default)
// and NEVER re-scan; after expiry a single retry is allowed.
type instanceSeedBackoffEntry struct {
	lastValue int64
	until     time.Time
}

var instanceSeedBackoff = struct {
	mu      sync.Mutex
	entries map[string]instanceSeedBackoffEntry
}{entries: make(map[string]instanceSeedBackoffEntry)}

// seedBackoffTTL returns the backoff window after a failed seed attempt,
// jittered 5-15 minutes so warm Lambda instances do not resynchronize their
// retries. Overridable in tests.
var seedBackoffTTL = func() time.Duration {
	return 5*time.Minute + time.Duration(rand.Int63n(int64(10*time.Minute)))
}

// recordInstanceSeedBackoff starts the backoff window for a seed metric after
// a failed attempt, remembering the best-known value so reads keep serving
// real data instead of zeros while the table is temporarily unwritable.
func recordInstanceSeedBackoff(metric string, lastValue int64) {
	instanceSeedBackoff.mu.Lock()
	defer instanceSeedBackoff.mu.Unlock()
	instanceSeedBackoff.entries[metric] = instanceSeedBackoffEntry{
		lastValue: lastValue,
		until:     time.Now().Add(seedBackoffTTL()),
	}
}

// instanceSeedBackoffValue returns the last known value while a seed attempt
// is inside its backoff window. Expired entries are lazily cleaned.
func instanceSeedBackoffValue(metric string) (int64, bool) {
	instanceSeedBackoff.mu.Lock()
	defer instanceSeedBackoff.mu.Unlock()
	e, ok := instanceSeedBackoff.entries[metric]
	if !ok {
		return 0, false
	}
	if time.Now().After(e.until) {
		delete(instanceSeedBackoff.entries, metric)
		return 0, false
	}
	return e.lastValue, true
}

// bumpInstanceTotalUsers atomically adjusts the TOTAL_USERS counter.
func bumpInstanceTotalUsers(ctx context.Context, db core.DB, logger *zap.Logger, delta int64) {
	bumpInstanceCountItem(ctx, db, logger, models.TotalUsersMetricSK, "TotalUsers", delta)
}

// bumpInstanceTotalDomains atomically adjusts the TOTAL_DOMAINS counter.
func bumpInstanceTotalDomains(ctx context.Context, db core.DB, logger *zap.Logger, delta int64) {
	bumpInstanceCountItem(ctx, db, logger, models.TotalDomainsMetricSK, "Value", delta)
}

// bumpInstanceCountItem applies an atomic Add to an InstanceMetrics counter,
// mirroring the established StatusRepository.bumpInstanceTotalStatuses
// pattern. Decrements are guarded so the counter can never go negative.
func bumpInstanceCountItem(ctx context.Context, db core.DB, logger *zap.Logger, sk, field string, delta int64) {
	builder := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", sk).
		UpdateBuilder().
		Add(field, delta).
		Set("UpdatedAt", time.Now())
	if delta < 0 {
		builder = builder.Condition(field, ">=", -delta)
	}
	if err := builder.Execute(); err != nil {
		logger.Warn("failed to update instance count metric",
			zap.String("metric", sk),
			zap.Int64("delta", delta),
			zap.Error(err))
	}
}

// readTotalUsersCount returns the maintained TOTAL_USERS counter.
func readTotalUsersCount(ctx context.Context, db core.DB, logger *zap.Logger) (int64, error) {
	return readInstanceMetricsField(ctx, db, logger, models.TotalUsersMetricSK, "TotalUsers")
}

// readTotalDomainsCount returns the maintained TOTAL_DOMAINS counter.
func readTotalDomainsCount(ctx context.Context, db core.DB, logger *zap.Logger) (int64, error) {
	return readInstanceMetricsField(ctx, db, logger, models.TotalDomainsMetricSK, "Value")
}

// readTotalStatusesCount returns the maintained TOTAL_STATUSES counter,
// preferring the legacy TotalStatuses attribute with Value as fallback.
func readTotalStatusesCount(ctx context.Context, db core.DB, logger *zap.Logger) (int64, error) {
	var metric models.InstanceMetrics
	err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.TotalStatusesMetricSK).
		First(&metric)
	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		logger.Error("failed to read total statuses metric", zap.Error(err))
		return 0, err
	}
	if metric.TotalStatuses != 0 {
		return metric.TotalStatuses, nil
	}
	return metric.Value, nil
}

// readInstanceMetricsField reads one int64 attribute of an InstanceMetrics
// counter item. A missing item reads as zero (an unseeded counter).
func readInstanceMetricsField(ctx context.Context, db core.DB, logger *zap.Logger, sk, field string) (int64, error) {
	var metric models.InstanceMetrics
	err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", sk).
		First(&metric)
	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		logger.Error("failed to read instance count metric",
			zap.String("metric", sk),
			zap.Error(err))
		return 0, err
	}
	switch field {
	case "TotalUsers":
		return metric.TotalUsers, nil
	case "Value":
		return metric.Value, nil
	default:
		return 0, nil
	}
}

// instanceMetricExists reports whether an InstanceMetrics item with the given
// SK already exists.
func instanceMetricExists(ctx context.Context, db core.DB, sk string) (bool, error) {
	var metric models.InstanceMetrics
	err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", sk).
		First(&metric)
	if err == nil {
		return true, nil
	}
	if errors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

// ensureTotalUsersSeeded returns the effective TOTAL_USERS value: the
// maintained counter once seeded, the last known value while a failed seed is
// in backoff, or the freshly computed value after a successful one-time seed.
func ensureTotalUsersSeeded(ctx context.Context, db core.DB, logger *zap.Logger) (int64, error) {
	return seedInstanceTotal(ctx, db, logger, models.TotalUsersMetricSK, func(ctx context.Context, db core.DB) (int64, error) {
		var users []models.User
		if err := db.WithContext(ctx).Model(&models.User{}).All(&users); err != nil {
			logger.Error("failed to compute total users seed", zap.Error(err))
			return 0, err
		}
		return int64(len(users)), nil
	}, "TotalUsers")
}

// ensureTotalDomainsSeeded returns the effective TOTAL_DOMAINS value and
// persists the per-domain DomainCounter items from a one-time scan of actor
// records (see seedInstanceTotal for the backoff semantics).
func ensureTotalDomainsSeeded(ctx context.Context, db core.DB, logger *zap.Logger) (int64, error) {
	return seedInstanceTotal(ctx, db, logger, models.TotalDomainsMetricSK, func(ctx context.Context, db core.DB) (int64, error) {
		var actors []models.Actor
		if err := db.WithContext(ctx).Model(&models.Actor{}).All(&actors); err != nil {
			logger.Error("failed to compute total domains seed", zap.Error(err))
			return 0, err
		}
		domainCounts := make(map[string]int)
		for _, actor := range actors {
			if actor.Actor == nil || actor.Actor.ID == "" {
				continue
			}
			if domain := domainFromActorID(actor.Actor.ID); domain != "" {
				domainCounts[domain]++
			}
		}
		now := time.Now()
		for domain, count := range domainCounts {
			counter := &models.DomainCounter{Domain: domain, Value: int64(count), UpdatedAt: now}
			_ = counter.UpdateKeys()
			if err := db.WithContext(ctx).Model(&models.DomainCounter{}).
				Where("PK", "=", counter.PK).
				Where("SK", "=", counter.SK).
				UpdateBuilder().
				Set("Value", int64(count)).
				Set("UpdatedAt", now).
				Execute(); err != nil {
				logger.Warn("failed to persist domain seed counter",
					zap.String("domain", domain),
					zap.Error(err))
			}
		}
		return int64(len(domainCounts)), nil
	}, "Value")
}

// seedInstanceTotal is the shared lazy-seed for the TOTAL_* counters: if the
// counter item already exists it is authoritative and no scan runs; otherwise
// one compute runs under a package mutex (re-checked) and the result is
// persisted with a Set, which also creates a missing item. It returns the
// effective value:
//
//   - counter already seeded -> the counter value;
//   - a failed attempt is inside its backoff window -> the last known value
//     without scanning again;
//   - fresh compute succeeded -> the computed (and persisted) value;
//   - fresh compute failed to scan -> error, with a backoff recorded so the
//     scan is not re-armed at the next read;
//   - persist failed -> the computed value (last known) plus a backoff, so
//     reads keep serving real data and the scan retries only after the window.
func seedInstanceTotal(ctx context.Context, db core.DB, logger *zap.Logger, sk string, compute func(context.Context, core.DB) (int64, error), field string) (int64, error) {
	exists, err := instanceMetricExists(ctx, db, sk)
	if err != nil {
		return 0, err
	}
	if exists {
		return readInstanceMetricsField(ctx, db, logger, sk, field)
	}
	if value, ok := instanceSeedBackoffValue(sk); ok {
		logger.Warn("instance count seed in backoff; serving last known value",
			zap.String("metric", sk),
			zap.Int64("value", value))
		return value, nil
	}

	instanceCountsSeedMu.Lock()
	defer instanceCountsSeedMu.Unlock()

	exists, err = instanceMetricExists(ctx, db, sk)
	if err != nil {
		return 0, err
	}
	if exists {
		return readInstanceMetricsField(ctx, db, logger, sk, field)
	}
	if value, ok := instanceSeedBackoffValue(sk); ok {
		logger.Warn("instance count seed in backoff; serving last known value",
			zap.String("metric", sk),
			zap.Int64("value", value))
		return value, nil
	}

	value, err := compute(ctx, db)
	if err != nil {
		// A failed compute would re-arm the scan at the next read: back off
		// with the documented default so the window is scan-free.
		recordInstanceSeedBackoff(sk, 0)
		return 0, err
	}

	now := time.Now()
	if err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", sk).
		UpdateBuilder().
		Set(field, value).
		Set("UpdatedAt", now).
		Execute(); err != nil {
		// Persist failure must not re-arm the scan either: remember the
		// computed value as the last known value and retry after the window.
		logger.Error("failed to persist instance count seed; entering backoff",
			zap.String("metric", sk),
			zap.Int64("value", value),
			zap.Error(err))
		recordInstanceSeedBackoff(sk, value)
		return value, nil
	}
	logger.Info("seeded instance count metric",
		zap.String("metric", sk),
		zap.Int64("value", value))
	return value, nil
}

// RecountResult reports what an offline recount rewrote.
type RecountResult struct {
	Users               int64 // TOTAL_USERS counter rewritten to
	Domains             int64 // TOTAL_DOMAINS counter rewritten to
	DomainCounters      int64 // per-domain counter items upserted
	StaleDomainCounters int64 // per-domain counter items removed
}

// RecountInstanceCounts recomputes TOTAL_USERS and TOTAL_DOMAINS (and the
// per-domain DomainCounter items) from bounded reads and, when apply is true,
// rewrites the counters. It is the drift remedy for the maintained counters:
// the lazy seed runs once ever, so any long-term divergence (a missed write,
// a manual mutation, a seed-window race) is corrected by deliberately running
// this tool — offline/invoked via `lesser recount-instance-counts`, NEVER on
// a request path. With apply=false the same computation is reported without
// writing anything (dry-run).
//
// The reads are bounded: paginated key-only projections (plus the single
// `actor` attribute needed to derive domains), never full-body materialization.
//
// Semantic note: TOTAL_USERS is computed as the number of USER#/METADATA rows
// (the canonical account row). The lazy seed reproduces the legacy
// whole-table scan semantics for compatibility; the recount writes the true
// account count, so a recount may change the public number once on tables
// whose legacy seed counted non-user item types.
func RecountInstanceCounts(ctx context.Context, db core.DB, logger *zap.Logger, apply bool) (*RecountResult, error) {
	// TOTAL_USERS: count the canonical USER#/METADATA account rows. Only the
	// key attributes are projected (encrypted payloads are never transferred).
	var userKeys []models.User
	if err := db.WithContext(ctx).Model(&models.User{}).
		Select("PK", "SK").
		Filter("PK", "begins_with", "USER#").
		Filter("SK", "=", "METADATA").
		All(&userKeys); err != nil {
		logger.Error("failed to recount user rows", zap.Error(err))
		return nil, err
	}
	users := int64(0)
	for _, u := range userKeys {
		if strings.HasPrefix(u.PK, "USER#") && u.SK == "METADATA" {
			users++
		}
	}

	// TOTAL_DOMAINS: distinct hosts of actor IDs. Only the actor JSON
	// attribute is projected, mirroring the seed's compute exactly.
	var actors []models.Actor
	if err := db.WithContext(ctx).Model(&models.Actor{}).
		Select("PK", "SK", "Actor").
		Filter("PK", "begins_with", "ACTOR#").
		Filter("SK", "=", "PROFILE").
		All(&actors); err != nil {
		logger.Error("failed to recount actor rows", zap.Error(err))
		return nil, err
	}
	domainCounts := make(map[string]int64)
	for _, actor := range actors {
		if actor.Actor == nil || actor.Actor.ID == "" {
			continue
		}
		if domain := domainFromActorID(actor.Actor.ID); domain != "" {
			domainCounts[domain]++
		}
	}

	now := time.Now()
	result := &RecountResult{Users: users, Domains: int64(len(domainCounts))}

	// Rebuild the per-domain counters: drop stale DomainCounter items and
	// upsert the current set, so subsequent actor create/delete maintenance
	// starts from a consistent tally.
	var existing []models.DomainCounter
	if err := db.WithContext(ctx).Model(&models.DomainCounter{}).
		Select("PK", "SK").
		Filter("PK", "begins_with", "DOMAIN#").
		All(&existing); err != nil {
		logger.Error("failed to recount existing domain counters", zap.Error(err))
		return nil, err
	}
	for _, counter := range existing {
		domain := strings.TrimPrefix(counter.PK, "DOMAIN#")
		if _, keep := domainCounts[domain]; keep {
			continue
		}
		if !apply {
			result.StaleDomainCounters++
			continue
		}
		if err := db.WithContext(ctx).Model(&models.DomainCounter{}).
			Where("PK", "=", counter.PK).
			Where("SK", "=", counter.SK).
			Delete(); err != nil {
			logger.Warn("failed to delete stale domain counter",
				zap.String("domain", domain),
				zap.Error(err))
			continue
		}
		result.StaleDomainCounters++
	}
	for domain, count := range domainCounts {
		counter := &models.DomainCounter{Domain: domain, Value: count, UpdatedAt: now}
		_ = counter.UpdateKeys()
		if apply {
			if err := db.WithContext(ctx).Model(&models.DomainCounter{}).
				Where("PK", "=", counter.PK).
				Where("SK", "=", counter.SK).
				UpdateBuilder().
				Set("Value", count).
				Set("UpdatedAt", now).
				Execute(); err != nil {
				logger.Warn("failed to persist recount domain counter",
					zap.String("domain", domain),
					zap.Error(err))
				continue
			}
		}
		result.DomainCounters++
	}

	if !apply {
		logger.Info("recounted instance counters (dry-run, nothing written)",
			zap.Int64("users", users),
			zap.Int64("domains", result.Domains),
			zap.Int64("domainCounters", result.DomainCounters),
			zap.Int64("staleDomainCounters", result.StaleDomainCounters))
		return result, nil
	}

	// Rewrite the global counters (Set also creates a missing item).
	if err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.TotalUsersMetricSK).
		UpdateBuilder().
		Set("TotalUsers", users).
		Set("UpdatedAt", now).
		Execute(); err != nil {
		logger.Error("failed to rewrite TOTAL_USERS counter", zap.Error(err))
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.TotalDomainsMetricSK).
		UpdateBuilder().
		Set("Value", result.Domains).
		Set("UpdatedAt", now).
		Execute(); err != nil {
		logger.Error("failed to rewrite TOTAL_DOMAINS counter", zap.Error(err))
		return nil, err
	}

	logger.Info("recounted instance counters",
		zap.Int64("users", users),
		zap.Int64("domains", result.Domains),
		zap.Int64("domainCounters", result.DomainCounters),
		zap.Int64("staleDomainCounters", result.StaleDomainCounters))
	return result, nil
}

// recordActivityActorDay marks an actor as active on a UTC day. The marker is
// created conditionally; only the first activity of an actor in that day bumps
// the day counter. This is the write-path maintenance for active_month.
func recordActivityActorDay(ctx context.Context, db core.DB, logger *zap.Logger, actorID, day string) {
	if actorID == "" || day == "" {
		return
	}
	now := time.Now()
	marker := &models.ActivityActorDay{ActorID: actorID, Day: day, UpdatedAt: now}
	if err := marker.UpdateKeys(); err != nil {
		logger.Warn("failed to derive activity actor day keys", zap.Error(err))
		return
	}

	err := db.WithContext(ctx).Model(marker).IfNotExists().Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			// Actor already counted for this day.
			return
		}
		logger.Warn("failed to record activity actor day marker",
			zap.String("actor", actorID),
			zap.String("day", day),
			zap.Error(err))
		return
	}

	// First activity of this actor for the day: bump the day counter.
	if err := db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
		Where("PK", "=", models.ActivityDayKey(day)).
		Where("SK", "=", models.DayCounterSK).
		UpdateBuilder().
		Add("Value", 1).
		Set("UpdatedAt", now).
		Execute(); err != nil {
		logger.Warn("failed to bump active day counter",
			zap.String("day", day),
			zap.Error(err))
	}
}

// readActiveMonthCount returns the sum of per-day distinct-actor counters over
// the last `days` UTC days. The window sum is an upper bound on the true
// window-distinct count (see the file header).
func readActiveMonthCount(ctx context.Context, db core.DB, logger *zap.Logger, days int) (int, error) {
	if err := ensureActiveMonthSeeded(ctx, db, logger, days); err != nil {
		return 0, err
	}
	var total int64
	now := time.Now()
	for i := 0; i < days; i++ {
		day := models.DayFormat(now.AddDate(0, 0, -i))
		var counter models.ActivityDayCounter
		err := db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
			Where("PK", "=", models.ActivityDayKey(day)).
			Where("SK", "=", models.DayCounterSK).
			First(&counter)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			logger.Error("failed to read active day counter",
				zap.String("day", day),
				zap.Error(err))
			return 0, err
		}
		total += counter.Value
	}
	return int(total), nil
}

// ensureActiveMonthSeeded backfills the per-day rollup once from a scan of
// activity records, reproducing the legacy publishedAt-window computation for
// the first requested depth (rounded up to 30 days so a DAU-first read still
// seeds the monthly surface). The one-time scan reads item bodies exactly once;
// afterwards the rollup is maintained by recordActivityActorDay. A failed seed
// (scan error or seed-marker persist error) records an in-memory backoff so
// the scan is not re-armed at every read or 60s cache expiry; during the
// window the read path keeps summing whatever day counters exist.
func ensureActiveMonthSeeded(ctx context.Context, db core.DB, logger *zap.Logger, days int) error {
	seeded, err := instanceMetricExists(ctx, db, models.ActiveMonthSeedMetricSK)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}
	if _, ok := instanceSeedBackoffValue(models.ActiveMonthSeedMetricSK); ok {
		logger.Warn("active month seed in backoff; serving existing day counters")
		return nil
	}

	instanceCountsSeedMu.Lock()
	defer instanceCountsSeedMu.Unlock()

	seeded, err = instanceMetricExists(ctx, db, models.ActiveMonthSeedMetricSK)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}
	if _, ok := instanceSeedBackoffValue(models.ActiveMonthSeedMetricSK); ok {
		logger.Warn("active month seed in backoff; serving existing day counters")
		return nil
	}

	seedDays := days
	if seedDays < 30 {
		seedDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -seedDays)

	// One-time backfill scan. The legacy implementation filtered the scan with
	// Filter("PublishedAt", ">", cutoff), but that attribute does not resolve
	// portably across the table mapper and the fakedb emulator; the scan is
	// post-filtered in-memory with identical window semantics. The scan cost is
	// one-time, per-process single-flight, and backoff-gated after a failure.
	var activities []models.Activity
	if err := db.WithContext(ctx).Model(&models.Activity{}).
		All(&activities); err != nil {
		logger.Error("failed to compute active month seed; entering backoff", zap.Error(err))
		recordInstanceSeedBackoff(models.ActiveMonthSeedMetricSK, 0)
		return err
	}

	// Distinct actors per UTC day (same set semantics the legacy count used for
	// the whole window, applied per day).
	dayActors := make(map[string]map[string]bool)
	for _, activity := range activities {
		if activity.Activity == nil || activity.Activity.Actor == "" {
			continue
		}
		if !activityInWindow(activity, cutoff) {
			continue
		}
		day := activityDayOf(activity)
		if dayActors[day] == nil {
			dayActors[day] = make(map[string]bool)
		}
		dayActors[day][activity.Activity.Actor] = true
	}

	now := time.Now()
	for day, actors := range dayActors {
		if err := db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
			Where("PK", "=", models.ActivityDayKey(day)).
			Where("SK", "=", models.DayCounterSK).
			UpdateBuilder().
			Set("Value", int64(len(actors))).
			Set("UpdatedAt", now).
			Execute(); err != nil {
			logger.Warn("failed to persist active day seed counter",
				zap.String("day", day),
				zap.Error(err))
		}
	}

	// Mark today's actors so a same-day re-activation after the seed does not
	// double count (markers for past days can never be written again).
	today := models.DayFormat(now)
	if todayActors, ok := dayActors[today]; ok {
		for actorID := range todayActors {
			marker := &models.ActivityActorDay{ActorID: actorID, Day: today, UpdatedAt: now}
			_ = marker.UpdateKeys()
			if err := db.WithContext(ctx).Model(marker).IfNotExists().Create(); err != nil && !errors.IsConditionFailed(err) {
				logger.Warn("failed to persist active month seed marker",
					zap.String("actor", actorID),
					zap.Error(err))
			}
		}
	}

	// Persist the seed marker so the scan never runs again. A persist failure
	// here must NOT silently re-arm the scan at the next read: record the
	// in-memory backoff (the in-memory fallback for the marker-write-failure
	// path) so subsequent reads serve the day counters without re-scanning
	// until the window expires.
	if err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.ActiveMonthSeedMetricSK).
		UpdateBuilder().
		Set("UpdatedAt", now).
		Execute(); err != nil {
		logger.Warn("failed to persist active month seed marker metric; entering backoff", zap.Error(err))
		recordInstanceSeedBackoff(models.ActiveMonthSeedMetricSK, 0)
	}
	logger.Info("seeded active month rollup", zap.Int("days", seedDays))
	return nil
}

// activityDayOf returns the UTC day bucket for an activity record: the
// published time when available, otherwise the creation time.
func activityDayOf(activity models.Activity) string {
	if activity.Activity != nil && activity.Activity.Published != nil {
		return models.DayFormat(*activity.Activity.Published)
	}
	return models.DayFormat(activity.CreatedAt)
}

// activityInWindow reports whether an activity falls inside the seed window,
// mirroring the legacy publishedAt-cutoff semantics (creation time fallback).
func activityInWindow(activity models.Activity, cutoff time.Time) bool {
	if activity.Activity != nil && activity.Activity.Published != nil {
		return activity.Activity.Published.After(cutoff)
	}
	return activity.CreatedAt.After(cutoff)
}

// recordActorDomain tallies a new actor record for its domain. The first actor
// of a domain creates the DomainCounter item (conditional) and bumps the
// global TOTAL_DOMAINS counter; subsequent actors of the same domain only bump
// the per-domain tally.
func recordActorDomain(ctx context.Context, db core.DB, logger *zap.Logger, domain string) {
	if domain == "" {
		return
	}
	now := time.Now()
	counter := &models.DomainCounter{Domain: domain, Value: 1, UpdatedAt: now}
	if err := counter.UpdateKeys(); err != nil {
		logger.Warn("failed to derive domain counter keys", zap.Error(err))
		return
	}

	err := db.WithContext(ctx).Model(counter).IfNotExists().Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			// Domain already known: tally the actor.
			if err := db.WithContext(ctx).Model(&models.DomainCounter{}).
				Where("PK", "=", counter.PK).
				Where("SK", "=", counter.SK).
				UpdateBuilder().
				Add("Value", 1).
				Set("UpdatedAt", now).
				Execute(); err != nil {
				logger.Warn("failed to bump domain counter",
					zap.String("domain", domain),
					zap.Error(err))
			}
			return
		}
		logger.Warn("failed to record actor domain",
			zap.String("domain", domain),
			zap.Error(err))
		return
	}

	// First actor of this domain: bump the global domain count.
	bumpInstanceTotalDomains(ctx, db, logger, 1)
}

// releaseActorDomain tallies an actor record being deleted. When the last
// actor of a domain is removed the per-domain counter is dropped and the
// global TOTAL_DOMAINS counter is decremented.
func releaseActorDomain(ctx context.Context, db core.DB, logger *zap.Logger, domain string) {
	if domain == "" {
		return
	}
	now := time.Now()
	var updated models.DomainCounter
	err := db.WithContext(ctx).Model(&models.DomainCounter{}).
		Where("PK", "=", "DOMAIN#"+domain).
		Where("SK", "=", models.DayCounterSK).
		UpdateBuilder().
		Add("Value", -1).
		Condition("Value", ">=", 1).
		Set("UpdatedAt", now).
		ExecuteWithResult(&updated)
	if err != nil {
		if errors.IsConditionFailed(err) {
			// Counter already zero: nothing left to release.
			return
		}
		logger.Warn("failed to release actor domain",
			zap.String("domain", domain),
			zap.Error(err))
		return
	}
	if updated.Value > 0 {
		return
	}
	if err := db.WithContext(ctx).Model(&models.DomainCounter{}).
		Where("PK", "=", "DOMAIN#"+domain).
		Where("SK", "=", models.DayCounterSK).
		Delete(); err != nil {
		logger.Warn("failed to delete empty domain counter",
			zap.String("domain", domain),
			zap.Error(err))
		// Keep the global counter unchanged: the stale per-domain item at
		// zero is harmless and a later actor for the domain will tally it.
		return
	}
	bumpInstanceTotalDomains(ctx, db, logger, -1)
}

// domainFromActorID extracts the host from an actor ID URL, matching the
// legacy GetTotalDomainCount semantics exactly.
func domainFromActorID(actorID string) string {
	if actorID == "" {
		return ""
	}
	u, err := url.Parse(actorID)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
