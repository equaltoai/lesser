# Viewer Engagement Batch Pattern Implementation Plan

## Executive Summary

**Problem**: GraphQL API lacks viewer engagement fields (favourited, reblogged, bookmarked, pinned) for status objects. Current N+1 query pattern would require 80 individual DynamoDB queries for a 20-status timeline.

**Solution**: Implement efficient batch lookup pattern using DynamoDB BatchGetItem operations, reducing query count from O(N×M) to O(M) where N = statuses, M = engagement types.

**Impact**: 
- Latency reduction: ~95% (4 parallel operations vs 80 sequential queries)
- Cost efficiency: Same read units, dramatically reduced round-trips
- User experience: Instant feedback on which posts they've interacted with

---

## Current State Analysis

### Data Models

| Model | Purpose | PK Pattern | SK Pattern | GSI |
|-------|---------|------------|------------|-----|
| `Like` | ActivityPub Like | `object#{object_id}#likes` | `actor#{actor_id}` | GSI1: actor lookups |
| `Announce` | ActivityPub Announce/Reblog | `OBJECT#{object_id}#ANNOUNCES` | `ACTOR#{actor_id}` | GSI4: actor lookups |
| `Bookmark` | Local-only bookmark | `BOOKMARK#{username}` | `TIME#{timestamp}#{object_id}` _(listing)_ <br> `OBJECT#{object_id}` _(batch lookup)_ | None |
| `StatusPin` | Pinned status | `USER#{username}#PINS` | `STATUS#{status_id}` | None |

### Existing Query Methods

- `HasLiked(actor, object)` - Single Get operation
- `HasReblogged(actor, statusID)` - Single Get operation  
- `GetStatusEngagement(statusID, userID)` - Query with filters (7-day TTL data)
- `GetUserBookmarks(username)` - Query partition, no batch lookup support

### Current GraphQL Gap

File: `graph/core.graphql` (lines 100-135)
```graphql
type Object {
  likesCount: Int!
  sharesCount: Int!
  boosted: Boolean!  # Only this viewer field exists
  # MISSING: favourited, reblogged, bookmarked, pinned
}
```

File: `graph/schema.resolvers.go` (line 1121)
- `convertStatusToObject()` only populates `boosted` field
- No batch engagement lookup implementation

---

## Solution Design

### Core Pattern: Batch Engagement Lookup

```
Function: GetViewerEngagements(viewerID, statusIDs[])
Returns: Map<statusID, ViewerState{liked, reblogged, bookmarked, pinned}>

Parallel Execution:
├─ BatchGetItem: Likes (20 items, 1 round-trip)
├─ BatchGetItem: Announces (20 items, 1 round-trip)  
├─ BatchGetItem: Bookmarks (20 items, 1 round-trip) *requires Phase 1
└─ BatchGetItem: Pins (20 items, 1 round-trip)

Total: 4 parallel DynamoDB operations
Cost: ~10-20 RCUs (eventually consistent)
Latency: ~50-100ms (single parallel batch)
```

### Key Innovation: Dual-Write Bookmark Pattern

**Problem**: Current Bookmark SK includes timestamp (`{timestamp}#{object_id}`), preventing BatchGetItem lookups.

**Solution**: Write two records per bookmark to support both access patterns:

```
Record 1 (Time-ordered listing):
  PK: BOOKMARK#{username}
  SK: TIME#{timestamp}#{object_id}
  Purpose: Query user's bookmarks chronologically
  
Record 2 (Object-indexed lookup):
  PK: BOOKMARK#{username}  
  SK: OBJECT#{object_id}
  Purpose: BatchGetItem to check specific statuses
```

**Trade-offs**:
- ✅ Enables O(1) batch lookups instead of O(N) scan+filter
- ✅ Preserves chronological listing capability
- ❌ 2× write cost (acceptable for infrequent bookmark operations)
- ❌ 2× storage (negligible: ~1-2KB per bookmark)

---

## Implementation Phases

## Phase 1: Bookmark Model Dual-Write Pattern

**Goal**: Modify Bookmark model and repository to support both time-ordered and object-indexed access patterns.

### Tasks

#### 1.1: Update Bookmark Model
**File**: `pkg/storage/models/bookmark.go`

**Changes**:
- Add `RecordType` field to distinguish TIME vs OBJECT records
- Modify `UpdateKeys()` to support dual SK patterns
- Add factory methods: `NewTimeOrderedBookmark()`, `NewObjectIndexedBookmark()`
- Update documentation with dual-write pattern explanation

**Acceptance Criteria**:
- [ ] Model can generate both TIME# and OBJECT# SK patterns
- [ ] Backward compatible with existing bookmarks
- [ ] Unit tests for key generation logic

#### 1.2: Update BookmarkRepository Create Method
**File**: `pkg/storage/repositories/bookmark_repository.go`

