package moderation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Moderation action constants
const (
	actionAllow    = "allow"
	actionFlag     = "flag"
	actionHide     = "hide"
	actionEscalate = "escalate"
	actionBlock    = "block"
)

// ModerationStorage defines storage operations needed by the moderator
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific storage
type ModerationStorage interface {
	StoreModerationDecision(ctx context.Context, decision *ModerationResult) error
	UpdateModerationDecision(ctx context.Context, contentID string, review *ModerationReview) error
	GetModerationHistory(ctx context.Context, contentID string) ([]*ModerationResult, error)
	GetModerationQueue(ctx context.Context, filter *ModerationFilter) ([]*ModerationQueueItem, error)
}

// Moderator provides comprehensive content moderation
type Moderator struct {
	patternManager *PatternManager
	aiAnalyzer     *AIAnalyzer
	storage        ModerationStorage
}

// NewModerator creates a new moderator instance
func NewModerator(store ModerationStorage, aiAnalyzer *AIAnalyzer) *Moderator {
	return &Moderator{
		patternManager: NewPatternManager(),
		aiAnalyzer:     aiAnalyzer,
		storage:        store,
	}
}

// ModerateContent performs comprehensive content moderation
func (m *Moderator) ModerateContent(ctx context.Context, content *ContentSubmission) (*ModerationResult, error) {
	result := &ModerationResult{
		ContentID:   content.ID,
		ContentType: content.Type,
		SubmittedAt: content.SubmittedAt,
		ProcessedAt: time.Now(),
		Action:      actionAllow, // Default to allow
		Confidence:  0.0,
	}

	// Step 1: Pattern matching
	patternMatches, err := m.moderateWithPatterns(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("pattern moderation failed: %w", err)
	}
	result.PatternMatches = patternMatches

	// Step 2: AI analysis
	aiAnalysis, err := m.moderateWithAI(ctx, content)
	if err != nil {
		// AI failure shouldn't block moderation, log and continue
		fmt.Printf("AI moderation failed for %s: %v\n", content.ID, err)
	} else {
		result.AIAnalysis = aiAnalysis
	}

	// Step 3: Combine results and make decision
	m.calculateFinalDecision(result)

	// Step 4: Record moderation decision
	if err := m.recordModerationDecision(ctx, result); err != nil {
		fmt.Printf("Failed to record moderation decision: %v\n", err)
	}

	return result, nil
}

// moderateWithPatterns applies pattern-based moderation
func (m *Moderator) moderateWithPatterns(ctx context.Context, content *ContentSubmission) ([]*PatternMatch, error) {
	contentToModerate := &ContentToModerate{
		ID:       content.ID,
		Text:     content.Text,
		Author:   content.Author,
		Type:     content.Type,
		Metadata: content.Metadata,
	}

	// Generate content hashes if not provided
	if contentToModerate.TextHash == "" && content.Text != "" {
		contentToModerate.TextHash = generateTextHash(content.Text)
	}

	return m.patternManager.MatchContent(ctx, contentToModerate)
}

// moderateWithAI applies AI-based moderation
func (m *Moderator) moderateWithAI(ctx context.Context, content *ContentSubmission) (*AIAnalysisResult, error) {
	analysis := &AIAnalysisResult{}

	// Analyze text content
	if content.Text != "" {
		textContent := &TextContent{
			ID:   content.ID,
			Text: content.Text,
		}

		textAnalysis, err := m.aiAnalyzer.AnalyzeText(ctx, textContent)
		if err != nil {
			return nil, fmt.Errorf("text analysis failed: %w", err)
		}
		analysis.TextAnalysis = textAnalysis
	}

	// Analyze image content
	if content.ImageBytes != nil {
		imageContent := &ImageContent{
			ID:         content.ID,
			URL:        content.ImageURL,
			ImageBytes: content.ImageBytes,
		}

		imageAnalysis, err := m.aiAnalyzer.AnalyzeImage(ctx, imageContent)
		if err != nil {
			return nil, fmt.Errorf("image analysis failed: %w", err)
		}
		analysis.ImageAnalysis = imageAnalysis
	}

	return analysis, nil
}

