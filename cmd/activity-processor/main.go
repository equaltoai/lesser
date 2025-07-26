package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/common"
)

type ActivityProcessor struct {
	db        *dynamorm.LambdaDB
	tableName string
	logger    *zap.Logger
}

func NewActivityProcessor() (*ActivityProcessor, error) {
	// Get table name from environment
	tableName := os.Getenv("DYNAMO_TABLE_NAME")
	if tableName == "" {
		tableName = "lesser-main"
	}

	// Initialize DynamORM with Lambda optimization
	db, err := dynamorm.NewLambdaOptimized()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Set timeout buffer to prevent Lambda timeouts
	if lambdaDB, ok := db.WithLambdaTimeoutBuffer(500 * time.Millisecond).(*dynamorm.LambdaDB); ok {
		db = lambdaDB
	}

	return &ActivityProcessor{
		db:        db,
		tableName: tableName,
		logger:    common.Logger(),
	}, nil
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
		if err := dynamorm.UnmarshalStreamImage(record.Change.NewImage, &activity); err != nil {
			return fmt.Errorf("failed to unmarshal new image: %w", err)
		}

	case "REMOVE":
		if record.Change.OldImage == nil {
			return fmt.Errorf("no old image in remove record %s", record.EventID)
		}

		// Convert DynamoDB attribute values using DynamORM
		if err := dynamorm.UnmarshalStreamImage(record.Change.OldImage, &activity); err != nil {
			return fmt.Errorf("failed to unmarshal old image: %w", err)
		}

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
}) error {
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
}) error {
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
}) error {
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
}) error {
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
}) error {
	_ = ctx // unused parameter
	// Process activities sent by local users
	ap.logger.Debug("processing outbox activity",
		zap.String("pk", activity.PK),
		zap.String("username", activity.Username),
	)

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
}) error {
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
}) error {
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

func main() {
	processor, err := NewActivityProcessor()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize activity processor: %v", err))
	}

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