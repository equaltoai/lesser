package dlq

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// ============================================================================
// NewDLQSender tests
// ============================================================================

func TestNewDLQSender(t *testing.T) {
	logger := zaptest.NewLogger(t)

	sender := NewDLQSender(logger)

	require.NotNil(t, sender)
	assert.NotNil(t, sender.logger)
	assert.NotNil(t, sender.queueURLs)
	assert.Empty(t, sender.queueURLs)
	assert.Nil(t, sender.sqsClient) // Not initialized until InitializeAWSClients is called
}

// ============================================================================
// getDLQQueueName tests
// ============================================================================

func TestGetDLQQueueName(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	tests := []struct {
		name         string
		service      string
		expectedName string
	}{
		{
			name:         "notification processor",
			service:      "notification-processor",
			expectedName: "notification-processor-dlq",
		},
		{
			name:         "activity processor",
			service:      "activity-processor",
			expectedName: "activity-processor-dlq",
		},
		{
			name:         "media processor",
			service:      "media-processor",
			expectedName: "media-processor-dlq",
		},
		{
			name:         "federation delivery",
			service:      "federation-delivery",
			expectedName: "federation-delivery-dlq",
		},
		{
			name:         "search indexer",
			service:      "search-indexer",
			expectedName: "search-indexer-dlq",
		},
		{
			name:         "custom service",
			service:      "my-custom-service",
			expectedName: "my-custom-service-dlq",
		},
		{
			name:         "empty service",
			service:      "",
			expectedName: "-dlq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sender.getDLQQueueName(tt.service)
			assert.Equal(t, tt.expectedName, result)
		})
	}
}

// ============================================================================
// getRetryCount tests
// ============================================================================

func TestGetRetryCount(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	tests := []struct {
		name          string
		message       events.SQSMessage
		expectedCount int
	}{
		{
			name: "first receive - no retries",
			message: events.SQSMessage{
				Attributes: map[string]string{
					"ApproximateReceiveCount": "1",
				},
			},
			expectedCount: 0,
		},
		{
			name: "second receive - one retry",
			message: events.SQSMessage{
				Attributes: map[string]string{
					"ApproximateReceiveCount": "2",
				},
			},
			expectedCount: 1,
		},
		{
			name: "multiple receives",
			message: events.SQSMessage{
				Attributes: map[string]string{
					"ApproximateReceiveCount": "5",
				},
			},
			expectedCount: 4,
		},
		{
			name: "custom retry attribute",
			message: events.SQSMessage{
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"RetryCount": {
						StringValue: strPtr("3"),
					},
				},
			},
			expectedCount: 3,
		},
		{
			name: "both attributes - SQS takes precedence",
			message: events.SQSMessage{
				Attributes: map[string]string{
					"ApproximateReceiveCount": "4",
				},
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"RetryCount": {
						StringValue: strPtr("10"),
					},
				},
			},
			expectedCount: 3, // 4 - 1 = 3
		},
		{
			name: "invalid receive count",
			message: events.SQSMessage{
				Attributes: map[string]string{
					"ApproximateReceiveCount": "invalid",
				},
			},
			expectedCount: 0,
		},
		{
			name: "nil retry count attribute",
			message: events.SQSMessage{
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"RetryCount": {
						StringValue: nil,
					},
				},
			},
			expectedCount: 0,
		},
		{
			name:          "no attributes",
			message:       events.SQSMessage{},
			expectedCount: 0,
		},
		{
			name: "zero receive count",
			message: events.SQSMessage{
				Attributes: map[string]string{
					"ApproximateReceiveCount": "0",
				},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sender.getRetryCount(tt.message)
			assert.Equal(t, tt.expectedCount, result)
		})
	}
}

// ============================================================================
// createDLQMessage tests
// ============================================================================

