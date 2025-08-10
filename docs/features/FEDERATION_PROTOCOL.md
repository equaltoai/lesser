# Lesser Federation Protocol Implementation

Lesser implements comprehensive ActivityPub federation with advanced features like intelligent routing, enhanced retry mechanisms, and production-grade reliability. This document details the complete federation architecture and implementation.

## Overview

Lesser's federation system enables seamless communication with the entire Fediverse (10M+ users across thousands of instances). The implementation prioritizes reliability, cost-efficiency, and standards compliance while adding intelligent enhancements for better delivery and discovery.

## Core ActivityPub Implementation

### Supported Activity Types

Lesser implements all standard ActivityPub activities with full federation support:

#### Core Activities
- **Create**: New posts, replies, media
- **Update**: Content edits, profile changes
- **Delete**: Content removal with tombstones
- **Follow**: Following relationships
- **Accept**: Follow request approval
- **Reject**: Follow request denial
- **Like**: Post reactions/favorites
- **Announce**: Boosts/retweets
- **Undo**: Activity reversal (unfavorite, unfollow, etc.)
- **Block**: User blocking
- **Flag**: Content reporting
- **Add/Remove**: Collection management

#### Implementation Reference
```go
// File: pkg/activitypub/types.go
type Activity struct {
    BaseObject
    Actor      string `json:"actor"`
    Object     any    `json:"object"`
    Target     string `json:"target,omitempty"`
    Origin     string `json:"origin,omitempty"`
    Instrument string `json:"instrument,omitempty"`
}
```

### Object Types

#### Notes (Posts)
```go
type Note struct {
    BaseObject
    Content        string       `json:"content"`
    AttributedTo   string       `json:"attributedTo"`
    Attachment     []Attachment `json:"attachment,omitempty"`
    Tag            []Tag        `json:"tag,omitempty"`
    ConversationID string       `json:"conversationId,omitempty"`
    Visibility     string       `json:"_:visibility,omitempty"`
}
```

#### Actors (Users/Services)
```go
type Actor struct {
    BaseObject
    PreferredUsername string     `json:"preferredUsername"`
    Name              string     `json:"name,omitempty"`
    Inbox             string     `json:"inbox"`
    Outbox            string     `json:"outbox"`
    PublicKey         *PublicKey `json:"publicKey,omitempty"`
    Endpoints         *Endpoints `json:"endpoints,omitempty"`
    // ... additional fields
}
```

## Inbox Processing Architecture

### Inbox Handler
**File**: `cmd/inbox/main.go`

The inbox receives and processes all incoming ActivityPub activities:

#### Security Verification
1. **HTTP Signature Verification**: All activities cryptographically verified
2. **Actor Validation**: Confirm sender authenticity
3. **Content Sanitization**: Prevent XSS and injection attacks
4. **Rate Limiting**: Per-domain delivery limits

#### Processing Pipeline
```go
type InboxHandler struct {
    db                           core.DB
    actorRepository              *repositories.ActorRepository
    activityRepository           *repositories.ActivityRepository
    relationshipRepository       *repositories.RelationshipRepository
    signatureService             *federation.SignatureService
    // ... other dependencies
}
```

#### Activity Processing Flow
1. **Authentication**: HTTP signature verification
2. **Validation**: Schema and content validation
3. **Actor Resolution**: Fetch/cache actor if needed
4. **Processing**: Activity-specific business logic
5. **Storage**: Persist changes to DynamoDB
6. **Propagation**: Update timelines and indexes
7. **Notifications**: Generate user notifications

## Outbox Publishing

### Activity Publishing
Activities are published through both REST API and GraphQL endpoints with consistent behavior:

#### REST Integration
```go
// File: cmd/api/lift/statuses.go
// Activities are published with proper addressing and federation
```

#### GraphQL Integration
GraphQL mutations automatically generate corresponding ActivityPub activities for federation.

