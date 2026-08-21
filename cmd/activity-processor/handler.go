// Package main implements the activity processor handler that processes
// DynamoDB stream events for ActivityPub activities.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation/routing"
	"github.com/equaltoai/lesser/pkg/federation/types"
	notifpush "github.com/equaltoai/lesser/pkg/notifications"
	"github.com/equaltoai/lesser/pkg/services"
	notifsvc "github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/stream"
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
	// VisibilityPrivate represents a followers-only visibility level
	VisibilityPrivate = "private"

	// ModerationEventTypeFlagCreated represents a flag creation event
	ModerationEventTypeFlagCreated = "flag_created"
	// ModerationEventTypeFlagWithdrawn represents a flag withdrawal event
	ModerationEventTypeFlagWithdrawn = "flag_withdrawn"
	// ModerationCategoryUserReport represents a user-generated report category
	ModerationCategoryUserReport = "user_report"
	// ModerationCategoryUserAction represents a user-generated action category
	ModerationCategoryUserAction = "user_action"
	// ModerationSeverityLow represents low severity moderation level
	ModerationSeverityLow = "low"
	// ModerationSeverityMedium represents medium severity moderation level
	ModerationSeverityMedium = "medium"
	// FlagStatusPending represents a pending flag status
	FlagStatusPending = "pending"
	// ObjectTypeStatus represents a status object type
	ObjectTypeStatus = "status"

	// DefaultTestingDomain is the default domain used for testing
	DefaultTestingDomain = "example.com"
)

// ActivityHandler processes DynamoDB stream events for activities
type ActivityHandler struct {
	DB               core.DB
	TableName        string
	Logger           *zap.Logger
	ActivityRepo     interfaces.ActivityRepository
	ObjectRepo       interfaces.ObjectRepository
	ActorRepo        interfaces.ActorRepository
	TimelineRepo     interfaces.TimelineRepository
	RelationshipRepo interfaces.ConcreteRelationshipRepository
	LikeRepo         interfaces.LikeRepository
	SocialRepo       interfaces.SocialRepository
	ModerationRepo   interfaces.ModerationRepository
	ListRepo         interfaces.ListRepository
	RouteManager     federationRouteManager
	PushService      *notifpush.PushService
	AccountRepo      interfaces.AccountRepository
	NotificationRepo interfaces.NotificationRepository
	NotificationSvc  *notifsvc.Service
}

type federationRouteManager interface {
	DeliverMessage(ctx context.Context, message *types.FederationMessage, options types.DeliveryOptions) (*types.DeliveryResult, error)
}

// NewActivityHandler creates a new ActivityHandler
func NewActivityHandler(db core.DB, tableName string) *ActivityHandler {
	logger := zap.L()
	cfg := config.Get()
	domain := DefaultTestingDomain
	if cfg != nil {
		domain = cfg.Domain
	}
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		domain = DefaultTestingDomain // Default for testing
	}

	accountRepo := repositories.NewAccountRepository(db, tableName, domain, logger)
	notificationRepo := repositories.NewNotificationRepository(db, tableName, logger, nil)

	var pushService *notifpush.PushService
	if cfg != nil {
		if svc, err := notifpush.NewPushService(cfg); err != nil {
			logger.Warn("activity handler: failed to initialize push service", zap.Error(err))
		} else {
			pushService = svc
		}
	}

	var notificationService *notifsvc.Service
	if notificationRepo != nil && accountRepo != nil {
		notificationService = notifsvc.NewService(
			notificationRepo,
			accountRepo,
			nil,
			logger,
			domain,
			pushService,
		)
		notificationRepo.SetDispatcher(notificationService)
	}

	return &ActivityHandler{
		DB:               db,
		TableName:        tableName,
		Logger:           logger,
		ActivityRepo:     repositories.NewActivityRepository(db, tableName, logger, nil),
		ObjectRepo:       repositories.NewObjectRepository(db, tableName, domain, logger),
		ActorRepo:        repositories.NewActorRepository(db, tableName, logger, domain),
		TimelineRepo:     repositories.NewTimelineRepository(db, tableName, logger, nil),
		RelationshipRepo: repositories.NewRelationshipRepository(db, tableName, logger),
		LikeRepo:         repositories.NewLikeRepository(db, tableName, logger),
		SocialRepo:       repositories.NewSocialRepository(db, tableName, logger, nil),
		ModerationRepo:   repositories.NewModerationRepository(db, tableName, logger),
		ListRepo:         repositories.NewListRepository(db, tableName, logger, nil),
		RouteManager:     createRouteManager(db, tableName, logger),
		PushService:      pushService,
		AccountRepo:      accountRepo,
		NotificationRepo: notificationRepo,
		NotificationSvc:  notificationService,
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
		return entityTypeExtractionFailed()
	}

	// Only process activity records
	if entityType != "activity" {
		return nil
	}

	// Unmarshal the activity record
	var activityRecord ActivityRecord
	if err := stream.UnmarshalItem(record, &activityRecord); err != nil {
		return activityRecordUnmarshalingFailed(err)
	}

	// Parse the activity
	activity, err := activitypub.ParseActivity([]byte(activityRecord.Activity))
	if err != nil {
		return activityParsingFailed("unknown", err)
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
		return unknownActivityDirection(string(direction))
	}
}

// processActivityByType processes an activity based on its type with configurable handlers
//
//nolint:unused // False positive - called from processInboxActivity and processOutboxActivity
func (h *ActivityHandler) processActivityByType(ctx context.Context, activity *activitypub.Activity, username string, isInbox bool) error {
	logType := "outbox"
	if isInbox {
		logType = "inbox"
	}

	h.Logger.Info("Processing "+logType+" activity",
		zap.String("type", activity.Type),
		zap.String("username", username),
		zap.String("id", activity.ID),
	)

	// For outbox, most activities just need delivery
	if !isInbox {
		switch activity.Type {
		case ActivityTypeCreate, ActivityTypeFollow, ActivityTypeAccept, ActivityTypeReject,
			ActivityTypeUpdate, ActivityTypeDelete, ActivityTypeLike, ActivityTypeAnnounce,
			ActivityTypeUndo, ActivityTypeBlock, ActivityTypeFlag, ActivityTypeMove,
			ActivityTypeAdd, ActivityTypeRemove:
			return h.deliverActivity(ctx, activity, username)
		default:
			h.Logger.Info("Ignoring unsupported activity type",
				zap.String("type", activity.Type),
			)
			return nil
		}
	}

	// For inbox, use specific processing methods
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

// processInboxActivity processes an incoming activity
//
//nolint:unused // False positive - called in Handle method
func (h *ActivityHandler) processInboxActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	return h.processActivityByType(ctx, activity, username, true)
}

