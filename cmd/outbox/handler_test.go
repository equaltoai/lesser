package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/internal/testutil/mocks"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOutboxProcessor(t *testing.T) {
	t.Run("successful SQS message processing", func(t *testing.T) {
		// Create test processor
		processor, err := NewOutboxProcessor()
		require.NoError(t, err)

		// Setup mock storage
		mockStore := new(mocks.MockStorage)
		processor.store = mockStore

		// Create test activity
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/123",
				Type: activitypub.FollowType,
			},
			Actor:  "https://example.com/users/alice",
			Object: "https://remote.example/users/bob",
		}

		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "alice",
		}

		// Create test message
		deliveryMsg := ActivityDeliveryMessage{
			Activity:    activity,
			Actor:       actor,
			TargetInbox: "https://remote.example/inbox",
			Attempt:     1,
		}

		msgJSON, _ := json.Marshal(deliveryMsg)

		// Create SQS event
		sqsEvent := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId: "test-message-id",
					Body:      string(msgJSON),
				},
			},
		}

		// Setup mock expectations - outbox processor mainly tracks federation activities
		mockStore.On("RecordFederationActivity", mock.Anything, mock.AnythingOfType("*github.com/equaltoai/lesser/pkg/storage.FederationActivity")).Return(nil)
		mockStore.On("RecordActivity", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)

		// Create Lift context with SQS event
		ctx := &lift.Context{}
		ctx.Set("requestID", "test-request-id")

		// Execute handler
		err = processor.HandleSQS(ctx, sqsEvent)

		// Assert - should not error on successful processing
		assert.NoError(t, err)

		// Verify mock expectations
		mockStore.AssertExpectations(t)
	})

	t.Run("invalid message format", func(t *testing.T) {
		// Create test processor
		processor, err := NewOutboxProcessor()
		require.NoError(t, err)

		// Create SQS event with invalid JSON
		sqsEvent := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId: "test-message-id",
					Body:      "invalid json",
				},
			},
		}

		// Create Lift context
		ctx := &lift.Context{}
		ctx.Set("requestID", "test-request-id")

		// Execute handler
		err = processor.HandleSQS(ctx, sqsEvent)

		// Assert - should return error for batch with failures
		assert.Error(t, err)
		liftErr, ok := err.(*lift.LiftError)
		assert.True(t, ok)
		assert.Equal(t, "PARTIAL_FAILURE", liftErr.Code)
	})

	t.Run("missing activity in message", func(t *testing.T) {
		// Create test processor
		processor, err := NewOutboxProcessor()
		require.NoError(t, err)

		// Create message without activity
		deliveryMsg := ActivityDeliveryMessage{
			Activity:    nil, // Missing activity
			TargetInbox: "https://remote.example/inbox",
		}

		msgJSON, _ := json.Marshal(deliveryMsg)

		// Create SQS event
		sqsEvent := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId: "test-message-id",
					Body:      string(msgJSON),
				},
			},
		}

		// Create Lift context
		ctx := &lift.Context{}
		ctx.Set("requestID", "test-request-id")

		// Execute handler
		err = processor.HandleSQS(ctx, sqsEvent)

		// Assert - should return error for batch with failures
		assert.Error(t, err)
		liftErr, ok := err.(*lift.LiftError)
		assert.True(t, ok)
		assert.Equal(t, "PARTIAL_FAILURE", liftErr.Code)
	})
}

func TestDeliveryRetry(t *testing.T) {
	processor, err := NewOutboxProcessor()
	require.NoError(t, err)

	// Test exponential backoff calculation
	t.Run("calculateBackoffDelay", func(t *testing.T) {
		tests := []struct {
			attempt     int
			expectedMin time.Duration
			expectedMax time.Duration
		}{
			{1, time.Second, 3 * time.Second},      // First retry
			{2, 2 * time.Second, 6 * time.Second},  // Second retry
			{3, 4 * time.Second, 12 * time.Second}, // Third retry
		}

		for _, tt := range tests {
			t.Run(string(rune(tt.attempt)), func(t *testing.T) {
				delay := processor.calculateBackoffDelay(tt.attempt)
				assert.True(t, delay >= tt.expectedMin, "delay %v should be >= %v", delay, tt.expectedMin)
				assert.True(t, delay <= processor.retryConfig.MaxDelay, "delay %v should be <= %v", delay, processor.retryConfig.MaxDelay)
			})
		}
	})

	t.Run("isPermanentError", func(t *testing.T) {
		tests := []struct {
			statusCode int
			expected   bool
		}{
			{400, true},  // Bad Request - permanent
			{401, true},  // Unauthorized - permanent
			{403, true},  // Forbidden - permanent
			{404, true},  // Not Found - permanent
			{410, true},  // Gone - permanent
			{422, true},  // Unprocessable Entity - permanent
			{500, false}, // Internal Server Error - temporary
			{502, false}, // Bad Gateway - temporary
			{503, false}, // Service Unavailable - temporary
		}

		for _, tt := range tests {
			t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
				result := processor.isPermanentError(tt.statusCode)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestExtractDomainFromURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/inbox", "example.com"},
		{"https://example.com:8080/inbox", "example.com"},
		{"http://example.com/inbox", "example.com"},
		{"https://sub.example.com/path/to/inbox", "sub.example.com"},
		{"", ""},
		{"example.com", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractDomainFromURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
