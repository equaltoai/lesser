# Lesser Client App (Greater) Guide

This guide describes how to build and deploy a standalone “Greater” web client for a Lesser instance while keeping all
system endpoints on the same stage apex domain and serving the client UI under `/l/`.

## Summary (recommended approach)

- **UI**: a static web app built with **Greater Components** (vendored) + your framework of choice (SvelteKit or Vite).
- **Infra**: use **Lift CDK** (already used by Lesser) to provision, per stage:
  - `CloudFront` distribution on the **stage apex domain**
  - an `S3` bucket for the client UI
  - path routing so `/<prefix>/…` is served from S3 while API routes continue to hit the API origin
- **Deploy**: build the client, then run `lesser client deploy` to upload artifacts into the stage client bucket and invalidate `CloudFront`.

## Domain + path requirements

Lesser deployments always include at least two stages:

- **dev**: `dev.<base-domain>`
- **live**: `<base-domain>`

The client UI must be reachable at:

- `https://dev.<base-domain>/l/…`
- `https://<base-domain>/l/…`

The stage apex root must redirect to the client:

- `GET https://dev.<base-domain>/` → `302 Location: /l/`
- `GET https://<base-domain>/` → `302 Location: /l/`

Top-level paths are reserved for system endpoints (examples):

- `/api/*`, `/api/v1/*`, `/api/v2/*` (Mastodon-compatible API)
- `/.well-known/*` (WebFinger / NodeInfo discovery)
- `/auth/*` (auth UI)
- `/setup/*` (bootstrap endpoints)
- `/media/*` (media CDN/origin)
- `/l/*` (client UI)

## How the stage apex domain works today (Lift CDK)

Lesser stage stacks provision a single distribution for the stage apex domain using Lift CDK:

- Construct: `liftcdk.NewPathRoutedFrontendDistribution(...)`
- Default origin: API origin (typically `api.<stage-domain>`)
- Additional behaviors:
  - `/l` and `/l/*` → client S3 bucket
  - `/auth` and `/auth/*` → auth S3 bucket
  - `/auth/wallet/*` → API origin (bypass auth UI routing)

### Important: how `/l/` is served from S3

The distribution attaches a CloudFront Function that rewrites request paths:

- `GET /l` or `GET /l/` → S3 key `/index.html`
- `GET /l/_assets/app.js` → S3 key `/_assets/app.js` (prefix is stripped)
- `GET /l/some/route` (extensionless) → S3 key `/index.html` (SPA fallback)

This means your client build output must be rooted at:

```
dist/
  index.html
  _assets/...
  ...other static files...
```

…and the browser must request all client assets under `/l/…` so those requests hit the client bucket behavior.

## Root redirect (`/` → `/l/`)

Because the stage apex distribution’s default behavior routes to the API origin, Lesser implements the redirect in the
**Lesser API**:

- `GET /` returns `302` with `Location: /l/`

If you move ownership of the stage apex distribution into the client project, you can instead implement the redirect at
CloudFront (viewer-request function) so the request never reaches the origin.

## Client application requirements

### 1) Base path: `/l`

The client must be built with a base path of `/l/` (or use relative asset URLs) so that:

- All static assets load from `/l/_assets/...` (not `/_assets/...`)
- All internal routes render under `/l/...`
- Refreshing a deep link (e.g. `/l/@alice`) loads the SPA successfully

### 2) No API secrets in the client

The client must be “public config only”. It should derive API URLs from the current origin and should not embed secrets.

Recommended defaults (no env vars required):

- API: `window.location.origin` (same domain)
- Auth: `window.location.origin + "/auth"`
- WebSocket (GraphQL subscriptions): `wss://ws.${window.location.host}`
- WebSocket streaming (optional): `wss://ws.${window.location.host}/stream`

### 3) Compatible with stage isolation

Dev and live are separate deployments. The client must not hardcode a domain; it should behave correctly on whichever
stage apex it’s served from.

### 4) Greater Components (vendored)

Use the Greater CLI vendored mode so the client app is self-contained:

- `greater add primitives icons utils`
- commit `components.json` + `src/lib/greater/**` + `src/lib/styles/greater/**`

This matches how `auth-ui/` is set up today.

## Recommended implementation: SvelteKit (static)

SvelteKit gives you routing + data loading ergonomics while still producing a static build.

### Key config items

- Set base path to `/l`:
  - `kit.paths.base = '/l'`
- Use adapter-static (or equivalent).
- Ensure client-side routing works under `/l/*` (SPA fallback is handled by CloudFront).

## Alternative implementation: Vite + Svelte (SPA)

If you’re using Vite directly:

- Set `base: '/l/'` in `vite.config.ts`.
- Build output to `dist/` with `index.html` at the root of `dist/`.

## Deploying the client to dev + live

### Prerequisites

- A Lesser deployment already exists for the app + base domain (stage stacks create the buckets + distribution).
- AWS access via the deployer’s `AWS_PROFILE`.
- `lesser up` uploads a placeholder `index.html` into the client bucket so `/l/` is reachable even before the real client is deployed.

### Recommended: deploy via the Lesser CLI

After your client builds to `dist/`:

```bash
./lesser client deploy \
  --app <slug> \
  --base-domain <example.com> \
  --aws-profile <profile> \
  --dist ./dist
```

Notes:

- Defaults to deploying **both** `dev` and `live` (`--stage both`).
- Use `--stage dev|staging|live|all` to target specific stages.
- The CLI reads the deployment receipt from `~/.lesser/<app>/<base-domain>/state.json` (or `--state <path>`).

### Locate the per-stage bucket + distribution

From the Lesser deployment receipt (recommended):

- `~/.lesser/<app>/<base-domain>/state.json`
  - `stages.dev.stack_outputs.ClientBucketName`
  - `stages.dev.stack_outputs.FrontendDistributionId`
  - `stages.live.stack_outputs.ClientBucketName`
  - `stages.live.stack_outputs.FrontendDistributionId`

Bucket names are also deterministic:

- `<app>-dev-client-<accountId>-<region>`
- `<app>-live-client-<accountId>-<region>`

### Manual deploy (fallback)

Upload the client build output (the contents of `dist/`) to each stage bucket root.

Caching guidance:

- `index.html`: short TTL (or no-cache)
- hashed assets (e.g. `_assets/*.js`): long TTL (`immutable`)

### Invalidate CloudFront

Invalidate at least:

- `/l`
- `/l/*`

This ensures the new `index.html` is picked up immediately.

### Verification checklist

- `GET https://dev.<base-domain>/l/` returns HTML
- Browser loads JS/CSS from `/l/_assets/...` (not from `/_assets/...`)
- Deep link refresh works: `GET https://dev.<base-domain>/l/<any-route>` loads the SPA

## If you want the client repo to own infra (optional)

If you prefer the client project to provision the stage apex distribution itself (instead of Lesser doing it), use Lift
CDK’s `PathRoutedFrontendDistribution` with:

- `DomainName`: stage apex domain (`dev.<base-domain>` / `<base-domain>`)
- `HostedZone`: existing Route53 hosted zone for `<base-domain>`
- `ApiOriginDomainName`: `api.<stage-domain>`
- `ClientPathPrefix`: `l`
- `ClientSinglePageApp`: `true`

This is only recommended if you’re also moving ownership of the stage apex distribution out of Lesser, since two stacks
cannot both “own” the same apex alias + CloudFront distribution cleanly.

## Decisions

- Rendering: SPA-only for v1 (static build).
- Root redirect: `/` 302 redirects to `/l/`.
- Caching: HTML short TTL (or no-cache); hashed assets long TTL (`immutable`).
