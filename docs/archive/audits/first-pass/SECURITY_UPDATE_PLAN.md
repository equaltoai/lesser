# Lesser Security Update Plan

## Executive Summary

The security audit identified 33 vulnerabilities across Lesser's codebase, including 4 critical and 4 high-severity issues. This plan provides a structured approach to remediate these vulnerabilities, prioritizing the most severe issues and implementing architectural improvements to prevent future vulnerabilities.

## Remediation Timeline

### Phase 1: Critical Authentication & Authorization (Week 1)
**Goal**: Implement authentication across all API endpoints

#### 1.1 GraphQL Authentication (LSS-024) - CRITICAL
- [ ] Create authentication middleware for GraphQL endpoint
- [ ] Extract and validate JWT tokens from headers
- [ ] Inject authenticated user context into resolvers
- [ ] Add authorization checks to all GraphQL resolvers
- [ ] Test all queries and mutations require authentication

**Implementation**:
```go
// cmd/graphql/middleware/auth.go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        user, err := validateToken(token)
        if err != nil {
            http.Error(w, "Unauthorized", 401)
            return
        }
        ctx := context.WithValue(r.Context(), "user", user)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

#### 1.2 REST API Central Authentication (LSS-025) - CRITICAL
- [ ] Replace manual routing with chi or gorilla/mux router
- [ ] Implement authentication middleware
- [ ] Apply middleware to all routes by default
- [ ] Explicitly mark public endpoints
- [ ] Remove authentication from individual handlers

**Implementation**:
```go
// cmd/api/main.go
router := chi.NewRouter()
router.Use(authMiddleware)

// Public routes
router.Group(func(r chi.Router) {
    r.Use(publicMiddleware) // Skip auth
    r.Post("/oauth/authorize", handleOAuthAuthorize)
    r.Get("/.well-known/webfinger", handleWebFinger)
})

// Protected routes
router.Route("/api/v1", func(r chi.Router) {
    r.Get("/accounts/{id}", handleGetAccount)
    // ... all other endpoints
})
```

#### 1.3 Outbox Authentication (LSS-031) - HIGH
- [ ] Add authentication to GET outbox endpoint
- [ ] Implement visibility filtering based on requester
- [ ] Only show public posts to non-followers
- [ ] Respect protected account settings

### Phase 2: SSRF Protection (Week 1-2)
**Goal**: Prevent server-side request forgery across all services

#### 2.1 Centralized HTTP Client (LSS-007, LSS-010, LSS-020)
- [ ] Create `pkg/httpclient` package
- [ ] Implement URL validation against private IP ranges
- [ ] Disable HTTP redirects
- [ ] Add request timeouts
- [ ] Replace all direct HTTP calls with secure client

**Implementation**:
```go
// pkg/httpclient/client.go
func NewSecureClient() *http.Client {
    return &http.Client{
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return http.ErrUseLastResponse // Disable redirects
        },
        Timeout: 30 * time.Second,
        Transport: &secureTransport{},
    }
}

func (t *secureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    if err := validateURL(req.URL); err != nil {
        return nil, fmt.Errorf("invalid URL: %w", err)
    }
    return http.DefaultTransport.RoundTrip(req)
}

func validateURL(u *url.URL) error {
    // Check against private IP ranges
    // 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8
    // 169.254.169.254 (AWS metadata)
}
```

#### 2.2 Application-Level Protection (No VPC)
- [ ] All SSRF protection implemented in secure HTTP client
- [ ] Log all blocked SSRF attempts for monitoring
- [ ] Configure Lambda function permissions with least privilege
- [ ] Use API Gateway request validation where possible

### Phase 3: Input Validation & DoS Prevention (Week 2)
**Goal**: Prevent denial of service and injection attacks

#### 3.1 Request Size Limits (LSS-021, LSS-029, LSS-032)
- [ ] Add size limits to all request body reads
- [ ] Implement file size checks before processing
- [ ] Use io.LimitReader consistently

**Implementation**:
```go
const maxRequestSize = 1 * 1024 * 1024 // 1MB

func readBody(r io.Reader) ([]byte, error) {
    limited := io.LimitReader(r, maxRequestSize)
    return io.ReadAll(limited)
}
```

#### 3.2 HTML Sanitization (LSS-001) - CRITICAL
- [ ] Replace custom sanitizer with bluemonday
- [ ] Use strict UGC policy
- [ ] Add comprehensive tests

**Implementation**:
```go
import "github.com/microcosm-cc/bluemonday"

var sanitizer = bluemonday.UGCPolicy()

func SanitizeHTML(input string) string {
    return sanitizer.Sanitize(input)
}
```

#### 3.3 File Type Validation (LSS-027) - HIGH
- [ ] Implement magic byte detection
- [ ] Validate files before processing
- [ ] Maintain allowlist of safe file types

### Phase 4: Data Protection (Week 2-3)
**Goal**: Prevent data leakage and ensure privacy

#### 4.1 Block List Enforcement (LSS-030) - CRITICAL
- [ ] Filter delivery recipients against blocklist
- [ ] Implement in outbox delivery function
- [ ] Add comprehensive tests
- [ ] Log blocked delivery attempts

**Implementation**:
```go
func filterBlockedRecipients(actor Actor, recipients []string) []string {
    blocked, _ := store.GetBlockedActors(actor.ID)
    blockedMap := make(map[string]bool)
    for _, b := range blocked {
        blockedMap[b] = true
    }
    
    filtered := []string{}
    for _, r := range recipients {
        if !blockedMap[r] {
            filtered = append(filtered, r)
        }
    }
    return filtered
}
```

#### 4.2 Path Traversal Prevention (LSS-028)
- [ ] Sanitize usernames in S3 keys
- [ ] Implement strict validation regex
- [ ] Add tests for malicious usernames

### Phase 5: Cryptographic & ID Security (Week 3)
**Goal**: Improve randomness and cryptographic operations

#### 5.1 Secure Random IDs (LSS-009, LSS-022, LSS-033)
- [ ] Replace all math/rand with crypto/rand
- [ ] Create centralized ID generation functions
- [ ] Update all services to use secure IDs

**Implementation**:
```go
import "crypto/rand"

