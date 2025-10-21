package cloudwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
	"github.com/pay-theory/lift/pkg/utils/sanitization"
)

// sharedLoggerState contains state shared between logger instances created via WithFields
type sharedLoggerState struct {
	buffer        chan *observability.LogEntry
	flushSignal   chan struct{}
	done          chan struct{}
	wg            *sync.WaitGroup
	mu            *sync.RWMutex
	sequenceToken *string
	stats         *loggerStats
	closeOnce     *sync.Once
	closed        *int32
}

// CloudWatchLogger implements the StructuredLogger interface with CloudWatch Logs backend
type CloudWatchLogger struct {
	client        observability.CloudWatchLogsClient
	contextFields map[string]any
	snsNotifier   *observability.SNSNotifier
	shared        *sharedLoggerState
	logGroup      string
	logStream     string
	batchSize     int
	flushInterval time.Duration
}

// loggerStats tracks performance metrics
type loggerStats struct {
	lastError        string
	entriesLogged    int64
	entriesDropped   int64
	flushCount       int64
	lastFlush        int64
	errorCount       int64
	averageFlushTime int64
}

// CloudWatchLoggerOptions contains optional parameters for NewCloudWatchLogger
type CloudWatchLoggerOptions struct {
	Notifier *observability.SNSNotifier
}

// NewCloudWatchLogger creates a new CloudWatch logger instance
func NewCloudWatchLogger(config observability.LoggerConfig, client observability.CloudWatchLogsClient, opts ...CloudWatchLoggerOptions) (observability.StructuredLogger, error) {
	if config.BatchSize <= 0 {
		config.BatchSize = 25 // CloudWatch Logs max batch size
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}
	if config.BufferSize <= 0 {
		config.BufferSize = config.BatchSize * 2
	}

	var closed int32
	logger := &CloudWatchLogger{
		client:        client,
		logGroup:      config.LogGroup,
		logStream:     config.LogStream,
		batchSize:     config.BatchSize,
		flushInterval: config.FlushInterval,
		contextFields: make(map[string]any),
		shared: &sharedLoggerState{
			buffer:        make(chan *observability.LogEntry, config.BufferSize),
			flushSignal:   make(chan struct{}),
			done:          make(chan struct{}),
			wg:            &sync.WaitGroup{},
			mu:            &sync.RWMutex{},
			sequenceToken: nil,
			stats:         &loggerStats{},
			closeOnce:     &sync.Once{},
			closed:        &closed,
		},
	}

	// Configure SNS notifier if provided
	if len(opts) > 0 && opts[0].Notifier != nil {
		logger.snsNotifier = opts[0].Notifier
	}

	// Ensure log group and stream exist
	if err := logger.ensureLogGroupExists(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ensure log group exists: %w", err)
	}

	if err := logger.ensureLogStreamExists(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ensure log stream exists: %w", err)
	}

	// Start background flusher
	logger.shared.wg.Add(1)
	go logger.flushLoop()

	return logger, nil
}

// Debug logs a debug message (with enhanced sanitization for security)
func (l *CloudWatchLogger) Debug(message string, fields ...map[string]any) {
	l.log("DEBUG", message, fields...)
}

// Info logs an info message
func (l *CloudWatchLogger) Info(message string, fields ...map[string]any) {
	l.log("INFO", message, fields...)
}

// Warn logs a warning message
func (l *CloudWatchLogger) Warn(message string, fields ...map[string]any) {
	l.log("WARN", message, fields...)
}

// Error logs an error message
func (l *CloudWatchLogger) Error(message string, fields ...map[string]any) {
	l.log("ERROR", message, fields...)
}

// WithField returns a new logger with an additional field
func (l *CloudWatchLogger) WithField(key string, value any) lift.Logger {
	return l.WithFields(map[string]any{key: value})
}

// WithFields returns a new logger with additional fields
func (l *CloudWatchLogger) WithFields(fields map[string]any) lift.Logger {
	newFields := make(map[string]any)
	for k, v := range l.contextFields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &CloudWatchLogger{
		client:        l.client,
		logGroup:      l.logGroup,
		logStream:     l.logStream,
		batchSize:     l.batchSize,
		flushInterval: l.flushInterval,
		contextFields: newFields,     // Only context fields are different
		snsNotifier:   l.snsNotifier, // Share SNS notifier
		shared:        l.shared,      // Share all state
	}
}

