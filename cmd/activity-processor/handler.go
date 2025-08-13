// Package main implements the activity processor handler that processes
// DynamoDB stream events for ActivityPub activities.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// ActivityDirection represents the direction of an activity (inbox or outbox)
type ActivityDirection string

const (
	// InboxDirection indicates an activity received in the inbox
	InboxDirection ActivityDirection = "inbox"
	// OutboxDirection indicates an activity sent from the outbox
	OutboxDirection ActivityDirection = "outbox"

	// ActivityTypeFollow represents a Follow activity type
	ActivityTypeFollow = "Follow"
	// ActivityTypeLike represents a Like activity type
	ActivityTypeLike = "Like"
	// ActivityTypeAnnounce represents an Announce activity type
	ActivityTypeAnnounce = "Announce"
	// ActivityTypeBlock represents a Block activity type
	ActivityTypeBlock = "Block"

	// ObjectTypeNote represents a Note object type
	ObjectTypeNote = "Note"
	// ObjectTypeObject represents a generic Object type
	ObjectTypeObject = "Object"

	// VisibilityPublic represents a public visibility level
	VisibilityPublic = "public"
	// VisibilityDirect represents a direct message visibility level
	VisibilityDirect = "direct"
	
	// DefaultTestingDomain is the default domain used for testing
	DefaultTestingDomain = "example.com"
)

// ActivityHandler processes DynamoDB stream events for activities
type ActivityHandler struct {
	DB               core.DB
	TableName        string
	Logger           *zap.Logger
	ObjectRepo       *repositories.ObjectRepository
	ActorRepo        *repositories.ActorRepository
	TimelineRepo     *repositories.TimelineRepository
	RelationshipRepo *repositories.RelationshipRepository
	LikeRepo         *repositories.LikeRepository
	SocialRepo       *repositories.SocialRepository
}

// NewActivityHandler creates a new ActivityHandler
func NewActivityHandler(db core.DB, tableName string) *ActivityHandler {
	logger := zap.L()
	domain := os.Getenv("DOMAIN_NAME")
	if domain == "" {
		domain = DefaultTestingDomain // Default for testing
	}
	return &ActivityHandler{
		DB:               db,
		TableName:        tableName,
		Logger:           logger,
		ObjectRepo:       repositories.NewObjectRepository(db, tableName, domain, logger),
		ActorRepo:        repositories.NewActorRepository(db, tableName, logger),
		TimelineRepo:     repositories.NewTimelineRepository(db, tableName, logger),
		RelationshipRepo: repositories.NewRelationshipRepository(db, tableName, logger),
		LikeRepo:         repositories.NewLikeRepository(db, tableName, logger),
		SocialRepo:       repositories.NewSocialRepository(db, logger),
	}
}

// processRecord overrides the BaseHandler's processRecord method
//
//nolint:unused // Method reserved for BaseHandler interface compatibility
func (h *ActivityHandler) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Only process INSERT events for activities
	if record.EventName != "INSERT" {
		return nil
	}

	// Extract entity type from PK
	entityType, err := stream.GetEventType(record)
	if err != nil {
		return fmt.Errorf("failed to get entity type: %w", err)
	}

	// Only process activity records
	if entityType != "activity" {
		return nil
	}

	// Unmarshal the activity record
	var activityRecord ActivityRecord
	if err := stream.UnmarshalItem(record, &activityRecord); err != nil {
		return fmt.Errorf("failed to unmarshal activity record: %w", err)
	}

	// Parse the activity
	activity, err := activitypub.ParseActivity([]byte(activityRecord.Activity))
	if err != nil {
		return fmt.Errorf("failed to parse activity: %w", err)
	}

	// Determine direction (inbox or outbox)
	direction := InboxDirection
	if strings.Contains(activityRecord.SK, "outbox") {
		direction = OutboxDirection
	}

	// Process the activity based on direction
	switch direction {
	case InboxDirection:
		return h.processInboxActivity(ctx, activity, activityRecord.Username)
	case OutboxDirection:
		return h.processOutboxActivity(ctx, activity, activityRecord.Username)
	default:
		return fmt.Errorf("unknown activity direction: %s", direction)
	}
}

// processInboxActivity processes an incoming activity
//
//nolint:unused // False positive - called in Handle method
func (h *ActivityHandler) processInboxActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing inbox activity",
		zap.String("type", activity.Type),
		zap.String("username", username),
		zap.String("id", activity.ID),
	)

	// Process based on activity type
	switch activity.Type {
	case ActivityTypeFollow:
		return h.processFollowActivity(ctx, activity, username)
	case "Accept":
		return h.processAcceptActivity(ctx, activity, username)
	case "Reject":
		return h.processRejectActivity(ctx, activity, username)
	case "Create":
		return h.processCreateActivity(ctx, activity, username)
	case "Update":
		return h.processUpdateActivity(ctx, activity, username)
	case "Delete":
		return h.processDeleteActivity(ctx, activity, username)
	case ActivityTypeLike:
		return h.processLikeActivity(ctx, activity, username)
	case ActivityTypeAnnounce:
		return h.processAnnounceActivity(ctx, activity, username)
	case "Undo":
		return h.processUndoActivity(ctx, activity, username)
	case ActivityTypeBlock:
		return h.processBlockActivity(ctx, activity, username)
	case "Flag":
		return h.processFlagActivity(ctx, activity, username)
	case "Move":
		return h.processMoveActivity(ctx, activity, username)
	case "Add":
		return h.processAddActivity(ctx, activity, username)
	case "Remove":
		return h.processRemoveActivity(ctx, activity, username)
	default:
		h.Logger.Info("Ignoring unsupported activity type",
			zap.String("type", activity.Type),
		)
		return nil
	}
}

