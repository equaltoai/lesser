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
   - **No request-adjacent scan (operator doctrine 2026-08-26):** the lazy one-time seed scans introduced by #1468 are REMOVED. No code path reachable from an HTTP handler ever calls `All()` on User/Actor/Activity — reads are point reads (or sums of point reads) of the maintained counters, and an unseeded counter reads as the documented default (0).
   - **Counters are seeded/maintained off the request path:** write-path maintenance (create/delete bumps) plus the offline recount tool below. A fresh instance never scans on read; an already-broken instance (whose v1.6.24 lazy seed never completed) is unblocked by running the recount offline.
   - **Approximation (disclosed):** active_month is the SUM of per-day distinct actor counts — an actor active on multiple days counts once per day, so the sum can exceed the true window-distinct count. Documented as acceptable for the public stats surface (and in the `NodeInfoUsers.activeMonth` OpenAPI description). Unseeded tables read as zeros until the write path or the offline recount populates them; the public surface is eventually consistent via maintained writes + offline recount.
   - **The recount tool is the sanctioned seed mechanism:** `RecountInstanceCounts` (CLI: `lesser recount-instance-counts`, offline, `--apply` to write) recomputes `TOTAL_USERS` / `TOTAL_DOMAINS` / per-domain `DomainCounter` items AND the active-month per-UTC-day rollup (+ the `SEED#ACTIVE_MONTH` marker) from bounded paginated key-only projections and rewrites them. Semantic note: the recount writes the true `USER#`/`METADATA` account count; the legacy `GetTotalUserCount` counted every item a scan surfaced, so a recount can correct that legacy-inherited value once. Domain semantics agree (only real actor rows carry the `actor` attribute); the active-month rollup covers the 200-day retention window so the widest reader window (180 days) is served.
   - **`TOTAL_STATUSES` source shift on federated instances (disclosed):** the legacy `GetTotalStatusCount` scanned `Object` rows with `Type = "Note"`; the counter counts canonical `Status` rows maintained by the status write path. Federated Notes that are stored **only** as Object rows — the activity-processor announce-fetch path (`cmd/activity-processor` `storeRemoteObject`) and the thread-sync reply path (`pkg/federation/sync` `storeNote`) — do NOT go through `StatusRepository.CreateStatus` and are therefore not counted. Inbox Notes materialized via `federation.MaterializeRemoteNote` DO create a Status and are counted. On federated instances with object-only Notes, the counter reads lower than the legacy scan; this is the intended semantic (the counter reflects actual statuses), documented here and in the PR body.
   - **Cache:** the public `/api/v1/instance` and `/api/v2/instance` count blocks are gated by a 60s instance-local TTL cache. The cache is success-only (a failed read is never cached as zeros — a previous value is served stale instead), and computes under a per-process mutex so concurrent misses within one instance collapse.

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

## Baselined scan backlog — dispositions (operator doctrine 2026-08-25)

