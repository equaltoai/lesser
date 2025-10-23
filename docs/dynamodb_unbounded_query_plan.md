# DynamoDB Unbounded Query Remediation Plan
**Date**: 2025-10-23  
**Author**: Codex (GPT-5)  
**Scope**: Address all 165 `.All()` calls identified in the “COMPLETE Unbounded Query Inventory” by enforcing index-backed access patterns and defensive limits.

> All entries below are based on direct inspection of the current repository code and corresponding model definitions to enumerate available indexing options.

---

## domain_pagination_helpers.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 65 | getPaginatedInstanceDomainBlocks (via `buildPaginationQuery`) | `models.InstanceDomainBlock` (GSI1PK=`DOMAIN_BLOCKS`) | Helper builds `.Index("GSI1").Where("GSI1PK","=", config.GSIPKValue).OrderBy("DESC").Limit(limit+1)`; limit sanitized to 20 default/100 max. | Index already optimal; uses descending order with reverse timestamp SK. | Maintain limit normalization; ensure cursor semantics use `Where("GSI1SK","<", cursor)` rather than `.Cursor` for deterministic pagination. |
| 164 | getPaginatedDomainItems | `models.EmailDomainBlock` / `models.DomainAllow` (GSI1PK variants) | Same helper as above; returns `limit+1` results. | GSI1 provides proper ordering. | Document limit handling; ensure convertors propagate `GSI1SK`; consider exposing max limit constant to calling repositories. |

---

## domain_block_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 139 | GetUserDomainBlocks | `models.UserDomainBlock` (PK=`USER#{username}`, SK=`DOMAIN_BLOCK#{domain}`) | `.Where("PK","=", fmt.Sprintf("USER#%s", username)).Limit(limit+1)` with cursor via `.Cursor`. | Primary key only; no secondary indexes required. | Tighten cursor handling (use `Where("SK",">",cursor)` instead of `Cursor` for deterministic pagination), enforce limit bounds, provide sentinel trimming, and add optional startswith filter if needed. |

---

## dlq_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 164 | GetDLQMessagesByErrorType | `models.DLQMessage` (GSI1PK=`DLQ_ERROR#{type}`, GSI1SK=`{timestamp}#{service}#{id}`) | `.Index("error-index").Where("GSI1PK","=", ...).OrderBy("DESC").Limit(limit+1)`; cursor `<` on SK. | GSI1 sorted by timestamp; appropriate. | Ensure limit normalized (default 20, max 200); maintain sentinel; provide projection to reduce payload. |
| 189 | GetDLQMessagesForReprocessing | `models.DLQMessage` (GSI2PK=`DLQ_RETRY#{service}#{status}`) | `.Index("retry-index").Where("GSI2PK","=", ...).OrderBy("ASC").Limit(limit)`; no cursor support. | GSI2 keyed by service/status/time. | Add limit guard and sentinel; implement pagination via cursor (last GSI2SK) while preserving ascending order for reprocessing. |
| 219 | GetDLQMessagesByStatus | `models.DLQMessage` (same GSI2) | `.Index("retry-index").Where("GSI2PK","=", ...).OrderBy("DESC").Limit(limit+1)` with cursor `<`. | Already uses sentinel; ensure limit sanitized. | Validate limit boundaries and ensure `cursor` format matches `timestamp#id`. |
| 492 | SearchDLQMessages | `models.DLQMessage` (GSI3PK=`DLQ_SERVICE#{service}`) | `.Index("service-index").Where("GSI3PK","=", ...).OrderBy("DESC").Limit(limit+1)` plus filter expressions. | GSI3 for service-level browse; heavy filter usage may require additional indexes for error/status combos. | Enforce limit (default 50, max 200), convert some filters to key conditions by introducing additional GSIs (e.g., `status-index`) if high-cardinality filters degrade performance; maintain sentinel for pagination. |

---

