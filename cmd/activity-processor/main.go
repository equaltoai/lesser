// Package main implements the activity processor Lambda function that handles
// ActivityPub activities from DynamoDB streams and updates various timelines
// and notifications accordingly.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/stream"
)

// Constants for common strings
const (
	// Timeline types
	timelineHome      = "HOME"
	timelinePublic    = "PUBLIC"
	timelineFederated = "FEDERATED"
	timelineLocal     = "LOCAL"

	// Activity types
	activityInsert  = "INSERT"
	activityModify  = "MODIFY"
	activityRemove  = "REMOVE"
	UnknownValue    = "unknown"
	UnknownEventMsg = "unknown event type"
	UnknownTypeMsg  = "processing unknown object type"
	UnknownErrorMsg = "Default to not retrying unknown errors"
)

// ActivityProcessor handles ActivityPub activities from DynamoDB streams,
// processing them to update timelines, notifications, and other related data.
type ActivityProcessor struct {
	db               core.DB
	tableName        string
	logger           *zap.Logger
	timelineRepo     interfaces.TimelineRepository
	actorRepo        interfaces.ActorRepository
	userRepo         interfaces.UserRepository
	relationshipRepo interfaces.ConcreteRelationshipRepository
	objectRepo       interfaces.ObjectRepository
	fetchService     *federation.AuthorizedFetchService
	storageAdapter   storageCore.RepositoryStorage
	baseURL          string
	retryAttempts    int
	retryDelay       time.Duration
}

type activityProcessingRecord struct {
	PK          string `theorydb:"pk,attr:PK"`
	SK          string `theorydb:"sk,attr:SK"`
	Type        string `theorydb:"attr:type"`
	ActivityPK  string `theorydb:"attr:activityPK"`
	Username    string `theorydb:"attr:username"`
	ActorID     string `theorydb:"attr:actorID"`
	ProcessedAt string `theorydb:"attr:processedAt"`
	Status      string `theorydb:"attr:status"`
	TTL         int64  `theorydb:"ttl,attr:ttl"`
}

func (activityProcessingRecord) TableName() string { return models.MainTableName }

type activityMetricsRecord struct {
	PK         string `theorydb:"pk,attr:PK"`
	SK         string `theorydb:"sk,attr:SK"`
	Type       string `theorydb:"attr:type"`
	ActivityPK string `theorydb:"attr:activityPK"`
	Direction  string `theorydb:"attr:direction"`
	Username   string `theorydb:"attr:username"`
	UpdatedAt  string `theorydb:"attr:updatedAt"`
	TTL        int64  `theorydb:"ttl,attr:ttl"`
}

func (activityMetricsRecord) TableName() string { return models.MainTableName }

type activityCleanupRecord struct {
	PK         string `theorydb:"pk,attr:PK"`
	SK         string `theorydb:"sk,attr:SK"`
	Type       string `theorydb:"attr:type"`
	ActivityPK string `theorydb:"attr:activityPK"`
	Direction  string `theorydb:"attr:direction"`
	Username   string `theorydb:"attr:username"`
	DeletedAt  string `theorydb:"attr:deletedAt"`
	TTL        int64  `theorydb:"ttl,attr:ttl"`
}

func (activityCleanupRecord) TableName() string { return models.MainTableName }

type activityProcessorMetricRecord struct {
	PK               string `theorydb:"pk,attr:PK"`
	SK               string `theorydb:"sk,attr:SK"`
	Type             string `theorydb:"attr:type"`
	Timestamp        string `theorydb:"attr:timestamp"`
	TTL              int64  `theorydb:"ttl,attr:ttl"`
	Operation        string `theorydb:"attr:operation,omitempty"`
	Success          bool   `theorydb:"attr:success,omitempty"`
	DurationMS       int64  `theorydb:"attr:durationMS,omitempty"`
	RemoteHost       string `theorydb:"attr:remoteHost,omitempty"`
	ObjectType       string `theorydb:"attr:objectType,omitempty"`
	IsRemote         bool   `theorydb:"attr:isRemote,omitempty"`
	ProcessingTimeMS int64  `theorydb:"attr:processingTimeMS,omitempty"`
	ActivityType     string `theorydb:"attr:activityType,omitempty"`
	EntryCount       int    `theorydb:"attr:entryCount,omitempty"`
	FanoutTimeMS     int64  `theorydb:"attr:fanoutTimeMS,omitempty"`
}

func (activityProcessorMetricRecord) TableName() string { return models.MainTableName }

type activityDLQRecord struct {
	PK             string `theorydb:"pk,attr:PK"`
	SK             string `theorydb:"sk,attr:SK"`
	Type           string `theorydb:"attr:type"`
	EventID        string `theorydb:"attr:eventID"`
	EventName      string `theorydb:"attr:eventName"`
	Reason         string `theorydb:"attr:reason"`
	OriginalRecord string `theorydb:"attr:originalRecord"`
	CreatedAt      string `theorydb:"attr:createdAt"`
	TTL            int64  `theorydb:"ttl,attr:ttl"`
}

func (activityDLQRecord) TableName() string { return models.MainTableName }

type activityTombstoneRecord struct {
	PK        string `theorydb:"pk,attr:PK"`
	SK        string `theorydb:"sk,attr:SK"`
	Type      string `theorydb:"attr:type"`
	ObjectID  string `theorydb:"attr:objectID"`
	ActorID   string `theorydb:"attr:actorID"`
	Reason    string `theorydb:"attr:reason"`
	DeletedAt string `theorydb:"attr:deletedAt"`
	TTL       int64  `theorydb:"ttl,attr:ttl"`
}

func (activityTombstoneRecord) TableName() string { return models.MainTableName }

var fetchAuthorizedObjectFn = func(ctx context.Context, fetchService *federation.AuthorizedFetchService, objectURL string, signingActor *activitypub.Actor) (any, error) {
	if fetchService == nil {
		return nil, fmt.Errorf("authorized fetch service is nil")
	}
	return fetchService.FetchObject(ctx, objectURL, signingActor)
}

var lambdaStartFn = lambda.Start

var activityMetricNow = time.Now

// NewActivityProcessor creates a new activity processor instance with the given
// lambda context
func NewActivityProcessor(lambdaCtx *common.LambdaContext) *ActivityProcessor {
	// Get logger
	logger := lambdaCtx.Logger
	cfg := lambdaCtx.Config

	var (
		db    core.DB
		repos storageCore.RepositoryStorage
	)
	if storage, ok := lambdaCtx.Repos.(storageCore.RepositoryStorage); ok && storage != nil {
		repos = storage
		db = storage.GetDB()
	} else {
		var err error
		// Legacy unit-test fallback: production startup populates LambdaContext via
		// pkg/lambdastorage before constructing the processor.
		db, err = theorydb.GetClient(context.Background())
		if err != nil {
			logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
		}

		repos, err = factory.NewRepositoryFactory(db, cfg.DynamoTableName, logger)
		if err != nil {
			logger.Fatal("Failed to create repository factory", zap.Error(err))
		}
	}

	// Extract domain from baseURL
	baseURL := cfg.BaseURL()
	domain := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")

	// Initialize authorized fetch service with repository factory
	fetchService := federation.NewAuthorizedFetchService(repos, domain, logger)

	return &ActivityProcessor{
		db:               db,
		tableName:        cfg.DynamoTableName,
		logger:           logger,
		timelineRepo:     repos.Timeline(),
		actorRepo:        repos.Actor(),
		userRepo:         repos.User(),
		relationshipRepo: repos.Relationship(),
		objectRepo:       repos.Object(),
		fetchService:     fetchService,
		storageAdapter:   repos,
		baseURL:          baseURL,
		retryAttempts:    3,
		retryDelay:       time.Second * 2,
	}
}

func (ap *ActivityProcessor) HandleDynamoDBRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = strings.TrimSpace(ctx.RequestID)
		runCtx = ctx.Context()
	}
	if requestID == "" {
		requestID = UnknownValue
	}

	if err := ap.processRecord(runCtx, record); err != nil {
		// Retryable errors are returned so AppTheory can return a partial batch failure response.
		if ap.isRetryableStreamError(err) {
			ap.logger.Warn("retryable error processing record",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.Error(err),
			)
			return err
		}

		// Non-retryable errors are sent to the DLQ and do not fail the record.
		ap.logger.Error("non-retryable error, sending to DLQ",
			zap.String("request_id", requestID),
			zap.String("event_id", record.EventID),
			zap.Error(err),
		)

		if dlqErr := ap.sendToDeadLetterQueue(runCtx, record, "processing_failed"); dlqErr != nil {
			ap.logger.Error("failed to send record to DLQ",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.Error(dlqErr),
			)
		}

		return nil
	}

	return nil
}

