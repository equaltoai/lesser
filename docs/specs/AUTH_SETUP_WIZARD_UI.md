# Auth UI: Setup Wizard (Locked Deployments)

> **Status**: Implemented (MVP)  
> **Created**: 2025-12-24  
> **Depends On**: `docs/specs/LESSER_UP_CLI.md`, `docs/specs/AUTH_UI_GREATER_CLI_MIGRATION.md`

## Summary

Lesser deployments come up **locked but reachable**. A temporary bootstrap actor exists only to let the operator establish a “real” admin and finalize activation.

This document specifies the Auth UI wizard that runs on `https://<stage-domain>/auth/setup` and drives the backend activation contract served by the system API paths under `https://<stage-domain>/setup/*`.

## Implementation

- Route: `auth-ui/src/pages/setup.astro`
- Wizard: `auth-ui/src/components/SetupWizard.svelte`

## Goals

- Provide a guided, low-footgun setup flow for brand new deployments:
  - Bootstrap wallet login (Ethereum, connect + sign).
  - Create the “real” admin (wallet required).
  - Encourage enrolling a passkey for the admin (optional but strongly recommended).
  - Finalize activation (deletes bootstrap actor).
- Keep UI hosted under the `/auth` path (never serve HTML from system API endpoints).
- No cookies, no server sessions; **use `sessionStorage`** for UX continuity.
- No secret/key material is collected or stored by the UI (no mnemonics, no private keys).
- Work for all stages:
  - dev: `dev.<base-domain>/auth`
  - staging: `staging.<base-domain>/auth` (optional)
  - live: `<base-domain>/auth`

## Non-goals

- Headless deployments (CLI only).
- Recovery flows if credentials are lost (assume teardown is required).
- Non-operator user onboarding or full admin console.
- System actor setup (future).

## Domains & Endpoint Routing

### URLs

The UI is served from:

- `https://<stage-domain>/auth`

The API calls must target the same origin, using system paths:

- `https://<stage-domain>/setup/*`
- `https://<stage-domain>/auth/*` (wallet endpoints live under `/auth/wallet/*`)
- `https://<stage-domain>/oauth/*`
- `https://<stage-domain>/api/v1/*`

### Deriving bases in the UI

Given `window.location`:

- `origin = window.location.origin`
- `wizardBasePath = "/auth"`
- `systemBase = origin` (calls use absolute paths like `fetch("/setup/status")`)

If the page is not served under `/auth`, treat as misconfiguration and show an actionable error (“This page must be served from /auth on your Lesser domain”).

## Storage & Security Model

### Browser storage

Use `sessionStorage` (not `localStorage`) for wizard continuity:

- Wizard progress can survive navigation and refresh during setup.
- Data is cleared when the tab/session ends.

### `sessionStorage` keys (proposed)

All keys are namespaced and versioned:

- `lesser.setup.v1.stageDomain` → `string`
- `lesser.setup.v1.systemBase` → `string`
- `lesser.setup.v1.step` → `string` (e.g. `status|bootstrap_login|create_admin|admin_login|passkey|finalize|done`)
- `lesser.setup.v1.setupToken` → `string` (Bearer token from `POST /setup/bootstrap/verify`)
- `lesser.setup.v1.bootstrapAddress` → `string` (0x… lowercased)
- `lesser.setup.v1.adminUsername` → `string`
- `lesser.setup.v1.adminJwt` → `string` (JWT after admin login)

Clear all `lesser.setup.v1.*` keys on successful finalize.

### Security constraints

- Never collect/store mnemonics or private keys.
- Wallet operations must be performed via the user’s wallet provider (MetaMask/etc).
- Tokens are passed only via:
  - `Authorization: Bearer <token>` for API calls
  - `sessionStorage` for temporary persistence

## Backend Contract (Required)

### Setup endpoints (system paths)

From `https://<stage-domain>`:

- `GET /setup/status`
- `POST /setup/bootstrap/challenge`
- `POST /setup/bootstrap/verify` → returns `setup_token`
- `POST /setup/admin` (requires setup token)
- `POST /setup/finalize` (requires admin auth)

### Wallet auth endpoints (system paths)

From `https://<stage-domain>`:

- `POST /auth/wallet/challenge` (requires `username` binding)
- `POST /auth/wallet/login`

### Passkey endpoints (system paths)

From `https://<stage-domain>`:

- `POST /api/v1/auth/webauthn/register/begin` (requires admin JWT)
- `POST /api/v1/auth/webauthn/register/finish` (requires admin JWT)

Passkeys are stage-scoped because the WebAuthn RP ID is the stage domain (for example `dev.example.com`), which matches the UI origin host.

## Wizard Flow

### Step 0: Status (gating)

Call:

- `GET {systemBase}/setup/status`

UI behavior:

- If `locked=false`: show “Instance already activated” with links to:
  - client (`https://<stage-domain>/l`)
  - auth (`https://<stage-domain>/auth`)
  - system (`https://<stage-domain>`)
  - exit setup
- If `locked=true`: show:
  - bootstrap actor `acct` and actor URL
  - whether bootstrap wallet address is configured
  - stage URLs
  - continue

### Step 1: Bootstrap wallet login (wallet-only)

Inputs:

- Wallet address (via wallet provider)
- Chain ID (default 1)

Calls:

1. `POST {systemBase}/setup/bootstrap/challenge` with `{ address, chainId }`
2. Wallet signs returned `challenge` message
3. `POST {systemBase}/setup/bootstrap/verify` with `{ challengeId, address, signature, message }`

Outputs:

- Store `setup_token` in `sessionStorage` as `lesser.setup.v1.setupToken`.

### Step 2: Create real admin (wallet required)

Requirements:

- Operator chooses `adminUsername` (must not be `bootstrap`).
- Operator wallet must be connected and able to sign.

Calls:

1. Create username-bound challenge for the real admin:
   - `POST {systemBase}/auth/wallet/challenge` with `{ address, chainId, username: adminUsername }`
2. Wallet signs challenge message.
3. Create admin:
   - `POST {systemBase}/setup/admin`
   - Headers: `Authorization: Bearer {setupToken}`
   - Body:
     - `username`, optional `displayName`
     - `wallet`: `{ challengeId, address, signature, message }`

UI behavior:

- If `409 primary admin already created`: treat as resumable; fetch status and proceed to admin login.
- On success: proceed to admin login.

### Step 3: Login as real admin (wallet)

Calls:

1. `POST {systemBase}/auth/wallet/challenge` with `{ address, chainId, username: adminUsername }`
2. Wallet signs challenge message.
3. `POST {systemBase}/auth/wallet/login` with `{ challengeId, address, signature, message }`

Outputs:

- Store admin JWT in `sessionStorage` as `lesser.setup.v1.adminJwt`.

### Step 4: Encourage passkey enrollment (optional)

Goal:

- Encourage the operator to register a passkey in addition to the wallet.

Calls:

1. `POST {systemBase}/api/v1/auth/webauthn/register/begin`
   - Headers: `Authorization: Bearer {adminJwt}`
2. Browser completes WebAuthn ceremony.
3. `POST {systemBase}/api/v1/auth/webauthn/register/finish`
   - Headers: `Authorization: Bearer {adminJwt}`

UX requirement:

- Allow skipping, but require explicit acknowledgement (“I understand: if I lose the wallet, teardown is required”).

### Step 5: Finalize activation (destructive)

Calls:

- `POST {systemBase}/setup/finalize`
- Headers: `Authorization: Bearer {adminJwt}`

Effects:

- Instance unlocked (liveness enabled).
- Bootstrap actor/user deleted.

UI behavior:

- Clear all `lesser.setup.v1.*` keys.
- Show final links:
  - client: `https://<stage-domain>/l`
  - auth: `https://<stage-domain>/auth`
  - setup status check: `https://<stage-domain>/setup/status`

## Error Handling Requirements (Minimum)

- Network errors and non-2xx responses show an `Alert` with:
  - action to retry
  - “copy debug info” (request URL + status + correlation ID if present)
- Explicit handling for:
  - `401/403` on setup token: re-run bootstrap login step
  - `409` on admin creation: treat as resumable
  - `409` on finalize (already activated): treat as done, show links

## UX Requirements

- Render clear step progress with the ability to resume after refresh.
- Use wallet-first language (“connect wallet + sign”) and explain why.
- Strongly encourage adding a passkey, but keep it optional.
- Avoid any instruction that asks the operator to paste/enter private key material.

## Success Criteria

- From a fresh `lesser up` deployment, the operator can complete all steps to activation using only the Auth UI.
- Setup state can be resumed after refresh via `sessionStorage`.
- No secrets are written to disk by the UI and no secrets are persisted in AWS for the bootstrap wallet.
