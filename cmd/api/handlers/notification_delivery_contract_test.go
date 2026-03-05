package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/stretchr/testify/require"
)

func TestCommNotificationDeliveryContract_AcceptsFixture(t *testing.T) {
	fixturePath := filepath.Join("..", "testdata", "notification_deliver_fixture_v3.json")
	raw, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	var req apiModels.NotificationDeliveryRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	normalized, err := normalizeCommNotificationDeliveryRequest(&req)
	require.NoError(t, err)

	require.Equal(t, commNotificationTypeInbound, normalized.NotificationType)
	require.Equal(t, commNotificationChannelEmail, normalized.Channel)
	require.Equal(t, "alice@example.com", normalized.FromAddress)
	require.Equal(t, "", normalized.FromSoulAgentID)
	require.Equal(t, "Alice", normalized.FromDisplayName)
	require.Equal(t, "agent-bob@lessersoul.ai", normalized.ToAddress)
	require.Equal(t, "Re: Project collaboration", normalized.Subject)
	require.Equal(t, "Hello Bob,\n\nAre you available to collaborate on the project?", normalized.Body)
	require.Equal(t, time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC), normalized.ReceivedAt)
	require.Equal(t, "comm-msg-001", normalized.MessageID)
	require.Equal(t, "", normalized.InReplyTo)
	require.Len(t, normalized.Attachments, 1)
	require.Equal(t, "att-1", normalized.Attachments[0].ID)
	require.Equal(t, "proposal.pdf", normalized.Attachments[0].Filename)
	require.Equal(t, "application/pdf", normalized.Attachments[0].ContentType)
	require.Equal(t, int64(123456), normalized.Attachments[0].SizeBytes)
	require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", normalized.Attachments[0].SHA256)
}

func TestCommNotificationDeliveryContract_RejectsInvalidPayload(t *testing.T) {
	req := &apiModels.NotificationDeliveryRequest{
		Type:       "communication:inbound",
		Channel:    "pager",
		From:       apiModels.NotificationDeliveryFrom{Address: "alice@example.com"},
		Body:       "hi",
		ReceivedAt: "2026-03-04T12:00:00Z",
		MessageID:  "comm-msg-001",
	}

	_, err := normalizeCommNotificationDeliveryRequest(req)
	require.Error(t, err)
}

func TestCommNotificationID_IsStable(t *testing.T) {
	id1, err := commNotificationID("admin", "comm-msg-001")
	require.NoError(t, err)

	id2, err := commNotificationID("admin", "comm-msg-001")
	require.NoError(t, err)

	id3, err := commNotificationID("admin", "comm-msg-002")
	require.NoError(t, err)

	require.Equal(t, id1, id2)
	require.NotEqual(t, id1, id3)
}

func TestCommNotificationDeliveryContract_HelperCoverage(t *testing.T) {
	t.Run("sender recipient and in-reply-to normalize", func(t *testing.T) {
		soulAgentID := "agent-123"
		fromAddress, displayName, normalizedSoulAgentID, err := normalizeCommNotificationSender(apiModels.NotificationDeliveryFrom{
			Address:     "alice@example.com",
			DisplayName: "Alice <Admin>",
			SoulAgentID: &soulAgentID,
		})
		require.NoError(t, err)
		require.Equal(t, "alice@example.com", fromAddress)
		require.Equal(t, "Alice &lt;Admin&gt;", displayName)
		require.Equal(t, soulAgentID, normalizedSoulAgentID)

		toAddress, err := normalizeCommNotificationRecipient(commNotificationChannelSMS, nil)
		require.NoError(t, err)
		require.Empty(t, toAddress)

		replyTo := "  parent-1  "
		inReplyTo, err := normalizeCommNotificationInReplyTo(&replyTo)
		require.NoError(t, err)
		require.Equal(t, "parent-1", inReplyTo)

		inReplyTo, err = normalizeCommNotificationInReplyTo(nil)
		require.NoError(t, err)
		require.Empty(t, inReplyTo)
	})

	t.Run("content timestamps and ids validate", func(t *testing.T) {
		subject, body, err := normalizeCommNotificationContent(commNotificationChannelSMS, "", "hello <b>world</b>")
		require.NoError(t, err)
		require.Empty(t, subject)
		require.Equal(t, "hello &lt;b&gt;world&lt;/b&gt;", body)

		_, _, err = normalizeCommNotificationContent(commNotificationChannelEmail, "", "hello")
		require.Error(t, err)

		receivedAt, err := normalizeCommNotificationReceivedAt("2026-03-04T12:00:00Z")
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC), receivedAt)

		_, err = normalizeCommNotificationReceivedAt("not-a-timestamp")
		require.Error(t, err)

		messageID, err := normalizeCommNotificationMessageID(" message-1 ")
		require.NoError(t, err)
		require.Equal(t, "message-1", messageID)
	})

	t.Run("attachments normalize and validate", func(t *testing.T) {
		attachments, err := normalizeCommNotificationAttachments([]apiModels.NotificationDeliveryAttachment{
			{
				ID:          "att-1",
				Filename:    "proposal <final>.pdf",
				ContentType: "application/pdf",
				SizeBytes:   123,
				SHA256:      "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			},
		})
		require.NoError(t, err)
		require.Len(t, attachments, 1)
		require.Equal(t, "proposal &lt;final&gt;.pdf", attachments[0].Filename)
		require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", attachments[0].SHA256)

		_, err = normalizeCommNotificationAttachment(0, apiModels.NotificationDeliveryAttachment{
			ID:          "att-2",
			Filename:    "bad.bin",
			ContentType: "application/octet-stream",
			SizeBytes:   -1,
			SHA256:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		})
		require.Error(t, err)

		_, err = normalizeCommNotificationAttachmentSHA256("attachments[0].sha256", "invalid")
		require.Error(t, err)
	})

	t.Run("attachment integer limits and type validation", func(t *testing.T) {
		_, err := normalizeCommNotificationType("communication:outbound")
		require.Error(t, err)

		_, err = normalizeCommNotificationChannel("pager")
		require.Error(t, err)

		_, err = normalizeCommNotificationRecipient(commNotificationChannelEmail, nil)
		require.Error(t, err)

		var attachments []apiModels.NotificationDeliveryAttachment
		for i := 0; i < commNotificationMaxAttachments+1; i++ {
			attachments = append(attachments, apiModels.NotificationDeliveryAttachment{
				ID:          "att",
				Filename:    "file.txt",
				ContentType: "text/plain",
				SizeBytes:   1,
				SHA256:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			})
		}
		_, err = normalizeCommNotificationAttachments(attachments)
		require.Error(t, err)
	})
}
