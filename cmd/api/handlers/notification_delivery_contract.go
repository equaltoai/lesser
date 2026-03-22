package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
)

const (
	commNotificationTypeInbound = "communication:inbound"

	commNotificationChannelEmail = "email"
	commNotificationChannelSMS   = "sms"
	commNotificationChannelVoice = "voice"
)

const (
	commNotificationMaxTypeLen         = 64
	commNotificationMaxChannelLen      = 16
	commNotificationMaxFromAddressLen  = 320
	commNotificationMaxToAddressLen    = 320
	commNotificationMaxPhoneNumberLen  = 32
	commNotificationMaxDisplayNameLen  = 200
	commNotificationMaxSubjectLen      = 500
	commNotificationMaxBodyLen         = 25_000
	commNotificationMaxBodyMimeTypeLen = 128
	commNotificationMaxMessageIDLen    = 200
	commNotificationMaxInReplyToLen    = 200
	commNotificationMaxSoulAgentIDLen  = 128
	commNotificationMaxAttachments     = 20

	commNotificationMaxAttachmentIDLen          = 128
	commNotificationMaxAttachmentFilenameLen    = 255
	commNotificationMaxAttachmentContentTypeLen = 128
	commNotificationMaxAttachmentSizeBytes      = int64(1024 * 1024 * 1024) // 1GB
	commNotificationAttachmentSHA256Len         = 64
	commNotificationMaxAttachmentsMetadataBytes = 8 * 1024
)

type commNotificationDelivery struct {
	NotificationType string
	Channel          string

	FromIdentifier  string
	FromAddress     string
	FromNumber      string
	FromSoulAgentID string
	FromDisplayName string
	ToIdentifier    string
	ToAddress       string
	ToNumber        string

	Subject      string
	Body         string
	BodyMimeType string

	ReceivedAt time.Time
	MessageID  string
	InReplyTo  string

	Attachments []commNotificationAttachment
}

type commNotificationAttachment struct {
	ID          string
	Filename    string
	ContentType string
	SizeBytes   int64
	SHA256      string
}

func normalizeCommNotificationDeliveryRequest(req *apiModels.NotificationDeliveryRequest) (*commNotificationDelivery, error) {
	if req == nil {
		return nil, apperrors.ValidationFailed("body", "missing request body")
	}

	typ, err := normalizeCommNotificationType(req.Type)
	if err != nil {
		return nil, err
	}

	channel, err := normalizeCommNotificationChannel(req.Channel)
	if err != nil {
		return nil, err
	}

	fromIdentifier, fromAddress, fromNumber, displayName, soulAgentID, err := normalizeCommNotificationSender(channel, req.From)
	if err != nil {
		return nil, err
	}

	toIdentifier, toAddress, toNumber, err := normalizeCommNotificationRecipient(channel, req.To)
	if err != nil {
		return nil, err
	}

	subject, body, err := normalizeCommNotificationContent(channel, req.Subject, req.Body)
	if err != nil {
		return nil, err
	}

	bodyMimeType, err := normalizeCommNotificationBodyMimeType(req.BodyMimeType)
	if err != nil {
		return nil, err
	}

	receivedAt, err := normalizeCommNotificationReceivedAt(req.ReceivedAt)
	if err != nil {
		return nil, err
	}

	messageID, err := normalizeCommNotificationMessageID(req.MessageID)
	if err != nil {
		return nil, err
	}

	inReplyTo, err := normalizeCommNotificationInReplyTo(req.InReplyTo)
	if err != nil {
		return nil, err
	}

	attachments, err := normalizeCommNotificationAttachments(req.Attachments)
	if err != nil {
		return nil, err
	}

	return &commNotificationDelivery{
		NotificationType: typ,
		Channel:          channel,
		FromIdentifier:   fromIdentifier,
		FromAddress:      fromAddress,
		FromNumber:       fromNumber,
		FromSoulAgentID:  soulAgentID,
		FromDisplayName:  displayName,
		ToIdentifier:     toIdentifier,
		ToAddress:        toAddress,
		ToNumber:         toNumber,
		Subject:          subject,
		Body:             body,
		BodyMimeType:     bodyMimeType,
		ReceivedAt:       receivedAt.UTC(),
		MessageID:        messageID,
		InReplyTo:        inReplyTo,
		Attachments:      attachments,
	}, nil
}

