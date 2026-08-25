# DynamoDB Table-Wide Scan Inventory (and Scan-Free Redesigns)

This repo uses a single DynamoDB table (`PK`/`SK`) plus generic GSIs (`gsi1`…`gsi9` with `gsi{N}PK`/`gsi{N}SK`), plus a small number of dedicated indexes (e.g. `oauth-clients-index`).

**Goal:** eliminate *table-wide* (or *index-wide*) scans in production paths by redesigning access patterns to use:

- **Query** on base table: `PK = ...` (+ optional `SK` range/prefix)
- **Query** on a GSI: `Index("gsiN")` + `gsiNPK = ...` (+ optional `gsiNSK` range/prefix)
- **TTL** (attribute `ttl`) instead of “scan to delete expired”
- **Reverse index items** (extra rows) when you must look up by an attribute that isn’t in any key

**Important TableTheory note:** calling `.Scan(...)` issues a DynamoDB **Scan** (table scan / index scan). Prefer `.First(...)` / `.All(...)` with **key conditions** so TableTheory can compile to a DynamoDB **Query**.

This document is intentionally pragmatic: it inventories known table/index scans and proposes scan-free replacements that fit the existing `gsi1…gsi9` pattern.

---

## Incident: `trend cleanup` deleted the primary admin user

**Observed symptoms (from export):**

- Instance config still declares `primaryAdminUsername = simulacrum` (`INSTANCE#CONFIG` / `STATE`).
- The core items for that account are missing:
  - expected `USER#simulacrum` / `METADATA`
  - expected `ACTOR#simulacrum` / `PROFILE`
- Only association records remain (examples from the export):
  - `USER#simulacrum` / `DEVICE#...`
  - `USER#simulacrum` / `WALLET#0x...`
  - `WALLET#ethereum#0x...` / `USER#simulacrum`

**CloudWatch evidence (no CloudTrail required):**

- The scheduled Lambda `simulacrum-dev-trend-aggregator` ran at **2026-03-02T02:00:39Z** (which is **2026-03-01 21:00:39 EST**) and logged:
  - “starting cleanup of old trend data” with cutoff **2026-02-23T02:00:39Z**
  - “deleted old hashtag trends” with `count: 2` (and `before: 2026-02-23T02:00:39Z`)

**Root cause in code:**

- `pkg/storage/repositories/analytics_repository.go` – `TrendingRepository.deleteOldTrendsGeneric`
  - performs `Filter("UpdatedAt", "<", before).Scan(...)` (table scan)
  - then deletes every returned item via `Model(trend).Delete()`
  - because it’s a scan with only an `UpdatedAt` filter, it can deserialize *any* item with an `updatedAt` attribute (User, Actor, etc.) and then delete it by `PK`/`SK`.

### Trend cleanup redesign (concrete)

Trend models already use **date-bucketed PKs** and set **TTL** automatically:

- `pkg/storage/models/trends.go`
  - `HashtagTrend.PK = TREND_TYPE#HASHTAG#YYYY-MM-DD` and `ttl = updatedAt + 7d`
  - `StatusTrend.PK = TREND_TYPE#STATUS#YYYY-MM-DD` and `ttl = updatedAt + 7d`
  - `LinkTrend.PK = TREND_TYPE#LINK#YYYY-MM-DD` and `ttl = updatedAt + 7d`

That means the scan-based cleanup is unnecessary and dangerous.

**Recommended implementation path (no scans):**

1. **Remove manual cleanup** in `cmd/trend-aggregator/main.go` (or make the repository methods no-ops).
   - Keep the log lines (so it’s obvious the job ran) but do not delete by scanning.
2. If you still want deterministic deletion (instead of eventual TTL), delete by **known partitions**:
   - compute `cutoffDate := before.UTC().Format(common.DateFormat)`
   - for each `date < cutoffDate` you want to purge (usually just `date = cutoffDate.AddDate(0,0,-1)` if this cron runs daily):
     - query `PK = TREND_TYPE#HASHTAG#<date>` (and similarly for STATUS/LINK)
     - delete the returned items
3. Add a guardrail in deletion loops:
   - refuse to delete if `PK` does not start with the expected `TREND_TYPE#<TYPE>#` prefix.

This leverages the existing schema and eliminates the entire class of “wrong model unmarshaled and deleted” bugs.

---

## Inventory: known table/index scans (with redesigns)

### P0 – Scan + Delete (data-loss risk)

1) `pkg/storage/repositories/analytics_repository.go:889` – `TrendingRepository.deleteOldTrendsGeneric`
   - **Current (fixed):** TTL-only. Manual cleanup is a no-op to prevent scan-based deletion.

