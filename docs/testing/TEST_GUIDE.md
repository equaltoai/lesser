# Lesser Comprehensive Testing Guide

Lesser implements comprehensive testing infrastructure designed for serverless, federated social media with extensive coverage across unit tests, integration tests, federation tests, and performance benchmarks. This guide covers all testing approaches and best practices.

## Testing Philosophy

Lesser embraces multi-layered testing to ensure reliability, performance, and compatibility:

1. **Unit Tests** - Component-level testing with mocks
2. **Integration Tests** - End-to-end API testing  
3. **Federation Tests** - ActivityPub protocol compliance
4. **Performance Tests** - Load testing and benchmarks
5. **Security Tests** - Authentication and authorization
6. **Cost Tests** - Resource usage and budget validation

## Test Architecture Overview

### Core Testing Components

#### Test Infrastructure
- **Testing Harness** (`pkg/testing/harness/`) - Integration test utilities
- **Test Factories** (`pkg/testing/factories/`) - Consistent test data generation
- **Enhanced Mocks** (`pkg/testing/mocks/`) - Sophisticated mock implementations
- **Benchmarks** (`pkg/testing/benchmarks/`) - Performance testing suite

#### Framework Integration
- **Lift Testing** - Web framework test utilities
- **DynamORM Mocks** - Database layer testing
- **AWS Service Mocks** - External service simulation
- **ActivityPub Mocks** - Federation protocol testing

## Unit Testing

### Go Unit Tests

Lesser uses standard Go testing with testify for enhanced assertions:

#### Basic Test Structure
```go
func TestUserService(t *testing.T) {
    // Setup
    mockStorage := mocks.NewMockStorage()
    service := NewUserService(mockStorage)
    
    // Test data
    testUser := &models.User{
        Username: "testuser",
        Email:    "test@example.com",
    }
    
    // Mock expectations
    mockStorage.On("CreateUser", mock.Anything, testUser).
        Return(testUser, nil)
    
    // Execute
    result, err := service.CreateUser(context.Background(), testUser)
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, testUser.Username, result.Username)
    mockStorage.AssertExpectations(t)
}
```

