# Lesser Code Audit Report (Deep Review)

Date: 2026-01-04  
Repo: `penny-advanced-interfaces/lesser`

## Executive Summary (Rubric v1.0)

Numerical scoring is defined by `docs/planning/lesser-10of10-rubric.md` (versioned, repeatable). The table below is the
rubric-based scorecard at the time of this audit.

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | 8/10 | QUA-3 (pkg coverage ≥ 90.0%) |
| Consistency | 10/10 | — |
| Completeness | 8/10 | COM-4 (OpenAPI strict) |
| Security | 10/10 | — |

## Method & Scope

- Reviewed SSRF-related outbound HTTP clients and URL validation paths (`pkg/httpclient`, `pkg/ai`).
- Reviewed authentication-adjacent code and middleware defaults (`pkg/auth`, `pkg/middleware`).
- Sampled repo health artifacts (coverage reports, doc verification scripts, “incomplete implementations” inventory).
- Did not perform dynamic penetration testing; findings are from static review + unit-test inspection.

## Key Findings (Prioritized)

### P0 — SSRF / DNS rebinding hardening (addressed)

**Impact:** attacker-controlled hostnames can pass “resolved-IP checks” but later be dialed by hostname again, reintroducing private-network access via DNS rebinding (TOCTOU between check and use).

**Locations:**
- `pkg/httpclient/client.go` (now dials resolved IPs via a cloned `*http.Transport` with a hardened `DialContext`).
- `pkg/httpclient/federation_client.go` (now dials resolved IPs inside `secureDialContext`).
- `pkg/ai/ssrf_http_client.go` (AI image download defaults to an SSRF-protected `*http.Client` without importing `pkg/httpclient`).

**Status:** addressed by dialing resolved IPs (not hostnames) after validation, with shared URL validation (`pkg/ssrf`) and
unit tests asserting `IP:port` dialing.

### P1 — GraphQL abuse-resilience limits (addressed)

**Impact:** without enforced limits, GraphQL endpoints are susceptible to resource exhaustion via deep/complex queries.

**Status:** enforced request timeout, depth limit, complexity limit, parser token limit, and introspection gating are now
implemented and configurable.

### P1 — Sensitive material logged (addressed)

**Impact:** authentication artifacts can end up in logs (and therefore log retention/search systems).

**Status:**
- Sensitive log scrubbing is enabled on the default production loggers (core-wrapped zap).
- Scrubbing covers both messages and fields, including error fields (`zap.Error(...)`), with regression tests.

### P1 — CSP defaults are permissive for scripts/styles (open)

**Impact:** if unsafe CSP directives are enabled outside development, XSS hardening is significantly reduced.

**Location:**
- `pkg/middleware/security_headers.go` (`DevelopmentSecurityHeadersConfig()` includes `'unsafe-inline'`/`'unsafe-eval'`).

**Status:** default CSP is nonce-based and does not include unsafe directives; development config intentionally relaxes CSP.

**Recommended remediation:** ensure “dev-only CSP” cannot be enabled accidentally in production deployments and keep CSP
coverage in tests (including any CloudFront header policies).

### P2 — Doc drift + verifier false positives (addressed)

**Impact:** doc verification tooling can block changes unrelated to real drift; auth documentation can mislead implementers.

**Locations:**
- `scripts/check_implementation_status.sh` now excludes transient caches and avoids substring false positives (e.g. “Mastodon”).
- `pkg/auth/README.md` is aligned to Lesser’s passwordless posture and points to architecture docs.

**Recommended remediation:** adjust docs to match the CLI-first workflow and reconcile auth docs with actual code paths.

### P2 — Duplicated SSRF/IP-blocking logic

**Impact:** security fixes may be applied in one place but missed elsewhere; inconsistent allow/block decisions across features.

**Locations:**
- `pkg/httpclient/client.go` vs `pkg/httpclient/federation_client.go` vs `pkg/ai/service.go`.
  - (dialing implementation lives in `pkg/ai/ssrf_http_client.go`)

**Recommended remediation:** centralize “blocked IP / metadata” logic into a shared helper (or export from `pkg/httpclient`) and apply consistently.

## Category Notes (Narrative)

### Quality

- Good:
  - Clear domain separation in `pkg/` and extensive unit tests; coverage baseline is high (repository artifact `coverage.out` reports ~77% statements).
  - Consistent use of contextual errors and typed error helpers in many areas.
- Needs attention:
  - Historically, some security checks looked correct but did not enforce what they claimed (DNS-rebinding/TOCTOU); primary outbound clients are now hardened, but duplicated SSRF logic remains.
  - Some overbroad “string-prefix” network checks (e.g., blocking all `172.*`) risk breaking legitimate traffic.

### Consistency

- Good:
  - Package organization is mostly coherent (`pkg/services`, `pkg/storage`, `graph/`).
  - “CLI-first” workflow exists (`./lesser`), and repo tooling is robust.
- Needs attention:
  - SSRF policy is centralized in `pkg/ssrf`, but SSRF-hardened dialing exists in multiple callers (`pkg/httpclient`, `pkg/ai`); keep behavior aligned and consider consolidating the dial logic.

### Completeness

- Good:
  - Many features have coverage and harness support; multiple verification commands exist.
- Needs attention:
  - Keep strict contract checks (especially OpenAPI) green to prevent spec drift.

### Security

- Good:
  - There is an explicit SSRF-focused HTTP client (`pkg/httpclient`), and middleware contains a comprehensive security header set.
- Needs attention:
  - Keep SSRF-hardened outbound paths consistent (including proxy behavior and “dial validated IPs” invariants).
  - Keep “dev-only relaxations” (CSP, insecure TLS) gated so they cannot be enabled accidentally.

## Recommended Remediation Plan

See:

- `docs/planning/lesser-10of10-rubric.md` (versioned scoring)
- `docs/planning/lesser-10of10-roadmap.md` (milestones mapped to rubric IDs)
- `docs/planning/lesser-10of10-plan.md` (Phase 1 hardening history)
- `docs/security-milestones.md` (security milestone framing)