2) `pkg/storage/repositories/hashtag_batch_helpers.go:61` – `deleteOldHashtagTrendRecordsBatch` (and related)
   - **Current (fixed):** TTL-only. Helpers are no-ops to prevent scan-based deletion.

3) `pkg/storage/repositories/object_repository.go:2288` – `ObjectRepository.CleanupExpiredTombstones`
   - **Current (fixed):** TTL-only. Manual cleanup is a no-op (no scans).

4) `pkg/storage/repositories/user_repository.go:2735` – `UserRepository.DeleteExpiredTimelineEntries`
   - **Current (fixed):** TTL-only. Manual cleanup is a no-op (no scans).

5) `pkg/storage/repositories/media_repository.go:1276` – `MediaRepository.DeleteExpiredMedia`
   - **Current (fixed):** TTL-only. Manual cleanup is a no-op (no scans).

6) `pkg/storage/repositories/notification_repository.go:797` – `NotificationRepository.DeleteExpiredNotifications`
   - **Current (fixed):** TTL-only. Manual cleanup is a no-op (no scans).

7) `pkg/storage/repositories/dlq_repository.go:618` – `DLQRepository.CleanupExpiredMessages`
   - **Current (fixed):** TTL-only. Manual cleanup is a no-op (no scans).

8) `pkg/storage/repositories/media_analytics_repository.go:519` – `MediaAnalyticsRepository.CleanupOldAnalytics`
   - **Current (fixed):** TTL-only. Manual cleanup is a no-op (no scans).

9) `pkg/storage/repositories/metrics_repository.go:475` – `MetricsRepository.cleanupRawMetrics`
   - **Current (fixed):** TTL-only (raw metrics already store `ttl`); manual cleanup is a no-op (no scans).

10) `pkg/storage/repositories/metrics_repository.go:438` – `MetricsRepository.cleanupAggregatedMetricsByPeriod`
   - **Current (fixed):** Aggregated metrics write **GSI2** keys:
     - `gsi2PK = METRICS_AGG#<period>` (exact)
     - `gsi2SK = WINDOW#<windowStart>#TYPE#<type>#SERVICE#<service>`
     - cleanup queries `Index("gsi2")` with `gsi2PK = ...` and `gsi2SK < WINDOW#<cutoff>` (no scans).
   - **Backfill:** `cmd/tools/dynamodb-backfill-m4` sets `gsi2PK/gsi2SK` for existing `metrics_agg#...` rows.

### P0 – Table/index scans that can break auth/login flows

11) `pkg/storage/repositories/auth_repository.go:101` – `AuthRepository.GetWebAuthnCredential`
   - **Current (fixed):** query `WebAuthnCredential` via GSI1 (`gsi1PK = WEBAUTHN_CREDENTIAL#<id>`)
   - **Scan-free redesign:** use the existing `WebAuthnCredential` GSI1:
     - `Index("gsi1").Where("gsi1PK","=","WEBAUTHN_CREDENTIAL#<id>").Limit(1).All(...)`

12) `pkg/storage/repositories/account_repository_auth.go:697` – `AccountRepository.GetSessionByRefreshToken`
   - **Current (fixed):** query `Session` via GSI2 (`gsi2PK = TOKEN#hash(refreshToken)`), no scan
   - **Scan-free redesign:** treat the session “refresh token” as the indexed token:
     - store refresh token in `Session.AccessToken` (migration convenience; short-lived access tokens are JWTs)
     - `Index("gsi2").Where("gsi2PK","=","TOKEN#<sha256(refreshToken)[:16]>").Limit(1).All(...)`

13) `pkg/storage/repositories/account_repository_auth.go:816` – `AccountRepository.GetDevice`
   - **Current (fixed):** query `Device` via GSI3 (`gsi3PK = DEVICEID#<deviceID>`), no scan
   - **Scan-free redesign:** add a deviceID lookup key on the `Device` item:
     - `gsi3PK = DEVICEID#<deviceID>`
     - `gsi3SK = USER#<username>`

14) `pkg/storage/repositories/filter_repository.go:109` – `FilterRepository.GetFilter`
   - **Current (fixed):** query `Filter` via GSI1 (`gsi1PK = FILTER#<filterID>`), no scan
   - **Scan-free redesign:** add a filterID lookup key on the `Filter` item:
     - `gsi1PK = FILTER#<filterID>`
     - `gsi1SK = USER#<username>`

15) `pkg/storage/repositories/activity_repository.go:84` – `ActivityRepository.GetActivity`
   - **Current (fixed):** query `Activity` via GSI2 (`gsi2PK = ACTIVITYID#<id>`), no scan
   - **Scan-free redesign:** add an ID lookup key on the `Activity` item:
     - `gsi2PK = ACTIVITYID#<id>`
     - `gsi2SK = SK` (e.g. `ACTIVITY#<timestamp>#<id>`)

