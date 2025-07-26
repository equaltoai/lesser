package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

type ActivityProcessor struct {
	db               core.DB
	tableName        string
	logger           *zap.Logger
	timelineRepo     *repositories.TimelineRepository
	actorRepo        *repositories.ActorRepository
	userRepo         *repositories.UserRepository
	baseURL          string
}

func NewActivityProcessor(db core.DB, tableName string, baseURL string) *ActivityProcessor {
	// Initialize repositories
	timelineRepo := repositories.NewTimelineRepository(db, tableName)
	actorRepo := repositories.NewActorRepository(db)
	userRepo := repositories.NewUserRepository(db)

	return &ActivityProcessor{
		db:           db,
		tableName:    tableName,
		logger:       common.Logger(),
		timelineRepo: timelineRepo,
		actorRepo:    actorRepo,
		userRepo:     userRepo,
		baseURL:      baseURL,
	}
}

func (ap *ActivityProcessor) HandleStream(ctx context.Context, event events.DynamoDBEvent) error {
	// Add request tracking
	requestID := uuid.New().String()

	ap.logger.Info("processing activity stream batch",
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	// Process records in parallel with error collection
	var errors []error
	var errorMutex sync.Mutex

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
				errors = append(errors, err)
				errorMutex.Unlock()

				ap.logger.Error("failed to process record",
					zap.String("event_id", record.EventID),
					zap.Error(err),
				)
			}
		}(record)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("partial batch failure: %d of %d records failed", len(errors), len(event.Records))
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
	_ = ctx // unused parameter
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

	return ap.db.Model(&inboxRecord).Create()
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

	return ap.db.Model(&outboxRecord).Create()
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
	_ = ctx // unused parameter
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

	return ap.db.Model(&metricsRecord).Create()
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
	_ = ctx // unused parameter
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

	return ap.db.Model(&cleanupRecord).Create()
}

// fanOutToTimelines handles timeline fanout for Create activities
func (ap *ActivityProcessor) fanOutToTimelines(ctx context.Context, activity *activitypub.Activity, username string) error {
	// Extract the note/object from the activity
	var note *activitypub.Note
	switch obj := activity.Object.(type) {
	case map[string]interface{}:
		// Convert map to Note
		noteData, err := json.Marshal(obj)
		if err != nil {
			return fmt.Errorf("failed to marshal note: %w", err)
		}
		note = &activitypub.Note{}
		if err := json.Unmarshal(noteData, note); err != nil {
			return fmt.Errorf("failed to unmarshal note: %w", err)
		}
	case *activitypub.Note:
		note = obj
	default:
		ap.logger.Warn("unsupported object type in Create activity", zap.Any("object", activity.Object))
		return nil
	}

	// Determine visibility from addressing
	visibility := ap.determineVisibility(note.To, note.CC)
	
	// Create timeline entries
	var entries []*models.Timeline
	now := time.Now()

	// Extract content and metadata
	content := note.Content
	if len(content) > 500 {
		content = content[:500]
	}

	// Base entry for all timelines
	baseEntry := models.Timeline{
		PostID:      note.ID,
		ActorID:     activity.Actor,
		ActorHandle: username,
		Content:     content,
		ContentType: "Note",
		HasMedia:    len(note.Attachment) > 0,
		IsReply:     note.InReplyTo != "",
		InReplyTo:   note.InReplyTo,
		IsBoost:     false,
		Visibility:  visibility,
		Language:    ap.extractLanguage(note),
		Sensitive:   note.Sensitive,
		SpoilerText: note.Summary,
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

	ap.logger.Info("successfully fanned out Create activity",
		zap.String("post_id", note.ID),
		zap.String("visibility", visibility),
		zap.Int("timeline_count", len(entries)),
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

	// For now, we'll create minimal timeline entries for announces
	// In a full implementation, you'd fetch the original object
	var entries []*models.Timeline
	now := time.Now()

	baseEntry := models.Timeline{
		PostID:      activity.ID,
		ActorID:     activity.Actor,
		ActorHandle: username,
		Content:     fmt.Sprintf("Boosted: %s", announcedID),
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

	ap.logger.Info("successfully fanned out Announce activity",
		zap.String("activity_id", activity.ID),
		zap.String("announced_id", announcedID),
		zap.Int("timeline_count", len(entries)),
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
	// For now, default to English
	// In a full implementation, detect from content or use note.Language
	return "en"
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

	// For now, return empty list as we'd need a followers repository
	// In a full implementation, query the followers list
	_ = actor
	return []string{}, nil
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
	// Handle DynamoDB stream events with logging middleware
	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		start := time.Now()
		defer func() {
			duration := time.Since(start)
			processor.logger.Info("request completed",
				zap.Duration("duration", duration),
			)
		}()
		return processor.HandleStream(ctx, event)
	})
}
