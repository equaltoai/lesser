package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/trust"
)

// StorageInterface defines the storage operations needed by the consensus engine
type StorageInterface interface {
	GetModerationEvent(ctx context.Context, eventID string) (*ModerationEvent, error)
	AddModerationReview(ctx context.Context, review *Review) error
	GetModerationReviews(ctx context.Context, eventID string) ([]*Review, error)
	CreateModerationDecision(ctx context.Context, decision *ModerationDecision) error
	GetModerationQueue(ctx context.Context, limit int, cursor string) ([]*QueueItem, string, error)
	GetTrustScore(ctx context.Context, actorID, category string) (*trust.TrustScore, error)
	RecordTrustUpdate(ctx context.Context, update *trust.TrustUpdate) error
}

// ConsensusEngine handles consensus calculation for moderation decisions
type ConsensusEngine struct {
	storage StorageInterface
	config  *ConsensusConfig
}

// NewConsensusEngine creates a new consensus engine
func NewConsensusEngine(storage StorageInterface, config *ConsensusConfig) *ConsensusEngine {
	if config == nil {
		config = DefaultConsensusConfig()
	}
	return &ConsensusEngine{
		storage: storage,
		config:  config,
	}
}

// CalculateConsensus calculates consensus from reviews with trust weighting
func (e *ConsensusEngine) CalculateConsensus(ctx context.Context, event *ModerationEvent, reviews []*Review) (*ModerationDecision, error) {
	if len(reviews) < e.config.MinReviewers {
		return nil, fmt.Errorf("insufficient reviewers: %d < %d", len(reviews), e.config.MinReviewers)
	}

	// Get trust scores for all reviewers
	reviewsWithTrust := make([]*ReviewWithTrust, 0, len(reviews))
	var totalTrustWeight float64

	for _, review := range reviews {
		// Get reviewer's trust score
		trustScore, err := e.storage.GetTrustScore(ctx, review.ReviewerID, string(trust.TrustCategoryContent))
		if err != nil {
			// If we can't get trust score, use default
			trustScore = &trust.TrustScore{
				Score:      0.5,
				Confidence: 0.1,
			}
		}

		weight := calculateReviewWeight(trustScore, review)
		reviewsWithTrust = append(reviewsWithTrust, &ReviewWithTrust{
			Review: review,
			Trust:  trustScore,
			Weight: weight,
		})
		totalTrustWeight += weight
	}

	if totalTrustWeight < e.config.MinTrustWeight {
		return nil, fmt.Errorf("insufficient trust weight: %.2f < %.2f", totalTrustWeight, e.config.MinTrustWeight)
	}

	// Calculate weighted consensus
	decision := &ModerationDecision{
		ID:               generateID("decision"),
		EventID:          event.ID,
		ObjectID:         event.ObjectID,
		ReviewerCount:    len(reviews),
		TrustWeightTotal: totalTrustWeight,
		Reviews:          reviews,
	}

	// Group reviews by action
	actionWeights := make(map[ActionType]float64)
	actionCounts := make(map[ActionType]int)

	for _, rwt := range reviewsWithTrust {
		actionWeights[rwt.Review.Action] += rwt.Weight
		actionCounts[rwt.Review.Action]++
	}

	// Find action with highest weighted support
	var bestAction ActionType
	var bestWeight float64

	for action, weight := range actionWeights {
		if weight > bestWeight {
			bestAction = action
			bestWeight = weight
		}
	}

	decision.Action = bestAction
	decision.ConsensusScore = bestWeight / totalTrustWeight

	// Check if consensus meets thresholds
	if decision.ConsensusScore < e.config.ConsensusThreshold {
		return nil, fmt.Errorf("insufficient consensus: %.2f < %.2f",
			decision.ConsensusScore, e.config.ConsensusThreshold)
	}

	// For critical actions, require higher consensus
	if isCriticalAction(bestAction) && decision.ConsensusScore < e.config.CriticalThreshold {
		return nil, fmt.Errorf("insufficient consensus for critical action: %.2f < %.2f",
			decision.ConsensusScore, e.config.CriticalThreshold)
	}

	// Check if escalation is needed
	if decision.ConsensusScore >= e.config.ConsensusThreshold &&
		decision.ConsensusScore < e.config.EscalationThreshold {
		// Mark for escalation/additional review
		decision.Action = ActionTypeWarning // Downgrade to warning pending escalation
	}

	return decision, nil
}

// ProcessReview processes a new review and checks if consensus is reached
func (e *ConsensusEngine) ProcessReview(ctx context.Context, eventID string, review *Review) (*ModerationDecision, error) {
	// Get the event
	event, err := e.storage.GetModerationEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get moderation event: %w", err)
	}

	// Add the review
	err = e.storage.AddModerationReview(ctx, review)
	if err != nil {
		return nil, fmt.Errorf("failed to add review: %w", err)
	}

	// Get all reviews
	reviews, err := e.storage.GetModerationReviews(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reviews: %w", err)
	}

	// Check if we have enough reviews
	if len(reviews) < e.config.MinReviewers {
		return nil, nil // Not enough reviews yet
	}

	// Try to calculate consensus
	decision, err := e.CalculateConsensus(ctx, event, reviews)
	if err != nil {
		// Log but don't fail - consensus might be reached with more reviews
		return nil, nil
	}

	// Store the decision
	err = e.storage.CreateModerationDecision(ctx, decision)
	if err != nil {
		return nil, fmt.Errorf("failed to store decision: %w", err)
	}

	// Update trust scores based on consensus outcome
	go e.updateTrustScores(ctx, decision, reviews)

	return decision, nil
}

