# ADR 0002: DM v1 per-recipient soft delete

**Status:** Proposed (2026-02-19)

## Context

- Users expect “delete for me” semantics in their DM clients: removing a conversation from their inbox without deleting the recipient’s copy or the federated ActivityPub records.
- The existing conversation model stores a single `Conversation` record plus participant records; there is currently no first-class per-viewer deletion state, so the only way to drop a conversation is to delete the entire record, which would remove it for everyone and break audit/federation.

## Decision

1. **Track per-participant deletion metadata in the participant record.** Each `ConversationParticipantRecord` will gain `deletedAt` (nullable timestamp) and `deletedFromInbox` (boolean) attributes. Marking a conversation “deleted for me” sets those fields rather than removing the conversation row.
2. **Keep the conversation canonical but hide it from the viewer.** When the viewer requests to “delete for me”, the service updates only their participant record; the main conversation entry and other participant records remain untouched so federation and moderation pipelines keep referenceable history.
3. **Expose the state via GraphQL and the new mutation.** A new mutation `deleteConversationForMe(conversationId: ID!): Boolean!` will drive this change, and the `Conversation` type will gain a `viewerDeletedAt` field so clients know whether to show “undo”/“show hidden” UI.
4. **Honor the deletion marker in inbox/request queries.** Queries that list conversations filter out any participant record with `deletedAt` set unless the user explicitly asks for archived conversations; for fairness, a “restore” path simply clears `deletedAt` and `deletedFromInbox`.

## Consequences

- Repository writes now need to carefully update only the participant record for the viewer, which means `ConversationParticipantRecord.BeforeCreate`/`Update` must populate the new keys and DynamoDB attribute names.
- We keep the underlying message data so that compliance investigations or federation retries can still read the conversation even after one participant hides it.
- Because the data is never deleted, we must consider periodic cleanup only when both participants have deleted the conversation and remove older metadata if necessary.

## Next steps

- Incorporate `deleteConversationForMe` into the GraphQL schema diff to guide the greater-components UI.
- Update streaming/push helpers to stop sending real-time notifications for conversations that are hidden by the viewer.

## References

- `pkg/storage/models/conversation.go` (`ConversationParticipantRecord`)
- `pkg/services/conversations/service.go`
