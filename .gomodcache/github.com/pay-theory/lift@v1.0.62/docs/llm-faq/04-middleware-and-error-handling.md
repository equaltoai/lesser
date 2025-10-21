# Lift Middleware Options and Error Handling

## Overview

This comprehensive guide covers all middleware options available in Lift, how to use them, how they interact, and how to handle errors effectively in Lift applications. Middleware in Lift provides cross-cutting concerns like logging, authentication, rate limiting, and error handling.

## What is Middleware in Lift?

Middleware in Lift is a function that wraps request handlers to provide additional functionality. Middleware executes before (and optionally after) your handler runs, allowing you to:

- Modify the request before it reaches your handler
- Modify the response after your handler completes
- Short-circuit request processing (e.g., authentication failure)
- Add cross-cutting concerns (logging, metrics, tracing)
- Handle errors consistently

### Middleware Signature

```go
type Middleware func(Handler) Handler
```

Middleware receives a `Handler` and returns a new `Handler` that wraps it.

## Built-in Middleware Options

Lift provides production-ready middleware in the `pkg/middleware` package. Here's a complete catalog:

### 1. RequestID Middleware

**Purpose:** Generates a unique request ID for distributed tracing and log correlation.

**When to use:** Always use this as your first middleware - it enables request tracking across services.

```go
app.Use(middleware.RequestID())
```

**What it does:**
- Generates a unique UUID for each request
- Adds request ID to context
- Includes request ID in all subsequent logs
- Passes request ID in `X-Request-ID` response header

**Best practice:**
```go
// ALWAYS place RequestID first
app.Use(middleware.RequestID())    // 1st: Generate ID
app.Use(middleware.Logger())       // 2nd: Logger uses ID
app.Use(middleware.Recover())      // 3rd: Recovery logs with ID
```

### 2. Logger Middleware

**Purpose:** Provides structured logging for all requests.

**When to use:** Essential for production - use in all applications.

```go
app.Use(middleware.Logger())
```

**What it logs:**
- Request method, path, and query parameters
- Response status code and size
- Request duration
- Error messages (if any)
- Request ID (if RequestID middleware is used)
- User ID and Tenant ID (if available)

**Configuration:**
```go
// Logger uses the app's log level from Config
config := &lift.Config{
    LogLevel: "INFO", // DEBUG, INFO, WARN, ERROR
}
app.WithConfig(config)
```

**Example log output:**
```json
{
  "level": "info",
  "ts": "2025-10-03T19:33:09Z",
  "msg": "Request completed",
  "method": "POST",
  "path": "/api/v1/users",
  "status": 201,
  "duration_ms": 45,
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "user_123",
  "tenant_id": "tenant_abc"
}
```

### 3. Recover Middleware

**Purpose:** Catches panics and converts them to proper HTTP error responses.

**When to use:** Always use this to prevent crashes from taking down your Lambda.

```go
app.Use(middleware.Recover())
```

**What it does:**
- Catches any panic in handlers or downstream middleware
- Logs the panic with stack trace
- Returns a 500 Internal Server Error response
- Prevents Lambda from crashing
- Increments panic metrics (if metrics enabled)

**Example:**
```go
app.Use(middleware.Recover())

app.GET("/panic", func(ctx *lift.Context) error {
    panic("something went wrong") // Caught by Recover middleware
})

// Client receives:
// Status: 500
// Body: {"error": "Internal server error", "request_id": "..."}
```

### 4. JWT Authentication Middleware

**Purpose:** Validates JSON Web Tokens and extracts user claims.

**When to use:** For protected routes that require authentication.

```go
jwtMiddleware := middleware.JWTAuth(middleware.JWTConfig{
    Secret: os.Getenv("JWT_SECRET"),
})

api := app.Group("/api")
api.Use(jwtMiddleware) // All /api/* routes require JWT
```

**Configuration options:**
```go
type JWTConfig struct {
    Secret          string              // Required: JWT signing secret
    SigningMethod   jwt.SigningMethod   // Optional: default is HS256
    ContextKey      string              // Optional: where to store claims
    TokenLookup     string              // Optional: "header:Authorization"
    AuthScheme      string              // Optional: "Bearer"
    Claims          jwt.Claims          // Optional: custom claims type
}
```

