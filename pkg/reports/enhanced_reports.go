package reports

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aron23/lesser/pkg/moderation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/trust"
	"go.uber.org/zap"
)

// EnhancedReportService provides advanced report handling with trust integration
type EnhancedReportService struct {
	store  storage.Storage
	logger *zap.Logger
}

// NewEnhancedReportService creates a new enhanced report service
func NewEnhancedReportService(store storage.Storage, logger *zap.Logger) *EnhancedReportService {
	return &EnhancedReportService{
		store:  store,
		logger: logger,
	}
}

// ReporterReliability represents a reporter's reliability metrics
type ReporterReliability struct {
	Username         string    `json:"username"`
	TotalReports     int       `json:"total_reports"`
	ValidReports     int       `json:"valid_reports"`
	FalseReports     int       `json:"false_reports"`
	PendingReports   int       `json:"pending_reports"`
	ReliabilityScore float64   `json:"reliability_score"`
	TrustModifier    float64   `json:"trust_modifier"`
	LastCalculated   time.Time `json:"last_calculated"`
}

// CalculateReporterReliability calculates the reliability score for a reporter
func (s *EnhancedReportService) CalculateReporterReliability(ctx context.Context, username string) (*ReporterReliability, error) {
	// Get report stats
	stats, err := s.store.GetReportStats(ctx, username)
	if err != nil {
		s.logger.Error("failed to get report stats", zap.String("username", username), zap.Error(err))
		stats = &storage.ReportStats{} // Use empty stats if error
	}

	// Calculate valid reports (resolved - false reports)
	validReports := stats.ResolvedReports - stats.FalseReports
	if validReports < 0 {
		validReports = 0
	}

	// Calculate pending reports
	pendingReports := stats.TotalReports - stats.ResolvedReports

	// Calculate reliability score (0.0 to 1.0)
	var reliabilityScore float64
	if stats.TotalReports == 0 {
		// New reporter - start with neutral score
		reliabilityScore = 0.5
	} else if stats.ResolvedReports == 0 {
		// No resolved reports yet
		reliabilityScore = 0.5
	} else {
		// Calculate based on accuracy
		accuracy := float64(validReports) / float64(stats.ResolvedReports)

		// Apply logarithmic scaling to reward consistent reporters
		reportCount := math.Log10(float64(stats.ResolvedReports) + 1)
		weightedAccuracy := accuracy * (0.5 + 0.5*math.Min(reportCount/2, 1))

		// Penalize false reports more heavily
		falsePenalty := math.Pow(float64(stats.FalseReports)/float64(stats.TotalReports+1), 2)
		reliabilityScore = weightedAccuracy - falsePenalty

		// Clamp to valid range
		reliabilityScore = math.Max(0.1, math.Min(0.95, reliabilityScore))
	}

	// Calculate trust modifier for use in moderation events
	// Maps reliability score to trust weight modifier (0.5 to 1.5)
	trustModifier := 0.5 + reliabilityScore

	return &ReporterReliability{
		Username:         username,
		TotalReports:     stats.TotalReports,
		ValidReports:     validReports,
		FalseReports:     stats.FalseReports,
		PendingReports:   pendingReports,
		ReliabilityScore: reliabilityScore,
		TrustModifier:    trustModifier,
		LastCalculated:   time.Now(),
	}, nil
}

