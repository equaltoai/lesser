# Lesser: Crawler Rate Limiting Roadmap (Spec draft, 2026-02-03)

This roadmap translates `.pai/specs/crawler-rate-limiting.md` into an implementable plan for Lesser’s **multi-Lambda**
HTTP architecture (AppTheory runtime + API Gateway v2 routing via `infra/cdk/inventory/lambdas.go`).

The spec focuses on “Layer 3” (Lambda middleware). In Lesser, that means: shared middleware in `pkg/` plus explicit
middleware registration in each HTTP Lambda’s `buildApp(...)` (or equivalent) without disrupting ActivityPub
federation.

## Invariants / constraints (do not violate)

- **Federation-first:** ActivityPub federation must continue to work normally (actor fetches, inbox delivery, outbox
  reads, collections reads, WebFinger, NodeInfo). Any crawler controls must be designed to avoid federation false
  positives.
- **Low-cost fast path:** blocked/limited crawler requests should return early **before** DynamoDB-heavy work (actor
  lookups, signature verification, auth services, etc.) to reduce DynamoDB read costs and Lambda duration.
- **Rate-limit key cardinality:** crawler limits must not key by full path (e.g., `/users/alice`) or they will be
  trivially bypassed and/or generate excessive DynamoDB keys. Use stable keys (category + IP/domain + coarse route
  class).
- **Safe rollout:** start with **observe-only** (log + metrics) before enforcement. Provide an operator “escape hatch”
  for debugging federation.
- **CI stays green after every milestone:** each milestone explicitly requires `./lesser verify ci` to pass.

## Baseline: what Lesser already has (use as building blocks)

- **HTTP Lambdas split by route:**
  - `cmd/actor` → `GET /users/{username}`
  - `cmd/objects` → `GET /objects/{id}`
  - `cmd/collections` → `GET /users/{username}/(followers|following|liked)`
  - `cmd/webfinger` → `GET /.well-known/webfinger`
  - `cmd/api` → `ANY /api/v1/*`, `ANY /api/v2/*`, `GET /.well-known/nodeinfo`
  - `cmd/inbox` / `cmd/outbox` → federation inbox/outbox routes
  - Canonical source of route wiring: `infra/cdk/inventory/lambdas.go`
- **Robots control is currently incomplete:** multiple Lambdas set `x-robots-tag: noindex, nofollow` (e.g.
  `cmd/api/middleware.go`), but there is **no** `/robots.txt` route in the inventory.
- **Existing distributed rate limiting:** `pkg/ratelimit.ApplyRateLimit` provides DynamoDB-backed sliding window
  limiting (fail-open), but its default keying uses `ctx.Request.Path` which is not sufficient for crawler-wide limits.
- **Observability primitives exist:** EMF metrics collectors and middleware exist (e.g. `pkg/observability/*` and
  `cmd/api` / `cmd/inbox` integrations), suitable for crawler category counters.

## Roadmap milestones

Milestones are ordered and intended to be implemented sequentially. Each milestone includes acceptance criteria and
verification commands; the verification lists are intentionally conservative and always include `./lesser verify ci`.

---

### M0 — Architecture alignment + decisions (spec → Lesser)

**Goal:** make the “Layer 3” spec concrete in the context of Lesser’s AppTheory + per-route Lambda design.

**Decisions (locked for M1–M3)**
- `GET /robots.txt` is served by the existing `cmd/api` Lambda:
  - route registered in `cmd/api/routes.go`
  - route declared in `infra/cdk/inventory/lambdas.go` under the `api` Lambda
- Shared implementation lives in `pkg/crawler` and exposes:
  - pure classification helpers (no AWS/DB deps)
  - an AppTheory middleware (`func Middleware(...) apptheory.Middleware`)
  - `robots.txt` response helper/handler for AppTheory
- Classification priority rules:
  - explicit AI crawler UA blocklist is **never** overridden by `Accept: application/activity+json`
  - federation detection uses `Accept` + path heuristics as a signal, but must not become a bypass mechanism
- Rate limiting key strategy:
  - use stable “route classes” + stable identifiers (category + IP/domain/engine)
  - never key by full path or per-resource identifiers (e.g. `/users/alice`)