**Full example:**
```go
jwtConfig := middleware.JWTConfig{
    Secret:        os.Getenv("JWT_SECRET"),
    SigningMethod: jwt.SigningMethodHS256,
    TokenLookup:   "header:Authorization",
    AuthScheme:    "Bearer",
}

api.Use(middleware.JWTAuth(jwtConfig))

// In your handler, access claims:
app.GET("/profile", func(ctx *lift.Context) error {
    userID := ctx.UserID()      // From JWT claims
    tenantID := ctx.TenantID()  // From JWT claims
    
    // Use the IDs...
    return ctx.JSON(map[string]string{
        "user_id": userID,
        "tenant_id": tenantID,
    })
})
```

**JWT Claims Expected:**
```json
{
  "user_id": "user_123",
  "tenant_id": "tenant_abc",
  "exp": 1696387200,
  "iat": 1696383600
}
```

### 5. Rate Limiting Middleware

**Purpose:** Prevents abuse by limiting request rates per user or IP.

**When to use:** For public APIs or routes that need protection from excessive requests.

#### IP-Based Rate Limiting

```go
// Limit by IP address
ipLimiter, err := middleware.IPRateLimitWithLimited(
    1000,      // 1000 requests
    time.Hour, // per hour
)
if err != nil {
    panic(err)
}
app.Use(ipLimiter)
```

#### User-Based Rate Limiting

```go
// Limit by authenticated user
userLimiter, err := middleware.UserRateLimitWithLimited(
    100,              // 100 requests
    15*time.Minute,   // per 15 minutes
)
if err != nil {
    panic(err)
}

// Apply to authenticated routes
api := app.Group("/api/v1")
api.Use(jwtMiddleware)      // Authenticate first
api.Use(userLimiter)        // Then rate limit by user
```

#### Tenant-Based Rate Limiting

```go
// Limit by tenant (multi-tenant apps)
tenantLimiter, err := middleware.TenantRateLimitWithLimited(
    10000,     // 10,000 requests
    time.Hour, // per hour per tenant
)
if err != nil {
    panic(err)
}

api.Use(tenantLimiter)
```

**What happens when limit exceeded:**
- Returns HTTP 429 (Too Many Requests)
- Includes `Retry-After` header
- Logs rate limit violation
- Increments rate limit metrics

**Response example:**
```json
{
  "error": "rate limit exceeded",
  "retry_after": 300
}
```

### 6. CORS Middleware

**Purpose:** Handles Cross-Origin Resource Sharing for web applications.

**When to use:** When your API is called from web browsers.

```go
corsConfig := middleware.CORSConfig{
    AllowOrigins:     []string{"https://example.com", "https://app.example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    ExposeHeaders:    []string{"X-Request-ID"},
    AllowCredentials: true,
    MaxAge:           3600, // Cache preflight for 1 hour
}

app.Use(middleware.CORS(corsConfig))
```

**Simple CORS (allow all):**
```go
// Use config-based CORS
config := &lift.Config{
    CORSEnabled:    true,
    AllowedOrigins: []string{"*"}, // Not recommended for production
}
app.WithConfig(config)
```

### 7. Enhanced Observability Middleware

**Purpose:** Unified logging, metrics, and tracing with sampling control.

**When to use:** Production applications that need comprehensive observability.

```go
app.Use(middleware.EnhancedObservabilityMiddleware(
    middleware.EnhancedObservabilityConfig{
        EnableLogging:   true,
        EnableMetrics:   true,
        EnableTracing:   true,
        SampleRate:      0.1,  // 10% of requests
        DefaultTags: map[string]string{
            "service":     "my-app",
            "environment": "production",
        },
        TenantIDFunc: func(ctx *lift.Context) string {
            return ctx.TenantID()
        },
        UserIDFunc: func(ctx *lift.Context) string {
            return ctx.UserID()
        },
    },
))
```

**What it provides:**
- **Logging:** Structured logs with request/response details
- **Metrics:** CloudWatch metrics for duration, errors, status codes
- **Tracing:** AWS X-Ray segments for distributed tracing
- **Sampling:** Control observability overhead
- **Correlation:** Automatic tenant/user tagging

**Metrics emitted:**
- `request.duration` - Request duration in milliseconds
- `request.count` - Total request count
- `request.errors` - Error count
- `request.status.XXX` - Count by status code

### 8. Load Shedding Middleware

**Purpose:** Protects your Lambda from overload by rejecting requests when under stress.

**When to use:** High-traffic applications that need graceful degradation.

```go
// Configure load shedding
loadConfig := middleware.ConfigureLoadSheddingForApp(
    app,
    middleware.NewBasicLoadShedding("my-app"),
)

app.Use(middleware.LoadSheddingMiddleware(loadConfig))
```