**Changes**:
- Modify `CreateBookmark()` to write both records transactionally
- Use `TransactWriteItems` or fallback to two conditional writes
- Handle duplicate bookmark detection for both records
- Update error handling for transactional failures

**Acceptance Criteria**:
- [ ] Single `CreateBookmark()` call writes 2 DynamoDB items
- [ ] Transaction succeeds or rolls back atomically
- [ ] Duplicate bookmarks detected and handled gracefully
- [ ] Unit tests with mocked DynamoDB

#### 1.3: Update BookmarkRepository Delete Method
**File**: `pkg/storage/repositories/bookmark_repository.go`

**Changes**:
- Modify `DeleteBookmark()` to remove both records
- Query TIME# record to get timestamp, construct both keys
- Use `TransactWriteItems` for atomic deletion
- Handle case where only one record exists (migration period)

**Acceptance Criteria**:
- [ ] Deletes both TIME# and OBJECT# records
- [ ] Gracefully handles partial deletion (migration state)
- [ ] Unit tests for both deletion paths

#### 1.4: Add Batch Bookmark Lookup Method
**File**: `pkg/storage/repositories/bookmark_repository.go`

**Changes**:
- Add `CheckBookmarksForStatuses(ctx, username, statusIDs[]) map[string]bool`
- Use `BatchGetItem` with OBJECT# keys
- Return map of statusID → true for bookmarked items
- Handle pagination (max 100 items per BatchGetItem)

**Acceptance Criteria**:
- [ ] Method returns correct bookmark state for N status IDs
- [ ] Handles empty input gracefully
- [ ] Handles >100 statusIDs with multiple batches
- [ ] Unit tests with various input sizes

#### 1.5: Backward Compatibility
**File**: `pkg/storage/repositories/bookmark_repository.go`

**Changes**:
- Update `GetUserBookmarks()` to filter for TIME# prefix
- Add fallback logic: if OBJECT# record not found, query TIME# records
- Document migration path for existing bookmarks

**Acceptance Criteria**:
- [ ] Existing bookmark queries work unchanged
- [ ] Batch lookup degrades gracefully for unmigrated bookmarks
- [ ] Migration script documented (not executed in this phase)

**Phase 1 Deliverables**:
- [x] Updated Bookmark model with dual-write support
- [x] Repository methods for create/delete/batch-lookup
- [x] Unit tests with >90% coverage
- [x] Documentation of dual-write pattern and locking protocol
- [x] Notes service, service registry, and API handlers now depend on `BookmarkRepository` directly (no more Account/User shortcuts)
- [x] No breaking changes to existing API
- [x] Bookmark repository now relies on DynamORM's conditional expressions, transaction builder, and retry-aware BatchGet helpers (mirroring the patterns in `cmd/dynamorm-service`).

**Phase 1 Estimated Effort**: 4-6 hours

---

## Phase 2: Batch Engagement Repository Layer

**Goal**: Create unified repository interface for batch engagement lookups across all interaction types.

### Tasks

#### 2.1: Create EngagementRepository
**File**: `pkg/storage/repositories/engagement_repository.go` (new)

**Structure**:
```go
type ViewerEngagementState struct {
    StatusID   string
    Liked      bool
    Reblogged  bool
    Bookmarked bool
    Pinned     bool
}

type EngagementRepository struct {
    db               core.DB
    tableName        string
    likeRepo         *LikeRepository
    bookmarkRepo     *BookmarkRepository
    logger           *zap.Logger
}

// Core method
func (r *EngagementRepository) GetViewerEngagementsForStatuses(
    ctx context.Context,
    viewerID string,
    statusIDs []string,
) (map[string]*ViewerEngagementState, error)
```

**Acceptance Criteria**:
- [ ] Repository instantiated with dependencies
- [ ] Interface defined for batch engagement queries
- [ ] Error handling for partial failures
- [ ] Logging for observability

#### 2.2: Implement Batch Like Lookup
**File**: `pkg/storage/repositories/engagement_repository.go`

**Implementation**:
```go
func (r *EngagementRepository) batchCheckLikes(
    ctx context.Context,
    viewerID string,
    statusIDs []string,
) (map[string]bool, error) {
    // Build BatchGetItem keys
    keys := make([]struct{PK, SK string}, len(statusIDs))
    for i, statusID := range statusIDs {
        keys[i] = struct{PK, SK string}{
            PK: fmt.Sprintf("object#%s#likes", statusID),
            SK: fmt.Sprintf("actor#%s", viewerID),
        }
    }
    
    // Execute BatchGetItem
    // Parse results into map
    // Return statusID → liked bool
}
```

**Acceptance Criteria**:
- [ ] Uses DynamoDB BatchGetItem API
- [ ] Handles 100-item limit with pagination
- [ ] Returns accurate liked state for each status
- [ ] Unit tests with mock BatchGetItem responses