**Acceptance criteria**
- A single “source of truth” decision record exists (this roadmap + a short design note is sufficient) describing the
  items above and listing the HTTP Lambdas to be updated.
- `/robots.txt` routing approach is chosen and scoped (existing Lambda vs new Lambda).
- `./lesser verify ci` is green.

**Suggested verification**
```bash
./lesser verify ci
```

---

### M1 — `/robots.txt` endpoint (static + aggressively cached)

**Goal:** publish a standards-compatible robots policy that disallows known AI training crawlers and sets conservative
defaults, while explicitly allowing common search engines at low rates.

**Implementation notes**
- Add `pkg/crawler/robots.go` (or equivalent) containing the `RobotsTxt` constant and a handler for AppTheory.
- Wire `GET /robots.txt` in:
  - Lambda code: chosen `cmd/*` `buildApp`/route registration
  - Infra: `infra/cdk/inventory/lambdas.go` route inventory
- Return headers:
  - `content-type: text/plain; charset=utf-8`
  - `cache-control: public, max-age=86400` (24h)
  - `x-robots-tag: noindex` (optional; robots.txt itself doesn’t need indexing)

**Acceptance criteria**
- `GET /robots.txt` returns the expected content and headers.
- The robots policy explicitly `Disallow`s known AI crawlers (from the spec list) and does not block federation
endpoints accidentally via overly broad patterns.
- `./lesser verify ci` is green.

**Suggested verification**
```bash
./lesser verify ci
```

---

### M2 — Crawler classifier (pure library + tests)

**Goal:** implement a deterministic request classifier that maps requests into a small set of categories suitable for
rate limiting/blocking decisions.

**Recommended categories**
- `Human` (default)
- `Federation` (ActivityPub peers)
- `SearchEngine` (Google/Bing/DDG/etc.)
- `AICrawler` (explicit UA list; candidate for block)
- `GenericBot` (unknown bot signatures; candidate for heavy limit)
- `Suspicious` (empty UA, common scraping clients; candidate for block or very heavy limit)

**Implementation notes**
- Add `pkg/crawler/classifier.go` that consumes only strings (UA, Accept, Path) and returns `(category, reason)`.
- Include an explicit “known federation software” allowlist (UA patterns) and a small “allowed integration bots” list
  (Slack/Discord previews) to reduce false positives.
- Unit tests should cover:
  - federation UA + `Accept: application/activity+json` → `Federation`
  - federation endpoints by path (e.g. `/users/*/inbox`, `/.well-known/webfinger`) → `Federation` (unless explicitly
    blocked by AI UA)
  - GPTBot/Meta/Anthropic/etc. → `AICrawler` even if `Accept` indicates ActivityPub
  - common browser UAs → `Human`
  - empty/very short UA → `Suspicious`

**Acceptance criteria**
- Classifier logic is pure (no AWS, no DB) and is unit tested with a clear table of cases.
- The classifier cannot be trivially bypassed by setting `Accept: application/activity+json` when the UA matches the AI
  crawler blocklist.
- `./lesser verify ci` is green.

**Suggested verification**
```bash
./lesser verify ci
```

---

### M3 — Middleware integration (observe-only mode)

**Goal:** roll out classification everywhere without changing behavior yet (log + metrics only).

**Implementation notes**
- Add `pkg/crawler/middleware.go` as an AppTheory middleware that:
  - extracts `User-Agent`, `Accept`, `X-Forwarded-For`, and path
  - classifies the request
  - emits structured logs (category + reason) and lightweight EMF counters (optional)
  - does **not** block/limit in this milestone
- Register the middleware early in each HTTP Lambda’s chain:
  - `cmd/api` (before auth/cost work; after timeout)
  - `cmd/actor`, `cmd/objects`, `cmd/collections`, `cmd/webfinger`
  - `cmd/inbox` / `cmd/outbox` only if federation-safe classification is already proven (otherwise defer to M4+)
- Add a config flag/mode (e.g. `CRAWLER_PROTECTION_MODE=observe`) with a safe default (off/observe).

**Acceptance criteria**
- Middleware is present and enabled in observe-only mode for the selected HTTP Lambdas.
- No route behavior changes (no new 403/429 responses) when in observe-only mode.
- Metrics/logs allow operators to answer: “which UA/IPs are driving traffic and how would they be classified?”
- `./lesser verify ci` is green.

