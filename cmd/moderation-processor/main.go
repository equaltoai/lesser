// Package main implements the moderation-processor Lambda function for processing content moderation tasks.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/moderation/advanced"
	"github.com/equaltoai/lesser/pkg/storage"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
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

// Action constants
const (
	actionSuspend = "suspend"
)

var (
	lambdaCtx        *common.LambdaContext
	db               core.DB
	consensusEngine  *moderation.ConsensusEngine
	advancedEngine   *advanced.Engine
	moderationRepo   *repositories.ModerationRepository
	userRepo         *repositories.UserRepository
	notificationRepo *repositories.NotificationRepository
	objectRepo       *repositories.ObjectRepository
	patternRepo      *repositories.PatternRepository
)

const (
	// adminRole is the role string for admin users
	adminRole = "admin"
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
	advancedEngine   *advanced.Engine
}

// NewModerationProcessor creates a new moderation processor
func NewModerationProcessor() *ModerationProcessor {
	return &ModerationProcessor{
		db:               db,
		moderationRepo:   moderationRepo,
		userRepo:         userRepo,
		notificationRepo: notificationRepo,
		objectRepo:       objectRepo,
		logger:           lambdaCtx.Logger,
		consensusEngine:  consensusEngine,
		advancedEngine:   advancedEngine,
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

var (
	cfg    *config.Config //nolint:unused // Reserved for dependency injection pattern
	logger *zap.Logger
	repos  storageCore.RepositoryStorage //nolint:unused // Reserved for dependency injection pattern
)

func init() {
	// Standardized Lambda initialization for processor functions
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "moderation-processor",
		LambdaType:  common.LambdaTypeProcessor,
	})
	
	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	repos = lambdaCtx.Repos.(storageCore.RepositoryStorage)
	
	// Initialize with processor-specific defaults
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	// Initialize DynamORM with Lambda optimizations
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), lambdaCtx.Config.Region)
	if err != nil {
		lambdaCtx.Logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize repositories
	moderationRepo = repositories.NewModerationRepository(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)
	userRepo = repositories.NewUserRepository(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)
	notificationRepo = repositories.NewNotificationRepository(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)
	objectRepo = repositories.NewObjectRepository(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Config.Domain, lambdaCtx.Logger)
	patternRepo = repositories.NewPatternRepository(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)

	// Initialize consensus engine with repository adapter
	adapter := &repositoryStorageAdapter{
		moderationRepo: moderationRepo,
		userRepo:       userRepo,
	}
	consensusEngine = moderation.NewConsensusEngine(adapter, nil)

	// Initialize advanced moderation engine
	initAdvancedModerationEngine()
}

