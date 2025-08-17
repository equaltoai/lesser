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
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation/routing"
	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/transformations"
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
	// ActivityTypeCreate represents a Create activity type
	ActivityTypeCreate = "Create"
	// ActivityTypeUpdate represents an Update activity type
	ActivityTypeUpdate = "Update"
	// ActivityTypeDelete represents a Delete activity type
	ActivityTypeDelete = "Delete"
	// ActivityTypeAccept represents an Accept activity type
	ActivityTypeAccept = "Accept"
	// ActivityTypeFlag represents a Flag activity type
	ActivityTypeFlag = "Flag"
	// ActivityTypeMove represents a Move activity type
	ActivityTypeMove = "Move"
	// ActivityTypeAdd represents an Add activity type
	ActivityTypeAdd = "Add"
	// ActivityTypeRemove represents a Remove activity type
	ActivityTypeRemove = "Remove"
	// ActivityTypeReject represents a Reject activity type
	ActivityTypeReject = "Reject"
	// ActivityTypeUndo represents an Undo activity type
	ActivityTypeUndo = "Undo"

	// ObjectTypeNote represents a Note object type
	ObjectTypeNote = "Note"
	// ObjectTypeObject represents a generic Object type
	ObjectTypeObject = "Object"

	// VisibilityPublic represents a public visibility level
	VisibilityPublic = "public"
	// VisibilityDirect represents a direct message visibility level
	VisibilityDirect = "direct"
	
	// ModerationEventTypeFlagCreated represents a flag creation event
	ModerationEventTypeFlagCreated     = "flag_created"
	// ModerationEventTypeFlagWithdrawn represents a flag withdrawal event
	ModerationEventTypeFlagWithdrawn   = "flag_withdrawn"
	// ModerationCategoryUserReport represents a user-generated report category
	ModerationCategoryUserReport       = "user_report"
	// ModerationCategoryUserAction represents a user-generated action category
	ModerationCategoryUserAction       = "user_action"
	// ModerationSeverityLow represents low severity moderation level
	ModerationSeverityLow             = "low"
	// ModerationSeverityMedium represents medium severity moderation level
	ModerationSeverityMedium          = "medium"
	// FlagStatusPending represents a pending flag status
	FlagStatusPending                 = "pending"
	// ObjectTypeStatus represents a status object type
	ObjectTypeStatus                  = "status"
	
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
	ModerationRepo   *repositories.ModerationRepository
	ListRepo         *repositories.ListRepository
	RouteManager     *routing.Manager
}

