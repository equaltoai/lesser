package dynamorm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// TestUser is a test model
type TestUser struct {
	StandardModel
	Name string `json:"name"`
}

func TestTransaction_Execute(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)

	// Setup transaction mock behavior
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Run(func(args mock.Arguments) {
			// Extract the transaction function
			txFunc := args.Get(0).(func(*core.Tx) error)

			// Create a mock transaction
			mockTx := &core.Tx{}

			// Execute the transaction function with the mock transaction
			_ = txFunc(mockTx)
		}).
		Return(nil)

	// Create transaction wrapper
	tx := NewTransaction(mockDB)

	// Execute transaction
	err := tx.Execute(context.Background(), func(tx *Transaction) error {
		// Transaction logic here
		return nil
	})

	// Verify results
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestTransaction_ExecuteWithError(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)

	// Expected error
	expectedErr := errors.New("transaction error")

	// Setup transaction mock behavior
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Run(func(args mock.Arguments) {
			// Extract the transaction function
			txFunc := args.Get(0).(func(*core.Tx) error)

			// Create a mock transaction
			mockTx := &core.Tx{}

			// Execute the transaction function with the mock transaction
			_ = txFunc(mockTx)
		}).
		Return(expectedErr)

	// Create transaction wrapper
	tx := NewTransaction(mockDB)

	// Execute transaction
	err := tx.Execute(context.Background(), func(tx *Transaction) error {
		// Transaction logic here
		return nil
	})

	// Verify results
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	mockDB.AssertExpectations(t)
}

func TestTransaction_Operations(t *testing.T) {
	// Create mock DB and transaction
	mockDB := new(mocks.MockDB)

	// Create a mock transaction
	mockTx := &MockTx{}

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{
			PK: "user#123",
			SK: "user#123",
		},
		Name: "John Doe",
	}

	// No need to set expectations on MockTx since it's not a testify mock

	// Create transaction wrapper
	tx := &Transaction{
		tx:     mockTx,
		client: mockDB,
	}

	// Test Put
	err := tx.Put(user)
	assert.NoError(t, err)

	// Test Delete
	err = tx.Delete(user)
	assert.NoError(t, err)

	// Test Update
	err = tx.Update(user)
	assert.NoError(t, err)

	// Test ConditionCheck
	key := map[string]any{
		"PK": "user#123",
		"SK": "user#123",
	}
	err = tx.ConditionCheck("users", key, "attribute_exists(PK)")
	assert.NoError(t, err)

	// No need to verify expectations since MockTx is not a testify mock
}

func TestTransaction_OperationsWithoutTransaction(t *testing.T) {
	// Create transaction wrapper without a transaction
	tx := &Transaction{
		client: new(mocks.MockDB),
	}

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{
			PK: "user#123",
			SK: "user#123",
		},
		Name: "John Doe",
	}

	// Test Put
	err := tx.Put(user)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction not started")

	// Test Delete
	err = tx.Delete(user)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction not started")

	// Test Update
	err = tx.Update(user)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction not started")

	// Test ConditionCheck
	key := map[string]any{
		"PK": "user#123",
		"SK": "user#123",
	}
	err = tx.ConditionCheck("users", key, "attribute_exists(PK)")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction not started")
}

// Tests for the new TransactionManager

func TestTransactionManager_ExecuteWrite_Success(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	// Setup successful transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Run(func(args mock.Arguments) {
			txFunc := args.Get(0).(func(*core.Tx) error)
			assert.NoError(t, txFunc(&core.Tx{}))
		}).
		Return(nil)

	// Create transaction manager
	manager := NewTransactionManager(mockDB, logger)

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{
			PK: "user#123",
			SK: "user#123",
		},
		Name: "John Doe",
	}

	// Create operations
	operations := []TransactionOperation{
		{Type: OperationPut, Item: user},
		{Type: OperationUpdate, Item: user, UpdateExpression: "SET #name = :name", Values: []any{"John"}},
		{Type: OperationDelete, TableName: "users", Key: map[string]any{"PK": "user#123", "SK": "user#123"}},
		{Type: OperationConditionCheck, TableName: "users", Key: map[string]any{"PK": "user#123"}, Condition: "attribute_exists(PK)"},
	}

	// Execute transaction
	err := manager.ExecuteWrite(context.Background(), operations...)

	// Verify results
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestTransactionManager_ExecuteWrite_WithRetry(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	// Setup transaction to fail first, then succeed
	retryableErr := errors.New("transaction conflict")
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Return(retryableErr).Once()
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Run(func(args mock.Arguments) {
			txFunc := args.Get(0).(func(*core.Tx) error)
			assert.NoError(t, txFunc(&core.Tx{}))
		}).
		Return(nil).Once()

	// Create transaction manager
	manager := NewTransactionManager(mockDB, logger)

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{
			PK: "user#123",
			SK: "user#123",
		},
		Name: "John Doe",
	}

	// Create operations
	operations := []TransactionOperation{
		{Type: OperationPut, Item: user},
	}

	// Execute transaction
	err := manager.ExecuteWrite(context.Background(), operations...)

	// Verify results
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestTransactionManager_ExecuteWrite_NonRetryableError(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	// Setup transaction to fail with non-retryable error
	nonRetryableErr := errors.New("validation failed")
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Return(nonRetryableErr).Once()

	// Create transaction manager
	manager := NewTransactionManager(mockDB, logger)

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{
			PK: "user#123",
			SK: "user#123",
		},
		Name: "John Doe",
	}

	// Create operations
	operations := []TransactionOperation{
		{Type: OperationPut, Item: user},
	}

	// Execute transaction
	err := manager.ExecuteWrite(context.Background(), operations...)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-retryable")
	mockDB.AssertExpectations(t)
}

