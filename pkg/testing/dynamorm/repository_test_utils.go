// Package dynamorm provides testing utilities and test cases for DynamORM repository validation.
package dynamorm

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// RepositoryTestCase defines a test case for repository testing
type RepositoryTestCase struct {
	Name         string
	SetupFunc    func(*mocks.MockDB)
	TestFunc     func(context.Context) error
	ExpectError  bool
	ErrorMsg     string
	ValidateFunc func(*testing.T, *mocks.MockDB)
}

// RepositoryTestSuite provides utilities for testing DynamORM repositories
type RepositoryTestSuite struct {
	t      *testing.T
	mockDB *mocks.MockDB
	ctx    context.Context
}

// NewRepositoryTestSuite creates a new repository test suite
func NewRepositoryTestSuite(t *testing.T) *RepositoryTestSuite {
	return &RepositoryTestSuite{
		t:      t,
		mockDB: new(mocks.MockDB),
		ctx:    context.Background(),
	}
}

// GetMockDB returns the mock database
func (s *RepositoryTestSuite) GetMockDB() *mocks.MockDB {
	return s.mockDB
}

// RunTest executes a repository test case
func (s *RepositoryTestSuite) RunTest(tc RepositoryTestCase) {
	s.t.Run(tc.Name, func(t *testing.T) {
		// Reset mock
		s.mockDB = new(mocks.MockDB)

		// Setup mocks
		if tc.SetupFunc != nil {
			tc.SetupFunc(s.mockDB)
		}

		// Execute test
		err := tc.TestFunc(s.ctx)

		// Validate error
		if tc.ExpectError {
			assert.Error(t, err)
			if tc.ErrorMsg != "" {
				assert.Contains(t, err.Error(), tc.ErrorMsg)
			}
		} else {
			assert.NoError(t, err)
		}

		// Custom validation
		if tc.ValidateFunc != nil {
			tc.ValidateFunc(t, s.mockDB)
		}

		// Assert expectations
		s.mockDB.AssertExpectations(t)
	})
}

// Common repository test scenarios

// TestCreate tests repository Create operations
func TestCreate(t *testing.T, createFunc func(context.Context, interface{}) error, item interface{}) {
	suite := NewRepositoryTestSuite(t)

	testCases := []RepositoryTestCase{
		{
			Name: "Successful create",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", item).Return(mockQuery)
				mockQuery.On("Create").Return(nil)
			},
			TestFunc: func(ctx context.Context) error {
				return createFunc(ctx, item)
			},
			ExpectError: false,
		},
		{
			Name: "Create with conflict",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", item).Return(mockQuery)
				mockQuery.On("Create").Return(dynamorm.ErrConditionalCheckFailed)
			},
			TestFunc: func(ctx context.Context) error {
				return createFunc(ctx, item)
			},
			ExpectError: true,
			ErrorMsg:    "already exists",
		},
		{
			Name: "Database error",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", item).Return(mockQuery)
				mockQuery.On("Create").Return(fmt.Errorf("database error"))
			},
			TestFunc: func(ctx context.Context) error {
				return createFunc(ctx, item)
			},
			ExpectError: true,
			ErrorMsg:    "database error",
		},
	}

	for _, tc := range testCases {
		suite.RunTest(tc)
	}
}

// TestGet tests repository Get operations
func TestGet(t *testing.T, getFunc func(context.Context, string) (interface{}, error), id string, expectedItem interface{}) {
	suite := NewRepositoryTestSuite(t)

	testCases := []RepositoryTestCase{
		{
			Name: "Successful get",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", mock.Anything).Return(mockQuery)
				mockQuery.On("Get", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
					// Copy expected item to the argument
					dest := args.Get(0)
					copyStruct(expectedItem, dest)
				})
			},
			TestFunc: func(ctx context.Context) error {
				_, err := getFunc(ctx, id)
				return err
			},
			ExpectError: false,
		},
		{
			Name: "Item not found",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", mock.Anything).Return(mockQuery)
				mockQuery.On("Get", mock.Anything).Return(storage.ErrNotFound)
			},
			TestFunc: func(ctx context.Context) error {
				_, err := getFunc(ctx, id)
				return err
			},
			ExpectError: true,
			ErrorMsg:    "not found",
		},
	}

	for _, tc := range testCases {
		suite.RunTest(tc)
	}
}

