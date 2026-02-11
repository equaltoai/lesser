# Lesser: CLI Auth (Wallet → OAuth Device Flow) + Automation Safety Rails Roadmap (Spec draft, 2026-02-10)

This roadmap defines how to extend the existing `lesser` CLI so it can authenticate with a wallet (via the web UI)
and interact with Lesser’s API like any other OAuth client, **while enforcing stricter server-side and client-side
rate protection** because CLI access is a high-abuse-risk path.

Core decisions locked for this roadmap:
- **True device-style flow** (no inbound ports required; headless friendly).
- **Wallet handling stays in the web UI**; the CLI never needs to sign wallet challenges.
- **CLI traffic is treated like “LLM agent traffic” for safety rails** (concurrency + circuit-breakers + stricter
  throttles) **regardless of username**.
- **Server-side classification is not spoofable** (no “X-CLI: true” headers); it must be derived from OAuth issuance.

Non-goals (for this roadmap):
- Build a full interactive TUI.
- Replace existing web client auth.
- Make local credential encryption resilient to full local compromise (goal is “headless-safe + reduces accidents”).

---

## Invariants / constraints (do not violate)

- **Email-free / passwordless:** no email-based flows; authentication remains wallet/WebAuthn-first.
- **Minimal user interaction:** web auth is one-time/infrequent; routine CLI usage should be non-interactive.
- **Headless-first:** the default auth flow must work without opening ports or requiring a GUI on the CLI machine.
- **Automation safety rails apply regardless of username:** any token minted for the CLI class must be governed.
- **Rate protection exists on both sides:**
  - client-side: guardrails to prevent accidental/naive abuse
  - server-side: strict throttles + concurrency caps + lockouts
- **Fail-safe security posture:** if classification is missing/unknown, default to stricter behavior for device-flow
  tokens (and/or deny device grant issuance when policy disallows).

---

## Baseline (what Lesser already has)

- OAuth endpoints: `GET /oauth/authorize`, `POST /oauth/consent`, `POST /oauth/token` (auth code + refresh).
- Wallet login endpoints used by the web UI: `/auth/wallet/*` (username required).
- OAuth app registration: `POST /api/v1/apps`.
- LLM-agent safety rails middleware (concurrency + error-rate lockout) currently triggered by `claims.IsAgent`.
- Distributed rate limiting primitives:
  - `pkg/ratelimit.ApplyRateLimit` (Dynamo-backed sliding window; currently keyed by path)
  - `repos.RateLimit().CheckAPIRateLimit(...)` counters/lockouts used by agent rails

---

## Roadmap milestones

Milestones are ordered to keep the system safe at every step. Each milestone includes acceptance criteria and
suggested verification commands (adapt as implementation evolves).

### M0 — Contract + policy decisions (device flow + automation class)

**Goal:** freeze the contract and the server-side classification approach so the CLI + API don’t drift.

**Decisions (locked for M1+)**
- Implement RFC 8628-style device authorization:
  - `POST /oauth/device/code` (issue device_code + user_code)
  - `POST /oauth/token` supports `grant_type=urn:ietf:params:oauth:grant-type:device_code`
- Tokens minted from the device grant are classified as **automation/CLI** (new claim, not `is_agent`).
- Refresh-token exchange preserves the same classification.
- Instance policy/feature flag gates:
  - `AllowDeviceFlow` (default false)
  - `AllowCLIAutomation` (default false, or implied by AllowDeviceFlow)

**Acceptance criteria**
- OpenAPI contract updated (`docs/contracts/openapi.yaml`) with:
  - request/response schemas for device-code issuance and polling
  - documented error codes: `authorization_pending`, `slow_down`, `access_denied`, `expired_token`, `invalid_client`
- A short design note exists (this roadmap is sufficient) documenting:
  - classification mechanism (“derived from grant type and persisted through refresh”)
  - rate-limit posture and why it applies regardless of username

**Suggested verification**
```bash
./lesser verify openapi --strict
./lesser test unit
```

