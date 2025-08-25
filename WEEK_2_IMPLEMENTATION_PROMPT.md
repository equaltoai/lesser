# Week 2 Implementation Prompt - Code Quality & Optimization

## Context
You are continuing work on the Lesser codebase after successfully completing Week 1. All critical issues have been resolved: storage migration complete, Sonic removed, security vulnerabilities fixed, TODOs eliminated, and environment configuration cleaned up. 

Week 2 focuses on code quality improvements and optimizations that will make the codebase more maintainable and efficient.

## Completed Status from Week 1
- ✅ Storage migration (h.store → h.repos) complete
- ✅ Sonic removed, using standard library JSON
- ✅ Security vulnerabilities addressed (4096-bit RSA, bcrypt, proper JWT)
- ✅ All 18 TODOs implemented
- ✅ Environment variables reduced by 57% with proper separation

## Your Mission for Week 2

### Day 1-2: Code Duplication Reduction

**Objective:** Extract common patterns identified in audits to reduce duplication by 30%+

**Target Areas:**
1. **Pagination Logic** (30+ duplications across handlers)
2. **Authentication Extraction** (multiple copies)
3. **Error Response Formatting** (50+ locations)
4. **Rate Limit Checking** (redundant implementations)

**Instructions:**

#### Step 1: Create Common Pagination Helper
```bash
# Find all pagination patterns
grep -rn "page\|limit\|offset" --include="*.go" cmd/api/lift/ | grep -v test
```

1. **Create `pkg/common/pagination.go`:**
   ```go
   package common
   
   import (
       "strconv"
       "net/http"
   )
   
   type PaginationParams struct {
       Limit  int
       Offset int
       Page   int
   }
   
   func GetPaginationParams(r *http.Request) PaginationParams {
       // Extract and validate pagination parameters
       // Default: limit=20, offset=0
       // Convert page to offset if provided
       // Return standardized struct
   }
   ```

2. **Update all handlers to use common helper:**
   - Replace inline pagination logic
   - Use consistent parameter names
   - Apply same validation rules

#### Step 2: Centralize Authentication Extraction
```bash
# Find all auth extraction patterns
grep -rn "GetAccountFromContext\|GetUsernameFromContext" --include="*.go" cmd/api/lift/
```

1. **Create unified auth middleware in `pkg/auth/extraction.go`:**
   - Single source of truth for auth extraction
   - Consistent error handling
   - Proper context usage

2. **Update all handlers to use centralized extraction**

#### Step 3: Standardize Error Responses
```bash
# Find error response patterns
grep -rn "ErrorResponse\|sendError\|WriteError" --include="*.go" cmd/api/lift/
```

1. **Create `pkg/common/responses.go`:**
   ```go
   package common
   
   func SendError(w http.ResponseWriter, code int, message string) {
       // Standardized error response format
       // Consistent with Mastodon API
       // Proper content-type headers
   }
   
   func SendJSON(w http.ResponseWriter, code int, data interface{}) {
       // Standardized JSON response
       // Proper headers
       // Consistent formatting
   }
   ```

2. **Replace all inline error formatting**

**Success Criteria:**
- Duplication reduced by 30%+ (measure with `golangci-lint run --enable=dupl`)
- All handlers use common helpers
- Consistent response formats throughout

### Day 2-3: Performance Optimization

**Objective:** Implement performance improvements identified in audits

#### Priority 1: Implement DataLoader Pattern for N+1 Prevention

1. **Identify N+1 Query Patterns:**
   ```bash
   grep -rn "for.*range.*Get\|for.*range.*Load" --include="*.go" pkg/services/
   ```

2. **Create DataLoader for common entities:**
   - User loader
   - Status loader
   - Account loader
   
   **Location:** `pkg/dataloader/loaders.go`

3. **Integration points:**
   - Timeline generation
   - Notification fetching
   - Federation activities

#### Priority 2: Optimize DynamoDB Query Patterns

1. **Add batch operations where appropriate:**
   ```bash
   # Find multiple single-item operations that could be batched
   grep -rn "GetItem\|PutItem" --include="*.go" pkg/storage/repositories/ | head -20
   ```

2. **Implement batch helpers in repositories:**
   - BatchGet for multiple items
   - BatchWrite for bulk inserts
   - Transaction support for atomic operations

#### Priority 3: Add Caching Layer for Federation Keys

