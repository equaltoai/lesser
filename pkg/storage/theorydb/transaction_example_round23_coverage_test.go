package theorydb

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormMocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
)

type transactionTestDB struct {
	query core.Query
}

func (db *transactionTestDB) Model(_ any) core.Query {
	return db.query
}

func (db *transactionTestDB) Migrate() error { return nil }

func (db *transactionTestDB) AutoMigrate(_ ...any) error { return nil }

func (db *transactionTestDB) Close() error { return nil }

func (db *transactionTestDB) WithContext(_ context.Context) core.DB { return db }

func (db *transactionTestDB) Transact() core.TransactionBuilder {
	return new(dynamormMocks.MockTransactionBuilder)
}

func (db *transactionTestDB) TransactWrite(_ context.Context, fn func(core.TransactionBuilder) error) error {
	if fn == nil {
		return nil
	}
	return fn(new(dynamormMocks.MockTransactionBuilder))
}

func TestExampleCreateUserWithPosts_UsesInjectedClient_Round23(t *testing.T) {
	originalClient := client
	originalErr := clientErr
	originalLambdaDB := lambdaDB
	// Avoid copying sync.Once which contains a mutex
	wasInitialized := client != nil // Heuristic: if client is set, Once has likely run

	t.Cleanup(func() {
		client = originalClient
		clientErr = originalErr
		lambdaDB = originalLambdaDB

		// Restore Once state
		clientOnce = sync.Once{}
		if wasInitialized {
			clientOnce.Do(func() {})
		}
	})

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()

	db := &transactionTestDB{query: q}
	client = db
	clientErr = nil
	clientOnce = sync.Once{}
	clientOnce.Do(func() {}) // mark initialized to bypass tabletheory.New(...)

	require.NoError(t, ExampleCreateUserWithPosts(context.Background()))
}

func TestExampleCreateUserWithPosts_GetClientError_Round23(t *testing.T) {
	originalClient := client
	originalErr := clientErr
	originalLambdaDB := lambdaDB
	// Avoid copying sync.Once which contains a mutex
	wasInitialized := client != nil // Heuristic: if client is set, Once has likely run

	t.Cleanup(func() {
		client = originalClient
		clientErr = originalErr
		lambdaDB = originalLambdaDB

		// Restore Once state
		clientOnce = sync.Once{}
		if wasInitialized {
			clientOnce.Do(func() {})
		}
	})

	client = nil
	clientErr = errors.New("client unavailable")
	clientOnce = sync.Once{}
	clientOnce.Do(func() {})

	err := ExampleCreateUserWithPosts(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get DynamoDB client")
}

func TestExampleTransferBalance_SucceedsAndFailsForInsufficientFunds_Round23(t *testing.T) {
	originalClient := client
	originalErr := clientErr
	originalLambdaDB := lambdaDB
	// Avoid copying sync.Once which contains a mutex
	wasInitialized := client != nil // Heuristic: if client is set, Once has likely run

	t.Cleanup(func() {
		client = originalClient
		clientErr = originalErr
		lambdaDB = originalLambdaDB

		// Restore Once state
		clientOnce = sync.Once{}
		if wasInitialized {
			clientOnce.Do(func() {})
		}
	})

	q := new(dynamormMocks.MockQuery)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()
	q.On("Update").Return(nil).Maybe()

	var firstCalls int
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		firstCalls++
		dest := args.Get(0)
		destValue := reflect.ValueOf(dest)
		require.Equal(t, reflect.Ptr, destValue.Kind())
		balance := 0.0
		if firstCalls == 1 {
			balance = 100.0
		} else {
			balance = 5.0
		}
		destValue.Elem().FieldByName("Balance").SetFloat(balance)
	}).Return(nil).Maybe()

	db := &transactionTestDB{query: q}
	client = db
	clientErr = nil
	clientOnce = sync.Once{}
	clientOnce.Do(func() {})

	require.NoError(t, ExampleTransferBalance(context.Background(), "from", "to", 10))

	firstCalls = 0
	err := ExampleTransferBalance(context.Background(), "from", "to", 500)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient balance")
}
