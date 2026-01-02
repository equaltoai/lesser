package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestScrubbingCore_WithAndSyncAndCheck(t *testing.T) {
	t.Parallel()

	base, _ := observer.New(zapcore.InfoLevel)
	scrubber := NewSensitiveDataScrubber()
	core := NewScrubbingCore(base, scrubber)

	wrapped := core.With([]zapcore.Field{zap.String("k", "v")})
	_, ok := wrapped.(*ScrubbingCore)
	assert.True(t, ok)

	assert.NoError(t, wrapped.Sync())

	checked := &zapcore.CheckedEntry{}
	// Not enabled => returns original checked entry.
	disabledCore, _ := observer.New(zapcore.ErrorLevel)
	disabled := NewScrubbingCore(disabledCore, scrubber)
	assert.Same(t, checked, disabled.Check(zapcore.Entry{Level: zapcore.InfoLevel}, checked))
}

func TestProductionLoggerConfigAndConstructor(t *testing.T) {
	t.Parallel()

	scrubber := NewSensitiveDataScrubber()
	cfg := ProductionLoggerConfig(scrubber)
	assert.False(t, cfg.Development)
	assert.Equal(t, "json", cfg.Encoding)

	logger, err := NewProductionLoggerWithScrubbing()
	require.NoError(t, err)
	require.NotNil(t, logger)
	logger.Info("hello")
	_ = logger.Sync()
}

func TestLoggerMiddleware_LogSafeWarnAndDebug(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	scrubber := NewSensitiveDataScrubber()
	mw := NewLoggerMiddleware(logger, scrubber)

	ctx := context.Background()
	mw.LogSafeWarn(ctx, "Bearer abcdefghijklmnopqrstuvwxyz0123456789")
	mw.LogSafeDebug(ctx, "AKIA1234567890ABCDEF")

	entries := recorded.All()
	require.Len(t, entries, 2)
	assert.Contains(t, entries[0].Message, "[REDACTED]")
	assert.Contains(t, entries[1].Message, "[AWS_ACCESS_KEY_REDACTED]")
}

func TestAuditLogger_LogSecurityEvent(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	scrubber := NewSensitiveDataScrubber()

	audit := NewAuditLogger(logger, scrubber)
	audit.LogSecurityEvent(context.Background(), "attack", "high", "Bearer abcdefghijklmnopqrstuvwxyz0123456789", map[string]interface{}{
		"password": "secret123",
	})

	entries := recorded.All()
	require.Len(t, entries, 1)
	ctx := entries[0].ContextMap()
	assert.Contains(t, ctx["description"].(string), "[REDACTED]")
	assert.Equal(t, "[REDACTED]", ctx["metadata"].(map[string]interface{})["password"])
}

func TestGlobalScrubberConvenienceFunctions(t *testing.T) {
	t.Parallel()

	require.NotNil(t, GetGlobalScrubber())

	out := ScrubString("Bearer abcdefghijklmnopqrstuvwxyz0123456789")
	assert.Contains(t, out, "[REDACTED]")

	jsonOut := ScrubJSON(map[string]interface{}{"password": "secret123"})
	assert.Equal(t, "[REDACTED]", jsonOut["password"])
}