### Publishing Flow
1. **Activity Creation**: Generate proper ActivityPub JSON
2. **Addressing**: Determine recipients (To, CC, BTo, BCC)
3. **Signing**: HTTP signature generation
4. **Delivery**: Queue for asynchronous delivery
5. **Retry**: Enhanced retry with exponential backoff

## Enhanced Delivery System

### Intelligent Route Manager

**File**: `pkg/federation/routing/route_manager.go`

The route manager optimizes federation delivery with smart routing:

#### Features
- **Instance Health Monitoring**: Track remote instance availability
- **Circuit Breaker**: Prevent cascade failures
- **Load Balancing**: Distribute delivery across healthy instances  
- **Route Optimization**: Choose best delivery paths
- **Cost Tracking**: Monitor federation expenses

#### Configuration
```go
type ManagerConfig struct {
    RoutingConfig        *types.RoutingConfig
    OptimizerConfig      *OptimizerConfig
    CircuitBreakerConfig *models.CircuitBreakerConfig
    CacheTTL             time.Duration
    FederationStore      federation.FederationStorage
}
```

### Enhanced Retry Mechanism

**File**: `pkg/federation/enhanced_retry.go`

Polynomial retry with intelligent failure handling:

#### Retry Strategy
```go
type EnhancedRetryMessage struct {
    DeliveryID        string                `json:"delivery_id"`
    Activity          *activitypub.Activity `json:"activity"`
    RetryCount        int                   `json:"retry_count"`
    MaxRetries        int                   `json:"max_retries"`
    RetryPolicy       string                `json:"retry_policy"`
    MaxRetryDuration  time.Duration         `json:"max_retry_duration"`
    FailedInboxes     map[string]string     `json:"failed_inboxes,omitempty"`
}
```

#### Retry Policies
- **Linear**: Fixed intervals for non-critical activities
- **Exponential**: Increasing intervals with jitter
- **Polynomial**: Optimized for federation patterns
- **Circuit Breaker**: Skip failing instances temporarily

## Discovery and WebFinger

### WebFinger Implementation
**Endpoint**: `/.well-known/webfinger`

RFC 7033 compliant WebFinger for actor discovery:

```json
{
  "subject": "acct:username@instance.com",
  "links": [
    {
      "rel": "self",
      "type": "application/activity+json",
      "href": "https://instance.com/users/username"
    }
  ]
}
```

### Actor Discovery
1. **WebFinger Lookup**: Find actor URI from username@domain
2. **Actor Fetch**: Retrieve full actor document
3. **Key Caching**: Cache public keys for signature verification
4. **Profile Sync**: Update cached actor information

## HTTP Signatures

### Enhanced Signature Support

**File**: `pkg/federation/httpsig_enhanced.go`

Supports both legacy and modern HTTP signature standards:

#### Signature Verification
```go
type EnhancedHTTPSignature struct {
    HTTPSignature
    Created int64 // Unix timestamp (for hs2019)
    Expires int64 // Unix timestamp (for hs2019)
}
```

#### Supported Algorithms
- **RSA-SHA256**: Legacy compatibility
- **ECDSA**: Modern elliptic curve signatures
- **Ed25519**: High-performance signatures
- **HS2019**: Modern signature suite

### Signature Generation
1. **Key Selection**: Choose appropriate signing algorithm
2. **Header Selection**: Include required headers (Date, Host, Digest)
3. **Signature String**: Build canonical signature string
4. **Signing**: Generate cryptographic signature
5. **Header Attachment**: Add signature to HTTP request

## Collections and Pagination

### Actor Collections
Standard ActivityPub collections with cursor-based pagination:

#### Supported Collections
- **Outbox**: Actor's published activities
- **Inbox**: Actor's received activities (private)
- **Following**: Accounts the actor follows
- **Followers**: Accounts following the actor
- **Liked**: Actor's favorited content
- **Featured**: Pinned posts

#### Collection Format
```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "https://instance.com/users/username/followers",
  "type": "OrderedCollection",
  "totalItems": 1234,
  "first": "https://instance.com/users/username/followers?page=1"
}
```