// TestUpdate tests repository Update operations
func TestUpdate(t *testing.T, updateFunc func(context.Context, interface{}) error, item interface{}) {
	suite := NewRepositoryTestSuite(t)

	testCases := []RepositoryTestCase{
		{
			Name: "Successful update",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", item).Return(mockQuery)
				mockQuery.On("Update").Return(nil)
			},
			TestFunc: func(ctx context.Context) error {
				return updateFunc(ctx, item)
			},
			ExpectError: false,
		},
		{
			Name: "Update non-existent item",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", item).Return(mockQuery)
				mockQuery.On("Update").Return(storage.ErrNotFound)
			},
			TestFunc: func(ctx context.Context) error {
				return updateFunc(ctx, item)
			},
			ExpectError: true,
			ErrorMsg:    "not found",
		},
		{
			Name: "Concurrent modification",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", item).Return(mockQuery)
				mockQuery.On("Update").Return(dynamorm.ErrConditionalCheckFailed)
			},
			TestFunc: func(ctx context.Context) error {
				return updateFunc(ctx, item)
			},
			ExpectError: true,
			ErrorMsg:    "concurrent modification",
		},
	}

	for _, tc := range testCases {
		suite.RunTest(tc)
	}
}

// TestDelete tests repository Delete operations
func TestDelete(t *testing.T, deleteFunc func(context.Context, string) error, id string) {
	suite := NewRepositoryTestSuite(t)

	testCases := []RepositoryTestCase{
		{
			Name: "Successful delete",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", mock.Anything).Return(mockQuery)
				mockQuery.On("Delete").Return(nil)
			},
			TestFunc: func(ctx context.Context) error {
				return deleteFunc(ctx, id)
			},
			ExpectError: false,
		},
		{
			Name: "Delete non-existent item",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", mock.Anything).Return(mockQuery)
				mockQuery.On("Delete").Return(nil) // DynamoDB doesn't error on missing deletes
			},
			TestFunc: func(ctx context.Context) error {
				return deleteFunc(ctx, id)
			},
			ExpectError: false,
		},
	}

	for _, tc := range testCases {
		suite.RunTest(tc)
	}
}

// TestQuery tests repository Query operations
func TestQuery(t *testing.T, queryFunc func(context.Context, string, int, string) ([]interface{}, string, error)) {
	suite := NewRepositoryTestSuite(t)

	mockItems := []interface{}{
		&models.Status{StatusID: "1"},
		&models.Status{StatusID: "2"},
	}

	testCases := []RepositoryTestCase{
		{
			Name: "Successful query",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", mock.Anything).Return(mockQuery)
				mockQuery.On("Where", mock.Anything, mock.Anything).Return(mockQuery)
				mockQuery.On("Limit", mock.Anything).Return(mockQuery)
				mockQuery.On("Query", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
					// Simulate returning items
					result := args.Get(0).(*[]interface{})
					*result = mockItems
				})
			},
			TestFunc: func(ctx context.Context) error {
				items, cursor, err := queryFunc(ctx, "test-id", 10, "")
				if err != nil {
					return err
				}
				assert.Len(t, items, 2)
				assert.NotEmpty(t, cursor)
				return nil
			},
			ExpectError: false,
		},
		{
			Name: "Empty results",
			SetupFunc: func(db *mocks.MockDB) {
				mockQuery := new(mocks.MockQuery)
				db.On("Model", mock.Anything).Return(mockQuery)
				mockQuery.On("Where", mock.Anything, mock.Anything).Return(mockQuery)
				mockQuery.On("Limit", mock.Anything).Return(mockQuery)
				mockQuery.On("Query", mock.Anything).Return(nil)
			},
			TestFunc: func(ctx context.Context) error {
				items, _, err := queryFunc(ctx, "test-id", 10, "")
				if err != nil {
					return err
				}
				assert.Empty(t, items)
				return nil
			},
			ExpectError: false,
		},
	}

	for _, tc := range testCases {
		suite.RunTest(tc)
	}
}

// TestTransaction tests repository transaction operations
func TestTransaction(t *testing.T, txFunc func(context.Context, func(core.Tx) error) error) {
	suite := NewRepositoryTestSuite(t)

	testCases := []RepositoryTestCase{
		{
			Name: "Successful transaction",
			SetupFunc: func(db *mocks.MockDB) {
				db.On("Transaction", mock.Anything).Return(nil)
			},
			TestFunc: func(ctx context.Context) error {
				return txFunc(ctx, func(_ core.Tx) error {
					return nil // Simplified for test
				})
			},
			ExpectError: false,
		},
		{
			Name: "Transaction rollback on error",
			SetupFunc: func(db *mocks.MockDB) {
				db.On("Transaction", mock.Anything).Return(fmt.Errorf("transaction failed"))
			},
			TestFunc: func(ctx context.Context) error {
				return txFunc(ctx, func(_ core.Tx) error {
					return fmt.Errorf("put error")
				})
			},
			ExpectError: true,
			ErrorMsg:    "transaction failed",
		},
	}

	for _, tc := range testCases {
		suite.RunTest(tc)
	}
}

// Performance test utilities

// BenchmarkRepository benchmarks repository operations
func BenchmarkRepository(b *testing.B, operation func()) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		operation()
	}
}

