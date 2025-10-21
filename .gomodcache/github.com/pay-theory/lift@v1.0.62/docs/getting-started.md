# Getting Started with Lift

<!-- AI Training: This is the primary onboarding guide for new Lift users -->
**This guide teaches developers how to create their first Lift application. It follows a progressive disclosure pattern - starting simple and adding complexity as needed.**

## Prerequisites

Before you begin, ensure you have:
- Go 1.21 or later installed (tested with Go 1.23.10)
- AWS account with Lambda access
- Basic understanding of serverless concepts

## Installation

```bash
go get github.com/pay-theory/lift
```

## Your First Lift Application

Let's create a simple REST API that manages a todo list.

### Step 1: Basic Setup

Create `main.go`:

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
)

func main() {
    // Create a new Lift application
    app := lift.New()
    
    // Add essential middleware
    app.Use(
        middleware.RequestID(),  // Always first
        middleware.Logger(),     // Structured logging
        middleware.Recover(),    // Panic recovery
    )
    
    // Define a simple route
    app.GET("/hello", func(ctx *lift.Context) error {
        return ctx.JSON(map[string]string{
            "message": "Hello from Lift!",
        })
    })
    
    // Start the Lambda handler
    lambda.Start(app.HandleRequest)
}
```

### Step 2: Understanding the Context

The `lift.Context` is your gateway to all request and response operations:

```go
func handler(ctx *lift.Context) error {
    // Access request data
    name := ctx.Query("name")           // Query parameters
    userID := ctx.Param("id")          // Path parameters
    auth := ctx.Header("Authorization") // Headers
    
    // Parse JSON body
    var data map[string]interface{}
    if err := ctx.ParseRequest(&data); err != nil {
        return lift.ValidationError("Invalid JSON")
    }
    
    // Send response
    return ctx.JSON(map[string]interface{}{
        "greeting": "Hello, " + name,
        "data": data,
    })
}
```

### Step 3: Building a Complete API

Let's expand our example to a full todo API:

```go
package main

import (
    "time"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
)

// Domain types
type Todo struct {
    ID        string    `json:"id"`
    Title     string    `json:"title" validate:"required,min=3"`
    Completed bool      `json:"completed"`
    CreatedAt time.Time `json:"created_at"`
}

type CreateTodoRequest struct {
    Title string `json:"title" validate:"required,min=3,max=100"`
}

// In-memory storage (replace with DynamoDB in production)
var todos = make(map[string]*Todo)

func main() {
    app := lift.New()
    
    // Configure the app
    config := &lift.Config{
        MaxRequestSize: 1 * 1024 * 1024, // 1MB
        Timeout:        25,               // 25 seconds
        LogLevel:       "INFO",
        MetricsEnabled: true,
    }
    app.WithConfig(config)
    
    // Global middleware
    app.Use(
        middleware.RequestID(),
        middleware.Logger(),
        middleware.Recover(),
    )
    
    // Public routes
    app.GET("/health", HealthCheck)
    
    // API routes
    api := app.Group("/api/v1")
    
    // Todo routes
    api.GET("/todos", ListTodos)
    api.GET("/todos/:id", GetTodo)
    api.POST("/todos", CreateTodo)
    api.PUT("/todos/:id", UpdateTodo)
    api.DELETE("/todos/:id", DeleteTodo)
    
    lambda.Start(app.HandleRequest)
}

// Handlers
func HealthCheck(ctx *lift.Context) error {
    return ctx.JSON(map[string]string{
        "status": "healthy",
        "time":   time.Now().Format(time.RFC3339),
    })
}

func ListTodos(ctx *lift.Context) error {
    // Convert map to slice
    result := make([]*Todo, 0, len(todos))
    for _, todo := range todos {
        result = append(result, todo)
    }
    
    return ctx.JSON(result)
}

func GetTodo(ctx *lift.Context) error {
    id := ctx.Param("id")
    
    todo, exists := todos[id]
    if !exists {
        return lift.NotFound("todo not found")
    }
    
    return ctx.JSON(todo)
}

func CreateTodo(ctx *lift.Context) error {
    var req CreateTodoRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return lift.ValidationError(err.Error())
    }
    
    todo := &Todo{
        ID:        generateID(),
        Title:     req.Title,
        Completed: false,
        CreatedAt: time.Now(),
    }
    
    todos[todo.ID] = todo
    
    ctx.Status(201)
    return ctx.JSON(todo)
}

func UpdateTodo(ctx *lift.Context) error {
    id := ctx.Param("id")
    
    todo, exists := todos[id]
    if !exists {
        return lift.NotFound("todo not found")
    }
    
    var updates struct {
        Title     *string `json:"title"`
        Completed *bool   `json:"completed"`
    }
    
    if err := ctx.ParseRequest(&updates); err != nil {
        return lift.ValidationError(err.Error())
    }
    
    if updates.Title != nil {
        todo.Title = *updates.Title
    }
    if updates.Completed != nil {
        todo.Completed = *updates.Completed
    }
    
    return ctx.JSON(todo)
}

