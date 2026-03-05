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
	commNotificationMaxTypeLen        = 64
	commNotificationMaxChannelLen     = 16
	commNotificationMaxFromAddressLen = 320
	commNotificationMaxToAddressLen   = 320
	commNotificationMaxDisplayNameLen = 200
	commNotificationMaxSubjectLen     = 500
	commNotificationMaxBodyLen        = 25_000
	commNotificationMaxMessageIDLen   = 200
	commNotificationMaxInReplyToLen   = 200
	commNotificationMaxSoulAgentIDLen = 128
	commNotificationMaxAttachments    = 20

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

	FromAddress     string
	FromSoulAgentID string
	FromDisplayName string
	ToAddress       string

	Subject string
	Body    string

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

	typ, err := common.ValidateAndSanitizeString("type", req.Type, 1, commNotificationMaxTypeLen)
	if err != nil {
		return nil, apperrors.ValidationFailed("type", err.Error())
	}
	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ != commNotificationTypeInbound {
		return nil, apperrors.ValidationFailed("type", "unsupported notification type")
	}

	channel, err := common.ValidateAndSanitizeString("channel", req.Channel, 1, commNotificationMaxChannelLen)
	if err != nil {
		return nil, apperrors.ValidationFailed("channel", err.Error())
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	switch channel {
	case commNotificationChannelEmail, commNotificationChannelSMS, commNotificationChannelVoice:
	default:
		return nil, apperrors.ValidationFailed("channel", "unsupported channel")
	}

	fromAddress, err := common.ValidateAndSanitizeString("from.address", req.From.Address, 1, commNotificationMaxFromAddressLen)
	if err != nil {
		return nil, apperrors.ValidationFailed("from.address", err.Error())
	}

	toAddress := ""
	if req.To != nil {
		normalized, err := common.ValidateAndSanitizeString("to.address", req.To.Address, 1, commNotificationMaxToAddressLen)
		if err != nil {
			return nil, apperrors.ValidationFailed("to.address", err.Error())
		}
		toAddress = normalized
	} else if channel == commNotificationChannelEmail {
		return nil, apperrors.ValidationFailed("to.address", "email recipient is required")
	}

	displayName, err := common.ValidateAndSanitizeString("from.displayName", req.From.DisplayName, 0, commNotificationMaxDisplayNameLen)
	if err != nil {
		return nil, apperrors.ValidationFailed("from.displayName", err.Error())
	}
	displayName = common.EscapeHTML(displayName)

	soulAgentID := ""
	if req.From.SoulAgentID != nil {
		normalized, err := common.ValidateAndSanitizeString("from.soulAgentId", *req.From.SoulAgentID, 0, commNotificationMaxSoulAgentIDLen)
		if err != nil {
			return nil, apperrors.ValidationFailed("from.soulAgentId", err.Error())
		}
		soulAgentID = normalized
	}

	subject, err := common.ValidateAndSanitizeString("subject", req.Subject, 0, commNotificationMaxSubjectLen)
	if err != nil {
		return nil, apperrors.ValidationFailed("subject", err.Error())
	}
	subject = common.EscapeHTML(subject)
	if channel == commNotificationChannelEmail && strings.TrimSpace(subject) == "" {
		return nil, apperrors.ValidationFailed("subject", "email subject is required")
	}

	body, err := common.ValidateAndSanitizeString("body", req.Body, 1, commNotificationMaxBodyLen)
	if err != nil {
		return nil, apperrors.ValidationFailed("body", err.Error())
	}
	body = common.EscapeHTML(body)
	if strings.TrimSpace(body) == "" {
		return nil, apperrors.ValidationFailed("body", "body cannot be blank")
	}

	receivedAtRaw, err := common.ValidateAndSanitizeString("receivedAt", req.ReceivedAt, 1, 64)
	if err != nil {
		return nil, apperrors.ValidationFailed("receivedAt", err.Error())
	}
	receivedAt, err := time.Parse(time.RFC3339Nano, receivedAtRaw)
	if err != nil {
		return nil, apperrors.ValidationFailed("receivedAt", "must be RFC3339 timestamp")
	}

	messageID, err := common.ValidateAndSanitizeString("messageId", req.MessageID, 1, commNotificationMaxMessageIDLen)
	if err != nil {
		return nil, apperrors.ValidationFailed("messageId", err.Error())
	}

	inReplyTo := ""
	if req.InReplyTo != nil {
		normalized, err := common.ValidateAndSanitizeString("inReplyTo", *req.InReplyTo, 0, commNotificationMaxInReplyToLen)
		if err != nil {
			return nil, apperrors.ValidationFailed("inReplyTo", err.Error())
		}
		inReplyTo = normalized
	}

	attachments := make([]commNotificationAttachment, 0, len(req.Attachments))
	metadataBytes := 0
	if len(req.Attachments) > commNotificationMaxAttachments {
		return nil, apperrors.ValidationFailed("attachments", "too many attachments")
	}
	for idx, attachment := range req.Attachments {
		id, err := common.ValidateAndSanitizeString(fmt.Sprintf("attachments[%d].id", idx), attachment.ID, 1, commNotificationMaxAttachmentIDLen)
		if err != nil {
			return nil, apperrors.ValidationFailed(fmt.Sprintf("attachments[%d].id", idx), err.Error())
		}

		filename, err := common.ValidateAndSanitizeString(fmt.Sprintf("attachments[%d].filename", idx), attachment.Filename, 1, commNotificationMaxAttachmentFilenameLen)
		if err != nil {
			return nil, apperrors.ValidationFailed(fmt.Sprintf("attachments[%d].filename", idx), err.Error())
		}
		filename = common.EscapeHTML(filename)

		contentType, err := common.ValidateAndSanitizeString(fmt.Sprintf("attachments[%d].contentType", idx), attachment.ContentType, 1, commNotificationMaxAttachmentContentTypeLen)
		if err != nil {
			return nil, apperrors.ValidationFailed(fmt.Sprintf("attachments[%d].contentType", idx), err.Error())
		}

		if attachment.SizeBytes < 0 {
			return nil, apperrors.ValidationFailed(fmt.Sprintf("attachments[%d].sizeBytes", idx), "sizeBytes cannot be negative")
		}
		if attachment.SizeBytes > commNotificationMaxAttachmentSizeBytes {
			return nil, apperrors.ValidationFailed(fmt.Sprintf("attachments[%d].sizeBytes", idx), "sizeBytes is too large")
		}

		sha256sum, err := common.ValidateAndSanitizeString(fmt.Sprintf("attachments[%d].sha256", idx), attachment.SHA256, commNotificationAttachmentSHA256Len, commNotificationAttachmentSHA256Len)
		if err != nil {
			return nil, apperrors.ValidationFailed(fmt.Sprintf("attachments[%d].sha256", idx), err.Error())
		}
		if _, err := hex.DecodeString(sha256sum); err != nil {
			return nil, apperrors.ValidationFailed(fmt.Sprintf("attachments[%d].sha256", idx), "sha256 must be hex-encoded")
		}
		sha256sum = strings.ToLower(sha256sum)

		metadataBytes += len(id) + len(filename) + len(contentType) + len(sha256sum)
		if metadataBytes > commNotificationMaxAttachmentsMetadataBytes {
			return nil, apperrors.ValidationFailed("attachments", "attachments metadata is too large")
		}

		attachments = append(attachments, commNotificationAttachment{
			ID:          id,
			Filename:    filename,
			ContentType: contentType,
			SizeBytes:   attachment.SizeBytes,
			SHA256:      sha256sum,
		})
	}

	return &commNotificationDelivery{
		NotificationType: typ,
		Channel:          channel,
		FromAddress:      fromAddress,
		FromSoulAgentID:  soulAgentID,
		FromDisplayName:  displayName,
		ToAddress:        toAddress,
		Subject:          subject,
		Body:             body,
		ReceivedAt:       receivedAt.UTC(),
		MessageID:        messageID,
		InReplyTo:        inReplyTo,
		Attachments:      attachments,
	}, nil
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
