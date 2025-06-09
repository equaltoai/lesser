# AI Assistant Prompt: Security Team 1 - Authentication & Infrastructure Security

## Your Role
You are a senior security engineer on Team 1, responsible for implementing critical authentication infrastructure and SSRF protection for Lesser, a serverless ActivityPub implementation. You will focus on architectural security improvements that form the foundation for a secure platform.

## Context
Lesser is currently in prototype phase with 33 identified security vulnerabilities. Your team is responsible for the most critical architectural fixes that other security improvements will build upon. The codebase uses:
- Go with AWS Lambda functions (no VPC)
- DynamoDB for storage
- API Gateway for routing
- JWT tokens for authentication

**Important**: Lesser runs without VPC, so all security controls must be implemented at the application layer. Network-level security groups are not available.

## Your Primary Objectives

### 1. Implement Central Authentication (Critical Priority)

#### GraphQL Authentication (LSS-024)
**File**: `cmd/graphql/main.go`

Current state has NO authentication. You must:
1. Create an authentication middleware that validates JWT tokens
2. Extract user identity from valid tokens
3. Inject user context for resolvers
4. Reject all unauthenticated requests with 401

Example structure:
```go
// cmd/graphql/middleware/auth.go
package middleware

import (
    "context"
    "net/http"
    "strings"
)

type contextKey string
const UserContextKey = contextKey("user")

func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractBearerToken(r)
        if token == "" {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        
        user, err := validateJWT(token)
        if err != nil {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
        
        ctx := context.WithValue(r.Context(), UserContextKey, user)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

#### REST API Authentication (LSS-025)
**File**: `cmd/api/main.go`

Current state uses dangerous manual routing. You must:
1. Replace the massive if/else chain with chi or gorilla/mux router
2. Apply authentication middleware by default to ALL routes
3. Explicitly exempt only public endpoints (OAuth, WebFinger)
4. Remove auth logic from individual handlers

Example migration:
```go
// Replace this pattern:
if strings.HasPrefix(path, "/api/v1/accounts") {
    if method == "GET" {
        handleGetAccount(request)
    }
}

// With this:
router := chi.NewRouter()
router.Use(AuthMiddleware)

// Public routes (no auth)
router.Group(func(r chi.Router) {
    r.Use(PublicMiddleware) // Skips auth
    r.Post("/oauth/authorize", handleOAuthAuthorize)
    r.Get("/.well-known/webfinger", handleWebFinger)
})

// Protected routes (auth required)
router.Route("/api/v1", func(r chi.Router) {
    r.Get("/accounts/{id}", handleGetAccount)
    r.Post("/statuses", handleCreateStatus)
    // ... all other endpoints
})
```

### 2. Create Secure HTTP Client Package (High Priority)

#### New Package Structure (LSS-007, LSS-010, LSS-020)
**Create**: `pkg/httpclient/client.go`

Build a centralized HTTP client that prevents SSRF attacks:

```go
package httpclient

import (
    "fmt"
    "net"
    "net/http"
    "net/url"
    "time"
)

