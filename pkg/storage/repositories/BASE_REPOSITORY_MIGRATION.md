# BaseRepository Migration Guide

## Overview

The BaseRepository pattern provides a generic foundation for DynamoDB operations using DynamORM, eliminating boilerplate code across repositories.

## Benefits Demonstrated

### AnnouncementRepository Case Study
- **Original**: 494 lines
- **With BaseRepository**: 177 lines
- **Reduction**: 317 lines (64%)

### Code Savings Per Operation
- Create: ~20 lines saved
- Get: ~15 lines saved
- Update: ~15 lines saved
- Delete: ~15 lines saved
- Query: ~20 lines saved
- Count: ~15 lines saved

## Migration Pattern

### 1. Extend BaseRepository

```go
type YourRepository struct {
    *BaseRepository[*models.YourModel]
    // Additional fields if needed
}
```

### 2. Implement Model Interface

Your model must implement the BaseModel interface:

```go
type YourModel struct {
    PK string `dynamodbav:"PK"`
    SK string `dynamodbav:"SK"`
    // ... other fields
}

func (m *YourModel) UpdateKeys() error {
    // Update GSI keys if needed
    return nil
}

func (m *YourModel) GetPK() string {
    return m.PK
}

func (m *YourModel) GetSK() string {
    return m.SK
}
```

### 3. Use BaseRepository Methods

Replace boilerplate with BaseRepository calls:

```go
// Before: 20+ lines of create logic
// After:
return r.Create(ctx, model)

// Before: 15+ lines of get logic
// After:
return r.Get(ctx, pk, sk, result)

// Before: 15+ lines of delete logic
// After:
return r.Delete(ctx, pk, sk)
```

## Available Methods

- `Create(ctx, item)` - Create a new item
- `Get(ctx, pk, sk, result)` - Get single item
- `Update(ctx, item)` - Update existing item
- `Delete(ctx, pk, sk)` - Delete item
- `Query(ctx, pk, limit)` - Query by partition key
- `QueryWithSKPrefix(ctx, pk, skPrefix, limit)` - Query with SK prefix
- `QueryGSI(ctx, indexName, pk, limit)` - Query GSI
- `BatchGet(ctx, keys)` - Batch get multiple items
- `Count(ctx, pk)` - Count items
- `Exists(ctx, pk, sk)` - Check existence

## Migration Priority

Repositories that would benefit most from BaseRepository:

1. **High benefit** (>300 lines, simple CRUD):
   - MediaRepository
   - NotificationRepository
   - PollRepository
   - ScheduledStatusRepository

2. **Medium benefit** (200-300 lines, some custom logic):
   - ListRepository
   - DomainBlockRepository
   - MarkerRepository
   - EmojiRepository

3. **Low benefit** (<200 lines or complex custom logic):
   - ActorRepository (complex queries)
   - ObjectRepository (complex transformations)
   - TimelineRepository (complex pagination)

## Considerations

### When to Use BaseRepository
- Simple CRUD operations
- Standard DynamoDB patterns
- Minimal custom query logic
- Want to reduce boilerplate

### When NOT to Use BaseRepository
- Complex multi-table transactions
- Heavily customized query patterns
- Need fine-grained control over operations
- Legacy code with specific behaviors

## Future Enhancements

1. **Transaction Support**: Add batch transaction methods
2. **Pagination Helpers**: Built-in cursor pagination
3. **Conditional Operations**: Support for conditional writes
4. **Metrics Integration**: Built-in CloudWatch metrics
5. **Caching Layer**: Optional caching support

## Conclusion

BaseRepository provides significant code reduction (50-70%) for standard repository operations while maintaining flexibility for custom logic. New repositories should use BaseRepository by default, and existing repositories can be migrated based on the priority list above.