#### 2.3: Implement Batch Announce Lookup
**File**: `pkg/storage/repositories/engagement_repository.go`

**Implementation**:
```go
func (r *EngagementRepository) batchCheckAnnounces(
    ctx context.Context,
    viewerID string,
    statusIDs []string,
) (map[string]bool, error) {
    // Similar to likes but with OBJECT#/ACTOR# pattern
    keys := make([]struct{PK, SK string}, len(statusIDs))
    for i, statusID := range statusIDs {
        keys[i] = struct{PK, SK string}{
            PK: fmt.Sprintf("OBJECT#%s#ANNOUNCES", statusID),
            SK: fmt.Sprintf("ACTOR#%s", viewerID),
        }
    }
    
    // Execute BatchGetItem
    // Return statusID → reblogged bool
}
```

**Acceptance Criteria**:
- [ ] Uses correct OBJECT#/ACTOR# key pattern
- [ ] Handles batch pagination
- [ ] Returns accurate reblog state
- [ ] Unit tests

#### 2.4: Implement Batch Pin Lookup
**File**: `pkg/storage/repositories/engagement_repository.go`

**Implementation**:
```go
func (r *EngagementRepository) batchCheckPins(
    ctx context.Context,
    viewerID string,
    statusIDs []string,
) (map[string]bool, error) {
    // StatusPin has deterministic SK: STATUS#{status_id}
    keys := make([]struct{PK, SK string}, len(statusIDs))
    for i, statusID := range statusIDs {
        keys[i] = struct{PK, SK string}{
            PK: fmt.Sprintf("USER#%s#PINS", viewerID),
            SK: fmt.Sprintf("STATUS#%s", statusID),
        }
    }
    
    // Execute BatchGetItem
    // Return statusID → pinned bool
}
```

**Acceptance Criteria**:
- [ ] Uses correct USER#PINS pattern
- [ ] BatchGetItem implementation
- [ ] Returns accurate pin state
- [ ] Unit tests

#### 2.5: Orchestrate Parallel Execution
**File**: `pkg/storage/repositories/engagement_repository.go`

**Implementation**:
```go
func (r *EngagementRepository) GetViewerEngagementsForStatuses(
    ctx context.Context,
    viewerID string,
    statusIDs []string,
) (map[string]*ViewerEngagementState, error) {
    // Execute all 4 batch operations in parallel
    var wg sync.WaitGroup
    var likes, reblogs, bookmarks, pins map[string]bool
    var errs []error
    
    wg.Add(4)
    
    go func() { 
        defer wg.Done()
        likes, err = r.batchCheckLikes(ctx, viewerID, statusIDs)
        if err != nil { errs = append(errs, err) }
    }()
    
    go func() { 
        defer wg.Done()
        reblogs, err = r.batchCheckAnnounces(ctx, viewerID, statusIDs)
        if err != nil { errs = append(errs, err) }
    }()
    
    go func() { 
        defer wg.Done()
        bookmarks, err = r.bookmarkRepo.CheckBookmarksForStatuses(ctx, viewerID, statusIDs)
        if err != nil { errs = append(errs, err) }
    }()
    
    go func() { 
        defer wg.Done()
        pins, err = r.batchCheckPins(ctx, viewerID, statusIDs)
        if err != nil { errs = append(errs, err) }
    }()
    
    wg.Wait()
    
    // Merge results into ViewerEngagementState map
    // Handle partial failures gracefully
}
```

**Acceptance Criteria**:
- [ ] All 4 operations execute in parallel
- [ ] Partial failures don't break entire operation
- [ ] Results merged correctly into unified map
- [ ] Performance tests show <100ms p99 latency for 20 statuses
- [ ] Integration tests with real DynamoDB

**Phase 2 Deliverables**:
- [ ] New EngagementRepository with batch methods
- [ ] Parallel execution of 4 batch operations
- [ ] Unit tests for each batch method
- [ ] Integration tests with DynamoDB Local
- [ ] Performance benchmarks

**Phase 2 Estimated Effort**: 6-8 hours

---

## Phase 3: GraphQL Schema & Resolver Integration

**Goal**: Expose viewer engagement fields in GraphQL API and integrate batch lookup in resolvers.

### Tasks

#### 3.1: Update GraphQL Schema
**File**: `graph/core.graphql`

**Changes**:
```graphql
type Object {
  id: ID!
  type: String!
  # ... existing fields ...
  
  # Engagement counts (existing)
  likesCount: Int!
  sharesCount: Int!
  repliesCount: Int!
  
  # Viewer engagement state (NEW)
  favourited: Boolean!   # Has viewer liked this status
  reblogged: Boolean!    # Has viewer reblogged/announced this status
  bookmarked: Boolean!   # Has viewer bookmarked this status
  pinned: Boolean!       # Has viewer pinned this status (on their profile)
  
  # Existing viewer state
  boosted: Boolean!      # Legacy field, maps to reblogged
}
```

