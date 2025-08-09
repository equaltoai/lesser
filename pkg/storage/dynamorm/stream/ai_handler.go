package stream

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ModerationRepository defines the interface for moderation actions
type ModerationRepository interface {
	CreateFlag(ctx context.Context, flag *storage.Flag) error
	CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error
	CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error
}

// AIAnalysisStreamHandler processes DynamoDB stream events for AI analysis results
type AIAnalysisStreamHandler struct {
	logger           *zap.Logger
	moderationRepo   ModerationRepository
}

// NewAIAnalysisStreamHandler creates a new AI analysis stream handler
func NewAIAnalysisStreamHandler(logger *zap.Logger, moderationRepo ModerationRepository) *AIAnalysisStreamHandler {
	return &AIAnalysisStreamHandler{
		logger:         logger,
		moderationRepo: moderationRepo,
	}
}

// HandleStreamEvent processes a DynamoDB stream event for AI analysis
func (h *AIAnalysisStreamHandler) HandleStreamEvent(_ context.Context, record events.DynamoDBEventRecord) error {
	// Check if this is an AI analysis record
	if !h.isAIAnalysisRecord(record) {
		return nil // Not an AI analysis record, skip
	}

	switch record.EventName {
	case "INSERT":
		return h.handleAIAnalysisCreated(record)
	case "MODIFY":
		return h.handleAIAnalysisUpdated(record)
	case "REMOVE":
		return h.handleAIAnalysisDeleted(record)
	default:
		h.logger.Warn("Unknown event type for AI analysis stream",
			zap.String("eventName", record.EventName),
			zap.String("eventID", record.EventID))
		return nil
	}
}

// isAIAnalysisRecord checks if the stream record is for an AI analysis
func (h *AIAnalysisStreamHandler) isAIAnalysisRecord(record events.DynamoDBEventRecord) bool {
	// Check the primary key to see if it's an AI analysis record
	var pk string
	if record.Change.NewImage != nil {
		if pkAttr, exists := record.Change.NewImage["PK"]; exists {
			pk = pkAttr.String()
		}
	} else if record.Change.OldImage != nil {
		if pkAttr, exists := record.Change.OldImage["PK"]; exists {
			pk = pkAttr.String()
		}
	}

	// AI analysis records have PK format: "AI#{object_id}"
	return strings.HasPrefix(pk, "AI#") && !strings.HasPrefix(pk, "AI#ANALYSIS#")
}

// handleAIAnalysisCreated processes new AI analysis results
func (h *AIAnalysisStreamHandler) handleAIAnalysisCreated(record events.DynamoDBEventRecord) error {
	h.logger.Info("Processing new AI analysis result",
		zap.String("eventID", record.EventID))

	// Unmarshal the AI analysis record
	var analysis models.AIAnalysis
	if err := UnmarshalItem(record, &analysis); err != nil {
		h.logger.Error("Failed to unmarshal AI analysis record",
			zap.String("eventID", record.EventID),
			zap.Error(err))
		return fmt.Errorf("failed to unmarshal AI analysis: %w", err)
	}

	// Process the analysis results
	return h.processAnalysisResult(&analysis, "created")
}

// handleAIAnalysisUpdated processes updated AI analysis results
func (h *AIAnalysisStreamHandler) handleAIAnalysisUpdated(record events.DynamoDBEventRecord) error {
	h.logger.Info("Processing updated AI analysis result",
		zap.String("eventID", record.EventID))

	// Unmarshal the AI analysis record
	var analysis models.AIAnalysis
	if err := UnmarshalItem(record, &analysis); err != nil {
		h.logger.Error("Failed to unmarshal AI analysis record",
			zap.String("eventID", record.EventID),
			zap.Error(err))
		return fmt.Errorf("failed to unmarshal AI analysis: %w", err)
	}

	// Process the updated analysis results
	return h.processAnalysisResult(&analysis, "updated")
}

// handleAIAnalysisDeleted processes deleted AI analysis results
func (h *AIAnalysisStreamHandler) handleAIAnalysisDeleted(record events.DynamoDBEventRecord) error {
	h.logger.Info("Processing deleted AI analysis result",
		zap.String("eventID", record.EventID))

	// For deletions, we might want to clean up related data
	// For now, just log the event
	return nil
}

