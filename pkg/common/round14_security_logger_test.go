package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSecurityLoggerHelpers(t *testing.T) {
	t.Run("MaskSensitiveData", func(t *testing.T) {
		assert.Equal(t, "****", MaskSensitiveData(""))
		assert.Equal(t, "****", MaskSensitiveData("abcd"))
		assert.Equal(t, "ab****yz", MaskSensitiveData("abWXYZyz"))
	})

	t.Run("WithSecurityContext adds fields", func(t *testing.T) {
		core, obs := observer.New(zapcore.InfoLevel)
		logger := zap.New(core)

		ctxLogger := WithSecurityContext(logger, SecurityContext{
			UserID:    "u1",
			SessionID: "s1",
			IP:        "127.0.0.1",
			UserAgent: "ua",
			RequestID: "r1",
		})
		ctxLogger.Info("hello")

		entries := obs.All()
		if len(entries) == 0 {
			t.Fatalf("expected at least one log entry")
		}
		fields := entries[0].ContextMap()
		assert.Equal(t, "u1", fields["user_id"])
		assert.Equal(t, "****", fields["session_id"])
		assert.Equal(t, "127.0.0.0/24", fields["ip"])
		assert.Equal(t, "ua", fields["user_agent"])
		assert.Equal(t, "r1", fields["request_id"])
	})
}

func TestLogSecurityEvent_DoesNotPanic(t *testing.T) {
	orig := SecurityLogger
	t.Cleanup(func() { SecurityLogger = orig })

	// Avoid test output noise by using a no-op logger.
	SecurityLogger = zap.NewNop()

	LogSecurityEvent(EventTokenReuse, zap.String("k", "v"))
	LogSecurityEvent(EventAuthFailure, zap.String("k", "v"))
	LogSecurityEvent(EventSuspiciousActivity, zap.String("k", "v"))

	LogAuthFailure("bad", "alice", "127.0.0.1", "ua", "r1")
	LogCSRFFailure("bad", "user-1", "127.0.0.1", "/path", "ua", "r1")
	LogRateLimit("user-1", "127.0.0.1", "/endpoint", 10, "r1")
	LogSuspiciousActivity("weird", "user-1", "127.0.0.1", map[string]any{"a": 1}, "r1")
}

func TestLogSecurityEvent_InitializesGlobalLoggerWhenNil(t *testing.T) {
	orig := SecurityLogger
	t.Cleanup(func() { SecurityLogger = orig })

	SecurityLogger = nil
	LogSecurityEvent(EventSuspiciousActivity, zap.String("k", "v"))
	assert.NotNil(t, SecurityLogger)
}
