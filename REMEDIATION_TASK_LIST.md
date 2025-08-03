# Comprehensive Remediation Task List
*Generated: August 2, 2025*

## Priority Order:
1. **Replace Direct AWS SDK Calls** (Architecture violation - CRITICAL)
2. **Replace Polling Patterns** (Serverless anti-pattern - HIGH)
3. **Implement Stubs** (Functionality gaps - MEDIUM)

---

## PRIORITY 1: Replace Direct AWS SDK Calls (30+ files)

### Task Group A: Federation Subsystem DynamORM Migration
**Impact: Critical - Entire federation system violates architecture**

- **TASK-AWS-1**: Convert federation routing health checker to DynamORM
  - File: `/pkg/federation/routing/health_checker.go`
  - Create models: `federation_health.go`
  - Remove: `dynamodb.Client` usage
  
- **TASK-AWS-2**: Convert federation circuit breaker to DynamORM
  - File: `/pkg/federation/routing/circuit_breaker.go`
  - Create models: `circuit_state.go`
  - Implement atomic state transitions

- **TASK-AWS-3**: Convert federation instance registry to DynamORM
  - File: `/pkg/federation/routing/instance_registry.go`
  - Create models: `federation_instance.go`
  - Replace all Query/Scan operations

- **TASK-AWS-4**: Convert federation route manager to DynamORM
  - File: `/pkg/federation/routing/route_manager.go`
  - Create models: `federation_route.go`
  - Implement route persistence

- **TASK-AWS-5**: Convert federation route optimizer to DynamORM
  - File: `/pkg/federation/routing/route_optimizer.go`
  - Use existing models
  - Replace optimization storage

- **TASK-AWS-6**: Convert federation query optimizer to DynamORM
  - File: `/pkg/federation/routing/query_optimizer.go`
  - Create models: `query_cache.go`
  - Implement cache operations

- **TASK-AWS-7**: Convert federation cost storage to DynamORM
  - File: `/pkg/federation/cost/storage.go`
  - Create models: `federation_cost.go`
  - Track per-instance costs

### Task Group B: Media Streaming DynamORM Migration
**Impact: High - Core media functionality**

- **TASK-AWS-8**: Convert media session management to DynamORM
  - File: `/pkg/media/streaming/session.go`
  - Create models: `media_session.go`
  - Implement TTL for sessions

- **TASK-AWS-9**: Convert media streamer to DynamORM
  - File: `/pkg/media/streaming/streamer.go`
  - Use session models
  - Remove direct SDK calls

- **TASK-AWS-10**: Convert media storage to DynamORM
  - File: `/pkg/media/streaming/storage.go`
  - Create models: `media_metadata.go`
  - Implement media tracking

### Task Group C: Moderation Advanced DynamORM Migration
**Impact: High - Security and content moderation**

- **TASK-AWS-11**: Convert pattern matcher to DynamORM
  - File: `/pkg/moderation/advanced/pattern_matcher.go`
  - Already has RefreshPatterns() method
  - Create models: `moderation_pattern.go`
  - Remove all SDK imports

- **TASK-AWS-12**: Convert threat intel to DynamORM
  - File: `/pkg/moderation/advanced/threat_intel.go`
  - Already has RefreshThreats() method
  - Create models: `threat_intel.go`
  - Implement threat storage

- **TASK-AWS-13**: Convert moderation metrics to DynamORM
  - File: `/pkg/moderation/advanced/metrics.go`
  - Create models: `moderation_metrics.go`
  - Track moderation statistics

### Task Group D: Core Services DynamORM Migration
**Impact: High - Core platform functionality**

- **TASK-AWS-14**: Convert rate limiter to DynamORM
  - File: `/pkg/ratelimit/limiter.go`
  - Create models: `rate_limit.go`
  - Implement atomic counters

- **TASK-AWS-15**: Convert websocket subscriptions to DynamORM
  - File: `/pkg/websocket/subscriptions.go`
  - Create models: `websocket_subscription.go`
  - Track active connections

- **TASK-AWS-16**: Convert reputation service to DynamORM
  - File: `/pkg/reputation/service.go`
  - Create models: `reputation_score.go`
  - Implement score tracking

- **TASK-AWS-17**: Convert trust service to DynamORM
  - File: `/pkg/trust/service.go`
  - Create models: `trust_relationship.go`
  - Implement trust graph

