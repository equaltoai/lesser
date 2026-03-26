# ADR 0006: DM rewrite repository contract

**Status:** Accepted (2026-03-25)

## Context

- `pkg/storage/interfaces/conversation.go` still exposes DM-facing methods shaped around snapshot hydration, request-state scan filtering, and legacy unread/message rows.
- ADRs 0003-0005 already freeze the target per-user state row, lifecycle semantics, and ownership split.
- M1 and M5 need the repository contract to describe the target read model now, even if the live implementation stays on the legacy surface until follow-on milestones land.

## Decision

1. **Introduce a target DM repository contract beside the legacy conversation repository.**
   - `pkg/storage/interfaces/direct_message_contract.go` defines:
     - `UserConversationFolder`
     - `UserConversationStateContract`
     - `DirectMessageRepository`
   - This is the authoritative M0 contract for canonical per-user DM state reads.
2. **Make the target contract point-read and keyed-query based.**
   - Required methods:
     - `GetUserConversationState`
     - `ListUserConversationStatesByFolder`
     - `ListUnreadUserConversationStates`
     - `ListConversationParticipantStates`
   - Shared thread identity stays on:
     - `GetConversation`
     - `GetConversationByParticipants`
3. **Do not carry scan-shaped DM list behavior into the target contract.**
   - The target contract has no equivalent of `GetUserConversations` that implies scan-side filtering.
   - The target contract has no equivalent of `GetUserConversationsByRequestState` that filters request state after loading snapshot rows.
4. **Keep DM thread reads on `StatusRepository`, not the DM repository.**
   - The target contract does not define a conversation-message API because DM thread truth remains on `Status` rows keyed by `ConversationID`.
   - This aligns the repository contract with ADR 0005 ownership rules.
5. **Mark the current conversation interface as legacy in code.**
   - `pkg/storage/interfaces/conversation.go` now calls out snapshot/list/unread/message methods as legacy rewrite targets.
   - This preserves current compile-time behavior while making the intended replacement explicit to future milestone work.

## Consequences

- M1 can implement canonical per-user state reads without reinterpreting which interface shape was intended.
- M4 and M5 can target folder and unread keyed queries directly.
- The codebase now distinguishes "current runtime surface" from "approved rewrite contract," which reduces accidental expansion of the legacy snapshot API.

## Next steps

- Implement the real `UserConversationState` storage model so the target contract can be backed by concrete rows.
- Remove legacy scan and snapshot list methods once M1/M5 migration work is complete.

## References

- [0003-dm-rewrite-user-conversation-state.md](./0003-dm-rewrite-user-conversation-state.md)
- [0004-dm-rewrite-viewer-state-semantics.md](./0004-dm-rewrite-viewer-state-semantics.md)
- [0005-dm-rewrite-row-ownership.md](./0005-dm-rewrite-row-ownership.md)
- [pkg/storage/interfaces/direct_message_contract.go](../../../pkg/storage/interfaces/direct_message_contract.go)
- [pkg/storage/interfaces/conversation.go](../../../pkg/storage/interfaces/conversation.go)
