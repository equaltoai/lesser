package services

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

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
	return n.createNotification(ctx, targetActorID, followerActorID, "follow", followActivity.ID)
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
	if authorActorID == "" {
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
	return n.createNotification(ctx, authorActorID, likerActorID, "favourite", likeActivity.ID)
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
	if note.InReplyTo == "" {
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
	if parentAuthorActorID == "" {
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
	return n.createNotification(ctx, parentAuthorActorID, replierActorID, "mention", replyActivity.ID)
}

// CreateMentionNotification creates notifications for mentioned users
func (n *notificationService) CreateMentionNotification(ctx context.Context, mentions []string, activity *activitypub.Activity) error {
	mentionerActorID := activity.Actor

	for _, mentionedActorID := range mentions {
		// Don't notify if user mentioned themselves
		if mentionedActorID == mentionerActorID {
			continue
		}

		// Create mention notification
		if err := n.createNotification(ctx, mentionedActorID, mentionerActorID, "mention", activity.ID); err != nil {
			n.logger.Warn("failed to create mention notification",
				zap.String("mentioned_actor", mentionedActorID),
				zap.Error(err))
		}
	}

	return nil
}

// Helper methods

func (n *notificationService) createNotification(ctx context.Context, recipientActorID, fromActorID, notificationType, activityID string) error {
	// Extract username from actor ID for storage
	recipientUsername := n.extractUsernameFromActorID(recipientActorID)
	fromUsername := n.extractUsernameFromActorID(fromActorID)

	if recipientUsername == "" || fromUsername == "" {
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