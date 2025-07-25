# AI Assistant Prompt: Security Team 2 Week 2 - Validation & Rate Limiting

## Your Role
You are continuing as the senior security engineer on Team 2. In Week 1, you successfully fixed XSS vulnerabilities, blocked user protections, and request size limits. Now you'll implement comprehensive validation, prevent SQL injection, and add rate limiting.

## Week 1 Accomplishments ✅
- XSS prevention with bluemonday
- Blocked user data protection
- Request size limits enforced
- File type validation
- Path traversal prevention
- Secure ID generation

## Context
You can now rely on Team 1's authentication infrastructure:
- User context available via `ctx.Value(middleware.UserContextKey)`
- All endpoints have auth (except explicitly public ones)
- Secure HTTP client available for outbound requests

## Week 2 Objectives

### 1. Username & Domain Validation (LSS-003) - Medium Priority

#### Comprehensive Input Validation
**Create**: `pkg/activitypub/validators.go`

```go
package activitypub

import (
    "fmt"
    "net/url"
    "regexp"
    "strings"
)

var (
    // Username: 1-30 chars, alphanumeric + underscore, no double underscore
    usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_]{0,28}[a-zA-Z0-9])?$`)
    
    // Domain: valid hostname
    domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
    
    // Webfinger format
    webfingerRegex = regexp.MustCompile(`^acct:([^@]+)@([^@]+)$`)
)

func ValidateUsername(username string) error {
    if username == "" {
        return fmt.Errorf("username cannot be empty")
    }
    
    if len(username) > 30 {
        return fmt.Errorf("username too long (max 30 characters)")
    }
    
    if !usernameRegex.MatchString(username) {
        return fmt.Errorf("username can only contain letters, numbers, and underscores")
    }
    
    // Check for reserved usernames
    reserved := []string{"admin", "root", "system", "api", "well-known"}
    lowerUsername := strings.ToLower(username)
    for _, r := range reserved {
        if lowerUsername == r {
            return fmt.Errorf("username '%s' is reserved", username)
        }
    }
    
    return nil
}

func ValidateDomain(domain string) error {
    if domain == "" {
        return fmt.Errorf("domain cannot be empty")
    }
    
    // Check IP addresses are not used as domains
    if net.ParseIP(domain) != nil {
        return fmt.Errorf("IP addresses cannot be used as domains")
    }
    
    if !domainRegex.MatchString(domain) {
        return fmt.Errorf("invalid domain format")
    }
    
    // Additional checks
    if strings.Contains(domain, "..") {
        return fmt.Errorf("invalid domain: consecutive dots")
    }
    
    return nil
}

func ValidateActorID(actorID string) error {
    u, err := url.Parse(actorID)
    if err != nil {
        return fmt.Errorf("invalid actor ID URL: %w", err)
    }
    
    // Must be HTTPS in production
    if u.Scheme != "https" && u.Scheme != "http" {
        return fmt.Errorf("actor ID must use HTTP(S)")
    }
    
    // Validate domain part
    if err := ValidateDomain(u.Hostname()); err != nil {
        return fmt.Errorf("invalid domain in actor ID: %w", err)
    }
    
    // Path must not be empty
    if u.Path == "" || u.Path == "/" {
        return fmt.Errorf("actor ID must have a path")
    }
    
    return nil
}

func ValidateWebfinger(resource string) error {
    matches := webfingerRegex.FindStringSubmatch(resource)
    if len(matches) != 3 {
        return fmt.Errorf("invalid webfinger format (expected acct:user@domain)")
    }
    
    username := matches[1]
    domain := matches[2]
    
    if err := ValidateUsername(username); err != nil {
        return fmt.Errorf("invalid username in webfinger: %w", err)
    }
    
    if err := ValidateDomain(domain); err != nil {
        return fmt.Errorf("invalid domain in webfinger: %w", err)
    }
    
    return nil
}
```

#### Apply Validation Everywhere
Update all endpoints that accept usernames or domains:

```go
// cmd/actor/main.go
func handleCreateActor(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    var req CreateActorRequest
    json.Unmarshal([]byte(request.Body), &req)
    
    // Validate username
    if err := activitypub.ValidateUsername(req.Username); err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 400,
            Body:       fmt.Sprintf(`{"error": "%s"}`, err.Error()),
        }, nil
    }
    
    // ... rest of handler
}
```

### 2. SQL Injection Prevention (LSS-004) - Medium Priority

#### Parameterized Queries for DynamoDB
**Create**: `pkg/storage/dynamodb/safe_queries.go`

```go
package dynamodb

