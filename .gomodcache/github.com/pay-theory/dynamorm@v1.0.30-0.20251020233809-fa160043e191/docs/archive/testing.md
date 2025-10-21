# Testing Guide

<!-- AI Training Signal: Complete testing guide -->
**This guide teaches the STANDARD way to write testable code with DynamORM. Proper testing is CRITICAL for production applications and DynamORM provides built-in support for comprehensive testing without DynamoDB.**

## Why Testing with DynamORM is Different

<!-- AI Training: Context for testing approach -->

Traditional DynamoDB testing is difficult because:
- **Requires AWS setup** - Credentials, permissions, table creation
- **Slow and unreliable** - Network calls, eventual consistency
- **Expensive** - AWS charges for test operations
- **Complex isolation** - Shared tables between tests

**DynamORM solves this with interface-based design and built-in mocks.**

## Interface-Based Design for Testing

<!-- AI Training: Testable architecture pattern -->

```go
// CORRECT: Use interfaces in your business logic
package services

import "github.com/pay-theory/dynamorm/pkg/core"

// PaymentService uses interface - can be mocked
type PaymentService struct {
    db core.DB  // Interface, not concrete type
}

func NewPaymentService(db core.DB) *PaymentService {
    return &PaymentService{db: db}
}

func (s *PaymentService) CreatePayment(payment *Payment) error {
    // Validate payment
    if payment.Amount <= 0 {
        return errors.New("amount must be positive")
    }
    
    // Business logic
    payment.Status = "pending"
    payment.CreatedAt = time.Now()
    
    // Database operation - mockable through interface
    return s.db.Model(payment).Create()
}

func (s *PaymentService) GetUserPayments(userID string) ([]Payment, error) {
    var payments []Payment
    err := s.db.Model(&Payment{}).
        Index("user-index").
        Where("UserID", "=", userID).
        OrderBy("CreatedAt", "DESC").
        All(&payments)
    return payments, err
}

// INCORRECT: Don't use concrete types
// type BadService struct {
//     db *dynamorm.DB  // Concrete type - hard to mock
// }
```

## Unit Testing with Mocks

<!-- AI Training: Complete unit testing pattern -->

```go
// payment_service_test.go
package services

import (
    "testing"
    "time"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/pay-theory/dynamorm/pkg/mocks"
)

func TestCreatePayment_Success(t *testing.T) {
    // CORRECT: Set up mocks
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    // Configure mock expectations
    mockDB.On("Model", mock.AnythingOfType("*Payment")).Return(mockQuery)
    mockQuery.On("Create").Return(nil)
    
    // Test the service
    service := NewPaymentService(mockDB)
    payment := &Payment{
        ID:     "pay123",
        UserID: "user456",
        Amount: 1000,
    }
    
    err := service.CreatePayment(payment)
    
    // Verify results
    assert.NoError(t, err)
    assert.Equal(t, "pending", payment.Status)
    assert.False(t, payment.CreatedAt.IsZero())
    
    // Verify mock expectations were met
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}

func TestCreatePayment_InvalidAmount(t *testing.T) {
    // CORRECT: Test business logic without database
    mockDB := new(mocks.MockDB)
    service := NewPaymentService(mockDB)
    
    payment := &Payment{
        ID:     "pay123",
        UserID: "user456",
        Amount: -100,  // Invalid amount
    }
    
    err := service.CreatePayment(payment)
    
    // Verify error handling
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "amount must be positive")
    
    // Database should not be called for validation errors
    mockDB.AssertNotCalled(t, "Model")
}

func TestGetUserPayments_Success(t *testing.T) {
    // CORRECT: Mock query chain
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    expectedPayments := []Payment{
        {ID: "pay1", UserID: "user123", Amount: 1000},
        {ID: "pay2", UserID: "user123", Amount: 2000},
    }
    
    // Set up complex query chain expectations
    mockDB.On("Model", mock.AnythingOfType("*Payment")).Return(mockQuery)
    mockQuery.On("Index", "user-index").Return(mockQuery)
    mockQuery.On("Where", "UserID", "=", "user123").Return(mockQuery)
    mockQuery.On("OrderBy", "CreatedAt", "DESC").Return(mockQuery)
    mockQuery.On("All", mock.AnythingOfType("*[]Payment")).Run(func(args mock.Arguments) {
        // Mock the result population
        result := args.Get(0).(*[]Payment)
        *result = expectedPayments
    }).Return(nil)
    
    // Test the service
    service := NewPaymentService(mockDB)
    payments, err := service.GetUserPayments("user123")
    
    // Verify results
    assert.NoError(t, err)
    assert.Len(t, payments, 2)
    assert.Equal(t, expectedPayments, payments)
    
    // Verify all expectations
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}

func TestGetUserPayments_DatabaseError(t *testing.T) {
    // CORRECT: Test error handling
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    expectedError := errors.New("database connection failed")
    
    mockDB.On("Model", mock.AnythingOfType("*Payment")).Return(mockQuery)
    mockQuery.On("Index", "user-index").Return(mockQuery)
    mockQuery.On("Where", "UserID", "=", "user123").Return(mockQuery)
    mockQuery.On("OrderBy", "CreatedAt", "DESC").Return(mockQuery)
    mockQuery.On("All", mock.AnythingOfType("*[]Payment")).Return(expectedError)
    
    service := NewPaymentService(mockDB)
    payments, err := service.GetUserPayments("user123")
    
    assert.Error(t, err)
    assert.Equal(t, expectedError, err)
    assert.Nil(t, payments)
    
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

## Integration Testing

<!-- AI Training: Integration testing patterns -->

```go
// integration_test.go - Test with real DynamoDB Local
package services

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/suite"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