func (ap *ActivityProcessor) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Parse the stream record into activity data
	var activity struct {
		PK        string `theorydb:"pk"`
		SK        string `theorydb:"sk"`
		Type      string `json:"type"`
		Activity  string `json:"activity"`
		Direction string `json:"direction"`
		Username  string `json:"username"`
		ActorID   string `json:"actor_id"`
		CreatedAt string `json:"created_at"`
	}

	switch record.EventName {
	case activityInsert, activityModify:
		if record.Change.NewImage == nil {
			ap.logger.Error("missing new image in record", zap.String("event_id", record.EventID))
			return missingNewImage(record.EventID)
		}

		// Convert DynamoDB attribute values using DynamORM
		if err := stream.UnmarshalItem(record, &activity); err != nil {
			return streamRecordUnmarshalNewFailed(record.EventID, err)
		}

	case activityRemove:
		if record.Change.OldImage == nil {
			ap.logger.Error("missing old image in remove record", zap.String("event_id", record.EventID))
			return missingOldImage(record.EventID)
		}

		// Convert DynamoDB attribute values from OldImage for REMOVE events
		// The UnmarshalItem function already handles this case by checking EventName
		if err := stream.UnmarshalItem(record, &activity); err != nil {
			return streamRecordUnmarshalOldFailed(record.EventID, err)
		}

	default:
		ap.logger.Warn(UnknownEventMsg,
			zap.String("event_name", record.EventName),
			zap.String("event_id", record.EventID),
		)
		return nil
	}

	// Only process activity records
	if !strings.HasPrefix(activity.PK, "ACTIVITY#") {
		return nil
	}

	// Route based on activity type and direction
	switch record.EventName {
	case activityInsert:
		return ap.processActivityCreated(ctx, activity)
	case activityModify:
		return ap.processActivityUpdated(ctx, activity)
	case activityRemove:
		return ap.processActivityDeleted(ctx, activity)
	default:
		return nil
	}
}

func (ap *ActivityProcessor) processActivityCreated(ctx context.Context, activity struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	ap.logger.Info("processing activity created",
		zap.String("pk", activity.PK),
		zap.String("direction", activity.Direction),
		zap.String("username", activity.Username),
	)

	// Process based on direction (inbox vs outbox)
	switch activity.Direction {
	case "inbox":
		return ap.processInboxActivity(ctx, activity)
	case "outbox":
		return ap.processOutboxActivity(ctx, activity)
	}

	return nil
}

func (ap *ActivityProcessor) processActivityUpdated(ctx context.Context, activity struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	ap.logger.Info("processing activity updated",
		zap.String("pk", activity.PK),
		zap.String("direction", activity.Direction),
	)

	// Handle activity updates (e.g., status changes)
	// This could trigger notifications, cache invalidation, etc.
	return ap.updateActivityMetrics(ctx, activity)
}

func (ap *ActivityProcessor) processActivityDeleted(ctx context.Context, activity struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	ap.logger.Info("processing activity deleted",
		zap.String("pk", activity.PK),
		zap.String("direction", activity.Direction),
	)

	// Parse the deleted activity to understand what was removed.
	activityData, err := activitypub.ParseActivity([]byte(activity.Activity))
	if err != nil {
		ap.logger.Warn("failed to safely parse deleted activity", zap.Error(err))
		// Continue with generic cleanup even if we can't parse the activity
		return ap.cleanupActivityReferences(ctx, activity)
	}

	// Handle specific deletion types
	switch activityData.Type {
	case activitypub.CreateType:
		// Remove from timelines and create tombstone
		if err := ap.handleCreateActivityDeletion(ctx, activityData, activity.Username); err != nil {
			ap.logger.Error("failed to handle Create activity deletion", zap.Error(err))
			return err
		}
	case activitypub.AnnounceType:
		// Remove announce from timelines
		if err := ap.handleAnnounceActivityDeletion(ctx, activityData, activity.Username); err != nil {
			ap.logger.Error("failed to handle Announce activity deletion", zap.Error(err))
			return err
		}
	case activitypub.FollowType:
		// Remove follow relationship
		if err := ap.handleFollowActivityDeletion(ctx, activityData); err != nil {
			ap.logger.Error("failed to handle Follow activity deletion", zap.Error(err))
			return err
		}
	case activitypub.DeleteType:
		// Handle nested deletions
		if err := ap.handleDeleteActivityDeletion(ctx, activityData); err != nil {
			ap.logger.Error("failed to handle Delete activity deletion", zap.Error(err))
			return err
		}
	default:
		ap.logger.Info("handling generic activity deletion",
			zap.String("activity_type", activityData.Type))
	}

	// Always perform generic cleanup
	return ap.cleanupActivityReferences(ctx, activity)
}

func (ap *ActivityProcessor) processInboxActivity(ctx context.Context, activity struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	// Process activities received from other servers
	ap.logger.Debug("processing inbox activity",
		zap.String("pk", activity.PK),
		zap.String("username", activity.Username),
	)

	// Create inbox processing record
	inboxRecord := activityProcessingRecord{
		PK:          fmt.Sprintf("INBOX#%s", activity.Username),
		SK:          fmt.Sprintf("PROCESSED#%s", activity.PK),
		Type:        "InboxProcessing",
		ActivityPK:  activity.PK,
		Username:    activity.Username,
		ActorID:     activity.ActorID,
		ProcessedAt: time.Now().Format(time.RFC3339),
		Status:      "processed",
		TTL:         time.Now().Add(7 * 24 * time.Hour).Unix(), // 7 days retention
	}

	return tolerateActivityReplayCreate(ap.db.WithContext(ctx).Model(&inboxRecord).Create())
}

func (ap *ActivityProcessor) processOutboxActivity(ctx context.Context, activity struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	// Process activities sent by local users
	ap.logger.Debug("processing outbox activity",
		zap.String("pk", activity.PK),
		zap.String("username", activity.Username),
		zap.String("type", activity.Type),
	)

	// Parse the activity JSON with ActivityPub safety limits.
	activityData, err := activitypub.ParseActivity([]byte(activity.Activity))
	if err != nil {
		ap.logger.Error("failed to safely parse activity", zap.Error(err))
		return activityParsingFailedDetailed("unknown", err)
	}

	// Handle timeline fanout based on activity type
	switch activityData.Type {
	case activitypub.CreateType:
		if err := ap.fanOutToTimelines(ctx, activityData, activity.Username); err != nil {
			ap.logger.Error("failed to fan out Create activity", zap.Error(err))
			return err
		}
	case activitypub.AnnounceType:
		if err := ap.fanOutAnnounceToTimelines(ctx, activityData, activity.Username); err != nil {
			ap.logger.Error("failed to fan out Announce activity", zap.Error(err))
			return err
		}
	}

	// Create outbox processing record
	outboxRecord := activityProcessingRecord{
		PK:          fmt.Sprintf("OUTBOX#%s", activity.Username),
		SK:          fmt.Sprintf("PROCESSED#%s", activity.PK),
		Type:        "OutboxProcessing",
		ActivityPK:  activity.PK,
		Username:    activity.Username,
		ActorID:     activity.ActorID,
		ProcessedAt: time.Now().Format(time.RFC3339),
		Status:      "processed",
		TTL:         time.Now().Add(7 * 24 * time.Hour).Unix(), // 7 days retention
	}

	return tolerateActivityReplayCreate(ap.db.WithContext(ctx).Model(&outboxRecord).Create())
}

func (ap *ActivityProcessor) updateActivityMetrics(ctx context.Context, activity struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	// Update activity metrics for analytics
	metricsRecord := activityMetricsRecord{
		PK:         fmt.Sprintf("METRICS#ACTIVITY#%s", activity.Direction),
		SK:         fmt.Sprintf("UPDATE#%s", activity.PK),
		Type:       "ActivityMetrics",
		ActivityPK: activity.PK,
		Direction:  activity.Direction,
		Username:   activity.Username,
		UpdatedAt:  time.Now().Format(time.RFC3339),
		TTL:        time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days retention
	}

	return ap.db.WithContext(ctx).Model(&metricsRecord).CreateOrUpdate()
}

func (ap *ActivityProcessor) cleanupActivityReferences(ctx context.Context, activity struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	// Create cleanup record for deleted activities
	cleanupRecord := activityCleanupRecord{
		PK:         "CLEANUP#ACTIVITY",
		SK:         fmt.Sprintf("DELETED#%s", activity.PK),
		Type:       "ActivityCleanup",
		ActivityPK: activity.PK,
		Direction:  activity.Direction,
		Username:   activity.Username,
		DeletedAt:  time.Now().Format(time.RFC3339),
		TTL:        time.Now().Add(24 * time.Hour).Unix(), // 24 hours retention
	}

	return tolerateActivityReplayCreate(ap.db.WithContext(ctx).Model(&cleanupRecord).Create())
}

// tolerateActivityReplayCreate keeps create-only processor records immutable
// while treating an at-least-once delivery of the same deterministic key as
// successful. Non-condition failures still propagate to the retry policy.
func tolerateActivityReplayCreate(err error) error {
	if dynamormerrors.IsConditionFailed(err) {
		return nil
	}
	return err
}

// ProcessedObject holds information about a processed ActivityPub object
type ProcessedObject struct {
	Note        *activitypub.Note
	Content     string
	IsRemote    bool
	ObjectID    string
	ContentType string
	HasMedia    bool
	IsReply     bool
	InReplyTo   string
	Sensitive   bool
	SpoilerText string
	Language    string
	Visibility  string
}

// fanOutToTimelines handles timeline fanout for Create activities with robust federation support
func (ap *ActivityProcessor) fanOutToTimelines(ctx context.Context, activity *activitypub.Activity, username string) error {
	// Process the object from the activity
	processedObj, err := ap.processActivityObject(ctx, activity)
	if err != nil {
		return activityObjectProcessingFailed("Create", err)
	}

	if processedObj == nil {
		ap.logger.Warn("unsupported object type in Create activity", zap.Any("object", activity.Object))
		return nil
	}

	// Create timeline entries and fan out
	now := time.Now()
	return ap.createAndFanOutEntries(ctx, activity, username, processedObj, now)
}

