# Lift Patterns Guide

This guide provides a comprehensive overview of the core patterns and best practices for using the Lift framework in our project. These patterns ensure consistency, maintainability, and optimal performance across our serverless applications.

## Table of Contents

1. [Lambda Handler Initialization](#lambda-handler-initialization)
2. [API Gateway JSON Parsing](#api-gateway-json-parsing)
3. [Multi-tenant Request Handling](#multi-tenant-request-handling)
4. [DynamoDB Table Design](#dynamodb-table-design)
5. [DynamORM Integration](#dynamorm-integration)
6. [WebSocket API Implementation](#websocket-api-implementation)
7. [Common Table Patterns](#common-table-patterns)
8. [Best Practices](#best-practices)

## Lambda Handler Initialization

### Canonical Pattern

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
)

func main() {
    // Step 1: Create the Lift application
    app := lift.New()
    
    // Step 2: Configure your routes
    app.GET("/health", healthHandler)
    app.POST("/users", createUserHandler)
    
    // Step 3: Start the Lambda handler
    // CRITICAL: Use app.HandleRequest, NOT app.Start()
    lambda.Start(app.HandleRequest)
}
```

### Key Points

- Always use `lambda.Start(app.HandleRequest)` for Lambda functions
- Never use `app.Start()` in Lambda functions (only for local development)
- Avoid creating custom handler wrappers around `app.HandleRequest`
- Lift automatically detects and routes different AWS event types

### Multi-Event Handler Pattern

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
    
    // EventBridge handler
    app.EventBridge("user-signup", userSignupHandler)
    
    // ONE lambda.Start call handles ALL event types
    lambda.Start(app.HandleRequest)
}
```

## API Gateway JSON Parsing

### Standard Pattern

```go
func createUserHandler(ctx *lift.Context) error {
    var req CreateUserRequest
    
    // Standard parsing - works identically for v1 and v2
    if err := ctx.ParseRequest(&req); err != nil {
        // Lift returns structured errors automatically
        return err
    }
    
    // Process the request
    user := processUser(req)
    
    // Return JSON response
    return ctx.JSON(user)
}
```

### Handling Empty Bodies

```go
func flexibleHandler(ctx *lift.Context) error {
    // Check if body exists before parsing
    if len(ctx.Request.Body) == 0 {
        // Handle empty body case explicitly
        return ctx.JSON(map[string]string{
            "message": "No data provided",
        })
    }
    
    var req MyRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return err
    }
    
    // Process non-empty request
    return ctx.JSON(processRequest(req))
}
```

### Key JSON Parsing Facts

- Base64 handling is automatic (Lift checks `isBase64Encoded` flag)
- Error responses are automatically structured as JSON
- Validation is automatic if struct tags are present
- Content Type is not required (Lift assumes JSON for `ParseRequest`)

## Multi-tenant Request Handling

### Canonical Pattern

#### Step 1: Configure Authentication Middleware

```go
func main() {
    app := lift.New()
    
    // JWT middleware automatically extracts tenant ID from token
    app.Use(middleware.JWT(middleware.JWTConfig{
        SecretKey:       os.Getenv("JWT_SECRET"),
        RequireTenantID: true, // This enforces tenant ID in JWT
    }))
    
    app.POST("/api/users", createUserHandler)
    
    lambda.Start(app.HandleRequest)
}
```

#### Step 2: Use Tenant Context in Handlers

```go
func createUserHandler(ctx *lift.Context) error {
    // Tenant ID is ALREADY set by middleware
    tenantID := ctx.TenantID()
    if tenantID == "" {
        return lift.Unauthorized("Tenant context required")
    }
    
    // Parse request body - NO special configuration needed
    var req CreateUserRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return err
    }
    
    // Use tenant ID in your business logic
    user := User{
        TenantID: tenantID,
        Name:     req.Name,
        Email:    req.Email,
    }
    
    return ctx.JSON(user)
}
```

### Alternative: Custom Tenant Header

```go
func tenantMiddleware() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            tenantID := ctx.Header("X-Tenant-ID")
            if tenantID == "" {
                return lift.Unauthorized("X-Tenant-ID header required")
            }
            
            // Set tenant ID in context for handlers to use
            ctx.Set("tenant_id", tenantID)
            
            return next.Handle(ctx)
        })
    }
}

// Usage
app.Use(tenantMiddleware())
```

### Type-Safe Handlers with Multi-tenancy

```go
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
```

## DynamoDB Table Design

### Standard Table Structure

All Lift tables use the same basic structure:

- Primary key: `pk` (partition key)
- Sort key: `sk` (sort key)
- Global Secondary Indexes (GSIs) defined in infrastructure code
- DynamORM struct tags map model fields to existing GSIs
- Time-to-live (TTL) attributes for automatic data expiration

### CDK Table Creation

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

liftTable.Table.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
    IndexName: jsii.String("tenant-index"),
    PartitionKey: &awsdynamodb.Attribute{
        Name: jsii.String("TenantID"),
        Type: awsdynamodb.AttributeType_STRING,
    },
    SortKey: &awsdynamodb.Attribute{
        Name: jsii.String("CreatedAt"),
        Type: awsdynamodb.AttributeType_STRING,
    },
})
```

## DynamORM Integration

### DynamORM Model Definition

```go
type User struct {
    // Keys - DynamORM uses field names as DynamoDB attribute names
    PK string `dynamorm:"pk"`  // user#{user_id}
    SK string `dynamorm:"sk"`  // user#{user_id}
    
    // GSI attributes
    Email     string `dynamorm:"index:email-index,pk"`      // For email lookups
    TenantID  string `dynamorm:"index:tenant-index,pk"`     // For tenant queries
    CreatedAt string `dynamorm:"index:tenant-index,sk"`     // For sorting
    
    // Data attributes
    UserID    string    `json:"user_id"`
    Name      string    `json:"name"`
    Status    string    `json:"status"`
    UpdatedAt time.Time `json:"updated_at"`
    TTL       int64     `json:"ttl,omitempty" dynamorm:"ttl"`
}
```

### Multi-Tenant Patterns

#### Tenant-Isolated Data

```go
type TenantUser struct {
    // Composite keys for tenant isolation
    PK string `dynamorm:"pk"`  // tenant#{tenant_id}
    SK string `dynamorm:"sk"`  // user#{user_id}
    
    // Tenant queries
    TenantID   string `dynamorm:"index:tenant-index,pk"`
    EntityType string `dynamorm:"index:tenant-index,sk"`  // "user"
    
    // User data
    UserID   string `json:"user_id"`
    Email    string `json:"email"`
    Name     string `json:"name"`
}

// Query all users for a tenant
users, err := dynamorm.Query[TenantUser](ctx, db).
    WithPK(fmt.Sprintf("tenant#%s", tenantID)).
    WithSKPrefix("user#").
    Execute()
```

### Using DynamORM in Lambda Functions

```go
package main

import (
    "context"
    "os"
    
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
    "github.com/pay-theory/lift/pkg/lift"
)

var db *dynamorm.Client

func init() {
    // Initialize DynamORM client
    client, err := dynamorm.NewClient(session.Config{
        Region: os.Getenv("AWS_REGION"),
    })
    if err != nil {
        panic(err)
    }
    db = client
}

func handler(ctx *lift.Context) error {
    tableName := os.Getenv("DYNAMODB_TABLE")
    
    // Create a new user
    user := &User{
        PK:        fmt.Sprintf("user#%s", userID),
        SK:        fmt.Sprintf("user#%s", userID),
        UserID:    userID,
        Email:     email,
        TenantID:  ctx.TenantID(),
        Name:      name,
        Status:    "active",
        CreatedAt: time.Now().Format(time.RFC3339),
        UpdatedAt: time.Now(),
    }
    
    err := dynamorm.Put(ctx.Context, db, tableName, user).Execute()
    if err != nil {
        return lift.NewError(500, "Failed to create user", nil)
    }
    
    return ctx.JSON(user)
}
```

## WebSocket API Implementation

### Modular Pattern (Required for v1.0.54+)

```go
// Step 1: Create Lambda functions at stack level (not nested)
connectFunction := awslambda.NewFunction(stack, jsii.String("Connect"), &awslambda.FunctionProps{
    FunctionName: jsii.String("my-app-ws-connect"),
    Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    Architecture: awslambda.Architecture_ARM_64(),
    Code:         awslambda.Code_FromAsset(jsii.String("./dist/connect"), nil),
    Handler:      jsii.String("bootstrap"),
    Timeout:      awscdk.Duration_Seconds(jsii.Number(30)),
    Environment: &map[string]*string{
        "APP_NAME": jsii.String("my-app"),
    },
})

disconnectFunction := awslambda.NewFunction(stack, jsii.String("Disconnect"), &awslambda.FunctionProps{
    FunctionName: jsii.String("my-app-ws-disconnect"),
    Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    Architecture: awslambda.Architecture_ARM_64(),
    Code:         awslambda.Code_FromAsset(jsii.String("./dist/disconnect"), nil),
    Handler:      jsii.String("bootstrap"),
    Timeout:      awscdk.Duration_Seconds(jsii.Number(30)),
})

defaultFunction := awslambda.NewFunction(stack, jsii.String("Default"), &awslambda.FunctionProps{
    FunctionName: jsii.String("my-app-ws-default"),
    Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    Architecture: awslambda.Architecture_ARM_64(),
    Code:         awslambda.Code_FromAsset(jsii.String("./dist/default"), nil),
    Handler:      jsii.String("bootstrap"),
    Timeout:      awscdk.Duration_Seconds(jsii.Number(30)),
})

// Step 2: Create WebSocket API with external functions
wsApi := constructs.NewWebSocketAPI(stack, jsii.String("API"), &constructs.WebSocketAPIProps{
    ApiName:                 jsii.String("my-app-websocket"),
    Description:             jsii.String("WebSocket API for my app"),
    // Pass the functions - this prevents internal function creation
    ConnectRouteFunction:    connectFunction,
    DisconnectRouteFunction: disconnectFunction,
    DefaultRouteFunction:    defaultFunction,
    // Other configuration
    EnableConnectionManagement: jsii.Bool(true),
    EnableAccessLogging:        jsii.Bool(true),
    EnableMultiTenant:          jsii.Bool(true),
})

// Step 3: Grant necessary permissions
if wsApi.ConnectionTable != nil {
    wsApi.ConnectionTable.GrantReadWrite(connectFunction)
    wsApi.ConnectionTable.GrantReadWrite(disconnectFunction)
    wsApi.ConnectionTable.GrantReadWrite(defaultFunction)
}
```

### Benefits of Modular Pattern

1. **Shorter Resource Names**: Flatter nesting hierarchy results in shorter CloudFormation resource names
2. **Better Control**: Full control over Lambda function configuration
3. **Reusability**: Functions can be shared across multiple constructs
4. **Clearer Architecture**: Infrastructure code better reflects the actual architecture

## Common Table Patterns

### 1. Connection Table (WebSocket)

```go
type Connection struct {
    PK string `dynamorm:"pk"`  // connection#{connection_id}
    SK string `dynamorm:"sk"`  // connection#{connection_id}
    
    // Indexes
    UserID    string `dynamorm:"index:user-connections,pk"`
    Timestamp string `dynamorm:"index:user-connections,sk"`
    
    // Connection data
    ConnectionID string    `json:"connection_id"`
    Endpoint     string    `json:"endpoint"`
    ConnectedAt  time.Time `json:"connected_at"`
    TTL          int64     `json:"ttl"`
}
```

### 2. Rate Limiting Table

```go
type RateLimit struct {
    PK string `dynamorm:"pk"`  // ratelimit#{identifier}#{window}
    SK string `dynamorm:"sk"`  // ratelimit#{identifier}#{window}
    
    // Rate limit indexes
    IPAddress  string `dynamorm:"index:ip-index,pk"`
    UserID     string `dynamorm:"index:user-index,pk"`
    TenantID   string `dynamorm:"index:tenant-index,pk"`
    
    // Rate limit data
    Identifier string `json:"identifier"`
    WindowTime string `json:"window_time"`
    Count      int    `json:"count"`
    ExpiresAt  int64  `json:"expires_at" dynamorm:"ttl"`
}
```

### 3. Idempotency Table

```go
type IdempotencyRecord struct {
    PK string `dynamorm:"pk"`  // idempotency#{key}
    SK string `dynamorm:"sk"`  // idempotency#{key}
    
    // Lookup indexes
    FunctionName string `dynamorm:"index:function-index,pk"`
    Status       string `dynamorm:"index:status-index,pk"`
    Timestamp    string `dynamorm:"index:status-index,sk"`
    
    // Idempotency data
    IdempotencyKey string          `json:"idempotency_key"`
    Response       json.RawMessage `json:"response"`
    Status         string          `json:"status"`
    ExpiresAt      int64           `json:"expires_at" dynamorm:"ttl"`
}
```

### 4. Event Store

```go
type Event struct {
    PK string `dynamorm:"pk"`  // stream#{stream_id}
    SK string `dynamorm:"sk"`  // event#{timestamp}#{event_id}
    
    // Event indexes
    EventType  string `dynamorm:"index:type-index,pk"`
    Timestamp  string `dynamorm:"index:type-index,sk"`
    AggregateID string `dynamorm:"index:aggregate-index,pk"`
    Version     int    `dynamorm:"index:aggregate-index,sk"`
    
    // Event data
    EventID   string          `json:"event_id"`
    Data      json.RawMessage `json:"data"`
    Metadata  map[string]any  `json:"metadata"`
}
```

## Best Practices

### Lambda Handler Best Practices

1. **Use `app.HandleRequest`**: Always use `lambda.Start(app.HandleRequest)` for Lambda functions
2. **Avoid Custom Wrappers**: Don't create custom handler functions around `app.HandleRequest`
3. **Use Type-Safe Handlers**: Leverage Lift's type-safe handler pattern for cleaner code
4. **Middleware for Cross-Cutting Concerns**: Use middleware for authentication, logging, etc.

### DynamoDB Best Practices

1. **Always use composite keys** for clear entity identification:
   - `pk`: `entity_type#{id}` (e.g., `user#123`, `tenant#abc`)
   - `sk`: Depends on access pattern (could be same as pk or hierarchical)

2. **Design indexes based on access patterns**:
   - Use GSIs for alternative query patterns
   - Keep GSI keys meaningful and efficient

3. **Implement TTL for temporary data**:
   - Rate limits, idempotency records, temporary tokens
   - Set appropriate expiration times

4. **Use batch operations** for efficiency:
   ```go
   items := []any{user1, user2, user3}
   err := dynamorm.BatchPut(ctx, db, tableName, items).Execute()
   ```

5. **Handle multi-tenancy at the data layer**:
   - Include tenant ID in composite keys
   - Use tenant-specific GSIs for efficient queries
   - Validate tenant access in Lambda handlers

### WebSocket API Best Practices

1. **Use short, meaningful IDs** for constructs (e.g., "API" instead of "WebSocketAPI")
2. **Create all Lambda functions at the stack level**
3. **Use consistent naming patterns** for functions
4. **Consider creating a helper function** to standardize Lambda creation
5. **Always test your CloudFormation template** to ensure resource names are within limits

## Troubleshooting

### Common Issues

1. **GSI not created**: GSIs must be created in your infrastructure (CDK, CloudFormation, or AWS Console). DynamORM struct tags only tell DynamORM how to use existing GSIs - they don't create them. Ensure both:
   - Your infrastructure creates the GSIs with matching names
   - Your struct has proper index tags that match the GSI names

2. **Query returns no results**: Check your key construction. Use composite keys correctly.

3. **TTL not working**: Ensure the TTL attribute name matches between CDK and your model tags.

4. **Access denied**: Verify Lambda has proper IAM permissions for the table and indexes.

### Debug Tips

Enable DynamORM debugging:
```go
os.Setenv("DYNAMORM_DEBUG", "true")
```

Log table operations:
```go
app := lift.New(lift.WithDebug(true))
```

## Migration from Old DynamORM Tables

If you have existing tables using the old DynamORM-specific constructs, migrate them to the standard format:

### Old Structure
```go
// Before - DynamORM-specific table
table := constructs.NewDynamORMTable(stack, id, &constructs.DynamORMTableProps{
    PartitionKey: "TenantID",
    SortKey:      "UserID",
    GSI1PartitionKey: "Email",
    // ...
})
```

### New Structure
```go
// After - Standard Lift table
table := constructs.NewLiftTable(stack, id, &constructs.LiftTableProps{
    TableName: jsii.String("my-table"),
})

// Define structure in DynamORM model
type User struct {
    PK       string `dynamorm:"pk"`        // tenant#{tenant_id}
    SK       string `dynamorm:"sk"`        // user#{user_id}
    Email    string `dynamorm:"index:email-index,pk"`
    TenantID string `json:"tenant_id"`
    UserID   string `json:"user_id"`
}
```

## Summary

These patterns form the foundation of Lift applications:

1. **Lambda Initialization**: Always use `lambda.Start(app.HandleRequest)`
2. **JSON Parsing**: Use `ctx.ParseRequest(&req)` - works identically for all API Gateway versions
3. **Multi-tenancy**: Set tenant context in middleware, access with `ctx.TenantID()`
4. **DynamoDB Tables**: Use standard table structure with `pk` and `sk` keys
5. **DynamORM Models**: Define models with proper struct tags for GSIs
6. **WebSocket API**: Use modular pattern with externally created Lambda functions

Remember: Lift is designed to eliminate boilerplate. If you find yourself writing complex initialization or parsing code, you're likely doing it wrong. The framework handles the complexity internally.