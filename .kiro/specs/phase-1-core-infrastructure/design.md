# Design Document: Phase 1 Core Infrastructure

## Overview

This design outlines the implementation of standardized core infrastructure for Lesser, building upon the existing DynamORM and Lift integration. The infrastructure will provide consistent patterns for Lambda function initialization, middleware stacks, error handling, authentication, and testing across all 23+ Lambda functions in the system. This foundation will enable rapid development while maintaining high code quality and performance standards.

## Architecture

### Current State Analysis

Lesser currently has:
- Basic Lift integration in the API Lambda function
- Partial DynamORM implementation in storage repositories
- Custom authentication middleware
- Inconsistent error handling across functions
- Limited testing infrastructure

### Target Architecture

The core infrastructure will provide:
- **Standardized Application Factory**: Consistent Lift app initialization patterns
- **Unified Middleware Stack**: Common middleware for logging, CORS, auth, and cost tracking
- **Enhanced DynamORM Integration**: Cost tracking, transactions, batch operations, and migrations
- **Comprehensive Authentication**: Multi-method auth with tenant support
- **Testing Framework**: Mocking, integration testing, and performance validation

### Design Principles

1. **Convention over Configuration**: Provide sensible defaults with customization options
2. **Performance First**: Optimize for Lambda cold starts and execution efficiency
3. **Type Safety**: Leverage Go's type system for compile-time error detection
4. **Testability**: Design for easy unit and integration testing
5. **Observability**: Built-in logging, metrics, and cost tracking

## Components and Interfaces

### 1. Standardized Lift Application Structure

#### Application Factory

```go
// pkg/lift/app.go
package lift

import (
    "time"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
    "github.com/aron23/lesser/pkg/auth"
    "go.uber.org/zap"
)

type AppConfig struct {
    Debug             bool
    Timeout           time.Duration
    EnableCORS        bool
    EnableMetrics     bool
    EnableCostTracking bool
    AuthRequired      bool
    TenantRequired    bool
}

type AppBuilder struct {
    config AppConfig
    app    *lift.App
    logger *zap.Logger
}

func NewAppBuilder(config AppConfig, logger *zap.Logger) *AppBuilder {
    opts := []lift.Option{}
    if config.Debug {
        opts = append(opts, lift.WithDebug())
    }
    
    return &AppBuilder{
        config: config,
        app:    lift.New(opts...),
        logger: logger,
    }
}

func (ab *AppBuilder) WithStandardMiddleware() *AppBuilder {
    // Timeout middleware (first)
    if ab.config.Timeout > 0 {
        ab.app.Use(middleware.TimeoutMiddleware(middleware.TimeoutConfig{
            DefaultTimeout: ab.config.Timeout,
        }))
    }
    
    // Custom logging middleware (matches existing pattern)
    ab.app.Use(ab.createLoggingMiddleware())
    
    // CORS middleware (matches existing pattern)
    if ab.config.EnableCORS {
        ab.app.Use(ab.createCORSMiddleware())
    }
    
    // Cost tracking middleware (if enabled)
    if ab.config.EnableCostTracking {
        ab.app.Use(ab.createCostTrackingMiddleware())
    }
    
    return ab
}

func (ab *AppBuilder) Build() *lift.App {
    return ab.app
}

// Convenience functions for common patterns
func NewHTTPApp(config AppConfig, logger *zap.Logger) *lift.App {
    return NewAppBuilder(config, logger).
        WithStandardMiddleware().
        Build()
}

func NewSQSApp(config AppConfig, logger *zap.Logger) *lift.App {
    config.EnableCORS = false // Not needed for SQS
    return NewAppBuilder(config, logger).
        WithStandardMiddleware().
        Build()
}

func NewDynamoDBStreamApp(config AppConfig, logger *zap.Logger) *lift.App {
    config.EnableCORS = false
    config.AuthRequired = false
    return NewAppBuilder(config, logger).
        WithStandardMiddleware().
        Build()
}
```

#### Middleware Stack Implementation

