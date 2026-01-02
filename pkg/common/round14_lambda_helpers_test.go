package common

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultLambdaInitOptions_AllTypes(t *testing.T) {
	t.Run("api", func(t *testing.T) {
		opts := DefaultLambdaInitOptions(LambdaTypeAPI)
		assert.True(t, opts.InitializeStorage)
		assert.True(t, opts.InitializeRepositories)
		assert.True(t, opts.InitializeEMFMetrics)
		assert.True(t, opts.InitializeHealthChecker)
		assert.True(t, opts.InitializeTracingManager)
		assert.True(t, opts.InitializeMetricsCollector)
		assert.True(t, opts.InitializeLatencyTracking)
		assert.True(t, opts.InitializeAuthService)
		assert.True(t, opts.InitializeStreamingServices)
		assert.False(t, opts.InitializeFederationServices)
	})

	t.Run("federation", func(t *testing.T) {
		opts := DefaultLambdaInitOptions(LambdaTypeFederation)
		assert.True(t, opts.InitializeStorage)
		assert.True(t, opts.InitializeRepositories)
		assert.True(t, opts.InitializeEMFMetrics)
		assert.True(t, opts.InitializeTracingManager)
		assert.True(t, opts.InitializeAlerting)
		assert.True(t, opts.InitializeFederationServices)
		assert.False(t, opts.InitializeAuthService)
		assert.False(t, opts.InitializeStreamingServices)
	})

	t.Run("processor", func(t *testing.T) {
		opts := DefaultLambdaInitOptions(LambdaTypeProcessor)
		assert.True(t, opts.InitializeStorage)
		assert.True(t, opts.InitializeRepositories)
		assert.True(t, opts.OptimizeForColdStart)
		assert.False(t, opts.EnableServiceCaching)
	})

	t.Run("media", func(t *testing.T) {
		opts := DefaultLambdaInitOptions(LambdaTypeMedia)
		assert.True(t, opts.InitializeStorage)
		assert.True(t, opts.InitializeRepositories)
		assert.False(t, opts.OptimizeForColdStart)
		assert.True(t, opts.InitializeAlerting)
	})

	t.Run("ai", func(t *testing.T) {
		opts := DefaultLambdaInitOptions(LambdaTypeAI)
		assert.True(t, opts.InitializeStorage)
		assert.True(t, opts.InitializeRepositories)
		assert.False(t, opts.OptimizeForColdStart)
		assert.True(t, opts.InitializeAlerting)
	})

	t.Run("default", func(t *testing.T) {
		opts := DefaultLambdaInitOptions(LambdaTypeBasic)
		assert.True(t, opts.InitializeStorage)
		assert.True(t, opts.InitializeRepositories)
		assert.True(t, opts.OptimizeForColdStart)
	})
}

func TestLambdaContext_InitializeStorageServices(t *testing.T) {
	origInitDynamORM := initializeDynamORMFunc
	origInitRepos := initializeRepositoryFactoryFunc
	t.Cleanup(func() {
		initializeDynamORMFunc = origInitDynamORM
		initializeRepositoryFactoryFunc = origInitRepos
	})

	t.Run("disabled returns nil", func(t *testing.T) {
		lambdaCtx := &LambdaContext{
			Config: &config.Config{DynamoTableName: "tbl", Region: "us-east-1"},
			Logger: zap.NewNop(),
		}
		require.NoError(t, lambdaCtx.InitializeStorageServices(LambdaInitOptions{InitializeStorage: false}))
	})

	t.Run("missing table name returns error", func(t *testing.T) {
		lambdaCtx := &LambdaContext{
			Config: &config.Config{DynamoTableName: "", Region: "us-east-1"},
			Logger: zap.NewNop(),
		}
		err := lambdaCtx.InitializeStorageServices(LambdaInitOptions{InitializeStorage: true})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDynamoTableRequired)
	})

	t.Run("cold start optimized initializes db and repos", func(t *testing.T) {
		db := &struct{}{}
		repos := &struct{}{}
		awsServices := &awsInit.AWSServices{Logger: zap.NewNop()}

		initializeDynamORMFunc = func(_ context.Context, region string, optimize bool) (interface{}, error) {
			assert.Equal(t, "us-east-1", region)
			assert.True(t, optimize)
			return db, nil
		}
		initializeRepositoryFactoryFunc = func(dbArg interface{}, table string, awsArg interface{}, _ *zap.Logger) (interface{}, error) {
			assert.Same(t, db, dbArg)
			assert.Equal(t, "tbl", table)
			assert.Same(t, awsServices, awsArg)
			return repos, nil
		}

		lambdaCtx := &LambdaContext{
			Config:      &config.Config{DynamoTableName: "tbl", Region: "us-east-1"},
			Logger:      zap.NewNop(),
			AWSServices: awsServices,
			ServiceName: "api",
		}

		err := lambdaCtx.InitializeStorageServices(LambdaInitOptions{
			InitializeStorage:      true,
			InitializeRepositories: true,
			OptimizeForColdStart:   true,
		})
		require.NoError(t, err)
		assert.Same(t, db, lambdaCtx.DynamoDB)
		assert.Same(t, repos, lambdaCtx.Repos)
	})

	t.Run("non-optimized initializes db only", func(t *testing.T) {
		db := &struct{}{}
		initializeDynamORMFunc = func(_ context.Context, _ string, optimize bool) (interface{}, error) {
			assert.False(t, optimize)
			return db, nil
		}
		initializeRepositoryFactoryFunc = func(_ interface{}, _ string, _ interface{}, _ *zap.Logger) (interface{}, error) {
			t.Fatal("initializeRepositoryFactory should not be called")
			return nil, nil
		}

		lambdaCtx := &LambdaContext{
			Config: &config.Config{DynamoTableName: "tbl", Region: "us-east-1"},
			Logger: zap.NewNop(),
		}
		err := lambdaCtx.InitializeStorageServices(LambdaInitOptions{
			InitializeStorage:      true,
			InitializeRepositories: false,
			OptimizeForColdStart:   false,
		})
		require.NoError(t, err)
		assert.Same(t, db, lambdaCtx.DynamoDB)
		assert.Nil(t, lambdaCtx.Repos)
	})

	t.Run("init failure returns error", func(t *testing.T) {
		initializeDynamORMFunc = func(_ context.Context, _ string, _ bool) (interface{}, error) {
			return nil, stdErrors.New("nope")
		}
		lambdaCtx := &LambdaContext{
			Config: &config.Config{DynamoTableName: "tbl", Region: "us-east-1"},
			Logger: zap.NewNop(),
		}
		err := lambdaCtx.InitializeStorageServices(LambdaInitOptions{
			InitializeStorage:    true,
			OptimizeForColdStart: true,
		})
		require.Error(t, err)
	})
}

