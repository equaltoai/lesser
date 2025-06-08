package advanced

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"go.uber.org/zap"
)

// Engine is the main moderation engine implementation
type Engine struct {
	config *ModerationConfig
	logger *zap.Logger

	// Analyzers
	textAnalyzer  *TextAnalyzer
	imageAnalyzer *ImageAnalyzer

	// Components
	patternMatcher   *PatternMatcher
	reputationScorer *ReputationScorer
	decisionEngine   *DecisionEngine
	threatIntel      *ThreatIntelligence

	// Storage
	db        *dynamodb.Client
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
	db *dynamodb.Client,
	tableName string,
	logger *zap.Logger,
	costTracker CostTracker,
) *Engine {
	// Create components
	textAnalyzer := NewTextAnalyzer(comprehendClient, logger, config, costTracker)
	imageAnalyzer := NewImageAnalyzer(rekognitionClient, logger, config, costTracker)
	patternMatcher := NewPatternMatcher(db, tableName, logger)
	reputationScorer := NewReputationScorer(db, tableName, logger, config)
	threatIntel := NewThreatIntelligence(db, tableName, logger)
	decisionEngine := NewDecisionEngine(config, logger, reputationScorer)
	metrics := NewModerationMetrics(db, tableName, logger)

	return &Engine{
		config:           config,
		logger:           logger,
		textAnalyzer:     textAnalyzer,
		imageAnalyzer:    imageAnalyzer,
		patternMatcher:   patternMatcher,
		reputationScorer: reputationScorer,
		decisionEngine:   decisionEngine,
		threatIntel:      threatIntel,
		db:               db,
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

// AnalyzeVideo analyzes video content
func (e *Engine) AnalyzeVideo(videoURL string, metadata ContentMetadata) (*VideoAnalysis, error) {
	// Video analysis would sample frames and analyze them as images
	// Plus transcribe audio for text analysis
	// This is a placeholder for the full implementation
	return nil, fmt.Errorf("video analysis not yet implemented")
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

func (e *Engine) checkRateLimits(ctx context.Context, actorID string) error {
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
	// Store in DynamoDB for later retrieval and analysis
	// This helps with improving the system and handling appeals
	return nil
}

func (e *Engine) storeDecision(ctx context.Context, decision *ModerationDecision, metadata ContentMetadata) error {
	// Store decision in DynamoDB for audit trail
	item := map[string]types.AttributeValue{
		"PK":         &types.AttributeValueMemberS{Value: fmt.Sprintf("CONTENT#%s", metadata.ContentID)},
		"SK":         &types.AttributeValueMemberS{Value: fmt.Sprintf("DECISION#%d", time.Now().UnixNano())},
		"Decision":   &types.AttributeValueMemberS{Value: string(decision.Decision)},
		"Confidence": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", decision.Confidence)},
		"AuthorID":   &types.AttributeValueMemberS{Value: metadata.AuthorID},
		"Timestamp":  &types.AttributeValueMemberS{Value: decision.DecidedAt.Format(time.RFC3339)},
		"TTL":        &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())},
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(e.tableName),
		Item:      item,
	}

	_, err := e.db.PutItem(ctx, putInput)
	return err
}

func (e *Engine) sendToReviewQueue(ctx context.Context, decision *ModerationDecision, metadata ContentMetadata) error {
	// Send to human review queue
	// In production, this would send to SQS or similar
	e.logger.Info("content sent to review queue",
		zap.String("contentID", metadata.ContentID),
		zap.Int("priority", decision.ReviewPriority))
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
