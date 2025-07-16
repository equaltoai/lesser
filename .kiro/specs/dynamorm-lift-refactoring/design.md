# Design Document: DynamORM and Lift Integration

## Overview

This design outlines the approach for refactoring Lesser to use the DynamORM and Lift libraries from the reference folder. The refactoring will be implemented incrementally, starting with core packages and gradually extending to Lambda functions. The design focuses on maintaining compatibility with existing functionality while improving code quality, reducing boilerplate, and optimizing performance.

## Architecture

### Current Architecture

Lesser currently uses:
- Direct AWS SDK calls for DynamoDB operations
- Custom Lambda handling code for different event sources
- Manual JSON parsing and validation
- Custom error handling and logging

### Target Architecture

After refactoring, Lesser will use:
- DynamORM for all DynamoDB operations
- Lift framework for Lambda handlers
- Standardized request parsing and validation
- Unified error handling and logging

### High-Level Changes

1. **Storage Layer Refactoring**:
   - Replace direct AWS SDK calls with DynamORM
   - Refactor data models to use DynamORM struct tags with standard `pk` and `sk` keys
   - Implement DynamORM's query builder pattern for data access

2. **Lambda Handler Refactoring**:
   - Replace custom Lambda handlers with Lift framework's canonical pattern
   - Implement Lift's middleware for cross-cutting concerns
   - Use Lift's Context for request/response handling

3. **Integration Points**:
   - Ensure DynamORM and Lift work together seamlessly
   - Maintain compatibility with existing code during transition

## Components and Interfaces

### DynamORM Integration

#### Core Data Models

All data models in `pkg/storage` will be refactored to use DynamORM struct tags following the standard pattern:

```go
// Current implementation
type User struct {
    ID        string `json:"id"`
    Email     string `json:"email"`
    Name      string `json:"name"`
    CreatedAt int64  `json:"created_at"`
}

// Refactored implementation
type User struct {
    // Primary keys using standard naming
    PK        string    `dynamorm:"pk" json:"id"`           // user#{id}
    SK        string    `dynamorm:"sk" json:"id"`           // user#{id}
    
    // GSI attributes
    Email     string    `dynamorm:"index:email-index,pk" json:"email"`
    
    // Standard attributes
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

For multi-tenant models, we'll follow the tenant-isolated data pattern:

```go
type TenantResource struct {
    // Composite keys for tenant isolation
    PK        string    `dynamorm:"pk" json:"tenant_id"`    // tenant#{tenant_id}
    SK        string    `dynamorm:"sk" json:"resource_id"`  // resource#{resource_id}
    
    // GSI for cross-tenant queries
    ResourceType string  `dynamorm:"index:type-index,pk" json:"resource_type"`
    CreatedAt    string  `dynamorm:"index:type-index,sk" json:"created_at"`
    
    // Tenant-specific GSI
    TenantID     string  `dynamorm:"index:tenant-index,pk" json:"tenant_id"`
    
    // Business data
    Name         string  `json:"name"`
    Description  string  `json:"description"`
}
```

#### Repository Pattern

The existing repository pattern will be maintained but implemented using DynamORM's query builder:

```go
// Current implementation
func (r *UserRepository) GetUser(id string) (*User, error) {
    input := &dynamodb.GetItemInput{
        TableName: aws.String(r.tableName),
        Key: map[string]*dynamodb.AttributeValue{
            "id": {S: aws.String(id)},
        },
    }
    result, err := r.client.GetItem(input)
    // Error handling and unmarshaling...
}

// Refactored implementation
func (r *UserRepository) GetUser(id string) (*User, error) {
    user := &User{}
    err := r.db.Model(user).
        Where("PK", "=", fmt.Sprintf("user#%s", id)).
        Where("SK", "=", fmt.Sprintf("user#%s", id)).
        First(user)
        
    if err != nil {
        if errors.Is(err, dynamorm.ErrNotFound) {
            return nil, ErrUserNotFound
        }
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    
    return user, nil
}
```

#### Lambda Optimization

Lambda functions will use DynamORM's Lambda-optimized initialization pattern:

```go
// Global variable for connection reuse
var db *dynamorm.LambdaDB

func init() {
    // CRITICAL: Initialize once, reuse across invocations
    var err error
    db, err = dynamorm.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
    
    // Pre-register models to reduce cold start time
    if err := db.PreRegisterModels(&User{}, &Order{}, &Product{}); err != nil {
        panic(err)
    }
    
    // Set timeout buffer to prevent Lambda timeouts
    db = db.WithLambdaTimeoutBuffer(500 * time.Millisecond)
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Use pre-initialized db...
}
```

### Lift Integration

#### Lambda Handler Structure

Lambda functions will be refactored to use Lift's canonical application structure:

```go
// Current implementation
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Parse request
    var req Request
    json.Unmarshal([]byte(event.Body), &req)
    
    // Process request
    result, err := processRequest(req)
    if err != nil {
        return events.APIGatewayProxyResponse{
            StatusCode: 500,
            Body: `{"error": "Internal server error"}`,
        }, nil
    }
    
    // Return response
    resp, _ := json.Marshal(result)
    return events.APIGatewayProxyResponse{
        StatusCode: 200,
        Body: string(resp),
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
    }, nil
}