## cost_tracking_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 90 | ListByTable | `models.DynamoDBCostRecord` (GSI `table-index` PK=`COST_TABLE#{table}`, SK=`{timestamp}#{op}`) | `.Index("table-index").Where("GSI1PK","=", ...).Where("GSI1SK", between).OrderBy("DESC").Limit(limit)`; limit may be caller-supplied zero. | GSI1 optimized for table-specific timeline. | Clamp limit (default 100, max 1000); fetch `limit+1` for pagination; return cursor (last GSI1SK). |
| 1029 | GetRelayCostsByURL | `models.RelayCost` (GSI1PK=`RELAY_COSTS#{url}`, GSI1SK=`{timestamp}#{operation}`) | `.Index("GSI1").Where("GSI1PK","=", ...).Where("GSI1SK", between).OrderBy("DESC").Limit(limit)`; limit optional. | GSI1 proper; also `GSI2` for daily aggregates. | Guard limit default (100) and sentinel; provide pagination cursor; add ability to filter by operation. |
| 1053 | GetRelayCostsByDateRange | `models.RelayCost` (GSI2PK=`RELAY_COSTS_DAILY#{date}`) | Loops days, each `.Index("GSI2").Where("GSI2PK","=", ...).Limit(limit)` without bounds (per day). | GSI2 aggregated per date; lacks global time index for cross-day queries. | Introduce total result cap (1000) with per-day limit `min(limit, 200)` and stop once overall cap reached; design supplemental GSI (`index:gsi3`) keyed by `GSI3PK="RELAY_COSTS_RANGE"` / `GSI3SK="{timestamp}#{relayURL}"` so cross-day queries become single indexed call. |
| 1157 | GetRelayMetricsHistory | `models.RelayMetrics` (GSI1PK=`RELAY_METRICS#{url}`, GSI1SK=`daily#{timestamp}`) | `.Index("GSI1").Where("GSI1PK","=", ...).Where("GSI1SK", between).OrderBy("DESC").Limit(limit)`; limit optional. | GSI1 sorted by period label. | Enforce limit default 100, max 500; fetch sentinel; convert to pagination result with cursor. |

---

## community_note_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 319 | GetCommunityNotesByAuthor | `models.CommunityNote` (GSI3PK=`AUTHOR#{authorID}#NOTES`, GSI3SK=`{created_at}#{id}`) | `.Index("gsi3").Where("GSI3PK","=", ...).Limit(limit)`; cursor uses `<` on `GSI3SK` but `limit` not clamped. | GSI3 sorted by timestamp; other GSIs for object and visibility. | Enforce limit defaults (20 ≤ limit ≤ 200), fetch `limit+1` for pagination, ensure cursor parsing uses same timestamp format to avoid skipping. |

---

## base_repository_helpers.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 62 | QueryGSIWithTimeRangeHelper | Generic `T` (time-range GSI) | `.Index(indexName)...Limit(limit)` but caller may pass zero. | GSI must expose SK as ISO timestamps; ensure `gsiSK` sorted descending for order. | Require positive limit (default 100, max 500); add sentinel (`limit+1`) and return cursor to caller; allow configurable order direction. |

---