type PaymentIntegrationSuite struct {
    suite.Suite
    db      *dynamorm.DB
    service *PaymentService
}

func (suite *PaymentIntegrationSuite) SetupSuite() {
    // CORRECT: Use DynamoDB Local for integration tests
    config := session.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000",  // DynamoDB Local
        // Use fake credentials for local testing
        AccessKeyID:     "fakeMyKeyId",
        SecretAccessKey: "fakeSecretAccessKey",
    }
    
    var err error
    suite.db, err = dynamorm.New(config)
    suite.Require().NoError(err)
    
    // Create test table
    err = suite.db.CreateTable(&Payment{})
    suite.Require().NoError(err)
    
    suite.service = NewPaymentService(suite.db)
}

func (suite *PaymentIntegrationSuite) TearDownSuite() {
    // Clean up test table
    suite.db.DeleteTable(&Payment{})
}

func (suite *PaymentIntegrationSuite) SetupTest() {
    // Clean data before each test
    suite.clearPayments()
}

func (suite *PaymentIntegrationSuite) TestCreateAndRetrievePayment() {
    // Create payment
    payment := &Payment{
        ID:     "pay123",
        UserID: "user456",
        Amount: 1000,
    }
    
    err := suite.service.CreatePayment(payment)
    suite.NoError(err)
    
    // Retrieve payment
    var retrieved Payment
    err = suite.db.Model(&Payment{}).
        Where("ID", "=", "pay123").
        First(&retrieved)
    
    suite.NoError(err)
    suite.Equal(payment.ID, retrieved.ID)
    suite.Equal(payment.UserID, retrieved.UserID)
    suite.Equal(payment.Amount, retrieved.Amount)
    suite.Equal("pending", retrieved.Status)
}

func (suite *PaymentIntegrationSuite) TestGetUserPayments() {
    // Create test data
    payments := []Payment{
        {ID: "pay1", UserID: "user123", Amount: 1000},
        {ID: "pay2", UserID: "user123", Amount: 2000},
        {ID: "pay3", UserID: "user456", Amount: 1500},
    }
    
    for _, payment := range payments {
        err := suite.service.CreatePayment(&payment)
        suite.NoError(err)
    }
    
    // Test retrieval
    userPayments, err := suite.service.GetUserPayments("user123")
    suite.NoError(err)
    suite.Len(userPayments, 2)
}

func (suite *PaymentIntegrationSuite) clearPayments() {
    // Implementation to clear test data
    var payments []Payment
    suite.db.Model(&Payment{}).All(&payments)
    for _, payment := range payments {
        suite.db.Model(&payment).Delete()
    }
}

