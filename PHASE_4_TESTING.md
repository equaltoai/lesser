# Phase 4: Testing & Quality Assurance - Detailed Implementation Checklist

## 4.1 Unit Testing

### 4.1.1 Lift Handler Test Utilities
**File:** `pkg/lift/testing/handler_test_utils.go`

- [ ] Create test context builder
  ```go
  package testing
  
  import (
      "bytes"
      "encoding/json"
      "net/http"
      "net/http/httptest"
      "github.com/pay-theory/lift/pkg/lift"
  )
  
  type TestContextBuilder struct {
      method  string
      path    string
      body    interface{}
      headers map[string]string
      params  map[string]string
      query   map[string]string
      auth    *AuthInfo
  }
  
  type AuthInfo struct {
      UserID string
      Scopes []string
      Claims map[string]interface{}
  }
  
  func NewTestContext() *TestContextBuilder {
      return &TestContextBuilder{
          method:  "GET",
          path:    "/",
          headers: make(map[string]string),
          params:  make(map[string]string),
          query:   make(map[string]string),
      }
  }
  
  func (b *TestContextBuilder) WithMethod(method string) *TestContextBuilder {
      b.method = method
      return b
  }
  
  func (b *TestContextBuilder) WithPath(path string) *TestContextBuilder {
      b.path = path
      return b
  }
  
  func (b *TestContextBuilder) WithJSON(body interface{}) *TestContextBuilder {
      b.body = body
      b.headers["Content-Type"] = "application/json"
      return b
  }
  
  func (b *TestContextBuilder) WithAuth(userID string, scopes ...string) *TestContextBuilder {
      b.auth = &AuthInfo{
          UserID: userID,
          Scopes: scopes,
          Claims: map[string]interface{}{
              "sub": userID,
          },
      }
      return b
  }
  
  func (b *TestContextBuilder) Build() (*lift.Context, *httptest.ResponseRecorder) {
      var bodyReader *bytes.Reader
      if b.body != nil {
          bodyBytes, _ := json.Marshal(b.body)
          bodyReader = bytes.NewReader(bodyBytes)
      } else {
          bodyReader = bytes.NewReader([]byte{})
      }
      
      req := httptest.NewRequest(b.method, b.path, bodyReader)
      
      // Set headers
      for k, v := range b.headers {
          req.Header.Set(k, v)
      }
      
      // Create response recorder
      rec := httptest.NewRecorder()
      
      // Create Lift context
      ctx := lift.NewContext(req, rec)
      
      // Set params
      for k, v := range b.params {
          ctx.SetParam(k, v)
      }
      
      // Set auth info
      if b.auth != nil {
          ctx.Set("userID", b.auth.UserID)
          ctx.Set("scopes", b.auth.Scopes)
          ctx.Set("claims", b.auth.Claims)
      }
      
      return ctx, rec
  }
  ```

- [ ] Create handler test suite
  ```go
  type HandlerTestSuite struct {
      suite.Suite
      app      *lift.App
      store    *MockStorage
      handler  interface{}
  }
  
  func (s *HandlerTestSuite) SetupTest() {
      s.store = NewMockStorage()
      s.app = lift.New()
      
      // Add common middleware
      s.app.Use(middleware.ErrorHandlerMiddleware())
      s.app.Use(middleware.LoggingMiddleware(zap.NewNop()))
  }
  
  func (s *HandlerTestSuite) TestHandler(handler lift.HandlerFunc, tc TestCase) {
      ctx, rec := tc.Context.Build()
      
      err := handler(ctx)
      
      if tc.ExpectError {
          s.Error(err)
          if tc.ExpectedErrorCode != 0 {
              liftErr, ok := err.(*lift.Error)
              s.True(ok, "expected lift.Error")
              s.Equal(tc.ExpectedErrorCode, liftErr.StatusCode)
          }
      } else {
          s.NoError(err)
          s.Equal(tc.ExpectedStatus, rec.Code)
          
          if tc.ValidateResponse != nil {
              tc.ValidateResponse(s.T(), rec.Body.Bytes())
          }
      }
  }
  
  type TestCase struct {
      Name              string
      Context           *TestContextBuilder
      Setup             func(*MockStorage)
      ExpectError       bool
      ExpectedErrorCode int
      ExpectedStatus    int
      ValidateResponse  func(*testing.T, []byte)
  }
  ```

- [ ] Create event handler test utilities
  ```go
  func NewDynamoDBStreamEvent(records ...DynamoDBRecord) events.DynamoDBEvent {
      var eventRecords []events.DynamoDBEventRecord
      
      for _, record := range records {
          eventRecord := events.DynamoDBEventRecord{
              EventID:   uuid.New().String(),
              EventName: record.EventName,
              Change: events.DynamoDBStreamRecord{
                  NewImage: record.NewImage,
                  OldImage: record.OldImage,
                  Keys:     record.Keys,
              },
          }
          eventRecords = append(eventRecords, eventRecord)
      }
      
      return events.DynamoDBEvent{
          Records: eventRecords,
      }
  }
  
  func NewSQSEvent(messages ...SQSMessage) events.SQSEvent {
      var records []events.SQSMessage
      
      for _, msg := range messages {
          record := events.SQSMessage{
              MessageId: uuid.New().String(),
              Body:      msg.Body,
              Attributes: map[string]string{
                  "ApproximateReceiveCount": "1",
              },
          }
          records = append(records, record)
      }
      
      return events.SQSEvent{
          Records: records,
      }
  }
  ```