func DeleteTodo(ctx *lift.Context) error {
    id := ctx.Param("id")
    
    if _, exists := todos[id]; !exists {
        return lift.NotFound("todo not found")
    }
    
    delete(todos, id)
    
    ctx.Status(204)
    return nil
}

func generateID() string {
    return fmt.Sprintf("%d", time.Now().UnixNano())
}
```

### Step 4: Adding Authentication

Protect your API with JWT authentication:

```go
import (
    "os"
    "github.com/pay-theory/lift/pkg/middleware"
)

func main() {
    app := lift.New()
    
    // ... existing setup ...
    
    // Public routes
    app.POST("/auth/login", Login)
    app.POST("/auth/register", Register)
    
    // Protected API routes
    api := app.Group("/api/v1")
    
    // Add JWT middleware
    jwtMiddleware := middleware.JWTAuth(middleware.JWTConfig{
        Secret: os.Getenv("JWT_SECRET"),
    })
    api.Use(jwtMiddleware)
    
    // These routes now require authentication
    api.GET("/todos", ListTodos)
    api.POST("/todos", CreateTodo)
    
    lambda.Start(app.HandleRequest)
}

// Access user info in handlers
func ListTodos(ctx *lift.Context) error {
    userID := ctx.UserID() // From JWT claims
    
    // Filter todos by user
    userTodos := filterTodosByUser(userID)
    
    return ctx.JSON(userTodos)
}
```

### Step 5: Adding Rate Limiting

Protect your API from abuse:

```go
import (
    "time"
    "github.com/pay-theory/lift/pkg/middleware"
)

func main() {
    app := lift.New()
    
    // ... existing setup ...
    
    // Global rate limit by IP
    ipLimiter, err := middleware.IPRateLimitWithLimited(
        1000,      // 1000 requests
        time.Hour, // per hour
    )
    if err != nil {
        panic(err)
    }
    app.Use(ipLimiter)
    
    // User-specific rate limit for authenticated routes
    api := app.Group("/api/v1")
    api.Use(jwtMiddleware)
    
    userLimiter, err := middleware.UserRateLimitWithLimited(
        100,              // 100 requests
        15*time.Minute,   // per 15 minutes
    )
    if err != nil {
        panic(err)
    }
    api.Use(userLimiter)
    
    lambda.Start(app.HandleRequest)
}
```

### Step 6: Error Handling

Lift provides structured error handling:

```go
func CreateTodo(ctx *lift.Context) error {
    var req CreateTodoRequest
    if err := ctx.ParseRequest(&req); err != nil {
        // Automatic 422 with validation details
        return lift.ValidationError(err.Error())
    }
    
    // Check authorization
    if !canCreateTodo(ctx.UserID()) {
        // Returns 403
        return lift.AuthorizationError("insufficient permissions")
    }
    
    // Create todo
    todo, err := createTodoInDB(req)
    if err != nil {
        ctx.Logger.Error("Failed to create todo", "error", err)
        // Returns 500 with safe message
        return lift.NewLiftError("DATABASE_ERROR", "Failed to create todo", 500)
    }
    
    ctx.Status(201)
    return ctx.JSON(todo)
}
```

### Step 7: Testing Your Application

Write tests using Lift's testing utilities:

```go
package main

import (
    "testing"
    
    "github.com/pay-theory/lift/pkg/testing"
    "github.com/stretchr/testify/assert"
)

func TestCreateTodo(t *testing.T) {
    // Create test app
    app := testing.NewTestApp()
    
    // Add the route to test
    app.App().POST("/api/v1/todos", CreateTodo)
    
    // Make request with authentication
    resp := app.WithHeaders(map[string]string{
        "Authorization": "Bearer test-token",
    }).POST("/api/v1/todos", map[string]string{
        "title": "Test Todo",
    })
    
    // Assert results
    resp.AssertStatus(201)
    
    // Check response body
    resp.AssertJSONPath("$.title", "Test Todo")
}
```

## Deployment

### Using SAM (AWS Serverless Application Model)

Create `template.yaml` (see the included `template.yaml` file in the project root):

```yaml
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Globals:
  Function:
    Timeout: 30
    MemorySize: 512
    Runtime: provided.al2
    Architectures:
      - arm64

Resources:
  TodoAPI:
    Type: AWS::Serverless::Function
    Properties:
      CodeUri: .
      Handler: bootstrap
      Environment:
        Variables:
          JWT_SECRET: !Ref JWTSecret
      Events:
        ApiEvent:
          Type: HttpApi
          Properties:
            Path: /{proxy+}
            Method: ANY

  JWTSecret:
    Type: AWS::SecretsManager::Secret
    Properties:
      Description: JWT signing secret
      GenerateSecretString:
        SecretStringTemplate: '{}'
        GenerateStringKey: 'secret'
        PasswordLength: 32

