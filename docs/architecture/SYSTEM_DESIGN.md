# Lesser - Advanced Serverless ActivityPub Design

## Overview

Lesser is a revolutionary serverless ActivityPub implementation that proves federated social media can be essentially free to operate. By combining serverless architecture with innovative features like reactive moderation and real-time cost tracking, Lesser enables a new generation of sustainable social platforms.

## Core Principles

1. **Cost Transparency** - Every operation tracks its cost in real-time
2. **Reactive Systems** - Changes propagate through event streams
3. **Federation First** - 100% ActivityPub compliant
4. **Developer Joy** - APIs that make sense, tools that delight
5. **Community Moderation** - Distributed consensus, not central control

## Architecture Overview

### Serverless Stack
- **Compute**: AWS Lambda (Go 1.21+)
- **Storage**: DynamoDB (single-table design)
- **Search**: OpenSearch Serverless + AWS Bedrock
- **Queue**: SQS for reliable delivery
- **Media**: S3 + CloudFront CDN
- **Monitoring**: CloudWatch + X-Ray
- **Deploy**: Pulumi IaC

### Cost Profile
- **Per User**: $0.01-0.05/month
- **Per 1K Activities**: ~$0.10
- **Media Storage**: $0.023/GB
- **Search Index**: ~$0.50/month base

## Core Systems

### 1. ActivityPub Implementation

#### Actor System
```go
type Actor struct {
    ID                string    `json:"id"`
    Type              string    `json:"type"`
    PreferredUsername string    `json:"preferredUsername"`
    Inbox             string    `json:"inbox"`
    Outbox            string    `json:"outbox"`
    PublicKey         PublicKey `json:"publicKey"`
    
    // Enhanced fields
    TrustScore        float64   `json:"-"` // Internal only
    Reputation        Reputation `json:"-"`
}
```

#### Activity Processing Pipeline
```
Incoming Activity → Lambda → Validation → Storage → Stream → Processors
                                ↓                      ↓
                          HTTP Signature          Moderation
                              Check                  Mesh
```

#### Supported Activities
- ✅ Create, Update, Delete
- ✅ Follow, Accept, Reject  
- ✅ Like, Announce, Undo
- ✅ Block, Flag, Move
- ✅ Add, Remove
- ✅ All standard ActivityPub types

### 2. Storage Architecture

#### DynamoDB Single Table Design
```
Primary Table Structure:
PK                          SK                          Type
ACTOR#alice                 PROFILE                     Actor
ACTOR#alice                 ACTIVITY#2024-01-20#123     Activity
OBJECT#note-456            METADATA                    Note
TIMELINE#alice#HOME        2024-01-20T10:00:00#789    TimelineEntry
MODERATION#EVENT#123       EVENT                       ModerationEvent
TRUST#alice#bob            EDGE                        TrustEdge

GSI1 (Timeline Index):
GSI1PK                     GSI1SK
USER#alice#TIMELINE        2024-01-20T10:00:00#123

GSI2 (Moderation Queue):
GSI2PK                     GSI2SK
MODERATION#PENDING         2024-01-20T10:00:00#123

GSI3 (Search Index):
GSI3PK                     GSI3SK  
SEARCH#USER               alice#alice_jones

GSI4 (Cost Tracking):
GSI4PK                     GSI4SK
COST#2024-01-20           alice#12:00:00
```

#### Event Sourcing via Streams
- DynamoDB Streams trigger Lambda functions
- Every change becomes an event
- Enables time-travel debugging
- Powers the reactive moderation mesh

### 3. Reactive Moderation Mesh

#### Event-Driven Moderation
```go
type ModerationEvent struct {
    ID         string    `json:"id"`
    Type       EventType `json:"type"`
    Target     string    `json:"target"`
    Actor      string    `json:"actor"`
    Confidence float64   `json:"confidence"`
    Evidence   []Evidence `json:"evidence"`
    
    // Propagation control
    Visibility Visibility `json:"visibility"`
    Recipients []string   `json:"recipients"`
}
```

#### Trust Graph
```go
type TrustGraph struct {
    // Edges represent trust relationships
    // Weights range from -1 (distrust) to 1 (full trust)
    // Categories: content, behavior, technical
}

// Trust propagates through the network
// High-trust users' decisions carry more weight
// Consensus emerges from the graph
```

