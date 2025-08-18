# PHASE 4: AUTHENTICATION AND ERROR HANDLING CONSOLIDATION PLAN

## Executive Summary

Following our proven Phase 1 methodology that achieved 99.0% validation consolidation, this plan systematically consolidates authentication and error response patterns across the codebase.

### Key Findings
- **ExtractBearerToken usage**: 80+ occurrences across 34 files
- **Error response patterns**: 438 ctx.Status(4XX).JSON() occurrences across 37 files
- **JWT validation patterns**: 40+ ValidateAccessToken calls
- **Test mode patterns**: 50+ X-Test-Username header checks
- **Authorization header extraction**: Multiple fallback patterns across files

### Consolidation Framework Created
- `/pkg/common/auth_helpers.go` - 15 authentication helper functions
- `/pkg/common/error_responses.go` - 45+ standardized error response functions

## Migration Strategy

### Batch Migration Plan (20 Systematic Batches)

#### **BATCH 1: Core Authentication Functions (HIGH IMPACT)**
**Target**: Extract and consolidate main auth flow in API handlers
**Files**: 
- `/cmd/api/lift/quotes.go`
- `/cmd/api/lift/polls.go`
- `/cmd/api/lift/follow_requests.go`

**Pattern**: Replace 15-20 line auth blocks with single function call
```go
// BEFORE (20 lines)
testUsername := ctx.Header("X-Test-Username")
if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
    testUsername = ctx.Request.Request.Headers["X-Test-Username"]
}
var username string
if testUsername != "" {
    username = testUsername
} else {
    authHeader := ctx.Header("Authorization")
    if authHeader == "" {
        authHeader = ctx.Header("authorization")
    }
    if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
        authHeader = ctx.Request.Request.Headers["Authorization"]
        if authHeader == "" {
            authHeader = ctx.Request.Request.Headers["authorization"]
        }
    }
    token, err := auth.ExtractBearerToken(authHeader)
    if err != nil {
        return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
    }
    oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
    claims, err := oauthSvc.ValidateAccessToken(token)
    if err != nil {
        return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
    }
    if !claims.HasScope(auth.ScopeWrite) {
        return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
    }
    username = claims.Username
}

// AFTER (3 lines)
authResult := common.ExtractAndValidateAuth(ctx, auth.ScopeWrite, h.cfg.JWTSecret, h.repos, h.logger)
if authResult.Error != nil {
    return common.RespondAuthError(ctx, authResult)
}
username := authResult.Context.Username
```

**Estimated Reduction**: 80% line reduction (300+ lines → 60 lines)

#### **BATCH 2: Unauthorized Error Responses (HIGH FREQUENCY)**
**Target**: Consolidate 401 error responses
**Files**:
- `/cmd/api/lift/trends.go`
- `/cmd/api/lift/bookmarks.go`
- `/cmd/api/lift/status_info.go`
- `/cmd/api/lift/webfinger.go`

**Pattern**: Replace map[string]string{"error": "unauthorized"} variants
```go
// BEFORE
return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
return ctx.Status(401).JSON(map[string]string{"error": "invalid token"})

// AFTER
return common.RespondUnauthorized(ctx)
return common.RespondInvalidToken(ctx)
```

**Estimated Reduction**: 95% consistency improvement (15+ variants → 3 standard functions)

#### **BATCH 3: Forbidden/Insufficient Scope Errors (HIGH FREQUENCY)**
**Target**: Consolidate 403 error responses
**Files**:
- `/cmd/api/lift/status_interactions.go`
- `/cmd/api/lift/announcements.go`
- `/cmd/api/lift/conversations.go`
- `/cmd/api/lift/filters.go`

**Pattern**: Standardize scope validation errors
```go
// BEFORE
return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
return ctx.Status(403).JSON(map[string]string{"error": "not authorized"})

// AFTER
return common.RespondInsufficientScope(ctx, auth.ScopeWrite)
return common.RespondNotAuthorized(ctx, "status")
```

