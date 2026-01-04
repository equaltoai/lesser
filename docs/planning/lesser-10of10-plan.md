# Lesser: 10/10 Codebase Plan (Quality, Consistency, Completeness, Security)

This document synthesizes the current audit findings into an actionable plan to bring Lesser to **10/10** across:
**Quality**, **Consistency**, **Completeness**, and **Security**.

## Baseline (observed)

Current observed grades (2026-01-04):

| Category | Grade |
| --- | ---: |
| Quality | 8/10 |
| Consistency | 7/10 |
| Completeness | 8/10 |
| Security | 7/10 |

Supporting signals and repo tooling:

- Lint: `./lesser lint` → `0 issues`
- Security static scan: `./lesser sec-scan` (gosec) → `0 issues`
- Go vuln scan: `govulncheck -show verbose ./...` → `0` reachable vulns (but `github.com/aws/aws-sdk-go` has module-level advisories)
- Unit tests: `./lesser test unit` → pass
- Docs drift: `./lesser verify docs` → pass
- Coverage snapshot: `go tool cover -func=coverage.out` → ~`77%` statements (repo artifact; not a gate yet)

Related audit artifacts:

- `docs/lesser-code-audit.md` (older snapshot; update as part of this plan)
- `docs/security-milestones.md` (existing milestone framing; extend/close gaps)

## What “10/10” Means (Definition of Done)

10/10 means “excellent by default”, not “perfect forever”. The criteria below are intentionally measurable.

### Quality 10/10 (Maintainable, testable, change-friendly)

- No “god files” in critical paths: largest handlers/services are broken into cohesive packages with clear boundaries.
- Panics are limited to true invariants (startup wiring, codegen); runtime request paths never panic without recovery.
- Clear interfaces + dependency injection where it reduces coupling (esp. outbound IO).
- Tests are fast, reliable, and meaningful: unit tests run deterministically; integration tests are scoped and opt-in.
- Complexity budgets are enforced (gocyclo/gocognit) and steadily ratcheted down in hotspots.

### Consistency 10/10 (One way to do the important things)

- One canonical approach each for:
  - URL validation + SSRF rules
  - outbound HTTP client construction
  - auth/session/cookie primitives
  - error shaping (REST + GraphQL)
  - logging fields (request_id, actor/user, tenant/app)
- Environment/config keys are standardized; legacy keys are supported via deprecation shims and documented.
- Security middleware selection is uniform (API vs federation vs media vs websocket).

### Completeness 10/10 (No “mystery meat”)

- No disabled tests (`*.go.disabled`) without an explicit tracked replacement.
- No “not implemented” placeholders outside mocks/examples.
- Pagination semantics are consistent across REST + GraphQL and fully documented.
- Docs are accurate, CLI-first, and aligned with the intended passwordless posture.
- Coverage targets are explicit and met for security-critical packages.

### Security 10/10 (Abuse-resilient and reviewable)

- GraphQL has enforced depth/complexity/time limits; introspection/playground are gated.
- All outbound HTTP that touches user-controlled URLs is SSRF-hardened and uses shared rules.
- No misleading “constant-time” comparisons; auth/security comparisons use `crypto/subtle`.
- Logging is scrubbed by default; sensitive fields never appear in logs even on errors.
- Supply chain is actively managed (reachable vulns: 0; module vulns: 0 or documented exceptions + compensating controls).
- CI runs security checks and blocks merges on regressions.

## Milestones (Sequenced for Safety + Momentum)

### M0 — Establish the 10/10 bar (1–2 days)

- [ ] Create a “quality bar” doc section in `docs/CONTRIBUTING.md` (what’s gated; what’s best-effort).
- [ ] Add a single `./lesser verify all` (or equivalent) command that CI can run deterministically.
- [ ] Decide and document target numbers:
  - GraphQL max depth + complexity
  - Min coverage for security-critical packages (`pkg/auth`, `pkg/httpclient`, `pkg/ssrf`, `pkg/middleware`)
  - Complexity thresholds (start with “no worse than today”, then ratchet).

**Acceptance criteria**
- One command exists that a CI job can run to validate quality/security/docs gates.
- Targets are written down (even if initially conservative).

**Suggested verification**
```bash
./lesser verify
./lesser lint
./lesser sec-scan
govulncheck ./...
./lesser test unit
./lesser verify docs
```

### M1 — GraphQL abuse-resilience (P1 security + P1 quality)

Primary gaps: GraphQL currently lacks enforced limits and enables introspection unconditionally.

- [ ] Add enforced GraphQL query depth limit.
- [ ] Add enforced GraphQL complexity limit (use gqlgen complexity config/extensions).
- [ ] Gate introspection (and playground) behind `DebugMode` and/or explicit allowlist config.
- [ ] Add request timeout enforcement at the handler layer (ensure every request has a deadline).
- [ ] Add tests that prove limits are enforced (unit tests; no network required).
- [ ] Add operational docs: “why limits exist”, “how to tune”, “how to debug”.

**Acceptance criteria**
- In non-debug deployments: introspection disabled and queries above limits are rejected with clear errors.
- Limits are covered by tests and cannot regress silently.

**Suggested verification**
```bash
go test ./cmd/graphql/... ./graph/... ./pkg/...
./lesser test unit
```

