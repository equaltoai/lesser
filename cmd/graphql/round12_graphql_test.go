package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pay-theory/dynamorm"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/streaming"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
)

type fakeStreamQueue struct{}

func (f *fakeStreamQueue) QueueEventForUser(context.Context, string, string, map[string]interface{}) error {
	return nil
}
func (f *fakeStreamQueue) QueueEventForStream(context.Context, string, string, map[string]interface{}) error {
	return nil
}
func (f *fakeStreamQueue) QueueEventForConversation(context.Context, string, string, map[string]interface{}) error {
	return nil
}
func (f *fakeStreamQueue) QueueEventForFollowers(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func TestOAuthMiddlewareAdapter_ValidateAccessToken_Round12(t *testing.T) {
	originalLogger := logger
	t.Cleanup(func() { logger = originalLogger })
	logger = zaptest.NewLogger(t)

	secret := "secret"
	oauthService := auth.NewOAuthService(secret, &config.Config{}, nil, nil)
	adapter := &oauthMiddlewareAdapter{service: oauthService}

	claims := &auth.Claims{
		Username: "alice",
		Scopes:   []string{"read"},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)

	got, err := adapter.ValidateAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, "alice", got.GetUsername())

	_, err = adapter.ValidateAccessToken("not-a-token")
	require.Error(t, err)
}

func TestGraphQLResponseWriter_AndBytesReader_Round12(t *testing.T) {
	req := lift.NewRequest(nil)
	req.Method = http.MethodPost
	req.Path = "/graphql"
	ctx := lift.NewContext(context.Background(), req)

	w := &graphQLResponseWriter{
		liftCtx: ctx,
		header:  make(http.Header),
	}

	w.Header().Set("X-Test", "1")
	_, err := w.Write([]byte("body"))
	require.NoError(t, err)
	require.Equal(t, "1", ctx.Response.Headers["X-Test"])
	require.Equal(t, 200, ctx.Response.StatusCode)
	require.Equal(t, "body", ctx.Response.Body)

	w2 := &graphQLResponseWriter{
		liftCtx: ctx,
		header:  make(http.Header),
	}
	w2.WriteHeader(201)
	_, err = w2.Write([]byte("created"))
	require.NoError(t, err)
	require.Equal(t, 201, ctx.Response.StatusCode)
	require.Equal(t, "created", ctx.Response.Body)

	r := &bytesReader{data: []byte("abc")}
	buf := make([]byte, 2)
	n, err := r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, "ab", string(buf[:n]))
	n, err = r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "c", string(buf[:n]))
	_, err = r.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, r.Close())
}

func TestCreateMiddlewares_Round12(t *testing.T) {
	originalRepos := repos
	originalLogger := logger
	originalCfg := cfg
	originalCostTracker := costTracker
	originalCostSvc := costTrackingService
	t.Cleanup(func() {
		repos = originalRepos
		logger = originalLogger
		cfg = originalCfg
		costTracker = originalCostTracker
		costTrackingService = originalCostSvc
	})

	logger = zap.NewNop()
	repos = nil
	cfg = &config.Config{JWTSecret: "secret"}
	costTracker = cost.New()
	costTrackingService = nil

	t.Run("data_loader_sets_loaders", func(t *testing.T) {
		mw := createDataLoaderMiddleware()
		called := false
		next := lift.HandlerFunc(func(ctx *lift.Context) error {
			called = true
			val := ctx.Get("loaders")
			_, ok := val.(*graph.Loaders)
			require.True(t, ok)
			return nil
		})
		ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
		require.NoError(t, mw(next).Handle(ctx))
		require.True(t, called)
	})

	t.Run("cost_tracking_sets_tracker", func(t *testing.T) {
		mw := createCostTrackingMiddleware()
		next := lift.HandlerFunc(func(ctx *lift.Context) error {
			require.Same(t, costTracker, ctx.Get("cost_tracker"))
			return nil
		})
		ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
		ctx.Request.Method = http.MethodGet
		ctx.Request.Path = "/graphql"
		require.NoError(t, mw(next).Handle(ctx))
	})

	t.Run("auth_middleware_constructs", func(t *testing.T) {
		mw := createAuthMiddleware()
		require.NotNil(t, mw)
	})
}

