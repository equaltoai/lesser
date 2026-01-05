package middleware

import (
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubMetrics struct {
	panics []struct {
		path  string
		value string
	}
}

func (s *stubMetrics) RecordPanic(path string, panicValue string) {
	s.panics = append(s.panics, struct {
		path  string
		value string
	}{path: path, value: panicValue})
}

func TestPanicRecovery(t *testing.T) {
	logger := zap.NewNop()
	mw := PanicRecovery(logger)

	ctx := newTestLiftContext("GET", "/panic")
	ctx.Request.Headers["X-Request-Id"] = "req-123"

	handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		panic("boom")
	}))

	err := handler.Handle(ctx)
	require.NoError(t, err)
	assert.Equal(t, 500, ctx.Response.StatusCode)

	body, ok := ctx.Response.Body.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "internal_server_error", body["error"])
	assert.Equal(t, "req-123", body["request_id"])
}

func TestPanicRecoveryWithMetrics(t *testing.T) {
	logger := zap.NewNop()
	metrics := &stubMetrics{}
	mw := PanicRecoveryWithMetrics(logger, metrics)

	ctx := newTestLiftContext("GET", "/panic")

	handler := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
		panic("boom")
	}))

	err := handler.Handle(ctx)
	require.NoError(t, err)
	assert.Equal(t, 500, ctx.Response.StatusCode)

	require.Len(t, metrics.panics, 1)
	assert.Equal(t, "/panic", metrics.panics[0].path)
	assert.Equal(t, "boom", metrics.panics[0].value)
}

func TestShouldAlert(t *testing.T) {
	assert.False(t, shouldAlert("context canceled"))
	assert.True(t, shouldAlert("boom"))
}

