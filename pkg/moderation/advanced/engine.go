package advanced

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	tmtypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rekognitionTypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/common"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// Engine is the main moderation engine implementation
type Engine struct {
	config *ModerationConfig
	logger *zap.Logger

	// Analyzers (interfaces to support both AWS and non-AWS implementations)
	textAnalyzer  TextAnalyzerInterface
	imageAnalyzer ImageAnalyzerInterface

	// Components
	patternMatcher   *PatternMatcher
	reputationScorer *ReputationScorer
	decisionEngine   *DecisionEngine
	threatIntel      *ThreatIntelligence

	// Storage
	dynamoRM  core.DB
	tableName string

	// Metrics
	metrics     *ModerationMetrics
	costTracker CostTracker
}

// NewEngine creates a new moderation engine
func NewEngine(
	config *ModerationConfig,
	comprehendClient *comprehend.Client,
	rekognitionClient *rekognition.Client,
	tableName string,
	patternRepo PatternRepository,
	logger *zap.Logger,
	costTracker CostTracker,
	dynamoRM core.DB,
) *Engine {
	// Create analyzers based on configuration and available clients
	var textAnalyzer TextAnalyzerInterface
	var imageAnalyzer ImageAnalyzerInterface

	// Check global AWS moderation flags from lesser config
	// This allows runtime configuration without requiring mode changes
	awsModDisabled := false
	comprehendDisabled := false
	rekognitionDisabled := false

	// Check if global config is available (imported from pkg/config)
	// This is a safe way to check feature flags without hard dependency
	if globalCfg := getGlobalConfig(); globalCfg != nil {
		awsModDisabled = globalCfg.GetDisableAWSModeration()
		comprehendDisabled = globalCfg.GetDisableComprehend()
		rekognitionDisabled = globalCfg.GetDisableRekognition()

		logger.Info("checked global moderation feature flags",
			zap.Bool("aws_moderation_disabled", awsModDisabled),
			zap.Bool("comprehend_disabled", comprehendDisabled),
			zap.Bool("rekognition_disabled", rekognitionDisabled))
	}

	// Use AWS analyzers only if:
	// 1. Feature is enabled in moderation config
	// 2. AWS client is available
	// 3. AWS moderation is not globally disabled
	// 4. Specific service is not disabled
	useComprehend := config.EnableTextAnalysis &&
		comprehendClient != nil &&
		!awsModDisabled &&
		!comprehendDisabled

	useRekognition := config.EnableImageAnalysis &&
		rekognitionClient != nil &&
		!awsModDisabled &&
		!rekognitionDisabled

	if useComprehend {
		logger.Info("using AWS Comprehend for text analysis")
		textAnalyzer = NewTextAnalyzer(comprehendClient, logger, config, costTracker)
	} else {
		logger.Info("using no-op text analyzer",
			zap.Bool("config_enabled", config.EnableTextAnalysis),
			zap.Bool("client_available", comprehendClient != nil),
			zap.Bool("aws_disabled", awsModDisabled),
			zap.Bool("comprehend_disabled", comprehendDisabled))
		textAnalyzer = NewNoOpTextAnalyzer(logger, config)
	}

	if useRekognition {
		logger.Info("using AWS Rekognition for image analysis")
		imageAnalyzer = NewImageAnalyzer(rekognitionClient, logger, config, costTracker)
	} else {
		logger.Info("using no-op image analyzer",
			zap.Bool("config_enabled", config.EnableImageAnalysis),
			zap.Bool("client_available", rekognitionClient != nil),
			zap.Bool("aws_disabled", awsModDisabled),
			zap.Bool("rekognition_disabled", rekognitionDisabled))
		imageAnalyzer = NewNoOpImageAnalyzer(logger, config)
	}
	patternMatcher := NewPatternMatcher(patternRepo, logger)
	reputationScorer := NewReputationScorer(dynamoRM, logger, config)

	// Create threat intelligence repository and component
	threatRepo := repositories.NewThreatIntelRepository(dynamoRM, tableName, logger, nil)
	threatIntel := NewThreatIntelligence(threatRepo, logger)

	decisionEngine := NewDecisionEngine(config, logger, reputationScorer)

	// Create moderation metrics repository and component
	metricsRepo := repositories.NewModerationMetricsRepository(dynamoRM, logger)
	metrics := NewModerationMetrics(metricsRepo, logger)

	return &Engine{
		config:           config,
		logger:           logger,
		textAnalyzer:     textAnalyzer,
		imageAnalyzer:    imageAnalyzer,
		patternMatcher:   patternMatcher,
		reputationScorer: reputationScorer,
		decisionEngine:   decisionEngine,
		threatIntel:      threatIntel,
		dynamoRM:         dynamoRM,
		tableName:        tableName,
		metrics:          metrics,
		costTracker:      costTracker,
	}
}

