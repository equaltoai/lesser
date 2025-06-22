# Remaining Stub Implementation Tasks

This document tracks the remaining stub implementations that need to be completed in the Lesser ActivityPub implementation. As of the latest audit, **18 stub implementations remain** across various categories.

## Status Overview

✅ **Completed (8 stubs)**
- Federation severance GraphQL resolvers (4 resolvers)
- Quote context GraphQL resolvers (4 resolvers)

🔄 **Remaining (18 stubs)**
- Direct timeline implementation (1 stub)
- Remote resolution functionality (1 stub) 
- Threading sync functionality (4 stubs)
- Quote permissions checking (2 stubs)
- API handlers (5 stubs)
- Media processing (3 stubs)
- Analytics (2 stubs)

---

## High Priority Tasks

### 1. Direct Timeline Implementation
**File**: `graph/schema.resolvers.go:3280-3282`
**Status**: ❌ Not Implemented
**Impact**: High - Core messaging functionality

```go
// Timeline (Direct Messages) - currently returns error
return nil, fmt.Errorf("direct timeline not yet implemented")
```

**Requirements**:
- Implement direct message timeline querying
- Add private message support
- Integrate with existing timeline infrastructure
- Handle pagination and filtering

**Dependencies**: 
- Private messaging storage layer
- Direct message permissions
- Timeline rendering system

---

### 2. Remote Resolution Functionality
**File**: `pkg/storage/dynamodb/search_graphql.go:99-102`
**Status**: ❌ Not Implemented
**Impact**: Medium - Federation search functionality

```go
// TODO: Implement remote actor resolution
s.logger().Debug("remote resolution not implemented yet", zap.String("query", query))
```

**Requirements**:
- Implement ActivityPub actor resolution from remote servers
- Add WebFinger lookup support
- Cache resolved actors for performance
- Handle resolution failures gracefully

**Dependencies**:
- HTTP client for remote requests
- WebFinger protocol implementation
- Actor caching system

---

## Medium Priority Tasks

### 3. Threading Sync Functionality
**Files**: `pkg/storage/dynamodb/threads_graphql.go`
**Status**: ❌ Not Implemented (4 functions)
**Impact**: Medium - Thread completeness and federation

#### 3.1 Remote Thread Synchronization
```go
func SyncThreadFromRemote(ctx context.Context, statusID string) error
// Lines 48-55: Actual remote fetching logic not implemented
```

#### 3.2 Missing Replies Sync
```go
func SyncMissingRepliesFromRemote(ctx context.Context, statusID string) error
// Lines 89-90: Remote fetching of missing replies not implemented
```

#### 3.3 Thread Ancestor Traversal
```go
func getThreadAncestors(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error)
// Lines 125-132: Proper ancestor traversal not implemented
```

#### 3.4 Missing Reply Detection
```go
func identifyMissingReplies(ctx context.Context, statusID string) ([]string, error)
// Lines 324-332: Gap analysis not implemented
```

**Requirements**:
- Implement ActivityPub collection fetching
- Add thread gap detection algorithms
- Handle remote server failures
- Optimize for large thread trees

---

### 4. Quote Permissions Checking
**File**: `pkg/storage/dynamodb/quotes_graphql.go:145-146`
**Status**: ⚠️ Partially Implemented
**Impact**: Medium - Quote functionality completeness

```go
// TODO: Check if quoter is a follower (AllowFollowers)
// TODO: Check if quoter is mentioned in the status (AllowMentioned)
// These would require additional queries
```

**Requirements**:
- Implement follower relationship checking for quote permissions
- Add mention detection for quote permissions
- Integrate with existing relationship storage
- Optimize permission checking queries

**Dependencies**:
- Relationship storage queries
- Mention parsing system
- Permission evaluation engine

---

## API Handler Stubs

### 5. OAuth External Providers
**File**: `cmd/api/handlers/oauth_external.go`
**Status**: ❌ Explicitly Disabled (4 functions)
**Impact**: Low - External OAuth integration

All external OAuth provider functions return:
```go
return BadRequest("external OAuth providers are not supported")
```