// TestConcurrentOperations tests concurrent repository operations
func TestConcurrentOperations(t *testing.T, operations []func() error, _ int) {
	var wg sync.WaitGroup
	errors := make(chan error, len(operations))

	// Execute operations concurrently
	for _, op := range operations {
		wg.Add(1)
		go func(operation func() error) {
			defer wg.Done()
			if err := operation(); err != nil {
				errors <- err
			}
		}(op)
	}

	// Wait for completion
	wg.Wait()
	close(errors)

	// Check for errors
	var errCount int
	for err := range errors {
		t.Logf("Concurrent operation error: %v", err)
		errCount++
	}

	assert.Less(t, errCount, len(operations)/2, "Too many concurrent operation failures")
}

// Test data builders

// BuildTestActor creates a test actor for repository testing
func BuildTestActor(id string) *models.Actor {
	return &models.Actor{
		PK:        fmt.Sprintf("actor#%s", id),
		SK:        fmt.Sprintf("actor#%s", id),
		Username:  fmt.Sprintf("user_%s", id),
		NumericID: id,
		GSI1PK:    "USERNAME_SEARCH#us",
		GSI1SK:    fmt.Sprintf("user_%s", id),
	}
}

// BuildTestStatus creates a test status for repository testing
func BuildTestStatus(id, authorID string) *models.Status {
	return &models.Status{
		PK:             fmt.Sprintf("status#%s", id),
		SK:             fmt.Sprintf("status#%s", id),
		StatusID:       id,
		AuthorID:       authorID,
		AuthorUsername: fmt.Sprintf("user_%s", authorID),
		Content:        "Test status content",
		GSI1PK:         fmt.Sprintf("AUTHOR#%s", authorID),
		GSI1SK:         fmt.Sprintf("%s#%s", time.Now().Format(time.RFC3339), id),
	}
}

// BuildTestTimeline creates a test timeline entry
func BuildTestTimeline(actorID, statusID string) *models.Timeline {
	return &models.Timeline{
		PK:         fmt.Sprintf("timeline#%s", actorID),
		SK:         fmt.Sprintf("%s#%s", time.Now().Format(time.RFC3339Nano), statusID),
		PostID:     statusID,
		TimelineID: fmt.Sprintf("%s:%s", actorID, statusID),
		GSI1PK:     fmt.Sprintf("HOME_TIMELINE#%s", actorID),
		GSI1SK:     time.Now().Format(time.RFC3339Nano),
	}
}

// Helper functions

// copyStruct copies struct values using reflection for generic type support
func copyStruct(src, dest interface{}) {
	srcVal := reflect.ValueOf(src)
	destVal := reflect.ValueOf(dest)

	// Ensure we have pointers
	if srcVal.Kind() != reflect.Ptr || destVal.Kind() != reflect.Ptr {
		return
	}

	// Get the underlying elements
	srcElem := srcVal.Elem()
	destElem := destVal.Elem()

	// Ensure both are structs and of the same type
	if srcElem.Kind() != reflect.Struct || destElem.Kind() != reflect.Struct {
		return
	}

	if srcElem.Type() != destElem.Type() {
		return
	}

	// Ensure dest is settable
	if !destElem.CanSet() {
		return
	}

	// Copy the struct
	destElem.Set(srcElem)
}

// AssertEventualConsistency tests eventual consistency scenarios
func AssertEventualConsistency(t *testing.T, checkFunc func() (bool, error), timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		success, err := checkFunc()
		if err != nil {
			t.Fatalf("Check function error: %v", err)
		}
		if success {
			return
		}
		<-ticker.C
	}

	t.Fatal("Eventual consistency check timed out")
}

// MockCostTracker tracks DynamoDB operation costs in tests
type MockCostTracker struct {
	mu         sync.Mutex
	operations map[string]int
	totalRCU   float64
	totalWCU   float64
}

// NewMockCostTracker creates a new cost tracker
func NewMockCostTracker() *MockCostTracker {
	return &MockCostTracker{
		operations: make(map[string]int),
	}
}

// TrackOperation records an operation
func (m *MockCostTracker) TrackOperation(operation string, rcu, wcu float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.operations[operation]++
	m.totalRCU += rcu
	m.totalWCU += wcu
}

// GetTotalCost returns total consumed capacity
func (m *MockCostTracker) GetTotalCost() (rcu, wcu float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totalRCU, m.totalWCU
}

// AssertCostWithinBudget verifies operations stay within cost budget
func (m *MockCostTracker) AssertCostWithinBudget(t *testing.T, maxRCU, maxWCU float64) {
	rcu, wcu := m.GetTotalCost()
	assert.LessOrEqual(t, rcu, maxRCU, "Read capacity exceeded budget")
	assert.LessOrEqual(t, wcu, maxWCU, "Write capacity exceeded budget")
}
