# Testing Guide for Lesser

This guide explains how to write tests for the Lesser project using Lift and DynamORM's built-in testing utilities.

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
make test

# Run specific package tests
go test ./pkg/storage/...

# Run with coverage
go test -cover ./...

# Run integration tests
make test-integration

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