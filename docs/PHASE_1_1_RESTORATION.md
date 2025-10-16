# Phase 1.1 Restoration: Hashtag Following System

**Status**: LOST WORK RECOVERY  
**Effort**: 2-3 days  
**Priority**: CRITICAL BLOCKER  
**Dependency**: None

---

## Architecture (DO NOT DEVIATE)

All work goes in SAFE files (not schema.resolvers.go):
- **Service layer**: `pkg/services/hashtags/service.go` (business logic - SAFE)
- **Storage**: `pkg/storage/repositories/hashtag_repository.go` (data access - SAFE)
- **Resolvers**: `graph/query_resolvers_hashtags.go`, `graph/mutation_resolvers_hashtags.go`, `graph/subscription_resolvers_hashtags.go` (SAFE)
- **Helpers**: `graph/helpers.go` (conversions - SAFE)

**NEVER put implementation in `schema.resolvers.go`** - it's auto-generated and will be wiped.

---

## Work Breakdown

### PART 1: Service Layer & Storage (Day 1)

**File**: `pkg/services/hashtags/service.go`

Build these methods (patterns from `/pkg/services/notes/service.go`):

```go
type Service struct {
    hashtagRepo    *repositories.HashtagRepository
    accountRepo    interfaces.AccountRepository
    objectRepo     *repositories.ObjectRepository
    publisher      streaming.Publisher
    logger         *zap.Logger
}

// GetHashtag returns complete hashtag data
func (s *Service) GetHashtag(ctx context.Context, name, viewerID string) (*Hashtag, error) {
    // 1. Get hashtag stats from hashtag indexer or compute
    // 2. Check if viewer is following it
    // 3. Get related hashtags (co-occurrence)
    // 4. Get notification settings for viewer
    // 5. Return full Hashtag object
}

// FollowHashtag creates follow relationship
func (s *Service) FollowHashtag(ctx context.Context, userID, hashtag string, settings *NotificationSettings) error {
    // 1. Create HashtagFollow record in storage
    // 2. Store notification settings
    // 3. Emit hashtagFollowed event to streaming.EventBus
    // 4. Update user's followed count
}

// UnfollowHashtag removes follow relationship
func (s *Service) UnfollowHashtag(ctx context.Context, userID, hashtag string) error {
    // 1. Delete HashtagFollow record
    // 2. Emit hashtagUnfollowed event
    // 3. Update user's followed count
}

// GetFollowedHashtags lists user's followed hashtags
func (s *Service) GetFollowedHashtags(ctx context.Context, userID string, pagination *PaginationOptions) ([]*Hashtag, string, error) {
    // 1. Query all HashtagFollow records for user
    // 2. For each, get current stats
    // 3. Enrich with notification settings
    // 4. Sort by recency or activity
    // 5. Apply pagination
}

// MuteHashtag creates mute record with expiration
func (s *Service) MuteHashtag(ctx context.Context, userID, hashtag string, until *time.Time) error {
    // 1. Create HashtagMute record with expiration
    // 2. Emit hashtagMuted event
}

// GetHashtagActivity returns channel of activity updates
func (s *Service) GetHashtagActivity(ctx context.Context, hashtags []string) (<-chan *ActivityEvent, error) {
    // 1. Subscribe to streaming event bus
    // 2. Filter for hashtag-related events
    // 3. Return channel that streams events
    // Use pattern from /pkg/services/notes/service.go (if exists)
}
```

**Reference patterns**:
- `/pkg/services/notes/service.go` - Service layer template
- `/pkg/services/relationships/service.go` - Follow-like patterns
- `/pkg/services/notifications/service.go` - Subscription/streaming patterns

---

### PART 2: Storage Models & Repository (Day 1-2)

**File**: `pkg/storage/repositories/hashtag_repository.go`

Add these methods (patterns from `/pkg/storage/repositories/relationship_repository.go`):