**Acceptance Criteria**:
- [ ] Schema compiles with gqlgen
- [ ] Fields documented with comments
- [ ] Backward compatible (no breaking changes)

#### 3.2: Update Generated GraphQL Types
**Command**: `make generate` or `go run github.com/99designs/gqlgen generate`

**Verification**:
- [ ] `graph/generated.go` includes new fields
- [ ] `graph/model/models_gen.go` updated
- [ ] No compilation errors

#### 3.3: Add DataLoader for Engagement State
**File**: `graph/dataloader.go` (new or extend existing)

**Implementation**:
```go
type EngagementLoader struct {
    repo      *repositories.EngagementRepository
    batchSize int
    wait      time.Duration
}

func (l *EngagementLoader) Load(
    ctx context.Context,
    viewerID string,
    statusID string,
) (*repositories.ViewerEngagementState, error) {
    // DataLoader pattern: batches individual loads
    // Returns cached result if already loaded
}

func (l *EngagementLoader) LoadMany(
    ctx context.Context,
    viewerID string,
    statusIDs []string,
) ([]*repositories.ViewerEngagementState, error) {
    // Batch load multiple statuses at once
    return l.repo.GetViewerEngagementsForStatuses(ctx, viewerID, statusIDs)
}
```

**Acceptance Criteria**:
- [ ] DataLoader batches requests within 10ms window
- [ ] Cache prevents duplicate queries within same request
- [ ] Handles errors gracefully
- [ ] Unit tests for batching logic

#### 3.4: Update Object Resolver
**File**: `graph/schema.resolvers.go`

**Modify**: `convertStatusToObject()` method

**Changes**:
```go
func convertStatusToObject(
    ctx context.Context,
    status *models.Status,
    engagementState *repositories.ViewerEngagementState, // NEW parameter
) *model.Object {
    obj := &model.Object{
        ID:           status.StatusID,
        Type:         "Note",
        LikesCount:   status.LikeCount,
        SharesCount:  status.ReblogCount,
        RepliesCount: status.ReplyCount,
        
        // NEW: Set viewer engagement fields
        Favourited:   engagementState != nil && engagementState.Liked,
        Reblogged:    engagementState != nil && engagementState.Reblogged,
        Bookmarked:   engagementState != nil && engagementState.Bookmarked,
        Pinned:       engagementState != nil && engagementState.Pinned,
        
        // Existing field (backward compat)
        Boosted:      engagementState != nil && engagementState.Reblogged,
    }
    
    // ... rest of conversion logic
    return obj
}
```

**Acceptance Criteria**:
- [ ] All new fields populated correctly
- [ ] Handles nil viewer (unauthenticated requests)
- [ ] Backward compatible with existing Boosted field
- [ ] Unit tests for conversion logic

#### 3.5: Update Timeline Resolvers
**Files**: 
- `graph/schema.resolvers.go` (timeline queries)
- Any resolver that returns `[Object]`

**Pattern**:
```go
func (r *queryResolver) PublicTimeline(
    ctx context.Context,
    limit int,
) ([]*model.Object, error) {
    // 1. Fetch statuses from repository
    statuses, err := r.statusRepo.GetPublicTimeline(ctx, limit)
    if err != nil {
        return nil, err
    }
    
    // 2. Extract viewer from context
    viewer := auth.GetViewerFromContext(ctx)
    if viewer == nil {
        // Unauthenticated: return without engagement state
        return convertStatusesToObjects(ctx, statuses, nil), nil
    }
    
    // 3. Batch load engagement state
    statusIDs := extractStatusIDs(statuses)
    engagementMap, err := r.engagementLoader.LoadMany(ctx, viewer.ID, statusIDs)
    if err != nil {
        // Log error but don't fail entire query
        log.Error("failed to load engagement state", zap.Error(err))
        engagementMap = make(map[string]*ViewerEngagementState)
    }
    
    // 4. Convert with engagement state
    objects := make([]*model.Object, len(statuses))
    for i, status := range statuses {
        engagement := engagementMap[status.StatusID]
        objects[i] = convertStatusToObject(ctx, status, engagement)
    }
    
    return objects, nil
}
```

**Resolvers to Update**:
- [ ] `PublicTimeline`
- [ ] `HomeTimeline`
- [ ] `UserTimeline`
- [ ] `HashtagTimeline`
- [ ] `ConversationThread`
- [ ] `SearchStatuses`
- [ ] `StatusDetail` (single status query)

**Acceptance Criteria**:
- [ ] All timeline queries include engagement state
- [ ] Unauthenticated requests work (all fields false)
- [ ] Batch loading happens once per query
- [ ] Integration tests for each resolver

#### 3.6: Wire Up Dependencies
**Files**:
- `cmd/graphql/main.go`
- `cmd/api/main.go`

