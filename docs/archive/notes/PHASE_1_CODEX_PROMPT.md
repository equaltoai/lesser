# Codex Phase 1 Prompt: Bookmark Model Dual-Write Pattern

You are implementing **Phase 1** of the Viewer Engagement Batch Pattern project. Reference the full plan at `docs/architecture/dynamodb/VIEWER_ENGAGEMENT_BATCH_PATTERN.md`.

## Context
We need to modify the Bookmark model to support efficient batch lookups for GraphQL timeline queries. Currently, bookmarks use `SK: {timestamp}#{object_id}`, which prevents BatchGetItem operations since we can't construct the exact key without knowing the timestamp.

## Solution: Dual-Write Pattern with Locking
Write **two records** per bookmark to support both access patterns:
1. **TIME# record**: For chronological listing of user's bookmarks (written first with `Locked: true`)
2. **OBJECT# record**: For batch "did user bookmark these N statuses?" lookups (stores TIME# SK, unlocks TIME# record on success)

**Consistency Guarantee**: The TIME# record is created with a `Locked` flag to prevent concurrent writes. Once the OBJECT# record successfully writes, an atomic update unlocks the TIME# record. This ensures both records are always in sync.

## Your Tasks (Phase 1)

### Task 1.1: Update Bookmark Model
**File**: `pkg/storage/models/bookmark.go`

**Requirements**:
- Add `RecordType` string field to distinguish "TIME" vs "OBJECT" records
- Add `TimeRecordSK` string field to store the TIME# SK on OBJECT# records (for deletion)
- Add `Locked` boolean field to prevent concurrent writes during dual-write operation
- Modify `UpdateKeys()` method to accept a `recordType` parameter
- Create two factory methods:
  - `NewTimeOrderedBookmark(username, objectID, createdAt)` → generates `SK: TIME#{timestamp}#{objectID}`, sets `Locked: true`
  - `NewObjectIndexedBookmark(username, objectID, createdAt, timeRecordSK)` → generates `SK: OBJECT#{objectID}` and stores `TimeRecordSK`
- Both factories should set the same `PK: BOOKMARK#{username}`
- Add clear documentation explaining the dual-write pattern with locking mechanism

**Validation**:
- Both record types must have identical data fields (Username, ObjectID, CreatedAt)
- TIME# record must start with `Locked: true`
- OBJECT# record must store TimeRecordSK pointing to the TIME# record's SK
- Keys must be deterministic and idempotent
- Write unit tests in `pkg/storage/models/bookmark_test.go`

---

### Task 1.2: Update BookmarkRepository Create Method
**File**: `pkg/storage/repositories/bookmark_repository.go`

**Requirements**:
- Modify `CreateBookmark(ctx, username, objectID)` to implement three-phase atomic write:
  1. **Phase 1**: Create TIME# record with `Locked: true` and condition expression `attribute_not_exists(PK)`
  2. **Phase 2**: Create OBJECT# record with `TimeRecordSK` reference
  3. **Phase 3**: Unlock TIME# record by setting `Locked: false` (atomic update with condition `Locked = true`)
- Use DynamoDB `TransactWriteItems` for phases 1+2, then atomic update for phase 3
- Handle duplicate detection: if Phase 1 fails with condition check, bookmark already exists (idempotent)
- If Phase 2 fails, Phase 3 cleanup unlocks the TIME# record (orphaned locked record recovery)
- Update logging to show all three phases

**Example Logic**:
```go
func (r *BookmarkRepository) CreateBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
    now := time.Now()
    
    // Phase 1: Create TIME# record with lock
    timeRecord := models.NewTimeOrderedBookmark(username, objectID, now)
    // timeRecord.Locked = true (set by factory)
    
    err := r.db.WithContext(ctx).Model(timeRecord).
        Condition("attribute_not_exists(PK)").  // Prevent duplicates
        Create()
    if err != nil {
        if errors.IsConditionFailed(err) {
            // Bookmark already exists - idempotent
            return r.GetBookmark(ctx, username, objectID)
        }
        return nil, err
    }
    
    // Phase 2: Create OBJECT# record with reference to TIME# SK
    objectRecord := models.NewObjectIndexedBookmark(username, objectID, now, timeRecord.SK)
    
    err = r.db.WithContext(ctx).Model(objectRecord).
        Condition("attribute_not_exists(PK)").
        Create()
    if err != nil {
        // Rollback: unlock TIME# record (allow future attempts)
        r.unlockTimeRecord(ctx, timeRecord.PK, timeRecord.SK)
        return nil, err
    }
    
    // Phase 3: Unlock TIME# record (atomic update)
    err = r.db.WithContext(ctx).Model(&models.Bookmark{}).
        Where("PK", "=", timeRecord.PK).
        Where("SK", "=", timeRecord.SK).
        UpdateBuilder().
        Set("Locked", false).
        Condition("Locked", "=", true).  // Only unlock if still locked
        Execute()
    if err != nil {
        r.logger.Warn("failed to unlock TIME# record, but bookmark created successfully",
            zap.String("pk", timeRecord.PK),
            zap.String("sk", timeRecord.SK),
            zap.Error(err))
        // Non-fatal: bookmark is created, lock will be cleaned up later
    }
    
    return objectRecord, nil
}
```