**Testing Requirements:**
- [ ] Test context builder functionality
- [ ] Test auth context setting
- [ ] Test event creation helpers
- [ ] Test error assertions

**Acceptance Criteria:**
- Easy to create test contexts
- Supports all handler types
- Clear test assertions
- Minimal boilerplate

### 4.1.2 Repository Test Suite
**File:** `pkg/storage/dynamorm/repositories/testing/repository_suite.go`

- [ ] Create base repository test suite
  ```go
  package testing
  
  type RepositoryTestSuite struct {
      suite.Suite
      client     *dynamorm.Client
      cleaner    *DataCleaner
      repository interface{}
  }
  
  func (s *RepositoryTestSuite) SetupSuite() {
      // Use test table
      tableName := os.Getenv("TEST_DYNAMODB_TABLE")
      if tableName == "" {
          s.T().Fatal("TEST_DYNAMODB_TABLE not set")
      }
      
      s.client = dynamorm.NewClient(dynamorm.Config{
          Table:  tableName,
          Region: "us-east-1",
      })
      
      s.cleaner = NewDataCleaner(s.client)
  }
  
  func (s *RepositoryTestSuite) SetupTest() {
      // Clear any existing test data
      s.cleaner.Reset()
  }
  
  func (s *RepositoryTestSuite) TearDownTest() {
      // Clean up test data
      if err := s.cleaner.Clean(); err != nil {
          s.T().Errorf("failed to clean test data: %v", err)
      }
  }
  
  func (s *RepositoryTestSuite) Track(items ...interface{}) {
      for _, item := range items {
          s.cleaner.Track(item)
      }
  }
  ```

- [ ] Create repository test cases
  ```go
  type RepositoryTest struct {
      Name     string
      Setup    func() interface{}      // Create test data
      Execute  func(interface{}) error // Run repository method
      Validate func(error)              // Validate result
  }
  
  func (s *RepositoryTestSuite) RunRepositoryTests(tests []RepositoryTest) {
      for _, test := range tests {
          s.Run(test.Name, func() {
              // Setup
              data := test.Setup()
              if data != nil {
                  s.Track(data)
              }
              
              // Execute
              err := test.Execute(data)
              
              // Validate
              test.Validate(err)
          })
      }
  }
  
  // Example test implementation
  func (s *UserRepositoryTestSuite) TestGetByID() {
      tests := []RepositoryTest{
          {
              Name: "existing user",
              Setup: func() interface{} {
                  user := &User{
                      ID:       "test-user-1",
                      Username: "testuser",
                      Email:    "test@example.com",
                  }
                  s.NoError(s.repository.Save(context.Background(), user))
                  return user
              },
              Execute: func(data interface{}) error {
                  user := data.(*User)
                  found, err := s.repository.GetByID(context.Background(), user.ID)
                  s.NoError(err)
                  s.Equal(user.Username, found.Username)
                  return nil
              },
              Validate: func(err error) {
                  s.NoError(err)
              },
          },
          {
              Name: "non-existent user",
              Setup: func() interface{} {
                  return nil
              },
              Execute: func(data interface{}) error {
                  _, err := s.repository.GetByID(context.Background(), "non-existent")
                  return err
              },
              Validate: func(err error) {
                  s.Error(err)
                  s.True(errors.Is(err, ErrNotFound))
              },
          },
      }
      
      s.RunRepositoryTests(tests)
  }
  ```

- [ ] Create transaction test helpers
  ```go
  func (s *RepositoryTestSuite) TestTransaction(t *testing.T, fn func() error) {
      // Start transaction
      tx := s.client.NewTransaction()
      
      // Execute function
      err := fn()
      
      if err != nil {
          // Verify rollback
          s.verifyNoChanges()
      } else {
          // Verify commit
          s.verifyChangesApplied()
      }
  }
  
  func (s *RepositoryTestSuite) TestConcurrentOperations(t *testing.T, operations []func()) {
      var wg sync.WaitGroup
      errors := make(chan error, len(operations))
      
      for _, op := range operations {
          wg.Add(1)
          go func(operation func()) {
              defer wg.Done()
              defer func() {
                  if r := recover(); r != nil {
                      errors <- fmt.Errorf("panic: %v", r)
                  }
              }()
              operation()
          }(op)
      }
      
      wg.Wait()
      close(errors)
      
      // Check for errors
      for err := range errors {
          s.NoError(err)
      }
  }
  ```

