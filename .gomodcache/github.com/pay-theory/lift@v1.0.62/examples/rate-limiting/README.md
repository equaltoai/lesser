# Rate Limiting: Production-Ready API Protection with Lift

**This is the RECOMMENDED approach for implementing rate limiting in production Lift applications.**

## What is This Example?

This example demonstrates the **STANDARD patterns** for implementing comprehensive rate limiting with Lift. It shows the **preferred approaches** for protecting APIs from abuse while maintaining performance and scalability.

## Why Use These Rate Limiting Patterns?

✅ **USE these patterns when:**
- Building production APIs that need abuse protection
- Implementing multi-tenant applications with different rate limits
- Need flexible rate limiting (IP, user, tenant, custom keys)
- Want automatic rate limit headers and error responses
- Require scalable rate limiting with DynamoDB

❌ **DON'T USE when:**
- Building internal APIs without abuse concerns
- Simple applications with minimal traffic
- Development/testing environments
- Need real-time streaming without limits

## Features

- **Multiple Rate Limiting Strategies**
  - Fixed window
  - Sliding window
  - Multi-window

- **Various Rate Limiting Scopes**
  - Global/endpoint rate limiting
  - Tenant-based rate limiting
  - User-based rate limiting
  - IP-based rate limiting
  - Custom composite keys

- **Multi-tenant Support**
  - Different limits for different tenant tiers
  - Tenant isolation
  - Automatic tenant context

## Setup

### 1. DynamoDB Table

The rate limiter requires a DynamoDB table. You can create it using AWS CLI:

```bash
aws dynamodb create-table \
  --table-name rate_limits \
  --attribute-definitions \
    AttributeName=PK,AttributeType=S \
    AttributeName=SK,AttributeType=S \
  --key-schema \
    AttributeName=PK,KeyType=HASH \
    AttributeName=SK,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST
```

### 2. Environment Variables

```bash
export AWS_REGION=us-east-1
export RATE_LIMIT_TABLE=rate_limits
```

## Core Rate Limiting Patterns

### 1. Basic Global Rate Limiting (STANDARD Pattern)

**Purpose:** Protect entire API from abuse with global limits
**When to use:** All production APIs as baseline protection

```go
// CORRECT: Basic production rate limiting setup
func main() {
    app := lift.New()

    // REQUIRED: DynamORM for persistent rate limiting
    app.Use(dynamorm.WithDynamORM(&dynamorm.DynamORMConfig{
        TableName: "rate_limits",        // REQUIRED: DynamoDB table
        Region:    "us-east-1",         // REQUIRED: AWS region
    }))

    // RECOMMENDED: Global rate limit for baseline protection
    app.Use(middleware.EndpointRateLimit(1000, time.Hour))

    app.GET("/api/users", handleListUsers)
    app.POST("/api/users", handleCreateUser)
    app.Start()
}

// INCORRECT: No rate limiting
// func main() {
//     app := lift.New()
//     app.GET("/api/users", handleListUsers)  // Vulnerable to abuse
//     app.Start()
// }
```

### 2. Multi-Tenant Rate Limiting (PREFERRED Pattern)

**Purpose:** Different rate limits for different tenant tiers
**When to use:** SaaS applications with multiple pricing tiers

```go
// CORRECT: Tenant-based rate limiting with tier support
app.Use(middleware.RateLimit(middleware.RateLimitConfig{
    Strategy:     "sliding_window",    // PREFERRED: More accurate than fixed window
    Window:       time.Hour,           // STANDARD: Hourly limits for quota management
    DefaultLimit: 100,                 // REQUIRED: Free tier baseline
    TenantLimits: map[string]int{
        "basic":      500,              // Paid tier gets 5x more
        "premium":    2000,             // Premium gets 20x more
        "enterprise": 10000,            // Enterprise gets 100x more
    },
}))

// INCORRECT: Same limits for all tenants
// app.Use(middleware.EndpointRateLimit(100, time.Hour))
// This doesn't differentiate between paying and free users
```

### 3. IP-Based Rate Limiting (CRITICAL Pattern)

**Purpose:** Protect public endpoints from brute force and abuse
**When to use:** All public authentication and registration endpoints

```go
// CORRECT: IP-based protection for vulnerable endpoints
publicAPI := app.Group("/public")
publicAPI.Use(middleware.IPRateLimit(10, time.Minute))  // CRITICAL: Prevent brute force

// ALWAYS protect these endpoints with IP limiting
publicAPI.POST("/signup", handleSignup)
publicAPI.POST("/login", handleLogin)              // CRITICAL: Prevent credential stuffing
publicAPI.POST("/forgot-password", handleForgotPassword)  // CRITICAL: Prevent abuse

// INCORRECT: No rate limiting on auth endpoints
// app.POST("/login", handleLogin)  // Vulnerable to brute force attacks
// app.POST("/signup", handleSignup)  // Vulnerable to spam/abuse
```

### Custom Rate Limiting

