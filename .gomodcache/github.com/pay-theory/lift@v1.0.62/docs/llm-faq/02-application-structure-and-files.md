# Lift Application Structure and Required Files

## Overview

This guide explains the structure of a Lift application, the required files for a basic app, and the organizational patterns that make Lift applications maintainable and scalable.

## Minimal Application Structure

### What Files Are Required for a Basic Lift Application?

At minimum, a Lift application requires only **one file**: a Go source file containing your Lambda handler. Here's the absolute minimum structure:

```
my-lambda/
├── go.mod          # Go module definition (required)
├── go.sum          # Dependency checksums (auto-generated)
└── main.go         # Your Lambda handler (required)
```

**Example minimal `main.go`:**

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
)

func main() {
    app := lift.New()
    
    app.GET("/", func(ctx *lift.Context) error {
        return ctx.JSON(map[string]string{"message": "Hello, World!"})
    })
    
    lambda.Start(app.HandleRequest)
}
```

**Example minimal `go.mod`:**

```go
module github.com/yourcompany/my-lambda

go 1.23.10

require (
    github.com/pay-theory/lift
    github.com/aws/aws-lambda-go v1.49.0
)
```

This is everything you need to deploy a working Lambda function with Lift.

## Recommended Production Structure

While a single file works for simple functions, production applications benefit from proper organization:

```
my-lift-app/
├── cmd/
│   └── main.go                 # Lambda entry point
├── pkg/
│   ├── handlers/               # HTTP/Event handlers
│   │   ├── users.go
│   │   ├── orders.go
│   │   └── health.go
│   ├── models/                 # Data models
│   │   ├── user.go
│   │   └── order.go
│   ├── services/               # Business logic
│   │   ├── user_service.go
│   │   └── order_service.go
│   └── middleware/             # Custom middleware
│       └── auth.go
├── cdk/                        # Infrastructure as code (optional)
│   ├── main.go
│   ├── cdk.json
│   └── stacks/
├── tests/                      # Test files
│   ├── integration/
│   └── unit/
├── dist/                       # Build output (gitignored)
├── .gitignore
├── go.mod
├── go.sum
├── Makefile                    # Build automation
├── README.md
└── cdk/                       # CDK infrastructure (optional)
```

## File-by-File Breakdown

### Core Application Files

#### 1. `cmd/main.go` - Lambda Entry Point

This is the entry point for your Lambda function. It should be thin - just application assembly:

```go
package main

import (
    "os"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
    
    "github.com/yourcompany/my-app/pkg/handlers"
)

func main() {
    // Create application
    app := lift.New()
    
    // Configure from environment
    config := &lift.Config{
        MaxRequestSize:  10 * 1024 * 1024,
        Timeout:         25,
        LogLevel:        os.Getenv("LOG_LEVEL"),
        MetricsEnabled:  true,
        RequireTenantID: os.Getenv("ENVIRONMENT") == "production",
    }
    app.WithConfig(config)
    
    // Global middleware
    app.Use(middleware.RequestID())
    app.Use(middleware.Logger())
    app.Use(middleware.Recover())
    
    // Register routes
    handlers.RegisterRoutes(app)
    
    // Start Lambda
    lambda.Start(app.HandleRequest)
}
```

**Purpose:** 
- Application initialization and configuration
- Middleware registration
- Route registration
- Lambda runtime integration

**Best Practices:**
- Keep it simple - no business logic here
- Use environment variables for configuration
- Defer to package-level registration functions

#### 2. `pkg/handlers/` - Request Handlers

Handler files contain the functions that process requests:

```go
// pkg/handlers/users.go
package handlers

import (
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/yourcompany/my-app/pkg/models"
    "github.com/yourcompany/my-app/pkg/services"
)

// GetUser retrieves a user by ID
func GetUser(ctx *lift.Context) error {
    userID := ctx.Param("id")
    
    user, err := services.GetUserByID(ctx.Context, userID)
    if err != nil {
        return lift.NotFound("user not found")
    }
    
    return ctx.JSON(user)
}