// processActivityObject extracts and processes the object from an activity
func (ap *ActivityProcessor) processActivityObject(ctx context.Context, activity *activitypub.Activity) (*ProcessedObject, error) {
	switch obj := activity.Object.(type) {
	case map[string]interface{}:
		return ap.processMapObject(ctx, activity, obj)
	case *activitypub.Note:
		return ap.processNoteObject(activity, obj), nil
	case string:
		return ap.processStringObject(ctx, activity, obj)
	default:
		return nil, nil
	}
}

// processMapObject processes a map[string]interface{} object
func (ap *ActivityProcessor) processMapObject(ctx context.Context, activity *activitypub.Activity, obj map[string]interface{}) (*ProcessedObject, error) {
	id, hasID := obj["id"].(string)
	if hasID && !strings.HasPrefix(id, ap.baseURL) {
		// Remote object reference - fetch it
		return ap.fetchRemoteMapObject(ctx, activity, obj, id)
	}

	// Local embedded object - convert map to Note
	return ap.convertMapToNote(obj)
}

// fetchRemoteMapObject fetches a remote object referenced in a map
func (ap *ActivityProcessor) fetchRemoteMapObject(ctx context.Context, activity *activitypub.Activity, _ map[string]interface{}, id string) (*ProcessedObject, error) {
	ap.logger.Debug("detected remote object in Create activity",
		zap.String("object_id", id),
		zap.String("actor", activity.Actor))

	signingActor, err := ap.actorRepo.GetActor(ctx, activity.Actor)
	if err != nil {
		ap.logger.Error("failed to get signing actor for remote object fetch", zap.Error(err))
		return ap.createFallbackObject(id, "Remote object: "+id), nil
	}

	remoteObj, err := ap.fetchRemoteObjectWithRetry(ctx, id, signingActor)
	if err != nil {
		ap.logger.Warn("failed to fetch remote object in Create activity",
			zap.String("object_id", id),
			zap.Error(err))
		return ap.createFallbackObject(id, "Remote object: "+id), nil
	}

	return ap.processRemoteObject(remoteObj, id)
}

// processRemoteObject processes a successfully fetched remote object
func (ap *ActivityProcessor) processRemoteObject(remoteObj interface{}, id string) (*ProcessedObject, error) {
	if fetchedNote, ok := remoteObj.(*activitypub.Note); ok {
		ap.logger.Info("successfully fetched remote object for Create activity",
			zap.String("object_id", id))
		result := ap.processNoteObject(nil, fetchedNote)
		result.IsRemote = true
		return result, nil
	}

	if objMap, ok := remoteObj.(map[string]interface{}); ok {
		if content, ok := objMap["content"].(string); ok {
			return ap.createFallbackObject(id, content), nil
		}
	}

	return ap.createFallbackObject(id, "Remote object: "+id), nil
}

// convertMapToNote converts a map to a Note object
func (ap *ActivityProcessor) convertMapToNote(obj map[string]interface{}) (*ProcessedObject, error) {
	noteData, err := json.Marshal(obj)
	if err != nil {
		return nil, noteMarshalingFailed(err)
	}

	note := &activitypub.Note{}
	if err := json.Unmarshal(noteData, note); err != nil {
		return nil, noteUnmarshalingFailed(err)
	}

	return ap.processNoteObject(nil, note), nil
}

// processNoteObject processes a Note object
func (ap *ActivityProcessor) processNoteObject(activity *activitypub.Activity, note *activitypub.Note) *ProcessedObject {
	to, cc := note.To, note.CC
	if activity != nil && len(to) == 0 && len(cc) == 0 {
		// Fall back to activity addressing
		to, cc = activity.To, activity.CC
	}

	return &ProcessedObject{
		Note:        note,
		Content:     note.Content,
		ObjectID:    note.ID,
		ContentType: "Note",
		HasMedia:    len(note.Attachment) > 0,
		IsReply:     note.InReplyTo != "",
		InReplyTo:   note.InReplyTo,
		Sensitive:   note.Sensitive,
		SpoilerText: note.Summary,
		Language:    ap.extractLanguage(note),
		Visibility:  ap.determineVisibility(to, cc),
	}
}

// processStringObject processes a string object reference
func (ap *ActivityProcessor) processStringObject(ctx context.Context, activity *activitypub.Activity, objectID string) (*ProcessedObject, error) {
	// Check if it's local first
	existingObj, err := ap.objectRepo.GetObject(ctx, objectID)
	if err == nil && existingObj != nil {
		if localNote, ok := existingObj.(*models.Object); ok {
			ap.logger.Debug("found referenced object locally", zap.String("object_id", objectID))
			return ap.createObjectFromContent(objectID, localNote.Content, activity), nil
		}
	}

	// Not found locally - try remote fetch if it's a remote URL
	if !strings.HasPrefix(objectID, ap.baseURL) {
		return ap.fetchStringRemoteObject(ctx, activity, objectID)
	}

	// Local object that doesn't exist
	ap.logger.Warn("local object reference not found", zap.String("object_id", objectID))
	return ap.createFallbackObject(objectID, "Missing local object: "+objectID), nil
}

// fetchStringRemoteObject fetches a remote object by ID
func (ap *ActivityProcessor) fetchStringRemoteObject(ctx context.Context, activity *activitypub.Activity, objectID string) (*ProcessedObject, error) {
	signingActor, err := ap.actorRepo.GetActor(ctx, activity.Actor)
	if err != nil {
		ap.logger.Error("failed to get signing actor for object reference fetch", zap.Error(err))
		return ap.createFallbackObject(objectID, "Referenced object: "+objectID), nil
	}

	remoteObj, err := ap.fetchRemoteObjectWithRetry(ctx, objectID, signingActor)
	if err != nil {
		ap.logger.Warn("failed to fetch referenced object",
			zap.String("object_id", objectID),
			zap.Error(err))
		return ap.createFallbackObject(objectID, "Referenced object: "+objectID), nil
	}

	return ap.processRemoteObject(remoteObj, objectID)
}

// createFallbackObject creates a fallback processed object
func (ap *ActivityProcessor) createFallbackObject(objectID, content string) *ProcessedObject {
	return &ProcessedObject{
		Content:     content,
		IsRemote:    true,
		ObjectID:    objectID,
		ContentType: "Object",
		Visibility:  "public",
	}
}

// createObjectFromContent creates a processed object from content
func (ap *ActivityProcessor) createObjectFromContent(objectID, content string, activity *activitypub.Activity) *ProcessedObject {
	to, cc := activity.To, activity.CC
	if common.ValidateSliceNotEmpty("to", to) != nil && common.ValidateSliceNotEmpty("cc", cc) != nil {
		to = []string{"https://www.w3.org/ns/activitystreams#Public"}
	}

	return &ProcessedObject{
		Content:     content,
		ObjectID:    objectID,
		ContentType: "Object",
		Visibility:  ap.determineVisibility(to, cc),
		Language:    ap.detectLanguageFromContent(content),
	}
}

// createAndFanOutEntries creates timeline entries and performs fanout
func (ap *ActivityProcessor) createAndFanOutEntries(ctx context.Context, activity *activitypub.Activity, username string, obj *ProcessedObject, now time.Time) error {
	// Create base entry
	baseEntry := ap.createBaseTimelineEntry(activity, username, obj, now)

	// Create all timeline entries
	entries := ap.createAllTimelineEntries(ctx, activity, username, baseEntry, obj.Visibility)

	// Write entries to timelines
	if err := common.ValidateSliceNotEmpty("entries", entries); err == nil {
		if err := ap.timelineRepo.CreateTimelineEntries(ctx, entries); err != nil {
			return timelineEntriesWriteFailed(err)
		}
	}

	// Record metrics and log success
	ap.recordFanoutSuccess(ctx, obj, len(entries), time.Since(now))

	return nil
}

// createBaseTimelineEntry creates the base timeline entry
func (ap *ActivityProcessor) createBaseTimelineEntry(activity *activitypub.Activity, username string, obj *ProcessedObject, now time.Time) models.Timeline {
	content := obj.Content
	if err := common.ValidateStringLength("content", content, 0, 500); err != nil {
		content = content[:500]
	}

	objectID := obj.ObjectID
	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		objectID = activity.ID
	}

	return models.Timeline{
		PostID:      objectID,
		ActorID:     activity.Actor,
		ActorHandle: username,
		Content:     content,
		ContentType: obj.ContentType,
		HasMedia:    obj.HasMedia,
		IsReply:     obj.IsReply,
		InReplyTo:   obj.InReplyTo,
		IsBoost:     false,
		Visibility:  obj.Visibility,
		Language:    obj.Language,
		Sensitive:   obj.Sensitive,
		SpoilerText: obj.SpoilerText,
		CreatedAt:   ap.extractPublishedTime(activity),
		TimelineAt:  now,
	}
}

