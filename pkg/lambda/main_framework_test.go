package lambda

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	awsSDK "github.com/aws/aws-sdk-go-v2/aws"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeEMFMetrics struct {
	business []string
	latency  []string
	success  []string
	errors   []string

	throughputCount int
}

func (f *fakeEMFMetrics) RecordBusinessMetric(name string, _ float64, _ string, _ map[string]string) {
	f.business = append(f.business, name)
}

func (f *fakeEMFMetrics) RecordLatency(operation string, _ time.Duration) {
	f.latency = append(f.latency, operation)
}

func (f *fakeEMFMetrics) RecordThroughput(_ string, _ int) {
	f.throughputCount++
}

func (f *fakeEMFMetrics) RecordError(operation string, _ string) {
	f.errors = append(f.errors, operation)
}

func (f *fakeEMFMetrics) RecordSuccess(operation string) {
	f.success = append(f.success, operation)
}

func TestDefaultMainConfig_Behavior(t *testing.T) {
	cfg := DefaultMainConfig("svc", common.LambdaTypeAPI)
	assert.Equal(t, "svc", cfg.ServiceName)
	assert.Equal(t, common.LambdaTypeAPI, cfg.LambdaType)
	assert.True(t, cfg.EnableCORS)
	assert.True(t, cfg.EnableRateLimit)
	assert.True(t, cfg.EnableMetrics)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

func TestStandardizedMain_HappyPath(t *testing.T) {
	origInit := initializeLambdaFunc
	origInitOpts := initializeWithOptionsFunc
	origStart := lambdaStartFunc
	origFatalf := logFatalfFunc
	t.Cleanup(func() {
		initializeLambdaFunc = origInit
		initializeWithOptionsFunc = origInitOpts
		lambdaStartFunc = origStart
		logFatalfFunc = origFatalf
	})

	metrics := &fakeEMFMetrics{}
	lambdaCtx := &common.LambdaContext{
		Config:      &config.Config{Region: "us-east-1", Version: "test"},
		Logger:      zap.NewNop(),
		StartTime:   time.Now(),
		AWSServices: &awsInit.AWSServices{Config: awsSDK.Config{}},
		Repos:       struct{}{}, // non-nil to exercise rate-limit branch
		EMFMetrics:  metrics,
	}

	var sawLambdaConfig common.LambdaConfig
	initializeLambdaFunc = func(cfg common.LambdaConfig) (*common.LambdaContext, error) {
		sawLambdaConfig = cfg
		return lambdaCtx, nil
	}

	var sawOptions common.LambdaInitOptions
	initializeWithOptionsFunc = func(_ *common.LambdaContext, opts common.LambdaInitOptions) error {
		sawOptions = opts
		return nil
	}

	var startedHandler any
	lambdaStartFunc = func(handler any) {
		startedHandler = handler
	}
	logFatalfFunc = func(string, ...any) {
		panic("unexpected fatalf")
	}

	customInitCalled := false
	routeConfigCalled := false

	StandardizedMain(MainConfig{
		ServiceName:     "svc",
		LambdaType:      common.LambdaTypeAPI,
		EnableRateLimit: true,
		InitCustomServices: func(*common.LambdaContext) error {
			customInitCalled = true
			return nil
		},
		ConfigureRoutes: func(app *liftPkg.App, _ *common.LambdaContext) error {
			routeConfigCalled = true
			// No routes needed for this test.
			_ = app
			return nil
		},
	})

	require.Equal(t, "svc", sawLambdaConfig.ServiceName)
	require.Equal(t, common.LambdaTypeAPI, sawLambdaConfig.LambdaType)
	require.True(t, sawOptions.InitializeStorage)
	require.True(t, customInitCalled)
	require.True(t, routeConfigCalled)
	require.NotNil(t, startedHandler)

	// Exercise the created handler to cover metrics and flush behavior.
	h, ok := startedHandler.(func(context.Context, any) (any, error))
	require.True(t, ok)
	_, err := h(context.Background(), nil)
	require.Error(t, err)

	assert.Contains(t, metrics.business, "ColdStarts")
	assert.Contains(t, metrics.business, "ColdStartDuration")
	assert.GreaterOrEqual(t, metrics.throughputCount, 1)
	assert.NotEmpty(t, metrics.latency)
	assert.NotEmpty(t, metrics.errors)
}

func TestStandardizedMain_InitializationErrorsFatal(t *testing.T) {
	origInit := initializeLambdaFunc
	origFatalf := logFatalfFunc
	t.Cleanup(func() {
		initializeLambdaFunc = origInit
		logFatalfFunc = origFatalf
	})

	initializeLambdaFunc = func(common.LambdaConfig) (*common.LambdaContext, error) {
		return nil, stdErrors.New("boom")
	}
	logFatalfFunc = func(string, ...any) {
		panic("fatal")
	}
	lambdaStartFunc = func(any) {}

	require.Panics(t, func() {
		StandardizedMain(DefaultMainConfig("svc", common.LambdaTypeBasic))
	})
}

func TestCreateRequestIDMiddleware_SetsRequestID(t *testing.T) {
	mw := createRequestIDMiddleware()

	req := liftPkg.NewRequest(nil)
	ctx := liftPkg.NewContext(context.Background(), req)

	handler := mw(liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
		return ctx.Status(200).JSON(map[string]string{"ok": "1"})
	}))

	require.NoError(t, handler.Handle(ctx))
	assert.NotEmpty(t, ctx.GetRequestID())
}

func TestPanicRecovery_Recovers(t *testing.T) {
	mw := PanicRecovery(zap.NewNop())

	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/panic"
	req.Headers = map[string]string{"X-Request-Id": "req-1"}
	ctx := liftPkg.NewContext(context.Background(), req)

	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
		panic("boom")
	}))

	require.NoError(t, handler.Handle(ctx))
	require.NotNil(t, ctx.Response)
	assert.Equal(t, 500, ctx.Response.StatusCode)
}
