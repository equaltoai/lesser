# Phase 2.3 Error Handling Standardization - Final Completion

## CURRENT OUTSTANDING PROGRESS ✅

**Architectural Foundation Complete:**
- ✅ **Repository Layer**: 100% standardized with storage.Err* constants
- ✅ **HTTP Response Framework**: Comprehensive 467-line error_responses.go with 40+ functions
- ✅ **Adoption Growth**: 34 → 42 files using centralized patterns (23% increase)

## REMAINING WORK FOR 100% COMPLETION

### 🎯 Critical Targets

**123 files** still using fmt.Errorf() in service layers
**12 inline HTTP responses** in API layer using ctx.Status().JSON()
**27 files** using errors.New in core domains

### 📊 High-Priority Files Analysis

**Top fmt.Errorf() Offenders:**
1. `/Users/aronprice/lesser/pkg/services/notes/service.go` - **74 occurrences**
2. `/Users/aronprice/lesser/pkg/services/relationships/service.go` - **69 occurrences**  
3. `/Users/aronprice/lesser/pkg/services/lists/service.go` - **31 occurrences**
4. `/Users/aronprice/lesser/pkg/services/scheduled/service.go` - **25 occurrences**
5. `/Users/aronprice/lesser/pkg/services/quotes/quote_service.go` - **17 occurrences**

**API Layer Inline Responses:**
- `/Users/aronprice/lesser/cmd/api/lift/accounts.go` - Password validation (422)
- `/Users/aronprice/lesser/cmd/api/lift/notes.go` - Rate limiting (429)
- `/Users/aronprice/lesser/cmd/api/lift/oembed.go` - Multiple 400/403/404 responses

## IMPLEMENTATION STRATEGY

### Phase 1: Service Layer Error Constants (Priority 1)

**Create domain-specific error files:**

```go
// pkg/services/errors.go
package services

import "errors"

var (
    // Business Logic Errors
    ErrServiceUnavailable    = errors.New("service temporarily unavailable")
    ErrInvalidOperation      = errors.New("invalid operation")
    ErrResourceExhausted     = errors.New("resource limit exceeded")
    ErrOperationNotAllowed   = errors.New("operation not allowed")
    ErrConcurrentModification = errors.New("concurrent modification detected")
    
    // Content/Media Errors
    ErrContentTooLarge       = errors.New("content exceeds size limit")
    ErrUnsupportedMediaType  = errors.New("unsupported media type")
    ErrContentModeration     = errors.New("content rejected by moderation")
    
    // Timeline/Feed Errors
    ErrTimelineEmpty         = errors.New("timeline is empty")
    ErrInvalidTimelineRange  = errors.New("invalid timeline range")
    
    // Relationship Errors
    ErrCannotFollowSelf      = errors.New("cannot follow yourself")
    ErrAlreadyFollowing      = errors.New("already following user")
    ErrNotFollowing          = errors.New("not following user")
    ErrBlocked               = errors.New("user is blocked")
    
    // Note/Status Errors
    ErrEmptyContent          = errors.New("content cannot be empty")
    ErrInvalidVisibility     = errors.New("invalid visibility setting")
    ErrEditWindowExpired     = errors.New("edit window has expired")
    ErrCannotEditOthers      = errors.New("cannot edit other users' content")
)
```

### Phase 2: High-Impact Service Migration (Priority 1)

**Target Order:**
1. **pkg/services/notes/service.go** (74 occurrences)
2. **pkg/services/relationships/service.go** (69 occurrences)
3. **pkg/services/lists/service.go** (31 occurrences)

**Migration Pattern:**
```go
// ❌ BEFORE
return fmt.Errorf("cannot follow yourself")

// ✅ AFTER  
return services.ErrCannotFollowSelf

// ❌ BEFORE (with context)
return fmt.Errorf("user %s not found", userID)

// ✅ AFTER (preserve context)
return fmt.Errorf("user %s: %w", userID, storage.ErrNotFound)
```

### Phase 3: API Layer HTTP Response Migration (Priority 2)

**Replace remaining inline responses:**

```go
// ❌ BEFORE (/cmd/api/lift/accounts.go:422)
return ctx.Status(422).JSON(map[string]string{
    "error": fmt.Sprintf("Password is too weak (%s). Suggestions: %s", ...),
})

// ✅ AFTER
return common.RespondUnprocessableEntity(ctx, 
    fmt.Sprintf("Password is too weak (%s). Suggestions: %s", ...))

// ❌ BEFORE (/cmd/api/lift/notes.go:429)
return ctx.Status(429).JSON(map[string]any{
    "error": "Rate limit exceeded",
})

// ✅ AFTER
return common.RespondRateLimited(ctx)

// ❌ BEFORE (/cmd/api/lift/oembed.go:404)
return ctx.Status(404).JSON(map[string]string{
    "error": "status not found",
})

// ✅ AFTER
return common.RespondStatusNotFound(ctx)
```

