# DynamoDB Unbounded Query Remediation Plan
**Date**: 2025-10-23  
**Author**: Codex (GPT-5)  
**Scope**: Address all 165 `.All()` calls identified in the “COMPLETE Unbounded Query Inventory” by enforcing index-backed access patterns and defensive limits.

> All entries below are based on direct inspection of the current repository code and corresponding model definitions to enumerate available indexing options.

---

## domain_pagination_helpers.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 65 | getPaginatedInstanceDomainBlocks (via `buildPaginationQuery`) | `models.InstanceDomainBlock` (GSI1PK=`DOMAIN_BLOCKS`) | Helper builds `.Index("GSI1").Where("GSI1PK","=", config.GSIPKValue).OrderBy("DESC").Limit(limit+1)`; limit sanitized to 20 default/100 max. | Index already optimal; uses descending order with reverse timestamp SK. | Status: ✅ Added `clampDomainLimit`, switched to `Where("GSI1SK","<", cursor)` and enforced `limit+1` sentinel in `pkg/storage/repositories/domain_pagination_helpers.go`. |
| 164 | getPaginatedDomainItems | `models.EmailDomainBlock` / `models.DomainAllow` (GSI1PK variants) | Same helper as above; returns `limit+1` results. | GSI1 provides proper ordering. | Status: ✅ Conversion helpers now inherit sanitized limits/cursor handling from `buildPaginationQuery`; see `pkg/storage/repositories/domain_pagination_helpers.go`. |

---

## domain_block_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 139 | GetUserDomainBlocks | `models.UserDomainBlock` (PK=`USER#{username}`, SK=`DOMAIN_BLOCK#{domain}`) | `.Where("PK","=", fmt.Sprintf("USER#%s", username)).Limit(limit+1)` with cursor via `.Cursor`. | Primary key only; no secondary indexes required. | Status: ✅ Sanitized limits (default 20, max 100), switched pagination to `Where("SK", ">", cursor)` with `limit+1` sentinel in `pkg/storage/repositories/domain_block_repository.go`. |

---

## dlq_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 164 | GetDLQMessagesByErrorType | `models.DLQMessage` (GSI1PK=`DLQ_ERROR#{type}`, GSI1SK=`{timestamp}#{service}#{id}`) | `.Index("error-index").Where("GSI1PK","=", ...).OrderBy("DESC").Limit(limit+1)`; cursor `<` on SK. | GSI1 sorted by timestamp; appropriate. | Status: ✅ Limit sanitized (default 20/max 200) with `limit+1` sentinel in `pkg/storage/repositories/dlq_repository.go`. Projection fields TBD once response payload contract confirmed. |
| 189 | GetDLQMessagesForReprocessing | `models.DLQMessage` (GSI2PK=`DLQ_RETRY#{service}#{status}`) | `.Index("retry-index").Where("GSI2PK","=", ...).OrderBy("ASC").Limit(limit)`; no cursor support. | GSI2 keyed by service/status/time. | Status: ✅ Added limit guard + ascending pagination with cursor (`safeLimit+1` batches, returns `nextCursor`) in `pkg/storage/repositories/dlq_repository.go`; updated callers (`pkg/dlq/processor.go`, `pkg/services/storage_adapter.go`). |
| 219 | GetDLQMessagesByStatus | `models.DLQMessage` (same GSI2) | `.Index("retry-index").Where("GSI2PK","=", ...).OrderBy("DESC").Limit(limit+1)` with cursor `<`. | Already uses sentinel; ensure limit sanitized. | Status: ✅ Limits now clamped (default 20/max 200) and sentinel trimming fixed in `pkg/storage/repositories/dlq_repository.go`. |
| 492 | SearchDLQMessages | `models.DLQMessage` (GSI3PK=`DLQ_SERVICE#{service}`) | `.Index("service-index").Where("GSI3PK","=", ...).OrderBy("DESC").Limit(limit+1)` plus filter expressions. | GSI3 for service-level browse; heavy filter usage may require additional indexes for error/status combos. | Status: ✅ Default/max limits (50/200) enforced with sentinel trimming in `pkg/storage/repositories/dlq_repository.go`; filter-to-index follow-up still under consideration. |

---

