// Package validation contains inbox-specific ActivityPub validation helpers.
package validation

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// ValidateRequestBody enforces request body presence and size limits.
func ValidateRequestBody(logger *zap.Logger, body []byte) error {
	if err := common.ValidateSliceNotEmpty("body", body); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", "request body is required", 400)
	}

	if err := common.ValidateStringLength("request body", string(body), 0, common.MaxActivitySize); err != nil {
		if logger != nil {
			logger.Warn("request body too large", zap.Int("size", len(body)))
		}
		return lift.NewLiftError("PAYLOAD_TOO_LARGE", "request body too large", 413)
	}

	return nil
}

// ParseActivity parses and sanitizes an ActivityPub activity from the raw request body.
func ParseActivity(logger *zap.Logger, body []byte) (*activitypub.Activity, error) {
	if err := common.ValidateActivityPubJSON(string(body), "activity"); err != nil {
		if logger != nil {
			logger.Warn("invalid JSON format", zap.Error(err))
		}
		return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid JSON: %v", err), 400)
	}

	var activity activitypub.Activity
	if err := common.ParseActivityPubObject(body, &activity); err != nil {
		if logger != nil {
			logger.Warn("failed to parse activity", zap.Error(err))
		}
		return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid activity: %v", err), 400)
	}

	if err := common.ValidateActivityPubURL(activity.ID, "id"); err != nil {
		if logger != nil {
			logger.Warn("invalid activity ID URL", zap.String("id", activity.ID), zap.Error(err))
		}
		return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid activity ID: %v", err), 400)
	}

	if activity.Published != nil && !activity.Published.IsZero() {
		published := activity.Published.Format(time.RFC3339)
		if err := common.ValidateActivityPubTimestamp(published, "published"); err != nil {
			if logger != nil {
				logger.Warn("invalid activity timestamp", zap.String("published", published), zap.Error(err))
			}
			return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid timestamp: %v", err), 400)
		}
	}

	if objMap, ok := activity.Object.(map[string]any); ok {
		common.SanitizeActivityPubObjectDefault(objMap)
		activity.Object = objMap
	}

	if logger != nil {
		logger.Info("processing activity",
			zap.String("type", activity.Type),
			zap.String("actor", activity.Actor),
			zap.String("id", activity.ID))
	}

	return &activity, nil
}

// ValidateActivity validates required fields and addressing for the activity and target actor.
func ValidateActivity(logger *zap.Logger, activity *activitypub.Activity, actor *activitypub.Actor) error {
	if err := ValidateBasicActivity(activity); err != nil {
		return err
	}

	if err := ValidateBasicActor(actor); err != nil {
		return err
	}

	if err := ValidateActivityAddressing(activity); err != nil {
		return err
	}

	if err := ValidateActorUsername(activity.Actor); err != nil {
		return err
	}

	if err := ValidateActorPublicKey(actor); err != nil {
		return err
	}

	if err := ValidateCreateActivityObject(activity); err != nil {
		return err
	}

	if err := ValidateComprehensiveAddressing(logger, activity); err != nil {
		return err
	}

	if err := ValidateActivityTargeting(logger, activity, actor); err != nil {
		return err
	}

	return nil
}

// ValidateBasicActivity validates the basic ActivityPub activity structure.
func ValidateBasicActivity(activity *activitypub.Activity) error {
	context := []interface{}(activitypub.Context)
	if len(activity.Context) > 0 {
		context = []interface{}(activity.Context)
	}

	activityMap := map[string]interface{}{
		"@context": context,
		"id":       activity.ID,
		"type":     activity.Type,
		"actor":    activity.Actor,
		"object":   activity.Object,
		"to":       stringSliceToInterfaceSlice(activity.To),
		"cc":       stringSliceToInterfaceSlice(activity.CC),
		"bto":      stringSliceToInterfaceSlice(activity.BTo),
		"bcc":      stringSliceToInterfaceSlice(activity.BCC),
	}
	if err := common.ValidateActivityPubActivity(activityMap); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid activity: %v", err), 400)
	}
	return nil
}

// ValidateBasicActor validates the basic ActivityPub actor structure.
func ValidateBasicActor(actor *activitypub.Actor) error {
	actorMap := map[string]interface{}{
		"@context":          []interface{}(activitypub.Context),
		"id":                actor.ID,
		"type":              actor.Type,
		"preferredUsername": actor.PreferredUsername,
		"inbox":             actor.Inbox,
		"outbox":            actor.Outbox,
	}
	if err := common.ValidateActivityPubActor(actorMap); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid actor: %v", err), 400)
	}
	return nil
}

