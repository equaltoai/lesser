# How to Create a Basic Lift Application

## Overview

This guide walks through creating a basic Lift application step-by-step, from initial setup to a working Lambda function. It covers three approaches: minimal, basic production-ready, and complete with Lambda and API Gateway integration.

## Quick Start: Minimal Application

### Step 1: Create Project Directory

```bash
mkdir my-lift-app
cd my-lift-app
```

### Step 2: Initialize Go Module

```bash
go mod init github.com/yourcompany/my-lift-app
```

### Step 3: Install Lift

```bash
go get github.com/pay-theory/lift/pkg/lift
go get github.com/aws/aws-lambda-go/lambda
```

### Step 4: Create main.go

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
)

func main() {
    // Create new Lift application
    app := lift.New()
    
    // Define a simple route
    app.GET("/hello", func(ctx *lift.Context) error {
        return ctx.JSON(map[string]string{
            "message": "Hello from Lift!",
        })
    })
    
    // Start Lambda handler
    lambda.Start(app.HandleRequest)
}
```

### Step 5: Build for Lambda

```bash
GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap main.go
```

You now have a working Lambda function in the `bootstrap` binary!

## Creating a Production-Ready Application

For production use, follow these enhanced steps:

### Step 1: Project Setup

```bash
# Create project structure
mkdir -p my-lift-app/{cmd,pkg/handlers}
cd my-lift-app

# Initialize module
go mod init github.com/yourcompany/my-lift-app

# Install dependencies
go get github.com/pay-theory/lift/pkg/lift
go get github.com/pay-theory/lift/pkg/middleware
go get github.com/aws/aws-lambda-go/lambda
```

### Step 2: Define Data Models

Create `pkg/models/models.go`:

```go
package models

import "time"

// HealthResponse is returned by the health check endpoint
type HealthResponse struct {
    Status    string    `json:"status"`
    Timestamp time.Time `json:"timestamp"`
    Version   string    `json:"version"`
}

// CreateItemRequest represents a request to create an item
type CreateItemRequest struct {
    Name        string `json:"name" validate:"required,min=3,max=100"`
    Description string `json:"description" validate:"max=500"`
}

// ItemResponse represents an item in responses
type ItemResponse struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Description string    `json:"description"`
    CreatedAt   time.Time `json:"created_at"`
    TenantID    string    `json:"tenant_id,omitempty"`
}
```

### Step 3: Create Handlers

Create `pkg/handlers/health.go`:

```go
package handlers

import (
    "time"
    
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/yourcompany/my-lift-app/pkg/models"
)

// HealthCheck handles health check requests
func HealthCheck(ctx *lift.Context) error {
    response := models.HealthResponse{
        Status:    "healthy",
        Timestamp: time.Now(),
        Version:   "1.0.0",
    }
    
    return ctx.JSON(response)
}
```

Create `pkg/handlers/items.go`:

```go
package handlers

import (
    "time"
    
    "github.com/google/uuid"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/yourcompany/my-lift-app/pkg/models"
)

// In-memory storage (replace with database in production)
var items = make(map[string]*models.ItemResponse)

// ListItems returns all items
func ListItems(ctx *lift.Context) error {
    itemList := make([]*models.ItemResponse, 0, len(items))
    for _, item := range items {
        itemList = append(itemList, item)
    }
    
    return ctx.JSON(itemList)
}

// GetItem returns a single item by ID
func GetItem(ctx *lift.Context) error {
    id := ctx.Param("id")
    
    item, exists := items[id]
    if !exists {
        return lift.NotFound("item not found")
    }
    
    return ctx.JSON(item)
}

// CreateItem creates a new item
func CreateItem(ctx *lift.Context) error {
    var req models.CreateItemRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return lift.ValidationError(err.Error())
    }
    
    item := &models.ItemResponse{
        ID:          uuid.New().String(),
        Name:        req.Name,
        Description: req.Description,
        CreatedAt:   time.Now(),
        TenantID:    ctx.TenantID(),
    }
    
    items[item.ID] = item
    
    ctx.Status(201)
    return ctx.JSON(item)
}

// DeleteItem deletes an item by ID
func DeleteItem(ctx *lift.Context) error {
    id := ctx.Param("id")
    
    if _, exists := items[id]; !exists {
        return lift.NotFound("item not found")
    }
    
    delete(items, id)
    
    ctx.Status(204)
    return nil
}
```

Create `pkg/handlers/routes.go`:

```go
package handlers

import "github.com/pay-theory/lift/pkg/lift"

