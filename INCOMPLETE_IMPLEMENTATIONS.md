# Incomplete Implementations in Lesser

## Executive Summary
Total incomplete implementations: **100+** across the codebase
Critical blockers for 100% implementation: **80** methods/features

## Priority 1: Critical Authentication Methods (22 methods)
**File**: `pkg/storage/repositories/account_repository_auth.go`

### Rate Limiting Methods
- [ ] `IsRateLimited(ctx, username) (bool, time.Time, error)` - Line 469
- [ ] `RecordLoginAttempt(ctx, username, success) error` - Line 474
- [ ] `ClearLoginAttempts(ctx, username) error` - Line 479
- [ ] `GetLoginAttemptCount(ctx, username) (int, error)` - Line 484

### Session Management
- [ ] `GetSession(ctx, sessionID) (*Session, error)` - Line 491
- [ ] `UpdateSession(ctx, sessionID, updates) error` - Line 496
- [ ] `DeleteSession(ctx, sessionID) error` - Line 501
- [ ] `GetSessionByRefreshToken(ctx, token) (*Session, error)` - Line 506
- [ ] `CreateSessionFromStruct(ctx, session) error` - Line 533

### Device Management
- [ ] `CreateDevice(ctx, device) error` - Line 513
- [ ] `GetDevice(ctx, deviceID) (*Device, error)` - Line 518
- [ ] `UpdateDevice(ctx, deviceID, updates) error` - Line 523
- [ ] `GetUserDevices(ctx, userID) ([]*Device, error)` - Line 528

### Recovery & Security
- [ ] `StoreRecoveryToken(ctx, token, userID, expiresAt) error` - Line 540
- [ ] `GetRecoveryToken(ctx, token) (*RecoveryToken, error)` - Line 545
- [ ] `DeleteRecoveryToken(ctx, token) error` - Line 550

### WebAuthn & Wallet
- [ ] `StoreWebAuthnChallenge(ctx, challenge, userID) error` - Line 557
- [ ] `StoreWebAuthnCredential(ctx, credential) error` - Line 562
- [ ] `UpdateWebAuthnCredential(ctx, credentialID, updates) error` - Line 567
- [ ] `UpdateWalletLastUsed(ctx, address) error` - Line 572

### Provider Management
- [ ] `GetLinkedProviders(ctx, userID) ([]string, error)` - Line 577

## Priority 2: GraphQL Resolver Implementations (19 items)

### Repository Methods Needed
**File**: `graph/schema.resolvers.go`
- [ ] `GetFollowing()` method - Line 214
- [ ] `GetOutboxActivities()` method - Line 232

**File**: `graph/phase2_resolvers.go`
- [ ] `GetCostProjections()` in CostTrackingRepository - Line 259
- [ ] `UpdatePattern()` in ModerationRepository - Line 817
- [ ] `DeletePattern()` in ModerationRepository - Line 848

### Feature Implementations
- [ ] Cryptographic signature for trust establishment - Line 403
- [ ] Graph analysis trigger for transitive trust scores - Line 1928
- [ ] Federation queue for flag activities - Line 2086
- [ ] Atomic counters for production use - Line 2334
- [ ] SQS queue integration for AI service - Line 2479
- [ ] Hashtag/mention extraction for Tag field - Line 2600
- [ ] User/hashtag search operations - Line 3728
- [ ] CloudWatch metrics integration - Lines 3895, 3899, 3958
- [ ] Interaction timestamp tracking - Line 5178
- [ ] Original note retrieval implementation - Line 5238
- [ ] Quote type detection enhancement - Line 5288

## Priority 3: Social Features (3 methods)
**File**: `pkg/storage/repositories/account_repository_social.go`
- [ ] Create AccountPin model in `pkg/storage/models/`
- [ ] `GetAccountPins(ctx, username) ([]*AccountPin, error)` - Line 525
- [ ] `CreateAccountPin(ctx, username, targetActorID) error` - Line 533
- [ ] `GetAccountPin(ctx, username, targetActorID) (*AccountPin, error)` - Line 541