// processAnalysisResult processes the AI analysis result
func (h *AIAnalysisStreamHandler) processAnalysisResult(analysis *models.AIAnalysis, action string) error {
	h.logger.Info("Processing AI analysis result",
		zap.String("analysisID", analysis.ID),
		zap.String("objectID", analysis.ObjectID),
		zap.String("objectType", analysis.ObjectType),
		zap.String("moderationAction", analysis.ModerationAction),
		zap.Float64("overallRisk", analysis.OverallRisk),
		zap.Float64("confidence", analysis.Confidence),
		zap.String("action", action))

	// Here you would typically:
	// 1. Update the analysis request status to completed
	// 2. Apply moderation actions if necessary
	// 3. Send notifications to interested parties
	// 4. Update metrics and analytics
	
	// For high-risk content, take immediate action
	if analysis.OverallRisk > 0.8 && analysis.ModerationAction != "none" {
		h.logger.Warn("High-risk content detected",
			zap.String("objectID", analysis.ObjectID),
			zap.String("moderationAction", analysis.ModerationAction),
			zap.Float64("risk", analysis.OverallRisk))
		
		// Apply automatic moderation actions based on analysis results
		if err := h.applyModerationAction(context.Background(), analysis); err != nil {
			h.logger.Error("Failed to apply automatic moderation action",
				zap.String("objectID", analysis.ObjectID),
				zap.String("action", analysis.ModerationAction),
				zap.Error(err))
		}
	}

	// Log analysis statistics for monitoring
	h.logAnalysisStats(analysis)

	return nil
}

// logAnalysisStats logs detailed analysis statistics
func (h *AIAnalysisStreamHandler) logAnalysisStats(analysis *models.AIAnalysis) {
	fields := []zap.Field{
		zap.String("analysisID", analysis.ID),
		zap.String("objectID", analysis.ObjectID),
		zap.String("objectType", analysis.ObjectType),
		zap.Float64("overallRisk", analysis.OverallRisk),
		zap.Float64("confidence", analysis.Confidence),
		zap.String("moderationAction", analysis.ModerationAction),
	}

	// Add text analysis stats if available
	if analysis.TextAnalysis != nil {
		fields = append(fields,
			zap.Float64("toxicityScore", analysis.TextAnalysis.ToxicityScore),
			zap.String("sentiment", analysis.TextAnalysis.Sentiment),
			zap.Bool("containsPII", analysis.TextAnalysis.ContainsPII),
			zap.String("dominantLanguage", analysis.TextAnalysis.DominantLanguage),
		)
	}

	// Add image analysis stats if available
	if analysis.ImageAnalysis != nil {
		fields = append(fields,
			zap.Bool("isNSFW", analysis.ImageAnalysis.IsNSFW),
			zap.Float64("nsfwConfidence", analysis.ImageAnalysis.NSFWConfidence),
			zap.Float64("violenceScore", analysis.ImageAnalysis.ViolenceScore),
			zap.Bool("weaponsDetected", analysis.ImageAnalysis.WeaponsDetected),
		)
	}

	// Add spam analysis stats if available
	if analysis.SpamAnalysis != nil {
		fields = append(fields,
			zap.Float64("spamScore", analysis.SpamAnalysis.SpamScore),
			zap.Float64("linkDensity", analysis.SpamAnalysis.LinkDensity),
			zap.Float64("repetitionScore", analysis.SpamAnalysis.RepetitionScore),
		)
	}

	// Add AI detection stats if available
	if analysis.AIDetection != nil {
		fields = append(fields,
			zap.Float64("aiGeneratedProbability", analysis.AIDetection.AIGeneratedProbability),
			zap.String("generationModel", analysis.AIDetection.GenerationModel),
			zap.Float64("semanticCoherence", analysis.AIDetection.SemanticCoherence),
		)
	}

	h.logger.Info("AI analysis statistics", fields...)
}

