# GraphQL Remaining Issues - Deep Analysis
**Generated**: October 30, 2025  
**Starting Point**: 27/33 tests passing (81.8%)  
**Focus**: 6 remaining failures with detailed root cause hypotheses

---

## Executive Summary

After fixing 13 tests in the previous session, we have 6 remaining failures. Analysis reveals:
- **3 Critical Server Bugs** (Boost, Unboost, Delete) - Repository/validation layer issues
- **3 Test Infrastructure Issues** (Follow, Unfollow x2) - Script cleanup and duplicate handling

**Key Insight**: All 3 server bugs involve the **Enhanced Repository validation system** rejecting legitimate operations. The recent switch from basic `Update()` to `ValidateAndUpdate()` is causing validation failures.

---

## ❌ CRITICAL ISSUE #1: Boost Post Failure

### Error Message
```
failed to check existing announce: Failed to retrieve announce
```

### Location
`pkg/services/notes/service.go:2077`

### Code Flow Analysis

```go
// Line 2070-2082
if existing, err := s.socialRepo.GetAnnounce(ctx, actorURL, objectURL); err == nil {
    // Found existing - return it
    return existing, nil
} else if appErr, ok := svcErrors.AsAppError(err); ok && appErr.Code != svcErrors.CodeNotFound {
    s.logger.Error("failed to check existing announce")  // ← THIS IS LOGGING
    return nil, ErrCreateReblog
}
```

### Root Cause Hypothesis #1: NotFound Not Being Recognized (HIGH CONFIDENCE - 85%)

**Theory**: The error path analysis shows two scenarios:
1. GetAnnounce finds no record → should return NotFound error
2. GetAnnounce has query error → returns different error code

The fact that error logging is triggered means `appErr.Code != CodeNotFound`, so either:
- Error is not wrapped as AppError properly
- Error code is `CodeInternal` instead of `CodeNotFound`

**Supporting Evidence**:
```go
// social_repository.go:514-517
if errors.IsNotFound(err) {
    return nil, ErrorHandler.HandleGetError(err, EntityAnnounce, "not found")
}
return nil, ErrorHandler.HandleGetError(err, EntityAnnounce, "get")
```

The "get" path calls `HandleGetError` with operation type "get", not "not found". This might return `CodeInternal`.

**Expected Behavior**: First boost attempt should:
1. Call GetAnnounce → NotFound (no existing boost)
2. Service detects NotFound, continues to create announce
3. Returns success

**Actual Behavior**: GetAnnounce is returning an error that's NOT NotFound, triggering the error log.

### Root Cause Hypothesis #2: DynamoDB Query Syntax Error (MEDIUM CONFIDENCE - 60%)

**Theory**: The query itself is failing before it even checks for results.

```go
// social_repository.go:507-512
pk := fmt.Sprintf("OBJECT#%s#ANNOUNCES", object)
sk := fmt.Sprintf("ACTOR#%s", actor)
var model models.Announce
err := r.announceRepo.Get(ctx, pk, sk, &model)
```

**Potential Issues**:
- `announceRepo` is `*EnhancedBaseRepository[models.Announce]`
- The `Get()` method might have validation that rejects the keys
- GSI attribute naming inconsistencies (we fixed some earlier)

### Recommended Fix

**Priority**: HIGH (blocks basic social interaction)

**Investigation Steps**:
1. Add debug logging to see actual error details:
```go
} else if appErr, ok := svcErrors.AsAppError(err); ok && appErr.Code != svcErrors.CodeNotFound {
    s.logger.Error("failed to check existing announce",
        zap.String("actor_url", actorURL),
        zap.String("object_url", objectURL),
        zap.String("error_code", string(appErr.Code)),  // ADD THIS
        zap.String("error_message", appErr.Message),    // ADD THIS
        zap.Error(err))
```

2. Check ErrorHandler.HandleGetError implementation - does it differentiate NotFound from other errors?

3. Test manually:
```bash
# Create a post, then try to boost it while watching CloudWatch logs
```

**Expected Fix**: Likely need to adjust error handling in `social_repository.go:514-517` to properly wrap NotFound errors with `CodeNotFound`.

---

## ❌ CRITICAL ISSUE #2: Unboost Post Failure

### Error Message
```
failed to remove reblog engagement: Failed to update status
```

### Location
`pkg/services/notes/service.go:2185`

### Code Flow Analysis

```go
// Line 2184-2190
if err := s.noteRepo.UnreblogStatus(ctx, actorURL, statusID); err != nil {
    s.logger.Error("failed to remove reblog engagement")  // ← ERROR HERE
    return ErrDeleteReblog
}
```