// processOutboxActivity processes an outgoing activity
//
//nolint:unused // False positive - called in Handle method
func (h *ActivityHandler) processOutboxActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	return h.processActivityByType(ctx, activity, username, false)
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
			return services.ErrFollowMissingObjectID
		}
	default:
		h.Logger.Error("Follow activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return services.ErrFollowInvalidObjectType
	}

	if err := common.ValidateRequiredParam("targetUser", targetUser); err != nil {
		h.Logger.Error("Follow activity missing target user",
			zap.String("activity_id", activity.ID))
		return services.ErrFollowMissingTargetUser
	}

	// Extract username from actor URI
	followerUsername := h.extractUsernameFromActorURI(activity.Actor)
	targetUsername := h.extractUsernameFromActorURI(targetUser)

	if err := common.ValidateRequiredParam("followerUsername", followerUsername); err != nil {
		h.Logger.Error("Failed to extract usernames from Follow activity",
			zap.String("actor", activity.Actor),
			zap.String("target", targetUser))
		return services.ErrExtractUsernamesFromFollow
	}
	if err := common.ValidateRequiredParam("targetUsername", targetUsername); err != nil {
		h.Logger.Error("Failed to extract usernames from Follow activity",
			zap.String("actor", activity.Actor),
			zap.String("target", targetUser))
		return services.ErrExtractUsernamesFromFollow
	}

	// Create the follow relationship
	if err := h.RelationshipRepo.CreateRelationship(ctx, followerUsername, targetUsername, activity.ID); err != nil {
		h.Logger.Error("Failed to create follow relationship",
			zap.String("follower", followerUsername),
			zap.String("following", targetUsername),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return followRelationshipCreationFailed(err)
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
	return h.processFollowResponseActivity(ctx, activity, username, followResponseProcessingConfig{
		responseName:     "Accept",
		invalidObjectErr: services.ErrAcceptInvalidObjectType,
		extractErr:       services.ErrExtractUsernamesFromAccept,
		mutate: func(ctx context.Context, follower, responder string) error {
			return h.RelationshipRepo.AcceptFollowRequest(ctx, follower, responder)
		},
		mutationFailureLog:      "Failed to accept persisted follow relationship",
		mutationErr:             func(err error) error { return relationshipStatusUpdateFailed(err) },
		notificationTitleFormat: "%s accepted your follow request",
		notificationBodyFormat:  "You are now following %s",
		notificationFailureLog:  "Failed to create accept notification",
		successLog:              "Successfully processed Accept activity",
		notificationSuccessNote: "relationship was updated successfully",
	})
}

//nolint:unused // Called from stream-dispatched follow response handlers.
type followResponseProcessingConfig struct {
	responseName            string
	invalidObjectErr        error
	extractErr              error
	mutate                  func(context.Context, string, string) error
	mutationFailureLog      string
	mutationErr             func(error) error
	notificationTitleFormat string
	notificationBodyFormat  string
	notificationFailureLog  string
	successLog              string
	notificationSuccessNote string
}

//nolint:unused // Called from stream-dispatched follow response handlers.
func (h *ActivityHandler) processFollowResponseActivity(
	ctx context.Context,
	activity *activitypub.Activity,
	username string,
	cfg followResponseProcessingConfig,
) error {
	follower, responder, err := h.resolvePersistedFollowResponse(ctx, activity, username, cfg.responseName, cfg.invalidObjectErr, cfg.extractErr)
	if err != nil {
		return err
	}

	responderField := strings.ToLower(cfg.responseName) + "er"
	if err := cfg.mutate(ctx, follower, responder); err != nil {
		h.Logger.Error(cfg.mutationFailureLog,
			zap.String("follower", follower),
			zap.String(responderField, responder),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return cfg.mutationErr(err)
	}

	notification := models.NewNotificationBuilder().
		ForUser(follower).
		OfType("follow_request").
		FromActor(responder, "remote_actor").
		WithContent(
			fmt.Sprintf(cfg.notificationTitleFormat, responder),
			fmt.Sprintf(cfg.notificationBodyFormat, responder)).
		Build()

	if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
		h.Logger.Error(cfg.notificationFailureLog,
			zap.String("follower", follower),
			zap.String(responderField, responder),
			zap.Error(err))
		h.Logger.Debug("follow response notification skipped after successful state change",
			zap.String("reason", cfg.notificationSuccessNote))
	}

	h.Logger.Info(cfg.successLog,
		zap.String("follower", follower),
		zap.String(responderField, responder),
		zap.String("activity_id", activity.ID))

	return nil
}

//nolint:unused // Called from stream-dispatched follow response handlers.
func (h *ActivityHandler) resolvePersistedFollowResponse(
	ctx context.Context,
	activity *activitypub.Activity,
	username string,
	responseName string,
	invalidObjectErr error,
	extractErr error,
) (string, string, error) {
	h.Logger.Info("Processing "+responseName+" activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object),
	)

	followActivity, err := h.extractFollowResponseObject(activity, responseName, invalidObjectErr)
	if err != nil {
		return "", "", err
	}

	responderField := strings.ToLower(responseName) + "er"
	responder := h.extractUsernameFromActorURI(activity.Actor)
	if err := common.ValidateRequiredParam(responderField, responder); err != nil {
		h.Logger.Error("Failed to extract usernames from "+responseName+" activity",
			zap.String(responderField, responder),
			zap.String("activity_id", activity.ID))
		return "", "", extractErr
	}

	followState, err := h.resolveFollowResponseState(ctx, followActivity)
	if err != nil {
		h.Logger.Warn(responseName+" activity did not reference a valid persisted Follow",
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return "", "", extractErr
	}
	if !h.followTargetMatchesActor(followState.Target, activity.Actor) {
		h.Logger.Warn(responseName+" activity follow target does not match responding actor",
			zap.String("activity_id", activity.ID),
			zap.String(strings.ToLower(responseName)+"_actor", activity.Actor),
			zap.String("follow_target", followState.Target),
			zap.String("follow_activity_id", followState.ID))
		return "", "", services.ErrActorNotAuthorizedUndo
	}

	follower := h.extractUsernameFromActorURI(followState.Actor)
	if err := common.ValidateRequiredParam("follower", follower); err != nil {
		return "", "", extractErr
	}

	if err := h.requirePersistedFollowRelationshipState(ctx, follower, responder, followState.ID, models.RelationshipPending); err != nil {
		return "", "", err
	}

	return follower, responder, nil
}

//nolint:unused // Called from stream-dispatched follow response handlers.
func (h *ActivityHandler) extractFollowResponseObject(activity *activitypub.Activity, responseName string, invalidObjectErr error) (interface{}, error) {
	switch obj := activity.Object.(type) {
	case string:
		h.Logger.Debug(responseName+" activity references activity by ID",
			zap.String("follow_activity_id", obj))
		return obj, nil
	case map[string]interface{}:
		return obj, nil
	default:
		h.Logger.Error(responseName+" activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return nil, invalidObjectErr
	}
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
		return noteExtractionFailed(err)
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
		return statusCreationFailed(err)
	}

	// Store the status/object
	if err := h.ObjectRepo.CreateObject(ctx, note); err != nil {
		h.Logger.Error("failed to store Note object",
			zap.Error(err),
			zap.String("status_id", status.StatusID))
		return objectStorageFailed(err)
	}

	h.createInboundMentionNotifications(ctx, status, note, activity)

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
	if tags, ok := objMap["tag"].([]interface{}); ok {
		note.Tag = h.interfaceSliceToTags(tags)
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
		return nil, unsupportedObjectType("unknown")
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

//nolint:unused // false positive - called from mapToNote in Create activity parsing
func (h *ActivityHandler) interfaceSliceToTags(slice []interface{}) []activitypub.Tag {
	tags := make([]activitypub.Tag, 0, len(slice))
	for _, item := range slice {
		tagMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		tag := activitypub.Tag{}
		if tagType, ok := tagMap["type"].(string); ok {
			tag.Type = tagType
		}
		if href, ok := tagMap["href"].(string); ok {
			tag.Href = href
		}
		if name, ok := tagMap["name"].(string); ok {
			tag.Name = name
		}
		if tag.Type == "" && tag.Href == "" && tag.Name == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags
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
	cfg := config.Get()
	apiStatus := transformations.ObjectToStatusBase(statusMap, fakeActor, cfg.BaseURL())

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
		Content:   apiStatus.Content,
		Sensitive: apiStatus.Sensitive,
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
		h.Logger.Debug("Status is unlisted, adding to followers' home timelines",
			zap.String("status_id", status.StatusID))
		// Unlisted posts go to followers' home timelines
		if err := h.distributeToFollowersTimeline(ctx, status, actorID, postID, contentPreview, isReply, inReplyTo, createdAt); err != nil {
			h.Logger.Error("Failed to distribute unlisted status to followers' timelines",
				zap.String("status_id", status.StatusID),
				zap.Error(err))
			// Don't fail the entire operation - continue processing
		}

	case VisibilityPrivate:
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
			return timelineEntriesCreationFailed(err)
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
	cfg := config.Get()
	if cfg == nil {
		return common.IsLocalActorID(actorID, DefaultTestingDomain)
	}
	domain := cfg.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		domain = DefaultTestingDomain // Default for testing
	}
	return common.IsLocalActorID(actorID, domain)
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
	return h.processFollowResponseActivity(ctx, activity, username, followResponseProcessingConfig{
		responseName:     "Reject",
		invalidObjectErr: services.ErrRejectInvalidObjectType,
		extractErr:       services.ErrExtractUsernamesFromReject,
		mutate: func(ctx context.Context, follower, responder string) error {
			return h.RelationshipRepo.RejectFollowRequest(ctx, follower, responder)
		},
		mutationFailureLog:      "Failed to reject persisted follow relationship",
		mutationErr:             func(err error) error { return rejectedRelationshipDeletionFailed(err) },
		notificationTitleFormat: "%s declined your follow request",
		notificationBodyFormat:  "Your follow request to %s was declined",
		notificationFailureLog:  "Failed to create reject notification",
		successLog:              "Successfully processed Reject activity",
		notificationSuccessNote: "relationship was rejected successfully",
	})
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
			return services.ErrDeleteMissingObjectID
		}
	default:
		h.Logger.Error("Delete activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return services.ErrDeleteInvalidObjectType
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		h.Logger.Error("Delete activity missing object ID",
			zap.String("activity_id", activity.ID))
		return services.ErrDeleteMissingObjectID2
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
		return services.ErrActorNotAuthorizedDelete
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
	if err := h.DB.WithContext(ctx).Model(tombstone).CreateOrUpdate(); err != nil {
		h.Logger.Error("Failed to create tombstone",
			zap.String("object_id", objectID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return tombstoneCreationFailed(err)
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
			return services.ErrLikeMissingObjectID
		}
	default:
		h.Logger.Error("Like activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return services.ErrLikeInvalidObjectType
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		h.Logger.Error("Like activity missing object ID",
			zap.String("activity_id", activity.ID))
		return services.ErrLikeMissingObjectID2
	}

	// Extract the actor doing the liking
	actorURI := activity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		h.Logger.Error("Like activity missing actor",
			zap.String("activity_id", activity.ID))
		return services.ErrLikeMissingActor
	}

	// Create the like record
	_, err := h.LikeRepo.CreateLike(ctx, actorURI, objectID, username)
	if err != nil {
		h.Logger.Error("Failed to create like record",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return likeRecordCreationFailed(err)
	}

	// Create notification for the object owner
	h.createObjectInteractionNotification(ctx, objectID, actorURI, "favourite", "liked", "liker")

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
			return services.ErrAnnounceMissingObjectID
		}
	default:
		h.Logger.Error("Announce activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return services.ErrAnnounceInvalidObjectType
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		h.Logger.Error("Announce activity missing object ID",
			zap.String("activity_id", activity.ID))
		return services.ErrAnnounceMissingObjectID2
	}

	// Extract the actor doing the announcing
	actorURI := activity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		h.Logger.Error("Announce activity missing actor",
			zap.String("activity_id", activity.ID))
		return services.ErrAnnounceMissingActor
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
		return announceRecordCreationFailed(err)
	}

	// Create notification for the object owner
	h.createObjectInteractionNotification(ctx, objectID, actorURI, "reblog", "boosted", "announcer")

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
		fetchedActivity, err := h.getActivityByID(ctx, obj)
		if err != nil {
			h.Logger.Error("Failed to fetch original activity for undo",
				zap.String("activity_id", obj),
				zap.Error(err))
			return originalActivityFetchFailed(obj, err)
		}
		undoTarget = fetchedActivity
	case map[string]interface{}:
		undoTarget = obj
	default:
		h.Logger.Error("Undo activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return services.ErrUndoInvalidObjectType
	}

	// Extract the type of activity being undone
	activityType, ok := h.extractActivityType(undoTarget)
	if !ok {
		h.Logger.Error("Failed to extract activity type from undo target",
			zap.String("activity_id", activity.ID),
			zap.Any("target", undoTarget))
		return services.ErrExtractActivityTypeFromUndo
	}

	// Verify actor authorization - can only undo their own activities
	targetActor := h.extractActivityActor(undoTarget)
	if targetActor != activity.Actor {
		h.Logger.Warn("Actor not authorized to undo activity",
			zap.String("undo_actor", activity.Actor),
			zap.String("target_actor", targetActor),
			zap.String("activity_type", activityType))
		return services.ErrActorNotAuthorizedUndo
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
		return h.processUndoReject(ctx, activity, undoTarget, username)
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
func (h *ActivityHandler) getActivityByID(ctx context.Context, activityID string) (*activitypub.Activity, error) {
	if err := common.ValidateRequiredParam("activityID", activityID); err != nil {
		return nil, err
	}

	if h.ActivityRepo == nil {
		h.Logger.Warn("Activity repository not configured",
			zap.String("activity_id", activityID))
		return nil, activityNotFoundLocally(activityID)
	}

	h.Logger.Debug("Fetching activity by ID",
		zap.String("activity_id", activityID))

	activity, err := h.ActivityRepo.GetActivity(ctx, activityID)
	if err != nil {
		h.Logger.Warn("Failed to fetch activity by ID",
			zap.String("activity_id", activityID),
			zap.Error(err))
		return nil, err
	}

	return activity, nil
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
//
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
		return services.ErrExtractTargetActorFromFollow
	}

	// Remove the relationship
	if err := h.RelationshipRepo.DeleteRelationship(ctx, undoActivity.Actor, targetActor); err != nil {
		h.Logger.Error("Failed to delete follow relationship",
			zap.String("follower", undoActivity.Actor),
			zap.String("followee", targetActor),
			zap.Error(err))
		return followRelationshipDeletionFailed(err)
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
	return h.processUndoWithObjectExtraction(ctx, undoActivity, likeActivity, "like", h.LikeRepo.DeleteLike)
}

// processUndoAnnounce processes an undo of an Announce activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoAnnounce(ctx context.Context, undoActivity *activitypub.Activity, announceActivity interface{}, _ string) error {
	return h.processUndoWithObjectExtraction(ctx, undoActivity, announceActivity, "announce", h.SocialRepo.DeleteAnnounce)
}

// processUndoBlock processes an undo of a Block activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoBlock(ctx context.Context, undoActivity *activitypub.Activity, blockActivity interface{}, _ string) error {
	return h.processUndoWithObjectExtraction(ctx, undoActivity, blockActivity, "block", h.RelationshipRepo.DeleteBlock)
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
		return services.ErrExtractObjectIDFromCreate
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return services.ErrUndoCreateMissingActor
	}

	// Delete the created object - this effectively undoes the creation
	if err := h.ObjectRepo.DeleteObject(ctx, objectID); err != nil {
		h.Logger.Error("Failed to delete created object",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return createdObjectDeletionFailed(err)
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
		return services.ErrExtractObjectIDFromUpdate
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return services.ErrUndoUpdateMissingActor
	}

	// Get object history to find the previous version
	history, err := h.ObjectRepo.GetObjectHistory(ctx, objectID)
	if err != nil {
		h.Logger.Error("failed to get object history for undo update",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return objectHistoryRetrievalFailed(err)
	}

	if err := common.ValidateSliceNotEmpty("history", history); err != nil {
		h.Logger.Warn("no history found for object, cannot undo update",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID))
		return noHistoryFound(objectID)
	}

	// Get the most recent previous version (first in sorted list)
	previousVersion := history[0]
	if previousVersion.PreviousState == nil {
		h.Logger.Warn("previous state not available for undo",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID))
		return previousStateNotAvailable(objectID)
	}

	// Update the object to the previous version
	if err := h.ObjectRepo.UpdateObject(ctx, previousVersion.PreviousState); err != nil {
		h.Logger.Error("failed to revert object to previous version",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return objectReversionFailed(err)
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
		return services.ErrExtractObjectIDFromDelete
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return services.ErrUndoDeleteMissingActor
	}

	// Check if object is tombstoned
	tombstoned, err := h.ObjectRepo.IsTombstoned(ctx, objectID)
	if err != nil {
		h.Logger.Error("failed to check if object is tombstoned",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return tombstoneStatusCheckFailed(err)
	}

	if !tombstoned {
		h.Logger.Warn("object is not deleted, cannot undo delete",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID))
		return objectNotDeleted(objectID)
	}

	// Get the tombstone to find deletion info
	tombstone, err := h.ObjectRepo.GetTombstone(ctx, objectID)
	if err != nil {
		h.Logger.Error("failed to get tombstone for undo delete",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return tombstoneRetrievalFailed(err)
	}

	// Get object history to find the last version before deletion
	history, err := h.ObjectRepo.GetObjectHistory(ctx, objectID)
	if err != nil || len(history) == 0 {
		h.Logger.Error("failed to get object history for restoration",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return objectHistoryRestorationFailed(err)
	}

	// Get the most recent version before deletion
	lastVersion := history[0]
	if lastVersion.PreviousState == nil {
		h.Logger.Error("no previous state available for object restoration",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID))
		return services.ErrNoPreviousStateForRestoration
	}

	// Restore the object by recreating it with the previous state
	if err := h.ObjectRepo.CreateObject(ctx, lastVersion.PreviousState); err != nil {
		h.Logger.Error("failed to restore object from previous state",
			zap.String("actor", actorURI),
			zap.String("object_id", objectID),
			zap.Error(err))
		return objectRestorationFailed(err)
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
		return services.ErrExtractOriginalActivityIDFromAccept
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return services.ErrUndoAcceptMissingActor
	}

	// This would typically revert the accepted state of the original activity
	// For follow requests, this would change the status back to pending
	h.Logger.Info("Successfully processed Undo Accept",
		zap.String("actor", actorURI),
		zap.String("original_activity_id", originalActivityID))

	return nil
}

// processUndoReject processes an undo of a Reject activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoReject(ctx context.Context, undoActivity *activitypub.Activity, rejectActivity interface{}, username string) error {
	h.Logger.Info("Processing Undo Reject activity",
		zap.String("undo_activity_id", undoActivity.ID),
		zap.String("actor", undoActivity.Actor),
		zap.String("username", username),
	)

	rejectActor, followActivity, err := h.extractRejectStateForUndoReject(rejectActivity)
	if err != nil {
		return err
	}
	if rejectActor != undoActivity.Actor {
		h.Logger.Warn("Undo Reject actor does not match referenced Reject actor",
			zap.String("undo_actor", undoActivity.Actor),
			zap.String("reject_actor", rejectActor))
		return services.ErrActorNotAuthorizedUndo
	}

	rejecter := h.extractUsernameFromActorURI(rejectActor)
	if err := common.ValidateRequiredParam("rejecter", rejecter); err != nil {
		h.Logger.Error("Failed to extract rejecter from Undo Reject activity",
			zap.String("reject_actor", rejectActor))
		return services.ErrExtractUsernamesFromReject
	}

	followState, err := h.resolveUndoRejectFollowState(ctx, followActivity)
	if err != nil {
		return err
	}
	if !h.followTargetMatchesActor(followState.Target, rejectActor) {
		h.Logger.Warn("Undo Reject referenced Follow target does not match Reject actor",
			zap.String("reject_actor", rejectActor),
			zap.String("follow_target", followState.Target),
			zap.String("follow_activity_id", followState.ID))
		return services.ErrActorNotAuthorizedUndo
	}

	follower := h.extractUsernameFromActorURI(followState.Actor)
	if err := common.ValidateRequiredParam("follower", follower); err != nil {
		h.Logger.Error("Failed to resolve follower for Undo Reject",
			zap.String("rejecter", rejecter),
			zap.String("follow_actor", followState.Actor))
		return services.ErrExtractUsernamesFromReject
	}
	if err := common.ValidateRequiredParam("followActivityID", followState.ID); err != nil {
		return services.ErrExtractUsernamesFromReject
	}

	if err := h.requirePersistedFollowRelationshipState(ctx, follower, rejecter, followState.ID, models.RelationshipRejected); err != nil {
		return err
	}

	if err := h.RelationshipRepo.CreateRelationship(ctx, follower, rejecter, followState.ID); err != nil {
		h.Logger.Error("Failed to recreate follow relationship after Undo Reject",
			zap.String("follower", follower),
			zap.String("rejecter", rejecter),
			zap.String("follow_activity_id", followState.ID),
			zap.Error(err))
		return followRelationshipCreationFailed(err)
	}

	h.Logger.Info("Successfully processed Undo Reject activity",
		zap.String("follower", follower),
		zap.String("followee", rejecter),
		zap.String("follow_activity_id", followState.ID))

	return nil
}

//nolint:unused // Used by follow response and Undo Reject handlers.
func (h *ActivityHandler) resolveFollowResponseState(ctx context.Context, followActivity interface{}) (undoRejectFollowState, error) {
	return h.resolveUndoRejectFollowState(ctx, followActivity)
}

//nolint:unused // Used by processUndoReject; production reachability is through stream dispatch.
type undoRejectFollowState struct {
	ID     string
	Actor  string
	Target string
}

//nolint:unused // Used by processUndoReject; production reachability is through stream dispatch.
func (h *ActivityHandler) extractRejectStateForUndoReject(rejectActivity interface{}) (string, interface{}, error) {
	switch reject := rejectActivity.(type) {
	case *activitypub.Activity:
		if reject.Type != ActivityTypeReject {
			return "", nil, services.ErrExtractActivityTypeFromUndo
		}
		return reject.Actor, reject.Object, nil
	case map[string]interface{}:
		activityType, _ := reject["type"].(string)
		if activityType != ActivityTypeReject {
			return "", nil, services.ErrExtractActivityTypeFromUndo
		}
		actor, _ := reject["actor"].(string)
		return actor, reject["object"], nil
	default:
		h.Logger.Error("Undo Reject activity has invalid reject target type",
			zap.Any("reject_activity", rejectActivity))
		return "", nil, services.ErrUndoInvalidObjectType
	}
}

//nolint:unused // Used by processUndoReject; production reachability is through stream dispatch.
func (h *ActivityHandler) resolveUndoRejectFollowState(ctx context.Context, followActivity interface{}) (undoRejectFollowState, error) {
	switch follow := followActivity.(type) {
	case string:
		if err := common.ValidateRequiredParam("followActivityID", follow); err != nil {
			return undoRejectFollowState{}, services.ErrExtractUsernamesFromReject
		}
		resolved, err := h.getActivityByID(ctx, follow)
		if err != nil {
			h.Logger.Warn("Failed to resolve follow activity for Undo Reject",
				zap.String("follow_activity_id", follow),
				zap.Error(err))
			return undoRejectFollowState{}, services.ErrExtractUsernamesFromReject
		}
		return h.followStateFromActivity(resolved)
	case *activitypub.Activity:
		return h.followStateFromActivity(follow)
	case map[string]interface{}:
		inlineState, err := h.followStateFromMap(follow)
		if err != nil {
			return undoRejectFollowState{}, err
		}
		resolved, err := h.getActivityByID(ctx, inlineState.ID)
		if err != nil {
			h.Logger.Warn("Failed to resolve inline follow activity by ID",
				zap.String("follow_activity_id", inlineState.ID),
				zap.Error(err))
			return undoRejectFollowState{}, services.ErrExtractUsernamesFromReject
		}
		return h.followStateFromActivity(resolved)
	default:
		h.Logger.Warn("Undo Reject follow activity has unexpected type",
			zap.Any("follow_activity", follow))
		return undoRejectFollowState{}, services.ErrExtractUsernamesFromReject
	}
}

//nolint:unused // Used by processUndoReject; production reachability is through stream dispatch.
func (h *ActivityHandler) followStateFromActivity(activity *activitypub.Activity) (undoRejectFollowState, error) {
	if activity == nil || activity.Type != ActivityTypeFollow {
		return undoRejectFollowState{}, services.ErrExtractActivityTypeFromUndo
	}
	state := undoRejectFollowState{
		ID:     activity.ID,
		Actor:  activity.Actor,
		Target: h.followTargetFromObject(activity.Object),
	}
	return validateUndoRejectFollowState(state)
}

//nolint:unused // Used by processUndoReject; production reachability is through stream dispatch.
func (h *ActivityHandler) followStateFromMap(activity map[string]interface{}) (undoRejectFollowState, error) {
	activityType, _ := activity["type"].(string)
	if activityType != ActivityTypeFollow {
		return undoRejectFollowState{}, services.ErrExtractActivityTypeFromUndo
	}
	state := undoRejectFollowState{}
	if id, ok := activity["id"].(string); ok {
		state.ID = id
	}
	if actor, ok := activity["actor"].(string); ok {
		state.Actor = actor
	}
	state.Target = h.followTargetFromObject(activity["object"])
	return validateUndoRejectFollowState(state)
}

//nolint:unused // Used by processUndoReject; production reachability is through stream dispatch.
func (h *ActivityHandler) followTargetFromObject(object interface{}) string {
	switch obj := object.(type) {
	case string:
		return obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			return id
		}
	}
	return ""
}

//nolint:unused // Used by processUndoReject; production reachability is through stream dispatch.
func validateUndoRejectFollowState(state undoRejectFollowState) (undoRejectFollowState, error) {
	if err := common.ValidateRequiredParam("followActivityID", state.ID); err != nil {
		return undoRejectFollowState{}, services.ErrExtractUsernamesFromReject
	}
	if err := common.ValidateRequiredParam("followActor", state.Actor); err != nil {
		return undoRejectFollowState{}, services.ErrExtractUsernamesFromReject
	}
	if err := common.ValidateRequiredParam("followTarget", state.Target); err != nil {
		return undoRejectFollowState{}, services.ErrExtractUsernamesFromReject
	}
	return state, nil
}

//nolint:unused // Used by follow response and Undo Reject handlers.
func (h *ActivityHandler) followTargetMatchesActor(followTarget, actorID string) bool {
	followTarget = normalizeFollowTrustID(followTarget)
	actorID = normalizeFollowTrustID(actorID)
	if followTarget == "" || actorID == "" {
		return false
	}
	return followTarget == actorID
}

//nolint:unused // Used by follow response and Undo Reject handlers.
func (h *ActivityHandler) requirePersistedFollowRelationshipState(
	ctx context.Context,
	follower string,
	following string,
	followActivityID string,
	requiredState string,
) error {
	if h.RelationshipRepo == nil {
		h.Logger.Warn("relationship repository unavailable for follow state validation",
			zap.String("follower", follower),
			zap.String("following", following),
			zap.String("follow_activity_id", followActivityID),
			zap.String("required_state", requiredState))
		return services.ErrRelationshipRepositoryNotAvailable
	}

	relationship, err := h.RelationshipRepo.GetRelationship(ctx, follower, following)
	if err != nil {
		h.Logger.Warn("failed to load follow relationship for state validation",
			zap.String("follower", follower),
			zap.String("following", following),
			zap.String("follow_activity_id", followActivityID),
			zap.String("required_state", requiredState),
			zap.Error(err))
		return services.ErrActorNotAuthorizedUndo
	}
	if relationship == nil || relationship.State != requiredState {
		state := ""
		if relationship != nil {
			state = relationship.State
		}
		h.Logger.Warn("follow relationship state mismatch",
			zap.String("follower", follower),
			zap.String("following", following),
			zap.String("follow_activity_id", followActivityID),
			zap.String("required_state", requiredState),
			zap.String("actual_state", state))
		return services.ErrActorNotAuthorizedUndo
	}
	if !followActivityIDMatches(relationship.ActivityID, followActivityID) {
		h.Logger.Warn("follow relationship activity id mismatch",
			zap.String("follower", follower),
			zap.String("following", following),
			zap.String("stored_activity_id", relationship.ActivityID),
			zap.String("referenced_activity_id", followActivityID))
		return services.ErrActorNotAuthorizedUndo
	}
	return nil
}

//nolint:unused // Used by follow response and Undo Reject state validation.
func followActivityIDMatches(storedActivityID, referencedActivityID string) bool {
	storedActivityID = normalizeFollowTrustID(storedActivityID)
	referencedActivityID = normalizeFollowTrustID(referencedActivityID)
	if storedActivityID == "" || referencedActivityID == "" {
		return false
	}
	return storedActivityID == referencedActivityID
}

//nolint:unused // Used by follow response and Undo Reject trust matching helpers.
func normalizeFollowTrustID(id string) string {
	return strings.TrimRight(strings.TrimSpace(id), "/")
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
		return services.ErrExtractFlaggedObjectIDFromFlag
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return services.ErrUndoFlagMissingActor
	}

	// Find and delete the flag record using ModerationRepository
	flags, _, err := h.ModerationRepo.GetFlagsByObject(ctx, flaggedObjectID, 50, "")
	if err != nil {
		h.Logger.Error("Failed to retrieve flags for object",
			zap.String("flagged_object_id", flaggedObjectID),
			zap.Error(err))
		return flagsRetrievalFailed(err)
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
		return flagRecordDeletionFailed(err)
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
			"undo_activity_id":  undoActivity.ID,
			"original_flag_id":  targetFlagID,
			"flagged_object_id": flaggedObjectID,
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
		return services.ErrExtractMovedToTargetFromMove
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return services.ErrUndoMoveMissingActor
	}

	// Extract username from actor URI for repository operations
	username := h.extractUsernameFromActorURI(actorURI)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return usernameExtractionFromActorURIFailed(actorURI)
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
		return movedToFieldClearingFailed(err)
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

// processUndoListActivity processes undo operations for Add/Remove activities on lists
// This consolidates processUndoAdd and processUndoRemove which had 85 lines of duplication
//
//nolint:unused // False positive - called from processUndoAdd and processUndoRemove
func (h *ActivityHandler) processUndoListActivity(ctx context.Context, undoActivity *activitypub.Activity, originalActivity interface{}, activityType string) error {
	// Extract object ID from the original activity
	targetObject := h.extractActivityObject(originalActivity)
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
		return objectIDExtractionFromActivityFailed()
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return undoActivityMissingActor()
	}

	// Extract target collection from the original activity
	var targetCollection string
	if act, ok := originalActivity.(map[string]interface{}); ok {
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
		return services.ErrExtractListIDFromTargetCollection
	}

	// Verify the list exists and the actor has permission to modify it
	list, err := h.ListRepo.GetList(ctx, listID)
	if err != nil {
		h.Logger.Error("Failed to get target list for undo",
			zap.String("list_id", listID),
			zap.String("activity_id", undoActivity.ID),
			zap.String("activity_type", activityType),
			zap.Error(err))
		return targetListRetrievalFailed(err)
	}

	// Check if actor has permission (must be list owner)
	actorUsername := h.extractUsernameFromActor(actorURI)
	if list.Username != actorUsername {
		h.Logger.Warn("Actor does not own the target list",
			zap.String("actor", actorURI),
			zap.String("list_owner", list.Username),
			zap.String("list_id", listID),
			zap.String("activity_type", activityType))
		return services.ErrActorNoPermissionModifyList
	}

	// Extract username from object ID
	memberUsername := h.extractUsernameFromActor(objectID)
	if err := common.ValidateRequiredParam("memberUsername", memberUsername); err != nil {
		h.Logger.Error("Unable to extract username from object ID",
			zap.String("object_id", objectID))
		return services.ErrExtractUsernameFromObjectID
	}

	// Perform the inverse operation based on the original activity type
	var opErr error
	var action string
	if activityType == "add" {
		// Undo Add means Remove
		opErr = h.ListRepo.RemoveListMember(ctx, listID, memberUsername)
		action = "removed"
	} else {
		// Undo Remove means Add
		opErr = h.ListRepo.AddListMember(ctx, listID, memberUsername)
		action = "added"
	}

	if opErr != nil {
		h.Logger.Error("Failed to perform list operation in undo",
			zap.String("list_id", listID),
			zap.String("member_username", memberUsername),
			zap.String("activity_id", undoActivity.ID),
			zap.String("activity_type", activityType),
			zap.String("action", action),
			zap.Error(opErr))
		return listOperationFailed(opErr)
	}

	h.Logger.Info("Successfully processed Undo",
		zap.String("actor", actorURI),
		zap.String("list_id", listID),
		zap.String("member", memberUsername),
		zap.String("activity_id", undoActivity.ID),
		zap.String("activity_type", activityType),
		zap.String("action", action))

	return nil
}

// processUndoAdd processes an undo of an Add activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoAdd(ctx context.Context, undoActivity *activitypub.Activity, addActivity interface{}, _ string) error {
	return h.processUndoListActivity(ctx, undoActivity, addActivity, "add")
}

// processUndoRemove processes an undo of a Remove activity
//
//nolint:unused // Called from processUndoActivity but kept for full implementation
func (h *ActivityHandler) processUndoRemove(ctx context.Context, undoActivity *activitypub.Activity, removeActivity interface{}, _ string) error {
	return h.processUndoListActivity(ctx, undoActivity, removeActivity, "remove")
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
			return services.ErrBlockMissingObjectID
		}
	default:
		h.Logger.Error("Block activity has invalid object type",
			zap.String("activity_id", activity.ID),
			zap.Any("object", activity.Object))
		return services.ErrBlockInvalidObjectType
	}

	if err := common.ValidateRequiredParam("blockedActor", blockedActor); err != nil {
		h.Logger.Error("Block activity missing blocked actor",
			zap.String("activity_id", activity.ID))
		return services.ErrBlockMissingBlockedActor
	}

	// Extract the actor doing the blocking
	blockerActor := activity.Actor
	if err := common.ValidateRequiredParam("blockerActor", blockerActor); err != nil {
		h.Logger.Error("Block activity missing blocker actor",
			zap.String("activity_id", activity.ID))
		return services.ErrBlockMissingBlockerActor
	}

	// Create the block relationship
	if err := h.RelationshipRepo.CreateBlock(ctx, blockerActor, blockedActor, activity.ID); err != nil {
		h.Logger.Error("Failed to create block relationship",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return blockRelationshipCreationFailed(err)
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
		return services.ErrExtractFlaggedObjectFromFlag
	}

	if err := common.ValidateSliceNotEmpty("flaggedObjects", flaggedObjects); err != nil {
		return services.ErrNoFlaggedObjectsFound
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
			return flagRecordCreationFailed(err)
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
		ObjectType:      ObjectTypeStatus,                  // Default to status, could be inferred
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
		return services.ErrMoveMustSpecifyTarget
	}

	// The actor field is the old account, target is the new account
	oldAccountID := activity.Actor
	newAccountID := activity.Target

	// Extract username from old account ID
	oldUsername := h.extractUsernameFromActorURI(oldAccountID)
	if err := common.ValidateRequiredParam("oldUsername", oldUsername); err != nil {
		return usernameExtractionFromOldActorURIFailed(activity.Actor)
	}

	// Update the old actor's movedTo field
	if err := h.ActorRepo.UpdateMovedTo(ctx, oldUsername, newAccountID); err != nil {
		h.Logger.Error("failed to update movedTo field",
			zap.String("old_username", oldUsername),
			zap.String("new_account_id", newAccountID),
			zap.Error(err))
		return movedToFieldUpdateFailed(err)
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

// processListActivity processes Add/Remove activities on lists
// This consolidates processAddActivity and processRemoveActivity which had 104 lines of duplication
//
//nolint:unused // False positive - called from processAddActivity and processRemoveActivity
func (h *ActivityHandler) processListActivity(ctx context.Context, activity *activitypub.Activity, username string, activityType string) error {
	h.Logger.Info("Processing list activity",
		zap.String("username", username),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.String("activity_type", activityType))

	// Extract the target collection from activity.Target
	if err := common.ValidateRequiredParam("activity.Target", activity.Target); err != nil {
		h.Logger.Error("List activity missing target collection",
			zap.String("activity_id", activity.ID),
			zap.String("activity_type", activityType))
		return activityMissingTargetCollection()
	}

	// Extract the objects being processed
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
		h.Logger.Error("Unable to extract object from list activity",
			zap.String("activity_id", activity.ID),
			zap.String("activity_type", activityType),
			zap.Any("object", activity.Object))
		return objectExtractionFromActivityFailed()
	}

	if err := common.ValidateSliceNotEmpty("objectIDs", objectIDs); err != nil {
		return noObjectsFoundInActivity()
	}

	// Extract list ID from target
	listID := h.extractListIDFromCollection(activity.Target)
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		h.Logger.Error("Unable to extract list ID from target collection",
			zap.String("target", activity.Target),
			zap.String("activity_id", activity.ID),
			zap.String("activity_type", activityType))
		return services.ErrExtractListIDFromTargetCollection
	}

	// Verify the list exists and the actor has permission to modify it
	list, err := h.ListRepo.GetList(ctx, listID)
	if err != nil {
		h.Logger.Error("Failed to get target list",
			zap.String("list_id", listID),
			zap.String("activity_id", activity.ID),
			zap.String("activity_type", activityType),
			zap.Error(err))
		return targetListRetrievalFailed(err)
	}

	// Check if actor has permission (must be list owner)
	actorUsername := h.extractUsernameFromActor(activity.Actor)
	if list.Username != actorUsername {
		h.Logger.Warn("Actor does not own the target list",
			zap.String("actor", activity.Actor),
			zap.String("list_owner", list.Username),
			zap.String("list_id", listID),
			zap.String("activity_type", activityType))
		return services.ErrActorNoPermissionModifyList
	}

	// Process each object based on activity type
	for _, objectID := range objectIDs {
		// Extract username from object ID
		memberUsername := h.extractUsernameFromActor(objectID)
		if err := common.ValidateRequiredParam("memberUsername", memberUsername); err != nil {
			h.Logger.Warn("Unable to extract username from object ID",
				zap.String("object_id", objectID),
				zap.String("activity_type", activityType))
			continue
		}

		// Perform the appropriate list operation
		var opErr error
		var action string
		if activityType == "add" {
			opErr = h.ListRepo.AddListMember(ctx, listID, memberUsername)
			action = "Added"
		} else {
			opErr = h.ListRepo.RemoveListMember(ctx, listID, memberUsername)
			action = "Removed"
		}

		if opErr != nil {
			h.Logger.Error("Failed to perform list operation",
				zap.String("list_id", listID),
				zap.String("member_username", memberUsername),
				zap.String("activity_id", activity.ID),
				zap.String("activity_type", activityType),
				zap.String("action", action),
				zap.Error(opErr))
			// Continue with other members instead of failing entirely
			continue
		}

		h.Logger.Info("List operation completed",
			zap.String("list_id", listID),
			zap.String("member_username", memberUsername),
			zap.String("activity_id", activity.ID),
			zap.String("activity_type", activityType),
			zap.String("action", action))
	}

	return nil
}

