package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// SecurityLogger is the global logger for security-related events
var SecurityLogger *zap.Logger

// Security event types
const (
	EventAuthFailure        = "auth_failure"
	EventCSRFFailure        = "csrf_failure"
	EventTokenReuse         = "token_reuse"
	EventRateLimitExceed    = "rate_limit_exceed"
	EventSSRFBlocked        = "ssrf_blocked"
	EventSuspiciousActivity = "suspicious_activity"
	EventPasswordFailure    = "password_failure"
	EventAccountLocked      = "account_locked"
	EventTokenRevoked       = "token_revoked"
	EventTokenFamilyRevoked = "token_family_revoked" // #nosec G101 - not a credential
	EventUserTokensRevoked  = "user_tokens_revoked"  // #nosec G101 - not a credential
	EventSecurityAlert      = "security_alert"
)

// InitSecurityLogger initializes the security-specific logger
func InitSecurityLogger() {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}

	// Add security-specific fields
	config.InitialFields = map[string]any{
		"service": "lesser",
		"type":    "security",
	}

	// Ensure sensitive data is not logged
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.LevelKey = "severity"
	config.EncoderConfig.NameKey = "logger"
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.StacktraceKey = "stacktrace"

	logger, err := config.Build(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return logging.NewScrubbingCore(core, logging.GetGlobalScrubber())
	}))
	if err != nil {
		SecurityLogger = zap.NewNop()
		return
	}
	SecurityLogger = logger
}

// LogSecurityEvent logs a security-relevant event
func LogSecurityEvent(event string, fields ...zap.Field) {
	if SecurityLogger == nil {
		InitSecurityLogger()
	}

	// Always include timestamp and event type
	allFields := append([]zap.Field{
		zap.String("event", event),
		zap.Int64("timestamp_unix", time.Now().Unix()),
	}, fields...)

	// Use appropriate log level based on event type
	switch event {
	case EventTokenReuse, EventAccountLocked, EventSecurityAlert:
		SecurityLogger.Error("Security event", allFields...)
	case EventAuthFailure, EventCSRFFailure, EventPasswordFailure:
		SecurityLogger.Warn("Security event", allFields...)
	default:
		SecurityLogger.Info("Security event", allFields...)
	}
}

func normalizeIPPrefix(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// X-Forwarded-For style lists.
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}

	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return raw
	}

	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2])
	}

	v6 := ip.To16()
	if v6 == nil {
		return raw
	}

	// /64 prefix
	prefix := make(net.IP, len(v6))
	copy(prefix, v6)
	for i := 8; i < 16; i++ {
		prefix[i] = 0
	}
	return prefix.String() + "/64"
}

// LogAuthFailure logs authentication failures
func LogAuthFailure(reason string, username string, ip string, userAgent string, requestID string) {
	LogSecurityEvent(EventAuthFailure,
		zap.String("reason", reason),
		zap.String("username", username),
		zap.String("ip", normalizeIPPrefix(ip)),
		zap.String("user_agent", userAgent),
		zap.String("request_id", requestID),
	)
}

// LogCSRFFailure logs CSRF validation failures
func LogCSRFFailure(reason string, username string, ip string, path string, userAgent string, requestID string) {
	LogSecurityEvent(EventCSRFFailure,
		zap.String("reason", reason),
		zap.String("username", username),
		zap.String("ip", normalizeIPPrefix(ip)),
		zap.String("path", path),
		zap.String("user_agent", userAgent),
		zap.String("request_id", requestID),
	)
}

// LogRateLimit logs rate limit exceeded events
func LogRateLimit(userID string, ip string, endpoint string, limit int, requestID string) {
	LogSecurityEvent(EventRateLimitExceed,
		zap.String("user_id", userID),
		zap.String("ip", normalizeIPPrefix(ip)),
		zap.String("endpoint", endpoint),
		zap.Int("limit", limit),
		zap.String("request_id", requestID),
	)
}

// LogSuspiciousActivity logs general suspicious activity
func LogSuspiciousActivity(activity string, userID string, ip string, details map[string]any, requestID string) {
	fields := []zap.Field{
		zap.String("activity", activity),
		zap.String("user_id", userID),
		zap.String("ip", normalizeIPPrefix(ip)),
		zap.String("request_id", requestID),
	}

	// Add details as fields
	for k, v := range details {
		fields = append(fields, zap.Any(k, v))
	}

	LogSecurityEvent(EventSuspiciousActivity, fields...)
}

// MaskSensitiveData masks sensitive information in logs
func MaskSensitiveData(data string) string {
	if len(data) <= 4 {
		return "****"
	}
	// Show first 2 and last 2 characters
	return data[:2] + "****" + data[len(data)-2:]
}

// SecurityContext provides context for security logging
type SecurityContext struct {
	UserID    string
	SessionID string
	IP        string
	UserAgent string
	RequestID string
}

// WithSecurityContext adds security context to logger
func WithSecurityContext(logger *zap.Logger, ctx SecurityContext) *zap.Logger {
	return logger.With(
		zap.String("user_id", ctx.UserID),
		zap.String("session_id", MaskSensitiveData(ctx.SessionID)),
		zap.String("ip", normalizeIPPrefix(ctx.IP)),
		zap.String("user_agent", ctx.UserAgent),
		zap.String("request_id", ctx.RequestID),
	)
}
