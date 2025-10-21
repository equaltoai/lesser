# DynamORM Integration Guide for Lift CDK Constructs

This guide provides comprehensive information for integrating DynamORM with Lift CDK constructs, specifically for RateLimitedFunction and IdempotentFunction.

## 1. Table Structure Patterns

### Rate Limiting Table Structure

```go
package models

import (
    "os"
    "time"
)

// RateLimitRecord is compatible with both DynamORM and the Limited library
type RateLimitRecord struct {
    // Primary key: identifier (could be IP, UserID, TenantID+UserID, etc.)
    Identifier string `dynamorm:"pk" json:"identifier"`
    
    // Sort key: window timestamp (for sliding window rate limiting)
    WindowTime string `dynamorm:"sk" json:"window_time"`
    
    // GSI for querying by different dimensions
    IPAddress  string    `dynamorm:"index:gsi-ip,pk" json:"ip_address,omitempty"`
    UserID     string    `dynamorm:"index:gsi-user,pk" json:"user_id,omitempty"`
    TenantID   string    `dynamorm:"index:gsi-tenant,pk" json:"tenant_id,omitempty"`
    
    // Rate limit data
    Count      int       `json:"count"`
    BucketKey  string    `dynamorm:"index:gsi-bucket,pk" json:"bucket_key"`
    
    // TTL for automatic cleanup (set to window end + buffer)
    ExpiresAt  time.Time `dynamorm:"ttl" json:"expires_at"`
    
    // Metadata
    CreatedAt  time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt  time.Time `dynamorm:"updated_at" json:"updated_at"`
}

func (r *RateLimitRecord) TableName() string {
    return os.Getenv("RATE_LIMIT_TABLE_NAME")
}
```

**Key Design Decisions:**
- **PK/SK Pattern**: Use identifier as PK and window timestamp as SK for efficient range queries
- **GSIs**: Separate indexes for IP, User, and Tenant queries
- **TTL**: Automatic cleanup of expired rate limit records
- **Flexible Identifier**: Can be IP, UserID, or composite like "tenant:123:user:456"

### Idempotency Table Structure

```go
package models

import (
    "os"
    "time"
)

// IdempotencyRecord stores idempotent request data
type IdempotencyRecord struct {
    // Primary key: idempotency key (from header or request)
    IdempotencyKey string `dynamorm:"pk" json:"idempotency_key"`
    
    // Sort key: constant value for single item per key
    SK string `dynamorm:"sk" json:"sk" default:"IDEMPOTENCY"`
    
    // GSIs for querying
    FunctionName string    `dynamorm:"index:gsi-function,pk" json:"function_name"`
    TenantID     string    `dynamorm:"index:gsi-tenant,pk" json:"tenant_id,omitempty"`
    Status       string    `dynamorm:"index:gsi-status,pk" json:"status"`
    Timestamp    time.Time `dynamorm:"index:gsi-timestamp,pk" json:"timestamp"`
    
    // Request/Response data
    RequestHash  string    `json:"request_hash"`
    RequestBody  string    `dynamorm:"json" json:"request_body"` // Stored as JSON
    Response     string    `dynamorm:"json" json:"response"`     // Can be up to 400KB
    StatusCode   int       `json:"status_code"`
    
    // State management
    LockToken    string    `json:"lock_token,omitempty"`
    LockedUntil  time.Time `json:"locked_until,omitempty"`
    
    // TTL for automatic cleanup
    ExpiresAt    time.Time `dynamorm:"ttl" json:"expires_at"`
    
    // Metadata
    CreatedAt    time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt    time.Time `dynamorm:"updated_at" json:"updated_at"`
    CompletedAt  time.Time `json:"completed_at,omitempty"`
}

func (i *IdempotencyRecord) TableName() string {
    return os.Getenv("IDEMPOTENCY_TABLE_NAME")
}

// Status constants
const (
    IdempotencyStatusPending    = "PENDING"
    IdempotencyStatusProcessing = "PROCESSING"
    IdempotencyStatusCompleted  = "COMPLETED"
    IdempotencyStatusFailed     = "FAILED"
)
```