**What it does:**
- Monitors system load (memory, CPU, request duration)
- Rejects requests when thresholds exceeded
- Returns 503 Service Unavailable
- Prevents cascade failures
- Auto-recovers when load decreases

**Advanced configuration:**
```go
config := middleware.LoadSheddingConfig{
    MaxConcurrentRequests: 100,
    MaxMemoryUsage:        0.8,  // 80%
    MaxCPUUsage:           0.9,  // 90%
    AdaptiveThreshold:     true,
}

loadConfig := middleware.NewLoadShedding("my-app", config)
```

### 9. Retry Middleware

**Purpose:** Automatically retries failed requests with backoff.

**When to use:** For idempotent operations that may fail transiently.

```go
retryConfig := middleware.RetryConfig{
    MaxRetries:     3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff:     2 * time.Second,
    BackoffFactor:  2.0,
    RetryOn: []int{
        500, // Internal Server Error
        502, // Bad Gateway
        503, // Service Unavailable
        504, // Gateway Timeout
    },
}

app.Use(middleware.Retry(retryConfig))
```

**Retry behavior:**
- Exponential backoff (100ms, 200ms, 400ms...)
- Only retries configured status codes
- Logs each retry attempt
- Gives up after MaxRetries

### 10. Circuit Breaker Middleware

**Purpose:** Prevents cascading failures by "opening" the circuit when errors exceed threshold.

**When to use:** When calling external services that might fail.

```go
cbConfig := middleware.CircuitBreakerConfig{
    Threshold:   5,                  // Open after 5 failures
    Timeout:     30 * time.Second,   // Stay open for 30s
    MaxRequests: 2,                  // Allow 2 requests when half-open
}

app.Use(middleware.CircuitBreaker(cbConfig))
```

**Circuit states:**
- **Closed:** Normal operation, requests pass through
- **Open:** Too many failures, reject requests immediately (fast fail)
- **Half-Open:** Testing if service recovered, allow limited requests

### 11. Idempotency Middleware

**Purpose:** Ensures the same request isn't processed multiple times.

**When to use:** For payment processing, order creation, or any non-idempotent operation.

```go
idempotencyConfig := middleware.IdempotencyConfig{
    KeyHeader:    "Idempotency-Key",
    TTL:          24 * time.Hour,
    TableName:    os.Getenv("IDEMPOTENCY_TABLE"),
}

app.Use(middleware.Idempotency(idempotencyConfig))
```

**How it works:**
- Client provides `Idempotency-Key` header
- First request: Processes and stores result
- Duplicate requests: Returns cached result
- Keys expire after TTL

**Usage:**
```bash
curl -X POST https://api.example.com/payments \
  -H "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000" \
  -d '{"amount": 100}'
```

### 12. Security Headers Middleware

**Purpose:** Adds security headers to all responses.

**When to use:** Always, for production applications.

```go
app.Use(middleware.SecurityHeaders())
```

**Headers added:**
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security: max-age=31536000`
- `Content-Security-Policy: default-src 'self'`

## Error Handling in Lift

Lift provides structured error handling with predefined error types and automatic formatting.

### Built-in Error Types

#### 1. Validation Error (422)

**When to use:** Request data is invalid or fails validation.

```go
func CreateUser(ctx *lift.Context) error {
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        // ParseRequest automatically returns ValidationError
        return err
    }
    
    // Manual validation
    if req.Age < 18 {
        return lift.ValidationError("user must be 18 or older")
    }
    
    // Continue processing...
}
```

**Response:**
```json
{
  "error": "validation error",
  "message": "user must be 18 or older",
  "status": 422
}
```

#### 2. Unauthorized (401)

**When to use:** Authentication is required but not provided.

```go
func SecureEndpoint(ctx *lift.Context) error {
    token := ctx.Header("Authorization")
    if token == "" {
        return lift.Unauthorized("authentication required")
    }
    
    // Continue...
}
```

#### 3. Authorization Error (403)

**When to use:** User is authenticated but lacks permission.

```go
func AdminOnly(ctx *lift.Context) error {
    if !isAdmin(ctx.UserID()) {
        return lift.AuthorizationError("admin access required")
    }
    
    // Continue...
}
```

#### 4. Not Found (404)

**When to use:** Requested resource doesn't exist.

```go
func GetUser(ctx *lift.Context) error {
    userID := ctx.Param("id")
    
    user, err := db.GetUser(userID)
    if err == db.ErrNotFound {
        return lift.NotFound("user not found")
    }
    
    return ctx.JSON(user)
}
```

#### 5. System Error (500)

**When to use:** Internal server errors, database failures, etc.

```go
func CreateOrder(ctx *lift.Context) error {
    order, err := processOrder(ctx)
    if err != nil {
        ctx.Logger.Error("Failed to process order", "error", err)
        return lift.SystemError("failed to create order").WithCause(err)
    }
    
    return ctx.JSON(order)
}
```

#### 6. Custom Errors

**When to use:** Application-specific error conditions.

```go
func ProcessPayment(ctx *lift.Context) error {
    err := chargeCard(ctx)
    if err != nil {
        return lift.NewLiftError(
            "PAYMENT_FAILED",
            "Unable to process payment",
            402, // Payment Required
        )
    }
    
    return ctx.JSON(response)
}
```

**Response:**
```json
{
  "error": "PAYMENT_FAILED",
  "message": "Unable to process payment",
  "status": 402
}
```

### Error Handling Best Practices

#### 1. Log Errors with Context

```go
func Handler(ctx *lift.Context) error {
    user, err := getUser(ctx.Param("id"))
    if err != nil {
        ctx.Logger.Error("Failed to get user",
            "error", err,
            "user_id", ctx.Param("id"),
            "tenant_id", ctx.TenantID(),
        )
        return lift.SystemError("failed to retrieve user")
    }
    return ctx.JSON(user)
}
```

#### 2. Don't Leak Internal Details

```go
// ❌ BAD: Exposes internal details
func BadHandler(ctx *lift.Context) error {
    err := db.Query("SELECT * FROM users...")
    if err != nil {
        return fmt.Errorf("database error: %v", err) // Exposes SQL
    }
}