```go
// Extend existing middleware patterns in cmd/api/middleware.go

func (ab *AppBuilder) createLoggingMiddleware() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            start := time.Now()

            // Process the request
            err := next.Handle(ctx)

            // Log the request after processing (matches existing pattern)
            ab.logger.Info("API request",
                zap.String("method", ctx.Request.Method),
                zap.String("path", ctx.Request.Path),
                zap.Int("status", ctx.Response.StatusCode),
                zap.Duration("duration", time.Since(start)),
                zap.String("request_id", ctx.GetRequestID()))

            return err
        })
    }
}

func (ab *AppBuilder) createCORSMiddleware() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            // Set CORS headers (matches existing pattern)
            ctx.Response.Header("Access-Control-Allow-Origin", "*")
            ctx.Response.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
            ctx.Response.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto, X-CSRF-Token")

            // Handle OPTIONS requests
            if ctx.Request.Method == "OPTIONS" {
                return ctx.Status(200).Text("")
            }

            // Process the request
            return next.Handle(ctx)
        })
    }
}

func (ab *AppBuilder) createCostTrackingMiddleware() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            // Initialize cost tracking context
            costCtx := cost.WithTracking(ctx.Request.Context())
            ctx.Request = ctx.Request.WithContext(costCtx)
            
            // Process request
            err := next.Handle(ctx)
            
            // Extract and log costs
            costs := cost.GetOperationCosts(costCtx)
            if len(costs) > 0 {
                totalCost := 0.0
                for _, c := range costs {
                    totalCost += c.Amount
                }
                
                ab.logger.Info("request_costs",
                    zap.String("request_id", ctx.GetRequestID()),
                    zap.Float64("total_cost", totalCost),
                    zap.Any("operations", costs),
                )
            }
            
            return err
        })
    }
}

// Enhanced auth middleware that builds on existing auth.Middleware
func CreateAuthMiddleware(authMiddleware *auth.Middleware, requireTenant bool) lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            // Convert Lift context to API Gateway request for existing middleware
            request := convertLiftContextToAPIGatewayRequest(ctx)
            
            // Use existing auth middleware
            claims, err := authMiddleware.RequireAuth(ctx.Request.Context(), request)
            if err != nil {
                return ctx.Unauthorized("Authentication required", err)
            }
            
            // Set claims in context (matches existing pattern)
            ctx.Set("claims", claims)
            
            // Handle tenant resolution if required
            if requireTenant {
                tenantID, err := resolveTenant(ctx)
                if err != nil {
                    return ctx.BadRequest("Tenant context required", err)
                }
                ctx.Set("tenantID", tenantID)
            }
            
            return next.Handle(ctx)
        })
    }
}
```

#### Context Utilities

```go
// pkg/lift/context/utils.go
package context

import (
    "errors"
    "strconv"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/aron23/lesser/pkg/auth"
)

// Authentication context helpers (matches existing patterns)
func GetClaims(ctx *lift.Context) (*auth.Claims, error) {
    claims, ok := ctx.Get("claims").(*auth.Claims)
    if !ok {
        return nil, errors.New("claims not found in context")
    }
    return claims, nil
}

func MustGetClaims(ctx *lift.Context) *auth.Claims {
    claims, err := GetClaims(ctx)
    if err != nil {
        panic(err) // Should never happen with proper middleware
    }
    return claims
}

func GetUserID(ctx *lift.Context) (string, error) {
    claims, err := GetClaims(ctx)
    if err != nil {
        return "", err
    }
    return claims.Username, nil
}

func MustGetUserID(ctx *lift.Context) string {
    claims := MustGetClaims(ctx)
    return claims.Username
}

func GetTenantID(ctx *lift.Context) (string, error) {
    tenantID, ok := ctx.Get("tenantID").(string)
    if !ok {
        return "", errors.New("tenant ID not found in context")
    }
    return tenantID, nil
}

func GetRequestID(ctx *lift.Context) string {
    return ctx.GetRequestID() // Use Lift's built-in method
}

// Response helpers
func RespondWithData(ctx *lift.Context, data interface{}) error {
    return ctx.JSON(data)
}

func RespondWithError(ctx *lift.Context, err error) error {
    return err // Lift handles error conversion
}

type PaginationResponse struct {
    Data       interface{} `json:"data"`
    Pagination Pagination  `json:"pagination"`
}

type Pagination struct {
    Limit  int    `json:"limit"`
    Offset int    `json:"offset"`
    Total  int    `json:"total"`
    Next   string `json:"next,omitempty"`
    Prev   string `json:"prev,omitempty"`
}

func RespondWithPagination(ctx *lift.Context, data interface{}, pagination Pagination) error {
    response := PaginationResponse{
        Data:       data,
        Pagination: pagination,
    }
    return ctx.JSON(response)
}

// Pagination parameter extraction
func GetPaginationParams(ctx *lift.Context) (limit, offset int) {
    limit = 20 // Default limit
    offset = 0 // Default offset
    
    if l := ctx.QueryParam("limit"); l != "" {
        if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
            limit = parsed
        }
    }
    
    if o := ctx.QueryParam("offset"); o != "" {
        if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
            offset = parsed
        }
    }
    
    return limit, offset
}
```

