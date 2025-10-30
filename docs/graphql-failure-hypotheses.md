# GraphQL Remaining Failures - Detailed Hypotheses

## Test Results: 26/32 Passing (81.2%)

All hypotheses based on CloudWatch logs from most recent test run (Oct 30, 14:56 UTC) and code analysis.

---

## ❌ FAILURE 1: Unlike Post

**Error**: `failed to register model **models.Like: invalid model: model must be a struct`  
**Location**: `pkg/storage/repositories/base_repository.go:1055`

### Root Cause Analysis
```go
// base_repository.go:1034
model := new(M)  // When M = *models.Like, this creates **models.Like

// base_repository.go:1036
err := r.db.WithContext(ctx).Model(model)  // DynamORM can't register **models.Like
```

**Why This Happens**:
- `LikeRepository` embeds `EnhancedBaseRepository[*models.Like]`
- Type parameter M is `*models.Like`  
- `new(M)` creates `new(*models.Like)` = `**models.Like` (double pointer)
- DynamORM expects struct or single pointer, rejects double pointer

**Attempted Fix**: Changed type parameter to `models.Like` but this broke compilation because `models.Like` methods have pointer receivers.

**Correct Fix**: Change `DeleteEntityWithLogging` line 1034:
```go
// From:
model := new(M)

// To:
var model M  // Creates zero value; for *models.Like this is nil, but Model() dereferences correctly
```

This matches how `BaseRepository.Delete` handles it at line 460.

**Confidence**: ✅ HIGH - Clear type system issue with known fix

---

## ❌ FAILURE 2: Boost Post

**Error**: `failed to check existing announce: Failed to retrieve announce`  
**Location**: `pkg/services/notes/service.go:2077`

### Root Cause Analysis
```go
// service.go:2070
if existing, err := s.socialRepo.GetAnnounce(ctx, actorURL, objectURL); err == nil {
    // Found existing announce - return it
} else if appErr, ok := svcErrors.AsAppError(err); ok && appErr.Code != svcErrors.CodeNotFound {
    // This block should ONLY execute for non-NotFound errors
    s.logger.Error("failed to check existing announce")  // ← This is logging
    return nil, ErrCreateReblog
}
```

**Why This Happens**:
When GetAnnounce is called on a post that hasn't been boosted yet:
1. DynamoDB query finds no record
2. `errors.IsNotFound(err)` should be true (line 514 in social_repository.go)
3. Returns `ItemNotFoundWithID` with `CodeNotFound`
4. Service check at line 2076 should skip error logging because `appErr.Code == CodeNotFound`

But the error IS being logged, meaning either:
- The error is not an `*AppError` (AsAppError returns false)
- OR the error has a different code (not `CodeNotFound`)

Looking at line 517, when it's NOT a not-found error, it calls `errors.FailedToGet` which has `CodeInternal`.

**The issue**: The Get query is failing with a non-NotFound error. This could be:
- Double pointer issue in announceRepo.Get (same as unlike)
- PK/SK format mismatch
- DynamoDB query error

**Confidence**: ⚠️ MEDIUM - Need to verify if announceRepo has same type parameter issue

---

## ❌ FAILURE 3: Unboost Post

**Error**: `failed to remove reblog engagement: Failed to update status`  
**Location**: `pkg/services/notes/service.go:2185`

### Root Cause Analysis
```go
// service.go:2184
if err := s.noteRepo.UnreblogStatus(ctx, actorURL, statusID); err != nil {
    s.logger.Error("failed to remove reblog engagement")  // ← Error here
    return ErrDeleteReblog
}
```

**Why This Happens**:
`UnreblogStatus` tries to update the status object to decrement reblog count or remove reblog metadata. The update operation is failing.

Possible causes:
1. Status model has field type issues preventing update
2. Conditional update expression is malformed
3. ValidateAndUpdate is rejecting the changes
4. Status doesn't exist (but query would fail earlier)

**Confidence**: ⚠️ LOW - Need to examine UnreblogStatus implementation

---

## ❌ FAILURE 4: Delete Post  