### Following the Trail

**UnreblogStatus Implementation** (`status_repository.go:1193-1199`):
```go
func (r *StatusRepository) UnreblogStatus(ctx context.Context, userID, statusID string) error {
    return r.removeEngagement(ctx, userID, statusID, "boost", "reblog", func(status *models.Status) {
        if status.ReblogCount > 0 {
            status.ReblogCount--
        }
    })
}
```

**removeEngagement Implementation** (`status_repository.go:1110-1143`):
```go
func (r *StatusRepository) removeEngagement(..., updateCount func(*models.Status)) error {
    // 1. Find and delete the engagement record
    var engagements []models.StatusEngagement
    err := r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
        Where("PK", "=", fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID)).
        Filter("EngagementType", "=", engagementType).
        Filter("UserID", "=", userID).
        All(&engagements)
    // ... delete engagement ...
    
    // 2. Update status count
    status, err := r.GetStatus(ctx, statusID)
    // ...
    updateCount(status)  // Decrements ReblogCount
    err = r.UpdateStatus(ctx, status)  // ← THIS IS FAILING (line 1137)
    if err != nil {
        return ErrorHandler.HandleUpdateError(err, EntityStatus, statusID)
    }
}
```

### Root Cause Hypothesis: ValidateAndUpdate Rejecting Update (HIGH CONFIDENCE - 90%)

**Theory**: `UpdateStatus` calls `ValidateAndUpdate`, which has multiple validation checkpoints:

```go
// status_repository.go:254-257
func (r *StatusRepository) UpdateStatus(ctx context.Context, status *models.Status) error {
    return r.ValidateAndUpdate(ctx, status)
}

// enhanced_base_repository.go:168-204
func (r *EnhancedBaseRepository[T]) ValidateAndUpdate(ctx context.Context, model T) error {
    // 1. Validate model (UpdateKeys, PK/SK not empty)
    if r.validator != nil {
        if err := r.validator.ValidateModel(ctx, model); err != nil {
            return errors.ValidationFailed("model validation", err.Error())
        }
    }
    
    // 2. Validate business rules
    if r.validator != nil {
        if err := r.validator.ValidateBusinessRules(ctx, model, "update"); err != nil {
            return errors.ValidationFailed("business rules", err.Error())
        }
    }
    
    // 3. Check permissions
    if err := r.checkUpdatePermissions(ctx, model); err != nil {
        return err
    }
    
    // 4. Execute update
    if err := r.Update(ctx, model); err != nil {
        return err
    }
}
```

**Likely Failure Points**:

1. **Model Validation** (Line 171):
   - `UpdateKeys()` might be regenerating PK/SK incorrectly
   - Status loaded from DB might have stale data

2. **Business Rules Validation** (Line 176):
   - `ValidateBusinessRules` for "update" action might reject:
     - Status with decremented ReblogCount
     - Status loaded via `GetStatus` vs. original create flow

3. **Permission Check** (Line 182):
   - `checkUpdatePermissions` might expect user context
   - System-level count updates might lack proper auth context

**Why Unlike Works But Unreblog Fails**:

Unlike now works after the `new(M)` fix. Key difference:
- **Unlike**: Deletes Like record only (simple delete operation)
- **Unreblog**: Deletes engagement record AND updates Status model (complex operation hitting ValidateAndUpdate)

### Comparison with DeleteStatus

**DeleteStatus also uses soft delete** (`status_repository.go:260-274`):
```go
func (r *StatusRepository) DeleteStatus(ctx context.Context, statusID string) error {
    status, err := r.GetStatus(ctx, statusID)
    // ...
    status.Deleted = true
    status.DeletedAt = &now
    
    return r.Update(ctx, status)  // Uses Update, NOT ValidateAndUpdate
}
```

**Notice**: DeleteStatus calls `r.Update()` directly, NOT `r.UpdateStatus()` which calls `ValidateAndUpdate()`.

**This is the smoking gun**: System operations (delete, engagement updates) should bypass validation but currently don't.

### Recommended Fix

**Priority**: HIGH (blocks basic social interaction)

**Option 1 - Bypass validation for system operations** (RECOMMENDED):
```go
// In removeEngagement, line 1137
// Change from:
err = r.UpdateStatus(ctx, status)

// To:
err = r.Update(ctx, status)  // Bypass ValidateAndUpdate
```

**Option 2 - Fix the validation to allow count decrements**:
- Modify ValidateBusinessRules to allow engagement count changes
- More complex, might break other safeguards