// applyModerationAction applies the recommended moderation action based on AI analysis
func (h *AIAnalysisStreamHandler) applyModerationAction(ctx context.Context, analysis *models.AIAnalysis) error {
	// Get appropriate thresholds based on content type
	thresholds := ai.GetThresholds(strings.ToLower(analysis.ObjectType))
	
	// Determine action based on overall risk and specific analysis results
	action := h.determineModerationAction(analysis, thresholds)
	
	if action == ai.ActionNone {
		return nil // No action needed
	}
	
	// Create moderation event to record the AI decision
	moderationEvent := &storage.ModerationEvent{
		EventType:       "ai_moderation",
		ObjectID:        analysis.ObjectID,
		ObjectType:      analysis.ObjectType,
		ActorID:         "ai_system", // Automated action
		Category:        h.determineCategory(analysis),
		Severity:        h.determineSeverity(analysis.OverallRisk),
		ConfidenceScore: analysis.Confidence,
		Evidence:        h.buildEvidenceSlice(analysis),
		Reason:          fmt.Sprintf("AI analysis detected risk score of %.2f with action: %s", analysis.OverallRisk, action),
	}
	
	// Create the moderation event
	if err := h.moderationRepo.CreateModerationEvent(ctx, moderationEvent); err != nil {
		h.logger.Error("Failed to create moderation event",
			zap.String("objectID", analysis.ObjectID),
			zap.Error(err))
		return fmt.Errorf("failed to create moderation event: %w", err)
	}
	
	// Apply specific action based on the decision
	switch action {
	case ai.ActionFlag:
		return h.flagContent(ctx, analysis, "AI detected potentially problematic content")
	case ai.ActionHide:
		return h.hideContent(ctx, analysis, "AI detected high-risk content")
	case ai.ActionRemove:
		return h.removeContent(ctx, analysis, "AI detected very high-risk content")
	case ai.ActionReview:
		return h.flagForReview(ctx, analysis, "AI flagged content for human review")
	default:
		h.logger.Debug("No automatic action taken",
			zap.String("objectID", analysis.ObjectID),
			zap.String("action", action))
		return nil
	}
}

// determineModerationAction determines the appropriate moderation action
func (h *AIAnalysisStreamHandler) determineModerationAction(analysis *models.AIAnalysis, thresholds ai.ThresholdSet) string {
	risk := analysis.OverallRisk
	
	// Check for immediate removal conditions
	if h.shouldAutoRemove(analysis, thresholds) {
		return ai.ActionRemove
	}
	
	// Check for hiding conditions
	if h.shouldAutoHide(analysis, thresholds) {
		return ai.ActionHide
	}
	
	// Check for flagging conditions
	if risk >= thresholds.Flag {
		return ai.ActionFlag
	}
	
	// Check for review conditions
	if risk >= thresholds.WarnAuthor {
		return ai.ActionReview
	}
	
	return ai.ActionNone
}

// shouldAutoRemove checks if content should be automatically removed
func (h *AIAnalysisStreamHandler) shouldAutoRemove(analysis *models.AIAnalysis, thresholds ai.ThresholdSet) bool {
	// Very high overall risk
	if analysis.OverallRisk >= thresholds.AutoRemove {
		return true
	}
	
	// Specific high-confidence violations
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW && 
		analysis.ImageAnalysis.NSFWConfidence > 0.95 {
		return true
	}
	
	if analysis.TextAnalysis != nil && analysis.TextAnalysis.ToxicityScore > 0.9 &&
		analysis.Confidence > 0.8 {
		return true
	}
	
	return false
}

// shouldAutoHide checks if content should be automatically hidden
func (h *AIAnalysisStreamHandler) shouldAutoHide(analysis *models.AIAnalysis, thresholds ai.ThresholdSet) bool {
	// High overall risk
	if analysis.OverallRisk >= thresholds.AutoHide {
		return true
	}
	
	// Specific moderate-confidence violations
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW && 
		analysis.ImageAnalysis.NSFWConfidence > 0.8 {
		return true
	}
	
	if analysis.SpamAnalysis != nil && analysis.SpamAnalysis.SpamScore > 0.8 {
		return true
	}
	
	return false
}