func TestTransactionManager_ExecuteWrite_MaxRetriesExceeded(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	// Setup transaction to always fail with retryable error
	retryableErr := errors.New("throttling")
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Return(retryableErr)

	// Create transaction manager
	manager := NewTransactionManager(mockDB, logger)

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{
			PK: "user#123",
			SK: "user#123",
		},
		Name: "John Doe",
	}

	// Create operations
	operations := []TransactionOperation{
		{Type: OperationPut, Item: user},
	}

	// Execute transaction with custom config for faster test
	config := TransactionConfig{
		MaxRetries:    2,
		BaseDelay:     1 * time.Millisecond,
		MaxDelay:      10 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	err := manager.ExecuteWriteWithConfig(context.Background(), config, operations...)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after")
	// Should have been called 3 times (initial + 2 retries)
	mockDB.AssertNumberOfCalls(t, "Transaction", 3)
}

func TestTransactionManager_ExecuteWrite_WithCostTracking(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	tracker := cost.New()

	// Setup successful transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Run(func(args mock.Arguments) {
			txFunc := args.Get(0).(func(*core.Tx) error)
			assert.NoError(t, txFunc(&core.Tx{}))
		}).
		Return(nil)

	// Create transaction manager with tracker
	manager := NewTransactionManagerWithTracker(mockDB, logger, tracker)

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{
			PK: "user#123",
			SK: "user#123",
		},
		Name: "John Doe",
	}

	// Create operations
	operations := []TransactionOperation{
		{Type: OperationPut, Item: user},
		{Type: OperationUpdate, Item: user},
	}

	// Execute transaction
	err := manager.ExecuteWrite(context.Background(), operations...)

	// Verify results
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)

	// Verify cost tracking (2 operations * 2 = 4 write units for transactions)
	costs := tracker.CalculateCost()
	assert.Equal(t, int64(4), costs.DynamoDBWrites)
}

func TestTransactionManager_ValidateOperations(t *testing.T) {
	manager := NewTransactionManager(nil, nil)

	tests := []struct {
		name        string
		operations  []TransactionOperation
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty operations",
			operations:  []TransactionOperation{},
			expectError: false,
		},
		{
			name: "valid put operation",
			operations: []TransactionOperation{
				{Type: OperationPut, Item: &TestUser{}},
			},
			expectError: false,
		},
		{
			name: "invalid put operation - no item",
			operations: []TransactionOperation{
				{Type: OperationPut, Item: nil},
			},
			expectError: true,
			errorMsg:    "put operation requires item",
		},
		{
			name: "valid condition check",
			operations: []TransactionOperation{
				{
					Type:      OperationConditionCheck,
					TableName: "users",
					Key:       map[string]any{"PK": "user#123"},
					Condition: "attribute_exists(PK)",
				},
			},
			expectError: false,
		},
		{
			name: "invalid condition check - missing condition",
			operations: []TransactionOperation{
				{
					Type:      OperationConditionCheck,
					TableName: "users",
					Key:       map[string]any{"PK": "user#123"},
				},
			},
			expectError: true,
			errorMsg:    "condition check requires key, tableName, and condition",
		},
		{
			name: "too many operations",
			operations: func() []TransactionOperation {
				ops := make([]TransactionOperation, 101)
				for i := range ops {
					ops[i] = TransactionOperation{Type: OperationPut, Item: &TestUser{}}
				}
				return ops
			}(),
			expectError: true,
			errorMsg:    "too many operations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.validateOperations(tt.operations)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTransactionManager_IsRetryableError(t *testing.T) {
	manager := NewTransactionManager(nil, nil)

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
		{
			name:      "transaction conflict",
			err:       errors.New("transaction conflict occurred"),
			retryable: true,
		},
		{
			name:      "throttling error",
			err:       errors.New("request throttling detected"),
			retryable: true,
		},
		{
			name:      "validation error",
			err:       errors.New("validation failed"),
			retryable: false,
		},
		{
			name:      "service unavailable",
			err:       errors.New("service unavailable"),
			retryable: true,
		},
		{
			name:      "unknown error",
			err:       errors.New("unknown error"),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isRetryableError(tt.err)
			assert.Equal(t, tt.retryable, result)
		})
	}
}