**Changes**:
```go
// Initialize repositories
engagementRepo := repositories.NewEngagementRepository(
    db,
    tableName,
    likeRepo,
    bookmarkRepo,
    logger,
)

// Create DataLoader
engagementLoader := graph.NewEngagementLoader(engagementRepo)

// Pass to resolver
resolver := graph.NewResolver(
    statusRepo,
    engagementRepo,
    engagementLoader,
    // ... other dependencies
)
```

**Acceptance Criteria**:
- [ ] EngagementRepository instantiated at startup
- [ ] DataLoader injected into resolver
- [ ] No dependency injection errors
- [ ] Smoke tests pass

**Phase 3 Deliverables**:
- [ ] GraphQL schema with 4 new viewer engagement fields
- [ ] DataLoader for efficient batching
- [ ] All timeline resolvers updated
- [ ] Integration tests with GraphQL queries
- [ ] API documentation updated

**Phase 3 Estimated Effort**: 6-8 hours

---

## Phase 4: Testing & Validation

**Goal**: Comprehensive testing to ensure correctness, performance, and reliability.

### Tasks

#### 4.1: Unit Tests
**Coverage Target**: >90% for new code

**Test Files**:
- [ ] `pkg/storage/models/bookmark_test.go` - Dual-write key generation
- [ ] `pkg/storage/repositories/bookmark_repository_test.go` - CRUD operations
- [ ] `pkg/storage/repositories/engagement_repository_test.go` - Batch operations
- [ ] `graph/schema_resolvers_test.go` - Resolver logic with engagement state

**Test Scenarios**:
- [ ] Empty status list
- [ ] Single status
- [ ] 20 statuses (typical timeline)
- [ ] 100 statuses (pagination boundary)
- [ ] Unauthenticated viewer (nil context)
- [ ] Viewer with no interactions (all false)
- [ ] Viewer with partial interactions
- [ ] Viewer with all interactions
- [ ] DynamoDB errors (timeout, throttling)
- [ ] Partial failure (1 of 4 batch operations fails)

#### 4.2: Integration Tests
**File**: `tests/integration/engagement_test.go` (new)

**Setup**:
- [ ] DynamoDB Local container
- [ ] Seed test data (users, statuses, interactions)
- [ ] GraphQL test server

**Test Cases**:
```go
func TestBatchEngagementLookup(t *testing.T) {
    // Given: User has liked status1, reblogged status2, bookmarked status3
    // When: Query timeline with [status1, status2, status3, status4]
    // Then: Engagement state matches expected
}

func TestGraphQLTimelineWithEngagement(t *testing.T) {
    // Given: Authenticated viewer
    // When: Query { publicTimeline { id favourited reblogged bookmarked pinned } }
    // Then: Response includes correct engagement state
}

func TestUnauthenticatedTimeline(t *testing.T) {
    // Given: No authentication
    // When: Query timeline
    // Then: All engagement fields are false, no errors
}

func TestDualWriteBookmark(t *testing.T) {
    // Given: User creates bookmark
    // When: Check both TIME# and OBJECT# records
    // Then: Both exist with same data
}

func TestBookmarkDelete(t *testing.T) {
    // Given: Bookmark with both records exists
    // When: Delete bookmark
    // Then: Both records removed atomically
}
```

**Acceptance Criteria**:
- [ ] All integration tests pass
- [ ] Tests run in CI/CD pipeline
- [ ] Average test execution time <5 seconds

#### 4.3: Performance Tests
**File**: `tests/performance/engagement_bench_test.go` (new)

**Benchmarks**:
```go
func BenchmarkBatchEngagementLookup20Statuses(b *testing.B) {
    // Measure: Time to load engagement for 20 statuses
    // Target: <100ms p99
}

func BenchmarkBatchEngagementLookup100Statuses(b *testing.B) {
    // Measure: Time to load engagement for 100 statuses
    // Target: <200ms p99
}

func BenchmarkGraphQLTimelineQuery(b *testing.B) {
    // Measure: Full GraphQL timeline query latency
    // Target: <300ms p99
}

func BenchmarkDualWriteBookmark(b *testing.B) {
    // Measure: Bookmark creation time
    // Compare: Old single-write vs new dual-write
    // Acceptable: <2× overhead
}
```

**Metrics to Collect**:
- [ ] Latency (p50, p95, p99)
- [ ] DynamoDB RCU consumption
- [ ] DynamoDB WCU consumption (bookmark writes)
- [ ] Memory allocation
- [ ] Goroutine count

**Acceptance Criteria**:
- [ ] 20-status timeline engagement lookup: <100ms p99
- [ ] 100-status engagement lookup: <200ms p99
- [ ] Dual-write bookmark overhead: <2× single write
- [ ] No memory leaks under load

#### 4.4: Manual Testing with Greater Frontend
**Prerequisites**:
- [ ] Lesser API deployed to dev environment
- [ ] Greater frontend pointed at dev Lesser API

