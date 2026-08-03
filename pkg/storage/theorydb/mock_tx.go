package theorydb

import (
	"fmt"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// MockTx adapts Lesser's transaction helper surface to the TableTheory v3
// transaction builder. TableTheory removed the legacy core.Tx callback API;
// this shim exists only inside Lesser's storage helpers and does not recreate
// or fork TableTheory's removed transaction implementation.
type MockTx struct {
	Builder core.TransactionBuilder
}

// Put adds a Put operation to the transaction.
func (m *MockTx) Put(item any) error {
	if m == nil || m.Builder == nil {
		return fmt.Errorf("transaction builder not initialized")
	}
	m.Builder.Put(item)
	return nil
}

// Create adds a Create operation to the transaction.
func (m *MockTx) Create(item any) error {
	if m == nil || m.Builder == nil {
		return fmt.Errorf("transaction builder not initialized")
	}
	m.Builder.Create(item)
	return nil
}

// Delete adds a Delete operation to the transaction.
func (m *MockTx) Delete(item any) error {
	if m == nil || m.Builder == nil {
		return fmt.Errorf("transaction builder not initialized")
	}
	m.Builder.Delete(item)
	return nil
}

// Update adds an Update operation to the transaction.
func (m *MockTx) Update(item any) error {
	if m == nil || m.Builder == nil {
		return fmt.Errorf("transaction builder not initialized")
	}
	fields := inferTransactionUpdateFields(item)
	if len(fields) == 0 {
		return fmt.Errorf("transaction update requires fields")
	}
	m.Builder.Update(item, fields)
	return nil
}

// UpdateWithExpression adds an Update operation with expression to the transaction.
// Lesser's legacy helper accepted a raw expression; TableTheory transactions
// use typed update builders, so this falls back to the field-inference update path.
func (m *MockTx) UpdateWithExpression(item any, _ string, _ ...any) error {
	return m.Update(item)
}

// DeleteByKey adds a Delete operation by key to the transaction.
func (m *MockTx) DeleteByKey(tableName string, key map[string]any) error {
	item, err := newTransactionKeyItem(tableName, key)
	if err != nil {
		return err
	}
	return m.Delete(item)
}

// ConditionCheck adds a condition check to the transaction.
func (m *MockTx) ConditionCheck(tableName string, key map[string]any, condition string, values ...any) error {
	if m == nil || m.Builder == nil {
		return fmt.Errorf("transaction builder not initialized")
	}
	item, err := newTransactionKeyItem(tableName, key)
	if err != nil {
		return err
	}
	conditions := transactionConditions(TransactionOperation{
		Condition: condition,
		Values:    values,
	})
	if len(conditions) == 0 {
		return fmt.Errorf("condition check requires condition")
	}
	m.Builder.ConditionCheck(item, conditions...)
	return nil
}
