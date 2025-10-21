# Core Components of a Lift Application

## Overview

This comprehensive guide explains all core components that make up a Lift application, how they interact, and how to use them effectively. Understanding these components is essential for building robust Lambda functions with Lift.

## The Five Core Components

Every Lift application consists of five fundamental components:

1. **App** - The application container
2. **Context** - Request/response handler
3. **Handler** - Request processing functions
4. **Middleware** - Cross-cutting functionality
5. **Config** - Application configuration

## 1. App Component

The `App` is the central container for your Lift application. It manages routes, middleware, configuration, and the Lambda execution lifecycle.

### Creating an App

```go
app := lift.New()
```

This single line creates a new Lift application with sensible defaults.

### App Responsibilities

The App component:
- **Route Management:** Registers HTTP and event handlers
- **Middleware Stack:** Manages middleware execution order
- **Configuration:** Holds application-wide settings
- **Event Routing:** Routes different AWS events to appropriate handlers
- **Lifecycle Management:** Handles startup and shutdown

### App Structure

```go
type App struct {
    Config      *Config              // Application configuration
    Middleware  []Middleware         // Global middleware stack
    Routes      map[string]Handler   // HTTP route handlers
    EventHandlers map[string]Handler // Event handlers (SQS, S3, etc.)
}
```

### App Methods

#### Route Registration

```go
// HTTP methods
app.GET(path, handler)
app.POST(path, handler)
app.PUT(path, handler)
app.PATCH(path, handler)
app.DELETE(path, handler)
app.OPTIONS(path, handler)
app.HEAD(path, handler)

// Generic handler
app.Handle(method, path, handler)

// Example
app.GET("/users", GetUsers)
app.POST("/users", CreateUser)
app.PUT("/users/:id", UpdateUser)
app.DELETE("/users/:id", DeleteUser)
```

#### Route Groups

Group related routes and apply middleware to them:

```go
// Create a group
api := app.Group("/api/v1")

// Apply middleware to the group
api.Use(middleware.JWTAuth(jwtConfig))
api.Use(rateLimiter)

// Register routes on the group
api.GET("/users", GetUsers)
api.POST("/orders", CreateOrder)

// Nested groups
admin := api.Group("/admin")
admin.Use(middleware.RequireRole("admin"))
admin.GET("/stats", GetStats)
```

#### Event Handlers

```go
// SQS events
app.SQS("queue-name", ProcessSQSMessage)

// S3 events
app.S3("bucket-uploads", ProcessS3Upload)

// EventBridge scheduled events
app.EventBridge("daily-report", GenerateDailyReport)

// DynamoDB Streams
app.DynamoDBStream("user-updates", ProcessUserUpdate)

// Kinesis streams
app.Kinesis("data-stream", ProcessStreamRecord)

// SNS events
app.SNS("notifications", ProcessNotification)
```

#### Configuration

```go
config := &lift.Config{
    MaxRequestSize:  10 * 1024 * 1024,  // 10MB
    MaxResponseSize: 6 * 1024 * 1024,   // 6MB
    Timeout:         30,                 // 30 seconds
    LogLevel:        "INFO",
    MetricsEnabled:  true,
    TracingEnabled:  true,
}

app.WithConfig(config)
```

#### Middleware

```go
// Global middleware (applies to all routes)
app.Use(middleware.RequestID())
app.Use(middleware.Logger())
app.Use(middleware.Recover())
```

#### Lambda Integration

```go
// Start the Lambda handler
lambda.Start(app.HandleRequest)
```

### Complete App Example

```go
package main

import (
    "os"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
)

func main() {
    // 1. Create app
    app := lift.New()
    
    // 2. Configure
    config := &lift.Config{
        LogLevel:        "INFO",
        MetricsEnabled:  true,
    }
    app.WithConfig(config)
    
    // 3. Add middleware
    app.Use(middleware.RequestID())
    app.Use(middleware.Logger())
    app.Use(middleware.Recover())
    
    // 4. Register routes
    app.GET("/health", HealthCheck)
    
    api := app.Group("/api/v1")
    api.Use(jwtMiddleware)
    api.GET("/users", GetUsers)
    api.POST("/users", CreateUser)
    
    // 5. Register event handlers
    app.SQS("orders", ProcessOrder)
    
    // 6. Start Lambda
    lambda.Start(app.HandleRequest)
}
```

