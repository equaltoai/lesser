# AI Assistant Prompt for Lesser Development

## 🚀 LESSER 2.0: Revolutionary Serverless ActivityPub Platform

**Status**: Mastodon API Complete ✅ | Now Building the Future of Federation 🔮

You are helping develop Lesser, a revolutionary serverless ActivityPub platform that demonstrates federated social media can be essentially free to operate while providing superior features through serverless architecture and reactive systems.

**Phase 1 Complete**: Full Mastodon API compatibility achieved! Now advancing to Phase 2: Building features no other ActivityPub server has attempted.

## Architecture Evolution

Lesser is evolving from a Mastodon-compatible server to a platform that fundamentally changes how social media infrastructure works:

### Core Principles
1. **Everything is an Event** - All actions trigger DynamoDB streams
2. **Reactive by Default** - Changes propagate through Lambda functions  
3. **Cost-Conscious** - Every operation tracks its cost in real-time
4. **Federation-First** - Built for interoperability from day one
5. **Developer Experience** - APIs that make sense, debug tools that delight

### New Architecture Components
- **Real-Time Cost Tracking**: Every API response includes cost metadata
- **Activity Streams**: WebSocket/SSE for real-time monitoring
- **Reactive Moderation Mesh**: Consensus-based moderation with trust graphs
- **GraphQL Gateway**: Modern API alongside REST
- **AI Integration Layer**: AWS Bedrock, Comprehend, and Rekognition

## Current Implementation Roadmap

### 📊 Phase 1: Enhanced Core Platform (Weeks 1-2) - CURRENT FOCUS
**Goal**: Add real-time cost tracking and activity streaming

🔲 **1.1 Real-Time Cost Tracking**
- Instrument all storage operations with cost calculation
- Add `X-Cost-*` headers to all API responses
- Create cost aggregation Lambda
- Store cost history with analytics

🔲 **1.2 Activity Stream API**
- WebSocket endpoint at `wss://instance.com/api/v1/streaming`
- SSE endpoint at `https://instance.com/api/v1/streaming/events`
- Stream federation activities, moderation decisions, cost alerts
- Real-time system metrics

🔲 **1.3 Enhanced Metrics API**
- Current metrics (active users, requests/min, latency)
- Daily aggregates (costs, posts, interactions)
- Predictive analytics (monthly cost, storage growth)

### 🛡️ Phase 2: Reactive Moderation Mesh (Weeks 3-4)
**Goal**: Build consensus-based moderation with trust graphs

🔲 **2.1 Moderation Event System**
- Event-driven moderation pipeline
- Moderation events with confidence scores
- Evidence tracking and audit trail

🔲 **2.2 Trust Graph Engine**
- Directional trust relationships
- Category-based trust (content, behavior, technical)
- Trust score propagation algorithms

🔲 **2.3 Consensus Engine**
- Weighted review aggregation
- Configurable consensus thresholds
- Sub-second decision making

🔲 **2.4 Moderation API**
- Flag content with evidence
- Review queue with priority scoring
- Consensus visualization

### 🛠️ Phase 3: Developer Experience (Week 5)
**Goal**: Make Lesser a joy to develop against

🔲 **3.1 GraphQL Gateway**
- Unified query interface
- Real-time subscriptions
- Cost-aware query planning

🔲 **3.2 Debug Endpoints**
- Federation debugging tools
- Object explanation with storage details
- Activity replay for testing

🔲 **3.3 Testing Utilities**
- Test data generation
- Federation test harness
- Performance benchmarking

### ⚡ Phase 4: Advanced Features (Weeks 6-7)
**Goal**: Features no other ActivityPub server has

🔲 **4.1 Portable Reputation API**
- Cryptographically signed reputation
- Cross-instance trust portability
- Vouch system for new users

🔲 **4.2 Community Notes**
- Crowdsourced context on posts
- Voting and visibility algorithms
- Multi-language support

🔲 **4.3 AI Integration**
- Sentiment analysis
- Toxicity detection
- AI-generated content detection
- Spam probability scoring

🔲 **4.4 Plugin System**
- Lambda-based plugins
- Hook into activity pipeline
- Custom moderation rules

