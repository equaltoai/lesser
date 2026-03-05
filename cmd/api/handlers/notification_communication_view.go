package handlers

import (
	"encoding/json"
	"strconv"
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

	var to *apiModels.CommunicationTo
	if data != nil {
		switch raw := data["to"].(type) {
		case map[string]interface{}:
			if address := extractStringFromNotificationData(raw, "address"); address != "" {
				to = &apiModels.CommunicationTo{Address: address}
			}
		}
	}

	var attachments []apiModels.CommunicationAttachment
	if data != nil {
		switch raw := data["attachments"].(type) {
		case []interface{}:
			for _, item := range raw {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if attachment := communicationAttachmentFromData(itemMap); attachment != nil {
					attachments = append(attachments, *attachment)
				}
			}
		case []map[string]interface{}:
			for _, itemMap := range raw {
				if attachment := communicationAttachmentFromData(itemMap); attachment != nil {
					attachments = append(attachments, *attachment)
				}
			}
		}
	}

	return &apiModels.CommunicationNotification{
		Channel:     channel,
		From:        from,
		To:          to,
		Attachments: attachments,
		Subject:     subject,
		Body:        body,
		ReceivedAt:  receivedAt,
		MessageID:   messageID,
		InReplyTo:   inReplyTo,
		ThreadID:    threadID,
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

func extractInt64FromNotificationData(data map[string]interface{}, keys ...string) int64 {
	if data == nil {
		return 0
	}

	for _, key := range keys {
		raw, ok := data[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case int64:
			return value
		case int:
			return int64(value)
		case float64:
			return int64(value)
		case json.Number:
			if parsed, err := value.Int64(); err == nil {
				return parsed
			}
		case string:
			if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
				return parsed
			}
		case []byte:
			if parsed, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64); err == nil {
				return parsed
			}
		}
	}

	return 0
}

func communicationAttachmentFromData(data map[string]interface{}) *apiModels.CommunicationAttachment {
	id := extractStringFromNotificationData(data, "id")
	if id == "" {
		return nil
	}

	return &apiModels.CommunicationAttachment{
		ID:          id,
		Filename:    extractStringFromNotificationData(data, "filename"),
		ContentType: extractStringFromNotificationData(data, "contentType", "content_type"),
		SizeBytes:   extractInt64FromNotificationData(data, "sizeBytes", "size_bytes"),
		SHA256:      extractStringFromNotificationData(data, "sha256"),
	}
}