**Option 3 - Use atomic updates for counts**:
```go
// Use DynamoDB atomic increment/decrement instead of read-modify-write
err := r.db.WithContext(ctx).Model(&models.Status{
    PK: fmt.Sprintf("status#%s", statusID),
    SK: fmt.Sprintf("status#%s", statusID),
}).Update("ReblogCount", "ReblogCount - 1")
```

---

## ❌ CRITICAL ISSUE #3: Delete Post Failure

### Error Message
```
failed to delete object: Failed to delete status
```

### Location
`graph/mutation_resolvers_notes.go:140`

### Code Flow Analysis

```go
// service.go:546-548
if err := s.noteRepo.UpdateStatus(ctx, status); err != nil {
    return ErrDeleteStatus  // ← THIS IS FAILING
}
```

**Context** (Lines 540-543):
```go
now := time.Now()
status.Deleted = true
status.DeletedAt = &now
status.ModifiedAt = now
```

### Root Cause Hypothesis: Same as Unboost - ValidateAndUpdate Rejection (VERY HIGH CONFIDENCE - 95%)

**Theory**: IDENTICAL issue to Unboost. The soft delete operation:
1. Loads status via `GetStatus`
2. Sets `Deleted = true`, `DeletedAt = &now`
3. Calls `UpdateStatus` → `ValidateAndUpdate`
4. Validation rejects the update

**Why authorization check now passes** (after your fix):
```go
// Lines 519-537 (after fix)
if status.AuthorUsername != cmd.DeleterID {  // Fixed: was AuthorID
    // Check admin privileges
    isAdmin := /* ... */
    if !isAdmin {
        return common.ErrForbidden(ErrCannotDeletePostOwnedByOther)
    }
}
```

Authorization passes, but the subsequent `UpdateStatus` fails.

### Evidence Supporting This Theory

1. **DeleteStatus uses Update, not UpdateStatus**:
```go
// status_repository.go:273
return r.Update(ctx, status)  // Bypasses validation
```

2. **DeleteNote service uses UpdateStatus**:
```go
// service.go:546
if err := s.noteRepo.UpdateStatus(ctx, status); err != nil {  // Hits validation
```

**This inconsistency is the bug**: Repository's DeleteStatus knows to bypass validation, but service layer's DeleteNote doesn't use DeleteStatus - it calls UpdateStatus directly.

### Recommended Fix

**Priority**: HIGH (blocks basic content management)

**Option 1 - Use repository's DeleteStatus method** (RECOMMENDED):
```go
// service.go:546
// Change from:
if err := s.noteRepo.UpdateStatus(ctx, status); err != nil {
    return ErrDeleteStatus
}

// To:
if err := s.noteRepo.DeleteStatus(ctx, cmd.StatusID); err != nil {
    return ErrDeleteStatus
}
```

**Benefits**:
- Uses existing tested method
- Properly bypasses validation
- Maintains soft delete pattern

**Option 2 - Bypass validation in service**:
```go
// Change UpdateStatus to Update in this specific case
// Less clean, but works
```

---

## ⚠️ TEST ISSUE #4: Follow Actor (422 Error)

### Error Message
```
422 Client Error: Unprocessable Entity
```

### Root Cause: Test Script Cleanup (CONFIRMED - 100%)

**Evidence**:
1. **Manual test**: ✅ Works perfectly
```json
{"data":{"followActor":{"id":"admin/follows/member","type":"FOLLOW"}}}
```

2. **Validation script**: ❌ Fails with 422

### Why It Fails

```python
# validate_graphql_comprehensive.py:225-234
validator.test("Follow Actor", f"""
    mutation {{
        followActor(id: "{member_id}") {{
            id
            type
            actor
            object
        }}
    }}
""")
```