- **TASK-AWS-18**: Convert notes storage to DynamORM
  - File: `/pkg/notes/storage.go`
  - Create models: `note.go`
  - Implement note persistence

### Task Group E: Auth System Cleanup
**Impact: Medium - Some already migrated**

- **TASK-AWS-19**: Remove legacy CSRF DynamoDB implementation
  - File: `/pkg/auth/csrf_dynamodb.go`
  - Already replaced with DynamORM version
  - Delete file and tests

- **TASK-AWS-20**: Convert refresh tokens to DynamORM
  - File: `/pkg/auth/refresh_tokens.go`
  - Create models: `refresh_token.go`
  - Implement token families

### Task Group F: Repository Layer Fixes
**Impact: Medium - Partial implementations**

- **TASK-AWS-21**: Fix rate limit repository
  - File: `/pkg/storage/repositories/rate_limit_repository.go`
  - Remove any remaining SDK usage
  - Use DynamORM patterns

- **TASK-AWS-22**: Fix soft delete pattern
  - File: `/pkg/storage/dynamorm/patterns/soft_delete.go`
  - Remove SDK usage in tests
  - Use proper DynamORM

---

## PRIORITY 2: Replace Polling Patterns (17 files, excluding acceptable fallbacks)

### Task Group G: Federation Polling Removal
**Impact: Critical - Lambda anti-pattern**

- **TASK-POLL-1**: Replace federation health check polling
  - File: `/pkg/federation/routing/health_checker.go`
  - Remove per-instance tickers
  - Implement EventBridge triggers

- **TASK-POLL-2**: Replace federation circuit breaker polling
  - File: `/pkg/federation/routing/circuit_breaker.go`
  - Already has serverless version created
  - Migrate to new implementation

- **TASK-POLL-3**: Replace route optimizer polling
  - File: `/pkg/federation/routing/route_optimizer.go`
  - Remove 5-minute ticker
  - Trigger via EventBridge

- **TASK-POLL-4**: Replace query optimizer polling
  - File: `/pkg/federation/routing/query_optimizer.go`
  - Remove flush ticker
  - Use synchronous operations

- **TASK-POLL-5**: Replace instance registry polling
  - File: `/pkg/federation/routing/instance_registry.go`
  - Remove registration refresh tickers
  - Use TTL and events

- **TASK-POLL-6**: Replace federation metrics polling
  - File: `/pkg/federation/routing/metrics.go`
  - Remove aggregation ticker
  - Use CloudWatch EMF

### Task Group H: Monitoring & Observability Polling
**Impact: High - Wasted resources**

- **TASK-POLL-7**: Migrate to EMF metrics (partially done)
  - File: `/pkg/observability/metrics.go`
  - Complete migration to EMF
  - Remove remaining ticker

- **TASK-POLL-8**: Fix CloudWatch metrics
  - File: `/pkg/monitoring/cloudwatch_metrics.go`
  - Remove flush ticker
  - Use EMF format

- **TASK-POLL-9**: Fix moderation metrics
  - File: `/pkg/moderation/advanced/metrics.go`
  - Remove aggregation ticker
  - Use EMF or synchronous

---

## PRIORITY 3: Implement Stubs (76 occurrences)

### Task Group I: Critical Federation Stubs
**Impact: High - Core functionality missing**

- **TASK-STUB-1**: Implement federation storage methods
  - File: `/pkg/federation/dynamorm_storage.go`
  - Implement GetFollowers (line 46)
  - Implement CheckCache (line 53)
  - Implement StoreCache (line 60)

- **TASK-STUB-2**: Implement activity processor stubs
  - File: `/cmd/activity-processor/main.go`
  - Fetch original object (line 548)
  - Detect language (line 645)
  - Query followers list (line 670)

### Task Group J: Storage Layer Stubs
**Impact: High - Data operations incomplete**

- **TASK-STUB-3**: Implement timeline batch operations
  - Files: `/pkg/storage/repositories/timeline_repository.go`
  - Lines 57, 351, 365, 385: Batch operations
  - Use DynamORM batch writer

- **TASK-STUB-4**: Implement notification batch operations
  - File: `/pkg/storage/repositories/notification_repository.go`
  - Lines 418, 446: Batch operations
  - Use DynamORM batch writer