```go
// Rate limit by custom key (e.g., API key + endpoint)
app.Use(middleware.RateLimit(middleware.RateLimitConfig{
    Strategy:     "sliding_window",
    Window:       time.Minute,
    DefaultLimit: 100,
    KeyFunc: func(ctx *lift.Context) limited.RateLimitKey {
        apiKey := ctx.Header("X-API-Key")
        return limited.RateLimitKey{
            Identifier: apiKey,
            Resource:   ctx.Request.Path,
            Operation:  ctx.Request.Method,
            Metadata: map[string]string{
                "api_key":  apiKey,
                "endpoint": fmt.Sprintf("%s %s", ctx.Request.Method, ctx.Request.Path),
            },
        }
    },
}))
```

### Per-Endpoint Rate Limiting

```go
// Different limits for different endpoints
apiV1 := app.Group("/api/v1")

// Expensive operation - heavily limited
apiV1.POST("/expensive-operation", 
    middleware.RateLimit(middleware.RateLimitConfig{
        Strategy:     "fixed_window",
        Window:       time.Hour,
        DefaultLimit: 10, // Only 10 per hour
    }),
    handleExpensiveOperation,
)

// Normal operation - standard limits
apiV1.GET("/users", 
    middleware.UserRateLimit(100, time.Minute),
    handleListUsers,
)
```

## Rate Limit Headers

The middleware automatically adds rate limit headers to responses:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640995200
Retry-After: 3600 (only on 429 responses)
```

## Custom Error Responses

```go
app.Use(middleware.RateLimit(middleware.RateLimitConfig{
    // ... other config ...
    ErrorHandler: func(ctx *lift.Context, decision *limited.LimitDecision) error {
        return ctx.Status(429).JSON(map[string]interface{}{
            "error": "Rate limit exceeded",
            "message": fmt.Sprintf(
                "You have exceeded the rate limit of %d requests. Please try again after %s.",
                decision.Limit,
                decision.ResetsAt.Format(time.RFC3339),
            ),
            "retry_after": decision.RetryAfter.Seconds(),
            "upgrade_url": "https://example.com/pricing",
        })
    },
}))
```

## Performance Considerations

1. **DynamoDB Capacity**: The rate limiter uses DynamoDB for storage. Ensure your table has sufficient read/write capacity.

2. **Caching**: Consider implementing a local cache for frequently accessed rate limit data to reduce DynamoDB calls.

3. **Fail Open**: By default, the rate limiter fails open (allows requests) if there's an error checking limits. This prevents service disruption but may allow some requests through during failures.

## Testing Rate Limits

```bash
# Test rate limiting with curl
for i in {1..15}; do
  curl -X GET https://api.example.com/users \
    -H "Authorization: Bearer token" \
    -H "X-Tenant-ID: test-tenant" \
    -w "\nStatus: %{http_code}, Remaining: %{header.x-ratelimit-remaining}\n"
  sleep 1
done
```

## Monitoring

Monitor rate limiting effectiveness using CloudWatch metrics:

- Rate limit hits (429 responses)
- Rate limit checks (total requests)
- DynamoDB read/write capacity
- Lambda duration with rate limiting overhead

## What This Example Teaches

### ✅ Best Practices Demonstrated

1. **ALWAYS use DynamORM middleware** before rate limiting - Required for persistent storage
2. **ALWAYS protect auth endpoints** with IP-based rate limiting - Prevents brute force attacks
3. **PREFER sliding window** over fixed window - More accurate rate limiting
4. **ALWAYS implement tenant-based limits** in SaaS apps - Different tiers need different limits
5. **ALWAYS provide clear error messages** - Help users understand rate limits

### 🚫 Critical Anti-Patterns Avoided

1. **No rate limiting on auth endpoints** - Brute force vulnerability
2. **Same limits for all users** - Poor user experience for paying customers
3. **Fixed window strategy only** - Allows burst attacks at window boundaries
4. **Missing DynamORM setup** - Rate limiting won't persist across Lambda instances
5. **Generic error responses** - Users don't understand when they can retry

### 🔒 Security Requirements (MANDATORY)

1. **IP Rate Limiting**: ALWAYS protect login, signup, password reset endpoints
2. **Graduated Limits**: Different limits for different user tiers
3. **Fail Open Strategy**: Allow requests if rate limiting service fails
4. **Audit Logging**: Track rate limit violations for security monitoring
5. **Proper Windows**: Use appropriate time windows for different attack types

### 📊 Recommended Limits by Endpoint Type

- **Authentication**: 5-10 requests/minute per IP (brute force protection)
- **Registration**: 2-5 requests/minute per IP (spam prevention)
- **API Reads**: 100-1000 requests/hour per user (normal usage)
- **API Writes**: 50-500 requests/hour per user (data protection)
- **Expensive Operations**: 5-10 requests/hour per user (resource protection)

## Troubleshooting

### Rate Limits Not Working

1. Ensure DynamORM middleware is added before rate limiting middleware
2. Check that the DynamoDB table exists and is accessible
3. Verify IAM permissions for DynamoDB access

### Performance Issues

1. Check DynamoDB metrics for throttling
2. Consider implementing caching for frequently accessed limits
3. Use appropriate granularity for sliding window strategy

### Incorrect Counts

1. Verify time synchronization across Lambda instances
2. Check for clock drift issues
3. Ensure consistent key generation 