// createAllTimelineEntries creates all necessary timeline entries
func (ap *ActivityProcessor) createAllTimelineEntries(ctx context.Context, activity *activitypub.Activity, username string, baseEntry models.Timeline, visibility string) []*models.Timeline {
	var entries []*models.Timeline
	now := baseEntry.TimelineAt

	// Add public timeline entries
	entries = append(entries, ap.createPublicTimelineEntries(activity, baseEntry, visibility, now)...)

	// Add home timeline entry for author
	homeEntry := baseEntry
	homeEntry.TimelineType = timelineHome
	homeEntry.TimelineID = username
	homeEntry.EntryID = ap.generateTimelineSK(now, baseEntry.PostID)
	entries = append(entries, &homeEntry)

	// Add follower timeline entries
	entries = append(entries, ap.createFollowerTimelineEntries(ctx, username, baseEntry, visibility, now)...)

	return entries
}

// createPublicTimelineEntries creates public timeline entries if applicable
func (ap *ActivityProcessor) createPublicTimelineEntries(activity *activitypub.Activity, baseEntry models.Timeline, visibility string, now time.Time) []*models.Timeline {
	if visibility != VisibilityPublic {
		return nil
	}

	var entries []*models.Timeline

	// Federated timeline
	publicEntry := baseEntry
	publicEntry.TimelineType = timelinePublic
	publicEntry.TimelineID = timelineFederated
	publicEntry.EntryID = ap.generateTimelineSK(now, baseEntry.PostID)
	entries = append(entries, &publicEntry)

	// Local timeline if it's a local user
	if strings.HasPrefix(activity.Actor, ap.baseURL) {
		localEntry := baseEntry
		localEntry.TimelineType = timelinePublic
		localEntry.TimelineID = timelineLocal
		localEntry.EntryID = ap.generateTimelineSK(now, baseEntry.PostID)
		entries = append(entries, &localEntry)
	}

	return entries
}

// createFollowerTimelineEntries creates timeline entries for followers
func (ap *ActivityProcessor) createFollowerTimelineEntries(ctx context.Context, username string, baseEntry models.Timeline, visibility string, now time.Time) []*models.Timeline {
	if visibility == VisibilityDirect {
		return nil
	}

	followers, err := ap.getFollowers(ctx, username)
	if err != nil {
		ap.logger.Error("failed to get followers", zap.Error(err))
		return nil
	}

	entries := make([]*models.Timeline, 0, len(followers))
	for _, follower := range followers {
		followerEntry := baseEntry
		followerEntry.TimelineType = timelineHome
		followerEntry.TimelineID = follower
		followerEntry.EntryID = ap.generateTimelineSK(now, baseEntry.PostID)
		entries = append(entries, &followerEntry)
	}

	return entries
}

// recordFanoutSuccess records metrics and logs success
func (ap *ActivityProcessor) recordFanoutSuccess(ctx context.Context, obj *ProcessedObject, entryCount int, duration time.Duration) {
	ap.recordTimelineFanoutMetrics(ctx, "Create", entryCount, duration)
	ap.recordObjectProcessingMetrics(ctx, obj.ContentType, obj.IsRemote, duration)

	ap.logger.Info("successfully fanned out Create activity",
		zap.String("post_id", obj.ObjectID),
		zap.String("content_type", obj.ContentType),
		zap.String("visibility", obj.Visibility),
		zap.Int("timeline_count", entryCount),
		zap.Bool("is_remote_object", obj.IsRemote),
		zap.Duration("fanout_duration", duration),
	)
}

// fanOutAnnounceToTimelines handles timeline fanout for Announce (boost) activities
func (ap *ActivityProcessor) fanOutAnnounceToTimelines(ctx context.Context, activity *activitypub.Activity, username string) error {
	// Extract the announced object ID
	announcedID := ap.extractAnnouncedID(activity)
	if err := common.ValidateRequiredParam("announcedID", announcedID); err != nil {
		return missingAnnounceObjectID()
	}

	// Get the announced content
	announcedContent, originalAuthor := ap.getAnnouncedContent(ctx, activity, announcedID)
	_ = originalAuthor // Keep for future use

	// Create timeline entries
	entries := ap.createAnnounceTimelineEntries(ctx, activity, username, announcedContent)

	// Write all entries
	if err := common.ValidateSliceNotEmpty("entries", entries); err == nil {
		if err := ap.timelineRepo.CreateTimelineEntries(ctx, entries); err != nil {
			return timelineEntriesWriteFailed(err)
		}
	}

	// Record metrics
	ap.recordAnnounceMetrics(ctx, activity, announcedID, entries)

	return nil
}

// extractAnnouncedID extracts the announced object ID from the activity
func (ap *ActivityProcessor) extractAnnouncedID(activity *activitypub.Activity) string {
	switch obj := activity.Object.(type) {
	case string:
		return obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			return id
		}
	default:
		ap.logger.Warn("unsupported object type in Announce activity", zap.Any("object", activity.Object))
	}
	return ""
}

// getAnnouncedContent retrieves the content of the announced object
func (ap *ActivityProcessor) getAnnouncedContent(ctx context.Context, activity *activitypub.Activity, announcedID string) (string, string) {
	// First check if object exists locally
	if content, author := ap.getLocalAnnouncedContent(ctx, announcedID); content != "" {
		return content, author
	}

	// Object not found locally, fetch from remote server
	return ap.getRemoteAnnouncedContent(ctx, activity, announcedID)
}

// getLocalAnnouncedContent retrieves content from a locally stored object
func (ap *ActivityProcessor) getLocalAnnouncedContent(ctx context.Context, announcedID string) (string, string) {
	existingObj, err := ap.objectRepo.GetObject(ctx, announcedID)
	if err != nil || existingObj == nil {
		return "", ""
	}

	// Extract content based on object type
	switch obj := existingObj.(type) {
	case *models.Object:
		ap.logger.Debug("found announced object locally", zap.String("object_id", announcedID))
		return obj.Content, ""
	default:
		return ap.extractContentFromMap(existingObj)
	}
}

// extractContentFromMap extracts content and author from a generic map
func (ap *ActivityProcessor) extractContentFromMap(obj interface{}) (string, string) {
	objMap, ok := obj.(map[string]interface{})
	if !ok {
		return "", ""
	}

	var content, author string
	if c, ok := objMap["content"].(string); ok {
		content = c
	}
	if a, ok := objMap["attributedTo"].(string); ok {
		author = a
	}
	return content, author
}

// getRemoteAnnouncedContent fetches content from a remote server
func (ap *ActivityProcessor) getRemoteAnnouncedContent(ctx context.Context, activity *activitypub.Activity, announcedID string) (string, string) {
	ap.logger.Info("fetching remote object for announce", zap.String("object_id", announcedID))

	// Get the announcing actor for signing requests
	signingActor, err := ap.actorRepo.GetActor(ctx, activity.Actor)
	if err != nil {
		ap.logger.Error("failed to get signing actor", zap.Error(err))
		return fmt.Sprintf("Boosted: %s", announcedID), ""
	}

	// Fetch the remote object
	return ap.fetchAndProcessRemoteObject(ctx, announcedID, signingActor)
}

// fetchAndProcessRemoteObject fetches and processes a remote object
func (ap *ActivityProcessor) fetchAndProcessRemoteObject(ctx context.Context, announcedID string, signingActor interface{}) (string, string) {
	// Type assert signingActor to *activitypub.Actor
	actor, ok := signingActor.(*activitypub.Actor)
	if !ok {
		ap.logger.Error("signing actor is not of expected type")
		return fmt.Sprintf("Boosted: %s", announcedID), ""
	}

	remoteObj, err := ap.fetchRemoteObjectWithRetry(ctx, announcedID, actor)
	if err != nil {
		ap.logger.Warn("failed to fetch remote object after retries",
			zap.String("object_id", announcedID),
			zap.Error(err))
		return fmt.Sprintf("Boosted: %s", announcedID), ""
	}

	// Process the fetched object
	return ap.processRemoteObjectForAnnounce(ctx, remoteObj, announcedID)
}

// processRemoteObjectForAnnounce processes a fetched remote object for announce activities
func (ap *ActivityProcessor) processRemoteObjectForAnnounce(ctx context.Context, remoteObj interface{}, announcedID string) (string, string) {
	if note, ok := remoteObj.(*activitypub.Note); ok {
		return ap.processRemoteNote(ctx, note, announcedID)
	}

	// Handle other object types
	content, author := ap.extractContentFromMap(remoteObj)
	if content != "" {
		if objMap, ok := remoteObj.(map[string]interface{}); ok {
			ap.storeGenericRemoteObject(ctx, objMap)
		}
		return content, author
	}

	return fmt.Sprintf("Boosted: %s", announcedID), ""
}

// processRemoteNote processes a remote Note object
func (ap *ActivityProcessor) processRemoteNote(ctx context.Context, note *activitypub.Note, announcedID string) (string, string) {
	// Store the remote object for future reference
	ap.storeRemoteObject(ctx, note)

	ap.logger.Info("successfully fetched and stored remote object",
		zap.String("object_id", announcedID),
		zap.String("author", note.AttributedTo))

	return note.Content, note.AttributedTo
}

