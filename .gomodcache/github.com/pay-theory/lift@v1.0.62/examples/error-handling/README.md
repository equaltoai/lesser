# Error Handling: Robust Error Management with Lift

**This is the RECOMMENDED approach for handling errors in Lift applications.**

## What is This Example?

This example demonstrates the **STANDARD patterns** for error handling in Lift applications. It shows the **preferred practices** for graceful error handling, application startup validation, and event handler error management.

## Why Use These Error Patterns?

✅ **USE these patterns when:**
- Building production Lift applications
- Need graceful error handling and recovery
- Want consistent error responses across handlers
- Require validation during application startup
- Building event-driven architectures

❌ **DON'T USE when:**
- Building development/testing utilities only
- Error handling requirements are minimal
- Single-use scripts or simple tools

## Quick Start

```go
// This is the CORRECT way to handle errors in Lift
package main

import (
    "log"
    "github.com/pay-theory/lift/pkg/lift"
)

func main() {
    app := lift.New()
    
    // PREFERRED: Always check registration errors
    if err := app.GET("/health", healthHandler); err != nil {
        log.Fatalf("Failed to register health endpoint: %v", err)
    }
    
    // REQUIRED: Always check startup errors
    if err := app.Start(); err != nil {
        log.Fatalf("Failed to start application: %v", err)
    }
}

// INCORRECT: Ignoring errors leads to silent failures
// app.GET("/health", healthHandler)  // No error checking
// app.Start()                        // No error checking
```

## Core Error Handling Patterns

### 1. Route Registration Error Handling (REQUIRED Pattern)

**Purpose:** Validate handler registration during application startup
**When to use:** All route and event handler registrations

```go
// CORRECT: Check all registration errors
if err := app.GET("/users", getUserHandler); err != nil {
    log.Fatalf("Failed to register GET /users: %v", err)
}

if err := app.POST("/users", createUserHandler); err != nil {
    log.Fatalf("Failed to register POST /users: %v", err)
}

// This catches issues like:
// - Invalid handler types
// - Duplicate routes
// - Configuration errors

// INCORRECT: Silent registration failures
// app.GET("/users", getUserHandler)   // Might fail silently
// app.POST("/users", createUserHandler)
```

### 2. Invalid Handler Detection (STANDARD Pattern)

**Purpose:** Demonstrate how Lift validates handler types
**When to use:** Understanding Lift's type safety

```go
// CORRECT: This will return an error (and should)
if err := app.POST("/invalid", "this is not a valid handler"); err != nil {
    log.Printf("Expected error: %v", err)
    // Lift correctly rejects invalid handlers
}

// Valid handler types that Lift accepts:
// - func(*lift.Context) error
// - lift.SimpleHandler functions
// - lift.Handler interface implementations

// INCORRECT: Not checking for invalid handlers
// app.POST("/invalid", "string")  // Silent failure, runtime issues
```

### 3. Event Handler Error Management (PREFERRED Pattern)

**Purpose:** Robust error handling for AWS event sources
**When to use:** All SQS, EventBridge, S3, and other event handlers

```go
// CORRECT: Event handler registration with error checking
if err := app.SQS("my-queue", func(ctx *lift.Context) error {
    log.Println("Processing SQS message")
    
    // Event handlers should return errors for failed processing
    // Lift will handle retry logic and dead letter queues
    return nil
}); err != nil {
    log.Fatalf("Failed to register SQS handler: %v", err)
}

if err := app.EventBridge("custom.event", func(ctx *lift.Context) error {
    log.Println("Processing EventBridge event")
    return nil
}); err != nil {
    log.Fatalf("Failed to register EventBridge handler: %v", err)
}

// INCORRECT: No error handling for event sources
// app.SQS("my-queue", sqsHandler)        // Might fail silently
// app.EventBridge("custom.event", ebHandler)
```

### 4. Application Startup Validation (CRITICAL Pattern)

**Purpose:** Ensure application is properly configured before serving traffic
**When to use:** All production Lift applications

```go
// CORRECT: Always validate startup
if err := app.Start(); err != nil {
    log.Fatalf("Failed to start application: %v", err)
    // This catches:
    // - Configuration errors
    // - Missing environment variables
    // - AWS permission issues
    // - Invalid route configurations
}

log.Println("Application started successfully")

// INCORRECT: Starting without validation
// app.Start()  // No error checking - silent failures
```

## Handler-Level Error Patterns

### 5. Business Logic Error Handling (STANDARD Pattern)

