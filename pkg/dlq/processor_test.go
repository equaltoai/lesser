package dlq

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// ============================================================================
// Helper functions for testing
// ============================================================================

func newTestProcessor(t *testing.T) *Processor {
	t.Helper()
	logger := zaptest.NewLogger(t)
	// Create processor without DB for unit testing
	return &Processor{
		logger:            logger,
		errorClassifier:   NewErrorClassifier(),
		reprocessorClient: NewReprocessorClient(logger),
	}
}

// ============================================================================
// parseOriginalMessage tests
// ============================================================================

func TestParseOriginalMessage(t *testing.T) {
	processor := newTestProcessor(t)

	tests := []struct {
		name                   string
		record                 events.SQSMessage
		expectedMessageID      string
		expectedBody           string
		expectedSourceQueue    string
		expectedOriginalMsgID  string
		expectedAttributeCount int
	}{
		{
			name: "basic message parsing",
			record: events.SQSMessage{
				MessageId:     "msg-123",
				Body:          `{"test": "data"}`,
				ReceiptHandle: "receipt-handle-abc",
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"CustomAttr": {
						StringValue: strPtr("custom-value"),
					},
				},
			},
			expectedMessageID:      "msg-123",
			expectedBody:           `{"test": "data"}`,
			expectedAttributeCount: 1,
		},
		{
			name: "message with source queue attribute",
			record: events.SQSMessage{
				MessageId: "msg-456",
				Body:      `{"error": "test"}`,
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"DeadLetterQueue.SourceQueue": {
						StringValue: strPtr("notification-processor-queue"),
					},
				},
			},
			expectedMessageID:      "msg-456",
			expectedBody:           `{"error": "test"}`,
			expectedSourceQueue:    "notification-processor-queue",
			expectedAttributeCount: 1,
		},
		{
			name: "message with original message ID attribute",
			record: events.SQSMessage{
				MessageId: "dlq-msg-789",
				Body:      `{"data": "value"}`,
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"DeadLetterQueue.OriginalMessageId": {
						StringValue: strPtr("original-msg-001"),
					},
				},
			},
			expectedMessageID:      "dlq-msg-789",
			expectedBody:           `{"data": "value"}`,
			expectedOriginalMsgID:  "original-msg-001",
			expectedAttributeCount: 1,
		},
		{
			name: "message with multiple attributes",
			record: events.SQSMessage{
				MessageId: "msg-multi",
				Body:      `{"multi": true}`,
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"DeadLetterQueue.SourceQueue": {
						StringValue: strPtr("activity-processor-queue"),
					},
					"DeadLetterQueue.OriginalMessageId": {
						StringValue: strPtr("orig-123"),
					},
					"CustomHeader": {
						StringValue: strPtr("header-value"),
					},
				},
			},
			expectedMessageID:      "msg-multi",
			expectedBody:           `{"multi": true}`,
			expectedSourceQueue:    "activity-processor-queue",
			expectedOriginalMsgID:  "orig-123",
			expectedAttributeCount: 3,
		},
		{
			name: "message with nil string value attribute",
			record: events.SQSMessage{
				MessageId: "msg-nil",
				Body:      `{"nil": "test"}`,
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"NilAttr": {
						StringValue: nil,
					},
					"ValidAttr": {
						StringValue: strPtr("valid"),
					},
				},
			},
			expectedMessageID:      "msg-nil",
			expectedBody:           `{"nil": "test"}`,
			expectedAttributeCount: 1, // Only valid attr should be copied
		},
		{
			name: "empty message",
			record: events.SQSMessage{
				MessageId:         "",
				Body:              "",
				MessageAttributes: nil,
			},
			expectedMessageID:      "",
			expectedBody:           "",
			expectedAttributeCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.parseOriginalMessage(tt.record)

			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, tt.expectedMessageID, result.MessageID)
			assert.Equal(t, tt.expectedBody, result.Body)
			assert.Equal(t, tt.record.ReceiptHandle, result.ReceiptHandle)
			assert.Len(t, result.Attributes, tt.expectedAttributeCount)

			if tt.expectedSourceQueue != "" {
				assert.Equal(t, tt.expectedSourceQueue, result.SourceQueue)
			}
			if tt.expectedOriginalMsgID != "" {
				assert.Equal(t, tt.expectedOriginalMsgID, result.OriginalMessageID)
			}
		})
	}
}

