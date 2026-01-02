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
	"github.com/equaltoai/lesser/pkg/lift"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestStandardizedMain_FatalPaths(t *testing.T) {
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

	lambdaCtx := &common.LambdaContext{
		Config:      &config.Config{Region: "us-east-1", Version: "test"},
		Logger:      zap.NewNop(),
		StartTime:   time.Now(),
		AWSServices: &awsInit.AWSServices{Config: awsSDK.Config{}},
	}
	initializeLambdaFunc = func(common.LambdaConfig) (*common.LambdaContext, error) {
		return lambdaCtx, nil
	}
	lambdaStartFunc = func(any) {}
	logFatalfFunc = func(string, ...any) { panic("fatal") }

	t.Run("init options error", func(t *testing.T) {
		initializeWithOptionsFunc = func(*common.LambdaContext, common.LambdaInitOptions) error {
			return stdErrors.New("boom")
		}

		require.Panics(t, func() {
			StandardizedMain(DefaultMainConfig("svc", common.LambdaTypeBasic))
		})
	})

	t.Run("custom init error", func(t *testing.T) {
		initializeWithOptionsFunc = func(*common.LambdaContext, common.LambdaInitOptions) error {
			return nil
		}

		require.Panics(t, func() {
			StandardizedMain(MainConfig{
				ServiceName: "svc",
				LambdaType:  common.LambdaTypeBasic,
				InitCustomServices: func(*common.LambdaContext) error {
					return stdErrors.New("boom")
				},
			})
		})
	})

	t.Run("configure routes error", func(t *testing.T) {
		initializeWithOptionsFunc = func(*common.LambdaContext, common.LambdaInitOptions) error {
			return nil
		}

		require.Panics(t, func() {
			StandardizedMain(MainConfig{
				ServiceName: "svc",
				LambdaType:  common.LambdaTypeBasic,
				ConfigureRoutes: func(*liftPkg.App, *common.LambdaContext) error {
					return stdErrors.New("boom")
				},
			})
		})
	})
}

func TestStandardizedMain_CustomMiddlewareBranch(t *testing.T) {
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

	lambdaCtx := &common.LambdaContext{
		Config:      &config.Config{Region: "us-east-1", Version: "test"},
		Logger:      zap.NewNop(),
		StartTime:   time.Now(),
		AWSServices: &awsInit.AWSServices{Config: awsSDK.Config{}},
	}
	initializeLambdaFunc = func(common.LambdaConfig) (*common.LambdaContext, error) {
		return lambdaCtx, nil
	}
	initializeWithOptionsFunc = func(*common.LambdaContext, common.LambdaInitOptions) error { return nil }

	var started any
	lambdaStartFunc = func(handler any) { started = handler }
	logFatalfFunc = func(string, ...any) { panic("unexpected fatalf") }

	customCalled := false

	StandardizedMain(MainConfig{
		ServiceName: "svc",
		LambdaType:  common.LambdaTypeBasic,
		CreateCustomMiddleware: func(*common.LambdaContext) []liftPkg.Middleware {
			customCalled = true
			return []liftPkg.Middleware{
				func(next liftPkg.Handler) liftPkg.Handler {
					return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
						ctx.Set("custom_middleware", true)
						return next.Handle(ctx)
					})
				},
			}
		},
	})

	require.True(t, customCalled)
	require.NotNil(t, started)
}

