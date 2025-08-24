# Phase 2 Repository Pattern Refactoring - Complete Codebase Enhancement

## Session Objective
**Systematically refactor ALL 82 repository files** to implement enhanced patterns and eliminate boilerplate across the entire Lesser pre-release codebase.

---

## Phase 2 Overview & Context

### 🎯 Strategic Goals
**Primary Objective**: Refactor ALL repository patterns for consistency and maintainability  
**Timeline**: 3-4 weeks  
**Impact**: 20-30% reduction in repository code across entire codebase  
**Risk Level**: Low-Medium (pre-release refactoring, no production constraints)  

### 📊 Current State Analysis
**Repository Statistics**:
- **82 repository files** requiring complete refactoring
- **Existing BaseRepository foundation** provides starting point
- **DynamORM integration** with cost tracking established
- **Pre-release status** allows aggressive refactoring

**Refactoring Opportunities**:
- **Eliminate boilerplate**: Common CRUD operations with repetitive patterns
- **Standardize validation**: Scattered validation logic across repositories
- **Unify error handling**: Leverage Phase 1 centralized error system
- **Consolidate queries**: Similar pagination and filtering implementations
- **Enhance patterns**: Cost tracking, caching, event emission consistency

---

## Phase 2 Implementation Strategy

### Phase 2.1: Enhanced BaseRepository Foundation (Week 1)
**Goal**: Create comprehensive enhanced base repository for ALL repositories

### Phase 2.2: Systematic Repository Refactoring (Weeks 2-3)
**Goal**: Refactor ALL 82 repositories using batched approach for consistency

### Phase 2.3: Advanced Pattern Implementation (Week 3)
**Goal**: Implement specialized patterns and caching across entire codebase

### Phase 2.4: Comprehensive Validation (Week 4)
**Goal**: Validate entire refactored repository layer for consistency and performance

---

## Current Repository Architecture Analysis

### Existing BaseRepository Structure
```go
// Current BaseRepository (pkg/storage/repositories/base_repository.go)
type BaseRepository[T BaseModel] struct {
    db          core.DB
    tableName   string
    logger      *zap.Logger
    costService *cost.TrackingService
    repoName    string
}
```

**Current Capabilities**:
- ✅ Generic CRUD operations with DynamORM
- ✅ Cost tracking integration
- ✅ Logging support
- ✅ Type safety with generics

**Enhancement Opportunities**:
- 🔄 Common validation patterns
- 🔄 Permission checking standardization
- 🔄 Query pattern standardization
- 🔄 Caching integration
- 🔄 Event emission patterns

### Repository Usage Patterns
**High-Complexity, High-Usage** (Priority for enhancement):
1. `account_repository.go` - User/Actor unified operations
2. `status_repository.go` - Content management
3. `notification_repository.go` - Push notifications
4. `relationship_repository.go` - Social connections
5. `activity_repository.go` - ActivityPub activities

**Medium-Complexity** (Secondary migration):
6. `timeline_repository.go` - Timeline operations
7. `media_repository.go` - Media management
8. `hashtag_repository.go` - Hashtag operations
9. `search_repository.go` - Search functionality
10. `auth_repository.go` - Authentication

---

## Phase 2.1: Enhanced BaseRepository Implementation

### Task 2.1.1: Repository Pattern Analysis
**Objective**: Analyze and document common patterns across 88 repositories

**Implementation Steps**:

1. **Pattern Analysis Script**:
```bash
# Analyze common method signatures
echo "=== Common Repository Method Patterns ==="
grep -r "func.*Repository.*Create" pkg/storage/repositories/ | \
  awk '{print $2}' | sort | uniq -c | sort -nr > /tmp/create_patterns.txt

grep -r "func.*Repository.*Update" pkg/storage/repositories/ | \
  awk '{print $2}' | sort | uniq -c | sort -nr > /tmp/update_patterns.txt

grep -r "func.*Repository.*Delete" pkg/storage/repositories/ | \
  awk '{print $2}' | sort | uniq -c | sort -nr > /tmp/delete_patterns.txt

grep -r "func.*Repository.*Find" pkg/storage/repositories/ | \
  awk '{print $2}' | sort | uniq -c | sort -nr > /tmp/find_patterns.txt

echo "Create patterns:"; cat /tmp/create_patterns.txt | head -10
echo "Update patterns:"; cat /tmp/update_patterns.txt | head -10  
echo "Delete patterns:"; cat /tmp/delete_patterns.txt | head -10
echo "Find patterns:"; cat /tmp/find_patterns.txt | head -10
```