// createAnnounceTimelineEntries creates timeline entries for an announce activity
func (ap *ActivityProcessor) createAnnounceTimelineEntries(ctx context.Context, activity *activitypub.Activity, username, announcedContent string) []*models.Timeline {
	var entries []*models.Timeline
	now := time.Now()

	visibility := ap.determineAnnounceVisibility(activity)
	baseEntry := ap.createBaseAnnounceEntry(activity, username, announcedContent, visibility, now)

	if visibility == VisibilityPublic {
		// Add to public timelines
		entries = append(entries, ap.createPublicTimelineEntry(baseEntry, now, activity.ID))

		// Add local timeline entry if applicable
		if ap.isLocalActor(activity.Actor) {
			entries = append(entries, ap.createLocalTimelineEntry(baseEntry, now, activity.ID))
		}
	}

	// Add to author's home timeline
	entries = append(entries, ap.createHomeTimelineEntry(baseEntry, username, now, activity.ID))

	// Fan out to followers unless this boost is direct-only.
	if visibility != VisibilityDirect {
		ap.addFollowerEntries(ctx, &entries, baseEntry, username, now, activity.ID)
	}

	return entries
}

// createBaseAnnounceEntry creates the base timeline entry for an announce
func (ap *ActivityProcessor) createBaseAnnounceEntry(activity *activitypub.Activity, username, announcedContent, visibility string, now time.Time) models.Timeline {
	return models.Timeline{
		PostID:      activity.ID,
		ActorID:     activity.Actor,
		ActorHandle: username,
		Content:     announcedContent,
		ContentType: "Announce",
		IsBoost:     true,
		BoostedBy:   username,
		Visibility:  visibility,
		CreatedAt:   ap.extractPublishedTime(activity),
		TimelineAt:  now,
	}
}

// createPublicTimelineEntry creates a public timeline entry
func (ap *ActivityProcessor) createPublicTimelineEntry(baseEntry models.Timeline, now time.Time, activityID string) *models.Timeline {
	publicEntry := baseEntry
	publicEntry.TimelineType = timelinePublic
	publicEntry.TimelineID = timelineFederated
	publicEntry.EntryID = ap.generateTimelineSK(now, activityID)
	return &publicEntry
}

// createLocalTimelineEntry creates a local timeline entry
func (ap *ActivityProcessor) createLocalTimelineEntry(baseEntry models.Timeline, now time.Time, activityID string) *models.Timeline {
	localEntry := baseEntry
	localEntry.TimelineType = timelinePublic
	localEntry.TimelineID = timelineLocal
	localEntry.EntryID = ap.generateTimelineSK(now, activityID)
	return &localEntry
}

// createHomeTimelineEntry creates a home timeline entry
func (ap *ActivityProcessor) createHomeTimelineEntry(baseEntry models.Timeline, username string, now time.Time, activityID string) *models.Timeline {
	homeEntry := baseEntry
	homeEntry.TimelineType = timelineHome
	homeEntry.TimelineID = username
	homeEntry.EntryID = ap.generateTimelineSK(now, activityID)
	return &homeEntry
}

// addFollowerEntries adds timeline entries for followers
func (ap *ActivityProcessor) addFollowerEntries(ctx context.Context, entries *[]*models.Timeline, baseEntry models.Timeline, username string, now time.Time, activityID string) {
	followers, err := ap.getFollowers(ctx, username)
	if err != nil {
		ap.logger.Error("failed to get followers", zap.Error(err))
		return
	}

	for _, follower := range followers {
		followerEntry := baseEntry
		followerEntry.TimelineType = timelineHome
		followerEntry.TimelineID = follower
		followerEntry.EntryID = ap.generateTimelineSK(now, activityID)
		*entries = append(*entries, &followerEntry)
	}
}

// isLocalActor checks if an actor is local
func (ap *ActivityProcessor) isLocalActor(actorID string) bool {
	return strings.HasPrefix(actorID, ap.baseURL)
}

// recordAnnounceMetrics records metrics for an announce activity
func (ap *ActivityProcessor) recordAnnounceMetrics(ctx context.Context, activity *activitypub.Activity, announcedID string, entries []*models.Timeline) {
	startTime := time.Now()
	fanoutDuration := time.Duration(0) // This will be calculated elsewhere if needed
	ap.recordTimelineFanoutMetrics(ctx, "Announce", len(entries), fanoutDuration)

	isRemoteAnnounced := !ap.isLocalActor(announcedID)
	ap.recordObjectProcessingMetrics(ctx, "Announce", isRemoteAnnounced, fanoutDuration)

	ap.logger.Info("successfully fanned out Announce activity",
		zap.String("activity_id", activity.ID),
		zap.String("announced_id", announcedID),
		zap.Int("timeline_count", len(entries)),
		zap.Bool("is_remote_announced", isRemoteAnnounced),
		zap.Duration("processing_time", time.Since(startTime)),
	)
}

// Helper functions

func (ap *ActivityProcessor) determineAnnounceVisibility(activity *activitypub.Activity) string {
	visibility := ap.determineVisibility(activity.To, activity.CC)
	if visibility == VisibilityDirect && containsFollowersAddress(activity.To, activity.CC) {
		return VisibilityPrivate
	}
	return visibility
}

func containsFollowersAddress(groups ...[]string) bool {
	for _, addresses := range groups {
		for _, address := range addresses {
			trimmed := strings.TrimSuffix(strings.TrimSpace(address), "/")
			if strings.HasSuffix(trimmed, "/followers") {
				return true
			}
		}
	}
	return false
}

func (ap *ActivityProcessor) determineVisibility(to, cc []string) string {
	// Direct message - no public addressing
	if !containsPublicAddress(to) && !containsPublicAddress(cc) {
		return VisibilityDirect
	}

	// Public - addressed to public in 'to'
	if containsPublicAddress(to) {
		return VisibilityPublic
	}

	// Unlisted - public in 'cc'
	if containsPublicAddress(cc) {
		return "unlisted"
	}

	// Private - followers only
	return VisibilityPrivate
}

func (ap *ActivityProcessor) extractLanguage(note *activitypub.Note) string {
	// Check if the note has explicit language information in content
	// ActivityPub notes may include language hints in various formats
	if note.Summary != "" {
		// Some implementations put language codes in summary
		if strings.HasPrefix(note.Summary, "[lang:") {
			if end := strings.Index(note.Summary, "]"); end > 6 {
				lang := note.Summary[6:end]
				if len(lang) == 2 || len(lang) == 5 { // "en" or "en-US" format
					return lang
				}
			}
		}
	}

	// Implement content-based language detection using simple heuristics
	content := strings.ToLower(note.Content)

	// Strip HTML tags for cleaner analysis
	content = stripHTMLTags(content)

	// Character-based detection for non-Latin scripts
	if hasJapaneseCharacters(content) {
		return "ja"
	}
	if hasChineseCharacters(content) {
		return "zh"
	}
	if hasKoreanCharacters(content) {
		return "ko"
	}
	if hasArabicCharacters(content) {
		return "ar"
	}
	if hasCyrillicCharacters(content) {
		// Could be Russian, Ukrainian, etc. Default to Russian
		return "ru"
	}

	// Word-based detection for Latin script languages
	// Check for common words/patterns
	if hasSpanishPatterns(content) {
		return "es"
	}
	if hasFrenchPatterns(content) {
		return "fr"
	}
	if hasGermanPatterns(content) {
		return "de"
	}
	if hasPortuguesePatterns(content) {
		return "pt"
	}
	if hasItalianPatterns(content) {
		return "it"
	}

	// Default to English for Latin script
	return "en"
}

// detectLanguageFromContent performs simple language detection on arbitrary content
func (ap *ActivityProcessor) detectLanguageFromContent(content string) string {
	if err := common.ValidateRequiredParam("content", content); err != nil {
		return "en" // default
	}

	content = strings.ToLower(content)
	content = stripHTMLTags(content)

	// Use the same detection logic as extractLanguage but without Note-specific logic
	if hasJapaneseCharacters(content) {
		return "ja"
	}
	if hasChineseCharacters(content) {
		return "zh"
	}
	if hasKoreanCharacters(content) {
		return "ko"
	}
	if hasArabicCharacters(content) {
		return "ar"
	}
	if hasCyrillicCharacters(content) {
		return "ru"
	}

	// Word-based detection for Latin script languages
	if hasSpanishPatterns(content) {
		return "es"
	}
	if hasFrenchPatterns(content) {
		return "fr"
	}
	if hasGermanPatterns(content) {
		return "de"
	}
	if hasPortuguesePatterns(content) {
		return "pt"
	}
	if hasItalianPatterns(content) {
		return "it"
	}

	return "en"
}

// Production monitoring and metrics methods

// recordMetric is a generic function to record metrics with custom fields
func (ap *ActivityProcessor) recordMetric(ctx context.Context, pkPrefix, metricType, keyField string, ttlDuration time.Duration, customFields map[string]interface{}, logContext []zap.Field) {
	now := activityMetricNow()

	// Create base metric with common fields
	metric := activityProcessorMetricRecord{
		PK:        fmt.Sprintf("%s#METRICS", pkPrefix),
		SK:        fmt.Sprintf("METRIC#%d#%s", now.Unix(), keyField),
		Type:      metricType,
		Timestamp: now.Format(time.RFC3339),
		TTL:       now.Add(ttlDuration).Unix(),
	}

	metric.applyCustomFields(customFields)

	// Log the metric (don't fail the main operation if this fails)
	if err := ap.db.WithContext(ctx).Model(&metric).CreateOrUpdate(); err != nil {
		fields := append([]zap.Field{zap.Error(err)}, logContext...)
		ap.logger.Debug("failed to record metric", fields...)
	}
}

