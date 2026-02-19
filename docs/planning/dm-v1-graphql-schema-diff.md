# DM v1 GraphQL schema diff (1:1, inbox/requests, following-only default, delete-for-me)

## Summary

Lesser’s current GraphQL contract (`docs/contracts/graphql-schema.graphql`) exposes the bare `Conversation` type plus `conversations`/`conversation` queries and very limited mutation support (`markConversationAsRead`, `deleteConversation`). DM v1 requires:

1. Explicit inbox vs. requests lists.
2. Per-viewer metadata so UI clients can hide conversations they deleted or show why a request is pending.
3. A default privacy policy (`FOLLOWING_ONLY`) plus the ability to accept/decline requests.
4. A “delete-for-me” mutation that hides a conversation without removing it for other participants.

This diff doc enumerates the schema extensions needed to deliver those requirements.

## Baseline (current contract)

- `type Conversation { id, lastStatus, unread, accounts, createdAt, updatedAt }` (`docs/contracts/graphql-schema.graphql`, lines 682‑703).
- Queries `conversations` and `conversation` return the stored conversation records.
- Mutations touch only read/unread state or full deletion; no request lifecycle or per-viewer metadata is surfaced.
- `PrivacyPreferences` exists but has no DM-specific fields (see `graph/phase3.graphql`, lines ~660‑720).

## Proposed additions

### 1. Inbox + requests queries

- `dmInbox(first: Int = 20, after: Cursor): ConversationConnection!` – filters participant metadata to `ACCEPTED` requests and hides conversations the viewer deleted. Reuse the existing `Conversation` fields plus a new `ConversationConnection` for pagination if needed.
- `dmRequests(first: Int = 20, after: Cursor): DirectMessageRequestConnection!` – returns pending request metadata (`from`, `conversation`, timestamps) so the client can present them separately from the inbox.

### 2. Per-viewer metadata

- `enum DmRequestState { PENDING, ACCEPTED, DECLINED }`.
- `type ConversationViewerMetadata { deletedAt: Time, requestState: DmRequestState!, lastSeenAt: Time }`.
- Add `viewerMetadata: ConversationViewerMetadata!` to `Conversation` and expose `requestState` on `Conversation` only when the user is the recipient. This mirrors the `ConversationParticipantRecord` additions described in ADR 0001/0002.

### 3. Privacy defaults

- Extend `PrivacyPreferences` with `dmPolicy: DmPolicy!`, where `DmPolicy` is `FOLLOWING_ONLY | ANYONE`.
- Update `UpdateUserPreferencesInput` to allow setting `dmPolicy`. The default value for new users should be `FOLLOWING_ONLY`, aligning with the DM policy in ADR 0001.

### 4. Mutations for request lifecycle and delete-for-me

- `acceptDirectMessageRequest(requestId: ID!): Conversation!` – flips the recipient’s metadata to `ACCEPTED`, publishes the conversation to the inbox, and triggers delivery for any queued messages.
- `declineDirectMessageRequest(requestId: ID!): Boolean!` – marks the request `DECLINED`.
- `deleteConversationForMe(conversationId: ID!): Boolean!` – sets `viewerMetadata.deletedAt` so the client can hide the conversation without affecting other participants (see ADR 0002).

## Impact and rollout notes

- These schema changes must land before the Greater client surfaces the “requests” tab; otherwise, the UI cannot distinguish inbox vs. pending threads.
- The stored contract (`docs/contracts/graphql-schema.graphql`) will change, so `greater-components`’ pinned schema must be regenerated and its `LESSER_REF.txt` updated (see the companion contract pin doc).
- Backend code (repositories, services, streaming/push helpers) must populate the new metadata so GraphQL can rely on it.

## Next steps

1. Implement the schema changes in `graph/*.graphql` and regenerate `docs/contracts/graphql-schema.graphql`.
2. Wire the new queries/mutations to the conversation service (respecting the DM policy, metadata, and delete-for-me semantics).
3. Push the updated contract to the `greater-components/docs/lesser/contracts` mirror and invoke the pin/regeneration workflow documented elsewhere.