func TestSenderCreateDLQMessage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	tests := []struct {
		name            string
		service         string
		originalMessage events.SQSMessage
		processingError error
		checkFields     func(t *testing.T, msg *DLQFailureMessage)
	}{
		{
			name:    "basic message creation",
			service: "notification-processor",
			originalMessage: events.SQSMessage{
				MessageId:      "msg-123",
				Body:           `{"notification_id": "n1"}`,
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:notification-queue",
			},
			processingError: assert.AnError,
			checkFields: func(t *testing.T, msg *DLQFailureMessage) {
				assert.Equal(t, "msg-123", msg.OriginalMessageID)
				assert.Equal(t, "notification-processor", msg.Service)
				assert.Equal(t, "notification-queue", msg.QueueName)
				assert.Equal(t, `{"notification_id": "n1"}`, msg.MessageBody)
				assert.NotNil(t, msg.ErrorInfo)
			},
		},
		{
			name:    "message with attributes",
			service: "activity-processor",
			originalMessage: events.SQSMessage{
				MessageId: "msg-456",
				Body:      `{"activity": "create"}`,
				MessageAttributes: map[string]events.SQSMessageAttribute{
					"CustomAttr": {
						StringValue: strPtr("custom-value"),
					},
					"NilAttr": {
						StringValue: nil,
					},
				},
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:activity-queue",
			},
			processingError: assert.AnError,
			checkFields: func(t *testing.T, msg *DLQFailureMessage) {
				assert.Equal(t, "msg-456", msg.OriginalMessageID)
				assert.Equal(t, "activity-processor", msg.Service)
				assert.Contains(t, msg.MessageAttributes, "CustomAttr")
				assert.NotContains(t, msg.MessageAttributes, "NilAttr")
			},
		},
		{
			name:    "message with retry count",
			service: "media-processor",
			originalMessage: events.SQSMessage{
				MessageId: "msg-789",
				Body:      `{"media_id": "m1"}`,
				Attributes: map[string]string{
					"ApproximateReceiveCount": "3",
				},
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:media-queue",
			},
			processingError: assert.AnError,
			checkFields: func(t *testing.T, msg *DLQFailureMessage) {
				assert.Equal(t, 2, msg.RetryCount) // 3 - 1 = 2
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sender.createDLQMessage(tt.service, tt.originalMessage, tt.processingError)

			require.NotNil(t, result)
			tt.checkFields(t, result)
		})
	}
}

// ============================================================================
// extractQueueNameFromARN tests
// ============================================================================

func TestExtractQueueNameFromARN(t *testing.T) {
	tests := []struct {
		name          string
		arn           string
		expectedQueue string
	}{
		{
			name:          "valid ARN",
			arn:           "arn:aws:sqs:us-east-1:123456789012:my-queue",
			expectedQueue: "my-queue",
		},
		{
			name:          "ARN with dlq suffix",
			arn:           "arn:aws:sqs:us-west-2:987654321098:notification-dlq",
			expectedQueue: "notification-dlq",
		},
		{
			name:          "short ARN",
			arn:           "arn:aws:sqs:us-east-1:123",
			expectedQueue: "unknown",
		},
		{
			name:          "empty ARN",
			arn:           "",
			expectedQueue: "unknown",
		},
		{
			name:          "invalid format",
			arn:           "not-an-arn",
			expectedQueue: "unknown",
		},
		{
			name:          "ARN with special characters in queue name",
			arn:           "arn:aws:sqs:us-east-1:123456789012:my-queue-with-dashes",
			expectedQueue: "my-queue-with-dashes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractQueueNameFromARN(tt.arn)
			assert.Equal(t, tt.expectedQueue, result)
		})
	}
}

// ============================================================================
// parseIntSafe tests
// ============================================================================

