# DynamORM Cost Tracking Implementation Guide

## DynamORM v1.0.29 Interface Signatures

### core.DB Interface
```go
type DB interface {
    Model(model any) Query
    Transaction(fn func(tx *Tx) error) error
    Migrate() error
    AutoMigrate(models ...any) error
    Close() error
    WithContext(ctx context.Context) DB
}
```

### core.Query Interface (Complete DynamORM v1.0.29)
```go
type Query interface {
    // Query construction methods (return Query for chaining)
    Where(field string, op string, value any) Query
    Index(indexName string) Query
    Filter(field string, op string, value any) Query
    OrFilter(field string, op string, value any) Query
    FilterGroup(func(Query)) Query
    OrFilterGroup(func(Query)) Query
    OrderBy(field string, order string) Query
    Limit(limit int) Query
    Offset(offset int) Query
    Select(fields ...string) Query
    ConsistentRead() Query
    WithRetry(maxRetries int, initialDelay time.Duration) Query
    
    // Pagination
    Cursor(cursor string) Query
    SetCursor(cursor string) error
    
    // Context
    WithContext(ctx context.Context) Query
    
    // Terminal methods (execute queries)
    First(dest any) error
    All(dest any) error
    AllPaginated(dest any) (*PaginatedResult, error)
    Count() (int64, error)
    Create() error
    CreateOrUpdate() error
    Update(fields ...string) error
    UpdateBuilder() UpdateBuilder
    Delete() error
    
    // Scan operations
    Scan(dest any) error
    ParallelScan(segment int32, totalSegments int32) Query
    ScanAllSegments(dest any, totalSegments int32) error
    
    // Batch operations
    BatchGet(keys []any, dest any) error
    BatchCreate(items any) error
    BatchDelete(keys []any) error
    BatchWrite(putItems []any, deleteKeys []any) error
    BatchUpdateWithOptions(items []any, fields []string, options ...any) error
}
```

## Missing Methods in Your Implementation

Your `CostTrackingQuery` is missing these methods:

1. **Pagination Methods**:
   - `Cursor(cursor string) Query`
   - `SetCursor(cursor string) error`

2. **Context Method**:
   - `WithContext(ctx context.Context) Query`

3. **Scan Operations**:
   - `Scan(dest any) error`
   - `ParallelScan(segment int32, totalSegments int32) Query`
   - `ScanAllSegments(dest any, totalSegments int32) error`

4. **Batch Operations** (including the one causing your error):
   - `BatchGet(keys []any, dest any) error`
   - `BatchCreate(items any) error` ← **This is the missing method causing your compilation error**
   - `BatchDelete(keys []any) error`
   - `BatchWrite(putItems []any, deleteKeys []any) error`
   - `BatchUpdateWithOptions(items []any, fields []string, options ...any) error`

## Recommended Implementation Pattern

### Complete Implementation Example for Missing Methods

```go
// BatchCreate implementation
func (ctq *CostTrackingQuery) BatchCreate(items any) error {
    // Count items for cost tracking
    itemCount := 1 // Default
    // Use reflection to count if items is a slice
    if reflect.TypeOf(items).Kind() == reflect.Slice {
        itemCount = reflect.ValueOf(items).Len()
    }
    
    err := ctq.query.BatchCreate(items)
    if err == nil && ctq.tracker != nil {
        ctq.tracker.TrackDynamoWrite(itemCount)
    }
    
    if ctq.logger != nil {
        ctq.logger.Debug("dynamodb_batch_create_tracked",
            zap.Int("write_units", itemCount),
            zap.Error(err),
        )
    }
    
    return err
}

// Cursor implementation
func (ctq *CostTrackingQuery) Cursor(cursor string) core.Query {
    ctq.query = ctq.query.Cursor(cursor)
    return ctq
}

// SetCursor implementation
func (ctq *CostTrackingQuery) SetCursor(cursor string) error {
    return ctq.query.SetCursor(cursor)
}

// WithContext implementation
func (ctq *CostTrackingQuery) WithContext(ctx context.Context) core.Query {
    ctq.query = ctq.query.WithContext(ctx)
    return ctq
}

// Scan implementation
func (ctq *CostTrackingQuery) Scan(dest any) error {
    // Scans are expensive - estimate high RCU usage
    err := ctq.query.Scan(dest)
    if err == nil && ctq.tracker != nil {
        ctq.tracker.TrackDynamoRead(100) // Conservative estimate for scan
    }
    return err
}

// ParallelScan implementation
func (ctq *CostTrackingQuery) ParallelScan(segment int32, totalSegments int32) core.Query {
    ctq.query = ctq.query.ParallelScan(segment, totalSegments)
    return ctq
}

// BatchGet implementation
func (ctq *CostTrackingQuery) BatchGet(keys []any, dest any) error {
    err := ctq.query.BatchGet(keys, dest)
    if err == nil && ctq.tracker != nil {
        ctq.tracker.TrackDynamoRead(len(keys))
    }
    return err
}

// BatchDelete implementation
func (ctq *CostTrackingQuery) BatchDelete(keys []any) error {
    err := ctq.query.BatchDelete(keys)
    if err == nil && ctq.tracker != nil {
        ctq.tracker.TrackDynamoWrite(len(keys))
    }
    return err
}

// BatchWrite implementation
func (ctq *CostTrackingQuery) BatchWrite(putItems []any, deleteKeys []any) error {
    err := ctq.query.BatchWrite(putItems, deleteKeys)
    if err == nil && ctq.tracker != nil {
        totalWrites := len(putItems) + len(deleteKeys)
        ctq.tracker.TrackDynamoWrite(totalWrites)
    }
    return err
}

// ScanAllSegments implementation
func (ctq *CostTrackingQuery) ScanAllSegments(dest any, totalSegments int32) error {
    err := ctq.query.ScanAllSegments(dest, totalSegments)
    if err == nil && ctq.tracker != nil {
        // Parallel scan across segments - estimate high usage
        ctq.tracker.TrackDynamoRead(int(totalSegments) * 100)
    }
    return err
}

// BatchUpdateWithOptions implementation
func (ctq *CostTrackingQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
    err := ctq.query.BatchUpdateWithOptions(items, fields, options...)
    if err == nil && ctq.tracker != nil {
        ctq.tracker.TrackDynamoWrite(len(items))
    }
    return err
}
```