#### **BATCH 4: Bad Request/Validation Errors (HIGH FREQUENCY)**
**Target**: Consolidate 400 error responses
**Files**:
- `/cmd/api/lift/relationships.go`
- `/cmd/api/lift/timelines.go`
- `/cmd/api/lift/domain_blocks.go`
- `/cmd/api/lift/scheduled_statuses.go`

**Pattern**: Standardize validation error responses
```go
// BEFORE
return ctx.Status(400).JSON(map[string]string{"error": err.Error()})
return ctx.Status(400).JSON(map[string]string{"error": "missing account id"})

// AFTER
return common.RespondValidationError(ctx, err)
return common.RespondMissingAccountID(ctx)
```

#### **BATCH 5: Not Found Errors (HIGH FREQUENCY)**
**Target**: Consolidate 404 error responses
**Files**:
- `/cmd/api/lift/oembed.go`
- `/cmd/api/lift/preferences.go`
- `/cmd/api/lift/mutes.go`
- `/cmd/api/lift/notes.go`

**Pattern**: Standardize not found responses
```go
// BEFORE
return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
return ctx.Status(404).JSON(map[string]string{"error": "account not found"})

// AFTER
return common.RespondStatusNotFound(ctx)
return common.RespondAccountNotFound(ctx)
```

#### **BATCH 6: Optional Authentication Patterns**
**Target**: Endpoints with optional authentication
**Files**:
- `/cmd/api/lift/timelines.go` (public timeline)
- `/cmd/api/lift/search.go`
- `/cmd/api/lift/tags.go`

**Pattern**: Replace complex optional auth logic
```go
// BEFORE (10-15 lines)
var username string
if token, err := auth.ExtractBearerToken(authHeader); err == nil {
    claims, err := oauthSvc.ValidateAccessToken(token)
    if err == nil {
        username = claims.Username
    }
}

// AFTER (2 lines)
authResult := common.ExtractOptionalAuth(ctx, h.cfg.JWTSecret, h.repos, h.logger)
username := authResult.Context.Username
```

#### **BATCH 7: Multiple Scope Validation**
**Target**: Endpoints accepting multiple scopes
**Files**:
- `/cmd/api/lift/follow_requests.go`
- `/cmd/api/lift/domain_blocks.go`
- `/cmd/api/lift/mutes.go`

**Pattern**: Consolidate complex scope checking
```go
// BEFORE
if !claims.HasScope("read:follows") && !claims.HasScope(auth.ScopeRead) {
    return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
}

// AFTER
authResult := common.ExtractAndValidateAuthWithMultipleScopes(ctx, 
    []string{"read:follows", auth.ScopeRead}, h.cfg.JWTSecret, h.repos, h.logger)
```

#### **BATCH 8: Server Error Standardization**
**Target**: Consolidate 500 error responses
**Files**:
- `/cmd/api/lift/apps.go`
- `/cmd/api/lift/statuses_unified_boost.go`
- `/cmd/api/lift/instance.go`
- `/cmd/api/lift/custom_emojis.go`

**Pattern**: Standardize internal server errors
```go
// BEFORE
return ctx.Status(500).JSON(map[string]string{"error": "failed to create status"})
return ctx.Status(500).JSON(map[string]string{"error": "database error"})

// AFTER
return common.RespondFailedToCreate(ctx, "status")
return common.RespondDatabaseError(ctx)
```

#### **BATCH 9: Unprocessable Entity (422) Errors**
**Target**: Consolidate 422 validation errors
**Files**:
- `/cmd/api/lift/quotes.go`
- `/cmd/api/lift/statuses.go`
- Various content validation endpoints

**Pattern**: Standardize content validation errors
```go
// BEFORE
return ctx.Status(422).JSON(map[string]string{"error": "status text too long"})

// AFTER
return common.RespondStatusTooLong(ctx)
```