- **TASK-STUB-5**: Implement object repository features
  - File: `/pkg/storage/repositories/object_repository.go`
  - Reply count tracking (line 897)
  - Quote tracking (lines 967, 1142)
  - Quote permissions (lines 1154, 1163)
  - Status metadata (lines 1177, 1188)

- **TASK-STUB-6**: Implement search repository features
  - File: `/pkg/storage/repositories/search_repository.go`
  - Status removal from index (line 869)
  - Vector search (line 1104)

- **TASK-STUB-7**: Implement user repository features
  - File: `/pkg/storage/repositories/user_repository.go`
  - Pagination (line 203)
  - Activity queries (line 212)

### Task Group K: GraphQL Resolver Stubs
**Impact: Medium - API functionality gaps**

- **TASK-STUB-8**: Implement trust metrics
  - File: `/graph/schema.resolvers.go`
  - Calculate trust metrics (lines 392-396)
  - Count trust relationships

- **TASK-STUB-9**: Implement cost calculations
  - File: `/graph/schema.resolvers.go`
  - Daily total (line 767)
  - Monthly projection (line 768)
  - Use CloudWatch metrics

- **TASK-STUB-10**: Implement DataLoader integration
  - File: `/graph/schema.resolvers.go`
  - Actor loading (lines 425-426)
  - Proper DataLoader setup

- **TASK-STUB-11**: Implement quote features
  - File: `/graph/schema.resolvers.go`
  - Quote policy (line 2500)
  - Quote context (line 2696)
  - Withdrawal status (lines 2503, 3184)

- **TASK-STUB-12**: Implement moderation features
  - File: `/graph/schema.resolvers.go`
  - Role-based access (line 4007)
  - Moderation permissions (line 5279)

### Task Group L: Lambda Processor Stubs
**Impact: Medium - Background processing**

- **TASK-STUB-13**: Implement notification delivery
  - File: `/cmd/notification-processor/main.go`
  - SNS/FCM/APNS delivery (line 389)
  - Actor details lookup (line 554)
  - User preferences (line 563)
  - Active connections (line 582)

- **TASK-STUB-14**: Implement stream router features
  - File: `/cmd/stream-router/main.go`
  - Account details fetching (line 608)
  - DynamoDB record extraction (line 801)

- **TASK-STUB-15**: Implement note processor features
  - File: `/cmd/note-processor/main.go`
  - Subscription queries (line 483)
  - Analysis storage (line 564)

### Task Group M: Analytics & URLs
**Impact: Low - Cosmetic/reporting**

- **TASK-STUB-16**: Fix hardcoded URLs
  - File: `/pkg/storage/repositories/analytics_repository.go`
  - Lines 265, 333, 527: Use actual domain
  - Get domain from environment

- **TASK-STUB-17**: Implement analytics features
  - File: `/pkg/storage/repositories/analytics_repository.go`
  - Popular query counter (line 1060)
  - Actual status fetching (line 1088)

---

## Execution Strategy

### Phase 1 (Weeks 1-2): Critical AWS SDK Replacements
- Complete all federation DynamORM migrations (TASK-AWS-1 to 7)
- Complete media streaming migrations (TASK-AWS-8 to 10)
- Use lift-dynamorm-expert agent for each task

### Phase 2 (Weeks 3-4): Remaining AWS SDK & Polling
- Complete moderation and core services (TASK-AWS-11 to 22)
- Remove all federation polling (TASK-POLL-1 to 6)
- Remove monitoring polling (TASK-POLL-7 to 9)

### Phase 3 (Weeks 5-6): Stub Implementations
- Implement critical federation stubs (TASK-STUB-1 to 2)
- Implement storage layer stubs (TASK-STUB-3 to 7)
- Focus on functionality that blocks other features

### Phase 4 (Weeks 7-8): Complete Remaining Stubs
- Implement GraphQL resolver stubs (TASK-STUB-8 to 12)
- Implement Lambda processor stubs (TASK-STUB-13 to 15)
- Fix cosmetic issues (TASK-STUB-16 to 17)

## Success Metrics
- 0 direct AWS SDK imports (except in Lift/DynamORM internals)
- 0 polling patterns (except GraphQL subscription fallbacks)
- 0 "in a real" or "in a full" comments
- 100% DynamORM compliance
- All tests passing