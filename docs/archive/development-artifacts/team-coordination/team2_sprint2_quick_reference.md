# Team 2 Sprint 2 Quick Reference

## 🎯 Your Mission
Implement timeline queries - the core social experience of Lesser

## 📅 Day-by-Day Plan
| Day | Task | Why Start Here |
|-----|------|----------------|
| 1 | PUBLIC Timeline | No auth required, simplest |
| 2 | HOME Timeline | Uses real follower data |
| 3 | HASHTAG & LIST | Adds filtering capability |
| 4 | Search (Local) | Foundation for discovery |
| 5 | Testing & Metrics | Ensure quality |

## 🔧 Key Patterns to Follow

### 1. Always Use DataLoader
```go
// DON'T: Direct storage calls in loops
for _, obj := range objects {
    author, _ := r.Storage.GetActor(ctx, obj.AttributedTo) // N+1!
}

// DO: Batch with DataLoader
authorIDs := extractAuthorIDs(objects)
r.Loaders.ActorLoader.LoadMany(ctx, authorIDs)
```

### 2. Track Every Operation
```go
r.CostTracker.TrackDynamoRead(1)      // For queries
r.CostTracker.TrackDynamoWrite(1)     // For mutations
```

### 3. Handle Pagination
```go
objects, cursor, err := r.Storage.GetPublicTimeline(ctx, first, after)
// Always return cursor for next page
```

### 4. Error Handling Pattern
```go
if err != nil {
    r.Logger.Error("operation failed", 
        zap.Error(err),
        zap.String("operation", "getPublicTimeline"))
    return nil, fmt.Errorf("failed to get timeline: %w", err)
}
```

## 📊 Available Storage Methods

### Timeline Queries
- `GetPublicTimeline(ctx, limit, cursor)`
- `GetActorOutbox(ctx, actorID, limit, cursor)`
- `GetHashtagTimeline(ctx, hashtag, limit, cursor)`

### Social Graph (From Team 1!)
- `GetFollowing(ctx, username, limit, cursor)` - Who user follows
- `GetFollowers(ctx, username, limit, cursor)` - Who follows user
- `GetBlocks(ctx, username)` - Blocked users
- `GetMutes(ctx, username)` - Muted users

### Lists & Preferences
- `GetListMembers(ctx, listID)`
- `GetUserLists(ctx, username)`
- `GetActorPreferences(ctx, username)`

## 🏗️ Building Connections

```go
func buildTimelineConnection(objects []storage.Object, cursor string) *TimelineConnection {
    edges := make([]*TimelineEdge, len(objects))
    for i, obj := range objects {
        edges[i] = &TimelineEdge{
            Node:   convertObjectToStatus(obj),
            Cursor: encodeCursor(obj.ID),
        }
    }
    
    return &TimelineConnection{
        Edges: edges,
        PageInfo: &PageInfo{
            HasNextPage: cursor != "",
            EndCursor:   cursor,
        },
    }
}
```

## ✅ Success Checklist

- [ ] PUBLIC timeline returns real posts
- [ ] HOME timeline filters by following
- [ ] Cursor pagination works
- [ ] No N+1 queries (check logs!)
- [ ] Costs tracked on all operations
- [ ] Errors handled gracefully
- [ ] Integration tests written

## 🚨 Common Pitfalls to Avoid

1. **Forgetting DataLoader** - Always batch related queries
2. **Missing Cost Tracking** - Every storage call costs money
3. **Ignoring Cursors** - Pagination is required for large datasets
4. **Direct Storage in Loops** - This creates N+1 queries

## 📚 Resources

- Storage Interface: `pkg/storage/interface.go`
- DataLoader Setup: `graph/dataloaders.go`
- Team 1 Patterns: `export_generator_completion_summary.md`
- GraphQL Schema: `graph/schema.graphql`

---

**Remember**: You're building on a solid foundation. DataLoader is set up, Team 1 has provided all the data, and the patterns are established. Just follow them! 