// AnalyzeContent analyzes text content
func (e *Engine) AnalyzeContent(content string, metadata ContentMetadata) (*ContentAnalysis, error) {
	ctx := context.Background()

	// Check rate limits
	if err := e.checkRateLimits(ctx, metadata.AuthorID); err != nil {
		return nil, err
	}

	// Start timer for metrics
	startTime := time.Now()

	// Perform text analysis
	analysis, err := e.textAnalyzer.AnalyzeText(ctx, content, metadata)
	if err != nil {
		e.logger.Error("text analysis failed",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
		return nil, fmt.Errorf("text analysis: %w", err)
	}

	// Check patterns
	patternMatches, err := e.patternMatcher.MatchContent(ctx, content, metadata)
	if err != nil {
		e.logger.Warn("pattern matching failed",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
	}

	// Check threat intelligence
	threatMatches, err := e.threatIntel.CheckContent(ctx, content, metadata)
	if err != nil {
		e.logger.Warn("threat intelligence check failed",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
	}

	// Get reputation score
	reputation, err := e.reputationScorer.GetReputationScore(ctx, metadata.AuthorID)
	if err != nil {
		e.logger.Warn("failed to get reputation score",
			zap.String("authorID", metadata.AuthorID),
			zap.Error(err))
	}

	// Make moderation decision
	moderationAnalysis := &ModerationAnalysis{
		ContentMetadata: metadata,
		TextAnalysis:    analysis,
		PatternMatches:  patternMatches,
		ReputationScore: reputation,
		ThreatMatches:   threatMatches,
	}

	decision, err := e.decisionEngine.MakeDecision(ctx, moderationAnalysis)
	if err != nil {
		e.logger.Error("decision making failed",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
		return nil, fmt.Errorf("make decision: %w", err)
	}

	// Execute decision
	if err := e.executeDecision(ctx, decision, metadata); err != nil {
		e.logger.Error("failed to execute decision",
			zap.String("contentID", metadata.ContentID),
			zap.String("decision", string(decision.Decision)),
			zap.Error(err))
	}

	// Update metrics
	e.metrics.RecordAnalysis(ctx, "text", time.Since(startTime), decision)

	// Store analysis result
	if err := e.storeAnalysisResult(ctx, moderationAnalysis, decision); err != nil {
		e.logger.Warn("failed to store analysis result",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
	}

	return analysis, nil
}

// AnalyzeImage analyzes image content
func (e *Engine) AnalyzeImage(imageURL string, metadata ContentMetadata) (*ImageAnalysis, error) {
	ctx := context.Background()

	// Check rate limits
	if err := e.checkRateLimits(ctx, metadata.AuthorID); err != nil {
		return nil, err
	}

	// Start timer for metrics
	startTime := time.Now()

	// Perform image analysis
	analysis, err := e.imageAnalyzer.AnalyzeImage(ctx, imageURL, metadata)
	if err != nil {
		e.logger.Error("image analysis failed",
			zap.String("contentID", metadata.ContentID),
			zap.String("imageURL", imageURL),
			zap.Error(err))
		return nil, fmt.Errorf("image analysis: %w", err)
	}

	// Check for text in image against patterns
	var textContent string
	for _, text := range analysis.Text {
		textContent += text.Text + " "
	}

	patternMatches, _ := e.patternMatcher.MatchContent(ctx, textContent, metadata)

	// Get reputation score
	reputation, _ := e.reputationScorer.GetReputationScore(ctx, metadata.AuthorID)

	// Make moderation decision
	moderationAnalysis := &ModerationAnalysis{
		ContentMetadata: metadata,
		ImageAnalysis:   analysis,
		PatternMatches:  patternMatches,
		ReputationScore: reputation,
	}

	decision, err := e.decisionEngine.MakeDecision(ctx, moderationAnalysis)
	if err != nil {
		e.logger.Error("decision making failed",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
		return nil, fmt.Errorf("make decision: %w", err)
	}

	// Execute decision
	if err := e.executeDecision(ctx, decision, metadata); err != nil {
		e.logger.Error("failed to execute decision",
			zap.String("contentID", metadata.ContentID),
			zap.String("decision", string(decision.Decision)),
			zap.Error(err))
	}

	// Update metrics
	e.metrics.RecordAnalysis(ctx, "image", time.Since(startTime), decision)

	// Store analysis result
	if err := e.storeAnalysisResult(ctx, moderationAnalysis, decision); err != nil {
		e.logger.Warn("failed to store analysis result",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
	}

	return analysis, nil
}

// AnalyzeVideo analyzes video content using AWS Rekognition Video
func (e *Engine) AnalyzeVideo(videoURL string, metadata ContentMetadata) (*VideoAnalysis, error) {
	ctx := context.Background()

	// Check rate limits
	if err := e.checkRateLimits(ctx, metadata.AuthorID); err != nil {
		return nil, err
	}

	startTime := time.Now()

	// Initialize video analyzer if needed
	// Check if we should use AWS-based video analysis
	var videoAnalyzer VideoAnalyzerInterface
	if globalCfg := getGlobalConfig(); globalCfg != nil &&
		(globalCfg.GetDisableAWSModeration() || globalCfg.GetDisableRekognition()) {
		// Use no-op video analyzer when AWS is disabled
		e.logger.Info("using no-op video analyzer (AWS disabled by feature flags)")
		videoAnalyzer = NewNoOpVideoAnalyzer(e.logger, e.config)
	} else if awsAnalyzer, ok := e.imageAnalyzer.(*ImageAnalyzer); ok && awsAnalyzer.GetClient() != nil {
		// Use AWS video analyzer when image analyzer is AWS-based
		e.logger.Info("using AWS video analyzer")
		videoAnalyzer = NewVideoAnalyzer(awsAnalyzer.GetClient(), e.logger, e.config, e.costTracker)
	} else {
		// Fallback to no-op analyzer if image analyzer is no-op
		e.logger.Info("using no-op video analyzer (image analyzer is no-op)")
		videoAnalyzer = NewNoOpVideoAnalyzer(e.logger, e.config)
	}

	// Perform video analysis
	analysis, err := videoAnalyzer.AnalyzeVideo(ctx, videoURL, metadata)
	if err != nil {
		e.logger.Error("video analysis failed",
			zap.String("contentID", metadata.ContentID),
			zap.String("videoURL", videoURL),
			zap.Error(err))
		return nil, fmt.Errorf("video analysis: %w", err)
	}

	// Analyze transcribed audio text if available
	var textContent string
	if analysis.Audio.Transcription != "" {
		textContent = analysis.Audio.Transcription
	}

	// Extract text from video frames
	for _, frame := range analysis.Frames {
		for _, text := range frame.ImageAnalysis.Text {
			textContent += text.Text + " "
		}
	}

	// Check patterns against extracted text
	var patternMatches []PatternMatch
	if textContent != "" {
		matches, _ := e.patternMatcher.MatchContent(ctx, textContent, metadata)
		patternMatches = matches
	}

	// Check threat intelligence
	threatMatches, _ := e.threatIntel.CheckContent(ctx, textContent, metadata)

	// Get reputation score
	reputation, _ := e.reputationScorer.GetReputationScore(ctx, metadata.AuthorID)

	// Make moderation decision
	moderationAnalysis := &ModerationAnalysis{
		ContentMetadata: metadata,
		VideoAnalysis:   analysis,
		PatternMatches:  patternMatches,
		ReputationScore: reputation,
		ThreatMatches:   threatMatches,
	}

	decision, err := e.decisionEngine.MakeDecision(ctx, moderationAnalysis)
	if err != nil {
		e.logger.Error("decision making failed",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
		return nil, fmt.Errorf("make decision: %w", err)
	}

	// Execute decision
	if err := e.executeDecision(ctx, decision, metadata); err != nil {
		e.logger.Error("failed to execute decision",
			zap.String("contentID", metadata.ContentID),
			zap.String("decision", string(decision.Decision)),
			zap.Error(err))
	}

	// Update metrics
	e.metrics.RecordAnalysis(ctx, "video", time.Since(startTime), decision)

	// Store analysis result
	if err := e.storeAnalysisResult(ctx, moderationAnalysis, decision); err != nil {
		e.logger.Warn("failed to store analysis result",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
	}

	return analysis, nil
}

// Pattern management methods

// CreatePattern creates a new moderation pattern
func (e *Engine) CreatePattern(pattern *ModerationPattern) error {
	ctx := context.Background()
	return e.patternMatcher.CreatePattern(ctx, pattern)
}

// UpdatePattern updates an existing pattern
func (e *Engine) UpdatePattern(patternID string, pattern *ModerationPattern) error {
	ctx := context.Background()
	return e.patternMatcher.UpdatePattern(ctx, patternID, pattern)
}

// DeletePattern deletes a pattern
func (e *Engine) DeletePattern(patternID string) error {
	ctx := context.Background()
	return e.patternMatcher.DeletePattern(ctx, patternID)
}

// GetPatterns retrieves patterns based on filter
func (e *Engine) GetPatterns(filter PatternFilter) ([]*ModerationPattern, error) {
	ctx := context.Background()
	return e.patternMatcher.GetPatterns(ctx, filter)
}

// Reputation management methods

// GetReputationScore gets a user's reputation score
func (e *Engine) GetReputationScore(actorID string) (*ReputationScore, error) {
	ctx := context.Background()
	return e.reputationScorer.GetReputationScore(ctx, actorID)
}

// UpdateReputation updates a user's reputation
func (e *Engine) UpdateReputation(actorID string, event ReputationEvent) error {
	ctx := context.Background()
	return e.reputationScorer.UpdateReputation(ctx, actorID, event)
}

// Threat intelligence methods

// ShareThreat shares threat intelligence
func (e *Engine) ShareThreat(threat *ThreatIntel) error {
	ctx := context.Background()
	return e.threatIntel.ShareThreat(ctx, threat)
}

// GetSharedThreats retrieves shared threats
func (e *Engine) GetSharedThreats(since time.Time) ([]*ThreatIntel, error) {
	ctx := context.Background()
	return e.threatIntel.GetSharedThreats(ctx, since)
}

// Decision making

// MakeDecision makes a moderation decision based on analysis
func (e *Engine) MakeDecision(analysis *ModerationAnalysis) (*ModerationDecision, error) {
	ctx := context.Background()
	return e.decisionEngine.MakeDecision(ctx, analysis)
}

// Reporting methods

// GetModerationStats gets moderation statistics
func (e *Engine) GetModerationStats(timeRange TimeRange) (*ModerationStats, error) {
	ctx := context.Background()
	return e.metrics.GetStats(ctx, timeRange)
}

// GetFalsePositiveRate calculates the false positive rate
func (e *Engine) GetFalsePositiveRate(timeRange TimeRange) (float64, error) {
	ctx := context.Background()
	stats, err := e.metrics.GetStats(ctx, timeRange)
	if err != nil {
		return 0, err
	}

	total := stats.TruePositives + stats.FalsePositives
	if total == 0 {
		return 0, nil
	}

	return float64(stats.FalsePositives) / float64(total), nil
}

// Helper methods

func (e *Engine) checkRateLimits(_ context.Context, _ string) error {
	// Simple rate limiting - in production, use a proper rate limiter
	// Check if user is making too many requests
	return nil
}

func (e *Engine) executeDecision(ctx context.Context, decision *ModerationDecision, metadata ContentMetadata) error {
	// Update reputation based on decision
	if decision.Decision != ActionAllow {
		event := ReputationEvent{
			EventType:   "violation",
			Severity:    e.getHighestSeverity(decision.Reasons),
			Description: fmt.Sprintf("Content moderated: %s", decision.Decision),
			Timestamp:   time.Now(),
		}

		if err := e.reputationScorer.UpdateReputation(ctx, metadata.AuthorID, event); err != nil {
			e.logger.Warn("failed to update reputation",
				zap.String("authorID", metadata.AuthorID),
				zap.Error(err))
		}
	}

	// Store decision for audit trail
	if err := e.storeDecision(ctx, decision, metadata); err != nil {
		return fmt.Errorf("store decision: %w", err)
	}

	// Send to review queue if needed
	if decision.RequiresReview {
		if err := e.sendToReviewQueue(ctx, decision, metadata); err != nil {
			e.logger.Warn("failed to send to review queue",
				zap.String("contentID", metadata.ContentID),
				zap.Error(err))
		}
	}

	return nil
}

func (e *Engine) storeAnalysisResult(ctx context.Context, analysis *ModerationAnalysis, decision *ModerationDecision) error {
	// Create moderation repository using DynamORM
	moderationRepo := repositories.NewModerationRepository(e.dynamoRM, e.tableName, e.logger)

	// Prepare analysis result data for storage
	analysisData := map[string]interface{}{
		"content_id":   analysis.ContentMetadata.ContentID,
		"author_id":    analysis.ContentMetadata.AuthorID,
		"content_type": string(analysis.ContentMetadata.ContentType),
		"confidence":   decision.Confidence,
	}

	// Determine analysis type
	analysisType := "combined"
	if analysis.TextAnalysis != nil && analysis.ImageAnalysis == nil && analysis.VideoAnalysis == nil {
		analysisType = "text"
	} else if analysis.ImageAnalysis != nil && analysis.TextAnalysis == nil && analysis.VideoAnalysis == nil {
		analysisType = "image"
	} else if analysis.VideoAnalysis != nil && analysis.TextAnalysis == nil && analysis.ImageAnalysis == nil {
		analysisType = "video"
	}
	analysisData["analysis_type"] = analysisType

	// Store full analysis results
	results := make(map[string]interface{})
	if analysis.TextAnalysis != nil {
		results["text_analysis"] = map[string]interface{}{
			"sentiment":   analysis.TextAnalysis.Sentiment,
			"toxicity":    analysis.TextAnalysis.Toxicity,
			"pii":         analysis.TextAnalysis.PII,
			"topics":      analysis.TextAnalysis.Topics,
			"language":    analysis.TextAnalysis.Language,
			"threats":     analysis.TextAnalysis.Threats,
			"analyzed_at": analysis.TextAnalysis.AnalyzedAt,
		}
	}

	if analysis.ImageAnalysis != nil {
		results["image_analysis"] = map[string]interface{}{
			"explicit":    analysis.ImageAnalysis.Explicit,
			"violence":    analysis.ImageAnalysis.Violence,
			"text":        analysis.ImageAnalysis.Text,
			"objects":     analysis.ImageAnalysis.Objects,
			"faces":       analysis.ImageAnalysis.Faces,
			"analyzed_at": analysis.ImageAnalysis.AnalyzedAt,
		}
	}

	if analysis.VideoAnalysis != nil {
		results["video_analysis"] = map[string]interface{}{
			"frames":      analysis.VideoAnalysis.Frames,
			"audio":       analysis.VideoAnalysis.Audio,
			"duration":    analysis.VideoAnalysis.Duration,
			"analyzed_at": analysis.VideoAnalysis.AnalyzedAt,
		}
	}

	analysisData["results"] = results

	// Store pattern matches
	if err := common.ValidateSliceNotEmpty("analysis.PatternMatches", analysis.PatternMatches); err == nil {
		patternMatches := make([]interface{}, len(analysis.PatternMatches))
		for i, match := range analysis.PatternMatches {
			patternMatches[i] = map[string]interface{}{
				"pattern_id":   match.PatternID,
				"pattern_name": match.PatternName,
				"match_text":   match.MatchText,
				"location":     match.Location,
				"confidence":   match.Confidence,
			}
		}
		analysisData["pattern_matches"] = patternMatches
	}

	// Store threat matches
	if err := common.ValidateSliceNotEmpty("analysis.ThreatMatches", analysis.ThreatMatches); err == nil {
		threatMatches := make([]interface{}, len(analysis.ThreatMatches))
		for i, match := range analysis.ThreatMatches {
			threatMatches[i] = map[string]interface{}{
				"threat_id":   match.ThreatID,
				"threat_type": match.ThreatType,
				"indicator":   match.Indicator,
				"confidence":  match.Confidence,
			}
		}
		analysisData["threat_matches"] = threatMatches
	}

	// Store reputation score
	if analysis.ReputationScore != nil {
		analysisData["reputation_score"] = map[string]interface{}{
			"actor_id":             analysis.ReputationScore.ActorID,
			"score":                analysis.ReputationScore.Score,
			"level":                analysis.ReputationScore.Level,
			"violation_count":      analysis.ReputationScore.ViolationCount,
			"false_positive_count": analysis.ReputationScore.FalsePositiveCount,
			"content_count":        analysis.ReputationScore.ContentCount,
			"last_violation":       analysis.ReputationScore.LastViolation,
			"factors":              analysis.ReputationScore.Factors,
			"updated_at":           analysis.ReputationScore.UpdatedAt,
		}
	}

	// Store analysis result
	if err := moderationRepo.StoreAnalysisResult(ctx, analysisData); err != nil {
		e.logger.Warn("Failed to store analysis result",
			zap.String("contentID", analysis.ContentMetadata.ContentID),
			zap.Error(err))
		return fmt.Errorf("store analysis result: %w", err)
	}

	return nil
}

func (e *Engine) storeDecision(ctx context.Context, decision *ModerationDecision, metadata ContentMetadata) error {
	// Create moderation repository using DynamORM
	moderationRepo := repositories.NewModerationRepository(e.dynamoRM, e.tableName, e.logger)

	// Prepare decision data for storage
	decisionData := map[string]interface{}{
		"content_id":      metadata.ContentID,
		"author_id":       metadata.AuthorID,
		"action":          string(decision.Decision),
		"confidence":      decision.Confidence,
		"requires_review": decision.RequiresReview,
		"review_priority": decision.ReviewPriority,
		"recommendations": decision.Recommendations,
	}

	// Add expiration time if set
	if !decision.ExpiresAt.IsZero() {
		decisionData["expires_at"] = decision.ExpiresAt
	}

	// Convert reasons to storable format
	if err := common.ValidateSliceNotEmpty("decision.Reasons", decision.Reasons); err == nil {
		reasons := make([]interface{}, len(decision.Reasons))
		for i, reason := range decision.Reasons {
			reasonMap := map[string]interface{}{
				"type":        reason.Type,
				"severity":    string(reason.Severity),
				"description": reason.Description,
			}

			// Store evidence (converted to map if needed)
			if reason.Evidence != nil {
				reasonMap["evidence"] = reason.Evidence
			}

			reasons[i] = reasonMap
		}
		decisionData["reasons"] = reasons
	}

	// Add metadata with decision context
	decisionMetadata := map[string]interface{}{
		"decided_at":    decision.DecidedAt,
		"content_type":  string(metadata.ContentType),
		"author_domain": metadata.AuthorDomain,
		"language":      metadata.Language,
		"context":       metadata.Context,
	}

	// Add mentions, hashtags, URLs if present
	if err := common.ValidateSliceNotEmpty("metadata.Mentions", metadata.Mentions); err == nil {
		decisionMetadata["mentions"] = metadata.Mentions
	}
	if err := common.ValidateSliceNotEmpty("metadata.Hashtags", metadata.Hashtags); err == nil {
		decisionMetadata["hashtags"] = metadata.Hashtags
	}
	if err := common.ValidateSliceNotEmpty("metadata.URLs", metadata.URLs); err == nil {
		decisionMetadata["urls"] = metadata.URLs
	}

	decisionData["metadata"] = decisionMetadata

	// Store decision
	if err := moderationRepo.StoreDecision(ctx, decisionData); err != nil {
		e.logger.Warn("Failed to store decision",
			zap.String("contentID", metadata.ContentID),
			zap.String("action", string(decision.Decision)),
			zap.Error(err))
		return fmt.Errorf("store decision: %w", err)
	}

	const (
		statusApplied = "applied"
		statusPending = "pending"
	)

	// Update enforcement status based on action
	var enforcementStatus string
	switch decision.Decision {
	case ActionAllow:
		enforcementStatus = statusApplied // No enforcement needed
	case ActionFlag:
		enforcementStatus = statusPending // Needs human review
	case ActionQuarantine, ActionRemove, ActionShadowBan:
		enforcementStatus = statusPending // Needs system enforcement
	default:
		enforcementStatus = statusPending
	}

	// Update enforcement status if not "allow"
	if decision.Decision != ActionAllow {
		if err := moderationRepo.UpdateEnforcementStatus(ctx, metadata.ContentID, enforcementStatus); err != nil {
			e.logger.Warn("Failed to update enforcement status",
				zap.String("contentID", metadata.ContentID),
				zap.String("status", enforcementStatus),
				zap.Error(err))
		}
	}

	return nil
}

func (e *Engine) sendToReviewQueue(ctx context.Context, decision *ModerationDecision, metadata ContentMetadata) error {

	// Create queue item for the decision
	queueItem := &models.ModerationReviewQueue{
		ID:        fmt.Sprintf("queue_%d", time.Now().UnixNano()),
		ContentID: metadata.ContentID,
		AuthorID:  metadata.AuthorID,
		Status:    "pending",
		Priority:  decision.ReviewPriority,
		Category:  "moderation",
		Reason:    fmt.Sprintf("Action: %s (requires review)", decision.Decision),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set severity based on decision action
	switch decision.Decision {
	case ActionRemove, ActionShadowBan:
		queueItem.Severity = "high"
	case ActionQuarantine:
		queueItem.Severity = "medium"
	case ActionFlag:
		queueItem.Severity = "low"
	default:
		queueItem.Severity = "medium"
	}

	// Set evidence from decision reasons
	if err := common.ValidateSliceNotEmpty("decision.Reasons", decision.Reasons); err == nil {
		evidence := map[string]interface{}{
			"reasons":         decision.Reasons,
			"recommendations": decision.Recommendations,
			"confidence":      decision.Confidence,
		}
		queueItem.Evidence = evidence
	}

	// Set deadline based on priority
	switch {
	case decision.ReviewPriority >= 8:
		deadline := time.Now().Add(1 * time.Hour)
		queueItem.Deadline = &deadline
	case decision.ReviewPriority >= 5:
		deadline := time.Now().Add(24 * time.Hour)
		queueItem.Deadline = &deadline
	default:
		deadline := time.Now().Add(7 * 24 * time.Hour)
		queueItem.Deadline = &deadline
	}

	// Update keys and create
	queueItem.UpdateKeys()

	if err := e.dynamoRM.WithContext(ctx).Model(queueItem).Create(); err != nil {
		e.logger.Error("Failed to send content to review queue",
			zap.String("contentID", metadata.ContentID),
			zap.Error(err))
		return fmt.Errorf("failed to send to review queue: %w", err)
	}

	e.logger.Info("Content sent to review queue",
		zap.String("contentID", metadata.ContentID),
		zap.String("queue_id", queueItem.ID),
		zap.Int("priority", decision.ReviewPriority),
		zap.String("severity", queueItem.Severity))

	return nil
}

func (e *Engine) getHighestSeverity(reasons []DecisionReason) Severity {
	highest := SeverityLow
	severityOrder := map[Severity]int{
		SeverityCritical: 4,
		SeverityHigh:     3,
		SeverityMedium:   2,
		SeverityLow:      1,
	}

	for _, reason := range reasons {
		if severityOrder[reason.Severity] > severityOrder[highest] {
			highest = reason.Severity
		}
	}

	return highest
}

// AnalyzeContentBatch analyzes multiple pieces of content in parallel
func (e *Engine) AnalyzeContentBatch(contents []struct {
	Content  string
	Metadata ContentMetadata
}) ([]*ContentAnalysis, error) {
	var wg sync.WaitGroup
	analyses := make([]*ContentAnalysis, len(contents))
	errors := make([]error, len(contents))

	for i, content := range contents {
		wg.Add(1)
		go func(idx int, c string, m ContentMetadata) {
			defer wg.Done()
			analysis, err := e.AnalyzeContent(c, m)
			analyses[idx] = analysis
			errors[idx] = err
		}(i, content.Content, content.Metadata)
	}

	wg.Wait()

	// Check for errors
	var firstError error
	for _, err := range errors {
		if err != nil && firstError == nil {
			firstError = err
		}
	}

	return analyses, firstError
}

// VideoAnalyzer handles video content analysis using AWS Rekognition Video
type VideoAnalyzer struct {
	client      *rekognition.Client
	logger      *zap.Logger
	config      *ModerationConfig
	costTracker CostTracker

	// Image analyzer for frame analysis
	imageAnalyzer *ImageAnalyzer

	// S3 client for frame uploads
	s3Client   *s3.Client
	s3Transfer *transfermanager.Client
	bucketName string

	// Cache for results
	resultCache sync.Map
	cacheTTL    time.Duration
}

// NewVideoAnalyzer creates a new video analyzer
func NewVideoAnalyzer(client *rekognition.Client, logger *zap.Logger, config *ModerationConfig, costTracker CostTracker) *VideoAnalyzer {
	// Initialize S3 client for frame uploads
	s3Client := s3.NewFromConfig(aws.Config{})
	transferClient := transfermanager.New(s3Client)

	// Get bucket name from centralized config
	globalCfg := appconfig.Get()
	bucketName := globalCfg.MediaBucketName
	if bucketName == "" {
		bucketName = globalCfg.S3BucketName // fallback to main S3 bucket
	}
	if bucketName == "" {
		bucketName = "lesser-media" // final fallback
	}

	return &VideoAnalyzer{
		client:        client,
		logger:        logger,
		config:        config,
		costTracker:   costTracker,
		imageAnalyzer: NewImageAnalyzer(client, logger, config, costTracker),
		s3Client:      s3Client,
		s3Transfer:    transferClient,
		bucketName:    bucketName,
		cacheTTL:      30 * time.Minute, // Longer cache for videos due to processing cost
	}
}

// extractFrameAtTimestamp extracts a single frame from video at the specified timestamp
func (va *VideoAnalyzer) extractFrameAtTimestamp(ctx context.Context, video *rekognitionTypes.Video, timestamp time.Duration) (string, error) {
	if video.S3Object == nil {
		return "", fmt.Errorf("S3 object information required for frame extraction")
	}

	bucket := aws.ToString(video.S3Object.Bucket)
	key := aws.ToString(video.S3Object.Name)

	va.logger.Debug("extracting frame from video",
		zap.String("bucket", bucket),
		zap.String("key", key),
		zap.Duration("timestamp", timestamp))

	// Download video file from S3 to temporary location
	tempVideoPath, err := va.downloadVideoFromS3(ctx, bucket, key)
	if err != nil {
		return "", fmt.Errorf("failed to download video from S3: %w", err)
	}
	defer va.cleanupLocalFile(tempVideoPath)

	// Extract frame using FFmpeg
	frameImagePath, err := va.extractFrameWithFFmpeg(tempVideoPath, timestamp)
	if err != nil {
		return "", fmt.Errorf("failed to extract frame with FFmpeg: %w", err)
	}
	defer va.cleanupLocalFile(frameImagePath)

	// Upload extracted frame to S3 for analysis
	frameS3URL, err := va.uploadFrameToS3(ctx, frameImagePath)
	if err != nil {
		return "", fmt.Errorf("failed to upload frame to S3: %w", err)
	}

	va.logger.Debug("successfully extracted and uploaded frame",
		zap.String("frame_s3_url", frameS3URL),
		zap.Duration("timestamp", timestamp))

	return frameS3URL, nil
}

// downloadVideoFromS3 downloads video file from S3 to local temporary file
func (va *VideoAnalyzer) downloadVideoFromS3(ctx context.Context, bucket, key string) (string, error) {
	// Create temporary file for video
	tempFile, err := va.createTempFile("video_", ".mp4")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			va.logger.Error("Failed to close temp file", zap.Error(err))
		}
	}()

	// Download from S3
	_, err = va.s3Transfer.DownloadObject(ctx, &transfermanager.DownloadObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(key),
		WriterAt:     tempFile,
		ChecksumMode: tmtypes.ChecksumModeEnabled,
	})
	if err != nil {
		va.cleanupLocalFile(tempFile.Name())
		return "", fmt.Errorf("failed to download from S3: %w", err)
	}

	return tempFile.Name(), nil
}

// extractFrameWithFFmpeg uses FFmpeg to extract a frame at the specified timestamp
func (va *VideoAnalyzer) extractFrameWithFFmpeg(videoPath string, timestamp time.Duration) (string, error) {
	// Create temporary file for extracted frame
	frameFile, err := va.createTempFile("frame_", ".jpg")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for frame: %w", err)
	}
	if err := frameFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close frame file: %w", err)
	} // Close file handle so ffmpeg can write to it

	// Convert timestamp to FFmpeg format (HH:MM:SS.mmm)
	timestampStr := fmt.Sprintf("%.3f", timestamp.Seconds())

	// Run FFmpeg command to extract frame
	// #nosec G204 - ffmpeg command with validated input paths for video processing
	cmd := exec.Command("ffmpeg",
		"-i", videoPath, // Input video file
		"-ss", timestampStr, // Seek to timestamp
		"-vframes", "1", // Extract only 1 frame
		"-q:v", "2", // High quality
		"-y",             // Overwrite output file
		frameFile.Name(), // Output frame file
	)

	// Capture FFmpeg output for debugging
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		va.cleanupLocalFile(frameFile.Name())
		return "", fmt.Errorf("FFmpeg command failed: %w, stderr: %s", err, stderr.String())
	}

	// Verify frame file was created and has content
	if fileInfo, err := os.Stat(frameFile.Name()); err != nil || fileInfo.Size() == 0 {
		va.cleanupLocalFile(frameFile.Name())
		return "", fmt.Errorf("extracted frame file is empty or missing")
	}

	va.logger.Debug("FFmpeg frame extraction completed",
		zap.String("frame_path", frameFile.Name()),
		zap.Duration("timestamp", timestamp))

	return frameFile.Name(), nil
}

// uploadFrameToS3 uploads extracted frame image to S3 and returns the URL
func (va *VideoAnalyzer) uploadFrameToS3(ctx context.Context, framePath string) (string, error) {
	// Open frame file
	// #nosec G304 - framePath is from controlled temp file creation
	frameFile, err := os.Open(framePath)
	if err != nil {
		return "", fmt.Errorf("failed to open frame file: %w", err)
	}
	defer func() {
		if err := frameFile.Close(); err != nil {
			va.logger.Error("Failed to close frame file", zap.Error(err))
		}
	}()

	// Generate unique S3 key for frame
	frameKey, err := va.generateFrameS3Key()
	if err != nil {
		return "", fmt.Errorf("failed to generate S3 key: %w", err)
	}

	// Upload to S3
	_, err = va.s3Transfer.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket:      aws.String(va.bucketName),
		Key:         aws.String(frameKey),
		Body:        frameFile,
		ContentType: aws.String("image/jpeg"),
		Metadata: map[string]string{
			"purpose":    "moderation-frame",
			"created-at": time.Now().Format(time.RFC3339),
			"ttl":        fmt.Sprintf("%d", time.Now().Add(1*time.Hour).Unix()), // 1 hour TTL
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload frame to S3: %w", err)
	}

	// Construct S3 URL
	frameURL := fmt.Sprintf("s3://%s/%s", va.bucketName, frameKey)

	va.logger.Debug("uploaded frame to S3",
		zap.String("s3_key", frameKey),
		zap.String("s3_url", frameURL))

	return frameURL, nil
}

// cleanupTemporaryFrame removes the temporary frame from S3
func (va *VideoAnalyzer) cleanupTemporaryFrame(ctx context.Context, frameURL string) {
	if !isS3URL(frameURL) {
		va.logger.Warn("cannot cleanup non-S3 frame URL", zap.String("url", frameURL))
		return
	}

	bucket, key, err := parseS3URL(frameURL)
	if err != nil {
		va.logger.Warn("failed to parse S3 URL for cleanup", zap.String("url", frameURL), zap.Error(err))
		return
	}

	_, err = va.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		va.logger.Warn("failed to cleanup temporary frame",
			zap.String("s3_url", frameURL),
			zap.Error(err))
	} else {
		va.logger.Debug("cleaned up temporary frame", zap.String("s3_url", frameURL))
	}
}

// Helper methods

// createTempFile creates a temporary file with the given prefix and suffix
func (va *VideoAnalyzer) createTempFile(prefix, suffix string) (*os.File, error) {
	tempDir := os.TempDir()
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	fileName := fmt.Sprintf("%s%s%s", prefix, hex.EncodeToString(randomBytes), suffix)
	filePath := filepath.Join(tempDir, fileName)

	// #nosec G304 - filePath is controlled by createTempFile method
	return os.Create(filePath)
}

// cleanupLocalFile removes a local temporary file
func (va *VideoAnalyzer) cleanupLocalFile(filePath string) {
	if err := os.Remove(filePath); err != nil {
		va.logger.Warn("failed to cleanup local file", zap.String("path", filePath), zap.Error(err))
	} else {
		va.logger.Debug("cleaned up local file", zap.String("path", filePath))
	}
}

// generateFrameS3Key generates a unique S3 key for extracted frames
func (va *VideoAnalyzer) generateFrameS3Key() (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	timestamp := time.Now().Format("2006/01/02")
	frameID := hex.EncodeToString(randomBytes)

	return fmt.Sprintf("moderation/frames/%s/%s.jpg", timestamp, frameID), nil
}

// parseS3URL parses an S3 URL and returns bucket and key
func parseS3URL(s3URL string) (bucket, key string, err error) {
	if !strings.HasPrefix(s3URL, "s3://") {
		return "", "", fmt.Errorf("invalid S3 URL format: %s", s3URL)
	}

	// Remove s3:// prefix
	path := s3URL[5:]

	// Split into bucket and key
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid S3 URL format: %s", s3URL)
	}

	return parts[0], parts[1], nil
}

// AnalyzeVideo performs comprehensive video analysis with frame sampling and audio processing
func (va *VideoAnalyzer) AnalyzeVideo(ctx context.Context, videoURL string, _ ContentMetadata) (*VideoAnalysis, error) {
	startTime := time.Now()

	// Check cache first
	cacheKey := fmt.Sprintf("video:%s", videoURL)
	if cached, ok := va.resultCache.Load(cacheKey); ok {
		if result, ok := cached.(*cachedVideoResult); ok && time.Since(result.cachedAt) < va.cacheTTL {
			va.logger.Debug("returning cached video analysis", zap.String("videoURL", videoURL))
			return result.analysis, nil
		}
	}

	// Create video input for S3-hosted video
	if !isS3URL(videoURL) {
		return nil, fmt.Errorf("non-S3 video URLs not supported - video must be stored in S3")
	}

	s3Object := &rekognitionTypes.Video{
		S3Object: &rekognitionTypes.S3Object{
			Bucket: aws.String(va.config.S3Bucket),
			Name:   aws.String(extractS3Key(videoURL)),
		},
	}

	analysis := &VideoAnalysis{
		VideoURL:   videoURL,
		Frames:     []FrameAnalysis{},
		AnalyzedAt: time.Now(),
	}

	// Get video metadata first to determine sampling strategy
	duration, err := va.getVideoDuration(ctx, s3Object)
	if err != nil {
		va.logger.Warn("could not determine video duration, using default sampling",
			zap.String("videoURL", videoURL),
			zap.Error(err))
		duration = 60 * time.Second // Default assumption
	}
	analysis.Duration = duration

	// Determine frame sampling strategy based on video length and cost constraints
	sampleIntervals := va.calculateFrameSamplingStrategy(duration)

	// Start video analysis jobs in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Content moderation detection (async job)
	wg.Add(1)
	go func() {
		defer wg.Done()
		moderationResults, err := va.startContentModerationDetection(ctx, s3Object)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("content moderation: %w", err))
			mu.Unlock()
			return
		}
		va.processModerationResults(moderationResults, analysis)
	}()

	// Text detection (async job)
	wg.Add(1)
	go func() {
		defer wg.Done()
		textResults, err := va.startTextDetection(ctx, s3Object)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("text detection: %w", err))
			mu.Unlock()
			return
		}
		va.processTextResults(textResults, analysis)
	}()

	// Face detection (async job)
	wg.Add(1)
	go func() {
		defer wg.Done()
		faceResults, err := va.startFaceDetection(ctx, s3Object)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("face detection: %w", err))
			mu.Unlock()
			return
		}
		va.processFaceResults(faceResults, analysis)
	}()

	// Label detection (async job)
	wg.Add(1)
	go func() {
		defer wg.Done()
		labelResults, err := va.startLabelDetection(ctx, s3Object)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("label detection: %w", err))
			mu.Unlock()
			return
		}
		va.processLabelResults(labelResults, analysis)
	}()

	// Sample key frames for detailed analysis
	wg.Add(1)
	go func() {
		defer wg.Done()
		frames, err := va.analyzeKeyFrames(ctx, s3Object, sampleIntervals)
		if err != nil {
			mu.Lock()
			errors = append(errors, fmt.Errorf("frame analysis: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		analysis.Frames = frames
		mu.Unlock()
	}()

	// Audio transcription (if enabled)
	if va.config.EnableTextAnalysis {
		wg.Add(1)
		go func() {
			defer wg.Done()
			audioAnalysis, err := va.transcribeAudio(ctx, s3Object)
			if err != nil {
				va.logger.Warn("audio transcription failed",
					zap.String("videoURL", videoURL),
					zap.Error(err))
				// Not a fatal error, continue without audio analysis
				audioAnalysis = &AudioAnalysis{
					Transcription: "",
					Language:      "unknown",
				}
			}
			mu.Lock()
			analysis.Audio = *audioAnalysis
			mu.Unlock()
		}()
	}

	// Wait for all analysis to complete
	wg.Wait()

	// Check for critical errors
	if err := common.ValidateSliceNotEmpty("errors", errors); err == nil {
		va.logger.Error("video analysis completed with errors",
			zap.String("videoURL", videoURL),
			zap.Errors("errors", errors))
		// Don't fail completely if some analyses succeeded
	}

	analysis.ProcessingTime = time.Since(startTime)

	// Cache the result
	va.resultCache.Store(cacheKey, &cachedVideoResult{
		analysis: analysis,
		cachedAt: time.Now(),
	})

	// Store frame thumbnails in S3 for manual review if needed
	if err := va.storeThumbnails(ctx, analysis); err != nil {
		va.logger.Warn("failed to store frame thumbnails",
			zap.String("videoURL", videoURL),
			zap.Error(err))
	}

	return analysis, nil
}

// calculateFrameSamplingStrategy determines optimal frame sampling based on video duration and cost constraints
func (va *VideoAnalyzer) calculateFrameSamplingStrategy(duration time.Duration) []time.Duration {
	var intervals []time.Duration

	switch {
	case duration <= 30*time.Second:
		// Short videos: sample every 5 seconds
		for i := 0; i < int(duration.Seconds()); i += 5 {
			intervals = append(intervals, time.Duration(i)*time.Second)
		}
	case duration <= 2*time.Minute:
		// Medium videos: sample every 10 seconds
		for i := 0; i < int(duration.Seconds()); i += 10 {
			intervals = append(intervals, time.Duration(i)*time.Second)
		}
	case duration <= 5*time.Minute:
		// Long videos: sample every 15 seconds
		for i := 0; i < int(duration.Seconds()); i += 15 {
			intervals = append(intervals, time.Duration(i)*time.Second)
		}
	default:
		// Very long videos: sample every 30 seconds, max 20 frames
		interval := int(duration.Seconds()) / 20
		if interval < 30 {
			interval = 30
		}
		for i := 0; i < int(duration.Seconds()) && len(intervals) < 20; i += interval {
			intervals = append(intervals, time.Duration(i)*time.Second)
		}
	}

	// Always include first and last frames
	if len(intervals) == 0 || intervals[0] != 0 {
		intervals = append([]time.Duration{0}, intervals...)
	}
	if len(intervals) == 0 || intervals[len(intervals)-1] != duration {
		intervals = append(intervals, duration)
	}

	return intervals
}

// startContentModerationDetection starts asynchronous content moderation detection
func (va *VideoAnalyzer) startContentModerationDetection(ctx context.Context, video *rekognitionTypes.Video) ([]rekognition.GetContentModerationOutput, error) {
	// Start moderation detection job
	startInput := &rekognition.StartContentModerationInput{
		Video:         video,
		MinConfidence: aws.Float32(float32(va.config.ConfidenceThreshold * 100)),
	}

	startResult, err := va.client.StartContentModeration(ctx, startInput)
	if err != nil {
		return nil, fmt.Errorf("start content moderation: %w", err)
	}

	// Track cost
	if va.costTracker != nil {
		if tracker, ok := va.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("StartContentModeration", 1)
		}
	}

	// Wait for job completion with timeout
	jobID := *startResult.JobId
	return va.waitForModerationJob(ctx, jobID, 5*time.Minute)
}

// startTextDetection starts asynchronous text detection in video
func (va *VideoAnalyzer) startTextDetection(ctx context.Context, video *rekognitionTypes.Video) ([]rekognition.GetTextDetectionOutput, error) {
	startInput := &rekognition.StartTextDetectionInput{
		Video: video,
		NotificationChannel: &rekognitionTypes.NotificationChannel{
			SNSTopicArn: aws.String(""), // Optional: configure for async notification
			RoleArn:     aws.String(""), // Optional: IAM role for SNS
		},
	}

	startResult, err := va.client.StartTextDetection(ctx, startInput)
	if err != nil {
		return nil, fmt.Errorf("start text detection: %w", err)
	}

	// Track cost
	if va.costTracker != nil {
		if tracker, ok := va.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("StartTextDetection", 1)
		}
	}

	jobID := *startResult.JobId
	return va.waitForTextDetectionJob(ctx, jobID, 5*time.Minute)
}

