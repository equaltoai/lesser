# Team B Corrected Tasks - Federation & Infrastructure Support

**Mission**: Support Team A's GraphQL implementations with storage layer and federation features

**Current Status**: Storage layer solid, need to add GraphQL support infrastructure
**Priority**: HIGH - Support critical GraphQL operations  
**Timeline**: 2-3 weeks coordinated with Team A

## CRITICAL PRIORITY: Storage Support for GraphQL Operations

### Task 1: Search Infrastructure (HIGHEST PRIORITY)
**Support Team A's Search GraphQL implementation**

#### 1A: Enhanced Search Storage Methods
**File**: `pkg/storage/interface.go` + `pkg/storage/dynamodb/search_service.go`

Add missing search methods:
```go
// Add to storage interface
type Storage interface {
    // ... existing methods
    
    // Multi-type search support
    SearchAll(ctx context.Context, query string, limit int) (*SearchResults, error)
    SearchAccounts(ctx context.Context, query string, resolve bool, limit int, offset int) ([]*User, error)
    SearchStatuses(ctx context.Context, query string, limit int, maxID, minID string) ([]*Status, error)
    SearchHashtags(ctx context.Context, query string, limit int) ([]*Tag, error)
}

type SearchResults struct {
    Accounts []*User   `json:"accounts"`
    Statuses []*Status `json:"statuses"`  
    Hashtags []*Tag    `json:"hashtags"`
}
```

#### 1B: Search Index Optimization
- [ ] Optimize existing semantic search for GraphQL queries
- [ ] Add proper result ranking and relevance scoring
- [ ] Implement search result caching for performance
- [ ] Add search analytics and tracking

### Task 2: Notification Storage Infrastructure (HIGHEST PRIORITY)
**Support Team A's Notifications GraphQL implementation**

#### 2A: Enhanced Notification Storage
**File**: `pkg/storage/interface.go` + `pkg/storage/dynamodb/notifications.go`

```go
// Enhanced notification methods
type Storage interface {
    // ... existing methods
    
    // Notification filtering and pagination  
    GetNotificationsFiltered(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, includeFiltered bool) ([]*Notification, error)
    GetNotificationsByAccount(ctx context.Context, userID, accountID string, limit int) ([]*Notification, error)
    MarkNotificationAsRead(ctx context.Context, userID, notificationID string) error
    MarkAllNotificationsAsRead(ctx context.Context, userID string) error
    GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error)
}
```

#### 2B: Notification Performance Optimization
- [ ] Add GSI for notification filtering by type
- [ ] Implement notification aggregation for performance
- [ ] Add push notification integration points
- [ ] Optimize notification queries for large user bases

### Task 3: Hashtag Storage Infrastructure (HIGH PRIORITY)
**Support Team A's Hashtag GraphQL operations**

#### 3A: Hashtag Following Storage
**File**: `pkg/storage/interface.go` + `pkg/storage/dynamodb/hashtag_follow.go`

```go
// Hashtag following methods (may already exist - verify and enhance)
type Storage interface {
    // ... existing methods
    
    // Hashtag following operations
    FollowHashtag(ctx context.Context, userID, hashtag string) error
    UnfollowHashtag(ctx context.Context, userID, hashtag string) error
    IsFollowingHashtag(ctx context.Context, userID, hashtag string) (bool, error)
    GetFollowedHashtags(ctx context.Context, userID string) ([]*Tag, error)
    UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error
    MuteHashtag(ctx context.Context, userID, hashtag string) error
    IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error)
    
    // Hashtag timeline and discovery
    GetHashtagTimeline(ctx context.Context, hashtag string, maxID *string, limit int) ([]*Status, error)
    GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int) ([]*Status, error)
    GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*Tag, error)
    GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*Activity, error)
}
```

#### 3B: Hashtag Analytics and Trends
- [ ] Enhance existing hashtag trend calculations
- [ ] Add hashtag usage analytics
- [ ] Implement hashtag recommendation algorithms
- [ ] Track hashtag engagement metrics

### Task 4: Quote Tweet Storage Infrastructure (MEDIUM PRIORITY)
**Support Team A's Quote Tweet operations**

#### 4A: Quote Tweet Storage Methods
**File**: `pkg/storage/interface.go` + `pkg/storage/dynamodb/quotes.go`

```go
type Storage interface {
    // ... existing methods
    
    // Quote tweet operations
    WithdrawStatusFromQuotes(ctx context.Context, statusID string) error
    UpdateQuotePermissions(ctx context.Context, statusID string, permissions QuotePermissions) error
    IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error)
    GetQuoteType(ctx context.Context, statusID string) (string, error)
    IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error)
    GetQuoteActivity(ctx context.Context, statusID string, since time.Time) ([]*Activity, error)
    GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*Status, error)
}

type QuotePermissions struct {
    AllowPublic   bool
    AllowFollowers bool 
    AllowMentioned bool
    BlockList     []string
}
```

