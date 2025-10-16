# Phase 1.1.1 Blocker: Hashtag Subscription Harmonization

**Status**: Blocker for Phase 1.1 completion  
**Dependency**: Phase 1.1 Issues #1, #2, #4 must be fixed first  
**Duration**: 4-6 hours  
**Objective**: Align hashtag activity subscription with shared `GraphQLSubscriptionManager` infrastructure

---

## 🎯 Mission

Phase 1.1 currently implements hashtag subscriptions with **duplicate infrastructure** that bypasses the shared `GraphQLSubscriptionManager`. This creates:
- ❌ Maintenance burden (two subscription paths)
- ❌ Inconsistent error handling
- ❌ No lifecycle management standardization
- ❌ Poor pattern for Phase 2/3 subscriptions

**Phase 1.1.1 Goal**: Eliminate the duplicate path. Use the shared manager that already handles `timeline`, `notifications`, `cost`, and `moderation` subscriptions.

**Success**: `Subscription.hashtagActivity` uses the same infrastructure as all other subscriptions.

---

## 📊 Current State (Before Fix)

### GraphQL Resolver (Wrong Pattern)
```go
// Location: graph/schema.resolvers.go, line ~11549
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }

    ch := make(chan *model.HashtagActivityUpdate, 100)
    
    // ❌ DIRECTLY SUBSCRIBES TO EVENT BUS (WRONG)
    internalEventBus, err := r.getEventBusForHashtagActivity()
    
    filter := r.createHashtagActivityFilter(username, hashtags)
    subscriber, err := r.subscribeToHashtagActivityEvents(internalEventBus, username, filter)
    
    // ❌ CUSTOM EVENT FORWARDING (WRONG)
    r.startHashtagActivityEventForwarding(ctx, ch, subscriber, username, hashtags)
    
    return ch, nil
}

// ❌ DUPLICATE HELPER FUNCTIONS (SHOULD NOT EXIST)
func (r *subscriptionResolver) getEventBusForHashtagActivity() { /* ... */ }
func (r *subscriptionResolver) createHashtagActivityFilter() { /* ... */ }
func (r *subscriptionResolver) subscribeToHashtagActivityEvents() { /* ... */ }
func (r *subscriptionResolver) startHashtagActivityEventForwarding() { /* ... */ }
func (r *subscriptionResolver) convertEventToHashtagActivityUpdate() { /* ... */ }
func (r *subscriptionResolver) sendHashtagActivityUpdate() { /* ... */ }
func (r *subscriptionResolver) extractMatchedHashtags() { /* ... */ }
```

### GraphQLSubscriptionManager (Missing Method)
```go
// Location: graph/subscription_manager.go
// ❌ NO SubscribeToHashtagActivity method exists
// This is what we need to add
```

---

## ✅ Target State (After Fix)

### GraphQL Resolver (Clean)
```go
// Location: graph/schema.resolvers.go, line ~11549
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }

    // ✅ ONE LINE: Delegate to manager
    return r.subscriptionManager.SubscribeToHashtagActivity(ctx, username, hashtags)
}
```

### GraphQLSubscriptionManager (Complete)
```go
// Location: graph/subscription_manager.go
// ✅ NEW METHOD: SubscribeToHashtagActivity
// ✅ Uses existing infrastructure: createGenericEventBusSubscription
// ✅ Follows pattern of SubscribeToTimeline, SubscribeToNotifications, etc.
```

---

## 🔧 Implementation Steps

### Step 1: Add Method to GraphQLSubscriptionManager

**Location**: `graph/subscription_manager.go` (end of file, before `cleanupLoop()`)

**Pattern to follow**: Study existing methods in same file:
- Line ~118: `SubscribeToTimeline()` - GOOD EXAMPLE
- Look at structure, filter building, processor function

**Add this method**:

