package handlers

import (
	"crypto/sha256"
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
	commNotificationMaxDisplayNameLen = 200
	commNotificationMaxSubjectLen     = 500
	commNotificationMaxBodyLen        = 25_000
	commNotificationMaxMessageIDLen   = 200
	commNotificationMaxInReplyToLen   = 200
	commNotificationMaxSoulAgentIDLen = 128
)

type commNotificationDelivery struct {
	NotificationType string
	Channel          string

	FromAddress     string
	FromSoulAgentID string
	FromDisplayName string

	Subject string
	Body    string

	ReceivedAt time.Time
	MessageID  string
	InReplyTo  string
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

	return &commNotificationDelivery{
		NotificationType: typ,
		Channel:          channel,
		FromAddress:      fromAddress,
		FromSoulAgentID:  soulAgentID,
		FromDisplayName:  displayName,
		Subject:          subject,
		Body:             body,
		ReceivedAt:       receivedAt.UTC(),
		MessageID:        messageID,
		InReplyTo:        inReplyTo,
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