2. **Validation Pattern Analysis**:
```bash
# Find common validation patterns
echo "=== Common Validation Patterns ==="
grep -r "validate\|Validate" pkg/storage/repositories/*.go | \
  grep -v test | head -20

# Find permission checking patterns  
echo "=== Permission Checking Patterns ==="
grep -r "permission\|Permission\|auth\|Auth" pkg/storage/repositories/*.go | \
  grep -v test | head -20

# Find error handling patterns (post Phase 1)
echo "=== Error Handling Patterns ==="
grep -r "errors\." pkg/storage/repositories/*.go | \
  grep -v test | head -20
```

3. **Documentation Creation**:
Create `REPOSITORY_PATTERN_ANALYSIS.md` with:
- Common method signatures and their frequencies
- Validation patterns across repositories
- Error handling patterns (enhanced by Phase 1)
- Permission checking approaches
- Caching usage patterns
- Event emission patterns

### Task 2.1.2: Enhanced BaseRepository Design
**Objective**: Create enhanced base repository with comprehensive functionality

**Implementation**:

1. **Create Enhanced BaseRepository**:
```go
// File: pkg/storage/repositories/enhanced_base_repository.go
package repositories

import (
    "context"
    
    "github.com/equaltoai/lesser/pkg/common"
    "github.com/equaltoai/lesser/pkg/cost"
    "github.com/equaltoai/lesser/pkg/errors"
    "go.uber.org/zap"
)

// ValidationService provides standardized validation across repositories
type ValidationService interface {
    ValidateModel(ctx context.Context, model BaseModel) error
    ValidateBusinessRules(ctx context.Context, model BaseModel, action string) error
    ValidateRequiredFields(ctx context.Context, model BaseModel) error
}

// PermissionService provides standardized permission checking
type PermissionService interface {
    CheckPermissions(ctx context.Context, actor string, action string, resource BaseModel) error
    HasPermission(ctx context.Context, actor string, permission string) bool
}

// CachingService provides repository-level caching
type CachingService interface {
    Get(ctx context.Context, key string, dest interface{}) error
    Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    InvalidatePattern(ctx context.Context, pattern string) error
}

// EventService provides standardized event emission
type EventService interface {
    Emit(ctx context.Context, event Event) error
    EmitAsync(ctx context.Context, event Event) error
}

// EnhancedBaseRepository provides comprehensive CRUD + business logic patterns
type EnhancedBaseRepository[T BaseModel] struct {
    *BaseRepository[T] // Embed existing functionality
    
    // Enhanced services
    validator   ValidationService
    permissions PermissionService
    caching     CachingService
    events      EventService
}
```

2. **Implement Common Validation Methods**:
```go
// Common validation patterns used by 90%+ repositories
func (r *EnhancedBaseRepository[T]) ValidateAndCreate(ctx context.Context, model T) error {
    // 1. Validate required fields
    if err := r.validator.ValidateRequiredFields(ctx, model); err != nil {
        return errors.ValidationFailed("required fields", err)
    }
    
    // 2. Validate business rules
    if err := r.validator.ValidateBusinessRules(ctx, model, "create"); err != nil {
        return errors.ValidationFailed("business rules", err)
    }
    
    // 3. Check permissions
    if err := r.checkCreatePermissions(ctx, model); err != nil {
        return err
    }
    
    // 4. Execute create with cost tracking
    if err := r.Create(ctx, model); err != nil {
        return err
    }
    
    // 5. Emit creation event
    if r.events != nil {
        _ = r.events.EmitAsync(ctx, NewCreatedEvent(model))
    }
    
    return nil
}

func (r *EnhancedBaseRepository[T]) ValidateAndUpdate(ctx context.Context, model T) error {
    // Similar pattern for updates
    // ... implementation
}

func (r *EnhancedBaseRepository[T]) ValidatePermissions(ctx context.Context, action string, model T) error {
    if r.permissions == nil {
        return nil // No permission service configured
    }
    
    actor := common.GetActorFromContext(ctx)
    if actor == "" {
        return errors.AuthenticationRequired()
    }
    
    return r.permissions.CheckPermissions(ctx, actor, action, model)
}
```