// startFaceDetection starts asynchronous face detection in video
func (va *VideoAnalyzer) startFaceDetection(ctx context.Context, video *rekognitionTypes.Video) ([]rekognition.GetFaceDetectionOutput, error) {
	startInput := &rekognition.StartFaceDetectionInput{
		Video:          video,
		FaceAttributes: "ALL", // Detect age, gender, emotions, etc.
	}

	startResult, err := va.client.StartFaceDetection(ctx, startInput)
	if err != nil {
		return nil, fmt.Errorf("start face detection: %w", err)
	}

	// Track cost
	if va.costTracker != nil {
		if tracker, ok := va.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("StartFaceDetection", 1)
		}
	}

	jobID := *startResult.JobId
	return va.waitForFaceDetectionJob(ctx, jobID, 5*time.Minute)
}

// startLabelDetection starts asynchronous label detection in video
func (va *VideoAnalyzer) startLabelDetection(ctx context.Context, video *rekognitionTypes.Video) ([]rekognition.GetLabelDetectionOutput, error) {
	startInput := &rekognition.StartLabelDetectionInput{
		Video:         video,
		MinConfidence: aws.Float32(float32(va.config.ConfidenceThreshold * 100)),
	}

	startResult, err := va.client.StartLabelDetection(ctx, startInput)
	if err != nil {
		return nil, fmt.Errorf("start label detection: %w", err)
	}

	// Track cost
	if va.costTracker != nil {
		if tracker, ok := va.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest("StartLabelDetection", 1)
		}
	}

	jobID := *startResult.JobId
	return va.waitForLabelDetectionJob(ctx, jobID, 5*time.Minute)
}

