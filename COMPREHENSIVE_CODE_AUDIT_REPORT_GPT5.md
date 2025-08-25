## Comprehensive Code Audit Report — lesser (pre-release)

### Scope and methodology
- Reviewed federation (HTTP Signatures, authorized fetch), HTTP client/transport, auth/session/CSRF, security headers/CORS, input sanitization, rate limiting/backpressure, and secrets/config handling across `cmd/*`, `pkg/*`.
- Focused on code quality, consistency, and security posture. Highlighted high-impact, low-risk improvements.

### Ratings
- **Quality: 8/10**
  - Modular design, good separation (`pkg/federation`, `pkg/auth`, `pkg/middleware`, `pkg/httpclient`).
  - Structured logging and observability integrated.
  - Strong test scaffolding in `cmd/api` and `pkg/testing`.
- **Consistency: 7/10**
  - Mostly consistent naming and structure; some duplication/drift between multiple implementations of similar concerns (security headers/CORS across locations).
- **Security: 8/10**
  - Solid foundations: robust HTTP Signatures, digest/clock-skew checks, SSRF-safe HTTP client with DNS caching and private-IP blocking, CSRF protections, cookie security defaults, rate limiting for API and federation, strong security headers and conditional HSTS.

### Strengths observed
- **Federation verification**
  - Supports `hs2019`, RSA/ECDSA/Ed25519; enhanced verification path; digest enforcement and timestamp skew checking.
  - Public key fetching with cache/retry (`SignatureService`), and digest compatibility (`sha-256` lower-case) in inbox path.

- **HTTP client SSRF protection** (`pkg/httpclient`)
  - Scheme validation, metadata endpoint blocking, private IP/ranges and loopback blocked, redirect limiting, DNS cache with TOCTOU hostname guard.

- **Auth/session/cookies/CSRF**
  - JWT with explicit signing method checks; short-lived tokens; timing-safe helpers; secure cookie defaults (Secure, HttpOnly, SameSite=Strict); single-use CSRF tokens with storage-backed validation.

- **Input sanitization**
  - `bluemonday` policies tailored for ActivityPub; escaping for non-HTML string fields; media type whitelisting.

- **Rate limiting and abuse controls**
  - API and federation-specific middlewares with Dynamo-backed tracking, blocking with `Retry-After`, and standard rate-limit headers.
  - Streaming backpressure with token-bucket and load-aware shutdown manager.

- **Security headers and HSTS**
  - Comprehensive security header middleware with CSP nonce support; API middleware sets CSP, HSTS when over HTTPS; removal of server identification headers.

### Notable risks and inconsistencies
- **HTTP transport hardening gaps**
  - Default `http.Transport` is used; not all clients set explicit TLS minimums/handshake and response header timeouts, or connection caps. This is a defense-in-depth gap.

- **Sensitive header logging**
  - Some error paths log full `Signature`/`Digest` values and PEM contents. Prefer redaction (log length/hash) to reduce leakage risk.

- **Embedding policy on oEmbed route**
  - `X-Frame-Options: ALLOWALL` on the embed endpoint is intentional for embedding but is legacy and broad. Prefer route-local CSP `frame-ancestors *` and omit the XFO header (modern browsers follow CSP) to avoid accidental propagation.

- **Authorized fetch bootstrap**
  - First-time actor fetch is unsigned (practical reality). Consider caching/preloading known actors, stronger throttling for unknown actors, and tighter rate limiting on actor/doc fetch to mitigate TOFU risks.

- **Headers/CORS duplication**
  - Multiple implementations (global API middleware vs. reusable `pkg/middleware/security_headers.go` and enhanced CORS) can drift over time. Centralization would improve consistency.

- **Config defaults**
  - A dev-only JWT secret default appears in services registry; ensure non-test stages fail fast if defaults are present.

- **Sanitization coverage for embeds**
  - Ensure content rendered into embed HTML is sanitized/escaped consistently, even if upstream sanitization already occurs.

### Prioritized recommendations
1) **Harden HTTP transports (high)**
   - In `pkg/httpclient`, wrap `http.Transport` with:
     - `TLSClientConfig` with `MinVersion = tls.VersionTLS12` (or 1.3 where possible).
     - `TLSHandshakeTimeout`, `ResponseHeaderTimeout`, `ExpectContinueTimeout`.
     - Connection caps: `MaxConnsPerHost`, `MaxIdleConns`, `MaxIdleConnsPerHost`.
     - Consider `DialContext` verification to ensure remote addr matches pre-resolved IPs (bind resolution to connection).

2) **Redact sensitive data in logs (high)**
   - Never log full `Signature`, `Digest`, JWTs, or PEMs. Log an identifier, hash (e.g., SHA-256 of the value), or length.

3) **Embed route framing policy (medium)**
   - Replace `X-Frame-Options: ALLOWALL` with CSP `frame-ancestors *` scoped to the embed route and omit XFO to prefer CSP.

4) **Centralize security headers (medium)**
   - Adopt `EnhancedSecurityHeaders` across routes: use `ActivityPubSecurityHeaders` for federation endpoints, `APISecurityHeaders` for API, and a custom config for `oembed`.

5) **Authorized fetch hardening (medium)**
   - Cache/pin known actor keys; throttle unknown actor fetch; enforce HTTPS-only for actor/doc retrieval; add stricter federation rate limits for first-time keys.

6) **Config enforcement (medium)**
   - Fail startup if a default placeholder secret is detected in non-test stages; document rotation procedures.

7) **Security test additions (nice-to-have)**
   - Unit tests for: logging redaction, sanitizer coverage on embed content, transport timeouts present, and digest/clock-skew negative paths.

### Expected impact
- These changes reduce attack surface (transport and logging), improve policy correctness (framing), and increase consistency across services. They are low-risk and largely internal changes.

### References (non-exhaustive)
- Federation: `pkg/federation/httpsig.go`, `httpsig_enhanced.go`, `signature_service.go`, `cmd/inbox/main.go` (verification/digest paths)
- HTTP client: `pkg/httpclient/client.go`
- Headers/CORS: `pkg/middleware/security_headers.go`, `pkg/middleware/cors_enhanced.go`, `cmd/api/middleware.go`
- Auth/CSRF: `pkg/auth/*.go`, cookies in `pkg/common/cookies.go`
- Sanitization: `pkg/common/sanitize.go`, `pkg/activitypub/validation.go`
- Rate limiting: `pkg/ratelimit/middleware.go`, streaming backpressure in `pkg/streaming/*`