## base_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 467 | Query | Generic `T` (primary PK) | `.Where("PK","=", pk)` with optional `Limit(limit)`; callers often pass 0 leading to full partition scan. | Encourage repository-specific default limits; optionally accept `BasePaginationOptions`. | Require explicit limit param; if caller needs all items, force chunked iteration; add instrumentation to detect unlimited usage. |
| 520 | QueryWithSKPrefix | Generic `T` (PK + SK prefix) | `.Where("PK","=", pk).Where("SK","BEGINS_WITH", prefix)`; only limited when caller passes limit. | Primary key usage; consider GSI if prefix values large. | Add limit normalization + sentinel; provide pagination token support; encourage usage via new helper returning `NextCursor`. |
| 547 | QueryGSI | Generic `T` (GSI with naming convention) | `.Index(indexName).Where(indexName+"PK","=", pk)`; limit optional. | Works for any defined GSI; need to enforce limit. | Add required `limit` argument (default & max) to avoid unlimited queries; expose `OrderBy` parameter for consistent pagination. |
| 586 | BatchGet | Generic `T` | Builds query by chaining `.Where` multiple times then `.All`, effectively converting to union scan with no limit. | DynamoDB batch get should use `BatchGet` API; no indexes needed. | Replace with DynamoDB `BatchGet` via `core.BatchGet` to fetch keys in chunks; remove `.All` misuse; ensure 100 item chunk limit. |
| 779 | QueryCollectionWithConversion (GSI path) | Configurable `M` with optional `GSIConfig` | When `config.GSIConfig` present, `.Index(config.IndexName)` with `.Limit(limit)` but limit may be `0` (callers rely on default). | Encourage `limit` sanity; optionally use `UseCursor` for `Limit(limit+1)`. | Force default limit (e.g., 50) if unspecified; add sentinel for pagination; ensure SK range filters use `Where` not `Filter`. |
| 793 | QueryCollectionWithConversion (table path) | Same as above using primary PK | `.Where("PK","=", pk).Limit(limit)` optional; may rely on unbounded default. | Primary key; allow SK prefix filter. | Same as above: enforce limit; add sentinel; provide ability to order descending by SK. |
| 1081 | ListAggregatedByPeriod | Aggregated metrics models (PK=`{prefix}#{period}#{type}`) | `.Where("PK","=", pk).Where("SK", ">=", startSK).Where("SK","<=", endSK).OrderBy("SK", DESC).Limit(limit)` but limit can be zero. | Primary key; consider `Count` for totals. | Require limit >0 (default 100), add sentinel for more pages, return cursor based on last SK. |
| 1227 | FindWithPagination | Generic `T` (PK + SK ordering) | `.Where("PK","=", pk).Limit(opts.Limit+1)` ensures limit, but `opts.Limit` sanitized inside. | Already uses limit and order by. | Document best practice; ensure `opts.Limit` clamped to <=100 (already). Add instrumentation to ensure `opts.Cursor` not reused. |
| 1422 | QueryWithFilter | Generic `T` filtered by attributes | `.Where("PK","=", pk)` plus `.Filter` and optional limit. | Filtering occurs server-side; need limit. | Add default limit (50) when absent; convert `Filter` to key condition when possible by refactoring to `Where`. |
| 1473 | QueryBetween | Generic `T` range query on SK | `.Where("PK","=", pk).Where("SK", between).` optional limit. | Primary indexes; for reversed order consider `OrderBy`. | Force limit default (100) and sentinel; allow specifying order direction for timeline-style queries. |

---

## auth_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 966 | queryWalletCredentials | `models.WalletCredential` (PK=`USER#{username}`, SK=`WALLET#{address}`) | Primary key query `.Where("PK","=", pk).Where("SK","BEGINS_WITH", skPrefix)`; optional `.Limit(limit)` only when caller passes >0, otherwise unbounded. | Primary table keys suffice; consider leveraging `models.WalletIndex` (PK=`WALLET#{type}#{address}`) for reverse lookups. | Enforce default limit (e.g., 25) and max (100), add sentinel retrieval. For reverse wallet lookup, supplement with query on `WalletIndex` using dedicated GSI if future expansion needed. |

---

## audit_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 152 | GetSecurityEvents | `models.AuthAuditLog` (GSI4PK=`SEVERITY#{level}`, GSI4SK=`AUDIT#{unix}`) | `.Index("GSI4").Where("GSI4PK","=", fmt.Sprintf("SEVERITY#%s", severity)).Limit(limit)` plus optional time range `GSI4SK` bounds. | GSI4 purposely supports severity filtering; also available GSIs for user/IP/session. | Retain GSI4 query; clamp limit to [1, 1000] (already enforced). Add `.Limit(limit+1)` to detect more pages and include next cursor (timestamp) for forward scanning. |

---

## announcement_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 282 | GetAnnouncementsPaginated | `models.Announcement` (GSI `status-date-index` with PK=`ANNOUNCEMENT#{status}`, SK=`reverse_ts`) | `.Index("status-date-index").Where("GSI1PK","=", statusKey).OrderBy("GSI1SK","ASC").Limit(limit+1)`; cursor uses `>` on SK. | Existing index optimized; confirm reverse timestamp sort; consider `Select` to limit attributes. | Retain index, ensure `limit` sanitized (already 20–100). Add unit tests for cursor; ensure `OrderBy` direction matches reverse timestamp logic. |
| 359 | GetAnnouncementsByAdmin | `models.Announcement` (GSI `admin-index` PK=`ADMIN#{username}`, SK=`{published_at}#{id}`) | `.Index("admin-index").Where("GSI2PK","=", "ADMIN#...").OrderBy("GSI2SK","DESC").Limit(limit+1); cursor `<` on SK. | GSI suited for admin view. | Keep query; ensure limit guard (20–100) and add coverage verifying pagination. |

---

## activity_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 149 | GetInboxActivities | `models.Activity` (GSI1PK=`INBOX#{username}`, GSI1SK=`timestamp`) | `.Index("GSI1").Where("GSI1PK","=","INBOX#"+username).Limit(limit).OrderBy("GSI1SK","DESC")`; cursor uses explicit dynamorm cursor map. | GSI1 already ideal for inbox; consider projecting only SK + payload to reduce cost. | Keep GSI1 but convert to `.Limit(limit+1)` for pagination sentinel; validate cursor decode while preserving existing `since` semantics; optionally add `Select(Fields)` to trim payload. |
| 229 | GetOutboxActivities (cursor path) | `models.Activity` | Primary key query `.Where("PK","=", "ACTOR#"+username).Where("SK","BEGINS_WITH","ACTIVITY#").Limit(limit).OrderBy("SK","DESC")` when cursor provided. | Primary table key adequate; optionally add GSI on `CreatedAt` for faster descending queries. | Harmonize path to use helper returning `Limit(limit+1)` and use `Cursor` only after `OrderBy`; ensure limit normalization (already 20 default). |
| 238 | GetOutboxActivities (no cursor) | `models.Activity` | Same as above but executed when no cursor. | Same as above. | Factor into shared function to avoid duplication; ensure we apply sentinel limit and convert to consistent pagination tokens. |

