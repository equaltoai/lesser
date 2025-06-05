# Lesser Server-Side Implementation Plan

## Overview

This document outlines the comprehensive server-side implementation plan for Lesser, a revolutionary serverless ActivityPub platform. All features described here are backend APIs and services that will be consumed by Greater (the frontend).

**Core Innovation**: Lesser demonstrates that federated social media can be essentially free to operate while providing superior features through serverless architecture and reactive systems.

## Architecture Principles

1. **Everything is an Event** - All actions trigger DynamoDB streams
2. **Reactive by Default** - Changes propagate through Lambda functions
3. **Cost-Conscious** - Every operation tracks its cost in real-time
4. **Federation-First** - Built for interoperability from day one
5. **Developer Experience** - APIs that make sense, debug tools that delight

## Phase 1: Enhanced Core Platform (Week 1-2)

### 1.1 Real-Time Cost Tracking

**Goal**: Every API response includes cost metadata

```go
// pkg/cost/tracker.go
type CostTracker struct {
    rates *AWSRates
}

type OperationCost struct {
    LambdaInvocations   int64   `json:"lambda_invocations"`
    DynamoDBReads      int64   `json:"dynamodb_reads"`
    DynamoDBWrites     int64   `json:"dynamodb_writes"`
    S3Operations       int64   `json:"s3_operations"`
    DataTransferBytes  int64   `json:"data_transfer_bytes"`
    TotalCostMicros    int64   `json:"total_cost_micros"` // Millionths of a cent
}

// Every API response includes:
type APIResponse struct {
    Data     interface{}    `json:"data"`
    Cost     *OperationCost `json:"cost"`
    Duration int64          `json:"duration_ms"`
}
```

**Implementation**:
- Instrument all storage operations
- Add cost calculation middleware
- Create cost aggregation Lambda
- Store cost history in DynamoDB

### 1.2 Activity Stream API

**Goal**: WebSocket and SSE endpoints for real-time activity monitoring

```go
// cmd/api/handlers/stream.go
type StreamEvent struct {
    ID        string                 `json:"id"`
    Type      string                 `json:"type"`
    Timestamp int64                  `json:"timestamp"`
    Actor     string                 `json:"actor"`
    Data      map[string]interface{} `json:"data"`
    Cost      *OperationCost         `json:"cost"`
}

// WebSocket: wss://instance.com/api/v1/streaming
// SSE: https://instance.com/api/v1/streaming/events
```

**Events to Stream**:
- Incoming federation activities
- Outgoing federation activities
- Moderation decisions
- Cost threshold alerts
- System performance metrics

### 1.3 Enhanced Metrics API

```go
// GET /api/v1/instance/metrics
type InstanceMetrics struct {
    Current struct {
        ActiveUsers      int     `json:"active_users"`
        FederatedPeers   int     `json:"federated_peers"`
        RequestsPerMin   float64 `json:"requests_per_minute"`
        MedianLatencyMs  float64 `json:"median_latency_ms"`
        ErrorRate        float64 `json:"error_rate"`
    } `json:"current"`
    
    Today struct {
        TotalRequests    int64   `json:"total_requests"`
        TotalCostMicros  int64   `json:"total_cost_micros"`
        UniqueVisitors   int     `json:"unique_visitors"`
        PostsCreated     int     `json:"posts_created"`
        InteractionsCount int     `json:"interactions_count"`
    } `json:"today"`
    
    Predicted struct {
        MonthlyCostUSD   float64 `json:"monthly_cost_usd"`
        StorageGrowthGB  float64 `json:"storage_growth_gb"`
    } `json:"predicted"`
}
```

## Phase 2: Reactive Moderation Mesh (Week 3-4)

### 2.1 Moderation Event System

```go
// pkg/moderation/events.go
type ModerationEvent struct {
    // Identity
    ID          string    `json:"id"`
    Type        EventType `json:"type"`
    Timestamp   int64     `json:"timestamp"`
    
    // Target
    ObjectID    string    `json:"object_id"`
    ObjectType  string    `json:"object_type"`
    
    // Actor
    ActorID     string    `json:"actor_id"`
    ActorTrust  float64   `json:"actor_trust"`
    
    // Decision
    Action      Action    `json:"action"`
    Confidence  float64   `json:"confidence"`
    Evidence    []Evidence `json:"evidence"`
    
    // Propagation
    Visibility  Visibility `json:"visibility"`
    Recipients  []string   `json:"recipients,omitempty"`
}

type EventType string
const (
    EventFlag      EventType = "flag"
    EventReview    EventType = "review"
    EventConsensus EventType = "consensus"
    EventAppeal    EventType = "appeal"
    EventOverride  EventType = "override"
)
```

