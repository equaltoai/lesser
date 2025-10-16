# Phase 1.1 Remediation Prompt - Production Ready Implementation

**Status**: Current implementation has surface fixes but 4 critical issues remain blocking production readiness.

---

## 🎯 EXACT ISSUES TO FIX

### Issue 1: Hashtag Model Converter Not Fully Populating Required Fields

**Location**: `graph/schema.resolvers.go` - `convertHashtagToModel()` function (lines ~3582-3688)

**Problem**: 
The schema requires these **non-null fields** (see line 828-840 of schema.graphql):
- `name: String!` ✅ Populated
- `displayName: String!` ✅ Populated
- `url: String!` ✅ Populated
- `followerCount: Int!` ❌ Using placeholder/synthetic
- `postCount: Int!` ❌ Using placeholder/synthetic
- `trendingScore: Float!` ❌ Using placeholder/synthetic
- `isFollowing: Boolean!` ✅ Populated
- `posts(first, after): PostConnection!` ❌ **MISSING ENTIRELY** (non-null connection)
- `relatedHashtags: [Hashtag!]!` ❌ Placeholder nodes, not real data
- `analytics: HashtagAnalytics!` ❌ Synthetic/hardcoded
- `notificationSettings: HashtagNotificationSettings` ❌ Hardcoded defaults

**Required Fix**:

1. **Fetch real notification settings** in converter:
   ```go
   // In convertHashtagToModel, after getting hashtag info:
   notificationSettings, err := s.hashtagRepo.GetHashtagNotificationSettings(ctx, viewerID, result.Name)
   if err == nil && notificationSettings != nil {
       hashtag.NotificationSettings = &model.HashtagNotificationSettings{
           Level: parseNotificationLevel(notificationSettings.Level),
           Muted: notificationSettings.Muted,
           MutedUntil: convertTimePointer(notificationSettings.MutedUntil),
           Filters: convertNotificationFilters(notificationSettings.Filters),
       }
   } else {
       // Only default if fetch fails
       hashtag.NotificationSettings = &model.HashtagNotificationSettings{
           Level: model.NotificationLevelAll,
           Muted: false,
       }
   }
   ```

2. **Build posts connection** (required non-null field):
   ```go
   // Add after stats are set
   postsResult, err := s.GetHashtagTimeline(ctx, &hashtags.GetHashtagTimelineQuery{
       Hashtag:    result.Name,
       First:      20,
       After:      nil,
       Visibility: model.VisibilityPublic,
   })
   if err != nil {
       r.Logger.Warn("failed to get hashtag posts", zap.String("hashtag", result.Name))
       postsResult = &hashtags.PostConnection{Posts: []*storage.StatusSearchResult{}, HasMore: false}
   }
   
   // Convert posts to PostConnection model (use existing convertStatusToObject pattern)
   edges := make([]*model.PostEdge, len(postsResult.Posts))
   for i, post := range postsResult.Posts {
       edges[i] = &model.PostEdge{
           Node:   convertStatusToObject(ctx, post), // reuse existing converter
           Cursor: model.Cursor(post.StatusID),
       }
   }
   hashtag.Posts = &model.PostConnection{
       Edges:      edges,
       PageInfo:   &model.PageInfo{HasNextPage: postsResult.HasMore},
       TotalCount: len(edges),
   }
   ```

3. **Fetch real analytics** instead of synthetic:
   ```go
   // Query analytics from service/repository
   analytics, err := s.hashtagRepo.GetHashtagAnalytics(ctx, result.Name)
   if err != nil {
       r.Logger.Warn("failed to get hashtag analytics", zap.String("hashtag", result.Name))
       // Still need to return non-null, but with zero values
       hashtag.Analytics = &model.HashtagAnalytics{
           HourlyPosts: []int{},
           DailyPosts:  []int{},
           TopPosters:  []*activitypub.Actor{},
           Sentiment:   0.0,
           Engagement:  0.0,
       }
   } else {
       hashtag.Analytics = &model.HashtagAnalytics{
           HourlyPosts: analytics.HourlyPostCounts,
           DailyPosts:  analytics.DailyPostCounts,
           TopPosters:  convertActorsToModels(analytics.TopPosters),
           Sentiment:   analytics.SentimentScore,
           Engagement:  analytics.EngagementScore,
       }
   }
   ```