**Test Scenarios**:
1. **Timeline viewing**:
   - [ ] Open home timeline
   - [ ] Verify heart icon filled for liked posts
   - [ ] Verify reblog icon filled for reblogged posts
   - [ ] Verify bookmark icon filled for bookmarked posts
   - [ ] Verify pin indicator for pinned posts

2. **Interaction toggling**:
   - [ ] Like a post → icon updates immediately
   - [ ] Unlike a post → icon updates immediately
   - [ ] Reblog a post → icon updates immediately
   - [ ] Bookmark a post → icon updates immediately
   - [ ] Scroll timeline → engagement state persists correctly

3. **Edge cases**:
   - [ ] Log out → all engagement icons disappear
   - [ ] Log in as different user → correct engagement state
   - [ ] Refresh page → engagement state preserved

**Acceptance Criteria**:
- [ ] All manual test scenarios pass
- [ ] No visual glitches or race conditions
- [ ] User experience feels instant (<100ms perceived latency)

#### 4.5: Load Testing
**Tool**: k6, Gatling, or Artillery

**Scenarios**:
```javascript
// Scenario 1: Authenticated timeline queries
export default function() {
  const token = authenticateUser();
  const response = http.post(GRAPHQL_URL, {
    query: `{ publicTimeline(limit: 20) { 
      id favourited reblogged bookmarked pinned 
    }}`
  }, { headers: { Authorization: `Bearer ${token}` }});
  
  check(response, {
    'status is 200': (r) => r.status === 200,
    'has engagement data': (r) => JSON.parse(r.body).data.publicTimeline[0].favourited !== undefined,
  });
}

// Scenario 2: Mixed authenticated/unauthenticated
// Scenario 3: High interaction rate (many likes/bookmarks)
```

**Load Profiles**:
- [ ] 100 concurrent users, 10 requests/sec, 5 minutes
- [ ] 500 concurrent users, 50 requests/sec, 5 minutes
- [ ] Spike test: 0 → 1000 users in 30 seconds

**Acceptance Criteria**:
- [ ] <1% error rate under sustained load
- [ ] p95 latency stays below 300ms
- [ ] DynamoDB auto-scaling handles load
- [ ] No Lambda throttling errors

**Phase 4 Deliverables**:
- [ ] Unit test suite with >90% coverage
- [ ] Integration tests for all workflows
- [ ] Performance benchmarks meeting targets
- [ ] Manual testing checklist completed
- [ ] Load testing report
- [ ] Bug fixes for any issues found

**Phase 4 Estimated Effort**: 8-10 hours

---

## Phase 5: Migration & Deployment

**Goal**: Safely deploy to production with zero downtime and data migration for existing bookmarks.

### Tasks

#### 5.1: Data Migration Script
**File**: `scripts/migrate_bookmarks.go` (new)

**Purpose**: Backfill OBJECT# records for existing TIME# bookmarks

**Implementation**:
```go
func migrateBookmarks(ctx context.Context, db core.DB, tableName string) error {
    // 1. Scan all BOOKMARK# partitions
    // 2. For each TIME#{timestamp}#{objectID} record:
    //    a. Extract objectID from SK
    //    b. Create OBJECT#{objectID} record with same data
    //    c. Use condition expression to avoid duplicates
    // 3. Batch write new records (25 per batch)
    // 4. Log progress and errors
    // 5. Dry-run mode for validation
}
```

**Features**:
- [ ] Dry-run mode (validate without writing)
- [ ] Resumable (checkpoint progress)
- [ ] Rate limiting (avoid DynamoDB throttling)
- [ ] Progress reporting (% complete, ETA)
- [ ] Error handling (log failures, continue)

**Acceptance Criteria**:
- [ ] Successfully migrates 100% of test bookmarks
- [ ] No duplicate records created
- [ ] Respects DynamoDB write capacity
- [ ] Completes migration of 10k bookmarks in <30 minutes

#### 5.2: Feature Flag Implementation
**File**: `pkg/config/features.go`

**Flag**: `ENABLE_BATCH_ENGAGEMENT_LOOKUP`

**Implementation**:
```go
type FeatureFlags struct {
    EnableBatchEngagementLookup bool `env:"ENABLE_BATCH_ENGAGEMENT_LOOKUP" default:"false"`
}

// In resolver
func (r *queryResolver) PublicTimeline(ctx context.Context, limit int) ([]*model.Object, error) {
    statuses := r.fetchStatuses(ctx, limit)
    
    if config.Features().EnableBatchEngagementLookup {
        // New batch path
        engagements := r.engagementRepo.GetViewerEngagements(ctx, viewer, statusIDs)
        return convertWithEngagements(statuses, engagements), nil
    } else {
        // Old path (no engagement or fallback to N+1)
        return convertStatuses(statuses), nil
    }
}
```

