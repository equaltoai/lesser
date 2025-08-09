// Package main implements the moderation-processor Lambda function for processing content moderation tasks.
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
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const requestIDKey contextKey = "request_id"

// Severity string constants
const (
	severityLow      = "low"
	severityMedium   = "medium"
	severityHigh     = "high"
	severityCritical = "critical"
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
	return convertStorageToModerationEvent(storageEvent), nil
}

func (s *repositoryStorageAdapter) AddModerationReview(ctx context.Context, review *moderation.Review) error {
	// Convert moderation.Review to storage.ModerationReview
	storageReview := convertModerationToStorageReview(review)
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
		reviews[i] = convertStorageToModerationReview(sr)
	}
	return reviews, nil
}

func (s *repositoryStorageAdapter) CreateModerationDecision(ctx context.Context, decision *moderation.ModerationDecision) error {
	// Convert moderation.ModerationDecision to storage.ModerationDecision
	storageDecision := convertModerationToStorageDecision(decision)
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
		items[i] = convertStorageToModerationQueueItem(si)
	}
	return items, nextCursor, nil
}

func (s *repositoryStorageAdapter) GetTrustScore(ctx context.Context, actorID, category string) (*models.TrustScore, error) {
	// Use the UserRepository which has trust functionality implemented
	score, err := s.userRepo.GetTrustScore(ctx, actorID, category)
	if err != nil {
		return nil, err
	}
	// Convert storage.TrustScore to models.TrustScore
	return &models.TrustScore{
		ActorID:         score.ActorID,
		Category:        score.Category,
		Score:           score.Score,
		DirectScore:     score.DirectScore,
		PropagatedScore: score.PropagatedScore,
		Confidence:      score.Confidence,
		TrusterCount:    score.TrusterCount,
		LastCalculated:  score.LastCalculated,
		CacheTTL:        score.CacheTTL,
	}, nil
}

func (s *repositoryStorageAdapter) RecordTrustUpdate(ctx context.Context, update *models.TrustUpdate) error {
	// Use the UserRepository which has trust functionality implemented
	// Convert models.TrustUpdate to storage.TrustUpdate
	return s.userRepo.RecordTrustUpdate(ctx, &storage.TrustUpdate{
		ActorID:   update.ActorID,
		Category:  update.Category,
		Delta:     update.Delta,
		Reason:    update.Reason,
		EventID:   update.EventID,
		Timestamp: update.Timestamp,
	})
}

// Conversion functions between moderation and storage types

func convertStorageToModerationEvent(storageEvent *storage.ModerationEvent) *moderation.ModerationEvent {
	if storageEvent == nil {
		return nil
	}

	// Convert Evidence from []any to []Evidence
	var evidence []moderation.Evidence
	for _, e := range storageEvent.Evidence {
		if evidenceMap, ok := e.(map[string]interface{}); ok {
			evidence = append(evidence, moderation.Evidence{
				Type:        getStringFromMap(evidenceMap, "type"),
				Score:       getFloatFromMap(evidenceMap, "score"),
				Description: getStringFromMap(evidenceMap, "description"),
				Metadata:    getMapFromMap(evidenceMap, "metadata"),
				Timestamp:   getTimeFromMap(evidenceMap, "timestamp"),
			})
		}
	}

	return &moderation.ModerationEvent{
		ID:              storageEvent.ID,
		EventType:       moderation.EventType(storageEvent.EventType),
		ObjectID:        storageEvent.ObjectID,
		ObjectType:      storageEvent.ObjectType,
		ActorID:         storageEvent.ActorID,
		Category:        moderation.Category(storageEvent.Category),
		Severity:        parseSeverity(storageEvent.Severity),
		ConfidenceScore: storageEvent.ConfidenceScore,
		Evidence:        evidence,
		Reason:          storageEvent.Reason,
		Created:         storageEvent.Created,
		Updated:         storageEvent.Updated,
		TTL:             storageEvent.TTL,
	}
}

