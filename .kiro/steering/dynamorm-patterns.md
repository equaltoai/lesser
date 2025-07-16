# DynamORM Patterns Guide

This guide provides a comprehensive overview of the core patterns and best practices for using DynamORM in our project. These patterns ensure consistency, maintainability, and optimal performance across our DynamoDB applications.

## Table of Contents

1. [Model Definition](#model-definition)
2. [Basic Operations](#basic-operations)
3. [Query Patterns](#query-patterns)
4. [Multi-Tenant Patterns](#multi-tenant-patterns)
5. [Lambda Optimization](#lambda-optimization)
6. [Testing Strategies](#testing-strategies)
7. [Common Table Patterns](#common-table-patterns)
8. [Troubleshooting](#troubleshooting)

## Model Definition

### Basic Model Structure

```go
// Standard model definition
type User struct {
    // REQUIRED: Every model needs a partition key
    ID        string    `dynamorm:"pk" json:"id"`
    
    // OPTIONAL: Sort key for hierarchical data
    Email     string    `dynamorm:"sk" json:"email"`
    
    // Standard attributes with JSON serialization
    Name      string    `json:"name"`
    Age       int       `json:"age"`
    Active    bool      `json:"active"`
    
    // Custom attribute names in DynamoDB
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
    
    // Global Secondary Index definitions
    Status    string    `dynamorm:"index:status-index,pk" json:"status"`     // GSI partition key
    Region    string    `dynamorm:"index:status-index,sk" json:"region"`     // GSI sort key
}
```

### Struct Tags Reference

#### Primary Key Tags

```go
type Product struct {
    // Partition key (REQUIRED for every model)
    ID       string `dynamorm:"pk" json:"id"`                    // Simple partition key
    
    // Sort key (OPTIONAL - enables hierarchical queries)
    Category string `dynamorm:"sk" json:"category"`              // Compound key with ID
}
```

#### Global Secondary Index (GSI) Tags

```go
type Order struct {
    ID         string    `dynamorm:"pk" json:"id"`                          // Main table partition key
    Timestamp  string    `dynamorm:"sk" json:"timestamp"`                   // Main table sort key
    
    // GSI definition: index-name,key-type
    CustomerID string    `dynamorm:"index:customer-index,pk" json:"customer_id"`     // GSI partition key
    Status     string    `dynamorm:"index:customer-index,sk" json:"status"`          // GSI sort key
    
    // Multiple GSIs on same field
    Amount     int64     `dynamorm:"index:amount-index,pk" json:"amount"`            // Different GSI
}
```

### Advanced Model Patterns

#### Embedded Structs

```go
// Base model with common fields
type BaseModel struct {
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
    Version   int       `dynamorm:"version" json:"version"`
}

type User struct {
    ID       string `dynamorm:"pk" json:"id"`
    Email    string `dynamorm:"sk" json:"email"`
    
    BaseModel  // Embedded fields are automatically recognized
    
    Name     string `json:"name"`
    Active   bool   `json:"active"`
}
```

## Basic Operations

### Initializing DynamORM

```go
// Standard initialization
config := session.Config{
    Region: "us-east-1",
}

db, err := dynamorm.New(config)
if err != nil {
    log.Fatal("Failed to initialize DynamORM:", err)
}

// For local development
config := session.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000",  // DynamoDB Local
    AccessKeyID:     "fakeMyKeyId",
    SecretAccessKey: "fakeSecretAccessKey",
}
```

### Create Operations

```go
// Create a new item
user := &User{
    ID:        "user123",
    Email:     "john@example.com",
    Name:      "John Doe",
    Age:       30,
    Active:    true,
    CreatedAt: time.Now(),
    Status:    "active",
}

err := db.Model(user).Create()
if err != nil {
    return fmt.Errorf("failed to create user: %w", err)
}
```

### Read Operations

```go
// Get item by primary key
var user User
err := db.Model(&User{}).
    Where("ID", "=", "user123").
    First(&user)
if err != nil {
    return fmt.Errorf("failed to find user: %w", err)
}

// Query multiple items
var users []User
err = db.Model(&User{}).
    Where("Status", "=", "active").   // Uses status-index automatically
    Limit(10).
    All(&users)
if err != nil {
    return fmt.Errorf("failed to query users: %w", err)
}
```

### Update Operations

```go
// Update an existing item
var user User
err := db.Model(&User{}).
    Where("ID", "=", userID).
    First(&user)
if err != nil {
    return fmt.Errorf("user not found: %w", err)
}

// Update fields
user.Name = "John Smith"
user.Age = 31

// Save changes
err = db.Model(&user).Update()
if err != nil {
    return fmt.Errorf("failed to update user: %w", err)
}

// Conditional update
err = db.Model(&User{}).
    Where("ID", "=", userID).
    Where("Age", "=", expectedAge).  // Condition for update
    Set("Name", "John Smith").
    Set("Age", 31).
    UpdateFields()
if err != nil {
    return fmt.Errorf("conditional update failed: %w", err)
}
```

### Delete Operations

```go
// Delete an item
user := &User{ID: userID}
err := db.Model(user).Delete()
if err != nil {
    return fmt.Errorf("failed to delete user: %w", err)
}

// Conditional delete
err = db.Model(&User{}).
    Where("ID", "=", userID).
    Where("Active", "=", false).  // Only delete inactive users
    Delete()
if err != nil {
    return fmt.Errorf("conditional delete failed: %w", err)
}
```

## Query Patterns

### Basic Queries

```go
// Query by primary key
var user User
err := db.Model(&User{}).
    Where("ID", "=", "user123").
    First(&user)

// Query with sort key
var note Note
err = db.Model(&Note{}).
    Where("UserID", "=", "user123").  // Partition key
    Where("NoteID", "=", "note456").  // Sort key
    First(&note)
```

### GSI Queries

```go
// Query using GSI
var users []User
err := db.Model(&User{}).
    Index("status-index").           // Use specific GSI
    Where("Status", "=", "active").  // GSI partition key
    Where("Region", "=", "us-east"). // GSI sort key
    All(&users)

// Query with sorting
var orders []Order
err = db.Model(&Order{}).
    Index("customer-index").
    Where("CustomerID", "=", "cust123").
    OrderBy("Timestamp", "DESC").    // Sort by sort key
    Limit(10).                       // Limit results
    All(&orders)
```

### Advanced Query Patterns

```go
// Query with pagination
var users []User
var lastEvaluatedKey map[string]interface{}

err := db.Model(&User{}).
    Index("status-index").
    Where("Status", "=", "active").
    Limit(10).
    AllWithLastEvaluatedKey(&users, &lastEvaluatedKey)

// Continue from last query
if len(lastEvaluatedKey) > 0 {
    err = db.Model(&User{}).
        Index("status-index").
        Where("Status", "=", "active").
        Limit(10).
        StartFromKey(lastEvaluatedKey).
        All(&moreUsers)
}

// Query with projection (only fetch specific attributes)
var userNames []string
err = db.Model(&User{}).
    Index("status-index").
    Where("Status", "=", "active").
    Project("Name").  // Only fetch Name field
    All(&userNames)
```

## Multi-Tenant Patterns

### Tenant-Isolated Data

```go
type TenantUser struct {
    // Composite keys for tenant isolation
    TenantID   string `dynamorm:"pk" json:"tenant_id"`        // Tenant partition
    UserID     string `dynamorm:"sk" json:"user_id"`          // User within tenant
    
    // GSI for user lookup across tenants (if needed)
    Email      string `dynamorm:"index:email-index,pk" json:"email"`
    
    Name       string `json:"name"`
    Role       string `json:"role"`
}

// Query all users for a tenant
users, err := db.Model(&TenantUser{}).
    Where("TenantID", "=", tenantID).
    All(&users)

// Query specific user in tenant
var user TenantUser
err = db.Model(&TenantUser{}).
    Where("TenantID", "=", tenantID).
    Where("UserID", "=", userID).
    First(&user)

// Query across tenants by email
var usersByEmail []TenantUser
err = db.Model(&TenantUser{}).
    Index("email-index").
    Where("Email", "=", email).
    All(&usersByEmail)
```

### Multi-Tenant Service Pattern

```go
// Service with tenant context
type UserService struct {
    db       *dynamorm.DB
    tenantID string
}

func NewUserService(db *dynamorm.DB, tenantID string) *UserService {
    return &UserService{
        db:       db,
        tenantID: tenantID,
    }
}

func (s *UserService) CreateUser(user *TenantUser) error {
    // Ensure tenant isolation
    user.TenantID = s.tenantID
    
    return s.db.Model(user).Create()
}

func (s *UserService) GetUser(userID string) (*TenantUser, error) {
    var user TenantUser
    err := s.db.Model(&TenantUser{}).
        Where("TenantID", "=", s.tenantID).
        Where("UserID", "=", userID).
        First(&user)
    
    if err != nil {
        return nil, err
    }
    
    return &user, nil
}

func (s *UserService) ListUsers() ([]TenantUser, error) {
    var users []TenantUser
    err := s.db.Model(&TenantUser{}).
        Where("TenantID", "=", s.tenantID).
        All(&users)
    
    return users, err
}
```

## Lambda Optimization

### Lambda Initialization Pattern

```go
// Global variable for connection reuse
var db *dynamorm.LambdaDB

func init() {
    // CRITICAL: Initialize once, reuse across invocations
    // This reduces cold start time by 91%
    var err error
    db, err = dynamorm.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
    
    // Pre-register models to reduce cold start time
    if err := db.PreRegisterModels(&User{}, &Order{}, &Product{}); err != nil {
        panic(err)
    }
}

// Alternative: Use LambdaInit helper
// func init() {
//     var err error
//     db, err = dynamorm.LambdaInit(&User{}, &Order{}, &Product{})
//     if err != nil {
//         panic(err)
//     }
// }

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Use the pre-initialized db
    var user User
    err := db.Model(&User{}).
        Where("ID", "=", request.PathParameters["id"]).
        First(&user)
    
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 404,
            Body:       `{"error": "User not found"}`,
        }, nil
    }
    
    response, _ := json.Marshal(user)
    return events.APIGatewayProxyResponse{
        StatusCode: 200,
        Body:       string(response),
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
    }, nil
}
```

### Lambda Timeout Handling

```go
func init() {
    var err error
    db, err = dynamorm.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
    
    // Set timeout buffer to prevent Lambda timeouts
    db = db.WithLambdaTimeoutBuffer(500 * time.Millisecond)
}
```

## Testing Strategies

### Interface-Based Design for Testing

```go
// Use interfaces in your business logic
package services

import "github.com/pay-theory/dynamorm/pkg/core"

// PaymentService uses interface - can be mocked
type PaymentService struct {
    db core.DB  // Interface, not concrete type
}

func NewPaymentService(db core.DB) *PaymentService {
    return &PaymentService{db: db}
}

func (s *PaymentService) CreatePayment(payment *Payment) error {
    // Validate payment
    if payment.Amount <= 0 {
        return errors.New("amount must be positive")
    }
    
    // Business logic
    payment.Status = "pending"
    payment.CreatedAt = time.Now()
    
    // Database operation - mockable through interface
    return s.db.Model(payment).Create()
}
```

### Unit Testing with Mocks

```go
// payment_service_test.go
package services

import (
    "testing"
    "time"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/pay-theory/dynamorm/pkg/mocks"
)

func TestCreatePayment_Success(t *testing.T) {
    // Set up mocks
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    // Configure mock expectations
    mockDB.On("Model", mock.AnythingOfType("*Payment")).Return(mockQuery)
    mockQuery.On("Create").Return(nil)
    
    // Test the service
    service := NewPaymentService(mockDB)
    payment := &Payment{
        ID:     "pay123",
        UserID: "user456",
        Amount: 1000,
    }
    
    err := service.CreatePayment(payment)
    
    // Verify results
    assert.NoError(t, err)
    assert.Equal(t, "pending", payment.Status)
    assert.False(t, payment.CreatedAt.IsZero())
    
    // Verify mock expectations were met
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

### Integration Testing

```go
// integration_test.go - Test with real DynamoDB Local
package services

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/suite"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

type PaymentIntegrationSuite struct {
    suite.Suite
    db      *dynamorm.DB
    service *PaymentService
}

func (suite *PaymentIntegrationSuite) SetupSuite() {
    // Use DynamoDB Local for integration tests
    config := session.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000",  // DynamoDB Local
        // Use fake credentials for local testing
        AccessKeyID:     "fakeMyKeyId",
        SecretAccessKey: "fakeSecretAccessKey",
    }
    
    var err error
    suite.db, err = dynamorm.New(config)
    suite.Require().NoError(err)
    
    // Create test table
    err = suite.db.CreateTable(&Payment{})
    suite.Require().NoError(err)
    
    suite.service = NewPaymentService(suite.db)
}

func (suite *PaymentIntegrationSuite) TearDownSuite() {
    // Clean up test table
    suite.db.DeleteTable(&Payment{})
}

func (suite *PaymentIntegrationSuite) SetupTest() {
    // Clean data before each test
    suite.clearPayments()
}

func (suite *PaymentIntegrationSuite) TestCreateAndRetrievePayment() {
    // Create payment
    payment := &Payment{
        ID:     "pay123",
        UserID: "user456",
        Amount: 1000,
    }
    
    err := suite.service.CreatePayment(payment)
    suite.NoError(err)
    
    // Retrieve payment
    var retrieved Payment
    err = suite.db.Model(&Payment{}).
        Where("ID", "=", "pay123").
        First(&retrieved)
    
    suite.NoError(err)
    suite.Equal(payment.ID, retrieved.ID)
    suite.Equal(payment.UserID, retrieved.UserID)
    suite.Equal(payment.Amount, retrieved.Amount)
    suite.Equal("pending", retrieved.Status)
}
```

## Common Table Patterns

### User Management

```go
// User entity with email index
type User struct {
    ID        string    `dynamorm:"pk" json:"id"`
    Email     string    `dynamorm:"index:email-index,pk" json:"email"`
    Name      string    `json:"name"`
    Status    string    `dynamorm:"index:status-index,pk" json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// User session for auth
type UserSession struct {
    UserID    string    `dynamorm:"pk" json:"user_id"`
    SessionID string    `dynamorm:"sk" json:"session_id"`
    ExpiresAt time.Time `json:"expires_at"`
    CreatedAt time.Time `json:"created_at"`
}

// User activity log
type UserActivity struct {
    UserID    string    `dynamorm:"pk" json:"user_id"`
    Timestamp string    `dynamorm:"sk" json:"timestamp"`
    Action    string    `dynamorm:"index:action-index,pk" json:"action"`
    Details   map[string]interface{} `json:"details"`
}
```

### E-commerce System

```go
// Product catalog
type Product struct {
    ID          string  `dynamorm:"pk" json:"id"`
    SKU         string  `dynamorm:"sk" json:"sku"`
    Category    string  `dynamorm:"index:category-index,pk" json:"category"`
    Price       int64   `dynamorm:"index:category-index,sk" json:"price"`
    Name        string  `json:"name"`
    Description string  `json:"description"`
    InStock     bool    `json:"in_stock"`
    CreatedAt   time.Time `json:"created_at"`
}

// Customer orders
type Order struct {
    ID         string    `dynamorm:"pk" json:"id"`
    OrderNum   string    `dynamorm:"sk" json:"order_number"`
    CustomerID string    `dynamorm:"index:customer-index,pk" json:"customer_id"`
    Status     string    `dynamorm:"index:customer-index,sk" json:"status"`
    Total      int64     `json:"total"`
    Items      []OrderItem `json:"items"`
    CreatedAt  time.Time `json:"created_at"`
}

type OrderItem struct {
    ProductID string `json:"product_id"`
    Quantity  int    `json:"quantity"`
    Price     int64  `json:"price"`
}
```

### Time-Series Data

```go
// Time-series pattern for metrics
type Metric struct {
    // Partition by entity (user, system, etc.)
    EntityID  string    `dynamorm:"pk" json:"entity_id"`
    
    // Sort by timestamp for time-range queries
    Timestamp string    `dynamorm:"sk" json:"timestamp"`  // Use ISO format
    
    // GSI for metric type queries
    MetricType string   `dynamorm:"index:metric-index,pk" json:"metric_type"`
    
    Value     float64   `json:"value"`
    Unit      string    `json:"unit"`
    Tags      map[string]string `json:"tags"`
}

// Query time range for entity
metrics, err := db.Model(&Metric{}).
    Where("EntityID", "=", "server123").
    Where("Timestamp", "BETWEEN", startTime, endTime).
    All(&metrics)

// Query by metric type across entities
cpuMetrics, err := db.Model(&Metric{}).
    Index("metric-index").
    Where("MetricType", "=", "cpu").
    All(&cpuMetrics)
```

## Troubleshooting

### Common Error Messages and Solutions

#### ValidationException: One or more parameter values were invalid

**Problem:** Your struct definition doesn't match the DynamoDB table schema.

**Solution:**
```go
// Verify your struct tags match table schema
type User struct {
    ID    string `dynamorm:"pk"`     // Must match table's partition key
    Email string `dynamorm:"sk"`     // Must match table's sort key (if exists)
    Name  string `json:"name"`       // Regular attribute
}

// Check your table schema:
// aws dynamodb describe-table --table-name users
```

#### ResourceNotFoundException: Requested resource not found

**Problem:** Table doesn't exist or you're using the wrong table name.

**Solution:**
```go
// Create table from model (development only)
err := db.CreateTable(&User{})
if err != nil {
    log.Printf("Failed to create table: %v", err)
}

// Override table name if needed
func (User) TableName() string {
    return "custom_users"  // Use different table name
}
```

#### Query operation: Query cost is too high

**Problem:** Your query is scanning the entire table instead of using an index.

**Solution:**
```go
// Use proper index for the query
type User struct {
    ID     string `dynamorm:"pk"`
    Email  string `dynamorm:"sk"`
    Age    int    `dynamorm:"index:age-index,pk"`  // Create GSI for age queries
    Status string `dynamorm:"index:age-index,sk"`  // Sort by status
}

// Now query efficiently:
var users []User
err := db.Model(&User{}).
    Index("age-index").           // Use the index
    Where("Age", "=", 25).        // Exact match on partition key
    Where("Status", "=", "active"). // Filter on sort key
    All(&users)
```

#### Cold start timeouts in Lambda

**Problem:** Lambda function timing out on cold starts.

**Solution:**
```go
// Initialize once, reuse across invocations
var db *dynamorm.LambdaDB

func init() {
    // This runs once per Lambda container
    var err error
    db, err = dynamorm.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
}
```

### Performance Issues

#### Slow Query Performance

**Solutions:**
```go
// 1. Use proper indexes
type User struct {
    ID     string `dynamorm:"pk"`
    Email  string `dynamorm:"sk"`
    Status string `dynamorm:"index:status-index,pk"`  // Index for status queries
}

// 2. Use pagination for large result sets
var users []User
err := db.Model(&User{}).
    Index("status-index").
    Where("Status", "=", "active").
    Limit(100).  // Limit results
    All(&users)

// 3. Use projection to reduce data transfer
var usernames []string
err := db.Model(&User{}).
    Index("status-index").
    Where("Status", "=", "active").
    Project("Name").  // Only fetch Name field
    All(&usernames)
```

## Best Practices

1. **Use composite keys for clear entity identification**:
   - `pk`: `entity_type#{id}` (e.g., `user#123`, `tenant#abc`)
   - `sk`: Depends on access pattern (could be same as pk or hierarchical)

2. **Design indexes based on access patterns**:
   - Use GSIs for alternative query patterns
   - Keep GSI keys meaningful and efficient

3. **Initialize DynamORM once in Lambda functions**:
   - Use global variables and init() function
   - Use NewLambdaOptimized() for best performance

4. **Use interface-based design for testability**:
   - Accept core.DB interface instead of concrete types
   - Use mocks for unit testing

5. **Handle multi-tenancy at the data layer**:
   - Include tenant ID in composite keys
   - Use tenant-specific GSIs for efficient queries

6. **Use batch operations for efficiency**:
   ```go
   items := []interface{}{user1, user2, user3}
   err := db.BatchWrite().Add(items...).Execute()
   ```

7. **Implement proper error handling**:
   ```go
   if err := db.Model(user).Create(); err != nil {
       return fmt.Errorf("failed to create user: %w", err)
   }
   ```

8. **Use pagination for large result sets**:
   ```go
   var lastEvaluatedKey map[string]interface{}
   err := db.Model(&User{}).
       Limit(100).
       AllWithLastEvaluatedKey(&users, &lastEvaluatedKey)
   ```

9. **Use transactions for atomic operations**:
   ```go
   err := db.Transaction(func(tx *dynamorm.Tx) error {
       // Multiple operations in a transaction
       if err := tx.Model(account1).Update(); err != nil {
           return err
       }
       return tx.Model(account2).Update()
   })
   ```

10. **Use TTL for temporary data**:
    ```go
    session := &UserSession{
        UserID:    userID,
        SessionID: sessionID,
        ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),  // TTL value
    }
    ```

By following these patterns and best practices, you'll build efficient, maintainable, and scalable applications with DynamORM and DynamoDB.