func GenerateSecureID() string {
    b := make([]byte, 16)
    _, err := rand.Read(b)
    if err != nil {
        panic(err)
    }
    return hex.EncodeToString(b)
}
```

#### 5.2 Improve Cryptographic Operations (LSS-005)
- [ ] Explicitly use crypto/rand.Reader in RSA operations
- [ ] Review all cryptographic code for best practices

### Phase 6: Enhanced Security Features (Week 3-4)
**Goal**: Implement advanced security controls

#### 6.1 Rate Limiter Improvements (LSS-019)
- [ ] Implement explicit lockout records
- [ ] Add TTL-based lockout storage
- [ ] Simplify rate limit checking logic

#### 6.2 Session Security (LSS-014)
- [ ] Implement refresh token reuse detection
- [ ] Revoke entire session family on reuse
- [ ] Add security alerting for token reuse

#### 6.3 Logging Security (LSS-015)
- [ ] Replace all fmt.Printf with structured logging
- [ ] Ensure no sensitive data in logs
- [ ] Configure log aggregation and monitoring

### Phase 7: Federation Security (Week 4)
**Goal**: Secure federation mechanisms

#### 7.1 Type-Safe Unmarshaling (LSS-002, LSS-011)
- [ ] Implement custom UnmarshalJSON for any fields
- [ ] Add JSON size limits for federation
- [ ] Validate all incoming ActivityPub objects

#### 7.2 HTTP Signature Improvements (LSS-006, LSS-012)
- [ ] Support multiple signature algorithms
- [ ] Verify keyId matches fetched actor
- [ ] Add comprehensive validation

#### 7.3 Delivery Reliability (LSS-008)
- [ ] Implement exponential backoff retry
- [ ] Add retry queues for failed deliveries
- [ ] Monitor delivery success rates

### Phase 8: Final Hardening (Week 5)
**Goal**: Complete remaining security improvements

#### 8.1 Validation Coverage (LSS-003)
- [ ] Implement validation for all ActivityPub types
- [ ] Add validation tests for each type
- [ ] Ensure consistent validation across services

#### 8.2 WebAuthn Enhancements (LSS-016, LSS-017, LSS-018)
- [ ] Make user verification configurable
- [ ] Support multiple origins via configuration
- [ ] Implement resident key support

#### 8.3 Security Posture Improvements (LSS-004, LSS-023)
- [ ] Fix validation error messages
- [ ] Change to "fail closed" for security checks
- [ ] Review all error handling for security implications

## Testing Strategy

### Security Testing Checklist
- [ ] Authentication bypass attempts on all endpoints
- [ ] SSRF testing with various URL formats
- [ ] XSS payload testing after sanitization
- [ ] File upload with malicious files
- [ ] Rate limiting effectiveness
- [ ] Token reuse detection
- [ ] Blocklist enforcement
- [ ] Path traversal attempts

### Automated Security Scanning
- [ ] Set up SAST (Static Application Security Testing)
- [ ] Configure dependency vulnerability scanning
- [ ] Implement security regression tests
- [ ] Add security checks to CI/CD pipeline

## Monitoring & Alerting

### Security Monitoring
- [ ] Failed authentication attempts
- [ ] Unusual request patterns
- [ ] SSRF attempt detection
- [ ] File type mismatches
- [ ] Token reuse events
- [ ] Rate limit violations

### Security Metrics
- [ ] Track remediation progress
- [ ] Monitor security event frequency
- [ ] Measure mean time to detect/respond
- [ ] Calculate security debt reduction

## Long-term Security Improvements

### Architectural Changes
1. **API Gateway Authorizers**: Move auth to API Gateway layer
2. **WAF Integration**: Add Web Application Firewall rules
3. **Secret Management**: Use AWS Secrets Manager
4. **Infrastructure as Code**: Security policies in Pulumi

### Process Improvements
1. **Security Reviews**: Mandatory for new features
2. **Threat Modeling**: For major changes
3. **Security Training**: For development team
4. **Bug Bounty**: Consider after Phase 8

## Success Criteria

### Phase 1 Complete When:
- All API endpoints require authentication
- No unauthenticated data access possible
- Central auth middleware in place

### Phase 2 Complete When:
- No direct HTTP calls in codebase
- All outbound requests use secure client
- SSRF attempts logged and blocked

### All Phases Complete When:
- All critical/high vulnerabilities remediated
- Security test suite passing
- Monitoring and alerting operational
- Security documentation updated

## Risk Mitigation During Remediation

### Deployment Strategy
- Deploy security fixes to staging first
- Run full regression tests
- Monitor for breaking changes
- Have rollback plan ready

### Communication
- Notify users of security improvements
- Document any breaking changes
- Provide migration guides if needed

---

*This security update plan addresses all 33 findings from the security audit. Following this plan will transform Lesser from a prototype into a production-ready, secure platform.* 