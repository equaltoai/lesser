package theorydb

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
)

type stubDB struct {
	transactionCalls int
	query            core.Query
}

func (s *stubDB) Model(any) core.Query                { return s.query }
func (s *stubDB) Migrate() error                      { return nil }
func (s *stubDB) AutoMigrate(...any) error            { return nil }
func (s *stubDB) Close() error                        { return nil }
func (s *stubDB) WithContext(context.Context) core.DB { return s }
func (s *stubDB) Transact() core.TransactionBuilder {
	return new(dynamormMocks.MockTransactionBuilder)
}
func (s *stubDB) TransactWrite(_ context.Context, fn func(core.TransactionBuilder) error) error {
	s.transactionCalls++
	if fn == nil {
		return nil
	}
	return fn(new(dynamormMocks.MockTransactionBuilder))
}

func TestExecuteTransaction_RunsFnWithClient(t *testing.T) {
	resetClientState()

	origGetConfig := getAppConfig
	origNewClient := newDynamormStandardClient
	t.Cleanup(func() {
		getAppConfig = origGetConfig
		newDynamormStandardClient = origNewClient
		resetClientState()
	})

	getAppConfig = func() *config.Config {
		return &config.Config{Region: "us-east-1"}
	}

	q := new(dynamormMocks.MockQuery)
	q.On("Create").Return(nil).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()
	q.On("Update").Return(nil).Maybe()
	db := &stubDB{query: q}
	newDynamormStandardClient = func(session.Config) (core.DB, error) {
		return db, nil
	}

	called := false
	err := ExecuteTransaction(context.Background(), func(tx *Transaction) error {
		called = true
		return tx.Put(&TestUser{Name: "ok"})
	})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, 1, db.transactionCalls)
}

func TestExecuteTransaction_PropagatesClientErrors(t *testing.T) {
	resetClientState()

	origGetConfig := getAppConfig
	origNewClient := newDynamormStandardClient
	t.Cleanup(func() {
		getAppConfig = origGetConfig
		newDynamormStandardClient = origNewClient
		resetClientState()
	})

	getAppConfig = func() *config.Config {
		return &config.Config{Region: "us-east-1"}
	}

	newDynamormStandardClient = func(session.Config) (core.DB, error) {
		return nil, stdErrors.New("boom")
	}

	err := ExecuteTransaction(context.Background(), func(*Transaction) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get DynamoDB client")
}

func TestExecuteLambdaTransaction_PropagatesClientErrors(t *testing.T) {
	resetClientState()

	orig := newDynamormLambdaOptimized
	t.Cleanup(func() {
		newDynamormLambdaOptimized = orig
		resetClientState()
	})

	newDynamormLambdaOptimized = func() (*tabletheory.LambdaDB, error) {
		return nil, stdErrors.New("boom")
	}

	err := ExecuteLambdaTransaction(context.Background(), func(*Transaction) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get Lambda DynamoDB client")
}