// calculateFinalDecision combines pattern and AI results to make final decision
func (m *Moderator) calculateFinalDecision(result *ModerationResult) {
	var scores []float64
	var actions []string
	var reasons []string

	// Pattern matching contribution
	if len(result.PatternMatches) > 0 {
		patternScore, patternAction := m.evaluatePatternMatches(result.PatternMatches)
		scores = append(scores, patternScore)
		actions = append(actions, patternAction)

		for _, match := range result.PatternMatches {
			reasons = append(reasons, fmt.Sprintf("Pattern match: %s (%s)", match.PatternName, match.Severity))
		}
	}

	// AI analysis contribution
	if result.AIAnalysis != nil {
		aiScore, aiAction := m.evaluateAIAnalysis(result.AIAnalysis)
		scores = append(scores, aiScore)
		actions = append(actions, aiAction)

		if result.AIAnalysis.TextAnalysis != nil {
			reasons = append(reasons, fmt.Sprintf("Text AI score: %.1f (%s)",
				result.AIAnalysis.TextAnalysis.ModerationScore,
				result.AIAnalysis.TextAnalysis.RiskLevel))
		}

		if result.AIAnalysis.ImageAnalysis != nil {
			reasons = append(reasons, fmt.Sprintf("Image AI score: %.1f (%s)",
				result.AIAnalysis.ImageAnalysis.ModerationScore,
				result.AIAnalysis.ImageAnalysis.RiskLevel))
		}
	}

	// Calculate final score (weighted average)
	if len(scores) > 0 {
		var totalWeight float64
		var weightedSum float64

		for i, score := range scores {
			weight := 1.0
			if i == 0 { // Pattern matching gets higher weight for precision
				weight = 1.5
			}
			weightedSum += score * weight
			totalWeight += weight
		}

		result.Score = weightedSum / totalWeight
	}

	// Determine final action based on highest severity action
	result.Action = m.determineHighestSeverityAction(actions)

	// Set confidence based on agreement between methods
	result.Confidence = m.calculateConfidence(actions, scores)

	// Set reasons
	result.Reasons = reasons

	// Add recommendations
	result.Recommendations = m.generateRecommendations(result)
}

// evaluatePatternMatches evaluates pattern match results
func (m *Moderator) evaluatePatternMatches(matches []*PatternMatch) (float64, string) {
	if len(matches) == 0 {
		return 0.0, actionAllow
	}

	var maxScore float64
	var highestAction string
	severityWeights := map[string]float64{
		"low":      25.0,
		"medium":   50.0,
		"high":     75.0,
		"critical": 100.0,
	}

	for _, match := range matches {
		score := severityWeights[match.Severity] * match.Confidence
		if score > maxScore {
			maxScore = score
			highestAction = match.Action
		}
	}

	return maxScore, highestAction
}

// evaluateAIAnalysis evaluates AI analysis results
func (m *Moderator) evaluateAIAnalysis(analysis *AIAnalysisResult) (float64, string) {
	var maxScore float64
	action := actionAllow

	if analysis.TextAnalysis != nil {
		score := analysis.TextAnalysis.ModerationScore
		if score > maxScore {
			maxScore = score
		}
	}

	if analysis.ImageAnalysis != nil {
		score := analysis.ImageAnalysis.ModerationScore
		if score > maxScore {
			maxScore = score
		}
	}

	// Determine action based on score
	if maxScore >= 80 {
		action = actionBlock
	} else if maxScore >= 60 {
		action = actionEscalate
	} else if maxScore >= 40 {
		action = actionFlag
	}

	return maxScore, action
}