#### **BATCH 10: Test Mode Header Extraction**
**Target**: Consolidate X-Test-Username handling
**Files**:
- `/cmd/api/lift/markers.go`
- `/cmd/api/lift/search.go`
- `/cmd/api/lift/translation.go`
- `/cmd/api/lift/reputation.go`

**Pattern**: Replace duplicate test header logic
```go
// BEFORE (5-8 lines)
testUsername := ctx.Header("X-Test-Username")
if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
    testUsername = ctx.Request.Request.Headers["X-Test-Username"]
}

// AFTER (1 line)
testUsername := common.GetTestUsername(ctx)
```

#### **BATCH 11: Authorization Header Extraction**
**Target**: Consolidate auth header retrieval patterns
**Files**:
- `/cmd/api/lift/handler.go`
- `/cmd/api/lift/accounts.go`
- `/pkg/middleware/search_privacy.go`

**Pattern**: Replace complex header extraction
```go
// BEFORE (8-10 lines)
authHeader := ctx.Header("Authorization")
if authHeader == "" {
    authHeader = ctx.Header("authorization")
}
if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
    authHeader = ctx.Request.Request.Headers["Authorization"]
    if authHeader == "" {
        authHeader = ctx.Request.Request.Headers["authorization"]
    }
}

// AFTER (1 line)
authHeader := common.ExtractAuthHeader(ctx)
```

#### **BATCH 12: Composite Auth Validation**
**Target**: Endpoints with complex auth requirements
**Files**:
- Admin endpoints in `/cmd/api/lift/`
- Moderation endpoints
- Federation endpoints

**Pattern**: Replace multi-step auth validation
```go
// BEFORE (15-20 lines of validation)
// Complex auth + admin scope + additional checks

// AFTER (5 lines)
authResult := common.ExtractAndValidateAuth(ctx, auth.AdminScope, cfg.JWTSecret, repos, logger)
if authResult.Error != nil {
    return common.RespondAuthError(ctx, authResult)
}
```

#### **BATCH 13: GraphQL Authentication**
**Target**: GraphQL resolvers with auth
**Files**:
- `/graph/subscription_handlers.go`
- GraphQL resolver files

**Pattern**: Standardize GraphQL auth patterns

#### **BATCH 14: Middleware Authentication**
**Target**: Authentication middleware
**Files**:
- `/pkg/auth/middleware.go`
- `/pkg/auth/middleware_lift.go`
- Custom middleware files

**Pattern**: Consolidate middleware auth logic

#### **BATCH 15: Lambda Function Authentication**
**Target**: Lambda entry points with auth
**Files**:
- `/cmd/*/main.go` files with auth requirements
- `/cmd/outbox/main.go`

**Pattern**: Standardize Lambda auth patterns

#### **BATCH 16: Testing Mock Consolidation**
**Target**: Test files with auth mocking
**Files**:
- `/pkg/lift/testing/mocks.go`
- Various `*_test.go` files

**Pattern**: Consolidate test auth patterns

#### **BATCH 17: Rate Limiting Integration**
**Target**: Rate limiting with auth
**Files**:
- `/pkg/ratelimit/middleware.go`
- Rate-limited endpoints

**Pattern**: Integrate auth with rate limiting

#### **BATCH 18: WebSocket Authentication**
**Target**: WebSocket-specific auth patterns
**Files**:
- `/pkg/streaming/handlers/`
- WebSocket connection handlers

**Pattern**: Standardize streaming auth

#### **BATCH 19: Federation Authentication**
**Target**: ActivityPub federation auth
**Files**:
- Federation-related handlers
- ActivityPub signature validation

**Pattern**: Consolidate federation auth patterns

#### **BATCH 20: Final Error Response Standardization**
**Target**: Remaining error response patterns
**Files**:
- Any remaining files with manual error responses
- Legacy error patterns

**Pattern**: Complete error response migration

## Implementation Steps

