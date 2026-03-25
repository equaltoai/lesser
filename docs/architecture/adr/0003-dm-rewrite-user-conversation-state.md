# ADR 0003: DM rewrite canonical user conversation state

**Status:** Accepted (2026-03-25)

## Context

- The current DM implementation mixes shared conversation metadata, embedded participant snapshots, legacy unread rows, and ad hoc list filtering.
- M0 needs one frozen per-user schema before M1 starts changing storage models and repository contracts.
- The follow-on milestones already depend on a stable answer to four questions:
  1. what the point-readable per-user DM row is called,
  2. which fields it owns,
  3. which keyed queries it must support,
  4. which attributes are index materialization only.

## Decision

1. **Adopt `UserConversationState` as the canonical per-user DM row name.** It is the only planned row type that may own viewer-specific DM state after the rewrite. The current `ConversationParticipantRecord` remains a legacy compatibility row until M1 replaces it.
2. **Give `UserConversationState` a stable point-readable primary key.** The target base-table key is:
   - `PK = USER_CONVERSATION_STATE#<viewerID>`
   - `SK = CONVERSATION#<conversationID>`
3. **Materialize keyed DM list access directly on the row.** The target index attributes are:
   - `gsi1PK = USER_CONVERSATION_FOLDER#<viewerID>#<folder>`
   - `gsi1SK = <sortAt RFC3339Nano>#<conversationID>`
   - `gsi2PK = USER_CONVERSATION_UNREAD#<viewerID>` when `Unread=true` and omitted otherwise
   - `gsi2SK = <sortAt RFC3339Nano>#<conversationID>`
   - `gsi3PK = CONVERSATION#<conversationID>`
   - `gsi3SK = USER#<viewerID>`
4. **Freeze the target field set now so later milestones do not re-litigate ownership.** The planned storage shape is:

```go
type UserConversationState struct {
    PK string // USER_CONVERSATION_STATE#<viewerID>
    SK string // CONVERSATION#<conversationID>

    GSI1PK string // USER_CONVERSATION_FOLDER#<viewerID>#<folder>
    GSI1SK string // <sortAt RFC3339Nano>#<conversationID>
    GSI2PK string // USER_CONVERSATION_UNREAD#<viewerID> when unread
    GSI2SK string // <sortAt RFC3339Nano>#<conversationID>
    GSI3PK string // CONVERSATION#<conversationID>
    GSI3SK string // USER#<viewerID>

    ViewerID                  string
    ConversationID            string
    CounterpartID             string
    Folder                    string
    RequestState              string
    PreviewStatusID           string
    PreviewStatusPublishedAt  time.Time
    SortAt                    time.Time
    Unread                    bool
    LastReadAt                *time.Time
    DeletedAt                 *time.Time
    RequestedAt               *time.Time
    AcceptedAt                *time.Time
    DeclinedAt                *time.Time
    CreatedAt                 time.Time
    UpdatedAt                 time.Time
}
```

5. **Treat index attributes as denormalized access paths, not independent sources of truth.**
   - `Folder`, `RequestState`, `Unread`, `PreviewStatusID`, `PreviewStatusPublishedAt`, `SortAt`, and `DeletedAt` are logical fields on `UserConversationState`.
   - `gsi1*`, `gsi2*`, and `gsi3*` exist only to support keyed queries over that same row.
6. **Keep the row intentionally 1:1 aware.** `CounterpartID` is required because the DM rewrite remains 1:1 only and later list APIs need a stable actor ID for batch enrichment without embedded snapshots.

## Consequences

- M1 can add the real storage model without reopening naming, key shape, or index design.
- M4 and M5 can target keyed folder and unread queries instead of scan-and-filter repository behavior.
- Preview data can become explicitly per-viewer without keeping a nested full conversation snapshot.
- Migration work can deterministically backfill one `UserConversationState` row per viewer per conversation.

## Next steps

- Define the exact meaning of `Folder`, `RequestState`, `DeletedAt`, and reopen transitions before write-path work begins.
- Map every DM field to its owning row type so preview, unread, request state, and thread state stop overlapping.
- Refactor repository contracts to point at `UserConversationState` point reads and keyed queries.

## References

- [0001-dm-v1-inbox-requests.md](./0001-dm-v1-inbox-requests.md)
- [0002-dm-v1-delete-for-me.md](./0002-dm-v1-delete-for-me.md)
- [dm-v1-graphql-schema-diff.md](../../planning/dm-v1-graphql-schema-diff.md)
- [pkg/storage/models/conversation.go](../../../pkg/storage/models/conversation.go)
- [pkg/storage/interfaces/conversation.go](../../../pkg/storage/interfaces/conversation.go)
