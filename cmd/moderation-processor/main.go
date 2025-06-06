package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/moderation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aron23/lesser/pkg/trust"
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
	return s.storage.GetModerationQueue(ctx, limit, cursor)
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

	// TODO: Send notifications to moderators
	// TODO: Trigger any automatic actions based on severity

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
		// TODO: Implement account silencing
		logger.Info("Would silence account", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeSuspend:
		// TODO: Implement account suspension
		logger.Info("Would suspend account", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeRemove:
		// TODO: Implement content removal
		logger.Info("Would remove content", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeWarning:
		// TODO: Send warning notification
		logger.Info("Would send warning", zap.String("object_id", decision.ObjectID))
	}

	return nil
}

// Helper functions to extract data from DynamoDB records

func getStringAttribute(attr events.DynamoDBAttributeValue) string {
	if attr.DataType() == events.DataTypeString {
		return attr.String()
	}
	return ""
}

func getReviewFromRecord(record events.DynamoDBEventRecord) (*moderation.Review, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "REVIEW" {
		return nil, fmt.Errorf("not a review record")
	}

	// In a real implementation, we would properly unmarshal the Review field
	// For now, create a basic review
	review := &moderation.Review{
		EventID:    "extracted_event_id",
		ReviewerID: "extracted_reviewer_id",
		Action:     moderation.ActionTypeWarning,
		Confidence: 0.8,
	}

	return review, nil
}

func getEventFromRecord(record events.DynamoDBEventRecord) (*moderation.ModerationEvent, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "EVENT" {
		return nil, fmt.Errorf("not an event record")
	}

	// In a real implementation, we would properly unmarshal the Event field
	event := &moderation.ModerationEvent{
		ID:        "extracted_id",
		EventType: moderation.EventTypeFlagged,
		ObjectID:  "extracted_object_id",
		Category:  moderation.CategoryHateSpeech,
		Severity:  moderation.SeverityHigh,
	}

	return event, nil
}

func getDecisionFromRecord(record events.DynamoDBEventRecord) (*moderation.ModerationDecision, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "DECISION" {
		return nil, fmt.Errorf("not a decision record")
	}

	// In a real implementation, we would properly unmarshal the Decision field
	decision := &moderation.ModerationDecision{
		ID:       "extracted_id",
		ObjectID: "extracted_object_id",
		Action:   moderation.ActionTypeWarning,
	}

	return decision, nil
}

func main() {
	lambda.Start(handler)
}
