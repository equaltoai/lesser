# Lambda Integration Test Suite

## Overview

The Lambda Integration Test Suite provides comprehensive, production-ready testing capabilities for AWS Lambda functions in the Lesser serverless ActivityPub implementation. This suite integrates with:

- **Lift Framework**: Type-safe Lambda testing with HTTP simulation
- **DynamORM**: Test data management with proper repository patterns
- **Test Factories**: Realistic test data generation
- **Performance Monitoring**: Cold start, warm start, and throughput analysis
- **Concurrency Testing**: Load testing with concurrent execution scenarios

## Key Features

### 🔧 Comprehensive Infrastructure Integration
- DynamORM repository pattern integration
- Lift testing framework compatibility
- Authentication service testing
- Real AWS service simulation

### 📊 Advanced Performance Monitoring
- Cold start vs warm start analysis
- Memory usage tracking
- Percentile-based latency analysis (P50, P95, P99)
- Throughput and concurrency metrics

### 🎯 Production-Ready Test Scenarios
- API Gateway integration testing
- ActivityPub federation testing
- Media processing workflows
- Background job processing (SQS)

### 🚀 Scalability Testing
- Concurrent execution testing
- Load testing with realistic traffic patterns
- Performance threshold validation

## Architecture

### Core Components

```go
type LambdaIntegrationTestSuite struct {
    // Core infrastructure
    config *config.Config
    logger *zap.Logger
    repos  core.RepositoryStorage
    db     dynamormCore.DB
    
    // Lift testing integration
    liftTestSuite *testing.IntegrationTestSuite
    testApp       *testing.TestApp
    
    // Test data management
    actorFactory    *factories.ActorFactory
    testDataManager *TestDataManager
    
    // Performance tracking
    performanceTracker *PerformanceTracker
    concurrencyTracker *ConcurrencyTracker
}
```

### Test Data Management

The suite uses DynamORM patterns for test data:

```go
// Automatically creates and tracks test data
DataRequirements: &TestDataRequirements{
    Users:    10,
    Actors:   5,
    Statuses: 50,
    Activities: 25,
    CustomData: map[string]interface{}{
        "federation_enabled": true,
    },
}
```

## Usage Examples

### Basic Integration Test

```go
func TestAPILambdaIntegration(t *testing.T) {
    handler := lambda.NewHandler(myLambdaFunction)
    suite := integration.NewLambdaIntegrationTestSuite(t, handler)
    
    testCase := integration.LambdaIntegrationTestCase{
        Name:        "API_Health_Check",
        Description: "Test API health endpoint",
        Event:       integration.BuildAPIGatewayEvent("GET", "/health", nil, nil),
        Timeout:     10 * time.Second,
        PerformanceThresholds: &integration.PerformanceThresholds{
            MaxColdStart:   5 * time.Second,
            MaxWarmStart:   2 * time.Second,
            MaxMemoryMB:    256,
            MinSuccessRate: 99.0,
        },
    }
    
    suite.RunIntegrationTest(testCase)
}
```

### Authenticated API Testing

```go
testCase := integration.LambdaIntegrationTestCase{
    Name:           "Authenticated_User_Timeline",
    RequiredAuth:   true,
    RequiredScopes: []string{"read"},
    DataRequirements: &integration.TestDataRequirements{
        Users:    1,
        Actors:   1,
        Statuses: 10,
    },
    ExecuteFunc: func(suite *integration.LambdaIntegrationTestSuite, event interface{}) (interface{}, error) {
        headers := map[string]string{
            "Authorization": "Bearer " + suite.GetTestToken("standard"),
        }
        authEvent := integration.BuildAPIGatewayEvent("GET", "/api/v1/timelines/home", nil, headers)
        return suite.invokeLambda(context.Background(), authEvent)
    },
}
```

### Concurrency Testing

```go
concurrencyTest := integration.LambdaConcurrencyTest{
    Name:               "High_Load_Timeline_Requests",
    ConcurrentRequests: 50,
    RequestBuilder: func(index int) interface{} {
        return integration.BuildAPIGatewayEvent("GET", "/api/v1/timelines/public", nil, nil)
    },
    MaxDuration: 30 * time.Second,
}

suite.RunConcurrencyTest(concurrencyTest)
```

### ActivityPub Federation Testing

```go
activityPubTest := integration.LambdaIntegrationTestCase{
    Name: "ActivityPub_Inbox_Processing",
    DataRequirements: &integration.TestDataRequirements{
        Actors: 2,
    },
    ExecuteFunc: func(suite *integration.LambdaIntegrationTestSuite, _ interface{}) (interface{}, error) {
        activity := map[string]interface{}{
            "@context": "https://www.w3.org/ns/activitystreams",
            "type":     "Follow",
            "actor":    "https://remote.example.com/users/remote_user",
            "object":   "https://test.example.com/users/local_user",
        }
        event := integration.BuildAPIGatewayEvent("POST", "/inbox", activity, map[string]string{
            "Content-Type": "application/activity+json",
        })
        return suite.invokeLambda(context.Background(), event)
    },
}
```

## Pre-built Test Scenarios

The suite includes pre-built test scenarios for common Lambda patterns:

### API Lambda Tests
```go
apiTests := suite.APILambdaTestScenarios()
// Includes: health checks, authentication, CRUD operations
```

### ActivityPub Lambda Tests
```go
federationTests := suite.ActivityPubLambdaTestScenarios()
// Includes: inbox processing, outbox delivery, signature validation
```

