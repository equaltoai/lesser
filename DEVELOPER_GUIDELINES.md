# Lesser Developer Guidelines

## Table of Contents
1. [Architecture Principles](#architecture-principles)
2. [Technology Stack](#technology-stack)
3. [Code Organization](#code-organization)
4. [Naming Conventions](#naming-conventions)
5. [Error Handling](#error-handling)
6. [Logging](#logging)
7. [Testing Strategy](#testing-strategy)
8. [Documentation](#documentation)
9. [Security](#security)
10. [Performance](#performance)

## Architecture Principles

### 1. Serverless-First Design
- Each Lambda function should have a single, well-defined responsibility
- Minimize cold start times by keeping functions lightweight
- Avoid long-running processes; use SQS for async work

### 2. Event-Driven Architecture
- Use DynamoDB Streams and SQS for decoupling
- Design for eventual consistency
- Implement idempotency for all operations

### 3. Cost Optimization
- Minimize Lambda execution time
- Use efficient DynamoDB query patterns
- Cache aggressively where appropriate

## Technology Stack

### Core Libraries

```go
// Configuration - We use simple env vars for Lambda
// Avoid heavy config libraries like viper to reduce cold starts
github.com/lesser/lesser/pkg/config

// Logging - Structured logging with zap
go.uber.org/zap

// AWS SDK v2
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/service/dynamodb
github.com/aws/aws-sdk-go-v2/service/s3
github.com/aws/aws-sdk-go-v2/service/sqs

// HTTP Signatures
github.com/go-fed/httpsig

// JSON handling
encoding/json // Prefer standard library for ActivityPub JSON-LD

// Testing
github.com/stretchr/testify/assert
github.com/stretchr/testify/mock
```

### Why Not Gin/Echo/Fiber?
Lambda functions use `events.APIGatewayProxyRequest` and don't need traditional HTTP routers. Using web frameworks adds unnecessary overhead and increases cold start times.

### Why Not Viper?
Lambda functions get configuration from environment variables. Viper adds complexity and size without benefit in a serverless context.

## Code Organization

### Project Structure
```
lesser/
├── cmd/                      # Lambda function entry points
│   ├── {function}/
│   │   ├── main.go          # Lambda handler
│   │   └── handler_test.go  # Handler tests
├── pkg/                      # Shared packages
│   ├── activitypub/         # ActivityPub types and logic
│   │   ├── types.go
│   │   ├── validation.go
│   │   └── marshal.go
│   ├── storage/             # Storage interfaces and implementations
│   │   ├── interface.go
│   │   ├── dynamodb/
│   │   │   ├── client.go
│   │   │   ├── actor.go
│   │   │   └── activity.go
│   ├── federation/          # Federation logic
│   │   ├── httpsig.go       # HTTP signature verification/signing
│   │   ├── delivery.go      # Activity delivery
│   │   └── discovery.go     # Remote actor discovery
│   ├── auth/                # Authentication/authorization
│   │   ├── jwt.go
│   │   └── middleware.go
│   └── common/              # Common utilities
│       ├── errors.go
│       ├── logging.go
│       └── response.go
├── internal/                 # Internal packages (not for external use)
│   └── testutil/            # Test utilities
└── infra/                   # Infrastructure as code (Pulumi)
```

### Package Design Rules
1. Keep packages small and focused
2. Define interfaces in the package that uses them, not the package that implements them
3. Avoid circular dependencies
4. Prefer composition over inheritance

## Naming Conventions

### General Rules
- Use meaningful, descriptive names
- Avoid abbreviations except for well-known ones (URL, API, etc.)
- Be consistent across the codebase

### Go-Specific Conventions

#### Variables and Functions
```go
// Use camelCase for variables and functions
userName := "alice"
func getUserProfile(username string) (*Actor, error)

// Use descriptive names for function parameters
func CreateActivity(ctx context.Context, actorID string, activity *Activity) error

// Boolean variables should be named as questions
var isPublic bool
var hasAttachment bool
```

#### Constants
```go
// Use PascalCase for exported constants
const MaxUploadSize = 10 * 1024 * 1024

// Use camelCase for unexported constants
const defaultTimeout = 30 * time.Second

// Group related constants
const (
    StatusPending  = "pending"
    StatusAccepted = "accepted"
    StatusRejected = "rejected"
)
```

#### Types and Interfaces
```go
// Use PascalCase for types
type ActorProfile struct {
    Username string
    Domain   string
}

// Interface names should describe behavior, often ending in -er
type Storer interface {
    Store(ctx context.Context, data []byte) error
}

// Avoid Interface suffix
// Bad: StorageInterface
// Good: Storage
```

#### Files
- Use lowercase with underscores for file names: `actor_storage.go`
- Test files: `actor_storage_test.go`
- Group related functionality in the same file

#### DynamoDB Keys
```go
// Use consistent prefixes for partition and sort keys
const (
    ActorPKPrefix     = "ACTOR#"      // PK: ACTOR#{username}
    ActivitySKPrefix  = "ACTIVITY#"   // SK: ACTIVITY#{timestamp}#{id}
    FollowPKPrefix    = "FOLLOW#"     // PK: FOLLOW#{username}
)
```

## Error Handling

### Error Design
```go
// Define domain-specific errors
type ActorNotFoundError struct {
    Username string
}

func (e ActorNotFoundError) Error() string {
    return fmt.Sprintf("actor not found: %s", e.Username)
}

// Use error wrapping for context
func GetActor(username string) (*Actor, error) {
    actor, err := db.GetItem(...)
    if err != nil {
        return nil, fmt.Errorf("failed to get actor %s: %w", username, err)
    }
    return actor, nil
}
```

### Lambda Error Responses
```go
// Consistent error response structure
type ErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message,omitempty"`
    Code    string `json:"code,omitempty"`
}

// Helper function for error responses
func errorResponse(statusCode int, err error) events.APIGatewayProxyResponse {
    body, _ := json.Marshal(ErrorResponse{
        Error: http.StatusText(statusCode),
        Message: err.Error(),
    })
    
    return events.APIGatewayProxyResponse{
        StatusCode: statusCode,
        Headers:    map[string]string{"Content-Type": "application/json"},
        Body:       string(body),
    }
}
```

## Logging

### Setup
```go
// Initialize logger in Lambda function
import "go.uber.org/zap"

var logger *zap.Logger

func init() {
    cfg := zap.NewProductionConfig()
    cfg.Level = zap.NewAtomicLevelAt(getLogLevel())
    
    logger, _ = cfg.Build()
    defer logger.Sync()
}
```

### Logging Standards
```go
// Include request ID for tracing
logger.Info("processing activity",
    zap.String("request_id", requestID),
    zap.String("activity_type", activity.Type),
    zap.String("actor", activity.Actor),
)

// Log errors with context
logger.Error("failed to store activity",
    zap.Error(err),
    zap.String("activity_id", activity.ID),
    zap.String("username", username),
)

// Use structured fields, not string concatenation
// Bad: logger.Info("Processing activity " + activityID)
// Good: logger.Info("processing activity", zap.String("activity_id", activityID))
```

### Log Levels
- **Debug**: Detailed information for debugging
- **Info**: General operational information
- **Warn**: Warning conditions that should be investigated
- **Error**: Error conditions that prevented an operation
- **Fatal**: Critical errors that cause Lambda termination (use sparingly)

## Testing Strategy

### Test Organization
```go
// Test file naming: {file}_test.go
// Test function naming: Test{Function}_{Scenario}_{ExpectedResult}

func TestGetActor_ValidUsername_ReturnsActor(t *testing.T) {
    // Arrange
    username := "testuser"
    mockStorage := &mocks.Storage{}
    mockStorage.On("GetActor", username).Return(&Actor{...}, nil)
    
    // Act
    actor, err := GetActor(mockStorage, username)
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, username, actor.Username)
    mockStorage.AssertExpectations(t)
}
```

### Testing Levels

#### 1. Unit Tests
- Test individual functions in isolation
- Mock external dependencies (DynamoDB, S3, etc.)
- Aim for >80% code coverage
- Run with: `make test`

#### 2. Integration Tests
```go
// Place in {package}_integration_test.go
// Use build tags to separate from unit tests
// +build integration

func TestDynamoDBStorage_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // Test against local DynamoDB
    storage := dynamodb.NewWithEndpoint("http://localhost:8000")
    // ... test real operations
}
```

#### 3. End-to-End Tests
- Test complete ActivityPub flows
- Use Docker Compose for local testing environment
- Test federation with mock ActivityPub servers

### Mocking
```go
// Define mocks in internal/testutil/mocks/
// Use mockery or hand-write mocks for interfaces

type MockStorage struct {
    mock.Mock
}

func (m *MockStorage) GetActor(ctx context.Context, username string) (*Actor, error) {
    args := m.Called(ctx, username)
    return args.Get(0).(*Actor), args.Error(1)
}
```

### Table-Driven Tests
```go
func TestValidateUsername(t *testing.T) {
    tests := []struct {
        name     string
        username string
        wantErr  bool
    }{
        {"valid username", "alice", false},
        {"empty username", "", true},
        {"special characters", "alice@bob", true},
        {"too long", strings.Repeat("a", 256), true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateUsername(tt.username)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

## Documentation

### Code Documentation
```go
// Package activitypub provides types and utilities for ActivityPub protocol.
// It implements the W3C ActivityPub specification for federated social networking.
package activitypub

// Actor represents an ActivityPub actor that can perform activities.
// Actors can be of type Person, Service, Group, Organization, or Application.
type Actor struct {
    // PreferredUsername is the actor's handle without the domain.
    // Example: "alice" (not "alice@example.com")
    PreferredUsername string `json:"preferredUsername"`
    
    // Inbox is the URL for receiving activities from other actors.
    // Must be an absolute URL.
    Inbox string `json:"inbox"`
}

// GetActor retrieves an actor by username from storage.
// Returns ActorNotFoundError if the actor doesn't exist.
func GetActor(username string) (*Actor, error) {
    // Implementation details...
}
```

### API Documentation
- Document all Lambda functions in their main.go files
- Include example requests and responses
- Document error conditions and status codes

### README Files
- Each package should have a README.md explaining its purpose
- Include usage examples
- Document any special considerations

## Security

### Input Validation
```go
// Validate all inputs at the edge (Lambda handlers)
func validateActorUsername(username string) error {
    if username == "" {
        return errors.New("username cannot be empty")
    }
    if len(username) > 255 {
        return errors.New("username too long")
    }
    if !usernameRegex.MatchString(username) {
        return errors.New("invalid username format")
    }
    return nil
}
```

### Authentication & Authorization
- Verify HTTP signatures for all federation endpoints
- Use JWT tokens for client authentication
- Implement proper authorization checks

### Sensitive Data
- Never log sensitive data (private keys, tokens)
- Encrypt private keys at rest in DynamoDB
- Use AWS KMS for key management

## Performance

### Lambda Optimization
```go
// Initialize expensive resources outside handler
var (
    dynamoClient *dynamodb.Client
    logger       *zap.Logger
)

func init() {
    // Initialize once, reuse across invocations
    cfg, _ := config.LoadDefaultConfig(context.Background())
    dynamoClient = dynamodb.NewFromConfig(cfg)
    logger, _ = zap.NewProduction()
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Handler uses pre-initialized resources
}
```

### DynamoDB Best Practices
- Use batch operations where possible
- Design for single-table queries
- Avoid scans, use queries with indexes
- Implement exponential backoff for retries

### Caching Strategy
- Cache actor profiles in Lambda memory
- Use Cache-Control headers appropriately
- Consider CloudFront for static content

## Code Review Checklist

Before submitting a PR, ensure:
- [ ] Code follows naming conventions
- [ ] All functions have appropriate documentation
- [ ] Error handling is consistent
- [ ] Unit tests are included (>80% coverage)
- [ ] No sensitive data in logs
- [ ] DynamoDB queries are optimized
- [ ] Lambda functions follow cold-start optimization
- [ ] Dependencies are minimal and justified
- [ ] Code is formatted with `gofmt`
- [ ] No linter warnings (`golangci-lint run`)

## Useful Commands

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Format code
make fmt

# Run linter
make lint

# Build specific Lambda
make build-webfinger

# Run integration tests
go test -tags=integration ./...

# Generate mocks
mockery --name=Storage --dir=pkg/storage --output=internal/testutil/mocks
``` 