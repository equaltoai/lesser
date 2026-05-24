package graph

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
)

func communicationNotificationFromData(
	notifType string,
	createdAt time.Time,
	data map[string]interface{},
) *model.CommunicationNotification {
	if !isCommunicationNotificationType(notifType) {
		return nil
	}

	messageID := extractCommunicationString(data, "messageId", "message_id")
	if messageID == "" {
		return nil
	}

	inReplyTo := extractCommunicationString(data, "inReplyTo", "in_reply_to")

	return &model.CommunicationNotification{
		Channel:     extractCommunicationString(data, "channel"),
		From:        communicationFromData(data),
		To:          communicationToData(data),
		Attachments: communicationAttachmentsFromData(data),
		Subject:     optionalString(extractCommunicationString(data, "subject", "title")),
		Body:        optionalString(extractCommunicationString(data, "body")),
		ReceivedAt:  model.Time(communicationReceivedAtFromData(createdAt, data)),
		MessageID:   messageID,
		InReplyTo:   optionalString(inReplyTo),
		ThreadID:    communicationThreadID(messageID, inReplyTo),
	}
}

func isCommunicationNotificationType(notifType string) bool {
	typ := strings.ToLower(strings.TrimSpace(notifType))
	return strings.HasPrefix(typ, "communication:")
}

func communicationNotificationActor(actorID string, data map[string]interface{}) *activitypub.Actor {
	from, _ := data["from"].(map[string]interface{})
	displayName := extractCommunicationString(from, "displayName", "display_name")
	for _, candidate := range []string{
		extractCommunicationString(from, "address"),
		actorID,
		extractCommunicationString(from, "soulAgentId", "soul_agent_id"),
	} {
		if actor := communicationEmailActorPlaceholder(candidate, displayName); actor != nil {
			return actor
		}
	}
	return nil
}

func communicationEmailActorPlaceholder(address string, displayName string) *activitypub.Actor {
	address = strings.TrimSpace(address)
	if !validCommunicationEmailActorAddress(address) || strings.Contains(address, "://") || strings.Count(address, "@") != 1 {
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

func validCommunicationEmailActorAddress(address string) bool {
	if address == "" || len(address) > 320 {
		return false
	}
	return strings.IndexFunc(address, func(r rune) bool {
		return unicode.IsControl(r) || r == '<' || r == '>' || r == '"' || r == '\''
	}) == -1
}

func communicationReceivedAtFromData(createdAt time.Time, data map[string]interface{}) time.Time {
	receivedAt := createdAt.UTC()
	raw := extractCommunicationString(data, "receivedAt", "received_at")
	if raw == "" {
		return receivedAt
	}

	if parsed, ok := parseCommunicationTime(raw); ok {
		return parsed
	}

	return receivedAt
}

func parseCommunicationTime(raw string) (time.Time, bool) {
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

func communicationFromData(data map[string]interface{}) *model.CommunicationFrom {
	raw, _ := data["from"].(map[string]interface{})

	return &model.CommunicationFrom{
		Address:     extractCommunicationString(raw, "address"),
		DisplayName: optionalString(extractCommunicationString(raw, "displayName", "display_name")),
		SoulAgentID: optionalString(extractCommunicationString(raw, "soulAgentId", "soul_agent_id")),
	}
}

func communicationToData(data map[string]interface{}) *model.CommunicationTo {
	raw, ok := data["to"].(map[string]interface{})
	if !ok {
		return nil
	}

	address := extractCommunicationString(raw, "address")
	if address == "" {
		return nil
	}

	return &model.CommunicationTo{Address: address}
}

func communicationAttachmentsFromData(data map[string]interface{}) []*model.CommunicationAttachment {
	if data == nil {
		return []*model.CommunicationAttachment{}
	}

	switch raw := data["attachments"].(type) {
	case []interface{}:
		return communicationAttachmentsFromInterfaceSlice(raw)
	case []map[string]interface{}:
		return communicationAttachmentsFromMapSlice(raw)
	default:
		return []*model.CommunicationAttachment{}
	}
}

func communicationAttachmentsFromInterfaceSlice(raw []interface{}) []*model.CommunicationAttachment {
	attachments := make([]*model.CommunicationAttachment, 0, len(raw))
	for _, item := range raw {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if attachment := communicationAttachmentFromData(itemMap); attachment != nil {
			attachments = append(attachments, attachment)
		}
	}

	return attachments
}

func communicationAttachmentsFromMapSlice(raw []map[string]interface{}) []*model.CommunicationAttachment {
	attachments := make([]*model.CommunicationAttachment, 0, len(raw))
	for _, itemMap := range raw {
		if attachment := communicationAttachmentFromData(itemMap); attachment != nil {
			attachments = append(attachments, attachment)
		}
	}

	return attachments
}

func extractCommunicationString(data map[string]interface{}, keys ...string) string {
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

func extractCommunicationInt64(data map[string]interface{}, keys ...string) int64 {
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

func communicationAttachmentFromData(data map[string]interface{}) *model.CommunicationAttachment {
	id := extractCommunicationString(data, "id")
	if id == "" {
		return nil
	}

	sizeBytes, ok := safeIntFromInt64(extractCommunicationInt64(data, "sizeBytes", "size_bytes"))
	if !ok {
		sizeBytes = 0
	}

	return &model.CommunicationAttachment{
		ID:          id,
		Filename:    extractCommunicationString(data, "filename"),
		ContentType: extractCommunicationString(data, "contentType", "content_type"),
		SizeBytes:   sizeBytes,
		Sha256:      extractCommunicationString(data, "sha256"),
	}
}
