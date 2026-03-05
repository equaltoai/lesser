package handlers

import (
	"crypto/subtle"
	"strings"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const commDeliveryAuditEventType = "comm.notification.deliver"

// HandleDeliverNotificationLift handles POST /api/v1/notifications/deliver.
//
// This endpoint is intended for internal, machine-to-machine delivery from lesser-host
// (v3 spec §6.3). It is authenticated with an instance-scoped bearer key.
func (h *Handler) HandleDeliverNotificationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || h.cfg == nil {
		return common.RespondServiceUnavailable(ctx, "notification delivery")
	}

	expectedKey := strings.TrimSpace(h.cfg.InstanceAPIKey)
	if expectedKey == "" {
		return common.RespondServiceUnavailable(ctx, "notification delivery")
	}

	authHeader := headerValue(ctx, "authorization")
	if authHeader == "" {
		authHeader = headerValue(ctx, "Authorization")
	}

	token, err := common.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondMissingAuth(ctx)
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(expectedKey)) != 1 {
		return common.RespondForbidden(ctx, "invalid instance api key")
	}

	recipient := strings.TrimSpace(h.cfg.AdminUsername)
	if recipient == "" {
		return common.RespondServiceUnavailable(ctx, "notification delivery")
	}

	var req apiModels.NotificationDeliveryRequest
	if resp, err := common.ParseRequestStrictWithValidation(ctx, &req); resp != nil || err != nil {
		return resp, err
	}

	delivery, err := normalizeCommNotificationDeliveryRequest(&req)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	notificationID, err := commNotificationID(recipient, delivery.MessageID)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return common.RespondServiceUnavailable(ctx, "notification")
	}

	from := map[string]interface{}{
		"address":     delivery.FromAddress,
		"displayName": delivery.FromDisplayName,
		"soulAgentId": nil,
	}
	if delivery.FromSoulAgentID != "" {
		from["soulAgentId"] = delivery.FromSoulAgentID
	}

	data := map[string]interface{}{
		"channel":    delivery.Channel,
		"from":       from,
		"receivedAt": delivery.ReceivedAt.Format(time.RFC3339Nano),
		"messageId":  delivery.MessageID,
	}
	if delivery.ToAddress != "" {
		data["to"] = map[string]interface{}{
			"address": delivery.ToAddress,
		}
	}
	if delivery.InReplyTo != "" {
		data["inReplyTo"] = delivery.InReplyTo
	}
	if len(delivery.Attachments) > 0 {
		attachments := make([]map[string]interface{}, 0, len(delivery.Attachments))
		for _, attachment := range delivery.Attachments {
			attachments = append(attachments, map[string]interface{}{
				"id":          attachment.ID,
				"filename":    attachment.Filename,
				"contentType": attachment.ContentType,
				"sizeBytes":   attachment.SizeBytes,
				"sha256":      attachment.SHA256,
			})
		}
		data["attachments"] = attachments
	}

	cmd := &notifications.CreateNotificationCommand{
		ID:        notificationID,
		CreatedAt: &delivery.ReceivedAt,
		UserID:    recipient,
		Type:      delivery.NotificationType,
		ActorID:   delivery.FromAddress,
		ActorType: "external",
		Title:     delivery.Subject,
		Body:      delivery.Body,
		Data:      data,
	}

	createResult, createErr := notificationService.CreateNotification(ctx.Context(), cmd)
	if createErr != nil && apperrors.HasCode(createErr, apperrors.CodeAlreadyExists) {
		// Idempotent delivery: repeated delivery of the same messageId maps to the same notification ID.
		createErr = nil
	}
	if createErr != nil {
		h.recordCommDeliveryAuditEvent(ctx, recipient, delivery, false, createErr.Error(), false)
		if apperrors.HasCode(createErr, apperrors.CodeValidationFailed) {
			return common.RespondValidationError(ctx, createErr)
		}
		return common.RespondInternalServerError(ctx, "failed to deliver notification")
	}

	idempotent := createResult == nil
	h.recordCommDeliveryAuditEvent(ctx, recipient, delivery, true, "", idempotent)

	return noContent(), nil
}

func (h *Handler) recordCommDeliveryAuditEvent(ctx *apptheory.Context, recipient string, delivery *commNotificationDelivery, success bool, failureReason string, idempotent bool) {
	if h == nil || h.repos == nil || h.repos.Audit() == nil || ctx == nil || delivery == nil {
		return
	}

	metadata := map[string]interface{}{
		"channel":     delivery.Channel,
		"message_id":  delivery.MessageID,
		"in_reply_to": delivery.InReplyTo,
		"received_at": delivery.ReceivedAt.Format(time.RFC3339Nano),
		"idempotent":  idempotent,
	}
	if delivery.FromAddress != "" {
		metadata["from_address"] = delivery.FromAddress
	}
	if delivery.FromSoulAgentID != "" {
		metadata["from_soul_agent_id"] = delivery.FromSoulAgentID
	}

	if err := h.repos.Audit().StoreAuditEvent(
		ctx.Context(),
		commDeliveryAuditEventType,
		"INFO",
		recipient,
		recipient,
		headerValue(ctx, "x-forwarded-for"),
		headerValue(ctx, "user-agent"),
		"",
		"",
		"",
		success,
		failureReason,
		metadata,
	); err != nil {
		h.logger.Debug("failed to store comm delivery audit event", zap.Error(err))
	}
}