### 🚄 Phase 5: Performance & Scale (Week 8)
**Goal**: <50ms responses at any scale

🔲 **5.1 Caching Strategy**
- CloudFront + DAX + Lambda memory
- Cache warming for hot paths
- Intelligent TTL management

🔲 **5.2 Timeline Optimizations**
- Pre-computed timeline chunks
- Parallel generation
- Hybrid fan-out strategies

## Project Structure
- `/cmd/api/` - API Lambda handlers (includes media handling)
- `/cmd/search-indexer/` - OpenSearch indexing Lambda
- `/cmd/activity-processor/` - Activity processing Lambda
- `/pkg/storage/dynamodb/` - DynamoDB storage layer
- `/pkg/activitypub/` - ActivityPub types and logic
- `/pkg/mastodon/` - Mastodon API converters and services
- `/pkg/cost/` - Cost tracking infrastructure (NEW)
- `/pkg/moderation/` - Moderation mesh system (NEW)
- `/pkg/trust/` - Trust graph engine (NEW)
- `/infra/` - Pulumi infrastructure code
- `test_api_automated.py` - API test script
- `test_media_urls.py` - Media CDN verification script

## Implementation Status

### ✅ Foundation Complete (Phase 1 - Mastodon Compatibility)
- OAuth 2.0 authentication with scopes
- Full Mastodon API implementation
- ActivityPub federation
- Advanced AI-powered search
- Media CDN with CloudFront
- Push notifications
- Lists, polls, filters, mutes
- Complete test suite

### 🚧 Current Sprint: Real-Time Cost Tracking (Week 1)

**This Week's Goals**:
1. **Cost Instrumentation**
   - [ ] Create `pkg/cost/tracker.go` with AWS pricing rates
   - [ ] Instrument DynamoDB operations in storage layer
   - [ ] Add cost calculation to Lambda invocations
   - [ ] Track S3 operations and data transfer

2. **API Integration**
   - [ ] Add cost middleware to all handlers
   - [ ] Include `X-Cost-*` headers in responses
   - [ ] Return cost in JSON response metadata
   - [ ] Create `/api/v1/instance/costs` endpoint

3. **Cost Analytics**
   - [ ] Store operation costs in DynamoDB
   - [ ] Create cost aggregation Lambda
   - [ ] Build daily/monthly roll-ups
   - [ ] Add predictive cost modeling

### 🎯 Next Sprints

**Week 2: Activity Streams**
- WebSocket handler implementation
- SSE endpoint for simpler clients
- Event routing from DynamoDB streams
- Real-time metrics dashboard data

**Weeks 3-4: Reactive Moderation Mesh**
- Event-driven moderation pipeline
- Trust graph storage and queries
- Consensus engine implementation
- Moderation queue APIs

**Week 5: Developer Experience**
- GraphQL schema and resolvers
- Debug endpoints for federation
- Testing utilities and data generators

## Implementation Guidelines

### 🏗️ Building Cost-Conscious Infrastructure

When implementing cost tracking:
1. **Granular Tracking** - Track costs at the operation level, not just request level
2. **Async Aggregation** - Use DynamoDB streams for cost roll-ups to avoid blocking
3. **Predictive Modeling** - Use historical data to predict monthly costs
4. **Cost Alerts** - Trigger notifications when costs exceed thresholds

### 🔄 Event-Driven Architecture

All new features should follow the reactive pattern:
1. **Write to DynamoDB** - All state changes go through DynamoDB
2. **Stream Processing** - Lambda functions react to DynamoDB streams
3. **Parallel Execution** - Multiple handlers can process the same event
4. **Eventually Consistent** - Embrace async processing for scalability

### 📊 Metrics and Observability

Every new feature must include:
1. **CloudWatch Metrics** - Custom metrics for feature-specific monitoring
2. **X-Ray Tracing** - Distributed tracing for debugging
3. **Cost Attribution** - Track costs per feature/operation
4. **Performance Baselines** - Establish and monitor performance targets

## Development Workflow

### Building New Features