// processOutboxActivity processes an outgoing activity
//
//nolint:unused // False positive - called in Handle method
func (h *ActivityHandler) processOutboxActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing outbox activity",
		zap.String("type", activity.Type),
		zap.String("username", username),
		zap.String("id", activity.ID),
	)

	// Process based on activity type
	switch activity.Type {
	case "Create":
		return h.deliverActivity(ctx, activity, username)
	case "Follow":
		return h.deliverActivity(ctx, activity, username)
	case "Accept":
		return h.deliverActivity(ctx, activity, username)
	case "Reject":
		return h.deliverActivity(ctx, activity, username)
	case "Update":
		return h.deliverActivity(ctx, activity, username)
	case "Delete":
		return h.deliverActivity(ctx, activity, username)
	case "Like":
		return h.deliverActivity(ctx, activity, username)
	case "Announce":
		return h.deliverActivity(ctx, activity, username)
	case "Undo":
		return h.deliverActivity(ctx, activity, username)
	case "Block":
		return h.deliverActivity(ctx, activity, username)
	case "Flag":
		return h.deliverActivity(ctx, activity, username)
	case "Move":
		return h.deliverActivity(ctx, activity, username)
	case "Add":
		return h.deliverActivity(ctx, activity, username)
	case "Remove":
		return h.deliverActivity(ctx, activity, username)
	default:
		h.Logger.Info("Ignoring unsupported activity type",
			zap.String("type", activity.Type),
		)
		return nil
	}
}

// processFollowActivity processes a Follow activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processFollowActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Follow activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	// Extract the target user being followed
	var targetUser string
	switch obj := activity.Object.(type) {
	case string:
		targetUser = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			targetUser = id
		} else {
			h.Logger.Error("Follow activity object missing id field",
				zap.String("activity_id", activity.ID))
			return fmt.Errorf("follow activity object missing id field")
		}
	default:
		h.Logger.Error("Follow activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("follow activity has invalid object type")
	}

	if targetUser == "" {
		h.Logger.Error("Follow activity missing target user",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("follow activity missing target user")
	}

	// Extract username from actor URI
	followerUsername := h.extractUsernameFromActorURI(activity.Actor)
	targetUsername := h.extractUsernameFromActorURI(targetUser)

	if followerUsername == "" || targetUsername == "" {
		h.Logger.Error("Failed to extract usernames from Follow activity",
			zap.String("actor", activity.Actor),
			zap.String("target", targetUser))
		return fmt.Errorf("failed to extract usernames from Follow activity")
	}

	// Create the follow relationship
	if err := h.RelationshipRepo.CreateRelationship(ctx, followerUsername, targetUsername, activity.ID); err != nil {
		h.Logger.Error("Failed to create follow relationship",
			zap.String("follower", followerUsername),
			zap.String("following", targetUsername),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create follow relationship: %w", err)
	}

	// Create follow notification for the target user
	notification := models.NewNotificationBuilder().
		ForUser(targetUsername).
		OfType("follow").
		FromActor(followerUsername, "remote_actor").
		WithContent(
			fmt.Sprintf("%s started following you", followerUsername),
			fmt.Sprintf("You have a new follower: %s", followerUsername)).
		Build()

	if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
		h.Logger.Error("Failed to create follow notification",
			zap.String("target_user", targetUsername),
			zap.String("follower", followerUsername),
			zap.Error(err))
		// Don't return error - the relationship was created successfully
	}

	h.Logger.Info("Successfully processed Follow activity",
		zap.String("follower", followerUsername),
		zap.String("following", targetUsername),
		zap.String("activity_id", activity.ID))

	return nil
}

// processAcceptActivity processes an Accept activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processAcceptActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Accept activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	// Extract the activity being accepted (typically a Follow activity)
	var followActivity interface{}
	switch obj := activity.Object.(type) {
	case string:
		// Object is just an ID - we'll need to find the original activity
		h.Logger.Debug("Accept activity references activity by ID",
			zap.String("follow_activity_id", obj))
		// For now, we'll extract info from the Accept activity itself
		followActivity = obj
	case map[string]interface{}:
		followActivity = obj
	default:
		h.Logger.Error("Accept activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("accept activity has invalid object type")
	}

	// Extract the actors involved
	// The actor of the Accept is who is accepting (followee)
	// We need to find who initiated the follow (follower)
	accepter := h.extractUsernameFromActorURI(activity.Actor)

	var follower string
	switch followObj := followActivity.(type) {
	case string:
		// If it's just an ID, we can't easily extract the follower
		// We'd need to look up the original activity or infer from context
		h.Logger.Debug("Cannot extract follower from activity ID, using context username",
			zap.String("activity_id", followObj),
			zap.String("context_username", username))
		follower = username // The user context might be the follower
	case map[string]interface{}:
		if actor, ok := followObj["actor"].(string); ok {
			follower = h.extractUsernameFromActorURI(actor)
		}
	}

	if follower == "" || accepter == "" {
		h.Logger.Error("Failed to extract usernames from Accept activity",
			zap.String("accepter", accepter),
			zap.String("follower", follower))
		return fmt.Errorf("failed to extract usernames from Accept activity")
	}

	// Update the relationship status to accepted
	updates := map[string]interface{}{
		"State": "accepted",
	}
	
	if err := h.RelationshipRepo.UpdateRelationship(ctx, follower, accepter, updates); err != nil {
		h.Logger.Error("Failed to update relationship status to accepted",
			zap.String("follower", follower),
			zap.String("accepter", accepter),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to update relationship status: %w", err)
	}

	// Create notification for the follower that their follow request was accepted
	notification := models.NewNotificationBuilder().
		ForUser(follower).
		OfType("follow_request").
		FromActor(accepter, "remote_actor").
		WithContent(
			fmt.Sprintf("%s accepted your follow request", accepter),
			fmt.Sprintf("You are now following %s", accepter)).
		Build()

	if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
		h.Logger.Error("Failed to create accept notification",
			zap.String("follower", follower),
			zap.String("accepter", accepter),
			zap.Error(err))
		// Don't return error - the relationship was updated successfully
	}

	h.Logger.Info("Successfully processed Accept activity",
		zap.String("follower", follower),
		zap.String("accepter", accepter),
		zap.String("activity_id", activity.ID))

	return nil
}

