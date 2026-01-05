package routing

import (
	"context"
	"encoding/json"
	stdliberrors "errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/monitoring"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInboxInit_Round10_ExtractServicesFromContext_AllFields(t *testing.T) {
	env := newInboxTestEnv(t)

	metrics := env.handler.emfMetrics
	if metrics == nil {
		metrics = observability.NewEMFMetrics(env.logger, "Lesser/Test", "inbox")
	}

	alertManager := env.handler.alertManager
	if alertManager == nil {
		alertManager = monitoring.NewAlertManagerWithConfig(&monitoring.AlertManagerConfig{
			Logger:  env.logger,
			Enabled: true,
		})
	}

	lambdaCtx := &common.LambdaContext{
		Repos:            env.handler.storageAdapter,
		DynamoDB:         env.mockDB,
		SignatureService: env.handler.signatureService,
		DeliveryService:  env.handler.deliveryService,
		CostCalculator:   env.handler.costCalculator,
		RateLimiter:      env.handler.rateLimiter,
		AuthMiddleware:   env.handler.authMiddleware,
		EMFMetrics:       metrics,
		AlertManager:     alertManager,
	}

	services := extractServicesFromContext(lambdaCtx)
	require.NotNil(t, services.repoFactory)
	require.NotNil(t, services.db)
	require.NotNil(t, services.signatureService)
	require.NotNil(t, services.deliveryService)
	require.NotNil(t, services.costCalculator)
	require.NotNil(t, services.rateLimiter)
	require.NotNil(t, services.authMiddleware)
	require.NotNil(t, services.emfMetrics)
	require.NotNil(t, services.alertManager)
}

func TestInboxInit_Round10_InitializeStorage_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	repoFactory, coreDB, err := initializeStorage(env.handler.storageAdapter, env.mockDB, env.cfg, nil, env.logger)
	require.NoError(t, err)
	require.NotNil(t, repoFactory)
	require.NotNil(t, coreDB)

	_, _, err = initializeStorage(env.handler.storageAdapter, "not-a-db", env.cfg, nil, env.logger)
	require.Error(t, err)

	previousGet := getDynamormClient
	previousFactory := newRepositoryFactory
	t.Cleanup(func() {
		getDynamormClient = previousGet
		newRepositoryFactory = previousFactory
	})

	getDynamormClient = func(_ context.Context) (dynamormCore.DB, error) {
		return nil, stdliberrors.New("boom")
	}
	_, _, err = initializeStorage(nil, nil, env.cfg, nil, env.logger)
	require.Error(t, err)

	getDynamormClient = func(_ context.Context) (dynamormCore.DB, error) {
		return env.mockDB, nil
	}
	newRepositoryFactory = func(_ dynamormCore.DB, _ string, _ *zap.Logger) (*factory.RepositoryFactory, error) {
		return nil, stdliberrors.New("boom")
	}
	_, _, err = initializeStorage(nil, nil, env.cfg, nil, env.logger)
	require.Error(t, err)

	expectedFactory := env.handler.storageAdapter.(*factory.RepositoryFactory)
	newRepositoryFactory = func(_ dynamormCore.DB, _ string, _ *zap.Logger) (*factory.RepositoryFactory, error) {
		return expectedFactory, nil
	}
	repoFactory, coreDB, err = initializeStorage(nil, nil, env.cfg, nil, env.logger)
	require.NoError(t, err)
	require.Equal(t, expectedFactory, repoFactory)
	require.Equal(t, env.mockDB, coreDB)
}

func TestInboxMain_Round10_InitializeLambdaCtxFn_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	lambdaCtx := &common.LambdaContext{
		Config: env.cfg,
		Logger: env.logger,
	}

	previousMustInit := mustInitializeLambda
	previousWithOptions := initializeWithOptions
	t.Cleanup(func() {
		mustInitializeLambda = previousMustInit
		initializeWithOptions = previousWithOptions
	})

	mustInitializeLambda = func(_ common.LambdaConfig) *common.LambdaContext { return lambdaCtx }
	initializeWithOptions = func(_ *common.LambdaContext, _ common.LambdaInitOptions) error {
		return stdliberrors.New("init failed")
	}
	require.Equal(t, lambdaCtx, initializeLambdaCtxFn(common.LambdaConfig{ServiceName: "inbox", LambdaType: common.LambdaTypeFederation}))

	initializeWithOptions = func(_ *common.LambdaContext, _ common.LambdaInitOptions) error { return nil }
	require.Equal(t, lambdaCtx, initializeLambdaCtxFn(common.LambdaConfig{ServiceName: "inbox", LambdaType: common.LambdaTypeFederation}))
}