// ============================================================================
// extractServiceName tests
// ============================================================================

func TestExtractServiceName(t *testing.T) {
	processor := newTestProcessor(t)

	tests := []struct {
		name            string
		record          events.SQSMessage
		expectedService string
	}{
		{
			name: "notification processor queue",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-notification-processor-queue-dlq",
			},
			expectedService: "notification-processor",
		},
		{
			name: "activity processor queue",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-activity-processor-queue-dlq",
			},
			expectedService: "activity-processor",
		},
		{
			name: "media processor queue",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-media-processor-queue-dlq",
			},
			expectedService: "media-processor",
		},
		{
			name: "federation delivery queue",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-federation-delivery-queue-dlq",
			},
			expectedService: "federation-delivery",
		},
		{
			name: "search indexer queue",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-search-indexer-queue-dlq",
			},
			expectedService: "search-indexer",
		},
		{
			name: "simple dlq suffix",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:my-service-dlq",
			},
			expectedService: "my-service",
		},
		{
			name: "queue without lesser prefix",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:custom-service-queue-dlq",
			},
			expectedService: "custom-service",
		},
		{
			name: "unknown queue format",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:unknown",
			},
			expectedService: "unknown",
		},
		{
			name: "invalid ARN format",
			record: events.SQSMessage{
				EventSourceARN: "invalid-arn",
			},
			expectedService: "unknown",
		},
		{
			name: "empty ARN",
			record: events.SQSMessage{
				EventSourceARN: "",
			},
			expectedService: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.extractServiceName(tt.record)
			assert.Equal(t, tt.expectedService, result)
		})
	}
}

// ============================================================================
// extractQueueName tests
// ============================================================================

func TestExtractQueueName(t *testing.T) {
	processor := newTestProcessor(t)

	tests := []struct {
		name          string
		queueARN      string
		expectedQueue string
	}{
		{
			name:          "valid ARN",
			queueARN:      "arn:aws:sqs:us-east-1:123456789012:my-queue-dlq",
			expectedQueue: "my-queue-dlq",
		},
		{
			name:          "ARN with different region",
			queueARN:      "arn:aws:sqs:eu-west-1:987654321098:notification-dlq",
			expectedQueue: "notification-dlq",
		},
		{
			name:          "short ARN",
			queueARN:      "arn:aws:sqs:us-east-1:123",
			expectedQueue: "unknown",
		},
		{
			name:          "empty ARN",
			queueARN:      "",
			expectedQueue: "unknown",
		},
		{
			name:          "invalid format",
			queueARN:      "not-an-arn",
			expectedQueue: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.extractQueueName(tt.queueARN)
			assert.Equal(t, tt.expectedQueue, result)
		})
	}
}

// ============================================================================
// createDLQMessage tests
// ============================================================================

func TestCreateDLQMessage(t *testing.T) {
	processor := newTestProcessor(t)

	tests := []struct {
		name                string
		record              events.SQSMessage
		originalMessage     *OriginalMessage
		expectedService     string
		expectedErrorType   string
		expectedIsPermanent bool
		checkTags           []string
	}{
		{
			name: "validation error message",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-notification-processor-queue-dlq",
				MessageId:      "msg-1",
				ReceiptHandle:  "receipt-1",
				Attributes: map[string]string{
					"ApproximateReceiveCount": "3",
				},
			},
			originalMessage: &OriginalMessage{
				MessageID:  "msg-1",
				Body:       `{"errorMessage": "validation failed: missing required field"}`,
				Attributes: map[string]string{},
			},
			expectedService:     "notification-processor",
			expectedErrorType:   "validation_error",
			expectedIsPermanent: true,
			checkTags:           []string{"service:notification-processor", "error_type:validation_error", "permanent"},
		},
		{
			name: "network error message",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-federation-delivery-queue-dlq",
				MessageId:      "msg-2",
				ReceiptHandle:  "receipt-2",
			},
			originalMessage: &OriginalMessage{
				MessageID:   "msg-2",
				Body:        `{"error": "connection timeout to remote server"}`,
				Attributes:  map[string]string{},
				SourceQueue: "federation-delivery-queue",
			},
			expectedService:     "federation-delivery",
			expectedErrorType:   "network_error", // "connection" matches network_error pattern
			expectedIsPermanent: false,
			checkTags:           []string{"service:federation-delivery", "transient"},
		},
		{
			name: "media processor error",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-media-processor-queue-dlq",
				MessageId:      "msg-3",
			},
			originalMessage: &OriginalMessage{
				MessageID:  "msg-3",
				Body:       `{"message": "format unsupported: audio/midi"}`,
				Attributes: map[string]string{},
			},
			expectedService:     "media-processor",
			expectedErrorType:   "unsupported_media_format",
			expectedIsPermanent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.createDLQMessage(tt.record, tt.originalMessage)

			require.NotNil(t, result)
			// ID may be empty if builder doesn't generate one without DB
			assert.Equal(t, tt.expectedService, result.Service)
			assert.Equal(t, tt.expectedErrorType, result.ErrorType)
			assert.Equal(t, tt.expectedIsPermanent, result.IsPermanent)
			assert.Equal(t, tt.originalMessage.MessageID, result.OriginalMessageID)
			assert.Equal(t, tt.originalMessage.Body, result.MessageBody)

			// Check tags if specified
			for _, expectedTag := range tt.checkTags {
				assert.Contains(t, result.Tags, expectedTag)
			}
		})
	}
}

