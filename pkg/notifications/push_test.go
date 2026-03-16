package notifications

import (
	"context"
	"strings"
	"testing"

	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestNewPushService_ReturnsNilWhenUnconfigured(t *testing.T) {
	t.Run("nil_cfg", func(t *testing.T) {
		svc, err := NewPushService(nil)
		require.NoError(t, err)
		require.Nil(t, svc)
	})

	t.Run("blank_queue_url", func(t *testing.T) {
		svc, err := NewPushService(&appConfig.Config{PushNotificationQueueURL: "   "})
		require.NoError(t, err)
		require.Nil(t, svc)
	})

	t.Run("creates_service_with_queue_url", func(t *testing.T) {
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_REGION", "us-east-1")

		svc, err := NewPushService(&appConfig.Config{PushNotificationQueueURL: " https://example.com/queue "})
		require.NoError(t, err)
		require.NotNil(t, svc)
		require.Equal(t, "https://example.com/queue", svc.queueURL)
		require.NotNil(t, svc.sqsClient)
	})
}

func TestPushService_QueueNotification_NilServiceNoops(t *testing.T) {
	var svc *PushService
	require.NoError(t, svc.QueueNotification(context.Background(), &PushMessage{
		Username:         "alice",
		NotificationType: "follow",
		Title:            "x",
		Body:             "y",
		NotificationID:   "n1",
		AccessToken:      "t",
	}))
}

func TestFormatNotificationTitle(t *testing.T) {
	tests := []struct {
		notificationType string
		actorName        string
		want             string
	}{
		{"follow", "alice", "alice followed you"},
		{"favourite", "alice", "alice favourited your post"},
		{"reblog", "alice", "alice boosted your post"},
		{"mention", "alice", "alice mentioned you"},
		{"reply", "alice", "alice replied to your post"},
		{"poll", "alice", "A poll you voted in has ended"},
		{"follow_request", "alice", "alice requested to follow you"},
		{"status", "alice", "alice posted"},
		{"update", "alice", "alice edited a post"},
		{"unknown", "alice", "New notification"},
	}

	for _, tc := range tests {
		require.Equal(t, tc.want, FormatNotificationTitle(tc.notificationType, tc.actorName))
	}
}

func TestFormatNotificationBody(t *testing.T) {
	require.Equal(t, "", FormatNotificationBody("follow", "hi"))

	long := strings.Repeat("a", 200)
	body := FormatNotificationBody("mention", long)
	require.Len(t, body, 100)
	require.True(t, strings.HasSuffix(body, "..."))

	replyBody := FormatNotificationBody("reply", long)
	require.Len(t, replyBody, 100)
	require.True(t, strings.HasSuffix(replyBody, "..."))
}
