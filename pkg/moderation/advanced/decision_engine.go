package advanced

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"go.uber.org/zap"
)

// DecisionEngine makes moderation decisions based on analysis results
type DecisionEngine struct {
	config           *ModerationConfig
	logger           *zap.Logger
	reputationScorer *ReputationScorer

	// Decision thresholds
	autoRemoveThreshold float64
	quarantineThreshold float64
	flagThreshold       float64
	shadowBanThreshold  float64
}

// NewDecisionEngine creates a new decision engine
func NewDecisionEngine(config *ModerationConfig, logger *zap.Logger, reputationScorer *ReputationScorer) *DecisionEngine {
	return &DecisionEngine{
		config:              config,
		logger:              logger,
		reputationScorer:    reputationScorer,
		autoRemoveThreshold: config.AutoRemoveThreshold,
		quarantineThreshold: config.QuarantineThreshold,
		flagThreshold:       config.FlagThreshold,
		shadowBanThreshold:  0.95, // Very high confidence for shadow banning
	}
}

// MakeDecision makes a moderation decision based on all analysis results
func (de *DecisionEngine) MakeDecision(ctx context.Context, analysis *ModerationAnalysis) (*ModerationDecision, error) {
	decision := &ModerationDecision{
		ContentID:       analysis.ContentMetadata.ContentID,
		Decision:        ActionAllow,
		Confidence:      0.0,
		Reasons:         []DecisionReason{},
		RequiresReview:  false,
		ReviewPriority:  0,
		Recommendations: []string{},
		DecidedAt:       time.Now(),
	}

	// Collect all signals
	signals := de.collectSignals(analysis)

	// Calculate weighted score
	totalScore, confidence := de.calculateWeightedScore(signals)
	decision.Confidence = confidence

	// Factor in reputation
	reputationMultiplier := 1.0
	if analysis.ReputationScore != nil {
		reputationMultiplier = de.calculateReputationMultiplier(analysis.ReputationScore)
		totalScore *= reputationMultiplier

		// Add reputation reason if it affected the decision
		if reputationMultiplier != 1.0 {
			decision.Reasons = append(decision.Reasons, DecisionReason{
				Type:        "reputation",
				Severity:    de.getReputationSeverity(analysis.ReputationScore),
				Description: fmt.Sprintf("User reputation: %s (score: %.1f)", analysis.ReputationScore.Level, analysis.ReputationScore.Score),
				Evidence:    analysis.ReputationScore,
			})
		}
	}

	// Make action decision based on score
	decision.Decision = de.determineAction(totalScore, signals)

	// Add specific reasons for the decision
	decision.Reasons = append(decision.Reasons, de.generateReasons(signals, analysis)...)

	// Sort reasons by severity
	de.sortReasonsBySeverity(decision.Reasons)

	// Set review requirements
	de.setReviewRequirements(decision, signals, totalScore)

	// Add recommendations
	decision.Recommendations = de.generateRecommendations(decision, analysis)

	// Set expiration for temporary actions
	if decision.Decision == ActionQuarantine || decision.Decision == ActionFlag {
		decision.ExpiresAt = time.Now().Add(24 * time.Hour)
	}

	// Log the decision
	de.logDecision(decision, analysis)

	return decision, nil
}

// Signal represents a moderation signal
type Signal struct {
	Type       string
	Severity   Severity
	Score      float64
	Confidence float64
	Evidence   interface{}
}