import (
    "fmt"
    "regexp"
    "github.com/aws/aws-sdk-go/service/dynamodb"
    "github.com/aws/aws-sdk-go/service/dynamodb/expression"
)

// Safe attribute name validation
var safeAttributeRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

func ValidateAttributeName(name string) error {
    if !safeAttributeRegex.MatchString(name) {
        return fmt.Errorf("invalid attribute name: %s", name)
    }
    return nil
}

// Safe query builder
type SafeQueryBuilder struct {
    keyCondition expression.KeyConditionBuilder
    filter       expression.ConditionBuilder
    projection   expression.ProjectionBuilder
}

func NewSafeQuery() *SafeQueryBuilder {
    return &SafeQueryBuilder{}
}

func (q *SafeQueryBuilder) WithKey(attribute string, value any) error {
    if err := ValidateAttributeName(attribute); err != nil {
        return err
    }
    
    key := expression.Key(attribute)
    q.keyCondition = key.Equal(expression.Value(value))
    return nil
}

func (q *SafeQueryBuilder) WithFilter(attribute string, op string, value any) error {
    if err := ValidateAttributeName(attribute); err != nil {
        return err
    }
    
    attr := expression.Name(attribute)
    
    switch op {
    case "=":
        q.filter = attr.Equal(expression.Value(value))
    case "!=":
        q.filter = attr.NotEqual(expression.Value(value))
    case ">":
        q.filter = attr.GreaterThan(expression.Value(value))
    case "<":
        q.filter = attr.LessThan(expression.Value(value))
    case "contains":
        q.filter = attr.Contains(fmt.Sprintf("%v", value))
    default:
        return fmt.Errorf("unsupported operator: %s", op)
    }
    
    return nil
}

func (q *SafeQueryBuilder) Build() (expression.Expression, error) {
    builder := expression.NewBuilder()
    
    if q.keyCondition.IsSet() {
        builder = builder.WithKeyCondition(q.keyCondition)
    }
    
    if q.filter.IsSet() {
        builder = builder.WithFilter(q.filter)
    }
    
    return builder.Build()
}

// Example usage in store
func (s *Store) GetActorsByDomain(domain string) ([]Actor, error) {
    // Validate domain first
    if err := activitypub.ValidateDomain(domain); err != nil {
        return nil, err
    }
    
    // Build safe query
    qb := NewSafeQuery()
    qb.WithFilter("domain", "=", domain)
    
    expr, err := qb.Build()
    if err != nil {
        return nil, err
    }
    
    // Execute query
    result, err := s.db.Scan(&dynamodb.ScanInput{
        TableName:                 aws.String("actors"),
        ExpressionAttributeNames:  expr.Names(),
        ExpressionAttributeValues: expr.Values(),
        FilterExpression:          expr.Filter(),
    })
    
    // ... process results
}
```

#### Prevent NoSQL Injection in Searches
```go
// pkg/storage/dynamodb/search.go
func (s *Store) SearchActors(query string) ([]Actor, error) {
    // Sanitize search query
    sanitized := SanitizeSearchQuery(query)
    
    // Use parameterized expression
    filter := expression.Name("searchableText").Contains(sanitized)
    
    expr, _ := expression.NewBuilder().WithFilter(filter).Build()
    
    // Safe to execute
    return s.scanWithExpression("actors", expr)
}

func SanitizeSearchQuery(query string) string {
    // Remove any DynamoDB expression syntax
    dangerous := []string{"AND", "OR", "NOT", "BETWEEN", "IN", "(", ")", "[", "]"}
    
    result := query
    for _, d := range dangerous {
        result = strings.ReplaceAll(result, d, "")
    }
    
    // Limit length
    if len(result) > 100 {
        result = result[:100]
    }
    
    return strings.TrimSpace(result)
}
```

### 3. Open Redirect Prevention (LSS-006) - Medium Priority

#### URL Validation for Redirects
**Create**: `pkg/common/redirect.go`

```go
package common