**Acceptance Criteria**:
- [ ] Flag defaults to OFF
- [ ] Can be toggled without code deployment
- [ ] Both code paths functional
- [ ] Metrics track flag state

#### 5.3: Observability & Monitoring
**Metrics to Add**:

```go
// Prometheus metrics
var (
    batchEngagementLookupDuration = prometheus.NewHistogramVec(...)
    batchEngagementLookupErrors   = prometheus.NewCounterVec(...)
    batchEngagementCacheHitRate   = prometheus.NewGaugeVec(...)
    bookmarkDualWriteSuccess      = prometheus.NewCounter(...)
    bookmarkDualWriteFailure      = prometheus.NewCounter(...)
)
```

**Logs to Add**:
- [ ] Batch engagement lookup timing
- [ ] DynamoDB BatchGetItem request size
- [ ] Partial failure details (which operations failed)
- [ ] Bookmark dual-write success/failure
- [ ] Migration script progress

**CloudWatch Alarms**:
- [ ] High error rate on batch engagement lookup (>1%)
- [ ] High latency on batch operations (p99 >300ms)
- [ ] Bookmark dual-write failure rate (>0.5%)
- [ ] DynamoDB throttling on engagement queries

**Acceptance Criteria**:
- [ ] All metrics exported to Prometheus/CloudWatch
- [ ] Grafana dashboard created
- [ ] Alarms configured and tested
- [ ] On-call runbook updated

#### 5.4: Deployment Plan
**Strategy**: Blue/Green deployment with gradual rollout

**Steps**:

1. **Pre-deployment (Dev environment)**:
   - [ ] Deploy code with feature flag OFF
   - [ ] Run integration tests
   - [ ] Verify no regressions

2. **Migration (Production)**:
   - [ ] Run bookmark migration script in dry-run mode
   - [ ] Review migration plan with team
   - [ ] Execute migration during low-traffic window
   - [ ] Verify migration success (sample checks)

3. **Deployment (Production)**:
   - [ ] Deploy new code with feature flag OFF
   - [ ] Smoke test existing functionality
   - [ ] Enable feature flag for 1% of traffic (canary)
   - [ ] Monitor metrics for 1 hour
   - [ ] Increase to 10% of traffic
   - [ ] Monitor for 2 hours
   - [ ] Increase to 50% of traffic
   - [ ] Monitor for 2 hours
   - [ ] Increase to 100% of traffic

4. **Post-deployment**:
   - [ ] Monitor dashboards for 24 hours
   - [ ] Verify Greater frontend shows engagement state
   - [ ] Collect user feedback
   - [ ] Document lessons learned

**Rollback Plan**:
- [ ] Disable feature flag (instant rollback)
- [ ] Redeploy previous version if flag insufficient
- [ ] OBJECT# bookmark records remain (harmless)
- [ ] No data loss

**Acceptance Criteria**:
- [ ] Zero downtime during deployment
- [ ] <1% error rate during rollout
- [ ] All monitoring dashboards green
- [ ] User-visible feature working correctly

#### 5.5: Documentation Updates
**Files to Update**:

- [ ] `docs/api-reference.md` - Document new GraphQL fields
- [ ] `docs/architecture.md` - Explain batch engagement pattern
- [ ] `docs/gsi_usage_guide.md` - Document bookmark dual-write pattern
- [ ] `README.md` - Update feature list
- [ ] `RUNBOOK.md` - Add troubleshooting for engagement queries

**New Documentation**:
- [ ] `docs/VIEWER_ENGAGEMENT_PATTERN.md` - Deep dive on implementation
- [ ] `docs/BOOKMARK_MIGRATION.md` - Migration process and rollback
- [ ] API changelog entry for GraphQL schema changes

**Acceptance Criteria**:
- [ ] All docs reviewed and approved
- [ ] Code examples tested and working
- [ ] Troubleshooting guide includes common issues

**Phase 5 Deliverables**:
- [ ] Migration script for existing bookmarks
- [ ] Feature flag implementation
- [ ] Monitoring dashboards and alarms
- [ ] Production deployment completed
- [ ] Documentation updated
- [ ] Post-deployment report

**Phase 5 Estimated Effort**: 8-10 hours

---

## Success Metrics

### Performance
- ✅ Timeline query latency: <300ms p99 (down from >500ms with N+1)
- ✅ Engagement lookup latency: <100ms p99
- ✅ DynamoDB RCU consumption: <20 RCUs per timeline query
- ✅ GraphQL query success rate: >99.9%

### Correctness
- ✅ Engagement state accuracy: 100% (verified via manual testing)
- ✅ No race conditions on bookmark create/delete
- ✅ Transaction success rate: >99.5% for dual-write bookmarks

### User Experience
- ✅ Greater frontend displays engagement state correctly
- ✅ Instant feedback on like/reblog/bookmark actions
- ✅ No visual glitches or stale data