// ✅ GOOD: Generic message to client
func GoodHandler(ctx *lift.Context) error {
    err := db.Query("SELECT * FROM users...")
    if err != nil {
        ctx.Logger.Error("Database query failed", "error", err)
        return lift.SystemError("failed to retrieve data")
    }
}
```

#### 3. Use Appropriate Status Codes

```go
func Handler(ctx *lift.Context) error {
    // 400: Client sent bad data
    if invalidInput {
        return lift.ValidationError("invalid input")
    }
    
    // 401: No authentication
    if !authenticated {
        return lift.Unauthorized("authentication required")
    }
    
    // 403: Authenticated but not authorized
    if !authorized {
        return lift.AuthorizationError("insufficient permissions")
    }
    
    // 404: Resource not found
    if !exists {
        return lift.NotFound("resource not found")
    }
    
    // 500: Server error
    if systemError {
        return lift.SystemError("internal error")
    }
}
```

## Middleware Order and Composition

The order of middleware matters! Here's the recommended order:

```go
func main() {
    app := lift.New()
    
    // 1. Request ID - Must be first for tracing
    app.Use(middleware.RequestID())
    
    // 2. Recovery - Catch panics early
    app.Use(middleware.Recover())
    
    // 3. Logger - Log all requests (uses Request ID)
    app.Use(middleware.Logger())
    
    // 4. Security headers - Apply to all responses
    app.Use(middleware.SecurityHeaders())
    
    // 5. CORS - Handle preflight requests early
    app.Use(middleware.CORS(corsConfig))
    
    // 6. Load shedding - Reject when overloaded
    app.Use(middleware.LoadSheddingMiddleware(loadConfig))
    
    // 7. Observability - Comprehensive monitoring
    app.Use(middleware.EnhancedObservabilityMiddleware(obsConfig))
    
    // 8. Route-specific middleware (auth, rate limiting) applied to groups
    api := app.Group("/api/v1")
    api.Use(middleware.JWTAuth(jwtConfig))
    api.Use(userRateLimiter)
    
    // 9. Handlers execute last
    api.POST("/orders", CreateOrder)
}
```

## Summary

### Essential Middleware for Production
1. **RequestID** - Always first
2. **Logger** - Always second
3. **Recover** - Always third
4. **SecurityHeaders** - Security basics
5. **EnhancedObservability** - Comprehensive monitoring

### Middleware for Specific Needs
- **JWTAuth** - Protected routes
- **Rate limiting** - Prevent abuse
- **CORS** - Web browser support
- **Load shedding** - Overload protection
- **Idempotency** - Duplicate prevention
- **Circuit breaker** - Failure isolation

### Error Handling Principles
- Use appropriate status codes
- Log detailed errors server-side
- Return generic messages to clients
- Include request IDs for debugging
- Handle errors consistently across handlers

With Lift's middleware and error handling, you can build robust, production-ready Lambda functions with minimal boilerplate code.



