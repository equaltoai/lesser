# Phase 2 Implementation Prompt: Fix DynamoDB Index Usage

**Based on comprehensive codebase analysis completed 2025-10-22**

---

## CONTEXT

Phase 1 eliminated goroutines from repository layer. Phase 2 fixes inefficient DynamoDB access patterns identified through systematic index analysis.

**Analysis documents**:
- Index inventory: `docs/DYNAMODB_INDEX_ANALYSIS.md`
- Architecture fix plan: `docs/DYNAMODB_ARCHITECTURE_FIX.md`

---

## OBJECTIVE

Replace scan-and-filter patterns with proper index usage. Fix 164 .All() queries by:
1. Using proper GSI partition key WHERE clauses
2. Using begins_with() for prefix matching (not client-side filtering)
3. Using .Count() for counts (not fetch-and-count)
4. Adding .Limit() as defense-in-depth

**PRINCIPLE**: Query efficiently at the database level, not fetch-all-and-filter in application code.

---

## IMPLEMENTATION TASKS

### Task 1: Fix GetUserNotifications Over-Fetch Pattern

**File**: `pkg/storage/repositories/notification_repository.go:121-173`

**Current anti-pattern**:
```go
// Fetches 6x needed items
query.Limit((opts.Limit * 5) + opts.Limit + 5)  // 120 items for limit=20
err := query.All(&notifications)

// Filters client-side by SK prefix
filtered := make([]*models.Notification, 0, len(notifications))
for i := range notifications {
    if strings.HasPrefix(notifications[i].SK, "notif#") {  // Client filter
        filtered = append(filtered, &notifications[i])
    }
}
```

**Fix to**:
```go
// Query exactly what we need with DynamoDB filter
query := r.db.WithContext(ctx).Model(&models.Notification{}).
    Where("PK", "=", "USER#" + userID).
    Where("SK", "begins_with", "notif#").  // ✅ DynamoDB-level filter
    OrderBy("SK", "DESC").
    Limit(opts.Limit + 1)  // +1 to detect hasMore

if opts.Cursor != "" {
    query = query.Where("SK", "<", opts.Cursor)
}

var notifications []models.Notification
err := query.All(&notifications)

// No client-side filtering needed - all results are valid
hasMore := len(notifications) > opts.Limit
if hasMore {
    notifications = notifications[:opts.Limit]
}

result := &interfaces.PaginatedResult[*models.Notification]{
    Items:   convertToPointers(notifications),
    HasMore: hasMore,
}
if hasMore {
    result.NextCursor = notifications[opts.Limit-1].SK
}
```

**Validation**: 
- Over-fetch factor reduced from 6x to 1.05x (limit+1)
- No client-side filtering loop
- RCUs reduced by ~83%

---

### Task 2: Fix GetUnreadNotifications (Same Pattern)

**File**: `pkg/storage/repositories/notification_repository.go:175-229`

Apply same fix as Task 1:
- Remove 6x over-fetch
- Add `Where("SK", "begins_with", "notif#")`
- Remove client-side filtering loop

---

### Task 3: Fix GetNotificationGroups Index Misuse

**File**: `pkg/storage/repositories/notification_repository.go:449-524`

**Current anti-pattern**:
```go
// Uses group-index without GSI3PK WHERE
query := r.db.Model(&models.Notification{}).
    Index("group-index")  // ❌ Scans entire index

// Later filters for userID client-side
for i := range allNotifications {
    if notif.UserID != userID {  // ❌ Wrong! Can't filter by userID on group-index
        continue
    }
}
```

**Problem**: group-index has GSI3PK=`NOTIF_GROUP#{groupKey}`, not userID. Can't efficiently query by user on this index.

**Fix to**:
```go
// Use table PK instead of group-index
query := r.db.Model(&models.Notification{}).
    Where("PK", "=", "USER#" + userID).
    Where("SK", "begins_with", "notif#").
    OrderBy("SK", "DESC").
    Limit(opts.Limit * 3)  // Some overhead for grouping logic

if opts.Cursor != "" {
    query = query.Where("SK", "<", opts.Cursor)
}

var allNotifications []models.Notification
err := query.All(&allNotifications)

// Grouping logic stays the same (client-side is appropriate here)
// But now we're only processing notifications for this user
```

