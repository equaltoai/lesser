# Lesser Deployment CLI (`lesser up`) — Requirements & Design

## Summary

Lesser needs a first-party CLI to deploy and initialize a new Lesser instance without `make`, `.env` files, or manual environment-variable choreography.

This document defines the required behavior of a `lesser` CLI that, given three inputs:

- `app` (URL-friendly slug)
- `base-domain` (assume an existing public Route53 hosted zone)
- `aws-profile` (string used as `AWS_PROFILE`)

can bring up an instance in a **locked-but-reachable** state, provide URLs and next steps, and hand off to an in-product setup wizard that creates a “real” admin and finalizes activation.

## Goals

- Single entrypoint: `lesser up` runs all required steps to reach **locked but reachable**.
- No `.env` files; no required environment variables beyond the three explicit inputs.
- Deterministic naming for all resources based on `app`, stage, AWS account ID, and region:
  - Non-global names: `<app>-<stage>-<resource>` and `<app>-shared-<resource>`
  - Global-unique names (e.g. S3): include account and region.
- Stacks:
  - One shared CloudFormation stack, deployed first: `<app>-shared`
  - One stack per stage: `<app>-dev`, `<app>-live`, optional `<app>-staging`
  - No CloudFormation exports; cross-stack references use well-known SSM Parameter Store names.
- Stages:
  - Always deploy **dev** and **live**.
  - Optionally deploy **staging**.
- Domains:
  - Live uses the apex for the client app.
  - Dev/staging use stage-prefixed domains.
- Locked behavior:
  - Instance returns empty collections for content list endpoints.
  - Non-existent content returns `404`.
  - Signups and publishing are blocked until activation.
  - NodeInfo behaves normally.
  - WebFinger returns only bootstrap actor; otherwise `404`.
  - Federation for bootstrap actor returns `403`.
- Admin bootstrap:
  - Bootstrap admin wallet is **generated once locally** (Ethereum, 24-word mnemonic).
  - Bootstrap wallet key material is **never persisted to AWS** and **never embedded in the binary**.
  - User is responsible for securing the key material.
  - The deployed instance stays locked until a real admin is created via the setup wizard.

## Non-goals (initial scope)

- Headless/non-interactive deployments.
- Destroy/teardown (`lesser down`) and rotation workflows.
- Supporting hosted zone discovery beyond exact match for `base-domain` (see “Hosted Zone”).

## Inputs

Required:

- `--app <slug>`
- `--base-domain <example.com>`
- `--aws-profile <profile>`

Optional:

- `--with-staging` (deploy staging stage as well)

Derived:

- AWS account ID: `sts:GetCallerIdentity`
- AWS region: “default region for the selected AWS profile”

## Outputs

`lesser up` prints:

- Stage URLs (dev and live; staging if enabled):
  - Setup URL: `https://<stage>.<base-domain>/auth/setup` (dev/staging), `https://<base-domain>/auth/setup` (live)
  - Client URL: `https://<stage>.<base-domain>/l` (dev/staging), `https://<base-domain>/l` (live)
  - System URL: `https://<stage>.<base-domain>` (dev/staging), `https://<base-domain>` (live)
  - WS URL: `wss://ws.<stage>.<base-domain>` (dev/staging), `wss://ws.<base-domain>` (live)
- A “next steps” section that instructs the user to:
  - Import the bootstrap mnemonic into a wallet (e.g. Metamask)
  - Visit the setup URL for dev/live and complete the wizard

Local state:

- A non-secret deployment receipt at: `~/.lesser/<app>/<base-domain>/state.json`
  - Contains account ID, region, stage list, stack names, outputs, and timestamps.
- Bootstrap admin key material:
  - Printed once to the terminal during `lesser up`.
  - Not written to disk by default.
  - If the user passes `--out <path>`, write a file with `0600` permissions containing the 24-word mnemonic and derived address.
  - The CLI must clearly warn this file is sensitive and user-owned.

## Domain & Certificate Strategy

### Domain map

Stages:

- dev: `dev.<base-domain>`
- staging (optional): `staging.<base-domain>`
- live: `<base-domain>` (no stage prefix)

Service hostnames (wildcard-friendly; subdomain set may grow):

- `ws`
- `media`
- (future additions permitted without cert redesign)

### Certificates (per-stage, stage stacks)

Each stage stack creates one ACM certificate in the deployment region with SANs:

- dev: `dev.<base-domain>` and `*.dev.<base-domain>`
- staging: `staging.<base-domain>` and `*.staging.<base-domain>`
- live: `<base-domain>` and `*.<base-domain>`

Validation:

