package theorydb

import (
	"context"
	"fmt"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
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
	client, ok := t.client.(interface {
		TransactWrite(context.Context, func(core.TransactionBuilder) error) error
	})
	if !ok {
		return fmt.Errorf("tabletheory transaction support requires core.ExtendedDB")
	}

	return client.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
		txWrapper := &Transaction{
			tx:     &MockTx{Builder: tx},
			client: t.client,
		}
		return fn(txWrapper)
	})
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
