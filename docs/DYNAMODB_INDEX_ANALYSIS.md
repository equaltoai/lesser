# DynamoDB Index Usage Analysis
**Date**: 2025-10-22  
**Purpose**: Document current index structure and identify misuse patterns for Phase 2 fixes

---

## INDEX INVENTORY

### Notification Model

**Table PK/SK**:
- PK: `USER#{userID}` 
- SK: `notif#{timestamp}#{notificationID}`

**GSI1 - type-index**:
- GSI1PK: `NOTIF_TYPE#{type}` (mention, reblog, favourite, etc.)
- GSI1SK: `{created_at}#{userID}#{id}`
- **Purpose**: Query notifications by type across all users

**GSI2 - actor-index**:
- GSI2PK: `NOTIF_ACTOR#{actorID}`
- GSI2SK: `{created_at}#{userID}#{id}`
- **Purpose**: Find all notifications triggered by a specific actor

**GSI3 - group-index**:
- GSI3PK: `NOTIF_GROUP#{groupKey}`
- GSI3SK: `{created_at}#{id}`
- **Purpose**: Group related notifications for consolidation

### WebSocketConnection Model

**Table PK/SK**:
- PK: `CONN#{connectionID}`
- SK: `CONN#{connectionID}`

**GSI1**:
- GSI1PK: `USER#{userID}`
- GSI1SK: `CONN#{timestamp}`
- **Purpose**: Query all connections for a user

**GSI2**:
- GSI2PK: `STATE#{state}` (connected, idle, error, etc.)
- GSI2SK: `CONN#{connectionID}`
- **Purpose**: Query connections by state (for monitoring/cleanup)

### WebSocketSubscription Model

**Table PK/SK**:
- PK: `SUB#{stream}`
- SK: `CONN#{connectionID}`

**GSI1**:
- GSI1PK: `CONN#{connectionID}`
- GSI1SK: `STREAM#{stream}`
- **Purpose**: Query all subscriptions for a connection

---

## ANTI-PATTERNS IDENTIFIED

### Anti-Pattern #1: Index Scan Without Partition Key

**Example**: `notification_repository.go:459-472`

```go
// ❌ BAD - Scans entire group-index
query := r.db.Model(&models.Notification{}).
    Index("group-index")  // No WHERE on GSI3PK!
    
// Later filters client-side:
for _, notif := range allNotifications {
    if notif.UserID != userID {  // ❌ Should be in query
        continue
    }
}
```

**Why bad**: Scans the entire GSI, then filters in application code

**Fix**:
```go
// ✅ GOOD - Proper key condition
// But wait - group-index is GSI3PK=NOTIF_GROUP#{groupKey}
// Can't query by userID on this index!
// Need to use table PK instead or different GSI
query := r.db.Model(&models.Notification{}).
    Where("PK", "=", "USER#" + userID).  // Use table PK
    Limit(opts.Limit)
```

**Root issue**: Wrong index chosen for the query - should use table PK not group-index

---

### Anti-Pattern #2: Over-Fetch Then Filter

**Example**: `notification_repository.go:139-152`

```go
// ❌ BAD - Fetches 6x needed items
query.Limit((opts.Limit * 5) + opts.Limit + 5)  // Fetch 120 for limit=20
err := query.All(&notifications)

// Filter for notifications only
for i := range notifications {
    if strings.HasPrefix(notifications[i].SK, "notif#") {  // ❌ Filter in code
        filtered = append(filtered, &notifications[i])
    }
}
```

**Why bad**: Wastes RCUs fetching items we'll discard

**Fix**:
```go
// ✅ GOOD - Filter at query level
query := r.db.Model(&models.Notification{}).
    Where("PK", "=", "USER#" + userID).
    Where("SK", "begins_with", "notif#").  // DynamoDB filter
    OrderBy("SK", "DESC").
    Limit(opts.Limit)  // Fetch exactly what we need
err := query.All(&notifications)
```