#### Table-Driven Tests
```go
func TestValidateUsername(t *testing.T) {
    tests := []struct {
        name     string
        username string
        wantErr  bool
        errMsg   string
    }{
        {"valid username", "validuser", false, ""},
        {"empty username", "", true, "username required"},
        {"too long", strings.Repeat("a", 100), true, "username too long"},
        {"invalid chars", "user@domain", true, "invalid characters"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateUsername(tt.username)
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

### Key Test Files

#### Authentication Tests
- `pkg/auth/oauth_test.go` - OAuth 2.0 flow testing
- `pkg/auth/refresh_tokens_test.go` - Token lifecycle testing  
- `pkg/auth/csrf_test.go` - CSRF protection testing

#### Federation Tests
- `pkg/federation/httpsig_test.go` - HTTP signature verification
- `pkg/federation/relationship_tracker_test.go` - Relationship tracking
- `pkg/activitypub/validation_test.go` - ActivityPub validation

#### Storage Tests
- `pkg/storage/repositories/*_test.go` - Repository layer testing
- `pkg/storage/models/*_test.go` - Data model testing
- `pkg/cost/dynamorm_tracker_test.go` - Cost tracking testing

## Integration Testing

### API Integration Tests

Lesser provides comprehensive API testing with Python pytest:

#### Core API Test Files
```bash
# Authentication and accounts
tests/test_api.py                    # Core API functionality
tests/test_account_search.py         # Account search
tests/test_oauth.py                  # OAuth flow testing

# Content and interactions  
tests/test_conversations.py          # Conversation threads
tests/test_favorites.py              # Favorite functionality
tests/test_polls.py                  # Poll functionality
tests/test_statuses.py               # Status operations

# Advanced features
tests/test_lists.py                  # List management
tests/test_filters_mutes.py          # Content filtering
tests/test_notifications.py         # Notification system
tests/test_streaming.py              # WebSocket streaming
```

#### Integration Test Harness
```go
import (
    "github.com/equaltoai/lesser/pkg/testing/harness"
    "github.com/equaltoai/lesser/pkg/testing/factories"
)

func TestAPIIntegration(t *testing.T) {
    // Create test harness
    harness := harness.NewIntegrationTestHarness(t, &harness.TestConfig{
        Domain:    "test.example.com",
        UseMemory: true,
        LogLevel:  zaptest.WarnLevel,
    })
    
    // Start test server
    harness.StartServer(createApp())
    defer harness.Cleanup()
    
    // Create test data
    actor := harness.CreateTestActor("testuser")
    
    // Test API endpoints
    resp := harness.MakeRequest("GET", "/users/testuser", nil, nil)
    assert.Equal(t, 200, resp.StatusCode)
    
    // Test authenticated requests
    token := harness.CreateAuthToken(actor.ID)
    resp = harness.MakeAuthenticatedRequest("POST", "/api/v1/statuses", 
        map[string]interface{}{"status": "Hello world"}, token)
    assert.Equal(t, 201, resp.StatusCode)
}
```

### Lift Framework Testing

#### Handler Testing with TestApp
```go
import lifttesting "github.com/pay-theory/lift/pkg/testing"

func TestStatusHandler(t *testing.T) {
    // Create test app
    testApp := lifttesting.NewTestApp()
    
    // Setup dependencies
    mockStorage := mocks.NewEnhancedMockStorage()
    handler := NewStatusHandler(mockStorage)
    
    // Register routes
    testApp.App().POST("/api/v1/statuses", handler.CreateStatus)
    testApp.App().GET("/api/v1/statuses/:id", handler.GetStatus)
    
    // Start app
    err := testApp.Start()
    require.NoError(t, err)
    defer testApp.Stop()
    
    // Test status creation
    resp := testApp.
        WithAuth(&lifttesting.AuthConfig{UserID: "user123"}).
        POST("/api/v1/statuses", map[string]any{
            "status": "Test post",
            "visibility": "public",
        })
    
    assert.Equal(t, 201, resp.StatusCode)
    
    var status models.Status
    err = json.Unmarshal(resp.Body, &status)
    require.NoError(t, err)
    
    // Test status retrieval
    getResp := testApp.GET("/api/v1/statuses/" + status.ID)
    assert.Equal(t, 200, getResp.StatusCode)
}
```

## Federation Testing

### ActivityPub Protocol Tests

#### Federation Test Files
```bash
# Federation protocol testing
tests/test_federation_complete.py    # Full federation test suite
tests/test_federation_search.py      # Federated search
tests/federation_test_harness.py     # Mock federation instance

# ActivityPub compliance
tests/test_activitypub_spec.py       # ActivityPub spec compliance
tests/test_activitypub_collections.py # Collections testing
```

#### HTTP Signature Testing
```go
func TestHTTPSignatureVerification(t *testing.T) {
    tests := []struct {
        name        string
        keyData     string
        signature   string
        expectValid bool
    }{
        {
            name:        "valid RSA signature",
            keyData:     testRSAPublicKey,
            signature:   validRSASignature,
            expectValid: true,
        },
        {
            name:        "invalid signature",
            keyData:     testRSAPublicKey,
            signature:   invalidSignature,
            expectValid: false,
        },
        {
            name:        "expired signature",
            keyData:     testRSAPublicKey,
            signature:   expiredSignature,
            expectValid: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            publicKey, err := parsePublicKey(tt.keyData)
            require.NoError(t, err)
            
            req := createTestRequest(tt.signature)
            err = VerifyHTTPSignature(req, publicKey)
            
            if tt.expectValid {
                assert.NoError(t, err)
            } else {
                assert.Error(t, err)
            }
        })
    }
}
```

#### Federation Mock Service
```go
type MockFederationInstance struct {
    Domain     string
    ActorStore map[string]*activitypub.Actor
    InboxItems []activitypub.Activity
    logger     *zap.Logger
}

func (m *MockFederationInstance) HandleWebFinger(resource string) (*WebFingerResponse, error) {
    // Mock WebFinger response
    return &WebFingerResponse{
        Subject: resource,
        Links: []WebFingerLink{
            {
                Rel:  "self",
                Type: "application/activity+json",
                Href: fmt.Sprintf("https://%s/users/%s", m.Domain, username),
            },
        },
    }, nil
}

func (m *MockFederationInstance) HandleInbox(activity *activitypub.Activity) error {
    // Store received activities for verification
    m.InboxItems = append(m.InboxItems, *activity)
    return nil
}
```

## Performance Testing

### Benchmarks

#### Storage Benchmarks
```go
func BenchmarkUserRepository(b *testing.B) {
    storage := setupBenchmarkStorage(b)
    repo := repositories.NewUserRepository(storage, zap.NewNop())
    
    user := &models.User{
        Username: "benchuser",
        Email:    "bench@example.com",
    }
    
    b.Run("CreateUser", func(b *testing.B) {
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            userCopy := *user
            userCopy.Username = fmt.Sprintf("benchuser%d", i)
            _, err := repo.CreateUser(context.Background(), &userCopy)
            if err != nil {
                b.Fatal(err)
            }
        }
    })
    
    b.Run("GetUser", func(b *testing.B) {
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            _, err := repo.GetUserByUsername(context.Background(), user.Username)
            if err != nil {
                b.Fatal(err)
            }
        }
    })
}
```

#### API Performance Benchmarks
```go
func BenchmarkAPIEndpoints(b *testing.B) {
    testApp := setupBenchmarkApp(b)
    
    b.Run("Timeline", func(b *testing.B) {
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            resp := testApp.GET("/api/v1/timelines/home")
            if resp.StatusCode != 200 {
                b.Fatalf("unexpected status: %d", resp.StatusCode)
            }
        }
    })
    
    b.Run("PostStatus", func(b *testing.B) {
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            resp := testApp.POST("/api/v1/statuses", map[string]any{
                "status": fmt.Sprintf("Benchmark post %d", i),
            })
            if resp.StatusCode != 201 {
                b.Fatalf("unexpected status: %d", resp.StatusCode)
            }
        }
    })
}
```

### Load Testing with k6

#### Load Test Configuration
```javascript
// k6/timeline_test.js
import http from 'k6/http';
import { check } from 'k6';
import { Rate } from 'k6/metrics';

export let errorRate = new Rate('errors');

export let options = {
  stages: [
    { duration: '30s', target: 10 },   // Ramp up
    { duration: '1m', target: 50 },    // Stay at 50 users
    { duration: '30s', target: 0 },    // Ramp down
  ],
  thresholds: {
    errors: ['rate<0.1'], // Error rate < 10%
    http_req_duration: ['p(95)<500'], // 95% of requests < 500ms
  },
};

export default function() {
  let response = http.get(`${__ENV.BASE_URL}/api/v1/timelines/home`, {
    headers: {
      'Authorization': `Bearer ${__ENV.ACCESS_TOKEN}`,
    },
  });
  
  let success = check(response, {
    'status is 200': (r) => r.status === 200,
    'response time < 500ms': (r) => r.timings.duration < 500,
  });
  
  errorRate.add(!success);
}
```

### Memory and Performance Profiling
```go
func TestMemoryUsage(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping memory test in short mode")
    }
    
    runtime.GC()
    var m1 runtime.MemStats
    runtime.ReadMemStats(&m1)
    
    // Run operation that should be memory-efficient
    for i := 0; i < 1000; i++ {
        processLargeTimeline()
    }
    
    runtime.GC()
    var m2 runtime.MemStats
    runtime.ReadMemStats(&m2)
    
    memUsed := m2.Alloc - m1.Alloc
    t.Logf("Memory used: %d bytes", memUsed)
    
    // Assert reasonable memory usage
    assert.Less(t, memUsed, uint64(50*1024*1024), "Memory usage too high")
}
```

## Test Data Factories

### Consistent Test Data Generation

```go
import "github.com/equaltoai/lesser/pkg/testing/factories"

func TestWithFactories(t *testing.T) {
    // Create actor factory
    actorFactory := factories.NewActorFactory("test.example.com")
    
    // Generate different actor types
    regularUser := actorFactory.CreateActor(factories.ActorOptions{
        Username: "regular_user",
        Bot:      false,
        Locked:   false,
    })
    
    botAccount := actorFactory.CreateActor(factories.ActorOptions{
        Username: "test_bot",
        Bot:      true,
        Locked:   false,
    })
    
    // Create timeline scenarios
    timelineFactory := factories.NewTimelineFactory("test.example.com")
    
    // Generate different timeline patterns
    emptyTimeline := timelineFactory.CreateTimelineScenario("empty", factories.EmptyTimeline)
    mixedTimeline := timelineFactory.CreateTimelineScenario("mixed", factories.MixedTimeline)
    conversationTimeline := timelineFactory.CreateTimelineScenario("conversation", factories.ConversationTimeline)
    
    // Use generated data in tests
    assert.Empty(t, emptyTimeline.Posts)
    assert.NotEmpty(t, mixedTimeline.Posts)
    assert.True(t, hasReplies(conversationTimeline.Posts))
}
```

### Timeline Scenario Types
```go
const (
    EmptyTimeline       TimelineType = "empty"        // No content
    SimpleTimeline      TimelineType = "simple"       // Basic posts
    MixedTimeline       TimelineType = "mixed"        // Posts, replies, boosts
    HighVolumeTimeline  TimelineType = "high_volume"  // Many posts for performance
    ConversationTimeline TimelineType = "conversation" // Threaded conversations
)
```

## Testing Commands and Automation

### Makefile Targets

#### Basic Testing
```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Enforce minimum coverage (70%)
make test-coverage-enforce

# Detailed coverage by package
make test-coverage-detail
```

#### Specialized Testing
```bash
# Run only unit tests (fast)
make test-unit

# Run integration tests
make test-integration

# Run benchmark tests
make test-benchmark

# Run with race detection
make test-race

# Test specific package
make test-package PKG=./pkg/auth
```

#### Load Testing
```bash
# Run k6 load tests
make test-load

# Test authentication endpoints
make k6-auth

# Test timeline performance
make k6-timeline

# Test posting performance
make k6-posting

# Test federation performance
make k6-federation
```

#### Python API Tests
```bash
# Run Python API tests
make test-api

# Run federation tests
make test-federation

# Run search tests
make test-search

# Run AI integration tests
make test-ai

# Run authentication tests
make test-auth
```

### Continuous Integration

#### GitHub Actions Integration
```yaml
# .github/workflows/test.yml
name: Test Suite

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      dynamodb:
        image: amazon/dynamodb-local:latest
        ports:
          - 8000:8000
    
    steps:
      - uses: actions/checkout@v2
      
      - name: Setup Go
        uses: actions/setup-go@v2
        with:
          go-version: 1.21
      
      - name: Setup Python
        uses: actions/setup-python@v2
        with:
          python-version: 3.9
      
      - name: Install dependencies
        run: |
          go mod download
          pip install -r requirements.txt
      
      - name: Run Go tests
        run: make test-coverage-enforce
      
      - name: Run integration tests
        run: make test-integration
        env:
          DYNAMODB_ENDPOINT: http://localhost:8000
      
      - name: Run API tests
        run: make test-api
        env:
          TEST_BASE_URL: http://localhost:8080
      
      - name: Upload coverage
        uses: codecov/codecov-action@v2
        with:
          file: coverage.out
```

## Mock Services and Test Doubles

### Enhanced Mock Storage
```go
type EnhancedMockStorage struct {
    *mocks.MockStorage
    latency    time.Duration
    errorRate  float64
    opCount    int64
    state      map[string]interface{}
    mu         sync.RWMutex
}

func (e *EnhancedMockStorage) SetLatencySimulation(latency time.Duration) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.latency = latency
}

func (e *EnhancedMockStorage) SetErrorRate(rate float64) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.errorRate = rate
}

func (e *EnhancedMockStorage) GetUser(ctx context.Context, userID string) (*models.User, error) {
    // Simulate latency
    if e.latency > 0 {
        time.Sleep(e.latency)
    }
    
    // Simulate errors
    if e.errorRate > 0 && rand.Float64() < e.errorRate {
        return nil, errors.New("simulated error")
    }
    
    // Track operations
    atomic.AddInt64(&e.opCount, 1)
    
    return e.MockStorage.GetUser(ctx, userID)
}
```

### Mock Federation Service
```go
type MockFederationService struct {
    deliveries      []DeliveryRecord
    responses       map[string]http.Response
    signatureValid  bool
    mu              sync.RWMutex
}

type DeliveryRecord struct {
    TargetInbox string
    Activity    activitypub.Activity
    Timestamp   time.Time
}

func (m *MockFederationService) DeliverActivity(ctx context.Context, activity *activitypub.Activity, targetInbox string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.deliveries = append(m.deliveries, DeliveryRecord{
        TargetInbox: targetInbox,
        Activity:    *activity,
        Timestamp:   time.Now(),
    })
    
    return nil
}

func (m *MockFederationService) GetDeliveries() []DeliveryRecord {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return append([]DeliveryRecord{}, m.deliveries...)
}
```

## Security Testing

### Authentication Testing
```go
func TestAuthenticationSecurity(t *testing.T) {
    testApp := setupSecureTestApp(t)
    
    t.Run("RejectsUnauthenticatedRequests", func(t *testing.T) {
        resp := testApp.POST("/api/v1/statuses", map[string]any{
            "status": "This should fail",
        })
        assert.Equal(t, 401, resp.StatusCode)
    })
    
    t.Run("RejectsExpiredTokens", func(t *testing.T) {
        expiredToken := generateExpiredJWT()
        resp := testApp.
            WithHeader("Authorization", "Bearer "+expiredToken).
            POST("/api/v1/statuses", map[string]any{
                "status": "This should fail",
            })
        assert.Equal(t, 401, resp.StatusCode)
    })
    
    t.Run("RespectsRateLimits", func(t *testing.T) {
        token := generateValidJWT()
        
        // Make requests up to rate limit
        for i := 0; i < 100; i++ {
            resp := testApp.
                WithHeader("Authorization", "Bearer "+token).
                POST("/api/v1/statuses", map[string]any{
                    "status": fmt.Sprintf("Post %d", i),
                })
            if i < 50 {
                assert.Equal(t, 201, resp.StatusCode)
            } else {
                assert.Equal(t, 429, resp.StatusCode)
            }
        }
    })
}
```

### Input Validation Testing
```go
func TestInputValidation(t *testing.T) {
    tests := []struct {
        name   string
        input  map[string]any
        expect int
    }{
        {"XSS attempt", map[string]any{"status": "<script>alert('xss')</script>"}, 400},
        {"SQL injection", map[string]any{"status": "'; DROP TABLE users; --"}, 400},
        {"Extremely long content", map[string]any{"status": strings.Repeat("a", 10000)}, 400},
        {"Valid content", map[string]any{"status": "Hello world!"}, 201},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resp := testApp.
                WithAuth(&lifttesting.AuthConfig{UserID: "user123"}).
                POST("/api/v1/statuses", tt.input)
            assert.Equal(t, tt.expect, resp.StatusCode)
        })
    }
}
```

## Cost Testing

### Resource Usage Validation
```go
func TestCostTracking(t *testing.T) {
    testApp := setupCostTrackingApp(t)
    
    t.Run("TracksDynamoDBCosts", func(t *testing.T) {
        resp := testApp.GET("/api/v1/timelines/home")
        assert.Equal(t, 200, resp.StatusCode)
        
        var response struct {
            Data interface{} `json:"data"`
            Cost struct {
                TotalCostMicros int64 `json:"total_cost_micros"`
                Breakdown       map[string]int64 `json:"breakdown"`
            } `json:"cost"`
        }
        
        err := json.Unmarshal(resp.Body, &response)
        require.NoError(t, err)
        
        assert.Greater(t, response.Cost.TotalCostMicros, int64(0))
        assert.Contains(t, response.Cost.Breakdown, "dynamodb_reads")
    })
    
    t.Run("StaysWithinBudget", func(t *testing.T) {
        budget := int64(1000) // $0.001 budget
        totalCost := int64(0)
        
        for i := 0; i < 100; i++ {
            resp := testApp.GET("/api/v1/timelines/home")
            
            var response struct {
                Cost struct {
                    TotalCostMicros int64 `json:"total_cost_micros"`
                } `json:"cost"`
            }
            
            err := json.Unmarshal(resp.Body, &response)
            require.NoError(t, err)
            
            totalCost += response.Cost.TotalCostMicros
        }
        
        assert.Less(t, totalCost, budget, "Total cost exceeded budget")
    })
}
```

## Test Environment Setup

### Environment Configuration
```bash
# Test environment variables
export TEST_ENV=true
export DYNAMODB_ENDPOINT=http://localhost:8000
export JWT_SECRET=test-jwt-secret-key
export DOMAIN_NAME=test.lesser.local
export S3_BUCKET_NAME=lesser-test-media
export CLOUDFRONT_DOMAIN=test-cdn.lesser.local

# Feature flags for testing
export ENABLE_FEDERATION=true
export ENABLE_SEARCH=true
export ENABLE_PUSH_NOTIFICATIONS=true
export ENABLE_COST_TRACKING=true

# Test data configuration
export TEST_INSTANCE_DOMAIN=test.example.com
export TEST_USER_COUNT=100
export TEST_STATUS_COUNT=1000
```

### Docker Compose for Testing
```yaml
# docker-compose.test.yml
version: '3.8'

services:
  dynamodb-local:
    image: amazon/dynamodb-local:latest
    ports:
      - "8000:8000"
    command: ["-jar", "DynamoDBLocal.jar", "-sharedDb"]
  
  localstack:
    image: localstack/localstack:latest
    ports:
      - "4566:4566"
    environment:
      - SERVICES=s3,sqs,sns
      - DEBUG=1
      - DATA_DIR=/tmp/localstack/data
    volumes:
      - "./tmp/localstack:/tmp/localstack"
      - "/var/run/docker.sock:/var/run/docker.sock"
  
  redis:
    image: redis:alpine
    ports:
      - "6379:6379"
```

## Testing Best Practices

### Test Organization
1. **Follow AAA Pattern** - Arrange, Act, Assert
2. **Use Descriptive Names** - Test names should describe the scenario
3. **Keep Tests Independent** - No shared state between tests
4. **Use Table-Driven Tests** - For testing multiple scenarios
5. **Mock External Dependencies** - Don't test external services

### Performance Testing Guidelines
1. **Establish Baselines** - Record performance benchmarks
2. **Test Realistic Scenarios** - Use production-like data volumes
3. **Monitor Memory** - Track memory allocations and leaks
4. **Test Concurrency** - Validate thread safety
5. **Set Appropriate Thresholds** - Define acceptable performance limits

### Coverage and Quality
1. **Aim for 70%+ Coverage** - Enforce minimum coverage thresholds
2. **Focus on Critical Paths** - Prioritize important functionality
3. **Test Error Conditions** - Include failure scenarios
4. **Validate Edge Cases** - Test boundary conditions
5. **Review Coverage Reports** - Identify testing gaps

### Continuous Integration
1. **Run Tests on Every PR** - Automated testing workflow
2. **Parallel Test Execution** - Faster feedback loops
3. **Fail Fast** - Stop on first failure in CI
4. **Cache Dependencies** - Speed up CI pipeline
5. **Report Test Results** - Clear pass/fail reporting

Lesser's comprehensive testing infrastructure ensures reliability, performance, and security while maintaining cost efficiency through intelligent test design and automated validation.