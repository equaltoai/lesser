# ADR 0010: Canonical persisted status note contract

**Status:** Accepted (2026-03-27)

## Context

- S0 established one canonical status-create contract, but the nested `Status.Note` payload still depended on framework struct marshaling.
- The DM outage showed that note persistence can still regress when different write modes or marshalers touch the same ActivityPub payload differently.
- `Status.Note` includes nested context, audience, tag, attachment, quote, and agent-attribution data that should be owned explicitly instead of relying on incidental reflection behavior.

## Decision

1. `pkg/storage/notecontract` owns the canonical persisted `Status.Note` representation.
2. The canonical persisted representation is a Dynamo-safe nested map that preserves the current stored note field layout:
   - `BaseObject`
   - `Content`
   - `AttributedTo`
   - `Attachment`
   - `Tag`
   - `ConversationID`
   - `Visibility`
   - `QuoteURL`
   - `Quoteable`
   - `QuoteNotifications`
   - `QuoteContext`
   - `AgentAttribution`
3. `pkg/storage/notecontract.Marshal` is the only supported way to turn an `activitypub.Note` into a persisted payload shape.
4. `pkg/storage/notecontract.Unmarshal` is the supported read boundary for hydrating stored note data back into `activitypub.Note`.
5. `pkg/storage/notecontract.Normalize` is the pre-persistence normalization step that status write paths must cross before the row is created.
6. `pkg/storage/theorydb` is responsible for wiring this contract into DynamoDB client serialization through a custom `activitypub.Note` converter.

## Consequences

- public note create, DM send, and any future status write path can share one explicit note serializer instead of relying on different framework marshalers.
- the nested persisted note shape is now testable without needing to infer behavior from raw reflection output.
- read compatibility can remain broader than write compatibility; the contract may accept legacy or ActivityPub JSON aliases while still writing one canonical stored shape.
- no immediate migration is required because the contract preserves the existing stored field layout rather than introducing a new top-level note schema.

## Verification

- package tests round-trip live-shaped public and DM note fixtures through `Marshal`, `Unmarshal`, and `Normalize`
- TheoryDB converter tests verify the canonical note converter is registered and round-trips the same fixtures
- repository and service contract tests compare persisted note output across public-note and DM entry points

## References

- [status-note-persistence-inventory.md](../status-note-persistence-inventory.md)
- [0009-status-create-contract.md](./0009-status-create-contract.md)
- [notecontract.go](../../../pkg/storage/notecontract/notecontract.go)