import (
    "fmt"
    "net/url"
    "strings"
)

// Whitelist of allowed redirect hosts
var allowedRedirectHosts = map[string]bool{
    "lesser.example.com": true,
    "auth.lesser.example.com": true,
}

func ValidateRedirectURL(redirectURL string, currentHost string) error {
    if redirectURL == "" {
        return fmt.Errorf("redirect URL cannot be empty")
    }
    
    // Parse the URL
    u, err := url.Parse(redirectURL)
    if err != nil {
        return fmt.Errorf("invalid redirect URL: %w", err)
    }
    
    // Relative URLs are safe (same origin)
    if u.Host == "" {
        // But check for protocol-relative URLs
        if strings.HasPrefix(redirectURL, "//") {
            return fmt.Errorf("protocol-relative URLs not allowed")
        }
        return nil
    }
    
    // Check against whitelist
    if allowedRedirectHosts[u.Host] {
        return nil
    }
    
    // Allow same host
    if u.Host == currentHost {
        return nil
    }
    
    return fmt.Errorf("redirect to external host not allowed: %s", u.Host)
}

// Safe redirect handler
func SafeRedirect(w http.ResponseWriter, r *http.Request, defaultPath string) {
    redirectTo := r.URL.Query().Get("redirect_uri")
    
    // Validate the redirect URL
    err := ValidateRedirectURL(redirectTo, r.Host)
    if err != nil {
        logger.Warn("Invalid redirect attempt", 
            zap.String("url", redirectTo),
            zap.Error(err))
        redirectTo = defaultPath
    }
    
    http.Redirect(w, r, redirectTo, http.StatusFound)
}
```

#### Update OAuth Handler
```go
// cmd/auth/main.go
func handleOAuthAuthorize(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    redirectURI := request.QueryStringParameters["redirect_uri"]
    
    // Validate redirect URI
    if err := common.ValidateRedirectURL(redirectURI, request.Headers["Host"]); err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 400,
            Body:       `{"error": "invalid_redirect_uri"}`,
        }, nil
    }
    
    // ... rest of OAuth flow
}
```

### 4. Rate Limiting Implementation (LSS-008) - Medium Priority

#### Distributed Rate Limiter
**Create**: `pkg/ratelimit/limiter.go`

```go
package ratelimit

import (
    "fmt"
    "time"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/service/dynamodb"
)

type RateLimiter struct {
    db        *dynamodb.DynamoDB
    tableName string
}

type RateLimit struct {
    Key       string    // user:endpoint
    Count     int       
    Window    time.Time // Start of current window
    Blocked   bool      // Explicitly blocked
    BlockedUntil time.Time
}

func NewRateLimiter(db *dynamodb.DynamoDB, table string) *RateLimiter {
    return &RateLimiter{db: db, tableName: table}
}

func (rl *RateLimiter) Check(userID, endpoint string, limit int, window time.Duration) error {
    key := fmt.Sprintf("%s:%s", userID, endpoint)
    now := time.Now()
    windowStart := now.Truncate(window)
    
    // Get current rate limit data
    result, err := rl.db.GetItem(&dynamodb.GetItemInput{
        TableName: aws.String(rl.tableName),
        Key: map[string]*dynamodb.AttributeValue{
            "key": {S: aws.String(key)},
        },
    })
    
    var current RateLimit
    if err == nil && result.Item != nil {
        // Parse existing item
        dynamodbattribute.UnmarshalMap(result.Item, &current)
    }
    
    // Check if explicitly blocked
    if current.Blocked && now.Before(current.BlockedUntil) {
        return fmt.Errorf("rate limit exceeded, blocked until %v", current.BlockedUntil)
    }
    
    // Reset if new window
    if current.Window.Before(windowStart) {
        current.Count = 0
        current.Window = windowStart
    }
    
    // Increment counter
    current.Count++
    
    // Check limit
    if current.Count > limit {
        // Block for increasing durations
        blockDuration := time.Duration(current.Count/limit) * time.Hour
        if blockDuration > 24*time.Hour {
            blockDuration = 24 * time.Hour
        }
        
        current.Blocked = true
        current.BlockedUntil = now.Add(blockDuration)
        
        // Update database
        rl.updateLimit(key, current)
        
        return fmt.Errorf("rate limit exceeded (%d > %d)", current.Count, limit)
    }
    
    // Update counter
    rl.updateLimit(key, current)
    
    return nil
}

