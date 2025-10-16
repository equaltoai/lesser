# Phase 1.1 Final Fixes - 4 Remaining Issues

**Status**: Phase 1.1 is 80% complete. 4 blocking issues prevent production readiness.

---

## Issue #1: FollowHashtag/UnfollowHashtag Mutations Return Incomplete Models

**Location**: `graph/schema.resolvers.go` lines 8655-8698 and 8585-8628

**Problem**: Mutations hand-roll minimal `model.Hashtag` instead of using the converter. Payloads are inconsistent with Query responses.

```go
// CURRENT (WRONG)
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

**Fix**: After mutation succeeds, fetch complete data and use converter (3-line change):

```go
// After successful follow/unfollow operation...

// Fetch complete hashtag using service
result, err := hashtagService.GetHashtag(ctx, &hashtags.GetHashtagQuery{
    Name:     hashtag,
    ViewerID: username,
})
if err != nil {
    r.Logger.Error("failed to fetch hashtag after mutation", zap.Error(err))
    // Return error - don't return incomplete data
    return nil, err
}

// Use converter for full population
fullHashtag := r.convertHashtagToModel(ctx, result, username)

return &model.HashtagFollowPayload{
    Success: true,
    Hashtag: fullHashtag,  // Now contains all required fields
}, nil
```

**Apply to ALL mutation resolvers**:
- `FollowHashtag()` - line ~8538
- `UnfollowHashtag()` - line ~8585
- `UpdateHashtagNotifications()` - line ~8628
- `MuteHashtag()` - line ~8687

---

## Issue #2: Notification Settings Are Hardcoded Defaults, Not Real Data

**Location**: `graph/schema.resolvers.go` - `fetchHashtagNotificationSettings()` function (lines 3723-3766)

**Problem**: Function only checks if hashtag is followed, then returns hardcoded `NotificationLevelAll` and `Muted=false`. It doesn't read actual stored preferences or mute duration.

```go
// CURRENT (WRONG)
// Build notification settings
now := model.Time(time.Now())
return &model.HashtagNotificationSettings{
    Level:   model.NotificationLevelAll,  // ← HARDCODED
    Muted:   false,                       // ← HARDCODED
    Filters: []*model.NotificationFilter{},
}, &now
```

**Root Cause**: HashtagRepository doesn't have methods to fetch:
- Actual notification level (ALL, MUTUALS, FOLLOWING, NONE)
- Mute status and duration
- Notification filters

**Fix Strategy**: Update repository with new methods, then use them:

```go
// Step 1: Add to HashtagRepository (pkg/storage/repositories/hashtag_repository.go)
// Add a new method to fetch full notification settings:

func (r *HashtagRepository) GetHashtagNotificationSettings(ctx context.Context, userID, hashtag string) (*storage.HashtagNotificationSettings, error) {
    // Query storage for:
    // - notification level (stored in follow record)
    // - muted status and duration (stored in mute record)
    // - filters (stored in notification filters table)
    // Return complete object or nil if not found
}

// Step 2: Use in fetchHashtagNotificationSettings():

func (r *Resolver) fetchHashtagNotificationSettings(ctx context.Context, hashtagName string, userID string) (*model.HashtagNotificationSettings, *model.Time) {
    hashtagRepo := r.Storage.Hashtag()
    if hashtagRepo == nil {
        return nil, nil
    }

    // Fetch real settings from storage
    settings, err := hashtagRepo.GetHashtagNotificationSettings(ctx, userID, hashtagName)
    if err != nil || settings == nil {
        // Hashtag not followed, return nil
        return nil, nil
    }

    // Convert storage model to GraphQL model
    now := model.Time(time.Now())
    filters := make([]*model.NotificationFilter, len(settings.Filters))
    for i, f := range settings.Filters {
        filters[i] = &model.NotificationFilter{
            Type:  f.Type,
            Value: f.Value,
        }
    }

    return &model.HashtagNotificationSettings{
        Level:      parseNotificationLevel(settings.Level),  // Use actual level
        Muted:      settings.Muted,                          // Use actual mute status
        MutedUntil: convertTimePointer(settings.MutedUntil), // Use actual mute duration
        Filters:    filters,
    }, &now
}