// CreateUser creates a new user
func CreateUser(ctx *lift.Context) error {
    var req models.CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return lift.ValidationError(err.Error())
    }
    
    user, err := services.CreateUser(ctx.Context, req)
    if err != nil {
        ctx.Logger.Error("Failed to create user", "error", err)
        return lift.NewLiftError("CREATE_FAILED", "Failed to create user", 500)
    }
    
    ctx.Status(201)
    return ctx.JSON(user)
}
```

**Purpose:**
- Handle HTTP requests and responses
- Parse and validate input
- Call business logic services
- Return appropriate responses

**Best Practices:**
- One handler per HTTP operation
- Delegate business logic to services
- Use structured error responses
- Log errors with context

#### 3. `pkg/handlers/routes.go` - Route Registration

Centralize route registration in a dedicated file:

```go
// pkg/handlers/routes.go
package handlers

import (
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
)

// RegisterRoutes registers all application routes
func RegisterRoutes(app *lift.App) {
    // Public routes
    app.GET("/health", HealthCheck)
    app.POST("/auth/login", Login)
    
    // Protected API routes
    api := app.Group("/api/v1")
    api.Use(middleware.JWTAuth(middleware.JWTConfig{
        Secret: os.Getenv("JWT_SECRET"),
    }))
    
    // User routes
    api.GET("/users", ListUsers)
    api.GET("/users/:id", GetUser)
    api.POST("/users", CreateUser)
    api.PUT("/users/:id", UpdateUser)
    api.DELETE("/users/:id", DeleteUser)
    
    // Order routes
    api.GET("/orders", ListOrders)
    api.POST("/orders", CreateOrder)
}
```

**Purpose:**
- Single source of truth for all routes
- Group related routes
- Apply middleware to route groups

#### 4. `pkg/models/` - Data Models

Define your data structures:

```go
// pkg/models/user.go
package models

import "time"

// User represents a user in the system
type User struct {
    // DynamoDB keys
    PK string `dynamorm:"pk" json:"-"`              // user#123
    SK string `dynamorm:"sk" json:"-"`              // user#123
    
    // GSI keys for efficient queries
    Email    string `dynamorm:"index:email-index,pk" json:"email"`
    TenantID string `dynamorm:"index:tenant-index,pk" json:"tenant_id"`
    
    // User data
    UserID    string    `json:"user_id"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// CreateUserRequest is the request body for creating a user
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required,min=2,max=100"`
    Email string `json:"email" validate:"required,email"`
}