---

### M1 — Server: device authorization sessions (storage + endpoints)

**Goal:** add a true device-style login handshake that works headless without exposing tokens via URLs.

**Implementation notes (recommended)**
- Add a storage model for device auth sessions with TTL (DynamoDB):
  - `device_code` (high entropy, treated as secret) stored hashed or as PK; **never logged**
  - `user_code` (short, human-entered), stored with a lookup index
  - `client_id`, requested `scopes`, `created_at`, `expires_at`, `poll_interval`
  - `status`: `pending|approved|denied|consumed|expired`
  - `approved_username` (set only when approved)
  - polling throttle metadata (`last_poll_at`, `poll_count`) for abuse control
- Add endpoints:
  - `POST /oauth/device/code` → issue codes + verification URLs
  - Extend `POST /oauth/token` for device grant → return OAuth tokens or the correct RFC error
- Add strict rate limits to:
  - device-code issuance (per IP + per client_id)
  - device token polling (per device_code + per IP; enforce `interval` and return `slow_down`)

**Acceptance criteria**
- `POST /oauth/device/code` returns:
  - `device_code`, `user_code`, `verification_uri`, `verification_uri_complete`, `expires_in`, `interval`
- Polling `POST /oauth/token`:
  - returns `authorization_pending` until approved
  - enforces polling interval and uses `slow_down` when violated
  - returns `access_denied` on denial
  - returns `expired_token` after expiry
  - returns access + refresh tokens once approved and marks the session consumed (one-time)
- No secrets are written to logs (device_code, refresh_token, authorization code).

**Suggested verification**
```bash
./lesser test unit
./lesser lint
```

---

### M2 — Web UI: device verification + wallet login + consent

**Goal:** let a user approve a device login from any browser, using existing wallet auth, with minimal friction.

**Implementation notes (recommended)**
- Add an auth UI route like `GET /auth/device`:
  - supports `?user_code=XXXX-XXXX` deep link
  - renders “enter code” if missing; then shows app + scopes and requests consent
- Reuse existing wallet login UX:
  - **requires username** (supports wallets with multiple actors)
  - after login, confirm the account that will authorize the CLI session
- On approval, write `approved_username` + `approved_at` to device session.
- On denial, mark `denied` and optionally record reason/audit event.

**Acceptance criteria**
- A user can complete device flow from a different machine:
  - CLI shows URL + code
  - user authenticates via wallet on the web UI and approves
  - CLI receives tokens via polling
- Consent screen clearly labels:
  - instance domain
  - app name (“lesser cli”)
  - requested scopes
  - the username being authorized

**Suggested verification**
```bash
./lesser test unit
./lesser test integration
```

---

### M2.5 — GraphQL: agent parity + attribution (client blockers)

**Goal:** ensure GraphQL-first clients (and any CLI usage of GraphQL) can correctly discover, filter, and attribute
agent-authored content without falling back to REST.

Tracked by client issues:
- #73 GraphQL agent directory + management parity (delegate/activity/update/admin ops)
- #74 GraphQL timeline filter parity: `excludeAgents` (REST `exclude_agents=true`)
- #75 GraphQL agent attribution on `Object` + `agentAttribution` on `createNote`

**Implementation notes (recommended)**
- Identity/labeling:
  - add `Actor.isAgent: Boolean!` derived from the *account* type/state (not token classification)
  - optionally add `Actor.agentInfo` (or a dedicated `Agent` type) mirroring REST/OpenAPI fields
- Timelines:
  - add `excludeAgents` support for `Query.timeline(...)` (either as a direct arg or a filter input)
- Attribution:
  - expose `Object.agentAttribution` mirroring REST/OpenAPI `AgentPostAttribution`
  - allow `agentAttribution` in `createNote` input (policy-enforced; reject/ignore for non-agent tokens)
- Agent management:
  - add GraphQL equivalents for listing/reading/updating/deleting agents, delegation, activity, and admin policy/ops
