# Lift Framework Remediation Plan

This document translates the review findings into a phased execution plan with clear
objectives, sequencing, and implementation guidance. Every previously catalogued issue
(1-25) appears below as a task with explicit subtasks, file pointers, and recommended
validation steps.

## Phase 1 - Production Stabilisation (Weeks 1-2)

### Objectives
- Restore end-to-end request handling so generated services behave as documented.
- Reinstate the runtime guardrails (timeouts, payload limits, tenant enforcement) that production teams rely on.
- Ensure instrumentation toggles and CDK patterns deliver the experience described in the README.

### Exit Criteria
- Lambda deployments created via `lift new` successfully proxy requests through `App.HandleRequest` in integration tests.
- Requests exceeding configured body/response limits or lacking tenant identifiers are rejected with structured errors.
- Telemetry toggles and the API URL getters behave according to documentation, with regression tests guarding the paths.

### Work Breakdown

**P1.1 Restore Lambda request pipeline** *(Issue 6)*
- Subtasks:
  - Delegate `deployment.LambdaDeployment.Handle` to `app.HandleRequest` (`pkg/deployment/lambda.go`).
  - Translate the Lift response/error types into Lambda-compatible payloads.
  - Update CLI scaffolding (`pkg/cli/commands.go`) and add integration tests that execute the generated binary against sample HTTP and SQS events.
- Guidance: Reuse existing adapter logic to keep behaviour consistent; add golden tests under `pkg/deployment`.

**P1.2 Enforce runtime guardrails** *(Issue 1)*
- Subtasks:
  - Enforce `MaxRequestSize` before unmarshalling in each adapter.
  - Wrap handler execution with `context.WithTimeout` based on `config.Timeout`.
  - Validate response payload sizes prior to returning to the platform.
  - Emit structured metric/log entries when limits trip.
- Guidance: Extend `pkg/lift/app.go` and adapters; add unit tests to `pkg/lift/adapters` covering limit breaches.

**P1.3 Align event parsing with documentation** *(Issue 2)*
- Subtasks:
  - Populate `Request.Body` for SQS/S3 adapters or introduce typed helpers (e.g. `ctx.SQSRecords()`).
  - Update README examples and add regression tests in `pkg/lift/adapters`.
- Guidance: Ensure size checks reuse the logic from P1.2.

**P1.4 Complete response validation** *(Issue 3)*
- Subtasks:
  - Implement `validateResponse` in `pkg/features/validation.go`, mirroring request-side validation.
  - Introduce response buffering or context flags required for schema validation.
  - Add failure metrics and test coverage.
- Guidance: Consider using existing validator interfaces to avoid duplicating schemas.

**P1.5 Enforce tenant requirements** *(Issue 4)*
- Subtasks:
  - Enforce `RequireTenantID` centrally in `pkg/lift/app.go`.
  - Feed violations into metrics/logging.
  - Align middleware helpers (`pkg/middleware/auth.go`) with the central flag to prevent divergence.
- Guidance: Add tests ensuring tenant-less requests fail when the flag is enabled.

**P1.6 Honour telemetry toggles** *(Issue 5)*
- Subtasks:
  - Gate tracer and metrics initialisation off `TracingEnabled`/`MetricsEnabled` (`pkg/lift/app.go`).
  - Provide hooks for custom tracer/metrics factories.
  - Extend docs and add integration tests that assert instrumentation toggles.
- Guidance: Ensure disabling metrics shuts down collectors cleanly to support later phases.

**P1.7 Fix CDK API URL getters** *(Issue 20)*
- Subtasks:
  - Restore `$default` stage creation or compute URLs from the custom stage in `pkg/cdk/constructs/api.go`.
  - Update pattern getters (`pkg/cdk/patterns/basic_api.go`, `secure_api.go`) and regenerate tests.
- Guidance: Add assertions in pattern tests verifying `GetApiUrl()` returns non-nil values post-synthesis.