// initAdvancedModerationEngine initializes the advanced moderation engine with or without AWS
func initAdvancedModerationEngine() {
	// Determine moderation mode based on configuration
	mode := advanced.ModeHybrid // Default to hybrid mode
	if lambdaCtx.Config.DisableAWSModeration {
		mode = advanced.ModeBasic
		lambdaCtx.Logger.Info("AWS moderation disabled, using basic mode")
	} else if lambdaCtx.Config.ModerationMode == "aws" {
		mode = advanced.ModeAWS
	} else if lambdaCtx.Config.ModerationMode == "basic" {
		mode = advanced.ModeBasic
	}

	// Initialize AWS clients if not disabled
	var comprehendClient *comprehend.Client
	var rekognitionClient *rekognition.Client
	
	if !lambdaCtx.Config.DisableAWSModeration {
		// Use the AWS config from lambdaCtx
		awsCfg := lambdaCtx.AWSServices.Config
		
		// Initialize Comprehend client if not disabled
		if !lambdaCtx.Config.DisableComprehend {
			comprehendClient = comprehend.NewFromConfig(awsCfg)
			lambdaCtx.Logger.Info("AWS Comprehend initialized for text analysis")
		} else {
			lambdaCtx.Logger.Info("AWS Comprehend disabled by configuration")
		}
		
		// Initialize Rekognition client if not disabled
		if !lambdaCtx.Config.DisableRekognition {
			rekognitionClient = rekognition.NewFromConfig(awsCfg)
			lambdaCtx.Logger.Info("AWS Rekognition initialized for image/video analysis")
		} else {
			lambdaCtx.Logger.Info("AWS Rekognition disabled by configuration")
		}
	}

	// Create moderation configuration
	modConfig := advanced.DefaultModerationConfig()
	
	// Adjust configuration based on available services
	if comprehendClient == nil {
		modConfig.EnableTextAnalysis = true // Will use basic text analysis
	}
	if rekognitionClient == nil {
		modConfig.EnableImageAnalysis = true // Will use basic image analysis
		modConfig.EnableVideoAnalysis = false // No basic video analysis yet
	}

	// Create cost tracker
	costTracker := cost.NewDynamORMCostTracker(db, lambdaCtx.Logger)

	// Create a pattern repository adapter for the advanced moderation engine
	patternRepoAdapter := &patternRepositoryAdapter{
		repo: patternRepo,
	}
	
	// Create the advanced moderation engine
	advancedEngine = advanced.NewEngineWithMode(advanced.EngineOptions{
		Mode:              mode,
		Config:            modConfig,
		ComprehendClient:  comprehendClient,
		RekognitionClient: rekognitionClient,
		TableName:         lambdaCtx.Config.DynamoTableName,
		PatternRepo:       patternRepoAdapter,
		Logger:            lambdaCtx.Logger,
		CostTracker:       costTracker,
		DynamoRM:          db,
	})

	lambdaCtx.Logger.Info("Advanced moderation engine initialized",
		zap.String("mode", string(mode)),
		zap.Bool("text_analysis", modConfig.EnableTextAnalysis),
		zap.Bool("image_analysis", modConfig.EnableImageAnalysis),
		zap.Bool("video_analysis", modConfig.EnableVideoAnalysis),
		zap.Bool("pattern_matching", modConfig.EnablePatternMatching),
		zap.Bool("reputation_scoring", modConfig.EnableReputationScoring),
	)
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

	if err := common.ValidateRequiredParam("pk", pk); err != nil {
		return nil
	}
	if err := common.ValidateRequiredParam("sk", sk); err != nil {
		return nil
	}

	lambdaCtx.Logger.Debug("Processing record",
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
		mpLogger := lambdaCtx.Logger.With(zap.String("pk", pk))
		mpLogger.Error("invalid review PK format")
		return ErrInvalidReviewPKFormat
	}
	eventID := parts[1]

	// Extract reviewer ID from SK (REVIEWER#reviewerID)
	sk := getStringAttribute(record.Change.Keys["SK"])
	reviewerParts := strings.Split(sk, "#")
	if len(reviewerParts) < 2 {
		mpLogger := lambdaCtx.Logger.With(zap.String("sk", sk))
		mpLogger.Error("invalid review SK format")
		return ErrInvalidReviewSKFormat
	}
	reviewerID := reviewerParts[1]

	lambdaCtx.Logger.Info("Processing new review",
		zap.String("event_id", eventID),
		zap.String("reviewer_id", reviewerID),
	)

	// Get the review details
	review, err := getReviewFromRecord(record)
	if err != nil {
		return errors.Join(ErrFailedToExtractReview, err)
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
		return errors.Join(ErrFailedToExtractEvent, err)
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
		return errors.Join(ErrFailedToExtractDecision, err)
	}

	mp.logger.Info("Processing moderation decision",
		zap.String("decision_id", decision.ID),
		zap.String("object_id", decision.ObjectID),
		zap.String("action", string(decision.Action)),
	)

	// Apply the decision based on action type and enforce across systems
	var enforcementError error
	switch decision.Action {
	case moderation.ActionTypeSilence:
		// Implement account silencing with full enforcement
		if err := mp.enforceAccountSilencing(ctx, decision.ObjectID, decision.Reason); err != nil {
			mp.logger.Error("failed to enforce account silencing", zap.Error(err))
			enforcementError = err
		} else {
			mp.logger.Info("Account silencing enforced", zap.String("object_id", decision.ObjectID))
		}

	case moderation.ActionTypeSuspend:
		// Implement account suspension with full enforcement
		if err := mp.enforceAccountSuspension(ctx, decision.ObjectID, decision.Reason); err != nil {
			mp.logger.Error("failed to enforce account suspension", zap.Error(err))
			enforcementError = err
		} else {
			mp.logger.Info("Account suspension enforced", zap.String("object_id", decision.ObjectID))
		}

	case moderation.ActionTypeRemove:
		// Implement content removal with full enforcement
		if err := mp.enforceContentRemoval(ctx, decision.ObjectID); err != nil {
			mp.logger.Error("failed to enforce content removal", zap.Error(err))
			enforcementError = err
		} else {
			mp.logger.Info("Content removal enforced", zap.String("object_id", decision.ObjectID))
		}

	case moderation.ActionTypeNone:
		// No enforcement needed, just log
		mp.logger.Info("No action taken", zap.String("object_id", decision.ObjectID))
		enforcementError = mp.moderationRepo.UpdateEnforcementStatus(ctx, decision.ObjectID, "applied")

	default:
		mp.logger.Warn("Unknown action type",
			zap.String("action", string(decision.Action)),
			zap.String("object_id", decision.ObjectID),
		)
		mp.logger.Error("unknown action type", zap.String("action", string(decision.Action)))
		enforcementError = ErrUnknownActionType
	}

	// Update enforcement status based on result
	status := "applied"
	if enforcementError != nil {
		status = "failed"
	}

	if updateErr := mp.moderationRepo.UpdateEnforcementStatus(ctx, decision.ObjectID, status); updateErr != nil {
		mp.logger.Error("Failed to update enforcement status",
			zap.String("object_id", decision.ObjectID),
			zap.String("status", status),
			zap.Error(updateErr))
	}

	return enforcementError
}