---

### Anti-Pattern #3: Fetch-All for Count

**Example**: `streaming_connection_repository.go:432-446`

```go
// ❌ BAD - Fetches all items to count
connectedConns, _ := r.GetConnectionsByState(ctx, Connected)
idleConns, _ := r.GetConnectionsByState(ctx, Idle)
return len(connectedConns) + len(idleConns), nil
```

**Why bad**: Fetches entire items (with all attributes) just to count

**Fix**:
```go
// ✅ GOOD - Use Count()
connectedCount, _ := r.GetDB().Model(&models.WebSocketConnection{}).
    Index("GSI2").
    Where("GSI2PK", "=", "STATE#connected").
    Count()
    
idleCount, _ := r.GetDB().Model(&models.WebSocketConnection{}).
    Index("GSI2").
    Where("GSI2PK", "=", "STATE#idle").
    Count()
    
return connectedCount + idleCount, nil
```

---

## PHASE 2 IMPLEMENTATION PLAN

### Part A: Index Misuse Fixes (Priority: CRITICAL)

#### 1. notification_repository.go

**GetNotificationGroups (lines 449-524)**:
- **Issue**: Uses `Index("group-index")` without WHERE on GSI3PK
- **Current**: Scans entire index, filters client-side for userID
- **Fix**: Can't use group-index for user queries - use table PK instead
- **Change**: Remove Index(), use `Where("PK", "=", "USER#" + userID)`

**GetUserNotifications (lines 121-173)**:
- **Issue**: Over-fetches 6x items, filters by SK prefix client-side
- **Fix**: Use `Where("SK", "begins_with", "notif#")` in query
- **Remove**: Client-side prefix filtering loop

**GetUnreadNotifications (lines 175-229)**:
- **Issue**: Same as above - over-fetch and filter
- **Fix**: Use begins_with() for SK filter

**ConsolidateNotifications (lines 533-580)**:
- **Issue**: No limit on GSI query
- **Fix**: Add `.Limit(100)` and handle truncation

#### 2. streaming_connection_repository.go

**GetConnectionsByState (lines 728-744)**:
- **Issue**: Already uses proper WHERE on GSI2PK ✓
- **Missing**: No limit
- **Fix**: Add `.Limit(1000)` 

**GetConnectionsByUser (lines 655-673)**:
- **Issue**: Already uses proper WHERE on GSI1PK ✓
- **Missing**: No limit
- **Fix**: Add `.Limit(100)`

**Replace Fetch-and-Count**:
- Create `GetConnectionCountByState()`
- Create `GetUserConnectionCount()`
- Update `GetTotalActiveConnectionsCount()` to use counts
- Update `GetActiveConnectionsCount()` to use count

---

### Part B: Add Defensive Limits (Priority: HIGH)

For all remaining .All() queries that already use proper WHERE:
- Add `.Limit(100)` or appropriate limit for use case
- Add pagination support where needed
- Log warning if result count == limit (truncated)

---

## DETAILED FIX SPECIFICATIONS

### Fix #1: GetNotificationGroups

**Current (lines 449-524)**:
```go
query := r.db.Model(&models.Notification{}).
    Index("group-index")  // ❌ Wrong index for this query
```

**Fixed**:
```go
// Use table PK since we're querying by userID
query := r.db.Model(&models.Notification{}).
    Where("PK", "=", "USER#" + userID).
    Where("SK", "begins_with", "notif#").
    OrderBy("SK", "DESC").
    Limit(opts.Limit * 2)  // Account for grouping

// Rest of grouping logic stays the same
```

---

### Fix #2: GetUserNotifications

**Current (lines 139-152)**:
```go
query.Limit((opts.Limit * 5) + opts.Limit + 5)  // ❌ Over-fetch
err := query.All(&notifications)

for i := range notifications {
    if strings.HasPrefix(notifications[i].SK, "notif#") {  // ❌ Client filter
        filtered = append(filtered, &notifications[i])
    }
}
```

