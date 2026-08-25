package handlers

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

const commDeliveryAuditEventType = "comm.notification.deliver"
const commNotificationRecipientResolutionFailure = "notification recipient could not be resolved"

// ResolveNotificationDeliveryPrincipal verifies the instance-scoped credential
// used by the internal notification-delivery route. It is intentionally separate
// from the handler's defense-in-depth check so SecureApp can classify the route as
// InternalOnly without treating ordinary OAuth principals as internal callers.
func (h *Handler) ResolveNotificationDeliveryPrincipal(ctx *apptheory.Context) (*apptheory.SecurePrincipal, error) {
	if h == nil || h.cfg == nil || ctx == nil {
		return nil, nil
	}
	expectedKeys, err := h.notificationDeliveryKeys(ctx.Context())
	if err != nil || len(expectedKeys) == 0 {
		return nil, err
	}
	token, err := common.ExtractBearerToken(ctx.Header("Authorization"))
	if err != nil || !matchesNotificationDeliveryKey(token, expectedKeys) {
		return nil, nil
	}
	return &apptheory.SecurePrincipal{
		Identity: "lesser-host-notification-delivery",
		Kind:     apptheory.PrincipalInternal,
	}, nil
}

// HandleDeliverNotificationLift handles POST /api/v1/notifications/deliver.
//
// This endpoint is intended for internal, machine-to-machine delivery from lesser-host
// (v3 spec §6.3). It is authenticated with an instance-scoped bearer key.
func (h *Handler) HandleDeliverNotificationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || h.cfg == nil {
		return common.RespondServiceUnavailable(ctx, "notification delivery")
	}

	expectedKeys, err := h.notificationDeliveryKeys(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to resolve notification delivery keys", zap.Error(err))
		return common.RespondServiceUnavailable(ctx, "notification delivery")
	}
	if len(expectedKeys) == 0 {
		return common.RespondServiceUnavailable(ctx, "notification delivery")
	}

	authHeader := ctx.Header("Authorization")

	token, err := common.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondMissingAuth(ctx)
	}
	if !matchesNotificationDeliveryKey(token, expectedKeys) {
		return common.RespondForbidden(ctx, "invalid instance api key")
	}

	req, err := apptheory.BindRequest[apiModels.NotificationDeliveryRequest](ctx, apptheory.BindConfig[apiModels.NotificationDeliveryRequest]{
		Body:       true,
		StrictJSON: true,
	})
	if err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	delivery, err := normalizeCommNotificationDeliveryRequest(&req)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	recipient, addressedRecipient := h.resolveNotificationDeliveryRecipient(ctx.Context(), delivery)
	if recipient == "" {
		if addressedRecipient {
			h.logger.Warn("notification recipient resolution failed",
				zap.String("to_identifier", delivery.ToIdentifier),
				zap.String("to_address", delivery.ToAddress),
				zap.String("to_number", delivery.ToNumber),
				zap.String("channel", delivery.Channel))
			h.recordCommDeliveryAuditEvent(ctx, "", delivery, false, commNotificationRecipientResolutionFailure, false)
			return common.RespondUnprocessableEntity(ctx, commNotificationRecipientResolutionFailure)
		}
		return common.RespondServiceUnavailable(ctx, "notification delivery")
	}

	notificationID, err := commNotificationID(recipient, delivery.MessageID)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return common.RespondServiceUnavailable(ctx, "notification")
	}

	data := commNotificationData(delivery)

	cmd := &notifications.CreateNotificationCommand{
		ID:        notificationID,
		CreatedAt: &delivery.ReceivedAt,
		UserID:    recipient,
		Type:      delivery.NotificationType,
		ActorID:   delivery.FromIdentifier,
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
		h.logger.Error("failed to create notification",
			zap.String("user_id", cmd.UserID),
			zap.String("type", cmd.Type),
			zap.Error(createErr))
		h.recordCommDeliveryAuditEvent(ctx, recipient, delivery, false, createErr.Error(), false)
		if apperrors.HasCode(createErr, apperrors.CodeValidationFailed) {
			return common.RespondValidationError(ctx, createErr)
		}
		return common.RespondInternalServerError(ctx, "failed to deliver notification")
	}

	idempotent := createResult == nil
	h.recordCommDeliveryAuditEvent(ctx, recipient, delivery, true, "", idempotent)

	return apptheory.NoContent(), nil
}

