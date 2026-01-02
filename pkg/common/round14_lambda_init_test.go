package common

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLambdaInit_PureHelpers(t *testing.T) {
	t.Run("detectLambdaType", func(t *testing.T) {
		assert.Equal(t, LambdaTypeAPI, detectLambdaType("api"))
		assert.Equal(t, LambdaTypeProcessor, detectLambdaType("activity-processor"))
		assert.Equal(t, LambdaTypeMedia, detectLambdaType("media"))
		assert.Equal(t, LambdaTypeFederation, detectLambdaType("inbox"))
		assert.Equal(t, LambdaTypeFederation, detectLambdaType("outbox"))
		assert.Equal(t, LambdaTypeFederation, detectLambdaType("federation-delivery"))
		assert.Equal(t, LambdaTypeAI, detectLambdaType("ai"))
		assert.Equal(t, LambdaTypeBasic, detectLambdaType("unknown-service"))
	})

	t.Run("getTimeoutForLambdaType", func(t *testing.T) {
		assert.Equal(t, 5*time.Minute, getTimeoutForLambdaType(LambdaTypeMedia))
		assert.Equal(t, 2*time.Minute, getTimeoutForLambdaType(LambdaTypeAI))
		assert.Equal(t, 30*time.Second, getTimeoutForLambdaType(LambdaTypeBasic))
	})

	t.Run("shouldEnableHealthCheck", func(t *testing.T) {
		assert.True(t, shouldEnableHealthCheck(LambdaTypeAPI))
		assert.True(t, shouldEnableHealthCheck(LambdaTypeFederation))
		assert.False(t, shouldEnableHealthCheck(LambdaTypeProcessor))
	})

	t.Run("titleCase", func(t *testing.T) {
		assert.Equal(t, "", TitleCase(""))
		assert.Equal(t, "Hello", TitleCase("hello"))
		assert.Equal(t, "Hello", TitleCase("Hello"))
		assert.Equal(t, "1abc", TitleCase("1abc"))
	})

	t.Run("getDefaultServiceConfig", func(t *testing.T) {
		apiCfg := getDefaultServiceConfig(LambdaTypeAPI)
		assert.True(t, apiCfg.RequiresDynamoDB)
		assert.True(t, apiCfg.RequiresS3)
		assert.True(t, apiCfg.RequiresCloudWatch)

		procCfg := getDefaultServiceConfig(LambdaTypeProcessor)
		assert.True(t, procCfg.RequiresDynamoDB)
		assert.True(t, procCfg.RequiresSQS)
		assert.False(t, procCfg.RequiresS3)

		mediaCfg := getDefaultServiceConfig(LambdaTypeMedia)
		assert.True(t, mediaCfg.RequiresMediaConvert)
		assert.Equal(t, 5*time.Minute, mediaCfg.RequestTimeout)

		fedCfg := getDefaultServiceConfig(LambdaTypeFederation)
		assert.True(t, fedCfg.RequiresSQS)

		aiCfg := getDefaultServiceConfig(LambdaTypeAI)
		assert.True(t, aiCfg.RequiresComprehend)
		assert.Equal(t, 2*time.Minute, aiCfg.RequestTimeout)

		basicCfg := getDefaultServiceConfig(LambdaTypeBasic)
		assert.True(t, basicCfg.RequiresDynamoDB)
		assert.False(t, basicCfg.RequiresCloudWatch)
	})
}

func TestInitializeLambda_UsesInjectedAWSInitAndDefaults(t *testing.T) {
	orig := initializeAWSServicesFunc
	t.Cleanup(func() { initializeAWSServicesFunc = orig })

	var gotServiceCfg awsInit.ServiceConfig
	initializeAWSServicesFunc = func(ctx context.Context, cfg awsInit.ServiceConfig, logger *zap.Logger) (*awsInit.AWSServices, error) {
		gotServiceCfg = cfg
		return &awsInit.AWSServices{Logger: logger}, nil
	}

	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "api")
	lambdaCtx, err := InitializeLambda(LambdaConfig{
		ServiceName: "",
		LambdaType:  "",
		Version:     "test",
	})
	require.NoError(t, err)
	require.NotNil(t, lambdaCtx)

	assert.Equal(t, "api", lambdaCtx.ServiceName)
	assert.Equal(t, LambdaTypeAPI, lambdaCtx.LambdaType)
	assert.NotNil(t, lambdaCtx.AWSServices)

	assert.Equal(t, "api", gotServiceCfg.ServiceName)
	assert.NotZero(t, gotServiceCfg.RequestTimeout)
	assert.Equal(t, 3, gotServiceCfg.RetryMaxAttempts)
}

func TestInitializeLambda_ReturnsErrorWhenAWSInitFails(t *testing.T) {
	orig := initializeAWSServicesFunc
	t.Cleanup(func() { initializeAWSServicesFunc = orig })

	initializeAWSServicesFunc = func(_ context.Context, _ awsInit.ServiceConfig, _ *zap.Logger) (*awsInit.AWSServices, error) {
		return nil, stdErrors.New("boom")
	}

	_, err := InitializeLambda(LambdaConfig{
		ServiceName: "api",
		LambdaType:  LambdaTypeAPI,
		Version:     "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize AWS services")
}

func TestMustInitializeLambda_PanicsOnError(t *testing.T) {
	orig := initializeAWSServicesFunc
	t.Cleanup(func() { initializeAWSServicesFunc = orig })

	initializeAWSServicesFunc = func(_ context.Context, _ awsInit.ServiceConfig, _ *zap.Logger) (*awsInit.AWSServices, error) {
		return nil, stdErrors.New("boom")
	}

	require.Panics(t, func() {
		_ = MustInitializeLambda(LambdaConfig{
			ServiceName: "api",
			LambdaType:  LambdaTypeAPI,
			Version:     "test",
		})
	})
}

func TestLambdaContext_CreateLambdaHandler(t *testing.T) {
	t.Run("cold start path", func(t *testing.T) {
		lambdaCtx := &LambdaContext{
			Logger:      zap.NewNop(),
			StartTime:   time.Now(),
			ServiceName: "test",
		}

		h := lambdaCtx.CreateLambdaHandler(func(_ context.Context, _ interface{}) (interface{}, error) {
			return "ok", nil
		})

		out, err := h(context.Background(), map[string]any{"hello": "world"})
		require.NoError(t, err)
		assert.Equal(t, "ok", out)
	})

	t.Run("non-cold start path", func(t *testing.T) {
		lambdaCtx := &LambdaContext{
			Logger:      zap.NewNop(),
			StartTime:   time.Now().Add(-1 * time.Minute),
			ServiceName: "test",
		}

		h := lambdaCtx.CreateLambdaHandler(func(_ context.Context, _ interface{}) (interface{}, error) {
			return "ok", nil
		})

		out, err := h(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "ok", out)
	})
}
