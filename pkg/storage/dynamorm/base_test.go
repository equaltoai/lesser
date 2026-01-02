package dynamorm

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDB struct{}

func (f fakeDB) Model(any) core.Query                      { return nil }
func (f fakeDB) Transaction(func(tx *core.Tx) error) error { return nil }
func (f fakeDB) Migrate() error                            { return nil }
func (f fakeDB) AutoMigrate(...any) error                  { return nil }
func (f fakeDB) Close() error                              { return nil }
func (f fakeDB) WithContext(context.Context) core.DB       { return f }

func TestBaseRepository_Getters(t *testing.T) {
	db := fakeDB{}
	repo := NewBaseRepository(db, "test-table")

	assert.Equal(t, "test-table", repo.GetTableName())
	assert.Equal(t, db, repo.GetDB())
}

func TestBaseModel_Hooks(t *testing.T) {
	var m BaseModel
	require.NoError(t, m.BeforeCreate())
	assert.False(t, m.CreatedAt.IsZero())
	assert.True(t, m.CreatedAt.Equal(m.UpdatedAt))

	prev := m.UpdatedAt
	require.NoError(t, m.BeforeUpdate())
	assert.True(t, m.UpdatedAt.After(prev) || m.UpdatedAt.Equal(prev))
}

func TestNewLambdaOptimizedClient_RegionSelection(t *testing.T) {
	orig := newDynamormClient
	t.Cleanup(func() { newDynamormClient = orig })

	var gotCfg session.Config
	newDynamormClient = func(cfg session.Config) (core.DB, error) {
		gotCfg = cfg
		return fakeDB{}, nil
	}

	t.Run("explicit region wins", func(t *testing.T) {
		t.Setenv("AWS_REGION", "eu-central-1")
		t.Setenv("AWS_DEFAULT_REGION", "ap-southeast-1")

		_, err := NewLambdaOptimizedClient(context.Background(), " us-west-2 ")
		require.NoError(t, err)
		assert.Equal(t, "us-west-2", gotCfg.Region)
	})

	t.Run("AWS_REGION fallback", func(t *testing.T) {
		t.Setenv("AWS_REGION", " eu-central-1 ")
		t.Setenv("AWS_DEFAULT_REGION", "ap-southeast-1")

		_, err := NewLambdaOptimizedClient(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "eu-central-1", gotCfg.Region)
	})

	t.Run("AWS_DEFAULT_REGION fallback", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", " ap-southeast-1 ")

		_, err := NewLambdaOptimizedClient(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "ap-southeast-1", gotCfg.Region)
	})

	t.Run("default when env empty", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")

		_, err := NewLambdaOptimizedClient(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", gotCfg.Region)
	})

	t.Run("propagates init errors", func(t *testing.T) {
		newDynamormClient = func(cfg session.Config) (core.DB, error) {
			gotCfg = cfg
			return nil, stdErrors.New("boom")
		}

		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")

		_, err := NewLambdaOptimizedClient(context.Background(), "")
		require.Error(t, err)
	})
}

func TestPreRegisterModels_NoOp(t *testing.T) {
	require.NoError(t, PreRegisterModels(nil, 1, "x"))
}
