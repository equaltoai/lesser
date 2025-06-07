# Lesser Developer Guidelines

## Table of Contents
1. [Architecture Principles](#architecture-principles)
2. [Technology Stack](#technology-stack)
3. [Code Organization](#code-organization)
4. [Naming Conventions](#naming-conventions)
5. [Error Handling](#error-handling)
6. [Logging](#logging)
7. [Testing Strategy](#testing-strategy)
8. [Event-Driven Patterns](#event-driven-patterns)
9. [Cost Tracking](#cost-tracking)
10. [Moderation System](#moderation-system)
11. [Documentation](#documentation)
12. [Security](#security)
13. [Performance](#performance)

## Architecture Principles

### 1. Serverless-First Design
- Each Lambda function should have a single, well-defined responsibility
- Minimize cold start times by keeping functions lightweight
- Avoid long-running processes; use SQS for async work
- Initialize expensive resources outside handler for reuse

### 2. Event-Driven Architecture
- Use DynamoDB Streams and SQS for decoupling
- Design for eventual consistency
- Implement idempotency for all operations
- Every significant action should emit an event

### 3. Cost Optimization
- Track cost of every operation
- Minimize Lambda execution time
- Use efficient DynamoDB query patterns
- Cache aggressively where appropriate

### 4. Reactive Systems
- Changes propagate through event streams
- Use DynamoDB Streams as the source of truth
- Build systems that react to changes, not poll for them
- Enable time-travel debugging through event sourcing

### 5. Federation First
- Always consider federation implications
- Support both local and remote actors
- Implement proper HTTP signatures
- Handle federation failures gracefully

## Technology Stack

### Core Libraries

```go
// Configuration - We use simple env vars for Lambda
// Avoid heavy config libraries like viper to reduce cold starts
github.com/lesserdao/lesser/pkg/config

// Logging - Structured logging with zap
go.uber.org/zap

// AWS SDK v2 - Use v2 exclusively
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/service/dynamodb
github.com/aws/aws-sdk-go-v2/service/s3
github.com/aws/aws-sdk-go-v2/service/sqs
github.com/aws/aws-sdk-go-v2/service/comprehend
github.com/aws/aws-sdk-go-v2/service/bedrock


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
│   ├── api/                  # Mastodon API handlers
│   │   ├── handlers/         # Individual endpoint handlers
│   │   └── main.go          # API Lambda entry
│   ├── activitypub/         # Federation endpoints
│   ├── workers/             # Background processors
│   │   ├── activity-processor/
│   │   ├── search-indexer/
│   │   └── moderation-processor/
│   └── streams/             # Real-time endpoints
├── pkg/                      # Shared packages
│   ├── activitypub/         # ActivityPub types and logic
│   ├── storage/             # Storage interfaces and implementations
│   │   ├── interface.go
│   │   └── dynamodb/
│   ├── federation/          # Federation logic
│   ├── auth/                # Authentication/authorization
│   ├── moderation/          # Reactive moderation system
│   ├── search/              # Search strategies
│   ├── cost/                # Cost tracking
│   ├── trust/               # Trust graph
│   └── common/              # Common utilities
├── internal/                 # Internal packages (not for external use)
│   └── testutil/            # Test utilities
└── infra/                   # Infrastructure as code (Pulumi)
```

### Package Design Rules
1. Keep packages small and focused
2. Define interfaces in the package that uses them
3. Avoid circular dependencies
4. Prefer composition over inheritance
5. Use dependency injection for testability

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
var requiresModeration bool
```

#### Constants
```go
// Use PascalCase for exported constants
const MaxUploadSize = 10 * 1024 * 1024
const DefaultTrustScore = 50.0

// Use camelCase for unexported constants
const defaultTimeout = 30 * time.Second
const moderationQueueSize = 100

// Group related constants
const (
    StatusPending  = "pending"
    StatusAccepted = "accepted"
    StatusRejected = "rejected"
)

const (
    ActionAllow  = "allow"
    ActionFlag   = "flag"
    ActionHold   = "hold"
    ActionReject = "reject"
)
```

#### Types and Interfaces
```go
// Use PascalCase for types
type ActorProfile struct {
    Username string
    Domain   string
    TrustScore float64
}

// Interface names should describe behavior, often ending in -er
type Storer interface {
    Store(ctx context.Context, data []byte) error
}

type Moderator interface {
    Moderate(ctx context.Context, content Content) (*ModerationResult, error)
}

// Avoid Interface suffix
// Bad: StorageInterface
// Good: Storage
```

#### Files
- Use lowercase with underscores for file names: `actor_storage.go`
- Test files: `actor_storage_test.go`
- Group related functionality in the same file
- Keep files under 500 lines when possible

#### DynamoDB Keys
```go
// Use consistent prefixes for partition and sort keys
const (
    // Actor keys
    ActorPKPrefix     = "ACTOR#"      // PK: ACTOR#{username}
    ActivitySKPrefix  = "ACTIVITY#"   // SK: ACTIVITY#{timestamp}#{id}
    
    // Relationship keys
    FollowPKPrefix    = "FOLLOW#"     // PK: FOLLOW#{username}
    
    // Timeline keys
    TimelinePKPrefix  = "TIMELINE#"   // PK: TIMELINE#{username}#HOME
    
    // Moderation keys
    ModerationPKPrefix = "MODERATION#" // PK: MODERATION#EVENT#{id}
    
    // Cost tracking keys
    CostPKPrefix      = "COST#"       // PK: COST#{date}
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

// Moderation-specific errors
type ModerationError struct {
    Type   string
    Reason string
    Code   string
}
```

### Lambda Error Responses
```go
// Consistent error response structure with cost tracking
type ErrorResponse struct {
    Error   string         `json:"error"`
    Message string         `json:"message,omitempty"`
    Code    string         `json:"code,omitempty"`
    Cost    *OperationCost `json:"cost,omitempty"`
}

// Helper function for error responses
func errorResponse(statusCode int, err error, cost *OperationCost) events.APIGatewayProxyResponse {
    body, _ := json.Marshal(ErrorResponse{
        Error: http.StatusText(statusCode),
        Message: err.Error(),
        Cost: cost,
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
// Include request ID and cost for tracing
logger.Info("processing activity",
    zap.String("request_id", requestID),
    zap.String("activity_type", activity.Type),
    zap.String("actor", activity.Actor),
    zap.Int64("cost_micros", cost.TotalCostMicros),
)

// Log moderation events with full context
logger.Info("moderation decision",
    zap.String("event_id", event.ID),
    zap.String("action", event.Action),
    zap.Float64("confidence", event.Confidence),
    zap.Int("reviewers", len(event.Reviewers)),
)

// Log errors with context
logger.Error("failed to store activity",
    zap.Error(err),
    zap.String("activity_id", activity.ID),
    zap.String("username", username),
    zap.Int64("cost_micros", cost.TotalCostMicros),
)
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

// Test moderation flows
func TestModerationMesh_FlagContent_TriggersReview(t *testing.T) {
    // Test the reactive moderation system
}
```

### Testing Levels

#### 1. Unit Tests
- Test individual functions in isolation
- Mock external dependencies (DynamoDB, S3, etc.)
- Aim for >80% code coverage
- Test cost calculation logic thoroughly
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

// Test event streams
func TestDynamoDBStreams_Integration(t *testing.T) {
    // Test that events flow through the system
}
```

#### 3. End-to-End Tests
- Test complete ActivityPub flows
- Use Docker Compose for local testing environment
- Test federation with mock ActivityPub servers
- Test moderation consensus flows
- Verify cost tracking accuracy

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

// Mock moderation service
type MockModerator struct {
    mock.Mock
}

func (m *MockModerator) Moderate(ctx context.Context, content Content) (*ModerationResult, error) {
    args := m.Called(ctx, content)
    return args.Get(0).(*ModerationResult), args.Error(1)
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

## Event-Driven Patterns

### Event Types
```go
// Define clear event types
type EventType string

const (
    // Activity events
    EventActivityCreated   EventType = "activity.created"
    EventActivityDelivered EventType = "activity.delivered"
    
    // Moderation events
    EventContentFlagged    EventType = "content.flagged"
    EventConsensusReached  EventType = "consensus.reached"
    
    // Trust events
    EventTrustUpdated      EventType = "trust.updated"
    EventReputationChanged EventType = "reputation.changed"
)
```

### Event Processing
```go
// Process DynamoDB Stream records
func ProcessStreamRecord(record events.DynamoDBEventRecord) error {
    switch record.EventName {
    case "INSERT":
        return handleInsert(record.Change.NewImage)
    case "MODIFY":
        return handleModify(record.Change.OldImage, record.Change.NewImage)
    case "REMOVE":
        return handleRemove(record.Change.OldImage)
    }
    return nil
}

// Emit events for downstream processing
func emitEvent(ctx context.Context, event Event) error {
    // Send to SQS for processing
    // Track cost of event emission
    return nil
}
```

## Cost Tracking

### Implementation Pattern
```go
// Track costs at every layer
type CostTracker struct {
    lambdaInvocations int64
    dynamoDBReads     int64
    dynamoDBWrites    int64
    s3Operations      int64
    dataTransfer      int64
}

func (c *CostTracker) AddDynamoDBRead(units int64) {
    atomic.AddInt64(&c.dynamoDBReads, units)
}

func (c *CostTracker) Calculate() *OperationCost {
    return &OperationCost{
        LambdaInvocations: c.lambdaInvocations,
        DynamoDBReads:     c.dynamoDBReads,
        DynamoDBWrites:    c.dynamoDBWrites,
        TotalCostMicros:   c.calculateTotal(),
    }
}

// Include cost in all responses
type Response struct {
    Data interface{}    `json:"data"`
    Cost *OperationCost `json:"cost"`
}
```

### Cost Optimization Rules
1. Batch DynamoDB operations when possible
2. Use projection expressions to reduce data transfer
3. Cache frequently accessed data in Lambda memory
4. Use CloudFront for media delivery
5. Monitor and alert on cost anomalies

## Moderation System

### Moderation Events
```go
// Moderation flows through events
type ModerationEvent struct {
    ID         string
    Type       EventType
    Target     string
    Actor      string
    Confidence float64
    Evidence   []Evidence
}

// Process moderation in stages
func ProcessModeration(ctx context.Context, event ModerationEvent) error {
    // 1. AI pre-screening
    aiResult := aiModerator.Screen(ctx, event.Target)
    
    // 2. Check confidence threshold
    if aiResult.Confidence > 0.95 {
        return autoModerate(ctx, event, aiResult)
    }
    
    // 3. Queue for human review
    return queueForReview(ctx, event)
}
```

### Trust Graph
```go
// Trust relationships affect moderation weight
type TrustEdge struct {
    From       string
    To         string
    Category   string
    Weight     float64
    Confidence float64
}

// Query trust relationships
func GetTrustedReviewers(ctx context.Context, category string, count int) ([]string, error) {
    // Return reviewers with high trust in category
}
```

## Documentation

### Code Documentation
```go
// Package moderation implements the reactive moderation mesh for Lesser.
// It provides distributed, consensus-based content moderation using trust graphs
// and AI pre-screening.
package moderation

// ModerationMesh coordinates distributed moderation decisions across the network.
// It uses a trust graph to weight reviewer input and reaches consensus through
// a Byzantine fault-tolerant algorithm.
type ModerationMesh struct {
    storage Storage
    trust   TrustGraph
    ai      AIScreener
}

// Moderate initiates the moderation process for the given content.
// It returns immediately with a pending result and processes asynchronously.
// The actual moderation decision will be available via the returned event ID.
func (m *ModerationMesh) Moderate(ctx context.Context, content Content) (*ModerationResult, error) {
    // Implementation
}
```

### API Documentation
- Document all Lambda functions in their main.go files
- Include example requests and responses
- Document error conditions and status codes
- Include cost estimates for operations

### README Files
- Each package should have a README.md explaining its purpose
- Include usage examples
- Document any special considerations
- Explain the reactive patterns used

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

// Validate moderation inputs
func validateModerationRequest(req ModerationRequest) error {
    if req.TargetID == "" {
        return errors.New("target ID required")
    }
    if req.Confidence < 0 || req.Confidence > 1 {
        return errors.New("confidence must be between 0 and 1")
    }
    return nil
}
```

### Authentication & Authorization
- Verify HTTP signatures for all federation endpoints
- Use JWT tokens for client authentication
- Implement proper authorization checks
- Trust scores affect rate limits

### Sensitive Data
- Never log sensitive data (private keys, tokens)
- Encrypt private keys at rest in DynamoDB
- Use AWS KMS for key management
- Anonymize data in logs

## Performance

### Lambda Optimization
```go
// Initialize expensive resources outside handler
var (
    dynamoClient *dynamodb.Client
    logger       *zap.Logger
    costTracker  *CostTracker
    moderator    *ModerationMesh
)

func init() {
    // Initialize once, reuse across invocations
    cfg, _ := config.LoadDefaultConfig(context.Background())
    dynamoClient = dynamodb.NewFromConfig(cfg)
    logger, _ = zap.NewProduction()
    costTracker = NewCostTracker()
    moderator = NewModerationMesh(dynamoClient)
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    // Handler uses pre-initialized resources
    // Track costs throughout execution
}
```

### DynamoDB Best Practices
- Use batch operations where possible
- Design for single-table queries
- Avoid scans, use queries with indexes
- Implement exponential backoff for retries
- Use DynamoDB Streams for reactive updates

### Caching Strategy
- Cache actor profiles in Lambda memory
- Use Cache-Control headers appropriately
- Consider CloudFront for static content
- Cache trust scores with TTL
- Implement cache warming for hot paths

## Code Review Checklist

Before submitting a PR, ensure:
- [ ] Code follows naming conventions
- [ ] All functions have appropriate documentation
- [ ] Error handling is consistent
- [ ] Unit tests are included (>80% coverage)
- [ ] Cost tracking is implemented
- [ ] Moderation events are emitted where needed
- [ ] No sensitive data in logs
- [ ] DynamoDB queries are optimized
- [ ] Lambda functions follow cold-start optimization
- [ ] Dependencies are minimal and justified
- [ ] Code is formatted with `gofmt`
- [ ] No linter warnings (`golangci-lint run`)
- [ ] Event patterns are followed
- [ ] Trust graph updates are considered

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
make build-api

# Run integration tests
go test -tags=integration ./...

# Generate mocks
mockery --name=Storage --dir=pkg/storage --output=internal/testutil/mocks

# Test moderation flows
make test-moderation

# Simulate cost for operations
make cost-simulate

# Deploy to AWS
cd infra && pulumi up
```

## New Feature Guidelines

When implementing new features:
1. Consider federation implications
2. Add cost tracking from the start
3. Emit events for significant actions
4. Update the trust graph if relevant
5. Add to the GraphQL schema
6. Document in the API reference
7. Add integration tests
8. Update cost estimates

## Reactive Patterns

### Event Sourcing
- Every change is an event
- Events are immutable
- State is derived from events
- Enables time-travel debugging

### CQRS (Command Query Responsibility Segregation)
- Separate read and write models
- Optimize for different access patterns
- Use projections for complex queries

### Eventual Consistency
- Embrace async processing
- Design UIs that handle delays
- Use optimistic updates
- Provide status indicators

---

*Lesser is more than just another ActivityPub implementation. It's a demonstration that federated social media can be sustainable, transparent, and community-governed. These guidelines help us maintain that vision.* 