func TestTransactionManager_CalculateBackoffDelay(t *testing.T) {
	manager := NewTransactionManager(nil, nil)
	config := DefaultTransactionConfig()

	// Test first attempt (attempt 0)
	delay0 := manager.calculateBackoffDelay(0, config)
	assert.True(t, delay0 >= config.BaseDelay*9/10) // Allow for jitter
	assert.True(t, delay0 <= config.BaseDelay*11/10)

	// Test second attempt (attempt 1)
	delay1 := manager.calculateBackoffDelay(1, config)
	expectedDelay1 := time.Duration(float64(config.BaseDelay) * config.BackoffFactor)
	assert.True(t, delay1 >= expectedDelay1*9/10) // Allow for jitter
	assert.True(t, delay1 <= expectedDelay1*11/10)

	// Test max delay cap
	config.MaxDelay = 50 * time.Millisecond
	delayMax := manager.calculateBackoffDelay(10, config) // Large attempt number
	assert.True(t, delayMax <= config.MaxDelay*11/10)     // Allow for jitter
}

// Tests for TransactionBuilder

func TestTransactionBuilder_FluentInterface(t *testing.T) {
	user1 := &TestUser{
		StandardModel: StandardModel{PK: "user#1", SK: "user#1"},
		Name:          "User 1",
	}
	user2 := &TestUser{
		StandardModel: StandardModel{PK: "user#2", SK: "user#2"},
		Name:          "User 2",
	}

	builder := NewTransactionBuilder().
		WithMaxRetries(5).
		WithBaseDelay(50*time.Millisecond).
		Put(user1).
		Update(user2).
		ConditionCheck("users", map[string]any{"PK": "user#3"}, "attribute_not_exists(PK)")

	operations, config := builder.Build()

	// Verify operations
	assert.Len(t, operations, 3)
	assert.Equal(t, OperationPut, operations[0].Type)
	assert.Equal(t, user1, operations[0].Item)
	assert.Equal(t, OperationUpdate, operations[1].Type)
	assert.Equal(t, user2, operations[1].Item)
	assert.Equal(t, OperationConditionCheck, operations[2].Type)

	// Verify config
	assert.Equal(t, 5, config.MaxRetries)
	assert.Equal(t, 50*time.Millisecond, config.BaseDelay)
}

func TestTransactionBuilder_Execute(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	// Setup successful transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).Return(nil)

	// Create transaction manager
	manager := NewTransactionManager(mockDB, logger)

	// Create builder with operations
	user := &TestUser{
		StandardModel: StandardModel{PK: "user#123", SK: "user#123"},
		Name:          "John Doe",
	}

	builder := NewTransactionBuilder().Put(user)

	// Execute using builder
	err := builder.Execute(context.Background(), manager)

	// Verify results
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestTransactionBuilder_Clear(t *testing.T) {
	user := &TestUser{
		StandardModel: StandardModel{PK: "user#123", SK: "user#123"},
		Name:          "John Doe",
	}

	builder := NewTransactionBuilder().
		Put(user).
		Update(user)

	// Verify operations exist
	assert.Equal(t, 2, builder.GetOperationCount())

	// Clear operations
	builder.Clear()

	// Verify operations cleared
	assert.Equal(t, 0, builder.GetOperationCount())
}

// Tests for convenience functions

func TestExecuteSimpleTransaction(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)

	// Setup successful transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).Return(nil)

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{PK: "user#123", SK: "user#123"},
		Name:          "John Doe",
	}

	// Create operations
	operations := []TransactionOperation{
		{Type: OperationPut, Item: user},
	}

	// Execute simple transaction
	err := ExecuteSimpleTransaction(context.Background(), mockDB, operations...)

	// Verify results
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestExecuteTransactionWithCostTracking(t *testing.T) {
	// Create mock DB
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	tracker := cost.New()

	// Setup successful transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).Return(nil)

	// Test user
	user := &TestUser{
		StandardModel: StandardModel{PK: "user#123", SK: "user#123"},
		Name:          "John Doe",
	}

	// Create operations
	operations := []TransactionOperation{
		{Type: OperationPut, Item: user},
	}

	// Execute transaction with cost tracking
	err := ExecuteTransactionWithCostTracking(context.Background(), mockDB, logger, tracker, operations...)

	// Verify results
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)

	// Verify cost tracking
	costs := tracker.CalculateCost()
	assert.Equal(t, int64(2), costs.DynamoDBWrites) // 1 operation * 2 for transaction
}
