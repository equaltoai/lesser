// Package ai provides AI integration services for content moderation and assistance
package ai

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Error constants for AI service
var (
	// ErrSaveAnalysis is returned when AI analysis saving fails
	ErrSaveAnalysis = errors.New("failed to save AI analysis")
)

type aiRepository interface {
	SaveAnalysis(ctx context.Context, analysis *ai.AIAnalysis) error
	GetAnalysis(ctx context.Context, objectID string) (*ai.AIAnalysis, error)
	QueueForAnalysis(ctx context.Context, objectID string) error
	GetStats(ctx context.Context, period string) (*ai.AIStats, error)
}

// Service provides AI analysis operations following the service-first architecture
type Service struct {
	storage   core.RepositoryStorage
	publisher streaming.Publisher
	logger    *zap.Logger
	aiRepo    aiRepository
}

// NewService creates a new AI service
func NewService(storage core.RepositoryStorage, publisher streaming.Publisher, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	// For now, we need to type assert to get the factory
	var aiRepo aiRepository
	if f, ok := storage.(*factory.RepositoryFactory); ok {
		aiRepo = f.AI()
	}

	return &Service{
		storage:   storage,
		publisher: publisher,
		logger:    logger.With(zap.String("service", "ai")),
		aiRepo:    aiRepo,
	}
}

// SaveAnalysisCommand contains the AI analysis to save
type SaveAnalysisCommand struct {
	Analysis *ai.AIAnalysis
	UserID   string // User context for event publishing
}

// SaveAnalysisResult contains the result of saving an analysis
type SaveAnalysisResult struct {
	Success bool
	Events  []*streaming.Event
}

// SaveAnalysis saves an AI analysis result and publishes events
func (s *Service) SaveAnalysis(ctx context.Context, cmd *SaveAnalysisCommand) (*SaveAnalysisResult, error) {
	if cmd == nil {
		return nil, &ValidationError{Field: "command", Message: "required"}
	}
	if cmd.Analysis == nil {
		return nil, &ValidationError{Field: "analysis", Message: "required"}
	}
	if s.aiRepo == nil {
		return nil, errors.New("AI repository not configured")
	}

	// Save the analysis using the repository
	err := s.aiRepo.SaveAnalysis(ctx, cmd.Analysis)
	if err != nil {
		s.logger.Error("failed to save AI analysis",
			zap.String("analysis_id", cmd.Analysis.ID),
			zap.String("object_id", cmd.Analysis.ObjectID),
			zap.Error(err))
		return nil, errors.Join(ErrSaveAnalysis, err)
	}

	// Publish event via queue publisher for DynamoDB Streams delivery
	var events []*streaming.Event
	if s.publisher != nil {
		event := s.createAnalysisEvent(cmd.Analysis, cmd.UserID)

		// Publish to stream for real-time subscriptions via stream-router
		if err := s.publisher.PublishToStream(ctx, "ai_analysis", event); err != nil {
			s.logger.Warn("failed to publish AI analysis event to stream",
				zap.String("analysis_id", cmd.Analysis.ID),
				zap.Error(err))
		}

		// Also publish to user if specified - stream-router will deliver to user's WebSocket connections
		if cmd.UserID != "" {
			if err := s.publisher.PublishToUser(ctx, cmd.UserID, event); err != nil {
				s.logger.Warn("failed to publish AI analysis event to user",
					zap.String("user_id", cmd.UserID),
					zap.String("analysis_id", cmd.Analysis.ID),
					zap.Error(err))
			}
		}

		events = append(events, event)
	}

	s.logger.Info("AI analysis saved and events published",
		zap.String("analysis_id", cmd.Analysis.ID),
		zap.String("object_id", cmd.Analysis.ObjectID),
		zap.Float64("overall_risk", cmd.Analysis.OverallRisk),
		zap.String("moderation_action", cmd.Analysis.ModerationAction))

	return &SaveAnalysisResult{
		Success: true,
		Events:  events,
	}, nil
}

