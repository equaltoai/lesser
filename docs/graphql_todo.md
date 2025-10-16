# GraphQL Implementation TODO

## Critical Priority 🔴

### 1. Hashtag Following System
**Impact**: High - Core Mastodon feature  
**Effort**: Medium (2-3 days)

```graphql
# Queries to implement
hashtag(name: String!): Hashtag
followedHashtags(first: Int, after: String): HashtagConnection!
hashtagTimeline(hashtag: String!, first: Int, after: String): PostConnection!
multiHashtagTimeline(hashtags: [String!]!, mode: HashtagMode!, first: Int, after: String): PostConnection!
suggestedHashtags(limit: Int): [HashtagSuggestion!]!

# Mutations to implement
followHashtag(hashtag: String!, notifyLevel: NotificationLevel): HashtagFollowPayload!
unfollowHashtag(hashtag: String!): UnfollowHashtagPayload!
updateHashtagNotifications(hashtag: String!, settings: HashtagNotificationSettingsInput!): UpdateHashtagNotificationsPayload!
muteHashtag(hashtag: String!, until: Time): MuteHashtagPayload!

# Subscription to implement
hashtagActivity(hashtags: [String!]!): HashtagActivityUpdate!
```

**Implementation Steps**:
1. Create `pkg/services/hashtags/` service
2. Add hashtag follow tracking to storage
3. Implement timeline filtering by hashtag
4. Add notification preferences
5. Implement related hashtag suggestions
6. Add subscription support

**Storage Requirements**:
- `HashtagFollow` model (user, hashtag, notification_level, followed_at)
- `HashtagMute` model (user, hashtag, muted_until)
- Indexes on hashtag + user combinations

---

### 2. Thread Synchronization
**Impact**: High - Federation completeness  
**Effort**: Medium (3-4 days)

```graphql
# Queries to implement
threadContext(noteId: ID!): ThreadContext

# Mutations to implement
syncThread(noteUrl: String!, depth: Int): SyncThreadPayload!
syncMissingReplies(noteId: ID!): SyncRepliesPayload!
```

**Implementation Steps**:
1. Create `pkg/services/threads/` service
2. Implement thread traversal (up to root, down to leaves)
3. Add remote note fetching via ActivityPub
4. Track sync status per thread
5. Add depth limiting
6. Handle circular references
7. Queue background sync jobs

**Storage Requirements**:
- `ThreadSync` model (root_note_id, sync_status, last_sync, missing_count)
- Thread parent-child relationship indexes

---

### 3. Phase 2 Alert Subscriptions
**Impact**: Medium - Monitoring capability  
**Effort**: Low (1-2 days)

```graphql
# Subscriptions to implement
moderationAlerts(severity: ModerationSeverity): ModerationAlert!
costAlerts(thresholdUSD: Float!): CostAlert!
budgetAlerts(domain: String): BudgetAlert!
federationHealthUpdates(domain: String): FederationHealthUpdate!
```

**Implementation Steps**:
1. Integrate with existing subscription manager
2. Add alert threshold checking
3. Implement event filtering
4. Add alert deduplication
5. Connect to cost tracking service
6. Connect to moderation service

---

## High Priority 🟡

### 4. Media Streaming Enhancements
**Impact**: Medium - Better user experience  
**Effort**: High (4-5 days)

**Missing Features**:
- HLS/DASH manifest generation
- Adaptive bitrate streaming
- Video transcoding pipeline
- Quality selection
- Bandwidth optimization
- Streaming analytics

**Implementation Steps**:
1. Set up MediaConvert for transcoding
2. Generate HLS playlists
3. Implement adaptive bitrate ladder
4. Add CloudFront integration
5. Track streaming metrics
6. Implement quality reporting

---

### 5. Federation Graph Visualization
**Impact**: Medium - Admin/monitoring tool  
**Effort**: High (5-6 days)

```graphql
# Queries to implement (currently return empty)
federationMap(depth: Int): FederationGraph!
instanceRelationships(domain: String!): InstanceRelations!
federationFlow(period: TimePeriod!): FederationFlow!
```