func TestLambdaMiddlewareFactories_ExerciseBranches(t *testing.T) {
	logger := zap.NewNop()

	t.Run("logging middleware sets request id and logger", func(t *testing.T) {
		mw := createLoggingMiddleware(logger)

		req := liftPkg.NewRequest(nil)
		req.Method = "GET"
		req.Path = "/test"
		req.Headers = map[string]string{"User-Agent": "ua", "X-Forwarded-For": "1.2.3.4"}
		ctx := liftPkg.NewContext(context.Background(), req)

		handler := mw(liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			return ctx.Status(200).JSON(map[string]string{"ok": "1"})
		}))

		require.NoError(t, handler.Handle(ctx))
		require.NotEmpty(t, ctx.GetRequestID())
		require.NotNil(t, ctx.Get("logger"))
		require.Equal(t, 200, ctx.Response.StatusCode)
	})

	t.Run("logging middleware uses existing request id and warns for 4xx", func(t *testing.T) {
		mw := createLoggingMiddleware(logger)

		req := liftPkg.NewRequest(nil)
		req.Method = "GET"
		req.Path = "/notfound"
		ctx := liftPkg.NewContext(context.Background(), req)
		ctx.SetRequestID("req-1")

		handler := mw(liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			return ctx.Status(404).JSON(map[string]string{"error": "not_found"})
		}))

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, "req-1", ctx.GetRequestID())
		require.Equal(t, 404, ctx.Response.StatusCode)
	})

	t.Run("logging middleware logs error path", func(t *testing.T) {
		mw := createLoggingMiddleware(logger)

		ctx := liftPkg.NewContext(context.Background(), liftPkg.NewRequest(nil))
		handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
			return stdErrors.New("boom")
		}))

		require.Error(t, handler.Handle(ctx))
	})

	t.Run("cost tracking middleware returns underlying error", func(t *testing.T) {
		mw := createCostTrackingMiddleware(logger)

		ctx := liftPkg.NewContext(context.Background(), liftPkg.NewRequest(nil))
		handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
			return stdErrors.New("boom")
		}))

		require.Error(t, handler.Handle(ctx))
	})

	t.Run("emf metrics middleware records error and success", func(t *testing.T) {
		metrics := &fakeEMFMetrics{}
		lambdaCtx := &common.LambdaContext{Logger: logger, EMFMetrics: metrics}
		mw := createEMFMetricsMiddleware(lambdaCtx)

		okCtx := liftPkg.NewContext(context.Background(), liftPkg.NewRequest(nil))
		okHandler := mw(liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			return ctx.Status(200).JSON(map[string]string{"ok": "1"})
		}))
		require.NoError(t, okHandler.Handle(okCtx))
		require.NotEmpty(t, metrics.latency)
		require.NotEmpty(t, metrics.success)

		errCtx := liftPkg.NewContext(context.Background(), liftPkg.NewRequest(nil))
		errHandler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error { return stdErrors.New("boom") }))
		require.Error(t, errHandler.Handle(errCtx))
		require.NotEmpty(t, metrics.errors)
	})
}

func TestCreateStandardizedLambdaHandler_SuccessAndMetricsBranches(t *testing.T) {
	makeAPIEvent := func(method, path string, headers map[string]string) map[string]any {
		eventHeaders := make(map[string]any, len(headers))
		for k, v := range headers {
			eventHeaders[k] = v
		}
		return map[string]any{
			"version":  "2.0",
			"routeKey": "$default",
			"headers":  eventHeaders,
			"requestContext": map[string]any{
				"stage": "$default",
				"http": map[string]any{
					"method": method,
					"path":   path,
				},
			},
		}
	}

	t.Run("success records success metrics when enabled", func(t *testing.T) {
		metrics := &fakeEMFMetrics{}
		lambdaCtx := &common.LambdaContext{
			Config:      &config.Config{Region: "us-east-1", Version: "test"},
			Logger:      zap.NewNop(),
			StartTime:   time.Now(),
			AWSServices: &awsInit.AWSServices{Config: awsSDK.Config{}},
			EMFMetrics:  metrics,
		}

		app := lift.NewHTTPApp(lift.AppConfig{
			Debug:              false,
			Timeout:            time.Second,
			EnableCORS:         false,
			EnableMetrics:      false,
			EnableCostTracking: false,
			AWSConfig:          &lambdaCtx.AWSServices.Config,
		}, lambdaCtx.Logger)

		_ = app.GET("/ok", func(ctx *liftPkg.Context) error {
			return ctx.Status(200).JSON(map[string]string{"ok": "1"})
		})

		handler := createStandardizedLambdaHandler(app, lambdaCtx, "svc")
		_, err := handler(context.Background(), makeAPIEvent("GET", "/ok", map[string]string{"Host": "localhost"}))
		require.NoError(t, err)

		require.NotEmpty(t, metrics.success)
	})

	t.Run("metrics disabled and wrong-type metrics do not panic", func(t *testing.T) {
		lambdaCtx := &common.LambdaContext{
			Config:      &config.Config{Region: "us-east-1", Version: "test"},
			Logger:      zap.NewNop(),
			StartTime:   time.Now(),
			AWSServices: &awsInit.AWSServices{Config: awsSDK.Config{}},
			EMFMetrics:  struct{}{},
		}

		app := lift.NewHTTPApp(lift.AppConfig{
			Debug:              false,
			Timeout:            time.Second,
			EnableCORS:         false,
			EnableMetrics:      false,
			EnableCostTracking: false,
			AWSConfig:          &lambdaCtx.AWSServices.Config,
		}, lambdaCtx.Logger)

		_ = app.GET("/ok", func(ctx *liftPkg.Context) error {
			return ctx.Status(200).JSON(map[string]string{"ok": "1"})
		})

		handler := createStandardizedLambdaHandler(app, lambdaCtx, "svc")
		_, err := handler(context.Background(), makeAPIEvent("GET", "/ok", map[string]string{"Host": "localhost"}))
		require.NoError(t, err)
	})
}
