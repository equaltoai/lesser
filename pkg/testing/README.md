# Lesser Testing Infrastructure

This package provides comprehensive testing infrastructure for the Lesser project, including integration test harness, mock services, test data factories, and performance benchmarks.

**Note**: This document covers the enhanced testing infrastructure. For basic Lift/DynamORM testing patterns, see the [original testing guide](#original-testing-guide) below.

## Overview

The testing infrastructure is organized into several key components:

- **Harness** (`harness/`): Integration test utilities and API clients
- **Factories** (`factories/`): Test data generators for consistent test data
- **Mocks** (`mocks/`): Enhanced mock implementations for external services
- **Benchmarks** (`benchmarks/`): Performance benchmarks for critical paths

## Quick Start

### Integration Testing

```go
import (
    "github.com/equaltoai/lesser/pkg/testing/harness"
    "github.com/equaltoai/lesser/pkg/testing/factories"
)

func TestAPIIntegration(t *testing.T) {
    // Create test harness
    harness := harness.NewIntegrationTestHarness(t, nil)
    
    // Start test server with your app
    harness.StartServer(myApp)
    
    // Create test data
    actor := harness.CreateTestActor("testuser")
    
    // Make API requests
    resp := harness.MakeRequest("GET", "/users/testuser", nil, nil)
    assert.Equal(t, 200, resp.StatusCode)
}
```

### Test Data Factories

```go
import "github.com/equaltoai/lesser/pkg/testing/factories"

func TestWithFactories(t *testing.T) {
    // Create actor factory
    actorFactory := factories.NewActorFactory("test.example.com")
    
    // Generate test actors
    actor := actorFactory.CreateActor(factories.ActorOptions{
        Username: "testuser",
        Bot:      false,
        Locked:   false,
    })
    
    // Create timeline scenarios
    timelineFactory := factories.NewTimelineFactory("test.example.com")
    timeline := timelineFactory.CreateTimelineScenario("user", factories.MixedTimeline)
}
```

### Benchmarking

```go
import "github.com/equaltoai/lesser/pkg/testing/benchmarks"

func BenchmarkStorageOperations(b *testing.B) {
    storage := mocks.NewEnhancedMockStorage()
    suite := benchmarks.NewStorageBenchmarkSuite(storage)
    
    suite.Setup(b)
    suite.RunAllBenchmarks(b)
}
```

## Components

### Integration Test Harness

The integration test harness (`harness/`) provides:

- **IntegrationTestHarness**: Complete testing environment setup
- **APIClient**: HTTP client for API testing
- **MastodonAPIClient**: Mastodon-specific API methods
- **ActivityPubClient**: ActivityPub protocol testing
- **TestAssertions**: Common test assertions

Features:
- Automatic test data cleanup
- Configurable storage backends (memory/DynamoDB)
- Built-in test server management
- Timeout and error handling
- Comprehensive logging

### Test Data Factories

The factories (`factories/`) provide consistent test data generation:

- **ActorFactory**: Creates actors, users, bots, and locked accounts
- **ActivityFactory**: Generates ActivityPub activities, notes, and interactions
- **TimelineFactory**: Creates timeline scenarios for different test cases

Timeline scenarios:
- `EmptyTimeline`: No content
- `SimpleTimeline`: Basic posts
- `MixedTimeline`: Posts, replies, boosts, likes
- `HighVolumeTimeline`: Many posts for performance testing
- `ConversationTimeline`: Threaded conversations

### Enhanced Mock Services

The mocks (`mocks/`) provide sophisticated mock implementations:

- **EnhancedMockStorage**: Stateful storage mock with relationship tracking
- **MockExternalService**: HTTP service mock with request logging
- **MockLogger**: Logger that captures log entries

Mock features:
- Configurable latency simulation
- Error rate simulation
- Operation counting
- State management
- Request/response logging

### Performance Benchmarks

The benchmarks (`benchmarks/`) provide performance testing for:

- **Storage operations**: Create/read/update/delete performance
- **API endpoints**: HTTP request/response benchmarks
- **Federation**: ActivityPub protocol performance
- **Memory usage**: Allocation and garbage collection
- **Concurrent access**: Thread safety and scalability

## CLI Commands

The repo standard is to use the `lesser` CLI for test workflows.

### Basic testing
```bash
./lesser test
./lesser test unit
./lesser test integration
```

### Coverage / race
```bash
./lesser test coverage
./lesser test race
```

### Benchmarks / package-only
Use `go test` directly:
```bash
go test ./pkg/storage/...
go test -bench=. ./...
```

### Load testing
See `tests/README.md` (k6 scripts under `tests/load/` and `tests/k6/`).

## Configuration

### Test Configuration

```go
config := &harness.TestConfig{
    Domain:        "test.example.com",
    TableName:     "test-table",
    UseMemory:     true,              // Use in-memory storage
    LogLevel:      zaptest.WarnLevel, // Reduce log noise
    ServerTimeout: 30 * time.Second,
    CleanupMode:   harness.CleanupOnSuccess, // Clean up on success only
}
```

### Mock Configuration

```go
storage := mocks.NewEnhancedMockStorage()
storage.SetLatencySimulation(5 * time.Millisecond) // Add 5ms latency
storage.SetErrorRate(0.01) // 1% error rate
```

## Best Practices

### Integration Tests

1. **Use the harness**: Always use `IntegrationTestHarness` for integration tests
2. **Clean test data**: Configure appropriate cleanup mode
3. **Use factories**: Generate consistent test data with factories
4. **Test realistic scenarios**: Use timeline scenarios that match real usage
5. **Verify behavior**: Use assertions to validate expected behavior

### Unit Tests

1. **Use table-driven tests**: For testing multiple scenarios
2. **Mock external dependencies**: Use enhanced mocks for external services
3. **Test edge cases**: Include error conditions and boundary cases
4. **Keep tests fast**: Use in-memory storage and minimal setup

### Benchmarks

1. **Establish baselines**: Record performance baselines for comparison
2. **Test realistic loads**: Use scenarios that match production usage
3. **Monitor memory**: Use `-benchmem` flag to track allocations
4. **Test concurrency**: Use parallel benchmarks for concurrent access patterns

### Coverage

1. **Aim for meaningful coverage**: Use `./lesser test coverage` and review `coverage.out` / `coverage.html`
2. **Focus on critical paths**: Ensure high coverage for important functionality
3. **Don't chase 100%**: Focus on meaningful tests over coverage percentage
4. **Review regularly**: Use detailed coverage reports to identify gaps

## Examples

See the example test files for complete usage examples:
- `harness/example_test.go`: Integration testing examples
- `benchmarks/example_test.go`: Benchmarking examples

## Contributing

When adding new test infrastructure:

1. **Follow existing patterns**: Use the established structure and naming
2. **Add documentation**: Include clear documentation and examples
3. **Test your tests**: Ensure test utilities work correctly
4. **Update the `lesser` CLI**: Keep test workflows user-facing and consistent
5. **Benchmark critical paths**: Add benchmarks for performance-sensitive code

## Environment Variables

- `CI`: Set to enable CI-specific behavior
- `INTEGRATION_TEST`: Set to enable integration tests
- `TEST_ENV`: Set to `integration` for integration test mode

## Dependencies

- `testify`: Assertions and testing utilities
- `zap`: Logging (with zaptest for testing)
- `lift`: Web framework for API testing
- `dynamorm`: ORM for DynamoDB testing
- `entr`: File watching (optional; use with `go test` in a local loop)
- `k6`: Load testing tool (optional)

This testing infrastructure provides a solid foundation for maintaining code quality and performance in the Lesser project.

---

## Original Testing Guide

The following section covers basic Lift and DynamORM testing patterns:

## Key Principles

1. **Use Interface-Based Mocking**: Mock at the interface level, not AWS SDK types
2. **Leverage Framework Utilities**: Use Lift's `TestApp` and DynamORM's mocks
3. **Table-Driven Tests**: Use Go's table-driven test pattern
4. **Integration Over Unit**: Focus on testing complete flows

## Testing Lift Handlers

### Using Lift's TestApp

```go
import lifttesting "github.com/pay-theory/lift/pkg/testing"

func TestHandler(t *testing.T) {
    // Create test app
    testApp := lifttesting.NewTestApp()
    
    // Add your handler
    testApp.App().POST("/api/endpoint", yourHandler)
    
    // Start the app
    err := testApp.Start()
    require.NoError(t, err)
    defer testApp.Stop()
    
    // Make requests
    resp := testApp.
        WithHeader("Authorization", "Bearer token").
        POST("/api/endpoint", requestBody)
    
    // Assert response
    assert.Equal(t, 200, resp.StatusCode)
}
```

### Testing with Dependencies

```go
func TestHandlerWithStorage(t *testing.T) {
    // Create mock storage (implements Storage interface)
    mockStorage := NewMockStorage()
    mockStorage.On("GetUser", mock.Anything, "user123").
        Return(&User{ID: "user123", Name: "Test"}, nil)
    
    // Create handler with mock
    handler := NewUserHandler(mockStorage)
    
    // Test using TestApp
    testApp := lifttesting.NewTestApp()
    testApp.App().GET("/users/:id", handler.GetUser)
    
    err := testApp.Start()
    require.NoError(t, err)
    defer testApp.Stop()
    
    resp := testApp.GET("/users/user123")
    assert.Equal(t, 200, resp.StatusCode)
    
    // Verify mock was called
    mockStorage.AssertExpectations(t)
}
```

## Testing DynamORM Repositories

### Using DynamORM Mocks

```go
import (
    "github.com/pay-theory/dynamorm/pkg/mocks"
    lifttesting "github.com/pay-theory/lift/pkg/testing"
)

func TestRepository(t *testing.T) {
    // Option 1: Use DynamORM's MockDB
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    mockDB.On("Model", mock.Anything).Return(mockQuery)
    mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
    mockQuery.On("First", mock.Anything).Return(nil)
    
    repo := NewUserRepository(mockDB)
    user, err := repo.GetByID(ctx, "123")
    
    // Option 2: Use Lift's MockDynamORM (higher level)
    mockDynamORM := lifttesting.NewMockDynamORM()
    mockDynamORM.WithData("users", map[string]any{
        "user-123": map[string]any{
            "pk": "USER#user-123",
            "sk": "USER#user-123",
            "username": "testuser",
        },
    })
    
    repo := NewUserRepository(mockDynamORM)
    user, err := repo.GetByID(ctx, "user-123")
}
```

### Testing Transactions

```go
func TestTransaction(t *testing.T) {
    mockDB := new(mocks.MockDB)
    
    // Mock transaction
    mockDB.On("Transaction", mock.Anything).Return(nil)
    
    repo := NewRepository(mockDB)
    err := repo.CreateWithTransaction(ctx, item)
    
    assert.NoError(t, err)
    mockDB.AssertExpectations(t)
}
```

## Integration Testing

### Testing Complete Flows

```go
func TestCompleteStatusFlow(t *testing.T) {
    // Setup test environment
    testApp := lifttesting.NewTestApp()
    mockDynamORM := lifttesting.NewMockDynamORM()
    
    // Wire up dependencies
    storage := dynamodb.NewStorage(mockDynamORM)
    handlers := api.NewHandlers(storage)
    
    // Register routes
    testApp.App().POST("/api/v1/statuses", handlers.CreateStatus)
    testApp.App().GET("/api/v1/statuses/:id", handlers.GetStatus)
    
    err := testApp.Start()
    require.NoError(t, err)
    defer testApp.Stop()
    
    // Create status
    createResp := testApp.
        WithAuth(&lifttesting.AuthConfig{UserID: "user123"}).
        POST("/api/v1/statuses", map[string]any{
            "status": "Hello, world!",
        })
    
    assert.Equal(t, 201, createResp.StatusCode)
    
    var status Status
    err = json.Unmarshal(createResp.Body, &status)
    require.NoError(t, err)
    
    // Get status
    getResp := testApp.GET("/api/v1/statuses/" + status.ID)
    assert.Equal(t, 200, getResp.StatusCode)
}
```

### Testing Event Handlers

```go
func TestDynamoDBStreamHandler(t *testing.T) {
    handler := NewStreamHandler(mockStorage)
    
    // Create test event
    event := events.DynamoDBEvent{
        Records: []events.DynamoDBEventRecord{
            {
                EventName: "INSERT",
                Change: events.DynamoDBStreamRecord{
                    NewImage: map[string]events.DynamoDBAttributeValue{
                        "pk": events.NewStringAttribute("STATUS#123"),
                        "sk": events.NewStringAttribute("STATUS#123"),
                    },
                },
            },
        },
    }
    
    // Process event
    err := handler.Handle(context.Background(), event)
    assert.NoError(t, err)
}
```

## Testing Patterns

### Table-Driven Tests

```go
func TestUserValidation(t *testing.T) {
    tests := []struct {
        name    string
        user    User
        wantErr bool
        errMsg  string
    }{
        {
            name:    "valid user",
            user:    User{Username: "valid", Email: "test@example.com"},
            wantErr: false,
        },
        {
            name:    "missing username",
            user:    User{Email: "test@example.com"},
            wantErr: true,
            errMsg:  "username required",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateUser(tt.user)
            if tt.wantErr {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Testing Error Cases

```go
func TestErrorHandling(t *testing.T) {
    mockStorage := NewMockStorage()
    
    // Test various error scenarios
    t.Run("not found", func(t *testing.T) {
        mockStorage.On("GetUser", mock.Anything, "missing").
            Return(nil, storage.ErrNotFound)
        
        handler := NewUserHandler(mockStorage)
        resp := testRequest(handler, "GET", "/users/missing")
        
        assert.Equal(t, 404, resp.StatusCode)
    })
    
    t.Run("internal error", func(t *testing.T) {
        mockStorage.On("GetUser", mock.Anything, "error").
            Return(nil, errors.New("database error"))
        
        handler := NewUserHandler(mockStorage)
        resp := testRequest(handler, "GET", "/users/error")
        
        assert.Equal(t, 500, resp.StatusCode)
    })
}
```

## Performance Testing

### Using Lift's Benchmark Suite

```go
func BenchmarkHandler(b *testing.B) {
    testApp := lifttesting.NewTestApp()
    testApp.App().POST("/api/endpoint", handler)
    
    err := testApp.Start()
    require.NoError(b, err)
    defer testApp.Stop()
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        resp := testApp.POST("/api/endpoint", testData)
        if resp.StatusCode != 200 {
            b.Fatalf("unexpected status: %d", resp.StatusCode)
        }
    }
}
```

### Load Testing

```go
func TestLoadScenario(t *testing.T) {
    testApp := lifttesting.NewTestApp()
    // ... setup
    
    loadTester := lifttesting.NewLoadTester(testApp, &lifttesting.LoadTestConfig{
        Duration:        30 * time.Second,
        TargetRPS:       100,
        MaxConcurrency:  10,
    })
    
    result, err := loadTester.RunLoadTest(context.Background(), func(app *lifttesting.TestApp) *lifttesting.TestResponse {
        return app.POST("/api/endpoint", testData)
    })
    
    require.NoError(t, err)
    assert.Less(t, result.ErrorRate, 0.01) // <1% errors
    assert.Less(t, result.P95Latency, 100*time.Millisecond)
}
```

## Environment Setup

### Test Environment Variables

Create a `.env.test` file:

```bash
# Test database
TEST_DYNAMODB_TABLE=lesser-test
AWS_REGION=us-east-1

