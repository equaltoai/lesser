# GraphQL Validation Remediation Plan

Owner: Project Management / API Platform  
Scope: Resolve validation gaps identified on 2025-10-29 for `dev.lesser.host` and establish automated coverage for the GraphQL schema.

## Phase 1 – Data Hydration & Resolver Fixes

**Goal:** Restore baseline query functionality so existing seeded data surfaces correctly.

- Task 1.1 – Diagnose Actor Resolver
  - Confirm repository response for `ACTOR#admin` and peers (inspect `actor_repository` projections).
  - Patch resolver or data mapping so `id`, `username`, `createdAt`, and other non-null fields hydrate.
  - Add targeted unit/integration test for `actor(username: "admin")`.
- Task 1.2 – Restore Status Retrieval Paths
  - Trace `timeline` and `object` resolvers to ensure `PUBLIC_TIMELINE` GSI usage and status model decoding align with current table schema.
  - Validate fetch of known status `e7aba65c-a4c9-4b5a-bba7-07157d7030f7`.
  - Update tests (`tests/system/test_graphql_reads.py`) to include explicit object query after fix.
- Task 1.3 – Execute Regression Pass
  - Rerun `tests/system/test_graphql.py` and `test_graphql_reads.py`; record results.
  - Update `docs/graphql-validation-gaps.md` with resolved items.

## Phase 2 – Error Handling & Empty-State Coverage

**Goal:** Ensure GraphQL returns graceful empty payloads instead of backend errors when data is absent.

- Task 2.1 – Relationship & Discovery Queries
  - Fix `followers`/`following` to degrade to empty `ActorListPage` when no edges exist.
  - Adjust `lists`, `suggestions`, `profileDirectory` to return empty collections without errors.
  - Extend system tests to assert empty state responses.
- Task 2.2 – Moderation & Federation Endpoints
  - Harden `moderationQueue`, `conversations`, and `federationStatus` to capture repository/provider errors and translate them to user-facing messages or empty payloads.
  - Capture CloudWatch logs (if available) for root cause hints.
  - Add integration smoke checks to `tests/system/test_graphql_reads.py` and document expected behavior.
- Task 2.3 – Documentation Refresh
  - Update `docs/graphql-validation-gaps.md` as items close.
  - Add troubleshooting tips to `docs/dev-seed-validation-checklist.md` for empty-state expectations.

## Phase 3 – Mutation & Subscription Validation

**Goal:** Verify core mutations and streaming endpoints once read paths are stable.

- Task 3.1 – Mutation Coverage
  - Identify critical mutations (profile updates, status creation, relationships, push subscriptions).
  - Create reproducible scripts (extend `tests/system` or add new fixtures) to exercise each mutation against dev.
  - Validate Dynamo side effects and GraphQL responses; capture evidence in docs.
- Task 3.2 – Subscription Smoke Tests
  - Investigate `timelineUpdates` Dynamo timeouts (`StreamingConnectionRepository.WriteSubscription`).
  - Coordinate with infra team to resolve Dynamo capacity or schema issues.
  - Once stable, script WebSocket subscription check and document procedure.
- Task 3.3 – Automation Hook
  - The new `make seed-and-validate` target provides a single command to seed and validate the environment.
  - Wire results into CI or scheduled validation job (optional stretch).

## Phase 4 – Reporting & Long-Term Safeguards

**Goal:** Institutionalize GraphQL validation and maintain visibility.

- Task 4.1 – Finalize Documentation
  - Convert gap notes into a living runbook (link from `docs/README.md`).
  - Document validation cadence and responsible parties.
- Task 4.2 – Metrics & Alerting
  - Define success metrics (e.g., % of queries passing, response SLAs).
  - Coordinate with observability team to instrument GraphQL error alarms for dev/staging.
- Task 4.3 – Retrospective & Handoff
  - Summarize lessons learned and future improvements.
  - Assign ongoing ownership for GraphQL schema validation (Project Manager + API team).

## Working Practices

- Use the `make seed-and-validate` target to run the automated seeding and validation process.
- Maintain `docs/graphql-validation-gaps.md` after every validation run.
- Use `AWS_PROFILE=Lesser` and seeded persona tokens for consistent auth.
- Log discoveries immediately; avoid rerolling the Dynamo dataset mid-investigation unless coordinated.
- Project manager to host weekly sync until completion, tracking tasks in issue tracker aligned with phases above.
