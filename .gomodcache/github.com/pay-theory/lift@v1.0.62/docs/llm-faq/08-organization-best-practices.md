# Best Practices for Organizing Your Lift Application

## Overview

This comprehensive guide covers best practices for organizing Lift applications, from small single-function Lambdas to large multi-service systems. Following these patterns ensures maintainability, testability, and scalability as your application grows.

## Project Organization Principles

### 1. Separation of Concerns

Separate different aspects of your application:
- **Infrastructure** (CDK) separate from application code
- **Business logic** separate from HTTP handling
- **Data access** separate from business logic
- **Tests** mirror source structure

### 2. Standard Go Layout

Follow the Standard Go Project Layout (2025):
- `cmd/` - Application entry points
- `pkg/` - Library code
- `internal/` - Private application code
- `tests/` or `*_test.go` - Test files

### 3. Domain-Driven Structure

Organize by business domain, not technical layer:
- ✅ `users/`, `orders/`, `payments/`
- ❌ `controllers/`, `models/`, `services/`

## Small Application Structure (Single Service)

For simple Lambda functions with one responsibility:

```
my-service/
├── cmd/
│   └── main.go                 # Lambda entry point
├── internal/
│   ├── handlers/               # HTTP handlers
│   │   ├── health.go
│   │   └── users.go
│   ├── models/                 # Data models
│   │   └── user.go
│   └── service/                # Business logic
│       └── user_service.go
├── tests/
│   ├── handlers_test.go
│   └── service_test.go
├── dist/                       # Build output (gitignored)
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── template.yaml              # SAM template
```

### Example Files

**`cmd/main.go`** - Entry point
```go
package main

import (
    "os"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
    
    "github.com/yourcompany/my-service/internal/handlers"
)

func main() {
    app := lift.New()
    
    app.WithConfig(&lift.Config{
        LogLevel: os.Getenv("LOG_LEVEL"),
    })
    
    app.Use(middleware.RequestID())
    app.Use(middleware.Logger())
    app.Use(middleware.Recover())
    
    handlers.RegisterRoutes(app)
    
    lambda.Start(app.HandleRequest)
}
```

**`internal/handlers/users.go`** - HTTP handlers
```go
package handlers

import (
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/yourcompany/my-service/internal/models"
    "github.com/yourcompany/my-service/internal/service"
)

func GetUser(ctx *lift.Context) error {
    userID := ctx.Param("id")
    
    user, err := service.GetUserByID(ctx.Context, userID)
    if err != nil {
        return lift.NotFound("user not found")
    }
    
    return ctx.JSON(user)
}

func RegisterRoutes(app *lift.App) {
    app.GET("/health", HealthCheck)
    app.GET("/users/:id", GetUser)
    app.POST("/users", CreateUser)
}
```

**`internal/service/user_service.go`** - Business logic
```go
package service

import (
    "context"
    
    "github.com/yourcompany/my-service/internal/models"
)

func GetUserByID(ctx context.Context, userID string) (*models.User, error) {
    // Business logic here
    return &models.User{}, nil
}
```

## Medium Application Structure (Multiple Domains)

For applications with multiple business domains:

```
my-app/
├── cmd/
│   └── api/
│       └── main.go             # API Lambda entry point
├── internal/
│   ├── users/                  # User domain
│   │   ├── handler.go          # User HTTP handlers
│   │   ├── service.go          # User business logic
│   │   ├── model.go            # User data models
│   │   └── repository.go       # User data access
│   ├── orders/                 # Order domain
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── model.go
│   │   └── repository.go
│   ├── payments/               # Payment domain
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── model.go
│   │   └── repository.go
│   ├── shared/                 # Shared utilities
│   │   ├── auth/              # Authentication helpers
│   │   ├── database/          # Database connection
│   │   └── errors/            # Custom errors
│   └── config/                 # Configuration
│       └── config.go
├── pkg/                        # Public libraries
│   └── client/                 # API client (if needed)
├── cdk/                        # Infrastructure
│   ├── main.go
│   ├── cdk.json
│   └── stacks/
├── tests/
│   ├── integration/
│   └── unit/
├── dist/
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Domain Package Pattern

Each domain is self-contained:

```go
// internal/users/model.go
package users