### Pagination Implementation
- **Cursor-based**: Efficient pagination for large collections
- **Privacy Respecting**: Honor follower visibility settings
- **Performance Optimized**: Minimal database queries

## Content Addressing

### Visibility Levels
Lesser supports comprehensive content visibility:

#### Visibility Types
- **Public**: Visible to everyone, appears in timelines
- **Unlisted**: Visible to everyone, doesn't appear in public timelines
- **Followers**: Only visible to followers
- **Direct**: Only visible to mentioned users

#### Addressing Implementation
```go
// Public post addressing
To: ["https://www.w3.org/ns/activitystreams#Public"]
CC: ["https://instance.com/users/username/followers"]

// Followers-only post addressing
To: ["https://instance.com/users/username/followers"]
CC: []

// Direct message addressing
To: ["https://recipient.com/users/recipient"]
CC: []
```

## Federation Analytics

### Delivery Tracking
**File**: `pkg/federation/analytics_aggregator.go`

Comprehensive federation metrics:

#### Tracked Metrics
- **Delivery Success Rate**: Per-instance delivery statistics
- **Latency**: Response times by instance
- **Error Classification**: Categorized failure reasons
- **Cost Analysis**: Federation delivery costs
- **Volume Analysis**: Activity volume patterns

#### Cost Tracking
```go
// Federation cost categories
const (
    costCategoryHTTPRequests = "http_requests"
    costCategoryRetries      = "retries"
    costCategoryStorage      = "storage"
    costCategoryQueue        = "queue"
)
```

## Instance Health Monitoring

### Health Checks
Continuous monitoring of remote instance health:

#### Health Indicators
- **Response Time**: Average latency to instance
- **Success Rate**: Percentage of successful deliveries
- **Error Patterns**: Classification of error types
- **Availability**: Instance uptime tracking

#### Circuit Breaker
**File**: `pkg/federation/routing/circuit_breaker.go`

Prevents cascade failures:
- **Open**: Skip failing instances
- **Half-Open**: Test instance recovery
- **Closed**: Normal delivery
- **Threshold-Based**: Configurable failure thresholds

## Rate Limiting

### Federation Rate Limits
Per-domain rate limiting for federation:

#### Limit Categories
- **Inbox Delivery**: Limits on sending activities
- **Actor Fetching**: Limits on actor discovery
- **Media Fetching**: Limits on media downloads
- **Collection Crawling**: Limits on collection pagination

#### Implementation
```go
// Rate limit headers
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 847
X-RateLimit-Reset: 1640995200
```

## Error Handling and Resilience

### Error Classification
**File**: `pkg/common/error_constants.go`

Structured error handling for federation:

#### Error Types
- **Temporary**: Retry with backoff
- **Permanent**: Don't retry, log for analysis
- **Rate Limit**: Respect retry-after headers
- **Authentication**: Re-fetch actor keys
- **Network**: Apply circuit breaker logic

### Failure Recovery
1. **Graceful Degradation**: Continue operation during partial failures
2. **Retry Logic**: Intelligent retry with exponential backoff
3. **Circuit Breaking**: Prevent cascade failures
4. **Manual Recovery**: Admin tools for handling edge cases

## Configuration

### Environment Variables
```bash
# Federation settings
DOMAIN_NAME=your-instance.com
PRIVATE_KEY_SECRET=<activity-pub-signing-key>
FEDERATION_TIMEOUT_SECONDS=30
MAX_FEDERATION_RETRIES=5

# Route management
FEDERATION_CIRCUIT_BREAKER_THRESHOLD=10
FEDERATION_HEALTH_CHECK_INTERVAL=300
FEDERATION_ROUTE_CACHE_TTL=3600

# Rate limiting
FEDERATION_RATE_LIMIT_PER_DOMAIN=1000
FEDERATION_RATE_LIMIT_WINDOW=3600
```

### DynamoDB Configuration
Federation uses optimized DynamoDB patterns:

#### Key Patterns
- **Actors**: `ACTOR#domain#username`
- **Activities**: `ACTIVITY#id`
- **Relationships**: `FOLLOW#follower#following`
- **Public Keys**: `PUBKEY#actor_uri`