// UserResponse is returned when creating or retrieving a user
type UserResponse struct {
    UserID    string    `json:"user_id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    TenantID  string    `json:"tenant_id,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}
```

**Purpose:**
- Define data structures
- Specify validation rules
- Document data schema
- Enable type-safe operations

**Best Practices:**
- Use validation tags
- Separate request/response models from database models
- Include JSON tags for serialization
- Document fields with comments

#### 5. `pkg/services/` - Business Logic

Services contain your core business logic:

```go
// pkg/services/user_service.go
package services

import (
    "context"
    "fmt"
    "time"
    
    "github.com/google/uuid"
    "github.com/yourcompany/my-app/pkg/models"
)

// CreateUser creates a new user in the system
func CreateUser(ctx context.Context, req models.CreateUserRequest) (*models.User, error) {
    // Validate email is unique
    existing, _ := GetUserByEmail(ctx, req.Email)
    if existing != nil {
        return nil, fmt.Errorf("email already exists")
    }
    
    // Create user
    user := &models.User{
        UserID:    uuid.New().String(),
        Name:      req.Name,
        Email:     req.Email,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
    
    // Set DynamoDB keys
    user.PK = fmt.Sprintf("user#%s", user.UserID)
    user.SK = fmt.Sprintf("user#%s", user.UserID)
    
    // Save to database
    if err := saveUser(ctx, user); err != nil {
        return nil, fmt.Errorf("failed to save user: %w", err)
    }
    
    return user, nil
}

// GetUserByID retrieves a user by ID
func GetUserByID(ctx context.Context, userID string) (*models.User, error) {
    pk := fmt.Sprintf("user#%s", userID)
    return getUserByPK(ctx, pk)
}
```

**Purpose:**
- Implement business logic
- Coordinate between data sources
- Enforce business rules
- Handle transactions

**Best Practices:**
- Keep handlers thin, services thick
- Services should be testable without HTTP
- Return domain errors, not HTTP errors
- Use context for cancellation and timeouts

### Supporting Files

#### 6. `go.mod` and `go.sum`

**`go.mod`** defines your module and dependencies:

```go
module github.com/yourcompany/my-app

go 1.23.10

require (
    github.com/pay-theory/lift
    github.com/aws/aws-lambda-go v1.49.0
    github.com/pay-theory/dynamorm v1.0.23
    github.com/golang-jwt/jwt/v5 v5.2.2
    github.com/google/uuid v1.6.0
)
```

**`go.sum`** contains checksums (auto-generated, don't edit manually).

#### 7. `Makefile` - Build Automation

Automate common tasks:

```makefile
.PHONY: build test deploy clean

# Build for Lambda (ARM64)
build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
		-tags lambda.norpc \
		-o dist/bootstrap \
		cmd/main.go

# Build for Lambda (x86_64)
build-x86:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
		-tags lambda.norpc \
		-o dist/bootstrap \
		cmd/main.go

# Run tests
test:
	go test -v -cover ./...

# Run tests with coverage report
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# Deploy using CDK
deploy: build
	cd cdk && cdk deploy

# Clean build artifacts
clean:
	rm -rf dist/
	rm -f coverage.out

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run linter
lint:
	golangci-lint run ./...
```

#### 8. `.gitignore`

```gitignore
# Build output
dist/
bootstrap
*.zip

# Go build artifacts
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test coverage
coverage.out
*.test

# Go workspace
go.work
go.work.sum

# IDE
.vscode/
.idea/
*.swp
*.swo

# AWS
cdk.out/

# Environment
.env
.env.local
```

#### 9. `README.md` - Documentation

Document your application:

```markdown
# My Lift Application

## Overview
Brief description of what this Lambda function does.

## Prerequisites
- Go 1.23.10+
- AWS CLI configured and CDK installed

## Development

### Build
```bash
make build
```

### Test
```bash
make test
```

### Deploy
```bash
make deploy
```

## Environment Variables
- `LOG_LEVEL`: Logging level (DEBUG, INFO, WARN, ERROR)
- `JWT_SECRET`: Secret for JWT validation
- `TABLE_NAME`: DynamoDB table name

## API Endpoints
- `GET /health` - Health check
- `POST /api/v1/users` - Create user
- `GET /api/v1/users/:id` - Get user by ID
```

## Optional Infrastructure Files

### 10. `cdk/` - AWS CDK Infrastructure

For CDK deployments:

```bash
cd cdk/
```

```go
// cdk/main.go
package main

import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/jsii-runtime-go"
    "github.com/pay-theory/lift/pkg/cdk/patterns"
)

func main() {
    app := awscdk.NewApp(nil)
    
    patterns.NewLiftApp(awscdk.NewStack(app, jsii.String("MyApp"), nil),
        jsii.String("my-app"),
        &patterns.LiftAppProps{
            AppName:            jsii.String("my-app"),
            CodeAssetPath:      jsii.String("../dist"),
            EnableDatabase:     jsii.Bool(true),
            EnableRateLimiting: jsii.Bool(true),
        })
    
    app.Synth(nil)
}
```

## Summary

### Minimum Required Files
1. `main.go` - Lambda handler
2. `go.mod` - Module definition
3. `go.sum` - Dependencies (auto-generated)

### Recommended for Production
4. Organized package structure (`cmd/`, `pkg/`)
5. `Makefile` - Build automation
6. `.gitignore` - Version control
7. `README.md` - Documentation
8. Infrastructure code (CDK)

### Best Practices
- **Separate concerns:** handlers, services, models in different packages
- **One handler per file:** Easy to find and maintain
- **Centralized route registration:** Single source of truth
- **Automate builds:** Use Makefile or build scripts
- **Document thoroughly:** README with setup and API docs

This structure scales from simple single-file functions to complex multi-service applications while maintaining Go best practices from 2025.

