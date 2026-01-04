# Lesser Code Audit Report (Deep Review)

Date: 2026-01-04  
Repo: `penny-advanced-interfaces/lesser`

## Executive Summary (Grades 1–10)

| Category | Grade | Why |
| --- | ---: | --- |
| Quality | 8/10 | Strong modularity and test discipline; remaining drag is primarily maintainability hotspots (very large handlers/services) and a few legacy sharp edges. |
| Consistency | 8/10 | Canonical GraphQL limits, URL/SSRF validation, and log scrubbing are now standardized; some naming/shape inconsistencies remain in older handlers. |
| Completeness | 9/10 | Deterministic verification gates exist, “implementation status” tooling is signal-not-noise, and docs are closer to reality; remaining work is largely M4 refactors and CI/supply chain hardening. |
| Security | 8/10 | GraphQL abuse limits + SSRF URL policy + default log scrubbing are in place with regression tests; CSP tightening and broader outbound-call auditing remain. |

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

**Impact:** default `script-src` includes `'unsafe-inline'`; nonce generation exists but doesn’t remove unsafe inline allowances, reducing XSS hardening.

**Location:**
- `pkg/middleware/security_headers.go` default CSP directives.

**Recommended remediation:** shift to nonce-based `script-src`/`style-src` (remove `'unsafe-inline'`), tighten `frame-src`/`connect-src` to known needs, and add tests for header output.

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

**Recommended remediation:** centralize “blocked IP / metadata” logic into a shared helper (or export from `pkg/httpclient`) and apply consistently.

## Category Notes

### Quality (7/10)

- Good:
  - Clear domain separation in `pkg/` and extensive unit tests; coverage baseline is high (repository artifact `coverage.out` reports ~77% statements).
  - Consistent use of contextual errors and typed error helpers in many areas.
- Needs attention:
  - Historically, some security checks looked correct but did not enforce what they claimed (DNS-rebinding/TOCTOU); primary outbound clients are now hardened, but duplicated SSRF logic remains.
  - Some overbroad “string-prefix” network checks (e.g., blocking all `172.*`) risk breaking legitimate traffic.

### Consistency (6/10)

- Good:
  - Package organization is mostly coherent (`pkg/services`, `pkg/storage`, `graph/`).
  - “CLI-first” workflow exists (`./lesser`), and repo tooling is robust.
- Needs attention:
  - Multiple bespoke SSRF validators exist; URL validation rules should be consistent across federation/media/AI download paths.

### Completeness (6/10)

- Good:
  - Many features have coverage and harness support; multiple verification commands exist.
- Needs attention:
  - Tracked TODOs/incomplete items exist (generate via `scripts/check_implementation_status.sh`), and some docs don’t perfectly match current behavior.

### Security (6/10)

- Good:
  - There is an explicit SSRF-focused HTTP client (`pkg/httpclient`), and middleware contains a comprehensive security header set.
- Needs attention:
  - Consolidate SSRF/IP-blocking logic and verify remaining outbound call sites (including proxy behavior) follow the same rules.
  - CSP defaults should be tightened to match the presence of nonce support.
  - Sensitive auth artifacts should not be logged.

## Recommended Remediation Plan

See:

- `docs/planning/lesser-10of10-plan.md` (10/10 milestones)
- `docs/security-milestones.md` (security milestone framing)