### 2. Enhanced DynamORM Infrastructure

#### Cost Tracking Integration (Build on existing pkg/cost)

```go
// Enhance existing pkg/cost/tracker.go to work with DynamORM
package cost

import (
    "context"
    "sync"
    "time"
    "github.com/pay-theory/dynamorm/pkg/core"
)

// Extend existing cost tracking to work with DynamORM
type DynamORMCostTracker struct {
    *Tracker // Embed existing cost tracker
    client   core.DB
    mu       sync.RWMutex
}

func NewDynamORMCostTracker(client core.DB) *DynamORMCostTracker {
    return &DynamORMCostTracker{
        Tracker: NewTracker(), // Use existing tracker
        client:  client,
    }
}

func (ct *DynamORMCostTracker) TrackOperation(ctx context.Context, operation string, fn func() error) error {
    startTime := time.Now()
    
    // Get initial consumed capacity from existing tracker
    initialCapacity := ct.Tracker.GetConsumedCapacity()
    
    // Execute operation
    err := fn()
    
    // Calculate cost using existing logic
    finalCapacity := ct.Tracker.GetConsumedCapacity()
    cost := ct.Tracker.CalculateCost(
        finalCapacity.ReadCapacityUnits - initialCapacity.ReadCapacityUnits,
        finalCapacity.WriteCapacityUnits - initialCapacity.WriteCapacityUnits,
    )
    
    // Store cost in context using existing method
    ct.Tracker.AddOperationCost(ctx, OperationCost{
        Operation:   operation,
        ConsumedRCU: finalCapacity.ReadCapacityUnits - initialCapacity.ReadCapacityUnits,
        ConsumedWCU: finalCapacity.WriteCapacityUnits - initialCapacity.WriteCapacityUnits,
        Cost:        cost,
        Timestamp:   startTime,
        TableName:   ct.getTableName(),
    })
    
    return err
}

// Integration with existing DynamORM client initialization
func WrapWithCostTracking(client core.DB) core.DB {
    tracker := NewDynamORMCostTracker(client)
    return &CostTrackingDB{
        DB:      client,
        tracker: tracker,
    }
}
```

#### Transaction Manager