func normalizeCommNotificationType(raw string) (string, error) {
	typ, err := common.ValidateAndSanitizeString("type", raw, 1, commNotificationMaxTypeLen)
	if err != nil {
		return "", apperrors.ValidationFailed("type", err.Error())
	}

	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ != commNotificationTypeInbound {
		return "", apperrors.ValidationFailed("type", "unsupported notification type")
	}

	return typ, nil
}

func normalizeCommNotificationChannel(raw string) (string, error) {
	channel, err := common.ValidateAndSanitizeString("channel", raw, 1, commNotificationMaxChannelLen)
	if err != nil {
		return "", apperrors.ValidationFailed("channel", err.Error())
	}

	channel = strings.ToLower(strings.TrimSpace(channel))
	switch channel {
	case commNotificationChannelEmail, commNotificationChannelSMS, commNotificationChannelVoice:
		return channel, nil
	default:
		return "", apperrors.ValidationFailed("channel", "unsupported channel")
	}
}

func normalizeCommNotificationSender(channel string, from apiModels.NotificationDeliveryFrom) (string, string, string, string, string, error) {
	fromAddress, err := normalizeCommNotificationOptionalAddress("from.address", from.Address, commNotificationMaxFromAddressLen)
	if err != nil {
		return "", "", "", "", "", err
	}

	fromNumber, err := normalizeCommNotificationOptionalAddress("from.number", from.Number, commNotificationMaxPhoneNumberLen)
	if err != nil {
		return "", "", "", "", "", err
	}

	displayName, err := common.ValidateAndSanitizeString("from.displayName", from.DisplayName, 0, commNotificationMaxDisplayNameLen)
	if err != nil {
		return "", "", "", "", "", apperrors.ValidationFailed("from.displayName", err.Error())
	}

	soulAgentID := ""
	if from.SoulAgentID != nil {
		soulAgentID, err = common.ValidateAndSanitizeString("from.soulAgentId", *from.SoulAgentID, 0, commNotificationMaxSoulAgentIDLen)
		if err != nil {
			return "", "", "", "", "", apperrors.ValidationFailed("from.soulAgentId", err.Error())
		}
	}

	identifier, field, err := normalizeCommNotificationIdentity(channel, "from", fromAddress, fromNumber)
	if err != nil {
		return "", "", "", "", "", apperrors.ValidationFailed(field, err.Error())
	}

	return identifier, fromAddress, fromNumber, common.EscapeHTML(displayName), soulAgentID, nil
}

func normalizeCommNotificationRecipient(channel string, to *apiModels.NotificationDeliveryTo) (string, string, string, error) {
	if to == nil {
		if channel == commNotificationChannelEmail {
			return "", "", "", apperrors.ValidationFailed("to.address", "email recipient is required")
		}
		return "", "", "", nil
	}

	address, err := normalizeCommNotificationOptionalAddress("to.address", to.Address, commNotificationMaxToAddressLen)
	if err != nil {
		return "", "", "", err
	}

	number, err := normalizeCommNotificationOptionalAddress("to.number", to.Number, commNotificationMaxPhoneNumberLen)
	if err != nil {
		return "", "", "", err
	}

	identifier, field, identityErr := normalizeCommNotificationIdentity(channel, "to", address, number)
	if identityErr != nil {
		return "", "", "", apperrors.ValidationFailed(field, identityErr.Error())
	}

	return identifier, address, number, nil
}

