package common

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

	SecurityLogger, _ = config.Build()
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

// LogAuthFailure logs authentication failures
func LogAuthFailure(reason string, username string, ip string, userAgent string) {
	LogSecurityEvent(EventAuthFailure,
		zap.String("reason", reason),
		zap.String("username", username),
		zap.String("ip", ip),
		zap.String("user_agent", userAgent),
	)
}

// LogCSRFFailure logs CSRF validation failures
func LogCSRFFailure(reason string, userID string, ip string, path string) {
	LogSecurityEvent(EventCSRFFailure,
		zap.String("reason", reason),
		zap.String("user_id", userID),
		zap.String("ip", ip),
		zap.String("path", path),
	)
}

// LogRateLimit logs rate limit exceeded events
func LogRateLimit(userID string, ip string, endpoint string, limit int) {
	LogSecurityEvent(EventRateLimitExceed,
		zap.String("user_id", userID),
		zap.String("ip", ip),
		zap.String("endpoint", endpoint),
		zap.Int("limit", limit),
	)
}

// LogSuspiciousActivity logs general suspicious activity
func LogSuspiciousActivity(activity string, userID string, ip string, details map[string]any) {
	fields := []zap.Field{
		zap.String("activity", activity),
		zap.String("user_id", userID),
		zap.String("ip", ip),
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
		zap.String("session_id", ctx.SessionID),
		zap.String("ip", ctx.IP),
		zap.String("user_agent", ctx.UserAgent),
		zap.String("request_id", ctx.RequestID),
	)
}