#### Consensus Algorithm
1. Event triggers review request
2. Trust graph selects diverse reviewers
3. Reviews collected with confidence scores
4. Weighted consensus calculated
5. Decision executed and logged
6. Learnings fed back to trust graph

### 4. AI-Enhanced Features

#### Search Architecture
```
Query → AWS Comprehend (understanding) → Strategies:
  ├─ ExactMatchStrategy (DynamoDB GSI)
  ├─ FuzzySearchStrategy (OpenSearch)  
  ├─ SemanticSearchStrategy (Bedrock embeddings)
  └─ PopularityStrategy (engagement signals)
```

#### AI Services Integration
- **AWS Comprehend**: Query understanding, sentiment analysis
- **AWS Bedrock**: Semantic embeddings, AI content detection
- **AWS Rekognition**: Image moderation, NSFW detection
- **OpenSearch**: Vector search, fuzzy matching

### 5. Real-Time Cost Tracking

#### Cost Calculation
```go
type OperationCost struct {
    LambdaInvocations  int64 `json:"lambda_invocations"`
    DynamoDBReads      int64 `json:"dynamodb_reads"`
    DynamoDBWrites     int64 `json:"dynamodb_writes"`
    S3Operations       int64 `json:"s3_operations"`
    DataTransferBytes  int64 `json:"data_transfer_bytes"`
    TotalCostMicros    int64 `json:"total_cost_micros"`
}

// Every API response includes cost
type APIResponse struct {
    Data     interface{}    `json:"data"`
    Cost     *OperationCost `json:"cost"`
    Duration int64          `json:"duration_ms"`
}
```

#### Cost Optimization
- Lambda ARM Graviton2 processors
- DynamoDB on-demand pricing
- S3 intelligent tiering
- CloudFront caching
- Connection pooling

### 6. Federation Enhancements

#### Delivery Optimization
- Shared inbox support
- Delivery coalescing
- Instance health tracking
- Automatic retry with backoff
- Predictive pre-warming

#### Federation Debugging
```go
// Real-time federation monitoring
type FederationEvent struct {
    Instance   string
    Activity   string
    Status     string
    Latency    time.Duration
    Error      error
    HTTPStatus int
}
```

### 7. Developer Experience

#### GraphQL API
```graphql
type Query {
  # ActivityPub queries
  actor(id: ID!): Actor
  object(id: ID!): Object
  
  # Moderation queries
  moderationQueue: ModerationQueue
  trustGraph(actor: ID!): TrustGraph
  
  # Analytics
  instanceMetrics: Metrics
  costBreakdown(period: Period!): CostBreakdown
}

type Subscription {
  # Real-time streams
  activityStream: Activity
  moderationEvents: ModerationEvent
  costUpdates: CostUpdate
}
```

#### Debug Tools
- Time-travel replay
- Federation X-ray tracing
- Cost simulation
- Activity explain plans

## Advanced Features

### Community Notes
```go
type CommunityNote struct {
    ID        string   `json:"id"`
    ObjectID  string   `json:"object_id"`
    Content   string   `json:"content"`
    Sources   []string `json:"sources"`
    
    // Voting determines visibility
    HelpfulVotes    int     `json:"helpful_votes"`
    NotHelpfulVotes int     `json:"not_helpful_votes"`
    Score           float64 `json:"score"`
    
    // Federation support
    Federated bool `json:"federated"`
}
```

### Bot Detection
- Registration velocity tracking
- Posting pattern analysis
- AI content detection
- Network graph analysis
- Automated responses

### Portable Reputation
```go
type PortableReputation struct {
    Actor      string    `json:"actor"`
    TrustScore float64   `json:"trust_score"`
    Categories map[string]float64 `json:"categories"`
    Vouchers   []Vouch   `json:"vouchers"`
    
    // Cryptographically signed
    Signature  string    `json:"signature"`
    ValidUntil time.Time `json:"valid_until"`
}

// Queryable via .well-known/reputation
```

## Performance Characteristics

### Latency Targets
- Actor profile: <50ms
- Timeline generation: <100ms
- Activity delivery: <200ms
- Search results: <150ms
- Moderation decision: <1s