// RegisterRoutes registers all application routes
func RegisterRoutes(app *lift.App) {
    // Public routes
    app.GET("/health", HealthCheck)
    
    // API routes
    api := app.Group("/api/v1")
    api.GET("/items", ListItems)
    api.GET("/items/:id", GetItem)
    api.POST("/items", CreateItem)
    api.DELETE("/items/:id", DeleteItem)
}
```

### Step 4: Create Main Entry Point

Create `cmd/main.go`:

```go
package main

import (
    "os"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
    
    "github.com/yourcompany/my-lift-app/pkg/handlers"
)

func main() {
    // Create application
    app := lift.New()
    
    // Configure application
    config := &lift.Config{
        MaxRequestSize:  5 * 1024 * 1024, // 5MB
        MaxResponseSize: 6 * 1024 * 1024, // 6MB (Lambda limit)
        Timeout:         25,               // 25 seconds
        LogLevel:        getEnv("LOG_LEVEL", "INFO"),
        MetricsEnabled:  true,
        TracingEnabled:  true,
        CORSEnabled:     true,
    }
    app.WithConfig(config)
    
    // Add essential middleware (order matters!)
    app.Use(middleware.RequestID())    // Generate request ID first
    app.Use(middleware.Logger())       // Log with request ID
    app.Use(middleware.Recover())      // Catch panics
    
    // Add observability
    app.Use(middleware.EnhancedObservabilityMiddleware(
        middleware.EnhancedObservabilityConfig{
            EnableLogging: true,
            EnableMetrics: true,
            EnableTracing: true,
            SampleRate:    0.1, // 10% sampling
        },
    ))
    
    // Register all routes
    handlers.RegisterRoutes(app)
    
    // Start Lambda handler
    lambda.Start(app.HandleRequest)
}

// getEnv gets an environment variable with a fallback default
func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

### Step 5: Add Build Automation

Create `Makefile`:

```makefile
.PHONY: build test clean deploy

# Build for Lambda ARM64 (recommended)
build:
	@echo "Building for Lambda ARM64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
		-tags lambda.norpc \
		-ldflags="-s -w" \
		-o dist/bootstrap \
		cmd/main.go
	@echo "Build complete: dist/bootstrap"

# Run tests
test:
	@echo "Running tests..."
	go test -v -cover ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf dist/
	rm -f coverage.out

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Run infrastructure (requires CDK)
infra: build
	cd cdk && cdk deploy

# Package for deployment
package: build
	@echo "Packaging..."
	cd dist && zip function.zip bootstrap
```

### Step 6: Add Infrastructure (Optional)

Create `cdk/main.go` for CDK:

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
    
    stack := awscdk.NewStack(app, jsii.String("MyLiftApp"), nil)
    
    // Use Lift CDK patterns for quick setup
    liftApp := patterns.NewLiftApp(stack, jsii.String("my-app"),
        &patterns.LiftAppProps{
            AppName:            jsii.String("my-app"),
            CodeAssetPath:      jsii.String("../dist"),
            EnableDatabase:     jsii.Bool(false), // Add DynamoDB if needed
            EnableRateLimiting: jsii.Bool(true),
        })
    
    app.Synth(nil)
}
```

### Step 7: Build and Test

```bash
# Install dependencies
make deps

# Build the application
make build

# Run tests (if you have any)
make test

# Deploy infrastructure with CDK
make infra
```

### Step 8: Deploy

```bash
# Deploy with CDK
cd cdk && cdk deploy

# Or deploy with AWS CLI
aws lambda update-function-code \
    --function-name my-lift-function \
    --zip-file fileb://dist/function.zip
```

## Creating Application with Lambda and API Gateway Integration

For a complete application with full AWS integration:

### Complete Application Structure

```
my-lift-app/
├── cmd/
│   └── main.go
├── pkg/
│   ├── handlers/
│   │   ├── routes.go
│   │   ├── health.go
│   │   └── items.go
│   ├── models/
│   │   └── models.go
│   ├── services/
│   │   └── item_service.go
│   └── middleware/
│       └── auth.go
├── cdk/
│   ├── main.go
│   ├── cdk.json
│   └── go.mod
├── tests/
│   ├── integration/
│   │   └── api_test.go
│   └── unit/
│       └── handlers_test.go
├── dist/
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── template.yaml
```

### Enhanced Main with Full Integration

```go
package main

import (
    "os"
    "time"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
    
    "github.com/yourcompany/my-lift-app/pkg/handlers"
)

