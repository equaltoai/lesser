# Lift/DynamORM Migration Plan
## Complete Legacy AWS SDK to Lift/DynamORM Conversion

### Executive Summary

This plan outlines the systematic migration of 22 remaining Lambda functions from legacy AWS SDK patterns to Lift/DynamORM patterns. The migration is organized into 5 phases, prioritizing low-risk functions first and building expertise before tackling protocol-critical components.

**Target Outcome:** Remove all legacy `pkg/storage/dynamodb` dependencies and achieve 100% Lift/DynamORM coverage across the entire Lesser codebase.

---

## Current State Analysis

### ✅ **Already Migrated (8 functions)**
- `api` - Main REST API (full Lift implementation)
- `auth` - Authentication service (OAuth flows)
- `inbox` - ActivityPub inbox handler
- `outbox` - ActivityPub outbox handler  
- `push-delivery` - Push notification delivery
- `activity-processor` - Basic activity processing
- `ai-processor` - AI integration processing
- `search-indexer` - Search index management

### ❌ **Requires Migration (22 functions)**

#### **Phase 1: Simple Utilities (4 functions)**
- `webfinger` - WebFinger protocol handler
- `stream-router` - Stream routing logic
- `streaming` - WebSocket streaming management
- `timeline-fanout` - Timeline distribution

#### **Phase 2: Specialized Processors (6 functions)**
- `federation-tracker` - Federation activity tracking
- `federation-aggregator` - Federation metrics aggregation
- `metrics-aggregator` - System metrics collection
- `cost-aggregator` - Cost tracking aggregation
- `moderation-processor` - Content moderation
- `notification-processor` - Notification delivery

#### **Phase 3: Data Management (5 functions)**
- `media-processor` - Media upload/processing
- `timeline-processor` - Timeline generation
- `search-processor` - Search operation handling
- `analytics-processor` - Usage analytics
- `backup-processor` - Data backup operations

#### **Phase 4: Protocol Functions (4 functions)**
- `graphql` - GraphQL API server
- `federation-delivery` - ActivityPub delivery
- `federation-inbox-processor` - Inbox processing
- `federation-outbox-processor` - Outbox processing

#### **Phase 5: Resource Intensive (3 functions)**
- `migration-runner` - Data migration tasks
- `batch-processor` - Batch operation processing
- `maintenance-processor` - System maintenance

---

## Migration Strategy

### **Phase 1: Simple Utilities (Week 1)**
**Risk Level:** Low  
**Effort:** 2-3 days per function  
**Dependencies:** None

#### **1.1 webfinger**
- **Current:** Direct DynamoDB queries for user lookup
- **Target:** Use `UserRepository.GetUser()` and `ActorRepository.GetByUsername()`
- **Pattern:** Simple Lift handler with single repository dependency

```go
func Handler(ctx lift.Context) lift.Response {
    userRepo := repositories.NewUserRepository(db)
    // Convert to DynamORM calls
}
```

#### **1.2 stream-router**
- **Current:** DynamoDB stream event processing
- **Target:** Use appropriate repositories based on stream record type
- **Pattern:** Stream event router with repository factory pattern

#### **1.3 streaming**
- **Current:** WebSocket connection management with DynamoDB state
- **Target:** Use `SessionRepository` and `NotificationRepository`
- **Pattern:** WebSocket handler with real-time repository operations

#### **1.4 timeline-fanout**
- **Current:** Direct DynamoDB batch operations for timeline distribution
- **Target:** Use `TimelineRepository.BatchInsert()` operations
- **Pattern:** Batch processing with DynamORM optimized operations

### **Phase 2: Specialized Processors (Week 2-3)**
**Risk Level:** Medium  
**Effort:** 3-4 days per function  
**Dependencies:** Phase 1 patterns established

#### **2.1 federation-tracker**
- **Current:** Federation activity logging and metrics
- **Target:** Create `FederationActivityRepository` for tracking
- **New Repository Required:** ✅ High priority

#### **2.2 federation-aggregator**
- **Current:** Federation statistics aggregation
- **Target:** Use `FederationActivityRepository` with aggregation queries
- **Pattern:** Aggregation processor with GSI queries

#### **2.3 metrics-aggregator**
- **Current:** System metrics collection from various sources
- **Target:** Create `MetricsRepository` for centralized metrics storage
- **New Repository Required:** ✅ Medium priority

#### **2.4 cost-aggregator**
- **Current:** Cost tracking across all operations
- **Target:** Create `CostTrackingRepository` integrated with existing cost framework
- **New Repository Required:** ✅ High priority (integrates with existing cost tracking)

