# DynamORM Soft Delete Pattern

This package provides comprehensive soft delete functionality for DynamoDB models.

## Features

### Core Components

#### SoftDeletable Interface
Defines the contract for soft-deletable entities:

```go
type SoftDeletable interface {
    SoftDelete()
    Restore()
    IsDeleted() bool
    GetDeletedAt() *time.Time
    SetDeletedAt(*time.Time)
    GetDeletedBy() string
    SetDeletedBy(string)
}
```

#### SoftDeleteModel
Embeddable struct that provides default soft delete functionality:

```go
type MyModel struct {
    ID   string `dynamodbav:"pk"`
    Name string `dynamodbav:"name"`
    
    // Embed soft delete functionality
    patterns.SoftDeleteModel
}
```

#### SoftDeleteRepository
Repository with soft delete-aware operations:

```go
repo := patterns.NewSoftDeleteRepository(client, "my-table", logger)

// Soft delete an item
err := repo.SoftDelete(ctx, model, "user123")

// Restore a soft-deleted item
err := repo.Restore(ctx, model)

// Hard delete (permanent)
err := repo.HardDelete(ctx, keys)
```

### Query Modes

#### Default (Active Only)
By default, all queries exclude soft-deleted items:

```go
// Only returns active items
items, err := repo.Query(ctx, queryInput)
```

#### Include Deleted Items
Use `WithDeleted()` to include soft-deleted items:

```go
// Returns both active and soft-deleted items
withDeletedRepo := repo.WithDeleted()
items, err := withDeletedRepo.Query(ctx, queryInput)
```

#### Only Deleted Items
Use `QueryOnlyDeleted()` to return only soft-deleted items:

```go
// Returns only soft-deleted items
deletedItems, err := repo.QueryOnlyDeleted(ctx, queryInput)
```

## Usage Examples

### Basic Model

```go
type User struct {
    ID       string    `dynamodbav:"pk" json:"id"`
    Username string    `dynamodbav:"username" json:"username"`
    Email    string    `dynamodbav:"email" json:"email"`
    
    // Embed soft delete functionality
    patterns.SoftDeleteModel
    
    CreatedAt time.Time `dynamodbav:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamodbav:"updated_at" json:"updated_at"`
}

// Ensure User implements SoftDeletable
var _ patterns.SoftDeletable = (*User)(nil)
```

### Repository Operations

```go
// Initialize repository
repo := patterns.NewSoftDeleteRepository(dynamoClient, "users", logger)

// Create user
user := &User{
    ID:       "user123",
    Username: "john_doe",
    Email:    "john@example.com",
}

// Soft delete
err := repo.SoftDelete(ctx, user, "admin_user")
if err != nil {
    log.Fatal(err)
}

// Check if deleted
if user.IsDeleted() {
    fmt.Printf("User deleted at: %v by: %s\n", 
        user.GetDeletedAt(), user.GetDeletedBy())
}

// Restore
err = repo.Restore(ctx, user)
if err != nil {
    log.Fatal(err)
}
```

### Cleanup Operations

#### Manual Cleanup
Remove items that have been soft-deleted for more than 30 days:

```go
deleted, err := repo.CleanupOldDeletes(ctx, 30*24*time.Hour, 25)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Permanently deleted %d items\n", deleted)
```

#### Get Old Deleted Items
Query for items to be cleaned up:

```go
oldItems, err := repo.GetDeletedItemsOlderThan(ctx, 30*24*time.Hour)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Found %d items ready for cleanup\n", len(oldItems))
```

### Convenience Functions

```go
// Check if item is deleted
if patterns.IsItemDeleted(user) {
    fmt.Println("User is soft deleted")
}

// Get deletion info
deletedAt, deletedBy, isDeleted := patterns.GetItemDeletionInfo(user)
if isDeleted {
    fmt.Printf("Deleted at %v by %s\n", deletedAt, deletedBy)
}

// Soft delete with user tracking
err := patterns.SoftDeleteByUser(ctx, repo, user, "admin123")

// Restore item
err := patterns.RestoreItem(ctx, repo, user)
```

## Implementation Details

### DynamoDB Fields

The `SoftDeleteModel` adds these fields to your table:

- `deleted_at` (optional): Timestamp when item was soft deleted
- `deleted_by` (optional): User ID who performed the soft delete

### Query Filtering

The repository automatically adds DynamoDB filter expressions to exclude soft-deleted items:

```go
// Active items only (default)
FilterExpression: attribute_not_exists(deleted_at)

// Deleted items only
FilterExpression: attribute_exists(deleted_at)
```

### Batch Operations

Cleanup operations use DynamoDB batch operations with proper error handling:

- Respects 25-item batch limits
- Handles unprocessed items
- Provides progress feedback
- Logs cleanup statistics

## Testing

Comprehensive tests cover:
- Model lifecycle (soft delete, restore)
- Repository operations (query, scan, get)
- Filtering behavior
- Cleanup operations
- Error handling

Run tests:
```bash
go test ./pkg/storage/dynamorm/patterns/ -v
```

## Best Practices

### When to Use Soft Delete

✅ **Good candidates:**
- User accounts
- Important business records
- Data with audit requirements
- Items referenced by other entities

❌ **Avoid for:**
- Temporary/cache data
- Log entries
- High-volume transactional data

### Cleanup Strategy

1. **Regular cleanup**: Schedule cleanup jobs to run weekly/monthly
2. **Retention policy**: Define clear retention periods (30-90 days)
3. **Monitoring**: Track soft delete statistics
4. **Backup**: Consider backing up before permanent deletion

### Performance Considerations

- Soft deleted items still consume storage
- Queries include additional filter expressions
- Use GSIs for efficient soft-delete queries
- Monitor table scan costs with high soft-delete ratios

## Error Handling

The package provides comprehensive error handling:

```go
// Soft delete
if err := repo.SoftDelete(ctx, model, userID); err != nil {
    if errors.Is(err, patterns.ErrAlreadyDeleted) {
        // Handle already deleted
    }
    // Handle other errors
}

// Restore
if err := repo.Restore(ctx, model); err != nil {
    if errors.Is(err, patterns.ErrNotDeleted) {
        // Handle not deleted
    }
    // Handle other errors
}
```