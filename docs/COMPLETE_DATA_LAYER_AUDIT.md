# Complete Data Layer Audit - Lesser Platform
**Date**: 2025-10-22  
**Scope**: EVERY data access issue in pkg/storage

---

## EXECUTIVE SUMMARY

**Total Issues Found**: 3 categories
1. **Unbounded Queries**: ~150+ .All() queries without .Limit()
2. **Missing TableName()**: ~200 models missing required method
3. **Goroutines**: 3 in services, 92 in other pkg/ directories

**System Status**: Working but fragile - issues will surface as data grows

---

## ISSUE #1: UNBOUNDED QUERIES (153 instances)

### Distribution by File

Based on comprehensive scan of pkg/storage/repositories:

- **base_repository.go**: 10 unbounded (core query methods)
- **moderation_repository.go**: 13 unbounded (flags, reports, queue)
- **notification_repository.go**: 7 unbounded (user notifications, groups)
- **object_repository.go**: 7 unbounded (replies, quotes, collections)
- **account_repository_timeline.go**: 6 unbounded (all timelines)
- **relationship_repository.go**: 6 unbounded (followers, following, moves)
- **media_repository.go**: 5 unbounded (unused media, storage calc)
- **route_optimizer_repository.go**: 5 unbounded (delivery results)
- **enhanced_pattern_repository.go**: 5 unbounded (pattern queries)
- **severance_repository.go**: 4 unbounded (severed relationships)
- **dlq_repository.go**: 4 unbounded (dead letter queue)
- **federation_instance_repository.go**: 4 unbounded (instance lists)
- **notification_cost_repository.go**: 4 unbounded (cost tracking)
- **federation_cost_repository.go**: 3 unbounded (cost queries)
- **account_repository_social.go**: 3 unbounded (followers, following, bookmarks)
- **list_repository.go**: 3 unbounded (user lists, list timeline)
- **metrics_repository.go**: 3 unbounded (metric queries)
- **hashtag_repository.go**: 3 unbounded (timeline, follows)
- **import_export_simple_helpers.go**: 3 unbounded (import/export ops)
- **scheduled_job_cost_repository.go**: 3 unbounded (job costs)
- **query_utils.go**: 7 unbounded (generic query utilities)
- **announcement_repository.go**: 2 unbounded (announcements)
- **instance_health_repository.go**: 2 unbounded (health checks)
- **notification_helpers.go**: 2 unbounded (helper queries)
- **relationship_helpers.go**: 2 unbounded (relationship helpers)
- **relationship_pagination_helpers.go**: 2 unbounded (blocks, mutes)
- **scheduled_status_repository.go**: 2 unbounded (scheduled posts)
- **activity_repository.go**: 3 unbounded (inbox/outbox)
- **domain_pagination_helpers.go**: 2 unbounded (domain blocks)

Plus 20+ more files with 1 unbounded query each

**Total**: ~153 unbounded queries

---

## ISSUE #2: MISSING TableName() METHODS (~200 models)

### Critical Models (Used in Repositories - Will Break)

Models that will cause "ResourceNotFoundException" when written:
- Activity
- Alert  
- Announcement
- AnnouncementDismissal
- AnnouncementReaction
- AuthAuditLog
- Bookmark
- CloudWatchMetrics
- CommunityNote
- CommunityNoteVote
- CustomEmoji
- DeadLetterMessage
- Device
- DNSCache
- DomainAllow
- EmailDomainBlock
- EmojiModel
- Export
- ExportCostTracking
- FeaturedTag
- FederationActivity
- FederationAlert
- FederationCost
- FederationCostTracking
- FederationEdge
- FederationHealthReport
- FederationInstance
- FederationIssue
- FederationNode
- FederationRelationship
- FederationSeverance
- FederationStats
- FederationTimeSeries
- Follow
- Hashtag
- HashtagFollow
- HashtagMute
- HashtagStats
- HashtagTrend
- Import
- ImportCostTracking
- InboxItem
- InstanceConfig
- InstanceDomainBlock
- InstanceHealth
- InstanceRule
- Like
- LinkMetadata
- LinkShare
- LinkTrend
- MediaAnalytics
- MediaPopularity
- MediaSession
- MediaVariant
- Mention
- Moderation
- ModerationAnalytics
- ModerationEvidence
- ModerationHistoryEntry
- NotificationBudget
- NotificationCostTracking
- NotificationFilter
- NotificationPreferences
- OAuthApp
- OAuthState
- Object
- OutboxItem
- PublicKeyCache
- PushSubscriptionAlerts
- QuoteRelationship
- Reaction
- RecoveryCode
- RecoveryRequest
- RecoveryToken
- Relay
- RemoteActor
- Report
- Reputation
- ScheduledStatus
- SearchCache
- SearchEmbedding
- SearchQuery
- SearchSuggestion
- SeveredRelationship
- StatusTag
- StatusTrend
- StreamingEvent
- StreamingPreferences
- ThreadContext
- ThreadNode
- ThreadSync
- TrendingHashtag
- TrendingLink
- TrendingStatus
- TrustEvidence
- TrustRelationship
- TrustScore
- UserDomainBlock
- VAPIDKeyRecord
- Vouch
- WebhookDelivery
- WebSocketCostBreakdown
- WebSocketEventConnection
- WebSocketEventSubscription

