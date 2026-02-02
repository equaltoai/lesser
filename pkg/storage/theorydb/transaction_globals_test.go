package theorydb

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type stubDB struct {
	transactionCalls int
}

func (s *stubDB) Model(any) core.Query { return nil }
func (s *stubDB) Transaction(fn func(tx *core.Tx) error) error {
	s.transactionCalls++
	return fn(&core.Tx{})
}
func (s *stubDB) Migrate() error                      { return nil }
func (s *stubDB) AutoMigrate(...any) error            { return nil }
func (s *stubDB) Close() error                        { return nil }
func (s *stubDB) WithContext(context.Context) core.DB { return s }

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

	db := &stubDB{}
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