#### **2.5 moderation-processor**
- **Current:** Content moderation decisions and actions
- **Target:** Create `ModerationRepository` for moderation logs and decisions
- **New Repository Required:** ✅ High priority

#### **2.6 notification-processor**
- **Current:** Notification delivery and status tracking
- **Target:** Extend `NotificationRepository` with delivery tracking
- **Pattern:** Notification queue processor with status updates

### **Phase 3: Data Management (Week 4-5)**
**Risk Level:** Medium-High  
**Effort:** 4-5 days per function  
**Dependencies:** Core repositories from Phase 2

#### **3.1 media-processor**
- **Current:** Media upload, processing, and storage coordination
- **Target:** Use `MediaRepository` with S3 integration
- **Pattern:** Multi-stage processor (upload → process → store metadata)

#### **3.2 timeline-processor**
- **Current:** Timeline generation and maintenance
- **Target:** Use `TimelineRepository` with optimized query patterns
- **Pattern:** Timeline computation with batch operations

#### **3.3 search-processor**
- **Current:** Search indexing and query processing
- **Target:** Create `SearchRepository` for search operations
- **New Repository Required:** ✅ High complexity

#### **3.4 analytics-processor**
- **Current:** Usage analytics and reporting
- **Target:** Create `AnalyticsRepository` for analytics data
- **New Repository Required:** ✅ Medium priority

#### **3.5 backup-processor**
- **Current:** Data backup and archival operations
- **Target:** Use all repositories with backup-specific operations
- **Pattern:** Cross-repository backup coordinator

### **Phase 4: Protocol Functions (Week 6)**
**Risk Level:** High  
**Effort:** 5-7 days per function  
**Dependencies:** All core repositories must be stable

#### **4.1 graphql**
- **Current:** GraphQL server with direct DynamoDB resolvers
- **Target:** Convert all resolvers to use DynamORM repositories
- **Pattern:** GraphQL resolver integration with repository layer
- **Critical:** Must maintain exact API compatibility

#### **4.2 federation-delivery**
- **Current:** ActivityPub delivery with retry logic
- **Target:** Use `ActivityRepository` and `ActorRepository` with delivery tracking
- **Pattern:** Delivery processor with retry mechanisms

#### **4.3 federation-inbox-processor**
- **Current:** ActivityPub inbox message processing
- **Target:** Use multiple repositories for activity processing
- **Pattern:** Multi-repository activity processor

#### **4.4 federation-outbox-processor**
- **Current:** ActivityPub outbox message generation
- **Target:** Use activity and timeline repositories for outbox generation
- **Pattern:** Activity generation with timeline integration

### **Phase 5: Resource Intensive (Week 7)**
**Risk Level:** High  
**Effort:** 7-10 days per function  
**Dependencies:** All patterns proven stable

#### **5.1 migration-runner**
- **Current:** Data migration and schema changes
- **Target:** Use all repositories with migration-specific operations
- **Pattern:** Multi-repository migration coordinator

#### **5.2 batch-processor**
- **Current:** Large-scale batch operations
- **Target:** Use repository batch operations with DynamORM optimization
- **Pattern:** Optimized batch processor with cost awareness

#### **5.3 maintenance-processor**
- **Current:** System maintenance and cleanup
- **Target:** Use all repositories for maintenance operations
- **Pattern:** System-wide maintenance coordinator

---

## Repository Requirements

### **New Repositories Required**

#### **FederationActivityRepository**
```go
type FederationActivityRepository struct {
    db core.DB
}

// Track federation activities and metrics
func (r *FederationActivityRepository) LogActivity(ctx context.Context, activity *FederationActivity) error
func (r *FederationActivityRepository) GetMetrics(ctx context.Context, timeRange TimeRange) (*FederationMetrics, error)
```

#### **MetricsRepository**
```go
type MetricsRepository struct {
    db core.DB
}

// Store and retrieve system metrics
func (r *MetricsRepository) RecordMetric(ctx context.Context, metric *SystemMetric) error
func (r *MetricsRepository) GetAggregatedMetrics(ctx context.Context, period Period) ([]*AggregatedMetric, error)
```

#### **CostTrackingRepository**
```go
type CostTrackingRepository struct {
    db core.DB
}

// Integration with existing cost framework
func (r *CostTrackingRepository) RecordCost(ctx context.Context, cost *OperationCost) error
func (r *CostTrackingRepository) GetCostSummary(ctx context.Context, period Period) (*CostSummary, error)
```

#### **ModerationRepository**
```go
type ModerationRepository struct {
    db core.DB
}

// Content moderation and decisions
func (r *ModerationRepository) RecordModerationAction(ctx context.Context, action *ModerationAction) error
func (r *ModerationRepository) GetModerationHistory(ctx context.Context, contentID string) ([]*ModerationAction, error)
```

