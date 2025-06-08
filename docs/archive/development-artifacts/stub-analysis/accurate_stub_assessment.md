# Accurate Stub Assessment

## What's Actually Working

### ✅ Core Social Features (FULLY IMPLEMENTED)
- **Follow/Unfollow**: Complete implementation in `pkg/storage/dynamodb/relationships.go`
  - CreateFollow, AcceptFollow, RemoveFollow, GetFollowers, GetFollowing
  - Proper state management (pending/accepted/rejected)
  - Follower/following counts maintained
  
- **Likes**: Complete implementation in `pkg/storage/dynamodb/likes.go`
  - CreateLike, DeleteLike, GetObjectLikes, CountObjectLikes
  - Proper indexing for queries

- **Blocks/Mutes**: Likely implemented (based on storage interface)

- **Posts/Objects**: Full CRUD operations

- **API Endpoints**: The Mastodon API endpoints for these features work

## ❌ What's Actually Broken

### 1. Export Generator Can't Access Real Data
The export generator has its own stub functions that don't call the storage layer:
```go
// This is in export-generator/main.go
func getFollowers(ctx context.Context, username string) ([]string, error) {
    // For now, return empty list
    return []string{}, nil  // ← THIS IS THE PROBLEM
}
```

But the REAL implementation exists:
```go
// This is in pkg/storage/dynamodb/relationships.go
func (s *dynamoDBStorage) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
    // FULL IMPLEMENTATION with DynamoDB queries
}
```

**The Issue**: The export generator isn't using the storage layer!

### 2. Import/Export Job Listing
- `getUserImportJobs()` - returns empty array
- `getUserExportJobs()` - returns empty array

### 3. GraphQL API
- 58/60 resolvers panic
- Only Actor and InstanceMetrics work

### 4. Media Processing
- Video/audio return fake durations

## The Real Problem

It's not that social features aren't implemented - they are! The problem is:

1. **Export Generator is isolated** - It's not using the storage layer that has all the real implementations
2. **Missing wiring** - The export functions need to call the storage layer, not implement their own stubs

## What Needs to Be Done

### Priority 1: Wire Export Generator to Storage
Instead of:
```go
func getFollowers(ctx context.Context, username string) ([]string, error) {
    return []string{}, nil
}
```

Should be:
```go
func getFollowers(ctx context.Context, username string) ([]string, error) {
    // Get storage instance
    storage := getStorageInstance()
    
    // Use the REAL implementation
    followers, _, err := storage.GetFollowers(ctx, username, 1000, "")
    return followers, err
}
```

### Priority 2: Fix Import/Export Listing
Similar issue - need to implement DynamoDB queries for job listing

### Priority 3: GraphQL
This is legitimately not implemented and needs work

## Time Estimate (Revised)

Since the core functionality EXISTS:
- **Export Generator**: 2-3 days to wire all functions to storage
- **Import/Export Lists**: 1 day 
- **GraphQL**: Still 1-2 weeks (actual implementation needed)
- **Media**: 2-3 days for real processing

## Key Insight

Lesser has WORKING social features. The export system just isn't connected to them. This is a much smaller problem than reimplementing everything! 