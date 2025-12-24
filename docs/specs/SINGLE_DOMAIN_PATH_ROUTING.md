# Single-Domain Path Routing (`/l` + `/auth`)

> **Status**: Draft  
> **Created**: 2025-12-24  
> **Related**: `docs/specs/LESSER_UP_CLI.md`, `docs/specs/AUTH_SETUP_WIZARD_UI.md`

## Summary

Move Lesser stage deployments to a **single primary domain per stage** (dev/staging/live), using **path-based routing** for UIs:

- Client/content UI at `/<domain>/l/*`
- Auth UI (login/consent/setup) at `/<domain>/auth/*`
- All system endpoints remain at top-level paths on the same origin (Mastodon + ActivityPub + OAuth + setup).

This eliminates the need for cross-subdomain “JWT in URL” bridging, removes CloudFront path allowlist footguns for new system endpoints (like `/setup/*`), and keeps ActivityPub/Mastodon paths stable.

## Goals

- Single origin per stage for all system endpoints: `https://<stage-domain>`
- UI isolation via path prefixes:
  - `https://<stage-domain>/l/*` (client app)
  - `https://<stage-domain>/auth/*` (auth UI + setup wizard)
- Keep WebSockets standardized on `wss://ws.<stage-domain>` (unchanged for now).
- Avoid token-in-URL patterns for OAuth bridging.
- Make adding new system endpoints safe by default (no CDN allowlist updates required).

## Non-goals (for this change)

- Replacing the WebSocket subdomain with a path-based WS endpoint.
- Implementing the auth setup wizard UI itself (separate effort; see `docs/specs/AUTH_SETUP_WIZARD_UI.md`).
- Removing legacy subdomains immediately (optional compatibility period).

## URL Layout

### System (API, federation, OAuth, setup)

All served from the stage apex domain:

- `/.well-known/*`
- `/nodeinfo/*`
- `/users/*`, `/inbox/*`, `/objects/*`, etc.
- `/api/v1/*`, `/api/v2/*`
- `/oauth/*`
- `/setup/*`
- `/auth/wallet/*` (wallet endpoints remain API-owned even though they share the `/auth` prefix)

### Client UI

- Base: `/<domain>/l`
- SPA routes: `/<domain>/l/*`

### Auth UI

- Base: `/<domain>/auth`
- SPA routes: `/<domain>/auth/*`
- Setup wizard entry: `/<domain>/auth/setup`

## CDN Design (CloudFront)

One CloudFront distribution on `https://<stage-domain>` with multiple origins:

- **Origin A (system):** API Gateway custom domain for the stage (or regional invoke domain)
- **Origin B (client UI):** S3 bucket containing client build output
- **Origin C (auth UI):** S3 bucket containing auth-ui build output

### Behaviors (ordered)

1. `/auth/wallet/*` → Origin A (system)
2. `/auth/*` → Origin C (auth UI)
3. `/l/*` → Origin B (client UI)
4. `default (*)` → Origin A (system)

### Viewer-request rewrite (CloudFront Function)

Add a CloudFront Function to support SPAs under subpaths without rewriting system 404s:

- For requests starting with `/l`:
  - Strip `/l` prefix before fetching from the client origin
  - If the path has no file extension, rewrite to `/index.html`
- For requests starting with `/auth`:
  - Strip `/auth` prefix before fetching from the auth origin
  - If the path has no file extension, rewrite to `/index.html`

**Important:** do not apply SPA fallback globally; only under `/l/*` and `/auth/*`.

## Certificates & DNS

- Stage apex uses a CloudFront certificate for `<stage-domain>` and `*.<stage-domain>` (supports `ws.<stage-domain>` and `media.<stage-domain>` while leaving room for future subdomains).
- Route53 `A/AAAA` for `<stage-domain>` points to the CloudFront distribution.
- Keep `ws.<stage-domain>` (API Gateway v2 domain) and `media.<stage-domain>` (media delivery) as subdomains.

## Backend Changes (OAuth + Redirects)

### Redirect target changes

Update all redirects that currently target `https://auth.<stage-domain>/…` to the path-based auth UI:

- `https://<stage-domain>/auth/login`
- `https://<stage-domain>/auth/consent`
- `https://<stage-domain>/auth/setup`

### Remove token-in-URL bridging

Current flow passes a bearer JWT via `access_token` query params when navigating from Auth UI to `/oauth/authorize`. With single-origin routing this is no longer required.

Preferred approach (no cookies):

- Add a “UI mode” for `/oauth/authorize`:
  - When `Accept: application/json` (or `?mode=ui`) is present, return `{ next_url }` instead of issuing a 302.
  - Auth UI calls `/oauth/authorize` via `fetch()` with `Authorization: Bearer <jwt>`, then `window.location.href = next_url`.

Alternative (cookies allowed):

- Set an HttpOnly cookie scoped to `/oauth/*` after successful login, and keep `/oauth/authorize` as a redirect-based flow.

## Auth UI Changes (Astro)

- Build/deploy under the `/auth` base path (assets + router).
- Derive system API base from `window.location.origin` and use absolute paths (e.g. `fetch("/setup/status")`).
- Update OAuth bridge implementation to avoid writing bearer tokens to URLs.

## Client UI Changes

- Build/deploy under the `/l` base path (assets + router).
- Use system paths for API calls (same origin), not `api.<stage-domain>`.

## Compatibility / Migration

No legacy subdomains are provisioned by default.

- Canonical UI + system origin is always `https://<stage-domain>` with `/auth/*` and `/l/*`.
- `api.<stage-domain>` and `ws.<stage-domain>` may still exist as implementation details (origins), but are not part of the public contract.

## Acceptance Criteria

- `https://<stage-domain>/l` loads and supports deep links (refresh does not 404).
- `https://<stage-domain>/auth/login` and `https://<stage-domain>/auth/consent` load and support deep links.
- `/setup/*` endpoints always reach the API (no CDN allowlist gaps).
- OAuth authorization flow completes without bearer tokens in URLs.
- WebSockets remain reachable at `wss://ws.<stage-domain>`.
