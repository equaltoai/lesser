package dynamorm

import (
	"context"
	"errors"
	"testing"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestUser is a test model
type TestUser struct {
	StandardModel
	Name  string `json:"name"`
	Email string `json:"email"`
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
			txFunc(mockTx)
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
			txFunc(mockTx)
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
		Name:  "John Doe",
		Email: "john@example.com",
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
	key := map[string]interface{}{
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
		Name:  "John Doe",
		Email: "john@example.com",
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
	key := map[string]interface{}{
		"PK": "user#123",
		"SK": "user#123",
	}
	err = tx.ConditionCheck("users", key, "attribute_exists(PK)")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction not started")
}

// Skip the global function tests since we can't easily mock them
func TestExecuteTransaction_Basic(t *testing.T) {
	// Just verify the function exists and has the right signature
	// We can't easily test the implementation without being able to mock GetClient
	var _ func(context.Context, TransactionFunc) error = ExecuteTransaction
}

// Skip the global function tests since we can't easily mock them
func TestExecuteLambdaTransaction_Basic(t *testing.T) {
	// Just verify the function exists and has the right signature
	// We can't easily test the implementation without being able to mock GetLambdaClient
	var _ func(context.Context, TransactionFunc) error = ExecuteLambdaTransaction
}
