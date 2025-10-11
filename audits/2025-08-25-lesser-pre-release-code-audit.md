## Lesser Pre‑Release Code Audit

Date: 2025-08-25
Scope: Full repository review (security, code quality, consistency) with emphasis on ActivityPub federation, API surface, authn/z, storage access, HTTP clients/servers, and observability.

### Executive summary

- **Overall posture**: Strong foundations with thoughtful security middleware, rate limiting, SSRF-safe federation client, HTTP signature support, and observability. Architecture favors least-privilege serverless patterns.
- **Highest priorities**:
  - Remove `"'unsafe-inline'"` from CSP for web clients; rely on nonces. Tighten `connect-src` where feasible.
  - Enforce response size limits on federation HTTP client and remote actor fetches to prevent memory abuse.
  - Ensure the sensitive data scrubbing logger is consistently used in all Lambdas.
  - Turn on input validation middleware for federation endpoints (currently implemented but not applied globally).
  - Guard prod configs against `AllowInsecureTLS` and broaden SSRF allowlists/denylists as noted below.

### What we reviewed

- API bootstrap and global middleware: `cmd/api/main.go`, `pkg/middleware/*`
- Federation security: `pkg/federation/*` (HTTP signatures, digest, authorized fetch, delivery)
- HTTP client hardening: `pkg/httpclient/federation_client.go`
- Auth extraction/middleware: `pkg/auth/*`
- Logging and PII scrubbing: `pkg/logging/scrubber.go`
- Input validation audit utilities: `pkg/security/input_validation_audit.go`
- Storage layer patterns and isolation (spot checks): `pkg/storage/repositories/*`, `pkg/services/registry.go`
- Dependencies: `go.mod`

## Strengths

- **Security headers middleware** with sensible defaults and endpoint-aware profiles; HSTS enabled by default; explicit removal of `Server`/`X-Powered-By`.
- **Rate limiting** combines sliding windows, burst detection, progressive delays, and repository-backed persistence, with federation-aware limits.
- **Federation HTTP client** implements SSRF protections via custom dialer, DNS validation, blocked private/metadata ranges, redirect validation, TLS min version.
- **HTTP Signatures & Digest**: Support for hs2019/legacy algorithms; digest verification compatibility; authorized fetch path.
- **CORS**: Federation endpoints correctly allow wildcard origins with `AllowCredentials: false`. Web client CORS uses env-driven allowlists.
- **Observability**: EMF metrics, latency aggregation/alerting, tracing hooks, and centralized cost tracking middleware.
- **Logging scrubber**: Comprehensive patterns for tokens, keys, PII with a Zap Core wrapper.

## Findings and recommendations

### 1) Content Security Policy for web clients

- Observed: `WebClientSecurityHeaders()` includes `script-src 'self' 'unsafe-inline'` and `style-src 'self' 'unsafe-inline'`. Nonces are generated and injected, but `'unsafe-inline'` defeats the nonce protection.
- Risk: Inline execution widens XSS blast radius.
- Recommendation:
  - Remove `'unsafe-inline'` for `script-src` and `style-src`; rely on the generated nonce (`csp-nonce`).
  - Keep a short allowlist for any third-party origins if truly required and prefer subresource integrity.
  - Consider splitting stricter CSP for authenticated app vs. public pages.
- References: `pkg/middleware/security_headers.go` (Default and WebClient configs)

### 2) Federation client response size enforcement

- Observed: `FederationClientConfig.MaxResponseSize` exists but is not enforced when reading response bodies (e.g., actor fetch and general GET/POST paths).
- Risk: Large responses can cause memory pressure or denial of service.
- Recommendation:
  - Wrap body reads with `io.LimitedReader` using `MaxResponseSize` across federation fetch paths (e.g., actor fetch, WebFinger/remote object fetches).
  - Fail closed with 413/"response too large" semantics, and log domain for reputation tracking.
- References: `pkg/httpclient/federation_client.go`, `pkg/federation/delivery.go` (actor fetch)

### 3) TLS and redirect hardening for federation

- Observed: `AllowInsecureTLS` toggles `InsecureSkipVerify`; default is false. Redirects are validated via `validateFederationURL`.
- Risk: Misconfiguration could weaken TLS checks in prod.
- Recommendation:
  - Enforce `AllowInsecureTLS=false` in production via config validation and CI policy checks.
  - Expand `validateFederationURL` to reject additional internal TLDs and dotless hostnames; log and count rejections by domain.
  - Consider HTTP Signature or instance trust gating on redirects (e.g., disallow cross-scheme upgrade/downgrade during redirects).
- References: `pkg/httpclient/federation_client.go`

### 4) Input validation middleware coverage

- Observed: `ApplyInputValidation` is implemented but not applied in `cmd/api/main.go`. Federation endpoints rely on handler-level checks.
- Risk: Inconsistent validation across inbox/outbox could allow oversized or malformed objects.
- Recommendation:
  - Apply the input validation middleware for federation-oriented services (`inbox`, `outbox`, `actor`, `objects`, `webfinger`) via `ApplySecurityMiddleware(... SecurityTypeFederation)` plus `ApplyInputValidation` where appropriate.
  - Add unit tests for common invalid content types and oversize payloads.
