package lift

import (
	"context"
	stdErrors "errors"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/lambdacontext"
	awsSDK "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeCloudWatchClient struct {
	calls  int
	inputs []*cloudwatch.PutMetricDataInput
	err    error
}

func (f *fakeCloudWatchClient) PutMetricData(_ context.Context, input *cloudwatch.PutMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	f.calls++
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return nil, f.err
	}
	return &cloudwatch.PutMetricDataOutput{}, nil
}

func TestMetricsMiddleware_DetectColdStart(t *testing.T) {
	mm := &MetricsMiddleware{isFirstRun: true}
	require.True(t, mm.detectColdStart())
	require.False(t, mm.detectColdStart())
}

func TestMetricsMiddleware_getOperationName(t *testing.T) {
	ctx := createTestContext()
	ctx.Request.Method = "GET"
	ctx.Request.Path = "/api/v1/users/{id}"

	mm := &MetricsMiddleware{config: DefaultMetricsConfig()}
	require.Equal(t, "GET_api_v1_users_id", mm.getOperationName(ctx))

	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "my-fn")
	ctx = createTestContext()
	ctx.Request.Method = "GET"
	ctx.Request.Path = ""
	require.Equal(t, "my-fn", mm.getOperationName(ctx))
}

func TestMetricsMiddleware_RecordRequestMetricsAndFlush(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "my-fn")
	t.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "1")
	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "128")

	cfg := DefaultMetricsConfig()
	cfg.BufferSize = 1 // force flush in middleware when metrics are added
	cfg.Namespace = "Test"

	mm := NewMetricsMiddleware(awsSDK.Config{}, cfg, zap.NewNop())
	fakeCW := &fakeCloudWatchClient{}
	mm.cloudwatch = fakeCW

	// Seed the buffer and flush to cover batching.
	for i := 0; i < 25; i++ {
		mm.buffer.metrics = append(mm.buffer.metrics, types.MetricDatum{
			MetricName: awsSDK.String("m"),
			Value:      awsSDK.Float64(1),
			Unit:       types.StandardUnitCount,
		})
	}
	mm.flushMetrics(context.Background())
	require.Equal(t, 2, fakeCW.calls) // 20 + 5
	require.Len(t, mm.buffer.metrics, 0)

	// Cover error logging path (still clears buffer).
	mm.buffer.metrics = append(mm.buffer.metrics, types.MetricDatum{MetricName: awsSDK.String("m")})
	fakeCW.err = stdErrors.New("boom")
	mm.flushMetrics(context.Background())
	require.Equal(t, 3, fakeCW.calls)
	require.Len(t, mm.buffer.metrics, 0)
}

func TestMetricsMiddleware_Middleware_RecordsMetrics(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "my-fn")
	t.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "1")
	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "128")

	cfg := DefaultMetricsConfig()
	cfg.Namespace = "Test"
	cfg.BufferSize = 500
	cfg.EnableColdStart = true

	mm := NewMetricsMiddleware(awsSDK.Config{}, cfg, zap.NewNop())
	mm.cloudwatch = &fakeCloudWatchClient{}

	lctx := &lambdacontext.LambdaContext{AwsRequestID: "aws-1"}

	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/ok"
	ctx := liftPkg.NewContext(lambdacontext.NewContext(context.Background(), lctx), req)
	ctx.Logger = &liftPkg.NoOpLogger{}
	ctx.Response = liftPkg.NewResponse()
	ctx.SetTenantID("tenant1")

	handler := mm.Middleware()(liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
		ctx.Status(200)
		return ctx.JSON(map[string]string{"ok": "1"})
	}))

	require.NoError(t, handler.Handle(ctx))
	require.NotEmpty(t, mm.buffer.metrics)

	// Ensure no accidental environment leak into other tests.
	require.Equal(t, "my-fn", os.Getenv("AWS_LAMBDA_FUNCTION_NAME"))
}
