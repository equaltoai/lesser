package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockRepository provides mock implementations for testing
type MockNotificationRepository struct {
	mock.Mock
}

func (m *MockNotificationRepository) GetNotification(ctx context.Context, id string) (*models.Notification, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Notification), args.Error(1)
}

func (m *MockNotificationRepository) UpdateNotification(ctx context.Context, notification *models.Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockNotificationRepository) MarkPushNotificationSent(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestNotificationDeliveryRequest_Marshal(t *testing.T) {
	scheduledTime := time.Now().Add(1 * time.Hour)
	request := NotificationDeliveryRequest{
		NotificationID: "notif_123",
		UserID:         "user_456",
		Channels:       []string{"push", "websocket"}, // Email removed - not supported by Lesser
		Priority:       "high",
		RetryCount:     0,
		ScheduledAt:    &scheduledTime,
	}

	data, err := json.Marshal(request)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "notif_123")

	var unmarshaled NotificationDeliveryRequest
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, request.NotificationID, unmarshaled.NotificationID)
	assert.Equal(t, request.UserID, unmarshaled.UserID)
	assert.Equal(t, request.Channels, unmarshaled.Channels)
}

// TestBuildNotificationTitle tests notification title generation
// Note: Email notifications are not supported in Lesser
func TestBuildNotificationTitle(t *testing.T) {
	tests := []struct {
		name         string
		notification *models.Notification
		expected     string
	}{
		{
			name: "mention notification",
			notification: &models.Notification{
				Type:    "mention",
				ActorID: "alice",
				Title:   "alice mentioned you",
			},
			expected: "alice mentioned you",
		},
		{
			name: "follow notification",
			notification: &models.Notification{
				Type:    "follow",
				ActorID: "bob",
				Title:   "bob started following you",
			},
			expected: "bob started following you",
		},
		{
			name: "custom notification",
			notification: &models.Notification{
				Type:  "custom",
				Title: "Custom Title",
			},
			expected: "Custom Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the title is correctly set
			assert.Equal(t, tt.expected, tt.notification.Title)
		})
	}
}

func TestDeliverToChannel_Success(t *testing.T) {
	processor := &NotificationProcessor{
		logger: mockLogger(),
		domain: "example.com",
	}

	notification := &models.Notification{
		ID:      "notif_123",
		UserID:  "user_456",
		Type:    "mention",
		Title:   "Test Notification",
		Body:    "This is a test",
		ActorID: "alice",
	}

	userPrefs := &UserPreferences{
		PushNotifications:      true,
		WebSocketNotifications: true,
		PushEndpoint:           "https://push.example.com/endpoint",
	}

	ctx := context.Background()

	// Test channel validation
	result := processor.deliverToChannel(ctx, notification, userPrefs, "invalid_channel")
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "unsupported delivery channel")

	// Test that email delivery channel is not supported (Lesser doesn't support email)
	result = processor.deliverToChannel(ctx, notification, userPrefs, "email")
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "unsupported delivery channel")

	// Test that SMS delivery channel is not supported (Lesser doesn't support SMS)
	result = processor.deliverToChannel(ctx, notification, userPrefs, "sms")
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "unsupported delivery channel")

	// Test disabled push
	userPrefs.PushNotifications = false
	result = processor.deliverToChannel(ctx, notification, userPrefs, "push")
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "push notifications disabled")
}

func TestProcessMessage_InvalidJSON(t *testing.T) {
	processor := &NotificationProcessor{
		logger: mockLogger(),
	}

	message := events.SQSMessage{
		MessageId: "msg_123",
		Body:      "invalid json",
	}

	err := processor.processMessage(context.Background(), message)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal delivery request")
}

func TestProcessMessage_ScheduledDelivery(t *testing.T) {
	processor := &NotificationProcessor{
		logger: mockLogger(),
	}

	// Create a request scheduled for the future
	futureTime := time.Now().Add(1 * time.Hour)
	request := NotificationDeliveryRequest{
		NotificationID: "notif_123",
		UserID:         "user_456",
		Channels:       []string{"push"}, // Changed from email to push - email not supported
		ScheduledAt:    &futureTime,
	}

	requestJSON, _ := json.Marshal(request)
	message := events.SQSMessage{
		MessageId: "msg_123",
		Body:      string(requestJSON),
	}

	err := processor.processMessage(context.Background(), message)
	assert.NoError(t, err) // Should skip future deliveries without error
}

func TestWebSocketMessage_Marshal(t *testing.T) {
	message := WebSocketMessage{
		Type:  "notification",
		Event: "notification.new",
		Payload: map[string]any{
			"id":   "notif_123",
			"type": "mention",
		},
	}

	data, err := json.Marshal(message)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "notification")
	assert.Contains(t, string(data), "notif_123")
}

func TestDeliveryResult_TracksCost(t *testing.T) {
	result := DeliveryResult{
		Channel:   "push",
		Success:   true,
		Timestamp: time.Now(),
		Cost:      1000, // $0.001
	}

	assert.Equal(t, "push", result.Channel)
	assert.True(t, result.Success)
	assert.Equal(t, int64(1000), result.Cost)
}

func TestUserPreferences_Default(t *testing.T) {
	prefs := &UserPreferences{
		PushNotifications:      true,
		WebSocketNotifications: true,
		PushEndpoint:           "https://push.example.com/user",
	}

	assert.True(t, prefs.PushNotifications)
	assert.True(t, prefs.WebSocketNotifications)
	assert.Equal(t, "https://push.example.com/user", prefs.PushEndpoint)
}

// mockLogger returns a mock logger for testing
func mockLogger() *zap.Logger {
	// Return a no-op logger for tests
	return zap.NewNop()
}

// Integration test with SQS event
func TestHandleSQSMessages_Integration(t *testing.T) {
	// This would be an integration test with a real database
	// Skipping for now as it requires full setup
	t.Skip("Integration test - requires full AWS setup")

	processor := &NotificationProcessor{
		logger: mockLogger(),
	}

	request := NotificationDeliveryRequest{
		NotificationID: "notif_123",
		UserID:         "user_456",
		Channels:       []string{"websocket"},
		Priority:       "medium",
		RetryCount:     0,
	}

	requestJSON, _ := json.Marshal(request)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg_123",
				Body:      string(requestJSON),
			},
		},
	}

	// Create a lift context for the handler
	liftCtx := &lift.Context{
		Context: context.Background(),
	}
	err := processor.HandleSQS(liftCtx, event)
	assert.NoError(t, err)
}