The audit gates (`./lesser verify audit`) baseline every remaining scan callsite so CI fails on any new or changed occurrence. This section gives every baselined callsite a disposition: **fixed-in-PR**, **deliberate (documented) tooling**, or **tracked for elimination** under the umbrella issue [Eliminate baselined unbounded scans — operator doctrine 2026-08-25](https://github.com/equaltoai/lesser/issues/1469).

### `goDynamoDBAllNoKey` (key-less `All(...)` on fresh chains — scan with no key condition)

**Gate gap — Where-clause blindness (closed in wave part 1, umbrella #1469):** until 2026-08-26 the detector treated *any* `Where(...)` in the chain as proof of a key condition, so every `.All()` chain that carried a filter was invisible to the gate and absent from this baseline. The detector now counts a fresh `Model(...)`/`WithContext(...)` `.All()` chain whenever no `Where(...)` constrains a partition key (`PK`/`gsiNPK`/`oauthClientsPK`) with equality — the only shape TableTheory compiles to a DynamoDB Query (its `partitionConditionsForKeys` demotes a partition key with any operator other than `=` to a filter condition, which still scans). Sort-key ranges, non-key attribute predicates, and partition-key operators other than `=` are filters, not bounds, and are counted. Chains whose `Where(...)` field/operator is not a string literal, and chains on pre-built query variables, remain statically indeterminate and are deliberately not flagged. Of the 5 indeterminate sites identified during this closure, 2 are **runtime GSI partition-key queries** (`alert_repository.go:181`, `metrics_repository.go:591` — the field name is caller-supplied configuration, and both callers pass index/field pairs whose partition attribute matches the model, so the query genuinely binds a key); the other 3 (`base_repository.go:1235` `QueryHistoryWithDateRange`, `:1292` `QueryMetricsByTimeRange`, `:1516` `FindBySK`) were **production-unused helpers** — zero callers anywhere in the repo outside their own coverage tests — whose queries were bounded *only* when a caller passed matching index/field configuration. The 3 dead helpers and their coverage tests are **deleted in this PR** (2026-08-26 rework), shrinking the gate's blind surface; the 2 runtime GSI queries remain. Result: 31 sites newly visible, 45 total baselined (5 deliberate + 40 elimination-pending). **Wave part 2 (batch A, 2026-08-27) eliminated 13 request-adjacent sites** (see the disposition table below: CMS fallbacks deleted, dead `GetWeeklyActivity` deleted, GSI-keyed rewrites for scheduled status / announcement dismissals / API rate limits / reviewer stats / filter CRUD / quality-change events / pattern statistics / pattern listing); the `goDynamoDBAllNoKey` baseline now stands at **32 sites (5 deliberate offline + 27 elimination-pending)**.

**Model-blind key trust (disclosed, wave part 1 rework):** the detector treats any literal `Where("gsiNPK","=",…)` (or `Where("oauthClientsPK","=",…)`) as a bound without checking that the model actually declares the index — ~250 unflagged sites rely on the uniform lowercase naming convention, and a query whose index/field pair is not declared by the model would compile to a Scan (or fail loudly at the real DynamoDB), not a Query. This is accepted deliberately: the flagged set must stay free of false positives, every recognized partition-key field name matches the lowercase convention the models actually write, and the sibling `goDynamoDBBadPKWhere` gate keeps the same names consistent.

| Callsite | What it does | Disposition |
|---|---|---|
| `pkg/storage/repositories/instance_counts.go` (5) | 5 bounded offline recount reads (`RecountInstanceCounts` key-only projections: users, actors, domain counters, activities, existing day counters). The 3 request-adjacent lazy seed scans were removed (PR #1476) — no scan runs on any request path | **Deliberate, documented** (entry 23): offline `lesser recount-instance-counts` tool only; the recount is the sanctioned seed mechanism (it also writes the active-month rollup + `SEED#ACTIVE_MONTH` marker) |
| `pkg/storage/repositories/announcement_repository.go:500` (1) | scans dismissals for cleanup (no GSI) | **Fixed in wave part 2 (batch A, #1469):** `AnnouncementDismissal` gained GSI1 (`ANN_DISMISSED#<announcementID>` / `USER#<username>`) maintained by `UpdateKeys`; `DeleteAnnouncement` cleanup queries `Index("gsi1")` by announcement and deletes the returned rows. Legacy dismissal rows without gsi1 keys are left as harmless orphan rows (documented) |
| `pkg/storage/repositories/dlq_repository.go:632` (1) | `GetSimilarMessages` filter-scan by `SimilarityHash` | elimination pending — wave #1469 |
| `pkg/storage/repositories/media_repository.go:1438` (1) | full `Media` scan | elimination pending — wave #1469 |
| `pkg/storage/repositories/moderation_repository.go:1302` (1) | scans reviews by reviewer (no reviewer index) | **Fixed in wave part 2 (batch A, #1469):** `ModerationReview` gained GSI1 (`REVIEWER#<reviewerID>` / `TIME#<created>#REVIEW#<event_id>`) set in `UpdateKeys`; `GetReviewerStats` queries `Index("gsi1")` by reviewer. Legacy reviews without gsi1 are TTL-transient (~30d) and undercount stats until written post-change |
| `pkg/storage/repositories/rate_limit_repository.go:225` (1) | clears rate limits via `PK begins_with` filter-scan | **Fixed in wave part 2 (batch A, #1469):** `APIRateLimit` gained GSI1 (`USER_RATELIMIT#<userID>` / `ENDPOINT#<endpoint>#WINDOW#<windowStart>`) set in `UpdateKeys`/`NewAPIRateLimit` and refreshed in `updateAPIRateLimit`; `ClearAPIRateLimitsForUser` queries `Index("gsi1")` and batch-deletes. Legacy rows are TTL-transient (~24h) and expire instead of being cleared |
| `pkg/storage/repositories/search_cost_repository.go:233,270` (2) | scans `SearchQueryStats` by period | elimination pending — wave #1469 |
| `pkg/storage/repositories/search_repository.go:1614,1720` (2) | prunes old suggestions / scans by `last_used` | elimination pending — wave #1469 |
| `graph/query_resolvers_cms.go:999` (1) | `cmsScanSeriesBySlug` back-compat fallback: `SK BEGINS_WITH ID#` scan | **Fixed in wave part 2 (batch A, #1469):** fallback scan deleted; slug lookup uses the tenant-scoped/global slug index + viewer-scoped list (both backfill the index on write). Legacy rows without an index entry are not found by slug until backfilled (documented) |
| `graph/query_resolvers_cms.go:1348` (1) | CMS membership back-compat fallback: `SK = USER#…` scan | **Fixed in wave part 2 (batch A, #1469):** fallback scan deleted; `MyPublications` uses the GSI1 membership path only. Legacy membership rows without gsi1 keys are not listed until backfilled (documented) |
| `pkg/federation/cost/repository_adapter.go:319` (1) | `ListInstanceConfigs`: scan by `Type = InstanceConfig` | **Fixed in wave part 2 (batch E, #1469):** `FederationInstanceConfigTracking` gained GSI3 global listing (`INSTANCE_CONFIGS#ALL` / `INSTANCE#<domain>`) maintained by `UpdateKeys` (the single writer `SaveInstanceConfig` calls it); `ListInstanceConfigs` queries `Index("gsi3")`. Legacy config rows without gsi3 are not listed until next written (configs are permanent, updated on save) |
| `pkg/storage/repositories/account_repository_auth.go:443` (1) | `GetUserByRecoveryCode`: `SK BEGINS_WITH RECOVERY_CODE#` scan (request-adjacent: recovery-code login) | elimination pending — wave #1469 |
| `pkg/storage/repositories/account_repository_refresh_tokens.go:346` (1) | `CleanupExpiredAdvancedTokens`: `SK = TOKEN` scan | elimination pending — wave #1469 |
| `pkg/storage/repositories/activity_repository.go:492` (1) | weekly activity time-range scan (`CreatedAt` range) | **Fixed in wave part 2 (batch A, #1469):** `ActivityRepository.GetWeeklyActivity` had zero production callers (the request path uses `InstanceRepository.GetWeeklyActivity`, a point read of the `INSTANCE#ACTIVITY`/`ACTIVITY#WEEK#<date>` item); the dead method + interface entry + mocks were deleted |
| `pkg/storage/repositories/activity_repository.go:570` (1) | `GetHashtagActivity`: `CreatedAt >= since` scan | elimination pending — wave #1469 |
| `pkg/storage/repositories/analytics_repository.go:633` (1) | hashtag metadata scan by `SK = METADATA` (filters `LastUsed`) | **elimination pending — wave #1469** (batch E GSI1 attempt reverted 2026-08-27): the wave initially gave `Hashtag` a GSI1 global listing (`HASHTAGS#ALL` / `<LastUsed RFC3339>#<name>`) and rerouted `GetRecentHashtags` to it, but the only writer of hashtag metadata rows is `HashtagRepository.IndexHashtag`, which has **zero production callers** — no live writer would maintain the index, so keying the read on it would silently return nothing. Reverted to the baselined `SK = METADATA` scan with a "no live metadata writer — no rows exist" disposition |
| `pkg/storage/repositories/analytics_repository.go:669` (1) | `GetRecentStatusesWithEngagement`: `EngagedAt >= since` scan | **Fixed in wave part 2 (batch E, #1469):** `StatusEngagement` gained GSI1 global listing (`ENGAGEMENTS#ALL` / `<EngagedAt RFC3339>#<statusID>#<userID>`) maintained by `UpdateKeys` (both writers — `RecordStatusEngagement` and `StatusRepository.createEngagementAndIncrement` — now call it, unifying the previously divergent SK timestamp formats on new rows); the read is `Index("gsi1")` with a `gsi1SK >=` window. Legacy rows without gsi1 are TTL-transient (7d) and not returned until next written |
| `pkg/storage/repositories/analytics_repository.go:735` (1) | `GetRecentLinks`: `SharedAt >= since` scan | **Fixed in wave part 2 (batch E, #1469):** `LinkShare` gained GSI1 global listing (`LINK_SHARES#ALL` / `<SharedAt RFC3339>#<url>#<statusID>`) maintained by `UpdateKeys` (the single writer `RecordLinkShare` now calls it); the read is `Index("gsi1")` with a `gsi1SK >=` window. Legacy rows without gsi1 are TTL-transient (7d) and not returned until next written |
| `pkg/storage/repositories/analytics_repository.go:2193` (1) | `queryQualityChangeEvents`: `PK begins_with QUALITY_CHANGE#` scan | **Fixed in wave part 2 (batch A, #1469):** `MediaAnalytics` quality-change rows now carry GSI3 (`MEDIA_QUALITY#<mediaID>` / `TS#<unix>#<quality>`), set in `SetQualityChange`; the count query is `Index("gsi3")` per media with a `gsi3SK >=` window. Legacy rows are TTL-transient (7d) |
| `pkg/storage/repositories/analytics_repository.go:2740,2756` (2) | engagement scans by `SK begins_with like#` | elimination pending — wave #1469 |
| `pkg/storage/repositories/enhanced_pattern_repository.go:719` (1) | `CleanupExpiredPatterns`: `SK = METADATA` scan | elimination pending — wave #1469 |
| `pkg/storage/repositories/enhanced_pattern_repository.go:752` (1) | `GetPatternStatistics`: `SK = METADATA` scan | **Fixed in wave part 2 (batch A, #1469):** `EnhancedModerationPattern` gained GSI4 (`ENHANCED_PATTERNS#ALL` / `<updated_at>#<pattern_id>`) set in `UpdateKeys` (write paths `CreatePattern`/`UpdatePattern` refresh keys); `GetPatternStatistics` queries `Index("gsi4")` |
| `pkg/storage/repositories/federation_activity_repository.go:145` (1) | recent federation activities: GSI1 sort-key range scan (`gsi1SK >= since`, no `gsi1PK`) | **Fixed in wave part 2 (batch E, #1469):** `FederationActivity` gained GSI3 global listing (`FED_ACTIVITY#ALL` / `<RFC3339>#<domain>#<id>`) maintained by `setupGSIKeys` on every write (`RecordFederationActivity` → `ValidateAndCreate` → `UpdateKeys`); `GetRecentActivities` queries `Index("gsi3")` with a `gsi3SK >=` window. Legacy activities without gsi3 are TTL-transient (90d) and not returned until next written |
| `pkg/storage/repositories/filter_repository.go:343` (1) | filter-status lookup by `SK` pattern | elimination pending — wave #1469 |
| `pkg/storage/repositories/hashtag_trending_engine.go:351` (1) | `getCandidateHashtags`: `SK = METADATA` + `LastUsed` filter | **elimination pending — wave #1469** (batch E GSI1 attempt reverted 2026-08-27): the wave initially rerouted this to the new `Hashtag` GSI1 `HASHTAGS#ALL` key, but that index has no live writer (see the `analytics_repository.go:633` row — hashtag metadata rows are only written by the production-unused `IndexHashtag`), so it would never populate. Reverted to the baselined `SK = METADATA` scan with the no-live-writer disposition |
| `pkg/storage/repositories/instance_health_repository.go:519` (1) | health summaries: `SK = SUMMARY#1h` scan | **Fixed in wave part 2 (batch E, #1469):** `InstanceHealthSummary` gained GSI1 window listing (`HEALTH_SUMMARY#<window>` / `INSTANCE#<domain>`) maintained by `UpdateKeys` (the writer `SaveHealthSummary` goes through `ValidateAndCreateOrUpdate` → `UpdateKeys`, and `NewInstanceHealthSummary` calls it); `GetUnhealthyInstances` queries `Index("gsi1")` with `gsi1PK = HEALTH_SUMMARY#1h`. Legacy summaries without gsi1 are TTL-transient (30d) and not evaluated until next written |
| `pkg/storage/repositories/moderation_repository.go:653` (1) | moderation patterns: `SK = PATTERN` scan | **Fixed in wave part 2 (batch A, #1469):** `ModerationPattern` gained GSI3 (`MODERATION_PATTERNS#ALL` / `<updated_at>#<pattern_id>`) set in `UpdateKeys`; `GetModerationPatterns`' all-patterns branch queries `Index("gsi3")` (the old branch scanned `SK = "PATTERN"` while the model writes `SK = METADATA`, so it returned nothing; the keyed query makes it functional) |
| `pkg/storage/repositories/moderation_repository.go:1676,1712,1725` (3) | keyword / filter-entity lookups by `SK` (no FilterID known) | **Fixed in wave part 2 (batch A, #1469):** `UpdateFilterKeyword`/`DeleteFilterKeyword`/`DeleteFilterStatus` now take `filterID` (all REST/GraphQL callers have it from the route) and perform point reads/deletes (`PK = FILTER#<filterID>`, `SK = KEYWORD#<id>` / `STATUS#<id>`) |
| `pkg/storage/repositories/moderation_repository.go:2095` (1) | `GetFlag`: `SK LIKE %#id` scan | elimination pending — wave #1469 |
| `pkg/storage/repositories/moderation_repository.go:2945,2957,2972` (3) | reports/flags by `AssignedTo` (no assignee index) | **Fixed in wave part 2 (batch E, #1469):** `Report` gained GSI4 assignee index (`ASSIGNED#<assignee>` / `<status>#REPORT#<createdAtUnix>`) derived by `UpdateKeys` on every writer (`CreateReport`, `AssignReport`, `UnassignReport`, `UpdateReportStatus`); the cleared shape is persisted by explicit `UpdateBuilder` REMOVE in `UnassignReport` / `UpdateReportStatus` — tabletheory v3.0.6's implicit `Update()` skips empty omitempty attributes, so a plain update would leave the stale `ASSIGNED#<mod>` entry and overcount (batch E rework, 2026-08-27); `GetPendingModerationCount` counts open/in-progress assigned reports via keyed `gsi4SK begins_with` ranges. The flags branch queries the existing GSI2 (`FLAG_STATUS#pending`) with the assignee filter preserved — the `Flag` model has no `AssignedTo` attribute, so that branch always counted zero (preserved). Legacy assigned reports without gsi4 are not counted until next written (assign/status updates refresh keys) |
| `pkg/storage/repositories/notification_repository.go:760` (1) | notification delete-cascade by `ObjectID` | **Fixed in wave part 2 (batch E, #1469):** `Notification` gained GSI5 object listing (`NOTIF_OBJECT#<targetID>` / `<created_at>#<userID>#<id>`) maintained by `setupGSIKeys` on every create (`CreateNotification`/`CreateNotifications` → `BeforeCreate`); `DeleteNotificationsByObject` pages the partition keyed with a sort-key cursor. **Behavior correction:** the previous `ObjectID` filter matched nothing — the model references objects via `TargetID` — so the cascade now performs the intended delete of notifications about the object (disclosed in PR #1489) |
| `pkg/storage/repositories/object_repository.go:957` (1) | `CountWithdrawnQuotes`: `SK = QUOTED#…` scan | elimination pending — wave #1469 |
| `pkg/storage/repositories/scheduled_status_repository.go:147` (1) | scheduled status by `SK = ID#…` scan (no username known) | **Fixed in wave part 2 (batch A, #1469):** `ScheduledStatus` gained GSI2 (`SCHEDULED_ID#<id>` / `USER#<username>#SCHEDULED`) set in `UpdateKeys`; `GetScheduledStatus` queries `Index("gsi2")` with `Limit(1)`. Legacy rows without gsi2 are short-lived (deleted after publish) |
| `pkg/storage/repositories/threat_intel_repository.go:229` (1) | threat metadata: `SK = METADATA` scan | **Fixed in wave part 2 (batch E, #1469):** `LoadActiveThreats` queries the existing GSI2 global listing (`THREATS` / `<lastSeen>#<id>`) maintained by `ThreatIntel.UpdateKeys` on every create/update; the in-memory TTL/PK-prefix filters are preserved. Legacy threat rows without gsi2 keys (or written before `UpdateKeys` maintenance) are not loaded until next written (threats are TTL-transient) |

Notes: `All(...)` chained onto a pre-built query variable (`query.Limit(n).All(...)`), or a fresh chain whose `Where(...)` field/operator is not a string literal, is statically indeterminate and is deliberately not flagged; the gate targets inline key-less scan callsites. Of the 5 indeterminate sites verified during wave part 1, 2 are runtime GSI partition-key queries (`alert_repository.go:181`, `metrics_repository.go:591`) and 3 were production-unused helpers (`base_repository.go` `QueryHistoryWithDateRange`/`QueryMetricsByTimeRange`/`FindBySK`) deleted in the wave part 1 rework (2026-08-26).

### `goDynamoDBCountNoKey` (key-less `Count(...)` on fresh chains — counted full-table scan)

**Gate gap — Count() blindness (closed in wave part 1 rework, umbrella #1469):** TableTheory `Count()` shares `All()`'s compile path (`pkg/query/query_execution.go:80-111` in tabletheory v3.0.6 — `Compile()` decides Query vs Scan from the same key-condition rules, then `ExecuteScan` runs `Select=COUNT`), so a fresh-chain key-less `Count()` is a counted full-table scan the `goDynamoDBAllNoKey` gate could not see. This gate applies the exact same rules as its `All()` sibling: a fresh `Model(...)`/`WithContext(...)` `Count()` chain is counted unless a `Where(...)` constrains a partition key with equality; pre-built-query-variable and non-literal-`Where` chains stay indeterminate and are not flagged. Raw-condition shape `Where("PK = ? AND SK = ?", …)`: TableTheory v3.0.6's `Where(field, op, value)` is strictly 3-arg, so the field string is taken literally and the condition is a filter at most — never a key condition. The detector parses the shape determinately when the bound values are string literals (flagged, correctly — it compiles to a Scan) and treats it as indeterminate when the bound values are variables (conservatively not flagged). **Enumeration only in this PR** — the live site below is recorded, not modified; elimination lands under the wave.

| Callsite | What it does | Disposition |
|---|---|---|
| `pkg/storage/theorydb/patterns/soft_delete.go:323,330` (2) | `SoftDeleteRepository.GetSoftDeleteStats`: counts all items (`:323`, no conditions) and soft-deleted items (`:330`, `deleted_at` filter) via full-table counted scans | **elimination pending — wave #1469** (enumeration only; not modified in this PR) |

### `goDynamoDBQueryScan` (literal `.Scan(...)` callsites)

The full count-per-file baseline lives in `tools/audit_gates/baseline.yml` (~40 files). Disposition classes:

- **Deliberate offline tools (documented):** `cmd/lesser/migrate_*.go` and `pkg/storage/repositories/instance_counts.go` (the recount tool) — one-time operator maintenance, invoked manually offline. Never on a request path.
- **Fixed / scan-free redesigns:** the entries 1–34 above already carry their scan-free redesigns; most are implemented (TTL-only cleanups, GSI queries, exact-partition queries).
- **Fixed in wave part 2 (batch A, #1469, 2026-08-27):** 36 request-adjacent `.Scan` sites eliminated. Keyed partition/GSI queries were converted `.Scan` → `.All` (login-attempt reads, list memberships, account pins, status-by-URL via GSI7, thread nodes/missing replies, account preferences, popular media, media/jobs/spending/transcoding lists, vouch reads). Non-keyed sites were redesigned: import/export listings now query GSI1 (`USER#<username>`), `GetWalletByAddress` dropped its fallback scan (reverse index authoritative), `GetAllTrustRelationships` uses a new GSI3 global listing key on `TrustRelationship` (legacy rows without gsi3 are not listed by the admin view until the relationship is next written — the key is maintained on every create/update via `UpdateKeys`, and trust relationships are continuously created; no offline backfill tool is shipped with this wave, consistent with the other GSI additions here), and the 11 `media_repository.go` list queries got a default-limit clamp so `limit=0` no longer means unbounded (admin `UnmarkAllMediaAsSensitive` now paginates with cursors). **One request-path site remains baselined as a blocker:** `status_repository.go` `SearchStatuses` (Content `CONTAINS` full-text search, reachable via the GraphQL `LinkTimeline` parity resolver) — it cannot be made scan-free without a content-search index/backend, so it stays dispositioned `elimination pending` rather than being redesigned in this wave.
- **Fixed in wave part 2 (batch F, #1469, 2026-08-27):** all 22 baselined `pkg/storage/repositories/federation_repository.go` `.Scan` sites eliminated. 21 were keyed partition/GSI queries (PK or gsiNPK equality already present) converted `.Scan` → `.All`: `GetKnownInstances` / `GetFederationStatistics` / `GetFederationNodes` / `GetFederationNodesByHealth` (gsi1 `FEDERATION_ACTIVE`), `GetInstanceStats` / `GetInstanceHealthReport` / `GetFederationActivitiesByTimeRange` (PK `FEDERATION#<domain>#<month>` / gsi1 `FEDERATION_DAILY#<date>`), `GetFederationCosts` / `GetCostProjections` (PK `FEDERATION_COSTS#<month>`), `CalculateFederationClusters` / `GetFederationClusters` (PK `FEDERATION_CLUSTER#CLUSTERS`), `GetInstanceConnections` / `GetRecentInstanceConnections` (gsi2), `GetAffectedRelationships` (PK `follow#<user>`), `ListFailedDeliveries` (gsi1 `FAILED_DELIVERIES`), `GetInboxItems` / `GetPublicOutbox` (gsi1 `INBOX#<actor>` / `PUBLIC_OUTBOX#<actor>`), `GetOutboxItems` (PK `ACTOR#<actor>`), `GetDetailedFederationMetrics` (PK `FEDERATION_TIMESERIES#<domain>#<period>`), `GetDetailedMetricsByPeriod` (gsi2 `PERIOD#<period>`), `GetFederationCostsByUser` (PK `USER_FEDERATION_COSTS#<user>`; note: its `.Offset(offset)` at `federation_repository.go:2918` is a no-op — tabletheory v3.0.6 compiles `Offset` only on the scan path and no executor reads it, so this pagination parameter never paginated, before or after conversion). The one key-less site — `GetAllFederationEdges` pagination (`fetchEdgePageWithCursor`) — now queries a new **additive GSI3 listing key on `FederationEdge`** (`FED_EDGES#ALL` / `SRC#<source>#TGT#<target>`) maintained by `UpdateKeys` (single write entry `UpdateFederationEdge`, callers `pkg/federation/relationship_tracker.go`); the page cursor is the previous page's extra-item `gsi3SK` (internal, unexported). `FED_EDGES#ALL` concentrates every edge on one gsi3 partition — a deliberate wave decision consistent with batch A's `TrustRelationship` GSI3 global listing, acceptable at current federation-graph scale. Per-site scan-forbidding tests in `scanfree_wave1469_federation_test.go` (the fake overrides the DynamoDB client `Scan` method) prove every site answers without scanning. **Legacy-row / write-path gaps documented (behavior preserved, no backfill in this wave):** (a) the `GetInstanceStats` / `GetInstanceHealthReport` / `GetFederationActivitiesByTimeRange` surfaces are live readers of populated partitions — `RecordFederationActivity` → `ValidateAndCreate` → `validateAndCreate` → the promoted `BaseRepository.Create` (`pkg/storage/repositories/base_repository.go:109`) calls `item.UpdateKeys()`, and `FederationCostActivity.UpdateKeys()` (`pkg/storage/models/federation.go:85-108`) populates PK `FEDERATION#<domain>#<month>`, SK `ACTIVITY#<compactTime>#<id>`, and GSI1 `FEDERATION_DAILY#<date>` / `DOMAIN#<domain>#<id>`. These queries read the populated partitions; the conversion changed their access pattern from full-table Scan-with-filter to keyed Query, with result sets unchanged. (b) `GetFederationCostsByUser` queries PK `USER_FEDERATION_COSTS#<user>` which `FederationCost.UpdateKeys` never writes (it maintains `FEDERATION_COSTS#<month>` only) — the GraphQL listing stays empty; a per-user cost key is a data-model change, deferred. (c) `GetInstanceConnections(domain, "")` queries gsi2 `CONNECTION#<domain>` while the writer maintains gsi2 `INSTANCE#<domain>#CONNECTIONS#<type>` (and no production path constructs `InstanceConnection` rows at all) — the untyped branch stays empty; the typed branch is functional. (d) `GetRecentInstanceConnections` queries gsi2 `INSTANCE#<domain>#CONNECTIONS` (no `#type`) which the writer never emits — stays empty; zero production callers (dead surface). Legacy `FederationEdge` rows written before batch F lack the `FED_EDGES#ALL` gsi3 key and are not listed by the global view until the edge is next written (edges are continuously written by federation tracking).
- **Fixed in wave part 2 (batch E, #1469, 2026-08-27):** the event-path class — 3 literal `.Scan` files eliminated (11 keyed sites) and 10 key-less `All` sites rerouted. Keyed `.Scan` → `.All` conversions: `pkg/federation/relationship_tracker.go` (4: `processStateTransitions` / `archiveDormantRelationships` / `GetRelationshipsByState` — gsi1 `FEDERATION_STATE#<state>`, bounded per-tick batches, deliberate; `GetUserRelationships` — PK `USER#<userID>#FEDERATION`, no live production caller, converted for gate hygiene), `pkg/storage/repositories/media_analytics_repository.go` (5: `GetMediaAnalyticsByDate` gsi1 `DATE#<date>`, `GetMediaAnalyticsByVariant` gsi2 `VARIANT#<variant>`, `GetMediaAnalyticsByTimeRange` / `GetAllMediaAnalyticsByTimeRange` / `GetBandwidthByTimeRange` per-day gsi1 `DATE#<date>` queries), `pkg/storage/repositories/media_metadata_repository.go` (2: `GetMediaMetadataByStatus` / `CleanupExpiredMetadata` gsi1 `STATUS#<status>`). `analytics_repository.go` `GetPopularSearchQueries` was first delegated to the keyed GSI8 `PopularQueryCounter` path `GetTopQueries` and **reverted** to its baselined scan in the batch E rework (2026-08-27): the counter's GSI8 partition key re-points on every increment (`PopularQueryCounter.UpdateKeys` sets `GSI8PK = POPULAR#<bucket>#<date>` from the last write's Date), so only today's partition is populated and it cannot answer the sole caller's 7-day window (`scorePopularQueries`) — the raw-row aggregation is the only source with that semantics (window semantics pinned by `TestGetPopularSearchQueries_WindowSemantics`). Key-less `All` reroutes (additive GSI listing keys on pre-provisioned slots, maintained in `UpdateKeys`, exhaustive writer enumeration per site in the table above): `StatusEngagement` GSI1 `ENGAGEMENTS#ALL`, `LinkShare` GSI1 `LINK_SHARES#ALL` (trend-aggregator reads), `Report` GSI4 `ASSIGNED#<assignee>` (`GetPendingModerationCount`), `FederationActivity` GSI3 `FED_ACTIVITY#ALL` (`GetRecentActivities`), `FederationInstanceConfigTracking` GSI3 `INSTANCE_CONFIGS#ALL` (`ListInstanceConfigs`), `InstanceHealthSummary` GSI1 `HEALTH_SUMMARY#1h` (`GetUnhealthyInstances`), `Notification` GSI5 `NOTIF_OBJECT#<targetID>` (`DeleteNotificationsByObject`). `Hashtag` GSI1 `HASHTAGS#ALL` (for `GetRecentHashtags` / `getCandidateHashtags`) was also added and **reverted**: the only hashtag-metadata writer `HashtagRepository.IndexHashtag` has zero production callers, so the index would never populate — both reads stay on their baselined `SK = METADATA` scans with a "no live metadata writer — no rows exist" disposition (see the `analytics_repository.go:633` / `hashtag_trending_engine.go:351` rows). Two silent (unbaselined) key-less reads were also keyed: `pattern_repository.go` `GetPatterns` → existing `ModerationPattern` GSI3 `MODERATION_PATTERNS#ALL` (batch A shape) and `moderation_repository.go` `GetModerationDecisionsByModerator` → existing `ModerationReview` GSI1 `REVIEWER#<reviewerID>` (batch A shape). **Behavior corrections disclosed:** `DeleteNotificationsByObject` previously filtered on a nonexistent `ObjectID` attribute (Notification references objects via `TargetID`), so the cascade never deleted anything — it now performs the intended cascade keyed on `TargetID`; `StatusEngagement` writers now converge on the model's canonical SK timestamp format (previously compact-vs-unixnano, no reader depended on either).
- **Tracked for elimination:** the remaining production-path scans are scheduled for removal under #1469 (e.g. `pkg/moderation/advanced/reputation.go`, `pkg/storage/repositories/search_cost_repository.go`, `pkg/storage/repositories/search_repository.go`), with redesigns to be proposed in their own PRs referencing #1469. Batch E left the following baselined sites untouched as out-of-scope classes: request-path leftovers (batch A class — `moderation_repository.go` `GetAuditLogs`/`getAuditLogsByGSI` keyed `.Scan` admin reads, `analytics_repository.go` `GetUserSearchHistory` keyed `.Scan`) and LATENT/no-live-caller sites (`analytics_repository.go` `GetActorInteraction` — zero production callers; `moderation_repository.go` `GetFlag` — enumerated as a separate batch).

The umbrella issue #1469 is the single tracking point for the backlog elimination; do not attempt to fix the backlog inside unrelated PRs.

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
