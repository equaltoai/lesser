package theorydb

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// TransactionManager provides enhanced transaction management with retry logic and cost tracking
type TransactionManager struct {
	client  core.DB
	logger  *zap.Logger
	tracker *cost.Tracker
}

// TransactionOperation represents a single operation within a transaction
type TransactionOperation struct {
	Type             OperationType
	Item             any
	Key              map[string]any
	Fields           []string
	UpdateExpression string
	Condition        string
	Conditions       []core.TransactCondition
	Values           []any
	TableName        string
}

// OperationType defines the type of transaction operation
type OperationType int

const (
	// OperationPut represents a put operation
	OperationPut OperationType = iota
	// OperationUpdate represents an update operation
	OperationUpdate
	// OperationDelete represents a delete operation
	OperationDelete
	// OperationConditionCheck represents a condition check operation
	OperationConditionCheck
)

// String returns the string representation of the operation type
func (ot OperationType) String() string {
	switch ot {
	case OperationPut:
		return "put"
	case OperationUpdate:
		return "update"
	case OperationDelete:
		return "delete"
	case OperationConditionCheck:
		return "condition_check"
	default:
		return "unknown"
	}
}

// TransactionConfig holds configuration for transaction execution
type TransactionConfig struct {
	MaxRetries         int
	BaseDelay          time.Duration
	MaxDelay           time.Duration
	BackoffFactor      float64
	EnableCostTracking bool
}