func TestInitializeGraphQL_Branches_Round12(t *testing.T) {
	originalRunningUnitTests := runningUnitTestsFn
	originalMustInitialize := mustInitializeLambdaFn
	originalInitializeWithDefaults := initializeWithDefaultsFn
	originalExtract := extractStandardizedServicesFn
	originalManual := initializeManualServicesFn
	originalSpecific := initializeGraphQLSpecificServicesFn
	originalLambdaCtx := lambdaCtx
	originalInitTime := initTime

	t.Cleanup(func() {
		runningUnitTestsFn = originalRunningUnitTests
		mustInitializeLambdaFn = originalMustInitialize
		initializeWithDefaultsFn = originalInitializeWithDefaults
		extractStandardizedServicesFn = originalExtract
		initializeManualServicesFn = originalManual
		initializeGraphQLSpecificServicesFn = originalSpecific
		lambdaCtx = originalLambdaCtx
		initTime = originalInitTime
	})

	runningUnitTestsFn = func() bool { return false }

	var mustInitCalls int
	mustInitializeLambdaFn = func(cfg common.LambdaConfig) *common.LambdaContext {
		mustInitCalls++
		require.Equal(t, "graphql", cfg.ServiceName)
		require.Equal(t, common.LambdaTypeAPI, cfg.LambdaType)
		require.Equal(t, 30*time.Second, cfg.RequestTimeout)
		return &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &config.Config{},
		}
	}

	t.Run("defaults_success_uses_standardized_extraction", func(t *testing.T) {
		var extracted, manual, specific int
		initializeWithDefaultsFn = func(*common.LambdaContext) error { return nil }
		extractStandardizedServicesFn = func() { extracted++ }
		initializeManualServicesFn = func() { manual++ }
		initializeGraphQLSpecificServicesFn = func() { specific++ }

		initializeGraphQL()
		require.Equal(t, 1, mustInitCalls)
		require.Equal(t, 1, extracted)
		require.Equal(t, 0, manual)
		require.Equal(t, 1, specific)
		require.False(t, initTime.IsZero())
		require.NotNil(t, lambdaCtx)
	})

	t.Run("defaults_error_falls_back_to_manual_init", func(t *testing.T) {
		var extracted, manual, specific int
		initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
		extractStandardizedServicesFn = func() { extracted++ }
		initializeManualServicesFn = func() { manual++ }
		initializeGraphQLSpecificServicesFn = func() { specific++ }

		initializeGraphQL()
		require.Equal(t, 2, mustInitCalls)
		require.Equal(t, 0, extracted)
		require.Equal(t, 1, manual)
		require.Equal(t, 1, specific)
	})

	t.Run("on_start_respects_running_unit_tests_flag", func(t *testing.T) {
		mustInitCalls = 0
		initializeWithDefaultsFn = func(*common.LambdaContext) error { return nil }
		extractStandardizedServicesFn = func() {}
		initializeManualServicesFn = func() {}
		initializeGraphQLSpecificServicesFn = func() {}

		runningUnitTestsFn = func() bool { return false }
		initializeGraphQLOnStart()
		require.Equal(t, 1, mustInitCalls)

		mustInitCalls = 0
		runningUnitTestsFn = func() bool { return true }
		initializeGraphQLOnStart()
		require.Equal(t, 0, mustInitCalls)
	})
}

type fakeInvocationTracker struct {
	called chan cost.LambdaOperation
}

func (f *fakeInvocationTracker) TrackLambdaInvocation(_ context.Context, op cost.LambdaOperation) error {
	f.called <- op
	return nil
}

func TestCreateCostTrackingMiddleware_CentralizedTrackingBranch_Round12(t *testing.T) {
	originalLogger := logger
	originalTracker := costTracker
	originalService := costTrackingService
	originalEnv := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
	t.Cleanup(func() {
		logger = originalLogger
		costTracker = originalTracker
		costTrackingService = originalService
		_ = os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", originalEnv)
	})

	logger = zap.NewNop()
	costTracker = cost.New()
	inv := &fakeInvocationTracker{called: make(chan cost.LambdaOperation, 1)}
	costTrackingService = inv
	require.NoError(t, os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "256"))

	mw := createCostTrackingMiddleware()
	next := lift.HandlerFunc(func(ctx *lift.Context) error {
		ctx.Request.Method = http.MethodGet
		ctx.Request.Path = "/ready"
		return nil
	})

	ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
	ctx.Request.Method = http.MethodGet
	ctx.Request.Path = "/ready"
	require.NoError(t, mw(next).Handle(ctx))

	select {
	case op := <-inv.called:
		require.Equal(t, "graphql", op.FunctionName)
		require.Equal(t, int64(256), op.MemoryMB)
	case <-time.After(time.Second):
		t.Fatal("expected TrackLambdaInvocation to be called")
	}
}