// collectSignals extracts all signals from the analysis
func (de *DecisionEngine) collectSignals(analysis *ModerationAnalysis) []Signal {
	signals := []Signal{}

	// Text analysis signals
	if analysis.TextAnalysis != nil {
		// Toxicity
		if analysis.TextAnalysis.Toxicity.IsToxic {
			signals = append(signals, Signal{
				Type:       "toxicity",
				Severity:   de.getToxicitySeverity(analysis.TextAnalysis.Toxicity.ToxicityScore),
				Score:      analysis.TextAnalysis.Toxicity.ToxicityScore,
				Confidence: analysis.TextAnalysis.Toxicity.Confidence,
				Evidence:   analysis.TextAnalysis.Toxicity,
			})
		}

		// Threats
		for _, threat := range analysis.TextAnalysis.Threats {
			signals = append(signals, Signal{
				Type:       "threat",
				Severity:   threat.Severity,
				Score:      de.getSeverityScore(threat.Severity),
				Confidence: threat.Confidence,
				Evidence:   threat,
			})
		}

		// PII
		if len(analysis.TextAnalysis.PII) > 0 {
			signals = append(signals, Signal{
				Type:       "pii",
				Severity:   SeverityMedium,
				Score:      0.7,
				Confidence: 0.9,
				Evidence:   analysis.TextAnalysis.PII,
			})
		}

		// Sentiment (extreme negative)
		if analysis.TextAnalysis.Sentiment.Negative > 0.9 {
			signals = append(signals, Signal{
				Type:       "extreme_negative_sentiment",
				Severity:   SeverityLow,
				Score:      analysis.TextAnalysis.Sentiment.Negative * 0.5,
				Confidence: analysis.TextAnalysis.Sentiment.Confidence,
				Evidence:   analysis.TextAnalysis.Sentiment,
			})
		}
	}

	// Image analysis signals
	if analysis.ImageAnalysis != nil {
		// Explicit content
		if analysis.ImageAnalysis.Explicit.IsExplicit {
			signals = append(signals, Signal{
				Type:       "explicit_content",
				Severity:   SeverityHigh,
				Score:      analysis.ImageAnalysis.Explicit.NudityScore,
				Confidence: analysis.ImageAnalysis.Explicit.Confidence,
				Evidence:   analysis.ImageAnalysis.Explicit,
			})
		}

		// Violence
		if analysis.ImageAnalysis.Violence.HasViolence {
			signals = append(signals, Signal{
				Type:       "violence",
				Severity:   SeverityHigh,
				Score:      analysis.ImageAnalysis.Violence.ViolenceScore,
				Confidence: analysis.ImageAnalysis.Violence.Confidence,
				Evidence:   analysis.ImageAnalysis.Violence,
			})
		}
	}

	// Pattern matches
	for _, match := range analysis.PatternMatches {
		signals = append(signals, Signal{
			Type:       "pattern_match",
			Severity:   SeverityMedium, // Would come from pattern definition
			Score:      0.8,
			Confidence: match.Confidence,
			Evidence:   match,
		})
	}

	// Threat matches
	for _, threat := range analysis.ThreatMatches {
		signals = append(signals, Signal{
			Type:       "threat_match",
			Severity:   SeverityHigh,
			Score:      0.9,
			Confidence: threat.Confidence,
			Evidence:   threat,
		})
	}

	return signals
}

// calculateWeightedScore calculates a weighted score from all signals
func (de *DecisionEngine) calculateWeightedScore(signals []Signal) (float64, float64) {
	if len(signals) == 0 {
		return 0.0, 1.0
	}

	totalWeight := 0.0
	weightedSum := 0.0
	confidenceSum := 0.0

	for _, signal := range signals {
		weight := de.getSignalWeight(signal)
		weightedSum += signal.Score * signal.Confidence * weight
		totalWeight += weight
		confidenceSum += signal.Confidence
	}

	if totalWeight == 0 {
		return 0.0, 0.0
	}

	score := weightedSum / totalWeight
	avgConfidence := confidenceSum / float64(len(signals))

	return score, avgConfidence
}

// getSignalWeight returns the weight for a signal type
func (de *DecisionEngine) getSignalWeight(signal Signal) float64 {
	// Base weights by type
	typeWeights := map[string]float64{
		"threat":                     3.0,
		"explicit_content":           2.5,
		"violence":                   2.5,
		"toxicity":                   2.0,
		"threat_match":               2.0,
		"pattern_match":              1.5,
		"pii":                        1.5,
		"extreme_negative_sentiment": 1.0,
	}

	weight := typeWeights[signal.Type]
	if weight == 0 {
		weight = 1.0
	}

	// Adjust by severity
	severityMultipliers := map[Severity]float64{
		SeverityCritical: 2.0,
		SeverityHigh:     1.5,
		SeverityMedium:   1.0,
		SeverityLow:      0.5,
	}

	if multiplier, ok := severityMultipliers[signal.Severity]; ok {
		weight *= multiplier
	}

	return weight
}