func normalizeCommNotificationContent(channel, rawSubject, rawBody string) (string, string, error) {
	subject, err := common.ValidateAndSanitizeString("subject", rawSubject, 0, commNotificationMaxSubjectLen)
	if err != nil {
		return "", "", apperrors.ValidationFailed("subject", err.Error())
	}

	subject = common.EscapeHTML(subject)
	if channel == commNotificationChannelEmail && strings.TrimSpace(subject) == "" {
		return "", "", apperrors.ValidationFailed("subject", "email subject is required")
	}

	body, err := common.ValidateAndSanitizeString("body", rawBody, 1, commNotificationMaxBodyLen)
	if err != nil {
		return "", "", apperrors.ValidationFailed("body", err.Error())
	}

	body = common.EscapeHTML(body)
	if strings.TrimSpace(body) == "" {
		return "", "", apperrors.ValidationFailed("body", "body cannot be blank")
	}

	return subject, body, nil
}

func normalizeCommNotificationBodyMimeType(raw string) (string, error) {
	bodyMimeType, err := common.ValidateAndSanitizeString("bodyMimeType", raw, 0, commNotificationMaxBodyMimeTypeLen)
	if err != nil {
		return "", apperrors.ValidationFailed("bodyMimeType", err.Error())
	}

	return strings.ToLower(strings.TrimSpace(bodyMimeType)), nil
}

func normalizeCommNotificationReceivedAt(raw string) (time.Time, error) {
	receivedAtRaw, err := common.ValidateAndSanitizeString("receivedAt", raw, 1, 64)
	if err != nil {
		return time.Time{}, apperrors.ValidationFailed("receivedAt", err.Error())
	}

	receivedAt, err := time.Parse(time.RFC3339Nano, receivedAtRaw)
	if err != nil {
		return time.Time{}, apperrors.ValidationFailed("receivedAt", "must be RFC3339 timestamp")
	}

	return receivedAt.UTC(), nil
}

func normalizeCommNotificationMessageID(raw string) (string, error) {
	messageID, err := common.ValidateAndSanitizeString("messageId", raw, 1, commNotificationMaxMessageIDLen)
	if err != nil {
		return "", apperrors.ValidationFailed("messageId", err.Error())
	}

	return messageID, nil
}

func normalizeCommNotificationInReplyTo(raw *string) (string, error) {
	if raw == nil {
		return "", nil
	}

	inReplyTo, err := common.ValidateAndSanitizeString("inReplyTo", *raw, 0, commNotificationMaxInReplyToLen)
	if err != nil {
		return "", apperrors.ValidationFailed("inReplyTo", err.Error())
	}

	return inReplyTo, nil
}

func normalizeCommNotificationAttachments(raw []apiModels.NotificationDeliveryAttachment) ([]commNotificationAttachment, error) {
	if len(raw) > commNotificationMaxAttachments {
		return nil, apperrors.ValidationFailed("attachments", "too many attachments")
	}

	attachments := make([]commNotificationAttachment, 0, len(raw))
	metadataBytes := 0
	for idx, attachment := range raw {
		normalized, err := normalizeCommNotificationAttachment(idx, attachment)
		if err != nil {
			return nil, err
		}

		metadataBytes += len(normalized.ID) + len(normalized.Filename) + len(normalized.ContentType) + len(normalized.SHA256)
		if metadataBytes > commNotificationMaxAttachmentsMetadataBytes {
			return nil, apperrors.ValidationFailed("attachments", "attachments metadata is too large")
		}

		attachments = append(attachments, normalized)
	}

	return attachments, nil
}

func normalizeCommNotificationOptionalAddress(field, raw string, maxLen int) (string, error) {
	value, err := common.ValidateAndSanitizeString(field, raw, 0, maxLen)
	if err != nil {
		return "", apperrors.ValidationFailed(field, err.Error())
	}

	return value, nil
}