func TestHandlePlayground_AndHandleGraphQL_Round12(t *testing.T) {
	originalCfg := cfg
	originalLogger := logger
	originalHandler := graphQLHandler
	t.Cleanup(func() {
		cfg = originalCfg
		logger = originalLogger
		graphQLHandler = originalHandler
	})

	logger = zap.NewNop()
	cfg = &config.Config{EnablePlayground: false}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
	ctx.Request.Method = http.MethodGet
	ctx.Request.Path = "/playground"
	err := handlePlayground(ctx)
	require.Error(t, err)

	cfg.EnablePlayground = true
	ctx = lift.NewContext(context.Background(), lift.NewRequest(nil))
	ctx.Request.Method = http.MethodGet
	ctx.Request.Path = "/playground"
	require.NoError(t, handlePlayground(ctx))
	require.Contains(t, ctx.Response.Body, "graphiql")

	costTracker = cost.New()
	fakeClaims := &auth.Claims{Username: "claims-user", Scopes: []string{"read"}}
	graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		require.Equal(t, `{"query":"{ me { id } }"}`, string(body))
		require.Equal(t, "claims-user", r.Context().Value(contextKeyUser).(string))
		claimsVal := r.Context().Value(common.ContextKeyClaims)
		claims, ok := claimsVal.(common.Claims)
		require.True(t, ok)
		require.Equal(t, "claims-user", claims.GetUsername())
		require.Equal(t, "ct", r.Context().Value(contextKeyCostTracker))
		w.Header().Set("X-GraphQL", "ok")
		w.WriteHeader(202)
		_, _ = w.Write([]byte("graphql-ok"))
	})

	ctx = lift.NewContext(context.Background(), lift.NewRequest(nil))
	ctx.Request.Method = http.MethodPost
	ctx.Request.Path = "/graphql"
	ctx.Request.Body = []byte(`{"query":"{ me { id } }"}`)
	ctx.Request.Headers["Authorization"] = "Bearer test"
	ctx.Set("user", "ignored")
	ctx.Set("username", "ignored-too")
	ctx.Set("claims", fakeClaims)
	ctx.Set("cost_tracker", "ct")
	ctx.Set("loaders", &graph.Loaders{})
	require.NoError(t, handleGraphQL(ctx))
	require.Equal(t, "ok", ctx.Response.Headers["X-Graphql"])
	require.Equal(t, 202, ctx.Response.StatusCode)
	require.Equal(t, "graphql-ok", ctx.Response.Body)
}

func TestHandleGraphQL_AdditionalBranches_Round12(t *testing.T) {
	originalLogger := logger
	originalHandler := graphQLHandler
	t.Cleanup(func() {
		logger = originalLogger
		graphQLHandler = originalHandler
	})

	logger = zap.NewNop()

	t.Run("claims_wrong_type_and_loaders_wrong_type", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "bad-loaders", r.Context().Value(contextKeyLoaders))
			require.Nil(t, r.Context().Value(common.ContextKeyClaims))
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
		ctx.Request.Method = http.MethodGet
		ctx.Request.Path = "/graphql"
		ctx.Request.Body = []byte(`{}`)
		ctx.Set("claims", "not-claims")
		ctx.Set("loaders", "bad-loaders")
		require.NoError(t, handleGraphQL(ctx))
		require.Equal(t, 200, ctx.Response.StatusCode)
	})

	t.Run("claims_empty_username", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claimsVal := r.Context().Value(common.ContextKeyClaims)
			claims, ok := claimsVal.(common.Claims)
			require.True(t, ok)
			require.Equal(t, "", claims.GetUsername())
			require.Nil(t, r.Context().Value(contextKeyUser))
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
		ctx.Request.Method = http.MethodGet
		ctx.Request.Path = "/graphql"
		ctx.Request.Body = []byte(`{}`)
		ctx.Set("claims", &auth.Claims{})
		require.NoError(t, handleGraphQL(ctx))
	})

	t.Run("no_claims_no_loaders", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := lift.NewContext(context.Background(), lift.NewRequest(nil))
		ctx.Request.Method = http.MethodGet
		ctx.Request.Path = "/graphql"
		ctx.Request.Body = []byte(`{}`)
		require.NoError(t, handleGraphQL(ctx))
	})
}