func commNotificationData(delivery *commNotificationDelivery) map[string]interface{} {
	data := map[string]interface{}{
		"channel":    delivery.Channel,
		"from":       commNotificationParty(delivery.FromAddress, delivery.FromNumber, delivery.FromDisplayName, delivery.FromSoulAgentID),
		"receivedAt": delivery.ReceivedAt.Format(time.RFC3339Nano),
		"messageId":  delivery.MessageID,
	}
	if to := commNotificationParty(delivery.ToAddress, delivery.ToNumber, "", ""); len(to) > 0 {
		data["to"] = to
	}
	if delivery.InReplyTo != "" {
		data["inReplyTo"] = delivery.InReplyTo
	}
	if delivery.BodyMimeType != "" {
		data["bodyMimeType"] = delivery.BodyMimeType
	}
	if attachments := commNotificationAttachmentsData(delivery.Attachments); len(attachments) > 0 {
		data["attachments"] = attachments
	}
	return data
}

func commNotificationParty(address, number, displayName, soulAgentID string) map[string]interface{} {
	party := map[string]interface{}{}
	if address != "" {
		party["address"] = address
	}
	if number != "" {
		party["number"] = number
	}
	if displayName != "" {
		party["displayName"] = displayName
	}
	if soulAgentID != "" {
		party["soulAgentId"] = soulAgentID
	}
	return party
}

func commNotificationAttachmentsData(attachments []commNotificationAttachment) []map[string]interface{} {
	if len(attachments) == 0 {
		return nil
	}

	data := make([]map[string]interface{}, 0, len(attachments))
	for _, attachment := range attachments {
		data = append(data, map[string]interface{}{
			"id":          attachment.ID,
			"filename":    attachment.Filename,
			"contentType": attachment.ContentType,
			"sizeBytes":   attachment.SizeBytes,
			"sha256":      attachment.SHA256,
		})
	}

	return data
}

func (h *Handler) resolveNotificationDeliveryRecipient(ctx context.Context, delivery *commNotificationDelivery) (string, bool) {
	if h.notificationDeliveryHasExplicitRecipient(delivery) {
		username := h.notificationDeliveryAddressedRecipient(ctx, delivery)
		if username == "" {
			return "", true
		}
		return h.notificationDeliveryResolvedUsername(ctx, username), true
	}
	return h.notificationDeliveryResolvedUsername(ctx, h.notificationDeliveryRecipient(ctx)), false
}

func (h *Handler) notificationDeliveryHasExplicitRecipient(delivery *commNotificationDelivery) bool {
	if delivery == nil {
		return false
	}
	return strings.TrimSpace(delivery.ToIdentifier) != "" ||
		strings.TrimSpace(delivery.ToAddress) != "" ||
		strings.TrimSpace(delivery.ToNumber) != ""
}

func (h *Handler) notificationDeliveryAddressedRecipient(ctx context.Context, delivery *commNotificationDelivery) string {
	if delivery == nil {
		return ""
	}

	if username := h.notificationDeliveryBoundAgentIdentifier(ctx, delivery.ToSoulAgentID); username != "" {
		return username
	}

	if username := h.notificationDeliveryBoundContactIdentifier(ctx, delivery.ToAddress); username != "" {
		return username
	}

	if username := h.notificationDeliveryBoundContactIdentifier(ctx, delivery.ToNumber); username != "" {
		return username
	}

	return ""
}

func (h *Handler) notificationDeliveryRecipient(ctx context.Context) string {
	if h == nil || h.cfg == nil {
		return ""
	}

	if recipient := strings.TrimSpace(h.cfg.AdminUsername); recipient != "" {
		return recipient
	}

	if h.repos == nil || h.repos.Instance() == nil {
		return ""
	}

	state, err := h.repos.Instance().GetInstanceState(ctx)
	if err != nil || state == nil {
		return ""
	}

	return strings.TrimSpace(state.PrimaryAdminUsername)
}

func (h *Handler) notificationDeliveryResolvedUsername(ctx context.Context, username string) string {
	username = strings.TrimSpace(username)
	if username == "" || h == nil || h.repos == nil || h.repos.Account() == nil {
		return username
	}

	account, err := h.repos.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil {
		return username
	}

	resolvedUsername := strings.TrimSpace(account.User.Username)
	if resolvedUsername == "" {
		return username
	}

	return resolvedUsername
}