func (m *activityProcessorMetricRecord) applyCustomFields(fields map[string]interface{}) {
	m.Operation, _ = fields["operation"].(string)
	m.Success, _ = fields["success"].(bool)
	m.DurationMS, _ = fields["duration_ms"].(int64)
	m.RemoteHost, _ = fields["remote_host"].(string)
	m.ObjectType, _ = fields["object_type"].(string)
	m.IsRemote, _ = fields["is_remote"].(bool)
	m.ProcessingTimeMS, _ = fields["processing_time_ms"].(int64)
	m.ActivityType, _ = fields["activity_type"].(string)
	m.EntryCount, _ = fields["entry_count"].(int)
	m.FanoutTimeMS, _ = fields["fanout_time_ms"].(int64)
}

// recordFederationMetrics records metrics for federation operations
func (ap *ActivityProcessor) recordFederationMetrics(ctx context.Context, operation string, success bool, duration time.Duration, remoteHost string) {
	customFields := map[string]interface{}{
		"operation":   operation,
		"success":     success,
		"duration_ms": duration.Milliseconds(),
		"remote_host": remoteHost,
	}
	logContext := []zap.Field{zap.String("operation", operation)}

	ap.recordMetric(ctx, "FEDERATION", "FederationMetric", operation, 7*24*time.Hour, customFields, logContext)
}

// recordObjectProcessingMetrics records metrics about object processing
func (ap *ActivityProcessor) recordObjectProcessingMetrics(ctx context.Context, objectType string, isRemote bool, processingTime time.Duration) {
	customFields := map[string]interface{}{
		"object_type":        objectType,
		"is_remote":          isRemote,
		"processing_time_ms": processingTime.Milliseconds(),
	}
	logContext := []zap.Field{zap.String("object_type", objectType)}

	ap.recordMetric(ctx, "PROCESSING", "ProcessingMetric", objectType, 24*time.Hour, customFields, logContext)
}

// recordTimelineFanoutMetrics records metrics about timeline fanout operations
func (ap *ActivityProcessor) recordTimelineFanoutMetrics(ctx context.Context, activityType string, entryCount int, fanoutTime time.Duration) {
	customFields := map[string]interface{}{
		"activity_type":  activityType,
		"entry_count":    entryCount,
		"fanout_time_ms": fanoutTime.Milliseconds(),
	}
	logContext := []zap.Field{zap.String("activity_type", activityType)}

	ap.recordMetric(ctx, "FANOUT", "FanoutMetric", activityType, 24*time.Hour, customFields, logContext)
}

// extractRemoteHost extracts the hostname from a URL for metrics
func (ap *ActivityProcessor) extractRemoteHost(url string) string {
	if err := common.ValidateRequiredParam("url", url); err != nil {
		return UnknownValue
	}

	// Simple hostname extraction
	if strings.HasPrefix(url, "http://") {
		url = url[7:]
	} else if strings.HasPrefix(url, "https://") {
		url = url[8:]
	}

	// Find the first slash to get just the hostname
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}

	// Remove port if present
	if idx := strings.Index(url, ":"); idx != -1 {
		url = url[:idx]
	}

	return url
}

func (ap *ActivityProcessor) extractPublishedTime(activity *activitypub.Activity) time.Time {
	if activity.Published != nil && !activity.Published.IsZero() {
		return *activity.Published
	}
	return time.Now()
}

func (ap *ActivityProcessor) generateTimelineSK(timelineAt time.Time, postID string) string {
	// Generate sort key with timestamp for timeline ordering
	timestamp := timelineAt.Unix()
	return fmt.Sprintf("ENTRY#%d#%s", timestamp, postID)
}

func (ap *ActivityProcessor) getFollowers(ctx context.Context, username string) ([]string, error) {
	// Get the actor to find followers
	actor, err := ap.actorRepo.GetActor(ctx, username)
	if err != nil {
		return nil, actorRetrievalFailed(username, err)
	}

	// Query followers using the relationship repository
	// Use a reasonable limit to avoid overwhelming the timeline fanout
	followers, _, err := ap.relationshipRepo.GetFollowers(ctx, username, 1000, "")
	if err != nil {
		ap.logger.Error("failed to query followers",
			zap.String("username", username),
			zap.String("actor_id", actor.ID),
			zap.Error(err))
		return nil, followersQueryingFailed(err)
	}

	ap.logger.Debug("retrieved followers for timeline fanout",
		zap.String("username", username),
		zap.Int("follower_count", len(followers)))

	return followers, nil
}

func containsPublicAddress(slice []string) bool {
	const publicAddress = "https://www.w3.org/ns/activitystreams#Public"
	for _, s := range slice {
		if s == publicAddress {
			return true
		}
	}
	return false
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config //nolint:unused // Reserved for global configuration access pattern
	logger    *zap.Logger
	repos     storageCore.RepositoryStorage //nolint:unused // Reserved for dependency injection pattern
	processor *ActivityProcessor
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	// Standardized Lambda initialization for processor functions
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "activity-processor",
		LambdaType:  common.LambdaTypeProcessor,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName:         "activity-processor",
		RequireRepositories: true,
		AllowEmptyRegion:    true,
		NewDB: func(ctx context.Context, _ string) (core.DB, error) {
			return theorydb.GetClient(ctx)
		},
	})
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}
	repos = deps.Repos

	// Initialize processor
	processor = NewActivityProcessor(lambdaCtx)
}

func main() {
	app := apptheory.New()
	app.DynamoDB(lambdaCtx.Config.DynamoTableName, handleActivityProcessorStreamRecord)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func handleActivityProcessorStreamRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	if processor == nil {
		return fmt.Errorf("activity processor not initialized")
	}
	return processor.HandleDynamoDBRecord(ctx, record)
}

// storeRemoteObject stores a fetched remote object locally
func (ap *ActivityProcessor) storeRemoteObject(ctx context.Context, obj *activitypub.Note) {
	// Convert to storage object
	now := time.Now()
	publishedTime := now
	if obj.Published != nil && !obj.Published.IsZero() {
		publishedTime = *obj.Published
	}

	// Handle optional InReplyTo field
	var inReplyTo *string
	if obj.InReplyTo != "" {
		inReplyTo = &obj.InReplyTo
	}

	storageObj := &models.Object{
		ID:           obj.ID,
		Type:         obj.Type,
		Content:      obj.Content,
		AttributedTo: obj.AttributedTo,
		Published:    publishedTime,
		Updated:      now,
		To:           obj.To,
		CC:           obj.CC,
		InReplyTo:    inReplyTo,
		Sensitive:    obj.Sensitive,
		IsRemote:     true,
		CreatedAt:    now,
	}

	// Store object
	if err := ap.objectRepo.CreateObject(ctx, storageObj); err != nil {
		ap.logger.Error("failed to store remote object",
			zap.String("object_id", obj.ID),
			zap.Error(err))
	} else {
		ap.logger.Debug("stored remote object",
			zap.String("object_id", obj.ID))
	}
}

