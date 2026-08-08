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

- Add a folder argument to the existing `conversations` query so clients can request Inbox vs Requests without inventing parallel query shapes:
  - `enum ConversationFolder { INBOX, REQUESTS }`
  - `conversations(folder: ConversationFolder = INBOX, first: Int = 20, after: Cursor): [Conversation!]!`
  - `conversationConnection(folder: ConversationFolder = INBOX, first: Int = 20, after: Cursor): ConversationConnection!` (preferred paginated read)
- Add a dedicated messages query for thread views:
  - `conversationMessages(conversationId: ID!, first: Int = 50, after: Cursor): ObjectConnection!` (oldest-to-newest edge order)
- Optional but useful:
  - `messageRequestsCount: Int!`
  - `searchParticipants(query: String!, first: Int = 20, after: Cursor): ActorListPage!` (or a dedicated connection)

### 2. Per-viewer metadata

- `enum DmRequestState { PENDING, ACCEPTED, DECLINED }`.
- `type ConversationViewerMetadata { deletedAt: Time, requestState: DmRequestState!, lastSeenAt: Time }`.
- Add `viewerMetadata: ConversationViewerMetadata!` to `Conversation` and expose `requestState` on `Conversation` only when the user is the recipient. This mirrors the `ConversationParticipantRecord` additions described in ADR 0001/0002.

### 3. Privacy defaults

- Extend `PrivacyPreferences` with `directMessagesFrom: DirectMessagesFrom!`, where `DirectMessagesFrom` is `FOLLOWING_ONLY | ANYONE`.
- Update `UpdateUserPreferencesInput` to allow setting `directMessagesFrom`. The default value for new users should be `FOLLOWING_ONLY`, aligning with ADR 0001.

### 4. Mutations for request lifecycle and delete-for-me

- `createConversation(participantId: ID!): Conversation!` (v1: exactly one participant)
- `sendMessage(conversationId: ID!, content: String!, mediaIds: [ID!]): Object!` (or a dedicated `DirectMessage!`)
- `acceptMessageRequest(conversationId: ID!): Conversation!` – flips the recipient’s metadata to `ACCEPTED` and moves the thread to Inbox.
- `declineMessageRequest(conversationId: ID!): Boolean!` – marks the request `DECLINED` (and/or hides it for the viewer, per ADR 0002 and the chosen decline semantics).
- `deleteConversationForMe(conversationId: ID!): Boolean!` – sets `viewerMetadata.deletedAt` so the client can hide the conversation without affecting other participants (see ADR 0002).

## Impact and rollout notes

- These schema changes must land before the Greater client surfaces the “requests” tab; otherwise, the UI cannot distinguish inbox vs. pending threads.
- The stored contract (`docs/contracts/graphql-schema.graphql`) will change, so `greater-components`’ pinned schema must be regenerated and its `LESSER_REF.txt` updated (see the companion contract pin doc).
- Backend code (repositories, services, streaming/push helpers) must populate the new metadata so GraphQL can rely on it.

## Next steps

1. Implement the schema changes in `graph/*.graphql` and regenerate `docs/contracts/graphql-schema.graphql`.
2. Wire the new queries/mutations to the conversation service (respecting the DM policy, metadata, and delete-for-me semantics).
3. Push the updated contract to the `greater-components/docs/lesser/contracts` mirror and invoke the pin/regeneration workflow documented elsewhere.