### Phase 1: Framework Verification (COMPLETED)
- ✅ Created `/pkg/common/auth_helpers.go` with 15 auth functions
- ✅ Created `/pkg/common/error_responses.go` with 45+ error response functions
- ✅ Framework compilation verified

### Phase 2: Batch Implementation (20 batches × 5-7 days each = 100-140 days)

For each batch:
1. **Pre-Migration Analysis** (1 day)
   - Identify exact files and line numbers
   - Document current patterns
   - Test compilation baseline

2. **Migration Implementation** (2-3 days)
   - Replace patterns with consolidated functions
   - Update imports
   - Maintain exact behavior

3. **Verification & Testing** (1-2 days)
   - Compile all affected files
   - Run relevant tests
   - Verify no behavior changes

4. **Documentation & Review** (1 day)
   - Update batch status
   - Document any edge cases
   - Prepare next batch

### Phase 3: Validation & Measurement

#### Success Metrics (Target: 95%+ Consolidation)
- **Authentication Pattern Reduction**: 80+ ExtractBearerToken calls → 10-15 helper calls
- **Error Response Consolidation**: 438 manual error responses → 50-100 standardized calls
- **Line Count Reduction**: Estimated 2000+ lines → 400-600 lines
- **Pattern Consistency**: 95%+ standardization across endpoints

#### Quality Gates
- Zero compilation errors after each batch
- All existing tests pass
- No behavior changes in API responses
- Performance impact < 5%

## Migration Priority Order

### HIGH PRIORITY (Batches 1-8)
- Core authentication flows (highest impact)
- Most frequent error patterns
- Critical API endpoints

### MEDIUM PRIORITY (Batches 9-16)
- Specialized authentication patterns
- Test mode consolidation
- Middleware integration

### LOW PRIORITY (Batches 17-20)
- Edge cases and legacy patterns
- Final standardization
- Documentation cleanup

## Expected Benefits

### Code Quality Improvements
- **Maintainability**: Centralized auth logic in `/pkg/common/`
- **Consistency**: Standardized error responses across API
- **Testability**: Consolidated functions easier to unit test
- **Security**: Reduced chance of auth bypass bugs

### Developer Experience
- **Faster Development**: Pre-built auth and error functions
- **Reduced Bugs**: Consistent error handling patterns
- **Easier Debugging**: Standardized error messages
- **Better Documentation**: Clear auth patterns

### Performance Benefits
- **Reduced Code Duplication**: 70-80% reduction in auth code
- **Optimized Error Handling**: Faster error response generation
- **Better Caching**: Centralized auth validation logic

## Risk Mitigation

### Technical Risks
- **Behavior Changes**: Comprehensive testing after each batch
- **Performance Impact**: Benchmark critical paths
- **Integration Issues**: Gradual rollout with rollback capability

### Process Risks
- **Large Scale Changes**: Small batch approach (5-10 files per batch)
- **Team Coordination**: Clear batch documentation and review process
- **Timeline Pressure**: Flexible batch scheduling based on priorities

## Rollback Strategy

Each batch includes:
1. **Git Branch**: Separate branch for each batch
2. **Automated Testing**: Full test suite run before merge
3. **Rollback Plan**: Document exact files changed and reversion steps
4. **Monitoring**: Track error rates and performance after deployment

## Success Criteria

### Quantitative Goals
- ✅ **95%+ Consolidation Rate**: Match Phase 1 success rate
- ✅ **Zero Behavior Changes**: All existing API tests pass
- ✅ **70%+ Line Reduction**: Significant code reduction
- ✅ **100% Compilation Success**: No broken builds

### Qualitative Goals
- Improved code maintainability and readability
- Standardized authentication patterns across codebase
- Consistent error response formats
- Better developer experience for future development

This comprehensive plan follows the exact methodology that achieved 99.0% success in Phase 1, ensuring systematic and safe consolidation of authentication and error handling patterns across the entire codebase.