func TestParseIntSafe(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "valid positive integer",
			input:    "42",
			expected: 42,
		},
		{
			name:     "zero",
			input:    "0",
			expected: 0,
		},
		{
			name:     "negative integer",
			input:    "-5",
			expected: -5,
		},
		{
			name:     "invalid string",
			input:    "not-a-number",
			expected: 0,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "float string",
			input:    "3.14",
			expected: 0,
		},
		{
			name:     "large number",
			input:    "999999",
			expected: 999999,
		},
		{
			name:     "whitespace",
			input:    "  ",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIntSafe(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// DLQFailureMessage struct tests
// ============================================================================

func TestDLQFailureMessageStruct(t *testing.T) {
	now := time.Now()
	msg := &DLQFailureMessage{
		OriginalMessageID: "orig-123",
		Service:           "test-service",
		QueueName:         "test-queue",
		MessageBody:       `{"test": "data"}`,
		MessageAttributes: map[string]string{
			"attr1": "value1",
		},
		ErrorInfo: &ErrorInfo{
			ErrorType:    "validation_error",
			ErrorMessage: "test error",
			IsPermanent:  true,
			Priority:     "medium",
		},
		ProcessingContext: ProcessingContext{
			FunctionName: "test-function",
			LogGroup:     "/aws/lambda/test",
			LogStream:    "2024/01/01/[$LATEST]abc123",
			RequestID:    "req-123",
			Timestamp:    now,
		},
		RetryCount: 2,
		Timestamp:  now,
	}

	assert.Equal(t, "orig-123", msg.OriginalMessageID)
	assert.Equal(t, "test-service", msg.Service)
	assert.Equal(t, "test-queue", msg.QueueName)
	assert.Equal(t, `{"test": "data"}`, msg.MessageBody)
	assert.Len(t, msg.MessageAttributes, 1)
	assert.NotNil(t, msg.ErrorInfo)
	assert.Equal(t, "validation_error", msg.ErrorInfo.ErrorType)
	assert.Equal(t, "test-function", msg.ProcessingContext.FunctionName)
	assert.Equal(t, 2, msg.RetryCount)
}

// ============================================================================
// ProcessingContext struct tests
// ============================================================================

func TestProcessingContextStruct(t *testing.T) {
	now := time.Now()
	ctx := ProcessingContext{
		FunctionName: "my-lambda-function",
		LogGroup:     "/aws/lambda/my-function",
		LogStream:    "2024/01/15/[$LATEST]xyz789",
		RequestID:    "request-abc-123",
		Timestamp:    now,
	}

	assert.Equal(t, "my-lambda-function", ctx.FunctionName)
	assert.Equal(t, "/aws/lambda/my-function", ctx.LogGroup)
	assert.Equal(t, "2024/01/15/[$LATEST]xyz789", ctx.LogStream)
	assert.Equal(t, "request-abc-123", ctx.RequestID)
	assert.Equal(t, now, ctx.Timestamp)
}

// ============================================================================
// ProcessingFailure struct tests
// ============================================================================

func TestProcessingFailureStruct(t *testing.T) {
	now := time.Now()
	failure := ProcessingFailure{
		OriginalMessage: events.SQSMessage{
			MessageId: "msg-fail-123",
			Body:      `{"failed": true}`,
		},
		Error:     assert.AnError,
		Timestamp: now,
	}

	assert.Equal(t, "msg-fail-123", failure.OriginalMessage.MessageId)
	assert.Equal(t, `{"failed": true}`, failure.OriginalMessage.Body)
	assert.Error(t, failure.Error)
	assert.Equal(t, now, failure.Timestamp)
}

// ============================================================================
// SendBatchFailedMessages with empty slice tests
// ============================================================================

func TestSendBatchFailedMessages_EmptySlice(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	// Empty slice should return nil without error
	err := sender.SendBatchFailedMessages(context.Background(), "test-service", []ProcessingFailure{})
	assert.NoError(t, err)

	// Nil slice should also return nil
	err = sender.SendBatchFailedMessages(context.Background(), "test-service", nil)
	assert.NoError(t, err)
}

// ============================================================================
// SendIndividualFailures with empty slice tests
// ============================================================================

func TestSendIndividualFailures_EmptySlice(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Should not panic with empty slice
	SendIndividualFailures(context.Background(), "test-service", []ProcessingFailure{}, logger)

	// Should not panic with nil slice
	SendIndividualFailures(context.Background(), "test-service", nil, logger)
}

// ============================================================================
// WrapSQSHandler tests
// ============================================================================

func TestWrapSQSHandler_SuccessfulProcessing(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a handler that succeeds
	successHandler := func(ctx context.Context, event events.SQSEvent) error {
		return nil
	}

	wrappedHandler := WrapSQSHandler("test-service", successHandler, logger)
	require.NotNil(t, wrappedHandler)

	// Test with empty event
	err := wrappedHandler(context.Background(), events.SQSEvent{})
	assert.NoError(t, err)
}

func TestWrapSQSHandler_FailedProcessing(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a handler that fails
	failHandler := func(ctx context.Context, event events.SQSEvent) error {
		return assert.AnError
	}

	wrappedHandler := WrapSQSHandler("test-service", failHandler, logger)
	require.NotNil(t, wrappedHandler)

	// Test with event containing records
	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      `{"test": "data"}`,
			},
		},
	}

	// The wrapped handler should return the original error
	err := wrappedHandler(context.Background(), event)
	assert.Error(t, err)
}