// ValidateActivityAddressing validates all addressing fields in the activity.
func ValidateActivityAddressing(activity *activitypub.Activity) error {
	addressingFields := []struct {
		field interface{}
		name  string
	}{
		{stringSliceToInterfaceSlice(activity.To), "to"},
		{stringSliceToInterfaceSlice(activity.CC), "cc"},
		{stringSliceToInterfaceSlice(activity.BTo), "bto"},
		{stringSliceToInterfaceSlice(activity.BCC), "bcc"},
	}

	for _, addr := range addressingFields {
		if err := common.ValidateActivityPubAddressing(addr.field, addr.name); err != nil {
			return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid '%s' addressing: %v", addr.name, err), 400)
		}
	}
	return nil
}

// ValidateActorUsername validates the actor URL and username format.
func ValidateActorUsername(actorURL string) error {
	parsedURL, err := url.Parse(actorURL)
	if err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid actor username: %v", err), 400)
	}

	path := strings.Trim(parsedURL.Path, "/")
	parts := strings.Split(path, "/")
	username := parts[len(parts)-1]
	if err := common.ValidateActivityPubUsername(username); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid actor username: %v", err), 400)
	}
	return nil
}

// ValidateActorPublicKey validates the public key field when present.
func ValidateActorPublicKey(actor *activitypub.Actor) error {
	if actor.PublicKey == nil {
		return nil
	}

	publicKeyMap := map[string]interface{}{
		"id":           actor.PublicKey.ID,
		"owner":        actor.PublicKey.Owner,
		"publicKeyPem": actor.PublicKey.PublicKeyPem,
	}
	if err := common.ValidateActivityPubPublicKey(publicKeyMap); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid public key: %v", err), 400)
	}
	return nil
}

// ValidateCreateActivityObject validates the embedded object structure for Create activities.
func ValidateCreateActivityObject(activity *activitypub.Activity) error {
	if activity.Type != "Create" {
		return nil
	}

	objMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		return nil
	}

	if err := ValidateObjectAttachments(objMap); err != nil {
		return err
	}

	if err := ValidateObjectTags(objMap); err != nil {
		return err
	}

	if err := ValidateNoteObject(objMap); err != nil {
		return err
	}

	return nil
}

// ValidateObjectAttachments validates ActivityPub attachments on an embedded object.
func ValidateObjectAttachments(objMap map[string]interface{}) error {
	if attachments, exists := objMap["attachment"]; exists {
		if err := common.ValidateActivityPubAttachments(attachments, "attachment"); err != nil {
			return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid attachments: %v", err), 400)
		}
	}
	return nil
}

// ValidateObjectTags validates ActivityPub tags on an embedded object.
func ValidateObjectTags(objMap map[string]interface{}) error {
	if tags, exists := objMap["tag"]; exists {
		if err := common.ValidateActivityPubTags(tags, "tag"); err != nil {
			return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid tags: %v", err), 400)
		}
	}
	return nil
}

// ValidateNoteObject validates embedded Note objects when present.
func ValidateNoteObject(objMap map[string]interface{}) error {
	if objType, exists := objMap["type"]; exists && objType == "Note" {
		if err := common.ValidateActivityPubNote(objMap); err != nil {
			return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid note object: %v", err), 400)
		}
	}
	return nil
}

// ValidateComprehensiveAddressing validates addressing using the ActivityPub validator.
func ValidateComprehensiveAddressing(logger *zap.Logger, activity *activitypub.Activity) error {
	addressingValidator := activitypub.NewAddressingValidator()
	if err := addressingValidator.ValidateAddressing(activity); err != nil {
		if logger != nil {
			logger.Warn("invalid activity addressing", zap.Error(err))
		}
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid addressing: %v", err), 400)
	}
	return nil
}

// ValidateActivityTargeting ensures the activity is addressed to the target actor.
func ValidateActivityTargeting(logger *zap.Logger, activity *activitypub.Activity, actor *activitypub.Actor) error {
	if !IsAddressedTo(activity, actor) {
		if logger != nil {
			logger.Warn("activity not addressed to this actor",
				zap.String("actor_id", actor.ID),
				zap.Any("to", activity.To),
				zap.Any("cc", activity.CC))
		}
		return lift.NewLiftError("VALIDATION_ERROR", "activity is not addressed to this actor", 400)
	}
	return nil
}

// IsAddressedTo returns true if the activity addresses the actor or inbox.
func IsAddressedTo(activity *activitypub.Activity, actor *activitypub.Actor) bool {
	actorID := actor.ID
	inboxURL := actor.Inbox

	for _, to := range activity.To {
		if to == actorID || to == inboxURL || to == activitypub.PublicAddress {
			return true
		}
	}

	for _, cc := range activity.CC {
		if cc == actorID || cc == inboxURL || cc == activitypub.PublicAddress {
			return true
		}
	}

	for _, bto := range activity.BTo {
		if bto == actorID || bto == inboxURL {
			return true
		}
	}

	for _, bcc := range activity.BCC {
		if bcc == actorID || bcc == inboxURL {
			return true
		}
	}

	return false
}

func stringSliceToInterfaceSlice(values []string) []interface{} {
	result := make([]interface{}, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}