**Testing Requirements:**
- [ ] Test CRUD operations
- [ ] Test query operations
- [ ] Test transactions
- [ ] Test concurrent access
- [ ] Test error scenarios

**Acceptance Criteria:**
- All repository methods tested
- Data isolation between tests
- Concurrent operations safe
- Performance benchmarks included

### 4.1.3 Middleware Testing
**File:** `pkg/lift/middleware/middleware_test.go`

- [ ] Create middleware test framework
  ```go
  type MiddlewareTestCase struct {
      Name           string
      Middleware     lift.Middleware
      Context        *TestContextBuilder
      NextError      error
      ExpectedError  error
      ValidateContext func(*testing.T, *lift.Context)
  }
  
  func TestMiddleware(t *testing.T, tc MiddlewareTestCase) {
      ctx, _ := tc.Context.Build()
      
      called := false
      next := func(ctx *lift.Context) error {
          called = true
          return tc.NextError
      }
      
      handler := tc.Middleware(next)
      err := handler(ctx)
      
      if tc.ExpectedError != nil {
          assert.Error(t, err)
          assert.Equal(t, tc.ExpectedError, err)
      } else {
          assert.NoError(t, err)
      }
      
      if tc.NextError == nil {
          assert.True(t, called, "next handler should be called")
      }
      
      if tc.ValidateContext != nil {
          tc.ValidateContext(t, ctx)
      }
  }
  ```

- [ ] Test auth middleware
  ```go
  func TestAuthMiddleware(t *testing.T) {
      validator := &MockTokenValidator{}
      middleware := NewAuthMiddleware(AuthConfig{
          TokenValidator: validator,
          RequireAuth:    true,
      })
      
      tests := []MiddlewareTestCase{
          {
              Name:       "valid token",
              Middleware: middleware,
              Context: NewTestContext().
                  WithHeader("Authorization", "Bearer valid-token"),
              ValidateContext: func(t *testing.T, ctx *lift.Context) {
                  userID, _ := ctx.Get("auth:userID").(string)
                  assert.Equal(t, "user-123", userID)
              },
          },
          {
              Name:       "missing token",
              Middleware: middleware,
              Context:    NewTestContext(),
              ExpectedError: lift.NewError(401, "authentication required"),
          },
          {
              Name:       "invalid token",
              Middleware: middleware,
              Context: NewTestContext().
                  WithHeader("Authorization", "Bearer invalid-token"),
              ExpectedError: lift.NewError(401, "invalid token"),
          },
      }
      
      for _, tc := range tests {
          t.Run(tc.Name, func(t *testing.T) {
              TestMiddleware(t, tc)
          })
      }
  }
  ```

- [ ] Test rate limiting middleware
  ```go
  func TestRateLimitMiddleware(t *testing.T) {
      limiter := NewRateLimiter(2, time.Second) // 2 requests per second
      middleware := RateLimitMiddleware(limiter)
      
      // First two requests should succeed
      for i := 0; i < 2; i++ {
          tc := MiddlewareTestCase{
              Name:       fmt.Sprintf("request %d", i+1),
              Middleware: middleware,
              Context:    NewTestContext().WithAuth("user-123"),
          }
          TestMiddleware(t, tc)
      }
      
      // Third request should be rate limited
      tc := MiddlewareTestCase{
          Name:       "rate limited",
          Middleware: middleware,
          Context:    NewTestContext().WithAuth("user-123"),
          ExpectedError: lift.NewError(429, "rate limit exceeded"),
      }
      TestMiddleware(t, tc)
  }
  ```

**Testing Requirements:**
- [ ] Test middleware ordering
- [ ] Test error propagation
- [ ] Test context modification
- [ ] Test performance impact

**Acceptance Criteria:**
- All middleware fully tested
- Edge cases covered
- Performance benchmarked
- Integration tested

### 4.1.4 Mock Creation Utilities
**File:** `pkg/testing/mocks/generator.go`

- [ ] Create mock generator
  ```go
  package mocks
  
  //go:generate mockgen -destination=storage_mock.go -package=mocks github.com/aron23/lesser/pkg/storage Storage
  //go:generate mockgen -destination=auth_mock.go -package=mocks github.com/aron23/lesser/pkg/auth Service
  
  type MockBuilder struct {
      storage *MockStorage
      auth    *MockAuthService
  }
  
  func NewMockBuilder() *MockBuilder {
      return &MockBuilder{
          storage: NewMockStorage(gomock.NewController(nil)),
          auth:    NewMockAuthService(gomock.NewController(nil)),
      }
  }
  
  func (mb *MockBuilder) WithUser(user *User) *MockBuilder {
      mb.storage.EXPECT().
          GetUser(gomock.Any(), user.ID).
          Return(user, nil).
          AnyTimes()
      return mb
  }
  
  func (mb *MockBuilder) WithAuthToken(token string, claims *Claims) *MockBuilder {
      mb.auth.EXPECT().
          ValidateToken(token).
          Return(claims, nil).
          AnyTimes()
      return mb
  }
  ```