// CreateEnhancedModerationEvent creates a moderation event with reporter trust weighting
func (s *EnhancedReportService) CreateEnhancedModerationEvent(ctx context.Context, report *storage.Report, reporterActorID string) (*moderation.ModerationEvent, error) {
	// Calculate reporter reliability
	reliability, err := s.CalculateReporterReliability(ctx, report.ReporterID)
	if err != nil {
		s.logger.Warn("failed to calculate reporter reliability", zap.Error(err))
		reliability = &ReporterReliability{ReliabilityScore: 0.5, TrustModifier: 1.0}
	}

	// Get reporter's trust score in content moderation
	trustScore, err := s.store.GetTrustScore(ctx, reporterActorID, string(trust.TrustCategoryContent))
	if err != nil {
		s.logger.Warn("failed to get reporter trust score", zap.Error(err))
	}

	// Combine reliability and trust for final confidence
	baseConfidence := 0.7 // Base confidence for user reports
	if trustScore != nil {
		baseConfidence = (baseConfidence + trustScore.Score) / 2
	}
	finalConfidence := baseConfidence * reliability.TrustModifier

	// Determine object type and ID
	objectType := "Actor"
	objectID := report.TargetAccountID
	if len(report.StatusIDs) > 0 {
		objectType = "Note"
		objectID = report.StatusIDs[0]
	}

	// Map report category to moderation category
	var modCategory moderation.Category
	switch report.Category {
	case "spam":
		modCategory = moderation.CategorySpam
	case "violation":
		modCategory = moderation.CategoryOther
	default:
		modCategory = moderation.CategoryOther
	}

	// Determine severity based on report details
	severity := moderation.SeverityMedium
	if len(report.RuleIDs) > 0 {
		severity = moderation.SeverityHigh // Rule violations are more serious
	}

	// Create the enhanced moderation event
	now := time.Now()
	event := &moderation.ModerationEvent{
		ID:              fmt.Sprintf("mod-report-%s-%d", report.ID, now.Unix()),
		EventType:       moderation.EventTypeFlagged,
		ObjectID:        objectID,
		ObjectType:      objectType,
		ActorID:         reporterActorID,
		Category:        modCategory,
		Severity:        severity,
		ConfidenceScore: finalConfidence,
		Evidence: []moderation.Evidence{
			{
				Type:        "user_report",
				Score:       finalConfidence,
				Description: report.Comment,
				Metadata: map[string]interface{}{
					"report_id":            report.ID,
					"reporter_username":    report.ReporterID,
					"reporter_reliability": reliability.ReliabilityScore,
					"trust_modifier":       reliability.TrustModifier,
					"rule_violations":      report.RuleIDs,
					"forwarded":            report.Forwarded,
				},
				Timestamp: now,
			},
		},
		Reason:  report.Comment,
		Created: now,
		Updated: now,
		TTL:     now.Add(90 * 24 * time.Hour).Unix(), // 90 day TTL
	}

	// Create the moderation event
	if err := s.store.CreateModerationEvent(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to create moderation event: %w", err)
	}

	// Log the enhanced report creation
	s.logger.Info("created enhanced moderation event for report",
		zap.String("report_id", report.ID),
		zap.String("event_id", event.ID),
		zap.Float64("reporter_reliability", reliability.ReliabilityScore),
		zap.Float64("final_confidence", finalConfidence))

	return event, nil
}

