## Lesser Stub and Placeholder Audit (Current Code)

Date: 2025-08-09

Scope: Entire repository, focusing on Go code. This audit identifies TODOs, "not implemented" markers, placeholders, and areas referencing email/SMS notification channels (which Lesser does not support).

### Summary (Go code only)

- TODO/FIXME/TBD/XXX/HACK: 293 matches across 90 files
- "not implemented"/unimplemented/NYI: 12 matches across 6 files
- "placeholder"/WIP: 57 matches across 22 files
- Panic("not implemented"): 0 active occurrences in Go code (example/commented occurrences exist in examples and docs)

Note: Repository-wide (including docs and tests) contains many additional matches; counts above are restricted to Go code to reflect runtime impact.

### High-Priority Implementation Gaps

- Blurhash generation not implemented
  - File: `graph/schema.resolvers.go`
  - Evidence: comment stating blurhash generation is not implemented (near the attachments resolver).
  - Impact: Degraded media previews; user experience and compatibility with Mastodon clients affected.
  - Action: Implement blurhash generation during media processing and/or lazily for existing media; persist in attachment metadata.

- Federation routing: batch coordination missing
  - File: `pkg/federation/routing/query_optimizer.go`
  - Evidence: "Batch query coordinator removed - not implemented".
  - Impact: Suboptimal federation query efficiency and cost; potential duplicate fetches.
  - Action: Implement a batch coordinator with request coalescing and fan-out control.

- OAuth storage repository placeholders
  - File: `pkg/storage/repositories/oauth_repository.go`
  - Evidence: Placeholder section for additional OAuth operations.
  - Impact: Incomplete OAuth scenarios (e.g., auth codes/refresh flows) may be missing.
  - Action: Fill out CRUD and lifecycle operations for codes/tokens aligned with `models`.

### Medium-Priority Implementation Gaps / Placeholders

- GraphQL resolvers: simulated/sample data in analytics and federation
  - File: `graph/schema.resolvers.go`
  - Evidence: multiple sections mark simulated statuses for domains, sample AI statistics, and storage-location strings.
  - Impact: Non-authoritative analytics/federation insight returned to clients.
  - Action: Replace simulated values with real repository-backed metrics and instance health data.

- Moderation filters: update handlers log "not implemented"
  - File: `graph/phase3.resolvers.go`
  - Evidence: severity/assignedTo/priority/unhandled filter updates log not-implemented debug messages.
  - Impact: Filter UI changes won’t persist; potential admin UX gaps.
  - Action: Add fields to filter structures and wire through repository methods.

- Transactional patterns: conceptual tests reference unimplemented transaction ops
  - File: `pkg/storage/dynamorm/repositories/transaction_support_test.go`
  - Evidence: tests expect placeholder errors for transactional operations.
  - Impact: No production impact if not used at runtime; signals missing transactional support in storage.
  - Action: Implement transactional helpers when required or remove conceptual tests if out-of-scope.

- WebSocket connection cleanup uses demonstration/sample data
  - File: `pkg/storage/repositories/streaming_connection_repository.go`
  - Evidence: "For demonstration purposes, create sample stale connections"; TTL cleanup placeholder notes.
  - Impact: Risk of non-deterministic behavior if sample logic remains enabled; audit usage path.
  - Action: Ensure sample code is gated for tests only; implement real stale-scan queries.

- Metrics aggregator and federation health cleanup placeholders
  - Files: `cmd/metrics-aggregator/main.go`, `pkg/federation/health/serverless_checker.go`
  - Evidence: placeholders for cleanup routines.
  - Impact: Potential data retention issues if cleanup not implemented elsewhere.
  - Action: Move cleanup into repositories with tested retention policies; wire from lambdas.

- Moderation metrics placeholder weights
  - File: `pkg/storage/repositories/moderation_metrics_repository.go`
  - Evidence: hardcoded confidence aggregation placeholder (e.g., 0.8).
  - Impact: Inaccurate confidence metrics.
  - Action: Parse and weight by actual confidence scores.

