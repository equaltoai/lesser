# Incomplete Implementations Checklist

Use this checklist to drive remediation. Each item includes the concrete follow-up needed so we can track progress methodically.

## Activity Processor

- [x] Implement full ActivityPub “undo reject” handling (`cmd/activity-processor/handler.go:1298`):
  - [x] Validate actor permissions and resolve the referenced activity.
  - [x] Revert stored state (follows, blocks, etc.) as appropriate.
  - [x] Emit any required federation or streaming events.
  - [x] Backfill unit/integration coverage for the new path.

## Storage Patterns

- [x] Replace soft delete logging stubs with real persistence in `pkg/storage/dynamorm/patterns/soft_delete.go`:
  - [x] `SoftDelete` updates the record via DynamORM.
  - [x] `Restore` persists the undelete mutation.
  - [x] `HardDelete` issues a true delete against DynamoDB.
  - [x] `CleanupOldDeletes` finds and removes stale soft-deleted rows.
  - [x] `GetDeletedItemsOlderThan` returns the identified records.
  - [x] Add regression tests exercising these flows.

- [x] Wire `pkg/storage/dynamorm/migrations/gsi_helpers.go` into a real migration repository:
  - [x] Persist migration records in `CreateGSI`.
  - [x] Update records in `DeleteGSI`.
  - [x] Read actual status in `GetGSIStatus`.
  - [x] Return stored rows in `ListGSIMigrations`.
  - [x] Add unit coverage validating the helper against the chosen repository.

## OAuth Client Pagination

- [x] Design and document a cursor-based strategy for listing OAuth clients (likely a GSI).
  - Dedicated `oauth-clients-index` stores all clients under `PK=OAUTH_CLIENTS` with `SK=CREATED_AT#{desc_ts}#CLIENT#{client_id}` to provide deterministic newest-first ordering.
- [x] Implement the design in `pkg/storage/repositories/account_repository_oauth.go:418`.
- [x] Add tests proving deterministic pagination and cursor semantics.

## Comment Clean-Up (Pagination Already Implemented)

Remove or rewrite misleading TODO-style comments. Once updated, adjust the audit script so these files no longer surface as gaps.

- [x] `pkg/storage/repositories/announcement_repository.go:273`
- [x] `pkg/storage/repositories/announcement_repository.go:350`
- [x] `pkg/storage/repositories/base_repository.go:723`
- [x] `pkg/storage/repositories/dlq_repository.go:478`
- [x] `pkg/storage/repositories/federation_instance_repository.go:774`
- [x] `pkg/storage/repositories/federation_repository.go:2955`
- [x] `pkg/storage/repositories/federation_repository_phase3_test.go:133`
- [x] `pkg/storage/repositories/hashtag_repository.go:551`
- [x] `pkg/storage/repositories/hashtag_repository.go:596`
- [x] `pkg/storage/repositories/list_repository.go:161`
- [x] `pkg/storage/repositories/list_repository.go:453`
- [x] `pkg/storage/repositories/media_repository.go:1080`
- [x] `pkg/storage/repositories/media_repository.go:1125`
- [x] `pkg/storage/repositories/media_repository.go:1185`
- [x] `pkg/storage/repositories/notification_helpers.go:53`
- [x] `pkg/storage/repositories/notification_repository.go:117`
- [x] `pkg/storage/repositories/notification_repository.go:167`
- [x] `pkg/storage/repositories/notification_repository.go:220`
- [x] `pkg/storage/repositories/notification_repository.go:434`
- [x] `pkg/storage/repositories/relay_repository.go:173`
- [x] `pkg/storage/repositories/scheduled_status_repository.go:168`
- [x] `pkg/storage/repositories/search_repository.go:1066`
- [x] `pkg/storage/repositories/social_repository.go:728`
- [x] `pkg/storage/repositories/social_repository.go:955`
- [x] `pkg/storage/repositories/status_repository.go:678`
- [x] `pkg/storage/repositories/timeline_repository.go:87`
- [x] `pkg/storage/repositories/timeline_repository.go:380`
- [x] `pkg/storage/repositories/user_repository.go:1191`
- [x] `pkg/storage/repositories/user_repository.go:1228`
- [x] `pkg/storage/repositories/user_repository.go:2719`
- [x] `pkg/storage/repositories/utils.go:211`

- [ ] Update `scripts/check_implementation_status.sh` filters once comments are resolved so it ignores intentional scaffolding (e.g., mocks) and no longer surfaces completed pagination support.

## Verification

- [ ] Re-run `bash scripts/check_implementation_status.sh`.
- [ ] Confirm `INCOMPLETE_IMPLEMENTATIONS.md` only contains outstanding work.
- [ ] Capture test evidence after each functional fix.
