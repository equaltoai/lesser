# Lint Remediation Plan (Unused & Revive)

## Scope
- `make lint` currently reports 29 `unused` and 18 `revive` findings in addition to other lint failures.
- This document focuses on the `unused` diagnostics (dead symbols) and the `revive` warnings (unused params, package/exported comments) so we can unblock lint.

## Summary of Recommended Sequencing
- **Phase 1 – Dead code cleanup:** remove or repurpose symbols that are no longer wired in (`unused` category) to shrink surface area and avoid future regressions.
- **Phase 2 – Resolver parameter fidelity:** make the GraphQL subscription manager respect the parameters that gqlgen exposes (thresholds, severities) or adjust the schema/signatures if the values are no longer part of the contract.
- **Phase 3 – Documentation & hygiene:** add the missing package/exported comments and tighten lingering helper signatures so `revive` stays green.

## `unused` Diagnostics

| Location | Symbols | Assessment | Proposed Action |
| --- | --- | --- | --- |
| `cmd/graphql-ws/main.go:257` | `(*wsServer).persistSubscription` | Legacy helper; all subscription persistence now runs through `GraphQLSubscriptionManager.createSubscriptionRecord`. No call sites remain. | Delete the method and drop any now-unused imports. |
| `cmd/metrics-processor/main.go:537-876` | `createStreamingEvent`, `extractEventUserInfo`, `buildEventMetadata`, `createBaseEventMetadata`, `addEventUserInfo`, `addEventPercentiles`, `addEventDimensions`, `addEventKnownDimensions`, `addEventCustomDimensions`, `calculateEventPriority`, `isSecurityOrModerationEvent`, `isHighVolumeEvent`, `logPublishError`, `logPublishSuccess`, `determineEventStreams` | These were part of the pre-stream-router event publishing path. The live code now builds GraphQL subscription payloads through `publishMetricsSubscriptionEvent` and the newer `determinePriority` helpers. Keeping both sets risks divergence. | Delete the unused block and rely on the newer helpers; ensure any tests still compiling after the removal. |
| `graph/schema.resolvers.go:4932,4980` | `convertEventToQuoteActivity`, `convertEventToModerationItem` | Replaced by `graph/event_converter.go` once the subscription manager began centralising conversions. No references remain outside this file. | Remove helpers and update imports; confirm generated resolvers no longer reference them. |
| `graph/subscription_manager.go:288` | `marshalFilterMetadata` | Never invoked; leftover from an earlier attempt to persist filter JSON alongside subscriptions. Could become useful if we store per-subscription thresholds. | Either delete now or wire it back in when implementing threshold-aware persistence (see revive items below). Document the chosen path. |
| `pkg/config/config.go:419` | `getEnvOrPanic` | No callers. Environment lookups use the existing `getEnvOrDefault`/`mustGetJWTSecret` helpers. | Remove the function; if a panic-on-miss helper is still desired create it when a caller exists. |
| `pkg/services/hashtags/service.go:468,568` | `wrapActivityEvent`, `buildHashtagStreams` | Helpers for the deprecated in-process hashtag event bus. The service now returns an immediately closed channel and GraphQL handles streaming. | Delete the helpers and associated comments; ensure no tests rely on them. |
| `pkg/services/registry.go:1744-1815` | `createNotesFederationAdapter`, `createAccountsFederationAdapter`, `createRelationshipsFederationAdapter`, `createThreadsFederationAdapter` | Locking variants were superseded by the `create<Foo>FederationAdapterUnlocked` helpers invoked inside the service initialisers. | Remove the unused locked variants; keep the unlocked versions used in `Notes()` / `Accounts()` initialisation. |
| `tmp/test_compile.go:13-16` | `metaAdapter` type + methods | Compile-time scaffold that no longer feeds any interface assertions or runtime code. Lints fail because nothing references the type. | Either remove the file entirely or convert it to a `_test.go` file with explicit interface assertions (e.g. `var _ dynamorm.MetadataAdapter = (*metaAdapter)(nil)`). |

> **Cross-check**: after deleting code, run `make lint` + `make test` to catch any implicit dependencies that may have been relying on package init side effects.

## `revive` Diagnostics

### Unused parameters (revive `unused-parameter`)

