# Audit Synthesis Action Plan - Lesser Pre-Release

## Executive Summary

Four comprehensive audits were conducted by different AI models on the Lesser codebase. This synthesis consolidates findings into actionable tasks, acknowledging that:
- The application is AI-generated and prefers production testing over unit tests
- golangci-lint is already configured with comprehensive rules
- Sonic JSON should be replaced with Go 1.25's native JSON v2

**Overall Assessment:** B+ (85/100)
- **Strengths:** Solid architecture, strong security foundations, comprehensive error handling, cost optimization, existing linting setup
- **Critical Issues:** Storage migration incomplete, Sonic dependency needs replacement, security vulnerabilities

## Critical Issues - MUST FIX BEFORE RELEASE

### 1. Storage Layer Migration (Phase 5.6) 🔴
**Consensus:** All audits identified this as the highest priority
- **Impact:** Architectural inconsistency affecting ~50 files in `cmd/api/lift/`
- **Scope:** ~500+ references to old `h.store` pattern need migration to `h.repos`
- **Files Most Affected:** 
  - `admin.go` (100+ references)
  - `statuses.go` (50+ references)
  - `accounts.go`, `notes.go`, etc.
- **Action:** Use existing migration tool at `tools/migrate_lift_storage_to_repos.go`
- **Timeline:** 2-3 days with automated tooling
- **Verification:** 
  ```bash
  grep -n "h.store" cmd/api/lift/*.go | wc -l  # Should be 0
  grep -n "originalStorage\." pkg/storage/adapter.go | wc -l  # Should be 0
  ```

### 2. Replace Sonic with Go 1.25 JSON v2 🔴
**Current State:** Only `pkg/common/json.go` uses Sonic
- **Action Steps:**
  1. Update to Go 1.25 when available
  2. Replace Sonic wrapper with native JSON v2
  3. Maintain same API surface in `pkg/common/json.go`
  4. Remove `github.com/bytedance/sonic` from go.mod
- **Timeline:** 1 day once Go 1.25 is available
- **Benefits:** Eliminates dependency issues, native performance improvements

### 3. Security Vulnerabilities (gosec findings) 🔴
**From gosec-results.json analysis:**

#### High Priority Fixes:
1. **Integer Overflow (CWE-190)** - 167 instances
   - Focus on `graph/generated.go` 
   - Use safe math operations where critical

2. **Timing Attack Vulnerability**
   - Implement constant-time comparison in `pkg/auth/service.go`
   - Use `crypto/subtle.ConstantTimeCompare`

3. **Path Traversal (CWE-22)** - 5 instances
   - Sanitize with `filepath.Clean`
   - Validate against directory traversal

4. **Weak Cryptography**
   - Upgrade RSA keys from 2048 to 4096 bits
   - Make key sizes configurable
   - Replace any MD5/SHA1 with SHA-256+

5. **JWT Secret Management**
   - Move to AWS Secrets Manager with rotation
   - Remove any hardcoded secrets

## High Priority Issues - SHOULD FIX

### 1. Code Duplication Reduction ⚠️
**Note:** golangci-lint already configured with dupl (threshold: 150)
- **Major Patterns to Extract:**
  - Pagination logic (30+ duplications)
  - Authentication extraction
  - Error response formatting
  - Rate limit checking
- **Actions:**
  1. Create `pkg/common/pagination.go`
  2. Implement unified auth middleware
  3. Centralize error response handling
- **Timeline:** 3-4 days

### 2. Environment Configuration ⚠️
- **Problem:** 71 instances of direct `os.Getenv()` usage
- **Action:** Replace with centralized `config.Get()`
- **Exception:** AWS Lambda runtime variables
- **Timeline:** 1-2 days

### 3. TODO/FIXME Resolution ⚠️
- **Count:** 18-22 TODO comments
- **Critical TODOs:**
  - Attachment transformations in `pkg/transformations/converters.go`
  - Relationship integration in `pkg/storage/repositories/status_repository.go`
