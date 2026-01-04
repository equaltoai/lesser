# Lesser: 10/10 Plan (Quality, Consistency, Completeness, Security)

This plan turns the 2026-01-04 audit into sequenced, measurable work to bring Lesser from the current **8/10s** to **10/10** across: **Quality**, **Consistency**, **Completeness**, and **Security**.

## Baseline (2026-01-04)

| Category | Grade |
| --- | ---: |
| Quality | 8/10 |
| Consistency | 8/10 |
| Completeness | 8/10 |
| Security | 8/10 |

### What was actually verified

- Ran: `./lesser test unit`, `./lesser lint` (0 issues), `./lesser sec-scan` (gosec: 0 issues), `./lesser vuln-check` (no vulns), `./lesser verify ci` (lint + security + supply-chain + docs/schema/OpenAPI/GraphQL coverage + inventory/lambda-set parity + unit tests).
- Manual review focused on: auth/session + CSRF, outbound HTTP/SSRF, federation signatures + authorized fetch, GraphQL abuse limits, log scrubbing/PII hygiene, and key CDK constructs (IAM/KMS/CloudFront).

### Key blockers to 10/10 (prioritized)

**Security**

- High: CloudFront CSP weakens XSS defenses if serving HTML (CSP includes `'unsafe-inline'` and `'unsafe-eval'` and can override stricter origin headers): `infra/cdk/constructs/cloudfront_caching.go:330`, `infra/cdk/constructs/api_caching.go:452`.
- High: Unbounded reads of untrusted HTTP bodies (`io.ReadAll(resp.Body)`) in production paths: `pkg/federation/delivery.go:210`, `pkg/observability/webhook_delivery.go:476`, `pkg/federation/routing/route_manager.go:1523`, `pkg/storage/repositories/federation_repository.go:1359`.
- Medium: Broad execute-api scope (`arn:aws:execute-api:*:*:*/*`) for `ManageConnections`/`Invoke`: `infra/cdk/stacks/shared_stack.go:216`, `infra/cdk/stacks/shared_stack.go:219`.
- Medium: OAuth client secrets stored as plaintext field: `pkg/storage/models/oauth_client.go:24`.
- Low: “Insecure TLS” capability exists (gated): `pkg/httpclient/federation_client.go:81`.
- Low: Misleading config knob (`WebhookConfig.VerifySSL`) exists but is unused: `pkg/observability/webhook_delivery.go:37`.

**Quality / Consistency / Completeness**

- Maintainability hot spot: `cmd/inbox/main.go` is very large, increasing review surface and making subtle bugs harder to spot.
- Tooling mismatch: staticcheck Go version pinned to 1.21 while module targets Go 1.25: `.golangci.yml:126`, `go.mod:3`.
- Disabled/parked artifacts remain: `pkg/auth/refresh_tokens_test.go.disabled`, `graph/dataloader_test.go.disabled`, `pkg/moderation/advanced/pattern_repository_bridge.go.disabled`.

### Strengths to keep (already in place)

- GraphQL has enforced token/depth/complexity limits; introspection defaults off.
- SSRF protections exist and are used by hardened outbound HTTP clients.
- Log scrubber exists for PII/sensitive hygiene.
- `./lesser verify ci` already enforces lint + security + supply chain + contract drift prevention.

Related docs:

- `docs/lesser-code-audit.md` (audit details)
- `docs/security-milestones.md` (security milestone framing)

## What “10/10” Means (Definition of Done)

10/10 means “excellent by default”, not “perfect forever”. The criteria below are intentionally measurable and CI-enforceable.

### Security 10/10 (Abuse-resilient and reviewable)

- CloudFront CSP is behavior-specific and never weakens HTML routes (no `'unsafe-eval'`; no `'unsafe-inline'` for HTML) and does not unintentionally override stricter origin CSP for dynamic pages.
- All outbound HTTP reads from untrusted servers are size-capped and safely logged (truncated snippets only; scrubber always applied).
- IAM policies are least-privilege (no broad wildcards where stack outputs can scope ARNs).
- OAuth client secret storage is hardened (hashed or KMS-encrypted) with a migration + rotation story.
- Debug-only TLS bypasses cannot be enabled accidentally in production.
- `./lesser verify ci` blocks regressions in all of the above.

### Quality 10/10 (Maintainable, testable, change-friendly)

- No “god files” in critical paths: largest handlers/services are split into cohesive packages with clear boundaries.
- Complexity budgets (gocyclo/gocognit) are enforced and trending down in hotspots.
- Tests cover security-critical helpers and failure modes (size caps, SSRF edge cases, signature failures).

### Consistency 10/10 (One way to do the important things)

- Toolchain/analyzer versions are aligned (Go/staticcheck/golangci-lint settings match `go.mod`).
- One canonical approach for outbound HTTP: client construction, SSRF validation, timeouts, response-size caps, and logging behavior.
- Config knobs match behavior (no dead flags that mislead operators).

### Completeness 10/10 (No “mystery meat”)

- No `*.go.disabled` files without an explicit tracked replacement and rationale.
- No “not implemented” placeholders outside mocks/examples.
- Docs are accurate, CLI-first, and reflect actual runtime/security posture.

## Milestones (sequenced for safety + momentum)

### M0 — Turn the audit into enforced gates (P0)

Goal: prevent regressions while the high-impact fixes land.