**Key Design Decisions:**
- **Single Item per Key**: Using constant SK ensures one record per idempotency key
- **Status Tracking**: Track request lifecycle (pending → processing → completed/failed)
- **Lock Mechanism**: Prevent concurrent processing of same idempotency key
- **Large Response Support**: Using `dynamorm:"json"` tag for responses up to 400KB
- **Multiple Query Patterns**: GSIs for function name, tenant, status, and timestamp queries

## 2. DynamORM Table Configuration

### CDK Table Creation

When creating tables in CDK for DynamORM, ensure these configurations:

```typescript
// For Rate Limiting Table
const rateLimitTable = new dynamodb.Table(this, 'RateLimitTable', {
    tableName: props.rateLimitTableName,
    partitionKey: {
        name: 'Identifier',
        type: dynamodb.AttributeType.STRING
    },
    sortKey: {
        name: 'WindowTime',
        type: dynamodb.AttributeType.STRING
    },
    billingMode: dynamodb.BillingMode.PAY_PER_REQUEST, // Or provisioned
    pointInTimeRecovery: true,
    timeToLiveAttribute: 'ExpiresAt',
    stream: dynamodb.StreamViewType.NEW_AND_OLD_IMAGES, // If needed
});

// Add GSIs
rateLimitTable.addGlobalSecondaryIndex({
    indexName: 'gsi-ip',
    partitionKey: {
        name: 'IPAddress',
        type: dynamodb.AttributeType.STRING
    },
    projectionType: dynamodb.ProjectionType.ALL
});

rateLimitTable.addGlobalSecondaryIndex({
    indexName: 'gsi-user',
    partitionKey: {
        name: 'UserID',
        type: dynamodb.AttributeType.STRING
    },
    projectionType: dynamodb.ProjectionType.ALL
});

rateLimitTable.addGlobalSecondaryIndex({
    indexName: 'gsi-tenant',
    partitionKey: {
        name: 'TenantID',
        type: dynamodb.AttributeType.STRING
    },
    projectionType: dynamodb.ProjectionType.ALL
});

// For Idempotency Table
const idempotencyTable = new dynamodb.Table(this, 'IdempotencyTable', {
    tableName: props.idempotencyTableName,
    partitionKey: {
        name: 'IdempotencyKey',
        type: dynamodb.AttributeType.STRING
    },
    sortKey: {
        name: 'SK',
        type: dynamodb.AttributeType.STRING
    },
    billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
    pointInTimeRecovery: true,
    timeToLiveAttribute: 'ExpiresAt',
});

// Add GSIs for idempotency
idempotencyTable.addGlobalSecondaryIndex({
    indexName: 'gsi-function',
    partitionKey: {
        name: 'FunctionName',
        type: dynamodb.AttributeType.STRING
    },
    projectionType: dynamodb.ProjectionType.ALL
});

idempotencyTable.addGlobalSecondaryIndex({
    indexName: 'gsi-status',
    partitionKey: {
        name: 'Status',
        type: dynamodb.AttributeType.STRING
    },
    projectionType: dynamodb.ProjectionType.ALL
});
```

### Important DynamORM Table Requirements

1. **Attribute Names**: Must match struct field names exactly (case-sensitive)
2. **TTL Attribute**: Must be Unix timestamp in seconds (DynamORM handles conversion)
3. **GSI Names**: Must match the `dynamorm:"index:name,pk"` tag format
4. **Billing Mode**: DynamORM works with both PAY_PER_REQUEST and PROVISIONED
5. **Streams**: Enable if you need change data capture or event processing

## 3. Runtime Integration

### Environment Variables

DynamORM expects these environment variables:

```typescript
// In your CDK construct, set these on the Lambda function
myFunction.addEnvironment('AWS_REGION', Stack.of(this).region);
myFunction.addEnvironment('RATE_LIMIT_TABLE_NAME', rateLimitTable.tableName);
myFunction.addEnvironment('IDEMPOTENCY_TABLE_NAME', idempotencyTable.tableName);

// Optional DynamORM configuration
myFunction.addEnvironment('DYNAMORM_DEBUG', 'false');
myFunction.addEnvironment('DYNAMORM_RETRY_MAX_ATTEMPTS', '3');
myFunction.addEnvironment('DYNAMORM_RETRY_BASE_DELAY', '100'); // milliseconds
```

### Lambda Handler Setup

```go
package main

import (
    "context"
    "os"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/protection"
    "github.com/pay-theory/limited"
)

var (
    db *dynamorm.DB
    rateLimiter *limited.Limiter
)

func init() {
    // Initialize DynamORM with Lambda optimizations
    db = dynamorm.New(
        dynamorm.WithLambdaOptimizations(),
        dynamorm.WithRetryPolicy(3, 100), // 3 retries, 100ms base delay
    )
    
    // Register models
    db.RegisterModel(&RateLimitRecord{})
    db.RegisterModel(&IdempotencyRecord{})
    
    // Initialize Limited library with DynamORM backend
    rateLimiter = limited.New(
        limited.WithBackend(NewDynamORMBackend(db)),
        limited.WithWindowSize(time.Minute),
    )
}

func handler(ctx context.Context, event interface{}) (interface{}, error) {
    // Your handler logic
    return nil, nil
}

func main() {
    lambda.Start(handler)
}
```

### IAM Permissions