// Helper to convert storage level to GraphQL level
func parseNotificationLevel(storageLevel string) model.NotificationLevel {
    switch storageLevel {
    case "ALL":
        return model.NotificationLevelAll
    case "MUTUALS":
        return model.NotificationLevelMutuals
    case "FOLLOWING":
        return model.NotificationLevelFollowing
    case "NONE":
        return model.NotificationLevelNone
    default:
        return model.NotificationLevelAll
    }
}
```

**Acceptance Criteria**:
- [ ] HashtagRepository has `GetHashtagNotificationSettings()` method
- [ ] Method returns actual stored level (not hardcoded)
- [ ] Method returns actual mute status and duration
- [ ] fetchHashtagNotificationSettings uses repository method
- [ ] No hardcoded defaults except when fetch fails

---

## Issue #3: Subscription Bypasses Shared Infrastructure

**Location**: `graph/schema.resolvers.go` - `HashtagActivity()` subscription (lines ~11549-11636)

**Problem**: Direct event bus subscription instead of using `GraphQLSubscriptionManager`. This duplicates logic that already exists in the manager.

**Critical Discovery**: `GraphQLSubscriptionManager` already has subscription infrastructure! Study:
- File: `graph/subscription_manager.go` (504 lines)
- File: `graph/subscription_factory.go` (factory pattern)
- File: `graph/subscription_handlers.go` (event processors)

The correct pattern is to add a new method to `GraphQLSubscriptionManager`:

```go
// In graph/subscription_manager.go, add new method:

func (sm *GraphQLSubscriptionManager) SubscribeToHashtagActivity(
    ctx context.Context,
    userID string,
    hashtags []string,
) (<-chan *model.HashtagActivityUpdate, error) {
    // 1. Validate inputs
    if userID == "" || len(hashtags) == 0 {
        return nil, errors.New("userID and hashtags required")
    }

    // 2. Create output channel
    ch := make(chan *model.HashtagActivityUpdate, 100)

    // 3. Build event filter
    streams := make([]string, len(hashtags))
    for i, tag := range hashtags {
        normalized := strings.ToLower(strings.TrimPrefix(tag, "#"))
        streams[i] = fmt.Sprintf("hashtag:%s", normalized)
    }

    filter := &streaming.EventFilter{
        Types: []streaming.EventType{
            streaming.EventTypeCreate,
            streaming.EventTypeUpdate,
        },
        Streams: streams,
        UserID:  userID,
    }

    // 4. Create config for generic subscription
    config := &SubscriptionConfig{
        ID:            fmt.Sprintf("hashtag_activity_%s_%d", userID, time.Now().UnixNano()),
        Type:          "hashtag_activity",
        UserID:        userID,
        Filter:        filter,
        OutputChannel: ch,
        BufferSize:    100,
        Params: map[string]interface{}{
            "hashtags": hashtags,
        },
    }

    // 5. Create event processor
    processor := func(sub *GraphQLSubscription, outputChannel interface{}) {
        ch := outputChannel.(chan *model.HashtagActivityUpdate)
        defer close(ch)

        for {
            select {
            case event := <-sub.Subscriber.Channel:
                if event == nil {
                    return
                }

                // Convert event to HashtagActivityUpdate
                update := sm.convertEventToHashtagActivityUpdate(event, hashtags)
                if update != nil {
                    select {
                    case ch <- update:
                        sub.LastActivity = time.Now()
                    case <-sub.Context.Done():
                        return
                    }
                }

            case <-sub.Subscriber.Quit:
                return
            case <-sub.Context.Done():
                return
            }
        }
    }

    // 6. Create subscription using manager's infrastructure
    if err := sm.createGenericEventBusSubscription(ctx, config, processor); err != nil {
        close(ch)
        return nil, err
    }

    return ch, nil
}

// Helper to convert event to GraphQL model
func (sm *GraphQLSubscriptionManager) convertEventToHashtagActivityUpdate(
    event *streaming.Event,
    requestedHashtags []string,
) *model.HashtagActivityUpdate {
    if event == nil {
        return nil
    }

    // Extract post/note from event
    note, ok := event.Data.(*activitypub.Note)
    if !ok {
        sm.logger.Debug("event data is not a note", zap.String("eventType", event.Type.String()))
        return nil
    }

    // Find which hashtags matched
    matchedHashtags := extractMatchedHashtags(note.Content, requestedHashtags)
    if len(matchedHashtags) == 0 {
        return nil
    }

    // Convert note to Object (reuse existing converter)
    post := &model.Object{
        ID:      note.ID,
        Content: note.Content,
        // ... populate other fields
    }

    return &model.HashtagActivityUpdate{
        Type:      event.Type.String(),
        Post:      post,
        Hashtags:  matchedHashtags,
        Timestamp: time.Now(),
    }
}

// Helper function
func extractMatchedHashtags(content string, requested []string) []string {
    matched := make([]string, 0)
    contentLower := strings.ToLower(content)
    for _, req := range requested {
        normalized := strings.ToLower(strings.TrimPrefix(req, "#"))
        if strings.Contains(contentLower, "#"+normalized) {
            matched = append(matched, normalized)
        }
    }
    return matched
}
```

Then in resolver, call the manager method:

```go
// In graph/schema.resolvers.go - HashtagActivity subscription resolver

