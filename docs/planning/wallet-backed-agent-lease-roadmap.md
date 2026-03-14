# Lesser: Wallet-Backed Agent Lease Roadmap (2026-03-12)

This roadmap defines the Lesser implementation plan for replacing long-lived delegated OAuth refresh tokens with a
wallet-backed lease flow for continuously running local agents.

It is intended to serve two roles:

- the implementation tracker for Lesser
- the contract source for the later Simulacrum UI issue that replaces token delegation on the agent page

## Problem

The current delegated agent OAuth flow is intentionally bounded:

- delegated refresh tokens use `RefreshTokenDuration = 7 * 24 * time.Hour`
- delegated refresh rotation does not extend the original delegation window
- old refresh tokens are deleted on successful rotation

That behavior is reasonable for short-lived human delegation, but it is a poor fit for a continuously running local
agent. We need a durable access model that avoids indefinite bearer refresh tokens while still allowing long-term local
operation.

## Goal

Introduce a wallet-backed `AgentAccessLease` flow where:

- the principal explicitly authorizes local access for an agent
- the agent explicitly accepts the lease with its own wallet
- the local runtime renews lease-window access tokens by signing server-issued renewal challenges
- Lesser enforces revocation, idle expiry, and absolute expiry on the server side
- no long-lived bearer refresh token is required for the steady-state local-agent flow

## Invariants

- Lesser remains email-free.
- Principal approval and agent acceptance must be cryptographically distinct acts.
- The principal wallet must already be linked to the principal account.
- The agent wallet must already be linked to the target agent account.
- Renewal must require proof of possession of the agent-side signer.
- Access token TTL is bounded by the active lease window.
- Renewal may extend the idle window, but must never extend the absolute expiry.
- The legacy `/api/v1/agents/delegate` flow remains available for short-lived/manual delegation, but it is not the
  durable local-agent path.

## Phase Plan

### M0: Contract freeze

Acceptance criteria:

- this roadmap names the Lesser REST contract and state model
- naming and payload choices are stable enough for Simulacrum to target
- a clear handoff section exists for the later Simulacrum issue

### M1: Lease storage + challenge primitives

Acceptance criteria:

- add a persisted `AgentAccessLease` model
- add a persisted `AgentAccessLeaseChallenge` model
- challenges are short-lived, one-time-use, and TTL-backed
- leases track:
  `id`, `agent_username`, `principal_username`, `principal_wallet`, `agent_wallet`, `scopes`, `device_label`,
  `status`, `idle_timeout_hours`, `idle_expires_at`, `absolute_expires_at`, `last_used_at`, `lease_version`,
  `revoked_at`, `revoked_by`, `revoked_reason`

### M2: Enrollment APIs

Acceptance criteria:

- principal can request an approval challenge from the agent page
- agent can request an acceptance challenge for the same pending lease
- Lesser validates:
  - authenticated caller is the agent owner
  - requested scopes are within the caller token scopes
  - requested scopes are within the agent delegated scope envelope
  - principal wallet belongs to the owner account
  - agent wallet belongs to the agent account
- Lesser can finalize a lease after verifying both signatures

### M3: Renewal APIs

Acceptance criteria:

- Lesser issues a short-lived renewal challenge for an existing active lease
- Lesser verifies the renewal signature against the agent wallet
- Lesser updates `last_used_at`
- Lesser slides `idle_expires_at` forward up to `absolute_expires_at`
- Lesser mints a new lease-window access token without minting a durable refresh token

### M4: Lease management APIs

Acceptance criteria:

- list active and historical leases for an agent
- revoke a lease immediately
- revocation blocks future renewals
- revoked leases remain inspectable until TTL cleanup

### M5: Hardening follow-up

Acceptance criteria:

- audit events for lease create, renew, revoke, and failed verification
- signer rotation support
- EIP-712 typed data is the default for wallet-signed lease actions
- session-key authorization exists so the agent wallet need not sign every renewal

## Lesser REST Contract

Phase 1 contract for Lesser:

### 1. Create principal approval challenge

`POST /api/v1/agents/{username}/access-leases/challenge/principal`

Auth:

- bearer token required
- caller must be the agent owner

Request:

```json
{
  "principal_wallet": "0x...",
  "agent_wallet": "0x...",
  "scopes": ["read", "write", "follow"],
  "device_label": "local-agent",
  "lease_id": "",
  "idle_timeout_hours": 168,
  "absolute_ttl_hours": 2160
}
```

Behavior:

