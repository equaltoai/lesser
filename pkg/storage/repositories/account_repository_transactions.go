package repositories

import (
	"context"
	"fmt"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

type accountRepositoryTransactionalDB interface {
	core.DB
	TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error
}

func (r *AccountRepository) transactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	txDB, err := r.transactionalDB()
	if err != nil {
		return err
	}
	return txDB.TransactWrite(ctx, fn)
}

func (r *AccountRepository) transactionalDB() (accountRepositoryTransactionalDB, error) {
	if txDB, ok := r.db.(accountRepositoryTransactionalDB); ok && txDB != nil {
		return txDB, nil
	}
	return nil, fmt.Errorf("database does not support transact write operations")
}
