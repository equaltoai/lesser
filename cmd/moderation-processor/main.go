package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/trust"
)

var (
	db               core.DB
	consensusEngine  *moderation.ConsensusEngine
	logger           *zap.Logger
	cfg              *config.Config
	moderationRepo   *repositories.ModerationRepository
	userRepo         *repositories.UserRepository
	notificationRepo *repositories.NotificationRepository
	objectRepo       *repositories.ObjectRepository
)

// ModerationProcessor handles DynamoDB stream events for moderation
type ModerationProcessor struct {
	db               core.DB
	moderationRepo   *repositories.ModerationRepository
	userRepo         *repositories.UserRepository
	notificationRepo *repositories.NotificationRepository
	objectRepo       *repositories.ObjectRepository
	logger           *zap.Logger
	consensusEngine  *moderation.ConsensusEngine
}

// NewModerationProcessor creates a new moderation processor
func NewModerationProcessor() *ModerationProcessor {
	return &ModerationProcessor{
		db:               db,
		moderationRepo:   moderationRepo,
		userRepo:         userRepo,
		notificationRepo: notificationRepo,
		objectRepo:       objectRepo,
		logger:           logger,
		consensusEngine:  consensusEngine,
	}
}

// storageAdapter adapts repositories to moderation.StorageInterface
type repositoryStorageAdapter struct {
	moderationRepo *repositories.ModerationRepository
	userRepo       *repositories.UserRepository
}

func (s *repositoryStorageAdapter) GetModerationEvent(ctx context.Context, eventID string) (*moderation.ModerationEvent, error) {
	// Convert storage.ModerationEvent to moderation.ModerationEvent
	storageEvent, err := s.moderationRepo.GetModerationEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	return (*moderation.ModerationEvent)(storageEvent), nil
}

func (s *repositoryStorageAdapter) AddModerationReview(ctx context.Context, review *moderation.Review) error {
	// Convert moderation.Review to storage.ModerationReview
	storageReview := (*storage.ModerationReview)(review)
	return s.moderationRepo.AddModerationReview(ctx, storageReview)
}

func (s *repositoryStorageAdapter) GetModerationReviews(ctx context.Context, eventID string) ([]*moderation.Review, error) {
	storageReviews, err := s.moderationRepo.GetModerationReviews(ctx, eventID)
	if err != nil {
		return nil, err
	}
	// Convert []*storage.ModerationReview to []*moderation.Review
	reviews := make([]*moderation.Review, len(storageReviews))
	for i, sr := range storageReviews {
		reviews[i] = (*moderation.Review)(sr)
	}
	return reviews, nil
}

func (s *repositoryStorageAdapter) CreateModerationDecision(ctx context.Context, decision *moderation.ModerationDecision) error {
	// Convert moderation.ModerationDecision to storage.ModerationDecision
	storageDecision := (*storage.ModerationDecision)(decision)
	return s.moderationRepo.CreateModerationDecision(ctx, storageDecision)
}

func (s *repositoryStorageAdapter) GetModerationQueue(ctx context.Context, limit int, cursor string) ([]*moderation.QueueItem, string, error) {
	storageItems, nextCursor, err := s.moderationRepo.GetModerationQueuePaginated(ctx, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	// Convert []*storage.ModerationQueueItem to []*moderation.QueueItem
	items := make([]*moderation.QueueItem, len(storageItems))
	for i, si := range storageItems {
		items[i] = (*moderation.QueueItem)(si)
	}
	return items, nextCursor, nil
}

func (s *repositoryStorageAdapter) GetTrustScore(ctx context.Context, actorID, category string) (*trust.TrustScore, error) {
	// This would need a trust repository - for now return nil
	return nil, fmt.Errorf("trust score retrieval not implemented in repository adapter")
}

func (s *repositoryStorageAdapter) RecordTrustUpdate(ctx context.Context, update *trust.TrustUpdate) error {
	// This would need a trust repository - for now return nil
	return fmt.Errorf("trust update recording not implemented in repository adapter")
}

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

	// Initialize repositories
	moderationRepo = repositories.NewModerationRepository(db, cfg.DynamoTableName, logger)
	userRepo = repositories.NewUserRepository(db, cfg.DynamoTableName, logger)
	notificationRepo = repositories.NewNotificationRepository(db, cfg.DynamoTableName, logger)
	objectRepo = repositories.NewObjectRepository(db, cfg.DynamoTableName, cfg.Domain, logger)

	// Initialize consensus engine with repository adapter
	adapter := &repositoryStorageAdapter{
		moderationRepo: moderationRepo,
		userRepo:       userRepo,
	}
	consensusEngine = moderation.NewConsensusEngine(adapter, nil)
}