func (rl *RateLimiter) updateLimit(key string, limit RateLimit) error {
    item, _ := dynamodbattribute.MarshalMap(limit)
    
    _, err := rl.db.PutItem(&dynamodb.PutItemInput{
        TableName: aws.String(rl.tableName),
        Item:      item,
    })
    
    return err
}

// Middleware
func RateLimitMiddleware(limiter *RateLimiter) func(http.HandlerFunc) http.HandlerFunc {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // Get user from context
            user := r.Context().Value(middleware.UserContextKey).(User)
            
            // Define limits per endpoint
            limits := map[string]int{
                "POST:/api/v1/statuses": 30,    // 30 posts per hour
                "POST:/api/v1/media":    10,     // 10 uploads per hour
                "POST:/api/v1/accounts": 5,      // 5 follows per hour
            }
            
            endpoint := fmt.Sprintf("%s:%s", r.Method, r.URL.Path)
            limit, exists := limits[endpoint]
            if !exists {
                limit = 100 // Default limit
            }
            
            // Check rate limit
            err := limiter.Check(user.ID, endpoint, limit, time.Hour)
            if err != nil {
                w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
                w.Header().Set("X-RateLimit-Remaining", "0")
                w.Header().Set("Retry-After", "3600")
                
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            
            next(w, r)
        }
    }
}
```

### 5. Outbox Size Limits (LSS-032) - Medium Priority

Add to existing request size limit utilities:

```go
// cmd/outbox/main.go
func handlePostOutbox(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Apply size limit
    body, err := common.ReadRequestBody(
        strings.NewReader(request.Body), 
        512 * 1024, // 512KB limit for activities
    )
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 413,
            Body:       `{"error": "Request too large"}`,
        }, nil
    }
    
    // Parse activity
    var activity activitypub.Activity
    if err := json.Unmarshal(body, &activity); err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 400,
            Body:       `{"error": "Invalid activity"}`,
        }, nil
    }
    
    // Additional validation for specific activity types
    switch activity.Type {
    case "Create":
        if note, ok := activity.Object.(map[string]any); ok {
            content := note["content"].(string)
            if len(content) > 5000 {
                return events.APIGatewayProxyResponse{
                    StatusCode: 400,
                    Body:       `{"error": "Note content too long (max 5000 chars)"}`,
                }, nil
            }
        }
    }
    
    // ... rest of handler
}
```

## Success Criteria

### Validation Complete When:
- [ ] All username/domain inputs validated
- [ ] Reserved usernames blocked
- [ ] Webfinger format enforced
- [ ] IP addresses rejected as domains

### SQL Injection Prevention When:
- [ ] All queries use safe builders
- [ ] No string concatenation in queries
- [ ] Search queries sanitized
- [ ] Attribute names validated

### Rate Limiting When:
- [ ] Per-user, per-endpoint limits
- [ ] Blocking for repeat offenders
- [ ] Headers indicate limits
- [ ] DynamoDB-backed for distribution

## Testing

### Validation Tests
```go
func TestUsernameValidation(t *testing.T) {
    invalid := []string{
        "",               // empty
        "a",             // too short
        "_underscore",   // starts with underscore
        "admin",         // reserved
        "user name",     // space
        "user@domain",   // @ symbol
        strings.Repeat("a", 31), // too long
    }
    
    for _, username := range invalid {
        if err := ValidateUsername(username); err == nil {
            t.Errorf("Expected error for username: %s", username)
        }
    }
}
```

### Rate Limit Tests
```bash
# Hit rate limit
for i in {1..31}; do
    curl -X POST $API/statuses \
      -H "Authorization: Bearer $TOKEN" \
      -d '{"content": "test"}'
done
# 31st request should fail with 429
```

## Important Notes

1. **Performance**: Use caching for rate limit checks
2. **User Experience**: Return helpful validation errors
3. **Security**: Never trust client-side validation
4. **Monitoring**: Log all rate limit violations

Remember: Input validation is the first line of defense against many attacks! 