// processCreateActivity processes a Create activity with visibility controls
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processCreateActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Create activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
		zap.String("actor", activity.Actor),
	)

	// Extract the object from the activity
	note, err := h.extractNoteFromActivity(activity)
	if err != nil {
		h.Logger.Error("failed to extract Note from Create activity",
			zap.Error(err),
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("failed to extract Note: %w", err)
	}

	// Create addressing validator for visibility processing
	addressingValidator := activitypub.NewAddressingValidator()

	// Determine visibility level
	visibility := addressingValidator.GetVisibilityLevel(activity)
	h.Logger.Debug("determined activity visibility",
		zap.String("visibility", visibility),
		zap.String("activity_id", activity.ID))

	// Create status model from the Note
	status, err := h.createStatusFromNote(note, activity, visibility)
	if err != nil {
		h.Logger.Error("failed to create status from Note",
			zap.Error(err),
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("failed to create status: %w", err)
	}

	// Store the status/object
	if err := h.ObjectRepo.CreateObject(ctx, note); err != nil {
		h.Logger.Error("failed to store Note object",
			zap.Error(err),
			zap.String("status_id", status.StatusID))
		return fmt.Errorf("failed to store object: %w", err)
	}

	// Add to appropriate timelines based on visibility
	if err := h.processStatusForTimelines(ctx, status, visibility, username); err != nil {
		h.Logger.Error("failed to process status for timelines",
			zap.Error(err),
			zap.String("status_id", status.StatusID))
		// Don't return error here - status is stored, timeline addition can be retried
	}

	h.Logger.Info("successfully processed Create activity",
		zap.String("status_id", status.StatusID),
		zap.String("visibility", visibility))

	return nil
}


// mapToNote converts a map to a Note object
//
//nolint:unused // Kept for future implementation of enhanced note mapping
func (h *ActivityHandler) mapToNote(objMap map[string]interface{}) (*activitypub.Note, error) {
	note := &activitypub.Note{}
	
	// Extract basic fields
	if id, ok := objMap["id"].(string); ok {
		note.ID = id
	}
	if noteType, ok := objMap["type"].(string); ok {
		note.Type = noteType
	}
	if content, ok := objMap["content"].(string); ok {
		note.Content = content
	}
	if attributedTo, ok := objMap["attributedTo"].(string); ok {
		note.AttributedTo = attributedTo
	}
	if sensitive, ok := objMap["sensitive"].(bool); ok {
		note.Sensitive = sensitive
	}

	// Extract addressing fields
	if to, ok := objMap["to"].([]interface{}); ok {
		note.To = h.interfaceSliceToStringSlice(to)
	}
	if cc, ok := objMap["cc"].([]interface{}); ok {
		note.CC = h.interfaceSliceToStringSlice(cc)
	}
	if bto, ok := objMap["bto"].([]interface{}); ok {
		note.BTo = h.interfaceSliceToStringSlice(bto)
	}
	if bcc, ok := objMap["bcc"].([]interface{}); ok {
		note.BCC = h.interfaceSliceToStringSlice(bcc)
	}

	// Extract inReplyTo
	if inReplyTo, ok := objMap["inReplyTo"].(string); ok {
		note.InReplyTo = inReplyTo
	}

	return note, nil
}

// extractNoteFromActivity extracts a Note object from a Create activity
//
//nolint:unused // False positive - called from processCreateActivity
func (h *ActivityHandler) extractNoteFromActivity(activity *activitypub.Activity) (*activitypub.Note, error) {
	switch obj := activity.Object.(type) {
	case *activitypub.Note:
		return obj, nil
	case map[string]interface{}:
		// Convert map to Note
		return h.mapToNote(obj)
	default:
		return nil, fmt.Errorf("unsupported object type: %T", obj)
	}
}

// interfaceSliceToStringSlice converts []interface{} to []string
//
//nolint:unused // False positive - called from mapToNote
func (h *ActivityHandler) interfaceSliceToStringSlice(slice []interface{}) []string {
	result := make([]string, len(slice))
	for i, v := range slice {
		if str, ok := v.(string); ok {
			result[i] = str
		}
	}
	return result
}

// createStatusFromNote creates a Status model from a Note and activity
//
//nolint:unused // False positive - called from processCreateActivity
func (h *ActivityHandler) createStatusFromNote(note *activitypub.Note, _ *activitypub.Activity, visibility string) (*models.Status, error) {
	statusID := h.extractStatusID(note.ID)
	
	status := &models.Status{
		StatusID:      statusID,
		Note:          note,
		AuthorID:      note.AttributedTo,
		Visibility:    visibility,
		ToRecipients:  note.To,
		CcRecipients:  note.CC,
		BtoRecipients: note.BTo,
		BccRecipients: note.BCC,
		CreatedAt:     time.Now(),
		ModifiedAt:    time.Now(),
	}

	// Set published time from Note if available
	if note.Published != nil {
		status.PublishedAt = *note.Published
	} else {
		status.PublishedAt = time.Now()
	}

	return status, nil
}

// processStatusForTimelines adds status to appropriate timelines based on visibility
//
//nolint:unused // False positive - called from processCreateActivity
func (h *ActivityHandler) processStatusForTimelines(ctx context.Context, status *models.Status, visibility, targetUsername string) error {
	h.Logger.Info("processing status for timelines",
		zap.String("visibility", visibility),
		zap.String("target_username", targetUsername),
		zap.String("status_id", status.StatusID))

	timelineEntries := make([]*models.Timeline, 0)

	// Extract basic info from status
	postID := status.StatusID
	actorID := status.AuthorID
	createdAt := status.PublishedAt

	// Extract content preview (first 500 chars)
	var contentPreview string
	if status.Note != nil && status.Note.Content != "" {
		contentPreview = status.Note.Content
		if len(contentPreview) > 500 {
			contentPreview = contentPreview[:500] + "..."
		}
	}

	// Extract other metadata
	isReply := status.Note != nil && status.Note.InReplyTo != ""
	inReplyTo := ""
	if isReply && status.Note != nil {
		inReplyTo = status.Note.InReplyTo
	}

	// Process based on visibility
	switch visibility {
	case VisibilityPublic:
		h.Logger.Debug("Adding to public and local timelines", 
			zap.String("status_id", status.StatusID))

		// Add to federated public timeline
		federatedEntry := &models.Timeline{
			TimelineType: "PUBLIC",
			TimelineID:   "FEDERATED", 
			PostID:       postID,
			ActorID:      actorID,
			ActorHandle:  h.extractUsernameFromActorURI(actorID),
			Content:      contentPreview,
			ContentType:  "Note",
			Visibility:   visibility,
			IsReply:      isReply,
			InReplyTo:    inReplyTo,
			CreatedAt:    createdAt,
		}
		timelineEntries = append(timelineEntries, federatedEntry)

		// Add to local public timeline if it's a local actor
		if h.isLocalActor(actorID) {
			localEntry := &models.Timeline{
				TimelineType: "PUBLIC",
				TimelineID:   "LOCAL",
				PostID:       postID,
				ActorID:      actorID,
				ActorHandle:  h.extractUsernameFromActorURI(actorID),
				Content:      contentPreview,
				ContentType:  "Note",
				Visibility:   visibility,
				IsReply:      isReply,
				InReplyTo:    inReplyTo,
				CreatedAt:    createdAt,
			}
			timelineEntries = append(timelineEntries, localEntry)
		}

	case "unlisted":
		h.Logger.Debug("Status is unlisted, not adding to public timelines", 
			zap.String("status_id", status.StatusID))
		// Unlisted posts don't go to public timelines

	case "private":
		h.Logger.Debug("Status is private, would add to followers' home timelines", 
			zap.String("status_id", status.StatusID))
		// Private posts only go to followers' home timelines
		// TODO: Implement follower timeline distribution

	case VisibilityDirect:
		h.Logger.Debug("Status is direct, would add to conversations timeline", 
			zap.String("status_id", status.StatusID))
		// Direct messages go to conversations, not public timelines

	default:
		h.Logger.Warn("Unknown visibility level, treating as direct",
			zap.String("visibility", visibility),
			zap.String("status_id", status.StatusID))
		// Default to most restrictive (direct)
	}

	// Create timeline entries if any were added
	if len(timelineEntries) > 0 {
		if err := h.TimelineRepo.CreateTimelineEntries(ctx, timelineEntries); err != nil {
			h.Logger.Error("Failed to create timeline entries",
				zap.String("status_id", status.StatusID),
				zap.Int("entries_count", len(timelineEntries)),
				zap.Error(err))
			return fmt.Errorf("failed to create timeline entries: %w", err)
		}

		h.Logger.Info("Successfully added status to timelines",
			zap.String("status_id", status.StatusID),
			zap.String("visibility", visibility),
			zap.Int("timeline_entries", len(timelineEntries)))
	}

	return nil
}

// isLocalActor checks if an actor is from the local domain
//
//nolint:unused // Helper method for timeline processing
func (h *ActivityHandler) isLocalActor(actorID string) bool {
	domain := os.Getenv("DOMAIN_NAME")
	if domain == "" {
		domain = DefaultTestingDomain // Default for testing
	}
	return strings.Contains(actorID, domain)
}

// extractStatusID extracts status ID from a Note ID URL
//
//nolint:unused // False positive - called from createStatusFromNote
func (h *ActivityHandler) extractStatusID(noteID string) string {
	// Extract the last part of the URL as status ID
	parts := strings.Split(noteID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return noteID
}



// processRejectActivity processes a Reject activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processRejectActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Reject activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	// Extract the activity being rejected (typically a Follow activity)
	var followActivity interface{}
	switch obj := activity.Object.(type) {
	case string:
		h.Logger.Debug("Reject activity references activity by ID",
			zap.String("follow_activity_id", obj))
		followActivity = obj
	case map[string]interface{}:
		followActivity = obj
	default:
		h.Logger.Error("Reject activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("reject activity has invalid object type")
	}

	// Extract the actors involved
	// The actor of the Reject is who is rejecting (followee)
	// We need to find who initiated the follow (follower)
	rejecter := h.extractUsernameFromActorURI(activity.Actor)

	var follower string
	switch followObj := followActivity.(type) {
	case string:
		// If it's just an ID, infer follower from context
		h.Logger.Debug("Cannot extract follower from activity ID, using context username",
			zap.String("activity_id", followObj),
			zap.String("context_username", username))
		follower = username
	case map[string]interface{}:
		if actor, ok := followObj["actor"].(string); ok {
			follower = h.extractUsernameFromActorURI(actor)
		}
	}

	if follower == "" || rejecter == "" {
		h.Logger.Error("Failed to extract usernames from Reject activity",
			zap.String("rejecter", rejecter),
			zap.String("follower", follower))
		return fmt.Errorf("failed to extract usernames from Reject activity")
	}

	// Delete the follow relationship entirely (rejected follow requests should be removed)
	if err := h.RelationshipRepo.DeleteRelationship(ctx, follower, rejecter); err != nil {
		h.Logger.Error("Failed to delete rejected follow relationship",
			zap.String("follower", follower),
			zap.String("rejecter", rejecter),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to delete rejected relationship: %w", err)
	}

	// Optional: Create notification for the follower that their follow request was rejected
	// Some implementations might choose not to notify on rejection to avoid spam
	notification := models.NewNotificationBuilder().
		ForUser(follower).
		OfType("follow_request").
		FromActor(rejecter, "remote_actor").
		WithContent(
			fmt.Sprintf("%s declined your follow request", rejecter),
			fmt.Sprintf("Your follow request to %s was declined", rejecter)).
		Build()

	if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
		h.Logger.Error("Failed to create reject notification",
			zap.String("follower", follower),
			zap.String("rejecter", rejecter),
			zap.Error(err))
		// Don't return error - the relationship was deleted successfully
	}

	h.Logger.Info("Successfully processed Reject activity",
		zap.String("follower", follower),
		zap.String("rejecter", rejecter),
		zap.String("activity_id", activity.ID))

	return nil
}

// processUpdateActivity processes an Update activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processUpdateActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	h.Logger.Info("Processing Update activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
	)
	return nil
}

