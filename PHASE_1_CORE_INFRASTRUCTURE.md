# Phase 1: Core Infrastructure - Detailed Implementation Checklist

## 1.1 Standardize Lift Application Structure

### 1.1.1 Create Shared Application Factory
**File:** `pkg/lift/app.go`

- [ ] Create base application factory
  ```go
  package lift
  
  import (
      "github.com/pay-theory/lift/pkg/lift"
      "github.com/pay-theory/lift/pkg/middleware"
  )
  
  type AppConfig struct {
      Debug bool
      Timeout time.Duration
      EnableCORS bool
      EnableMetrics bool
  }
  
  func NewApp(cfg AppConfig) *lift.App {
      opts := []lift.Option{}
      if cfg.Debug {
          opts = append(opts, lift.WithDebug())
      }
      
      app := lift.New(opts...)
      
      // Add standard middleware
      if cfg.Timeout > 0 {
          app.Use(middleware.TimeoutMiddleware(middleware.TimeoutConfig{
              DefaultTimeout: cfg.Timeout,
          }))
      }
      
      return app
  }
  ```

- [ ] Create Lambda-specific app builders
  ```go
  func NewHTTPApp(cfg AppConfig) *lift.App
  func NewSQSApp(cfg AppConfig) *lift.App
  func NewDynamoDBStreamApp(cfg AppConfig) *lift.App
  ```

- [ ] Add initialization helpers
  ```go
  func InitializeApp(appType string) (*lift.App, error)
  ```

**Testing Requirements:**
- [ ] Unit tests for each app builder
- [ ] Test middleware ordering
- [ ] Test configuration validation

**Acceptance Criteria:**
- All app builders return properly configured Lift apps
- Middleware is applied in correct order
- Configuration is validated

### 1.1.2 Implement Common Middleware Stack
**File:** `pkg/lift/middleware/stack.go`

- [ ] Create logging middleware
  ```go
  func LoggingMiddleware(logger *zap.Logger) lift.Middleware {
      return func(next lift.HandlerFunc) lift.HandlerFunc {
          return func(ctx *lift.Context) error {
              start := time.Now()
              
              // Add request ID to context
              requestID := uuid.New().String()
              ctx.Set("requestID", requestID)
              
              // Log request
              logger.Info("request_start",
                  zap.String("request_id", requestID),
                  zap.String("method", ctx.Request().Method),
                  zap.String("path", ctx.Request().URL.Path),
              )
              
              // Process request
              err := next(ctx)
              
              // Log response
              duration := time.Since(start)
              logger.Info("request_complete",
                  zap.String("request_id", requestID),
                  zap.Duration("duration", duration),
                  zap.Error(err),
              )
              
              return err
          }
      }
  }
  ```

- [ ] Create CORS middleware
  ```go
  func CORSMiddleware(allowedOrigins []string) lift.Middleware
  ```

- [ ] Create authentication middleware
  ```go
  func AuthMiddleware(authService auth.Service) lift.Middleware {
      return func(next lift.HandlerFunc) lift.HandlerFunc {
          return func(ctx *lift.Context) error {
              token := extractToken(ctx.Request())
              if token == "" {
                  return lift.NewError(http.StatusUnauthorized, "missing token")
              }
              
              claims, err := authService.ValidateToken(token)
              if err != nil {
                  return lift.NewError(http.StatusUnauthorized, "invalid token")
              }
              
              ctx.Set("claims", claims)
              ctx.Set("userID", claims.UserID)
              
              return next(ctx)
          }
      }
  }
  ```

- [ ] Create cost tracking middleware
  ```go
  func CostTrackingMiddleware() lift.Middleware
  ```

- [ ] Create rate limiting middleware
  ```go
  func RateLimitMiddleware(limiter RateLimiter) lift.Middleware
  ```

**Testing Requirements:**
- [ ] Test each middleware independently
- [ ] Test middleware chain execution
- [ ] Test error handling in middleware
- [ ] Test context propagation

**Acceptance Criteria:**
- All middleware follows Lift patterns
- Proper error handling and propagation
- Context values are accessible downstream
- Performance overhead is minimal