### Media Processing Tests
```go
mediaTests := suite.MediaProcessingLambdaTestScenarios()
// Includes: upload processing, transcoding, thumbnail generation
```

### Background Job Tests
```go
jobTests := suite.BackgroundJobLambdaTestScenarios()
// Includes: SQS processing, retry logic, DLQ handling
```

## Performance Analysis

The suite provides comprehensive performance analysis:

### Metrics Collected

- **Execution Metrics**: Invocations, successes, errors, timeouts
- **Performance Metrics**: Cold/warm starts, duration percentiles, throughput
- **Memory Metrics**: Peak usage, average usage across invocations
- **Concurrency Metrics**: Peak concurrency, queue depth, wait times

### Quality Assessment

Tests are automatically scored based on:
- Error rates
- Performance (P95 latency)
- Cold start frequency
- Throughput capabilities

Results: `EXCELLENT` | `GOOD` | `ACCEPTABLE` | `NEEDS_IMPROVEMENT` | `POOR`

## Configuration Options

### Test Suite Options

```go
suite := integration.NewLambdaIntegrationTestSuite(t, handler,
    integration.WithConfig(customConfig),
    integration.WithLogger(customLogger),
    integration.WithRepositories(customRepos),
    integration.WithDynamoDBClient(customDB),
)
```

### Performance Thresholds

```go
thresholds := &integration.PerformanceThresholds{
    MaxColdStart:   5 * time.Second,   // Maximum acceptable cold start
    MaxWarmStart:   2 * time.Second,   // Maximum acceptable warm start
    MaxMemoryMB:    512,               // Maximum memory usage
    MinSuccessRate: 99.0,              // Minimum success rate (%)
    MaxErrorRate:   1.0,               // Maximum error rate (%)
}
```

### Data Requirements

```go
dataReqs := &integration.TestDataRequirements{
    Users:        10,                   // Number of test users
    Actors:       5,                    // Number of test actors
    Statuses:     50,                   // Number of test statuses
    Activities:   25,                   // Number of test activities
    PreserveData: false,               // Clean up after test
    CustomData: map[string]interface{}{
        "feature_flags": []string{"new_timeline"},
    },
}
```

## Integration with Existing Infrastructure

### DynamORM Integration
- Uses proper repository patterns
- Maintains data consistency
- Tracks costs and performance

### Lift Framework Integration
- Type-safe HTTP testing
- Middleware testing
- Context management

### Authentication Integration
- JWT token generation
- Scope-based testing
- Multi-tenant support

## Best Practices

### 1. Test Organization
```go
func TestAPILambdaIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    suite := integration.NewLambdaIntegrationTestSuite(t, handler)
    defer suite.AddCleanup(func() error {
        return customCleanup()
    })
    
    // Group related tests
    apiTests := suite.APILambdaTestScenarios()
    suite.RunIntegrationTests(apiTests)
}
```

### 2. Performance Testing
- Set realistic performance thresholds
- Test both cold and warm start scenarios
- Include concurrency testing for user-facing endpoints

### 3. Data Management
- Use minimal test data sets
- Clean up after tests unless debugging
- Use factories for consistent test data

### 4. Error Testing
- Test error conditions explicitly
- Validate error responses
- Test timeout scenarios

## CI/CD Integration

### Environment Variables
```bash
INTEGRATION_TESTS=true          # Enable integration tests
AWS_REGION=us-east-1           # AWS region for testing
DYNAMODB_TABLE=lesser-test     # Test table name
```

### Test Commands
```bash
# Run all integration tests
go test -v ./pkg/testing/integration/... -tags=integration

# Run only fast integration tests
go test -v ./pkg/testing/integration/... -short

# Run with performance benchmarking
go test -v ./pkg/testing/integration/... -bench=.
```

## Troubleshooting

### Common Issues

1. **DynamoDB Connection Errors**
   - Ensure AWS credentials are configured
   - Check DynamoDB Local is running for local tests
   - Verify table permissions

2. **Performance Test Failures**
   - Cold starts may be slower in test environment
   - Adjust thresholds for local testing
   - Check system resources during testing

3. **Authentication Failures**
   - Verify test token generation
   - Check required scopes match endpoint requirements
   - Ensure auth middleware is configured

### Debug Mode
```go
suite := integration.NewLambdaIntegrationTestSuite(t, handler,
    integration.WithLogger(zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))),
)
```

## Migration from Minimal Testing

The new comprehensive integration test suite replaces minimal Lambda testing patterns:

### Before (Minimal)
```go
func TestLambda(t *testing.T) {
    result, err := handler.Invoke(context.Background(), event)
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

### After (Comprehensive)
```go
func TestLambda(t *testing.T) {
    suite := integration.NewLambdaIntegrationTestSuite(t, handler)
    
    testCase := integration.LambdaIntegrationTestCase{
        Name: "Comprehensive_Lambda_Test",
        Event: event,
        PerformanceThresholds: &integration.PerformanceThresholds{
            MaxColdStart:   5 * time.Second,
            MinSuccessRate: 99.0,
        },
        DataRequirements: &integration.TestDataRequirements{
            Users: 1,
        },
    }
    
    suite.RunIntegrationTest(testCase)
}
```

This provides:
- ✅ Performance monitoring
- ✅ Test data management
- ✅ Comprehensive metrics
- ✅ Quality assessment
- ✅ Production readiness validation