// Helper structs and methods for results processing would go here
// (Truncated for brevity - these would handle the async job polling and result processing)

type cachedVideoResult struct {
	analysis *VideoAnalysis
	cachedAt time.Time
}

// Utility functions
func (va *VideoAnalyzer) getVideoDuration(_ context.Context, video *rekognitionTypes.Video) (time.Duration, error) {
	// Use S3 HeadObject to get metadata first
	if video.S3Object == nil {
		return 60 * time.Second, fmt.Errorf("S3 object information required for video duration")
	}

	// For production, you would typically use AWS MediaConvert, MediaInfo, or extract metadata
	// from the video file directly. For now, we'll use a heuristic based on file size
	// and default assumptions, but this should be enhanced with actual video metadata extraction.

	// Default duration based on typical video lengths
	defaultDuration := 60 * time.Second

	// In a production environment, you could:
	// 1. Use AWS MediaConvert to get video metadata
	// 2. Use AWS Lambda with FFmpeg layer to extract duration
	// 3. Store duration metadata when video is uploaded
	// 4. Use AWS Elemental MediaPackage for live content

	va.logger.Debug("using default video duration",
		zap.String("bucket", aws.ToString(video.S3Object.Bucket)),
		zap.String("key", aws.ToString(video.S3Object.Name)),
		zap.Duration("duration", defaultDuration))

	// You could enhance this by checking the file extension and applying
	// different heuristics for different video formats
	return defaultDuration, nil
}