type User struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
}

// internal/users/repository.go
package users

type Repository interface {
    GetByID(ctx context.Context, id string) (*User, error)
    Create(ctx context.Context, user *User) error
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
}

type dynamoRepository struct {
    // ...
}

func NewRepository(tableName string) Repository {
    return &dynamoRepository{/* ... */}
}

// internal/users/service.go
package users

type Service struct {
    repo Repository
}

func NewService(repo Repository) *Service {
    return &Service{repo: repo}
}

func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
    return s.repo.GetByID(ctx, id)
}

func (s *Service) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    user := &User{
        ID:    generateID(),
        Name:  req.Name,
        Email: req.Email,
    }
    
    if err := s.repo.Create(ctx, user); err != nil {
        return nil, err
    }
    
    return user, nil
}

// internal/users/handler.go
package users

import "github.com/pay-theory/lift/pkg/lift"

type Handler struct {
    service *Service
}

func NewHandler(service *Service) *Handler {
    return &Handler{service: service}
}

func (h *Handler) GetUser(ctx *lift.Context) error {
    user, err := h.service.GetUser(ctx.Context, ctx.Param("id"))
    if err != nil {
        return lift.NotFound("user not found")
    }
    return ctx.JSON(user)
}

func (h *Handler) CreateUser(ctx *lift.Context) error {
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return lift.ValidationError(err.Error())
    }
    
    user, err := h.service.CreateUser(ctx.Context, req)
    if err != nil {
        return lift.SystemError("failed to create user")
    }
    
    ctx.Status(201)
    return ctx.JSON(user)
}

func (h *Handler) RegisterRoutes(app *lift.App) {
    app.GET("/users/:id", h.GetUser)
    app.POST("/users", h.CreateUser)
}
```

### Main Application Assembly

```go
// cmd/api/main.go
package main

import (
    "os"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
    
    "github.com/yourcompany/my-app/internal/config"
    "github.com/yourcompany/my-app/internal/shared/database"
    "github.com/yourcompany/my-app/internal/users"
    "github.com/yourcompany/my-app/internal/orders"
    "github.com/yourcompany/my-app/internal/payments"
)