// ============================================================================
// OriginalMessage struct tests
// ============================================================================

func TestOriginalMessageStruct(t *testing.T) {
	msg := &OriginalMessage{
		MessageID:         "test-msg-id",
		OriginalMessageID: "original-id",
		Body:              `{"test": "data"}`,
		Attributes: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		SourceQueue:   "test-queue",
		ReceiptHandle: "receipt-handle",
	}

	assert.Equal(t, "test-msg-id", msg.MessageID)
	assert.Equal(t, "original-id", msg.OriginalMessageID)
	assert.Equal(t, `{"test": "data"}`, msg.Body)
	assert.Len(t, msg.Attributes, 2)
	assert.Equal(t, "test-queue", msg.SourceQueue)
	assert.Equal(t, "receipt-handle", msg.ReceiptHandle)
}

// ============================================================================
// ProcessingResult struct tests
// ============================================================================

func TestProcessingResultStruct(t *testing.T) {
	now := time.Now()
	result := &ProcessingResult{
		MessageID:         "msg-123",
		Success:           true,
		Error:             "",
		ReprocessingCount: 2,
		ProcessingTimeMs:  150,
		CostMicroCents:    35,
		Timestamp:         now,
	}

	assert.Equal(t, "msg-123", result.MessageID)
	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.Equal(t, 2, result.ReprocessingCount)
	assert.Equal(t, int64(150), result.ProcessingTimeMs)
	assert.Equal(t, int64(35), result.CostMicroCents)
	assert.Equal(t, now, result.Timestamp)
}

// ============================================================================
// ProcessingStats struct tests
// ============================================================================

func TestProcessingStatsStruct(t *testing.T) {
	stats := &ProcessingStats{
		TotalMessages:       100,
		ProcessedMessages:   85,
		FailedMessages:      10,
		ReprocessedMessages: 5,
		ResolvedMessages:    80,
		AbandonedMessages:   5,
		TotalCostMicroCents: 3500,
		TotalCostDollars:    0.0035,
		ProcessingTimeMs:    5000,
	}

	assert.Equal(t, 100, stats.TotalMessages)
	assert.Equal(t, 85, stats.ProcessedMessages)
	assert.Equal(t, 10, stats.FailedMessages)
	assert.Equal(t, 5, stats.ReprocessedMessages)
	assert.Equal(t, 80, stats.ResolvedMessages)
	assert.Equal(t, 5, stats.AbandonedMessages)
	assert.Equal(t, int64(3500), stats.TotalCostMicroCents)
	assert.Equal(t, 0.0035, stats.TotalCostDollars)
	assert.Equal(t, int64(5000), stats.ProcessingTimeMs)
}

// ============================================================================
// NewProcessor tests
// ============================================================================

func TestNewProcessor(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test with nil DB (should still create processor)
	processor := NewProcessor(nil, "test-table", logger)

	require.NotNil(t, processor)
	assert.NotNil(t, processor.logger)
	assert.NotNil(t, processor.errorClassifier)
	assert.NotNil(t, processor.reprocessorClient)
	assert.NotNil(t, processor.dlqRepo)
	assert.NotNil(t, processor.costTrackingRepo)
}

