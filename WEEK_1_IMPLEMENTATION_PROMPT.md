# Week 1 Implementation Prompt - Critical Foundation

## Context
You are working on the Lesser codebase, a serverless ActivityPub implementation. Four AI models have audited the code and identified critical issues that must be fixed before release. This is Week 1 of a 4-week plan focusing on the most critical foundation issues.

## Your Mission for Week 1

### Day 1-2: Complete Storage Migration (HIGHEST PRIORITY)

**Objective:** Migrate all `h.store` references to `h.repos` pattern in the API handlers.

**Background:** 
- Phase 5.6 migration is incomplete with ~500+ references to the old pattern
- A migration tool exists at `tools/migrate_lift_storage_to_repos.go`
- This affects ~50 files in `cmd/api/lift/`

**Step-by-step instructions:**

1. **First, run the migration tool:**
   ```bash
   go run tools/migrate_lift_storage_to_repos.go
   ```

2. **CRITICAL: Verify ALL files are migrated, not just examples:**
   ```bash
   # This MUST return 0 - if not, migration is incomplete
   grep -n "h.store" cmd/api/lift/*.go | wc -l
   
   # If above returns non-zero, list ALL files that still need migration:
   grep -l "h.store" cmd/api/lift/*.go
   ```
   
   **DO NOT STOP** after checking just admin.go, statuses.go, accounts.go, and notes.go.
   **EVERY FILE** in cmd/api/lift/ must be migrated - approximately 50 files total.

3. **Run comprehensive verification:**
   ```bash
   # Should return 0 - no more h.store references
   grep -n "h.store" cmd/api/lift/*.go | wc -l
   
   # Should return 0 - no originalStorage delegation
   grep -n "originalStorage\." pkg/storage/adapter.go | wc -l
   ```

4. **Test compilation:**
   ```bash
   go build ./cmd/api/...
   ```

5. **If migration tool doesn't handle everything:**
   ```bash
   # Get the COMPLETE list of files still needing migration
   grep -l "h.store" cmd/api/lift/*.go > files_to_migrate.txt
   
   # Count how many files remain
   wc -l files_to_migrate.txt
   ```
   
   **You MUST migrate ALL files in the list**, not just a sample.
   If there are 30 files, migrate all 30. If there are 50, migrate all 50.

**Success Criteria:**
- **MANDATORY**: `grep -n "h.store" cmd/api/lift/*.go | wc -l` returns exactly 0
- **MANDATORY**: ALL ~50 files in cmd/api/lift/ are migrated
- All files compile successfully
- No references to `originalStorage` in the adapter

### Day 3: Replace Sonic with Native JSON (When Go 1.25 Available)

**Objective:** Remove the Sonic dependency and use native JSON handling.

**Current State:** 
- Only `pkg/common/json.go` uses Sonic
- Go 1.24 is current version (waiting for 1.25)

**If Go 1.25 is available:**

1. **Update Go version:**
   ```bash
   # Update go.mod
   go mod edit -go=1.25
   go mod tidy
   ```

2. **Rewrite pkg/common/json.go:**
   ```go
   package common

   import (
       "encoding/json/v2"
       "io"
   )

   // Marshal serializes v to JSON with ActivityPub optimizations
   func Marshal(v any) ([]byte, error) {
       // Use json/v2 with appropriate options
       return json.Marshal(v)
   }

   // Unmarshal deserializes JSON data
   func Unmarshal(data []byte, v any) error {
       return json.Unmarshal(data, v)
   }

   // Keep the same API surface for compatibility
   ```

3. **Remove Sonic from dependencies:**
   ```bash
   go mod edit -droprequire github.com/bytedance/sonic
   go mod tidy
   ```

**If Go 1.25 is NOT available:**

1. **Create a temporary standard JSON wrapper:**
   - Replace Sonic with encoding/json temporarily
   - Maintain the same API in pkg/common/json.go
   - Document that this will be upgraded to json/v2 when available

2. **Remove Sonic dependency:**
   ```bash
   go mod edit -droprequire github.com/bytedance/sonic
   go mod tidy
   ```

**Success Criteria:**
- No Sonic imports in the codebase
- All JSON operations work correctly
- Performance is acceptable (even if temporarily slower)

### Day 4-5: Fix Critical Security Vulnerabilities

**Objective:** Address the highest severity security issues from gosec findings.

#### Priority 1: Timing Attack Prevention