### P1 – “List all” / “count all” scans

16) `pkg/storage/repositories/relay_repository.go:164` – `RelayRepository.GetAllRelays`
   - **Current (fixed):** query relays via GSI8 (`gsi8PK = RELAYS`) and paginate by `gsi8SK`.
   - **Backfill:** `cmd/tools/dynamodb-backfill-m3` sets relay GSI8 keys for existing rows.

17) `pkg/storage/repositories/status_repository.go:450` – `StatusRepository.GetTotalStatusCount`
   - **Current (fixed):** reads the instance counter item `PK=INSTANCE#METRICS`, `SK=TOTAL_STATUSES` (no scan).
   - **Scan-free redesign:** maintain this counter on create/delete (and seed via a one-time backfill tool).

18) `pkg/storage/repositories/status_repository.go:469` – `StatusRepository.ListStatusesForAdmin` (+ related count/search)
   - **Current (fixed):** query statuses via the admin timeline on GSI8 (`gsi8PK = ADMIN_TIMELINE`) and apply filters as Query filter expressions; remote-only is handled via post-filtering (no scans).
   - **Backfill:** `cmd/tools/dynamodb-backfill-m3` sets status GSI8 keys and seeds the counters used for totals.

19) `pkg/storage/repositories/moderation_repository.go:1100` – `ModerationRepository.scanAllModerationEvents`
   - **Current (fixed):** query all moderation events via GSI4 (`gsi4PK = MODERATION_EVENTS`) and paginate by `gsi4SK` (no scans).
   - **Backfill:** `cmd/tools/dynamodb-backfill-m3` sets moderation-event GSI4 keys for existing rows.

20) `pkg/storage/repositories/circuit_breaker_repository.go:200` – `CircuitBreakerRepository.GetAllCircuitStates`
   - **Current (fixed):** query circuit states via GSI8 (`gsi8PK = CIRCUIT_STATES`) (no scans).
   - **Backfill:** `cmd/tools/dynamodb-backfill-m3` sets circuit-state GSI8 keys for existing rows.

21) `pkg/storage/repositories/streaming_connection_repository.go:326` – `StreamingConnectionRepository.GetIdleConnections`
22) `pkg/storage/repositories/streaming_connection_repository.go:590` – `StreamingConnectionRepository.GetStaleConnections`
   - **Current (fixed):** query the existing `WebSocketConnection` GSI2 state partitions (`gsi2PK = STATE#<state>`) and filter by `LastActivity` / `ttl` in memory (no scans).
   - **Future enhancement:** if ordering by `LastActivity` becomes necessary at scale, add a time-bucketed listing key (unused GSI) rather than scanning.

23) Public instance stats counts (issue #1467) – `TrendingRepository.GetActiveUserCount` / `GetTotalUserCount` / `GetTotalStatusCount` / `GetTotalDomainCount` (previously `analytics_repository.go` scans of every Activity/User/Object/Actor row into memory, unique-counted in Go) and `InstanceRepository.GetTotalUserCount` / `GetTotalDomainCount` (reads of unmaintained metrics items)
   - **Current (fixed):** all four public counts are O(1) point reads of maintained counter items:
     - `active_month` → sum of per-UTC-day `ActivityDayCounter` items (`PK=ACTIVITY_DAY#<date>`, `SK=COUNTER`), maintained by the activity write path via `ActivityActorDay` markers (`PK=ACTIVITY_ACTOR#<actor>`, `SK=DAY#<date>`) so an actor counts once per day.
     - `TOTAL_USERS` → `InstanceMetrics` item `SK=TOTAL_USERS` (`totalUsers` attr), bumped on user/account create/delete.
     - `TOTAL_STATUSES` → `InstanceMetrics` item `SK=TOTAL_STATUSES` (existing counter, see entry 17).
     - `TOTAL_DOMAINS` → `InstanceMetrics` item `SK=TOTAL_DOMAINS` (`value` attr), maintained via per-domain `DomainCounter` items (`PK=DOMAIN#<host>`, `SK=COUNTER`) on actor create/delete.
   - **Lazy one-time seed:** each counter is computed from a single scan and persisted on first read (single-flight, marker-gated). The active-month seed buckets a one-time scan by day in memory (the legacy `publishedAt` filter attribute does not resolve portably).
   - **Approximation (disclosed):** active_month is the SUM of per-day distinct actor counts — an actor active on multiple days counts once per day, so the sum can exceed the true window-distinct count. Documented as acceptable for the public stats surface. Totals can drift ±1 in the deploy-to-first-read window (seed vs. concurrent write); the error is bounded and does not accumulate.
   - **Cache:** the public `/api/v1/instance` and `/api/v2/instance` count blocks are additionally gated by a 60s instance-local TTL cache (success-only) so bursts collapse to one compute.

### P1 – Partition-key prefix/range misuse (guaranteed scans)

23) `pkg/storage/repositories/trust_repository.go:240` – `TrustRepository.GetTrustRelationships`
24) `pkg/storage/repositories/trust_repository.go:274` – `TrustRepository.GetTrustedByRelationships`
   - **Current (fixed):** enumerate the small, fixed category set and query each exact partition:
     - outgoing: `PK = TRUST#<truster>#<category>`
     - incoming: `Index("gsi1")` + `gsi1PK = TRUSTED#<trustee>#<category>`
   - Cursor is a base64url-encoded JSON blob `{category,last_sk}` (no TableTheory scan cursors, no partition-key prefixes).