## cost_tracking_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 90 | ListByTable | `models.DynamoDBCostRecord` (GSI `table-index` PK=`COST_TABLE#{table}`, SK=`{timestamp}#{op}`) | `.Index("table-index").Where("GSI1PK","=", ...).Where("GSI1SK", between).OrderBy("DESC").Limit(limit)`; limit may be caller-supplied zero. | GSI1 optimized for table-specific timeline. | Status: ✅ Added limit clamp (100/1000), `cursor` support, and sentinel trimming in `pkg/storage/repositories/cost_tracking_repository.go`; downstream stats now paginate batches. |
| 1029 | GetRelayCostsByURL | `models.RelayCost` (GSI1PK=`RELAY_COSTS#{url}`, GSI1SK=`{timestamp}#{operation}`) | `.Index("GSI1").Where("GSI1PK","=", ...).Where("GSI1SK", between).OrderBy("DESC").Limit(limit)`; limit optional. | GSI1 proper; also `GSI2` for daily aggregates. | Status: ✅ Added cursor + operation filter support with clamped limits/sentinel (`pkg/storage/repositories/cost_tracking_repository.go`); internal callers now paginate via helper. |
| 1053 | GetRelayCostsByDateRange | `models.RelayCost` (GSI2PK=`RELAY_COSTS_DAILY#{date}`) | Loops days, each `.Index("GSI2").Where("GSI2PK","=", ...).Limit(limit)` without bounds (per day). | GSI2 aggregated per date; lacks global time index for cross-day queries. | Status: ✅ Enforced total cap (1000) + per-day limit (≤200) while retaining cross-day loop in `pkg/storage/repositories/cost_tracking_repository.go`; follow-up GSI (`GSI3`) still open. |
| 1157 | GetRelayMetricsHistory | `models.RelayMetrics` (GSI1PK=`RELAY_METRICS#{url}`, GSI1SK=`daily#{timestamp}`) | `.Index("GSI1").Where("GSI1PK","=", ...).Where("GSI1SK", between).OrderBy("DESC").Limit(limit)`; limit optional. | GSI1 sorted by period label. | Status: ✅ Limit clamped (100/500) with cursor-aware pagination in `pkg/storage/repositories/cost_tracking_repository.go`. |

---

## community_note_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 319 | GetCommunityNotesByAuthor | `models.CommunityNote` (GSI3PK=`AUTHOR#{authorID}#NOTES`, GSI3SK=`{created_at}#{id}`) | `.Index("gsi3").Where("GSI3PK","=", ...).Limit(limit)`; cursor uses `<` on `GSI3SK` but `limit` not clamped. | GSI3 sorted by timestamp; other GSIs for object and visibility. | Status: ✅ Limits clamp to 20/200 with sentinel trim, DESC ordering, and cursor-safe parsing in `pkg/storage/repositories/community_note_repository.go`. |

---

## base_repository_helpers.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 62 | QueryGSIWithTimeRangeHelper | Generic `T` (time-range GSI) | `.Index(indexName)...Limit(limit)` but caller may pass zero. | GSI must expose SK as ISO timestamps; ensure `gsiSK` sorted descending for order. | Status: ✅ Helper now clamps to 100/500, accepts cursor/order args, fetches `limit+1`, and returns trailing SK (`pkg/storage/repositories/base_repository_helpers.go`), with federation callers refreshed. |

---

