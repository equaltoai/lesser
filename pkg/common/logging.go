// Package common provides shared logging utilities for the Lesser application.
package common

import (
	"context"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/equaltoai/lesser/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func init() {
	// Initialize logger with Lambda-optimized configuration
	zapCfg := zap.NewProductionConfig()

	// Set log level from centralized config
	appCfg := config.Get()
	logLevel := appCfg.LogLevel
	switch logLevel {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Optimize for Lambda environment
	zapCfg.OutputPaths = []string{"stdout"}
	zapCfg.ErrorOutputPaths = []string{"stderr"}
	zapCfg.EncoderConfig.TimeKey = "timestamp"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var err error
	logger, err = zapCfg.Build()
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
