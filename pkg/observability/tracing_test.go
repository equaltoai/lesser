package observability

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/aws/aws-xray-sdk-go/v2/xray"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type noopXRayEmitter struct{}

func (noopXRayEmitter) Emit(_ *xray.Segment) {}

func (noopXRayEmitter) RefreshEmitterWithAddress(_ *net.UDPAddr) {}

func configureNoopXRay(t *testing.T) {
	t.Helper()
	_ = xray.Configure(xray.Config{Emitter: noopXRayEmitter{}})
}

func TestTracingManager_DisabledPaths(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tm := NewTracingManager(logger, &TracingConfig{Enabled: false, LocalTesting: false})
	ctx := context.Background()

	segmentCtx, segment := tm.StartSegment(ctx, "seg")
	assert.Equal(t, ctx, segmentCtx)
	assert.Nil(t, segment)

	assert.Equal(t, "local-testing", NewTracingManager(logger, &TracingConfig{Enabled: true, LocalTesting: true}).GetTraceContext(ctx).TraceID)
	assert.False(t, tm.IsEnabled())
}

func TestTracingManager_EnabledPaths(t *testing.T) {
	configureNoopXRay(t)
	logger := zaptest.NewLogger(t)

	tm := NewTracingManager(logger, &TracingConfig{
		ServiceName:    "svc",
		ServiceVersion: "v1",
		SamplingRate:   1,
		DaemonAddress:  "127.0.0.1:2000",
		Enabled:        true,
		LocalTesting:   false,
	})

	ctx, segment := func() (context.Context, *xray.Segment) {
		segmentCtx, seg := tm.StartSegment(context.Background(), "seg")
		require.NotNil(t, seg)
		return segmentCtx, seg
	}()
	defer segment.Close(nil)

	tm.AddAnnotation(ctx, "k", "v")
	tm.AddMetadata(ctx, "ns", map[string]interface{}{"x": "y"})
	tm.SetHTTPRequest(ctx, "GET", "https://example.com", "ua", "127.0.0.1")
	tm.SetHTTPResponse(ctx, 204, 123)
	tm.SetUser(ctx, "user")

	tm.AddError(ctx, errors.New("boom"), false)
	require.NotNil(t, xray.GetSegment(ctx))
	assert.True(t, xray.GetSegment(ctx).Fault)

	traceCtx := tm.GetTraceContext(ctx)
	require.NotNil(t, traceCtx)
	assert.NotEmpty(t, traceCtx.TraceID)

	headers := map[string]string{}
	tm.InjectTraceHeaders(ctx, headers)
	assert.Contains(t, headers, "X-Amzn-Trace-Id")

	assert.Nil(t, tm.ExtractTraceHeaders(map[string]string{}))
	extracted := tm.ExtractTraceHeaders(map[string]string{"X-Amzn-Trace-Id": "Root=1-abc;Parent=def;Sampled=1"})
	require.NotNil(t, extracted)
	assert.NotEmpty(t, extracted.TraceID)

	err := tm.TraceDatabase(ctx, "GetItem", "tbl", func(_ context.Context) error { return nil })
	require.NoError(t, err)

	boom := errors.New("boom")
	err = tm.TraceExternalCall(ctx, "remote", "GET", "https://example.com", func(_ context.Context) error { return boom })
	require.ErrorIs(t, err, boom)

	err = tm.TraceLambdaFunction(ctx, "fn", func(_ context.Context) error { return nil })
	require.NoError(t, err)

	err = tm.TraceLambdaFunction(ctx, "fn", func(_ context.Context) error { return boom })
	require.ErrorIs(t, err, boom)
}

func TestTracingManager_CreateTracingMiddleware(t *testing.T) {
	configureNoopXRay(t)
	logger := zaptest.NewLogger(t)

	tmDisabled := NewTracingManager(logger, &TracingConfig{Enabled: false, LocalTesting: false})
	called := false
	err := tmDisabled.CreateTracingMiddleware()(func(_ context.Context) error {
		called = true
		return nil
	})(context.Background())
	require.NoError(t, err)
	assert.True(t, called)

	tmEnabled := NewTracingManager(logger, &TracingConfig{
		ServiceName:  "svc",
		Enabled:      true,
		LocalTesting: false,
	})
	err = tmEnabled.CreateTracingMiddleware()(func(_ context.Context) error { return errors.New("boom") })(context.Background())
	require.Error(t, err)
}

func TestTraceContext_PropertyHelpers(t *testing.T) {
	var tc TraceContext
	value, ok := tc.GetProperty("k")
	assert.False(t, ok)
	assert.Nil(t, value)

	tc.SetProperty("k", "v")
	value, ok = tc.GetProperty("k")
	assert.True(t, ok)
	assert.Equal(t, "v", value)

	assert.Equal(t, 1, boolToInt(true))
	assert.Equal(t, 0, boolToInt(false))
}

func TestTracingManager_DefaultConfigAndNoSegmentContext(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tm := NewTracingManager(logger, nil)
	require.NotNil(t, tm)
	require.NotNil(t, tm.config)
	require.Equal(t, "lesser-service", tm.config.ServiceName)

	configureNoopXRay(t)
	enabled := NewTracingManager(logger, &TracingConfig{
		ServiceName:  "svc",
		Enabled:      true,
		LocalTesting: false,
	})

	ctx := context.Background()
	enabled.AddAnnotation(ctx, "k", "v")
	enabled.AddMetadata(ctx, "ns", map[string]interface{}{"x": "y"})
	enabled.SetHTTPRequest(ctx, "GET", "https://example.com", "ua", "127.0.0.1")
	enabled.SetHTTPResponse(ctx, 200, 10)
	enabled.SetUser(ctx, "user")
	enabled.AddError(ctx, errors.New("boom"), false)

	traceCtx := enabled.GetTraceContext(ctx)
	require.NotNil(t, traceCtx)
	require.Equal(t, "no-segment", traceCtx.TraceID)

	headers := map[string]string{}
	enabled.InjectTraceHeaders(ctx, headers)
	require.Contains(t, headers, "X-Amzn-Trace-Id")
}
