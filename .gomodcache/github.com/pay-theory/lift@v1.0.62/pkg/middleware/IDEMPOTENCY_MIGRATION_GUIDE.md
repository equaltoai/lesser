# Migrating to Lift Idempotency Middleware

This guide helps teams migrate from custom idempotency implementations to Lift's built-in idempotency middleware.

## Common Migration Scenarios

### 1. From Custom Database Table

If you currently store idempotency keys in a custom database table:

**Before (Custom Implementation):**
```go
func paymentHandler(ctx *lift.Context) error {
    idempKey := ctx.Header("X-Idempotency-Key")
    
    // Check if already processed
    existing, err := db.Query("SELECT response FROM idempotency WHERE key = ?", idempKey)
    if err == nil && existing != nil {
        return ctx.JSON(existing.Response)
    }
    
    // Process payment
    result := processPayment()
    
    // Store for idempotency
    db.Exec("INSERT INTO idempotency (key, response) VALUES (?, ?)", 
        idempKey, result)
    
    return ctx.JSON(result)
}
```

**After (Lift Middleware):**
```go
// One-time setup
app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
    Store:      store,
    HeaderName: "X-Idempotency-Key", // Match your existing header
})))

// Simplified handler - no idempotency logic needed!
func paymentHandler(ctx *lift.Context) error {
    result := processPayment()
    return ctx.JSON(result)
}
```

### 2. From Redis-Based Implementation

**Before:**
```go
func createOrderHandler(ctx *lift.Context) error {
    key := ctx.Header("Idempotency-Key")
    
    // Check Redis
    cached, _ := redisClient.Get(ctx, "idem:"+key).Result()
    if cached != "" {
        var response OrderResponse
        json.Unmarshal([]byte(cached), &response)
        return ctx.JSON(response)
    }
    
    // Create order
    order := createOrder()
    
    // Cache in Redis
    data, _ := json.Marshal(order)
    redisClient.Set(ctx, "idem:"+key, data, 24*time.Hour)
    
    return ctx.JSON(order)
}
```

**After:**
```go
// Create Redis-backed store (you implement this interface)
type RedisIdempotencyStore struct {
    client *redis.Client
}

func (r *RedisIdempotencyStore) Get(ctx context.Context, key string) (*middleware.IdempotencyRecord, error) {
    // Implementation
}

func (r *RedisIdempotencyStore) Set(ctx context.Context, key string, record *middleware.IdempotencyRecord) error {
    // Implementation
}

// Use it
store := &RedisIdempotencyStore{client: redisClient}
app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
    Store: store,
})))

// Clean handler
func createOrderHandler(ctx *lift.Context) error {
    order := createOrder()
    return ctx.JSON(order)
}
```

### 3. From In-Handler Checks

**Before:**
```go
var processedRequests = make(map[string]interface{})
var mu sync.Mutex

func handler(ctx *lift.Context) error {
    key := ctx.Header("Request-ID")
    
    mu.Lock()
    if result, exists := processedRequests[key]; exists {
        mu.Unlock()
        return ctx.JSON(result)
    }
    mu.Unlock()
    
    // Process
    result := doWork()
    
    mu.Lock()
    processedRequests[key] = result
    mu.Unlock()
    
    return ctx.JSON(result)
}
```

**After:**
```go
// Just add middleware
app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
    Store:      middleware.NewMemoryIdempotencyStore(),
    HeaderName: "Request-ID", // Match your header
})))

// Clean handler
func handler(ctx *lift.Context) error {
    result := doWork()
    return ctx.JSON(result)
}
```

## Migration Strategy

### Phase 1: Parallel Running (1-2 weeks)

Run both systems simultaneously to ensure compatibility:

```go
// Wrapper to use both systems
func DualIdempotencyMiddleware(oldStore, newStore IdempotencyStore) lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            key := ctx.Header("Idempotency-Key")
            if key == "" {
                return next.Handle(ctx)
            }
            
            // Check old system
            oldResult, _ := checkOldSystem(key)
            
            // Check new system  
            newResult, _ := newStore.Get(ctx.Request.Context(), key)
            
            // Log discrepancies
            if oldResult != nil && newResult == nil {
                log.Printf("Key %s found in old but not new", key)
            }
            
            // Use new middleware
            return middleware.Idempotency(middleware.IdempotencyOptions{
                Store: newStore,
            })(next).Handle(ctx)
        })
    }
}
```

### Phase 2: Migration Script

Migrate existing idempotency records:

```go
func migrateIdempotencyRecords() error {
    // Read from old system
    oldRecords := queryOldIdempotencyTable()
    
    // Convert to new format
    for _, old := range oldRecords {
        record := &middleware.IdempotencyRecord{
            Key:        old.Key,
            Status:     "completed",
            Response:   old.Response,
            StatusCode: old.StatusCode,
            CreatedAt:  old.CreatedAt,
            ExpiresAt:  old.CreatedAt.Add(24 * time.Hour),
        }
        
        // Store in new system
        err := newStore.Set(context.Background(), record.Key, record)
        if err != nil {
            log.Printf("Failed to migrate key %s: %v", record.Key, err)
        }
    }
    
    return nil
}
```

### Phase 3: Cutover Checklist

- [ ] All services updated to use new middleware
- [ ] Historical records migrated
- [ ] Monitoring shows equal hit rates
- [ ] No discrepancies in dual-run mode
- [ ] Client libraries updated (if needed)
- [ ] Old system deprecated

## Header Migration

If changing header names:

```go
// Temporary: Accept both headers
func HeaderMigrationMiddleware(next lift.Handler) lift.Handler {
    return lift.HandlerFunc(func(ctx *lift.Context) error {
        // Check new header first
        if ctx.Header("Idempotency-Key") == "" {
            // Fall back to old header
            if oldKey := ctx.Header("X-Request-ID"); oldKey != "" {
                ctx.Request.Headers["Idempotency-Key"] = oldKey
                log.Printf("Migrated header from X-Request-ID to Idempotency-Key")
            }
        }
        return next.Handle(ctx)
    })
}

app.Use(HeaderMigrationMiddleware)
app.Use(lift.Middleware(middleware.Idempotency(...)))
```

## Monitoring Migration

### Metrics to Track

```go
var (
    oldSystemHits = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "idempotency_old_system_hits",
    })
    newSystemHits = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "idempotency_new_system_hits",
    })
    migrationErrors = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "idempotency_migration_errors",
    })
)
```

### Validation Tests

```go
func TestMigrationCompatibility(t *testing.T) {
    // Test that both systems return same results
    testCases := []struct {
        name string
        key  string
        body string
    }{
        {"payment", "pay-123", `{"amount":100}`},
        {"order", "order-456", `{"items":["A","B"]}`},
    }
    
    for _, tc := range testCases {
        // Process with old system
        oldResult := processWithOldSystem(tc.key, tc.body)
        
        // Process with new system  
        newResult := processWithNewSystem(tc.key, tc.body)
        
        // Verify same result
        assert.Equal(t, oldResult, newResult)
    }
}
```

## Rollback Plan

If issues arise, you can quickly rollback:

```go
// Feature flag for easy rollback
if featureFlags.UseNewIdempotency() {
    app.Use(lift.Middleware(middleware.Idempotency(...)))
} else {
    app.Use(legacyIdempotencyMiddleware)
}
```

## Common Migration Issues

### Issue 1: Different Key Formats

**Problem**: Old system uses composite keys like `user:123:action:create`

**Solution**: Create adapter in store implementation:
```go
func (s *AdapterStore) Get(ctx context.Context, key string) (*IdempotencyRecord, error) {
    // Convert new format to old format
    oldKey := convertKeyFormat(key)
    return s.oldStore.Get(ctx, oldKey)
}
```

### Issue 2: Different TTL Handling

**Problem**: Old system has different expiration logic

**Solution**: Implement custom TTL in store:
```go
type CustomTTLStore struct {
    baseStore IdempotencyStore
}

func (s *CustomTTLStore) Set(ctx context.Context, key string, record *IdempotencyRecord) error {
    // Apply custom TTL logic
    record.ExpiresAt = calculateCustomTTL(record)
    return s.baseStore.Set(ctx, key, record)
}
```

### Issue 3: Response Format Differences

**Problem**: Old system stores responses differently

**Solution**: Add response transformer:
```go
func TransformResponse(old OldResponse) interface{} {
    return map[string]interface{}{
        "id":     old.ID,
        "status": old.Status,
        // Map old fields to new format
    }
}
```

## Success Criteria

Your migration is complete when:

1. ✅ All handlers use new middleware (no idempotency logic in handlers)
2. ✅ Monitoring shows 100% of idempotent requests handled by new system
3. ✅ Old idempotency code removed
4. ✅ Tests updated to use new patterns
5. ✅ Documentation updated
6. ✅ Team trained on new system

## Benefits After Migration

- **Cleaner Code**: No idempotency logic in handlers
- **Consistent Behavior**: Same idempotency rules everywhere  
- **Better Monitoring**: Built-in metrics and logging
- **Easier Testing**: Simplified test setup
- **Proven Solution**: Battle-tested middleware

## Support

For migration assistance:
- Review the [main guide](IDEMPOTENCY_GUIDE.md)
- Check [examples](../../examples/response-interception/)
- File issues for migration challenges
- Reach out to the Lift team for complex migrations