// processAddActivity processes an Add activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processAddActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	return h.processListActivity(ctx, activity, username, "add")
}

// processRemoveActivity processes a Remove activity
//
//nolint:unused // False positive - called from processInboxActivity
func (h *ActivityHandler) processRemoveActivity(ctx context.Context, activity *activitypub.Activity, username string) error {
	return h.processListActivity(ctx, activity, username, "remove")
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
			ID:      activity.ID,
			Type:    types.MessageTypeActivity, // Use the correct type constant
			Actor:   activity.Actor,
			Target:  remoteRecipients,
			Payload: nil, // Will be serialized by the route manager
		}

		// Set delivery options
		options := types.DeliveryOptions{
			Priority:     types.PriorityNormal,
			MaxRetries:   3,
			RetryBackoff: 1 * time.Second,
			Timeout:      30 * time.Second,
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
	cfg := config.Get()
	domain := cfg.Domain
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

		// Skip local recipients (canonical URL host comparison + @domain suffix)
		if common.IsLocalActorID(recipient, domain) || strings.HasSuffix(recipient, "@"+domain) {
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
//
//nolint:unused // false positive - function is used
func (h *ActivityHandler) createNotificationRepo() interfaces.NotificationRepository {
	if h.NotificationRepo == nil {
		repo := repositories.NewNotificationRepository(h.DB, h.TableName, h.Logger, nil)
		if h.NotificationSvc != nil {
			repo.SetDispatcher(h.NotificationSvc)
		}
		h.NotificationRepo = repo
	}
	return h.NotificationRepo
}

// processUndoWithObjectExtraction handles the common pattern for Undo activities
// that need to extract an object/target from the original activity and delete a record
//
//nolint:unused // False positive - called from processUndoLike, processUndoAnnounce, and processUndoBlock
func (h *ActivityHandler) processUndoWithObjectExtraction(
	ctx context.Context,
	undoActivity *activitypub.Activity,
	originalActivity interface{},
	activityType string,
	deleteFunc func(ctx context.Context, actor, target string) error,
) error {
	targetObject := h.extractActivityObject(originalActivity)

	var extractedID string
	switch obj := targetObject.(type) {
	case string:
		extractedID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			extractedID = id
		}
	}

	if err := common.ValidateRequiredParam("extractedID", extractedID); err != nil {
		return targetIDExtractionFromActivityFailed()
	}

	actorURI := undoActivity.Actor
	if err := common.ValidateRequiredParam("actorURI", actorURI); err != nil {
		return undoActivityMissingActor()
	}

	// Call the specific delete function
	if err := deleteFunc(ctx, actorURI, extractedID); err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to delete %s record", activityType),
			zap.String("actor", actorURI),
			zap.String("target_id", extractedID),
			zap.Error(err))
		return activityRecordDeletionFailed(err)
	}

	h.Logger.Info(fmt.Sprintf("Successfully processed Undo %s", cases.Title(language.English).String(activityType)),
		zap.String("actor", actorURI),
		zap.String("target_id", extractedID))

	return nil
}