func convertStorageToModerationReview(storageReview *storage.ModerationReview) *moderation.Review {
	if storageReview == nil {
		return nil
	}

	return &moderation.Review{
		ID:         storageReview.ID,
		EventID:    storageReview.EventID,
		ReviewerID: storageReview.ReviewerID,
		Action:     moderation.ActionType(storageReview.Action),
		Category:   parseCategoryFromString(storageReview.Severity), // Map severity to category
		Severity:   parseSeverity(storageReview.Severity),
		Confidence: storageReview.Confidence,
		Notes:      storageReview.Note,
		Weight:     storageReview.ReviewerRep, // Use reviewer rep as weight
		Created:    storageReview.Created,
	}
}

func convertModerationToStorageReview(review *moderation.Review) *storage.ModerationReview {
	if review == nil {
		return nil
	}

	return &storage.ModerationReview{
		ID:          review.ID,
		EventID:     review.EventID,
		ReviewerID:  review.ReviewerID,
		ReviewerRep: review.Weight,
		Action:      string(review.Action),
		Severity:    severityToString(review.Severity),
		Note:        review.Notes,
		Confidence:  review.Confidence,
		Created:     review.Created,
	}
}

func convertModerationToStorageDecision(decision *moderation.ModerationDecision) *storage.ModerationDecision {
	if decision == nil {
		return nil
	}

	// Convert Reviews to interface{} slice
	reviews := make([]interface{}, 0, len(decision.Reviews))
	for _, review := range decision.Reviews {
		reviews = append(reviews, convertModerationToStorageReview(review))
	}

	storageDecision := &storage.ModerationDecision{
		ID:               decision.ID,
		EventID:          decision.EventID,
		ObjectID:         decision.ObjectID,
		Action:           string(decision.Action),
		ConsensusScore:   decision.ConsensusScore,
		ReviewerCount:    decision.ReviewerCount,
		TrustWeightTotal: decision.TrustWeightTotal,
		Reviews:          reviews,
		Decided:          decision.Decided,
	}

	// Set expires if AppliedAt is set
	if decision.AppliedAt != nil {
		storageDecision.Expires = decision.AppliedAt
	}

	return storageDecision
}

func convertStorageToModerationQueueItem(storageItem *storage.ModerationQueueItem) *moderation.QueueItem {
	if storageItem == nil {
		return nil
	}

	return &moderation.QueueItem{
		Event:          convertStorageToModerationEvent(storageItem.Event),
		Priority:       float64(storageItem.Priority),
		ReviewCount:    storageItem.ReviewCount,
		LastReviewedAt: storageItem.ReviewedAt,
	}
}

// Helper functions for type conversions

func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloatFromMap(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
		if i, ok := v.(int); ok {
			return float64(i)
		}
	}
	return 0.0
}

func getMapFromMap(m map[string]interface{}, key string) map[string]any {
	if v, ok := m[key]; ok {
		if subMap, ok := v.(map[string]interface{}); ok {
			result := make(map[string]any)
			for k, v := range subMap {
				result[k] = v
			}
			return result
		}
	}
	return nil
}