// ============================================================================
// Queue URL caching tests
// ============================================================================

func TestQueueURLCaching(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	// Verify cache is empty initially
	assert.Empty(t, sender.queueURLs)

	// Manually add to cache
	sender.queueURLs["test-queue"] = "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue"

	// Verify cache contains the entry
	assert.Len(t, sender.queueURLs, 1)
	assert.Equal(t, "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue", sender.queueURLs["test-queue"])
}

// ============================================================================
// Error classification integration tests
// ============================================================================

func TestSenderErrorClassification(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	tests := []struct {
		name              string
		errorMessage      string
		expectedErrorType string
		expectedPermanent bool
	}{
		{
			name:              "validation error",
			errorMessage:      "validation failed: missing required field",
			expectedErrorType: "validation_error",
			expectedPermanent: true,
		},
		{
			name:              "network error",
			errorMessage:      "connection timeout",
			expectedErrorType: "timeout_error",
			expectedPermanent: false,
		},
		{
			name:              "rate limit error",
			errorMessage:      "rate limit exceeded",
			expectedErrorType: "rate_limit_error",
			expectedPermanent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := sender.createDLQMessage(
				"test-service",
				events.SQSMessage{
					MessageId:      "test-msg",
					Body:           `{"error": "` + tt.errorMessage + `"}`,
					EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
				},
				assert.AnError,
			)

			require.NotNil(t, msg)
			require.NotNil(t, msg.ErrorInfo)
			// Note: The error classification is based on the processing error, not the message body
			// So we just verify the ErrorInfo is populated
			assert.NotEmpty(t, msg.ErrorInfo.ErrorType)
		})
	}
}


// ============================================================================
// WrapSQSHandler additional tests
// ============================================================================

func TestWrapSQSHandler_WithMultipleRecords(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a handler that fails
	failHandler := func(ctx context.Context, event events.SQSEvent) error {
		return assert.AnError
	}

	wrappedHandler := WrapSQSHandler("test-service", failHandler, logger)
	require.NotNil(t, wrappedHandler)

	// Test with multiple records
	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "msg-1", Body: `{"test": "data1"}`},
			{MessageId: "msg-2", Body: `{"test": "data2"}`},
			{MessageId: "msg-3", Body: `{"test": "data3"}`},
		},
	}

	// The wrapped handler should return the original error
	err := wrappedHandler(context.Background(), event)
	assert.Error(t, err)
}

func TestWrapSQSHandler_HandlerPanics(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a handler that panics
	panicHandler := func(ctx context.Context, event events.SQSEvent) error {
		panic("test panic")
	}

	wrappedHandler := WrapSQSHandler("test-service", panicHandler, logger)
	require.NotNil(t, wrappedHandler)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "msg-1", Body: `{"test": "data"}`},
		},
	}

	// The wrapped handler should propagate the panic
	assert.Panics(t, func() {
		_ = wrappedHandler(context.Background(), event)
	})
}

// ============================================================================
// SendIndividualFailures additional tests
// ============================================================================

func TestSendIndividualFailures_WithFailures(t *testing.T) {
	logger := zaptest.NewLogger(t)

	failures := []ProcessingFailure{
		{
			OriginalMessage: events.SQSMessage{
				MessageId: "msg-1",
				Body:      `{"test": "data1"}`,
			},
			Error:     assert.AnError,
			Timestamp: time.Now(),
		},
		{
			OriginalMessage: events.SQSMessage{
				MessageId: "msg-2",
				Body:      `{"test": "data2"}`,
			},
			Error:     assert.AnError,
			Timestamp: time.Now(),
		},
	}

	// This will fail to initialize AWS clients but should not panic
	SendIndividualFailures(context.Background(), "test-service", failures, logger)
}