**File:** `pkg/auth/service.go`

1. **Find password verification code:**
   ```bash
   grep -n "VerifyPassword" pkg/auth/service.go
   ```

2. **Implement constant-time comparison:**
   ```go
   import "crypto/subtle"
   
   // Use subtle.ConstantTimeCompare for password hash comparison
   ```

#### Priority 2: Path Traversal Prevention (CWE-22)

1. **Find the 5 instances:**
   ```bash
   # Check gosec results for specific locations
   grep -A 5 "CWE-22" gosec-results.json
   ```

2. **Fix each instance:**
   ```go
   import "path/filepath"
   
   // Before
   path := userInput
   
   // After
   path := filepath.Clean(userInput)
   // Also validate the path is within expected directory
   ```

#### Priority 3: Integer Overflow in Generated Code

**IMPORTANT: DO NOT EDIT graph/generated.go - it's auto-generated!**

1. **Acknowledge the issue but don't fix directly:**
   - The 167 integer overflow instances are in generated code
   - Would require 2+ billion GraphQL deferred fields to exploit (extremely unlikely)
   
2. **Proper approaches (if needed):**
   ```bash
   # Check if newer gqlgen version fixes this
   go get -u github.com/99designs/gqlgen
   
   # Or exclude from linting
   # Already excluded in .golangci.yml under exclude-rules
   ```

3. **Focus on non-generated code instead:**
   ```bash
   # Find integer overflows in actual source code (not generated)
   grep -rn "int32(" --include="*.go" --exclude="*generated*"
   ```

#### Priority 4: RSA Key Size

1. **Find RSA key generation:**
   ```bash
   grep -rn "rsa.GenerateKey" --include="*.go"
   ```

2. **Update from 2048 to 4096 bits:**
   ```go
   // Before
   key, err := rsa.GenerateKey(rand.Reader, 2048)
   
   // After
   keySize := config.GetInt("rsa_key_size", 4096)
   key, err := rsa.GenerateKey(rand.Reader, keySize)
   ```

#### Priority 5: JWT Secret Management

1. **Find JWT secret usage:**
   ```bash
   grep -rn "jwt" --include="*.go" | grep -i secret
   ```

2. **Ensure secrets come from AWS Secrets Manager:**
   - No hardcoded secrets
   - Use the existing secrets manager client
   - Implement rotation support

**Success Criteria:**
- Constant-time password comparison implemented
- All path traversal vulnerabilities fixed
- Critical integer overflows handled
- RSA keys upgraded to 4096 bits
- JWT secrets properly managed

## Verification Checklist

After completing Week 1, verify:

```bash
# 1. Storage migration complete
grep -n "h.store" cmd/api/lift/*.go | wc -l  # Should output: 0

# 2. No Sonic dependency
grep -n "sonic" go.mod  # Should find nothing

# 3. Build succeeds
go build ./...

# 4. Linting passes for security
golangci-lint run --enable=gosec ./...

# 5. Critical paths compile
go build ./cmd/api
go build ./cmd/auth
go build ./cmd/graphql
```

## Important Notes

1. **Commit frequently:** Make atomic commits for each major change
2. **Test after each change:** Even without unit tests, ensure compilation
3. **Document decisions:** If you deviate from the plan, document why
4. **Ask for clarification:** If migration tool fails or patterns are unclear
5. **Priority order:** Storage migration > Security fixes > JSON replacement

## Common Pitfalls to Avoid

1. **NEVER edit generated files** (graph/generated.go, *.pb.go, etc.) - They'll be overwritten
2. **Don't skip verification** - Always run the grep commands to confirm
3. **Don't ignore compilation errors** - Fix them immediately
4. **Don't make assumptions** - Check the actual code patterns first
5. **Check .golangci.yml exclusions** - Many issues may already be suppressed for valid reasons

## Resources

- Migration tool: `tools/migrate_lift_storage_to_repos.go`
- Audit reports: `COMPREHENSIVE_CODE_AUDIT_REPORT_*.md`
- Action plan: `AUDIT_SYNTHESIS_ACTION_PLAN.md`
- Linting config: `.golangci.yml`

## Success Metrics for Week 1

- [ ] Storage migration 100% complete
- [ ] Sonic dependency removed
- [ ] All critical security vulnerabilities patched
- [ ] Codebase compiles without errors
- [ ] No regression in existing functionality

Begin with the storage migration as it's the most critical architectural issue. Good luck!