func TestExtractStandardizedServices_AndResolveStreamQueue_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalCostTracker := costTracker
	originalCostSvc := costTrackingService
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		costTracker = originalCostTracker
		costTrackingService = originalCostSvc
	})

	mockStorage := &testingmocks.MockRepositoryStorage{}
	lambdaCtx = &common.LambdaContext{
		Config: &config.Config{JWTSecret: "secret"},
		Logger: zap.NewNop(),
		Repos:  mockStorage,
		AWSServices: &awsinit.AWSServices{
			Config: aws.Config{Region: "us-east-1"},
		},
		StreamQueue: streaming.StreamQueueService(&fakeStreamQueue{}),
	}

	extractStandardizedServices()
	require.Same(t, mockStorage, repos.(*testingmocks.MockRepositoryStorage))
	require.NotNil(t, costTracker)
	require.NotNil(t, logger)

	got := resolveStreamQueue()
	require.NotNil(t, got)
}

func TestExtractStandardizedServices_InitializesCentralizedCostTracking_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalLogger := logger
	originalRepos := repos
	originalCostSvc := costTrackingService
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		logger = originalLogger
		repos = originalRepos
		costTrackingService = originalCostSvc
	})

	logger = zap.NewNop()
	mockStorage := &testingmocks.MockRepositoryStorage{}
	cw := cloudwatch.NewFromConfig(aws.Config{Region: "us-east-1"})
	lambdaCtx = &common.LambdaContext{
		Config: &config.Config{JWTSecret: "secret"},
		Logger: logger,
		Repos:  mockStorage,
		AWSServices: &awsinit.AWSServices{
			CloudWatch: cw,
		},
	}

	extractStandardizedServices()
	require.NotNil(t, costTrackingService)
	if svc, ok := costTrackingService.(*cost.TrackingService); ok {
		_ = svc.Close(context.Background())
	}
}

func TestInitializeManualServices_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	originalNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
		newRepositoryFactoryFn = originalNewFactory
	})

	lambdaCtx = &common.LambdaContext{
		Logger: zap.NewNop(),
		Config: &config.Config{
			Region:          "us-east-1",
			DynamoTableName: "",
		},
	}

	var gotTable string
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &dynamorm.LambdaDB{}, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, tableName string, _ *zap.Logger) (storagecore.RepositoryStorage, error) {
		gotTable = tableName
		return &testingmocks.MockRepositoryStorage{}, nil
	}

	initializeManualServices()
	require.NotNil(t, repos)
	require.NotNil(t, lambdaCtx.Repos)
	require.Equal(t, "lesser-main", gotTable)
}

func TestInitializeManualServices_UsesConfiguredTableName_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	originalNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
		newRepositoryFactoryFn = originalNewFactory
	})

	lambdaCtx = &common.LambdaContext{
		Logger: zap.NewNop(),
		Config: &config.Config{
			Region:          "us-east-1",
			DynamoTableName: "custom-table",
		},
	}

	var gotTable string
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &dynamorm.LambdaDB{}, nil }
	newRepositoryFactoryFn = func(_ dynamormCore.DB, tableName string, _ *zap.Logger) (storagecore.RepositoryStorage, error) {
		gotTable = tableName
		return &testingmocks.MockRepositoryStorage{}, nil
	}

	initializeManualServices()
	require.NotNil(t, repos)
	require.Equal(t, "custom-table", gotTable)
}

func TestResolveStreamQueue_FallbackError_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
	})

	logger = zap.NewNop()
	cfg = &config.Config{Region: "us-east-1"}
	repos = nil
	lambdaCtx = &common.LambdaContext{
		StreamQueue: "not-a-queue",
	}

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return nil, errors.New("boom") }
	require.Nil(t, resolveStreamQueue())
}

func TestResolveStreamQueue_UsesLambdaContextDynamoDB_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
	})

	logger = zap.NewNop()
	cfg = &config.Config{Region: "us-east-1", DynamoTableName: "tbl"}
	repos = nil
	lambdaCtx = &common.LambdaContext{DynamoDB: &dynamorm.LambdaDB{}}
	require.NotNil(t, resolveStreamQueue())
}

func TestResolveStreamQueue_UsesReposGetDB_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
	})

	logger = zap.NewNop()
	cfg = &config.Config{Region: "us-east-1", DynamoTableName: "tbl"}

	mockStorage := &testingmocks.MockRepositoryStorage{}
	mockStorage.On("GetDB").Return(&dynamorm.LambdaDB{})
	repos = mockStorage
	lambdaCtx = &common.LambdaContext{}

	require.NotNil(t, resolveStreamQueue())
	mockStorage.AssertExpectations(t)
}

