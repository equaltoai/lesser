package theorydb

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
)

type fakeDB struct{}

func (f fakeDB) Model(any) core.Query                { return nil }
func (f fakeDB) Migrate() error                      { return nil }
func (f fakeDB) AutoMigrate(...any) error            { return nil }
func (f fakeDB) Close() error                        { return nil }
func (f fakeDB) WithContext(context.Context) core.DB { return f }

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
	origGetConfig := getAppConfig
	origLambdaClient := newDynamormLambdaOptimizedWithEnv
	origStandardClient := newDynamormStandardClient
	t.Cleanup(func() {
		getAppConfig = origGetConfig
		newDynamormLambdaOptimizedWithEnv = origLambdaClient
		newDynamormStandardClient = origStandardClient
	})

	getAppConfig = func() *config.Config { return &config.Config{} }
	newDynamormStandardClient = func(cfg session.Config) (core.DB, error) {
		t.Fatalf("NewLambdaOptimizedClient must not create a standard TableTheory DB: %#v", cfg)
		return nil, nil
	}

	var gotOpts lambdaOptimizedClientOptions
	newDynamormLambdaOptimizedWithEnv = func(opts lambdaOptimizedClientOptions) (*tabletheory.LambdaDB, error) {
		gotOpts = opts
		return tabletheory.NewLambdaOptimized()
	}

	t.Run("explicit region wins", func(t *testing.T) {
		t.Setenv("AWS_REGION", "eu-central-1")
		t.Setenv("AWS_DEFAULT_REGION", "ap-southeast-1")

		_, err := NewLambdaOptimizedClient(context.Background(), " us-west-2 ")
		require.NoError(t, err)
		assert.Equal(t, "us-west-2", gotOpts.Region)
	})

	t.Run("AWS_REGION fallback", func(t *testing.T) {
		t.Setenv("AWS_REGION", " eu-central-1 ")
		t.Setenv("AWS_DEFAULT_REGION", "ap-southeast-1")

		_, err := NewLambdaOptimizedClient(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "eu-central-1", gotOpts.Region)
	})

	t.Run("AWS_DEFAULT_REGION fallback", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", " ap-southeast-1 ")

		_, err := NewLambdaOptimizedClient(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "ap-southeast-1", gotOpts.Region)
	})

	t.Run("default when env empty", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")

		_, err := NewLambdaOptimizedClient(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", gotOpts.Region)
	})

	t.Run("propagates init errors", func(t *testing.T) {
		newDynamormLambdaOptimizedWithEnv = func(opts lambdaOptimizedClientOptions) (*tabletheory.LambdaDB, error) {
			gotOpts = opts
			return nil, stdErrors.New("boom")
		}

		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")

		_, err := NewLambdaOptimizedClient(context.Background(), "")
		require.Error(t, err)
		assert.Equal(t, "us-east-1", gotOpts.Region)
	})
}

func TestNewLambdaOptimizedClient_LocalEndpointBehavior(t *testing.T) {
	origGetConfig := getAppConfig
	origLambdaClient := newDynamormLambdaOptimizedWithEnv
	t.Cleanup(func() {
		getAppConfig = origGetConfig
		newDynamormLambdaOptimizedWithEnv = origLambdaClient
	})

	getAppConfig = func() *config.Config {
		return &config.Config{DynamoDBEndpoint: " http://localhost:8000 "}
	}

	var gotOpts lambdaOptimizedClientOptions
	newDynamormLambdaOptimizedWithEnv = func(opts lambdaOptimizedClientOptions) (*tabletheory.LambdaDB, error) {
		gotOpts = opts
		return tabletheory.NewLambdaOptimized()
	}

	db, err := NewLambdaOptimizedClient(nil, "us-west-2")
	require.NoError(t, err)
	require.IsType(t, &tabletheory.LambdaDB{}, db)
	assert.Equal(t, "us-west-2", gotOpts.Region)
	assert.Equal(t, "http://localhost:8000", gotOpts.Endpoint)
}

func TestNewLambdaOptimizedClient_ContextHandling(t *testing.T) {
	t.Run("nil context returns buffered lambda client without deadline", func(t *testing.T) {
		db, err := NewLambdaOptimizedClient(nil, "us-east-1")
		require.NoError(t, err)
		require.IsType(t, &tabletheory.LambdaDB{}, db)

		assert.Equal(t, defaultTimeoutBuffer, lambdaTimeoutBufferOf(t, db))
		assert.True(t, lambdaDeadlineOf(t, db).IsZero())
	})

	t.Run("deadline context applies lambda timeout", func(t *testing.T) {
		deadline := time.Now().Add(30 * time.Second).Round(0)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		db, err := NewLambdaOptimizedClient(ctx, "us-east-1")
		require.NoError(t, err)
		require.IsType(t, &tabletheory.LambdaDB{}, db)

		assert.Equal(t, defaultTimeoutBuffer, lambdaTimeoutBufferOf(t, db))
		assert.Equal(t, deadline, lambdaDeadlineOf(t, db))
	})
}

func TestPreRegisterModels_NoOp(t *testing.T) {
	require.NoError(t, PreRegisterModels(nil, 1, "x"))
}