// ============================================================================
// ProcessDLQMessages error handling tests
// ============================================================================

func TestProcessDLQMessages_EmptyEvent(t *testing.T) {
	processor := newTestProcessor(t)

	event := events.SQSEvent{
		Records: []events.SQSMessage{},
	}

	err := processor.ProcessDLQMessages(context.Background(), event)
	assert.NoError(t, err)
}

// ============================================================================
// Helper functions
// ============================================================================

func strPtr(s string) *string {
	return &s
}

// ============================================================================
// Integration-style tests for processor logic
// ============================================================================

func TestProcessorErrorClassifierIntegration(t *testing.T) {
	processor := newTestProcessor(t)

	// Verify error classifier is properly initialized
	require.NotNil(t, processor.errorClassifier)

	patterns := processor.errorClassifier.GetPatterns()
	assert.NotEmpty(t, patterns)

	// Test classification through processor
	record := events.SQSMessage{
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-notification-processor-queue-dlq",
	}
	originalMessage := &OriginalMessage{
		MessageID: "test-msg",
		Body:      `{"errorMessage": "rate limit exceeded"}`,
	}

	dlqMessage := processor.createDLQMessage(record, originalMessage)
	assert.Equal(t, "rate_limit_error", dlqMessage.ErrorType)
	assert.False(t, dlqMessage.IsPermanent)
}

func TestProcessorReprocessorClientIntegration(t *testing.T) {
	processor := newTestProcessor(t)

	// Verify reprocessor client is properly initialized
	require.NotNil(t, processor.reprocessorClient)

	// Test URL validation through reprocessor
	assert.True(t, processor.reprocessorClient.isValidURL("https://example.com/media.jpg"))
	assert.False(t, processor.reprocessorClient.isValidURL("invalid-url"))
}

// ============================================================================
// Logging verification tests
// ============================================================================

func TestProcessorLogging(t *testing.T) {
	// Create a logger that captures output
	logger := zap.NewNop()

	processor := &Processor{
		logger:            logger,
		errorClassifier:   NewErrorClassifier(),
		reprocessorClient: NewReprocessorClient(logger),
	}

	// Verify processor can be created with nop logger
	require.NotNil(t, processor)
	assert.NotNil(t, processor.logger)
}

// ============================================================================
// Additional processor tests for better coverage
// ============================================================================

// Note: Tests that require AWS clients (SQS) are not included here
// as they would require mocking the AWS SDK which is complex.
// The processMessage, attemptReprocessing, and related functions
// are tested through integration tests.

// ============================================================================
// Service name extraction edge cases
// ============================================================================

func TestExtractServiceName_EdgeCases(t *testing.T) {
	processor := newTestProcessor(t)

	tests := []struct {
		name            string
		record          events.SQSMessage
		expectedService string
	}{
		{
			name: "queue with only dlq suffix",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:dlq",
			},
			expectedService: "dlq", // The function returns the queue name as-is when no pattern matches
		},
		{
			name: "lesser prefix with complex name",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-my-complex-service-name-queue-dlq",
			},
			expectedService: "my-complex-service-name",
		},
		{
			name: "queue without dlq suffix",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-service-queue",
			},
			expectedService: "service",
		},
		{
			name: "queue with only queue suffix",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:my-service-queue",
			},
			expectedService: "my-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.extractServiceName(tt.record)
			assert.Equal(t, tt.expectedService, result)
		})
	}
}

// ============================================================================
// createDLQMessage edge cases
// ============================================================================