```go
// pkg/storage/dynamorm/transactions/manager.go
package transactions

import (
    "context"
    "fmt"
    "time"
    "github.com/pay-theory/dynamorm/pkg/core"
)

type TransactionManager struct {
    client core.DB
}

type TransactionOperation struct {
    Type             OperationType
    Item             interface{}
    Key              core.Key
    UpdateExpression string
    Condition        string
}

type OperationType int

const (
    OperationPut OperationType = iota
    OperationUpdate
    OperationDelete
    OperationConditionCheck
)

func NewTransactionManager(client core.DB) *TransactionManager {
    return &TransactionManager{client: client}
}

func (tm *TransactionManager) ExecuteWrite(ctx context.Context, operations ...TransactionOperation) error {
    tx := tm.client.NewTransaction()
    
    for _, op := range operations {
        switch op.Type {
        case OperationPut:
            tx.Put(op.Item)
        case OperationUpdate:
            tx.Update(op.Item, op.UpdateExpression)
        case OperationDelete:
            tx.Delete(op.Key)
        case OperationConditionCheck:
            tx.ConditionCheck(op.Key, op.Condition)
        }
    }
    
    return tx.Execute()
}

func (tm *TransactionManager) ExecuteWithRetry(ctx context.Context, operations ...TransactionOperation) error {
    maxRetries := 3
    baseDelay := 100 * time.Millisecond
    
    for attempt := 0; attempt < maxRetries; attempt++ {
        err := tm.ExecuteWrite(ctx, operations...)
        if err == nil {
            return nil
        }
        
        // Check if error is retryable (transaction conflict)
        if !isRetryableError(err) {
            return err
        }
        
        // Exponential backoff
        delay := baseDelay * time.Duration(1<<attempt)
        time.Sleep(delay)
    }
    
    return fmt.Errorf("transaction failed after %d attempts", maxRetries)
}

type TransactionBuilder struct {
    operations []TransactionOperation
}

func NewTransactionBuilder() *TransactionBuilder {
    return &TransactionBuilder{
        operations: make([]TransactionOperation, 0),
    }
}

func (tb *TransactionBuilder) Put(item interface{}) *TransactionBuilder {
    tb.operations = append(tb.operations, TransactionOperation{
        Type: OperationPut,
        Item: item,
    })
    return tb
}

func (tb *TransactionBuilder) Update(item interface{}, expr string) *TransactionBuilder {
    tb.operations = append(tb.operations, TransactionOperation{
        Type:             OperationUpdate,
        Item:             item,
        UpdateExpression: expr,
    })
    return tb
}

func (tb *TransactionBuilder) Delete(key core.Key) *TransactionBuilder {
    tb.operations = append(tb.operations, TransactionOperation{
        Type: OperationDelete,
        Key:  key,
    })
    return tb
}

func (tb *TransactionBuilder) Build() []TransactionOperation {
    return tb.operations
}
```

#### Batch Operations

```go
// pkg/storage/dynamorm/batch/operations.go
package batch

import (
    "context"
    "fmt"
    "sync"
    "github.com/pay-theory/dynamorm/pkg/core"
)

type BatchWriter struct {
    client    core.DB
    batchSize int
}

func NewBatchWriter(client core.DB, batchSize int) *BatchWriter {
    if batchSize > 25 { // DynamoDB limit
        batchSize = 25
    }
    return &BatchWriter{
        client:    client,
        batchSize: batchSize,
    }
}

func (bw *BatchWriter) WriteItems(ctx context.Context, items []interface{}) error {
    for i := 0; i < len(items); i += bw.batchSize {
        end := i + bw.batchSize
        if end > len(items) {
            end = len(items)
        }
        
        batch := items[i:end]
        if err := bw.writeBatch(ctx, batch); err != nil {
            return fmt.Errorf("batch write failed at index %d: %w", i, err)
        }
    }
    return nil
}

func (bw *BatchWriter) WriteItemsParallel(ctx context.Context, items []interface{}, workers int) error {
    if workers <= 0 {
        workers = 1
    }
    
    // Create work channels
    workChan := make(chan []interface{}, workers)
    errChan := make(chan error, workers)
    
    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for batch := range workChan {
                if err := bw.writeBatch(ctx, batch); err != nil {
                    errChan <- err
                    return
                }
            }
        }()
    }
    
    // Send work
    go func() {
        defer close(workChan)
        for i := 0; i < len(items); i += bw.batchSize {
            end := i + bw.batchSize
            if end > len(items) {
                end = len(items)
            }
            
            select {
            case workChan <- items[i:end]:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Wait for completion
    go func() {
        wg.Wait()
        close(errChan)
    }()
    
    // Check for errors
    for err := range errChan {
        if err != nil {
            return err
        }
    }
    
    return ctx.Err()
}

func (bw *BatchWriter) writeBatch(ctx context.Context, items []interface{}) error {
    batch := bw.client.NewBatchWrite()
    for _, item := range items {
        batch.Put(item)
    }
    return batch.Execute()
}
```

