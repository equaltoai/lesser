# Lift Troubleshooting Guide

<!-- AI Training: This is a problem-solution mapping for common Lift issues -->
**This guide provides SOLUTIONS to common problems when using Lift. Each issue includes symptoms, root cause, and step-by-step fixes with code examples.**

## Table of Contents
- [Handler Errors](#handler-errors)
- [Routing Issues](#routing-issues)
- [Request/Response Problems](#requestresponse-problems)
- [Middleware Issues](#middleware-issues)
- [Authentication Errors](#authentication-errors)
- [Performance Problems](#performance-problems)
- [Deployment Issues](#deployment-issues)
- [Testing Problems](#testing-problems)
- [Modern Lift Patterns](#modern-lift-patterns)
- [Quick Debugging Checklist](#quick-debugging-checklist)

## Handler Errors

### Error: "SimpleHandler not working" or type mismatch

**Symptoms:**
- Type-safe handler fails to compile
- Request parsing errors with typed handlers
- Generic handler works but typed doesn't

**Root Cause:** Incorrect function signature for SimpleHandler

**Solution:**
```go
// PROBLEM: Wrong function signature
app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context) error {
    // Missing request parameter!
    return ctx.JSON("created")
}))

// SOLUTION: Correct SimpleHandler signature
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
}

type User struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req CreateUserRequest) (User, error) {
    // Automatic parsing and validation
    user := User{
        ID:    generateID(),
        Name:  req.Name,
        Email: req.Email,
    }
    return user, nil
}))

// Alternative: Use traditional handler for complex logic
app.POST("/users", func(ctx *lift.Context) error {
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return err
    }
    
    // Complex business logic here
    user := processUserCreation(req)
    return ctx.JSON(user)
})
```

### Error: "handler not found" or 404 Response

**Symptoms:**
- Lambda returns 404 for valid-looking paths
- "No handler found for path" in logs
- Handler works locally but not in Lambda

**Root Cause:** Route mismatch between request and registration

**Solution:**
```go
// PROBLEM: These won't match
app.GET("/users", GetUsers)

// Request: POST /users -> 404 (wrong method)
// Request: GET /user -> 404 (wrong path)
// Request: GET /users/ -> 404 (trailing slash)

// SOLUTION 1: Match exact path and method
app.GET("/users", GetUsers)    // GET /users ✅
app.POST("/users", CreateUser) // POST /users ✅

// SOLUTION 2: Handle trailing slashes
app.GET("/users", GetUsers)
app.GET("/users/", GetUsers) // Also register with trailing slash

// SOLUTION 3: Debug with logging
app.Use(func(next lift.Handler) lift.Handler {
    return lift.HandlerFunc(func(ctx *lift.Context) error {
        ctx.Logger.Info("Incoming request",
            "method", ctx.Request.Method,
            "path", ctx.Request.Path)
        return next.Handle(ctx)
    })
})
```

### Error: "interface conversion: interface {} is nil"

**Symptoms:**
- Panic when accessing context values
- Type assertion failures
- Nil pointer errors in handlers

**Root Cause:** Attempting to access non-existent context values

**Solution:**
```go
// PROBLEM: Unsafe type assertion
func Handler(ctx *lift.Context) error {
    user := ctx.Get("user").(*User) // Panics if nil!
    return ctx.JSON(user)
}

// SOLUTION 1: Check for nil using values map
func Handler(ctx *lift.Context) error {
    val, exists := ctx.values["user"]
    if !exists || val == nil {
        return lift.Unauthorized("user not found in context")
    }
    user := val.(*User)
    return ctx.JSON(user)
}

// SOLUTION 2: Use type assertion with ok
func Handler(ctx *lift.Context) error {
    val, exists := ctx.values["user"]
    if !exists {
        return lift.Unauthorized("user not found in context")
    }
    
    user, ok := val.(*User)
    if !ok {
        return lift.Unauthorized("invalid user context")
    }
    return ctx.JSON(user)
}

// SOLUTION 3: Ensure middleware sets value
func AuthMiddleware(ctx *lift.Context) error {
    token := ctx.Header("Authorization")
    if token == "" {
        return lift.Unauthorized("authentication required")
    }
    
    user := validateToken(token)
    ctx.Set("user", user) // Must set before handler
    return ctx.Next()
}
```

### Error: "json: cannot unmarshal string into Go struct field"

**Symptoms:**
- 400 Bad Request on valid-looking JSON
- Validation errors when types seem correct
- "cannot unmarshal" errors

**Root Cause:** JSON types don't match struct field types

**Solution:**
```go
// PROBLEM: Type mismatch
type Request struct {
    Count int    `json:"count"`    // Expects number
    Active bool  `json:"active"`   // Expects boolean
}

// Client sends: {"count": "5", "active": "true"} // Strings!

// SOLUTION 1: Fix client to send correct types
// Client should send: {"count": 5, "active": true}

// SOLUTION 2: Use flexible types
type Request struct {
    Count  json.Number `json:"count"`  // Accepts number or string
    Active string      `json:"active"` // Parse manually
}

func Handler(ctx *lift.Context) error {
    var req Request
    if err := ctx.ParseRequest(&req); err != nil {
        return lift.ValidationError("invalid JSON types")
    }
    
    // Convert types
    count, _ := req.Count.Int64()
    active := req.Active == "true"
}

// SOLUTION 3: Custom unmarshal
type FlexBool bool

func (f *FlexBool) UnmarshalJSON(data []byte) error {
    var b bool
    if err := json.Unmarshal(data, &b); err == nil {
        *f = FlexBool(b)
        return nil
    }
    
    var s string
    if err := json.Unmarshal(data, &s); err == nil {
        *f = FlexBool(s == "true" || s == "1")
        return nil
    }
    
    return errors.New("cannot unmarshal to bool")
}
```

## Routing Issues

### Error: "path parameter not found"

**Symptoms:**
- ctx.Param() returns empty string
- Path parameters not extracted
- Works with hardcoded values but not parameters

**Root Cause:** Parameter name mismatch or incorrect route syntax

**Solution:**
```go
// PROBLEM: Parameter name mismatch
app.GET("/users/:id", func(ctx *lift.Context) error {
    userID := ctx.Param("userId") // Wrong name! Returns ""
    return ctx.JSON(map[string]string{"id": userID})
})

// SOLUTION: Use exact parameter name from route
app.GET("/users/:id", func(ctx *lift.Context) error {
    userID := ctx.Param("id") // Matches :id in route
    return ctx.JSON(map[string]string{"id": userID})
})

// Multiple parameters
app.GET("/orgs/:orgId/users/:userId", func(ctx *lift.Context) error {
    orgID := ctx.Param("orgId")     // Matches :orgId
    userID := ctx.Param("userId")   // Matches :userId
    
    if orgID == "" || userID == "" {
        return lift.ValidationError("missing parameters")
    }
    
    return ctx.JSON(map[string]string{
        "org":  orgID,
        "user": userID,
    })
})
```

### Error: "route already registered"

**Symptoms:**
- Panic during app initialization
- "duplicate route" errors
- App fails to start

**Root Cause:** Registering same path/method combination twice

**Solution:**
```go
// PROBLEM: Duplicate routes
app.GET("/users", GetUsers)
app.GET("/users", ListUsers) // Panic! Already registered

// SOLUTION 1: Use different paths
app.GET("/users", GetUsers)
app.GET("/users/list", ListUsers)

// SOLUTION 2: Use single handler with logic
app.GET("/users", func(ctx *lift.Context) error {
    if ctx.Query("format") == "list" {
        return listUsers(ctx)
    }
    return getUsers(ctx)
})

// SOLUTION 3: Use route groups for organization
v1 := app.Group("/v1")
v1.GET("/users", GetUsersV1)

v2 := app.Group("/v2")
v2.GET("/users", GetUsersV2) // Different prefix, no conflict
```

## Request/Response Problems

### Error: "request body too large"

**Symptoms:**
- 413 Payload Too Large
- Request rejected before handler
- Works with small payloads, fails with large ones

**Root Cause:** Request exceeds max body size limit

**Solution:**
```go
// PROBLEM: Default limit too small
app := lift.New() // Default 10MB limit

// SOLUTION 1: Increase limit using Config
config := &lift.Config{
    MaxRequestSize: 50 * 1024 * 1024, // 50MB
}
app := lift.New(lift.WithConfig(config))

// SOLUTION 2: Stream large files instead
func UploadHandler(ctx *lift.Context) error {
    // For S3, get presigned URL instead
    presignedURL := generatePresignedURL()
    
    return ctx.JSON(map[string]string{
        "upload_url": presignedURL,
        "method": "PUT",
    })
}

// SOLUTION 3: Compress payloads
// Client: gzip compress before sending
// Server: middleware auto-decompresses
```

### Error: "context deadline exceeded"

**Symptoms:**
- Handler times out
- 504 Gateway Timeout
- Partial processing before timeout

**Root Cause:** Handler exceeds Lambda timeout

**Solution:**
```go
// PROBLEM: Long-running operation
func SlowHandler(ctx *lift.Context) error {
    time.Sleep(30 * time.Second) // Lambda times out!
    return ctx.JSON("done")
}

// SOLUTION 1: Make handler timeout-aware
func TimeoutAwareHandler(ctx *lift.Context) error {
    deadline, _ := ctx.Deadline()
    
    for i := 0; i < 1000; i++ {
        select {
        case <-ctx.Done():
            ctx.Logger.Warn("Operation cancelled", "processed", i)
            return lift.GatewayTimeout("operation timed out")
        default:
            // Process one item
            processItem(i)
            
            // Check if close to deadline
            if time.Until(deadline) < 5*time.Second {
                // Save progress and return
                saveProgress(i)
                return ctx.JSON(map[string]interface{}{
                    "processed": i,
                    "continue": true,
                })
            }
        }
    }
    
    return ctx.JSON("completed")
}

// SOLUTION 2: Use async processing
func AsyncHandler(ctx *lift.Context) error {
    jobID := generateJobID()
    
    // Queue for background processing
    err := sqs.SendMessage(&sqs.SendMessageInput{
        QueueUrl: queueURL,
        MessageBody: aws.String(json.Marshal(map[string]string{
            "job_id": jobID,
            "data": ctx.Body(),
        })),
    })
    
    if err != nil {
        return lift.SystemError("failed to enqueue job")
    }
    
    // Return immediately with job ID
    return ctx.Status(202).JSON(map[string]string{
        "job_id": jobID,
        "status": "processing",
    })
}
```

## Middleware Issues

### Error: "middleware executing in wrong order"

**Symptoms:**
- Logger doesn't show request IDs
- Auth runs after business logic
- Error handling doesn't catch errors

**Root Cause:** Middleware registered in wrong order

**Solution:**
```go
// PROBLEM: Wrong order
app.Use(
    middleware.Logger(),      // Needs request ID!
    middleware.RequestID(),   // Too late
    middleware.ErrorHandler(),
    middleware.Recover(),     // Should be before error handler
)

// SOLUTION: Correct order
app.Use(
    middleware.RequestID(),    // 1. Generate ID first
    middleware.Logger(),       // 2. Log with ID
    middleware.Recover(),      // 3. Catch panics
    middleware.ErrorHandler(), // 4. Format errors (including panics)
)

// For auth middleware
api := app.Group("/api")
api.Use(
    middleware.CORS([]string{"*"}),
    middleware.JWTAuth(middleware.JWTConfig{Secret: os.Getenv("JWT_SECRET")}),
    // Add rate limiting middleware as needed (e.g., middleware.UserRateLimitWithLimited)
)

// Visual middleware flow:
// Request -> RequestID -> Logger -> Recover -> ErrorHandler -> Handler
// Response <- ErrorHandler <- Recover <- Logger <- RequestID <- Handler
```

### Error: "middleware not executing"

**Symptoms:**
- Logs missing for certain routes
- Auth bypassed on some endpoints
- Middleware effects not visible

**Root Cause:** Middleware applied incorrectly

**Solution:**
```go
// PROBLEM: Middleware added after routes
app.GET("/users", GetUsers)
app.Use(middleware.Logger()) // Too late! Route already registered

// SOLUTION 1: Add middleware before routes
app.Use(middleware.Logger()) // First
app.GET("/users", GetUsers)  // Then routes

// PROBLEM: Group middleware not inherited
api := app.Group("/api")
api.GET("/public", PublicHandler)
api.Use(middleware.Auth()) // Only affects routes AFTER this line
api.GET("/private", PrivateHandler)

// SOLUTION 2: Add group middleware first
api := app.Group("/api")
api.Use(middleware.Auth()) // First
api.GET("/public", PublicHandler)   // Has auth
api.GET("/private", PrivateHandler) // Has auth

// SOLUTION 3: Different middleware for different groups
// Public routes - no auth
public := app.Group("/public")
public.Use(middleware.RateLimitIP())
public.GET("/status", Status)

// API routes - require auth
api := app.Group("/api")
api.Use(middleware.JWTAuth(middleware.JWTConfig{Secret: os.Getenv("JWT_SECRET")}))
api.Use(middleware.RateLimitUser())
api.GET("/profile", GetProfile)
```

## Authentication Errors

### Error: "unauthorized" on valid token

**Symptoms:**
- JWT validation fails with valid token
- Works in testing, fails in production
- Intermittent auth failures

**Root Cause:** JWT configuration mismatch

**Solution:**
```go
// PROBLEM: Hard-coded secret
api.Use(middleware.JWTAuth(middleware.JWTConfig{
    Secret: "test-secret", // Different in production!
}))

// SOLUTION 1: Use environment variable
api.Use(middleware.JWTAuth(middleware.JWTConfig{
    Secret: os.Getenv("JWT_SECRET"),
    Claims: &CustomClaims{},
}))

// SOLUTION 2: Handle RS256 tokens (public key)
publicKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(os.Getenv("JWT_PUBLIC_KEY")))
if err != nil {
    panic(err)
}

api.Use(middleware.JWTAuth(middleware.JWTConfig{
    PublicKey: publicKey,
    Algorithm: "RS256",
}))

// SOLUTION 3: Debug token issues
api.Use(func(ctx *lift.Context) error {
    auth := ctx.Header("Authorization")
    if auth == "" {
        ctx.Logger.Warn("No Authorization header")
        return lift.Unauthorized("missing token")
    }
    
    if !strings.HasPrefix(auth, "Bearer ") {
        ctx.Logger.Warn("Invalid Authorization format", "header", auth)
        return lift.Unauthorized("invalid token format")
    }
    
    return ctx.Next()
})
```

### Error: "invalid token claims"

**Symptoms:**
- Token validates but claims are empty
- Can't access user ID or tenant ID
- Type assertion failures on claims

**Root Cause:** Claims structure mismatch

**Solution:**
```go
// PROBLEM: Default claims don't match token
api.Use(middleware.JWTAuth(middleware.JWTConfig{
    Secret: secret,
    // Using default jwt.MapClaims
}))

// Token has custom structure:
// {
//   "user_id": "123",
//   "tenant_id": "abc",
//   "roles": ["admin"]
// }

// SOLUTION: Define custom claims
type CustomClaims struct {
    jwt.RegisteredClaims
    UserID   string   `json:"user_id"`
    TenantID string   `json:"tenant_id"`
    Roles    []string `json:"roles"`
}

api.Use(middleware.JWTAuth(middleware.JWTConfig{
    Secret: secret,
    Claims: &CustomClaims{}, // Use custom structure
}))

// Access in handler
func Handler(ctx *lift.Context) error {
    claims := ctx.Get("claims").(*CustomClaims)
    
    // Now properly typed
    userID := claims.UserID
    tenantID := claims.TenantID
    isAdmin := contains(claims.Roles, "admin")
    
    return ctx.JSON(map[string]interface{}{
        "user": userID,
        "tenant": tenantID,
        "admin": isAdmin,
    })
}
```

## Performance Problems

### Error: "cold start too slow"

**Symptoms:**
- First request takes >3 seconds
- Subsequent requests are fast
- High P99 latencies

**Root Cause:** Large binary or initialization overhead

**Solution:**
```go
// PROBLEM: Heavy initialization in handler
func Handler(ctx *lift.Context) error {
    db := initializeDatabase()    // Runs every request!
    cache := connectToRedis()     // Expensive!
    client := createHTTPClient()  // Unnecessary!
    
    // ... use services
}

// SOLUTION 1: Initialize at package level
var (
    db     *Database
    cache  *Redis
    client *http.Client
)

func init() {
    db = initializeDatabase()      // Once at cold start
    cache = connectToRedis()       // Reused across requests
    client = createHTTPClient()    // Shared instance
}

func Handler(ctx *lift.Context) error {
    // Use pre-initialized services
    data := db.Query("SELECT ...")
    return ctx.JSON(data)
}

// SOLUTION 2: Lazy initialization
var (
    dbOnce sync.Once
    db     *Database
)

func getDB() *Database {
    dbOnce.Do(func() {
        db = initializeDatabase()
    })
    return db
}

// SOLUTION 3: Reduce binary size
// go.mod - remove unused dependencies
// Use build tags to exclude debug code
// Build with: -ldflags="-s -w" to strip debug info
```

### Error: "memory limit exceeded"

**Symptoms:**
- Lambda killed with "Runtime exited with error"
- OutOfMemory errors in CloudWatch
- Works locally, fails in Lambda

**Root Cause:** Memory leak or large allocations

**Solution:**
```go
// PROBLEM: Unbounded growth
var cache = make(map[string]interface{})

func Handler(ctx *lift.Context) error {
    // Cache grows forever!
    cache[ctx.RequestID()] = getLargeData()
    return ctx.JSON("ok")
}

// SOLUTION 1: Use bounded cache
import "github.com/hashicorp/golang-lru"

var cache, _ = lru.New(1000) // Max 1000 items

func Handler(ctx *lift.Context) error {
    cache.Add(ctx.RequestID(), getData())
    return ctx.JSON("ok")
}

// SOLUTION 2: Clear data after use
func Handler(ctx *lift.Context) error {
    data := processLargeFile()
    result := analyze(data)
    
    // Clear reference for GC
    data = nil
    
    return ctx.JSON(result)
}

// SOLUTION 3: Stream instead of loading all
func Handler(ctx *lift.Context) error {
    reader := getDataStream()
    defer reader.Close()
    
    scanner := bufio.NewScanner(reader)
    count := 0
    
    for scanner.Scan() {
        processLine(scanner.Text())
        count++
    }
    
    return ctx.JSON(map[string]int{"processed": count})
}
```

## Deployment Issues

### Error: "invalid handler signature"

**Symptoms:**
- Lambda fails to start
- "handler is nil" errors
- Works locally but not in Lambda

**Root Cause:** Wrong handler registration for Lambda

**Solution:**
```go
// PROBLEM: Not starting Lambda runtime
func main() {
    app := lift.New()
    app.GET("/", Handler)
    
    // Missing Lambda start!
    http.ListenAndServe(":8080", app) // Works locally only
}

// SOLUTION: Use Lambda runtime
func main() {
    app := lift.New()
    app.GET("/", Handler)
    
    // For Lambda
    lambda.Start(app.HandleRequest)
    
    // For local testing, use different build
    // go build -tags local
}

// With build tags:
// +build !local

package main

func main() {
    app := createApp()
    lambda.Start(app.HandleRequest)
}

// +build local

package main

func main() {
    app := createApp()
    http.ListenAndServe(":8080", app)
}
```

### Error: "bootstrap not found"

**Symptoms:**
- Lambda fails with "Runtime.InvalidEntrypoint"
- Cannot find handler error
- Deployment succeeds but execution fails

**Root Cause:** Binary named incorrectly for custom runtime

**Solution:**
```bash
# PROBLEM: Wrong binary name
go build -o myapp main.go
zip function.zip myapp

# SOLUTION: Must be named 'bootstrap'
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
zip function.zip bootstrap

# Complete build script:
#!/bin/bash
echo "Building for Lambda..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -ldflags="-s -w" -o bootstrap main.go

echo "Creating deployment package..."
zip -r function.zip bootstrap

echo "Updating Lambda function..."
aws lambda update-function-code \
    --function-name my-function \
    --zip-file fileb://function.zip

echo "Done!"
```

## Testing Problems

### Error: "cannot create test context"

**Symptoms:**
- Test helpers not found
- Type mismatches in tests
- Nil pointer in test context

**Root Cause:** Using wrong test utilities

**Solution:**
```go
// PROBLEM: Creating context manually
func TestHandler(t *testing.T) {
    ctx := &lift.Context{} // Incomplete initialization!
    err := Handler(ctx)    // Nil pointer panic
}

// SOLUTION: Use TestApp pattern
import "github.com/pay-theory/lift/pkg/testing"

func TestHandler(t *testing.T) {
    // Create test app
    app := testing.NewTestApp()
    
    // Configure app with routes
    app.App().POST("/users", CreateUser)
    
    // Make test request with proper headers
    app.WithHeaders(map[string]string{
        "Authorization": "Bearer token",
    })
    
    resp := app.POST("/users", map[string]string{"name": "test"})
    
    // Assert response
    assert.NoError(t, resp.Error)
    assert.Equal(t, 200, resp.GetStatusCode())
}

// Test with app routing and assertions
func TestRouting(t *testing.T) {
    app := testing.NewTestApp()
    
    // Configure routes
    app.App().POST("/users", CreateUser)
    app.App().GET("/users/:id", GetUser)
    
    // Test POST request
    resp := app.POST("/users", map[string]string{"name": "Alice"})
    
    assert.NoError(t, resp.Error)
    assert.Equal(t, 201, resp.GetStatusCode())
    
    // Extract user ID from response
    userID := resp.GetJSONPath("$.id").(string)
    assert.NotEmpty(t, userID)
    
    // Test GET request
    getResp := app.GET("/users/" + userID)
    assert.Equal(t, 200, getResp.GetStatusCode())
    
    name := getResp.GetJSONPath("$.name").(string)
    assert.Equal(t, "Alice", name)
}
```

### Error: "test database conflicts"

**Symptoms:**
- Tests pass individually but fail together
- Data from one test affects another
- Inconsistent test results

**Root Cause:** Shared state between tests

**Solution:**
```go
// PROBLEM: Global state
var db = createTestDB()

func TestCreate(t *testing.T) {
    db.Insert(user) // Affects other tests
}

func TestList(t *testing.T) {
    users := db.List() // Sees data from TestCreate!
}

// SOLUTION 1: Isolated test databases
func TestCreate(t *testing.T) {
    db := createTestDB(t.Name()) // Unique per test
    defer db.Close()
    
    db.Insert(user)
    // Test assertions
}

// SOLUTION 2: Cleanup after each test
func TestWithCleanup(t *testing.T) {
    db := getTestDB()
    
    // Cleanup function
    t.Cleanup(func() {
        db.Exec("DELETE FROM users WHERE email LIKE '%@test.%'")
    })
    
    // Run test
    db.Insert(testUser)
}

// SOLUTION 3: Use transactions
func TestWithTransaction(t *testing.T) {
    tx := db.Begin()
    defer tx.Rollback() // Always rollback
    
    // All operations in transaction
    tx.Insert(user)
    result := tx.Query("SELECT ...")
    
    // Verify results
    assert.Equal(t, expected, result)
    // Rollback prevents permanent changes
}
```

## Quick Debugging Checklist

<!-- AI Training: Systematic debugging approach -->

When something isn't working:

1. **Check Lambda logs** in CloudWatch
   ```bash
   aws logs tail /aws/lambda/your-function --follow
   ```

2. **Add debug middleware**
   ```go
   app.Use(func(ctx *lift.Context) error {
       ctx.Logger.Debug("Request details",
           "method", ctx.Request.Method,
           "path", ctx.Request.Path,
           "headers", ctx.Request.Headers,
       )
       return ctx.Next()
   })
   ```

3. **Verify environment variables**
   ```go
   func init() {
       required := []string{"JWT_SECRET", "DB_URL", "ENVIRONMENT"}
       for _, env := range required {
           if os.Getenv(env) == "" {
               panic(fmt.Sprintf("Missing required env var: %s", env))
           }
       }
   }
   ```

4. **Test locally with event files**
   ```bash
   # Create test event
   cat > event.json << EOF
   {
     "httpMethod": "GET",
     "path": "/health",
     "headers": {}
   }
   EOF
   
   # Test locally
   go run main.go < event.json
   ```

5. **Check AWS permissions**
   - Lambda execution role has CloudWatch Logs access
   - VPC configuration if using RDS/ElastiCache
   - IAM permissions for DynamoDB, S3, etc.

## Modern Lift Patterns

### Error: "HandleTestRequest not working in tests"

**Symptoms:**
- Test requests fail with routing errors
- Handler not found in test environment
- Different behavior between test and production

**Root Cause:** Not using proper test request handling

**Solution:**
```go
// PROBLEM: Direct handler call bypasses routing
func TestHandler(t *testing.T) {
    ctx := lift.NewContext(context.Background(), &lift.Request{...})
    err := MyHandler(ctx) // Bypasses middleware and routing!
}

// SOLUTION: Use HandleTestRequest for proper testing
func TestHandler(t *testing.T) {
    app := lift.New()
    app.Use(middleware.Logger())
    app.POST("/users", CreateUser)
    
    // Create test context
    ctx := lift.NewContext(context.Background(), &lift.Request{
        Method: "POST",
        Path:   "/users",
        Body:   []byte(`{"name": "test"}`),
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
    })
    
    // Use HandleTestRequest to go through full pipeline
    err := app.HandleTestRequest(ctx)
    assert.NoError(t, err)
    assert.Equal(t, 201, ctx.Response.StatusCode)
}
```

### Error: "Error response format inconsistent"

**Symptoms:**
- Different error formats across endpoints
- Client can't parse error responses
- Missing error details in responses

**Root Cause:** Not using structured error responses

**Solution:**
```go
// PROBLEM: Inconsistent error responses
func Handler(ctx *lift.Context) error {
    if someCondition {
        return ctx.Status(400).JSON("Bad request") // String response
    }
    if anotherCondition {
        return ctx.Status(404).JSON(map[string]string{"error": "Not found"}) // Different format
    }
}

// SOLUTION: Use LiftError for consistent responses
func Handler(ctx *lift.Context) error {
    if someCondition {
        return lift.ValidationError("Invalid input data").
            WithDetail("field", "name").
            WithDetail("reason", "required")
    }
    if anotherCondition {
        return lift.NotFound("Resource not found").
            WithDetail("resource_id", ctx.Param("id"))
    }
    
    // System errors with stack traces
    if err := riskyOperation(); err != nil {
        return lift.SystemError("Operation failed").
            WithCause(err).
            WithStackTrace()
    }
    
    return nil
}
```

Remember: Most issues are configuration mismatches between local and Lambda environments!
