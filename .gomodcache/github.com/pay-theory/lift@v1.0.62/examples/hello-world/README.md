# Hello World: Basic Lift Lambda Function

**This is the RECOMMENDED starting point for building AWS Lambda functions with the Lift framework.**

## What is This Example?

This example demonstrates the **fundamental patterns** for building serverless functions with Lift. It shows the **preferred approach** over raw Lambda handlers, providing automatic error handling, request parsing, and response formatting.

## Why Use Lift for Lambda?

✅ **USE Lift when:**
- Building AWS Lambda functions in Go
- Need automatic error handling and logging
- Want type-safe request/response handling
- Require consistent patterns across functions
- Building multi-tenant applications

❌ **DON'T USE when:**
- Not using AWS Lambda
- Need custom runtime behavior
- Building non-HTTP functions (use event adapters instead)

## Quick Start

```go
// This is the CORRECT pattern for Lift Lambda functions
package main

import "github.com/pay-theory/lift/pkg/lift"

func main() {
    app := lift.New()
    
    // PREFERRED: Simple endpoint handler
    app.GET("/hello", func(ctx *lift.Context) error {
        return ctx.JSON(map[string]string{
            "message": "Hello, World!",
        })
    })
    
    // Start the application - REQUIRED for all Lift functions
    app.Start()
}

// INCORRECT: Don't use raw Lambda handlers
// func handler(event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
//     // This lacks error handling, logging, and type safety
// }
```

## Core Patterns Demonstrated

### 1. Basic HTTP Endpoints

**Purpose:** Handle simple GET requests with query parameters
**When to use:** Health checks, simple data retrieval

```go
// CORRECT: Query parameter handling
app.GET("/hello", func(ctx *lift.Context) error {
    name := ctx.Query("name")  // Automatic parameter extraction
    if name == "" {
        name = "World"
    }
    
    return ctx.JSON(map[string]string{
        "message": fmt.Sprintf("Hello, %s!", name),
        "tenant":  ctx.TenantID(), // Built-in multi-tenant support
    })
})
```

### 2. Type-Safe Request Handling

**Purpose:** Parse and validate request bodies automatically
**When to use:** POST/PUT endpoints with structured data

```go
// PREFERRED: Type-safe handlers prevent runtime errors
type UserRequest struct {
    Name string `json:"name" validate:"required"`
    Age  int    `json:"age" validate:"min=0,max=120"`
}

type UserResponse struct {
    Message  string `json:"message"`
    UserID   string `json:"user_id"`
    TenantID string `json:"tenant_id,omitempty"`
}

app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req UserRequest) (UserResponse, error) {
    // Automatic JSON parsing and validation
    // No need for manual error checking
    
    return UserResponse{
        Message:  fmt.Sprintf("User %s created", req.Name),
        UserID:   "user_123",
        TenantID: ctx.TenantID(),
    }, nil
}))

// INCORRECT: Manual parsing is error-prone
// app.POST("/users", func(ctx *lift.Context) error {
//     var req UserRequest
//     if err := json.Unmarshal(body, &req); err != nil { // Error-prone
//         return err
//     }
//     // ... manual validation
// })
```

### 3. Path Parameters

**Purpose:** Extract dynamic values from URL paths
**When to use:** Resource-specific operations (GET /users/:id)

```go
// CORRECT: Automatic path parameter extraction
app.GET("/users/:id", func(ctx *lift.Context) error {
    userID := ctx.Param("id")  // Safe parameter access
    
    return ctx.JSON(map[string]any{
        "user_id": userID,
        "tenant":  ctx.TenantID(),
    })
})
```

### 4. Error Handling

**Purpose:** Consistent error responses and logging
**When to use:** All endpoints should follow this pattern

```go
// CORRECT: Simple error return - Lift handles the rest
app.POST("/error", func(ctx *lift.Context) error {
    return fmt.Errorf("this is a demo error")
    // Lift automatically:
    // - Logs the error with context
    // - Returns proper HTTP status
    // - Includes request ID for tracing
})

// INCORRECT: Manual error response construction
// app.POST("/error", func(ctx *lift.Context) error {
//     ctx.Status(500)
//     return ctx.JSON(map[string]string{"error": "manual error"})
// })
```

## Installation and Deployment

### Local Development

```bash
# Clone and run locally
cd examples/hello-world
go run main.go

# The function is ready but needs Lambda runtime to handle requests
# Use AWS SAM or Serverless Framework for local testing
```

### Lambda Deployment

```bash
# Build for Lambda
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
zip function.zip bootstrap

# Deploy with AWS CLI
aws lambda create-function \
  --function-name hello-world-lift \
  --runtime provided.al2 \
  --role arn:aws:iam::ACCOUNT:role/lambda-role \
  --handler bootstrap \
  --zip-file fileb://function.zip
```

## What This Example Teaches

### ✅ Best Practices Demonstrated

1. **Always use `lift.New()`** - Creates application with defaults
2. **Always call `app.Start()`** - Required for Lambda integration
3. **Prefer type-safe handlers** - Use `lift.SimpleHandler` for validation
4. **Use `ctx.TenantID()`** - Built-in multi-tenant support
5. **Return errors directly** - Lift handles HTTP status and logging

### 🚫 Anti-Patterns Avoided

1. **Raw Lambda handlers** - No error handling or logging
2. **Manual JSON parsing** - Error-prone and verbose
3. **Custom error responses** - Inconsistent across functions
4. **Hardcoded status codes** - Let Lift determine appropriate responses

## Next Steps

After mastering this example:

1. **Error Handling** → See `examples/error-handling/`
2. **Authentication** → See `examples/jwt-auth/`
3. **Database Integration** → See `examples/basic-crud-api/`
4. **Production Patterns** → See `examples/production-api/`

## Common Issues

### Issue: "Function not responding"
**Cause:** Missing `app.Start()` call
**Solution:** Always include `app.Start()` in your main function

### Issue: "Validation errors not working"
**Cause:** Using regular handlers instead of `lift.SimpleHandler`
**Solution:** Use `lift.SimpleHandler` for automatic validation

```go
// CORRECT: Automatic validation
app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req UserRequest) (UserResponse, error) {
    // Validation happens automatically
}))
```

This example provides the foundation for all Lift applications - master these patterns before moving to more complex examples.