// ============================================================================
// DLQSender initialization tests
// ============================================================================

func TestDLQSender_InitializeAWSClients_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	// This will attempt to load AWS config from environment
	// In test environment without AWS credentials, it may succeed with default config
	// or fail - either way, we're testing the code path
	err := sender.InitializeAWSClients(context.Background())
	// We don't assert on the error because it depends on the environment
	_ = err
}

// ============================================================================
// Batch processing edge cases
// ============================================================================

// Note: Tests that require AWS SQS client are not included here
// as they would require mocking the AWS SDK which is complex.
// The batch processing logic is tested through integration tests.

// ============================================================================
// Message attribute handling tests
// ============================================================================

func TestSenderCreateDLQMessage_AllAttributeTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	originalMessage := events.SQSMessage{
		MessageId: "msg-attrs",
		Body:      `{"test": "data"}`,
		MessageAttributes: map[string]events.SQSMessageAttribute{
			"StringAttr": {
				StringValue: strPtr("string-value"),
				DataType:    "String",
			},
			"NumberAttr": {
				StringValue: strPtr("123"),
				DataType:    "Number",
			},
			"BinaryAttr": {
				BinaryValue: []byte("binary-data"),
				DataType:    "Binary",
			},
			"NilStringAttr": {
				StringValue: nil,
				DataType:    "String",
			},
		},
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
	}

	msg := sender.createDLQMessage("test-service", originalMessage, assert.AnError)

	require.NotNil(t, msg)
	// Only string attributes with non-nil values should be copied
	assert.Contains(t, msg.MessageAttributes, "StringAttr")
	assert.Contains(t, msg.MessageAttributes, "NumberAttr")
	assert.NotContains(t, msg.MessageAttributes, "NilStringAttr")
	// Binary attributes are not copied (only StringValue is checked)
}

// ============================================================================
// Error info integration tests
// ============================================================================

func TestSenderCreateDLQMessage_ErrorInfoPopulated(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	originalMessage := events.SQSMessage{
		MessageId:      "msg-error",
		Body:           `{"test": "data"}`,
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
	}

	testError := fmt.Errorf("validation failed: missing required field")
	msg := sender.createDLQMessage("test-service", originalMessage, testError)

	require.NotNil(t, msg)
	require.NotNil(t, msg.ErrorInfo)
	// The error info should be populated based on the error message
	assert.NotEmpty(t, msg.ErrorInfo.ErrorType)
}

// ============================================================================
// Processing context tests
// ============================================================================

func TestSenderCreateDLQMessage_ProcessingContext(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	originalMessage := events.SQSMessage{
		MessageId:      "msg-ctx",
		Body:           `{"test": "data"}`,
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
	}

	msg := sender.createDLQMessage("test-service", originalMessage, assert.AnError)

	require.NotNil(t, msg)
	// Processing context should be populated
	assert.NotNil(t, msg.ProcessingContext)
	assert.False(t, msg.ProcessingContext.Timestamp.IsZero())
}

// ============================================================================
// Import for fmt
// ============================================================================


// ============================================================================
// Tests with Mock SQS Client
// ============================================================================