- [ ] Create behavior presets
  ```go
  type MockPreset string
  
  const (
      PresetAuthenticatedUser MockPreset = "authenticated_user"
      PresetAdminUser        MockPreset = "admin_user"
      PresetRateLimited      MockPreset = "rate_limited"
  )
  
  func (mb *MockBuilder) WithPreset(preset MockPreset) *MockBuilder {
      switch preset {
      case PresetAuthenticatedUser:
          user := &User{ID: "user-123", Username: "testuser"}
          claims := &Claims{UserID: "user-123", Scopes: []string{"read", "write"}}
          mb.WithUser(user).WithAuthToken("test-token", claims)
          
      case PresetAdminUser:
          user := &User{ID: "admin-123", Username: "admin", IsAdmin: true}
          claims := &Claims{UserID: "admin-123", Scopes: []string{"admin"}}
          mb.WithUser(user).WithAuthToken("admin-token", claims)
          
      case PresetRateLimited:
          mb.storage.EXPECT().
              CheckRateLimit(gomock.Any(), gomock.Any()).
              Return(errors.New("rate limit exceeded")).
              AnyTimes()
      }
      
      return mb
  }
  ```

**Testing Requirements:**
- [ ] Test mock generation
- [ ] Test preset behaviors
- [ ] Test mock expectations
- [ ] Test error scenarios

**Acceptance Criteria:**
- Easy mock creation
- Reusable presets
- Type-safe mocks
- Clear error messages

## 4.2 Integration Testing

### 4.2.1 End-to-End Lambda Tests
**File:** `tests/integration/lambda_e2e_test.go`

- [ ] Create Lambda test harness
  ```go
  package integration
  
  type LambdaTestHarness struct {
      lambdaClient *lambda.Client
      functions    map[string]string // name -> ARN
      testData     *TestDataManager
  }
  
  func NewLambdaTestHarness() (*LambdaTestHarness, error) {
      cfg, err := config.LoadDefaultConfig(context.Background())
      if err != nil {
          return nil, err
      }
      
      return &LambdaTestHarness{
          lambdaClient: lambda.NewFromConfig(cfg),
          functions:    loadFunctionARNs(),
          testData:     NewTestDataManager(),
      }, nil
  }
  
  func (h *LambdaTestHarness) InvokeFunction(name string, payload interface{}) (*lambda.InvokeOutput, error) {
      payloadBytes, err := json.Marshal(payload)
      if err != nil {
          return nil, err
      }
      
      return h.lambdaClient.Invoke(context.Background(), &lambda.InvokeInput{
          FunctionName: aws.String(h.functions[name]),
          Payload:      payloadBytes,
      })
  }
  
  func (h *LambdaTestHarness) InvokeHTTPFunction(name string, request APIGatewayRequest) (*APIGatewayResponse, error) {
      result, err := h.InvokeFunction(name, request)
      if err != nil {
          return nil, err
      }
      
      var response APIGatewayResponse
      if err := json.Unmarshal(result.Payload, &response); err != nil {
          return nil, err
      }
      
      return &response, nil
  }
  ```

- [ ] Create end-to-end test scenarios
  ```go
  func TestCreateStatusFlow(t *testing.T) {
      harness, err := NewLambdaTestHarness()
      require.NoError(t, err)
      
      // Step 1: Create user
      user := harness.testData.CreateUser(t)
      
      // Step 2: Authenticate
      token := harness.testData.AuthenticateUser(t, user)
      
      // Step 3: Create status via API Lambda
      createReq := APIGatewayRequest{
          HTTPMethod: "POST",
          Path:       "/api/v1/statuses",
          Headers: map[string]string{
              "Authorization": "Bearer " + token,
          },
          Body: `{"status": "Hello from integration test!"}`,
      }
      
      createResp, err := harness.InvokeHTTPFunction("api", createReq)
      require.NoError(t, err)
      assert.Equal(t, 201, createResp.StatusCode)
      
      var status Status
      err = json.Unmarshal([]byte(createResp.Body), &status)
      require.NoError(t, err)
      
      // Step 4: Verify status appears in timeline
      timelineReq := APIGatewayRequest{
          HTTPMethod: "GET",
          Path:       "/api/v1/timelines/home",
          Headers: map[string]string{
              "Authorization": "Bearer " + token,
          },
      }
      
      // Wait for eventual consistency
      require.Eventually(t, func() bool {
          timelineResp, err := harness.InvokeHTTPFunction("api", timelineReq)
          if err != nil || timelineResp.StatusCode != 200 {
              return false
          }
          
          var timeline []Status
          json.Unmarshal([]byte(timelineResp.Body), &timeline)
          
          for _, s := range timeline {
              if s.ID == status.ID {
                  return true
              }
          }
          return false
      }, 10*time.Second, 500*time.Millisecond)
      
      // Step 5: Verify activity processor created timeline entries
      harness.testData.VerifyTimelineEntries(t, user.ID, status.ID)
  }
  ```

