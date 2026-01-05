package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type transactionRunnerDB struct {
	query         core.Query
	transactionFn func(fn func(*core.Tx) error) error
}

func (db *transactionRunnerDB) Model(_ any) core.Query {
	return db.query
}

func (db *transactionRunnerDB) Transaction(fn func(*core.Tx) error) error {
	if db.transactionFn != nil {
		return db.transactionFn(fn)
	}
	tx := &core.Tx{}
	tx.SetDB(db)
	return fn(tx)
}

func (db *transactionRunnerDB) Migrate() error { return nil }

func (db *transactionRunnerDB) AutoMigrate(_ ...any) error { return nil }

func (db *transactionRunnerDB) Close() error { return nil }

func (db *transactionRunnerDB) WithContext(_ context.Context) core.DB { return db }

func TestTransactionManager_BeginCommitRollbackTransaction_Round23(t *testing.T) {
	t.Parallel()

	tm := NewTransactionManager(&transactionRunnerDB{}, zap.NewNop(), cost.New())

	txCtx, err := tm.BeginTransaction(context.Background())
	require.NoError(t, err)
	require.NotNil(t, txCtx)
	require.Equal(t, 0, txCtx.GetOperationCount())

	require.Error(t, tm.CommitTransaction(context.Background(), nil))
	require.NoError(t, tm.CommitTransaction(context.Background(), txCtx))

	txCtx.operationsCnt = 2
	require.NoError(t, tm.CommitTransaction(context.Background(), txCtx))

	require.Error(t, tm.RollbackTransaction(context.Background(), nil))
	require.NoError(t, tm.RollbackTransaction(context.Background(), txCtx))
	require.Equal(t, 0, txCtx.GetOperationCount())
}

func TestTransactionContext_OperationsRequireTx_Round23(t *testing.T) {
	t.Parallel()

	txCtx := &TransactionContext{tx: nil}

	require.ErrorContains(t, txCtx.Put(map[string]any{"PK": "p"}), "transaction not initialized")
	require.ErrorContains(t, txCtx.Update(map[string]any{"PK": "p"}), "transaction not initialized")
	require.ErrorContains(t, txCtx.Delete(map[string]any{"PK": "p"}), "transaction not initialized")

	require.ErrorContains(t, txCtx.ConditionCheck(map[string]any{"PK": "p"}, "attribute_exists(PK)"), "transaction not initialized")
	require.ErrorContains(t, txCtx.UpdateWithExpression(map[string]any{}, "SET X = :x", 1), "transaction not initialized")
	require.ErrorContains(t, txCtx.DeleteByKey("table", map[string]any{"PK": "p"}), "transaction not initialized")

	txCtx.tx = &core.Tx{}
	require.ErrorContains(t, txCtx.ConditionCheck("not-a-map", "attribute_exists(PK)"), "condition check requires key")
	txCtx.tx = nil

	require.NoError(t, txCtx.TransactionalGet(nil))
	require.NoError(t, txCtx.TransactionalBatchGet([]any{"a", "b"}))
	require.ErrorContains(t, txCtx.TransactionalBatchWrite([]any{"a"}, nil), "batch put failed")
}

func TestTransactionContext_PutDeleteUpdate_FailurePaths_Round23(t *testing.T) {
	t.Parallel()

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(errors.New("create failed")).Once()
	q.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	q.On("Delete").Return(errors.New("delete failed")).Once()

	db := &transactionRunnerDB{query: q}
	tx := &core.Tx{}
	tx.SetDB(db)

	txCtx := &TransactionContext{tx: tx}

	require.ErrorContains(t, txCtx.Put(map[string]any{"PK": "p"}), "transaction put failed")
	require.ErrorContains(t, txCtx.Update(map[string]any{"PK": "p"}), "transaction update failed")
	require.ErrorContains(t, txCtx.Delete(map[string]any{"PK": "p"}), "transaction delete failed")
}