func (va *VideoAnalyzer) analyzeKeyFrames(ctx context.Context, video *rekognitionTypes.Video, intervals []time.Duration) ([]FrameAnalysis, error) {
	// Extract frames at specified intervals and analyze as images
	var frames []FrameAnalysis

	va.logger.Info("analyzing key frames",
		zap.String("bucket", aws.ToString(video.S3Object.Bucket)),
		zap.String("key", aws.ToString(video.S3Object.Name)),
		zap.Int("intervals", len(intervals)))

	for i, timestamp := range intervals {
		va.logger.Debug("processing frame",
			zap.Int("frame_index", i),
			zap.Duration("timestamp", timestamp))

		// Initialize frame analysis structure
		frameAnalysis := FrameAnalysis{
			Timestamp: timestamp,
			ImageAnalysis: ImageAnalysis{
				AnalyzedAt: time.Now(),
				Explicit: ExplicitContent{
					IsExplicit: false,
					Confidence: 0.0,
				},
				Violence: ViolenceDetection{
					HasViolence: false,
					Confidence:  0.0,
				},
				Text:    []TextInImage{},
				Objects: []ObjectDetection{},
				Faces:   []FaceAnalysis{},
			},
		}

		// Extract frame at timestamp using FFmpeg
		frameImageURL, err := va.extractFrameAtTimestamp(ctx, video, timestamp)
		if err != nil {
			va.logger.Warn("frame extraction failed",
				zap.Int("frame_index", i),
				zap.Duration("timestamp", timestamp),
				zap.Error(err))
			// Continue with empty analysis for this frame
		} else {
			// Analyze extracted frame using existing ImageAnalyzer
			metadata := ContentMetadata{
				ContentType: ContentTypeImage,
				Context:     "video_frame",
				Timestamp:   time.Now(),
			}

			imageAnalysis, err := va.imageAnalyzer.AnalyzeImage(ctx, frameImageURL, metadata)
			if err != nil {
				va.logger.Warn("frame analysis failed",
					zap.String("frame_url", frameImageURL),
					zap.Int("frame_index", i),
					zap.Duration("timestamp", timestamp),
					zap.Error(err))
				// Continue with empty analysis for this frame
			} else {
				// Use the actual analysis results
				frameAnalysis.ImageAnalysis = *imageAnalysis

				va.logger.Debug("completed frame analysis",
					zap.String("frame_url", frameImageURL),
					zap.Int("frame_index", i),
					zap.Duration("timestamp", timestamp),
					zap.Bool("explicit_detected", imageAnalysis.Explicit.IsExplicit),
					zap.Bool("violence_detected", imageAnalysis.Violence.HasViolence),
					zap.Int("objects_detected", len(imageAnalysis.Objects)),
					zap.Int("faces_detected", len(imageAnalysis.Faces)),
					zap.Int("text_detected", len(imageAnalysis.Text)))
			}

			// Clean up temporary frame from S3
			va.cleanupTemporaryFrame(ctx, frameImageURL)
		}

		frames = append(frames, frameAnalysis)
	}

	va.logger.Info("completed key frame analysis",
		zap.Int("total_frames", len(frames)),
		zap.Int("successful_extractions", va.countSuccessfulFrames(frames)))

	return frames, nil
}

