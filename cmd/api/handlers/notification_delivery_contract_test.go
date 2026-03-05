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