3. **Implement Common Query Patterns**:
```go
// Query patterns used by 80%+ repositories
func (r *EnhancedBaseRepository[T]) FindWithPagination(ctx context.Context, query Query, paginate PaginationParams) (*PaginatedResult[T], error) {
    // Standardized pagination implementation
    // ... implementation
}

func (r *EnhancedBaseRepository[T]) FindByMultipleFields(ctx context.Context, filters map[string]any) ([]T, error) {
    // Standardized multi-field query implementation
    // ... implementation
}

func (r *EnhancedBaseRepository[T]) CountWhere(ctx context.Context, conditions map[string]any) (int64, error) {
    // Standardized counting implementation
    // ... implementation
}

func (r *EnhancedBaseRepository[T]) FindWithCache(ctx context.Context, key string, query Query) ([]T, error) {
    if r.caching != nil {
        // Try cache first
        var cached []T
        if err := r.caching.Get(ctx, key, &cached); err == nil {
            return cached, nil
        }
    }
    
    // Execute query
    results, err := r.ExecuteQuery(ctx, query)
    if err != nil {
        return nil, err
    }
    
    // Cache results if caching is enabled
    if r.caching != nil && len(results) > 0 {
        _ = r.caching.Set(ctx, key, results, time.Hour) // 1 hour default TTL
    }
    
    return results, nil
}
```

---

## Phase 2.2: High-Impact Repository Migration Implementation

### Task 2.2.1: Complete Repository Refactoring Strategy
**Objective**: Systematically refactor ALL 82 repositories using batched approach

**Refactoring Batches** (Complete coverage, no repositories left behind):

**Batch 1: Core Foundation Repositories** (Week 2.1 - 15 repos):
- `base_repository.go` - Enhanced foundation implementation
- `account_repository*.go` - All 8 account-related repositories  
- `user_repository.go` - User management
- `actor_repository.go` - ActivityPub actors
- `auth_repository.go` - Authentication core
- `status_repository.go` - Content management
- `activity_repository.go` - ActivityPub activities
- `object_repository.go` - Generic objects

**Batch 2: Social & Federation Repositories** (Week 2.2 - 20 repos):
- `relationship_repository.go` - Social connections
- `notification_repository.go` - Push notifications  
- `timeline_repository.go` - Timeline operations
- `like_repository.go` - Likes/favorites
- `block_repository.go` - Blocking functionality
- `mute_repository.go` - Muting functionality
- `federation_*.go` - All 4 federation repositories
- `hashtag_repository.go` - Hashtag management
- `featured_tag_repository.go` - Featured tags
- `bookmark_repository.go` - Bookmarks
- `list_repository.go` - User lists
- `conversation_repository.go` - Conversations
- `quote_repository.go` - Quote functionality
- `announcement_repository.go` - Announcements
- `community_note_repository.go` - Community notes
- `social_repository.go` - Social features

**Batch 3: Content & Media Repositories** (Week 2.3 - 15 repos):
- `media_*.go` - All 4 media repositories
- `poll_repository.go` - Polling functionality
- `scheduled_status_repository.go` - Scheduled posts
- `marker_repository.go` - Read markers
- `filter_repository.go` - Content filters
- `pattern_repository.go` - Content patterns
- `enhanced_pattern_repository.go` - Enhanced patterns
- `moderation_*.go` - All 2 moderation repositories
- `emoji_repository.go` - Custom emojis
- `search_*.go` - All 2 search repositories