// WithRequestID adds request ID to logger context
func (l *CloudWatchLogger) WithRequestID(requestID string) observability.StructuredLogger {
	logger := l.WithField("request_id", requestID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return l
}

// WithTenantID adds tenant ID to logger context
func (l *CloudWatchLogger) WithTenantID(tenantID string) observability.StructuredLogger {
	logger := l.WithField("tenant_id", tenantID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return l
}

// WithUserID adds user ID to logger context
func (l *CloudWatchLogger) WithUserID(userID string) observability.StructuredLogger {
	logger := l.WithField("user_id", userID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return l
}

// WithTraceID adds trace ID to logger context
func (l *CloudWatchLogger) WithTraceID(traceID string) observability.StructuredLogger {
	logger := l.WithField("trace_id", traceID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return l
}

// WithSpanID adds span ID to logger context
func (l *CloudWatchLogger) WithSpanID(spanID string) observability.StructuredLogger {
	logger := l.WithField("span_id", spanID)
	if structured, ok := logger.(observability.StructuredLogger); ok {
		return structured
	}
	return l
}

// log is the internal logging method
func (l *CloudWatchLogger) log(level, message string, fieldMaps ...map[string]any) {
	builder := newLogEntryBuilder(l, level, message, fieldMaps)
	builder.build()
}

// logEntryBuilder builds and processes log entries
type logEntryBuilder struct {
	logger    *CloudWatchLogger
	entry     *observability.LogEntry
	level     string
	message   string
	fieldMaps []map[string]any
}

// newLogEntryBuilder creates a new log entry builder
func newLogEntryBuilder(logger *CloudWatchLogger, level, message string, fieldMaps []map[string]any) *logEntryBuilder {
	return &logEntryBuilder{
		logger:    logger,
		level:     level,
		message:   message,
		fieldMaps: fieldMaps,
	}
}

// build constructs and processes the log entry
func (b *logEntryBuilder) build() {
	if !b.shouldLog() {
		return
	}

	b.createEntry()
	b.addContextFields()
	b.mergeFieldMaps()
	b.submitEntry()
}

// shouldLog checks if logging should proceed
func (b *logEntryBuilder) shouldLog() bool {
	return atomic.LoadInt32(b.logger.shared.closed) == 0
}

// createEntry creates the base log entry
func (b *logEntryBuilder) createEntry() {
	b.entry = &observability.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     b.level,
		Message:   b.message,
		Fields:    make(map[string]any),
	}
}

// addContextFields adds context fields to the entry
func (b *logEntryBuilder) addContextFields() {
	for k, v := range b.logger.contextFields {
		b.processContextField(k, v)
	}
}

// processContextField handles a single context field
func (b *logEntryBuilder) processContextField(key string, value any) {
	switch key {
	case "request_id":
		b.setStringField(&b.entry.RequestID, value)
	case "tenant_id":
		b.setStringField(&b.entry.TenantID, value)
	case "user_id":
		b.setStringField(&b.entry.UserID, value)
	case "trace_id":
		b.setStringField(&b.entry.TraceID, value)
	case "span_id":
		b.setStringField(&b.entry.SpanID, value)
	default:
		b.entry.Fields[key] = b.logger.sanitizeFieldValue(key, value)
	}
}

// setStringField sets a string field if the value is a string
func (b *logEntryBuilder) setStringField(field *string, value any) {
	if s, ok := value.(string); ok {
		*field = s
	}
}

// mergeFieldMaps merges all provided field maps
func (b *logEntryBuilder) mergeFieldMaps() {
	for _, fieldMap := range b.fieldMaps {
		for k, v := range fieldMap {
			b.entry.Fields[k] = b.logger.sanitizeFieldValue(k, v)
		}
	}
}

// submitEntry sends the entry to the buffer and handles notifications
func (b *logEntryBuilder) submitEntry() {
	select {
	case b.logger.shared.buffer <- b.entry:
		atomic.AddInt64(&b.logger.shared.stats.entriesLogged, 1)
		b.handleErrorNotification()
	default:
		atomic.AddInt64(&b.logger.shared.stats.entriesDropped, 1)
	}
}

// handleErrorNotification sends SNS notification for errors if configured
func (b *logEntryBuilder) handleErrorNotification() {
	if b.level != "ERROR" || b.logger.snsNotifier == nil {
		return
	}

	go b.sendAsyncNotification(b.entry)
}

// sendAsyncNotification sends the notification asynchronously
func (b *logEntryBuilder) sendAsyncNotification(e *observability.LogEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := b.logger.snsNotifier.NotifyError(ctx, e); err != nil {
		atomic.AddInt64(&b.logger.shared.stats.errorCount, 1)
		b.logger.shared.stats.lastError = fmt.Sprintf("SNS notification failed: %v", err)
	}
}

// sanitizeFieldValue sanitizes field values to prevent sensitive data exposure
func (l *CloudWatchLogger) sanitizeFieldValue(key string, value any) any {
	return sanitization.SanitizeFieldValue(key, value)
}

// flushLoop runs in background to batch and send logs
func (l *CloudWatchLogger) flushLoop() {
	defer l.shared.wg.Done()

	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	handler := newLogBatchHandler(l)

	for {
		select {
		case entry := <-l.shared.buffer:
			handler.addEntry(entry)

		case <-l.shared.flushSignal:
			handler.flushIfNeeded()

		case <-ticker.C:
			handler.flushIfNeeded()

		case <-l.shared.done:
			handler.shutdown(l.shared.buffer)
			return
		}
	}
}

// logBatchHandler manages batching and flushing of log entries
type logBatchHandler struct {
	logger    *CloudWatchLogger
	batch     []*observability.LogEntry
	batchSize int
}

// newLogBatchHandler creates a new log batch handler
func newLogBatchHandler(logger *CloudWatchLogger) *logBatchHandler {
	return &logBatchHandler{
		logger:    logger,
		batch:     make([]*observability.LogEntry, 0),
		batchSize: logger.batchSize,
	}
}

// addEntry adds an entry to the batch and flushes if needed
func (h *logBatchHandler) addEntry(entry *observability.LogEntry) {
	h.batch = append(h.batch, entry)
	if len(h.batch) >= h.batchSize {
		h.flushAndReset()
	}
}

// flushIfNeeded flushes the batch if it has entries
func (h *logBatchHandler) flushIfNeeded() {
	if len(h.batch) > 0 {
		h.flushAndReset()
	}
}

// flushAndReset flushes the current batch and resets it
func (h *logBatchHandler) flushAndReset() {
	h.logger.flushBatch(h.batch)
	h.batch = h.batch[:0] // Reset slice
}

// shutdown handles final cleanup and draining
func (h *logBatchHandler) shutdown(buffer chan *observability.LogEntry) {
	// Flush current batch
	h.flushIfNeeded()

	// Drain remaining buffer entries
	h.drainBuffer(buffer)
}

// drainBuffer drains remaining entries from the buffer
func (h *logBatchHandler) drainBuffer(buffer chan *observability.LogEntry) {
	for {
		select {
		case entry := <-buffer:
			h.addEntry(entry)
		default:
			h.flushIfNeeded()
			return
		}
	}
}

// flushBatch sends a batch of log entries to CloudWatch
func (l *CloudWatchLogger) flushBatch(batch []*observability.LogEntry) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()

	defer func() {
		duration := time.Since(start)
		atomic.AddInt64(&l.shared.stats.flushCount, 1)
		atomic.StoreInt64(&l.shared.stats.lastFlush, time.Now().Unix())

		// Update average flush time
		currentAvg := atomic.LoadInt64(&l.shared.stats.averageFlushTime)
		flushCount := atomic.LoadInt64(&l.shared.stats.flushCount)
		newAvg := (currentAvg*(flushCount-1) + duration.Nanoseconds()) / flushCount
		atomic.StoreInt64(&l.shared.stats.averageFlushTime, newAvg)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Convert entries to CloudWatch format
	events := make([]types.InputLogEvent, len(batch))
	for i, entry := range batch {
		message, err := json.Marshal(entry)
		if err != nil {
			// Fallback to string representation if JSON marshaling fails
			message = []byte(fmt.Sprintf(`{"error": "failed to marshal log entry", "level": "%s", "message": "%s"}`, entry.Level, entry.Message))
		}
		events[i] = types.InputLogEvent{
			Message:   aws.String(string(message)),
			Timestamp: aws.Int64(entry.Timestamp.UnixMilli()),
		}
	}

	// Send to CloudWatch
	l.shared.mu.Lock()
	input := &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(l.logGroup),
		LogStreamName: aws.String(l.logStream),
		LogEvents:     events,
		SequenceToken: l.shared.sequenceToken,
	}
	l.shared.mu.Unlock()

	output, err := l.client.PutLogEvents(ctx, input)
	if err != nil {
		atomic.AddInt64(&l.shared.stats.errorCount, 1)
		l.shared.stats.lastError = err.Error()
		return
	}

	// Update sequence token
	l.shared.mu.Lock()
	l.shared.sequenceToken = output.NextSequenceToken
	l.shared.mu.Unlock()
}

// ensureLogGroupExists creates the log group if it doesn't exist
func (l *CloudWatchLogger) ensureLogGroupExists(ctx context.Context) error {
	_, err := l.client.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(l.logGroup),
	})

	// Ignore if already exists
	if err != nil {
		var alreadyExists *types.ResourceAlreadyExistsException
		if !errors.As(err, &alreadyExists) {
			return err
		}
	}

	return nil
}