// flagContent creates a flag for the content
func (h *AIAnalysisStreamHandler) flagContent(ctx context.Context, analysis *models.AIAnalysis, reason string) error {
	flag := &storage.Flag{
		ContentID:    analysis.ObjectID,
		FlaggerID:    "ai_system",
		Reason:       reason,
		Status:       string(storage.FlagStatusPending),
		Actor:        "ai_system",
		Content:      fmt.Sprintf("AI moderation: %s (confidence: %.2f)", reason, analysis.Confidence),
	}
	
	if err := h.moderationRepo.CreateFlag(ctx, flag); err != nil {
		return fmt.Errorf("failed to flag content: %w", err)
	}
	
	h.logger.Info("Content flagged by AI",
		zap.String("objectID", analysis.ObjectID),
		zap.String("reason", reason))
	
	return nil
}

// hideContent creates a moderation decision to hide the content
func (h *AIAnalysisStreamHandler) hideContent(ctx context.Context, analysis *models.AIAnalysis, reason string) error {
	decision := &storage.ModerationDecision{
		ObjectID:      analysis.ObjectID,
		DeciderID:     "ai_system",
		Decision:      "hide",
		Action:        "hide_content",
		Reason:        reason,
		Appeal:        true,
		Decided:       time.Now(),
		ConsensusScore: analysis.Confidence,
		ReviewerCount: 1,
		Metadata:      h.buildEvidence(analysis),
	}
	
	if err := h.moderationRepo.CreateModerationDecision(ctx, decision); err != nil {
		return fmt.Errorf("failed to hide content: %w", err)
	}
	
	h.logger.Warn("Content hidden by AI",
		zap.String("objectID", analysis.ObjectID),
		zap.String("reason", reason))
	
	return nil
}

// removeContent creates a moderation decision to remove the content
func (h *AIAnalysisStreamHandler) removeContent(ctx context.Context, analysis *models.AIAnalysis, reason string) error {
	decision := &storage.ModerationDecision{
		ObjectID:      analysis.ObjectID,
		DeciderID:     "ai_system",
		Decision:      "delete",
		Action:        "remove_content",
		Reason:        reason,
		Appeal:        true,
		Decided:       time.Now(),
		ConsensusScore: analysis.Confidence,
		ReviewerCount: 1,
		Metadata:      h.buildEvidence(analysis),
	}
	
	if err := h.moderationRepo.CreateModerationDecision(ctx, decision); err != nil {
		return fmt.Errorf("failed to remove content: %w", err)
	}
	
	h.logger.Error("Content removed by AI",
		zap.String("objectID", analysis.ObjectID),
		zap.String("reason", reason))
	
	return nil
}

// flagForReview creates a flag for human review
func (h *AIAnalysisStreamHandler) flagForReview(ctx context.Context, analysis *models.AIAnalysis, reason string) error {
	flag := &storage.Flag{
		ContentID:    analysis.ObjectID,
		FlaggerID:    "ai_system",
		Reason:       reason,
		Status:       string(storage.FlagStatusPending),
		Actor:        "ai_system",
		Content:      fmt.Sprintf("AI moderation for review: %s (confidence: %.2f)", reason, analysis.Confidence),
	}
	
	if err := h.moderationRepo.CreateFlag(ctx, flag); err != nil {
		return fmt.Errorf("failed to flag for review: %w", err)
	}
	
	h.logger.Info("Content flagged for human review",
		zap.String("objectID", analysis.ObjectID))
	
	return nil
}

// determineCategory determines the moderation category based on analysis
func (h *AIAnalysisStreamHandler) determineCategory(analysis *models.AIAnalysis) string {
	// Prioritize the most severe category detected
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW {
		return "nsfw"
	}
	
	if analysis.TextAnalysis != nil && analysis.TextAnalysis.ToxicityScore > 0.5 {
		return "toxicity"
	}
	
	if analysis.SpamAnalysis != nil && analysis.SpamAnalysis.SpamScore > 0.5 {
		return "spam"
	}
	
	if analysis.AIDetection != nil && analysis.AIDetection.AIGeneratedProbability > 0.7 {
		return "ai_generated"
	}
	
	return "general"
}

// determineSeverity determines severity level based on risk score
func (h *AIAnalysisStreamHandler) determineSeverity(riskScore float64) string {
	switch {
	case riskScore >= 0.9:
		return "critical"
	case riskScore >= 0.7:
		return "high"
	case riskScore >= 0.5:
		return "medium"
	default:
		return "low"
	}
}