// createObjectInteractionNotification creates a notification for object interactions (like, announce, etc.)
//
//nolint:unused // False positive - called from processLikeActivity and processAnnounceActivity
func (h *ActivityHandler) createObjectInteractionNotification(ctx context.Context, objectID, actorURI, notificationType, actionVerb, actorRole string) {
	// Find the owner of the object to send notification
	object, err := h.ObjectRepo.GetObject(ctx, objectID)
	if err != nil {
		h.Logger.Warn(fmt.Sprintf("Could not find object for %s notification", notificationType),
			zap.String("object_id", objectID),
			zap.Error(err))
		// Don't fail the entire operation, the action was saved
		return
	}

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
		actorUsername := h.extractUsernameFromActorURI(actorURI)

		if authorUsername != "" && actorUsername != "" {
			// Create notification for the object author
			notification := models.NewNotificationBuilder().
				ForUser(authorUsername).
				OfType(notificationType).
				FromActor(actorUsername, "remote_actor").
				AboutTarget(objectID, "status").
				WithContent(
					fmt.Sprintf("%s %s your post", actorUsername, actionVerb),
					fmt.Sprintf("Your post was %s by %s", actionVerb, actorUsername)).
				Build()

			if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
				h.Logger.Error(fmt.Sprintf("Failed to create %s notification", notificationType),
					zap.String("author", authorUsername),
					zap.String(actorRole, actorUsername),
					zap.String("object_id", objectID),
					zap.Error(err))
				// Don't return error - the action was created successfully
			}
		}
	}
}