1. **Create `pkg/cache/federation_cache.go`:**
   - TTL-based caching for public keys
   - Memory-efficient storage
   - Automatic invalidation

2. **Integration points:**
   - HTTP signature verification
   - Actor fetching
   - Instance metadata

**Success Criteria:**
- DataLoader implemented for main entities
- Batch operations available in repositories
- Federation cache operational

### Day 3-4: Linting and Code Style Enforcement

**Objective:** Address all linting issues and enforce consistent style

#### Step 1: Run Full Linting Suite
```bash
golangci-lint run ./... > linting_report.txt
```

#### Step 2: Fix Issues by Category

1. **goconst violations** (string constants):
   ```bash
   golangci-lint run --enable-only=goconst --fix ./...
   ```
   
   Create constants for repeated strings:
   - Status states ("suspended", "active", etc.)
   - Operation types ("create", "update", "delete")
   - Common strings appearing 3+ times

2. **revive issues** (style violations):
   ```bash
   golangci-lint run --enable-only=revive ./...
   ```
   
   Fix:
   - Unused parameters (rename to `_`)
   - Package naming issues
   - Missing package comments

3. **ineffassign** (ineffective assignments):
   ```bash
   golangci-lint run --enable-only=ineffassign ./...
   ```

**Success Criteria:**
- Zero linting errors in core packages
- Consistent code style throughout
- All string constants extracted

### Day 4-5: Documentation and API Consistency

**Objective:** Improve documentation and ensure API consistency

#### Step 1: Add Package Documentation

1. **Add package comments to all packages missing them:**
   ```go
   // Package batch provides utilities for batch operations in DynamoDB.
   package batch
   ```

2. **Priority packages:**
   - pkg/storage/dynamorm/batch
   - pkg/storage/interfaces
   - pkg/federation/types

#### Step 2: API Response Consistency Audit

1. **Verify all API responses match Mastodon format:**
   ```bash
   # Check response structures
   grep -rn "ctx.JSON\|json.Marshal" --include="*.go" cmd/api/lift/ | head -20
   ```

2. **Ensure consistent:**
   - Field naming (snake_case for JSON)
   - Null vs omitted fields
   - Error response format
   - Pagination headers

#### Step 3: Generate API Documentation

1. **Create `docs/API_REFERENCE.md`:**
   - Document all endpoints
   - Include request/response examples
   - Note any deviations from Mastodon API

**Success Criteria:**
- All packages have documentation
- API responses consistently formatted
- Comprehensive API reference created

## Verification Commands

Run these after each day to verify progress:

```bash
# Day 1-2: Check duplication reduction
golangci-lint run --enable=dupl ./... | grep "duplicate of" | wc -l

# Day 2-3: Check for N+1 patterns
grep -rn "for.*range.*Get" --include="*.go" pkg/services/ | wc -l

# Day 3-4: Check linting status
golangci-lint run ./... 2>&1 | grep -E "^[^:]+:[0-9]+:[0-9]+:" | wc -l

# Day 4-5: Check documentation
grep -L "^// Package" pkg/*/[a-z]*.go | wc -l
```

## Important Notes

1. **Maintain Working State**: After each change, ensure `go build ./...` succeeds
2. **Test Critical Paths**: Run `make test` after major changes
3. **Commit Frequently**: Make atomic commits for each improvement
4. **Performance Over Perfection**: Focus on measurable improvements
5. **Document Decisions**: Add comments explaining why patterns were chosen

## Constraints

- **DO NOT** introduce new dependencies without strong justification
- **DO NOT** break existing API contracts
- **DO NOT** over-engineer solutions - simple is better
- **MAINTAIN** the current architecture patterns
- **PRESERVE** all security improvements from Week 1

## Success Metrics for Week 2

- [ ] Code duplication reduced by 30%+
- [ ] Common patterns extracted and reused
- [ ] N+1 queries eliminated in critical paths
- [ ] Linting errors reduced to near-zero
- [ ] All packages properly documented
- [ ] API responses consistently formatted
- [ ] Performance improvements measurable

## Resources

- Linting config: `.golangci.yml`
- Existing patterns: `pkg/common/`, `pkg/auth/`
- Repository patterns: `pkg/storage/repositories/`
- API examples: `cmd/api/lift/`

Begin with code duplication reduction as it provides the foundation for other improvements. Focus on extracting patterns that appear most frequently first.

Good luck with Week 2!