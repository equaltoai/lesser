# Complete Implementation Strategy for Lesser

## Mission
Demonstrate that social networks can be completely reimagined using serverless ActivityPub. Every feature must work to prove the architecture.

## Core Principle
**No stubs. No shortcuts. Complete implementation.**

## Implementation Order (Based on Dependencies)

### Phase 1: Data Foundation (Week 1)
**Must work first because everything else depends on it**

#### 1. Import/Export List Functions
```go
// cmd/api/handlers/imports.go - getUserImportJobs()
// cmd/api/handlers/exports.go - getUserExportJobs()
```
These gate all testing of the import/export system.

#### 2. Core Export Data Functions
Start with the social graph - it's foundational:
```go
// These 4 functions unlock social network testing
getFollowers()      // Who follows you
getFollowing()      // Who you follow  
getFollowingActors() // Full actor data
getFollowersActors() // Full actor data
```

### Phase 2: Content & Activity (Week 1-2)
**The heart of any social network**

#### 3. User Content Export
```go
getOutbox()     // All user posts - CRITICAL
getLikes()      // User engagement data
getBookmarks()  // Saved content
getBookmarksForExport()
```

#### 4. Safety & Privacy Features
```go
getBlocks()     // User safety
getMutes()      // User preferences
getListsWithMembers() // Organization
getListsForExport()
```

### Phase 3: Media & Rich Content (Week 2)
**Modern social networks need real media handling**

#### 5. Video Processing
- Actual duration extraction using FFmpeg
- Real thumbnail generation
- Multiple quality transcoding

#### 6. Audio Processing  
- Real duration and waveform
- Metadata extraction
- Cover art handling

### Phase 4: GraphQL API (Week 3)
**Alternative API for modern clients**

#### 7. Replace All Panics
First, make it not crash (1 day)

#### 8. Implement Core Queries
- Actor queries (profile data)
- Timeline queries (content streams)
- Object queries (individual posts)

#### 9. Implement Mutations
- Content creation
- Social actions (follow, like, share)
- Moderation actions

## Implementation Approach

### For Each Feature:

1. **Understand the Data Model**
   - What's the DynamoDB schema?
   - What are the access patterns?
   - What GSIs are needed?

2. **Look for Similar Implementations**
   ```bash
   # Find similar queries in the codebase
   grep -r "Query.*GSI1" pkg/storage/dynamodb/
   ```

3. **Write the Test First**
   ```go
   func TestFeature_ReturnsRealData(t *testing.T) {
       // Create known data
       // Call function
       // Verify ACTUAL data returned
   }
   ```

4. **Implement Incrementally**
   - Get basic query working
   - Add pagination
   - Handle edge cases
   - Add proper error handling

## Parallel Work Streams

While implementing, these can happen in parallel:

### Stream A: Import/Export Pipeline
- Person 1: Fix list functions
- Person 2: Implement social graph exports
- Person 3: Implement content exports

### Stream B: Media Processing
- FFmpeg integration
- Lambda configuration
- S3 permissions

### Stream C: GraphQL
- Schema first
- Resolvers incrementally
- Testing framework

## Success Metrics

### Week 1 Checkpoint
- [ ] Can list imports/exports
- [ ] Can export real followers/following
- [ ] Can export real posts
- [ ] Zero "for now" comments in these areas

### Week 2 Checkpoint  
- [ ] Complete export generates real data
- [ ] Media uploads show correct duration
- [ ] All export types functional
- [ ] GraphQL doesn't panic

### Week 3 Checkpoint
- [ ] Full import/export cycle works
- [ ] GraphQL API functional
- [ ] All stubs eliminated
- [ ] Can demo complete social network features

## No Compromise Rules

1. **If it's in the API, it must work** - No fake endpoints
2. **If it returns data, it must be real** - No hardcoded responses  
3. **If it's part of the demo, it must be complete** - No "imagine this works"
4. **If Mastodon has it, we must match it** - Full compatibility

## Resources Needed

### Technical
- DynamoDB GSI configurations
- FFmpeg Lambda layer
- S3 bucket with proper permissions
- Test data generators

### Knowledge
- ActivityPub spec for proper export format
- Mastodon API for compatibility
- DynamoDB query patterns
- Media processing pipelines

## Daily Checklist

- [ ] Implemented at least one complete feature
- [ ] Removed at least one stub
- [ ] Added tests that verify real functionality
- [ ] Can demo what was built

## The Vision

When complete, Lesser will prove that:
- Serverless can handle social network scale
- ActivityPub enables true data portability  
- Users can truly own their social graph
- The architecture is production-ready

**Every stub we leave is a doubt in that vision. No stubs.** 