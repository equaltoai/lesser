# Team A Corrected Tasks - GraphQL & API Completion

**Mission**: Fix critical GraphQL panics and complete API functionality

**Current Status**: 75% complete - Major GraphQL layer gaps identified
**Priority**: CRITICAL - 27 GraphQL operations panic when called
**Timeline**: 2-3 weeks for full completion

## CRITICAL PRIORITY: Fix GraphQL Panics

### Task 1: Core Search & Notifications (HIGHEST PRIORITY)
**File**: `graph/schema.resolvers.go`
**Issue**: Search and Notifications panic - core user functionality broken

#### 1A: Implement Search Operation (Line 2892)
```go
// Current: panic(fmt.Errorf("not implemented: Search"))
// Required: Implement actual search across posts, users, hashtags
func (r *queryResolver) Search(ctx context.Context, q string, type_ *string, resolve *bool, limit *int, maxID *string, minID *string, offset *int) (*model.Results, error) {
    // Implement comprehensive search
    return r.searchService.Search(ctx, q, type_, resolve, limit, maxID, minID, offset)
}
```

#### 1B: Implement Notifications Operation (Line 2897)
```go
// Current: panic(fmt.Errorf("not implemented: Notifications"))
// Required: Return user's notifications
func (r *queryResolver) Notifications(ctx context.Context, maxID *string, sinceID *string, minID *string, limit *int, excludeTypes []string, accountID *string, includeFiltered *bool) ([]*model.Notification, error) {
    // Implement notification fetching
    return r.notificationService.GetNotifications(ctx, maxID, sinceID, minID, limit, excludeTypes, accountID, includeFiltered)
}
```

### Task 2: Hashtag Operations (HIGH PRIORITY)
**File**: `graph/schema.resolvers.go`
**Issue**: 6 hashtag operations panic - hashtag functionality completely broken

#### 2A: Hashtag Following (Lines 2670, 2675, 2680, 2685)
```go
// Implement FollowHashtag
func (r *mutationResolver) FollowHashtag(ctx context.Context, name string) (*model.Tag, error) {
    return r.hashtagService.FollowHashtag(ctx, name)
}

// Implement UnfollowHashtag  
func (r *mutationResolver) UnfollowHashtag(ctx context.Context, name string) (*model.Tag, error) {
    return r.hashtagService.UnfollowHashtag(ctx, name)
}

// Implement UpdateHashtagNotifications
func (r *mutationResolver) UpdateHashtagNotifications(ctx context.Context, name string, notify bool) (*model.Tag, error) {
    return r.hashtagService.UpdateNotifications(ctx, name, notify)
}

// Implement MuteHashtag
func (r *mutationResolver) MuteHashtag(ctx context.Context, name string) (*model.Tag, error) {
    return r.hashtagService.MuteHashtag(ctx, name)
}
```

#### 2B: Hashtag Queries (Lines 3724, 3729, 3734, 3739, 3744, 3945)
```go
// Implement all hashtag query operations
func (r *queryResolver) Hashtag(ctx context.Context, name string) (*model.Tag, error) {
    return r.hashtagService.GetHashtag(ctx, name)
}

func (r *queryResolver) FollowedHashtags(ctx context.Context) ([]*model.Tag, error) {
    return r.hashtagService.GetFollowedHashtags(ctx)
}

func (r *queryResolver) HashtagTimeline(ctx context.Context, hashtag string, maxID *string, limit *int) ([]*model.Status, error) {
    return r.hashtagService.GetTimeline(ctx, hashtag, maxID, limit)
}

// Implement remaining hashtag operations...
```

### Task 3: Quote Tweet Operations (MEDIUM PRIORITY)
**File**: `graph/schema.resolvers.go`  
**Issue**: 6 quote operations panic (Lines 2660, 2665, 3774, 3779, 3784, 3940)

```go
// Implement all quote-related operations
func (r *mutationResolver) WithdrawFromQuotes(ctx context.Context, statusID string) (*model.Status, error) {
    return r.quoteService.WithdrawFromQuotes(ctx, statusID)
}

func (r *mutationResolver) UpdateQuotePermissions(ctx context.Context, statusID string, permissions string) (*model.Status, error) {
    return r.quoteService.UpdatePermissions(ctx, statusID, permissions)
}

// Implement remaining quote operations...
```

