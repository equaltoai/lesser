# Team 2 Mutations Quick Reference

## 🎯 Mutation Implementation Order

### Phase 1: Content Creation (Start Here!)
```graphql
mutation CreateNote($input: CreateNoteInput!) {
  createNote(input: $input) {
    activity { id type }
    status { id content createdAt }
  }
}
```

### Phase 2: Social Interactions
```graphql
mutation LikePost($id: ID!) {
  likeObject(id: $id) { id type actor }
}

mutation FollowUser($id: ID!) {
  followActor(id: $id) { id type actor }
}
```

### Phase 3: Content Management
```graphql
mutation DeletePost($id: ID!) {
  deleteObject(id: $id)
}
```

## 🔧 Mutation Pattern Template

```go
func (r *mutationResolver) MutationName(ctx context.Context, input InputType) (*PayloadType, error) {
    // 1. Authenticate (most mutations require auth)
    username := getUsernameFromContext(ctx)
    if username == "" {
        return nil, errors.New("authentication required")
    }
    
    // 2. Validate input
    if err := validateInput(input); err != nil {
        return nil, fmt.Errorf("invalid input: %w", err)
    }
    
    // 3. Check permissions
    if !canUserPerformAction(ctx, username, input) {
        return nil, errors.New("insufficient permissions")
    }
    
    // 4. Track costs (writes are expensive!)
    r.CostTracker.TrackDynamoWrite(1)
    
    // 5. Build ActivityPub object/activity
    activity := buildActivity(username, input)
    
    // 6. Store in database
    if err := r.Storage.CreateActivity(ctx, activity); err != nil {
        r.Logger.Error("storage failed", zap.Error(err))
        return nil, fmt.Errorf("failed to store: %w", err)
    }
    
    // 7. Queue for federation (async, don't block)
    go func() {
        if err := r.FederationQueue.Send(ctx, activity); err != nil {
            r.Logger.Error("federation failed", zap.Error(err))
        }
    }()
    
    // 8. Return GraphQL payload
    return &PayloadType{
        Success: true,
        Result: convertToGraphQL(activity),
    }, nil
}
```

## 📋 ActivityPub Activity Types

### Create Note
```go
activity := &activitypub.Activity{
    Type:      "Create",
    Actor:     actorURL,
    Object: &activitypub.Note{
        Type:         "Note",
        Content:      input.Content,
        AttributedTo: actorURL,
        To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
        Published:    time.Now(),
    },
}
```

### Like Object
```go
activity := &activitypub.Activity{
    Type:   "Like",
    Actor:  actorURL,
    Object: objectURL,
}
```

### Follow Actor
```go
activity := &activitypub.Activity{
    Type:   "Follow",
    Actor:  actorURL,
    Object: targetActorURL,
}
```

## ⚠️ Important Considerations

### 1. Idempotency
```go
// Check if action already performed
existing, _ := r.Storage.GetActivity(ctx, activityID)
if existing != nil {
    return convertToGraphQL(existing), nil // Don't duplicate
}
```

### 2. Visibility & Privacy
```go
func determineAudience(visibility string) []string {
    switch visibility {
    case "public":
        return []string{"https://www.w3.org/ns/activitystreams#Public"}
    case "followers":
        return []string{actorURL + "/followers"}
    case "direct":
        return mentionedActors
    }
}
```

### 3. Media Attachments
```go
// Use Team 1's media processor
if len(input.MediaIDs) > 0 {
    attachments := make([]activitypub.Attachment, len(input.MediaIDs))
    for i, mediaID := range input.MediaIDs {
        media, _ := r.Storage.GetMedia(ctx, mediaID)
        attachments[i] = convertToAttachment(media)
    }
    note.Attachment = attachments
}
```

### 4. Federation Queue
```go
// Don't let federation failures block the mutation
if err := r.FederationQueue.Send(ctx, activity); err != nil {
    r.Logger.Error("federation failed but continuing", 
        zap.Error(err),
        zap.String("activity", activity.ID))
    // Activity is stored locally, federation will retry
}
```

## 🧪 Testing Checklist

For each mutation:
- [ ] Authentication required (except public actions)
- [ ] Input validation comprehensive
- [ ] ActivityPub format correct
- [ ] Storage transaction successful
- [ ] Federation queued (verify with logs)
- [ ] Cost tracking accurate
- [ ] Error handling graceful
- [ ] Idempotency handled

## 🚀 Quick Start

1. Copy the pattern template
2. Replace with your mutation specifics
3. Use ActivityPub types from examples
4. Test with GraphQL playground
5. Verify federation with test instance

## 📚 Resources

- ActivityPub Spec: https://www.w3.org/TR/activitypub/
- Storage Methods: `pkg/storage/interface.go`
- Federation Queue: `pkg/federation/queue.go`
- Cost Tracking: `pkg/cost/tracker.go`
- GraphQL Schema: `graph/schema.graphql`

---

Remember: Mutations change state and federate to other instances. Always validate, track costs, and handle errors gracefully! 