// processDeleteActivity processes a Delete activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processDeleteActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Delete activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	// Extract object ID from activity
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		} else {
			h.Logger.Error("Delete activity object missing id field",
				zap.String("activity_id", activity.ID))
			return fmt.Errorf("delete activity object missing id field")
		}
	default:
		h.Logger.Error("Delete activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("delete activity has invalid object type")
	}

	if objectID == "" {
		h.Logger.Error("Delete activity missing object ID",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("delete activity missing object ID")
	}

	// Verify actor authorization - can only delete their own objects
	existingObject, err := h.ObjectRepo.GetObject(ctx, objectID)
	if err != nil {
		// Object might already be deleted or not exist
		h.Logger.Info("Object not found for delete activity (may already be deleted)",
			zap.String("object_id", objectID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return nil // Not an error - idempotent operation
	}

	// Check if actor is authorized to delete this object
	if !h.isAuthorizedToDelete(existingObject, activity.Actor) {
		h.Logger.Warn("Actor not authorized to delete object",
			zap.String("actor", activity.Actor),
			zap.String("object_id", objectID),
			zap.String("object_attributed_to", h.getObjectAuthor(existingObject)))
		return fmt.Errorf("actor not authorized to delete object")
	}

	// Extract object type from the existingObject
	var objectType string
	switch obj := existingObject.(type) {
	case *models.Object:
		objectType = obj.Type
	case *models.Status:
		objectType = ObjectTypeNote
	case map[string]interface{}:
		if t, ok := obj["type"].(string); ok {
			objectType = t
		} else {
			objectType = ObjectTypeObject
		}
	default:
		objectType = "Object"
	}

	// Create tombstone
	tombstone := &models.Tombstone{
		ID:         objectID,
		FormerType: objectType,
		DeletedBy:  activity.Actor,
		Summary:    fmt.Sprintf("Object deleted by %s", activity.Actor),
		Deleted:    time.Now(),
	}

	// Create the tombstone (this will replace the original object)
	if err := h.DB.WithContext(ctx).Model(tombstone).Create(); err != nil {
		h.Logger.Error("Failed to create tombstone",
			zap.String("object_id", objectID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create tombstone: %w", err)
	}

	// Perform cascade deletion (remove from timelines, notifications, etc.)
	if err := h.performCascadeDeletion(ctx, objectID, activity.Actor); err != nil {
		h.Logger.Warn("Failed to perform complete cascade deletion",
			zap.String("object_id", objectID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		// Continue processing - the main deletion succeeded
	}

	h.Logger.Info("Successfully processed Delete activity",
		zap.String("object_id", objectID),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor))

	return nil
}

// isAuthorizedToDelete checks if the actor is authorized to delete the object
// Currently used in processDeleteActivity but kept for potential future use
//
//nolint:unused // Kept for future use
func (h *ActivityHandler) isAuthorizedToDelete(object interface{}, actorID string) bool {
	// Extract the author/attribution from the object
	author := h.getObjectAuthor(object)
	return author == actorID
}

// getObjectAuthor extracts the author/attributedTo field from an object
// Currently used in isAuthorizedToDelete but kept for potential future use
//
//nolint:unused // Kept for future use
func (h *ActivityHandler) getObjectAuthor(object interface{}) string {
	switch obj := object.(type) {
	case *models.Object:
		return obj.AttributedTo
	case map[string]interface{}:
		if attributedTo, ok := obj["attributedTo"].(string); ok {
			return attributedTo
		}
		if actor, ok := obj["actor"].(string); ok {
			return actor
		}
	}
	return ""
}

// performCascadeDeletion removes deleted object from timelines and related data
//
//nolint:unused // Kept for future implementation
func (h *ActivityHandler) performCascadeDeletion(_ context.Context, objectID, actorID string) error {
	h.Logger.Debug("Performing cascade deletion",
		zap.String("object_id", objectID),
		zap.String("actor_id", actorID))

	// TODO: Implement timeline removal methods when TimelineRepository supports them
	// The following operations should be implemented:
	// - Remove from public timeline
	// - Remove from local timeline  
	// - Remove from federated timeline
	// - Remove from followers' home timelines
	
	h.Logger.Info("Timeline removal not yet implemented",
		zap.String("object_id", objectID),
		zap.String("actor_id", actorID))

	return nil
}

// processLikeActivity processes a Like activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processLikeActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Like activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	// Extract the object being liked
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		} else {
			h.Logger.Error("Like activity object missing id field",
				zap.String("activity_id", activity.ID))
			return fmt.Errorf("like activity object missing id field")
		}
	default:
		h.Logger.Error("Like activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("like activity has invalid object type")
	}

	if objectID == "" {
		h.Logger.Error("Like activity missing object ID",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("like activity missing object ID")
	}

	// Extract the actor doing the liking
	actorURI := activity.Actor
	if actorURI == "" {
		h.Logger.Error("Like activity missing actor",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("like activity missing actor")
	}

	// Create the like record
	_, err := h.LikeRepo.CreateLike(ctx, actorURI, objectID)
	if err != nil {
		h.Logger.Error("Failed to create like record",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create like record: %w", err)
	}

	// Find the owner of the object being liked to send notification
	object, err := h.ObjectRepo.GetObject(ctx, objectID)
	if err != nil {
		h.Logger.Warn("Could not find object for like notification",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Don't fail the entire operation, the like was saved
	} else {
		// Extract object author for notification
		var objectAuthor string
		switch obj := object.(type) {
		case *models.Status:
			objectAuthor = obj.AuthorID
		case *models.Object:
			objectAuthor = obj.AttributedTo
		case map[string]interface{}:
			if author, ok := obj["attributedTo"].(string); ok {
				objectAuthor = author
			}
		}

		if objectAuthor != "" {
			authorUsername := h.extractUsernameFromActorURI(objectAuthor)
			likerUsername := h.extractUsernameFromActorURI(actorURI)

			if authorUsername != "" && likerUsername != "" {
				// Create favourite notification for the object author
				notification := models.NewNotificationBuilder().
					ForUser(authorUsername).
					OfType("favourite").
					FromActor(likerUsername, "remote_actor").
					AboutTarget(objectID, "status").
					WithContent(
						fmt.Sprintf("%s liked your post", likerUsername),
						fmt.Sprintf("Your post was liked by %s", likerUsername)).
					Build()

				if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
					h.Logger.Error("Failed to create favourite notification",
						zap.String("author", authorUsername),
						zap.String("liker", likerUsername),
						zap.String("object_id", objectID),
						zap.Error(err))
					// Don't return error - the like was created successfully
				}
			}
		}
	}

	h.Logger.Info("Successfully processed Like activity",
		zap.String("actor", actorURI),
		zap.String("object_id", objectID),
		zap.String("activity_id", activity.ID))

	return nil
}

// processAnnounceActivity processes an Announce activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processAnnounceActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Announce activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	// Extract the object being announced/boosted
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		} else {
			h.Logger.Error("Announce activity object missing id field",
				zap.String("activity_id", activity.ID))
			return fmt.Errorf("announce activity object missing id field")
		}
	default:
		h.Logger.Error("Announce activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("announce activity has invalid object type")
	}

	if objectID == "" {
		h.Logger.Error("Announce activity missing object ID",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("announce activity missing object ID")
	}

	// Extract the actor doing the announcing
	actorURI := activity.Actor
	if actorURI == "" {
		h.Logger.Error("Announce activity missing actor",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("announce activity missing actor")
	}

	// Create the announce record using storage.Announce format
	announce := &storage.Announce{
		Actor:     actorURI,
		Object:    objectID,
		ID:        activity.ID,
		Published: time.Now(),
		CreatedAt: time.Now(),
		To:        activity.To,
		CC:        activity.CC,
	}

	if err := h.SocialRepo.CreateAnnounce(ctx, announce); err != nil {
		h.Logger.Error("Failed to create announce record",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create announce record: %w", err)
	}

	// Find the owner of the object being announced to send notification
	object, err := h.ObjectRepo.GetObject(ctx, objectID)
	if err != nil {
		h.Logger.Warn("Could not find object for announce notification",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Don't fail the entire operation, the announce was saved
	} else {
		// Extract object author for notification
		var objectAuthor string
		switch obj := object.(type) {
		case *models.Status:
			objectAuthor = obj.AuthorID
		case *models.Object:
			objectAuthor = obj.AttributedTo
		case map[string]interface{}:
			if author, ok := obj["attributedTo"].(string); ok {
				objectAuthor = author
			}
		}

		if objectAuthor != "" {
			authorUsername := h.extractUsernameFromActorURI(objectAuthor)
			announcerUsername := h.extractUsernameFromActorURI(actorURI)

			if authorUsername != "" && announcerUsername != "" {
				// Create reblog notification for the object author
				notification := models.NewNotificationBuilder().
					ForUser(authorUsername).
					OfType("reblog").
					FromActor(announcerUsername, "remote_actor").
					AboutTarget(objectID, "status").
					WithContent(
						fmt.Sprintf("%s boosted your post", announcerUsername),
						fmt.Sprintf("Your post was boosted by %s", announcerUsername)).
					Build()

				if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
					h.Logger.Error("Failed to create reblog notification",
						zap.String("author", authorUsername),
						zap.String("announcer", announcerUsername),
						zap.String("object_id", objectID),
						zap.Error(err))
					// Don't return error - the announce was created successfully
				}
			}
		}
	}

	h.Logger.Info("Successfully processed Announce activity",
		zap.String("actor", actorURI),
		zap.String("object_id", objectID),
		zap.String("activity_id", activity.ID))

	return nil
}

// processUndoActivity processes an Undo activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processUndoActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Undo activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	// Extract the activity being undone
	var undoTarget interface{}
	switch obj := activity.Object.(type) {
	case string:
		// Object is just an ID - fetch the full activity
		originalActivity, err := h.getActivityByID(ctx, obj)
		if err != nil {
			h.Logger.Error("Failed to fetch original activity for undo",
				zap.String("activity_id", obj),
				zap.Error(err))
			return fmt.Errorf("failed to fetch original activity: %w", err)
		}
		undoTarget = originalActivity
	case map[string]interface{}:
		undoTarget = obj
	default:
		h.Logger.Error("Undo activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("undo activity has invalid object type")
	}

	// Extract the type of activity being undone
	activityType, ok := h.extractActivityType(undoTarget)
	if !ok {
		h.Logger.Error("Failed to extract activity type from undo target",
			zap.String("activity_id", activity.ID),
			zap.Any("target", undoTarget))
		return fmt.Errorf("failed to extract activity type from undo target")
	}

	// Verify actor authorization - can only undo their own activities
	targetActor := h.extractActivityActor(undoTarget)
	if targetActor != activity.Actor {
		h.Logger.Warn("Actor not authorized to undo activity",
			zap.String("undo_actor", activity.Actor),
			zap.String("target_actor", targetActor),
			zap.String("activity_type", activityType))
		return fmt.Errorf("actor not authorized to undo activity")
	}

	// Process undo based on activity type
	switch activityType {
	case ActivityTypeFollow:
		return h.processUndoFollow(ctx, activity, undoTarget, username)
	case ActivityTypeLike:
		return h.processUndoLike(ctx, activity, undoTarget, username)
	case ActivityTypeAnnounce:
		return h.processUndoAnnounce(ctx, activity, undoTarget, username)
	case ActivityTypeBlock:
		return h.processUndoBlock(ctx, activity, undoTarget, username)
	default:
		h.Logger.Info("Unsupported undo activity type",
			zap.String("activity_type", activityType),
			zap.String("activity_id", activity.ID))
		return nil // Not an error - just not implemented yet
	}
}

// getActivityByID fetches an activity by its ID
//
//nolint:unused // Kept for future implementation
func (h *ActivityHandler) getActivityByID(_ context.Context, activityID string) (*activitypub.Activity, error) {
	// Try to get from our local storage first
	// This is a simplified implementation - in practice you might need more sophisticated lookup
	h.Logger.Debug("Fetching activity by ID",
		zap.String("activity_id", activityID))
	
	// For now, return an error indicating the activity wasn't found locally
	// In a full implementation, this would check local storage and potentially fetch from remote
	return nil, fmt.Errorf("activity not found locally: %s", activityID)
}

// extractActivityType extracts the type field from an activity object
// Currently used in processUndoActivity but kept for potential future use
//
//nolint:unused // Kept for future use
func (h *ActivityHandler) extractActivityType(activity interface{}) (string, bool) {
	switch act := activity.(type) {
	case *activitypub.Activity:
		return act.Type, true
	case map[string]interface{}:
		if actType, ok := act["type"].(string); ok {
			return actType, true
		}
	}
	return "", false
}

// extractActivityActor extracts the actor field from an activity object
// Currently used in processUndoActivity but kept for potential future use
//
//nolint:unused // Kept for future use
func (h *ActivityHandler) extractActivityActor(activity interface{}) string {
	switch act := activity.(type) {
	case *activitypub.Activity:
		return act.Actor
	case map[string]interface{}:
		if actor, ok := act["actor"].(string); ok {
			return actor
		}
	}
	return ""
}

// extractActivityObject extracts the object field from an activity
// Currently used in processUndo* methods but kept for potential future use
//
//nolint:unused // Kept for future use
func (h *ActivityHandler) extractActivityObject(activity interface{}) interface{} {
	switch act := activity.(type) {
	case *activitypub.Activity:
		return act.Object
	case map[string]interface{}:
		return act["object"]
	}
	return nil
}

// processUndoFollow processes an undo of a Follow activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoFollow(ctx context.Context, undoActivity *activitypub.Activity, followActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(followActivity)
	
	var targetActor string
	switch obj := targetObject.(type) {
	case string:
		targetActor = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			targetActor = id
		}
	}

	if targetActor == "" {
		return fmt.Errorf("unable to extract target actor from follow activity")
	}

	// Remove the relationship
	if err := h.RelationshipRepo.DeleteRelationship(ctx, undoActivity.Actor, targetActor); err != nil {
		h.Logger.Error("Failed to delete follow relationship",
			zap.String("follower", undoActivity.Actor),
			zap.String("followee", targetActor),
			zap.Error(err))
		return fmt.Errorf("failed to delete follow relationship: %w", err)
	}

	h.Logger.Info("Successfully processed Undo Follow",
		zap.String("follower", undoActivity.Actor),
		zap.String("followee", targetActor))

	return nil
}

// processUndoLike processes an undo of a Like activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoLike(ctx context.Context, undoActivity *activitypub.Activity, likeActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(likeActivity)
	
	var objectID string
	switch obj := targetObject.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if objectID == "" {
		return fmt.Errorf("unable to extract object ID from like activity")
	}

	actorURI := undoActivity.Actor
	if actorURI == "" {
		return fmt.Errorf("undo like activity missing actor")
	}

	// Remove the like record
	if err := h.LikeRepo.DeleteLike(ctx, actorURI, objectID); err != nil {
		h.Logger.Error("Failed to delete like record",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to delete like record: %w", err)
	}

	h.Logger.Info("Successfully processed Undo Like",
		zap.String("actor", actorURI),
		zap.String("object_id", objectID))

	return nil
}

// processUndoAnnounce processes an undo of an Announce activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoAnnounce(ctx context.Context, undoActivity *activitypub.Activity, announceActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(announceActivity)
	
	var objectID string
	switch obj := targetObject.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if objectID == "" {
		return fmt.Errorf("unable to extract object ID from announce activity")
	}

	actorURI := undoActivity.Actor
	if actorURI == "" {
		return fmt.Errorf("undo announce activity missing actor")
	}

	// Remove the announce/boost record
	if err := h.SocialRepo.DeleteAnnounce(ctx, actorURI, objectID); err != nil {
		h.Logger.Error("Failed to delete announce record",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to delete announce record: %w", err)
	}

	h.Logger.Info("Successfully processed Undo Announce",
		zap.String("actor", actorURI),
		zap.String("object_id", objectID))

	return nil
}

// processUndoBlock processes an undo of a Block activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoBlock(ctx context.Context, undoActivity *activitypub.Activity, blockActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(blockActivity)
	
	var blockedActor string
	switch obj := targetObject.(type) {
	case string:
		blockedActor = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			blockedActor = id
		}
	}

	if blockedActor == "" {
		return fmt.Errorf("unable to extract blocked actor from block activity")
	}

	blockerActor := undoActivity.Actor
	if blockerActor == "" {
		return fmt.Errorf("undo block activity missing actor")
	}

	// Remove the block relationship
	if err := h.RelationshipRepo.DeleteBlock(ctx, blockerActor, blockedActor); err != nil {
		h.Logger.Error("Failed to delete block relationship",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.Error(err))
		return fmt.Errorf("failed to delete block relationship: %w", err)
	}

	h.Logger.Info("Successfully processed Undo Block",
		zap.String("blocker", blockerActor),
		zap.String("blocked", blockedActor))

	return nil
}

// processBlockActivity processes a Block activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processBlockActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Block activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	// Extract the actor being blocked
	var blockedActor string
	switch obj := activity.Object.(type) {
	case string:
		blockedActor = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			blockedActor = id
		} else {
			h.Logger.Error("Block activity object missing id field",
				zap.String("activity_id", activity.ID))
			return fmt.Errorf("block activity object missing id field")
		}
	default:
		h.Logger.Error("Block activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("block activity has invalid object type")
	}

	if blockedActor == "" {
		h.Logger.Error("Block activity missing blocked actor",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("block activity missing blocked actor")
	}

	// Extract the actor doing the blocking
	blockerActor := activity.Actor
	if blockerActor == "" {
		h.Logger.Error("Block activity missing blocker actor",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("block activity missing blocker actor")
	}

	// Create the block relationship
	if err := h.RelationshipRepo.CreateBlock(ctx, blockerActor, blockedActor, activity.ID); err != nil {
		h.Logger.Error("Failed to create block relationship",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create block relationship: %w", err)
	}

	// Remove any existing follow relationships in both directions
	// Blocks should sever follow relationships
	blockerUsername := h.extractUsernameFromActorURI(blockerActor)
	blockedUsername := h.extractUsernameFromActorURI(blockedActor)

	if blockerUsername != "" && blockedUsername != "" {
		// Remove blocker -> blocked follow relationship
		if err := h.RelationshipRepo.DeleteRelationship(ctx, blockerUsername, blockedUsername); err != nil {
			h.Logger.Warn("Failed to remove follow relationship (blocker->blocked)",
				zap.String("blocker", blockerUsername),
				zap.String("blocked", blockedUsername),
				zap.Error(err))
			// Don't fail the entire operation
		}

		// Remove blocked -> blocker follow relationship
		if err := h.RelationshipRepo.DeleteRelationship(ctx, blockedUsername, blockerUsername); err != nil {
			h.Logger.Warn("Failed to remove follow relationship (blocked->blocker)",
				zap.String("blocked", blockedUsername),
				zap.String("blocker", blockerUsername),
				zap.Error(err))
			// Don't fail the entire operation
		}
	}

	h.Logger.Info("Successfully processed Block activity",
		zap.String("blocker", blockerActor),
		zap.String("blocked", blockedActor),
		zap.String("activity_id", activity.ID))

	return nil
}

// processFlagActivity processes a Flag activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processFlagActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	h.Logger.Info("Processing Flag activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
	)
	return nil
}

// processMoveActivity processes a Move activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processMoveActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	h.Logger.Info("Processing Move activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
	)
	return nil
}

// processAddActivity processes an Add activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processAddActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	h.Logger.Info("Processing Add activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
	)
	return nil
}

