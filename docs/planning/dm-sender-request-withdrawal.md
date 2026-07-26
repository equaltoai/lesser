# DM sender request withdrawal

**Status:** Design only — not implemented in lesser#1267

## Verdict

The current DM lifecycle does not have a safe sender-side cancel/withdraw operation. Adding one is not a handler-only
change:

- `pkg/storage/models/conversation.go` defines only `PENDING`, `ACCEPTED`, and `DECLINED` request states.
- `pkg/services/conversations/service.go` applies accept/decline decisions to the authenticated recipient's
  `UserConversationState`.
- `DeleteConversation` is deliberately delete-for-me: it hides only the caller's row and leaves the counterpart's
  pending request intact.
- A sender cannot reuse accept/decline safely because those operations do not establish which participant originated
  the pending request, and treating a withdrawal as a decline would record a recipient decision that never happened.

Overloading `deleteConversation` or `declineMessageRequest` would therefore change existing semantics and could let a
sender bypass the pending-request anti-spam gate. Lesser#1267 only corrects recipient resolution and does not add that
contract or state transition.

## Proposed follow-up contract

Add a Lesser-specific GraphQL mutation rather than changing a Mastodon-compatible REST endpoint:

```graphql
withdrawMessageRequest(conversationId: ID!): Boolean!
```

The operation must:

1. Require an authenticated participant in a 1:1 conversation.
2. Establish that the caller is the originator of the currently pending request and that the counterpart's canonical
   `UserConversationState` is still `PENDING`.
3. Reject recipient calls, accepted/declined requests, non-1:1 conversations, and stale transitions.
4. Apply the withdrawal conditionally so a concurrent recipient accept/decline and sender withdrawal cannot both win.
5. Emit a metadata-only `dm.request.withdraw` audit event.

This mutation is local request-lifecycle control, not message recall. It must not claim that already delivered or
federated ActivityPub content was erased.

## Required lifecycle and schema work

The canonical state needs an explicit transition that cannot be confused with recipient decline or delete-for-me:

- add `DmRequestStateWithdrawn`;
- add a non-query-visible `UserConversationFolderWithdrawn`;
- add `WithdrawnAt` to `UserConversationState`;
- clear `RequestedAt` and unread-index materialization when withdrawal succeeds;
- keep the sender row accepted, preserving ADR 0004's sender-state invariant;
- keep shared conversation and `Status` rows for audit and federation history.

The next send to the same counterpart may create a fresh request only through the normal request-rate-limit path. It
must not reuse the withdrawn request or bypass the one-message pending-request rule. A follow-up design should decide
whether the existing per-recipient window is sufficient or whether withdrawal also starts a dedicated cooldown.

Remote recipients do not have a local counterpart `UserConversationState`; remote-message recall or an ActivityPub
`Delete`/`Undo` convention is a separate federation design and is out of scope for this mutation.

## Follow-up gates

A future implementation requires:

- `validate-schema` for the new state, folder, timestamp, index materialization, and migration behavior;
- `preserve-mastodon-api-compat` for the additive GraphQL contract and regenerated
  `docs/contracts/graphql-schema.graphql`;
- concurrency tests for withdraw-vs-accept/decline;
- abuse tests proving cancel/resend cannot bypass pending-request or per-recipient rate limits;
- authorization tests for sender, recipient, non-participant, and remote-recipient cases.