## 2. Context Component

The `Context` is the most important component you'll interact with in your handlers. It provides access to request data, response methods, AWS services, and utilities.

### What is Context?

```go
type Context struct {
    // Request information
    Request  *Request
    Response *Response
    
    // AWS Lambda context
    Context  context.Context
    
    // Logging
    Logger   *Logger
    
    // Application reference
    App      *App
}
```

### Context Lifecycle

```
Request → Middleware → Handler(ctx *lift.Context) → Middleware → Response
                ↑                                              ↓
                └──── Context passed through all layers ──────┘
```

### Request Access Methods

#### Path Parameters

```go
func GetUser(ctx *lift.Context) error {
    // Route: /users/:id
    userID := ctx.Param("id")
    
    // Route: /users/:userID/posts/:postID
    userID := ctx.Param("userID")
    postID := ctx.Param("postID")
}
```

#### Query Parameters

```go
func SearchUsers(ctx *lift.Context) error {
    // GET /users?name=john&page=2&limit=20
    
    name := ctx.Query("name")              // "john"
    page := ctx.QueryInt("page", 1)        // 2 (default: 1)
    limit := ctx.QueryInt("limit", 10)     // 20 (default: 10)
    active := ctx.QueryBool("active", true) // true
}
```

#### Headers

```go
func Handler(ctx *lift.Context) error {
    // Get header
    auth := ctx.Header("Authorization")
    contentType := ctx.Header("Content-Type")
    
    // Get all headers
    headers := ctx.Headers()
}
```

#### Request Body

```go
func CreateUser(ctx *lift.Context) error {
    // Parse JSON body into struct
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return lift.ValidationError(err.Error())
    }
    
    // req is now populated and validated
    user := createUser(req)
    return ctx.JSON(user)
}
```

#### Multi-Tenant Support

```go
func Handler(ctx *lift.Context) error {
    // Automatically extracted from JWT or headers
    tenantID := ctx.TenantID()
    userID := ctx.UserID()
    
    // Query scoped to tenant
    data := db.Query("SELECT * FROM data WHERE tenant_id = ?", tenantID)
    
    return ctx.JSON(data)
}
```

#### Request Metadata

```go
func Handler(ctx *lift.Context) error {
    // Unique request identifier
    requestID := ctx.RequestID()
    
    // HTTP method
    method := ctx.Method()
    
    // Request path
    path := ctx.Path()
    
    // Client IP
    ip := ctx.ClientIP()
}
```

### Response Methods

#### JSON Response

```go
func GetUser(ctx *lift.Context) error {
    user := User{ID: "123", Name: "John"}
    return ctx.JSON(user)
}
```

#### Status Code

```go
func CreateUser(ctx *lift.Context) error {
    user := createUser()
    
    ctx.Status(201) // Set status before JSON
    return ctx.JSON(user)
}
```

#### Set Headers

```go
func Handler(ctx *lift.Context) error {
    ctx.SetHeader("Cache-Control", "no-cache")
    ctx.SetHeader("X-Custom-Header", "value")
    
    return ctx.JSON(data)
}
```

#### Raw Response

```go
func ServeHTML(ctx *lift.Context) error {
    html := "<html><body>Hello</body></html>"
    
    ctx.SetHeader("Content-Type", "text/html")
    return ctx.String(html)
}
```

### Logging

```go
func Handler(ctx *lift.Context) error {
    // Structured logging
    ctx.Logger.Info("Processing request",
        "user_id", ctx.UserID(),
        "tenant_id", ctx.TenantID(),
    )
    
    ctx.Logger.Error("Failed to process",
        "error", err,
        "request_id", ctx.RequestID(),
    )
    
    // Different log levels
    ctx.Logger.Debug("Debug information")
    ctx.Logger.Warn("Warning message")
    ctx.Logger.Error("Error occurred")
}
```

