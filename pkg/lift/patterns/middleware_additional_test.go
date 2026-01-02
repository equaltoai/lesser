package patterns

import (
	"context"
	"errors"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recordingMetricsSender struct {
	calls  int
	last   MetricData
	retErr error
}

func (r *recordingMetricsSender) SendMetric(metric MetricData) error {
	r.calls++
	r.last = metric
	return r.retErr
}

func newTestContext(t *testing.T) *lift.Context {
	t.Helper()

	req := lift.NewRequest(&adapters.Request{
		Method:      "GET",
		Path:        "/test",
		Headers:     map[string]string{},
		QueryParams: map[string]string{},
		TriggerType: adapters.TriggerAPIGatewayV2,
	})
	return lift.NewContext(context.Background(), req)
}

func TestRequestIDMiddleware_SetsAndPreservesRequestID(t *testing.T) {
	ctx := newTestContext(t)

	called := 0
	h := RequestIDMiddleware("svc")(lift.HandlerFunc(func(ctx *lift.Context) error {
		called++
		require.NotEmpty(t, ctx.GetRequestID())
		require.Equal(t, ctx.GetRequestID(), ctx.Get("requestID"))
		return nil
	}))
	require.NoError(t, h.Handle(ctx))
	require.Equal(t, 1, called)

	original := ctx.GetRequestID()
	h = RequestIDMiddleware("svc")(lift.HandlerFunc(func(ctx *lift.Context) error {
		require.Equal(t, original, ctx.GetRequestID())
		return nil
	}))
	require.NoError(t, h.Handle(ctx))
}

func TestLoggingMiddleware_PropagatesHandlerError(t *testing.T) {
	ctx := newTestContext(t)
	logger := zap.NewNop()

	errBoom := errors.New("boom")
	h := LoggingMiddleware(logger)(lift.HandlerFunc(func(_ *lift.Context) error {
		return errBoom
	}))

	require.ErrorIs(t, h.Handle(ctx), errBoom)
}

func TestRecoveryMiddleware_RecoversAndWrites500(t *testing.T) {
	ctx := newTestContext(t)
	logger := zap.NewNop()

	h := RecoveryMiddleware(logger)(lift.HandlerFunc(func(_ *lift.Context) error {
		panic("boom")
	}))

	// Panic is swallowed; handler returns nil after recovery.
	require.NoError(t, h.Handle(ctx))
	require.Equal(t, 500, ctx.Response.StatusCode)
	require.NotNil(t, ctx.Response.Body)
}

func TestMetricsMiddleware_SendsMetricsAndDoesNotBlockOnFailure(t *testing.T) {
	ctx := newTestContext(t)

	sender := &recordingMetricsSender{}
	h := MetricsMiddleware("svc", sender)(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))

	require.NoError(t, h.Handle(ctx))
	require.Equal(t, 1, sender.calls)
	require.Equal(t, "svc.request", sender.last.Name)
	require.Equal(t, "success", sender.last.Tags["status"])

	// Failure to send metrics should not affect request.
	sender = &recordingMetricsSender{retErr: errors.New("send failed")}
	h = MetricsMiddleware("svc", sender)(lift.HandlerFunc(func(_ *lift.Context) error {
		return errors.New("handler failed")
	}))
	require.Error(t, h.Handle(ctx))
	require.Equal(t, 1, sender.calls)
	require.Equal(t, "error", sender.last.Tags["status"])

	// Nil sender is supported.
	h = MetricsMiddleware("svc", nil)(lift.HandlerFunc(func(_ *lift.Context) error { return nil }))
	require.NoError(t, h.Handle(ctx))
}