func TestLambdaContext_InitializeObservabilityServices(t *testing.T) {
	origEMF := initializeEMFMetricsFunc
	origHealth := initializeHealthCheckerFunc
	origTracing := initializeTracingManagerFunc
	origMetricsCollector := initializeMetricsCollectorFunc
	origLatency := initializeLatencyTrackingFunc
	origAlert := initializeAlertManagerFunc
	origIsTracingEnabled := isTracingEnabledFunc
	t.Cleanup(func() {
		initializeEMFMetricsFunc = origEMF
		initializeHealthCheckerFunc = origHealth
		initializeTracingManagerFunc = origTracing
		initializeMetricsCollectorFunc = origMetricsCollector
		initializeLatencyTrackingFunc = origLatency
		initializeAlertManagerFunc = origAlert
		isTracingEnabledFunc = origIsTracingEnabled
	})

	t.Run("metrics disabled returns early", func(t *testing.T) {
		lambdaCtx := &LambdaContext{
			Config: &config.Config{DisableMetrics: true},
			Logger: zap.NewNop(),
		}
		require.NoError(t, lambdaCtx.InitializeObservabilityServices(LambdaInitOptions{
			InitializeEMFMetrics:     true,
			InitializeHealthChecker:  true,
			InitializeTracingManager: true,
		}))
	})

	t.Run("initializes enabled services", func(t *testing.T) {
		emf := &struct{}{}
		health := &struct{}{}
		tracing := &struct{}{}
		metricsCollector := &struct{}{}
		latencyAgg := &struct{}{}
		latencyAlert := &struct{}{}
		alertMgr := &struct{}{}

		initializeEMFMetricsFunc = func(_ *zap.Logger, namespace, service string, _ interface{}) interface{} {
			assert.Contains(t, namespace, "Lesser/")
			assert.Equal(t, "api", service)
			return emf
		}
		initializeHealthCheckerFunc = func(_ *zap.Logger, _ interface{}, serviceName, _, tableName string) interface{} {
			assert.Equal(t, "api", serviceName)
			assert.Equal(t, "tbl", tableName)
			return health
		}
		initializeTracingManagerFunc = func(_ *zap.Logger, serviceName, version string) interface{} {
			assert.Equal(t, "api", serviceName)
			assert.Equal(t, "v1", version)
			return tracing
		}
		isTracingEnabledFunc = func(_ interface{}) bool { return true }
		initializeMetricsCollectorFunc = func(_ interface{}, _ *zap.Logger, _ string) interface{} { return metricsCollector }
		initializeLatencyTrackingFunc = func(_ *zap.Logger, _ interface{}, _ string) (interface{}, interface{}) {
			return latencyAgg, latencyAlert
		}
		initializeAlertManagerFunc = func(_ *zap.Logger) interface{} { return alertMgr }

		lambdaCtx := &LambdaContext{
			Config:      &config.Config{DisableMetrics: false, Version: "v1", DynamoTableName: "tbl"},
			Logger:      zap.NewNop(),
			ServiceName: "api",
			AWSServices: &awsInit.AWSServices{Logger: zap.NewNop()},
			Repos:       &struct{}{},
		}

		err := lambdaCtx.InitializeObservabilityServices(LambdaInitOptions{
			InitializeEMFMetrics:       true,
			InitializeHealthChecker:    true,
			InitializeTracingManager:   true,
			InitializeMetricsCollector: true,
			InitializeLatencyTracking:  true,
			InitializeAlerting:         true,
		})
		require.NoError(t, err)

		assert.Same(t, emf, lambdaCtx.EMFMetrics)
		assert.Same(t, health, lambdaCtx.HealthChecker)
		assert.Same(t, tracing, lambdaCtx.TracingManager)
		assert.Same(t, metricsCollector, lambdaCtx.MetricsCollector)
		assert.Same(t, latencyAgg, lambdaCtx.LatencyAggregator)
		assert.Same(t, latencyAlert, lambdaCtx.LatencyAlerter)
		assert.Same(t, alertMgr, lambdaCtx.AlertManager)
	})
}