### Context Timeouts

```go
func LongRunningHandler(ctx *lift.Context) error {
    // Get deadline
    deadline, ok := ctx.Deadline()
    
    // Check if context is done
    select {
    case <-ctx.Done():
        return lift.NewLiftError("TIMEOUT", "operation timed out", 504)
    default:
        // Continue processing
    }
    
    return ctx.JSON(result)
}
```

### Storage and State

```go
func MiddlewareSetValue(next lift.Handler) lift.Handler {
    return lift.HandlerFunc(func(ctx *lift.Context) error {
        // Store value in context
        ctx.Set("user_role", "admin")
        return next.Handle(ctx)
    })
}

func Handler(ctx *lift.Context) error {
    // Retrieve value from context
    role := ctx.Get("user_role").(string)
    
    if role == "admin" {
        // Special handling for admins
    }
}
```

## 3. Handler Component

Handlers are functions that process requests. Lift supports two handler types:

### Standard Handler

```go
type Handler interface {
    Handle(ctx *lift.Context) error
}

// Function signature
func MyHandler(ctx *lift.Context) error {
    // Process request
    return ctx.JSON(response)
}
```

### Type-Safe Handler (Recommended)

```go
// Using SimpleHandler for type safety
app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req CreateUserRequest) (UserResponse, error) {
    // req is automatically parsed and validated
    user := createUser(req)
    
    // Return response (automatically converted to JSON)
    return UserResponse{
        ID:   user.ID,
        Name: user.Name,
    }, nil
}))
```

### Handler Best Practices

#### 1. Keep Handlers Thin

```go
// ❌ BAD: Business logic in handler
func CreateUser(ctx *lift.Context) error {
    var req CreateUserRequest
    ctx.ParseRequest(&req)
    
    // Lots of business logic here...
    if existingUser := db.FindByEmail(req.Email); existingUser != nil {
        return lift.ValidationError("email exists")
    }
    
    user := &User{...}
    db.Save(user)
    sendWelcomeEmail(user)
    // ...
}

// ✅ GOOD: Delegate to service
func CreateUser(ctx *lift.Context) error {
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return lift.ValidationError(err.Error())
    }
    
    user, err := userService.Create(ctx.Context, req)
    if err != nil {
        return handleServiceError(err)
    }
    
    ctx.Status(201)
    return ctx.JSON(user)
}
```

#### 2. Use Type-Safe Handlers

```go
// ✅ Preferred: Type-safe
app.POST("/users", lift.SimpleHandler(createUser))

func createUser(ctx *lift.Context, req CreateUserRequest) (UserResponse, error) {
    // Compile-time type checking
    // Automatic parsing and validation
    // Automatic response serialization
}
```

#### 3. Return Appropriate Errors

```go
func GetUser(ctx *lift.Context) error {
    user, err := db.GetUser(ctx.Param("id"))
    
    if err == db.ErrNotFound {
        return lift.NotFound("user not found")
    }
    
    if err != nil {
        ctx.Logger.Error("Database error", "error", err)
        return lift.SystemError("failed to retrieve user")
    }
    
    return ctx.JSON(user)
}
```

## 4. Middleware Component

Middleware wraps handlers to provide additional functionality.

### Middleware Signature

```go
type Middleware func(Handler) Handler
```

### How Middleware Works

```go
Request
  ↓
RequestID Middleware (before)
  ↓
Logger Middleware (before)
  ↓
Auth Middleware (before)
  ↓
YOUR HANDLER
  ↓
Auth Middleware (after)
  ↓
Logger Middleware (after)
  ↓
RequestID Middleware (after)
  ↓
Response
```

### Creating Custom Middleware

```go
func TimingMiddleware() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            // Before handler
            start := time.Now()
            
            // Call next handler
            err := next.Handle(ctx)
            
            // After handler
            duration := time.Since(start)
            ctx.Logger.Info("Request completed",
                "duration_ms", duration.Milliseconds(),
            )
            
            return err
        })
    }
}

// Usage
app.Use(TimingMiddleware())
```

