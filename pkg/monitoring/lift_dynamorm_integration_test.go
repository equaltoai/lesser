package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultProductionConfigEnvironmentAndService(t *testing.T) {
	cfg := appconfig.Get()
	originalEnv := cfg.Environment
	originalStage := cfg.Stage
	originalService := cfg.ServiceName
	t.Cleanup(func() {
		cfg.Environment = originalEnv
		cfg.Stage = originalStage
		cfg.ServiceName = originalService
	})

	cfg.Environment = "dev"
	cfg.Stage = ""
	cfg.ServiceName = "svc"

	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "")

	got := DefaultProductionConfig()
	assert.Equal(t, "dev", got.Environment)
	assert.Equal(t, "svc", got.ServiceName)

	cfg.Environment = ""
	cfg.Stage = "staging"
	cfg.ServiceName = ""
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "lambda-name")

	got = DefaultProductionConfig()
	assert.Equal(t, "staging", got.Environment)
	assert.Equal(t, "lambda-name", got.ServiceName)
}

func TestProductionMonitorHelpers(t *testing.T) {
	pm := &ProductionMonitor{
		config: ProductionMetricsConfig{
			Environment: "test",
			ServiceName: "svc",
		},
		logger: zap.NewNop(),
		buffer: &MetricBuffer{
			metrics: make([]cwTypes.MetricDatum, 0, 100),
			maxSize: 100,
		},
		costTracker: cost.New(),
		initTime:    time.Now().Add(-10 * time.Millisecond),
		isFirstRun:  true,
		cloudwatch:  &stubCloudWatch{},
	}

	assert.True(t, pm.detectColdStart())
	assert.False(t, pm.detectColdStart())

	liftCtx := lift.NewContext(context.Background(), &lift.Request{
		Method:  "GET",
		Path:    "/api/v1/statuses",
		Headers: map[string]string{},
	})
	assert.Equal(t, "GET_api_v1_statuses", pm.getOperationName(liftCtx))

	assert.Equal(t, "timeout", classifyError(errors.New("timeout")))
	assert.Equal(t, "not_found", classifyError(errors.New("not found")))
	assert.Equal(t, "unauthorized", classifyError(errors.New("unauthorized")))
	assert.Equal(t, "forbidden", classifyError(errors.New("forbidden")))
	assert.Equal(t, "validation", classifyError(errors.New("validation failed")))
	assert.Equal(t, "throttling", classifyError(errors.New("throttled")))
	assert.Equal(t, StatusUnknown, classifyError(errors.New("other")))

	size, err := parseMemorySize("128")
	require.NoError(t, err)
	assert.Greater(t, size, 0.0)
	assert.Greater(t, getCurrentMemoryUsage(), 0.0)
}

func TestProductionMonitorRecordRequestAndCostMetrics(t *testing.T) {
	logger := zap.NewNop()
	cw := &stubCloudWatch{}

	pm := &ProductionMonitor{
		config: ProductionMetricsConfig{
			Namespace:                "ns",
			Environment:              "test",
			ServiceName:              "svc",
			EnablePerformanceMetrics: true,
		},
		cloudwatch: cw,
		logger:     logger,
		buffer: &MetricBuffer{
			metrics: make([]cwTypes.MetricDatum, 0, 1000),
			maxSize: 1000,
		},
		costTracker: cost.New(),
		initTime:    time.Now().Add(-5 * time.Millisecond),
		isFirstRun:  true,
	}

	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "128")
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "fn")
	t.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "1")

	lambdaCtx := &lambdacontext.LambdaContext{AwsRequestID: "aws-req", InvokedFunctionArn: "arn"}

	pm.recordRequestMetrics(context.Background(), RequestMetrics{
		Operation:   "op",
		Duration:    10 * time.Millisecond,
		StatusCode:  500,
		Method:      "GET",
		Path:        "/inbox",
		RequestID:   "req",
		TenantID:    "tenant",
		Error:       errors.New("boom"),
		IsColdStart: true,
		LambdaCtx:   lambdaCtx,
	})
	assert.Greater(t, len(pm.buffer.metrics), 0)

	// Unified tracker branch (early return).
	unified := cost.NewUnifiedTracker(nil, logger, "user", "req")
	pm.recordCostMetrics(context.WithValue(context.Background(), "unified_cost_tracker", unified), "op")

	// No tracker in context.
	pm.recordCostMetrics(context.Background(), "op")

	// Tracker present -> metrics added.
	tracker := cost.NewWithRequest("req", "api")
	pm.recordCostMetrics(cost.WithTracker(context.Background(), tracker), "op")
}

func TestProductionMonitorRecordBusinessMetrics(t *testing.T) {
	pm := &ProductionMonitor{
		config: ProductionMetricsConfig{
			Environment: "test",
			ServiceName: "svc",
		},
		logger: zap.NewNop(),
		buffer: &MetricBuffer{
			metrics: make([]cwTypes.MetricDatum, 0, 100),
			maxSize: 100,
		},
		cloudwatch: &stubCloudWatch{},
	}

	liftCtx := lift.NewContext(context.Background(), &lift.Request{
		Method:  "POST",
		Path:    "/inbox",
		Headers: map[string]string{},
	})
	liftCtx.Set("user_id", "admin_123")

	pm.recordBusinessMetrics(context.Background(), "op", liftCtx, nil)
	assert.Greater(t, len(pm.buffer.metrics), 0)
}

