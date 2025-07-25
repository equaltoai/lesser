# Lift and DynamORM Migration Implementation Checklist

## Phase 1: Core Infrastructure (Week 1)

### 1.1 Project Setup and Dependencies

#### Tasks:
- [ ] Update go.mod with Lift and DynamORM dependencies
- [ ] Configure build scripts for Lift
- [ ] Set up environment variables
- [ ] Create base directory structure

#### Implementation Details:

**1. Update `/Users/aronprice/lesser/go.mod`:**
```go
require (
    github.com/pay-theory/lift v1.0.0
    github.com/pay-theory/dynamorm v1.0.0
    github.com/aws/aws-lambda-go v1.41.0
    github.com/aws/aws-sdk-go-v2 v1.24.0
    github.com/aws/aws-sdk-go-v2/config v1.26.0
    github.com/aws/aws-sdk-go-v2/service/dynamodb v1.26.0
)
```

**2. Create `/Users/aronprice/lesser/pkg/lift/config.go`:**
```go
package lift

import (
    "os"
    "github.com/pay-theory/lift"
)

type Config struct {
    DomainName    string
    TableName     string
    Region        string
    Environment   string
    LogLevel      string
}

func LoadConfig() *Config {
    return &Config{
        DomainName:    os.Getenv("DOMAIN_NAME"),
        TableName:     os.Getenv("DYNAMODB_TABLE"),
        Region:        os.Getenv("AWS_REGION"),
        Environment:   getEnvOrDefault("ENVIRONMENT", "development"),
        LogLevel:      getEnvOrDefault("LOG_LEVEL", "info"),
    }
}

func getEnvOrDefault(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

**3. Update `/Users/aronprice/lesser/Makefile`:**
```makefile
# Add Lift-specific build targets
build-lift-lambdas:
	@echo "Building Lift Lambda functions..."
	@for dir in cmd/*/; do \
		if grep -q "github.com/pay-theory/lift" "$$dir/main.go" 2>/dev/null; then \
			func=$$(basename $$dir); \
			echo "Building $$func..."; \
			cd $$dir && GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap . && \
			zip -j ../../dist/$$func.zip bootstrap && \
			rm bootstrap; \
		fi \
	done

test-lift:
	go test -v ./pkg/lift/... ./pkg/middleware/... ./pkg/handlers/...
```

#### Testing Requirements:
- Unit tests for configuration loading
- Integration test for environment variable handling
- Build script validation

#### Acceptance Criteria:
- All dependencies resolve correctly
- Build process creates valid Lambda deployment packages
- Configuration loads from environment variables

#### Common Pitfalls:
- Don't hardcode configuration values
- Ensure ARM64 architecture for Lambda builds
- Check for dependency version conflicts

### 1.2 Base Lambda Pattern Implementation

#### Tasks:
- [ ] Create base Lambda handler pattern
- [ ] Implement standard middleware stack
- [ ] Create handler utilities
- [ ] Set up error handling patterns

#### Implementation Details:

**1. Create `/Users/aronprice/lesser/pkg/lift/base/handler.go`:**
```go
package base

import (
    "github.com/pay-theory/lift"
    "github.com/pay-theory/lift/middleware"
    "github.com/aws/aws-lambda-go/lambda"
    liftmiddleware "github.com/pay-theory/lesser/pkg/lift/middleware"
)

// BaseApp creates a new Lift app with standard middleware
func NewBaseApp() *lift.Lift {
    app := lift.New()
    
    // Standard middleware stack (order matters!)
    app.Use(middleware.RequestID())      // Generate request ID
    app.Use(middleware.Logger())         // Log with request ID
    app.Use(liftmiddleware.CostTracking()) // Track DynamoDB costs
    app.Use(middleware.Recover())        // Catch panics
    app.Use(liftmiddleware.ErrorHandler()) // Structured error responses
    
    return app
}

// StartLambda starts the Lambda handler
func StartLambda(app *lift.Lift) {
    lambda.Start(app.HandleRequest)
}
```

**2. Create `/Users/aronprice/lesser/pkg/lift/middleware/cost_tracking.go`:**
```go
package middleware

import (
    "context"
    "sync/atomic"
    "github.com/pay-theory/lift"
)

type CostTracker struct {
    ReadUnits  atomic.Float64
    WriteUnits atomic.Float64
}

const CostTrackerKey = "cost_tracker"

// CostTracking middleware adds cost tracking to context
func CostTracking() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return func(ctx *lift.Context) error {
            tracker := &CostTracker{}
            ctx.Set(CostTrackerKey, tracker)
            
            err := next(ctx)
            
            // Log costs after request
            readCost := tracker.ReadUnits.Load()
            writeCost := tracker.WriteUnits.Load()
            if readCost > 0 || writeCost > 0 {
                ctx.Logger().WithFields(map[string]interface{}{
                    "dynamodb_read_units":  readCost,
                    "dynamodb_write_units": writeCost,
                    "estimated_cost_usd":   calculateCost(readCost, writeCost),
                }).Info("DynamoDB operation costs")
            }
            
            return err
        }
    }
}

func calculateCost(readUnits, writeUnits float64) float64 {
    // $0.25 per million read units, $1.25 per million write units
    return (readUnits * 0.00000025) + (writeUnits * 0.00000125)
}

// AddCost adds consumed capacity to the tracker
func AddCost(ctx context.Context, readUnits, writeUnits float64) {
    if tracker, ok := ctx.Value(CostTrackerKey).(*CostTracker); ok {
        tracker.ReadUnits.Add(readUnits)
        tracker.WriteUnits.Add(writeUnits)
    }
}
```

**3. Create `/Users/aronprice/lesser/pkg/lift/middleware/error_handler.go`:**
```go
package middleware

import (
    "github.com/pay-theory/lift"
    "github.com/pay-theory/lesser/pkg/errors"
)

// ErrorHandler converts application errors to HTTP responses
func ErrorHandler() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return func(ctx *lift.Context) error {
            err := next(ctx)
            if err == nil {
                return nil
            }
            
            // Handle known error types
            switch e := err.(type) {
            case *errors.ValidationError:
                return ctx.JSON(400, map[string]interface{}{
                    "error": e.Error(),
                    "field": e.Field,
                })
            case *errors.NotFoundError:
                return ctx.JSON(404, map[string]interface{}{
                    "error": e.Error(),
                })
            case *errors.UnauthorizedError:
                return ctx.JSON(401, map[string]interface{}{
                    "error": "Unauthorized",
                })
            case *lift.LiftError:
                return ctx.JSON(e.StatusCode, map[string]interface{}{
                    "error": e.Error(),
                    "code":  e.Code,
                })
            default:
                // Log unexpected errors
                ctx.Logger().WithError(err).Error("Unhandled error")
                return ctx.JSON(500, map[string]interface{}{
                    "error": "Internal server error",
                })
            }
        }
    }
}
```

**4. Create `/Users/aronprice/lesser/pkg/lift/handlers/health.go`:**
```go
package handlers

import (
    "github.com/pay-theory/lift"
    "github.com/pay-theory/dynamorm"
)

type HealthResponse struct {
    Status   string `json:"status"`
    Version  string `json:"version"`
    Database string `json:"database"`
}

// HealthHandler returns service health status
func HealthHandler(db *dynamorm.DB, version string) lift.HandlerFunc {
    return lift.SimpleHandler(func(ctx *lift.Context) (HealthResponse, error) {
        // Test database connection
        dbStatus := "healthy"
        if err := db.Ping(ctx.Request.Context()); err != nil {
            dbStatus = "unhealthy"
        }
        
        return HealthResponse{
            Status:   "ok",
            Version:  version,
            Database: dbStatus,
        }, nil
    })
}
```

#### Testing Requirements:

**Create `/Users/aronprice/lesser/pkg/lift/base/handler_test.go`:**
```go
package base

import (
    "testing"
    "github.com/stretchr/testify/assert"
    lifttesting "github.com/pay-theory/lift/pkg/testing"
)

func TestBaseApp(t *testing.T) {
    app := NewBaseApp()
    
    // Add test route
    app.GET("/test", func(ctx *lift.Context) error {
        return ctx.JSON(200, map[string]string{"status": "ok"})
    })
    
    // Test request
    ctx := lifttesting.NewTestContext(
        lifttesting.WithMethod("GET"),
        lifttesting.WithPath("/test"),
    )
    
    err := app.ServeHTTP(ctx)
    assert.NoError(t, err)
    assert.Equal(t, 200, ctx.Response.StatusCode)
    
    // Verify middleware executed
    assert.NotEmpty(t, ctx.Get("request_id"))
    assert.NotNil(t, ctx.Get(CostTrackerKey))
}
```

#### Acceptance Criteria:
- Base app initializes with correct middleware order
- Cost tracking captures all DynamoDB operations
- Error handler returns appropriate HTTP status codes
- Health check endpoint works

#### Common Pitfalls:
- Middleware order is critical (RequestID must be first)
- Don't use `app.Start()` for Lambda - use `lambda.Start(app.HandleRequest)`
- Ensure error handler catches all error types
- Cost tracking must be thread-safe

### 1.3 DynamORM Integration Layer

#### Tasks:
- [ ] Create DynamORM connection management
- [ ] Implement model registration system
- [ ] Create transaction helpers
- [ ] Set up query builders

#### Implementation Details:

**1. Create `/Users/aronprice/lesser/pkg/dynamorm/connection.go`:**
```go
package dynamorm

import (
    "context"
    "sync"
    "github.com/pay-theory/dynamorm"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

var (
    db   *dynamorm.LambdaDB
    once sync.Once
)

// InitDB initializes the DynamORM connection (call in init())
func InitDB(tableName string) error {
    var err error
    once.Do(func() {
        // Load AWS config
        cfg, configErr := config.LoadDefaultConfig(context.Background())
        if configErr != nil {
            err = configErr
            return
        }
        
        // Create DynamoDB client
        client := dynamodb.NewFromConfig(cfg)
        
        // Create Lambda-optimized DB
        db, err = dynamorm.NewLambdaOptimized(
            dynamorm.WithClient(client),
            dynamorm.WithTableName(tableName),
            dynamorm.WithCostTracking(true),
        )
        
        if err == nil {
            // Register all models
            registerModels()
        }
    })
    return err
}

// GetDB returns the DynamORM database instance
func GetDB() *dynamorm.LambdaDB {
    if db == nil {
        panic("database not initialized - call InitDB() in init()")
    }
    return db
}

// registerModels registers all DynamORM models
func registerModels() {
    // Register models here as they're created
    // Example: db.Register(&models.User{})
}
```

**2. Create `/Users/aronprice/lesser/pkg/dynamorm/transaction.go`:**
```go
package dynamorm

import (
    "context"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/lesser/pkg/lift/middleware"
)

// Transaction wraps DynamORM transactions with cost tracking
type Transaction struct {
    tx  *dynamorm.Transaction
    ctx context.Context
}

// NewTransaction creates a new transaction
func NewTransaction(ctx context.Context) *Transaction {
    return &Transaction{
        tx:  GetDB().Transaction(),
        ctx: ctx,
    }
}

// Put adds a put operation to the transaction
func (t *Transaction) Put(item interface{}) *Transaction {
    t.tx.Put(item)
    return t
}

// Update adds an update operation
func (t *Transaction) Update(item interface{}) *Transaction {
    t.tx.Update(item)
    return t
}

// Delete adds a delete operation
func (t *Transaction) Delete(item interface{}) *Transaction {
    t.tx.Delete(item)
    return t
}

// Commit executes the transaction with cost tracking
func (t *Transaction) Commit() error {
    result, err := t.tx.Commit(t.ctx)
    if err != nil {
        return err
    }
    
    // Track costs
    if result.ConsumedCapacity != nil {
        readUnits := 0.0
        writeUnits := 0.0
        
        for _, capacity := range result.ConsumedCapacity {
            if capacity.ReadCapacityUnits != nil {
                readUnits += *capacity.ReadCapacityUnits
            }
            if capacity.WriteCapacityUnits != nil {
                writeUnits += *capacity.WriteCapacityUnits
            }
        }
        
        middleware.AddCost(t.ctx, readUnits, writeUnits)
    }
    
    return nil
}
```

**3. Create `/Users/aronprice/lesser/pkg/dynamorm/query_builder.go`:**
```go
package dynamorm

import (
    "context"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/lesser/pkg/lift/middleware"
)

// QueryBuilder wraps DynamORM queries with cost tracking
type QueryBuilder struct {
    query *dynamorm.Query
    ctx   context.Context
}

// NewQuery creates a new query builder
func NewQuery(ctx context.Context, model interface{}) *QueryBuilder {
    return &QueryBuilder{
        query: GetDB().Model(model),
        ctx:   ctx,
    }
}

// Where adds a condition
func (q *QueryBuilder) Where(condition string, args ...interface{}) *QueryBuilder {
    q.query = q.query.Where(condition, args...)
    return q
}

// Index specifies a GSI to use
func (q *QueryBuilder) Index(name string) *QueryBuilder {
    q.query = q.query.Index(name)
    return q
}

// Limit sets the query limit
func (q *QueryBuilder) Limit(n int) *QueryBuilder {
    q.query = q.query.Limit(n)
    return q
}

// ScanIndexForward sets sort order
func (q *QueryBuilder) ScanIndexForward(forward bool) *QueryBuilder {
    q.query = q.query.ScanIndexForward(forward)
    return q
}

// Find executes the query with cost tracking
func (q *QueryBuilder) Find(result interface{}) error {
    output, err := q.query.FindWithOutput(q.ctx, result)
    if err != nil {
        return err
    }
    
    // Track costs
    if output.ConsumedCapacity != nil {
        readUnits := 0.0
        if output.ConsumedCapacity.ReadCapacityUnits != nil {
            readUnits = *output.ConsumedCapacity.ReadCapacityUnits
        }
        middleware.AddCost(q.ctx, readUnits, 0)
    }
    
    return nil
}

// First gets the first result
func (q *QueryBuilder) First(result interface{}) error {
    return q.Limit(1).Find(result)
}

// Count returns the count of matching items
func (q *QueryBuilder) Count() (int64, error) {
    output, err := q.query.CountWithOutput(q.ctx)
    if err != nil {
        return 0, err
    }
    
    // Track costs
    if output.ConsumedCapacity != nil {
        readUnits := 0.0
        if output.ConsumedCapacity.ReadCapacityUnits != nil {
            readUnits = *output.ConsumedCapacity.ReadCapacityUnits
        }
        middleware.AddCost(q.ctx, readUnits, 0)
    }
    
    return output.Count, nil
}
```

**4. Create example model `/Users/aronprice/lesser/pkg/models/base.go`:**
```go
package models

import (
    "time"
    "github.com/pay-theory/dynamorm"
)

// BaseModel provides common fields for all models
type BaseModel struct {
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate sets timestamps
func (b *BaseModel) BeforeCreate() error {
    now := time.Now()
    b.CreatedAt = now
    b.UpdatedAt = now
    return nil
}

// BeforeUpdate updates timestamp
func (b *BaseModel) BeforeUpdate() error {
    b.UpdatedAt = time.Now()
    return nil
}

// Example model using Lift table pattern
type User struct {
    BaseModel
    
    // Keys follow Lift pattern
    PK string `dynamorm:"pk"` // user#{user_id} or tenant#{tenant_id}
    SK string `dynamorm:"sk"` // user#{user_id}
    
    // GSI fields (must match CDK definition)
    Email    string `dynamorm:"index:email-index,pk" json:"email"`
    TenantID string `dynamorm:"index:tenant-index,pk" json:"tenant_id"`
    Created  string `dynamorm:"index:tenant-index,sk" json:"-"`
    
    // Business fields
    UserID   string    `json:"user_id"`
    Username string    `json:"username"`
    Name     string    `json:"name"`
    Bio      string    `json:"bio,omitempty"`
    Avatar   string    `json:"avatar,omitempty"`
    TTL      int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}

// NewUser creates a new user with proper keys
func NewUser(tenantID, userID string) *User {
    return &User{
        PK:       "user#" + userID,
        SK:       "user#" + userID,
        UserID:   userID,
        TenantID: tenantID,
        Created:  time.Now().Format(time.RFC3339),
    }
}
```

#### Testing Requirements:

**Create `/Users/aronprice/lesser/pkg/dynamorm/connection_test.go`:**
```go
package dynamorm

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/pay-theory/dynamorm/pkg/mocks"
)

func TestInitDB(t *testing.T) {
    // Test initialization
    err := InitDB("test-table")
    assert.NoError(t, err)
    
    // Test singleton
    db1 := GetDB()
    db2 := GetDB()
    assert.Same(t, db1, db2)
}

func TestQueryBuilder(t *testing.T) {
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    mockDB.On("Model", mock.Anything).Return(mockQuery)
    mockQuery.On("Where", "UserID = ?", "123").Return(mockQuery)
    mockQuery.On("FindWithOutput", mock.Anything, mock.Anything).Return(&dynamodb.QueryOutput{
        ConsumedCapacity: &dynamodb.ConsumedCapacity{
            ReadCapacityUnits: aws.Float64(2.5),
        },
    }, nil)
    
    // Test query with cost tracking
    ctx := context.WithValue(context.Background(), middleware.CostTrackerKey, &middleware.CostTracker{})
    
    var users []User
    err := NewQuery(ctx, &User{}).
        Where("UserID = ?", "123").
        Find(&users)
    
    assert.NoError(t, err)
    
    // Verify cost was tracked
    tracker := ctx.Value(middleware.CostTrackerKey).(*middleware.CostTracker)
    assert.Equal(t, 2.5, tracker.ReadUnits.Load())
}
```

#### Acceptance Criteria:
- Database initializes once during cold start
- All queries track consumed capacity
- Transactions support batch operations
- Models follow Lift table patterns

#### Common Pitfalls:
- Initialize DB in init(), not in handler
- Always track consumed capacity
- Use proper key patterns (pk/sk for Lift tables)
- GSI fields must match CDK definitions exactly

### 1.4 Context Utilities

#### Tasks:
- [ ] Create context helpers for common patterns
- [ ] Implement user/tenant extraction
- [ ] Add request scoped storage
- [ ] Create context factories for testing

#### Implementation Details:

**1. Create `/Users/aronprice/lesser/pkg/lift/context/helpers.go`:**
```go
package context

import (
    "github.com/pay-theory/lift"
    "github.com/pay-theory/lesser/pkg/auth"
    "github.com/pay-theory/lesser/pkg/models"
)

// Common context keys
const (
    UserKey     = "user"
    TenantKey   = "tenant"
    RequestIDKey = "request_id"
    AccountKey  = "account"
)

// GetUser extracts the authenticated user from context
func GetUser(ctx *lift.Context) (*models.User, error) {
    if user, ok := ctx.Get(UserKey).(*models.User); ok {
        return user, nil
    }
    return nil, lift.NewLiftError("UNAUTHORIZED", "User not authenticated", 401)
}

// GetUserID extracts just the user ID
func GetUserID(ctx *lift.Context) (string, error) {
    user, err := GetUser(ctx)
    if err != nil {
        return "", err
    }
    return user.UserID, nil
}

// GetTenantID extracts the tenant ID (for multi-tenant support)
func GetTenantID(ctx *lift.Context) string {
    // First try Lift's built-in method
    if tenantID := ctx.TenantID(); tenantID != "" {
        return tenantID
    }
    
    // Fall back to user's tenant
    if user, err := GetUser(ctx); err == nil {
        return user.TenantID
    }
    
    return "default"
}

// GetAccount extracts the Mastodon account
func GetAccount(ctx *lift.Context) (*models.Account, error) {
    if account, ok := ctx.Get(AccountKey).(*models.Account); ok {
        return account, nil
    }
    
    // Try to load from user
    user, err := GetUser(ctx)
    if err != nil {
        return nil, err
    }
    
    // Load account (implement based on your data model)
    return loadAccountForUser(ctx, user)
}

// WithUser adds a user to the context
func WithUser(ctx *lift.Context, user *models.User) {
    ctx.Set(UserKey, user)
    if user.TenantID != "" {
        ctx.Set(TenantKey, user.TenantID)
    }
}

// GetRequestID returns the request ID
func GetRequestID(ctx *lift.Context) string {
    if id, ok := ctx.Get(RequestIDKey).(string); ok {
        return id
    }
    return ctx.RequestID() // Lift's built-in
}

// GetPagination extracts pagination parameters
func GetPagination(ctx *lift.Context) (limit int, sinceID, maxID string) {
    limit = 20 // default
    if l := ctx.QueryParam("limit"); l != "" {
        if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
            limit = parsed
        }
    }
    
    sinceID = ctx.QueryParam("since_id")
    maxID = ctx.QueryParam("max_id")
    return
}

// GetBoolParam gets a boolean query parameter
func GetBoolParam(ctx *lift.Context, name string, defaultValue bool) bool {
    value := ctx.QueryParam(name)
    if value == "" {
        return defaultValue
    }
    return value == "true" || value == "1" || value == "yes"
}
```

**2. Create `/Users/aronprice/lesser/pkg/lift/context/auth.go`:**
```go
package context

import (
    "strings"
    "github.com/pay-theory/lift"
    "github.com/pay-theory/lesser/pkg/auth"
    "github.com/pay-theory/lesser/pkg/models"
)

// RequireAuth is middleware that requires authentication
func RequireAuth(next lift.HandlerFunc) lift.HandlerFunc {
    return func(ctx *lift.Context) error {
        token := extractToken(ctx)
        if token == "" {
            return lift.NewLiftError("UNAUTHORIZED", "Missing authentication token", 401)
        }
        
        // Validate token and load user
        user, err := auth.ValidateToken(ctx.Request.Context(), token)
        if err != nil {
            return lift.NewLiftError("UNAUTHORIZED", "Invalid token", 401)
        }
        
        // Add to context
        WithUser(ctx, user)
        
        return next(ctx)
    }
}

// RequireScopes validates OAuth scopes
func RequireScopes(scopes ...string) lift.Middleware {
    return func(next lift.HandlerFunc) lift.HandlerFunc {
        return func(ctx *lift.Context) error {
            user, err := GetUser(ctx)
            if err != nil {
                return err
            }
            
            // Check scopes (implement based on your OAuth model)
            if !hasRequiredScopes(user, scopes) {
                return lift.NewLiftError("FORBIDDEN", "Insufficient scopes", 403)
            }
            
            return next(ctx)
        }
    }
}

// extractToken gets the bearer token from headers
func extractToken(ctx *lift.Context) string {
    auth := ctx.Request.Header.Get("Authorization")
    if auth == "" {
        return ""
    }
    
    parts := strings.Split(auth, " ")
    if len(parts) != 2 || parts[0] != "Bearer" {
        return ""
    }
    
    return parts[1]
}
```

**3. Create `/Users/aronprice/lesser/pkg/lift/context/testing.go`:**
```go
package context

import (
    "github.com/pay-theory/lift"
    lifttesting "github.com/pay-theory/lift/pkg/testing"
    "github.com/pay-theory/lesser/pkg/models"
)

// TestContextOption is an option for creating test contexts
type TestContextOption func(*lift.Context)

// NewTestContext creates a context for testing
func NewTestContext(opts ...TestContextOption) *lift.Context {
    ctx := lifttesting.NewTestContext()
    
    // Apply options
    for _, opt := range opts {
        opt(ctx)
    }
    
    return ctx
}

// WithTestUser adds a test user to context
func WithTestUser(userID, tenantID string) TestContextOption {
    return func(ctx *lift.Context) {
        user := &models.User{
            UserID:   userID,
            TenantID: tenantID,
            Username: "testuser",
            Email:    "test@example.com",
        }
        WithUser(ctx, user)
    }
}

// WithTestAccount adds a test account
func WithTestAccount(accountID string) TestContextOption {
    return func(ctx *lift.Context) {
        account := &models.Account{
            ID:       accountID,
            Username: "testaccount",
            Domain:   "example.com",
        }
        ctx.Set(AccountKey, account)
    }
}

// WithTestAuth adds test authentication
func WithTestAuth(token string) TestContextOption {
    return func(ctx *lift.Context) {
        ctx.Request.Header.Set("Authorization", "Bearer "+token)
    }
}

// WithQueryParams adds query parameters
func WithQueryParams(params map[string]string) TestContextOption {
    return func(ctx *lift.Context) {
        q := ctx.Request.URL.Query()
        for k, v := range params {
            q.Set(k, v)
        }
        ctx.Request.URL.RawQuery = q.Encode()
    }
}
```

#### Testing Requirements:

**Create `/Users/aronprice/lesser/pkg/lift/context/helpers_test.go`:**
```go
package context

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestGetUser(t *testing.T) {
    // Test with user
    ctx := NewTestContext(WithTestUser("123", "tenant1"))
    
    user, err := GetUser(ctx)
    assert.NoError(t, err)
    assert.Equal(t, "123", user.UserID)
    assert.Equal(t, "tenant1", user.TenantID)
    
    // Test without user
    ctx2 := NewTestContext()
    _, err = GetUser(ctx2)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "not authenticated")
}

func TestGetPagination(t *testing.T) {
    tests := []struct {
        name     string
        params   map[string]string
        wantLimit int
        wantSince string
        wantMax   string
    }{
        {
            name:      "defaults",
            params:    map[string]string{},
            wantLimit: 20,
        },
        {
            name:      "custom limit",
            params:    map[string]string{"limit": "50"},
            wantLimit: 50,
        },
        {
            name:      "limit too high",
            params:    map[string]string{"limit": "200"},
            wantLimit: 20, // Should use default
        },
        {
            name:      "with cursors",
            params:    map[string]string{"since_id": "123", "max_id": "456"},
            wantLimit: 20,
            wantSince: "123",
            wantMax:   "456",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := NewTestContext(WithQueryParams(tt.params))
            
            limit, sinceID, maxID := GetPagination(ctx)
            assert.Equal(t, tt.wantLimit, limit)
            assert.Equal(t, tt.wantSince, sinceID)
            assert.Equal(t, tt.wantMax, maxID)
        })
    }
}

func TestRequireAuth(t *testing.T) {
    handler := RequireAuth(func(ctx *lift.Context) error {
        user, _ := GetUser(ctx)
        return ctx.JSON(200, map[string]string{
            "user_id": user.UserID,
        })
    })
    
    // Test without auth
    ctx := NewTestContext()
    err := handler(ctx)
    assert.Error(t, err)
    
    // Test with valid auth
    ctx2 := NewTestContext(
        WithTestAuth("valid-token"),
        WithTestUser("123", "tenant1"),
    )
    err = handler(ctx2)
    assert.NoError(t, err)
}
```

#### Acceptance Criteria:
- Context helpers extract user/tenant reliably
- Authentication middleware validates tokens
- Pagination helpers handle edge cases
- Test utilities simplify testing

#### Common Pitfalls:
- Don't store sensitive data in context
- Always validate authentication before trusting user data
- Handle missing context values gracefully
- Use consistent key names across the application

### 1.5 Shared Models Migration

#### Tasks:
- [ ] Define DynamORM model patterns
- [ ] Migrate core models (User, Account, Status)
- [ ] Create model factories
- [ ] Implement model validation

#### Implementation Details:

**1. Create `/Users/aronprice/lesser/pkg/models/account.go`:**
```go
package models

import (
    "fmt"
    "time"
    "strings"
    "github.com/pay-theory/dynamorm"
)

// Account represents a Mastodon account (actor in ActivityPub)
type Account struct {
    BaseModel
    
    // DynamoDB keys (single-table design)
    PK string `dynamorm:"pk"` // account#{account_id}
    SK string `dynamorm:"sk"` // account#{account_id}
    
    // GSI for lookups
    Username     string `dynamorm:"index:username-index,pk" json:"username"`
    Domain       string `dynamorm:"index:username-index,sk" json:"domain"` // null for local
    InboxURL     string `dynamorm:"index:inbox-index,pk" json:"inbox_url,omitempty"`
    LastActiveAt string `dynamorm:"index:active-index,pk" json:"-"` // ISO timestamp
    
    // Core fields
    ID            string `json:"id"`
    DisplayName   string `json:"display_name"`
    Note          string `json:"note"` // Bio in HTML
    Avatar        string `json:"avatar"`
    Header        string `json:"header"`
    Locked        bool   `json:"locked"` // Requires follow approval
    Bot           bool   `json:"bot"`
    Discoverable  bool   `json:"discoverable"`
    
    // ActivityPub fields
    ActorURI      string            `json:"uri"`
    URL           string            `json:"url"`
    PublicKey     string            `json:"-"` // PEM format
    PrivateKey    string            `json:"-" dynamorm:"encrypted"` // Only for local accounts
    SharedInbox   string            `json:"shared_inbox_url,omitempty"`
    Endpoints     map[string]string `json:"endpoints,omitempty"`
    
    // Statistics (denormalized for performance)
    FollowersCount int64 `json:"followers_count"`
    FollowingCount int64 `json:"following_count"`
    StatusesCount  int64 `json:"statuses_count"`
    
    // Metadata
    Fields []AccountField `json:"fields,omitempty"`
    Emojis []string      `json:"emojis,omitempty"` // Custom emoji shortcodes
    
    // Federation
    LastWebfinger time.Time `json:"-"`
    LastFetch     time.Time `json:"-"`
}

type AccountField struct {
    Name       string    `json:"name"`
    Value      string    `json:"value"`
    VerifiedAt time.Time `json:"verified_at,omitempty"`
}

// NewLocalAccount creates a new local account
func NewLocalAccount(username string) *Account {
    id := GenerateID()
    now := time.Now()
    
    return &Account{
        PK:           fmt.Sprintf("account#%s", id),
        SK:           fmt.Sprintf("account#%s", id),
        ID:           id,
        Username:     strings.ToLower(username),
        Domain:       "", // Empty for local accounts
        Discoverable: true,
        LastActiveAt: now.Format(time.RFC3339),
        BaseModel: BaseModel{
            CreatedAt: now,
            UpdatedAt: now,
        },
    }
}

// NewRemoteAccount creates a new remote account
func NewRemoteAccount(username, domain string) *Account {
    id := GenerateID()
    now := time.Now()
    
    return &Account{
        PK:           fmt.Sprintf("account#%s", id),
        SK:           fmt.Sprintf("account#%s", id),
        ID:           id,
        Username:     strings.ToLower(username),
        Domain:       strings.ToLower(domain),
        LastActiveAt: now.Format(time.RFC3339),
        BaseModel: BaseModel{
            CreatedAt: now,
            UpdatedAt: now,
        },
    }
}

// IsLocal returns true if this is a local account
func (a *Account) IsLocal() bool {
    return a.Domain == ""
}

// GetWebfingerAcct returns the webfinger account string
func (a *Account) GetWebfingerAcct() string {
    if a.IsLocal() {
        return a.Username
    }
    return fmt.Sprintf("%s@%s", a.Username, a.Domain)
}

// Validate validates the account
func (a *Account) Validate() error {
    if a.Username == "" {
        return ValidationError{Field: "username", Message: "Username is required"}
    }
    
    if !isValidUsername(a.Username) {
        return ValidationError{Field: "username", Message: "Invalid username format"}
    }
    
    if len(a.DisplayName) > 30 {
        return ValidationError{Field: "display_name", Message: "Display name too long"}
    }
    
    if len(a.Note) > 500 {
        return ValidationError{Field: "note", Message: "Bio too long"}
    }
    
    return nil
}
```

**2. Create `/Users/aronprice/lesser/pkg/models/status.go`:**
```go
package models

import (
    "fmt"
    "time"
    "github.com/pay-theory/dynamorm"
)

// Status represents a Mastodon status (toot/post)
type Status struct {
    BaseModel
    
    // DynamoDB keys
    PK string `dynamorm:"pk"` // status#{status_id}
    SK string `dynamorm:"sk"` // status#{status_id}
    
    // GSI for timelines
    AccountID      string `dynamorm:"index:account-timeline,pk" json:"account_id"`
    CreatedAtSort  string `dynamorm:"index:account-timeline,sk" json:"-"` // For sorting
    Visibility     string `dynamorm:"index:public-timeline,pk" json:"visibility"`
    PublishedAt    string `dynamorm:"index:public-timeline,sk" json:"-"` // ISO timestamp
    ConversationID string `dynamorm:"index:conversation-index,pk" json:"conversation_id,omitempty"`
    
    // Core fields
    ID                string    `json:"id"`
    URI               string    `json:"uri"`
    URL               string    `json:"url,omitempty"`
    Content           string    `json:"content"` // HTML
    Text              string    `json:"text"`    // Plain text for search
    InReplyToID       string    `json:"in_reply_to_id,omitempty"`
    InReplyToAccountID string   `json:"in_reply_to_account_id,omitempty"`
    Sensitive         bool      `json:"sensitive"`
    SpoilerText       string    `json:"spoiler_text,omitempty"`
    Language          string    `json:"language,omitempty"`
    
    // Relationships
    Account          *Account  `json:"account,omitempty" dynamorm:"-"`
    ReblogID         string    `json:"reblog_id,omitempty"`
    Reblog           *Status   `json:"reblog,omitempty" dynamorm:"-"`
    Application      *Application `json:"application,omitempty" dynamorm:"-"`
    
    // Engagement metrics
    RepliesCount   int64 `json:"replies_count"`
    ReblogsCount   int64 `json:"reblogs_count"`
    FavouritesCount int64 `json:"favourites_count"`
    
    // User interaction state (populated dynamically)
    Favourited bool `json:"favourited,omitempty" dynamorm:"-"`
    Reblogged  bool `json:"reblogged,omitempty" dynamorm:"-"`
    Muted      bool `json:"muted,omitempty" dynamorm:"-"`
    Bookmarked bool `json:"bookmarked,omitempty" dynamorm:"-"`
    Pinned     bool `json:"pinned,omitempty" dynamorm:"-"`
    
    // Rich content
    MediaAttachments []string          `json:"media_attachments,omitempty"` // Attachment IDs
    Mentions         []Mention         `json:"mentions,omitempty"`
    Tags             []Tag             `json:"tags,omitempty"`
    Emojis           []string          `json:"emojis,omitempty"`
    Poll             *Poll             `json:"poll,omitempty"`
    Card             *PreviewCard      `json:"card,omitempty"`
    
    // Federation
    LocalOnly    bool `json:"local_only,omitempty"`
    Federated    bool `json:"federated"`
    FederatedAt  time.Time `json:"-"`
}

type Mention struct {
    ID       string `json:"id"`
    Username string `json:"username"`
    Acct     string `json:"acct"`
    URL      string `json:"url"`
}

type Tag struct {
    Name string `json:"name"`
    URL  string `json:"url"`
}

// Status visibility levels
const (
    VisibilityPublic   = "public"
    VisibilityUnlisted = "unlisted"
    VisibilityPrivate  = "private"
    VisibilityDirect   = "direct"
)

// NewStatus creates a new status
func NewStatus(accountID string, content string) *Status {
    id := GenerateID()
    now := time.Now()
    timestamp := now.Format(time.RFC3339)
    
    return &Status{
        PK:            fmt.Sprintf("status#%s", id),
        SK:            fmt.Sprintf("status#%s", id),
        ID:            id,
        AccountID:     accountID,
        Content:       content,
        Visibility:    VisibilityPublic,
        CreatedAtSort: fmt.Sprintf("%d#%s", now.Unix(), id),
        PublishedAt:   timestamp,
        Federated:     true,
        BaseModel: BaseModel{
            CreatedAt: now,
            UpdatedAt: now,
        },
    }
}

// Validate validates the status
func (s *Status) Validate() error {
    if s.AccountID == "" {
        return ValidationError{Field: "account_id", Message: "Account ID is required"}
    }
    
    if s.Content == "" && s.MediaAttachments == nil && s.Poll == nil {
        return ValidationError{Field: "content", Message: "Status must have content, media, or poll"}
    }
    
    if len(s.Content) > 5000 {
        return ValidationError{Field: "content", Message: "Content too long"}
    }
    
    if s.Visibility != VisibilityPublic && 
       s.Visibility != VisibilityUnlisted && 
       s.Visibility != VisibilityPrivate && 
       s.Visibility != VisibilityDirect {
        return ValidationError{Field: "visibility", Message: "Invalid visibility"}
    }
    
    return nil
}

// CanBeViewedBy checks if a status can be viewed by an account
func (s *Status) CanBeViewedBy(viewerID string) bool {
    switch s.Visibility {
    case VisibilityPublic, VisibilityUnlisted:
        return true
    case VisibilityPrivate:
        // Check if viewer follows the author
        return s.viewerFollowsAuthor(viewerID)
    case VisibilityDirect:
        // Check if viewer is mentioned
        return s.viewerIsMentioned(viewerID)
    }
    return false
}
```

**3. Create `/Users/aronprice/lesser/pkg/models/factories.go`:**
```go
package models

import (
    "fmt"
    "github.com/google/uuid"
    "github.com/oklog/ulid/v2"
    "math/rand"
    "time"
)

// ID generation
var entropy = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)

// GenerateID creates a new ULID
func GenerateID() string {
    return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// GenerateUUID creates a new UUID
func GenerateUUID() string {
    return uuid.New().String()
}

// Factory functions for testing
type Factory struct{}

var TestFactory = &Factory{}

// NewTestAccount creates a test account
func (f *Factory) NewTestAccount(opts ...func(*Account)) *Account {
    account := NewLocalAccount(fmt.Sprintf("user%d", rand.Intn(10000)))
    account.DisplayName = "Test User"
    account.Note = "Test bio"
    
    for _, opt := range opts {
        opt(account)
    }
    
    return account
}

// NewTestStatus creates a test status
func (f *Factory) NewTestStatus(accountID string, opts ...func(*Status)) *Status {
    status := NewStatus(accountID, "Test status content")
    
    for _, opt := range opts {
        opt(status)
    }
    
    return status
}

// WithUsername sets the username
func WithUsername(username string) func(*Account) {
    return func(a *Account) {
        a.Username = username
    }
}

// WithDomain sets the domain (for remote accounts)
func WithDomain(domain string) func(*Account) {
    return func(a *Account) {
        a.Domain = domain
    }
}

// WithContent sets status content
func WithContent(content string) func(*Status) {
    return func(s *Status) {
        s.Content = content
    }
}

// WithVisibility sets status visibility
func WithVisibility(visibility string) func(*Status) {
    return func(s *Status) {
        s.Visibility = visibility
    }
}

// WithReplyTo sets reply information
func WithReplyTo(statusID, accountID string) func(*Status) {
    return func(s *Status) {
        s.InReplyToID = statusID
        s.InReplyToAccountID = accountID
    }
}
```

**4. Create `/Users/aronprice/lesser/pkg/models/validation.go`:**
```go
package models

import (
    "fmt"
    "regexp"
    "strings"
)

// ValidationError represents a field validation error
type ValidationError struct {
    Field   string
    Message string
}

func (e ValidationError) Error() string {
    return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validation patterns
var (
    usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
    emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
    urlRegex      = regexp.MustCompile(`^https?://`)
)

// Validation functions
func isValidUsername(username string) bool {
    if len(username) < 1 || len(username) > 30 {
        return false
    }
    return usernameRegex.MatchString(username)
}

func isValidEmail(email string) bool {
    return emailRegex.MatchString(email)
}

func isValidURL(url string) bool {
    return urlRegex.MatchString(url)
}

// Sanitization functions
func sanitizeUsername(username string) string {
    // Convert to lowercase and remove invalid characters
    username = strings.ToLower(username)
    return usernameRegex.FindString(username)
}

func sanitizeHTML(html string) string {
    // Implement HTML sanitization
    // For now, strip all tags (implement proper sanitization)
    return stripTags(html)
}

func stripTags(html string) string {
    // Basic tag stripping (use a proper library in production)
    re := regexp.MustCompile(`<[^>]*>`)
    return re.ReplaceAllString(html, "")
}
```

#### Testing Requirements:

**Create `/Users/aronprice/lesser/pkg/models/account_test.go`:**
```go
package models

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestNewLocalAccount(t *testing.T) {
    account := NewLocalAccount("testuser")
    
    assert.NotEmpty(t, account.ID)
    assert.Equal(t, "testuser", account.Username)
    assert.Empty(t, account.Domain)
    assert.True(t, account.IsLocal())
    assert.Equal(t, "testuser", account.GetWebfingerAcct())
    assert.Contains(t, account.PK, "account#")
    assert.Equal(t, account.PK, account.SK)
}

func TestNewRemoteAccount(t *testing.T) {
    account := NewRemoteAccount("remoteuser", "example.com")
    
    assert.NotEmpty(t, account.ID)
    assert.Equal(t, "remoteuser", account.Username)
    assert.Equal(t, "example.com", account.Domain)
    assert.False(t, account.IsLocal())
    assert.Equal(t, "remoteuser@example.com", account.GetWebfingerAcct())
}

func TestAccountValidation(t *testing.T) {
    tests := []struct {
        name    string
        account *Account
        wantErr bool
        errField string
    }{
        {
            name:    "valid account",
            account: NewLocalAccount("validuser"),
            wantErr: false,
        },
        {
            name: "empty username",
            account: &Account{},
            wantErr: true,
            errField: "username",
        },
        {
            name: "invalid username",
            account: &Account{Username: "user@name"},
            wantErr: true,
            errField: "username",
        },
        {
            name: "display name too long",
            account: &Account{
                Username: "validuser",
                DisplayName: strings.Repeat("a", 31),
            },
            wantErr: true,
            errField: "display_name",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.account.Validate()
            if tt.wantErr {
                assert.Error(t, err)
                if ve, ok := err.(ValidationError); ok {
                    assert.Equal(t, tt.errField, ve.Field)
                }
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

#### Acceptance Criteria:
- Models follow DynamORM struct patterns exactly
- All models have proper validation
- Factory functions simplify testing
- GSI fields support required queries

#### Common Pitfalls:
- Don't forget dynamorm tags on key fields
- GSI fields must match infrastructure exactly
- Use consistent timestamp formats (RFC3339)
- Validate all user input

## Phase 1 Completion Checklist

### Pre-Deployment Validation:
- [ ] All unit tests pass
- [ ] Integration tests work with local DynamoDB
- [ ] Build scripts create valid Lambda packages
- [ ] Middleware executes in correct order
- [ ] Cost tracking captures all DB operations
- [ ] Error handling returns appropriate status codes
- [ ] Models validate all required fields
- [ ] Context utilities handle missing data gracefully

### Documentation Required:
- [ ] Migration guide for developers
- [ ] API changes documentation
- [ ] Performance benchmarks
- [ ] Cost analysis comparison

### Rollback Plan:
1. Keep existing code in parallel during migration
2. Use feature flags to toggle between old/new implementations
3. Monitor error rates and performance metrics
4. Have database backup strategy
5. Document all breaking changes

## Next Phases Overview

### Phase 2: API Migration (Week 2)
- Migrate auth endpoints to Lift
- Convert timeline handlers
- Implement streaming with Lift WebSocket support
- Migrate search functionality

### Phase 3: Federation Services (Week 3)
- Convert inbox/outbox to Lift event handlers
- Migrate federation workers
- Implement ActivityPub with DynamORM

### Phase 4: Background Workers (Week 4)
- Convert processors to Lift SQS handlers
- Migrate scheduled tasks
- Implement job queuing with DynamORM

### Phase 5: Optimization (Week 5)
- Performance tuning
- Cost optimization
- Final testing and deployment

## Success Metrics

1. **Performance**: 
   - Lambda cold start < 15ms
   - API response time < 100ms p95
   - Database query time < 10ms p95

2. **Cost**:
   - DynamoDB costs tracked per operation
   - Total cost per user < $0.01/month
   - Lambda execution time reduced by 20%

3. **Reliability**:
   - Error rate < 0.1%
   - All tests passing
   - Zero data loss during migration

4. **Developer Experience**:
   - Reduced boilerplate code by 50%
   - Improved test coverage to > 80%
   - Clear documentation and examples