```go
// SubscribeToHashtagActivity subscribes to hashtag activity updates
func (sm *GraphQLSubscriptionManager) SubscribeToHashtagActivity(
	ctx context.Context,
	userID string,
	hashtags []string,
) (<-chan *model.HashtagActivityUpdate, error) {
	// 1. Validate inputs
	if userID == "" {
		return nil, errors.New("userID required")
	}
	if len(hashtags) == 0 {
		return nil, errors.New("at least one hashtag required")
	}

	// 2. Create output channel
	ch := make(chan *model.HashtagActivityUpdate, 100)

	// 3. Normalize hashtag names and build stream identifiers
	normalizedHashtags := make([]string, len(hashtags))
	streams := make([]string, len(hashtags))
	for i, tag := range hashtags {
		normalized := strings.ToLower(strings.TrimPrefix(tag, "#"))
		normalizedHashtags[i] = normalized
		streams[i] = fmt.Sprintf("hashtag:%s", normalized)
	}

	// 4. Build event filter (matches what service produces)
	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeCreate,
			streaming.EventTypeUpdate,
		},
		Streams: streams,
		UserID:  userID,
	}

	// 5. Create subscription config
	config := &SubscriptionConfig{
		ID:            fmt.Sprintf("hashtag_activity_%s_%d", userID, time.Now().UnixNano()),
		Type:          "hashtag_activity",
		UserID:        userID,
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    100,
		Params: map[string]interface{}{
			"hashtags": normalizedHashtags,
		},
	}

	// 6. Create event processor function
	processor := func(sub *GraphQLSubscription, outputChannel interface{}) {
		ch := outputChannel.(chan *model.HashtagActivityUpdate)
		defer close(ch)

		for {
			select {
			case event := <-sub.Subscriber.Channel:
				if event == nil {
					return
				}

				// Convert event to GraphQL model
				update := sm.convertEventToHashtagActivityUpdate(event, normalizedHashtags)
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

	// 7. Create subscription using manager's standard infrastructure
	if err := sm.createGenericEventBusSubscription(ctx, config, processor); err != nil {
		close(ch)
		return nil, err
	}

	sm.logger.Info("hashtag activity subscription created",
		zap.String("user_id", userID),
		zap.Strings("hashtags", normalizedHashtags))

	return ch, nil
}

// convertEventToHashtagActivityUpdate converts streaming event to GraphQL model
func (sm *GraphQLSubscriptionManager) convertEventToHashtagActivityUpdate(
	event *streaming.Event,
	requestedHashtags []string,
) *model.HashtagActivityUpdate {
	if event == nil {
		return nil
	}

	// Extract note/post from event
	note, ok := event.Data.(*activitypub.Note)
	if !ok {
		sm.logger.Debug("event data is not a note",
			zap.String("eventType", fmt.Sprintf("%v", event.Type)))
		return nil
	}

	// Find which requested hashtags are in the note
	matchedHashtags := sm.extractMatchedHashtags(note.Content, requestedHashtags)
	if len(matchedHashtags) == 0 {
		// Event doesn't match any requested hashtags (shouldn't happen with filter)
		return nil
	}

	// Convert note to Object using existing converter
	// Note: You may need to import converter or pass as dependency
	post := &model.Object{
		ID:      note.ID,
		Content: note.Content,
		// Add other fields as needed - use pattern from existing converters
	}

	return &model.HashtagActivityUpdate{
		Type:      event.Type.String(),
		Post:      post,
		Hashtags:  matchedHashtags,
		Timestamp: time.Now(),
	}
}

// extractMatchedHashtags finds which requested hashtags appear in content
func (sm *GraphQLSubscriptionManager) extractMatchedHashtags(
	content string,
	requestedHashtags []string,
) []string {
	matched := make([]string, 0)
	contentLower := strings.ToLower(content)

	for _, requested := range requestedHashtags {
		normalized := strings.ToLower(strings.TrimPrefix(requested, "#"))
		if strings.Contains(contentLower, "#"+normalized) {
			matched = append(matched, normalized)
		}
	}

	return matched
}
```

**Reference implementation**: This follows the exact pattern of `SubscribeToTimeline()` and `SubscribeToNotifications()` in the same file.

---

### Step 2: Simplify GraphQL Resolver

**Location**: `graph/schema.resolvers.go`, line ~11549

**Before** (~40 lines including helpers):
```go
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }

    ch := make(chan *model.HashtagActivityUpdate, 100)
    r.Logger.Info("Hashtag activity subscription started", /* ... */)

    internalEventBus, err := r.getEventBusForHashtagActivity()
    if err != nil {
        close(ch)
        return ch, err
    }

    filter := r.createHashtagActivityFilter(username, hashtags)
    subscriber, err := r.subscribeToHashtagActivityEvents(internalEventBus, username, filter)
    if err != nil {
        close(ch)
        return ch, err
    }

    r.startHashtagActivityEventForwarding(ctx, ch, subscriber, username, hashtags)
    return ch, nil
}
```