```go
// Storage Models needed in pkg/storage/models/:
// - HashtagFollow: PK=user#{userID}, SK=hashtag#{name}
// - HashtagMute: PK=user#{userID}, SK=mute#{name}, TTL=expiration
// - HashtagNotificationSettings: PK=user#{userID}, SK=settings#{name}

// Repository methods:
func (r *HashtagRepository) FollowHashtag(ctx context.Context, userID, hashtag string) error
func (r *HashtagRepository) UnfollowHashtag(ctx context.Context, userID, hashtag string) error
func (r *HashtagRepository) IsFollowingHashtag(ctx context.Context, userID, hashtag string) (bool, error)
func (r *HashtagRepository) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]*storage.HashtagFollow, string, error)
func (r *HashtagRepository) MuteHashtag(ctx context.Context, userID, hashtag string, until *time.Time) error
func (r *HashtagRepository) UnmuteHashtag(ctx context.Context, userID, hashtag string) error
func (r *HashtagRepository) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error)
func (r *HashtagRepository) GetHashtagNotificationSettings(ctx context.Context, userID, hashtag string) (*storage.HashtagNotificationSettings, error)
func (r *HashtagRepository) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, settings *storage.HashtagNotificationSettings) error
```

**Pattern reference**: `/pkg/storage/repositories/relationship_repository.go`

---

### PART 3: GraphQL Helpers (Day 2)

**File**: `graph/helpers.go`

Add this converter function:

```go
// convertHashtagToModel converts service Hashtag to GraphQL model.Hashtag
// This is THE converter used by all resolvers - consistency is critical
func (r *Resolver) convertHashtagToModel(ctx context.Context, hashtag *services.Hashtag, viewerID string) *model.Hashtag {
    if hashtag == nil {
        return nil
    }

    // Get notification settings
    settings := r.fetchHashtagNotificationSettings(ctx, hashtag.Name, viewerID)
    
    // Get related hashtags
    related := r.getRelatedHashtags(ctx, hashtag.Name, 5)
    
    return &model.Hashtag{
        // Required fields
        Name:                  hashtag.Name,
        URL:                   fmt.Sprintf("https://%s/tags/%s", r.Registry.Config().Domain, hashtag.Name),
        DisplayName:           "#" + hashtag.Name,
        
        // Stats
        PostCount:             hashtag.PostCount,
        FollowersCount:        hashtag.FollowersCount,
        FollowingCount:        hashtag.FollowingCount,
        
        // Viewer context
        IsFollowing:           r.isFollowingHashtag(ctx, viewerID, hashtag.Name),
        IsMuted:               r.isHashtagMuted(ctx, viewerID, hashtag.Name),
        NotificationSettings:  settings,
        
        // Related
        RelatedHashtags:       related,
        
        // Timestamps
        CreatedAt:             model.Time(hashtag.CreatedAt),
        UpdatedAt:             model.Time(hashtag.UpdatedAt),
    }
}

// Helper to fetch notification settings (DO NOT hardcode)
func (r *Resolver) fetchHashtagNotificationSettings(ctx context.Context, hashtag, userID string) *model.HashtagNotificationSettings {
    // Query storage for actual settings (NOT hardcoded)
    settings, err := r.Storage.Hashtag().GetHashtagNotificationSettings(ctx, userID, hashtag)
    if err != nil {
        return &model.HashtagNotificationSettings{
            Level:   model.NotificationLevelNone,
            Muted:   false,
            Filters: []*model.NotificationFilter{},
        }
    }
    
    return &model.HashtagNotificationSettings{
        Level:      model.NotificationLevel(settings.Level),
        Muted:      settings.Muted,
        MutedUntil: model.Time(settings.MutedUntil),
        Filters:    settings.Filters,
    }
}

// Helper to get related hashtags (co-occurrence based)
func (r *Resolver) getRelatedHashtags(ctx context.Context, hashtag string, limit int) []*model.Hashtag {
    // Query object repository for posts with this hashtag
    // Find co-occurring hashtags in same posts
    // Return top N by frequency
    // DO NOT return empty slice
}

// Helper to check if user is following
func (r *Resolver) isFollowingHashtag(ctx context.Context, userID, hashtag string) bool {
    following, err := r.Storage.Hashtag().IsFollowingHashtag(ctx, userID, hashtag)
    if err != nil {
        return false
    }
    return following
}

// Helper to check if user has muted
func (r *Resolver) isHashtagMuted(ctx context.Context, userID, hashtag string) bool {
    muted, err := r.Storage.Hashtag().IsHashtagMuted(ctx, userID, hashtag)
    if err != nil {
        return false
    }
    return muted
}
```