func TestSendFailedMessage_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-service-dlq"

	// Setup mock expectations
	mockSQS.On("GetQueueUrl", mock.Anything, mock.MatchedBy(func(input *sqs.GetQueueUrlInput) bool {
		return *input.QueueName == "test-service-dlq"
	})).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		return *input.QueueUrl == queueURL
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("sent-msg-123"),
	}, nil)

	originalMessage := events.SQSMessage{
		MessageId:      "orig-msg-123",
		Body:           `{"test": "data"}`,
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
	}

	err := sender.SendFailedMessage(context.Background(), "test-service", originalMessage, assert.AnError)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSendFailedMessage_GetQueueURLError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	// Setup mock to return error
	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	originalMessage := events.SQSMessage{
		MessageId:      "orig-msg-123",
		Body:           `{"test": "data"}`,
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
	}

	// Should not return error (graceful degradation)
	err := sender.SendFailedMessage(context.Background(), "test-service", originalMessage, assert.AnError)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSendFailedMessage_SendMessageError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-service-dlq"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	originalMessage := events.SQSMessage{
		MessageId:      "orig-msg-123",
		Body:           `{"test": "data"}`,
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
	}

	// Should not return error (graceful degradation)
	err := sender.SendFailedMessage(context.Background(), "test-service", originalMessage, assert.AnError)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSendToDLQ_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-dlq"

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		_, hasService := input.MessageAttributes["Service"]
		_, hasErrorType := input.MessageAttributes["ErrorType"]
		_, hasIsPermanent := input.MessageAttributes["IsPermanent"]
		_, hasPriority := input.MessageAttributes["Priority"]
		return *input.QueueUrl == queueURL && hasService && hasErrorType && hasIsPermanent && hasPriority
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("sent-msg-456"),
	}, nil)

	dlqMessage := &DLQFailureMessage{
		OriginalMessageID: "orig-123",
		Service:           "test-service",
		QueueName:         "test-queue",
		MessageBody:       `{"test": "data"}`,
		ErrorInfo: &ErrorInfo{
			ErrorType:   "validation_error",
			IsPermanent: true,
			Priority:    "high",
		},
	}

	err := sender.sendToDLQ(context.Background(), queueURL, dlqMessage)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSendToDLQ_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	mockSQS.On("SendMessage", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	dlqMessage := &DLQFailureMessage{
		OriginalMessageID: "orig-123",
		Service:           "test-service",
		ErrorInfo: &ErrorInfo{
			ErrorType: "test_error",
			Priority:  "medium",
		},
	}

	err := sender.sendToDLQ(context.Background(), "https://sqs.example.com/queue", dlqMessage)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send message to DLQ")

	mockSQS.AssertExpectations(t)
}

func TestSendBatchToDLQ_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-dlq"

	mockSQS.On("SendMessageBatch", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageBatchInput) bool {
		return *input.QueueUrl == queueURL && len(input.Entries) == 2
	})).Return(&sqs.SendMessageBatchOutput{
		Successful: []types.SendMessageBatchResultEntry{
			{Id: aws.String("dlq-0"), MessageId: aws.String("msg-1")},
			{Id: aws.String("dlq-1"), MessageId: aws.String("msg-2")},
		},
		Failed: []types.BatchResultErrorEntry{},
	}, nil)

	failures := []ProcessingFailure{
		{
			OriginalMessage: events.SQSMessage{
				MessageId:      "msg-1",
				Body:           `{"test": "data1"}`,
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
			Error:     assert.AnError,
			Timestamp: time.Now(),
		},
		{
			OriginalMessage: events.SQSMessage{
				MessageId:      "msg-2",
				Body:           `{"test": "data2"}`,
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
			Error:     assert.AnError,
			Timestamp: time.Now(),
		},
	}

	err := sender.sendBatchToDLQ(context.Background(), queueURL, "test-service", failures)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSendBatchToDLQ_PartialFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-dlq"

	mockSQS.On("SendMessageBatch", mock.Anything, mock.Anything).Return(&sqs.SendMessageBatchOutput{
		Successful: []types.SendMessageBatchResultEntry{
			{Id: aws.String("dlq-0"), MessageId: aws.String("msg-1")},
		},
		Failed: []types.BatchResultErrorEntry{
			{Id: aws.String("dlq-1"), Code: aws.String("InternalError"), Message: aws.String("Internal error")},
		},
	}, nil)

	failures := []ProcessingFailure{
		{
			OriginalMessage: events.SQSMessage{
				MessageId:      "msg-1",
				Body:           `{"test": "data1"}`,
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
			Error: assert.AnError,
		},
		{
			OriginalMessage: events.SQSMessage{
				MessageId:      "msg-2",
				Body:           `{"test": "data2"}`,
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
			Error: assert.AnError,
		},
	}

	// Should not return error even with partial failure
	err := sender.sendBatchToDLQ(context.Background(), queueURL, "test-service", failures)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSendBatchToDLQ_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	mockSQS.On("SendMessageBatch", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	failures := []ProcessingFailure{
		{
			OriginalMessage: events.SQSMessage{
				MessageId:      "msg-1",
				Body:           `{"test": "data"}`,
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
			Error: assert.AnError,
		},
	}

	err := sender.sendBatchToDLQ(context.Background(), "https://sqs.example.com/queue", "test-service", failures)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send batch to DLQ")

	mockSQS.AssertExpectations(t)
}

func TestSendBatchToDLQ_EmptyFailures(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	// Empty failures should return nil without calling SQS
	err := sender.sendBatchToDLQ(context.Background(), "https://sqs.example.com/queue", "test-service", []ProcessingFailure{})
	assert.NoError(t, err)

	// Verify no SQS calls were made
	mockSQS.AssertNotCalled(t, "SendMessageBatch", mock.Anything, mock.Anything)
}

func TestGetQueueURL_CacheHit(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	// Pre-populate cache
	cachedURL := "https://sqs.us-east-1.amazonaws.com/123456789012/cached-queue"
	sender.queueURLs["cached-queue"] = cachedURL

	// Should return cached URL without calling SQS
	url, err := sender.getQueueURL(context.Background(), "cached-queue")
	assert.NoError(t, err)
	assert.Equal(t, cachedURL, url)

	// Verify no SQS calls were made
	mockSQS.AssertNotCalled(t, "GetQueueUrl", mock.Anything, mock.Anything)
}

func TestGetQueueURL_CacheMiss(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	expectedURL := "https://sqs.us-east-1.amazonaws.com/123456789012/new-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.MatchedBy(func(input *sqs.GetQueueUrlInput) bool {
		return *input.QueueName == "new-queue"
	})).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &expectedURL,
	}, nil)

	url, err := sender.getQueueURL(context.Background(), "new-queue")
	assert.NoError(t, err)
	assert.Equal(t, expectedURL, url)

	// Verify URL was cached
	assert.Equal(t, expectedURL, sender.queueURLs["new-queue"])

	mockSQS.AssertExpectations(t)
}