func TestInboxMain_Round10_MainWiring(t *testing.T) {
	env := newInboxTestEnv(t)

	previousInit := initializeLambdaCtxFn
	previousStart := startLambda
	t.Cleanup(func() {
		initializeLambdaCtxFn = previousInit
		startLambda = previousStart
	})

	initializeLambdaCtxFn = func(_ common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:         env.cfg,
			Logger:         env.logger,
			DynamoDB:       env.mockDB,
			Repos:          env.handler.storageAdapter,
			AuthMiddleware: auth.NewMiddleware(),
		}
	}
	startLambda = func(handler interface{}) {
		if h, ok := handler.(func(context.Context, interface{}) (interface{}, error)); ok {
			_, _ = h(context.Background(), map[string]any{})
		}
	}

	require.NotPanics(t, func() { Run() })
}

func TestInboxInit_Round10_InitializeObservabilityServices_CentralCostService(t *testing.T) {
	logger := zap.NewNop()

	cfg := config.Get()
	cfg.DisableAWSModeration = false
	cfg.Stage = "test"
	cfg.Region = "us-east-1"

	lambdaCtx := &common.LambdaContext{
		Config: cfg,
		AWSServices: &awsInit.AWSServices{
			CloudWatch: &cloudwatch.Client{},
		},
	}

	obs := initializeObservabilityServices(extractedServices{}, lambdaCtx, cfg, logger)
	require.NotNil(t, obs.centralizedCostService)
	require.NoError(t, obs.centralizedCostService.Close(context.Background()))
}

func TestInboxHandler_Round10_ParseActivity_ParseErrorBranch(t *testing.T) {
	env := newInboxTestEnv(t)

	raw := map[string]any{
		"@context":  activitypub.Context,
		"type":      activitypub.CreateType,
		"id":        env.cfg.BaseURL() + "/activities/parse-err",
		"actor":     env.remoteActorID,
		"to":        []string{env.local.ID},
		"published": "not-a-time",
		"object":    env.cfg.BaseURL() + "/objects/1",
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	_, err = env.handler.parseActivity(body)
	require.Error(t, err)
}

func TestInboxHandler_Round10_PerformSecurityChecks_NoActorDomain(t *testing.T) {
	env := newInboxTestEnv(t)

	ctx := newLiftContext("POST", "/users/alice/inbox", map[string]string{"Host": "localhost"}, nil, []byte("x"))
	ctx.SetParam("username", "alice")

	req := &InboxRequest{
		ActorDomain: "",
	}

	require.NoError(t, env.handler.performSecurityChecks(ctx, req))
}

func TestInboxHandler_Round10_CheckRateLimit_ErrorBranch(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	// Force a RateLimitLockout record to be found by prepending a matching expectation.
	call := env.mockQuery.On("First", mock.AnythingOfType("*models.RateLimitLockout")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*models.RateLimitLockout)
			out.UnlockTime = time.Now().Add(time.Hour)
		}).
		Return(nil).
		Once()
	env.mockQuery.ExpectedCalls = append([]*mock.Call{call}, env.mockQuery.ExpectedCalls[:len(env.mockQuery.ExpectedCalls)-1]...)

	ctx := newLiftContext("POST", "/users/alice/inbox", map[string]string{
		"Host":            "localhost",
		"X-Forwarded-For": "1.2.3.4",
	}, nil, []byte("x"))
	ctx.SetParam("username", "alice")

	req := &InboxRequest{
		Username:    "alice",
		ActorDomain: "remote.example",
		StartTime:   time.Now(),
		CostParams: &federation.CostCalculationParams{
			ActivityID:    "rate-limit-1",
			Domain:        "remote.example",
			ActivityType:  activitypub.CreateType,
			Direction:     "inbound",
			OperationType: "inbox_processing",
			Timestamp:     time.Now(),
		},
	}

	require.Error(t, env.handler.checkRateLimit(ctx, req))
}
