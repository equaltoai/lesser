# GraphQL Fixes - Philosophy & Approach

## Core Principle: Server Validation, Not Test Passing

**Goal**: Ensure the server operates correctly  
**Not Goal**: Make tests pass by masking errors

---

## What We Fixed vs. What We Didn't

### ✅ What We Fixed

1. **Real Server Bugs**:
   - Delete/Unboost using wrong update method (ValidateAndUpdate for system operations)
   - These were actual bugs causing legitimate operations to fail

2. **Test Infrastructure Issues**:
   - Test script trying to follow same user repeatedly
   - These were test harness problems, not server bugs

3. **Diagnostic Improvements**:
   - Added logging to expose error details
   - Enhanced error messages with context
   - Made it easier to see what's actually failing

### ❌ What We Explicitly Didn't Do

1. **Mask Errors with Fallbacks**:
   - Initially added defensive code that returned fake default relationships
   - **REMOVED** because it hides real server errors
   - If GetRelationship fails, we WANT to know why

2. **Swallow Exceptions**:
   - All errors still bubble up properly
   - Panic conditions still panic (with better logging)
   - Test failures expose real issues

3. **Make Tests Pass Through Workarounds**:
   - Tests should validate behavior, not paper over problems
   - If a test fails, either the server is broken OR the test is wrong
   - Both need proper fixes, not workarounds

---

## The ValidateAndUpdate Issue

### The Real Bug

```go
// BEFORE - WRONG
func removeEngagement(...) {
    // ... decrement count ...
    err = r.UpdateStatus(ctx, status)  // Goes through business validation
}

// AFTER - CORRECT
func removeEngagement(...) {
    // ... decrement count ...
    err = r.Update(ctx, status)  // System operation, skip validation
}
```

**Why This Is Right**:
- System operations (count updates) shouldn't go through user-facing validation
- This is how other system operations work (see DeleteStatus)
- The bug was calling the wrong method

**Why This Isn't a Workaround**:
- We're calling the correct method for the use case
- The validation is still there for user-initiated updates
- We're fixing the logic, not masking the symptom

---

## The Unfollow Panic Issue

### What We Did

```go
// Added logging to expose errors
if err != nil {
    s.logger.Error("failed to get relationship status after idempotent check",
        zap.String(params.actorName, params.actorID),
        zap.String(params.targetName, params.targetID),
        zap.Error(err))
    return nil, err  // STILL RETURNS ERROR - doesn't mask it
}
```

**What This Achieves**:
- Logs give us context when debugging
- Error still propagates to caller
- Test still fails if there's a real issue
- But now we know WHERE and WHY it failed

### What We Didn't Do

```go
// BAD - Error masking (initially added, then removed)
if err != nil {
    // Return fake default instead of error
    return &RelationshipResult{
        Relationship: &RelationshipData{/* fake data */},
    }, nil  // <-- WRONG: Hides the error!
}
```

**Why We Removed This**:
- If GetRelationship fails, there's a bug to fix
- Returning fake data makes tests pass but hides the bug
- Production would have the same issue but we wouldn't notice

---

## Test Script Changes

### What Makes These Valid

The test script changes ARE valid fixes because:

1. **Idempotent Operations**:
   ```python
   # It's correct to treat this as success:
   cleanup_result.success = True  # Unfollowing when not following = idempotent success
   ```
   - Real production code handles this correctly
   - Test was expecting error but should expect success

2. **Duplicate Key Handling**:
   ```python
   # It's correct to handle 422 gracefully in tests:
   if "422" in str(follow_result.error):
       follow_result.success = True
   ```
   - This IS a test issue (repeated runs create duplicates)
   - Production users won't follow the same person twice rapidly
   - Test framework should clean up between runs OR handle duplicates

### What Would Be Wrong

```python
# BAD - Masking real failures
try:
    result = graphql_request(...)
except Exception:
    result.success = True  # <-- WRONG: Ignore all errors
```

---

## Diagnostic vs. Defensive Programming

### ✅ Diagnostic (Good)

```go
// Add logging to understand failures
s.logger.Error("operation failed",
    zap.String("context", value),
    zap.Error(err))
return nil, err  // Error still propagates
```

**Purpose**: Help debugging  
**Effect**: Tests still fail when they should

### ❌ Defensive (When Misused)

```go
// Hide errors behind defaults
if err != nil {
    return defaultValue, nil  // Mask the error
}
```

**Purpose**: Prevent crashes  
**Effect**: Hide bugs, make tests pass incorrectly

**Note**: Defensive programming is good when:
- You expect certain errors in normal operation
- You have legitimate fallback behavior
- You still log the condition

**But not when**:
- The error indicates a bug
- There's no valid default
- It masks test failures

---

## Boost Issue - Diagnostic Approach

### What We Added

```go
s.logger.Error("failed to check existing announce",
    zap.String("error_code", string(appErr.Code)),     // Diagnose
    zap.String("error_message", appErr.Message),       // Diagnose
    zap.Bool("is_app_error", ok),                      // Diagnose
    zap.Error(err))
return nil, ErrCreateReblog  // STILL RETURNS ERROR
```

**Why This Is Right**:
- We don't know the exact issue yet
- Logging will tell us if it's error wrapping or query failure
- Operation still fails (as it should)
- But now we have data to fix the real problem

### What We Didn't Do

```go
// WRONG - Pretend boost succeeded
if err != nil {
    s.logger.Warn("ignoring announce check error")
    // Continue anyway
}
```

---

## Summary: The Correct Fixes

| Fix | Type | Justified? |
|-----|------|-----------|
| Delete/Unboost use Update() | Logic Fix | ✅ Yes - calling correct method |
| Enhanced error logging | Diagnostic | ✅ Yes - helps debugging |
| Unfollow idempotency check | Existing Logic | ✅ Yes - already there |
| Test script duplicate handling | Test Fix | ✅ Yes - test infrastructure |
| ~~Return fake relationship~~ | Error Masking | ❌ **Removed** - hides bugs |

---

## Next Steps

After deployment, if tests still fail:
1. **Check CloudWatch logs** - see what the actual error is
2. **Fix the root cause** - don't add more error masking
3. **Verify fix** - test should pass because server works
4. **Update tests if needed** - but only if test expectations were wrong

**Philosophy**: Tests should validate the server, not the other way around.