func main() {
    // Create application
    app := lift.New()
    defer app.Stop() // Cleanup on exit
    
    // Configuration
    config := &lift.Config{
        MaxRequestSize:  10 * 1024 * 1024, // 10MB
        MaxResponseSize: 6 * 1024 * 1024,  // 6MB
        Timeout:         25,                // 25 seconds
        LogLevel:        getEnv("LOG_LEVEL", "INFO"),
        MetricsEnabled:  true,
        TracingEnabled:  true,
        CORSEnabled:     true,
        AllowedOrigins:  []string{"https://example.com"},
        RequireTenantID: getEnv("ENVIRONMENT", "dev") == "production",
    }
    app.WithConfig(config)
    
    // Global middleware stack
    app.Use(middleware.RequestID())
    app.Use(middleware.Logger())
    app.Use(middleware.Recover())
    
    // Load shedding for protection
    loadConfig := middleware.ConfigureLoadSheddingForApp(app,
        middleware.NewBasicLoadShedding("my-app"))
    app.Use(middleware.LoadSheddingMiddleware(loadConfig))
    
    // Enhanced observability
    app.Use(middleware.EnhancedObservabilityMiddleware(
        middleware.EnhancedObservabilityConfig{
            EnableLogging: true,
            EnableMetrics: true,
            EnableTracing: true,
            SampleRate:    0.1,
            DefaultTags: map[string]string{
                "service":     "my-app",
                "environment": getEnv("ENVIRONMENT", "dev"),
            },
        },
    ))
    
    // Public routes (no auth required)
    app.GET("/health", handlers.HealthCheck)
    
    // Protected API routes
    api := app.Group("/api/v1")
    
    // JWT authentication for API routes
    if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
        api.Use(middleware.JWTAuth(middleware.JWTConfig{
            Secret: jwtSecret,
        }))
    }
    
    // Rate limiting
    rateLimiter, err := middleware.UserRateLimitWithLimited(
        100,              // 100 requests
        15*time.Minute,   // per 15 minutes
    )
    if err == nil {
        api.Use(rateLimiter)
    }
    
    // Register all API routes
    handlers.RegisterAPIRoutes(api)
    
    // Event handlers (SQS, S3, etc.)
    app.SQS("process-items", handlers.ProcessItemQueue)
    app.EventBridge("daily-cleanup", handlers.DailyCleanup)
    
    // Start Lambda handler
    lambda.Start(app.HandleRequest)
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

### CDK Infrastructure

Create `cdk/main.go`:

```go
package main

import (
    "os"
    
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/jsii-runtime-go"
    "github.com/pay-theory/lift/pkg/cdk/patterns"
)

func main() {
    app := awscdk.NewApp(nil)
    
    env := &awscdk.Environment{
        Account: jsii.String(os.Getenv("CDK_DEFAULT_ACCOUNT")),
        Region:  jsii.String(os.Getenv("CDK_DEFAULT_REGION")),
    }
    
    stack := awscdk.NewStack(app, jsii.String("MyLiftAppStack"), &awscdk.StackProps{
        Env: env,
    })
    
    // Use Lift's pre-configured pattern
    patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
        AppName:            jsii.String("my-lift-app"),
        CodeAssetPath:      jsii.String("../dist"),
        EnableDatabase:     jsii.Bool(true),
        EnableRateLimiting: jsii.Bool(true),
        EnableTracing:      jsii.Bool(true),
        MemorySize:         jsii.Number(512),
        Environment: &map[string]*string{
            "LOG_LEVEL":   jsii.String("INFO"),
            "ENVIRONMENT": jsii.String("production"),
        },
    })
    
    app.Synth(nil)
}
```

### Deploy with CDK

```bash
# Build application
make build

# Deploy infrastructure
cd cdk
cdk deploy
```

## Summary

### Quick Start (Minimal)
1. Initialize Go module
2. Install Lift
3. Create main.go with basic routes
4. Build for Lambda
5. Deploy

### Production Application
1. Organized project structure
2. Separate handlers, models, services
3. Full middleware stack
4. Build automation
5. Infrastructure as code
6. Testing setup

### Best Practices for Creating Lift Applications
- **Start simple, add complexity as needed**
- **Use middleware for cross-cutting concerns**
- **Separate business logic from HTTP handlers**
- **Configure from environment variables**
- **Include health check endpoints**
- **Add observability from the start**
- **Automate builds with Make or similar**
- **Use type-safe handlers when possible**
- **Follow Go 2025 best practices** (proper module structure, clear interfaces)

You now have everything needed to create Lift applications ranging from simple single-function handlers to complex multi-service applications with full AWS integration.

