package logging

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSensitiveDataScrubber_ScrubString_EnableDisable(t *testing.T) {
	s := NewSensitiveDataScrubber()

	input := "Bearer abcdefghijklmnopqrstuvwxyz0123456789"
	out := s.ScrubString(input)
	assert.Contains(t, out, "Bearer ")
	assert.Contains(t, out, "[REDACTED]")

	s.Disable()
	assert.Equal(t, input, s.ScrubString(input))
	assert.False(t, s.IsEnabled())

	s.Enable()
	assert.True(t, s.IsEnabled())
}

func TestSensitiveDataScrubber_ScrubString_CommonPatterns(t *testing.T) {
	s := NewSensitiveDataScrubber()

	assert.Contains(t, s.ScrubString("AKIA1234567890ABCDEF"), "[AWS_ACCESS_KEY_REDACTED]")

	email := s.ScrubString("contact alice@example.com")
	assert.Contains(t, email, "ali***@example.com")

	ip := s.ScrubString("ip 192.168.1.10")
	assert.Contains(t, ip, "192.168.***.10")
}

func TestSensitiveDataScrubber_ScrubJSON_Recursive(t *testing.T) {
	s := NewSensitiveDataScrubber()

	input := map[string]interface{}{
		"password": "secret123",
		"nested": map[string]interface{}{
			"token": "Bearer abcdefghijklmnopqrstuvwxyz0123456789",
		},
		"items": []interface{}{
			"AKIA1234567890ABCDEF",
			map[string]interface{}{"access_token": "abc"},
		},
	}

	out := s.ScrubJSON(input)
	assert.Equal(t, "[REDACTED]", out["password"])

	nested := out["nested"].(map[string]interface{})
	assert.Equal(t, "[REDACTED]", nested["token"])

	items := out["items"].([]interface{})
	assert.Contains(t, items[0].(string), "[AWS_ACCESS_KEY_REDACTED]")
	assert.Equal(t, "[REDACTED]", items[1].(map[string]interface{})["access_token"])
}

func TestScrubbingCore_ScrubsMessageAndFields(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	scrubber := NewSensitiveDataScrubber()
	logger := zap.New(NewScrubbingCore(core, scrubber))

	logger.Info("Bearer abcdefghijklmnopqrstuvwxyz0123456789",
		zap.String("auth", "Bearer abcdefghijklmnopqrstuvwxyz0123456789"),
		zap.Any("payload", map[string]interface{}{"password": "secret123"}),
	)

	entries := recorded.All()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Message, "[REDACTED]")

	ctx := entries[0].ContextMap()
	assert.Contains(t, ctx["auth"].(string), "[REDACTED]")
	assert.Equal(t, "[REDACTED]", ctx["payload"].(map[string]interface{})["password"])
}

func TestScrubbingCore_ScrubsSensitiveKeysAndErrors(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(NewScrubbingCore(core, NewSensitiveDataScrubber()))

	logger.Info("ok",
		zap.String("Authorization", "Bearer abcdefghijklmnopqrstuvwxyz0123456789"),
		zap.String("payload", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhbGljZSJ9.signature"),
		zap.String("signature", "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		zap.String("csrf_token", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		zap.String("client_secret", "super-secret-value"),
		zap.Error(errors.New("Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789")),
	)

	entries := recorded.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	assert.Equal(t, "[REDACTED]", fields["Authorization"])
	assert.Contains(t, fields["payload"].(string), "JWT_REDACTED")
	assert.Equal(t, "[REDACTED]", fields["signature"])
	assert.Equal(t, "[REDACTED]", fields["csrf_token"])
	assert.Equal(t, "[REDACTED]", fields["client_secret"])
	assert.Contains(t, fields["error"].(string), "[REDACTED]")
}

func TestLoggerMiddleware_WithContextAndSafeLogs(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	scrubber := NewSensitiveDataScrubber()
	mw := NewLoggerMiddleware(logger, scrubber)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "correlation_id", "corr-1")
	ctx = context.WithValue(ctx, "user_id", "u1")
	ctx = context.WithValue(ctx, "request_id", "r1")

	mw.LogSafeInfo(ctx, "Bearer abcdefghijklmnopqrstuvwxyz0123456789")
	mw.LogSafeError(ctx, "failed", errors.New("password=secret123"))

	entries := recorded.All()
	require.Len(t, entries, 2)
	assert.Contains(t, entries[0].Message, "[REDACTED]")

	fields := entries[1].ContextMap()
	assert.Equal(t, "corr-1", fields["correlation_id"])
	assert.Equal(t, "u1", fields["user_id"])
	assert.Equal(t, "r1", fields["request_id"])
	assert.Contains(t, fields["error"].(string), "[DB_PASSWORD_REDACTED]")
}

func TestAuditLogger_ScrubsMetadataAndReason(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	scrubber := NewSensitiveDataScrubber()

	audit := NewAuditLogger(logger, scrubber)
	audit.LogAuthenticationEvent(context.Background(), "login", "u1", true, map[string]interface{}{"password": "secret123"})
	audit.LogAuthorizationEvent(context.Background(), "read", "resource", "u1", false, "Bearer abcdefghijklmnopqrstuvwxyz0123456789")

	entries := recorded.All()
	require.Len(t, entries, 2)
	assert.Equal(t, "authentication_event", entries[0].Message)
	assert.Equal(t, "[REDACTED]", entries[0].ContextMap()["metadata"].(map[string]interface{})["password"])
	assert.Contains(t, entries[1].ContextMap()["reason"].(string), "[REDACTED]")
}

func TestMin(t *testing.T) {
	assert.Equal(t, 1, Min(1, 2))
	assert.Equal(t, 1, Min(2, 1))
}