// calculateReputationMultiplier calculates how reputation affects the decision
func (de *DecisionEngine) calculateReputationMultiplier(reputation *ReputationScore) float64 {
	switch reputation.Level {
	case "trusted":
		return 0.7 // 30% reduction in severity for trusted users
	case "normal":
		return 1.0 // No change
	case "suspicious":
		return 1.3 // 30% increase in severity
	case "bad_actor":
		return 1.5 // 50% increase in severity
	default:
		return 1.0
	}
}

// determineAction determines the action based on the total score
func (de *DecisionEngine) determineAction(score float64, signals []Signal) ModerationAction {
	// Check for critical signals that warrant immediate action
	for _, signal := range signals {
		if signal.Severity == SeverityCritical && signal.Confidence > 0.8 {
			if signal.Type == "threat" && signal.Evidence != nil {
				if threat, ok := signal.Evidence.(ThreatIndicator); ok && threat.Type == "SELF_HARM" {
					return ActionFlag // Flag for immediate human review with resources
				}
			}
			return ActionRemove
		}
	}

	// Score-based decisions
	switch {
	case score >= de.autoRemoveThreshold:
		return ActionRemove
	case score >= de.shadowBanThreshold:
		return ActionShadowBan
	case score >= de.quarantineThreshold:
		return ActionQuarantine
	case score >= de.flagThreshold:
		return ActionFlag
	default:
		return ActionAllow
	}
}

// generateReasons generates detailed reasons for the decision
func (de *DecisionEngine) generateReasons(signals []Signal, analysis *ModerationAnalysis) []DecisionReason {
	reasons := []DecisionReason{}

	for _, signal := range signals {
		reason := DecisionReason{
			Type:     signal.Type,
			Severity: signal.Severity,
			Evidence: signal.Evidence,
		}

		// Generate description based on signal type
		switch signal.Type {
		case "toxicity":
			if toxicity, ok := signal.Evidence.(ToxicityAnalysis); ok {
				categories := []string{}
				for _, cat := range toxicity.Categories {
					categories = append(categories, cat.Category)
				}
				reason.Description = fmt.Sprintf("Toxic content detected (score: %.2f, categories: %v)",
					toxicity.ToxicityScore, categories)
			}

		case "threat":
			if threat, ok := signal.Evidence.(ThreatIndicator); ok {
				reason.Description = fmt.Sprintf("%s threat detected: %v",
					threat.Type, threat.Evidence)
			}

		case "explicit_content":
			reason.Description = fmt.Sprintf("Explicit content detected (nudity: %.2f)", signal.Score)

		case "violence":
			reason.Description = fmt.Sprintf("Violence detected (score: %.2f)", signal.Score)

		case "pattern_match":
			if match, ok := signal.Evidence.(PatternMatch); ok {
				reason.Description = fmt.Sprintf("Matched pattern '%s': %s",
					match.PatternName, match.MatchText)
			}

		case "pii":
			if piiList, ok := signal.Evidence.([]PIIEntity); ok {
				reason.Description = fmt.Sprintf("PII detected: %d entities", len(piiList))
			}
		}

		reasons = append(reasons, reason)
	}

	return reasons
}

// setReviewRequirements determines if human review is needed
func (de *DecisionEngine) setReviewRequirements(decision *ModerationDecision, signals []Signal, score float64) {
	// Always require review for certain actions
	if decision.Decision == ActionRemove || decision.Decision == ActionShadowBan {
		decision.RequiresReview = true
		decision.ReviewPriority = 8
	}

	// Require review for low confidence decisions
	if decision.Confidence < 0.7 && decision.Decision != ActionAllow {
		decision.RequiresReview = true
		decision.ReviewPriority = max(decision.ReviewPriority, 5)
	}

	// Require review for critical threats
	for _, signal := range signals {
		if signal.Type == "threat" && signal.Severity == SeverityCritical {
			decision.RequiresReview = true
			decision.ReviewPriority = 10
			break
		}
	}

	// Require review for borderline cases
	if math.Abs(score-de.flagThreshold) < 0.1 {
		decision.RequiresReview = true
		decision.ReviewPriority = max(decision.ReviewPriority, 3)
	}
}

