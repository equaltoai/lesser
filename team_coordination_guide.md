# Team Coordination Guide

## Overview
Two backend teams working in parallel to eliminate all stubs in Lesser. Team 1 focuses on infrastructure, Team 2 on GraphQL.

## Team Responsibilities

### Team 1: Core Infrastructure (16 stubs)
- Export Generator (12 functions)
- Import/Export Jobs (2 functions)  
- Media Processing (2 functions)
- Establishes storage patterns

### Team 2: GraphQL API (58+ stubs)
- Query resolvers (~20)
- Mutation resolvers (~15)
- Subscription resolvers (~5)
- Field resolvers (~20)

## Dependency Matrix

| Week | Team 1 Work | Team 2 Work | Dependencies |
|------|-------------|-------------|--------------|
| 1-2 | Export Generator, Job Management | DataLoader setup, Core queries (actor, object) | Team 2 can start independently |
| 3-4 | Media Processing | Timeline queries, Search | Team 2 needs export data for timelines |
| 5-6 | Supporting Team 2 | Mutations (create, like, follow) | Team 2 needs media processing |
| 7-8 | Testing & Polish | AI features, Trust graph | Teams integrate features |
| 9-10 | Final integration | Subscriptions, Performance | Joint testing & optimization |

## Critical Handoff Points

### Week 2 → Week 3
**Team 1 → Team 2**: Export generator patterns
```go
// Team 1 establishes this pattern:
followers, nextCursor, err := storageClient.GetFollowers(ctx, username, 100, cursor)

// Team 2 uses same pattern in GraphQL:
followers, nextCursor, err := r.Storage.GetFollowers(ctx, username, limit, cursor)
```

### Week 4 → Week 5  
**Team 1 → Team 2**: Media processing available
```go
// Team 1 implements:
func processVideo(ctx context.Context, data []byte) (ProcessingResult, error)

// Team 2 can now use in mutations:
result, err := r.MediaProcessor.ProcessVideo(ctx, upload.Data)
```

## Communication Protocol

### Daily Sync Points
1. **Storage Pattern Updates** - Team 1 documents any new patterns
2. **Type/Model Changes** - Coordinate on shared structures
3. **Blocker Identification** - Surface dependencies early

### Shared Code Locations
```
pkg/storage/         # Both teams use this
pkg/activitypub/     # Shared types
pkg/mastodon/        # Type conversions
internal/testutil/   # Shared test utilities
```

## Parallel Work Opportunities

### Week 1-2 (Maximum Parallelism)
- Team 1: All export functions
- Team 2: DataLoader, actor/object queries, error handling
- No dependencies

### Week 3-4 (Some Dependencies)
- Team 1: Media processing
- Team 2: Timeline queries (needs some export data)
- Coordinate on timeline data models

### Week 5+ (Integration Phase)
- Both teams focus on integration
- Joint testing sessions
- Performance optimization

## Success Metrics

### Team 1
- ✅ 16 stubs eliminated
- ✅ Storage patterns documented
- ✅ All functions have tests

### Team 2  
- ✅ 58+ GraphQL stubs eliminated
- ✅ < 50ms p95 latency
- ✅ Zero N+1 queries

### Joint Success
- ✅ Full integration tests pass
- ✅ No remaining stubs
- ✅ Production deployment ready

## Conflict Resolution

### Code Conflicts
- Team 1 owns: `cmd/export-generator/`, `cmd/media-processor/`
- Team 2 owns: `graph/`, `cmd/graphql/`
- Shared: `pkg/` - coordinate changes

### Pattern Conflicts
- Team 1 establishes patterns first
- Team 2 follows established patterns
- Discuss before diverging

## Tools & Resources

### Shared Testing Infrastructure
```bash
# Local DynamoDB for both teams
docker run -p 8000:8000 amazon/dynamodb-local

# Shared test data generator
go run scripts/generate_test_data.go
```

### Documentation
- Team 1: Update `comprehensive_stub_implementation_plan.md` with progress
- Team 2: Update `graphql_schema_resolution_plan.md` with progress
- Both: Update this coordination guide with learnings

## Emergency Procedures

If blocked:
1. Check if other team has workaround
2. Implement minimal stub to unblock
3. Document in shared blockers list
4. Revisit in next sync

Remember: Communication prevents duplication and ensures compatibility! 