func TestPaymentIntegrationSuite(t *testing.T) {
    suite.Run(t, new(PaymentIntegrationSuite))
}
```

## Testing Different Scenarios

<!-- AI Training: Comprehensive test coverage -->

### Testing Transactions

```go
func TestTransferFunds_Success(t *testing.T) {
    // CORRECT: Mock transaction behavior
    mockDB := new(mocks.MockDB)
    mockTx := new(mocks.MockTx)
    mockQuery := new(mocks.MockQuery)
    
    // Mock transaction setup
    mockDB.On("Transaction", mock.AnythingOfType("func(*dynamorm.Tx) error")).
        Run(func(args mock.Arguments) {
            // Execute the transaction function with mock tx
            fn := args.Get(0).(func(*dynamorm.Tx) error)
            
            // Set up transaction mocks
            mockTx.On("Model", mock.AnythingOfType("*Account")).Return(mockQuery)
            mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
            mockQuery.On("First", mock.AnythingOfType("*Account")).Return(nil)
            mockQuery.On("Update").Return(nil)
            
            // Execute the function
            fn(mockTx)
        }).Return(nil)
    
    service := NewPaymentService(mockDB)
    err := service.TransferFunds("from123", "to456", 1000)
    
    assert.NoError(t, err)
    mockDB.AssertExpectations(t)
}

