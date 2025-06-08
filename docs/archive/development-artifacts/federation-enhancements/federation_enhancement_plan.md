# Lesser Federation Enhancement Plan

## 🎯 Mission: Address Real Federation Community Needs

Based on analysis of Mastodon community requests and federation pain points, this plan outlines how Lesser can become the most federation-friendly ActivityPub implementation.

## 📊 Community Pain Points Analysis

### Top Unmet Needs (from Mastodon community)
1. **Quote Posts with Safety** (#1 request for years)
2. **Hashtag Following** (basic feature gap)
3. **Thread Synchronization** (content discovery)
4. **Severed Relationships API** (federation breaks)
5. **Better Media Handling** (attachments, audio)
6. **Enhanced Migration** (account portability)

## 🚀 Phase 1: Core Federation Features (Weeks 1-4)

### 1. Quote Posts with Safety Controls

**Implementation in `pkg/activitypub/types.go`:**
```go
type QuoteNote struct {
    Note
    QuoteURL          string            `json:"quoteUrl"`
    Quoteable         bool              `json:"_:quoteable"`
    QuoteNotifications bool             `json:"_:quoteNotifications"`
    QuoteContext      *QuoteContext     `json:"_:quoteContext,omitempty"`
}

type QuoteContext struct {
    OriginalAuthor    string    `json:"originalAuthor"`
    QuoteCount        int       `json:"quoteCount"`
    AllowWithdrawal   bool      `json:"allowWithdrawal"`
}
```

**Storage Schema (DynamoDB):**
```go
// New GSI: quotes-by-target
// PK: targetNoteID
// SK: quoteNoteID
// Attributes: quoterID, timestamp, withdrawn
```

**GraphQL Mutations:**
```graphql
mutation {
  createQuoteNote(input: {
    content: "My thoughts on this..."
    quoteUrl: "https://example.com/@user/12345"
    quoteable: true
    visibility: PUBLIC
  }) {
    id
    url
    quoteContext {
      originalAuthor
      quoteAllowed
    }
  }
}
```

### 2. Hashtag Following & Notifications

**New Package: `pkg/hashtags/following.go`:**
```go
type HashtagFollow struct {
    UserID      string
    Hashtag     string
    CreatedAt   time.Time
    NotifyLevel string // "all", "mutuals", "none"
}

type HashtagNotificationPreference struct {
    UserID               string
    Hashtag             string
    NotifyFromFollowing bool
    NotifyFromMutuals   bool
    MinimumEngagement   int
}
```

**Federation Protocol Extension:**
```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "type": "Follow",
  "actor": "https://example.com/users/alice",
  "object": {
    "type": "Hashtag",
    "name": "#golang",
    "href": "https://example.com/tags/golang"
  }
}
```

### 3. Thread Synchronization

**Enhanced in `pkg/federation/sync.go`:**
```go
type ThreadSyncRequest struct {
    ConversationID string
    OriginServer   string
    Depth          int
    IncludeBoosts  bool
    IncludeReplies bool
}

func (f *FederationService) SyncCompleteThread(ctx context.Context, req ThreadSyncRequest) error {
    // Fetch missing context from origin
    // Update local thread representation
    // Maintain conversation integrity
}
```

### 4. Severed Relationships API

**New Package: `pkg/federation/relationships/severed.go`:**
```go
type SeveredRelationship struct {
    ID               string
    LocalInstance    string
    RemoteInstance   string
    Reason          SeveranceReason
    AffectedFollows []AffectedFollow
    Timestamp       time.Time
    Reversible      bool
}

type AffectedFollow struct {
    LocalUser    string
    RemoteUser   string
    Direction    string // "following", "follower", "mutual"
    LastActivity time.Time
}
```

**REST API Endpoint:**
```go
// GET /api/v1/severed_relationships
// Returns list of broken federation relationships
// Compatible with Mastodon API proposal
```

## 🔄 Phase 2: Enhanced Federation (Weeks 5-8)

### 5. Advanced Moderation Federation

**Leverage Lesser's Trust System:**
```go
type FederatedModerationReport struct {
    Report
    OriginInstance     string
    TrustScore        float64
    ConsensusRequired bool
    CrossInstanceVotes map[string]ModerationVote
}

type ModerationConsensus struct {
    ReportID      string
    Participants  []string // Instance domains
    Decision      string
    TrustWeighted bool
}
```

### 6. Media Processing Federation

**Enhanced Media Handling:**
```go
type FederatedMediaAttachment struct {
    MediaAttachment
    OriginURL        string
    StreamingAllowed bool
    CostEstimate     float64
    FocalPoint       *FocalPoint
    Alternates       []MediaAlternate // Different resolutions
}
```

### 7. Instance Capability Exchange

**New Protocol Extension:**
```json
{
  "type": "InstanceCapabilities",
  "capabilities": {
    "quotePosts": true,
    "communityNotes": true,
    "hashtagFollowing": true,
    "costTracking": true,
    "aiSearch": true,
    "trustScoring": true
  },
  "limits": {
    "maxPostLength": 5000,
    "maxMediaSize": 100000000,
    "rateLimit": "1000/hour"
  },
  "costTransparency": {
    "averagePostCost": 0.0001,
    "averageMediaCost": 0.001
  }
}
```

## 🌟 Phase 3: Lesser Differentiators (Weeks 9-12)

### 8. Cost-Aware Federation

**Unique to Lesser's Architecture:**
```go
type FederationCostPolicy struct {
    RemoteInstance     string
    MaxDailyCost      float64
    CurrentDailyCost  float64
    ThrottleThreshold float64
    Actions           []CostAction
}

type CostAction struct {
    Threshold float64
    Action    string // "throttle", "media_only", "text_only", "suspend"
}
```

### 9. Community Notes Federation

**Cross-Instance Fact Checking:**
```go
type FederatedCommunityNote struct {
    CommunityNote
    OriginInstance    string
    CrossInstanceVotes map[string]NoteVote
    TrustWeighted     bool
    ConsensusScore    float64
}

// Federation protocol for community notes
{
  "type": "CommunityNote",
  "inReplyTo": "https://remote.instance/@user/post",
  "content": "This claim needs context...",
  "evidenceLinks": ["https://..."],
  "trustScore": 0.85
}
```

### 10. AI-Powered Cross-Instance Search

**Semantic Search Federation:**
```go
type FederatedSearchQuery struct {
    Query            string
    SearchVectors    []float64 // Embeddings
    TargetInstances  []string
    LanguageHint     string
    MaxResults       int
    CostBudget       float64
}
```

## 📈 Implementation Roadmap

### Month 1: Foundation
- [x] Week 1-2: Quote posts with safety
- [x] Week 3-4: Hashtag following

### Month 2: Core Features  
- [ ] Week 5-6: Thread synchronization
- [ ] Week 7-8: Severed relationships

### Month 3: Differentiators
- [ ] Week 9-10: Cost-aware federation
- [ ] Week 11-12: Community notes federation

## 🎯 Success Metrics

### Technical Metrics
- Federation reliability > 99.9%
- Thread sync completeness > 95%
- Quote post adoption rate
- Hashtag follow usage

### Community Metrics
- Instance adoption rate
- User satisfaction scores
- Federation partnership growth
- Community notes accuracy

## 🔧 Technical Requirements

### Infrastructure Needs
1. Enhanced DynamoDB indexes for relationships
2. S3 for media streaming cache
3. Lambda for thread sync workers
4. EventBridge for federation events

### Protocol Extensions
1. Quote post vocabulary
2. Hashtag following activities
3. Cost transparency headers
4. Community notes federation

## 🚀 Competitive Advantages

### vs. Mastodon
- ✅ Quote posts (finally!)
- ✅ Better thread sync
- ✅ Cost transparency
- ✅ AI-powered search

### vs. Other Implementations
- ✅ Serverless scalability
- ✅ Trust-based moderation
- ✅ Community notes
- ✅ Zero baseline costs

## 📊 Resource Allocation

### Development Team
- 2 engineers on protocol extensions
- 1 engineer on storage layer
- 1 engineer on GraphQL integration
- 1 engineer on testing/QA

### Timeline
- 12 weeks total
- 4 sprints of 3 weeks each
- Weekly federation testing
- Bi-weekly community feedback

## 🎉 Expected Outcomes

1. **Most Requested Features**: Quote posts and hashtag following
2. **Better Federation**: Reliable thread sync and relationships
3. **Unique Value**: Cost transparency and community notes
4. **Community Trust**: Addressing real pain points

## 📝 Next Steps

1. **Immediate**: Start quote post implementation
2. **Week 1**: Design federation protocol extensions
3. **Week 2**: Begin hashtag following system
4. **Ongoing**: Community feedback integration

---

**Bottom Line**: Lesser will become the most federation-friendly ActivityPub implementation by addressing real community needs while introducing innovative features that leverage its unique architecture. 