# GraphQL Resolver Implementation Guide

## Overview

This guide provides detailed implementation examples for each GraphQL resolver in Lesser, showing how to transform the stubbed implementations into production-ready code.

## Table of Contents

1. [Query Resolvers](#query-resolvers)
2. [Mutation Resolvers](#mutation-resolvers)
3. [Field Resolvers](#field-resolvers)
4. [Subscription Resolvers](#subscription-resolvers)
5. [DataLoader Integration](#dataloader-integration)
6. [Error Handling](#error-handling)
7. [Testing Examples](#testing-examples)

## Query Resolvers

### Actor Query

```go
// graph/schema.resolvers.go

func (r *queryResolver) Actor(ctx context.Context, id *string, username *string) (*activitypub.Actor, error) {
    // Input validation
    if id == nil && username == nil {
        return nil, fmt.Errorf("either id or username must be provided")
    }

    // Track operation cost
    r.CostTracker.TrackDynamoRead(1)

    var actor *activitypub.Actor
    var err error

    if username != nil {
        // Direct username lookup
        actor, err = r.Storage.GetActor(ctx, *username)
    } else if id != nil {
        // Extract username from ActivityPub ID
        username := r.MastodonConv.ExtractUsernameFromActorID(*id)
        if username == "" {
            return nil, fmt.Errorf("invalid actor id format: %s", *id)
        }
        
        // Check if remote actor
        if strings.Contains(*id, "https://") {
            // Try cached remote actor first
            actor, err = r.Storage.GetCachedRemoteActor(ctx, *id)
            if err == storage.ErrNotFound {
                // Fetch from remote if not cached
                actor, err = r.Federation.FetchRemoteActor(ctx, *id)
                if err == nil {
                    // Cache for future use
                    r.Storage.CacheRemoteActor(ctx, *id, actor, 24*time.Hour)
                }
            }
        } else {
            actor, err = r.Storage.GetActor(ctx, username)
        }
    }

    if err != nil {
        if err == storage.ErrNotFound {
            return nil, fmt.Errorf("actor not found")
        }
        r.Logger.Error("Failed to get actor", zap.Error(err))
        return nil, fmt.Errorf("internal error fetching actor")
    }

    return actor, nil
}
```

### Object Query

```go
func (r *queryResolver) Object(ctx context.Context, id string) (*model.Object, error) {
    // Track cost
    r.CostTracker.TrackDynamoRead(1)

    // Get object from storage
    obj, err := r.Storage.GetObject(ctx, id)
    if err != nil {
        if err == storage.ErrNotFound {
            return nil, fmt.Errorf("object not found: %s", id)
        }
        r.Logger.Error("Failed to get object", zap.String("id", id), zap.Error(err))
        return nil, fmt.Errorf("internal error")
    }

    // Convert to GraphQL model
    graphqlObj := r.convertToGraphQLObject(obj)

    // Load engagement metrics
    likes, _ := r.Storage.CountObjectLikes(ctx, id)
    announces, _ := r.Storage.CountObjectAnnounces(ctx, id)
    
    graphqlObj.LikesCount = likes
    graphqlObj.SharesCount = announces

    // Load community notes if any
    notes, _ := r.Storage.GetVisibleCommunityNotes(ctx, id)
    graphqlObj.CommunityNotes = r.convertCommunityNotes(notes)

    // Calculate estimated cost
    graphqlObj.EstimatedCost = r.CostTracker.EstimateObjectCost(graphqlObj)

    return graphqlObj, nil
}
```

### Timeline Query

```go
func (r *queryResolver) Timeline(ctx context.Context, typeArg model.TimelineType, hashtag *string, 
    listID *string, first *int, after *model.Cursor) (*model.ObjectConnection, error) {
    
    // Default limit
    limit := 20
    if first != nil && *first > 0 && *first <= 100 {
        limit = *first
    }

    // Parse cursor
    var cursor string
    if after != nil {
        cursor = string(*after)
    }

    // Track cost based on timeline type
    switch typeArg {
    case model.TimelineTypeHome:
        r.CostTracker.TrackDynamoRead(2) // More expensive
    default:
        r.CostTracker.TrackDynamoRead(1)
    }

    // Get authenticated user from context
    user := r.getUserFromContext(ctx)
    
    var entries []*storage.TimelineEntry
    var nextCursor string
    var err error

    switch typeArg {
    case model.TimelineTypeHome:
        if user == nil {
            return nil, fmt.Errorf("authentication required for home timeline")
        }
        entries, nextCursor, err = r.Storage.GetHomeTimeline(ctx, user.Username, limit, cursor)

    case model.TimelineTypePublic:
        entries, nextCursor, err = r.Storage.GetPublicTimeline(ctx, false, limit, cursor)

    case model.TimelineTypeLocal:
        entries, nextCursor, err = r.Storage.GetPublicTimeline(ctx, true, limit, cursor)

    case model.TimelineTypeHashtag:
        if hashtag == nil {
            return nil, fmt.Errorf("hashtag parameter required for hashtag timeline")
        }
        entries, nextCursor, err = r.Storage.GetHashtagTimeline(ctx, *hashtag, false, limit, cursor)

    case model.TimelineTypeList:
        if listID == nil {
            return nil, fmt.Errorf("listId parameter required for list timeline")
        }
        entries, nextCursor, err = r.Storage.GetListTimeline(ctx, *listID, limit, cursor)

    case model.TimelineTypeDirect:
        if user == nil {
            return nil, fmt.Errorf("authentication required for direct timeline")
        }
        // Direct messages are filtered from home timeline
        entries, nextCursor, err = r.Storage.GetHomeTimeline(ctx, user.Username, limit, cursor)
        // Filter for direct visibility
        entries = r.filterDirectMessages(entries)
    }

    if err != nil {
        r.Logger.Error("Failed to get timeline", zap.Error(err))
        return nil, fmt.Errorf("failed to load timeline")
    }

    // Convert to GraphQL connection
    return r.buildObjectConnection(ctx, entries, nextCursor, limit)
}
```

### Search Query

```go
func (r *queryResolver) Search(ctx context.Context, query string, typeArg *string, 
    first *int, after *model.Cursor) (*model.ObjectConnection, error) {
    
    // Validate query
    query = strings.TrimSpace(query)
    if query == "" {
        return nil, fmt.Errorf("search query cannot be empty")
    }

    // Default limit
    limit := 20
    if first != nil && *first > 0 && *first <= 100 {
        limit = *first
    }

    // Track search cost
    r.CostTracker.TrackDynamoRead(3) // Search is expensive

    // Determine search type
    searchType := "all"
    if typeArg != nil {
        searchType = *typeArg
    }

    // Multi-strategy search
    var results []*storage.StatusSearchResult
    var err error

    // Check if URL search
    if strings.HasPrefix(query, "https://") {
        results, err = r.performURLSearch(ctx, query)
    } else {
        // Use AI-enhanced search
        searchOptions := storage.StatusSearchOptions{
            Limit: limit,
            Offset: 0,
        }

        if after != nil {
            offset, _ := r.decodeCursor(string(*after))
            searchOptions.Offset = offset
        }

        results, err = r.AISearch.SearchWithAI(ctx, query, searchOptions)
    }

    if err != nil {
        r.Logger.Error("Search failed", zap.String("query", query), zap.Error(err))
        return nil, fmt.Errorf("search failed")
    }

    // Convert to GraphQL objects
    return r.buildSearchResultConnection(ctx, results, query, limit)
}
```

### Instance Metrics Query

```go
func (r *queryResolver) InstanceMetrics(ctx context.Context) (*model.InstanceMetrics, error) {
    // Track metric query
    r.CostTracker.TrackDynamoRead(4) // Multiple queries

    // Get various metrics in parallel
    eg, ctx := errgroup.WithContext(ctx)
    
    var activeUsers int64
    var totalUsers int64
    var totalStatuses int64
    var storageGB float64

    eg.Go(func() error {
        var err error
        activeUsers, err = r.Storage.GetActiveUserCount(ctx, 7) // Last 7 days
        return err
    })

    eg.Go(func() error {
        var err error
        totalUsers, err = r.Storage.GetTotalUserCount(ctx)
        return err
    })

    eg.Go(func() error {
        var err error
        totalStatuses, err = r.Storage.GetTotalStatusCount(ctx)
        return err
    })

    eg.Go(func() error {
        // Get storage metrics from CloudWatch
        var err error
        storageGB, err = r.Metrics.GetStorageUsedGB(ctx)
        return err
    })

    if err := eg.Wait(); err != nil {
        r.Logger.Error("Failed to get instance metrics", zap.Error(err))
        return nil, fmt.Errorf("failed to load metrics")
    }

    // Calculate request rate
    requestRate := r.Metrics.GetRequestsPerMinute()

    // Get average latency
    avgLatency := r.Metrics.GetAverageLatency()

    // Calculate estimated monthly cost
    monthlyStatsCount := totalStatuses * 30 // Rough estimate
    dynamoCost := r.CostCalculator.EstimateDynamoDBCost(monthlyStatsCount)
    s3Cost := r.CostCalculator.EstimateS3Cost(storageGB)
    lambdaCost := r.CostCalculator.EstimateLambdaCost(requestRate * 60 * 24 * 30)
    totalCost := dynamoCost + s3Cost + lambdaCost

    return &model.InstanceMetrics{
        ActiveUsers:          int(activeUsers),
        RequestsPerMinute:    int(requestRate),
        AverageLatencyMs:     avgLatency,
        StorageUsedGb:        storageGB,
        EstimatedMonthlyCost: totalCost,
        LastUpdated:          model.Time(time.Now()),
    }, nil
}
```

### Trust Graph Query

```go
func (r *queryResolver) TrustGraph(ctx context.Context, actorID string, 
    category *trust.TrustCategory) ([]*trust.TrustEdge, error) {
    
    r.CostTracker.TrackDynamoRead(2)

    // Get trust relationships
    relationships, _, err := r.Storage.GetTrustRelationships(ctx, actorID, 100, "")
    if err != nil {
        r.Logger.Error("Failed to get trust relationships", zap.Error(err))
        return nil, fmt.Errorf("failed to load trust graph")
    }

    // Filter by category if specified
    var edges []*trust.TrustEdge
    for _, rel := range relationships {
        if category != nil && rel.Category != string(*category) {
            continue
        }

        edge := &trust.TrustEdge{
            From:     rel.TrusterID,
            To:       rel.TrusteeID,
            Category: trust.TrustCategory(rel.Category),
            Score:    rel.Score,
            UpdatedAt: model.Time(rel.UpdatedAt),
        }
        edges = append(edges, edge)
    }

    return edges, nil
}
```

## Mutation Resolvers

### Create Note Mutation

```go
func (r *mutationResolver) CreateNote(ctx context.Context, input model.CreateNoteInput) (*model.CreateNotePayload, error) {
    // Get authenticated user
    user := r.getUserFromContext(ctx)
    if user == nil {
        return nil, fmt.Errorf("authentication required")
    }

    // Validate input
    if strings.TrimSpace(input.Content) == "" {
        return nil, fmt.Errorf("content cannot be empty")
    }

    // Track creation cost
    r.CostTracker.TrackDynamoWrite(1)
    r.CostTracker.TrackDynamoRead(len(input.Mentions)) // For mention lookups

    // Create ActivityPub Note object
    note := &activitypub.Note{
        Type:      "Note",
        ID:        r.generateObjectID(user.Username, "note"),
        Actor:     r.generateActorID(user.Username),
        Content:   input.Content,
        Published: time.Now(),
        To:        r.determineAudience(input.Visibility),
        CC:        []string{},
    }

    // Process mentions
    for _, mention := range input.Mentions {
        mentionedActor, err := r.Storage.GetActor(ctx, mention)
        if err == nil {
            note.Tag = append(note.Tag, activitypub.Mention{
                Type: "Mention",
                Href: r.generateActorID(mention),
                Name: "@" + mention,
            })
            note.CC = append(note.CC, r.generateActorID(mention))
        }
    }

    // Process hashtags
    for _, tag := range input.Tags {
        note.Tag = append(note.Tag, activitypub.Hashtag{
            Type: "Hashtag",
            Name: "#" + tag,
            Href: r.generateHashtagURL(tag),
        })
        // Index for search
        r.Storage.IndexHashtag(ctx, tag, note.ID, user.ActorID, string(input.Visibility))
    }

    // Handle reply
    if input.InReplyToID != nil {
        note.InReplyTo = *input.InReplyToID
    }

    // Handle sensitive content
    if input.Sensitive != nil && *input.Sensitive {
        note.Sensitive = true
        if input.SpoilerText != nil {
            note.Summary = *input.SpoilerText
        }
    }

    // Store the note
    if err := r.Storage.CreateObject(ctx, note); err != nil {
        r.Logger.Error("Failed to create note", zap.Error(err))
        return nil, fmt.Errorf("failed to create note")
    }

    // Create the Create activity
    activity := &activitypub.Activity{
        Type:      "Create",
        ID:        r.generateActivityID(user.Username, "create"),
        Actor:     note.Actor,
        Object:    note,
        Published: note.Published,
        To:        note.To,
        CC:        note.CC,
    }

    // Store the activity
    if err := r.Storage.CreateActivity(ctx, activity); err != nil {
        r.Logger.Error("Failed to create activity", zap.Error(err))
        return nil, fmt.Errorf("failed to create activity")
    }

    // Fan out to timelines
    if err := r.Storage.FanOutPost(ctx, activity); err != nil {
        r.Logger.Error("Failed to fan out post", zap.Error(err))
        // Don't fail the request, fan out can be retried
    }

    // Queue for federation
    if err := r.Federation.QueueActivity(ctx, activity); err != nil {
        r.Logger.Error("Failed to queue federation", zap.Error(err))
        // Don't fail the request
    }

    // Convert to GraphQL response
    graphqlObject := r.convertNoteToGraphQL(note)
    
    // Calculate operation cost
    operationCost := r.CostTracker.GetLastOperationCost()
    monthlyProjection := r.CostTracker.ProjectMonthlyCost()

    return &model.CreateNotePayload{
        Object:   graphqlObject,
        Activity: activity,
        Cost: &model.CostUpdate{
            OperationCost:     operationCost,
            DailyTotal:        r.CostTracker.GetDailyTotal(),
            MonthlyProjection: monthlyProjection,
        },
    }, nil
}
```

### Follow Actor Mutation

```go
func (r *mutationResolver) FollowActor(ctx context.Context, id string) (*activitypub.Activity, error) {
    // Get authenticated user
    user := r.getUserFromContext(ctx)
    if user == nil {
        return nil, fmt.Errorf("authentication required")
    }

    // Track cost
    r.CostTracker.TrackDynamoWrite(1)
    r.CostTracker.TrackDynamoRead(1)

    // Extract username from ID
    targetUsername := r.MastodonConv.ExtractUsernameFromActorID(id)
    if targetUsername == "" {
        return nil, fmt.Errorf("invalid actor ID")
    }

    // Check if already following
    isFollowing, _ := r.Storage.IsFollowing(ctx, user.Username, targetUsername)
    if isFollowing {
        return nil, fmt.Errorf("already following this actor")
    }

    // Create Follow activity
    activity := &activitypub.Activity{
        Type:      "Follow",
        ID:        r.generateActivityID(user.Username, "follow"),
        Actor:     r.generateActorID(user.Username),
        Object:    id,
        Published: time.Now(),
    }

    // Store the follow relationship
    if err := r.Storage.CreateFollow(ctx, user.Username, targetUsername, activity.ID); err != nil {
        r.Logger.Error("Failed to create follow", zap.Error(err))
        return nil, fmt.Errorf("failed to follow actor")
    }

    // Store the activity
    if err := r.Storage.CreateActivity(ctx, activity); err != nil {
        r.Logger.Error("Failed to create follow activity", zap.Error(err))
        return nil, fmt.Errorf("failed to create activity")
    }

    // Queue for federation if remote actor
    if strings.Contains(id, "https://") {
        if err := r.Federation.QueueActivity(ctx, activity); err != nil {
            r.Logger.Error("Failed to queue follow for federation", zap.Error(err))
        }
    } else {
        // Local follow - auto accept
        r.Storage.AcceptFollow(ctx, user.Username, targetUsername)
    }

    return activity, nil
}
```

### Update Trust Mutation

```go
func (r *mutationResolver) UpdateTrust(ctx context.Context, input model.TrustInput) (*trust.TrustEdge, error) {
    // Get authenticated user
    user := r.getUserFromContext(ctx)
    if user == nil {
        return nil, fmt.Errorf("authentication required")
    }

    // Validate score
    if input.Score < -1.0 || input.Score > 1.0 {
        return nil, fmt.Errorf("trust score must be between -1.0 and 1.0")
    }

    // Track cost
    r.CostTracker.TrackDynamoWrite(1)

    // Create or update trust relationship
    relationship := &trust.TrustRelationship{
        TrusterID: user.ActorID,
        TrusteeID: input.TargetActorID,
        Category:  string(input.Category),
        Score:     input.Score,
        UpdatedAt: time.Now(),
    }

    if err := r.Storage.CreateTrustRelationship(ctx, relationship); err != nil {
        r.Logger.Error("Failed to update trust", zap.Error(err))
        return nil, fmt.Errorf("failed to update trust relationship")
    }

    // Record trust update for history
    update := &trust.TrustUpdate{
        TrusterID: user.ActorID,
        TrusteeID: input.TargetActorID,
        Category:  string(input.Category),
        OldScore:  0, // Would need to fetch previous
        NewScore:  input.Score,
        Reason:    "Manual update",
        Timestamp: time.Now(),
    }
    r.Storage.RecordTrustUpdate(ctx, update)

    // Return edge
    return &trust.TrustEdge{
        From:      relationship.TrusterID,
        To:        relationship.TrusteeID,
        Category:  trust.TrustCategory(relationship.Category),
        Score:     relationship.Score,
        UpdatedAt: model.Time(relationship.UpdatedAt),
    }, nil
}
```

## Field Resolvers

### Actor Field Resolvers

```go
func (r *actorResolver) TrustScore(ctx context.Context, obj *activitypub.Actor) (float64, error) {
    // Use DataLoader to batch load trust scores
    loader := r.getDataLoader(ctx).TrustScoreLoader
    score, err := loader.Load(obj.ID)()
    if err != nil {
        return 0.0, nil // Default to 0 on error
    }
    return score.(float64), nil
}

func (r *actorResolver) Reputation(ctx context.Context, obj *activitypub.Actor) (*model.Reputation, error) {
    // Load from DataLoader
    loader := r.getDataLoader(ctx).ReputationLoader
    rep, err := loader.Load(obj.ID)()
    if err != nil {
        return nil, nil // Optional field
    }
    return rep.(*model.Reputation), nil
}

func (r *actorResolver) Vouches(ctx context.Context, obj *activitypub.Actor) ([]*model.Vouch, error) {
    // Load vouches
    vouches, err := r.Storage.GetVouchesForActor(ctx, obj.ID, true)
    if err != nil {
        return []*model.Vouch{}, nil
    }
    
    // Convert to GraphQL model
    var result []*model.Vouch
    for _, v := range vouches {
        result = append(result, r.convertVouchToGraphQL(v))
    }
    return result, nil
}
```

### Object Field Resolvers

```go
func (r *objectResolver) Actor(ctx context.Context, obj *model.Object) (*activitypub.Actor, error) {
    // Use DataLoader to batch load actors
    loader := r.getDataLoader(ctx).ActorLoader
    actor, err := loader.Load(obj.ActorID)()
    if err != nil {
        r.Logger.Error("Failed to load actor for object", zap.Error(err))
        return nil, err
    }
    return actor.(*activitypub.Actor), nil
}

func (r *objectResolver) CommunityNotes(ctx context.Context, obj *model.Object) ([]*model.CommunityNote, error) {
    // Only load if requested (field selection)
    notes, err := r.Storage.GetVisibleCommunityNotes(ctx, obj.ID)
    if err != nil {
        return []*model.CommunityNote{}, nil
    }
    return r.convertCommunityNotes(notes), nil
}

func (r *objectResolver) ModerationScore(ctx context.Context, obj *model.Object) (*float64, error) {
    // Calculate moderation score if needed
    if obj.ModerationScore == nil {
        score := r.Moderation.CalculateScore(ctx, obj)
        return &score, nil
    }
    return obj.ModerationScore, nil
}
```

## Subscription Resolvers

### Activity Stream Subscription

```go
func (r *subscriptionResolver) ActivityStream(ctx context.Context, types []model.ActivityType) (<-chan *activitypub.Activity, error) {
    // Get authenticated user
    user := r.getUserFromContext(ctx)
    if user == nil {
        return nil, fmt.Errorf("authentication required")
    }

    // Create channel for streaming
    ch := make(chan *activitypub.Activity, 10)

    // Subscribe to activity stream
    sub := r.PubSub.Subscribe(fmt.Sprintf("user:%s:activities", user.Username))

    go func() {
        defer close(ch)
        defer sub.Close()

        for {
            select {
            case <-ctx.Done():
                return
            case msg := <-sub.Channel():
                // Parse activity
                var activity activitypub.Activity
                if err := json.Unmarshal([]byte(msg.Payload), &activity); err != nil {
                    r.Logger.Error("Failed to unmarshal activity", zap.Error(err))
                    continue
                }

                // Filter by type if specified
                if len(types) > 0 {
                    typeMatch := false
                    for _, t := range types {
                        if string(t) == activity.Type {
                            typeMatch = true
                            break
                        }
                    }
                    if !typeMatch {
                        continue
                    }
                }

                // Send to channel
                select {
                case ch <- &activity:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()

    return ch, nil
}
```

### Cost Updates Subscription

```go
func (r *subscriptionResolver) CostUpdates(ctx context.Context, threshold *int) (<-chan *model.CostUpdate, error) {
    // Get authenticated user
    user := r.getUserFromContext(ctx)
    if user == nil {
        return nil, fmt.Errorf("authentication required")
    }

    ch := make(chan *model.CostUpdate, 10)
    
    // Default threshold
    costThreshold := 1000 // micros
    if threshold != nil {
        costThreshold = *threshold
    }

    // Subscribe to cost updates
    sub := r.PubSub.Subscribe(fmt.Sprintf("user:%s:costs", user.Username))

    go func() {
        defer close(ch)
        defer sub.Close()

        for {
            select {
            case <-ctx.Done():
                return
            case msg := <-sub.Channel():
                var update model.CostUpdate
                if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
                    continue
                }

                // Only send if above threshold
                if update.OperationCost >= costThreshold {
                    select {
                    case ch <- &update:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }
    }()

    return ch, nil
}
```

## DataLoader Integration

### Setup DataLoaders

```go
// graph/dataloader.go

type Loaders struct {
    ActorLoader      *dataloader.Loader
    ObjectLoader     *dataloader.Loader
    TrustScoreLoader *dataloader.Loader
    ReputationLoader *dataloader.Loader
}

func NewLoaders(storage storage.Storage, logger *zap.Logger) *Loaders {
    return &Loaders{
        ActorLoader: dataloader.NewBatchedLoader(
            func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
                results := make([]*dataloader.Result, len(keys))
                
                // Batch load actors
                actors := make(map[string]*activitypub.Actor)
                for _, key := range keys {
                    actor, err := storage.GetActor(ctx, key.String())
                    if err != nil {
                        results = append(results, &dataloader.Result{Error: err})
                    } else {
                        actors[key.String()] = actor
                    }
                }

                // Return in order
                for i, key := range keys {
                    if actor, ok := actors[key.String()]; ok {
                        results[i] = &dataloader.Result{Data: actor}
                    } else {
                        results[i] = &dataloader.Result{Error: fmt.Errorf("actor not found")}
                    }
                }

                return results
            },
            dataloader.WithCache(&dataloader.InMemoryCache{}),
            dataloader.WithBatchCapacity(50),
        ),

        TrustScoreLoader: dataloader.NewBatchedLoader(
            func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
                results := make([]*dataloader.Result, len(keys))
                
                // Batch load trust scores
                for i, key := range keys {
                    score, err := storage.GetTrustScore(ctx, key.String(), "overall")
                    if err != nil {
                        results[i] = &dataloader.Result{Data: float64(0)}
                    } else {
                        results[i] = &dataloader.Result{Data: score.Score}
                    }
                }

                return results
            },
        ),
    }
}

// Middleware to inject DataLoaders into context
func DataLoaderMiddleware(loaders *Loaders) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := context.WithValue(r.Context(), "dataloaders", loaders)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// Helper to get DataLoaders from context
func (r *Resolver) getDataLoader(ctx context.Context) *Loaders {
    return ctx.Value("dataloaders").(*Loaders)
}
```

## Error Handling

### Custom Error Types

```go
// graph/errors.go

type GraphQLError struct {
    Message    string
    Code       string
    Extensions map[string]any
}

func (e GraphQLError) Error() string {
    return e.Message
}

// Common errors
var (
    ErrAuthentication = GraphQLError{
        Message: "Authentication required",
        Code:    "UNAUTHENTICATED",
    }
    
    ErrNotFound = GraphQLError{
        Message: "Resource not found",
        Code:    "NOT_FOUND",
    }
    
    ErrRateLimit = GraphQLError{
        Message: "Rate limit exceeded",
        Code:    "RATE_LIMITED",
        Extensions: map[string]any{
            "retryAfter": 60,
        },
    }
)

// Error handler middleware
func ErrorHandler(logger *zap.Logger) graphql.ErrorPresenterFunc {
    return func(ctx context.Context, err error) *gqlerror.Error {
        // Log the error
        logger.Error("GraphQL error", zap.Error(err))

        // Convert to GraphQL error
        var gqlErr *gqlerror.Error
        
        switch e := err.(type) {
        case GraphQLError:
            gqlErr = &gqlerror.Error{
                Message: e.Message,
                Extensions: map[string]any{
                    "code": e.Code,
                },
            }
            for k, v := range e.Extensions {
                gqlErr.Extensions[k] = v
            }
            
        case *gqlerror.Error:
            gqlErr = e
            
        default:
            gqlErr = &gqlerror.Error{
                Message: "Internal server error",
                Extensions: map[string]any{
                    "code": "INTERNAL_ERROR",
                },
            }
        }

        return gqlErr
    }
}
```

## Testing Examples

### Resolver Unit Tests

```go
// graph/resolvers_test.go

func TestActorQuery(t *testing.T) {
    // Setup
    mockStorage := &mocks.MockStorage{}
    resolver := &Resolver{
        Storage:     mockStorage,
        CostTracker: cost.NewTracker(),
        Logger:      zap.NewNop(),
    }

    // Test data
    expectedActor := &activitypub.Actor{
        ID:       "https://example.com/users/alice",
        Username: "alice",
    }

    // Mock expectations
    mockStorage.On("GetActor", mock.Anything, "alice").Return(expectedActor, nil)

    // Execute
    actor, err := resolver.Query().Actor(context.Background(), nil, ptr("alice"))

    // Assert
    assert.NoError(t, err)
    assert.Equal(t, expectedActor, actor)
    mockStorage.AssertExpectations(t)
}

func TestCreateNoteMutation(t *testing.T) {
    // Setup
    ctx := context.WithValue(context.Background(), "user", &User{
        Username: "alice",
        ActorID:  "https://example.com/users/alice",
    })

    mockStorage := &mocks.MockStorage{}
    resolver := &Resolver{
        Storage:      mockStorage,
        CostTracker:  cost.NewTracker(),
        Logger:       zap.NewNop(),
        Federation:   &mocks.MockFederation{},
        MastodonConv: mastodon.NewConverter("example.com"),
    }

    // Input
    input := model.CreateNoteInput{
        Content:    "Hello, world!",
        Visibility: model.VisibilityPublic,
    }

    // Mock expectations
    mockStorage.On("CreateObject", mock.Anything, mock.AnythingOfType("*activitypub.Note")).Return(nil)
    mockStorage.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
    mockStorage.On("FanOutPost", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

    // Execute
    payload, err := resolver.Mutation().CreateNote(ctx, input)

    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, payload)
    assert.Equal(t, "Hello, world!", payload.Object.Content)
    assert.NotNil(t, payload.Cost)
    mockStorage.AssertExpectations(t)
}
```

### Integration Tests

```go
// graph/integration_test.go

func TestTimelineQueryIntegration(t *testing.T) {
    // Setup test server
    srv := testutil.NewTestServer(t)
    defer srv.Close()

    // Create test data
    user := srv.CreateTestUser("alice")
    srv.CreateTestPost(user, "Post 1")
    srv.CreateTestPost(user, "Post 2")

    // GraphQL query
    query := `
        query GetTimeline {
            timeline(type: HOME, first: 10) {
                edges {
                    node {
                        id
                        content
                        createdAt
                    }
                }
                pageInfo {
                    hasNextPage
                    endCursor
                }
            }
        }
    `

    // Execute query
    resp := srv.GraphQLRequest(query, nil, user.Token)

    // Assert
    assert.True(t, resp.Success)
    assert.Len(t, resp.Data.Timeline.Edges, 2)
    assert.Equal(t, "Post 2", resp.Data.Timeline.Edges[0].Node.Content)
}
```

## Conclusion

This implementation guide provides comprehensive examples for transforming Lesser's stubbed GraphQL resolvers into production-ready code. Key aspects covered:

1. **Complete resolver implementations** with proper error handling and cost tracking
2. **DataLoader integration** to prevent N+1 queries
3. **Real-time subscriptions** using WebSocket connections
4. **Comprehensive testing** strategies
5. **Security and performance** best practices

Following these patterns will ensure a robust, scalable, and maintainable GraphQL API for Lesser. 