- Featured tags placeholder URL
  - File: `pkg/storage/repositories/featured_tag_repository.go`
  - Evidence: URL uses `https://localhost/...` placeholder.
  - Impact: Broken links in clients.
  - Action: Build URLs from configured base domain.

### Email/SMS References (Notifications Not Supported)

Policy: Lesser will not support email or SMS notifications.

Findings:

- Notification preferences include email flags
  - Files: `graph/model/models_gen.go` (`NotificationPreferences.Email`), `graph/generated.go` (email field wiring), `pkg/storage/types.go` (`NotificationPreferences` includes `Email`), `pkg/storage/models/notification_preferences.go` (email-enabled field persisted).
  - Tests and models explicitly ignore email/SMS delivery
    - Files: `cmd/notification-processor/main_test.go` (email channel returns false), `pkg/storage/models/notification_cost_tracking.go` (email/sms costs return 0 and are ignored), `pkg/storage/models/notification_delivery.go` (email/sms blocked).
  - Risk: APIs may expose email notification toggles that don’t function, causing confusion.
  - Actions:
    - Remove or hard-disable `email` in API schemas and resolvers for notification preferences.
    - Ensure all delivery paths reject `email`/`sms` consistently (already enforced in core paths), and remove dead code for email delivery.
    - Update UI/API documentation to explicitly state that only push/websocket/in-app are supported.

Non-notification email usage (acceptable):

- User/account records and validation utilities use email for identity and contact metadata (e.g., `pkg/storage/repositories/user_repository.go`, validation rules). These are in-scope.

### Additional Notable Placeholders (Go code)

- S3 placeholder object creation for streaming directories
  - File: `pkg/media/streaming/storage.go` (creates `.placeholder` objects per quality).

- Observability/example code and infra
  - Files: `pkg/observability/emf_integration_example.go`, `infra/cdk/stacks/monitoring_stack.go` (placeholder dashboard widgets).

- Cost calculations
  - File: `pkg/cost/scheduled_job_cost_tracker.go` (external API cost placeholder logic).

### Recommended Remediation Plan

1) Critical API correctness
   - Implement blurhash generation.
   - Remove or hard-disable email/SMS notification preferences in GraphQL models and resolvers; keep server enforcement rejecting these channels.
   - Replace simulated analytics/federation samples with repository-backed values.

2) Federation efficiency and cleanup
   - Implement batch query coordinator in federation routing.
   - Replace placeholder cleanup logic with repository-level retention functions invoked by lambdas.

3) Storage and moderation quality
   - Solidify transactional helpers or defer/remove conceptual transaction tests until required.
   - Replace placeholder confidence aggregation with real parsing and weighting.
   - Build featured tag URLs from configured base domain.

4) Code health
   - Systematically resolve TODOs in active code paths; convert long-lived TODOs to issues and remove comments.
   - Guard or remove demonstration/sample code paths from production builds.

### File Index (selected excerpts)

- `graph/schema.resolvers.go`
  - Blurhash: not implemented comment near attachment/attachment preview resolvers.
  - Simulated federation/analytics values in multiple sections (e.g., AI statistics, storage location strings).

- `graph/phase3.resolvers.go`
  - NotificationPreferences constructed with `Email: true` by default; contradicts non-support policy.
  - Moderation filter update handlers log not-implemented.

- `pkg/federation/routing/query_optimizer.go`
  - Explicit "not implemented" note regarding batch coordination.

- `pkg/storage/repositories/oauth_repository.go`
  - Placeholder section indicating more OAuth operations to implement.

- `pkg/storage/repositories/streaming_connection_repository.go`
  - Demonstration/sample stale connection generation; TTL cleanup placeholder notes.

- `cmd/metrics-aggregator/main.go`, `pkg/federation/health/serverless_checker.go`
  - Cleanup placeholders noted in code.

- `pkg/storage/repositories/moderation_metrics_repository.go`
  - Placeholder confidence weighting (e.g., 0.8 multiplier) rather than parsed scoring.

- `pkg/storage/repositories/featured_tag_repository.go`
  - Placeholder domain used for tag URLs.

