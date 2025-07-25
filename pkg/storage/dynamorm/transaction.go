package dynamorm

import (
	"context"
	"fmt"

	"github.com/pay-theory/dynamorm/pkg/core"
)

// TransactionFunc is a function that executes within a transaction
type TransactionFunc func(tx *Transaction) error

// TxOperations defines the operations that can be performed in a transaction
type TxOperations interface {
	Put(item any) error
	Delete(item any) error
	Update(item any) error
	ConditionCheck(tableName string, key map[string]any, condition string, values ...any) error
}

// Transaction represents a DynamoDB transaction
type Transaction struct {
	tx     TxOperations
	client core.DB
}

// NewTransaction creates a new transaction wrapper
func NewTransaction(client core.DB) *Transaction {
	return &Transaction{
		client: client,
	}
}

// Execute runs the provided function within a transaction
// If the function returns an error, the transaction is aborted
// Otherwise, the transaction is committed
func (t *Transaction) Execute(ctx context.Context, fn TransactionFunc) error {
	// Create a new transaction
	err := t.client.Transaction(func(tx *core.Tx) error {
		// Create a transaction wrapper with a mock tx that implements TxOperations
		txWrapper := &Transaction{
			tx:     &MockTx{Tx: *tx},
			client: t.client,
		}

		// Execute the transaction function
		return fn(txWrapper)
	})

	return err
}

// Put adds a Put operation to the transaction
func (t *Transaction) Put(item any) error {
	if t.tx == nil {
		return fmt.Errorf("transaction not started")
	}

	return t.tx.Put(item)
}

// Delete adds a Delete operation to the transaction
func (t *Transaction) Delete(item any) error {
	if t.tx == nil {
		return fmt.Errorf("transaction not started")
	}

	return t.tx.Delete(item)
}

// Update adds an Update operation to the transaction
func (t *Transaction) Update(item any) error {
	if t.tx == nil {
		return fmt.Errorf("transaction not started")
	}

	return t.tx.Update(item)
}

// ConditionCheck adds a condition check to the transaction
// The transaction will fail if the condition is not met
func (t *Transaction) ConditionCheck(tableName string, key map[string]any, condition string, values ...any) error {
	if t.tx == nil {
		return fmt.Errorf("transaction not started")
	}

	return t.tx.ConditionCheck(tableName, key, condition, values...)
}

// ExecuteTransaction is a helper function to execute a transaction with the default client
func ExecuteTransaction(ctx context.Context, fn TransactionFunc) error {
	client, err := GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get DynamoDB client: %w", err)
	}

	tx := NewTransaction(client)
	return tx.Execute(ctx, fn)
}

// ExecuteLambdaTransaction is a helper function to execute a transaction with the Lambda-optimized client
func ExecuteLambdaTransaction(ctx context.Context, fn TransactionFunc) error {
	client, err := GetLambdaClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Lambda DynamoDB client: %w", err)
	}

	tx := NewTransaction(client)
	return tx.Execute(ctx, fn)
}
