package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNotification_TableName(t *testing.T) {
	n := &Notification{}
	assert.Equal(t, MainTableName, n.TableName())
}

// TestNotification_BeforeCreate removed - complex fixtures and model hooks
// better suited for integration tests

func TestNotification_Validate(t *testing.T) {
	tests := []struct {
		name         string
		notification *Notification
		wantErr      bool
		errMsg       string
	}{
		{
			name: "valid notification",
			notification: &Notification{
				ID:      "notif123",
				UserID:  "user123",
				Type:    "mention",
				ActorID: "actor123",
			},
			wantErr: false,
		},
		{
			name: "empty ID",
			notification: &Notification{
				UserID:  "user123",
				Type:    "mention",
				ActorID: "actor123",
			},
			wantErr: true,
			errMsg:  "validation failed for ID",
		},
		{
			name: "whitespace UserID",
			notification: &Notification{
				ID:      "notif123",
				UserID:  "   ",
				Type:    "mention",
				ActorID: "actor123",
			},
			wantErr: true,
			errMsg:  "validation failed for UserID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.notification.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNotification_MarkRead(t *testing.T) {
	notification := &Notification{}
	assert.False(t, notification.IsRead)
	assert.Nil(t, notification.ReadAt)

	notification.MarkRead()
	assert.True(t, notification.IsRead)
	assert.NotNil(t, notification.ReadAt)
	assert.True(t, time.Since(*notification.ReadAt) < time.Second)

	// Calling MarkRead again shouldn't change ReadAt
	originalReadAt := *notification.ReadAt
	time.Sleep(10 * time.Millisecond)
	notification.MarkRead()
	assert.Equal(t, originalReadAt, *notification.ReadAt)
}

func TestNotification_MarkUnread(t *testing.T) {
	notification := &Notification{
		IsRead: true,
		ReadAt: &time.Time{},
	}

	notification.MarkUnread()
	assert.False(t, notification.IsRead)
	assert.Nil(t, notification.ReadAt)
}

func TestNotification_MarkPushSent(t *testing.T) {
	notification := &Notification{
		PushError: "some error",
	}

	notification.MarkPushSent()
	assert.True(t, notification.PushSent)
	assert.NotNil(t, notification.PushSentAt)
	assert.Empty(t, notification.PushError)
	assert.True(t, time.Since(*notification.PushSentAt) < time.Second)
}

func TestNotification_MarkPushFailed(t *testing.T) {
	notification := &Notification{
		PushSent: true,
	}

	errorMsg := "failed to send push"
	notification.MarkPushFailed(errorMsg)

	assert.False(t, notification.PushSent)
	assert.Equal(t, errorMsg, notification.PushError)
}

func TestNotification_ShouldSendPush(t *testing.T) {
	tests := []struct {
		name         string
		notification *Notification
		expected     bool
	}{
		{
			name: "should send - not sent, no error",
			notification: &Notification{
				PushSent:  false,
				PushError: "",
			},
			expected: true,
		},
		{
			name: "should not send - already sent",
			notification: &Notification{
				PushSent:  true,
				PushError: "",
			},
			expected: false,
		},
		{
			name: "should not send - has error",
			notification: &Notification{
				PushSent:  false,
				PushError: "failed",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.notification.ShouldSendPush()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNotification_DataManagement(t *testing.T) {
	notification := &Notification{}

	// Test getting non-existent key
	value, exists := notification.GetData("key1")
	assert.Nil(t, value)
	assert.False(t, exists)

	// Test setting and getting
	notification.SetData("key1", "value1")
	notification.SetData("key2", 42)

	value, exists = notification.GetData("key1")
	assert.Equal(t, "value1", value)
	assert.True(t, exists)

	value, exists = notification.GetData("key2")
	assert.Equal(t, 42, value)
	assert.True(t, exists)

	// Test non-existent key
	value, exists = notification.GetData("key3")
	assert.Nil(t, value)
	assert.False(t, exists)
}

func TestNotification_IncrementGroupCount(t *testing.T) {
	notification := &Notification{
		GroupCount: 1,
	}

	notification.IncrementGroupCount()
	assert.Equal(t, 2, notification.GroupCount)

	notification.IncrementGroupCount()
	assert.Equal(t, 3, notification.GroupCount)
}

func TestNotification_generateGroupKey(t *testing.T) {
	createTime := time.Date(2023, 1, 1, 12, 30, 45, 0, time.UTC)
	notification := &Notification{
		UserID:    "user123",
		Type:      "mention",
		ActorID:   "actor456",
		TargetID:  "status789",
		CreatedAt: createTime,
	}

	groupKey := notification.generateGroupKey()

	// Should group by hour window (12:00)
	expected := "user123:mention:actor456:status789:2023010112"
	assert.Equal(t, expected, groupKey)

	// Test different hour
	notification.CreatedAt = time.Date(2023, 1, 1, 13, 15, 30, 0, time.UTC)
	groupKey = notification.generateGroupKey()
	expected = "user123:mention:actor456:status789:2023010113"
	assert.Equal(t, expected, groupKey)
}

func TestIsValidNotificationType(t *testing.T) {
	tests := []struct {
		notifType string
		expected  bool
	}{
		{"mention", true},
		{"reblog", true},
		{"favourite", true},
		{"follow", true},
		{"follow_request", true},
		{"poll", true},
		{"status", true},
		{"update", true},
		{"admin.sign_up", true},
		{"admin.report", true},
		{"MENTION", true}, // Case insensitive
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.notifType, func(t *testing.T) {
			result := isValidNotificationType(tt.notifType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNotificationBuilder(t *testing.T) {
	notification := NewNotificationBuilder().
		ForUser("user123").
		OfType("mention").
		FromActor("actor456", "user").
		AboutTarget("status789", "status").
		WithContent("Test Title", "Test Body").
		WithData("key1", "value1").
		WithGroupKey("custom-group").
		Build()

	assert.Equal(t, "user123", notification.UserID)
	assert.Equal(t, "mention", notification.Type)
	assert.Equal(t, "actor456", notification.ActorID)
	assert.Equal(t, "user", notification.ActorType)
	assert.Equal(t, "status789", notification.TargetID)
	assert.Equal(t, "status", notification.TargetType)
	assert.Equal(t, "Test Title", notification.Title)
	assert.Equal(t, "Test Body", notification.Body)
	assert.Equal(t, "custom-group", notification.GroupKey)

	value, exists := notification.GetData("key1")
	assert.True(t, exists)
	assert.Equal(t, "value1", value)
}

func TestNewMentionNotification(t *testing.T) {
	notification := NewMentionNotification("user123", "actor456", "status789")

	assert.Equal(t, "user123", notification.UserID)
	assert.Equal(t, "mention", notification.Type)
	assert.Equal(t, "actor456", notification.ActorID)
	assert.Equal(t, "user", notification.ActorType)
	assert.Equal(t, "status789", notification.TargetID)
	assert.Equal(t, "status", notification.TargetType)
	assert.Equal(t, "New mention", notification.Title)
	assert.Equal(t, "You were mentioned in a post", notification.Body)
}

func TestNewFollowNotification(t *testing.T) {
	notification := NewFollowNotification("user123", "follower456")

	assert.Equal(t, "user123", notification.UserID)
	assert.Equal(t, "follow", notification.Type)
	assert.Equal(t, "follower456", notification.ActorID)
	assert.Equal(t, "user", notification.ActorType)
	assert.Equal(t, "follower456", notification.TargetID)
	assert.Equal(t, "user", notification.TargetType)
	assert.Equal(t, "New follower", notification.Title)
	assert.Equal(t, "Someone started following you", notification.Body)
}

func TestNewReblogNotification(t *testing.T) {
	notification := NewReblogNotification("user123", "reblogger456", "status789")

	assert.Equal(t, "user123", notification.UserID)
	assert.Equal(t, "reblog", notification.Type)
	assert.Equal(t, "reblogger456", notification.ActorID)
	assert.Equal(t, "user", notification.ActorType)
	assert.Equal(t, "status789", notification.TargetID)
	assert.Equal(t, "status", notification.TargetType)
	assert.Equal(t, "Your post was reblogged", notification.Title)
	assert.Equal(t, "Someone reblogged your post", notification.Body)
}

func TestNewFavouriteNotification(t *testing.T) {
	notification := NewFavouriteNotification("user123", "liker456", "status789")

	assert.Equal(t, "user123", notification.UserID)
	assert.Equal(t, "favourite", notification.Type)
	assert.Equal(t, "liker456", notification.ActorID)
	assert.Equal(t, "user", notification.ActorType)
	assert.Equal(t, "status789", notification.TargetID)
	assert.Equal(t, "status", notification.TargetType)
	assert.Equal(t, "Your post was favourited", notification.Title)
	assert.Equal(t, "Someone favourited your post", notification.Body)
}

func TestNewFollowRequestNotification(t *testing.T) {
	notification := NewFollowRequestNotification("user123", "requester456")

	assert.Equal(t, "user123", notification.UserID)
	assert.Equal(t, "follow_request", notification.Type)
	assert.Equal(t, "requester456", notification.ActorID)
	assert.Equal(t, "user", notification.ActorType)
	assert.Equal(t, "requester456", notification.TargetID)
	assert.Equal(t, "user", notification.TargetType)
	assert.Equal(t, "New follow request", notification.Title)
	assert.Equal(t, "Someone requested to follow you", notification.Body)
}

func TestNotification_setupGSIKeys(t *testing.T) {
	createTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	notification := &Notification{
		ID:        "notif123",
		UserID:    "user123",
		Type:      "mention",
		ActorID:   "actor456",
		GroupKey:  "group123",
		CreatedAt: createTime,
	}

	notification.setupGSIKeys()

	expectedTimeStr := "2023-01-01T12:00:00Z"

	assert.Equal(t, "NOTIF_TYPE#mention", notification.GSI1PK)
	assert.Equal(t, expectedTimeStr+"#user123#notif123", notification.GSI1SK)
	assert.Equal(t, "NOTIF_ACTOR#actor456", notification.GSI2PK)
	assert.Equal(t, expectedTimeStr+"#user123#notif123", notification.GSI2SK)
	assert.Equal(t, "NOTIF_GROUP#group123", notification.GSI3PK)
	assert.Equal(t, expectedTimeStr+"#notif123", notification.GSI3SK)

	// Test with empty ActorID
	notification.ActorID = ""
	notification.setupGSIKeys()
	assert.Empty(t, notification.GSI2PK)
	assert.Empty(t, notification.GSI2SK)
}

// TestCalculateEmailCost verifies email cost calculations always return 0
func TestCalculateEmailCost(t *testing.T) {
	// Email notifications are not supported by Lesser
	cost := CalculateEmailCost(1)
	assert.Equal(t, int64(0), cost)

	cost = CalculateEmailCost(100)
	assert.Equal(t, int64(0), cost)

	cost = CalculateEmailCost(1000)
	assert.Equal(t, int64(0), cost)
}

// TestCalculateSMSCost verifies SMS cost calculations always return 0
func TestCalculateSMSCost(t *testing.T) {
	// SMS notifications are not supported by Lesser
	cost := CalculateSMSCost(1)
	assert.Equal(t, int64(0), cost)

	cost = CalculateSMSCost(100)
	assert.Equal(t, int64(0), cost)

	cost = CalculateSMSCost(1000)
	assert.Equal(t, int64(0), cost)
}

// TestNotificationCostTracking_AddCost verifies email/SMS costs are ignored
func TestNotificationCostTracking_AddCost(t *testing.T) {
	tracking := &NotificationCostTracking{}

	// Test that email costs are ignored
	tracking.AddCost("email", 1000)
	assert.Equal(t, int64(0), tracking.TotalCostMicroCents)

	// Test that SMS costs are ignored
	tracking.AddCost("sms", 1000)
	assert.Equal(t, int64(0), tracking.TotalCostMicroCents)

	// Test that push costs are added
	tracking.AddCost("push", 500)
	assert.Equal(t, int64(500), tracking.PushCostMicroCents)
	assert.Equal(t, int64(500), tracking.TotalCostMicroCents)

	// Test that websocket costs are added
	tracking.AddCost("websocket", 300)
	assert.Equal(t, int64(300), tracking.WebSocketCostMicroCents)
	assert.Equal(t, int64(800), tracking.TotalCostMicroCents)
}

// TestNotificationDelivery_EmailSMSChannelsRejected verifies email/SMS channels are rejected
func TestNotificationDelivery_EmailSMSChannelsRejected(t *testing.T) {
	// Test email delivery method validation
	delivery := NewNotificationDelivery("notif_123", "email")
	err := delivery.Validate()
	assert.Error(t, err)
	assert.Equal(t, "Invalid delivery method: email", err.Error())

	// Test SMS delivery method validation
	delivery = NewNotificationDelivery("notif_123", "sms")
	err = delivery.Validate()
	assert.Error(t, err)
	assert.Equal(t, "Invalid delivery method: sms", err.Error())

	// Test valid delivery methods
	delivery = NewNotificationDelivery("notif_123", "push")
	err = delivery.Validate()
	assert.NoError(t, err)

	delivery = NewNotificationDelivery("notif_123", "websocket")
	err = delivery.Validate()
	assert.NoError(t, err)
}
