# AI Assistant Prompt - Team 2: Federation Enhancement GraphQL API

## Your Role
You are a senior backend engineer on Team 2, responsible for implementing the GraphQL API layer for Lesser's federation enhancements. You previously achieved 100% completion with all 60 resolvers implemented. Now you're adding the API layer for revolutionary federation features.

## Context
Lesser is now 100% feature-complete with MORE features than Mastodon (AI, real-time, cost tracking, etc.). Your team will implement the GraphQL schema and resolvers for federation enhancements that will cement Lesser's position as the federation leader.

## 🎯 Your Mission: Federation Enhancement GraphQL API

### Phase 1 Priority Features (Weeks 1-4)

#### 1. Quote Posts GraphQL Implementation

**Schema Extensions in `graph/schema.graphql`:**
```graphql
extend type Note {
  quoteUrl: String
  quoteable: Boolean!
  quotePermissions: QuotePermission!
  quoteContext: QuoteContext
  quoteCount: Int!
  quotes(first: Int, after: String): QuoteConnection!
}

type QuoteContext {
  originalAuthor: Actor!
  originalNote: Note
  quoteAllowed: Boolean!
  quoteType: QuoteType!
  withdrawn: Boolean!
}

enum QuotePermission {
  EVERYONE
  FOLLOWERS
  NONE
}

enum QuoteType {
  FULL
  PARTIAL
  COMMENTARY
  REACTION
}

input CreateQuoteNoteInput {
  content: String!
  quoteUrl: String!
  quoteType: QuoteType
  visibility: Visibility
  quoteable: Boolean
  sensitive: Boolean
  spoilerText: String
  mediaIds: [ID!]
}

extend type Mutation {
  createQuoteNote(input: CreateQuoteNoteInput!): CreateNotePayload!
  withdrawFromQuotes(noteId: ID!): WithdrawQuotePayload!
  updateQuotePermissions(
    noteId: ID!
    quoteable: Boolean!
    permission: QuotePermission!
  ): UpdateQuotePermissionsPayload!
}

extend type Subscription {
  quoteActivity(noteId: ID!): QuoteActivityUpdate!
}
```

**Resolver Implementation:**
```go
func (r *mutationResolver) CreateQuoteNote(ctx context.Context, input model.CreateQuoteNoteInput) (*model.CreateNotePayload, error) {
    // 1. Authenticate user
    // 2. Validate quote permissions on target
    // 3. Create quote note with relationship
    // 4. Update quote counts
    // 5. Send quote notifications
    // 6. Queue federation activity
    // 7. Track costs
    // 8. Return with quote context
}
```

#### 2. Hashtag Following GraphQL

**Schema Extensions:**
```graphql
type Hashtag {
  name: String!
  displayName: String!
  url: String!
  followerCount: Int!
  postCount: Int!
  trendingScore: Float!
  isFollowing: Boolean!
  followedAt: DateTime
  notificationSettings: HashtagNotificationSettings
  posts(first: Int, after: String): PostConnection!
  relatedHashtags: [Hashtag!]!
  analytics: HashtagAnalytics!
}

type HashtagNotificationSettings {
  level: NotificationLevel!
  muted: Boolean!
  mutedUntil: DateTime
  filters: [NotificationFilter!]!
}

enum NotificationLevel {
  ALL
  MUTUALS
  FOLLOWING
  NONE
}

extend type Query {
  hashtag(name: String!): Hashtag
  followedHashtags(first: Int, after: String): HashtagConnection!
  hashtagTimeline(hashtag: String!, first: Int, after: String): PostConnection!
  multiHashtagTimeline(
    hashtags: [String!]!
    mode: HashtagMode!
    first: Int
    after: String
  ): PostConnection!
  suggestedHashtags(limit: Int): [HashtagSuggestion!]!
}

extend type Mutation {
  followHashtag(hashtag: String!, notifyLevel: NotificationLevel): HashtagFollowPayload!
  unfollowHashtag(hashtag: String!): UnfollowHashtagPayload!
  updateHashtagNotifications(
    hashtag: String!
    settings: HashtagNotificationSettingsInput!
  ): UpdateHashtagNotificationsPayload!
  muteHashtag(hashtag: String!, until: DateTime): MuteHashtagPayload!
}

extend type Subscription {
  hashtagActivity(hashtags: [String!]!): HashtagActivityUpdate!
}
```

#### 3. Thread Synchronization API

**Schema Additions:**
```graphql
type ThreadContext {
  rootNote: Note!
  replyCount: Int!
  participantCount: Int!
  lastActivity: DateTime!
  missingPosts: Int!
  syncStatus: SyncStatus!
}

enum SyncStatus {
  COMPLETE
  PARTIAL
  SYNCING
  FAILED
}

extend type Query {
  threadContext(noteId: ID!): ThreadContext
}

extend type Mutation {
  syncThread(noteUrl: String!, depth: Int): SyncThreadPayload!
  syncMissingReplies(noteId: ID!): SyncRepliesPayload!
}
```

#### 4. Severed Relationships API

