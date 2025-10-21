package zap

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
	"github.com/pay-theory/lift/pkg/utils/sanitization"
)

const (
	formatConsole = "console"
)

// ZapLogger implements the StructuredLogger interface using Zap
type ZapLogger struct {
	logger        *zap.Logger
	sugar         *zap.SugaredLogger
	stats         *loggerStats
	contextFields map[string]any
	snsNotifier   *observability.SNSNotifier
	config        observability.LoggerConfig
}

// loggerStats tracks logger performance metrics
type loggerStats struct {
	lastError      string
	entriesLogged  int64
	entriesDropped int64
	flushCount     int64
	lastFlush      int64
	errorCount     int64
}

// ZapLoggerOptions contains optional parameters for NewZapLogger
type ZapLoggerOptions struct {
	Notifier *observability.SNSNotifier
}

// NewZapLogger creates a new Zap-based logger
func NewZapLogger(config observability.LoggerConfig, opts ...ZapLoggerOptions) (*ZapLogger, error) {
	zapConfig := buildZapConfig(config)

	logger, err := zapConfig.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	zapLogger := &ZapLogger{
		logger:        logger,
		sugar:         logger.Sugar(),
		config:        config,
		stats:         &loggerStats{},
		contextFields: make(map[string]any),
	}

	// Configure SNS notifier if provided
	if len(opts) > 0 && opts[0].Notifier != nil {
		zapLogger.snsNotifier = opts[0].Notifier
	}

	return zapLogger, nil
}

// buildZapConfig creates a Zap configuration from our LoggerConfig
func buildZapConfig(config observability.LoggerConfig) zap.Config {
	var level zapcore.Level
	switch config.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel // Default to info level for production security
	}

	zapConfig := zap.Config{
		Level:       zap.NewAtomicLevelAt(level),
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding: "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "timestamp",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "message",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	if config.Format == formatConsole {
		zapConfig.Encoding = formatConsole
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	return zapConfig
}

// Debug logs a debug message (with enhanced sanitization for security)
func (z *ZapLogger) Debug(message string, fields ...map[string]any) {
	z.log(zapcore.DebugLevel, message, fields...)
}

// Info logs an info message
func (z *ZapLogger) Info(message string, fields ...map[string]any) {
	z.log(zapcore.InfoLevel, message, fields...)
}

// Warn logs a warning message
func (z *ZapLogger) Warn(message string, fields ...map[string]any) {
	z.log(zapcore.WarnLevel, message, fields...)
}

// Error logs an error message
func (z *ZapLogger) Error(message string, fields ...map[string]any) {
	z.log(zapcore.ErrorLevel, message, fields...)

	// Send SNS notification asynchronously if configured
	if z.snsNotifier != nil {
		// Create a LogEntry for the SNS notifier
		entry := &observability.LogEntry{
			Timestamp: time.Now().UTC(),
			Level:     "ERROR",
			Message:   message,
			Fields:    z.mergeFields(fields...),
		}

		// Async SNS notification to avoid blocking the logger
		go func(e *observability.LogEntry) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := z.snsNotifier.NotifyError(ctx, e); err != nil {
				// Log SNS error but don't block
				atomic.AddInt64(&z.stats.errorCount, 1)
				z.stats.lastError = fmt.Sprintf("SNS notification failed: %v", err)
			}
		}(entry)
	}
}

// WithField returns a new logger with an additional field
func (z *ZapLogger) WithField(key string, value any) lift.Logger {
	return z.WithFields(map[string]any{key: value})
}

// WithFields returns a new logger with additional fields
func (z *ZapLogger) WithFields(fields map[string]any) lift.Logger {
	newFields := make(map[string]any)
	for k, v := range z.contextFields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &ZapLogger{
		logger:        z.logger,
		sugar:         z.sugar,
		config:        z.config,
		stats:         z.stats,
		contextFields: newFields,
		snsNotifier:   z.snsNotifier, // Share SNS notifier
	}
}