```typescript
// Grant permissions to Lambda
rateLimitTable.grantReadWriteData(myFunction);
idempotencyTable.grantReadWriteData(myFunction);

// Or create a custom policy for fine-grained control
const dynamoPolicy = new iam.PolicyStatement({
    effect: iam.Effect.ALLOW,
    actions: [
        'dynamodb:GetItem',
        'dynamodb:PutItem',
        'dynamodb:UpdateItem',
        'dynamodb:DeleteItem',
        'dynamodb:Query',
        'dynamodb:BatchGetItem',
        'dynamodb:BatchWriteItem',
        'dynamodb:ConditionCheckItem',
    ],
    resources: [
        rateLimitTable.tableArn,
        `${rateLimitTable.tableArn}/index/*`,
        idempotencyTable.tableArn,
        `${idempotencyTable.tableArn}/index/*`,
    ],
});

// For multi-tenant isolation
const tenantPolicy = new iam.PolicyStatement({
    effect: iam.Effect.ALLOW,
    actions: ['dynamodb:Query'],
    resources: [`${rateLimitTable.tableArn}/index/gsi-tenant`],
    conditions: {
        'ForAllValues:StringEquals': {
            'dynamodb:LeadingKeys': ['${aws:PrincipalTag/TenantID}']
        }
    }
});

myFunction.addToRolePolicy(dynamoPolicy);
```

## 4. Limited Library Integration

### DynamORM Backend for Limited

```go
package backends

import (
    "context"
    "fmt"
    "time"
    
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/limited"
)

type DynamORMBackend struct {
    db *dynamorm.DB
}

func NewDynamORMBackend(db *dynamorm.DB) *DynamORMBackend {
    return &DynamORMBackend{db: db}
}

// Implement limited.Backend interface
func (b *DynamORMBackend) Increment(ctx context.Context, key string, window time.Time) (int64, error) {
    record := &RateLimitRecord{
        Identifier: key,
        WindowTime: window.Format(time.RFC3339),
        Count:      1,
        ExpiresAt:  window.Add(2 * time.Hour), // TTL buffer
    }
    
    // Use DynamORM's UpdateBuilder for atomic increment
    result := b.db.Model(record).
        Where("Identifier", "=", key).
        Where("WindowTime", "=", window.Format(time.RFC3339)).
        Update(ctx).
        Add("Count", 1).
        SetIfNotExists("Count", 1).
        SetIfNotExists("ExpiresAt", window.Add(2 * time.Hour)).
        Return("Count").
        Execute()
    
    if result.Error != nil {
        return 0, result.Error
    }
    
    // Extract count from result
    var count int64
    if err := result.Unmarshal(&count, "Count"); err != nil {
        return 0, err
    }
    
    return count, nil
}

func (b *DynamORMBackend) Get(ctx context.Context, key string, window time.Time) (int64, error) {
    var record RateLimitRecord
    
    result := b.db.Model(&RateLimitRecord{}).
        Where("Identifier", "=", key).
        Where("WindowTime", "=", window.Format(time.RFC3339)).
        First(ctx, &record)
    
    if result.Error != nil {
        if result.IsNotFound() {
            return 0, nil
        }
        return 0, result.Error
    }
    
    return int64(record.Count), nil
}

func (b *DynamORMBackend) Reset(ctx context.Context, key string, window time.Time) error {
    result := b.db.Model(&RateLimitRecord{}).
        Where("Identifier", "=", key).
        Where("WindowTime", "=", window.Format(time.RFC3339)).
        Delete(ctx)
    
    return result.Error
}
```

### Usage Example

```go
// In your Lambda handler
func rateLimitedHandler(ctx context.Context, event APIGatewayRequest) (APIGatewayResponse, error) {
    // Extract identifier (IP, UserID, etc.)
    identifier := fmt.Sprintf("ip:%s", event.RequestContext.Identity.SourceIP)
    
    // Check rate limit
    allowed, err := rateLimiter.Allow(ctx, identifier)
    if err != nil {
        return APIGatewayResponse{StatusCode: 500}, err
    }
    
    if !allowed {
        return APIGatewayResponse{
            StatusCode: 429,
            Body:       "Rate limit exceeded",
            Headers:    map[string]string{"Retry-After": "60"},
        }, nil
    }
    
    // Process request
    return processRequest(ctx, event)
}
```

## 5. Code Examples

### Complete Rate Limiting Implementation

```go
package handlers

import (
    "context"
    "fmt"
    "time"
    
    "github.com/pay-theory/dynamorm"
)

type RateLimitService struct {
    db *dynamorm.DB
}

func NewRateLimitService(db *dynamorm.DB) *RateLimitService {
    return &RateLimitService{db: db}
}

func (s *RateLimitService) CheckAndIncrement(ctx context.Context, identifier string, limit int) (bool, error) {
    window := time.Now().Truncate(time.Minute)
    
    // Atomic increment with conditional check
    result := s.db.Model(&RateLimitRecord{}).
        Where("Identifier", "=", identifier).
        Where("WindowTime", "=", window.Format(time.RFC3339)).
        Update(ctx).
        Add("Count", 1).
        SetIfNotExists("Count", 1).
        SetIfNotExists("CreatedAt", time.Now()).
        SetIfNotExists("ExpiresAt", window.Add(2 * time.Hour)).
        Condition("Count", "<", limit). // Only increment if under limit
        Return("Count").
        Execute()
    
    if result.Error != nil {
        // Check if condition failed (rate limit exceeded)
        if result.IsConditionFailed() {
            return false, nil
        }
        return false, result.Error
    }
    
    return true, nil
}

func (s *RateLimitService) GetUsage(ctx context.Context, identifier string) (int, error) {
    window := time.Now().Truncate(time.Minute)
    
    var record RateLimitRecord
    result := s.db.Model(&RateLimitRecord{}).
        Where("Identifier", "=", identifier).
        Where("WindowTime", "=", window.Format(time.RFC3339)).
        First(ctx, &record)
    
    if result.Error != nil {
        if result.IsNotFound() {
            return 0, nil
        }
        return 0, result.Error
    }
    
    return record.Count, nil
}
```

### Complete Idempotency Implementation

```go
package handlers

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "time"
    
    "github.com/google/uuid"
    "github.com/pay-theory/dynamorm"
)

type IdempotencyService struct {
    db *dynamorm.DB
}

func NewIdempotencyService(db *dynamorm.DB) *IdempotencyService {
    return &IdempotencyService{db: db}
}

func (s *IdempotencyService) ProcessIdempotent(
    ctx context.Context,
    key string,
    functionName string,
    request interface{},
    handler func() (interface{}, error),
) (interface{}, error) {
    // Calculate request hash
    requestHash := s.hashRequest(request)
    
    // Check for existing record
    existing, err := s.getExistingRecord(ctx, key)
    if err != nil {
        return nil, err
    }
    
    // If exists and completed, return cached response
    if existing != nil {
        if existing.Status == IdempotencyStatusCompleted {
            if existing.RequestHash != requestHash {
                return nil, errors.New("idempotency key reused with different request")
            }
            
            var response interface{}
            if err := json.Unmarshal([]byte(existing.Response), &response); err != nil {
                return nil, err
            }
            return response, nil
        }
        
        // If processing, check lock
        if existing.Status == IdempotencyStatusProcessing {
            if time.Now().Before(existing.LockedUntil) {
                return nil, errors.New("request is already being processed")
            }
            // Lock expired, we can take over
        }
    }
    
    // Create or update record with lock
    lockToken := uuid.New().String()
    record := &IdempotencyRecord{
        IdempotencyKey: key,
        SK:            "IDEMPOTENCY",
        FunctionName:  functionName,
        Status:        IdempotencyStatusProcessing,
        RequestHash:   requestHash,
        LockToken:     lockToken,
        LockedUntil:   time.Now().Add(5 * time.Minute),
        ExpiresAt:     time.Now().Add(24 * time.Hour),
    }
    
    // Marshal request
    requestBytes, _ := json.Marshal(request)
    record.RequestBody = string(requestBytes)
    
    // Try to acquire lock
    result := s.db.Model(record).
        Create(ctx).
        ConditionExpression("attribute_not_exists(IdempotencyKey) OR #status <> :processing OR #locked < :now").
        ExpressionAttributeNames(map[string]string{
            "#status": "Status",
            "#locked": "LockedUntil",
        }).
        ExpressionAttributeValues(map[string]interface{}{
            ":processing": IdempotencyStatusProcessing,
            ":now":        time.Now(),
        }).
        Execute()
    
    if result.Error != nil {
        if result.IsConditionFailed() {
            return nil, errors.New("could not acquire idempotency lock")
        }
        return nil, result.Error
    }
    
    // Process the request
    response, handlerErr := handler()
    
    // Update record with result
    status := IdempotencyStatusCompleted
    if handlerErr != nil {
        status = IdempotencyStatusFailed
    }
    
    responseBytes, _ := json.Marshal(response)
    
    updateResult := s.db.Model(&IdempotencyRecord{}).
        Where("IdempotencyKey", "=", key).
        Update(ctx).
        Set("Status", status).
        Set("Response", string(responseBytes)).
        Set("CompletedAt", time.Now()).
        Condition("LockToken", "=", lockToken). // Ensure we still own the lock
        Execute()
    
    if updateResult.Error != nil {
        return nil, updateResult.Error
    }
    
    if handlerErr != nil {
        return nil, handlerErr
    }
    
    return response, nil
}

func (s *IdempotencyService) getExistingRecord(ctx context.Context, key string) (*IdempotencyRecord, error) {
    var record IdempotencyRecord
    
    result := s.db.Model(&IdempotencyRecord{}).
        Where("IdempotencyKey", "=", key).
        Where("SK", "=", "IDEMPOTENCY").
        First(ctx, &record)
    
    if result.Error != nil {
        if result.IsNotFound() {
            return nil, nil
        }
        return nil, result.Error
    }
    
    return &record, nil
}

func (s *IdempotencyService) hashRequest(request interface{}) string {
    data, _ := json.Marshal(request)
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}
```

## 6. Migration Path

### For Existing Tables

If you have existing DynamoDB tables, you can migrate to DynamORM-compatible structure:

1. **Add Missing Attributes**: Ensure all required attributes exist
2. **Create GSIs**: Add any missing GSIs required by the models
3. **Enable TTL**: Configure TTL on the appropriate attribute
4. **Update IAM**: Ensure Lambda has permissions for all operations

### Backward Compatibility

DynamORM is flexible with existing data:
- Missing attributes are handled gracefully
- Can work with existing table structures
- Supports custom attribute names via tags
- Can coexist with non-DynamORM code

### Breaking Changes to Consider

1. **Attribute Names**: DynamORM expects specific attribute names
2. **TTL Format**: Must be Unix timestamp in seconds
3. **Type Safety**: Strongly typed models may reject malformed data
4. **GSI Names**: Must match the model tags exactly

## 7. Best Practices

### Performance Optimization

1. **Use Lambda Optimizations**:
   ```go
   db := dynamorm.New(dynamorm.WithLambdaOptimizations())
   ```

2. **Batch Operations**: Use batch methods for multiple items
   ```go
   db.Model(&RateLimitRecord{}).BatchCreate(ctx, records)
   ```

3. **Projection Optimization**: Only fetch needed attributes
   ```go
   db.Model(&RateLimitRecord{}).
       Select("Count", "ExpiresAt").
       Where("Identifier", "=", key).
       First(ctx, &record)
   ```

### Error Handling

```go
result := db.Model(&RateLimitRecord{}).Create(ctx, record)

if result.Error != nil {
    switch {
    case result.IsConditionFailed():
        // Handle conditional check failure
    case result.IsThrottled():
        // Handle throttling with backoff
    case result.IsNotFound():
        // Handle not found
    default:
        // Handle other errors
    }
}
```

### Testing

Use DynamORM's mock interfaces for unit testing:

```go
import "github.com/pay-theory/dynamorm/pkg/mocks"

func TestRateLimit(t *testing.T) {
    mockDB := mocks.NewMockDB()
    mockDB.On("Model", &RateLimitRecord{}).Return(mockQuery)
    
    service := NewRateLimitService(mockDB)
    // Test your service
}
```

## 8. Common Patterns

### Multi-Tenant Rate Limiting

```go
// Composite identifier for tenant isolation
identifier := fmt.Sprintf("tenant:%s:user:%s", tenantID, userID)

// Query by tenant
var records []RateLimitRecord
db.Model(&RateLimitRecord{}).
    UseIndex("gsi-tenant").
    Where("TenantID", "=", tenantID).
    All(ctx, &records)
```

### Sliding Window Rate Limiting

```go
// Query multiple windows for sliding window
windows := []string{
    time.Now().Truncate(time.Minute).Format(time.RFC3339),
    time.Now().Truncate(time.Minute).Add(-time.Minute).Format(time.RFC3339),
}

var total int
for _, window := range windows {
    var record RateLimitRecord
    db.Model(&RateLimitRecord{}).
        Where("Identifier", "=", identifier).
        Where("WindowTime", "=", window).
        First(ctx, &record)
    
    total += record.Count
}
```

### Distributed Locking for Idempotency

```go
// Acquire distributed lock with timeout
lockResult := db.Model(&IdempotencyRecord{}).
    Where("IdempotencyKey", "=", key).
    Update(ctx).
    Set("LockToken", lockToken).
    Set("LockedUntil", time.Now().Add(5*time.Minute)).
    Condition("attribute_not_exists(LockToken) OR LockedUntil < :now").
    ExpressionAttributeValues(map[string]interface{}{
        ":now": time.Now(),
    }).
    Execute()

if lockResult.IsConditionFailed() {
    // Lock is held by another process
}
```

## Summary

This guide provides the foundation for integrating DynamORM with Lift CDK constructs. The key points are:

1. **Model Design**: Use DynamORM struct tags to define table structure
2. **Table Configuration**: Ensure CDK tables match DynamORM expectations
3. **Runtime Setup**: Configure environment variables and Lambda optimizations
4. **Integration**: Implement Limited library backend using DynamORM
5. **Best Practices**: Follow DynamORM patterns for performance and reliability

For further questions or specific implementation details, refer to the DynamORM examples in `/examples/` directory or the comprehensive test suite in the repository.