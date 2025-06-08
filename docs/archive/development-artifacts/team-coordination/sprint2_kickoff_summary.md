# Sprint 2 Kickoff - Both Teams Ready! 🚀

## Team Status After Sprint 1

### 🔧 Team 1: Infrastructure
**Completed**: Media Processing ✅
**Next Up**: Export Generator (CRITICAL PATH)
**Sprint 2 Mission**: Unblock Team 2 by implementing social graph exports

### 🎯 Team 2: GraphQL
**Completed**: DataLoader + Core Queries ✅
**Next Up**: Timeline Queries
**Sprint 2 Mission**: Build timeline functionality using storage layer

## Critical Path Alert 🚨

```
Team 2 Timeline Queries
         ↓
    NEEDS DATA FROM
         ↓
Team 1 Export Generator ← NOT STARTED YET!
         ↓
    WORKAROUND
         ↓
Use Storage Layer Directly
```

## Sprint 2 Day-by-Day Plan

### Monday (Day 1)
- **Team 1**: Implement `getFollowers()` - this unblocks everything
- **Team 2**: Start PUBLIC timeline query - no auth needed

### Tuesday (Day 2)  
- **Team 1**: Complete social graph exports (4 functions)
- **Team 2**: Implement HOME timeline with pagination

### Wednesday (Day 3)
- **Team 1**: Start content exports (outbox, likes, bookmarks)
- **Team 2**: Add hashtag and list timelines

### Thursday (Day 4)
- **Team 1**: Complete remaining export functions
- **Team 2**: Implement search functionality

### Friday (Day 5)
- **Both Teams**: Integration testing
- **Team 1**: Job management APIs
- **Team 2**: Notifications and metrics

## Key Code Patterns

### Team 1 - Export Fix Pattern
```go
// Replace this:
return []mastodon.Account{}, nil

// With this:
followers, _, err := storageClient.GetFollowers(ctx, userID, 1000, "")
// ... convert and return
```

### Team 2 - Timeline Pattern
```go
func (r *queryResolver) Timeline(ctx context.Context, args TimelineArgs) (*TimelineConnection, error) {
    // 1. Track cost
    r.CostTracker.TrackDynamoRead(1)
    
    // 2. Get objects based on timeline type
    objects, cursor, err := r.Storage.GetPublicTimeline(ctx, args.First, args.After)
    
    // 3. Use DataLoader for authors
    for _, obj := range objects {
        r.Loaders.LoadActor(ctx, obj.AttributedTo)
    }
    
    // 4. Return connection
    return buildTimelineConnection(objects, cursor), nil
}
```

## Success Metrics for Sprint 2

- [ ] Export Generator returns real data (Team 1)
- [ ] At least 2 timeline types working (Team 2)
- [ ] Zero N+1 queries in new code (Team 2)
- [ ] Integration tests passing (Both)
- [ ] Cost tracking on all operations (Both)

## Remember

1. **Team 1**: Every export function you fix unblocks Team 2
2. **Team 2**: Use storage layer directly, don't wait for exports
3. **Both**: Sync daily on data models and converters
4. **Priority**: Get basic functionality working before optimizing

---

**LET'S GO!** 🚀 Both teams have clear missions and the path forward is well-defined! 