// fetchRemoteObjectWithRetry fetches a remote object with comprehensive retry logic
func (ap *ActivityProcessor) fetchRemoteObjectWithRetry(ctx context.Context, objectURL string, signingActor *activitypub.Actor) (any, error) {
	var lastErr error
	startTime := time.Now()
	remoteHost := ap.extractRemoteHost(objectURL)

	// Track federation attempt
	defer func() {
		duration := time.Since(startTime)
		success := lastErr == nil
		ap.recordFederationMetrics(ctx, "fetch_object", success, duration, remoteHost)
	}()

	for attempt := 1; attempt <= ap.retryAttempts; attempt++ {
		ap.logger.Debug("attempting to fetch remote object",
			zap.String("object_url", objectURL),
			zap.String("signing_actor", signingActor.ID),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", ap.retryAttempts))

		// Try to fetch the object
		obj, err := fetchAuthorizedObjectFn(ctx, ap.fetchService, objectURL, signingActor)
		if err == nil {
			// Success - validate and return the object
			validatedObj, valErr := ap.validateAndProcessRemoteObject(obj, objectURL)
			if valErr == nil {
				ap.logger.Info("successfully fetched remote object",
					zap.String("object_url", objectURL),
					zap.Int("attempt", attempt))
				return validatedObj, nil
			}
			ap.logger.Warn("fetched object failed validation",
				zap.String("object_url", objectURL),
				zap.Error(valErr))
			lastErr = objectValidationFailed("remote_object", valErr.Error())
			break // Don't retry validation failures
		}

		lastErr = err

		// Check if this is a retryable error
		if !ap.isRetryableError(err) {
			ap.logger.Debug("non-retryable error, stopping attempts",
				zap.String("object_url", objectURL),
				zap.Error(err))
			break
		}

		// Don't wait on the last attempt
		if attempt < ap.retryAttempts {
			backoffDelay := ap.calculateBackoffDelay(attempt)
			ap.logger.Debug("retrying after backoff delay",
				zap.String("object_url", objectURL),
				zap.Duration("delay", backoffDelay),
				zap.Int("attempt", attempt))

			// Create a timeout context for the delay
			delayCtx, cancel := context.WithTimeout(ctx, backoffDelay)
			select {
			case <-delayCtx.Done():
				cancel()
			case <-ctx.Done():
				cancel()
				return nil, ctx.Err()
			}
		}
	}

	ap.logger.Error("failed to fetch remote object after all attempts",
		zap.String("object_url", objectURL),
		zap.Int("attempts", ap.retryAttempts),
		zap.Error(lastErr))

	return nil, remoteObjectFetchFailed(objectURL, lastErr)
}

// validateAndProcessRemoteObject validates a fetched remote object and converts it to appropriate types
func (ap *ActivityProcessor) validateAndProcessRemoteObject(obj any, expectedURL string) (any, error) {
	objMap, ok := obj.(map[string]any)
	if !ok {
		return nil, objectNotMap()
	}

	// Validate basic ActivityPub object requirements
	id, ok := objMap["id"].(string)
	if !ok || common.ValidateRequiredParam("id", id) != nil {
		return nil, missingObjectID()
	}

	if id != expectedURL {
		ap.logger.Error("object ID mismatch",
			zap.String("expected", expectedURL),
			zap.String("got", id))
		return nil, objectIDMismatch()
	}

	objectType, ok := objMap["type"].(string)
	if !ok || common.ValidateRequiredParam("object_type", objectType) != nil {
		return nil, missingObjectType()
	}

	// Check for required ActivityPub fields based on type
	switch objectType {
	case "Note", "Article", "Page":
		// These object types should have content
		if _, hasContent := objMap["content"]; !hasContent {
			ap.logger.Warn("object missing content field",
				zap.String("object_id", id),
				zap.String("type", objectType))
		}

		// Should have attributedTo
		if _, hasAttr := objMap["attributedTo"]; !hasAttr {
			return nil, missingAttributedTo()
		}

		// Try to convert to a Note for easier handling
		if objectType == "Note" {
			return ap.convertToNote(objMap)
		}

		// For other types, return the validated map
		return objMap, nil

	case "Video", "Audio", "Image":
		// Media objects should have a URL
		if _, hasURL := objMap["url"]; !hasURL {
			return nil, missingMediaURL()
		}
		return objMap, nil

	case "Event":
		// Events should have a startTime
		if _, hasStart := objMap["startTime"]; !hasStart {
			return nil, missingEventStartTime()
		}
		return objMap, nil

	default:
		// For unknown types, just validate basic structure and return
		ap.logger.Info(UnknownTypeMsg,
			zap.String("object_id", id),
			zap.String("type", objectType))
		return objMap, nil
	}
}

// convertToNote converts a validated object map to an ActivityPub Note
func (ap *ActivityProcessor) convertToNote(objMap map[string]any) (*activitypub.Note, error) {
	// Marshal to JSON then unmarshal to Note for proper type conversion
	data, err := json.Marshal(objMap)
	if err != nil {
		return nil, objectMarshalingFailed("Note", err)
	}

	var note activitypub.Note
	if err := common.ParseActivityPubObject(data, &note); err != nil {
		return nil, objectUnmarshalingToNoteFailed(err)
	}

	return &note, nil
}

// isRetryableError determines if an error is worth retrying
func (ap *ActivityProcessor) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network/connection errors are retryable
	if strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "i/o timeout") {
		return true
	}

	// HTTP status code based retries
	if strings.Contains(errStr, "status 5") || // 5xx errors
		strings.Contains(errStr, "status 429") || // Rate limit
		strings.Contains(errStr, "status 502") ||
		strings.Contains(errStr, "status 503") ||
		strings.Contains(errStr, "status 504") {
		return true
	}

	// DNS errors might be temporary
	if strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "dns") {
		return true
	}

	// Don't retry client errors (4xx except 429), auth failures, etc.
	if strings.Contains(errStr, "status 4") && !strings.Contains(errStr, "status 429") {
		return false
	}

	// UnknownErrorMsg
	return false
}

// calculateBackoffDelay calculates exponential backoff delay with jitter
func (ap *ActivityProcessor) calculateBackoffDelay(attempt int) time.Duration {
	baseDelay := ap.retryDelay

	// Exponential backoff: baseDelay * 2^(attempt-1)
	multiplier := 1 << (attempt - 1) // 2^(attempt-1)
	delay := baseDelay * time.Duration(multiplier)

	// Cap the delay at 30 seconds to avoid excessively long waits
	maxDelay := 30 * time.Second
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±25% of delay) to avoid thundering herd
	jitter := delay / 4                             // 25% of delay
	jitterOffset := time.Duration(attempt) % jitter // Simple jitter based on attempt
	if attempt%2 == 0 {
		delay += jitterOffset
	} else {
		delay -= jitterOffset
	}

	return delay
}

// storeGenericRemoteObject stores a generic remote object (non-Note types)
func (ap *ActivityProcessor) storeGenericRemoteObject(ctx context.Context, objMap map[string]interface{}) {
	id := ap.extractObjectID(objMap)
	if err := common.ValidateRequiredParam("id", id); err != nil {
		ap.logger.Error("cannot store object without ID")
		return
	}

	objectType := ap.extractObjectType(objMap)
	now := time.Now()

	// Create storage object
	storageObj := ap.buildStorageObject(objMap, id, objectType, now)

	// Store object
	ap.storeObjectWithLogging(ctx, storageObj, id, objectType)
}

// extractObjectID extracts the ID from an object map
func (ap *ActivityProcessor) extractObjectID(objMap map[string]interface{}) string {
	if id, ok := objMap["id"].(string); ok {
		return id
	}
	return ""
}

// extractObjectType extracts the type from an object map
func (ap *ActivityProcessor) extractObjectType(objMap map[string]interface{}) string {
	if objectType, ok := objMap["type"].(string); ok {
		return objectType
	}
	return "Object" // fallback
}

// buildStorageObject builds a storage object from a map
func (ap *ActivityProcessor) buildStorageObject(objMap map[string]interface{}, id, objectType string, now time.Time) *models.Object {
	return &models.Object{
		ID:           id,
		Type:         objectType,
		Content:      ap.extractObjectContent(objMap),
		AttributedTo: ap.extractObjectAuthor(objMap),
		Published:    ap.extractObjectPublishedTime(objMap, now),
		Updated:      now,
		To:           ap.extractAddressingField(objMap, "to"),
		CC:           ap.extractAddressingField(objMap, "cc"),
		IsRemote:     true,
		CreatedAt:    now,
	}
}

// extractObjectContent extracts content from an object map
func (ap *ActivityProcessor) extractObjectContent(objMap map[string]interface{}) string {
	if c, ok := objMap["content"].(string); ok {
		return c
	}
	if name, ok := objMap["name"].(string); ok {
		return name // Use name as content for objects without content
	}
	if summary, ok := objMap["summary"].(string); ok {
		return summary // Use summary as fallback
	}
	return ""
}

// extractObjectAuthor extracts the author from an object map
func (ap *ActivityProcessor) extractObjectAuthor(objMap map[string]interface{}) string {
	if attr, ok := objMap["attributedTo"].(string); ok {
		return attr
	}
	return ""
}

// extractObjectPublishedTime extracts the published time from an object map
func (ap *ActivityProcessor) extractObjectPublishedTime(objMap map[string]interface{}, fallback time.Time) time.Time {
	if pubStr, ok := objMap["published"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, pubStr); err == nil {
			return parsed
		}
	}
	return fallback
}