// processRemoveActivity processes a Remove activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processRemoveActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	_ = ctx // unused parameter
	h.Logger.Info("Processing Remove activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
	)
	return nil
}

// deliverActivity delivers an activity to remote servers
//
//nolint:unused // False positive - called from processOutboxActivity
func (h *ActivityHandler) deliverActivity(_ context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Delivering activity to federation network",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
		zap.String("actor", activity.Actor),
		zap.Any("to", activity.To),
		zap.Any("cc", activity.CC),
	)

	// Extract recipients from the activity
	recipients := make([]string, 0)
	
	// Add direct recipients (To field)
	if activity.To != nil {
		recipients = append(recipients, activity.To...)
	}
	
	// Add carbon copy recipients (CC field)
	if activity.CC != nil {
		recipients = append(recipients, activity.CC...)
	}

	if len(recipients) == 0 {
		h.Logger.Debug("No recipients found for activity, skipping delivery",
			zap.String("activity_id", activity.ID))
		return nil
	}

	// Filter out local recipients and public addresses
	remoteRecipients := h.filterRemoteRecipients(recipients)
	
	if len(remoteRecipients) == 0 {
		h.Logger.Debug("No remote recipients found for activity, skipping delivery",
			zap.String("activity_id", activity.ID),
			zap.Any("all_recipients", recipients))
		return nil
	}

	// For now, we'll queue the activity for federation delivery
	// This is a simplified implementation - in production you'd:
	// 1. Resolve remote actor inboxes
	// 2. Group by shared inbox for efficiency
	// 3. Queue delivery jobs to SQS
	// 4. Handle signatures and authentication
	
	h.Logger.Info("Queueing activity for federation delivery",
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
		zap.Int("remote_recipients", len(remoteRecipients)),
		zap.Any("recipients", remoteRecipients))

	// TODO: Implement actual federation delivery
	// This would involve:
	// - Creating a federation delivery service
	// - Resolving actor inboxes
	// - Queueing delivery jobs
	// - Handling retries and failures
	
	// For now, just log that we would deliver
	for _, recipient := range remoteRecipients {
		h.Logger.Debug("Would deliver activity to recipient",
			zap.String("activity_id", activity.ID),
			zap.String("recipient", recipient))
	}

	return nil
}