4. **Populate relatedHashtags properly**:
   ```go
   // Current: s.getRelatedHashtags returns []string with names
   // Need to convert to real Hashtag models
   relatedHashtags := make([]*model.Hashtag, 0)
   if len(result.RelatedTags) > 0 {
       for _, tagName := range result.RelatedTags {
           relatedResult, err := s.GetHashtag(ctx, &hashtags.GetHashtagQuery{
               Name:     tagName,
               ViewerID: viewerID,
           })
           if err == nil {
               // Recursive call to convertHashtagToModel, but limit depth to avoid infinite loops
               related := convertHashtagToModelLimited(ctx, relatedResult, viewerID, depth+1)
               if related != nil {
                   relatedHashtags = append(relatedHashtags, related)
               }
           }
       }
   }
   hashtag.RelatedHashtags = relatedHashtags
   ```

**Reference Pattern**: Study `convertNoteToObject()` (lines 3192-3300) - it fully populates all fields from data sources, handles nested conversions, and gracefully handles missing data.

**Acceptance Criteria**:
- [ ] All non-null fields populated (never nil)
- [ ] No synthetic/hardcoded data except when fetch fails
- [ ] Posts connection contains actual posts
- [ ] RelatedHashtags are real Hashtag objects (not stubs)
- [ ] NotificationSettings come from database, not defaults
- [ ] Schema validation passes (no GraphQL errors on required fields)

---

### Issue 2: Mutation Payloads Don't Use Shared Converter

**Location**: `graph/schema.resolvers.go`
- `FollowHashtag()` mutation (lines ~8538-8582)
- `UnfollowHashtag()` mutation (lines ~8585-8628)
- `UpdateHashtagNotifications()` mutation (lines ~8628-8687)
- `MuteHashtag()` mutation (lines ~8687-8750)

**Problem**: 
These mutations create `model.Hashtag` inline with only 4-5 fields instead of using `convertHashtagToModel()`:

```go
// Current (WRONG)
return &model.HashtagFollowPayload{
    Success: true,
    Hashtag: &model.Hashtag{
        Name:        hashtag,
        DisplayName: "#" + hashtag,
        URL:         fmt.Sprintf("https://%s/tags/%s", config.Get().Domain, hashtag),
        IsFollowing: true,
    },
}, nil
```

This leaves all other fields nil/zero, violating the schema contract and being inconsistent with other mutations.

**Required Fix**:

In each mutation, after the operation succeeds, fetch the complete hashtag data and use converter:

```go
// FollowHashtag mutation - CORRECT PATTERN
func (r *mutationResolver) FollowHashtag(ctx context.Context, hashtag string, notifyLevel *model.NotificationLevel) (*model.HashtagFollowPayload, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }

    // ... perform follow operation ...
    hashtagService := r.Registry.Hashtags()
    _, err = hashtagService.FollowHashtag(ctx, &hashtags.FollowHashtagCommand{
        UserID:               username,
        Hashtag:              hashtag,
        NotificationsEnabled: notificationsEnabled,
    })
    if err != nil {
        r.Logger.Error("failed to follow hashtag", /* ... */)
        return nil, err
    }

    // AFTER successful follow, fetch complete hashtag to return
    result, err := hashtagService.GetHashtag(ctx, &hashtags.GetHashtagQuery{
        Name:     hashtag,
        ViewerID: username,
    })
    if err != nil {
        // If fetch fails, still return success but with minimal data
        r.Logger.Warn("failed to fetch hashtag after follow", zap.Error(err))
        return &model.HashtagFollowPayload{
            Success: true,
            Hashtag: &model.Hashtag{
                Name:        hashtag,
                DisplayName: "#" + hashtag,
                URL:         fmt.Sprintf("https://%s/tags/%s", config.Get().Domain, hashtag),
                IsFollowing: true,
            },
        }, nil
    }

    // Use shared converter for full population
    fullHashtag := r.convertHashtagToModel(ctx, result, username)
    
    return &model.HashtagFollowPayload{
        Success: true,
        Hashtag: fullHashtag,
    }, nil
}
```