func main() {
    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        panic(err)
    }
    
    // Initialize database
    db := database.NewConnection(cfg.DatabaseTable)
    
    // Create domain services
    userRepo := users.NewRepository(db)
    userService := users.NewService(userRepo)
    userHandler := users.NewHandler(userService)
    
    orderRepo := orders.NewRepository(db)
    orderService := orders.NewService(orderRepo)
    orderHandler := orders.NewHandler(orderService)
    
    paymentService := payments.NewService(cfg.PaymentAPIKey)
    paymentHandler := payments.NewHandler(paymentService)
    
    // Create Lift app
    app := lift.New()
    app.WithConfig(cfg.Lift)
    
    // Global middleware
    app.Use(middleware.RequestID())
    app.Use(middleware.Logger())
    app.Use(middleware.Recover())
    
    // Health check
    app.GET("/health", healthCheck)
    
    // API routes
    api := app.Group("/api/v1")
    api.Use(middleware.JWTAuth(middleware.JWTConfig{
        Secret: cfg.JWTSecret,
    }))
    
    // Register domain routes
    userHandler.RegisterRoutes(api)
    orderHandler.RegisterRoutes(api)
    paymentHandler.RegisterRoutes(api)
    
    // Start Lambda
    lambda.Start(app.HandleRequest)
}
```

## Large Application Structure (Microservices)

For large systems with multiple Lambda functions:

```
my-platform/
├── services/                   # Individual services
│   ├── user-service/
│   │   ├── cmd/
│   │   │   └── main.go
│   │   ├── internal/
│   │   │   ├── handlers/
│   │   │   ├── service/
│   │   │   └── repository/
│   │   ├── go.mod
│   │   └── Makefile
│   ├── order-service/
│   │   ├── cmd/
│   │   ├── internal/
│   │   ├── go.mod
│   │   └── Makefile
│   └── payment-service/
│       ├── cmd/
│       ├── internal/
│       ├── go.mod
│       └── Makefile
├── pkg/                        # Shared libraries
│   ├── auth/                   # Auth utilities
│   ├── logging/                # Logging utilities
│   ├── models/                 # Shared models
│   └── errors/                 # Error types
├── infrastructure/             # Infrastructure as code
│   ├── cdk/
│   │   ├── main.go
│   │   ├── stacks/
│   │   │   ├── user_service_stack.go
│   │   │   ├── order_service_stack.go
│   │   │   └── payment_service_stack.go
│   │   └── cdk.json
│   └── shared/
│       ├── networking.go       # VPC, subnets
│       ├── database.go         # DynamoDB tables
│       └── messaging.go        # SQS, SNS
├── tests/
│   ├── integration/
│   └── e2e/
├── scripts/                    # Build and deployment scripts
│   ├── build-all.sh
│   ├── deploy-all.sh
│   └── test-all.sh
├── go.work                     # Go workspace
├── Makefile                    # Root Makefile
└── README.md
```

### Go Workspace for Microservices

```go
// go.work
go 1.23.10

use (
    ./services/user-service
    ./services/order-service
    ./services/payment-service
    ./pkg/auth
    ./pkg/logging
    ./pkg/models
)
```

This allows you to:
- Work across multiple modules
- Share code between services
- Test integration locally

## Code Organization Best Practices

### 1. Handler Organization

**By Resource (Recommended for REST APIs):**
```
handlers/
├── users.go        # All user endpoints
├── orders.go       # All order endpoints
└── payments.go     # All payment endpoints
```

**By Operation (For Complex Domains):**
```
users/
├── get_user.go
├── create_user.go
├── update_user.go
├── delete_user.go
└── list_users.go
```

### 2. Service Layer Organization

```go
// service/user_service.go
package service

type UserService struct {
    repo UserRepository
    cache Cache
    notifier Notifier
}

// Core operations
func (s *UserService) Create(ctx context.Context, req CreateUserRequest) (*User, error)
func (s *UserService) GetByID(ctx context.Context, id string) (*User, error)
func (s *UserService) Update(ctx context.Context, id string, req UpdateUserRequest) (*User, error)
func (s *UserService) Delete(ctx context.Context, id string) error
func (s *UserService) List(ctx context.Context, filter ListFilter) ([]User, error)

// Business operations
func (s *UserService) ChangePassword(ctx context.Context, userID string, oldPass, newPass string) error
func (s *UserService) VerifyEmail(ctx context.Context, userID, token string) error
```

### 3. Repository Pattern

```go
// repository/user_repository.go
package repository

type UserRepository interface {
    // CRUD operations
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id string) (*User, error)
    GetByEmail(ctx context.Context, email string) (*User, error)
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
    
    // Query operations
    List(ctx context.Context, filter ListFilter) ([]User, error)
    Count(ctx context.Context, filter ListFilter) (int, error)
}

// DynamoDB implementation
type dynamoUserRepository struct {
    tableName string
    client    *dynamodb.Client
}

func NewDynamoUserRepository(tableName string) UserRepository {
    return &dynamoUserRepository{
        tableName: tableName,
        // ... initialize client
    }
}
```

### 4. Model Organization

```go
// models/user.go
package models

// Domain model (database)
type User struct {
    PK        string    `dynamorm:"pk"`
    SK        string    `dynamorm:"sk"`
    ID        string    `json:"id"`
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
}