---

## account_repository.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 1302 | SearchAccounts | `models.User` (GSI `user-list-index` with PK=`USERS`, SK=`{created_at}#{username}`) | `.Index("user-list-index").Where("GSI1PK","=","USERS").Limit(500)` then client-side filters + manual `opts.Limit`. | `user-list-index` optimizes chronological paging but not prefix search. Extend model with `GSI5PK/GSI5SK` (using `index:gsi5,pk/sk`) patterned as `GSI5PK="USER_HANDLE_PREFIX#"+strings.ToLower(username[:2])`, `GSI5SK=strings.ToLower(username)` for lexicographic matching. | Backfill `gsi5` fields, update repository to query `.Index("gsi5").Where("GSI5PK","=", prefixKey).Where("GSI5SK","BEGINS_WITH", normalizedQuery).Limit(opts.Limit+1)`, and delete the obsolete client-side filtering path so all lookups stay index-backed. |

---

## account_repository_timeline.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 39 | GetHomeTimeline | `models.TimelineEntry` (legacy PK currently queried as `USER#{username}`, SK prefix `HOME#`) | Table PK query with `.Where("PK","=", fmt.Sprintf("USER#%s", username)).Where("SK","BEGINS_WITH","HOME#").Limit(limit)`; pagination uses `<`/`>` on SK. | Need to reconcile model struct (`TIMELINE#HOME#{username}`) with repository keys; existing primary key should be used with sanitized prefix. | Normalize key pattern (ensure `TimelineEntry.UpdateKeys` matches), enforce `limit` guard and `+1` sentinel; consider moving to GSI for descending order if required. |
| 75 | GetLocalTimeline | `models.TimelineEntry` (GSI `local-timeline-index` w/ PK=`LOCAL_TIMELINE`) | `.Index("local-timeline-index").Where("GSI1PK","=","LOCAL_TIMELINE").Limit(limit)`. | Confirm actual GSI fields exist (add struct fields if missing). | Add explicit limit normalization and sentinel; ensure query orders by SK descending; add sparse projection to limit read costs. |
| 117 | GetPublicTimeline | `models.TimelineEntry` (GSIs `public-timeline-index`, `media-timeline-index` using `GSI2PK` for timeline flavor) | Chooses index by `onlyMedia` flag, `.Where("GSI2PK","=", gsiPK).Limit(limit)`. | Ensure model exposes `GSI2PK/GSI2SK` fields; confirm TTL/visibility filters. | Validate index mapping, clamp limit, and add begins_with filter for effective ordering; evaluate projection. |
| 153 | GetHashtagTimeline | `models.TimelineEntry` (GSI `hashtag-timeline-index` w/ PK=`HASHTAG#{tag}`) | `.Index("hashtag-timeline-index").Where("GSI3PK","=", fmt.Sprintf("HASHTAG#%s", hashtag)).Limit(limit)`. | Confirm uppercase naming and SK sort order; may require GSI3 fields in model. | Guard limit and sentinel, add begins_with or `<` pagination for SK; ensure request enforces sanitized hashtag. |
| 198 | GetListTimeline | `models.TimelineEntry` (GSI `list-timeline-index` w/ PK=`LIST#{listID}`) | `.Index("list-timeline-index").Where("GSI4PK","=", fmt.Sprintf("LIST#%s", listID)).Limit(limit)` with optional `<`/`>`. | Ensure `TimelineEntry` carries `GSI4` fields; may require struct update. | Normalize limit, sentinel, and projection; confirm list owner validation. |
| 297 | GetConversations | `models.Conversation` participant records (PK=`USER_CONVERSATIONS#{username}`) | Table PK query `.Where("PK","=", fmt.Sprintf("USER#%s", username)).Where("SK","BEGINS_WITH","CONVERSATION#").Limit(limit)` but key pattern likely legacy. | Participant records provide PK `USER_CONVERSATIONS#{username}` with timestamp SK; use that pattern consistently. | Update retrieval to `PK="USER_CONVERSATIONS#{username}"`, enforce limit + sentinel, and optionally add filter on unread state using GSI if available. |

