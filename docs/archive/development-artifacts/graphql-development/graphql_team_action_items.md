# GraphQL Team - Immediate Action Items

## 🚨 Critical Fix Required Before Proceeding

### Issue: DataLoader Not Being Used
You built a great DataLoader infrastructure but aren't using it! This will cause severe N+1 query performance problems.

### Fix Required (Day 1 Priority):

1. **Update Actor Query** (graph/schema.resolvers.go:239)
```go
// WRONG (current):
actor, err = r.Storage.GetActor(ctx, *username)

// RIGHT (use DataLoader):
actor, err = graph.LoadActor(ctx, *username)
```

2. **Update Object Query** (graph/schema.resolvers.go:288)
```go
// WRONG (current):
obj, err := r.Storage.GetObject(ctx, id)

// RIGHT (use DataLoader):
obj, err := graph.LoadObject(ctx, id)
```

3. **Add Test to Verify DataLoader Usage**
```go
func TestDataLoaderPreventsN1Queries(t *testing.T) {
    // Create test server with mock storage that counts calls
    mockStorage := &MockStorage{
        callCount: make(map[string]int),
    }
    
    // Execute GraphQL query that would cause N+1
    query := `{
        timeline(type: HOME) {
            edges {
                node {
                    id
                    author { username }  # This should batch load
                }
            }
        }
    }`
    
    // Verify storage was called once for objects, once for authors
    assert.Equal(t, 1, mockStorage.callCount["GetObjects"])
    assert.Equal(t, 1, mockStorage.callCount["GetActors"]) // Batched!
}
```

## 📋 Week 2 Remaining Tasks

### 1. Complete Object Type Support
Add missing object types to the switch statement:
```go
case *activitypub.Image:
    result = &model.Object{
        ID:          o.ID,
        Type:        model.ObjectTypeImage,
        MediaType:   o.MediaType,
        URL:         o.URL,
        // ... other image fields
    }
case *activitypub.Video:
    // Similar pattern
case *activitypub.Audio:
    // Similar pattern
```

### 2. Fix Timestamp Handling
```go
// Use actual timestamps from objects
if o.Published != nil {
    result.CreatedAt = model.Time(*o.Published)
}
if o.Updated != nil {
    result.UpdatedAt = model.Time(*o.Updated)
}
```

### 3. Implement Me Query
```go
func (r *queryResolver) Me(ctx context.Context) (*activitypub.Actor, error) {
    // Get authenticated user from context
    username := r.GetUsernameFromContext(ctx)
    if username == "" {
        return nil, fmt.Errorf("not authenticated")
    }
    
    // Use DataLoader!
    return graph.LoadActor(ctx, username)
}
```

### 4. Add More DataLoaders
Consider adding loaders for:
- FollowerCount
- FollowingCount  
- ObjectCounts
- ActivityBatch

## 🧪 Testing Requirements

Before moving to Week 3:
1. Unit test each resolver
2. Integration test with real DynamoDB Local
3. Verify DataLoader batching with logs
4. Load test to ensure no N+1 queries

## 📊 Success Metrics

Track these to ensure DataLoader is working:
- DynamoDB read count per GraphQL query
- Response time for complex queries
- Memory usage (DataLoader caches per request)

## 🚀 Once Fixed, You Can Proceed to Week 3-4

Timeline & Search queries will heavily benefit from proper DataLoader usage:
- Timeline queries fetch many objects
- Each object has an author
- Without DataLoader: 1 + N queries
- With DataLoader: 2 queries total

## Remember

**DataLoader is not optional** - it's critical for GraphQL performance. Fix this before any other work! 