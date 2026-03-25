# ADR 0004: DM rewrite viewer state semantics

**Status:** Accepted (2026-03-25)

## Context

- Lesser already has partial DM v1 behavior in `pkg/services/conversations/service.go`, but the semantics are still split across request-state checks, `DeletedAt` handling, and older ADRs.
- M3 and M4 need one authoritative lifecycle model for inbox, requests, decline, hidden, and delete-for-me behavior.
- The target `UserConversationState` row from ADR 0003 needs exact viewer-visible meanings before transactional write paths can be rewritten.

## Decision

1. **Use `Folder` for viewer-visible placement and `RequestState` for viewer consent state.**
   - `Folder=INBOX` means the thread is visible in the default DM list.
   - `Folder=REQUESTS` means the thread is awaiting the viewer's acceptance.
   - `Folder=DECLINED` means the viewer explicitly declined the inbound request.
   - `Folder=HIDDEN` means the thread is hidden for this viewer via delete-for-me or equivalent archive behavior.
2. **Keep the sender side visible and accepted by default.**
   - The sender's `UserConversationState` is always `RequestState=ACCEPTED`.
   - The sender's row may still become `Folder=HIDDEN` if they delete the conversation for themselves.
   - The sender never has a `REQUESTS` or `DECLINED` folder for their own outbound DM row.
3. **Define recipient-side request semantics exactly once.**
   - A new inbound DM lands in `Folder=INBOX` with `RequestState=ACCEPTED` when policy allows direct delivery.
   - A new inbound DM lands in `Folder=REQUESTS` with `RequestState=PENDING` when policy requires consent.
   - Accepting a request moves the row to `Folder=INBOX`, sets `RequestState=ACCEPTED`, and stamps `AcceptedAt`.
   - Declining a request moves the row to `Folder=DECLINED`, sets `RequestState=DECLINED`, and stamps `DeclinedAt`.
4. **Treat `DECLINED` and `HIDDEN` as distinct states.**
   - `DECLINED` records an explicit viewer decision about inbound consent.
   - `HIDDEN` records viewer-specific list suppression and does not erase the last consent decision.
   - A declined row is not returned by default inbox or requests queries.
   - A hidden row is not returned by default inbox, requests, or unread queries.
5. **Define delete-for-me as a one-row viewer-only mutation.**
   - Delete-for-me sets `DeletedAt` and moves the viewer row to `Folder=HIDDEN`.
   - It never deletes the shared conversation row, DM `Status` rows, or the counterpart's `UserConversationState`.
   - Restoring a hidden row clears `DeletedAt` and recomputes `Folder` from the retained consent state.
6. **Define reopen behavior for future inbound DMs.**
   - A new inbound DM to a hidden accepted thread clears `DeletedAt`, moves the viewer row back to `Folder=INBOX`, and marks it unread.
   - A new inbound DM to a declined thread reopens as a fresh request: clear `DeclinedAt`, set `RequestState=PENDING`, set `RequestedAt` to the new inbound time, move the row to `Folder=REQUESTS`, and mark it unread.
   - A pending request remains a single outstanding consent decision; later write-path work must not append unlimited pending-request spam to the viewer's visible inbox.
7. **Keep unread as a secondary view, not a competing lifecycle.**
   - `Unread=true` means the viewer has unseen thread activity.
   - Unread does not override `Folder`; it only provides a sparse query over visible per-user state.

## Consequences

- M3 can encode send-time request transitions without guessing how hidden vs declined should behave.
- M4 can implement accept, decline, unread, and delete-for-me as direct mutations of one viewer row.
- The UI contract can explain why a thread is visible, hidden, or waiting on consent without inferring semantics from partial fields.
- Abuse resistance remains compatible with the lifecycle because a pending request is a first-class state rather than an emergent filter.

## Next steps

- Map each lifecycle field to its owning row type so the viewer contract does not overlap with shared conversation metadata.
- Refactor repository contracts so folder and unread queries target keyed `UserConversationState` access paths.

## References

- [0001-dm-v1-inbox-requests.md](./0001-dm-v1-inbox-requests.md)
- [0002-dm-v1-delete-for-me.md](./0002-dm-v1-delete-for-me.md)
- [0003-dm-rewrite-user-conversation-state.md](./0003-dm-rewrite-user-conversation-state.md)
- [security-dm-threat-model.md](../../security-dm-threat-model.md)
- [pkg/services/conversations/service.go](../../../pkg/services/conversations/service.go)