- Important: keep separation between **account-level** agent state (`Actor.isAgent`) and **token-level** automation
  classification (`client_class=cli`). CLI tokens must not imply `isAgent`.

**Acceptance criteria**
- GraphQL timelines can exclude agent-authored content when requested (same semantics as REST).
- GraphQL object/timeline queries return agent attribution when present.
- GraphQL status creation can include agent attribution (subject to policy).
- Agent directory + management operations are available via GraphQL (owner + admin parity with REST).

**Suggested verification**
```bash
./lesser test unit
./lesser verify graphql-coverage --strict
```

---

### M3 — Token classification: `client_class=cli` (persisted across refresh)

**Goal:** make CLI traffic enforceable server-side without spoofable headers and without depending on the account type.

**Implementation notes (recommended)**
- Add a new JWT claim (example): `client_class` with values like `web|cli|agent`.
  - Device-grant tokens must be minted with `client_class=cli`.
  - Do **not** set `is_agent=true` for CLI tokens (avoids agent-only content restrictions).
- Ensure the classification persists through refresh:
  - store `client_class` (or equivalent) on refresh token records, or
  - store `client_class` on the OAuth client record and derive it during refresh issuance.
- Ensure CLI tokens have a stable session identifier (`sid`) for concurrency enforcement:
  - set `sid` for `client_class=cli` tokens (and ensure refresh preserves it, or uses a stable family/session id).

**Acceptance criteria**
- Access tokens minted for device flow contain `client_class=cli`.
- Refreshing a CLI token yields a new access token still containing `client_class=cli`.
- Concurrency enforcement has a stable identifier to key on (not path, not IP-only).

**Suggested verification**
```bash
./lesser test unit
./lesser test race
```

---

### M4 — Server: automation safety rails + strict throttles for CLI tokens

**Goal:** apply “agent-like” safety rails and strict rate limits to CLI tokens **regardless of username**, to reduce
abuse impact even if credentials leak or the CLI is scripted aggressively.

**Implementation notes (recommended)**
- Extend/rename the existing middleware so it can be triggered by:
  - `claims.IsAgent == true` **OR**
  - `claims.ClientClass == "cli"` (or equivalent)
- Enforce (initial suggested defaults; should be configurable):
  - **max concurrent in-flight requests per session (`sid`):** 2
  - **strict request throttles (per `sid`):**
    - **burst:** 20 requests / 10s
    - **sustained:** 60 requests / 1m
  - **error-rate circuit breaker:** >10% 4xx/5xx in 1 minute (min 10 requests) → 1 hour lockout
  - **GraphQL depth limit:** 3 for `client_class=cli` (same as agents), without setting `is_agent=true`
- Important: keep **agent-account** restrictions keyed to account type (`user.IsAgent`) and keep **automation-client**
  restrictions keyed to token classification (`client_class=cli`). Do **not** set `is_agent=true` for CLI tokens, or
  human accounts using the CLI will trip agent-only rails (and GraphQL depth is currently derived from `is_agent`).
- Ensure rate-limit keys have stable cardinality:
  - do not key by full path; use route classes (e.g., `read`, `write`, `search`, `graphql`) or “api:all”
- Add clear 429 semantics:
  - `retry-after`
  - `x-ratelimit-*` headers
- Add an operator escape hatch:
  - instance config to tune limits
  - ability to disable device flow entirely

**Acceptance criteria**
- Any request with a `client_class=cli` access token:
  - is subject to strict throttles and concurrency caps
  - can be locked out by circuit breakers
  - returns consistent 429 responses with `retry-after`
- Safety rails apply even for human accounts using the CLI.
- Limits are configurable at the instance level without code changes.

**Suggested verification**
```bash
./lesser test unit
./lesser test integration
./lesser lint
```

---

### M5 — CLI: `lesser auth` (device login + encrypted session store)

**Goal:** add first-class CLI authentication while keeping the CLI simple, headless-friendly, and safe-by-default.