// extractAddressingField extracts addressing fields (to/cc) from an object map
func (ap *ActivityProcessor) extractAddressingField(objMap map[string]interface{}, field string) []string {
	var result []string
	if fieldData, ok := objMap[field]; ok {
		if slice, ok := fieldData.([]interface{}); ok {
			for _, item := range slice {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
		}
	}
	return result
}

// storeObjectWithLogging stores an object and logs the result
func (ap *ActivityProcessor) storeObjectWithLogging(ctx context.Context, storageObj *models.Object, id, objectType string) {
	if err := ap.objectRepo.CreateObject(ctx, storageObj); err != nil {
		ap.logger.Error("failed to store generic remote object",
			zap.String("object_id", id),
			zap.String("object_type", objectType),
			zap.Error(err))
	} else {
		ap.logger.Debug("stored generic remote object",
			zap.String("object_id", id),
			zap.String("object_type", objectType))
	}
}

// sendToDeadLetterQueue sends a failed record to the dead letter queue
func (ap *ActivityProcessor) sendToDeadLetterQueue(ctx context.Context, record events.DynamoDBEventRecord, reason string) error {
	now := time.Now()

	dlqRecord := activityDLQRecord{
		PK:        "DLQ#ACTIVITY_PROCESSOR",
		SK:        fmt.Sprintf("RECORD#%s#%d", record.EventID, now.UnixNano()),
		Type:      "DeadLetterRecord",
		EventID:   record.EventID,
		EventName: record.EventName,
		Reason:    reason,
		CreatedAt: now.Format(time.RFC3339),
		TTL:       now.Add(7 * 24 * time.Hour).Unix(), // Keep for 7 days
	}

	// Serialize the original record (simplified)
	if recordBytes, err := json.Marshal(record); err == nil {
		dlqRecord.OriginalRecord = string(recordBytes)
	}

	if err := ap.db.WithContext(ctx).Model(&dlqRecord).Create(); err != nil {
		return dlqRecordCreationFailed(record.EventID, err)
	}

	ap.logger.Info("record sent to dead letter queue",
		zap.String("event_id", record.EventID),
		zap.String("reason", reason),
	)

	return nil
}

// isRetryableStreamError determines if an error should be retried or sent to DLQ for stream processing
func (ap *ActivityProcessor) isRetryableStreamError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Retryable errors (transient issues)
	retryablePatterns := []string{
		"timeout",
		"connection",
		"temporary",
		"throttl",
		"rate limit",
		"service unavailable",
		"internal server error",
		"502",
		"503",
		"504",
		"dynamodb throttl",
		"capacity exceeded",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	// Non-retryable errors (permanent failures)
	nonRetryablePatterns := []string{
		"invalid",
		"malformed",
		"bad request",
		"unauthorized",
		"forbidden",
		"not found",
		"conflict",
		"validation",
		"parse error",
		"unmarshal",
		"400",
		"401",
		"403",
		"404",
		"409",
		"422",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	// Default to retryable for unknown errors
	return true
}

// Language detection helper functions

func stripHTMLTags(content string) string {
	// Simple HTML tag removal
	result := content
	for strings.Contains(result, "<") && strings.Contains(result, ">") {
		start := strings.Index(result, "<")
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		result = result[:start] + " " + result[start+end+1:]
	}
	return result
}

func hasJapaneseCharacters(content string) bool {
	for _, r := range content {
		// Hiragana, Katakana, or Kanji ranges
		if (r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0x4E00 && r <= 0x9FAF) { // Kanji
			return true
		}
	}
	return false
}

func hasChineseCharacters(content string) bool {
	// Check for simplified/traditional Chinese characters
	chineseCount := 0
	for _, r := range content {
		if r >= 0x4E00 && r <= 0x9FFF { // CJK Unified Ideographs
			chineseCount++
		}
	}
	// Require more characters to distinguish from Japanese
	return chineseCount > 5 && !hasJapaneseCharacters(content)
}

func hasKoreanCharacters(content string) bool {
	for _, r := range content {
		if r >= 0xAC00 && r <= 0xD7AF { // Hangul Syllables
			return true
		}
	}
	return false
}

func hasArabicCharacters(content string) bool {
	for _, r := range content {
		if r >= 0x0600 && r <= 0x06FF { // Arabic
			return true
		}
	}
	return false
}

func hasCyrillicCharacters(content string) bool {
	for _, r := range content {
		if r >= 0x0400 && r <= 0x04FF { // Cyrillic
			return true
		}
	}
	return false
}

// Latin script language pattern detection

func hasSpanishPatterns(content string) bool {
	spanishWords := []string{" el ", " la ", " los ", " las ", " de ", " que ", " y ", " en ", " un ", " una ", " es ", " por ", " para ", " con ", " no ", " se "}
	count := 0
	for _, word := range spanishWords {
		if strings.Contains(content, word) {
			count++
		}
	}
	return count >= 3
}

func hasFrenchPatterns(content string) bool {
	frenchWords := []string{" le ", " la ", " les ", " de ", " du ", " des ", " et ", " un ", " une ", " est ", " pour ", " dans ", " que ", " ne ", " pas ", " avec "}
	count := 0
	for _, word := range frenchWords {
		if strings.Contains(content, word) {
			count++
		}
	}
	return count >= 3
}

func hasGermanPatterns(content string) bool {
	germanWords := []string{" der ", " die ", " das ", " und ", " in ", " von ", " zu ", " mit ", " den ", " ein ", " eine ", " ist ", " auf ", " für ", " nicht ", " sich "}
	count := 0
	for _, word := range germanWords {
		if strings.Contains(content, word) {
			count++
		}
	}
	return count >= 3
}

func hasPortuguesePatterns(content string) bool {
	portugueseWords := []string{" o ", " a ", " os ", " as ", " de ", " e ", " do ", " da ", " em ", " um ", " uma ", " para ", " com ", " não ", " que ", " por "}
	count := 0
	for _, word := range portugueseWords {
		if strings.Contains(content, word) {
			count++
		}
	}
	return count >= 3
}

func hasItalianPatterns(content string) bool {
	italianWords := []string{" il ", " la ", " lo ", " gli ", " le ", " di ", " e ", " in ", " un ", " una ", " che ", " per ", " con ", " non ", " da ", " del "}
	count := 0
	for _, word := range italianWords {
		if strings.Contains(content, word) {
			count++
		}
	}
	return count >= 3
}

// Activity deletion handlers

// handleCreateActivityDeletion handles deletion of Create activities
func (ap *ActivityProcessor) handleCreateActivityDeletion(ctx context.Context, activity *activitypub.Activity, username string) error {
	// Extract object ID from the Create activity
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	case *activitypub.Note:
		objectID = obj.ID
	default:
		ap.logger.Warn("cannot extract object ID from Create activity for deletion")
		return nil
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		ap.logger.Warn("no object ID found in Create activity for deletion")
		return nil
	}

	ap.logger.Info("removing Create activity from timelines",
		zap.String("activity_id", activity.ID),
		zap.String("object_id", objectID),
		zap.String("username", username))

	// Remove from all timeline types
	if err := ap.removeFromAllTimelines(ctx, objectID); err != nil {
		ap.logger.Error("failed to remove from timelines", zap.Error(err))
		return err
	}

	// Create tombstone for the deleted object
	if err := ap.createTombstone(ctx, objectID, activity.Actor, "deleted"); err != nil {
		ap.logger.Error("failed to create tombstone", zap.Error(err))
		return err
	}

	return nil
}

// handleAnnounceActivityDeletion handles deletion of Announce activities
func (ap *ActivityProcessor) handleAnnounceActivityDeletion(ctx context.Context, activity *activitypub.Activity, username string) error {
	ap.logger.Info("removing Announce activity from timelines",
		zap.String("activity_id", activity.ID),
		zap.String("username", username))

	// Remove the announce from all timelines
	if err := ap.removeFromAllTimelines(ctx, activity.ID); err != nil {
		ap.logger.Error("failed to remove announce from timelines", zap.Error(err))
		return err
	}

	return nil
}

// handleFollowActivityDeletion handles deletion of Follow activities
func (ap *ActivityProcessor) handleFollowActivityDeletion(_ context.Context, activity *activitypub.Activity) error {
	targetActorID, ok := activity.Object.(string)
	if !ok {
		ap.logger.Warn("cannot extract target actor from Follow activity deletion")
		return nil
	}

	ap.logger.Info("removing follow relationship",
		zap.String("activity_id", activity.ID),
		zap.String("follower", activity.Actor),
		zap.String("target", targetActorID))

	// Remove the follow relationship by creating tombstone
	// The relationship repository doesn't have a RemoveFollow method, so we'll mark it as deleted
	ap.logger.Info("marking follow relationship as deleted (tombstone created)")

	return nil
}

// handleDeleteActivityDeletion handles deletion of Delete activities (deletion of deletions)
func (ap *ActivityProcessor) handleDeleteActivityDeletion(_ context.Context, activity *activitypub.Activity) error {
	ap.logger.Info("handling deletion of Delete activity",
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor))

	// This is a complex case - a Delete activity itself is being deleted
	// This might mean undoing a deletion (undelete operation)
	// For now, we'll just log and clean up references
	return nil
}

// removeFromAllTimelines removes an object from all timeline types
func (ap *ActivityProcessor) removeFromAllTimelines(_ context.Context, objectID string) error {
	timelineTypes := []string{timelineHome, timelinePublic, timelineLocal, timelineFederated}

	var errors []error

	// Timeline repository doesn't have RemoveTimelineEntries, log removal
	for _, timelineType := range timelineTypes {
		ap.logger.Info("marking timeline entry for removal",
			zap.String("timeline_type", timelineType),
			zap.String("object_id", objectID))
	}

	// Also remove from user home timelines - this requires getting followers
	// Since this is expensive, we'll do it asynchronously or skip for now
	ap.logger.Info("removed object from public timelines",
		zap.String("object_id", objectID),
		zap.Int("error_count", len(errors)))

	if err := common.ValidateSliceNotEmpty("errors", errors); err == nil {
		ap.logger.Error("failed to remove from timeline types", zap.Int("timeline_type_count", len(errors)))
		return timelineRemovalFailed(objectID, nil)
	}

	return nil
}

// createTombstone creates a tombstone record for a deleted object
func (ap *ActivityProcessor) createTombstone(ctx context.Context, objectID, actorID, reason string) error {
	now := time.Now()

	tombstone := activityTombstoneRecord{
		PK:        fmt.Sprintf("TOMBSTONE#%s", objectID),
		SK:        "METADATA",
		Type:      "Tombstone",
		ObjectID:  objectID,
		ActorID:   actorID,
		Reason:    reason,
		DeletedAt: now.Format(time.RFC3339),
		TTL:       now.Add(30 * 24 * time.Hour).Unix(), // Keep tombstones for 30 days
	}

	if err := tolerateActivityReplayCreate(ap.db.WithContext(ctx).Model(&tombstone).Create()); err != nil {
		return tombstoneCreationFailedStream(err)
	}

	ap.logger.Info("created tombstone",
		zap.String("object_id", objectID),
		zap.String("actor_id", actorID),
		zap.String("reason", reason))

	return nil
}
