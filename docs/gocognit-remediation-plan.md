# Gocognit Remediation Plan

This plan outlines how we will resolve the ten cognitive complexity violations reported by `make lint` (gocognit threshold 30). The findings cluster around ActivityPub actor hydration/conversion, repository update helpers, and timeline aggregation. The workstreams below are ordered by impact and the risk of latent bugs.

## Issue Summary

| Function | File | Complexity | Notes |
| -------- | ---- | ---------- | ----- |
| `(*AccountRepository).applyUserUpdates` | `pkg/storage/repositories/account_repository.go` | 104 | Giant switch with type assertions; hard to audit field coverage. |
| `(*Service).buildAccountFromActor` | `pkg/services/relationships/service.go` | 56 | Blends identifier normalization, profile reconciliation, and persistence lookups. |
| `(*Resolver).convertAccountToActor` | `graph/schema.resolvers.go` | 54 | Reimplements ActivityPub actor hydration already handled elsewhere. |
| `(*Service).sanitizeActivityActor` | `pkg/services/relationships/service.go` | 54 | Combines fallback actor creation with deep copy logic. |
| `(*Service).hydrateAccountActor` | `pkg/services/accounts/service.go` | 42 | Duplicates many of the same actor field defaults. |
| `(*Service).CreateNote` | `pkg/services/notes/service.go` | 35 | Responsible for validation, persistence, media, analytics, and events in one method. |
| `(*TrackingRepository).GetAggregatedCostsByPeriod` | `pkg/storage/repositories/cost_tracking_repository.go` | 36 | Nested pagination fetch + merge responsibilities. |
| `(*StatusRepository).GetHomeTimeline` | `pkg/storage/repositories/status_repository.go` | 34 | Multiple follow lookup strategies + aggregation + pagination. |
| `(*AccountRepository).ensureActorIdentifiers` | `pkg/storage/repositories/account_repository.go` | 34 | Repeats URL construction logic scattered across services. |
| `(*queryResolver).HashtagTimeline` | `graph/query_resolvers_hashtags.go` | 45 | Builds timeline, hydrates statuses, filters media, and formats GraphQL edges. |

## Workstream 1 – Standardize ActivityPub Actor Hydration

- **Goal:** De-duplicate and centralize actor normalization shared by `convertAccountToActor`, `hydrateAccountActor`, `sanitizeActivityActor`, `buildAccountFromActor`, and `ensureActorIdentifiers`.
- **Approach:**
  1. Extract a new package (e.g. `pkg/activitypubutil` or `pkg/services/actors`) with focused helpers:
     - `BuildLocalActor(username string, baseURL string, user *storage.User, existing *activitypub.Actor) *activitypub.Actor`
     - `MergeActorMetadata(dst, src *activitypub.Actor)`
     - `DerivePreferredUsername(actor *activitypub.Actor, fallback string) string`
  2. Gradually rewrite the five high-complexity functions to delegate to the shared helpers. Keep each high-level function responsible for orchestration (e.g., repository lookups or GraphQL conversion) while helpers encapsulate conditional field population.
  3. Add unit tests for the new helper package covering common cases (local accounts, remote actors, missing URLs) to guard against regressions when removing inline logic.
- **Expected Result:** Each target function becomes a thin wrapper (<30 complexity) that composes shared helpers; ActivityPub defaults stay consistent across the codebase.

## Workstream 2 – Account Repository Update Simplification

- **Scope:** `(*AccountRepository).applyUserUpdates`
- **Approach:**
  1. Introduce a typed struct (e.g., `UserUpdatePayload`) with JSON tags and validation to replace the raw `map[string]interface{}` updates map.
  2. Add a translation layer that marshals the map into the struct using `mapstructure` or manual decoding with centralized type assertion helpers.
  3. Break the big switch into smaller focused setters grouped by concern (profile fields, status flags, security fields, metadata).
  4. Add targeted unit tests for update application covering string/bool conversions, `fields`, and `recovery_methods` array handling.
- **Expected Result:** Complexity drops dramatically; field coverage is easier to audit; tests prevent regressions in partial updates.

## Workstream 3 – Timeline Builders

- **Targets:** `HashtagTimeline` resolver and `StatusRepository.GetHomeTimeline`.
- **Approach:**
  1. Extract reusable helpers for cursor handling, limit normalization, and edge construction (`buildPostEdges`, `resolveStatusesForHashtag`, `fetchFollowingActorIDs`).
  2. Move multi-branch follow list resolution in `GetHomeTimeline` into dedicated strategy functions (`fetchFollowingFromRelationshipRepo`, `fetchFollowingFromLegacyRepo`) and reuse them in other call sites if applicable.
  3. For `HashtagTimeline`, split media filtering and GraphQL edge composition into helpers so the main resolver reads like: fetch posts → hydrate statuses → filter → build connection.
  4. Add integration-level tests (or extend existing GraphQL tests) to cover media-only filtering and pagination after refactor.
- **Expected Result:** Both functions fall below the gocognit threshold, and timeline-building rules are encapsulated for future reuse.

## Workstream 4 – Activity & Note Service Cleanup

- **Targets:** `CreateNote`
- **Approach:**
  1. Slice the method into the existing private helpers where possible (`buildActivityPubNote`, `prepareMediaAttachments`, `emitStatusCreatedEvents`, etc.) and introduce new ones for conversation threading and analytics updates.
  2. Convert the conversation-ID resolution block into a helper (`resolveConversationID`) with dedicated tests.
  3. Ensure error handling remains consistent by returning early inside helpers and wrapping errors at the top level.
- **Expected Result:** Core orchestration logic remains in `CreateNote` (<30 complexity) with clear sequencing. Regression risk is mitigated by unit tests against the new helpers.

## Workstream 5 – Cost Aggregation Refactor

- **Target:** `GetAggregatedCostsByPeriod`
- **Approach:**
  1. Introduce helper functions: `fetchAggregatesForOperation`, `mergeAggregatesByWindow`, and `finalizeCostMetrics`.
  2. Replace nested loops with a streaming approach that yields chunks via channels or iterative helper to keep each loop focused.
  3. Add unit tests using fake repositories to exercise pagination, empty datasets, and merge scenarios.
- **Expected Result:** Complexity reduced by separating fetch, merge, and finalize stages. Tests validate the aggregation math.

## Workstream 6 – Validation & Tooling

- After each workstream, rerun `golangci-lint run --config .golangci.yml --enable-only gocognit`.
- Maintain existing unit/integration coverage; extend where new helpers are introduced.
- Update developer documentation (if needed) to reference the new ActivityPub helper utilities or repository update patterns.

## Timeline / Sequencing

1. **Week 1:** Workstream 2 (highest complexity, least external coupling) and foundational helper extraction for Workstream 1.
2. **Week 2:** Complete ActivityPub helper adoption across services (Workstream 1) with regression tests.
3. **Week 3:** Address timelines (Workstream 3) and note creation refactor (Workstream 4), coordinating with API team for GraphQL verification.
4. **Week 4:** Finish cost aggregation refactor (Workstream 5) and run a full lint/test sweep.

This sequencing prioritizes risk reduction and shared helper work before tackling user-facing flows. Adjustments can be made if downstream teams need specific fixes sooner.

