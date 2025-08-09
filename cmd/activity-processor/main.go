// Package main implements the activity processor Lambda function that handles
// ActivityPub activities from DynamoDB streams and updates various timelines
// and notifications accordingly.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// Constants for common strings
const (
	// Timeline types
	timelineHome      = "HOME"
	timelinePublic    = "PUBLIC"
	timelineFederated = "FEDERATED"
	timelineLocal     = "LOCAL"
	
	// Activity types
	activityInsert = "INSERT"
	activityModify = "MODIFY"
	activityRemove = "REMOVE"
	UnknownValue    = "unknown"
	UnknownEventMsg = "unknown event type"
	UnknownTypeMsg  = "processing unknown object type"
	UnknownErrorMsg = "Default to not retrying unknown errors"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const requestIDKey contextKey = "request_id"

// ActivityProcessor handles ActivityPub activities from DynamoDB streams,
// processing them to update timelines, notifications, and other related data.
type ActivityProcessor struct {
	db               core.DB
	tableName        string
	logger           *zap.Logger
	timelineRepo     *repositories.TimelineRepository
	actorRepo        *repositories.ActorRepository
	userRepo         *repositories.UserRepository
	relationshipRepo *repositories.RelationshipRepository
	objectRepo       *repositories.ObjectRepository
	fetchService     *federation.AuthorizedFetchService
	storageAdapter   storageCore.RepositoryStorage
	baseURL          string
	retryAttempts    int
	retryDelay       time.Duration
}

// NewActivityProcessor creates a new activity processor instance with the given
// database connection, table name, and base URL for the instance.
func NewActivityProcessor(db core.DB, tableName string, baseURL string) *ActivityProcessor {
	// Get logger
	logger := common.Logger()

	// Initialize repositories
	timelineRepo := repositories.NewTimelineRepository(db, tableName, logger)
	actorRepo := repositories.NewActorRepository(db, tableName, logger)
	userRepo := repositories.NewUserRepository(db, tableName, logger)
	relationshipRepo := repositories.NewRelationshipRepository(db, tableName, logger)
	// Extract domain from baseURL
	domain := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")

	objectRepo := repositories.NewObjectRepository(db, tableName, domain, logger)

	// Create a complete storage adapter for federation service
	// This implements the full core.RepositoryStorage interface needed by AuthorizedFetchService
	adapter := NewStorageAdapter(db, tableName, logger)

	// Initialize authorized fetch service
	fetchService := federation.NewAuthorizedFetchService(adapter, domain, logger)

	return &ActivityProcessor{
		db:               db,
		tableName:        tableName,
		logger:           logger,
		timelineRepo:     timelineRepo,
		actorRepo:        actorRepo,
		userRepo:         userRepo,
		relationshipRepo: relationshipRepo,
		objectRepo:       objectRepo,
		fetchService:     fetchService,
		storageAdapter:   adapter,
		baseURL:          baseURL,
		retryAttempts:    3,
		retryDelay:       time.Second * 2,
	}
}

// HandleStream processes DynamoDB stream events with Lift-style patterns
func (ap *ActivityProcessor) HandleStream(ctx context.Context, event events.DynamoDBEvent) error {
	// Generate request ID for tracking (Lift pattern)
	requestID := uuid.New().String()

	// Add request ID to context for downstream use
	ctx = context.WithValue(ctx, requestIDKey, requestID)

	ap.logger.Info("processing activity stream batch",
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	// Process records in parallel with error collection
	var errorList []error
	var errorMutex sync.Mutex

	// Track batch processing metrics
	batchStartTime := time.Now()
	defer func() {
		batchDuration := time.Since(batchStartTime)

		// Record batch processing metrics
		batchMetric := struct {
			PK        string `dynamorm:"pk"`
			SK        string `dynamorm:"sk"`
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
			Records   int    `json:"record_count"`
			Errors    int    `json:"error_count"`
			Duration  int64  `json:"duration_ms"`
			Timestamp string `json:"timestamp"`
			TTL       int64  `dynamorm:"ttl"`
		}{
			PK:        "BATCH#METRICS",
			SK:        fmt.Sprintf("BATCH#%d#%s", batchStartTime.Unix(), requestID),
			Type:      "BatchProcessingMetric",
			RequestID: requestID,
			Records:   len(event.Records),
			Errors:    len(errorList),
			Duration:  batchDuration.Milliseconds(),
			Timestamp: batchStartTime.Format(time.RFC3339),
			TTL:       batchStartTime.Add(24 * time.Hour).Unix(),
		}

		if err := ap.db.WithContext(ctx).Model(&batchMetric).Create(); err != nil {
			ap.logger.Debug("failed to record batch metric", zap.Error(err))
		}
	}()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Limit concurrency to 10

	for _, record := range event.Records {
		wg.Add(1)
		sem <- struct{}{}

		go func(record events.DynamoDBEventRecord) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := ap.processRecord(ctx, record); err != nil {
				errorMutex.Lock()
				errorList = append(errorList, err)
				errorMutex.Unlock()

				ap.logger.Error("failed to process record",
					zap.String("event_id", record.EventID),
					zap.Error(err),
				)
			}
		}(record)
	}

	wg.Wait()

	if len(errorList) > 0 {
		return fmt.Errorf("partial batch failure: %d of %d records failed", len(errorList), len(event.Records))
	}

	return nil
}

func (ap *ActivityProcessor) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Parse the stream record into activity data
	var activity struct {
		PK        string `dynamorm:"pk"`
		SK        string `dynamorm:"sk"`
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
			return fmt.Errorf("no new image in record %s", record.EventID)
		}

		// Convert DynamoDB attribute values using DynamORM
		if err := stream.UnmarshalItem(record, &activity); err != nil {
			return fmt.Errorf("failed to unmarshal new image: %w", err)
		}

	case activityRemove:
		if record.Change.OldImage == nil {
			return fmt.Errorf("no old image in remove record %s", record.EventID)
		}

		// Convert DynamoDB attribute values using DynamORM
		// For REMOVE events, we need to handle the old image differently
		// For now, we'll skip processing REMOVE events since the stream.UnmarshalItem
		// function is designed for NewImage
		return nil

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
	PK        string `dynamorm:"pk"`
	SK        string `dynamorm:"sk"`
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
	PK        string `dynamorm:"pk"`
	SK        string `dynamorm:"sk"`
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
	PK        string `dynamorm:"pk"`
	SK        string `dynamorm:"sk"`
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

	// Handle activity deletion cleanup
	return ap.cleanupActivityReferences(ctx, activity)
}

