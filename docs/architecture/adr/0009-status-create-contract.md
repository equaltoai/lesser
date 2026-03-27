# ADR 0009: Canonical status-create contract

**Status:** Accepted (2026-03-27)

## Context

- first-party note creation had split into two ownership models:
  - public notes and boosts used `pkg/storage/repositories/StatusRepository`
  - DM send used `ConversationRepository.ApplyDirectMessageSend` with a raw transactional `tx.Create(status)`
- the DM-only path already diverged from canonical behavior and failed on the current live `Note.Context` shape.
- S0 needs one explicit contract that keeps status-row ownership centralized while still allowing DM send to share a transaction with conversation metadata and per-viewer state rows.

## Decision

1. `pkg/storage/repositories/StatusRepository` is the sole owner of first-party `Status` row creation.
2. The public status-create contract is `pkg/storage/interfaces.CanonicalStatusCreateRepository`:
   - `CreateStatus(ctx, status)`
   - `PrepareStatusCreate(status)`
   - `StageStatusCreate(tx, status)`
   - `FinalizeCreatedStatus(ctx, status)`
3. Callers that only need a status row must use `CreateStatus`.
4. Callers that also own companion transactional writes may only integrate through the staged contract:
   - call `PrepareStatusCreate` once before the transaction
   - pass `StageStatusCreate` into the transaction-owning repository
   - call `FinalizeCreatedStatus` only after the transaction commits
5. `ConversationRepository` owns conversation metadata and `UserConversationState` writes, but it does not own status creation.
   - it may stage a caller-supplied status write inside the DM transaction
   - it may not call `tx.Create(status)` or otherwise create `Status` rows directly
6. Status-create side effects are split into two buckets:
   - required: persist the prepared `Status` row itself
   - best-effort post-create: instance metrics, canonical index normalization, and supplemental hashtag timeline index rows

## Consequences

- public notes, boosts, new-thread DMs, and existing-thread DMs can share one canonical status persistence contract.
- DM orchestration keeps transaction ownership for conversation/state rows without reintroducing an alternate status-write escape hatch.
- future first-party note flows have a narrower allowed integration seam, so bypassing canonical status creation becomes structurally harder.

## Verification

- repository transaction tests assert DM send stages status writes through a caller-supplied callback instead of a repository-owned raw create
- conversation service tests keep DM send behavior aligned across transactional and non-transactional paths
- the live-stack verifier covers public note create, DM create-and-send, and DM send-to-existing-thread against one deployed table

## References

- [0006-dm-rewrite-repository-contract.md](./0006-dm-rewrite-repository-contract.md)
- [status-create-entry-point-inventory.md](../status-create-entry-point-inventory.md)
- [status_create_contract.go](../../../pkg/storage/interfaces/status_create_contract.go)
- [status_repository.go](../../../pkg/storage/repositories/status_repository.go)
- [conversation_send_repository.go](../../../pkg/storage/repositories/conversation_send_repository.go)