### 2.2 Trust Graph Engine

```go
// pkg/trust/graph.go
type TrustGraph struct {
    storage *dynamodb.Client
}

// Trust relationships are directional edges
type TrustEdge struct {
    FromActor   string    `json:"from_actor"`
    ToActor     string    `json:"to_actor"`
    Category    string    `json:"category"` // content, behavior, technical
    Trust       float64   `json:"trust"`    // -1 to 1
    Confidence  float64   `json:"confidence"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// API Endpoints
// GET /api/v1/trust/graph?actor=@user@instance.com
// POST /api/v1/trust/edges
// GET /api/v1/trust/recommendations?category=content&count=5
```

### 2.3 Consensus Engine

```go
// pkg/moderation/consensus.go
type ConsensusEngine struct {
    MinReviewers      int           `json:"min_reviewers"`
    ConsensusTimeout  time.Duration `json:"consensus_timeout"`
    ThresholdMap      map[Action]float64 `json:"threshold_map"`
}

func (e *ConsensusEngine) Process(reviews []Review) *ConsensusResult {
    // Weight by reviewer trust and confidence
    weighted := e.weightReviews(reviews)
    
    // Calculate action scores
    scores := e.calculateActionScores(weighted)
    
    // Determine winning action
    action, confidence := e.determineAction(scores)
    
    return &ConsensusResult{
        Action:     action,
        Confidence: confidence,
        Reasoning:  e.generateReasoning(reviews),
        Reviewers:  len(reviews),
        Duration:   time.Since(reviews[0].Timestamp),
    }
}
```

### 2.4 Moderation API

```go
// POST /api/v1/moderation/flag
type FlagRequest struct {
    ObjectID string   `json:"object_id"`
    Reason   string   `json:"reason"`
    Evidence []string `json:"evidence,omitempty"`
}

// GET /api/v1/moderation/queue
type QueueResponse struct {
    Items []QueueItem `json:"items"`
    Stats QueueStats `json:"stats"`
}

// POST /api/v1/moderation/review
type ReviewRequest struct {
    EventID    string  `json:"event_id"`
    Action     Action  `json:"action"`
    Confidence float64 `json:"confidence"`
    Note       string  `json:"note,omitempty"`
}

// GET /api/v1/moderation/history/:object_id
type HistoryResponse struct {
    Object     interface{}        `json:"object"`
    Events     []ModerationEvent  `json:"events"`
    Timeline   []TimelineEntry    `json:"timeline"`
    CurrentStatus Status          `json:"current_status"`
}
```

## Phase 3: Developer Experience (Week 5)

### 3.1 GraphQL Gateway

```graphql
# /api/graphql endpoint
type Query {
  # Actor queries
  actor(id: ID, username: String): Actor
  actors(first: Int, after: String): ActorConnection
  
  # Content queries
  object(id: ID!): Object
  timeline(type: TimelineType!, first: Int, after: String): ObjectConnection
  
  # Moderation queries
  moderationQueue(filter: ModerationFilter): ModerationQueue
  trustGraph(actor: ID!): TrustGraph
  
  # Instance queries
  instanceMetrics: InstanceMetrics
  federationStatus: FederationStatus
  costBreakdown(period: Period): CostBreakdown
}

type Mutation {
  # Content mutations
  createNote(input: CreateNoteInput!): Note
  deleteObject(id: ID!): DeleteResult
  
  # Moderation mutations
  flag(input: FlagInput!): ModerationEvent
  review(input: ReviewInput!): ModerationEvent
  
  # Trust mutations
  updateTrust(input: TrustInput!): TrustEdge
}

type Subscription {
  # Real-time subscriptions
  activityStream(filter: ActivityFilter): Activity
  moderationEvents(objectId: ID): ModerationEvent
  costUpdates: CostUpdate
}
```

### 3.2 Debug Endpoints

```go
// GET /api/v1/debug/federation/:domain
type FederationDebug struct {
    Domain         string          `json:"domain"`
    LastContact    time.Time       `json:"last_contact"`
    Status         string          `json:"status"`
    SharedInbox    string          `json:"shared_inbox"`
    Errors         []FedError      `json:"recent_errors"`
    DeliveryStats  DeliveryStats   `json:"delivery_stats"`
    InstanceInfo   interface{}     `json:"instance_info"`
}