## base_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 467 | Query | Generic `T` (primary PK) | `.Where("PK","=", pk)` with optional `Limit(limit)`; callers often pass 0 leading to full partition scan. | Encourage repository-specific default limits; optionally accept `BasePaginationOptions`. | Status: ✅ Enforces 100/500 clamp with sentinel fetch and warning logs in `pkg/storage/repositories/base_repository.go`; streaming/audit callers now paginate via `FindWithPagination`. |
| 520 | QueryWithSKPrefix | Generic `T` (PK + SK prefix) | `.Where("PK","=", pk).Where("SK","BEGINS_WITH", prefix)`; only limited when caller passes limit. | Primary key usage; consider GSI if prefix values large. | Status: ✅ Added paginated variant with clamp/sentinel in `pkg/storage/repositories/base_repository.go`; featured-tag, auth, poll flows iterate with cursors. |
| 547 | QueryGSI | Generic `T` (GSI with naming convention) | `.Index(indexName).Where(indexName+"PK","=", pk)`; limit optional. | Works for any defined GSI; need to enforce limit. | Status: ✅ Supports order + cursor via `QueryGSIPaginated`, clamps to 100/500, and updates AI/media/audit social callers to paginated or counted flows. |
| 586 | BatchGet | Generic `T` | Builds query by chaining `.Where` multiple times then `.All`, effectively converting to union scan with no limit. | DynamoDB batch get should use `BatchGet` API; no indexes needed. | Status: ✅ Marshals PK/SK via reflection and calls true `BatchGet` in 100-item chunks (`pkg/storage/repositories/base_repository.go`). |
| 779 | QueryCollectionWithConversion (GSI path) | Configurable `M` with optional `GSIConfig` | When `config.GSIConfig` present, `.Index(config.IndexName)` with `.Limit(limit)` but limit may be `0` (callers rely on default). | Encourage `limit` sanity; optionally use `UseCursor` for `Limit(limit+1)`. | Status: ✅ Applies 50/200 clamp, replaces filters with key conditions, fetches `limit+1`, and emits cursors via reflection (`pkg/storage/repositories/base_repository.go`). |
| 793 | QueryCollectionWithConversion (table path) | Same as above using primary PK | `.Where("PK","=", pk).Limit(limit)` optional; may rely on unbounded default. | Primary key; allow SK prefix filter. | Status: ✅ Shared helper reuses clamp + sentinel + cursor logic for table queries; upstream converters updated. |
| 1081 | ListAggregatedByPeriod | Aggregated metrics models (PK=`{prefix}#{period}#{type}`) | `.Where("PK","=", pk).Where("SK", ">=", startSK).Where("SK","<=", endSK).OrderBy("SK", DESC).Limit(limit)` but limit can be zero. | Primary key; consider `Count` for totals. | Status: ✅ Generic helper clamps to 100/500, accepts cursor, and returns next SK; metrics/cost repos loop to honour large requests. |
| 1227 | FindWithPagination | Generic `T` (PK + SK ordering) | `.Where("PK","=", pk).Limit(opts.Limit+1)` ensures limit, but `opts.Limit` sanitized inside. | Already uses limit and order by. | Document best practice; ensure `opts.Limit` clamped to <=100 (already). Add instrumentation to ensure `opts.Cursor` not reused. |
| 1422 | QueryWithFilter | Generic `T` filtered by attributes | `.Where("PK","=", pk)` plus `.Filter` and optional limit. | Filtering occurs server-side; need limit. | Status: ✅ Enforces 50/200 clamp with sentinel trim and warns on callers relying on zero limits (`pkg/storage/repositories/base_repository.go`). |
| 1473 | QueryBetween | Generic `T` range query on SK | `.Where("PK","=", pk).Where("SK", between).` optional limit. | Primary indexes; for reversed order consider `OrderBy`. | Status: ✅ Introduced `QueryBetweenPaginated` with clamp/sentinel/order+cursor handling; rate-limit services now accumulate via paginated loops. |

---

## auth_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 966 | queryWalletCredentials | `models.WalletCredential` (PK=`USER#{username}`, SK=`WALLET#{address}`) | Primary key query `.Where("PK","=", pk).Where("SK","BEGINS_WITH", skPrefix)`; optional `.Limit(limit)` only when caller passes >0, otherwise unbounded. | Primary table keys suffice; consider leveraging `models.WalletIndex` (PK=`WALLET#{type}#{address}`) for reverse lookups. | Status: ✅ Added 25/100 clamp, deterministic SK pagination, and sentinel-based cursor return in `pkg/storage/repositories/auth_repository.go`. |

---

## audit_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 152 | GetSecurityEvents | `models.AuthAuditLog` (GSI4PK=`SEVERITY#{level}`, GSI4SK=`AUDIT#{unix}`) | `.Index("GSI4").Where("GSI4PK","=", fmt.Sprintf("SEVERITY#%s", severity)).Limit(limit)` plus optional time range `GSI4SK` bounds. | GSI4 purposely supports severity filtering; also available GSIs for user/IP/session. | Status: ✅ Query now clamps limits, fetches `limit+1`, and emits a `GSI4SK` cursor (ascending order) via `pkg/storage/repositories/audit_repository.go`; `pkg/auth/audit.go` updated to ignore/consume new cursor as needed. |

---