**Apply same pattern to**:
- `UnfollowHashtag()` - fetch after unfollow, use converter
- `UpdateHashtagNotifications()` - fetch after update, use converter  
- `MuteHashtag()` - fetch after mute, use converter

**Reference Pattern**: Study other mutation resolvers that use `convertXToModel()` helpers - e.g., CreateNote, UpdateList, etc.

**Acceptance Criteria**:
- [ ] All mutations call `convertHashtagToModel()` for payload
- [ ] Payload contains all required fields (same as Query responses)
- [ ] Consistent data between Query and Mutation responses
- [ ] Graceful degradation if post-op fetch fails

---

### Issue 3: Service.GetHashtagActivity Still a Broken Placeholder

**Location**: `pkg/services/hashtags/service.go` (lines ~773-793)

**Problem**:
```go
func (s *Service) GetHashtagActivity(_ context.Context, hashtags []string) (<-chan *streaming.Event, error) {
    if s.publisher == nil {
        return nil, ErrPublisherNotAvailable
    }
    activityChan := make(chan *streaming.Event, 100)
    // Subscribe to hashtag streams (no producer wired)
    return activityChan, nil  // ← RETURNS CHANNEL THAT NEVER RECEIVES EVENTS
}
```

The channel is created but never connected to any event producer. Subscribers will block forever waiting for events.

**Required Fix**:

Wire the channel to the actual event stream:

```go
func (s *Service) GetHashtagActivity(ctx context.Context, hashtags []string) (<-chan *streaming.Event, error) {
    if s.publisher == nil {
        return nil, ErrPublisherNotAvailable
    }
    
    activityChan := make(chan *streaming.Event, 100)
    
    // Start background goroutine that listens to actual events
    go func() {
        defer close(activityChan)
        
        // Subscribe to event stream (using existing streaming infrastructure)
        eventFilter := &streaming.EventFilter{
            Types: []streaming.EventType{
                streaming.EventTypeCreate,  // New posts
                streaming.EventTypeUpdate,  // Updated posts
            },
            Streams: nil,  // Will be set per hashtag below
        }
        
        // Normalize hashtags
        normalizedTags := make([]string, len(hashtags))
        for i, tag := range hashtags {
            normalizedTags[i] = strings.ToLower(strings.TrimPrefix(tag, "#"))
        }
        
        // Build stream names from hashtags
        for _, tag := range normalizedTags {
            eventFilter.Streams = append(eventFilter.Streams, fmt.Sprintf("hashtag:%s", tag))
        }
        
        // Subscribe to event bus
        subscriber, err := streaming.GetGlobalEventBus(nil).Subscribe(
            fmt.Sprintf("hashtag_activity_%d", time.Now().UnixNano()),
            eventFilter,
            100,
        )
        if err != nil {
            s.logger.Error("failed to subscribe to hashtag activity", zap.Error(err))
            return
        }
        defer subscriber.Close()
        
        // Forward events from subscriber to channel
        for {
            select {
            case event := <-subscriber.Channel:
                if event == nil {
                    return
                }
                select {
                case activityChan <- event:
                case <-ctx.Done():
                    return
                }
            case <-subscriber.Quit:
                return
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return activityChan, nil
}
```

**Reference Pattern**: Study `NotificationStream` subscription handler (lines 1910-2006) - it shows the complete pattern for wiring events.

**Acceptance Criteria**:
- [ ] Channel receives events (not blocking forever)
- [ ] Events are filtered by requested hashtags
- [ ] Goroutine properly cleans up on context close
- [ ] Events match streaming.Event structure

---

### Issue 4: Subscription Bypasses SubscriptionManager

**Location**: `graph/schema.resolvers.go` - `HashtagActivity()` subscription (lines ~11328-11406)

