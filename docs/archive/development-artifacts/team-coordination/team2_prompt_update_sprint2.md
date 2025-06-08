# Team 2 GraphQL Prompt Updated for Sprint 2

## Summary of Updates

Team 2's prompt has been updated to reflect their Sprint 1 accomplishments and guide them through Sprint 2's timeline implementation work.

### Key Changes Made

1. **Updated Context**
   - Acknowledged they've built the foundation
   - Emphasized Team 1 has unblocked them completely
   - Changed tone from "fixing stubs" to "implementing features"

2. **Added Sprint 2 Starting Point Section**
   - Clear summary of what they accomplished
   - Clear summary of what Team 1 delivered
   - Specific mission for Sprint 2

3. **Timeline Implementation Guide**
   - Concrete code examples for PUBLIC timeline
   - Code example for HOME timeline using real data
   - Key patterns from Team 1's work

4. **Day-by-Day Sprint Plan**
   - Day 1: PUBLIC timeline
   - Day 2: HOME timeline
   - Day 3: HASHTAG and LIST timelines
   - Day 4: Search implementation
   - Day 5: Testing and metrics

5. **Updated Success Criteria**
   - Sprint 2 specific goals (2+ timeline types)
   - Overall progress tracking (4/60 resolvers done)
   - Performance targets maintained

6. **Removed Duplicates**
   - Cleaned up duplicate Week 3-4 section
   - Streamlined the scope sections

## Key Guidance Provided

### Implementation Pattern
```go
// 1. Track costs
r.CostTracker.TrackDynamoRead(1)

// 2. Get data from storage
objects, cursor, err := r.Storage.GetPublicTimeline(ctx, first, after)

// 3. Use DataLoader for related data
r.Loaders.ActorLoader.LoadMany(ctx, authorIDs)

// 4. Build GraphQL response
return buildTimelineConnection(objects, cursor), nil
```

### Available Data
- Follower/following relationships
- User posts (outbox)
- Blocks/mutes for filtering
- List memberships
- User preferences

## Expected Outcomes

By end of Sprint 2, Team 2 should have:
- ✅ At least 2 timeline types working
- ✅ Proper pagination implemented
- ✅ Search functionality started
- ✅ No N+1 queries (DataLoader working)
- ✅ 10-15% of resolvers complete (up from 7%)

## Bottom Line

Team 2 now has:
1. Clear understanding of their progress
2. Specific implementation examples
3. Day-by-day plan
4. Access to all needed data
5. Patterns to follow from Team 1

They're ready to build the core social features that users interact with! 