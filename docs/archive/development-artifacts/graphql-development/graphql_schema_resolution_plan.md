# Lesser GraphQL Schema Resolution Plan

## Executive Summary

This document presents a comprehensive plan to resolve the stubbed GraphQL implementation in Lesser, transforming it into a fully functional, production-ready GraphQL API that maintains 100% compatibility with the Mastodon API while introducing innovative Lesser enhancements.

## Table of Contents

1. [Current State Analysis](#current-state-analysis)
2. [Architecture Overview](#architecture-overview)
3. [Implementation Strategy](#implementation-strategy)
4. [Phase 1: Core Queries](#phase-1-core-queries)
5. [Phase 2: Mutations](#phase-2-mutations)
6. [Phase 3: Enhanced Features](#phase-3-enhanced-features)
7. [Phase 4: Subscriptions](#phase-4-subscriptions)
8. [Performance Optimization](#performance-optimization)
9. [Testing Strategy](#testing-strategy)
10. [Security Considerations](#security-considerations)
11. [Migration Path](#migration-path)
12. [Timeline & Milestones](#timeline--milestones)

## Current State Analysis

### Implemented Components
- ✅ Complete GraphQL schema (`graph/schema.graphql`)
- ✅ Generated types and server code
- ✅ Basic resolver structure
- ✅ Lambda handler setup
- ✅ Cost tracking integration
- ⚠️ Partial `actor` query implementation
- ⚠️ Mock `instanceMetrics` query

### Stubbed Components
- ❌ 95% of query resolvers
- ❌ All mutation resolvers
- ❌ All subscription resolvers
- ❌ Most field resolvers
- ❌ DataLoader integration
- ❌ Real-time subscriptions

### Available Dependencies
- `storage.Storage` - Comprehensive data access layer
- `cost.Tracker` - Real-time cost tracking
- `mastodon.Converter` - Type conversion utilities
- `ai.Service` - AI analysis capabilities
- `moderation.Service` - Moderation system
- `trust.Service` - Trust graph operations

## Architecture Overview

```
┌─────────────────┐
│  GraphQL Client │
└────────┬────────┘
         │
    ┌────▼────┐
    │ Lambda  │
    │ Handler │
    └────┬────┘
         │
┌────────▼────────┐     ┌──────────────┐
│ GraphQL Server  │────▶│ DataLoader   │
│   (gqlgen)      │     │ (Batching)   │
└────────┬────────┘     └──────────────┘
         │
┌────────▼────────┐
│    Resolvers    │
└────────┬────────┘
         │
┌────────▼────────────────────────┐
│        Service Layer            │
├─────────────┬───────────────────┤
│ ActivityPub │ Mastodon │  AI    │
│  Service    │ Converter│Service │
└─────────────┴─────────┬─────────┘
                        │
              ┌─────────▼─────────┐
              │  Storage Layer    │
              │   (DynamoDB)      │
              └───────────────────┘
```

## Implementation Strategy

### Core Principles

1. **Incremental Implementation** - Start with core queries, then mutations, then advanced features
2. **Test-Driven Development** - Write tests before implementing each resolver
3. **Performance First** - Use DataLoader from the start to prevent N+1 queries
4. **Cost Awareness** - Track every operation's cost
5. **Type Safety** - Leverage Go's type system and gqlgen's code generation

### Resolver Implementation Pattern

```go
func (r *queryResolver) ExampleQuery(ctx context.Context, args Args) (*Result, error) {
    // 1. Validate input
    if err := validateArgs(args); err != nil {
        return nil, err
    }
    
    // 2. Track costs
    r.CostTracker.TrackDynamoRead(1)
    
    // 3. Load data (with DataLoader for relationships)
    data, err := r.Storage.GetData(ctx, args.ID)
    if err != nil {
        r.Logger.Error("Failed to get data", zap.Error(err))
        return nil, err
    }
    
    // 4. Convert to GraphQL types
    result := r.MastodonConv.ConvertToGraphQL(data)
    
    // 5. Return result
    return result, nil
}
```

## Phase 1: Core Queries

### Priority 1: Actor & Object Queries

#### 1.1 Actor Query (Enhancement)
```go
// Current: Partial implementation
// Target: Full implementation with all fields

func (r *queryResolver) Actor(ctx context.Context, id *string, username *string) (*activitypub.Actor, error) {
    // Enhanced implementation with:
    // - Proper caching
    // - Remote actor fetching
    // - Trust score calculation
    // - Field verification
}
```

#### 1.2 Object Query
```go
func (r *queryResolver) Object(ctx context.Context, id string) (*model.Object, error) {
    // Implementation:
    // - Support all object types (Note, Article, etc.)
    // - Include engagement metrics
    // - Add community notes
    // - Calculate estimated cost
}
```

### Priority 2: Timeline Queries

#### 2.1 Timeline Query
```go
func (r *queryResolver) Timeline(ctx context.Context, typeArg model.TimelineType, ...) (*model.ObjectConnection, error) {
    // Implementation for each timeline type:
    // - HOME: User's home timeline
    // - PUBLIC: Public timeline (local/federated)
    // - HASHTAG: Hashtag timeline
    // - LIST: List timeline
    // - DIRECT: Direct messages
}
```

#### 2.2 Search Query
```go
func (r *queryResolver) Search(ctx context.Context, query string, ...) (*model.ObjectConnection, error) {
    // Multi-strategy search:
    // - Exact match (DynamoDB GSI)
    // - Semantic search (AI embeddings)
    // - Popularity-based ranking
}
```

### Priority 3: Instance & Cost Queries

#### 3.1 Instance Metrics (Enhancement)
```go
func (r *queryResolver) InstanceMetrics(ctx context.Context) (*model.InstanceMetrics, error) {
    // Real implementation:
    // - Query actual metrics from CloudWatch
    // - Calculate real costs
    // - Get active user counts
}
```

#### 3.2 Cost Breakdown
```go
func (r *queryResolver) CostBreakdown(ctx context.Context, period *model.Period) (*model.CostBreakdown, error) {
    // Detailed cost analysis:
    // - Break down by operation type
    // - Historical trends
    // - Projections
}
```

## Phase 2: Mutations

### Priority 1: Content Operations

#### 2.1 Create Note
```go
func (r *mutationResolver) CreateNote(ctx context.Context, input model.CreateNoteInput) (*model.CreateNotePayload, error) {
    // Full ActivityPub Create activity:
    // - Validate content
    // - Process mentions/tags
    // - Fan out to timelines
    // - Queue federation delivery
    // - Return with cost info
}
```

#### 2.2 Social Operations
```go
// Like, Share, Follow operations
func (r *mutationResolver) LikeObject(ctx context.Context, id string) (*activitypub.Activity, error)
func (r *mutationResolver) ShareObject(ctx context.Context, id string) (*activitypub.Activity, error)
func (r *mutationResolver) FollowActor(ctx context.Context, id string) (*activitypub.Activity, error)
```

### Priority 2: Moderation Operations

#### 2.3 Trust Updates
```go
func (r *mutationResolver) UpdateTrust(ctx context.Context, input model.TrustInput) (*trust.TrustEdge, error) {
    // Trust graph updates:
    // - Validate trust relationship
    // - Update trust scores
    // - Propagate through graph
}
```

#### 2.4 Community Notes
```go
func (r *mutationResolver) AddCommunityNote(ctx context.Context, input model.CommunityNoteInput) (*model.CommunityNotePayload, error) {
    // Community moderation:
    // - Validate note content
    // - Check rate limits
    // - Calculate initial score
    // - Notify relevant users
}
```

## Phase 3: Enhanced Features

### AI Integration

#### 3.1 AI Analysis Queries
```go
func (r *queryResolver) AIAnalysis(ctx context.Context, objectId string) (*model.AIAnalysis, error) {
    // Comprehensive AI analysis:
    // - Text sentiment & toxicity
    // - Image moderation
    // - AI content detection
    // - Spam analysis
}
```

#### 3.2 AI Capabilities
```go
func (r *queryResolver) AICapabilities(ctx context.Context) (*model.AICapabilities, error) {
    // Return available AI features and costs
}
```

### Trust & Reputation

#### 3.3 Trust Graph
```go
func (r *queryResolver) TrustGraph(ctx context.Context, actorId string, category *trust.TrustCategory) ([]*trust.TrustEdge, error) {
    // Visualize trust relationships:
    // - Load trust edges
    // - Calculate paths
    // - Include scores
}
```

#### 3.4 Reputation Queries
```go
func (r *actorResolver) Reputation(ctx context.Context, obj *activitypub.Actor) (*model.Reputation, error) {
    // Portable reputation:
    // - Calculate scores
    // - Generate evidence
    // - Sign cryptographically
}
```

### Debug & Admin Tools

#### 3.5 Object Explanation
```go
func (r *queryResolver) ExplainObject(ctx context.Context, id string) (*model.ObjectExplanation, error) {
    // Detailed object info:
    // - Storage location
    // - Access patterns
    // - Cost breakdown
}
```

#### 3.6 Federation Status
```go
func (r *queryResolver) FederationStatus(ctx context.Context, domain string) (*model.FederationStatus, error) {
    // Instance health:
    // - Reachability
    // - Software version
    // - Trust score
}
```

## Phase 4: Subscriptions

### Real-time Updates

#### 4.1 Activity Stream
```go
func (r *subscriptionResolver) ActivityStream(ctx context.Context, types []model.ActivityType) (<-chan *activitypub.Activity, error) {
    // WebSocket via Lambda:
    // - Filter by activity types
    // - Stream real-time updates
    // - Handle reconnection
}
```

#### 4.2 Cost Updates
```go
func (r *subscriptionResolver) CostUpdates(ctx context.Context, threshold *int) (<-chan *model.CostUpdate, error) {
    // Real-time cost monitoring:
    // - Track operation costs
    // - Alert on thresholds
    // - Project monthly costs
}
```

#### 4.3 Moderation Events
```go
func (r *subscriptionResolver) ModerationEvents(ctx context.Context, actorId *string) (<-chan *moderation.ModerationDecision, error) {
    // Live moderation updates:
    // - New reports
    // - Decisions made
    // - Trust changes
}
```

## Performance Optimization

### DataLoader Integration

```go
// Prevent N+1 queries with batching
type Loaders struct {
    ActorLoader     *dataloader.Loader
    ObjectLoader    *dataloader.Loader
    TrustScoreLoader *dataloader.Loader
}

func NewLoaders(storage storage.Storage) *Loaders {
    return &Loaders{
        ActorLoader: dataloader.NewBatchedLoader(
            func(keys []string) []*dataloader.Result {
                // Batch load actors
            },
        ),
    }
}
```

### Caching Strategy

1. **Lambda Container Reuse** - Cache frequently accessed data in memory
2. **DynamoDB Caching** - Use DynamoDB's built-in caching
3. **CloudFront** - Cache public GraphQL queries
4. **Query Complexity** - Implement query depth and complexity limits

### Cost Optimization

```go
// Track and optimize expensive operations
middleware := func(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
    start := time.Now()
    costBefore := r.CostTracker.GetCurrentCost()
    
    resp := next(ctx)
    
    duration := time.Since(start)
    costAfter := r.CostTracker.GetCurrentCost()
    
    // Add cost headers
    resp.Extensions["cost"] = map[string]any{
        "total_micros": costAfter - costBefore,
        "duration_ms": duration.Milliseconds(),
    }
    
    return resp
}
```

## Testing Strategy

### Unit Tests

```go
func TestActorResolver(t *testing.T) {
    // Test each resolver method
    // Mock storage layer
    // Verify cost tracking
    // Check error handling
}
```

### Integration Tests

```python
# test_graphql_integration.py
def test_create_note_mutation():
    """Test full create note flow"""
    mutation = """
        mutation CreateNote($input: CreateNoteInput!) {
            createNote(input: $input) {
                object { id content }
                activity { id type }
                cost { operationCost }
            }
        }
    """
    # Execute and verify
```

### Performance Tests

```go
func BenchmarkTimelineQuery(b *testing.B) {
    // Benchmark timeline generation
    // Measure query complexity
    // Verify N+1 prevention
}
```

## Security Considerations

### Authentication & Authorization

```go
func (r *Resolver) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
            // Extract JWT token
            // Verify permissions
            // Add user to context
            ctx := context.WithValue(req.Context(), "user", user)
            next.ServeHTTP(w, req.WithContext(ctx))
        })
    }
}
```

### Rate Limiting

```go
type RateLimiter struct {
    costLimit   int
    timeWindow  time.Duration
}

func (r *RateLimiter) CheckLimit(ctx context.Context, userID string) error {
    // Check user's recent query costs
    // Enforce limits based on trust score
}
```

### Query Validation

```go
func ValidateQuery(query string) error {
    // Check query depth
    // Validate complexity
    // Prevent malicious queries
}
```

## Migration Path

### From REST to GraphQL

1. **Dual Support** - Run GraphQL alongside REST
2. **Client Migration** - Provide migration guides
3. **Deprecation Timeline** - Phase out REST endpoints
4. **Monitoring** - Track GraphQL adoption

### Schema Evolution

```graphql
# Use @deprecated directive
type Query {
  # Deprecated in favor of timeline query
  publicTimeline: [Object!]! @deprecated(reason: "Use timeline(type: PUBLIC)")
}
```

## Timeline & Milestones

### Week 1-2: Phase 1 Implementation
- [ ] Complete actor and object queries
- [ ] Implement timeline queries
- [ ] Add search functionality
- [ ] Real instance metrics

### Week 3-4: Phase 2 Implementation
- [ ] Content creation mutations
- [ ] Social graph operations
- [ ] Basic moderation features

### Week 5-6: Phase 3 Implementation
- [ ] AI integration
- [ ] Trust graph visualization
- [ ] Admin tools

### Week 7-8: Phase 4 Implementation
- [ ] WebSocket subscriptions
- [ ] Real-time updates
- [ ] Performance optimization

### Week 9-10: Polish & Launch
- [ ] Comprehensive testing
- [ ] Documentation
- [ ] Performance tuning
- [ ] Production deployment

## Success Metrics

1. **Performance**
   - < 50ms p95 latency for simple queries
   - < 200ms p95 for complex queries
   - Zero N+1 queries

2. **Cost**
   - < $0.001 per 1000 queries
   - Cost tracking accuracy > 99%

3. **Adoption**
   - 50% of API traffic via GraphQL within 3 months
   - 90% client satisfaction

4. **Reliability**
   - 99.9% uptime
   - < 0.1% error rate

## Conclusion

This comprehensive plan transforms Lesser's GraphQL implementation from stubbed code to a production-ready, feature-rich API that showcases the platform's innovative capabilities while maintaining full Mastodon compatibility. The phased approach ensures steady progress while maintaining system stability.

The combination of serverless architecture, real-time cost tracking, AI integration, and community-driven moderation creates a GraphQL API that is not just functional, but revolutionary in the federated social media space. 