**Problem**:
```go
// Current (WRONG) - bypasses SubscriptionManager
subscriber, err := internalEventBus.Subscribe(...)
go func() {
    defer func() {
        close(activityChan)
        if subscriber != nil {
            subscriber.Close()
        }
    }()
    // ... forward events ...
}()
```

This direct event bus usage:
1. Misses standard lifecycle management
2. Skips error handling patterns
3. Doesn't use shared infrastructure
4. Inconsistent with other subscriptions

**Required Fix**:

Refactor to use `SubscriptionManager` pattern like `NotificationStream()`:

```go
// HashtagActivity subscription resolver
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }

    ch := make(chan *model.HashtagActivityUpdate, 100)
    r.Logger.Info("Hashtag activity subscription started",
        zap.String("user", username),
        zap.Strings("hashtags", hashtags))

    // Get event bus (with validation, like NotificationStream does)
    internalEventBus, err := r.getEventBusForHashtagActivity()
    if err != nil {
        close(ch)
        return ch, err
    }

    // Create filter
    filter := r.createHashtagActivityFilter(username, hashtags)

    // Subscribe using standard pattern
    subscriber, err := r.subscribeToHashtagActivityEvents(internalEventBus, username, filter)
    if err != nil {
        close(ch)
        return ch, err
    }

    // Start forwarding
    r.startHashtagActivityEventForwarding(ctx, ch, subscriber, username, hashtags)
    return ch, nil
}

// Helper: Get event bus with validation
func (r *subscriptionResolver) getEventBusForHashtagActivity() (*streaming.EventBus, error) {
    internalEventBus := streaming.GetGlobalEventBus(r.Logger)
    if internalEventBus == nil || !internalEventBus.IsRunning() {
        r.Logger.Error("Internal EventBus not available for HashtagActivity")
        return nil, ErrInternalEventBusUnavailable
    }
    return internalEventBus, nil
}

// Helper: Create filter
func (r *subscriptionResolver) createHashtagActivityFilter(username string, hashtags []string) *streaming.EventFilter {
    streams := make([]string, len(hashtags))
    for i, tag := range hashtags {
        normalized := strings.ToLower(strings.TrimPrefix(tag, "#"))
        streams[i] = fmt.Sprintf("hashtag:%s", normalized)
    }
    
    return &streaming.EventFilter{
        Types: []streaming.EventType{
            streaming.EventTypeCreate,
            streaming.EventTypeUpdate,
        },
        Streams: streams,
        UserID:  username,
    }
}

// Helper: Subscribe
func (r *subscriptionResolver) subscribeToHashtagActivityEvents(eventBus *streaming.EventBus, username string, filter *streaming.EventFilter) (*streaming.Subscriber, error) {
    subscriber, err := eventBus.Subscribe(
        fmt.Sprintf("hashtag_activity_%s_%d", username, time.Now().UnixNano()),
        filter,
        100,
    )
    if err != nil {
        r.Logger.Error("Failed to subscribe to hashtag activity", zap.Error(err))
        return nil, errors.Join(errors.New("failed to subscribe"), err)
    }
    return subscriber, nil
}

// Helper: Start forwarding events
func (r *subscriptionResolver) startHashtagActivityEventForwarding(ctx context.Context, ch chan *model.HashtagActivityUpdate, subscriber *streaming.Subscriber, username string, hashtags []string) {
    go func() {
        defer func() {
            close(ch)
            if subscriber != nil {
                subscriber.Close()
            }
        }()

        for {
            select {
            case event := <-subscriber.Channel:
                if event == nil {
                    return
                }

                // Convert event to HashtagActivityUpdate
                update := r.convertEventToHashtagActivityUpdate(ctx, event, username, hashtags)
                if update != nil {
                    r.sendHashtagActivityUpdate(ctx, ch, update)
                }

            case <-subscriber.Quit:
                return
            case <-ctx.Done():
                return
            }
        }
    }()
}

// Helper: Convert event to GraphQL model
func (r *subscriptionResolver) convertEventToHashtagActivityUpdate(ctx context.Context, event *streaming.Event, username string, hashtags []string) *model.HashtagActivityUpdate {
    if event == nil || event.Data == nil {
        return nil
    }

    // Extract post/note from event
    note, ok := event.Data.(*activitypub.Note)
    if !ok {
        r.Logger.Debug("event data is not a note", zap.String("eventType", event.Type.String()))
        return nil
    }

    post := r.convertNoteToObject(ctx, note)
    if post == nil {
        return nil
    }

    // Extract which hashtags matched
    matchedHashtags := r.extractMatchedHashtags(post, hashtags)

    return &model.HashtagActivityUpdate{
        Type:      event.Type.String(),
        Post:      post,
        Hashtags:  matchedHashtags,
        Timestamp: time.Now(),
    }
}

// Helper: Send update with context awareness
func (r *subscriptionResolver) sendHashtagActivityUpdate(ctx context.Context, ch chan *model.HashtagActivityUpdate, update *model.HashtagActivityUpdate) {
    select {
    case ch <- update:
    case <-ctx.Done():
        return
    }
}

// Helper: Extract matched hashtags from post
func (r *subscriptionResolver) extractMatchedHashtags(post *model.Object, requestedHashtags []string) []string {
    matched := make([]string, 0)
    contentLower := strings.ToLower(post.Content)
    
    for _, req := range requestedHashtags {
        normalized := strings.ToLower(strings.TrimPrefix(req, "#"))
        if strings.Contains(contentLower, "#"+normalized) {
            matched = append(matched, normalized)
        }
    }
    
    return matched
}
```