**Batch 4: Infrastructure & Metrics Repositories** (Week 2.4 - 17 repos):
- `metrics_repository.go` - Core metrics
- `analytics_repository.go` - Analytics
- `cost_tracking_repository.go` - Cost tracking
- `*_cost_repository.go` - All 4 cost repositories
- `cloudwatch_metrics_repository.go` - CloudWatch
- `routing_metrics_repository.go` - Routing metrics
- `instance_*.go` - All 2 instance repositories
- `circuit_breaker_repository.go` - Circuit breaking
- `rate_limit_repository.go` - Rate limiting
- `audit_repository.go` - Auditing
- `threat_intel_repository.go` - Security
- `trust_repository.go` - Trust scoring
- `alert_repository.go` - Alerting

**Batch 5: Support & Utility Repositories** (Week 2.5 - 15 repos):
- `streaming_*.go` - All 3 streaming repositories
- `websocket_*.go` - All 2 WebSocket repositories  
- `oauth_*.go` - All 2 OAuth repositories
- `auth_refresh_token_repository.go` - Token management
- `recovery_repository.go` - Account recovery
- `wallet_repository.go` - Wallet functionality
- `push_subscription_repository.go` - Push subscriptions
- `csrf_repository.go` - CSRF protection
- `query_cache_repository.go` - Query caching
- `public_key_cache_repository.go` - Key caching
- `dns_cache_repository.go` - DNS caching
- `dlq_repository.go` - Dead letter queue
- `domain_block_repository.go` - Domain blocking
- `relay_repository.go` - ActivityPub relays
- `route_optimizer_repository.go` - Route optimization

### Task 2.2.2: Repository Migration Template
**Objective**: Create standardized migration approach

**Migration Template**:
```go
// BEFORE: Traditional repository structure
type AccountRepository struct {
    *BaseRepository[*models.User]
    db        core.DB
    logger    *zap.Logger
    tableName string
    domain    string
    
    // Custom validation logic scattered throughout
    // Custom permission checks in each method
    // Manual error handling
    // No caching integration
}

func (r *AccountRepository) CreateUser(ctx context.Context, user *models.User) error {
    // Manual validation
    if user.Username == "" {
        return errors.NewValidationError("username", "required")
    }
    
    // Manual permission check  
    if !r.hasCreatePermission(ctx) {
        return errors.NewAuthError("insufficient permissions")
    }
    
    // Manual business rule validation
    if exists, err := r.checkUsernameExists(ctx, user.Username); err != nil {
        return err
    } else if exists {
        return errors.NewConflictError("username already exists")
    }
    
    // Create user
    if err := r.Create(ctx, user); err != nil {
        return err
    }
    
    // Manual event emission
    r.emitUserCreatedEvent(ctx, user)
    
    return nil
}

// AFTER: Enhanced repository structure  
type AccountRepository struct {
    *EnhancedBaseRepository[*models.User] // Enhanced functionality
    
    // Repository-specific services remain
    domain      string
    statusRepo  interfaces.StatusRepository
    
    // Business logic helpers (reduced)
}

func (r *AccountRepository) CreateUser(ctx context.Context, user *models.User) error {
    // All common patterns handled by enhanced base
    return r.ValidateAndCreate(ctx, user)
}

// Only repository-specific business logic remains
func (r *AccountRepository) GetUserWithStats(ctx context.Context, username string) (*models.UserWithStats, error) {
    // Repository-specific functionality that can't be generalized
    // ... implementation
}
```

### Task 2.2.2: Batch Refactoring Execution Process  
**Objective**: Systematic refactoring with complete coverage

**Per-Batch Process** (Applied to ALL 82 repositories):
1. **Batch Analysis**: Document patterns within batch repositories
2. **Enhanced Implementation**: Refactor entire batch to enhanced patterns
3. **Compilation Validation**: Ensure all batch repositories compile successfully
4. **Pattern Consistency**: Verify consistent enhanced patterns across batch
5. **Integration Testing**: Validate StorageAdapter integration maintained
6. **Performance Validation**: Ensure enhanced patterns don't degrade performance
7. **Next Batch**: Move to next batch once current batch fully validated