// determineHighestSeverityAction determines the most restrictive action
func (m *Moderator) determineHighestSeverityAction(actions []string) string {
	if len(actions) == 0 {
		return actionAllow
	}

	actionSeverity := map[string]int{
		actionAllow:    0,
		actionFlag:     1,
		actionHide:     2,
		actionEscalate: 3,
		actionBlock:    4,
	}

	highestSeverity := 0
	result := actionAllow

	for _, action := range actions {
		if severity, exists := actionSeverity[action]; exists && severity > highestSeverity {
			highestSeverity = severity
			result = action
		}
	}

	return result
}

// calculateConfidence calculates confidence based on agreement between methods
func (m *Moderator) calculateConfidence(actions []string, scores []float64) float64 {
	if len(actions) <= 1 {
		return 0.7 // Moderate confidence with single method
	}

	// Check action agreement
	actionAgreement := true
	if len(actions) > 1 {
		firstAction := actions[0]
		for _, action := range actions[1:] {
			if action != firstAction {
				actionAgreement = false
				break
			}
		}
	}

	// Check score similarity
	scoreAgreement := true
	if len(scores) > 1 {
		var scoreVariance float64
		var scoreMean float64

		for _, score := range scores {
			scoreMean += score
		}
		scoreMean /= float64(len(scores))

		for _, score := range scores {
			scoreVariance += (score - scoreMean) * (score - scoreMean)
		}
		scoreVariance /= float64(len(scores))

		if scoreVariance > 400 { // High variance threshold
			scoreAgreement = false
		}
	}

	// Calculate confidence
	confidence := 0.5 // Base confidence
	if actionAgreement {
		confidence += 0.3
	}
	if scoreAgreement {
		confidence += 0.2
	}

	return confidence
}

// generateRecommendations generates recommendations based on moderation result
func (m *Moderator) generateRecommendations(result *ModerationResult) []string {
	var recommendations []string

	switch result.Action {
	case actionBlock:
		recommendations = append(recommendations, "Content blocked - remove immediately")
		recommendations = append(recommendations, "Consider user suspension if repeat offense")
	case actionEscalate:
		recommendations = append(recommendations, "Escalate to human moderator for review")
		recommendations = append(recommendations, "Monitor user activity closely")
	case actionFlag:
		recommendations = append(recommendations, "Flag for moderator attention")
		recommendations = append(recommendations, "Consider content warning")
	case actionHide:
		recommendations = append(recommendations, "Hide content from public view")
		recommendations = append(recommendations, "Allow appeal process")
	}

	if result.Confidence < 0.6 {
		recommendations = append(recommendations, "Low confidence decision - consider manual review")
	}

	// Add AI-specific recommendations
	if result.AIAnalysis != nil {
		if result.AIAnalysis.TextAnalysis != nil {
			recommendations = append(recommendations, result.AIAnalysis.TextAnalysis.Recommendations...)
		}
		if result.AIAnalysis.ImageAnalysis != nil {
			recommendations = append(recommendations, result.AIAnalysis.ImageAnalysis.Recommendations...)
		}
	}

	return recommendations
}

// recordModerationDecision records the moderation decision for tracking
func (m *Moderator) recordModerationDecision(ctx context.Context, result *ModerationResult) error {
	// This would store the moderation decision in the database
	// For audit trail and effectiveness tracking

	decision := &ModerationResult{
		ContentID:       result.ContentID,
		Action:          result.Action,
		Score:           result.Score,
		Confidence:      result.Confidence,
		ProcessedAt:     result.ProcessedAt,
		PatternMatches:  result.PatternMatches,
		AIAnalysis:      result.AIAnalysis,
		Reasons:         result.Reasons,
		Recommendations: result.Recommendations,
	}

	// Store decision in the database for audit trail and effectiveness tracking
	return m.storage.StoreModerationDecision(ctx, decision)
}

// GetModerationQueue retrieves items that need manual review
func (m *Moderator) GetModerationQueue(ctx context.Context, filter *ModerationFilter) ([]*ModerationQueueItem, error) {
	// This would retrieve items flagged for manual review
	// Implementation would depend on storage layer
	return m.storage.GetModerationQueue(ctx, filter)
}