**Validation**:
- Run existing unit tests - they must still pass
- Add new test: verify TIME# record starts locked, then unlocks
- Add new test: verify both records exist after create
- Add test: duplicate create should be idempotent
- Add test: concurrent create attempts blocked by lock
- Add test: Phase 2 failure leaves TIME# locked (recovery scenario)

**Status**: Completed via `db.TransactWrite()` using `dynamorm.IfNotExists()` for both puts and a conditional update that flips `Locked` to false only when the TIME row is still locked. Transaction failures now surface `customerrors.ErrConditionFailed`/`TransactionError` metadata and trigger a self-healing pass for legacy TIME-only records.

---

### Task 1.3: Update BookmarkRepository Delete Method
**File**: `pkg/storage/repositories/bookmark_repository.go`

**Requirements**:
- Modify `DeleteBookmark(ctx, username, objectID)` to remove **both** records
- First, Get the OBJECT# record to retrieve the stored `TimeRecordSK`
- Delete both TIME# and OBJECT# records using their known keys
- Use transactional delete if possible
- Handle gracefully if only one record exists (migration period compatibility)

**Example Logic**:
```go
func (r *BookmarkRepository) DeleteBookmark(ctx context.Context, username, objectID string) error {
    pk := fmt.Sprintf("BOOKMARK#%s", username)
    objectSK := fmt.Sprintf("OBJECT#%s", objectID)
    
    // Get OBJECT# record to find TIME# SK
    var objectRecord models.Bookmark
    err := r.db.WithContext(ctx).Get(pk, objectSK, &objectRecord)
    if err != nil {
        if errors.IsNotFound(err) {
            return nil // Idempotent: bookmark doesn't exist
        }
        return err
    }
    
    // Delete both records atomically using known keys
    timeKey := struct{PK, SK string}{PK: pk, SK: objectRecord.TimeRecordSK}
    objectKey := struct{PK, SK string}{PK: pk, SK: objectSK}
    
    return r.transactionalDelete(ctx, timeKey, objectKey)
}
```