// countSuccessfulFrames counts frames that were successfully analyzed
func (va *VideoAnalyzer) countSuccessfulFrames(frames []FrameAnalysis) int {
	count := 0
	for _, frame := range frames {
		// Consider a frame successful if it has any analysis data
		if frame.ImageAnalysis.Explicit.Confidence > 0 ||
			frame.ImageAnalysis.Violence.Confidence > 0 ||
			len(frame.ImageAnalysis.Objects) > 0 ||
			len(frame.ImageAnalysis.Faces) > 0 ||
			len(frame.ImageAnalysis.Text) > 0 {
			count++
		}
	}
	return count
}

func (va *VideoAnalyzer) transcribeAudio(_ context.Context, video *rekognitionTypes.Video) (*AudioAnalysis, error) {
	// Audio transcription implementation using AWS Transcribe
	// This provides a framework for integrating with AWS Transcribe service

	if video.S3Object == nil {
		return &AudioAnalysis{
			Transcription: "",
			Language:      "unknown",
			TextAnalysis:  nil,
		}, fmt.Errorf("S3 object information required for audio transcription")
	}

	va.logger.Info("starting audio transcription analysis",
		zap.String("bucket", aws.ToString(video.S3Object.Bucket)),
		zap.String("key", aws.ToString(video.S3Object.Name)))

	// For production implementation, this would:
	// 1. Use AWS Transcribe service to start a transcription job
	// 2. Poll for job completion with proper error handling and retries
	// 3. Download and parse the transcript JSON from S3
	// 4. Extract the text content and language information
	// 5. Optionally run text analysis on the transcript

	// Production implementation would look like:
	// - Start transcription job with AWS Transcribe
	// - Wait for completion using exponential backoff
	// - Download transcript JSON from S3
	// - Parse transcript and extract text
	// - Run content analysis on transcript text if needed

	s3URI := fmt.Sprintf("s3://%s/%s", aws.ToString(video.S3Object.Bucket), aws.ToString(video.S3Object.Name))

	va.logger.Debug("transcription configuration",
		zap.String("s3_uri", s3URI),
		zap.Bool("text_analysis_enabled", va.config.EnableTextAnalysis))

	// For now, return empty transcription with proper structure
	// In production, this would contain the actual transcribed text
	audioAnalysis := &AudioAnalysis{
		Transcription: "",   // Would contain actual transcript
		Language:      "en", // Would be detected from audio
		TextAnalysis:  nil,  // Would contain text analysis if enabled
	}

	va.logger.Info("audio transcription framework ready",
		zap.String("s3_uri", s3URI),
		zap.String("status", "awaiting_transcribe_service_integration"))

	return audioAnalysis, nil
}

func (va *VideoAnalyzer) storeThumbnails(_ context.Context, analysis *VideoAnalysis) error {
	// Store frame thumbnails in S3 for manual review
	// This would be useful for human reviewers to quickly assess flagged content

	if err := common.ValidateSliceNotEmpty("analysis.Frames", analysis.Frames); err != nil {
		return nil // No frames to store
	}

	va.logger.Info("storing frame thumbnails for review",
		zap.String("video_url", analysis.VideoURL),
		zap.Int("frame_count", len(analysis.Frames)))

	// In production, this would:
	// 1. Generate thumbnail images from extracted frames
	// 2. Upload thumbnails to S3 with organized naming scheme
	// 3. Create manifest file listing all thumbnails
	// 4. Store metadata for easy retrieval during manual review

	// Example implementation structure:
	// for i, frame := range analysis.Frames {
	//     thumbnailKey := fmt.Sprintf("moderation/thumbnails/%s/frame_%03d_%s.jpg",
	//         extractVideoID(analysis.VideoURL), i, frame.Timestamp.String())
	//
	//     // Upload thumbnail to S3
	//     err := va.uploadThumbnailToS3(ctx, frameImageData, va.config.S3Bucket, thumbnailKey)
	//     if err != nil {
	//         va.logger.Warn("failed to upload thumbnail", zap.Error(err))
	//         continue
	//     }
	// }

	// For now, just log that we would store thumbnails
	for i, frame := range analysis.Frames {
		thumbnailPath := fmt.Sprintf("moderation/thumbnails/%s/frame_%03d_%s.jpg",
			va.extractVideoID(analysis.VideoURL), i, frame.Timestamp.String())

		va.logger.Debug("would store thumbnail",
			zap.String("path", thumbnailPath),
			zap.Duration("timestamp", frame.Timestamp))
	}

	return nil
}

// extractVideoID extracts a unique identifier from video URL for organizing thumbnails
func (va *VideoAnalyzer) extractVideoID(videoURL string) string {
	// Extract a unique identifier from the video URL
	// This could be the S3 key or a hash of the URL
	if isS3URL(videoURL) {
		key := extractS3Key(videoURL)
		// Remove file extension and path separators for clean ID
		key = strings.ReplaceAll(key, "/", "_")
		key = strings.ReplaceAll(key, ".", "_")
		return key
	}

	// Fallback: use hash of URL or timestamp
	return fmt.Sprintf("video_%d", time.Now().UnixNano())
}

// Async job polling methods with real implementations
func (va *VideoAnalyzer) waitForModerationJob(ctx context.Context, jobID string, timeout time.Duration) ([]rekognition.GetContentModerationOutput, error) {
	results, err := va.waitForJob(ctx, jobID, timeout, va.createModerationJobHandler())
	if err != nil {
		return nil, err
	}

	// Convert interface{} results back to proper type
	var typed []rekognition.GetContentModerationOutput
	for _, result := range results {
		if moderationResult, ok := result.(*rekognition.GetContentModerationOutput); ok {
			typed = append(typed, *moderationResult)
		}
	}
	return typed, nil
}

// rekognitionJobConfig holds configuration for creating job handlers
type rekognitionJobConfig struct {
	jobType          string
	operationType    string
	getResult        func(ctx context.Context, jobID string, nextToken *string) (interface{}, error)
	getJobStatus     func(result interface{}) rekognitionTypes.VideoJobStatus
	getNextToken     func(result interface{}) *string
	getStatusMessage func(result interface{}) *string
}

// createJobHandler creates a generic job handler with provided configuration
func (va *VideoAnalyzer) createJobHandler(config rekognitionJobConfig) *rekognitionJobHandler {
	return &rekognitionJobHandler{
		jobType:          config.jobType,
		operationType:    config.operationType,
		getResult:        config.getResult,
		getJobStatus:     config.getJobStatus,
		getNextToken:     config.getNextToken,
		getStatusMessage: config.getStatusMessage,
	}
}

// jobHandlerFactory creates standardized job handlers using reflection-like patterns
type jobHandlerFactory struct {
	va *VideoAnalyzer
}

// createStandardJobConfig creates a standard job configuration with type-safe accessors
func (f *jobHandlerFactory) createStandardJobConfig(
	jobType, operationType string,
	getResultFunc func(ctx context.Context, jobID string, nextToken *string) (interface{}, error),
	resultAccessor func(interface{}) (rekognitionTypes.VideoJobStatus, *string, *string),
) rekognitionJobConfig {
	return rekognitionJobConfig{
		jobType:       jobType,
		operationType: operationType,
		getResult:     getResultFunc,
		getJobStatus: func(result interface{}) rekognitionTypes.VideoJobStatus {
			status, _, _ := resultAccessor(result)
			return status
		},
		getNextToken: func(result interface{}) *string {
			_, nextToken, _ := resultAccessor(result)
			return nextToken
		},
		getStatusMessage: func(result interface{}) *string {
			_, _, statusMessage := resultAccessor(result)
			return statusMessage
		},
	}
}

