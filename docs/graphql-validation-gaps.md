# GraphQL Validation Gaps

Maintains the running list of issues or work still required to complete GraphQL verification for `dev.lesser.host`. Each item should include the scope, current status, owner (when known), and the next concrete step.

## Open Items

- **Remote follow propagation**  
  - *Scope*: When federation is fully wired, outbound follow activities should enqueue to the federation delivery queue.  
  - *Status*: Placeholder guard prevents panics; actual queue adapter not yet implemented.  
  - *Owner*: Federation / Relationships team (TBD).  
  - *Next step*: Build and inject the queue-backed `FederationService` during Lambda init, then retest `followActor` for remote actors.
- **Analytics metric collisions**  
  - *Scope*: `createNote` intermittently returns HTTP 503 because `TrendingRepository.RecordInstanceMetric` hits a `ConditionalCheckFailed` when attempting to create a duplicate daily metric item.  
  - *Status*: Metrics table writes succeed for the first post, then fail on subsequent posts for the same day, aborting the mutation.  
  - *Owner*: Analytics service (TBD).  
  - *Next step*: Switch to an update/increment pattern (or tolerant upsert) for instance metrics so note creation no longer depends on a unique PutItem.
- **Status counters stale**  
  - *Scope*: `actor(username:...) { statusesCount }` still returns `0` for all personas even after multiple successful `createNote` mutations.  
  - *Status*: Dynamo writes exist (`status#…` items), but the counters/GSIs feeding `statusesCount` aren’t updated.  
  - *Owner*: Notes/Timeline service (TBD).  
  - *Next step*: Backfill counter updates (or query derived counts) as part of the note create path before Phase 3 timeline validation.
## Resolved Items

- **CreateNote mutation timeout**  
  - *Scope*: `createNote` requests previously hit a 30 s Lambda timeout because the service registry deadlocked when initialising the analytics dependency, and Dynamo validation rejected empty primary/index keys.
  - *Resolution*: Registry now initialises analytics without re-locking, status models set PK/SK/timestamps during `UpdateKeys`, and optional GSI attributes skip empty strings. `createNote` returns HTTP 200 and writes `status#…` records on `lesser-development` (validated 2025-10-20).  
  - *Owner*: GraphQL / Notes service.
- **Follow activity published timestamp**  
  - *Scope*: `followActor` GraphQL mutation failed when requesting `published` because the resolver returned `null` for a non-null field.
  - *Resolution*: Relationships service now builds follow activities with canonical IDs and timestamps; the GraphQL resolver falls back to setting `published` during response construction. Verified via mutation against `dev.lesser.host` on 2025-10-20.  
  - *Owner*: Relationships service (GraphQL team).
- **Relationship listings omit display names**  
  - *Scope*: `followers`/`following` GraphQL queries returned actors with empty `displayName` even when the profile defined one (e.g., admin → “Administrator”).  
  - *Resolution*: `buildAccountFromActor` now hydrates accounts with stored user metadata when available, so relationship queries surface display names. Verified with `followActor` + `following`/`followers` checks on `dev.lesser.host` (2025-10-19).  
  - *Owner*: Relationships service (GraphQL team).
- **User preference model/table alignment**  
  - *Scope*: Loading user preferences during profile mutations crashed with invalid struct tag errors and attempted to read from a nonexistent `UserPreferenceses` table.  
  - *Resolution*: `models.UserPreferences` now maps to the main table without unsupported `dynamorm` tags, unblocking preference hydration for authenticated GraphQL requests (2025-10-20).  
  - *Owner*: Storage team.
- **Follow activity object hydration**  
  - *Scope*: `followActor` responses previously returned `object: null`, preventing clients from showing the followed actor metadata.  
  - *Resolution*: Relationships service now embeds the followed actor in the activity payload and the GraphQL resolver converts it to a `model.Object`, returning full actor details. Verified against `dev.lesser.host` on 2025-10-20.  
  - *Owner*: Relationships service (GraphQL team).
- **CreateNote activity payload**  
  - *Scope*: The `createNote` mutation persisted statuses but returned `activity.object: null`, preventing clients from rendering the new note from the mutation response.  
  - *Resolution*: Activity resolver now detects `*models.Status` payloads and converts them via `convertStatusToObject`, returning hydrated GraphQL objects. Covered by `TestActivityResolverObjectConvertsStatus` (`JWT_SECRET=test-secret go test ./graph/...`) on 2025-10-20.  
  - *Owner*: GraphQL / Notes service.
- **Analytics metrics IAM mismatch**  
  - *Scope*: GraphQL `createNote` emitted `AccessDeniedException` when analytics attempted to read from the legacy `lesser-table` DynamoDB table.  
  - *Resolution*: All analytics/trending models now use `MainTableName` (`lesser-development` in dev). After redeploy (2025-10-20 @ 16:50 UTC), `createNote` executes with no CloudWatch errors.  
  - *Owner*: Analytics / Operations team.
- **Profile mutation stores**  
  - *Scope*: `updateProfile` GraphQL mutation previously failed with `failed to store account` because Dynamo optimistic locking rejected updates on `USER#*` records.
  - *Resolution*: Actor/user models now carry canonical `Version` attributes and `AccountRepository.UpdateAccount` writes through the normalized DynamORM update path. Verified via `updateProfile` mutation on `dev.lesser.host` (2025-10-20 @ 16:32 UTC) with CloudWatch logs showing version increment and no errors.  
  - *Owner*: Accounts service (GraphQL team).
- **Timeline retrieval coverage**  
  - *Scope*: `timeline(type: HOME, …)` previously returned empty edges because seeded personas lacked posts and legacy context values failed to unmarshal.  
  - *Resolution*: ActivityPub context now normalizes through a DynamORM type converter and personas are reseeded with fresh notes; home/public timelines return populated edges (validated via GraphQL queries on 2025-10-21).  
  - *Owner*: GraphQL / Notes service.
