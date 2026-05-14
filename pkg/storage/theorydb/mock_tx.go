package theorydb

import (
	"fmt"
	"strings"

	"github.com/theory-cloud/tabletheory/pkg/core"
	errors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// MockTx is a mock implementation of the core.Tx interface for testing
type MockTx struct {
	core.Tx
	Builder core.TransactionBuilder
}

// Put adds a Put operation to the transaction
func (m *MockTx) Put(item any) error {
	if m.Builder != nil {
		m.Builder.Put(item)
		return nil
	}
	return runTxOperation(func() error {
		return m.Create(item)
	})
}

// Delete adds a Delete operation to the transaction
func (m *MockTx) Delete(item any) error {
	if m.Builder != nil {
		m.Builder.Delete(item)
		return nil
	}
	return runTxOperation(func() error {
		return m.Tx.Delete(item)
	})
}

// Update adds an Update operation to the transaction
func (m *MockTx) Update(item any) error {
	fields := inferTransactionUpdateFields(item)
	if len(fields) == 0 {
		return fmt.Errorf("transaction update requires fields")
	}
	if m.Builder != nil {
		m.Builder.Update(item, fields)
		return nil
	}
	return runTxOperation(func() error {
		return m.Tx.Update(item, fields...)
	})
}

// UpdateWithExpression adds an Update operation with expression to the transaction
func (m *MockTx) UpdateWithExpression(item any, _ string, _ ...any) error {
	if m.Builder != nil || len(inferTransactionUpdateFields(item)) > 0 {
		return m.Update(item)
	}
	return runTxOperation(func() error {
		return m.Tx.Model(item).Update()
	})
}

// DeleteByKey adds a Delete operation by key to the transaction
func (m *MockTx) DeleteByKey(tableName string, key map[string]any) error {
	item, err := newTransactionKeyItem(tableName, key)
	if err != nil {
		return err
	}
	return m.Delete(item)
}

// ConditionCheck adds a condition check to the transaction
func (m *MockTx) ConditionCheck(tableName string, key map[string]any, condition string, values ...any) error {
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
	if m.Builder != nil {
		m.Builder.ConditionCheck(item, conditions...)
		return nil
	}
	return m.conditionCheckWithLegacyTx(item, condition)
}

func (m *MockTx) conditionCheckWithLegacyTx(item any, condition string) error {
	normalized := strings.ToLower(strings.TrimSpace(condition))
	return runTxOperation(func() error {
		err := m.Tx.Model(item).First(item)
		if strings.Contains(normalized, "attribute_not_exists") {
			if err == nil {
				return fmt.Errorf("transaction condition check failed")
			}
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return err
	})
}

func runTxOperation(fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("transaction operation failed: %v", recovered)
		}
	}()
	return fn()
}