// processRecord processes a single DynamoDB stream record
func (mp *ModerationProcessor) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Only process INSERT and MODIFY events
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Check if this is a moderation-related record
	pk := getStringAttribute(record.Change.Keys["PK"])
	sk := getStringAttribute(record.Change.Keys["SK"])

	if pk == "" || sk == "" {
		return nil
	}

	logger.Debug("Processing record",
		zap.String("pk", pk),
		zap.String("sk", sk),
		zap.String("event_name", record.EventName),
	)

	// Handle different types of moderation records
	switch {
	case strings.HasPrefix(pk, "REVIEW#"):
		// New review added
		return mp.handleNewReview(ctx, record)

	case strings.HasPrefix(pk, "EVENT#") && record.EventName == "INSERT":
		// New moderation event created
		return mp.handleNewEvent(ctx, record)

	case strings.HasPrefix(pk, "DECISION#"):
		// Decision made - trigger actions
		return mp.handleDecision(ctx, record)
	}

	return nil
}

// handleNewReview processes a new review and checks for consensus
func (mp *ModerationProcessor) handleNewReview(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Extract event ID from PK (REVIEW#eventID)
	pk := getStringAttribute(record.Change.Keys["PK"])
	parts := strings.Split(pk, "#")
	if len(parts) < 2 {
		return fmt.Errorf("invalid review PK format: %s", pk)
	}
	eventID := parts[1]

	// Extract reviewer ID from SK (REVIEWER#reviewerID)
	sk := getStringAttribute(record.Change.Keys["SK"])
	reviewerParts := strings.Split(sk, "#")
	if len(reviewerParts) < 2 {
		return fmt.Errorf("invalid review SK format: %s", sk)
	}
	reviewerID := reviewerParts[1]

	logger.Info("Processing new review",
		zap.String("event_id", eventID),
		zap.String("reviewer_id", reviewerID),
	)

	// Get the review details
	review, err := getReviewFromRecord(record)
	if err != nil {
		return fmt.Errorf("failed to extract review: %w", err)
	}

	// Process the review and check for consensus
	decision, err := mp.consensusEngine.ProcessReview(ctx, eventID, review)
	if err != nil {
		mp.logger.Warn("Failed to process review",
			zap.String("event_id", eventID),
			zap.Error(err),
		)
		return nil // Don't fail the whole batch
	}

	if decision != nil {
		mp.logger.Info("Consensus reached",
			zap.String("event_id", eventID),
			zap.String("decision_id", decision.ID),
			zap.String("action", string(decision.Action)),
			zap.Float64("consensus_score", decision.ConsensusScore),
		)
	}

	return nil
}

// handleNewEvent processes a new moderation event
func (mp *ModerationProcessor) handleNewEvent(ctx context.Context, record events.DynamoDBEventRecord) error {
	event, err := getEventFromRecord(record)
	if err != nil {
		return fmt.Errorf("failed to extract event: %w", err)
	}

	mp.logger.Info("New moderation event created",
		zap.String("event_id", event.ID),
		zap.String("object_id", event.ObjectID),
		zap.String("type", string(event.EventType)),
		zap.String("category", string(event.Category)),
		zap.Int("severity", int(event.Severity)),
	)

	// Send notifications to moderators
	if err := mp.sendModeratorNotification(ctx, event); err != nil {
		mp.logger.Error("failed to send moderator notification", zap.Error(err))
	}

	// Trigger automatic actions based on severity
	if err := mp.triggerAutomaticActions(ctx, event); err != nil {
		mp.logger.Error("failed to trigger automatic actions", zap.Error(err))
	}

	return nil
}

// handleDecision processes a moderation decision
func (mp *ModerationProcessor) handleDecision(ctx context.Context, record events.DynamoDBEventRecord) error {
	decision, err := getDecisionFromRecord(record)
	if err != nil {
		return fmt.Errorf("failed to extract decision: %w", err)
	}

	mp.logger.Info("Processing moderation decision",
		zap.String("decision_id", decision.ID),
		zap.String("object_id", decision.ObjectID),
		zap.String("action", string(decision.Action)),
	)

	// Apply the decision based on action type
	switch decision.Action {
	case moderation.ActionTypeSilence:
		// Implement account silencing
		if err := mp.silenceAccount(ctx, decision.ObjectID, decision.Reason); err != nil {
			mp.logger.Error("failed to silence account", zap.Error(err))
			return err
		}
		mp.logger.Info("Account silenced", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeSuspend:
		// Implement account suspension
		if err := mp.suspendAccount(ctx, decision.ObjectID, decision.Reason); err != nil {
			mp.logger.Error("failed to suspend account", zap.Error(err))
			return err
		}
		mp.logger.Info("Account suspended", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeRemove:
		// Implement content removal
		if err := mp.removeContent(ctx, decision.ObjectID); err != nil {
			mp.logger.Error("failed to remove content", zap.Error(err))
			return err
		}
		mp.logger.Info("Content removed", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeNone:
		mp.logger.Info("No action taken", zap.String("object_id", decision.ObjectID))

	default:
		mp.logger.Warn("Unknown action type",
			zap.String("action", string(decision.Action)),
			zap.String("object_id", decision.ObjectID),
		)
	}

	return nil
}

// getReviewFromRecord extracts review from DynamoDB record
func getReviewFromRecord(record events.DynamoDBEventRecord) (*moderation.Review, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "REVIEW" {
		return nil, fmt.Errorf("not a review record")
	}

	// Extract event ID from PK
	pk := getStringAttribute(record.Change.Keys["PK"])
	eventID := strings.TrimPrefix(pk, "REVIEW#")

	// Extract reviewer ID from SK
	sk := getStringAttribute(record.Change.Keys["SK"])
	reviewerID := strings.TrimPrefix(sk, "REVIEWER#")

	// Extract review data from NewImage
	review := &moderation.Review{
		EventID:    eventID,
		ReviewerID: reviewerID,
		Action:     moderation.ActionTypeWarning, // Default
		Weight:     1.0,                          // Default
	}

	// Extract action if present
	if actionAttr, ok := record.Change.NewImage["Action"]; ok {
		if action := getStringAttribute(actionAttr); action != "" {
			review.Action = moderation.ActionType(action)
		}
	}

	// Extract weight if present
	if weightAttr, ok := record.Change.NewImage["Weight"]; ok {
		if weightAttr.DataType() == events.DataTypeNumber {
			if weight, err := weightAttr.Float(); err == nil {
				review.Weight = weight
			}
		}
	}

	// Extract created timestamp if present
	if createdAttr, ok := record.Change.NewImage["Created"]; ok {
		if timestamp := getStringAttribute(createdAttr); timestamp != "" {
			if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
				review.Created = t
			}
		}
	}

	return review, nil
}

// getEventFromRecord extracts moderation event from DynamoDB record
func getEventFromRecord(record events.DynamoDBEventRecord) (*moderation.ModerationEvent, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "EVENT" {
		return nil, fmt.Errorf("not an event record")
	}

	// Extract event ID from PK
	pk := getStringAttribute(record.Change.Keys["PK"])
	objectID := strings.TrimPrefix(pk, "EVENT#")

	// Create event with extracted data
	event := &moderation.ModerationEvent{
		ObjectID:  objectID,
		EventType: moderation.EventTypeFlagged,   // Default
		Category:  moderation.CategoryHateSpeech, // Default
		Severity:  moderation.SeverityMedium,     // Default
	}

	// Extract ID if present
	if idAttr, ok := record.Change.NewImage["ID"]; ok {
		event.ID = getStringAttribute(idAttr)
	}

	// Extract actor ID if present
	if actorAttr, ok := record.Change.NewImage["ActorID"]; ok {
		event.ActorID = getStringAttribute(actorAttr)
	}

	// Extract event type if present
	if typeAttr, ok := record.Change.NewImage["EventType"]; ok {
		if eventType := getStringAttribute(typeAttr); eventType != "" {
			event.EventType = moderation.EventType(eventType)
		}
	}

	// Extract category if present
	if catAttr, ok := record.Change.NewImage["Category"]; ok {
		if category := getStringAttribute(catAttr); category != "" {
			event.Category = moderation.Category(category)
		}
	}

	// Extract severity if present
	if sevAttr, ok := record.Change.NewImage["Severity"]; ok {
		if sevAttr.DataType() == events.DataTypeNumber {
			if sev, err := sevAttr.Float(); err == nil {
				event.Severity = moderation.Severity(sev)
			}
		}
	}

	return event, nil
}

// getDecisionFromRecord extracts moderation decision from DynamoDB record
func getDecisionFromRecord(record events.DynamoDBEventRecord) (*moderation.ModerationDecision, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "DECISION" {
		return nil, fmt.Errorf("not a decision record")
	}

	// Extract object ID from PK
	pk := getStringAttribute(record.Change.Keys["PK"])
	objectID := strings.TrimPrefix(pk, "DECISION#")

	// Create decision with extracted data
	decision := &moderation.ModerationDecision{
		ObjectID: objectID,
		Action:   moderation.ActionTypeWarning, // Default
	}

	// Extract ID if present
	if idAttr, ok := record.Change.NewImage["ID"]; ok {
		decision.ID = getStringAttribute(idAttr)
	}

	// Extract event ID if present
	if eventAttr, ok := record.Change.NewImage["EventID"]; ok {
		decision.EventID = getStringAttribute(eventAttr)
	}

	// Extract action if present
	if actionAttr, ok := record.Change.NewImage["Action"]; ok {
		if action := getStringAttribute(actionAttr); action != "" {
			decision.Action = moderation.ActionType(action)
		}
	}

	// Extract reason if present
	if reasonAttr, ok := record.Change.NewImage["Reason"]; ok {
		decision.Reason = getStringAttribute(reasonAttr)
	}

	// Extract consensus score if present
	if scoreAttr, ok := record.Change.NewImage["ConsensusScore"]; ok {
		if scoreAttr.DataType() == events.DataTypeNumber {
			if score, err := scoreAttr.Float(); err == nil {
				decision.ConsensusScore = score
			}
		}
	}

	return decision, nil
}

// sendModeratorNotification sends notifications to moderators about moderation events
func (mp *ModerationProcessor) sendModeratorNotification(ctx context.Context, event *moderation.ModerationEvent) error {
	// Get list of moderators - for now use a simple approach
	// In a full implementation, you'd have a proper role-based user query
	moderators := []string{"admin", "moderator1", "moderator2"}

	// Create notification for each moderator
	for _, moderatorID := range moderators {
		notification := &models.Notification{
			ID:         fmt.Sprintf("mod_%s_%d", event.ID, time.Now().UnixNano()),
			UserID:     moderatorID,
			Type:       "moderation",
			ActorID:    event.ActorID,
			TargetID:   event.ObjectID,
			TargetType: "status",
			Title:      "New Moderation Event",
			Body:       fmt.Sprintf("New %s event for review", event.Category),
			IsRead:     false,
			CreatedAt:  time.Now(),
		}

		if err := mp.notificationRepo.CreateNotification(ctx, notification); err != nil {
			mp.logger.Error("Failed to create notification",
				zap.String("moderator_id", moderatorID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// triggerAutomaticActions triggers automatic actions based on event severity
func (mp *ModerationProcessor) triggerAutomaticActions(ctx context.Context, event *moderation.ModerationEvent) error {
	// High severity events trigger automatic actions
	if event.Severity >= 8 {
		mp.logger.Info("High severity event - triggering automatic action",
			zap.String("event_id", event.ID),
			zap.Int("severity", int(event.Severity)),
		)

		// Create automatic review from system
		review := &moderation.Review{
			ID:         fmt.Sprintf("auto_%s_%d", event.ID, time.Now().UnixNano()),
			EventID:    event.ID,
			ReviewerID: "system",
			Action:     moderation.ActionTypeRemove,
			Weight:     1000.0, // System reviews have high weight
			Created:    time.Now(),
		}

		storageReview := (*storage.ModerationReview)(review)
		if err := mp.moderationRepo.AddModerationReview(ctx, storageReview); err != nil {
			return fmt.Errorf("failed to add automatic review: %w", err)
		}

		// Process immediately to potentially trigger consensus
		decision, err := mp.consensusEngine.ProcessReview(ctx, event.ID, review)
		if err != nil {
			return fmt.Errorf("failed to process automatic review: %w", err)
		}

		if decision != nil {
			mp.logger.Info("Automatic decision made",
				zap.String("event_id", event.ID),
				zap.String("decision_id", decision.ID),
				zap.String("action", string(decision.Action)),
			)
		}
	}

	return nil
}

// silenceAccount silences a user account
func (mp *ModerationProcessor) silenceAccount(ctx context.Context, username string, reason string) error {
	updates := map[string]interface{}{
		"silenced":        true,
		"silenced_at":     time.Now().Format(time.RFC3339),
		"silenced_reason": reason,
	}
	
	return mp.userRepo.UpdateUser(ctx, username, updates)
}

// suspendAccount suspends a user account
func (mp *ModerationProcessor) suspendAccount(ctx context.Context, username string, reason string) error {
	updates := map[string]interface{}{
		"suspended":        true,
		"suspended_at":     time.Now().Format(time.RFC3339),
		"suspended_reason": reason,
	}
	
	return mp.userRepo.UpdateUser(ctx, username, updates)
}

// removeContent removes content (status/object)
func (mp *ModerationProcessor) removeContent(ctx context.Context, objectID string) error {
	// Delete the object
	return mp.objectRepo.DeleteObject(ctx, objectID)
}

// Helper functions to extract data from DynamoDB records

func getStringAttribute(attr events.DynamoDBAttributeValue) string {
	if attr.DataType() == events.DataTypeString {
		return attr.String()
	}
	return ""
}

func main() {
	// Create moderation processor
	processor := NewModerationProcessor()

	// Start Lambda with traditional approach but Lift-style patterns
	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		start := time.Now()
		requestID := fmt.Sprintf("moderation-processor-%d", time.Now().UnixNano())
		
		// Recovery handling (Lift pattern)
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic in DynamoDB stream handler",
					zap.String("request_id", requestID),
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
			}
		}()

		// Add request ID to context
		ctx = context.WithValue(ctx, "request_id", requestID)

		logger.Info("processing moderation stream batch",
			zap.String("request_id", requestID),
			zap.Int("record_count", len(event.Records)),
		)

		// Process the stream event
		var errors []error
		for _, record := range event.Records {
			if err := processor.processRecord(ctx, record); err != nil {
				logger.Error("Failed to process record",
					zap.String("request_id", requestID),
					zap.String("event_id", record.EventID),
					zap.Error(err),
				)
				errors = append(errors, err)
			}
		}

		// Log completion (Lift pattern)
		duration := time.Since(start)
		if len(errors) > 0 {
			err := fmt.Errorf("failed to process %d of %d records", len(errors), len(event.Records))
			logger.Error("DynamoDB stream processing failed",
				zap.String("request_id", requestID),
				zap.Error(err),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
			return err
		} else {
			logger.Info("DynamoDB stream processing completed",
				zap.String("request_id", requestID),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
		}

		return nil
	})
}