// DefaultTransactionConfig returns the default transaction configuration
func DefaultTransactionConfig() TransactionConfig {
	return TransactionConfig{
		MaxRetries:         3,
		BaseDelay:          100 * time.Millisecond,
		MaxDelay:           5 * time.Second,
		BackoffFactor:      2.0,
		EnableCostTracking: true,
	}
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(client core.DB, logger *zap.Logger) *TransactionManager {
	return &TransactionManager{
		client: client,
		logger: logger,
	}
}

// NewTransactionManagerWithTracker creates a new transaction manager with cost tracking
func NewTransactionManagerWithTracker(client core.DB, logger *zap.Logger, tracker *cost.Tracker) *TransactionManager {
	return &TransactionManager{
		client:  client,
		logger:  logger,
		tracker: tracker,
	}
}

// ExecuteWrite executes a transaction with the provided operations
func (tm *TransactionManager) ExecuteWrite(ctx context.Context, operations ...TransactionOperation) error {
	config := DefaultTransactionConfig()
	return tm.ExecuteWriteWithConfig(ctx, config, operations...)
}

// ExecuteWriteWithConfig executes a transaction with custom configuration
func (tm *TransactionManager) ExecuteWriteWithConfig(ctx context.Context, config TransactionConfig, operations ...TransactionOperation) error {
	if err := common.ValidateSliceNotEmpty("operations", operations); err != nil {
		return fmt.Errorf("no operations provided for transaction")
	}

	// Validate operations
	if err := tm.validateOperations(operations); err != nil {
		return fmt.Errorf("invalid operations: %w", err)
	}

	startTime := time.Now()
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Track cost if enabled
		if config.EnableCostTracking && tm.tracker != nil {
			defer func() {
				// Transactions consume 2x write capacity units
				writeUnits := len(operations) * 2
				if err := tm.tracker.TrackDynamoWrite(writeUnits); err != nil {
					zap.L().Warn("failed to track transaction write cost", zap.Error(err))
				}
			}()
		}

		// Execute the transaction
		err := tm.executeTransaction(ctx, operations)
		if err == nil {
			// Success - log and return
			if tm.logger != nil {
				tm.logger.Debug("transaction_completed",
					zap.Int("operation_count", len(operations)),
					zap.Int("attempt", attempt+1),
					zap.Duration("duration", time.Since(startTime)),
				)
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !tm.isRetryableError(err) {
			if tm.logger != nil {
				tm.logger.Error("transaction_failed_non_retryable",
					zap.Error(err),
					zap.Int("operation_count", len(operations)),
					zap.Int("attempt", attempt+1),
				)
			}
			return fmt.Errorf("transaction failed (non-retryable): %w", err)
		}

		// Don't sleep after the last attempt
		if attempt < config.MaxRetries {
			delay := tm.calculateBackoffDelay(attempt, config)
			if tm.logger != nil {
				tm.logger.Warn("transaction_retry",
					zap.Error(err),
					zap.Int("attempt", attempt+1),
					zap.Int("max_retries", config.MaxRetries),
					zap.Duration("delay", delay),
				)
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("transaction canceled: %w", ctx.Err())
			case <-time.After(delay):
				// Continue to next attempt
			}
		}
	}

	if tm.logger != nil {
		tm.logger.Error("transaction_failed_max_retries",
			zap.Error(lastErr),
			zap.Int("operation_count", len(operations)),
			zap.Int("max_retries", config.MaxRetries),
			zap.Duration("total_duration", time.Since(startTime)),
		)
	}

	return fmt.Errorf("transaction failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// ExecuteWithRetry executes a transaction with default retry configuration
func (tm *TransactionManager) ExecuteWithRetry(ctx context.Context, operations ...TransactionOperation) error {
	return tm.ExecuteWrite(ctx, operations...)
}

// executeTransaction executes the actual transaction
func (tm *TransactionManager) executeTransaction(ctx context.Context, operations []TransactionOperation) error {
	if client, ok := tm.client.(interface {
		TransactWrite(context.Context, func(core.TransactionBuilder) error) error
	}); ok {
		return client.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
			for i, op := range operations {
				if err := tm.executeBuilderOperation(tx, op); err != nil {
					return fmt.Errorf("operation %d (%s) failed: %w", i, op.Type.String(), err)
				}
			}
			return nil
		})
	}

	return tm.client.Transaction(func(tx *core.Tx) error {
		for i, op := range operations {
			if err := tm.executeOperation(tx, op); err != nil {
				return fmt.Errorf("operation %d (%s) failed: %w", i, op.Type.String(), err)
			}
		}
		return nil
	})
}

func (tm *TransactionManager) executeBuilderOperation(tx core.TransactionBuilder, op TransactionOperation) error {
	conditions := transactionConditions(op)

	switch op.Type {
	case OperationPut:
		if op.Item == nil {
			return fmt.Errorf("put operation requires item")
		}
		tx.Put(op.Item, conditions...)
		return nil
	case OperationUpdate:
		if op.Item == nil {
			return fmt.Errorf("update operation requires item")
		}
		fields := op.Fields
		if len(fields) == 0 {
			fields = inferTransactionUpdateFields(op.Item)
		}
		if len(fields) == 0 {
			return fmt.Errorf("update operation requires fields")
		}
		tx.Update(op.Item, fields, conditions...)
		return nil
	case OperationDelete:
		if op.Item != nil {
			tx.Delete(op.Item, conditions...)
			return nil
		}
		keyItem, err := newTransactionKeyItem(op.TableName, op.Key)
		if err != nil {
			return err
		}
		tx.Delete(keyItem, conditions...)
		return nil
	case OperationConditionCheck:
		if op.Condition == "" && len(op.Conditions) == 0 {
			return fmt.Errorf("condition check requires condition")
		}
		item := op.Item
		if item == nil {
			keyItem, err := newTransactionKeyItem(op.TableName, op.Key)
			if err != nil {
				return err
			}
			item = keyItem
		}
		tx.ConditionCheck(item, conditions...)
		return nil
	default:
		return fmt.Errorf("unsupported operation type: %v", op.Type)
	}
}

// executeOperation executes a single operation within a transaction
func (tm *TransactionManager) executeOperation(tx *core.Tx, op TransactionOperation) error {
	// Create a wrapper that implements TxOperations
	txOps := &MockTx{Tx: *tx}

	switch op.Type {
	case OperationPut:
		return txOps.Put(op.Item)
	case OperationUpdate:
		if op.UpdateExpression != "" {
			// Use update expression if provided
			return txOps.UpdateWithExpression(op.Item, op.UpdateExpression, op.Values...)
		}
		return txOps.Update(op.Item)
	case OperationDelete:
		if op.Item != nil {
			return txOps.Delete(op.Item)
		}
		if op.Key != nil && op.TableName != "" {
			return txOps.DeleteByKey(op.TableName, op.Key)
		}
		return fmt.Errorf("delete operation requires either item or key+tableName")
	case OperationConditionCheck:
		if op.Key == nil || op.TableName == "" || op.Condition == "" {
			return fmt.Errorf("condition check requires key, tableName, and condition")
		}
		return txOps.ConditionCheck(op.TableName, op.Key, op.Condition, op.Values...)
	default:
		return fmt.Errorf("unsupported operation type: %v", op.Type)
	}
}

func transactionConditions(op TransactionOperation) []core.TransactCondition {
	conditions := append([]core.TransactCondition(nil), op.Conditions...)
	if strings.TrimSpace(op.Condition) != "" {
		conditions = append(conditions, core.TransactCondition{
			Kind:       core.TransactConditionKindExpression,
			Expression: op.Condition,
			Values:     transactionConditionValues(op.Values),
		})
	}
	return conditions
}

func transactionConditionValues(values []any) map[string]any {
	if len(values) == 0 {
		return nil
	}

	result := make(map[string]any, len(values))
	for i, value := range values {
		result[fmt.Sprintf(":v%d", i)] = value
	}
	return result
}

func inferTransactionUpdateFields(item any) []string {
	value := reflect.ValueOf(item)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return nil
	}

	itemType := value.Type()
	fields := make([]string, 0, itemType.NumField())
	for i := range itemType.NumField() {
		field := itemType.Field(i)
		if field.PkgPath != "" || field.Anonymous {
			continue
		}
		if skipTransactionUpdateField(field) {
			continue
		}
		if name := theoryDBAttrName(field); name != "" {
			fields = append(fields, name)
			continue
		}
		fields = append(fields, field.Name)
	}
	return fields
}

func skipTransactionUpdateField(field reflect.StructField) bool {
	if field.Name == "PK" || field.Name == "SK" || field.Name == "Version" || strings.HasPrefix(field.Name, "GSI") {
		return true
	}
	for _, part := range strings.Split(field.Tag.Get("theorydb"), ",") {
		switch {
		case part == "pk", part == "sk", part == "version", part == "ttl":
			return true
		case strings.HasPrefix(part, "index:"):
			return true
		}
	}
	return false
}

func theoryDBAttrName(field reflect.StructField) string {
	for _, part := range strings.Split(field.Tag.Get("theorydb"), ",") {
		if strings.HasPrefix(part, "attr:") {
			return strings.TrimPrefix(part, "attr:")
		}
	}
	return ""
}

// validateOperations validates the provided operations
func (tm *TransactionManager) validateOperations(operations []TransactionOperation) error {
	if len(operations) > 100 {
		return fmt.Errorf("too many operations: %d (max 100)", len(operations))
	}

	for i, op := range operations {
		if err := tm.validateOperation(op); err != nil {
			return fmt.Errorf("operation %d: %w", i, err)
		}
	}

	return nil
}

// validateOperation validates a single operation
func (tm *TransactionManager) validateOperation(op TransactionOperation) error {
	switch op.Type {
	case OperationPut:
		if op.Item == nil {
			return fmt.Errorf("put operation requires item")
		}
	case OperationUpdate:
		if op.Item == nil {
			return fmt.Errorf("update operation requires item")
		}
	case OperationDelete:
		if op.Item == nil && (op.Key == nil || op.TableName == "") {
			return fmt.Errorf("delete operation requires either item or key+tableName")
		}
	case OperationConditionCheck:
		if op.Key == nil || op.TableName == "" || op.Condition == "" {
			return fmt.Errorf("condition check requires key, tableName, and condition")
		}
	default:
		return fmt.Errorf("unsupported operation type: %v", op.Type)
	}
	return nil
}

// isRetryableError determines if an error is retryable
func (tm *TransactionManager) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific retryable errors
	if IsTransactionCanceled(err) {
		return true
	}

	if IsThrottling(err) {
		return true
	}

	// Check error message for retryable patterns
	errMsg := err.Error()
	retryablePatterns := []string{
		"transaction conflict",
		"throttling",
		"service unavailable",
		"internal server error",
		"timeout",
		"connection reset",
		"temporary failure",
	}

	for _, pattern := range retryablePatterns {
		if contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// calculateBackoffDelay calculates the delay for exponential backoff
func (tm *TransactionManager) calculateBackoffDelay(attempt int, config TransactionConfig) time.Duration {
	delay := float64(config.BaseDelay) * math.Pow(config.BackoffFactor, float64(attempt))

	// Add jitter to prevent thundering herd
	jitterFactor := float64(2*time.Now().UnixNano()%1000)/1000.0 - 1 // ±10% jitter
	jitter := delay * 0.1 * jitterFactor
	delay += jitter

	// Cap at max delay
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	return time.Duration(delay)
}

// TransactionBuilder provides a fluent interface for building transactions
type TransactionBuilder struct {
	operations []TransactionOperation
	config     TransactionConfig
}

// NewTransactionBuilder creates a new transaction builder
func NewTransactionBuilder() *TransactionBuilder {
	return &TransactionBuilder{
		operations: make([]TransactionOperation, 0),
		config:     DefaultTransactionConfig(),
	}
}

// WithConfig sets the transaction configuration
func (tb *TransactionBuilder) WithConfig(config TransactionConfig) *TransactionBuilder {
	tb.config = config
	return tb
}

// WithMaxRetries sets the maximum number of retries
func (tb *TransactionBuilder) WithMaxRetries(maxRetries int) *TransactionBuilder {
	tb.config.MaxRetries = maxRetries
	return tb
}

// WithBaseDelay sets the base delay for exponential backoff
func (tb *TransactionBuilder) WithBaseDelay(delay time.Duration) *TransactionBuilder {
	tb.config.BaseDelay = delay
	return tb
}

// Put adds a put operation to the transaction
func (tb *TransactionBuilder) Put(item any) *TransactionBuilder {
	tb.operations = append(tb.operations, TransactionOperation{
		Type: OperationPut,
		Item: item,
	})
	return tb
}

// Update adds an update operation to the transaction
func (tb *TransactionBuilder) Update(item any) *TransactionBuilder {
	tb.operations = append(tb.operations, TransactionOperation{
		Type: OperationUpdate,
		Item: item,
	})
	return tb
}

// UpdateWithExpression adds an update operation with expression to the transaction
func (tb *TransactionBuilder) UpdateWithExpression(item any, expr string, values ...any) *TransactionBuilder {
	tb.operations = append(tb.operations, TransactionOperation{
		Type:             OperationUpdate,
		Item:             item,
		UpdateExpression: expr,
		Values:           values,
	})
	return tb
}

// Delete adds a delete operation to the transaction
func (tb *TransactionBuilder) Delete(item any) *TransactionBuilder {
	tb.operations = append(tb.operations, TransactionOperation{
		Type: OperationDelete,
		Item: item,
	})
	return tb
}

// DeleteByKey adds a delete operation by key to the transaction
func (tb *TransactionBuilder) DeleteByKey(tableName string, key map[string]any) *TransactionBuilder {
	tb.operations = append(tb.operations, TransactionOperation{
		Type:      OperationDelete,
		TableName: tableName,
		Key:       key,
	})
	return tb
}

// ConditionCheck adds a condition check to the transaction
func (tb *TransactionBuilder) ConditionCheck(tableName string, key map[string]any, condition string, values ...any) *TransactionBuilder {
	tb.operations = append(tb.operations, TransactionOperation{
		Type:      OperationConditionCheck,
		TableName: tableName,
		Key:       key,
		Condition: condition,
		Values:    values,
	})
	return tb
}

// Build returns the operations and configuration
func (tb *TransactionBuilder) Build() ([]TransactionOperation, TransactionConfig) {
	return tb.operations, tb.config
}

// Execute executes the transaction using the provided manager
func (tb *TransactionBuilder) Execute(ctx context.Context, manager *TransactionManager) error {
	return manager.ExecuteWriteWithConfig(ctx, tb.config, tb.operations...)
}

// GetOperationCount returns the number of operations in the builder
func (tb *TransactionBuilder) GetOperationCount() int {
	return len(tb.operations)
}

// Clear clears all operations from the builder
func (tb *TransactionBuilder) Clear() *TransactionBuilder {
	tb.operations = tb.operations[:0]
	return tb
}

// Helper functions

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsSubstring(s, substr))))
}

// containsSubstring performs a simple substring search
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Convenience functions for common transaction patterns

// ExecuteSimpleTransaction executes a simple transaction with default settings
func ExecuteSimpleTransaction(ctx context.Context, client core.DB, operations ...TransactionOperation) error {
	manager := NewTransactionManager(client, nil)
	return manager.ExecuteWrite(ctx, operations...)
}

// ExecuteTransactionWithLogger executes a transaction with logging
func ExecuteTransactionWithLogger(ctx context.Context, client core.DB, logger *zap.Logger, operations ...TransactionOperation) error {
	manager := NewTransactionManager(client, logger)
	return manager.ExecuteWrite(ctx, operations...)
}

// ExecuteTransactionWithCostTracking executes a transaction with cost tracking
func ExecuteTransactionWithCostTracking(ctx context.Context, client core.DB, logger *zap.Logger, tracker *cost.Tracker, operations ...TransactionOperation) error {
	manager := NewTransactionManagerWithTracker(client, logger, tracker)
	return manager.ExecuteWrite(ctx, operations...)
}