### Option 2: Context-Based Manual Tracking

Instead of wrapping, use manual tracking in your repository methods:

```go
func (r *UserRepository) GetUser(ctx context.Context, username string) (*User, error) {
    tracker := cost.FromContext(ctx)
    
    var user User
    err := r.db.WithContext(ctx).Model(&user).
        Where("PK", "=", "user#"+username).
        First(&user)
    
    if err == nil && tracker != nil {
        tracker.TrackDynamoRead(1)
    }
    
    return &user, err
}
```

### Option 3: Hybrid Approach

Keep the existing AWS SDK-level `DynamoDBCostWrapper` for accurate consumed capacity tracking, and use DynamORM for the higher-level operations.

## Key Implementation Considerations

### No Built-in Hooks
DynamORM v1.0.29 does not provide middleware, hooks, or interceptor patterns. Wrapping is the only way to intercept operations.

### No Consumed Capacity Access
DynamORM abstracts away AWS SDK responses, so you cannot access the actual `ConsumedCapacity`. You must either:
1. Use estimates based on operation type
2. Track at the AWS SDK level separately
3. Fork DynamORM to expose this data

### Cost Estimation Guidelines
- **Read Operations**:
  - `First()`: 1 RCU (single item)
  - `All()`: Estimate 10 RCU or count items after retrieval
  - `Count()`: 1 RCU minimum, more for large result sets
  - `Scan()`: High RCU usage - scans entire table
  - `BatchGet()`: 1 RCU per item retrieved
  
- **Write Operations**:
  - `Create()`, `Update()`, `Delete()`: 1 WCU per item
  - `CreateOrUpdate()`: 1 WCU
  - `Transaction()`: 2 WCU per item in transaction
  - `BatchCreate()`: 1 WCU per item in batch
  - `BatchDelete()`: 1 WCU per item deleted
  - `BatchWrite()`: 1 WCU per operation (put or delete)

### Best Practice Recommendation

Given the limitations, I recommend:

1. **For new code**: Use Option 2 (manual tracking) as it's simpler and more explicit
2. **For accurate costs**: Keep the existing AWS SDK wrapper for operations that need precise consumed capacity
3. **For comprehensive tracking**: Implement the full wrapper (Option 1) but accept that costs will be estimates

## Integration Pattern

```go
// In your repository initialization
func NewUserRepository(db core.DB, logger *zap.Logger) *UserRepository {
    // Option 1: Wrap the DB
    costTrackingDB := cost.NewCostTrackingDB(db, cost.New(), logger)
    
    // Option 2: Use regular DB and track manually
    return &UserRepository{
        db:     db,  // or costTrackingDB
        logger: logger,
    }
}

// In your handlers
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    ctx := cost.WithTracker(r.Context(), cost.New())
    
    user, err := h.userRepo.GetUser(ctx, username)
    
    // Get cost summary
    tracker := cost.FromContext(ctx)
    if tracker != nil {
        costSummary := tracker.CalculateCost()
        h.logger.Info("operation_cost", 
            zap.Float64("total_cost", costSummary.TotalCost),
            zap.Int64("read_units", costSummary.DynamoDBReads),
        )
    }
}
```