**P1.8 Apply middleware to non-HTTP triggers** *(Issue 25)*
- Subtasks:
  - Ensure `routeEvent` and `HandleEvent` execute the global middleware chain for SQS/S3/EventBridge routes, covering logging and metrics (`pkg/lift/app.go`, `pkg/lift/event_router.go`).
  - Provide an explicit escape hatch so HTTP-only middleware (auth) is skipped or tagged as inapplicable for non-HTTP triggers.
  - Update developer documentation/README to clarify which middleware applies to event-driven handlers and why auth is excluded by default.
- Guidance: Reuse existing middleware pipeline infrastructure; add integration tests that assert logging/metrics middleware runs for SQS/S3 events while auth does not.

## Phase 2 - Reliability & Resilience (Weeks 2-4)

### Objectives
- Fix retry, caching, load-shedding, and encryption defects that threaten runtime stability.
- Ensure background schedulers and middleware cease cleanly to prevent goroutine leaks.

### Exit Criteria
- HTTP and service-client retries behave deterministically with full payloads.
- Secret cache operations are free from data races and panic conditions under load tests.
- Encryption requires non-empty keys before releasing artefacts.

### Work Breakdown

**P2.1 Correct retry string matching** *(Issue 7)*
- Replace the bespoke prefix-only helper with substring matching (`pkg/services/client.go`).
- Add unit tests covering mid-string matches.

**P2.2 Stop leaking load-shedding goroutines** *(Issue 8)*
- Gate metrics collector startup on config flags and expose a stop hook in `pkg/middleware/loadshedding.go`.
- Tie the ticker to app lifecycle context; add cleanup coverage in tests.

**P2.3 Rebuild HTTP requests between retries** *(Issues 9 & 12)*
- Use `req.GetBody` or rebuild requests inside `executeWithRetry` (`pkg/services/client.go`).
- Ensure headers/context are cloned per attempt; add regression tests for POST/PUT retries.

**P2.4 Fix secret-cache concurrency** *(Issues 10 & 11)*
- Upgrade to write locks before mutating maps in `pkg/security/secrets.go`.
- Add concurrent expiration tests and vet race-free behaviour.

**P2.5 Enforce non-empty encryption keys** *(Issue 16)*
- Validate `DataProtectionConfig.EncryptionKey` in `pkg/security/dataprotection.go`.
- Emit configuration errors and extend documentation/tests.

**P2.6 Validate background scheduler intervals** *(Issue 17)*
- Default zero-valued ticker durations for analytics/performance schedulers (`pkg/observability/analytics/performance_analytics.go`).
- Return descriptive errors when configuration is invalid; add unit tests to cover zero intervals.

## Phase 3 - Observability & Analytics Maturity (Weeks 4-6)

### Objectives
- Deliver multi-tenant observability as documented (metrics, tracing, sampling).
- Stabilise analytics and metrics collectors to prevent panics and support restart scenarios.

### Exit Criteria
- Tenant/user dimensions flow automatically through metrics and tracing helpers.
- Enhanced observability honours sample rates with accompanying regression tests.
- Analytics engine can restart without panics; derived collectors close safely.

### Work Breakdown

**P3.1 Inject tenant/user dimensions** *(Issues 13 & 14)*
- Update CloudWatch metrics (`pkg/observability/cloudwatch/metrics.go`) and tracer middleware (`pkg/middleware/enhanced_observability.go`) to auto-attach tenant/user tags.
- Document extension points (`TenantIDFunc`, `UserIDFunc`) and add tests.

**P3.2 Honour enhanced observability sample rate** *(Issue 15)*
- Implement probabilistic sampling or provider-specific sampling rules in `pkg/middleware/enhanced_observability.go`.
- Cover behaviour with unit tests.

**P3.3 Prevent metrics collector double-close panics** *(Issue 18)*
- Track ownership/reference counts for `stopCh`/`doneCh` in `pkg/observability/cloudwatch/metrics.go`.
- Add tests that close derived collectors in varying orders.