25) `pkg/storage/repositories/bookmark_repository.go:414` – `BookmarkRepository.CascadeDeleteObjectBookmarks`
   - **Current (fixed):** query the OBJECT record index on **GSI8**:
     - OBJECT records write `gsi8PK = BOOKMARK_OBJECT#<objectID>` and `gsi8SK = USER#<username>#TIME#<createdAt>`
     - cascade delete queries `Index("gsi8")` by that partition and deletes both OBJECT + TIME rows using `TimeRecordSK` (no scans).
   - **Backfill:** `cmd/tools/dynamodb-backfill-m4` sets bookmark `gsi8PK/gsi8SK` for existing OBJECT rows.

26) `pkg/storage/repositories/query_cache_repository.go:117` – `QueryCacheRepository.invalidateCachePrefix`
   - **Current (fixed):** cache items are keyed:
     - `PK=CACHE#<namespace>` where `namespace` is the substring before `:` in the cache key
     - `SK=KEY#<cacheKey>`
     - prefix invalidation is `Query PK=CACHE#<namespace>` + `SK begins_with KEY#<prefix>` (no scans).

27) `pkg/storage/repositories/rate_limit_repository.go:669` – `RateLimitRepository.IsDomainBlocked` (via `checkBlockedStatus`)
   - **Current (fixed):** store a domain-wide lockout record:
     - `PK=RATELIMIT#DOMAIN#<domain>`, `SK=LOCKOUT` (RateLimitLockout; TTL matches unlock time)
     - `IsDomainBlocked` calls `IsRateLimited("DOMAIN#<domain>")` (no scans)
     - `CheckFederationRateLimit` writes/extends the lockout whenever a domain is blocked.

28) `pkg/storage/repositories/federation_repository.go:1873` – `FederationRepository.GetSeveredRelationships`
   - **Current (fixed):** query by exact `PK=SEVERED#<localInstance>` and paginate by `SK` (no scans).

29) `pkg/storage/repositories/federation_repository.go:2473` – `FederationRepository.GetStrongestConnectionsByType`
   - **Current (fixed):** FederationEdge writes a strongest-by-type listing key on **GSI8**:
     - `gsi8PK = FED_EDGES#TYPE#<connectionType>`
     - `gsi8SK = STRENGTH#<padded>#LAST#<unix>#SRC#<src>#TGT#<tgt>`
     - query `Index("gsi8")` by exact `gsi8PK` and order by `gsi8SK DESC` (no scans).
   - **Backfill:** `cmd/tools/dynamodb-backfill-m4` sets `gsi8PK/gsi8SK` for existing edge rows.

30) `pkg/storage/repositories/instance_repository.go:606` – `InstanceRepository.countLocalComments`
   - **Current (fixed):** returns `PK=INSTANCE#METRICS`, `SK=LOCAL_COMMENTS` only (no fallback scan).
   - **Scan-free redesign:** maintain `LOCAL_COMMENTS` as a real-time counter (no scan fallback). Historical recounts should be done offline via `cmd/tools/dynamodb-backfill-m3`.

### P2 – Time-range scans due to key design (fixable with bucketing)

31) `pkg/storage/repositories/ai_cost_repository.go:88` – `AICostRepository.GetAICostsByTimeRange`
   - **Current (fixed):** bucketed month partitions on GSI1 (no scans):
     - `gsi1PK = AI_COSTS#YYYY-MM`
     - `gsi1SK = TS#<unix_millis>#TYPE#<operationType>#OP#<operationID>`
     - query is `Index("gsi1")` + `gsi1PK = ...` + `gsi1SK BETWEEN TS#start..TS#end~` per month bucket.
   - **Backfill:** `cmd/tools/dynamodb-backfill-m5` sets the new `gsi1PK/gsi1SK` keys for existing AI cost rows.