// ensureLogStreamExists creates the log stream if it doesn't exist
func (l *CloudWatchLogger) ensureLogStreamExists(ctx context.Context) error {
	_, err := l.client.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(l.logGroup),
		LogStreamName: aws.String(l.logStream),
	})

	// Ignore if already exists
	if err != nil {
		var alreadyExists *types.ResourceAlreadyExistsException
		if !errors.As(err, &alreadyExists) {
			return err
		}
	}

	return nil
}

// Flush forces a flush of buffered log entries
func (l *CloudWatchLogger) Flush(ctx context.Context) error {
	// Give a small delay to ensure any recent log entries have been buffered
	time.Sleep(25 * time.Millisecond)

	// Send flush signal to background loop
	select {
	case l.shared.flushSignal <- struct{}{}:
		// Signal sent successfully
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
		return fmt.Errorf("flush signal timeout")
	}

	// Give a little time for the background loop to process the flush
	time.Sleep(50 * time.Millisecond)

	return nil
}

// Close gracefully shuts down the logger
func (l *CloudWatchLogger) Close() error {
	var err error
	l.shared.closeOnce.Do(func() {
		// Mark as closed to prevent new logs
		atomic.StoreInt32(l.shared.closed, 1)

		// Signal shutdown
		close(l.shared.done)

		// Wait for the flush loop to finish
		l.shared.wg.Wait()

		// Close the buffer channel after the flush loop has exited
		close(l.shared.buffer)
	})
	return err
}