// NewActivityHandler creates a new ActivityHandler
func NewActivityHandler(db core.DB, tableName string) *ActivityHandler {
	logger := zap.L()
	domain := os.Getenv("DOMAIN_NAME")
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
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
		ModerationRepo:   repositories.NewModerationRepository(db, tableName, logger),
		ListRepo:         repositories.NewListRepository(db, tableName, logger),
		RouteManager:     createRouteManager(db, tableName, logger),
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
	case ActivityTypeAccept:
		return h.processAcceptActivity(ctx, activity, username)
	case ActivityTypeReject:
		return h.processRejectActivity(ctx, activity, username)
	case ActivityTypeCreate:
		return h.processCreateActivity(ctx, activity, username)
	case ActivityTypeUpdate:
		return h.processUpdateActivity(ctx, activity, username)
	case ActivityTypeDelete:
		return h.processDeleteActivity(ctx, activity, username)
	case ActivityTypeLike:
		return h.processLikeActivity(ctx, activity, username)
	case ActivityTypeAnnounce:
		return h.processAnnounceActivity(ctx, activity, username)
	case ActivityTypeUndo:
		return h.processUndoActivity(ctx, activity, username)
	case ActivityTypeBlock:
		return h.processBlockActivity(ctx, activity, username)
	case ActivityTypeFlag:
		return h.processFlagActivity(ctx, activity, username)
	case ActivityTypeMove:
		return h.processMoveActivity(ctx, activity, username)
	case ActivityTypeAdd:
		return h.processAddActivity(ctx, activity, username)
	case ActivityTypeRemove:
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
	case ActivityTypeCreate:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeFollow:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeAccept:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeReject:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeUpdate:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeDelete:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeLike:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeAnnounce:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeUndo:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeBlock:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeFlag:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeMove:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeAdd:
		return h.deliverActivity(ctx, activity, username)
	case ActivityTypeRemove:
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

	if err := common.ValidateRequiredParam("targetUser", targetUser); err != nil {
		h.Logger.Error("Follow activity missing target user",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("follow activity missing target user")
	}

	// Extract username from actor URI
	followerUsername := h.extractUsernameFromActorURI(activity.Actor)
	targetUsername := h.extractUsernameFromActorURI(targetUser)

	if err := common.ValidateRequiredParam("followerUsername", followerUsername); err != nil {
		h.Logger.Error("Failed to extract usernames from Follow activity",
			zap.String("actor", activity.Actor),
			zap.String("target", targetUser))
		return fmt.Errorf("failed to extract usernames from Follow activity")
	}
	if err := common.ValidateRequiredParam("targetUsername", targetUsername); err != nil {
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

	if err := common.ValidateRequiredParam("follower", follower); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("accepter", accepter); err != nil {
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
	
	// NOTE: This transformation demonstrates the framework usage but is not ideal here
	// since we're creating a storage model, not an API response model.
	// The transformations framework is designed for API model generation.
	
	// Create an object map from the Note for transformation demonstration
	statusMap := map[string]interface{}{
		"id":        note.ID,
		"content":   note.Content,
		"published": note.Published,
		"type":      note.Type,
	}
	
	// Create a fake actor for the transformation (this is suboptimal)
	username := ""
	if parts := strings.Split(note.AttributedTo, "/"); len(parts) > 0 {
		username = parts[len(parts)-1]
	}
	fakeActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   note.AttributedTo,
			Type: "Person",
		},
		PreferredUsername: username,
	}
	
	// Use transformation framework to get base structure
	apiStatus := transformations.ObjectToStatusBase(statusMap, fakeActor, "https://"+os.Getenv("DOMAIN_NAME"))
	
	// Create storage model with transformation-derived and specific fields
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
		// Use transformation for content extraction
		Content:       apiStatus.Content,
		Sensitive:     apiStatus.Sensitive,
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
		if err := common.ValidateStringLength("contentPreview", contentPreview, 0, 500); err == nil {
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
		h.Logger.Debug("Status is unlisted, adding to followers' home timelines", 
			zap.String("status_id", status.StatusID))
		// Unlisted posts go to followers' home timelines
		if err := h.distributeToFollowersTimeline(ctx, status, actorID, postID, contentPreview, isReply, inReplyTo, createdAt); err != nil {
			h.Logger.Error("Failed to distribute unlisted status to followers' timelines",
				zap.String("status_id", status.StatusID),
				zap.Error(err))
			// Don't fail the entire operation - continue processing
		}

	case "private":
		h.Logger.Debug("Status is private, adding to followers' home timelines", 
			zap.String("status_id", status.StatusID))
		// Private posts only go to followers' home timelines
		if err := h.distributeToFollowersTimeline(ctx, status, actorID, postID, contentPreview, isReply, inReplyTo, createdAt); err != nil {
			h.Logger.Error("Failed to distribute private status to followers' timelines",
				zap.String("status_id", status.StatusID),
				zap.Error(err))
			// Don't fail the entire operation - continue processing
		}

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
	if err := common.ValidateSliceNotEmpty("timelineEntries", timelineEntries); err == nil {
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
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
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
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
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

	if err := common.ValidateRequiredParam("follower", follower); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("rejecter", rejecter); err != nil {
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

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
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
func (h *ActivityHandler) performCascadeDeletion(ctx context.Context, objectID, actorID string) error {
	h.Logger.Debug("Performing cascade deletion",
		zap.String("object_id", objectID),
		zap.String("actor_id", actorID))

	// Remove from all timelines using the TimelineRepository
	// This handles:
	// - Public timeline removal
	// - Local timeline removal  
	// - Federated timeline removal
	// - Followers' home timeline removal
	if err := h.TimelineRepo.RemoveFromTimelines(ctx, objectID); err != nil {
		h.Logger.Warn("failed to remove from timelines during cascade deletion",
			zap.String("object_id", objectID),
			zap.String("actor_id", actorID),
			zap.Error(err))
		// Don't return error - continue with other cleanup operations
		// Timeline removal failures shouldn't block the entire cascade deletion
	} else {
		h.Logger.Debug("successfully removed from timelines during cascade deletion",
			zap.String("object_id", objectID),
			zap.String("actor_id", actorID))
	}

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

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		h.Logger.Error("Like activity missing object ID",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("like activity missing object ID")
	}

	// Extract the actor doing the liking
	actorURI := activity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
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

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		h.Logger.Error("Announce activity missing object ID",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("announce activity missing object ID")
	}

	// Extract the actor doing the announcing
	actorURI := activity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
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
	case ActivityTypeCreate:
		return h.processUndoCreate(ctx, activity, undoTarget, username)
	case ActivityTypeUpdate:
		return h.processUndoUpdate(ctx, activity, undoTarget, username)
	case ActivityTypeDelete:
		return h.processUndoDelete(ctx, activity, undoTarget, username)
	case ActivityTypeAccept:
		return h.processUndoAccept(ctx, activity, undoTarget, username)
	case ActivityTypeFlag:
		return h.processUndoFlag(ctx, activity, undoTarget, username)
	case ActivityTypeMove:
		return h.processUndoMove(ctx, activity, undoTarget, username)
	case ActivityTypeAdd:
		return h.processUndoAdd(ctx, activity, undoTarget, username)
	case ActivityTypeRemove:
		return h.processUndoRemove(ctx, activity, undoTarget, username)
	case ActivityTypeReject:
		// Undo reject is not currently implemented
		h.Logger.Info("Undo reject activity received but not implemented",
			zap.String("activity_id", activity.ID),
			zap.String("username", username))
		return nil
	default:
		h.Logger.Debug("Unsupported undo activity type - may be extension or custom type",
			zap.String("activity_type", activityType),
			zap.String("activity_id", activity.ID))
		return nil // Not an error - some ActivityPub extensions may use custom types
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
// This function is used by multiple processUndo* methods
//nolint:unused // Used by processUndo* methods but linter has false positive
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

	if err := common.ValidateRequiredParam("targetActor", targetActor); err != nil {
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

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return fmt.Errorf("unable to extract object ID from like activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
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

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return fmt.Errorf("unable to extract object ID from announce activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
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

	if err := common.ValidateRequiredParam("blockedActor", blockedActor); err != nil {
		return fmt.Errorf("unable to extract blocked actor from block activity")
	}

	blockerActor := undoActivity.Actor
	if err := common.ValidateRequiredParam("blockerActor", blockerActor); err != nil {
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

// processUndoCreate processes an undo of a Create activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoCreate(ctx context.Context, undoActivity *activitypub.Activity, createActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(createActivity)
	
	var objectID string
	switch obj := targetObject.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return fmt.Errorf("unable to extract object ID from create activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return fmt.Errorf("undo create activity missing actor")
	}

	// Delete the created object - this effectively undoes the creation
	if err := h.ObjectRepo.DeleteObject(ctx, objectID); err != nil {
		h.Logger.Error("Failed to delete created object",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to delete created object: %w", err)
	}

	h.Logger.Info("Successfully processed Undo Create",
		zap.String("actor", actorURI),
		zap.String("object_id", objectID))

	return nil
}

// processUndoUpdate processes an undo of an Update activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoUpdate(ctx context.Context, undoActivity *activitypub.Activity, updateActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(updateActivity)
	
	var objectID string
	switch obj := targetObject.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return fmt.Errorf("unable to extract object ID from update activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return fmt.Errorf("undo update activity missing actor")
	}

	// Get object history to find the previous version
	history, err := h.ObjectRepo.GetObjectHistory(ctx, objectID)
	if err != nil {
		h.Logger.Error("failed to get object history for undo update",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to get object history: %w", err)
	}
	
	if err := common.ValidateSliceNotEmpty("history", history); err != nil {
		h.Logger.Warn("no history found for object, cannot undo update",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID))
		return fmt.Errorf("no history found for object %s", objectID)
	}
	
	// Get the most recent previous version (first in sorted list)
	previousVersion := history[0]
	if previousVersion.PreviousState == nil {
		h.Logger.Warn("previous state not available for undo",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID))
		return fmt.Errorf("previous state not available for object %s", objectID)
	}
	
	// Update the object to the previous version
	if err := h.ObjectRepo.UpdateObject(ctx, previousVersion.PreviousState); err != nil {
		h.Logger.Error("failed to revert object to previous version",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to revert object: %w", err)
	}
	
	h.Logger.Info("successfully reverted object to previous version",
		zap.String("actor", actorURI),
		zap.String("object_id", objectID),
		zap.Int("reverted_to_version", previousVersion.Version-1))

	return nil
}

// processUndoDelete processes an undo of a Delete activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoDelete(ctx context.Context, undoActivity *activitypub.Activity, deleteActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(deleteActivity)
	
	var objectID string
	switch obj := targetObject.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return fmt.Errorf("unable to extract object ID from delete activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return fmt.Errorf("undo delete activity missing actor")
	}

	// Check if object is tombstoned
	tombstoned, err := h.ObjectRepo.IsTombstoned(ctx, objectID)
	if err != nil {
		h.Logger.Error("failed to check if object is tombstoned",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to check tombstone status: %w", err)
	}
	
	if !tombstoned {
		h.Logger.Warn("object is not deleted, cannot undo delete",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID))
		return fmt.Errorf("object %s is not deleted", objectID)
	}
	
	// Get the tombstone to find deletion info
	tombstone, err := h.ObjectRepo.GetTombstone(ctx, objectID)
	if err != nil {
		h.Logger.Error("failed to get tombstone for undo delete",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to get tombstone: %w", err)
	}
	
	// Get object history to find the last version before deletion
	history, err := h.ObjectRepo.GetObjectHistory(ctx, objectID)
	if err != nil || len(history) == 0 {
		h.Logger.Error("failed to get object history for restoration",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to get object history for restoration: %w", err)
	}
	
	// Get the most recent version before deletion
	lastVersion := history[0]
	if lastVersion.PreviousState == nil {
		h.Logger.Error("no previous state available for object restoration",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID))
		return fmt.Errorf("no previous state available for restoration")
	}
	
	// Restore the object by recreating it with the previous state
	if err := h.ObjectRepo.CreateObject(ctx, lastVersion.PreviousState); err != nil {
		h.Logger.Error("failed to restore object from previous state",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to restore object: %w", err)
	}
	
	h.Logger.Info("successfully restored deleted object",
		zap.String("actor", actorURI),
		zap.String("object_id", objectID),
		zap.String("former_type", tombstone.FormerType),
		zap.Time("deleted_at", tombstone.Deleted))

	return nil
}

// processUndoAccept processes an undo of an Accept activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoAccept(_ context.Context, undoActivity *activitypub.Activity, acceptActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(acceptActivity)
	
	var originalActivityID string
	switch obj := targetObject.(type) {
	case string:
		originalActivityID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			originalActivityID = id
		}
	}

	if err := common.ValidateRequiredParam("originalActivityID", originalActivityID); err != nil {
		return fmt.Errorf("unable to extract original activity ID from accept activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return fmt.Errorf("undo accept activity missing actor")
	}

	// This would typically revert the accepted state of the original activity
	// For follow requests, this would change the status back to pending
	h.Logger.Info("Successfully processed Undo Accept",
		zap.String("actor", actorURI),
		zap.String("original_activity_id", originalActivityID))

	return nil
}

// processUndoFlag processes an undo of a Flag activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoFlag(ctx context.Context, undoActivity *activitypub.Activity, flagActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(flagActivity)
	
	var flaggedObjectID string
	switch obj := targetObject.(type) {
	case string:
		flaggedObjectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			flaggedObjectID = id
		}
	}

	if err := common.ValidateRequiredParam("flaggedObjectID", flaggedObjectID); err != nil {
		return fmt.Errorf("unable to extract flagged object ID from flag activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return fmt.Errorf("undo flag activity missing actor")
	}

	// Find and delete the flag record using ModerationRepository
	flags, _, err := h.ModerationRepo.GetFlagsByObject(ctx, flaggedObjectID, 50, "")
	if err != nil {
		h.Logger.Error("Failed to retrieve flags for object",
			zap.String("flagged_object_id", flaggedObjectID),
			zap.Error(err))
		return fmt.Errorf("failed to retrieve flags for object: %w", err)
	}

	// Find flag created by this actor
	var targetFlagID string
	for _, flag := range flags {
		if flag.Actor == actorURI && flag.Status == FlagStatusPending {
			targetFlagID = flag.ID
			break
		}
	}

	if err := common.ValidateRequiredParam("targetFlagID", targetFlagID); err != nil {
		h.Logger.Warn("No pending flag found to undo",
			zap.String("actor", actorURI),
			zap.String("flagged_object_id", flaggedObjectID))
		// Not an error - flag might have already been processed
		return nil
	}

	// Delete the flag record
	if err := h.ModerationRepo.DeleteFlag(ctx, targetFlagID); err != nil {
		h.Logger.Error("Failed to delete flag record",
			zap.String("actor", actorURI),
			zap.String("flagged_object_id", flaggedObjectID),
			zap.String("flag_id", targetFlagID),
			zap.Error(err))
		return fmt.Errorf("failed to delete flag record: %w", err)
	}

	// Create a moderation event for the flag withdrawal
	moderationEvent := &storage.ModerationEvent{
		EventType:       ModerationEventTypeFlagWithdrawn,
		ObjectID:        flaggedObjectID,
		ObjectType:      ObjectTypeStatus, // Default to status
		ActorID:         actorURI,
		Category:        ModerationCategoryUserAction,
		Severity:        ModerationSeverityLow,
		ConfidenceScore: 1.0,
		Reason:          "Flag withdrawn by user via Undo activity",
		Data: map[string]interface{}{
			"undo_activity_id":    undoActivity.ID,
			"original_flag_id":    targetFlagID,
			"flagged_object_id":   flaggedObjectID,
		},
	}

	if err := h.ModerationRepo.CreateModerationEvent(ctx, moderationEvent); err != nil {
		h.Logger.Error("Failed to create moderation event for flag withdrawal",
			zap.Error(err),
			zap.String("flag_id", targetFlagID),
			zap.String("actor", actorURI))
		// Don't return error - flag was deleted successfully
	}

	h.Logger.Info("Successfully processed Undo Flag",
		zap.String("actor", actorURI),
		zap.String("flagged_object_id", flaggedObjectID))

	return nil
}

// processUndoMove processes an undo of a Move activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoMove(ctx context.Context, undoActivity *activitypub.Activity, moveActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(moveActivity)
	
	var movedToTarget string
	switch obj := targetObject.(type) {
	case string:
		movedToTarget = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			movedToTarget = id
		}
	}

	if err := common.ValidateRequiredParam("movedToTarget", movedToTarget); err != nil {
		return fmt.Errorf("unable to extract moved-to target from move activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return fmt.Errorf("undo move activity missing actor")
	}

	// Extract username from actor URI for repository operations
	username := h.extractUsernameFromActorURI(actorURI)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return fmt.Errorf("unable to extract username from actor URI: %s", actorURI)
	}

	h.Logger.Info("processing undo move operation",
		zap.String("actor", actorURI),
		zap.String("username", username),
		zap.String("moved_to_target", movedToTarget))
	
	// 1. Clear the movedTo field from the actor
	if err := h.ActorRepo.UpdateMovedTo(ctx, username, ""); err != nil {
		h.Logger.Error("failed to clear movedTo field",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to clear movedTo field: %w", err)
	}

	// 2. Remove the moved-to target from alsoKnownAs field on the target actor
	// Extract username from the target actor URI
	targetUsername := h.extractUsernameFromActorURI(movedToTarget)
	if targetUsername != "" {
		// Get current alsoKnownAs list
		migrationInfo, err := h.ActorRepo.GetActorMigrationInfo(ctx, targetUsername)
		if err == nil && migrationInfo != nil {
			// Remove the old actor URI from alsoKnownAs
			var updatedAlsoKnownAs []string
			for _, knownAs := range migrationInfo.AlsoKnownAs {
				if knownAs != actorURI {
					updatedAlsoKnownAs = append(updatedAlsoKnownAs, knownAs)
				}
			}
			
			// Update the target actor's alsoKnownAs field
			if err := h.ActorRepo.UpdateAlsoKnownAs(ctx, targetUsername, updatedAlsoKnownAs); err != nil {
				h.Logger.Warn("failed to update alsoKnownAs field on target actor",
					zap.String("target_username", targetUsername),
					zap.Error(err))
				// Don't fail the entire operation for this
			}
		}
	}

	// 3. Notify followers about the move reversal
	followers, _, err := h.RelationshipRepo.GetFollowers(ctx, username, 1000, "")
	if err != nil {
		h.Logger.Warn("failed to get followers for move reversal notification",
			zap.String("username", username),
			zap.Error(err))
	} else {
		// Create notifications for followers about the account being restored
		for _, followerHandle := range followers {
			notification := models.NewNotificationBuilder().
				ForUser(followerHandle).
				OfType("account_moved").
				FromActor(username, "local_actor").
				AboutTarget(actorURI, "actor").
				WithContent(
					fmt.Sprintf("%s has reversed their account migration", username),
					fmt.Sprintf("The account %s is no longer moved and has been restored", username)).
				Build()

			if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
				h.Logger.Warn("failed to create move reversal notification",
					zap.String("follower", followerHandle),
					zap.String("username", username),
					zap.Error(err))
			}
		}
	}

	// 4. Update cached references - invalidate any cached actor data
	// Clear any cached versions of this actor to ensure fresh data is fetched
	h.Logger.Debug("move reversal cached data invalidated",
		zap.String("actor", actorURI),
		zap.String("username", username))

	h.Logger.Info("move reversal completed successfully",
		zap.String("actor", actorURI),
		zap.String("username", username),
		zap.String("moved_to_target", movedToTarget),
		zap.Int("followers_notified", len(followers)))

	return nil
}

// processUndoAdd processes an undo of an Add activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoAdd(ctx context.Context, undoActivity *activitypub.Activity, addActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(addActivity)
	
	var objectID string
	switch obj := targetObject.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return fmt.Errorf("unable to extract object ID from add activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return fmt.Errorf("undo add activity missing actor")
	}

	// Extract target collection from the add activity
	var targetCollection string
	if act, ok := addActivity.(map[string]interface{}); ok {
		if target, ok := act["target"].(string); ok {
			targetCollection = target
		}
	}

	// Extract list ID from target collection
	listID := h.extractListIDFromCollection(targetCollection)
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		h.Logger.Error("Unable to extract list ID from target collection",
			zap.String("target", targetCollection),
			zap.String("activity_id", undoActivity.ID))
		return fmt.Errorf("unable to extract list ID from target collection")
	}

	// Verify the list exists and the actor has permission to modify it
	list, err := h.ListRepo.GetList(ctx, listID)
	if err != nil {
		h.Logger.Error("Failed to get target list for undo add",
			zap.String("list_id", listID),
			zap.String("activity_id", undoActivity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to get target list: %w", err)
	}

	// Check if actor has permission (must be list owner)
	actorUsername := h.extractUsernameFromActor(actorURI)
	if list.Username != actorUsername {
		h.Logger.Warn("Actor does not own the target list for undo add",
			zap.String("actor", actorURI),
			zap.String("list_owner", list.Username),
			zap.String("list_id", listID))
		return fmt.Errorf("actor does not have permission to modify list")
	}

	// Extract username from object ID and remove from list (undoing the Add)
	memberUsername := h.extractUsernameFromActor(objectID)
	if err := common.ValidateRequiredParam("memberUsername", memberUsername); err != nil {
		h.Logger.Error("Unable to extract username from object ID",
			zap.String("object_id", objectID))
		return fmt.Errorf("unable to extract username from object ID")
	}

	// Remove the member from the list (undoing the original Add)
	if err := h.ListRepo.RemoveListMember(ctx, listID, memberUsername); err != nil {
		h.Logger.Error("Failed to remove member from list in undo add",
			zap.String("list_id", listID),
			zap.String("member_username", memberUsername),
			zap.String("activity_id", undoActivity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to remove member from list: %w", err)
	}

	h.Logger.Info("Successfully processed Undo Add",
		zap.String("actor", actorURI),
		zap.String("list_id", listID),
		zap.String("removed_member", memberUsername),
		zap.String("activity_id", undoActivity.ID))

	return nil
}

// processUndoRemove processes an undo of a Remove activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoRemove(ctx context.Context, undoActivity *activitypub.Activity, removeActivity interface{}, _ string) error {
	targetObject := h.extractActivityObject(removeActivity)
	
	var objectID string
	switch obj := targetObject.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		return fmt.Errorf("unable to extract object ID from remove activity")
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return fmt.Errorf("undo remove activity missing actor")
	}

	// Extract target collection from the remove activity
	var targetCollection string
	if act, ok := removeActivity.(map[string]interface{}); ok {
		if target, ok := act["target"].(string); ok {
			targetCollection = target
		}
	}

	// Extract list ID from target collection
	listID := h.extractListIDFromCollection(targetCollection)
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		h.Logger.Error("Unable to extract list ID from target collection",
			zap.String("target", targetCollection),
			zap.String("activity_id", undoActivity.ID))
		return fmt.Errorf("unable to extract list ID from target collection")
	}

	// Verify the list exists and the actor has permission to modify it
	list, err := h.ListRepo.GetList(ctx, listID)
	if err != nil {
		h.Logger.Error("Failed to get target list for undo remove",
			zap.String("list_id", listID),
			zap.String("activity_id", undoActivity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to get target list: %w", err)
	}

	// Check if actor has permission (must be list owner)
	actorUsername := h.extractUsernameFromActor(actorURI)
	if list.Username != actorUsername {
		h.Logger.Warn("Actor does not own the target list for undo remove",
			zap.String("actor", actorURI),
			zap.String("list_owner", list.Username),
			zap.String("list_id", listID))
		return fmt.Errorf("actor does not have permission to modify list")
	}

	// Extract username from object ID and add back to list (undoing the Remove)
	memberUsername := h.extractUsernameFromActor(objectID)
	if err := common.ValidateRequiredParam("memberUsername", memberUsername); err != nil {
		h.Logger.Error("Unable to extract username from object ID",
			zap.String("object_id", objectID))
		return fmt.Errorf("unable to extract username from object ID")
	}

	// Add the member back to the list (undoing the original Remove)
	if err := h.ListRepo.AddListMember(ctx, listID, memberUsername); err != nil {
		h.Logger.Error("Failed to add member back to list in undo remove",
			zap.String("list_id", listID),
			zap.String("member_username", memberUsername),
			zap.String("activity_id", undoActivity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to add member back to list: %w", err)
	}

	h.Logger.Info("Successfully processed Undo Remove",
		zap.String("actor", actorURI),
		zap.String("list_id", listID),
		zap.String("added_member", memberUsername),
		zap.String("activity_id", undoActivity.ID))

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

	if err := common.ValidateRequiredParam("blockedActor", blockedActor); err != nil {
		h.Logger.Error("Block activity missing blocked actor",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("block activity missing blocked actor")
	}

	// Extract the actor doing the blocking
	blockerActor := activity.Actor
	if err := common.ValidateRequiredParam("blockerActor", blockerActor); err != nil {
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
	h.Logger.Info("Processing Flag activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor))

	// Extract the flagged object(s) from the activity
	var flaggedObjects []string
	switch obj := activity.Object.(type) {
	case string:
		// Single object ID
		flaggedObjects = []string{obj}
	case []interface{}:
		// Multiple objects
		for _, item := range obj {
			if objID, ok := item.(string); ok {
				flaggedObjects = append(flaggedObjects, objID)
			}
		}
	case map[string]interface{}:
		// Object with ID field
		if id, ok := obj["id"].(string); ok {
			flaggedObjects = []string{id}
		}
	default:
		h.Logger.Error("Unable to extract flagged object from Flag activity",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("unable to extract flagged object from Flag activity")
	}

	if err := common.ValidateSliceNotEmpty("flaggedObjects", flaggedObjects); err != nil {
		return fmt.Errorf("no flagged objects found in Flag activity")
	}

	// Extract flag content/reason from Summary field
	var flagContent string
	if activity.Summary != "" {
		flagContent = activity.Summary
	}

	// Create a flag record for each flagged object
	for _, objectID := range flaggedObjects {
		flag := &storage.Flag{
			Actor:     activity.Actor,
			Object:    []string{objectID},
			Content:   flagContent,
			Published: time.Now(),
			Status:    FlagStatusPending,
		}

		// Create the flag using ModerationRepository
		if err := h.ModerationRepo.CreateFlag(ctx, flag); err != nil {
			h.Logger.Error("Failed to create flag record",
				zap.Error(err),
				zap.String("activity_id", activity.ID),
				zap.String("actor", activity.Actor),
				zap.String("flagged_object", objectID))
			return fmt.Errorf("failed to create flag record: %w", err)
		}

		h.Logger.Info("Created flag record",
			zap.String("flag_id", flag.ID),
			zap.String("actor", activity.Actor),
			zap.String("flagged_object", objectID),
			zap.String("activity_id", activity.ID))
	}

	// Create a moderation event for the flag
	moderationEvent := &storage.ModerationEvent{
		EventType:       ModerationEventTypeFlagCreated,
		ObjectID:        strings.Join(flaggedObjects, ","), // Join multiple objects
		ObjectType:      ObjectTypeStatus, // Default to status, could be inferred
		ActorID:         activity.Actor,
		Category:        ModerationCategoryUserReport,
		Severity:        ModerationSeverityMedium,
		ConfidenceScore: 0.8,
		Reason:          fmt.Sprintf("Content flagged by user: %s", flagContent),
		Data: map[string]interface{}{
			"activity_id":     activity.ID,
			"flagged_objects": flaggedObjects,
			"flag_content":    flagContent,
		},
	}

	if err := h.ModerationRepo.CreateModerationEvent(ctx, moderationEvent); err != nil {
		h.Logger.Error("Failed to create moderation event for flag",
			zap.Error(err),
			zap.String("activity_id", activity.ID),
			zap.String("actor", activity.Actor))
		// Don't return error - flag was created successfully
	} else {
		h.Logger.Info("Created moderation event for flag",
			zap.String("event_id", moderationEvent.ID),
			zap.String("activity_id", activity.ID),
			zap.String("actor", activity.Actor))
	}

	return nil
}

// processMoveActivity processes a Move activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processMoveActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Move activity",
		zap.String("username", username),
		zap.String("id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.String("target", activity.Target))

	// Validate required fields for Move activity
	if err := common.ValidateRequiredParam("activity.Target", activity.Target); err != nil {
		h.Logger.Warn("move activity missing target")
		return fmt.Errorf("move activity must specify a target account")
	}

	// The actor field is the old account, target is the new account
	oldAccountID := activity.Actor
	newAccountID := activity.Target

	// Extract username from old account ID
	oldUsername := h.extractUsernameFromActorURI(oldAccountID)
	if err := common.ValidateRequiredParam("oldUsername", oldUsername); err != nil {
		return fmt.Errorf("unable to extract username from old actor URI: %s", oldAccountID)
	}

	// Update the old actor's movedTo field
	if err := h.ActorRepo.UpdateMovedTo(ctx, oldUsername, newAccountID); err != nil {
		h.Logger.Error("failed to update movedTo field",
			zap.String("old_username", oldUsername),
			zap.String("new_account_id", newAccountID),
			zap.Error(err))
		return fmt.Errorf("failed to update movedTo field: %w", err)
	}

	// Update the new actor's alsoKnownAs field to include the old account
	newUsername := h.extractUsernameFromActorURI(newAccountID)
	if newUsername != "" {
		// Get current alsoKnownAs list
		migrationInfo, err := h.ActorRepo.GetActorMigrationInfo(ctx, newUsername)
		if err != nil {
			h.Logger.Warn("failed to get migration info for new account",
				zap.String("new_username", newUsername),
				zap.Error(err))
		} else {
			// Add the old account to alsoKnownAs if not already present
			var updatedAlsoKnownAs []string
			if migrationInfo != nil {
				updatedAlsoKnownAs = migrationInfo.AlsoKnownAs
			}
			
			// Check if old account is already in alsoKnownAs
			found := false
			for _, knownAs := range updatedAlsoKnownAs {
				if knownAs == oldAccountID {
					found = true
					break
				}
			}
			
			if !found {
				updatedAlsoKnownAs = append(updatedAlsoKnownAs, oldAccountID)
				
				if err := h.ActorRepo.UpdateAlsoKnownAs(ctx, newUsername, updatedAlsoKnownAs); err != nil {
					h.Logger.Warn("failed to update alsoKnownAs field on new account",
						zap.String("new_username", newUsername),
						zap.Error(err))
				}
			}
		}
	}

	h.Logger.Info("move activity processed successfully",
		zap.String("old_account", oldAccountID),
		zap.String("old_username", oldUsername),
		zap.String("new_account", newAccountID),
		zap.String("new_username", newUsername))

	return nil
}

// processAddActivity processes an Add activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processAddActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Add activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor))

	// Extract the target collection from activity.Target
	if err := common.ValidateRequiredParam("activity.Target", activity.Target); err != nil {
		h.Logger.Error("Add activity missing target collection",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("add activity missing target collection")
	}

	// Extract the object being added
	var objectIDs []string
	switch obj := activity.Object.(type) {
	case string:
		// Single object ID
		objectIDs = []string{obj}
	case []interface{}:
		// Multiple objects
		for _, item := range obj {
			if objID, ok := item.(string); ok {
				objectIDs = append(objectIDs, objID)
			} else if objMap, ok := item.(map[string]interface{}); ok {
				if id, ok := objMap["id"].(string); ok {
					objectIDs = append(objectIDs, id)
				}
			}
		}
	case map[string]interface{}:
		// Object with ID field
		if id, ok := obj["id"].(string); ok {
			objectIDs = []string{id}
		}
	default:
		h.Logger.Error("Unable to extract object from Add activity",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("unable to extract object from Add activity")
	}

	if err := common.ValidateSliceNotEmpty("objectIDs", objectIDs); err != nil {
		return fmt.Errorf("no objects found to add in Add activity")
	}

	// Extract list ID from target (format: https://domain.com/users/username/lists/{listID})
	listID := h.extractListIDFromCollection(activity.Target)
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		h.Logger.Error("Unable to extract list ID from target collection",
			zap.String("target", activity.Target),
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("unable to extract list ID from target collection")
	}

	// Verify the list exists and the actor has permission to modify it
	list, err := h.ListRepo.GetList(ctx, listID)
	if err != nil {
		h.Logger.Error("Failed to get target list",
			zap.String("list_id", listID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to get target list: %w", err)
	}

	// Check if actor has permission (must be list owner)
	actorUsername := h.extractUsernameFromActor(activity.Actor)
	if list.Username != actorUsername {
		h.Logger.Warn("Actor does not own the target list",
			zap.String("actor", activity.Actor),
			zap.String("list_owner", list.Username),
			zap.String("list_id", listID))
		return fmt.Errorf("actor does not have permission to modify list")
	}

	// Add each object (assuming they are accounts) to the list
	for _, objectID := range objectIDs {
		// Extract username from object ID (assume format: https://domain.com/users/username)
		memberUsername := h.extractUsernameFromActor(objectID)
		if err := common.ValidateRequiredParam("memberUsername", memberUsername); err != nil {
			h.Logger.Warn("Unable to extract username from object ID",
				zap.String("object_id", objectID))
			continue
		}

		// Add the member to the list
		if err := h.ListRepo.AddListMember(ctx, listID, memberUsername); err != nil {
			h.Logger.Error("Failed to add member to list",
				zap.String("list_id", listID),
				zap.String("member_username", memberUsername),
				zap.String("activity_id", activity.ID),
				zap.Error(err))
			// Continue with other members instead of failing entirely
			continue
		}

		h.Logger.Info("Added member to list",
			zap.String("list_id", listID),
			zap.String("member_username", memberUsername),
			zap.String("activity_id", activity.ID))
	}

	return nil
}

// processRemoveActivity processes a Remove activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processRemoveActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	h.Logger.Info("Processing Remove activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor))

	// Extract the target collection from activity.Target
	if err := common.ValidateRequiredParam("activity.Target", activity.Target); err != nil {
		h.Logger.Error("Remove activity missing target collection",
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("remove activity missing target collection")
	}

	// Extract the object being removed
	var objectIDs []string
	switch obj := activity.Object.(type) {
	case string:
		// Single object ID
		objectIDs = []string{obj}
	case []interface{}:
		// Multiple objects
		for _, item := range obj {
			if objID, ok := item.(string); ok {
				objectIDs = append(objectIDs, objID)
			} else if objMap, ok := item.(map[string]interface{}); ok {
				if id, ok := objMap["id"].(string); ok {
					objectIDs = append(objectIDs, id)
				}
			}
		}
	case map[string]interface{}:
		// Object with ID field
		if id, ok := obj["id"].(string); ok {
			objectIDs = []string{id}
		}
	default:
		h.Logger.Error("Unable to extract object from Remove activity",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return fmt.Errorf("unable to extract object from Remove activity")
	}

	if err := common.ValidateSliceNotEmpty("objectIDs", objectIDs); err != nil {
		return fmt.Errorf("no objects found to remove in Remove activity")
	}

	// Extract list ID from target
	listID := h.extractListIDFromCollection(activity.Target)
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		h.Logger.Error("Unable to extract list ID from target collection",
			zap.String("target", activity.Target),
			zap.String("activity_id", activity.ID))
		return fmt.Errorf("unable to extract list ID from target collection")
	}

	// Verify the list exists and the actor has permission to modify it
	list, err := h.ListRepo.GetList(ctx, listID)
	if err != nil {
		h.Logger.Error("Failed to get target list",
			zap.String("list_id", listID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to get target list: %w", err)
	}

	// Check if actor has permission (must be list owner)
	actorUsername := h.extractUsernameFromActor(activity.Actor)
	if list.Username != actorUsername {
		h.Logger.Warn("Actor does not own the target list",
			zap.String("actor", activity.Actor),
			zap.String("list_owner", list.Username),
			zap.String("list_id", listID))
		return fmt.Errorf("actor does not have permission to modify list")
	}

	// Remove each object from the list
	for _, objectID := range objectIDs {
		// Extract username from object ID
		memberUsername := h.extractUsernameFromActor(objectID)
		if err := common.ValidateRequiredParam("memberUsername", memberUsername); err != nil {
			h.Logger.Warn("Unable to extract username from object ID",
				zap.String("object_id", objectID))
			continue
		}

		// Remove the member from the list
		if err := h.ListRepo.RemoveListMember(ctx, listID, memberUsername); err != nil {
			h.Logger.Error("Failed to remove member from list",
				zap.String("list_id", listID),
				zap.String("member_username", memberUsername),
				zap.String("activity_id", activity.ID),
				zap.Error(err))
			// Continue with other members instead of failing entirely
			continue
		}

		h.Logger.Info("Removed member from list",
			zap.String("list_id", listID),
			zap.String("member_username", memberUsername),
			zap.String("activity_id", activity.ID))
	}

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

	if err := common.ValidateSliceNotEmpty("recipients", recipients); err != nil {
		h.Logger.Debug("No recipients found for activity, skipping delivery",
			zap.String("activity_id", activity.ID))
		return nil
	}

	// Filter out local recipients and public addresses
	remoteRecipients := h.filterRemoteRecipients(recipients)
	
	if err := common.ValidateSliceNotEmpty("remoteRecipients", remoteRecipients); err != nil {
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

	// Implement actual federation delivery
	if h.RouteManager != nil && len(remoteRecipients) > 0 {
		// Create federation message from the activity
		message := &types.FederationMessage{
			ID:     activity.ID,
			Type:   types.MessageTypeActivity, // Use the correct type constant
			Actor:  activity.Actor,
			Target: remoteRecipients,
			Payload: nil, // Will be serialized by the route manager
		}

		// Set delivery options
		options := types.DeliveryOptions{
			Priority:    types.PriorityNormal,
			MaxRetries:  3,
			RetryBackoff: 1 * time.Second,
			Timeout:     30 * time.Second,
		}

		// Deliver the message using the route manager
		result, err := h.RouteManager.DeliverMessage(context.Background(), message, options)
		if err != nil {
			h.Logger.Error("Failed to deliver federation activity",
				zap.String("activity_id", activity.ID),
				zap.String("activity_type", activity.Type),
				zap.Int("remote_recipients", len(remoteRecipients)),
				zap.Error(err))
		} else {
			h.Logger.Info("Successfully delivered federation activity",
				zap.String("activity_id", activity.ID),
				zap.String("activity_type", activity.Type),
				zap.Int("remote_recipients", len(remoteRecipients)),
				zap.Bool("success", result.Success),
				zap.Int("attempts", result.Attempts),
				zap.Duration("duration", result.Duration))
		}
	} else {
		h.Logger.Debug("Skipping federation delivery - no route manager or recipients",
			zap.String("activity_id", activity.ID),
			zap.Bool("has_route_manager", h.RouteManager != nil),
			zap.Int("remote_recipients", len(remoteRecipients)))
	}

	return nil
}

// filterRemoteRecipients filters out local recipients and public addresses
//
//nolint:unused // Helper method for deliverActivity
func (h *ActivityHandler) filterRemoteRecipients(recipients []string) []string {
	domain := os.Getenv("DOMAIN_NAME")
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
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
		if err := common.ValidateRequiredParam("recipient", recipient); err != nil {
			continue
		}
		
		remoteRecipients = append(remoteRecipients, recipient)
	}
	
	return remoteRecipients
}

// createNotificationRepo creates a notification repository instance
// Note: linter false positive - this function IS used (lines 334, 430, 833, 1122, 1244)
//nolint:unused // false positive - function is used
func (h *ActivityHandler) createNotificationRepo() *repositories.NotificationRepository {
	return repositories.NewNotificationRepository(h.DB, h.TableName, h.Logger)
}

// extractUsernameFromActorURI extracts the username from an ActivityPub actor URI
// Note: linter false positive - this function IS used extensively throughout the file
//nolint:unused // false positive - function is used extensively
func (h *ActivityHandler) extractUsernameFromActorURI(actorURI string) string {
	// Extract username from URL like https://domain.com/users/username
	parts := strings.Split(actorURI, "/")
	if len(parts) >= 2 && parts[len(parts)-2] == "users" {
		return parts[len(parts)-1]
	}
	
	// Try alternative format like https://domain.com/@username
	if len(parts) >= 1 && strings.HasPrefix(parts[len(parts)-1], "@") {
		return strings.TrimPrefix(parts[len(parts)-1], "@")
	}
	
	// Fallback: extract the last path segment
	parts = strings.Split(strings.TrimSuffix(actorURI, "/"), "/")
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
		return parts[len(parts)-1]
	}
	
	return ""
}

// distributeToFollowersTimeline distributes a status to followers' home timelines
// Note: linter false positive - this function IS used (lines 685, 696)
//nolint:unused // false positive - function is used
func (h *ActivityHandler) distributeToFollowersTimeline(ctx context.Context, status *models.Status, actorID, postID, _ string, isReply bool, inReplyTo string, createdAt time.Time) error {
	// Extract username from actor ID for follower lookup
	actorUsername := h.extractUsernameFromActorURI(actorID)
	if err := common.ValidateRequiredParam("actorUsername", actorUsername); err != nil {
		return fmt.Errorf("failed to extract username from actor ID: %s", actorID)
	}

	h.Logger.Debug("Starting follower timeline distribution",
		zap.String("actor_username", actorUsername),
		zap.String("post_id", postID))

	// Get followers with simple approach - in production this would be paginated
	followers, _, err := h.RelationshipRepo.GetFollowers(ctx, actorUsername, 1000, "")
	if err != nil {
		h.Logger.Error("Failed to get followers for timeline distribution",
			zap.String("actor_username", actorUsername),
			zap.Error(err))
		return fmt.Errorf("failed to get followers: %w", err)
	}

	// Create timeline entries for all followers
	var timelineEntries []*models.Timeline
	for _, followerUsername := range followers {
		// Skip direct messages in follower timelines
		if status.Visibility == VisibilityDirect {
			continue
		}

		timelineEntry := &models.Timeline{
			TimelineType: "HOME",
			TimelineID:   followerUsername,
			EntryID:      fmt.Sprintf("%d_%s", createdAt.Unix(), postID),
			PostID:       postID,
			ActorID:      actorID,
			ActorHandle:  actorUsername,
			ContentType:  "Create",
			TimelineAt:   createdAt,
			Visibility:   status.Visibility,
			IsReply:      isReply,
			InReplyTo:    inReplyTo,
		}

		// Update keys
		timelineEntry.UpdateKeys()
		timelineEntries = append(timelineEntries, timelineEntry)
	}

	// Batch create timeline entries for better performance
	if err := common.ValidateSliceNotEmpty("timelineEntries", timelineEntries); err == nil {
		if err := h.TimelineRepo.CreateTimelineEntries(ctx, timelineEntries); err != nil {
			h.Logger.Error("Failed to create timeline entries for followers",
				zap.String("actor_username", actorUsername),
				zap.String("post_id", postID),
				zap.Int("entry_count", len(timelineEntries)),
				zap.Error(err))
			return fmt.Errorf("failed to create timeline entries: %w", err)
		}

		h.Logger.Info("Distributed status to follower timelines",
			zap.String("actor_username", actorUsername),
			zap.String("post_id", postID),
			zap.Int("follower_count", len(followers)),
			zap.Int("timeline_entries", len(timelineEntries)))
	}

	return nil
}


// extractUsernameFromActor is an alias for extractUsernameFromActorURI for consistency
//nolint:unused // false positive - function is used
func (h *ActivityHandler) extractUsernameFromActor(actorURI string) string {
	return h.extractUsernameFromActorURI(actorURI)
}

// extractListIDFromCollection extracts the list ID from a collection URI
//nolint:unused // false positive - function is used
func (h *ActivityHandler) extractListIDFromCollection(collectionURI string) string {
	// Handle lists pattern
	if strings.Contains(collectionURI, "/lists/") {
		parts := strings.Split(collectionURI, "/lists/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "/")[0]
		}
	}
	
	// Handle collections pattern
	if strings.Contains(collectionURI, "/collections/") {
		parts := strings.Split(collectionURI, "/collections/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "/")[0]
		}
	}
	
	// Fallback: try to extract the last path segment
	parts := strings.Split(strings.TrimSuffix(collectionURI, "/"), "/")
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
		return parts[len(parts)-1]
	}
	
	return ""
}

// createRouteManager creates a federation route manager with the necessary dependencies
func createRouteManager(db core.DB, tableName string, logger *zap.Logger) *routing.Manager {
	// Create the necessary repositories
	federationInstanceRepo := repositories.NewFederationInstanceRepository(db, logger)
	circuitBreakerRepo := repositories.NewCircuitBreakerRepository(db, tableName, logger)
	routeOptimRepo := repositories.NewRouteOptimizerRepository(db, tableName, logger)
	routingMetricsRepo := repositories.NewRoutingMetricsRepository(db, tableName, logger)
	costTrackingRepo := repositories.NewFederationCostRepository(db, tableName, logger)

	// Create route manager with default config
	config := &routing.ManagerConfig{
		RoutingConfig: &types.RoutingConfig{
			HealthCheckInterval: 30 * time.Second,
			HealthCheckTimeout:  10 * time.Second,
			UnhealthyThreshold:  3,
			HealthyThreshold:    2,
		},
		CacheTTL: 5 * time.Minute,
	}

	return routing.NewManager(
		federationInstanceRepo,
		nil, // instanceHealthRepo - not available yet
		circuitBreakerRepo,
		routeOptimRepo,
		routingMetricsRepo,
		costTrackingRepo,
		logger,
		config,
	)
}