func getTimeFromMap(m map[string]interface{}, key string) time.Time {
	if v, ok := m[key]; ok {
		if t, ok := v.(time.Time); ok {
			return t
		}
		if s, ok := v.(string); ok {
			if parsed, err := time.Parse(time.RFC3339, s); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func parseSeverity(severityStr string) moderation.Severity {
	switch strings.ToLower(severityStr) {
	case severityLow, "1":
		return moderation.SeverityLow
	case severityMedium, "2":
		return moderation.SeverityMedium
	case severityHigh, "3":
		return moderation.SeverityHigh
	case severityCritical, "4":
		return moderation.SeverityCritical
	default:
		return moderation.SeverityMedium
	}
}

func severityToString(severity moderation.Severity) string {
	switch severity {
	case moderation.SeverityLow:
		return severityLow
	case moderation.SeverityMedium:
		return severityMedium
	case moderation.SeverityHigh:
		return severityHigh
	case moderation.SeverityCritical:
		return severityCritical
	default:
		return severityMedium
	}
}

func parseCategoryFromString(s string) moderation.Category {
	switch strings.ToLower(s) {
	case "spam":
		return moderation.CategorySpam
	case "hate_speech":
		return moderation.CategoryHateSpeech
	case "harassment":
		return moderation.CategoryHarassment
	case "misinformation":
		return moderation.CategoryMisinformation
	case "nsfw":
		return moderation.CategoryNSFW
	case "violence":
		return moderation.CategoryViolence
	default:
		return moderation.CategoryOther
	}
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

// ModerationAssignment represents a moderation task assignment
type ModerationAssignment struct {
	ID           string    `json:"id"`
	EventID      string    `json:"event_id"`
	ModeratorID  string    `json:"moderator_id"`
	Priority     string    `json:"priority"`
	AssignedAt   time.Time `json:"assigned_at"`
	Deadline     time.Time `json:"deadline"`
	AutoAssigned bool      `json:"auto_assigned"`
	Strategy     string    `json:"strategy"`
}

// ModeratorSelectionStrategy defines how moderators are selected for assignments
type ModeratorSelectionStrategy string

// Moderator selection strategies
const (
	// StrategyRoundRobin assigns reports to moderators in round-robin fashion
	StrategyRoundRobin     ModeratorSelectionStrategy = "round_robin"
	// StrategyWorkloadBased assigns based on current moderator workload
	StrategyWorkloadBased  ModeratorSelectionStrategy = "workload_based"
	// StrategyExpertiseBased assigns based on moderator expertise areas
	StrategyExpertiseBased ModeratorSelectionStrategy = "expertise_based"
	// StrategyRandom assigns reports randomly to available moderators
	StrategyRandom         ModeratorSelectionStrategy = "random"
)

// ModeratorSelector handles intelligent moderator selection and assignment
type ModeratorSelector struct {
	userRepo         *repositories.UserRepository
	moderationRepo   *repositories.ModerationRepository
	logger           *zap.Logger
	lastAssignment   map[string]int // Track round-robin state
	assignmentCounts map[string]int // Track workload
}

// NewModeratorSelector creates a new moderator selector
func NewModeratorSelector(userRepo *repositories.UserRepository, moderationRepo *repositories.ModerationRepository, logger *zap.Logger) *ModeratorSelector {
	return &ModeratorSelector{
		userRepo:         userRepo,
		moderationRepo:   moderationRepo,
		logger:           logger,
		lastAssignment:   make(map[string]int),
		assignmentCounts: make(map[string]int),
	}
}

// SelectModerators selects appropriate moderators for a moderation event
func (ms *ModeratorSelector) SelectModerators(ctx context.Context, event *moderation.ModerationEvent, strategy ModeratorSelectionStrategy) ([]*storage.User, error) {
	// Get all moderators and admins
	moderators, err := ms.getAvailableModerators(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get available moderators: %w", err)
	}

	if len(moderators) == 0 {
		ms.logger.Warn("no moderators available for assignment", 
			zap.String("event_id", event.ID))
		return []*storage.User{}, nil
	}

	// Apply selection strategy
	switch strategy {
	case StrategyRoundRobin:
		return ms.selectRoundRobin(moderators, event), nil
	case StrategyWorkloadBased:
		return ms.selectByWorkload(ctx, moderators, event)
	case StrategyExpertiseBased:
		return ms.selectByExpertise(moderators, event), nil
	case StrategyRandom:
		return ms.selectRandom(moderators, event), nil
	default:
		return ms.selectRoundRobin(moderators, event), nil
	}
}

// getAvailableModerators retrieves all users with moderator or admin roles
func (ms *ModeratorSelector) getAvailableModerators(ctx context.Context) ([]*storage.User, error) {
	// Get moderators
	moderators, err := ms.userRepo.ListUsersByRole(ctx, "moderator")
	if err != nil {
		ms.logger.Warn("failed to get moderators", zap.Error(err))
		moderators = []*storage.User{}
	}

	// Get admins
	admins, err := ms.userRepo.ListUsersByRole(ctx, "admin")
	if err != nil {
		ms.logger.Warn("failed to get admins", zap.Error(err))
		admins = []*storage.User{}
	}

	// Combine and filter active users
	allModerators := append(moderators, admins...)
	activeModerators := make([]*storage.User, 0, len(allModerators))

	for _, moderator := range allModerators {
		if ms.isModeratorAvailable(moderator) {
			activeModerators = append(activeModerators, moderator)
		}
	}

	ms.logger.Info("found available moderators", 
		zap.Int("total_moderators", len(moderators)),
		zap.Int("total_admins", len(admins)),
		zap.Int("available", len(activeModerators)))

	return activeModerators, nil
}

// isModeratorAvailable checks if a moderator is available for assignments
func (ms *ModeratorSelector) isModeratorAvailable(moderator *storage.User) bool {
	// Filter out suspended or inactive accounts
	if moderator.Suspended || !moderator.Approved {
		return false
	}

	// Could add additional availability checks here:
	// - Last seen timestamp
	// - Current workload
	// - Time zone considerations
	// - Manual availability status

	return true
}

// selectRoundRobin selects moderators using round-robin strategy
func (ms *ModeratorSelector) selectRoundRobin(moderators []*storage.User, event *moderation.ModerationEvent) []*storage.User {
	if len(moderators) == 0 {
		return []*storage.User{}
	}

	// Determine number of moderators to assign based on severity
	count := ms.getModeratorsCountBySeverity(event)
	if count > len(moderators) {
		count = len(moderators)
	}

	selected := make([]*storage.User, 0, count)
	startIndex := ms.lastAssignment["global"] % len(moderators)

	for i := 0; i < count; i++ {
		index := (startIndex + i) % len(moderators)
		selected = append(selected, moderators[index])
	}

	// Update round-robin state
	ms.lastAssignment["global"] = (startIndex + count) % len(moderators)

	return selected
}

// selectByWorkload selects moderators based on current workload
func (ms *ModeratorSelector) selectByWorkload(ctx context.Context, moderators []*storage.User, event *moderation.ModerationEvent) ([]*storage.User, error) {
	if len(moderators) == 0 {
		return []*storage.User{}, nil
	}

	// Get current workloads for each moderator
	workloads := make(map[string]int)
	for _, moderator := range moderators {
		// Count pending assignments for this moderator
		count, err := ms.moderationRepo.GetPendingModerationCount(ctx, moderator.Username)
		if err != nil {
			ms.logger.Warn("failed to get workload for moderator", 
				zap.String("moderator", moderator.Username), 
				zap.Error(err))
			count = 0
		}
		workloads[moderator.Username] = count
	}

	// Sort by workload (ascending)
	type moderatorWorkload struct {
		user     *storage.User
		workload int
	}

	workloadList := make([]moderatorWorkload, 0, len(moderators))
	for _, moderator := range moderators {
		workloadList = append(workloadList, moderatorWorkload{
			user:     moderator,
			workload: workloads[moderator.Username],
		})
	}

	// Sort by workload
	for i := 0; i < len(workloadList); i++ {
		for j := i + 1; j < len(workloadList); j++ {
			if workloadList[i].workload > workloadList[j].workload {
				workloadList[i], workloadList[j] = workloadList[j], workloadList[i]
			}
		}
	}

	// Select moderators with lowest workload
	count := ms.getModeratorsCountBySeverity(event)
	if count > len(workloadList) {
		count = len(workloadList)
	}

	selected := make([]*storage.User, 0, count)
	for i := 0; i < count; i++ {
		selected = append(selected, workloadList[i].user)
	}

	return selected, nil
}

// selectByExpertise selects moderators based on category expertise
func (ms *ModeratorSelector) selectByExpertise(moderators []*storage.User, event *moderation.ModerationEvent) []*storage.User {
	if len(moderators) == 0 {
		return []*storage.User{}
	}

	// For now, prioritize admins for high-severity events
	// In a full implementation, you'd have expertise mappings
	selected := make([]*storage.User, 0)

	// Prioritize admins for critical events
	if event.Severity >= 8 {
		for _, moderator := range moderators {
			if moderator.Role == "admin" {
				selected = append(selected, moderator)
			}
		}
	}

	// If no admins or not critical, fall back to round-robin
	if len(selected) == 0 {
		return ms.selectRoundRobin(moderators, event)
	}

	// Limit based on severity
	count := ms.getModeratorsCountBySeverity(event)
	if count > len(selected) {
		count = len(selected)
	}

	return selected[:count]
}

// selectRandom selects moderators randomly
func (ms *ModeratorSelector) selectRandom(moderators []*storage.User, event *moderation.ModerationEvent) []*storage.User {
	if len(moderators) == 0 {
		return []*storage.User{}
	}

	count := ms.getModeratorsCountBySeverity(event)
	if count > len(moderators) {
		count = len(moderators)
	}

	// Simple pseudo-random selection based on event ID hash
	selected := make([]*storage.User, 0, count)
	hash := 0
	for _, char := range event.ID {
		hash = (hash*31 + int(char)) % len(moderators)
	}

	for i := 0; i < count; i++ {
		index := (hash + i) % len(moderators)
		selected = append(selected, moderators[index])
	}

	return selected
}

// getModeratorsCountBySeverity returns how many moderators to assign based on event severity
func (ms *ModeratorSelector) getModeratorsCountBySeverity(event *moderation.ModerationEvent) int {
	switch {
	case event.Severity >= 9: // Critical
		return 3
	case event.Severity >= 7: // High
		return 2
	default: // Normal/Low
		return 1
	}
}

// getAssignmentStrategy determines the assignment strategy based on event properties
func (mp *ModerationProcessor) getAssignmentStrategy(event *moderation.ModerationEvent) ModeratorSelectionStrategy {
	// Use expertise-based for high severity events
	if event.Severity >= 8 {
		return StrategyExpertiseBased
	}

	// Use workload-based during high activity
	// Could check queue size or time of day here
	return StrategyWorkloadBased
}

// sendModeratorNotification sends notifications to selected moderators about moderation events
func (mp *ModerationProcessor) sendModeratorNotification(ctx context.Context, event *moderation.ModerationEvent) error {
	// Create moderator selector
	selector := NewModeratorSelector(mp.userRepo, mp.moderationRepo, mp.logger)

	// Determine assignment strategy
	strategy := mp.getAssignmentStrategy(event)

	// Select appropriate moderators
	selectedModerators, err := selector.SelectModerators(ctx, event, strategy)
	if err != nil {
		mp.logger.Error("failed to select moderators", 
			zap.String("event_id", event.ID),
			zap.Error(err))
		return err
	}

	if len(selectedModerators) == 0 {
		mp.logger.Warn("no moderators available for assignment",
			zap.String("event_id", event.ID),
			zap.String("category", string(event.Category)),
			zap.Int("severity", int(event.Severity)))

		// Fallback: notify all admins if no moderators available
		return mp.notifyFallbackAdmins(ctx, event)
	}

	// Create assignments and notifications
	for i, moderator := range selectedModerators {
		// Create assignment record
		assignment := &ModerationAssignment{
			ID:           fmt.Sprintf("assign_%s_%s_%d", event.ID, moderator.Username, time.Now().UnixNano()),
			EventID:      event.ID,
			ModeratorID:  moderator.Username,
			Priority:     mp.getPriorityString(event.Severity),
			AssignedAt:   time.Now(),
			Deadline:     mp.calculateDeadline(event.Severity),
			AutoAssigned: true,
			Strategy:     string(strategy),
		}

		// Store assignment (would need assignment repository in full implementation)
		mp.logger.Info("moderator assigned to event",
			zap.String("event_id", event.ID),
			zap.String("moderator_id", moderator.Username),
			zap.String("strategy", string(strategy)),
			zap.Int("position", i+1),
			zap.Int("total_assigned", len(selectedModerators)))

		// Create notification
		notification := &models.Notification{
			ID:         fmt.Sprintf("mod_%s_%s_%d", event.ID, moderator.Username, time.Now().UnixNano()),
			UserID:     moderator.Username,
			Type:       "moderation",
			ActorID:    event.ActorID,
			TargetID:   event.ObjectID,
			TargetType: "moderation_event",
			Title:      mp.getNotificationTitle(event),
			Body:       mp.getNotificationBody(event, assignment),
			IsRead:     false,
			CreatedAt:  time.Now(),
		}

		if err := mp.notificationRepo.CreateNotification(ctx, notification); err != nil {
			mp.logger.Error("failed to create moderator notification",
				zap.String("moderator_id", moderator.Username),
				zap.String("event_id", event.ID),
				zap.Error(err))
		}
	}

	return nil
}

// notifyFallbackAdmins notifies all admins when no moderators are available
func (mp *ModerationProcessor) notifyFallbackAdmins(ctx context.Context, event *moderation.ModerationEvent) error {
	admins, err := mp.userRepo.ListUsersByRole(ctx, "admin")
	if err != nil {
		return fmt.Errorf("failed to get admin list for fallback notification: %w", err)
	}

	if len(admins) == 0 {
		mp.logger.Error("no admins available for fallback notification",
			zap.String("event_id", event.ID))
		return fmt.Errorf("no admins available for fallback")
	}

	for _, admin := range admins {
		if !admin.Suspended && admin.Approved {
			notification := &models.Notification{
				ID:         fmt.Sprintf("fallback_%s_%s_%d", event.ID, admin.Username, time.Now().UnixNano()),
				UserID:     admin.Username,
				Type:       "moderation_urgent",
				ActorID:    event.ActorID,
				TargetID:   event.ObjectID,
				TargetType: "moderation_event",
				Title:      "URGENT: No Moderators Available",
				Body:       fmt.Sprintf("Critical moderation event requires immediate attention - no moderators available. Category: %s, Severity: %d", event.Category, int(event.Severity)),
				IsRead:     false,
				CreatedAt:  time.Now(),
			}

			if err := mp.notificationRepo.CreateNotification(ctx, notification); err != nil {
				mp.logger.Error("failed to create fallback admin notification",
					zap.String("admin_id", admin.Username),
					zap.Error(err))
			}
		}
	}

	mp.logger.Info("fallback admin notifications sent",
		zap.String("event_id", event.ID),
		zap.Int("admins_notified", len(admins)))

	return nil
}

// Helper methods for notification content and timing

func (mp *ModerationProcessor) getPriorityString(severity moderation.Severity) string {
	switch {
	case severity >= 9:
		return "critical"
	case severity >= 7:
		return "high"
	case severity >= 5:
		return "normal"
	default:
		return "low"
	}
}

func (mp *ModerationProcessor) calculateDeadline(severity moderation.Severity) time.Time {
	now := time.Now()
	switch {
	case severity >= 9: // Critical - 15 minutes
		return now.Add(15 * time.Minute)
	case severity >= 7: // High - 2 hours
		return now.Add(2 * time.Hour)
	case severity >= 5: // Normal - 24 hours
		return now.Add(24 * time.Hour)
	default: // Low - 7 days
		return now.Add(7 * 24 * time.Hour)
	}
}

func (mp *ModerationProcessor) getNotificationTitle(event *moderation.ModerationEvent) string {
	priority := mp.getPriorityString(event.Severity)
	switch priority {
	case "critical":
		return "🚨 CRITICAL Moderation Required"
	case "high":
		return "⚠️ High Priority Moderation"
	case "normal":
		return "📋 Moderation Review Required"
	default:
		return "📝 Low Priority Review"
	}
}

func (mp *ModerationProcessor) getNotificationBody(event *moderation.ModerationEvent, assignment *ModerationAssignment) string {
	return fmt.Sprintf(
		"New %s content flagged for review.\n"+
			"Priority: %s\n"+
			"Deadline: %s\n"+
			"Assignment Strategy: %s\n"+
			"Event ID: %s",
		event.Category,
		assignment.Priority,
		assignment.Deadline.Format("15:04 Jan 02"),
		assignment.Strategy,
		event.ID,
	)
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

		storageReview := convertModerationToStorageReview(review)
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
		ctx = context.WithValue(ctx, requestIDKey, requestID)

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
		}
		logger.Info("DynamoDB stream processing completed",
			zap.String("request_id", requestID),
			zap.Duration("duration", duration),
			zap.Int("record_count", len(event.Records)),
		)

		return nil
	})
}
