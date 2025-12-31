package dynamorm

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type transactionTestDB struct {
	query core.Query
}

func (db *transactionTestDB) Model(_ any) core.Query {
	return db.query
}

func (db *transactionTestDB) Transaction(fn func(tx *core.Tx) error) error {
	return fn(&core.Tx{})
}

func (db *transactionTestDB) Migrate() error { return nil }

func (db *transactionTestDB) AutoMigrate(_ ...any) error { return nil }

func (db *transactionTestDB) Close() error { return nil }

func (db *transactionTestDB) WithContext(_ context.Context) core.DB { return db }

func TestExampleCreateUserWithPosts_UsesInjectedClient_Round23(t *testing.T) {
	originalClient := client
	originalErr := clientErr
	originalOnce := clientOnce
	originalLambdaDB := lambdaDB
	t.Cleanup(func() {
		client = originalClient
		clientErr = originalErr
		clientOnce = originalOnce
		lambdaDB = originalLambdaDB
	})

	db := &transactionTestDB{}
	client = db
	clientErr = nil
	clientOnce = sync.Once{}
	clientOnce.Do(func() {}) // mark initialized to bypass dynamorm.New(...)

	require.NoError(t, ExampleCreateUserWithPosts(context.Background()))
}

func TestExampleCreateUserWithPosts_GetClientError_Round23(t *testing.T) {
	originalClient := client
	originalErr := clientErr
	originalOnce := clientOnce
	originalLambdaDB := lambdaDB
	t.Cleanup(func() {
		client = originalClient
		clientErr = originalErr
		clientOnce = originalOnce
		lambdaDB = originalLambdaDB
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
	originalOnce := clientOnce
	originalLambdaDB := lambdaDB
	t.Cleanup(func() {
		client = originalClient
		clientErr = originalErr
		clientOnce = originalOnce
		lambdaDB = originalLambdaDB
	})

	q := new(dynamormMocks.MockQuery)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

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