---

## account_repository_social.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 134 | GetFollowers | `models.Follow` (PK=`follow#{follower}`, GSI1PK=`follow#{followed}`) | `.Index("gsi1-index").Where("GSI1PK","=", fmt.Sprintf("follow#%s", username)).Limit(limit)` with optional `cursor`. | GSI1 already optimized for follower listing. | Keep GSI1 query; enforce limit normalization (default 40, max 200) via helper to avoid `limit=0` bypass; add `.Limit(safeLimit+1)` for pagination sentinel; project only necessary attributes. |
| 181 | GetFollowing | `models.Follow` (PK=`follow#{follower}`) | Primary key query `.Where("PK","=", fmt.Sprintf("follow#%s", username)).Where("SK","BEGINS_WITH","following#").Limit(limit)` | Primary table PK/SK supports prefix query; GSI not needed. | Clamp `limit`, add `+1` sentinel to compute cursor; ensure `BEGINS_WITH` used with sanitized prefix constant; project minimal attributes. |
| 495 | GetBookmarks | `models.Bookmark` (PK=`BOOKMARK#{username}`) | `.Where("PK","=", fmt.Sprintf("BOOKMARK#%s", username)).Limit(limit)` with optional SK cursor but no `limit` normalization. | Only primary key available; no GSIs defined. Optionally add descending SK index if reverse order needed. | Add guard for limit (default 40, max 400) and `Limit(limit+1)` for pagination; consider migrating to descending timestamp or add new GSI if reverse order required. |

---

## account_repository_oauth.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 460 | ListOAuthClients | `models.OAuthClient` (PK=`OAUTH_CLIENT#{clientID}`, SK=`METADATA`, `oauth-clients-index` with PK=`OAUTH_CLIENTS`, SK=`CREATED_AT#{desc}#CLIENT#{clientID}`) | Uses `.Index(oauthClientsIndexName)` with `.Where(oauthClientsPartitionAttr,"=", "OAUTH_CLIENTS")` and `.OrderBy` but limit comes directly from caller. | Existing GSI already optimal for global listing. Need to guarantee `limit` normalized before query (defaults and max). | Harden input normalization (already present) and add unit guard verifying limit >0; keep `.Limit(limit+1)`; add projection expression to retrieve minimal attributes if necessary for listing. |

---

## account_repository_auth.go

| Line | Method | Model(s) | Current Access Pattern | Indexing Options Identified | Planned Remediation |
| ---- | ------ | -------- | ---------------------- | --------------------------- | ------------------- |
| 1175 | UpdateWebAuthnCredential | `models.WebAuthnCredential` (PK=`USER#{userID}`, SK=`WEBAUTHN_CRED#{credentialID}`) | `Model(&WebAuthnCredential{}).Where("ID","=", credentialID).All(&credentials)` without limit, forcing full table evaluation on non-key attribute. | Model currently lacks secondary index fields. Add `GSI1PK`/`GSI1SK` with `dynamorm:"index:gsi1,pk"` / `sk`, following house naming, using key pattern `GSI1PK="WEBAUTHN_CREDENTIAL#"+credentialID`, `GSI1SK="USER#"+userID` (optionally append timestamp for uniqueness). | Backfill new `gsi1` fields and switch repository call to `.Index("gsi1").Where("GSI1PK","=", "WEBAUTHN_CREDENTIAL#"+credentialID).Limit(1)` so the scan path is fully removed. |

---