// IsHealthy returns true if the logger is functioning properly
func (l *CloudWatchLogger) IsHealthy() bool {
	errorCount := atomic.LoadInt64(&l.shared.stats.errorCount)
	entriesLogged := atomic.LoadInt64(&l.shared.stats.entriesLogged)

	// Consider healthy if:
	// 1. No errors at all, OR
	// 2. We haven't logged anything yet, OR
	// 3. Error rate is less than 10% of total entries
	if errorCount == 0 {
		return true
	}
	if entriesLogged == 0 {
		return true
	}

	errorRate := float64(errorCount) / float64(entriesLogged)
	return errorRate < 0.1 // Less than 10% error rate
}

// GetStats returns logger performance statistics
func (l *CloudWatchLogger) GetStats() observability.LoggerStats {
	return observability.LoggerStats{
		EntriesLogged:    atomic.LoadInt64(&l.shared.stats.entriesLogged),
		EntriesDropped:   atomic.LoadInt64(&l.shared.stats.entriesDropped),
		FlushCount:       atomic.LoadInt64(&l.shared.stats.flushCount),
		LastFlush:        time.Unix(atomic.LoadInt64(&l.shared.stats.lastFlush), 0),
		BufferSize:       len(l.shared.buffer),
		BufferCapacity:   cap(l.shared.buffer),
		AverageFlushTime: time.Duration(atomic.LoadInt64(&l.shared.stats.averageFlushTime)),
		ErrorCount:       atomic.LoadInt64(&l.shared.stats.errorCount),
		LastError:        l.shared.stats.lastError,
	}
}