**Functions**:
- `HandleOAuthProviderAuthorize` (line 28)
- `HandleOAuthProviderCallback` (line 89)
- `HandleLinkOAuthProvider` (line 96)
- `HandleUnlinkOAuthProvider` (line 103)

**Decision Required**: Determine if external OAuth support is needed for Lesser's architecture.

### 6. Email Functionality
**File**: `cmd/api/handlers/admin.go`
**Status**: ❌ Not Implemented (3 functions)
**Impact**: Low - Administrative notifications

**Functions**:
- `sendModerationEmail` (line 1493)
- `sendWelcomeEmail` (line 1513) 
- `sendRejectionEmail` (line 1533)

All return `nil` with comment: "Email sending via AWS SES not implemented in this version"

**Requirements**:
- AWS SES integration
- Email template system
- Delivery tracking
- Bounce handling

---

## Media Processing Stubs

### 7. Quality Tracking
**Files**: Various media processing files
**Status**: ⚠️ Hardcoded Values
**Impact**: Low - Media analytics

**Issues**:
- `pkg/media/streaming/streamer.go:360` - Hardcoded rebuffer metrics
- `pkg/media/analytics.go:529-530` - Placeholder cost calculations

### 8. DASH Streaming
**File**: `pkg/media/streaming/dash.go:216-225`
**Status**: ❌ Not Implemented
**Impact**: Low - Advanced video streaming

```go
func GenerateLiveMPD() error {
    return fmt.Errorf("live DASH not yet implemented")
}
```

**Requirements**:
- Dynamic manifest updates
- Sliding window segments
- Availability timing
- Presentation delay handling

### 9. Video Analysis
**Files**: Various streaming files
**Status**: ⚠️ Placeholder Analytics
**Impact**: Low - Media insights

**Issues**:
- Hardcoded analytics percentages
- Missing segment duration parsing
- Simplified keyframe detection

---

## Analytics Stubs

### 10. Federation Analytics
**File**: `pkg/media/analytics.go`
**Status**: ⚠️ Rough Estimates
**Impact**: Low - Operational insights

Placeholder calculations include:
- Per-user rate estimation
- Trending status determination
- Cost breakdown approximations

### 11. Streaming Analytics
**File**: `pkg/media/streaming.go:335`
**Status**: ⚠️ Hardcoded Metrics
**Impact**: Low - Performance monitoring

Hardcoded values:
- Quality breakdown percentages
- Geographic distribution estimates
- Buffer event rates
- Average watch times

---

## Implementation Priority Recommendations

### Phase 1: Core Functionality (High Priority)
1. **Direct Timeline Implementation** - Essential for private messaging
2. **Remote Resolution** - Critical for federation completeness

### Phase 2: Federation Enhancement (Medium Priority)
3. **Threading Sync** - Improves conversation completeness
4. **Quote Permissions** - Completes quote functionality

### Phase 3: Feature Completeness (Low Priority)
5. **Email Functionality** - Administrative convenience
6. **Media Processing** - Advanced streaming features
7. **Analytics** - Operational insights

### Phase 4: Optional Features
8. **External OAuth** - Based on architectural decisions

---

## Implementation Guidelines

### Before Starting Any Implementation:

1. **Review Dependencies**: Ensure all required components are available
2. **Check Storage Interface**: Verify necessary storage methods exist
3. **Test Infrastructure**: Confirm testing capabilities for the feature
4. **Documentation**: Update this document when starting/completing tasks

### Development Standards:

- Follow existing code patterns and conventions
- Include comprehensive error handling and logging
- Add appropriate cost tracking for DynamoDB operations
- Write unit tests for new functionality
- Update GraphQL schema if needed

### Testing Requirements:

- Unit tests for business logic
- Integration tests for external dependencies
- Load testing for performance-critical features
- Federation testing with real ActivityPub instances

---

## Notes

- **Federation Focus**: Lesser prioritizes ActivityPub compatibility over auxiliary features
- **Cost Consciousness**: All implementations should include DynamoDB cost tracking
- **Serverless Architecture**: Solutions must work within Lambda constraints
- **Real-time Considerations**: Some features may need WebSocket support

---

*Last Updated: 2025-01-22*
*Completed Stubs: 8/26 (31%)*
*Remaining Stubs: 18/26 (69%)*