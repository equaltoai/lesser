package repositories

import (
	"context"
	"net/url"
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
// All counters are seeded lazily ONCE from a bounded scan on first read
// (single-flight). Tradeoffs, documented and disclosed:
//
//   - The first public read after deploy pays one table scan per unseeded
//     counter (warm, single-flight, then persisted). After that every read is
//     a point read.
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

// instanceCountsSeedMu serializes the one-time lazy seeds so a burst of
// first reads collapses to a single compute.
var instanceCountsSeedMu sync.Mutex

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

// ensureTotalUsersSeeded computes and persists the TOTAL_USERS counter from a
// one-time scan when it has never been created. Idempotent and single-flight.
func ensureTotalUsersSeeded(ctx context.Context, db core.DB, logger *zap.Logger) error {
	return seedInstanceTotal(ctx, db, logger, models.TotalUsersMetricSK, func(ctx context.Context, db core.DB) (int64, error) {
		var users []models.User
		if err := db.WithContext(ctx).Model(&models.User{}).All(&users); err != nil {
			logger.Error("failed to compute total users seed", zap.Error(err))
			return 0, err
		}
		return int64(len(users)), nil
	}, "TotalUsers")
}

// ensureTotalDomainsSeeded computes and persists the TOTAL_DOMAINS counter and
// the per-domain DomainCounter items from a one-time scan of actor records.
// Idempotent and single-flight.
func ensureTotalDomainsSeeded(ctx context.Context, db core.DB, logger *zap.Logger) error {
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

// seedInstanceTotal is the shared single-flight lazy-seed for the TOTAL_*
// counters: if the counter item already exists it is authoritative and no scan
// runs; otherwise one compute runs under a package mutex (re-checked) and the
// result is persisted with a Set, which also creates a missing item.
func seedInstanceTotal(ctx context.Context, db core.DB, logger *zap.Logger, sk string, compute func(context.Context, core.DB) (int64, error), field string) error {
	exists, err := instanceMetricExists(ctx, db, sk)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	instanceCountsSeedMu.Lock()
	defer instanceCountsSeedMu.Unlock()

	exists, err = instanceMetricExists(ctx, db, sk)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	value, err := compute(ctx, db)
	if err != nil {
		return err
	}

	now := time.Now()
	if err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", sk).
		UpdateBuilder().
		Set(field, value).
		Set("UpdatedAt", now).
		Execute(); err != nil {
		logger.Error("failed to persist instance count seed",
			zap.String("metric", sk),
			zap.Error(err))
		return err
	}
	logger.Info("seeded instance count metric",
		zap.String("metric", sk),
		zap.Int64("value", value))
	return nil
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
// afterwards the rollup is maintained by recordActivityActorDay.
func ensureActiveMonthSeeded(ctx context.Context, db core.DB, logger *zap.Logger, days int) error {
	seeded, err := instanceMetricExists(ctx, db, models.ActiveMonthSeedMetricSK)
	if err != nil {
		return err
	}
	if seeded {
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

	seedDays := days
	if seedDays < 30 {
		seedDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -seedDays)

	// One-time backfill scan. The legacy implementation filtered the scan with
	// Filter("PublishedAt", ">", cutoff), but that attribute does not resolve
	// portably across the table mapper and the fakedb emulator; the scan is
	// post-filtered in-memory with identical window semantics. The scan cost is
	// one-time and single-flight.
	var activities []models.Activity
	if err := db.WithContext(ctx).Model(&models.Activity{}).
		All(&activities); err != nil {
		logger.Error("failed to compute active month seed", zap.Error(err))
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

	// Persist the seed marker so the scan never runs again.
	if err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.ActiveMonthSeedMetricSK).
		UpdateBuilder().
		Set("UpdatedAt", now).
		Execute(); err != nil {
		logger.Warn("failed to persist active month seed marker metric", zap.Error(err))
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