### 3. Lift-Native Authentication Infrastructure

#### New Lift-Native Authentication Middleware

```go
// Create new Lift-native auth middleware in pkg/lift/auth/middleware.go
package auth

import (
    "strings"
    "errors"
    "os"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/aron23/lesser/pkg/auth"
    "github.com/aron23/lesser/pkg/storage"
)

// LiftAuthService provides Lift-native authentication
type LiftAuthService struct {
    authService *auth.AuthService
}

func NewLiftAuthService(store storage.Storage) (*LiftAuthService, error) {
    authService, err := auth.NewAuthService(store)
    if err != nil {
        return nil, err
    }
    
    return &LiftAuthService{
        authService: authService,
    }, nil
}

// RequireAuth creates Lift middleware for authentication
func (las *LiftAuthService) RequireAuth() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            // Extract Bearer token directly from Lift context
            token := extractBearerToken(ctx)
            if token == "" {
                return ctx.Unauthorized("Authentication required", nil)
            }
            
            // Validate token using auth service directly
            claims, err := las.authService.ValidateAccessToken(token)
            if err != nil {
                return ctx.Unauthorized("Invalid token", err)
            }
            
            // Set claims in context
            ctx.Set("claims", claims)
            
            return next.Handle(ctx)
        })
    }
}

// RequireScope creates middleware for scope validation
func (las *LiftAuthService) RequireScope(scope string) lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
            if !ok {
                return ctx.Unauthorized("Authentication required", nil)
            }
            
            if !claims.HasScope(scope) {
                return ctx.Forbidden("Insufficient permissions", nil)
            }
            
            return next.Handle(ctx)
        })
    }
}

// OptionalAuth creates middleware for optional authentication
func (las *LiftAuthService) OptionalAuth() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            // Extract Bearer token
            token := extractBearerToken(ctx)
            if token != "" {
                // Validate token if present
                claims, err := las.authService.ValidateAccessToken(token)
                if err == nil {
                    ctx.Set("claims", claims)
                }
            }
            
            return next.Handle(ctx)
        })
    }
}

// RequireTenant creates middleware for tenant resolution
func (las *LiftAuthService) RequireTenant() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            tenantID, err := resolveTenant(ctx)
            if err != nil {
                return ctx.BadRequest("Tenant context required", err)
            }
            
            ctx.Set("tenantID", tenantID)
            return next.Handle(ctx)
        })
    }
}

// Helper functions
func extractBearerToken(ctx *lift.Context) string {
    authHeader := ctx.Header("Authorization")
    if authHeader == "" {
        return ""
    }
    
    parts := strings.SplitN(authHeader, " ", 2)
    if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
        return ""
    }
    
    return parts[1]
}

func resolveTenant(ctx *lift.Context) (string, error) {
    // Try header first
    if tenantID := ctx.Header("X-Tenant-ID"); tenantID != "" {
        return tenantID, nil
    }
    
    // Try subdomain
    if host := ctx.Header("Host"); host != "" {
        parts := strings.Split(host, ".")
        if len(parts) > 2 {
            return parts[0], nil
        }
    }
    
    // Try path parameter
    if tenantID := ctx.Param("tenant"); tenantID != "" {
        return tenantID, nil
    }
    
    return "", errors.New("tenant not specified")
}
```

#### Pre-Release Implementation Strategy

Since Lesser is pre-release with no existing users or data, we can implement a clean, direct approach:

```go
// Pre-release approach: Direct implementation of Lift-native auth
// - No migration needed - implement Lift-native auth directly
// - Remove API Gateway auth dependencies entirely
// - Clean, efficient implementation from the start

// Example usage in Lambda functions:
func main() {
    app := lift.NewHTTPApp(config, logger)
    
    // Initialize Lift-native auth service
    liftAuth, err := auth.NewLiftAuthService(store)
    if err != nil {
        panic(err)
    }
    
    // Public routes (no auth)
    app.GET("/health", healthHandler)
    
    // Authenticated routes
    authGroup := app.Group("", liftAuth.RequireAuth())
    authGroup.GET("/profile", profileHandler)
    
    // Admin routes (require scope)
    adminGroup := app.Group("", liftAuth.RequireAuth(), liftAuth.RequireScope("admin"))
    adminGroup.GET("/admin/users", adminUsersHandler)
    
    // Multi-tenant routes
    tenantGroup := app.Group("", liftAuth.RequireAuth(), liftAuth.RequireTenant())
    tenantGroup.GET("/tenant/data", tenantDataHandler)
    
    lambda.Start(app.HandleRequest)
}

// Context utilities for Lift-native auth
func GetClaims(ctx *lift.Context) (*auth.EnhancedClaims, error) {
    claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
    if !ok {
        return nil, errors.New("claims not found in context")
    }
    return claims, nil
}

func GetUserID(ctx *lift.Context) (string, error) {
    claims, err := GetClaims(ctx)
    if err != nil {
        return "", err
    }
    return claims.Username, nil
}

func GetTenantID(ctx *lift.Context) (string, error) {
    tenantID, ok := ctx.Get("tenantID").(string)
    if !ok {
        return "", errors.New("tenant ID not found in context")
    }
    return tenantID, nil
}

// Benefits of pre-release implementation:
// - No context conversion overhead
// - Clean, efficient Lift-native patterns
// - No legacy API Gateway dependencies
// - Optimal performance from day one
```

### 4. Comprehensive Testing Infrastructure

#### Testing Utilities

```go
// pkg/lift/testing/helpers.go
package testing

import (
    "context"
    "net/http"
    "net/http/httptest"
    "strings"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/aron23/lesser/pkg/auth"
)

// Test context builders
func NewTestContext(method, path, body string) *lift.Context {
    req := httptest.NewRequest(method, path, strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    
    rec := httptest.NewRecorder()
    
    // Create Lift context
    ctx := lift.NewContext(req, rec)
    return ctx
}

func NewAuthenticatedContext(method, path, body, userID string, scopes ...string) *lift.Context {
    ctx := NewTestContext(method, path, body)
    
    // Create test claims
    claims := &auth.EnhancedClaims{
        Claims: auth.Claims{
            Username: userID,
            Scopes:   scopes,
        },
        SessionID: "test-session",
        DeviceID:  "test-device",
    }
    
    // Set auth context
    ctx.Set("auth:claims", claims)
    ctx.Set("auth:userID", userID)
    ctx.Set("auth:sessionID", "test-session")
    ctx.Set("auth:deviceID", "test-device")
    ctx.Set("auth:scopes", scopes)
    
    return ctx
}

func NewTenantContext(method, path, body, userID, tenantID string, scopes ...string) *lift.Context {
    ctx := NewAuthenticatedContext(method, path, body, userID, scopes...)
    ctx.Set("auth:tenantID", tenantID)
    return ctx
}

// Token generators for integration tests
func GenerateTestToken(userID string, scopes ...string) string {
    authService, _ := auth.GetService()
    
    claims := &auth.EnhancedClaims{
        Claims: auth.Claims{
            Username: userID,
            Scopes:   scopes,
        },
        SessionID: "test-session",
        DeviceID:  "test-device",
    }
    
    token, _ := authService.GenerateTestToken(claims)
    return token
}

func GenerateExpiredToken(userID string) string {
    // Implementation for expired token generation
    return "expired-token"
}

func GenerateAdminToken(userID string) string {
    return GenerateTestToken(userID, "read", "write", "admin")
}

// Mock storage for testing
type MockStorage struct {
    // Mock implementation
}

func NewMockStorage() *MockStorage {
    return &MockStorage{}
}

// Integration test helpers
func SetupIntegrationTest() (*dynamorm.DB, func()) {
    // Setup DynamoDB Local connection
    config := session.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000",
        AccessKeyID:     "fakeMyKeyId",
        SecretAccessKey: "fakeSecretAccessKey",
    }
    
    db, err := dynamorm.New(config)
    if err != nil {
        panic(err)
    }
    
    // Create test tables
    createTestTables(db)
    
    // Return cleanup function
    cleanup := func() {
        dropTestTables(db)
    }
    
    return db, cleanup
}
```