### Acceptance Criteria for Closure

- No user-facing APIs return simulated/placeholder data.
- Blurhash generation present for new media; backfill plan documented or executed.
- Federation routing implements batch coordination or an alternative with equivalent performance.
- Email/SMS notification fields removed or forced to false in all public APIs; delivery paths reject these methods.
- Demonstration/sample code paths not reachable in production.
- Placeholder cleanup functions replaced by repository implementations with tests.

### Detailed Implementation Tasks and Subtasks

1) Blurhash generation (media previews)
- Implement in media processing pipeline
  - [ ] Use `pkg/media/blurhash.go` to generate blurhash for images and video keyframes
  - [ ] Generate during upload/transcoding; avoid per-request computation
  - [ ] Persist on `models.MediaAttachment` (ensure field exists and is indexed if queried)
- Resolver wiring
  - [ ] `graph/schema.resolvers.go`: return stored blurhash, never simulate
  - [ ] Add nil-safe fallback only when legacy records lack blurhash
- Backfill
  - [ ] Add backfill job in `cmd/media-processor` to crawl media without blurhash and populate
  - [ ] Add idempotency and rate limiting; track costs
- Tests
  - [ ] Unit test blurhash generation for common media types
  - [ ] Resolver test verifies non-empty blurhash when available

2) Federation routing batch coordinator
- Design and interfaces
  - [ ] Define `BatchCoordinator` interface in `pkg/federation/routing`
  - [ ] Coalesce outbound fetches by domain and path with dedupe keys and TTL
- Implementation
  - [ ] Add in-memory (Lambda-lifecycle) and optional DynamoDB-backed coordination
  - [ ] Integrate with `route_manager.go` and existing circuit breaker
  - [ ] Ensure cost tracking hooks fire once per physical request
- Tests
  - [ ] Concurrency tests proving coalescing under load
  - [ ] Failure-path tests: fallback when coordination unavailable

3) OAuth repository placeholders
- Scope missing methods (examples)
  - [ ] `CreateAuthorizationCode`, `GetAuthorizationCode`, `ConsumeAuthorizationCode`
  - [ ] `CreateRefreshToken`, `RevokeRefreshToken`, `ListActiveTokens`
- Storage/model support
  - [ ] Define models with TTL and GSIs for code lookups/user scoping
  - [ ] Implement repository CRUD with validation and expiry
- Integration
  - [ ] Wire into `pkg/auth` flows; retire any in-memory fallbacks
- Tests
  - [ ] Unit tests for code issuance/consumption and token revocation
  - [ ] Property tests for TTL and uniqueness constraints

4) Replace simulated GraphQL analytics/federation data
- Identify simulated sections
  - [ ] `graph/schema.resolvers.go`: AI stats, storage location strings, instance federation status
- Replace with repository-backed data
  - [ ] Add/extend methods in `AnalyticsRepository`, `FederationRepository` to fetch real metrics
  - [ ] Remove hardcoded counts/ratios; compute from stored aggregates or time series
- Tests
  - [ ] Snapshot or golden tests using seeded fixtures in `pkg/storage/dynamorm/repositories/testing`

5) Moderation filter updates (not-implemented logs)
- Schema and storage
  - [ ] Extend filter structs to include `Severity`, `AssignedTo`, `Priority`, `Unhandled`
  - [ ] Add persistence model and repository methods to create/update filters
- Resolvers
  - [ ] Implement the update resolvers in `graph/phase3.resolvers.go`
  - [ ] Enforce validation and authorization
- Tests
  - [ ] Unit and resolver tests ensuring persisted state changes

6) Transactional support (conceptual tests currently placeholder)
- Decision
  - [ ] Confirm if DynamORM provides transaction semantics required; if not needed, mark tests as skipped with rationale
- If needed
  - [ ] Implement transaction helpers in `pkg/storage/dynamorm/repositories`
  - [ ] Replace placeholder errors in `ConditionalCreate/Update/Delete`
  - [ ] Add integration tests verifying atomicity using mock DB or local Dynamo