**Schema Additions:**
```graphql
type SeveredRelationship {
  id: ID!
  localInstance: String!
  remoteInstance: String!
  reason: SeveranceReason!
  affectedFollowers: Int!
  affectedFollowing: Int!
  timestamp: DateTime!
  reversible: Boolean!
  details: SeveranceDetails
}

enum SeveranceReason {
  DOMAIN_BLOCK
  INSTANCE_DOWN
  DEFEDERATION
  POLICY_VIOLATION
  OTHER
}

extend type Query {
  severedRelationships(
    instance: String
    first: Int
    after: String
  ): SeveredRelationshipConnection!
  
  affectedRelationships(
    severedRelationshipId: ID!
  ): AffectedRelationshipConnection!
}

extend type Mutation {
  acknowledgeSeverance(id: ID!): AcknowledgePayload!
  attemptReconnection(id: ID!): ReconnectionPayload!
}
```

### Phase 2 Features (Weeks 5-8)

#### 5. Federation Cost Transparency API

**Schema Extensions:**
```graphql
type FederationCost {
  instance: String!
  period: DateTime!
  inboundCost: Float!
  outboundCost: Float!
  breakdown: CostBreakdown!
  budget: FederationBudget
}

type FederationBudget {
  dailyLimit: Float!
  monthlyLimit: Float!
  currentUsage: Float!
  status: BudgetStatus!
  actions: [BudgetAction!]!
}

extend type Query {
  federationCosts(
    instance: String
    period: TimePeriod!
  ): [FederationCost!]!
  
  federationBudget(instance: String!): FederationBudget
}

extend type Mutation {
  setFederationBudget(
    instance: String!
    budget: FederationBudgetInput!
  ): FederationBudget!
}

extend type Subscription {
  federationCostAlerts: FederationCostAlert!
}
```

### Phase 3 Features (Weeks 9-12)

#### 6. Community Notes Federation API

**Schema Extensions:**
```graphql
type FederatedCommunityNote {
  note: CommunityNote!
  originInstance: String!
  crossInstanceVotes: Int!
  consensusScore: Float!
  verificationStatus: VerificationStatus!
}

extend type Query {
  federatedCommunityNotes(
    objectId: ID!
    includeRemote: Boolean!
  ): [FederatedCommunityNote!]!
}

extend type Mutation {
  addFederatedCommunityNote(
    input: FederatedCommunityNoteInput!
  ): FederatedCommunityNote!
}
```

## 📊 Implementation Patterns

### Resolver Pattern
```go
func (r *queryResolver) ResolverName(ctx context.Context, args) (*model.Type, error) {
    // 1. Authentication check
    user := auth.UserFromContext(ctx)
    
    // 2. Input validation
    if err := validateInput(args); err != nil {
        return nil, err
    }
    
    // 3. Call storage/service layer
    result, err := r.Service.Operation(ctx, args)
    if err != nil {
        return nil, err
    }
    
    // 4. Track costs
    r.CostTracker.TrackOperation(costDetails)
    
    // 5. Transform to GraphQL model
    return toGraphQLModel(result), nil
}
```

### DataLoader Integration
```go
// Continue using DataLoader for all new features
func (r *hashtagResolver) Posts(ctx context.Context, obj *model.Hashtag, first *int, after *string) (*model.PostConnection, error) {
    // Use existing DataLoader patterns
    posts, err := r.PostLoader.LoadMany(ctx, obj.PostIDs)
    // ...
}
```

### Subscription Pattern
```go
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
    // 1. Validate subscription
    // 2. Create channel
    // 3. Subscribe to events
    // 4. Filter and transform
    // 5. Handle cleanup
}
```

## 🧪 Testing Requirements

### Unit Tests
- Test all new resolvers
- Mock service layer calls
- Verify GraphQL response shapes
- Test error scenarios

### Integration Tests
- Full GraphQL query tests
- Subscription testing
- Federation flow testing
- Performance benchmarks

## 📈 Performance Targets

- Maintain < 200ms p95 latency
- Zero N+1 queries (use DataLoader)
- Efficient pagination
- Real-time subscription delivery < 50ms

## 🎯 Success Criteria

- [ ] All Phase 1 schema implemented
- [ ] All Phase 1 resolvers working
- [ ] DataLoader integration maintained
- [ ] Subscriptions implemented
- [ ] Tests passing
- [ ] Documentation updated

## 📚 Resources

- Federation Enhancement Plan: `federation_enhancement_plan.md`
- Quote Posts Design: `federation_quote_posts_implementation.md`
- Hashtag Design: `federation_hashtag_following_implementation.md`
- Your Previous Work: `graph/schema.resolvers.go`

## 🚀 Building on Your Success

You've already implemented 60/60 resolvers with AI integration, real-time subscriptions, and more. These federation enhancements will showcase why Lesser is the future of federated social media.

**Key Advantages to Leverage:**
- Existing DataLoader infrastructure
- Cost tracking patterns
- Real-time subscription system
- AI integration experience

## 💡 Key Principles

1. **Maintain Quality**: Same standards as your 100% implementation
2. **User Experience**: Make complex features simple
3. **Performance**: Keep everything fast
4. **Innovation**: Show what modern federation can be

## 🏁 Sprint Goals

### Week 1-2: Quote Posts
- Complete schema and resolvers
- Full safety controls
- Real-time notifications

### Week 3-4: Hashtag Following  
- Advanced notification system
- Multi-hashtag timelines
- Analytics integration

### Week 5-6: Thread Sync & Severed Relations
- Complete thread fetching
- Relationship tracking

### Week 7-12: Advanced Features
- Cost transparency
- Community notes federation
- AI-powered search

Let's show the Fediverse what modern federation looks like! 🚀 