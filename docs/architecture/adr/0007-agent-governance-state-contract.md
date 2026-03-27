# 0007 AgentGovernanceState Contract

## Status

Accepted

## Context

Agent quarantine, delegation, and verification state previously lived inside `User.Metadata`.
That made generic account hydration sensitive to governance-schema drift and let unrelated product paths fail while loading `GetUser` or `GetAccount`.

## Decision

Agent governance state moves to a dedicated row:

- PK: `USER#{username}`
- SK: `AGENT_GOVERNANCE`

The canonical typed fields are:

- `username`
- `quarantineStatus`
- `quarantineStart`
- `quarantineEnd`
- `quarantineApprovedBy`
- `quarantineApprovedAt`
- `delegatedScopes`
- `selfScopes`
- `selfSovereign`
- `verified`
- `verifiedAt`
- `verifiedBy`
- `verifiedReason`
- `unverifiedAt`
- `unverifiedBy`
- `unverifiedReason`
- `keyRotatedAt`
- `createdAt`
- `updatedAt`

## Ownership Rules

- The core `USER#{username} / METADATA` row owns identity, profile, and non-governance agent fields such as `isAgent`, `agentType`, `agentVersion`, `agentOwner`, and `agentCapabilities`.
- The `USER#{username} / AGENT_GOVERNANCE` row owns quarantine, delegation envelope, self-sovereign governance, verification, and key-rotation governance state.
- Runtime code must not treat `User.Metadata` as an authoritative governance source after cutover.

## Repository Contract

`AccountRepository` is the runtime access boundary and exposes:

- `GetAgentGovernanceState(ctx, username)`
- `GetAgentGovernanceStatesByUsernames(ctx, usernames)`
- `PutAgentGovernanceState(ctx, state)`
- `DeleteAgentGovernanceState(ctx, username)`

Behavioral expectations:

- usernames are canonicalized to lowercase
- missing rows return `storage.ErrNotFound`
- writes normalize timestamps to UTC
- delegated and self-sovereign scopes are trimmed, deduplicated, and sorted
- updates preserve the original `CreatedAt`

## Service Contract

`pkg/services/accounts.Service` mirrors the repository accessors for runtime callers that should not reach into repositories directly.

## Migration Contract

- Existing live governance fields are backfilled from user metadata into `AGENT_GOVERNANCE`.
- Reader cutover happens only after parity is verified.
- Legacy governance keys are then removed from `User.Metadata`.

## Consequences

- governance-schema changes no longer threaten generic user/account hydration
- REST and GraphQL can read governance state through a typed contract instead of raw metadata decoding
- migration is required before legacy metadata can be removed from live rows

## References

- `docs/architecture/agent-governance-legacy-metadata-inventory.md`
- `pkg/storage/models/agent_governance_state.go`
- `pkg/storage/repositories/agent_governance_repository.go`