### Phase 4: Domain-Specific Error Files (Priority 3)

**Create specialized error constants:**

```go
// pkg/auth/errors.go
var (
    ErrInvalidCredentials     = errors.New("invalid credentials")
    ErrAccountLocked          = errors.New("account locked")
    ErrPasswordExpired        = errors.New("password expired")
    ErrInvalidScope           = errors.New("invalid OAuth scope")
    ErrTokenRevoked           = errors.New("token has been revoked")
)

// pkg/federation/errors.go
var (
    ErrInvalidSignature       = errors.New("invalid ActivityPub signature")
    ErrInstanceBlocked        = errors.New("instance is blocked")
    ErrDeliveryFailed         = errors.New("delivery failed")
    ErrUnsupportedActivityType = errors.New("unsupported activity type")
    ErrMalformedActivity      = errors.New("malformed ActivityPub object")
)
```

## IMPLEMENTATION TASKS

### Task 2.3.3: Service Layer Constants Creation
**Action**: Create pkg/services/errors.go with comprehensive error constants
**Target**: Foundation for service layer standardization
**Completion**: All service error types defined

### Task 2.3.4: High-Impact Service Files Migration  
**Action**: Migrate top 5 service files (243 total fmt.Errorf occurrences)
**Target**: pkg/services/notes/, relationships/, lists/, scheduled/, quotes/
**Completion**: 80% reduction in service layer fmt.Errorf usage

### Task 2.3.5: API Layer HTTP Response Cleanup
**Action**: Replace all 12 inline ctx.Status().JSON() with common.Respond* functions
**Target**: cmd/api/lift/ files (accounts.go, notes.go, oembed.go)
**Completion**: Zero inline HTTP error responses in API layer

### Task 2.3.6: Domain Error Constants
**Action**: Create pkg/auth/errors.go and pkg/federation/errors.go
**Target**: Domain-specific error standardization
**Completion**: All core domains have standardized errors

### Task 2.3.7: Remaining Service Files Cleanup
**Action**: Migrate remaining 90+ service files with fmt.Errorf usage
**Target**: Complete service layer standardization
**Completion**: <10 fmt.Errorf calls remaining across entire codebase

## SUCCESS CRITERIA FOR 100% COMPLETION

### Quantitative Targets:
- [ ] **fmt.Errorf() usage**: <10 total occurrences in service layers (down from 123)
- [ ] **Inline HTTP responses**: 0 ctx.Status().JSON() in API layer (down from 12)
- [ ] **Domain error files**: 5+ domains with standardized error.go files
- [ ] **Centralized adoption**: 80+ files using common.Respond* patterns (up from 42)

### Qualitative Targets:
- [ ] **Predictable errors**: All service layers return typed errors
- [ ] **Consistent HTTP responses**: All API endpoints use standardized functions
- [ ] **Maintainable architecture**: Error handling follows single responsibility
- [ ] **Type safety**: Error checking uses errors.Is() instead of string matching

## VERIFICATION COMMANDS

**Track Progress:**
```bash
# Service layer fmt.Errorf count
rg "fmt\.Errorf" pkg/services/ pkg/auth/ pkg/federation/ --type go | wc -l

# API layer inline responses
rg "ctx\.Status\(4[0-9][0-9]\)\.JSON" cmd/api/lift/ --type go | wc -l

# Centralized error adoption
rg "common\.Respond" --type go --files-with-matches | wc -l

# Domain error files exist
ls pkg/services/errors.go pkg/auth/errors.go pkg/federation/errors.go
```

**Final Verification:**
```bash
# Should build without errors
go build ./...

# Error handling should be consistent
rg "fmt\.Errorf.*not found" --type go | wc -l  # Should be 0
rg "storage\.ErrNotFound" --type go | wc -l     # Should be >100
```

## EXPECTED OUTCOME

**100% Error Handling Standardization:**
- Predictable error semantics across all layers
- Consistent HTTP API responses for all clients
- Type-safe error handling with errors.Is()/errors.As()
- Maintainable error architecture for future development
- Foundation for enhanced error monitoring and alerting

This final phase completes the comprehensive error handling standardization initiative across the entire Lesser application stack.