```go
// CORRECT: Let Lift handle error responses
app.GET("/users/:id", func(ctx *lift.Context) error {
    userID := ctx.Param("id")
    
    user, err := getUserFromDB(userID)
    if err != nil {
        // Return error directly - Lift handles the response
        return fmt.Errorf("user not found: %w", err)
    }
    
    return ctx.JSON(user)
})

// INCORRECT: Manual error response construction
// app.GET("/users/:id", func(ctx *lift.Context) error {
//     userID := ctx.Param("id")
//     user, err := getUserFromDB(userID)
//     if err != nil {
//         ctx.Status(404)  // Manual status setting
//         return ctx.JSON(map[string]string{"error": "user not found"})
//     }
//     return ctx.JSON(user)
// })
```

### 6. Structured Error Responses (PREFERRED Pattern)

```go
// CORRECT: Use Lift's structured errors
app.POST("/users", func(ctx *lift.Context) error {
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        // Lift automatically returns 400 Bad Request
        return err
    }
    
    if req.Email == "" {
        // Custom validation with proper HTTP status
        return lift.NewError(400, "Email is required", map[string]interface{}{
            "field": "email",
            "code":  "REQUIRED",
        })
    }
    
    return ctx.JSON(201, user)
})
```

## Common Error Scenarios

### Scenario 1: Database Connection Errors

```go
// CORRECT: Handle database errors gracefully
app.GET("/users", func(ctx *lift.Context) error {
    users, err := db.GetUsers()
    if err != nil {
        // Log the error and return user-friendly message
        ctx.Logger().Error("Database error", "error", err)
        return lift.NewError(500, "Service temporarily unavailable", nil)
    }
    
    return ctx.JSON(users)
})
```

### Scenario 2: Validation Errors

```go
// CORRECT: Clear validation error messages
app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req CreateUserRequest) (UserResponse, error) {
    // Validation happens automatically
    // Custom business validation
    if !isValidDomain(req.Email) {
        return UserResponse{}, lift.NewError(400, "Invalid email domain", map[string]interface{}{
            "field":     "email",
            "allowed":   []string{"@company.com", "@partner.com"},
            "provided":  req.Email,
        })
    }
    
    return UserResponse{User: user}, nil
}))
```

### Scenario 3: Upstream Service Errors

```go
// CORRECT: Handle external service failures
app.POST("/payments", func(ctx *lift.Context) error {
    payment, err := paymentService.Process(ctx, req)
    if err != nil {
        // Different handling based on error type
        if errors.Is(err, ErrInsufficientFunds) {
            return lift.NewError(402, "Insufficient funds", nil)
        }
        if errors.Is(err, ErrServiceUnavailable) {
            return lift.NewError(503, "Payment service unavailable", nil)
        }
        
        // Generic error for unexpected issues
        ctx.Logger().Error("Payment processing error", "error", err)
        return lift.NewError(500, "Payment processing failed", nil)
    }
    
    return ctx.JSON(201, payment)
})
```

## What This Example Teaches

### ✅ Best Practices Demonstrated

1. **ALWAYS check registration errors** - Prevents silent failures at startup
2. **ALWAYS validate app.Start()** - Catches configuration issues early
3. **PREFER returning errors directly** - Let Lift handle HTTP responses
4. **ALWAYS log unexpected errors** - Essential for debugging
5. **USE structured errors** - Consistent API responses

### 🚫 Critical Anti-Patterns Avoided

1. **Ignoring registration errors** - Leads to runtime failures
2. **Manual HTTP status codes** - Inconsistent error responses
3. **Swallowing errors** - Makes debugging impossible
4. **Generic error messages** - Poor user experience
5. **No startup validation** - Production issues

## Testing Error Handling

```go
// CORRECT: Test error scenarios
func TestErrorHandling(t *testing.T) {
    app := testing.NewTestApp()
    
    // Test invalid registration
    err := app.App().POST("/test", "invalid handler")
    assert.Error(t, err)
    
    // Test handler errors
    app.App().GET("/error", func(ctx *lift.Context) error {
        return fmt.Errorf("test error")
    })
    
    response := app.GET("/error")
    assert.Equal(t, 500, response.StatusCode)
    assert.Contains(t, response.Body, "test error")
}
```

## Next Steps

After mastering error handling:

1. **Authentication** → See `examples/jwt-auth/`
2. **Database Integration** → See `examples/basic-crud-api/`
3. **Production Patterns** → See `examples/production-api/`
4. **Observability** → See `examples/observability-demo/`

## Common Issues

### Issue: "Handler registration failed"
**Cause:** Invalid handler type passed to route method
**Solution:** Use correct handler signatures:

```go
// CORRECT handler types:
func(ctx *lift.Context) error
lift.SimpleHandler(func(ctx *lift.Context, req T) (R, error))
```

### Issue: "Application won't start"
**Cause:** Configuration or environment issues
**Solution:** Check the error from `app.Start()` for specific details

### Issue: "Errors not showing proper status codes"
**Cause:** Using manual status setting instead of returning errors
**Solution:** Return errors directly, let Lift handle status codes

This example demonstrates the foundation of reliable error handling - master these patterns before building complex applications.