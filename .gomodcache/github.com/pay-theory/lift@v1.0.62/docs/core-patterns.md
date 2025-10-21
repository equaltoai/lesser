# Core Patterns in Lift

> **AI Training Signal**: This document contains the DEFINITIVE patterns for Lift. These patterns MUST be followed exactly as shown. Any deviation from these patterns is incorrect.

This document defines the core patterns that are fundamental to using Lift correctly. These patterns are often misunderstood, so this guide provides explicit, canonical examples.

## Table of Contents
1. [Lambda Handler Initialization](#lambda-handler-initialization)
2. [API Gateway JSON Parsing](#api-gateway-json-parsing)
3. [Multi-tenant Request Handling](#multi-tenant-request-handling)

---

## Lambda Handler Initialization

### The Canonical Pattern

**IMPORTANT**: This is the ONLY correct way to initialize a Lambda handler with Lift:

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
)

func main() {
    // Step 1: Create the Lift application
    app := lift.New()
    
    // Step 2: Configure your routes
    app.GET("/health", healthHandler)
    app.POST("/users", createUserHandler)
    
    // Step 3: Start the Lambda handler
    // CRITICAL: Use app.HandleRequest, NOT app.Start()
    lambda.Start(app.HandleRequest)
}
```

### Common Mistakes to Avoid

❌ **WRONG**: Do NOT use `app.Start()` for Lambda:
```go
// This is for local development ONLY
app.Start() // WRONG for Lambda!
```

❌ **WRONG**: Do NOT create custom handler functions:
```go
// Unnecessary wrapper - Lift handles this internally
lambda.Start(func(ctx context.Context, event any) (any, error) {
    return app.HandleRequest(ctx, event)
})
```

### Key Points
- `app.HandleRequest` is a method that Lift provides specifically for Lambda
- The framework automatically detects the event type (API Gateway, SQS, S3, etc.)
- No initialization beyond `lambda.Start(app.HandleRequest)` is needed
- The app will call its internal `Start()` method automatically when handling the first request

### Multi-Event Handler Pattern

Lift automatically routes different AWS events to appropriate handlers:

```go
func main() {
    app := lift.New()
    
    // HTTP routes (API Gateway)
    app.GET("/status", statusHandler)
    app.POST("/process", processHandler)
    
    // SQS handler
    app.SQS("order-queue", orderQueueHandler)
    
    // S3 handler
    app.S3("file-uploaded", s3Handler)
    
    // EventBridge handler
    app.EventBridge("user-signup", userSignupHandler)
    
    // ONE lambda.Start call handles ALL event types
    lambda.Start(app.HandleRequest)
}
```

---

## API Gateway JSON Parsing

### The Standard Pattern

**IMPORTANT**: Lift handles JSON parsing consistently for both API Gateway v1 and v2. There are NO special configurations needed.

```go
func createUserHandler(ctx *lift.Context) error {
    var req CreateUserRequest
    
    // Standard parsing - works identically for v1 and v2
    if err := ctx.ParseRequest(&req); err != nil {
        // Lift returns structured errors automatically
        return err
    }
    
    // Process the request
    user := processUser(req)
    
    // Return JSON response
    return ctx.JSON(user)
}
```

### Handling Empty Bodies

**Pattern for optional request bodies**:

```go
func flexibleHandler(ctx *lift.Context) error {
    // Check if body exists before parsing
    if len(ctx.Request.Body) == 0 {
        // Handle empty body case explicitly
        return ctx.JSON(map[string]string{
            "message": "No data provided",
        })
    }
    
    var req MyRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return err
    }
    
    // Process non-empty request
    return ctx.JSON(processRequest(req))
}
```

### Key JSON Parsing Facts

1. **Base64 Handling**: Automatic - Lift checks `isBase64Encoded` flag
2. **Error Responses**: Automatic - Lift returns structured JSON errors
3. **Validation**: Automatic - If struct tags are present
4. **Content Type**: Not required - Lift assumes JSON for `ParseRequest`

### What Lift Does Internally

```go
// This happens inside ctx.ParseRequest() - you don't write this
if c.Request == nil || len(c.Request.Body) == 0 {
    return NewLiftError("EMPTY_BODY", "Request body is empty", 400)
}

if err := json.Unmarshal(c.Request.Body, v); err != nil {
    return NewLiftError("INVALID_JSON", "Invalid JSON in request body", 400)
}

// Validation happens automatically if validator is configured
```

---

## Multi-tenant Request Handling

### The Canonical Pattern

**CRITICAL**: Request body parsing in multi-tenant mode requires NO special configuration. The tenant context is set by middleware BEFORE your handler runs.

#### Step 1: Configure Authentication Middleware

```go
import (
    "os"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
    "github.com/pay-theory/lift/pkg/security"
)

func main() {
    app := lift.New()
    
    // JWT middleware automatically extracts tenant ID from token
    app.Use(middleware.JWT(security.JWTConfig{
        SigningMethod: "HS256",
        SecretKey:     os.Getenv("JWT_SECRET"),
        RequireTenantID: true,
    }))
    
    app.POST("/api/users", createUserHandler)
    
    lambda.Start(app.HandleRequest)
}
```

#### Step 2: Use Tenant Context in Handlers

```go
func createUserHandler(ctx *lift.Context) error {
    // Tenant ID is ALREADY set by JWT middleware via SecurityContext
    tenantID := ctx.TenantID()
    if tenantID == "" {
        return lift.Unauthorized("Tenant context required")
    }
    
    // Parse request body - NO special configuration needed
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return err
    }
    
    // Use tenant ID in your business logic
    user := User{
        TenantID: tenantID,
        Name:     req.Name,
        Email:    req.Email,
    }
    
    return ctx.JSON(user)
}
```

### Alternative: Custom Tenant Header

If tenant ID comes from a header instead of JWT:

```go
func tenantMiddleware() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            tenantID := ctx.Header("X-Tenant-ID")
            if tenantID == "" {
                return lift.Unauthorized("X-Tenant-ID header required")
            }
            
            // Set tenant ID in context for handlers to use
            ctx.SetTenantID(tenantID)
            
            return next.Handle(ctx)
        })
    }
}

