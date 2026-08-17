package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type guardedDeleteTestDB struct {
	query     *mocks.MockQuery
	txBuilder core.TransactionBuilder
	txErr     error
}

func (db *guardedDeleteTestDB) Model(any) core.Query {
	if db.query == nil {
		db.query = new(mocks.MockQuery)
	}
	return db.query
}

func (db *guardedDeleteTestDB) Migrate() error                      { return nil }
func (db *guardedDeleteTestDB) AutoMigrate(...any) error            { return nil }
func (db *guardedDeleteTestDB) Close() error                        { return nil }
func (db *guardedDeleteTestDB) WithContext(context.Context) core.DB { return db }

func (db *guardedDeleteTestDB) TransactWrite(_ context.Context, fn func(core.TransactionBuilder) error) error {
	builder := db.txBuilder
	if builder == nil {
		builder = new(mocks.MockTransactionBuilder)
	}
	if err := fn(builder); err != nil {
		return err
	}
	if db.txErr != nil {
		return db.txErr
	}
	return builder.Execute()
}

func TestAccountRepository_GuardedDeleteTransactionalHelpers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repoWithoutTx := NewAccountRepository(new(mocks.MockDB), "test-table", "example.com", zap.NewNop())
	require.Error(t, repoWithoutTx.transactWrite(ctx, func(core.TransactionBuilder) error { return nil }))
	_, err := repoWithoutTx.transactionalDB()
	require.Error(t, err)

	repoWithTx := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())
	require.NoError(t, repoWithTx.transactWrite(ctx, func(core.TransactionBuilder) error { return nil }))
}

func TestAccountRepository_DeleteWebAuthnCredentialConditionedOnSurvivor(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		repo := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())

		require.NoError(t, repo.DeleteWebAuthnCredentialConditionedOnSurvivor(
			context.Background(),
			"alice",
			"cred-1",
			"",
			"0xabc",
		))
	})

	t.Run("condition failure is surfaced without wrapping", func(t *testing.T) {
		txErr := &dynamormerrors.TransactionError{
			Err:            dynamormerrors.ErrConditionFailed,
			Operation:      "condition_check",
			OperationIndex: 1,
			Reason:         "ConditionalCheckFailed",
		}

		repo := NewAccountRepository(&guardedDeleteTestDB{txErr: txErr}, "test-table", "example.com", zap.NewNop())

		err := repo.DeleteWebAuthnCredentialConditionedOnSurvivor(
			context.Background(),
			"alice",
			"cred-1",
			"",
			"0xabc",
		)
		require.ErrorIs(t, err, dynamormerrors.ErrConditionFailed)
	})
}

func TestAccountRepository_DeleteWalletCredentialConditionedOnSurvivor(t *testing.T) {
	t.Parallel()

	t.Run("success cleans up wallet index entries best effort", func(t *testing.T) {
		query := new(mocks.MockQuery)
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Delete").Return(nil).Maybe()

		repo := NewAccountRepository(&guardedDeleteTestDB{query: query}, "test-table", "example.com", zap.NewNop())

		require.NoError(t, repo.DeleteWalletCredentialConditionedOnSurvivor(
			context.Background(),
			"alice",
			"0xabc",
			"ethereum",
			"cred-1",
			"",
		))
	})

	t.Run("condition failure is surfaced without wrapping", func(t *testing.T) {
		txErr := &dynamormerrors.TransactionError{
			Err:            dynamormerrors.ErrConditionFailed,
			Operation:      "condition_check",
			OperationIndex: 1,
			Reason:         "ConditionalCheckFailed",
		}

		repo := NewAccountRepository(&guardedDeleteTestDB{txErr: txErr}, "test-table", "example.com", zap.NewNop())

		err := repo.DeleteWalletCredentialConditionedOnSurvivor(
			context.Background(),
			"alice",
			"0xabc",
			"ethereum",
			"cred-1",
			"",
		)
		require.ErrorIs(t, err, dynamormerrors.ErrConditionFailed)
	})

	t.Run("empty survivor is rejected", func(t *testing.T) {
		repo := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())

		err := repo.DeleteWalletCredentialConditionedOnSurvivor(
			context.Background(),
			"alice",
			"0xabc",
			"ethereum",
			"",
			"",
		)
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})
}
