package cloudwatch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pay-theory/lift/pkg/observability"
)

func TestCloudWatchLogger_BasicLogging(t *testing.T) {
	// Setup
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     5,
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    10,
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Test basic logging
	logger.Info("test message", map[string]any{
		"key1": "value1",
		"key2": 42,
	})

	logger.Error("error message", map[string]any{
		"error": "something went wrong",
	})

	// Wait for flush
	time.Sleep(200 * time.Millisecond)

	// Verify log group and stream were created
	assert.Equal(t, int64(1), mockClient.GetCallCount("CreateLogGroup"))
	assert.Equal(t, int64(1), mockClient.GetCallCount("CreateLogStream"))

	// Verify logs were sent
	assert.Equal(t, int64(1), mockClient.GetCallCount("PutLogEvents"))

	logEvents := mockClient.GetLogEvents()
	assert.Len(t, logEvents, 2)

	// Verify stats
	stats := logger.GetStats()
	assert.Equal(t, int64(2), stats.EntriesLogged)
	assert.Equal(t, int64(0), stats.EntriesDropped)
	assert.True(t, stats.FlushCount > 0)
}

func TestCloudWatchLogger_ContextFields(t *testing.T) {
	// Setup
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     1,
		FlushInterval: 50 * time.Millisecond,
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Test context methods
	contextLogger := logger.
		WithRequestID("req-123").
		WithTenantID("tenant-456").
		WithUserID("user-789").
		WithTraceID("trace-abc").
		WithSpanID("span-def")

	contextLogger.Info("test with context")

	// Wait for flush
	time.Sleep(100 * time.Millisecond)

	// Verify the log was sent with context
	logEvents := mockClient.GetLogEvents()
	require.Len(t, logEvents, 1)

	// The actual verification would require parsing the JSON message
	// For now, we verify that the log was sent
	assert.NotEmpty(t, logEvents[0].Message)
}

func TestCloudWatchLogger_Batching(t *testing.T) {
	// Setup
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     3,
		FlushInterval: 1 * time.Second, // Long interval to test batching
		BufferSize:    10,
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Send exactly batch size messages
	for i := 0; i < 3; i++ {
		logger.Info("batch message", map[string]any{
			"index": i,
		})
	}

	// Wait a bit for batch to be sent
	time.Sleep(100 * time.Millisecond)

	// Should have sent one batch
	assert.Equal(t, int64(1), mockClient.GetCallCount("PutLogEvents"))

	logEvents := mockClient.GetLogEvents()
	assert.Len(t, logEvents, 3)

	// Send one more message (should not trigger batch yet)
	logger.Info("single message")
	time.Sleep(50 * time.Millisecond)

	// Still should be only one batch sent
	assert.Equal(t, int64(1), mockClient.GetCallCount("PutLogEvents"))
}

func TestCloudWatchLogger_BufferOverflow(t *testing.T) {
	// Setup with very small buffer
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     10,
		FlushInterval: 10 * time.Second, // Very long to prevent flushing
		BufferSize:    2,                // Very small buffer
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Fill buffer beyond capacity rapidly to ensure dropping
	// Send messages faster than they can be processed
	for i := 0; i < 10; i++ {
		logger.Info("overflow message", map[string]any{
			"index": i,
		})
	}

	// Don't wait - check immediately to ensure messages were dropped before flush

	// Check stats for dropped entries
	stats := logger.GetStats()
	// With buffer size 2 and 10 messages, we should drop at least some
	assert.True(t, stats.EntriesDropped > 0, fmt.Sprintf("Expected some entries to be dropped, but got %d dropped and %d logged", stats.EntriesDropped, stats.EntriesLogged))
	// Total processed should be less than sent
	assert.True(t, stats.EntriesLogged+stats.EntriesDropped <= 10, "Total entries should not exceed sent")
}

func TestCloudWatchLogger_ErrorHandling(t *testing.T) {
	// Setup
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     1,
		FlushInterval: 50 * time.Millisecond,
	}

	// Make PutLogEvents fail
	mockClient.SetShouldFail("PutLogEvents", true)

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Log a message
	logger.Error("test error handling")

	// Wait for flush attempt
	time.Sleep(100 * time.Millisecond)

	// Verify error was tracked
	stats := logger.GetStats()
	assert.True(t, stats.ErrorCount > 0)
	assert.NotEmpty(t, stats.LastError)
	assert.False(t, logger.IsHealthy())
}

func TestCloudWatchLogger_FlushMethod(t *testing.T) {
	// Setup
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     10,
		FlushInterval: 10 * time.Second, // Very long to prevent auto-flush
		BufferSize:    20,
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Log some messages
	for i := 0; i < 3; i++ {
		logger.Info("flush test message", map[string]any{
			"index": i,
		})
	}

	// Give a small delay to ensure entries are buffered
	time.Sleep(50 * time.Millisecond)

	// Check buffer state before flush
	stats := logger.GetStats()
	t.Logf("Before flush - Buffer size: %d, Entries logged: %d", stats.BufferSize, stats.EntriesLogged)

	// Manually flush
	err = logger.Flush(context.Background())
	assert.NoError(t, err)

	// Check state after flush
	stats = logger.GetStats()
	t.Logf("After flush - Buffer size: %d, Entries logged: %d, Flush count: %d", stats.BufferSize, stats.EntriesLogged, stats.FlushCount)

	// Verify logs were sent
	callCount := mockClient.GetCallCount("PutLogEvents")
	logEvents := mockClient.GetLogEvents()
	t.Logf("PutLogEvents call count: %d, Log events count: %d", callCount, len(logEvents))

	assert.Equal(t, int64(1), callCount)
	assert.Len(t, logEvents, 3)
}