### Task 5: Thread Synchronization Infrastructure (MEDIUM PRIORITY)
**Support Team A's Thread Management operations**

#### 5A: Thread Storage Methods
**File**: `pkg/storage/interface.go` + `pkg/storage/dynamodb/threads.go`

```go
type Storage interface {
    // ... existing methods
    
    // Thread management
    SyncThreadFromRemote(ctx context.Context, statusID string) (*Status, error)
    SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*Status, error)
    GetThreadContext(ctx context.Context, statusID string) (*ThreadContext, error)
    MarkThreadAsSynced(ctx context.Context, statusID string) error
    GetMissingReplies(ctx context.Context, statusID string) ([]*Status, error)
}

type ThreadContext struct {
    Ancestors   []*Status `json:"ancestors"`
    Descendants []*Status `json:"descendants"`
}
```

### Task 6: Federation Severance Infrastructure (MEDIUM PRIORITY)
**Support federation disconnect/reconnect operations**

#### 6A: Federation Relationship Tracking
**File**: `pkg/storage/interface.go` + `pkg/storage/dynamodb/federation_graph.go`

```go
type Storage interface {
    // ... existing methods
    
    // Federation severance tracking
    AcknowledgeSeverance(ctx context.Context, userID, domain string) error
    AttemptReconnection(ctx context.Context, userID, domain string) error
    GetSeveredRelationships(ctx context.Context, userID string) ([]*SeveredRelationship, error)
    GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*Relationship, error)
    TrackFederationIssue(ctx context.Context, domain, issueType string) error
}

type SeveredRelationship struct {
    Domain      string    `json:"domain"`
    SeveredAt   time.Time `json:"severed_at"`
    Acknowledged bool     `json:"acknowledged"`
    Reason      string    `json:"reason"`
}
```

## SECONDARY PRIORITY: Performance & Infrastructure

### Task 7: Database Optimization for GraphQL
**Optimize DynamoDB for GraphQL query patterns**

#### 7A: GSI Optimization
- [ ] Add GSIs for notification filtering
- [ ] Optimize hashtag timeline queries
- [ ] Add search result caching GSIs
- [ ] Monitor and optimize costly queries

#### 7B: Caching Layer Enhancement
- [ ] Implement Redis caching for frequently accessed data
- [ ] Add GraphQL query result caching
- [ ] Cache hashtag timelines and trends
- [ ] Cache search results with TTL

### Task 8: Federation Protocol Enhancement
**Improve federation robustness**

#### 8A: Enhanced Activity Processing
- [ ] Add retry logic for failed federation operations
- [ ] Implement federation health monitoring
- [ ] Add federation analytics and reporting
- [ ] Optimize delivery performance

#### 8B: Federation Security
- [ ] Enhance signature verification
- [ ] Add federation rate limiting
- [ ] Implement federation blocklists
- [ ] Add federation audit logging

## Implementation Coordination with Team A

### Phase 1 (Week 1): Critical Infrastructure
**Priority**: Support Search & Notifications
1. Implement search storage methods
2. Enhance notification storage
3. Coordinate with Team A on interfaces

### Phase 2 (Week 2): Social Features  
**Priority**: Support Hashtag operations
1. Complete hashtag following infrastructure
2. Implement hashtag timeline optimization
3. Add hashtag analytics

### Phase 3 (Week 3): Advanced Features
**Priority**: Support Quote tweets and Threading
1. Implement quote tweet storage
2. Add thread synchronization
3. Complete federation severance tracking

## Testing Requirements

### Database Integration Tests:
- [ ] Test all new storage methods with real DynamoDB
- [ ] Performance test GraphQL query patterns
- [ ] Test federation integration scenarios
- [ ] Validate caching behavior

### Build Requirements:
```bash
# After each storage method:
make fmt && make lint && make build && make test

# Integration testing:
make test-storage
make test-federation
```

## Success Criteria

### Task Complete When:
- [ ] All storage methods needed by Team A's GraphQL implementations exist
- [ ] Performance is optimized for GraphQL query patterns
- [ ] Federation works reliably with new features
- [ ] All integration tests pass
- [ ] Database schema supports all new operations

## Timeline Coordination

**Week 1**: Search & Notification infrastructure (coordinate with Team A)
**Week 2**: Hashtag & social infrastructure (coordinate with Team A)  
**Week 3**: Advanced features & performance optimization

**Target**: Complete infrastructure support for all Team A GraphQL implementations within 3 weeks