---

### PART 4: Query Resolvers (Day 2)

**File**: `graph/query_resolvers_hashtags.go`

Implement these (copy from stubs, delegate to service):

```go
// Query.hashtag(name)
func (r *queryResolver) Hashtag(ctx context.Context, name string) (*model.Hashtag, error) {
    username := r.optionalAuth(ctx)
    
    // Call service to get hashtag
    hashtag, err := r.Registry.Hashtags().GetHashtag(ctx, name, username)
    if err != nil {
        r.Logger.Error("failed to get hashtag", zap.Error(err))
        return nil, err
    }
    
    // Convert to GraphQL model using the converter
    return r.convertHashtagToModel(ctx, hashtag, username), nil
}

// Query.followedHashtags(first, after)
func (r *queryResolver) FollowedHashtags(ctx context.Context, first *int, after *string) (*model.HashtagConnection, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }
    
    limit := 20
    if first != nil {
        limit = *first
    }
    
    // Get followed hashtags from service
    hashtags, nextCursor, err := r.Registry.Hashtags().GetFollowedHashtags(ctx, username, &interfaces.PaginationOptions{
        Limit:  limit,
        Cursor: after,
    })
    if err != nil {
        r.Logger.Error("failed to get followed hashtags", zap.Error(err))
        return nil, err
    }
    
    // Convert to edges
    edges := make([]*model.HashtagEdge, len(hashtags))
    for i, hashtag := range hashtags {
        edges[i] = &model.HashtagEdge{
            Node:   r.convertHashtagToModel(ctx, hashtag, username),
            Cursor: model.Cursor(hashtag.Name), // Use hashtag name as cursor
        }
    }
    
    return &model.HashtagConnection{
        Edges:      edges,
        PageInfo:   &model.PageInfo{HasNextPage: nextCursor != ""},
    }, nil
}

// Query.hashtagTimeline(hashtag, first, after)
// Query.multiHashtagTimeline(hashtags, mode, first, after)
// Query.suggestedHashtags(limit)
// Similar patterns - delegate to service
```

---

### PART 5: Mutation Resolvers (Day 2-3)

**File**: `graph/mutation_resolvers_hashtags.go`

**CRITICAL**: After mutation succeeds, fetch fresh data and use converter:

```go
// Mutation.followHashtag
func (r *mutationResolver) FollowHashtag(ctx context.Context, hashtag string, level *model.NotificationLevel) (*model.HashtagFollowPayload, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }
    
    // Build settings from input
    settings := &storage.HashtagNotificationSettings{
        Level: "all", // Default
    }
    if level != nil {
        settings.Level = string(*level)
    }
    
    // Call service to follow
    err = r.Registry.Hashtags().FollowHashtag(ctx, username, hashtag, settings)
    if err != nil {
        r.Logger.Error("failed to follow hashtag", zap.String("hashtag", hashtag), zap.Error(err))
        return &model.HashtagFollowPayload{Success: false}, err
    }
    
    // FETCH FRESH DATA - don't hand-roll the hashtag
    freshHashtag, err := r.Registry.Hashtags().GetHashtag(ctx, hashtag, username)
    if err != nil {
        r.Logger.Error("failed to fetch hashtag after follow", zap.Error(err))
        return nil, err
    }
    
    // Use converter for full population
    return &model.HashtagFollowPayload{
        Success: true,
        Hashtag: r.convertHashtagToModel(ctx, freshHashtag, username),
    }, nil
}

// Mutation.unfollowHashtag
// Mutation.updateHashtagNotifications
// Mutation.muteHashtag
// Same pattern: call service -> fetch fresh data -> use converter
```