- [ ] Test federation flow
  ```go
  func TestFederationFlow(t *testing.T) {
      harness, err := NewLambdaTestHarness()
      require.NoError(t, err)
      
      // Step 1: Simulate incoming follow activity
      activity := &ActivityPubActivity{
          Type:  "Follow",
          Actor: "https://remote.server/users/alice",
          Object: "https://our.server/users/bob",
      }
      
      inboxReq := APIGatewayRequest{
          HTTPMethod: "POST",
          Path:       "/inbox",
          Headers: map[string]string{
              "Signature": generateSignature(activity),
          },
          Body: marshalActivity(activity),
      }
      
      inboxResp, err := harness.InvokeHTTPFunction("inbox", inboxReq)
      require.NoError(t, err)
      assert.Equal(t, 202, inboxResp.StatusCode)
      
      // Step 2: Verify follow was processed
      require.Eventually(t, func() bool {
          return harness.testData.FollowExists("https://remote.server/users/alice", "bob")
      }, 10*time.Second, 500*time.Millisecond)
      
      // Step 3: Verify Accept activity was sent
      harness.testData.VerifyActivitySent(t, "Accept", "https://remote.server/inbox")
  }
  ```

**Testing Requirements:**
- [ ] Test complete user flows
- [ ] Test federation scenarios
- [ ] Test error recovery
- [ ] Test performance under load

**Acceptance Criteria:**
- All major flows tested
- Federation verified
- Error handling confirmed
- Performance acceptable

### 4.2.2 DynamoDB Stream Testing
**File:** `tests/integration/stream_processing_test.go`

- [ ] Create stream test utilities
  ```go
  type StreamTestUtils struct {
      dynamoClient *dynamodb.Client
      tableName    string
  }
  
  func (u *StreamTestUtils) TriggerStreamEvent(t *testing.T, operation string, item interface{}) {
      switch operation {
      case "INSERT":
          _, err := u.dynamoClient.PutItem(context.Background(), &dynamodb.PutItemInput{
              TableName: &u.tableName,
              Item:      marshalItem(item),
          })
          require.NoError(t, err)
          
      case "UPDATE":
          _, err := u.dynamoClient.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
              TableName: &u.tableName,
              Key:       getKey(item),
              UpdateExpression: aws.String("SET #attr = :val"),
              // ... update logic
          })
          require.NoError(t, err)
          
      case "DELETE":
          _, err := u.dynamoClient.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
              TableName: &u.tableName,
              Key:       getKey(item),
          })
          require.NoError(t, err)
      }
  }
  
  func (u *StreamTestUtils) VerifyProcessorHandled(t *testing.T, processorName string, eventID string) {
      // Check CloudWatch logs or metrics
      require.Eventually(t, func() bool {
          return u.checkProcessorLogs(processorName, eventID)
      }, 30*time.Second, 1*time.Second)
  }
  ```

- [ ] Test stream processor resilience
  ```go
  func TestStreamProcessorResilience(t *testing.T) {
      utils := &StreamTestUtils{
          dynamoClient: getDynamoClient(),
          tableName:    getTestTableName(),
      }
      
      // Test 1: Large batch processing
      t.Run("large batch", func(t *testing.T) {
          items := make([]interface{}, 100)
          for i := 0; i < 100; i++ {
              items[i] = &Status{
                  ID:      fmt.Sprintf("status-%d", i),
                  Content: "Test status",
              }
          }
          
          // Trigger all at once
          for _, item := range items {
              utils.TriggerStreamEvent(t, "INSERT", item)
          }
          
          // Verify all processed
          for _, item := range items {
              status := item.(*Status)
              utils.VerifyProcessorHandled(t, "activity-processor", status.ID)
          }
      })
      
      // Test 2: Error recovery
      t.Run("error recovery", func(t *testing.T) {
          // Insert malformed data
          malformed := map[string]interface{}{
              "pk": "INVALID",
              "sk": "INVALID",
              "bad_field": struct{}{}, // This should cause processing error
          }
          
          utils.TriggerStreamEvent(t, "INSERT", malformed)
          
          // Verify error was logged but processor continued
          utils.VerifyProcessorRecovered(t, "activity-processor")
      })
  }
  ```

**Testing Requirements:**
- [ ] Test stream event delivery
- [ ] Test batch processing
- [ ] Test error scenarios
- [ ] Test eventual consistency

**Acceptance Criteria:**
- Stream events processed
- Errors don't stop processing
- Performance under load
- Data consistency maintained

### 4.2.3 Performance Benchmarks
**File:** `tests/benchmarks/performance_test.go`