### Scaling Profile
- 0 → 10K req/s: Automatic
- No capacity planning
- No server management
- Pay only for usage

### Reliability
- 99.9% uptime target
- Multi-AZ deployment
- Automatic failover
- Self-healing

## Security Model

### Authentication
- OAuth 2.0 + JWT for clients
- HTTP Signatures for federation
- PKCE for enhanced security
- Refresh token rotation

### Authorization
- Scope-based permissions
- Actor-level access control
- Trust-weighted rate limits

### Data Protection
- Encryption at rest (DynamoDB)
- Encryption in transit (TLS)
- Private keys in AWS KMS
- PII handling compliance

## Cost Analysis

### Small Instance (100 users)
```
Lambda:     1M requests × $0.20/M = $0.20
DynamoDB:   10K writes × $1.25/M = $0.01
            50K reads × $0.25/M = $0.01
S3:         10GB × $0.023/GB = $0.23
CloudFront: 50GB × $0.085/GB = $4.25
Total:      ~$5/month ($0.05/user)
```

### Medium Instance (1,000 users)
```
Lambda:     10M requests × $0.20/M = $2
DynamoDB:   100K writes × $1.25/M = $0.13
            500K reads × $0.25/M = $0.13
S3:         100GB × $0.023/GB = $2.30
CloudFront: 500GB × $0.085/GB = $42.50
Total:      ~$50/month ($0.05/user)
```

### Comparison
| Platform | 100 Users | 1K Users | 10K Users |
|----------|-----------|----------|-----------|
| Mastodon | $50-100   | $200-500 | $1K-5K    |
| Lesser   | $5        | $50      | $500      |
| Savings  | 90-95%    | 75-90%   | 50-90%    |

## Implementation Status

### ✅ Complete (~98%)
- Full ActivityPub protocol
- Mastodon API compatibility  
- OAuth 2.0 authentication
- Media handling with CDN
- Advanced search with AI
- Push notifications
- Polls, filters, mutes
- Lists management
- Federation with all platforms

### 🚧 In Progress
- Notifications system (final feature!)
- Reactive moderation mesh
- Community notes
- Greater UI

### 🔮 Planned
- Bot detection network
- Reputation system
- Plugin marketplace
- Multi-language UI

## Deployment

### Prerequisites
- AWS Account
- Domain name
- Pulumi CLI
- Go 1.21+

### One-Command Deploy
```bash
cd infra
pulumi up
# That's it! Your instance is live.
```

### Configuration
```yaml
domain: yourdomain.com
region: us-east-1
features:
  moderation_mesh: true
  ai_search: true
  community_notes: true
  cost_tracking: true
```

## Migration Path

### From Mastodon
```bash
lesser import mastodon --archive=backup.tar.gz
# Preserves all data, followers, posts
```

### Data Portability
- Export to ActivityPub JSON
- Export to static site
- Export to SQLite
- Full data ownership

## Monitoring & Operations

### CloudWatch Dashboards
1. System Health
2. Cost Tracking  
3. Federation Status
4. Moderation Queue
5. User Activity

### Alerts
- Cost anomalies
- Federation failures
- Moderation backlog
- Performance degradation

### Maintenance
- Zero downtime deployments
- Automatic scaling
- Self-healing infrastructure
- No server management

## Future Directions

### Technical Roadmap
1. Edge computing with Lambda@Edge
2. Multi-region deployment
3. IPFS integration for media
4. Blockchain-backed reputation
5. Decentralized identity

### Feature Roadmap
1. Video streaming support
2. E2E encryption
3. Collaborative editing
4. Voice/video calls
5. Virtual events

## Conclusion

Lesser represents a fundamental shift in how social media infrastructure works. By making it reactive, transparent, and essentially free to operate, we enable a new generation of sustainable, community-owned social platforms.

The combination of serverless architecture, reactive moderation, and cost transparency creates a platform that is:
- **Sustainable** - Costs scale with usage
- **Transparent** - Every decision is logged
- **Resilient** - No single point of failure
- **Accessible** - Anyone can run an instance

Lesser proves that the future of social media is federated, and that federation doesn't have to be expensive. 