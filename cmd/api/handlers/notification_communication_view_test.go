package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCommunicationNotificationView_HelperCoverage(t *testing.T) {
	t.Run("parses fallback timestamp and []map attachments", func(t *testing.T) {
		createdAt := time.Date(2026, time.March, 5, 10, 0, 0, 0, time.UTC)
		data := map[string]interface{}{
			"channel": "email",
			"from": map[string]interface{}{
				"address":       "alice@example.com",
				"display_name":  "Alice",
				"soul_agent_id": "agent-1",
			},
			"to": map[string]interface{}{
				"address": "agent-bob@lessersoul.ai",
			},
			"received_at": "2026-03-04T12:00:00Z",
			"message_id":  "comm-msg-100",
			"attachments": []map[string]interface{}{
				{
					"id":           "att-1",
					"filename":     "proposal.pdf",
					"content_type": "application/pdf",
					"size_bytes":   json.Number("42"),
					"sha256":       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
		}

		notification := communicationNotificationFromData("communication:inbound", createdAt, data)
		require.NotNil(t, notification)
		require.Equal(t, time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC), notification.ReceivedAt)
		require.Equal(t, "alice@example.com", notification.From.Address)
		require.Equal(t, "Alice", notification.From.DisplayName)
		require.Equal(t, "agent-1", notification.From.SoulAgentID)
		require.NotNil(t, notification.To)
		require.Equal(t, "agent-bob@lessersoul.ai", notification.To.Address)
		require.Len(t, notification.Attachments, 1)
		require.Equal(t, int64(42), notification.Attachments[0].SizeBytes)
	})

	t.Run("falls back on invalid data variants", func(t *testing.T) {
		createdAt := time.Date(2026, time.March, 5, 10, 0, 0, 0, time.UTC)
		data := map[string]interface{}{
			"messageId":  "comm-msg-101",
			"receivedAt": "not-a-timestamp",
			"to": map[string]interface{}{
				"address": "   ",
			},
			"attachments": "not-a-slice",
		}

		notification := communicationNotificationFromData("communication:inbound", createdAt, data)
		require.NotNil(t, notification)
		require.Equal(t, createdAt, notification.ReceivedAt)
		require.Nil(t, notification.To)
		require.Nil(t, notification.Attachments)

		require.Nil(t, communicationNotificationFromData("mention", createdAt, data))
		require.Nil(t, communicationNotificationFromData("communication:inbound", createdAt, map[string]interface{}{}))
	})

	t.Run("extract helpers cover numeric forms", func(t *testing.T) {
		require.Equal(t, int64(7), extractInt64FromNotificationData(map[string]interface{}{"sizeBytes": 7}, "sizeBytes"))
		require.Equal(t, int64(8), extractInt64FromNotificationData(map[string]interface{}{"sizeBytes": float64(8)}, "sizeBytes"))
		require.Equal(t, int64(9), extractInt64FromNotificationData(map[string]interface{}{"sizeBytes": "9"}, "sizeBytes"))
		require.Equal(t, int64(10), extractInt64FromNotificationData(map[string]interface{}{"sizeBytes": []byte("10")}, "sizeBytes"))
		require.Equal(t, int64(11), extractInt64FromNotificationData(map[string]interface{}{"sizeBytes": json.Number("11")}, "sizeBytes"))
		require.Zero(t, extractInt64FromNotificationData(nil, "sizeBytes"))

		require.Equal(t, "hello", extractStringFromNotificationData(map[string]interface{}{"body": []byte(" hello ")}, "body"))
		require.Empty(t, extractStringFromNotificationData(map[string]interface{}{"body": "   "}, "body"))

		parsed, ok := parseNotificationTime("2026-03-04T12:00:00Z")
		require.True(t, ok)
		require.Equal(t, time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC), parsed)

		_, ok = parseNotificationTime("invalid")
		require.False(t, ok)

		require.Nil(t, communicationAttachmentFromData(map[string]interface{}{}))
	})

	t.Run("project37 treats compound email actor placeholders as external", func(t *testing.T) {
		actor := communicationEmailActorPlaceholder("pilot.simulacrum@lessersoul.ai", "Pilot")
		require.NotNil(t, actor)
		require.Equal(t, "pilot.simulacrum@lessersoul.ai", actor.ID)
		require.Equal(t, "pilot.simulacrum@lessersoul.ai", actor.URL)
		require.Equal(t, "pilot.simulacrum@lessersoul.ai", actor.PreferredUsername)
		require.Equal(t, "Pilot", actor.Name)

		require.Nil(t, communicationEmailActorPlaceholder("pilot.simulacrum", "Pilot"))
		require.Nil(t, communicationEmailActorPlaceholder("https://lessersoul.ai/users/pilot", "Pilot"))
	})
}
