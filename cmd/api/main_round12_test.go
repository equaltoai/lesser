package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	liftapi "github.com/equaltoai/lesser/cmd/api/lift"
	"github.com/equaltoai/lesser/pkg/auth"
	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/observability"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

type streamQueueStub struct{}

func (s *streamQueueStub) QueueEventForUser(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func (s *streamQueueStub) QueueEventForStream(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func (s *streamQueueStub) QueueEventForConversation(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func (s *streamQueueStub) QueueEventForFollowers(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func TestExtractStandardizedServicesRound12(t *testing.T) {
	repos = nil
	authService = nil
	emfMetrics = nil
	healthChecker = nil
	tracingManager = nil
	metricsCollector = nil
	latencyAggregator = nil
	latencyAlerter = nil
	costTrackingService = nil

	cfg = &config.Config{Domain: "example.com", Stage: "development", Region: "us-east-1"}
	logger = zap.NewNop()
	repoStorage := newMainTestRepos(t)

	cwClient := cloudwatch.NewFromConfig(aws.Config{Region: "us-east-1"})
	metricsRecorder := observability.NewDefaultMetricsRecorder(func(context.Context, *storagemodels.MetricRecord) error {
		return nil
	}, "api")
	lambdaCtx = &common.LambdaContext{
		Config:        cfg,
		Logger:        logger,
		Repos:         repoStorage,
		AuthService:   &auth.AuthService{},
		EMFMetrics:    observability.NewEMFMetrics(logger, "Lesser/Test", "api"),
		HealthChecker: &observability.HealthChecker{},
		TracingManager: observability.NewTracingManager(logger, &observability.TracingConfig{
			ServiceName:    "lesser-api",
			ServiceVersion: "test",
			Enabled:        false,
			LocalTesting:   true,
		}),
		MetricsCollector:  &observability.MetricsCollector{},
		LatencyAggregator: observability.NewLatencyAggregator(logger, metricsRecorder),
		LatencyAlerter:    &observability.LatencyAlerter{},
		AWSServices:       &awsinit.AWSServices{CloudWatch: cwClient},
	}

	t.Cleanup(func() {
		if costTrackingService != nil {
			_ = costTrackingService.Close(context.Background())
		}
		if latencyAggregator != nil {
			latencyAggregator.Stop()
		}
	})

	extractStandardizedServices()

	require.NotNil(t, repos)
	require.NotNil(t, authService)
	require.NotNil(t, emfMetrics)
	require.NotNil(t, healthChecker)
	require.NotNil(t, tracingManager)
	require.NotNil(t, latencyAggregator)
	require.NotNil(t, latencyAlerter)
	require.NotNil(t, costTrackingService)
}

func TestInitializeManualServicesRound12(t *testing.T) {
	origNewClient := newLambdaOptimizedClient
	origNewRepoFactory := newRepositoryFactory
	origNewAuthService := newAuthService
	t.Cleanup(func() {
		newLambdaOptimizedClient = origNewClient
		newRepositoryFactory = origNewRepoFactory
		newAuthService = origNewAuthService
	})

	mockDB := new(mocks.MockDB)
	newLambdaOptimizedClient = func(_ context.Context, _ string) (dynamormCore.DB, error) { return mockDB, nil }
	newRepositoryFactory = func(dynamormCore.DB, string, *zap.Logger) (storagecore.RepositoryStorage, error) {
		return newMainTestRepos(t), nil
	}
	newAuthService = func(_ *config.Config, _ auth.StorageProvider) (*auth.AuthService, error) {
		return &auth.AuthService{}, nil
	}

	logger = zap.NewNop()
	cfg = &config.Config{
		Domain:          "example.com",
		Region:          "us-east-1",
		Stage:           "development",
		Version:         "test",
		DynamoTableName: "test-table",
	}

	lambdaCtx = &common.LambdaContext{
		Config: cfg,
		Logger: logger,
		AWSServices: &awsinit.AWSServices{
			Config: aws.Config{Region: "us-east-1"},
		},
	}

	t.Cleanup(func() {
		if latencyAggregator != nil {
			latencyAggregator.Stop()
		}
		if costTrackingService != nil {
			_ = costTrackingService.Close(context.Background())
		}
	})

	initializeManualServices()

	require.NotNil(t, repos)
	require.NotNil(t, authService)
	require.NotNil(t, emfMetrics)
	require.NotNil(t, healthChecker)
	require.NotNil(t, tracingManager)
	require.NotNil(t, latencyAggregator)
	require.NotNil(t, latencyAlerter)
	require.NotNil(t, costTrackingService)
}

func TestInitializeAPISpecificServicesRound12(t *testing.T) {
	origNewLiftHandler := newLiftHandler
	origNewStreamQueue := newStreamQueue
	origCreateAuthMw := createAPIAuthMiddlewareFromAuthService
	t.Cleanup(func() {
		newLiftHandler = origNewLiftHandler
		newStreamQueue = origNewStreamQueue
		createAPIAuthMiddlewareFromAuthService = origCreateAuthMw
	})

	cfg = &config.Config{
		Domain:          "example.com",
		Region:          "us-east-1",
		Stage:           "development",
		DynamoTableName: "test-table",
	}
	logger = zap.NewNop()
	repos = newMainTestRepos(t)
	authService = &auth.AuthService{}

	mockDB := new(mocks.MockDB)
	streamQueue := &streamQueueStub{}

	created := false
	newLiftHandler = func(_ *config.Config, _ storagecore.RepositoryStorage, _ *zap.Logger, _ lift.Middleware, _ streaming.StreamQueueService) *liftapi.Handler {
		created = true
		return &liftapi.Handler{}
	}
	newStreamQueueCalled := false
	newStreamQueue = func(_ dynamormCore.DB, _ string, _ *zap.Logger) streaming.StreamQueueService {
		newStreamQueueCalled = true
		return streamQueue
	}
	createAPIAuthMiddlewareFromAuthService = func(_ *auth.AuthService, _ *zap.Logger) lift.Middleware {
		return func(next lift.Handler) lift.Handler { return next }
	}

	// Uses configured stream queue.
	lambdaCtx = &common.LambdaContext{
		StreamQueue: streamQueue,
		DynamoDB:    mockDB,
	}
	initializeAPISpecificServices()

	require.True(t, created)
	require.NotNil(t, liftHandler)
	require.False(t, newStreamQueueCalled)

	// Falls back to DynamoDB-backed stream queue creation.
	lambdaCtx = &common.LambdaContext{
		DynamoDB: mockDB,
	}
	initializeAPISpecificServices()
	require.True(t, newStreamQueueCalled)
}

func TestConfigureHealthRoutesRound12(t *testing.T) {
	cfg = &config.Config{
		Domain:          "example.com",
		Region:          "us-east-1",
		Version:         "test",
		DynamoTableName: "test-table",
	}
	startTime = time.Now().Add(-1 * time.Hour)

	repos = newMainTestRepos(t)

	app := lift.New()
	configureHealthRoutes(app)

	call := func(path string) *lift.Response {
		event := map[string]any{
			"version":  "2.0",
			"routeKey": "GET " + path,
			"requestContext": map[string]any{
				"requestId": "test-request-id",
				"http": map[string]any{
					"method": "GET",
					"path":   path,
				},
			},
		}
		result, err := app.HandleRequest(context.Background(), event)
		require.NoError(t, err)
		resp, ok := result.(*lift.Response)
		require.True(t, ok)
		return resp
	}

	require.Equal(t, 200, call("/health/live").StatusCode)
	require.Equal(t, 200, call("/health").StatusCode)
	require.Equal(t, 200, call("/health/ready").StatusCode)
	require.Equal(t, 200, call("/health/detailed").StatusCode)

	// Now exercise a failing DB dependency path.
	errorRepos := func(t *testing.T) *mainTestRepos {
		t.Helper()

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Maybe()

		logger := zap.NewNop()
		account := repositories.NewAccountRepository(mockDB, "test-table", "example.com", logger)
		metric := repositories.NewMetricRecordRepository(mockDB, "test-table", logger, nil)
		return &mainTestRepos{
			MockRepositoryStorage: &liftapi.MockRepositoryStorage{},
			account:               account,
			metricRecord:          metric,
		}
	}

	repos = errorRepos(t)
	require.Equal(t, 503, call("/health/ready").StatusCode)
	require.Equal(t, 503, call("/health/detailed").StatusCode)
}

func TestMain_HealthEndpointsUseAppTheoryRound12(t *testing.T) {
	origLambdaStart := lambdaStart
	origRoutes := configureLiftRoutesFn
	origLock := createInstanceLockMiddlewareFn
	origAuthRoutes := configureAPIRoutesAppTheoryFn
	origHealth := healthChecker
	t.Cleanup(func() {
		lambdaStart = origLambdaStart
		configureLiftRoutesFn = origRoutes
		createInstanceLockMiddlewareFn = origLock
		configureAPIRoutesAppTheoryFn = origAuthRoutes
		healthChecker = origHealth
	})

	cfg = &config.Config{
		Domain:          "example.com",
		Region:          "us-east-1",
		Stage:           "development",
		Version:         "test",
		DynamoTableName: "test-table",
		DebugMode:       true,
	}
	logger = zap.NewNop()
	lambdaCtx = &common.LambdaContext{Logger: logger}
	repos = newMainTestRepos(t)
	startTime = time.Now().Add(-1 * time.Hour)
	healthChecker = &observability.HealthChecker{}

	createInstanceLockMiddlewareFn = func(_ storagecore.RepositoryStorage, _ *zap.Logger) lift.Middleware {
		return func(next lift.Handler) lift.Handler { return next }
	}
	configureLiftRoutesFn = func(_ *lift.App) {}
	configureAPIRoutesAppTheoryFn = func(_ *apptheory.App) {}

	var captured any
	lambdaStart = func(h any) { captured = h }

	main()

	handler, ok := captured.(func(context.Context, interface{}) (interface{}, error))
	require.True(t, ok)

	call := func(path string) events.APIGatewayV2HTTPResponse {
		respAny, err := handler(context.Background(), map[string]any{
			"version":  "2.0",
			"routeKey": "GET " + path,
			"requestContext": map[string]any{
				"requestId": "test-request-id",
				"http": map[string]any{
					"method": "GET",
					"path":   path,
				},
			},
		})
		require.NoError(t, err)
		resp, ok := respAny.(events.APIGatewayV2HTTPResponse)
		require.True(t, ok)
		return resp
	}

	require.Equal(t, 200, call("/health/live").StatusCode)
	require.Equal(t, 200, call("/health").StatusCode)
	require.Equal(t, 200, call("/health/ready").StatusCode)
	require.Equal(t, 200, call("/health/detailed").StatusCode)

	// Now exercise a failing DB dependency path.
	errorRepos := func(t *testing.T) *mainTestRepos {
		t.Helper()

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Maybe()

		logger := zap.NewNop()
		account := repositories.NewAccountRepository(mockDB, "test-table", "example.com", logger)
		metric := repositories.NewMetricRecordRepository(mockDB, "test-table", logger, nil)
		return &mainTestRepos{
			MockRepositoryStorage: &liftapi.MockRepositoryStorage{},
			account:               account,
			metricRecord:          metric,
		}
	}

	repos = errorRepos(t)
	require.Equal(t, 503, call("/health/ready").StatusCode)
	require.Equal(t, 503, call("/health/detailed").StatusCode)
}

func TestTracingAndMetricsMiddlewareRound12(t *testing.T) {
	origTracing := tracingManager
	origEMF := emfMetrics
	t.Cleanup(func() {
		tracingManager = origTracing
		emfMetrics = origEMF
	})

	// Tracing: disabled path.
	tracingManager = nil
	mw := createTracingMiddleware()
	ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
	require.NoError(t, mw(lift.HandlerFunc(func(*lift.Context) error { return nil })).Handle(ctx))

	// Tracing: enabled path (exercise header parsing + error path).
	tracingManager = observability.NewTracingManager(zap.NewNop(), &observability.TracingConfig{
		ServiceName:    "lesser-api",
		ServiceVersion: "test",
		SamplingRate:   1.0,
		Enabled:        true,
		LocalTesting:   false,
	})
	mw = createTracingMiddleware()
	ctx = lift.NewContext(context.Background(), nil)
	require.NoError(t, mw(lift.HandlerFunc(func(*lift.Context) error { return nil })).Handle(ctx))

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method: "POST",
		Path:   "/trace",
		Headers: map[string]string{
			"User-Agent":      "test-agent",
			"X-Forwarded-For": "203.0.113.10",
		},
	}))
	require.Error(t, mw(lift.HandlerFunc(func(*lift.Context) error { return errors.New("boom") })).Handle(ctx))

	// EMF: recordEMFMetrics no-op when disabled.
	emfMetrics = nil
	recordEMFMetrics(requestInfo{method: "GET", path: "/"}, 200, 5*time.Millisecond, nil)

	// EMF: recordEMFMetrics error + success paths (drives determineErrorType branches).
	emfMetrics = observability.NewEMFMetrics(zap.NewNop(), "Lesser/Test", "api")
	recordEMFMetrics(requestInfo{method: "GET", path: "/auth", endpoint: "GET /auth"}, 401, 5*time.Millisecond, errors.New("unauth"))
	recordEMFMetrics(requestInfo{method: "GET", path: "/ok", endpoint: "GET /ok"}, 200, 5*time.Millisecond, nil)

	// EMF middleware: early return when EMF disabled and when request is missing.
	emfMetrics = nil
	emfMW := createEMFMetricsMiddleware()
	require.NoError(t, emfMW(lift.HandlerFunc(func(*lift.Context) error { return nil })).Handle(lift.NewContext(context.Background(), lift.NewRequest(nil))))

	emfMetrics = observability.NewEMFMetrics(zap.NewNop(), "Lesser/Test", "api")
	emfMW = createEMFMetricsMiddleware()
	require.NoError(t, emfMW(lift.HandlerFunc(func(*lift.Context) error { return nil })).Handle(lift.NewContext(context.Background(), nil)))
	require.Error(t, emfMW(lift.HandlerFunc(func(*lift.Context) error { return errors.New("boom") })).Handle(lift.NewContext(context.Background(), lift.NewRequest(nil))))
}

func TestExtractRequestInfoRound12_NilRequest(t *testing.T) {
	info := extractRequestInfo(&lift.Context{})
	require.Equal(t, "GET", info.method)
	require.Equal(t, "/", info.path)
	require.Equal(t, "GET /", info.endpoint)

	info = extractRequestInfo(lift.NewContext(context.Background(), &lift.Request{}))
	require.Equal(t, "GET", info.method)
	require.Equal(t, "/", info.path)
	require.Equal(t, "GET /", info.endpoint)
}

func TestDetermineErrorTypeRound12(t *testing.T) {
	require.Equal(t, observability.ErrorTypeInternal, determineErrorType(200))
	require.Equal(t, observability.ErrorTypeInternal, determineErrorType(500))
	require.Equal(t, observability.ErrorTypeAuthentication, determineErrorType(401))
	require.Equal(t, observability.ErrorTypeAuthorization, determineErrorType(403))
	require.Equal(t, observability.ErrorTypeNotFound, determineErrorType(404))
	require.Equal(t, observability.ErrorTypeConflict, determineErrorType(409))
	require.Equal(t, observability.ErrorTypeRateLimit, determineErrorType(429))
	require.Equal(t, observability.ErrorTypeValidation, determineErrorType(422))
}

func TestCreateCentralizedCostTrackingMiddlewareRound12(t *testing.T) {
	logger = zap.NewNop()

	mw := createCentralizedCostTrackingMiddleware()
	ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
	require.NoError(t, mw(lift.HandlerFunc(func(*lift.Context) error { return nil })).Handle(ctx))

	costTrackingService = cost.NewTrackingService(nil, logger, cost.DefaultTrackingServiceConfig())
	t.Cleanup(func() { _ = costTrackingService.Close(context.Background()) })

	origTrack := trackLambdaInvocation
	t.Cleanup(func() { trackLambdaInvocation = origTrack })

	done := make(chan struct{}, 1)
	trackLambdaInvocation = func(_ context.Context, _ *cost.TrackingService, _ cost.LambdaOperation) error {
		done <- struct{}{}
		return nil
	}

	require.NoError(t, os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "256"))
	t.Cleanup(func() { _ = os.Unsetenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE") })

	mw = createCentralizedCostTrackingMiddleware()
	ctx = lift.NewContext(context.Background(), lift.NewRequest(nil))
	require.NoError(t, mw(lift.HandlerFunc(func(*lift.Context) error { return nil })).Handle(ctx))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cost tracking")
	}
}

func TestMainRound12(t *testing.T) {
	origLambdaStart := lambdaStart
	origRoutes := configureLiftRoutesFn
	origLock := createInstanceLockMiddlewareFn
	origAuthRoutes := configureAPIRoutesAppTheoryFn
	t.Cleanup(func() {
		lambdaStart = origLambdaStart
		configureLiftRoutesFn = origRoutes
		createInstanceLockMiddlewareFn = origLock
		configureAPIRoutesAppTheoryFn = origAuthRoutes
	})

	cfg = &config.Config{
		Domain:          "example.com",
		Region:          "us-east-1",
		Stage:           "development",
		Version:         "test",
		DynamoTableName: "test-table",
		DebugMode:       true,
	}
	logger = zap.NewNop()
	lambdaCtx = &common.LambdaContext{Logger: logger}
	repos = newMainTestRepos(t)
	startTime = time.Now()
	emfMetrics = observability.NewEMFMetrics(logger, "Lesser/Test", "api")

	createInstanceLockMiddlewareFn = func(_ storagecore.RepositoryStorage, _ *zap.Logger) lift.Middleware {
		return func(next lift.Handler) lift.Handler { return next }
	}
	configureLiftRoutesFn = func(app *lift.App) {
		_ = app.GET("/ping", func(ctx *lift.Context) error { return ctx.Status(200).JSON(map[string]string{"ok": "true"}) })
	}
	configureAPIRoutesAppTheoryFn = func(_ *apptheory.App) {}

	var captured any
	lambdaStart = func(h any) { captured = h }

	main()

	handler, ok := captured.(func(context.Context, interface{}) (interface{}, error))
	require.True(t, ok)

	respAny, err := handler(context.Background(), map[string]any{
		"version":  "2.0",
		"routeKey": "GET /ping",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
			"http": map[string]any{
				"method": "GET",
				"path":   "/ping",
			},
		},
	})
	require.NoError(t, err)

	resp, ok := respAny.(*lift.Response)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)
}