// createModerationJobHandler creates a handler for content moderation jobs
func (va *VideoAnalyzer) createModerationJobHandler() *rekognitionJobHandler {
	factory := &jobHandlerFactory{va: va}
	config := factory.createStandardJobConfig(
		"content moderation",
		"GetContentModeration",
		func(ctx context.Context, jobID string, nextToken *string) (interface{}, error) {
			input := &rekognition.GetContentModerationInput{
				JobId:     aws.String(jobID),
				NextToken: nextToken,
			}
			return va.client.GetContentModeration(ctx, input)
		},
		func(result interface{}) (rekognitionTypes.VideoJobStatus, *string, *string) {
			if moderationResult, ok := result.(*rekognition.GetContentModerationOutput); ok {
				return moderationResult.JobStatus, moderationResult.NextToken, moderationResult.StatusMessage
			}
			return rekognitionTypes.VideoJobStatusFailed, nil, nil
		},
	)
	return va.createJobHandler(config)
}

func (va *VideoAnalyzer) waitForTextDetectionJob(ctx context.Context, jobID string, timeout time.Duration) ([]rekognition.GetTextDetectionOutput, error) {
	results, err := va.waitForJob(ctx, jobID, timeout, va.createTextDetectionJobHandler())
	if err != nil {
		return nil, err
	}

	// Convert interface{} results back to proper type
	var typed []rekognition.GetTextDetectionOutput
	for _, result := range results {
		if textResult, ok := result.(*rekognition.GetTextDetectionOutput); ok {
			typed = append(typed, *textResult)
		}
	}
	return typed, nil
}

func (va *VideoAnalyzer) waitForFaceDetectionJob(ctx context.Context, jobID string, timeout time.Duration) ([]rekognition.GetFaceDetectionOutput, error) {
	results, err := va.waitForJob(ctx, jobID, timeout, va.createFaceDetectionJobHandler())
	if err != nil {
		return nil, err
	}

	// Convert interface{} results back to proper type
	var typed []rekognition.GetFaceDetectionOutput
	for _, result := range results {
		if faceResult, ok := result.(*rekognition.GetFaceDetectionOutput); ok {
			typed = append(typed, *faceResult)
		}
	}
	return typed, nil
}

func (va *VideoAnalyzer) waitForLabelDetectionJob(ctx context.Context, jobID string, timeout time.Duration) ([]rekognition.GetLabelDetectionOutput, error) {
	results, err := va.waitForJob(ctx, jobID, timeout, va.createLabelDetectionJobHandler())
	if err != nil {
		return nil, err
	}

	// Convert interface{} results back to proper type
	var typed []rekognition.GetLabelDetectionOutput
	for _, result := range results {
		if labelResult, ok := result.(*rekognition.GetLabelDetectionOutput); ok {
			typed = append(typed, *labelResult)
		}
	}
	return typed, nil
}

// rekognitionJobHandler defines the interface for handling different Rekognition job types
type rekognitionJobHandler struct {
	jobType          string
	operationType    string
	getResult        func(ctx context.Context, jobID string, nextToken *string) (interface{}, error)
	getJobStatus     func(result interface{}) rekognitionTypes.VideoJobStatus
	getNextToken     func(result interface{}) *string
	getStatusMessage func(result interface{}) *string
}

// waitForJob waits for any Rekognition job type and returns results as interface{}
func (va *VideoAnalyzer) waitForJob(ctx context.Context, jobID string, timeout time.Duration, handler *rekognitionJobHandler) ([]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	poller := &jobPoller{
		va:      va,
		handler: handler,
	}

	return poller.poll(ctx, jobID)
}

// jobPoller handles the polling logic for Rekognition jobs
type jobPoller struct {
	va      *VideoAnalyzer
	handler *rekognitionJobHandler
}

// poll executes the polling loop for a specific job
func (p *jobPoller) poll(ctx context.Context, jobID string) ([]interface{}, error) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		if err := p.checkContext(ctx, jobID); err != nil {
			return nil, err
		}

		result, err := p.getJobResult(ctx, jobID, nil)
		if err != nil {
			return nil, err
		}

		p.trackCost()

		switch p.handler.getJobStatus(result) {
		case rekognitionTypes.VideoJobStatusSucceeded:
			return p.collectAllPages(ctx, jobID, result)
		case rekognitionTypes.VideoJobStatusFailed:
			return nil, p.createFailureError(result)
		case rekognitionTypes.VideoJobStatusInProgress:
			backoff = p.waitWithBackoff(backoff, maxBackoff)
			continue
		default:
			return nil, fmt.Errorf("unexpected job status: %s", p.handler.getJobStatus(result))
		}
	}
}

// checkContext verifies the context is still valid
func (p *jobPoller) checkContext(ctx context.Context, jobID string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for %s job %s: %w", p.handler.jobType, jobID, ctx.Err())
	default:
		return nil
	}
}

// getJobResult fetches the job result from Rekognition
func (p *jobPoller) getJobResult(ctx context.Context, jobID string, nextToken *string) (interface{}, error) {
	result, err := p.handler.getResult(ctx, jobID, nextToken)
	if err != nil {
		return nil, fmt.Errorf("get %s result: %w", p.handler.jobType, err)
	}
	return result, nil
}

// trackCost records the cost of the operation
func (p *jobPoller) trackCost() {
	if p.va.costTracker != nil {
		if tracker, ok := p.va.costTracker.(RekognitionCostTracker); ok {
			tracker.TrackRekognitionRequest(p.handler.operationType, 1)
		}
	}
}

// collectAllPages gathers all paginated results
func (p *jobPoller) collectAllPages(ctx context.Context, jobID string, firstResult interface{}) ([]interface{}, error) {
	results := []interface{}{firstResult}
	nextToken := p.handler.getNextToken(firstResult)

	for nextToken != nil {
		nextResult, err := p.getJobResult(ctx, jobID, nextToken)
		if err != nil {
			p.va.logger.Warn("failed to get next page of results", zap.Error(err))
			break
		}

		results = append(results, nextResult)
		nextToken = p.handler.getNextToken(nextResult)
		p.trackCost()
	}

	return results, nil
}

// createFailureError creates an error for failed jobs
func (p *jobPoller) createFailureError(result interface{}) error {
	return fmt.Errorf("%s job failed: %s", p.handler.jobType, aws.ToString(p.handler.getStatusMessage(result)))
}

// waitWithBackoff implements exponential backoff waiting
func (p *jobPoller) waitWithBackoff(currentBackoff, maxBackoff time.Duration) time.Duration {
	time.Sleep(currentBackoff)
	if currentBackoff < maxBackoff {
		return currentBackoff * 2
	}
	return currentBackoff
}

// createTextDetectionJobHandler creates a handler for text detection jobs
func (va *VideoAnalyzer) createTextDetectionJobHandler() *rekognitionJobHandler {
	factory := &jobHandlerFactory{va: va}
	config := factory.createStandardJobConfig(
		"text detection",
		"GetTextDetection",
		func(ctx context.Context, jobID string, nextToken *string) (interface{}, error) {
			input := &rekognition.GetTextDetectionInput{
				JobId:     aws.String(jobID),
				NextToken: nextToken,
			}
			return va.client.GetTextDetection(ctx, input)
		},
		func(result interface{}) (rekognitionTypes.VideoJobStatus, *string, *string) {
			if textResult, ok := result.(*rekognition.GetTextDetectionOutput); ok {
				return textResult.JobStatus, textResult.NextToken, textResult.StatusMessage
			}
			return rekognitionTypes.VideoJobStatusFailed, nil, nil
		},
	)
	return va.createJobHandler(config)
}

// createFaceDetectionJobHandler creates a handler for face detection jobs
func (va *VideoAnalyzer) createFaceDetectionJobHandler() *rekognitionJobHandler {
	factory := &jobHandlerFactory{va: va}
	config := factory.createStandardJobConfig(
		"face detection",
		"GetFaceDetection",
		func(ctx context.Context, jobID string, nextToken *string) (interface{}, error) {
			input := &rekognition.GetFaceDetectionInput{
				JobId:     aws.String(jobID),
				NextToken: nextToken,
			}
			return va.client.GetFaceDetection(ctx, input)
		},
		func(result interface{}) (rekognitionTypes.VideoJobStatus, *string, *string) {
			if faceResult, ok := result.(*rekognition.GetFaceDetectionOutput); ok {
				return faceResult.JobStatus, faceResult.NextToken, faceResult.StatusMessage
			}
			return rekognitionTypes.VideoJobStatusFailed, nil, nil
		},
	)
	return va.createJobHandler(config)
}

// createLabelDetectionJobHandler creates a handler for label detection jobs
func (va *VideoAnalyzer) createLabelDetectionJobHandler() *rekognitionJobHandler {
	factory := &jobHandlerFactory{va: va}
	config := factory.createStandardJobConfig(
		"label detection",
		"GetLabelDetection",
		func(ctx context.Context, jobID string, nextToken *string) (interface{}, error) {
			input := &rekognition.GetLabelDetectionInput{
				JobId:     aws.String(jobID),
				NextToken: nextToken,
			}
			return va.client.GetLabelDetection(ctx, input)
		},
		func(result interface{}) (rekognitionTypes.VideoJobStatus, *string, *string) {
			if labelResult, ok := result.(*rekognition.GetLabelDetectionOutput); ok {
				return labelResult.JobStatus, labelResult.NextToken, labelResult.StatusMessage
			}
			return rekognitionTypes.VideoJobStatusFailed, nil, nil
		},
	)
	return va.createJobHandler(config)
}