func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }

    // Use the manager's method - simple!
    return r.subscriptionManager.SubscribeToHashtagActivity(ctx, username, hashtags)
}
```

**Acceptance Criteria**:
- [ ] `GraphQLSubscriptionManager` has `SubscribeToHashtagActivity()` method
- [ ] Resolver calls manager method (not direct event bus)
- [ ] Follows exact pattern of other subscription methods
- [ ] Proper lifecycle management via manager
- [ ] Cleanup via manager infrastructure

---

## Issue #4: Service Events Have Empty Payloads

**Location**: `pkg/services/hashtags/service.go` lines 773-857

**Problem**: Events are created with `Payload: nil`, so downstream converters can't access post data, author info, etc.

```go
// CURRENT (WRONG)
event := &streaming.Event{
    Type:      string(internalEvent.Type),
    Stream:    "",
    Payload:   nil,  // ← EMPTY - DOWNSTREAM CONVERTERS CAN'T USE THIS
    Timestamp: internalEvent.Timestamp,
}
```

**Fix**: Populate Payload with actual event data:

```go
// In GetHashtagActivity service method

event := &streaming.Event{
    Type:      string(internalEvent.Type),
    Stream:    fmt.Sprintf("hashtag:%s", tagName),  // Add stream name
    Payload:   internalEvent.Object,                // ← INCLUDE ACTUAL DATA
    Timestamp: internalEvent.Timestamp,
    Data:      internalEvent.Object,                // May need both fields depending on streaming.Event struct
}
```

Check what fields `streaming.Event` has and populate appropriately:
- `Type` - event type (CREATE, UPDATE, DELETE)
- `Payload` or `Data` - the actual note/object
- `Stream` - stream identifier for routing
- `Timestamp` - when event occurred
- Any other fields required by subscribers

**Acceptance Criteria**:
- [ ] Events include complete post/note object
- [ ] Downstream converters can access content, author, etc.
- [ ] No nil Payload fields
- [ ] Events match streaming.Event structure

---

## Summary of All 4 Issues

| Issue | Root Cause | Impact | Fix Complexity |
|-------|-----------|--------|-----------------|
| #1 | Mutations don't use converter | Inconsistent responses | Low (3-line pattern) |
| #2 | Notification settings hardcoded | Wrong user data | Medium (need repo method) |
| #3 | Subscription bypasses manager | Doesn't use shared infra | High (add manager method) |
| #4 | Events have empty payloads | Converters fail | Low (populate fields) |

---

## Implementation Order (Dependencies)

1. **Fix #2 first** (Notification Settings - 1.5 hours)
   - Add repository method
   - Update resolver function
   - No other dependencies

2. **Fix #1** (Mutation Payloads - 30 mins)
   - Depends on: Nothing
   - Applied to 4 mutations
   - Simple pattern replication

3. **Fix #4** (Event Payloads - 30 mins)
   - Depends on: Nothing
   - Simple field population
   - May need to check streaming.Event struct

4. **Fix #3** (Subscription Manager - 2 hours)
   - Depends on: #4 (need good events)
   - Most complex
   - New manager method + resolver integration

**Total: ~4.5 hours**

---

## Validation After Fixes

```
MUST PASS:
- [ ] Query.hashtag returns full model (non-null fields)
- [ ] Query.followedHashtags returns full hashtags
- [ ] Mutation.followHashtag returns full hashtag (same fields as Query)
- [ ] Mutation.unfollowHashtag returns full hashtag
- [ ] Mutation.updateHashtagNotifications returns full hashtag
- [ ] Mutation.muteHashtag returns full hashtag
- [ ] All mutations consistent with queries
- [ ] NotificationSettings has real level (not ALL)
- [ ] NotificationSettings has real mute status
- [ ] NotificationSettings has real mutedUntil duration
- [ ] Subscription.hashtagActivity uses SubscriptionManager
- [ ] Subscription events flow to subscribers
- [ ] No hardcoded/synthetic data except on error fallback
- [ ] All tests pass
- [ ] No regressions
```

---

## Success Criteria

Phase 1.1 is **PRODUCTION READY** when:

✅ **Schema Contract Honored**
- All required fields populated per schema
- No null/zero values for non-null fields
- Consistent responses across Query/Mutation

✅ **Real Data Used**
- Notification settings from database
- Mute status from database
- Event payloads contain full data
- No hardcoded/synthetic data

✅ **Shared Patterns**
- Subscriptions use SubscriptionManager
- Resolvers use converters
- Mutations use same conversion as queries

✅ **Audit Passes**
- No remaining blocking issues
- Code review approval
- All criteria met