// ReviewModerationDecision allows moderators to review and update decisions
func (m *Moderator) ReviewModerationDecision(ctx context.Context, review *ModerationReview) error {
	// Update the original decision
	if err := m.storage.UpdateModerationDecision(ctx, review.ContentID, review); err != nil {
		return fmt.Errorf("failed to update moderation decision: %w", err)
	}

	// Update pattern effectiveness if pattern was involved
	for patternID, feedback := range review.PatternFeedback {
		if err := m.patternManager.UpdatePatternStats(ctx, patternID, feedback.WasMatch, feedback.WasFalsePositive); err != nil {
			fmt.Printf("Failed to update pattern stats for %s: %v\n", patternID, err)
		}
	}

	return nil
}

// Helper functions

func generateTextHash(text string) string {
	// Use SHA-256 to create a cryptographically secure hash of the text
	hash := sha256.Sum256([]byte(text))
	// Return hex-encoded hash for content identification and deduplication
	return hex.EncodeToString(hash[:])
}

// Types for moderation system

// ContentSubmission represents content submitted for moderation
type ContentSubmission struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"` // post/comment/message/profile
	Text        string         `json:"text,omitempty"`
	ImageURL    string         `json:"image_url,omitempty"`
	ImageBytes  []byte         `json:"-"`
	Author      string         `json:"author"`
	SubmittedAt time.Time      `json:"submitted_at"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// ModerationResult represents the result of content moderation
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific result
type ModerationResult struct {
	ContentID       string            `json:"content_id"`
	ContentType     string            `json:"content_type"`
	Action          string            `json:"action"` // allow/flag/hide/escalate/block
	Score           float64           `json:"score"`
	Confidence      float64           `json:"confidence"`
	Reasons         []string          `json:"reasons"`
	Recommendations []string          `json:"recommendations"`
	PatternMatches  []*PatternMatch   `json:"pattern_matches,omitempty"`
	AIAnalysis      *AIAnalysisResult `json:"ai_analysis,omitempty"`
	SubmittedAt     time.Time         `json:"submitted_at"`
	ProcessedAt     time.Time         `json:"processed_at"`
}

// AIAnalysisResult represents AI analysis results for moderation
type AIAnalysisResult struct {
	TextAnalysis  *TextAnalysis  `json:"text_analysis,omitempty"`
	ImageAnalysis *ImageAnalysis `json:"image_analysis,omitempty"`
}

// ModerationFilter represents filters for querying moderation results
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific filter
type ModerationFilter struct {
	Action      string    `json:"action,omitempty"`
	MinScore    float64   `json:"min_score,omitempty"`
	MaxScore    float64   `json:"max_score,omitempty"`
	StartTime   time.Time `json:"start_time,omitempty"`
	EndTime     time.Time `json:"end_time,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	Limit       int       `json:"limit,omitempty"`
}

// ModerationQueueItem represents an item in the moderation queue
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific queue item
type ModerationQueueItem struct {
	ContentID   string    `json:"content_id"`
	ContentType string    `json:"content_type"`
	Action      string    `json:"action"`
	Score       float64   `json:"score"`
	Confidence  float64   `json:"confidence"`
	FlaggedAt   time.Time `json:"flagged_at"`
	Priority    string    `json:"priority"`
}

// ModerationReview represents a human review of moderated content
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific review
type ModerationReview struct {
	ContentID       string                      `json:"content_id"`
	ReviewerID      string                      `json:"reviewer_id"`
	Decision        string                      `json:"decision"` // approve/reject/modify
	Action          string                      `json:"action,omitempty"`
	Comments        string                      `json:"comments,omitempty"`
	PatternFeedback map[string]*PatternFeedback `json:"pattern_feedback,omitempty"`
	ReviewedAt      time.Time                   `json:"reviewed_at"`
}

// PatternFeedback represents feedback on pattern match accuracy
type PatternFeedback struct {
	WasMatch         bool `json:"was_match"`
	WasFalsePositive bool `json:"was_false_positive"`
}
