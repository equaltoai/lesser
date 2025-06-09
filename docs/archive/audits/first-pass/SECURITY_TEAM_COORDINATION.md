# Security Implementation Team Coordination

## Overview
This document coordinates the parallel security implementation work between Team 1 (Authentication & Infrastructure) and Team 2 (Input Validation & Data Protection).

## Team Assignments

### Team 1: Authentication & Infrastructure Security
**Focus**: Foundation security that other features depend on
- Central authentication for GraphQL and REST APIs
- Secure HTTP client to prevent SSRF
- Outbox authentication and visibility

**Critical Path**: Other teams need auth infrastructure first

### Team 2: Input Validation & Data Protection  
**Focus**: Preventing injection attacks and data leaks
- XSS prevention with proper sanitization
- Block list enforcement
- Request size limits and file validation
- Path traversal prevention

**Dependencies**: Some features need Team 1's auth context

## Implementation Timeline

### Week 1 Sprint Plan

#### Day 1-2: Parallel Foundation Work
**Team 1**:
- [ ] Create auth middleware package
- [ ] Start GraphQL auth implementation
- [ ] Design secure HTTP client interface

**Team 2**:
- [ ] Replace HTML sanitizer with bluemonday
- [ ] Create request size limit utilities
- [ ] Design file validation framework

#### Day 3-4: Integration Points
**Team 1**:
- [ ] Complete REST API router migration
- [ ] Finish secure HTTP client
- [ ] Start outbox authentication

**Team 2**:
- [ ] Implement block list filtering (needs auth context)
- [ ] Add size limits to all handlers
- [ ] Complete file type validation

#### Day 5: Testing & Integration
**Both Teams**:
- [ ] Integration testing
- [ ] Security test suite
- [ ] Code review exchange
- [ ] Documentation updates

## Key Integration Points

### 1. Authentication Context
Team 1 provides → Team 2 consumes

```go
// Team 1 provides in context:
type User struct {
    ID       string
    Username string
    Roles    []string
}

// Team 2 retrieves:
user := ctx.Value(middleware.UserContextKey).(User)
```

### 2. Secure HTTP Client
Team 1 provides → Both teams use

```go
// Team 1 creates:
client := httpclient.NewSecureClient()

// Team 2 uses for federation:
resp, err := client.Get(remoteURL)
```

### 3. Error Handling Standards
Both teams follow consistent patterns:

```go
// Security errors are logged but return generic messages
logger.Error("Security violation", 
    zap.String("type", "xss_attempt"),
    zap.String("user", user.ID))
return errors.New("invalid input")
```

## Code Review Checklist

### For Team 1 Reviews:
- [ ] Auth middleware covers all endpoints
- [ ] No auth logic in individual handlers  
- [ ] SSRF protection is comprehensive
- [ ] Errors don't leak sensitive info

### For Team 2 Reviews:
- [ ] All user input is sanitized
- [ ] Size limits prevent DoS
- [ ] File types are validated
- [ ] Block lists are enforced

## Testing Coordination

### Shared Test Data
```go
// test/fixtures/users.go
var (
    TestUserAlice = User{
        ID:       "user:alice",
        Username: "alice",
    }
    TestUserBob = User{
        ID:       "user:bob", 
        Username: "bob",
    }
    TestUserMallory = User{
        ID:       "user:mallory",
        Username: "mallory",
        Blocked:  true,
    }
)
```

### Integration Test Scenarios
1. **Authenticated XSS Attempt**: Team 1 auth + Team 2 sanitization
2. **Blocked User Activity**: Team 1 auth + Team 2 block filter
3. **Large File Upload**: Team 1 auth + Team 2 size limits
4. **SSRF via Federation**: Team 1 HTTP client + Team 2 validation

## Communication Protocol

### Daily Sync Points
- **Morning**: Share overnight progress
- **Midday**: Discuss any blockers
- **Evening**: Integration test results

### Escalation Path
1. Try to resolve between teams
2. Escalate blocking issues immediately
3. Security questions → Security team lead
4. Architecture questions → Tech lead

## Definition of Done

### Team 1 Deliverables:
- [ ] Auth middleware package with tests
- [ ] GraphQL using auth middleware
- [ ] REST API using router + auth
- [ ] Secure HTTP client package
- [ ] Outbox with visibility rules
- [ ] Documentation updated

### Team 2 Deliverables:
- [ ] XSS protection with bluemonday
- [ ] Block list filtering implemented
- [ ] Size limits on all endpoints
- [ ] File validation framework
- [ ] Path traversal prevention
- [ ] Secure ID generation

### Joint Deliverables:
- [ ] Integration test suite
- [ ] Security test coverage
- [ ] Updated security tracking
- [ ] Deployment instructions

## Success Metrics

- **Zero** auth bypass vulnerabilities
- **Zero** XSS vulnerabilities in tests
- **100%** of endpoints have size limits
- **100%** of HTTP calls use secure client
- All critical issues from audit resolved

## Risk Management

### Potential Blockers:
1. **Auth complexity** - Team 2 blocked if auth isn't ready
   - *Mitigation*: Team 2 can mock auth context for testing

2. **Integration issues** - Middleware conflicts
   - *Mitigation*: Daily integration tests

3. **Performance impact** - Security adds latency
   - *Mitigation*: Benchmark critical paths

## Next Phase Preview

After Week 1 completion, both teams will:
- Implement remaining medium priority issues
- Add comprehensive security monitoring
- Create security runbooks
- Prepare for security re-audit

---

Remember: **Communication is key**. Don't work in silos. Share early, share often. 