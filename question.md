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

### core.Query Interface
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
}
```

## Current Implementation Issues

1. **Type Compatibility**: Your `CostTrackingDB.Model()` returns `*CostTrackingQuery` but must return `core.Query`
2. **Missing Methods**: The `CostTrackingQuery` is missing most Query interface methods
3. **Method Signatures**: The `Update()` method takes variadic `fields ...string` parameter

## Recommended Implementation Pattern

### Option 1: Complete Wrapper Implementation

```go
// Ensure CostTrackingQuery implements core.Query
type CostTrackingQuery struct {
    query   core.Query  // Delegate to actual query
    tracker *Tracker
    logger  *zap.Logger
}

// All chain methods must return core.Query (not *CostTrackingQuery)
func (c *CostTrackingQuery) Where(field string, op string, value any) core.Query {
    c.query = c.query.Where(field, op, value)
    return c  // This works because CostTrackingQuery implements core.Query
}

// Terminal methods track costs and execute
func (c *CostTrackingQuery) First(dest any) error {
    c.tracker.TrackDynamoRead(1)  // Estimate
    return c.query.First(dest)
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
  
- **Write Operations**:
  - `Create()`, `Update()`, `Delete()`: 1 WCU per item
  - `CreateOrUpdate()`: 1 WCU
  - `Transaction()`: 2 WCU per item in transaction

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