// updateTrustScores updates reviewer trust scores based on consensus alignment
func (e *ConsensusEngine) updateTrustScores(ctx context.Context, decision *ModerationDecision, reviews []*Review) {
	for _, review := range reviews {
		var delta float64

		// Calculate trust delta based on alignment with consensus
		if review.Action == decision.Action {
			// Agreed with consensus - small positive update
			delta = 0.01 * decision.ConsensusScore
		} else {
			// Disagreed with consensus - small negative update
			delta = -0.005 * (1 - decision.ConsensusScore)
		}

		// Weight by decision severity
		if decision.Action == ActionTypeSuspend || decision.Action == ActionTypeRemove {
			delta *= 2.0
		}

		update := &trust.TrustUpdate{
			ActorID:  review.ReviewerID,
			Category: trust.TrustCategoryContent,
			Delta:    delta,
			Reason:   fmt.Sprintf("Moderation consensus alignment for event %s", decision.EventID),
			EventID:  decision.EventID,
		}

		e.storage.RecordTrustUpdate(ctx, update)
	}
}

// ReviewWithTrust combines a review with trust information
type ReviewWithTrust struct {
	Review *Review
	Trust  *trust.TrustScore
	Weight float64
}

// calculateReviewWeight calculates the weight of a review based on trust
func calculateReviewWeight(trust *trust.TrustScore, review *Review) float64 {
	// Base weight from trust score (0-1 range)
	baseWeight := (trust.Score + 1.0) / 2.0 // Convert from -1..1 to 0..1

	// Multiply by trust confidence
	weight := baseWeight * trust.Confidence

	// Multiply by review confidence
	weight *= review.Confidence

	// Ensure minimum weight
	if weight < 0.1 {
		weight = 0.1
	}

	return weight
}

// isCriticalAction checks if an action is critical (requires higher consensus)
func isCriticalAction(action ActionType) bool {
	return action == ActionTypeSuspend || action == ActionTypeRemove
}

// generateID generates a unique ID for entities
func generateID(prefix string) string {
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixNano(), generateRandomString(6))
}

// generateRandomString generates a random string (simplified version)
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// CheckTimeouts checks for events that have timed out without consensus
func (e *ConsensusEngine) CheckTimeouts(ctx context.Context) error {
	// This would be called periodically by a Lambda function
	// to auto-decide on events that have been pending too long

	queue, _, err := e.storage.GetModerationQueue(ctx, 100, "")
	if err != nil {
		return fmt.Errorf("failed to get moderation queue: %w", err)
	}

	now := time.Now()
	timeoutDuration := time.Duration(e.config.ReviewTimeoutHours) * time.Hour

	for _, item := range queue {
		if now.Sub(item.Event.Created) > timeoutDuration {
			// Auto-decide based on severity and existing reviews
			decision := &ModerationDecision{
				ID:             generateID("timeout-decision"),
				EventID:        item.Event.ID,
				ObjectID:       item.Event.ObjectID,
				Action:         e.getDefaultAction(item.Event),
				ConsensusScore: 0.0, // No consensus
				ReviewerCount:  item.ReviewCount,
				Decided:        now,
			}

			err = e.storage.CreateModerationDecision(ctx, decision)
			if err != nil {
				// Log error but continue processing other items
				continue
			}
		}
	}

	return nil
}

// getDefaultAction returns a default action based on event severity
func (e *ConsensusEngine) getDefaultAction(event *ModerationEvent) ActionType {
	switch event.Severity {
	case SeverityCritical:
		return ActionTypeSilence // Default to silence for critical
	case SeverityHigh:
		return ActionTypeWarning
	default:
		return ActionTypeNone
	}
}

// GetConsensusStats returns statistics about consensus decisions
func (e *ConsensusEngine) GetConsensusStats(ctx context.Context, startTime, endTime time.Time) (*ConsensusStats, error) {
	// This would query stored decisions and calculate statistics
	// Implementation depends on specific requirements
	return &ConsensusStats{
		TotalDecisions:   0,
		AverageConsensus: 0.0,
		AverageReviewers: 0.0,
		ActionBreakdown:  make(map[ActionType]int),
		TimeToDecision:   0,
	}, nil
}

// ConsensusStats represents statistics about consensus decisions
type ConsensusStats struct {
	TotalDecisions   int
	AverageConsensus float64
	AverageReviewers float64
	ActionBreakdown  map[ActionType]int
	TimeToDecision   time.Duration // Average time from event to decision
}
