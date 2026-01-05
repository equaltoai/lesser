# Owner Bootstrap Requirements (Locked Deployments)

This document captures the non-negotiable requirements for bootstrapping owner/admin access for a newly deployed Lesser instance.

In the current model, deployments come up **locked but reachable**. A temporary bootstrap actor exists only to let the operator establish a “real” admin and finalize activation.

## Goals

- Deployments always start in a **locked** state.
- While locked, liveness is disabled (publishing and new signups are blocked).
- Exactly one bootstrap actor exists while locked (default username `bootstrap`).
- Bootstrap authentication is wallet-only: **connect wallet + sign challenge** (Ethereum).
- Bootstrap key material is **generated once locally** and is **never persisted to AWS**.
- Activation requires creating a real admin (passkey and/or wallet), then finalizing activation, which deletes the bootstrap actor/user.

## Seeded State (Per Stage)

Each stage’s data plane must contain:

- An `InstanceState` record with:
  - `locked=true`
  - `bootstrapWalletAddress=<0x...>` (lowercased)
  - `primaryAdminUsername=""` until created
- The bootstrap actor/user (only while locked).

Behavior while locked:

- Timeline/list endpoints return empty collections.
- Requests for non-existent objects return `404`.
- WebFinger returns only the bootstrap actor and `404` for everything else.
- Federation requests for the bootstrap actor return `403`.

## Setup API Contract

The setup wizard UI is served under `https://<stage-domain>/auth/setup`, and the backend contract is served by the stage
apex domain:

- Base: `https://<stage-domain>/setup/*`
- `GET /setup/status`
- `POST /setup/bootstrap/challenge`
- `POST /setup/bootstrap/verify` → returns a short-lived setup session token
- `POST /setup/admin` → creates the primary admin (requires setup token)
- `POST /setup/finalize` → unlocks the instance and deletes the bootstrap actor/user (requires admin auth)

## Idempotency Requirements

- Re-running `lesser up` must not rotate the bootstrap wallet address once recorded.
- If bootstrap state exists, reruns should be a no-op for key material and data seeding.
- Infrastructure deploys must remain safe to re-run (CloudFormation idempotency).

## Secrets and Keys

- Non-user secrets/keys (JWT signing secret, ActivityPub signing key, CDN keys, etc.) may be stored in AWS (Secrets Manager/SSM) and shared across stages when appropriate.
- The bootstrap admin wallet mnemonic/private key is never stored in AWS, never embedded in the binary, and is solely the operator’s responsibility to secure.

## References

- `docs/deployment.md`