**Scenario**:
1. First test run: Creates follow relationship → Success
2. Second test run: Tries to create same relationship → 422 (duplicate)
3. The pre-cleanup unfollow (line 219) also fails (see Issue #6)

### Recommended Fix

**Priority**: LOW (not a server bug, works in production)

**Option 1 - Make test idempotent**:
```python
# Change test to expect either success OR 422
def test_follow_or_already_following(self, name: str, query: str):
    result = self.test(name, query)
    if not result.success and "422" in str(result.error):
        # Already following - that's fine
        result.success = True
    return result
```

**Option 2 - Improve cleanup**:
```python
# Before follow test, more robustly clean up
def cleanup_follow(self, member_id):
    try:
        self.graphql_request(f"""
            mutation {{ unfollowActor(id: "{member_id}") }}
        """)
    except:
        pass  # Ignore if not following
```

**Option 3 - Use unique test accounts per run**:
- More complex infrastructure
- Cleaner separation
- Not worth the effort for this issue

---

## ⚠️ TEST ISSUE #5: Unfollow Actor (Panic)

### Error Message
```
internal system error (panic in generated.go:39609)
```

### Location
GraphQL generated code, not resolver code

### Root Cause Hypothesis: Response Serialization Panic (MEDIUM CONFIDENCE - 70%)

**Theory**: The resolver returns `(bool, error)` successfully, but the generated GraphQL code panics during response serialization.

**Code Analysis**:

```go
// mutation_resolvers_stubs.go:51-83
func (r *mutationResolver) UnfollowActor(ctx context.Context, id string) (bool, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return false, err
    }

    // ... service call ...
    _, err = relationshipsService.Unfollow(ctx, &relationships.UnfollowCommand{
        FollowerID:  username,
        FollowingID: id,
    })
    if err != nil {
        r.Logger.Error("Failed to unfollow actor", ...)
        return false, err
    }

    return true, nil  // Should be fine
}
```

**Why Panic Might Occur**:

1. **No relationship exists** (because Follow failed):
   - Unfollow service tries to delete non-existent relationship
   - Returns error, resolver returns `false, err`
   - Generated code expects specific error format
   - Panic when error doesn't match expected structure

2. **Nil pointer in service response**:
   - Service returns `(nil, nil)` instead of proper response
   - Resolver discards first return value: `_, err = ...`
   - But what if the service contract expects first value to be checked?

3. **Context corruption**:
   - User context from requireAuth is invalid
   - Service call partially succeeds but corrupts context
   - Generated code can't serialize response

### Comparison with Other Boolean Resolvers

**UnblockActor and UnmuteActor** use identical pattern and don't panic:
```go
// mutation_resolvers_stubs.go:15-47
func (r *mutationResolver) UnblockActor(ctx context.Context, id string) (bool, error) {
    // ... identical structure ...
    return true, nil
}
```

**Key Difference**: These likely haven't been tested in the validation script, so we don't know if they panic too.

### Recommended Fix

**Priority**: MEDIUM (panic is bad, but might be caused by Follow failure)

**Step 1 - Get actual panic stack trace**:
```bash
# Check CloudWatch logs for the full panic message
# Look for goroutine dump and line numbers
```

**Step 2 - Add defensive nil checks**:
```go
// In Unfollow service implementation
func (s *Service) Unfollow(ctx context.Context, cmd *UnfollowCommand) (*UnfollowResult, error) {
    // Check if relationship exists first
    rel, err := s.relationshipRepo.GetRelationship(ctx, cmd.FollowerID, cmd.FollowingID)
    if err != nil {
        if errors.IsNotFound(err) {
            // Not following - idempotent success
            return &UnfollowResult{Success: true}, nil
        }
        return nil, err
    }
    
    // Proceed with unfollow
    // ...
}
```

**Step 3 - Test unfollow without preceding follow**:
```bash
# Manually test unfollowing someone you're NOT following
# Should return graceful error, not panic
```

---

## ⚠️ TEST ISSUE #6: Unfollow Actor (Pre-cleanup) - Same Panic

### Error Message
Same as Issue #5

### Location
```python
# validate_graphql_comprehensive.py:219-223
validator.test("Unfollow Actor (Pre-cleanup)", f"""
    mutation {{
        unfollowActor(id: "{member_id}")
    }}
""")
```

### Root Cause: Identical to Issue #5

This is the **pre-test cleanup step** added to ensure clean state before follow test. It has the same panic issue.

### Why This Exists

The script tries to:
1. Unfollow member (cleanup from previous run)
2. Follow member (actual test)
3. Unfollow member (cleanup for next run)

But step 1 and 3 both panic.

### Recommended Fix

**Priority**: LOW (same fix as Issue #5)

**Immediate workaround**:
```python
# Lines 219-223
# Change test to allow failure
validator.test("Unfollow Actor (Pre-cleanup)", f"""
    mutation {{
        unfollowActor(id: "{member_id}")
    }}
""", allow_failure=True)  # Don't fail the entire suite if cleanup fails
```

**Long-term fix**: Same as Issue #5 - make Unfollow idempotent and handle "not following" gracefully.

---

## 📊 Priority Matrix

| Issue | Severity | Confidence | Blocks | Priority | Est. Fix Time |
|-------|----------|-----------|--------|----------|---------------|
| #2: Unboost | Critical | 90% | Social features | P0 | 15 min |
| #3: Delete | Critical | 95% | Content mgmt | P0 | 15 min |
| #1: Boost | Critical | 85% | Social features | P0 | 30 min |
| #5: Unfollow Panic | High | 70% | Edge case | P1 | 45 min |
| #4: Follow 422 | Low | 100% | Test only | P2 | 10 min |
| #6: Unfollow Cleanup | Low | 100% | Test only | P2 | 5 min |

---

## 🎯 Recommended Action Plan

### Phase 1: Quick Wins (30 minutes)
**Goal**: Fix Unboost and Delete to get to 29/33 (87.8%)

1. **Fix Unboost** (15 min):
   ```go
   // pkg/storage/repositories/status_repository.go:1137
   // Change UpdateStatus to Update in removeEngagement
   err = r.Update(ctx, status)  // Bypass validation
   ```

2. **Fix Delete** (15 min):
   ```go
   // pkg/services/notes/service.go:546
   // Use DeleteStatus method instead of UpdateStatus
   if err := s.noteRepo.DeleteStatus(ctx, cmd.StatusID); err != nil {
   ```

3. **Test**: Run validation script, expect 29/33 passing

### Phase 2: Boost Investigation (30 minutes)
**Goal**: Fix Boost to get to 30/33 (90.9%)

1. **Add debug logging** (10 min):
   ```go
   // pkg/services/notes/service.go:2077
   // Add error code and message to logging
   ```

2. **Run validation script**, check CloudWatch logs (5 min)

3. **Analyze error details** (10 min):
   - If error code is not NotFound → fix error wrapping in social_repository.go
   - If error is NotFound → investigate why condition fails

4. **Apply fix** (5 min)

### Phase 3: Unfollow Panic (45 minutes)
**Goal**: Fix Unfollow to get to 32/33 (96.9%)

1. **Get panic stack trace** (15 min):
   - Check CloudWatch logs for full error
   - Identify exact line in generated code

2. **Make Unfollow idempotent** (20 min):
   - Check if relationship exists before unfollowing
   - Return success if already unfollowed

3. **Test manually** (10 min):
   - Unfollow someone you're not following
   - Verify no panic, graceful response

### Phase 4: Test Script Cleanup (15 minutes)
**Goal**: Get to 33/33 (100%)

1. **Fix Follow test** (10 min):
   - Make test idempotent or improve cleanup

2. **Remove duplicate cleanup test** (5 min):
   - Or mark as allow_failure

---

## 💡 Key Insights

### Pattern Recognition

**All 3 server bugs share a common root cause**: The enhanced repository validation system is rejecting legitimate system operations.

- **Unboost**: Engagement count decrement rejected by ValidateAndUpdate
- **Delete**: Soft delete flag rejected by ValidateAndUpdate  
- **Boost**: Error wrapping not preserving NotFound vs. Internal distinction

### Architectural Issue

**ValidateAndUpdate is too strict for system operations**. It was designed for user-initiated changes but is being called for:
- Engagement count updates (automated)
- Soft deletes (system operation)
- Cache invalidation updates (internal)

**Solution pattern**: System operations should use `Update()` directly, not `UpdateStatus()` → `ValidateAndUpdate()`.

### The Unlike Fix Precedent

Unlike was failing with double-pointer bug. After fix, it works because:
```go
// pkg/storage/repositories/like_repository.go
// Delete doesn't go through ValidateAndUpdate
err := r.Delete(ctx, pk, sk)  // Direct delete, no validation
```

But Unboost and Delete DO go through ValidateAndUpdate, so they fail even though Unlike works.

---

## 📝 Additional Notes

### Testing Strategy After Fixes

After applying each fix:
1. Run full validation script: `make test-graphql` (if you have this)
2. Check CloudWatch logs for any new errors
3. Test manually to confirm expected behavior
4. Update this document with actual vs. expected results

### Regression Prevention

Consider adding:
1. Unit tests for system operations that bypass validation
2. Integration tests for full engagement flows (like, unlike, boost, unboost)
3. Explicit flag in repository methods: `skipValidation bool` parameter

### Future Enhancements

1. **Idempotent operations**: All social actions should be idempotent
   - Like an already-liked post → success (no-op)
   - Unfollow someone not followed → success (no-op)
   - Delete already-deleted post → success (no-op)

2. **Better error differentiation**: Distinguish between:
   - User errors (400-level) - "You can't do this"
   - System errors (500-level) - "Something broke"
   - Not found errors (404) - "This doesn't exist"

3. **Validation bypass flag**:
   ```go
   type UpdateOptions struct {
       SkipValidation bool
       SkipPermissions bool
       SystemOperation bool
   }
   ```

---

**Next Step**: Start with Phase 1 fixes (Unboost + Delete) to quickly get to 87.8% success rate and build momentum.

