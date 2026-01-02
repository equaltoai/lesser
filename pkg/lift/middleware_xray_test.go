package lift

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-xray-sdk-go/xray"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestXRayMiddleware_Disabled_PassesThrough(t *testing.T) {
	t.Setenv("_X_AMZN_TRACE_ID", "")

	xm := NewXRayMiddleware("svc", "dev", zap.NewNop())
	mw := xm.Middleware()

	ctx := createTestContext()
	nextCalled := false
	handler := mw(func(*liftPkg.Context) error {
		nextCalled = true
		return nil
	})

	require.NoError(t, handler(ctx))
	require.True(t, nextCalled)
}

func TestXRayMiddleware_Enabled_RunsHandler(t *testing.T) {
	t.Setenv("_X_AMZN_TRACE_ID", "trace")

	xm := NewXRayMiddleware("svc", "dev", zap.NewNop())
	mw := xm.Middleware()

	ctx := createTestContext()
	ctx.Request.Method = "GET"
	ctx.Request.Path = "/test"
	ctx.SetTenantID("tenant1")
	ctx.Set("user_id", "user1")

	nextCalled := false
	handler := mw(func(ctx *liftPkg.Context) error {
		nextCalled = true
		return ctx.JSON(map[string]string{"ok": "1"})
	})

	require.NoError(t, handler(ctx))
	require.True(t, nextCalled)
	require.Equal(t, 200, ctx.Response.StatusCode)
}

func TestTraceHelpers_WithAndWithoutSegment(t *testing.T) {
	called := false
	require.NoError(t, TraceDynamoDB(context.Background(), "GetItem", "tbl", func(context.Context) error {
		called = true
		return nil
	}))
	require.True(t, called)

	segCtx, seg := xray.BeginSegment(context.Background(), "test")
	defer seg.Close(nil)

	require.NoError(t, TraceFederation(segCtx, "example.com", "Fetch", func(context.Context) error {
		return nil
	}))
	require.Error(t, TraceS3(segCtx, "GetObject", "bucket", "key", func(context.Context) error {
		return stdErrors.New("boom")
	}))
	require.NoError(t, TraceSQS(segCtx, "SendMessage", "queue", 1, func(context.Context) error { return nil }))
}

func TestXRayMiddleware_addLambdaMetadata(t *testing.T) {
	t.Setenv("_X_AMZN_TRACE_ID", "trace")

	xm := NewXRayMiddleware("svc", "dev", zap.NewNop())
	_, seg := xray.BeginSegment(context.Background(), "test")
	defer seg.Close(nil)

	lctx := &lambdacontext.LambdaContext{
		AwsRequestID:       "aws-1",
		InvokedFunctionArn: "arn",
	}
	ctxWithLambda := lambdacontext.NewContext(context.Background(), lctx)

	// Should not panic and should accept the lambda context.
	xm.addLambdaMetadata(ctxWithLambda, seg)
}
