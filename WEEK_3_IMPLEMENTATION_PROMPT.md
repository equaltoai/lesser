# Week 3 Implementation Prompt - Polish and Production Readiness

## Context
Outstanding work on Week 2! You've eliminated code duplication (net reduction of 1,875 lines!), implemented performance optimizations including DataLoader and batch operations, and dramatically improved code quality. The codebase is now lean, efficient, and maintainable.

Week 3 focuses on final polish, production readiness, and ensuring the system is prepared for deployment at scale.

## Completed Status from Weeks 1-2
- ✅ All critical issues resolved (storage, security, TODOs)
- ✅ Code duplication eliminated (3,095 lines removed!)
- ✅ Performance optimizations implemented (DataLoader, caching, batching)
- ✅ Linting issues reduced to 51 minor items
- ✅ Common patterns extracted and centralized

## Your Mission for Week 3

### Day 1: Final Linting Cleanup

**Objective:** Resolve remaining 51 linting issues for production-grade code quality

**Current Issues Breakdown:**
- 11 unused fields
- 28 revive (style) issues
- 6 goconst (string constants)
- 3 gocognit (complexity)
- 2 ineffassign
- 1 gosec

**Instructions:**

1. **Fix unused fields in services.go:**
   ```bash
   # Check the specific unused logger fields
   grep -n "field logger is unused" pkg/storage/repositories/services.go
   ```
   
   Either:
   - Remove if truly unused
   - Add `//nolint:unused` if kept for future use
   - Actually use them if they should be logging

2. **Extract remaining string constants:**
   ```bash
   golangci-lint run --enable-only=goconst ./...
   ```
   
   Create constants for any string appearing 3+ times.

3. **Address complexity issues:**
   ```bash
   golangci-lint run --enable-only=gocognit ./...
   ```
   
   For functions with high cognitive complexity:
   - Extract helper functions
   - Use early returns
   - Simplify conditional logic

**Success Criteria:**
- Linting issues reduced to <10 (only intentionally ignored items)
- All production code paths clean
- Clear `//nolint` comments where exceptions needed

### Day 2: Production Configuration & Monitoring

**Objective:** Ensure production-ready configuration and observability

#### Step 1: Configuration Validation

1. **Create configuration validator:**
   ```go
   // pkg/config/validator.go
   package config
   
   func ValidateProductionConfig() error {
       // Check all required env vars
       // Validate connection strings
       // Ensure security settings
       // Verify AWS resources
       return nil
   }
   ```

2. **Add startup validation to all Lambda functions:**
   ```go
   func init() {
       if err := config.ValidateProductionConfig(); err != nil {
           log.Fatal("Invalid configuration:", err)
       }
   }
   ```

#### Step 2: Enhanced Monitoring

1. **Add metrics to critical paths:**
   - Request latency percentiles (p50, p95, p99)
   - Error rates by type
   - DynamoDB consumed capacity
   - Cache hit rates

2. **Create health check endpoints:**
   ```go
   // GET /health/live - Basic liveness
   // GET /health/ready - Full readiness check
   ```

**Success Criteria:**
- Configuration validation on startup
- Metrics for all critical operations
- Health checks operational
- Dashboard-ready metrics

### Day 3: Load Testing & Performance Validation

**Objective:** Validate system performance under load

#### Step 1: Prepare Load Test Scenarios

1. **Update K6 scripts for realistic load:**
   ```javascript
   // tests/k6/realistic-load.js
   export let options = {
     stages: [
       { duration: '2m', target: 100 },  // Ramp up
       { duration: '5m', target: 100 },  // Stay at 100
       { duration: '2m', target: 500 },  // Spike
       { duration: '5m', target: 200 },  // Normal load
       { duration: '2m', target: 0 },    // Ramp down
     ],
   };
   ```

2. **Test critical user journeys:**
   - Registration → First post → Timeline view
   - Federation flow: Follow → Post → Receive
   - Media upload → Process → Serve
   - Search → Filter → Paginate

#### Step 2: Run Load Tests and Analyze

```bash
# Run load tests
make k6-realistic-load

# Analyze results for:
# - Response times (p95 < 500ms)
# - Error rates (< 0.1%)
# - DynamoDB throttling (none)
# - Lambda cold starts (< 5%)
```