#### GSI Indexes
- **GSI1**: Actor discovery by domain
- **GSI2**: Activity lookup by type
- **GSI3**: Relationship queries
- **GSI4**: Timeline construction

## Testing Federation

### Test Tools
**File**: `cmd/federation-delivery/retry_test.go`

Comprehensive federation testing:

#### Unit Tests
- HTTP signature verification
- Activity parsing and validation
- Retry logic behavior
- Route optimization

#### Integration Tests
- End-to-end federation flow
- Cross-instance communication
- Error handling scenarios
- Performance benchmarks

### Testing Commands
```bash
# Run federation tests
make test-federation

# Test specific instance federation  
curl -X POST https://your-instance.com/inbox \
  -H "Content-Type: application/activity+json" \
  -H "Signature: ..." \
  -d @test-activity.json

# Check federation health
curl https://your-instance.com/.well-known/nodeinfo
```

## Troubleshooting

### Common Issues

#### Signature Verification Failures
```bash
# Check actor public key
curl -H "Accept: application/activity+json" \
  https://remote-instance.com/users/username

# Verify signature generation
curl -v -X POST https://your-instance.com/inbox \
  -H "Signature: ..." -d @activity.json
```

#### Delivery Failures  
```bash
# Check instance health
curl -H "Accept: application/activity+json" \
  https://remote-instance.com/users/username

# Monitor delivery queues
aws sqs get-queue-attributes --queue-url <queue-url>
```

#### Actor Discovery Issues
```bash
# Test WebFinger
curl "https://remote-instance.com/.well-known/webfinger?resource=acct:user@domain.com"

# Test actor endpoint
curl -H "Accept: application/activity+json" \
  https://remote-instance.com/users/username
```

### Debugging Tools

#### Logs and Metrics
- **CloudWatch Logs**: Structured federation logs
- **Custom Metrics**: Federation-specific metrics
- **Tracing**: X-Ray integration for request tracing

#### Admin Tools
- **Federation Health Dashboard**: Instance status overview
- **Delivery Queue Monitor**: Queue depth and processing
- **Error Analysis**: Error classification and trends

## Best Practices

### Performance Optimization
1. **Batch Deliveries**: Group activities by instance
2. **Connection Pooling**: Reuse HTTP connections  
3. **Compression**: Enable gzip for large payloads
4. **Caching**: Cache actor documents and public keys

### Reliability Patterns
1. **Idempotency**: Handle duplicate activities gracefully
2. **Timeouts**: Set appropriate HTTP timeouts
3. **Circuit Breakers**: Prevent cascade failures
4. **Dead Letter Queues**: Handle permanent failures

### Security Considerations
1. **Signature Verification**: Always verify HTTP signatures
2. **Content Sanitization**: Sanitize all incoming content
3. **Rate Limiting**: Prevent abuse from federated instances
4. **Access Control**: Respect privacy and visibility settings

## Federation Standards Compliance

Lesser implements and extends ActivityPub standards:

### Core Standards
- **ActivityPub W3C Recommendation**: Full compliance
- **ActivityStreams 2.0**: Complete vocabulary support
- **HTTP Signatures**: Draft-ietf-httpbis-message-signatures
- **WebFinger RFC 7033**: Actor discovery
- **JSON-LD**: Proper context handling

### Mastodon Extensions
- **Mastodon API**: Extensive compatibility
- **Custom Emojis**: Full support
- **Media Attachments**: Complete implementation
- **Content Warnings**: Privacy-respecting handling

### Lesser Extensions
- **Cost Tracking**: Federation cost analysis
- **Smart Routing**: Intelligent delivery optimization
- **Enhanced Retry**: Polynomial retry algorithms
- **Health Monitoring**: Instance reliability tracking

Lesser's federation implementation provides enterprise-grade reliability while maintaining full compatibility with the existing Fediverse ecosystem. The intelligent enhancements provide better performance and cost-efficiency without breaking interoperability.