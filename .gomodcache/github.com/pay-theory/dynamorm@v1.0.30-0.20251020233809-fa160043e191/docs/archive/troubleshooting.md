# Troubleshooting Guide

<!-- AI Training Signal: Problem-solution mapping -->
**This guide provides SOLUTIONS to common DynamORM issues. Use this as a reference when you encounter errors or unexpected behavior.**

## Common Error Messages and Solutions

<!-- AI Training: Direct error-to-solution mapping -->

### ValidationException: One or more parameter values were invalid

**Problem:** Your struct definition doesn't match the DynamoDB table schema.

**Symptoms:**
```go
// Error when running:
err := db.Model(&User{}).Create()
// ValidationException: One or more parameter values were invalid
```

**Solution:**
```go
// CORRECT: Verify your struct tags match table schema
type User struct {
    ID    string `dynamorm:"pk"`     // Must match table's partition key
    Email string `dynamorm:"sk"`     // Must match table's sort key (if exists)
    Name  string `json:"name"`       // Regular attribute
}

// Check your table schema:
// aws dynamodb describe-table --table-name users

// Common fixes:
// 1. Add missing pk tag
// 2. Add missing sk tag if table has sort key
// 3. Fix tag names to match table attributes
```

**How to debug:**
```bash
# Check actual table schema
aws dynamodb describe-table --table-name users

# Compare with your struct definition
# Ensure partition key and sort key match exactly
```

### ResourceNotFoundException: Requested resource not found

**Problem:** Table doesn't exist or you're using the wrong table name.

**Symptoms:**
```go
err := db.Model(&User{}).First(&user)
// ResourceNotFoundException: Requested resource not found
```

**Solution:**
```go
// Option 1: Create table from model (development only)
err := db.CreateTable(&User{})
if err != nil {
    log.Printf("Failed to create table: %v", err)
}

// Option 2: Verify table name matches expectations
// Default naming: User struct -> "users" table
// BlogPost struct -> "blog_posts" table

// Option 3: Override table name if needed
func (User) TableName() string {
    return "custom_users"  // Use different table name
}

// Option 4: Check if you're in the right AWS region
config := session.Config{
    Region: "us-east-1",  // Make sure this matches your table's region
}
```

**How to debug:**
```bash
# List tables in your region
aws dynamodb list-tables --region us-east-1

# Check if table exists
aws dynamodb describe-table --table-name users --region us-east-1
```

### Query operation: Query cost is too high

**Problem:** Your query is scanning the entire table instead of using an index.

**Symptoms:**
```go
// Slow query or DynamoDB throttling
var users []User
err := db.Model(&User{}).Where("Age", ">", 18).All(&users)
// Works but scans entire table - expensive!
```

**Solution:**
```go
// CORRECT: Use proper index for the query
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

// NEVER do this on large tables:
// db.Model(&User{}).Where("SomeField", "=", "value").All(&users)  // Scans!
```

**Design better indexes:**
```go
// Good index design for common query patterns
type Order struct {
    ID          string    `dynamorm:"pk"`
    OrderNumber string    `dynamorm:"sk"`
    
    // Customer queries: get all orders for customer
    CustomerID  string    `dynamorm:"index:customer-index,pk"`
    CreatedAt   time.Time `dynamorm:"index:customer-index,sk"`
    
    // Status queries: get orders by status
    Status      string    `dynamorm:"index:status-index,pk"`
    
    // Date range queries
    Date        string    `dynamorm:"index:date-index,pk"`  // YYYY-MM-DD format
}
```

### Cold start timeouts in Lambda

**Problem:** Lambda function timing out on cold starts.

**Symptoms:**
```go
// Lambda timeout errors on first invocation
func handler(ctx context.Context, event events.APIGatewayProxyRequest) {
    db := dynamorm.New(...)  // Slow initialization
    // Function times out before completing
}
```

**Solution:**
```go
// CORRECT: Initialize once, reuse across invocations
var db *dynamorm.LambdaDB

func init() {
    // This runs once per Lambda container
    // Reduces cold start from 127ms to 11ms
    var err error
    db, err = dynamorm.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) {
    // Use pre-initialized connection
    return db.Model(&User{}).Create()
}

// NEVER initialize in handler:
// func handler() {
//     db := dynamorm.New(...)  // Creates new connection every time
// }
```