**After** (~5 lines):
```go
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
    username, err := r.requireAuth(ctx)
    if err != nil {
        return nil, err
    }

    return r.subscriptionManager.SubscribeToHashtagActivity(ctx, username, hashtags)
}
```

---

### Step 3: Delete Duplicate Helper Functions

**Location**: `graph/schema.resolvers.go` - find and DELETE these functions entirely:

```go
// DELETE: func (r *subscriptionResolver) getEventBusForHashtagActivity()
// DELETE: func (r *subscriptionResolver) createHashtagActivityFilter()
// DELETE: func (r *subscriptionResolver) subscribeToHashtagActivityEvents()
// DELETE: func (r *subscriptionResolver) startHashtagActivityEventForwarding()
// DELETE: func (r *subscriptionResolver) convertEventToHashtagActivityUpdate()
// DELETE: func (r *subscriptionResolver) sendHashtagActivityUpdate()
// DELETE: func (r *subscriptionResolver) extractMatchedHashtags()
```

**Search pattern**: Look for functions starting with "Hashtag" after the main `HashtagActivity()` resolver. Delete all helpers that were added to support it.

**Verify**: After deletion, `HashtagActivity()` resolver should be the ONLY hashtag-related subscription function.

---

### Step 4: Update Imports in subscription_manager.go (if needed)

Check if `subscription_manager.go` has all necessary imports:

```go
import (
    // ... existing imports ...
    "github.com/equaltoai/lesser/graph/model"
    "github.com/equaltoai/lesser/pkg/activitypub"
    "github.com/equaltoai/lesser/pkg/streaming"
    // Add if missing:
    "strings"
    "fmt"
    "errors"
    "time"
)
```

---

## ✅ Verification Checklist

### Code Changes
- [ ] New `SubscribeToHashtagActivity()` method added to `GraphQLSubscriptionManager`
- [ ] Helper converters added to `GraphQLSubscriptionManager`
- [ ] `HashtagActivity()` resolver simplified to 1-line manager call
- [ ] All duplicate helper functions deleted from `schema.resolvers.go`
- [ ] No duplicate code remains

### Event Payload Contract
- [ ] Service produces `streaming.Event` with populated `Data` field
- [ ] Manager can extract `activitypub.Note` from event
- [ ] Converters can access note content, author, etc.
- [ ] No nil payloads reaching converters

### Subscription Manager Integration
- [ ] Uses `createGenericEventBusSubscription()` (shared infrastructure)
- [ ] Follows error handling pattern of other subscriptions
- [ ] Lifecycle properly managed by manager
- [ ] Cleanup via manager's existing cleanup loop

### Testing
- [ ] Existing subscription tests still pass
- [ ] Hashtag subscription produces events
- [ ] Events reach subscribers
- [ ] Filters work correctly (only matching hashtags sent)
- [ ] Cleanup on disconnect works
- [ ] No memory leaks on subscription close

### Regression
- [ ] `Subscription.timeline` still works
- [ ] `Subscription.notifications` still works
- [ ] `Subscription.cost` still works
- [ ] `Subscription.moderation` still works
- [ ] Subscription manager stats accurate
- [ ] No side effects on other features

---

## 📚 Reference Implementations

### Study These (Exact Patterns to Follow)

1. **Location**: `graph/subscription_manager.go` line ~118
   - **Method**: `SubscribeToTimeline()`
   - **What to learn**: Filter building, processor function structure, error handling

2. **Location**: `graph/subscription_manager.go` line ~200+ (estimate)
   - **Method**: `SubscribeToNotifications()` or `SubscribeToCostAlerts()`
   - **What to learn**: Event conversion pattern, lifecycle management