32) `pkg/storage/repositories/federation_cost_repository.go:80` – `FederationCostRepository.GetFederationCosts`
   - **Current (fixed):** domain + month buckets on GSI1 (no scans):
     - `gsi1PK = FED_COSTS#DOMAIN#<domain>#YYYY-MM`
     - `gsi1SK = TS#<unix_millis>#TYPE#<activityType>#ID#<activityID>`
     - query is `Index("gsi1")` + exact `gsi1PK` + `gsi1SK` range per month bucket.
   - **Backfill:** `cmd/tools/dynamodb-backfill-m5` sets the new `gsi1PK/gsi1SK` keys for existing federation cost rows.

33) `pkg/storage/repositories/analytics_repository.go:1396` – `TrendingRepository.GetEngagementByDateRange`
   - **Current (fixed):** engagement metrics are queried by existing day-bucketed PKs (no scans):
     - for each `date` in `[startDate,endDate]`, `Query PK = METRICS#<type>#<date>`
     - merge results and apply the caller’s `limit`.

34) `pkg/storage/repositories/analytics_repository.go:1659` – `TrendingRepository.PruneStaleTrends`
   - **Current (fixed):** TTL-only. Manual cleanup is a no-op (no scans).

---

## Reusable redesign patterns

### 1) “Expiry index” pattern (when TTL is not enough)

If you need deterministic deletion (not eventual TTL), add a queryable expiry index using an existing unused GSI for that model type:

- `gsi8PK = EXPIRY#<entityType>#<YYYY-MM-DD>`
- `gsi8SK = TS#<unix>#PK#<pk>#SK#<sk>`

Then a cleanup job queries `gsi8PK = ...` for old days and batch-deletes.

### 2) Reverse index item pattern (attribute → PK/SK)

For lookups like `filterID → user`, `deviceID → user`, `activityID → actor`, add a small index row:

- `PK = <LOOKUP_KIND>#<id>`
- `SK = <TARGET_PK>` (or `USER#username`, etc.)

This avoids adding new table GSIs and keeps lookups O(1).

### 3) Global listing pattern (admin pages / dashboards)

If you ever need “list all X”, do not scan:

- pick an unused GSI for that model type (often `gsi8`)
- set:
  - `gsi8PK = <COLLECTION_NAME>`
  - `gsi8SK = <sortable key, usually TIME#...>`

Then list pages are a query with cursor pagination.

---

## Roadmap: eliminate every scan in this inventory

This roadmap is scoped to the specific call sites listed above (items 1–34). The intent is that each milestone is “shippable”: after each milestone, you should be able to deploy and be strictly better than before (less cost, lower latency, lower blast radius).

### Milestone M0 (P0): stop data-loss patterns (scan + delete)

**Covers inventory items:** 1–2

**Why first:** these patterns already caused catastrophic deletion (`USER#simulacrum` and `ACTOR#simulacrum`). They must be eliminated before any other scan reductions.

**Implementation guide**

1) **Remove scan-based trend cleanup**
   - `pkg/storage/repositories/analytics_repository.go`:
     - delete or rewrite `TrendingRepository.deleteOldTrendsGeneric` so it cannot scan/filter on non-key attributes
     - if deletion remains, only delete items whose `PK` begins with the expected `TREND_TYPE#<TYPE>#` prefix
   - `cmd/trend-aggregator/main.go`:
     - stop calling the scan-based cleanup path entirely (preferred: rely on TTL)
     - if deterministic deletion is required, delete by querying the **known trend partitions** (date-bucketed `PK`s) and batch deleting those results

2) **Remove scan-based hashtag trend cleanup helpers**
   - `pkg/storage/repositories/hashtag_batch_helpers.go`:
     - remove `Filter(...).Scan(...)` patterns entirely
     - prefer TTL-only; if deterministic deletion is required, delete by querying known buckets (date / hashtag partition keys), not by filtering `UpdatedAt/UsedAt` with a scan

3) **Add “safe delete” guardrails anywhere deletion is driven by query results**
   - Before deleting any item returned from a query:
     - verify `PK` matches the intended entity prefix (and, when possible, `SK` as well)
     - if it doesn’t, **skip** + **log a warning** with the unexpected `PK/SK`

**Acceptance criteria**

- `pkg/storage/repositories/analytics_repository.go` no longer calls `.Scan(...)` for trend cleanup, and cannot delete items with `PK` outside `TREND_TYPE#...`.
- `pkg/storage/repositories/hashtag_batch_helpers.go` no longer performs `.Scan(...)` to find deletions by non-key attributes.
- The `trend-aggregator` scheduled Lambda performs **no DynamoDB Scan** operations against `simulacrum-*-main-table` (verify via CloudWatch DynamoDB metrics: `SuccessfulRequestLatency`/`ConsumedReadCapacityUnits` split by `Operation=Scan`, and/or by instrumenting TableTheory executor logging).

### Milestone M1 (P0): eliminate scan-to-delete “expired” rows (TTL / expiry-index only)