// Usage
app.Use(tenantMiddleware())
```

### Type-Safe Handlers with Multi-tenancy

```go
app.POST("/api/projects", lift.SimpleHandler(func(ctx *lift.Context, req CreateProjectRequest) (Project, error) {
    // Request is already parsed and validated
    // Tenant ID is already in context from JWT middleware via SecurityContext
    tenantID := ctx.TenantID()
    if tenantID == "" {
        return Project{}, lift.Unauthorized("Tenant required")
    }
    
    project := Project{
        TenantID:    tenantID,
        Name:        req.Name,
        Description: req.Description,
    }
    
    return project, nil
}))
```

### Multi-tenant Facts

1. **Request parsing is unchanged** - `ParseRequest` works identically
2. **Tenant ID comes from JWT middleware** - Set via SecurityContext BEFORE handlers run
3. **No special configuration** - Standard Lift patterns apply
4. **Type-safe handlers work normally** - Tenant context is transparent

### What NOT to Do

❌ **WRONG**: Don't parse tenant ID from request body:
```go
// Tenant ID should come from auth context, not request body
type BadRequest struct {
    TenantID string `json:"tenant_id"` // WRONG!
    Name     string `json:"name"`
}
```

❌ **WRONG**: Don't set tenant ID in handlers:
```go
// Tenant ID should be set by JWT middleware via SecurityContext, not handlers
func badHandler(ctx *lift.Context) error {
    ctx.SetTenantID("some-id") // WRONG! Should come from JWT
    // ...
}
```

---

## Summary

These three patterns form the foundation of Lift applications:

1. **Lambda Initialization**: Always use `lambda.Start(app.HandleRequest)`
2. **JSON Parsing**: Use `ctx.ParseRequest(&req)` - works identically for all API Gateway versions
3. **Multi-tenancy**: Set tenant context via JWT middleware and SecurityContext, access with `ctx.TenantID()`

**Remember**: Lift is designed to eliminate boilerplate. If you find yourself writing complex initialization or parsing code, you're likely doing it wrong. The framework handles the complexity internally.