**Suggested verification**
```bash
./lesser verify ci
```

---

### M4 — Soft enforcement (rate limit SearchEngine + GenericBot)

**Goal:** start reducing cost from high-volume crawlers without risking federation disruption.

**Implementation notes**
- Implement category-based rate limits for:
  - `SearchEngine`: very low sustained rate + small burst
  - `GenericBot`: very low sustained rate + tiny burst
- Keep `AICrawler` in observe-only (log-only) until false positives are understood.
- Build a crawler-specific limiter that avoids per-path keys (either by extending `pkg/ratelimit` to accept a custom
  `RateLimitKey`, or by using the underlying limiter directly from `pkg/crawler`).
- Response requirements:
  - `429 Too Many Requests` for exceeded limits
  - `retry-after` + `x-ratelimit-*` headers

**Acceptance criteria**
- Search engines and generic bots are rate limited with stable keys (category + IP/engine identity + route class).
- Federation-like requests are not rate limited (or have a much higher limit) and remain functional.
- Rate limit failures are fail-open for federation safety (but must log so operators can detect limiter outages).
- `./lesser verify ci` is green.

**Suggested verification**
```bash
./lesser verify ci
```

---

### M5 — Hard enforcement (block AI crawlers + suspicious clients)

**Goal:** block known AI training crawlers and the most suspicious scraping patterns while preserving federation.

**Implementation notes**
- Enforce `AICrawler` as a hard block:
  - `403 Forbidden` with a plain-text explanation referencing `robots.txt`
  - no DynamoDB calls on the blocked path
- Decide the posture for `Suspicious`:
  - block outright (403) **or** extremely low rate limit (429) depending on observed false positives
- Add an operator “escape hatch” for federation debugging (examples):
  - a short-lived allowlist keyed by IP/CIDR in config
  - an admin-only header/token bypass on non-federation endpoints

**Acceptance criteria**
- Requests with AI crawler UAs are blocked quickly and consistently, including when they set ActivityPub-ish `Accept`
  headers.
- Legitimate federation traffic is not blocked (verify with representative Mastodon/Pleroma UA cases + inbox/outbox
  paths).
- Operators have a documented emergency bypass to resolve false positives quickly.
- `./lesser verify ci` is green.

**Suggested verification**
```bash
./lesser verify ci
```

---

### M6 — Operationalization (metrics, alarms, runbooks, tuning)

**Goal:** make crawler protection observable, tuneable, and safe to operate.

**Implementation notes**
- Emit EMF metrics (by category and route class), e.g.:
  - `crawler.blocked.count`
  - `crawler.rate_limited.count`
  - `crawler.allowed.federation.count`
- Add dashboards/alarms (CDK + `pkg/observability` patterns) for:
  - spikes in blocked AI crawlers
  - spikes in rate-limited bots
  - suspicious drops in federation traffic
- Add a short ops runbook section (recommended location: `docs/operations/` or `docs/monitoring.md`) describing:
  - how to toggle modes (off/observe/limit/block)
  - how to add/remove UA patterns safely
  - how to use the escape hatch

**Acceptance criteria**
- Operators can detect crawler pressure and confirm enforcement is working via dashboards and logs.
- Limits and blocklists are configurable without code changes (env/config) and are validated at startup.
- `./lesser verify ci` is green.

**Suggested verification**
```bash
./lesser verify ci
```

---

### M7 — Optional: edge defenses (CloudFront/WAF)

**Goal:** reduce Lambda invocations by stopping abusive crawlers at the edge (outside the scope of the Layer 3 spec,
but aligned with cost goals).

**Implementation notes**
- Add AWS WAF rate-based rules (and/or known bad UA/IP rules) at CloudFront or API Gateway.
- Consider geo/IP reputation blocks where appropriate.
- Ensure WAF rules do not block federation partner networks; use allowlists carefully.

**Acceptance criteria**
- Documented WAF rules exist with a rollback plan and metrics proving reduced Lambda invocations under crawler load.
- `./lesser verify ci` is green.

**Suggested verification**
```bash
./lesser verify ci
```
