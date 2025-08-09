package dynamorm

import (
	"github.com/pay-theory/dynamorm/pkg/core"
)

// MockTx is a mock implementation of the core.Tx interface for testing
type MockTx struct {
	core.Tx
}

// Put adds a Put operation to the transaction
func (m *MockTx) Put(_ any) error {
	return nil
}

// Delete adds a Delete operation to the transaction
func (m *MockTx) Delete(_ any) error {
	return nil
}

// Update adds an Update operation to the transaction
func (m *MockTx) Update(_ any) error {
	return nil
}

// UpdateWithExpression adds an Update operation with expression to the transaction
func (m *MockTx) UpdateWithExpression(_ any, _ string, _ ...any) error {
	return nil
}

// DeleteByKey adds a Delete operation by key to the transaction
func (m *MockTx) DeleteByKey(_ string, _ map[string]any) error {
	return nil
}

// ConditionCheck adds a condition check to the transaction
func (m *MockTx) ConditionCheck(_ string, _ map[string]any, _ string, _ ...any) error {
	return nil
}
