# GraphQL Mock Data Removal - Implementation Plan

## Overview
The GraphQL resolvers are currently returning mock/test data instead of real data from the database. This is causing fake posts from mastodon.social and other instances to appear in timelines.

## Current Issues

### 1. Mock Data in Timelines
- External posts from mastodon.social appearing in LOCAL and FEDERATED timelines
- These are hardcoded sample data, not real federated content
- Found in GraphQL resolver files generating test data

### 2. Affected Files
Based on the search results, these files contain mock/sample data:
- `/graph/phase2_resolvers.go` - Contains federation cost analytics with mock domains
- `/graph/phase3_resolvers.go` - Likely contains more mock data
- `/graph/schema.resolvers.go` - Contains sample trust relationships with mastodon.social users
- `/graph/subscriptions.go` - May contain mock subscription data
- `/graph/helpers.go` - May contain helper functions generating test data

## Required Changes

### Phase 1: Audit Mock Data Generation
1. **Review each resolver file** to identify all locations where mock data is generated
2. **Document each mock data generator** with:
   - Function name
   - What type of data it generates
   - Which GraphQL queries/mutations use it
   - Dependencies on this data

### Phase 2: Replace with Real Database Queries

#### 2.1 Timeline Resolvers
- Replace mock timeline data with calls to `store.GetPublicTimeline()`
- Ensure proper pagination using cursors
- Remove any hardcoded posts from external domains

#### 2.2 Federation Cost Resolvers
- Replace sample domains array with real federation statistics
- Query actual cost data from DynamoDB
- May need new storage methods if not already implemented

#### 2.3 Trust Relationship Resolvers
- Remove hardcoded actor URLs (alice, bob, carol@mastodon.social, dave)
- Query real trust relationships from storage
- Implement proper trust edge retrieval

#### 2.4 Other Mock Data
- Audit remaining resolvers for any other mock data
- Replace with appropriate database queries

### Phase 3: Storage Layer Updates

#### 3.1 Missing Storage Methods
Identify and implement any missing storage methods needed by GraphQL:
- Federation cost tracking
- Trust relationship queries
- Analytics data retrieval

#### 3.2 GraphQL-Specific Queries
Some GraphQL queries may need optimized storage methods:
- Aggregated statistics
- Time-series data
- Complex filtering/sorting

### Phase 4: Testing & Validation

#### 4.1 Unit Tests
- Update GraphQL resolver tests to use real data fixtures
- Remove tests that rely on mock data behavior
- Add tests for edge cases (empty results, pagination)

#### 4.2 Integration Tests
- Verify all GraphQL queries return real data
- Test pagination and filtering
- Ensure no mock data leaks through

#### 4.3 Manual Testing
- Check all timelines show only real posts
- Verify federation statistics are accurate
- Confirm trust relationships reflect actual data

## Implementation Order

1. **Start with timeline resolvers** (highest user impact)
2. **Fix federation/trust data** (affects federation features)
3. **Update analytics/metrics** (lower priority)
4. **Clean up any remaining mock data**

## Potential Challenges

1. **Missing Storage Methods**: Some GraphQL queries may expect data that isn't currently stored
2. **Performance**: Mock data is fast; real queries need optimization
3. **Empty States**: Need graceful handling when no real data exists
4. **Backwards Compatibility**: Ensure changes don't break existing clients

## Code Locations to Review

```go
// Example of mock data generation found:
sampleActors := []string{
    "https://example.com/users/alice",
    "https://example.com/users/bob",
    "https://mastodon.social/users/carol",
    "https://fosstodon.org/users/dave",
}

// This needs to be replaced with:
actors, err := r.Storage.GetActors(ctx, limit, cursor)
```

## Success Criteria

1. No hardcoded external domain posts in timelines
2. All GraphQL queries return data from database
3. Empty states handled gracefully
4. No performance regression
5. All tests passing with real data

## Next Steps

1. Create detailed audit of all mock data locations
2. Prioritize which resolvers to fix first
3. Implement storage methods if missing
4. Update resolvers one by one
5. Test thoroughly at each step