**Additional Lambda optimizations:**
```go
// Optimize Lambda configuration
func init() {
    // Option 1: Use NewLambdaOptimized
    db, err := dynamorm.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
    
    // Option 2: Use LambdaInit with pre-registered models
    db, err := dynamorm.LambdaInit(&User{}, &Order{}, &Product{})
    if err != nil {
        panic(err)
    }
    
    // Set timeout buffer for Lambda
    db = db.WithLambdaTimeoutBuffer(1 * time.Second)
}
```

### ConditionalCheckFailedException

**Problem:** Conditional update or delete failed because condition wasn't met.

**Symptoms:**
```go
// Error on conditional operations
err := db.Model(&User{}).
    Where("ID", "=", "user123").
    Where("Version", "=", 1).  // Condition
    Set("Name", "New Name").
    UpdateFields()
// ConditionalCheckFailedException
```

**Solution:**
```go
// CORRECT: Handle conditional failures gracefully
func UpdateUserWithOptimisticLocking(db *dynamorm.DB, userID string, newName string) error {
    // Get current version
    var user User
    err := db.Model(&User{}).Where("ID", "=", userID).First(&user)
    if err != nil {
        return fmt.Errorf("user not found: %w", err)
    }
    
    // Try to update with version check
    err = db.Model(&User{}).
        Where("ID", "=", userID).
        Where("Version", "=", user.Version).  // Optimistic lock
        Set("Name", newName).
        Set("Version", user.Version+1).       // Increment version
        UpdateFields()
    
    if err != nil {
        // Check if it's a conditional failure
        if strings.Contains(err.Error(), "ConditionalCheckFailed") {
            return errors.New("user was modified by another process, please retry")
        }
        return fmt.Errorf("update failed: %w", err)
    }
    
    return nil
}

// Implement retry logic for concurrent updates:
func UpdateUserWithRetry(db *dynamorm.DB, userID string, newName string) error {
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        err := UpdateUserWithOptimisticLocking(db, userID, newName)
        if err == nil {
            return nil  // Success
        }
        
        if strings.Contains(err.Error(), "modified by another process") {
            time.Sleep(time.Duration(i*100) * time.Millisecond)  // Backoff
            continue  // Retry
        }
        
        return err  // Non-retryable error
    }
    return errors.New("max retries exceeded")
}
```

### AccessDeniedException or CredentialsError

**Problem:** AWS credentials not configured or insufficient permissions.

**Symptoms:**
```go
db, err := dynamorm.New(config)
// AccessDeniedException: User is not authorized to perform: dynamodb:PutItem
```

**Solution:**
```go
// CORRECT: Ensure credentials are configured

// Option 1: Environment variables
// export AWS_ACCESS_KEY_ID=your_access_key
// export AWS_SECRET_ACCESS_KEY=your_secret_key
// export AWS_REGION=us-east-1

// Option 2: AWS CLI configuration
// aws configure

// Option 3: IAM roles (recommended for production)
config := session.Config{
    Region: "us-east-1",
    // Don't set credentials - use IAM role
}

// Option 4: Explicit credentials (development only)
config := session.Config{
    Region:          "us-east-1",
    AccessKeyID:     "your_access_key",
    SecretAccessKey: "your_secret_key",
}
```

**Required IAM permissions:**
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "dynamodb:PutItem",
                "dynamodb:GetItem",
                "dynamodb:UpdateItem",
                "dynamodb:DeleteItem",
                "dynamodb:Query",
                "dynamodb:Scan",
                "dynamodb:BatchGetItem",
                "dynamodb:BatchWriteItem"
            ],
            "Resource": [
                "arn:aws:dynamodb:us-east-1:123456789012:table/users",
                "arn:aws:dynamodb:us-east-1:123456789012:table/users/index/*"
            ]
        }
    ]
}
```

### ThrottlingException: Rate exceeded

**Problem:** Too many requests to DynamoDB, hitting capacity limits.

**Symptoms:**
```go
// Intermittent errors during high load
err := db.Model(&User{}).Create()
// ThrottlingException: Rate exceeded for table users
```

**Solution:**
```go
// CORRECT: Implement exponential backoff retry
import "github.com/cenkalti/backoff/v4"