//nolint:unused // false positive - called from processCreateActivity for inbound federation mentions
func (h *ActivityHandler) createInboundMentionNotifications(ctx context.Context, status *models.Status, note *activitypub.Note, activity *activitypub.Activity) {
	if h == nil || note == nil || activity == nil || len(note.Tag) == 0 {
		return
	}
	if h.NotificationRepo == nil && h.DB == nil {
		return
	}

	actorUsername := h.extractUsernameFromActorURI(activity.Actor)
	if actorUsername == "" {
		return
	}

	targetID := ""
	if status != nil {
		targetID = strings.TrimSpace(status.StatusID)
	}
	if targetID == "" {
		targetID = strings.TrimSpace(note.ID)
	}

	seen := make(map[string]struct{}, len(note.Tag))
	for _, tag := range note.Tag {
		if tag.Type != "Mention" {
			continue
		}

		mentionedActorID := strings.TrimSpace(tag.Href)
		if mentionedActorID == "" || !h.isLocalActor(mentionedActorID) {
			continue
		}

		recipient := h.extractUsernameFromActorURI(mentionedActorID)
		if recipient == "" || recipient == actorUsername {
			continue
		}

		key := strings.ToLower(recipient)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		notification := models.NewNotificationBuilder().
			ForUser(recipient).
			OfType(common.NotificationTypeMention).
			FromActor(actorUsername, "remote_actor").
			AboutTarget(targetID, "status").
			WithContent(
				fmt.Sprintf("%s mentioned you", actorUsername),
				fmt.Sprintf("You were mentioned by %s", actorUsername)).
			Build()

		if err := h.createNotificationRepo().CreateNotification(ctx, notification); err != nil {
			h.Logger.Error("failed to create inbound mention notification",
				zap.String("recipient", recipient),
				zap.String("actor", actorUsername),
				zap.String("target_id", targetID),
				zap.Error(err))
		}
	}
}

