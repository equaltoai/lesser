package dynamorm

import (
	"github.com/pay-theory/dynamorm/pkg/core"
)

// MockTx is a mock implementation of the core.Tx interface for testing
type MockTx struct {
	core.Tx
}

// Put adds a Put operation to the transaction
func (m *MockTx) Put(item interface{}) error {
	return nil
}

// Delete adds a Delete operation to the transaction
func (m *MockTx) Delete(item interface{}) error {
	return nil
}

// Update adds an Update operation to the transaction
func (m *MockTx) Update(item interface{}) error {
	return nil
}

// ConditionCheck adds a condition check to the transaction
func (m *MockTx) ConditionCheck(tableName string, key map[string]interface{}, condition string, values ...interface{}) error {
	return nil
}
