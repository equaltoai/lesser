# Security + Quality Milestones

This document converts the audit priorities into milestones with acceptance criteria.

## Priorities

- **P0 (blocker):** SSRF / DNS rebinding hardening for outbound HTTP clients.
- **P1 (high):** Remove sensitive material from logs; tighten CSP defaults.
- **P2 (medium):** Fix doc drift and verifier false positives; reduce duplicated security logic.

## Milestone 1 — SSRF / DNS Rebinding Hardening (P0)

**Goal:** ensure outbound HTTP connections are established only to pre-validated public IPs (no TOCTOU gap between “DNS check” and “dial”), including federation and AI media download.

**Acceptance criteria**
- `pkg/httpclient` `SecureClient` dials a validated IP address (not a hostname) after resolving and applying private/link-local/metadata blocks.
- `pkg/httpclient` `FederationClient` dials a validated IP address (not a hostname) after resolving and applying private/link-local/metadata blocks.
- Unit tests assert that the dial target is `IP:port` for both clients (no network required).
- `pkg/ai` image download defaults to a hardened HTTP client (SSRF-safe).
- Targeted tests pass: `go test ./pkg/httpclient/...` and `go test ./pkg/ai/...`.

## Milestone 2 — Sensitive Logging + CSP Tightening (P1)

**Goal:** reduce exposure of authentication artifacts and improve browser-side defenses.

**Acceptance criteria**
- No code path logs raw wallet signatures (log sanitized metadata only).
- Default CSP removes `'unsafe-inline'` for scripts/styles and uses the existing nonce mechanism.
- Development CSP remains usable for local work (explicitly limited to dev configuration).
- Header behavior is covered by unit tests (nonce present when enabled; policies emitted as expected).

## Milestone 3 — Documentation Alignment (P2)

**Goal:** ensure docs match the CLI-first workflow and current implementation.

**note from user**
Lesser is intended to be passwordless and any code or docs saying otherwise are incorrect.

**Acceptance criteria**
- `./lesser verify docs` passes without drift findings.
- Auth documentation is consistent with current behavior (passwordless/OAuth flow descriptions match code paths).
- Remove or rephrase doc lines that trigger verifier false positives while preserving meaning.

## Milestone 4 — Consolidate SSRF Helpers (P2)

**Goal:** apply one consistent definition of “blocked IP/host” across the codebase.

**Acceptance criteria**
- Private/link-local/metadata IP detection is implemented once (shared helper) and reused by federation, AI download, and general outbound HTTP clients.
- No duplicated IP-block lists remain in the main SSRF paths.
- Regression tests cover representative edge cases (IPv6, IP literals, metadata IPs, mixed A/AAAA results).