func TestTransactionManager_ExecuteWithRetry_RetriesOnRetryable_Round23(t *testing.T) {
	t.Parallel()

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()

	attempt := 0
	db := &transactionRunnerDB{query: q}
	db.transactionFn = func(fn func(*core.Tx) error) error {
		tx := &core.Tx{}
		tx.SetDB(db)
		_ = fn(tx)

		attempt++
		if attempt == 1 {
			return errors.New("ThrottlingException: try again")
		}
		return nil
	}

	tm := NewTransactionManager(db, zap.NewNop(), cost.New())
	config := TransactionConfig{MaxRetries: 1, BackoffDuration: 0, Logger: zap.NewNop()}

	var fnCalls int
	err := tm.ExecuteWithRetry(context.Background(), config, func(_ *TransactionContext) error {
		fnCalls++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, fnCalls)
}

func TestTransactionManager_ExecuteWithRetry_StopsOnNonRetryable_Round23(t *testing.T) {
	t.Parallel()

	db := &transactionRunnerDB{
		transactionFn: func(fn func(*core.Tx) error) error {
			return errors.New("fatal")
		},
	}

	tm := NewTransactionManager(db, zap.NewNop(), nil)
	config := TransactionConfig{MaxRetries: 3, BackoffDuration: 0}

	err := tm.ExecuteWithRetry(context.Background(), config, func(_ *TransactionContext) error { return nil })
	require.Error(t, err)
}

func TestTransactionManager_ExecuteNested_MergesOnSuccess_Round23(t *testing.T) {
	t.Parallel()

	tm := NewTransactionManager(&transactionRunnerDB{}, zap.NewNop(), nil)
	parent := &TransactionContext{tx: nil, operationsCnt: 5}

	require.Error(t, tm.ExecuteNested(context.Background(), nil, func(*TransactionContext) error { return nil }))

	require.NoError(t, tm.ExecuteNested(context.Background(), parent, func(nested *TransactionContext) error {
		_ = nested.TransactionalGet(nil)
		_ = nested.TransactionalBatchGet([]any{"a", "b"})
		return nil
	}))
	require.Equal(t, 8, parent.operationsCnt)

	parent.operationsCnt = 5
	require.Error(t, tm.ExecuteNested(context.Background(), parent, func(nested *TransactionContext) error {
		_ = nested.TransactionalGet(nil)
		return errors.New("boom")
	}))
	require.Equal(t, 5, parent.operationsCnt)
}

func TestTransactionalRepository_FollowUserTransactional_Succeeds_Round23(t *testing.T) {
	t.Parallel()

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()

	db := &transactionRunnerDB{query: q}
	repo := NewTransactionalRepository(db, "main", zap.NewNop(), cost.New())

	require.NoError(t, repo.FollowUserTransactional(context.Background(), "alice", "bob"))
}

func TestTransactionalRepository_CreateStatusWithChecksTransactional_Succeeds_Round23(t *testing.T) {
	t.Parallel()

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()

	db := &transactionRunnerDB{query: q}
	repo := NewTransactionalRepository(db, "main", zap.NewNop(), cost.New())

	status := map[string]any{"UserID": "alice", "PK": "STATUS#1", "SK": "STATUS#1"}
	require.NoError(t, repo.CreateStatusWithChecksTransactional(context.Background(), status))
}

func TestTransactionalRepository_TransferOwnershipTransactional_Succeeds_Round23(t *testing.T) {
	t.Parallel()

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()

	db := &transactionRunnerDB{query: q}
	repo := NewTransactionalRepository(db, "main", zap.NewNop(), cost.New())

	require.NoError(t, repo.TransferOwnershipTransactional(context.Background(), "from", "to", []string{"r1", "r2"}))
}

func TestTransactionManager_ExecuteWithConsistency_ContextCanceled_Round23(t *testing.T) {
	t.Parallel()

	db := &transactionRunnerDB{
		transactionFn: func(fn func(*core.Tx) error) error {
			return errors.New("ThrottlingException")
		},
	}
	tm := NewTransactionManager(db, zap.NewNop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := tm.ExecuteWithConsistency(ctx, "strong", func(*TransactionContext) error { return nil })
	require.ErrorIs(t, err, context.Canceled)
}

func TestTransactionContext_BatchWrite_Succeeds_Round23(t *testing.T) {
	t.Parallel()

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()

	db := &transactionRunnerDB{query: q}
	tx := &core.Tx{}
	tx.SetDB(db)

	txCtx := &TransactionContext{tx: tx}
	require.NoError(t, txCtx.TransactionalBatchWrite([]any{map[string]any{"PK": "p1"}}, []any{map[string]any{"PK": "p2"}}))
	require.GreaterOrEqual(t, txCtx.operationsCnt, 2)
}

func TestTransactionContext_DeleteByKey_And_UpdateWithExpression_Succeed_Round23(t *testing.T) {
	t.Parallel()

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()

	db := &transactionRunnerDB{query: q}
	tx := &core.Tx{}
	tx.SetDB(db)

	txCtx := &TransactionContext{tx: tx}
	require.NoError(t, txCtx.UpdateWithExpression(map[string]any{"PK": "p"}, "SET X = :x", 1))
	require.NoError(t, txCtx.DeleteByKey("table", map[string]any{"PK": "p"}))
}

func TestTransactionManager_ExecuteIsolated_Succeeds_Round23(t *testing.T) {
	t.Parallel()

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()

	db := &transactionRunnerDB{query: q}
	tm := NewTransactionManager(db, zap.NewNop(), cost.New())

	require.NoError(t, tm.ExecuteIsolated(context.Background(), "serializable", func(*TransactionContext) error { return nil }))
}
