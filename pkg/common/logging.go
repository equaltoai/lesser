package common

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func init() {
	// Initialize logger with Lambda-optimized configuration
	cfg := zap.NewProductionConfig()

	// Set log level from environment
	logLevel := os.Getenv("LOG_LEVEL")
	switch logLevel {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Optimize for Lambda environment
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var err error
	logger, err = cfg.Build()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
}

// Logger returns the global logger instance
func Logger() *zap.Logger {
	return logger
}

// WithContext returns a logger with Lambda context fields
func WithContext(ctx context.Context) *zap.Logger {
	lc, ok := lambdacontext.FromContext(ctx)
	if !ok {
		return logger
	}

	return logger.With(
		zap.String("request_id", lc.AwsRequestID),
		zap.String("function_name", lambdacontext.FunctionName),
		zap.String("function_version", lambdacontext.FunctionVersion),
	)
}

// WithFields returns a logger with additional fields
func WithFields(fields ...zap.Field) *zap.Logger {
	return logger.With(fields...)
}

// Sync flushes any buffered log entries
func Sync() {
	_ = logger.Sync()
}
