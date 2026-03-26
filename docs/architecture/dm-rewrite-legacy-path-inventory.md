# DM Rewrite Legacy Path Inventory

Date: 2026-03-25

This inventory marks the legacy DM storage shapes and scan-based runtime paths that the rewrite must remove once the canonical `UserConversationState` model is in place.

## Legacy row types to retire

### Embedded snapshot compatibility

- `pkg/storage/models/conversation.go`
  - `ConversationSnapshot`
  - `ConversationParticipantRecord`
  - `(*ConversationParticipantRecord).HydrateConversation`
  - `(*ConversationParticipantRecord).SyncConversationData`
- Why it goes away:
  - embeds a full conversation snapshot inside per-user rows
  - makes DM list correctness depend on snapshot hydration rather than canonical per-user state
  - keeps preview and unread state coupled to copied conversation metadata

### Legacy unread/read-state row

- `pkg/storage/models/conversation_status.go`
  - `ConversationStatus`
- Why it goes away:
  - duplicates unread/read truth that ADRs 0003-0005 assign to `UserConversationState`
  - forces read flows to touch a separate compatibility row

### Legacy conversation-message row

- `pkg/storage/models/conversation_status.go`
  - `ConversationMessage`
- Why it goes away:
  - duplicates thread membership already carried by direct `Status` rows via `ConversationID`
  - creates a partial DM write path that the rewrite is explicitly removing

## Scan and snapshot runtime paths to replace

### Conversation repository

- `pkg/storage/repositories/conversation_repository.go`
  - `GetUserConversations`
  - `GetUserConversationsByRequestState`
  - `GetUnreadConversations`
  - `scanUserConversationsByRequestState`
  - `fetchUserConversationParticipantRecords`
  - `GetUnreadConversationCount`
  - `GetConversationStatuses`
  - `RemoveStatusFromConversation`
- Why they go away:
  - they load participant snapshot rows and filter in Go instead of querying keyed canonical DM state
  - unread listing still fans out through `GetUserConversations` plus legacy unread rows instead of querying a sparse unread index
  - unread counting fans out across legacy unread rows
  - thread reads and message mutation still assume `ConversationMessage`

### Timeline compatibility bridge

- `pkg/storage/repositories/timeline_repository.go`
  - `GetConversations`
- Why it goes away:
  - reads DM lists by hydrating embedded `ConversationParticipantRecord` snapshots
  - duplicates list logic outside the canonical DM repository path

## Transitional tooling to remove after migration and cutover

### Snapshot repair and legacy backfills

- `cmd/lesser/migrate_conversation_participant_snapshots.go`
- `cmd/lesser/migrate_conversation_metadata.go`
- `cmd/lesser/migrate_conversations.go`
- `tools/dm_conversation_backfill/main.go`

These stay only long enough to support migration, corruption repair, and cutover verification. Once the canonical `UserConversationState` model is backfilled and the live code no longer reads legacy snapshot rows, these tools become dead compatibility code.

## Rewrite targets by milestone

- M1:
  - remove embedded snapshot dependence
  - replace participant snapshot rows with real `UserConversationState`
  - replace snapshot-shaped repository contracts
- M4:
  - move read/unread and request transitions onto canonical per-user state
- M5:
  - replace scan-shaped DM list paths with keyed folder/unread queries
  - keep thread reads on `StatusRepository.GetConversationThread`
- M8:
  - remove all remaining legacy DM compatibility glue, row types, and scan paths

## Source ADRs

- `docs/architecture/adr/0003-dm-rewrite-user-conversation-state.md`
- `docs/architecture/adr/0004-dm-rewrite-viewer-state-semantics.md`
- `docs/architecture/adr/0005-dm-rewrite-row-ownership.md`
- `docs/architecture/adr/0006-dm-rewrite-repository-contract.md`
