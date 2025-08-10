# Lesser Release Completeness Checklist

This checklist expands each audit item with concrete acceptance criteria and suggested implementation steps. Cross off each item when done and link to PRs.

## 1) Search Privacy Token Validation
- Acceptance
  - Requests to search endpoints enforce auth where required; unauthenticated flows respect privacy rules.
  - No TODOs; unit tests cover valid/invalid tokens, IP-based fallback, and rate limits.
- Implementation
  - Update `pkg/middleware/search_privacy.go`: replace TODO with JWT/actor scope validation using existing auth libs.
  - Add tests in `pkg/middleware/search_privacy_test.go` for success/failure paths.

## 2) Complete Inbox/Outbox Handlers
- Acceptance
  - `cmd/activity-processor/handler.go` processes Follow/Accept/Reject/Block/Move/Add/Remove with state changes in storage and emits necessary side effects (notifications, timeline updates, federation responses).
  - Integration tests simulate activities and assert data mutations + fanout.
- Implementation
  - Implement storage calls via repositories for relationships/collections.
  - Wire notification creation for follow/mention-like events.
  - Ensure Undo/Delete produce tombstones and reverse side effects.

## 3) Definitive Mastodon API Coverage
- Acceptance
  - A generated matrix from `cmd/api/lift/*` routes lists every endpoint and status (OK/Partial/Unsupported) with tests.
  - Docs updated to reflect accurate percentage and any intentional omissions.
- Implementation
  - Add script to enumerate handlers and compare with Mastodon spec list; produce markdown table under `docs/api/MASTODON_API_STATUS.md`.
  - Add smoke tests for critical endpoints and client flows (Ivory/Tusky/Elk compat set).

## 4) GraphQL Resolver Gaps
- Acceptance
  - No panics from generated resolvers; all schema fields have implementations or are explicitly disabled.
  - Basic queries/mutations/subscriptions covered by tests and cost tracking headers present.
- Implementation
  - Audit `graph/schema.resolvers.go` and `graph/generated.go` for unimplemented cases; add resolvers or remove schema entries not intended for GA.
  - Add resolver tests using in-memory/mocked repositories.

## 5) OAuth Discovery Endpoint Availability
- Acceptance
  - `GET /.well-known/oauth-authorization-server` returns metadata JSON from the auth Lambda.
  - Postman collection and tests validate discovery.
- Implementation
  - Expose handler in `cmd/auth` if missing; ensure infra routes map well-known path.
  - Document in `docs/security/authentication/*` and link from Quick Start.

## 6) VAPID Keys Enforcement
- Acceptance
  - Production boot fails fast or disables push endpoints when VAPID keys are absent; no placeholder keys in prod.
  - CLI `configure-instance -generate-vapid` path documented and tested.
- Implementation
  - Update `cmd/api/lift/misc.go` and `cmd/api/lift/instance.go` to conditionally expose push config; log warnings in non-prod only.
  - Add health check that reports push readiness.

## 7) Moderation Enforcement Propagation
- Acceptance
  - Suspend/Silence/Remove actions update all read models: timelines, search indexes, streaming updates; federation Undo/Delete issued when appropriate.
  - Tests verify end-to-end enforcement.
- Implementation
  - In `cmd/moderation-processor/main.go` ensure enforcement methods update downstream systems and emit required activities.
  - Hook search indexer to remove/deprioritize content on enforcement events.

## 8) Host-Meta Endpoint
- Acceptance
  - `GET /.well-known/host-meta` serves XRD/JRD linking to WebFinger.
- Implementation
  - Add simple handler in API gateway, cached via CloudFront; unit test link correctness.

## 9) Federation & Push DLQ Wiring
- Acceptance
  - Delivery and push processors have DLQ configured; failures appear in DLQ with retry metadata.
  - Runbook documented in `docs/operations`.
- Implementation
  - Confirm env vars and infra stacks for queue + DLQ; add monitoring for DLQ depth.

## 10) Remove Email/SMS References
- Acceptance
  - No code comments or docs prescribe email/SMS for notifications or auth flows.
- Implementation
  - Replace stray mentions in `pkg/auth/recovery_federation.go` and API docs with Web Push/WebAuthn alternatives.

## 11) SSE Endpoint Parity
- Acceptance
  - Ddocument WebSocket-only stance. 
- Implementation
  - Update docs/client guidance.

## 12) Infra, CORS, and Security Headers Verification
- Acceptance
  - All public endpoints have correct CORS/security headers; preflight passes for Mastodon clients and browsers.
  - Security tests confirm.
- Implementation
  - Audit middleware and API Gateway/Lambda integrations; adjust default headers and allowed origins as per config.

## 13) Alarms & Dashboards
- Acceptance
  - P0/P1/P2 alarms configured for API latency, error rate, queue depth, spend rate; dashboards populated.
- Implementation
  - Verify `pkg/monitoring` configuration and CDK stacks; add missing alarms.

## 14) Client Compatibility Smoke Tests
- Acceptance
  - Scripts run through core flows for Ivory/Tusky/Elk/Phanpy; issues are documented or fixed.
- Implementation
  - Extend `tests/api` suites and Postman collection; run in CI.

---

When all items are checked, update `docs/FEATURE_COMPATIBILITY_MATRIX.md` and `docs/api/MASTODON_API_STATUS.md` with the final, tested coverage figures.