**Implementation Steps**:
1. Create graph data aggregation job
2. Compute instance relationships
3. Calculate connection weights
4. Implement clustering algorithm
5. Add force-directed layout coordinates
6. Cache computed graphs
7. Add incremental updates

**Storage Requirements**:
- Graph cache (periodically recomputed)
- Instance relationship metrics
- Connection weight calculations

---

### 6. Severed Relationships
**Impact**: Medium - Federation reliability  
**Effort**: Medium (3-4 days)

```graphql
# Queries to implement
severedRelationships(instance: String, first: Int, after: String): SeveredRelationshipConnection!
affectedRelationships(severedRelationshipId: ID!): AffectedRelationshipConnection!

# Mutations to implement
acknowledgeSeverance(id: ID!): AcknowledgePayload!
attemptReconnection(id: ID!): ReconnectionPayload!
```

**Implementation Steps**:
1. Detect instance blocks/defederation
2. Track affected relationships
3. Notify users of severance
4. Implement reconnection attempts
5. Handle partial recoveries
6. Add audit logging

**Storage Requirements**:
- `SeveredRelationship` model
- `AffectedRelationship` linking table
- Reconnection attempt history

---

## Medium Priority 🟢

### 7. Streaming Analytics
**Impact**: Low - Nice to have  
**Effort**: Medium (3-4 days)

```graphql
streamingAnalytics(mediaId: ID!): StreamingAnalytics!
popularStreams(first: Int!, after: String): StreamConnection!
bandwidthUsage(period: TimePeriod!): BandwidthReport!
```

**Integration Points**:
- CloudWatch metrics
- Media service events
- Usage tracking

---

### 8. Performance Monitoring Dashboard
**Impact**: Low - Admin tool  
**Effort**: Low (2-3 days)

```graphql
performanceMetrics(service: ServiceCategory!): PerformanceReport!
slowQueries(threshold: Duration!): [QueryPerformance!]!
infrastructureHealth: InfrastructureStatus!
```

**Integration Points**:
- Existing observability package
- EMF metrics
- Lambda logs
- DynamoDB metrics

---

### 9. Moderation Dashboard Enhancements
**Impact**: Low - Admin tool  
**Effort**: Medium (3-4 days)

```graphql
moderationDashboard(filter: ModerationFilter): ModerationDashboard!
patternEffectiveness(patternId: ID!): PatternStats!
moderatorActivity(moderatorId: ID!, period: TimePeriod!): ModeratorStats!
trainModerationModel(samples: [ModerationSample!]!): TrainingResult!
```

**Requirements**:
- Pattern matching engine
- ML training pipeline
- Effectiveness metrics
- Moderator performance tracking

---

## Implementation Order Recommendation

### Sprint 1 (Week 1-2)
1. **Hashtag Following** - Critical Mastodon feature
2. **Phase 2 Alert Subscriptions** - Quick win

### Sprint 2 (Week 3-4)
3. **Thread Synchronization** - Federation improvement
4. **Severed Relationships** - Federation reliability

### Sprint 3 (Week 5-6)
5. **Media Streaming Enhancements** - UX improvement
6. **Federation Graph Visualization** - Admin tooling

### Sprint 4+ (Week 7+)
7. **Streaming Analytics** - Optional features
8. **Performance Monitoring** - Admin tooling
9. **Moderation Dashboard** - Advanced features

---

## Testing Requirements

For each feature, add:
- Unit tests for service logic
- Integration tests for GraphQL resolvers
- Subscription connection tests
- End-to-end workflow tests

---

## Documentation Requirements

For each feature, add:
- GraphQL schema documentation
- Usage examples
- Client SDK updates
- API reference updates

---

## Estimated Total Effort

- **Critical Priority**: 6-9 days
- **High Priority**: 12-15 days
- **Medium Priority**: 8-11 days
- **Total**: 26-35 days (5-7 weeks for one developer)

## Current Status Summary

- ✅ **Core Mastodon API**: 95% complete
- ✅ **Lesser Extensions**: 90% complete
- ⚠️ **Phase 2 Features**: 40% complete
- ⚠️ **Phase 3 Features**: 20% complete
- ✅ **Subscriptions**: 70% complete

**Overall**: 85% complete, production-ready for core features

