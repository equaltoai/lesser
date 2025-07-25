package dynamorm

import (
	"github.com/pay-theory/dynamorm/pkg/core"
)

// MockTx is a mock implementation of the core.Tx interface for testing
type MockTx struct {
	core.Tx
}

// Put adds a Put operation to the transaction
func (m *MockTx) Put(item any) error {
	return nil
}

// Delete adds a Delete operation to the transaction
func (m *MockTx) Delete(item any) error {
	return nil
}

// Update adds an Update operation to the transaction
func (m *MockTx) Update(item any) error {
	return nil
}

// UpdateWithExpression adds an Update operation with expression to the transaction
func (m *MockTx) UpdateWithExpression(item any, expr string, values ...any) error {
	return nil
}

// DeleteByKey adds a Delete operation by key to the transaction
func (m *MockTx) DeleteByKey(tableName string, key map[string]any) error {
	return nil
}

// ConditionCheck adds a condition check to the transaction
func (m *MockTx) ConditionCheck(tableName string, key map[string]any, condition string, values ...any) error {
	return nil
}