- [ ] Create benchmark suite
  ```go
  type BenchmarkSuite struct {
      harness *LambdaTestHarness
      metrics *MetricsCollector
  }
  
  func BenchmarkAPIEndpoints(b *testing.B) {
      suite := setupBenchmarkSuite(b)
      
      b.Run("GetTimeline", func(b *testing.B) {
          token := suite.getAuthToken()
          
          b.ResetTimer()
          for i := 0; i < b.N; i++ {
              start := time.Now()
              
              resp, err := suite.harness.InvokeHTTPFunction("api", APIGatewayRequest{
                  HTTPMethod: "GET",
                  Path:       "/api/v1/timelines/home",
                  Headers: map[string]string{
                      "Authorization": "Bearer " + token,
                  },
              })
              
              duration := time.Since(start)
              
              require.NoError(b, err)
              require.Equal(b, 200, resp.StatusCode)
              
              suite.metrics.RecordLatency("GetTimeline", duration)
          }
          
          b.ReportMetric(suite.metrics.P95("GetTimeline").Seconds(), "p95_sec")
          b.ReportMetric(suite.metrics.P99("GetTimeline").Seconds(), "p99_sec")
      })
  }
  
  func BenchmarkConcurrentRequests(b *testing.B) {
      suite := setupBenchmarkSuite(b)
      concurrency := 10
      
      b.Run("ConcurrentPosts", func(b *testing.B) {
          tokens := suite.getMultipleAuthTokens(concurrency)
          
          b.ResetTimer()
          
          var wg sync.WaitGroup
          for i := 0; i < b.N; i++ {
              for j := 0; j < concurrency; j++ {
                  wg.Add(1)
                  go func(token string, iter int) {
                      defer wg.Done()
                      
                      _, err := suite.harness.InvokeHTTPFunction("api", APIGatewayRequest{
                          HTTPMethod: "POST",
                          Path:       "/api/v1/statuses",
                          Headers: map[string]string{
                              "Authorization": "Bearer " + token,
                          },
                          Body: fmt.Sprintf(`{"status": "Concurrent test %d"}`, iter),
                      })
                      
                      if err != nil {
                          b.Error(err)
                      }
                  }(tokens[j], i)
              }
          }
          
          wg.Wait()
      })
  }
  ```

- [ ] Create cold start benchmarks
  ```go
  func BenchmarkColdStarts(b *testing.B) {
      suite := setupBenchmarkSuite(b)
      
      functions := []string{"api", "inbox", "activity-processor"}
      
      for _, fn := range functions {
          b.Run(fn, func(b *testing.B) {
              for i := 0; i < b.N; i++ {
                  // Force cold start by updating environment variable
                  suite.forceColdStart(fn)
                  
                  start := time.Now()
                  
                  _, err := suite.harness.InvokeFunction(fn, getTestPayload(fn))
                  
                  coldStartTime := time.Since(start)
                  
                  require.NoError(b, err)
                  
                  suite.metrics.RecordColdStart(fn, coldStartTime)
                  
                  // Wait before next iteration
                  time.Sleep(5 * time.Second)
              }
              
              b.ReportMetric(suite.metrics.AvgColdStart(fn).Seconds(), "avg_cold_start_sec")
          })
      }
  }
  ```

**Testing Requirements:**
- [ ] Benchmark all endpoints
- [ ] Test concurrent load
- [ ] Measure cold starts
- [ ] Track memory usage

**Acceptance Criteria:**
- P95 latency < 100ms
- P99 latency < 500ms
- Cold starts < 1s
- Memory usage stable

### 4.2.4 Cost Analysis Tests
**File:** `tests/integration/cost_analysis_test.go`

- [ ] Create cost tracking tests
  ```go
  type CostAnalyzer struct {
      dynamoClient   *dynamodb.Client
      cloudwatchClient *cloudwatch.Client
      startTime      time.Time
      endTime        time.Time
  }
  
  func TestOperationCosts(t *testing.T) {
      analyzer := NewCostAnalyzer()
      
      testCases := []struct {
          name      string
          operation func() error
          maxCost   float64
      }{
          {
              name: "Create Status",
              operation: func() error {
                  return createStatus("Test status")
              },
              maxCost: 0.001, // $0.001
          },
          {
              name: "Get Timeline (100 items)",
              operation: func() error {
                  return getTimeline(100)
              },
              maxCost: 0.002, // $0.002
          },
          {
              name: "Federation Delivery",
              operation: func() error {
                  return deliverActivity(10) // 10 recipients
              },
              maxCost: 0.005, // $0.005
          },
      }
      
      for _, tc := range testCases {
          t.Run(tc.name, func(t *testing.T) {
              analyzer.StartTracking()
              
              err := tc.operation()
              require.NoError(t, err)
              
              cost := analyzer.CalculateCost()
              assert.LessOrEqual(t, cost, tc.maxCost,
                  "Operation %s exceeded cost limit: got $%.4f, want <= $%.4f",
                  tc.name, cost, tc.maxCost)
          })
      }
  }
  
  func (a *CostAnalyzer) CalculateCost() float64 {
      // Get DynamoDB consumed capacity
      dynamoCost := a.getDynamoDBCost()
      
      // Get Lambda invocation cost
      lambdaCost := a.getLambdaCost()
      
      // Get data transfer cost
      transferCost := a.getDataTransferCost()
      
      return dynamoCost + lambdaCost + transferCost
  }
  ```