### Middleware with Configuration

```go
func AuthMiddleware(config AuthConfig) lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            token := ctx.Header("Authorization")
            
            if !validateToken(token, config.Secret) {
                return lift.Unauthorized("invalid token")
            }
            
            return next.Handle(ctx)
        })
    }
}

// Usage
app.Use(AuthMiddleware(AuthConfig{
    Secret: os.Getenv("JWT_SECRET"),
}))
```

## 5. Config Component

Configuration holds application-wide settings.

### Config Structure

```go
type Config struct {
    // Size limits
    MaxRequestSize  int64    // Maximum request body size
    MaxResponseSize int64    // Maximum response body size
    
    // Timeouts
    Timeout         int      // Request timeout in seconds
    
    // Logging
    LogLevel        string   // "DEBUG", "INFO", "WARN", "ERROR"
    
    // Features
    MetricsEnabled  bool     // Enable CloudWatch metrics
    TracingEnabled  bool     // Enable X-Ray tracing
    Debug           bool     // Enable debug mode
    
    // CORS
    CORSEnabled     bool     // Enable CORS
    AllowedOrigins  []string // Allowed CORS origins
    
    // Multi-tenant
    RequireTenantID bool     // Require tenant ID for all requests
}
```

### Default Configuration

```go
&Config{
    MaxRequestSize:  10 * 1024 * 1024,  // 10MB
    MaxResponseSize: 6 * 1024 * 1024,   // 6MB (Lambda limit)
    Timeout:         30,                 // 30 seconds
    LogLevel:        "INFO",
    MetricsEnabled:  true,
    TracingEnabled:  false,
    Debug:           false,
    CORSEnabled:     true,
    AllowedOrigins:  []string{"*"},
    RequireTenantID: false,
}
```

### Configuring Your App

```go
// Method 1: Create config and apply
config := &lift.Config{
    LogLevel:        "DEBUG",
    Timeout:         60,
    RequireTenantID: true,
}
app.WithConfig(config)

// Method 2: Use default and modify
config := lift.DefaultConfig()
config.LogLevel = os.Getenv("LOG_LEVEL")
config.MetricsEnabled = os.Getenv("ENV") == "production"
app.WithConfig(config)
```

### Environment-Based Configuration

```go
func loadConfig() *lift.Config {
    return &lift.Config{
        LogLevel:        getEnv("LOG_LEVEL", "INFO"),
        Timeout:         getEnvInt("TIMEOUT", 30),
        MetricsEnabled:  getEnvBool("METRICS_ENABLED", true),
        RequireTenantID: getEnv("ENV", "dev") == "production",
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

## Component Interaction Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         Lambda Event                         │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                        App Component                         │
│  - Routes events to handlers                                 │
│  - Manages middleware stack                                  │
│  - Holds configuration                                       │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                   Middleware Stack                           │
│  RequestID → Logger → Recover → Auth → RateLimit            │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                   Context Component                          │
│  - Request data (params, query, body, headers)              │
│  - Response methods (JSON, Status, Headers)                 │
│  - Utilities (logging, tenant/user info)                    │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                   Handler Component                          │
│  func(ctx *lift.Context) error {                            │
│      // Your business logic                                  │
│      return ctx.JSON(response)                               │
│  }                                                           │
└─────────────────────────────────────────────────────────────┘
```

## Summary

### Core Components
1. **App** - Application container and router
2. **Context** - Request/response interface
3. **Handler** - Request processing functions
4. **Middleware** - Cross-cutting functionality
5. **Config** - Application settings

### Key Responsibilities
- **App:** Route management, middleware orchestration
- **Context:** Request/response access, utilities
- **Handler:** Business logic processing
- **Middleware:** Reusable request processing
- **Config:** Application-wide settings

### Best Practices
- Use App for structure and routing
- Use Context for all request/response operations
- Keep handlers thin, delegate to services
- Use middleware for cross-cutting concerns
- Configure from environment variables

Understanding these core components enables you to build well-structured, maintainable Lambda functions with Lift.



