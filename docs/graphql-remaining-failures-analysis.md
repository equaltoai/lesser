# GraphQL Remaining Failures - Root Cause Analysis

## Summary
6 mutations are failing with specific, identifiable bugs in the codebase. All failures have clear error messages in CloudWatch logs.

---

## ❌ CATEGORY 1: Repository Type Errors (Double Pointer Bug)

### Unlike Post - `unlikeObject`
**Error**: `failed to delete like: invalid model: model must be a struct`  
**Location**: `pkg/storage/repositories/base_repository.go:1055`

**Root Cause**: 
```
failed to register model **models.Like: invalid model: model must be a struct
```
The code is passing `**models.Like` (double pointer) instead of `*models.Like` to `DeleteEntityWithLogging`. This happens in `pkg/storage/repositories/like_repository.go:94` when calling `DeleteLike`.

**Hypothesis**: The Like model is being incorrectly dereferenced or the repository method signature expects a different type. The DynamORM library is rejecting the double-pointer type.

---

## ❌ CATEGORY 2: Query Failures (Database Lookup Issues)

### Boost Post - `shareObject`
**Error**: `failed to check existing announce: Failed to retrieve announce`  
**Location**: `pkg/services/notes/service.go:2073`

**Root Cause**: When trying to boost a post, the system first checks if the user has already boosted it. This query is failing.

**Hypothesis**: 
- GSI query for existing announces may be malformed
- The announce lookup may be using incorrect partition/sort keys
- Related to the GSI attribute name issues we fixed earlier (gsI2PK vs GSI2PK)

### Unboost Post - `unshareObject`  
**Error**: `failed to remove reblog engagement: Failed to update status`  
**Location**: `pkg/services/notes/service.go:2181`

**Root Cause**: Similar to boost - trying to remove a reblog that may not exist or can't be found.

**Hypothesis**: Same underlying query issue as boost. The system can't find the announce/reblog record to delete it.

---

## ❌ CATEGORY 3: Authorization Issues

### Delete Post - `deleteObject`
**Error**: `Access denied`  
**Location**: `graph/mutation_resolvers_notes.go:140`

**Root Cause**: The delete operation is performing an authorization check that's failing even though the user owns the post.

**Hypothesis**:
- The ownership validation logic may be comparing different ID formats (URL vs username)
- The authenticated user's ID from context may not match the post's attributedTo field
- Possible that the test created posts under one ID format but auth uses another

---

## ❌ CATEGORY 4: HTTP 422 Errors (Not in Recent Logs)

### Follow Actor - `followActor`
**Error**: `422 Client Error: Unprocessable Entity`  
**Location**: Returned by GraphQL endpoint

### Bookmark Post - `bookmarkObject`
**Error**: `422 Client Error: Unprocessable Entity`  
**Location**: Returned by GraphQL endpoint

**Hypothesis**: 
- These errors suggest validation failures or constraint violations
- 422 typically means "semantically incorrect" - the request is well-formed but can't be processed
- May be duplicate key violations (trying to follow/bookmark twice)
- Or validation rules rejecting the operation

---

## 🎯 Unified Root Cause Hypothesis

**Primary Issue**: **DynamoDB query/model mismatch after recent schema changes**

All failures share common characteristics:
1. **Write operations work** (createNote, likeObject succeed)
2. **Read operations work** (timelines, queries succeed)  
3. **Undo/Delete operations fail** (unlike, unboost, delete fail)
4. **Duplicate-check operations fail** (boost checking existing, follow/bookmark 422s)

This suggests:
- **Models or indexes were recently changed** causing lookups to fail
- **The GSI fixes we made** may have resolved some issues but not others
- **Type mismatches** in repository layer (double pointer bug proves this)

**Specific Technical Issues**:
1. **Repository layer has type bugs**: Double pointer `**models.Like` instead of `*models.Like`
2. **GSI queries failing**: Announce/reblog lookups can't find records
3. **Authorization using wrong ID format**: Delete checking ownership with mismatched IDs
4. **Constraint validation**: Follow/bookmark hitting duplicate key or validation rules

---

## 🔧 Recommended Fixes (Priority Order)

### HIGH PRIORITY
1. **Fix Unlike Double Pointer Bug**
   - File: `pkg/storage/repositories/like_repository.go:94`
   - Change how Like is passed to DeleteEntityWithLogging
   - Search for other double-pointer issues in repository layer

2. **Fix Announce/Reblog Queries**
   - File: `pkg/services/notes/service.go:2073, 2181`
   - Review announce lookup queries (likely using wrong GSI or attribute names)
   - May need to apply similar gsI2PK → GSI2PK fixes

### MEDIUM PRIORITY  
3. **Fix Delete Authorization**
   - File: `graph/mutation_resolvers_notes.go:140`
   - Review ownership comparison logic
   - Ensure authenticated user ID format matches post.attributedTo format

4. **Debug 422 Errors**
   - Enable detailed validation error messages
   - Add logging to relationship/bookmark services to see why validation fails
   - Check for duplicate key violations

---

## 📊 Expected Impact After Fixes

| Mutation | Current | After Fixes |
|----------|---------|-------------|
| Unlike Post | ❌ Type Error | ✅ Should Work |
| Boost Post | ❌ Query Fail | ✅ With GSI Fix |
| Unboost Post | ❌ Query Fail | ✅ With GSI Fix |
| Delete Post | ❌ Auth Fail | ✅ With ID Fix |
| Follow Actor | ❌ 422 Error | ⚠️ Needs Investigation |
| Bookmark Post | ❌ 422 Error | ⚠️ Needs Investigation |

**Projected Success Rate**: 81.2% → ~90-95% (5-6 fixes out of 6)