// Request models
type CreateUserRequest struct {
    Email string `json:"email" validate:"required,email"`
    Name  string `json:"name" validate:"required,min=2"`
}

type UpdateUserRequest struct {
    Name string `json:"name,omitempty" validate:"omitempty,min=2"`
}

// Response models
type UserResponse struct {
    ID        string    `json:"id"`
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
}

// Conversion functions
func (u *User) ToResponse() *UserResponse {
    return &UserResponse{
        ID:        u.ID,
        Email:     u.Email,
        Name:      u.Name,
        CreatedAt: u.CreatedAt,
    }
}
```

### 5. Configuration Management

```go
// config/config.go
package config

type Config struct {
    // Lift config
    Lift *lift.Config
    
    // Database
    DatabaseTable string
    
    // AWS Services
    S3Bucket    string
    SQSQueueURL string
    
    // External APIs
    PaymentAPIURL string
    PaymentAPIKey string
    
    // Feature flags
    EnableCache    bool
    EnableMetrics  bool
}

func Load() (*Config, error) {
    return &Config{
        Lift: &lift.Config{
            LogLevel: getEnv("LOG_LEVEL", "INFO"),
            // ...
        },
        DatabaseTable:  mustGetEnv("DYNAMODB_TABLE"),
        S3Bucket:       mustGetEnv("S3_BUCKET"),
        SQSQueueURL:    getEnv("SQS_QUEUE_URL", ""),
        PaymentAPIURL:  mustGetEnv("PAYMENT_API_URL"),
        PaymentAPIKey:  mustGetEnv("PAYMENT_API_KEY"),
        EnableCache:    getEnvBool("ENABLE_CACHE", true),
        EnableMetrics:  getEnvBool("ENABLE_METRICS", true),
    }, nil
}
```

## Naming Conventions

### Files
- Lowercase with underscores: `user_service.go`, `order_handler.go`
- Test files: `user_service_test.go`
- Main files: `main.go`

### Packages
- Lowercase, no underscores: `users`, `orders`, `payments`
- Singular names: `user` not `users` (unless it's a collection)

### Variables and Functions
```go
// Exported (public)
type UserService struct{}
func NewUserService() *UserService

// Unexported (private)
type userRepository struct{}
func getUserByEmail(email string) *User

// Constants
const MaxRetries = 3
const defaultTimeout = 30
```

### Interfaces
```go
// Interface names
type UserRepository interface{} // -er suffix
type Notifier interface{}       // -er suffix
type Cache interface{}           // Noun

// Interface implementations don't need special naming
type dynamoUserRepository struct{} // implements UserRepository
```

## Testing Organization

### Test File Placement

```
internal/
├── users/
│   ├── handler.go
│   ├── handler_test.go      # Unit tests alongside source
│   ├── service.go
│   ├── service_test.go
│   ├── repository.go
│   └── repository_test.go
```

### Integration Tests

```
tests/
├── integration/
│   ├── users_integration_test.go
│   ├── orders_integration_test.go
│   └── payments_integration_test.go
└── e2e/
    └── api_e2e_test.go
```

## Summary

### Small Applications
- `cmd/` for entry point
- `internal/` for application code
- Organize by layer: handlers, service, models

### Medium Applications
- `cmd/` for entry points
- `internal/` organized by domain
- Each domain self-contained with handler/service/repository

### Large Applications
- `services/` directory with separate services
- `pkg/` for shared libraries
- `infrastructure/` for IaC
- `go.work` for workspace management

### Key Principles
1. **Separation of concerns** - handlers, services, repositories
2. **Domain-driven design** - organize by business domain
3. **Standard Go layout** - follow community conventions
4. **Testability** - structure enables easy testing
5. **Scalability** - structure grows with application
6. **Clarity** - code organization reflects architecture

Following these patterns ensures your Lift applications remain maintainable and scalable as they grow from simple functions to complex systems.

