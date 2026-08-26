package repositories

import (
	"context"
	"net/url"
	"strings"
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
// NO request-adjacent full-table scan is sanctioned on any of these paths,
// marker-gated or not. The prior "deliberate one-time lazy seed" disposition
// (introduced by #1468, revoked by operator doctrine 2026-08-26) is removed:
// the reads below are point reads (or sums of point reads) and NEVER call
// All() on User/Actor/Activity. An unseeded counter reads as the documented
// default (0) until maintenance or the offline recount populates it.
//
// Counters are seeded and maintained OFF the request path:
//
//   - write-path maintenance: user/actor/account/status/activity write paths
//     bump the counters on create/delete;
//   - offline recount: `lesser recount-instance-counts` (RecountInstanceCounts)
//     recomputes TOTAL_USERS / TOTAL_DOMAINS / per-domain DomainCounter items
//     and the active-month per-day rollup (+ the SEED#ACTIVE_MONTH marker)
//     from bounded paginated key-only projections. This is the unblock for
//     instances whose v1.6.24 lazy seed never completed.
//
// Tradeoffs, documented and disclosed:
//
//   - active_month is the SUM of per-day distinct actor counts, so an actor
//     active on multiple days inside the window is counted once per day. The
//     legacy implementation returned the true window-distinct count; the sum
//     is an upper bound documented as acceptable for the public surface.
//   - active_month rollup records activity on the day it is persisted
//     (published time when available, else creation time).
//   - An unseeded table reads as zeros until the write path or the offline
//     recount populates the counters; the public surface is eventually
//     consistent via maintained writes + offline recount.

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

// readTotalUsersCount returns the maintained TOTAL_USERS counter (point read).
// An unseeded counter reads as the documented default (0): seeding happens off
// the request path via write-path maintenance and the offline recount (see the
// file header). This path NEVER scans.
func readTotalUsersCount(ctx context.Context, db core.DB, logger *zap.Logger) (int64, error) {
	return readInstanceMetricsField(ctx, db, logger, models.TotalUsersMetricSK, "TotalUsers")
}

// readTotalDomainsCount returns the maintained TOTAL_DOMAINS counter (point
// read). An unseeded counter reads as the documented default (0); see
// readTotalUsersCount. This path NEVER scans.
func readTotalDomainsCount(ctx context.Context, db core.DB, logger *zap.Logger) (int64, error) {
	return readInstanceMetricsField(ctx, db, logger, models.TotalDomainsMetricSK, "Value")
}

// RecountResult reports what an offline recount rewrote.
type RecountResult struct {
	Users                int64 // TOTAL_USERS counter rewritten to
	Domains              int64 // TOTAL_DOMAINS counter rewritten to
	DomainCounters       int64 // per-domain counter items upserted
	StaleDomainCounters  int64 // per-domain counter items removed
	ActiveMonthDays      int64 // per-UTC-day counters upserted
	StaleActiveMonthDays int64 // stale in-window per-day counters removed
	ActiveMonthSum       int64 // sum of the rebuilt per-day rollup
	ActiveMonthSeedMarker bool // SEED#ACTIVE_MONTH marker written (apply only)
}

// RecountInstanceCounts recomputes TOTAL_USERS, TOTAL_DOMAINS (and the
// per-domain DomainCounter items), and the active-month per-UTC-day rollup
// (+ the SEED#ACTIVE_MONTH marker) from bounded reads and, when apply is true,
// rewrites them. It is the drift remedy AND the sanctioned seed mechanism for
// the maintained counters: there is no request-adjacent lazy seed, so any
// long-term divergence (a missed write, a manual mutation) or an unseeded
// table (a broken v1.6.24 deploy whose lazy seed never completed) is corrected
// by deliberately running this tool — offline/invoked via
// `lesser recount-instance-counts`, NEVER on a request path. With apply=false
// the same computation is reported without writing anything (dry-run).
//
// The reads are bounded: paginated key-only projections (plus the single
// attributes needed to derive domains and activity days), never full-body
// materialization. They are scans in the DynamoDB sense (offline tooling), but
// no request goroutine ever reaches them.
//
// Semantic notes:
//
//   - TOTAL_USERS is computed as the number of USER#/METADATA rows (the
//     canonical account row).
//   - The active-month rollup covers the retention window
//     (activeMonthWindowDays) so the widest reader window
//     (pkg/services/accounts reads 180 days) is served; day counters outside
//     the window are left to TTL.
func RecountInstanceCounts(ctx context.Context, db core.DB, logger *zap.Logger, apply bool) (*RecountResult, error) {
	users, err := recountTotalUsers(ctx, db, logger)
	if err != nil {
		return nil, err
	}
	domainCounts, err := recountActorDomains(ctx, db, logger)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	result := &RecountResult{Users: users, Domains: int64(len(domainCounts))}

	if err := recountDomainCounters(ctx, db, logger, result, domainCounts, apply, now); err != nil {
		return nil, err
	}
	if err := recountActiveMonthRollup(ctx, db, logger, result, apply, now); err != nil {
		return nil, err
	}

	if !apply {
		logger.Info("recounted instance counters (dry-run, nothing written)",
			zap.Int64("users", users),
			zap.Int64("domains", result.Domains),
			zap.Int64("domainCounters", result.DomainCounters),
			zap.Int64("staleDomainCounters", result.StaleDomainCounters),
			zap.Int64("activeMonthDays", result.ActiveMonthDays),
			zap.Int64("staleActiveMonthDays", result.StaleActiveMonthDays),
			zap.Int64("activeMonthSum", result.ActiveMonthSum))
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

	// Persist the SEED#ACTIVE_MONTH marker so no deploy can ever re-arm a
	// request-adjacent active-month scan. A failure is fatal: the operator
	// re-runs the idempotent recount and retries.
	if err := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.ActiveMonthSeedMetricSK).
		UpdateBuilder().
		Set("UpdatedAt", now).
		Execute(); err != nil {
		logger.Error("failed to persist active month seed marker", zap.Error(err))
		return nil, err
	}
	result.ActiveMonthSeedMarker = true

	logger.Info("recounted instance counters",
		zap.Int64("users", users),
		zap.Int64("domains", result.Domains),
		zap.Int64("domainCounters", result.DomainCounters),
		zap.Int64("staleDomainCounters", result.StaleDomainCounters),
		zap.Int64("activeMonthDays", result.ActiveMonthDays),
		zap.Int64("staleActiveMonthDays", result.StaleActiveMonthDays),
		zap.Int64("activeMonthSum", result.ActiveMonthSum))
	return result, nil
}

// recountTotalUsers counts the canonical USER#/METADATA account rows via a
// bounded key-only projection (encrypted payloads are never transferred).
func recountTotalUsers(ctx context.Context, db core.DB, logger *zap.Logger) (int64, error) {
	var userKeys []models.User
	if err := db.WithContext(ctx).Model(&models.User{}).
		Select("PK", "SK").
		Filter("PK", "begins_with", "USER#").
		Filter("SK", "=", "METADATA").
		All(&userKeys); err != nil {
		logger.Error("failed to recount user rows", zap.Error(err))
		return 0, err
	}
	users := int64(0)
	for _, u := range userKeys {
		if strings.HasPrefix(u.PK, "USER#") && u.SK == models.SKMetadata {
			users++
		}
	}
	return users, nil
}

// recountActorDomains computes the distinct hosts of actor IDs from a bounded
// projection of the actor JSON attribute.
func recountActorDomains(ctx context.Context, db core.DB, logger *zap.Logger) (map[string]int64, error) {
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
	return domainCounts, nil
}

// recountDomainCounters rebuilds the per-domain DomainCounter items: stale
// counters are dropped and the current set upserted, so subsequent actor
// create/delete maintenance starts from a consistent tally.
func recountDomainCounters(ctx context.Context, db core.DB, logger *zap.Logger, result *RecountResult, domainCounts map[string]int64, apply bool, now time.Time) error {
	var existing []models.DomainCounter
	if err := db.WithContext(ctx).Model(&models.DomainCounter{}).
		Select("PK", "SK").
		Filter("PK", "begins_with", "DOMAIN#").
		All(&existing); err != nil {
		logger.Error("failed to recount existing domain counters", zap.Error(err))
		return err
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
	return nil
}

// activeMonthWindowDays is the rollup horizon for the offline active-month
// recount. It must cover the widest reader window (pkg/services/accounts reads
// 180 days for the half-year figure) and stay inside the ActivityDayCounter
// TTL retention (200 days) so rebuilt counters are not immediately expired.
const activeMonthWindowDays = 200

// recountActiveMonthRollup rebuilds the active-month per-UTC-day rollup from
// the activity projection and, in apply mode, persists the day counters, drops
// stale in-window counters, writes today's ActivityActorDay markers (so the
// write path does not double-count today), and persists the SEED#ACTIVE_MONTH
// marker. It reports progress on result. This is what unblocks instances whose
// v1.6.24 lazy active-month seed never completed.
func recountActiveMonthRollup(ctx context.Context, db core.DB, logger *zap.Logger, result *RecountResult, apply bool, now time.Time) error {
	cutoff := now.AddDate(0, 0, -activeMonthWindowDays)
	dayActors, err := collectActiveMonthDayActors(ctx, db, logger, cutoff)
	if err != nil {
		return err
	}
	for _, actors := range dayActors {
		result.ActiveMonthSum += int64(len(actors))
	}

	if err := recountStaleActiveMonthDays(ctx, db, logger, result, dayActors, apply, cutoff); err != nil {
		return err
	}
	if err := recountActiveMonthDayCounters(ctx, db, logger, result, dayActors, apply, now); err != nil {
		return err
	}
	recountActiveMonthMarkers(ctx, db, logger, dayActors, apply, now)
	return nil
}

// collectActiveMonthDayActors computes the distinct-actor set per UTC day from
// a bounded projection of activity rows inside the retention window.
func collectActiveMonthDayActors(ctx context.Context, db core.DB, logger *zap.Logger, cutoff time.Time) (map[string]map[string]bool, error) {
	// Project only the attributes needed to bucket an activity into its UTC day:
	// the activity JSON (actor + published time) and the creation-time fallback.
	var activities []models.Activity
	if err := db.WithContext(ctx).Model(&models.Activity{}).
		Select("PK", "SK", "Activity", "CreatedAt").
		Filter("SK", "begins_with", "ACTIVITY#").
		All(&activities); err != nil {
		logger.Error("failed to recount activity rows", zap.Error(err))
		return nil, err
	}

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
	return dayActors, nil
}

// recountStaleActiveMonthDays drops in-window day counters that have no
// activity rows, so subsequent write-path maintenance starts from a consistent
// rollup. Days outside the retention window are left to TTL.
func recountStaleActiveMonthDays(ctx context.Context, db core.DB, logger *zap.Logger, result *RecountResult, dayActors map[string]map[string]bool, apply bool, cutoff time.Time) error {
	var existingDays []models.ActivityDayCounter
	if err := db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
		Select("PK", "SK").
		Filter("PK", "begins_with", "ACTIVITY_DAY#").
		All(&existingDays); err != nil {
		logger.Error("failed to recount existing active month counters", zap.Error(err))
		return err
	}
	cutoffDay := models.DayFormat(cutoff)
	for _, dayCounter := range existingDays {
		day := strings.TrimPrefix(dayCounter.PK, "ACTIVITY_DAY#")
		if day < cutoffDay {
			continue // outside the retention window; TTL handles it
		}
		if _, keep := dayActors[day]; keep {
			continue
		}
		if !apply {
			result.StaleActiveMonthDays++
			continue
		}
		if err := db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
			Where("PK", "=", dayCounter.PK).
			Where("SK", "=", dayCounter.SK).
			Delete(); err != nil {
			logger.Warn("failed to delete stale active month counter",
				zap.String("day", day),
				zap.Error(err))
			continue
		}
		result.StaleActiveMonthDays++
	}
	return nil
}

// recountActiveMonthDayCounters upserts the per-day counter items for the
// recomputed rollup.
func recountActiveMonthDayCounters(ctx context.Context, db core.DB, logger *zap.Logger, result *RecountResult, dayActors map[string]map[string]bool, apply bool, now time.Time) error {
	for day, actors := range dayActors {
		if !apply {
			result.ActiveMonthDays++
			continue
		}
		counter := &models.ActivityDayCounter{Date: day, Value: int64(len(actors)), UpdatedAt: now}
		_ = counter.UpdateKeys()
		if err := db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
			Where("PK", "=", counter.PK).
			Where("SK", "=", counter.SK).
			UpdateBuilder().
			Set("Value", counter.Value).
			Set("UpdatedAt", now).
			Execute(); err != nil {
			logger.Warn("failed to persist active month counter",
				zap.String("day", day),
				zap.Error(err))
			continue
		}
		result.ActiveMonthDays++
	}
	return nil
}

// recountActiveMonthMarkers writes today's ActivityActorDay markers so a
// same-day re-activation after the recount does not double count (the write
// path bumps the counter only when the marker is newly created; markers for
// past days can never be written again).
func recountActiveMonthMarkers(ctx context.Context, db core.DB, logger *zap.Logger, dayActors map[string]map[string]bool, apply bool, now time.Time) {
	if !apply {
		return
	}
	today := models.DayFormat(now)
	todayActors, ok := dayActors[today]
	if !ok {
		return
	}
	for actorID := range todayActors {
		marker := &models.ActivityActorDay{ActorID: actorID, Day: today, UpdatedAt: now}
		_ = marker.UpdateKeys()
		if err := db.WithContext(ctx).Model(marker).IfNotExists().Create(); err != nil && !errors.IsConditionFailed(err) {
			logger.Warn("failed to persist active month recount marker",
				zap.String("actor", actorID),
				zap.Error(err))
		}
	}
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
// window-distinct count (see the file header). Every read is a point read of a
// maintained day counter; missing days sum as zero, and the read NEVER scans.
// The rollup is populated off the request path (activity write path + the
// offline recount).
func readActiveMonthCount(ctx context.Context, db core.DB, logger *zap.Logger, days int) (int, error) {
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

// activityDayOf returns the UTC day bucket for an activity record: the
// published time when available, otherwise the creation time. Used by the
// offline active-month recount.
func activityDayOf(activity models.Activity) string {
	if activity.Activity != nil && activity.Activity.Published != nil {
		return models.DayFormat(*activity.Activity.Published)
	}
	return models.DayFormat(activity.CreatedAt)
}

// activityInWindow reports whether an activity falls inside the recount
// window, mirroring the legacy publishedAt-cutoff semantics (creation time
// fallback).
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
