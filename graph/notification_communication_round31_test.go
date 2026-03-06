package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound31CommunicationNotificationFromData(t *testing.T) {
	createdAt := time.Date(2026, time.March, 6, 12, 0, 0, 0, time.UTC)
	data := map[string]interface{}{
		"channel":    "email",
		"subject":    "Quarterly update",
		"body":       "Attached is the latest report.",
		"messageId":  "msg-1",
		"inReplyTo":  "thread-1",
		"receivedAt": "2026-03-06T11:59:00Z",
		"from": map[string]interface{}{
			"address":     "sender@example.com",
			"displayName": "Sender",
			"soulAgentId": "agent-123",
		},
		"to": map[string]interface{}{
			"address": "alice@example.com",
		},
		"attachments": []map[string]interface{}{
			{
				"id":          "att-1",
				"filename":    "report.pdf",
				"contentType": "application/pdf",
				"sizeBytes":   "42",
				"sha256":      "abc123",
			},
		},
	}

	notification := communicationNotificationFromData("communication:inbound", createdAt, data)
	require.NotNil(t, notification)
	require.Equal(t, "email", notification.Channel)
	require.Equal(t, "sender@example.com", notification.From.Address)
	require.Equal(t, "Sender", *notification.From.DisplayName)
	require.Equal(t, "agent-123", *notification.From.SoulAgentID)
	require.NotNil(t, notification.To)
	require.Equal(t, "alice@example.com", notification.To.Address)
	require.Equal(t, "Quarterly update", *notification.Subject)
	require.Equal(t, "Attached is the latest report.", *notification.Body)
	require.Equal(t, "msg-1", notification.MessageID)
	require.Equal(t, "thread-1", *notification.InReplyTo)
	require.Equal(t, "thread-1", notification.ThreadID)
	require.Equal(t, time.Date(2026, time.March, 6, 11, 59, 0, 0, time.UTC), time.Time(notification.ReceivedAt))
	require.Len(t, notification.Attachments, 1)
	require.Equal(t, "att-1", notification.Attachments[0].ID)
	require.Equal(t, "report.pdf", notification.Attachments[0].Filename)
	require.Equal(t, "application/pdf", notification.Attachments[0].ContentType)
	require.Equal(t, 42, notification.Attachments[0].SizeBytes)
	require.Equal(t, "abc123", notification.Attachments[0].Sha256)

	require.Nil(t, communicationNotificationFromData("mention", createdAt, data))
	require.Nil(t, communicationNotificationFromData("communication:inbound", createdAt, map[string]interface{}{}))
}

func TestRound31NotificationsQueryIncludesCommunicationPayload(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")

	repo, ok := storageRepo.notificationRepo.(*inmemory.NotificationRepository)
	require.True(t, ok)

	createdAt := time.Date(2026, time.March, 6, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateNotification(ctx, &models.Notification{
		ID:        "notif-comm-1",
		UserID:    "alice",
		Type:      "communication:inbound",
		ActorID:   "bob",
		CreatedAt: createdAt,
		Data: map[string]interface{}{
			"channel":   "email",
			"subject":   "Ping",
			"body":      "Hello",
			"messageId": "msg-2",
			"from": map[string]interface{}{
				"address":      "bob@example.com",
				"display_name": "Bob",
			},
			"attachments": []interface{}{
				map[string]interface{}{
					"id":          "att-2",
					"filename":    "hello.txt",
					"contentType": "text/plain",
					"sizeBytes":   float64(12),
					"sha256":      "def456",
				},
			},
		},
	}))

	conn, err := resolver.Query().Notifications(ctx, []string{"communication:inbound"}, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Len(t, conn.Edges, 1)

	communication := conn.Edges[0].Node.Communication
	require.NotNil(t, communication)
	require.Equal(t, "email", communication.Channel)
	require.Equal(t, "bob@example.com", communication.From.Address)
	require.Equal(t, "Bob", *communication.From.DisplayName)
	require.Equal(t, "Ping", *communication.Subject)
	require.Equal(t, "Hello", *communication.Body)
	require.Equal(t, "msg-2", communication.MessageID)
	require.Equal(t, "msg-2", communication.ThreadID)
	require.Len(t, communication.Attachments, 1)
	require.Equal(t, "att-2", communication.Attachments[0].ID)
}