- DNS validation using Route53 records in the hosted zone for `<base-domain>`.

## Hosted Zone

The CLI assumes a public Route53 hosted zone exists for the exact `base-domain`.

Behavior:

- If the hosted zone cannot be found, `lesser up` must emit an actionable error and exit before deploying anything.
- The CLI must not attempt to create hosted zones.

## Stack Architecture

### Shared stack: `<app>-shared`

Purpose: global shared resources and “registry” values for stage stacks.

Contains (minimum):

- KMS key(s) for in-AWS encryption needs (not for bootstrap wallet; see admin bootstrap).
- IAM roles/policies used by stage stacks (Lambda execution roles, etc.).
- SSM Parameter Store “registry” values with well-known names.

Does not contain:

- Stage-specific certificates
- Stage-specific DNS records
- Stage-specific service infrastructure

SSM parameter naming (examples):

- `/<app>/shared/kms/encryption-key-arn`
- `/<app>/shared/iam/lambda-basic-role-arn`
- `/<app>/shared/iam/lambda-encryption-role-arn`

### Stage stacks: `<app>-dev`, `<app>-staging` (optional), `<app>-live`

Each stage stack contains:

- ACM certificate for the stage (per “Certificates”).
- Route53 records for:
  - Stage apex domain (dev/staging stage apex; live apex)
  - `ws.*` and `media.*` for the stage
- One CloudFront distribution on the stage apex that routes by path:
  - `/l/*` → client app (S3)
  - `/auth/*` → auth UI (S3)
  - all other paths → API (API Gateway)
- API infrastructure on the stage apex (system endpoints).
- WebSocket endpoint at `ws...`.
- Data plane resources (tables, queues, buckets), named using `<app>-<stage>-<resource>`.

Cross-stack references:

- Stage stacks must read shared ARNs/IDs from SSM Parameter Store (no CloudFormation exports/imports).

Global uniqueness:

- S3 bucket names and other global-unique identifiers must include `accountId` and `region`:
  - Example: `<app>-<stage>-media-<accountId>-<region>`

## CLI Behavior

### `lesser up`

High-level steps:

1. Resolve AWS identity (account ID, region) using `aws-profile`.
2. Resolve Route53 hosted zone for `base-domain`; fail fast if missing.
3. Deploy shared stack `<app>-shared`.
4. Deploy stage stacks:
   - Always `dev` and `live`
   - `staging` if `--with-staging`
5. Perform “bootstrap admin” initialization for each deployed stage:
   - Create bootstrap actor (and only that actor) in the stage’s data store.
   - Configure instance state as locked.
6. Generate bootstrap wallet once locally and display it once.
7. Write local receipt files under `~/.lesser/<app>/<base-domain>/`.
8. Print stage URLs and next steps.

### Local key generation

Wallet requirements:

- Ethereum wallet
- 24-word mnemonic (BIP-39)
- Deterministic derivation path: `m/44'/60'/0'/0/0`

Security requirements:

- No key material persisted to AWS.
- No key material embedded or stored inside the `lesser` binary.
- Default output is to a local file with `0600`; the CLI must warn users to back it up.

## Setup Wizard Backend Contract (Required for This Effort)

This section defines the backend state model and endpoints that must exist so the setup wizard UI can be implemented later without reworking infrastructure.

### State model

The instance has an activation state stored in the stage data plane (DynamoDB):

- `instance_state`: one of `locked`, `active`
- `bootstrap_actor_id`: stable identifier for the bootstrap actor (exists only while locked)

Storage:

- DynamoDB item in the stage’s main table: `PK="CONFIG"`, `SK="INSTANCE"`
- Fields:
  - `instance_state`
  - `bootstrap_actor_id`
  - `created_at`, `updated_at`

Invariants:

- `locked`: signups and publishing are blocked; bootstrap actor exists; content is empty (see “Locked Instance Behavior”).
- `active`: signups/publishing allowed; bootstrap actor deleted; normal instance behavior.

### Authentication requirements

Bootstrap authentication is via wallet signature only:

- Challenge-response flow using **EIP-191 `personal_sign`** returning a short-lived setup token.
- The setup token is only valid while `instance_state=locked`.
- Challenge requirements:
  - server-generated nonce
  - short TTL (e.g. 5 minutes)
  - single-use
  - message must bind at least: `domain`, `address`, `nonce`, `issued_at`, `expires_at`

Real-admin authentication methods supported (wizard creates at least one):

- Passkey (WebAuthn) bound to the stage domain (RP ID is stage-specific).
- Wallet signature credential (similar challenge-response binding) for admin login.

### Endpoints (proposed)