### 1.1.3 Standardize Error Handling
**File:** `pkg/lift/errors/errors.go`

- [ ] Create custom error types
  ```go
  package errors
  
  import "github.com/pay-theory/lift/pkg/lift"
  
  type DomainError struct {
      Code    string
      Message string
      Details map[string]any
  }
  
  func (e DomainError) Error() string {
      return e.Message
  }
  
  func NewValidationError(field, message string) error {
      return lift.NewError(http.StatusBadRequest, message).
          WithDetail("field", field).
          WithDetail("code", "VALIDATION_ERROR")
  }
  
  func NewNotFoundError(resource string) error {
      return lift.NewError(http.StatusNotFound, "resource not found").
          WithDetail("resource", resource).
          WithDetail("code", "NOT_FOUND")
  }
  ```

- [ ] Create error handler middleware
  ```go
  func ErrorHandlerMiddleware() lift.Middleware {
      return func(next lift.HandlerFunc) lift.HandlerFunc {
          return func(ctx *lift.Context) error {
              err := next(ctx)
              if err == nil {
                  return nil
              }
              
              // Convert domain errors to Lift errors
              if domainErr, ok := err.(DomainError); ok {
                  return lift.NewError(http.StatusBadRequest, domainErr.Message).
                      WithDetail("code", domainErr.Code)
              }
              
              // Handle other error types
              return err
          }
      }
  }
  ```

- [ ] Create error response formatter
  ```go
  func FormatErrorResponse(err error) map[string]any
  ```

**Testing Requirements:**
- [ ] Test error conversion
- [ ] Test error response formatting
- [ ] Test error logging

**Acceptance Criteria:**
- Consistent error format across all endpoints
- Proper HTTP status codes
- Detailed error information for debugging
- No sensitive information in error responses

### 1.1.4 Create Context Utilities
**File:** `pkg/lift/context/utils.go`

- [ ] Create context helpers
  ```go
  package context
  
  import "github.com/pay-theory/lift/pkg/lift"
  
  func GetUserID(ctx *lift.Context) (string, error) {
      userID, ok := ctx.Get("userID").(string)
      if !ok {
          return "", errors.New("user ID not found in context")
      }
      return userID, nil
  }
  
  func GetClaims(ctx *lift.Context) (*auth.Claims, error) {
      claims, ok := ctx.Get("claims").(*auth.Claims)
      if !ok {
          return nil, errors.New("claims not found in context")
      }
      return claims, nil
  }
  
  func GetRequestID(ctx *lift.Context) string {
      requestID, _ := ctx.Get("requestID").(string)
      return requestID
  }
  
  func WithCostTracking(ctx *lift.Context) context.Context {
      return storage.WithCostTracking(ctx.Request().Context())
  }
  ```

- [ ] Create pagination helpers
  ```go
  func GetPaginationParams(ctx *lift.Context) (limit, offset int)
  func SetPaginationHeaders(ctx *lift.Context, total, limit, offset int)
  ```

- [ ] Create response helpers
  ```go
  func RespondWithData(ctx *lift.Context, data any) error
  func RespondWithError(ctx *lift.Context, err error) error
  func RespondWithPagination(ctx *lift.Context, data any, pagination Pagination) error
  ```

**Testing Requirements:**
- [ ] Test context value retrieval
- [ ] Test error cases
- [ ] Test response formatting

**Acceptance Criteria:**
- Type-safe context access
- Consistent response formats
- Proper error handling

## 1.2 Complete DynamORM Infrastructure

### 1.2.1 Enable Cost Tracking
**File:** `pkg/storage/dynamorm/cost/tracker.go`

