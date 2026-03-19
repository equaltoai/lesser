package services

import (
	"context"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const notificationPostSnapshotKey = "postSnapshot"

// notificationService implements NotificationService
type notificationService struct {
	deps    *ServiceDependencies
	storage StorageAdapter
	logger  *zap.Logger
}

// NewNotificationService creates a new notification service
func NewNotificationService(deps *ServiceDependencies) NotificationService {
	return &notificationService{
		deps:    deps,
		storage: CreateStorageAdapter(deps.Repos),
		logger:  deps.Logger.(*zap.Logger),
	}
}

// CreateFollowNotification creates a notification when someone follows a user
func (n *notificationService) CreateFollowNotification(ctx context.Context, followActivity *activitypub.Activity) error {
	// Extract target user from follow activity
	targetActorID, ok := followActivity.Object.(string)
	if !ok {
		n.logger.Warn("invalid follow activity: object is not string")
		return nil
	}

	// Extract follower actor ID
	followerActorID := followActivity.Actor

	// Create follow notification
	return n.createNotification(ctx, targetActorID, followerActorID, "follow", followActivity.ID, nil)
}

// CreateLikeNotification creates a notification when someone likes a post
func (n *notificationService) CreateLikeNotification(ctx context.Context, likeActivity *activitypub.Activity) error {
	// Get the object being liked
	objectID, ok := likeActivity.Object.(string)
	if !ok {
		n.logger.Warn("invalid like activity: object is not string")
		return nil
	}

	// Get the object to find its author
	object, err := n.storage.GetObject(ctx, objectID)
	if err != nil {
		n.logger.Warn("failed to get object for like notification", zap.Error(err))
		return nil
	}

	// Extract author actor ID
	authorActorID := n.extractAttributedTo(object)
	if err := common.ValidateRequiredParam("authorActorID", authorActorID); err != nil {
		n.logger.Warn("no attributed author found for liked object")
		return nil
	}

	// Extract liker actor ID
	likerActorID := likeActivity.Actor

	// Don't notify if user liked their own post
	if authorActorID == likerActorID {
		return nil
	}

	// Create like notification
	return n.createNotification(ctx, authorActorID, likerActorID, "favourite", likeActivity.ID, n.buildPostSnapshotFromObject(object, objectID, ""))
}

// CreateReplyNotification creates a notification when someone replies to a post
func (n *notificationService) CreateReplyNotification(ctx context.Context, replyActivity *activitypub.Activity) error {
	// Extract reply note
	note, ok := replyActivity.Object.(*activitypub.Note)
	if !ok {
		n.logger.Warn("invalid reply activity: object is not Note")
		return nil
	}

	// Check if it's actually a reply
	if err := common.ValidateRequiredParam("note.InReplyTo", note.InReplyTo); err != nil {
		return nil
	}

	// Get the parent object to find its author
	parentObject, err := n.storage.GetObject(ctx, note.InReplyTo)
	if err != nil {
		n.logger.Warn("failed to get parent object for reply notification", zap.Error(err))
		return nil
	}

	// Extract parent author actor ID
	parentAuthorActorID := n.extractAttributedTo(parentObject)
	if err := common.ValidateRequiredParam("parentAuthorActorID", parentAuthorActorID); err != nil {
		n.logger.Warn("no attributed author found for parent object")
		return nil
	}

	// Extract replier actor ID
	replierActorID := replyActivity.Actor

	// Don't notify if user replied to their own post
	if parentAuthorActorID == replierActorID {
		return nil
	}

	// Create reply notification
	return n.createNotification(ctx, parentAuthorActorID, replierActorID, "reply", replyActivity.ID, n.buildPostSnapshotFromObject(note, firstNonEmptyString(note.ID, replyActivity.ID), n.snapshotVisibilityFromActivity(replyActivity)))
}

// CreateMentionNotification creates notifications for mentioned users
func (n *notificationService) CreateMentionNotification(ctx context.Context, mentions []string, activity *activitypub.Activity) error {
	mentionerActorID := activity.Actor
	postSnapshot := n.postSnapshotForActivity(ctx, activity)

	for _, mentionedActorID := range mentions {
		// Don't notify if user mentioned themselves
		if mentionedActorID == mentionerActorID {
			continue
		}

		// Create mention notification
		if err := n.createNotification(ctx, mentionedActorID, mentionerActorID, "mention", activity.ID, postSnapshot); err != nil {
			n.logger.Warn("failed to create mention notification",
				zap.String("mentioned_actor", mentionedActorID),
				zap.Error(err))
		}
	}

	return nil
}

// Helper methods

func (n *notificationService) createNotification(ctx context.Context, recipientActorID, fromActorID, notificationType, activityID string, postSnapshot map[string]interface{}) error {
	// Extract username from actor ID for storage
	recipientUsername := n.extractUsernameFromActorID(recipientActorID)
	fromUsername := n.extractUsernameFromActorID(fromActorID)

	if common.ValidateRequiredParam("recipientUsername", recipientUsername) != nil || common.ValidateRequiredParam("fromUsername", fromUsername) != nil {
		n.logger.Warn("invalid actor ID format for notification",
			zap.String("recipient", recipientActorID),
			zap.String("from", fromActorID))
		return nil
	}

	// Create notification using the appropriate builder based on type
	var notification *models.Notification
	switch notificationType {
	case "mention":
		notification = models.NewMentionNotification(recipientUsername, fromUsername, activityID)
	case "reply":
		notification = models.NewReplyNotification(recipientUsername, fromUsername, activityID)
	case "follow":
		notification = models.NewFollowNotification(recipientUsername, fromUsername)
	case "favourite":
		notification = models.NewFavouriteNotification(recipientUsername, fromUsername, activityID)
	case "reblog":
		notification = models.NewReblogNotification(recipientUsername, fromUsername, activityID)
	default:
		// Use builder for custom types
		notification = models.NewNotificationBuilder().
			ForUser(recipientUsername).
			OfType(notificationType).
			FromActor(fromUsername, "user").
			Build()
	}

	if len(postSnapshot) > 0 {
		notification.SetData(notificationPostSnapshotKey, cloneNotificationPostSnapshot(postSnapshot))
	}

	// Create the notification
	if err := n.storage.CreateNotification(ctx, notification); err != nil {
		n.logger.Error("failed to create notification",
			zap.String("recipient", recipientUsername),
			zap.String("from", fromUsername),
			zap.String("type", notificationType),
			zap.Error(err))
		return err
	}

	n.logger.Info("notification created",
		zap.String("recipient", recipientUsername),
		zap.String("from", fromUsername),
		zap.String("type", notificationType))
	return nil
}

func (n *notificationService) extractAttributedTo(object interface{}) string {
	switch obj := object.(type) {
	case *activitypub.Note:
		return obj.AttributedTo
	case map[string]interface{}:
		if attr, ok := obj["attributedTo"].(string); ok {
			return attr
		}
	}
	return ""
}

func (n *notificationService) extractUsernameFromActorID(actorID string) string {
	// Parse actor ID to extract username
	// Format: https://domain/users/username or https://domain/@username
	if strings.Contains(actorID, "/users/") {
		parts := strings.Split(actorID, "/users/")
		if len(parts) == 2 {
			return parts[1]
		}
	} else if strings.Contains(actorID, "/@") {
		parts := strings.Split(actorID, "/@")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}

func (n *notificationService) postSnapshotForActivity(ctx context.Context, activity *activitypub.Activity) map[string]interface{} {
	if activity == nil {
		return nil
	}

	fallbackVisibility := n.snapshotVisibilityFromActivity(activity)

	switch object := activity.Object.(type) {
	case nil:
		return nil
	case string:
		storedObject, err := n.storage.GetObject(ctx, object)
		if err != nil {
			n.logger.Warn("failed to load activity object for notification snapshot",
				zap.String("object_id", object),
				zap.Error(err))
			return nil
		}
		return n.buildPostSnapshotFromObject(storedObject, object, fallbackVisibility)
	default:
		return n.buildPostSnapshotFromObject(object, activity.ID, fallbackVisibility)
	}
}

func (n *notificationService) buildPostSnapshotFromObject(object interface{}, fallbackID, fallbackVisibility string) map[string]interface{} {
	switch obj := object.(type) {
	case *activitypub.Note:
		return buildNotificationPostSnapshot(
			firstNonEmptyString(obj.ID, fallbackID),
			firstNonEmptyString(obj.ID, fallbackID),
			obj.Content,
			timestampString(obj.Published),
			n.resolveSnapshotVisibility(obj.Visibility, obj.To, obj.CC, obj.BTo, obj.BCC, fallbackVisibility),
			obj.InReplyTo,
			obj.AttributedTo,
		)
	case *activitypub.QuoteNote:
		return n.buildPostSnapshotFromObject(&obj.Note, fallbackID, fallbackVisibility)
	case *activitypub.Article:
		return n.buildPostSnapshotFromObject(&obj.Note, fallbackID, fallbackVisibility)
	case *models.Object:
		return buildNotificationPostSnapshot(
			firstNonEmptyString(obj.ID, fallbackID),
			firstNonEmptyString(obj.URL, obj.ID, fallbackID),
			obj.Content,
			timestampString(&obj.Published),
			n.resolveSnapshotVisibility(obj.Visibility, obj.To, obj.CC, obj.BTo, obj.BCC, fallbackVisibility),
			derefString(obj.InReplyTo),
			obj.AttributedTo,
		)
	case map[string]interface{}:
		return n.buildPostSnapshotFromMap(obj, fallbackID, fallbackVisibility)
	default:
		return nil
	}
}

func (n *notificationService) buildPostSnapshotFromMap(object map[string]interface{}, fallbackID, fallbackVisibility string) map[string]interface{} {
	id := stringValueFromMap(object, "id")
	url := stringValueFromMap(object, "url")
	content := stringValueFromMap(object, "content")
	createdAt := firstNonEmptyString(timestampString(anyTimePointerFromMap(object, "createdAt")), timestampString(anyTimePointerFromMap(object, "published")))
	visibility := n.resolveSnapshotVisibility(
		stringValueFromMap(object, "visibility"),
		stringSliceFromAny(object["to"]),
		stringSliceFromAny(object["cc"]),
		stringSliceFromAny(object["bto"]),
		stringSliceFromAny(object["bcc"]),
		fallbackVisibility,
	)
	inReplyTo := firstNonEmptyString(stringValueFromMap(object, "inReplyTo"), stringValueFromMap(object, "inReplyToId"))
	attributedTo := stringValueFromMap(object, "attributedTo")

	return buildNotificationPostSnapshot(
		firstNonEmptyString(id, fallbackID),
		firstNonEmptyString(url, id, fallbackID),
		content,
		createdAt,
		visibility,
		inReplyTo,
		attributedTo,
	)
}

func (n *notificationService) snapshotVisibilityFromActivity(activity *activitypub.Activity) string {
	if activity == nil {
		return ""
	}

	visibility := activitypub.NewAddressingValidator().GetVisibilityLevel(activity)
	if visibility == unknownValue {
		return ""
	}

	return visibility
}

func (n *notificationService) resolveSnapshotVisibility(explicit string, to, cc, bto, bcc []string, fallback string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}

	derived := activitypub.NewAddressingValidator().GetVisibilityLevel(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			To:  to,
			CC:  cc,
			BTo: bto,
			BCC: bcc,
		},
	})
	if derived != "" && derived != unknownValue {
		return derived
	}

	if trimmed := strings.TrimSpace(fallback); trimmed != "" {
		return trimmed
	}

	return models.VisibilityPublic
}

