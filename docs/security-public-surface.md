# Public Surface Matrix (Living)

Date: 2026-02-07  
Repo: `lesser`  

This document defines Lesser’s intended **unauthenticated/public surface**.

Policy:
- **Default-deny:** an endpoint/resolver is **not** considered public unless explicitly listed here.
- **Public content** must be consistent with ActivityPub visibility models (`public` / `unlisted`) and must not leak
  private metadata.
- **GraphQL is non-public by default** (auth required for all operations).

Related:
- Gaps inventory: `docs/security-gaps.md`
- Remediation plan: `docs/security-remediation-roadmap.md`
- MCP auth contract: `docs/specs/mcp-actor-url-auth-contract.md`

Legend:
- **Public:** no auth required (but may still enforce content visibility)
- **Auth:** requires valid user auth
- **Mod/Admin:** requires moderator/admin role
- **Internal:** should only be reachable by infrastructure / trusted callers (not the open internet)

---

## ActivityPub + discovery

- **Public** `GET /.well-known/webfinger` (service: `cmd/webfinger`)
- **Public** `GET /.well-known/nodeinfo` (service: `cmd/api`)
- **Public** `GET /.well-known/lesser-soul-agent` (service: `cmd/api`)
- **Public** `GET /.well-known/mcp.json` (service: `lesser-body` MCP Lambda)
- **Public** `GET /.well-known/oauth-protected-resource/mcp/{actor}` (service: `lesser-body` MCP Lambda)
- **Public** `GET /nodeinfo/2.0` (service: `cmd/api`)
- **Public** `GET /users/:username` (service: `cmd/actor`)
  - Content negotiation:
    - ActivityPub actor JSON (e.g., `application/activity+json`) is public
    - HTML profile page (`text/html`) is public but must be safe-by-construction (escape/template + CSP)
- **Public** `GET /objects/:id` (service: `cmd/objects`)
  - Content negotiation:
    - ActivityPub object JSON is public *only if the object is public/unlisted*
    - HTML object view is public *only if the object is public/unlisted* and must be safe-by-construction

Notes:
- “Authorized fetch” (private object delivery to authenticated remote servers) is handled by ActivityPub-level
  authentication and is tracked separately from this Mastodon/GraphQL policy.

---

## MCP (`lesser-body`)

- **Auth** `GET/POST/DELETE /mcp/{actor}` (canonical remote MCP endpoint; actor-scoped form)
- **Auth** `GET/POST/DELETE /mcp` (legacy compatibility path; not the canonical actor-scoped form for new remote clients)

---

## Mastodon-compatible REST API (`cmd/api`)

### Public (pre-auth + discovery + public content)

- **Public** `POST /api/v1/apps` (OAuth app registration)
- **Public** OAuth entrypoints: `GET /oauth/authorize`, `POST /oauth/token`, `POST /oauth/consent`
- **Public** Wallet auth:
  - `POST /auth/wallet/challenge`
  - `POST /auth/wallet/verify`
  - `POST /auth/wallet/login`
  - `POST /auth/wallet/link` (registration flow only; requires wallet proof; must not allow unsigned linking)
- **Public** WebAuthn login:
  - `POST /api/v1/auth/webauthn/login/begin`
  - `POST /api/v1/auth/webauthn/login/finish`
- **Public** Registration (Auth UI only; wallet/WebAuthn only):
  - `POST /api/v1/accounts`
    - Requires a verified wallet/WebAuthn proof (e.g., a verified wallet challenge) and must not allow bare username-only signup.
- **Public** Instance + trends:
  - `GET /api/v1/instance` + related `GET /api/v1/instance/*`
  - `GET /api/v2/instance`
  - `GET /api/v1/trends*`, `GET /api/v2/trends*`
- **Public** Public timelines:
  - `GET /api/v1/timelines/public`
  - `GET /api/v1/timelines/tag/{hashtag}`
- **Public** Public embeds:
  - `GET /api/oembed`
  - `GET /embed/{id}`
- **Public** Public status reads (must enforce visibility; unauth must see only public/unlisted):
  - `GET /api/v1/statuses/{id}`
  - `GET /api/v1/statuses/{id}/context`
  - `GET /api/v1/statuses/{id}/history`
  - `GET /api/v1/statuses/{id}/quotes`
  - `GET /api/v1/accounts/{id}/statuses`
- **Public** lesser.host trust proxy (no auth required):
  - `GET /api/v1/trust/jwks.json`
  - `GET /api/v1/trust/attestations`
  - `GET /api/v1/trust/attestations/{id}`
- **Public** Community notes reads (must not leak private objects):
  - `GET /api/v1/notes/{object_id}`
  - `GET /api/v1/accounts/{id}/notes`

### Auth (user-scoped)

- **Auth** account management: `GET /api/v1/accounts/verify_credentials`, `PATCH /api/v1/accounts/update_credentials`
- **Auth** WebAuthn credential management:
  - `POST /api/v1/auth/webauthn/register/begin`
  - `POST /api/v1/auth/webauthn/register/finish`
  - `GET/PUT/DELETE /api/v1/auth/webauthn/credentials*`
- **Auth** wallet management:
  - `GET /auth/wallet/list`
  - `DELETE /auth/wallet/unlink/{address}`
- **Auth** private timelines and user data:
  - `GET /api/v1/timelines/home`, `GET /api/v1/timelines/direct`, `GET /api/v1/timelines/list/{list_id}`
  - bookmarks/favourites, notifications, preferences, markers, push subscriptions
  - conversations, scheduled statuses, follow requests, lists, domain blocks, exports/imports
- **Auth** sensitive status endpoints:
  - `GET /api/v1/statuses/{id}/source` (author-only)
  - `GET /api/v1/statuses/{id}/favourited_by`, `GET /api/v1/statuses/{id}/reblogged_by`
  - all status write/interaction endpoints (`POST/PUT/DELETE /api/v1/statuses*`, favourite/reblog/etc.)

### Mod/Admin

- **Mod/Admin** moderation endpoints (`/api/v1/moderation/*`, `/api/v1/admin/moderation/*`)
- **Admin** admin endpoints (`/api/v1/admin/*`)

### Internal/bootstrap

- **Internal** setup bootstrap endpoints (`/setup/*`) once an instance is activated (still reachable for locked instances
  during bootstrap; should not be broadly internet-exposed post-activation).

---

## GraphQL (`cmd/graphql`)

- **Auth** `POST/GET /graphql`, `POST/GET /api/graphql`, `GET /subscriptions`
  - No GraphQL operations are intended to be publicly callable.
- **Mod/Admin** moderation queries/mutations/subscriptions (role-gated in resolvers; see Milestone 5)
- **Admin** ops/insights + operator controls (cost/perf, federation management, AI analysis/debug)
- **Public/Internal** health endpoints:
  - `GET /health`, `GET /ready`
  - `GET /playground` is dev-only (requires `EnablePlayground`)

---

## Streaming SSE (`cmd/sse`)

- **Auth** `GET /api/v1/streaming/*` (all SSE streaming endpoints currently require auth)
  - `GET /api/v1/streaming/list` additionally enforces list ownership/membership (Milestone 6).

---

## Streaming WebSocket (`cmd/streaming`)

- **Public** WebSocket connect is allowed without auth so clients can subscribe to public streams (e.g. `public*`,
  `hashtag:*`).
- **Auth** user-scoped streams:
  - `user`, `user:notification`, `direct`
  - `list:<id>` (requires list ownership/membership; Milestone 6)
  - Canonical forms (`user:<username>`, `user:notification:<username>`, `direct:<username>`) are restricted to the
    authenticated user’s own username.
