# Phase 5.6 Execution Plan: Complete StorageAdapter Removal

## Current Status
- ✅ cmd/api/main.go - StorageAdapter initialization removed
- ❌ Lift handler files - 500+ h.store references remaining
- ❌ Test files - Still using MockStorage
- ❌ StorageAdapter files - Still exist in codebase

## Execution Steps

### Step 1: Prepare Migration Tools (1-2 hours)
1. ✅ Create method mapping documentation (COMPLETE)
2. ✅ Create migration script (COMPLETE)
3. Test migration script on a single file
4. Refine script based on test results

### Step 2: Implement Missing Methods (2-3 hours)
Before running the migration, we need to implement missing methods:

1. **Endorsement methods in RelationshipRepository**:
   ```go
   // Add to relationship_repository.go
   func (r *RelationshipRepository) CreateEndorsement(ctx context.Context, endorsement *models.Endorsement) error
   func (r *RelationshipRepository) DeleteEndorsement(ctx context.Context, endorserID, endorsedID string) error
   ```

2. **Verify all count methods are wired correctly**:
   - Ensure GetFollowersCount calls are updated to handle username vs actorID
   - Ensure GetStatusCount calls are updated appropriately

### Step 3: Run Automated Migration (1 hour)
1. Create backups of all lift handler files
2. Run the migration script:
   ```bash
   go run tools/migrate_lift_storage_to_repos.go /Users/aronprice/lesser/cmd/api/lift
   ```
3. Review the migration output
4. Check for any files that failed

### Step 4: Manual Review and Fixes (20-30 hours)
This is the most time-consuming part. For each file:

1. **Compile and identify errors**:
   ```bash
   go build ./cmd/api/lift/[filename].go
   ```

2. **Common issues to fix**:
   - Parameter type mismatches (actorID vs username)
   - Missing method implementations
   - Import statements (may need to add imports)
   - Type conversions between storage and repository types
   - Error handling differences

3. **Files to process (by complexity)**:
   
   **Low Complexity (< 10 references each):**
   - webfinger.go (1)
   - trends.go (1)
   - translation.go (2)
   - imports.go (2)
   - debug.go (5)
   - search.go (5)
   - markers.go (5)
   - custom_emojis.go (5)
   - exports.go (5)
   - preferences.go (5)
   
   **Medium Complexity (10-20 references each):**
   - status_interactions.go (6)
   - notes.go (8)
   - oauth.go (5)
   - media.go (10)
   - metrics.go (10)
   - polls.go (10)
   - announcements.go (10)
   - scheduled_statuses.go (10)
   - endorsements.go (10)
   - domain_blocks.go (10)
   - push_subscriptions.go (10)
   - recovery.go (10)
   - wallet.go (10)
   - webauthn.go (10)
   - ai.go (10)
   
   **High Complexity (15-30 references each):**
   - status_pins.go (15)
   - conversations.go (17)
   - lists.go (15)
   - bookmarks.go (14)
   - relationships.go (15)
   - follow_requests.go (15)
   - mutes.go (15)
   - filters.go (20)
   - reports.go (20)
   - misc.go (20)
   - tags.go (25)
   
   **Very High Complexity (30+ references each):**
   - discovery.go (8 + complex logic)
   - accounts.go (30)
   - timelines.go (30)
   - interactions.go (45)
   - statuses.go (50)
   - admin.go (50)

### Step 5: Update Test Files (5-10 hours)
1. Find all test files using MockStorage:
   ```bash
   grep -r "MockStorage" cmd/api/lift/*_test.go
   ```

2. Update each test file to use MockRepositoryStorage
3. Fix any compilation errors
4. Run tests to ensure they pass

### Step 6: Remove Legacy Code (1-2 hours)
1. Delete StorageAdapter files:
   ```bash
   rm pkg/storage/dynamorm/adapter.go
   rm pkg/storage/dynamorm/mock_adapter.go
   ```

2. Delete storage.Storage interface:
   ```bash
   rm pkg/storage/interface.go
   ```

3. Remove any remaining storage imports

### Step 7: Final Validation (2-3 hours)
1. Full build of all services:
   ```bash
   go build ./...
   ```

2. Run all tests:
   ```bash
   go test ./...
   ```

3. Integration testing with key workflows

## Risk Mitigation

### Incremental Approach
- Process files in order of complexity
- Test after each file
- Commit working changes frequently

### Rollback Strategy
- Keep backups of all files
- Use git branches for the migration
- Test thoroughly before merging

### Common Pitfalls to Avoid
1. **Don't create stubs** - Find real implementations
2. **Check parameter types** - actorID vs username is common
3. **Verify error handling** - Repository errors may differ
4. **Test edge cases** - Nil checks, empty results, etc.

## Success Metrics
- [ ] All h.store references removed
- [ ] All lift handler files compile
- [ ] All tests pass
- [ ] No runtime errors in integration tests
- [ ] StorageAdapter files deleted
- [ ] storage.Storage interface deleted

## Time Estimate
- Preparation: 2-3 hours
- Implementation: 2-3 hours  
- Automated Migration: 1 hour
- Manual Review/Fixes: 20-30 hours
- Testing: 5-10 hours
- Cleanup: 1-2 hours
- Validation: 2-3 hours
- **Total: 33-52 hours**

## Next Immediate Actions
1. Test the migration script on status_pins.go (simplest file)
2. Verify the script works correctly
3. Implement missing endorsement methods
4. Begin systematic migration starting with low-complexity files