All endpoints are stage-local and served by the stage domain (system paths). The Auth UI consumes them from `/<stage-domain>/auth/*`.

Public/read-only:

- `GET /setup/status`
  - Returns:
    - `instance_state`
    - `bootstrap_actor` (minimal public descriptor; no secrets)
    - flags indicating whether a real admin exists and whether finalize is allowed

Bootstrap auth:

- `POST /setup/bootstrap/challenge`
  - Request: `{ "address": "0x..." }`
  - Response: `{ "challenge": "...", "expires_at": "..." }`

- `POST /setup/bootstrap/verify`
  - Request: `{ "address": "0x...", "signature": "0x...", "challenge": "..." }`
  - Response: `{ "setup_token": "...", "expires_at": "..." }`

Wizard actions (require `setup_token`):

- `POST /setup/admin`
  - Creates the first “real admin” account and binds credentials.
  - Request supports at least:
    - passkey registration payload (WebAuthn attestation)
    - wallet credential binding (address + proof)
  - Response: identifiers for the created admin and next-step hints.

- `POST /setup/finalize`
  - Preconditions:
    - caller is authenticated as a real admin (not bootstrap)
    - `instance_state=locked`
  - Effect:
    - delete bootstrap actor
    - set `instance_state=active`
  - Response: success + instance URLs.

### Authorization rules

- While locked:
  - Only setup endpoints are permitted for privileged actions.
  - Bootstrap actor cannot be used as a normal user (no publishing, no federation; see locked behavior).
- Finalize:
  - Must be executed by a real admin, after they have successfully authenticated.
  - Must delete the bootstrap actor entirely.

### User handoff: Setup Wizard (Out of Scope for This Effort)

This effort establishes the infrastructure and backend contract needed to support a setup wizard, but does **not** implement the wizard application itself.

Wizard integration requirements (infra/back-end only):

- The wizard begins once the bootstrap wallet logs in by “connect wallet + sign challenge”.
- Expected wizard steps (implemented by a separate project):
  1. Authenticate as bootstrap.
  2. Create a “real admin” with passkey and/or wallet credentials.
     - Passkey binding is per-stage (RP ID is the stage’s `auth` domain).
     - Encourage users to configure both passkey and wallet.
  3. Login as the real admin.
  4. Finalize activation by clicking a button that:
     - Deletes the bootstrap actor/account entirely.
     - Unlocks the instance (liveness enabled).
- If bootstrap credentials are lost before activation, recovery is teardown and redeploy.

## Locked Instance Behavior (Pre-activation)

While locked:

- Content list endpoints (timelines, feeds, search results) return empty collections.
- Requests for content that does not exist return `404`.
- Signup endpoints return `403`.
- Publishing endpoints return `403`.
- NodeInfo endpoints behave normally.
- `/.well-known/webfinger`:
  - Returns only the bootstrap actor record.
  - Returns `404` for any other actor.
- Federation:
  - Bootstrap actor federation requests return `403` (not intended to be a full account).
  - Other actors return what exists (generally none → `404` / empty collections).

## Review of Existing Domains (current codebase)

Current infra references additional stage subdomains for service origins (notably `api.<domain>`, `ws.<domain>`, and `media.<domain>`).

New requirement moves human UIs to stage-apex paths:

- Client UI: `https://<domain>/l/*`
- Auth UI: `https://<domain>/auth/*`

System endpoints remain on the stage apex at top-level paths (for Mastodon/ActivityPub compatibility), while WebSockets stay standardized on `ws.<domain>` (current implementation supports streaming behind `ws.<domain>/stream`).

## Acceptance Criteria

- A user can run:
  - `lesser up --app <app> --base-domain <base-domain> --aws-profile <profile> [--with-staging]`
  - and reach a state where dev and live are deployed, reachable, and locked.
- No `.env` files are required.
- Hosted zone mismatch fails fast with a clear error.
- Shared stack deploys before any stage stack.
- Stage stacks reference shared resources only via SSM parameter names.
- Bootstrap mnemonic is generated locally, written to `~/.lesser/...` with secure permissions, and never uploaded to AWS.
- Setup wizard can:
  - authenticate via wallet signature (bootstrap),
  - create real admin (passkey/wallet),
  - finalize activation and delete bootstrap,
  - resulting in an instance that can accept signups and publish content.

## Implementation Plan (This Effort)

This plan delivers:

- A `lesser` CLI with `lesser up` that deploys shared + dev/live (+ optional staging) to a locked-but-reachable state.
- Backend support for locked gating, bootstrap auth challenge, and finalize activation (wizard UI out of scope).

### Milestone 0 — Inventory and naming alignment

