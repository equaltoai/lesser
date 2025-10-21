# Lesser dev.lesser.host – Phase 1 Prep Report

## 1. Environment Reachability
- `dig dev.lesser.host` → **NXDOMAIN**; no A/AAAA records configured (authority: `lesser.host` SOA).
- HTTPS probes (`curl https://dev.lesser.host/graphql`) fail with `Could not resolve host`; TLS handshake impossible, no certificate observed.
- WebSocket checks (`/graphql/ws`) not attempted because DNS resolution fails; need DNS fix before transport validation.
- Latency metrics unavailable pending DNS delegation; seeding and GraphQL automation currently blocked at network bootstrap.

## 2. Credential Inventory
- No service/admin credentials provisioned; Secrets Manager contains **no** entries scoped to `dev.lesser.host` or `development` beyond CDN key (`lesser/cdn-private-key-development`).
- DynamoDB table `lesser-development` exists but has **no** bootstrap data; `scripts/generate_bootstrap_data.js` documents historical manual seeding flow and required artifacts (Dynamo items, OAuth client, JWT secret).
- Authentication test (`viewer`/`verify_credentials`) could not be executed: missing DNS + credentials.
- Storage location & rotation policy absent; recommend adopting AWS Secrets Manager (`lesser/dev/service-account`) with quarterly rotation tied to automation pipeline.

## 3. Media Storage Readiness
- Buckets present and reachable with `AWS_PROFILE=Lesser`:
  - `lesser-media-development` (region `us-east-1`) – probe file upload/download/delete succeeded.
  - `lesser-streaming-development` – same CRUD probe succeeded.
- No CloudFront/VAPID integration verified:
- CDN private key secret and key pair (`lesser/cdn-private-key-development`, CloudFront public key + key group) now provisioned via CDK custom resource; edge domain `cdn.dev.lesser.host` still lacks DNS records until new stack deploys.
  - VAPID keys absent for development (only `lesser/lesser.host/vapid-keys` found); push registration will fail until generated (see `cmd/init-deploy`, `cmd/configure-instance` helpers).

## 4. Data Safety Net
- DynamoDB `lesser-development` has **no backups** (`aws dynamodb list-backups …` returned `[]`); PITR is intentionally disabled for dev per team guidance.
- No documented snapshot cadence for dev; runbooks cover production/staging only. Owner for dev backups unnamed—defaulting to Platform/Infra is implied but unconfirmed.
- Rollback options: (a) ad-hoc DynamoDB `create-backup` prior to seeding, (b) rerun infrastructure deploy (`infra/cdk`), (c) regenerate bootstrap fixtures (`scripts/generate_bootstrap_data.js`). None automated today.

## 5. Fixture Asset Staging
- Repository has **no** `testdata/` directories or committed media fixtures; integration tests generate assets in-memory.
- Historical bootstrap script (`scripts/generate_bootstrap_data.js`) produces Dynamo JSON + creds but not media/prefs.
- Identified gaps for Phase 2:
  - Sample avatar/banner media (PNG/JPEG, <=1 MB).
  - Push subscription stub (`tests/api/test_push_notifications.py`) expects valid VAPID public key.
  - Instance preference/announcement JSON for baseline configuration.

### Asset Manifest (proposed)
| Asset | Source Path | Status | Intended Use |
| --- | --- | --- | --- |
| `testdata/media/sample-avatar.png` | (new) derive from design assets or generate 512×512 placeholder | Missing | Seed default user avatar & media upload smoke tests |
| `testdata/media/sample-banner.jpg` | (new) 1500×500 placeholder | Missing | Profile header for bootstrap accounts |
| `testdata/config/default-instance.json` | (new) export from admin configuration CLI | Missing | Apply consistent instance settings post-seed |
| `testdata/push/webpush-subscription.json` | (new) sanitized subscription payload | Missing | Validate push workflow with seeded data |
| `bootstrap/scripts/README.md` | `scripts/generate_bootstrap_data.js` (existing) | Present | Document bootstrap credential process |

## Risks & Blockers
- **DNS misconfiguration** – `dev.lesser.host` unresolved → Blocks all env validation & seeding. *Owner:* Platform/Infra (assign). *Action:* Deploy updated CDK stack (creates Route 53 A/AAAA aliases) and confirm records propagate.
- **Missing auth credentials** – No service/admin tokens; seeding automation cannot authenticate. *Owner:* App/Platform. *Action:* Generate via bootstrap script or Cognito/Lambda workflow; store in Secrets Manager with rotation policy.
- **Manual rollback required** – With PITR disabled, any bad seed requires recreate/restore by hand. *Owner:* Data Reliability/Platform. *Action:* Take ad-hoc snapshots before major seed runs or script table reset.
- **Push notification secrets incomplete** – Dev VAPID keys absent; tests relying on push flows will fail. *Owner:* App team. *Action:* Run `cmd/configure-instance --generate-vapid` against dev env and store secret.
- **Media CDN offline** – `cdn.dev.lesser.host` missing DNS; signed URL workflows and media caching unavailable. *Owner:* Platform/Infra. *Action:* Deploy new CloudFront alias records (added to CDK) and validate signed URL delivery once the auto-managed key pair is active.

## Tooling & Automation Recommendations
- Extend bootstrap script to emit Secrets Manager entries (JWT, admin token) and optional seed CLI that runs end-to-end.
- Add CI/CD guard checking DNS + TLS before Phase 2 runs (simple `dig`/`curl` health check job).
- Script an ad-hoc snapshot/rollback helper (`aws dynamodb create-backup` + truncate) to mitigate dev seeding mistakes without PITR.
- Create fixture repository (`testdata/`) with lightweight reusable assets; enforce via lint/checklist.
- Provide Make target (e.g., `make dev-seed-bootstrap`) wrapping `scripts/generate_bootstrap_data.js`, VAPID generation, and media fixture uploads for reproducibility.