### Task 4: Thread Management (MEDIUM PRIORITY)
**File**: `graph/schema.resolvers.go`
**Issue**: 3 thread operations panic (Lines 2690, 2695, 3749)

```go
// Implement thread synchronization
func (r *mutationResolver) SyncThread(ctx context.Context, statusID string) (*model.Status, error) {
    return r.threadService.SyncThread(ctx, statusID)
}

func (r *mutationResolver) SyncMissingReplies(ctx context.Context, statusID string) ([]*model.Status, error) {
    return r.threadService.SyncMissingReplies(ctx, statusID)
}

func (r *queryResolver) ThreadContext(ctx context.Context, statusID string) (*model.Context, error) {
    return r.threadService.GetContext(ctx, statusID)
}
```

## SECONDARY PRIORITY: Complete API Handlers

### Task 5: Admin Interface Completion
**File**: `cmd/api/handlers/admin.go`
**Issue**: Missing User struct fields (Lines 212, 339)

- [ ] Add "Silenced" field to User struct in models
- [ ] Update admin user queries to include silenced status
- [ ] Ensure proper serialization of silenced field

### Task 6: Instance Configuration
**File**: `cmd/api/handlers/instance.go`  
**Issue**: Versioned terms of service not implemented (Line 358)

- [ ] Implement versioned terms of service storage
- [ ] Add terms version tracking in database
- [ ] Create API endpoints for terms management

### Task 7: Relationship Handler Fix
**File**: `cmd/api/handlers/relationships.go`
**Issue**: Signature mismatch with isDomainBlocked (Line 155)

- [ ] Fix method signature to match interface
- [ ] Ensure domain blocking functionality works correctly

## Implementation Requirements

### Service Layer Creation
Team A must create these new service layers:

```go
// Required service interfaces
type SearchService interface {
    Search(ctx context.Context, query string, type_ *string, resolve *bool, limit *int, maxID, minID *string, offset *int) (*model.Results, error)
}

type NotificationService interface {
    GetNotifications(ctx context.Context, maxID, sinceID, minID *string, limit *int, excludeTypes []string, accountID *string, includeFiltered *bool) ([]*model.Notification, error)
}

type HashtagService interface {
    FollowHashtag(ctx context.Context, name string) (*model.Tag, error)
    UnfollowHashtag(ctx context.Context, name string) (*model.Tag, error)
    UpdateNotifications(ctx context.Context, name string, notify bool) (*model.Tag, error)
    MuteHashtag(ctx context.Context, name string) (*model.Tag, error)
    GetHashtag(ctx context.Context, name string) (*model.Tag, error)
    GetFollowedHashtags(ctx context.Context) ([]*model.Tag, error)
    GetTimeline(ctx context.Context, hashtag string, maxID *string, limit *int) ([]*model.Status, error)
}

type QuoteService interface {
    WithdrawFromQuotes(ctx context.Context, statusID string) (*model.Status, error)
    UpdatePermissions(ctx context.Context, statusID string, permissions string) (*model.Status, error)
    // ... other quote methods
}

type ThreadService interface {
    SyncThread(ctx context.Context, statusID string) (*model.Status, error)
    SyncMissingReplies(ctx context.Context, statusID string) ([]*model.Status, error)
    GetContext(ctx context.Context, statusID string) (*model.Context, error)
}
```

## Testing Requirements

### Critical Testing:
- [ ] Test all 27 previously-panicking GraphQL operations
- [ ] Integration tests for search functionality
- [ ] End-to-end tests for notification system
- [ ] GraphQL schema validation tests

### Build Requirements:
```bash
# After each service implementation:
make fmt && make lint && make build && make test

# Final verification:
go test ./graph/... -v
```

## Success Criteria

### Task Complete When:
- [ ] All 27 GraphQL panic statements replaced with working implementations
- [ ] Search returns actual results (not panic)
- [ ] Notifications return user's actual notifications (not panic)
- [ ] Hashtag operations work end-to-end
- [ ] Admin interface fields complete
- [ ] All integration tests pass

## Timeline

- **Week 1**: Search & Notifications (critical user functionality)
- **Week 2**: Hashtag operations (social discovery)
- **Week 3**: Quote tweets, thread management, admin fixes

**Target**: 3 weeks to eliminate all GraphQL panics and complete API layer