**Covers inventory items:** 3–9, 34

**Why:** “scan to delete expired” is an anti-pattern in DynamoDB. TTL exists to do exactly this, and expiry queries require an explicit index pattern.

**Implementation guide**

1) **Default strategy: TTL-only**
   - For each cleanup method in items 3–9 and 34:
     - remove the scan path and let TTL remove the item eventually
     - ensure business logic treats records as expired based on timestamp, not on physical deletion (i.e., don’t rely on “row must be gone”)

2) **When deterministic deletion is required: implement an “expiry index”**
   - Use the pattern already described in this doc:
     - `gsi8PK = EXPIRY#<entityType>#<YYYY-MM-DD>`
     - `gsi8SK = TS#<unix>#PK#<pk>#SK#<sk>`
   - Cleanup job:
     - queries old `gsi8PK` partitions (per day)
     - batch deletes the referenced items
   - This is still scan-free because it is a **Query** per day-bucket, not a Scan.

3) **Remove scheduled jobs that exist only to compensate for missing TTL**
   - If the job only existed because TTL wasn’t written consistently, fix the models so they always write `ttl` and delete the job logic.

**Acceptance criteria**

- Items 3–9 and 34 no longer execute DynamoDB Scan operations on the main table or any GSI.
- A quick grep-based guardrail can be met for these files: no `.Scan(` remains in the specific cleanup functions.
- Production behavior remains correct even if TTL deletion lags (no “expired but still present” correctness issues).

### Milestone M2 (P0): auth/login lookups must be key-based (no scans)

**Covers inventory items:** 11–15

**Why:** auth flows are latency-sensitive and reliability-critical. Any scan in a login path becomes a user-facing outage as the table grows.

**Implementation guide**

1) **WebAuthn credential lookup**
   - `pkg/storage/repositories/auth_repository.go` (`GetWebAuthnCredential`):
     - replace scan-by-attribute with a GSI query using `WebAuthnCredential.GSI1PK = WEBAUTHN_CREDENTIAL#<id>`
     - use `.Index("gsi1").Where("gsi1PK","=",...).Limit(1).All(...)`

2) **Session lookup by refresh token**
   - `pkg/storage/repositories/account_repository_auth.go` (`GetSessionByRefreshToken`):
     - stop scanning `Session` records
     - treat the session refresh token as the indexed token (stored in `Session.AccessToken`)
     - query `Index("gsi2")` with `gsi2PK = TOKEN#<sha256(refreshToken)[:16]>`
   - Ensure refresh token rotation updates the indexed token field and recomputes GSI keys.

3) **Device lookup by deviceID**
   - Add a deviceID lookup key on the `Device` item:
     - `gsi3PK = DEVICEID#<deviceID>`
     - `gsi3SK = USER#<username>`
   - `GetDevice(deviceID)` becomes a `gsi3` query (`gsi3PK = DEVICEID#...`) with `Limit(1)`.

4) **Filter lookup by filterID**
   - Add a filterID lookup key on the `Filter` item:
     - `gsi1PK = FILTER#<filterID>`
     - `gsi1SK = USER#<username>`
   - `GetFilter(filterID)` becomes a `gsi1` query (`gsi1PK = FILTER#...`) with `Limit(1)`.

5) **Activity lookup by activity ID**
   - Add an ID lookup key on the `Activity` item:
     - `gsi2PK = ACTIVITYID#<id>`
     - `gsi2SK = SK` (for stable uniqueness and optional ordering)
   - Update `CreateActivity` to write these keys with the activity record.

6) **Backfill (one-time)**
   - For any new GSI keys, run a one-time backfill job that scans existing rows *outside* production request paths.
   - Tool: `cmd/tools/dynamodb-backfill-m2`
     - Example: `go run ./cmd/tools/dynamodb-backfill-m2 --table <your-table-name> --region us-east-1 --dry-run=false`

**Acceptance criteria**

- Items 11–15 execute **zero** DynamoDB Scan operations.
- Login flows succeed with only `GetItem` / `Query` operations (verify via logs + DynamoDB metrics by operation).
- A backfill procedure exists (documented command/tool) for the newly-added index keys.

### Milestone M3 (P1): “list all / count all” must be index-backed (no scans)

**Covers inventory items:** 16–22, 30

**Why:** these endpoints are classic scan traps (admin pages, dashboards, cleanup of stale WS connections). They’ll grow without bound.

**Implementation guide**