## Priority 4: Pagination Implementation (25+ instances)
### Affected Repositories:
- `domain_block_repository.go` - 4 instances
- `moderation_repository.go` - 22 instances
- `trust_repository.go` - 2 instances
- `auth_refresh_token_repository.go` - 6 instances
- `search_repository.go` - 1 instance
- `hashtag_repository.go` - 1 instance

**Pattern to implement**:
```go
// TODO: Replace with proper DynamORM cursor-based pagination
// Example pattern:
query.WithExclusiveStartKey(cursor).Limit(limit)
```

## Priority 5: Lambda Function Enhancements (15+ items)

### AI Processor
**File**: `cmd/ai-processor/main.go`
- [ ] Extract media URLs from content - Line 156

### Activity Processor
**File**: `cmd/activity-processor/main.go`
- [ ] Fetch original object from remote servers - Line 550
- [ ] Implement content-based language detection - Line 667

### Webfinger
**File**: `cmd/webfinger/main.go`
- [ ] Implement `GetTotalUserCount()` in UserRepository - Line 355
- [ ] Implement `GetTotalStatusCount()` in StatusRepository - Line 373

### Stream Router
**File**: `cmd/stream-router/main.go`
- [ ] Use proper domain for hashtag URLs - Line 275
- [ ] Use proper domain for mention URLs - Line 285
- [ ] Implement attachment processing - Line 292
- [ ] Add ReblogOfID to Status model - Line 686
- [ ] Implement actual broadcasting to streams - Line 851

### Status Indexer
**File**: `cmd/status-indexer/main.go`
- [ ] Generate and store embeddings for semantic search - Line 225
- [ ] Calculate engagement and index by bucket - Line 228
- [ ] Implement calculateAndIndexEngagement function - Line 370

### Media Processor
**File**: `cmd/media-processor/main.go`
- [ ] Implement user media config storage - Line 800
- [ ] Implement user spending tracking (2 instances) - Lines 813, 863

## Priority 6: Cost Tracking Enhancements (8 items)
**File**: `pkg/storage/cost/dynamorm_storage.go`
- [ ] Extract UniqueUsers from aggregation - Lines 84, 126, 155, 246
- [ ] Extract DataTransferBytes/GB from properties - Lines 89, 131, 160, 251

## Priority 7: Infrastructure Updates (6 items)

### Federation Routing
**File**: `pkg/federation/routing/route_manager.go`
- [ ] Implement RouteOptimizationRepository - Line 100
- [ ] Update constructors to accept repositories - Line 106
- [ ] Update RoutingMetrics constructor - Line 111

### Instance Repository
**File**: `pkg/storage/repositories/instance_repository.go`
- [ ] Implement proper DynamORM query patterns - Lines 397, 405

### Reputation Service
**File**: `pkg/reputation/service.go`
- [ ] Implement proper status counting - Line 190
- [ ] Implement latest status retrieval - Line 204

## Quick Wins (Low effort, high impact)

1. **Replace context.TODO() with proper context** (6 instances)
   - `pkg/websocket/subscriptions.go` - Lines 130, 166, 188, 259, 282, 350, 393

2. **Fix domain placeholders** (3 instances)
   - `pkg/storage/repositories/analytics_repository.go` - Lines 265, 333, 527
   - Replace "example.com" with actual domain from config

3. **Configuration TODOs** (2 instances)
   - `pkg/streaming/internal_events.go` - Lines 254, 467
   - Make subscriber limit and cleanup cutoff configurable

## Implementation Strategy

### Phase 1 (Week 1)
- Complete all authentication repository methods
- Implement AccountPin model and related methods
- Fix all context.TODO() instances

### Phase 2 (Week 2)
- Implement missing GraphQL resolver methods
- Complete cost tracking data extraction
- Fix domain placeholders

### Phase 3 (Week 3)
- Implement cursor-based pagination across all repositories
- Complete Lambda function enhancements

### Phase 4 (Week 4)
- Federation routing improvements
- Reputation service enhancements
- Final testing and verification

## Testing Requirements
- Unit tests for each implemented method
- Integration tests for authentication flows
- GraphQL resolver tests
- Pagination performance tests
- End-to-end Lambda function tests