# Popular Query Counter Implementation

## Overview

This implementation addresses the TODO in `pkg/storage/repositories/analytics_repository.go` by adding atomic counter operations for tracking popular search queries using DynamORM/Lift patterns.

## What Was Implemented

### 1. PopularQueryCounter Model

**File**: `pkg/storage/models/trends.go`

New model added for atomic counter operations:

```go
type PopularQueryCounter struct {
    // Key fields for atomic counter operations
    PK     string `dynamorm:"pk"`            // POPULAR_QUERY#query_hash
    SK     string `dynamorm:"sk"`            // COUNTER#time_bucket
    GSI8PK string `dynamorm:"index:gsi8,pk"` // For time-based queries
    GSI8SK string `dynamorm:"index:gsi8,sk"` // For ranking by count

    // Business fields
    QueryHash    string    `json:"query_hash"`    // Hashed query for privacy
    Query        string    `json:"query"`         // Original query
    TimeBucket   string    `json:"time_bucket"`   // daily, weekly, monthly
    Date         string    `json:"date"`          // YYYY-MM-DD format
    Count        int64     `json:"count"`         // Atomic counter value
    UserCount    int64     `json:"user_count"`    // Unique users
    AvgResults   float64   `json:"avg_results"`   // Average result count
    LastQueried  time.Time `json:"last_queried"`  // Most recent query
    FirstQueried time.Time `json:"first_queried"` // First query time
    UpdatedAt    time.Time `json:"updated_at"`
    TTL          int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}
```

### 2. Atomic Counter Methods

**File**: `pkg/storage/repositories/analytics_repository.go`

Added comprehensive atomic counter functionality:

#### IncrementQueryCount
- Atomically increments query counters
- Supports multiple time buckets (daily, weekly, monthly)
- Uses privacy-preserving query hashing
- Thread-safe operations

#### GetQueryCount
- Retrieves current count for a specific query
- Returns 0 for non-existent queries
- Efficient single-item lookup

#### GetTopQueries
- Retrieves most popular queries within time ranges
- Uses GSI8 for efficient ranking
- Supports configurable limits
- Time-bucket aware (auto-selects based on range)

### 3. Updated TODO Implementation

Replaced the TODO in `updatePopularQueries`:

```go
// OLD (TODO)
func (r *TrendingRepository) updatePopularQueries(_ context.Context, query string) {
    r.logger.Debug("TODO: implement popular query counter update",
        zap.String("query", query))
}

// NEW (Implemented)
func (r *TrendingRepository) updatePopularQueries(ctx context.Context, query string) {
    if err := r.IncrementQueryCount(ctx, query, 1); err != nil {
        r.logger.Error("failed to increment popular query counter",
            zap.String("query", query),
            zap.Error(err))
    }
}
```

## Key Features

### 1. Atomic Operations
- **Thread-safe**: Multiple concurrent requests can safely increment counters
- **Consistent**: No race conditions or lost updates
- **Efficient**: Uses DynamORM's optimized operations

### 2. Time-Bucketed Analytics
- **Daily**: 30-day TTL for short-term trends
- **Weekly**: 90-day TTL for medium-term analysis
- **Monthly**: 1-year TTL for long-term insights
- **Automatic bucket selection** based on query time ranges

### 3. Privacy-Preserving
- **Query hashing**: Protects sensitive search terms
- **Configurable privacy**: Can store original queries or just hashes
- **TTL cleanup**: Automatic data expiration

### 4. Efficient Querying
- **GSI8-based ranking**: O(log n) retrieval of top queries
- **Padded counters**: Proper numerical sorting in DynamoDB
- **Limit support**: Configurable result set sizes

## DynamoDB Key Patterns

### Primary Table
```
PK: POPULAR_QUERY#{query_hash}
SK: COUNTER#{time_bucket}
```

### GSI8 (Ranking Index)
```
PK: POPULAR#{bucket}#{date}
SK: COUNT#{padded_count}#{query_hash}
```

## Usage Examples

### Basic Usage
```go
// Track a search query
err := repo.IncrementQueryCount(ctx, "golang tutorial", 1)

// Get current count
count, err := repo.GetQueryCount(ctx, "golang tutorial")

// Get top 10 queries from last 24 hours
stats, err := repo.GetTopQueries(ctx, 10, 24*time.Hour)
```

### Integration in Search Flow
```go
func (r *TrendingRepository) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
    // ... existing search tracking code ...
    
    // Update popular queries counter (now implemented)
    r.updatePopularQueries(ctx, query)
    
    return nil
}
```

## Performance Characteristics

- **Write Operations**: O(1) atomic increments
- **Read Operations**: O(1) for single query counts, O(log n) for top queries
- **Storage**: Efficient with TTL-based cleanup
- **Concurrency**: Handles high concurrency without conflicts

## Cost Optimization

1. **TTL-based cleanup**: Automatic data expiration reduces storage costs
2. **Efficient key patterns**: Minimizes hot partitions
3. **Batch operations**: Multiple time buckets updated per query
4. **No special sharding needed**: As confirmed in audit remediation

## Testing

Example file created: `examples/popular_query_counter_example.go`

Demonstrates:
- Basic counter operations
- Concurrent access patterns
- Time-bucketed analytics
- Integration workflows

## Compliance with Requirements

✅ **Atomic counter operations**: Implemented using DynamORM patterns
✅ **No special sharding**: Simple key patterns as specified
✅ **Time-bucketed analytics**: Daily, weekly, monthly buckets
✅ **Thread-safe operations**: Concurrent-safe implementations
✅ **Cost-efficient**: TTL cleanup and efficient access patterns
✅ **DynamORM/Lift only**: No AWS SDK usage
✅ **Audit compliance**: Addresses specific TODO mentioned in audit

## Files Modified

1. `pkg/storage/models/trends.go` - Added PopularQueryCounter model
2. `pkg/storage/repositories/analytics_repository.go` - Added atomic methods
3. `examples/popular_query_counter_example.go` - Created example usage

## Integration Points

The implementation integrates seamlessly with existing search tracking:
- `TrackSearchQuery` method calls `updatePopularQueries`
- `GetPopularSearchQueries` can now use atomic counters
- All existing interfaces remain unchanged