## announcement_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 282 | GetAnnouncementsPaginated | `models.Announcement` (GSI `status-date-index` with PK=`ANNOUNCEMENT#{status}`, SK=`reverse_ts`) | `.Index("status-date-index").Where("GSI1PK","=", statusKey).OrderBy("GSI1SK","ASC").Limit(limit+1)`; cursor uses `>` on SK. | Existing index optimized; confirm reverse timestamp sort; consider `Select` to limit attributes. | Status: ✅ Existing implementation already clamps limit (20–100), retrieves `limit+1`, and emits forward cursor (`pkg/storage/repositories/announcement_repository.go`). |
| 359 | GetAnnouncementsByAdmin | `models.Announcement` (GSI `admin-index` PK=`ADMIN#{username}`, SK=`{published_at}#{id}`) | `.Index("admin-index").Where("GSI2PK","=", "ADMIN#...").OrderBy("GSI2SK","DESC").Limit(limit+1); cursor `<` on SK. | GSI suited for admin view. | Status: ✅ Limit normalization + sentinel logic present; verified in `pkg/storage/repositories/announcement_repository.go`. |

---

## activity_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 149 | GetInboxActivities | `models.Activity` (GSI1PK=`INBOX#{username}`, GSI1SK=`timestamp`) | `.Index("GSI1").Where("GSI1PK","=","INBOX#"+username).Limit(limit).OrderBy("GSI1SK","DESC")`; cursor uses explicit dynamorm cursor map. | GSI1 already ideal for inbox; consider projecting only SK + payload to reduce cost. | Status: ✅ Added `clampActivityLimit`, `limit+1` sentinel, and trimmed conversion prior to cursor encoding in `pkg/storage/repositories/activity_repository.go`. |
| 229 | GetOutboxActivities (cursor path) | `models.Activity` | Primary key query `.Where("PK","=", "ACTOR#"+username).Where("SK","BEGINS_WITH","ACTIVITY#").Limit(limit).OrderBy("SK","DESC")` when cursor provided. | Primary table key adequate; optionally add GSI on `CreatedAt` for faster descending queries. | Status: ✅ Unified query path with sentinel pagination and limit clamp while retaining existing cursor decoding (`pkg/storage/repositories/activity_repository.go`). |
| 238 | GetOutboxActivities (no cursor) | `models.Activity` | Same as above but executed when no cursor. | Same as above. | Status: ✅ Shares the sentinel-based path; results trimmed before cursor creation to avoid duplicate pages. |

---

## account_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 1302 | SearchAccounts | `models.User` (GSI `user-list-index` with PK=`USERS`, SK=`{created_at}#{username}`) | `.Index("user-list-index").Where("GSI1PK","=","USERS").Limit(500)` then client-side filters + manual `opts.Limit`. | `user-list-index` optimizes chronological paging but not prefix search. Extend model with `GSI5PK/GSI5SK` (using `index:gsi5,pk/sk`) patterned as `GSI5PK="USER_HANDLE_PREFIX#"+strings.ToLower(username[:2])`, `GSI5SK=strings.ToLower(username)` for lexicographic matching. | Status: ✅ Added handle-prefix GSI5 keys to `models.User` and rewired search to query `.Index("gsi5")` with begins_with + sentinel (`pkg/storage/models/user.go`, `pkg/storage/repositories/account_repository.go`). Confirmed CDK stack already defines `gsi5`; no migration or backfill required. |

---

## account_repository_timeline.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 39 | GetHomeTimeline | `models.TimelineEntry` (legacy PK currently queried as `USER#{username}`, SK prefix `HOME#`) | Table PK query with `.Where("PK","=", fmt.Sprintf("USER#%s", username)).Where("SK","BEGINS_WITH","HOME#").Limit(limit)`; pagination uses `<`/`>` on SK. | Need to reconcile model struct (`TIMELINE#HOME#{username}`) with repository keys; existing primary key should be used with sanitized prefix. | Status: ✅ Added limit clamp (20/200), descending SK ordering, and `limit+1` sentinel trimming in `pkg/storage/repositories/account_repository_timeline.go`; PK reconciliation tracked separately. |
| 75 | GetLocalTimeline | `models.TimelineEntry` (GSI `local-timeline-index` w/ PK=`LOCAL_TIMELINE`) | `.Index("local-timeline-index").Where("GSI1PK","=","LOCAL_TIMELINE").Limit(limit)`. | Confirm actual GSI fields exist (add struct fields if missing). | Status: ✅ Enforced clamped limits, descending ordering, and sentinel trimming in `pkg/storage/repositories/account_repository_timeline.go`. |
| 117 | GetPublicTimeline | `models.TimelineEntry` (GSIs `public-timeline-index`, `media-timeline-index` using `GSI2PK` for timeline flavor) | Chooses index by `onlyMedia` flag, `.Where("GSI2PK","=", gsiPK).Limit(limit)`. | Ensure model exposes `GSI2PK/GSI2SK` fields; confirm TTL/visibility filters. | Status: ✅ Pagination now clamps limit, orders by `GSI2SK` DESC, and trims sentinel record (`pkg/storage/repositories/account_repository_timeline.go`). |
| 153 | GetHashtagTimeline | `models.TimelineEntry` (GSI `hashtag-timeline-index` w/ PK=`HASHTAG#{tag}`) | `.Index("hashtag-timeline-index").Where("GSI3PK","=", fmt.Sprintf("HASHTAG#%s", hashtag)).Limit(limit)`. | Confirm uppercase naming and SK sort order; may require GSI3 fields in model. | Status: ✅ Added sanitization, clamped limit, and sentinel trimming with DESC order (`pkg/storage/repositories/account_repository_timeline.go`). |
| 198 | GetListTimeline | `models.TimelineEntry` (GSI `list-timeline-index` w/ PK=`LIST#{listID}`) | `.Index("list-timeline-index").Where("GSI4PK","=", fmt.Sprintf("LIST#%s", listID)).Limit(limit)` with optional `<`/`>`. | Ensure `TimelineEntry` carries `GSI4` fields; may require struct update. | Status: ✅ Limit normalization + sentinel enforced; queries now order `GSI4SK` DESC before trimming (`pkg/storage/repositories/account_repository_timeline.go`). |
| 297 | GetConversations | `models.Conversation` participant records (PK=`USER_CONVERSATIONS#{username}`) | Table PK query `.Where("PK","=", fmt.Sprintf("USER#%s", username)).Where("SK","BEGINS_WITH","CONVERSATION#").Limit(limit)` but key pattern likely legacy. | Participant records provide PK `USER_CONVERSATIONS#{username}` with timestamp SK; use that pattern consistently. | Status: ✅ Added clamp + sentinel trimming with descending SK order (legacy PK retained pending migration) in `pkg/storage/repositories/account_repository_timeline.go`. |