# Test auth
JWT_SECRET=test-secret
TEST_USER_ID=test-user-123

# Feature flags
ENABLE_FEDERATION=true
ENABLE_SEARCH=true
```

### Running Tests

```bash
# Run all tests
./lesser test

# Run specific package tests
go test ./pkg/storage/...

# Run with coverage
go test -cover ./...

# Run integration tests
./lesser test integration

# Run benchmarks
go test -bench=. ./...
```

## Best Practices

1. **Mock at the Right Level**: Mock your interfaces, not third-party libraries
2. **Use Test Fixtures**: Create reusable test data builders
3. **Test Behavior, Not Implementation**: Focus on what the code does, not how
4. **Clean Up**: Always clean up test data and resources
5. **Parallel Tests**: Use `t.Parallel()` where appropriate
6. **Descriptive Names**: Use clear test names that describe the scenario

## Common Pitfalls to Avoid

1. **Don't Mock AWS SDK Types**: Use interface-based mocks instead
2. **Don't Test Framework Code**: Trust that Lift and DynamORM work
3. **Don't Overuse Mocks**: Sometimes integration tests are clearer
4. **Don't Ignore Errors**: Always check errors in tests
5. **Don't Share State**: Each test should be independent

## Resources

- [Lift Testing Documentation](https://github.com/pay-theory/lift/tree/main/pkg/testing)
- [DynamORM Testing Guide](https://github.com/pay-theory/dynamorm/blob/main/docs/testing.md)
- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test)
- [Testify Documentation](https://github.com/stretchr/testify)