**Command UX (recommended)**
- `lesser auth login --base-url <https://instance>`:
  - calls `POST /api/v1/apps` (if needed) to obtain `client_id` (and store it)
  - calls `POST /oauth/device/code`
  - prints `verification_uri_complete` (and `user_code` as fallback)
  - polls until success/deny/timeout
  - stores an encrypted session blob locally
- `lesser auth status` / `lesser auth whoami`
- `lesser auth logout`:
  - deletes local session
  - (optional, later milestone) calls token revocation endpoint
- Non-interactive/multi-step mode for scripts:
  - `lesser auth device start --json`
  - `lesser auth device poll --device-code ...` (or reads from a temp file)

**Local encryption (recommended)**
- Store a single JSON session blob (client_id, refresh_token, base_url, username, scopes, timestamps).
- Encrypt with AES-256-GCM.
- Key derivation:
  - default: machine-derived secret (hostname + user + `/etc/machine-id` + homedir + base_url) → SHA-256 → 32 bytes
  - override for portability/CI: `LESSER_AUTH_SECRET` (or `--secret-file`)
- File layout and permissions:
  - `~/.lesser/auth/<base_url_hash>/session.enc` (dir `0700`, file `0600`)

**Acceptance criteria**
- A headless user can authenticate by copying a URL/code into any browser.
- Refresh tokens are never stored in plaintext.
- The CLI automatically refreshes access tokens and survives token expiry.
- The CLI never prints tokens by default; debug logging redacts secrets.

**Suggested verification**
```bash
./lesser test unit
./lesser lint
```

---

### M6 — CLI: API client + client-side rate protection

**Goal:** make “CLI behaves like any other client” true in practice while preventing accidental abuse.

**Implementation notes (recommended)**
- Add an HTTP client layer used by all CLI API commands:
  - reads encrypted session
  - refreshes tokens automatically
  - retries with backoff on transient failures
  - respects `retry-after` on 429
- Client-side throttles (defaults; configurable):
  - max concurrency: 2
  - max requests/sec: conservative token bucket
  - optional “cost” weighting (search/graphql/write heavier)
- Provide a minimal “API passthrough” command:
  - `lesser api request --method GET --path /api/v1/accounts/verify_credentials`
  - plus a small set of ergonomic shortcuts over time (statuses, timelines, etc.)

**Acceptance criteria**
- With a valid session, `lesser api request ...` works against protected endpoints.
- The CLI never exceeds its configured concurrency/RPS even under parallel command usage.
- `429` handling does not busy-loop; it sleeps at least `retry-after`.

**Suggested verification**
```bash
./lesser test unit
./lesser test race
```

---

### M7 — E2E tests, observability, docs, rollout

**Goal:** make this safe to ship and easy to operate.

**Implementation notes (recommended)**
- Tests:
  - unit tests for device-flow state machine and error mapping
  - unit tests for encrypted session store (permissions, encrypt/decrypt, corruption handling)
  - integration test for “device start → approve → token → refresh”
- Observability:
  - metrics for CLI issuance, polling volume, approvals/denials, throttles, lockouts
  - structured logs include `client_class`, `client_id`, and rate-limit decisions (but never secrets)
- Docs:
  - `docs/cli/auth.md` (headless flow, secret override, troubleshooting)
  - operator doc: how to tune CLI limits and how lockouts work
- Rollout:
  - feature flags default to disabled
  - staged enablement plan (dev → staging → live)

**Acceptance criteria**
- `./lesser verify ci` passes.
- Operators can enable/disable device flow and tune CLI limits via config.
- Docs cover headless and non-interactive flows clearly.

**Suggested verification**
```bash
./lesser verify ci
./lesser test integration
```

---

## Optional follow-ons (post-M7)

- **Loopback callback flow** as a convenience for laptop users (still classified as `client_class=cli`).
- **Token revocation endpoint** (RFC 7009) for server-side logout and incident response.
- **WebSocket/SSE “push” for device completion** to reduce polling load (keep polling as the baseline).
- **Keyring integration** when available (must remain headless-safe).