**Validation**:
- Uses correct key path (table PK not wrong GSI)
- Reduced over-fetch from unlimited to 3x
- Client-side grouping is acceptable (can't be done in DynamoDB)

---

### Task 4: Add Limit to ConsolidateNotifications

**File**: `pkg/storage/repositories/notification_repository.go:533-580`

**Current**:
```go
err := r.db.Model(&models.Notification{}).
    Index("group-index").
    Where("GSI3PK", "=", "NOTIF_GROUP#"+groupKey).
    All(&notifications)  // ❌ No limit
```

**Fix to**:
```go
err := r.db.Model(&models.Notification{}).
    Index("group-index").
    Where("GSI3PK", "=", "NOTIF_GROUP#"+groupKey).
    Limit(100).  // ✅ Defensive limit
    All(&notifications)

if len(notifications) == 100 {
    r.logger.Warn("notification group truncated at limit",
        zap.String("group_key", groupKey),
        zap.Int("limit", 100))
}
```

---

### Task 5: Verify/Fix GetNotificationsAdvanced Index

**File**: `pkg/storage/repositories/notification_repository.go:~710`

**Current**:
```go
query := r.db.Model(&models.Notification{}).
    Index("user-notifications-index").  // ❌ Index not defined in model?
    Filter("UserID", "=", userID)
```

**Investigation needed**:
- Check if "user-notifications-index" exists in model definition
- If not, replace with table PK: `Where("PK", "=", "USER#" + userID)`
- Add proper limit

---

### Task 6: Create Count-Based Connection Limit Methods

**File**: `pkg/storage/repositories/streaming_connection_repository.go`

**Add new methods** (after line 744):
```go
// GetConnectionCountByState returns count of connections in a specific state
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
        return 0, ErrorHandler.HandleQueryError(err, "connection count by state", string(state))
    }
    
    return count, nil
}

// GetUserConnectionCount returns count of connections for a user
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

**Update existing methods** (lines 705-746):
```go
// GetActiveConnectionsCount - Replace fetch-all with Count
func (r *StreamingConnectionRepository) GetActiveConnectionsCount(
    ctx context.Context, 
    userID string,
) (int, error) {
    // Use efficient count query instead of fetching all
    return r.GetUserConnectionCount(ctx, userID)
}

// GetTotalActiveConnectionsCount - Replace fetch-all with Count
func (r *StreamingConnectionRepository) GetTotalActiveConnectionsCount(ctx context.Context) (int, error) {
    connectedCount, err := r.GetConnectionCountByState(ctx, models.ConnectionStateConnected)
    if err != nil {
        return 0, err
    }
    
    idleCount, err := r.GetConnectionCountByState(ctx, models.ConnectionState Idle)
    if err != nil {
        return 0, err
    }
    
    return connectedCount + idleCount, nil
}
```

**Re-enable connection limits** (line 65):
```go
// Now efficient with Count() queries - can re-enable
if err := r.checkConnectionLimits(ctx, userID); err != nil {
    return ErrorHandler.HandleCreateError(err, "streaming connection", "connection limit check")
}
```

---

### Task 7: Add Limits to Remaining Queries

**Files** (in priority order):
1. `streaming_connection_repository.go:662,680` - Add `.Limit(100)`
2. `notification_repository.go` - Any remaining unbounded queries
3. `relationship_repository.go` - Add limits to all `.All()` 
4. `social_repository.go` - Add limits to block/mute queries
5. `status_repository.go` - Verify timeline queries have limits

**Pattern**:
```go
// Add limit before .All()
query.Limit(100).All(&results)

// Or use method parameter
query.Limit(limit).All(&results)
```

---

## TESTING AFTER EACH TASK

```bash
# Build
make build

# Deploy
AWS_PROFILE=Lesser make deploy-graphql-ws

# Test notifications API
curl -X POST https://dev.lesser.host/api/graphql \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"query { notifications(limit: 20) { id type createdAt } }"}'

# Check query timing in logs
AWS_PROFILE=Lesser aws logs tail /aws/lambda/lesser-development-graphql-ws \
  --since 2m --region us-east-1 | grep "elapsed\|duration"

# Verify no errors
AWS_PROFILE=Lesser aws logs tail /aws/lambda/lesser-development-graphql-ws \
  --since 2m --region us-east-1 | grep "error\|failed"
```

---

## SUCCESS CRITERIA

After completing all tasks:
- [ ] GetUserNotifications uses begins_with(), no client filtering
- [ ] GetUnreadNotifications uses begins_with(), no client filtering  
- [ ] GetNotificationGroups uses table PK not group-index
- [ ] All connection count methods use .Count()
- [ ] checkConnectionLimits re-enabled with efficient queries
- [ ] All queries have explicit limits
- [ ] No undefined indexes referenced
- [ ] Build succeeds
- [ ] Notification queries <50ms
- [ ] Connection limit checks <50ms
- [ ] Zero RCU spikes from scans

---

## VERIFICATION COMMANDS

```bash
# Check for over-fetch patterns
rg "Limit.*\*.*\+" pkg/storage/repositories/notification_repository.go

# Check for client-side filtering
rg "strings.HasPrefix.*SK" pkg/storage/repositories/notification_repository.go

# Check all queries have limits
rg "\.All\(" pkg/storage/repositories/notification_repository.go -B 3 | \
  grep "Index\|Where\|All" | \
  grep -v "Limit"

# Verify Count() usage increased
rg "\.Count\(\)" pkg/storage/repositories/streaming_connection_repository.go

# Check for undefined indexes
rg "Index\(\"user-notifications-index\"\)" pkg/storage
```

---

**START WITH**: Task 1 (GetUserNotifications) - it's the most impactful and demonstrates the pattern for other fixes.

