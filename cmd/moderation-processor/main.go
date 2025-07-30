package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamodb"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	store           storage.Storage
	consensusEngine *moderation.ConsensusEngine
	logger          *zap.Logger
)

// storageAdapter adapts storage.Storage to moderation.StorageInterface
type storageAdapter struct {
	storage storage.Storage
}

func (s *storageAdapter) GetModerationEvent(ctx context.Context, eventID string) (*moderation.ModerationEvent, error) {
	return s.storage.GetModerationEvent(ctx, eventID)
}

func (s *storageAdapter) AddModerationReview(ctx context.Context, review *moderation.Review) error {
	return s.storage.AddModerationReview(ctx, review)
}

func (s *storageAdapter) GetModerationReviews(ctx context.Context, eventID string) ([]*moderation.Review, error) {
	return s.storage.GetModerationReviews(ctx, eventID)
}

func (s *storageAdapter) CreateModerationDecision(ctx context.Context, decision *moderation.ModerationDecision) error {
	return s.storage.CreateModerationDecision(ctx, decision)
}

func (s *storageAdapter) GetModerationQueue(ctx context.Context, limit int, cursor string) ([]*moderation.QueueItem, string, error) {
	return s.storage.GetModerationQueuePaginated(ctx, limit, cursor)
}

func (s *storageAdapter) GetTrustScore(ctx context.Context, actorID, category string) (*trust.TrustScore, error) {
	return s.storage.GetTrustScore(ctx, actorID, category)
}

func (s *storageAdapter) RecordTrustUpdate(ctx context.Context, update *trust.TrustUpdate) error {
	return s.storage.RecordTrustUpdate(ctx, update)
}

func init() {
	// Initialize logger
	logger = common.Logger()

	// Initialize storage
	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}

	// Initialize consensus engine with storage adapter
	adapter := &storageAdapter{storage: store}
	consensusEngine = moderation.NewConsensusEngine(adapter, nil)
}