func normalizeCommNotificationIdentity(channel, prefix, address, number string) (string, string, error) {
	switch channel {
	case commNotificationChannelEmail:
		if address == "" {
			return "", prefix + ".address", fmt.Errorf("email %s is required", identitySubject(prefix))
		}
		return address, "", nil
	case commNotificationChannelSMS, commNotificationChannelVoice:
		if number != "" {
			return number, "", nil
		}
		if address != "" {
			return address, "", nil
		}
		return "", prefix + ".number", fmt.Errorf("%s address or number is required", identitySubject(prefix))
	default:
		if address != "" {
			return address, "", nil
		}
		if number != "" {
			return number, "", nil
		}
		return "", prefix + ".address", fmt.Errorf("%s address is required", identitySubject(prefix))
	}
}

func identitySubject(prefix string) string {
	if prefix == "from" {
		return "sender"
	}
	return "recipient"
}

func normalizeCommNotificationAttachment(idx int, attachment apiModels.NotificationDeliveryAttachment) (commNotificationAttachment, error) {
	fieldName := func(suffix string) string {
		return fmt.Sprintf("attachments[%d].%s", idx, suffix)
	}

	id, err := common.ValidateAndSanitizeString(fieldName("id"), attachment.ID, 1, commNotificationMaxAttachmentIDLen)
	if err != nil {
		return commNotificationAttachment{}, apperrors.ValidationFailed(fieldName("id"), err.Error())
	}

	filename, err := common.ValidateAndSanitizeString(fieldName("filename"), attachment.Filename, 1, commNotificationMaxAttachmentFilenameLen)
	if err != nil {
		return commNotificationAttachment{}, apperrors.ValidationFailed(fieldName("filename"), err.Error())
	}

	contentType, err := common.ValidateAndSanitizeString(fieldName("contentType"), attachment.ContentType, 1, commNotificationMaxAttachmentContentTypeLen)
	if err != nil {
		return commNotificationAttachment{}, apperrors.ValidationFailed(fieldName("contentType"), err.Error())
	}

	if attachment.SizeBytes < 0 {
		return commNotificationAttachment{}, apperrors.ValidationFailed(fieldName("sizeBytes"), "sizeBytes cannot be negative")
	}
	if attachment.SizeBytes > commNotificationMaxAttachmentSizeBytes {
		return commNotificationAttachment{}, apperrors.ValidationFailed(fieldName("sizeBytes"), "sizeBytes is too large")
	}

	sha256sum, err := normalizeCommNotificationAttachmentSHA256(fieldName("sha256"), attachment.SHA256)
	if err != nil {
		return commNotificationAttachment{}, err
	}

	return commNotificationAttachment{
		ID:          id,
		Filename:    common.EscapeHTML(filename),
		ContentType: contentType,
		SizeBytes:   attachment.SizeBytes,
		SHA256:      sha256sum,
	}, nil
}

func normalizeCommNotificationAttachmentSHA256(fieldName, raw string) (string, error) {
	sha256sum, err := common.ValidateAndSanitizeString(fieldName, raw, commNotificationAttachmentSHA256Len, commNotificationAttachmentSHA256Len)
	if err != nil {
		return "", apperrors.ValidationFailed(fieldName, err.Error())
	}

	if _, err := hex.DecodeString(sha256sum); err != nil {
		return "", apperrors.ValidationFailed(fieldName, "sha256 must be hex-encoded")
	}

	return strings.ToLower(sha256sum), nil
}

// commNotificationID deterministically maps (userID, messageID) to a stable instance notification ID.
// This is the instance-side idempotency strategy for comm-worker deliveries (v3 roadmap L-M0/L-M1).
func commNotificationID(userID, messageID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", apperrors.ValidationFailed("user_id", "user id is required")
	}

	messageID, err := common.ValidateAndSanitizeString("messageId", messageID, 1, commNotificationMaxMessageIDLen)
	if err != nil {
		return "", apperrors.ValidationFailed("messageId", err.Error())
	}

	sum := sha256.Sum256([]byte("comm:" + userID + ":" + messageID))
	return fmt.Sprintf("comm-%x", sum[:16]), nil
}
