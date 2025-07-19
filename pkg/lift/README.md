# Lift Framework

Lift is a lightweight framework for building AWS Lambda functions in Go. It provides a simple, consistent way to handle different event types, middleware support, and standardized error handling.

## Features

- Simple, consistent API for handling different event types
- Middleware support for cross-cutting concerns
- Standardized error handling
- Type-safe request/response handling
- Multi-tenant support
- JWT authentication

## Getting Started

### Basic Lambda Function

```go
package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/your-org/lesser/pkg/lift"
)

func main() {
	// Create Lift application
	app := lift.New()

	// Add middleware
	app.Use(lift.RequestLogger())
	app.Use(lift.RequestID())
	app.Use(lift.Recover())

	// Add routes
	app.GET("/health", lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	}))

	app.POST("/users", lift.HandlerFunc(createUserHandler))

	// Start Lambda handler
	lambda.Start(app.HandleRequest)
}

func createUserHandler(ctx *lift.Context) error {
	var req CreateUserRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return err
	}

	// Process request
	user, err := userService.CreateUser(req)
	if err != nil {
		return mapErrorToLiftError(err)
	}

	return ctx.JSON(user)
}
```

### Type-Safe Handlers

```go
// Type-safe handler
app.POST("/users", lift.SimpleHandler(func(ctx *lift.Context, req CreateUserRequest) (User, error) {
	// Request is already parsed and validated
	user, err := userService.CreateUser(req)
	if err != nil {
		return User{}, mapErrorToLiftError(err)
	}

	return user, nil
}))
```

### Multi-Event Handler

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

	// ONE lambda.Start call handles ALL event types
	lambda.Start(app.HandleRequest)
}

func orderQueueHandler(ctx context.Context, event events.SQSEvent) error {
	// Process SQS event
	for _, record := range event.Records {
		// Process record
		_ = record.Body
	}

	return nil
}

func s3Handler(ctx context.Context, event events.S3Event) error {
	// Process S3 event
	for _, record := range event.Records {
		// Process record
		_ = record.S3.Object.Key
	}

	return nil
}
```

### Multi-Tenant Support

```go
func main() {
	app := lift.New()

	// Add JWT middleware
	app.Use(lift.JWT(lift.JWTConfig{
		SecretKey:      os.Getenv("JWT_SECRET"),
		RequireTenantID: true,
	}))

	// Add tenant middleware
	app.Use(lift.TenantMiddleware(lift.DefaultTenantConfig()))

	// Add routes
	app.POST("/api/projects", lift.SimpleHandler(func(ctx *lift.Context, req CreateProjectRequest) (Project, error) {
		// Request is already parsed and validated
		// Tenant ID is already in context from middleware
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

	// Start Lambda handler
	lambda.Start(app.HandleRequest)
}
```

## Error Handling

```go
// Helper function to map domain errors to Lift errors
func mapErrorToLiftError(err error) error {
	switch {
	case errors.Is(err, ErrUserNotFound):
		return lift.NotFound("User not found")
	case errors.Is(err, ErrInvalidInput):
		return lift.ValidationError(err.Error())
	case errors.Is(err, ErrUnauthorized):
		return lift.Unauthorized(err.Error())
	case errors.Is(err, ErrForbidden):
		return lift.Forbidden(err.Error())
	default:
		return lift.NewLiftError("INTERNAL_ERROR", "An internal error occurred", 500)
	}
}
```

## Middleware

```go
// Custom middleware
func customMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Do something before the handler
			fmt.Println("Before handler")

			// Call the next handler
			err := next.Handle(ctx)

			// Do something after the handler
			fmt.Println("After handler")

			return err
		})
	}
}

// Usage
app.Use(customMiddleware())
```

## Request Validation

```go
type CreateUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"min=0,max=150"`
}

func createUserHandler(ctx *lift.Context) error {
	var req CreateUserRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return err
	}

	// Request is already validated
	// ...
}
```

## Context Methods

- `ctx.JSON(v interface{})` - Send JSON response
- `ctx.JSONWithStatus(v interface{}, statusCode int)` - Send JSON response with status code
- `ctx.String(s string)` - Send string response
- `ctx.StringWithStatus(s string, statusCode int)` - Send string response with status code
- `ctx.NoContent(statusCode int)` - Send response with no content
- `ctx.Redirect(url string, statusCode int)` - Send redirect response
- `ctx.Error(statusCode int, message string)` - Send error response
- `ctx.ParseRequest(v interface{})` - Parse request body into struct
- `ctx.SetHeader(key, value string)` - Set response header
- `ctx.Header(key string)` - Get request header
- `ctx.Query(key string)` - Get query parameter
- `ctx.Param(key string)` - Get path parameter
- `ctx.Set(key string, value interface{})` - Set context value
- `ctx.Get(key string)` - Get context value
- `ctx.TenantID()` - Get tenant ID
- `ctx.UserID()` - Get user ID