// GET /api/v1/debug/object/:id/explain
type ObjectExplanation struct {
    Object        interface{}     `json:"object"`
    Storage       StorageDebug    `json:"storage"`
    Indexes       []IndexEntry    `json:"indexes"`
    References    []Reference     `json:"references"`
    Cost          CostBreakdown   `json:"cost_to_retrieve"`
}

// POST /api/v1/debug/replay
type ReplayRequest struct {
    StartTime     time.Time       `json:"start_time"`
    EndTime       time.Time       `json:"end_time"`
    Filter        *ReplayFilter   `json:"filter,omitempty"`
    Speed         float64         `json:"speed"` // 1.0 = real-time
}
```

### 3.3 Testing Utilities

```go
// POST /api/v1/test/generate
type GenerateTestData struct {
    Users         int    `json:"users"`
    PostsPerUser  int    `json:"posts_per_user"`
    Interactions  int    `json:"interactions"`
    Pattern       string `json:"pattern"` // "normal", "viral", "spam"
}

// POST /api/v1/test/federate
type FederationTest struct {
    TargetDomain  string `json:"target_domain"`
    TestType      string `json:"test_type"`
    ExpectedResult interface{} `json:"expected_result"`
}
```

## Phase 4: Advanced Features (Week 6-7)

### 4.1 Portable Reputation API

```go
// GET /.well-known/reputation?actor=@user@instance.com
type PortableReputation struct {
    Actor        string           `json:"actor"`
    TrustScore   float64          `json:"trust_score"`
    Confidence   float64          `json:"confidence"`
    Categories   map[string]float64 `json:"categories"`
    Vouchers     []Vouch          `json:"vouchers"`
    History      []ReputationEvent `json:"history"`
    Signature    string           `json:"signature"`
    PublicKey    string           `json:"public_key"`
    ValidUntil   time.Time        `json:"valid_until"`
}

// POST /api/v1/reputation/vouch
type VouchRequest struct {
    Actor     string   `json:"actor"`
    Category  string   `json:"category"`
    Confidence float64 `json:"confidence"`
    Note      string   `json:"note,omitempty"`
}
```

### 4.2 Community Notes API

```go
// POST /api/v1/notes
type CreateNoteRequest struct {
    ObjectID  string   `json:"object_id"`
    Content   string   `json:"content"`
    Sources   []string `json:"sources"`
    Language  string   `json:"language,omitempty"`
}

// GET /api/v1/notes/:object_id
type NotesResponse struct {
    Notes []CommunityNote `json:"notes"`
    Stats NoteStats      `json:"stats"`
}

// POST /api/v1/notes/:id/vote
type VoteRequest struct {
    Vote     VoteType `json:"vote"` // helpful, not_helpful, neutral
    Reason   string   `json:"reason,omitempty"`
}

// Note visibility algorithm runs as Lambda triggered by votes
```

### 4.3 AI Integration Layer

```go
// Internal service, not exposed directly
type AIService struct {
    comprehend  *comprehend.Client
    rekognition *rekognition.Client
    bedrock     *bedrock.Client
}

// AI results enrich moderation events
type AIAnalysis struct {
    TextSentiment    *SentimentAnalysis  `json:"text_sentiment,omitempty"`
    TextToxicity     *ToxicityAnalysis   `json:"text_toxicity,omitempty"`
    ImageModeration  *ImageAnalysis      `json:"image_moderation,omitempty"`
    AIGenerated      *AIDetection        `json:"ai_generated,omitempty"`
    SpamProbability  float64             `json:"spam_probability"`
}
```

### 4.4 Plugin System

```go
// POST /api/v1/plugins
type PluginRegistration struct {
    Name         string          `json:"name"`
    Version      string          `json:"version"`
    Type         PluginType      `json:"type"`
    LambdaARN    string          `json:"lambda_arn"`
    Triggers     []Trigger       `json:"triggers"`
    Permissions  []Permission    `json:"permissions"`
}

// Plugins can hook into:
// - Activity processing pipeline
// - Moderation decisions
// - Timeline generation
// - Federation events
```

## Phase 5: Performance & Scale (Week 8)

### 5.1 Caching Strategy

```go
// CloudFront + DynamoDB DAX + Lambda memory cache
type CacheLayer struct {
    Level     string        `json:"level"`
    TTL       time.Duration `json:"ttl"`
    HitRate   float64       `json:"hit_rate"`
}

