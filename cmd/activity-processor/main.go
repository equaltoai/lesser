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
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
)

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
	ctx = context.WithValue(ctx, "request_id", requestID)

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
			PK         string `dynamorm:"pk"`
			SK         string `dynamorm:"sk"`
			Type       string `json:"type"`
			RequestID  string `json:"request_id"`
			Records    int    `json:"record_count"`
			Errors     int    `json:"error_count"`
			Duration   int64  `json:"duration_ms"`
			Timestamp  string `json:"timestamp"`
			TTL        int64  `dynamorm:"ttl"`
		}{
			PK:         "BATCH#METRICS",
			SK:         fmt.Sprintf("BATCH#%d#%s", batchStartTime.Unix(), requestID),
			Type:       "BatchProcessingMetric",
			RequestID:  requestID,
			Records:    len(event.Records),
			Errors:     len(errorList),
			Duration:   batchDuration.Milliseconds(),
			Timestamp:  batchStartTime.Format(time.RFC3339),
			TTL:        batchStartTime.Add(24 * time.Hour).Unix(),
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
	case "INSERT", "MODIFY":
		if record.Change.NewImage == nil {
			return fmt.Errorf("no new image in record %s", record.EventID)
		}

		// Convert DynamoDB attribute values using DynamORM
		if err := stream.UnmarshalItem(record, &activity); err != nil {
			return fmt.Errorf("failed to unmarshal new image: %w", err)
		}

	case "REMOVE":
		if record.Change.OldImage == nil {
			return fmt.Errorf("no old image in remove record %s", record.EventID)
		}

		// Convert DynamoDB attribute values using DynamORM
		// For REMOVE events, we need to handle the old image differently
		// For now, we'll skip processing REMOVE events since the stream.UnmarshalItem
		// function is designed for NewImage
		return nil

	default:
		ap.logger.Warn("unknown event type",
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
	case "INSERT":
		return ap.processActivityCreated(ctx, activity)
	case "MODIFY":
		return ap.processActivityUpdated(ctx, activity)
	case "REMOVE":
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

// fanOutToTimelines handles timeline fanout for Create activities with robust federation support
func (ap *ActivityProcessor) fanOutToTimelines(ctx context.Context, activity *activitypub.Activity, username string) error {
	// Extract the note/object from the activity with enhanced handling
	var note *activitypub.Note
	var objectContent string
	var isRemoteObject bool
	
	switch obj := activity.Object.(type) {
	case map[string]interface{}:
		// Check if this is a local embedded object or a remote reference
		if id, hasID := obj["id"].(string); hasID && !strings.HasPrefix(id, ap.baseURL) {
			// This is a remote object reference - fetch it
			isRemoteObject = true
			ap.logger.Debug("detected remote object in Create activity", 
				zap.String("object_id", id),
				zap.String("actor", activity.Actor))
			
			// Get the signing actor for remote fetch
			signingActor, err := ap.actorRepo.GetActor(ctx, activity.Actor)
			if err != nil {
				ap.logger.Error("failed to get signing actor for remote object fetch", zap.Error(err))
				// Use the embedded object as fallback
				objectContent = fmt.Sprintf("Remote object: %s", id)
			} else {
				// Attempt to fetch the remote object
				remoteObj, err := ap.fetchRemoteObjectWithRetry(ctx, id, signingActor)
				if err != nil {
					ap.logger.Warn("failed to fetch remote object in Create activity",
						zap.String("object_id", id),
						zap.Error(err))
					// Use fallback content
					objectContent = fmt.Sprintf("Remote object: %s", id)
				} else {
					// Successfully fetched remote object
					if fetchedNote, ok := remoteObj.(*activitypub.Note); ok {
						note = fetchedNote
						ap.logger.Info("successfully fetched remote object for Create activity",
							zap.String("object_id", id))
					} else {
						// Handle non-Note remote objects
						if objMap, ok := remoteObj.(map[string]interface{}); ok {
							if content, ok := objMap["content"].(string); ok {
								objectContent = content
							} else {
								objectContent = fmt.Sprintf("Remote object: %s", id)
							}
						}
					}
				}
			}
		} else {
			// Local embedded object - convert map to Note
			noteData, err := json.Marshal(obj)
			if err != nil {
				return fmt.Errorf("failed to marshal note: %w", err)
			}
			note = &activitypub.Note{}
			if err := json.Unmarshal(noteData, note); err != nil {
				return fmt.Errorf("failed to unmarshal note: %w", err)
			}
		}
	case *activitypub.Note:
		note = obj
	case string:
		// Object is just an ID reference - need to fetch it
		isRemoteObject = true
		objectID := obj
		
		// Check if it's local first
		existingObj, err := ap.objectRepo.GetObject(ctx, objectID)
		if err == nil && existingObj != nil {
			// Found locally - extract content
			if localNote, ok := existingObj.(*models.Object); ok {
				objectContent = localNote.Content
				ap.logger.Debug("found referenced object locally", zap.String("object_id", objectID))
			}
		} else {
			// Not found locally - fetch remotely if it's a remote URL
			if !strings.HasPrefix(objectID, ap.baseURL) {
				signingActor, err := ap.actorRepo.GetActor(ctx, activity.Actor)
				if err != nil {
					ap.logger.Error("failed to get signing actor for object reference fetch", zap.Error(err))
					objectContent = fmt.Sprintf("Referenced object: %s", objectID)
				} else {
					remoteObj, err := ap.fetchRemoteObjectWithRetry(ctx, objectID, signingActor)
					if err != nil {
						ap.logger.Warn("failed to fetch referenced object",
							zap.String("object_id", objectID),
							zap.Error(err))
						objectContent = fmt.Sprintf("Referenced object: %s", objectID)
					} else {
						if fetchedNote, ok := remoteObj.(*activitypub.Note); ok {
							note = fetchedNote
							ap.logger.Info("successfully fetched referenced object",
								zap.String("object_id", objectID))
						} else if objMap, ok := remoteObj.(map[string]interface{}); ok {
							if content, ok := objMap["content"].(string); ok {
								objectContent = content
							}
						}
					}
				}
			} else {
				// Local object that doesn't exist - this shouldn't happen
				ap.logger.Warn("local object reference not found", zap.String("object_id", objectID))
				objectContent = fmt.Sprintf("Missing local object: %s", objectID)
			}
		}
	default:
		ap.logger.Warn("unsupported object type in Create activity", zap.Any("object", activity.Object))
		return nil
	}
	
	// If we have a note, use its content, otherwise use the extracted content
	if note != nil {
		objectContent = note.Content
	}

	// Determine visibility from addressing (use note if available, otherwise activity addressing)
	var to, cc []string
	if note != nil {
		to, cc = note.To, note.CC
	} else {
		// Fall back to activity addressing or default to public
		to = activity.To
		cc = activity.CC
		if len(to) == 0 && len(cc) == 0 {
			// Default to public for activities without explicit addressing
			to = []string{"https://www.w3.org/ns/activitystreams#Public"}
		}
	}
	visibility := ap.determineVisibility(to, cc)
	
	// Create timeline entries
	var entries []*models.Timeline
	now := time.Now()

	// Extract content and metadata
	content := objectContent
	if len(content) > 500 {
		content = content[:500]
	}

	// Determine object ID and content type
	var objectID string
	var contentType string
	var hasMedia bool
	var isReply bool
	var inReplyTo string
	var sensitive bool
	var spoilerText string
	var language string
	
	if note != nil {
		objectID = note.ID
		contentType = "Note"
		hasMedia = len(note.Attachment) > 0
		isReply = note.InReplyTo != ""
		inReplyTo = note.InReplyTo
		sensitive = note.Sensitive
		spoilerText = note.Summary
		language = ap.extractLanguage(note)
	} else {
		// For non-Note objects, extract what we can from the activity
		if activity.Object != nil {
			switch obj := activity.Object.(type) {
			case map[string]interface{}:
				if id, ok := obj["id"].(string); ok {
					objectID = id
				}
				if objType, ok := obj["type"].(string); ok {
					contentType = objType
				} else {
					contentType = "Object"
				}
				if replyTo, ok := obj["inReplyTo"].(string); ok && replyTo != "" {
					isReply = true
					inReplyTo = replyTo
				}
				if sens, ok := obj["sensitive"].(bool); ok {
					sensitive = sens
				}
				if summary, ok := obj["summary"].(string); ok {
					spoilerText = summary
				}
			case string:
				objectID = obj
				contentType = "Object"
			}
		}
		if objectID == "" {
			objectID = activity.ID
		}
		if contentType == "" {
			contentType = "Create"
		}
		// Use simple language detection on content
		language = ap.detectLanguageFromContent(content)
	}

	// Base entry for all timelines
	baseEntry := models.Timeline{
		PostID:      objectID,
		ActorID:     activity.Actor,
		ActorHandle: username,
		Content:     content,
		ContentType: contentType,
		HasMedia:    hasMedia,
		IsReply:     isReply,
		InReplyTo:   inReplyTo,
		IsBoost:     false,
		Visibility:  visibility,
		Language:    language,
		Sensitive:   sensitive,
		SpoilerText: spoilerText,
		CreatedAt:   ap.extractPublishedTime(activity),
		TimelineAt:  now,
	}

	// Add to public timelines if public
	if visibility == "public" {
		// Federated timeline
		publicEntry := baseEntry
		publicEntry.TimelineType = "PUBLIC"
		publicEntry.TimelineID = "FEDERATED"
		publicEntry.EntryID = ap.generateTimelineSK(now, note.ID)
		entries = append(entries, &publicEntry)

		// Local timeline if it's a local user
		if strings.HasPrefix(activity.Actor, ap.baseURL) {
			localEntry := baseEntry
			localEntry.TimelineType = "PUBLIC"
			localEntry.TimelineID = "LOCAL"
			localEntry.EntryID = ap.generateTimelineSK(now, note.ID)
			entries = append(entries, &localEntry)
		}
	}

	// Add to home timeline of the author
	homeEntry := baseEntry
	homeEntry.TimelineType = "HOME"
	homeEntry.TimelineID = username
	homeEntry.EntryID = ap.generateTimelineSK(now, note.ID)
	entries = append(entries, &homeEntry)

	// Fan out to followers' home timelines (for all visibility except direct)
	if visibility != "direct" {
		followers, err := ap.getFollowers(ctx, username)
		if err != nil {
			ap.logger.Error("failed to get followers", zap.Error(err))
			// Continue even if this fails
		} else {
			for _, follower := range followers {
				followerEntry := baseEntry
				followerEntry.TimelineType = "HOME"
				followerEntry.TimelineID = follower
				followerEntry.EntryID = ap.generateTimelineSK(now, note.ID)
				entries = append(entries, &followerEntry)
			}
		}
	}

	// Write all entries to timelines
	if len(entries) > 0 {
		if err := ap.timelineRepo.CreateTimelineEntries(ctx, entries); err != nil {
			return fmt.Errorf("failed to write timeline entries: %w", err)
		}
	}

	// Record metrics for monitoring
	fanoutDuration := time.Since(now)
	ap.recordTimelineFanoutMetrics(ctx, "Create", len(entries), fanoutDuration)
	ap.recordObjectProcessingMetrics(ctx, contentType, isRemoteObject, fanoutDuration)

	ap.logger.Info("successfully fanned out Create activity",
		zap.String("post_id", objectID),
		zap.String("content_type", contentType),
		zap.String("visibility", visibility),
		zap.Int("timeline_count", len(entries)),
		zap.Bool("is_remote_object", isRemoteObject),
		zap.Duration("fanout_duration", fanoutDuration),
	)

	return nil
}

// fanOutAnnounceToTimelines handles timeline fanout for Announce (boost) activities
func (ap *ActivityProcessor) fanOutAnnounceToTimelines(ctx context.Context, activity *activitypub.Activity, username string) error {
	// Extract the announced object ID
	var announcedID string
	switch obj := activity.Object.(type) {
	case string:
		announcedID = obj
	case map[string]interface{}:
		if id, ok := obj["id"].(string); ok {
			announcedID = id
		}
	default:
		ap.logger.Warn("unsupported object type in Announce activity", zap.Any("object", activity.Object))
		return nil
	}

	if announcedID == "" {
		return fmt.Errorf("no object ID in Announce activity")
	}

	// Fetch the announced object
	var announcedContent string
	var originalAuthor string
	
	// First check if object exists locally
	existingObj, err := ap.objectRepo.GetObject(ctx, announcedID)
	if err == nil && existingObj != nil {
		// Use existing object - need to type assert
		switch obj := existingObj.(type) {
		case *models.Object:
			announcedContent = obj.Content
			originalAuthor = obj.AttributedTo
			ap.logger.Debug("found announced object locally", 
				zap.String("object_id", announcedID))
		default:
			// Try to extract from generic map
			if objMap, ok := existingObj.(map[string]interface{}); ok {
				if content, ok := objMap["content"].(string); ok {
					announcedContent = content
				}
				if author, ok := objMap["attributedTo"].(string); ok {
					originalAuthor = author
				}
			}
		}
	} else {
		// Object not found locally, fetch from remote server
		ap.logger.Info("fetching remote object for announce",
			zap.String("object_id", announcedID))
		
		// Get the announcing actor for signing requests
		signingActor, err := ap.actorRepo.GetActor(ctx, activity.Actor)
		if err != nil {
			ap.logger.Error("failed to get signing actor", zap.Error(err))
			// Fall back to minimal content for local actor lookup failure
			announcedContent = fmt.Sprintf("Boosted: %s", announcedID)
		} else {
			// Fetch the remote object with robust error handling and retry logic
			remoteObj, err := ap.fetchRemoteObjectWithRetry(ctx, announcedID, signingActor)
			if err != nil {
				ap.logger.Warn("failed to fetch remote object after retries",
					zap.String("object_id", announcedID),
					zap.Error(err))
				// Use fallback content
				announcedContent = fmt.Sprintf("Boosted: %s", announcedID)
			} else {
				// Successfully fetched remote object
				if note, ok := remoteObj.(*activitypub.Note); ok {
					announcedContent = note.Content
					originalAuthor = note.AttributedTo
					
					// Store the remote object for future reference
					ap.storeRemoteObject(ctx, note)
					
					ap.logger.Info("successfully fetched and stored remote object",
						zap.String("object_id", announcedID),
						zap.String("author", originalAuthor))
				} else {
					// Handle other object types
					if objMap, ok := remoteObj.(map[string]interface{}); ok {
						if content, ok := objMap["content"].(string); ok {
							announcedContent = content
						}
						if author, ok := objMap["attributedTo"].(string); ok {
							originalAuthor = author
						}
						
						// Store generic object
						ap.storeGenericRemoteObject(ctx, objMap)
					} else {
						announcedContent = fmt.Sprintf("Boosted: %s", announcedID)
					}
				}
			}
		}
	}

	var entries []*models.Timeline
	now := time.Now()

	baseEntry := models.Timeline{
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

	// Add to public timelines
	// Federated timeline
	publicEntry := baseEntry
	publicEntry.TimelineType = "PUBLIC"
	publicEntry.TimelineID = "FEDERATED"
	publicEntry.EntryID = ap.generateTimelineSK(now, activity.ID)
	entries = append(entries, &publicEntry)

	// Local timeline if it's a local user
	if strings.HasPrefix(activity.Actor, ap.baseURL) {
		localEntry := baseEntry
		localEntry.TimelineType = "PUBLIC"
		localEntry.TimelineID = "LOCAL"
		localEntry.EntryID = ap.generateTimelineSK(now, activity.ID)
		entries = append(entries, &localEntry)
	}

	// Add to author's home timeline
	homeEntry := baseEntry
	homeEntry.TimelineType = "HOME"
	homeEntry.TimelineID = username
	homeEntry.EntryID = ap.generateTimelineSK(now, activity.ID)
	entries = append(entries, &homeEntry)

	// Fan out to followers
	followers, err := ap.getFollowers(ctx, username)
	if err != nil {
		ap.logger.Error("failed to get followers", zap.Error(err))
	} else {
		for _, follower := range followers {
			followerEntry := baseEntry
			followerEntry.TimelineType = "HOME"
			followerEntry.TimelineID = follower
			followerEntry.EntryID = ap.generateTimelineSK(now, activity.ID)
			entries = append(entries, &followerEntry)
		}
	}

	// Write all entries
	if len(entries) > 0 {
		if err := ap.timelineRepo.CreateTimelineEntries(ctx, entries); err != nil {
			return fmt.Errorf("failed to write timeline entries: %w", err)
		}
	}

	// Record metrics for monitoring
	fanoutDuration := time.Since(now)
	ap.recordTimelineFanoutMetrics(ctx, "Announce", len(entries), fanoutDuration)
	
	// Determine if the announced object was remote
	isRemoteAnnounced := !strings.HasPrefix(announcedID, ap.baseURL)
	ap.recordObjectProcessingMetrics(ctx, "Announce", isRemoteAnnounced, fanoutDuration)

	ap.logger.Info("successfully fanned out Announce activity",
		zap.String("activity_id", activity.ID),
		zap.String("announced_id", announcedID),
		zap.Int("timeline_count", len(entries)),
		zap.Bool("is_remote_announced", isRemoteAnnounced),
		zap.Duration("fanout_duration", fanoutDuration),
	)

	return nil
}

// Helper functions

func (ap *ActivityProcessor) determineVisibility(to, cc []string) string {
	publicAddress := "https://www.w3.org/ns/activitystreams#Public"
	
	// Direct message - no public addressing
	if !contains(to, publicAddress) && !contains(cc, publicAddress) {
		return "direct"
	}
	
	// Public - addressed to public in 'to'
	if contains(to, publicAddress) {
		return "public"
	}
	
	// Unlisted - public in 'cc'
	if contains(cc, publicAddress) {
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

// recordFederationMetrics records metrics for federation operations
func (ap *ActivityProcessor) recordFederationMetrics(ctx context.Context, operation string, success bool, duration time.Duration, remoteHost string) {
	// Create metrics record for monitoring
	now := time.Now()
	
	metric := struct {
		PK        string `dynamorm:"pk"`
		SK        string `dynamorm:"sk"`
		Type      string `json:"type"`
		Operation string `json:"operation"`
		Success   bool   `json:"success"`
		Duration  int64  `json:"duration_ms"`
		Host      string `json:"remote_host"`
		Timestamp string `json:"timestamp"`
		TTL       int64  `dynamorm:"ttl"`
	}{
		PK:        "FEDERATION#METRICS",
		SK:        fmt.Sprintf("METRIC#%d#%s", now.Unix(), operation),
		Type:      "FederationMetric",
		Operation: operation,
		Success:   success,
		Duration:  duration.Milliseconds(),
		Host:      remoteHost,
		Timestamp: now.Format(time.RFC3339),
		TTL:       now.Add(7 * 24 * time.Hour).Unix(), // 7 days retention
	}
	
	// Log the metric (don't fail the main operation if this fails)
	if err := ap.db.WithContext(ctx).Model(&metric).Create(); err != nil {
		ap.logger.Debug("failed to record federation metric",
			zap.String("operation", operation),
			zap.Error(err))
	}
}

// recordObjectProcessingMetrics records metrics about object processing
func (ap *ActivityProcessor) recordObjectProcessingMetrics(ctx context.Context, objectType string, isRemote bool, processingTime time.Duration) {
	now := time.Now()
	
	metric := struct {
		PK             string `dynamorm:"pk"`
		SK             string `dynamorm:"sk"`
		Type           string `json:"type"`
		ObjectType     string `json:"object_type"`
		IsRemote       bool   `json:"is_remote"`
		ProcessingTime int64  `json:"processing_time_ms"`
		Timestamp      string `json:"timestamp"`
		TTL            int64  `dynamorm:"ttl"`
	}{
		PK:             "PROCESSING#METRICS",
		SK:             fmt.Sprintf("METRIC#%d#%s", now.Unix(), objectType),
		Type:           "ProcessingMetric",
		ObjectType:     objectType,
		IsRemote:       isRemote,
		ProcessingTime: processingTime.Milliseconds(),
		Timestamp:      now.Format(time.RFC3339),
		TTL:            now.Add(24 * time.Hour).Unix(), // 24 hours retention
	}
	
	if err := ap.db.WithContext(ctx).Model(&metric).Create(); err != nil {
		ap.logger.Debug("failed to record processing metric",
			zap.String("object_type", objectType),
			zap.Error(err))
	}
}

// recordTimelineFanoutMetrics records metrics about timeline fanout operations
func (ap *ActivityProcessor) recordTimelineFanoutMetrics(ctx context.Context, activityType string, entryCount int, fanoutTime time.Duration) {
	now := time.Now()
	
	metric := struct {
		PK           string `dynamorm:"pk"`
		SK           string `dynamorm:"sk"`
		Type         string `json:"type"`
		ActivityType string `json:"activity_type"`
		EntryCount   int    `json:"entry_count"`
		FanoutTime   int64  `json:"fanout_time_ms"`
		Timestamp    string `json:"timestamp"`
		TTL          int64  `dynamorm:"ttl"`
	}{
		PK:           "FANOUT#METRICS",
		SK:           fmt.Sprintf("METRIC#%d#%s", now.Unix(), activityType),
		Type:         "FanoutMetric",
		ActivityType: activityType,
		EntryCount:   entryCount,
		FanoutTime:   fanoutTime.Milliseconds(),
		Timestamp:    now.Format(time.RFC3339),
		TTL:          now.Add(24 * time.Hour).Unix(), // 24 hours retention
	}
	
	if err := ap.db.WithContext(ctx).Model(&metric).Create(); err != nil {
		ap.logger.Debug("failed to record fanout metric",
			zap.String("activity_type", activityType),
			zap.Error(err))
	}
}

// extractRemoteHost extracts the hostname from a URL for metrics
func (ap *ActivityProcessor) extractRemoteHost(url string) string {
	if url == "" {
		return "unknown"
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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
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
					requestID = "unknown"
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
			requestID = "unknown"
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
			if validatedObj, valErr := ap.validateAndProcessRemoteObject(obj, objectURL); valErr == nil {
				ap.logger.Info("successfully fetched remote object",
					zap.String("object_url", objectURL),
					zap.Int("attempt", attempt))
				return validatedObj, nil
			} else {
				ap.logger.Warn("fetched object failed validation",
					zap.String("object_url", objectURL),
					zap.Error(valErr))
				lastErr = fmt.Errorf("validation failed: %w", valErr)
				break // Don't retry validation failures
			}
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
		ap.logger.Info("processing unknown object type",
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
	
	// Default to not retrying unknown errors
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
	jitter := delay / 4                    // 25% of delay
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
	id, ok := objMap["id"].(string)
	if !ok {
		ap.logger.Error("cannot store object without ID")
		return
	}
	
	objectType, ok := objMap["type"].(string)
	if !ok {
		objectType = "Object" // fallback
	}
	
	// Extract common fields
	now := time.Now()
	
	// Handle published time
	publishedTime := now
	if pubStr, ok := objMap["published"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, pubStr); err == nil {
			publishedTime = parsed
		}
	}
	
	// Extract content (if any)
	content := ""
	if c, ok := objMap["content"].(string); ok {
		content = c
	} else if name, ok := objMap["name"].(string); ok {
		content = name // Use name as content for objects without content
	} else if summary, ok := objMap["summary"].(string); ok {
		content = summary // Use summary as fallback
	}
	
	// Extract author
	attributedTo := ""
	if attr, ok := objMap["attributedTo"].(string); ok {
		attributedTo = attr
	}
	
	// Extract addressing
	var to, cc []string
	if toField, ok := objMap["to"]; ok {
		if toSlice, ok := toField.([]interface{}); ok {
			for _, item := range toSlice {
				if str, ok := item.(string); ok {
					to = append(to, str)
				}
			}
		}
	}
	
	if ccField, ok := objMap["cc"]; ok {
		if ccSlice, ok := ccField.([]interface{}); ok {
			for _, item := range ccSlice {
				if str, ok := item.(string); ok {
					cc = append(cc, str)
				}
			}
		}
	}
	
	// Create storage object
	storageObj := &models.Object{
		ID:           id,
		Type:         objectType,
		Content:      content,
		AttributedTo: attributedTo,
		Published:    publishedTime,
		Updated:      now,
		To:           to,
		CC:           cc,
		IsRemote:     true,
		CreatedAt:    now,
	}
	
	// Store object
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
	accountRepo         *repositories.AccountRepository
	actorRepo           *repositories.ActorRepository
	objectRepo          *repositories.ObjectRepository
	activityRepo        *repositories.ActivityRepository
	timelineRepo        *repositories.TimelineRepository
	notificationRepo    *repositories.NotificationRepository
	likeRepo            *repositories.LikeRepository
	moderationRepo      *repositories.ModerationRepository
	listRepo            *repositories.ListRepository
	mediaRepo           *repositories.MediaRepository
	pollRepo            *repositories.PollRepository
	pushSubscriptionRepo *repositories.PushSubscriptionRepository
	hashtagRepo         *repositories.HashtagRepository
	scheduledStatusRepo *repositories.ScheduledStatusRepository
	announcementRepo    *repositories.AnnouncementRepository
	domainBlockRepo     *repositories.DomainBlockRepository
	relationshipRepo    *repositories.RelationshipRepository
	instanceRepo        *repositories.InstanceRepository
	federationRepo      *repositories.FederationRepository
	recoveryRepo        *repositories.RecoveryRepository
	analyticsRepo       *repositories.TrendingRepository
	socialRepo          *repositories.SocialRepository
	userRepo            *repositories.UserRepository
	statusRepo          *repositories.StatusRepository
	costRepo            *repositories.CostTrackingRepository
	trustRepo           *repositories.TrustRepository
	searchRepo          *repositories.SearchRepository
	relayRepo           *repositories.RelayRepository
	communityNoteRepo   *repositories.CommunityNoteRepository
	emojiRepo           *repositories.EmojiRepository
	rateLimitRepo       *repositories.RateLimitRepository
	conversationRepo    *repositories.ConversationRepository
	markerRepo          *repositories.MarkerRepository
	featuredTagRepo     *repositories.FeaturedTagRepository
	aiRepo              *repositories.AIRepository
	exportRepo          *repositories.ExportRepository
	importRepo          *repositories.ImportRepository
	dlqRepo             *repositories.DLQRepository
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
	
	return &StorageAdapter{
		db:        db,
		tableName: tableName,
		logger:    logger,
		
		// Initialize all repositories
		accountRepo:         repositories.NewAccountRepository(db, tableName, domain, logger),
		actorRepo:           repositories.NewActorRepository(db, tableName, logger),
		objectRepo:          repositories.NewObjectRepository(db, tableName, domain, logger),
		activityRepo:        repositories.NewActivityRepository(db, tableName, logger),
		timelineRepo:        repositories.NewTimelineRepository(db, tableName, logger),
		notificationRepo:    repositories.NewNotificationRepository(db, tableName, logger),
		likeRepo:            repositories.NewLikeRepository(db, tableName, logger),
		moderationRepo:      repositories.NewModerationRepository(db, tableName, logger),
		listRepo:            repositories.NewListRepository(db, tableName, logger),
		mediaRepo:           repositories.NewMediaRepository(db, tableName, logger),
		pollRepo:            repositories.NewPollRepository(db, tableName, logger),
		pushSubscriptionRepo: repositories.NewPushSubscriptionRepository(db, tableName, logger),
		hashtagRepo:         repositories.NewHashtagRepository(db, tableName, logger, domain),
		scheduledStatusRepo: repositories.NewScheduledStatusRepository(db, tableName, logger),
		announcementRepo:    repositories.NewAnnouncementRepository(db, tableName, logger),
		domainBlockRepo:     repositories.NewDomainBlockRepository(db, tableName, logger),
		relationshipRepo:    repositories.NewRelationshipRepository(db, tableName, logger),
		instanceRepo:        repositories.NewInstanceRepository(db, tableName, logger),
		federationRepo:      repositories.NewFederationRepository(db, logger),
		recoveryRepo:        repositories.NewRecoveryRepository(db, tableName, logger),
		analyticsRepo:       repositories.NewTrendingRepository(db, logger),
		socialRepo:          repositories.NewSocialRepository(db, logger),
		userRepo:            repositories.NewUserRepository(db, tableName, logger),
		statusRepo:          repositories.NewStatusRepository(db, tableName, logger),
		costRepo:            repositories.NewCostTrackingRepository(db, tableName, logger),
		trustRepo:           repositories.NewTrustRepository(db, logger),
		searchRepo:          repositories.NewSearchRepository(db, logger),
		relayRepo:           repositories.NewRelayRepository(db, tableName, logger),
		communityNoteRepo:   repositories.NewCommunityNoteRepository(db, tableName, logger),
		emojiRepo:           repositories.NewEmojiRepository(db, logger),
		rateLimitRepo:       repositories.NewRateLimitRepository(db, tableName, logger),
		conversationRepo:    repositories.NewConversationRepository(db, logger),
		markerRepo:          repositories.NewMarkerRepository(db, tableName, logger),
		featuredTagRepo:     repositories.NewFeaturedTagRepository(db, tableName, logger),
		aiRepo:              repositories.NewAIRepository(db, tableName, logger),
		exportRepo:          repositories.NewExportRepository(db, tableName, logger),
		importRepo:          repositories.NewImportRepository(db, tableName, logger),
		dlqRepo:             repositories.NewDLQRepository(db, tableName, logger),
	}
}

// Implement all core.RepositoryStorage interface methods

func (s *StorageAdapter) Account() *repositories.AccountRepository         { return s.accountRepo }
func (s *StorageAdapter) Actor() *repositories.ActorRepository             { return s.actorRepo }
func (s *StorageAdapter) Object() *repositories.ObjectRepository           { return s.objectRepo }
func (s *StorageAdapter) Activity() *repositories.ActivityRepository       { return s.activityRepo }
func (s *StorageAdapter) Timeline() *repositories.TimelineRepository       { return s.timelineRepo }
func (s *StorageAdapter) Notification() *repositories.NotificationRepository { return s.notificationRepo }
func (s *StorageAdapter) Like() *repositories.LikeRepository               { return s.likeRepo }
func (s *StorageAdapter) Moderation() *repositories.ModerationRepository   { return s.moderationRepo }
func (s *StorageAdapter) List() *repositories.ListRepository               { return s.listRepo }
func (s *StorageAdapter) Media() *repositories.MediaRepository             { return s.mediaRepo }
func (s *StorageAdapter) Poll() *repositories.PollRepository               { return s.pollRepo }
func (s *StorageAdapter) PushSubscription() *repositories.PushSubscriptionRepository { return s.pushSubscriptionRepo }
func (s *StorageAdapter) Hashtag() *repositories.HashtagRepository         { return s.hashtagRepo }
func (s *StorageAdapter) ScheduledStatus() *repositories.ScheduledStatusRepository { return s.scheduledStatusRepo }
func (s *StorageAdapter) Announcement() *repositories.AnnouncementRepository { return s.announcementRepo }
func (s *StorageAdapter) DomainBlock() *repositories.DomainBlockRepository { return s.domainBlockRepo }
func (s *StorageAdapter) Relationship() *repositories.RelationshipRepository { return s.relationshipRepo }
func (s *StorageAdapter) Instance() *repositories.InstanceRepository       { return s.instanceRepo }
func (s *StorageAdapter) Federation() *repositories.FederationRepository   { return s.federationRepo }
func (s *StorageAdapter) Recovery() *repositories.RecoveryRepository       { return s.recoveryRepo }
func (s *StorageAdapter) Analytics() *repositories.TrendingRepository      { return s.analyticsRepo }
func (s *StorageAdapter) Social() *repositories.SocialRepository           { return s.socialRepo }
func (s *StorageAdapter) User() *repositories.UserRepository               { return s.userRepo }
func (s *StorageAdapter) Status() *repositories.StatusRepository           { return s.statusRepo }
func (s *StorageAdapter) Cost() *repositories.CostTrackingRepository       { return s.costRepo }
func (s *StorageAdapter) Trust() *repositories.TrustRepository             { return s.trustRepo }
func (s *StorageAdapter) Search() *repositories.SearchRepository           { return s.searchRepo }
func (s *StorageAdapter) Relay() *repositories.RelayRepository             { return s.relayRepo }
func (s *StorageAdapter) CommunityNote() *repositories.CommunityNoteRepository { return s.communityNoteRepo }
func (s *StorageAdapter) Emoji() *repositories.EmojiRepository             { return s.emojiRepo }
func (s *StorageAdapter) RateLimit() *repositories.RateLimitRepository     { return s.rateLimitRepo }
func (s *StorageAdapter) Conversation() *repositories.ConversationRepository { return s.conversationRepo }
func (s *StorageAdapter) Marker() *repositories.MarkerRepository           { return s.markerRepo }
func (s *StorageAdapter) FeaturedTag() *repositories.FeaturedTagRepository { return s.featuredTagRepo }
func (s *StorageAdapter) AI() *repositories.AIRepository                   { return s.aiRepo }
func (s *StorageAdapter) Export() *repositories.ExportRepository           { return s.exportRepo }
func (s *StorageAdapter) Import() *repositories.ImportRepository           { return s.importRepo }
func (s *StorageAdapter) DLQ() *repositories.DLQRepository                 { return s.dlqRepo }

// Utility methods
func (s *StorageAdapter) GetDB() core.DB {
	return s.db
}

func (s *StorageAdapter) GetTableName() string {
	return s.tableName
}

func (s *StorageAdapter) GetLogger() *zap.Logger {
	return s.logger
}