1) **Global listing indexes (Relays, Moderation events, Circuit states)**
   - For each entity needing “list all”:
     - pick an unused GSI field on that model (commonly `gsi8`)
     - write a **single partition** suitable for global listing:
       - Relays: `gsi8PK = RELAYS`, `gsi8SK = URL#<url>`
       - Moderation events: `gsi4PK = MODERATION_EVENTS`, `gsi4SK = TIME#<rfc3339>#<id>`
       - Circuit states: `gsi8PK = CIRCUIT_STATES`, `gsi8SK = INSTANCE#<instanceID>`
     - query that partition for pagination (no scans)

2) **Total counts (Statuses, Local comments)**
   - Replace scan-based counts with counter items:
     - `PK=INSTANCE#METRICS`, `SK=TOTAL_STATUSES` (fields: `totalStatuses` + `value`)
     - `PK=INSTANCE#METRICS`, `SK=LOCAL_COMMENTS` (field: `value`)
   - Update counters atomically on create/delete paths.
   - For this prototype, provide a one-time “recount” tool (offline) if counters ever drift.

3) **Admin status listing**
   - Replace scan-with-filters with an “admin timeline” index:
     - `gsi8PK = ADMIN_TIMELINE` (and optionally `ADMIN_TIMELINE#local` / `ADMIN_TIMELINE#remote`)
     - `gsi8SK = <publishedAt_unix>#<statusID>`
   - If you need extra filters (visibility/media/flagged), encode them in:
     - separate partitions (preferred) or
     - sort-key prefixes that can be ranged/prefixed (limited).

4) **WebSocket connection cleanup**
   - Replace full scans with queries on the existing connection state index:
     - `Index("gsi2").Where("gsi2PK","=","STATE#connected")` etc.
   - If “idle/stale by last activity” needs ordering, add a time-bucketed listing key (e.g. per day) rather than scanning.

5) **Backfill (one-time)**
   - For any new GSI keys or newly-introduced counters, run a one-time backfill outside production request paths:
     - Tool: `cmd/tools/dynamodb-backfill-m3`
     - Example:
       - `go run ./cmd/tools/dynamodb-backfill-m3 --table <your-table-name> --region us-east-1 --dry-run=false --local-domain <your-domain>`

**Acceptance criteria**

- Items 16–22 and 30 execute **zero** DynamoDB Scan operations.
- Admin listing endpoints paginate via Query on a single partition key (table or GSI).
- Global counts do not call `.Count()` on a scan-filtered query.

### Milestone M4 (P1): eliminate partition-key prefix/range misuse (guaranteed scans)

**Covers inventory items:** 10, 23–29

**Why:** `begins_with` / range comparisons on a **partition key** force index scans; DynamoDB can only range on the sort key.

**Implementation guide**

1) **Aggregated metrics cleanup (partition key redesign)**
   - `pkg/storage/repositories/metrics_repository.go`:
     - redesign `AggregatedMetrics` keying so the partition key is **exact** for the period:
       - example: `gsi2PK = METRICS_AGG#<period>` (exact)
       - `gsi2SK = WINDOW#<windowStart>#TYPE#<metricType>#SERVICE#<service>`
     - cleanup becomes: query `gsi2PK = METRICS_AGG#<period>` and range on `gsi2SK < cutoff`

2) **Trust relationships**
   - Enumerate the small, fixed set of categories and query each exact partition:
     - `PK=TRUST#<truster>#<category>` (exact)
     - reverse: `gsi1PK=TRUSTED#<trustee>#<category>` (exact)

3) **Bookmarks cascade delete**
   - Add an object→bookmark index (GSI partition on the OBJECT record) so you can query “all bookmarks for object X” without scanning.

4) **Query cache invalidation**
   - Change cache schema so invalidation is `Query PK = CACHE#<namespace>` plus an `SK` prefix/range, not `PK begins_with`.

5) **Rate limit domain blocking**
   - Store an explicit domain block record at an exact key:
     - `PK=RATELIMIT#DOMAIN#<domain>`, `SK=LOCKOUT` (RateLimitLockout; TTL matches unlock time)

6) **Federation severed relationships**
   - Ensure the query is by exact `PK=SEVERED#<localInstance>` with `SK` pagination; remove `PK begins_with` filters.

7) **Federation “strongest connections”**
   - Add a type-partitioned listing key (GSI):
     - `gsi8PK = FED_EDGES#TYPE#<connectionType>`
     - `gsi8SK = STRENGTH#<padded>#LAST#<unix>#SRC#...#TGT#...`

8) **Backfill (one-time)**
   - Tool: `cmd/tools/dynamodb-backfill-m4` backfills the new aggregated-metrics and federation-edge index keys (and bookmark object index keys) for existing rows.

**Acceptance criteria**

- Items 10 and 23–29 have no `begins_with` / ranges on partition keys in their query builders.
- These codepaths execute **zero** DynamoDB Scan operations.

### Milestone M5 (P2): time-range queries must be bucketed (no scans)

**Covers inventory items:** 31–33