#### **SearchRepository**
```go
type SearchRepository struct {
    db core.DB
}

// Search operations and indexing
func (r *SearchRepository) IndexContent(ctx context.Context, content *SearchableContent) error
func (r *SearchRepository) Search(ctx context.Context, query *SearchQuery) (*SearchResults, error)
```

#### **AnalyticsRepository**
```go
type AnalyticsRepository struct {
    db core.DB
}

// Usage analytics and reporting
func (r *AnalyticsRepository) RecordEvent(ctx context.Context, event *AnalyticsEvent) error
func (r *AnalyticsRepository) GetReport(ctx context.Context, reportType ReportType) (*AnalyticsReport, error)
```

---

## Standard Implementation Patterns

### **Canonical Lift Handler Pattern**
```go
package main

import (
    "github.com/equaltoai/lesser/pkg/lift"
    "github.com/equaltoai/lesser/pkg/storage/repositories"
    "github.com/pay-theory/dynamorm/pkg/core"
)

func Handler(ctx lift.Context) lift.Response {
    // 1. Initialize database connection
    db, err := core.NewDB(config)
    if err != nil {
        return lift.ErrorResponse(500, "Database connection failed")
    }

    // 2. Initialize repositories
    userRepo := repositories.NewUserRepository(db)
    
    // 3. Parse request
    var request RequestType
    if err := ctx.ParseJSON(&request); err != nil {
        return lift.ErrorResponse(400, "Invalid request")
    }

    // 4. Business logic with DynamORM operations
    result, err := userRepo.GetUser(ctx.Context, request.UserID)
    if err != nil {
        return lift.ErrorResponse(500, "Operation failed")
    }

    // 5. Return response
    return lift.JSONResponse(200, result)
}

func main() {
    lift.Start(
        lift.WithHandler(Handler),
        lift.WithMiddleware(
            lift.CORSMiddleware(),
            lift.LoggingMiddleware(),
            lift.ErrorMiddleware(),
        ),
    )
}
```

### **DynamORM Repository Usage Pattern**
```go
// Repository initialization
db, err := core.NewDB(config)
if err != nil {
    return fmt.Errorf("failed to initialize database: %w", err)
}

// Repository operations
repo := repositories.NewUserRepository(db)

// Create operation
user := &models.User{Username: "example"}
if err := repo.CreateUser(ctx, user); err != nil {
    return fmt.Errorf("failed to create user: %w", err)
}

// Query operation
user, err := repo.GetUser(ctx, "username")
if err != nil {
    return fmt.Errorf("failed to get user: %w", err)
}

// Update operation
updates := map[string]any{"email": "new@example.com"}
if err := repo.UpdateUser(ctx, "username", updates); err != nil {
    return fmt.Errorf("failed to update user: %w", err)
}
```

### **Error Handling Pattern**
```go
func handleDynamORMError(err error) lift.Response {
    if errors.IsNotFound(err) {
        return lift.ErrorResponse(404, "Resource not found")
    }
    if errors.IsConditionFailed(err) {
        return lift.ErrorResponse(409, "Resource conflict")
    }
    if errors.IsValidationError(err) {
        return lift.ErrorResponse(400, "Validation failed")
    }
    
    // Log unexpected errors
    log.Printf("Unexpected database error: %v", err)
    return lift.ErrorResponse(500, "Internal server error")
}
```

### **Middleware Configuration**
```go
lift.Start(
    lift.WithHandler(Handler),
    lift.WithMiddleware(
        // Request processing order
        lift.CORSMiddleware(),           // CORS headers
        lift.LoggingMiddleware(),        // Request logging
        lift.AuthMiddleware(),           // Authentication
        lift.RateLimitMiddleware(),      // Rate limiting
        lift.ValidationMiddleware(),     // Input validation
        lift.ErrorMiddleware(),          // Error handling
        lift.CostTrackingMiddleware(),   // Cost monitoring
    ),
)
```

---

## Testing Strategy

### **Phase Testing Requirements**

#### **Unit Tests**
- Each migrated function must have 90%+ test coverage
- Repository mocking using DynamORM test utilities
- Error condition testing for all code paths

#### **Integration Tests**
- End-to-end tests for each function with real DynamoDB
- Cross-repository operation testing
- Performance regression testing

#### **Migration Validation**
- Before/after functionality comparison
- Performance benchmarking
- Protocol compliance verification (especially for federation functions)