// Refactored implementation
func main() {
    app := lift.New()
    
    // Configure middleware
    app.Use(middleware.RequestID())
    app.Use(middleware.Logger())
    app.Use(middleware.Recover())
    
    // Configure routes
    app.GET("/health", healthHandler)
    app.POST("/users", createUserHandler)
    
    // Start the Lambda handler
    // CRITICAL: Use app.HandleRequest, NOT app.Start()
    lambda.Start(app.HandleRequest)
}

func createUserHandler(ctx *lift.Context) error {
    var req CreateUserRequest
    
    // Standard parsing - works identically for v1 and v2
    if err := ctx.ParseRequest(&req); err != nil {
        // Lift returns structured errors automatically
        return err
    }
    
    // Process the request
    user, err := userService.CreateUser(req)
    if err != nil {
        return mapErrorToLiftError(err)
    }
    
    // Return JSON response
    return ctx.JSON(user)
}
```

#### Multi-Event Handler Pattern

For Lambda functions that need to handle multiple event types:

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
```

#### Request Validation

Request validation will use Lift's automatic validation via struct tags:

```go
// Current implementation
type Request struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

func validateRequest(req Request) error {
    if req.Name == "" {
        return errors.New("name is required")
    }
    if req.Age < 0 {
        return errors.New("age must be positive")
    }
    return nil
}

// Refactored implementation
type Request struct {
    Name string `json:"name" validate:"required"`
    Age  int    `json:"age" validate:"min=0"`
}

// Validation is automatic with Lift's ctx.ParseRequest
```

#### Error Handling

Error handling will use Lift's standardized error types:

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

## Data Models

### DynamoDB Table Design

All tables will follow the standard Lift table structure with `pk` and `sk` keys:

```go
// CDK Table Creation
liftTable := constructs.NewLiftTable(stack, jsii.String("MyTable"), &constructs.LiftTableProps{
    TableName:           jsii.String("my-app-table"),
    TimeToLiveAttribute: jsii.String("ttl"),
    EnableStreams:       jsii.Bool(true),
})

// Add GSIs using the underlying DynamoDB table
liftTable.Table.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
    IndexName: jsii.String("email-index"),
    PartitionKey: &awsdynamodb.Attribute{
        Name: jsii.String("Email"),
        Type: awsdynamodb.AttributeType_STRING,
    },
})
```

### Data Access Patterns

DynamORM's query builder will be used for all data access patterns, following the recommended patterns:

```go
// Get item by primary key
note, err := db.Model(&Note{}).
    Where("PK", "=", fmt.Sprintf("note#%s", noteID)).
    Where("SK", "=", fmt.Sprintf("note#%s", noteID)).
    First(&note)

// Query by GSI
var notes []Note
err := db.Model(&Note{}).
    Index("timeline-index").
    Where("TimelineID", "=", timelineID).
    Where("Timestamp", ">", startTime).
    OrderBy("Timestamp", "DESC").
    Limit(10).
    All(&notes)

// Batch operations for efficiency
err := db.BatchGet().
    Add(&Note{PK: "note#note1", SK: "note#note1"}).
    Add(&Note{PK: "note#note2", SK: "note#note2"}).
    Execute()
```

### Multi-Tenant Data Access

For multi-tenant data access, we'll follow the tenant-isolated pattern:

```go
// Query all resources for a tenant
resources, err := db.Model(&TenantResource{}).
    Where("PK", "=", fmt.Sprintf("tenant#%s", tenantID)).
    All(&resources)

// Query specific resource in tenant
var resource TenantResource
err = db.Model(&TenantResource{}).
    Where("PK", "=", fmt.Sprintf("tenant#%s", tenantID)).
    Where("SK", "=", fmt.Sprintf("resource#%s", resourceID)).
    First(&resource)

// Query across tenants by type (admin access)
var resourcesByType []TenantResource
err = db.Model(&TenantResource{}).
    Index("type-index").
    Where("ResourceType", "=", resourceType).
    All(&resourcesByType)
```

## Error Handling

### Standardized Error Types

Lift provides standardized error types that will be used throughout the application:

- `lift.ValidationError`: For input validation errors
- `lift.NotFound`: For resource not found errors
- `lift.Unauthorized`: For authentication errors
- `lift.Forbidden`: For authorization errors
- `lift.NewLiftError`: For custom error types

### Error Mapping

Errors from DynamORM will be mapped to Lift error types:

```go
err := db.Model(&User{}).Where("PK", "=", fmt.Sprintf("user#%s", id)).First(&user)
if err != nil {
    switch {
    case errors.Is(err, dynamorm.ErrNotFound):
        return lift.NotFound("User not found")
    case errors.Is(err, dynamorm.ErrValidation):
        return lift.ValidationError(err.Error())
    default:
        return lift.NewLiftError("DATABASE_ERROR", "Database operation failed", 500)
    }
}
```

## Testing Strategy

### Interface-Based Design for Testing

We'll use interface-based design for better testability:

```go
// Define interfaces for repositories
type UserRepository interface {
    GetUser(id string) (*User, error)
    CreateUser(user *User) error
    UpdateUser(user *User) error
    DeleteUser(id string) error
}

// Implement repository with DynamORM
type DynamoUserRepository struct {
    db        core.DB  // Interface, not concrete type
    tableName string
}

func NewDynamoUserRepository(db core.DB, tableName string) *DynamoUserRepository {
    return &DynamoUserRepository{
        db:        db,
        tableName: tableName,
    }
}

// Service layer uses repository interface
type UserService struct {
    repo UserRepository
}

func NewUserService(repo UserRepository) *UserService {
    return &UserService{
        repo: repo,
    }
}
```

### Unit Testing with Mocks

Unit tests will use DynamORM's mocking capabilities:

```go
func TestUserService_GetUser(t *testing.T) {
    // Set up mocks
    mockRepo := new(mocks.MockUserRepository)
    
    // Configure mock expectations
    mockRepo.On("GetUser", "user123").Return(&User{
        PK:   "user#user123",
        SK:   "user#user123",
        Name: "John Doe",
    }, nil)
    
    // Create service with mock repository
    service := NewUserService(mockRepo)
    
    // Test the service
    user, err := service.GetUser("user123")
    
    // Verify results
    assert.NoError(t, err)
    assert.Equal(t, "John Doe", user.Name)
    
    // Verify mock expectations were met
    mockRepo.AssertExpectations(t)
}
```

### Integration Testing

Integration tests will use DynamoDB Local for testing DynamORM operations:

```go
type UserRepositoryIntegrationSuite struct {
    suite.Suite
    db        *dynamorm.DB
    repo      *DynamoUserRepository
    tableName string
}

func (suite *UserRepositoryIntegrationSuite) SetupSuite() {
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
    suite.tableName = "test_users"
    err = suite.db.CreateTable(&User{})
    suite.Require().NoError(err)
    
    suite.repo = NewDynamoUserRepository(suite.db, suite.tableName)
}

func (suite *UserRepositoryIntegrationSuite) TearDownSuite() {
    // Clean up test table
    suite.db.DeleteTable(&User{})
}
```

## Migration Strategy

### Phased Approach

The refactoring will be implemented in phases:

1. **Phase 1**: Core packages
   - Refactor `pkg/storage` to use DynamORM
   - Create adapters for backward compatibility

2. **Phase 2**: Lambda functions
   - Refactor one Lambda function as a proof of concept
   - Validate performance and functionality
   - Refactor remaining Lambda functions

3. **Phase 3**: Integration
   - Ensure all components work together
   - Remove adapters and legacy code

### Backward Compatibility

During the migration, backward compatibility will be maintained through adapters:

```go
// Adapter for backward compatibility
type LegacyRepository struct {
    newRepo *DynamoUserRepository
}

func (r *LegacyRepository) GetUser(id string) (*User, error) {
    // Call new repository with legacy interface
    return r.newRepo.GetUser(id)
}
```

## Performance Considerations

### Cold Start Optimization

Lambda cold starts will be optimized using:
- DynamORM's Lambda-optimized initialization pattern
- Pre-registering models to reduce cold start time
- Setting timeout buffer to prevent Lambda timeouts
- Lift's minimal overhead

### Memory Usage

Memory usage will be optimized using:
- DynamORM's efficient marshaling/unmarshaling
- Lift's minimal memory allocations
- Proper resource cleanup
- Pagination for large result sets

### Query Optimization

DynamoDB queries will be optimized using:
- Proper index selection based on access patterns
- Composite keys for efficient queries
- Batch operations where appropriate
- Projection to reduce data transfer

## Security Considerations

### Input Validation

Input validation will be enhanced using:
- Lift's automatic validation via struct tags
- Consistent validation across all endpoints

### Error Sanitization

Error sanitization will be improved using:
- Lift's standardized error types
- Proper error mapping
- No leakage of internal errors

## Conclusion

This refactoring will significantly improve the Lesser codebase by:
- Reducing boilerplate code with DynamORM and Lift
- Improving type safety and compile-time error checking
- Optimizing Lambda performance and cold starts
- Standardizing error handling and logging
- Enhancing testability with interface-based design and mocking capabilities

The phased approach ensures that the refactoring can be implemented incrementally with minimal disruption to the existing functionality.