// filterRemoteRecipients filters out local recipients and public addresses
//
//nolint:unused // Helper method for deliverActivity
func (h *ActivityHandler) filterRemoteRecipients(recipients []string) []string {
	domain := os.Getenv("DOMAIN_NAME")
	if domain == "" {
		domain = DefaultTestingDomain // Default for testing
	}
	
	remoteRecipients := make([]string, 0)
	
	for _, recipient := range recipients {
		// Skip public addresses
		if recipient == "https://www.w3.org/ns/activitystreams#Public" ||
		   recipient == "as:Public" ||
		   recipient == "Public" {
			continue
		}
		
		// Skip local recipients
		if strings.Contains(recipient, domain) {
			continue
		}
		
		// Skip empty recipients
		if recipient == "" {
			continue
		}
		
		remoteRecipients = append(remoteRecipients, recipient)
	}
	
	return remoteRecipients
}

// extractUsernameFromActorURI extracts the username from an ActivityPub actor URI
// Examples: 
// - https://example.com/users/alice -> alice
// - https://mastodon.social/@bob -> bob
//
//nolint:unused // Used by various activity processors
func (h *ActivityHandler) extractUsernameFromActorURI(actorURI string) string {
	// Handle common ActivityPub URI patterns
	if strings.Contains(actorURI, "/users/") {
		parts := strings.Split(actorURI, "/users/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "/")[0]
		}
	}
	
	if strings.Contains(actorURI, "/@") {
		parts := strings.Split(actorURI, "/@")
		if len(parts) > 1 {
			return strings.Split(parts[1], "/")[0]
		}
	}
	
	// Fallback: extract the last path segment
	parts := strings.Split(strings.TrimSuffix(actorURI, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	
	return ""
}

// createNotificationRepo creates a notification repository instance
//
//nolint:unused // Used by activity processors that create notifications
func (h *ActivityHandler) createNotificationRepo() *repositories.NotificationRepository {
	return repositories.NewNotificationRepository(h.DB, h.TableName, h.Logger)
}
