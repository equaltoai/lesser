# ADR 0008: Account core hydration contract

**Status:** Accepted (2026-03-27)

## Context

- `GetUser` and `GetAccount` sit on multiple hot paths, including DM send, notifications, relationships, API auth checks, and GraphQL account hydration.
- The DM verification failure showed that those calls were still vulnerable to optional extension drift because the core `USER#{username} / METADATA` row was hydrated as one monolithic payload.
- Agent governance has already moved to `USER#{username} / AGENT_GOVERNANCE`, but callers still need an explicit contract for which account fields are core and which fields are optional extensions.

## Decision

1. `GetUser` and `GetAccount` are the stable core-account primitives for runtime callers.
2. Core account hydration is defined as the following `USER` row fields:
   - identity and profile: `username`, `displayName`, `note`, `avatar`, `header`, `url`, `locked`, `discoverable`, `fields`
   - account state: `approved`, `suspended`, `silenced`, `role`, `locale`
   - recovery and safety preferences: `recoveryMethods`, `allowNSFW`, `requireNSFWWarning`
   - agent identity: `isAgent`, `agentType`, `agentCapabilities`, `agentVersion`, `agentOwner`, `agentCreatedBy`, `agentPublicKey`, `agentKeyType`
   - lifecycle/versioning: `createdAt`, `updatedAt`, `version`
3. `Metadata` is optional extension state.
   - `GetUser` and `GetAccount` must not fail because `Metadata` is missing or no longer decodes cleanly.
   - optional metadata may be omitted from the returned user when extension decoding fails.
4. Agent governance is not part of the core-account contract.
   - quarantine, delegation, verification, self-sovereign scopes, and key-rotation state must be read through typed governance accessors
   - API and GraphQL surfaces must not treat `User.Metadata` as a governance source
5. Registration-time wallet proof is not part of generic account metadata.
   - unauthenticated wallet-link completion must rely on typed `WalletChallenge` state
   - registration must not persist `registration_challenge_id` in `User.Metadata`
6. `GetAccount` keeps actor hydration as best effort.
   - a missing actor row still returns a usable account with `User` populated and `Actor == nil`
   - hot-path callers must consume only the account fields they actually need

## Consequences

- DM send and similar hot paths can rely on stable core user/account hydration without depending on extension-state decoding.
- future extension-schema changes in `Metadata` no longer take down core account reads.
- the account boundary now matches ADR 0007: governance lives behind typed rows and typed accessors instead of raw metadata maps.

## Verification

- repository regressions cover current live-shaped agent rows for `medic`, `arch`, and `pilot`
- DM send regressions cover account hydration preconditions against those fixtures
- `lesser verify account-hydration` can validate the deployed row shape against a live stage

## References

- [0007-agent-governance-state-contract.md](./0007-agent-governance-state-contract.md)
- [account_repository.go](../../../pkg/storage/repositories/account_repository.go)
- [service.go](../../../pkg/services/conversations/service.go)
