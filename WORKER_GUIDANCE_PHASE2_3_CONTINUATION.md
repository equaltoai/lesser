# Phase 2.3 Error Handling Standardization - Continuation Phase

## COMPLETED WORK ✅
- **Repository Layer**: Fully standardized with pkg/storage/errors.go (8 error constants, 113+ usages)
- **HTTP Response Layer**: Comprehensive common.error_responses.go with 20+ standardized functions
- **Foundation**: Solid architectural patterns established

## CURRENT STATUS
**134 files** still use fmt.Errorf() in core service layers (cmd/, graph/, pkg/services/, pkg/auth/, pkg/federation/, pkg/activitypub/)
**Target**: Eliminate inconsistent error handling across ALL service layers

## PHASE 2.3 CONTINUATION OBJECTIVES

### 🎯 Service Layer Error Standardization
**Goal**: Migrate 134 files from fmt.Errorf() to standardized error patterns

**Priority Areas**:
1. **cmd/api/lift/** - API endpoint handlers (highest impact)
2. **pkg/services/** - Business logic services  
3. **pkg/auth/** - Authentication services
4. **pkg/federation/** - ActivityPub federation
5. **graph/** - GraphQL resolvers
6. **pkg/activitypub/** - Protocol implementation

### 🔧 Implementation Strategy

#### Step 1: Service Layer Error Constants
Create standardized error constants for each domain:

```go
// pkg/services/errors.go
var (
    ErrServiceUnavailable = errors.New("service temporarily unavailable")
    ErrInvalidOperation   = errors.New("invalid operation")
    ErrResourceExhausted  = errors.New("resource limit exceeded")
)

// pkg/auth/errors.go  
var (
    ErrInvalidCredentials = errors.New("invalid credentials")
    ErrAccountLocked     = errors.New("account locked")
    ErrInvalidScope      = errors.New("invalid scope")
)

// pkg/federation/errors.go
var (
    ErrInvalidSignature  = errors.New("invalid ActivityPub signature")
    ErrInstanceBlocked   = errors.New("instance blocked")
    ErrDeliveryFailed    = errors.New("delivery failed")
)
```

#### Step 2: Eliminate fmt.Errorf() Patterns
**Replace**:
```go
return fmt.Errorf("user not found: %s", userID)
```
**With**:
```go
return storage.ErrNotFound
// OR for context:
return fmt.Errorf("user %s: %w", userID, storage.ErrNotFound)
```

#### Step 3: HTTP Error Response Consolidation
**Replace inline responses**:
```go
return ctx.Status(400).JSON(map[string]string{"error": "invalid request"})
```
**With centralized functions**:
```go
return common.RespondBadRequest(ctx, "invalid request")
```

### 📋 IMPLEMENTATION TASKS

#### Task 2.3.1: API Layer Standardization
- **Target**: cmd/api/lift/ handlers
- **Action**: Replace inline ctx.Status().JSON() with common.Respond* functions
- **Goal**: Consistent API error responses across all endpoints

#### Task 2.3.2: Service Layer Error Constants
- **Target**: pkg/services/, pkg/auth/, pkg/federation/
- **Action**: Create domain-specific error.go files with constants
- **Goal**: Eliminate fmt.Errorf() in business logic

#### Task 2.3.3: GraphQL Error Standardization  
- **Target**: graph/ resolvers
- **Action**: Use standardized errors in resolver functions
- **Goal**: Consistent GraphQL error responses

#### Task 2.3.4: ActivityPub Error Handling
- **Target**: pkg/activitypub/, pkg/federation/
- **Action**: Standardize federation protocol error handling
- **Goal**: Proper ActivityPub error semantics

### 🎯 SUCCESS CRITERIA

**Completion Metrics**:
- [ ] **fmt.Errorf() reduction**: <50 total occurrences in service layers
- [ ] **HTTP responses**: 100% use common.Respond* functions in API layer
- [ ] **Error constants**: Each domain has standardized error.go file
- [ ] **Compilation**: All code builds successfully
- [ ] **Consistency**: Predictable error handling across all layers

### 🚨 CRITICAL REQUIREMENTS

1. **Preserve Error Semantics**: Don't change error meaning or HTTP status codes
2. **Maintain Context**: Use fmt.Errorf("context: %w", err) when wrapping errors  
3. **Test Compatibility**: Ensure existing tests still pass
4. **API Compatibility**: Don't break client error handling expectations

### 📊 TRACKING PROGRESS

**Before Starting**:
```bash
# Count remaining fmt.Errorf usage in service layers
rg "fmt\.Errorf" cmd/ graph/ pkg/services/ pkg/auth/ pkg/federation/ pkg/activitypub/ --type go -c | wc -l

# Count inline HTTP error responses
rg "ctx\.Status\(4[0-9][0-9]\)\.JSON" cmd/api/lift/ --type go | wc -l
```

**After Each Batch**:
- Rerun counts to track reduction
- Verify compilation with `go build ./...`
- Test critical paths still work

### 🎯 EXPECTED OUTCOME
**Phase 2.3 Complete** when:
- Service layers use standardized error constants
- API layer uses centralized error response functions  
- Error handling is consistent across entire codebase
- <50 fmt.Errorf() calls remain (only for legitimate wrapping)

This continuation phase will achieve comprehensive error handling standardization across the entire Lesser application architecture.