// buildEvidence builds a structured evidence object from analysis results
func (h *AIAnalysisStreamHandler) buildEvidence(analysis *models.AIAnalysis) map[string]interface{} {
	evidence := map[string]interface{}{
		"overall_risk": analysis.OverallRisk,
		"confidence":   analysis.Confidence,
		"analysis_id":  analysis.ID,
	}
	
	if analysis.TextAnalysis != nil {
		evidence["text_analysis"] = map[string]interface{}{
			"toxicity_score": analysis.TextAnalysis.ToxicityScore,
			"sentiment":      analysis.TextAnalysis.Sentiment,
			"contains_pii":   analysis.TextAnalysis.ContainsPII,
		}
	}
	
	if analysis.ImageAnalysis != nil {
		evidence["image_analysis"] = map[string]interface{}{
			"is_nsfw":           analysis.ImageAnalysis.IsNSFW,
			"nsfw_confidence":   analysis.ImageAnalysis.NSFWConfidence,
			"violence_score":    analysis.ImageAnalysis.ViolenceScore,
			"weapons_detected":  analysis.ImageAnalysis.WeaponsDetected,
		}
	}
	
	if analysis.SpamAnalysis != nil {
		evidence["spam_analysis"] = map[string]interface{}{
			"spam_score":       analysis.SpamAnalysis.SpamScore,
			"link_density":     analysis.SpamAnalysis.LinkDensity,
			"repetition_score": analysis.SpamAnalysis.RepetitionScore,
		}
	}
	
	if analysis.AIDetection != nil {
		evidence["ai_detection"] = map[string]interface{}{
			"ai_probability":      analysis.AIDetection.AIGeneratedProbability,
			"generation_model":    analysis.AIDetection.GenerationModel,
			"semantic_coherence":  analysis.AIDetection.SemanticCoherence,
		}
	}
	
	return evidence
}

// buildEvidenceSlice builds a structured evidence slice from analysis results
func (h *AIAnalysisStreamHandler) buildEvidenceSlice(analysis *models.AIAnalysis) []any {
	evidence := []any{
		map[string]interface{}{
			"type":         "overall_analysis",
			"overall_risk": analysis.OverallRisk,
			"confidence":   analysis.Confidence,
			"analysis_id":  analysis.ID,
		},
	}
	
	if analysis.TextAnalysis != nil {
		evidence = append(evidence, map[string]interface{}{
			"type":           "text_analysis",
			"toxicity_score": analysis.TextAnalysis.ToxicityScore,
			"sentiment":      analysis.TextAnalysis.Sentiment,
			"contains_pii":   analysis.TextAnalysis.ContainsPII,
		})
	}
	
	if analysis.ImageAnalysis != nil {
		evidence = append(evidence, map[string]interface{}{
			"type":             "image_analysis",
			"is_nsfw":          analysis.ImageAnalysis.IsNSFW,
			"nsfw_confidence":  analysis.ImageAnalysis.NSFWConfidence,
			"violence_score":   analysis.ImageAnalysis.ViolenceScore,
			"weapons_detected": analysis.ImageAnalysis.WeaponsDetected,
		})
	}
	
	if analysis.SpamAnalysis != nil {
		evidence = append(evidence, map[string]interface{}{
			"type":             "spam_analysis",
			"spam_score":       analysis.SpamAnalysis.SpamScore,
			"link_density":     analysis.SpamAnalysis.LinkDensity,
			"repetition_score": analysis.SpamAnalysis.RepetitionScore,
		})
	}
	
	if analysis.AIDetection != nil {
		evidence = append(evidence, map[string]interface{}{
			"type":               "ai_detection",
			"ai_probability":     analysis.AIDetection.AIGeneratedProbability,
			"generation_model":   analysis.AIDetection.GenerationModel,
			"semantic_coherence": analysis.AIDetection.SemanticCoherence,
		})
	}
	
	return evidence
}

// ProcessStreamRecords processes multiple AI analysis stream records
func (h *AIAnalysisStreamHandler) ProcessStreamRecords(ctx context.Context, records []events.DynamoDBEventRecord) error {
	for _, record := range records {
		if err := h.HandleStreamEvent(ctx, record); err != nil {
			h.logger.Error("Failed to process AI analysis stream record",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Error(err))
			
			// Continue processing other records even if one fails
			continue
		}
	}
	return nil
}