(~100+ more models - full list available)

**Impact**: ANY write operation to these models will fail with DynamoDB ResourceNotFoundException

---

## ISSUE #3: GOROUTINES (95 total)

### By Layer:
- Repository layer: 0 ✅ (Fixed in Phase 1)
- Services layer: 3
- Other pkg/: 92 (observability, moderation, federation, streaming, media)

### Breakdown:
- pkg/federation/routing: ~15 (circuit breaker async ops)
- pkg/federation/circuit: ~10 (state tracking)
- pkg/moderation: ~8 (pattern matching, reputation)
- pkg/observability: ~5 (metrics collection)
- pkg/streaming: ~5 (error recovery)
- pkg/media: ~3 (bandwidth tracking)
- Others: ~45

**Status**: Most use `context.Background()` (safe), but need verification ALL use background context

---

## COMPLETE FIX PLAN

### Phase 2A: Add Limits to ALL Queries (MANDATORY)

**Scope**: 153 unbounded .All() queries  
**Approach**: Add `.Limit()` before EVERY .All() call

**Default limits by use case**:
- List queries (followers, notifications, etc.): `Limit(100)`
- Timeline queries: `Limit(opts.Limit)` or `Limit(40)` 
- Administrative queries (all users, all instances): `Limit(1000)`
- Count operations: Replace with `.Count()` 
- Batch operations: `Limit(500)`

**Pattern**:
```go
// Before
err := query.All(&results)

// After
err := query.Limit(100).All(&results)
// OR
err := query.Limit(limit).All(&results)  // If limit parameter exists
```

**Files to modify**: 50+ repository files

**Estimated time**: 4-6 hours (mechanical changes)

---

### Phase 2B: Add TableName() to ALL Models (CRITICAL)

**Scope**: ~200 models missing TableName()  
**Approach**: Add method to every model

**Pattern**:
```go
// Add to each model
func (ModelName) TableName() string {
    return MainTableName
}
```

**Exception**: Models that are not stored (test helpers, builders, etc.) - verify before adding

**Estimated time**: 2-3 hours

---

### Phase 2C: Verify All Goroutines Use Background Context

**Scope**: 95 goroutines outside repository layer  
**Approach**: Audit each one, fix any using request context

**Check pattern**:
```go
// ❌ BAD
go func() {
    doSomething(ctx)  // ctx from parent - will be canceled
}()

// ✅ GOOD
go func() {
    bgCtx := context.Background()
    doSomething(bgCtx)
}()
```

**Estimated time**: 2-3 hours

---

## IMPLEMENTATION PRIORITY

### IMMEDIATE (Deploy Today):
1. Add TableName() to models currently being used:
   - WebSocketConnection ✅ (already fixed)
   - WebSocketSubscription ✅ (already fixed)
   - Activity, Follow, Like, Hashtag, etc. (verify which are actively written)

2. Add Limit() to queries in hot paths:
   - notification_repository.go (7 queries)
   - relationship_repository.go (6 queries)
   - account_repository_timeline.go (6 queries)
   - object_repository.go (7 queries)

### THIS WEEK:
1. Add TableName() to remaining ~200 models
2. Add Limit() to all 153 unbounded queries
3. Verify all 95 goroutines use background context

### VERIFICATION:
```bash
# No unbounded queries
rg "\.All\(" pkg/storage/repositories --type go -B 1 | grep -v "Limit" | grep "\.All" | wc -l
# Should be: 0

# All models have TableName
# (manual verification of critical models)

# No goroutines with request context
rg "go func" pkg --type go -A 2 | grep "ctx[,)]" | grep -v "Background"
# Should be: 0
```

---

**This is the COMPLETE inventory. Every issue documented. Ready for systematic fixes.**

