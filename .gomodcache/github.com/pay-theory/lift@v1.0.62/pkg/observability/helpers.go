package observability

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NewDefaultLoggerConfig creates a LoggerConfig with sensible defaults for Lambda environments.
// It accepts a log level parameter and automatically configures CloudWatch log group and stream
// based on environment variables.
func NewDefaultLoggerConfig(level string) LoggerConfig {
	// Generate log group name from environment variables
	functionName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	partner := os.Getenv("PARTNER")
	stage := os.Getenv("STAGE")

	logGroup := fmt.Sprintf("/aws/lambda/%s-%s-%s", functionName, partner, stage)

	// Generate log stream name with timestamp and version
	version := os.Getenv("AWS_LAMBDA_FUNCTION_VERSION")
	if version == "" {
		version = "$LATEST"
	}

	// Create a unique log stream identifier
	streamID := strings.ReplaceAll(uuid.New().String(), "-", "")
	logStream := fmt.Sprintf("%s/[%s]%s",
		time.Now().Format("2006/01/02"),
		version,
		streamID,
	)

	return LoggerConfig{
		Level:         level,
		Format:        "json",
		LogGroup:      logGroup,
		LogStream:     logStream,
		BatchSize:     10,
		FlushInterval: 2 * time.Second,
		BufferSize:    50,
	}
}

// NewLoggerConfigWithOptions creates a LoggerConfig with custom options while maintaining defaults.
// It accepts a log level and allows overriding specific configuration options.
func NewLoggerConfigWithOptions(level string, opts ...LoggerConfigOption) LoggerConfig {
	// Start with defaults
	config := NewDefaultLoggerConfig(level)

	// Apply options
	for _, opt := range opts {
		opt(&config)
	}

	return config
}

// LoggerConfigOption is a functional option for customizing LoggerConfig
type LoggerConfigOption func(*LoggerConfig)

// WithLogGroup sets a custom log group
func WithLogGroup(logGroup string) LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.LogGroup = logGroup
	}
}

// WithLogStream sets a custom log stream
func WithLogStream(logStream string) LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.LogStream = logStream
	}
}

// WithBatchSize sets a custom batch size
func WithBatchSize(size int) LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.BatchSize = size
	}
}

// WithFlushInterval sets a custom flush interval
func WithFlushInterval(interval time.Duration) LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.FlushInterval = interval
	}
}

// WithBufferSize sets a custom buffer size
func WithBufferSize(size int) LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.BufferSize = size
	}
}

// WithFormat sets the log format (json or console)
func WithFormat(format string) LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.Format = format
	}
}

// WithTenantContext sets default tenant and user IDs
func WithTenantContext(tenantID, userID string) LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.DefaultTenantID = tenantID
		c.DefaultUserID = userID
	}
}

// WithAsyncLogging enables async logging
func WithAsyncLogging() LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.AsyncLogging = true
	}
}

// WithRetryConfig sets retry configuration
func WithRetryConfig(maxRetries int, retryDelay time.Duration) LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.MaxRetries = maxRetries
		c.RetryDelay = retryDelay
	}
}

// WithCallerInfo enables caller information in logs
func WithCallerInfo() LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.EnableCaller = true
	}
}

// WithStackTrace enables stack traces for errors
func WithStackTrace() LoggerConfigOption {
	return func(c *LoggerConfig) {
		c.EnableStack = true
	}
}