**Reference Pattern**: Study `NotificationStream()` (lines 1910-2006) and its helper methods (1939-2020) - this IS the correct pattern. Copy the structure exactly.

**Acceptance Criteria**:
- [ ] Uses `SubscriptionManager` pattern
- [ ] Has separate helper methods (getEventBus, createFilter, subscribe, forward)
- [ ] Proper error handling and logging
- [ ] Context-aware cleanup
- [ ] Events actually flow to subscribers

---

## 🔧 Implementation Order

1. **Fix Issue 1** (Converter): 90 mins
   - Required for all downstream fixes
   - Unlocks proper data population

2. **Fix Issue 2** (Mutations): 30 mins
   - Depends on Issue 1 fix
   - Simple pattern application

3. **Fix Issue 3** (Service method): 45 mins
   - Independent
   - Enables real event production

4. **Fix Issue 4** (Subscription): 60 mins
   - Depends on Issue 3
   - Replaces current broken logic

**Total Estimated Time**: 3.5-4 hours

---

## ✅ Validation Checklist

After fixes complete:

### Schema Compliance
- [ ] Query.hashtag response has all required fields populated (non-null)
- [ ] Query.followedHashtags has full Hashtag objects
- [ ] Query.hashtagTimeline has PostConnection with posts
- [ ] Query.suggestedHashtags has full Hashtag objects
- [ ] Mutation.followHashtag returns full Hashtag
- [ ] Mutation.unfollowHashtag returns full Hashtag
- [ ] All other mutations return full Hashtag
- [ ] No nil/zero values for non-null fields

### Service Compliance
- [ ] GetHashtagActivity produces events (test with subscriber)
- [ ] Related hashtags populated with real data
- [ ] Suggestions contain meaningful rankings

### Subscription Compliance
- [ ] HashtagActivity uses SubscriptionManager
- [ ] Helpers broken out and testable
- [ ] Follows NotificationStream pattern exactly
- [ ] Events flow to subscribers
- [ ] Proper cleanup on disconnect

### Test Compliance
- [ ] Existing tests still pass
- [ ] New converter tests added
- [ ] Mutation response tests added
- [ ] Subscription integration tests added

---

## 📊 Success Criteria

When complete, Phase 1.1 is **production-ready** when:
1. ✅ All GraphQL resolvers return fully populated models per schema
2. ✅ All required fields are non-nil (or properly handled with fallbacks)
3. ✅ Service methods have real implementations (not placeholders)
4. ✅ Subscriptions use shared infrastructure pattern
5. ✅ All audit findings resolved
6. ✅ Tests pass
7. ✅ Code review approval
