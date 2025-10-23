# DynamoDB Architecture Fix Plan
**Date**: 2025-10-22  
**Status**: COMPREHENSIVE FIX - No Band-aids

---

## PRINCIPLES

1. **ALL operations must be synchronous** - No goroutines in data layer
2. **ALL queries must use proper keys** - No scans disguised as queries
3. **ALL limits must be enforced at query level** - Not by fetching everything
4. **ALL operations must complete before Lambda returns** - No background tasks

---

## CRITICAL ARCHITECTURE PROBLEMS

### Problem #1: Asynchronous Event Emission

**File**: `pkg/storage/repositories/services.go:457-464`

**Current**:
```go
func EmitAsync(ctx context.Context, event Event) error {
    go func() {
        _ = e.Emit(ctx, event)  // ❌ Async with request context
    }()
    return nil
}
```

**Fix**: Call the synchronous emitter directly
```go
_ = r.events.Emit(ctx, event)
```

**Impact**: 4 call sites in enhanced_base_repository.go need updating

---

### Problem #2: Connection Count Requires Scanning

**File**: `pkg/storage/repositories/streaming_connection_repository.go:432-446`

**Current**:
```go
func GetTotalActiveConnectionsCount(ctx) (int, error) {
    connected, _ := GetConnectionsByState(ctx, Connected)  // Fetches ALL
    idle, _ := GetConnectionsByState(ctx, Idle)           // Fetches ALL
    return len(connected) + len(idle), nil
}
```

**Fix**: Use Count() queries
```go
func GetTotalActiveConnectionsCount(ctx) (int, error) {
    connectedCount, err := r.GetConnectionCountByState(ctx, Connected)
    if err != nil {
        return 0, err
    }
    
    idleCount, err := r.GetConnectionCountByState(ctx, Idle)
    if err != nil {
        return 0, err
    }
    
    return connectedCount + idleCount, nil
}

func GetConnectionCountByState(ctx, state) (int, error) {
    return r.GetDB().WithContext(ctx).
        Model(&models.WebSocketConnection{}).
        Index("GSI2").
        Where("GSI2PK", "=", fmt.Sprintf("STATE#%s", state)).
        Count()
}
```

---

### Problem #3: GetConnectionsByUser/Stream Without Limits

**Files**: 
- `streaming_connection_repository.go:655-673` (GetConnectionsByUser)
- `streaming_connection_repository.go:675-693` (GetSubscriptionsForStream)
- `streaming_connection_repository.go:728-744` (GetConnectionsByState)

**Current Pattern**:
```go
func GetConnectionsByUser(ctx, userID) ([]Connection, error) {
    var connections []Connection
    err := r.GetDB().Model(&Connection{}).
        Index("GSI1").
        Where("GSI1PK", "=", fmt.Sprintf("USER#%s", userID)).
        All(&connections)  // ❌ No limit
    return connections, err
}
```

**Fix**: Add mandatory limits, implement pagination
```go
func GetConnectionsByUser(ctx, userID string, limit int) ([]Connection, error) {
    if limit <= 0 || limit > 100 {
        limit = 100  // Enforce max limit
    }
    
    var connections []Connection
    err := r.GetDB().Model(&Connection{}).
        Index("GSI1").
        Where("GSI1PK", "=", fmt.Sprintf("USER#%s", userID)).
        Limit(limit).
        All(&connections)
    return connections, err
}
```

---

## COMPLETE FIX PLAN

### Step 1: Make Event Emission Synchronous

**Files to modify**:
- `pkg/storage/repositories/services.go`

**Changes**:
1. Remove `go func()` wrapper from `EmitAsync()`
2. Drop the redundant `EmitAsync()` helper
3. Call `Emit()` directly at all 4 call sites in `enhanced_base_repository.go`

**Result**: No more goroutines using request context

---

### Step 2: Fix Connection Limit Checks

**Files to modify**:
- `pkg/storage/repositories/streaming_connection_repository.go`

**Changes**:
1. Create `GetConnectionCountByState()` using `.Count()`
2. Update `GetTotalActiveConnectionsCount()` to use counts
3. Create `GetUserConnectionCount()` using `.Count()`
4. Update `GetActiveConnectionsCount()` to use count
5. Keep `checkConnectionLimits()` enabled with efficient queries

**Result**: Connection limits work without scans

---

### Step 3: Add Limits to All List Queries

**Files to modify** (9 files):
1. `streaming_connection_repository.go`
2. `notification_repository.go`
3. `relationship_repository.go`
4. `status_repository.go`
5. `social_repository.go`
6. `announcement_repository.go`
7. `scheduled_status_repository.go`
8. `media_repository.go`
9. `emoji_repository.go`

**Changes per file**:
1. Find all `.All()` calls
2. Add `.Limit(100)` before `.All()`
3. For methods with pagination params, use the param
4. For methods without limits, add limit parameter

**Pattern**:
```go
// Old signature
func GetItems(ctx) ([]Item, error)

// New signature
func GetItems(ctx, limit int) ([]Item, error)
    if limit <= 0 || limit > 1000 {
        limit = 100  // Default limit
    }
    query.Limit(limit).All(&items)
```

---

### Step 4: Implement Count-Based Queries

