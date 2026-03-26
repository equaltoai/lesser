# ADR 0005: DM rewrite row ownership

**Status:** Accepted (2026-03-25)

## Context

- The current DM code still spreads meaning across `Conversation`, `ConversationParticipantRecord`, `ConversationStatus`, `ConversationMessage`, and direct `Status` rows.
- M0 must stop that overlap before M1-M8 rewrite work starts, or each milestone will keep re-encoding unread, preview, and thread state in multiple places.
- The filesystem already shows the intended future split:
  - `pkg/storage/models/status.go` stores direct DM content on normal `Status` rows with `ConversationID`, recipients, mentions, and `VisibilityDirect`.
  - `pkg/services/conversations/service.go` already treats request state and delete-for-me as per-viewer metadata.

## Decision

1. **Shared conversation metadata belongs only to the shared `Conversation` row.**
   - Owner: `Conversation`
   - Fields:
     - conversation identity (`ID`)
     - canonical participant set for the 1:1 thread
     - shared timestamps (`CreatedAt`, shared `UpdatedAt`)
     - shared last-message metadata (`LastStatusID`, `LastMessageTime`, `TotalMessageCount`)
   - Non-owners:
     - unread
     - request state
     - viewer folder placement
     - viewer preview state
     - delete-for-me state
2. **Exact-participant lookup belongs only to the lookup row.**
   - Owner: `ConversationParticipantKey`
   - Fields:
     - sorted participant lookup key
     - resolved `ConversationID`
   - This row exists only to answer "do these exact participants already have a thread?"
3. **All viewer-specific DM state belongs only to `UserConversationState`.**
   - Owner: `UserConversationState`
   - Fields:
     - viewer identity (`ViewerID`)
     - counterpart actor identity (`CounterpartID`)
     - visible folder placement (`Folder`)
     - request lifecycle (`RequestState`, `RequestedAt`, `AcceptedAt`, `DeclinedAt`)
     - unread lifecycle (`Unread`, `LastReadAt`)
     - preview lifecycle (`PreviewStatusID`, `PreviewStatusPublishedAt`, `SortAt`)
     - delete-for-me / archive state (`DeletedAt`)
     - viewer-row timestamps (`CreatedAt`, viewer `UpdatedAt`)
4. **Thread truth belongs only to direct `Status` rows.**
   - Owner: `Status`
   - Fields:
     - DM body and note payload (`Note`, `Content`)
     - sender identity (`AuthorID`, `AuthorUsername`)
     - thread membership (`ConversationID`)
     - reply chain (`InReplyToID`)
     - recipients and privacy (`ToRecipients`, `CcRecipients`, `BtoRecipients`, `BccRecipients`, `Visibility`)
     - mentions, attachments, and published ordering (`Mentions`, `PublishedAt`)
   - Thread reads stay on the status thread index keyed by `ConversationID`; they do not move to a separate DM message row type.
5. **Legacy DM helper rows are explicitly non-canonical.**
   - `ConversationParticipantRecord` is a temporary compatibility row and not the target owner of per-user DM state.
   - `ConversationSnapshot` is compatibility payload only and not the target owner of preview or unread truth.
   - `ConversationStatus` is not the future owner of unread truth.
   - `ConversationMessage` is not the future owner of thread truth.

## Ownership Matrix

| Field family | Canonical owner after rewrite | Explicit non-owners |
| --- | --- | --- |
| Conversation identity and participant pair | `Conversation`, `ConversationParticipantKey` | `UserConversationState`, `ConversationStatus`, `ConversationMessage` |
| Inbox/requests/declined/hidden placement | `UserConversationState` | `Conversation`, `ConversationStatus`, `ConversationMessage` |
| Request consent state | `UserConversationState` | `Conversation`, `ConversationStatus`, `ConversationMessage` |
| Unread state | `UserConversationState` | `Conversation`, `ConversationStatus` |
| DM list preview pointer and ordering | `UserConversationState` | `Conversation`, embedded snapshots |
| Thread body, recipients, mentions, reply chain | `Status` | `ConversationMessage`, `ConversationStatus`, embedded snapshots |
| Shared last-message metadata | `Conversation` | `UserConversationState`, `ConversationMessage` |

## Consequences

- Later milestones can delete legacy unread and conversation-message rows without leaving ownership gaps.
- DM list APIs can derive responses from `UserConversationState` plus batch-loaded `Status` and actor data instead of embedded snapshots.
- Send, accept, decline, read, and delete-for-me flows can target one viewer row and one shared conversation row, while thread reads stay on `Status`.

## Next steps

- Update repository contracts so the read layer matches this ownership split.
- Mark the legacy snapshot and scan-based paths that must disappear once the new row types land.

## References

- [0003-dm-rewrite-user-conversation-state.md](./0003-dm-rewrite-user-conversation-state.md)
- [0004-dm-rewrite-viewer-state-semantics.md](./0004-dm-rewrite-viewer-state-semantics.md)
- [pkg/storage/models/conversation.go](../../../pkg/storage/models/conversation.go)
- [pkg/storage/models/conversation_status.go](../../../pkg/storage/models/conversation_status.go)
- [pkg/storage/models/status.go](../../../pkg/storage/models/status.go)
- [pkg/services/conversations/service.go](../../../pkg/services/conversations/service.go)