// UpdateReporterTrustOnDecision updates reporter's trust score based on moderation decision
func (s *EnhancedReportService) UpdateReporterTrustOnDecision(ctx context.Context, reportID string, decision *moderation.ModerationDecision, reporterActorID string) error {
	// Get the report
	report, err := s.store.GetReport(ctx, reportID)
	if err != nil {
		return fmt.Errorf("failed to get report: %w", err)
	}

	// Determine if the report was valid based on the decision
	reportValid := false
	switch decision.Action {
	case moderation.ActionTypeRemove, moderation.ActionTypeSuspend, moderation.ActionTypeSilence, moderation.ActionTypeWarning:
		reportValid = true
	case moderation.ActionTypeNone:
		reportValid = false
	}

	// Calculate trust adjustment
	var trustDelta float64
	if reportValid {
		// Increase trust for valid reports
		// Higher consensus = larger increase
		trustDelta = 0.05 * decision.ConsensusScore

		// Bonus for high-severity issues caught
		for _, review := range decision.Reviews {
			if review.Severity >= moderation.SeverityHigh {
				trustDelta += 0.02
				break
			}
		}
	} else {
		// Decrease trust for false reports
		// Lower consensus on "no action" = smaller decrease (uncertain case)
		trustDelta = -0.1 * decision.ConsensusScore

		// Heavier penalty for obviously false reports (high consensus on no action)
		if decision.ConsensusScore > 0.8 {
			trustDelta = -0.15
		}
	}

	// Update trust relationship
	trustRel := &trust.TrustRelationship{
		TrusterID:  "system",
		TrusteeID:  reporterActorID,
		Category:   trust.TrustCategoryContent,
		Score:      0.5, // Will be adjusted by delta
		Confidence: decision.ConsensusScore,
		Updated:    time.Now(),
	}

	// Get current trust score
	currentScore, err := s.store.GetTrustScore(ctx, reporterActorID, string(trust.TrustCategoryContent))
	if err == nil && currentScore != nil {
		trustRel.Score = currentScore.Score
	}

	// Apply delta
	trustRel.Score += trustDelta
	trustRel.Score = math.Max(0.0, math.Min(1.0, trustRel.Score)) // Clamp to 0-1

	// Update trust
	if err := s.store.CreateTrustRelationship(ctx, trustRel); err != nil {
		s.logger.Error("failed to update reporter trust", zap.Error(err))
		return err
	}

	// Update report stats
	updates := make(map[string]interface{})
	if reportValid {
		updates["action_taken"] = string(decision.Action)
	} else {
		// Track as false report
		if err := s.incrementFalseReports(ctx, report.ReporterID); err != nil {
			s.logger.Warn("failed to increment false reports", zap.Error(err))
		}
	}
	updates["moderation_decision_id"] = decision.ID

	// Update report with decision
	if err := s.store.UpdateReportStatus(ctx, reportID, storage.ReportStatusResolved, string(decision.Action), "system"); err != nil {
		s.logger.Error("failed to update report status", zap.Error(err))
	}

	s.logger.Info("updated reporter trust based on decision",
		zap.String("reporter", report.ReporterID),
		zap.Bool("report_valid", reportValid),
		zap.Float64("trust_delta", trustDelta),
		zap.Float64("new_trust_score", trustRel.Score))

	return nil
}

// incrementFalseReports increments the false report count for a user
func (s *EnhancedReportService) incrementFalseReports(ctx context.Context, username string) error {
	return s.store.IncrementFalseReports(ctx, username)
}

// GetReportModerationStatus gets the moderation status of a report
func (s *EnhancedReportService) GetReportModerationStatus(ctx context.Context, reportID string) (*ReportModerationStatus, error) {
	report, err := s.store.GetReport(ctx, reportID)
	if err != nil {
		return nil, err
	}

	status := &ReportModerationStatus{
		ReportID:          reportID,
		ModerationEventID: report.ModerationEventID,
		Status:            string(report.Status),
	}

	if report.ModerationEventID != "" {
		// Get moderation event details
		event, err := s.store.GetModerationEvent(ctx, report.ModerationEventID)
		if err == nil {
			status.ModerationStatus = string(event.EventType)
			status.ConsensusReached = false

			// Check for decision
			decision, err := s.store.GetModerationDecision(ctx, event.ObjectID)
			if err == nil && decision != nil {
				status.ConsensusReached = true
				status.ConsensusScore = decision.ConsensusScore
				status.Decision = string(decision.Action)
			}
		}
	}

	return status, nil
}

// ReportModerationStatus represents the current moderation status of a report
type ReportModerationStatus struct {
	ReportID          string  `json:"report_id"`
	ModerationEventID string  `json:"moderation_event_id,omitempty"`
	Status            string  `json:"status"`
	ModerationStatus  string  `json:"moderation_status,omitempty"`
	ConsensusReached  bool    `json:"consensus_reached"`
	ConsensusScore    float64 `json:"consensus_score,omitempty"`
	Decision          string  `json:"decision,omitempty"`
}