// Cache warming for hot paths
// - Actor profiles
// - Public timelines
// - Popular objects
// - Trust scores
```

### 5.2 Timeline Optimizations

```go
// Pre-computed timeline chunks
type TimelineChunk struct {
    UserID    string    `json:"user_id"`
    ChunkID   string    `json:"chunk_id"`
    StartTime time.Time `json:"start_time"`
    EndTime   time.Time `json:"end_time"`
    Items     []string  `json:"items"`
    Size      int       `json:"size"`
}

// Parallel timeline generation
// - Fan-out on write for home timelines
// - Lazy evaluation for lists
// - Hybrid approach for large accounts
```

### 5.3 Federation Optimizations

```go
// Shared inbox delivery
// Delivery coalescing
// Retry with exponential backoff
// Instance health tracking
// Predictive pre-warming
```

## API Authentication & Rate Limiting

### OAuth 2.0 + JWT
- Existing implementation
- Scopes for all new endpoints
- Rate limits based on trust score

### Rate Limit Headers
```
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 299
X-RateLimit-Reset: 1516131600
X-Cost-Limit-Monthly: 100000000
X-Cost-Remaining: 99999702
```

## Monitoring & Observability

### CloudWatch Dashboards
1. **System Health** - Latency, errors, throughput
2. **Cost Tracking** - Real-time spend by operation
3. **Moderation** - Queue depth, decision time, consensus rate
4. **Federation** - Delivery success, peer health
5. **User Activity** - Signups, posts, interactions

### X-Ray Tracing
- End-to-end request tracing
- Cost attribution per trace
- Performance bottleneck identification

### Custom Metrics
```go
// Publish to CloudWatch
type LesserMetrics struct {
    ModerationConsensusTime   time.Duration
    TrustGraphQueryTime       time.Duration
    FederationDeliverySuccess float64
    CostPerActiveUser         float64
}
```

## Developer Tools

### CLI Tool
```bash
lesser instance create --domain=myinstance.com
lesser cost estimate --users=1000 --posts-per-day=50
lesser moderation simulate --scenario=spam-attack
lesser federation test --target=mastodon.social
lesser debug timeline --user=@alice --explain
```

### SDK
```go
// Go SDK for Lesser instances
client := lesser.NewClient("https://myinstance.com", token)
metrics, _ := client.GetMetrics()
queue, _ := client.GetModerationQueue()
```

### Terraform/Pulumi Modules
```hcl
module "lesser_instance" {
  source = "github.com/lesser/infrastructure"
  domain = "myinstance.com"
  features = {
    moderation_mesh = true
    community_notes = true
    ai_integration  = true
  }
}
```

## Migration Tools

### Import from Mastodon
```bash
lesser import mastodon --backup=archive.tar.gz
```

### Export Everything
```bash
lesser export --format=activitypub --include-media
lesser export --format=mastodon-compatible
lesser export --format=static-site
```

## Success Metrics

### Week 1-2 Goals
- [ ] Cost tracking live on all endpoints
- [ ] Activity stream WebSocket working
- [ ] Basic metrics dashboard data available

### Week 3-4 Goals
- [ ] Moderation mesh processing events
- [ ] Trust graph queryable
- [ ] Consensus engine reaching decisions <1s

### Week 5 Goals
- [ ] GraphQL endpoint fully functional
- [ ] Debug tools helping development
- [ ] Test data generation working

### Week 6-7 Goals
- [ ] Portable reputation federated
- [ ] Community notes visible
- [ ] AI enriching moderation

### Week 8 Goals
- [ ] <50ms response times
- [ ] 99.9% uptime
- [ ] Cost per user <$0.01/month

## Next Steps

1. **Immediate** - Implement cost tracking infrastructure
2. **This Week** - Get streaming APIs working
3. **Next Week** - Deploy moderation mesh MVP
4. **UI Handoff** - Provide Greater team with:
   - GraphQL schema
   - WebSocket event types
   - REST API documentation
   - Cost/performance expectations

## Greater Frontend Requirements

The Greater UI will need to:
1. Display real-time cost information
2. Visualize the moderation mesh
3. Show trust relationships
4. Handle WebSocket streams
5. Provide moderation queue interface
6. Display consensus reasoning
7. Show federation debugging info
8. Render community notes
9. Display reputation scores
10. Provide admin dashboards

All APIs are designed to be consumed by both:
- Web frontend (React/Vue/Svelte)
- Mobile apps (React Native/Flutter)
- Desktop clients (Electron)
- Third-party developers

---

*This plan represents a fundamental shift in how social media infrastructure works. By making it reactive, transparent, and essentially free to operate, Lesser enables a new generation of federated applications.* 