- [ ] Test cost optimization
  ```go
  func TestCostOptimization(t *testing.T) {
      // Test batch operations vs individual
      t.Run("batch vs individual", func(t *testing.T) {
          analyzer := NewCostAnalyzer()
          
          // Individual operations
          analyzer.StartTracking()
          for i := 0; i < 25; i++ {
              createTimelineEntry(fmt.Sprintf("user-%d", i), "status-123")
          }
          individualCost := analyzer.CalculateCost()
          
          // Batch operation
          analyzer.StartTracking()
          users := make([]string, 25)
          for i := 0; i < 25; i++ {
              users[i] = fmt.Sprintf("user-%d", i)
          }
          batchCreateTimelineEntries(users, "status-123")
          batchCost := analyzer.CalculateCost()
          
          // Batch should be significantly cheaper
          assert.Less(t, batchCost, individualCost*0.5,
              "Batch operation should be at least 50%% cheaper")
      })
  }
  ```

**Testing Requirements:**
- [ ] Track operation costs
- [ ] Verify cost limits
- [ ] Test optimizations
- [ ] Generate cost reports

**Acceptance Criteria:**
- Costs within budget
- Optimizations effective
- No cost surprises
- Reports actionable

## 4.3 Load Testing Updates

### 4.3.1 Update k6 Scripts
**File:** `tests/k6/scenarios/lift_endpoints.js`

- [ ] Create Lift-aware load tests
  ```javascript
  import http from 'k6/http';
  import { check, sleep } from 'k6';
  import { Rate } from 'k6/metrics';
  
  const errorRate = new Rate('errors');
  
  export const options = {
      stages: [
          { duration: '2m', target: 100 },  // Ramp up
          { duration: '5m', target: 100 },  // Stay at 100 users
          { duration: '2m', target: 200 },  // Ramp to 200
          { duration: '5m', target: 200 },  // Stay at 200
          { duration: '2m', target: 0 },    // Ramp down
      ],
      thresholds: {
          http_req_duration: ['p(95)<500', 'p(99)<1000'],
          errors: ['rate<0.01'],
      },
  };
  
  export function setup() {
      // Create test users and get auth tokens
      const tokens = [];
      for (let i = 0; i < 100; i++) {
          const user = createTestUser(i);
          const token = authenticateUser(user);
          tokens.push(token);
      }
      return { tokens };
  }
  
  export default function(data) {
      const token = data.tokens[Math.floor(Math.random() * data.tokens.length)];
      
      // Test timeline endpoint
      const timelineRes = http.get(`${__ENV.API_URL}/api/v1/timelines/home`, {
          headers: {
              'Authorization': `Bearer ${token}`,
          },
      });
      
      check(timelineRes, {
          'timeline status is 200': (r) => r.status === 200,
          'timeline has content': (r) => r.body.length > 0,
          'timeline response time OK': (r) => r.timings.duration < 500,
      });
      
      errorRate.add(timelineRes.status !== 200);
      
      sleep(1);
      
      // Test status creation
      if (Math.random() < 0.1) { // 10% of requests create status
          const statusRes = http.post(
              `${__ENV.API_URL}/api/v1/statuses`,
              JSON.stringify({
                  status: `Load test status ${Date.now()}`,
              }),
              {
                  headers: {
                      'Authorization': `Bearer ${token}`,
                      'Content-Type': 'application/json',
                  },
              }
          );
          
          check(statusRes, {
              'status creation is 201': (r) => r.status === 201,
              'status has ID': (r) => JSON.parse(r.body).id !== undefined,
          });
          
          errorRate.add(statusRes.status !== 201);
      }
  }
  ```

- [ ] Create WebSocket load tests
  ```javascript
  import ws from 'k6/ws';
  import { check } from 'k6';
  
  export default function(data) {
      const token = data.tokens[Math.floor(Math.random() * data.tokens.length)];
      const url = `${__ENV.WS_URL}/api/v1/streaming?access_token=${token}`;
      
      const res = ws.connect(url, {}, function(socket) {
          socket.on('open', () => {
              console.log('WebSocket connected');
              
              // Subscribe to timeline
              socket.send(JSON.stringify({
                  type: 'subscribe',
                  stream: 'user',
              }));
          });
          
          socket.on('message', (data) => {
              const message = JSON.parse(data);
              check(message, {
                  'message has type': (m) => m.type !== undefined,
                  'message has payload': (m) => m.payload !== undefined,
              });
          });
          
          socket.on('error', (e) => {
              console.error('WebSocket error:', e);
          });
          
          // Keep connection open for 30 seconds
          socket.setTimeout(() => {
              socket.close();
          }, 30000);
      });
      
      check(res, {
          'WebSocket connection successful': (r) => r && r.status === 101,
      });
  }
  ```