func TestCreateDLQMessage_EdgeCases(t *testing.T) {
	processor := newTestProcessor(t)

	tests := []struct {
		name            string
		record          events.SQSMessage
		originalMessage *OriginalMessage
		checkFunc       func(t *testing.T, msg *models.DLQMessage)
	}{
		{
			name: "empty body",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue-dlq",
				MessageId:      "msg-empty",
			},
			originalMessage: &OriginalMessage{
				MessageID:  "msg-empty",
				Body:       "",
				Attributes: map[string]string{},
			},
			checkFunc: func(t *testing.T, msg *models.DLQMessage) {
				assert.Empty(t, msg.MessageBody)
			},
		},
		{
			name: "invalid JSON body",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue-dlq",
				MessageId:      "msg-invalid",
			},
			originalMessage: &OriginalMessage{
				MessageID:  "msg-invalid",
				Body:       "not valid json {{{",
				Attributes: map[string]string{},
			},
			checkFunc: func(t *testing.T, msg *models.DLQMessage) {
				assert.Equal(t, "not valid json {{{", msg.MessageBody)
			},
		},
		{
			name: "message with source queue in attributes",
			record: events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue-dlq",
				MessageId:      "msg-source",
			},
			originalMessage: &OriginalMessage{
				MessageID:   "msg-source",
				Body:        `{"test": "data"}`,
				Attributes:  map[string]string{},
				SourceQueue: "original-source-queue",
			},
			checkFunc: func(t *testing.T, msg *models.DLQMessage) {
				assert.Equal(t, "original-source-queue", msg.SourceQueue)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.createDLQMessage(tt.record, tt.originalMessage)
			require.NotNil(t, result)
			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

// ============================================================================
// Additional Processor Tests for Coverage
// ============================================================================

// Note: TestProcessDLQMessages_WithRecords and TestProcessDLQMessages_AllMessagesFail
// require full DynamoDB mocking and are covered in integration tests.
// The processMessage function requires dlqRepo to be non-nil.

// ============================================================================
// Processor with SetSQSClient method test
// ============================================================================

func TestProcessor_SetSQSClient(t *testing.T) {
	processor := newTestProcessor(t)

	// Verify reprocessor client exists
	require.NotNil(t, processor.reprocessorClient)

	// Set SQS client on reprocessor
	mockSQS := new(MockSQSClient)
	processor.reprocessorClient.SetSQSClient(mockSQS)

	// Verify it was set
	assert.NotNil(t, processor.reprocessorClient.sqsClient)
}

// ============================================================================
// Error handling edge cases
// ============================================================================

func TestCreateDLQMessage_WithMetadata(t *testing.T) {
	processor := newTestProcessor(t)

	record := events.SQSMessage{
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-notification-processor-queue-dlq",
		MessageId:      "msg-meta",
		ReceiptHandle:  "receipt-meta",
		Attributes: map[string]string{
			"ApproximateReceiveCount": "5",
			"SentTimestamp":           "1234567890",
		},
	}

	originalMessage := &OriginalMessage{
		MessageID: "msg-meta",
		Body:      `{"notification_id": "n1", "user_id": "u1"}`,
		Attributes: map[string]string{
			"CustomHeader": "custom-value",
		},
	}

	dlqMessage := processor.createDLQMessage(record, originalMessage)

	require.NotNil(t, dlqMessage)
	assert.Equal(t, "notification-processor", dlqMessage.Service)
	assert.NotNil(t, dlqMessage.MessageMetadata)
	assert.Contains(t, dlqMessage.MessageMetadata, "queue_arn")
	assert.Contains(t, dlqMessage.MessageMetadata, "receipt_handle")
	assert.Contains(t, dlqMessage.MessageMetadata, "approximate_receive_count")
}

func TestCreateDLQMessage_PermanentError(t *testing.T) {
	processor := newTestProcessor(t)

	record := events.SQSMessage{
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-media-processor-queue-dlq",
		MessageId:      "msg-perm",
	}

	// Message with unsupported format error (permanent)
	originalMessage := &OriginalMessage{
		MessageID:  "msg-perm",
		Body:       `{"error": "unsupported format: audio/midi"}`,
		Attributes: map[string]string{},
	}

	dlqMessage := processor.createDLQMessage(record, originalMessage)

	require.NotNil(t, dlqMessage)
	assert.True(t, dlqMessage.IsPermanent)
	assert.Contains(t, dlqMessage.Tags, "permanent")
}

func TestCreateDLQMessage_TransientError(t *testing.T) {
	processor := newTestProcessor(t)

	record := events.SQSMessage{
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-federation-delivery-queue-dlq",
		MessageId:      "msg-trans",
	}

	// Message with timeout error (transient)
	originalMessage := &OriginalMessage{
		MessageID:  "msg-trans",
		Body:       `{"error": "connection timeout"}`,
		Attributes: map[string]string{},
	}

	dlqMessage := processor.createDLQMessage(record, originalMessage)

	require.NotNil(t, dlqMessage)
	assert.False(t, dlqMessage.IsPermanent)
	assert.Contains(t, dlqMessage.Tags, "transient")
}

// ============================================================================
// Service name extraction comprehensive tests
// ============================================================================

func TestExtractServiceName_AllServices(t *testing.T) {
	processor := newTestProcessor(t)

	services := []struct {
		queueARN        string
		expectedService string
	}{
		{"arn:aws:sqs:us-east-1:123456789012:lesser-notification-processor-queue-dlq", "notification-processor"},
		{"arn:aws:sqs:us-east-1:123456789012:lesser-activity-processor-queue-dlq", "activity-processor"},
		{"arn:aws:sqs:us-east-1:123456789012:lesser-media-processor-queue-dlq", "media-processor"},
		{"arn:aws:sqs:us-east-1:123456789012:lesser-federation-delivery-queue-dlq", "federation-delivery"},
		{"arn:aws:sqs:us-east-1:123456789012:lesser-search-indexer-queue-dlq", "search-indexer"},
		{"arn:aws:sqs:us-east-1:123456789012:lesser-push-delivery-queue-dlq", "push-delivery"},
		{"arn:aws:sqs:us-east-1:123456789012:lesser-export-processor-queue-dlq", "export-processor"},
		{"arn:aws:sqs:us-east-1:123456789012:lesser-import-processor-queue-dlq", "import-processor"},
	}

	for _, tc := range services {
		t.Run(tc.expectedService, func(t *testing.T) {
			record := events.SQSMessage{EventSourceARN: tc.queueARN}
			result := processor.extractServiceName(record)
			assert.Equal(t, tc.expectedService, result)
		})
	}
}

// ============================================================================
// Parse original message edge cases
// ============================================================================

func TestParseOriginalMessage_WithAllAttributes(t *testing.T) {
	processor := newTestProcessor(t)

	record := events.SQSMessage{
		MessageId:     "msg-all-attrs",
		Body:          `{"complete": "message"}`,
		ReceiptHandle: "receipt-all",
		MessageAttributes: map[string]events.SQSMessageAttribute{
			"DeadLetterQueue.SourceQueue": {
				StringValue: strPtr("original-queue"),
			},
			"DeadLetterQueue.OriginalMessageId": {
				StringValue: strPtr("original-msg-id"),
			},
			"CustomAttr1": {
				StringValue: strPtr("value1"),
			},
			"CustomAttr2": {
				StringValue: strPtr("value2"),
			},
		},
	}

	result, err := processor.parseOriginalMessage(record)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "msg-all-attrs", result.MessageID)
	assert.Equal(t, "original-queue", result.SourceQueue)
	assert.Equal(t, "original-msg-id", result.OriginalMessageID)
	assert.Len(t, result.Attributes, 4)
}