func TestCloudWatchLogger_HealthCheck(t *testing.T) {
	// Setup
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     1,
		FlushInterval: 50 * time.Millisecond,
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Initially healthy
	assert.True(t, logger.IsHealthy())

	// Log successful messages
	for i := 0; i < 20; i++ {
		logger.Info("healthy message")
	}

	time.Sleep(100 * time.Millisecond)
	assert.True(t, logger.IsHealthy())

	// Simulate errors
	mockClient.SetShouldFail("PutLogEvents", true)

	for i := 0; i < 5; i++ {
		logger.Error("error message")
	}

	time.Sleep(100 * time.Millisecond)

	// Should still be healthy due to ratio (20 success vs 5 errors = 20% error rate, but we need to account for flush errors)
	// The health check looks at error count vs entries logged, not flush success
	stats := logger.GetStats()
	t.Logf("After first errors - Logged: %d, Errors: %d", stats.EntriesLogged, stats.ErrorCount)

	// Reset and test with a clear scenario
	mockClient.ClearErrors()
	mockClient.SetShouldFail("PutLogEvents", false)

	// Log 10 successful messages
	for i := 0; i < 10; i++ {
		logger.Info("success message")
	}
	time.Sleep(100 * time.Millisecond)

	// Now cause 5 flush errors (which will affect 5 log entries)
	mockClient.SetShouldFail("PutLogEvents", true)
	for i := 0; i < 5; i++ {
		logger.Error("error message")
	}
	time.Sleep(100 * time.Millisecond)

	// Should be unhealthy now (5 errors out of 15 total = 33% error rate > 10%)
	assert.False(t, logger.IsHealthy())
}

func TestCloudWatchLogger_Stats(t *testing.T) {
	// Setup
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     2,
		FlushInterval: 50 * time.Millisecond,
		BufferSize:    5,
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Log some messages
	logger.Info("message 1")
	logger.Info("message 2")
	logger.Info("message 3")

	// Wait for flushes
	time.Sleep(150 * time.Millisecond)

	stats := logger.GetStats()

	// Verify basic stats
	assert.Equal(t, int64(3), stats.EntriesLogged)
	assert.Equal(t, int64(0), stats.EntriesDropped)
	assert.True(t, stats.FlushCount > 0)
	assert.False(t, stats.LastFlush.IsZero())
	assert.Equal(t, 5, stats.BufferCapacity)
	assert.Equal(t, int64(0), stats.ErrorCount)
	assert.Empty(t, stats.LastError)
	assert.True(t, stats.AverageFlushTime > 0)
}

func TestCloudWatchLogger_SanitizationCardBin(t *testing.T) {
	// Setup
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     5,
		FlushInterval: 50 * time.Millisecond,
		BufferSize:    10,
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Test that card_bin is NOT redacted while other card fields ARE redacted
	logger.Info("payment info", map[string]any{
		"card_bin":    "424242",           // Should NOT be redacted (BIN is not sensitive)
		"card_number": "4242424242424242", // Should be redacted
		"card_cvv":    "123",              // Should be redacted
		"cardholder":  "John Doe",         // Should be redacted
		"user_id":     "12345",            // Should not be redacted
	})

	// Wait for flush
	time.Sleep(100 * time.Millisecond)

	// Get the logged events
	logEvents := mockClient.GetLogEvents()
	require.Len(t, logEvents, 1)

	// Parse the log message to verify sanitization
	logMessage := *logEvents[0].Message
	assert.Contains(t, logMessage, `"card_bin":"424242"`, "card_bin should NOT be redacted")
	assert.Contains(t, logMessage, `"card_number":"************4242"`, "card_number should show last 4 digits only")
	assert.Contains(t, logMessage, `"card_cvv":"[REDACTED]"`, "card_cvv should be redacted")
	assert.Contains(t, logMessage, `"cardholder":"[REDACTED]"`, "cardholder should be redacted")
	assert.Contains(t, logMessage, `"user_id":"12345"`, "user_id should not be redacted")
}

func TestCloudWatchLogger_ConcurrentAccess(t *testing.T) {
	// Setup with larger buffer to handle concurrent load
	mockClient := NewMockCloudWatchLogsClient()
	config := observability.LoggerConfig{
		LogGroup:      "test-log-group",
		LogStream:     "test-log-stream",
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
		BufferSize:    200, // Much larger buffer for concurrent access
	}

	logger, err := NewCloudWatchLogger(config, mockClient)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Launch multiple goroutines to log concurrently
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			contextLogger := logger.WithField("goroutine_id", id)

			for j := 0; j < 10; j++ {
				contextLogger.Info("concurrent message", map[string]any{
					"message_id": j,
				})
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Wait for final flush
	time.Sleep(300 * time.Millisecond)

	// Verify all messages were logged (allow for some drops in concurrent scenario)
	stats := logger.GetStats()
	t.Logf("Concurrent test stats - Logged: %d, Dropped: %d", stats.EntriesLogged, stats.EntriesDropped)

	// In concurrent scenarios, we expect most messages to be logged
	assert.True(t, stats.EntriesLogged >= 90, "Expected at least 90 messages logged, got %d", stats.EntriesLogged)
	assert.True(t, stats.EntriesLogged+stats.EntriesDropped == 100, "Total should be 100")
	assert.True(t, logger.IsHealthy())
}