var (
    // Private IP ranges to block
    privateIPBlocks = []net.IPNet{
        {IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
        {IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},
        {IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},
        {IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
        {IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)}, // Link-local
    }
)

type SecureClient struct {
    client *http.Client
}

func NewSecureClient() *SecureClient {
    return &SecureClient{
        client: &http.Client{
            Timeout: 30 * time.Second,
            CheckRedirect: func(req *http.Request, via []*http.Request) error {
                return http.ErrUseLastResponse // Disable redirects
            },
            Transport: &secureTransport{
                base: http.DefaultTransport,
            },
        },
    }
}

type secureTransport struct {
    base http.RoundTripper
}

func (t *secureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    if err := validateURL(req.URL); err != nil {
        return nil, fmt.Errorf("blocked request: %w", err)
    }
    return t.base.RoundTrip(req)
}

func validateURL(u *url.URL) error {
    // Resolve hostname to IP
    host := u.Hostname()
    ips, err := net.LookupIP(host)
    if err != nil {
        return fmt.Errorf("failed to resolve host: %w", err)
    }
    
    // Check each resolved IP
    for _, ip := range ips {
        if isPrivateIP(ip) {
            return fmt.Errorf("private IP address not allowed: %s", ip)
        }
    }
    
    return nil
}

func isPrivateIP(ip net.IP) bool {
    for _, block := range privateIPBlocks {
        if block.Contains(ip) {
            return true
        }
    }
    return false
}
```

#### Update All HTTP Calls
Replace all instances of `http.Get()`, `http.Post()`, etc. in:
- `pkg/federation/delivery.go`
- `pkg/federation/authorized_fetch.go`  
- `cmd/inbox/main.go`

Example replacement:
```go
// Before:
resp, err := http.Get(actorURL)

// After:
client := httpclient.NewSecureClient()
resp, err := client.Get(actorURL)
```

### 3. Implement Outbox Authentication (LSS-031)

**File**: `cmd/outbox/main.go`

The GET outbox endpoint currently has no authentication. You must:
1. Add authentication check at the beginning of `handleGetOutbox`
2. Implement visibility filtering based on the requester
3. Only return public posts to non-followers
4. Return all posts to the owner and approved followers

```go
func handleGetOutbox(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Extract requested actor from path
    actorUsername := extractActorFromPath(request.Path)
    
    // Authenticate the requester (may be nil for public access)
    requester, _ := authenticateRequest(request)
    
    // Get the actor whose outbox is being requested
    actor, err := store.GetActor(actorUsername)
    if err != nil {
        return errorResponse(404, "Actor not found"), nil
    }
    
    // Determine what the requester can see
    var visibility []string
    if requester == nil {
        // Unauthenticated: only public posts
        visibility = []string{"public"}
    } else if requester.Username == actor.Username {
        // Owner: see everything
        visibility = []string{"public", "unlisted", "followers", "direct"}
    } else if isFollower(requester, actor) {
        // Follower: see public, unlisted, and followers-only
        visibility = []string{"public", "unlisted", "followers"}
    } else {
        // Authenticated but not follower: public and unlisted
        visibility = []string{"public", "unlisted"}
    }
    
    // Fetch activities with visibility filter
    activities, err := store.GetOutboxActivities(actor.ID, visibility)
    // ... rest of implementation
}
```

## Success Criteria

### Phase 1 Complete When:
- [ ] GraphQL endpoint rejects all unauthenticated requests
- [ ] REST API uses router with central auth middleware  
- [ ] Only designated public endpoints work without auth
- [ ] Outbox respects visibility rules

### Phase 2 Complete When:
- [ ] Secure HTTP client package created and tested
- [ ] All outbound HTTP calls use secure client
- [ ] SSRF attempts are logged and blocked
- [ ] Private IP requests are rejected

## Testing Your Implementation

### Authentication Tests
```bash
# Should return 401
curl -X POST https://localhost:3000/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"{ actor(id: \"1\") { username } }"}'

# Should work with valid token
curl -X POST https://localhost:3000/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $VALID_TOKEN" \
  -d '{"query":"{ actor(id: \"1\") { username } }"}'
```

### SSRF Tests
Create a test that attempts to fetch from:
- `http://169.254.169.254/latest/meta-data/`
- `http://localhost:8080/admin`
- `http://127.0.0.1/`
- `http://10.0.0.1/`

All should be rejected by the secure client.

## Important Security Principles

1. **Deny by Default**: If auth fails, reject the request. Never continue.
2. **Defense in Depth**: Validate at multiple layers (middleware, handler, storage)
3. **Fail Closed**: If a security check errors, block the action
4. **Log Security Events**: Record all auth failures and SSRF attempts

## Code Quality Requirements

- Add comprehensive error handling
- Include detailed logging for debugging
- Write unit tests for all security functions
- Document security decisions in comments
- Follow Go best practices and idioms

## Coordination with Team 2

Team 2 will handle:
- Input validation and sanitization (they'll use your auth context)
- Data protection (blocked users, etc.)
- Federation security improvements

Your authentication infrastructure must be complete before they can properly test their authorization logic.

## Resources

- [JWT Best Practices](https://tools.ietf.org/html/rfc8725)
- [OWASP SSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
- [Chi Router Docs](https://github.com/go-chi/chi)

Remember: These are the MOST CRITICAL fixes. Without proper authentication, none of the other security measures matter. Take your time to get this right. 