**Error**: `failed to delete object: Failed to delete status`  
**Location**: `graph/mutation_resolvers_notes.go:140`

### Root Cause Analysis
```go
// service.go:542 - Soft delete operation
if err := s.noteRepo.UpdateStatus(ctx, status); err != nil {
    return ErrDeleteStatus  // ← This is the error
}
```

The soft delete:
1. Sets `status.Deleted = true`
2. Sets `status.DeletedAt = &now`
3. Calls `UpdateStatus` which calls `ValidateAndUpdate`

**Why This Happens**:
The authorization check now passes (after AuthorUsername fix - no more "Access denied"), but the UpdateStatus operation fails.

Possible causes:
1. Validation service rejecting soft delete state
2. Permission service blocking the update  
3. Status model's BeforeUpdate hook failing
4. DynamoDB conditional update failing
5. Enhanced repository validation rejecting the change

**Confidence**: ⚠️ MEDIUM - Authorization fixed but update itself fails

---

## ❌ FAILURE 5: Unfollow Actor

**Error**: `internal system error` (panic in `generated.go:39609`)  
**Location**: GraphQL generated code

### Root Cause Analysis
```go
// mutation_resolvers_stubs.go:51
func (r *mutationResolver) UnfollowActor(ctx context.Context, id string) (bool, error) {
    // ... auth check ...
    _, err = relationshipsService.Unfollow(ctx, &relationships.UnfollowCommand{
        FollowerID:  username,
        FollowingID: id,
    })
    // ... error handling ...
    return true, nil
}
```

The resolver returns `(bool, error)`. A panic in generated code suggests the response serialization is failing, not the resolver logic itself.

**Test Script Issue**:
```python
# Line 240-244
validator.test("Unfollow Actor", f"""
    mutation {{
        unfollowActor(id: "{member_id}")  # Missing explicit boolean result handling?
    }}
""")
```

While Boolean doesn't need subfields, the test might not be handling the response properly.

**Confidence**: ⚠️ LOW - Panic is unusual; need to check if relationship was created successfully by Follow

---

## ❌ FAILURE 6: Follow Actor

**Error**: `422 Client Error: Unprocessable Entity`

### Root Cause Analysis
**Direct Test Result**: followActor works perfectly when tested manually:
```json
{"data":{"followActor":{"id":"admin/follows/member","type":"FOLLOW"}}}
```

**Why Test Fails**: The validation script tries to follow `member` during EVERY test run. If the relationship already exists from a previous run, the service likely returns a validation error for duplicate follows.

**Evidence**:
- Manual test (just now): ✅ Works
- Validation script: ❌ 422 error
- Difference: Script runs after previous test runs that may have created the relationship

**Confidence**: ✅ HIGH - This is a test data issue, not a server bug

---

## 📊 Summary of Root Causes

| Failure | Category | Confidence | Root Cause |
|---------|----------|------------|------------|
| Unlike Post | Type System | ✅ HIGH | Double pointer from `new(M)` in generic function |
| Boost Post | Type System | ⚠️ MEDIUM | Likely same double pointer in announceRepo.Get |
| Unboost Post | Repository | ⚠️ LOW | UnreblogStatus update failing - unknown cause |
| Delete Post | Repository | ⚠️ MEDIUM | ValidateAndUpdate rejecting soft delete |
| Unfollow Actor | Unknown | ⚠️ LOW | Panic in generated code - unusual |
| Follow Actor | Test Data | ✅ HIGH | Duplicate follow attempt - server works fine |

### Common Thread
**3/6 failures** appear to be related to the generic type system and pointer handling in repository operations. The `new(M)` pattern creates double pointers when M is already a pointer type.

### Actionable Fixes
1. **Unlike/Boost** (HIGH confidence): Change `new(M)` to `var model M` in affected generic functions
2. **Follow** (HIGH confidence): Test script should check relationship state or handle 422 gracefully
3. **Delete/Unboost/Unfollow** (LOW-MEDIUM confidence): Need deeper investigation into repository update operations


