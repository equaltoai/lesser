package observability

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type lambdaHandlerStub struct{}

func (lambdaHandlerStub) Invoke(_ context.Context, _ []byte) ([]byte, error) { return nil, nil }

func TestXRayMiddleware_WrapsWhenEnabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := &XRayConfig{ServiceName: "svc", ServiceVersion: "v1", Enabled: false, LocalTesting: false}

	h := lambdaHandlerStub{}
	wrapped := WrapLambdaHandler(cfg, logger, h)
	assert.Equal(t, h, wrapped)

	cfg.Enabled = true
	cfg.LocalTesting = true
	wrapped = WrapLambdaHandler(cfg, logger, h)
	assert.Equal(t, h, wrapped)

	cfg.LocalTesting = false
	wrapped = WrapLambdaHandler(cfg, logger, h)
	assert.Equal(t, h, wrapped)

	fn := func() {}
	wrappedFn := WrapLambdaFunc(cfg, logger, fn)
	require.IsType(t, fn, wrappedFn)
	assert.Equal(t, reflect.ValueOf(fn).Pointer(), reflect.ValueOf(wrappedFn.(func())).Pointer())
}

func TestXRayMiddleware_TracingHelpers(t *testing.T) {
	configureNoopXRay(t)

	ctx, seg := xray.BeginSegment(context.Background(), "root")
	require.NotNil(t, seg)
	defer seg.Close(nil)

	AddServiceAnnotations(ctx, "svc", "v1", map[string]interface{}{"k": "v"})
	AddErrorToTrace(ctx, errors.New("boom"), false)
	assert.True(t, xray.GetSegment(ctx).Fault)

	called := false
	err := TraceSubsegment(ctx, "sub", func(_ context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)

	boom := errors.New("boom")
	err = TraceDatabaseOperation(ctx, "GetItem", "tbl", func(_ context.Context) error { return boom })
	require.ErrorIs(t, err, boom)

	err = TraceFederationCall(ctx, "remote", "GET", "https://example.com", func(_ context.Context) error { return nil })
	require.NoError(t, err)
}

func TestXRayMiddleware_TraceMediaProcessing(t *testing.T) {
	configureNoopXRay(t)

	ctx, seg := xray.BeginSegment(context.Background(), "root")
	require.NotNil(t, seg)
	defer seg.Close(nil)

	err := TraceMediaProcessing(ctx, "NotAType", "resize", func(_ context.Context) error { return nil })
	require.Error(t, err)

	called := false
	err = TraceMediaProcessing(ctx, "Image", "resize", func(_ context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)
}