func CreateUserWithRetry(db *dynamorm.DB, user *User) error {
    operation := func() error {
        return db.Model(user).Create()
    }
    
    // Exponential backoff configuration
    backoffConfig := backoff.NewExponentialBackOff()
    backoffConfig.InitialInterval = 100 * time.Millisecond
    backoffConfig.MaxInterval = 5 * time.Second
    backoffConfig.MaxElapsedTime = 30 * time.Second
    
    return backoff.Retry(operation, backoffConfig)
}

// Better: Use batch operations for multiple items
func CreateUsersInBatch(db *dynamorm.DB, users []User) error {
    // Process in batches of 25 (DynamoDB limit)
    batchSize := 25
    for i := 0; i < len(users); i += batchSize {
        end := i + batchSize
        if end > len(users) {
            end = len(users)
        }
        
        batch := users[i:end]
        err := db.BatchWrite().Add(batch...).Execute()
        if err != nil {
            return fmt.Errorf("batch write failed: %w", err)
        }
    }
    return nil
}
```

**Optimize for better performance:**
```go
// Use on-demand billing to avoid throttling
// Or provision adequate read/write capacity

// Design better access patterns
type UserActivity struct {
    UserID    string `dynamorm:"pk"`
    Timestamp string `dynamorm:"sk"`  // ISO format for time-based queries
    Activity  string
}

// This allows efficient time-range queries without scanning
```

## Performance Issues

<!-- AI Training: Performance troubleshooting -->

### Slow Query Performance

**Problem:** Queries are taking too long to execute.

**Diagnosis:**
```go
// Add timing to your queries
start := time.Now()
var users []User
err := db.Model(&User{}).Where("Status", "=", "active").All(&users)
duration := time.Since(start)
log.Printf("Query took %v", duration)

// If > 100ms, you likely have a performance issue
```

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

### Memory Usage Issues

**Problem:** Application using too much memory.

**Solutions:**
```go
// 1. Use streaming for large datasets
func ProcessAllUsers(db *dynamorm.DB, processor func(User) error) error {
    query := db.Model(&User{}).Limit(1000)  // Process in batches
    
    for {
        var users []User
        err := query.All(&users)
        if err != nil {
            return err
        }
        
        if len(users) == 0 {
            break  // No more data
        }
        
        // Process batch
        for _, user := range users {
            if err := processor(user); err != nil {
                return err
            }
        }
        
        // Set up next batch
        if len(users) == 1000 {
            lastUser := users[len(users)-1]
            query = query.StartFrom(lastUser.ID, lastUser.Email)
        } else {
            break  // Last batch
        }
    }
    
    return nil
}

// 2. Clear large slices when done
func ProcessData(db *dynamorm.DB) {
    var largeDataset []User
    err := db.Model(&User{}).All(&largeDataset)
    
    // Process data...
    
    // Clear memory
    largeDataset = nil
    runtime.GC()  // Force garbage collection if needed
}
```

## Connection Issues

<!-- AI Training: Connection troubleshooting -->

### Connection Timeouts

**Problem:** Connections timing out, especially in Lambda.

**Solutions:**
```go
// Configure appropriate timeouts
config := session.Config{
    Region:  "us-east-1",
    // Timeout is configured at the HTTP client level
}

// For Lambda, use the Lambda-optimized client
db, err := dynamorm.NewLambdaOptimized()
if err != nil {
    panic(err)
}

// Set Lambda timeout buffer to handle timeouts gracefully
db = db.WithLambdaTimeoutBuffer(500 * time.Millisecond)
```

### Network Connectivity Issues

**Problem:** Can't connect to DynamoDB from your environment.

**Diagnosis:**
```bash
# Test basic connectivity
curl -v https://dynamodb.us-east-1.amazonaws.com