```bash
# 1. Create feature branch
git checkout -b feature/cost-tracking

# 2. Implement in small, testable increments
# Start with core logic, then handlers, then integration

# 3. Test locally with SAM
sam local start-api

# 4. Run integration tests
python test_cost_tracking.py

# 5. Deploy to dev environment
cd infra && pulumi up -s dev

# 6. Validate in production-like environment
# Monitor CloudWatch logs and metrics
```

### Cost Tracking Implementation Example

```go
// pkg/cost/tracker.go
package cost

import (
    "context"
    "sync/atomic"
)

type Tracker struct {
    dynamoReads  atomic.Int64
    dynamoWrites atomic.Int64
    lambdaMs     atomic.Int64
    s3Gets       atomic.Int64
    s3Puts       atomic.Int64
    dataTransfer atomic.Int64
}

func (t *Tracker) TrackDynamoRead(items int) {
    t.dynamoReads.Add(int64(items))
}

func (t *Tracker) CalculateCost() *OperationCost {
    // Use current AWS pricing
    return &OperationCost{
        DynamoDBReads:    t.dynamoReads.Load(),
        DynamoDBWrites:   t.dynamoWrites.Load(),
        TotalCostMicros:  t.calculateTotal(),
    }
}
```

### Testing Cost Features

```python
# test_cost_tracking.py
def test_cost_headers():
    """Verify cost tracking headers are present"""
    response = client.get("/api/v1/accounts/verify_credentials")
    assert "X-Cost-Total-Micros" in response.headers
    assert "X-Cost-DynamoDB-Reads" in response.headers
    
def test_cost_aggregation():
    """Verify costs are aggregated correctly"""
    # Make several API calls
    # Check /api/v1/instance/costs endpoint
    # Verify aggregation is accurate
```

## Architecture Decisions

### Why Event-Driven?
- **Scalability**: Each component scales independently
- **Resilience**: Failures in one component don't cascade
- **Cost**: Only pay for actual processing time
- **Flexibility**: Easy to add new event processors

### Why Cost Tracking?
- **Transparency**: Users see the real cost of social media
- **Optimization**: Identify and optimize expensive operations
- **Budgeting**: Instances can set and enforce cost limits
- **Innovation**: Enables new cost-based features

### Why Moderation Mesh?
- **Decentralized**: No single point of failure or control
- **Transparent**: All decisions are auditable
- **Flexible**: Instances choose their own policies
- **Collaborative**: Learn from the network's decisions

## Success Metrics

### Phase 1 Success Criteria (Weeks 1-2)
- [ ] Cost data available on 100% of API calls
- [ ] <1ms overhead from cost tracking
- [ ] Activity stream handling 1000 events/second
- [ ] Cost prediction accuracy within 10%

### Phase 2 Success Criteria (Weeks 3-4)
- [ ] Moderation decisions in <1 second
- [ ] Trust graph queries in <50ms
- [ ] 95% consensus achievement rate
- [ ] Zero false positive removals

### Overall Project Success
- [ ] <$0.01/month per active user
- [ ] <50ms response times at p99
- [ ] 99.99% uptime
- [ ] Developer adoption from major clients

## Key Differentiators

Lesser 2.0 is not just another ActivityPub server:

1. **Cost Transparency** - First server to show real costs per operation
2. **Reactive Moderation** - Consensus-based decisions, not dictatorial
3. **Developer-First** - GraphQL, debug tools, comprehensive SDKs
4. **AI-Native** - Built-in AI services, not bolted on
5. **Truly Serverless** - Scales to zero, scales to millions

## Next Steps

**Immediate Priority**: Implement cost tracking infrastructure
1. Create `pkg/cost/tracker.go` with operation tracking
2. Add middleware to API handlers
3. Deploy cost aggregation Lambda
4. Update tests to verify cost headers

**This Week**: Complete Phase 1.1 (Cost Tracking)
- All API responses include cost metadata
- Cost history stored and queryable
- Basic cost analytics available
- Documentation updated

Remember: We're not just building features, we're demonstrating that federated social media can be essentially free while providing superior functionality. Every line of code should reflect this mission.

---

*Lesser 2.0: Making the impossible inevitable in federated social media.* 