## Data Models

### Standardized Model Patterns

All DynamoDB models will follow consistent patterns:

```go
// Base model with common fields
type BaseModel struct {
    CreatedAt  time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt  time.Time `dynamorm:"updated_at" json:"updated_at"`
    Version    int       `dynamorm:"version" json:"version"`
    TTL        *int64    `dynamorm:"ttl" json:"ttl,omitempty"`
}

// Standard entity pattern
type User struct {
    // Primary keys
    PK string `dynamorm:"pk" json:"pk"` // user#{id}
    SK string `dynamorm:"sk" json:"sk"` // user#{id}
    
    // GSI for email lookup
    Email string `dynamorm:"index:email-index,pk" json:"email"`
    
    // GSI for tenant queries
    TenantID string `dynamorm:"index:tenant-index,pk" json:"tenant_id"`
    Status   string `dynamorm:"index:tenant-index,sk" json:"status"`
    
    BaseModel
    
    // Business fields
    Username string `json:"username"`
    Name     string `json:"name"`
    Active   bool   `json:"active"`
}

// Multi-tenant pattern
type TenantResource struct {
    // Tenant isolation
    PK string `dynamorm:"pk" json:"pk"` // tenant#{tenant_id}
    SK string `dynamorm:"sk" json:"sk"` // resource#{resource_id}
    
    // Cross-tenant GSI (admin access)
    ResourceType string `dynamorm:"index:type-index,pk" json:"resource_type"`
    CreatedAt    string `dynamorm:"index:type-index,sk" json:"created_at"`
    
    BaseModel
    
    // Business fields
    TenantID    string `json:"tenant_id"`
    ResourceID  string `json:"resource_id"`
    Name        string `json:"name"`
    Description string `json:"description"`
}
```

## Error Handling

### Standardized Error Types

```go
// pkg/lift/errors/types.go
package errors

import (
    "github.com/pay-theory/lift/pkg/lift"
)

// Domain error types
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
    Code    string `json:"code"`
}

func (e ValidationError) Error() string {
    return e.Message
}

func NewValidationError(field, message string) error {
    return lift.NewError(400, message).
        WithDetail("field", field).
        WithDetail("code", "VALIDATION_ERROR")
}

func NewNotFoundError(resource string) error {
    return lift.NewError(404, "Resource not found").
        WithDetail("resource", resource).
        WithDetail("code", "NOT_FOUND")
}

func NewUnauthorizedError(message string) error {
    return lift.NewError(401, message).
        WithDetail("code", "UNAUTHORIZED")
}

func NewForbiddenError(message string) error {
    return lift.NewError(403, message).
        WithDetail("code", "FORBIDDEN")
}

func NewConflictError(message string) error {
    return lift.NewError(409, message).
        WithDetail("code", "CONFLICT")
}

func NewRateLimitError(retryAfter int) error {
    return lift.NewError(429, "Rate limit exceeded").
        WithDetail("code", "RATE_LIMIT").
        WithDetail("retry_after", retryAfter)
}

// Error mapping for DynamORM errors
func MapDynamORMError(err error) error {
    switch {
    case errors.Is(err, dynamorm.ErrNotFound):
        return NewNotFoundError("item")
    case errors.Is(err, dynamorm.ErrValidation):
        return NewValidationError("", err.Error())
    case errors.Is(err, dynamorm.ErrConditionalCheckFailed):
        return NewConflictError("Conditional check failed")
    default:
        return lift.NewError(500, "Internal server error").
            WithDetail("code", "INTERNAL_ERROR")
    }
}
```