### **Testing Tools**
```go
// Use DynamORM test utilities
import "github.com/pay-theory/dynamorm/pkg/testing"

func TestUserRepository(t *testing.T) {
    db := testing.NewMockDB()
    repo := repositories.NewUserRepository(db)
    
    // Test repository operations
}
```

---

## Risk Mitigation

### **High-Risk Functions**
1. **graphql** - Critical API compatibility
2. **federation-delivery** - ActivityPub protocol compliance
3. **federation-inbox-processor** - Protocol message handling
4. **federation-outbox-processor** - Protocol message generation

### **Mitigation Strategies**

#### **Feature Flags**
- Deploy new Lift versions alongside legacy
- Gradual traffic shifting using feature flags
- Immediate rollback capability

#### **Comprehensive Testing**
- Protocol compliance test suite
- Load testing with realistic traffic patterns
- Federation interoperability testing with real ActivityPub servers

#### **Monitoring**
- Detailed metrics during migration
- Error rate monitoring
- Performance regression detection
- Cost impact tracking

#### **Rollback Plan**
- Maintain legacy code until migration proven stable
- Automated rollback triggers on error thresholds
- Database operation rollback procedures

---

## Success Criteria

### **Technical Criteria**
- [ ] 100% Lift/DynamORM coverage across all Lambda functions
- [ ] Complete removal of `pkg/storage/dynamodb` dependencies
- [ ] No performance regressions (< 5% latency increase)
- [ ] No cost regressions (< 10% cost increase)
- [ ] 90%+ test coverage for all migrated functions

### **Functional Criteria**
- [ ] All Mastodon API endpoints maintain compatibility
- [ ] ActivityPub federation maintains protocol compliance
- [ ] GraphQL API maintains schema compatibility
- [ ] Real-time features (streaming, notifications) maintain performance
- [ ] Search functionality maintains accuracy and performance

### **Operational Criteria**
- [ ] Monitoring and alerting functional for all new implementations
- [ ] Error handling and logging maintain or improve current standards
- [ ] Cost tracking integration functional across all operations
- [ ] Documentation updated for all migrated functions

---

## Timeline

### **7-Week Migration Schedule**

**Week 1:** Phase 1 - Simple Utilities (4 functions)
**Week 2:** Phase 2A - Basic Processors (3 functions)
**Week 3:** Phase 2B - Advanced Processors (3 functions)
**Week 4:** Phase 3A - Data Management (3 functions)
**Week 5:** Phase 3B - Data Management (2 functions)
**Week 6:** Phase 4 - Protocol Functions (4 functions)
**Week 7:** Phase 5 - Resource Intensive (3 functions)

### **Milestones**
- **Week 2:** First repository creations validated
- **Week 4:** Core data operations migrated
- **Week 6:** Protocol compliance verified
- **Week 7:** Complete legacy code removal

---

## Implementation Checklist

### **Pre-Migration Setup**
- [ ] Create new repository interfaces
- [ ] Establish testing framework for DynamORM
- [ ] Set up feature flag infrastructure
- [ ] Create monitoring dashboards for migration progress

### **Per-Function Migration**
- [ ] Analyze current function dependencies
- [ ] Create DynamORM repository interfaces needed
- [ ] Implement Lift handler with proper middleware
- [ ] Create comprehensive test suite
- [ ] Deploy with feature flag (off by default)
- [ ] Validate functionality with integration tests
- [ ] Enable feature flag for gradual traffic
- [ ] Monitor performance and errors
- [ ] Complete traffic migration
- [ ] Remove legacy code dependencies

### **Post-Migration Cleanup**
- [ ] Remove all `pkg/storage/dynamodb` imports
- [ ] Delete legacy storage implementation files
- [ ] Update documentation and deployment guides
- [ ] Conduct final performance and cost analysis
- [ ] Celebrate successful migration! 🎉

---

## Expected Outcomes

### **Code Reduction**
- **Estimated removal:** 25,000+ lines of legacy storage code
- **Maintenance reduction:** Single pattern for all data operations
- **Testing simplification:** Unified test patterns across all functions

### **Performance Benefits**
- **DynamORM optimizations:** Built-in query optimization and caching
- **Lift framework benefits:** Standardized middleware and error handling
- **Cost tracking integration:** Automatic cost monitoring across all operations

### **Development Velocity**
- **Consistent patterns:** All functions follow same Lift/DynamORM patterns
- **Reduced context switching:** Single framework approach
- **Better testing:** Unified testing patterns and utilities
- **Easier debugging:** Consistent error handling and logging

This migration plan provides a systematic approach to achieving 100% Lift/DynamORM coverage while minimizing risk and maintaining operational stability throughout the process.