- [x] Add `./lesser verify audit` and run it as part of `./lesser verify ci`, enforcing:
  - CloudFront/CDK CSP regressions (no new `'unsafe-eval'` / `'unsafe-inline'` usage in `infra/cdk` beyond the tracked baseline).
  - No new `io.ReadAll(resp.Body)` callsites in non-test code beyond the tracked baseline.
  - No new `*.go.disabled` files (baseline until removed).
  - `.golangci.yml` staticcheck `go:` matches `go.mod`.
- [x] Document the new gates in `CONTRIBUTING.md` and link from this plan.

Details: `CONTRIBUTING.md` (“Quality bar (what we gate on)”).
Baseline: `tools/audit_gates/baseline.yml`.

**Acceptance criteria**
- `./lesser verify ci` fails on new violations and is green on main.

**Suggested verification**
```bash
./lesser verify audit
./lesser verify ci
```

### M1 — CloudFront CSP hardening (P0 security)

- [x] Split response header policies by behavior (HTML vs APIs/assets) so CSP is not one-size-fits-all.
- [x] Remove `'unsafe-eval'`; remove `'unsafe-inline'` for HTML behaviors (use nonces/hashes if inline scripts/styles are required).
- [x] Ensure origin CSP is preserved for dynamic HTML (avoid CloudFront overriding when the origin should be authoritative).
- [x] Add CDK assertions/unit tests verifying the deployed policy.
- [x] Update docs/runbooks: how to extend CSP safely.

**Acceptance criteria**
- HTML routes served via CloudFront have a strict CSP without unsafe directives, and origin CSP is not unintentionally weakened.

**Suggested verification**
```bash
./lesser test unit
./lesser verify ci
```

### M2 — Cap untrusted outbound HTTP body reads (P0 security + P1 quality)

- [ ] Add a shared helper: read response bodies with a hard cap + truncation marker (and ensure `resp.Body.Close()` always happens).
- [ ] Replace unbounded `io.ReadAll(resp.Body)` callsites in:
  - `pkg/federation/delivery.go`
  - `pkg/observability/webhook_delivery.go`
  - `pkg/federation/routing/route_manager.go`
  - `pkg/storage/repositories/federation_repository.go`
- [ ] Ensure logs only include truncated snippets and scrubber remains enforced.
- [ ] Add unit tests: cap enforced, snippet formatting stable, and large bodies do not allocate unbounded memory.

**Acceptance criteria**
- No production code reads untrusted HTTP bodies without a size cap.

**Suggested verification**
```bash
./lesser lint
./lesser test unit
./lesser verify ci
```

### M3 — Least-privilege execute-api permissions (P1 security)

- [ ] Replace wildcard `arn:aws:execute-api:*:*:*/*` with stack-scoped API/stage ARNs where possible.
- [ ] Split policies/roles by function (websocket management vs invoke) to reduce blast radius.
- [ ] Add CDK assertions that fail on wildcard execute-api resources.

**Acceptance criteria**
- Shared roles cannot invoke/manage arbitrary API Gateway resources outside the intended stack outputs.

**Suggested verification**
```bash
./lesser verify ci
```

### M4 — Harden OAuth client secret storage (P1 security + P2 completeness)

Decision needed: hash vs encrypt (threat model + operational requirements).

- [ ] Pick and document an approach:
  - Hash (preferred): Argon2id/bcrypt; only verify equality; never recover.
  - Encrypt: KMS envelope encryption; recoverable for specific workflows.
- [ ] Implement storage + verification changes with backwards-compatible migration.
- [ ] Add secret rotation workflow and operator docs.
- [ ] Add tests: migration, verification, and “never log the secret” regression coverage.

**Acceptance criteria**
- New secrets are not stored in plaintext; legacy secrets are migrated/rotated without downtime.

**Suggested verification**
```bash
./lesser test unit
./lesser sec-scan
./lesser verify ci
```

### M5 — Config + tooling hygiene (P2 consistency + P2 completeness)

- [ ] Wire `WebhookConfig.VerifySSL` to actual TLS verification behavior (default true) or replace it with a clearly named setting that matches the intended semantics.
- [ ] Add guardrails so `AllowInsecureTLS` cannot be enabled accidentally in production (explicit env + warning + CI gate).
- [ ] Align `.golangci.yml` staticcheck `go:` with `go.mod` (and pin toolchain versions where needed).
- [ ] Resolve `*.go.disabled` artifacts: re-enable with fixes, replace with tracked skipped tests, or move behind build tags with documented rationale.

**Acceptance criteria**
- No dead config knobs; toolchain/analyzer versions are consistent; disabled artifacts have an explicit, tested story.

**Suggested verification**
```bash
./lesser lint
./lesser test unit
./lesser verify ci
```

### M6 — Reduce maintainability hotspots (P2 quality)

- [ ] Break `cmd/inbox/main.go` into cohesive internal packages (routing, validation, federation, persistence) with clear interfaces.
- [ ] Add targeted unit tests around extracted units (parsers, validators, signature flows, idempotency).
- [ ] Ratchet complexity budgets downward for the moved code.

**Acceptance criteria**
- `cmd/inbox` entrypoint is mostly wiring; complex logic lives in testable packages with focused unit tests.

**Suggested verification**
```bash
./lesser lint
./lesser test unit
./lesser verify ci
```

## Recommended sequencing

1. M0 → 2. M1 → 3. M2 → 4. M3 → 5. M4 → 6. M5 → 7. M6

## Tracking (suggested)

Create one epic per milestone and tag PRs with:

- `security`, `quality`, `consistency`, `docs`, `testing`, `infra`


Each PR should include:

- The acceptance criteria it closes
- The verification commands executed (copy/paste)
- Any behavior changes or migration notes