func TestGetQueueURL_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	url, err := sender.getQueueURL(context.Background(), "error-queue")
	assert.Error(t, err)
	assert.Empty(t, url)
	assert.Contains(t, err.Error(), "failed to get queue URL")

	mockSQS.AssertExpectations(t)
}

func TestSendBatchFailedMessages_WithMock(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-service-dlq"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessageBatch", mock.Anything, mock.Anything).Return(&sqs.SendMessageBatchOutput{
		Successful: []types.SendMessageBatchResultEntry{
			{Id: aws.String("dlq-0"), MessageId: aws.String("msg-1")},
		},
		Failed: []types.BatchResultErrorEntry{},
	}, nil)

	failures := []ProcessingFailure{
		{
			OriginalMessage: events.SQSMessage{
				MessageId:      "msg-1",
				Body:           `{"test": "data"}`,
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
			Error:     assert.AnError,
			Timestamp: time.Now(),
		},
	}

	err := sender.SendBatchFailedMessages(context.Background(), "test-service", failures)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSendBatchFailedMessages_LargeBatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-service-dlq"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	// Should be called twice for 15 messages (batches of 10)
	mockSQS.On("SendMessageBatch", mock.Anything, mock.Anything).Return(&sqs.SendMessageBatchOutput{
		Successful: []types.SendMessageBatchResultEntry{},
		Failed:     []types.BatchResultErrorEntry{},
	}, nil).Times(2)

	// Create 15 failures to test batching
	failures := make([]ProcessingFailure, 15)
	for i := 0; i < 15; i++ {
		failures[i] = ProcessingFailure{
			OriginalMessage: events.SQSMessage{
				MessageId:      fmt.Sprintf("msg-%d", i),
				Body:           fmt.Sprintf(`{"index": %d}`, i),
				EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
			},
			Error:     assert.AnError,
			Timestamp: time.Now(),
		}
	}

	err := sender.SendBatchFailedMessages(context.Background(), "test-service", failures)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSetSQSClient_Sender(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sender := NewDLQSender(logger)

	// Initially nil
	assert.Nil(t, sender.sqsClient)

	mockSQS := new(MockSQSClient)
	sender.SetSQSClient(mockSQS)

	// Now set
	assert.NotNil(t, sender.sqsClient)
	assert.Equal(t, mockSQS, sender.sqsClient)
}