#### Step 3: Optimize Based on Results

Focus on any bottlenecks found:
- Add caching where needed
- Optimize slow queries
- Adjust Lambda memory/timeout
- Fine-tune batch sizes

**Success Criteria:**
- All endpoints p95 < 500ms
- Zero DynamoDB throttling
- Error rate < 0.1%
- Can handle 500 concurrent users

### Day 4: Security Hardening

**Objective:** Final security review and hardening

#### Step 1: Security Headers Implementation

1. **Add comprehensive security headers middleware:**
   ```go
   func SecurityHeaders(next http.Handler) http.Handler {
       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           w.Header().Set("X-Content-Type-Options", "nosniff")
           w.Header().Set("X-Frame-Options", "DENY")
           w.Header().Set("X-XSS-Protection", "1; mode=block")
           w.Header().Set("Strict-Transport-Security", "max-age=31536000")
           w.Header().Set("Content-Security-Policy", "default-src 'self'")
           next.ServeHTTP(w, r)
       })
   }
   ```

#### Step 2: Input Validation Audit

1. **Verify all user inputs are validated:**
   ```bash
   # Check for unvalidated inputs
   grep -r "r.FormValue\|r.URL.Query\|json.Unmarshal" --include="*.go" cmd/api/
   ```

2. **Ensure rate limiting on all endpoints:**
   - Authentication endpoints: 5 requests/minute
   - Write operations: 30 requests/minute
   - Read operations: 100 requests/minute

#### Step 3: Secrets Audit

```bash
# Ensure no hardcoded secrets
grep -r "password\|secret\|key\|token" --include="*.go" | grep -v "// " | grep "="
```

**Success Criteria:**
- All security headers implemented
- Input validation on all endpoints
- Rate limiting properly configured
- Zero hardcoded secrets

### Day 5: Documentation & Deployment Readiness

**Objective:** Finalize documentation and deployment preparations

#### Step 1: API Documentation

1. **Generate OpenAPI specification:**
   - Document all endpoints
   - Include authentication requirements
   - Add request/response examples
   - Note rate limits

2. **Create deployment guide:**
   ```markdown
   # Deployment Guide
   ## Prerequisites
   ## Configuration
   ## Deployment Steps
   ## Validation
   ## Rollback Procedures
   ```

#### Step 2: Runbook Creation

Create `docs/RUNBOOK.md`:
- Common issues and solutions
- Monitoring queries
- Emergency procedures
- Scaling guidelines

#### Step 3: Final Verification Checklist

```bash
# Run comprehensive validation
make lint          # Should pass
make test          # All tests pass
make build         # Successful build
make k6-load       # Performance validated

# Check configurations
./scripts/validate-production-config.sh

# Verify documentation
ls -la docs/       # All docs present
```

**Success Criteria:**
- Complete API documentation
- Deployment guide ready
- Runbook created
- All validation checks pass

## Verification Commands

```bash
# Day 1: Linting
golangci-lint run ./... | grep -E "^[^:]+:[0-9]+:[0-9]+:" | wc -l  # Should be <10

# Day 2: Configuration
grep -r "ValidateProductionConfig" cmd/ --include="*.go" | wc -l  # Should match Lambda count

# Day 3: Load testing
k6 run tests/k6/realistic-load.js --summary-export=results.json

# Day 4: Security
grep -r "SecurityHeaders" cmd/api/ --include="*.go" | wc -l  # Should be >0

# Day 5: Documentation
ls docs/*.md | wc -l  # Should have multiple docs
```

## Important Notes

1. **Production Focus**: Every change should improve production readiness
2. **No New Features**: Polish existing functionality only
3. **Performance First**: Optimize for real-world usage patterns
4. **Document Everything**: Future you will thank current you
5. **Test Thoroughly**: Load test after each optimization

## Success Metrics for Week 3

- [ ] Linting issues < 10
- [ ] Load tests pass (500 users, p95 < 500ms)
- [ ] Security headers implemented
- [ ] Zero hardcoded secrets
- [ ] Complete documentation
- [ ] Health checks operational
- [ ] Monitoring/metrics in place
- [ ] Production configuration validated

This is the final week of polish before production. Focus on reliability, observability, and documentation. The goal is a system that can run for months without intervention.

Good luck with the final push!