func TestProductionMonitorFlushMetricsBatchesAndClears(t *testing.T) {
	logger := zap.NewNop()
	cw := &stubCloudWatch{}

	pm := &ProductionMonitor{
		config: ProductionMetricsConfig{
			Namespace: "ns",
		},
		cloudwatch: cw,
		logger:     logger,
		buffer: &MetricBuffer{
			metrics: make([]cwTypes.MetricDatum, 0, 100),
			maxSize: 100,
		},
	}

	for i := 0; i < 25; i++ {
		pm.addMetric("m", 1, cwTypes.StandardUnitCount, nil)
	}
	pm.flushMetrics(context.Background())
	assert.Empty(t, pm.buffer.metrics)

	cw.mu.Lock()
	defer cw.mu.Unlock()
	require.Len(t, cw.putMetricDataInputs, 2)
	assert.Len(t, cw.putMetricDataInputs[0].MetricData, 20)
	assert.Len(t, cw.putMetricDataInputs[1].MetricData, 5)
}

func TestProductionMonitorLiftMiddleware(t *testing.T) {
	logger := zap.NewNop()

	pm := &ProductionMonitor{
		config: ProductionMetricsConfig{
			Environment:              "test",
			ServiceName:              "svc",
			EnableBusinessMetrics:    true,
			EnableCostTracking:       true,
			EnablePerformanceMetrics: true,
			BufferSize:               1000,
		},
		cloudwatch: &stubCloudWatch{},
		logger:     logger,
		buffer: &MetricBuffer{
			metrics: make([]cwTypes.MetricDatum, 0, 1000),
			maxSize: 1000,
		},
		costTracker: cost.New(),
		initTime:    time.Now().Add(-1 * time.Millisecond),
		isFirstRun:  true,
	}

	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "128")

	lc := &lambdacontext.LambdaContext{AwsRequestID: "aws-req", InvokedFunctionArn: "arn"}
	base := lambdacontext.NewContext(context.Background(), lc)

	liftCtx := lift.NewContext(base, &lift.Request{
		Method:  "GET",
		Path:    "/api/v1/statuses",
		Headers: map[string]string{},
	})
	liftCtx.RequestID = "req"
	liftCtx.SetTenantID("tenant")
	liftCtx.SetUserID("admin_123")

	mw := pm.LiftMiddleware()
	handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		ctx.Response.StatusCode = 500
		return errors.New("boom")
	}))

	require.Error(t, handler.Handle(liftCtx))
	assert.Greater(t, len(pm.buffer.metrics), 0)
}

func TestProductionFactoryHelpers(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "fn")

	app, monitor := NewProductionLiftApp(aws.Config{}, zap.NewNop())
	require.NotNil(t, app)
	require.NotNil(t, monitor)

	base := new(mocks.MockDB)
	wrapped := NewProductionDynamORMClient(base, monitor)
	require.NotNil(t, wrapped)
}

func TestProductionMonitorAdditionalCoverage(t *testing.T) {
	cfg := appconfig.Get()
	originalEnv := cfg.Environment
	originalStage := cfg.Stage
	originalService := cfg.ServiceName
	t.Cleanup(func() {
		cfg.Environment = originalEnv
		cfg.Stage = originalStage
		cfg.ServiceName = originalService
	})

	cfg.Environment = ""
	cfg.Stage = ""
	cfg.ServiceName = ""

	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "")

	assert.Equal(t, "production", getEnvironment())
	assert.Equal(t, StatusUnknown, getFunctionName())

	liftCtx := lift.NewContext(context.Background(), &lift.Request{Method: "GET", Path: "/notes"})
	assert.Equal(t, "", extractUserID(liftCtx))
	liftCtx.Set("user_id", "bot_123")
	assert.Equal(t, "bot_123", extractUserID(liftCtx))
	assert.Equal(t, "bot", classifyUser("bot_123"))
	assert.Equal(t, "user", classifyUser("user_123"))

	assert.Equal(t, "statuses", classifyEndpoint("/api/v1/statuses"))
	assert.Equal(t, "accounts", classifyEndpoint("/api/v1/accounts"))
	assert.Equal(t, "timelines", classifyEndpoint("/api/v1/timelines"))
	assert.Equal(t, "notifications", classifyEndpoint("/api/v1/notifications"))
	assert.Equal(t, "federation", classifyEndpoint("/inbox"))
	assert.Equal(t, "other", classifyEndpoint("/something-else"))

	pm := &ProductionMonitor{
		config: ProductionMetricsConfig{Namespace: "ns"},
		logger: zap.NewNop(),
		buffer: &MetricBuffer{metrics: nil, maxSize: 1},
		cloudwatch: &stubCloudWatch{
			putMetricDataErr: errors.New("flush failed"),
		},
	}
	pm.FlushOnExit(context.Background())
}