- [ ] Create cost tracking wrapper
  ```go
  package cost
  
  import (
      "context"
      "github.com/pay-theory/dynamorm"
  )
  
  type CostTracker struct {
      client *dynamorm.Client
  }
  
  func NewCostTracker(client *dynamorm.Client) *CostTracker {
      return &CostTracker{client: client}
  }
  
  func (ct *CostTracker) TrackOperation(ctx context.Context, operation string, fn func() error) error {
      // Get initial consumed capacity
      initialCapacity := ct.client.GetConsumedCapacity()
      
      // Execute operation
      err := fn()
      
      // Calculate cost
      finalCapacity := ct.client.GetConsumedCapacity()
      cost := calculateCost(finalCapacity - initialCapacity)
      
      // Store cost in context
      storage.AddOperationCost(ctx, operation, cost)
      
      return err
  }
  ```

- [ ] Create cost calculation utilities
  ```go
  func calculateCost(capacity ConsumedCapacity) float64
  func aggregateCosts(ctx context.Context) CostReport
  ```

- [ ] Integrate with repositories
  ```go
  func (r *UserRepository) GetByID(ctx context.Context, id string) (*User, error) {
      return r.costTracker.TrackOperation(ctx, "GetUserByID", func() error {
          // Existing GetByID logic
      })
  }
  ```

**Testing Requirements:**
- [ ] Test cost calculation accuracy
- [ ] Test cost aggregation
- [ ] Test context integration

**Acceptance Criteria:**
- All DynamoDB operations track costs
- Costs are aggregated per request
- Cost reports are accurate

### 1.2.2 Implement Transaction Utilities
**File:** `pkg/storage/dynamorm/transactions/manager.go`

- [ ] Create transaction manager
  ```go
  package transactions
  
  import "github.com/pay-theory/dynamorm"
  
  type TransactionManager struct {
      client *dynamorm.Client
  }
  
  func NewTransactionManager(client *dynamorm.Client) *TransactionManager {
      return &TransactionManager{client: client}
  }
  
  func (tm *TransactionManager) ExecuteWrite(operations ...TransactionOperation) error {
      tx := tm.client.NewTransaction()
      
      for _, op := range operations {
          switch op.Type {
          case OperationPut:
              tx.Put(op.Item)
          case OperationUpdate:
              tx.Update(op.Item, op.UpdateExpression)
          case OperationDelete:
              tx.Delete(op.Key)
          case OperationConditionCheck:
              tx.ConditionCheck(op.Key, op.Condition)
          }
      }
      
      return tx.Execute()
  }
  ```

- [ ] Create transaction builders
  ```go
  type TransactionBuilder struct {
      operations []TransactionOperation
  }
  
  func (tb *TransactionBuilder) Put(item any) *TransactionBuilder
  func (tb *TransactionBuilder) Update(item any, expr string) *TransactionBuilder
  func (tb *TransactionBuilder) Delete(key dynamorm.Key) *TransactionBuilder
  func (tb *TransactionBuilder) Build() []TransactionOperation
  ```

- [ ] Add retry logic
  ```go
  func (tm *TransactionManager) ExecuteWithRetry(operations ...TransactionOperation) error {
      return retry.Do(func() error {
          return tm.ExecuteWrite(operations...)
      }, retry.Attempts(3), retry.OnTransactionConflict())
  }
  ```

**Testing Requirements:**
- [ ] Test transaction success cases
- [ ] Test transaction rollback
- [ ] Test conflict resolution
- [ ] Test retry logic

**Acceptance Criteria:**
- Transactions maintain ACID properties
- Proper error handling for conflicts
- Retry logic handles transient failures

### 1.2.3 Create Batch Operation Helpers
**File:** `pkg/storage/dynamorm/batch/operations.go`

- [ ] Create batch writer
  ```go
  package batch
  
  type BatchWriter struct {
      client    *dynamorm.Client
      batchSize int
  }
  
  func NewBatchWriter(client *dynamorm.Client, batchSize int) *BatchWriter {
      return &BatchWriter{
          client:    client,
          batchSize: batchSize,
      }
  }
  
  func (bw *BatchWriter) WriteItems(items []any) error {
      for i := 0; i < len(items); i += bw.batchSize {
          end := i + bw.batchSize
          if end > len(items) {
              end = len(items)
          }
          
          batch := items[i:end]
          if err := bw.writeBatch(batch); err != nil {
              return fmt.Errorf("batch write failed at index %d: %w", i, err)
          }
      }
      return nil
  }
  ```