Outputs:
  ApiUrl:
    Description: API endpoint URL
    Value: !Sub 'https://${ServerlessHttpApi}.execute-api.${AWS::Region}.amazonaws.com/'
```

Build and deploy:

```bash
# Build for Lambda
GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap main.go

# Deploy
sam deploy --guided
```

## Next Steps

Now that you have a working Lift application:

1. **Add a Database**: Integrate with DynamoDB using [DynamORM](./DYNAMORM_GUIDE.md)
2. **Add Monitoring**: Set up CloudWatch dashboards and alerts
3. **Add WebSockets**: Build real-time features
4. **Add Event Processing**: Handle SQS, S3, and EventBridge events

## Common Patterns

### Type-Safe Handlers

Use `SimpleHandler` for automatic request parsing and validation. This is the **recommended approach** for handling structured requests:

**Benefits of SimpleHandler:**
- ✅ Automatic JSON parsing and validation
- ✅ Type-safe request/response handling
- ✅ Built-in error handling for malformed requests
- ✅ Cleaner, more maintainable code

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
}

type UserResponse struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

// Register with SimpleHandler - automatic parsing and validation
app.POST("/users", lift.SimpleHandler(createUser))

// Handler with automatic parsing - no manual JSON handling needed
func createUser(ctx *lift.Context, req CreateUserRequest) (UserResponse, error) {
    // req is already parsed and validated automatically
    // If validation fails, Lift returns 422 with details
    
    user := UserResponse{
        ID:    generateID(),
        Name:  req.Name,
        Email: req.Email,
    }
    
    if err := saveUser(user); err != nil {
        return UserResponse{}, lift.NewLiftError("SAVE_ERROR", "Failed to save user", 500)
    }
    
    return user, nil // Response is automatically serialized to JSON
}

// Alternative: Manual handler (NOT recommended)
func createUserManual(ctx *lift.Context) error {
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return err // Manual error handling
    }
    // Manual validation, manual response handling...
}
```

### Multi-Tenant Support

Build SaaS applications with tenant isolation:

```go
func main() {
    app := lift.New()
    
    config := &lift.Config{
        RequireTenantID: true, // Enforce tenant context
    }
    app.WithConfig(config)
    
    // Middleware extracts tenant from JWT
    api := app.Group("/api")
    api.Use(jwtMiddleware)
    
    api.GET("/data", GetTenantData)
    
    lambda.Start(app.HandleRequest)
}

func GetTenantData(ctx *lift.Context) error {
    tenantID := ctx.TenantID() // Automatically extracted
    
    // Query scoped to tenant
    data := db.Query(
        "SELECT * FROM data WHERE tenant_id = ?",
        tenantID,
    )
    
    return ctx.JSON(data)
}
```

### Event Processing

Handle various AWS events:

```go
func main() {
    app := lift.New()
    
    // HTTP routes
    app.GET("/health", HealthCheck)
    
    // SQS message processing
    app.SQS("process-orders", ProcessOrder)
    
    // S3 event handling
    app.S3("uploads", ProcessUpload)
    
    // EventBridge scheduled tasks
    app.EventBridge("daily-report", GenerateReport)
    
    lambda.Start(app.HandleRequest)
}

func ProcessOrder(ctx *lift.Context) error {
    // SQS message is in ctx.Request.Body
    var order Order
    if err := ctx.ParseRequest(&order); err != nil {
        return err
    }
    
    ctx.Logger.Info("Processing order", "order_id", order.ID)
    
    // Process the order
    return processOrder(order)
}
```

## Troubleshooting

### Common Issues

1. **Handler not found**: Ensure routes start with `/`
2. **Validation errors**: Check struct tags are correct
3. **Authentication failures**: Verify JWT secret matches
4. **Rate limit errors**: Check DynamoDB table exists
5. **SimpleHandler not working**: Ensure you're using the correct function signature `func(ctx *lift.Context, req RequestType) (ResponseType, error)`

### Error Codes

Lift uses standardized error codes defined in `pkg/lift/error_codes.go`:

- `VALIDATION_ERROR` (422) - Request validation failed
- `UNAUTHORIZED` (401) - Authentication required
- `AUTHORIZATION_ERROR` (403) - Insufficient permissions
- `NOT_FOUND` (404) - Resource not found
- `SYSTEM_ERROR` (500) - Internal server error

### Debug Mode

Enable detailed logging:

```go
config := &lift.Config{
    LogLevel: "DEBUG",
}
app.WithConfig(config)
```

## Resources

- [API Reference](./api-reference.md) - Complete API documentation
- [Testing Guide](./testing-guide.md) - Writing tests for Lift apps
- [Migration Guide](./migration-guide.md) - Migrating from raw Lambda
- [Examples](../examples/) - Sample applications

---

This guide provides the foundation for building serverless applications with Lift. As you grow more comfortable, explore advanced features like custom middleware, WebSocket support, and multi-region deployment.