## Testing Strategy

### Unit Testing Pattern

```go
// Example unit test
func TestUserService_CreateUser(t *testing.T) {
    // Setup
    mockRepo := new(mocks.MockUserRepository)
    service := NewUserService(mockRepo)
    
    // Configure mocks
    mockRepo.On("CreateUser", mock.AnythingOfType("*User")).Return(nil)
    
    // Test
    user := &User{
        Username: "testuser",
        Email:    "test@example.com",
        Name:     "Test User",
    }
    
    err := service.CreateUser(context.Background(), user)
    
    // Verify
    assert.NoError(t, err)
    mockRepo.AssertExpectations(t)
}
```

### Integration Testing Pattern

```go
// Example integration test
func TestUserRepository_Integration(t *testing.T) {
    // Setup
    db, cleanup := testing.SetupIntegrationTest()
    defer cleanup()
    
    repo := NewUserRepository(db)
    
    // Test
    user := &User{
        Username: "testuser",
        Email:    "test@example.com",
        Name:     "Test User",
    }
    
    err := repo.CreateUser(context.Background(), user)
    assert.NoError(t, err)
    
    // Verify
    retrieved, err := repo.GetUser(context.Background(), "testuser")
    assert.NoError(t, err)
    assert.Equal(t, user.Email, retrieved.Email)
}
```

## Performance Considerations

### Lambda Optimization

1. **Connection Reuse**: Initialize DynamORM and other clients in `init()` functions
2. **Pre-registration**: Register models during initialization to reduce cold start time
3. **Timeout Buffers**: Set appropriate timeout buffers to prevent Lambda timeouts
4. **Memory Optimization**: Use appropriate memory allocation for Lambda functions

### DynamoDB Optimization

1. **Query Patterns**: Design GSIs based on access patterns
2. **Batch Operations**: Use batch operations for bulk data processing
3. **Connection Pooling**: Reuse DynamoDB connections across requests
4. **Cost Tracking**: Monitor and optimize DynamoDB costs

## Security Considerations

### Authentication Security

1. **Token Validation**: Validate all tokens and check expiration
2. **Scope Enforcement**: Enforce proper scopes for API access
3. **Session Management**: Implement proper session lifecycle management
4. **Rate Limiting**: Implement rate limiting to prevent abuse

### Data Security

1. **Input Validation**: Validate all input data
2. **Output Sanitization**: Sanitize output to prevent information leakage
3. **Tenant Isolation**: Ensure proper tenant data isolation
4. **Audit Logging**: Log all security-relevant events

## Implementation Strategy

### Pre-Release Advantage

Since Lesser is pre-release with no existing users or data, we can implement a clean, direct approach:

1. **Direct Implementation**: No migration needed - implement optimal patterns from the start
2. **Clean Architecture**: Remove inefficient patterns and context conversion hacks
3. **Performance First**: Optimize for Lambda cold starts and execution efficiency
4. **Future-Proof**: Build infrastructure that scales with the project

### Implementation Phases

1. **Phase 1**: Core infrastructure components (Lift app factory, middleware, DynamORM enhancements)
2. **Phase 2**: Lift-native authentication (direct replacement, no coexistence needed)
3. **Phase 3**: Update existing Lambda functions to use new infrastructure
4. **Phase 4**: Performance validation and optimization

### Benefits of Pre-Release Implementation

1. **No Legacy Burden**: Can implement optimal patterns without backward compatibility concerns
2. **Performance Optimized**: No context conversion overhead or inefficient workarounds
3. **Clean Codebase**: Consistent patterns across all components from day one
4. **Faster Development**: No complex migration logic or dual-path implementations

## Conclusion

This core infrastructure design provides a solid foundation for Lesser's serverless architecture. By standardizing patterns for application initialization, middleware, authentication, and testing, we can ensure consistent, maintainable, and performant code across all Lambda functions. The infrastructure is designed to be extensible and can evolve with the project's needs while maintaining backward compatibility.