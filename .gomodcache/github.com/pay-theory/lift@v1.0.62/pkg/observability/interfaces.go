package observability

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/pay-theory/lift/pkg/lift"
)

// CloudWatchLogsClient defines the interface for CloudWatch Logs operations
// This interface allows for easy mocking and testing
type CloudWatchLogsClient interface {
	CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error)
	CreateLogStream(ctx context.Context, params *cloudwatchlogs.CreateLogStreamInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error)
	PutLogEvents(ctx context.Context, params *cloudwatchlogs.PutLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error)
	DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
	DescribeLogStreams(ctx context.Context, params *cloudwatchlogs.DescribeLogStreamsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error)
}

// SNSClient defines the interface for SNS operations
// This interface allows for easy mocking and testing
type SNSClient interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

// LogEntry represents a structured log entry with multi-tenant context
type LogEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	TenantID  string         `json:"tenant_id,omitempty"`
	UserID    string         `json:"user_id,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	SpanID    string         `json:"span_id,omitempty"`
}

// LogBuffer defines the interface for buffering log entries
type LogBuffer interface {
	Add(entry *LogEntry) error
	Flush(ctx context.Context) error
	Size() int
	IsFull() bool
	Clear()
	Close() error
}

// LogSink defines the interface for sending logs to a destination
type LogSink interface {
	Send(ctx context.Context, entries []*LogEntry) error
	Close() error
}

// StructuredLogger extends the basic lift.Logger with additional context methods
type StructuredLogger interface {
	// Explicitly declare lift.Logger methods to avoid interface embedding issues
	Debug(message string, fields ...map[string]any)
	Info(message string, fields ...map[string]any)
	Warn(message string, fields ...map[string]any)
	Error(message string, fields ...map[string]any)
	WithField(key string, value any) lift.Logger
	WithFields(fields map[string]any) lift.Logger

	// Context methods for multi-tenant logging
	WithRequestID(requestID string) StructuredLogger
	WithTenantID(tenantID string) StructuredLogger
	WithUserID(userID string) StructuredLogger
	WithTraceID(traceID string) StructuredLogger
	WithSpanID(spanID string) StructuredLogger

	// Performance and health methods
	Flush(ctx context.Context) error
	Close() error
	IsHealthy() bool
	GetStats() LoggerStats
}

// LoggerStats provides metrics about logger performance
type LoggerStats struct {
	LastFlush        time.Time     `json:"last_flush"`
	LastError        string        `json:"last_error,omitempty"`
	EntriesLogged    int64         `json:"entries_logged"`
	EntriesDropped   int64         `json:"entries_dropped"`
	FlushCount       int64         `json:"flush_count"`
	BufferSize       int           `json:"buffer_size"`
	BufferCapacity   int           `json:"buffer_capacity"`
	AverageFlushTime time.Duration `json:"average_flush_time"`
	ErrorCount       int64         `json:"error_count"`
}

// LoggerConfig holds configuration for logger implementations
type LoggerConfig struct {
	Format          string        `json:"format"`
	DefaultUserID   string        `json:"default_user_id"`
	Level           string        `json:"level"`
	LogGroup        string        `json:"log_group"`
	LogStream       string        `json:"log_stream"`
	DefaultTenantID string        `json:"default_tenant_id"`
	RetryDelay      time.Duration `json:"retry_delay"`
	BatchSize       int           `json:"batch_size"`
	FlushInterval   time.Duration `json:"flush_interval"`
	BufferSize      int           `json:"buffer_size"`
	MaxRetries      int           `json:"max_retries"`
	EnableStack     bool          `json:"enable_stack"`
	AsyncLogging    bool          `json:"async_logging"`
	EnableCaller    bool          `json:"enable_caller"`
}

// LoggerFactory creates logger instances with different configurations
type LoggerFactory interface {
	CreateConsoleLogger(config LoggerConfig) (StructuredLogger, error)
	CreateCloudWatchLogger(config LoggerConfig, client CloudWatchLogsClient) (StructuredLogger, error)
	CreateTestLogger() StructuredLogger
	CreateNoOpLogger() StructuredLogger
}

// MetricEntry represents a metric data point
type MetricEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Tags      map[string]string `json:"tags,omitempty"`
	Fields    map[string]any    `json:"fields,omitempty"`
	Name      string            `json:"name"`
	Unit      string            `json:"unit"`
	Value     float64           `json:"value"`
}

// MetricsCollector extends the basic lift.MetricsCollector with additional functionality
type MetricsCollector interface {
	// Explicitly declare lift.MetricsCollector methods to avoid interface embedding issues
	Counter(name string, tags ...map[string]string) lift.Counter
	Histogram(name string, tags ...map[string]string) lift.Histogram
	Gauge(name string, tags ...map[string]string) lift.Gauge
	Flush() error

	// Additional methods for enhanced functionality
	RecordLatency(operation string, duration time.Duration)
	RecordError(operation string)
	RecordSuccess(operation string)

	// Context methods
	WithTags(tags map[string]string) MetricsCollector
	WithTag(key, value string) MetricsCollector

	// Batch operations
	RecordBatch(entries []*MetricEntry) error

	// Performance methods
	Close() error
	GetStats() MetricsStats
}

// MetricsStats provides information about metrics collection
type MetricsStats struct {
	LastFlush       time.Time `json:"last_flush"`
	LastError       string    `json:"last_error,omitempty"`
	MetricsRecorded int64     `json:"metrics_recorded"`
	MetricsDropped  int64     `json:"metrics_dropped"`
	ErrorCount      int64     `json:"error_count"`
}

// HealthChecker defines the interface for health checking
type HealthChecker interface {
	Check(ctx context.Context) error
	Name() string
	IsCritical() bool
}

// HealthStatus represents the overall health status
type HealthStatus struct {
	Timestamp time.Time              `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks"`
	Version   string                 `json:"version,omitempty"`
	Healthy   bool                   `json:"healthy"`
}

// CheckResult represents the result of a single health check
type CheckResult struct {
	Message  string        `json:"message,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Healthy  bool          `json:"healthy"`
	Critical bool          `json:"critical"`
}

// ObservabilityProvider combines logging, metrics, and health checking
type ObservabilityProvider interface {
	Logger() StructuredLogger
	Metrics() MetricsCollector
	HealthChecker() HealthChecker
	Close() error
}

// TestObservabilityProvider provides test implementations
type TestObservabilityProvider interface {
	ObservabilityProvider

	// Test-specific methods
	GetLogEntries() []*LogEntry
	GetMetricEntries() []*MetricEntry
	ClearLogs()
	ClearMetrics()
	SimulateError(component string, err error)
	GetCallCount(method string) int
}