// createAnalysisEvent creates a streaming event for an AI analysis
func (s *Service) createAnalysisEvent(analysis *ai.AIAnalysis, userID string) *streaming.Event {
	// Create AIEventPayload following the standard structure
	aiPayload := &streaming.AIEventPayload{
		AnalysisID:   analysis.ID,
		ContentID:    analysis.ObjectID,
		ContentType:  analysis.ObjectType,
		AnalysisType: "comprehensive", // comprehensive analysis includes all types
		Results:      s.convertAnalysisToResults(analysis),
		Confidence:   analysis.Confidence,
		ModelVersion: analysis.Version,
		ProcessedAt:  analysis.AnalyzedAt,
	}

	// Convert to generic payload for streaming.Event
	payload := map[string]interface{}{
		"analysis_id":       aiPayload.AnalysisID,
		"content_id":        aiPayload.ContentID,
		"content_type":      aiPayload.ContentType,
		"analysis_type":     aiPayload.AnalysisType,
		"results":           aiPayload.Results,
		"confidence":        aiPayload.Confidence,
		"model_version":     aiPayload.ModelVersion,
		"processed_at":      aiPayload.ProcessedAt,
		"moderation_action": analysis.ModerationAction,
		"overall_risk":      analysis.OverallRisk,
		"user_id":           userID,
	}

	// Determine stream name based on priority
	streamName := "ai_analysis"
	if s.isHighPriority(analysis) {
		streamName = "ai_analysis_urgent"
	}

	return &streaming.Event{
		Type:      "ai.analysis.completed",
		Stream:    streamName,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

// convertAnalysisToResults converts AI analysis to a results map
func (s *Service) convertAnalysisToResults(analysis *ai.AIAnalysis) map[string]interface{} {
	results := make(map[string]interface{})

	// Add text analysis results
	if analysis.TextAnalysis != nil {
		results["text"] = map[string]interface{}{
			"sentiment":        analysis.TextAnalysis.Sentiment,
			"sentiment_scores": analysis.TextAnalysis.SentimentScores,
			"toxicity_score":   analysis.TextAnalysis.ToxicityScore,
			"toxicity_labels":  analysis.TextAnalysis.ToxicityLabels,
			"contains_pii":     analysis.TextAnalysis.ContainsPII,
			"pii_entities":     analysis.TextAnalysis.PIIEntities,
			"language":         analysis.TextAnalysis.DominantLanguage,
			"key_phrases":      analysis.TextAnalysis.KeyPhrases,
		}
	}

	// Add image analysis results
	if analysis.ImageAnalysis != nil {
		results["image"] = map[string]interface{}{
			"is_nsfw":           analysis.ImageAnalysis.IsNSFW,
			"nsfw_confidence":   analysis.ImageAnalysis.NSFWConfidence,
			"violence_score":    analysis.ImageAnalysis.ViolenceScore,
			"weapons_detected":  analysis.ImageAnalysis.WeaponsDetected,
			"deepfake_score":    analysis.ImageAnalysis.DeepfakeScore,
			"moderation_labels": analysis.ImageAnalysis.ModerationLabels,
			"detected_text":     analysis.ImageAnalysis.DetectedText,
		}
	}

	// Add AI detection results
	if analysis.AIDetection != nil {
		results["ai_detection"] = map[string]interface{}{
			"ai_generated_probability": analysis.AIDetection.AIGeneratedProbability,
			"generation_model":         analysis.AIDetection.GenerationModel,
			"pattern_consistency":      analysis.AIDetection.PatternConsistency,
			"style_deviation":          analysis.AIDetection.StyleDeviation,
			"semantic_coherence":       analysis.AIDetection.SemanticCoherence,
			"suspicious_patterns":      analysis.AIDetection.SuspiciousPatterns,
		}
	}

	// Add spam analysis results
	if analysis.SpamAnalysis != nil {
		results["spam"] = map[string]interface{}{
			"spam_score":       analysis.SpamAnalysis.SpamScore,
			"spam_indicators":  analysis.SpamAnalysis.SpamIndicators,
			"posting_velocity": analysis.SpamAnalysis.PostingVelocity,
			"repetition_score": analysis.SpamAnalysis.RepetitionScore,
			"link_density":     analysis.SpamAnalysis.LinkDensity,
			"follower_ratio":   analysis.SpamAnalysis.FollowerRatio,
		}
	}

	// Add overall assessment
	results["overall"] = map[string]interface{}{
		"risk_score":        analysis.OverallRisk,
		"moderation_action": analysis.ModerationAction,
		"confidence":        analysis.Confidence,
	}

	return results
}

// isHighPriority determines if the analysis requires high priority handling
func (s *Service) isHighPriority(analysis *ai.AIAnalysis) bool {
	// High priority for content that needs immediate moderation
	if analysis.ModerationAction == ai.ActionRemove || analysis.ModerationAction == ai.ActionHide {
		return true
	}

	// High priority for high-risk content
	if analysis.OverallRisk > 0.8 {
		return true
	}

	return false
}

// GetAnalysisQuery contains parameters for getting AI analysis
type GetAnalysisQuery struct {
	ObjectID string
}

// GetAnalysisResult contains the result of getting AI analysis
type GetAnalysisResult struct {
	Analysis *ai.AIAnalysis
	Events   []*streaming.Event
}

// GetAnalysis retrieves AI analysis for an object
func (s *Service) GetAnalysis(ctx context.Context, query *GetAnalysisQuery) (*GetAnalysisResult, error) {
	if query == nil {
		return nil, &ValidationError{Field: "query", Message: "required"}
	}
	if err := common.ValidateRequiredParam("query.ObjectID", query.ObjectID); err != nil {
		return nil, &ValidationError{Field: "object_id", Message: "required"}
	}
	if s.aiRepo == nil {
		return nil, errors.New("AI repository not configured")
	}

	analysis, err := s.aiRepo.GetAnalysis(ctx, query.ObjectID)
	if err != nil {
		s.logger.Error("failed to get AI analysis",
			zap.String("object_id", query.ObjectID),
			zap.Error(err))
		return nil, err
	}

	return &GetAnalysisResult{
		Analysis: analysis,
		Events:   []*streaming.Event{},
	}, nil
}

// QueueAnalysisCommand contains parameters for queuing AI analysis
type QueueAnalysisCommand struct {
	ObjectID   string
	ObjectType string
	Force      bool
}

// QueueAnalysisResult contains the result of queuing AI analysis
type QueueAnalysisResult struct {
	Queued bool
	Events []*streaming.Event
}

// QueueForAnalysis queues an object for AI analysis
func (s *Service) QueueForAnalysis(ctx context.Context, cmd *QueueAnalysisCommand) (*QueueAnalysisResult, error) {
	if cmd == nil {
		return nil, &ValidationError{Field: "command", Message: "required"}
	}
	if err := common.ValidateRequiredParam("cmd.ObjectID", cmd.ObjectID); err != nil {
		return nil, &ValidationError{Field: "object_id", Message: "required"}
	}
	if s.aiRepo == nil {
		return nil, errors.New("AI repository not configured")
	}

	// Check if analysis exists and is recent (unless force is true)
	if !cmd.Force {
		existing, _ := s.aiRepo.GetAnalysis(ctx, cmd.ObjectID)
		if existing != nil && time.Since(existing.AnalyzedAt) < 24*time.Hour {
			return &QueueAnalysisResult{
				Queued: false,
				Events: []*streaming.Event{},
			}, nil
		}
	}

	// Queue for analysis
	err := s.aiRepo.QueueForAnalysis(ctx, cmd.ObjectID)
	if err != nil {
		s.logger.Error("failed to queue object for analysis",
			zap.String("object_id", cmd.ObjectID),
			zap.Error(err))
		return nil, err
	}

	// Events are not published for AI analysis queueing (system-level operation)
	var events []*streaming.Event

	return &QueueAnalysisResult{
		Queued: true,
		Events: events,
	}, nil
}

// GetStatsQuery contains parameters for getting AI stats
type GetStatsQuery struct {
	Period string // "day", "week", "month"
}

// GetStatsResult contains the result of getting AI stats
type GetStatsResult struct {
	Stats  interface{}
	Events []*streaming.Event
}

// GetStats retrieves AI analysis statistics
func (s *Service) GetStats(ctx context.Context, query *GetStatsQuery) (*GetStatsResult, error) {
	if query == nil {
		return nil, &ValidationError{Field: "query", Message: "required"}
	}
	if s.aiRepo == nil {
		return nil, errors.New("AI repository not configured")
	}

	period := query.Period
	if err := common.ValidateRequiredParam("period", period); err != nil {
		period = "day"
	}

	stats, err := s.aiRepo.GetStats(ctx, period)
	if err != nil {
		s.logger.Error("failed to get AI stats",
			zap.String("period", period),
			zap.Error(err))
		return nil, err
	}

	return &GetStatsResult{
		Stats:  stats,
		Events: []*streaming.Event{},
	}, nil
}

// AnalysisEvent represents an AI analysis event for subscriptions
type AnalysisEvent struct {
	ID                   string
	ObjectID             string
	ObjectType           string
	AnalysisType         string
	Results              map[string]interface{}
	Confidence           float64
	ModelVersion         string
	ProcessedAt          time.Time
	ModerationAction     string
	ModerationConfidence float64
	ModerationReason     string
}

// SubscribeToAnalysisEvents creates a channel for receiving AI analysis events
// DEPRECATED: This method is deprecated on Lambda. Use GraphQL subscriptions (SubscribeToAIAnalysis) instead,
// which properly persists subscriptions in DynamoDB and delivers via stream-router.
func (s *Service) SubscribeToAnalysisEvents(_ context.Context, userID string, objectID *string) (<-chan *AnalysisEvent, error) {
	// Return empty channel with deprecation warning
	// This functionality is replaced by GraphQL subscriptions in the graph layer
	s.logger.Warn("SubscribeToAnalysisEvents called - this method is deprecated on Lambda, use GraphQL subscriptions instead",
		zap.String("user_id", userID),
		zap.Bool("filtered", objectID != nil && *objectID != ""))

	eventChan := make(chan *AnalysisEvent, 100)
	close(eventChan) // Close immediately to prevent blocking

	return eventChan, nil
}

// ConvertToModel converts an AI analysis to a DynamORM model
func (s *Service) ConvertToModel(analysis *ai.AIAnalysis) *models.AIAnalysis {
	model := &models.AIAnalysis{
		ID:               analysis.ID,
		ObjectID:         analysis.ObjectID,
		ObjectType:       analysis.ObjectType,
		AnalyzedAt:       analysis.AnalyzedAt,
		Version:          analysis.Version,
		TextAnalysis:     analysis.TextAnalysis,
		ImageAnalysis:    analysis.ImageAnalysis,
		AIDetection:      analysis.AIDetection,
		SpamAnalysis:     analysis.SpamAnalysis,
		OverallRisk:      analysis.OverallRisk,
		ModerationAction: analysis.ModerationAction,
		Confidence:       analysis.Confidence,
		CreatedAt:        analysis.AnalyzedAt,
		UpdatedAt:        time.Now(),
	}

	// Update the DynamoDB keys
	if err := model.UpdateKeys(); err != nil {
		s.logger.Error("failed to update AI analysis keys", zap.Error(err))
		// Continue returning model since this function doesn't return errors
	}

	return model
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