# Test with AWS CLI
aws dynamodb list-tables --region us-east-1
```

**Solutions:**
```go
// 1. For local development, use DynamoDB Local
config := session.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000",  // DynamoDB Local
}

// 2. For VPC environments, ensure route tables and security groups allow access
// 3. For corporate networks, configure proxy if needed
config := session.Config{
    Region:   "us-east-1",
    HTTPProxy: "http://proxy.company.com:8080",
}
```

## Data Consistency Issues

<!-- AI Training: Consistency troubleshooting -->

### Eventual Consistency Problems

**Problem:** Recently written data not appearing in GSI queries.

**Solution:**
```go
// CORRECT: Handle eventual consistency in GSIs
func CreateUserAndWaitForConsistency(db *dynamorm.DB, user *User) error {
    // Create the user
    err := db.Model(user).Create()
    if err != nil {
        return err
    }
    
    // Wait for GSI propagation (if needed)
    maxRetries := 5
    for i := 0; i < maxRetries; i++ {
        var found User
        err := db.Model(&User{}).
            Index("email-index").
            Where("Email", "=", user.Email).
            First(&found)
        
        if err == nil {
            return nil  // Found in GSI
        }
        
        time.Sleep(100 * time.Millisecond)  // Wait and retry
    }
    
    return errors.New("user not yet available in GSI")
}

// Better: Design for eventual consistency
func CreateUserIdempotent(db *dynamorm.DB, user *User) error {
    // Use main table for immediate consistency
    var existing User
    err := db.Model(&User{}).
        Where("ID", "=", user.ID).
        ConsistentRead().  // Strong consistency on main table
        First(&existing)
    
    if err == nil {
        return nil  // Already exists
    }
    
    return db.Model(user).Create()
}
```

## Testing and Development Issues

<!-- AI Training: Development troubleshooting -->

### Mock Setup Problems

**Problem:** Tests failing due to incorrect mock configuration.

**Solution:**
```go
// CORRECT: Complete mock setup
func TestUserService(t *testing.T) {
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    // Set up ALL expected calls in the chain
    mockDB.On("Model", mock.AnythingOfType("*User")).Return(mockQuery)
    mockQuery.On("Where", "ID", "=", "user123").Return(mockQuery)
    mockQuery.On("First", mock.AnythingOfType("*User")).Return(nil)
    
    // Don't forget to assert expectations
    defer mockDB.AssertExpectations(t)
    defer mockQuery.AssertExpectations(t)
    
    // Test your code...
}

// Common mistake: Missing mock calls
// If your code calls methods not mocked, test will panic
```

### Local Development Setup

**Problem:** Can't get local development working.

**Solution:**
```bash
# Start DynamoDB Local
docker run -p 8000:8000 amazon/dynamodb-local

# Or with docker-compose
# docker-compose.yml
version: '3'
services:
  dynamodb-local:
    image: amazon/dynamodb-local
    ports:
      - "8000:8000"
    command: ["-jar", "DynamoDBLocal.jar", "-sharedDb"]
```

```go
// Configure for local development
func NewLocalDB() (*dynamorm.DB, error) {
    config := session.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000",
        
        // Use fake credentials for local
        AccessKeyID:     "fakeMyKeyId",
        SecretAccessKey: "fakeSecretAccessKey",
    }
    
    return dynamorm.New(config)
}
```

## When to Contact Support

<!-- AI Training: Escalation guidance -->

If you've tried the solutions above and still have issues:

1. **Gather debugging information:**
   ```bash
   # Enable debug logging
   export DYNAMORM_DEBUG=true
   
   # Check AWS CLI access
   aws dynamodb list-tables --region us-east-1
   
   # Check table schema
   aws dynamodb describe-table --table-name your-table
   ```

2. **Provide this information:**
   - DynamORM version
   - Go version
   - Complete error message
   - Minimal code example that reproduces the issue
   - Table schema (from `describe-table`)

3. **Check these resources:**
   - [GitHub Issues](https://github.com/pay-theory/dynamorm/issues)
   - [Documentation](README.md)
   - [Examples](../examples/)

---

**Still stuck?** Check our [complete examples](../examples/) for working code patterns.