// Result processing methods
func (va *VideoAnalyzer) processModerationResults(results []rekognition.GetContentModerationOutput, _ *VideoAnalysis) {
	// Process moderation results and update analysis
	// Since VideoAnalysis doesn't have direct Explicit/Violence fields,
	// we'll update frame-level analysis or store in a summary structure

	moderationEvents := make(map[string][]ModerationEvent)

	for _, result := range results {
		for _, moderationLabel := range result.ModerationLabels {
			// Update explicit content analysis
			confidence := float64(aws.ToFloat32(moderationLabel.ModerationLabel.Confidence)) / 100.0
			timestamp := time.Duration(moderationLabel.Timestamp) * time.Millisecond
			labelName := aws.ToString(moderationLabel.ModerationLabel.Name)

			// Create moderation event
			event := ModerationEvent{
				Type:       labelName,
				Confidence: confidence,
				Timestamp:  timestamp,
			}

			// Categorize the moderation label
			switch labelName {
			case "Explicit Nudity", "Nudity":
				moderationEvents["explicit"] = append(moderationEvents["explicit"], event)
			case "Suggestive":
				moderationEvents["suggestive"] = append(moderationEvents["suggestive"], event)
			case "Violence", "Graphic Violence And Gore":
				moderationEvents["violence"] = append(moderationEvents["violence"], event)
			case "Visually Disturbing":
				moderationEvents["disturbing"] = append(moderationEvents["disturbing"], event)
			default:
				moderationEvents["other"] = append(moderationEvents["other"], event)
			}

			va.logger.Debug("processed moderation label",
				zap.String("label", labelName),
				zap.Float64("confidence", confidence),
				zap.Duration("timestamp", timestamp))
		}
	}

	// Store moderation events summary (could be added to VideoAnalysis struct if needed)
	va.logger.Info("moderation analysis completed",
		zap.Int("explicit_events", len(moderationEvents["explicit"])),
		zap.Int("violence_events", len(moderationEvents["violence"])),
		zap.Int("suggestive_events", len(moderationEvents["suggestive"])),
		zap.Int("disturbing_events", len(moderationEvents["disturbing"])))
}

// ModerationEvent represents a moderation event in the video
type ModerationEvent struct {
	Type       string
	Confidence float64
	Timestamp  time.Duration
}

func (va *VideoAnalyzer) processTextResults(results []rekognition.GetTextDetectionOutput, analysis *VideoAnalysis) {
	// Process text detection results and update analysis
	// Since VideoAnalysis doesn't have a direct Text field, we'll collect text
	// and could store it in the Audio.Transcription or frame-level data

	textItems := make(map[string]*TextInImage) // Deduplicate by text content

	for _, result := range results {
		for _, textDetection := range result.TextDetections {
			text := aws.ToString(textDetection.TextDetection.DetectedText)
			confidence := float64(aws.ToFloat32(textDetection.TextDetection.Confidence)) / 100.0

			if confidence < va.config.ConfidenceThreshold {
				continue
			}

			// Only process LINE type text (not individual words)
			if textDetection.TextDetection.Type == rekognitionTypes.TextTypesLine {
				textItem := &TextInImage{
					Text:       text,
					Confidence: confidence,
				}

				// Set bounding box if available
				if geo := textDetection.TextDetection.Geometry; geo != nil && geo.BoundingBox != nil {
					textItem.BoundingBox = BoundingBox{
						Left:   float64(aws.ToFloat32(geo.BoundingBox.Left)),
						Top:    float64(aws.ToFloat32(geo.BoundingBox.Top)),
						Width:  float64(aws.ToFloat32(geo.BoundingBox.Width)),
						Height: float64(aws.ToFloat32(geo.BoundingBox.Height)),
					}
				}

				// Keep the highest confidence version
				if existing, exists := textItems[text]; !exists || confidence > existing.Confidence {
					textItems[text] = textItem
				}
			}
		}
	}

	// Collect all detected text for potential storage in transcription
	var detectedTexts []string
	for _, textItem := range textItems {
		detectedTexts = append(detectedTexts, textItem.Text)
	}

	// Store detected text in audio transcription if empty
	if err := common.ValidateRequiredParam("analysis.Audio.Transcription", analysis.Audio.Transcription); err != nil && len(detectedTexts) > 0 {
		analysis.Audio.Transcription = strings.Join(detectedTexts, " ")
	}

	va.logger.Debug("processed text detection results",
		zap.Int("unique_text_items", len(textItems)),
		zap.Strings("detected_texts", detectedTexts))
}

func (va *VideoAnalyzer) processFaceResults(results []rekognition.GetFaceDetectionOutput, _ *VideoAnalysis) {
	// Process face detection results and update analysis
	// VideoAnalysis doesn't have a direct Faces field, so we'll store in frame data

	faceMap := make(map[string]*FaceAnalysis) // Deduplicate by approximate location

	for _, result := range results {
		for _, faceDetection := range result.Faces {
			face := faceDetection.Face
			confidence := float64(aws.ToFloat32(face.Confidence)) / 100.0

			if confidence < va.config.ConfidenceThreshold {
				continue
			}

			faceAnalysis := &FaceAnalysis{
				Confidence: confidence,
			}

			// Set bounding box
			if bbox := face.BoundingBox; bbox != nil {
				faceAnalysis.BoundingBox = BoundingBox{
					Left:   float64(aws.ToFloat32(bbox.Left)),
					Top:    float64(aws.ToFloat32(bbox.Top)),
					Width:  float64(aws.ToFloat32(bbox.Width)),
					Height: float64(aws.ToFloat32(bbox.Height)),
				}
			}

			// Process emotions
			for _, emotion := range face.Emotions {
				emotionConf := float64(aws.ToFloat32(emotion.Confidence)) / 100.0
				if emotionConf > 0.5 { // Only include high-confidence emotions
					faceAnalysis.Emotions = append(faceAnalysis.Emotions, Emotion{
						Type:       string(emotion.Type),
						Confidence: emotionConf,
					})
				}
			}

			// Process age range
			if face.AgeRange != nil {
				faceAnalysis.AgeRange = AgeRange{
					Low:  int(aws.ToInt32(face.AgeRange.Low)),
					High: int(aws.ToInt32(face.AgeRange.High)),
				}
			}

			// Process gender
			if face.Gender != nil {
				faceAnalysis.Gender = Gender{
					Value:      string(face.Gender.Value),
					Confidence: float64(aws.ToFloat32(face.Gender.Confidence)) / 100.0,
				}
			}

			// Create a key based on bounding box for deduplication
			key := fmt.Sprintf("%.3f_%.3f_%.3f_%.3f",
				faceAnalysis.BoundingBox.Left,
				faceAnalysis.BoundingBox.Top,
				faceAnalysis.BoundingBox.Width,
				faceAnalysis.BoundingBox.Height)

			// Keep the highest confidence version
			if existing, exists := faceMap[key]; !exists || confidence > existing.Confidence {
				faceMap[key] = faceAnalysis
			}
		}
	}

	// Store face count and high-level information
	faceCount := len(faceMap)

	va.logger.Debug("processed face detection results",
		zap.Int("unique_faces", faceCount))

	// Could store face information in frames or as metadata
	// For now, just log the processed results
}

func (va *VideoAnalyzer) processLabelResults(results []rekognition.GetLabelDetectionOutput, _ *VideoAnalysis) {
	// Process label detection results and update analysis
	// VideoAnalysis doesn't have direct Violence or Objects fields, so we'll track events

	labelMap := make(map[string]*ObjectDetection) // Deduplicate by label name
	violenceEvents := make([]string, 0)
	weaponEvents := make([]string, 0)

	for _, result := range results {
		for _, labelDetection := range result.Labels {
			label := labelDetection.Label
			confidence := float64(aws.ToFloat32(label.Confidence)) / 100.0

			if confidence < va.config.ConfidenceThreshold {
				continue
			}

			labelName := aws.ToString(label.Name)

			// Check for violence/weapon indicators
			if va.isViolenceLabel(labelName) {
				if confidence > va.config.ViolenceThreshold {
					violenceEvents = append(violenceEvents, labelName)
				}

				// Add to weapons detected list
				if va.isWeaponLabel(labelName) {
					weaponEvents = append(weaponEvents, labelName)
				}
			}

			objectDetection := &ObjectDetection{
				Name:       labelName,
				Confidence: confidence,
			}

			// Process parent categories
			for _, parent := range label.Parents {
				objectDetection.Parents = append(objectDetection.Parents, aws.ToString(parent.Name))
			}

			// Keep the highest confidence version
			if existing, exists := labelMap[labelName]; !exists || confidence > existing.Confidence {
				labelMap[labelName] = objectDetection
			}
		}
	}

	va.logger.Debug("processed label detection results",
		zap.Int("unique_labels", len(labelMap)),
		zap.Int("violence_events", len(violenceEvents)),
		zap.Int("weapon_events", len(weaponEvents)),
		zap.Strings("detected_violence", violenceEvents),
		zap.Strings("detected_weapons", weaponEvents))
}

// Helper methods for label categorization
func (va *VideoAnalyzer) isViolenceLabel(label string) bool {
	violenceLabels := []string{
		"Violence", "Weapon", "Gun", "Knife", "Sword", "Pistol", "Rifle",
		"Blade", "Fighting", "Blood", "Gore", "Explosion", "Fire", "Destruction",
	}

	for _, vLabel := range violenceLabels {
		if strings.EqualFold(label, vLabel) || strings.Contains(strings.ToLower(label), strings.ToLower(vLabel)) {
			return true
		}
	}
	return false
}

func (va *VideoAnalyzer) isWeaponLabel(label string) bool {
	weaponLabels := []string{
		"Weapon", "Gun", "Knife", "Sword", "Pistol", "Rifle", "Blade",
		"Handgun", "Shotgun", "Ammunition", "Grenade", "Bomb",
	}

	for _, wLabel := range weaponLabels {
		if strings.EqualFold(label, wLabel) {
			return true
		}
	}
	return false
}

// GlobalConfig interface for accessing global configuration
type GlobalConfig interface {
	GetDisableAWSModeration() bool
	GetDisableComprehend() bool
	GetDisableRekognition() bool
}

// getGlobalConfig safely attempts to get the global configuration
// Returns nil if config is not available to avoid hard dependencies
func getGlobalConfig() GlobalConfig {
	// Use centralized configuration for consistent config management
	cfg := appconfig.Get()
	return &configChecker{
		disableAWSModeration: cfg.DisableAWSModeration,
		disableComprehend:    cfg.DisableComprehend,
		disableRekognition:   cfg.DisableRekognition,
	}
}

// configChecker implements GlobalConfig interface
type configChecker struct {
	disableAWSModeration bool
	disableComprehend    bool
	disableRekognition   bool
}

func (c *configChecker) GetDisableAWSModeration() bool {
	return c.disableAWSModeration
}

func (c *configChecker) GetDisableComprehend() bool {
	return c.disableComprehend
}

func (c *configChecker) GetDisableRekognition() bool {
	return c.disableRekognition
}