**New methods to create**:
```go
// streaming_connection_repository.go
func GetConnectionCountByState(ctx, state) (int, error)
func GetUserConnectionCount(ctx, userID) (int, error)  
func GetStreamSubscriptionCount(ctx, stream) (int, error)

// notification_repository.go
func GetUnreadNotificationCount(ctx, userID) (int, error)

// relationship_repository.go
func GetFollowerCount(ctx, username) (int, error)
func GetFollowingCount(ctx, username) (int, error)

// status_repository.go
func GetUserStatusCount(ctx, userID) (int, error)
```

**Pattern**:
```go
func GetXCount(ctx, key) (int, error) {
    return r.GetDB().WithContext(ctx).
        Model(&Model{}).
        Index("GSI").
        Where("GSIPK", "=", key).
        Count()
}
```

---

### Step 5: Remove Enhanced Repository for High-Frequency Writes

**Target**: WebSocket subscriptions (ephemeral, don't need validation)

**Files**:
- `streaming_connection_repository.go:195`

**Change**:
```go
// Before
err := r.subscriptionRepo.ValidateAndCreate(ctx, subscription)

// After - direct create
subscription.UpdateKeys()
err := r.subscriptionRepo.Create(ctx, subscription)
```

**Alternative**: Create a `FastCreate()` method that skips validation for known-good data

---

## IMPLEMENTATION CHECKLIST

### Phase 1: Synchronous Operations (Day 1)
- [ ] Remove goroutine-based event emitters in services.go
- [ ] Make emission fully synchronous
- [ ] Test that events still work
- [ ] Deploy and verify no context errors

### Phase 2: Connection Limits (Day 1)
- [ ] Create `GetConnectionCountByState()`
- [ ] Create `GetUserConnectionCount()`
- [ ] Update `GetTotalActiveConnectionsCount()` to use counts
- [ ] Update `GetActiveConnectionsCount()` to use count
- [ ] Test connection limit enforcement

### Phase 3: Streaming Repository (Day 2)
- [ ] Add limits to `GetConnectionsByUser()`
- [ ] Add limits to `GetConnectionsByState()`
- [ ] Add limits to `GetSubscriptionsForStream()`
- [ ] Replace ValidateAndCreate with Create for subscriptions
- [ ] Test subscription writes (<100ms)

### Phase 4: Notification Repository (Day 2)
- [ ] Add limits to all `.All()` queries
- [ ] Create `GetUnreadNotificationCount()`
- [ ] Update callers to use limits

### Phase 5: Relationship Repository (Day 3)
- [ ] Add limits to following/followers queries
- [ ] Create `GetFollowerCount()`, `GetFollowingCount()`
- [ ] Implement cursor pagination for large lists

### Phase 6: Status Repository (Day 3)
- [ ] Add limits to timeline queries
- [ ] Add limits to status list queries
- [ ] Ensure all have pagination

### Phase 7: Remaining Repositories (Day 4)
- [ ] Social (blocks, mutes)
- [ ] Media queries
- [ ] Scheduled status queries
- [ ] Emoji queries
- [ ] Announcement queries

### Phase 8: Verification (Day 5)
- [ ] Run: `rg "go func" pkg/storage --type go` → 0 results
- [ ] Run: `rg "\.All\(" pkg/storage --type go -B 2 | grep -v "Limit"` → Only valid cases
- [ ] Load test: 1000 concurrent requests
- [ ] Verify all operations <100ms
- [ ] Check CloudWatch for no context errors

---

## CODE PATTERNS

### ✅ CORRECT: Synchronous with Limit
```go
func GetItems(ctx, limit int) ([]Item, error) {
    if limit <= 0 || limit > 1000 {
        limit = 100
    }
    
    var items []Item
    err := r.GetDB().WithContext(ctx).
        Model(&Item{}).
        Where("PK", "=", key).
        Limit(limit).
        All(&items)
    return items, err
}
```

### ✅ CORRECT: Count Query
```go
func GetItemCount(ctx, key) (int, error) {
    return r.GetDB().WithContext(ctx).
        Model(&Item{}).
        Where("PK", "=", key).
        Count()
}
```

### ✅ CORRECT: Paginated Query
```go
func GetItems(ctx, limit int, cursor string) ([]Item, string, error) {
    query := r.GetDB().Model(&Item{}).Limit(limit)
    
    if cursor != "" {
        query = query.Where("SK", ">", cursor)
    }
    
    var items []Item
    err := query.All(&items)
    
    nextCursor := ""
    if len(items) == limit {
        nextCursor = items[len(items)-1].SK
    }
    
    return items, nextCursor, err
}
```

### ❌ WRONG: Unbounded Scan
```go
func GetItems(ctx) ([]Item, error) {
    var items []Item
    err := r.GetDB().Model(&Item{}).All(&items)  // NO LIMIT
    return items, err
}
```

### ❌ WRONG: Goroutine with Request Context
```go
go func() {
    _ = doSomething(ctx)  // ctx will be canceled
}()
```

### ❌ WRONG: Fetch All Then Count
```go
items, _ := GetAll(ctx)
return len(items), nil  // Should use Count()
```

---

## SUCCESS CRITERIA

**After all fixes**:
- ✅ Subscription writes: <100ms (currently: 30s+)
- ✅ Zero "context deadline exceeded" errors
- ✅ Zero goroutines in pkg/storage
- ✅ All queries have explicit limits
- ✅ Count queries use `.Count()`, not fetch-and-count
- ✅ GraphQL subscriptions work end-to-end

---

**Ready to implement Phase 1?**