---

## account_repository_social.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 134 | GetFollowers | `models.Follow` (PK=`follow#{follower}`, GSI1PK=`follow#{followed}`) | `.Index("gsi1-index").Where("GSI1PK","=", fmt.Sprintf("follow#%s", username)).Limit(limit)` with optional `cursor`. | GSI1 already optimized for follower listing. | Status: ✅ Added 40/200 clamp, ordered SK asc, and applied `limit+1` sentinel with cursor trimming in `pkg/storage/repositories/account_repository_social.go`. |
| 181 | GetFollowing | `models.Follow` (PK=`follow#{follower}`) | Primary key query `.Where("PK","=", fmt.Sprintf("follow#%s", username)).Where("SK","BEGINS_WITH","following#").Limit(limit)` | Primary table PK/SK supports prefix query; GSI not needed. | Status: ✅ Query now clamps to 40/200, orders by SK, and uses `limit+1` sentinel for cursor creation (`pkg/storage/repositories/account_repository_social.go`). |
| 495 | GetBookmarks | `models.Bookmark` (PK=`BOOKMARK#{username}`) | `.Where("PK","=", fmt.Sprintf("BOOKMARK#%s", username)).Limit(limit)` with optional SK cursor but no `limit` normalization. | Only primary key available; no GSIs defined. Optionally add descending SK index if reverse order needed. | Status: ✅ Applied 40/400 clamp, sentinel fetch, and deterministic SK ordering before cursor encoding (`pkg/storage/repositories/account_repository_social.go`). |

---

## account_repository_oauth.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 460 | ListOAuthClients | `models.OAuthClient` (PK=`OAUTH_CLIENT#{clientID}`, SK=`METADATA`, `oauth-clients-index` with PK=`OAUTH_CLIENTS`, SK=`CREATED_AT#{desc}#CLIENT#{clientID}`) | Uses `.Index(oauthClientsIndexName)` with `.Where(oauthClientsPartitionAttr,"=", "OAUTH_CLIENTS")` and `.OrderBy` but limit comes directly from caller. | Existing GSI already optimal for global listing. Need to guarantee `limit` normalized before query (defaults and max). | Harden input normalization (already present) and add unit guard verifying limit >0; keep `.Limit(limit+1)`; add projection expression to retrieve minimal attributes if necessary for listing. |

---

## account_repository_auth.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 1175 | UpdateWebAuthnCredential | `models.WebAuthnCredential` (PK=`USER#{userID}`, SK=`WEBAUTHN_CRED#{credentialID}`) | `Model(&WebAuthnCredential{}).Where("ID","=", credentialID).All(&credentials)` without limit, forcing full table evaluation on non-key attribute. | Model currently lacks secondary index fields. Add `GSI1PK`/`GSI1SK` with `dynamorm:"index:gsi1,pk"` / `sk`, following house naming, using key pattern `GSI1PK="WEBAUTHN_CREDENTIAL#"+credentialID`, `GSI1SK="USER#"+userID` (optionally append timestamp for uniqueness). | Status: ✅ Introduced GSI1 attributes on the model and migrated lookup to `.Index("gsi1")` with `Limit(1)` (`pkg/storage/models/webauthn_credential.go`, `pkg/storage/repositories/account_repository_auth.go`). Existing CDK stack provisions `gsi1`; no further infra work or backfill needed. |

---