**Why:** “time range” across a huge keyspace needs bucketing in the partition key; range belongs in the sort key.

**Implementation guide**

1) **AI cost time-range queries**
   - Redesign the `AICost` GSI1 schema:
     - `gsi1PK = AI_COSTS#YYYY-MM` (or `YYYY-MM-DD` if volume is high)
     - `gsi1SK = TS#<unix>#TYPE#<operationType>#OP#<operationID>`
   - Query each month bucket in `[start,end]` using `gsi1SK BETWEEN ...` and merge results.

2) **Federation cost time-range + domain filtering**
   - Bucket by domain + month on GSI1:
     - `gsi1PK = FED_COSTS#DOMAIN#<domain>#YYYY-MM`
     - `gsi1SK = TS#<unix_millis>#TYPE#<activityType>#ID#<activityID>`
   - Query each month bucket in `[start,end]` using `gsi1SK BETWEEN ...` and merge results.

3) **Engagement by date range**
   - Prefer iterating known date buckets and querying exact PKs (existing `METRICS#type#date` pattern).
   - If using a GSI, ensure the query is `Index("...")` + exact `gsiNPK`, then range on `gsiNSK`.

4) **Backfill (one-time)**
   - Tool: `cmd/tools/dynamodb-backfill-m5` backfills the new AI-cost and federation-cost GSI1 keys for existing rows.

**Acceptance criteria**

- Items 31–33 do not perform partition-key ranges and do not execute DynamoDB Scan operations.
- Queries are O(number of time buckets), not O(size of table/index).

### Milestone M6: enforcement (prevent regressions)

**Covers:** all milestones

**Implementation guide**

1) **Codebase guardrails**
   - Enforce via `./lesser verify audit` (runs `go run ./tools/audit_gates --check`) with two baselined gates:
     - `goDynamoDBQueryScan`: counts TableTheory `Query.Scan(...)` in non-test Go code (prevents adding new scan callsites)
     - `goDynamoDBBadPKWhere`: counts `Where("PK"/"gsiNPK", "begins_with|>=|...")` misuse (prevents partition-key prefix/range regressions)
   - Allowlist only explicit one-time backfill tools by skipping `cmd/tools/` in these audits.
   - To regenerate the baseline snippet for these gates: `go run ./tools/audit_gates --dump-dynamodb-baseline`.

2) **Runtime observability**
   - Instrument the TableTheory executor/wrapper to log and/or emit metrics on Scan usage:
     - include table name, index name (if any), and a request correlation ID
   - Add a CloudWatch alarm for `Operation=Scan` in production environments.

**Acceptance criteria**

- CI prevents new scan patterns from landing in production code paths.
- In deployed environments, Scan operations trend to (and remain at) ~0 for application lambdas.

---

## Milestone map (inventory → milestone)

- **M0:** 1–2
- **M1:** 3–9, 34
- **M2:** 11–15
- **M3:** 16–22, 30
- **M4:** 10, 23–29
- **M5:** 31–33
- **M6:** enforcement for all milestones

---

## Verification checklist (practical)

### Static checks (cheap, fast)

- **No explicit scans in production code paths:**
  - `rg -n "\\.Scan\\(" pkg cmd graph`
  - After M6, this should be empty (or match only allowlisted one-time tooling paths, if any exist).
- **No partition-key prefix/range misuse in new code:**
  - `rg -n "Where\\(\\\"(PK|gsi[0-9]+PK)\\\",\\s*\\\"(begins_with|>=|<=|<|>)\\\"" pkg/storage/repositories`

### Repository-level verification (per milestone)

- **M0:** no `.Scan(` remains in:
  - `pkg/storage/repositories/analytics_repository.go`
  - `pkg/storage/repositories/hashtag_batch_helpers.go`
- **M1:** the cleanup methods in items 3–9 and 34 are either removed/no-op (TTL-only) or query an expiry index (no scans).
- **M2:** auth lookup methods in items 11–15 use only key-based reads (`GetItem` / `Query`) and do not scan.
- **M3:** admin listing/count endpoints in items 16–22 and 30 are backed by Queryable keys (GSI/global listing partitions and counter items).
- **M4/M5:** no `begins_with`/range on partition keys and no Scan operations remain for the listed methods.

### AWS-side verification (what to look at)

- **DynamoDB CloudWatch metrics** for the main table:
  - `ConsumedReadCapacityUnits` and `SuccessfulRequestLatency`
  - dimensioned by `Operation` (ensure `Operation=Scan` is ~0 in steady-state)
  - and, if available, by `GlobalSecondaryIndexName` to detect index scans.

### Functional regression tests

- Run the normal suite after each milestone:
  - `./lesser test unit`
  - `./lesser test` (or `go test ./...` if you want the direct Go runner)
