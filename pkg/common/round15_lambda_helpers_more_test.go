package common

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLambdaHelpers_PlaceholderFunctions(t *testing.T) {
	_, err := initializeDynamORM(context.Background(), "us-east-1", true)
	assert.Same(t, ErrDynamORMNotImplemented, err)

	_, err = initializeRepositoryFactory(nil, "tbl", nil, zap.NewNop())
	assert.Same(t, ErrRepositoryFactoryNotImplemented, err)

	_, _, err = initializeAuthServices(nil)
	assert.Same(t, ErrAuthServicesNotImplemented, err)

	assert.Nil(t, initializeEMFMetrics(zap.NewNop(), "ns", "svc", &config.Config{}))
	assert.Nil(t, initializeHealthChecker(zap.NewNop(), nil, "svc", "v1", "tbl"))
	assert.Nil(t, initializeTracingManager(zap.NewNop(), "svc", "v1"))
	assert.Nil(t, initializeMetricsCollector(nil, zap.NewNop(), "svc"))

	latAgg, latAlert := initializeLatencyTracking(zap.NewNop(), nil, "svc")
	assert.Nil(t, latAgg)
	assert.Nil(t, latAlert)

	assert.Nil(t, initializeAlertManager(zap.NewNop()))

	sig, del, cost, rl := initializeFederationServices(nil, zap.NewNop())
	assert.Nil(t, sig)
	assert.Nil(t, del)
	assert.Nil(t, cost)
	assert.Nil(t, rl)

	assert.Nil(t, initializeStreamingServices(nil, "tbl", zap.NewNop()))

	flushEMFMetrics(nil)
	flushMetricsCollector(nil)

	assert.False(t, isTracingEnabled(nil))
}

func TestLambdaHelpers_InitializeWithDefaults_AndWithOptions(t *testing.T) {
	origInitDynamORM := initializeDynamORMFunc
	origInitRepos := initializeRepositoryFactoryFunc
	origAuth := initializeAuthServicesFunc
	t.Cleanup(func() {
		initializeDynamORMFunc = origInitDynamORM
		initializeRepositoryFactoryFunc = origInitRepos
		initializeAuthServicesFunc = origAuth
	})

	t.Run("InitializeWithDefaults succeeds with stubbed dependencies", func(t *testing.T) {
		db := &struct{}{}
		repos := &struct{}{}

		initializeDynamORMFunc = func(_ context.Context, _ string, _ bool) (interface{}, error) { return db, nil }
		initializeRepositoryFactoryFunc = func(_ interface{}, _ string, _ interface{}, _ *zap.Logger) (interface{}, error) {
			return repos, nil
		}

		lambdaCtx := &LambdaContext{
			LambdaType:  LambdaTypeBasic,
			Config:      &config.Config{DynamoTableName: "tbl", Region: "us-east-1"},
			Logger:      zap.NewNop(),
			ServiceName: "basic",
		}
		require.NoError(t, lambdaCtx.InitializeWithDefaults())
		assert.Same(t, db, lambdaCtx.DynamoDB)
		assert.Same(t, repos, lambdaCtx.Repos)
	})

	t.Run("InitializeWithOptions wraps storage errors", func(t *testing.T) {
		initializeDynamORMFunc = func(_ context.Context, _ string, _ bool) (interface{}, error) {
			return nil, stdErrors.New("boom")
		}

		lambdaCtx := &LambdaContext{
			Config: &config.Config{DynamoTableName: "tbl", Region: "us-east-1"},
			Logger: zap.NewNop(),
		}
		err := lambdaCtx.InitializeWithOptions(LambdaInitOptions{
			InitializeStorage:    true,
			OptimizeForColdStart: true,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize storage services")
	})

	t.Run("InitializeWithOptions wraps service-specific errors", func(t *testing.T) {
		initializeAuthServicesFunc = func(_ interface{}) (interface{}, interface{}, error) {
			return nil, nil, stdErrors.New("bad auth")
		}

		lambdaCtx := &LambdaContext{
			Config: &config.Config{DisableMetrics: true},
			Logger: zap.NewNop(),
			Repos:  &struct{}{},
		}
		err := lambdaCtx.InitializeWithOptions(LambdaInitOptions{
			InitializeStorage:     false,
			InitializeAuthService: true,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize service-specific dependencies")
	})
}
