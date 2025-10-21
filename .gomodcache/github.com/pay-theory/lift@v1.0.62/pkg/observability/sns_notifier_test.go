package observability

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSNSClient implements a mock SNS client for testing
type mockSNSClient struct {
	publishErr   error
	publishCalls []sns.PublishInput
}

func (m *mockSNSClient) Publish(_ context.Context, params *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	m.publishCalls = append(m.publishCalls, *params)
	if m.publishErr != nil {
		return nil, m.publishErr
	}
	return &sns.PublishOutput{
		MessageId: aws.String("test-message-id"),
	}, nil
}

func TestSNSNotifier_NotifyError(t *testing.T) {
	tests := []struct {
		logEntry      *LogEntry
		validateMsg   func(t *testing.T, msg SNSNotificationMessage)
		name          string
		topicARN      string
		expectedCalls int
	}{
		{
			name: "error log triggers notification",
			logEntry: &LogEntry{
				Timestamp: time.Now(),
				Level:     "ERROR",
				Message:   "Test error message",
				Fields: map[string]any{
					"request_id": "test-123",
					"error_type": "TEST_ERROR",
				},
			},
			topicARN:      "arn:aws:sns:us-east-1:123456789012:test-topic",
			expectedCalls: 1,
			validateMsg: func(t *testing.T, msg SNSNotificationMessage) {
				assert.Equal(t, "ERROR", msg.Severity)
				assert.Equal(t, "LiftError", msg.AlertConfig.AlertType)
				assert.Equal(t, "SLACK", msg.AlertConfig.AlertTargetType)
				assert.Contains(t, msg.Message, "Test error message")
			},
		},
		{
			name: "non-error log does not trigger notification",
			logEntry: &LogEntry{
				Timestamp: time.Now(),
				Level:     "INFO",
				Message:   "Test info message",
			},
			topicARN:      "arn:aws:sns:us-east-1:123456789012:test-topic",
			expectedCalls: 0,
		},
		{
			name: "warning log does not trigger notification",
			logEntry: &LogEntry{
				Timestamp: time.Now(),
				Level:     "WARN",
				Message:   "Test warning message",
			},
			topicARN:      "arn:aws:sns:us-east-1:123456789012:test-topic",
			expectedCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock SNS client
			mockClient := &mockSNSClient{}

			// Create notifier
			notifier := NewSNSNotifier(SNSConfig{
				Client:   mockClient,
				TopicARN: tt.topicARN,
			})

			// Send notification
			err := notifier.NotifyError(context.Background(), tt.logEntry)
			require.NoError(t, err)

			// Verify calls
			assert.Len(t, mockClient.publishCalls, tt.expectedCalls)

			// Validate message if a call was made
			if tt.expectedCalls > 0 && tt.validateMsg != nil {
				publishCall := mockClient.publishCalls[0]
				assert.Equal(t, tt.topicARN, *publishCall.TargetArn)

				var msg SNSNotificationMessage
				err := json.Unmarshal([]byte(*publishCall.Message), &msg)
				require.NoError(t, err)

				tt.validateMsg(t, msg)
			}
		})
	}
}

func TestSNSNotifier_FieldOverrides(t *testing.T) {
	// Create mock SNS client
	mockClient := &mockSNSClient{}

	// Create notifier
	notifier := NewSNSNotifier(SNSConfig{
		Client:   mockClient,
		TopicARN: "arn:aws:sns:us-east-1:123456789012:test-topic",
	})

	// Create log entry with field overrides
	logEntry := &LogEntry{
		Timestamp: time.Now(),
		Level:     "ERROR",
		Message:   "Test error",
		Fields: map[string]any{
			"environment":   "production",
			"service":       "my-service",
			"function_name": "custom-function",
			"aws_region":    "us-west-2",
			"account_id":    "999888777666",
		},
	}

	// Send notification
	err := notifier.NotifyError(context.Background(), logEntry)
	require.NoError(t, err)

	// Verify message
	assert.Len(t, mockClient.publishCalls, 1)
	var msg SNSNotificationMessage
	err = json.Unmarshal([]byte(*mockClient.publishCalls[0].Message), &msg)
	require.NoError(t, err)

	assert.Equal(t, "production", msg.Environment)
	assert.Equal(t, "my-service", msg.Service)
	assert.Equal(t, "custom-function", msg.Function)
	assert.Equal(t, "custom-function", msg.Subsystem)
	assert.Equal(t, "us-west-2", msg.AWSRegion)
	assert.Equal(t, "999888777666", msg.AWSAccount)
}