- Prefer Lift CDK constructs wherever available (KMS keys, IAM roles, tables, static sites, CDNs, etc.) to reduce bespoke infra code.
- Confirm the complete set of required domains/subdomains in current infra and consolidate WebSocket hostname to `ws` (keeping wildcard certs).
- Define a single naming module used by CDK and the CLI:
  - stack names: `<app>-shared`, `<app>-dev`, `<app>-staging`, `<app>-live`
  - SSM parameters: `/<app>/shared/...`
  - stage resources: `<app>-<stage>-<resource>` (and global uniqueness suffix where needed)

### Milestone 1 — Shared stack refactor

- Update CDK to accept `--context app`, `--context baseDomain`, `--context stage` (or equivalent) and derive names deterministically.
- Ensure shared stack contains only:
  - KMS key(s) for in-AWS encryption needs
  - IAM roles/policies for stage stacks
  - SSM registry parameters (well-known names) required by stage stacks
- Remove CloudFormation exports/imports; stage stacks read shared values from SSM.
- Ensure CloudFormation execution roles have permission to read the shared SSM parameters.

### Milestone 2 — Stage stacks: domains, Route53, certs

- For each stage stack:
  - Discover the hosted zone for `base-domain` and error if missing.
  - Create Route53 records for:
    - client app: `dev.<base>`, `staging.<base>`, `<base>` (live)
    - service hosts: `api.*`, `ws.*`, `media.*`
  - Create ACM cert with SANs:
    - dev: `dev.<base>`, `*.dev.<base>`
    - staging: `staging.<base>`, `*.staging.<base>`
    - live: `<base>`, `*.<base>`
- Ensure stage stacks can be deployed independently once shared is present.

### Milestone 3 — Locked gating in the backend

- Add `instance_state` config item to the stage table:
  - `PK="CONFIG"`, `SK="INSTANCE"`
  - Initialize to `locked` at deploy/bootstrap time.
- Implement request gating:
  - list endpoints: return empty collections while locked
  - missing objects: `404`
  - signup/publish endpoints: `403 activation_required`
  - nodeinfo: normal
  - webfinger: only bootstrap actor; else `404`
  - federation bootstrap actor: `403`
- Add a small in-memory cache for `instance_state` with short TTL (e.g. 5–30s), with explicit refresh on finalize.

### Milestone 4 — Setup endpoints (wizard backend contract)

- Implement endpoints defined in “Setup Wizard Backend Contract”:
  - `GET /setup/status`
  - `POST /setup/bootstrap/challenge`
  - `POST /setup/bootstrap/verify`
  - `POST /setup/admin` (creates first real admin + binds credentials; passkey/wallet)
  - `POST /setup/finalize` (real admin only; deletes bootstrap; sets active)
- Bootstrap auth:
  - EIP-191 `personal_sign` message format and nonce storage with TTL + single-use.
- Finalize:
  - hard-delete bootstrap actor/account
  - set `instance_state=active`

### Milestone 5 — CLI: `lesser up`

- Implement a Go CLI (new `cmd/lesser` or similar) that:
  - Validates `app` slug and `base-domain` format.
  - Uses AWS SDK with `AWS_PROFILE` to obtain account ID + region.
  - Resolves Route53 hosted zone for `base-domain` (exact match) and errors if missing.
  - Deploys shared stack first, then stage stacks (dev/live; staging optional).
    - Deployment mechanism: shell out to the AWS CDK CLI (`cdk deploy`), not direct CloudFormation API calls.
    - Prerequisites: `cdk` (v2) must be installed and available on `PATH`.
    - The CLI must pass `AWS_PROFILE=<aws-profile>` to all CDK invocations and use `--require-approval never`.
    - The CLI should ensure CDK bootstrap is present (fail with actionable instructions or run `cdk bootstrap` if missing).
  - Generates a bootstrap Ethereum wallet locally (BIP-39 24 words, derivation `m/44'/60'/0'/0/0`).
    - Prints mnemonic once.
    - Writes mnemonic only if `--out <path>` is provided (chmod `0600`).
  - Calls the stage API bootstrap endpoint(s) (or writes directly to DynamoDB if preferred) to create:
    - bootstrap actor
    - `instance_state=locked`
  - Writes `~/.lesser/<app>/<base-domain>/state.json` (non-secret receipt).
  - Prints URLs and next steps for dev/live (+ staging if enabled).

### Milestone 6 — Documentation and operator UX

- Update deployment docs to point to `lesser up` instead of Make targets.
- Add a concise “What you should see” section:
  - empty timelines while locked
  - setup URLs
  - finalize activation deletes bootstrap and enables liveness