| Location | Parameter | Why it is unused today | Decision |
| --- | --- | --- | --- |
| `cmd/metrics-processor/main.go:816` | `event` in `publishEventAndLog` | Method was reduced to a no-op when DynamoDB streams took over; we still pass the event but never log it. | Keep the argument but log useful identifiers (e.g. `event.ID`, `event.Streams`) so we retain observability without tripping revive. |
| `graph/subscription_manager.go:263` | `ctx` in `deleteSubscriptionRecords` | Helper constructs a fresh background context, ignoring caller cancellation/deadline. | Switch to `context.WithTimeout(ctx, ...)` so upstream cancellation propagates. |
| `graph/subscription_manager.go:443` | `threshold` in `SubscribeToCostUpdates` | GraphQL schema exposes a threshold, but we drop it when constructing stream filters. | Decide between (a) implementing threshold-aware filtering (likely involves storing metadata per subscription and filtering inside the dispatcher) or (b) removing the argument from the schema if the feature is abandoned. Prefer wiring it up to avoid API churn. |
| `graph/subscription_manager.go:537` | `noteObj` in `SubscribeToQuoteActivity` | Stub leftover from the in-memory bus; the GraphQL resolver always passes `nil`. | Remove the parameter from the manager/subscription interfaces and update call sites. |
| `graph/subscription_manager.go:559` | `threshold` in `SubscribeToMetricsUpdates` | Value is ignored when building stream names; clients expect threshold filtering. | Implement server-side filtering (e.g. wrap the channel and drop updates below threshold, or encode threshold in subscription metadata so stream-router can filter). |
| `graph/subscription_manager.go:790` | `severity` in `SubscribeToModerationAlerts` | We subscribe to global + per-user streams but never apply the severity filter requested by clients. | Extend subscription routing to include severity (new stream suffix or in-channel filter) or remove the argument from schema if we cannot support it. |
| `graph/subscription_manager.go:811` | `thresholdUSD` in `SubscribeToCostAlerts` | Similar story—threshold propagated from GraphQL but ignored by the manager. | Same as cost updates: either use it (store in metadata / filter) or amend the schema. |
| `graph/subscription_manager.go:832` | `severity` in `SubscribeToPerformanceAlerts` | Value is ignored when mapping to streams. | Align with moderation alerts—add severity-aware routing or adjust schema. |
| `pkg/services/hashtags/service.go:308` | `ctx` in `GetHashtagActivity` | Deprecated method closes the channel immediately and never inspects the context. | Honour cancellation by checking `ctx.Err()` before work/logging or, if we keep the stub, rename the argument to `_` and document the deprecation. |

> _Note:_ Several of the subscription-related items likely need the same underlying change: persist per-subscription filters (thresholds/severities) and make the stream-router aware of them. Capture this as a single work item to avoid solving each in isolation.

### Package comment violations
Add a package-level comment summarising the module purpose for:
- `cmd/graphql-ws`
- `pkg/activitypubutil`
- `pkg/ratelimit`
- `pkg/storage/converters`
- `tmp/test_compile` (or delete the file as suggested above)

### Exported declarations missing comments
Either document the symbol or make it unexported if the wider project never uses it:
- `pkg/storage/repositories/account_repository.go:512` – `UserUpdatePayload` is file-local; rename to `userUpdatePayload` if we do not expose it, otherwise add a doc comment describing the update envelope.
- `pkg/storage/repositories/account_repository_social.go:148` – `GetFollowers` is part of the public repository contract; add a short comment describing the pagination semantics.
- `pkg/storage/repositories/activity_repository.go:135` – `GetInboxActivities`; add documentation noting filtering/ordering behaviour.
- `pkg/storage/repositories/cost_tracking_repository.go:1566` – `GetRelayCostSummary`; document the time range expectation and returned aggregate.

### Follow-up validation
- After applying the fixes above, rerun `make lint` to confirm `unused`/`revive` categories are clean.
- Run `make test` (with `ENVIRONMENT=dev` / `STAGE=dev` defaults) to ensure subscription behaviour still passes coverage, especially if we rewire filters.
- If we choose to alter the GraphQL schema (e.g., removing unused parameters), regenerate gqlgen artefacts and update any client documentation.