3. **Location**: `graph/subscription_factory.go` line ~25
   - **Function**: `createGenericEventBusSubscription()`
   - **What to learn**: How the manager creates subscriptions (you'll call this)

4. **Location**: `graph/schema.resolvers.go` line ~1910
   - **Method**: `NotificationStream()` resolver
   - **What to learn**: Resolver should be simple, delegates to manager

---

## 🎓 Key Concepts

### Why Shared Manager Matters

**Current (Wrong)**:
```
Resolver → Direct EventBus → Custom Lifecycle → Issues
```

**Target (Right)**:
```
Resolver → SubscriptionManager → Consistent EventBus → Standard Lifecycle
```

**Benefits**:
- Single point for error handling
- Automatic cleanup (via manager's cleanup loop)
- Consistent buffer sizes
- Monitored metrics
- Easier to debug (one code path)
- Template for Phases 2/3

### Event Flow

```
Service.GetHashtagActivity()
  ↓
  produces streaming.Event with Data=activitypub.Note
  ↓
SubscriptionManager.SubscribeToHashtagActivity()
  ↓
  creates filter, subscribes to event bus
  ↓
  processor receives events
  ↓
  convertEventToHashtagActivityUpdate() converts
  ↓
  sends model.HashtagActivityUpdate to channel
  ↓
Client receives update
```

---

## 🚨 Common Mistakes to Avoid

1. **❌ Don't leave old helper functions**
   - Delete ALL subscription helpers from resolver
   - Manager has the only version now

2. **❌ Don't create new subscription paths**
   - Use `createGenericEventBusSubscription()`
   - Don't wire to event bus directly

3. **❌ Don't forget context handling**
   - Use `sub.Context.Done()` for cancellation
   - Manager provides this

4. **❌ Don't skip event payload validation**
   - Verify service sends complete data
   - Converters need content, author, etc.

5. **❌ Don't forget imports**
   - subscription_manager needs proper imports
   - Check `strings`, `fmt`, `errors`, `time`

---

## 📋 Testing Strategy

### Unit Tests (for new manager method)

```go
// In graph/subscription_manager_test.go or new file

func TestSubscribeToHashtagActivity(t *testing.T) {
    // Test 1: Invalid inputs
    // - userID empty → error
    // - hashtags empty → error
    
    // Test 2: Successful subscription
    // - Returns channel
    // - Channel has buffer of 100
    // - Subscription tracked in manager
    
    // Test 3: Event filtering
    // - Only matching hashtags sent
    // - Unrelated hashtags filtered
    
    // Test 4: Cleanup
    // - Context cancellation closes channel
    // - Subscriber properly cleaned up
}
```

### Integration Tests (full flow)

```go
// Verify with real events
// 1. Start manager
// 2. Subscribe to hashtags
// 3. Publish events with hashtags
// 4. Verify events received
// 5. Verify cleanup on context cancel
```

---

## 🎯 Definition of Done

Phase 1.1.1 is complete when:

✅ **Code Quality**
- [ ] No duplicate subscription code
- [ ] Resolver is 1-line call to manager
- [ ] Manager method follows existing patterns
- [ ] All imports correct
- [ ] No lint errors

✅ **Functional**
- [ ] Subscriptions work (events received)
- [ ] Filters work (correct hashtags sent)
- [ ] Cleanup works (no memory leaks)
- [ ] Error handling works

✅ **Testing**
- [ ] All tests pass
- [ ] No regressions on other subscriptions
- [ ] New manager method tested
- [ ] Events validated

✅ **Architecture**
- [ ] Uses shared infrastructure
- [ ] Follows manager pattern exactly
- [ ] Could be template for Phase 2/3
- [ ] Audit would approve

---

## 🚀 Execution Plan

### Timeline: ~4-6 hours

1. **Hour 1**: Study existing subscription patterns (SubscribeToTimeline, etc.)
2. **Hour 1-2**: Implement `SubscribeToHashtagActivity()` in manager
3. **Hour 1**: Add helper functions to manager
4. **Hour 0.5**: Update resolver to 1-line call
5. **Hour 0.5**: Delete duplicate functions from resolver
6. **Hour 0.5**: Fix imports
7. **Hour 1**: Test and validate
8. **Hour 0.5**: Final verification

### Checkpoint: After Step 4, compile check
- Code should compile
- No syntax errors
- Imports resolved

### Checkpoint: After Step 6, functional test
- Subscriptions work
- Events flow
- No obvious issues

### Final: Full regression pass
- All tests pass
- Audit checklist complete

---

## 📞 When You're Done

Report:
1. Files modified
2. Lines added/removed
3. Test results (pass/fail counts)
4. Any blockers encountered
5. Ready for Phase 1.1 final audit

Once approved, Phase 1.1 is complete and we move to Phase 1.2.