- generates `lease_id` if omitted
- returns a challenge message bound to the lease details and principal wallet

### 2. Create agent acceptance challenge

`POST /api/v1/agents/{username}/access-leases/challenge/agent`

Auth:

- bearer token required
- caller must be the agent owner

Request:

```json
{
  "principal_wallet": "0x...",
  "agent_wallet": "0x...",
  "scopes": ["read", "write", "follow"],
  "device_label": "local-agent",
  "lease_id": "lease_...",
  "idle_timeout_hours": 168,
  "absolute_ttl_hours": 2160
}
```

Behavior:

- requires an existing `lease_id`
- returns a challenge message bound to the same lease details and agent wallet

### 3. Finalize lease

`POST /api/v1/agents/{username}/access-leases`

Auth:

- bearer token required
- caller must be the agent owner

Request:

```json
{
  "principal_challenge_id": "challenge_...",
  "principal_signature": "0x...",
  "agent_challenge_id": "challenge_...",
  "agent_signature": "0x..."
}
```

Behavior:

- validates both stored challenges
- validates both signatures
- marks both challenges used
- persists the lease

### 4. List leases

`GET /api/v1/agents/{username}/access-leases`

Auth:

- bearer token required
- caller must be the owner or an admin

### 5. Revoke lease

`POST /api/v1/agents/{username}/access-leases/{leaseID}/revoke`

Auth:

- bearer token required
- caller must be the owner or an admin

Request:

```json
{
  "reason": "operator request"
}
```

### 6. Create session-key authorization challenge

`POST /api/v1/agents/{username}/access-leases/{leaseID}/session-key/challenge`

Auth:

- no bearer token required

Request:

```json
{
  "session_public_key": "<base64 ed25519 public key>"
}
```

Behavior:

- validates that the lease is active
- returns an EIP-712 wallet challenge that binds the requested session key to the lease

### 7. Authorize session key

`POST /api/v1/agents/{username}/access-leases/{leaseID}/session-key`

Auth:

- no bearer token required

Request:

```json
{
  "challenge_id": "challenge_...",
  "signature": "0x..."
}
```

Behavior:

- validates the challenge and the agent-wallet EIP-712 signature
- stores the session public key on the lease

### 8. Create renewal challenge

`POST /api/v1/agents/{username}/access-leases/{leaseID}/renew/challenge`

Auth:

- no bearer token required

Behavior:

- validates that the lease is active
- if a session key is authorized, returns a short-lived session-key renewal challenge
- otherwise returns an EIP-712 renewal challenge for the agent wallet

### 9. Exchange renewal proof for access token

`POST /api/v1/agents/{username}/access-leases/{leaseID}/token`

Auth:

- no bearer token required

Request:

```json
{
  "challenge_id": "challenge_...",
  "signature": "0x..."
}
```

Behavior:

- validates the challenge and signature
- updates lease activity timestamps
- returns a lease-window access token
- does not return a refresh token

## Message format

Wallet-signed lease actions use EIP-712 typed data. The signed payload must bind:

- Lesser domain
- challenge id
- lease id
- action
- principal username
- agent username
- principal wallet
- agent wallet
- session public key when applicable
- scopes
- device label
- idle timeout
- absolute TTL or absolute expiry
- nonce
- issued at
- expires at

Session-key renewals use an Ed25519 signature over the server-issued renewal challenge string.

## Token model

- Lease renewal mints a lease-window agent access token with a dedicated client id, separate from the delegated refresh
  client.
- No durable refresh token is returned from the lease renewal path.
- Token TTL is capped by the remaining lease lifetime.

## Simulacrum Handoff

Once the Lesser contract is stable enough for consumption, create a Simulacrum issue that:

- replaces the current token delegation UI on the agent page
- drives the dual-signature flow:
  - sign as principal
  - sign as agent
- drives session-key authorization after lease creation
- shows the lease scope envelope and expiration policy before signing
- stores only the local material needed to request renewal challenges and sign them
- surfaces active leases, last used, idle expiry, absolute expiry, revoke, and signer rotation

## Suggested milestones to track in implementation PRs

- [ ] Add lease + challenge storage models
- [ ] Add enrollment API models and handlers
- [ ] Add renewal API models and handlers
- [ ] Add session-key authorization API models and handlers
- [ ] Add list + revoke handlers
- [ ] Add route coverage and route snapshot updates
- [ ] Add focused tests for helper logic and handler flows
- [ ] Add OpenAPI contract and follow-up docs