7) WebSocket stale connection cleanup
- Replace demo/sample logic
  - [ ] Remove sample connection creation; ensure not reachable in production
  - [ ] Implement query for stale records based on `LastSeen`/TTL and index
- Aggregator wiring
  - [ ] `cmd/websocket-cost-aggregator/main.go`: call repository cleanup, record reclaimed cost
- Tests
  - [ ] Repository unit tests for stale detection and deletion

8) Metrics aggregator and federation health cleanup
- Repository retention
  - [ ] Implement retention deletion methods with partitioned scans and batch writes
- Lambda handlers
  - [ ] Replace placeholder cleanup with repository calls; make retention window configurable
- Tests
  - [ ] Time-based tests ensuring only data older than threshold is removed

9) Moderation metrics placeholder weighting
- Proper confidence parsing
  - [ ] Update types to carry numeric confidences; parse from stored strings if needed
  - [ ] Replace hardcoded multipliers with calculated weights
- Tests
  - [ ] Unit tests covering distribution aggregation and edge cases

10) Featured tag URL placeholder
- Build real URLs
  - [ ] Inject configured BaseURL/domain into repository
  - [ ] Replace `https://localhost/...` with `fmt.Sprintf("%s/tags/%s", baseURL, tagName)`
- Tests
  - [ ] Ensure URLs are correct for custom domains/tenants

11) Email/SMS in notifications (not supported by Lesser)
- API/schema hard-disable
  - [ ] In GraphQL schema and generated models, deprecate or remove `NotificationPreferences.email`
  - [ ] If removal is breaking, return `false` and mark as deprecated; document non-support
- Backend enforcement
  - [ ] Verify all delivery paths reject `email`/`sms` (already enforced in `notification_delivery.go` and cost tracking)
  - [ ] Remove dead code related to email/SMS delivery
- Documentation
  - [ ] Update API docs to reflect supported channels (push, websocket, in-app)
- Tests
  - [ ] Ensure attempts to enable email/SMS are no-ops and/or return validation errors

12) Streaming S3 placeholder objects
- Production readiness
  - [ ] Confirm necessity of `.placeholder` objects; if only for listings, consider S3 List with prefixes
  - [ ] If retained, guard behind config and exclude from costs/metrics

13) Observability/examples and infra placeholders
- Build hygiene
  - [ ] Ensure example/placeholder code is excluded from production builds (build tags or separate packages)
  - [ ] Populate monitoring dashboards via IaC stacks instead of placeholders

14) External API cost placeholder logic
- Pricing config
  - [ ] Add configuration for external API unit costs per provider
  - [ ] Replace fixed microcents with configured rates and usage counters
- Tests
  - [ ] Unit tests for cost calculation with multiple providers/tiers

### Per-Item Definitions of Done (DoD)
- Blurhash: generated and persisted for new media; resolvers return stored value; backfill job run on a sampled dataset; tests green.
- Federation batching: coalescing functional under concurrency; measurable reduction in outbound calls; tests cover happy and failure paths.
- OAuth repo: all placeholder methods implemented; tokens/codes persisted with TTL; auth flows pass integration tests.
- GraphQL analytics/federation: no simulated values; numbers sourced from repositories; snapshot tests updated.
- Moderation filters: persisted updates; UI round-trip works; auth/validation enforced.
- Transactions: either implemented with tests or explicitly documented as out-of-scope with tests skipped.
- WebSocket cleanup: no demo data paths; stale deletion measurable; alerts recorded.
- Metrics/federation cleanup: repository retention in place; handlers configurable; tests for retention window.
- Moderation metrics: confidence parsing implemented; aggregation validated.
- Featured tag URLs: use configured domain; links valid in clients.
- Email/SMS: fields removed or deprecated and forced false; delivery enforcement confirmed; docs updated.
- Streaming placeholders: production behavior free of placeholder artifacts.
- Observability/infra: examples not built; dashboards non-empty after deploy.
- External API costs: config-driven calculations; unit tests pass.

### Notes

- Counts and findings reflect current code, not historical documents. This task plan is designed to be executed incrementally while maintaining a releasable branch.


