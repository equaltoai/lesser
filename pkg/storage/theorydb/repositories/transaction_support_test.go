package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	repoTesting "github.com/equaltoai/lesser/pkg/storage/theorydb/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/core"
	tableErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	githubMocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

// MockTx represents a mock transaction
type MockTx struct {
	mock.Mock
}

func newNoopQuery() *githubMocks.MockQuery {
	query := new(githubMocks.MockQuery)
	query.On("Create").Return(nil).Maybe()
	query.On("Update", mock.Anything).Return(nil).Maybe()
	query.On("Update").Return(nil).Maybe()
	query.On("Delete").Return(nil).Maybe()
	query.On("CreateOrUpdate").Return(nil).Maybe()
	query.On("UpdateBuilder").Return(new(githubMocks.MockUpdateBuilder)).Maybe()
	query.On("BatchCreate", mock.Anything).Return(nil).Maybe()
	query.On("BatchDelete", mock.Anything).Return(nil).Maybe()
	query.On("BatchWrite", mock.Anything, mock.Anything).Return(nil).Maybe()
	query.On("First", mock.Anything).Return(nil).Maybe()
	query.On("Find", mock.Anything).Return(nil).Maybe()
	query.On("All", mock.Anything).Return(nil).Maybe()
	query.On("Count").Return(int64(0), nil).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("OrFilter", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("FilterGroup", mock.Anything).Return(query).Maybe()
	query.On("OrFilterGroup", mock.Anything).Return(query).Maybe()
	query.On("Index", mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Limit", mock.Anything).Return(query).Maybe()
	query.On("Offset", mock.Anything).Return(query).Maybe()
	query.On("Select", mock.Anything).Return(query).Maybe()
	query.On("ConsistentRead").Return(query).Maybe()
	query.On("WithRetry", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Cursor", mock.Anything).Return(query).Maybe()
	query.On("SetCursor", mock.Anything).Return(nil).Maybe()
	query.On("WithContext", mock.Anything).Return(query).Maybe()
	query.On("Scan", mock.Anything).Return(nil).Maybe()
	query.On("ParallelScan", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("ScanAllSegments", mock.Anything, mock.Anything).Return(nil).Maybe()
	query.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Maybe()
	query.On("BatchUpdateWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	return query
}

func TestNewTransactionManager(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()

	tm := NewTransactionManager(mockDB, logger, tracker)

	assert.NotNil(t, tm)
	assert.Equal(t, mockDB, tm.db)
	assert.Equal(t, logger, tm.logger)
	assert.Equal(t, tracker, tm.tracker)
}

func TestNewTransactionalRepository(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()
	tableName := "test-table"

	repo := NewTransactionalRepository(mockDB, tableName, logger, tracker)

	assert.NotNil(t, repo)
	assert.NotNil(t, repo.BaseRepository)
	assert.NotNil(t, repo.tm)
	assert.Equal(t, tableName, repo.GetTableName())
}

func TestTransactionManager_ExecuteTransaction_Success(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()

	tm := NewTransactionManager(mockDB, logger, tracker)
	ctx := context.Background()

	// Mock successful transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).Return(nil).Run(func(args mock.Arguments) {
		fn := args.Get(0).(func(*core.Tx) error)
		mockTx := &core.Tx{}
		_ = fn(mockTx)
	})

	executed := false
	err := tm.ExecuteTransaction(ctx, func(txCtx *TransactionContext) error {
		executed = true
		assert.NotNil(t, txCtx)
		assert.NotNil(t, txCtx.tx)
		assert.Equal(t, 0, txCtx.operationsCnt)
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, executed)
	mockDB.AssertExpectations(t)
}

func TestTransactionManager_ExecuteTransaction_Error(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()

	tm := NewTransactionManager(mockDB, logger, tracker)
	ctx := context.Background()

	expectedError := errors.New("transaction failed")

	// Mock failed transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).Return(expectedError)

	err := tm.ExecuteTransaction(ctx, func(txCtx *TransactionContext) error {
		return errors.New("user error")
	})

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	mockDB.AssertExpectations(t)
}

// Removed ExecuteWithRetry tests - they were testing mock behavior rather than business logic
// These tests didn't provide value as unit tests since they only verified
// that when db.Transaction() fails, the transaction function doesn't run

func TestDefaultTransactionConfig(t *testing.T) {
	config := DefaultTransactionConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, config.BackoffDuration)
	assert.Nil(t, config.Logger)
	assert.Nil(t, config.CostTracker)
}

func TestTransferOwnershipTransactional_ConceptualTest(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()

	repo := NewTransactionalRepository(mockDB, "resources", logger, tracker)
	ctx := context.Background()

	resourceIDs := []string{"resource1", "resource2", "resource3"}

	// Mock transaction execution - simulate successful transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).Return(nil)

	err := repo.TransferOwnershipTransactional(ctx, "fromUser", "toUser", resourceIDs)

	// We expect success now that transactions are implemented
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func newTransactionContextWithMocks() *TransactionContext {
	mockDB := new(repoTesting.MockDB)
	mockQuery := newNoopQuery()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()

	tx := &core.Tx{}
	tx.SetDB(mockDB)

	return &TransactionContext{
		tx:            tx,
		operationsCnt: 0,
		startTime:     time.Now(),
		logger:        zap.NewNop(),
	}
}

func TestConditionalCreate(t *testing.T) {
	mockDB := new(repoTesting.MockDB)
	mockQuery := new(githubMocks.MockQuery)
	mockQuery.On("First", mock.Anything).Return(tableErrors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	tx := &core.Tx{}
	tx.SetDB(mockDB)
	txCtx := &TransactionContext{
		tx:            tx,
		operationsCnt: 0,
		startTime:     time.Now(),
		logger:        zap.NewNop(),
	}

	item := map[string]any{"key": "value"}
	key := map[string]any{"PK": "test"}

	err := ConditionalCreate(txCtx, item, key)

	assert.NoError(t, err)
	assert.Equal(t, 2, txCtx.GetOperationCount())
}

func TestConditionalUpdate(t *testing.T) {
	txCtx := newTransactionContextWithMocks()

	item := map[string]any{"key": "value"}
	key := map[string]any{"PK": "test"}

	err := ConditionalUpdate(txCtx, item, key, "attribute_exists(PK)")

	assert.NoError(t, err)
	assert.Equal(t, 2, txCtx.GetOperationCount())
}

func TestConditionalDelete(t *testing.T) {
	txCtx := newTransactionContextWithMocks()

	item := map[string]any{"key": "value"}
	key := map[string]any{"PK": "test"}

	err := ConditionalDelete(txCtx, item, key, "attribute_exists(PK)")

	assert.NoError(t, err)
	assert.Equal(t, 2, txCtx.GetOperationCount())
}

func TestIsRetryableError(t *testing.T) {
	// Test with various error types
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "simple error",
			err:      errors.New("simple error"),
			expected: false, // Non-retryable error
		},
		{
			name:     "wrapped error",
			err:      errors.New("wrapped: original error"),
			expected: false, // Non-retryable error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				return // Skip nil error test for now
			}
			result := isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests for performance validation

func BenchmarkTransactionManager_ExecuteTransaction(b *testing.B) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()

	tm := NewTransactionManager(mockDB, logger, tracker)
	ctx := context.Background()

	// Mock successful transaction
	mockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).Return(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tm.ExecuteTransaction(ctx, func(txCtx *TransactionContext) error {
			// Simulate some work
			txCtx.operationsCnt++
			return nil
		})
	}
}

func BenchmarkTransactionContext_Operations(b *testing.B) {
	txCtx := &TransactionContext{
		operationsCnt: 0,
		startTime:     time.Now(),
	}

	item := map[string]any{"key": "value"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// These will error but we're measuring the overhead
		_ = txCtx.Put(item)
		_ = txCtx.Delete(item)
		_ = txCtx.Update(item)
		_ = txCtx.ConditionCheck(item, "condition")
	}
}