**Fixed**:
```go
query := r.db.Model(&models.Notification{}).
    Where("PK", "=", "USER#" + userID).
    Where("SK", "begins_with", "notif#").  // ✅ DynamoDB filter
    OrderBy("SK", "DESC").
    Limit(opts.Limit)  // ✅ Exact amount needed

err := query.All(&notifications)
// No filtering loop needed
```

---

### Fix #3: Connection Count Methods

**Create new Count methods**:
```go
func (r *StreamingConnectionRepository) GetConnectionCountByState(
    ctx context.Context, 
    state models.ConnectionState,
) (int, error) {
    count, err := r.GetDB().WithContext(ctx).
        Model(&models.WebSocketConnection{}).
        Index("GSI2").
        Where("GSI2PK", "=", fmt.Sprintf("STATE#%s", state)).
        Count()
    
    if err != nil {
        return 0, ErrorHandler.HandleQueryError(err, "connection count", string(state))
    }
    
    return count, nil
}

func (r *StreamingConnectionRepository) GetUserConnectionCount(
    ctx context.Context,
    userID string,
) (int, error) {
    count, err := r.GetDB().WithContext(ctx).
        Model(&models.WebSocketConnection{}).
        Index("GSI1").
        Where("GSI1PK", "=", fmt.Sprintf("USER#%s", userID)).
        Count()
    
    if err != nil {
        return 0, ErrorHandler.HandleQueryError(err, "user connection count", userID)
    }
    
    return count, nil
}
```

**Update existing methods to use counts**:
```go
func (r *StreamingConnectionRepository) GetTotalActiveConnectionsCount(ctx) (int, error) {
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
```

---

## IMPLEMENTATION ORDER

### Day 1: Critical Notification Fixes
1. Fix GetUserNotifications (remove over-fetch, use begins_with)
2. Fix GetUnreadNotifications (same)
3. Fix GetNotificationGroups (use correct index/PK)
4. Add limit to ConsolidateNotifications
5. Test notification endpoints

### Day 2: Connection Count Optimization
1. Create GetConnectionCountByState()
2. Create GetUserConnectionCount()
3. Update GetTotalActiveConnectionsCount()
4. Update GetActiveConnectionsCount()
5. Re-enable checkConnectionLimits() with efficient queries
6. Test WebSocket connection limits

### Day 3: Add Defensive Limits
1. streaming_connection_repository.go remaining queries
2. relationship_repository.go queries
3. social_repository.go queries
4. status_repository.go queries

### Day 4: Verification
1. Test all endpoints
2. Verify query patterns in CloudWatch
3. Check RCU consumption
4. Load test

---

## VERIFICATION

After each fix:
```bash
# Build
make build

# Deploy
AWS_PROFILE=Lesser make deploy-graphql-ws

# Test notifications
curl -X POST https://dev.lesser.host/api/graphql \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"query":"query { notifications { id type } }"}'

# Check CloudWatch for query timing
AWS_PROFILE=Lesser aws logs tail /aws/lambda/lesser-development-graphql-ws \
  --since 5m --region us-east-1 | grep "elapsed"
```

After all fixes:
```bash
# Verify no unbounded scans
rg "\.All\(" pkg/storage/repositories/notification_repository.go -B 3 | \
  rg "Index\(" | \
  rg -v "Limit\("  # Should return empty

# Verify Count() usage
rg "\.Count\(\)" pkg/storage/repositories --type go | wc -l
# Should increase significantly
```

---

## SUCCESS METRICS

- [ ] GetUserNotifications: <50ms (currently varies)
- [ ] GetNotificationGroups: <100ms (currently may timeout)
- [ ] Connection limit checks: <50ms (currently instant since no connections)
- [ ] All queries have explicit limits
- [ ] No client-side filtering of query results
- [ ] .Count() used for all count operations

---

**This is the proper comprehensive fix - no band-aids, fix the architecture.**