**P3.4 Allow analytics engine restarts** *(Issue 19)*
- Reinitialise `stopCh` on `Start` and protect closure with idempotent logic in `pkg/observability/analytics/performance_analytics.go`.
- Add tests covering `Start -> Stop -> Start` flows.

## Phase 4 - Operational Excellence & Platform Readiness (Weeks 6-8)

### Objectives
- Ensure disaster-recovery automation and analytics tooling behave predictably under operational workflows.
- Harden shared infrastructure components (connection pools, optimizers) for production-scale usage.

### Exit Criteria
- DR testing no longer blocks health monitoring, and tickers survive default configurations.
- Connection pools respect configured ceilings and expose safe telemetry.
- Auto-optimisation produces actionable recommendations when enabled.

### Work Breakdown

**P4.1 Release locks during DR test notifications** *(Issue 21)*
- Refactor `performDRTest` to release `drm.mu` before sleeping and re-acquire only when mutating shared state (`pkg/disaster/recovery.go`).
- Add concurrency tests simulating health events during test scheduling.

**P4.2 Validate DR ticker configuration** *(Issue 22)*
- Default zero durations for health and testing tickers (`pkg/disaster/types.go`, `pkg/disaster/recovery.go`).
- Return configuration errors for invalid schedules; add unit coverage.

**P4.3 Enforce connection pool ceilings & safe stats** *(Issue 23)*
- Track total active+idle connections to honour `MaxConnections` in `pkg/performance/connection_pool.go`.
- Prevent division-by-zero in `PoolStats`; add tests for empty and saturated pools.

**P4.4 Surface auto-optimisation results** *(Issue 24)*
- Iterate `po.optimizers` inside `OptimizePerformance`, honour `EnableAutoOptimize`, and populate `result.Optimizations` (`pkg/testing/performance/optimizer.go`).
- Log/aggregate optimizer errors and extend tests with a mock optimizer that verifies the flow.

## Cross-Cutting Tasks

- **Documentation refresh**: Update README, CDK docs, and API references wherever behaviour changes (notably Phases 1 & 3).
- **Test matrix expansion**: Add integration suites for Lambda deployments, multi-tenant metrics, DR workflows, and connection pool saturation.
- **Release communication**: Prepare upgrade notes detailing breaking changes (tenant enforcement, encryption key validation, stricter limits).

## Implementation Guidance

- Tackle phases sequentially; each phase assumes the prior one is code-complete and passing CI.
- Maintain short-lived branches per phase to simplify reviews; land with feature flags where rollout risk is high.
- For runtime changes, add load/integration tests before promoting to production accounts.
- Capture metrics and logs during each phase to validate behavioural changes (e.g., rate of limit rejections, new telemetry tags).

---

*Issue mapping reference:*

| Issue | Phase | Task |
|-------|-------|------|
| 1 | P1 | P1.2 |
| 2 | P1 | P1.3 |
| 3 | P1 | P1.4 |
| 4 | P1 | P1.5 |
| 5 | P1 | P1.6 |
| 6 | P1 | P1.1 |
| 7 | P2 | P2.1 |
| 8 | P2 | P2.2 |
| 9 | P2 | P2.3 |
| 10 | P2 | P2.4 |
| 11 | P2 | P2.4 |
| 12 | P2 | P2.3 |
| 13 | P3 | P3.1 |
| 14 | P3 | P3.1 |
| 15 | P3 | P3.2 |
| 16 | P2 | P2.5 |
| 17 | P2 | P2.6 |
| 18 | P3 | P3.3 |
| 19 | P3 | P3.4 |
| 20 | P1 | P1.7 |
| 21 | P4 | P4.1 |
| 22 | P4 | P4.2 |
| 23 | P4 | P4.3 |
| 24 | P4 | P4.4 |
| 25 | P1 | P1.8 |