### Operational
- ✅ Zero-downtime deployment
- ✅ Migration completed without data loss
- ✅ No P0/P1 incidents during rollout
- ✅ Monitoring dashboards tracking all key metrics

---

## Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Migration script corrupts data | HIGH | Dry-run mode, incremental rollout, backups |
| Dual-write increases costs | MEDIUM | Monitor costs, bookmarks are infrequent operations |
| Batch operations timeout | MEDIUM | Implement timeouts, partial failure handling |
| Feature breaks existing API | HIGH | Feature flag, comprehensive testing, gradual rollout |
| DynamoDB throttling | MEDIUM | Rate limiting, auto-scaling, BatchGetItem pagination |
| Greater frontend integration issues | MEDIUM | Early manual testing, clear API documentation |

---

## Dependencies & Prerequisites

### Technical
- [x] DynamoDB single-table design in place
- [x] Like, Announce, Bookmark, StatusPin models exist
- [ ] Greater frontend ready to consume new GraphQL fields
- [x] Authentication/viewer context in GraphQL resolvers
- [x] gqlgen code generation setup

### Team
- [ ] Code review approval from senior engineer
- [ ] QA sign-off on integration tests
- [ ] Product approval for deployment plan
- [ ] DevOps support for production deployment

### Infrastructure
- [x] DynamoDB auto-scaling configured
- [x] CloudWatch monitoring in place
- [ ] Feature flag service (or environment variable support)
- [ ] CI/CD pipeline for automated testing

---

## Timeline Estimate

| Phase | Duration | Dependencies | Parallelizable |
|-------|----------|--------------|----------------|
| Phase 1: Bookmark Dual-Write | 4-6 hours | None | No |
| Phase 2: Engagement Repository | 6-8 hours | Phase 1 complete | Partially |
| Phase 3: GraphQL Integration | 6-8 hours | Phase 2 complete | No |
| Phase 4: Testing & Validation | 8-10 hours | Phase 3 complete | Partially |
| Phase 5: Migration & Deployment | 8-10 hours | Phase 4 complete | No |

**Total Sequential**: ~32-42 hours (4-5 days for single developer)  
**Total with Parallelization**: ~24-32 hours (3-4 days with careful task assignment)

---

## Appendix

### A. DynamoDB Cost Analysis

**Current (N+1 Pattern)**:
- 20 statuses × 4 checks = 80 GetItem operations
- 80 × 1 RCU (4KB) = 80 RCUs per timeline query
- 1000 timeline queries/day = 80,000 RCUs/day
- Cost: ~$0.10/day (~$3/month)

**Optimized (Batch Pattern)**:
- 4 BatchGetItem operations × 20 items = 80 item reads
- 80 × 0.5 RCU (eventually consistent) = 40 RCUs per timeline query
- 1000 timeline queries/day = 40,000 RCUs/day
- Cost: ~$0.05/day (~$1.50/month)

**Bookmark Dual-Write Overhead**:
- Assume 100 bookmarks/day
- Current: 100 writes × 1 WCU = 100 WCUs/day
- Dual-write: 100 × 2 writes × 1 WCU = 200 WCUs/day
- Cost increase: ~$0.001/day (~$0.03/month)

**Total savings**: ~50% RCU reduction, negligible WCU increase

### B. Alternative Approaches Considered

#### 1. GSI for Bookmark Object Lookup
**Rejected**: Would require new GSI (GSI limit is 20, already using 8), adds write amplification to all bookmarks.

#### 2. Single Query + In-Memory Filter for All Engagements
**Rejected**: Would require querying all user's likes/reblogs (could be 1000s), inefficient for users with many interactions.

#### 3. Cache Viewer Engagement in Status Model
**Rejected**: Viewer-specific data shouldn't be in Status model, breaks ActivityPub first-class citizen principle.

#### 4. Use StatusEngagement (7-day TTL) for Persistence
**Rejected**: TTL makes it unreliable, not suitable for permanent interactions like bookmarks.

### C. Glossary

- **BatchGetItem**: DynamoDB operation to fetch up to 100 items by primary key in a single request
- **Dual-Write**: Pattern where one logical operation writes multiple physical records
- **DataLoader**: Pattern for batching and caching data fetches in GraphQL resolvers
- **GSI**: Global Secondary Index in DynamoDB
- **N+1 Problem**: Anti-pattern where N items require N+1 queries (1 for list, N for details)
- **RCU**: Read Capacity Unit in DynamoDB (one 4KB strongly consistent read)
- **WCU**: Write Capacity Unit in DynamoDB (one 1KB write)

---

## Approval & Sign-off

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Tech Lead | | | |
| Product Manager | | | |
| QA Lead | | | |
| DevOps | | | |

---

*Document Version*: 1.0  
*Created*: 2025-11-09  
*Last Updated*: 2025-11-09  
*Status*: Ready for Review