**Testing Requirements:**
- [ ] Test all endpoints
- [ ] Test WebSocket connections
- [ ] Test federation endpoints
- [ ] Test error scenarios

**Acceptance Criteria:**
- Handle target load
- Error rate < 1%
- P95 latency acceptable
- No memory leaks

### 4.3.2 Lambda Concurrency Tests
**File:** `tests/k6/scenarios/concurrency.js`

- [ ] Test concurrent Lambda execution
  ```javascript
  export const options = {
      scenarios: {
          burst_load: {
              executor: 'shared-iterations',
              vus: 500,
              iterations: 500,
              maxDuration: '30s',
          },
          sustained_load: {
              executor: 'constant-vus',
              vus: 100,
              duration: '5m',
          },
      },
  };
  
  export default function() {
      // Test concurrent status creation
      const batch = [];
      for (let i = 0; i < 10; i++) {
          batch.push(
              http.post(
                  `${__ENV.API_URL}/api/v1/statuses`,
                  JSON.stringify({
                      status: `Concurrent test ${Date.now()}-${i}`,
                  }),
                  {
                      headers: {
                          'Authorization': `Bearer ${getToken()}`,
                          'Content-Type': 'application/json',
                      },
                  }
              )
          );
      }
      
      // Check all responses
      batch.forEach((res, i) => {
          check(res, {
              [`request ${i} successful`]: (r) => r.status === 201,
          });
      });
  }
  ```

**Testing Requirements:**
- [ ] Test burst traffic
- [ ] Test sustained load
- [ ] Test Lambda limits
- [ ] Test throttling behavior

**Acceptance Criteria:**
- Handle burst traffic
- Graceful degradation
- No data corruption
- Clear error messages

## 4.4 Testing Infrastructure

### 4.4.1 Test Environment Setup
**File:** `tests/setup/environment.go`

- [ ] Create test environment manager
  ```go
  type TestEnvironment struct {
      config    *TestConfig
      resources *TestResources
      cleaner   *ResourceCleaner
  }
  
  func NewTestEnvironment() (*TestEnvironment, error) {
      config := loadTestConfig()
      
      env := &TestEnvironment{
          config:    config,
          resources: &TestResources{},
          cleaner:   NewResourceCleaner(),
      }
      
      if err := env.setup(); err != nil {
          return nil, err
      }
      
      return env, nil
  }
  
  func (e *TestEnvironment) setup() error {
      // Create test DynamoDB table
      if err := e.createTestTable(); err != nil {
          return err
      }
      
      // Deploy test Lambdas
      if err := e.deployTestLambdas(); err != nil {
          return err
      }
      
      // Setup test data
      if err := e.seedTestData(); err != nil {
          return err
      }
      
      return nil
  }
  
  func (e *TestEnvironment) Cleanup() error {
      return e.cleaner.CleanAll()
  }
  ```

- [ ] Create CI/CD test pipeline
  ```yaml
  # .github/workflows/test.yml
  name: Test Suite
  
  on:
    push:
      branches: [ main, develop ]
    pull_request:
      branches: [ main ]
  
  jobs:
    unit-tests:
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v3
        
        - name: Set up Go
          uses: actions/setup-go@v4
          with:
            go-version: '1.21'
        
        - name: Run unit tests
          run: |
            go test -v -race -coverprofile=coverage.out ./...
            go tool cover -html=coverage.out -o coverage.html
        
        - name: Upload coverage
          uses: codecov/codecov-action@v3
          with:
            file: ./coverage.out
    
    integration-tests:
      runs-on: ubuntu-latest
      needs: unit-tests
      steps:
        - uses: actions/checkout@v3
        
        - name: Setup test environment
          run: make setup-test-env
        
        - name: Run integration tests
          run: make test-integration
        
        - name: Cleanup
          if: always()
          run: make cleanup-test-env
    
    load-tests:
      runs-on: ubuntu-latest
      needs: integration-tests
      steps:
        - uses: actions/checkout@v3
        
        - name: Install k6
          run: |
            sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
            echo "deb https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
            sudo apt-get update
            sudo apt-get install k6
        
        - name: Run load tests
          run: make test-load
        
        - name: Upload results
          uses: actions/upload-artifact@v3
          with:
            name: load-test-results
            path: tests/results/
  ```

**Testing Requirements:**
- [ ] Isolated test environment
- [ ] Automated cleanup
- [ ] CI/CD integration
- [ ] Result reporting

**Acceptance Criteria:**
- Tests run in isolation
- No test data leakage
- Fast feedback cycle
- Clear failure reports

## Success Metrics
- [ ] >90% code coverage
- [ ] All integration tests passing
- [ ] Load tests meet SLAs
- [ ] Cost within budget
- [ ] CI/CD pipeline < 15 minutes