// WithRequestID adds request ID to logger context
func (z *ZapLogger) WithRequestID(requestID string) observability.StructuredLogger {
	logger := z.WithField("request_id", requestID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return z
}

// WithTenantID adds tenant ID to logger context
func (z *ZapLogger) WithTenantID(tenantID string) observability.StructuredLogger {
	logger := z.WithField("tenant_id", tenantID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return z
}

// WithUserID adds user ID to logger context
func (z *ZapLogger) WithUserID(userID string) observability.StructuredLogger {
	logger := z.WithField("user_id", userID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return z
}

// WithTraceID adds trace ID to logger context
func (z *ZapLogger) WithTraceID(traceID string) observability.StructuredLogger {
	logger := z.WithField("trace_id", traceID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return z
}

// WithSpanID adds span ID to logger context
func (z *ZapLogger) WithSpanID(spanID string) observability.StructuredLogger {
	logger := z.WithField("span_id", spanID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return z
}

// log is the internal logging method
func (z *ZapLogger) log(level zapcore.Level, message string, fieldMaps ...map[string]any) {
	// Increment counter
	atomic.AddInt64(&z.stats.entriesLogged, 1)

	// Build fields slice
	// Calculate total capacity needed
	totalFields := len(z.contextFields)
	for _, fieldMap := range fieldMaps {
		totalFields += len(fieldMap)
	}
	zapFields := make([]zap.Field, 0, totalFields)

	// Add context fields
	for k, v := range z.contextFields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	// Add provided fields (sanitize sensitive data)
	for _, fieldMap := range fieldMaps {
		for k, v := range fieldMap {
			// Sanitize field values to prevent sensitive data leakage
			sanitizedValue := z.sanitizeFieldValue(k, v)
			zapFields = append(zapFields, zap.Any(k, sanitizedValue))
		}
	}

	// Log with appropriate level (all levels supported with enhanced sanitization)
	switch level {
	case zapcore.DebugLevel:
		z.logger.Debug(message, zapFields...)
	case zapcore.InfoLevel:
		z.logger.Info(message, zapFields...)
	case zapcore.WarnLevel:
		z.logger.Warn(message, zapFields...)
	case zapcore.ErrorLevel:
		z.logger.Error(message, zapFields...)
	}
}

// sanitizeFieldValue sanitizes field values to prevent sensitive data exposure
func (z *ZapLogger) sanitizeFieldValue(key string, value any) any {
	return sanitization.SanitizeFieldValue(key, value)
}

// Flush syncs the logger (Zap handles this automatically)
func (z *ZapLogger) Flush(_ context.Context) error {
	atomic.AddInt64(&z.stats.flushCount, 1)
	atomic.StoreInt64(&z.stats.lastFlush, time.Now().Unix())
	return z.logger.Sync()
}

// Close closes the logger
func (z *ZapLogger) Close() error {
	return z.logger.Sync()
}

// IsHealthy returns true if the logger is functioning properly
func (z *ZapLogger) IsHealthy() bool {
	return atomic.LoadInt64(&z.stats.errorCount) == 0
}

// GetStats returns logger performance statistics
func (z *ZapLogger) GetStats() observability.LoggerStats {
	return observability.LoggerStats{
		EntriesLogged:    atomic.LoadInt64(&z.stats.entriesLogged),
		EntriesDropped:   atomic.LoadInt64(&z.stats.entriesDropped),
		FlushCount:       atomic.LoadInt64(&z.stats.flushCount),
		LastFlush:        time.Unix(atomic.LoadInt64(&z.stats.lastFlush), 0),
		BufferSize:       0, // Zap manages its own buffering
		BufferCapacity:   0,
		AverageFlushTime: 0, // Would need more complex tracking
		ErrorCount:       atomic.LoadInt64(&z.stats.errorCount),
		LastError:        z.stats.lastError,
	}
}

// mergeFields merges context fields with additional field maps
func (z *ZapLogger) mergeFields(fieldMaps ...map[string]any) map[string]any {
	merged := make(map[string]any)

	// Start with context fields
	for k, v := range z.contextFields {
		merged[k] = v
	}

	// Merge additional fields
	for _, fields := range fieldMaps {
		for k, v := range fields {
			merged[k] = v
		}
	}

	return merged
}

// ZapLoggerFactory implements the LoggerFactory interface
type ZapLoggerFactory struct{}

// NewZapLoggerFactory creates a new Zap logger factory
func NewZapLoggerFactory() *ZapLoggerFactory {
	return &ZapLoggerFactory{}
}

// CreateConsoleLogger creates a console-based logger
func (f *ZapLoggerFactory) CreateConsoleLogger(config observability.LoggerConfig) (observability.StructuredLogger, error) {
	config.Format = "console"
	return NewZapLogger(config) // No options for console logger
}

// CreateCloudWatchLogger creates a CloudWatch-integrated logger
func (f *ZapLoggerFactory) CreateCloudWatchLogger(config observability.LoggerConfig, _ observability.CloudWatchLogsClient) (observability.StructuredLogger, error) {
	// For now, return a console logger - CloudWatch integration will be in the CloudWatch package
	config.Format = "json"
	logger, err := NewZapLogger(config) // No SNS options in factory method
	if err != nil {
		return nil, err
	}
	return logger, nil
}

// CreateTestLogger creates a logger suitable for testing
func (f *ZapLoggerFactory) CreateTestLogger() observability.StructuredLogger {
	config := observability.LoggerConfig{
		Level:  "debug", // Debug level available for testing with enhanced sanitization
		Format: "json",
	}
	logger, err := NewZapLogger(config) // No options for test logger
	if err != nil {
		// Fall back to a no-op logger for tests if creation fails
		return &NoOpStructuredLogger{&lift.NoOpLogger{}}
	}
	return logger
}

// CreateNoOpLogger creates a no-op logger
func (f *ZapLoggerFactory) CreateNoOpLogger() observability.StructuredLogger {
	return &NoOpStructuredLogger{}
}

// NoOpStructuredLogger is a no-op implementation for testing
type NoOpStructuredLogger struct {
	*lift.NoOpLogger
}

func (n *NoOpStructuredLogger) WithRequestID(_ string) observability.StructuredLogger {
	return n
}
func (n *NoOpStructuredLogger) WithTenantID(_ string) observability.StructuredLogger { return n }
func (n *NoOpStructuredLogger) WithUserID(_ string) observability.StructuredLogger   { return n }
func (n *NoOpStructuredLogger) WithTraceID(_ string) observability.StructuredLogger  { return n }
func (n *NoOpStructuredLogger) WithSpanID(_ string) observability.StructuredLogger   { return n }
func (n *NoOpStructuredLogger) Flush(_ context.Context) error                        { return nil }
func (n *NoOpStructuredLogger) Close() error                                         { return nil }
func (n *NoOpStructuredLogger) IsHealthy() bool                                      { return true }
func (n *NoOpStructuredLogger) GetStats() observability.LoggerStats {
	return observability.LoggerStats{}
}