- [ ] Create batch reader
  ```go
  func (br *BatchReader) GetItems(keys []dynamorm.Key) ([]any, error)
  ```

- [ ] Add parallel processing
  ```go
  func (bw *BatchWriter) WriteItemsParallel(items []any, workers int) error
  ```

**Testing Requirements:**
- [ ] Test batch size limits
- [ ] Test error handling mid-batch
- [ ] Test parallel processing

**Acceptance Criteria:**
- Respects DynamoDB batch limits
- Handles partial failures
- Provides progress tracking

### 1.2.4 Migration Utilities
**File:** `pkg/storage/dynamorm/migrations/migrator.go`

- [ ] Create migration interface
  ```go
  package migrations
  
  type Migration interface {
      ID() string
      Up(client *dynamorm.Client) error
      Down(client *dynamorm.Client) error
  }
  
  type Migrator struct {
      client     *dynamorm.Client
      migrations []Migration
  }
  
  func (m *Migrator) Run() error {
      applied := m.getAppliedMigrations()
      
      for _, migration := range m.migrations {
          if !applied[migration.ID()] {
              if err := m.applyMigration(migration); err != nil {
                  return fmt.Errorf("migration %s failed: %w", migration.ID(), err)
              }
          }
      }
      
      return nil
  }
  ```

- [ ] Create GSI migration helpers
  ```go
  func AddGSI(table, name string, hashKey, rangeKey string) Migration
  func UpdateGSI(table, name string, throughput Throughput) Migration
  ```

- [ ] Add validation
  ```go
  func (m *Migrator) Validate() error {
      // Check for duplicate IDs
      // Verify migration order
      // Check dependencies
  }
  ```

**Testing Requirements:**
- [ ] Test migration execution
- [ ] Test rollback functionality
- [ ] Test idempotency

**Acceptance Criteria:**
- Migrations are idempotent
- Rollback works correctly
- Migration history is tracked

## 1.3 Enhanced Authentication Integration

### 1.3.1 Migrate Auth Logic to Lift Middleware
**File:** `pkg/lift/middleware/auth.go`

- [ ] Create unified auth middleware
  ```go
  package middleware
  
  type AuthConfig struct {
      TokenValidator TokenValidator
      RequireAuth    bool
      AllowedScopes  []string
  }
  
  func NewAuthMiddleware(cfg AuthConfig) lift.Middleware {
      return func(next lift.HandlerFunc) lift.HandlerFunc {
          return func(ctx *lift.Context) error {
              token := extractBearerToken(ctx.Request())
              
              if token == "" && cfg.RequireAuth {
                  return lift.NewError(http.StatusUnauthorized, "authentication required")
              }
              
              if token != "" {
                  claims, err := cfg.TokenValidator.Validate(token)
                  if err != nil {
                      return lift.NewError(http.StatusUnauthorized, "invalid token")
                  }
                  
                  if !hasRequiredScopes(claims.Scopes, cfg.AllowedScopes) {
                      return lift.NewError(http.StatusForbidden, "insufficient permissions")
                  }
                  
                  ctx.Set("auth:claims", claims)
                  ctx.Set("auth:userID", claims.UserID)
                  ctx.Set("auth:scopes", claims.Scopes)
              }
              
              return next(ctx)
          }
      }
  }
  ```

- [ ] Create OAuth middleware
  ```go
  func OAuthMiddleware(provider OAuthProvider) lift.Middleware
  ```

- [ ] Create WebAuthn middleware
  ```go
  func WebAuthnMiddleware(authenticator WebAuthnAuthenticator) lift.Middleware
  ```

**Testing Requirements:**
- [ ] Test token extraction
- [ ] Test scope validation
- [ ] Test multiple auth methods

**Acceptance Criteria:**
- Supports all auth methods (OAuth, WebAuthn, API keys)
- Proper error messages
- Performance optimized

### 1.3.2 Implement Claims Storage
**File:** `pkg/lift/auth/claims.go`