func buildNotificationPostSnapshot(id, url, content, createdAt, visibility, inReplyToID, attributedTo string) map[string]interface{} {
	if strings.TrimSpace(id) == "" {
		return nil
	}

	snapshot := map[string]interface{}{
		"id":         id,
		"url":        firstNonEmptyString(url, id),
		"content":    content,
		"createdAt":  createdAt,
		"visibility": firstNonEmptyString(visibility, models.VisibilityPublic),
	}

	if trimmed := strings.TrimSpace(inReplyToID); trimmed != "" {
		snapshot["inReplyToId"] = trimmed
	}
	if trimmed := strings.TrimSpace(attributedTo); trimmed != "" {
		snapshot["attributedTo"] = trimmed
	}

	return snapshot
}

func cloneNotificationPostSnapshot(snapshot map[string]interface{}) map[string]interface{} {
	if len(snapshot) == 0 {
		return nil
	}

	cloned := make(map[string]interface{}, len(snapshot))
	for key, value := range snapshot {
		cloned[key] = value
	}

	return cloned
}

func stringValueFromMap(values map[string]interface{}, key string) string {
	raw, ok := values[key]
	if !ok {
		return ""
	}

	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func anyTimePointerFromMap(values map[string]interface{}, key string) *time.Time {
	raw, ok := values[key]
	if !ok {
		return nil
	}

	switch value := raw.(type) {
	case time.Time:
		return &value
	case *time.Time:
		return value
	case string:
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return &parsed
		}
	}

	return nil
}

func stringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				out = append(out, strings.TrimSpace(str))
			}
		}
		return out
	default:
		return nil
	}
}

func timestampString(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}

	return value.UTC().Format(time.RFC3339)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return ""
}