---

### PART 6: Subscription Resolver (Day 3)

**File**: `graph/subscription_resolvers_hashtags.go`

**CRITICAL**: Use unified SubscriptionManager pattern (NOT direct event bus):

```go
// Subscription.hashtagActivity(hashtags)
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }
    
    activityChan := make(chan *model.HashtagActivityUpdate, 100)
    
    // Use the subscription manager (centralized pattern)
    // DO NOT subscribe directly to event bus
    
    // Get subscription manager
    sm := r.SubscriptionManager
    if sm == nil {
        close(activityChan)
        return activityChan, errors.New("subscription manager not available")
    }
    
    // Create subscription config
    config := &SubscriptionConfig{
        ID:            fmt.Sprintf("hashtag_activity_%s_%d", username, time.Now().UnixNano()),
        Type:          "hashtag_activity",
        UserID:        username,
        BufferSize:    100,
        OutputChannel: activityChan,
        Filter: &streaming.EventFilter{
            Types:   []streaming.EventType{streaming.EventTypeStatus, streaming.EventTypeHashtagTrend},
            Streams: buildHashtagStreams(hashtags),
            UserID:  username,
        },
    }
    
    // Subscribe through manager
    err = sm.Subscribe(ctx, config)
    if err != nil {
        close(activityChan)
        return activityChan, err
    }
    
    // Start forwarding events in goroutine
    go func() {
        defer close(activityChan)
        // Manager handles sending to activityChan
    }()
    
    return activityChan, nil
}

// Helper to build stream names
func buildHashtagStreams(hashtags []string) []string {
    streams := []string{"hashtags:global"}
    for _, tag := range hashtags {
        streams = append(streams, fmt.Sprintf("hashtag:%s", strings.ToLower(tag)))
    }
    return streams
}
```

---

## Testing Checklist

**Unit Tests**: `pkg/services/hashtags/service_test.go`
- [ ] GetHashtag returns full model
- [ ] FollowHashtag creates record + emits event
- [ ] UnfollowHashtag deletes record + emits event
- [ ] GetFollowedHashtags respects pagination
- [ ] MuteHashtag respects TTL

**Resolver Tests**: `graph/*_resolvers_hashtags_test.go`
- [ ] Query.hashtag returns complete model
- [ ] Query.followedHashtags returns edges with pagination
- [ ] Mutation.followHashtag returns fresh data
- [ ] Mutation.unfollowHashtag works
- [ ] Subscription.hashtagActivity streams events

**Integration Test**: `graph/schema.resolvers_test.go`
- [ ] End-to-end follow/unfollow workflow
- [ ] Notification settings persist
- [ ] Related hashtags populate
- [ ] Muting prevents notifications

---

## Success Criteria

- [ ] All 10 Phase 1.1 operations working
- [ ] No hardcoded values (all data from storage/service)
- [ ] Subscriptions use unified manager
- [ ] Model converter used consistently
- [ ] Tests passing (80%+ coverage)
- [ ] No regressions on other features

---

## If You Get Stuck

1. **Look at `/pkg/services/notes/service.go`** - the gold standard
2. **Look at `/pkg/services/relationships/service.go`** - follow patterns
3. **Look at `/pkg/services/notifications/service.go`** - subscription patterns
4. **Look at `/graph/subscription_handlers.go`** - subscription manager integration

These are your blueprints. Follow the patterns exactly.

---

## DO NOT REPEAT THE MISTAKE

- ❌ Don't put logic in `schema.resolvers.go`
- ❌ Don't hardcode values
- ❌ Don't hand-roll model conversions
- ❌ Don't bypass the subscription manager
- ❌ Don't commit uncommitted work

Everything goes in SAFE files. Everything delegates to service layer. Everything uses converters.