- [ ] Define claims structure
  ```go
  package auth
  
  type Claims struct {
      UserID    string   `json:"sub"`
      Email     string   `json:"email"`
      Scopes    []string `json:"scopes"`
      IssuedAt  int64    `json:"iat"`
      ExpiresAt int64    `json:"exp"`
      Extra     map[string]any `json:"extra,omitempty"`
  }
  
  func (c *Claims) HasScope(scope string) bool
  func (c *Claims) IsExpired() bool
  func (c *Claims) Validate() error
  ```

- [ ] Create claims helpers
  ```go
  func GetClaims(ctx *lift.Context) (*Claims, error)
  func MustGetClaims(ctx *lift.Context) *Claims
  func SetClaims(ctx *lift.Context, claims *Claims)
  ```

**Testing Requirements:**
- [ ] Test claims validation
- [ ] Test context storage/retrieval
- [ ] Test expiration logic

**Acceptance Criteria:**
- Type-safe claims access
- Proper validation
- Easy to use in handlers

### 1.3.3 Multi-tenant Support
**File:** `pkg/lift/auth/tenant.go`

- [ ] Create tenant resolver
  ```go
  package auth
  
  type TenantResolver interface {
      ResolveTenant(ctx *lift.Context) (string, error)
  }
  
  type HeaderTenantResolver struct {
      headerName string
  }
  
  func (r *HeaderTenantResolver) ResolveTenant(ctx *lift.Context) (string, error) {
      tenant := ctx.Request().Header.Get(r.headerName)
      if tenant == "" {
          return "", errors.New("tenant not specified")
      }
      return tenant, nil
  }
  ```

- [ ] Create tenant middleware
  ```go
  func TenantMiddleware(resolver TenantResolver) lift.Middleware
  ```

- [ ] Add tenant isolation
  ```go
  func WithTenant(ctx context.Context, tenantID string) context.Context
  func GetTenant(ctx context.Context) (string, error)
  ```

**Testing Requirements:**
- [ ] Test tenant resolution methods
- [ ] Test isolation between tenants
- [ ] Test missing tenant handling

**Acceptance Criteria:**
- Multiple tenant resolution strategies
- Proper isolation
- Clear error messages

### 1.3.4 Auth Testing Helpers
**File:** `pkg/lift/auth/testing/helpers.go`

- [ ] Create test token generator
  ```go
  package testing
  
  func GenerateTestToken(claims *Claims) string
  func GenerateExpiredToken(userID string) string
  func GenerateAdminToken(userID string) string
  ```

- [ ] Create mock authenticator
  ```go
  type MockAuthenticator struct {
      ValidTokens map[string]*Claims
      ShouldFail  bool
  }
  
  func (m *MockAuthenticator) Validate(token string) (*Claims, error)
  ```

- [ ] Create test contexts
  ```go
  func NewAuthenticatedContext(userID string, scopes ...string) *lift.Context
  func NewUnauthenticatedContext() *lift.Context
  ```

**Testing Requirements:**
- [ ] Test helper functionality
- [ ] Test mock behavior
- [ ] Test edge cases

**Acceptance Criteria:**
- Easy to use in tests
- Covers common scenarios
- Consistent with production behavior

## Implementation Notes

### Dependencies
- Phase 1.1 must be completed before starting Phase 2
- Authentication middleware should be tested with existing handlers before full migration
- Cost tracking should be validated against current billing

### Common Pitfalls to Avoid
1. Don't mix Lift context with standard context
2. Ensure middleware order is correct (auth before business logic)
3. Test cold start performance with new abstractions
4. Validate cost tracking accuracy before full rollout

### Performance Considerations
- Monitor Lambda package size
- Profile cold start times
- Benchmark middleware overhead
- Test concurrent request handling

### Rollback Plan
1. Keep existing implementations alongside new ones
2. Use feature flags to control rollout
3. Monitor error rates and performance
4. Have quick rollback procedure ready

## Success Metrics
- [ ] All core infrastructure in place
- [ ] No increase in error rates
- [ ] Cold start times remain under 1s
- [ ] Cost tracking accurate to $0.001
- [ ] All tests passing with >90% coverage