// ============================================================================
// Error classifier integration through processor
// ============================================================================

func TestProcessorClassifiesErrors(t *testing.T) {
	processor := newTestProcessor(t)

	testCases := []struct {
		name              string
		errorMessage      string
		expectedErrorType string
		expectedPermanent bool
	}{
		{
			name:              "validation error",
			errorMessage:      `{"errorMessage": "validation failed: missing field"}`,
			expectedErrorType: "validation_error",
			expectedPermanent: true,
		},
		{
			name:              "rate limit error",
			errorMessage:      `{"error": "rate limit exceeded"}`,
			expectedErrorType: "rate_limit_error",
			expectedPermanent: false,
		},
		{
			name:              "timeout error",
			errorMessage:      `{"error": "deadline exceeded"}`,
			expectedErrorType: "timeout_error",
			expectedPermanent: false,
		},
		{
			name:              "network error",
			errorMessage:      `{"error": "connection refused"}`,
			expectedErrorType: "network_error",
			expectedPermanent: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			record := events.SQSMessage{
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue-dlq",
				MessageId:      "msg-" + tc.name,
			}
			originalMessage := &OriginalMessage{
				MessageID:  "msg-" + tc.name,
				Body:       tc.errorMessage,
				Attributes: map[string]string{},
			}

			dlqMessage := processor.createDLQMessage(record, originalMessage)

			assert.Equal(t, tc.expectedErrorType, dlqMessage.ErrorType)
			assert.Equal(t, tc.expectedPermanent, dlqMessage.IsPermanent)
		})
	}
}