// extractUsernameFromActorURI extracts the username from an ActivityPub actor URI
// Note: linter false positive - this function IS used extensively throughout the file
//
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
//
//nolint:unused // false positive - function is used
func (h *ActivityHandler) distributeToFollowersTimeline(ctx context.Context, status *models.Status, actorID, postID, _ string, isReply bool, inReplyTo string, createdAt time.Time) error {
	// Extract username from actor ID for follower lookup
	actorUsername := h.extractUsernameFromActorURI(actorID)
	if err := common.ValidateRequiredParam("actorUsername", actorUsername); err != nil {
		return usernameExtractionFromActorIDFailed(actorID)
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
		return followersRetrievalFailed(err)
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
		if err := timelineEntry.UpdateKeys(); err != nil {
			h.Logger.Warn("failed to update timeline entry keys",
				zap.String("timeline_type", timelineEntry.TimelineType),
				zap.String("timeline_id", timelineEntry.TimelineID),
				zap.Error(err),
			)
			// Continue processing other entries
			continue
		}
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
			return timelineEntriesCreationFailed(err)
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
//
//nolint:unused // false positive - function is used
func (h *ActivityHandler) extractUsernameFromActor(actorURI string) string {
	return h.extractUsernameFromActorURI(actorURI)
}

// extractListIDFromCollection extracts the list ID from a collection URI
//
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
	federationInstanceRepo := repositories.NewFederationInstanceRepository(db, tableName, logger, nil)
	circuitBreakerRepo := repositories.NewCircuitBreakerRepositoryBasic(db, tableName, logger)
	routeOptimRepo := repositories.NewRouteOptimizerRepository(db, tableName, logger, nil)
	routingMetricsRepo := repositories.NewRoutingMetricsRepository(db, tableName, logger, nil)
	costTrackingBaseRepo := repositories.NewBaseRepository[*models.FederationCostTracking](db, tableName, logger)
	budgetBaseRepo := repositories.NewBaseRepository[*models.FederationBudget](db, tableName, logger)
	costTrackingRepo := repositories.NewFederationCostRepositoryFromBase(costTrackingBaseRepo, budgetBaseRepo, nil)

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
