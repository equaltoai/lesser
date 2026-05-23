package handlers

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
)

func communicationNotificationFromData(notifType string, createdAt time.Time, data map[string]interface{}) *apiModels.CommunicationNotification {
	if !isCommunicationNotificationType(notifType) {
		return nil
	}

	messageID := extractStringFromNotificationData(data, "messageId", "message_id")
	if messageID == "" {
		return nil
	}

	channel := extractStringFromNotificationData(data, "channel")
	subject := extractStringFromNotificationData(data, "subject", "title")
	body := extractStringFromNotificationData(data, "body")
	receivedAt := communicationReceivedAtFromData(createdAt, data)
	inReplyTo := extractStringFromNotificationData(data, "inReplyTo", "in_reply_to")

	return &apiModels.CommunicationNotification{
		Channel:     channel,
		From:        communicationFromFromData(data),
		To:          communicationToFromData(data),
		Attachments: communicationAttachmentsFromData(data),
		Subject:     subject,
		Body:        body,
		ReceivedAt:  receivedAt,
		MessageID:   messageID,
		InReplyTo:   inReplyTo,
		ThreadID:    communicationThreadID(messageID, inReplyTo),
	}
}

func isCommunicationNotificationType(notifType string) bool {
	typ := strings.ToLower(strings.TrimSpace(notifType))
	return strings.HasPrefix(typ, "communication:")
}

func communicationReceivedAtFromData(createdAt time.Time, data map[string]interface{}) time.Time {
	receivedAt := createdAt.UTC()
	raw := extractStringFromNotificationData(data, "receivedAt", "received_at")
	if raw == "" {
		return receivedAt
	}

	if parsed, ok := parseNotificationTime(raw); ok {
		return parsed
	}

	return receivedAt
}

func parseNotificationTime(raw string) (time.Time, bool) {
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed.UTC(), true
	}

	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), true
	}

	return time.Time{}, false
}

func communicationThreadID(messageID, inReplyTo string) string {
	if inReplyTo != "" {
		return inReplyTo
	}

	return messageID
}

func communicationFromFromData(data map[string]interface{}) apiModels.CommunicationFrom {
	from := apiModels.CommunicationFrom{}
	raw, ok := data["from"].(map[string]interface{})
	if !ok {
		return from
	}

	from.Address = extractStringFromNotificationData(raw, "address")
	from.DisplayName = extractStringFromNotificationData(raw, "displayName", "display_name")
	from.SoulAgentID = extractStringFromNotificationData(raw, "soulAgentId", "soul_agent_id")
	return from
}

func communicationToFromData(data map[string]interface{}) *apiModels.CommunicationTo {
	raw, ok := data["to"].(map[string]interface{})
	if !ok {
		return nil
	}

	address := extractStringFromNotificationData(raw, "address")
	if address == "" {
		return nil
	}

	return &apiModels.CommunicationTo{Address: address}
}

func communicationEmailActorPlaceholder(address string, displayName string) *activitypub.Actor {
	address = strings.TrimSpace(address)
	if !validNotificationFallbackActorID(address) ||
		!notificationActorIDLooksEmailLike(address) ||
		strings.Count(address, "@") != 1 ||
		strings.Contains(address, "://") {
		return nil
	}

	// Reject email addresses that contain URL scheme indicators even after trimming.
	lower := strings.ToLower(address)
	for _, scheme := range dangerousURLSchemes {
		if strings.HasPrefix(lower, scheme+":") {
			return nil
		}
	}

	parts := strings.Split(address, "@")
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		name = address
	}

	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   address,
			Type: activitypub.PersonType,
		},
		PreferredUsername: address,
		Name:              name,
		// URL intentionally empty — email addresses are not profile URLs.
		// API consumers that render actor links should fall back to ID or omit the link.
	}
}

func communicationAttachmentsFromData(data map[string]interface{}) []apiModels.CommunicationAttachment {
	if data == nil {
		return nil
	}

	switch raw := data["attachments"].(type) {
	case []interface{}:
		return communicationAttachmentsFromInterfaceSlice(raw)
	case []map[string]interface{}:
		return communicationAttachmentsFromMapSlice(raw)
	default:
		return nil
	}
}

func communicationAttachmentsFromInterfaceSlice(raw []interface{}) []apiModels.CommunicationAttachment {
	attachments := make([]apiModels.CommunicationAttachment, 0, len(raw))
	for _, item := range raw {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if attachment := communicationAttachmentFromData(itemMap); attachment != nil {
			attachments = append(attachments, *attachment)
		}
	}

	return attachments
}

func communicationAttachmentsFromMapSlice(raw []map[string]interface{}) []apiModels.CommunicationAttachment {
	attachments := make([]apiModels.CommunicationAttachment, 0, len(raw))
	for _, itemMap := range raw {
		if attachment := communicationAttachmentFromData(itemMap); attachment != nil {
			attachments = append(attachments, *attachment)
		}
	}

	return attachments
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
