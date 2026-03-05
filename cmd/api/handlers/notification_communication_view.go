package handlers

import (
	"strings"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
)

func communicationNotificationFromData(notifType string, createdAt time.Time, data map[string]interface{}) *apiModels.CommunicationNotification {
	typ := strings.ToLower(strings.TrimSpace(notifType))
	if !strings.HasPrefix(typ, "communication:") {
		return nil
	}

	messageID := extractStringFromNotificationData(data, "messageId", "message_id")
	if messageID == "" {
		return nil
	}

	channel := extractStringFromNotificationData(data, "channel")

	subject := extractStringFromNotificationData(data, "subject", "title")
	body := extractStringFromNotificationData(data, "body")

	receivedAt := createdAt.UTC()
	if raw := extractStringFromNotificationData(data, "receivedAt", "received_at"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			receivedAt = parsed.UTC()
		} else if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			receivedAt = parsed.UTC()
		}
	}

	inReplyTo := extractStringFromNotificationData(data, "inReplyTo", "in_reply_to")

	threadID := messageID
	if inReplyTo != "" {
		threadID = inReplyTo
	}

	from := apiModels.CommunicationFrom{
		Address:     "",
		DisplayName: "",
		SoulAgentID: "",
	}

	if data != nil {
		switch raw := data["from"].(type) {
		case map[string]interface{}:
			from.Address = extractStringFromNotificationData(raw, "address")
			from.DisplayName = extractStringFromNotificationData(raw, "displayName", "display_name")
			from.SoulAgentID = extractStringFromNotificationData(raw, "soulAgentId", "soul_agent_id")
		}
	}

	return &apiModels.CommunicationNotification{
		Channel:    channel,
		From:       from,
		Subject:    subject,
		Body:       body,
		ReceivedAt: receivedAt,
		MessageID:  messageID,
		InReplyTo:  inReplyTo,
		ThreadID:   threadID,
	}
}

func extractStringFromNotificationData(data map[string]interface{}, keys ...string) string {
	if data == nil {
		return ""
	}

	for _, key := range keys {
		raw, ok := data[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case []byte:
			if trimmed := strings.TrimSpace(string(value)); trimmed != "" {
				return trimmed
			}
		}
	}

	return ""
}