- **Action:** Create GitHub issues for tracking
- **Timeline:** Ongoing

### 4. Error Handling Consistency ⚠️
- **Problem:** API handlers use string matching instead of structured errors
- **Actions:**
  1. Refactor API handlers to use `AppError` types
  2. Propagate structured errors from service layer
  3. Remove brittle string matching
- **Timeline:** 3-4 days

## Medium Priority Issues

### 1. Performance Optimization
- **Issues Identified:**
  - N+1 query problems
  - Large object serialization
  - Federation key caching
- **Actions:**
  1. Implement DataLoader pattern
  2. Consider DynamoDB DAX for reads
  3. Optimize ActivityPub serialization
- **Timeline:** Post-release

### 2. Security Hardening
- **Actions:**
  1. Implement security headers middleware
  2. Ensure gosec runs in CI (already in golangci.yml)
  3. Add request signature verification
  4. Set up AWS WAF and GuardDuty
- **Timeline:** 1 week

### 3. HTTP Client Management
- **Problem:** 2 instances of `http.DefaultClient`, 1 direct `http.Get()`
- **Action:** Use configured clients with timeouts
- **Timeline:** 1 day

## Low Priority Issues

### 1. Linting Improvements
**Note:** golangci-lint already well-configured with:
- gosec, govet, staticcheck enabled
- gocyclo (15), gocognit (20) thresholds set
- dupl (150 token threshold)
- revive with sensible rules

**Additional Actions:**
- Run `make lint` regularly
- Add pre-commit hooks
- Address linting warnings systematically

### 2. Documentation
- Add service layer inline docs
- Create API migration guides
- Consolidate into `DEVELOPER_GUIDELINES.md`

### 3. Dependency Updates
- Migrate from `gorilla/mux` to stdlib (already flagged)
- Update AWS SDK versions as needed
- Run `govulncheck` for scanning

## Testing Philosophy

Given the AI-generated nature and preference for production testing:
- **Skip:** Traditional unit test coverage requirements
- **Focus:** Integration tests in production environment
- **Maintain:** Existing Python API tests in `tests/`
- **Monitor:** Production metrics and real-world usage
- **Leverage:** Existing test harness for critical paths

## Recommended Timeline

### Week 1: Critical Foundation
- Days 1-2: Complete storage migration
- Day 3: Replace Sonic with JSON v2 (when Go 1.25 available)
- Days 4-5: Fix critical security vulnerabilities

### Week 2: Quality and Security
- Days 1-2: Reduce code duplication
- Days 2-3: Fix error handling consistency
- Days 3-4: Security hardening
- Day 5: Environment configuration cleanup

### Week 3: Polish
- Days 1-2: Resolve critical TODOs
- Days 2-3: HTTP client management
- Days 3-5: Run full linting and fix issues

### Week 4: Release Preparation
- Days 1-2: Final security review
- Days 2-3: Documentation updates
- Days 3-4: Production deployment testing
- Days 4-5: Launch preparation

## Success Metrics

- [ ] Storage migration complete (0 `h.store` references)
- [ ] Sonic replaced with native JSON v2
- [ ] All high-severity gosec issues resolved
- [ ] Zero critical TODOs remaining
- [ ] golangci-lint passes without errors
- [ ] Production deployment successful

## Tools Already Available

- **Linting:** `make lint` with comprehensive golangci.yml
- **Migration:** `tools/migrate_lift_storage_to_repos.go`
- **Testing:** Python API tests in `tests/`
- **Security:** gosec integrated in golangci-lint

## Conclusion

The Lesser codebase is well-architected with good tooling already in place. Focus on:
1. Completing the storage migration
2. Replacing Sonic with native JSON v2
3. Addressing security vulnerabilities
4. Leveraging existing tools (golangci-lint, migration scripts)

Given the AI-generated nature and production testing philosophy, the traditional emphasis on unit test coverage can be deprioritized in favor of real-world validation.

**Estimated Total Effort:** 2-3 weeks with 1-2 developers
**Confidence Level:** High, with clear actionable items and existing tooling