func (ap *ActivityProcessor) processInboxActivity(ctx context.Context, activity struct {
	PK        string `dynamorm:"pk"`
	SK        string `dynamorm:"sk"`
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
	inboxRecord := struct {
		PK          string `dynamorm:"pk"`
		SK          string `dynamorm:"sk"`
		Type        string `json:"type"`
		ActivityPK  string `json:"activity_pk"`
		Username    string `json:"username"`
		ActorID     string `json:"actor_id"`
		ProcessedAt string `json:"processed_at"`
		Status      string `json:"status"`
		TTL         int64  `dynamorm:"ttl"`
	}{
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

	return ap.db.WithContext(ctx).Model(&inboxRecord).Create()
}

func (ap *ActivityProcessor) processOutboxActivity(ctx context.Context, activity struct {
	PK        string `dynamorm:"pk"`
	SK        string `dynamorm:"sk"`
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

	// Parse the activity JSON
	var activityData activitypub.Activity
	if err := json.Unmarshal([]byte(activity.Activity), &activityData); err != nil {
		ap.logger.Error("failed to parse activity", zap.Error(err))
		return fmt.Errorf("failed to parse activity: %w", err)
	}

	// Handle timeline fanout based on activity type
	switch activityData.Type {
	case activitypub.CreateType:
		if err := ap.fanOutToTimelines(ctx, &activityData, activity.Username); err != nil {
			ap.logger.Error("failed to fan out Create activity", zap.Error(err))
			return err
		}
	case activitypub.AnnounceType:
		if err := ap.fanOutAnnounceToTimelines(ctx, &activityData, activity.Username); err != nil {
			ap.logger.Error("failed to fan out Announce activity", zap.Error(err))
			return err
		}
	}

	// Create outbox processing record
	outboxRecord := struct {
		PK          string `dynamorm:"pk"`
		SK          string `dynamorm:"sk"`
		Type        string `json:"type"`
		ActivityPK  string `json:"activity_pk"`
		Username    string `json:"username"`
		ActorID     string `json:"actor_id"`
		ProcessedAt string `json:"processed_at"`
		Status      string `json:"status"`
		TTL         int64  `dynamorm:"ttl"`
	}{
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

	return ap.db.WithContext(ctx).Model(&outboxRecord).Create()
}

func (ap *ActivityProcessor) updateActivityMetrics(ctx context.Context, activity struct {
	PK        string `dynamorm:"pk"`
	SK        string `dynamorm:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	// Update activity metrics for analytics
	metricsRecord := struct {
		PK         string `dynamorm:"pk"`
		SK         string `dynamorm:"sk"`
		Type       string `json:"type"`
		ActivityPK string `json:"activity_pk"`
		Direction  string `json:"direction"`
		Username   string `json:"username"`
		UpdatedAt  string `json:"updated_at"`
		TTL        int64  `dynamorm:"ttl"`
	}{
		PK:         fmt.Sprintf("METRICS#ACTIVITY#%s", activity.Direction),
		SK:         fmt.Sprintf("UPDATE#%s", activity.PK),
		Type:       "ActivityMetrics",
		ActivityPK: activity.PK,
		Direction:  activity.Direction,
		Username:   activity.Username,
		UpdatedAt:  time.Now().Format(time.RFC3339),
		TTL:        time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days retention
	}

	return ap.db.WithContext(ctx).Model(&metricsRecord).Create()
}

func (ap *ActivityProcessor) cleanupActivityReferences(ctx context.Context, activity struct {
	PK        string `dynamorm:"pk"`
	SK        string `dynamorm:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
},
) error {
	// Create cleanup record for deleted activities
	cleanupRecord := struct {
		PK         string `dynamorm:"pk"`
		SK         string `dynamorm:"sk"`
		Type       string `json:"type"`
		ActivityPK string `json:"activity_pk"`
		Direction  string `json:"direction"`
		Username   string `json:"username"`
		DeletedAt  string `json:"deleted_at"`
		TTL        int64  `dynamorm:"ttl"`
	}{
		PK:         "CLEANUP#ACTIVITY",
		SK:         fmt.Sprintf("DELETED#%s", activity.PK),
		Type:       "ActivityCleanup",
		ActivityPK: activity.PK,
		Direction:  activity.Direction,
		Username:   activity.Username,
		DeletedAt:  time.Now().Format(time.RFC3339),
		TTL:        time.Now().Add(24 * time.Hour).Unix(), // 24 hours retention
	}

	return ap.db.WithContext(ctx).Model(&cleanupRecord).Create()
}

// ProcessedObject holds information about a processed ActivityPub object
type ProcessedObject struct {
	Note           *activitypub.Note
	Content        string
	IsRemote       bool
	ObjectID       string
	ContentType    string
	HasMedia       bool
	IsReply        bool
	InReplyTo      string
	Sensitive      bool
	SpoilerText    string
	Language       string
	Visibility     string
}

// fanOutToTimelines handles timeline fanout for Create activities with robust federation support
func (ap *ActivityProcessor) fanOutToTimelines(ctx context.Context, activity *activitypub.Activity, username string) error {
	// Process the object from the activity
	processedObj, err := ap.processActivityObject(ctx, activity)
	if err != nil {
		return fmt.Errorf("failed to process activity object: %w", err)
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
		return nil, fmt.Errorf("failed to marshal note: %w", err)
	}

	note := &activitypub.Note{}
	if err := json.Unmarshal(noteData, note); err != nil {
		return nil, fmt.Errorf("failed to unmarshal note: %w", err)
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
	if len(to) == 0 && len(cc) == 0 {
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
	if len(entries) > 0 {
		if err := ap.timelineRepo.CreateTimelineEntries(ctx, entries); err != nil {
			return fmt.Errorf("failed to write timeline entries: %w", err)
		}
	}

	// Record metrics and log success
	ap.recordFanoutSuccess(ctx, obj, len(entries), time.Since(now))

	return nil
}

// createBaseTimelineEntry creates the base timeline entry
func (ap *ActivityProcessor) createBaseTimelineEntry(activity *activitypub.Activity, username string, obj *ProcessedObject, now time.Time) models.Timeline {
	content := obj.Content
	if len(content) > 500 {
		content = content[:500]
	}

	objectID := obj.ObjectID
	if objectID == "" {
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
	if visibility != "public" {
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
	if visibility == "direct" {
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
	if announcedID == "" {
		return fmt.Errorf("no object ID in Announce activity")
	}

	// Get the announced content
	announcedContent, originalAuthor := ap.getAnnouncedContent(ctx, activity, announcedID)
	_ = originalAuthor // Keep for future use

	// Create timeline entries
	entries := ap.createAnnounceTimelineEntries(ctx, activity, username, announcedContent)

	// Write all entries
	if len(entries) > 0 {
		if err := ap.timelineRepo.CreateTimelineEntries(ctx, entries); err != nil {
			return fmt.Errorf("failed to write timeline entries: %w", err)
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

	baseEntry := ap.createBaseAnnounceEntry(activity, username, announcedContent, now)

	// Add to public timelines
	entries = append(entries, ap.createPublicTimelineEntry(baseEntry, now, activity.ID))

	// Add local timeline entry if applicable
	if ap.isLocalActor(activity.Actor) {
		entries = append(entries, ap.createLocalTimelineEntry(baseEntry, now, activity.ID))
	}

	// Add to author's home timeline
	entries = append(entries, ap.createHomeTimelineEntry(baseEntry, username, now, activity.ID))

	// Fan out to followers
	ap.addFollowerEntries(ctx, &entries, baseEntry, username, now, activity.ID)

	return entries
}

// createBaseAnnounceEntry creates the base timeline entry for an announce
func (ap *ActivityProcessor) createBaseAnnounceEntry(activity *activitypub.Activity, username, announcedContent string, now time.Time) models.Timeline {
	return models.Timeline{
		PostID:      activity.ID,
		ActorID:     activity.Actor,
		ActorHandle: username,
		Content:     announcedContent,
		ContentType: "Announce",
		IsBoost:     true,
		BoostedBy:   username,
		Visibility:  "public", // Announces are typically public
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

func (ap *ActivityProcessor) determineVisibility(to, cc []string) string {
	// Direct message - no public addressing
	if !containsPublicAddress(to) && !containsPublicAddress(cc) {
		return "direct"
	}

	// Public - addressed to public in 'to'
	if containsPublicAddress(to) {
		return "public"
	}

	// Unlisted - public in 'cc'
	if containsPublicAddress(cc) {
		return "unlisted"
	}

	// Private - followers only
	return "private"
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
	if content == "" {
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
	now := time.Now()

	// Create base metric with common fields
	metric := map[string]interface{}{
		"PK":        fmt.Sprintf("%s#METRICS", pkPrefix),
		"SK":        fmt.Sprintf("METRIC#%d#%s", now.Unix(), keyField),
		"Type":      metricType,
		"Timestamp": now.Format(time.RFC3339),
		"TTL":       now.Add(ttlDuration).Unix(),
	}

	// Add custom fields
	for k, v := range customFields {
		metric[k] = v
	}

	// Log the metric (don't fail the main operation if this fails)
	if err := ap.db.WithContext(ctx).Model(&metric).Create(); err != nil {
		fields := append([]zap.Field{zap.Error(err)}, logContext...)
		ap.logger.Debug("failed to record metric", fields...)
	}
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
	if url == "" {
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
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Query followers using the relationship repository
	// Use a reasonable limit to avoid overwhelming the timeline fanout
	followers, _, err := ap.relationshipRepo.GetFollowers(ctx, username, 1000, "")
	if err != nil {
		ap.logger.Error("failed to query followers",
			zap.String("username", username),
			zap.String("actor_id", actor.ID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query followers: %w", err)
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
	logger    *zap.Logger
	cfg       *config.Config
	processor *ActivityProcessor
	db        core.DB
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize processor
	processor = NewActivityProcessor(db, cfg.DynamoTableName, cfg.BaseURL())
}

func main() {
	// DynamoDB Stream handler with Lift-style patterns but traditional Lambda execution
	// This provides structured logging, error handling, and request tracking
	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		start := time.Now()

		// Recovery handling (Lift pattern)
		defer func() {
			if r := recover(); r != nil {
				requestID := ctx.Value("request_id")
				if requestID == nil {
					requestID = UnknownValue
				}
				logger.Error("panic in DynamoDB stream handler",
					zap.Any("request_id", requestID),
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
			}
		}()

		// Process the stream event
		err := processor.HandleStream(ctx, event)

		// Log completion (Lift pattern)
		duration := time.Since(start)
		requestID := ctx.Value("request_id")
		if requestID == nil {
			requestID = UnknownValue
		}

		if err != nil {
			logger.Error("DynamoDB stream processing failed",
				zap.Any("request_id", requestID),
				zap.Error(err),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
		} else {
			logger.Info("DynamoDB stream processing completed",
				zap.Any("request_id", requestID),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
		}

		return err
	})
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
		obj, err := ap.fetchService.FetchObject(ctx, objectURL, signingActor)
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
			lastErr = fmt.Errorf("validation failed: %w", valErr)
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

	return nil, fmt.Errorf("failed after %d attempts: %w", ap.retryAttempts, lastErr)
}

// validateAndProcessRemoteObject validates a fetched remote object and converts it to appropriate types
func (ap *ActivityProcessor) validateAndProcessRemoteObject(obj any, expectedURL string) (any, error) {
	objMap, ok := obj.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("object is not a map[string]any")
	}

	// Validate basic ActivityPub object requirements
	id, ok := objMap["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("object missing or invalid 'id' field")
	}

	if id != expectedURL {
		return nil, fmt.Errorf("object ID mismatch: expected %s, got %s", expectedURL, id)
	}

	objectType, ok := objMap["type"].(string)
	if !ok || objectType == "" {
		return nil, fmt.Errorf("object missing or invalid 'type' field")
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
			return nil, fmt.Errorf("object missing 'attributedTo' field")
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
			return nil, fmt.Errorf("media object missing 'url' field")
		}
		return objMap, nil

	case "Event":
		// Events should have a startTime
		if _, hasStart := objMap["startTime"]; !hasStart {
			return nil, fmt.Errorf("event object missing 'startTime' field")
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
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}

	var note activitypub.Note
	if err := common.ParseActivityPubObject(data, &note); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to Note: %w", err)
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
	if id == "" {
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

// StorageAdapter implements the complete core.RepositoryStorage interface
// This provides all repositories needed by the federation system
type StorageAdapter struct {
	db        core.DB
	tableName string
	logger    *zap.Logger

	// Repository instances - initialized once and reused
	accountRepo          *repositories.AccountRepository
	actorRepo            *repositories.ActorRepository
	objectRepo           *repositories.ObjectRepository
	activityRepo         *repositories.ActivityRepository
	timelineRepo         *repositories.TimelineRepository
	notificationRepo     *repositories.NotificationRepository
	likeRepo             *repositories.LikeRepository
	moderationRepo       *repositories.ModerationRepository
	listRepo             *repositories.ListRepository
	mediaRepo            *repositories.MediaRepository
	pollRepo             *repositories.PollRepository
	pushSubscriptionRepo *repositories.PushSubscriptionRepository
	hashtagRepo          *repositories.HashtagRepository
	scheduledStatusRepo  *repositories.ScheduledStatusRepository
	announcementRepo     *repositories.AnnouncementRepository
	domainBlockRepo      *repositories.DomainBlockRepository
	relationshipRepo     *repositories.RelationshipRepository
	instanceRepo         *repositories.InstanceRepository
	federationRepo       *repositories.FederationRepository
	recoveryRepo         *repositories.RecoveryRepository
	analyticsRepo        *repositories.TrendingRepository
	socialRepo           *repositories.SocialRepository
	userRepo             *repositories.UserRepository
	statusRepo           *repositories.StatusRepository
	costRepo             *repositories.CostTrackingRepository
	trustRepo            *repositories.TrustRepository
	searchRepo           *repositories.SearchRepository
	relayRepo            *repositories.RelayRepository
	communityNoteRepo    *repositories.CommunityNoteRepository
	emojiRepo            *repositories.EmojiRepository
	rateLimitRepo        *repositories.RateLimitRepository
	conversationRepo     *repositories.ConversationRepository
	markerRepo           *repositories.MarkerRepository
	featuredTagRepo      *repositories.FeaturedTagRepository
	aiRepo               *repositories.AIRepository
	exportRepo           *repositories.ExportRepository
	importRepo           *repositories.ImportRepository
	dlqRepo              *repositories.DLQRepository
	metricRecordRepo     *repositories.MetricRecordRepository
	cloudWatchMetricsRepo *repositories.CloudWatchMetricsRepository
}

// NewStorageAdapter creates a new complete storage adapter with all repositories
func NewStorageAdapter(db core.DB, tableName string, logger *zap.Logger) *StorageAdapter {
	// Extract domain from environment or config for object repository
	domain := os.Getenv("DOMAIN_NAME")
	if domain == "" {
		cfg := config.Get()
		if cfg != nil {
			domain = strings.TrimPrefix(strings.TrimPrefix(cfg.BaseURL(), "https://"), "http://")
		}
	}
	if domain == "" {
		domain = "localhost" // fallback
	}

	// Load AWS config for CloudWatch metrics
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Warn("failed to load AWS config for CloudWatch metrics", zap.Error(err))
		// Use empty config as fallback
		awsCfg = aws.Config{}
	}

	return &StorageAdapter{
		db:        db,
		tableName: tableName,
		logger:    logger,

		// Initialize all repositories
		accountRepo:          repositories.NewAccountRepository(db, tableName, domain, logger),
		actorRepo:            repositories.NewActorRepository(db, tableName, logger),
		objectRepo:           repositories.NewObjectRepository(db, tableName, domain, logger),
		activityRepo:         repositories.NewActivityRepository(db, tableName, logger),
		timelineRepo:         repositories.NewTimelineRepository(db, tableName, logger),
		notificationRepo:     repositories.NewNotificationRepository(db, tableName, logger),
		likeRepo:             repositories.NewLikeRepository(db, tableName, logger),
		moderationRepo:       repositories.NewModerationRepository(db, tableName, logger),
		listRepo:             repositories.NewListRepository(db, tableName, logger),
		mediaRepo:            repositories.NewMediaRepository(db, tableName, logger),
		pollRepo:             repositories.NewPollRepository(db, tableName, logger),
		pushSubscriptionRepo: repositories.NewPushSubscriptionRepository(db, tableName, logger),
		hashtagRepo:          repositories.NewHashtagRepository(db, tableName, logger, domain),
		scheduledStatusRepo:  repositories.NewScheduledStatusRepository(db, tableName, logger),
		announcementRepo:     repositories.NewAnnouncementRepository(db, tableName, logger),
		domainBlockRepo:      repositories.NewDomainBlockRepository(db, tableName, logger),
		relationshipRepo:     repositories.NewRelationshipRepository(db, tableName, logger),
		instanceRepo:         repositories.NewInstanceRepository(db, tableName, logger),
		federationRepo:       repositories.NewFederationRepository(db, logger),
		recoveryRepo:         repositories.NewRecoveryRepository(db, tableName, logger),
		analyticsRepo:        repositories.NewTrendingRepository(db, logger),
		socialRepo:           repositories.NewSocialRepository(db, logger),
		userRepo:             repositories.NewUserRepository(db, tableName, logger),
		statusRepo:           repositories.NewStatusRepository(db, tableName, logger),
		costRepo:             repositories.NewCostTrackingRepository(db, tableName, logger),
		trustRepo:            repositories.NewTrustRepository(db, logger),
		searchRepo:           repositories.NewSearchRepository(db, logger),
		relayRepo:            repositories.NewRelayRepository(db, tableName, logger),
		communityNoteRepo:    repositories.NewCommunityNoteRepository(db, tableName, logger),
		emojiRepo:            repositories.NewEmojiRepository(db, logger),
		rateLimitRepo:        repositories.NewRateLimitRepository(db, tableName, logger),
		conversationRepo:     repositories.NewConversationRepository(db, logger),
		markerRepo:           repositories.NewMarkerRepository(db, tableName, logger),
		featuredTagRepo:      repositories.NewFeaturedTagRepository(db, tableName, logger),
		aiRepo:               repositories.NewAIRepository(db, tableName, logger),
		exportRepo:           repositories.NewExportRepository(db, tableName, logger),
		importRepo:           repositories.NewImportRepository(db, tableName, logger),
		dlqRepo:              repositories.NewDLQRepository(db, tableName, logger),
		metricRecordRepo:     repositories.NewMetricRecordRepository(db, tableName, logger),
		cloudWatchMetricsRepo: repositories.NewCloudWatchMetricsRepository(awsCfg, "Lesser/Production", "prod", logger),
	}
}

// Implement all core.RepositoryStorage interface methods

// Account returns the account repository for user account operations
func (s *StorageAdapter) Account() *repositories.AccountRepository { return s.accountRepo }

// Actor returns the actor repository for ActivityPub actor operations  
func (s *StorageAdapter) Actor() *repositories.ActorRepository { return s.actorRepo }

// Object returns the object repository for ActivityPub object operations
func (s *StorageAdapter) Object() *repositories.ObjectRepository { return s.objectRepo }

// Activity returns the activity repository for ActivityPub activity operations
func (s *StorageAdapter) Activity() *repositories.ActivityRepository { return s.activityRepo }

// Timeline returns the timeline repository for timeline operations
func (s *StorageAdapter) Timeline() *repositories.TimelineRepository { return s.timelineRepo }

// Notification returns the notification repository for notification operations
func (s *StorageAdapter) Notification() *repositories.NotificationRepository {
	return s.notificationRepo
}

// Like returns the like repository for like/favorite operations
func (s *StorageAdapter) Like() *repositories.LikeRepository { return s.likeRepo }

// Moderation returns the moderation repository for moderation operations
func (s *StorageAdapter) Moderation() *repositories.ModerationRepository { return s.moderationRepo }

// List returns the list repository for list operations
func (s *StorageAdapter) List() *repositories.ListRepository { return s.listRepo }
// Media returns the media repository for media operations
func (s *StorageAdapter) Media() *repositories.MediaRepository { return s.mediaRepo }

// Poll returns the poll repository for poll operations
func (s *StorageAdapter) Poll() *repositories.PollRepository { return s.pollRepo }

// PushSubscription returns the push subscription repository for push notification operations
func (s *StorageAdapter) PushSubscription() *repositories.PushSubscriptionRepository {
	return s.pushSubscriptionRepo
}
// Hashtag returns the hashtag repository for hashtag operations
func (s *StorageAdapter) Hashtag() *repositories.HashtagRepository { return s.hashtagRepo }
// ScheduledStatus returns the scheduled status repository for scheduled post operations
func (s *StorageAdapter) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return s.scheduledStatusRepo
}
// Announcement returns the announcement repository for announcement operations
func (s *StorageAdapter) Announcement() *repositories.AnnouncementRepository {
	return s.announcementRepo
}
// DomainBlock returns the domain block repository for domain blocking operations
func (s *StorageAdapter) DomainBlock() *repositories.DomainBlockRepository { return s.domainBlockRepo }
// Relationship returns the relationship repository for relationship operations
func (s *StorageAdapter) Relationship() *repositories.RelationshipRepository {
	return s.relationshipRepo
}
// Instance returns the instance repository for instance operations
func (s *StorageAdapter) Instance() *repositories.InstanceRepository     { return s.instanceRepo }
// Federation returns the federation repository for federation operations
func (s *StorageAdapter) Federation() *repositories.FederationRepository { return s.federationRepo }
// Recovery returns the recovery repository for recovery operations
func (s *StorageAdapter) Recovery() *repositories.RecoveryRepository     { return s.recoveryRepo }
// Analytics returns the analytics repository for analytics operations
func (s *StorageAdapter) Analytics() *repositories.TrendingRepository    { return s.analyticsRepo }
// Social returns the social repository for social operations
func (s *StorageAdapter) Social() *repositories.SocialRepository         { return s.socialRepo }
// User returns the user repository for user operations
func (s *StorageAdapter) User() *repositories.UserRepository             { return s.userRepo }
// Status returns the status repository for status operations
func (s *StorageAdapter) Status() *repositories.StatusRepository         { return s.statusRepo }
// Cost returns the cost tracking repository for cost operations
func (s *StorageAdapter) Cost() *repositories.CostTrackingRepository     { return s.costRepo }
// Trust returns the trust repository for trust operations
func (s *StorageAdapter) Trust() *repositories.TrustRepository           { return s.trustRepo }
// Search returns the search repository for search operations
func (s *StorageAdapter) Search() *repositories.SearchRepository         { return s.searchRepo }
// Relay returns the relay repository for relay operations
func (s *StorageAdapter) Relay() *repositories.RelayRepository           { return s.relayRepo }
// CommunityNote returns the community note repository for community note operations
func (s *StorageAdapter) CommunityNote() *repositories.CommunityNoteRepository {
	return s.communityNoteRepo
}
// Emoji returns the emoji repository for emoji operations
func (s *StorageAdapter) Emoji() *repositories.EmojiRepository         { return s.emojiRepo }
// RateLimit returns the rate limit repository for rate limiting operations
func (s *StorageAdapter) RateLimit() *repositories.RateLimitRepository { return s.rateLimitRepo }
// Conversation returns the conversation repository for conversation operations
func (s *StorageAdapter) Conversation() *repositories.ConversationRepository {
	return s.conversationRepo
}
// Marker returns the marker repository for marker operations
func (s *StorageAdapter) Marker() *repositories.MarkerRepository           { return s.markerRepo }
// FeaturedTag returns the featured tag repository for featured tag operations
func (s *StorageAdapter) FeaturedTag() *repositories.FeaturedTagRepository { return s.featuredTagRepo }
// AI returns the AI repository for AI operations
func (s *StorageAdapter) AI() *repositories.AIRepository                   { return s.aiRepo }
// Export returns the export repository for export operations
func (s *StorageAdapter) Export() *repositories.ExportRepository           { return s.exportRepo }
// Import returns the import repository for import operations
func (s *StorageAdapter) Import() *repositories.ImportRepository           { return s.importRepo }
// DLQ returns the DLQ repository for dead letter queue operations
func (s *StorageAdapter) DLQ() *repositories.DLQRepository                 { return s.dlqRepo }

// Utility methods

// GetDB returns the underlying database connection
func (s *StorageAdapter) GetDB() core.DB {
	return s.db
}

// GetTableName returns the DynamoDB table name
func (s *StorageAdapter) GetTableName() string {
	return s.tableName
}

// GetLogger returns the logger instance
func (s *StorageAdapter) GetLogger() *zap.Logger {
	return s.logger
}

// MetricRecord returns the metric record repository for metric operations
func (s *StorageAdapter) MetricRecord() *repositories.MetricRecordRepository {
	return s.metricRecordRepo
}

// CloudWatchMetrics returns the CloudWatch metrics repository for CloudWatch operations
func (s *StorageAdapter) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository {
	return s.cloudWatchMetricsRepo
}