### M2 — Canonical outbound URL + SSRF rules (P2 consistency + P1 security)

Primary gap: federation client URL validation is weaker/different than the general secure client.

- [ ] Create a single shared URL validation helper (scheme + hostname + blocked host suffixes + IP literal rules).
- [ ] Apply it to:
  - `pkg/httpclient` SecureClient
  - `pkg/httpclient` FederationClient
  - AI/media download paths (and any other external fetchers)
- [ ] Add redirect validation parity tests (scheme/host/blocked hostnames).
- [ ] Decide proxy posture for hardened clients (explicitly disable proxies or validate proxy behavior) and document it.

**Acceptance criteria**
- The same URL/SSRF policy is enforced across all outbound paths that can be influenced by untrusted input.
- Tests cover representative edge cases (IPv6, IP literals, metadata hostnames, mixed A/AAAA).

**Suggested verification**
```bash
go test ./pkg/httpclient/... ./pkg/ssrf/... ./pkg/ai/...
./lesser sec-scan
```

### M3 — Auth primitives correctness + log hygiene (P2 security + P2 consistency)

- [x] Replace any “constant-time compare” placeholders with real constant-time comparisons (`crypto/subtle`).
- [x] Ensure scrubber is applied to all logger cores used in production Lambdas (not just opt-in call sites).
- [x] Add “sensitive logging” regression tests (authorization headers, JWTs, wallet signatures, CSRF tokens).
- [x] Normalize auth/security event logging fields (username/user_id, request_id, ip prefixing, user agent).

**Acceptance criteria**
- Security comparisons are truly constant-time where claimed/needed.
- Sensitive values cannot appear in logs in normal error paths.

**Suggested verification**
```bash
go test ./pkg/auth/... ./pkg/logging/...
./lesser sec-scan
```

### M4 — Pay down maintainability hotspots (P1 quality + P2 consistency)

Primary gap: several critical files are extremely large, which increases regression risk and review cost.

- [ ] Refactor the largest Lambda handlers into cohesive internal packages:
  - `cmd/inbox` (route handlers, validation, persistence, federation logic)
  - `cmd/outbox`
  - `cmd/activity-processor`
- [ ] Refactor oversized services/repositories similarly:
  - `pkg/services/accounts`
  - `pkg/services/notes`
  - `pkg/storage/repositories/user_repository`
- [ ] Remove or narrow runtime `panic(...)` usage; ensure every Lambda entrypoint has panic recovery.
- [ ] Tighten complexity budgets incrementally and keep the trend line moving down.

**Acceptance criteria**
- Critical entrypoints are navigable (clear files/dirs by responsibility) and easier to test in isolation.
- No new panics are introduced in request paths; panics are recovered consistently.

**Suggested verification**
```bash
./lesser lint
./lesser test unit
```

### M5 — “No surprises” completeness pass (P1 completeness)

- [x] Re-enable disabled tests (rename/fix):
  - `graph/dataloader_test.go.disabled`
  - `pkg/auth/refresh_tokens_test.go.disabled`
  - `pkg/moderation/advanced/pattern_repository_bridge.go.disabled`
- [x] Make `scripts/check_implementation_status.sh` ignore transient caches (e.g. `tmp/go-mod-cache`) to remove false positives.
- [x] Resolve remaining pagination TODO markers and document pagination rules for REST + GraphQL.
- [x] Align auth docs with the passwordless posture; remove/mark deprecated flows.
- [x] Update `docs/lesser-code-audit.md` to reflect current reality and link to this plan.

**Acceptance criteria**
- No disabled tests remain without a documented replacement and rationale.
- “Implementation status” report is signal, not noise.
- Docs and code agree on auth posture and operator workflow.

**Suggested verification**
```bash
bash scripts/check_implementation_status.sh
./lesser verify docs
./lesser test unit
```

### M6 — Supply chain + CI hardening (P1 security + P1 quality)

- [x] Remove/replace legacy dependency `github.com/aws/aws-sdk-go` (indirect v1 removed by migrating to `github.com/aws/aws-xray-sdk-go/v2`).
- [x] Add CI (GitHub Actions) that builds lambdas and runs `./lesser verify ci`.
- [x] Add a reproducible module inventory snapshot (`./lesser verify supply-chain` → `report/module_inventory.txt`) and run it in `./lesser verify ci`.
- [x] Add a release checklist that includes security scanning + docs verification.

**Acceptance criteria**
- Reachable vulns: 0; module-level vulns: 0 (or explicitly accepted with rationale).
- CI enforces the quality bar on every PR.

## Recommended Sequencing

1. M0 (define the bar) → 2. M1 (GraphQL limits) → 3. M2 (SSRF/URL policy unification)
4. M3 (auth + logs) → 5. M5 (completeness) in parallel with 6. M4 (refactors)
7. M6 (CI + supply chain) once the gates are stable

## Tracking (Suggested)

Create one tracking epic per milestone and tag PRs with:

- `security`, `quality`, `consistency`, `docs`, `testing`, `infra`, `deps`

Each PR should include:

- The acceptance criteria it closes
- The verification commands executed (copy/paste)
- Any behavior changes or migration notes