func (h *Handler) notificationDeliveryBoundUsername(ctx context.Context, username string) string {
	if h == nil || h.repos == nil || h.repos.Instance() == nil {
		return ""
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}

	binding, err := h.repos.Instance().GetSoulBodyBindingByUsername(ctx, username)
	if err != nil || binding == nil {
		return ""
	}

	return strings.TrimSpace(binding.Username)
}

func (h *Handler) notificationDeliveryBoundAgentIdentifier(ctx context.Context, agentID string) string {
	if h == nil || h.repos == nil || h.repos.Instance() == nil {
		return ""
	}

	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}

	binding, err := h.repos.Instance().GetSoulBodyBinding(ctx, agentID)
	if err != nil || binding == nil {
		return ""
	}

	return strings.TrimSpace(binding.Username)
}

func (h *Handler) notificationDeliveryBoundContactIdentifier(ctx context.Context, identifier string) string {
	// Project 37 instance-scoped email addresses are opaque contact identifiers:
	// <agent-local-id>.<instance-slug>@lessersoul.ai is not a Lesser username.
	// Host must carry to.soulAgentId for these deliveries so Lesser can resolve
	// through InstanceSoulBodyBinding authority instead of local-part parsing.
	if notificationDeliveryRequiresAuthoritativeRecipient(identifier) {
		return ""
	}

	username := h.notificationDeliveryUsernameForContactIdentifier(ctx, identifier)
	if username == "" {
		return ""
	}

	if bindingUsername := h.notificationDeliveryBoundUsername(ctx, username); bindingUsername != "" {
		return bindingUsername
	}

	return username
}

func notificationDeliveryRequiresAuthoritativeRecipient(identifier string) bool {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	parts := strings.Split(identifier, "@")
	if len(parts) != 2 {
		return false
	}

	localPart := strings.TrimSpace(parts[0])
	domain := strings.Trim(strings.TrimSpace(parts[1]), ".")
	if localPart == "" || domain == "" {
		return false
	}
	if domain != "lessersoul.ai" && !strings.HasSuffix(domain, ".lessersoul.ai") {
		return false
	}

	return strings.Contains(localPart, ".")
}

func (h *Handler) notificationDeliveryUsernameForContactIdentifier(ctx context.Context, identifier string) string {
	if h == nil || h.repos == nil || h.repos.Account() == nil {
		return ""
	}

	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ""
	}

	if !strings.Contains(identifier, "@") {
		return ""
	}

	actor, err := h.repos.Account().SearchByWebfinger(ctx, identifier)
	if err != nil || actor == nil {
		return ""
	}
	if !h.actorAppearsLocal(actor) {
		return ""
	}
	return strings.TrimSpace(actor.PreferredUsername)
}

func (h *Handler) notificationDeliveryKeys(ctx context.Context) ([]string, error) {
	if h == nil || h.cfg == nil {
		return nil, nil
	}

	keys := []string{}
	var errs []error

	instanceAPIKey, err := h.cfg.ResolveInstanceAPIKey()
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to resolve instance api key for notification delivery", zap.Error(err))
		}
		errs = append(errs, fmt.Errorf("resolve instance api key: %w", err))
	} else {
		keys = appendUniqueNotificationDeliveryKey(keys, instanceAPIKey)
	}

	lesserHostKey, err := h.effectiveLesserHostInstanceKey(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to resolve lesser-host instance key for notification delivery", zap.Error(err))
		}
		errs = append(errs, fmt.Errorf("resolve lesser-host instance key: %w", err))
	} else {
		keys = appendUniqueNotificationDeliveryKey(keys, lesserHostKey)
	}

	if len(keys) > 0 {
		return keys, nil
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return nil, nil
}

func appendUniqueNotificationDeliveryKey(keys []string, raw string) []string {
	key := strings.TrimSpace(raw)
	if key == "" {
		return keys
	}
	for _, existing := range keys {
		if subtle.ConstantTimeCompare([]byte(existing), []byte(key)) == 1 {
			return keys
		}
	}
	return append(keys, key)
}

func matchesNotificationDeliveryKey(token string, expectedKeys []string) bool {
	for _, expectedKey := range expectedKeys {
		if subtle.ConstantTimeCompare([]byte(token), []byte(expectedKey)) == 1 {
			return true
		}
	}
	return false
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