func TestResolveStreamQueue_CreatesClientWhenNoDB_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		newLambdaOptimizedClientFn = originalNewClient
	})

	logger = zap.NewNop()
	cfg = &config.Config{Region: "us-east-1", DynamoTableName: "tbl"}
	repos = nil
	lambdaCtx = &common.LambdaContext{}
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) { return &dynamorm.LambdaDB{}, nil }

	require.NotNil(t, resolveStreamQueue())
}

func TestInitializeGraphQLSpecificServices_Round12(t *testing.T) {
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalLambdaCtx := lambdaCtx
	originalHandler := graphQLHandler
	originalCostTracker := costTracker
	t.Cleanup(func() {
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		lambdaCtx = originalLambdaCtx
		graphQLHandler = originalHandler
		costTracker = originalCostTracker
	})

	logger = zap.NewNop()
	cfg = &config.Config{
		Domain:          "example.com",
		JWTSecret:       "secret",
		DisableAI:       true,
		DynamoTableName: "tbl",
		S3BucketName:    "bucket",
		MaxUploadSize:   1024,
	}
	repos = &testingmocks.MockRepositoryStorage{}
	costTracker = cost.New()
	lambdaCtx = &common.LambdaContext{
		Config: cfg,
		Logger: logger,
		AWSServices: &awsinit.AWSServices{
			Config: aws.Config{Region: "us-east-1"},
		},
		Repos:       repos,
		StreamQueue: streaming.StreamQueueService(&fakeStreamQueue{}),
	}

	initializeGraphQLSpecificServices()
	require.NotNil(t, graphQLHandler)

	// Cover AI-enabled and debug tracing branches
	graphQLHandler = nil
	cfg.DisableAI = false
	cfg.DebugMode = true
	initializeGraphQLSpecificServices()
	require.NotNil(t, graphQLHandler)
}

func TestMain_RegistersAndStartsLambda_Round12(t *testing.T) {
	originalCfg := cfg
	originalLogger := logger
	originalRepos := repos
	originalLambdaCtx := lambdaCtx
	originalStart := lambdaStartFn
	originalGraphQLHandler := graphQLHandler
	t.Cleanup(func() {
		cfg = originalCfg
		logger = originalLogger
		repos = originalRepos
		lambdaCtx = originalLambdaCtx
		lambdaStartFn = originalStart
		graphQLHandler = originalGraphQLHandler
	})

	cfg = &config.Config{
		Domain:          "example.com",
		JWTSecret:       "secret",
		DisableAI:       true,
		DynamoTableName: "tbl",
		S3BucketName:    "bucket",
		MaxUploadSize:   1024,
		DebugMode:       false,
	}
	logger = zap.NewNop()
	repos = &testingmocks.MockRepositoryStorage{}
	costTracker = cost.New()
	lambdaCtx = &common.LambdaContext{
		Config:    cfg,
		Logger:    logger,
		StartTime: time.Now().Add(-time.Hour),
	}

	var started any
	lambdaStartFn = func(h any) { started = h }
	main()
	require.NotNil(t, started)

	h, ok := started.(func(context.Context, interface{}) (interface{}, error))
	require.True(t, ok)
	event := map[string]any{
		"version":  "2.0",
		"routeKey": "GET /ready",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
			"http": map[string]any{
				"method": "GET",
				"path":   "/ready",
			},
			"stage": "$default",
		},
	}
	result, err := h(context.Background(), event)
	require.NoError(t, err)
	resp, ok := result.(*lift.Response)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)

	result, err = h(context.Background(), map[string]any{
		"version":  "2.0",
		"routeKey": "GET /health",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
			"http": map[string]any{
				"method": "GET",
				"path":   "/health",
			},
			"stage": "$default",
		},
	})
	require.NoError(t, err)
	resp, ok = result.(*lift.Response)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)

	result, err = h(context.Background(), map[string]any{
		"version":  "2.0",
		"routeKey": "OPTIONS /graphql",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
			"http": map[string]any{
				"method": "OPTIONS",
				"path":   "/graphql",
			},
			"stage": "$default",
		},
	})
	require.NoError(t, err)
	resp, ok = result.(*lift.Response)
	require.True(t, ok)
	require.Equal(t, 204, resp.StatusCode)

	graphQLHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	result, err = h(context.Background(), map[string]any{
		"version":  "2.0",
		"routeKey": "GET /graphql",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
			"http": map[string]any{
				"method": "GET",
				"path":   "/graphql",
			},
			"stage": "$default",
		},
	})
	require.NoError(t, err)
	resp, ok = result.(*lift.Response)
	require.True(t, ok)
	require.Equal(t, 500, resp.StatusCode)

	started = nil
	cfg.DebugMode = true
	main()
	require.NotNil(t, started)
}