- References: `pkg/middleware/security_application.go`, `cmd/api/main.go`

### 5) Sensitive data scrubbing integration

- Observed: Scrubber and `NewProductionLoggerWithScrubbing()` exist, but the standard Lambda initialization returns a logger instance; integration point is not guaranteed in every service.
- Risk: Inconsistent redaction across Lambdas; accidental credential leakage in logs.
- Recommendation:
  - Ensure the centralized logger used in all Lambdas wraps the Zap core with `ScrubbingCore`.
  - Add a smoke test that logs known secrets and asserts redaction.
- References: `pkg/logging/scrubber.go`, `cmd/*/main.go`

### 6) Rate limiting behavior in serverless context

- Observed: Progressive delay uses `time.Sleep` and in-memory counters per instance; repository-backed checks add persistence.
- Risk: Sleep can prolong Lambda invocations; distributed enforcement may be bursty across warm instances.
- Recommendation:
  - Keep DB-backed checks for hard ceilings; consider token-bucket in DynamoDB for write-heavy endpoints to smooth multi-instance bursts.
  - For read endpoints with high QPS, prefer header-only 429 without `Sleep` when far over limit.
- References: `pkg/middleware/rate_limiter.go`

### 7) ActivityPub security profiles

- Observed: ActivityPub endpoints disable CSP and relax CORP/COEP for federation.
- Risk: Acceptable tradeoff but should be scoped.
- Recommendation:
  - Keep federation profile but confirm it is only applied to `/.well-known`, `inbox`, `outbox`, `users/*` endpoints.
  - Add comment tests to ensure no leakage of permissive headers to web client paths.
- References: `pkg/middleware/security_headers.go`

### 8) Authorized fetch and signature verification

- Observed: Signature service supports caching, retry, and algorithm negotiation; digest verification supports casing compatibility.
- Recommendation:
  - Add circuit-breaker/backoff for failing actor key fetches per domain; tie into reputation metrics.
  - Ensure `Date` skew checks are enforced (documented in spec; ensure max drift, e.g., ±5m).
- References: `pkg/federation/signature_service.go`, `pkg/federation/authorized_fetch.go`

### 9) Storage layer isolation (spot check)

- Observed: Repository pattern with instance-focused models and indices; multi-tenant isolation is a stated goal.
- Recommendation:
  - Codify tenant scoping helpers and require them in repositories (compile-time guardrails). Add tests that attempt cross-tenant reads/writes and assert failure.
  - Consider static analyzers to flag queries missing tenant keys.
- References: `pkg/storage/repositories/*`, `pkg/services/registry.go`

### 10) Dependency hygiene

- Observed: Recent `go` toolchain and pinned dependencies; use of `golang.org/x/*` libs.
- Recommendation:
  - Run `govulncheck` in CI on each PR and release branch; auto-fail on high/critical.
  - Enable Dependabot/Renovate with weekly updates and staged rollouts per service.
  - For crypto/web auth libs, pin minor versions and add regression tests.

## Additional hardening suggestions

- Add GraphQL query depth/complexity limits and max result windowing.
- Cap response sizes for API/GraphQL and federation GET responses; prefer streaming where possible.
- Add request/response sampling to detect anomalous payloads from specific domains; feed reputation.
- Consider certificate pinning for intra-instance callbacks if applicable.

## Implementation checklist

- [ ] Remove `'unsafe-inline'` from web client CSP; use nonce-only scripts/styles.
- [ ] Enforce `MaxResponseSize` with `io.LimitedReader` in federation client and actor/object fetches.
- [ ] Disallow `AllowInsecureTLS` in prod; add config validation and CI check.
- [ ] Apply `ApplyInputValidation` to federation services; add tests for invalid content types and oversize payloads.
- [ ] Wrap global logger with `ScrubbingCore` across all Lambda entries; add redaction smoke test.
- [ ] Add `govulncheck` and dependency update automation to CI.
- [ ] Add GraphQL complexity/depth limits.
- [ ] Add date skew verification for HTTP signatures.

## Notable references (code paths)

- Security headers and CORS: `pkg/middleware/security_headers.go`, `pkg/middleware/cors_configs.go`, `pkg/middleware/security_application.go`
- Rate limiting: `pkg/middleware/rate_limiter.go`
- Federation client: `pkg/httpclient/federation_client.go`
- HTTP signatures & digest: `pkg/federation/httpsig.go`, `pkg/federation/signature_service.go`, `cmd/inbox/main.go`
- Authorized fetch: `pkg/federation/authorized_fetch.go`
- Logging/PII: `pkg/logging/scrubber.go`
- API bootstrap: `cmd/api/main.go`
- Storage & services: `pkg/services/registry.go`, `pkg/storage/repositories/*`