**Aggressive Refactoring Approach** (Pre-release advantages):
- **Direct Replacement**: Replace implementations directly (no parallel versions)
- **Consistent Patterns**: Apply identical enhanced patterns across all repositories
- **Complete Coverage**: No repository left using old patterns
- **Unified Validation**: Test entire repository layer as cohesive unit

---

## Phase 2.3: Advanced Repository Patterns Implementation

### Task 2.3.1: Specialized Repository Classes
**Objective**: Create domain-specific enhanced base classes

**Specialized Classes**:

1. **Content Repository** (for Status, Note, Media repositories):
```go
type EnhancedContentRepository[T BaseModel] struct {
    *EnhancedBaseRepository[T]
    
    // Content-specific services
    searchService     SearchService
    contentAnalyzer   ContentAnalyzer
    moderationService ModerationService
}

func (r *EnhancedContentRepository[T]) CreateWithModeration(ctx context.Context, content T) error {
    // Content-specific validation and moderation
    // ... implementation
}
```

2. **Relationship Repository** (for Follow, Block, Mute repositories):
```go
type EnhancedRelationshipRepository[T BaseModel] struct {
    *EnhancedBaseRepository[T]
    
    // Relationship-specific services
    graphService    GraphService
    cacheManager    RelationshipCacheService
    federationSync  FederationSyncService
}

func (r *EnhancedRelationshipRepository[T]) CreateWithFederation(ctx context.Context, relationship T) error {
    // Relationship-specific logic with federation sync
    // ... implementation  
}
```

### Task 2.3.2: Caching Layer Integration
**Objective**: Add standardized caching to high-traffic repositories

**Implementation**:
- Cache-aside pattern for read operations
- Cache invalidation on write operations  
- Pattern-based cache invalidation
- TTL configuration per repository type

---

## Phase 2.4: Testing and Performance Validation

### Task 2.4.1: Comprehensive Repository Testing
**Objective**: Ensure enhanced repositories maintain functionality and improve performance

**Testing Strategy**:
1. **Unit Tests**: Test enhanced repository methods in isolation
2. **Integration Tests**: Test StorageAdapter integration
3. **Performance Tests**: Benchmark enhanced vs original repositories
4. **Migration Tests**: Verify migrated repositories behave identically

### Task 2.4.2: Performance Benchmarking
**Objective**: Validate enhancement benefits

**Benchmarks**:
- Repository operation performance (CRUD)
- Code complexity reduction measurement
- Memory usage analysis
- DynamoDB cost tracking verification

---

## Success Criteria & Validation

### ✅ Phase 2 Complete When:
1. **Enhanced BaseRepository**: Comprehensive functionality implemented and deployed
2. **ALL 82 Repositories**: Every repository refactored to enhanced patterns
3. **Specialized Classes**: Content, Social, Infrastructure specialized bases implemented  
4. **Caching Integration**: Cache-aside pattern integrated across entire repository layer
5. **Complete Validation**: All repositories compile, test, and perform consistently
6. **Unified Documentation**: Complete repository enhancement documentation

### 📊 Success Metrics:
- **100% repository coverage**: All 82 repositories using enhanced patterns
- **20-30% reduction** in total repository boilerplate code
- **Consistent patterns** across entire codebase (no exceptions)
- **Centralized logic** implemented universally (validation, caching, permissions)
- **Unified architecture** with no legacy patterns remaining

### 🎯 Expected Outcomes:
- **Complete consistency**: Every repository follows identical enhanced patterns
- **Massive boilerplate elimination**: Duplicated logic centralized and reused
- **Enhanced maintainability**: Single point of enhancement for all repositories  
- **Improved developer experience**: Consistent API across all data operations
- **Solid foundation**: Optimal base for Phase 3 (Test Coverage Expansion)
- **Pre-release advantage**: No legacy compatibility constraints

---

## Risk Management & Safety Measures