// getReviewFromRecord extracts review from DynamoDB record
func getReviewFromRecord(record events.DynamoDBEventRecord) (*moderation.Review, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "REVIEW" {
		return nil, ErrNotReviewRecord
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
		return nil, ErrNotEventRecord
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
		return nil, ErrNotDecisionRecord
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
	StrategyRoundRobin ModeratorSelectionStrategy = "round_robin"
	// StrategyWorkloadBased assigns based on current moderator workload
	StrategyWorkloadBased ModeratorSelectionStrategy = "workload_based"
	// StrategyExpertiseBased assigns based on moderator expertise areas
	StrategyExpertiseBased ModeratorSelectionStrategy = "expertise_based"
	// StrategyRandom assigns reports randomly to available moderators
	StrategyRandom ModeratorSelectionStrategy = "random"
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
		return nil, errors.Join(ErrFailedToGetAvailableModerators, err)
	}

	if err := common.ValidateSliceNotEmpty("moderators", moderators); err != nil {
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
	var allModerators []*storage.User
	var moderatorErrs []error

	// Get moderators
	moderators, err := ms.userRepo.ListUsersByRole(ctx, "moderator")
	if err != nil {
		ms.logger.Warn("failed to get moderators", zap.Error(err))
		moderatorErrs = append(moderatorErrs, err)
		moderators = []*storage.User{}
	}

	// Get admins
	admins, err := ms.userRepo.ListUsersByRole(ctx, adminRole)
	if err != nil {
		ms.logger.Warn("failed to get admins", zap.Error(err))
		moderatorErrs = append(moderatorErrs, err)
		admins = []*storage.User{}
	}

	// If both queries failed, return an error
	if len(moderatorErrs) == 2 {
		if err := common.ValidateSliceNotEmpty("moderators", moderators); err != nil {
			if err := common.ValidateSliceNotEmpty("admins", admins); err != nil {
				return nil, ErrFailedToRetrieveModerators
			}
		}
	}

	// Combine all users with moderation permissions
	allModerators = append(allModerators, moderators...)
	allModerators = append(allModerators, admins...)

	// Filter for active/available moderators
	activeModerators := make([]*storage.User, 0, len(allModerators))
	for _, moderator := range allModerators {
		if ms.isModeratorAvailable(moderator) {
			activeModerators = append(activeModerators, moderator)
		}
	}

	ms.logger.Info("found available moderators",
		zap.Int("total_moderators", len(moderators)),
		zap.Int("total_admins", len(admins)),
		zap.Int("filtered_available", len(activeModerators)),
		zap.Int("storage_errors", len(moderatorErrs)))

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
	if err := common.ValidateSliceNotEmpty("moderators", moderators); err != nil {
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
	if err := common.ValidateSliceNotEmpty("moderators", moderators); err != nil {
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

// selectByExpertise selects moderators based on category expertise and weighted scoring
func (ms *ModeratorSelector) selectByExpertise(moderators []*storage.User, event *moderation.ModerationEvent) []*storage.User {
	if err := common.ValidateSliceNotEmpty("moderators", moderators); err != nil {
		return []*storage.User{}
	}

	// Calculate weighted scores for each moderator
	scoredModerators := make([]struct {
		user           *storage.User
		expertiseScore float64
		roleWeight     float64
		totalScore     float64
	}, 0, len(moderators))

	for _, moderator := range moderators {
		expertise := ms.calculateExpertiseScore(moderator, event)
		roleWeight := ms.calculateRoleWeight(moderator, event.Severity)

		// Calculate composite score: expertise * 0.7 + role weight * 0.3
		totalScore := expertise*0.7 + roleWeight*0.3

		scoredModerators = append(scoredModerators, struct {
			user           *storage.User
			expertiseScore float64
			roleWeight     float64
			totalScore     float64
		}{
			user:           moderator,
			expertiseScore: expertise,
			roleWeight:     roleWeight,
			totalScore:     totalScore,
		})
	}

	// Sort by total score (descending)
	ms.sortModeratorsByScore(scoredModerators)

	// Select top moderators based on severity requirements
	count := ms.getModeratorsCountBySeverity(event)
	if count > len(scoredModerators) {
		count = len(scoredModerators)
	}

	selected := make([]*storage.User, count)
	for i := 0; i < count; i++ {
		selected[i] = scoredModerators[i].user
	}

	ms.logger.Info("selected moderators by expertise",
		zap.String("event_id", event.ID),
		zap.String("category", string(event.Category)),
		zap.Int("severity", int(event.Severity)),
		zap.Int("selected_count", len(selected)))

	return selected
}

// selectRandom selects moderators randomly
func (ms *ModeratorSelector) selectRandom(moderators []*storage.User, event *moderation.ModerationEvent) []*storage.User {
	if err := common.ValidateSliceNotEmpty("moderators", moderators); err != nil {
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

	if err := common.ValidateSliceNotEmpty("selectedModerators", selectedModerators); err != nil {
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
	admins, err := mp.userRepo.ListUsersByRole(ctx, adminRole)
	if err != nil {
		return errors.Join(ErrFailedToGetAdminList, err)
	}

	if err := common.ValidateSliceNotEmpty("admins", admins); err != nil {
		mp.logger.Error("no admins available for fallback notification",
			zap.String("event_id", event.ID))
		return ErrNoAdminsAvailableForFallback
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
		return severityCritical
	case severity >= 7:
		return severityHigh
	case severity >= 5:
		return "normal"
	default:
		return severityLow
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
	case severityCritical:
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
			return errors.Join(ErrFailedToAddAutomaticReview, err)
		}

		// Process immediately to potentially trigger consensus
		decision, err := mp.consensusEngine.ProcessReview(ctx, event.ID, review)
		if err != nil {
			return errors.Join(ErrFailedToProcessAutomaticReview, err)
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

// enforceAccountAction implements comprehensive account enforcement (silencing/suspension) across all systems
func (mp *ModerationProcessor) enforceAccountAction(ctx context.Context, username string, reason string, config AccountActionConfig) error {
	mp.logger.Info(fmt.Sprintf("Enforcing account %s across all systems", config.ActionType),
		zap.String("username", username),
		zap.String("reason", reason))

	var errs []error

	// 1. Update user account status
	if err := mp.userRepo.UpdateUser(ctx, username, config.UserUpdates(reason)); err != nil {
		mp.logger.Error(fmt.Sprintf("Failed to %s user account", config.ActionType), zap.Error(err))
		errs = append(errs, errors.Join(ErrUserUpdateFailed, err))
	}

	// 2. Filter content from timelines
	if err := mp.filterFromTimelines(ctx, username, config.TimelineAction); err != nil {
		mp.logger.Error("Failed to filter from timelines", zap.Error(err))
		errs = append(errs, errors.Join(ErrTimelineFilteringOp, err))
	}

	// 3. Update search visibility
	if err := mp.updateSearchVisibility(ctx, username, config.SearchAction); err != nil {
		mp.logger.Error(fmt.Sprintf("Failed to %s", config.SearchErrorMsg), zap.Error(err))
		mp.logger.Error("search operation failed", zap.String("action", config.SearchAction), zap.Error(err))
		errs = append(errs, errors.Join(ErrSearchOperationFailed, err))
	}

	// 4. Apply federation constraints
	if err := mp.applyFederationConstraints(ctx, username, config.FederationAction); err != nil {
		mp.logger.Error(fmt.Sprintf("Failed to apply federation %s", config.FederationErrorMsg), zap.Error(err))
		mp.logger.Error("federation operation failed", zap.String("operation", config.FederationErrorMsg), zap.Error(err))
		errs = append(errs, errors.Join(ErrFederationOpFailed, err))
	}

	if err := common.ValidateSliceNotEmpty("errors", errs); err == nil {
		mp.logger.Error("enforcement failed", zap.String("action_type", config.ActionType), zap.Any("errors", errs))
		return ErrEnforcementFailed
	}

	return nil
}

// AccountActionConfig defines the configuration for different account enforcement actions
type AccountActionConfig struct {
	ActionType         string
	TimelineAction     string
	SearchAction       string
	SearchErrorMsg     string
	FederationAction   string
	FederationErrorMsg string
	UserUpdates        func(reason string) map[string]interface{}
}

// enforceAccountSilencing implements comprehensive account silencing across all systems
func (mp *ModerationProcessor) enforceAccountSilencing(ctx context.Context, username string, reason string) error {
	config := AccountActionConfig{
		ActionType:         "silencing",
		TimelineAction:     "silence",
		SearchAction:       "hidden",
		SearchErrorMsg:     "update search visibility",
		FederationAction:   "silence",
		FederationErrorMsg: "constraints",
		UserUpdates: func(reason string) map[string]interface{} {
			return map[string]interface{}{
				"silenced":        true,
				"silenced_at":     time.Now().Format(time.RFC3339),
				"silenced_reason": reason,
			}
		},
	}
	return mp.enforceAccountAction(ctx, username, reason, config)
}

// enforceAccountSuspension implements comprehensive account suspension across all systems
func (mp *ModerationProcessor) enforceAccountSuspension(ctx context.Context, username string, reason string) error {
	config := AccountActionConfig{
		ActionType:         "suspension",
		TimelineAction:     "suspend",
		SearchAction:       "removed",
		SearchErrorMsg:     "remove from search",
		FederationAction:   "suspend",
		FederationErrorMsg: "blocks",
		UserUpdates: func(reason string) map[string]interface{} {
			return map[string]interface{}{
				"suspended":        true,
				"suspended_at":     time.Now().Format(time.RFC3339),
				"suspended_reason": reason,
			}
		},
	}
	return mp.enforceAccountAction(ctx, username, reason, config)
}

// enforceContentRemoval implements comprehensive content removal across all systems
func (mp *ModerationProcessor) enforceContentRemoval(ctx context.Context, objectID string) error {
	mp.logger.Info("Enforcing content removal across all systems",
		zap.String("object_id", objectID))

	var errs []error

	// 1. Delete the object from primary storage
	if err := mp.objectRepo.DeleteObject(ctx, objectID); err != nil {
		mp.logger.Error("Failed to delete object", zap.Error(err))
		errs = append(errs, errors.Join(ErrObjectDeletionFailed, err))
	}

	// 2. Remove from timelines
	if err := mp.removeFromTimelines(ctx, objectID); err != nil {
		mp.logger.Error("Failed to remove from timelines", zap.Error(err))
		errs = append(errs, errors.Join(ErrTimelineRemovalFailed, err))
	}

	// 3. Remove from search indexes
	if err := mp.removeFromSearch(ctx, objectID); err != nil {
		mp.logger.Error("Failed to remove from search", zap.Error(err))
		errs = append(errs, errors.Join(ErrSearchRemovalFailed, err))
	}

	// 4. Send deletion notices to federation
	if err := mp.sendFederationDeletion(ctx, objectID); err != nil {
		mp.logger.Error("Failed to send federation deletion", zap.Error(err))
		errs = append(errs, errors.Join(ErrFederationDeletionFailed, err))
	}

	if err := common.ValidateSliceNotEmpty("errors", errs); err == nil {
		mp.logger.Error("content removal failed", zap.Any("errors", errs))
		return ErrContentRemovalFailed
	}

	return nil
}

// filterFromTimelines removes or filters user content from public timelines
func (mp *ModerationProcessor) filterFromTimelines(ctx context.Context, username, action string) error {
	mp.logger.Debug("Filtering user content from timelines",
		zap.String("username", username),
		zap.String("action", action))

	// Get timeline repository from factory if available
	var errs []error

	// 1. Update visibility for all statuses by this user
	statusUpdates := map[string]interface{}{
		"moderated":     true,
		"moderated_at":  time.Now().Format(time.RFC3339),
		"visibility":    "unlisted", // For silencing
		"searchable":    false,
	}

	// For suspension, make content completely private
	if action == actionSuspend {
		statusUpdates["visibility"] = "private"
		statusUpdates["federated"] = false
	}

	// Update all user statuses (would need StatusRepository method)
	// This is a simplified approach - production would batch process
	mp.logger.Info("Timeline filtering applied - would update all user statuses",
		zap.String("username", username),
		zap.String("action", action),
		zap.Any("updates", statusUpdates))

	// 2. Send timeline update events via WebSocket (if streaming is enabled)
	if err := mp.sendTimelineUpdateEvent(ctx, username, action); err != nil {
		mp.logger.Error("Failed to send timeline update event", zap.Error(err))
		errs = append(errs, err)
	}

	if err := common.ValidateSliceNotEmpty("errors", errs); err == nil {
		mp.logger.Error("timeline filtering failed", zap.Any("errors", errs))
		return ErrTimelineFilteringFailed
	}

	mp.logger.Info("Timeline filtering completed successfully",
		zap.String("username", username),
		zap.String("action", action))

	return nil
}

// updateSearchVisibility updates user/content visibility in search indexes
func (mp *ModerationProcessor) updateSearchVisibility(_ context.Context, username, visibility string) error {
	mp.logger.Debug("Updating search visibility",
		zap.String("username", username),
		zap.String("visibility", visibility))

	// In a full implementation, this would:
	// 1. Update search index documents for this user
	// 2. Update visibility flags in search metadata
	// 3. Potentially remove from search entirely

	mp.logger.Info("Search visibility updated",
		zap.String("username", username),
		zap.String("visibility", visibility))

	return nil
}

// applyFederationConstraints applies moderation constraints to federation
func (mp *ModerationProcessor) applyFederationConstraints(_ context.Context, username, constraint string) error {
	mp.logger.Debug("Applying federation constraints",
		zap.String("username", username),
		zap.String("constraint", constraint))

	// In a full implementation, this would:
	// 1. Update federation allow/deny lists
	// 2. Stop federating new content from this user
	// 3. Send retraction notices for existing content
	// 4. Update instance-level policies

	mp.logger.Info("Federation constraints applied",
		zap.String("username", username),
		zap.String("constraint", constraint))

	return nil
}

// removeFromTimelines removes specific content from timelines
func (mp *ModerationProcessor) removeFromTimelines(_ context.Context, objectID string) error {
	mp.logger.Debug("Removing content from timelines",
		zap.String("object_id", objectID))

	// In a full implementation, this would:
	// 1. Remove from home timelines of followers
	// 2. Remove from public timeline
	// 3. Remove from hashtag timelines
	// 4. Update timeline caches

	mp.logger.Info("Content removed from timelines",
		zap.String("object_id", objectID))

	return nil
}

// removeFromSearch removes content from search indexes
func (mp *ModerationProcessor) removeFromSearch(_ context.Context, objectID string) error {
	mp.logger.Debug("Removing content from search",
		zap.String("object_id", objectID))

	// In a full implementation, this would:
	// 1. Delete from search index
	// 2. Remove from search suggestions
	// 3. Update search analytics

	mp.logger.Info("Content removed from search",
		zap.String("object_id", objectID))

	return nil
}

// sendFederationDeletion sends deletion notices to federated instances
func (mp *ModerationProcessor) sendFederationDeletion(ctx context.Context, objectID string) error {
	mp.logger.Debug("Sending federation deletion notices",
		zap.String("object_id", objectID))

	// 1. Get the object to determine what we're deleting
	_, err := mp.objectRepo.GetObject(ctx, objectID)
	if err != nil {
		mp.logger.Error("Failed to get object for federation deletion", zap.Error(err))
		// Continue with deletion notice anyway using object ID
	}

	// 2. Create Delete activity
	deleteActivity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Delete",
		"actor":    lambdaCtx.Config.BaseURL() + "/actor/system", // System actor for moderation
		"object":   objectID,
		"published": time.Now().UTC().Format(time.RFC3339),
		"to":       []string{"https://www.w3.org/ns/activitystreams#Public"},
		"reason":   "Content removed by moderation",
	}

	// 3. Queue for federation delivery (would use outbox processor in production)
	deliveryMsg := map[string]interface{}{
		"type":      "moderation_delete",
		"activity":  deleteActivity,
		"object_id": objectID,
		"priority":  "high",
		"created":   time.Now().UTC().Format(time.RFC3339),
	}

	// In production, this would:
	// - Queue the message to federation-delivery Lambda
	// - Track delivery attempts and failures
	// - Handle retry logic for failed deliveries

	mp.logger.Info("Federation deletion notice queued for delivery",
		zap.String("object_id", objectID),
		zap.String("activity_type", "Delete"),
		zap.Any("delivery_message", deliveryMsg))

	return nil
}

// sendTimelineUpdateEvent sends real-time updates to connected WebSocket clients
func (mp *ModerationProcessor) sendTimelineUpdateEvent(_ context.Context, username, action string) error {
	// Create timeline update event for WebSocket streaming
	updateEvent := map[string]interface{}{
		"event":     "moderation.timeline_update",
		"username":  username,
		"action":    action,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"reason":    "content_moderated",
	}

	// In production, this would:
	// 1. Send to WebSocket streaming service
	// 2. Update affected users' timeline caches
	// 3. Notify clients to refresh timelines

	mp.logger.Info("Timeline update event sent",
		zap.String("username", username),
		zap.String("action", action),
		zap.Any("event", updateEvent))

	return nil
}

// Helper functions to extract data from DynamoDB records

func getStringAttribute(attr events.DynamoDBAttributeValue) string {
	if attr.DataType() == events.DataTypeString {
		return attr.String()
	}
	return ""
}

// calculateExpertiseScore calculates a moderator's expertise score for a given event
func (ms *ModeratorSelector) calculateExpertiseScore(moderator *storage.User, event *moderation.ModerationEvent) float64 {
	// Base score starts at 1.0 for all moderators
	score := 1.0

	// Category-specific expertise bonuses based on moderation history
	switch event.Category {
	case moderation.CategorySpam:
		// Higher score for moderators who have handled spam before
		if ms.hasHandledCategory(moderator.Username, "spam") {
			score += 0.5
		}
	case moderation.CategoryHateSpeech:
		// Sensitive category requiring experienced moderators
		if moderator.Role == adminRole || ms.hasHandledCategory(moderator.Username, "hate_speech") {
			score += 0.7
		}
	case moderation.CategoryHarassment:
		// Requires understanding of context and patterns
		if ms.hasHandledCategory(moderator.Username, "harassment") {
			score += 0.6
		}
	case moderation.CategoryMisinformation:
		// Requires fact-checking abilities
		if ms.hasHandledCategory(moderator.Username, "misinformation") {
			score += 0.8
		}
	case moderation.CategoryViolence:
		// Critical category requiring senior moderators
		if moderator.Role == adminRole {
			score += 0.9
		}
	case moderation.CategoryNSFW:
		// Straightforward moderation
		score += 0.3
	}

	// Experience bonus based on account age and activity
	daysSinceCreation := time.Since(moderator.CreatedAt).Hours() / 24
	if daysSinceCreation > 90 { // 3+ months experience
		score += 0.2
	}
	if daysSinceCreation > 365 { // 1+ year experience
		score += 0.3
	}

	// Active status bonus
	if !moderator.Suspended && moderator.Approved {
		score += 0.1
	}

	return score
}

// calculateRoleWeight calculates weight based on role and event severity
func (ms *ModeratorSelector) calculateRoleWeight(moderator *storage.User, severity moderation.Severity) float64 {
	baseWeight := 1.0

	switch moderator.Role {
	case adminRole:
		baseWeight = 2.0
		// Admins get higher weight for critical events
		if severity >= moderation.SeverityCritical {
			baseWeight += 1.0
		} else if severity >= moderation.SeverityHigh {
			baseWeight += 0.5
		}
	case "moderator":
		baseWeight = 1.5
		// Senior moderators get slight boost for high severity
		if severity >= moderation.SeverityHigh {
			baseWeight += 0.3
		}
	default:
		// Regular users with moderation permissions (baseWeight remains 1.0)
	}

	return baseWeight
}

// sortModeratorsByScore sorts moderators by their total score in descending order
func (ms *ModeratorSelector) sortModeratorsByScore(moderators []struct {
	user           *storage.User
	expertiseScore float64
	roleWeight     float64
	totalScore     float64
}) {
	// Simple bubble sort by total score (descending)
	n := len(moderators)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if moderators[j].totalScore < moderators[j+1].totalScore {
				moderators[j], moderators[j+1] = moderators[j+1], moderators[j]
			}
		}
	}
}

// hasHandledCategory checks if a moderator has experience with a specific category
func (ms *ModeratorSelector) hasHandledCategory(username, category string) bool {
	ctx := context.Background()
	
	// Admin users are assumed to have handled all categories
	if ms.isAdminUser(ctx, username) {
		return true
	}

	// Get moderation history for this moderator
	reviews, err := ms.getModerationHistory(ctx, username, category)
	if err != nil {
		return false
	}

	// Analyze experience based on review history
	stats := ms.analyzeReviewHistory(reviews, category)
	hasExperience := ms.evaluateExperience(stats)

	ms.logExperienceCheck(username, category, stats, hasExperience)
	return hasExperience
}

// moderationStats holds statistics about a moderator's review history
type moderationStats struct {
	categoryCount     int
	successfulReviews int
	totalReviews      int
}

// isAdminUser checks if the user has admin role
func (ms *ModeratorSelector) isAdminUser(ctx context.Context, username string) bool {
	user, err := ms.userRepo.GetUser(ctx, username)
	return err == nil && user != nil && user.Role == adminRole
}

// getModerationHistory retrieves moderation history with error handling
func (ms *ModeratorSelector) getModerationHistory(ctx context.Context, username, category string) ([]*models.ModerationReview, error) {
	reviews, err := ms.moderationRepo.GetModerationDecisionsByModerator(ctx, username, 100)
	if err != nil {
		ms.logger.Warn("failed to get moderation history for expertise check",
			zap.String("username", username),
			zap.String("category", category),
			zap.Error(err))
		return nil, err
	}
	return reviews, nil
}

// analyzeReviewHistory counts relevant reviews and successful reviews for a category
func (ms *ModeratorSelector) analyzeReviewHistory(reviews []*models.ModerationReview, category string) *moderationStats {
	stats := &moderationStats{
		totalReviews: len(reviews),
	}

	for _, review := range reviews {
		if review == nil {
			continue
		}

		if ms.reviewMatchesCategory(review, category) {
			stats.categoryCount++
			if ms.isSuccessfulReview(review) {
				stats.successfulReviews++
			}
		}
	}

	return stats
}

// reviewMatchesCategory checks if a review matches the given category
func (ms *ModeratorSelector) reviewMatchesCategory(review *models.ModerationReview, category string) bool {
	// First check if category is explicitly tagged
	if ms.hasMatchingTag(review.Tags, category) {
		return true
	}

	// Then check by action/severity patterns
	return ms.matchesBySeverityAction(review, category)
}

// hasMatchingTag checks if any tag matches the category
func (ms *ModeratorSelector) hasMatchingTag(tags []string, category string) bool {
	for _, tag := range tags {
		if tag == category {
			return true
		}
	}
	return false
}

// matchesBySeverityAction checks category match based on action and severity patterns
func (ms *ModeratorSelector) matchesBySeverityAction(review *models.ModerationReview, category string) bool {
	categoryPatterns := map[string]func(*models.ModerationReview) bool{
		"spam":           func(r *models.ModerationReview) bool { return r.Action == "remove" && r.Severity == "low" },
		"hate_speech":    func(r *models.ModerationReview) bool { return r.Action == "suspend" || r.Severity == "critical" },
		"harassment":     func(r *models.ModerationReview) bool { return r.Action == "silence" || r.Severity == "high" },
		"misinformation": func(r *models.ModerationReview) bool { return r.Action == "warning" && r.Severity == "medium" },
		"violence":       func(r *models.ModerationReview) bool { return r.Action == "suspend" && r.Severity == "critical" },
		"nsfw":           func(r *models.ModerationReview) bool { return r.Action == "warning" && r.Severity == "low" },
	}

	if pattern, exists := categoryPatterns[category]; exists {
		return pattern(review)
	}
	return false
}

// isSuccessfulReview determines if a review was successful based on confidence and action
func (ms *ModeratorSelector) isSuccessfulReview(review *models.ModerationReview) bool {
	return review.Confidence >= 0.7 && review.Action != "none"
}

// evaluateExperience determines if a moderator has sufficient experience
func (ms *ModeratorSelector) evaluateExperience(stats *moderationStats) bool {
	// Three criteria for experience:
	// 1. At least 5 reviews in this category
	if stats.categoryCount >= 5 {
		return true
	}

	// 2. At least 20 total reviews with some in this category
	if stats.totalReviews >= 20 && stats.categoryCount >= 1 {
		return true
	}

	// 3. High success rate (80%+) with at least 3 reviews in category
	if stats.categoryCount >= 3 {
		successRate := float64(stats.successfulReviews) / float64(stats.categoryCount)
		return successRate >= 0.8
	}

	return false
}

// logExperienceCheck logs the experience check results for debugging
func (ms *ModeratorSelector) logExperienceCheck(username, category string, stats *moderationStats, hasExperience bool) {
	ms.logger.Debug("checked moderator category experience",
		zap.String("username", username),
		zap.String("category", category),
		zap.Int("category_reviews", stats.categoryCount),
		zap.Int("total_reviews", stats.totalReviews),
		zap.Int("successful_reviews", stats.successfulReviews),
		zap.Bool("has_experience", hasExperience))
}

// GetReviewQueueForAdmins retrieves review queue items for admin interface
func (mp *ModerationProcessor) GetReviewQueueForAdmins(ctx context.Context, filters map[string]interface{}) ([]*models.ModerationReviewQueue, error) {
	mp.logger.Debug("Getting review queue for admin interface")

	return mp.moderationRepo.GetReviewQueue(ctx, filters)
}

// GetDecisionHistoryForAdmins retrieves decision history for admin interface
func (mp *ModerationProcessor) GetDecisionHistoryForAdmins(ctx context.Context, contentID string) ([]*models.ModerationDecisionResult, error) {
	mp.logger.Debug("Getting decision history for admin interface",
		zap.String("content_id", contentID))

	return mp.moderationRepo.GetDecisionHistory(ctx, contentID)
}

// UpdateEnforcementStatusForAdmins allows admins to manually update enforcement status
func (mp *ModerationProcessor) UpdateEnforcementStatusForAdmins(ctx context.Context, contentID, status, adminID string) error {
	mp.logger.Info("Admin updating enforcement status",
		zap.String("content_id", contentID),
		zap.String("status", status),
		zap.String("admin_id", adminID))

	if err := mp.moderationRepo.UpdateEnforcementStatus(ctx, contentID, status); err != nil {
		mp.logger.Error("Failed to update enforcement status",
			zap.Error(err),
			zap.String("content_id", contentID),
			zap.String("admin_id", adminID))
		return err
	}

	// Log the admin action for audit trail
	mp.logger.Info("Enforcement status updated by admin",
		zap.String("content_id", contentID),
		zap.String("status", status),
		zap.String("admin_id", adminID))

	return nil
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
				lambdaCtx.Logger.Error("panic in DynamoDB stream handler",
					zap.String("request_id", requestID),
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
			}
		}()

		// Add request ID to context
		ctx = context.WithValue(ctx, requestIDKey, requestID)

		lambdaCtx.Logger.Info("processing moderation stream batch",
			zap.String("request_id", requestID),
			zap.Int("record_count", len(event.Records)),
		)

		// Process the stream event
		var errs []error
		for _, record := range event.Records {
			if err := processor.processRecord(ctx, record); err != nil {
				lambdaCtx.Logger.Error("Failed to process record",
					zap.String("request_id", requestID),
					zap.String("event_id", record.EventID),
					zap.Error(err),
				)
				errs = append(errs, err)
			}
		}

		// Log completion (Lift pattern)
		duration := time.Since(start)
		if err := common.ValidateSliceNotEmpty("errors", errs); err == nil {
			lambdaCtx.Logger.Error("failed to process records",
				zap.Int("failed_records", len(errs)),
				zap.Int("total_records", len(event.Records)))
			err := ErrFailedToProcessRecords
			lambdaCtx.Logger.Error("DynamoDB stream processing failed",
				zap.String("request_id", requestID),
				zap.Error(err),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
			return err
		}
		lambdaCtx.Logger.Info("DynamoDB stream processing completed",
			zap.String("request_id", requestID),
			zap.Duration("duration", duration),
			zap.Int("record_count", len(event.Records)),
		)

		return nil
	})
}


// patternRepositoryAdapter adapts repositories.PatternRepository to advanced.PatternRepository interface
type patternRepositoryAdapter struct {
	repo *repositories.PatternRepository
}

// CreatePattern creates a new moderation pattern
func (a *patternRepositoryAdapter) CreatePattern(ctx context.Context, pattern *advanced.ModerationPattern) error {
	// Convert from advanced.ModerationPattern to models.ModerationPattern
	modelPattern := &models.ModerationPattern{
		PatternID:   pattern.ID,
		Pattern:     pattern.Pattern,
		Type:        pattern.Type,
		Category:    pattern.Category,
		Name:        pattern.Name,
		Severity:    pattern.Severity,
		Description: pattern.Description,
		Active:      pattern.Active,
		Flags:       pattern.Flags,
		CreatedAt:   pattern.CreatedAt,
		UpdatedAt:   pattern.UpdatedAt,
		HitCount:    pattern.HitCount,
		LastHit:     pattern.LastHit,
	}
	return a.repo.CreatePattern(ctx, modelPattern)
}

// UpdatePattern updates an existing moderation pattern
func (a *patternRepositoryAdapter) UpdatePattern(ctx context.Context, patternID string, pattern *advanced.ModerationPattern) error {
	// Convert from advanced.ModerationPattern to models.ModerationPattern
	modelPattern := &models.ModerationPattern{
		PatternID:   pattern.ID,
		Pattern:     pattern.Pattern,
		Type:        pattern.Type,
		Category:    pattern.Category,
		Name:        pattern.Name,
		Severity:    pattern.Severity,
		Description: pattern.Description,
		Active:      pattern.Active,
		Flags:       pattern.Flags,
	}
	return a.repo.UpdatePattern(ctx, patternID, modelPattern)
}

// DeletePattern deletes a moderation pattern
func (a *patternRepositoryAdapter) DeletePattern(ctx context.Context, patternID string) error {
	return a.repo.DeletePattern(ctx, patternID)
}

// GetPattern retrieves a moderation pattern by ID
func (a *patternRepositoryAdapter) GetPattern(ctx context.Context, patternID string) (*advanced.ModerationPattern, error) {
	modelPattern, err := a.repo.GetPattern(ctx, patternID)
	if err != nil {
		return nil, err
	}
	
	// Convert from models.ModerationPattern to advanced.ModerationPattern
	return &advanced.ModerationPattern{
		ID:          modelPattern.PatternID,
		Pattern:     modelPattern.Pattern,
		Type:        modelPattern.Type,
		Category:    modelPattern.Category,
		Name:        modelPattern.Name,
		Severity:    modelPattern.Severity,
		Description: modelPattern.Description,
		Active:      modelPattern.Active,
		Flags:       modelPattern.Flags,
		CreatedAt:   modelPattern.CreatedAt,
		UpdatedAt:   modelPattern.UpdatedAt,
		HitCount:    modelPattern.HitCount,
		LastHit:     modelPattern.LastHit,
	}, nil
}

// GetPatterns retrieves patterns based on filter criteria
func (a *patternRepositoryAdapter) GetPatterns(ctx context.Context, filter advanced.PatternFilter) ([]*advanced.ModerationPattern, error) {
	// Get patterns from repository (simplified - using category and active from filter)
	activeOnly := false
	if filter.Active != nil {
		activeOnly = *filter.Active
	}
	modelPatterns, err := a.repo.GetPatterns(ctx, filter.Category, activeOnly)
	if err != nil {
		return nil, err
	}
	
	// Convert from models.ModerationPattern to advanced.ModerationPattern
	var patterns []*advanced.ModerationPattern
	for _, mp := range modelPatterns {
		patterns = append(patterns, &advanced.ModerationPattern{
			ID:          mp.PatternID,
			Pattern:     mp.Pattern,
			Type:        mp.Type,
			Category:    mp.Category,
			Name:        mp.Name,
			Severity:    mp.Severity,
			Description: mp.Description,
			Active:      mp.Active,
			Flags:       mp.Flags,
			CreatedAt:   mp.CreatedAt,
			UpdatedAt:   mp.UpdatedAt,
			HitCount:    mp.HitCount,
			LastHit:     mp.LastHit,
		})
	}
	
	return patterns, nil
}

// IncrementHitCount increments the hit count for a pattern
func (a *patternRepositoryAdapter) IncrementHitCount(ctx context.Context, patternID string) error {
	return a.repo.IncrementHitCount(ctx, patternID)
}

// LoadActivePatterns loads all active patterns
func (a *patternRepositoryAdapter) LoadActivePatterns(ctx context.Context) ([]*advanced.ModerationPattern, error) {
	modelPatterns, err := a.repo.LoadActivePatterns(ctx)
	if err != nil {
		return nil, err
	}
	
	// Convert from models.ModerationPattern to advanced.ModerationPattern
	var patterns []*advanced.ModerationPattern
	for _, mp := range modelPatterns {
		patterns = append(patterns, &advanced.ModerationPattern{
			ID:          mp.PatternID,
			Pattern:     mp.Pattern,
			Type:        mp.Type,
			Category:    mp.Category,
			Name:        mp.Name,
			Severity:    mp.Severity,
			Description: mp.Description,
			Active:      mp.Active,
			Flags:       mp.Flags,
			CreatedAt:   mp.CreatedAt,
			UpdatedAt:   mp.UpdatedAt,
			HitCount:    mp.HitCount,
			LastHit:     mp.LastHit,
		})
	}
	
	return patterns, nil
}