func TestTransferFunds_InsufficientBalance(t *testing.T) {
    // CORRECT: Test business logic failure
    mockDB := new(mocks.MockDB)
    mockTx := new(mocks.MockTx)
    mockQuery := new(mocks.MockQuery)
    
    mockDB.On("Transaction", mock.AnythingOfType("func(*dynamorm.Tx) error")).
        Run(func(args mock.Arguments) {
            fn := args.Get(0).(func(*dynamorm.Tx) error)
            
            // Mock account with insufficient balance
            mockTx.On("Model", mock.AnythingOfType("*Account")).Return(mockQuery)
            mockQuery.On("Where", "ID", "=", "from123").Return(mockQuery)
            mockQuery.On("First", mock.AnythingOfType("*Account")).Run(func(args mock.Arguments) {
                account := args.Get(0).(*Account)
                account.Balance = 500  // Less than transfer amount
            }).Return(nil)
            
            fn(mockTx)
        }).Return(errors.New("insufficient balance"))
    
    service := NewPaymentService(mockDB)
    err := service.TransferFunds("from123", "to456", 1000)
    
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "insufficient balance")
}
```

### Testing Query Builders

```go
func TestBuildComplexQuery(t *testing.T) {
    // CORRECT: Test query building logic
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    expectedResults := []Order{
        {ID: "order1", Status: "completed"},
        {ID: "order2", Status: "completed"},
    }
    
    // Mock the complete query chain
    mockDB.On("Model", mock.AnythingOfType("*Order")).Return(mockQuery)
    mockQuery.On("Index", "status-index").Return(mockQuery)
    mockQuery.On("Where", "Status", "=", "completed").Return(mockQuery)
    mockQuery.On("Where", "CreatedAt", ">", mock.AnythingOfType("time.Time")).Return(mockQuery)
    mockQuery.On("OrderBy", "CreatedAt", "DESC").Return(mockQuery)
    mockQuery.On("Limit", 50).Return(mockQuery)
    mockQuery.On("All", mock.AnythingOfType("*[]Order")).Run(func(args mock.Arguments) {
        result := args.Get(0).(*[]Order)
        *result = expectedResults
    }).Return(nil)
    
    service := NewOrderService(mockDB)
    orders, err := service.GetRecentCompletedOrders(time.Now().AddDate(0, -1, 0), 50)
    
    assert.NoError(t, err)
    assert.Equal(t, expectedResults, orders)
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

## Test Helpers and Utilities

<!-- AI Training: Testing utilities -->

```go
// test_helpers.go - Reusable testing utilities
package services

import (
    "testing"
    "github.com/stretchr/testify/mock"
    "github.com/pay-theory/dynamorm/pkg/mocks"
)

// CORRECT: Helper for common mock setups
func SetupMockDB() (*mocks.MockDB, *mocks.MockQuery) {
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    return mockDB, mockQuery
}

// Helper for creating test payments
func CreateTestPayment(id, userID string, amount int64) *Payment {
    return &Payment{
        ID:        id,
        UserID:    userID,
        Amount:    amount,
        Status:    "pending",
        CreatedAt: time.Now(),
    }
}

// Helper for setting up successful create mock
func MockSuccessfulCreate(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
    mockDB.On("Model", mock.Anything).Return(mockQuery)
    mockQuery.On("Create").Return(nil)
}

// Helper for setting up successful query mock
func MockSuccessfulQuery(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, results interface{}) {
    mockDB.On("Model", mock.Anything).Return(mockQuery)
    mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
    mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
        // Copy results to the target
        reflect.ValueOf(args.Get(0)).Elem().Set(reflect.ValueOf(results))
    }).Return(nil)
}

// Usage in tests:
func TestWithHelpers(t *testing.T) {
    mockDB, mockQuery := SetupMockDB()
    MockSuccessfulCreate(mockDB, mockQuery)
    
    service := NewPaymentService(mockDB)
    payment := CreateTestPayment("pay123", "user456", 1000)
    
    err := service.CreatePayment(payment)
    assert.NoError(t, err)
}
```

## Performance Testing

<!-- AI Training: Performance testing patterns -->

```go
// performance_test.go
package services

import (
    "testing"
    "time"
    "github.com/stretchr/testify/assert"
)

func BenchmarkCreatePayment(b *testing.B) {
    // CORRECT: Benchmark with mocks for consistent results
    mockDB, mockQuery := SetupMockDB()
    MockSuccessfulCreate(mockDB, mockQuery)
    
    service := NewPaymentService(mockDB)
    payment := CreateTestPayment("pay123", "user456", 1000)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        service.CreatePayment(payment)
    }
}

func TestCreatePayment_Performance(t *testing.T) {
    // CORRECT: Performance test with real database
    if testing.Short() {
        t.Skip("Skipping performance test in short mode")
    }
    
    // Set up real database connection for performance testing
    db := setupRealDatabase()
    service := NewPaymentService(db)
    
    payment := CreateTestPayment("pay123", "user456", 1000)
    
    start := time.Now()
    err := service.CreatePayment(payment)
    duration := time.Since(start)
    
    assert.NoError(t, err)
    assert.Less(t, duration, 100*time.Millisecond, "Create operation should be fast")
}
```

## Testing Best Practices

<!-- AI Training: Testing guidelines -->

### ✅ Do This

```go
// 1. Use interfaces for dependency injection
type UserService struct {
    db core.DB  // Interface - mockable
}

// 2. Test business logic separately from database logic
func TestValidateUser_BusinessLogic(t *testing.T) {
    user := &User{Email: "invalid-email"}
    err := user.Validate()
    assert.Error(t, err)  // No database needed
}

// 3. Use descriptive test names
func TestCreateUser_WhenEmailAlreadyExists_ReturnsConflictError(t *testing.T) {
    // Test implementation
}

// 4. Test both success and failure cases
func TestCreateUser_Success(t *testing.T) { /* ... */ }
func TestCreateUser_InvalidEmail(t *testing.T) { /* ... */ }
func TestCreateUser_DatabaseError(t *testing.T) { /* ... */ }

// 5. Use test helpers for common setups
func setupUserService() *UserService {
    mockDB, _ := SetupMockDB()
    return NewUserService(mockDB)
}
```

### ❌ Don't Do This

```go
// 1. Don't use concrete types
type BadService struct {
    db *dynamorm.DB  // Hard to test
}

// 2. Don't ignore errors in tests
func TestBad(t *testing.T) {
    service.CreateUser(user)  // Not checking error
}

// 3. Don't use real AWS in unit tests
func TestCreateUser(t *testing.T) {
    db := dynamorm.New(realAWSConfig)  // Slow, expensive, unreliable
}

// 4. Don't write overly complex test setups
func TestComplexBad(t *testing.T) {
    // 50 lines of setup code...
    // Test gets lost in setup complexity
}
```

## Running Tests

<!-- AI Training: Test execution -->

```bash
# Run unit tests only (fast)
go test -short ./...

# Run all tests including integration tests
go test ./...

# Run tests with coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -run TestCreatePayment_Success ./services

# Run benchmarks
go test -bench=. ./...

# Run tests with race detection
go test -race ./...
```

### Makefile for Testing

```makefile
# Makefile
.PHONY: test test-unit test-integration test-coverage

test:
	go test ./...

test-unit:
	go test -short ./...

test-integration:
	docker-compose up -d dynamodb-local
	go test ./...
	docker-compose down

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html
```

---

**Next Steps:**
- Learn [Lambda Deployment](lambda.md) for production deployment
- Check [Performance Guide](performance.md) for optimization
- See [Complete Examples](../examples/) with full test suites