### ⚠️ Risk Factors:
- **Large Scope**: Refactoring all 82 repositories simultaneously
- **Interface Preservation**: Must maintain StorageAdapter compatibility
- **Pattern Consistency**: Risk of introducing inconsistencies across refactoring
- **Performance Impact**: Enhanced patterns applied universally

### ✅ Mitigation Strategies:
- **Batch Processing**: Systematic 5-batch approach for manageable scope
- **Pre-release Advantage**: No production constraints allow aggressive refactoring
- **Comprehensive Validation**: Batch-by-batch compilation and testing
- **Pattern Templates**: Consistent refactoring templates across all repositories
- **Git Safety**: Each batch committed separately for easy rollback if needed
- **Unified Testing**: Complete repository layer validation at each batch completion

---

## Copy-Paste Ready Implementation Commands

### Phase 2.1 Setup Commands
```bash
# Create enhanced repository structure
mkdir -p /tmp/phase2_analysis

# Analyze repository patterns
echo "=== Repository Pattern Analysis ==="
echo "Total repositories: $(find pkg/storage/repositories -name '*repository*.go' -not -name '*test*' | wc -l)"

# Analyze common patterns
grep -r "func.*Create" pkg/storage/repositories/*.go | grep -v test | \
  awk '{print $2}' | sort | uniq -c | sort -nr > /tmp/phase2_analysis/create_patterns.txt

grep -r "func.*Update" pkg/storage/repositories/*.go | grep -v test | \
  awk '{print $2}' | sort | uniq -c | sort -nr > /tmp/phase2_analysis/update_patterns.txt

echo "Common Create patterns:"
head -10 /tmp/phase2_analysis/create_patterns.txt

echo "Common Update patterns:"
head -10 /tmp/phase2_analysis/update_patterns.txt
```

### Phase 2 Validation Commands
```bash
# Pre-refactoring validation
echo "=== Phase 2 Pre-Refactoring Validation ==="

# Verify Phase 1 completion (prerequisite)
echo "Phase 1 Status: $(find . -name 'errors.go' -exec grep -l 'github.com/equaltoai/lesser/pkg/errors' {} \; | wc -l)/$(find . -name 'errors.go' | wc -l) files migrated"

# Verify current compilation
JWT_SECRET=test go build ./pkg/storage/repositories && echo "✅ All current repositories compile successfully"

# Complete repository inventory 
echo "=== Complete Repository Inventory ==="
echo "Total repository files: $(find pkg/storage/repositories -name '*repository*.go' -not -name '*test*' -not -name '*helpers*' -not -name '*example*' | wc -l)"
find pkg/storage/repositories -name '*repository*.go' -not -name '*test*' -not -name '*helpers*' -not -name '*example*' | sort > /tmp/all_repositories.txt
echo "Full repository list created at: /tmp/all_repositories.txt"

# Baseline metrics for refactoring measurement
echo "=== Baseline Metrics ==="
echo "Total repository code lines: $(find pkg/storage/repositories -name '*repository*.go' -not -name '*test*' -not -name '*helpers*' -not -name '*example*' -exec wc -l {} + | tail -1 | awk '{print $1}')"
echo "BaseRepository usages: $(grep -r 'BaseRepository\[' pkg/storage/repositories/*.go | wc -l)"
```

---

## Next Steps After Phase 2 Completion

### Phase 3 Readiness:
Once Phase 2 reaches 100%:
- Enhanced repository patterns provide foundation for comprehensive testing
- Standardized validation patterns enable test template creation
- Reduced boilerplate makes test coverage expansion more manageable
- Performance benchmarking infrastructure ready for test performance validation

### Long-term Benefits:
- **Maintainability**: Centralized repository patterns
- **Developer Experience**: Consistent API across all repositories
- **Performance**: Optimized caching and query patterns
- **Testing**: Standardized testing approaches
- **Documentation**: Clear repository development guidelines

---

**🚀 Ready to Begin Phase 2 Implementation!**

*Estimated Phase Duration: 3-4 weeks*  
*Risk Level: Medium (manageable with proper testing)*  
*Expected Code Reduction: 20-30%*  
*Foundation for Phase 3: Test Coverage Expansion*