func TestLambdaContext_InitializeServiceSpecificDependencies(t *testing.T) {
	origAuth := initializeAuthServicesFunc
	origFederation := initializeFederationServicesFunc
	origStreaming := initializeStreamingServicesFunc
	t.Cleanup(func() {
		initializeAuthServicesFunc = origAuth
		initializeFederationServicesFunc = origFederation
		initializeStreamingServicesFunc = origStreaming
	})

	t.Run("initializes auth + federation + streaming when enabled", func(t *testing.T) {
		authSvc := &struct{}{}
		authMw := &struct{}{}
		initializeAuthServicesFunc = func(_ interface{}) (interface{}, interface{}, error) {
			return authSvc, authMw, nil
		}

		sig := &struct{}{}
		del := &struct{}{}
		cost := &struct{}{}
		rl := &struct{}{}
		initializeFederationServicesFunc = func(_ interface{}, _ *zap.Logger) (interface{}, interface{}, interface{}, interface{}) {
			return sig, del, cost, rl
		}

		streamQ := &struct{}{}
		initializeStreamingServicesFunc = func(_ interface{}, _ string, _ *zap.Logger) interface{} { return streamQ }

		lambdaCtx := &LambdaContext{
			Config:   &config.Config{DynamoTableName: "tbl"},
			Logger:   zap.NewNop(),
			Repos:    &struct{}{},
			DynamoDB: &struct{}{},
		}

		err := lambdaCtx.InitializeServiceSpecificDependencies(LambdaInitOptions{
			InitializeAuthService:        true,
			InitializeFederationServices: true,
			InitializeStreamingServices:  true,
		})
		require.NoError(t, err)

		assert.Same(t, authSvc, lambdaCtx.AuthService)
		assert.Same(t, authMw, lambdaCtx.AuthMiddleware)
		assert.Same(t, sig, lambdaCtx.SignatureService)
		assert.Same(t, del, lambdaCtx.DeliveryService)
		assert.Same(t, cost, lambdaCtx.CostCalculator)
		assert.Same(t, rl, lambdaCtx.RateLimiter)
		assert.Same(t, streamQ, lambdaCtx.StreamQueue)
	})

	t.Run("auth init error surfaces", func(t *testing.T) {
		initializeAuthServicesFunc = func(_ interface{}) (interface{}, interface{}, error) {
			return nil, nil, stdErrors.New("bad auth")
		}
		lambdaCtx := &LambdaContext{
			Config: &config.Config{DynamoTableName: "tbl"},
			Logger: zap.NewNop(),
			Repos:  &struct{}{},
		}
		err := lambdaCtx.InitializeServiceSpecificDependencies(LambdaInitOptions{InitializeAuthService: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize auth services")
	})
}

func TestLambdaContext_FlushObservabilityServices(t *testing.T) {
	origFlushEMF := flushEMFMetricsFunc
	origFlushMetrics := flushMetricsCollectorFunc
	t.Cleanup(func() {
		flushEMFMetricsFunc = origFlushEMF
		flushMetricsCollectorFunc = origFlushMetrics
	})

	emfCalled := false
	metricsCalled := false
	flushEMFMetricsFunc = func(_ interface{}) { emfCalled = true }
	flushMetricsCollectorFunc = func(_ interface{}) { metricsCalled = true }

	lambdaCtx := &LambdaContext{
		Logger:           zap.NewNop(),
		EMFMetrics:       &struct{}{},
		MetricsCollector: &struct{}{},
	}
	lambdaCtx.FlushObservabilityServices()

	assert.True(t, emfCalled)
	assert.True(t, metricsCalled)
}

func TestLambdaContext_CreateStandardizedLambdaHandler(t *testing.T) {
	origFlushEMF := flushEMFMetricsFunc
	t.Cleanup(func() { flushEMFMetricsFunc = origFlushEMF })

	flushed := false
	flushEMFMetricsFunc = func(_ interface{}) { flushed = true }

	lambdaCtx := &LambdaContext{
		Logger:      zap.NewNop(),
		ServiceName: "api",
		StartTime:   time.Now(),
		EMFMetrics:  &struct{}{},
	}

	h := lambdaCtx.CreateStandardizedLambdaHandler(func(_ context.Context, _ interface{}) (interface{}, error) {
		return map[string]string{"ok": "true"}, nil
	})

	out, err := h(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"ok": "true"}, out)
	assert.True(t, flushed)
}