// generateRecommendations generates actionable recommendations
func (de *DecisionEngine) generateRecommendations(decision *ModerationDecision, analysis *ModerationAnalysis) []string {
	recommendations := []string{}

	// Base recommendations on decision
	switch decision.Decision {
	case ActionRemove:
		recommendations = append(recommendations, "Content removed for policy violation")
		recommendations = append(recommendations, "Notify user with specific policy reference")

	case ActionQuarantine:
		recommendations = append(recommendations, "Content held for review")
		recommendations = append(recommendations, "Review within 24 hours")

	case ActionFlag:
		recommendations = append(recommendations, "Monitor user activity")
		recommendations = append(recommendations, "Review context and user history")

	case ActionShadowBan:
		recommendations = append(recommendations, "Limit content visibility")
		recommendations = append(recommendations, "Monitor for behavior change")
	}

	// Specific recommendations based on content
	for _, reason := range decision.Reasons {
		switch reason.Type {
		case "threat":
			if threat, ok := reason.Evidence.(ThreatIndicator); ok {
				switch threat.Type {
				case "SELF_HARM":
					recommendations = append(recommendations, "Provide mental health resources")
					recommendations = append(recommendations, "Consider wellness check")
				case "VIOLENCE":
					recommendations = append(recommendations, "Report to security team")
					recommendations = append(recommendations, "Document evidence")
				}
			}

		case "pii":
			recommendations = append(recommendations, "Redact personal information")
			recommendations = append(recommendations, "Warn user about privacy risks")

		case "explicit_content":
			recommendations = append(recommendations, "Apply content warning")
			recommendations = append(recommendations, "Restrict to adult audiences")
		}
	}

	// Reputation-based recommendations
	if analysis.ReputationScore != nil {
		switch analysis.ReputationScore.Level {
		case "bad_actor":
			recommendations = append(recommendations, "Consider account restrictions")
			recommendations = append(recommendations, "Review all recent content")

		case "suspicious":
			recommendations = append(recommendations, "Increase monitoring frequency")

		case "trusted":
			if decision.Decision != ActionAllow {
				recommendations = append(recommendations, "Double-check decision (trusted user)")
			}
		}
	}

	return recommendations
}

// Helper methods

func (de *DecisionEngine) getToxicitySeverity(score float64) Severity {
	switch {
	case score >= 0.9:
		return SeverityHigh
	case score >= 0.7:
		return SeverityMedium
	case score >= 0.5:
		return SeverityLow
	default:
		return SeverityLow
	}
}

func (de *DecisionEngine) getSeverityScore(severity Severity) float64 {
	scores := map[Severity]float64{
		SeverityCritical: 0.95,
		SeverityHigh:     0.8,
		SeverityMedium:   0.6,
		SeverityLow:      0.4,
	}

	if score, ok := scores[severity]; ok {
		return score
	}
	return 0.5
}

func (de *DecisionEngine) getReputationSeverity(reputation *ReputationScore) Severity {
	switch reputation.Level {
	case "bad_actor":
		return SeverityHigh
	case "suspicious":
		return SeverityMedium
	default:
		return SeverityLow
	}
}

func (de *DecisionEngine) sortReasonsBySeverity(reasons []DecisionReason) {
	severityOrder := map[Severity]int{
		SeverityCritical: 4,
		SeverityHigh:     3,
		SeverityMedium:   2,
		SeverityLow:      1,
	}

	sort.Slice(reasons, func(i, j int) bool {
		return severityOrder[reasons[i].Severity] > severityOrder[reasons[j].Severity]
	})
}

func (de *DecisionEngine) logDecision(decision *ModerationDecision, analysis *ModerationAnalysis) {
	de.logger.Info("moderation decision made",
		zap.String("contentID", decision.ContentID),
		zap.String("decision", string(decision.Decision)),
		zap.Float64("confidence", decision.Confidence),
		zap.Int("reasonCount", len(decision.Reasons)),
		zap.Bool("requiresReview", decision.RequiresReview),
		zap.String("authorID", analysis.ContentMetadata.AuthorID))

	// Log high-severity decisions with more detail
	if decision.Decision == ActionRemove || decision.Decision == ActionShadowBan {
		for _, reason := range decision.Reasons {
			de.logger.Warn("high-severity reason",
				zap.String("type", reason.Type),
				zap.String("severity", string(reason.Severity)),
				zap.String("description", reason.Description))
		}
	}
}