// handler processes DynamoDB stream events for moderation
func handler(ctx context.Context, event events.DynamoDBEvent) error {
	logger.Info("Processing DynamoDB stream event",
		zap.Int("records", len(event.Records)),
	)

	for _, record := range event.Records {
		if err := processRecord(ctx, record); err != nil {
			// Log error but continue processing other records
			logger.Error("Failed to process record",
				zap.String("event_id", record.EventID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// processRecord processes a single DynamoDB stream record
func processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
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
		return handleNewReview(ctx, record)

	case strings.HasPrefix(pk, "EVENT#") && record.EventName == "INSERT":
		// New moderation event created
		return handleNewEvent(ctx, record)

	case strings.HasPrefix(pk, "DECISION#"):
		// Decision made - trigger actions
		return handleDecision(ctx, record)
	}

	return nil
}

// handleNewReview processes a new review and checks for consensus
func handleNewReview(ctx context.Context, record events.DynamoDBEventRecord) error {
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
	decision, err := consensusEngine.ProcessReview(ctx, eventID, review)
	if err != nil {
		logger.Warn("Failed to process review",
			zap.String("event_id", eventID),
			zap.Error(err),
		)
		return nil // Don't fail the whole batch
	}

	if decision != nil {
		logger.Info("Consensus reached",
			zap.String("event_id", eventID),
			zap.String("decision_id", decision.ID),
			zap.String("action", string(decision.Action)),
			zap.Float64("consensus_score", decision.ConsensusScore),
		)
	}

	return nil
}

// handleNewEvent processes a new moderation event
func handleNewEvent(ctx context.Context, record events.DynamoDBEventRecord) error {
	event, err := getEventFromRecord(record)
	if err != nil {
		return fmt.Errorf("failed to extract event: %w", err)
	}

	logger.Info("New moderation event created",
		zap.String("event_id", event.ID),
		zap.String("object_id", event.ObjectID),
		zap.String("type", string(event.EventType)),
		zap.String("category", string(event.Category)),
		zap.Int("severity", int(event.Severity)),
	)

	// Send notifications to moderators
	if err := sendModeratorNotification(ctx, event); err != nil {
		logger.Error("failed to send moderator notification", zap.Error(err))
	}

	// Trigger automatic actions based on severity
	if err := triggerAutomaticActions(ctx, event); err != nil {
		logger.Error("failed to trigger automatic actions", zap.Error(err))
	}

	return nil
}

// handleDecision processes a moderation decision
func handleDecision(ctx context.Context, record events.DynamoDBEventRecord) error {
	decision, err := getDecisionFromRecord(record)
	if err != nil {
		return fmt.Errorf("failed to extract decision: %w", err)
	}

	logger.Info("Processing moderation decision",
		zap.String("decision_id", decision.ID),
		zap.String("object_id", decision.ObjectID),
		zap.String("action", string(decision.Action)),
	)

	// Apply the decision based on action type
	switch decision.Action {
	case moderation.ActionTypeSilence:
		// Implement account silencing
		if err := silenceAccount(ctx, decision.ObjectID, decision.Reason); err != nil {
			logger.Error("failed to silence account", zap.Error(err))
			return err
		}
		logger.Info("Account silenced", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeSuspend:
		// Implement account suspension
		if err := suspendAccount(ctx, decision.ObjectID, decision.Reason); err != nil {
			logger.Error("failed to suspend account", zap.Error(err))
			return err
		}
		logger.Info("Account suspended", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeRemove:
		// Implement content removal
		if err := removeContent(ctx, decision.ObjectID); err != nil {
			logger.Error("failed to remove content", zap.Error(err))
			return err
		}
		logger.Info("Content removed", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeNone:
		logger.Info("No action taken", zap.String("object_id", decision.ObjectID))

	default:
		logger.Warn("Unknown action type",
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
func sendModeratorNotification(ctx context.Context, event *moderation.ModerationEvent) error {
	// Get list of moderators - using ListUsers with role filter
	users, _, err := store.ListUsers(ctx, 100, "") // Get first 100 users
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	// Filter for moderators
	var moderators []string
	for _, user := range users {
		if user.Role == "moderator" || user.Role == "admin" {
			moderators = append(moderators, user.Username)
		}
	}

	// Create notification for each moderator
	for _, moderatorID := range moderators {
		notification := &storage.Notification{
			ID:        fmt.Sprintf("mod_%s_%d", event.ID, time.Now().UnixNano()),
			Username:  moderatorID,
			Type:      "moderation",
			CreatedAt: time.Now(),
			AccountID: event.ActorID,
			StatusID:  event.ObjectID,
		}

		if err := store.CreateNotification(ctx, notification); err != nil {
			logger.Error("Failed to create notification",
				zap.String("moderator_id", moderatorID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// triggerAutomaticActions triggers automatic actions based on event severity
func triggerAutomaticActions(ctx context.Context, event *moderation.ModerationEvent) error {
	// High severity events trigger automatic actions
	if event.Severity >= 8 {
		logger.Info("High severity event - triggering automatic action",
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

		if err := store.AddModerationReview(ctx, review); err != nil {
			return fmt.Errorf("failed to add automatic review: %w", err)
		}

		// Process immediately to potentially trigger consensus
		decision, err := consensusEngine.ProcessReview(ctx, event.ID, review)
		if err != nil {
			return fmt.Errorf("failed to process automatic review: %w", err)
		}

		if decision != nil {
			logger.Info("Automatic decision made",
				zap.String("event_id", event.ID),
				zap.String("decision_id", decision.ID),
				zap.String("action", string(decision.Action)),
			)
		}
	}

	return nil
}

// silenceAccount silences a user account
func silenceAccount(ctx context.Context, username string, reason string) error {
	updates := map[string]interface{}{
		"silenced":    true,
		"silenced_at": time.Now().Format(time.RFC3339),
		"silenced_reason": reason,
	}
	
	return store.UpdateUser(ctx, username, updates)
}

// suspendAccount suspends a user account
func suspendAccount(ctx context.Context, username string, reason string) error {
	updates := map[string]interface{}{
		"suspended":    true,
		"suspended_at": time.Now().Format(time.RFC3339),
		"suspended_reason": reason,
	}
	
	return store.UpdateUser(ctx, username, updates)
}

// removeContent removes content (status/object)
func removeContent(ctx context.Context, objectID string) error {
	// Delete the object
	return store.DeleteObject(ctx, objectID)
}

// Helper functions to extract data from DynamoDB records

func getStringAttribute(attr events.DynamoDBAttributeValue) string {
	if attr.DataType() == events.DataTypeString {
		return attr.String()
	}
	return ""
}

func main() {
	lambda.Start(handler)
}