**Validation**:
- Test: Delete removes both records (verify by checking both keys)
- Test: Delete is idempotent (no error if bookmark doesn't exist)
- Test: Handles case where only OBJECT# record exists (migration period)
- Test: Uses TimeRecordSK from OBJECT# record to delete TIME# record

**Status**: Both records are deleted inside a single transaction (with graceful fallback to legacy TIME rows) and the helper is fully covered by unit tests.

---

### Task 1.4: Add Batch Bookmark Lookup Method
**File**: `pkg/storage/repositories/bookmark_repository.go`

**Requirements**:
- Add new method: `CheckBookmarksForStatuses(ctx context.Context, username string, statusIDs []string) (map[string]bool, error)`
- Use DynamoDB `BatchGetItem` API to fetch OBJECT# records
- Return map: `statusID → true` (only include bookmarked statuses)
- Handle pagination: BatchGetItem max 100 items per request
- Handle partial failures gracefully

**Example Signature**:
```go
// CheckBookmarksForStatuses efficiently checks which statuses are bookmarked by the user.
// Uses BatchGetItem with OBJECT# keys for O(1) lookups instead of scanning TIME# records.
// Returns map of statusID -> true for bookmarked statuses (unbookmarked not included).
func (r *BookmarkRepository) CheckBookmarksForStatuses(
    ctx context.Context,
    username string,
    statusIDs []string,
) (map[string]bool, error) {
    // Build keys for OBJECT# records
    // Execute BatchGetItem (handle 100-item limit)
    // Parse results into map
}
```

**Validation**:
- Test: Empty statusIDs returns empty map
- Test: Single status returns correct bookmark state
- Test: 20 statuses returns correct states
- Test: 100+ statuses handles pagination correctly
- Test: DynamoDB error returns error (not panic)

---

### Task 1.5: Maintain Backward Compatibility
**File**: `pkg/storage/repositories/bookmark_repository.go`

**Requirements**:
- Update `GetUserBookmarks(username, limit, cursor)` to query only TIME# records
- Add filter: `SK begins_with "TIME#"` AND `Locked = false` (exclude incomplete bookmarks)
- Ensure existing callers continue to work unchanged
- Add fallback in batch lookup: if OBJECT# not found, try querying old format (temporary migration support)
- Add cleanup job documentation: locked TIME# records older than 5 minutes should be deleted (orphaned records)

**Example**:
```go
func (r *BookmarkRepository) GetUserBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*models.Bookmark, string, error) {
    // Query with SK prefix filter and lock check
    query := r.db.WithContext(ctx).Model(&models.Bookmark{}).
        Where("PK", "=", fmt.Sprintf("BOOKMARK#%s", username)).
        Where("SK", "BEGINS_WITH", "TIME#").  // NEW: Only TIME# records
        Filter("Locked", "=", false).         // NEW: Exclude locked/incomplete bookmarks
        OrderBy("SK", "DESC").
        Limit(limit)
    
    // Rest of existing logic unchanged
}
```

**Validation**:
- Test: GetUserBookmarks returns only TIME# records with Locked=false
- Test: Locked TIME# records are excluded from results
- Test: Results are sorted chronologically (newest first)
- Test: Pagination cursor works correctly

**Status**: Timeline queries now filter `Locked=false`, batch lookup uses the retry-aware `BatchGetBuilder`, and the orphan-lock cleanup job is documented as a follow-up maintenance task (delete TIME# rows that remain locked for 5+ minutes).

---

## Phase 1 Deliverables Checklist

Before marking Phase 1 complete, ensure:

- [x] `pkg/storage/models/bookmark.go`:
  - [x] RecordType field added
  - [x] TimeRecordSK field added (stores TIME# SK on OBJECT# records)
  - [x] Locked boolean field added (prevents concurrent writes)
  - [x] NewTimeOrderedBookmark() factory (sets Locked: true)
  - [x] NewObjectIndexedBookmark(username, objectID, createdAt, timeRecordSK) factory
  - [x] UpdateKeys() modified for dual patterns
  - [x] Documentation updated with locking mechanism

- [x] `pkg/storage/repositories/bookmark_repository.go`:
  - [x] CreateBookmark() implements three-phase locked write
  - [x] unlockTimeRecord() helper method for Phase 3
  - [x] DeleteBookmark() removes both records
  - [x] CheckBookmarksForStatuses() new method added
  - [x] GetUserBookmarks() filters for TIME# records with Locked=false
  - [x] Documentation for orphaned locked record cleanup (optional job)
  
- [x] Tests:
  - [x] Unit tests in `bookmark_test.go` (model with Locked field)
  - [x] Unit tests in `bookmark_repository_test.go` (repository with locking)
  - [x] Test: TIME# record starts locked, unlocks after OBJECT# write
  - [x] Test: Concurrent bookmark creates blocked by lock
  - [x] Test: GetUserBookmarks excludes locked records
  - [x] All existing tests still pass
  - [x] Code coverage >90% for changed code

- [x] Documentation:
  - [x] Inline comments explain dual-write pattern
  - [x] Method documentation includes usage examples

## Important Constraints

1. **No breaking changes**: Existing bookmark APIs must continue to work
2. **Atomic writes**: Use transactions when possible to prevent partial writes
3. **Idempotency**: Creating same bookmark twice should not error
4. **Error handling**: Gracefully handle DynamoDB errors (throttling, timeouts)
5. **Testing first**: Write tests before implementation where possible

## Files You'll Modify

- `pkg/storage/models/bookmark.go` (existing)
- `pkg/storage/repositories/bookmark_repository.go` (existing)
- `pkg/storage/models/bookmark_test.go` (new or update)
- `pkg/storage/repositories/bookmark_repository_test.go` (update)

## Success Criteria

✅ Phase 1 is complete when:
1. All tests pass (`make test`)
2. Dual-write creates both TIME# and OBJECT# records
3. Batch lookup method works for 1-100 status IDs
4. No regressions in existing bookmark functionality
5. Code reviewed and approved

**Estimated time**: 4-6 hours

Let me know when Phase 1 is complete, and I'll provide Phase 2 instructions!
